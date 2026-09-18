package rtpmedia

import "testing"

func TestThrottleDisabled(t *testing.T) {
	th := NewThrottle(0)
	for i := 0; i < 100; i++ {
		if !th.Allow(false) {
			t.Fatalf("disabled throttle dropped frame %d", i)
		}
	}
	if th.Kept != 100 || th.Dropped != 0 || th.Observed != 100 {
		t.Fatalf("counters: kept=%d dropped=%d observed=%d", th.Kept, th.Dropped, th.Observed)
	}
}

// TestThrottleGOP30Keep8 is the measured operating point: GOP 30, keep 8.
// The IDR plus 7 P-frames pass; the 22-frame tail drops. Delivered ≈32 AU/s
// at ~120 fps encode (8.05/30 of the input rate).
func TestThrottleGOP30Keep8(t *testing.T) {
	th := NewThrottle(8)
	kept, dropped := 0, 0
	for gop := 0; gop < 10; gop++ {
		for i := 0; i < 30; i++ {
			key := i == 0
			if th.Allow(key) {
				kept++
			} else {
				dropped++
			}
		}
	}
	if kept != 80 { // 10 GOPs × 8
		t.Fatalf("kept = %d, want 80", kept)
	}
	if dropped != 220 {
		t.Fatalf("dropped = %d, want 220", dropped)
	}
	if th.Observed != 300 || th.GOPs != 10 {
		t.Fatalf("observed=%d GOPs=%d, want 300/10", th.Observed, th.GOPs)
	}
}

// TestThrottleKeepsContiguousPrefix is the prediction-safety invariant: the
// kept set of each GOP must be exactly {IDR} ∪ first K-1 P-frames — a
// contiguous prefix — never a stride.
func TestThrottleKeepsContiguousPrefix(t *testing.T) {
	for _, keep := range []int{1, 2, 8, 15, 30, 31, 100} {
		th := NewThrottle(keep)
		for gop := 0; gop < 3; gop++ {
			keptPositions := []int{}
			for i := 0; i < 30; i++ {
				if th.Allow(i == 0) {
					keptPositions = append(keptPositions, i)
				}
			}
			want := keep
			if want > 30 {
				want = 30
			}
			if len(keptPositions) != want {
				t.Fatalf("keep=%d gop=%d: kept %d frames, want %d", keep, gop, len(keptPositions), want)
			}
			for pos, p := range keptPositions {
				if p != pos { // prefix ⇒ kept positions are 0..K-1
					t.Fatalf("keep=%d gop=%d: kept position %d at index %d — not a contiguous prefix", keep, gop, p, pos)
				}
			}
		}
	}
}

// TestThrottleKeyAlwaysKept: an IDR may arrive at any point (mid-GOP IDR on
// resolution change or sync request); it always starts a fresh GOP and is
// kept.
func TestThrottleKeyAlwaysKept(t *testing.T) {
	th := NewThrottle(4)
	seq := []bool{true, false, false, false, false, false,
		true, // mid-stream IDR at position 6
		false, false}
	keptAt := []int{}
	for i, key := range seq {
		if th.Allow(key) {
			keptAt = append(keptAt, i)
		}
	}
	// GOP 1 (6 frames): keep 0..3 → positions 0,1,2,3
	// GOP 2 starts at 6: keep 6,7,8 (only 2 P-frames exist)
	want := []int{0, 1, 2, 3, 6, 7, 8}
	eq := len(want) == len(keptAt)
	if eq {
		for i := range want {
			if want[i] != keptAt[i] {
				eq = false
			}
		}
	}
	if !eq {
		t.Fatalf("kept positions = %v, want %v", keptAt, want)
	}
	if th.GOPs != 2 {
		t.Fatalf("GOPs = %d, want 2", th.GOPs)
	}
}

// TestThrottleDeltaMatchesSpikeRate cross-checks the delivered-rate ratio at
// the ratified operating point against the spike measurement (~32 AU/s from
// ~120 fps ⇒ ratio ≈ 0.268; keep 8/30 ⇒ 0.2683 with the IDR always included).
func TestThrottleDeltaMatchesSpikeRate(t *testing.T) {
	th := NewThrottle(8)
	for gop := 0; gop < 4; gop++ {
		for i := 0; i < 30; i++ {
			th.Allow(i == 0)
		}
	}
	ratio := float64(th.Kept) / float64(th.Observed)
	// 8.05/30 across full GOPs (keep counts IDR+7 per 30).
	if ratio < 0.266 || ratio > 0.270 {
		t.Fatalf("delivered ratio = %.4f, want ≈8.05/30 ≈ 0.2683", ratio)
	}
}

func TestThrottleKeepLargerThanGOP(t *testing.T) {
	th := NewThrottle(100)
	for i := 0; i < 30; i++ {
		if !th.Allow(i == 0) {
			t.Fatalf("frame %d dropped though keep exceeds GOP length", i)
		}
	}
	if th.Dropped != 0 {
		t.Fatalf("Dropped = %d, want 0", th.Dropped)
	}
}

func TestThrottleKeepOneDeliversIDROnly(t *testing.T) {
	th := NewThrottle(1)
	for gop := 0; gop < 3; gop++ {
		for i := 0; i < 30; i++ {
			if got := th.Allow(i == 0); got != (i == 0) {
				t.Fatalf("keep=1: frame %d allowed=%v, want %v", i, got, i == 0)
			}
		}
	}
	if th.Kept != 3 || th.Dropped != 87 {
		t.Fatalf("kept=%d dropped=%d, want 3/87", th.Kept, th.Dropped)
	}
}
