package engine

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pion "github.com/pion/webrtc/v4"

	"github.com/om051p/phonebridge/core/pkg/receiver"
)

// newInboundManager builds a manager with no external engines: these tests are
// about the inbound session *slot* and its lifecycle, not about the planes.
func newInboundManager(t *testing.T) *SessionManager {
	t.Helper()
	cfg := DefaultSessionConfig()
	cfg.ConnectTimeout = 2 * time.Second
	mgr := NewSessionManager(cfg, nil, receiver.NewNullSink(), nil)
	t.Cleanup(func() { _ = mgr.CloseInbound() })
	return mgr
}

// newTestInboundSession builds a session directly, with lifecycle timings
// shortened so a test never has to wait out a production budget.
func newTestInboundSession(t *testing.T, cfg InboundSessionConfig) *InboundSession {
	t.Helper()
	if cfg.AnswerTimeout == 0 {
		cfg.AnswerTimeout = 250 * time.Millisecond
	}
	if cfg.DisconnectGrace == 0 {
		cfg.DisconnectGrace = 250 * time.Millisecond
	}
	cfg.IncludeLoopback = true
	sess, err := NewInboundSession(cfg)
	if err != nil {
		t.Fatalf("NewInboundSession: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return sess
}

// Deadline: a peer that dials in and never answers must not hold the slot.
func TestInboundSession_UnansweredOfferIsReclaimed(t *testing.T) {
	var deadReason atomic.Value
	var deadCount atomic.Int32
	sess := newTestInboundSession(t, InboundSessionConfig{
		AnswerTimeout: 150 * time.Millisecond,
		OnDead: func(_ *InboundSession, reason string) {
			deadCount.Add(1)
			deadReason.Store(reason)
		},
	})

	if _, err := sess.CreateOffer(); err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	waitFor(t, 3*time.Second, "the unanswered session to be reclaimed", sess.IsClosed)
	if got := deadCount.Load(); got != 1 {
		t.Fatalf("OnDead fired %d times, want exactly 1", got)
	}
	if reason, _ := deadReason.Load().(string); reason == "" {
		t.Fatal("OnDead must explain why the session died")
	}
}

// A peer that answers is alive even if ICE is still settling: the deadline
// exists to catch a vanished peer, not a slow handshake.
func TestInboundSession_AnsweredOfferSurvivesTheDeadline(t *testing.T) {
	sess := newTestInboundSession(t, InboundSessionConfig{AnswerTimeout: 100 * time.Millisecond})
	offer, err := sess.CreateOffer()
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if err := sess.SetRemoteAnswer(realAnswerFor(t, offer)); err != nil {
		t.Fatalf("SetRemoteAnswer: %v", err)
	}
	time.Sleep(400 * time.Millisecond)
	if sess.IsClosed() {
		t.Fatal("an answered session must not be reclaimed by the offer deadline")
	}
}

// A rejected answer must NOT count as a peer that answered: otherwise a bad
// handshake would silently cancel the watchdog that cleans the slot up.
func TestInboundSession_RejectedAnswerKeepsTheDeadlineArmed(t *testing.T) {
	sess := newTestInboundSession(t, InboundSessionConfig{AnswerTimeout: 150 * time.Millisecond})
	if _, err := sess.CreateOffer(); err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if err := sess.SetRemoteAnswer(pionAnswerStub()); err == nil {
		t.Fatal("an unrelated SDP answer should have been rejected")
	}
	waitFor(t, 3*time.Second, "the session to be reclaimed after a bad answer", sess.IsClosed)
}

// realAnswerFor produces a genuine SDP answer for the session's own offer, using
// a throwaway peer connection: no network is involved, only SDP negotiation, so
// the test stays deterministic.
func realAnswerFor(t *testing.T, offer pion.SessionDescription) pion.SessionDescription {
	t.Helper()
	api := pion.NewAPI()
	pc, err := api.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatalf("peer connection: %v", err)
	}
	t.Cleanup(func() { _ = pc.Close() })

	if err := pc.SetRemoteDescription(offer); err != nil {
		t.Fatalf("SetRemoteDescription: %v", err)
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("CreateAnswer: %v", err)
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		t.Fatalf("SetLocalDescription: %v", err)
	}
	local := pc.LocalDescription()
	if local == nil || local.SDP == "" {
		t.Fatal("no local answer produced")
	}
	return *local
}

// pionAnswerStub is a syntactically typed but semantically unrelated answer.
func pionAnswerStub() pion.SessionDescription {
	return pion.SessionDescription{Type: pion.SDPTypeAnswer, SDP: "v=0\r\ns=-\r\n"}
}

// Peer disappearance without /session/stop: closing the transport must release
// the session and fire OnDead exactly once, even though pion reports CLOSED back
// into our own teardown.
func TestInboundSession_TransportClosureReclaimsTheSession(t *testing.T) {
	var deadCount atomic.Int32
	sess := newTestInboundSession(t, InboundSessionConfig{
		// Long deadline: this test is about the transport, not the watchdog.
		AnswerTimeout: 10 * time.Second,
		OnDead:        func(*InboundSession, string) { deadCount.Add(1) },
	})
	if _, err := sess.CreateOffer(); err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	// The peer vanished: its transport goes away without a stop message.
	if err := sess.pc.Close(); err != nil {
		t.Fatalf("close peer connection: %v", err)
	}

	waitFor(t, 3*time.Second, "the dead transport to reclaim the session", sess.IsClosed)
	time.Sleep(50 * time.Millisecond)
	if got := deadCount.Load(); got != 1 {
		t.Fatalf("OnDead fired %d times, want exactly 1", got)
	}
}

func TestInboundSession_CloseIsIdempotent(t *testing.T) {
	sess := newTestInboundSession(t, InboundSessionConfig{AnswerTimeout: 10 * time.Second})
	if _, err := sess.CreateOffer(); err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("second Close must be a no-op, got %v", err)
	}
	if !sess.IsClosed() {
		t.Fatal("session must report closed")
	}
}

