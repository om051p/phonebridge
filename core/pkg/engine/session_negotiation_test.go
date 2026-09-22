package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	pion "github.com/pion/webrtc/v4"

	"github.com/om051p/phonebridge/core/pkg/crypto"
	"github.com/om051p/phonebridge/core/pkg/discovery"
	"github.com/om051p/phonebridge/core/pkg/receiver"
	"github.com/om051p/phonebridge/core/pkg/rtpmedia"
	"github.com/om051p/phonebridge/core/pkg/transfer"
)

// ---------------------------------------------------------------- fake phone

// fakePhone is a minimal implementation of the LAN signaling contract that the
// real Android device serves. It records the requests it receives so the
// negotiation request (version, capabilities, requested tuple) can be asserted
// on the wire rather than inferred.
type fakePhone struct {
	mu sync.Mutex

	offers    []offerRequest
	answers   int
	stops     []stopPayload
	stopped   chan struct{}
	stopOnce  sync.Once
	status    int
	errorBody string

	// answer builds the response for a request. Tests override it to script
	// rejections, legacy peers, missing actual tuples and so on.
	answer func(req offerRequest) offerResponse
}

func newFakePhone(answer func(offerRequest) offerResponse) *fakePhone {
	return &fakePhone{stopped: make(chan struct{}), answer: answer}
}

func (p *fakePhone) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/session/offer", func(w http.ResponseWriter, r *http.Request) {
		var req offerRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		p.mu.Lock()
		p.offers = append(p.offers, req)
		answer := p.answer
		status := p.status
		errorBody := p.errorBody
		p.mu.Unlock()

		if status != 0 {
			w.WriteHeader(status)
			if errorBody != "" {
				_, _ = w.Write([]byte(errorBody))
			}
			return
		}

		resp := offerResponse{Type: "offer", SDP: fakeOfferSDP()}
		if answer != nil {
			resp = answer(req)
			if resp.SDP == "" {
				resp.SDP = fakeOfferSDP()
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/session/answer", func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.answers++
		p.mu.Unlock()
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	mux.HandleFunc("/session/stop", func(w http.ResponseWriter, r *http.Request) {
		var payload stopPayload
		_ = json.NewDecoder(r.Body).Decode(&payload)
		p.mu.Lock()
		p.stops = append(p.stops, payload)
		p.mu.Unlock()
		p.stopOnce.Do(func() { close(p.stopped) })
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	return mux
}

func (p *fakePhone) offerCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.offers)
}

func (p *fakePhone) lastOffer() offerRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.offers) == 0 {
		return offerRequest{}
	}
	return p.offers[len(p.offers)-1]
}

// fakeOfferSDP is a syntactically valid offer. It never needs to complete ICE:
// the transport the session builds is a fake, so what is verified here is the
// negotiation and lifecycle contract, not the media path (the loopback test in
// signaling_test.go covers that with real peer connections).
func fakeOfferSDP() string {
	return "v=0\r\no=- 1 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n"
}

func accepted(req offerRequest) offerResponse {
	return offerResponse{
		Type:            "offer",
		ProtocolVersion: signalingVersion,
		Accepted:        boolPtr(true),
		Actual: &mediaParamsJSON{
			Width:       uint32(req.Requested.Width),
			Height:      uint32(req.Requested.Height),
			FPS:         uint32(req.Requested.FPS),
			BitrateKbps: uint32(req.Requested.BitrateKbps),
		},
	}
}

func boolPtr(v bool) *bool { return &v }

// ----------------------------------------------------------- fake transports

// fakeTransport stands in for one receiver/peer-connection instance. It reports
// into a shared probe so tests can assert the order of create/close and that no
// transport is ever left open.
type fakeTransport struct {
	id  int
	cfg receiver.Config

	probe *transportProbe

	mu         sync.Mutex
	closed     bool
	closeCount int
	stats      rtpmedia.StreamStats

	// transferCh is set by tests that exercise the file-transfer wiring; nil
	// otherwise, which is how a peer without the "transfer" channel looks.
	transferCh transfer.Channel

	// scripted behaviour for this attempt
	setOfferErr error
	connectOn   bool
	trackDelay  time.Duration
	trackOK     bool
}

