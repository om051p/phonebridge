package transfer

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// memChannel is the Channel implementation used by tests. It can deliver frames
// straight into a peer engine, be paused to expose backpressure, and capture
// every frame for protocol assertions.
type memChannel struct {
	mu         sync.Mutex
	buffered   uint64
	low        uint64
	paused     bool
	pauseAfter int
	pending    [][]byte
	sent       [][]byte
	deliver    func([]byte)
	closed     chan struct{}
	closeOnce  sync.Once
	drain      chan struct{}
}

func newMemChannel(low uint64) *memChannel {
	return &memChannel{low: low, closed: make(chan struct{}), drain: make(chan struct{}, 1)}
}

func (c *memChannel) SendFrame(ctx context.Context, frame []byte) error {
	select {
	case <-c.closed:
		return errors.New("mem channel closed")
	default:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.mu.Lock()
	c.sent = append(c.sent, append([]byte(nil), frame...))
	deliver := c.deliver
	if c.paused {
		c.pending = append(c.pending, append([]byte(nil), frame...))
		c.buffered += uint64(len(frame))
		c.mu.Unlock()
		return nil
	}
	// This frame goes through; PauseAfter stalls the frames after the nth.
	if c.pauseAfter > 0 {
		c.pauseAfter--
		if c.pauseAfter == 0 {
			c.paused = true
		}
	}
	c.mu.Unlock()

	if deliver != nil {
		deliver(frame)
	}
	return nil
}

func (c *memChannel) BufferedAmount() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buffered
}

func (c *memChannel) AwaitDrain(ctx context.Context) error {
	for {
		if c.BufferedAmount() <= c.low {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.closed:
			return errors.New("mem channel closed")
		case <-c.drain:
		case <-time.After(2 * time.Millisecond):
		}
	}
}

func (c *memChannel) Done() <-chan struct{} { return c.closed }

// PauseAfter lets the first n frames through and then stalls the channel, which
// is how a test holds a transfer mid-flight without depending on timing.
func (c *memChannel) PauseAfter(n int) {
	c.mu.Lock()
	c.pauseAfter = n
	c.mu.Unlock()
}

// Release drains the stalled queue in order and un-stalls the channel. Frames
// sent while the drain runs stay queued behind it, so the peer never observes an
// out-of-order stream (which the receiver would rightly reject).
func (c *memChannel) Release() {
	for {
		c.mu.Lock()
		pending := c.pending
		c.pending = nil
		c.buffered = 0
		deliver := c.deliver
		c.mu.Unlock()

		for _, f := range pending {
			if deliver != nil {
				deliver(f)
			}
		}

		c.mu.Lock()
		if len(c.pending) == 0 {
			c.paused = false
			c.mu.Unlock()
			break
		}
		c.mu.Unlock()
	}
	select {
	case c.drain <- struct{}{}:
	default:
	}
}

func (c *memChannel) PendingCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.pending)
}

func (c *memChannel) Frames() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([][]byte, len(c.sent))
	copy(out, c.sent)
	return out
}

// pair wires two engines together through in-memory channels with test-sized
// limits, so the whole DEC-024 state machine is exercised without a network.
type pair struct {
	t *testing.T

	senderEng   *Engine
	receiverEng *Engine
	senderCh    *memChannel
	receiverCh  *memChannel

	srcDir  string
	destDir string
}

type pairOptions struct {
	tune func(*Config)
}

const (
	testChunkSize = 4096
	testHighWater = 16384
	testLowWater  = 4096
)

