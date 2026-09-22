//go:build android || jni

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/transfer"
)

// fakeTransferHost stands in for Kotlin. It hands out a real file descriptor to
// a file in a temp dir, which is exactly what MediaStore/SAF do, so the bytes
// the destination writes are asserted on disk.
type fakeTransferHost struct {
	mu sync.Mutex

	dir string

	beginErr    error
	fd          int
	commitOK    bool
	freeSpace   int64
	oversized   []int
	begins      []beginCall
	commits     []string
	aborts      []string
	descriptors []string
}

type beginCall struct {
	filename string
	mimeType string
	size     int64
}

func newFakeTransferHost(t *testing.T) *fakeTransferHost {
	t.Helper()
	return &fakeTransferHost{dir: t.TempDir(), commitOK: true, freeSpace: 1 << 40, fd: -1}
}

func (h *fakeTransferHost) BeginDownload(filename, mimeType string, sizeBytes int64) (string, int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.begins = append(h.begins, beginCall{filename: filename, mimeType: mimeType, size: sizeBytes})
	if h.beginErr != nil {
		return "", -1, h.beginErr
	}
	path := filepath.Join(h.dir, filename)
	f, err := os.Create(path)
	if err != nil {
		return "", -1, err
	}
	h.descriptors = append(h.descriptors, path)
	fd := int(f.Fd())
	h.fd = fd
	// The platform keeps its own handle open, like a MediaStore pending row.
	return "handle-" + filename, fd, nil
}

func (h *fakeTransferHost) CommitDownload(handle string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.commits = append(h.commits, handle)
	if !h.commitOK {
		return "", false
	}
	return handle[len("handle-"):], true
}

func (h *fakeTransferHost) AbortDownload(handle string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.aborts = append(h.aborts, handle)
}

func (h *fakeTransferHost) FreeSpaceBytes() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.freeSpace
}

func (h *fakeTransferHost) OnOversizedFrame(size int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.oversized = append(h.oversized, size)
}

func (h *fakeTransferHost) lastDescriptor(t *testing.T) []byte {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.descriptors) == 0 {
		t.Fatal("no descriptor was handed out")
	}
	data, err := os.ReadFile(h.descriptors[len(h.descriptors)-1])
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	return data
}

// TestPlatformDestination_WritesInBoundedPieces pins the two properties that
// make Android storage safe under a 16 GiB policy: bytes reach the platform's
// descriptor incrementally (never accumulated whole in memory), and the entry is
// only committed after every write succeeded.
func TestPlatformDestination_WritesInBoundedPieces(t *testing.T) {
	host := newFakeTransferHost(t)
	dest := NewPlatformDestination(host)

	committer, err := dest.Begin(transfer.Meta{
		TransferID: "t-1",
		Filename:   "holiday.jpg",
		MimeType:   "image/jpeg",
		SizeBytes:  512 * 1024,
	})
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	payload := bytes.Repeat([]byte{0xA5}, 512*1024)
	if _, err := committer.Write(payload); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Before Commit the platform must not be told the file is ready.
	if got := len(host.commits); got != 0 {
		t.Fatalf("commit happened %d times before Commit()", got)
	}

	saved, err := committer.Commit()
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if saved != "holiday.jpg" {
		t.Fatalf("saved name = %q", saved)
	}
	if got := host.lastDescriptor(t); !bytes.Equal(got, payload) {
		t.Fatalf("descriptor received %d bytes, want %d identical bytes", len(got), len(payload))
	}
	if len(host.commits) != 1 || host.commits[0] != "handle-holiday.jpg" {
		t.Fatalf("commits = %v", host.commits)
	}
	if len(host.aborts) != 0 {
		t.Fatalf("aborts = %v", host.aborts)
	}
	if host.begins[0].mimeType != "image/jpeg" || host.begins[0].size != 512*1024 {
		t.Fatalf("begin call = %+v", host.begins[0])
	}
}

func TestPlatformDestination_FreeSpacePolicy(t *testing.T) {
	host := newFakeTransferHost(t)
	host.freeSpace = 1024 // far below size + margin
	dest := NewPlatformDestination(host)

	_, err := dest.Begin(transfer.Meta{TransferID: "t-2", Filename: "big.iso", SizeBytes: 4096})
	failure, ok := transfer.IsFailure(err)
	if !ok {
		t.Fatalf("want a typed failure, got %v", err)
	}
	if failure.Code.String() != "CODE_STORAGE_FAILED" {
		t.Fatalf("code = %s", failure.Code)
	}
	if len(host.begins) != 0 {
		t.Fatal("the platform must not be asked to create an entry when space is short")
	}

	// An unknown free-space answer (negative) skips the policy instead of
	// refusing a download the platform may well be able to store.
	host.freeSpace = -1
	if _, err := dest.Begin(transfer.Meta{TransferID: "t-3", Filename: "big.iso", SizeBytes: 4096}); err != nil {
		t.Fatalf("unknown free space must not refuse: %v", err)
	}
}