func (f *fakeTransport) SetRemoteOffer(offer pion.SessionDescription) (pion.SessionDescription, error) {
	if f.setOfferErr != nil {
		return pion.SessionDescription{}, f.setOfferErr
	}
	if f.connectOn && f.cfg.OnStateChange != nil {
		f.cfg.OnStateChange(pion.PeerConnectionStateConnected)
	}
	return pion.SessionDescription{Type: pion.SDPTypeAnswer, SDP: "v=0\r\nfake-answer\r\n"}, nil
}

func (f *fakeTransport) WaitForTrack(timeout time.Duration) error {
	// A phone with its screen off simply never produces a track: blocking is the
	// correct behaviour and must be distinguishable from a link failure.
	if f.trackDelay > 0 {
		time.Sleep(f.trackDelay)
	}
	if !f.trackOK {
		return errors.New("fake transport: no track")
	}
	return nil
}

// TransferChannel returns the fake transfer channel this transport exposes, if a
// test installed one. Returning the engine's port (not a Pion type) is exactly
// what the real transport does, so the wiring under test is the production one.
func (f *fakeTransport) TransferChannel() transfer.Channel {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.transferCh
}

func (f *fakeTransport) Stats() (rtpmedia.StreamStats, int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stats, 0
}

func (f *fakeTransport) Close() error {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil
	}
	f.closed = true
	f.closeCount++
	f.mu.Unlock()

	f.probe.record("close#%d", f.id)
	// A real receiver reports CLOSED when it is closed. The session must ignore
	// that callback for a transport it has already replaced, otherwise every
	// reconnect would immediately trigger another one.
	if f.cfg.OnStateChange != nil {
		f.cfg.OnStateChange(pion.PeerConnectionStateClosed)
	}
	return nil
}

func (f *fakeTransport) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func (f *fakeTransport) fireState(st pion.PeerConnectionState) {
	if f.cfg.OnStateChange != nil {
		f.cfg.OnStateChange(st)
	}
}

func (f *fakeTransport) fireDeviceError(code, message string) {
	if f.cfg.OnSessionError != nil {
		f.cfg.OnSessionError(code, message)
	}
}

// transportProbe is the test transport factory: it records every transport it
// builds and the order in which create/close events happen.
type transportProbe struct {
	mu     sync.Mutex
	events []string
	built  []*fakeTransport
	script func(id int, t *fakeTransport)
}

func newTransportProbe() *transportProbe { return &transportProbe{} }

func (p *transportProbe) factory(cfg receiver.Config) (transport, error) {
	p.mu.Lock()
	id := len(p.built) + 1
	t := &fakeTransport{id: id, cfg: cfg, probe: p, connectOn: true, trackOK: true}
	p.built = append(p.built, t)
	script := p.script
	p.mu.Unlock()

	if script != nil {
		script(id, t)
	}
	p.record("create#%d", id)
	return t, nil
}

func (p *transportProbe) record(format string, args ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, fmt.Sprintf(format, args...))
}

func (p *transportProbe) eventLog() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.events))
	copy(out, p.events)
	return out
}

func (p *transportProbe) transports() []*fakeTransport {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]*fakeTransport, len(p.built))
	copy(out, p.built)
	return out
}

func (p *transportProbe) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.built)
}

func (p *transportProbe) setScript(f func(id int, t *fakeTransport)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.script = f
}

func (p *transportProbe) open() int {
	n := 0
	for _, t := range p.transports() {
		if !t.isClosed() {
			n++
		}
	}
	return n
}

// --------------------------------------------------------------- test harness

type stateTrace struct {
	mu          sync.Mutex
	transitions []SessionState
	reconnects  int
}

func newStateTrace() *stateTrace { return &stateTrace{} }

