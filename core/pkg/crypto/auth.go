package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"sync"
	"time"
)

const (
	HeaderDeviceID  = "X-PhoneBridge-Device-ID"
	HeaderTimestamp = "X-PhoneBridge-Timestamp"
	HeaderNonce     = "X-PhoneBridge-Nonce"
	HeaderSignature = "X-PhoneBridge-Signature"

	DefaultReplayWindow = 30 * time.Second
)

// NonceCache guards against replay attacks within the clock tolerance window.
type NonceCache struct {
	mu        sync.Mutex
	seen      map[string]time.Time
	lastClean time.Time
}

// NewNonceCache initializes a replay protection cache.
func NewNonceCache() *NonceCache {
	return &NonceCache{
		seen:      make(map[string]time.Time),
		lastClean: time.Now(),
	}
}

// CheckAndRecord returns true if the nonce is fresh, or false if already seen.
func (c *NonceCache) CheckAndRecord(nonce string, now time.Time, ttl time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Periodic cleanup of expired nonces
	if now.Sub(c.lastClean) > ttl {
		cutoff := now.Add(-ttl)
		for k, t := range c.seen {
			if t.Before(cutoff) {
				delete(c.seen, k)
			}
		}
		c.lastClean = now
	}

	if _, exists := c.seen[nonce]; exists {
		return false
	}
	c.seen[nonce] = now
	return true
}

// SignRequest computes cryptographic authentication headers for an outgoing HTTP request.
func SignRequest(identity *DeviceIdentity, method, path string, body []byte) (map[string]string, error) {
	if identity == nil || len(identity.PrivateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid device identity for signing")
	}

	nowMs := time.Now().UnixMilli()

	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	nonce := hex.EncodeToString(nonceBytes)

	bodyHash := sha256.Sum256(body)
	sigMaterial := fmt.Sprintf("%s\n%s\n%d\n%s\n%x", method, path, nowMs, nonce, bodyHash)
	sig := Sign(identity.PrivateKey, []byte(sigMaterial))

	headers := map[string]string{
		HeaderDeviceID:  identity.DeviceID,
		HeaderTimestamp: strconv.FormatInt(nowMs, 10),
		HeaderNonce:     nonce,
		HeaderSignature: hex.EncodeToString(sig),
	}
	return headers, nil
}

// VerifyRequest authenticates an incoming HTTP request against the trust store and replay cache.
func VerifyRequest(
	store *TrustStore,
	method, path string,
	body []byte,
	getHeader func(string) string,
	nonces *NonceCache,
	window time.Duration,
) (string, error) {
	if store == nil {
		return "", fmt.Errorf("trust store is nil")
	}
	if window <= 0 {
		window = DefaultReplayWindow
	}

	deviceID := getHeader(HeaderDeviceID)
	tsStr := getHeader(HeaderTimestamp)
	nonce := getHeader(HeaderNonce)
	sigHex := getHeader(HeaderSignature)

	if deviceID == "" || tsStr == "" || nonce == "" || sigHex == "" {
		return "", fmt.Errorf("missing authentication headers")
	}

	// 1. Verify Timestamp within replay window
	tsMs, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid timestamp header: %w", err)
	}
	now := time.Now()
	msgTime := time.UnixMilli(tsMs)
	diff := now.Sub(msgTime)
	if diff < -window || diff > window {
		return "", fmt.Errorf("%w: clock difference %v exceeds window %v", ErrExpired, diff, window)
	}

	// 2. Verify Nonce against replay
	if nonces != nil {
		if !nonces.CheckAndRecord(nonce, now, window*2) {
			return "", fmt.Errorf("%w: nonce %s already seen", ErrReplay, nonce)
		}
	}

	// 3. Verify Trust Store & Device Identity
	entry, ok := store.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrUntrusted, deviceID)
	}
	if entry.Revoked {
		return "", fmt.Errorf("%w: %s", ErrRevoked, deviceID)
	}
	if len(entry.PublicKey) != ed25519.PublicKeySize {
		return "", fmt.Errorf("corrupted public key for device %s", deviceID)
	}

	// 4. Verify Cryptographic Signature
	sig, err := hex.DecodeString(sigHex)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return "", fmt.Errorf("malformed signature header")
	}

	bodyHash := sha256.Sum256(body)
	sigMaterial := fmt.Sprintf("%s\n%s\n%d\n%s\n%x", method, path, tsMs, nonce, bodyHash)

	if !Verify(entry.PublicKey, []byte(sigMaterial), sig) {
		return "", fmt.Errorf("%w: verification failed for %s", ErrSignature, deviceID)
	}

	return deviceID, nil
}
