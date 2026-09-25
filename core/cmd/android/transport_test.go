//go:build android || jni

package main

// Host tests for the media transport lifecycle (pure Go — the JNI exports
// themselves are exercised by the JVM-level harness, which loads the
// c-shared library; see android/app/src/test).

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	pion "github.com/pion/webrtc/v4"

	"github.com/om051p/phonebridge/core/pkg/rtpmedia"
)

// newReceiverPeer builds a bare remote PC that accepts our offer and returns
// a real answer (the loopback negotiation shape, so lifecycle tests exercise
// real SDP/ICE state — no capture, no Pion sender, just negotiation).
func newReceiverPeer(t *testing.T, offer pion.SessionDescription) pion.SessionDescription {
	t.Helper()
	se := &pion.SettingEngine{}
	se.SetICEMulticastDNSMode(0) // default; loopback tests elsewhere cover candidates
	api := pion.NewAPI(pion.WithSettingEngine(*se))
	recvPC, err := api.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatalf("receiver pc: %v", err)
	}
	t.Cleanup(func() { _ = recvPC.Close() })
	if err := recvPC.SetRemoteDescription(offer); err != nil {
		t.Fatalf("recv set remote: %v", err)
	}
	answer, err := recvPC.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("recv answer: %v", err)
	}
	if err := recvPC.SetLocalDescription(answer); err != nil {
		t.Fatalf("recv set local: %v", err)
	}
	// Give gathering a short window; non-trickle answer completeness is not
	// required for these lifecycle assertions.
	return *recvPC.LocalDescription()
}

func newTestTransport(t *testing.T) *MediaTransport {
	t.Helper()
	tr := newMediaTransport(0, 0, 0)
	t.Cleanup(tr.MediaStop)
	return tr
}

func TestMediaTransportLifecycleStateErrors(t *testing.T) {
	tr := newTestTransport(t)

	// idle: every action before Init errors or is a safe no-op.
	if _, err := tr.MediaCreateOffer(); err == nil {
		t.Fatal("create offer in idle must error")
	}
	if err := tr.MediaSetAnswer([]byte(`{}`)); err == nil {
		t.Fatal("set answer in idle must error")
	}
	if err := tr.MediaStart(); err == nil {
		t.Fatal("start in idle must error")
	}
	if tr.MediaOnFrame([]byte{0x41, 1}, 1, false) {
		t.Fatal("OnFrame in idle must return false (not admitted)")
	}
	tr.MediaStop() // idempotent, must not panic
	tr.MediaRelease()

	// Double Init: misuse error (Kotlin bug indicator).
	if err := tr.MediaInit(); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := tr.MediaInit(); err == nil {
		t.Fatal("double init must error")
	}
}

