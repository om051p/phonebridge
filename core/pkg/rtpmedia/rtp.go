package rtpmedia

// RTP timestamp conversion (DEC-021; Spike 04 VALIDATION.md).
//
// The RTP clock is 90 kHz and the timestamp is derived from the MediaCodec
// PTS (µs). PTS is monotonic on the validated device with no base reset —
// verified byte-exact over the 30-minute soak (57,464 AUs, ts_backward=0) —
// so the conversion is a pure scaling with wrap-around in uint32 as
// specified by RFC 3550. The absolute value is meaningless (no clock
// anchoring: absolute one-way delay remains unmeasured) but monotonicity and
// inter-frame deltas are exactly the sender's encode cadence.

// RTPClockHz is the RTP clock rate for H.264 per RFC 6184.
const RTPClockHz = 90000

// RTPTimestamp converts a MediaCodec PTS in microseconds to an RTP timestamp
// in the 90 kHz clock. ptsUs may be any int64 (monotonic sources start near
// 0); the result wraps modulo 2^32 per RFC 3550.
func RTPTimestamp(ptsUs int64) uint32 {
	// 90000/1e6 = 9/100 exactly, matching the spike-verified conversion.
	return uint32(uint64(ptsUs) * 9 / 100)
}
