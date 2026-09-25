package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/om051p/phonebridge/core/pkg/clipboard"
	"github.com/om051p/phonebridge/core/pkg/crypto"
	"github.com/om051p/phonebridge/core/pkg/discovery"
	"github.com/om051p/phonebridge/core/pkg/frames"
	"github.com/om051p/phonebridge/core/pkg/input"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"github.com/om051p/phonebridge/core/pkg/receiver"
	"github.com/om051p/phonebridge/core/pkg/rtpmedia"
	"github.com/om051p/phonebridge/core/pkg/transfer"
	pion "github.com/pion/webrtc/v4"
	"google.golang.org/protobuf/proto"
)

// defaultReconnectBackoff is the delay before each successive reconnect
// attempt. The last entry repeats for further attempts, so the effective cap is
// the last value (DEC-022: 500 ms, 1 s, 2 s, 4 s — cap 4 s, inside the 5 s
// ceiling the decision records).
var defaultReconnectBackoff = []time.Duration{
	500 * time.Millisecond,
	1 * time.Second,
	2 * time.Second,
	4 * time.Second,
}

// SessionState represents the operational phase of a device session.
type SessionState int

const (
	StateDisconnected SessionState = iota
	StateDiscovering
	StateConnecting
	StateConnected
	StateStreaming
	StateReconnecting
	StateStopped
	StateFailed
)

