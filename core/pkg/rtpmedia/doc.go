// Package rtpmedia implements the production H.264-over-RTP media primitives
// ratified by DEC-020/DEC-021 (Spike 04) and re-derived from the validated
// spike implementation. Foundation Step 1: send-side primitives only, pure
// stdlib, host-testable. It deliberately contains no Pion, JNI, signaling or
// session code — the transport session (pkg/webrtc) and the JNI surface
// (cmd/android) integrate these primitives in later foundation steps.
//
// Components and their measured provenance (see docs/decisions.md, DEC-021):
//
//   - Queue: bounded access-unit queue (256 slots default) with the DEC-020
//     drop policy — drop the incoming non-key frame on a full queue, evict
//     oldest frames to admit a key frame. The producer (capture side) never
//     blocks.
//   - Throttle: GOP-tail frame throttling. The validated encoder stream is
//     IPPP with no B-frames or reordering, so keeping a contiguous prefix
//     (IDR + first K AUs) of each GOP is prediction-safe by construction;
//     naive striding is not (SSIM 1.0 vs 0.9504, Spike 04 GOP-RETUNE.md).
//   - Cache: H.264 parameter-set (SPS/PPS) CSD cache with re-injection ahead
//     of every forwarded IDR lacking them. Measured: parameter sets occur
//     once per stream (1 of 57,464 AUs), so late join / reconnect / first-AU
//     loss is undecodable without this; ≈0.3% bandwidth overhead.
//   - Packetizer: RFC 6184 packetization (single NALU or FU-A, default MTU
//     1200 B), marker bit only on the final packet of an access unit.
//     Returns errors on invalid input (empty AU, NAL types 0/24–31) instead
//     of silently mangling the stream.
//   - Shaper: token-bucket burst shaper applied per RTP packet at the
//     emission point, before sequence-number assignment, so AU ordering,
//     timestamps, marker bits and IDR integrity are preserved by
//     construction. Measured defaults: 4 Mbps ceiling, 3000 kbit bucket,
//     500 ms idle-credit cap (bursts beyond the cap are the exact scenario
//     the shaper exists for; unshaped wake bursts lost packets and produced
//     hard decoder errors).
//   - RTPTimestamp: MediaCodec PTS (µs) → 90 kHz RTP timestamp conversion
//     (PTS is monotonic with no base reset — verified byte-exact over the
//     30-minute soak).
//
// Golden tests under testdata/ run every component against a real 40-AU
// slice of the Spike 04 burst-shaped capture (receiver-side AU index
// included), so production behaviour is pinned to actual device output.
//
// Status: Foundation Step 1 (implemented, unit+golden tested). Runtime wiring
// is Step 2+ per the production integration plan.
package rtpmedia
