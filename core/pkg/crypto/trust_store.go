package crypto

import (
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TrustEntry represents a persisted pairing relationship with a remote device.
type TrustEntry struct {
	DeviceID    string    `json:"device_id"`
	DisplayName string    `json:"display_name"`
	Platform    string    `json:"platform"`
	PublicKey   []byte    `json:"public_key"`
	PairedAt    time.Time `json:"paired_at"`
	LastSeen    time.Time `json:"last_seen"`
	Revoked     bool      `json:"revoked"`
}

type trustEntryStorage struct {
	DeviceID    string `json:"device_id"`
	DisplayName string `json:"display_name"`
	Platform    string `json:"platform"`
	PublicKey   string `json:"public_key"` // hex
	PairedAtMs  int64  `json:"paired_at_ms"`
	LastSeenMs  int64  `json:"last_seen_ms"`
	Revoked     bool   `json:"revoked"`
}

type trustFileStorage struct {
	Version int                 `json:"version"`
	Devices []trustEntryStorage `json:"devices"`
}

// TrustStore maintains verified and persisted peer trust records.
type TrustStore struct {
	mu      sync.RWMutex
	path    string
	devices map[string]TrustEntry
}

// NewTrustStore initializes a trust store from disk or creates an in-memory instance.
func NewTrustStore(path string) (*TrustStore, error) {
	ts := &TrustStore{
		path:    path,
		devices: make(map[string]TrustEntry),
	}

	if path == "" {
		return ts, nil
	}

	if data, err := os.ReadFile(path); err == nil {
		var s trustFileStorage
		if err := json.Unmarshal(data, &s); err == nil {
			for _, d := range s.Devices {
				pub, err := hex.DecodeString(d.PublicKey)
				if err != nil || len(pub) != ed25519.PublicKeySize {
					continue
				}
				ts.devices[d.DeviceID] = TrustEntry{
					DeviceID:    d.DeviceID,
					DisplayName: d.DisplayName,
					Platform:    d.Platform,
					PublicKey:   pub,
					PairedAt:    time.UnixMilli(d.PairedAtMs),
					LastSeen:    time.UnixMilli(d.LastSeenMs),
					Revoked:     d.Revoked,
				}
			}
		}
	}

	return ts, nil
}

// IsTrusted returns true if the device is present, not revoked, and has a valid key.
func (s *TrustStore) IsTrusted(deviceID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.devices[deviceID]
	if !ok {
		return false
	}
	return !entry.Revoked && len(entry.PublicKey) == ed25519.PublicKeySize
}

// Get returns the trust entry for the given device ID.
func (s *TrustStore) Get(deviceID string) (TrustEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.devices[deviceID]
	return entry, ok
}

// AddTrusted inserts or updates a trusted device and commits to disk.
//
// The device ID must be the canonical public-key fingerprint
// (Fingerprint(public key)). An empty DeviceID is canonicalized; a
// non-canonical DeviceID is rejected so a second logical record for the same
// key can never be created under a mismatched ID (connection-audit Phase A).
// Callers that reconcile a legacy or discovery-derived ID must use
// UpsertCanonical instead.
func (s *TrustStore) AddTrusted(entry TrustEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(entry.PublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key size: %d", len(entry.PublicKey))
	}
	canonical := Fingerprint(entry.PublicKey)
	if entry.DeviceID == "" {
		entry.DeviceID = canonical
	}
	if entry.DeviceID != canonical {
		return fmt.Errorf("device_id %q does not match public-key fingerprint %q", entry.DeviceID, canonical)
	}
	if entry.PairedAt.IsZero() {
		entry.PairedAt = time.Now()
	}
	entry.LastSeen = time.Now()
	entry.Revoked = false

	s.devices[entry.DeviceID] = entry
	return s.saveLocked()
}

// UpsertCanonical stores a trusted device under its canonical public-key
// fingerprint, folding any legacy row that holds the same public key under a
// different (e.g. discovery-derived) device ID.
//
// Same-key rows are the same logical device, so the fold preserves the
// earliest PairedAt and refreshes display metadata instead of accumulating a
// duplicate. Rows holding a DIFFERENT public key are never touched: a
// reinstall generates a new key and therefore a genuinely new record, which
// is retained alongside the old one (no destructive migration).
func (s *TrustStore) UpsertCanonical(entry TrustEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(entry.PublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key size: %d", len(entry.PublicKey))
	}
	canonical := Fingerprint(entry.PublicKey)
	earliest := entry.PairedAt
	consider := func(t time.Time) {
		if t.IsZero() {
			return
		}
		if earliest.IsZero() || t.Before(earliest) {
			earliest = t
		}
	}
	for id, existing := range s.devices {
		if id == canonical {
			// Re-pairing the same canonical record must not move its
			// original pairing time forward.
			consider(existing.PairedAt)
			continue
		}
		if len(existing.PublicKey) == len(entry.PublicKey) &&
			subtle.ConstantTimeCompare(existing.PublicKey, entry.PublicKey) == 1 {
			consider(existing.PairedAt)
			delete(s.devices, id)
		}
	}
	entry.DeviceID = canonical
	if earliest.IsZero() {
		earliest = time.Now()
	}
	entry.PairedAt = earliest
	entry.LastSeen = time.Now()
	entry.Revoked = false

	s.devices[entry.DeviceID] = entry
	return s.saveLocked()
}

// Revoke marks a device as revoked and saves to disk.
func (s *TrustStore) Revoke(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.devices[deviceID]
	if !ok {
		return fmt.Errorf("device %s not found in trust store", deviceID)
	}

	entry.Revoked = true
	s.devices[deviceID] = entry
	return s.saveLocked()
}

// Remove completely deletes a device from the trust store.
func (s *TrustStore) Remove(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.devices, deviceID)
	return s.saveLocked()
}

