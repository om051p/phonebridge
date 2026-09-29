package crypto

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
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

// TestVerifyRequest_AcceptsAndroidPeerOfferSignature pins the outbound signing
// format against the Android implementation.
//
// The signature below is produced by the Android client for exactly this
// (method, path, timestamp, nonce, body) tuple; the Kotlin test
// DeviceIdentitySigningTest asserts the phone emits this same hex for the same
// inputs. Because Ed25519 signatures are deterministic, matching hex proves the
// two languages build byte-identical canonical material - which is the only
// thing that decides whether a phone-originated request authenticates here.
func TestVerifyRequest_AcceptsAndroidPeerOfferSignature(t *testing.T) {
	const (
		androidDeviceID = "65b60673d6ed884bf01c2c222d82ada0740f29ac3355d6a925c81f17f47a27b8"
		androidPubHex   = "79b5562e8fe654f94078b112e8a98ba7901f853ae695bed7e0e3910bad049664"
		androidSigHex   = "3bb75ade65382eb8acb806e721df9fff2560d768d757c762e0b5f8fababb5cdfe8915d1e47e6380eb8bfbed1ac9b79e09ce072c4b1d03c24f87db3ee766e430d"
		androidTS       = "1790000000000"
		androidNonce    = "0f1e2d3c4b5a69788796a5b4c3d2e1f0"
	)
	pub, err := hex.DecodeString(androidPubHex)
	if err != nil {
		t.Fatalf("decode android public key: %v", err)
	}

	store, _ := NewTrustStore("")
	if err := store.AddTrusted(TrustEntry{
		DeviceID:    androidDeviceID,
		DisplayName: "Android Phone",
		Platform:    "android",
		PublicKey:   pub,
	}); err != nil {
		t.Fatalf("AddTrusted: %v", err)
	}

	const method = "POST"
	const path = "/session/peer-offer"
	body := []byte(`{"hello":"world"}`)

	headers := map[string]string{
		HeaderDeviceID:  androidDeviceID,
		HeaderTimestamp: androidTS,
		HeaderNonce:     androidNonce,
		HeaderSignature: androidSigHex,
	}

	// The vector's timestamp is fixed, so the freshness window is widened
	// deliberately: this test is about signing material, not replay timing.
	const window = 100000 * time.Hour
	nonces := NewNonceCache()
	deviceID, err := VerifyRequest(store, method, path, body, func(k string) string { return headers[k] }, nonces, window)
	if err != nil {
		t.Fatalf("the Android-signed request did not verify: %v", err)
	}
	if deviceID != androidDeviceID {
		t.Fatalf("verified device ID mismatch: %s != %s", deviceID, androidDeviceID)
	}

	// The same signature over a different path must NOT verify: that is what
	// makes the path part of the signature rather than decoration.
	nonces2 := NewNonceCache()
	if _, err := VerifyRequest(store, method, "/offer", body, func(k string) string { return headers[k] }, nonces2, window); err == nil {
		t.Fatalf("expected signature failure for a rewritten path")
	}

	// The production signing helper must reproduce the same bytes for the same
	// inputs, so the Android vector cannot drift away from this build silently.
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	bodyHash := sha256.Sum256(body)
	material := method + "\n" + path + "\n" + androidTS + "\n" + androidNonce + "\n" + hex.EncodeToString(bodyHash[:])
	if got := hex.EncodeToString(Sign(priv, []byte(material))); got != androidSigHex {
		t.Fatalf("signing material drifted: got %s want %s", got, androidSigHex)
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
