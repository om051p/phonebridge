package crypto

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIdentity_GenerateAndVerify(t *testing.T) {
	id, err := GenerateIdentity("Test Linux", "linux")
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}

	if id.DeviceID == "" || len(id.DeviceID) != 64 {
		t.Fatalf("invalid device ID format: %s", id.DeviceID)
	}
	if len(id.PublicKey) != ed25519.PublicKeySize {
		t.Fatalf("invalid public key size: %d", len(id.PublicKey))
	}
	if len(id.PrivateKey) != ed25519.PrivateKeySize {
		t.Fatalf("invalid private key size: %d", len(id.PrivateKey))
	}

	msg := []byte("hello phonebridge authentication")
	sig := Sign(id.PrivateKey, msg)

	if !Verify(id.PublicKey, msg, sig) {
		t.Fatal("signature verification failed")
	}

	tamperedMsg := []byte("hello phonebridge authentication tampered")
	if Verify(id.PublicKey, tamperedMsg, sig) {
		t.Fatal("signature verification should have failed for tampered message")
	}
}

func TestIdentity_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "sub", "identity.json")

	id1, err := LoadOrGenerateIdentity(keyPath, "My Laptop", "linux")
	if err != nil {
		t.Fatalf("LoadOrGenerateIdentity 1: %v", err)
	}

	id2, err := LoadOrGenerateIdentity(keyPath, "My Laptop", "linux")
	if err != nil {
		t.Fatalf("LoadOrGenerateIdentity 2: %v", err)
	}

	if id1.DeviceID != id2.DeviceID {
		t.Fatalf("device ID changed across reload: %s != %s", id1.DeviceID, id2.DeviceID)
	}
	if hex.EncodeToString(id1.PublicKey) != hex.EncodeToString(id2.PublicKey) {
		t.Fatal("public key changed across reload")
	}
}

func TestCalculateSAS_Symmetry(t *testing.T) {
	idA, _ := GenerateIdentity("A", "linux")
	idB, _ := GenerateIdentity("B", "android")

	token := "abcdef1234567890"

	sasAB := CalculateSAS(idA.PublicKey, idB.PublicKey, token)
	sasBA := CalculateSAS(idB.PublicKey, idA.PublicKey, token)

	if sasAB != sasBA {
		t.Fatalf("SAS must be symmetric: %s != %s", sasAB, sasBA)
	}
	if len(sasAB) != 6 {
		t.Fatalf("SAS must be 6 digits: %s", sasAB)
	}
}

func TestTrustStore_LifecycleAndPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "trust.json")

	store, err := NewTrustStore(storePath)
	if err != nil {
		t.Fatalf("NewTrustStore: %v", err)
	}

	phoneID, _ := GenerateIdentity("POCO F5", "android")

	// 1. Initially untrusted
	if store.IsTrusted(phoneID.DeviceID) {
		t.Fatal("device should not be trusted initially")
	}

	// 2. Add trusted
	err = store.AddTrusted(TrustEntry{
		DeviceID:    phoneID.DeviceID,
		DisplayName: phoneID.DisplayName,
		Platform:    phoneID.Platform,
		PublicKey:   phoneID.PublicKey,
	})
	if err != nil {
		t.Fatalf("AddTrusted: %v", err)
	}

	if !store.IsTrusted(phoneID.DeviceID) {
		t.Fatal("device should be trusted after AddTrusted")
	}

	// 3. Persistence across restart
	reloaded, err := NewTrustStore(storePath)
	if err != nil {
		t.Fatalf("reload trust store: %v", err)
	}
	if !reloaded.IsTrusted(phoneID.DeviceID) {
		t.Fatal("device should remain trusted after reload")
	}

	// 4. Revoke
	if err := reloaded.Revoke(phoneID.DeviceID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if reloaded.IsTrusted(phoneID.DeviceID) {
		t.Fatal("revoked device must not be trusted")
	}

	// 5. Persistence of revocation
	reloaded2, err := NewTrustStore(storePath)
	if err != nil {
		t.Fatalf("reload 2 trust store: %v", err)
	}
	if reloaded2.IsTrusted(phoneID.DeviceID) {
		t.Fatal("revoked device must remain revoked after reload")
	}
}

