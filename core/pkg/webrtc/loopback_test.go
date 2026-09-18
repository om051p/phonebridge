package webrtc

import (
	"sync"
	"testing"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	pion "github.com/pion/webrtc/v4"

	"github.com/om051p/phonebridge/core/pkg/rtpmedia"
)

// Loopback integration: two real Pion PeerConnections negotiate
// SDP/ICE/DTLS/SRTP over 127.0.0.1 (the Android↔Linux path in miniature),
// the Spike 04 golden capture streams end-to-end through the production
// pipeline, and the receive side depacketizes RFC 6184 back into access
// units. Assertions cover every DEC-021 wire property: sequence continuity,
// per-AU timestamps, marker semantics, SSRC/PT ownership, SPS/PPS on IDRs
// and byte-exact AU round-trip.

// depacketizeAUs reconstructs Annex-B access units from RTP packets
// (single NALU + FU-A; AU boundary = marker packet). Mirrors the receiver
// side of the transport contract.
func depacketizeAUs(pkts []rtp.Packet) [][][]byte {
	var aus [][][]byte
	var cur [][]byte
	var fu []byte
	fuOpen := false
	for _, p := range pkts {
		ty := rtpmedia.NALType(p.Payload[0])
		switch {
		case ty >= 1 && ty <= 23:
			cur = append(cur, p.Payload)
			fuOpen = false
		case ty == rtpmedia.NALTypeFUA:
			hdr := p.Payload[0]&0x60 | p.Payload[1]&0x1F
			if p.Payload[1]&0x80 != 0 { // start bit
				fu = append([]byte{hdr}, p.Payload[2:]...)
				fuOpen = true
			} else if fuOpen {
				fu = append(fu, p.Payload[2:]...)
			}
			if p.Payload[1]&0x40 != 0 { // end bit
				cur = append(cur, fu)
				fu = nil
				fuOpen = false
			}
		}
		if p.Marker {
			aus = append(aus, cur)
			cur = nil
		}
	}
	return aus
}

