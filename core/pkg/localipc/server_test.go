package localipc

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/om051p/phonebridge/core/pkg/crypto"
	"github.com/om051p/phonebridge/core/pkg/discovery"
	"github.com/om051p/phonebridge/core/pkg/engine"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgelocalipcv1"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"github.com/om051p/phonebridge/core/pkg/rtpmedia"
	"github.com/om051p/phonebridge/core/pkg/transfer"
)

func testSetup(t *testing.T) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	sock := filepath.Join(dir, "engine.sock")
	tok := filepath.Join(dir, "token")
	tokVal, err := GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return sock, tok, tokVal
}

func startTestServer(t *testing.T, cfg Config) (*Server, context.CancelFunc, <-chan error) {
	t.Helper()
	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- srv.Serve(ctx)
	}()

	// Wait until server signals Ready
	select {
	case <-srv.Ready():
		return srv, cancel, errCh
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatalf("timed out waiting for server to start on %s", cfg.SocketPath)
		return nil, nil, nil
	}
}

func TestLifecycleAndPermissions(t *testing.T) {
	sock, tok, tokVal := testSetup(t)
	sockDir := filepath.Dir(sock)

	cfg := Config{
		SocketPath:    sock,
		TokenPath:     tok,
		Token:         tokVal,
		ServerVersion: "0.1.0-test",
	}

	_, cancel, errCh := startTestServer(t, cfg)

	// Check directory permissions (0700)
	dirFi, err := os.Stat(sockDir)
	if err != nil {
		t.Fatalf("stat socket dir: %v", err)
	}
	if dirFi.Mode().Perm() != 0o700 {
		t.Errorf("socket dir perm = %#o, want 0700", dirFi.Mode().Perm())
	}

	// Check socket permissions (0600)
	sockFi, err := os.Stat(sock)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if sockFi.Mode().Perm() != 0o600 {
		t.Errorf("socket perm = %#o, want 0600", sockFi.Mode().Perm())
	}

	// Check token permissions (0600)
	tokFi, err := os.Stat(tok)
	if err != nil {
		t.Fatalf("stat token: %v", err)
	}
	if tokFi.Mode().Perm() != 0o600 {
		t.Errorf("token perm = %#o, want 0600", tokFi.Mode().Perm())
	}

	// Check token content
	readTok, err := ReadTokenFile(tok)
	if err != nil {
		t.Fatalf("ReadTokenFile: %v", err)
	}
	if readTok != tokVal {
		t.Errorf("token = %q, want %q", readTok, tokVal)
	}

	// Stop server
	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}

	// Verify socket and token are removed
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Errorf("socket file %s still exists after shutdown", sock)
	}
	if _, err := os.Stat(tok); !os.IsNotExist(err) {
		t.Errorf("token file %s still exists after shutdown", tok)
	}
}

func TestStaleSocketHandling(t *testing.T) {
	sock, tok, tokVal := testSetup(t)

	// 1. Create a dead/stale socket file (no listener)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ln.Close() // closed, but file remains

	// Starting server must detect dead socket, remove it, and bind successfully
	cfg := Config{
		SocketPath: sock,
		TokenPath:  tok,
		Token:      tokVal,
	}
	_, cancel, errCh := startTestServer(t, cfg)
	cancel()
	<-errCh

	// 2. Active listener conflict: active socket must cause error
	lnActive, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen active: %v", err)
	}
	defer lnActive.Close()

	srvConflicting, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	err = srvConflicting.Serve(context.Background())
	if err == nil {
		t.Fatal("expected conflict error when another daemon is active, got nil")
	}
}