// A closed session must refuse further work rather than send on a dead channel.
func TestInboundSession_ClosedSessionRejectsOperations(t *testing.T) {
	sess := newTestInboundSession(t, InboundSessionConfig{AnswerTimeout: 10 * time.Second})
	if _, err := sess.CreateOffer(); err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := sess.CreateOffer(); err == nil {
		t.Fatal("CreateOffer must fail on a closed session")
	}
	if err := sess.SetRemoteAnswer(pionAnswerStub()); err == nil {
		t.Fatal("SetRemoteAnswer must fail on a closed session")
	}
	if err := sess.SendClipboard([]byte("x")); err == nil {
		t.Fatal("SendClipboard must fail on a closed session")
	}
	if err := sess.SendTransfer([]byte("x")); err == nil {
		t.Fatal("SendTransfer must fail on a closed session")
	}
}

// The wedge: peer A dials in, dies without stopping, and peer B must still be
// able to start a session.
func TestSessionManager_PeerDeathDoesNotWedgeFutureOffers(t *testing.T) {
	mgr := newInboundManager(t)

	first, err := mgr.HandleInboundOffer(NegotiationRequest{PeerDeviceID: "peer-a"})
	if err != nil {
		t.Fatalf("first offer: %v", err)
	}
	if first.Code != CodeOK || first.Offer == "" {
		t.Fatalf("first offer not accepted: %+v", first)
	}

	// A second offer while the first is live is refused, naming the conflict.
	busy, err := mgr.HandleInboundOffer(NegotiationRequest{PeerDeviceID: "peer-b"})
	if err != nil {
		t.Fatalf("second offer: %v", err)
	}
	if busy.Code != CodeSessionBusy {
		t.Fatalf("expected SESSION_BUSY while a session is live, got %s", busy.Code)
	}

	// Peer A disappears without sending /session/stop.
	mgr.mu.RLock()
	live := mgr.inboundSess
	mgr.mu.RUnlock()
	if live == nil {
		t.Fatal("manager lost its inbound session reference")
	}
	if err := live.pc.Close(); err != nil {
		t.Fatalf("close peer transport: %v", err)
	}

	waitFor(t, 5*time.Second, "the manager to drop the dead inbound session", func() bool {
		mgr.mu.RLock()
		defer mgr.mu.RUnlock()
		return mgr.inboundSess == nil
	})

	// Peer B now gets a session instead of a permanent SESSION_BUSY.
	after, err := mgr.HandleInboundOffer(NegotiationRequest{PeerDeviceID: "peer-b"})
	if err != nil {
		t.Fatalf("offer after peer death: %v", err)
	}
	if after.Code != CodeOK || after.Offer == "" {
		t.Fatalf("a dead peer must not wedge the slot, got %+v", after)
	}
}

// Repeated offers: every accepted offer is independent, and an explicit stop
// releases the slot for the next one.
func TestSessionManager_RepeatedInboundOffersAndStops(t *testing.T) {
	mgr := newInboundManager(t)

	for i := 0; i < 3; i++ {
		resp, err := mgr.HandleInboundOffer(NegotiationRequest{PeerDeviceID: "peer"})
		if err != nil {
			t.Fatalf("offer %d: %v", i, err)
		}
		if resp.Code != CodeOK {
			t.Fatalf("offer %d not accepted: %+v", i, resp)
		}
		if err := mgr.HandleInboundStop("user stopped", CodeOK); err != nil {
			t.Fatalf("stop %d: %v", i, err)
		}
		mgr.mu.RLock()
		leaked := mgr.inboundSess
		mgr.mu.RUnlock()
		if leaked != nil {
			t.Fatalf("stop %d left the session allocated", i)
		}
	}
}