func (tr *stateTrace) callback() StateChangeCallback {
	return func(oldState, newState SessionState, reason string, code SessionReason) {
		tr.mu.Lock()
		defer tr.mu.Unlock()
		tr.transitions = append(tr.transitions, newState)
		if newState == StateReconnecting {
			tr.reconnects++
		}
	}
}

func (tr *stateTrace) reconnectCount() int {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.reconnects
}

func (tr *stateTrace) all() []SessionState {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	out := make([]SessionState, len(tr.transitions))
	copy(out, tr.transitions)
	return out
}

// countingSink proves the session owns the display path: it must survive a
// transport reconnect and be closed exactly once, on final teardown.
type countingSink struct {
	mu     sync.Mutex
	writes int
	closed int
}

func (s *countingSink) WriteAU(au rtpmedia.AccessUnit) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed > 0 {
		return errors.New("sink closed")
	}
	s.writes++
	return nil
}

func (s *countingSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed++
	return nil
}

func (s *countingSink) closeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *countingSink) writeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writes
}

type negotiationHarness struct {
	phone *fakePhone
	srv   *httptest.Server
	probe *transportProbe
	trace *stateTrace
	sess  *Session
	sink  *countingSink
}

// newNegotiationHarness wires a session to a fake phone over real HTTP and a
// fake transport factory, so the LAN contract and the lifecycle rules can both
// be exercised deterministically.
func newNegotiationHarness(t *testing.T, answer func(offerRequest) offerResponse, cfg func(*SessionConfig)) *negotiationHarness {
	t.Helper()

	phone := newFakePhone(answer)
	srv := httptest.NewServer(phone.handler())
	t.Cleanup(srv.Close)

	probe := newTransportProbe()
	trace := newStateTrace()

	reg := discovery.NewDeviceRegistry(discovery.DefaultRegistryConfig(), nil)
	reg.Upsert(discovery.Device{ID: "test-phone", Name: "Test Phone"})

	c := DefaultSessionConfig()
	c.TargetDeviceID = "test-phone"
	c.ConnectTimeout = 2 * time.Second
	c.ReconnectTimeout = 2 * time.Second
	// Fast, deterministic backoff for tests.
	c.ReconnectBackoff = []time.Duration{10 * time.Millisecond}
	if cfg != nil {
		cfg(&c)
	}

	sess := NewSession("sess-negotiation", c, reg, trace.callback())
	sess.SetTransportFactory(probe.factory)

	return &negotiationHarness{
		phone: phone,
		srv:   srv,
		probe: probe,
		trace: trace,
		sess:  sess,
		sink:  &countingSink{},
	}
}

func (h *negotiationHarness) endpoint() string {
	return strings.TrimPrefix(h.srv.URL, "http://")
}

func (h *negotiationHarness) connect(t *testing.T) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return h.sess.Connect(ctx, h.endpoint(), h.sink)
}

func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// ------------------------------------------------------------- negotiation

func TestSession_RequestsParametersBeforeOffer(t *testing.T) {
	h := newNegotiationHarness(t, accepted, func(c *SessionConfig) {
		c.PreferredWidth = 1080
		c.PreferredHeight = 2400
		c.PreferredFPS = 60
		c.PreferredBitrateKbps = 8000
	})

	if err := h.connect(t); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = h.sess.Stop("done") }()

	got := h.phone.lastOffer()
	if got.ProtocolVersion != signalingVersion {
		t.Errorf("handshake version: got %d, want %d", got.ProtocolVersion, signalingVersion)
	}
	if got.Version.Min != signalingVersion || got.Version.Max != signalingVersion {
		t.Errorf("version range not advertised: %+v", got.Version)
	}
	if len(got.Capabilities) == 0 {
		t.Error("expected the initiator to advertise its capabilities")
	}
	want := MediaParams{Width: 1080, Height: 2400, FPS: 60, BitrateKbps: 8000}
	if req := got.Requested.toMediaParams(); !req.Equal(want) {
		t.Errorf("requested tuple: got %s, want %s", req, want)
	}
	// Parameters must be settled before the offer exists (DEC-020: Android needs
	// a consent before capture exists).
	if h.phone.offerCount() != 1 {
		t.Errorf("expected exactly one offer request, got %d", h.phone.offerCount())
	}
}