func TestAuth_SignAndVerifyRequest(t *testing.T) {
	linuxID, _ := GenerateIdentity("Linux", "linux")
	androidID, _ := GenerateIdentity("Android", "android")

	store, _ := NewTrustStore("")
	_ = store.AddTrusted(TrustEntry{
		DeviceID:    linuxID.DeviceID,
		DisplayName: linuxID.DisplayName,
		Platform:    linuxID.Platform,
		PublicKey:   linuxID.PublicKey,
	})

	nonces := NewNonceCache()
	method := "POST"
	path := "/session/offer"
	body := []byte(`{"type":"offer","sdp":"v=0"}`)

	// 1. Normal signing & verification
	headers, err := SignRequest(linuxID, method, path, body)
	if err != nil {
		t.Fatalf("SignRequest: %v", err)
	}

	getHeader := func(k string) string {
		return headers[k]
	}

	devID, err := VerifyRequest(store, method, path, body, getHeader, nonces, 30*time.Second)
	if err != nil {
		t.Fatalf("VerifyRequest failed: %v", err)
	}
	if devID != linuxID.DeviceID {
		t.Fatalf("verified device ID mismatch: %s != %s", devID, linuxID.DeviceID)
	}

	// 2. Replay attack rejection
	_, err = VerifyRequest(store, method, path, body, getHeader, nonces, 30*time.Second)
	if err == nil || !strings.Contains(err.Error(), "replayed") {
		t.Fatalf("expected replay rejection, got %v", err)
	}

	// 3. Untrusted device rejection
	nonces2 := NewNonceCache()
	untrustedHeaders, _ := SignRequest(androidID, method, path, body)
	_, err = VerifyRequest(store, method, path, body, func(k string) string { return untrustedHeaders[k] }, nonces2, 30*time.Second)
	if err == nil || !strings.Contains(err.Error(), "not trusted") {
		t.Fatalf("expected untrusted rejection, got %v", err)
	}

	// 4. Tampered body rejection
	nonces3 := NewNonceCache()
	tamperedBody := []byte(`{"type":"offer","sdp":"v=TAMPERED"}`)
	validHeaders, _ := SignRequest(linuxID, method, path, body)
	_, err = VerifyRequest(store, method, path, tamperedBody, func(k string) string { return validHeaders[k] }, nonces3, 30*time.Second)
	if err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("expected signature failure for tampered body, got %v", err)
	}

	// 5. Revoked device rejection
	_ = store.Revoke(linuxID.DeviceID)
	nonces4 := NewNonceCache()
	revokedHeaders, _ := SignRequest(linuxID, method, path, body)
	_, err = VerifyRequest(store, method, path, body, func(k string) string { return revokedHeaders[k] }, nonces4, 30*time.Second)
	if err == nil || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("expected revoked rejection, got %v", err)
	}
}

func TestPairingClient_CompleteHandshake(t *testing.T) {
	clientIdentity, _ := GenerateIdentity("Linux Client", "linux")
	serverIdentity, _ := GenerateIdentity("Android Server", "android")

	clientStore, _ := NewTrustStore("")
	serverStore, _ := NewTrustStore("")

	var pendingToken string
	var calculatedSAS string

	// Mock server implementing pairing endpoints
	mux := http.NewServeMux()
	mux.HandleFunc("/pairing/request", func(w http.ResponseWriter, r *http.Request) {
		var req PairingRequestPayload
		_ = json.NewDecoder(r.Body).Decode(&req)
		remotePub, _ := hex.DecodeString(req.PublicKey)

		pendingToken = req.PairingToken
		calculatedSAS = CalculateSAS(serverIdentity.PublicKey, remotePub, pendingToken)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(PairingAcceptPayload{
			DisplayName: serverIdentity.DisplayName,
			Platform:    serverIdentity.Platform,
			PublicKey:   hex.EncodeToString(serverIdentity.PublicKey),
			SAS:         calculatedSAS,
		})
	})

	mux.HandleFunc("/pairing/confirm", func(w http.ResponseWriter, r *http.Request) {
		var confirm PairingConfirmPayload
		_ = json.NewDecoder(r.Body).Decode(&confirm)

		if !confirm.Confirmed || confirm.SAS != calculatedSAS || confirm.PairingToken != pendingToken {
			http.Error(w, "invalid confirmation", http.StatusBadRequest)
			return
		}

		_ = serverStore.AddTrusted(TrustEntry{
			DeviceID:    confirm.DeviceID,
			DisplayName: clientIdentity.DisplayName,
			Platform:    clientIdentity.Platform,
			PublicKey:   clientIdentity.PublicKey,
		})

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"paired"}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	endpoint := strings.TrimPrefix(srv.URL, "http://")
	client := NewPairingClient(2 * time.Second)

	entry, err := client.Pair(context.Background(), endpoint, clientIdentity, clientStore, func(name, sas string) bool {
		return sas == calculatedSAS
	})
	if err != nil {
		t.Fatalf("Pair failed: %v", err)
	}

	if entry.DeviceID != serverIdentity.DeviceID {
		t.Fatalf("paired device ID mismatch: %s != %s", entry.DeviceID, serverIdentity.DeviceID)
	}

	if !clientStore.IsTrusted(serverIdentity.DeviceID) {
		t.Fatal("server should be trusted in client store")
	}
	if !serverStore.IsTrusted(clientIdentity.DeviceID) {
		t.Fatal("client should be trusted in server store")
	}
}
