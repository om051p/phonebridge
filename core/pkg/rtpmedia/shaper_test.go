package rtpmedia

import (
	"sync"
	"testing"
	"time"
)

// fakeClock drives Shaper.Wait deterministically: sleep advances the clock by
// the requested delay, so token refill math runs exactly and tests never
// depend on wall-clock timing.
type fakeClock struct {
	mu    sync.Mutex
	now   time.Time
	slept time.Duration
	n     int
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(0, 1_000_000)}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Sleep(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
	f.slept += d
	f.n++
}

// Advance moves the clock without any Wait in between (simulated silence).
func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

func newTestShaper(t *testing.T, cfg ShaperConfig) (*Shaper, *fakeClock) {
	t.Helper()
	s := NewShaper(cfg)
	if !s.Enabled() {
		t.Fatal("shaper unexpectedly disabled")
	}
	fc := newFakeClock()
	s.SetClock(fc.Now, fc.Sleep)
	return s, fc
}

func TestShaperDisabledPassthrough(t *testing.T) {
	s := NewShaper(ShaperConfig{Kbps: -1})
	if s.Enabled() {
		t.Fatal("Kbps<0 must disable the shaper")
	}
	// A disabled (or nil) shaper never waits, for any size.
	if got := s.Wait(10 << 20); got != 0 {
		t.Fatalf("disabled shaper waited %v", got)
	}
	var nilShaper *Shaper
	if nilShaper.Enabled() || nilShaper.Wait(1000) != 0 {
		t.Fatal("nil shaper must be a zero-cost no-op")
	}
}

func TestShaperDefaultConfigIsDEC021(t *testing.T) {
	s := NewShaper(ShaperConfig{})
	if !s.Enabled() {
		t.Fatal("default config must enable the shaper")
	}
	if s.refillBits != DefaultShaperKbps*1000 {
		t.Fatalf("refill = %d bits/s, want %d (4 Mbps measured ceiling)", s.refillBits, DefaultShaperKbps*1000)
	}
	if s.capBits != DefaultShaperBurstKbit*1000 {
		t.Fatalf("bucket = %d bits, want %d (3000 kbit measured)", s.capBits, DefaultShaperBurstKbit*1000)
	}
	if s.idleCap != DefaultIdleCreditCap {
		t.Fatalf("idle cap = %v, want %v (500 ms measured)", s.idleCap, DefaultIdleCreditCap)
	}
}

func TestShaperFullBucketFirstPacketsNoWait(t *testing.T) {
	s, _ := newTestShaper(t, ShaperConfig{})
	// The bucket starts full: 3 Mbit / 375 kB. A 1200 B RTP packet never
	// waits while tokens remain.
	for i := 0; i < 250; i++ {
		if got := s.Wait(1200); got != 0 {
			t.Fatalf("packet %d waited %v with a full bucket", i, got)
		}
	}
	// 250 × 9600 bits = 2.4 Mbit consumed; still under the bucket.
	if s.HeldPackets != 0 || s.HeldNs != 0 {
		t.Fatalf("counters moved without waits: held=%d ns=%d", s.HeldPackets, s.HeldNs)
	}
}

func TestShaperSustainedRateIsKbps(t *testing.T) {
	s, fc := newTestShaper(t, ShaperConfig{})
	// Drain the full bucket exactly: 3,000,000 bits = 375,000 bytes.
	if got := s.Wait(375_000); got != 0 {
		t.Fatalf("full-bucket drain waited %v", got)
	}
	// 100 kB = 800,000 bits at 4 Mbps = exactly 200 ms.
	got := s.Wait(100_000)
	if got != 200*time.Millisecond {
		t.Fatalf("wait = %v, want exactly 200ms (4 Mbps)", got)
	}
	// The fake clock advanced only through the requested sleep.
	if fc.slept != 200*time.Millisecond {
		t.Fatalf("slept = %v, want 200ms", fc.slept)
	}
	if s.HeldPackets != 1 || s.HeldNs != int64(200*time.Millisecond) || s.MaxHoldNs != int64(200*time.Millisecond) {
		t.Fatalf("counters: held=%d ns=%d max=%d", s.HeldPackets, s.HeldNs, s.MaxHoldNs)
	}
}

