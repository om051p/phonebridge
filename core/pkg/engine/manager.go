package engine

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	pion "github.com/pion/webrtc/v4"

	"github.com/om051p/phonebridge/core/pkg/clipboard"
	"github.com/om051p/phonebridge/core/pkg/crypto"
	"github.com/om051p/phonebridge/core/pkg/discovery"
	"github.com/om051p/phonebridge/core/pkg/frames"
	"github.com/om051p/phonebridge/core/pkg/notification"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgelocalipcv1"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"github.com/om051p/phonebridge/core/pkg/receiver"
	"github.com/om051p/phonebridge/core/pkg/transfer"
)

// SessionEvent models an event emitted by SessionManager.
type SessionEvent struct {
	SessionID    string
	State        SessionState
	Reason       string
	ReasonCode   SessionReason
	ErrorMessage string
	// Requested/Actual carry the media negotiation when it is known, so the
	// local IPC snapshot and UI show what was asked for versus what the device
	// applied (DEC-022).
	Requested   MediaParams
	Actual      MediaParams
	ActualKnown bool
}

type pendingPairing struct {
	deviceID   string
	endpoint   string
	token      string
	sas        string
	remoteName string
	remotePub  []byte
	createdAt  time.Time
}

// SessionManager coordinates device discovery and active session lifecycle.
type SessionManager struct {
	mu               sync.RWMutex
	cfg              SessionConfig
	discovery        *discovery.Discovery
	activeSess       *Session
	inboundSess      *InboundSession
	onEvent          func(SessionEvent)
	sink             receiver.FrameSink
	sinkFactory      func() (receiver.FrameSink, error)
	identity         *crypto.DeviceIdentity
	trustStore       *crypto.TrustStore
	clipboardEngine  *clipboard.Engine
	transferEngine   *transfer.Engine
	clipboardAdapter clipboard.PlatformAdapter
	pendingPairings  map[string]*pendingPairing
	httpClient       *http.Client
	// frameHub receives completed frames for local-IPC StreamFrames (Phase 6
	// Slice 3). When set, sessions are wrapped PSIGuard(TapSink(sink)).
	frameHub *frames.Hub

	notificationStore *notification.Store
	onNotification    func(*phonebridgev1.NotificationFrame)
}

// NewSessionManager creates a new session coordinator.
func NewSessionManager(cfg SessionConfig, disc *discovery.Discovery, sink receiver.FrameSink, onEvent func(SessionEvent)) *SessionManager {
	return &SessionManager{
		cfg:               cfg,
		discovery:         disc,
		sink:              sink,
		onEvent:           onEvent,
		identity:          cfg.Identity,
		trustStore:        cfg.TrustStore,
		pendingPairings:   make(map[string]*pendingPairing),
		httpClient:        &http.Client{Timeout: 10 * time.Second},
		notificationStore: notification.NewStore(),
	}
}

// SetIdentity configures the local cryptographic device identity.
func (m *SessionManager) SetIdentity(identity *crypto.DeviceIdentity) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.identity = identity
	m.cfg.Identity = identity
}

// Identity returns the current device identity.
func (m *SessionManager) Identity() *crypto.DeviceIdentity {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.identity
}

// SetTrustStore configures the trusted peers store.
func (m *SessionManager) SetTrustStore(ts *crypto.TrustStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.trustStore = ts
	m.cfg.TrustStore = ts
}

// SetDiscovery updates the active mDNS discovery manager.
func (m *SessionManager) SetDiscovery(disc *discovery.Discovery) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.discovery = disc
}

// SetFrameHub installs the frame fan-out hub (Phase 6 Slice 3). Must be set
// before StartSession; nil keeps the previous behaviour (no in-app frames).
func (m *SessionManager) SetFrameHub(hub *frames.Hub) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.frameHub = hub
}

// FrameHub returns the installed hub (local IPC uses it to serve StreamFrames).
func (m *SessionManager) FrameHub() *frames.Hub {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.frameHub
}

// SetClipboardEngine configures the clipboard engine for all managed sessions.
func (m *SessionManager) SetClipboardEngine(eng *clipboard.Engine) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clipboardEngine = eng
	m.cfg.ClipboardEngine = eng
	if m.activeSess != nil {
		m.activeSess.SetClipboardEngine(eng)
	}
}

// ClipboardEngine returns the configured clipboard engine.
func (m *SessionManager) ClipboardEngine() *clipboard.Engine {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.clipboardEngine
}

// SetTransferEngine configures the file-transfer engine for all managed
// sessions, outbound and inbound (DEC-024).
func (m *SessionManager) SetTransferEngine(eng *transfer.Engine) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.transferEngine = eng
	m.cfg.TransferEngine = eng
	if m.activeSess != nil {
		m.activeSess.SetTransferEngine(eng)
	}
}

