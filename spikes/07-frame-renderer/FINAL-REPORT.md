# Spike 07 / Phase 6 Slice 3 — Final Renderer Gates

Evidence-only report for the four remaining production-readiness gates of the
Phase 6 screen renderer. No production renderer was implemented; all
experimental artifacts live under `spikes/07-frame-renderer/` (gitignored
`results/`). Nothing was committed or pushed.

Validated architecture (unchanged):

```text
H.264 Annex-B → Go receiver → FrameTap → ffmpeg → JPEG (q3)
→ latest-wins / 64 KiB chunks → UDS/gRPC StreamFrames → Flutter dart:ui → Canvas
```

---

## 1. P3 — Protocol: additive `StreamFrames` schema + compatibility

### Schema (smallest additive design)

In `proto/phonebridge/localipc/v1/local_ipc.proto`:

- New RPC `StreamFrames(StreamFramesRequest) returns (stream StreamFramesResponse)`
  — appended after `ListTransfers`, **separate from `StreamEvents`** so frame
  traffic can never drop control events (one-consumer rule preserved: one
  stream carries frames, the existing stream carries events).
- `StreamFramesRequest {}` (empty, forward-compatible).
- `StreamFramesResponse`:

| field | type | semantics |
|---|---|---|
| `frame_id` | `uint64` | monotonic frame id; **gaps permitted = drops** (latest-wins) |
| `chunk_index` | `uint32` | 0-based; chunks of one frame are contiguous + ordered |
| `chunk_count` | `uint32` | total chunks for this frame |
| `last_chunk` | `bool` | final chunk marker (frame complete) |
| `width`, `height` | `uint32` | dimensions per frame (self-describing decode) |
| `jpeg` | `bytes` | ≤ 64 KiB per message (DEC-018 message ceiling) |

Stream semantics documented in the proto: opt-in per client, best-effort
(frame gaps are the drop signal), terminal-end on stream close.

### Compatibility + gate results (buf 1.73.0, CI-pinned)

| gate | result |
|---|---|
| `buf lint` | ✅ OK |
| `buf breaking --against '.git#branch=main,subdir=proto'` | ✅ OK (additive only) |
| `buf generate` | ✅ OK — regenerated Go (`protocolbuffers/go v1.36.12`, `grpc/go v1.5.1` → `core/pkg/protocol/…`) and Dart (`protocolbuffers/dart v25.1.0` → `ui/lib/generated/…`); matches CI plugin pins so the CI drift gate passes on the regenerated set |
| `go build ./...` / `go vet ./...` | ✅ OK (re-verified at report time) |
| `go test -race -count=1 ./...` | ✅ 9/9 packages PASS |
| `flutter analyze` | ✅ clean |
| `flutter test` | ✅ 154/154 PASS |

P3 **passes**. The schema changes remain uncommitted in the working tree
alongside Slices 1–2 (no commits per instructions).

---

## 2. P4 — ffmpeg dependency result

Full detail: [P4-FFMPEG.md](P4-FFMPEG.md). Summary:

- **Probe** (`go/missingffmpeg`): with PATH stripped, sink construction fails
  with *typed* `exec.LookPath` errors (`binary "ffmpeg" not found in PATH` /
  `"ffplay"`) — detectable, not opaque.
- **No new packages**: `ffmpeg` and `ffplay` ship in the same distro package
  (confirmed via `dpkg`); the tap adds zero new dependencies wherever the
  display sink already works.
- **Graceful degradation**: `manager.go StartSession` already converts a
  failed sink → `NullSink` + `SINK_KIND_NULL`; the session never fails because
  of a missing binary. The frame tap must follow the same chain and surface a
  typed reason (recommended: `frames_reason=FFMPEG_MISSING`).
- **Packaging**: `linux/packaging` currently ships only a systemd unit (no
  .deb/Flatpak manifests → Phase 8). Recommendation: `.deb` with
  `Recommends: ffmpeg` (**not** `Depends` — renderer must degrade, not block),
  ffmpeg ≥ 4.4; systemd user unit inherits PATH (verified). **Flatpak remains
  an open risk** (no host exec under strict sandbox).

