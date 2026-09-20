// Package localipc provides the production UDS + gRPC engine service (DEC-018)
// implementing phonebridge.localipc.v1.
package localipc

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/om051p/phonebridge/core/pkg/crypto"
	"github.com/om051p/phonebridge/core/pkg/discovery"
	"github.com/om051p/phonebridge/core/pkg/engine"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgelocalipcv1"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

// Default paths per DEC-018.
func DefaultSocketPath() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "phonebridge", "engine.sock")
	}
	uid := os.Geteuid()
	runUser := fmt.Sprintf("/run/user/%d", uid)
	if fi, err := os.Stat(runUser); err == nil && fi.IsDir() {
		return filepath.Join(runUser, "phonebridge", "engine.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("phonebridge-%d", uid), "engine.sock")
}

func DefaultTokenPath() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "phonebridge", "token")
	}
	uid := os.Geteuid()
	runUser := fmt.Sprintf("/run/user/%d", uid)
	if fi, err := os.Stat(runUser); err == nil && fi.IsDir() {
		return filepath.Join(runUser, "phonebridge", "token")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("phonebridge-%d", uid), "token")
}

// GenerateToken generates >= 256 bits of CSPRNG randomness encoded as hex.
func GenerateToken() (string, error) {
	b := make([]byte, 32) // 256 bits
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// SessionOrchestrator manages device discovery and LAN session lifecycle for LocalEngineService.
type SessionOrchestrator interface {
	// StartSession asks the capture device for a session. requested carries the
	// media tuple the caller wants; zero fields mean "use the engine defaults".
	StartSession(ctx context.Context, deviceID string, requested engine.MediaParams) (*engine.Session, error)
	StopSession(sessionID, reason string) error
	GetSessionState(sessionID string) (engine.SessionSnapshot, error)
	ListDevices() []discovery.Device
	PairDevice(ctx context.Context, deviceID string) (string, string, error)
	ConfirmPairing(ctx context.Context, deviceID string, confirmed bool) error
	ListTrustedDevices() []crypto.TrustEntry
	RevokeDevice(deviceID string) error
	GetClipboardStatus(ctx context.Context) (*phonebridgelocalipcv1.GetClipboardStatusResponse, error)
	TriggerClipboardPull(ctx context.Context) error
}

// Config configures the production local IPC server.
type Config struct {
	SocketPath       string
	TokenPath        string
	Token            string
	ExpectUID        uint32
	ExpectUIDSet     bool
	ServerVersion    string
	DaemonGeneration uint64
	MaxRecvBytes     int
	Orchestrator     SessionOrchestrator
	Logf             func(format string, args ...any)
}

// Server implements phonebridgelocalipcv1.LocalEngineServiceServer.
type Server struct {
	phonebridgelocalipcv1.UnimplementedLocalEngineServiceServer

	cfg          Config
	orchestrator SessionOrchestrator
	startTime    time.Time

	grpcServer *grpc.Server
	listener   net.Listener

	ready     chan struct{}
	readyOnce sync.Once

	subscribersMu sync.RWMutex
	subscribers   map[chan *eventPayload]struct{}

	closed atomic.Bool
}

type eventPayload struct {
	envelope       *phonebridgev1.Envelope
	sessionEvent   *phonebridgelocalipcv1.SessionEvent
	clipboardEvent *phonebridgelocalipcv1.ClipboardStatusEvent
}

// NewServer creates a new local IPC server with sensible defaults.
func NewServer(cfg Config) (*Server, error) {
	if cfg.SocketPath == "" {
		cfg.SocketPath = DefaultSocketPath()
	}
	if cfg.TokenPath == "" {
		cfg.TokenPath = DefaultTokenPath()
	}
	if cfg.Token == "" {
		tok, err := GenerateToken()
		if err != nil {
			return nil, err
		}
		cfg.Token = tok
	}
	if !cfg.ExpectUIDSet {
		cfg.ExpectUID = uint32(os.Geteuid())
		cfg.ExpectUIDSet = true
	}
	if cfg.ServerVersion == "" {
		cfg.ServerVersion = "0.1.0-prod"
	}
	if cfg.DaemonGeneration == 0 {
		cfg.DaemonGeneration = uint64(time.Now().UnixNano())
	}
	if cfg.MaxRecvBytes == 0 {
		cfg.MaxRecvBytes = 4 * 1024 * 1024 // 4 MiB gRPC ceiling
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}

	return &Server{
		cfg:          cfg,
		orchestrator: cfg.Orchestrator,
		ready:        make(chan struct{}),
		subscribers:  make(map[chan *eventPayload]struct{}),
	}, nil
}

// SetOrchestrator sets or updates the active session orchestrator.
func (s *Server) SetOrchestrator(orch SessionOrchestrator) {
	s.orchestrator = orch
}

// Ready returns a channel that is closed when the server has successfully bound and is listening.
func (s *Server) Ready() <-chan struct{} {
	return s.ready
}

// Config returns the active configuration.
func (s *Server) Config() Config {
	return s.cfg
}

// Token returns the active bearer token.
func (s *Server) Token() string {
	return s.cfg.Token
}

// Serve initializes the socket, writes the token file, and serves gRPC until ctx is done.
func (s *Server) Serve(ctx context.Context) error {
	s.startTime = time.Now()

	// 1. Prepare directory with 0700 mode (DEC-018)
	socketDir := filepath.Dir(s.cfg.SocketPath)
	if err := os.MkdirAll(socketDir, 0o700); err != nil {
		return fmt.Errorf("mkdir socket dir %s: %w", socketDir, err)
	}
	if err := os.Chmod(socketDir, 0o700); err != nil {
		return fmt.Errorf("chmod socket dir %s: %w", socketDir, err)
	}

	tokenDir := filepath.Dir(s.cfg.TokenPath)
	if tokenDir != socketDir {
		if err := os.MkdirAll(tokenDir, 0o700); err != nil {
			return fmt.Errorf("mkdir token dir %s: %w", tokenDir, err)
		}
		if err := os.Chmod(tokenDir, 0o700); err != nil {
			return fmt.Errorf("chmod token dir %s: %w", tokenDir, err)
		}
	}

	// 2. Write bearer token file with mode 0600
	if err := os.WriteFile(s.cfg.TokenPath, []byte(s.cfg.Token+"\n"), 0o600); err != nil {
		return fmt.Errorf("write token file %s: %w", s.cfg.TokenPath, err)
	}
	_ = os.Chmod(s.cfg.TokenPath, 0o600)
	defer os.Remove(s.cfg.TokenPath)

	// 3. Stale socket check & cleanup
	if err := s.clearStaleSocket(); err != nil {
		return err
	}

	// 4. Bind socket under restrictive umask 0177, then chmod 0600
	oldUmask := syscall.Umask(0o177)
	ln, err := net.Listen("unix", s.cfg.SocketPath)
	syscall.Umask(oldUmask)
	if err != nil {
		return fmt.Errorf("listen unix %s: %w", s.cfg.SocketPath, err)
	}
	s.listener = ln
	defer ln.Close()
	defer os.Remove(s.cfg.SocketPath)

	if err := os.Chmod(s.cfg.SocketPath, 0o600); err != nil {
		return fmt.Errorf("chmod socket %s: %w", s.cfg.SocketPath, err)
	}

	// 5. Build gRPC server with credentials & interceptors
	srv := grpc.NewServer(
		grpc.Creds(UnixPeerCreds{}),
		grpc.ChainUnaryInterceptor(s.unaryGate),
		grpc.StreamInterceptor(s.streamGate),
		grpc.MaxRecvMsgSize(s.cfg.MaxRecvBytes),
	)
	s.grpcServer = srv
	phonebridgelocalipcv1.RegisterLocalEngineServiceServer(srv, s)

	// 6. Monitor cancellation for graceful shutdown
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-ctx.Done()
		s.closed.Store(true)
		s.closeAllSubscribers()

		done := make(chan struct{})
		go func() {
			srv.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			s.cfg.Logf("graceful stop timed out; forcing stop")
			srv.Stop()
		}
	}()

	s.cfg.Logf("serving localipc on %s (token: %s, uid: %d)", s.cfg.SocketPath, s.cfg.TokenPath, s.cfg.ExpectUID)
	s.readyOnce.Do(func() { close(s.ready) })
	serveErr := srv.Serve(ln)
	<-stopped

	if serveErr != nil && !errors.Is(serveErr, grpc.ErrServerStopped) {
		return serveErr
	}
	return nil
}

