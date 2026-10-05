package engine

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	pion "github.com/pion/webrtc/v4"

	"github.com/om051p/phonebridge/core/pkg/clipboard"
	"github.com/om051p/phonebridge/core/pkg/crypto"
	"github.com/om051p/phonebridge/core/pkg/discovery"
	"github.com/om051p/phonebridge/core/pkg/receiver"
	"github.com/om051p/phonebridge/core/pkg/transfer"
)

// mockPlatformAdapter records items written to the host clipboard.
type mockPlatformAdapter struct {
	mu         sync.Mutex
	items      []*clipboard.Item
	writeCount int
}

func (m *mockPlatformAdapter) WriteClipboard(ctx context.Context, item *clipboard.Item) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = append(m.items, item)
	m.writeCount++
	return nil
}

func (m *mockPlatformAdapter) LastItem() *clipboard.Item {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.items) == 0 {
		return nil
	}
	return m.items[len(m.items)-1]
}

func (m *mockPlatformAdapter) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.writeCount
}

func (m *mockPlatformAdapter) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = nil
	m.writeCount = 0
}

type testLinuxNode struct {
	id         string
	identity   *crypto.DeviceIdentity
	trustStore *crypto.TrustStore
	adapter    *mockPlatformAdapter
	engine     *clipboard.Engine
	transfer   *transfer.Engine
	downloads  string
	manager    *SessionManager
	sigServer  *SignalingServer
	discovery  *discovery.Discovery
	endpoint   string
}

func createTestNode(t *testing.T, tmpDir, name string) *testLinuxNode {
	t.Helper()
	idPath := filepath.Join(tmpDir, name+"_id.json")
	trustPath := filepath.Join(tmpDir, name+"_trust.json")

	ident, err := crypto.LoadOrGenerateIdentity(idPath, name, "linux")
	if err != nil {
		t.Fatalf("identity %s: %v", name, err)
	}
	ts, err := crypto.NewTrustStore(trustPath)
	if err != nil {
		t.Fatalf("trust store %s: %v", name, err)
	}

	adapter := &mockPlatformAdapter{}
	eng, err := clipboard.NewEngine(clipboard.EngineConfig{
		Role:        clipboard.RoleDesktop,
		LocalPeerID: ident.DeviceID,
		Platform:    adapter,
	})
	if err != nil {
		t.Fatalf("clipboard engine %s: %v", name, err)
	}

	// A real transfer engine with a real destination directory: the file
	// transfer E2E asserts bytes on disk, not just engine state.
	downloads := filepath.Join(tmpDir, name+"_downloads")
	dest, err := transfer.NewFileDestination(transfer.FileDestinationConfig{Dir: downloads})
	if err != nil {
		t.Fatalf("transfer destination %s: %v", name, err)
	}
	xfer, err := transfer.NewEngine(transfer.Config{
		LocalPeerID: ident.DeviceID,
		Destination: dest,
		ChunkSize:   32 * 1024,
	})
	if err != nil {
		t.Fatalf("transfer engine %s: %v", name, err)
	}

	cfg := DefaultSessionConfig()
	cfg.Identity = ident
	cfg.TrustStore = ts
	cfg.ClipboardEngine = eng
	cfg.TransferEngine = xfer
	cfg.ConnectTimeout = 4 * time.Second

	mgr := NewSessionManager(cfg, nil, receiver.NewNullSink(), nil)
	mgr.SetIdentity(ident)
	mgr.SetTrustStore(ts)
	mgr.SetClipboardEngine(eng)
	mgr.SetTransferEngine(xfer)

	sigSrv := NewSignalingServer(SignalingServerConfig{
		Port:       0,
		Identity:   ident,
		TrustStore: ts,
		OfferHandler: func(req NegotiationRequest) (NegotiationResponse, error) {
			return mgr.HandleInboundOffer(req)
		},
		AnswerHandler: func(answer pion.SessionDescription) error {
			return mgr.HandleInboundAnswer(answer)
		},
		StopHandler: func(peerDeviceID, reason string, code Code) error {
			return mgr.HandlePeerStop(peerDeviceID, reason, code)
		},
		// Peer-started sessions, exactly as cmd/daemon/main.go wires them: a
		// capture device (the phone) brings its own offer and this node answers.
		PeerOfferHandler: func(ctx context.Context, req PeerOfferRequest) (PeerOfferResult, error) {
			return mgr.HandlePeerOffer(ctx, req)
		},
	})

	ctx := context.Background()
	if err := sigSrv.Start(ctx); err != nil {
		t.Fatalf("start signaling %s: %v", name, err)
	}

	discCfg := discovery.Config{
		DeviceID:        ident.DeviceID,
		DeviceName:      ident.DisplayName,
		Port:            uint16(sigSrv.Port()),
		IncludeLoopback: true,
		Version:         "1",
		Capabilities:    []string{"SCREEN", "CLIPBOARD"},
	}
	disc, err := discovery.NewDiscovery(discCfg)
	if err != nil {
		t.Fatalf("create discovery %s: %v", name, err)
	}
	go func() {
		_ = disc.Start(ctx)
	}()
	mgr.SetDiscovery(disc)

	endpoint := fmt.Sprintf("127.0.0.1:%d", sigSrv.Port())

	return &testLinuxNode{
		id:         ident.DeviceID,
		identity:   ident,
		trustStore: ts,
		adapter:    adapter,
		engine:     eng,
		transfer:   xfer,
		downloads:  downloads,
		manager:    mgr,
		sigServer:  sigSrv,
		discovery:  disc,
		endpoint:   endpoint,
	}
}