// TransferEngine returns the configured file-transfer engine.
func (m *SessionManager) TransferEngine() *transfer.Engine {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.transferEngine
}

// SetClipboardAdapter configures the platform clipboard adapter.
func (m *SessionManager) SetClipboardAdapter(adapter clipboard.PlatformAdapter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clipboardAdapter = adapter
}

// GetClipboardStatus queries the state of the clipboard engine and adapter.
func (m *SessionManager) GetClipboardStatus(ctx context.Context) (*phonebridgelocalipcv1.GetClipboardStatusResponse, error) {
	m.mu.RLock()
	eng := m.clipboardEngine
	adapter := m.clipboardAdapter
	m.mu.RUnlock()

	resp := &phonebridgelocalipcv1.GetClipboardStatusResponse{
		State:          "STOPPED",
		MaxPayloadSize: clipboard.MaxPayloadSize,
	}

	if adapter != nil {
		if linuxAdapter, ok := adapter.(*clipboard.LinuxAdapter); ok {
			st := linuxAdapter.Status()
			resp.AdapterStatus = st.String()
			switch st {
			case clipboard.AdapterStatusReady:
				resp.State = "AMBIENT_ACTIVE"
			case clipboard.AdapterStatusCosmicFlagRequired:
				resp.State = "COSMIC_FLAG_REQUIRED"
			case clipboard.AdapterStatusNoDataControl:
				resp.State = "NO_DATA_CONTROL"
			case clipboard.AdapterStatusNoBackend:
				resp.State = "NO_BACKEND"
			case clipboard.AdapterStatusWaylandUnavailable:
				resp.State = "WAYLAND_UNAVAILABLE"
			case clipboard.AdapterStatusCrashed:
				resp.State = "UNAVAILABLE"
			default:
				resp.State = "STOPPED"
			}
		} else {
			resp.State = "READY"
			resp.AdapterStatus = "READY"
		}
	}

	if eng != nil {
		resp.IsConnected = eng.HasTransport()
		resp.RemotePeerId = eng.RemotePeerID()
		if cur := eng.CurrentItem(); cur != nil {
			resp.LastSyncMs = cur.CopiedAtMs
		}
	}

	return resp, nil
}

// TriggerClipboardPull re-broadcasts the active clipboard item or triggers reconnect sync.
func (m *SessionManager) TriggerClipboardPull(ctx context.Context) error {
	m.mu.RLock()
	eng := m.clipboardEngine
	m.mu.RUnlock()

	if eng == nil {
		return errors.New("clipboard engine not configured")
	}

	cur := eng.CurrentItem()
	if cur != nil {
		return eng.OnLocalClipboard(ctx, cur)
	}
	return nil
}

// SendFile offers a local file to the peer of the active session (DEC-024).
//
// The target is checked against the live session rather than the discovery
// registry: file transfer runs on the session's own DataChannel, so offering a
// file to a device that is merely discovered would be a promise the transport
// cannot keep (and the engine's CODE_UNAVAILABLE is the honest answer).
func (m *SessionManager) SendFile(ctx context.Context, deviceID, localPath, filename string) (string, error) {
	eng := m.TransferEngine()
	if eng == nil {
		return "", transfer.NewFailure(phonebridgev1.Code_CODE_UNAVAILABLE, transfer.ReasonUnsupportedPeer,
			"file transfer is not configured on this device")
	}

	m.mu.RLock()
	sess := m.activeSess
	m.mu.RUnlock()

	if sess == nil {
		return "", transfer.NewFailure(phonebridgev1.Code_CODE_UNAVAILABLE, transfer.ReasonNoSession,
			"no active session: connect to a device before sending a file")
	}
	if target := sess.Snapshot().TargetDevice.ID; deviceID != "" && deviceID != target {
		return "", transfer.NewFailure(phonebridgev1.Code_CODE_INVALID_ARGUMENT, transfer.ReasonNoSession,
			"device %s is not the active session's peer (%s)", deviceID, target)
	}
	if !sess.TransferReady() {
		return "", transfer.NewFailure(phonebridgev1.Code_CODE_UNAVAILABLE, transfer.ReasonNoSession,
			"the peer has not opened the transfer channel yet")
	}
	return eng.SendFile(ctx, localPath, filename)
}

// CancelTransfer aborts an in-flight transfer in either direction.
func (m *SessionManager) CancelTransfer(ctx context.Context, transferID string) error {
	eng := m.TransferEngine()
	if eng == nil {
		return errors.New("file transfer is not configured on this device")
	}
	return eng.Cancel(ctx, transferID)
}

