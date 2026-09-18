package rtpmedia

import (
	"bytes"
	"errors"
	"testing"
)

func payloadTypes(t [][]byte, pkts []Packet) []byte {
	types := make([]byte, 0, len(pkts))
	for _, p := range pkts {
		types = append(types, NALType(p.Payload[0]))
	}
	return types
}

func TestPacketizerSingleSmallNALU(t *testing.T) {
	p := NewPacketizer(1200)
	nals := [][]byte{testSPS, testPPS, nal(NALTypeSlice, 1, 2, 3)}
	pkts, err := p.PacketizeAU(nals)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkts) != 3 {
		t.Fatalf("got %d packets, want 3", len(pkts))
	}
	if got := payloadTypes(nals, pkts); !bytes.Equal(got, []byte{7, 8, 1}) {
		t.Fatalf("payload NAL types = %v, want [7 8 1]", got)
	}
	for i, pk := range pkts {
		if pk.Marker != (i == len(pkts)-1) {
			t.Fatalf("packet %d: marker=%v, want marker only on last", i, pk.Marker)
		}
	}
	if !bytes.Equal(pkts[0].Payload, testSPS) {
		t.Fatal("payload must be the NAL itself (single NALU packetization)")
	}
}

func TestPacketizerDefaultBudgetIs1200(t *testing.T) {
	if p := NewPacketizer(0); p.MaxPayload() != 1200 {
		t.Fatalf("default budget = %d, want 1200 (DEC-021)", p.MaxPayload())
	}
	if p := NewPacketizer(-5); p.MaxPayload() != 1200 {
		t.Fatalf("negative budget fallback = %d, want 1200", p.MaxPayload())
	}
}

// TestPacketizerFUAReassembly: fragment a NAL larger than the budget and
// verify the FU-A byte format and exact payload reassembly.
func TestPacketizerFUAReassembly(t *testing.T) {
	const mtu = 1200
	p := NewPacketizer(mtu)
	orig := make([]byte, 3500)
	for i := range orig {
		orig[i] = byte(i * 7)
	}
	orig[0] = 0x65 // IDR slice, nal_ref_idc=3
	nals := [][]byte{orig}
	pkts, err := p.PacketizeAU(nals)
	if err != nil {
		t.Fatal(err)
	}
	// 3499 payload bytes / 1198 per fragment → 3 packets.
	if len(pkts) != 3 {
		t.Fatalf("got %d FU packets, want 3", len(pkts))
	}
	var reassembled []byte
	for i, pk := range pkts {
		payload := pk.Payload
		if len(payload) < 3 {
			t.Fatalf("packet %d too short for FU-A", i)
		}
		if got := NALType(payload[0]); got != NALTypeFUA {
			t.Fatalf("packet %d: NAL type %d, want FU-A (28)", i, got)
		}
		if got := NALRefIDC(payload[0]); got != NALRefIDC(orig[0]) {
			t.Fatalf("packet %d: FU indicator NRI %02x != original %02x", i, got, NALRefIDC(orig[0]))
		}
		fuHdr := payload[1]
		if i == 0 && fuHdr&0x80 == 0 {
			t.Fatal("first FU packet missing start bit")
		}
		if i != 0 && fuHdr&0x80 != 0 {
			t.Fatalf("packet %d has spurious start bit", i)
		}
		if i == len(pkts)-1 && fuHdr&0x40 == 0 {
			t.Fatal("last FU packet missing end bit")
		}
		if i != len(pkts)-1 && fuHdr&0x40 != 0 {
			t.Fatalf("packet %d has spurious end bit", i)
		}
		if got := fuHdr & 0x1F; got != NALTypeIDR {
			t.Fatalf("packet %d: FU header type %d, want original type 5", i, got)
		}
		reassembled = append(reassembled, payload[2:]...)
		if pk.Marker != (i == len(pkts)-1) {
			t.Fatalf("packet %d: marker=%v, want marker only on last", i, pk.Marker)
		}
	}
	// FU-A carries the NAL header in the first FU header: reconstruct it.
	full := append([]byte{0x65}, reassembled...)
	if !bytes.Equal(full, orig) {
		t.Fatalf("FU-A reassembly mismatch: got %d bytes, want %d", len(full), len(orig))
	}
	for _, pk := range pkts {
		if len(pk.Payload) > mtu {
			t.Fatalf("packet of %d bytes exceeds budget %d", len(pk.Payload), mtu)
		}
	}
}

