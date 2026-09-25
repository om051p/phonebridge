//go:build android || jni

package main

// Regression for the live-E2E finding: the once-per-codec CSD AU arrives
// ~100 ms after capture start, BEFORE the first offer initializes a
// transport. MediaOnFrame must learn its parameter sets even while it
// correctly refuses to queue the frame (state < trInitialized) — otherwise
// the cache stays empty for the whole capture and every rebuilt transport
// sends bare IDRs (observed live as PARAM_SETS_MISSING).

import (
	"encoding/json"
	"testing"

	"github.com/om051p/phonebridge/core/pkg/rtpmedia"
)

func TestMediaOnFrameLearnsCSDBeforeInit(t *testing.T) {
	sps := []byte{0x67, 0x64, 0x00, 0x20, 0xAC, 0xB4, 0x05, 0xA0}
	pps := []byte{0x68, 0xEE, 0x06, 0xF2, 0xC0}
	idr := []byte{0x65, 0x88, 0x80, 0x11}

	tr := newTestTransport(t) // state == trIdle, no MediaInit yet

	// The CSD AU arrives right after encoder start, before any offer.
	if admitted := tr.MediaOnFrame(rtpmedia.JoinAnnexB([][]byte{sps, pps}), 1, true); admitted {
		t.Fatal("frames must not be admitted before MediaInit")
	}
	if !tr.psi.HasParameterSets() {
		t.Fatal("PSI must be learned from the CSD AU even before MediaInit")
	}

	// Later: the offer initializes the transport, and the first IDR (which on
	// the real wire carries no PSI) must be completed from the learned cache.
	if err := tr.MediaInit(); err != nil {
		t.Fatalf("init: %v", err)
	}
	tr.mu.Lock()
	s := tr.sender
	tr.mu.Unlock()
	completed := s.Cache().Prepare(rtpmedia.SplitAnnexB(rtpmedia.JoinAnnexB([][]byte{idr})))
	if len(completed) != 3 {
		t.Fatalf("bare IDR not completed after pre-init learn: %d NALs, want 3", len(completed))
	}
	if rtpmedia.NALType(completed[0][0]) != rtpmedia.NALTypeSPS ||
		rtpmedia.NALType(completed[1][0]) != rtpmedia.NALTypePPS {
		t.Fatal("completed IDR must lead with SPS then PPS")
	}
}

// LearnPSI is the transport-free learning point the JNI engine gate uses: it
// must cache parameter sets with no MediaInit at all, must not build a
// transport, and must leave the AU-classification counters (Prepare's job)
// untouched.
func TestLearnPSIWithoutInitBuildsNoTransport(t *testing.T) {
	sps := []byte{0x67, 0x64, 0x00, 0x20, 0xAC, 0xB4, 0x05, 0xA0}
	pps := []byte{0x68, 0xEE, 0x06, 0xF2, 0xC0}

	tr := newTestTransport(t)
	tr.LearnPSI(rtpmedia.JoinAnnexB([][]byte{sps, pps, {0x65, 0x88, 0x80, 0x11}}))

	if !tr.psi.HasParameterSets() {
		t.Fatal("LearnPSI must cache a complete parameter-set pair without MediaInit")
	}
	if st := tr.psi.Stats(); st.CachedSPS != int64(len(sps)) || st.CachedPPS != int64(len(pps)) {
		t.Fatalf("cached sizes = %d/%d, want %d/%d", st.CachedSPS, st.CachedPPS, len(sps), len(pps))
	}
	if st := tr.psi.Stats(); st.InjectedIDRs != 0 || st.InBandIDRs != 0 || st.IDRsNoCache != 0 {
		t.Fatalf("LearnPSI touched IDR counters: %+v", st)
	}
	tr.mu.Lock()
	noSender := tr.sender == nil && tr.session == nil
	tr.mu.Unlock()
	if !noSender {
		t.Fatal("LearnPSI must not build a transport")
	}

	// The counters must be visible in the diagnostics blob: this is the only
	// on-device proof that the phone-side cache learned anything.
	var stats map[string]any
	if err := json.Unmarshal(tr.MediaStatsJSON(), &stats); err != nil {
		t.Fatalf("stats json: %v", err)
	}
	if stats["psiHaveSPSPP"] != true {
		t.Fatalf("psiHaveSPSPP = %v, want true", stats["psiHaveSPSPP"])
	}
	if stats["psiSPSBytes"] != float64(len(sps)) || stats["psiPPSBytes"] != float64(len(pps)) {
		t.Fatalf("stats PSI sizes = %v/%v", stats["psiSPSBytes"], stats["psiPPSBytes"])
	}
}

