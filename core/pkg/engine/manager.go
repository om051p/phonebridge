package engine

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/om051p/phonebridge/core/pkg/crypto"
	"github.com/om051p/phonebridge/core/pkg/discovery"
	"github.com/om051p/phonebridge/core/pkg/receiver"
)

// SessionEvent models an event emitted by SessionManager.
type SessionEvent struct {
	SessionID    string
	State        SessionState
	Reason       string
	ErrorMessage string
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
	mu              sync.RWMutex
	cfg             SessionConfig
	discovery       *discovery.Discovery
	activeSess      *Session
	onEvent         func(SessionEvent)
	sink            receiver.FrameSink
	sinkFactory     func() (receiver.FrameSink, error)
	identity        *crypto.DeviceIdentity
	trustStore      *crypto.TrustStore
	pendingPairings map[string]*pendingPairing
	httpClient      *http.Client
}

// NewSessionManager creates a new session coordinator.
func NewSessionManager(cfg SessionConfig, disc *discovery.Discovery, sink receiver.FrameSink, onEvent func(SessionEvent)) *SessionManager {
	return &SessionManager{
		cfg:             cfg,
		discovery:       disc,
		sink:            sink,
		onEvent:         onEvent,
		identity:        cfg.Identity,
		trustStore:      cfg.TrustStore,
		pendingPairings: make(map[string]*pendingPairing),
		httpClient:      &http.Client{Timeout: 10 * time.Second},
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

// StartSession initiates a session targeting the given device ID.
func (m *SessionManager) StartSession(ctx context.Context, deviceID string) (*Session, error) {
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

	reg := m.discovery.Registry()
	sess := NewSession(sessionID, cfg, reg, func(oldState, newState SessionState, reason string) {
		if m.onEvent != nil {
			errMsg := ""
			if newState == StateFailed {
				errMsg = reason
			}
			m.onEvent(SessionEvent{
				SessionID:    sessionID,
				State:        newState,
				Reason:       reason,
				ErrorMessage: errMsg,
			})
		}
	})

	m.activeSess = sess

	// Asynchronously locate and connect to target device
	go func() {
		var sink receiver.FrameSink
		m.mu.RLock()
		factory := m.sinkFactory
		staticSink := m.sink
		m.mu.RUnlock()

		if factory != nil {
			if s, err := factory(); err == nil {
				sink = s
			}
		}
		if sink == nil && staticSink != nil {
			sink = staticSink
		}
		if sink == nil && (os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "") {
			if ds, err := receiver.NewDisplaySink("PhoneBridge Screen Mirror", true); err == nil {
				sink = ds
			}
		}
		if sink == nil {
			sink = receiver.NewNullSink()
		}
		if err := sess.LocateAndConnect(ctx, sink); err != nil {
			// LocateAndConnect already transitions to StateFailed on error
		}
	}()

	return sess, nil
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
	host := "127.0.0.1"
	if len(dev.Addresses) > 0 {
		host = dev.Addresses[0].String()
	}
	return fmt.Sprintf("%s:%d", host, port), nil
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
