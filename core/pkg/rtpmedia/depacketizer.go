package rtpmedia

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/pion/rtp"
)

var (
	// ErrTruncatedPacket indicates an RTP payload too short for its declared framing.
	ErrTruncatedPacket = errors.New("rtpmedia: truncated rtp packet")
	// ErrMalformedFUA indicates an invalid FU-A fragmentation header.
	ErrMalformedFUA = errors.New("rtpmedia: malformed fu-a header")
)

// Standard 4-byte Annex-B start code.
var startCode4 = []byte{0x00, 0x00, 0x00, 0x01}

// AccessUnit represents a fully assembled Annex-B H.264 access unit.
type AccessUnit struct {
	Data       []byte    // Complete Annex-B byte stream with 4-byte start codes
	Timestamp  uint32    // RTP timestamp (90 kHz clock)
	IsKeyframe bool      // True if the AU contains an IDR slice (nal_unit_type 5)
	SPSCount   int       // Number of SPS NALUs in this AU
	PPSCount   int       // Number of PPS NALUs in this AU
	ArrivedAt  time.Time // Local arrival time of the final packet completing this AU
}

// StreamStats captures cumulative and operational statistics for an incoming RTP stream.
type StreamStats struct {
	Packets     int64 // Total RTP packets processed
	BytesRTP    int64 // Total RTP payload bytes received (+12 byte header estimate)
	BytesH264   int64 // Total Annex-B H.264 bytes assembled
	AccessUnits int64 // Total access units assembled
	Keyframes   int64 // Total keyframe (IDR) access units
	SingleNALUs int64 // Count of single NAL units received
	FragPackets int64 // Count of FU-A packets received
	NALTypeIDR  int64 // Count of IDR NAL units
	NALTypeSPS  int64 // Count of SPS NAL units
	NALTypePPS  int64 // Count of PPS NAL units
	SeqGaps     int64 // Count of missing RTP packets detected
	DupSeq      int64 // Count of duplicate sequence numbers
	LatePackets int64 // Count of out-of-order / late packets
	TSBackward  int64 // Count of backward timestamp reversals
	FirstRxTime time.Time
	LastRxTime  time.Time
}

// Depacketizer reassembles RFC 6184 RTP packets into Annex-B H.264 access units
// and tracks sequence continuity, packet loss, and stream statistics.
type Depacketizer struct {
	mu sync.Mutex

	stats StreamStats

	lastSeq int64  // Last observed sequence number (-1 = uninitialized)
	lastTs  uint32 // Last observed timestamp
	hasTs   bool

	currentTs  uint32
	currentAU  []byte // Accumulated Annex-B bitstream for the current AU
	fuBuf      []byte // Accumulated payload for an active FU-A sequence
	fuOpen     bool   // True while assembling an FU-A fragment sequence
	firstRxSet bool
}

// NewDepacketizer creates an initialized Depacketizer.
func NewDepacketizer() *Depacketizer {
	return &Depacketizer{
		lastSeq: -1,
	}
}

// Push processes one incoming RTP packet. If the packet completes an Access Unit
// (indicated by the RTP marker bit or a timestamp advance), that Access Unit is returned.
// Returns (nil, nil) if the packet was consumed and more fragments are needed.
func (d *Depacketizer) Push(pkt *rtp.Packet) (*AccessUnit, error) {
	if pkt == nil {
		return nil, nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	if !d.firstRxSet {
		d.stats.FirstRxTime = now
		d.firstRxSet = true
	}
	d.stats.LastRxTime = now

	d.stats.Packets++
	d.stats.BytesRTP += int64(len(pkt.Payload)) + 12

	// 1. Sequence continuity tracking (RFC 3550 sequence numbers start at a random value)
	if d.lastSeq < 0 {
		d.lastSeq = int64(pkt.SequenceNumber)
	} else {
		expect := uint16(d.lastSeq) + 1
		if pkt.SequenceNumber == expect {
			// Normal in-order packet
		} else if pkt.SequenceNumber == uint16(d.lastSeq) {
			d.stats.DupSeq++
		} else {
			diff := int16(pkt.SequenceNumber - expect)
			if diff < 0 {
				d.stats.LatePackets++
			} else {
				d.stats.SeqGaps += int64(diff)
			}
		}
		d.lastSeq = int64(pkt.SequenceNumber)
	}

	// 2. Timestamp monotonicity
	if d.hasTs {
		if tsDelta(pkt.Timestamp, d.lastTs) < 0 {
			d.stats.TSBackward++
		}
	}
	d.lastTs = pkt.Timestamp
	d.hasTs = true

	// 3. Check for AU boundary due to timestamp transition
	var emittedAU *AccessUnit
	if len(d.currentAU) > 0 && pkt.Timestamp != d.currentTs {
		emittedAU = d.flushCurrentAULocked(now)
	}
	d.currentTs = pkt.Timestamp

	// 4. RFC 6184 payload unmarshaling
	if len(pkt.Payload) == 0 {
		return emittedAU, nil
	}

	nalType := NALType(pkt.Payload[0])
	switch {
	case nalType >= 1 && nalType <= 23:
		// Single NAL unit packet
		d.stats.SingleNALUs++
		d.recordNALTypeLocked(nalType)
		d.appendNALToCurrentAULocked(pkt.Payload)
		d.fuOpen = false
		d.fuBuf = d.fuBuf[:0]

	case nalType == NALTypeFUA:
		// FU-A Fragmentation Unit (RFC 6184 §5.8)
		if len(pkt.Payload) < 2 {
			return emittedAU, ErrTruncatedPacket
		}
		d.stats.FragPackets++

		fuIndicator := pkt.Payload[0]
		fuHeader := pkt.Payload[1]
		startBit := fuHeader&0x80 != 0
		endBit := fuHeader&0x40 != 0
		innerNALType := fuHeader & 0x1F

		reconstructedNALHeader := (fuIndicator & 0xE0) | innerNALType

		if startBit {
			d.fuBuf = append(d.fuBuf[:0], reconstructedNALHeader)
			d.fuBuf = append(d.fuBuf, pkt.Payload[2:]...)
			d.fuOpen = true
		} else if d.fuOpen {
			d.fuBuf = append(d.fuBuf, pkt.Payload[2:]...)
		}

		if endBit && d.fuOpen {
			d.recordNALTypeLocked(innerNALType)
			d.appendNALToCurrentAULocked(d.fuBuf)
			d.fuBuf = d.fuBuf[:0]
			d.fuOpen = false
		}

	case nalType == NALTypeSTAPA:
		// STAP-A aggregation packet (RFC 6184 §5.7.1)
		offset := 1
		for offset+2 <= len(pkt.Payload) {
			nalSize := int(binary.BigEndian.Uint16(pkt.Payload[offset : offset+2]))
			offset += 2
			if offset+nalSize > len(pkt.Payload) {
				break
			}
			singleNAL := pkt.Payload[offset : offset+nalSize]
			if len(singleNAL) > 0 {
				subType := NALType(singleNAL[0])
				d.recordNALTypeLocked(subType)
				d.appendNALToCurrentAULocked(singleNAL)
			}
			offset += nalSize
		}
		d.fuOpen = false
		d.fuBuf = d.fuBuf[:0]

	default:
		// Unrecognized NAL unit type; skip payload
	}

	// 5. Marker bit signals the end of an Access Unit
	if pkt.Marker && len(d.currentAU) > 0 {
		au := d.flushCurrentAULocked(now)
		if emittedAU == nil {
			emittedAU = au
		}
	}

	return emittedAU, nil
}

// Flush forces any buffered data to be emitted as an AccessUnit (e.g. at EOF/teardown).
func (d *Depacketizer) Flush() *AccessUnit {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.currentAU) == 0 {
		return nil
	}
	return d.flushCurrentAULocked(time.Now())
}

