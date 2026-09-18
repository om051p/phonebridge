package rtpmedia

import (
	"bytes"
	"testing"

	"github.com/pion/rtp"
)

func TestDepacketizerSingleNALU(t *testing.T) {
	dp := NewDepacketizer()

	// Single SPS (NAL type 7)
	sps := []byte{0x67, 0x42, 0x00, 0x1f}
	pkt1 := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 100,
			Timestamp:      90000,
			Marker:         false,
		},
		Payload: sps,
	}

	au, err := dp.Push(pkt1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if au != nil {
		t.Fatalf("expected nil AU before marker, got %+v", au)
	}

	// Single IDR Slice (NAL type 5) with Marker = true
	idr := []byte{0x65, 0x88, 0x84, 0x00}
	pkt2 := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 101,
			Timestamp:      90000,
			Marker:         true,
		},
		Payload: idr,
	}

	au, err = dp.Push(pkt2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if au == nil {
		t.Fatal("expected AU on marker packet, got nil")
	}

	if !au.IsKeyframe {
		t.Errorf("expected IsKeyframe = true")
	}
	if au.SPSCount != 1 {
		t.Errorf("expected SPSCount = 1, got %d", au.SPSCount)
	}
	if au.Timestamp != 90000 {
		t.Errorf("expected Timestamp = 90000, got %d", au.Timestamp)
	}

	// Verify Annex-B framing: startCode4 + sps + startCode4 + idr
	expectedData := append(append(append([]byte{}, startCode4...), sps...), append(append([]byte{}, startCode4...), idr...)...)
	if !bytes.Equal(au.Data, expectedData) {
		t.Errorf("AU data mismatch:\ngot  %x\nwant %x", au.Data, expectedData)
	}

	st := dp.Stats()
	if st.Packets != 2 {
		t.Errorf("expected 2 packets, got %d", st.Packets)
	}
	if st.AccessUnits != 1 {
		t.Errorf("expected 1 AU, got %d", st.AccessUnits)
	}
	if st.Keyframes != 1 {
		t.Errorf("expected 1 keyframe, got %d", st.Keyframes)
	}
	if st.NALTypeSPS != 1 || st.NALTypeIDR != 1 {
		t.Errorf("unexpected NAL counts: sps=%d idr=%d", st.NALTypeSPS, st.NALTypeIDR)
	}
}

func TestDepacketizerFUA(t *testing.T) {
	dp := NewDepacketizer()

	// Fragment an IDR slice (type 5, NRI = 3 -> header 0x65)
	// FU indicator: (0x65 & 0x60) | 28 = 0x60 | 0x1C = 0x7C
	// FU header start: 0x80 | 5 = 0x85
	// FU header mid:   0x00 | 5 = 0x05
	// FU header end:   0x40 | 5 = 0x45

	frag1 := []byte{0x7C, 0x85, 0x11, 0x22}
	frag2 := []byte{0x7C, 0x05, 0x33, 0x44}
	frag3 := []byte{0x7C, 0x45, 0x55, 0x66}

	p1 := &rtp.Packet{Header: rtp.Header{SequenceNumber: 50, Timestamp: 1000}, Payload: frag1}
	p2 := &rtp.Packet{Header: rtp.Header{SequenceNumber: 51, Timestamp: 1000}, Payload: frag2}
	p3 := &rtp.Packet{Header: rtp.Header{SequenceNumber: 52, Timestamp: 1000, Marker: true}, Payload: frag3}

	au, err := dp.Push(p1)
	if err != nil || au != nil {
		t.Fatalf("p1 unexpected: au=%v err=%v", au, err)
	}

	au, err = dp.Push(p2)
	if err != nil || au != nil {
		t.Fatalf("p2 unexpected: au=%v err=%v", au, err)
	}

	au, err = dp.Push(p3)
	if err != nil || au == nil {
		t.Fatalf("p3 failed: au=%v err=%v", au, err)
	}

	expectedReconstructed := []byte{0x65, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66}
	expectedAnnexB := append(append([]byte{}, startCode4...), expectedReconstructed...)

	if !bytes.Equal(au.Data, expectedAnnexB) {
		t.Errorf("FU-A data mismatch:\ngot  %x\nwant %x", au.Data, expectedAnnexB)
	}
	if !au.IsKeyframe {
		t.Errorf("expected IsKeyframe = true")
	}

	st := dp.Stats()
	if st.FragPackets != 3 {
		t.Errorf("expected 3 frag packets, got %d", st.FragPackets)
	}
	if st.AccessUnits != 1 {
		t.Errorf("expected 1 AU, got %d", st.AccessUnits)
	}
}