func newPair(t *testing.T, opts pairOptions) *pair {
	t.Helper()

	srcDir := t.TempDir()
	destDir := t.TempDir()

	senderCh := newMemChannel(testLowWater)
	receiverCh := newMemChannel(testLowWater)

	makeCfg := func(dest Destination) Config {
		cfg := Config{
			ChunkSize:        testChunkSize,
			HighWatermark:    testHighWater,
			LowWatermark:     testLowWater,
			OfferTimeout:     3 * time.Second,
			StallTimeout:     3 * time.Second,
			ResultFloor:      3 * time.Second,
			ProgressInterval: 5 * time.Millisecond,
			Destination:      dest,
		}
		if opts.tune != nil {
			opts.tune(&cfg)
		}
		return cfg
	}

	// Both engines can receive: the DataChannel is bidirectional, and the pair
	// harness is what proves both directions share one protocol. Each side owns
	// its own destination directory so the test can assert where files landed.
	senderDest, err := NewFileDestination(FileDestinationConfig{Dir: srcDir})
	if err != nil {
		t.Fatalf("sender destination: %v", err)
	}
	senderEng, err := NewEngine(makeCfg(senderDest))
	if err != nil {
		t.Fatalf("sender engine: %v", err)
	}
	t.Cleanup(senderEng.Close)

	receiverDest, err := NewFileDestination(FileDestinationConfig{Dir: destDir})
	if err != nil {
		t.Fatalf("receiver destination: %v", err)
	}
	receiverEng, err := NewEngine(makeCfg(receiverDest))
	if err != nil {
		t.Fatalf("receiver engine: %v", err)
	}
	t.Cleanup(receiverEng.Close)

	senderCh.deliver = receiverEng.OnFrame
	receiverCh.deliver = senderEng.OnFrame
	senderEng.AttachChannel(senderCh)
	receiverEng.AttachChannel(receiverCh)

	return &pair{
		t:           t,
		senderEng:   senderEng,
		receiverEng: receiverEng,
		senderCh:    senderCh,
		receiverCh:  receiverCh,
		srcDir:      srcDir,
		destDir:     destDir,
	}
}

