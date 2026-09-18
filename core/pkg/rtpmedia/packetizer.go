package rtpmedia

import "errors"

// RFC 6184 H.264 RTP packetization (DEC-021; Spike 04 validated mode:
// packetization-mode=1, single NALU + FU-A, 1200 B payload budget,
// marker bit only on the final packet of an access unit).
//
// Re-derived from the Spike 04 packetize path with the production hardening
// required by the foundation review: invalid input returns an error instead
// of being passed through or truncated silently. Validated input classes
// from real device output: single NALUs (types 1–23), SPS/PPS/SEI with the
// IDR access unit (7/8/6), and large IDR slices fragmented as FU-A.
//
// Errors:
//
//   - ErrEmptyAU: the access unit carried no NAL units;
//   - ErrInvalidNALU: NAL type 0 (unspecified) or a NAL already in RFC 6184
//     aggregation/fragmentation form (24–31) arrived from the encoder;
//   - ErrNALUTooLarge: a NAL exceeds the per-NAL fragmentation budget
//     (MaxPayload-2 byte chunks would not round-trip; cannot happen for
//     MaxPayload >= 3 but is asserted for production safety).
//
// The packetizer never truncates, drops or reorders NAL content; every byte
// of a valid access unit appears in exactly one output packet.
var (
	ErrEmptyAU      = errors.New("rtpmedia: access unit contains no NAL units")
	ErrInvalidNALU  = errors.New("rtpmedia: invalid NAL unit type (0 or 24-31) from encoder")
	ErrNALUTooLarge = errors.New("rtpmedia: NALU exceeds maximum fragmentable size")
)

// Packetizer packetizes start-code-stripped H.264 NAL units into RTP payloads.
type Packetizer struct {
	maxPayload int // per-RTP-packet payload budget in bytes
}

// NewPacketizer returns a packetizer with the given per-packet payload
// budget. Values <= 0 default to 1200 B (the DEC-021 measured budget,
// sized to stay clear of MTU fragmentation and encrypted-payload expansion).
func NewPacketizer(maxPayload int) *Packetizer {
	if maxPayload <= 0 {
		maxPayload = 1200
	}
	return &Packetizer{maxPayload: maxPayload}
}

// MaxPayload returns the configured per-packet payload budget.
func (p *Packetizer) MaxPayload() int { return p.maxPayload }

// Packet is one RTP payload with its metadata. Timestamp and sequence
// numbers are assigned by the transport layer; the packetizer fixes the
// payload, marker semantics, and fragmentation boundaries.
type Packet struct {
	Payload []byte
	Marker  bool // true only on the final packet of the access unit
}

// PacketizeAU packetizes one access unit (start-code-stripped NAL units) and
// returns the RTP payloads in order. The RTP marker bit is set on exactly
// the last packet of the access unit. All packets of an access unit share
// one RTP timestamp (set by the caller from the AU PTS via RTPTimestamp).
//
// On any validation error no partial packet list is returned (the AU is
// rejected whole — a half-sent access unit is undecodable anyway).
func (p *Packetizer) PacketizeAU(nals [][]byte) ([]Packet, error) {
	if len(nals) == 0 {
		return nil, ErrEmptyAU
	}
	pkts := make([]Packet, 0, 8)
	for _, nal := range nals {
		if len(nal) == 0 {
			continue
		}
		t := NALType(nal[0])
		if t == 0 || t >= NALTypeSTAPA {
			return nil, ErrInvalidNALU
		}
	}
	for _, nal := range nals {
		if len(nal) == 0 {
			continue
		}
		if len(nal) <= p.maxPayload {
			pkts = append(pkts, Packet{Payload: nal})
			continue
		}
		// FU-A (RFC 6184 §5.8): one byte FU indicator + one byte FU header
		// per packet, at least one payload byte per fragment. maxPayload < 3
		// would produce empty fragments — reject the configuration instead.
		if p.maxPayload < 3 {
			return nil, ErrNALUTooLarge
		}
		nri := NALRefIDC(nal[0])
		nalType := NALType(nal[0])
		indicator := nri | NALTypeFUA
		first := true
		for off := 1; off < len(nal); {
			chunk := p.maxPayload - 2
			if len(nal)-off < chunk {
				chunk = len(nal) - off
			}
			end := off+chunk >= len(nal)
			fuHdr := nalType
			if first {
				fuHdr |= 0x80 // start bit
			}
			if end {
				fuHdr |= 0x40 // end bit
			}
			payload := make([]byte, 0, chunk+2)
			payload = append(payload, indicator, fuHdr)
			payload = append(payload, nal[off:off+chunk]...)
			pkts = append(pkts, Packet{Payload: payload})
			first = false
			off += chunk
		}
	}
	if len(pkts) == 0 {
		return nil, ErrEmptyAU
	}
	// Exactly one marker packet per access unit: the final one.
	for i := range pkts {
		pkts[i].Marker = i == len(pkts)-1
	}
	return pkts, nil
}
