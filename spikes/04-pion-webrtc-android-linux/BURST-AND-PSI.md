# Spike 04 — Wake-burst shaping and SPS/PPS re-injection (DEC-021 prerequisites)

Isolated spike work. No production code, no decision-record edits, nothing committed.
Raw evidence: `results/{receiver-stats,receiver-health}-burst-*.txt`,
`results/captures/burst-*.{h264,idx}`, `results/device/results/spike04-session-burst-*.json`,
`results/decode-burst-*.log`. Companion to `VALIDATION.md` and `GOP-RETUNE.md`.

## 1. What was built (spike-local)

**Token-bucket burst shaper** (`go/bridge.go`). Applied per RTP packet inside the
single-writer send loop, *before* sequence-number assignment. Configuration is
passed from Kotlin (`shapeKbps`, `shapeBurstK` intent extras) through
`GoBridge.start(kbps, burstK, psi)` into `nativeStart`.

- Safety by construction: packets are only **delayed**, never reordered, re-split
  or re-timestamped — AU ordering, RTP timestamps, marker bits, FU-A framing and
  IDR integrity are untouched because emission order is identical to the unshaped
  path.
- Refill is capped at `maxIdleCredit = 500 ms`. Without this, tokens banked during
  the 10 s screen-off silence would release the wake burst at line rate — the
  exact scenario the shaper exists for.
- 4 Mbps ceiling is measurement-derived: worst observed burst ≈ 5.9× steady
  ≈ 4.1 Mbps. Bucket depth 3000 kbit ≈ 0.75 s of headroom.

**SPS/PPS re-injection** (`psiState.prepare`): the engine caches the parameter
sets from the CSD (first AU) and from any in-band repetition, then prepends
SPS+PPS to **every forwarded IDR AU** that does not already carry them. The
receiver's AU index now records per-AU SPS/PPS counts
(`idx bytes rtp_ts idr marker sps pps`) so re-injection is verifiable on the wire.

## 2. Burst characterization and A/B results (screen-off 12 s inside 60 s session)

Six trials, identical encoder config (720×1600, GOP 30, tail keep 8, PSI on).
Unshaped 2's first attempt and shaped 2's first attempt died device-side right
after consent (no crash-buffer entry; OS kill) and were re-run — the retry data
below is the successful pass.

| run | shape | rx pkt | rx AU | seq gaps | burst pkt/s | burst MB/s | burst AU | OWD p50/p90/max (ms) | decoder errors |
|---|---|---|---|---|---|---|---|---|---|
| burst-unshaped-1 | off | 5105 | 1504 | **12** | **648** | **0.75** | 16 | 23.4 / 61.0 / 114.8 | **2** |
| burst-unshaped-2 | off | 5074 | 1536 | 0 | 325 | 0.38 | 29 | 18.4 / 29.5 / 130.0 | 0 |
| burst-unshaped-3 | off | 5047 | 1540 | 0 | 327 | 0.38 | 32 | 17.8 / 38.0 / 92.1 | 0 |
| burst-shaped4-1 | **4000 kbps** | 4892 | 1504 | **0** | 417 | 0.48 | 17 | 16.3 / 25.5 / 142.6 | 0 |
| burst-shaped4-2 | **4000 kbps** | 4891 | 1504 | **0** | 293 | 0.34 | 33 | 18.8 / 28.9 / 118.5 | 0 |
| burst-shaped4-3 | **4000 kbps** | 5639 | 1576 | **0** | 356 | 0.41 | 3 | 18.2 / 30.3 / **584.5** | 0 |

Burst window anatomy (receiver per-second series, offsets from first packet):

| run | 0-fps window | peak burst second | back to 32 AU/s |
|---|---|---|---|
| unshaped-1 | 11 s | 648 pkt ≈ 6.0 Mbps (5.9× steady 91–96 pkt/s) | +1 s |
| unshaped-2/3 | 10 s | ~325 pkt ≈ 3.0 Mbps | +3 s / +1 s |
| shaped4-1 | 11 s | **417 pkt = exactly the 4 Mbps cap** (4e6/8/1200) | +1 s |
| shaped4-2/3 | 10 / 9 s | 293 / 356 pkt (bucket not exhausted) | +1 s |

Sender evidence (device JSON): queue max 4–7/256 unshaped, 4–16 shaped, **0 queue
drops everywhere**; shaper max-hold per packet 0.6–16.9 ms (only shaped runs);
largest AU 107–196 KB (post-wake IDR).

## 3. The loss/latency trade-off

- **Unshaped is a coin flip.** 1 of 3 trials lost 12 packets at a 648 pkt/s peak;
  the other two peaked at ~325 pkt/s and lost nothing. Consistent with the two
  earlier runs (335 pkt/s/0 lost, 352 pkt/s/44 lost): whenever the burst exceeds
  ~350–400 pkt/s on this Wi-Fi path, packets drop.