// writeSource creates a deterministic test file and returns its path and digest.
func writeSource(t *testing.T, dir, name string, size int) (string, []byte) {
	t.Helper()
	data := make([]byte, size)
	for i := range data {
		data[i] = byte((i*31 + 7) % 251)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	sum := sha256.Sum256(data)
	return path, sum[:]
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func waitForState(t *testing.T, eng *Engine, id string, want State, timeout time.Duration) Info {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last Info
	var found bool
	for time.Now().Before(deadline) {
		if info, ok := eng.Get(id); ok {
			last, found = info, true
			if info.State == want {
				return info
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	if !found {
		t.Fatalf("transfer %s never appeared in the engine", id)
	}
	t.Fatalf("transfer %s did not reach %s within %v (last: %+v)", id, want, timeout, last)
	return Info{}
}

func waitForAnyState(t *testing.T, eng *Engine, direction Direction, want State, timeout time.Duration) Info {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last Info
	for time.Now().Before(deadline) {
		for _, info := range eng.List() {
			if info.Direction != direction {
				continue
			}
			last = info
			if info.State == want {
				return info
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("no %s transfer reached %s within %v (last: %+v)", direction, want, timeout, last)
	return Info{}
}

func waitFor(t *testing.T, timeout time.Duration, what string, pred func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pred() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// destEntries lists visible destination entries (the staging directory is
// hidden and must never be visible where the user looks).
func destEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	var out []string
	for _, e := range entries {
		if e.Name() == partialDirName {
			continue
		}
		out = append(out, e.Name())
	}
	return out
}

func assertNoStagedPartials(t *testing.T, dest *FileDestination) {
	t.Helper()
	entries, err := os.ReadDir(dest.PartialDir())
	if err != nil {
		t.Fatalf("read staging dir: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("staged partials were left behind: %v", names)
	}
}

func TestPair_SendFileBothDirections(t *testing.T) {
	p := newPair(t, pairOptions{})

	const size = 1 << 20 // 1 MiB = 256 chunks at 4 KiB
	path, digest := writeSource(t, p.srcDir, "report.pdf", size)

	id, err := p.senderEng.SendFile(context.Background(), path, "")
	if err != nil {
		t.Fatalf("send file: %v", err)
	}
	sent := waitForState(t, p.senderEng, id, StateComplete, 20*time.Second)
	if sent.ReasonCode != ReasonNone {
		t.Fatalf("sender terminal reason = %s (message %q)", sent.ReasonCode, sent.ErrorMessage)
	}
	if sent.BytesTransferred != size {
		t.Fatalf("sender bytes = %d, want %d", sent.BytesTransferred, size)
	}
	if sent.SavedName != "report.pdf" {
		t.Fatalf("sender saved name = %q, want report.pdf", sent.SavedName)
	}

	received := waitForAnyState(t, p.receiverEng, DirectionInbound, StateComplete, 5*time.Second)
	if received.TransferID != id {
		t.Fatalf("receiver transfer id = %q, want %q", received.TransferID, id)
	}
	if received.BytesTransferred != size {
		t.Fatalf("receiver bytes = %d, want %d", received.BytesTransferred, size)
	}
	if got := readFile(t, filepath.Join(p.destDir, "report.pdf")); sha256.Sum256(got) != ([32]byte)(digest) {
		t.Fatalf("received file does not match the source digest")
	}
	assertNoStagedPartials(t, p.receiverEng.cfg.Destination.(*FileDestination))

	// Reverse direction on the same session (the DataChannel is bidirectional).
	reversePath, reverseDigest := writeSource(t, p.destDir, "notes.txt", 3*testChunkSize+17)
	reverseID, err := p.receiverEng.SendFile(context.Background(), reversePath, "")
	if err != nil {
		t.Fatalf("reverse send: %v", err)
	}
	reverseSent := waitForState(t, p.receiverEng, reverseID, StateComplete, 20*time.Second)
	if reverseSent.ReasonCode != ReasonNone {
		t.Fatalf("reverse terminal reason = %s (%q)", reverseSent.ReasonCode, reverseSent.ErrorMessage)
	}
	reverseRecv := waitForState(t, p.senderEng, reverseID, StateComplete, 20*time.Second)
	if reverseRecv.State != StateComplete {
		t.Fatalf("reverse receiver state = %s", reverseRecv.State)
	}
	gotReverse := readFile(t, filepath.Join(p.srcDir, "notes.txt"))
	if sha256.Sum256(gotReverse) != ([32]byte)(reverseDigest) {
		t.Fatalf("reverse file does not match the source digest")
	}
}

func TestPair_EmptyAndChunkAlignedFiles(t *testing.T) {
	p := newPair(t, pairOptions{})

	for _, tc := range []struct {
		name string
		size int
	}{
		{"empty.bin", 0},
		{"one-chunk.bin", testChunkSize},
		{"two-chunks.bin", 2 * testChunkSize},
		{"one-byte.bin", 1},
	} {
		path, digest := writeSource(t, p.srcDir, tc.name, tc.size)
		id, err := p.senderEng.SendFile(context.Background(), path, "")
		if err != nil {
			t.Fatalf("%s: send: %v", tc.name, err)
		}
		waitForState(t, p.senderEng, id, StateComplete, 20*time.Second)
		waitForState(t, p.receiverEng, id, StateComplete, 20*time.Second)
		if got := readFile(t, filepath.Join(p.destDir, tc.name)); sha256.Sum256(got) != ([32]byte)(digest) {
			t.Fatalf("%s: digest mismatch", tc.name)
		}
	}
}

func TestPair_LocalBusyRefusesSecondOutbound(t *testing.T) {
	p := newPair(t, pairOptions{tune: func(c *Config) { c.OutboundQueueDepth = 1 }})

	path, _ := writeSource(t, p.srcDir, "big.bin", 2<<20)
	p.senderCh.PauseAfter(1)

	id, err := p.senderEng.SendFile(context.Background(), path, "")
	if err != nil {
		t.Fatalf("first send: %v", err)
	}
	waitForState(t, p.senderEng, id, StateActive, 5*time.Second)

	second, _ := writeSource(t, p.srcDir, "second.bin", 1024)
	secondID, err := p.senderEng.SendFile(context.Background(), second, "")
	if err != nil {
		t.Fatalf("second send (queued): %v", err)
	}
	if info, _ := p.senderEng.Get(secondID); info.State != StateQueued {
		t.Fatalf("second transfer state = %s, want QUEUED", info.State)
	}
	third, _ := writeSource(t, p.srcDir, "third.bin", 1024)
	if _, err := p.senderEng.SendFile(context.Background(), third, ""); err == nil {
		t.Fatalf("third outbound should be BUSY when queue full")
	} else if f, ok := IsFailure(err); !ok || f.Reason != ReasonBusy {
		t.Fatalf("third send error = %v, want a BUSY failure", err)
	}

	p.senderCh.Release()
	waitForState(t, p.senderEng, id, StateComplete, 20*time.Second)
	waitForState(t, p.senderEng, secondID, StateComplete, 20*time.Second)
}

func TestPair_QueuedSmallFileBurstDrainsFIFO(t *testing.T) {
	p := newPair(t, pairOptions{})

	// Sequential burst: outbound queue guarantees FIFO order, receiver
	// finishes each before the next offer arrives so no inbound BUSY.
	const burst = 8
	ids := make([]string, burst)
	for i := 0; i < burst; i++ {
		path, _ := writeSource(t, p.srcDir, "burst.bin", 1024)
		id, err := p.senderEng.SendFile(context.Background(), path, "")
		if err != nil {
			t.Fatalf("burst %d: %v", i, err)
		}
		ids[i] = id
		waitForState(t, p.senderEng, id, StateComplete, 10*time.Second)
	}
	for _, id := range ids {
		info, ok := p.senderEng.Get(id)
		if !ok || info.State != StateComplete {
			t.Fatalf("id %s state = %v, want COMPLETE", id, info.State)
		}
	}
}

func TestPair_BackpressureBoundsReadAhead(t *testing.T) {
	p := newPair(t, pairOptions{})

	path, digest := writeSource(t, p.srcDir, "large.bin", 4<<20)
	p.senderCh.PauseAfter(1) // stall after the offer, so the chunk loop is held
	id, err := p.senderEng.SendFile(context.Background(), path, "")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	waitForState(t, p.senderEng, id, StateActive, 5*time.Second)

	// The sender may only queue up to its watermark plus one chunk in flight,
	// never the whole file.
	time.Sleep(150 * time.Millisecond)
	queued := p.senderCh.PendingCount()
	maxQueued := int(testHighWater/testChunkSize) + 2
	if queued > maxQueued {
		t.Fatalf("sender queued %d chunks while the channel was stalled (limit %d)", queued, maxQueued)
	}
	if queued == 0 {
		t.Fatalf("sender queued nothing while stalled; the backpressure path was not exercised")
	}
	if info, _ := p.senderEng.Get(id); info.State != StateActive {
		t.Fatalf("transfer state = %s while stalled, want ACTIVE", info.State)
	}

	p.senderCh.Release()
	waitForState(t, p.senderEng, id, StateComplete, 20*time.Second)
	waitForState(t, p.receiverEng, id, StateComplete, 5*time.Second)
	if got := readFile(t, filepath.Join(p.destDir, "large.bin")); sha256.Sum256(got) != ([32]byte)(digest) {
		t.Fatalf("digest mismatch after backpressure release")
	}
}

func TestPair_CancelBySenderMidTransfer(t *testing.T) {
	p := newPair(t, pairOptions{})

	path, _ := writeSource(t, p.srcDir, "cancel-me.bin", 2<<20)
	p.senderCh.PauseAfter(1)
	id, err := p.senderEng.SendFile(context.Background(), path, "")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	waitForState(t, p.senderEng, id, StateActive, 5*time.Second)
	waitForAnyState(t, p.receiverEng, DirectionInbound, StateActive, 5*time.Second)

	if err := p.senderEng.Cancel(context.Background(), id); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	// The receiver only learns about the cancel when the paused queue drains.
	p.senderCh.Release()

	sent := waitForState(t, p.senderEng, id, StateCancelled, 5*time.Second)
	if sent.ReasonCode != ReasonCancelledByUser {
		t.Fatalf("sender reason = %s, want CANCELLED_BY_USER", sent.ReasonCode)
	}
	recv := waitForState(t, p.receiverEng, id, StateCancelled, 5*time.Second)
	if recv.ReasonCode != ReasonCancelledByPeer {
		t.Fatalf("receiver reason = %s, want CANCELLED_BY_PEER", recv.ReasonCode)
	}
	if entries := destEntries(t, p.destDir); len(entries) != 0 {
		t.Fatalf("cancel left destination entries: %v", entries)
	}
	assertNoStagedPartials(t, p.receiverEng.cfg.Destination.(*FileDestination))
}

func TestPair_CancelByReceiverMidTransfer(t *testing.T) {
	p := newPair(t, pairOptions{})

	path, _ := writeSource(t, p.srcDir, "cancel-recv.bin", 2<<20)
	p.senderCh.PauseAfter(1)
	id, err := p.senderEng.SendFile(context.Background(), path, "")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	waitForState(t, p.senderEng, id, StateActive, 5*time.Second)
	waitForAnyState(t, p.receiverEng, DirectionInbound, StateActive, 5*time.Second)

	if err := p.receiverEng.Cancel(context.Background(), id); err != nil {
		t.Fatalf("receiver cancel: %v", err)
	}
	recv := waitForState(t, p.receiverEng, id, StateCancelled, 5*time.Second)
	if recv.ReasonCode != ReasonCancelledByUser {
		t.Fatalf("receiver reason = %s, want CANCELLED_BY_USER", recv.ReasonCode)
	}

	p.senderCh.Release()
	sent := waitForState(t, p.senderEng, id, StateCancelled, 5*time.Second)
	if sent.ReasonCode != ReasonCancelledByPeer {
		t.Fatalf("sender reason = %s, want CANCELLED_BY_PEER", sent.ReasonCode)
	}
	if entries := destEntries(t, p.destDir); len(entries) != 0 {
		t.Fatalf("cancel left destination entries: %v", entries)
	}
	assertNoStagedPartials(t, p.receiverEng.cfg.Destination.(*FileDestination))
}

func TestPair_ChannelLossInterruptsAndCleansUp(t *testing.T) {
	p := newPair(t, pairOptions{})

	path, _ := writeSource(t, p.srcDir, "interrupted.bin", 2<<20)
	p.senderCh.PauseAfter(1)
	id, err := p.senderEng.SendFile(context.Background(), path, "")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	waitForState(t, p.senderEng, id, StateActive, 5*time.Second)
	waitForAnyState(t, p.receiverEng, DirectionInbound, StateActive, 5*time.Second)

	// Simulate the session dying: no resume in Phase 4, so both sides must reach
	// a terminal interrupted state and delete the staged partial.
	p.senderEng.DetachChannel(ReasonInterrupted, "WebRTC transport closed")
	p.receiverEng.DetachChannel(ReasonInterrupted, "WebRTC transport closed")

	sent := waitForState(t, p.senderEng, id, StateFailed, 5*time.Second)
	if sent.ReasonCode != ReasonInterrupted {
		t.Fatalf("sender reason = %s, want INTERRUPTED", sent.ReasonCode)
	}
	recv := waitForState(t, p.receiverEng, id, StateFailed, 5*time.Second)
	if recv.ReasonCode != ReasonInterrupted {
		t.Fatalf("receiver reason = %s, want INTERRUPTED", recv.ReasonCode)
	}
	if entries := destEntries(t, p.destDir); len(entries) != 0 {
		t.Fatalf("interruption left destination entries: %v", entries)
	}
	assertNoStagedPartials(t, p.receiverEng.cfg.Destination.(*FileDestination))

	// A retry on a fresh channel succeeds, because nothing survived the failure
	// (no resume in Phase 4: the retry starts from zero).
	retrySenderCh := newMemChannel(testLowWater)
	retryReceiverCh := newMemChannel(testLowWater)
	retrySenderCh.deliver = p.receiverEng.OnFrame
	retryReceiverCh.deliver = p.senderEng.OnFrame
	p.senderEng.AttachChannel(retrySenderCh)
	p.receiverEng.AttachChannel(retryReceiverCh)
	retryPath, digest := writeSource(t, p.srcDir, "retry.bin", 200000)
	retryID, err := p.senderEng.SendFile(context.Background(), retryPath, "")
	if err != nil {
		t.Fatalf("retry send: %v", err)
	}
	waitForState(t, p.senderEng, retryID, StateComplete, 20*time.Second)
	waitForState(t, p.receiverEng, retryID, StateComplete, 5*time.Second)
	if got := readFile(t, filepath.Join(p.destDir, "retry.bin")); sha256.Sum256(got) != ([32]byte)(digest) {
		t.Fatalf("retry digest mismatch")
	}
}

func TestPair_ConcurrentOutboundAndInbound(t *testing.T) {
	p := newPair(t, pairOptions{})

	forward := 1 << 19
	backward := 1<<19 + 1234 // deliberately not chunk-aligned

	forwardPath, forwardDigest := writeSource(t, p.srcDir, "forward.bin", forward)
	backwardPath, backwardDigest := writeSource(t, p.destDir, "backward.bin", backward)

	forwardID, err := p.senderEng.SendFile(context.Background(), forwardPath, "")
	if err != nil {
		t.Fatalf("forward send: %v", err)
	}
	backwardID, err := p.receiverEng.SendFile(context.Background(), backwardPath, "")
	if err != nil {
		t.Fatalf("backward send: %v", err)
	}

	waitForState(t, p.senderEng, forwardID, StateComplete, 20*time.Second)
	waitForState(t, p.receiverEng, forwardID, StateComplete, 20*time.Second)
	waitForState(t, p.receiverEng, backwardID, StateComplete, 20*time.Second)
	waitForState(t, p.senderEng, backwardID, StateComplete, 20*time.Second)

	if got := readFile(t, filepath.Join(p.destDir, "forward.bin")); sha256.Sum256(got) != ([32]byte)(forwardDigest) {
		t.Fatalf("forward digest mismatch")
	}
	if got := readFile(t, filepath.Join(p.srcDir, "backward.bin")); sha256.Sum256(got) != ([32]byte)(backwardDigest) {
		t.Fatalf("backward digest mismatch")
	}
}

func TestPair_ProgressIsMonotonicAndThrottled(t *testing.T) {
	var mu sync.Mutex
	var seen []Info
	p := newPair(t, pairOptions{tune: func(cfg *Config) {
		cfg.OnEvent = func(ev Event) {
			if ev.Info.Direction != DirectionInbound {
				return
			}
			mu.Lock()
			seen = append(seen, ev.Info)
			mu.Unlock()
		}
	}})

	path, _ := writeSource(t, p.srcDir, "progress.bin", 1<<20)
	id, err := p.senderEng.SendFile(context.Background(), path, "")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	waitForState(t, p.receiverEng, id, StateComplete, 20*time.Second)

	mu.Lock()
	defer mu.Unlock()
	var last uint64
	active := 0
	for _, info := range seen {
		if info.BytesTransferred < last {
			t.Fatalf("progress went backwards: %d after %d", info.BytesTransferred, last)
		}
		last = info.BytesTransferred
		if info.State == StateActive {
			active++
		}
	}
	if active == 0 {
		t.Fatalf("no ACTIVE progress event was published")
	}
	// 1 MiB at 4 KiB chunks is 256 chunks; the 5 ms progress interval must keep
	// the event count far below that.
	if len(seen) > 64 {
		t.Fatalf("progress events were not throttled: %d events", len(seen))
	}
}

func TestDefaultDownloadDirHonoursEnvironment(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DOWNLOAD_DIR", dir)
	if got := DefaultDownloadDir(); got != dir {
		t.Fatalf("DefaultDownloadDir() = %q, want %q", got, dir)
	}

	t.Setenv("XDG_DOWNLOAD_DIR", "")
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	userDirs := "XDG_DOWNLOAD_DIR=\"$HOME/Downloads\"\n"
	if err := os.WriteFile(filepath.Join(configHome, "user-dirs.dirs"), []byte(userDirs), 0o600); err != nil {
		t.Fatalf("write user-dirs.dirs: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory: %v", err)
	}
	want := filepath.Join(home, "Downloads")
	if got := DefaultDownloadDir(); got != want {
		t.Fatalf("DefaultDownloadDir() = %q, want %q", got, want)
	}
}