// ListTransfers returns in-flight transfers plus the recent history, newest
// first.
func (m *SessionManager) ListTransfers() []transfer.Info {
	eng := m.TransferEngine()
	if eng == nil {
		return nil
	}
	return eng.List()
}

// SendInput validates and routes an input frame to the active session (DEC-027).
func (m *SessionManager) SendInput(ctx context.Context, sessionID string, frame *phonebridgev1.InputFrame) error {
	m.mu.RLock()
	sess := m.activeSess
	m.mu.RUnlock()

	if sess == nil {
		return errors.New("no active session: connect to a device before sending input")
	}
	if sessionID != "" && sess.SessionID() != sessionID {
		return fmt.Errorf("session %s is not the active session (%s)", sessionID, sess.SessionID())
	}
	return sess.SendInput(frame)
}

// TransferEngineReady reports whether the active session has a usable transfer
// channel; it is used to gate the UI's "send file" affordance.
func (m *SessionManager) TransferEngineReady() bool {
	m.mu.RLock()
	sess := m.activeSess
	m.mu.RUnlock()
	return sess != nil && sess.TransferReady()
}

// SetNotificationHandler registers a callback invoked when inbound notification frames arrive.
func (m *SessionManager) SetNotificationHandler(handler func(*phonebridgev1.NotificationFrame)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onNotification = handler
}

// ListNotifications returns the active in-memory mirrored notifications (DEC-028).
func (m *SessionManager) ListNotifications() []*phonebridgev1.NotificationPosted {
	return m.notificationStore.List()
}

// TrustStore returns the active trust store.
func (m *SessionManager) TrustStore() *crypto.TrustStore {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.trustStore
}

// SetSinkFactory configures dynamic FrameSink creation for each initiated session.
func (m *SessionManager) SetSinkFactory(factory func() (receiver.FrameSink, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sinkFactory = factory
}

// ListDevices returns all currently discovered LAN devices from mDNS.
func (m *SessionManager) ListDevices() []discovery.Device {
	if m.discovery == nil {
		return nil
	}
	return m.discovery.Registry().List()
}

// StartSession initiates a session targeting the given device ID, requesting
// the given media tuple (zero fields fall back to the manager's defaults).
func (m *SessionManager) StartSession(ctx context.Context, deviceID string, requested MediaParams) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeSess != nil {
		st := m.activeSess.State()
		if st != StateStopped && st != StateFailed && st != StateDisconnected {
			return nil, fmt.Errorf("a session is already active (id=%s, state=%s)", m.activeSess.sessionID, st)
		}
	}

	sessionID := generateSessionID()
	cfg := m.cfg
	cfg.TargetDeviceID = deviceID
	cfg.Identity = m.identity
	cfg.TrustStore = m.trustStore
	cfg.Requested = requested
	cfg.NotificationStore = m.notificationStore
	cfg.OnNotification = func(frame *phonebridgev1.NotificationFrame) {
		m.mu.RLock()
		handler := m.onNotification
		m.mu.RUnlock()
		if handler != nil {
			handler(frame)
		}
	}

	// Captured for the state callback (which must not re-lock m.mu), together
	// with the hub session token. The callback runs AFTER the transition has
	// been applied and its lock released, so a late terminal callback from this
	// session can land after the NEXT session has already begun — and the next
	// session is accepted precisely because this one's state is already
	// terminal. Ending with this session's own token makes that late call a
	// no-op instead of a cross-session stall (see frames.Hub's doc comment).
	hub := m.frameHub
	// hubTok is published by the connect goroutine (below) and read by the state
	// callback, so it carries its own mutex rather than reusing m.mu: the
	// callback must never take a lock the session path can hold across a
	// transition.
	hubTok := &sessionHubToken{}

	var reg *discovery.DeviceRegistry
	if m.discovery != nil {
		reg = m.discovery.Registry()
	}
	// Declared first because the state callback reads the session's negotiated
	// parameters; the callback only ever runs after StartSession returns.
	var sess *Session
	sess = NewSession(sessionID, cfg, reg, func(oldState, newState SessionState, reason string, code SessionReason) {
		// Frame-session lifecycle follows the session's terminal states: once
		// the session is gone the hub closes every StreamFrames subscription so
		// no stale frame can outlive it (Slice 3A). Idempotent, and scoped to
		// this session's token so it can never close a newer session's window.
		switch newState {
		case StateFailed, StateStopped, StateDisconnected:
			if tok := hubTok.get(); hub != nil && tok != 0 {
				// ErrSessionNotCurrent here means this session was already
				// superseded: the newer session's window must stay open.
				_ = hub.EndSession(tok)
			}
			m.notificationStore.Clear()
		}
		if m.onEvent != nil {
			errMsg := ""
			if newState == StateFailed {
				errMsg = reason
			}
			requested := sess.RequestedParams()
			actual, actualKnown := sess.NegotiatedParams()
			m.onEvent(SessionEvent{
				SessionID:    sessionID,
				State:        newState,
				Reason:       reason,
				ReasonCode:   code,
				ErrorMessage: errMsg,
				Requested:    requested,
				Actual:       actual,
				ActualKnown:  actualKnown,
			})
		}
	})

	m.activeSess = sess

	// Asynchronously locate and connect to target device
	go func() {
		m.mu.RLock()
		hub := m.frameHub
		m.mu.RUnlock()

		sink, tok := m.resolveSink(sess, hub)
		if tok != 0 {
			hubTok.set(tok)
		}
		// Run on the session's OWN lifecycle context: a request-scoped ctx
		// (the gRPC handler's) is cancelled the moment StartSession returns,
		// which killed the in-flight signaling POST and failed every session
		// started over real IPC (found in Phase 2 acceptance).
		if err := sess.LocateAndConnect(sess.LifecycleCtx(), sink); err != nil {
			// LocateAndConnect already transitions to StateFailed on error
		}
	}()

	return sess, nil
}