// clearStaleSocket tests whether an existing socket has an active listener.
func (s *Server) clearStaleSocket() error {
	if _, err := os.Lstat(s.cfg.SocketPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	conn, err := net.DialTimeout("unix", s.cfg.SocketPath, 250*time.Millisecond)
	if err == nil {
		conn.Close()
		return fmt.Errorf("active daemon already listening on %s", s.cfg.SocketPath)
	}
	s.cfg.Logf("removing stale socket %s (%v)", s.cfg.SocketPath, err)
	if err := os.Remove(s.cfg.SocketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket: %w", err)
	}
	return nil
}

// authorize enforces Gate 1 (SO_PEERCRED UID) and Gate 2 (Bearer token).
func (s *Server) authorize(ctx context.Context) error {
	// Gate 1: Kernel-verified SO_PEERCRED UID check
	info, ok := PeerInfoFromContext(ctx)
	if !ok {
		return status.Error(codes.Internal, "peer credentials unavailable")
	}
	if s.cfg.ExpectUIDSet && info.UID != s.cfg.ExpectUID {
		return status.Errorf(codes.PermissionDenied,
			"peer uid %d is not permitted (expected %d)", info.UID, s.cfg.ExpectUID)
	}

	// Gate 2: Constant-time Bearer token check
	if s.cfg.Token != "" {
		md, _ := metadata.FromIncomingContext(ctx)
		var got string
		if vs := md.Get("authorization"); len(vs) > 0 {
			got = vs[0]
		}
		want := "Bearer " + s.cfg.Token
		if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			return status.Error(codes.Unauthenticated, "missing or invalid bearer token")
		}
	}
	return nil
}