// TestPacketizerExactBudgetBoundary: a NAL of exactly maxPayload rides as a
// single NALU; one byte more fragments into two packets.
func TestPacketizerExactBudgetBoundary(t *testing.T) {
	p := NewPacketizer(1200)
	exact := append([]byte{0x41}, make([]byte, 1199)...) // type 1
	pkts, err := p.PacketizeAU([][]byte{exact})
	if err != nil {
		t.Fatal(err)
	}
	if len(pkts) != 1 || len(pkts[0].Payload) != 1200 {
		t.Fatalf("exact-budget NAL must ride as one packet, got %d packets", len(pkts))
	}
	over := append([]byte{0x41}, make([]byte, 1200)...)
	pkts, err = p.PacketizeAU([][]byte{over})
	if err != nil {
		t.Fatal(err)
	}
	if len(pkts) != 2 {
		t.Fatalf("budget+1 NAL must fragment into 2 packets, got %d", len(pkts))
	}
	if len(pkts[0].Payload) != 1200 || len(pkts[1].Payload) != 4 {
		t.Fatalf("fragment sizes = %d/%d, want 1200/4 (indicator+header+2 payload bytes)", len(pkts[0].Payload), len(pkts[1].Payload))
	}
}

// TestPacketizerSizedFUAustRight: chunk arithmetic at odd sizes.
func TestPacketizerFUAArithmetic(t *testing.T) {
	p := NewPacketizer(10)
	big := append([]byte{0x41}, make([]byte, 30)...) // 30-byte NAL, 29 payload
	pkts, err := p.PacketizeAU([][]byte{big})
	if err != nil {
		t.Fatal(err)
	}
	// chunks of 8: 8+8+8+6 payload bytes over 30 → 4 packets
	if len(pkts) != 4 {
		t.Fatalf("got %d packets, want 4", len(pkts))
	}
	sizes := []int{len(pkts[0].Payload), len(pkts[1].Payload), len(pkts[2].Payload), len(pkts[3].Payload)}
	if !bytes.Equal([]byte{byte(sizes[0]), byte(sizes[1]), byte(sizes[2]), byte(sizes[3])}, []byte{10, 10, 10, 8}) {
		t.Fatalf("fragment payload sizes = %v, want [10 10 10 8]", sizes)
	}
}