// sessionHubToken is the frame-hub session token published for one session. It
// owns its own mutex (see the comment at its use site in StartSession) so the
// session state callback never has to take m.mu.
type sessionHubToken struct {
	mu    sync.Mutex
	token uint64
}

func (t *sessionHubToken) set(v uint64) {
	t.mu.Lock()
	t.token = v
	t.mu.Unlock()
}

func (t *sessionHubToken) get() uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.token
}

// resolveSink selects and wires the frame sink for a session.
//
// Selection order is the production preference: a configured factory, then the
// static sink, then a display sink when a desktop session is available, then a
// null sink. The chosen sink is then wrapped as PSIGuard(TapSink(inner)) so the
// access-unit stream is teed into the StreamFrames hub behind a session-scoped
// PSI guard, and the hub session token is returned for the caller to publish —
// that is what lets a Stop end exactly this session's frame window, and a late
// callback from a superseded session become a no-op.
//
// Callers must NOT hold m.mu (this takes it briefly, both to read the sink
// configuration and to confirm the session is still the manager's active one).
// A zero token means the hub session was rolled back because this session had
// already been replaced.
func (m *SessionManager) resolveSink(sess *Session, hub *frames.Hub) (receiver.FrameSink, uint64) {
	var sink receiver.FrameSink
	var kind SinkKind
	m.mu.RLock()
	factory := m.sinkFactory
	staticSink := m.sink
	m.mu.RUnlock()

	if factory != nil {
		if s, err := factory(); err == nil {
			sink = s
			kind = classifySinkKind(s)
		}
	}
	if sink == nil && staticSink != nil {
		sink = staticSink
		kind = classifySinkKind(staticSink)
	}
	if sink == nil && (os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "") {
		if ds, err := receiver.NewDisplaySink("PhoneBridge Screen Mirror", true); err == nil {
			sink = ds
			// NewDisplaySink is a *PipeSink by construction: only this branch
			// launched ffplay, so only this branch can say so.
			kind = SinkKindDisplay
		}
	}
	if sink == nil {
		sink = receiver.NewNullSink()
		kind = SinkKindNull
	}
	// Record the classification before connecting, so a snapshot taken during
	// DISCOVERING/CONNECTING already reports the real sink.
	sess.SetSinkKind(kind)

	if hub == nil {
		return sink, 0
	}

	tok := hub.BeginSession()
	m.mu.Lock()
	current := m.activeSess == sess
	m.mu.Unlock()
	if !current {
		// A token minted for an already-replaced session: roll the Begin back
		// atomically, so this goroutine can never end the window that replaced
		// it (a later BeginSession may have landed between the check above and
		// this call).
		_ = hub.EndSessionIfCurrent(tok)
		return sink, 0
	}

	tap := frames.NewTapSink(sink, hub)
	guard := receiver.NewPSIGuardSink(tap)
	sess.SetFrameDiag(guard, tap)
	return guard, tok
}

// PeerOfferRequest is a session offer from a peer that is itself the capture
// device (the phone starting the session), plus where that peer's signaling
// server can be reached afterwards.
type PeerOfferRequest struct {
	// PeerDeviceID is the authenticated device id from the request signature.
	PeerDeviceID string
	// Endpoint is the peer's signaling endpoint. The signaling server derives it
	// from the authenticated request's source address, never from an
	// unauthenticated body field.
	Endpoint string
	// Offer is the peer's SDP offer.
	Offer pion.SessionDescription
	// Requested is the media tuple the peer asked for (advisory: the peer is
	// authoritative for what it captures, DEC-020).
	Requested MediaParams
}