// Concurrent offers: exactly one may win; every loser is told the device is
// busy and has no session left behind.
func TestSessionManager_ConcurrentInboundOffersElectOneWinner(t *testing.T) {
	mgr := newInboundManager(t)

	const attempts = 8
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		accepted int
		busy     int
		failed   []error
	)
	start := make(chan struct{})

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			resp, err := mgr.HandleInboundOffer(NegotiationRequest{PeerDeviceID: "peer"})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				failed = append(failed, err)
			case resp.Code == CodeOK:
				accepted++
			case resp.Code == CodeSessionBusy:
				busy++
			default:
				failed = append(failed, nil)
			}
		}()
	}
	close(start)
	wg.Wait()

	if len(failed) > 0 {
		t.Fatalf("offers failed unexpectedly: %v", failed)
	}
	if accepted != 1 {
		t.Fatalf("accepted %d offers, want exactly 1", accepted)
	}
	if busy != attempts-1 {
		t.Fatalf("busy %d offers, want %d", busy, attempts-1)
	}

	// Only the winner's session may survive: a losing session that stayed
	// allocated would hold a PeerConnection and its goroutines forever.
	mgr.mu.RLock()
	winner := mgr.inboundSess
	mgr.mu.RUnlock()
	if winner == nil {
		t.Fatal("no session was published")
	}
	if winner.IsClosed() {
		t.Fatal("the published session is already closed")
	}
}

// A live foreign session (the desktop-initiated path) must still block inbound
// offers: the busy gate is shared between both directions.
func TestSessionManager_InboundOfferBlockedByActiveOutboundSession(t *testing.T) {
	mgr := newInboundManager(t)

	cfg := mgr.cfg
	cfg.TargetDeviceID = "peer-x"
	sess := NewSession("session-live", cfg, nil, nil)
	if err := sess.Transition(StateConnecting, "test"); err != nil {
		t.Fatalf("transition: %v", err)
	}
	mgr.mu.Lock()
	mgr.activeSess = sess
	mgr.mu.Unlock()

	resp, err := mgr.HandleInboundOffer(NegotiationRequest{PeerDeviceID: "peer-y"})
	if err != nil {
		t.Fatalf("offer: %v", err)
	}
	if resp.Code != CodeSessionBusy {
		t.Fatalf("expected SESSION_BUSY against an outbound session, got %s", resp.Code)
	}

	// Once that session is terminal, inbound offers are allowed again.
	mgr.mu.Lock()
	mgr.activeSess = nil
	mgr.mu.Unlock()

	ok, err := mgr.HandleInboundOffer(NegotiationRequest{PeerDeviceID: "peer-y"})
	if err != nil {
		t.Fatalf("offer after terminal session: %v", err)
	}
	if ok.Code != CodeOK {
		t.Fatalf("a terminal session must not block inbound offers, got %s", ok.Code)
	}
}

// Offer creation waits for ICE gathering, so it must not be done while holding
// the manager lock: every other manager operation (including the Stop that
// would clear a stuck session) would queue behind it.
func TestSessionManager_InboundOfferDoesNotHoldTheManagerLock(t *testing.T) {
	mgr := newInboundManager(t)

	offerStarted := make(chan struct{})
	offerDone := make(chan time.Duration, 1)
	go func() {
		close(offerStarted)
		started := time.Now()
		if _, err := mgr.HandleInboundOffer(NegotiationRequest{PeerDeviceID: "peer"}); err != nil {
			t.Errorf("offer: %v", err)
		}
		offerDone <- time.Since(started)
	}()

	<-offerStarted
	var worst time.Duration
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		probeStart := time.Now()
		// Any manager read takes the same lock the offer path used to hold for
		// its whole duration.
		mgr.mu.RLock()
		mgr.mu.RUnlock()
		if waited := time.Since(probeStart); waited > worst {
			worst = waited
		}
		select {
		case d := <-offerDone:
			// Re-check once after the offer lands, then report.
			t.Logf("offer took %v, worst lock wait while it ran: %v", d, worst)
			if worst > 250*time.Millisecond {
				t.Fatalf("manager lock was held for %v during offer creation: "+
					"CreateOffer waits on ICE and must run outside the lock", worst)
			}
			return
		default:
		}
	}
	t.Fatal("offer never completed")
}
