package engine

import (
	"context"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/crypto"
	"github.com/om051p/phonebridge/core/pkg/discovery"
)

// Phase B: a snapshot must report the authoritative peer even before
// discovery resolves it (early DISCOVERING) and for peer-offer sessions that
// never consult the registry.
func TestSession_SnapshotReportsConfiguredTargetBeforeDiscovery(t *testing.T) {
	cfg := DefaultSessionConfig()
	cfg.TargetDeviceID = "explicit-peer-id"
	sess := NewSession("sess-peer", cfg, nil, nil)
	snap := sess.Snapshot()
	if snap.TargetDevice.ID != "explicit-peer-id" {
		t.Fatalf("snapshot must carry configured target, got %q", snap.TargetDevice.ID)
	}
}

// Phase B: SessionEvents must carry the authoritative target so observers
// never infer the peer from trust ordering.
func TestSessionManager_StartSessionEventCarriesTarget(t *testing.T) {
	ident, err := crypto.LoadOrGenerateIdentity("", "Linux Node", "linux")
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	store, err := crypto.NewTrustStore("")
	if err != nil {
		t.Fatalf("trust store: %v", err)
	}
	events := make(chan SessionEvent, 16)
	mgr := NewSessionManager(SessionConfig{Identity: ident, TrustStore: store}, nil, nil, func(e SessionEvent) {
		select {
		case events <- e:
		default:
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := mgr.StartSession(ctx, "phone-B", MediaParams{}); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		select {
		case e := <-events:
			if e.TargetDeviceID != "phone-B" {
				t.Fatalf("event for phone-B carries peer %q", e.TargetDeviceID)
			}
			return
		case <-time.After(100 * time.Millisecond):
			if time.Now().After(deadline) {
				t.Fatal("timed out waiting for session event with target")
			}
		}
	}
}

// Phase H: RemoveDevice deletes the record (Forget), while RevokeDevice
// preserves a revoked row. A live session targeting the device stops in
// both cases.
func TestSessionManager_RemoveVsRevokeSemantics(t *testing.T) {
	newMgr := func(t *testing.T) (*SessionManager, *crypto.TrustStore, string) {
		t.Helper()
		ident, err := crypto.LoadOrGenerateIdentity("", "Linux Node", "linux")
		if err != nil {
			t.Fatalf("identity: %v", err)
		}
		store, err := crypto.NewTrustStore("")
		if err != nil {
			t.Fatalf("trust store: %v", err)
		}
		pub := ident.PublicKey
		_ = pub
		peerIdent, err := crypto.LoadOrGenerateIdentity("", "Phone", "android")
		if err != nil {
			t.Fatalf("peer identity: %v", err)
		}
		if err := store.UpsertCanonical(crypto.TrustEntry{
			DisplayName: peerIdent.DisplayName,
			Platform:    peerIdent.Platform,
			PublicKey:   peerIdent.PublicKey,
		}); err != nil {
			t.Fatalf("seed trust: %v", err)
		}
		mgr := NewSessionManager(SessionConfig{Identity: ident, TrustStore: store}, nil, nil, nil)
		return mgr, store, peerIdent.DeviceID
	}

	t.Run("revoke preserves row", func(t *testing.T) {
		mgr, store, id := newMgr(t)
		if err := mgr.RevokeDevice(id); err != nil {
			t.Fatalf("revoke: %v", err)
		}
		entry, ok := store.Get(id)
		if !ok {
			t.Fatal("revoked row must be preserved")
		}
		if !entry.Revoked {
			t.Fatal("row must be flagged revoked")
		}
	})

	t.Run("remove deletes row", func(t *testing.T) {
		mgr, store, id := newMgr(t)
		if err := mgr.RemoveDevice(id); err != nil {
			t.Fatalf("remove: %v", err)
		}
		if _, ok := store.Get(id); ok {
			t.Fatal("removed row must be gone")
		}
		if len(store.List()) != 0 {
			t.Fatalf("store must be empty, got %d", len(store.List()))
		}
	})
}

// Phase D: stopping a session with no active session is a clean no-op error,
// and repeated stops are idempotent (no transport/sink left behind).
func TestSession_StopIdempotentWithoutSession(t *testing.T) {
	ident, err := crypto.LoadOrGenerateIdentity("", "Linux Node", "linux")
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	store, err := crypto.NewTrustStore("")
	if err != nil {
		t.Fatalf("trust store: %v", err)
	}
	mgr := NewSessionManager(SessionConfig{Identity: ident, TrustStore: store}, nil, nil, nil)
	if err := mgr.StopSession("", "no session"); err == nil {
		t.Fatal("expected no-active-session error")
	}
	reg := discovery.NewDeviceRegistry(discovery.DefaultRegistryConfig(), nil)
	sess := NewSession("sess-stop", DefaultSessionConfig(), reg, nil)
	if err := sess.Stop("first"); err != nil {
		t.Fatalf("first stop: %v", err)
	}
	if err := sess.Stop("second"); err != nil {
		t.Fatalf("second stop must be idempotent: %v", err)
	}
}