P4 **passes** (documented, no production files modified).

---

## 3. Real capture — POCO F5 → Linux at 720p30, q2/q3/q5

### Capture

`results/real/poco-720p30.h264` via the production engine
(discovery → trust → HTTP signaling → Pion → `FileSink`):

- 4,121,033 bytes, **216 AUs / 27 IDRs**, packets=3546, seq_gaps=0, dup=0,
  late=0, dropped=0, state STREAMING, actual **720×1600 @ 2500 kbps h264**,
  exit 0. GOP-tail delivered ≈ 8 fps (real DEC-021 operating point).

### Gate-3 blocker — diagnosis and resolution

First `p1 decode` run failed (`exit 69`, `non-existing PPS 0 referenced`).
Root cause is a **data property of the recorded stream, not the harness**:

- NAL census of the raw file: **189 × type-1 + 27 × type-5, zero type-7/8** —
  the recording contains **no SPS/PPS anywhere** (all 216 `.idx` rows:
  `sps=0 pps=0`).
- Plain `ffmpeg -f h264 -i file -f null -` fails identically → **not** the
  aggressive `-probesize 32` (that hypothesis is rejected).
- Resolution (measurement-only, no production code touched): splice the
  same-device parameter sets (SPS 18 B / PPS 5 B, byte-identical to the
  `psi_test` fixtures and the spike04 recording from this same
  23049PCD8I/SM7475 encoder) → `poco-720p30-spliced.h264` decodes
  **216/216 frames, 0 errors**, 720×1600 High.

⚠️ The underlying finding — **the live production wire carried no parameter
sets in this session despite DEC-021 obligation 2** — is carried forward as
risk R1 in §8.

### Measurements (`sweep2.log`, threads=1 where paced)

Decode floor (auto threads): **216 frames, 947 fps** (≈31× realtime), 1.54 cpu-s.

Size + capacity (flood, `p1 mjpeg`):

| q | avg | p50 | p95 | max | flood fps | cpu/216-flood | ≈cores @30fps |
|---|---|---|---|---|---|---|---|
| 2 | 93.6 KB | 100.1 KB | 140.8 KB | 145.5 KB | 458.9 | 3.17 s | 0.44 |
| 3 | 74.0 KB | 78.4 KB | 112.4 KB | 116.5 KB | 416.1 | 3.15 s | 0.44 |
| 5 | 56.5 KB | 57.8 KB | 85.5 KB | 88.3 KB | 449.0 | 3.26 s | 0.45 |

Paced 30 fps AU→JPEG latency (`p1 rtt`, threads=1, all 216 AUs):

| q | p50 | p95 | max | achieved fps |
|---|---|---|---|---|
| 2 | 105.5 ms | 110.5 ms | 114.2 ms | 30.36 |
| 3 | 105.2 ms | 109.2 ms | 115.9 ms | 30.42 |
| 5 | 71.7 ms | 77.1 ms | 82.5 ms | 30.25 |
| decode-only (`framecrc`) | 72.0 ms | 77.1 ms | 81.7 ms | 30.24 |

Consistent with P1's synthetic results (q3 AU→JPEG p50 104 ms, ≈70 ms of
which is ffmpeg pipeline residency shared with the display path; the tap's
*incremental* cost over ffplay is the MJPEG encode stage ≈34 ms, per
P1/P2). Real noisy camera-viewfinder content costs slightly more than
synthetic (`testsrc2`): 0.44 vs 0.36 cores at 30 fps.

Wire/chunking at 30 fps: q3 = 2.2 MB/s avg, worst frame 116.5 KB → 2 chunks
(60 msg/s); even q2 worst frame 145.5 KB → 3 chunks (90 msg/s) — all well
under the measured transport ceilings (129 msg/s @64 KiB, 8.1 MiB/s).

### Visual inspection (text / thin lines / artifacts)

Method: frames 0/60/130/200 encoded at q2/q3/q5, plus 2× nearest-neighbour
crops of (a) the camera mode-label text row (small white-on-black UI text)
and (b) the viewfinder's code-text/panel edges (thinnest lines in content)
— `results/real/qcmp/compare.html`, `compare2.html`.

