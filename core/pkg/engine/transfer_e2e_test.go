package engine

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/crypto"
	"github.com/om051p/phonebridge/core/pkg/receiver"
	"github.com/om051p/phonebridge/core/pkg/transfer"
)

// TestLinuxToLinux_BidirectionalFileTransferE2E is the Phase 4 Linux↔Linux proof.
//
// It uses the production stack end to end — Ed25519 pairing, LAN signaling,
// Pion WebRTC, the dedicated "transfer" DataChannel — with the only substitution
// being a NullSink instead of a display sink. It asserts what the milestone
// promises: a file crosses in each direction, the bytes on disk match the source
// digest, the peer is attributed on the activity record, and nothing is left in
// the staging directory.
func TestLinuxToLinux_BidirectionalFileTransferE2E(t *testing.T) {
	tmpDir := t.TempDir()

	nodeA := createTestNode(t, tmpDir, "Linux_Xfer_Alpha")
	defer nodeA.Close()

	nodeB := createTestNode(t, tmpDir, "Linux_Xfer_Beta")
	defer nodeB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// 1. Mutual trust: without it the signaling server refuses the session, and
	//    therefore no transfer channel can ever exist. Node B approves explicitly.
	approved := make(chan struct{})
	defer close(approved)
	approveFirstInboundPairing(t, nodeB.sigServer, approved)
	pairClient := crypto.NewPairingClient(3 * time.Second)
	if _, err := pairClient.Pair(ctx, nodeB.endpoint, nodeA.identity, nodeA.trustStore, func(remoteName, sas string) bool {
		return true
	}); err != nil {
		t.Fatalf("pair A->B failed: %v", err)
	}
	if !nodeA.trustStore.IsTrusted(nodeB.id) || !nodeB.trustStore.IsTrusted(nodeA.id) {
		t.Fatal("mutual trust was not established")
	}

	// 2. Node A initiates a session to Node B. A is the driver (its receiver owns
	//    the transfer channel), B is the responder (Its InboundSession creates it).
	sessCfg := DefaultSessionConfig()
	sessCfg.TargetDeviceID = nodeB.id
	sessCfg.Identity = nodeA.identity
	sessCfg.TrustStore = nodeA.trustStore
	sessCfg.ClipboardEngine = nodeA.engine
	sessCfg.TransferEngine = nodeA.transfer
	sessCfg.ConnectTimeout = 4 * time.Second

	sessA := NewSession("sess-xfer-a-to-b", sessCfg, nodeA.discovery.Registry(), nil)
	defer sessA.Stop("test teardown")

	if err := sessA.Connect(ctx, nodeB.endpoint, receiver.NewNullSink()); err != nil {
		t.Fatalf("Connect A->B failed: %v", err)
	}

	waitCond := func(what string, cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			if cond() {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("%s never became ready", what)
	}

	// Both ends must have the transfer channel bound before a file is offered:
	// the engine refuses to send without one rather than queueing silently.
	waitCond("driver transfer channel (A)", sessA.TransferReady)
	waitCond("responder transfer channel (B)", nodeB.transfer.ChannelReady)

	// --- A → B -------------------------------------------------------------
	srcA, digestA := writePseudoRandomFile(t, tmpDir, "alpha-to-beta.bin", 700*1024)
	idA, err := nodeA.transfer.SendFile(ctx, srcA, "alpha-to-beta.bin")
	if err != nil {
		t.Fatalf("send A->B: %v", err)
	}

	senderA := waitForNodeTransfer(t, nodeA.transfer, idA)
	if senderA.State != transfer.StateComplete {
		t.Fatalf("A sender state = %s (%s: %s)", senderA.State, senderA.ReasonCode, senderA.ErrorMessage)
	}
	if senderA.PeerDeviceID != nodeB.id {
		t.Fatalf("A sender peer = %q, want %q", senderA.PeerDeviceID, nodeB.id)
	}

	receiverB := waitForNodeTransfer(t, nodeB.transfer, idA)
	if receiverB.State != transfer.StateComplete {
		t.Fatalf("B receiver state = %s (%s: %s)", receiverB.State, receiverB.ReasonCode, receiverB.ErrorMessage)
	}
	if receiverB.PeerDeviceID != nodeA.id {
		t.Fatalf("B receiver peer = %q, want %q", receiverB.PeerDeviceID, nodeA.id)
	}
	assertFileMatches(t, filepath.Join(nodeB.downloads, receiverB.SavedName), digestA, "B received")

	// --- B → A (the reverse direction over the same session) ---------------
	srcB, digestB := writePseudoRandomFile(t, tmpDir, "beta-to-alpha.bin", 300*1024)
	idB, err := nodeB.transfer.SendFile(ctx, srcB, "beta-to-alpha.bin")
	if err != nil {
		t.Fatalf("send B->A: %v", err)
	}

	senderB := waitForNodeTransfer(t, nodeB.transfer, idB)
	if senderB.State != transfer.StateComplete {
		t.Fatalf("B sender state = %s (%s: %s)", senderB.State, senderB.ReasonCode, senderB.ErrorMessage)
	}
	receiverA := waitForNodeTransfer(t, nodeA.transfer, idB)
	if receiverA.State != transfer.StateComplete {
		t.Fatalf("A receiver state = %s (%s: %s)", receiverA.State, receiverA.ReasonCode, receiverA.ErrorMessage)
	}
	assertFileMatches(t, filepath.Join(nodeA.downloads, receiverA.SavedName), digestB, "A received")

	// --- history and staging hygiene --------------------------------------
	if got := nodeA.transfer.List(); len(got) < 2 {
		t.Fatalf("A history holds %d entries, want at least 2 (one each way)", len(got))
	}
	if got := nodeB.transfer.List(); len(got) < 2 {
		t.Fatalf("B history holds %d entries, want at least 2 (one each way)", len(got))
	}
	assertStagingEmpty(t, nodeA.downloads)
	assertStagingEmpty(t, nodeB.downloads)

	// --- session teardown detaches the channel on both ends (DEC-024) -------
	// There is no resume: after the session ends, a further send must fail fast
	// with a typed code instead of queueing against a transport that is gone.
	t.Run("session stop detaches the transfer channel on both ends", func(t *testing.T) {
		if err := sessA.Stop("transfer test teardown"); err != nil {
			t.Fatalf("stop session: %v", err)
		}
		if sessA.TransferReady() {
			t.Fatal("driver still reports a ready transfer channel after stop")
		}
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) && nodeB.transfer.ChannelReady() {
			time.Sleep(20 * time.Millisecond)
		}
		if nodeB.transfer.ChannelReady() {
			t.Fatal("responder still holds a transfer channel after the peer stopped")
		}

		_, err := nodeA.transfer.SendFile(ctx, srcA, "after-stop.bin")
		if err == nil {
			t.Fatal("send after session stop must fail")
		}
		failure, ok := transfer.IsFailure(err)
		if !ok {
			t.Fatalf("send after stop returned a non-typed error: %v", err)
		}
		if failure.Reason != transfer.ReasonNoSession {
			t.Fatalf("send after stop reason = %s, want %s", failure.Reason, transfer.ReasonNoSession)
		}
	})
}

