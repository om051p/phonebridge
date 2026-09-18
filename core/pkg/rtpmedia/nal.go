package rtpmedia

// NAL unit type classification (ITU-T H.264 §7.4.1).

// H.264 NAL unit types used by the production pipeline.
const (
	NALTypeSlice  = 1 // coded slice of a non-IDR picture (P)
	NALTypeIDR    = 5 // coded slice of an IDR picture
	NALTypeSEI    = 6
	NALTypeSPS    = 7  // sequence parameter set
	NALTypePPS    = 8  // picture parameter set
	NALTypeSTAPA  = 24 // RFC 6184 aggregation packets (transport only)
	NALTypeSTAPB  = 25
	NALTypeMTAP16 = 26
	NALTypeMTAP24 = 27
	NALTypeFUA    = 28 // RFC 6184 fragmentation units (transport only)
	NALTypeFUB    = 29
)

// NALType returns the 5-bit type field of a NAL header byte.
func NALType(b byte) byte { return b & 0x1F }

// NALRefIDC returns the 2-bit nal_ref_idc field of a NAL header byte.
func NALRefIDC(b byte) byte { return b & 0x60 }

// IsParameterSet reports whether the NAL is an SPS or PPS.
func IsParameterSet(nal []byte) bool {
	if len(nal) == 0 {
		return false
	}
	t := NALType(nal[0])
	return t == NALTypeSPS || t == NALTypePPS
}

// HasIDR reports whether the access unit (as NAL units) contains an IDR
// slice.
func HasIDR(nals [][]byte) bool {
	for _, n := range nals {
		if len(n) > 0 && NALType(n[0]) == NALTypeIDR {
			return true
		}
	}
	return false
}

// classify walks the NALs once, recording which parameter sets and slice
// types are present. It is the shared pre-pass of Cache.Prepare and
// Throttle.Observe.
type auClass struct {
	hasSPS, hasPPS, hasIDR, hasNonIDRSlice bool
}

func classify(nals [][]byte) auClass {
	var c auClass
	for _, n := range nals {
		if len(n) == 0 {
			continue
		}
		switch NALType(n[0]) {
		case NALTypeSPS:
			c.hasSPS = true
		case NALTypePPS:
			c.hasPPS = true
		case NALTypeIDR:
			c.hasIDR = true
		case NALTypeSlice:
			c.hasNonIDRSlice = true
		}
	}
	return c
}
