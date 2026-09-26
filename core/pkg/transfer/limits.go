package transfer

import (
	"fmt"
	"time"
)

// FrameVersion is the only TransferFrame format version this build speaks.
// A receiver refuses anything else with CODE_INCOMPATIBLE_VERSION instead of
// guessing field semantics (DEC-024).
const FrameVersion uint32 = 1

// Resource limits (DEC-024). Every value is an application-level choice, not a
// transport ceiling: the negotiated SCTP max message size in the pinned Pion
// build is 1 GiB-1, so the chunk size is about bounded work and memory, not
// about what can cross the wire. 64 KiB matches the DEC-018 bulk-chunk
// precedent and keeps one marshal copy small.
const (
	// DefaultChunkSize is the chunk payload size a sender uses for every chunk
	// except the last.
	DefaultChunkSize = 65536

	// MaxReceiveChunkSize caps the chunk_size a sender may declare; a larger
	// value is refused with CODE_INVALID_ARGUMENT before the transfer starts.
	MaxReceiveChunkSize = 65536

	// MaxFrameBytes caps a single received DataChannel message. A 64 KiB chunk
	// plus framing is ~64 KiB; anything near this bound is a protocol error or
	// an attack, never a legitimate frame.
	MaxFrameBytes = 131072

	// DefaultMaxFileSize is the per-device file-size policy (16 GiB).
	DefaultMaxFileSize uint64 = 16 << 30

	// DefaultHighWatermark is the sender's SCTP buffered-bytes ceiling: the
	// sender stops reading the source file while BufferedAmount is above it.
	DefaultHighWatermark uint64 = 1 << 20 // 1 MiB

	// DefaultLowWatermark is the threshold that signals "drained" (Pion's
	// OnBufferedAmountLow). Keeping it well below the high-watermark avoids
	// oscillation.
	DefaultLowWatermark uint64 = 256 << 10 // 256 KiB

	// DefaultFreeSpaceMargin is how much free space beyond the file itself the
	// receiver requires before accepting an offer.
	DefaultFreeSpaceMargin uint64 = 256 << 20 // 256 MiB

	// DefaultHistoryLimit is how many finished transfers the engine keeps for
	// the UI. Phase 4 does not persist history across daemon restarts.
	DefaultHistoryLimit = 50

	// DefaultProgressInterval throttles progress events so a fast LAN transfer
	// does not flood the UI or the local IPC stream.
	DefaultProgressInterval = 250 * time.Millisecond

	// DefaultOfferTimeout bounds the wait between FileOffer and FileAccept.
	DefaultOfferTimeout = 30 * time.Second

	// DefaultStallTimeout bounds how long a transfer may make no progress while
	// bytes remain.
	DefaultStallTimeout = 30 * time.Second

	// DefaultResultFloorTimeout is the minimum wait for FileResult after
	// FileComplete; the effective timeout also scales with file size (see
	// Engine.resultTimeout), because verifying and promoting a large file is
	// not instantaneous.
	DefaultResultFloorTimeout = 60 * time.Second

	// DefaultResultCapTimeout caps the size-derived FileResult timeout.
	DefaultResultCapTimeout = 5 * time.Minute

	// DefaultVerifyThroughput is the assumed receiver verify+promote throughput
	// used to scale the FileResult timeout with file size.
	DefaultVerifyThroughput uint64 = 100 << 20 // 100 MB/s

	// DefaultOutboundQueueDepth is the max number of queued outbound sends that
	// have not yet started on the wire (see Engine.SendFile). One more may be
	// active on the wire. Bounded to keep small-file bursts from growing
	// unbounded memory/FDs.
	DefaultOutboundQueueDepth = 16
)