func TestPlatformDestination_AbortDiscardsTheEntry(t *testing.T) {
	host := newFakeTransferHost(t)
	dest := NewPlatformDestination(host)

	committer, err := dest.Begin(transfer.Meta{TransferID: "t-4", Filename: "partial.bin", SizeBytes: 64})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := committer.Write([]byte("half a file")); err != nil {
		t.Fatal(err)
	}
	if err := committer.Abort(); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	if len(host.aborts) != 1 || host.aborts[0] != "handle-partial.bin" {
		t.Fatalf("aborts = %v", host.aborts)
	}
	if len(host.commits) != 0 {
		t.Fatalf("commits = %v", host.commits)
	}
	// A second Abort is a no-op: the engine may abort defensively.
	if err := committer.Abort(); err != nil {
		t.Fatalf("second Abort: %v", err)
	}
	if len(host.aborts) != 1 {
		t.Fatalf("second Abort must not touch the platform again: %v", host.aborts)
	}
}

func TestPlatformDestination_PublishFailureIsTypedAndCleanedUp(t *testing.T) {
	host := newFakeTransferHost(t)
	host.commitOK = false
	dest := NewPlatformDestination(host)

	committer, err := dest.Begin(transfer.Meta{TransferID: "t-5", Filename: "unpublishable.bin", SizeBytes: 32})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := committer.Write([]byte("bytes that will not persist")); err != nil {
		t.Fatal(err)
	}
	_, err = committer.Commit()
	failure, ok := transfer.IsFailure(err)
	if !ok {
		t.Fatalf("want a typed failure, got %v", err)
	}
	if failure.Reason != transfer.ReasonStorageFailed {
		t.Fatalf("reason = %s", failure.Reason)
	}
	if len(host.aborts) != 1 {
		t.Fatalf("a failed publish must delete the entry: aborts = %v", host.aborts)
	}
}

func TestPlatformDestination_RefusesWithoutHost(t *testing.T) {
	dest := NewPlatformDestination(nil)
	_, err := dest.Begin(transfer.Meta{TransferID: "t-6", Filename: "x.bin"})
	if _, ok := transfer.IsFailure(err); !ok {
		t.Fatalf("want a typed failure, got %v", err)
	}
}

