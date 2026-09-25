# Spike 07 — Phase 6 Slice 3 renderer prototypes + final gates

Validates the two unknowns of the **additive local-IPC frame transport**
architecture (Go receiver → ffmpeg MJPEG tap → local-IPC gRPC → Flutter
decode/paint) before any production implementation:

- **P1** — daemon-side decode/encode cost, latency, capacity, JPEG sizes,
  wire volume, keyframe re-join recovery. → [P1-RESULTS.md](P1-RESULTS.md)
- **P2** — real local-IPC transport ceiling + Flutter decode/raster/frame-age,
  including a 300 ms UI-stall backpressure proof. → [P2-RESULTS.md](P2-RESULTS.md)
- **P4** — ffmpeg runtime/packaging dependency + missing-binary behaviour.
  → [P4-FFMPEG.md](P4-FFMPEG.md)
- **Final gates** — P3 schema + buf/Go/Flutter gates, real POCO 720p30
  q2/q3/q5 sweep (incl. the missing-SPS/PPS diagnosis), decode-on-arrival
  before/after, decisions + smallest Slice 3 boundary.
  → [FINAL-REPORT.md](FINAL-REPORT.md)

Neither prototype touches production code: everything lives under
`spikes/07-frame-renderer/` (gitignored `results/` holds streams, JPEG corpus
and logs). P2 deliberately reuses the *real* `localipc.NewServer` and the
*real* `LocalIpcClient` so the measured path is the production path.

## Reproduce

```bash
./run_p1.sh        # generates Annex-B test streams + all P1 modes (~3 min)
./run_p2.sh        # 5 rounds: flood64K/flood1M/flood1.7M/decode/stall (~5 min)
```

Requirements: `ffmpeg` with libx264, `flutter`, Go ≥ 1.24 (`$HOME/go-sdk`).

## Layout

```
go/p1/          P1 harness (stdlib): decode floor, MJPEG/I420 flood, paced RTT, recovery
go/p2server/    P2 server: localipc.Server + BroadcastEnvelope flood/paced
dart/           P2 client bench package (flutter test; reuses phonebridge_ui's LocalIpcClient)
results/        streams, JPEG corpus, raw logs (gitignored)
run_p1.sh       P1 orchestrator
run_p2.sh       P2 orchestrator (server rounds ⇄ flutter test phases)
```

## Headline findings

1. **Tap config is load-bearing**: `-threads 1` (or `-flags low_delay`) —
   auto decoder threads add ~0.5 s residency; `-fflags nobuffer` silently
   drops AUs (production `NewDisplaySink` pairs it with `-framedrop`, which is
   a display policy, not a tap policy).
2. **720p30 fits every budget**: AU→JPEG p50 104 ms (70 ms decode is shared
   with ffplay + 34 ms encode), transport p50 1.4 ms, decode 5.3 ms, raster
   1.1 ms → **added over ffplay ≈ 58 ms p50**; capacity 10.8× realtime;
   0.36 cores encode+decode; 2.0 MiB/s wire vs 8.1 MiB/s ceiling.
3. **1080p60 in-app mirroring does not fit DEC-018's 64 KiB chunking**
   (180 msg/s vs 129 msg/s ceiling) — needs a chunk-rule exception decision
   or stays out of scope for Slice 3.
4. **Latest-wins coalescing bounds displayed age** by the stall itself
   (300 ms stall → max painted age 298.7 ms → instant recovery), with zero
   server-side drops at 30 fps.
5. **IDR-aligned re-join is lossless** (180/180 frames, 0 decoder errors) —
   the long-lived ffmpeg tap survives DEC-022 transport repair.

## Open before production (unchanged from the architecture decision)

- **P3**: additive `StreamFrames`/`FrameChunk` schema + `buf lint/breaking`
  (buf not installed locally; CI carries the pinned tool).
- **P4**: packaging `ffmpeg` as an explicit dependency.
- Manual q3-vs-real-capture quality check (synthetic `testsrc2` sizes only).
- Verify p95 frame age with the decode-on-arrival consumer (P2 measured a
  conservative 33 ms tick consumer).
- Decide 1080p60 scope / chunk-rule exception.
