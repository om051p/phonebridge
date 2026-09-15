// Package spikegrpc — Spike 01 (EXPERIMENTAL, throwaway).
//
// The spike daemon: a minimal gRPC service on top of a Unix domain socket, with
// two gates in front of it (SO_PEERCRED uid check + optional bearer token).
// Nothing here is product code; the shapes exist to measure feasibility,
// latency, lifecycle, and the trust boundary.

package spikegrpc

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/om051p/phonebridge/spikes/localipc-spike/internal/spikepb"
)

// Config configures the spike daemon.
type Config struct {
	// SocketPath is the UDS the daemon binds (product proposal:
	// $XDG_RUNTIME_DIR/phonebridge/engine.sock).
	SocketPath string

	// Token, when non-empty, is required as `authorization: Bearer <token>`
	// on every RPC. Empty disables the gate (measured as a separate case).
	Token string

	// ExpectUID is the only peer uid accepted. Normally os.Geteuid().
	ExpectUID uint32

	// ExpectUIDSet distinguishes "expect uid 0 (root)" from "unset".
	ExpectUIDSet bool

	// Version is reported by Ping for identity/reconnect checks.
	Version string

	// MaxEchoBytes bounds the payload a single Ping may echo back.
	MaxEchoBytes int

	// MaxRecvBytes is the gRPC max receive size (gRPC default is 4 MiB).
	MaxRecvBytes int

	// Announce prints machine-readable lifecycle lines on stdout.
	Announce bool

	// Logf receives human-readable diagnostics (stderr in the CLI).
	Logf func(format string, args ...any)
}

// Stats are counters used as leak/behaviour evidence.
type Stats struct {
	Pings        atomic.Uint64
	Subscribes   atomic.Uint64
	EventsSent   atomic.Uint64
	Rejected     atomic.Uint64
	ActiveStream atomic.Int64
	MaxStreams   atomic.Int64
}

// Server implements the spike service and owns the socket lifecycle.
type Server struct {
	spikepb.UnimplementedLocalIpcSpikeServiceServer

	cfg Config
	st  Stats
}

// New returns a server with defaults applied.
func New(cfg Config) *Server {
	if cfg.MaxEchoBytes == 0 {
		cfg.MaxEchoBytes = 64 * 1024
	}
	if cfg.MaxRecvBytes == 0 {
		cfg.MaxRecvBytes = 4 * 1024 * 1024
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	return &Server{cfg: cfg}
}

// StatsSnapshot is a plain-value copy of Stats.
//
// Stats itself must never be copied (it embeds atomics); the daemon reports this
// snapshot on exit so the harness can assert behaviour after a graceful stop.
type StatsSnapshot struct {
	Pings        uint64
	Subscribes   uint64
	EventsSent   uint64
	Rejected     uint64
	ActiveStream int64
	MaxStreams   int64
}

// Snapshot exposes the counters (used by the CLI's exit report).
func (s *Server) Snapshot() StatsSnapshot {
	return StatsSnapshot{
		Pings:        s.st.Pings.Load(),
		Subscribes:   s.st.Subscribes.Load(),
		EventsSent:   s.st.EventsSent.Load(),
		Rejected:     s.st.Rejected.Load(),
		ActiveStream: s.st.ActiveStream.Load(),
		MaxStreams:   s.st.MaxStreams.Load(),
	}
}

// Serve binds the socket and serves until ctx is cancelled.
//
// Lifecycle contract exercised by the harness:
//  1. parent dir created 0700 (tightened even if it pre-existed),
//  2. a live socket is never clobbered (a second daemon exits with an error),
//  3. a *stale* socket file (owner died) is detected by dialling it, then removed,
//  4. socket created under a restrictive umask and chmod'ed 0600,
//  5. SIGTERM/ctx cancel -> GracefulStop, then the socket file is removed.
func (s *Server) Serve(ctx context.Context) error {
	dir := filepath.Dir(s.cfg.SocketPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create socket dir: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("tighten socket dir: %w", err)
	}

	if err := s.clearStaleSocket(); err != nil {
		return err
	}

	// Restrictive umask so the socket can never be observed world-accessible,
	// even in the instant before the explicit chmod below.
	old := syscall.Umask(0o177)
	ln, err := net.Listen("unix", s.cfg.SocketPath)
	syscall.Umask(old)
	if err != nil {
		return fmt.Errorf("listen unix %s: %w", s.cfg.SocketPath, err)
	}
	defer ln.Close()
	defer os.Remove(s.cfg.SocketPath)

	if err := os.Chmod(s.cfg.SocketPath, 0o600); err != nil {
		return fmt.Errorf("chmod socket: %w", err)
	}

	srv := grpc.NewServer(
		grpc.Creds(UnixPeerCreds{}),
		grpc.ChainUnaryInterceptor(s.unaryGate),
		grpc.StreamInterceptor(s.streamGate),
		grpc.MaxRecvMsgSize(s.cfg.MaxRecvBytes),
	)
	spikepb.RegisterLocalIpcSpikeServiceServer(srv, s)

	if s.cfg.Announce {
		fmt.Printf("READY socket=%s mode=%s server_uid=%d expect_uid=%d token=%s version=%s\n",
			s.cfg.SocketPath, socketModeString(s.cfg.SocketPath), os.Geteuid(),
			s.cfg.ExpectUID, tokenState(s.cfg.Token), s.cfg.Version)
		_ = os.Stdout.Sync()
	}

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-ctx.Done()
		done := make(chan struct{})
		go func() {
			srv.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			// A wedged stream must not block shutdown of a user service.
			s.cfg.Logf("graceful stop timed out; forcing stop")
			srv.Stop()
		}
	}()

	serveErr := srv.Serve(ln)
	<-stopped

	if s.cfg.Announce {
		fmt.Printf("STATS pings=%d subscribes=%d events=%d rejected=%d max_streams=%d\n",
			s.st.Pings.Load(), s.st.Subscribes.Load(), s.st.EventsSent.Load(),
			s.st.Rejected.Load(), s.st.MaxStreams.Load())
		fmt.Println("STOPPED")
		_ = os.Stdout.Sync()
	}
	if serveErr != nil && !errors.Is(serveErr, grpc.ErrServerStopped) {
		return serveErr
	}
	return nil
}