// writePseudoRandomFile writes a deterministic pseudo-random file so the test
// can assert an exact digest without shipping a fixture.
func writePseudoRandomFile(t *testing.T, dir, name string, size int) (string, [32]byte) {
	t.Helper()
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, sha256.Sum256(buf)
}

// assertFileMatches checks a received file's size and digest.
func assertFileMatches(t *testing.T, path string, want [32]byte, what string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: read %s: %v", what, path, err)
	}
	if sha256.Sum256(got) != want {
		t.Fatalf("%s: digest mismatch at %s", what, path)
	}
	if len(got) == 0 {
		t.Fatalf("%s: file is empty", what)
	}
}

// assertStagingEmpty verifies no partial file survived a completed transfer.
func assertStagingEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, ".phonebridge-partial"))
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("read staging dir: %v", err)
	}
	if len(entries) != 0 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("staging dir %s still holds %v", dir, names)
	}
}

// waitForNodeTransfer waits until the engine reports a final state for id.
func waitForNodeTransfer(t *testing.T, eng *transfer.Engine, id string) transfer.Info {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	var last transfer.Info
	for time.Now().Before(deadline) {
		if info, ok := eng.Get(id); ok {
			last = info
			switch info.State {
			case transfer.StateComplete, transfer.StateCancelled, transfer.StateFailed:
				return info
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("transfer %s did not finish (last state %s, %s)", id, last.State, last.ErrorMessage)
	return transfer.Info{}
}