// TestPacketizerRealCaptureIDRFragments: the golden capture's IDR AUs are
// 10–20 KB — far beyond 1200 B — so production must fragment them exactly as
// the spike receiver observed (frag packets present, single-NALU P frames).
func TestPacketizerRealCaptureIDRFragments(t *testing.T) {
	aus, err := loadSliceAUs("testdata/spike04-shaped4-1-slice")
	if err != nil {
		t.Fatal(err)
	}
	p := NewPacketizer(1200)
	for _, au := range aus {
		nals := SplitAnnexB(au.data)
		pkts, err := p.PacketizeAU(nals)
		if err != nil {
			t.Fatalf("AU %d: %v", au.idx, err)
		}
		// Wire invariant from the capture: every AU ends with a marker packet.
		if !pkts[len(pkts)-1].Marker {
			t.Fatalf("AU %d: last packet missing marker", au.idx)
		}
		for i := 0; i < len(pkts)-1; i++ {
			if pkts[i].Marker {
				t.Fatalf("AU %d: packet %d carries a non-final marker", au.idx, i)
			}
		}
		// Fragmentation must occur exactly for oversized NALs.
		var oversize int
		for _, n := range nals {
			if len(n) > 1200 {
				oversize++
			}
		}
		fuPackets := 0
		for _, pk := range pkts {
			if NALType(pk.Payload[0]) == NALTypeFUA {
				fuPackets++
			}
		}
		if oversize > 0 && fuPackets == 0 {
			t.Fatalf("AU %d: oversized NALs present but no FU-A packets", au.idx)
		}
		if oversize == 0 && fuPackets != 0 {
			t.Fatalf("AU %d: FU-A packets without oversized NALs", au.idx)
		}
		// Reassembly must reproduce every NAL byte-exactly.
		var cur []byte
		reassembled := [][]byte{}
		active := false
		for _, pk := range pkts {
			t0 := NALType(pk.Payload[0])
			switch {
			case t0 >= 1 && t0 <= 23:
				reassembled = append(reassembled, pk.Payload)
				active = false
			case t0 == NALTypeFUA:
				if !active {
					cur = append([]byte{pk.Payload[0]&0x60 | (pk.Payload[1] & 0x1F)}, pk.Payload[2:]...)
					active = true
				} else {
					cur = append(cur, pk.Payload[2:]...)
				}
			}
			if active && pk.Marker {
				reassembled = append(reassembled, cur)
				active = false
			}
		}
		if len(reassembled) != len(nals) {
			t.Fatalf("AU %d: reassembled %d NALs, want %d", au.idx, len(reassembled), len(nals))
		}
		for i := range nals {
			if !bytes.Equal(reassembled[i], nals[i]) {
				t.Fatalf("AU %d NAL %d: reassembled bytes differ", au.idx, i)
			}
		}
	}
}

func TestPacketizerErrors(t *testing.T) {
	p := NewPacketizer(1200)

	// Empty AU.
	if _, err := p.PacketizeAU(nil); !errors.Is(err, ErrEmptyAU) {
		t.Fatalf("nil NALs: err=%v, want ErrEmptyAU", err)
	}
	if _, err := p.PacketizeAU([][]byte{}); !errors.Is(err, ErrEmptyAU) {
		t.Fatalf("empty NAL list: err=%v, want ErrEmptyAU", err)
	}
	// All-empty NALs also count as empty.
	if _, err := p.PacketizeAU([][]byte{nil, {}}); !errors.Is(err, ErrEmptyAU) {
		t.Fatalf("all-empty NALs: err=%v, want ErrEmptyAU", err)
	}

	// NAL type 0 (unspecified) is invalid.
	if _, err := p.PacketizeAU([][]byte{{0x00, 0x01}}); !errors.Is(err, ErrInvalidNALU) {
		t.Fatalf("type 0: err=%v, want ErrInvalidNALU", err)
	}
	// Aggregation/fragmentation types arriving from the encoder are invalid:
	// the encoder must never emit RFC 6184 transport types (24-31).
	for _, tp := range []byte{24, 25, 26, 27, 28, 29, 30, 31} {
		if _, err := p.PacketizeAU([][]byte{{tp | 0x60, 0x01}}); !errors.Is(err, ErrInvalidNALU) {
			t.Fatalf("type %d: err=%v, want ErrInvalidNALU", tp, err)
		}
	}

	// A valid NAL before an invalid one must reject the WHOLE AU (no partial
	// packet list can leak into the transport).
	_, err := p.PacketizeAU([][]byte{testSPS, {0x1C, 0x02}})
	if !errors.Is(err, ErrInvalidNALU) {
		t.Fatalf("mixed AU: err=%v, want ErrInvalidNALU", err)
	}

	// Empty NALs among valid ones are skipped, not fatal.
	pkts, err := p.PacketizeAU([][]byte{{}, testPPS, {}})
	if err != nil || len(pkts) != 1 {
		t.Fatalf("empty-NAL skip: err=%v pkts=%d, want 1 packet", err, len(pkts))
	}

	// Degenerate maxPayload configuration is rejected for oversized NALs.
	small := NewPacketizer(2)
	if _, err := small.PacketizeAU([][]byte{append([]byte{0x41}, make([]byte, 50)...)}); err == nil {
		t.Fatal("maxPayload=2 with oversized NAL must error")
	}
}