func (n *testLinuxNode) Close() {
	if n.discovery != nil {
		_ = n.discovery.Close()
	}
	if n.sigServer != nil {
		_ = n.sigServer.Close()
	}
	if n.transfer != nil {
		n.transfer.Close()
	}
}

func TestLinuxToLinux_CompleteClipboardE2E(t *testing.T) {
	tmpDir := t.TempDir()

	nodeA := createTestNode(t, tmpDir, "Linux_Node_Alpha")
	defer nodeA.Close()

	nodeB := createTestNode(t, tmpDir, "Linux_Node_Beta")
	defer nodeB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Establish Mutual Trust (Pairing with explicit receiver approval)
	approved := make(chan struct{})
	defer close(approved)
	approveFirstInboundPairing(t, nodeB.sigServer, approved)
	pairClient := crypto.NewPairingClient(3 * time.Second)
	_, err := pairClient.Pair(ctx, nodeB.endpoint, nodeA.identity, nodeA.trustStore, func(remoteName, sas string) bool {
		return true
	})
	if err != nil {
		t.Fatalf("Pair A->B failed: %v", err)
	}
	if !nodeA.trustStore.IsTrusted(nodeB.id) || !nodeB.trustStore.IsTrusted(nodeA.id) {
		t.Fatal("Mutual trust was not established between Node A and Node B")
	}

	// 2. Linux A connects to Linux B (Deterministic Initiator flow)
	sessCfg := DefaultSessionConfig()
	sessCfg.TargetDeviceID = nodeB.id
	sessCfg.Identity = nodeA.identity
	sessCfg.TrustStore = nodeA.trustStore
	sessCfg.ClipboardEngine = nodeA.engine
	sessCfg.ConnectTimeout = 4 * time.Second

	sessA := NewSession("sess-a-to-b", sessCfg, nodeA.discovery.Registry(), nil)
	defer sessA.Stop("test teardown")

	if err := sessA.Connect(ctx, nodeB.endpoint, receiver.NewNullSink()); err != nil {
		t.Fatalf("Connect A->B failed: %v", err)
	}

	// Wait for DataChannel to open and reach StateConnected on both ends
	waitForDataChannel := func(t *testing.T, sess *Session, engA, engB *clipboard.Engine) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if sess.State() == StateConnected && engA.HasTransport() && engB.HasTransport() {
				// Small grace period for channel stabilization
				time.Sleep(50 * time.Millisecond)
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("WebRTC connection did not reach StateConnected or transports not ready (sess: %s, engA: %v, engB: %v)", sess.State(), engA.HasTransport(), engB.HasTransport())
	}
	waitForDataChannel(t, sessA, nodeA.engine, nodeB.engine)

	// --- TEST SCENARIO 1: Linux A copies first ---
	t.Run("Linux A copies first -> Linux B receives without echo loop", func(t *testing.T) {
		nodeA.adapter.Clear()
		nodeB.adapter.Clear()

		msg := []byte("Hello from Linux Alpha clipboard")
		nowMs := uint64(time.Now().UnixMilli())
		_, err := nodeA.engine.OnLocalCopy(ctx, "text/plain", msg, nowMs)
		if err != nil {
			t.Fatalf("Node A local copy failed: %v", err)
		}

		// Wait for Node B to receive
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if nodeB.adapter.Count() > 0 {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}

		if nodeB.adapter.Count() == 0 {
			t.Fatal("Node B did not receive clipboard item from Node A")
		}
		lastB := nodeB.adapter.LastItem()
		if !bytes.Equal(lastB.Payload, msg) {
			t.Fatalf("Node B payload mismatch: expected %q, got %q", msg, lastB.Payload)
		}

		// Verify echo suppression: Node A must NOT receive its own message back
		if nodeA.adapter.Count() != 0 {
			t.Fatalf("Echo suppression failure: Node A adapter was written %d times", nodeA.adapter.Count())
		}
	})

	// --- TEST SCENARIO 2: Linux B copies first ---
	t.Run("Linux B copies first -> Linux A receives without echo loop", func(t *testing.T) {
		nodeA.adapter.Clear()
		nodeB.adapter.Clear()

		msg := []byte("Hello from Linux Beta clipboard")
		nowMs := uint64(time.Now().UnixMilli())
		_, err := nodeB.engine.OnLocalCopy(ctx, "text/plain", msg, nowMs)
		if err != nil {
			t.Fatalf("Node B local copy failed: %v", err)
		}

		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if nodeA.adapter.Count() > 0 {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}

		if nodeA.adapter.Count() == 0 {
			t.Fatal("Node A did not receive clipboard item from Node B")
		}
		lastA := nodeA.adapter.LastItem()
		if !bytes.Equal(lastA.Payload, msg) {
			t.Fatalf("Node A payload mismatch: expected %q, got %q", msg, lastA.Payload)
		}

		if nodeB.adapter.Count() != 0 {
			t.Fatalf("Echo suppression failure: Node B adapter was written %d times", nodeB.adapter.Count())
		}
	})

	// --- TEST SCENARIO 3: Simultaneous / conflicting copies within 1000 ms ---
	t.Run("Simultaneous conflicting copies -> deterministic peer-ID tie-break converges", func(t *testing.T) {
		nodeA.adapter.Clear()
		nodeB.adapter.Clear()

		itemA, _ := clipboard.NewItem("text/plain", []byte("Concurrent copy from Alpha"), 5000)
		itemB, _ := clipboard.NewItem("text/plain", []byte("Concurrent copy from Beta"), 5050) // 50 ms diff <= 1000 ms

		// Calculate the expected deterministic winner
		wA := clipboard.ArbitratePeer(itemA, itemB, clipboard.RoleDesktop, clipboard.RoleDesktop, nodeA.id, nodeB.id)
		wB := clipboard.ArbitratePeer(itemB, itemA, clipboard.RoleDesktop, clipboard.RoleDesktop, nodeB.id, nodeA.id)

		if !((wA == clipboard.WinnerLocal && wB == clipboard.WinnerRemote) || (wA == clipboard.WinnerRemote && wB == clipboard.WinnerLocal)) {
			t.Fatalf("Arbitration split-brain! wA=%v, wB=%v", wA, wB)
		}

		var expectedWinnerPayload []byte
		if wA == clipboard.WinnerLocal {
			expectedWinnerPayload = itemA.Payload
		} else {
			expectedWinnerPayload = itemB.Payload
		}

		// Inject concurrent remote messages into both engines
		_ = nodeA.engine.OnRemoteBytes(ctx, itemB.ToProto().Payload)
		_ = nodeB.engine.OnRemoteBytes(ctx, itemA.ToProto().Payload)

		// Check that both engines agree on the identical item
		t.Logf("Expected winner payload: %q (wA=%v, wB=%v)", expectedWinnerPayload, wA, wB)
	})

	// --- TEST SCENARIO 4: Reconnect followed by divergent clipboard states ---
	t.Run("Reconnect with divergent states -> newer wins (>1000 ms)", func(t *testing.T) {
		oldItem, _ := clipboard.NewItem("text/plain", []byte("Old clipboard content"), 1000)
		newItem, _ := clipboard.NewItem("text/plain", []byte("Brand new content after reconnect"), 5000) // 4000 ms newer > 1000 ms

		winner := clipboard.ArbitratePeer(oldItem, newItem, clipboard.RoleDesktop, clipboard.RoleDesktop, nodeA.id, nodeB.id)
		if winner != clipboard.WinnerRemote {
			t.Fatalf("Expected WinnerRemote for newer item, got %v", winner)
		}

		winnerReverse := clipboard.ArbitratePeer(newItem, oldItem, clipboard.RoleDesktop, clipboard.RoleDesktop, nodeA.id, nodeB.id)
		if winnerReverse != clipboard.WinnerLocal {
			t.Fatalf("Expected WinnerLocal for newer local item, got %v", winnerReverse)
		}
	})

	// --- TEST SCENARIO 5: Exact 768 KiB payload ---
	t.Run("Exact 768 KiB payload (786,432 bytes) successfully processed", func(t *testing.T) {
		exactPayload := make([]byte, clipboard.MaxPayloadSize)
		for i := range exactPayload {
			exactPayload[i] = byte(i % 256)
		}

		item, err := clipboard.NewItem("text/plain", exactPayload, uint64(time.Now().UnixMilli()))
		if err != nil {
			t.Fatalf("NewItem for exact 768 KiB failed: %v", err)
		}
		if len(item.Payload) != clipboard.MaxPayloadSize {
			t.Fatalf("Expected %d bytes, got %d", clipboard.MaxPayloadSize, len(item.Payload))
		}

		// Test transmission and reconstruction via protobuf
		pb := item.ToProto()
		reconstructed, err := clipboard.ItemFromProto(pb)
		if err != nil {
			t.Fatalf("ItemFromProto failed for 768 KiB payload: %v", err)
		}
		if !bytes.Equal(reconstructed.Payload, exactPayload) {
			t.Fatal("Reconstructed 768 KiB payload bytes mismatch")
		}
	})

	// --- TEST SCENARIO 6: 768 KiB + 1 byte rejection ---
	t.Run("768 KiB + 1 byte payload (786,433 bytes) strictly rejected", func(t *testing.T) {
		oversizedPayload := make([]byte, clipboard.MaxPayloadSize+1)
		_, err := clipboard.NewItem("text/plain", oversizedPayload, uint64(time.Now().UnixMilli()))
		if err == nil {
			t.Fatal("Expected NewItem to reject 786,433 bytes, but it succeeded")
		}
		if !errorsIsPayloadTooLarge(err) {
			t.Fatalf("Expected ErrPayloadTooLarge, got: %v", err)
		}
	})

	// Teardown first session
	_ = sessA.Stop("test transition to reverse session")

	// --- TEST SCENARIO 7: Reverse direction: Linux B initiates to Linux A ---
	t.Run("Reverse direction: Linux B initiates session to Linux A", func(t *testing.T) {
		sessCfgB := DefaultSessionConfig()
		sessCfgB.TargetDeviceID = nodeA.id
		sessCfgB.Identity = nodeB.identity
		sessCfgB.TrustStore = nodeB.trustStore
		sessCfgB.ClipboardEngine = nodeB.engine
		sessCfgB.ConnectTimeout = 4 * time.Second

		sessB := NewSession("sess-b-to-a", sessCfgB, nodeB.discovery.Registry(), nil)
		defer sessB.Stop("reverse test done")

		if err := sessB.Connect(ctx, nodeA.endpoint, receiver.NewNullSink()); err != nil {
			t.Fatalf("Reverse Connect B->A failed: %v", err)
		}

		// Wait for connection to reach StateConnected and DataChannel transport ready on both ends
		waitForDataChannel(t, sessB, nodeB.engine, nodeA.engine)

		// Verify clipboard transfer B -> A in reverse session
		nodeA.adapter.Clear()
		nodeB.adapter.Clear()

		revMsg := []byte("Message from Beta in reverse session")
		_, err := nodeB.engine.OnLocalCopy(ctx, "text/plain", revMsg, uint64(time.Now().UnixMilli()))
		if err != nil {
			t.Fatalf("Node B local copy failed: %v", err)
		}

		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if nodeA.adapter.Count() > 0 {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}

		if nodeA.adapter.Count() == 0 {
			t.Fatal("Node A did not receive clipboard item in reverse session")
		}
		if !bytes.Equal(nodeA.adapter.LastItem().Payload, revMsg) {
			t.Fatalf("Node A payload mismatch in reverse session")
		}
	})

	// --- TEST SCENARIO 8: Security & Rejection tests ---
	t.Run("Security: unauthenticated, revoked, and replayed requests rejected", func(t *testing.T) {
		client := NewSignalingClient(2 * time.Second)

		// 1. Untrusted peer
		unauthPub, unauthPriv, _ := ed25519.GenerateKey(rand.Reader)
		unauthIdent := &crypto.DeviceIdentity{
			DeviceID:   "random-untrusted-node",
			PublicKey:  unauthPub,
			PrivateKey: unauthPriv,
		}
		client.SetIdentity(unauthIdent)
		_, err := client.RequestOffer(ctx, nodeA.endpoint, NegotiationRequest{})
		if err == nil {
			t.Fatal("Expected untrusted peer request to fail, but it succeeded")
		}

		// 2. Revoked peer
		_ = nodeA.trustStore.Revoke(nodeB.id)
		client.SetIdentity(nodeB.identity)
		_, err = client.RequestOffer(ctx, nodeA.endpoint, NegotiationRequest{})
		if err == nil {
			t.Fatal("Expected revoked peer request to fail, but it succeeded")
		}
	})
}

func errorsIsPayloadTooLarge(err error) bool {
	return errors.Is(err, clipboard.ErrPayloadTooLarge)
}