func TestSession_RecordsNegotiatedActualParameters(t *testing.T) {
	h := newNegotiationHarness(t, func(req offerRequest) offerResponse {
		// The device downgrades: it cannot do 1080p60, so it reports 720x1600@30.
		return offerResponse{
			Type:            "offer",
			ProtocolVersion: signalingVersion,
			Accepted:        boolPtr(true),
			Actual:          &mediaParamsJSON{Width: 720, Height: 1600, FPS: 30, BitrateKbps: 4000},
		}
	}, func(c *SessionConfig) {
		c.PreferredWidth = 1080
		c.PreferredHeight = 2400
		c.PreferredFPS = 60
		c.PreferredBitrateKbps = 8000
	})

	if err := h.connect(t); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = h.sess.Stop("done") }()

	snap := h.sess.Snapshot()
	if !snap.ActualKnown {
		t.Fatal("expected the device-reported tuple to be known")
	}
	if snap.Actual.FPS != 30 || snap.Actual.Width != 720 {
		t.Errorf("actual tuple not recorded: %s", snap.Actual)
	}
	if snap.Requested.FPS != 60 {
		t.Errorf("requested tuple lost: %s", snap.Requested)
	}
	// No silent substitution: requested must not have been rewritten to match.
	if snap.Requested.Equal(snap.Actual) {
		t.Error("requested and actual must stay distinct when the device downgrades")
	}
}

func TestSession_UnknownActualIsNotBackfilled(t *testing.T) {
	h := newNegotiationHarness(t, func(req offerRequest) offerResponse {
		// A peer that reports no actual tuple at all.
		return offerResponse{Type: "offer", ProtocolVersion: signalingVersion}
	}, nil)

	if err := h.connect(t); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = h.sess.Stop("done") }()

	if _, known := h.sess.NegotiatedParams(); known {
		t.Fatal("an unreported actual tuple must stay unknown, not be assumed equal to the request")
	}
}

func TestSession_RejectedParametersFailTyped(t *testing.T) {
	h := newNegotiationHarness(t, func(req offerRequest) offerResponse {
		return offerResponse{
			Type:            "offer",
			ProtocolVersion: signalingVersion,
			Code:            string(CodeUnsupportedMediaParams),
			Message:         "device cannot capture 1080p60",
		}
	}, func(c *SessionConfig) {
		c.PreferredFPS = 60
	})

	err := h.connect(t)
	if err == nil {
		t.Fatal("expected a rejection to fail the connection")
	}
	if !strings.Contains(err.Error(), string(CodeUnsupportedMediaParams)) {
		t.Errorf("error must carry the typed code, got %v", err)
	}
	if got := h.sess.State(); got != StateFailed {
		t.Errorf("state: got %s, want FAILED", got)
	}
	if got := h.sess.ReasonCode(); got != ReasonUnsupportedMediaParams {
		t.Errorf("reason code: got %v, want UNSUPPORTED_MEDIA_PARAMS", got)
	}
	// A rejected request is not recoverable: the geometry cannot change without a
	// new user consent (DEC-020).
	if n := h.trace.reconnectCount(); n != 0 {
		t.Errorf("rejection must not trigger reconnect, saw %d", n)
	}
	if err := h.sess.Stop("cleanup"); err != nil {
		t.Errorf("stop after failure: %v", err)
	}
	if n := h.probe.open(); n != 0 {
		t.Errorf("expected all transports closed, %d still open", n)
	}
}

