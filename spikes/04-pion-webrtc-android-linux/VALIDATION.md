# Spike 04 — GOP 30 candidate validation

Isolated work (no production code, no protocol changes, no decision-record edits).
Evidence under `results/`. See `GOP-RETUNE.md` for the throttling-strategy proof.

## Harness defects found and fixed

Every remaining gap traced back to a harness bug, not to the media path.

1. **Missing receiver STATS.** `ensure_receiver` launched the receiver as
   `( cd … && "$BIN" … & echo $! > pid )`, so `$!` was the **subshell's** pid, not
   the receiver's (observed: file `179166` vs listener `179167`). `kill` therefore
   signalled the subshell and **orphaned the receiver**, which never flushed STATS
   and kept holding port 7804. Fixed with `exec` inside the subshell, plus a
   hardened `stop_receiver` (TERM → wait → KILL) and orphan cleanup by port.
2. **Per-second health line suppressed.** `-duration > 0` skipped the `t=…s`
   line, which is the only record of 0-fps windows and wake bursts. Now always
   emitted, with `late=` added.
3. **Probe scripts silently empty.** `consent_auto.py`'s `adb` calls inherited
   stdin and adb forwards stdin to the device shell, **draining the heredoc** that
   carried the screen-off probe — so it slept but ran none of its commands. Fixed
   by `stdin=DEVNULL` in the helper and by reading the probe script *before* any
   adb runs. (Same class of defect as the Spike 03 `adb_() { … </dev/null; }` fix.)
4. **Consent automation broken by a second device.** `consent_auto.py` built adb
   calls with no `-s`; once an emulator appeared alongside the phone every dump
   failed with "more than one device/emulator" and looked exactly like "the dialog
   never appeared". Now auto-selects the physical device.
5. **GOP lever plumbed through.** `KEY_FRAME_RATE`, `KEY_I_FRAME_INTERVAL` and the
   tail keep-count are now independent intent extras on every scenario path
   (session, probe, soak).
6. **Per-run receiver stats were overwritten.** `receiver_stats()` wrote a single
   fixed `receiver-stats.txt`, so the 30-minute soak silently destroyed the
   screen-off run's RTP statistics (and vice versa). Now each run also writes
   `receiver-stats-<label>.txt`, `receiver-health-<label>.txt` and
   `receiver-<label>.log`, so every run's evidence survives.
7. **Health-line extraction matched nothing.** The receiver prefixes its per-second
   line with a `log` timestamp (`2026/09/18 00:19:37 t=1s packets=…`), so a `^t=`
   grep produced an empty health file. Pattern now matches the embedded ` t=<n>s`.

## 1. GOP 30 end-to-end, clean run (tail, keep 8)

Encoder config: `KEY_FRAME_RATE=30`, `KEY_I_FRAME_INTERVAL=1` → 30-AU GOP, tail keep 8.

| Metric | Sender | Receiver |
|---|---|---|
| Access units | 640 pushed (2390 encoded, 1750 dropped) | **640** |
| RTP packets | 2315 sent | **2315** |
| H.264 bytes | 2,393,583 sent | 2,364,439 |
| Delivered rate | — | **32.28 AU/s** (matches host-derived 32.15) |
| seq gaps / dup / late / ts-backward | — | **0 / 0 / 0 / 0** |
| Encoded fps | 119.49 | — |
| Keyframes | 80 (4/s) | — |
| Encoded bitrate | 2491.7 kbps | 954.1 kbps payload |
| Encode latency (PTS→dequeued) | p50 20.99 / p90 26.43 / p99 30.82 / max 45.71 ms | — |
| CPU | 91.96% of one core | — |
| Go queue | max depth 5 / 256, 0 drops, push→RTP max 5.97 ms | — |
| **Relative one-way delay** | — | **p50 14.99 / p90 26.66 / p99 46.84 / max 90.12 ms** |

Decoder errors: 0. The transport remains byte-exact under GOP-tail throttling.

## 2. Screen-off → 0 fps → wake, through the live transport (tail, keep 8)