func TestGate1_PeerUID(t *testing.T) {
	sock, tok, tokVal := testSetup(t)

	// Configure server to expect a different UID (e.g. UID+9999)
	wrongUID := uint32(os.Geteuid() + 9999)
	cfg := Config{
		SocketPath:   sock,
		TokenPath:    tok,
		Token:        tokVal,
		ExpectUID:    wrongUID,
		ExpectUIDSet: true,
	}

	_, cancel, errCh := startTestServer(t, cfg)
	defer func() {
		cancel()
		<-errCh
	}()

	client, err := Dial(context.Background(), sock, tokVal)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	ctx, cCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cCancel()

	_, err = client.Ping(ctx, &phonebridgelocalipcv1.PingRequest{Nonce: 42})
	if err == nil {
		t.Fatal("expected PermissionDenied on UID mismatch, got nil error")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.PermissionDenied {
		t.Fatalf("expected code PermissionDenied, got code=%v, err=%v", st.Code(), err)
	}
}

func TestGate2_BearerToken(t *testing.T) {
	sock, tok, tokVal := testSetup(t)

	cfg := Config{
		SocketPath: sock,
		TokenPath:  tok,
		Token:      tokVal,
	}

	_, cancel, errCh := startTestServer(t, cfg)
	defer func() {
		cancel()
		<-errCh
	}()

	// 1. Missing token
	noTokenClient, err := Dial(context.Background(), sock, "")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer noTokenClient.Close()

	ctx, cCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cCancel()
	_, err = noTokenClient.Ping(ctx, &phonebridgelocalipcv1.PingRequest{Nonce: 1})
	if err == nil {
		t.Fatal("expected Unauthenticated on missing token, got nil")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", err)
	}

	// 2. Invalid token
	wrongTokenClient, err := Dial(context.Background(), sock, "bad-token-1234567890")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer wrongTokenClient.Close()

	ctx2, cCancel2 := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cCancel2()
	_, err = wrongTokenClient.Ping(ctx2, &phonebridgelocalipcv1.PingRequest{Nonce: 2})
	if err == nil {
		t.Fatal("expected Unauthenticated on invalid token, got nil")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", err)
	}

	// 3. Valid token
	validClient, err := Dial(context.Background(), sock, tokVal)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer validClient.Close()

	ctx3, cCancel3 := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cCancel3()
	resp, err := validClient.Ping(ctx3, &phonebridgelocalipcv1.PingRequest{Nonce: 3})
	if err != nil {
		t.Fatalf("valid token ping failed: %v", err)
	}
	if resp.GetNonce() != 3 {
		t.Errorf("nonce = %d, want 3", resp.GetNonce())
	}
}

func TestRPC_Handshake_Ping_Health(t *testing.T) {
	sock, tok, tokVal := testSetup(t)

	cfg := Config{
		SocketPath:       sock,
		TokenPath:        tok,
		Token:            tokVal,
		ServerVersion:    "1.2.3-prod",
		DaemonGeneration: 987654321,
	}

	_, cancel, errCh := startTestServer(t, cfg)
	defer func() {
		cancel()
		<-errCh
	}()

	client, err := Dial(context.Background(), sock, tokVal)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// 1. Handshake
	hs, err := client.Handshake(ctx, &phonebridgelocalipcv1.HandshakeRequest{ClientVersion: 1})
	if err != nil {
		t.Fatalf("Handshake: %v", err)
	}
	if hs.GetNegotiatedVersion() != 1 {
		t.Errorf("negotiated_version = %d, want 1", hs.GetNegotiatedVersion())
	}
	if hs.GetServerVersion() != "1.2.3-prod" {
		t.Errorf("server_version = %q, want 1.2.3-prod", hs.GetServerVersion())
	}
	if hs.GetDaemonGeneration() != 987654321 {
		t.Errorf("daemon_generation = %d, want 987654321", hs.GetDaemonGeneration())
	}

	// 2. Ping
	p, err := client.Ping(ctx, &phonebridgelocalipcv1.PingRequest{Nonce: 555})
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if p.GetNonce() != 555 {
		t.Errorf("nonce = %d, want 555", p.GetNonce())
	}
	if p.GetServerVersion() != "1.2.3-prod" {
		t.Errorf("server_version = %q, want 1.2.3-prod", p.GetServerVersion())
	}

	// 3. Health
	h, err := client.Health(ctx, &phonebridgelocalipcv1.HealthRequest{})
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !h.GetReady() {
		t.Error("ready is false, want true")
	}
	if h.GetDaemonGeneration() != 987654321 {
		t.Errorf("daemon_generation = %d, want 987654321", h.GetDaemonGeneration())
	}
	if h.GetServerVersion() != "1.2.3-prod" {
		t.Errorf("server_version = %q, want 1.2.3-prod", h.GetServerVersion())
	}
}

func TestRPC_StreamEvents_Broadcast(t *testing.T) {
	sock, tok, tokVal := testSetup(t)

	cfg := Config{
		SocketPath:       sock,
		TokenPath:        tok,
		Token:            tokVal,
		ServerVersion:    "1.0.0",
		DaemonGeneration: 11223344,
	}

	srv, cancel, errCh := startTestServer(t, cfg)
	defer func() {
		cancel()
		<-errCh
	}()

	client, err := Dial(context.Background(), sock, tokVal)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	ctx, streamCancel := context.WithCancel(context.Background())
	defer streamCancel()

	stream, err := client.StreamEvents(ctx, &phonebridgelocalipcv1.StreamEventsRequest{})
	if err != nil {
		t.Fatalf("StreamEvents: %v", err)
	}

	// Give subscriber registration a brief moment
	time.Sleep(50 * time.Millisecond)

	const eventCount = 5
	var wg sync.WaitGroup
	wg.Add(1)

	var received []*phonebridgelocalipcv1.StreamEventsResponse
	var recvErr error

	go func() {
		defer wg.Done()
		for i := 0; i < eventCount; i++ {
			resp, err := stream.Recv()
			if err != nil {
				recvErr = err
				return
			}
			received = append(received, resp)
		}
	}()

	// Broadcast envelopes
	for i := 1; i <= eventCount; i++ {
		env := &phonebridgev1.Envelope{
			Sequence:    uint64(i),
			TimestampMs: uint64(time.Now().UnixMilli()),
			SessionId:   fmt.Sprintf("session-%d", i),
		}
		srv.BroadcastEnvelope(env)
		time.Sleep(10 * time.Millisecond)
	}

	wg.Wait()
	if recvErr != nil {
		t.Fatalf("stream.Recv failed: %v", recvErr)
	}

	if len(received) != eventCount {
		t.Fatalf("received %d events, want %d", len(received), eventCount)
	}

	for i, ev := range received {
		expectedSeq := uint64(i + 1)
		if ev.GetSeq() != expectedSeq {
			t.Errorf("event %d seq = %d, want %d", i, ev.GetSeq(), expectedSeq)
		}
		if ev.GetDaemonGeneration() != 11223344 {
			t.Errorf("daemon_generation = %d, want 11223344", ev.GetDaemonGeneration())
		}
		if ev.GetEnvelope() == nil || ev.GetEnvelope().GetSequence() != expectedSeq {
			t.Errorf("envelope sequence = %v, want %d", ev.GetEnvelope(), expectedSeq)
		}
	}
}

func TestPollAndConnect(t *testing.T) {
	sock, tok, tokVal := testSetup(t)

	cfg := Config{
		SocketPath: sock,
		TokenPath:  tok,
		Token:      tokVal,
	}

	// Start server asynchronously
	go func() {
		time.Sleep(50 * time.Millisecond)
		srv, _ := NewServer(cfg)
		_ = srv.Serve(context.Background())
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client, err := PollAndConnect(ctx, sock, tok, 2*time.Second)
	if err != nil {
		t.Fatalf("PollAndConnect: %v", err)
	}
	defer client.Close()

	resp, err := client.Ping(ctx, &phonebridgelocalipcv1.PingRequest{Nonce: 100})
	if err != nil {
		t.Fatalf("Ping after PollAndConnect: %v", err)
	}
	if resp.GetNonce() != 100 {
		t.Errorf("nonce = %d, want 100", resp.GetNonce())
	}
}

type mockOrchestrator struct {
	mu            sync.Mutex
	startErr      error
	stopErr       error
	snapshot      engine.SessionSnapshot
	session       *engine.Session
	devices       []discovery.Device
	lastRequested engine.MediaParams

	pairName    string
	pairSAS     string
	pairErr     error
	confirmErr  error
	trustedDevs []crypto.TrustEntry
	revokeErr   error
	cbStatus    *phonebridgelocalipcv1.GetClipboardStatusResponse
	cbErr       error
	cbPullErr   error

	lastSendFile  [3]string
	sendFileID    string
	sendFileErr   error
	lastCancelled string
	cancelErr     error
	transfers     []transfer.Info
}

func (m *mockOrchestrator) StartSession(ctx context.Context, deviceID string, requested engine.MediaParams) (*engine.Session, error) {
	m.mu.Lock()
	m.lastRequested = requested
	m.mu.Unlock()
	if m.startErr != nil {
		return nil, m.startErr
	}
	return m.session, nil
}

func (m *mockOrchestrator) StopSession(sessionID, reason string) error {
	return m.stopErr
}

func (m *mockOrchestrator) GetSessionState(sessionID string) (engine.SessionSnapshot, error) {
	return m.snapshot, nil
}

func (m *mockOrchestrator) ListDevices() []discovery.Device {
	return m.devices
}

func (m *mockOrchestrator) PairDevice(ctx context.Context, deviceID string) (string, string, error) {
	if m.pairErr != nil {
		return "", "", m.pairErr
	}
	return m.pairName, m.pairSAS, nil
}

func (m *mockOrchestrator) ConfirmPairing(ctx context.Context, deviceID string, confirmed bool) error {
	return m.confirmErr
}

func (m *mockOrchestrator) ListTrustedDevices() []crypto.TrustEntry {
	return m.trustedDevs
}

func (m *mockOrchestrator) RevokeDevice(deviceID string) error {
	return m.revokeErr
}

func (m *mockOrchestrator) GetClipboardStatus(ctx context.Context) (*phonebridgelocalipcv1.GetClipboardStatusResponse, error) {
	if m.cbErr != nil {
		return nil, m.cbErr
	}
	if m.cbStatus == nil {
		return &phonebridgelocalipcv1.GetClipboardStatusResponse{
			State: "STOPPED",
		}, nil
	}
	return m.cbStatus, nil
}

func (m *mockOrchestrator) TriggerClipboardPull(ctx context.Context) error {
	return m.cbPullErr
}

func (m *mockOrchestrator) SendFile(ctx context.Context, deviceID, localPath, filename string) (string, error) {
	m.mu.Lock()
	m.lastSendFile = [3]string{deviceID, localPath, filename}
	m.mu.Unlock()
	if m.sendFileErr != nil {
		return "", m.sendFileErr
	}
	return m.sendFileID, nil
}

func (m *mockOrchestrator) CancelTransfer(ctx context.Context, transferID string) error {
	m.mu.Lock()
	m.lastCancelled = transferID
	m.mu.Unlock()
	return m.cancelErr
}

func (m *mockOrchestrator) ListTransfers() []transfer.Info {
	return m.transfers
}

func TestMediaParamsConversions(t *testing.T) {
	p := engine.MediaParams{Width: 720, Height: 1600, FPS: 30, BitrateKbps: 4000, Codec: "h264"}

	if back := FromProtoMediaParams(ToProtoMediaParams(p)); !back.Equal(p) {
		t.Errorf("round trip lost data: %s -> %s", p, back)
	}
	if got := FromProtoMediaParams(nil); !got.IsZero() {
		t.Errorf("nil must decode to a zero tuple, got %s", got)
	}

	// An unreported tuple must stay absent on the wire, so a client cannot read
	// "unknown" as "the device confirmed the request".
	if got := ToProtoMediaParamsKnown(p, false); got != nil {
		t.Errorf("an unknown tuple must be encoded as absent, got %v", got)
	}
	if got := ToProtoMediaParamsKnown(p, true); got == nil || got.GetFps() != 30 {
		t.Errorf("a known tuple must be encoded, got %v", got)
	}
}

func TestRPC_SessionLifecycle_And_Events(t *testing.T) {
	sock, tok, tokVal := testSetup(t)

	sess := engine.NewSession("sess-prod-1", engine.DefaultSessionConfig(), nil, nil)
	_ = sess.Transition(engine.StateConnecting, "test")

	orch := &mockOrchestrator{
		session: sess,
		snapshot: engine.SessionSnapshot{
			SessionID:         "sess-prod-1",
			State:             engine.StateStreaming,
			ConnectedDuration: 5 * time.Second,
			ErrorMessage:      "",
			ReasonCode:        engine.ReasonNone,
			Requested:         engine.MediaParams{Width: 1080, Height: 2400, FPS: 60, BitrateKbps: 8000},
			Actual:            engine.MediaParams{Width: 720, Height: 1600, FPS: 30, BitrateKbps: 4000},
			ActualKnown:       true,
			// Real receiver state: the sink classification and the
			// depacketizer's transport counters must ride the same snapshot
			// that already carries session state and negotiated media.
			SinkKind:   engine.SinkKindDisplay,
			SinkActive: true,
			Stats: rtpmedia.StreamStats{
				SeqGaps:     7,
				DupSeq:      8,
				LatePackets: 9,
				TSBackward:  10,
			},
		},
		devices: []discovery.Device{
			{
				ID:      "device-pixel-7",
				Name:    "Pixel 7",
				Model:   "Pixel 7",
				Version: "1.0",
				Port:    7804,
			},
		},
	}

	cfg := Config{
		SocketPath:       sock,
		TokenPath:        tok,
		Token:            tokVal,
		ServerVersion:    "1.0.0",
		DaemonGeneration: 55667788,
		Orchestrator:     orch,
	}

	srv, cancel, errCh := startTestServer(t, cfg)
	defer func() {
		cancel()
		<-errCh
	}()

	client, err := Dial(context.Background(), sock, tokVal)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// 1. Subscribe to StreamEvents
	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()

	stream, err := client.StreamEvents(streamCtx, &phonebridgelocalipcv1.StreamEventsRequest{})
	if err != nil {
		t.Fatalf("StreamEvents: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// 2. Test ListDevices
	listResp, err := client.ListDevices(ctx, &phonebridgelocalipcv1.ListDevicesRequest{})
	if err != nil {
		t.Fatalf("ListDevices failed: %v", err)
	}
	if len(listResp.GetDevices()) != 1 {
		t.Fatalf("expected 1 device, got %d", len(listResp.GetDevices()))
	}
	if listResp.GetDevices()[0].GetId() != "device-pixel-7" {
		t.Errorf("device id = %s, want device-pixel-7", listResp.GetDevices()[0].GetId())
	}

	// 3. Test StartSession — the requested media tuple must reach the engine,
	// because DEC-020 makes the parameters part of the capture consent.
	startResp, err := client.StartSession(ctx, &phonebridgelocalipcv1.StartSessionRequest{
		DeviceId: "device-pixel-7",
		Requested: &phonebridgev1.MediaParams{
			Width: 1080, Height: 2400, Fps: 60, BitrateKbps: 8000,
		},
	})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}
	orch.mu.Lock()
	gotRequested := orch.lastRequested
	orch.mu.Unlock()
	wantRequested := engine.MediaParams{Width: 1080, Height: 2400, FPS: 60, BitrateKbps: 8000}
	if !gotRequested.Equal(wantRequested) {
		t.Errorf("requested tuple reached the engine as %s, want %s", gotRequested, wantRequested)
	}
	if startResp.GetSessionId() != "sess-prod-1" {
		t.Errorf("sessionId = %s, want sess-prod-1", startResp.GetSessionId())
	}
	if startResp.GetState() != phonebridgelocalipcv1.SessionState_SESSION_STATE_CONNECTING {
		t.Errorf("state = %v, want SESSION_STATE_CONNECTING", startResp.GetState())
	}

	// 3. Broadcast SessionEvent and receive on stream
	srv.BroadcastSessionEvent(&phonebridgelocalipcv1.SessionEvent{
		SessionId: "sess-prod-1",
		State:     phonebridgelocalipcv1.SessionState_SESSION_STATE_STREAMING,
		Reason:    "media track active",
	})

	evResp, err := stream.Recv()
	if err != nil {
		t.Fatalf("stream.Recv session event: %v", err)
	}
	if evResp.GetSeq() != 1 {
		t.Errorf("seq = %d, want 1", evResp.GetSeq())
	}
	if evResp.GetSessionEvent() == nil {
		t.Fatalf("expected SessionEvent in stream response")
	}
	if evResp.GetSessionEvent().GetState() != phonebridgelocalipcv1.SessionState_SESSION_STATE_STREAMING {
		t.Errorf("session state = %v, want STREAMING", evResp.GetSessionEvent().GetState())
	}

	// 4. Test GetSessionState
	stateResp, err := client.GetSessionState(ctx, &phonebridgelocalipcv1.GetSessionStateRequest{
		SessionId: "sess-prod-1",
	})
	if err != nil {
		t.Fatalf("GetSessionState failed: %v", err)
	}
	if stateResp.GetSessionId() != "sess-prod-1" {
		t.Errorf("sessionId = %s, want sess-prod-1", stateResp.GetSessionId())
	}
	if stateResp.GetState() != phonebridgelocalipcv1.SessionState_SESSION_STATE_STREAMING {
		t.Errorf("state = %v, want STREAMING", stateResp.GetState())
	}
	if stateResp.GetConnectedDurationMs() < 5000 {
		t.Errorf("connected duration = %d ms, want >= 5000", stateResp.GetConnectedDurationMs())
	}
	// Negotiated media must be reported as requested-vs-actual, never collapsed.
	if got := stateResp.GetRequested(); got.GetWidth() != 1080 || got.GetFps() != 60 {
		t.Errorf("requested media = %v, want 1080x2400@60", got)
	}
	if got := stateResp.GetActual(); got.GetWidth() != 720 || got.GetFps() != 30 {
		t.Errorf("actual media = %v, want the device-reported 720x1600@30", got)
	}
	if stateResp.GetReasonCode() != phonebridgelocalipcv1.SessionReason_SESSION_REASON_NONE {
		t.Errorf("reason code = %v, want NONE", stateResp.GetReasonCode())
	}
	// Sink classification reaches the client verbatim (display pipe, active).
	if stateResp.GetSinkKind() != phonebridgelocalipcv1.SinkKind_SINK_KIND_DISPLAY {
		t.Errorf("sink kind = %v, want SINK_KIND_DISPLAY", stateResp.GetSinkKind())
	}
	if !stateResp.GetSinkActive() {
		t.Error("sink active = false, want true")
	}
	// Transport counters reach the client with the receiver's semantics intact.
	if st := stateResp.GetStats(); st.GetSeqGaps() != 7 || st.GetDupSeq() != 8 ||
		st.GetLatePackets() != 9 || st.GetTsBackward() != 10 {
		t.Errorf("transport counters = gaps %d dup %d late %d tsback %d, want 7 8 9 10",
			st.GetSeqGaps(), st.GetDupSeq(), st.GetLatePackets(), st.GetTsBackward())
	}

	// 5. Test StopSession
	stopResp, err := client.StopSession(ctx, &phonebridgelocalipcv1.StopSessionRequest{
		SessionId: "sess-prod-1",
		Reason:    "user closed",
	})
	if err != nil {
		t.Fatalf("StopSession failed: %v", err)
	}
	if stopResp.GetState() != phonebridgelocalipcv1.SessionState_SESSION_STATE_STOPPED {
		t.Errorf("stop state = %v, want STOPPED", stopResp.GetState())
	}
}

func TestRPC_PairingAndTrust(t *testing.T) {
	sock, tok, tokVal := testSetup(t)

	orch := &mockOrchestrator{
		pairName: "POCO F5",
		pairSAS:  "123456",
		trustedDevs: []crypto.TrustEntry{
			{
				DeviceID:    "dev-123",
				DisplayName: "POCO F5",
				Platform:    "android",
				PublicKey:   []byte("test-public-key-32-bytes-long!"),
				PairedAt:    time.Unix(1700000000, 0),
				LastSeen:    time.Unix(1700000100, 0),
				Revoked:     false,
			},
		},
	}

	cfg := Config{
		SocketPath:    sock,
		TokenPath:     tok,
		Token:         tokVal,
		ServerVersion: "1.0.0",
		Orchestrator:  orch,
	}

	srv, cancel, errCh := startTestServer(t, cfg)
	_ = srv
	defer func() {
		cancel()
		<-errCh
	}()

	client, err := Dial(context.Background(), sock, tokVal)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// 1. PairDevice
	pairResp, err := client.PairDevice(ctx, &phonebridgelocalipcv1.PairDeviceRequest{
		DeviceId: "dev-123",
	})
	if err != nil {
		t.Fatalf("PairDevice: %v", err)
	}
	if pairResp.GetDisplayName() != "POCO F5" || pairResp.GetSas() != "123456" {
		t.Errorf("unexpected PairDevice response: %+v", pairResp)
	}
	if !pairResp.GetPendingConfirmation() {
		t.Errorf("expected pending_confirmation = true")
	}

	// 2. ConfirmPairing
	confirmResp, err := client.ConfirmPairing(ctx, &phonebridgelocalipcv1.ConfirmPairingRequest{
		DeviceId:      "dev-123",
		UserConfirmed: true,
	})
	if err != nil {
		t.Fatalf("ConfirmPairing: %v", err)
	}
	if !confirmResp.GetSuccess() {
		t.Errorf("expected confirm success = true")
	}

	// 3. ListTrustedDevices
	listResp, err := client.ListTrustedDevices(ctx, &phonebridgelocalipcv1.ListTrustedDevicesRequest{})
	if err != nil {
		t.Fatalf("ListTrustedDevices: %v", err)
	}
	if len(listResp.GetDevices()) != 1 {
		t.Fatalf("expected 1 trusted device, got %d", len(listResp.GetDevices()))
	}
	dev := listResp.GetDevices()[0]
	if dev.GetDeviceId() != "dev-123" || dev.GetDisplayName() != "POCO F5" {
		t.Errorf("unexpected device in list: %+v", dev)
	}

	// 4. RevokeDevice
	revokeResp, err := client.RevokeDevice(ctx, &phonebridgelocalipcv1.RevokeDeviceRequest{
		DeviceId: "dev-123",
	})
	if err != nil {
		t.Fatalf("RevokeDevice: %v", err)
	}
	if !revokeResp.GetSuccess() {
		t.Errorf("expected revoke success = true")
	}
}

func TestServer_ClipboardEndpoints(t *testing.T) {
	tmpDir := t.TempDir()
	sock := filepath.Join(tmpDir, "engine.sock")
	tok := filepath.Join(tmpDir, "token")
	tokVal := "test-bearer-token-cb"

	mock := &mockOrchestrator{
		cbStatus: &phonebridgelocalipcv1.GetClipboardStatusResponse{
			State:          "AMBIENT_ACTIVE",
			IsConnected:    true,
			MaxPayloadSize: 786432,
			RemotePeerId:   "remote-peer-456",
			AdapterStatus:  "READY",
			LastSyncMs:     1726000000000,
		},
	}

	cfg := Config{
		SocketPath:    sock,
		TokenPath:     tok,
		Token:         tokVal,
		ServerVersion: "1.0.0",
		Orchestrator:  mock,
	}

	s, cancel, errCh := startTestServer(t, cfg)
	defer func() {
		cancel()
		<-errCh
	}()

	client, err := Dial(context.Background(), sock, tokVal)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// 1. GetClipboardStatus
	statusResp, err := client.GetClipboardStatus(ctx, &phonebridgelocalipcv1.GetClipboardStatusRequest{})
	if err != nil {
		t.Fatalf("GetClipboardStatus: %v", err)
	}
	if statusResp.GetState() != "AMBIENT_ACTIVE" {
		t.Errorf("expected state AMBIENT_ACTIVE, got %s", statusResp.GetState())
	}
	if !statusResp.GetIsConnected() {
		t.Errorf("expected is_connected = true")
	}
	if statusResp.GetMaxPayloadSize() != 786432 {
		t.Errorf("expected max payload 786432, got %d", statusResp.GetMaxPayloadSize())
	}
	if statusResp.GetRemotePeerId() != "remote-peer-456" {
		t.Errorf("expected remote peer remote-peer-456, got %s", statusResp.GetRemotePeerId())
	}

	// 2. TriggerClipboardPull
	pullResp, err := client.TriggerClipboardPull(ctx, &phonebridgelocalipcv1.TriggerClipboardPullRequest{})
	if err != nil {
		t.Fatalf("TriggerClipboardPull: %v", err)
	}
	if !pullResp.GetSuccess() {
		t.Errorf("expected pull success = true")
	}

	// 3. BroadcastClipboardEvent on StreamEvents
	streamCtx, streamCancel := context.WithTimeout(ctx, 3*time.Second)
	defer streamCancel()

	stream, err := client.StreamEvents(streamCtx, &phonebridgelocalipcv1.StreamEventsRequest{})
	if err != nil {
		t.Fatalf("StreamEvents: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	s.BroadcastClipboardEvent(&phonebridgelocalipcv1.ClipboardStatusEvent{
		State:          "COSMIC_FLAG_REQUIRED",
		AdapterStatus:  "COSMIC_FLAG_REQUIRED",
		MaxPayloadSize: 786432,
	})

	evt, err := stream.Recv()
	if err != nil {
		t.Fatalf("stream.Recv: %v", err)
	}
	if evt.GetClipboardEvent() == nil {
		t.Fatalf("expected clipboard event in StreamEventsResponse")
	}
	if evt.GetClipboardEvent().GetState() != "COSMIC_FLAG_REQUIRED" {
		t.Errorf("expected COSMIC_FLAG_REQUIRED, got %s", evt.GetClipboardEvent().GetState())
	}
}
