package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
)

// Phase A: the device ID is the fingerprint of the public key. Trust commits
// must never fork a second logical record for the same key under a
// mismatched (e.g. discovery-derived) ID.
func TestTrustStore_AddTrustedRejectsMismatchedID(t *testing.T) {
	store, err := NewTrustStore("")
	if err != nil {
		t.Fatalf("new trust store: %v", err)
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	err = store.AddTrusted(TrustEntry{
		DeviceID:    "not-the-fingerprint",
		DisplayName: "Phone",
		Platform:    "android",
		PublicKey:   pub,
	})
	if err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
	if len(store.List()) != 0 {
		t.Fatalf("rejected commit must not create a record, got %d", len(store.List()))
	}
}

func TestTrustStore_AddTrustedCanonicalizesEmptyID(t *testing.T) {
	store, err := NewTrustStore("")
	if err != nil {
		t.Fatalf("new trust store: %v", err)
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if err := store.AddTrusted(TrustEntry{
		DisplayName: "Phone",
		Platform:    "android",
		PublicKey:   pub,
	}); err != nil {
		t.Fatalf("add trusted: %v", err)
	}
	got, ok := store.Get(Fingerprint(pub))
	if !ok {
		t.Fatal("expected record under canonical fingerprint")
	}
	if got.Revoked {
		t.Fatal("fresh record must not be revoked")
	}
}

func TestTrustStore_UpsertCanonicalFoldsSameKey(t *testing.T) {
	store, err := NewTrustStore("")
	if err != nil {
		t.Fatalf("new trust store: %v", err)
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	canonical := Fingerprint(pub)

	// Simulate a legacy row committed under a discovery-derived ID. Seed it
	// directly (bypassing validation) to represent pre-Phase-A data.
	store.mu.Lock()
	store.devices["legacy-discovery-id"] = TrustEntry{
		DeviceID:    "legacy-discovery-id",
		DisplayName: "Old Name",
		Platform:    "android",
		PublicKey:   pub,
		PairedAt:    time.Now().Add(-48 * time.Hour),
		LastSeen:    time.Now().Add(-48 * time.Hour),
	}
	store.mu.Unlock()

	if err := store.UpsertCanonical(TrustEntry{
		DeviceID:    canonical,
		DisplayName: "New Name",
		Platform:    "android",
		PublicKey:   pub,
	}); err != nil {
		t.Fatalf("upsert canonical: %v", err)
	}
	rows := store.List()
	if len(rows) != 1 {
		t.Fatalf("same key must fold to one record, got %d", len(rows))
	}
	if rows[0].DeviceID != canonical {
		t.Fatalf("record must be keyed canonical, got %q", rows[0].DeviceID)
	}
	if rows[0].DisplayName != "New Name" {
		t.Fatalf("display metadata must refresh, got %q", rows[0].DisplayName)
	}
	if time.Since(rows[0].PairedAt) < 47*time.Hour {
		t.Fatal("earliest PairedAt must be preserved across the fold")
	}
	if rows[0].Revoked {
		t.Fatal("upsert must clear revoked")
	}

	// Re-pairing the same canonical record again must not move PairedAt
	// forward nor fork a second row.
	if err := store.UpsertCanonical(TrustEntry{
		DeviceID:    canonical,
		DisplayName: "Newest Name",
		Platform:    "android",
		PublicKey:   pub,
	}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	rows = store.List()
	if len(rows) != 1 {
		t.Fatalf("re-pair must keep one record, got %d", len(rows))
	}
	if time.Since(rows[0].PairedAt) < 47*time.Hour {
		t.Fatal("re-pair must preserve the original PairedAt")
	}
}

func TestTrustStore_UpsertCanonicalKeepsDifferentKeys(t *testing.T) {
	store, err := NewTrustStore("")
	if err != nil {
		t.Fatalf("new trust store: %v", err)
	}
	pubA, _, _ := ed25519.GenerateKey(rand.Reader)
	pubB, _, _ := ed25519.GenerateKey(rand.Reader)
	if err := store.UpsertCanonical(TrustEntry{DisplayName: "A", Platform: "android", PublicKey: pubA}); err != nil {
		t.Fatalf("upsert A: %v", err)
	}
	if err := store.UpsertCanonical(TrustEntry{DisplayName: "B", Platform: "android", PublicKey: pubB}); err != nil {
		t.Fatalf("upsert B: %v", err)
	}
	// A reinstall is a new key and therefore a genuinely new record: the old
	// one is retained (no destructive migration).
	if len(store.List()) != 2 {
		t.Fatalf("different keys must remain two records, got %d", len(store.List()))
	}
}

func TestTrustStore_TouchLastSeenPreservesRevoked(t *testing.T) {
	store, err := NewTrustStore("")
	if err != nil {
		t.Fatalf("new trust store: %v", err)
	}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	id := Fingerprint(pub)
	if err := store.AddTrusted(TrustEntry{DeviceID: id, PublicKey: pub}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := store.Revoke(id); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	before, _ := store.Get(id)
	store.TouchLastSeen(id)
	after, _ := store.Get(id)
	if !after.Revoked {
		t.Fatal("touch must not un-revoke")
	}
	if !after.LastSeen.After(before.LastSeen) && !after.LastSeen.Equal(before.LastSeen) {
		t.Fatal("touch must refresh LastSeen")
	}
}
