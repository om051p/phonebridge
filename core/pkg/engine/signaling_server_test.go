package engine

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	pion "github.com/pion/webrtc/v4"

	"github.com/om051p/phonebridge/core/pkg/crypto"
)

func TestSignalingServer_HealthAndPairing(t *testing.T) {
	tmpDir := t.TempDir()
	serverIDPath := filepath.Join(tmpDir, "server_identity.json")
	serverTrustPath := filepath.Join(tmpDir, "server_trust.json")
	clientIDPath := filepath.Join(tmpDir, "client_identity.json")
	clientTrustPath := filepath.Join(tmpDir, "client_trust.json")

	serverIdent, err := crypto.LoadOrGenerateIdentity(serverIDPath, "Server Node", "linux")
	if err != nil {
		t.Fatalf("generate server identity: %v", err)
	}
	serverTrust, err := crypto.NewTrustStore(serverTrustPath)
	if err != nil {
		t.Fatalf("generate server trust: %v", err)
	}

	clientIdent, err := crypto.LoadOrGenerateIdentity(clientIDPath, "Client Node", "linux")
	if err != nil {
		t.Fatalf("generate client identity: %v", err)
	}
	clientTrust, err := crypto.NewTrustStore(clientTrustPath)
	if err != nil {
		t.Fatalf("generate client trust: %v", err)
	}

	srv := NewSignalingServer(SignalingServerConfig{
		Port:       0, // Ephemeral
		Identity:   serverIdent,
		TrustStore: serverTrust,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start signaling server: %v", err)
	}
	defer srv.Close()

	endpoint := fmt.Sprintf("127.0.0.1:%d", srv.Port())

	// 1. Test /health
	resp, err := http.Get(fmt.Sprintf("http://%s/health", endpoint))
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for /health, got %d", resp.StatusCode)
	}

	// 2. Test PairingClient against SignalingServer
	pairClient := crypto.NewPairingClient(3 * time.Second)
	sasVerified := false
	entry, err := pairClient.Pair(ctx, endpoint, clientIdent, clientTrust, func(remoteName, sas string) bool {
		if remoteName != "Server Node" || len(sas) != 6 {
			t.Errorf("unexpected SAS details: name=%s, sas=%s", remoteName, sas)
			return false
		}
		sasVerified = true
		return true
	})
	if err != nil {
		t.Fatalf("Pair failed: %v", err)
	}
	if !sasVerified {
		t.Fatal("SAS confirmation callback was not invoked")
	}
	if entry.DeviceID != serverIdent.DeviceID {
		t.Fatalf("expected device ID %s, got %s", serverIdent.DeviceID, entry.DeviceID)
	}

	// Verify server trust store also contains the client
	clientEntry, ok := serverTrust.Get(clientIdent.DeviceID)
	if !ok {
		t.Fatalf("client %s not found in server trust store", clientIdent.DeviceID)
	}
	if clientEntry.DisplayName != "Client Node" {
		t.Fatalf("expected display name Client Node, got %s", clientEntry.DisplayName)
	}
}

