package engine

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/crypto"
)

// approveFirstInboundPairing simulates the receiving user tapping Accept: it
// watches srv for the first inbound request and approves it. Start it before
// the requester's Pair; the watcher exits after approving or when done closes
// (bounded by a deadline so a missing request cannot hang the suite).
func approveFirstInboundPairing(t *testing.T, srv *SignalingServer, done <-chan struct{}) {
	t.Helper()
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			select {
			case <-done:
				return
			default:
			}
			for _, p := range srv.PendingPairings() {
				if srv.ApprovePairing(p.Token, true) {
					return
				}
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
}

// A pairing confirm posted before the receiving user approves must NOT pair:
// the server holds the token (202 pending) until ApprovePairing, and only an
// explicit approval commits trust on both sides.
func TestSignalingServer_ConfirmWaitsForReceiverApproval(t *testing.T) {
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

	token := "receiver-approval-token-1"
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
	if len(accept.SAS) != 6 {
		t.Fatalf("expected 6-digit SAS, got %q", accept.SAS)
	}

	confirm := func() (int, string) {
		sig := crypto.Sign(clientIdent.PrivateKey, []byte(token+":"+accept.SAS))
		body, _ := json.Marshal(crypto.PairingConfirmPayload{
			DeviceID:     clientIdent.DeviceID,
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
		b, _ := io.ReadAll(r.Body)
		return r.StatusCode, string(b)
	}

	// 1. No approval yet: confirm must NOT pair.
	if code, body := confirm(); code != http.StatusAccepted {
		t.Fatalf("expected 202 pending before approval, got %d (%s)", code, body)
	}
	if _, ok := serverTrust.Get(clientIdent.DeviceID); ok {
		t.Fatal("trust committed without receiver approval")
	}

	// 2. Explicit approval: the same confirm now pairs.
	srv.ApprovePairing(token, true)
	if code, body := confirm(); code != http.StatusOK {
		t.Fatalf("expected 200 after approval, got %d (%s)", code, body)
	}
	if _, ok := serverTrust.Get(clientIdent.DeviceID); !ok {
		t.Fatal("trust not committed after approval")
	}

	// 3. Token is single-use: replaying confirm is rejected.
	if code, body := confirm(); code != http.StatusBadRequest {
		t.Fatalf("expected 400 on token replay, got %d (%s)", code, body)
	}
}

// A retried inbound request from the same peer supersedes the older one: the
// receiver sees exactly one dialog, and the stale token is dead.
func TestSignalingServer_DuplicateInboundRequestSupersedes(t *testing.T) {
	serverIdent, _ := crypto.LoadOrGenerateIdentity("", "Server Node", "linux")
	serverTrust, _ := crypto.NewTrustStore("")
	clientIdent, _ := crypto.LoadOrGenerateIdentity("", "Client Node", "android")

	srv := NewSignalingServer(SignalingServerConfig{Port: 0, Identity: serverIdent, TrustStore: serverTrust})
	ctx := t.Context()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	endpoint := fmt.Sprintf("127.0.0.1:%d", srv.Port())

	postRequest := func(token string) int {
		body, _ := json.Marshal(crypto.PairingRequestPayload{
			DisplayName:  clientIdent.DisplayName,
			Platform:     clientIdent.Platform,
			PublicKey:    hex.EncodeToString(clientIdent.PublicKey),
			PairingToken: token,
		})
		r, err := http.Post(fmt.Sprintf("http://%s/pairing/request", endpoint), "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("pairing request: %v", err)
		}
		defer r.Body.Close()
		_, _ = io.ReadAll(r.Body)
		return r.StatusCode
	}

	if code := postRequest("token-old"); code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", code)
	}
	if code := postRequest("token-new"); code != http.StatusOK {
		t.Fatalf("retry request: expected 200, got %d", code)
	}

	pending := srv.PendingPairings()
	if len(pending) != 1 {
		t.Fatalf("expected exactly 1 pending request, got %d", len(pending))
	}
	if pending[0].Token != "token-new" {
		t.Fatalf("expected newest token to survive, got %q", pending[0].Token)
	}
	if !srv.ApprovePairing("token-old", true) {
		t.Log("stale token correctly unknown after supersede")
	} else {
		t.Fatal("stale token must not be approvable after supersede")
	}
}

// A pairing request from a key the receiver already trusts is redundant: 409
// lets the requester report "already trusted" instead of opening a dialog.
func TestSignalingServer_AlreadyTrustedRequestRejected(t *testing.T) {
	serverIdent, _ := crypto.LoadOrGenerateIdentity("", "Server Node", "linux")
	serverTrust, _ := crypto.NewTrustStore("")
	clientIdent, _ := crypto.LoadOrGenerateIdentity("", "Client Node", "android")
	_ = serverTrust.AddTrusted(crypto.TrustEntry{
		DeviceID:    clientIdent.DeviceID,
		DisplayName: clientIdent.DisplayName,
		Platform:    clientIdent.Platform,
		PublicKey:   clientIdent.PublicKey,
	})

	srv := NewSignalingServer(SignalingServerConfig{Port: 0, Identity: serverIdent, TrustStore: serverTrust})
	ctx := t.Context()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	endpoint := fmt.Sprintf("127.0.0.1:%d", srv.Port())

	body, _ := json.Marshal(crypto.PairingRequestPayload{
		DisplayName:  clientIdent.DisplayName,
		Platform:     clientIdent.Platform,
		PublicKey:    hex.EncodeToString(clientIdent.PublicKey),
		PairingToken: "token-redundant",
	})
	r, err := http.Post(fmt.Sprintf("http://%s/pairing/request", endpoint), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("pairing request: %v", err)
	}
	defer r.Body.Close()
	_, _ = io.ReadAll(r.Body)
	if r.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 already-trusted, got %d", r.StatusCode)
	}
	if len(srv.PendingPairings()) != 0 {
		t.Fatal("redundant request must not create a pending entry")
	}
}

// The receiving user's explicit reject surfaces to the polling requester as a
// rejection and commits nothing.
func TestSignalingServer_ReceiverRejectionSurfaces(t *testing.T) {
	serverIdent, _ := crypto.LoadOrGenerateIdentity("", "Server Node", "linux")
	serverTrust, _ := crypto.NewTrustStore("")
	clientIdent, _ := crypto.LoadOrGenerateIdentity("", "Client Node", "android")

	srv := NewSignalingServer(SignalingServerConfig{Port: 0, Identity: serverIdent, TrustStore: serverTrust})
	ctx := t.Context()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	endpoint := fmt.Sprintf("127.0.0.1:%d", srv.Port())

	token := "receiver-rejects-token"
	reqBody, _ := json.Marshal(crypto.PairingRequestPayload{
		DisplayName:  clientIdent.DisplayName,
		Platform:     clientIdent.Platform,
		PublicKey:    hex.EncodeToString(clientIdent.PublicKey),
		PairingToken: token,
	})
	r, err := http.Post(fmt.Sprintf("http://%s/pairing/request", endpoint), "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("pairing request: %v", err)
	}
	var accept crypto.PairingAcceptPayload
	if err := json.NewDecoder(r.Body).Decode(&accept); err != nil {
		t.Fatalf("decode accept: %v", err)
	}
	r.Body.Close()

	if !srv.ApprovePairing(token, false) {
		t.Fatal("reject decision was not recorded")
	}
	sig := crypto.Sign(clientIdent.PrivateKey, []byte(token+":"+accept.SAS))
	confirmBody, _ := json.Marshal(crypto.PairingConfirmPayload{
		DeviceID:     clientIdent.DeviceID,
		PairingToken: token,
		SAS:          accept.SAS,
		Confirmed:    true,
		Signature:    hex.EncodeToString(sig),
	})
	cr, err := http.Post(fmt.Sprintf("http://%s/pairing/confirm", endpoint), "application/json", bytes.NewReader(confirmBody))
	if err != nil {
		t.Fatalf("pairing confirm: %v", err)
	}
	defer cr.Body.Close()
	cb, _ := io.ReadAll(cr.Body)
	if cr.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 after receiver reject, got %d (%s)", cr.StatusCode, string(cb))
	}
	if _, ok := serverTrust.Get(clientIdent.DeviceID); ok {
		t.Fatal("trust committed despite receiver rejection")
	}
	if len(srv.PendingPairings()) != 0 {
		t.Fatal("rejected request must not linger as pending")
	}
}

