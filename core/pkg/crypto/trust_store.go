package crypto

import (
	"crypto/ed25519"
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
func (s *TrustStore) AddTrusted(entry TrustEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(entry.PublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key size: %d", len(entry.PublicKey))
	}
	if entry.DeviceID == "" {
		entry.DeviceID = Fingerprint(entry.PublicKey)
	}
	if entry.PairedAt.IsZero() {
		entry.PairedAt = time.Now()
	}
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