func (s *Server) unaryGate(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

func (s *Server) streamGate(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if err := s.authorize(ss.Context()); err != nil {
		return err
	}
	return handler(srv, ss)
}

// Handshake negotiates protocol version and returns daemon generation.
func (s *Server) Handshake(_ context.Context, req *phonebridgelocalipcv1.HandshakeRequest) (*phonebridgelocalipcv1.HandshakeResponse, error) {
	clientVer := req.GetClientVersion()
	negotiated := uint32(1)
	if clientVer < 1 {
		negotiated = 0
	} else if clientVer < negotiated {
		negotiated = clientVer
	}

	return &phonebridgelocalipcv1.HandshakeResponse{
		NegotiatedVersion: negotiated,
		ServerVersion:     s.cfg.ServerVersion,
		DaemonGeneration:  s.cfg.DaemonGeneration,
	}, nil
}

// Ping returns a caller nonce and server identity.
func (s *Server) Ping(_ context.Context, req *phonebridgelocalipcv1.PingRequest) (*phonebridgelocalipcv1.PingResponse, error) {
	return &phonebridgelocalipcv1.PingResponse{
		Nonce:         req.GetNonce(),
		ServerVersion: s.cfg.ServerVersion,
	}, nil
}

// Health reports daemon readiness, uptime, and generation.
func (s *Server) Health(_ context.Context, _ *phonebridgelocalipcv1.HealthRequest) (*phonebridgelocalipcv1.HealthResponse, error) {
	uptime := uint64(0)
	if !s.startTime.IsZero() {
		uptime = uint64(time.Since(s.startTime).Milliseconds())
	}
	return &phonebridgelocalipcv1.HealthResponse{
		Ready:            !s.closed.Load(),
		DaemonGeneration: s.cfg.DaemonGeneration,
		ServerVersion:    s.cfg.ServerVersion,
		UptimeMs:         uptime,
	}, nil
}

// StreamEvents pushes relayed device-protocol messages and session events to the UI.
func (s *Server) StreamEvents(_ *phonebridgelocalipcv1.StreamEventsRequest, stream grpc.ServerStreamingServer[phonebridgelocalipcv1.StreamEventsResponse]) error {
	if s.closed.Load() {
		return status.Error(codes.Unavailable, "daemon is shutting down")
	}

	ch := make(chan *eventPayload, 64)
	s.subscribersMu.Lock()
	s.subscribers[ch] = struct{}{}
	s.subscribersMu.Unlock()

	defer func() {
		s.subscribersMu.Lock()
		delete(s.subscribers, ch)
		s.subscribersMu.Unlock()
	}()

	ctx := stream.Context()
	var seq uint64 = 1

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case item, ok := <-ch:
			if !ok {
				return nil
			}
			resp := &phonebridgelocalipcv1.StreamEventsResponse{
				Seq:              seq,
				DaemonGeneration: s.cfg.DaemonGeneration,
				Envelope:         item.envelope,
				SessionEvent:     item.sessionEvent,
				ClipboardEvent:   item.clipboardEvent,
			}
			if err := stream.Send(resp); err != nil {
				return err
			}
			seq++
		}
	}
}

