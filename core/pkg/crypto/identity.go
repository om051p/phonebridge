package crypto

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// DeviceIdentity represents this device's persistent cryptographic identity.
type DeviceIdentity struct {
	DeviceID    string             `json:"device_id"`
	DisplayName string             `json:"display_name"`
	Platform    string             `json:"platform"`
	PublicKey   ed25519.PublicKey  `json:"public_key"`
	PrivateKey  ed25519.PrivateKey `json:"private_key"`
}

type identityStorage struct {
	DeviceID    string `json:"device_id"`
	DisplayName string `json:"display_name"`
	Platform    string `json:"platform"`
	PublicKey   string `json:"public_key"`  // hex
	PrivateKey  string `json:"private_key"` // hex
}

// Fingerprint derives a stable 64-char hex SHA-256 fingerprint from an Ed25519 public key.
func Fingerprint(pub ed25519.PublicKey) string {
	h := sha256.Sum256(pub)
	return hex.EncodeToString(h[:])
}

// GenerateIdentity creates a fresh ephemeral or initial Ed25519 device identity.
func GenerateIdentity(displayName, platform string) (*DeviceIdentity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519 key: %w", err)
	}
	devID := Fingerprint(pub)
	return &DeviceIdentity{
		DeviceID:    devID,
		DisplayName: displayName,
		Platform:    platform,
		PublicKey:   pub,
		PrivateKey:  priv,
	}, nil
}

// LoadOrGenerateIdentity loads an identity from the given path or creates a new one at 0600.
func LoadOrGenerateIdentity(path, displayName, platform string) (*DeviceIdentity, error) {
	if path == "" {
		return GenerateIdentity(displayName, platform)
	}

	if data, err := os.ReadFile(path); err == nil {
		var s identityStorage
		if err := json.Unmarshal(data, &s); err == nil && s.DeviceID != "" {
			pubBytes, errPub := hex.DecodeString(s.PublicKey)
			privBytes, errPriv := hex.DecodeString(s.PrivateKey)
			if errPub == nil && errPriv == nil && len(pubBytes) == ed25519.PublicKeySize && len(privBytes) == ed25519.PrivateKeySize {
				return &DeviceIdentity{
					DeviceID:    s.DeviceID,
					DisplayName: s.DisplayName,
					Platform:    s.Platform,
					PublicKey:   ed25519.PublicKey(pubBytes),
					PrivateKey:  ed25519.PrivateKey(privBytes),
				}, nil
			}
		}
	}

	// Generate fresh identity
	id, err := GenerateIdentity(displayName, platform)
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("mkdir identity dir: %w", err)
	}

	s := identityStorage{
		DeviceID:    id.DeviceID,
		DisplayName: id.DisplayName,
		Platform:    id.Platform,
		PublicKey:   hex.EncodeToString(id.PublicKey),
		PrivateKey:  hex.EncodeToString(id.PrivateKey),
	}
	bytesData, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal identity: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, bytesData, 0600); err != nil {
		return nil, fmt.Errorf("write identity tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return nil, fmt.Errorf("commit identity file: %w", err)
	}

	return id, nil
}

// Sign creates an Ed25519 signature over message.
func Sign(priv ed25519.PrivateKey, message []byte) []byte {
	return ed25519.Sign(priv, message)
}

// Verify checks an Ed25519 signature.
func Verify(pub ed25519.PublicKey, message, sig []byte) bool {
	if len(pub) != ed25519.PublicKeySize || len(sig) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(pub, message, sig)
}

// CalculateSAS computes a deterministic human-verifiable Short Authentication String (6-digit PIN).
// Sorts keys lexicographically so both initiator and responder calculate the exact same PIN.
func CalculateSAS(localPub, remotePub []byte, token string) string {
	if len(localPub) == 0 || len(remotePub) == 0 {
		return "000000"
	}

	k1, k2 := localPub, remotePub
	if bytes.Compare(k1, k2) > 0 {
		k1, k2 = k2, k1
	}

	h := sha256.New()
	h.Write(k1)
	h.Write(k2)
	h.Write([]byte(token))
	digest := h.Sum(nil)

	val := binary.BigEndian.Uint32(digest[:4]) % 1000000
	return fmt.Sprintf("%06d", val)
}

// DefaultIdentityPath returns the OS-appropriate identity storage path on Linux.
func DefaultIdentityPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "phonebridge", "identity.json")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), fmt.Sprintf("phonebridge-%d-identity.json", os.Getuid()))
	}
	return filepath.Join(home, ".config", "phonebridge", "identity.json")
}

// DefaultTrustStorePath returns the OS-appropriate trust store path on Linux.
func DefaultTrustStorePath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "phonebridge", "trusted_devices.json")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), fmt.Sprintf("phonebridge-%d-trust.json", os.Getuid()))
	}
	return filepath.Join(home, ".config", "phonebridge", "trusted_devices.json")
}

var (
	ErrUntrusted = errors.New("device is not trusted")
	ErrRevoked   = errors.New("device trust has been revoked")
	ErrReplay    = errors.New("replayed authentication request detected")
	ErrExpired   = errors.New("request timestamp expired or skewed")
	ErrSignature = errors.New("cryptographic signature verification failed")
)