- **q2 vs q3: indistinguishable** at 2× on both text and thin lines; no
  ringing, no blocking, no chroma bleed on the red/green diff panels.
- **q5: measurably softer** — the finest text strokes blur together and the
  sharpest line edges fuzz, though still legible; still no blocking/ringing.
- All three: zero decode failures, correct dimensions.

---

## 4. Decode-on-arrival (gate 4) — before/after

The prototype's fixed 33 ms consumer tick was replaced in
`dart/test/p2_bench_test.dart` with decode-on-arrival: decode immediately
when idle, retain only the newest frame while a decode is in flight, and pick
it up the moment the current decode completes (no timer). RSS sampled at
window start / after each paint / window end. Same server, same paced 30 fps
corpus, same phases (`results/p2/*.client.log`).

### decode phase

| metric | before (33 ms tick) | after (decode-on-arrival) |
|---|---|---|
| received / painted | 271 / 240 | 271 / **271** (every frame painted) |
| coalesced_dropped | 31 | **0** |
| decode_fail | 0 | 0 |
| transport age p50/p95/max | 1.4 / 2.8 / 25.8 ms | 1.3 / 2.2 / 18.2 ms |
| decode ms p50/p95/max | 5.3 / 7.8 / 14.2 | 3.7 / 7.3 / 12.3 |
| raster ms p50/p95/max | 1.1 / 2.6 / 4.8 | 0.9 / 2.6 / 5.6 |
| **paint age p50/p95/max** | **24.4 / 39.1 / 48.1 ms** | **6.5 / 10.7 / 38.1 ms** |
| RSS start/end/peak | not measured | 166.4 / 178.4 / 197.8 MB (bounded) |

### stall phase (300 ms simulated isolate jank)

| metric | before | after |
|---|---|---|
| received / painted | 280 / 238 | 280 / **273** |
| coalesced_dropped | 41 | **7** |
| paint age p50/p95/max | 20.5 / 37.4 / 298.7 ms | 7.8 / 11.7 / 296.6 ms |
| post-stall paint age p50/p95/max | 21.3 / 36.4 / 298.7 ms | **7.8 / 11.4** / 296.6 ms |
| transport age max | 288.4 ms | 291.4 ms |
| decode_fail | 0 | 0 |
| RSS start/end/peak | not measured | 167.0 / 178.9 / 181.9 MB (bounded) |

Interpretation:

- **p95 send→painted: 39.1 → 10.7 ms**; the ≤60 ms combined-added-latency
  gate now has margin: tap encode ≈34/37 ms (P1) + 6.5/10.7 ms ≈
  **41 ms p50 / 48 ms p95 ✅** (was 58/76 ms — p95 previously missed the gate
  by ~16 ms purely because of the tick; hypothesis confirmed).
- Stall backlog stays **bounded by the stall itself** (max ≈297 ms ≈ the
  300 ms jank), and recovery p95 drops 36.4 → 11.4 ms.
- Memory bounded over the window (no monotonic growth; peak ≈ start + 31 MB
  including the flutter_test harness).

Gate 4 **passes**.

---

## 5. Decisions

| decision | value | evidence |
|---|---|---|
| **Resolution / FPS** | **720×1600 @ 30 fps** (DEC-021 operating point) | only configuration measured across P1–P4 + real capture; 1080p60 still violates the 64 KiB rule (180 msg/s > 129 msg/s measured ceiling) |
| **JPEG quality** | **`-q:v 3`** | q2 costs +26% bytes for imperceptible gain (2× crops identical); q5 saves 24% bytes but visibly softens the finest UI text — and reading phone UI text is the product; q3 avg 74 KB real / 67 KB synthetic fits every budget |
| **Chunk size** | **≤ 64 KiB per `FrameChunk`** (keep DEC-018; no exception) | q3 worst measured frame 116.5 KB → 2 chunks, 60 msg/s avg/worst-case < 129 msg/s ceiling; wire 2.2 MB/s ≪ 8.1 MiB/s. Larger messages buy nothing at 720p30 |

## 6. Unresolved risks