func TestSession_VersionMismatchIsHardFailure(t *testing.T) {
	h := newNegotiationHarness(t, func(req offerRequest) offerResponse {
		return offerResponse{
			Type:            "offer",
			ProtocolVersion: signalingVersion + 7,
			Accepted:        boolPtr(true),
		}
	}, nil)

	err := h.connect(t)
	if err == nil {
		t.Fatal("expected a version mismatch to fail the connection")
	}
	if !strings.Contains(err.Error(), string(CodeIncompatibleVersion)) {
		t.Errorf("expected INCOMPATIBLE_VERSION, got %v", err)
	}
	if got := h.sess.ReasonCode(); got != ReasonProtocolVersionMismatch {
		t.Errorf("reason code: got %v", got)
	}
	_ = h.sess.Stop("cleanup")
}

func TestSession_CapabilityWithoutScreenSupportRejected(t *testing.T) {
	h := newNegotiationHarness(t, func(req offerRequest) offerResponse {
		return offerResponse{
			Type:            "offer",
			ProtocolVersion: signalingVersion,
			Accepted:        boolPtr(true),
			Capabilities: []mediaCapabilityJSON{
				{Codecs: []string{"h264"}, SupportsScreen: false},
			},
		}
	}, nil)

	err := h.connect(t)
	if err == nil || !strings.Contains(err.Error(), "no screen-capture capability") {
		t.Fatalf("expected a capability rejection, got %v", err)
	}
	_ = h.sess.Stop("cleanup")
}

func TestSession_SessionBusyIsTerminal(t *testing.T) {
	h := newNegotiationHarness(t, accepted, nil)
	h.phone.mu.Lock()
	h.phone.status = http.StatusConflict
	h.phone.errorBody = `{"code":"SESSION_BUSY","message":"already streaming"}`
	h.phone.mu.Unlock()

	err := h.connect(t)
	if err == nil {
		t.Fatal("expected SESSION_BUSY to fail the connection")
	}
	if got := h.sess.ReasonCode(); got != ReasonSessionBusy {
		t.Errorf("reason code: got %v, want SESSION_BUSY", got)
	}
	if n := h.trace.reconnectCount(); n != 0 {
		t.Errorf("SESSION_BUSY must not trigger reconnect, saw %d", n)
	}
	_ = h.sess.Stop("cleanup")
}

// --------------------------------------------------------- reconnect lifecycle

