package engine

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
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
