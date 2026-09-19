package clipboard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

// Protocol constants for clipboard synchronization (DEC-023).
const (
	// MaxPayloadSize is the strict application-level ceiling on clipboard payload bytes:
	// 768 KiB (786,432 bytes). Oversized payloads are rejected before transmission.
	MaxPayloadSize = 786432

	// DefaultSuppressionCapacity is the maximum number of recent outbound digests
	// maintained in the LRU suppression cache.
	DefaultSuppressionCapacity = 32

	// DefaultSuppressionTTL is the time-to-live for cached digests (5,000 ms).
	DefaultSuppressionTTL = 5000 * time.Millisecond

	// ArbitrationThresholdMs is the timestamp difference threshold (1,000 ms)
	// used for reconnect conflict arbitration.
	ArbitrationThresholdMs uint64 = 1000
)

// Typed errors for clipboard synchronization.
var (
	// ErrPayloadTooLarge indicates that clipboard data exceeds the 768 KiB limit.
	ErrPayloadTooLarge = errors.New("clipboard payload exceeds 768 KiB limit (786432 bytes)")

	// ErrUnsupportedMIME indicates that the MIME type is not supported in V1.
	ErrUnsupportedMIME = errors.New("unsupported clipboard MIME type")

	// ErrInvalidDigest indicates that the SHA-256 digest does not match the payload.
	ErrInvalidDigest = errors.New("invalid clipboard SHA-256 digest")

	// ErrInvalidDigestLength indicates that the SHA-256 digest is not exactly 32 bytes.
	ErrInvalidDigestLength = errors.New("clipboard SHA-256 digest must be exactly 32 bytes")

	// ErrMalformedUpdate indicates that a received update message is malformed or nil.
	ErrMalformedUpdate = errors.New("malformed clipboard update")
)

// OversizedPayloadError records the observed payload size when the 768 KiB ceiling
// is exceeded, enabling a future UI/local-IPC layer to explicitly offer DEC-012 file transfer.
type OversizedPayloadError struct {
	Size    int
	MaxSize int
}

func (e *OversizedPayloadError) Error() string {
	return fmt.Sprintf("clipboard payload of %d bytes exceeds limit of %d bytes", e.Size, e.MaxSize)
}

func (e *OversizedPayloadError) Is(target error) bool {
	return target == ErrPayloadTooLarge
}

// Role defines the device role for deterministic conflict arbitration.
type Role int

const (
	// RoleDesktop represents a Linux/desktop device (wins arbitration tie-breaks).
	RoleDesktop Role = iota
	// RoleMobile represents an Android/mobile device.
	RoleMobile
)

func (r Role) String() string {
	switch r {
	case RoleDesktop:
		return "Desktop"
	case RoleMobile:
		return "Mobile"
	default:
		return "Unknown"
	}
}

// Winner represents the outcome of reconnect conflict arbitration.
type Winner int

const (
	// WinnerNone indicates a no-op (identical digests or both states empty).
	WinnerNone Winner = iota
	// WinnerLocal indicates the local clipboard state won arbitration.
	WinnerLocal
	// WinnerRemote indicates the remote clipboard state won arbitration.
	WinnerRemote
)

func (w Winner) String() string {
	switch w {
	case WinnerNone:
		return "None"
	case WinnerLocal:
		return "Local"
	case WinnerRemote:
		return "Remote"
	default:
		return "Unknown"
	}
}

// Clock provides an interface for time operations, enabling deterministic testing.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now()
}

// Transport sends clipboard updates over the underlying communication channel
// (e.g. WebRTC DataChannel "clipboard").
type Transport interface {
	SendClipboardUpdate(ctx context.Context, update *phonebridgev1.ClipboardUpdate) error
}

// TransportFunc is an adapter to allow the use of ordinary functions as Transport.
type TransportFunc func(ctx context.Context, update *phonebridgev1.ClipboardUpdate) error

func (f TransportFunc) SendClipboardUpdate(ctx context.Context, update *phonebridgev1.ClipboardUpdate) error {
	return f(ctx, update)
}

// PlatformAdapter writes clipboard data to the host operating system clipboard.
// Implemented later by platform-specific adapters (Wayland helper on Linux,
// companion IME on Android).
type PlatformAdapter interface {
	WriteClipboard(ctx context.Context, item *Item) error
}

// PlatformAdapterFunc is an adapter to allow the use of ordinary functions as PlatformAdapter.
type PlatformAdapterFunc func(ctx context.Context, item *Item) error

func (f PlatformAdapterFunc) WriteClipboard(ctx context.Context, item *Item) error {
	return f(ctx, item)
}

// Item represents an immutable clipboard item in memory.
type Item struct {
	MimeType   string
	Payload    []byte
	Digest     [32]byte
	CopiedAtMs uint64
}

// NewItem creates a new validated Item. It normalizes the MIME type,
// computes the SHA-256 digest, enforces the 768 KiB payload ceiling,
// and copies the payload to ensure immutability.
func NewItem(mimeType string, payload []byte, copiedAtMs uint64) (*Item, error) {
	if len(payload) > MaxPayloadSize {
		return nil, &OversizedPayloadError{Size: len(payload), MaxSize: MaxPayloadSize}
	}

	normMime, err := NormalizeMIME(mimeType)
	if err != nil {
		return nil, err
	}

	digest := sha256.Sum256(payload)
	payloadCopy := make([]byte, len(payload))
	copy(payloadCopy, payload)

	return &Item{
		MimeType:   normMime,
		Payload:    payloadCopy,
		Digest:     digest,
		CopiedAtMs: copiedAtMs,
	}, nil
}

// Equal returns true if both items have identical MIME type, payload, digest,
// and copied_at_ms.
func (i *Item) Equal(other *Item) bool {
	if i == nil && other == nil {
		return true
	}
	if i == nil || other == nil {
		return false
	}
	if i.Digest != other.Digest {
		return false
	}
	if i.MimeType != other.MimeType {
		return false
	}
	if i.CopiedAtMs != other.CopiedAtMs {
		return false
	}
	return bytes.Equal(i.Payload, other.Payload)
}

// EqualDigest returns true if both items have matching SHA-256 digests.
func (i *Item) EqualDigest(other *Item) bool {
	if i == nil && other == nil {
		return true
	}
	if i == nil || other == nil {
		return false
	}
	return i.Digest == other.Digest
}

// IsEmpty returns true if the item is nil or has zero payload bytes.
func (i *Item) IsEmpty() bool {
	return i == nil || len(i.Payload) == 0
}

// DigestSlice returns a copy of the 32-byte digest as a byte slice.
func (i *Item) DigestSlice() []byte {
	out := make([]byte, 32)
	copy(out, i.Digest[:])
	return out
}