func (s SessionState) String() string {
	switch s {
	case StateDisconnected:
		return "DISCONNECTED"
	case StateDiscovering:
		return "DISCOVERING"
	case StateConnecting:
		return "CONNECTING"
	case StateConnected:
		return "CONNECTED"
	case StateStreaming:
		return "STREAMING"
	case StateReconnecting:
		return "RECONNECTING"
	case StateStopped:
		return "STOPPED"
	case StateFailed:
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}

// SinkKind classifies the frame sink a session writes access units to. It lets
// the local IPC snapshot report the real display path (a launched ffplay, a
// generic pipe, a recording file, or nothing) instead of the UI assuming a
// window exists — a headless daemon silently runs a NullSink.
type SinkKind int

const (
	// SinkKindUnspecified means no sink has been classified for this session,
	// or the session reached a terminal state and its classification was
	// cleared.
	SinkKindUnspecified SinkKind = iota
	SinkKindNull
	SinkKindDisplay
	SinkKindPipe
	SinkKindFile
)

func (k SinkKind) String() string {
	switch k {
	case SinkKindNull:
		return "NULL"
	case SinkKindDisplay:
		return "DISPLAY"
	case SinkKindPipe:
		return "PIPE"
	case SinkKindFile:
		return "FILE"
	default:
		return "UNSPECIFIED"
	}
}

// transport is one media transport instance: a WebRTC peer connection with its
// depacketizer and frame sink behind it. *receiver.Receiver satisfies it.
//
// The session depends on this interface rather than the concrete type so the
// transport lifecycle rules can be proven with a fake — resources closed before
// replacement, no leak when Stop races a reconnect — without a network.
type transport interface {
	SetRemoteOffer(offer pion.SessionDescription) (pion.SessionDescription, error)
	WaitForTrack(timeout time.Duration) error
	Stats() (rtpmedia.StreamStats, int64)
	// TransferChannel is the file-transfer DataChannel adapter this transport
	// owns (DEC-024). It may be nil until the peer opens the channel; the
	// session attaches it to the transfer engine when the channel opens. The
	// declared type is the engine's own Channel port, so this package never
	// depends on Pion specifics.
	TransferChannel() transfer.Channel
	Close() error
}

// transportFactory builds one transport for one connection attempt. The session
// supplies the receiver.Config (including its own callbacks) so the transport
// reports state changes and typed device failures back to exactly the generation
// that owns it.
type transportFactory func(cfg receiver.Config) (transport, error)

func defaultTransportFactory(cfg receiver.Config) (transport, error) {
	return receiver.NewReceiver(cfg)
}

// SessionConfig configures a session request.
type SessionConfig struct {
	TargetDeviceID string
	// Requested overrides the negotiated media tuple. Zero fields fall back to
	// the Preferred* values below, so a caller can request a full tuple or just
	// one field (for example only the fps).
	Requested            MediaParams
	PreferredWidth       int
	PreferredHeight      int
	PreferredFPS         int
	PreferredBitrateKbps int
	DiscoveryTimeout     time.Duration
	ConnectTimeout       time.Duration
	ReconnectTimeout     time.Duration
	ReconnectBackoff     []time.Duration
	Identity             *crypto.DeviceIdentity
	TrustStore           *crypto.TrustStore
	ClipboardEngine      *clipboard.Engine
	// TransferEngine carries file transfers over the dedicated "transfer"
	// DataChannel (DEC-024). It is independent of the clipboard engine.
	TransferEngine *transfer.Engine
}

// DefaultSessionConfig returns production defaults for session configuration.
// The media defaults match the DEC-021 measured operating point (720p30 against
// a 4 Mbps shaper ceiling); they are a request, not an assumption — the capture
// device answers with what it actually applies.
func DefaultSessionConfig() SessionConfig {
	return SessionConfig{
		PreferredWidth:       720,
		PreferredHeight:      1600,
		PreferredFPS:         30,
		PreferredBitrateKbps: 4000,
		DiscoveryTimeout:     10 * time.Second,
		ConnectTimeout:       10 * time.Second,
		ReconnectTimeout:     15 * time.Second,
	}
}

// SessionSnapshot provides a point-in-time view of session state and metrics.
type SessionSnapshot struct {
	SessionID         string
	State             SessionState
	ReasonCode        SessionReason
	TargetDevice      discovery.Device
	StartTime         time.Time
	ConnectedDuration time.Duration
	ErrorMessage      string
	Stats             rtpmedia.StreamStats
	DroppedAUs        int64
	// SinkKind/SinkActive report where access units are being written while
	// the session lives; both are cleared on terminal transitions.
	SinkKind   SinkKind
	SinkActive bool
	// FramesReason is the typed condition of the in-app frame stream ("" =
	// healthy): FFMPEG_MISSING / FFMPEG_EXITED from the frame tap, or
	// PARAM_SETS_MISSING when IDRs arrived that could never be completed
	// with SPS/PPS. Cleared with SinkKind.
	FramesReason string
	// Requested is what this side asked for; Actual is what the capture device
	// reported applying. ActualKnown is false when the peer did not report it —
	// an unknown tuple is never back-filled from Requested, because assuming
	// "probably the same" is the silent substitution DEC-022 forbids.
	Requested   MediaParams
	Actual      MediaParams
	ActualKnown bool
	// ReconnectAttempts counts attempts in the current recovery window.
	ReconnectAttempts int
}

// StateChangeCallback receives state change events. code classifies the
// transition so callers never parse reason.
type StateChangeCallback func(oldState, newState SessionState, reason string, code SessionReason)

// Session manages a single device session lifecycle.
type Session struct {
	mu           sync.RWMutex
	sessionID    string
	state        SessionState
	cfg          SessionConfig
	targetDevice discovery.Device
	startTime    time.Time
	lastError    error
	reasonCode   SessionReason

	registry *discovery.DeviceRegistry

	// tr is the current transport; trGen identifies it. Callbacks from a
	// previous generation are ignored, which is what makes replacing a
	// transport mid-session safe: closing the old one fires its "closed"
	// callback, and that must not be mistaken for the new one failing.
	tr    transport
	trGen uint64

	requested   MediaParams
	actual      MediaParams
	actualKnown bool

	reconnecting      bool
	reconnectAttempts int

	targetEndpoint  string
	signaling       *SignalingClient
	trustStore      *crypto.TrustStore
	clipboardEngine *clipboard.Engine
	transferEngine  *transfer.Engine
	limiter         *input.Limiter

	sink       receiver.FrameSink
	sinkOwned  bool
	sinkClosed bool
	// sinkKind classifies the sink above. Only the code that chose the sink
	// can set it (a display sink is a *PipeSink by construction), and it is
	// cleared on terminal transitions so it can never outlive the session.
	sinkKind SinkKind

	// Frame-pipeline diagnostics (Phase 6 Slice 3A): the manager wraps the
	// chosen sink as PSIGuard(TapSink(inner)) and registers both parts here so
	// Snapshot can report a typed frames_reason. Cleared with sinkKind.
	frameGuard *receiver.PSIGuardSink
	frameTap   *frames.TapSink

	stopped  atomic.Bool
	terminal atomic.Bool
	stopOnce sync.Once

	factory transportFactory

	onStateChange StateChangeCallback
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewSession creates an unstarted session.
func NewSession(sessionID string, cfg SessionConfig, reg *discovery.DeviceRegistry, cb StateChangeCallback) *Session {
	if cfg.DiscoveryTimeout <= 0 {
		cfg.DiscoveryTimeout = 10 * time.Second
	}
	if cfg.ConnectTimeout <= 0 {
		cfg.ConnectTimeout = 10 * time.Second
	}
	if cfg.ReconnectTimeout <= 0 {
		cfg.ReconnectTimeout = 15 * time.Second
	}

	sigClient := NewSignalingClient(cfg.ConnectTimeout)
	if cfg.Identity != nil {
		sigClient.SetIdentity(cfg.Identity)
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Session{
		sessionID: sessionID,
		state:     StateDisconnected,
		cfg:       cfg,
		registry:  reg,
		requested: cfg.Requested.WithDefaults(MediaParams{
			Width:       cfg.PreferredWidth,
			Height:      cfg.PreferredHeight,
			FPS:         cfg.PreferredFPS,
			BitrateKbps: cfg.PreferredBitrateKbps,
		}),
		trustStore:      cfg.TrustStore,
		clipboardEngine: cfg.ClipboardEngine,
		transferEngine:  cfg.TransferEngine,
		limiter:         input.NewLimiter(input.DefaultRateLimitHz, input.DefaultBurstCapacity),
		signaling:       sigClient,
		factory:         defaultTransportFactory,
		onStateChange:   cb,
		ctx:             ctx,
		cancel:          cancel,
	}
}

// SetClipboardEngine updates the session's clipboard engine.
func (s *Session) SetClipboardEngine(eng *clipboard.Engine) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clipboardEngine = eng
}

// ClipboardEngine returns the session's active clipboard engine.
func (s *Session) ClipboardEngine() *clipboard.Engine {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clipboardEngine
}

// SetTransferEngine updates the session's transfer engine.
func (s *Session) SetTransferEngine(eng *transfer.Engine) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transferEngine = eng
}

// TransferEngine returns the session's active transfer engine.
func (s *Session) TransferEngine() *transfer.Engine {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.transferEngine
}

// TransferReady reports whether a transfer channel is currently bound to the
// session's transfer engine. A caller sends files only when this is true; the
// engine refuses earlier attempts with CODE_UNAVAILABLE rather than queueing
// them against a transport that may never appear.
func (s *Session) TransferReady() bool {
	eng := s.TransferEngine()
	return eng != nil && eng.ChannelReady()
}

// SendClipboard sends wire bytes over the active transport's clipboard DataChannel.
func (s *Session) SendClipboard(data []byte) error {
	s.mu.RLock()
	tr := s.tr
	stopping := s.ctx.Err() != nil
	s.mu.RUnlock()

	if stopping || tr == nil {
		return errors.New("engine: no active transport")
	}

	if cb, ok := tr.(interface{ SendClipboard([]byte) error }); ok {
		return cb.SendClipboard(data)
	}

	return errors.New("engine: transport does not support clipboard")
}

// SendInput validates, rate-limits, and serializes an input frame, sending it
// across the active WebRTC transport on the dedicated "input" DataChannel (DEC-027).
func (s *Session) SendInput(frame *phonebridgev1.InputFrame) error {
	if frame == nil {
		return input.ErrNilFrame
	}

	s.mu.RLock()
	st := s.state
	tr := s.tr
	stopping := s.ctx.Err() != nil
	lim := s.limiter
	s.mu.RUnlock()

	if stopping || tr == nil {
		return errors.New("engine: no active transport")
	}
	if st != StateStreaming {
		return fmt.Errorf("engine: session is in state %s, must be STREAMING to accept input", st)
	}

	if err := input.ValidateInputFrame(frame); err != nil {
		return err
	}

	if lim != nil && !lim.Allow(frame) {
		return input.ErrRateLimited
	}

	wireBytes, err := proto.Marshal(frame)
	if err != nil {
		return fmt.Errorf("engine: marshal input frame: %w", err)
	}

	if in, ok := tr.(interface{ SendInput([]byte) error }); ok {
		return in.SendInput(wireBytes)
	}

	return errors.New("engine: transport does not support input")
}

// SetTrustStore sets the trust store for checking device peer trust.
func (s *Session) SetTrustStore(ts *crypto.TrustStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trustStore = ts
}

// SetSignalingClient overrides the signaling client (e.g. for testing).
func (s *Session) SetSignalingClient(sig *SignalingClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.signaling = sig
}

// SetTransportFactory overrides how media transports are built. Test seam: it
// lets lifecycle rules be proven without a network.
func (s *Session) SetTransportFactory(f transportFactory) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f == nil {
		f = defaultTransportFactory
	}
	s.factory = f
}

