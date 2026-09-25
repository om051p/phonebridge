# Spike 07 — P2 Results: local-IPC frame transport + Flutter decode/paint

Machine: Linux x86_64, 24 cores, Flutter 3.47.2 (`flutter test`, debug JIT),
grpc-dart 5.x. Path under test = **the real production stack end-to-end**:
`localipc.NewServer` (SO_PEERCRED + bearer interceptors) → gRPC `StreamEvents`
server-stream over a Unix domain socket → production `LocalIpcClient`
(grpc-dart) → Dart protobuf parse → `dart:ui` JPEG decode → raster
(`Picture.toImage`, software raster in the test harness).

Payload framing is bench-local: `ClipboardUpdate.payload = [8-byte send µs][bytes]`.
Reproduce: `./run_p2.sh` (logs in `results/p2/`).

## TL;DR

| Question | Answer |
|---|---|
| Transport ceiling (64 KiB messages) | **129 msg/s, 8.1 MiB/s** — 720p30 needs 60 msg/s / 2.0 MiB/s ✅ |
| Transport ceiling (1 MiB / 1.65 MiB) | **24.3 msg/s @ 24.3 MiB/s** / **22.9 msg/s @ 37.8 MiB/s** |
| 1080p60 feasibility | ❌ with DEC-018's 64 KiB chunking (180 msg/s > 129 ceiling); ✅ only with messages >64 KiB (1×130 KB msg/frame = 60 msg/s) — scope/chunk-rule decision needed |
| I420-over-IPC | ❌ again: 49.4 MiB/s needed vs 37.8 MiB/s max measured at that message size |
| End-to-end frame age (30 fps, send→painted) | **p50 24.4 ms, p95 39.1 ms, max 48.1 ms** |
| JPEG decode (720p, engine) | p50 **5.3 ms**, p95 7.8 ms — cheap |
| Raster (720p full-frame) | p50 **1.1 ms**, p95 2.6 ms |
| Transport latency (UDS+gRPC+proto parse) | p50 **1.4 ms**, p95 2.8 ms, max 25.8 ms |
| 300 ms UI-stall resilience | ✅ displayed age bounded by the stall itself (max 298.7 ms), p95 after recovery 36.4 ms, zero server-side drops, instant drain via latest-wins coalescing |
| Decode failures | 0 of 551 real JPEG frames |

## 1. Transport ceiling (flood mode: server enqueues as fast as the subscriber
channel drains; client measures a settled 5 s window)

| Payload | msg/s | MiB/s | Notes |
|---|---|---|---|
| 65,544 B (≈64 KiB, DEC-018 bulk rule) | **129.1** | **8.1** | ≈7.75 ms per message |
| 1,048,584 B (1 MiB) | **24.3** | **24.3** | ≈41 ms per message |
| 1,728,008 B (I420 720p frame) | **22.9** | **37.8** | ≈44 ms per message |

Server-side enqueue rate was 7–8 M/s with drop-newest at the 64-slot
subscriber channel — i.e. the client pipeline is the binding constraint, as it
should be. The cost model is **not linear** in size; do not extrapolate —
re-measure if the frame message size changes.

### What each streaming scenario needs

| Scenario @ target rate | msgs/frame | msg/s needed | bytes/s | Fits ceiling? |
|---|---|---|---|---|
| 720p30, chunked ≤64 KiB (2×~34 KB) | 2 | 60 | 2.0 MiB/s | ✅ 46% of msg ceiling, 25% of byte ceiling |
| 720p30, single ~67 KB message (exceeds the rule as written) | 1 | 30 | 2.0 MiB/s | ✅ large margin |
| 1080p60, chunked ≤64 KiB (3 msgs) | 3 | **180** | 7.5 MiB/s | ❌ msg rate > 129 |
| 1080p60, single ~130 KB message (chunk-rule exception) | 1 | 60 | 7.5 MiB/s | ✅ likely, but at ~90% of interpolated msg-rate — verify before committing |
| 720p30 I420, single 1.65 MB message | 1 | 30 | 49.4 MiB/s | ❌ 49.4 > 37.8 measured max |

Paced runs (30 fps × ~66 KB real JPEGs): server `subscriber_drops=0` —
real traffic never touches the 64-slot buffer, so no frames are lost by design
at the target rate.

## 2. Frame pipeline latency (paced 30 fps, real JPEG corpus, latest-wins
coalescer painting on a 33 ms tick)

`p2_decode received=271 painted=240 coalesced_dropped=31 skipped_ticks=2 decode_fail=0`

