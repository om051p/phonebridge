package rtpmedia

import (
	"bytes"
	"testing"
)

// Parameter sets matching the validated device dimensions (SM7475: SPS 18 B,
// PPS 5 B per DEC-021) — synthetic but size-faithful.
var (
	testSPS = append([]byte{0x67, 0x64, 0x00, 0x20}, 0xAC, 0xB4, 0x05, 0xA0, 0x64, 0xD3, 0x70, 0x50, 0x60, 0x50, 0x6D, 0x0A, 0x13, 0x50)
	testPPS = []byte{0x68, 0xEE, 0x06, 0xF2, 0xC0}
)

func idrAU() [][]byte { return [][]byte{nal(NALTypeIDR, 1, 2, 3)} }

func pAU() [][]byte { return [][]byte{nal(NALTypeSlice, 9)} }

func TestCacheInjectsIntoIDRLackingParameterSets(t *testing.T) {
	c := &Cache{}
	// CSD-style AU: SPS+PPS pass through and are cached.
	got := c.Prepare([][]byte{testSPS, testPPS})
	if len(got) != 2 || !bytes.Equal(got[0], testSPS) || !bytes.Equal(got[1], testPPS) {
		t.Fatalf("CSD AU must pass through unchanged, got %d NALs", len(got))
	}
	if !c.HasParameterSets() {
		t.Fatal("SPS/PPS not cached after CSD AU")
	}
	if c.CachedSPS != int64(len(testSPS)) || c.CachedPPS != int64(len(testPPS)) {
		t.Fatalf("cached sizes: sps=%d pps=%d", c.CachedSPS, c.CachedPPS)
	}
	// IDR without parameter sets: injection must happen.
	got = c.Prepare(idrAU())
	if len(got) != 3 {
		t.Fatalf("injected IDR AU has %d NALs, want 3", len(got))
	}
	if !bytes.Equal(got[0], testSPS) || !bytes.Equal(got[1], testPPS) {
		t.Fatal("injected parameter sets do not match cache")
	}
	if !bytes.Equal(got[2], nal(NALTypeIDR, 1, 2, 3)) {
		t.Fatal("IDR NAL altered by injection")
	}
	if c.InjectedIDRs != 1 {
		t.Fatalf("InjectedIDRs = %d, want 1", c.InjectedIDRs)
	}
	if c.IDRsNoCache != 0 {
		t.Fatalf("IDRsNoCache = %d, want 0", c.IDRsNoCache)
	}
}

func TestCacheNeverDuplicatesInBandParameterSets(t *testing.T) {
	c := &Cache{}
	c.Prepare([][]byte{testSPS, testPPS})
	// IDR that carries its own SPS/PPS (in-band repetition): untouched.
	inBand := [][]byte{testSPS, testPPS, nal(NALTypeIDR, 7)}
	got := c.Prepare(inBand)
	if len(got) != 3 {
		t.Fatalf("in-band IDR AU has %d NALs, want 3 (no duplication)", len(got))
	}
	if c.InjectedIDRs != 0 {
		t.Fatalf("InjectedIDRs = %d, want 0", c.InjectedIDRs)
	}
	if c.InBandIDRs != 1 {
		t.Fatalf("InBandIDRs = %d, want 1", c.InBandIDRs)
	}
}

// The partial case matters: an IDR with SPS but no PPS is still completed
// (the missing PPS is prepended), because the receiver needs both.
func TestCacheCompletesPartialParameterSets(t *testing.T) {
	c := &Cache{}
	c.Prepare([][]byte{testSPS, testPPS})
	got := c.Prepare([][]byte{testSPS, nal(NALTypeIDR, 7)})
	if len(got) != 3 || !bytes.Equal(got[0], testSPS) || !bytes.Equal(got[1], testPPS) {
		t.Fatalf("partial parameter-set IDR not completed: %d NALs", len(got))
	}
	if c.InjectedIDRs != 1 {
		t.Fatalf("InjectedIDRs = %d, want 1", c.InjectedIDRs)
	}
}

func TestCacheIDRBeforeAnyCSDIsCountedNotInjected(t *testing.T) {
	c := &Cache{}
	got := c.Prepare(idrAU())
	if len(got) != 1 {
		t.Fatalf("IDR before CSD must pass through, got %d NALs", len(got))
	}
	if c.IDRsNoCache != 1 || c.InjectedIDRs != 0 {
		t.Fatalf("IDRsNoCache=%d InjectedIDRs=%d, want 1/0", c.IDRsNoCache, c.InjectedIDRs)
	}
}

