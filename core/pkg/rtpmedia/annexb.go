package rtpmedia

// Annex-B scanning (ITU-T H.264 Annex B byte-stream format).
//
// Re-derived from the Spike 04 packetize path (validated byte-exact against
// real device captures; see packetizer_test.go golden tests). Semantics:
//
//   - Start codes are 00 00 01 (3-byte) or 00 00 00 01 (4-byte). MediaCodec
//     emits 4-byte codes; both forms must parse.
//   - Zero bytes between NALs (leading/trailing_zero_8bits) and zero padding
//     are separators, never payload.
//   - Bytes before the first start code are ignored.
//   - Emulation-prevention sequences (00 00 03) inside a NAL are payload and
//     are never mistaken for start codes.
//   - Zero-length NALs (adjacent start codes) are skipped.
//
// Returned NALs are subslices of the input buffer; callers must treat them
// as read-only and must not hold them past the next mutation of the buffer.
// A NAL payload never begins with 00 00 01 (that byte sequence IS a start
// code), which keeps the scan forward-only and unambiguous.

// AnnexBIter iterates the NAL units of one Annex-B byte stream.
type AnnexBIter struct {
	buf []byte
	pos int // next scan position
}

// NewAnnexBIter returns an iterator over b.
func NewAnnexBIter(b []byte) *AnnexBIter { return &AnnexBIter{buf: b} }

// findStartCode locates the next start code at or after `from`. It returns
// the code start — including one extra leading zero byte when the 4-byte form
// is used, so separators are excluded from the preceding NAL — and the
// payload offset.
func findStartCode(b []byte, from int) (codeStart, payloadStart int, ok bool) {
	for i := from; i+2 < len(b); i++ {
		if b[i] == 0 && b[i+1] == 0 && b[i+2] == 1 {
			cs := i
			if cs > 0 && b[cs-1] == 0 {
				cs--
			}
			return cs, i + 3, true
		}
	}
	return 0, 0, false
}

// Next returns the next NAL unit with the start code stripped, or ok=false
// when the stream is exhausted.
func (it *AnnexBIter) Next() (nal []byte, ok bool) {
	for {
		_, ps, found := findStartCode(it.buf, it.pos)
		if !found {
			it.pos = len(it.buf)
			return nil, false
		}
		it.pos = ps
		end := len(it.buf)
		if cs, _, next := findStartCode(it.buf, ps); next {
			end = cs
		}
		// Trim trailing zero separators / padding; never cross the payload.
		for end > ps && it.buf[end-1] == 0 {
			end--
		}
		if end > ps {
			return it.buf[ps:end:end], true
		}
		// Zero-length NAL (adjacent start codes): skip this code. Resuming at
		// the payload offset is exact — anything real at ps would have made
		// the NAL non-empty — and strictly advances the scan.
		it.pos = ps
	}
}

// SplitAnnexB returns all NAL units of one Annex-B access unit.
// Convenience wrapper over AnnexBIter.
func SplitAnnexB(au []byte) [][]byte {
	var nals [][]byte
	it := NewAnnexBIter(au)
	for {
		nal, ok := it.Next()
		if !ok {
			return nals
		}
		nals = append(nals, nal)
	}
}