// No PSI ever learned: the sender still admits and forwards its IDRs, and the
// counters must say so — psiHaveSPSPP=false with psiIDRsNoCache>0 is the
// on-device signature of the receiver's frames_reason=PARAM_SETS_MISSING, and
// a bare IDR stays bare (never completed with fabricated parameter sets).
func TestMediaTransportMissingPSIReportedInStats(t *testing.T) {
	idr := []byte{0x65, 0x88, 0x80, 0x11}
	tr := newTestTransport(t)
	if err := tr.MediaInit(); err != nil {
		t.Fatalf("init: %v", err)
	}
	if !tr.MediaOnFrame(rtpmedia.JoinAnnexB([][]byte{idr}), 1, true) {
		t.Fatal("IDR must still be admitted (transport, not PSI, decides admission)")
	}

	tr.mu.Lock()
	s := tr.sender
	tr.mu.Unlock()
	// Exactly what the single-writer send loop does with every AU.
	out := s.Cache().Prepare(rtpmedia.SplitAnnexB(rtpmedia.JoinAnnexB([][]byte{idr})))
	if len(out) != 1 {
		t.Fatalf("bare IDR was completed without any parameter set: %d NALs", len(out))
	}

	var stats map[string]any
	if err := json.Unmarshal(tr.MediaStatsJSON(), &stats); err != nil {
		t.Fatalf("stats json: %v", err)
	}
	if stats["psiHaveSPSPP"] != false {
		t.Fatalf("psiHaveSPSPP = %v, want false", stats["psiHaveSPSPP"])
	}
	if stats["psiIDRsNoCache"] != float64(1) {
		t.Fatalf("psiIDRsNoCache = %v, want 1 (undecodable random-access point)", stats["psiIDRsNoCache"])
	}
	if stats["psiInjectedIDRs"] != float64(0) {
		t.Fatalf("psiInjectedIDRs = %v, want 0", stats["psiInjectedIDRs"])
	}
	if stats["pushedAUs"] != float64(1) {
		t.Fatalf("pushedAUs = %v, want 1", stats["pushedAUs"])
	}
}

// Duplicate and corrupt parameter sets: a re-learned identical CSD AU is not an
// update, a truncated set is refused (never injected), and a real resolution
// change is still absorbed.
func TestMediaTransportDuplicateAndCorruptPSI(t *testing.T) {
	sps := []byte{0x67, 0x64, 0x00, 0x20, 0xAC, 0xB4, 0x05, 0xA0}
	pps := []byte{0x68, 0xEE, 0x06, 0xF2, 0xC0}

	tr := newTestTransport(t)
	csd := rtpmedia.JoinAnnexB([][]byte{sps, pps})
	tr.LearnPSI(csd)

	// Duplicate (the encoder can re-emit its CSD within one capture): no
	// update, no cache churn.
	tr.LearnPSI(csd)
	if st := tr.psi.Stats(); st.SPSUpdates != 0 || st.PPSUpdates != 0 {
		t.Fatalf("duplicate CSD counted as update: %+v", st)
	}

	// Corrupt/truncated sets are refused and must not displace the valid cache.
	tr.LearnPSI(rtpmedia.JoinAnnexB([][]byte{{0x67}, {0x68, 0xEE}}))
	if got := tr.psi.SPS(); string(got) != string(sps) {
		t.Fatalf("valid SPS displaced by a corrupt set: %x", got)
	}

	// A genuine resolution change replaces the cache and is counted.
	sps2 := append([]byte(nil), sps...)
	sps2[len(sps2)-1] ^= 0xFF
	tr.LearnPSI(rtpmedia.JoinAnnexB([][]byte{sps2}))
	if got := tr.psi.SPS(); string(got) != string(sps2) {
		t.Fatalf("changed SPS not absorbed: %x", got)
	}
	if st := tr.psi.Stats(); st.SPSUpdates != 1 {
		t.Fatalf("SPSUpdates = %d, want 1 after a real change", st.SPSUpdates)
	}
}
