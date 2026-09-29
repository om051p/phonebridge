package transfer

import (
	"context"
	"crypto/sha256"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

// eventRecorder captures the engine's event stream so a driver can assert the
// exact transition sequence and ordering.
type eventRecorder struct {
	mu     sync.Mutex
	events []Info
}

func (r *eventRecorder) record(ev Event) {
	r.mu.Lock()
	r.events = append(r.events, ev.Info)
	r.mu.Unlock()
}

func (r *eventRecorder) snapshot() []Info {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Info, len(r.events))
	copy(out, r.events)
	return out
}

// statesFor returns the outbound (sender-side) transition stream for one id.
// newPair applies tune to BOTH engines, so the recorder sees the receiver's
// inbound stream too; the same id appears in each, so direction must isolate it.
func (r *eventRecorder) statesFor(id string) []State {
	var out []State
	for _, ev := range r.snapshot() {
		if ev.TransferID == id && ev.Direction == DirectionOutbound {
			out = append(out, ev.State)
		}
	}
	return out
}

// cancelBeforeOfferMonitor asserts the protocol ordering Cancel depends on: a
// FileCancel must never be emitted for a transfer whose FileOffer has not been
// sent. A cancel that overtakes its offer is ignored by the peer as an unknown
// id; the peer then accepts the offer and holds an accepted inbound for a
// transfer the sender has abandoned, wedging its inbound slot (DEC-024).
type cancelBeforeOfferMonitor struct {
	mu      sync.Mutex
	offered map[string]bool
	bad     []string
}

func newCancelBeforeOfferMonitor() *cancelBeforeOfferMonitor {
	return &cancelBeforeOfferMonitor{offered: map[string]bool{}}
}

func (m *cancelBeforeOfferMonitor) observe(frame []byte) {
	f, err := DecodeFrame(frame)
	if err != nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	switch body := f.Body.(type) {
	case *phonebridgev1.TransferFrame_Offer:
		m.offered[body.Offer.TransferId] = true
	case *phonebridgev1.TransferFrame_Cancel:
		if !m.offered[body.Cancel.TransferId] {
			m.bad = append(m.bad, body.Cancel.TransferId)
		}
	}
}

func (m *cancelBeforeOfferMonitor) violations() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.bad...)
}

// dedupeStates collapses repeated emits (progress re-emits ACTIVE) so a test can
// assert the ordered set of distinct transitions.
func dedupeStates(states []State) []State {
	var out []State
	for _, s := range states {
		if len(out) == 0 || out[len(out)-1] != s {
			out = append(out, s)
		}
	}
	return out
}

// TestBehavior_QueueSerializesFifoAndLandsBytes drives the whole outbound queue
// lifecycle: one transfer held on the wire, three queued behind it, then release.
// It asserts (a) only one outbound is ever on the wire, (b) activation order is
// FIFO, (c) the per-transfer transition sequence is QUEUED->PENDING->ACTIVE->
// COMPLETE, and (d) every file lands byte-exact at the destination.
func TestBehavior_QueueSerializesFifoAndLandsBytes(t *testing.T) {
	rec := &eventRecorder{}
	p := newPair(t, pairOptions{tune: func(c *Config) { c.OnEvent = rec.record }})

	p.senderCh.PauseAfter(1)

	names := []string{"a.bin", "b.bin", "c.bin", "d.bin"}
	sizes := []int{1 << 20, 8192, 12000, 4096}
	ids := make([]string, len(names))
	digests := make([][32]byte, len(names))
	for i, name := range names {
		path, dig := writeSource(t, p.srcDir, name, sizes[i])
		digests[i] = [32]byte(dig)
		id, err := p.senderEng.SendFile(context.Background(), path, "")
		if err != nil {
			t.Fatalf("%s: send: %v", name, err)
		}
		ids[i] = id
	}

	waitForState(t, p.senderEng, ids[0], StateActive, 5*time.Second)
	for _, id := range ids[1:] {
		if info, _ := p.senderEng.Get(id); info.State != StateQueued {
			t.Fatalf("transfer %s state = %s, want QUEUED", id, info.State)
		}
	}
	t.Logf("observed: head %s ACTIVE, %d queued", ids[0], len(ids)-1)

	p.senderCh.Release()
	for i, id := range ids {
		info := waitForState(t, p.senderEng, id, StateComplete, 20*time.Second)
		if info.ReasonCode != ReasonNone {
			t.Fatalf("%s: reason = %s (%q)", names[i], info.ReasonCode, info.ErrorMessage)
		}
	}

	for i, name := range names {
		got := readFile(t, filepath.Join(p.destDir, name))
		if sha256.Sum256(got) != digests[i] {
			t.Fatalf("%s: received bytes do not match the source digest", name)
		}
	}

	// (c) transition sequence for the queued transfers: QUEUED, PENDING, ACTIVE,
	// COMPLETE, with repeated progress emits collapsed.
	for i := 1; i < len(ids); i++ {
		got := dedupeStates(rec.statesFor(ids[i]))
		want := []State{StateQueued, StatePending, StateActive, StateComplete}
		if len(got) != len(want) {
			t.Fatalf("%s transition sequence = %v, want %v", names[i], got, want)
		}
		for j := range want {
			if got[j] != want[j] {
				t.Fatalf("%s transition sequence = %v, want %v", names[i], got, want)
			}
		}
	}

	// (a)+(b) replay the stream: at most one on the wire, activation is FIFO.
	onWire := map[string]bool{}
	var activated []string
	for _, ev := range rec.snapshot() {
		if ev.Direction != DirectionOutbound {
			continue
		}
		switch ev.State {
		case StatePending, StateActive, StateVerifying:
			onWire[ev.TransferID] = true
		case StateComplete, StateCancelled, StateFailed:
			delete(onWire, ev.TransferID)
		}
		if len(onWire) > 1 {
			t.Fatalf("two outbound transfers on the wire at once: %v", onWire)
		}
		if ev.State == StateActive && !hasString(activated, ev.TransferID) {
			activated = append(activated, ev.TransferID)
		}
	}
	if len(activated) != len(ids) {
		t.Fatalf("activated %d of %d transfers: %v", len(activated), len(ids), activated)
	}
	for i := range ids {
		if activated[i] != ids[i] {
			t.Fatalf("activation order = %v, want %v", activated, ids)
		}
	}
	t.Logf("observed FIFO activation order confirmed: %v", activated)
}