func (s *Server) broadcastItem(item *eventPayload) {
	if s.closed.Load() || item == nil {
		return
	}
	s.subscribersMu.RLock()
	defer s.subscribersMu.RUnlock()

	for ch := range s.subscribers {
		select {
		case ch <- item:
		default:
			// Non-blocking drop on full subscriber buffer
			s.cfg.Logf("localipc: subscriber buffer full; dropping event")
		}
	}
}

// BroadcastEnvelope pushes an opaque phonebridge.v1.Envelope to all active StreamEvents streams.
func (s *Server) BroadcastEnvelope(env *phonebridgev1.Envelope) {
	if env == nil {
		return
	}
	s.broadcastItem(&eventPayload{envelope: env})
}

// BroadcastSessionEvent pushes a SessionEvent to all active StreamEvents streams.
func (s *Server) BroadcastSessionEvent(event *phonebridgelocalipcv1.SessionEvent) {
	if event == nil {
		return
	}
	s.broadcastItem(&eventPayload{sessionEvent: event})
}

// BroadcastClipboardEvent pushes a ClipboardStatusEvent to all active StreamEvents streams.
func (s *Server) BroadcastClipboardEvent(event *phonebridgelocalipcv1.ClipboardStatusEvent) {
	if event == nil {
		return
	}
	s.broadcastItem(&eventPayload{clipboardEvent: event})
}

// StartSession initiates a session targeting the given device ID.
func (s *Server) StartSession(ctx context.Context, req *phonebridgelocalipcv1.StartSessionRequest) (*phonebridgelocalipcv1.StartSessionResponse, error) {
	if s.closed.Load() {
		return nil, status.Error(codes.Unavailable, "daemon is shutting down")
	}
	if req.GetDeviceId() == "" {
		return nil, status.Error(codes.InvalidArgument, "device_id cannot be empty")
	}
	if s.orchestrator == nil {
		return nil, status.Error(codes.FailedPrecondition, "session orchestrator not configured")
	}

	sess, err := s.orchestrator.StartSession(ctx, req.GetDeviceId(), FromProtoMediaParams(req.GetRequested()))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "start session failed: %v", err)
	}

	return &phonebridgelocalipcv1.StartSessionResponse{
		SessionId: sess.SessionID(),
		State:     ToProtoSessionState(sess.State()),
	}, nil
}

// StopSession terminates an active session.
func (s *Server) StopSession(_ context.Context, req *phonebridgelocalipcv1.StopSessionRequest) (*phonebridgelocalipcv1.StopSessionResponse, error) {
	if s.closed.Load() {
		return nil, status.Error(codes.Unavailable, "daemon is shutting down")
	}
	if s.orchestrator == nil {
		return nil, status.Error(codes.FailedPrecondition, "session orchestrator not configured")
	}

	reason := req.GetReason()
	if reason == "" {
		reason = "user requested stop"
	}

	if err := s.orchestrator.StopSession(req.GetSessionId(), reason); err != nil {
		return nil, status.Errorf(codes.Internal, "stop session failed: %v", err)
	}

	return &phonebridgelocalipcv1.StopSessionResponse{
		SessionId: req.GetSessionId(),
		State:     phonebridgelocalipcv1.SessionState_SESSION_STATE_STOPPED,
	}, nil
}

