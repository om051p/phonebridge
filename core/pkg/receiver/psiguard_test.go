package receiver

import (
	"bytes"
	"errors"
	"testing"

	"github.com/om051p/phonebridge/core/pkg/rtpmedia"
)

// Device-sized parameter sets (DEC-021: SPS 18 B / PPS 5 B on SM7475),
// byte-compatible with the rtpmedia psi_test fixtures.
var (
	testSPS = []byte{0x67, 0x64, 0x00, 0x20, 0xAC, 0xB4, 0x05, 0xA0, 0x64, 0xD3,
		0x70, 0x50, 0x60, 0x50, 0x6D, 0x0A, 0x13, 0x50}
	testPPS = []byte{0x68, 0xEE, 0x06, 0xF2, 0xC0}
)

// recordingSink captures forwarded AUs for assertions.
type recordingSink struct {
	au     []rtpmedia.AccessUnit
	closed int
	err    error
}

func (r *recordingSink) WriteAU(au rtpmedia.AccessUnit) error {
	if r.err != nil {
		return r.err
	}
	r.au = append(r.au, au)
	return nil
}

func (r *recordingSink) Close() error { r.closed++; return nil }

func idrNAL() []byte { return []byte{0x65, 0x88, 0x80, 0x11, 0x22, 0x33} }
func pNAL() []byte   { return []byte{0x41, 0x9A, 0x22, 0x44} }
func auOf(nals ...[]byte) rtpmedia.AccessUnit {
	key := false
	sps, pps := 0, 0
	for _, n := range nals {
		switch rtpmedia.NALType(n[0]) {
		case rtpmedia.NALTypeIDR:
			key = true
		case rtpmedia.NALTypeSPS:
			sps++
		case rtpmedia.NALTypePPS:
			pps++
		}
	}
	return rtpmedia.AccessUnit{
		Data:       rtpmedia.JoinAnnexB(nals),
		IsKeyframe: key,
		SPSCount:   sps,
		PPSCount:   pps,
	}
}

func nalsOf(au rtpmedia.AccessUnit) [][]byte { return rtpmedia.SplitAnnexB(au.Data) }

// SPS/PPS present: an in-band CSD AU is learned, forwarded byte-identical,
// and counted as an in-band IDR (never duplicated).
func TestPSIGuardLearnsInBandPSI(t *testing.T) {
	rec := &recordingSink{}
	g := NewPSIGuardSink(rec)

	csd := auOf(testSPS, testPPS, idrNAL())
	if err := g.WriteAU(csd); err != nil {
		t.Fatalf("WriteAU: %v", err)
	}
	if len(rec.au) != 1 {
		t.Fatalf("forwarded %d AUs, want 1", len(rec.au))
	}
	if !bytes.Equal(rec.au[0].Data, csd.Data) {
		t.Fatal("in-band CSD AU must be forwarded unchanged")
	}
	if rec.au[0].SPSCount != 1 || rec.au[0].PPSCount != 1 {
		t.Fatalf("SPS/PPS counts = %d/%d, want 1/1", rec.au[0].SPSCount, rec.au[0].PPSCount)
	}
	st := g.Stats()
	if !st.HaveSPSPP || st.InBandIDRs != 1 || st.InjectedIDRs != 0 {
		t.Fatalf("stats = %+v, want cached + 1 in-band, 0 injected", st)
	}
	if g.NeedsParamSets() {
		t.Fatal("must not report PARAM_SETS_MISSING after PSI arrived")
	}
}

// A bare IDR after cached PSI is completed: cached SPS+PPS prepended in
// canonical order, counts updated, forwarded bytes decodable.
func TestPSIGuardCompletesIDRWithCachedPSI(t *testing.T) {
	rec := &recordingSink{}
	g := NewPSIGuardSink(rec)

	if err := g.WriteAU(auOf(testSPS, testPPS, idrNAL())); err != nil {
		t.Fatalf("seed AU: %v", err)
	}
	bare := auOf(idrNAL())
	if err := g.WriteAU(bare); err != nil {
		t.Fatalf("bare IDR: %v", err)
	}

	got := rec.au[1]
	if bytes.Equal(got.Data, bare.Data) {
		t.Fatal("cached PSI must be prepended to the bare IDR")
	}
	nals := nalsOf(got)
	if len(nals) != 3 {
		t.Fatalf("completed AU has %d NALs, want 3", len(nals))
	}
	if rtpmedia.NALType(nals[0][0]) != rtpmedia.NALTypeSPS ||
		!bytes.Equal(nals[0], testSPS) {
		t.Fatalf("first NAL = %x, want cached SPS", nals[0])
	}
	if rtpmedia.NALType(nals[1][0]) != rtpmedia.NALTypePPS ||
		!bytes.Equal(nals[1], testPPS) {
		t.Fatalf("second NAL = %x, want cached PPS", nals[1])
	}
	if !bytes.Equal(nals[2], idrNAL()) {
		t.Fatalf("slice NAL = %x, want original IDR", nals[2])
	}
	if got.SPSCount != 1 || got.PPSCount != 1 || !got.IsKeyframe {
		t.Fatalf("completed AU metadata = sps %d pps %d key %v",
			got.SPSCount, got.PPSCount, got.IsKeyframe)
	}
	if st := g.Stats(); st.InjectedIDRs != 1 {
		t.Fatalf("InjectedIDRs = %d, want 1", st.InjectedIDRs)
	}
	if g.Reinjected() != 1 {
		t.Fatalf("Reinjected = %d, want 1", g.Reinjected())
	}
}