func TestCacheUpdatesOnChangedParameterSets(t *testing.T) {
	c := &Cache{}
	c.Prepare([][]byte{testSPS, testPPS})
	// Resolution change: new SPS bytes replace the cache.
	newSPS := append([]byte(nil), testSPS...)
	newSPS[len(newSPS)-1] ^= 0xFF
	c.Prepare([][]byte{newSPS})
	if !bytes.Equal(c.SPS(), newSPS) {
		t.Fatal("changed SPS did not replace the cache")
	}
	if c.SPSUpdates != 1 {
		t.Fatalf("SPSUpdates = %d, want 1", c.SPSUpdates)
	}
	// Identical retransmission must NOT count as an update.
	c.Prepare([][]byte{newSPS})
	if c.SPSUpdates != 1 {
		t.Fatalf("SPSUpdates after duplicate = %d, want 1", c.SPSUpdates)
	}
}

// TestCacheDoesNotMutateInput: injection builds a fresh slice; the caller's
// NAL slice and buffer stay untouched (the queue owns the backing buffer).
func TestCacheDoesNotMutateInput(t *testing.T) {
	c := &Cache{}
	c.Prepare([][]byte{testSPS, testPPS})
	idr := nal(NALTypeIDR, 1, 2, 3)
	in := [][]byte{idr}
	_ = c.Prepare(in)
	if len(in) != 1 || !bytes.Equal(in[0], idr) {
		t.Fatal("input NAL slice was mutated by Prepare")
	}
}

// TestCacheGoldenAgainstSpikeCapture runs the cache over the real device
// capture (40 AUs, receiver-side index records spsN/ppsN per AU on the wire).
// Stripping the parameter sets from every IDR AU and re-injecting them via
// the production cache must reproduce the exact wire NAL sequence the
// receiver observed — the DEC-021 obligation proven end-to-end on real bytes.
func TestCacheGoldenAgainstSpikeCapture(t *testing.T) {
	aus, err := loadSliceAUs("testdata/spike04-shaped4-1-slice")
	if err != nil {
		t.Fatal(err)
	}
	c := &Cache{}
	// Seed the cache the way production learns parameter sets: a normal
	// Prepare pass over the stream-start AU. In the real capture, AU 0 IS
	// the CSD AU — the encoder emits parameter sets exactly once per stream.
	c.Prepare(SplitAnnexB(aus[0].data))
	for _, au := range aus {
		nals := SplitAnnexB(au.data)
		cl := classify(nals)
		// The capture was recorded with spike re-injection active: every IDR
		// carried exactly one SPS and one PPS on the wire.
		if cl.hasIDR {
			if cl.hasSPS != true || cl.hasPPS != true {
				t.Fatalf("AU %d: capture IDR expected to carry SPS+PPS, got sps=%v pps=%v", au.idx, cl.hasSPS, cl.hasPPS)
			}
			// Simulate the encoder WITHOUT in-band parameter sets: strip
			// them, then verify the production cache rebuilds the exact wire
			// NAL sequence.
			var stripped [][]byte
			for _, n := range nals {
				if !IsParameterSet(n) {
					stripped = append(stripped, n)
				}
			}
			reinjected := c.Prepare(stripped)
			if len(reinjected) != len(nals) {
				t.Fatalf("AU %d: re-injected NAL count %d != wire %d", au.idx, len(reinjected), len(nals))
			}
			for i := range nals {
				if !bytes.Equal(reinjected[i], nals[i]) {
					t.Fatalf("AU %d NAL %d: re-injected bytes differ from wire", au.idx, i)
				}
			}
		} else {
			out := c.Prepare(nals)
			if len(out) != len(nals) {
				t.Fatalf("AU %d: non-IDR AU was altered", au.idx)
			}
		}
	}
	if c.InjectedIDRs != 5 {
		t.Fatalf("InjectedIDRs = %d, want 5 (IDR AUs at 0/8/16/24/32)", c.InjectedIDRs)
	}
	if !c.HasParameterSets() {
		t.Fatal("cache never populated from capture")
	}
	// Cached sizes must match the validated device dimensions from DEC-021.
	if c.CachedSPS != 18 || c.CachedPPS != 5 {
		t.Fatalf("cached sizes sps=%d pps=%d, want 18/5 (SM7475 measured)", c.CachedSPS, c.CachedPPS)
	}
}