func nalsListEqual(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !nalsEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// auTimestamp returns the RTP timestamp of AU i in the packet list (AUs are
// marker-terminated; all packets of an AU share one timestamp).
func auTimestamp(pkts []rtp.Packet, au int) uint32 {
	seen := 0
	var ts uint32
	have := false
	for _, p := range pkts {
		if !have {
			ts = p.Timestamp
			have = true
		}
		if p.Marker {
			if seen == au {
				return ts
			}
			seen++
			have = false
		}
	}
	return ts
}

// TestLoopbackGoldenCapture is the full-stack integration test.
func TestLoopbackGoldenCapture(t *testing.T) {
	aus := loadGoldenAUs(t)

	// --- receiver: raw Pion PC with a track collector -----------------------
	seRecv := &pion.SettingEngine{}
	seRecv.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	seRecv.SetIncludeLoopbackCandidate(true)
	apiRecv := pion.NewAPI(
		pion.WithSettingEngine(*seRecv),
		pion.WithInterceptorRegistry(&interceptor.Registry{}),
	)
	recvPC, err := apiRecv.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer recvPC.Close()

	var mu sync.Mutex
	var recvPkts []rtp.Packet
	trackSeen := make(chan *pion.TrackRemote, 1)
	recvPC.OnTrack(func(track *pion.TrackRemote, _ *pion.RTPReceiver) {
		trackSeen <- track
		for {
			pkt, _, err := track.ReadRTP()
			if err != nil {
				return
			}
			mu.Lock()
			recvPkts = append(recvPkts, *pkt)
			mu.Unlock()
		}
	})

	// --- sender: production Session ----------------------------------------
	sender := NewSender(nil, SenderConfig{PSIReinject: true})
	sess, err := NewSession(SessionConfig{
		IncludeLoopback: true,
		PortMin:         45100,
		PortMax:         45199,
	}, sender)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Stop()

	// Seed the PSI cache from the stream-start CSD AU (AU 0), exactly like
	// the capture scenario: encoder sends parameter sets once, transport
	// re-injects them on every forwarded IDR.
	sender.Cache().Prepare(rtpmedia.SplitAnnexB(aus[0].data))

	offer, err := sess.CreateOffer()
	if err != nil {
		t.Fatal(err)
	}
	if err := recvPC.SetRemoteDescription(offer); err != nil {
		t.Fatal(err)
	}
	answer, err := recvPC.CreateAnswer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := recvPC.SetLocalDescription(answer); err != nil {
		t.Fatal(err)
	}
	// wait for the answer's candidates
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && recvPC.ICEGatheringState() != pion.ICEGatheringStateComplete {
		time.Sleep(5 * time.Millisecond)
	}
	fullAnswer := *recvPC.LocalDescription()

	if err := sess.SetRemoteAnswer(fullAnswer); err != nil {
		t.Fatal(err)
	}
	if err := sess.WaitForState(pion.PeerConnectionStateConnected, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	// Start, then feed. Pion fires OnTrack only once the first RTP packet
	// flows, so pushing must lead the trackSeen wait. The stream start may
	// race the final bind step: a prefix of packets written before the first
	// binding hits TrackLocalStaticRTP's silent no-op (nil error, not a
	// SendError). Comparison therefore aligns on the received suffix.
	if err := sess.Start(); err != nil {
		t.Fatal(err)
	}
	trackDone := make(chan struct{})
	go func() {
		select {
		case <-trackSeen:
		case <-time.After(10 * time.Second):
		}
		close(trackDone)
	}()

	for i, au := range aus {
		if !sender.Push(stripParameterSets(au.data), int64(i)*33334, au.idr) {
			t.Fatalf("AU %d rejected by the queue", i)
		}
		time.Sleep(2 * time.Millisecond)
	}
	<-trackDone

	// Wait until the AU count (marker-terminated) stops growing for a
	// quiescence window.
	waitAUs := func() int {
		mu.Lock()
		defer mu.Unlock()
		n := 0
		for _, p := range recvPkts {
			if p.Marker {
				n++
			}
		}
		return n
	}
	lastN := -1
	stable := 0
	deadline = time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && stable < 40 {
		n := waitAUs()
		if n == lastN {
			stable++
		} else {
			stable = 0
			lastN = n
		}
		time.Sleep(50 * time.Millisecond)
	}

	mu.Lock()
	pkts := make([]rtp.Packet, len(recvPkts))
	copy(pkts, recvPkts)
	mu.Unlock()

	// --- wire assertions ----------------------------------------------------
	// SSRC/PT: Pion-owned; consistent across the stream, SSRC nonzero.
	if pkts[0].SSRC == 0 {
		t.Fatal("SSRC zero after bind")
	}
	for i, p := range pkts {
		if p.SSRC != pkts[0].SSRC {
			t.Fatalf("packet %d changed SSRC: %d != %d", i, p.SSRC, pkts[0].SSRC)
		}
		if p.PayloadType != pkts[0].PayloadType {
			t.Fatalf("packet %d changed payload type: %d != %d", i, p.PayloadType, pkts[0].PayloadType)
		}
		if p.Version != 2 {
			t.Fatalf("packet %d has RTP version %d", i, p.Version)
		}
	}
	// Sequence continuity across the SRTP-decrypted receive side.
	for i := 1; i < len(pkts); i++ {
		if pkts[i].SequenceNumber != pkts[i-1].SequenceNumber+1 {
			t.Fatalf("seq discontinuity at %d on the wire", i)
		}
	}
	// AU boundaries: timestamps per-AU constant and monotonic across the
	// whole received stream (a suffix-aligned prefix was possibly lost
	// pre-bind).
	// Drop a torn leading AU first: pre-bind loss can leave an FU-A
	// continuation with no start bit as the stream's opening packet.
	if len(pkts) > 0 {
		t0 := rtpmedia.NALType(pkts[0].Payload[0])
		if t0 == rtpmedia.NALTypeFUA && pkts[0].Payload[1]&0x80 == 0 {
			cut := -1
			for i, p := range pkts {
				if p.Marker {
					cut = i + 1
					break
				}
			}
			if cut < 0 {
				t.Fatal("no complete AU in the received stream")
			}
			pkts = pkts[cut:]
		}
	}
	received := depacketizeAUs(pkts)
	if len(received) < 1 {
		t.Fatal("depacketized zero AUs")
	}
	prevTS := uint32(0)
	for _, p := range pkts {
		if p.Marker && p.Timestamp < prevTS && prevTS != 0 {
			t.Fatalf("AU timestamp regression at ts %d", p.Timestamp)
		}
		prevTS = p.Timestamp
	}
	// Suffix alignment: match received AUs to golden AUs by RTP timestamp —
	// pre-bind packet loss leaves a *suffix* of the capture, so every
	// received AU must equal the golden AU with the same timestamp. The map
	// is keyed on the timestamps actually pushed (i × 33334 µs → i × 3000
	// ticks), not the capture's recorded ones.
	byTS := make(map[uint32][][]byte, len(aus))
	for i, au := range aus {
		byTS[rtpmedia.RTPTimestamp(int64(i)*33334)] = rtpmedia.SplitAnnexB(au.data)
	}
	for i, got := range received {
		ts := auTimestamp(pkts, i)
		want, ok := byTS[ts]
		if !ok {
			t.Fatalf("AU %d: wire timestamp %d not present in the golden capture", i, ts)
		}
		if !nalsListEqual(got, want) {
			t.Fatalf("AU %d: NAL mismatch on the wire (got %d NALs, want %d)", i, len(got), len(want))
		}
	}
	// IDR verification: every SPS-led AU on the wire must be a fully
	// re-injected IDR (SPS+PPS+IDR ordering) — at least the final GOP's
	// IDR must be present in the received suffix.
	idrs := 0
	for _, au := range received {
		if len(au) > 0 && rtpmedia.NALType(au[0][0]) == rtpmedia.NALTypeSPS {
			if len(au) < 3 || rtpmedia.NALType(au[1][0]) != rtpmedia.NALTypePPS || rtpmedia.NALType(au[2][0]) != rtpmedia.NALTypeIDR {
				t.Fatal("IDR AU does not start with SPS+PPS+IDR ordering")
			}
			idrs++
		}
	}
	if idrs < 1 {
		t.Fatalf("no re-injected IDR arrived (got %d AUs; sent=%d dropped=%d sendErrors=%d)",
			len(received), sender.SentAUs.Load(), sender.DroppedAUs.Load(), sender.SendErrors.Load())
	}
	if sender.DroppedAUs.Load() != 0 {
		t.Fatalf("queue dropped %d AUs under normal load", sender.DroppedAUs.Load())
	}

	// Clean stop while the receiver still reads: no hang, no panic.
	sess.Stop()
	_ = recvPC.Close()
}

// TestLoopbackControlDataChannel verifies the control DataChannel survives
// negotiation and passes a message end-to-end.
func TestLoopbackControlDataChannel(t *testing.T) {
	seRecv := &pion.SettingEngine{}
	seRecv.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	seRecv.SetIncludeLoopbackCandidate(true)
	apiRecv := pion.NewAPI(
		pion.WithSettingEngine(*seRecv),
		pion.WithInterceptorRegistry(&interceptor.Registry{}),
	)
	recvPC, err := apiRecv.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer recvPC.Close()

	got := make(chan []byte, 1)
	recvPC.OnDataChannel(func(dc *pion.DataChannel) {
		dc.OnMessage(func(msg pion.DataChannelMessage) { got <- msg.Data })
	})

	sender := NewSender(nil, SenderConfig{})
	sess, err := NewSession(SessionConfig{IncludeLoopback: true}, sender)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Stop()

	offer, err := sess.CreateOffer()
	if err != nil {
		t.Fatal(err)
	}
	if err := recvPC.SetRemoteDescription(offer); err != nil {
		t.Fatal(err)
	}
	answer, err := recvPC.CreateAnswer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := recvPC.SetLocalDescription(answer); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && recvPC.ICEGatheringState() != pion.ICEGatheringStateComplete {
		time.Sleep(5 * time.Millisecond)
	}
	if err := sess.SetRemoteAnswer(*recvPC.LocalDescription()); err != nil {
		t.Fatal(err)
	}
	if err := sess.WaitForState(pion.PeerConnectionStateConnected, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	// The channel must be open on the sender side before Send works; wait
	// for the open event.
	waitOpen := func() bool { return sess.dc.ReadyState() == pion.DataChannelStateOpen }
	if !waitCond(waitOpen, 10*time.Second) {
		t.Fatalf("control channel not open (state %s)", sess.dc.ReadyState())
	}
	if err := sess.SendControl([]byte(`{"type":"ping"}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case b := <-got:
		if string(b) != `{"type":"ping"}` {
			t.Fatalf("control payload = %q", b)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("control message never arrived")
	}
}