// RequestedParams returns the media tuple this session asks the device for.
func (s *Session) RequestedParams() MediaParams {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.requested
}

// NegotiatedParams returns what the capture device reported applying, and
// whether it reported anything at all.
func (s *Session) NegotiatedParams() (MediaParams, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.actual, s.actualKnown
}

// Transition validates and applies a state transition, classifying it as
// ReasonNone.
func (s *Session) Transition(next SessionState, reason string) error {
	return s.TransitionCode(next, reason, ReasonNone)
}

// TransitionCode validates and applies a state transition with a typed reason.
//
// The state callback is invoked with the lock released: observers commonly read
// the session back (the manager reports the negotiated tuple, for example), and
// a callback that re-entered the mutex while the transition held it would
// deadlock on Go's non-reentrant lock.
func (s *Session) TransitionCode(next SessionState, reason string, code SessionReason) error {
	old, err := s.applyTransition(next, reason, code)
	if err != nil {
		return err
	}

	s.mu.RLock()
	cb := s.onStateChange
	s.mu.RUnlock()
	if cb != nil {
		cb(old, next, reason, code)
	}
	return nil
}

// applyTransition performs the transition under the lock and returns the
// previous state so the caller can notify observers after unlocking.
func (s *Session) applyTransition(next SessionState, reason string, code SessionReason) (SessionState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.transitionLocked(next, reason, code)
}

func (s *Session) transitionLocked(next SessionState, reason string, code SessionReason) (SessionState, error) {
	if s.state == next {
		return s.state, nil
	}

	valid := false
	switch s.state {
	case StateDisconnected:
		valid = (next == StateDiscovering || next == StateConnecting || next == StateStopped || next == StateFailed)
	case StateDiscovering:
		valid = (next == StateConnecting || next == StateFailed || next == StateStopped)
	case StateConnecting:
		valid = (next == StateConnected || next == StateFailed || next == StateStopped)
	case StateConnected:
		valid = (next == StateStreaming || next == StateReconnecting || next == StateStopped || next == StateFailed)
	case StateStreaming:
		valid = (next == StateReconnecting || next == StateStopped || next == StateFailed)
	case StateReconnecting:
		valid = (next == StateStreaming || next == StateConnected || next == StateFailed || next == StateStopped)
	case StateStopped:
		valid = (next == StateDisconnected)
	case StateFailed:
		valid = (next == StateDisconnected || next == StateStopped)
	}

	if !valid {
		return s.state, fmt.Errorf("invalid state transition: %s -> %s (reason: %s)", s.state, next, reason)
	}

	old := s.state
	s.state = next
	// A sink classification describes a live session. Clear it when entering
	// any terminal or idle state so a snapshot can never report a sink for a
	// session that no longer has one (stop closes it, failure discards it).
	if next == StateStopped || next == StateFailed || next == StateDisconnected {
		s.sinkKind = SinkKindUnspecified
		s.frameGuard = nil
		s.frameTap = nil
	}
	if code != ReasonNone || next == StateFailed {
		s.reasonCode = code
	}
	if next == StateConnected && s.startTime.IsZero() {
		s.startTime = time.Now()
	}
	return old, nil
}