// Stats returns a snapshot copy of the current stream statistics.
func (d *Depacketizer) Stats() StreamStats {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stats
}

func (d *Depacketizer) appendNALToCurrentAULocked(nal []byte) {
	if len(nal) == 0 {
		return
	}
	d.currentAU = append(d.currentAU, startCode4...)
	d.currentAU = append(d.currentAU, nal...)
}

func (d *Depacketizer) recordNALTypeLocked(nalType byte) {
	switch nalType {
	case NALTypeIDR:
		d.stats.NALTypeIDR++
	case NALTypeSPS:
		d.stats.NALTypeSPS++
	case NALTypePPS:
		d.stats.NALTypePPS++
	}
}

func (d *Depacketizer) flushCurrentAULocked(arrivedAt time.Time) *AccessUnit {
	if len(d.currentAU) == 0 {
		return nil
	}

	dataCopy := make([]byte, len(d.currentAU))
	copy(dataCopy, d.currentAU)

	// Classify NALs in the assembled AU
	nals := SplitAnnexB(dataCopy)
	isKey := HasIDR(nals)
	spsCount := 0
	ppsCount := 0
	for _, n := range nals {
		if len(n) == 0 {
			continue
		}
		switch NALType(n[0]) {
		case NALTypeSPS:
			spsCount++
		case NALTypePPS:
			ppsCount++
		}
	}

	au := &AccessUnit{
		Data:       dataCopy,
		Timestamp:  d.currentTs,
		IsKeyframe: isKey,
		SPSCount:   spsCount,
		PPSCount:   ppsCount,
		ArrivedAt:  arrivedAt,
	}

	d.stats.AccessUnits++
	d.stats.BytesH264 += int64(len(dataCopy))
	if isKey {
		d.stats.Keyframes++
	}

	d.currentAU = d.currentAU[:0]
	return au
}

// tsDelta calculates the signed difference a - b under 32-bit modular arithmetic.
func tsDelta(a, b uint32) int64 {
	d := int64(a) - int64(b)
	if d > 1<<31 {
		d -= 1 << 32
	} else if d < -(1 << 31) {
		d += 1 << 32
	}
	return d
}

// String returns a human-readable summary of stream statistics.
func (s StreamStats) String() string {
	dur := s.LastRxTime.Sub(s.FirstRxTime)
	durSec := dur.Seconds()
	fps := 0.0
	kbps := 0.0
	if durSec > 0 {
		fps = float64(s.AccessUnits) / durSec
		kbps = float64(s.BytesH264*8) / durSec / 1000.0
	}
	return fmt.Sprintf("pkts=%d bytes_rtp=%d bytes_h264=%d aus=%d (%.1f fps) keyframes=%d kbps=%.1f gaps=%d dup=%d late=%d ts_back=%d sps=%d pps=%d",
		s.Packets, s.BytesRTP, s.BytesH264, s.AccessUnits, fps, s.Keyframes, kbps, s.SeqGaps, s.DupSeq, s.LatePackets, s.TSBackward, s.NALTypeSPS, s.NALTypePPS)
}