func TestSignalingServer_SessionFlowAuthenticated(t *testing.T) {
	tmpDir := t.TempDir()
	serverIDPath := filepath.Join(tmpDir, "server_id.json")
	serverTrustPath := filepath.Join(tmpDir, "server_trust.json")
	clientIDPath := filepath.Join(tmpDir, "client_id.json")

	serverIdent, _ := crypto.LoadOrGenerateIdentity(serverIDPath, "Server", "linux")
	serverTrust, _ := crypto.NewTrustStore(serverTrustPath)
	clientIdent, _ := crypto.LoadOrGenerateIdentity(clientIDPath, "Client", "linux")

	// Pre-trust client
	_ = serverTrust.AddTrusted(crypto.TrustEntry{
		DeviceID:    clientIdent.DeviceID,
		DisplayName: clientIdent.DisplayName,
		Platform:    clientIdent.Platform,
		PublicKey:   clientIdent.PublicKey,
		PairedAt:    time.Now(),
		LastSeen:    time.Now(),
	})

	offerHandled := false
	answerHandled := false
	stopHandled := false
	stopPeerID := ""

	srv := NewSignalingServer(SignalingServerConfig{
		Port:       0,
		Identity:   serverIdent,
		TrustStore: serverTrust,
		OfferHandler: func(req NegotiationRequest) (NegotiationResponse, error) {
			offerHandled = true
			return NegotiationResponse{
				Offer:    "v=0\r\no=- 999 2 IN IP4 127.0.0.1\r\ns=-\r\n",
				Accepted: true,
				Actual:   req.Requested,
			}, nil
		},
		AnswerHandler: func(answer pion.SessionDescription) error {
			answerHandled = true
			return nil
		},
		StopHandler: func(peerDeviceID, reason string, code Code) error {
			stopHandled = true
			stopPeerID = peerDeviceID
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer srv.Close()

	endpoint := fmt.Sprintf("127.0.0.1:%d", srv.Port())

	// Client with identity (authenticated)
	client := NewSignalingClient(2 * time.Second)
	client.SetIdentity(clientIdent)

	// 1. RequestOffer
	negResp, err := client.RequestOffer(ctx, endpoint, NegotiationRequest{
		Requested: MediaParams{Width: 1920, Height: 1080, FPS: 60},
	})
	if err != nil {
		t.Fatalf("RequestOffer failed: %v", err)
	}
	if !offerHandled || !negResp.Accepted || negResp.Offer == "" {
		t.Fatalf("unexpected offer response: %+v", negResp)
	}

	// 2. SendAnswer
	err = client.SendAnswer(ctx, endpoint, pion.SessionDescription{
		Type: pion.SDPTypeAnswer,
		SDP:  "v=0\r\no=- 888 2 IN IP4 127.0.0.1\r\ns=-\r\n",
	})
	if err != nil {
		t.Fatalf("SendAnswer failed: %v", err)
	}
	if !answerHandled {
		t.Fatal("AnswerHandler was not invoked")
	}

	// 3. StopSession
	err = client.StopSession(ctx, endpoint, "user disconnect", CodeOK)
	if err != nil {
		t.Fatalf("StopSession failed: %v", err)
	}
	if !stopHandled {
		t.Fatal("StopHandler was not invoked")
	}
	// The peer id must be the one the signature proved, not something the body
	// claimed: the manager relies on it to decide which session may end.
	if stopPeerID != clientIdent.DeviceID {
		t.Fatalf("StopHandler peer = %q, want %q", stopPeerID, clientIdent.DeviceID)
	}

	// 4. Test Unauthenticated Client (must be rejected with 401)
	unauthPub, unauthPriv, _ := ed25519.GenerateKey(rand.Reader)
	unauthIdent := &crypto.DeviceIdentity{
		DeviceID:   "untrusted-device",
		PublicKey:  unauthPub,
		PrivateKey: unauthPriv,
	}
	unauthClient := NewSignalingClient(2 * time.Second)
	unauthClient.SetIdentity(unauthIdent)

	_, err = unauthClient.RequestOffer(ctx, endpoint, NegotiationRequest{})
	if err == nil {
		t.Fatal("expected unauthenticated request to fail, but it succeeded")
	}
}

// postPairingConfirm posts a confirm payload for token/sas signed by signer.
func postPairingConfirm(t *testing.T, endpoint, token, sas string, signer *crypto.DeviceIdentity) int {
	t.Helper()
	sig := crypto.Sign(signer.PrivateKey, []byte(fmt.Sprintf("%s:%s", token, sas)))
	payload := crypto.PairingConfirmPayload{
		DeviceID:     signer.DeviceID,
		PairingToken: token,
		SAS:          sas,
		Confirmed:    true,
		Signature:    hex.EncodeToString(sig),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal confirm: %v", err)
	}
	resp, err := http.Post(fmt.Sprintf("http://%s/pairing/confirm", endpoint), "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("post confirm: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func startPairingTestServer(t *testing.T, tmpDir string) (*SignalingServer, *crypto.DeviceIdentity, *crypto.TrustStore, *crypto.DeviceIdentity, string) {
	t.Helper()
	serverIdent, err := crypto.LoadOrGenerateIdentity(filepath.Join(tmpDir, "server_id.json"), "Server", "linux")
	if err != nil {
		t.Fatalf("server identity: %v", err)
	}
	serverTrust, err := crypto.NewTrustStore(filepath.Join(tmpDir, "server_trust.json"))
	if err != nil {
		t.Fatalf("server trust: %v", err)
	}
	clientIdent, err := crypto.LoadOrGenerateIdentity(filepath.Join(tmpDir, "client_id.json"), "Client", "android")
	if err != nil {
		t.Fatalf("client identity: %v", err)
	}
	srv := NewSignalingServer(SignalingServerConfig{Port: 0, Identity: serverIdent, TrustStore: serverTrust})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv, serverIdent, serverTrust, clientIdent, fmt.Sprintf("127.0.0.1:%d", srv.Port())
}

// An expired pending token must be rejected AND removed, even with a valid signature.
func TestSignalingServer_PairingConfirmExpiredTokenRejected(t *testing.T) {
	tmpDir := t.TempDir()
	srv, serverIdent, serverTrust, clientIdent, endpoint := startPairingTestServer(t, tmpDir)

	token := "expired-token-1"
	sas := crypto.CalculateSAS(serverIdent.PublicKey, clientIdent.PublicKey, token)
	srv.mu.Lock()
	srv.pendingPairings[token] = serverPendingPairing{
		token: token, remoteName: "Client", remotePlatform: "android",
		remotePub: clientIdent.PublicKey, sas: sas,
		createdAt: time.Now().Add(-6 * time.Minute),
	}
	srv.mu.Unlock()

	if code := postPairingConfirm(t, endpoint, token, sas, clientIdent); code != http.StatusBadRequest {
		t.Fatalf("expected 400 for expired token, got %d", code)
	}
	srv.mu.Lock()
	_, stillThere := srv.pendingPairings[token]
	srv.mu.Unlock()
	if stillThere {
		t.Fatal("expired token was not removed from pendingPairings")
	}
	if _, ok := serverTrust.Get(clientIdent.DeviceID); ok {
		t.Fatal("expired token committed trust")
	}
}

// A duplicate confirm (replayed after success) must not create a second record.
func TestSignalingServer_PairingDuplicateConfirmSingleRecord(t *testing.T) {
	tmpDir := t.TempDir()
	_, serverIdent, serverTrust, clientIdent, _ := startPairingTestServer(t, tmpDir)

	token := "dup-token-1"
	sas := crypto.CalculateSAS(serverIdent.PublicKey, clientIdent.PublicKey, token)

	srv := NewSignalingServer(SignalingServerConfig{Port: 0, Identity: serverIdent, TrustStore: serverTrust})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer srv.Close()
	endpoint := fmt.Sprintf("127.0.0.1:%d", srv.Port())
	srv.mu.Lock()
	srv.pendingPairings[token] = serverPendingPairing{
		token: token, remoteName: "Client", remotePlatform: "android",
		remotePub: clientIdent.PublicKey, sas: sas, createdAt: time.Now(),
	}
	srv.mu.Unlock()

	if code := postPairingConfirm(t, endpoint, token, sas, clientIdent); code != http.StatusOK {
		t.Fatalf("expected 200 for first confirm, got %d", code)
	}
	if code := postPairingConfirm(t, endpoint, token, sas, clientIdent); code != http.StatusBadRequest {
		t.Fatalf("expected 400 for replayed confirm, got %d", code)
	}
	if n := len(serverTrust.List()); n != 1 {
		t.Fatalf("expected exactly 1 trust record, got %d", n)
	}
}