// Learn is the admission-side counterpart of Prepare (Slice 3A): it caches
// parameter sets without classifying the AU for completion and without
// touching the IDR counters, so a producer can teach the cache even when the
// AU itself is refused admission.
func TestCacheLearnIsSideEffectFree(t *testing.T) {
	c := &Cache{}
	// An AU that carries PSI *and* an IDR: Learn must cache the sets but leave
	// every IDR counter at zero (Prepare owns those).
	c.Learn([][]byte{testSPS, testPPS, nal(NALTypeIDR, 1, 2, 3)})
	if !c.HasParameterSets() {
		t.Fatal("Learn did not cache a complete parameter-set pair")
	}
	if c.CachedSPS != int64(len(testSPS)) || c.CachedPPS != int64(len(testPPS)) {
		t.Fatalf("cached sizes sps=%d pps=%d", c.CachedSPS, c.CachedPPS)
	}
	if c.InjectedIDRs != 0 || c.InBandIDRs != 0 || c.IDRsNoCache != 0 {
		t.Fatalf("Learn touched IDR counters: injected=%d in-band=%d no-cache=%d",
			c.InjectedIDRs, c.InBandIDRs, c.IDRsNoCache)
	}
	// Learning is idempotent: the identical CSD AU re-learned (the encoder can
	// emit it again after a codec restart within the same capture) must not
	// look like a mid-stream change.
	c.Learn([][]byte{testSPS, testPPS})
	if c.SPSUpdates != 0 || c.PPSUpdates != 0 {
		t.Fatalf("duplicate parameter sets counted as updates: sps=%d pps=%d",
			c.SPSUpdates, c.PPSUpdates)
	}
	// Degenerate input never panics and never invents a parameter set.
	c.Learn(nil)
	c.Learn([][]byte{{}, {0x41}})
	if c.SPSUpdates != 0 || c.PPSUpdates != 0 {
		t.Fatal("degenerate input changed the cache")
	}
	if !bytes.Equal(c.SPS(), testSPS) || !bytes.Equal(c.PPS(), testPPS) {
		t.Fatal("cache was mutated by degenerate input")
	}
}

// A partial or corrupt parameter set must never make the cache look usable:
// completeness requires BOTH a plausible SPS and a plausible PPS, so an IDR
// that arrives instead is counted as undecodable (IDRsNoCache) — the typed
// PARAM_SETS_MISSING condition — rather than being completed with garbage.
func TestCacheLearnRejectsPartialAndCorruptSets(t *testing.T) {
	c := &Cache{}
	// SPS only: cached, but not complete.
	c.Learn([][]byte{testSPS})
	if c.HasParameterSets() {
		t.Fatal("an SPS without a PPS must not report parameter sets available")
	}
	if got := c.Prepare(idrAU()); len(got) != 1 || c.IDRsNoCache != 1 {
		t.Fatalf("IDR completed without a PPS: %d NALs, no-cache=%d", len(got), c.IDRsNoCache)
	}

	// Truncated SPS (< 4 B) and PPS (< 2 B): refused, not cached, and the
	// already-valid cached SPS is not displaced.
	c.Learn([][]byte{{0x67}, {0x68}})
	if c.HasParameterSets() {
		t.Fatal("corrupt/truncated parameter sets completed the pair")
	}
	if got := c.SPS(); !bytes.Equal(got, testSPS) {
		t.Fatalf("corrupt SPS displaced the valid cached SPS: %x", got)
	}
	if c.PPS() != nil {
		t.Fatal("corrupt PPS was cached")
	}
	if c.CachedSPS != int64(len(testSPS)) {
		t.Fatalf("CachedSPS = %d, want the previous valid SPS (%d)", c.CachedSPS, len(testSPS))
	}

	// A corrupt PPS alongside a valid SPS still cannot complete the pair...
	c.Learn([][]byte{nal(NALTypeSPS, 0x64, 0x00, 0x20), {0x68}})
	if c.HasParameterSets() {
		t.Fatal("corrupt PPS completed the parameter-set pair")
	}
	// ...and the real PPS arriving later does (order independence).
	c.Learn([][]byte{testPPS})
	if !c.HasParameterSets() {
		t.Fatal("valid SPS + PPS did not complete the pair")
	}
	if got := c.Prepare(idrAU()); len(got) != 3 {
		t.Fatalf("IDR not completed after valid relearn: %d NALs", len(got))
	}
}

func TestCacheConcurrentPrepare(t *testing.T) {
	c := &Cache{}
	c.Prepare([][]byte{testSPS, testPPS})
	done := make(chan struct{})
	for g := 0; g < 4; g++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for i := 0; i < 200; i++ {
				_ = c.Prepare(idrAU())
				_ = c.Prepare(pAU())
				_ = c.SPS()
				_ = c.HasParameterSets()
			}
		}()
	}
	for i := 0; i < 4; i++ {
		<-done
	}
	if c.InjectedIDRs != 800 {
		t.Fatalf("InjectedIDRs = %d, want 800", c.InjectedIDRs)
	}
}