// Config configures an Engine. Zero values fall back to the defaults above, so
// a production caller only sets what it deviates from.
type Config struct {
	// LocalPeerID is this device's identity, recorded on events/history.
	LocalPeerID string

	// Destination receives inbound files. Required for receiving; a sender-only
	// engine may leave it nil, and an inbound offer is then refused with
	// CODE_UNAVAILABLE (never silently dropped).
	Destination Destination

	// MaxFileSize is this device's file-size policy (see DefaultMaxFileSize).
	MaxFileSize uint64

	// ChunkSize is the outbound chunk payload size (see DefaultChunkSize).
	ChunkSize int

	// HighWatermark/LowWatermark bound outbound buffered bytes.
	HighWatermark uint64
	LowWatermark  uint64

	OfferTimeout     time.Duration
	StallTimeout     time.Duration
	ResultFloor      time.Duration
	ResultCap        time.Duration
	VerifyThroughput uint64

	// HistoryLimit caps remembered finished transfers (newest first).
	HistoryLimit int

	// ProgressInterval throttles progress events.
	ProgressInterval time.Duration

	// OutboundQueueDepth is the max queued (not yet active) outbound sends.
	// Zero means DefaultOutboundQueueDepth.
	OutboundQueueDepth int

	// Now is the clock seam (tests inject a deterministic clock).
	Now func() time.Time

	// OnEvent receives every state/progress transition. It is called without
	// engine locks held and must not block: the daemon forwards it to the local
	// IPC subscribers and the Android bridge to the platform channel.
	OnEvent func(Event)
}

// withDefaults returns a copy with zero fields replaced by production defaults.
func (c Config) withDefaults() Config {
	if c.MaxFileSize == 0 {
		c.MaxFileSize = DefaultMaxFileSize
	}
	if c.ChunkSize <= 0 {
		c.ChunkSize = DefaultChunkSize
	}
	if c.ChunkSize > MaxReceiveChunkSize {
		c.ChunkSize = MaxReceiveChunkSize
	}
	if c.HighWatermark == 0 {
		c.HighWatermark = DefaultHighWatermark
	}
	if c.LowWatermark == 0 {
		c.LowWatermark = DefaultLowWatermark
	}
	if c.LowWatermark >= c.HighWatermark {
		// A low-watermark at or above the ceiling would never signal a drain.
		c.LowWatermark = c.HighWatermark / 4
		if c.LowWatermark == 0 {
			c.LowWatermark = 1
		}
	}
	if c.OfferTimeout <= 0 {
		c.OfferTimeout = DefaultOfferTimeout
	}
	if c.StallTimeout <= 0 {
		c.StallTimeout = DefaultStallTimeout
	}
	if c.ResultFloor <= 0 {
		c.ResultFloor = DefaultResultFloorTimeout
	}
	if c.ResultCap <= 0 {
		c.ResultCap = DefaultResultCapTimeout
	}
	if c.VerifyThroughput == 0 {
		c.VerifyThroughput = DefaultVerifyThroughput
	}
	if c.HistoryLimit <= 0 {
		c.HistoryLimit = DefaultHistoryLimit
	}
	if c.ProgressInterval <= 0 {
		c.ProgressInterval = DefaultProgressInterval
	}
	if c.OutboundQueueDepth <= 0 {
		c.OutboundQueueDepth = DefaultOutboundQueueDepth
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	return c
}

// validate rejects configurations that cannot work, so a construction mistake
// fails at startup instead of mid-transfer.
func (c Config) validate() error {
	if c.LowWatermark >= c.HighWatermark {
		return fmt.Errorf("transfer: low watermark %d must be below high watermark %d", c.LowWatermark, c.HighWatermark)
	}
	if c.ResultFloor > c.ResultCap {
		return fmt.Errorf("transfer: result floor timeout %v exceeds cap %v", c.ResultFloor, c.ResultCap)
	}
	return nil
}

// resultTimeout scales the FileResult wait with the file size: verifying and
// promoting a large file legitimately takes longer than a small one. The budget
// is twice the time the receiver is expected to need at VerifyThroughput,
// clamped to [ResultFloor, ResultCap].
func (c Config) resultTimeout(size uint64) time.Duration {
	if c.VerifyThroughput == 0 {
		return c.ResultFloor
	}
	scaled := time.Duration(float64(2*size) / float64(c.VerifyThroughput) * float64(time.Second))
	if scaled < c.ResultFloor {
		return c.ResultFloor
	}
	if scaled > c.ResultCap {
		return c.ResultCap
	}
	return scaled
}