// SessionID returns the unique session identifier. It is stable for the whole
// session, including across transport reconnects: a reconnect repairs the
// transport, it does not start a new session.
func (s *Session) SessionID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionID
}

// State returns the current session state.
func (s *Session) State() SessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// LifecycleCtx returns the context governing this session's lifetime. It is
// cancelled by Stop/teardown. Callers that launch work on behalf of the
// session (the manager's connect goroutine, for example) must derive from
// this — NOT from a request-scoped context, which is cancelled as soon as
// the RPC that started the session returns.
func (s *Session) LifecycleCtx() context.Context {
	return s.ctx
}

// ReasonCode returns the typed classification of the current state.
func (s *Session) ReasonCode() SessionReason {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reasonCode
}

// ReconnectAttempts returns how many attempts the current recovery window made.
func (s *Session) ReconnectAttempts() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reconnectAttempts
}

// Snapshot returns the current status and metrics.
func (s *Session) Snapshot() SessionSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var duration time.Duration
	if !s.startTime.IsZero() && (s.state == StateConnected || s.state == StateStreaming || s.state == StateReconnecting) {
		duration = time.Since(s.startTime)
	}

	var stats rtpmedia.StreamStats
	var dropped int64
	if s.tr != nil {
		stats, dropped = s.tr.Stats()
	}

	errMsg := ""
	if s.lastError != nil {
		errMsg = s.lastError.Error()
	}

	return SessionSnapshot{
		SessionID:         s.sessionID,
		State:             s.state,
		ReasonCode:        s.reasonCode,
		TargetDevice:      s.targetDevice,
		StartTime:         s.startTime,
		ConnectedDuration: duration,
		ErrorMessage:      errMsg,
		Stats:             stats,
		DroppedAUs:        dropped,
		SinkKind:          s.sinkKind,
		SinkActive:        s.sinkKind != SinkKindUnspecified && s.sink != nil && !s.sinkClosed,
		FramesReason:      s.framesReasonLocked(),
		Requested:         s.requested,
		Actual:            s.actual,
		ActualKnown:       s.actualKnown,
		ReconnectAttempts: s.reconnectAttempts,
	}
}

// SetTargetDevice associates the resolved device with the session.
func (s *Session) SetTargetDevice(dev discovery.Device) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.targetDevice = dev
}

// SetSinkKind records which kind of frame sink this session will write to.
// The manager sets it before connecting: the classification must be present in
// a snapshot taken during DISCOVERING/CONNECTING, and a later failure clears
// it through the terminal-transition rule in transitionLocked.
func (s *Session) SetSinkKind(kind SinkKind) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sinkKind = kind
}

// SetFrameDiag registers the frame-pipeline wrappers the manager installed
// around this session's sink (Phase 6 Slice 3A). Cleared with sinkKind on
// terminal transitions so diagnostics can never outlive the session.
func (s *Session) SetFrameDiag(guard *receiver.PSIGuardSink, tap *frames.TapSink) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frameGuard = guard
	s.frameTap = tap
}

// framesReasonLocked derives the typed frames_reason for Snapshot. Tap
// conditions (binary missing / converter dead) win over the PSI diagnosis;
// "" means the frame stream is healthy or has not been exercised yet.
// Caller holds s.mu.
func (s *Session) framesReasonLocked() string {
	if s.frameTap != nil {
		if r := s.frameTap.Reason(); r != "" {
			return r
		}
	}
	if s.frameGuard != nil && s.frameGuard.NeedsParamSets() {
		return frames.ReasonParamSetsMissing
	}
	return ""
}

// SetReceiver attaches the active transport. Retained for tests and for callers
// that build a receiver themselves.
func (s *Session) SetReceiver(r *receiver.Receiver) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tr = r
}

// Fail transitions the session to StateFailed with a reason code, recording the
// cause. A terminal failure stops recovery: the conditions that reach here
// (consent withdrawn, capture failed, rejected parameters) cannot heal by
// re-running the offer/answer exchange, so retrying would only delay the error
// the user needs to see.
func (s *Session) Fail(code SessionReason, err error) {
	if err == nil {
		err = errors.New(code.String())
	}
	s.mu.Lock()
	s.lastError = err
	s.mu.Unlock()

	// Mark terminal before transitioning so an in-flight reconnect loop stops.
	s.terminal.Store(true)
	_ = s.TransitionCode(StateFailed, err.Error(), code)
}

// Stop cleanly terminates the session. It is idempotent: concurrent or repeated
// calls perform teardown once and never leave a transport or sink behind.
func (s *Session) Stop(reason string) error {
	var err error
	s.stopOnce.Do(func() {
		err = s.teardown(reason)
	})
	return err
}