// An IDR before any parameter set was ever received is forwarded unchanged
// and diagnosed — the guard never fabricates parameter sets.
func TestPSIGuardMissingPSPassthrough(t *testing.T) {
	rec := &recordingSink{}
	g := NewPSIGuardSink(rec)

	bare := auOf(idrNAL())
	if err := g.WriteAU(bare); err != nil {
		t.Fatalf("WriteAU: %v", err)
	}
	if !bytes.Equal(rec.au[0].Data, bare.Data) {
		t.Fatal("undecodable IDR must be forwarded unchanged (no fabrication)")
	}
	st := g.Stats()
	if st.IDRsNoCache != 1 || st.HaveSPSPP {
		t.Fatalf("stats = %+v, want IDRsNoCache=1, uncached", st)
	}
	if !g.NeedsParamSets() {
		t.Fatal("must report PARAM_SETS_MISSING")
	}
	// Non-IDR AU without cache: counted only for IDRs.
	if err := g.WriteAU(auOf(pNAL())); err != nil {
		t.Fatalf("P AU: %v", err)
	}
	if st := g.Stats(); st.IDRsNoCache != 1 {
		t.Fatalf("P-frame must not increment IDRsNoCache, got %d", st.IDRsNoCache)
	}
}

// Reconnect: the guard lives with the session sink, so parameter sets learned
// before a transport rebuild complete the FIRST IDR after the reconnect —
// even though the rebuilt sender's wire stream carries no SPS/PPS.
func TestPSIGuardSurvivesReconnectFirstIDR(t *testing.T) {
	rec := &recordingSink{}
	g := NewPSIGuardSink(rec)

	// Transport 1: stream starts with the (lucky) in-band CSD AU.
	if err := g.WriteAU(auOf(testSPS, testPPS, idrNAL())); err != nil {
		t.Fatalf("pre-reconnect AU: %v", err)
	}
	// Transport rebuild (DEC-022): new Receiver, same session sink/guard.
	// First IDR of the rebuilt transport arrives bare.
	if err := g.WriteAU(auOf(idrNAL())); err != nil {
		t.Fatalf("post-reconnect IDR: %v", err)
	}
	got := rec.au[1]
	nals := nalsOf(got)
	if len(nals) != 3 ||
		rtpmedia.NALType(nals[0][0]) != rtpmedia.NALTypeSPS ||
		rtpmedia.NALType(nals[1][0]) != rtpmedia.NALTypePPS {
		t.Fatalf("first IDR after reconnect not completed: %d NALs", len(nals))
	}
	if st := g.Stats(); st.InjectedIDRs != 1 || st.IDRsNoCache != 0 {
		t.Fatalf("stats = %+v, want 1 injected / 0 no-cache", st)
	}
}

// Cache reset: a new session builds a new guard — no parameter-set state
// leaks from the previous session (resolution changes must relearn).
func TestPSIGuardCacheResetsPerSession(t *testing.T) {
	rec := &recordingSink{}
	g1 := NewPSIGuardSink(rec)
	if err := g1.WriteAU(auOf(testSPS, testPPS, idrNAL())); err != nil {
		t.Fatalf("session 1: %v", err)
	}
	if err := g1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if rec.closed != 1 {
		t.Fatalf("inner close count = %d, want 1", rec.closed)
	}

	g2 := NewPSIGuardSink(rec)
	if g2.Stats().HaveSPSPP {
		t.Fatal("new session guard must start with an empty cache")
	}
	bare := auOf(idrNAL())
	if err := g2.WriteAU(bare); err != nil {
		t.Fatalf("session 2: %v", err)
	}
	if !bytes.Equal(rec.au[1].Data, bare.Data) {
		t.Fatal("session 2 IDR must not be completed from session 1 cache")
	}
	if !g2.NeedsParamSets() {
		t.Fatal("session 2 must diagnose PARAM_SETS_MISSING")
	}
}

// Partial PSI: an IDR carrying only an SPS still gets its PPS completed.
func TestPSIGuardCompletesPartialPSI(t *testing.T) {
	rec := &recordingSink{}
	g := NewPSIGuardSink(rec)
	if err := g.WriteAU(auOf(testSPS, testPPS, pNAL())); err != nil {
		t.Fatalf("seed: %v", err)
	}
	partial := auOf(testSPS, idrNAL()) // SPS but no PPS
	if err := g.WriteAU(partial); err != nil {
		t.Fatalf("partial IDR: %v", err)
	}
	nals := nalsOf(rec.au[1])
	if len(nals) != 3 {
		t.Fatalf("partial AU has %d NALs, want SPS+PPS+IDR", len(nals))
	}
	if !bytes.Equal(nals[1], testPPS) {
		t.Fatalf("missing PPS not completed: %x", nals[1])
	}
}

// Inner errors propagate; Close is delegated.
func TestPSIGuardPropagatesAndCloses(t *testing.T) {
	rec := &recordingSink{err: errors.New("boom")}
	g := NewPSIGuardSink(rec)
	if err := g.WriteAU(auOf(idrNAL())); !errors.Is(err, rec.err) {
		t.Fatalf("WriteAU err = %v, want inner error", err)
	}
	if g.Inner() != FrameSink(rec) {
		t.Fatal("Inner must expose the wrapped sink")
	}
}