// TestBehavior_CancelWhileQueuedKeepsQueueDraining cancels a queued transfer and
// asserts it never touches the wire, that the head is undisturbed, and that the
// transfer behind it still runs to completion.
func TestBehavior_CancelWhileQueuedKeepsQueueDraining(t *testing.T) {
	rec := &eventRecorder{}
	p := newPair(t, pairOptions{tune: func(c *Config) { c.OnEvent = rec.record }})

	ordering := newCancelBeforeOfferMonitor()
	p.senderCh.Observe(ordering.observe)

	p.senderCh.PauseAfter(1)
	head, _ := writeSource(t, p.srcDir, "head.bin", 1<<20)
	headID, err := p.senderEng.SendFile(context.Background(), head, "")
	if err != nil {
		t.Fatalf("head send: %v", err)
	}
	waitForState(t, p.senderEng, headID, StateActive, 5*time.Second)

	victim, _ := writeSource(t, p.srcDir, "victim.bin", 8192)
	victimID, err := p.senderEng.SendFile(context.Background(), victim, "")
	if err != nil {
		t.Fatalf("victim send: %v", err)
	}
	tail, tailDig := writeSource(t, p.srcDir, "tail.bin", 16384)
	tailID, err := p.senderEng.SendFile(context.Background(), tail, "")
	if err != nil {
		t.Fatalf("tail send: %v", err)
	}

	if err := p.senderEng.Cancel(context.Background(), victimID); err != nil {
		t.Fatalf("cancel queued: %v", err)
	}
	info := waitForState(t, p.senderEng, victimID, StateCancelled, 5*time.Second)
	if info.ReasonCode != ReasonCancelledByUser {
		t.Fatalf("victim reason = %s, want CANCELLED_BY_USER", info.ReasonCode)
	}

	// Victim must never have gone PENDING or ACTIVE on the sender.
	for _, st := range dedupeStates(rec.statesFor(victimID)) {
		if st == StateActive || st == StatePending {
			t.Fatalf("cancelled-while-queued transfer touched the wire: %v", dedupeStates(rec.statesFor(victimID)))
		}
	}

	p.senderCh.Release()
	waitForState(t, p.senderEng, headID, StateComplete, 20*time.Second)
	waitForState(t, p.senderEng, tailID, StateComplete, 20*time.Second)

	// Only head.bin and tail.bin may have landed; victim.bin must not exist.
	entries := destEntries(t, p.destDir)
	if len(entries) != 2 || !hasString(entries, "head.bin") || !hasString(entries, "tail.bin") {
		t.Fatalf("destination entries = %v, want only head.bin and tail.bin", entries)
	}
	if got := readFile(t, filepath.Join(p.destDir, "tail.bin")); sha256.Sum256(got) != [32]byte(tailDig) {
		t.Fatalf("tail.bin bytes do not match the source digest")
	}
	if bad := ordering.violations(); len(bad) != 0 {
		t.Fatalf("FileCancel was sent before FileOffer for %v (the peer would have ignored it and then accepted the offer)", bad)
	}
	t.Logf("observed victim sequence (never PENDING/ACTIVE): %v", dedupeStates(rec.statesFor(victimID)))
}

// TestBehavior_DetachTerminatesActiveAndQueued kills the channel with one
// transfer on the wire and two queued, then asserts every transfer reaches a
// terminal FAILED/INTERRUPTED state and every source descriptor is released.
func TestBehavior_DetachTerminatesActiveAndQueued(t *testing.T) {
	rec := &eventRecorder{}
	p := newPair(t, pairOptions{tune: func(c *Config) { c.OnEvent = rec.record }})

	var baseline int
	if runtime.GOOS == "linux" {
		baseline = openFDCount(t)
	}

	p.senderCh.PauseAfter(1)
	ids := make([]string, 3)
	for i := 0; i < 3; i++ {
		path, _ := writeSource(t, p.srcDir, string(rune('a'+i))+".bin", 1<<20)
		id, err := p.senderEng.SendFile(context.Background(), path, "")
		if err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
		ids[i] = id
	}
	waitForState(t, p.senderEng, ids[0], StateActive, 5*time.Second)

	p.senderEng.DetachChannel(ReasonInterrupted, "transport closed")
	for i, id := range ids {
		info := waitForState(t, p.senderEng, id, StateFailed, 5*time.Second)
		if info.ReasonCode != ReasonInterrupted {
			t.Fatalf("transfer %d reason = %s, want INTERRUPTED", i, info.ReasonCode)
		}
	}

	if runtime.GOOS == "linux" {
		waitFor(t, 3*time.Second, "source fds released", func() bool {
			return openFDCount(t) <= baseline+1
		})
		t.Logf("observed fd baseline %d released after teardown", baseline)
	}

	terminal := 0
	for _, id := range ids {
		if info, ok := p.senderEng.Get(id); ok && info.State.Terminal() {
			terminal++
		}
	}
	if terminal != len(ids) {
		t.Fatalf("terminal transfers = %d, want %d", terminal, len(ids))
	}
	t.Logf("observed all %d transfers terminal after detach", terminal)
}

func hasString(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
