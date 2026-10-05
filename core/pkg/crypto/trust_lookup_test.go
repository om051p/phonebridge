package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

// FindByPublicKey lets the pairing receiver detect a re-request from an
// already-trusted device (409 already-trusted) without knowing its device ID:
// the request carries only a public key. A revoked entry must NOT count as
// trusted — a revoked peer re-pairs through the normal approval flow.
func TestTrustStore_FindByPublicKey(t *testing.T) {
	store, err := NewTrustStore("")
	if err != nil {
		t.Fatalf("new trust store: %v", err)
	}

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	entry := TrustEntry{
		DeviceID:    Fingerprint(pub),
		DisplayName: "Phone",
		Platform:    "android",
		PublicKey:   pub,
	}
	if err := store.AddTrusted(entry); err != nil {
		t.Fatalf("add trusted: %v", err)
	}

	found, ok := store.FindByPublicKey(pub)
	if !ok {
		t.Fatal("expected to find entry by public key")
	}
	if found.DeviceID != Fingerprint(pub) {
		t.Fatalf("expected %s, got %s", Fingerprint(pub), found.DeviceID)
	}

	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, ok := store.FindByPublicKey(otherPub); ok {
		t.Fatal("unknown public key must not match")
	}

	if err := store.Revoke(Fingerprint(pub)); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	found, ok = store.FindByPublicKey(pub)
	if !ok {
		t.Fatal("revoked entry must still be found (caller decides trust)")
	}
	if !found.Revoked {
		t.Fatal("expected revoked flag to survive lookup")
	}
}
