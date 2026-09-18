package rtpmedia

import "testing"

func TestRTPTimestampConversion(t *testing.T) {
	// 90000/1e6 = 9/100 exactly (spike-verified: ts = ptsUs * 9 / 100).
	cases := []struct {
		ptsUs int64
		want  uint32
	}{
		{0, 0},
		{1_000_000, 90000}, // 1 s
		{33_333, 2999},     // 1/30 s: 33333*9/100 floors to 2999
		{33_334, 3000},     // crosses at the next µs
		{16_667, 1500},     // 1/60 s
		{8_333, 749},       // 1/120 s
		// 30-minute soak span (1.8e9 µs) stays far below the uint32 wrap.
		{1_800_000_000, 162_000_000},
		// Wrap-around per RFC 3550: ptsUs*9/100 = 2^32 + 5.
		{47_721_858_900, 5},
	}
	for _, c := range cases {
		if got := RTPTimestamp(c.ptsUs); got != c.want {
			t.Fatalf("RTPTimestamp(%d) = %d, want %d", c.ptsUs, got, c.want)
		}
	}
}

// TestRTPTimestampMonotonic: floor scaling of a monotonic source must be
// non-decreasing — the soak-verified property (ts_backward = 0 for 30 min).
func TestRTPTimestampMonotonic(t *testing.T) {
	prev := RTPTimestamp(0)
	for i := int64(1); i <= 2000; i++ {
		cur := RTPTimestamp(33_333 * i) // ~30 fps cadence
		if cur < prev {
			t.Fatalf("timestamp went backwards at i=%d: %d < %d", i, cur, prev)
		}
		prev = cur
	}
}

// TestRTPTimestampDeltaAt30fps pins the delivered-cadence delta used by the
// receiver health metrics (1/30 s ≈ 3000 ticks).
func TestRTPTimestampDeltaAt30fps(t *testing.T) {
	a := RTPTimestamp(0)
	b := RTPTimestamp(33_334)
	if d := b - a; d != 3000 {
		t.Fatalf("delta = %d, want 3000", d)
	}
}
