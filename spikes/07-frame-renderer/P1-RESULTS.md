# Spike 07 — P1 Results: daemon-side decode/encode cost (Phase 6 Slice 3)

Machine: Linux x86_64, 24 cores, ffmpeg 8.0.1 (libx264/libavcodec), go1.24.13.
Streams: synthetic `testsrc2`, Annex-B, `repeat-headers=1`, no B-frames,
GOP 30/60 — mimicking DEC-020/021 framing (SPS/PPS before every IDR).
Reproduce: `./run_p1.sh` (raw log: `results/p1.log`).

> All numbers are from the committed harness (`go/p1`), which feeds parsed
> access units into `ffmpeg` over a pipe exactly as the proposed renderer tap
> would. Latency = AU handed to ffmpeg stdin → frame observable on stdout.

## TL;DR

| Question | Answer |
|---|---|
| Capacity | ✅ Ample: MJPEG flood ≥ **322 fps @720p** (10.8× realtime 30) and **148 fps @1080p60** (2.5× realtime 60), even single-threaded |
| Latency AU→JPEG | ✅ **720p30: p50 104 ms / p95 107 ms**; **1080p60: p50 73 ms / p95 77 ms** — with `-threads 1` |
| The latency knob | ⚠️ **Decoder thread count**: auto-threads adds **~0.5 s** residency; `-threads 1` (or `-flags low_delay`) is mandatory for the tap |
| Wire cost | 720p30 ≈ **2.0 MB/s**, 1080p60 ≈ **7.5 MB/s** at q3 (JPEG avg 67 KB / 130 KB) |
| I420 (no re-encode) | ❌ 49–111 MB/s **and** no dart:ui paint path for YUV — rejected |
| Keyframe re-join (DEC-021 model) | ✅ IDR-aligned rejoin: **180/180 frames, 0 decoder errors** |
| CPU (decode+encode) | 720p30 ≈ **0.36 cores**, 1080p60 ≈ **1.9 cores** at realtime |
| New finding | 🐛 production `NewDisplaySink` passes **`-fflags nobuffer`, which drops 30–60 AUs per run** on the raw demuxer (display-tolerable alongside `-framedrop`; fatal for a lossless tap) |

## 1. Decode floor (shared with the current ffplay renderer)

Same libavcodec input path, `-probesize 32 -analyzeduration 0`:

| Stream | Decode throughput | CPU |
|---|---|---|
| 720×1600@30 (900 AU) | 1339–1877 fps (45–62× realtime) | 4.0 s cpu / 900 frames ≈ 4.5 ms/frame |
| 1080×2400@60 (1200 AU) | 861 fps (14× realtime) | 36 ms/frame (from transcode runs) |

This residency is **not added cost** — ffplay pays it too.

## 2. Decoder residency vs thread count (paced 30 fps, framecrc probe)

| Config | p50 | p95 | Notes |
|---|---|---|---|
| `-threads 1` | **70.6 ms** | 72.7 ms | stable, no drift |
| `-threads 2` | 100.8 ms | 101.9 ms | +1 frame per thread |
| `-threads 4` | 167.6 ms | 168.8 ms | |
| auto (≈24 threads) | **567.4 ms** | 568.6 ms | frame-parallel fill ≈ (threads−1)×frame time |
| auto + `-flags low_delay` | **71.0 ms** | 73.6 ms | what production ffplay passes (`NewDisplaySink(lowLatency=true)`) |
| auto + low_delay + `-fflags nobuffer` | 1070 ms | — | **and 30/300 frames silently dropped** |

## 3. Full tap latency: AU → JPEG (mjpeg `-q:v 3`, paced, no backlog: achieved fps = target)

| Stream | Config | p50 | p95 | max |
|---|---|---|---|---|
| 720p30 | `-threads 1` | **104.3 ms** | 106.6 ms | 111.1 ms |
| 720p30 | auto | 668.8 ms | 675.5 ms | 681.0 ms |
| 720p30 | auto + low_delay | 178.1 ms | 188.2 ms | 273.3 ms |
| 720p30 @60 pace | `-threads 1` | 87.6 ms | 90.3 ms | 98.6 ms |
| 1080p60 | `-threads 1` | **73.1 ms** | 76.8 ms | 79.7 ms |
| 1080p60 | `-threads 4` | 102.0 ms | 108.4 ms | 116.9 ms |
| 1080p60 | auto | 336.9 ms | 441.4 ms | 579.8 ms |
| 1080p60 | auto + low_delay | 113.3 ms | 123.1 ms | 146.3 ms |