func TestSession_ReconnectReplacesTransportWithoutTouchingSink(t *testing.T) {
	h := newNegotiationHarness(t, accepted, nil)

	if err := h.connect(t); err != nil {
		t.Fatalf("connect: %v", err)
	}
	waitFor(t, time.Second, "STREAMING", func() bool { return h.sess.State() == StateStreaming })
	first := h.probe.transports()[0]

	// The link drops. This is a transport failure and must recover.
	first.fireState(pion.PeerConnectionStateDisconnected)

	waitFor(t, 3*time.Second, "recovered STREAMING", func() bool {
		return h.sess.State() == StateStreaming && h.probe.count() == 2
	})

	second := h.probe.transports()[1]
	if !first.isClosed() {
		t.Error("the replaced transport must be closed")
	}
	if second.isClosed() {
		t.Error("the replacement transport must be live")
	}

	// Ordering: the old transport is closed before the new one exists, so two
	// peer connections never overlap.
	log := h.probe.eventLog()
	closeIdx, create2Idx := -1, -1
	for i, e := range log {
		if e == "close#1" && closeIdx == -1 {
			closeIdx = i
		}
		if e == "create#2" && create2Idx == -1 {
			create2Idx = i
		}
	}
	if log[0] != "create#1" || closeIdx == -1 || create2Idx == -1 || closeIdx > create2Idx {
		t.Errorf("expected create#1, close#1, create#2 ordering, got %v", log)
	}

	// The display path is untouched: the sink outlives the transport, because a
	// reconnect repairs the transport, not the capture pipeline (DEC-022).
	if n := h.sink.closeCount(); n != 0 {
		t.Errorf("sink must survive a reconnect, close count = %d", n)
	}

	// Session identity is stable across the reconnect.
	if id := h.sess.SessionID(); id != "sess-negotiation" {
		t.Errorf("session ID changed across reconnect: %s", id)
	}

	// Exactly one reconnect window ran: closing the replaced transport fires its
	// own CLOSED callback, which must not be mistaken for a fresh failure.
	if n := h.trace.reconnectCount(); n != 1 {
		t.Errorf("expected exactly one RECONNECTING transition, got %d (%v)", n, h.trace.all())
	}
	if n := h.sess.ReconnectAttempts(); n != 1 {
		t.Errorf("expected one reconnect attempt, got %d", n)
	}

	// The reconnect re-sends the same request: the device must be able to see
	// that the parameters did not change, and must not restart capture.
	if n := h.phone.offerCount(); n != 2 {
		t.Errorf("expected the reconnect to re-request an offer, got %d request(s)", n)
	}

	if err := h.sess.Stop("done"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if n := h.probe.open(); n != 0 {
		t.Errorf("expected all transports closed after stop, %d open", n)
	}
	if n := h.sink.closeCount(); n != 1 {
		t.Errorf("sink must be closed exactly once on teardown, got %d", n)
	}
}

func TestSession_ReconnectBudgetExhaustedFailsTyped(t *testing.T) {
	h := newNegotiationHarness(t, accepted, func(c *SessionConfig) {
		c.ReconnectTimeout = 250 * time.Millisecond
		c.ReconnectBackoff = []time.Duration{10 * time.Millisecond}
	})

	if err := h.connect(t); err != nil {
		t.Fatalf("connect: %v", err)
	}
	waitFor(t, time.Second, "STREAMING", func() bool { return h.sess.State() == StateStreaming })

	// Every replacement transport fails before it can be used.
	h.probe.setScript(func(id int, tr *fakeTransport) {
		if id > 1 {
			tr.setOfferErr = errors.New("connection refused")
		}
	})

	h.probe.transports()[0].fireState(pion.PeerConnectionStateFailed)

	waitFor(t, 4*time.Second, "reconnect timeout", func() bool { return h.sess.State() == StateFailed })
	if got := h.sess.ReasonCode(); got != ReasonReconnectTimeout {
		t.Errorf("reason code: got %v, want RECONNECT_TIMEOUT", got)
	}
	if n := h.sess.ReconnectAttempts(); n < 2 {
		t.Errorf("expected the bounded window to retry more than once, got %d attempt(s)", n)
	}
	if n := h.probe.open(); n != 0 {
		t.Errorf("exhausted reconnect must leave no transport open, %d open", n)
	}
	if n := h.sink.closeCount(); n != 0 {
		t.Errorf("the sink belongs to the session and must only close on teardown, got %d", n)
	}
	_ = h.sess.Stop("cleanup")
}

func TestSession_StopRacingReconnectDoesNotLeak(t *testing.T) {
	h := newNegotiationHarness(t, accepted, func(c *SessionConfig) {
		c.ReconnectBackoff = []time.Duration{5 * time.Millisecond}
		c.ReconnectTimeout = 5 * time.Second
	})

	if err := h.connect(t); err != nil {
		t.Fatalf("connect: %v", err)
	}
	waitFor(t, time.Second, "STREAMING", func() bool { return h.sess.State() == StateStreaming })

	// Drive a reconnect window and stop the session while it is still running.
	h.probe.transports()[0].fireState(pion.PeerConnectionStateDisconnected)
	time.Sleep(20 * time.Millisecond)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = h.sess.Stop("user stopped")
		}()
	}
	wg.Wait()

	if got := h.sess.State(); got != StateStopped {
		t.Fatalf("state: got %s, want STOPPED", got)
	}
	// Give any stray reconnect goroutine a chance to leak before asserting.
	time.Sleep(150 * time.Millisecond)

	if n := h.probe.open(); n != 0 {
		t.Errorf("stop racing reconnect leaked %d transport(s)", n)
	}
	if n := h.sink.closeCount(); n != 1 {
		t.Errorf("sink must be closed exactly once, got %d", n)
	}
	if got := h.sess.State(); got != StateStopped {
		t.Errorf("a racing Stop must not resurrect the session, got %s", got)
	}
}

