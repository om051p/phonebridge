package crypto

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The receiver approves asynchronously: the requester polls confirm through
// 202-pending responses until the approval lands, then pairs. The requester's
// own SAS confirmation still gates everything (reject → no confirm at all).
func TestPairingClient_PollsWhileReceiverPending(t *testing.T) {
	clientIdentity, _ := GenerateIdentity("Linux Client", "linux")
	serverIdentity, _ := GenerateIdentity("Android Server", "android")

	clientStore, _ := NewTrustStore("")
	serverStore, _ := NewTrustStore("")

	var pendingToken string
	var calculatedSAS string
	var confirms atomic.Int32
	approved := make(chan struct{})

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
		n := confirms.Add(1)

		if !confirm.Confirmed || confirm.SAS != calculatedSAS || confirm.PairingToken != pendingToken {
			http.Error(w, "invalid confirmation", http.StatusBadRequest)
			return
		}
		if n <= 2 {
			// Receiver has not approved yet: hold the token.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"status":"pending"}`))
			return
		}

		_ = serverStore.AddTrusted(TrustEntry{
			DeviceID:    confirm.DeviceID,
			DisplayName: clientIdentity.DisplayName,
			Platform:    clientIdentity.Platform,
			PublicKey:   clientIdentity.PublicKey,
		})
		close(approved)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"paired"}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	endpoint := strings.TrimPrefix(srv.URL, "http://")
	client := NewPairingClient(2 * time.Second)
	client.PollInterval = 20 * time.Millisecond

	entry, err := client.Pair(context.Background(), endpoint, clientIdentity, clientStore, func(name, sas string) bool {
		return sas == calculatedSAS
	})
	if err != nil {
		t.Fatalf("Pair failed: %v", err)
	}
	if entry.DeviceID != serverIdentity.DeviceID {
		t.Fatalf("paired device ID mismatch: %s != %s", entry.DeviceID, serverIdentity.DeviceID)
	}
	if got := confirms.Load(); got != 3 {
		t.Fatalf("expected 3 confirm posts (2 pending + 1 paired), got %d", got)
	}
	if !clientStore.IsTrusted(serverIdentity.DeviceID) {
		t.Fatal("server should be trusted in client store")
	}
	select {
	case <-approved:
	default:
		t.Fatal("server never committed trust")
	}
}