// clearStaleSocket refuses to clobber a live daemon but removes a dead socket
// file left behind by SIGKILL or a crash.
//
// Without this, a crash leaves a socket path that makes every later bind fail
// with EADDRINUSE — the classic local-IPC lifecycle bug. Note the product
// systemd unit already sets Restart=on-failure, so this path is exercised in
// normal operation.
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
		return fmt.Errorf("another daemon is already listening on %s", s.cfg.SocketPath)
	}
	s.cfg.Logf("removing stale socket %s (%v)", s.cfg.SocketPath, err)
	if err := os.Remove(s.cfg.SocketPath); err != nil {
		return fmt.Errorf("remove stale socket: %w", err)
	}
	return nil
}

// authorize applies the peer-uid and token gates to one call.
//
// Order matters: the kernel-reported identity is checked first, so a foreign
// uid is rejected without the token ever being compared.
func (s *Server) authorize(ctx context.Context) error {
	info, ok := PeerInfoFromContext(ctx)
	if !ok {
		// Cannot happen with UnixPeerCreds; fail closed if it ever does.
		s.st.Rejected.Add(1)
		return status.Error(codes.Internal, "peer credentials unavailable")
	}
	if s.cfg.ExpectUIDSet && info.UID != s.cfg.ExpectUID {
		s.st.Rejected.Add(1)
		return status.Errorf(codes.PermissionDenied,
			"peer uid %d is not permitted (expected %d)", info.UID, s.cfg.ExpectUID)
	}
	if s.cfg.Token != "" {
		md, _ := metadata.FromIncomingContext(ctx)
		var got string
		if vs := md.Get("authorization"); len(vs) > 0 {
			got = vs[0]
		}
		want := "Bearer " + s.cfg.Token
		// Constant-time compare: a token is a secret even on a UDS.
		if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			s.st.Rejected.Add(1)
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

// Ping implements the unary round trip used for the latency measurement.
func (s *Server) Ping(_ context.Context, req *spikepb.PingRequest) (*spikepb.PingResponse, error) {
	s.st.Pings.Add(1)
	payload := req.GetPayload()
	if len(payload) > s.cfg.MaxEchoBytes {
		payload = payload[:s.cfg.MaxEchoBytes]
	}
	return &spikepb.PingResponse{
		Nonce:               req.GetNonce(),
		ServerVersion:       s.cfg.Version,
		Payload:             payload,
		ServerRecvUnixNanos: uint64(time.Now().UnixNano()),
	}, nil
}

// Subscribe implements the Go -> Flutter push path as a server stream.
func (s *Server) Subscribe(req *spikepb.SubscribeRequest, stream grpc.ServerStreamingServer[spikepb.SubscribeResponse]) error {
	s.st.Subscribes.Add(1)
	active := s.st.ActiveStream.Add(1)
	if active > s.st.MaxStreams.Load() { // approximate high-water mark
		s.st.MaxStreams.Store(active)
	}
	defer s.st.ActiveStream.Add(-1)

	interval := time.Duration(req.GetIntervalMs()) * time.Millisecond
	payload := make([]byte, req.GetPayloadSize())
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	for i := uint32(1); i <= req.GetCount(); i++ {
		if err := stream.Send(&spikepb.SubscribeResponse{
			Seq:              uint64(i),
			Kind:             "tick",
			EmittedUnixNanos: uint64(time.Now().UnixNano()),
			Payload:          payload,
		}); err != nil {
			return err
		}
		s.st.EventsSent.Add(1)
		if i == req.GetCount() || interval == 0 {
			continue
		}
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case <-time.After(interval):
		}
	}
	return nil
}

// WhoAmI reports the server's kernel-verified view of the caller.
func (s *Server) WhoAmI(ctx context.Context, _ *spikepb.WhoAmIRequest) (*spikepb.WhoAmIResponse, error) {
	info, ok := PeerInfoFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Internal, "peer credentials unavailable")
	}
	return &spikepb.WhoAmIResponse{
		PeerUid:       info.UID,
		PeerPid:       info.PID,
		PeerGid:       info.GID,
		SocketPath:    s.cfg.SocketPath,
		ServerUid:     uint32(os.Geteuid()),
		TokenRequired: s.cfg.Token != "",
	}, nil
}

func tokenState(token string) string {
	if token == "" {
		return "none"
	}
	return "required"
}

func socketModeString(path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return "unknown"
	}
	return fmt.Sprintf("%#o", fi.Mode().Perm())
}
