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
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/crypto"
)

// ConfirmPairing must ride out 202-pending confirms (receiver still deciding)
// and complete once the approval lands — the same poll contract as
// crypto.PairingClient, for the daemon's split pair/confirm UI flow.
func TestSessionManager_ConfirmPairingPollsPending(t *testing.T) {
	ident, err := crypto.LoadOrGenerateIdentity("", "Linux Node", "linux")
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	store, err := crypto.NewTrustStore("")
	if err != nil {
		t.Fatalf("trust store: %v", err)
	}

	var confirms atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/pairing/confirm", func(w http.ResponseWriter, r *http.Request) {
		n := confirms.Add(1)
		if n <= 2 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"status":"pending"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"paired"}`))
	})
	fake := httptest.NewServer(mux)
	defer fake.Close()
	endpoint := fake.Listener.Addr().String()

	mgr := NewSessionManager(SessionConfig{Identity: ident, TrustStore: store}, nil, nil, nil)
	mgr.confirmPollInterval = 20 * time.Millisecond
	remotePub, _, _ := ed25519.GenerateKey(rand.Reader)
	mgr.pendingPairings["dev-1"] = &pendingPairing{
		deviceID:   "dev-1",
		endpoint:   endpoint,
		token:      "tok-1",
		sas:        "123456",
		remoteName: "Phone",
		remotePub:  remotePub,
		createdAt:  time.Now(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mgr.ConfirmPairing(ctx, "dev-1", true); err != nil {
		t.Fatalf("ConfirmPairing failed: %v", err)
	}
	if got := confirms.Load(); got != 3 {
		t.Fatalf("expected 3 confirm posts, got %d", got)
	}
	if _, ok := store.Get(crypto.Fingerprint(remotePub)); !ok {
		t.Fatal("paired device not committed to trust store under its canonical fingerprint")
	}
	var decoded crypto.PairingStatusPayload
	_ = json.Unmarshal([]byte(`{"status":"paired"}`), &decoded)
	if decoded.Status != "paired" {
		t.Fatal("sanity: status payload decode")
	}
}

// The manager's inbound-pairing surface delegates to the wired signaling
// server: ListInboundPairings reads its pending snapshot, RespondInboundPairing
// records decisions there, and an unwired manager reports nothing.
func TestSessionManager_InboundPairingDelegatesToSignalingServer(t *testing.T) {
	ident, err := crypto.LoadOrGenerateIdentity("", "Linux Node", "linux")
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	store, err := crypto.NewTrustStore("")
	if err != nil {
		t.Fatalf("trust store: %v", err)
	}
	srv := NewSignalingServer(SignalingServerConfig{Port: 0, Identity: ident, TrustStore: store})
	ctx := t.Context()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	mgr := NewSessionManager(SessionConfig{Identity: ident, TrustStore: store}, nil, nil, nil)

	// Unwired: nothing to list, nothing to answer.
	if got := mgr.ListInboundPairings(); len(got) != 0 {
		t.Fatalf("unwired manager must list nothing, got %d", len(got))
	}
	if mgr.RespondInboundPairing("tok-x", true) {
		t.Fatal("unwired manager must not record decisions")
	}

	mgr.SetSignalingServer(srv)

	// Seed a pending request the way /pairing/request does, from a real
	// requester key so the final confirm can carry a valid signature.
	requesterPub, requesterPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("requester key: %v", err)
	}
	body, _ := json.Marshal(crypto.PairingRequestPayload{
		DisplayName:  "Phone",
		Platform:     "android",
		PublicKey:    hex.EncodeToString(requesterPub),
		PairingToken: "tok-mgr",
	})
	resp, err := http.Post(fmt.Sprintf("http://127.0.0.1:%d/pairing/request", srv.Port()), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("pairing request: %v", err)
	}
	resp.Body.Close()

	pending := mgr.ListInboundPairings()
	if len(pending) != 1 || pending[0].Token != "tok-mgr" {
		t.Fatalf("manager must surface the server's pending request, got %+v", pending)
	}
	if pending[0].RemoteName != "Phone" || pending[0].RemotePlatform != "android" {
		t.Fatalf("pending request metadata mismatch: %+v", pending[0])
	}

	// The receiving user approves through the manager surface; the recorded
	// decision takes effect on the requester's next confirm poll.
	if !mgr.RespondInboundPairing("tok-mgr", true) {
		t.Fatal("wired manager must record the decision")
	}
	sig := crypto.Sign(requesterPriv, []byte("tok-mgr:"+pending[0].SAS))
	confirmBody, _ := json.Marshal(crypto.PairingConfirmPayload{
		DeviceID:     crypto.Fingerprint(requesterPub),
		PairingToken: "tok-mgr",
		SAS:          pending[0].SAS,
		Confirmed:    true,
		Signature:    hex.EncodeToString(sig),
	})
	cr, err := http.Post(fmt.Sprintf("http://127.0.0.1:%d/pairing/confirm", srv.Port()), "application/json", bytes.NewReader(confirmBody))
	if err != nil {
		t.Fatalf("pairing confirm: %v", err)
	}
	defer cr.Body.Close()
	if cr.StatusCode != http.StatusOK {
		t.Fatalf("approval recorded via the manager must let the confirm pair, got %d", cr.StatusCode)
	}
	if _, ok := store.Get(crypto.Fingerprint(requesterPub)); !ok {
		t.Fatal("trust not committed after manager-mediated approval")
	}
}
