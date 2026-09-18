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