**Chosen tap config: `-threads 1` (plus `-probesize 32 -analyzeduration 0`,
never `-fflags nobuffer`).** Decomposition at 720p30: ~70 ms decode residency
(shared with ffplay) + ~34 ms MJPEG encode/pipe = 104 ms p50.

## 4. Capacity (flood = max sustainable throughput)

| Stream | q | threads | flood fps | headroom vs realtime | cpu per frame |
|---|---|---|---|---|---|
| 720p30 | 3 | auto | 602.8 | 20× | 14.8 ms |
| 720p30 | 3 | 1 | **322.8** | **10.8×** | 11.9 ms |
| 720p30 | 3 | 2 | 419.0 | 14× | 14.0 ms |
| 720p30 | 3 | 4 | 522.6 | 17× | 14.8 ms |
| 720p30 | 2 / 5 | auto | 556.9 / 685.7 | 19–23× | — |
| 1080p60 | 3 | auto | 255.7 | 4.3× | 36.2 ms |
| 1080p60 | 3 | 1 | **147.7** | **2.5×** | 31.4 ms |
| 1080p60 | 3 | 4 | 229.6 | 3.8× | 39.4 ms |

Realtime CPU (decode+MJPEG encode): 720p30 ≈ 11.9 ms/frame × 30 ≈ **0.36 cores**;
1080p60 ≈ 31.4 ms × 60 ≈ **1.9 cores**.

## 5. JPEG sizes & wire cost (synthetic content — see caveats)

| Stream | q2 avg | q3 avg | q3 IDR | q5 avg | q3 wire rate |
|---|---|---|---|---|---|
| 720p30 | 79.5 KB | **67.4 KB** | 69.9 KB (p95 72.3) | 53.8 KB | 67.4 KB × 30 ≈ **2.0 MB/s** |
| 1080p60 | 151.9 KB | **129.6 KB** | 147.5 KB (p95 151.6) | — | 129.6 KB × 60 ≈ **7.5 MB/s** |

All sizes fit DEC-018's ≤64 KiB bulk rule only partially — IDR JPEGs (70–152 KB)
exceed one 64 KiB chunk, confirming the chunk/reassemble requirement.

## 6. I420 (rawvideo, no re-encode) — rejected

| Stream | bytes/frame | @ target rate |
|---|---|---|
| 720p | 1,728,000 (1.65 MiB) | 49.4 MiB/s |
| 1080p | 3,888,000 (3.71 MiB) | 111.2 MiB/s |

25× the JPEG wire volume, **and** dart:ui cannot paint YUV — the UI would need a
pure-Dart YUV→RGB conversion (~1.15 M pixels/frame) that no repo component
provides. JPEG re-encode is the only variant with a zero-new-dependency path on
both sides.

## 7. Keyframe re-join recovery (DEC-021 re-injection model)

| Sequence | AUs in | frames out | stderr error lines | verdict |
|---|---|---|---|---|
| segA → segB (IDR-aligned, SPS/PPS-carrying) | 180 | **180** | **0** | ✅ clean |
| segA → segC (mid-GOP cut, negative control) | 180 | 172 | 0 | ⚠️ frames silently lost; libav concealed without stderr on synthetic content |

The tap's requirement — a long-lived ffmpeg surviving a DEC-022 transport
rerepair where the next AU after the gap is an IDR with SPS/PPS — is proven
lossless and error-free. (Mid-GOP corruption observability is content-dependent;
DEC-021's SSIM evidence remains authoritative for that failure mode.)

## 8. Verdict vs the P1 gate (≤60 ms added over ffplay)

- **Shared with ffplay:** decode residency ≈ 70 ms (ffplay: `low_delay`;
  tap: `-threads 1`).
- **Added by the tap (P1 part):** MJPEG encode + pipe ≈ **34 ms** (720p30).
- **Remaining budget for P2** (IPC + Dart decode + paint): **≤ ~26 ms** to hold
  the ≤60 ms added-latency gate. P2 measures exactly this.
- Capacity, CPU, recovery, and wire volume all pass with margin. The two
  hazards discovered (`-fflags nobuffer` frame loss; auto-thread ~0.5 s
  residency) are configuration rules, not architecture blockers.

## Caveats

- `testsrc2` is synthetic: JPEG byte sizes on real phone screens (text, status
  bars, video) will differ; q3 quality must be eyeballed on a real capture
  (manual step — display timing was not measured automatically).
- One machine (24-core x86_64); decoder thread scaling will differ elsewhere.
- ffplay's window/render stage was not timed; its decode stage is covered by
  the shared-residency measurement.
- Latency pairs AU-write → frame-readable on stdout; socket reads add µs–ms
  (measured in P2).