Two independent runs of the same scenario. Reported separately because they differ
in outcome, which is itself the finding.

### 2a. Authoritative run — zero loss (`receiver-stats-screenoff-720x1600.txt`)

Streaming window 61 s; steady rate 30.93 AU/s (32 AU/s) and 110.3 packets/s.
Offsets below are relative to first packet arrival.

| stream+s | Δpackets | ΔAUs | note |
|---|---|---|---|
| …0–25 | ~110 | 32 | steady |
| 26 | 51 | 16 | screen turning off |
| 27 | 5 | 5 | dimming out |
| **28–37** | **0** | **0** | **10 s true 0-fps window** |
| 38 | **335** | 11 | **wake burst = 3.04× steady** |
| 39 | 60 | 24 | recovery |
| 40 | 60 | 32 | **back to steady** |

- **10 consecutive seconds with zero packets and zero AUs.** The sender emits
  nothing while the display is off; the connection stays up (`ice`/`pc` connected,
  `ts_backward=0`) and resumes without a renegotiation.
- **Wake burst: 335 packets/s = 3.04× steady**, but only **11 AUs** — the burst is
  a handful of *large* recovery AUs (the post-wake IDR: `max_frame_b` 146,080 B vs
  25,248 B in the soak), not extra frames.
- **`seq_gaps: 0`, `dup_seq: 0`, `late_packets: 0`, `ts_backward: 0`** across the
  whole cycle → **no loss at all** through the power cycle in this run.
- **Recovery to steady state within 2 s** of the wake.
- Receiver delay tail widened: p50 13.7 / p90 24.5 / p99 94.0 / **max 124.0 ms**.

### 2b. Earlier run under identical config — 44 packets lost

Verified identical config by device JSON: `key_frame_rate=30`,
`key_interval_s=1`, `expected_gop_aus=30`, `keep=8`, `throttle=tail`.

That run recorded **+352 packets in one second (≈5.9× steady) and
`seq_gaps: 44`** (0.9% of 4,712), 0 late packets, delay max 112.4 ms, `ice`/`pc`
connected throughout, recovery in ~2 s (fps avg 95.28, `pushed` 1,528 — both
consistent with a ~11 s outage).

