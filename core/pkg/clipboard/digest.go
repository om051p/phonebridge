package clipboard

import (
	"crypto/sha256"
	"crypto/subtle"
)

// ComputeDigest calculates the raw 32-byte SHA-256 digest of the payload.
// Per DEC-023, SHA-256 is computed strictly over the raw payload bytes.
// MIME type and timestamps are NOT included in the digest.
func ComputeDigest(payload []byte) [32]byte {
	return sha256.Sum256(payload)
}

// ValidateDigest checks that the provided digest is exactly 32 bytes and
// matches the SHA-256 calculation over the payload. Comparison is constant-time.
func ValidateDigest(payload []byte, digest []byte) error {
	if len(digest) != 32 {
		return ErrInvalidDigestLength
	}

	expected := sha256.Sum256(payload)
	if subtle.ConstantTimeCompare(digest, expected[:]) != 1 {
		return ErrInvalidDigest
	}

	return nil
}