func TestSession_RepeatedStopIsIdempotent(t *testing.T) {
	h := newNegotiationHarness(t, accepted, nil)
	if err := h.connect(t); err != nil {
		t.Fatalf("connect: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := h.sess.Stop("stop"); err != nil {
			t.Fatalf("stop %d: %v", i, err)
		}
	}
	if n := h.sink.closeCount(); n != 1 {
		t.Errorf("sink closed %d times, want 1", n)
	}
	if n := h.probe.open(); n != 0 {
		t.Errorf("%d transports still open", n)
	}
}

// ------------------------------------------------- terminal device failures

func TestSession_ConsentRevokedIsTerminalAndNotReconnectable(t *testing.T) {
	h := newNegotiationHarness(t, accepted, nil)
	if err := h.connect(t); err != nil {
		t.Fatalf("connect: %v", err)
	}
	waitFor(t, time.Second, "STREAMING", func() bool { return h.sess.State() == StateStreaming })

	h.probe.transports()[0].fireDeviceError(string(CodeConsentRevoked), "MediaProjection consent withdrawn")

	waitFor(t, time.Second, "FAILED", func() bool { return h.sess.State() == StateFailed })
	if got := h.sess.ReasonCode(); got != ReasonConsentRevoked {
		t.Errorf("reason code: got %v, want CONSENT_REVOKED", got)
	}
	if n := h.probe.open(); n != 0 {
		t.Errorf("a revoked session must release its transport, %d open", n)
	}

	// The link was healthy; the screen simply stopped being shareable. Retrying
	// cannot restore a consent, so no reconnect may be attempted.
	time.Sleep(150 * time.Millisecond)
	if n := h.trace.reconnectCount(); n != 0 {
		t.Errorf("consent revocation must not trigger reconnect, saw %d", n)
	}
	_ = h.sess.Stop("cleanup")
}

func TestSession_CaptureFailureIsNotMisclassifiedAsTransportFailure(t *testing.T) {
	h := newNegotiationHarness(t, accepted, nil)
	if err := h.connect(t); err != nil {
		t.Fatalf("connect: %v", err)
	}
	waitFor(t, time.Second, "STREAMING", func() bool { return h.sess.State() == StateStreaming })

	h.probe.transports()[0].fireDeviceError(string(CodeCaptureFailed), "encoder reconfigure failed")

	waitFor(t, time.Second, "FAILED", func() bool { return h.sess.State() == StateFailed })
	if got := h.sess.ReasonCode(); got != ReasonCaptureFailed {
		t.Errorf("reason code: got %v, want CAPTURE_FAILED (a capture failure must not be reported as a transport failure)", got)
	}
	time.Sleep(150 * time.Millisecond)
	if n := h.trace.reconnectCount(); n != 0 {
		t.Errorf("capture failure must not trigger reconnect, saw %d", n)
	}
	_ = h.sess.Stop("cleanup")
}

func TestSession_DeviceTransportFailureDoesTriggerReconnect(t *testing.T) {
	// The mirror of the test above: a device-reported transport failure is
	// retryable, so it must recover rather than fail the session.
	h := newNegotiationHarness(t, accepted, nil)
	if err := h.connect(t); err != nil {
		t.Fatalf("connect: %v", err)
	}
	waitFor(t, time.Second, "STREAMING", func() bool { return h.sess.State() == StateStreaming })

	h.probe.transports()[0].fireDeviceError(string(CodeTransportFailed), "sender ICE restarted")

	waitFor(t, 3*time.Second, "recovered", func() bool {
		return h.sess.State() == StateStreaming && h.probe.count() == 2
	})
	_ = h.sess.Stop("cleanup")
}