func (s *Session) teardown(reason string) error {
	s.stopped.Store(true)
	s.terminal.Store(true)
	s.cancel() // stops a reconnect loop and any in-flight backoff sleep

	s.mu.Lock()
	// Bump the generation so the transport's own "closed" callback cannot be
	// mistaken for a live transport failing while we are tearing down.
	s.trGen++
	tr := s.tr
	s.tr = nil
	ep := s.targetEndpoint
	sig := s.signaling
	sink := s.sink
	ownSink := s.sinkOwned && !s.sinkClosed
	if ownSink {
		s.sinkClosed = true
	}
	s.mu.Unlock()

	if ep != "" && sig != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = sig.StopSession(ctx, ep, reason, CodeOK)
		cancel()
	}

	if tr != nil {
		_ = tr.Close()
	}
	if teng := s.TransferEngine(); teng != nil {
		// The transport is being torn down: a transfer in flight cannot continue
		// on it, and DEC-024 has no resume.
		teng.DetachChannel(transfer.ReasonInterrupted, "transport closed")
	}
	if eng := s.ClipboardEngine(); eng != nil {
		eng.SetTransport(nil)
	}
	// A session owns its sink (the receiver borrows it so it can outlive an
	// individual transport) and closes it exactly once, on final teardown.
	if ownSink && sink != nil {
		_ = sink.Close()
	}

	if s.State() == StateStopped {
		return nil
	}
	return s.TransitionCode(StateStopped, reason, ReasonUserStopped)
}

// Connect negotiates WebRTC via HTTP signaling with the device endpoint and binds the receiver.
//
// Parameters are settled before the offer is composed (DEC-022), and the
// capture device's answer is recorded as the authoritative actual tuple. A
// single attempt is made: an unmet request is reported typed rather than
// retried, because the fix is a different request or user action.
func (s *Session) Connect(ctx context.Context, endpoint string, sink receiver.FrameSink) error {
	s.mu.RLock()
	ts := s.trustStore
	targetID := s.cfg.TargetDeviceID
	if s.targetDevice.ID != "" {
		targetID = s.targetDevice.ID
	}
	s.mu.RUnlock()

	// Enforce peer trust check (DEC-007 / MASTER_HANDOFF §7)
	if ts != nil && targetID != "" && !ts.IsTrusted(targetID) {
		err := fmt.Errorf("device %s is not trusted: pairing required", targetID)
		s.Fail(ReasonDeviceNotTrusted, err)
		return err
	}

	if err := s.Transition(StateConnecting, "initiating WebRTC signaling"); err != nil {
		return err
	}

	s.mu.Lock()
	s.targetEndpoint = endpoint
	if sink == nil {
		sink = receiver.NewNullSink()
	}
	s.sink = sink
	s.sinkOwned = true
	sig := s.signaling
	if sig == nil {
		sig = NewSignalingClient(s.cfg.ConnectTimeout)
		s.signaling = sig
	}
	s.mu.Unlock()

	code, msg := s.attemptTransport(ctx)
	if code != CodeOK {
		err := fmt.Errorf("connect to %s failed: %s: %s", endpoint, code, msg)
		s.closeCurrentTransport()
		s.Fail(code.Reason(), err)
		return err
	}

	// Recovery is judged by transport state, never by frame arrival: a phone
	// whose screen is off legitimately produces 0 fps, and that must not look
	// like a broken link.
	if !s.waitForTransportUp(s.cfg.ConnectTimeout) {
		s.closeCurrentTransport()
		err := fmt.Errorf("connect to %s failed: transport did not come up within %v", endpoint, s.cfg.ConnectTimeout)
		s.Fail(ReasonTransportFailed, err)
		return err
	}

	return nil
}

