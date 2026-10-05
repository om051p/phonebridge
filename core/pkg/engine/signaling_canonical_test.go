package engine

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/om051p/phonebridge/core/pkg/crypto"
)

// Phase A: the receiver must reject a confirm whose claimed device_id does
// not equal the fingerprint of the authenticated (token-bound, signature
// verified) public key. Accepting it would fork a second logical trust
// record for the same key.
func TestSignalingServer_ConfirmRejectsMismatchedDeviceID(t *testing.T) {
	serverIdent, err := crypto.LoadOrGenerateIdentity("", "Server Node", "linux")
	if err != nil {
		t.Fatalf("server identity: %v", err)
	}
	serverTrust, err := crypto.NewTrustStore("")
	if err != nil {
		t.Fatalf("server trust: %v", err)
	}
	clientIdent, err := crypto.LoadOrGenerateIdentity("", "Client Node", "android")
	if err != nil {
		t.Fatalf("client identity: %v", err)
	}

	srv := NewSignalingServer(SignalingServerConfig{
		Port:       0,
		Identity:   serverIdent,
		TrustStore: serverTrust,
	})
	ctx := t.Context()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	endpoint := fmt.Sprintf("127.0.0.1:%d", srv.Port())

	token := "mismatched-id-token-1"
	reqPayload, _ := json.Marshal(crypto.PairingRequestPayload{
		DisplayName:  clientIdent.DisplayName,
		Platform:     clientIdent.Platform,
		PublicKey:    hex.EncodeToString(clientIdent.PublicKey),
		PairingToken: token,
	})
	resp, err := http.Post(fmt.Sprintf("http://%s/pairing/request", endpoint), "application/json", bytes.NewReader(reqPayload))
	if err != nil {
		t.Fatalf("pairing request: %v", err)
	}
	var accept crypto.PairingAcceptPayload
	if err := json.NewDecoder(resp.Body).Decode(&accept); err != nil {
		t.Fatalf("decode accept: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for request, got %d", resp.StatusCode)
	}
	srv.ApprovePairing(token, true)

	sig := crypto.Sign(clientIdent.PrivateKey, []byte(token+":"+accept.SAS))
	body, _ := json.Marshal(crypto.PairingConfirmPayload{
		DeviceID:     "totally-different-id",
		PairingToken: token,
		SAS:          accept.SAS,
		Confirmed:    true,
		Signature:    hex.EncodeToString(sig),
	})
	r, err := http.Post(fmt.Sprintf("http://%s/pairing/confirm", endpoint), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("pairing confirm: %v", err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for mismatched device_id, got %d", r.StatusCode)
	}
	if len(serverTrust.List()) != 0 {
		t.Fatalf("mismatched confirm must not create a trust record, got %d", len(serverTrust.List()))
	}
	if _, ok := serverTrust.FindByPublicKey(clientIdent.PublicKey); ok {
		t.Fatal("mismatched confirm must not store the key under any ID")
	}
}
