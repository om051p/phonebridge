package transfer

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

// wireMonitor proves the invariant this property test exists for: at most one
// outbound transfer may have an offer open on the wire at any instant. It
// combines the frames actually handed to the sender's channel (an offer opens a
// window; FileComplete/FileCancel close it) with the engine's terminal events
// (which close a transfer that was interrupted without a frame), all under one
// mutex so the interleaving it sees is the one the engine actually produced.
type wireMonitor struct {
	mu        sync.Mutex
	open      map[string]bool
	offers    int
	violation string
}

func newWireMonitor() *wireMonitor {
	return &wireMonitor{open: map[string]bool{}}
}

func (m *wireMonitor) frame(raw []byte) {
	f, err := DecodeFrame(raw)
	if err != nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	switch body := f.Body.(type) {
	case *phonebridgev1.TransferFrame_Offer:
		if body.Offer == nil {
			return
		}
		m.open[body.Offer.TransferId] = true
		m.offers++
		if n := len(m.open); n > 1 && m.violation == "" {
			m.violation = fmt.Sprintf("%d outbound offers open at once: %v", n, m.openIDsLocked())
		}
	case *phonebridgev1.TransferFrame_Complete:
		if body.Complete != nil {
			delete(m.open, body.Complete.TransferId)
		}
	case *phonebridgev1.TransferFrame_Cancel:
		if body.Cancel != nil {
			delete(m.open, body.Cancel.TransferId)
		}
	}
}

// terminal closes the window for an outbound transfer that ended without sending
// FileComplete/FileCancel (an interruption), so its stale offer cannot be
// mistaken for a live one later.
func (m *wireMonitor) terminal(ev Event) {
	if ev.Info.Direction != DirectionOutbound || !ev.Info.State.Terminal() {
		return
	}
	m.mu.Lock()
	delete(m.open, ev.Info.TransferID)
	m.mu.Unlock()
}

func (m *wireMonitor) openIDsLocked() []string {
	ids := make([]string, 0, len(m.open))
	for id := range m.open {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (m *wireMonitor) result() (int, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.offers, m.violation
}

func (m *wireMonitor) offerCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.offers
}

// TestQueueInvariant_AtMostOneOutboundOfferOnTheWire drives the real engine with
// randomized, concurrent SendFile / Cancel / DetachChannel interleavings and
// asserts the transport never carries two outbound offers at once. A violation
// is also visible as a transfer refused with BUSY ("a transfer is already in
// flight"), so that is asserted too.
func TestQueueInvariant_AtMostOneOutboundOfferOnTheWire(t *testing.T) {
	const iterations = 12
	for iter := 0; iter < iterations; iter++ {
		t.Run(fmt.Sprintf("seed-%d", iter), func(t *testing.T) {
			runQueueInvariantIteration(t, int64(iter)+1)
		})
	}
}

func runQueueInvariantIteration(t *testing.T, seed int64) {
	t.Helper()

	monitor := newWireMonitor()
	p := newPair(t, pairOptions{tune: func(c *Config) {
		c.OutboundQueueDepth = 32
		c.OnEvent = monitor.terminal
	}})

	// Every frame handed to the sender's channel passes through the monitor. The
	// small fixed delay models a real link: it widens the on-wire window enough
	// that concurrent sends actually queue behind the active one.
	p.senderCh.Observe(func(frame []byte) {
		monitor.frame(frame)
		time.Sleep(50 * time.Microsecond)
	})

	const files = 4
	paths := make([]string, files)
	for i := range paths {
		path, _ := writeSource(t, p.srcDir, fmt.Sprintf("f%d.bin", i), 48*1024)
		paths[i] = path
	}

	var mu sync.Mutex
	var ids []string

	const workers = 3
	const opsPerWorker = 50
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		rng := rand.New(rand.NewSource(seed*1000 + int64(w)))
		wg.Add(1)
		go func(rng *rand.Rand) {
			defer wg.Done()
			for i := 0; i < opsPerWorker; i++ {
				switch rng.Intn(10) {
				case 0, 1, 2, 3, 4, 5, 6: // offer a file
					id, err := p.senderEng.SendFile(context.Background(), paths[rng.Intn(files)], "")
					if err != nil {
						f, ok := IsFailure(err)
						if !ok {
							t.Errorf("SendFile returned a non-typed error: %v", err)
						} else if f.Reason != ReasonBusy && f.Reason != ReasonNoSession {
							t.Errorf("SendFile failed unexpectedly: %s: %s", f.Reason, f.Message)
						}
						break
					}
					mu.Lock()
					ids = append(ids, id)
					mu.Unlock()
				default: // cancel a previously offered transfer
					mu.Lock()
					var id string
					if n := len(ids); n > 0 {
						id = ids[rng.Intn(n)]
					}
					mu.Unlock()
					if id != "" {
						_ = p.senderEng.Cancel(context.Background(), id)
					}
				}
				time.Sleep(time.Duration(rng.Intn(300)) * time.Microsecond)
			}
		}(rng)
	}

	// A session drop lands somewhere during the burst (no reattach, so no offer
	// can ever reach a second channel generation). Wait for the burst to reach
	// the wire first, so the drop always interrupts a live session.
	detached := make(chan struct{})
	go func() {
		defer close(detached)
		rng := rand.New(rand.NewSource(seed))
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) && monitor.offerCount() == 0 {
			time.Sleep(200 * time.Microsecond)
		}
		time.Sleep(time.Duration(rng.Intn(3000)) * time.Microsecond)
		p.senderEng.DetachChannel(ReasonInterrupted, "property test: session drop")
	}()
	wg.Wait()
	<-detached

	// Quiesce: everything still in flight is terminated on both ends.
	p.senderEng.DetachChannel(ReasonInterrupted, "property test: teardown")
	p.receiverEng.DetachChannel(ReasonInterrupted, "property test: teardown")

	offers, violation := monitor.result()
	if offers == 0 {
		t.Fatalf("the monitor observed no offers; the channel hook is not wired")
	}
	if violation != "" {
		t.Fatalf("wire invariant violated (seed %d): %s", seed, violation)
	}

	// Two accepted outbound offers hitting the receiver at once would surface as
	// a refusal, never as silent success.
	for _, eng := range []*Engine{p.senderEng, p.receiverEng} {
		for _, info := range eng.List() {
			if info.State.Terminal() && info.ReasonCode == ReasonBusy {
				t.Fatalf("a transfer was refused with BUSY (two offers on the wire): %+v", info)
			}
		}
	}

	// Every accepted send reaches a terminal state once the session is gone.
	mu.Lock()
	all := append([]string(nil), ids...)
	mu.Unlock()
	for _, id := range all {
		info, ok := p.senderEng.Get(id)
		if !ok || !info.State.Terminal() {
			t.Fatalf("transfer %s did not reach a terminal state: %+v ok=%v", id, info, ok)
		}
	}

	t.Logf("seed %d: %d offers observed, %d transfers, 0 invariant violations", seed, offers, len(all))
}