func TestMediaTransportNegotiationFlow(t *testing.T) {
	tr := newTestTransport(t)
	if err := tr.MediaInit(); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Offer: real Pion SDP, JSON-serialized.
	offerJSON, err := tr.MediaCreateOffer()
	if err != nil {
		t.Fatalf("create offer: %v", err)
	}
	var offer sdpBlob
	if err := json.Unmarshal(offerJSON, &offer); err != nil {
		t.Fatalf("offer json: %v", err)
	}
	if offer.Type != "offer" || !strings.Contains(offer.SDP, "H264") {
		t.Fatalf("offer missing H264: type=%s", offer.Type)
	}

	// A frames may be pushed pre-start (queued; not yet sent) — must be
	// admitted without error.
	if !tr.MediaOnFrame([]byte{0, 0, 0, 1, 0x41, 1, 2, 3}, 1, false) {
		t.Fatal("OnFrame after Init must admit frames (queue them)")
	}

	// Set answer from a real receiver peer.
	var pionOffer pion.SessionDescription
	if err := json.Unmarshal(offerJSON, &pionOffer); err != nil {
		t.Fatalf("pion offer: %v", err)
	}
	answer := newReceiverPeer(t, pionOffer)
	ansJSON, _ := json.Marshal(sdpBlob{Type: answer.Type.String(), SDP: answer.SDP})
	if err := tr.MediaSetAnswer(ansJSON); err != nil {
		t.Fatalf("set answer: %v", err)
	}

	// Start only after negotiation; idempotent while streaming.
	if err := tr.MediaStart(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := tr.MediaStart(); err != nil {
		t.Fatalf("start must be idempotent: %v", err)
	}

	// Stop → refused actions; stop/release idempotent.
	tr.MediaStop()
	if err := tr.MediaStart(); err == nil {
		t.Fatal("start after stop must error")
	}
	tr.MediaStop()
	tr.MediaRelease()
	if err := tr.MediaInit(); err != nil {
		t.Fatalf("re-init after release must be allowed: %v", err)
	}
}

func TestMediaTransportBackpressureCounters(t *testing.T) {
	// Stop-then-stats: counters survive session teardown (diagnostic value).
	tr := newTestTransport(t)
	if err := tr.MediaInit(); err != nil {
		t.Fatalf("init: %v", err)
	}
	if !tr.MediaOnFrame([]byte{0, 0, 0, 1, 0x41, 1}, 1, false) {
		t.Fatal("frame not admitted")
	}
	tr.MediaStop()

	var stats map[string]any
	if err := json.Unmarshal(tr.MediaStatsJSON(), &stats); err != nil {
		t.Fatalf("stats json: %v", err)
	}
	if got := stats["pushedAUs"].(float64); got != 1 {
		t.Fatalf("pushedAUs = %v, want 1", got)
	}
	if got := stats["transportState"].(float64); got != trStopped {
		t.Fatalf("transportState = %v, want %d", got, trStopped)
	}
}

func TestMediaTransportConcurrentStop(t *testing.T) {
	tr := newTestTransport(t)
	if err := tr.MediaInit(); err != nil {
		t.Fatalf("init: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = tr.MediaOnFrame([]byte{0x41, byte(n)}, int64(n), false)
			tr.MediaStop()
			tr.MediaRelease()
		}(i)
	}
	wg.Wait() // -race watches the shared state machine
	_ = tr.MediaStatsJSON()
}

func TestSessionErrorPayload(t *testing.T) {
	payload, err := sessionErrorPayload("CONSENT_REVOKED", "user revoked screen sharing")
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	if decoded["type"] != "session_error" {
		t.Errorf("type = %v, want session_error", decoded["type"])
	}
	if decoded["code"] != "CONSENT_REVOKED" {
		t.Errorf("code = %v, want CONSENT_REVOKED", decoded["code"])
	}
	if decoded["message"] != "user revoked screen sharing" {
		t.Errorf("message = %v", decoded["message"])
	}

	// An unrecognised code must be refused, not forwarded: the receiver decides
	// recovery policy from the code, so a typo must not invent a failure class
	// the peer would not recognise.
	if _, err := sessionErrorPayload("NOT_A_REAL_CODE", "x"); err == nil {
		t.Error("expected an unknown code to be rejected")
	}
}

func TestMediaTransport_ReportSessionErrorRequiresNegotiation(t *testing.T) {
	tr := newMediaTransport(64, 4000, 3000)

	// Before any transport exists the call is a documented error, not a panic.
	if err := tr.ReportSessionError("CAPTURE_FAILED", "encoder died"); err == nil {
		t.Error("expected reporting to fail before init")
	}

	if err := tr.MediaInit(); err != nil {
		t.Fatalf("init: %v", err)
	}
	t.Cleanup(tr.MediaRelease)

	// Initialized but not negotiated: there is no control channel to send on, so
	// claiming success here would silently drop a real failure report.
	if err := tr.ReportSessionError("CAPTURE_FAILED", "encoder died"); err == nil {
		t.Error("expected reporting to fail before the answer is applied")
	}
}

func TestSDPBlobRoundTrip(t *testing.T) {
	b, err := json.Marshal(sdpBlob{Type: "answer", SDP: "v=0"})
	if err != nil {
		t.Fatal(err)
	}
	var got sdpBlob
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != "answer" || got.SDP != "v=0" {
		t.Fatalf("round trip: %+v", got)
	}
}

// Slice 3A PSI root-cause regression: the encoder emits SPS/PPS exactly once
// per codec lifetime (the CSD AU), while every offer/reconnect runs
// mediaRelease+mediaInit and discards queued frames. Parameter sets learned at
// ADMISSION must survive that rebuild, and every rebuilt Sender must share the
// same cache so the first IDR after the rebuild is re-injectable.
func TestMediaTransportPSISurvivesTransportRebuild(t *testing.T) {
	sps := []byte{0x67, 0x64, 0x00, 0x20, 0xAC, 0xB4, 0x05, 0xA0}
	pps := []byte{0x68, 0xEE, 0x06, 0xF2, 0xC0}
	idr := []byte{0x65, 0x88, 0x80, 0x11}

	tr := newTestTransport(t)

	// Capture start: transport Init, then the once-per-codec CSD AU arrives.
	if err := tr.MediaInit(); err != nil {
		t.Fatalf("init: %v", err)
	}
	csd := rtpmedia.JoinAnnexB([][]byte{sps, pps, idr})
	if !tr.MediaOnFrame(csd, 1, true) {
		t.Fatal("CSD AU must be admitted")
	}
	if !tr.psi.HasParameterSets() {
		t.Fatal("admission-side learning must cache SPS/PPS from the CSD AU")
	}

	// Offer-time transport rebuild (LanSignalingServer.handleOffer):
	// mediaRelease discards the queued CSD AU, mediaInit builds a new Sender.
	tr.MediaRelease()
	if err := tr.MediaInit(); err != nil {
		t.Fatalf("re-init: %v", err)
	}

	if !tr.psi.HasParameterSets() {
		t.Fatal("PSI cache must survive mediaRelease+mediaInit")
	}

	// The rebuilt Sender must share the transport-owned cache...
	tr.mu.Lock()
	s := tr.sender
	tr.mu.Unlock()
	if s == nil {
		t.Fatal("no sender after re-init")
	}
	if s.Cache() != tr.psi {
		t.Fatal("rebuilt Sender must share the transport PSI cache")
	}

	// ...so the first IDR after the rebuild (wire carries no PSI) is completed.
	bare := rtpmedia.JoinAnnexB([][]byte{idr})
	completed := s.Cache().Prepare(rtpmedia.SplitAnnexB(bare))
	if len(completed) != 3 {
		t.Fatalf("bare IDR after rebuild not completed: %d NALs, want 3", len(completed))
	}
	if rtpmedia.NALType(completed[0][0]) != rtpmedia.NALTypeSPS ||
		rtpmedia.NALType(completed[1][0]) != rtpmedia.NALTypePPS {
		t.Fatal("completed IDR must lead with cached SPS then PPS")
	}

	// A capture restart pushes a new CSD AU: learning must replace stale
	// parameter sets rather than refuse them.
	sps2 := append([]byte(nil), sps...)
	sps2[len(sps2)-1] ^= 0xFF
	tr.psi.Learn(rtpmedia.SplitAnnexB(rtpmedia.JoinAnnexB([][]byte{sps2, pps})))
	if got := tr.psi.SPS(); string(got) != string(sps2) {
		t.Fatalf("SPS not replaced on relearn: %x", got)
	}
}