- **Shaped is loss-free in all 3 trials** and capped the peak at ≤417 pkt/s
  (the configured ceiling) while steady-state throughput was untouched.
- **Latency cost is concentrated, not distributed.** p50/p90 are *identical* to
  unshaped (16–19 / 25–30 ms). The only cost appears in the max tail:
  shaped4-3 hit 584.5 ms because its post-wake IDR was 195 KB — a single AU needs
  ~390 ms to emit at 4 Mbps (195 KB × 8 / 4 Mbps), on top of the 200 ms bucket
  drain. That delay is *bounded by design*: bucket 3000 kbit + packet 1200 B at
  4 Mbps ⇒ worst-case hold ≈ 0.9 s for one frame's worth of packets.
- Interpretation: shaping converts *packet loss* (unrecoverable without NACK/RTX,
  which the bare sender does not implement) into *bounded delivery delay*
  (absorbed by a jitter buffer). For a remote-control use case, a one-off
  ≤0.6 s catch-up delay after a wake is acceptable; mid-stream loss is not.
- Alternative lever recorded for DEC-021: `S4Log` confirms the post-wake IDR size
  dominates the tail. A `MediaCodec.PARAMETER_KEY_REQUEST_SYNC_FRAME` on wake
  would *re-time* the burst but not shrink it; true tail reduction needs either
  intra-refresh or a higher cap on a path known to carry it.

## 4. SPS/PPS re-injection evidence

Before this change (measured, `VALIDATION.md` §5): parameter sets appeared in
**1 of 57,464 AUs** — only the stream-start AU carried them; a late join or
post-loss receiver could never decode.

After (per-run receiver stats + AU index):

| run | AUs carrying SPS | IDRs | IDRs missing SPS |
|---|---|---|---|
| burst-shaped4-1 | 188 | 80 | **0** |
| burst-shaped4-2 | 188 | 80 | **0** |
| burst-shaped4-3 | 197 | 197 | **0** |
| burst-unshaped-1/2/3 (PSI on, unshaped) | 188/192/193 | 80/66/22 | 0 |

(sender counters: `psi_cached_sps_b=18`, `psi_cached_pps_b=5`,
`psi_injected_idrs=79–196`, `psi_inband_idrs=1` — i.e. every IDR except the
stream-start AU, which already carries them, is completed.)

Decode validation of the re-injected streams: `ffprobe -count_frames` = AU count
exactly (1504/1576), `ffmpeg -v error` = **0 decoder error lines**. The negative
control is stark: **burst-unshaped-1, the run that lost 12 packets, produces 2
hard decoder errors** (`left block unavailable for requested intra mode`,
`error while decoding MB 0 8`) — proof that the observed loss class corrupts
decoding, and that the shaped runs avoid it.

## 5. Residual risks / limitations

1. n = 3 per arm, one device, one Wi-Fi path. The loss threshold (~350–400 pkt/s)
   is path-specific; the *mechanism* (cap instantaneous rate below path capacity)
   is not.
2. The shaper assumes single-track video. Multi-track or audio+video would need
   per-track budgets.
3. The 584 ms max tail is bounded but nonzero; interactive use may want an
   adaptive cap (drop the stale non-key backlog on wake instead of draining it).
4. PSI re-injection duplicates parameter sets ≈ every 250 ms at GOP 30 — ~0.3%
   overhead at 2.5 Mbps (measured: 188 SPS × 23 B ≈ 4.3 KB/s vs 2.5 Mbps).
   Negligible, but a production design might throttle to one set per IDR *change*.
5. Absolute one-way latency remains unmeasured (no clock anchoring); all delay
   figures here are excess-over-best-case on the receiver, as in `VALIDATION.md` §4.

## 6. Recommendation for DEC-021

Evidence is now sufficient to **draft** DEC-021 with these specifics:

- Transport must include a **token-bucket shaper** at the RTP emission point
  (ordering-preserving), sized from the measured burst profile: ceiling ≈ 4 Mbps,
  bucket ≈ 3000 kbit, idle-credit cap ≈ 500 ms. This replaced the observed
  3–6× wake bursts (and their 12–44-packet loss events) with zero loss in all
  shaped trials at a bounded ≤0.9 s worst-case hold.
- Transport must **cache and re-inject SPS/PPS ahead of every forwarded IDR**;
  relying on stream-start parameter sets is proven unsafe (1 in 57,464 AUs;
  late join un decodable; first-AU loss unrecoverable).
- Wake-burst pacing and PSI re-injection are the two transport obligations the
  30-minute soak did not cover; both now have spike evidence, decoded-output
  verification, and negative controls.

Not yet DEC-021 material (unchanged from `VALIDATION.md` §6): absolute one-way
latency (B1), cross-device encoder behaviour (B3/B6).
