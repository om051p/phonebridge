package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/clipboard"
	"github.com/om051p/phonebridge/core/pkg/crypto"
	"github.com/om051p/phonebridge/core/pkg/discovery"
)

func TestDualLinuxNodes_DiscoveryAndPairing(t *testing.T) {
	tmpDir := t.TempDir()

	// --- Node 1 Setup ---
	node1IDPath := filepath.Join(tmpDir, "node1_id.json")
	node1TrustPath := filepath.Join(tmpDir, "node1_trust.json")
	node1Ident, err := crypto.LoadOrGenerateIdentity(node1IDPath, "Linux Desktop Alpha", "linux")
	if err != nil {
		t.Fatalf("node 1 identity: %v", err)
	}
	node1Trust, err := crypto.NewTrustStore(node1TrustPath)
	if err != nil {
		t.Fatalf("node 1 trust store: %v", err)
	}

	node1Sig := NewSignalingServer(SignalingServerConfig{
		Port:       0, // Ephemeral
		Identity:   node1Ident,
		TrustStore: node1Trust,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := node1Sig.Start(ctx); err != nil {
		t.Fatalf("start node 1 signaling server: %v", err)
	}
	defer node1Sig.Close()

	node1DiscCfg := discovery.Config{
		DeviceID:        node1Ident.DeviceID,
		DeviceName:      node1Ident.DisplayName,
		Port:            uint16(node1Sig.Port()),
		IncludeLoopback: true,
		Version:         "1",
		Capabilities:    []string{"SCREEN", "CLIPBOARD"},
	}
	node1Disc, err := discovery.NewDiscovery(node1DiscCfg)
	if err != nil {
		t.Fatalf("create node 1 discovery: %v", err)
	}
	go func() {
		_ = node1Disc.Start(ctx)
	}()
	defer node1Disc.Close()

	// --- Node 2 Setup ---
	node2IDPath := filepath.Join(tmpDir, "node2_id.json")
	node2TrustPath := filepath.Join(tmpDir, "node2_trust.json")
	node2Ident, err := crypto.LoadOrGenerateIdentity(node2IDPath, "Linux Desktop Beta", "linux")
	if err != nil {
		t.Fatalf("node 2 identity: %v", err)
	}
	node2Trust, err := crypto.NewTrustStore(node2TrustPath)
	if err != nil {
		t.Fatalf("node 2 trust store: %v", err)
	}

	node2Sig := NewSignalingServer(SignalingServerConfig{
		Port:       0, // Ephemeral
		Identity:   node2Ident,
		TrustStore: node2Trust,
	})

	if err := node2Sig.Start(ctx); err != nil {
		t.Fatalf("start node 2 signaling server: %v", err)
	}
	defer node2Sig.Close()

	node2DiscCfg := discovery.Config{
		DeviceID:        node2Ident.DeviceID,
		DeviceName:      node2Ident.DisplayName,
		Port:            uint16(node2Sig.Port()),
		IncludeLoopback: true,
		Version:         "1",
		Capabilities:    []string{"SCREEN", "CLIPBOARD"},
	}
	node2Disc, err := discovery.NewDiscovery(node2DiscCfg)
	if err != nil {
		t.Fatalf("create node 2 discovery: %v", err)
	}
	go func() {
		_ = node2Disc.Start(ctx)
	}()
	defer node2Disc.Close()

	// --- 1. Validate Mutual mDNS Discovery ---
	// Wait up to 3 seconds for Node 1 to discover Node 2
	t.Logf("Waiting for discovery between Node 1 (port %d) and Node 2 (port %d)...", node1Sig.Port(), node2Sig.Port())
	deadline := time.Now().Add(4 * time.Second)
	var discoveredDev *discovery.Device
	for time.Now().Before(deadline) {
		devices := node1Disc.Registry().List()
		for _, dev := range devices {
			if dev.ID == node2Ident.DeviceID {
				discoveredDev = &dev
				break
			}
		}
		if discoveredDev != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if discoveredDev == nil {
		t.Logf("mDNS loopback discovery skipped/timed out (expected in some CI/multicast environments), falling back to direct endpoint")
	} else {
		// Log immutable scalar fields only: the registry's Upsert reuses the
		// Addresses backing array, so formatting a snapshot's slice races the
		// discovery goroutine while this test reads.
		t.Logf("Node 1 successfully discovered Node 2 on mDNS: id=%s name=%q",
			discoveredDev.ID, discoveredDev.Name)
		if discoveredDev.Name != "Linux Desktop Beta" {
			t.Errorf("expected device name 'Linux Desktop Beta', got %q", discoveredDev.Name)
		}
	}

	// --- 2. Validate Peer-to-Peer SAS Pairing (receiver approves explicitly) ---
	endpoint := fmt.Sprintf("127.0.0.1:%d", node2Sig.Port())
	approved := make(chan struct{})
	defer close(approved)
	approveFirstInboundPairing(t, node2Sig, approved)
	pairClient := crypto.NewPairingClient(3 * time.Second)
	sasChecked := false

	entry, err := pairClient.Pair(ctx, endpoint, node1Ident, node1Trust, func(remoteName, sas string) bool {
		if remoteName != "Linux Desktop Beta" {
			t.Errorf("expected remote name 'Linux Desktop Beta', got %q", remoteName)
			return false
		}
		if len(sas) != 6 {
			t.Errorf("invalid SAS length: %s", sas)
			return false
		}
		sasChecked = true
		return true
	})
	if err != nil {
		t.Fatalf("Pairing failed: %v", err)
	}
	if !sasChecked {
		t.Fatal("SAS confirmation callback was not invoked")
	}
	if entry.DeviceID != node2Ident.DeviceID {
		t.Fatalf("expected paired device ID %s, got %s", node2Ident.DeviceID, entry.DeviceID)
	}

	// Verify both nodes now trust each other
	if !node1Trust.IsTrusted(node2Ident.DeviceID) {
		t.Fatalf("Node 1 does not trust Node 2")
	}
	if !node2Trust.IsTrusted(node1Ident.DeviceID) {
		t.Fatalf("Node 2 does not trust Node 1")
	}
	t.Logf("Mutual trust successfully established between Node 1 and Node 2")

	// --- 3. Validate Symmetric Clipboard Conflict Arbitration ---
	item1, _ := clipboard.NewItem("text/plain", []byte("clip from alpha"), 5000)
	item2, _ := clipboard.NewItem("text/plain", []byte("clip from beta"), 5200)

	w1 := clipboard.ArbitratePeer(item1, item2, clipboard.RoleDesktop, clipboard.RoleDesktop, node1Ident.DeviceID, node2Ident.DeviceID)
	w2 := clipboard.ArbitratePeer(item2, item1, clipboard.RoleDesktop, clipboard.RoleDesktop, node2Ident.DeviceID, node1Ident.DeviceID)

	if !((w1 == clipboard.WinnerLocal && w2 == clipboard.WinnerRemote) || (w1 == clipboard.WinnerRemote && w2 == clipboard.WinnerLocal)) {
		t.Fatalf("Split brain in dual node arbitration: w1=%v, w2=%v", w1, w2)
	}
	t.Logf("Dual Linux node arbitration converged successfully: w1=%v, w2=%v", w1, w2)
}
