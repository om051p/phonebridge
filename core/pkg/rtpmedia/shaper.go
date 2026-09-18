package rtpmedia

import (
	"sync"
	"time"
)

// Token-bucket burst shaper (DEC-021 obligation 1; Spike 04
// BURST-AND-PSI.md).
//
// Applied per RTP packet at the emission point, BEFORE sequence-number
// assignment, inside the single-writer send loop: packets are only ever
// delayed, never reordered or re-split, so AU ordering, RTP timestamps,
// marker bits, FU-A fragmentation and IDR integrity are preserved by
// construction (spike-validated: shaped runs lost zero packets while
// unshaped wake bursts lost up to 44 packets and produced hard decoder
// errors).
//
// Defaults are the measured parameters, not guesses:
//   - ceiling 4000 kbps: the worst unshaped wake burst was ~6.0 Mbps
//     (5.9x steady), and 4 Mbps eliminated loss on the tested path;
//   - bucket 3000 kbit: ~0.75 s of headroom at the ceiling;
//   - idle-credit cap 500 ms: tokens must NOT accumulate over long silence.
//     Without the cap, tokens banked during a 10 s screen-off would release
//     the wake burst at line rate — defeating the shaper exactly when it
//     matters. The cap bounds worst-case added hold at ~0.9 s (measured max
//     584.5 ms on a 195 KB post-wake IDR).
//
// Wait is the only cost. p50/p90 delay was unchanged by shaping in the
// spike trials; only the wake tail pays.
const (
	DefaultShaperKbps      = 4000
	DefaultShaperBurstKbit = 3000
	DefaultIdleCreditCap   = 500 * time.Millisecond
)

// Shaper is a token bucket sized in bits with a capped idle credit.
// A nil *Shaper or one created with kbps <= 0 passes everything through
// (zero-cost no-op), matching the spike's "shaper disabled" mode.
type Shaper struct {
	mu         sync.Mutex
	capBits    int64
	tokens     int64
	refillBits int64 // bits per second
	last       time.Time
	idleCap    time.Duration
	enabled    bool
	now        func() time.Time
	sleep      func(time.Duration)

	// Counters.
	HeldPackets int64 // packets that waited for tokens
	HeldNs      int64 // total wait time
	MaxHoldNs   int64 // worst single-packet wait
}

// ShaperConfig configures NewShaper. Zero fields fall back to the measured
// DEC-021 defaults; Kbps <= 0 after fallback disables the shaper.
type ShaperConfig struct {
	Kbps      int           // sustained refill rate (0 -> DefaultShaperKbps)
	BurstKbit int           // bucket depth in kbit (0 -> DefaultShaperBurstKbit)
	IdleCap   time.Duration // idle-credit cap (0 -> DefaultIdleCreditCap; negative -> unlimited)
}

// NewShaper returns a shaper with a full bucket.
func NewShaper(cfg ShaperConfig) *Shaper {
	kbps := cfg.Kbps
	if kbps == 0 {
		kbps = DefaultShaperKbps
	}
	burst := cfg.BurstKbit
	if burst == 0 {
		burst = DefaultShaperBurstKbit
	}
	idleCap := cfg.IdleCap
	if idleCap == 0 {
		idleCap = DefaultIdleCreditCap
	}
	if kbps <= 0 {
		return &Shaper{enabled: false}
	}
	return &Shaper{
		capBits:    int64(burst) * 1000,
		tokens:     int64(burst) * 1000, // start full
		refillBits: int64(kbps) * 1000,
		last:       time.Now(),
		idleCap:    idleCap,
		enabled:    true,
		now:        time.Now,
		sleep:      time.Sleep,
	}
}

// Enabled reports whether the shaper actively paces.
func (s *Shaper) Enabled() bool { return s != nil && s.enabled }

// SetClock replaces the clock/sleep pair (test seam). The refill anchor is
// re-based into the new clock's frame: mixing clocks would compute a negative
// elapsed (test clocks have no monotonic reading) and poison the token math.
func (s *Shaper) SetClock(now func() time.Time, sleep func(time.Duration)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now, s.sleep = now, sleep
	if s.now != nil {
		s.last = s.now()
	}
}

// Wait blocks until nBytes of tokens are available and returns how long the
// caller waited. It is safe for concurrent use; in production it is called
// only from the single-writer send loop, so blocking delays without
// reordering. nBytes <= 0 never waits.
//
// A packet larger than the bucket can never wait out its own cost (refill is
// capped at the bucket size), so it pays at most one full bucket and is then
// emitted — Wait always terminates. This cannot occur in production (MTU-
// sized RTP packets are ~9.6 kbit against a 3 Mbit bucket) but keeps the
// primitive safe under arbitrary input.
func (s *Shaper) Wait(nBytes int) time.Duration {
	if !s.Enabled() || nBytes <= 0 {
		return 0
	}
	start := s.now()
	need := int64(nBytes) * 8
	for {
		s.mu.Lock()
		now := s.now()
		elapsed := now.Sub(s.last)
		s.last = now
		// Defensive: a clock that jumps backwards must not poison the bucket.
		if elapsed < 0 {
			elapsed = 0
		}
		// Cap idle credit: see DefaultIdleCreditCap. A negative cap means
		// unlimited accumulation (available for experiments only).
		if s.idleCap > 0 && elapsed > s.idleCap {
			elapsed = s.idleCap
		}
		s.tokens += elapsed.Nanoseconds() * s.refillBits / 1e9
		if s.tokens > s.capBits {
			s.tokens = s.capBits
		}
		// Oversized packets pay at most a full bucket (liveness; see above).
		effNeed := need
		if effNeed > s.capBits {
			effNeed = s.capBits
		}
		if s.tokens >= effNeed {
			s.tokens -= need
			if s.tokens < 0 {
				s.tokens = 0
			}
			held := now.Sub(start)
			s.mu.Unlock()
			if held > 0 {
				s.recordHold(held)
			}
			return held
		}
		deficit := effNeed - s.tokens
		sleepNs := deficit * 1e9 / s.refillBits
		// Never sleep past the idle cap in one go: beyond it no credit
		// accrues, so a longer sleep is pure overshoot (and would inflate the
		// measured hold). Shorter sleeps re-enter the loop and re-check.
		if s.idleCap > 0 && sleepNs > int64(s.idleCap) {
			sleepNs = int64(s.idleCap)
		}
		s.mu.Unlock()
		s.sleep(time.Duration(sleepNs))
	}
}

func (s *Shaper) recordHold(held time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.HeldPackets++
	s.HeldNs += int64(held)
	if int64(held) > s.MaxHoldNs {
		s.MaxHoldNs = int64(held)
	}
}