- **R1 — no parameter sets on the live wire (highest priority).** The real
  capture contains zero SPS/PPS NALs across 216 AUs / 27 IDRs, so the raw
  recording (and any receiver/tap joining this session) is undecodable
  without out-of-band parameter sets. DEC-021 claims every IDR carries
  SPS+PPS; spike04-era recordings confirm it did then. Most likely: the
  encoder's `CODEC_CONFIG` AU was consumed by an earlier transport instance
  (or not emitted this run) so the per-session `rtpmedia.Cache` never learned
  SPS/PPS and `IDRsNoCache` counted every IDR — **not conclusively
  confirmed** (the logcat window had rotated). The ffmpeg tap inherits this
  failure mode directly: with no parameter sets, the tap emits nothing.
  Slice 3 must include a receiver-side PSI guard (reuse `rtpmedia.Cache`,
  prepend cached SPS/PPS before the tap, emit a counter/typed
  `frames_reason`) and surface `IDRsNoCache` in stats.
- **R2 — latency provenance.** Paced AU→JPEG residency (≈105 ms q3) includes
  ffmpeg pipeline buffering and matches P1; the incremental-over-ffplay
  figure (≈34 ms) comes from P1's stage measurement, not a full
  capture→paint clock-anchored run (no clock anchoring — DEC-021 limitation).
- **R3 — single-device evidence.** One POCO F5 / SM7475 / one Wi-Fi path;
  sizes measured on worst-case noisy camera content (typical UI screens are
  cheaper).
- **R4 — GOP-tail cadence.** At the DEC-021 operating point delivered ≈8 fps
  with motion ≈59 ms then freeze ≈192 ms per 250 ms GOP; the renderer will
  show bursty motion. Product decision, not a renderer defect.
- **R5 — environment fidelity.** Flutter numbers come from flutter_test
  (debug Dart, no release AOT/GPU raster) — conservative (P2 §5).
- **R6 — packaging.** Flatpak host-exec risk open; .deb should `Recommends:
  ffmpeg`; typed `FFMPEG_MISSING` probe still to be added (Phase 8 / P4).
- **R7 — one-consumer rule.** `StreamFrames` keeps frames off
  `StreamEvents` by schema, but the implementation slice must enforce a
  single frame subscription and prove control events are never dropped
  under frame load (test required).

## 7. Smallest production Slice 3 boundary

In scope:

1. **Go tap** (receiver side): tee AUs from the existing receiver path into
   an ffmpeg subprocess with the verified flags (`-threads 1`, **no**
   `-fflags nobuffer`, `-q:v 3`, mjpeg) → JPEG bytes.
2. **PSI guard** (R1 mitigation): receiver-side `rtpmedia.Cache` prepend +
   counter/typed reason before the tap.
3. **Frame assembler**: latest-wins drop, ≤64 KiB chunking, emit on the
   already-merged `StreamFrames` RPC of the existing `localipc.Server`.
4. **Flutter consumer**: second, dedicated `LocalIpcClient` subscription →
   decode-on-arrival coalescer (measured p95 10.7 ms) → `dart:ui` paint at
   720×1600.
5. **Degradation**: ffmpeg missing → tap off + typed
   `frames_reason=FFMPEG_MISSING`, session unaffected (P4 chain).
6. **Exit criteria**: combined added latency p95 ≤ 60 ms (measured ≈48 ms),
   0 decode failures, control-event losslessness under frame load, buf/CI
   gates green.

Out of scope: production UI polish, multi-consumer fan-out, 1080p60,
packaging automation (Phase 8), and any Android/WebRTC/discovery change
(none were made or needed for measurement).

---

### Reproduce

```bash
./run_p1.sh                    # synthetic P1 modes
# real capture: results/real/poco-720p30-spliced.h264 (see §3)
go/bin/p1 decode|mjpeg|rtt -i results/real/poco-720p30-spliced.h264 -q {2,3,5}
./run_p2.sh                    # P2 rounds (flood/decode/stall)
```

Artifacts: `results/real/sweep2.log`, `results/real/qcmp/`,
`results/p2/{decode,stall}.client.log`.