// The receiver's reject removes the request from PendingPairings immediately,
// not only after the requester's next confirm poll. Otherwise the local UI
// keeps prompting the receiving user about a decision they already made, for
// as long as the requester takes to poll (up to the token TTL).
func TestSignalingServer_ReceiverRejectRemovesPendingImmediately(t *testing.T) {
	serverIdent, _ := crypto.LoadOrGenerateIdentity("", "Server Node", "linux")
	serverTrust, _ := crypto.NewTrustStore("")
	clientIdent, _ := crypto.LoadOrGenerateIdentity("", "Client Node", "android")

	srv := NewSignalingServer(SignalingServerConfig{Port: 0, Identity: serverIdent, TrustStore: serverTrust})
	ctx := t.Context()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	endpoint := fmt.Sprintf("127.0.0.1:%d", srv.Port())

	token := "receiver-reject-immediate-token"
	reqBody, _ := json.Marshal(crypto.PairingRequestPayload{
		DisplayName:  clientIdent.DisplayName,
		Platform:     clientIdent.Platform,
		PublicKey:    hex.EncodeToString(clientIdent.PublicKey),
		PairingToken: token,
	})
	r, err := http.Post(fmt.Sprintf("http://%s/pairing/request", endpoint), "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("pairing request: %v", err)
	}
	r.Body.Close()

	if !srv.ApprovePairing(token, false) {
		t.Fatal("reject decision was not recorded")
	}
	if got := srv.PendingPairings(); len(got) != 0 {
		t.Fatalf("rejected request still listed as pending: %+v", got)
	}

	// The requester's next confirm still learns a terminal outcome: unknown
	// token answers 400, and nothing is committed.
	sig := crypto.Sign(clientIdent.PrivateKey, []byte(token+":"+"000000"))
	confirmBody, _ := json.Marshal(crypto.PairingConfirmPayload{
		DeviceID:     clientIdent.DeviceID,
		PairingToken: token,
		SAS:          "000000",
		Confirmed:    true,
		Signature:    hex.EncodeToString(sig),
	})
	cr, err := http.Post(fmt.Sprintf("http://%s/pairing/confirm", endpoint), "application/json", bytes.NewReader(confirmBody))
	if err != nil {
		t.Fatalf("pairing confirm: %v", err)
	}
	defer cr.Body.Close()
	if cr.StatusCode != http.StatusBadRequest {
		cb, _ := io.ReadAll(cr.Body)
		t.Fatalf("expected 400 after receiver reject, got %d (%s)", cr.StatusCode, string(cb))
	}
	if _, ok := serverTrust.Get(clientIdent.DeviceID); ok {
		t.Fatal("trust committed despite receiver rejection")
	}
}