Caveat on reproducibility: that run's per-second series and bitstream were
**overwritten** by the later run, because per-run snapshotting did not exist at
the time — this is precisely the defect fixed as harness bug #6. Its surviving
evidence is the device JSON (`…-20260917-234531.json`) plus the gap count recorded
live. A third run with the same config (`…-20260917-234235.json`) is **excluded**:
its probe body was drained by the stdin bug (#3), so no screen-off ever happened
(fps avg 119.48, no zero window).

### Conclusion for this unknown

A wake burst of **3–6× steady instantaneous rate** is reproducible; whether it
costs packets is **not** deterministic — one run lost 44, one lost 0. The Go queue
never dropped anything in either case (`queue_max` 4–7 of 256), so any loss is
**downstream of the application**, on the path itself. Therefore:

- Go-side pacing/queueing is **required**, and its job is to *shape the wake burst*,
  not merely to buffer inside the app.
- A burst shaper must cap the recovery send rate (observed peaks 3.0–5.9×), and the
  evidence says the burst is ~1 s long.
- Keyframe size, not frame count, drives the burst — so GOP/frame-size awareness
  matters more than AU-rate awareness here.

## 3. 30-minute GOP 30 soak (`soak720p30-gop30`, 1800.004 s)

Config: `c2.qti.avc.encoder` (hardware), 720×1600, 30 fps requested,
CBR 2,500 kbps, `KEY_FRAME_RATE=30`, `KEY_I_FRAME_INTERVAL=1` (30-AU GOP), tail
keep 8. Device: Xiaomi 23049PCD8I (`marblein`, SM7475), Android 15 / SDK 35, 120 Hz.

### Accounting — sender vs receiver, byte-exact

| Metric | Sender (Go) | Receiver (Linux) |
|---|---|---|
| Access units | 57,464 `sent_aus` | **57,464** |
| RTP packets | 207,493 `sent_packets` | **207,493** |
| RTP bytes | 215,855,918 `sent_bytes` | **215,855,918** `bytes_rtp` |
| H.264 payload bytes | — | 213,242,412 `bytes_h264` |
| Delivered rate | — | **31.93 AU/s** |
| seq gaps / dup / late / ts-backward | — | **0 / 0 / 0 / 0** |
| Keyframes | 7,183 (3.99/s) | — |

- **Zero loss for 30 minutes**: 0 sequence gaps, 0 duplicates, 0 late packets,
  0 timestamp regressions.
- `AU-index byte sum == .h264 file size` (both 213,242,412) → the access-unit index
  is exact, so all counts above are trustworthy.

### Decode correctness

- `ffprobe -count_frames`: **57,464 frames decoded**, exactly equal to the AU index.
- `ffmpeg -v error … -f null -`: **exit 0, zero decoder error lines** over the full
  30-minute stream (720×1600 H.264 High, level 3.2).

### Encode-side performance and stability

| Metric | Value |
|---|---|
| Encoded frames / rate | 215,469 @ **119.705 fps** avg |
| Latency (PTS→dequeued, 20,000 samples) | p50 **20.07** / p90 **25.55** / **p99 29.68** / max 82.82 ms, mean 19.86 |
| Negative-latency samples | 0 |
| Encoded bitrate | 2,510.3 kbps (CBR target 2,500 → +0.4%) |
| CPU | 1,596,149 ms over 1,800,004 ms = **88.68% of one core** |
| Go queue | `queue_cap` 256, `queue_max` **4**, drops **0**, panics 0 |
| Go push→RTP worst case | `max_push2send_us` **14.24 ms** |
| Encoder/drain errors | `errors: []`, `notes: []`, `drain_errors: []` |
| First frame | 109 ms |

### Memory — no growth over 30 minutes

| | before | after | Δ |
|---|---|---|---|
| Total PSS | 110,945 KB | 109,474 KB | **−1,471 KB** |
| Native PSS | 11,722 KB | 11,825 KB | **+103 KB** |
| Graphics PSS | 43,452 KB | 43,936 KB | +484 KB |
| Java PSS | 2,206 KB | 7,186 KB | +4,980 KB |
| RSS | 178,280 KB | 154,908 KB | **−23,372 KB** |

Native heap is flat (+103 KB) while ~215 k frames were encoded and ~207 k RTP
packets were sent, and RSS *falls* — no leak signature.

### No performance degradation

Per-second buckets (n = 1,794), first 20 vs last 20:

| Series | mean (all) | first 20 | last 20 | min | max |
|---|---|---|---|---|---|
| `fps` | 119.705 | 119.436 | **119.675** | 114.428 | 122.0 |
| `pushed` (AU/s) | 32.031 | 32.000 | **32.000** | 30 | 35 |
| `kbps` | 2,510.3 | 2,484.5 | **2,503.3** | 2,119.8 | 3,006.0 |
| `frames` | 120.105 | 119.900 | 119.100 | 101 | 123 |

The only sub-120 `fps` bucket is the first (warm-up). Delivery is **perfectly flat
at 32 AU/s** for the entire run.

### Thermal

| | t=0 | t=1800 s |
|---|---|---|
| Thermal status | 0 (NONE) | **0 (NONE)** |
| Battery temp | 36.8 °C | **39.0 °C** |
| Battery level | 95% | 100% (charging) |

**No thermal throttling over 30 minutes** at 120 fps composition + encode + 2.5 Mbps
transport. `dev.phonebridge.spike04` held 88% of one core, `media.hwcodec` 49%,
`surfaceflinger` 77% at the end.

### Artifact size impact (debug build, arm64-v8a only)

| Artifact | Size |
|---|---|
| Debug APK | **10.1 MB** |
| `libphonebridge_spike04.so` (merged, unstripped) | 13.4 MB |
| `lib/arm64-v8a/libphonebridge_spike04.so` inside the APK (stripped) | **9.2 MB** |

The Go+Pion native library is **~91% of the APK** — it dominates everything else in
the spike by an order of magnitude. Stripping removes ~4.2 MB. This is a real
product consideration for DEC-021 (per-ABI size, and whether the Go engine ships in
the app at all vs. a preinstalled/companion component). It is a debug build with no
shrinking, so it is an upper bound, not a shipping figure.

### Toolchain (pinned, not system)

- Go **1.24.13** (`go.mod` `toolchain go1.24.13`, fetched into
  `~/.cache/phonebridge-spike04/`) — not the system Go, which is absent.
- Pion: `webrtc/v4 v4.2.20`, `rtp v1.10.5`, `ice/v4 v4.4.2`, `dtls/v3 v3.1.8`,
  `srtp/v3 v3.0.13`, `interceptor v0.1.49`.
- Receiver builds clean with the pinned toolchain.

### Host-side relative delay (jitter proxy — see §4)

p50 **17.25** / p90 **28.22** / p99 **80.87** / max **237.24 ms**, 57,464 samples,
0 dropped. Higher tail than the 20-second run (p99 46.8 ms) — expected for a
30-minute window: it accumulates every scheduling and GC hiccup, and slow monotonic
clock drift biases it over time. The *median* is essentially unchanged (15.0 → 17.2 ms).

## 4. Latency status — what is and is not measured

**Measured (rigorous):** the RTP timestamp is the MediaCodec PTS, a
`CLOCK_MONOTONIC` value in µs mapped to 90 kHz with **no base reset**. So on the
receiver, `arrival_monotonic_us − rtp_ts_us` equals a **constant clock offset plus
the true one-way delay**. Subtracting the observed minimum gives the **excess
one-way delay**, which is clock-sync independent and therefore exact:

Note the receiver reports this as *delay above the minimum observed sample*
(`minDelayUs` subtracted), so what it publishes is **excess** one-way delay —
i.e. jitter plus any fixed asymmetry — never total transit.

| Component | 20 s GOP 30 run | **30 min soak** |
|---|---|---|
| Relative one-way delay (PTS → arrival, above best case) | p50 15.0 / p90 26.7 / p99 46.8 / max 90.1 ms | **p50 17.25 / p90 28.22 / p99 80.87 / max 237.24 ms** |
| Device encode latency (PTS → buffer dequeued) | p50 20.99 / p90 26.43 / p99 30.82 / max 45.71 ms | **p50 20.07 / p90 25.55 / p99 29.68 / max 82.82 ms** |
| Go push → RTP written | max 5.97 ms | **max 14.24 ms** |

So two of the three legs of the budget are measured directly on-device and are
stable over 30 minutes; only the **network transit leg** is bounded by variation
rather than by an absolute figure.

**Not measured (and cannot be, as instrumented):** *absolute* one-way latency.
The constant offset between the two devices' monotonic clocks is unknowable
without shared time, so no absolute figure is claimed. Two further caveats:
the data-channel RTT (`rtt_us_dc`) reads 0 because the sender never answers the
receiver's ping, so there is currently no RTT cross-check either; and over a
30-minute run, clock rate drift (tens of ppm) can bias the relative delay slowly,
so long-run delay figures should be read as trends, not absolutes.

To make absolute one-way latency measurable would require an explicit handshake
(echo a known sender timestamp and halve the RTT, assuming a symmetric path) —
that assumption should be stated if it is ever used.

## 5. SPS/PPS — a real product blocker found

Scanning every access unit of the GOP-30 capture: **SPS (type 7) appears in 1 of
640 AUs, and only 1 of the 80 IDR AUs carries it.** The encoder emits parameter
sets **once, at stream start** (`prepend-sps-pps-to-idr-frames` is 0 and
`csd-0`/`csd-1` are absent from the output format).

Consequences for a shipping design:
- A receiver that joins late, reconnects, or misses the first access unit **cannot
  decode anything** for the rest of the session — later IDRs are not independently
  decodable without the parameter sets.
- Any packet loss on the first AU is unrecoverable.
- The Go/WebRTC layer must therefore **synthesise and re-inject SPS/PPS**
  (cached from the CSD) ahead of every IDR it forwards, and/or signal them
  out-of-band. This is a transport-layer requirement, not a capture-layer one.

## 6. Remaining blockers

| # | Blocker | Status |
|---|---|---|
| B1 | Absolute one-way latency unmeasured — no clock anchoring between phone `CLOCK_MONOTONIC` and host; data-channel RTT (`rtt_us_dc`) reads 0 because the sender never answers the ping | **Open.** Needs a spike-local handshake (echo a sender timestamp, halve RTT) or the data-channel ping wired up both ways. |
| B2 | `csd-0`/`csd-1` are **null** in the output format and `prepend-sps-pps-to-idr-frames` is 0; parameter sets are sent **once at stream start** (`nal_sps`/`nal_pps` = 1 of 57,464 AUs) | **RESOLVED (spike).** `psiState.prepare` caches CSD and re-injects SPS/PPS ahead of every forwarded IDR; verified on the wire (every IDR now carries SPS+PPS, `BURST-AND-PSI.md` §4) with 0 decoder errors. |
| B3 | `KEY_MAX_FPS_TO_ENCODER=30` is accepted and echoed back but **ignored** (actual 119.7 fps); throttling is Kotlin-side only | **Understood, not fixable at encoder level on this SoC.** Any lower rate must come from the Kotlin/Go path. Needs a check on at least one other device before it is called platform-wide. |
| B4 | Wake burst of 3.0–5.9× steady rate; lost 44 packets in one run and 0 in another | **RESOLVED (spike).** Token-bucket shaper (4 Mbps / 3000 kbit / 500 ms idle cap) eliminated loss in 3/3 shaped trials vs 1-in-3 unshaped (12 packets, 2 hard decoder errors); cost is a bounded tail hold ≤0.9 s (`BURST-AND-PSI.md` §2–3). |
| B5 | GOP-tail `tail` cadence is `~GOP/4` motion then freeze — 192 ms freeze at GOP 30 | **Accepted product cost** for the spike; the freezes are the reason GOP 15 (100 ms) is worth considering if its p99 encode latency tail is acceptable. |
| B6 | Single device, single SoC (SM7475), single encoder (`c2.qti.avc.encoder`), one network path | **Open.** No cross-vendor evidence yet. |

Non-blocking gaps: only two valid screen-off runs (small n for a
probabilistic loss result); encoder behaviour under thermal/power-save restrictions
not tested (thermal status stayed NONE throughout, so it was never exercised); no
multi-ABI (32-bit / armeabi-v7a) size measurement.

## 7. Is Spike 04 sufficient for DEC-020 / DEC-021 ratification?

**For DEC-020 (Kotlin owns capture/encode, Go owns transport/pacing): yes, with
one correction.** The evidence supports the split unambiguously — 30 minutes,
byte-exact delivery, zero loss, zero decoder errors, flat 32 AU/s, no memory
growth, no thermal throttling. But *the decision as currently written is wrong on
one point*: it implies a requested frame rate can be honoured at the encoder.
`KEY_MAX_FPS_TO_ENCODER` is accepted and ignored, so **frame-rate control is a
Kotlin/Go responsibility, and DEC-020 must say so explicitly** rather than
assuming the encoder will throttle.

**For DEC-021 (transport/pacing design): evidence is now sufficient to draft.**
The two transport-specific gaps are closed with prototype + measurements
(`BURST-AND-PSI.md`): a token-bucket shaper (4 Mbps / 3000 kbit / 500 ms idle
cap) removed wake-burst loss in 3/3 trials at a bounded ≤0.9 s worst-case hold,
and SPS/PPS re-injection makes every forwarded IDR independently decodable
(verified on the wire and by decode, with the unshaped lossy run as negative
control). Remaining before ratification: B1 (no clock-anchored absolute latency,
so no end-to-end budget can be asserted) and B3/B6 (single-device encoder
evidence).

Suggested wording constraint for whatever is ratified: state the throttling
strategy (`tail`, GOP 30, keep 8) as a **measured candidate**, not as a frozen
interface, because B3 (device-dependent encoder behaviour) has only one data point.
