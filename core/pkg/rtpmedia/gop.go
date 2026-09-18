package rtpmedia

// GOP-tail throttling (DEC-021 obligation 3; Spike 04 GOP-RETUNE.md).
//
// The encoder runs at panel rate (~120 fps on the validated device) and
// ignores frame-rate controls, so the delivered rate is cut on this side.
// The validated stream is IPPP with no B-frames and no reordering (decode
// order == display order, IDR-anchored GOPs), which makes the *prefix rule*
// prediction-safe by construction: inside one GOP every frame only
// references earlier frames, so keeping the contiguous prefix
// [IDR ... K-1 following AUs] and dropping the GOP tail can never leave a
// kept frame with a missing reference. Verified pixel-exact (SSIM = 1.0,
// zero imperfect frames) against full-rate references for GOP 15/30/60/240;
// naive striding (1-of-N) is NOT safe (93.3% of kept P-frames corrupted,
// mean SSIM 0.9504).
//
// The freeze cost is structural: per GOP the picture moves for the kept
// prefix, then holds the tail. Measured at GOP 30 / keep 8 (~32 AU/s at
// ~120 fps encoding): motion ≈59 ms then freeze ≈192 ms per 250 ms GOP.
//
// A GOP is delimited by IDR access units (no SPS/PPS heuristic needed: on
// the validated encoder every IDR anchors a GOP). Anything observed between
// two IDRs belongs to one GOP.

// Throttle applies the GOP-tail prefix rule.
type Throttle struct {
	keep int

	gopPos int // AUs observed since the current GOP's IDR (IDR = 0)
	kept   int // frames kept in the current GOP

	// Counters.
	Observed int64 // AUs seen (full encode rate)
	Kept     int64 // AUs passed through
	Dropped  int64 // AUs dropped (always GOP tails)
	GOPs     int64 // GOPs observed (IDR count)
}

// NewThrottle returns a throttle that keeps the first keep access units of
// every GOP. keep <= 0 disables throttling (every frame passes).
func NewThrottle(keep int) *Throttle {
	return &Throttle{keep: keep}
}

// Keep returns the configured prefix length (0 = disabled).
func (t *Throttle) Keep() int { return t.keep }

// Allow applies the rule to one access unit and returns whether to forward
// it. Observe is called with AUs in encode order; key must be true exactly
// for IDR AUs.
func (t *Throttle) Allow(key bool) bool {
	t.Observed++
	if key {
		t.GOPs++
		t.gopPos = 0
		t.kept = 0
	} else {
		t.gopPos++
	}
	if t.keep <= 0 {
		t.Kept++
		return true
	}
	// The IDR itself is always kept (it is the random-access point).
	if t.kept < t.keep {
		t.kept++
		t.Kept++
		return true
	}
	t.Dropped++
	return false
}

// KeptInGOP reports how many frames were kept from the GOP most recently
// observed (diagnostics; stable until the next AU is observed).
func (t *Throttle) KeptInGOP() int { return t.kept }

// GOPPos reports the position of the most recently observed AU within its
// GOP (0 = the IDR itself).
func (t *Throttle) GOPPos() int { return t.gopPos }