// TouchLastSeen refreshes the LastSeen timestamp of a known record without
// altering its identity. Unlike AddTrusted it never enforces canonical IDs,
// so legacy rows (e.g. committed under a discovery-derived ID before Phase A)
// keep receiving presence updates instead of being frozen by validation.
func (s *TrustStore) TouchLastSeen(deviceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.devices[deviceID]
	if !ok {
		return
	}
	entry.LastSeen = time.Now()
	s.devices[deviceID] = entry
	_ = s.saveLocked()
}

// FindByPublicKey returns the trust entry holding the given public key, if
// any. Revoked entries are returned as well — the caller decides whether a
// revoked match counts (re-pair) or not (already-trusted short-circuit).
func (s *TrustStore) FindByPublicKey(pub []byte) (TrustEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, entry := range s.devices {
		if len(entry.PublicKey) == len(pub) && len(pub) > 0 &&
			subtle.ConstantTimeCompare(entry.PublicKey, pub) == 1 {
			return entry, true
		}
	}
	return TrustEntry{}, false
}

// List returns a snapshot of all entries in the trust store.
func (s *TrustStore) List() []TrustEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]TrustEntry, 0, len(s.devices))
	for _, dev := range s.devices {
		out = append(out, dev)
	}
	return out
}

func (s *TrustStore) saveLocked() error {
	if s.path == "" {
		return nil
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("mkdir trust dir: %w", err)
	}

	storage := trustFileStorage{
		Version: 1,
		Devices: make([]trustEntryStorage, 0, len(s.devices)),
	}

	for _, d := range s.devices {
		storage.Devices = append(storage.Devices, trustEntryStorage{
			DeviceID:    d.DeviceID,
			DisplayName: d.DisplayName,
			Platform:    d.Platform,
			PublicKey:   hex.EncodeToString(d.PublicKey),
			PairedAtMs:  d.PairedAt.UnixMilli(),
			LastSeenMs:  d.LastSeen.UnixMilli(),
			Revoked:     d.Revoked,
		})
	}

	data, err := json.MarshalIndent(storage, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal trust store: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write trust tmp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit trust file: %w", err)
	}

	return nil
}