func TestDepacketizerSequenceGapsAndLoss(t *testing.T) {
	dp := NewDepacketizer()

	// Initial packet seq = 1000
	_, _ = dp.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 1000, Timestamp: 100}, Payload: []byte{0x41, 0x01}})

	// Duplicate seq = 1000
	_, _ = dp.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 1000, Timestamp: 100}, Payload: []byte{0x41, 0x01}})

	// Gap: jump to 1004 (missing 1001, 1002, 1003 -> gap = 3)
	_, _ = dp.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 1004, Timestamp: 100}, Payload: []byte{0x41, 0x02}})

	// Late / out-of-order: seq = 1002
	_, _ = dp.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 1002, Timestamp: 100}, Payload: []byte{0x41, 0x03}})

	st := dp.Stats()
	if st.DupSeq != 1 {
		t.Errorf("expected 1 DupSeq, got %d", st.DupSeq)
	}
	if st.SeqGaps != 3 {
		t.Errorf("expected 3 SeqGaps, got %d", st.SeqGaps)
	}
	if st.LatePackets != 1 {
		t.Errorf("expected 1 LatePacket, got %d", st.LatePackets)
	}
}

func TestDepacketizerTimestampBoundaryFlush(t *testing.T) {
	dp := NewDepacketizer()

	// Packet with ts = 1000 without marker bit
	_, _ = dp.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 1, Timestamp: 1000, Marker: false}, Payload: []byte{0x41, 0x01}})

	// Next packet with ts = 2000 triggers flush of the previous AU
	au, err := dp.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 2, Timestamp: 2000, Marker: false}, Payload: []byte{0x41, 0x02}})
	if err != nil {
		t.Fatal(err)
	}
	if au == nil {
		t.Fatal("expected flushed AU on timestamp advance")
	}
	if au.Timestamp != 1000 {
		t.Errorf("expected flushed AU timestamp 1000, got %d", au.Timestamp)
	}

	// Final flush at stream end
	finalAU := dp.Flush()
	if finalAU == nil {
		t.Fatal("expected final flushed AU")
	}
	if finalAU.Timestamp != 2000 {
		t.Errorf("expected final AU timestamp 2000, got %d", finalAU.Timestamp)
	}
}

func TestDepacketizerRoundTripGoldenCapture(t *testing.T) {
	aus, err := loadSliceAUs("testdata/spike04-shaped4-1-slice")
	if err != nil {
		t.Fatalf("loadSliceAUs: %v", err)
	}
	p := NewPacketizer(1200)
	dp := NewDepacketizer()

	seq := uint16(100)
	for auIdx, au := range aus {
		nals := SplitAnnexB(au.data)
		pkts, err := p.PacketizeAU(nals)
		if err != nil {
			t.Fatalf("AU %d packetize failed: %v", auIdx, err)
		}

		var receivedAU *AccessUnit
		for _, pkt := range pkts {
			rtpPkt := &rtp.Packet{
				Header: rtp.Header{
					SequenceNumber: seq,
					Timestamp:      au.rtpTS,
					Marker:         pkt.Marker,
				},
				Payload: pkt.Payload,
			}
			seq++

			emitted, err := dp.Push(rtpPkt)
			if err != nil {
				t.Fatalf("AU %d packet push failed: %v", auIdx, err)
			}
			if emitted != nil {
				receivedAU = emitted
			}
		}

		if receivedAU == nil {
			t.Fatalf("AU %d: expected completed AU, got nil", auIdx)
		}

		// Verify that all NALs in the original AU are present and byte-identical
		origNALs := SplitAnnexB(au.data)
		recvNALs := SplitAnnexB(receivedAU.Data)

		if len(origNALs) != len(recvNALs) {
			t.Fatalf("AU %d NAL count mismatch: orig=%d recv=%d", auIdx, len(origNALs), len(recvNALs))
		}

		for i := range origNALs {
			if !bytes.Equal(origNALs[i], recvNALs[i]) {
				t.Errorf("AU %d NAL %d mismatch", auIdx, i)
			}
		}

		if au.idr != receivedAU.IsKeyframe {
			t.Errorf("AU %d keyframe mismatch: orig=%v recv=%v", auIdx, au.idr, receivedAU.IsKeyframe)
		}
	}

	st := dp.Stats()
	if st.AccessUnits != int64(len(aus)) {
		t.Errorf("expected %d AUs, got %d", len(aus), st.AccessUnits)
	}
	if st.SeqGaps != 0 || st.DupSeq != 0 || st.LatePackets != 0 {
		t.Errorf("unexpected discontinuities: gaps=%d dup=%d late=%d", st.SeqGaps, st.DupSeq, st.LatePackets)
	}
}