// PeerOfferResult is the typed outcome of handling a peer offer.
type PeerOfferResult struct {
	// Answer is the SDP answer the peer must apply. Empty unless Code is CodeOK.
	Answer    string
	Code      Code
	Message   string
	SessionID string
}

// HandlePeerOffer answers a session offer from a peer that is the capture side
// (DEC-022), returning the SDP answer the peer must apply.
//
// Unlike StartSession this is synchronous: the caller is an HTTP handler that
// has to carry the answer back in its own response, and the peer cannot start
// sending until it has it. It reuses the same session construction, sink/hub
// wiring and transport factory as a desktop-initiated session, so a
// peer-started session reports the same telemetry and supports the same planes.
func (m *SessionManager) HandlePeerOffer(ctx context.Context, req PeerOfferRequest) (PeerOfferResult, error) {
	if req.PeerDeviceID == "" {
		return PeerOfferResult{Code: CodePermissionDenied, Message: "peer is not authenticated"}, nil
	}

	m.mu.Lock()
	if cur := m.activeSess; cur != nil {
		st := cur.State()
		if st != StateStopped && st != StateFailed && st != StateDisconnected {
			sessionID := cur.sessionID
			m.mu.Unlock()
			return PeerOfferResult{
				Code:      CodeSessionBusy,
				Message:   fmt.Sprintf("device is already in an active session (%s)", st),
				SessionID: sessionID,
			}, nil
		}
	}

	sessionID := generateSessionID()
	cfg := m.cfg
	cfg.TargetDeviceID = req.PeerDeviceID
	cfg.Identity = m.identity
	cfg.TrustStore = m.trustStore
	cfg.Requested = req.Requested
	cfg.NotificationStore = m.notificationStore
	cfg.OnNotification = func(frame *phonebridgev1.NotificationFrame) {
		m.mu.RLock()
		handler := m.onNotification
		m.mu.RUnlock()
		if handler != nil {
			handler(frame)
		}
	}

	hub := m.frameHub
	hubTok := &sessionHubToken{}

	var reg *discovery.DeviceRegistry
	if m.discovery != nil {
		reg = m.discovery.Registry()
	}

	var sess *Session
	sess = NewSession(sessionID, cfg, reg, func(oldState, newState SessionState, reason string, code SessionReason) {
		switch newState {
		case StateFailed, StateStopped, StateDisconnected:
			if tok := hubTok.get(); hub != nil && tok != 0 {
				_ = hub.EndSession(tok)
			}
			m.notificationStore.Clear()
		}
		if m.onEvent != nil {
			errMsg := ""
			if newState == StateFailed {
				errMsg = reason
			}
			requested := sess.RequestedParams()
			actual, actualKnown := sess.NegotiatedParams()
			m.onEvent(SessionEvent{
				SessionID:    sessionID,
				State:        newState,
				Reason:       reason,
				ReasonCode:   code,
				ErrorMessage: errMsg,
				Requested:    requested,
				Actual:       actual,
				ActualKnown:  actualKnown,
			})
		}
	})

	// The peer dialled us, so it may not be in the discovery registry at all; the
	// endpoint it was authenticated from is what a reconnect will use.
	sess.SetTargetEndpoint(req.Endpoint)
	m.activeSess = sess
	m.mu.Unlock()

	sink, tok := m.resolveSink(sess, hub)
	if tok != 0 {
		hubTok.set(tok)
	}

	answer, err := sess.AnswerPeerOffer(ctx, req.Offer, sink)
	if err != nil {
		return PeerOfferResult{
			Code:      codeForReason(sess.ReasonCode()),
			Message:   err.Error(),
			SessionID: sessionID,
		}, nil
	}

	return PeerOfferResult{
		Answer:    answer.SDP,
		Code:      CodeOK,
		SessionID: sessionID,
	}, nil
}

// StopSession halts the specified session or the current active session.
func (m *SessionManager) StopSession(sessionID, reason string) error {
	m.mu.Lock()
	sess := m.activeSess
	m.mu.Unlock()

	if sess == nil {
		return fmt.Errorf("no active session")
	}
	if sessionID != "" && sess.sessionID != sessionID {
		return fmt.Errorf("session %s is not active", sessionID)
	}

	return sess.Stop(reason)
}

// GetSessionState queries the snapshot of the specified session.
func (m *SessionManager) GetSessionState(sessionID string) (SessionSnapshot, error) {
	m.mu.RLock()
	sess := m.activeSess
	m.mu.RUnlock()

	if sess == nil {
		return SessionSnapshot{
			SessionID: sessionID,
			State:     StateDisconnected,
		}, nil
	}
	if sessionID != "" && sess.sessionID != sessionID {
		return SessionSnapshot{}, fmt.Errorf("session %s not found", sessionID)
	}

	return sess.Snapshot(), nil
}