// TestTransferBridge_EngineLifecycle drives the bridge the way Kotlin does and
// checks the observable contract: no engine before Init, idempotent Init, flat
// list/stats output, and a Stop that leaves the channel detached.
func TestTransferBridge_EngineLifecycle(t *testing.T) {
	host := newFakeTransferHost(t)
	bridge := &TransferBridge{}

	if bridge.ChannelReady() {
		t.Fatal("no engine yet, so no channel can be ready")
	}
	if err := bridge.Init(nil, "device-x"); err == nil {
		t.Fatal("Init without a storage host must fail")
	}

	if err := bridge.Init(host, "device-x"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := bridge.Init(host, "device-x"); err != nil {
		t.Fatalf("second Init must be a no-op: %v", err)
	}

	// Sending without a session fails with the engine's typed refusal; the
	// bridge must not invent its own error shape.
	if _, err := bridge.SendFile("/data/app/staging/report.pdf", "report.pdf"); err == nil {
		t.Fatal("sending without a transfer channel must fail")
	} else if _, ok := transfer.IsFailure(err); !ok {
		t.Fatalf("want a typed failure, got %v", err)
	}

	if got := bridge.List(); len(got) != 0 {
		t.Fatalf("history should be empty, got %d entries", len(got))
	}

	var rows []map[string]any
	if err := json.Unmarshal(bridge.ListJSON(), &rows); err != nil {
		t.Fatalf("ListJSON: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %v", rows)
	}

	stats := bridge.Stats()
	if !stats.Initialized {
		t.Fatal("stats must report the initialized state")
	}
	if stats.ChannelReady {
		t.Fatal("stats must not report a channel before one is attached")
	}
	var decoded map[string]any
	if err := json.Unmarshal(bridge.StatsJSON(), &decoded); err != nil {
		t.Fatalf("StatsJSON: %v", err)
	}
	if decoded["initialized"] != true {
		t.Fatalf("stats json = %v", decoded)
	}

	bridge.Stop()
	if bridge.ChannelReady() {
		t.Fatal("Stop must release the engine")
	}
	if bridge.Stats().Initialized {
		t.Fatal("stats must report the stopped state")
	}
	// Stop is idempotent: the service may be torn down twice.
	bridge.Stop()
}

// TestTransferBridge_EndToEndOverFakeChannel proves the Android wiring carries a
// real file through the real engine and the platform destination, with the
// in-memory channel standing in for the Pion DataChannel.
func TestTransferBridge_EndToEndOverFakeChannel(t *testing.T) {
	senderHost := newFakeTransferHost(t)
	receiverHost := newFakeTransferHost(t)

	sender := &TransferBridge{}
	if err := sender.Init(senderHost, "android-sender"); err != nil {
		t.Fatal(err)
	}
	defer sender.Stop()

	receiver := &TransferBridge{}
	if err := receiver.Init(receiverHost, "android-receiver"); err != nil {
		t.Fatal(err)
	}
	defer receiver.Stop()

	link := newFakeLink(t, sender, receiver)

	// Android is always the offerer, so it owns the channel and both ends attach
	// the moment their side reports open.
	sender.OnChannelOpen(link.a, "linux-peer")
	receiver.OnChannelOpen(link.b, "android-sender")

	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "notes.txt")
	payload := []byte("phase 4 android transfer bridge\n")
	if err := os.WriteFile(srcPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	id, err := sender.SendFile(srcPath, "notes.txt")
	if err != nil {
		t.Fatalf("SendFile: %v", err)
	}

	final := waitBridgeTransfer(t, receiver, id)
	if final.State != transfer.StateComplete {
		t.Fatalf("receiver state = %s (%s: %s)", final.State, final.ReasonCode, final.ErrorMessage)
	}
	if final.PeerDeviceID != "android-sender" {
		t.Fatalf("receiver attributed the transfer to %q", final.PeerDeviceID)
	}
	got := receiverHost.lastDescriptor(t)
	if !bytes.Equal(got, payload) {
		t.Fatalf("received %q, want %q", got, payload)
	}
	if sha256.Sum256(got) != sha256.Sum256(payload) {
		t.Fatal("digest mismatch")
	}
	if sender.Stats().SentFiles != 1 {
		t.Fatalf("sender stats = %+v", sender.Stats())
	}
	if receiver.Stats().ReceivedFiles != 1 {
		t.Fatalf("receiver stats = %+v", receiver.Stats())
	}
	if receiverHost.commits[0] == "" {
		t.Fatal("the platform must have been asked to publish the file")
	}

	// Closing the channel must interrupt rather than leave anything pending.
	link.close()
	receiver.OnChannelClose(link.b)
	if receiver.ChannelReady() {
		t.Fatal("channel must be detached after close")
	}
}

// waitBridgeTransfer polls the bridge until the transfer reaches a final state.
func waitBridgeTransfer(t *testing.T, bridge *TransferBridge, id string) transfer.Info {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var last transfer.Info
	for time.Now().Before(deadline) {
		for _, info := range bridge.List() {
			if info.TransferID != id {
				continue
			}
			last = info
			switch info.State {
			case transfer.StateComplete, transfer.StateCancelled, transfer.StateFailed:
				return info
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("transfer %s did not finish (last state %s)", id, last.State)
	return transfer.Info{}
}

// fakeXferChannel is an in-memory transfer.Channel that delivers each frame
// straight to the peer bridge, so an Android-side transfer runs through the real
// engine and the real platform destination without a network.
type fakeXferChannel struct {
	mu      sync.Mutex
	deliver func([]byte)
	closed  bool
	done    chan struct{}
	sent    int
}

var _ transfer.Channel = (*fakeXferChannel)(nil)

func (c *fakeXferChannel) SendFrame(ctx context.Context, frame []byte) error {
	c.mu.Lock()
	closed := c.closed
	c.sent++
	deliver := c.deliver
	c.mu.Unlock()
	if closed {
		return errFakeClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	deliver(frame)
	return nil
}

func (c *fakeXferChannel) BufferedAmount() uint64 { return 0 }

func (c *fakeXferChannel) AwaitDrain(ctx context.Context) error {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return errFakeClosed
	}
	return ctx.Err()
}

func (c *fakeXferChannel) Done() <-chan struct{} { return c.done }

func (c *fakeXferChannel) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	close(c.done)
}

type fakeLink struct {
	a *fakeXferChannel
	b *fakeXferChannel
}

func newFakeLink(t *testing.T, sender, receiver *TransferBridge) *fakeLink {
	t.Helper()
	// a is the sender's channel (frames land on the receiver); b is the reverse.
	a := &fakeXferChannel{done: make(chan struct{}), deliver: func(b []byte) { _ = receiver.OnRemoteBytes(b) }}
	b := &fakeXferChannel{done: make(chan struct{}), deliver: func(f []byte) { _ = sender.OnRemoteBytes(f) }}
	return &fakeLink{a: a, b: b}
}

func (l *fakeLink) close() {
	l.a.close()
	l.b.close()
}

var errFakeClosed = errors.New("fake channel closed")
