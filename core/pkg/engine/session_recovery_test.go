package engine

// Media-recovery regression tests (reliability hardening phase).
//
// A transport-level recovery used to end the reconnect loop without rebuilding
// the media pipeline: ICE self-healing surfaced CONNECTED mid-recovery and the
// loop treated it as recovered, so the session reported CONNECTED forever while
// the peer decoder could never resync (measured on device after a Wi-Fi
// interruption: CONNECTED with delivered=0 fps). The contract pinned here:
//
//   - a self-healed CONNECTED during recovery is folded back into
//     RECONNECTING and the loop performs the full transport replacement,
//   - CONNECTED after a reconnect means transport-up only; STREAMING appears
//     once the replacement's media track actually delivers,
//   - exhausting the reconnect budget fails the session cleanly, with no
//     resurrection from late transport callbacks.

import (
	"errors"
	"testing"
	"time"

	pion "github.com/pion/webrtc/v4"
)

func TestSession_SelfHealDoesNotSkipMediaRebuild(t *testing.T) {
	h := newNegotiationHarness(t, accepted, nil)

	if err := h.connect(t); err != nil {
		t.Fatalf("connect: %v", err)
	}
	waitFor(t, time.Second, "STREAMING", func() bool { return h.sess.State() == StateStreaming })
	first := h.probe.transports()[0]

	// The link drops, then ICE self-heals before the reconnect loop's first
	// attempt runs. Transport-up must not end recovery: the media pipeline is
	// still the broken one.
	first.fireState(pion.PeerConnectionStateDisconnected)
	first.fireState(pion.PeerConnectionStateConnected)

	waitFor(t, 3*time.Second, "media rebuilt on a replacement transport", func() bool {
		return h.sess.State() == StateStreaming && h.probe.count() >= 2
	})

	if !first.isClosed() {
		t.Error("the self-healed transport must be replaced (and closed)")
	}
	second := h.probe.transports()[1]
	if second.isClosed() {
		t.Error("the replacement transport must be live")
	}
	if n := h.sess.ReconnectAttempts(); n < 1 {
		t.Errorf("expected at least one reconnect attempt, got %d", n)
	}
}

func TestSession_ReconnectStaysConnectedUntilMediaReturns(t *testing.T) {
	h := newNegotiationHarness(t, accepted, func(c *SessionConfig) {
		c.ReconnectTimeout = 5 * time.Second
	})

	if err := h.connect(t); err != nil {
		t.Fatalf("connect: %v", err)
	}
	waitFor(t, time.Second, "STREAMING", func() bool { return h.sess.State() == StateStreaming })

	// The replacement transport comes up but its track never arrives (a phone
	// with a static screen legitimately delivers nothing, DEC-020).
	h.probe.setScript(func(id int, ft *fakeTransport) {
		if id >= 2 {
			ft.trackOK.Store(false)
		}
	})

	h.probe.transports()[0].fireState(pion.PeerConnectionStateDisconnected)

	waitFor(t, 3*time.Second, "CONNECTED after transport recovery", func() bool {
		return h.sess.State() == StateConnected
	})
	if h.sess.State() == StateStreaming {
		t.Fatal("CONNECTED must not be upgraded to STREAMING before media arrives")
	}
	if h.probe.count() != 2 {
		t.Fatalf("expected exactly one replacement transport, got %d", h.probe.count())
	}

	// Media starts flowing on the replacement: only now is the session
	// streaming again.
	h.probe.transports()[1].trackOK.Store(true)
	waitFor(t, 3*time.Second, "STREAMING after media returns", func() bool {
		return h.sess.State() == StateStreaming
	})
}

func TestSession_ReconnectBudgetExhaustionFailsCleanly(t *testing.T) {
	h := newNegotiationHarness(t, accepted, func(c *SessionConfig) {
		c.ReconnectTimeout = 300 * time.Millisecond
	})

	if err := h.connect(t); err != nil {
		t.Fatalf("connect: %v", err)
	}
	waitFor(t, time.Second, "STREAMING", func() bool { return h.sess.State() == StateStreaming })

	// Every replacement attempt fails with a retryable transport error until
	// the budget runs out.
	h.probe.setScript(func(id int, ft *fakeTransport) {
		if id >= 2 {
			ft.setOfferErr = errors.New("transport down")
		}
	})

	h.probe.transports()[0].fireState(pion.PeerConnectionStateDisconnected)

	waitFor(t, 3*time.Second, "FAILED", func() bool { return h.sess.State() == StateFailed })
	if rc := h.sess.ReasonCode(); rc != ReasonReconnectTimeout {
		t.Errorf("reason = %v, want %v", rc, ReasonReconnectTimeout)
	}
	if n := h.sess.ReconnectAttempts(); n < 1 {
		t.Errorf("expected at least one attempt before the budget ran out, got %d", n)
	}
	for i, ft := range h.probe.transports() {
		if !ft.isClosed() {
			t.Errorf("transport #%d must be closed after the session failed", i+1)
		}
	}

	// A failed session is terminal: late transport callbacks must not resurrect
	// it into a ghost.
	h.probe.transports()[0].fireState(pion.PeerConnectionStateConnected)
	if h.sess.State() != StateFailed {
		t.Errorf("state after late callback = %v, want FAILED", h.sess.State())
	}
}