// GetSessionState returns a snapshot of the current or specified session.
func (s *Server) GetSessionState(_ context.Context, req *phonebridgelocalipcv1.GetSessionStateRequest) (*phonebridgelocalipcv1.GetSessionStateResponse, error) {
	if s.closed.Load() {
		return nil, status.Error(codes.Unavailable, "daemon is shutting down")
	}
	if s.orchestrator == nil {
		return nil, status.Error(codes.FailedPrecondition, "session orchestrator not configured")
	}

	snap, err := s.orchestrator.GetSessionState(req.GetSessionId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "get session state failed: %v", err)
	}

	return &phonebridgelocalipcv1.GetSessionStateResponse{
		SessionId:           snap.SessionID,
		State:               ToProtoSessionState(snap.State),
		DeviceId:            snap.TargetDevice.ID,
		ConnectedDurationMs: uint64(snap.ConnectedDuration.Milliseconds()),
		ErrorMessage:        snap.ErrorMessage,
		ReasonCode:          ToProtoSessionReason(snap.ReasonCode),
		Requested:           ToProtoMediaParams(snap.Requested),
		// Actual is only reported when the device stated it: an absent tuple
		// tells the caller "not reported", which is different from a tuple that
		// happens to equal the request (DEC-022: no silent substitution).
		Actual:            ToProtoMediaParamsKnown(snap.Actual, snap.ActualKnown),
		ReconnectAttempts: uint32(snap.ReconnectAttempts),
		Stats: &phonebridgelocalipcv1.StreamStats{
			Packets:     uint64(snap.Stats.Packets),
			BytesRtp:    uint64(snap.Stats.BytesRTP),
			BytesH264:   uint64(snap.Stats.BytesH264),
			AccessUnits: uint64(snap.Stats.AccessUnits),
			Keyframes:   uint64(snap.Stats.Keyframes),
			DroppedAus:  snap.DroppedAUs,
		},
	}, nil
}

// ListDevices returns all currently discovered LAN devices from mDNS.
func (s *Server) ListDevices(_ context.Context, _ *phonebridgelocalipcv1.ListDevicesRequest) (*phonebridgelocalipcv1.ListDevicesResponse, error) {
	if s.closed.Load() {
		return nil, status.Error(codes.Unavailable, "daemon is shutting down")
	}
	if s.orchestrator == nil {
		return &phonebridgelocalipcv1.ListDevicesResponse{}, nil
	}

	devices := s.orchestrator.ListDevices()
	resp := &phonebridgelocalipcv1.ListDevicesResponse{
		Devices: make([]*phonebridgelocalipcv1.DiscoveredDevice, 0, len(devices)),
	}

	for _, d := range devices {
		addr := ""
		if a, ok := engine.BestDialAddr(d.Addresses); ok {
			addr = engine.DialHost(a)
		}
		resp.Devices = append(resp.Devices, &phonebridgelocalipcv1.DiscoveredDevice{
			Id:           d.ID,
			Name:         d.Name,
			Model:        d.Model,
			Version:      d.Version,
			Capabilities: d.Capabilities,
			State:        string(d.State),
			Address:      addr,
			Port:         uint32(d.Port),
			IsStale:      d.IsStale,
		})
	}

	return resp, nil
}

// PairDevice initiates pairing with a discovered LAN device.
func (s *Server) PairDevice(ctx context.Context, req *phonebridgelocalipcv1.PairDeviceRequest) (*phonebridgelocalipcv1.PairDeviceResponse, error) {
	if s.closed.Load() {
		return nil, status.Error(codes.Unavailable, "daemon is shutting down")
	}
	if s.orchestrator == nil {
		return nil, status.Error(codes.FailedPrecondition, "session orchestrator not configured")
	}

	name, sas, err := s.orchestrator.PairDevice(ctx, req.GetDeviceId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "pairing failed: %v", err)
	}

	return &phonebridgelocalipcv1.PairDeviceResponse{
		DeviceId:            req.GetDeviceId(),
		DisplayName:         name,
		Sas:                 sas,
		PendingConfirmation: true,
	}, nil
}

