package clipboard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"google.golang.org/protobuf/proto"
)

// fakeClock implements Clock for deterministic testing without sleeping.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(t time.Time) *fakeClock {
	return &fakeClock{now: t}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// ---------------------------------------------------------------------------
// 1. MIME Normalization Tests
// ---------------------------------------------------------------------------

func TestMIMENormalization(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{
			name:    "valid text/plain",
			input:   "text/plain",
			want:    CanonicalMIME,
			wantErr: nil,
		},
		{
			name:    "valid text/plain;charset=utf-8",
			input:   "text/plain;charset=utf-8",
			want:    CanonicalMIME,
			wantErr: nil,
		},
		{
			name:    "valid text/plain with space and charset",
			input:   "text/plain; charset=utf-8",
			want:    CanonicalMIME,
			wantErr: nil,
		},
		{
			name:    "valid text/plain uppercase",
			input:   "TEXT/PLAIN; CHARSET=UTF-8",
			want:    CanonicalMIME,
			wantErr: nil,
		},
		{
			name:    "valid text/plain utf8 without dash",
			input:   "text/plain; charset=utf8",
			want:    CanonicalMIME,
			wantErr: nil,
		},
		{
			name:    "unsupported HTML",
			input:   "text/html",
			wantErr: ErrUnsupportedMIME,
		},
		{
			name:    "unsupported URI list",
			input:   "text/uri-list",
			wantErr: ErrUnsupportedMIME,
		},
		{
			name:    "unsupported image MIME",
			input:   "image/png",
			wantErr: ErrUnsupportedMIME,
		},
		{
			name:    "unsupported binary MIME",
			input:   "application/octet-stream",
			wantErr: ErrUnsupportedMIME,
		},
		{
			name:    "unsupported ISO charset",
			input:   "text/plain; charset=iso-8859-1",
			wantErr: ErrUnsupportedMIME,
		},
		{
			name:    "unsupported UTF-16 charset",
			input:   "text/plain; charset=utf-16",
			wantErr: ErrUnsupportedMIME,
		},
		{
			name:    "unsupported extra parameter",
			input:   "text/plain; charset=utf-8; format=flowed",
			wantErr: ErrUnsupportedMIME,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: ErrUnsupportedMIME,
		},
		{
			name:    "whitespace string",
			input:   "   ",
			wantErr: ErrUnsupportedMIME,
		},
		{
			name:    "malformed MIME syntax",
			input:   "text;;;",
			wantErr: ErrUnsupportedMIME,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeMIME(tt.input)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("NormalizeMIME(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				}
				if IsSupportedMIME(tt.input) {
					t.Fatalf("IsSupportedMIME(%q) = true, want false", tt.input)
				}
			} else {
				if err != nil {
					t.Fatalf("NormalizeMIME(%q) unexpected error: %v", tt.input, err)
				}
				if got != tt.want {
					t.Fatalf("NormalizeMIME(%q) = %q, want %q", tt.input, got, tt.want)
				}
				if !IsSupportedMIME(tt.input) {
					t.Fatalf("IsSupportedMIME(%q) = false, want true", tt.input)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 2. Payload Enforcement Tests
// ---------------------------------------------------------------------------

func TestPayloadEnforcement(t *testing.T) {
	t.Run("empty payload accepted", func(t *testing.T) {
		item, err := NewItem("text/plain", []byte{}, 1000)
		if err != nil {
			t.Fatalf("unexpected error for empty payload: %v", err)
		}
		if len(item.Payload) != 0 {
			t.Fatalf("expected 0 bytes, got %d", len(item.Payload))
		}
	})

	t.Run("1-byte payload accepted", func(t *testing.T) {
		item, err := NewItem("text/plain", []byte{0x41}, 1000)
		if err != nil {
			t.Fatalf("unexpected error for 1-byte payload: %v", err)
		}
		if len(item.Payload) != 1 {
			t.Fatalf("expected 1 byte, got %d", len(item.Payload))
		}
	})

	t.Run("exact 786432-byte payload accepted", func(t *testing.T) {
		payload := make([]byte, MaxPayloadSize)
		item, err := NewItem("text/plain", payload, 1000)
		if err != nil {
			t.Fatalf("unexpected error for exact 786432 bytes: %v", err)
		}
		if len(item.Payload) != MaxPayloadSize {
			t.Fatalf("expected %d bytes, got %d", MaxPayloadSize, len(item.Payload))
		}
	})

	t.Run("786433-byte payload rejected", func(t *testing.T) {
		payload := make([]byte, MaxPayloadSize+1)
		item, err := NewItem("text/plain", payload, 1000)
		if err == nil {
			t.Fatalf("expected error for 786433 bytes, got nil")
		}
		if !errors.Is(err, ErrPayloadTooLarge) {
			t.Fatalf("expected ErrPayloadTooLarge, got %v", err)
		}
		var oversized *OversizedPayloadError
		if !errors.As(err, &oversized) {
			t.Fatalf("expected *OversizedPayloadError, got %T", err)
		}
		if oversized.Size != MaxPayloadSize+1 || oversized.MaxSize != MaxPayloadSize {
			t.Fatalf("unexpected oversized details: size=%d max=%d", oversized.Size, oversized.MaxSize)
		}
		if item != nil {
			t.Fatalf("expected nil item on rejection")
		}
	})

	t.Run("OnLocalCopy enforces payload limit and triggers hook", func(t *testing.T) {
		var hookCalled atomic.Bool
		var hookSize atomic.Int64

		engine, err := NewEngine(EngineConfig{
			Role: RoleDesktop,
			OnOversizedPayload: func(size int) {
				hookCalled.Store(true)
				hookSize.Store(int64(size))
			},
		})
		if err != nil {
			t.Fatalf("NewEngine failed: %v", err)
		}

		payload := make([]byte, MaxPayloadSize+100)
		item, err := engine.OnLocalCopy(context.Background(), "text/plain", payload, 1000)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !errors.Is(err, ErrPayloadTooLarge) {
			t.Fatalf("expected ErrPayloadTooLarge, got %v", err)
		}
		if item != nil {
			t.Fatalf("expected nil item")
		}
		if !hookCalled.Load() {
			t.Fatalf("expected OnOversizedPayload hook to be called")
		}
		if hookSize.Load() != int64(MaxPayloadSize+100) {
			t.Fatalf("hook received size %d, want %d", hookSize.Load(), MaxPayloadSize+100)
		}
	})

	t.Run("Oversized payload never reaches platform adapter", func(t *testing.T) {
		var platformCalled atomic.Bool
		platform := PlatformAdapterFunc(func(ctx context.Context, item *Item) error {
			platformCalled.Store(true)
			return nil
		})

		engine, err := NewEngine(EngineConfig{
			Role:     RoleDesktop,
			Platform: platform,
		})
		if err != nil {
			t.Fatalf("NewEngine failed: %v", err)
		}

		oversizedPayload := make([]byte, MaxPayloadSize+10)
		digest := sha256.Sum256(oversizedPayload)
		update := &phonebridgev1.ClipboardUpdate{
			MimeType:     MIMETextPlainUTF8,
			Payload:      oversizedPayload,
			Sha256Digest: digest[:],
			CopiedAtMs:   1000,
		}

		err = engine.OnRemoteClipboard(context.Background(), update)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !errors.Is(err, ErrPayloadTooLarge) {
			t.Fatalf("expected ErrPayloadTooLarge, got %v", err)
		}
		if platformCalled.Load() {
			t.Fatalf("platform adapter must never be called with oversized payload")
		}
	})
}

// ---------------------------------------------------------------------------
// 3. SHA-256 Digest Tests
// ---------------------------------------------------------------------------

func TestSHA256(t *testing.T) {
	t.Run("digest matches standard sha256.Sum256", func(t *testing.T) {
		payload := []byte("PhoneBridge Clipboard Test Payload")
		got := ComputeDigest(payload)
		want := sha256.Sum256(payload)
		if got != want {
			t.Fatalf("ComputeDigest = %x, want %x", got, want)
		}
	})

	t.Run("digest independent of MIME and timestamp", func(t *testing.T) {
		payload := []byte("Consistent Payload")
		item1, err := NewItem("text/plain", payload, 100)
		if err != nil {
			t.Fatalf("NewItem 1 failed: %v", err)
		}
		item2, err := NewItem("text/plain;charset=utf-8", payload, 99999)
		if err != nil {
			t.Fatalf("NewItem 2 failed: %v", err)
		}
		if item1.Digest != item2.Digest {
			t.Fatalf("digests should be identical: %x vs %x", item1.Digest, item2.Digest)
		}
	})

	t.Run("validate digest success", func(t *testing.T) {
		payload := []byte("Valid Payload")
		digest := sha256.Sum256(payload)
		if err := ValidateDigest(payload, digest[:]); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid digest length rejected", func(t *testing.T) {
		payload := []byte("Test")
		shortDigest := make([]byte, 31)
		longDigest := make([]byte, 33)
		nilDigest := []byte(nil)

		if err := ValidateDigest(payload, shortDigest); !errors.Is(err, ErrInvalidDigestLength) {
			t.Fatalf("expected ErrInvalidDigestLength for 31 bytes, got %v", err)
		}
		if err := ValidateDigest(payload, longDigest); !errors.Is(err, ErrInvalidDigestLength) {
			t.Fatalf("expected ErrInvalidDigestLength for 33 bytes, got %v", err)
		}
		if err := ValidateDigest(payload, nilDigest); !errors.Is(err, ErrInvalidDigestLength) {
			t.Fatalf("expected ErrInvalidDigestLength for nil, got %v", err)
		}
	})

	t.Run("invalid digest content rejected", func(t *testing.T) {
		payload := []byte("Test")
		wrongDigest := sha256.Sum256([]byte("Different Content"))
		if err := ValidateDigest(payload, wrongDigest[:]); !errors.Is(err, ErrInvalidDigest) {
			t.Fatalf("expected ErrInvalidDigest, got %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// 4. Echo Suppression Tests
// ---------------------------------------------------------------------------

func TestEchoSuppression(t *testing.T) {
	start := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)
	filter := NewEchoFilter(32, 5000*time.Millisecond, clock)

	t.Run("first event not suppressed", func(t *testing.T) {
		digest := sha256.Sum256([]byte("clip 1"))
		if filter.IsEcho(digest) {
			t.Fatalf("first event should not be suppressed")
		}
	})

	t.Run("duplicate event within TTL is suppressed", func(t *testing.T) {
		digest := sha256.Sum256([]byte("clip 2"))
		filter.Record(digest)

		// Immediately check
		if !filter.IsEcho(digest) {
			t.Fatalf("duplicate event should be suppressed")
		}

		// Advance within TTL (e.g. 2500 ms)
		clock.Advance(2500 * time.Millisecond)
		if !filter.IsEcho(digest) {
			t.Fatalf("duplicate event within TTL (2500ms) should be suppressed")
		}

		// Advance to exactly 5000 ms
		clock.Advance(2500 * time.Millisecond)
		if !filter.IsEcho(digest) {
			t.Fatalf("duplicate event at exactly 5000ms should be suppressed")
		}
	})

	t.Run("TTL expiration allows re-recording", func(t *testing.T) {
		digest := sha256.Sum256([]byte("clip 3"))
		filter.Record(digest)

		// Advance past TTL (5001 ms)
		clock.Advance(5001 * time.Millisecond)
		if filter.IsEcho(digest) {
			t.Fatalf("event past TTL should not be suppressed")
		}

		// Now re-recording should work
		filter.Record(digest)
		if !filter.IsEcho(digest) {
			t.Fatalf("re-recorded event should be suppressed")
		}
	})

	t.Run("capacity eviction evicts oldest", func(t *testing.T) {
		filter.Clear()
		var digests [33][32]byte
		for i := 0; i < 33; i++ {
			digests[i] = sha256.Sum256([]byte(fmt.Sprintf("capacity item %d", i)))
		}

		// Add 32 items
		for i := 0; i < 32; i++ {
			filter.Record(digests[i])
			clock.Advance(10 * time.Millisecond)
		}

		if filter.Len() != 32 {
			t.Fatalf("expected 32 items, got %d", filter.Len())
		}

		// All 32 should be present
		for i := 0; i < 32; i++ {
			if !filter.IsEcho(digests[i]) {
				t.Fatalf("digest %d should be in filter", i)
			}
		}

		// Add 33rd item: digest 0 should be evicted as the oldest
		filter.Record(digests[32])

		if filter.Len() != 32 {
			t.Fatalf("expected capacity to remain 32, got %d", filter.Len())
		}

		if filter.IsEcho(digests[0]) {
			t.Fatalf("oldest digest 0 should have been evicted")
		}
		if !filter.IsEcho(digests[32]) {
			t.Fatalf("newest digest 32 should be present")
		}
		// digests 1..31 should still be present
		for i := 1; i < 33; i++ {
			if !filter.IsEcho(digests[i]) {
				t.Fatalf("digest %d should still be present", i)
			}
		}
	})

	t.Run("concurrent suppression access", func(t *testing.T) {
		filter.Clear()
		var wg sync.WaitGroup
		concurrency := 50
		iterations := 100

		for c := 0; c < concurrency; c++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for i := 0; i < iterations; i++ {
					d := sha256.Sum256([]byte(fmt.Sprintf("concurrent-%d-%d", id, i)))
					filter.Record(d)
					_ = filter.IsEcho(d)
					_ = filter.Len()
				}
			}(c)
		}
		wg.Wait()

		if filter.Len() > 32 {
			t.Fatalf("filter capacity exceeded: %d > 32", filter.Len())
		}
	})
}

// ---------------------------------------------------------------------------
// 5. Reconnect Arbitration Tests
// ---------------------------------------------------------------------------

func TestReconnectArbitration(t *testing.T) {
	itemA, _ := NewItem("text/plain", []byte("clip A"), 2000)

	t.Run("identical reconnect state (same digest) -> WinnerNone", func(t *testing.T) {
		winner := Arbitrate(itemA, itemA, RoleDesktop)
		if winner != WinnerNone {
			t.Fatalf("expected WinnerNone, got %v", winner)
		}
		winner = Arbitrate(itemA, itemA, RoleMobile)
		if winner != WinnerNone {
			t.Fatalf("expected WinnerNone, got %v", winner)
		}
	})

	t.Run("remote newer state (>1000 ms newer) -> WinnerRemote", func(t *testing.T) {
		local, _ := NewItem("text/plain", []byte("old clip"), 1000)
		remote, _ := NewItem("text/plain", []byte("new clip"), 2001) // 1001 ms newer

		if w := Arbitrate(local, remote, RoleDesktop); w != WinnerRemote {
			t.Fatalf("Desktop role: expected WinnerRemote, got %v", w)
		}
		if w := Arbitrate(local, remote, RoleMobile); w != WinnerRemote {
			t.Fatalf("Mobile role: expected WinnerRemote, got %v", w)
		}
	})

	t.Run("local newer state (>1000 ms newer) -> WinnerLocal", func(t *testing.T) {
		local, _ := NewItem("text/plain", []byte("new clip"), 3001) // 1001 ms newer
		remote, _ := NewItem("text/plain", []byte("old clip"), 2000)

		if w := Arbitrate(local, remote, RoleDesktop); w != WinnerLocal {
			t.Fatalf("Desktop role: expected WinnerLocal, got %v", w)
		}
		if w := Arbitrate(local, remote, RoleMobile); w != WinnerLocal {
			t.Fatalf("Mobile role: expected WinnerLocal, got %v", w)
		}
	})

	t.Run("timestamp exactly 1000 ms apart -> Linux tie-break", func(t *testing.T) {
		// remote is 1000 ms newer: NOT > 1000 ms newer -> tie
		local, _ := NewItem("text/plain", []byte("clip 1"), 1000)
		remote, _ := NewItem("text/plain", []byte("clip 2"), 2000)

		if w := Arbitrate(local, remote, RoleDesktop); w != WinnerLocal {
			t.Fatalf("Desktop peer: expected WinnerLocal, got %v", w)
		}
		if w := Arbitrate(local, remote, RoleMobile); w != WinnerRemote {
			t.Fatalf("Mobile peer: expected WinnerRemote (Desktop wins), got %v", w)
		}
	})

	t.Run("timestamp within 1000 ms -> Linux tie-break", func(t *testing.T) {
		// 500 ms diff
		local, _ := NewItem("text/plain", []byte("clip 1"), 1500)
		remote, _ := NewItem("text/plain", []byte("clip 2"), 2000)

		if w := Arbitrate(local, remote, RoleDesktop); w != WinnerLocal {
			t.Fatalf("Desktop peer: expected WinnerLocal, got %v", w)
		}
		if w := Arbitrate(local, remote, RoleMobile); w != WinnerRemote {
			t.Fatalf("Mobile peer: expected WinnerRemote (Desktop wins), got %v", w)
		}
	})

	t.Run("equal timestamps -> Linux tie-break", func(t *testing.T) {
		local, _ := NewItem("text/plain", []byte("clip 1"), 5000)
		remote, _ := NewItem("text/plain", []byte("clip 2"), 5000)

		if w := Arbitrate(local, remote, RoleDesktop); w != WinnerLocal {
			t.Fatalf("Desktop peer: expected WinnerLocal, got %v", w)
		}
		if w := Arbitrate(local, remote, RoleMobile); w != WinnerRemote {
			t.Fatalf("Mobile peer: expected WinnerRemote (Desktop wins), got %v", w)
		}
	})

	t.Run("zero timestamps", func(t *testing.T) {
		// Both 0 -> tie -> Desktop wins
		local0, _ := NewItem("text/plain", []byte("clip 1"), 0)
		remote0, _ := NewItem("text/plain", []byte("clip 2"), 0)

		if w := Arbitrate(local0, remote0, RoleDesktop); w != WinnerLocal {
			t.Fatalf("Both 0 (Desktop): expected WinnerLocal, got %v", w)
		}
		if w := Arbitrate(local0, remote0, RoleMobile); w != WinnerRemote {
			t.Fatalf("Both 0 (Mobile): expected WinnerRemote, got %v", w)
		}

		// Device restart: local has 0, remote has 5000 -> remote wins (>1000 ms)
		remote5000, _ := NewItem("text/plain", []byte("clip 2"), 5000)
		if w := Arbitrate(local0, remote5000, RoleDesktop); w != WinnerRemote {
			t.Fatalf("Local 0 vs Remote 5000: expected WinnerRemote, got %v", w)
		}

		// Local has 5000, remote has 0 -> local wins (>1000 ms)
		local5000, _ := NewItem("text/plain", []byte("clip 1"), 5000)
		if w := Arbitrate(local5000, remote0, RoleDesktop); w != WinnerLocal {
			t.Fatalf("Local 5000 vs Remote 0: expected WinnerLocal, got %v", w)
		}
	})

	t.Run("empty local state", func(t *testing.T) {
		var localEmpty *Item
		remote, _ := NewItem("text/plain", []byte("remote clip"), 1000)

		if w := Arbitrate(localEmpty, remote, RoleDesktop); w != WinnerRemote {
			t.Fatalf("Empty local (Desktop): expected WinnerRemote, got %v", w)
		}
		if w := Arbitrate(localEmpty, remote, RoleMobile); w != WinnerRemote {
			t.Fatalf("Empty local (Mobile): expected WinnerRemote, got %v", w)
		}
	})

	t.Run("empty remote state", func(t *testing.T) {
		local, _ := NewItem("text/plain", []byte("local clip"), 1000)
		var remoteEmpty *Item

		if w := Arbitrate(local, remoteEmpty, RoleDesktop); w != WinnerLocal {
			t.Fatalf("Empty remote (Desktop): expected WinnerLocal, got %v", w)
		}
		if w := Arbitrate(local, remoteEmpty, RoleMobile); w != WinnerLocal {
			t.Fatalf("Empty remote (Mobile): expected WinnerLocal, got %v", w)
		}
	})

	t.Run("both empty states -> WinnerNone", func(t *testing.T) {
		if w := Arbitrate(nil, nil, RoleDesktop); w != WinnerNone {
			t.Fatalf("expected WinnerNone, got %v", w)
		}
	})

	t.Run("overflow safety with MaxUint64 timestamps", func(t *testing.T) {
		local, _ := NewItem("text/plain", []byte("clip 1"), math.MaxUint64-500)
		remote, _ := NewItem("text/plain", []byte("clip 2"), math.MaxUint64-100)

		// Diff is 400 ms <= 1000 ms -> Desktop wins
		if w := Arbitrate(local, remote, RoleDesktop); w != WinnerLocal {
			t.Fatalf("expected WinnerLocal, got %v", w)
		}
		if w := Arbitrate(local, remote, RoleMobile); w != WinnerRemote {
			t.Fatalf("expected WinnerRemote, got %v", w)
		}
	})

	t.Run("symmetric roles (Linux <-> Linux) deterministic tie-break -> no split-brain", func(t *testing.T) {
		itemA, _ := NewItem("text/plain", []byte("clip from linux A"), 2000)
		itemB, _ := NewItem("text/plain", []byte("clip from linux B"), 2200) // 200 ms diff <= 1000 ms

		// Test with peer IDs: Host A vs Host B
		// Host A evaluates: local=itemA, remote=itemB, localID="host-a", remoteID="host-b"
		wA := ArbitratePeer(itemA, itemB, RoleDesktop, RoleDesktop, "host-a", "host-b")
		// Host B evaluates: local=itemB, remote=itemA, localID="host-b", remoteID="host-a"
		wB := ArbitratePeer(itemB, itemA, RoleDesktop, RoleDesktop, "host-b", "host-a")

		// One must declare WinnerLocal, the other WinnerRemote, so both agree on the same winner
		if !((wA == WinnerLocal && wB == WinnerRemote) || (wA == WinnerRemote && wB == WinnerLocal)) {
			t.Fatalf("Split-brain detected! Host A got %v, Host B got %v", wA, wB)
		}
		if "host-b" > "host-a" {
			if wA != WinnerRemote || wB != WinnerLocal {
				t.Fatalf("Expected host-b to win: Host A got %v, Host B got %v", wA, wB)
			}
		}

		// Test fallback with empty peer IDs (digest comparison)
		wA_digest := ArbitratePeer(itemA, itemB, RoleDesktop, RoleDesktop, "", "")
		wB_digest := ArbitratePeer(itemB, itemA, RoleDesktop, RoleDesktop, "", "")
		if !((wA_digest == WinnerLocal && wB_digest == WinnerRemote) || (wA_digest == WinnerRemote && wB_digest == WinnerLocal)) {
			t.Fatalf("Digest tie-break split brain! Host A got %v, Host B got %v", wA_digest, wB_digest)
		}
	})
}

// ---------------------------------------------------------------------------
// 6. Protobuf Conversion Tests
// ---------------------------------------------------------------------------

func TestProtoConversion(t *testing.T) {
	t.Run("Item to proto and back round-trip", func(t *testing.T) {
		payload := []byte("Hello PhoneBridge")
		original, err := NewItem("text/plain", payload, 123456789)
		if err != nil {
			t.Fatalf("NewItem failed: %v", err)
		}

		pb := original.ToProto()
		if pb.MimeType != CanonicalMIME {
			t.Fatalf("pb.MimeType = %q, want %q", pb.MimeType, CanonicalMIME)
		}
		if !bytes.Equal(pb.Payload, payload) {
			t.Fatalf("payload mismatch")
		}
		if !bytes.Equal(pb.Sha256Digest, original.Digest[:]) {
			t.Fatalf("digest mismatch")
		}
		if pb.CopiedAtMs != 123456789 {
			t.Fatalf("CopiedAtMs mismatch")
		}

		decoded, err := ItemFromProto(pb)
		if err != nil {
			t.Fatalf("ItemFromProto failed: %v", err)
		}

		if !original.Equal(decoded) {
			t.Fatalf("decoded item does not match original")
		}
	})

	t.Run("nil proto rejected", func(t *testing.T) {
		item, err := ItemFromProto(nil)
		if !errors.Is(err, ErrMalformedUpdate) {
			t.Fatalf("expected ErrMalformedUpdate, got %v", err)
		}
		if item != nil {
			t.Fatalf("expected nil item")
		}
	})

	t.Run("oversized proto rejected", func(t *testing.T) {
		oversized := make([]byte, MaxPayloadSize+1)
		digest := sha256.Sum256(oversized)
		pb := &phonebridgev1.ClipboardUpdate{
			MimeType:     MIMETextPlainUTF8,
			Payload:      oversized,
			Sha256Digest: digest[:],
			CopiedAtMs:   1000,
		}

		item, err := ItemFromProto(pb)
		if !errors.Is(err, ErrPayloadTooLarge) {
			t.Fatalf("expected ErrPayloadTooLarge, got %v", err)
		}
		if item != nil {
			t.Fatalf("expected nil item")
		}
	})

	t.Run("invalid MIME in proto rejected", func(t *testing.T) {
		payload := []byte("test")
		digest := sha256.Sum256(payload)
		pb := &phonebridgev1.ClipboardUpdate{
			MimeType:     "text/html",
			Payload:      payload,
			Sha256Digest: digest[:],
			CopiedAtMs:   1000,
		}

		item, err := ItemFromProto(pb)
		if !errors.Is(err, ErrUnsupportedMIME) {
			t.Fatalf("expected ErrUnsupportedMIME, got %v", err)
		}
		if item != nil {
			t.Fatalf("expected nil item")
		}
	})

	t.Run("invalid digest length in proto rejected", func(t *testing.T) {
		payload := []byte("test")
		pb := &phonebridgev1.ClipboardUpdate{
			MimeType:     MIMETextPlainUTF8,
			Payload:      payload,
			Sha256Digest: []byte("short"),
			CopiedAtMs:   1000,
		}

		item, err := ItemFromProto(pb)
		if !errors.Is(err, ErrInvalidDigestLength) {
			t.Fatalf("expected ErrInvalidDigestLength, got %v", err)
		}
		if item != nil {
			t.Fatalf("expected nil item")
		}
	})

	t.Run("invalid digest content in proto rejected", func(t *testing.T) {
		payload := []byte("test")
		wrongDigest := sha256.Sum256([]byte("wrong"))
		pb := &phonebridgev1.ClipboardUpdate{
			MimeType:     MIMETextPlainUTF8,
			Payload:      payload,
			Sha256Digest: wrongDigest[:],
			CopiedAtMs:   1000,
		}

		item, err := ItemFromProto(pb)
		if !errors.Is(err, ErrInvalidDigest) {
			t.Fatalf("expected ErrInvalidDigest, got %v", err)
		}
		if item != nil {
			t.Fatalf("expected nil item")
		}
	})
}

// ---------------------------------------------------------------------------
// 7. Echo Cycle Prevention Tests (Android ↔ Linux)
// ---------------------------------------------------------------------------

func TestZeroEchoCycles(t *testing.T) {
	t.Run("Android -> Linux -> Android zero echo cycle", func(t *testing.T) {
		clock := newFakeClock(time.Now())

		var linuxPlatformWrites atomic.Int32
		var linuxTransportSends atomic.Int32
		var androidTransportSends atomic.Int32

		var linuxEngine *Engine
		var androidEngine *Engine

		// Linux platform adapter: when written to, simulates the Wayland helper
		// detecting the clipboard change and calling linuxEngine.OnLocalClipboard.
		linuxPlatform := PlatformAdapterFunc(func(ctx context.Context, item *Item) error {
			linuxPlatformWrites.Add(1)
			// Wayland change event fired back to engine:
			return linuxEngine.OnLocalClipboard(ctx, item)
		})

		// Linux transport: sends to Android engine
		linuxTransport := TransportFunc(func(ctx context.Context, update *phonebridgev1.ClipboardUpdate) error {
			linuxTransportSends.Add(1)
			return androidEngine.OnRemoteClipboard(ctx, update)
		})

		// Android transport: sends to Linux engine
		androidTransport := TransportFunc(func(ctx context.Context, update *phonebridgev1.ClipboardUpdate) error {
			androidTransportSends.Add(1)
			return linuxEngine.OnRemoteClipboard(ctx, update)
		})

		var err error
		linuxEngine, err = NewEngine(EngineConfig{
			Role:      RoleDesktop,
			Platform:  linuxPlatform,
			Transport: linuxTransport,
			Clock:     clock,
		})
		if err != nil {
			t.Fatalf("NewEngine Linux failed: %v", err)
		}

		androidEngine, err = NewEngine(EngineConfig{
			Role:      RoleMobile,
			Transport: androidTransport,
			Clock:     clock,
		})
		if err != nil {
			t.Fatalf("NewEngine Android failed: %v", err)
		}

		// User copies on Android
		payload := []byte("Copied on Android")
		_, err = androidEngine.OnLocalCopy(context.Background(), "text/plain", payload, 1000)
		if err != nil {
			t.Fatalf("Android OnLocalCopy failed: %v", err)
		}

		// Verifications:
		// 1. Android sent exactly 1 update to Linux
		if androidTransportSends.Load() != 1 {
			t.Fatalf("Android should have sent exactly 1 update, got %d", androidTransportSends.Load())
		}

		// 2. Linux platform received exactly 1 write
		if linuxPlatformWrites.Load() != 1 {
			t.Fatalf("Linux platform should have received exactly 1 write, got %d", linuxPlatformWrites.Load())
		}

		// 3. Linux engine MUST have suppressed the echo from Wayland, sending 0 updates back to Android
		if linuxTransportSends.Load() != 0 {
			t.Fatalf("Linux engine should have suppressed echo; sent %d updates back to Android!", linuxTransportSends.Load())
		}
	})

	t.Run("Linux -> Android -> Linux zero echo cycle", func(t *testing.T) {
		clock := newFakeClock(time.Now())

		var androidPlatformWrites atomic.Int32
		var androidTransportSends atomic.Int32
		var linuxTransportSends atomic.Int32

		var linuxEngine *Engine
		var androidEngine *Engine

		// Android platform adapter: when written to, simulates the companion IME
		// detecting the clipboard change and calling androidEngine.OnLocalClipboard.
		androidPlatform := PlatformAdapterFunc(func(ctx context.Context, item *Item) error {
			androidPlatformWrites.Add(1)
			// Android IME change event fired back to engine:
			return androidEngine.OnLocalClipboard(ctx, item)
		})

		// Android transport: sends to Linux engine
		androidTransport := TransportFunc(func(ctx context.Context, update *phonebridgev1.ClipboardUpdate) error {
			androidTransportSends.Add(1)
			return linuxEngine.OnRemoteClipboard(ctx, update)
		})

		// Linux transport: sends to Android engine
		linuxTransport := TransportFunc(func(ctx context.Context, update *phonebridgev1.ClipboardUpdate) error {
			linuxTransportSends.Add(1)
			return androidEngine.OnRemoteClipboard(ctx, update)
		})

		var err error
		linuxEngine, err = NewEngine(EngineConfig{
			Role:      RoleDesktop,
			Transport: linuxTransport,
			Clock:     clock,
		})
		if err != nil {
			t.Fatalf("NewEngine Linux failed: %v", err)
		}

		androidEngine, err = NewEngine(EngineConfig{
			Role:      RoleMobile,
			Platform:  androidPlatform,
			Transport: androidTransport,
			Clock:     clock,
		})
		if err != nil {
			t.Fatalf("NewEngine Android failed: %v", err)
		}

		// User copies on Linux
		payload := []byte("Copied on Linux")
		_, err = linuxEngine.OnLocalCopy(context.Background(), "text/plain", payload, 2000)
		if err != nil {
			t.Fatalf("Linux OnLocalCopy failed: %v", err)
		}

		// Verifications:
		// 1. Linux sent exactly 1 update to Android
		if linuxTransportSends.Load() != 1 {
			t.Fatalf("Linux should have sent exactly 1 update, got %d", linuxTransportSends.Load())
		}

		// 2. Android platform received exactly 1 write
		if androidPlatformWrites.Load() != 1 {
			t.Fatalf("Android platform should have received exactly 1 write, got %d", androidPlatformWrites.Load())
		}

		// 3. Android engine MUST have suppressed the echo from IME, sending 0 updates back to Linux
		if androidTransportSends.Load() != 0 {
			t.Fatalf("Android engine should have suppressed echo; sent %d updates back to Linux!", androidTransportSends.Load())
		}
	})
}

// ---------------------------------------------------------------------------
// 8. Concurrency and Deadlock Freedom Tests
// ---------------------------------------------------------------------------

func TestEngineConcurrency(t *testing.T) {
	clock := newFakeClock(time.Now())

	var platformCalls atomic.Int64
	var transportCalls atomic.Int64

	platform := PlatformAdapterFunc(func(ctx context.Context, item *Item) error {
		platformCalls.Add(1)
		return nil
	})

	transport := TransportFunc(func(ctx context.Context, update *phonebridgev1.ClipboardUpdate) error {
		transportCalls.Add(1)
		return nil
	})

	engine, err := NewEngine(EngineConfig{
		Role:      RoleDesktop,
		Platform:  platform,
		Transport: transport,
		Clock:     clock,
	})
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	var wg sync.WaitGroup
	numGoroutines := 20
	opsPerGoroutine := 50

	ctx := context.Background()

	// Concurrent local clipboard events
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				payload := []byte(fmt.Sprintf("local-%d-%d", id, i))
				_, _ = engine.OnLocalCopy(ctx, "text/plain", payload, uint64(i*100))
			}
		}(g)
	}

	// Concurrent remote clipboard events
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				payload := []byte(fmt.Sprintf("remote-%d-%d", id, i))
				d := sha256.Sum256(payload)
				up := &phonebridgev1.ClipboardUpdate{
					MimeType:     MIMETextPlainUTF8,
					Payload:      payload,
					Sha256Digest: d[:],
					CopiedAtMs:   uint64(i * 100),
				}
				_ = engine.OnRemoteClipboard(ctx, up)
			}
		}(g)
	}

	// Concurrent state reads
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				_ = engine.CurrentItem()
			}
		}()
	}

	// Concurrent reconnect sync
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				payload := []byte(fmt.Sprintf("sync-%d-%d", id, i))
				d := sha256.Sum256(payload)
				up := &phonebridgev1.ClipboardUpdate{
					MimeType:     MIMETextPlainUTF8,
					Payload:      payload,
					Sha256Digest: d[:],
					CopiedAtMs:   uint64(i * 100),
				}
				_, _ = engine.OnReconnectSync(ctx, up)
			}
		}(g)
	}

	wg.Wait()

	if engine.CurrentItem() == nil {
		t.Fatalf("expected current item to be set")
	}
}