// TestShaperIdleCreditCap is the screen-off scenario: a long silent period
// must bank at most 500 ms of tokens, so the wake burst cannot exit at line
// rate. With the cap: deficit = need − 500ms×4Mbps = 240,000 bits → 60 ms.
// Without the cap the bucket would refill to full during the silence and the
// same packet would exit after 0 ms.
func TestShaperIdleCreditCap(t *testing.T) {
	s, fc := newTestShaper(t, ShaperConfig{})
	// Drain the bucket fully (375 kB), then stay silent 10 s (screen off).
	if got := s.Wait(375_000); got != 0 {
		t.Fatalf("drain waited %v", got)
	}
	fc.Advance(10 * time.Second)
	// Wake burst: 280 kB = 2,240,000 bits. Capped refill = 2,000,000 bits
	// (500 ms), deficit 240,000 bits = exactly 60 ms at 4 Mbps.
	got := s.Wait(280_000)
	if got != 60*time.Millisecond {
		t.Fatalf("wake packet wait = %v, want exactly 60ms with the idle-credit cap", got)
	}
}

// TestShaperOversizedPacketTerminates: a single packet larger than the bucket
// can never wait out its own cost (tokens cap at bucket size), so it pays at
// most one full bucket and goes. Must terminate and stay bounded. (Cannot
// occur in production: MTU-sized RTP packets are ~9.6 kbit vs a 3 Mbit
// bucket.)
func TestShaperOversizedPacketTerminates(t *testing.T) {
	s, fc := newTestShaper(t, ShaperConfig{})
	_ = s.Wait(375_000)    // drain the bucket fully first
	got := s.Wait(500_000) // 4 Mbit > 3 Mbit bucket
	// Pays exactly the full bucket: 3,000,000 bits / 4 Mbps = 750 ms.
	if got != 750*time.Millisecond {
		t.Fatalf("oversized packet wait = %v, want the full bucket (750ms)", got)
	}
	if fc.slept != 750*time.Millisecond {
		t.Fatalf("slept = %v, want 750ms", fc.slept)
	}
	// And it terminates on a fresh (full) bucket too, consuming it whole.
	s2, _ := newTestShaper(t, ShaperConfig{})
	if got := s2.Wait(500_000); got != 0 {
		t.Fatalf("oversized packet on a full bucket waited %v, want 0", got)
	}
}

// TestShaperBurstSequenceMatchesSpikeShape: a wake sequence — drain, silence,
// then packets — holds the first post-wake packets briefly and then flows at
// the sustained rate, mirroring the spike burst trials (bounded hold, zero
// reordering by construction).
func TestShaperBurstSequenceMatchesSpikeShape(t *testing.T) {
	s, fc := newTestShaper(t, ShaperConfig{})
	_ = s.Wait(375_000) // steady-state drain
	fc.Advance(10 * time.Second)
	holds := []time.Duration{}
	for i := 0; i < 20; i++ {
		holds = append(holds, s.Wait(1200))
	}
	// After the capped 500 ms refill, the bucket holds 2 Mbit ≈ 208 packets:
	// every 1200 B packet must pass with zero wait.
	for i, h := range holds {
		if h != 0 {
			t.Fatalf("packet %d held %v, want 0 (bucket refilled under the cap)", i, h)
		}
	}
	if s.HeldPackets != 0 {
		t.Fatalf("HeldPackets = %d, want 0", s.HeldPackets)
	}
}

func TestShaperConcurrentWait(t *testing.T) {
	s := NewShaper(ShaperConfig{})
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				_ = s.Wait(1200)
			}
		}()
	}
	wg.Wait() // -race watches the shared bucket state
	// 200 × 9600 = 1.92 Mbit consumed from the 3 Mbit bucket: no token
	// starvation. (HeldPackets also counts lock-acquisition delay, which is
	// nonzero under contention by definition; the production caller is
	// single-writer. What matters is that the hold stays tiny.)
	if s.MaxHoldNs > int64(100*time.Millisecond) {
		t.Fatalf("MaxHoldNs = %d, want bounded far below the bucket timescale", s.MaxHoldNs)
	}
}

func TestShaperZeroAndNegativeSizes(t *testing.T) {
	s, _ := newTestShaper(t, ShaperConfig{})
	if got := s.Wait(0); got != 0 {
		t.Fatalf("Wait(0) waited %v", got)
	}
	if got := s.Wait(-100); got != 0 {
		t.Fatalf("Wait(negative) waited %v", got)
	}
}