// ConfirmPairing confirms or rejects the SAS for a pending pairing.
func (s *Server) ConfirmPairing(ctx context.Context, req *phonebridgelocalipcv1.ConfirmPairingRequest) (*phonebridgelocalipcv1.ConfirmPairingResponse, error) {
	if s.closed.Load() {
		return nil, status.Error(codes.Unavailable, "daemon is shutting down")
	}
	if s.orchestrator == nil {
		return nil, status.Error(codes.FailedPrecondition, "session orchestrator not configured")
	}

	err := s.orchestrator.ConfirmPairing(ctx, req.GetDeviceId(), req.GetUserConfirmed())
	if err != nil {
		return &phonebridgelocalipcv1.ConfirmPairingResponse{
			DeviceId:     req.GetDeviceId(),
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &phonebridgelocalipcv1.ConfirmPairingResponse{
		DeviceId: req.GetDeviceId(),
		Success:  true,
	}, nil
}

// ListTrustedDevices returns all trusted devices from the trust store.
func (s *Server) ListTrustedDevices(_ context.Context, _ *phonebridgelocalipcv1.ListTrustedDevicesRequest) (*phonebridgelocalipcv1.ListTrustedDevicesResponse, error) {
	if s.closed.Load() {
		return nil, status.Error(codes.Unavailable, "daemon is shutting down")
	}
	if s.orchestrator == nil {
		return &phonebridgelocalipcv1.ListTrustedDevicesResponse{}, nil
	}

	entries := s.orchestrator.ListTrustedDevices()
	resp := &phonebridgelocalipcv1.ListTrustedDevicesResponse{
		Devices: make([]*phonebridgelocalipcv1.TrustedDevice, 0, len(entries)),
	}
	for _, e := range entries {
		resp.Devices = append(resp.Devices, &phonebridgelocalipcv1.TrustedDevice{
			DeviceId:    e.DeviceID,
			DisplayName: e.DisplayName,
			Platform:    e.Platform,
			PublicKey:   e.PublicKey,
			PairedAtMs:  e.PairedAt.UnixMilli(),
			LastSeenMs:  e.LastSeen.UnixMilli(),
			Revoked:     e.Revoked,
		})
	}
	return resp, nil
}

// RevokeDevice revokes trust for a paired device.
func (s *Server) RevokeDevice(_ context.Context, req *phonebridgelocalipcv1.RevokeDeviceRequest) (*phonebridgelocalipcv1.RevokeDeviceResponse, error) {
	if s.closed.Load() {
		return nil, status.Error(codes.Unavailable, "daemon is shutting down")
	}
	if s.orchestrator == nil {
		return nil, status.Error(codes.FailedPrecondition, "session orchestrator not configured")
	}

	if err := s.orchestrator.RevokeDevice(req.GetDeviceId()); err != nil {
		return nil, status.Errorf(codes.Internal, "revoke device failed: %v", err)
	}

	return &phonebridgelocalipcv1.RevokeDeviceResponse{
		DeviceId: req.GetDeviceId(),
		Success:  true,
	}, nil
}

// GetClipboardStatus returns the current clipboard engine and adapter state.
func (s *Server) GetClipboardStatus(ctx context.Context, _ *phonebridgelocalipcv1.GetClipboardStatusRequest) (*phonebridgelocalipcv1.GetClipboardStatusResponse, error) {
	if s.closed.Load() {
		return nil, status.Error(codes.Unavailable, "daemon is shutting down")
	}
	if s.orchestrator == nil {
		return nil, status.Error(codes.FailedPrecondition, "session orchestrator not configured")
	}
	return s.orchestrator.GetClipboardStatus(ctx)
}

// TriggerClipboardPull reads the host clipboard and synchronizes it to the active peer.
func (s *Server) TriggerClipboardPull(ctx context.Context, _ *phonebridgelocalipcv1.TriggerClipboardPullRequest) (*phonebridgelocalipcv1.TriggerClipboardPullResponse, error) {
	if s.closed.Load() {
		return nil, status.Error(codes.Unavailable, "daemon is shutting down")
	}
	if s.orchestrator == nil {
		return nil, status.Error(codes.FailedPrecondition, "session orchestrator not configured")
	}
	if err := s.orchestrator.TriggerClipboardPull(ctx); err != nil {
		return &phonebridgelocalipcv1.TriggerClipboardPullResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}
	return &phonebridgelocalipcv1.TriggerClipboardPullResponse{
		Success: true,
	}, nil
}

// ToProtoSessionReason converts an internal engine.SessionReason to the wire enum.
// The two taxonomies are defined field-for-field alike, so this is a mapping and
// not a translation: a new reason must be added to both.
func ToProtoSessionReason(r engine.SessionReason) phonebridgelocalipcv1.SessionReason {
	switch r {
	case engine.ReasonNone:
		return phonebridgelocalipcv1.SessionReason_SESSION_REASON_NONE
	case engine.ReasonProtocolVersionMismatch:
		return phonebridgelocalipcv1.SessionReason_SESSION_REASON_PROTOCOL_VERSION_MISMATCH
	case engine.ReasonUnsupportedMediaParams:
		return phonebridgelocalipcv1.SessionReason_SESSION_REASON_UNSUPPORTED_MEDIA_PARAMS
	case engine.ReasonDeviceNotTrusted:
		return phonebridgelocalipcv1.SessionReason_SESSION_REASON_DEVICE_NOT_TRUSTED
	case engine.ReasonDeviceNotFound:
		return phonebridgelocalipcv1.SessionReason_SESSION_REASON_DEVICE_NOT_FOUND
	case engine.ReasonSessionBusy:
		return phonebridgelocalipcv1.SessionReason_SESSION_REASON_SESSION_BUSY
	case engine.ReasonConsentRevoked:
		return phonebridgelocalipcv1.SessionReason_SESSION_REASON_CONSENT_REVOKED
	case engine.ReasonCaptureFailed:
		return phonebridgelocalipcv1.SessionReason_SESSION_REASON_CAPTURE_FAILED
	case engine.ReasonTransportFailed:
		return phonebridgelocalipcv1.SessionReason_SESSION_REASON_TRANSPORT_FAILED
	case engine.ReasonReconnectTimeout:
		return phonebridgelocalipcv1.SessionReason_SESSION_REASON_RECONNECT_TIMEOUT
	case engine.ReasonSignalingFailed:
		return phonebridgelocalipcv1.SessionReason_SESSION_REASON_SIGNALING_FAILED
	case engine.ReasonUserStopped:
		return phonebridgelocalipcv1.SessionReason_SESSION_REASON_USER_STOPPED
	default:
		return phonebridgelocalipcv1.SessionReason_SESSION_REASON_UNSPECIFIED
	}
}

// ToProtoMediaParams converts a negotiated media tuple. A zero field stays zero,
// which the contract reads as "no preference / not reported".
func ToProtoMediaParams(p engine.MediaParams) *phonebridgev1.MediaParams {
	return &phonebridgev1.MediaParams{
		Width:       uint32(max0(p.Width)),
		Height:      uint32(max0(p.Height)),
		Fps:         uint32(max0(p.FPS)),
		BitrateKbps: uint32(max0(p.BitrateKbps)),
		Codec:       p.Codec,
	}
}

// ToProtoMediaParamsKnown returns nil when the tuple is not known, so a caller
// cannot mistake "the device did not say" for "the device said nothing".
func ToProtoMediaParamsKnown(p engine.MediaParams, known bool) *phonebridgev1.MediaParams {
	if !known {
		return nil
	}
	return ToProtoMediaParams(p)
}

// FromProtoMediaParams converts a wire tuple into the engine representation.
func FromProtoMediaParams(p *phonebridgev1.MediaParams) engine.MediaParams {
	if p == nil {
		return engine.MediaParams{}
	}
	return engine.MediaParams{
		Width:       int(p.GetWidth()),
		Height:      int(p.GetHeight()),
		FPS:         int(p.GetFps()),
		BitrateKbps: int(p.GetBitrateKbps()),
		Codec:       p.GetCodec(),
	}
}

func max0(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

// ToProtoSessionState converts an internal engine.SessionState to the protobuf SessionState enum.
func ToProtoSessionState(st engine.SessionState) phonebridgelocalipcv1.SessionState {
	switch st {
	case engine.StateDisconnected:
		return phonebridgelocalipcv1.SessionState_SESSION_STATE_DISCONNECTED
	case engine.StateDiscovering:
		return phonebridgelocalipcv1.SessionState_SESSION_STATE_DISCOVERING
	case engine.StateConnecting:
		return phonebridgelocalipcv1.SessionState_SESSION_STATE_CONNECTING
	case engine.StateConnected:
		return phonebridgelocalipcv1.SessionState_SESSION_STATE_CONNECTED
	case engine.StateStreaming:
		return phonebridgelocalipcv1.SessionState_SESSION_STATE_STREAMING
	case engine.StateReconnecting:
		return phonebridgelocalipcv1.SessionState_SESSION_STATE_RECONNECTING
	case engine.StateStopped:
		return phonebridgelocalipcv1.SessionState_SESSION_STATE_STOPPED
	case engine.StateFailed:
		return phonebridgelocalipcv1.SessionState_SESSION_STATE_FAILED
	default:
		return phonebridgelocalipcv1.SessionState_SESSION_STATE_UNSPECIFIED
	}
}

func (s *Server) closeAllSubscribers() {
	s.subscribersMu.Lock()
	defer s.subscribersMu.Unlock()
	for ch := range s.subscribers {
		close(ch)
		delete(s.subscribers, ch)
	}
}