// ---------------------------------------------------------------------------
// 9. Memory Boundedness Tests
// ---------------------------------------------------------------------------

func TestNoUnboundedMemoryGrowth(t *testing.T) {
	clock := newFakeClock(time.Now())
	engine, err := NewEngine(EngineConfig{
		Role:  RoleDesktop,
		Clock: clock,
	})
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	ctx := context.Background()

	// Push 1000 distinct items
	for i := 0; i < 1000; i++ {
		payload := []byte(fmt.Sprintf("item-%d", i))
		_, err := engine.OnLocalCopy(ctx, "text/plain", payload, uint64(i))
		if err != nil {
			t.Fatalf("OnLocalCopy failed at %d: %v", i, err)
		}
	}

	// Suppression filter must never exceed 32 items
	if filterLen := engine.EchoFilter().Len(); filterLen > 32 {
		t.Fatalf("EchoFilter length %d exceeds capacity 32", filterLen)
	}

	// Current item is only a single pointer
	curr := engine.CurrentItem()
	if curr == nil {
		t.Fatalf("expected current item")
	}
	expectedPayload := []byte("item-999")
	if !bytes.Equal(curr.Payload, expectedPayload) {
		t.Fatalf("current item payload mismatch: %s vs %s", curr.Payload, expectedPayload)
	}
}