| Stage | p50 | p95 | max |
|---|---|---|---|
| transport (send µs → Dart listener) | **1.4 ms** | 2.8 ms | 25.8 ms |
| JPEG decode (`instantiateImageCodec`, 720p) | **5.3 ms** | 7.8 ms | 14.2 ms |
| raster (`Picture.toImage(720,1600)`) | **1.1 ms** | 2.6 ms | 4.8 ms |
| **end-to-end (send → painted)** | **24.4 ms** | **39.1 ms** | 48.1 ms |

End-to-end includes the coalescer tick wait (0–33 ms by construction): the
measured consumer waits for the next 33 ms tick before decoding. A consumer
that decodes on arrival (coalescing only while busy) removes up to one tick —
bounded improvement of ≤33 ms, to be confirmed in the production slice.

Caveats: debug-JIT Dart is *slower* than release AOT, so these numbers are a
conservative bound for the real app; the test rasterizer is software — a GPU
raster should be no slower. Decode+raster CPU ≈ 6.4 ms/frame ≈ **0.19 cores**
at 30 fps.

## 3. Stall / backpressure proof (300 ms isolate stall at frame ~91)

```
INFO stall applied: 300.0 ms at received=91
received=280 painted=238 coalesced_dropped=41 decode_fail=0
transport_age_ms p50=1.3 p95=3.2 max=288.4
paint_age_ms    p50=20.5 p95=37.4 max=298.7
post_stall_paint_age_ms p50=21.3 p95=36.4 max=298.7
```

- Messages queued during the stall arrive **at most ~288 ms old** — the age
  ceiling equals the stall; nothing accumulates beyond it (gRPC/HTTP-2 buffers
  ~10 in-flight messages at 30 fps, far below the 64-slot server channel, so
  `subscriber_drops=0`).
- The oldest frame painted after recovery was 298.7 ms old; **p95 of painted
  age right after the stall is 36.4 ms** — i.e. the latest-wins coalescer
  drains the backlog instantly and the very next frames are back to steady
  state. No queue growth, no thundering herd of stale frames.
- This validates the proposed drop policy: **age-bounded (latest-wins), never
  queue-bounded lag.**

## 4. Verdict vs the P2 gate (≤ ~26 ms so the combined added latency over
ffplay holds ≤ 60 ms)

P1 established: decode residency ≈ 70 ms is shared with ffplay; the tap's
daemon-side addition is ~34 ms (MJPEG encode + pipe).

| Component | p50 | p95 |
|---|---|---|
| tap encode (P1) | 34 ms | ~37 ms |
| P2 send→painted, as measured (33 ms tick consumer) | 24.4 ms | 39.1 ms |
| **added over ffplay** | **≈ 58 ms ✅** | **≈ 76 ms ⚠️** |
| P2 with decode-on-arrival consumer (≤1 tick saved) | ≈ 24 → ≈ 12–24 | ≈ 39 → **≈ 25–40 ✅** |

- **p50 already meets the ≤60 ms gate with the conservative consumer.**
- **p95 misses it by ~16 ms purely because of the 33 ms tick** — a design
  choice, not a transport/decode limit. Decode-on-arrival (coalesce only while
  a decode is in flight) is the required production shape; verify p95 in the
  implementation slice.
- **Update (gate 4, measured):** decode-on-arrival replaced the tick in
  `dart/test/p2_bench_test.dart`; send→painted p95 dropped **39.1 → 10.7 ms**
  (p50 24.4 → 6.5 ms), every received frame painted, stall recovery p95
  36.4 → 11.4 ms, RSS bounded. Combined added over ffplay ≈ 41/48 ms
  (p50/p95) — gate ≤60 ms now met at p95. Full tables:
  [FINAL-REPORT.md](FINAL-REPORT.md) §4.
- If even that is too tight, the next lever is P1's `-q` (q5 cuts JPEG bytes
  20% and decode cost proportionally) or accepting 720p30-only in-app mirror.

## 5. What P2 did NOT measure

- Production `FrameChunk` protobuf shape (bench used `ClipboardUpdate.payload`
  as a byte carrier — same wire cost, different schema; schema itself is P3).
- Server-side cost of a tee sink feeding both ffplay and the IPC tap
  (P1 measured the ffmpeg stage standalone).
- True GPU rasterization and release-AOT Dart (flutter_test is conservative).
- Multiple concurrent UI frame consumers — by design there is exactly one
  (one-consumer rule); the bench enforced a single subscription.