// waitForTransportUp blocks until the transport reports connected (or media is
// already flowing).
func (s *Session) waitForTransportUp(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		switch s.State() {
		case StateConnected, StateStreaming:
			return true
		case StateFailed, StateStopped:
			return false
		}
		if s.ctx.Err() != nil {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// attemptTransport performs one full connection attempt: handshake + parameter
// negotiation, SDP offer/answer, and transport construction.
//
// Invariant: on CodeOK the session holds exactly one live transport; on any
// other outcome the transport created here is already closed and s.tr is nil.
// That invariant is what makes "resources are closed before replacement" and
// "no leaks under racing Stop" testable.
func (s *Session) attemptTransport(ctx context.Context) (Code, string) {
	old, gen := s.beginTransportReplacement()
	if old != nil {
		// Close the outgoing transport BEFORE its replacement exists, so two
		// peer connections never overlap. This is transport-only: the capture
		// pipeline on the device is untouched (DEC-020 forbids restarting it).
		_ = old.Close()
	}

	s.mu.RLock()
	ep := s.targetEndpoint
	sig := s.signaling
	sink := s.sink
	requested := s.requested
	ts := s.trustStore
	targetID := s.cfg.TargetDeviceID
	if s.targetDevice.ID != "" {
		targetID = s.targetDevice.ID
	}
	s.mu.RUnlock()

	// Re-check trust on every attempt: a device revoked mid-session must not be
	// quietly reconnected.
	if ts != nil && targetID != "" && !ts.IsTrusted(targetID) {
		return CodePermissionDenied, fmt.Sprintf("device %s is not trusted: pairing required", targetID)
	}

	tr, err := s.newTransport(gen, sink)
	if err != nil {
		return CodeTransportFailed, err.Error()
	}

	fail := func(code Code, msg string) (Code, string) {
		_ = tr.Close()
		return code, msg
	}

	resp, err := sig.RequestOffer(ctx, ep, NegotiationRequest{Requested: requested})
	if err != nil {
		var sigErr *SignalError
		if errors.As(err, &sigErr) {
			return fail(CodeSignalingFailed, err.Error())
		}
		return fail(CodeSignalingFailed, err.Error())
	}

	s.mu.Lock()
	s.actual = resp.Actual
	s.actualKnown = resp.ActualKnown
	s.mu.Unlock()

	// Version and capability validation happen before the SDP is consumed: a
	// peer we cannot talk to should not get as far as building ICE candidates.
	// A mismatch is a hard failure (DEC-022): there is no negotiation fallback
	// this milestone, because a half-negotiated session would have to guess at
	// semantics neither side promised.
	if resp.ProtocolVersion != 0 && resp.ProtocolVersion != signalingVersion {
		return fail(CodeIncompatibleVersion, fmt.Sprintf(
			"device speaks signaling version %d, this build speaks %d", resp.ProtocolVersion, signalingVersion))
	}
	// Only a peer that actually advertised capabilities can fail this check:
	// an absent capability list means "not reported", not "no screen support".
	if len(resp.Capabilities) > 0 {
		supportsScreen := false
		for _, c := range resp.Capabilities {
			if c.SupportsScreen {
				supportsScreen = true
				break
			}
		}
		if !supportsScreen {
			return fail(CodeUnsupportedMediaParams, "device advertised no screen-capture capability")
		}
	}

	if !resp.Accepted {
		code := resp.Code
		if code == CodeOK {
			code = CodeUnsupportedMediaParams
		}
		msg := resp.RejectReason
		if msg == "" {
			msg = fmt.Sprintf("device rejected %s", requested)
		}
		return fail(code, msg)
	}

	answer, err := tr.SetRemoteOffer(pion.SessionDescription{
		Type: pion.SDPTypeOffer,
		SDP:  resp.Offer,
	})
	if err != nil {
		return fail(CodeTransportFailed, fmt.Sprintf("set remote offer: %v", err))
	}

	if err := sig.SendAnswer(ctx, ep, answer); err != nil {
		return fail(CodeSignalingFailed, err.Error())
	}

	// Promote to STREAMING when media actually starts. Started per transport and
	// guarded by generation so a replaced transport cannot advance the state.
	go s.awaitTrack(gen, tr)

	return CodeOK, ""
}

// newTransport builds the transport for generation gen, wiring callbacks so
// they are ignored if this transport is later replaced.
func (s *Session) newTransport(gen uint64, sink receiver.FrameSink) (transport, error) {
	s.mu.RLock()
	factory := s.factory
	s.mu.RUnlock()

	tr, err := factory(receiver.Config{
		Sink: sink,
		// The session owns the sink: it must outlive an individual transport so
		// a reconnect does not restart the display path, and is closed once on
		// final teardown.
		KeepSinkOpen:    true,
		IncludeLoopback: true,
		OnStateChange: func(st pion.PeerConnectionState) {
			s.handleTransportState(gen, st)
		},
		OnSessionError: func(code, message string) {
			s.handleDeviceError(gen, code, message)
		},
		OnClipboardMessage: func(data []byte) {
			s.handleClipboardMessage(gen, data)
		},
		OnClipboardOpen: func() {
			s.handleClipboardOpen(gen)
		},
		OnTransferMessage: func(data []byte) {
			s.handleTransferMessage(gen, data)
		},
		OnTransferOpen: func() {
			s.handleTransferOpen(gen)
		},
		OnTransferClose: func() {
			s.handleTransferClose(gen)
		},
	})
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.tr = tr
	s.mu.Unlock()
	return tr, nil
}

// handleClipboardMessage processes an inbound clipboard payload from the transport.
func (s *Session) handleClipboardMessage(gen uint64, data []byte) {
	s.mu.RLock()
	stale := gen != s.trGen
	stopping := s.ctx.Err() != nil
	eng := s.clipboardEngine
	s.mu.RUnlock()

	if stale || stopping || eng == nil {
		return
	}

	_ = eng.OnRemoteBytes(s.ctx, data)
}

// handleClipboardOpen configures the outbound transport and triggers reconnect sync.
func (s *Session) handleClipboardOpen(gen uint64) {
	s.mu.RLock()
	stale := gen != s.trGen
	stopping := s.ctx.Err() != nil
	eng := s.clipboardEngine
	s.mu.RUnlock()

	if stale || stopping || eng == nil {
		return
	}

	targetID := s.cfg.TargetDeviceID
	if s.targetDevice.ID != "" {
		targetID = s.targetDevice.ID
	}
	remoteRole := clipboard.RoleMobile
	if s.trustStore != nil && targetID != "" {
		if entry, ok := s.trustStore.Get(targetID); ok && entry.Platform == "linux" {
			remoteRole = clipboard.RoleDesktop
		}
	}
	if targetID != "" {
		eng.SetPeer(remoteRole, targetID)
	}

	eng.SetTransport(clipboard.TransportFunc(func(ctx context.Context, update *phonebridgev1.ClipboardUpdate) error {
		wireBytes, err := proto.Marshal(update)
		if err != nil {
			return err
		}
		return s.SendClipboard(wireBytes)
	}))

	_ = eng.OnDataChannelOpen(s.ctx)
}

// handleTransferMessage feeds one transfer frame to the local transfer engine.
func (s *Session) handleTransferMessage(gen uint64, data []byte) {
	s.mu.RLock()
	stale := gen != s.trGen
	stopping := s.ctx.Err() != nil
	eng := s.transferEngine
	s.mu.RUnlock()

	if stale || stopping || eng == nil {
		return
	}
	eng.OnFrame(data)
}

// handleTransferOpen binds the freshly opened "transfer" DataChannel to the
// local transfer engine. Only the transport that created the channel (this
// side's receiver) has the adapter, so the session is the attach point.
func (s *Session) handleTransferOpen(gen uint64) {
	s.mu.RLock()
	stale := gen != s.trGen
	stopping := s.ctx.Err() != nil
	eng := s.transferEngine
	tr := s.tr
	s.mu.RUnlock()

	if stale || stopping || eng == nil || tr == nil {
		return
	}
	targetID := s.cfg.TargetDeviceID
	if s.targetDevice.ID != "" {
		targetID = s.targetDevice.ID
	}
	eng.SetPeerDeviceID(targetID)
	eng.AttachChannel(tr.TransferChannel())
}

// handleTransferClose interrupts in-flight transfers as soon as the channel is
// gone, rather than letting each one wait out its own stall timeout (DEC-024:
// an interrupted transfer fails, it never resumes).
func (s *Session) handleTransferClose(gen uint64) {
	s.mu.RLock()
	stale := gen != s.trGen
	eng := s.transferEngine
	tr := s.tr
	s.mu.RUnlock()

	if stale || eng == nil || tr == nil {
		return
	}
	eng.DetachChannelIf(tr.TransferChannel(), transfer.ReasonInterrupted, "transfer channel closed")
}

// beginTransportReplacement invalidates the outgoing transport's callbacks and
// detaches it, returning it (to be closed) plus the generation the new
// transport must use.
func (s *Session) beginTransportReplacement() (transport, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trGen++
	old := s.tr
	s.tr = nil
	return old, s.trGen
}

// closeCurrentTransport closes and detaches the live transport, invalidating its
// callbacks first so the close is not mistaken for a link failure.
func (s *Session) closeCurrentTransport() {
	old, _ := s.beginTransportReplacement()
	if old != nil {
		_ = old.Close()
	}
}

// handleTransportState reacts to a transport's connection state.
func (s *Session) handleTransportState(gen uint64, st pion.PeerConnectionState) {
	s.mu.RLock()
	stale := gen != s.trGen
	stopping := s.ctx.Err() != nil
	s.mu.RUnlock()
	if stale || stopping {
		return
	}

	switch st {
	case pion.PeerConnectionStateConnected:
		_ = s.TransitionCode(StateConnected, "WebRTC connected", ReasonNone)
	case pion.PeerConnectionStateDisconnected:
		s.RequestReconnect("WebRTC disconnected")
	case pion.PeerConnectionStateFailed:
		s.RequestReconnect("WebRTC connection failed")
	case pion.PeerConnectionStateClosed:
		// The live transport closed without us asking: treat it as a link loss
		// that may still be recoverable.
		s.RequestReconnect("WebRTC transport closed")
	}
}

// handleDeviceError reacts to a typed failure reported by the device over the
// transport's control channel. Sender-side conditions (consent withdrawal,
// capture failure) are terminal and must NOT be turned into reconnect attempts:
// reconnecting cannot restore a consent the user revoked, and misclassifying
// them as transport failures would hide the real cause from the user.
func (s *Session) handleDeviceError(gen uint64, code string, message string) {
	s.mu.RLock()
	stale := gen != s.trGen
	stopping := s.ctx.Err() != nil
	s.mu.RUnlock()
	if stale || stopping {
		return
	}

	parsed, ok := ParseCode(code)
	if !ok {
		parsed = CodeCaptureFailed
	}
	if parsed == CodeOK {
		return
	}
	if parsed.Retryable() {
		s.RequestReconnect(fmt.Sprintf("device reported %s: %s", parsed, message))
		return
	}

	reason := parsed.Reason()
	if reason == ReasonUnspecified {
		reason = ReasonCaptureFailed
	}
	err := fmt.Errorf("device reported %s: %s", parsed, message)
	if message == "" {
		err = fmt.Errorf("device reported %s", parsed)
	}
	s.closeCurrentTransport()
	s.Fail(reason, err)
}

// RequestReconnect enters RECONNECTING and starts the bounded recovery loop.
// It is a no-op when a recovery is already running, when the session is
// stopping, or when the state machine does not allow recovery from the current
// state (for example during the initial CONNECTING attempt, whose failure is
// reported directly).
func (s *Session) RequestReconnect(reason string) {
	if s.IsTerminal() {
		return
	}
	s.mu.Lock()
	if s.reconnecting {
		s.mu.Unlock()
		return
	}
	s.reconnecting = true
	s.mu.Unlock()

	if err := s.TransitionCode(StateReconnecting, reason, ReasonTransportFailed); err != nil {
		s.mu.Lock()
		s.reconnecting = false
		s.mu.Unlock()
		return
	}
	go s.reconnectLoop()
}

// IsTerminal reports whether the session has reached a state from which
// automatic recovery is not attempted.
func (s *Session) IsTerminal() bool {
	if s.stopped.Load() || s.terminal.Load() {
		return true
	}
	switch s.State() {
	case StateFailed, StateStopped:
		return true
	default:
		return false
	}
}

// reconnectLoop re-runs the offer/answer exchange with bounded backoff until the
// transport is back, the budget is exhausted, or the session stops.
//
// Capture is never touched: the device keeps its MediaProjection, VirtualDisplay
// and encoder alive, and DEC-020 records why restarting them would be wrong
// (re-binding an encoder to a live VirtualDisplay does not resume delivery, and
// a geometry change needs a fresh consent).
func (s *Session) reconnectLoop() {
	defer func() {
		s.mu.Lock()
		s.reconnecting = false
		s.mu.Unlock()
	}()

	deadline := time.Now().Add(s.cfg.ReconnectTimeout)

	for {
		if s.ctx.Err() != nil || s.stopped.Load() {
			return
		}
		switch s.State() {
		case StateStreaming, StateConnected:
			return // recovered
		case StateFailed, StateStopped:
			return // terminal
		}
		if !time.Now().Before(deadline) {
			s.closeCurrentTransport()
			s.Fail(ReasonReconnectTimeout, fmt.Errorf(
				"reconnect budget %v exhausted after %d attempt(s)",
				s.cfg.ReconnectTimeout, s.ReconnectAttempts()))
			return
		}

		delay := s.reconnectBackoff(s.ReconnectAttempts())
		if !sleepCtx(s.ctx, delay) {
			return // stopped while waiting
		}

		s.mu.Lock()
		s.reconnectAttempts++
		attempts := s.reconnectAttempts
		s.mu.Unlock()

		code, msg := s.attemptTransport(s.ctx)
		if code == CodeOK {
			if s.waitForTransportUp(s.cfg.ConnectTimeout) {
				_ = s.TransitionCode(StateConnected, "WebRTC reconnected", ReasonNone)
				return
			}
			// The attempt completed but the transport never came up: keep
			// trying while the budget lasts.
			continue
		}
		if !code.Retryable() {
			s.closeCurrentTransport()
			s.Fail(code.Reason(), fmt.Errorf("reconnect aborted after %d attempt(s): %s: %s", attempts, code, msg))
			return
		}
	}
}

// reconnectBackoff returns the delay before the given attempt index.
func (s *Session) reconnectBackoff(attempt int) time.Duration {
	s.mu.RLock()
	sched := s.cfg.ReconnectBackoff
	s.mu.RUnlock()
	if len(sched) == 0 {
		sched = defaultReconnectBackoff
	}
	if attempt < len(sched) {
		return sched[attempt]
	}
	return sched[len(sched)-1]
}

// awaitTrack promotes the session to STREAMING once media actually arrives.
// Media never arriving (screen off) is not a failure and does not trigger
// recovery — only transport state does.
//
// The wait is a poll for the lifetime of this transport generation, not a
// single bounded wait: the phone legitimately delivers ~0 fps while the
// screen is static (DEC-020), so the first RTP packet can arrive long after
// ConnectTimeout. A one-shot wait stranded healthy sessions in CONNECTED
// forever (Phase 2 acceptance: 2,414 RTP packets on the wire, state stuck at
// CONNECTED). Media arrival promotes; it can never fail the session.
func (s *Session) awaitTrack(gen uint64, tr transport) {
	for {
		err := tr.WaitForTrack(time.Second)
		s.mu.RLock()
		stale := gen != s.trGen
		s.mu.RUnlock()
		if err == nil {
			if stale {
				return
			}
			_ = s.TransitionCode(StateStreaming, "media track active", ReasonNone)
			return
		}
		if stale {
			return
		}
		select {
		case <-s.ctx.Done():
			return
		default:
		}
		if s.stopped.Load() {
			return
		}
		switch s.State() {
		case StateFailed, StateStopped:
			return
		}
	}
}

// LocateAndConnect resolves the target device and immediately establishes the media connection.
func (s *Session) LocateAndConnect(ctx context.Context, sink receiver.FrameSink) error {
	dev, err := s.LocateTarget(ctx)
	if err != nil {
		return err
	}

	port := dev.Port
	if port == 0 {
		port = 7804
	}

	endpoint, ok := Endpoint(dev.Addresses, port)
	if !ok {
		// No dialable address known (loopback fallback preserved for tests).
		endpoint = fmt.Sprintf("127.0.0.1:%d", port)
	}

	return s.Connect(ctx, endpoint, sink)
}

// LocateTarget resolves the target device in the registry within the discovery timeout.
func (s *Session) LocateTarget(ctx context.Context) (discovery.Device, error) {
	if s.cfg.TargetDeviceID == "" {
		return discovery.Device{}, errors.New("target device ID cannot be empty")
	}

	_ = s.Transition(StateDiscovering, "locating device via mDNS")

	// Check registry immediately
	if dev, ok := s.registry.Get(s.cfg.TargetDeviceID); ok && !dev.IsStale {
		s.SetTargetDevice(dev)
		return dev, nil
	}

	// Poll until timeout or found
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(s.cfg.DiscoveryTimeout)

	for {
		select {
		case <-ctx.Done():
			err := ctx.Err()
			if s.ctx.Err() == nil && !s.stopped.Load() {
				s.Fail(ReasonSignalingFailed, err)
			}
			return discovery.Device{}, err
		case <-s.ctx.Done():
			return discovery.Device{}, context.Canceled
		case <-timeout:
			err := fmt.Errorf("device %s not discovered within %v", s.cfg.TargetDeviceID, s.cfg.DiscoveryTimeout)
			s.Fail(ReasonDeviceNotFound, err)
			return discovery.Device{}, err
		case <-ticker.C:
			if dev, ok := s.registry.Get(s.cfg.TargetDeviceID); ok && !dev.IsStale {
				s.SetTargetDevice(dev)
				return dev, nil
			}
		}
	}
}

// sleepCtx sleeps for d, returning false if the context was cancelled first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