func TestSession_ZeroFPSDoesNotTriggerReconnect(t *testing.T) {
	// The link is up, but the phone's screen is off: no track ever arrives and no
	// frames are produced. That is a legitimate 0 fps, not a broken link.
	h := newNegotiationHarness(t, accepted, func(c *SessionConfig) {
		c.ReconnectBackoff = []time.Duration{10 * time.Millisecond}
		c.ReconnectTimeout = 200 * time.Millisecond
	})
	h.probe.setScript(func(id int, tr *fakeTransport) {
		tr.trackOK = false
		tr.trackDelay = 2 * time.Second
	})

	if err := h.connect(t); err != nil {
		t.Fatalf("connect: %v", err)
	}

	waitFor(t, time.Second, "CONNECTED", func() bool { return h.sess.State() == StateConnected })

	// Well past the reconnect budget: nothing may have been attempted, because
	// recovery is judged by transport state rather than frame arrival.
	time.Sleep(400 * time.Millisecond)

	if got := h.sess.State(); got != StateConnected {
		t.Errorf("state: got %s, want CONNECTED (0 fps is not a link failure)", got)
	}
	if n := h.trace.reconnectCount(); n != 0 {
		t.Errorf("zero-fps must not trigger reconnect, saw %d", n)
	}
	if n := h.probe.count(); n != 1 {
		t.Errorf("no replacement transport may be built, saw %d", n)
	}
	_ = h.sess.Stop("cleanup")
}

func TestSession_StopNotifiesDeviceWithTypedReason(t *testing.T) {
	h := newNegotiationHarness(t, accepted, nil)
	if err := h.connect(t); err != nil {
		t.Fatalf("connect: %v", err)
	}

	if err := h.sess.Stop("user stopped sharing"); err != nil {
		t.Fatalf("stop: %v", err)
	}

	select {
	case <-h.phone.stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("device was not told the session stopped")
	}

	h.phone.mu.Lock()
	defer h.phone.mu.Unlock()
	if len(h.phone.stops) != 1 {
		t.Fatalf("expected one stop notification, got %d", len(h.phone.stops))
	}
	if h.phone.stops[0].Reason != "user stopped sharing" {
		t.Errorf("stop reason not forwarded: %q", h.phone.stops[0].Reason)
	}
	if h.phone.stops[0].ReasonCode != string(CodeOK) {
		t.Errorf("stop reason code: got %q, want OK", h.phone.stops[0].ReasonCode)
	}
}

// ------------------------------------------------------------ trust re-check

func TestSession_RevokedTrustStopsReconnect(t *testing.T) {
	tempDir := t.TempDir()
	ts, err := crypto.NewTrustStore(filepath.Join(tempDir, "trusted.json"))
	if err != nil {
		t.Fatalf("trust store: %v", err)
	}
	const deviceID = "test-phone"
	if err := ts.AddTrusted(crypto.TrustEntry{
		DeviceID:    deviceID,
		DisplayName: "Test Phone",
		Platform:    "android",
		PublicKey:   make([]byte, 32),
		PairedAt:    time.Now(),
	}); err != nil {
		t.Fatalf("add trusted: %v", err)
	}

	h := newNegotiationHarness(t, accepted, nil)
	h.sess.SetTrustStore(ts)

	if err := h.connect(t); err != nil {
		t.Fatalf("connect: %v", err)
	}
	waitFor(t, time.Second, "STREAMING", func() bool { return h.sess.State() == StateStreaming })

	// Revoking trust mid-session must not be silently reconnected around.
	if err := ts.Revoke(deviceID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	h.probe.transports()[0].fireState(pion.PeerConnectionStateDisconnected)

	waitFor(t, 3*time.Second, "terminal failure", func() bool { return h.sess.State() == StateFailed })
	if got := h.sess.ReasonCode(); got != ReasonDeviceNotTrusted {
		t.Errorf("reason code: got %v, want DEVICE_NOT_TRUSTED", got)
	}
	if n := h.probe.open(); n != 0 {
		t.Errorf("a revoked device must release its transport, %d open", n)
	}
	_ = h.sess.Stop("cleanup")
}