// ---------------------------------------------------------------------------
// 10. Wire Framing (OnRemoteBytes) Tests
// ---------------------------------------------------------------------------

func TestOnRemoteBytes(t *testing.T) {
	var platformReceived *Item
	platform := PlatformAdapterFunc(func(ctx context.Context, item *Item) error {
		platformReceived = item
		return nil
	})

	engine, err := NewEngine(EngineConfig{
		Role:     RoleDesktop,
		Platform: platform,
	})
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	ctx := context.Background()

	t.Run("direct ClipboardUpdate wire bytes", func(t *testing.T) {
		payload := []byte("Direct update")
		d := sha256.Sum256(payload)
		up := &phonebridgev1.ClipboardUpdate{
			MimeType:     MIMETextPlainUTF8,
			Payload:      payload,
			Sha256Digest: d[:],
			CopiedAtMs:   1000,
		}
		data, err := proto.Marshal(up)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		if err := engine.OnRemoteBytes(ctx, data); err != nil {
			t.Fatalf("OnRemoteBytes failed: %v", err)
		}
		if platformReceived == nil || !bytes.Equal(platformReceived.Payload, payload) {
			t.Fatalf("platform adapter did not receive expected item")
		}
	})

	t.Run("Envelope framed wire bytes", func(t *testing.T) {
		payload := []byte("Framed update")
		d := sha256.Sum256(payload)
		env := &phonebridgev1.Envelope{
			Payload: &phonebridgev1.Envelope_ClipboardUpdate{
				ClipboardUpdate: &phonebridgev1.ClipboardUpdate{
					MimeType:     MIMETextPlainUTF8,
					Payload:      payload,
					Sha256Digest: d[:],
					CopiedAtMs:   2000,
				},
			},
		}
		data, err := proto.Marshal(env)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		if err := engine.OnRemoteBytes(ctx, data); err != nil {
			t.Fatalf("OnRemoteBytes failed: %v", err)
		}
		if platformReceived == nil || !bytes.Equal(platformReceived.Payload, payload) {
			t.Fatalf("platform adapter did not receive expected item")
		}
	})

	t.Run("corrupted bytes rejected", func(t *testing.T) {
		corrupted := []byte{0xFF, 0xFE, 0xFD}
		err := engine.OnRemoteBytes(ctx, corrupted)
		if !errors.Is(err, ErrMalformedUpdate) {
			t.Fatalf("expected ErrMalformedUpdate, got %v", err)
		}
	})
}
