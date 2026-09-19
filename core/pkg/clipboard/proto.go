package clipboard

import (
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

// ToProto converts the internal Item state into a phonebridge.v1.ClipboardUpdate protobuf message.
// Returns nil if the item is nil.
func (i *Item) ToProto() *phonebridgev1.ClipboardUpdate {
	if i == nil {
		return nil
	}

	payloadCopy := make([]byte, len(i.Payload))
	copy(payloadCopy, i.Payload)

	return &phonebridgev1.ClipboardUpdate{
		MimeType:     i.MimeType,
		Payload:      payloadCopy,
		Sha256Digest: i.DigestSlice(),
		CopiedAtMs:   i.CopiedAtMs,
	}
}

// ItemFromProto parses, validates, and converts a received phonebridge.v1.ClipboardUpdate
// protobuf message into an internal Item.
//
// Validation rules:
//   - pb must not be nil (ErrMalformedUpdate)
//   - Payload size must not exceed 768 KiB / 786,432 bytes (OversizedPayloadError)
//   - MIME type must be supported and is normalized (ErrUnsupportedMIME)
//   - SHA-256 digest length must be exactly 32 bytes (ErrInvalidDigestLength)
//   - SHA-256 digest must match the recomputed digest of raw payload bytes (ErrInvalidDigest)
func ItemFromProto(pb *phonebridgev1.ClipboardUpdate) (*Item, error) {
	if pb == nil {
		return nil, ErrMalformedUpdate
	}

	if len(pb.Payload) > MaxPayloadSize {
		return nil, &OversizedPayloadError{Size: len(pb.Payload), MaxSize: MaxPayloadSize}
	}

	normMime, err := NormalizeMIME(pb.MimeType)
	if err != nil {
		return nil, err
	}

	if err := ValidateDigest(pb.Payload, pb.Sha256Digest); err != nil {
		return nil, err
	}

	var digest [32]byte
	copy(digest[:], pb.Sha256Digest)

	payloadCopy := make([]byte, len(pb.Payload))
	copy(payloadCopy, pb.Payload)

	return &Item{
		MimeType:   normMime,
		Payload:    payloadCopy,
		Digest:     digest,
		CopiedAtMs: pb.CopiedAtMs,
	}, nil
}
