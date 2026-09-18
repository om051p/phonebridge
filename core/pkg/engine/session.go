package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/om051p/phonebridge/core/pkg/crypto"
	"github.com/om051p/phonebridge/core/pkg/discovery"
	"github.com/om051p/phonebridge/core/pkg/receiver"
	"github.com/om051p/phonebridge/core/pkg/rtpmedia"
	pion "github.com/pion/webrtc/v4"
)

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

// SessionConfig configures a session request.
type SessionConfig struct {
	TargetDeviceID   string
	PreferredWidth   int
	PreferredHeight  int
	PreferredFPS     int
	DiscoveryTimeout time.Duration
	ConnectTimeout   time.Duration
	ReconnectTimeout time.Duration
	Identity         *crypto.DeviceIdentity
	TrustStore       *crypto.TrustStore
}

// DefaultSessionConfig returns production defaults for session configuration.
func DefaultSessionConfig() SessionConfig {
	return SessionConfig{
		PreferredWidth:   720,
		PreferredHeight:  1600,
		PreferredFPS:     30,
		DiscoveryTimeout: 10 * time.Second,
		ConnectTimeout:   10 * time.Second,
		ReconnectTimeout: 15 * time.Second,
	}
}

// SessionSnapshot provides a point-in-time view of session state and metrics.
type SessionSnapshot struct {
	SessionID         string
	State             SessionState
	TargetDevice      discovery.Device
	StartTime         time.Time
	ConnectedDuration time.Duration
	ErrorMessage      string
	Stats             rtpmedia.StreamStats
	DroppedAUs        int64
}

// StateChangeCallback receives state change events.
type StateChangeCallback func(oldState, newState SessionState, reason string)

// Session manages a single device session lifecycle.
type Session struct {
	mu           sync.RWMutex
	sessionID    string
	state        SessionState
	cfg          SessionConfig
	targetDevice discovery.Device
	startTime    time.Time
	lastError    error

	registry *discovery.DeviceRegistry
	receiver *receiver.Receiver

	targetEndpoint string
	signaling      *SignalingClient
	trustStore     *crypto.TrustStore
	sink           receiver.FrameSink

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
		sessionID:     sessionID,
		state:         StateDisconnected,
		cfg:           cfg,
		registry:      reg,
		trustStore:    cfg.TrustStore,
		signaling:     sigClient,
		onStateChange: cb,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// SetTrustStore sets the trust store for checking device peer trust.
func (s *Session) SetTrustStore(ts *crypto.TrustStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trustStore = ts
}

// Transition validates and applies a state transition.
func (s *Session) Transition(next SessionState, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == next {
		return nil
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
		return fmt.Errorf("invalid state transition: %s -> %s (reason: %s)", s.state, next, reason)
	}

	old := s.state
	s.state = next
	if next == StateConnected && s.startTime.IsZero() {
		s.startTime = time.Now()
	}

	if s.onStateChange != nil {
		s.onStateChange(old, next, reason)
	}
	return nil
}

// SessionID returns the unique session identifier.
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
	if s.receiver != nil {
		stats, dropped = s.receiver.Stats()
	}

	errMsg := ""
	if s.lastError != nil {
		errMsg = s.lastError.Error()
	}

	return SessionSnapshot{
		SessionID:         s.sessionID,
		State:             s.state,
		TargetDevice:      s.targetDevice,
		StartTime:         s.startTime,
		ConnectedDuration: duration,
		ErrorMessage:      errMsg,
		Stats:             stats,
		DroppedAUs:        dropped,
	}
}

// SetTargetDevice associates the resolved device with the session.
func (s *Session) SetTargetDevice(dev discovery.Device) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.targetDevice = dev
}

// SetReceiver attaches the active WebRTC receiver.
func (s *Session) SetReceiver(r *receiver.Receiver) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.receiver = r
}

// Fail transitions the session to StateFailed with an error cause.
func (s *Session) Fail(err error) {
	s.mu.Lock()
	s.lastError = err
	s.mu.Unlock()
	_ = s.Transition(StateFailed, err.Error())
}

// SetSignalingClient overrides the signaling client (e.g. for testing).
func (s *Session) SetSignalingClient(sig *SignalingClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.signaling = sig
}

// Stop cleanly terminates the session.
func (s *Session) Stop(reason string) error {
	s.cancel()

	s.mu.Lock()
	r := s.receiver
	s.receiver = nil
	ep := s.targetEndpoint
	sig := s.signaling
	s.mu.Unlock()

	if ep != "" && sig != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = sig.StopSession(ctx, ep, reason)
		cancel()
	}

	if r != nil {
		_ = r.Close()
	}

	return s.Transition(StateStopped, reason)
}

// Connect negotiates WebRTC via HTTP signaling with the device endpoint and binds the receiver.
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
		s.Fail(err)
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
	sig := s.signaling
	if sig == nil {
		sig = NewSignalingClient(s.cfg.ConnectTimeout)
		s.signaling = sig
	}
	s.mu.Unlock()

	recv, err := receiver.NewReceiver(receiver.Config{
		Sink:            sink,
		IncludeLoopback: true,
		OnStateChange: func(st pion.PeerConnectionState) {
			switch st {
			case pion.PeerConnectionStateConnected:
				_ = s.Transition(StateConnected, "WebRTC connected")
			case pion.PeerConnectionStateDisconnected:
				_ = s.Transition(StateReconnecting, "WebRTC disconnected")
			case pion.PeerConnectionStateFailed:
				s.Fail(fmt.Errorf("WebRTC connection failed"))
			case pion.PeerConnectionStateClosed:
				_ = s.Transition(StateStopped, "WebRTC closed")
			}
		},
	})
	if err != nil {
		s.Fail(err)
		return fmt.Errorf("create receiver: %w", err)
	}
	s.SetReceiver(recv)

	// 1. Request SDP offer from device
	offer, err := sig.RequestOffer(ctx, endpoint)
	if err != nil {
		s.Fail(err)
		_ = recv.Close()
		return fmt.Errorf("request offer from %s: %w", endpoint, err)
	}

	// 2. Set remote offer & generate local answer
	answer, err := recv.SetRemoteOffer(offer)
	if err != nil {
		s.Fail(err)
		_ = recv.Close()
		return fmt.Errorf("set remote offer: %w", err)
	}

	// 3. Send SDP answer back to device
	if err := sig.SendAnswer(ctx, endpoint, answer); err != nil {
		s.Fail(err)
		_ = recv.Close()
		return fmt.Errorf("send answer to %s: %w", endpoint, err)
	}

	// 4. Background monitor for remote track arrival -> StateStreaming
	go func() {
		if err := recv.WaitForTrack(s.cfg.ConnectTimeout); err == nil {
			_ = s.Transition(StateStreaming, "media track active")
		}
	}()

	return nil
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

	var host string
	if len(dev.Addresses) > 0 {
		host = dev.Addresses[0].String()
	} else {
		host = "127.0.0.1"
	}

	endpoint := fmt.Sprintf("%s:%d", host, port)
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
			s.Fail(err)
			return discovery.Device{}, err
		case <-timeout:
			err := fmt.Errorf("device %s not discovered within %v", s.cfg.TargetDeviceID, s.cfg.DiscoveryTimeout)
			s.Fail(err)
			return discovery.Device{}, err
		case <-ticker.C:
			if dev, ok := s.registry.Get(s.cfg.TargetDeviceID); ok && !dev.IsStale {
				s.SetTargetDevice(dev)
				return dev, nil
			}
		}
	}
}
