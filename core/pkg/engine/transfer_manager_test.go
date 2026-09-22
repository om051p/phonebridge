package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/om051p/phonebridge/core/pkg/transfer"
)

// TestSendFileRequiresALiveSession pins the gate that keeps "send file" from
// becoming a promise the transport cannot keep: without an active session with
// an open transfer channel, the manager must refuse with a typed failure instead
// of queueing the file against a peer that is merely discovered.
func TestSendFileRequiresALiveSession(t *testing.T) {
	tmpDir := t.TempDir()
	dest, err := transfer.NewFileDestination(transfer.FileDestinationConfig{Dir: filepath.Join(tmpDir, "downloads")})
	if err != nil {
		t.Fatal(err)
	}
	eng, err := transfer.NewEngine(transfer.Config{LocalPeerID: "self", Destination: dest})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	src := filepath.Join(tmpDir, "payload.bin")
	if err := os.WriteFile(src, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	mgr := NewSessionManager(DefaultSessionConfig(), nil, nil, nil)

	t.Run("no transfer engine configured", func(t *testing.T) {
		_, err := mgr.SendFile(context.Background(), "", src, "")
		failure, ok := transfer.IsFailure(err)
		if !ok {
			t.Fatalf("want a typed failure, got %v", err)
		}
		if failure.Reason != transfer.ReasonUnsupportedPeer {
			t.Fatalf("reason = %s, want UNSUPPORTED_PEER", failure.Reason)
		}
	})

	mgr.SetTransferEngine(eng)

	t.Run("engine configured but no active session", func(t *testing.T) {
		_, err := mgr.SendFile(context.Background(), "", src, "")
		failure, ok := transfer.IsFailure(err)
		if !ok {
			t.Fatalf("want a typed failure, got %v", err)
		}
		if failure.Reason != transfer.ReasonNoSession {
			t.Fatalf("reason = %s, want NO_SESSION", failure.Reason)
		}
	})

	t.Run("listing and cancelling are safe without a session", func(t *testing.T) {
		if got := mgr.ListTransfers(); len(got) != 0 {
			t.Fatalf("history should be empty, got %d entries", len(got))
		}
		if mgr.TransferEngineReady() {
			t.Fatal("no session is active, so transfer must not report ready")
		}
		if err := mgr.CancelTransfer(context.Background(), "does-not-exist"); err == nil {
			t.Fatal("cancelling an unknown transfer must report an error")
		}
	})
}