// ActiveSession returns the current active session, if any.
func (m *SessionManager) ActiveSession() *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeSess
}

// classifySinkKind maps a concrete sink to the kind reported over local IPC.
// The display sink is deliberately NOT listed here: it is a *PipeSink by
// construction and is classified by the caller that chose to launch it.
func classifySinkKind(s receiver.FrameSink) SinkKind {
	switch s.(type) {
	case *receiver.NullSink:
		return SinkKindNull
	case *receiver.FileSink:
		return SinkKindFile
	case *receiver.PipeSink:
		return SinkKindPipe
	default:
		return SinkKindUnspecified
	}
}

func generateSessionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (m *SessionManager) resolveEndpoint(deviceID string) (string, error) {
	if m.discovery == nil {
		return "", fmt.Errorf("discovery not initialized")
	}
	dev, ok := m.discovery.Registry().Get(deviceID)
	if !ok {
		return "", fmt.Errorf("device %s not discovered on LAN", deviceID)
	}
	port := dev.Port
	if port == 0 {
		port = 7804
	}
	endpoint := fmt.Sprintf("127.0.0.1:%d", port)
	if ep, ok := Endpoint(dev.Addresses, port); ok {
		endpoint = ep
	}
	return endpoint, nil
}

// PairDevice initiates pairing with a discovered LAN device and returns the SAS.
func (m *SessionManager) PairDevice(ctx context.Context, deviceID string) (string, string, error) {
	m.mu.Lock()
	identity := m.identity
	m.mu.Unlock()

	if identity == nil {
		return "", "", fmt.Errorf("local device identity is not configured")
	}

	endpoint, err := m.resolveEndpoint(deviceID)
	if err != nil {
		return "", "", err
	}

	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", fmt.Errorf("generate pairing token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	reqPayload := crypto.PairingRequestPayload{
		DisplayName:  identity.DisplayName,
		Platform:     identity.Platform,
		PublicKey:    hex.EncodeToString(identity.PublicKey),
		PairingToken: token,
	}
	reqData, err := json.Marshal(reqPayload)
	if err != nil {
		return "", "", fmt.Errorf("marshal pair request: %w", err)
	}

	reqURL := fmt.Sprintf("http://%s/pairing/request", endpoint)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(reqData))
	if err != nil {
		return "", "", fmt.Errorf("create pair request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(httpReq)
	if err != nil {
		return "", "", fmt.Errorf("send pair request to %s: %w", reqURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("pairing request rejected (%d): %s", resp.StatusCode, string(b))
	}

	var accept crypto.PairingAcceptPayload
	if err := json.NewDecoder(resp.Body).Decode(&accept); err != nil {
		return "", "", fmt.Errorf("decode pair accept: %w", err)
	}

	remotePub, err := hex.DecodeString(accept.PublicKey)
	if err != nil || len(remotePub) != ed25519.PublicKeySize {
		return "", "", fmt.Errorf("invalid remote public key: %w", err)
	}

	expectedSAS := crypto.CalculateSAS(identity.PublicKey, remotePub, token)
	if accept.SAS != expectedSAS {
		return "", "", fmt.Errorf("SAS mismatch: remote %s != calculated %s", accept.SAS, expectedSAS)
	}

	m.mu.Lock()
	m.pendingPairings[deviceID] = &pendingPairing{
		deviceID:   deviceID,
		endpoint:   endpoint,
		token:      token,
		sas:        expectedSAS,
		remoteName: accept.DisplayName,
		remotePub:  remotePub,
		createdAt:  time.Now(),
	}
	m.mu.Unlock()

	return accept.DisplayName, expectedSAS, nil
}

// ConfirmPairing confirms or rejects a pending pairing request.
func (m *SessionManager) ConfirmPairing(ctx context.Context, deviceID string, confirmed bool) error {
	m.mu.Lock()
	pending, ok := m.pendingPairings[deviceID]
	identity := m.identity
	store := m.trustStore
	m.mu.Unlock()

	if !ok || pending == nil {
		return fmt.Errorf("no pending pairing for device %s", deviceID)
	}
	if time.Since(pending.createdAt) > pendingPairingTTL {
		return fmt.Errorf("pairing token for device %s expired", deviceID)
	}
	if identity == nil {
		return fmt.Errorf("local identity is not configured")
	}

	defer func() {
		m.mu.Lock()
		delete(m.pendingPairings, deviceID)
		m.mu.Unlock()
	}()

	sigMaterial := fmt.Sprintf("%s:%s", pending.token, pending.sas)
	sig := crypto.Sign(identity.PrivateKey, []byte(sigMaterial))

	confirmPayload := crypto.PairingConfirmPayload{
		DeviceID:     identity.DeviceID,
		PairingToken: pending.token,
		SAS:          pending.sas,
		Confirmed:    confirmed,
		Signature:    hex.EncodeToString(sig),
	}
	data, err := json.Marshal(confirmPayload)
	if err != nil {
		return fmt.Errorf("marshal confirm payload: %w", err)
	}

	url := fmt.Sprintf("http://%s/pairing/confirm", pending.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create confirm request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send confirm to %s: %w", url, err)
	}
	defer resp.Body.Close()

	if !confirmed {
		return fmt.Errorf("pairing rejected by user")
	}

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("remote rejected pairing confirm (%d): %s", resp.StatusCode, string(b))
	}

	if store != nil {
		entry := crypto.TrustEntry{
			DeviceID:    deviceID,
			DisplayName: pending.remoteName,
			Platform:    "android",
			PublicKey:   pending.remotePub,
			PairedAt:    time.Now(),
			LastSeen:    time.Now(),
			Revoked:     false,
		}
		if err := store.AddTrusted(entry); err != nil {
			return fmt.Errorf("save trusted device: %w", err)
		}
	}

	return nil
}

// ListTrustedDevices returns all trusted devices from the store.
func (m *SessionManager) ListTrustedDevices() []crypto.TrustEntry {
	m.mu.RLock()
	store := m.trustStore
	m.mu.RUnlock()
	if store == nil {
		return nil
	}
	return store.List()
}

// RevokeDevice revokes a paired device.
func (m *SessionManager) RevokeDevice(deviceID string) error {
	m.mu.Lock()
	store := m.trustStore
	sess := m.activeSess
	m.mu.Unlock()

	if store == nil {
		return fmt.Errorf("trust store is not configured")
	}

	if err := store.Revoke(deviceID); err != nil {
		return err
	}

	if sess != nil && sess.State() != StateStopped && sess.State() != StateFailed {
		if sess.cfg.TargetDeviceID == deviceID || (sess.targetDevice.ID == deviceID) {
			_ = sess.Stop("device trust revoked")
		}
	}

	return nil
}

// HandleInboundOffer processes an incoming POST /session/offer from an authenticated peer.
//
// The session mutex is held only to read the busy state and to publish the new
// session, never across construction: NewInboundSession allocates a
// PeerConnection and CreateOffer waits for ICE gathering (up to ~2 s), and
// holding m.mu for that long stalls every other manager operation, including
// the Stop that would clear a stuck session.
//
// Two concurrent offers are therefore possible. Only one may win: the loser is
// closed and told the device is busy, so no second PeerConnection is left
// allocated and no caller is told it owns a session it does not.
func (m *SessionManager) HandleInboundOffer(req NegotiationRequest) (NegotiationResponse, error) {
	if busy, resp := m.inboundBusyResponse(); busy {
		return resp, nil
	}

	remoteRole := clipboard.RoleDesktop
	if m.trustStore != nil && req.PeerDeviceID != "" {
		if entry, ok := m.trustStore.Get(req.PeerDeviceID); ok && entry.Platform == "android" {
			remoteRole = clipboard.RoleMobile
		}
	}

	m.mu.RLock()
	clipboardEngine := m.clipboardEngine
	transferEngine := m.transferEngine
	m.mu.RUnlock()

	if clipboardEngine != nil && req.PeerDeviceID != "" {
		clipboardEngine.SetPeer(remoteRole, req.PeerDeviceID)
	}

	inbound, err := NewInboundSession(InboundSessionConfig{
		IncludeLoopback: true,
		PeerDeviceID:    req.PeerDeviceID,
		ClipboardEngine: clipboardEngine,
		TransferEngine:  transferEngine,
		// A session that dies without a /session/stop must release the slot, or
		// the next offer from any peer is answered SESSION_BUSY forever.
		OnDead: m.clearInboundIf,
	})
	if err != nil {
		return NegotiationResponse{}, fmt.Errorf("create inbound session: %w", err)
	}

	offer, err := inbound.CreateOffer()
	if err != nil {
		_ = inbound.Close()
		return NegotiationResponse{}, fmt.Errorf("generate inbound offer: %w", err)
	}

	m.mu.Lock()
	if m.inboundSess != nil && !m.inboundSess.IsClosed() {
		m.mu.Unlock()
		_ = inbound.Close()
		return NegotiationResponse{
			Code:         CodeSessionBusy,
			Message:      "inbound session already active",
			RejectReason: "device is currently in an active session",
		}, nil
	}
	m.inboundSess = inbound
	m.mu.Unlock()

	return NegotiationResponse{
		Offer:           offer.SDP,
		ProtocolVersion: signalingVersion,
		Accepted:        true,
		Code:            CodeOK,
	}, nil
}

// inboundBusyResponse reports whether an inbound session may not be started,
// and the refusal to return if so.
func (m *SessionManager) inboundBusyResponse() (bool, NegotiationResponse) {
	m.mu.RLock()
	activeSess := m.activeSess
	inbound := m.inboundSess
	m.mu.RUnlock()

	if activeSess != nil {
		switch activeSess.State() {
		case StateStopped, StateFailed, StateDisconnected:
		default:
			return true, NegotiationResponse{
				Code:         CodeSessionBusy,
				Message:      "session already active",
				RejectReason: "device is currently in an active session",
			}
		}
	}
	if inbound != nil && !inbound.IsClosed() {
		return true, NegotiationResponse{
			Code:         CodeSessionBusy,
			Message:      "inbound session already active",
			RejectReason: "device is currently in an active session",
		}
	}
	return false, NegotiationResponse{}
}

// clearInboundIf drops the manager's reference to a session that closed itself.
// It is the InboundSessionConfig.OnDead hook: comparing the pointer keeps a
// dying session from clearing a newer one that already replaced it.
func (m *SessionManager) clearInboundIf(sess *InboundSession, reason string) {
	m.mu.Lock()
	if m.inboundSess == sess {
		m.inboundSess = nil
	}
	m.mu.Unlock()

	// A dead inbound session is a session the local UI should stop showing as
	// live; reporting it through the existing event channel keeps the daemon and
	// the desktop UI consistent without a second notification path.
	if m.onEvent != nil {
		m.onEvent(SessionEvent{
			SessionID:    "inbound",
			State:        StateFailed,
			Reason:       reason,
			ReasonCode:   ReasonTransportFailed,
			ErrorMessage: reason,
		})
	}
}

// CloseInbound closes any allocated inbound session and releases the slot. It
// exists so daemon shutdown does not leave a PeerConnection (and its goroutines)
// behind, which is also why it is safe to call when none is allocated.
func (m *SessionManager) CloseInbound() error {
	m.mu.Lock()
	inbound := m.inboundSess
	m.inboundSess = nil
	m.mu.Unlock()

	if inbound != nil {
		return inbound.Close()
	}
	return nil
}

// HandleInboundAnswer processes the SDP answer received via POST /session/answer.
func (m *SessionManager) HandleInboundAnswer(answer pion.SessionDescription) error {
	m.mu.Lock()
	inbound := m.inboundSess
	m.mu.Unlock()

	if inbound == nil || inbound.IsClosed() {
		return errors.New("no active inbound session")
	}

	return inbound.SetRemoteAnswer(answer)
}

// HandleInboundStop processes session termination received via POST /session/stop.
//
// Deprecated in favour of HandlePeerStop, which knows WHICH authenticated peer
// asked: kept only for callers that have no peer identity (tests, and the
// trust-store-less configuration).
func (m *SessionManager) HandleInboundStop(reason string, code Code) error {
	return m.HandlePeerStop("", reason, code)
}

// HandlePeerStop releases the session the named peer says it has ended.
//
// Three flavours of session can be live: a desktop-initiated one (activeSess,
// where that peer is the target), a peer-offer one (activeSess, where this node
// answered that peer's offer), and the legacy inbound responder (inboundSess).
// All three are released here — but only when the requesting device is actually
// part of the session: an authenticated peer must not be able to end a session
// it is not in, which is why the id comes from the verified signature rather
// than from the request body.
//
// Without this, a phone that stopped sharing left its peer-offer session alive
// until the transport timed out, and any offer made in that window came back
// SESSION_BUSY.
func (m *SessionManager) HandlePeerStop(peerDeviceID, reason string, code Code) error {
	m.mu.RLock()
	sess := m.activeSess
	inbound := m.inboundSess
	m.mu.RUnlock()

	var err error
	if sess != nil && peerDeviceID != "" && sess.TargetDeviceID() == peerDeviceID {
		// Stop is idempotent (stopOnce), so a session that is already winding
		// down reports nil rather than a double-teardown error.
		if e := sess.Stop(reason); e != nil {
			err = e
		}
	}

	if inbound != nil && (peerDeviceID == "" || inbound.cfg.PeerDeviceID == peerDeviceID) {
		if e := inbound.Close(); e != nil && err == nil {
			err = e
		}
		m.mu.Lock()
		if m.inboundSess == inbound {
			m.inboundSess = nil
		}
		m.mu.Unlock()
	}
	return err
}
