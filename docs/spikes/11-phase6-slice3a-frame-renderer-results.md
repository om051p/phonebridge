# Phase 6 Slice 3A Results: Frame Renderer + Producer-Side PSI Fix

> **Status:** `COMPLETE` · **Classification:** `VALIDATED` (beryllium + SM7475 / POCO F5)
> **Date:** 2026-09-25 · **Prerequisites:** Spike 04 (DEC-021), DEC-022,
> Spike 07 prototypes · **Decision:** DEC-025 · **Commits:**
> `37b9ca2` (producer PSI) + this renderer-stack commit

## 1. What shipped

Production frame path, additive to everything existing:

```text
Android MediaProjection → H.264 Annex-B → Pion/WebRTC → receiver
→ PSIGuardSink → frames.TapSink (ffmpeg MJPEG -q:v 3) → frames.Hub
→ StreamFrames (≤64 KiB, separate from StreamEvents)
→ Flutter decode-on-arrival coalescer → dart:ui paint
```

Files: `core/pkg/frames/`, `core/pkg/receiver/psiguard.go`, localipc
`StreamFrames` RPC + regenerated proto (Go/Dart), engine tap wiring +
`frames_reason`, daemon hub, Flutter consumer,
`ui/test/frames_e2e_test.dart` (opt-in via `PHONEBRIDGE_E2E=1`).

## 2. The PSI blocker and the fix

**Symptom (live E2E):** `frames_reason=PARAM_SETS_MISSING`, `sps=0 pps=0`
— bare IDRs on the wire. **Root cause:** the encoder emits SPS/PPS once per
codec lifetime (CSD AU), but `LanSignalingServer.handleOffer` rebuilds the
transport (`mediaRelease`+`mediaInit`) after capture starts; the CSD was
learned into the pre-offer Sender's per-Sender cache and discarded — and
`MediaOnFrame()` gated on init before learning.
**Fix:** transport-owned capture-scoped `psi` cache shared by every rebuilt
Sender (`SenderConfig.PSICache`); `MediaOnFrame()` learns via `LearnPSI`
before the state gate (JNI engine gate learns on its early-return branch
only); `Cache.Learn` caches plausible sets only (SPS ≥ 4 B, PPS ≥ 2 B);
`MediaStatsJSON` exposes `psi*` counters. Regression: CSD before/after init,
rebuild survival, relearn, missing PSI, duplicate/corrupt sets, shipped-lib
host-JVM probe (`GoBridgePsiProbeTest`).

## 3. Physical E2E evidence (POCO F1 / beryllium, Android 15)

Flow: consent ("Entire screen" — the Android 15 dialog defaults to
"A single app") → animated test pattern → daemon from this tree →
`PHONEBRIDGE_E2E=1 flutter test test/frames_e2e_test.dart`.

| Gate | Result |
|---|---|
| 720×1600 @ 30 fps tuple | capture 720×1600 @ 30; actual 720×1600 @ **16.0 delivered (GOP-tail; cause measured and closed-form — §7)** |
| JPEG q3 | tap args carry `-q:v 3` (avg JPEG ≈ 26 KB) |
| max chunk ≤ 64 KiB | `oversizeChunks=0` |
| zero decode failures | `decodeFailures=0` |
| p95 send→paint ≤ 60 ms | p50 ≈ 6.7 ms, **p95 ≈ 11 ms**, max ≈ 20 ms |
| bounded memory | Flutter RSS ≈ 138→173 MiB (debug harness, no growth) |
| control events lossless | `StreamEvents` seq gapless under frame load |
| no stale frame after Stop | painted count frozen after `stopSession` |
| reconnect restores video | Wi-Fi down/up → `TRANSPORT_FAILED` → 1 attempt → STREAMING |

**Device caveat (resolved in Slice 3C):** the established POCO F5 (DEC-021 baseline,
SM7475) was initially unattached during Slice 3A. The cross-device numbers on
beryllium/SD845 (`OMX.qcom.video.encoder.avc`) were subsequently complemented
by direct POCO F5 hardware validation in Slice 3C (§3.1).

### 3.1. Baseline Device Evidence (POCO F5 / SM7475 / marblein, Android 15)

Hardware validation on the baseline device (`89ceabd9`, `c2.qti.avc.encoder`,
120 Hz AMOLED panel) executed in Slice 3C via multi-session probe runs across
both 120 Hz and 60 Hz motion regimes.

| Metric | 120 Hz Composition (`EXTRA_ANIMATE`) | 60 Hz Composition (Chrome Canvas) |
|---|---|---|
| Encoder FPS (measured) | 120.0 fps (`Δenc=240` / 2 s) | 59.9 fps (`Δenc=120` / 2 s) |
| Keyframe Cadence (measured) | 4.0 IDR/s (`Δkf=8` / 2 s; 250 ms interval) | 2.0 IDR/s (`Δkf=4` / 2 s; 500 ms interval) |
| GOP Length (arithmetic) | 30 AUs (30 frames configured interval) | 30 AUs (30 frames configured interval) |
| Admitted AU/s (arithmetic) | **32.0 admitted AU/s** ($8 \times 4$ GOP/s) | **16.0 admitted AU/s** ($8 \times 2$ GOP/s) |
| Delivered FPS (measured mean) | **30.2 delivered fps** (range 28.0–31.6) | **14.5 delivered fps** (range 14.1–15.2) |
| Access Units / 12 s (measured) | ~390 AUs (~32.5 AU/s) | ~196 AUs (~16.3 AU/s) |
| Multi-cycle run result | **8/8 cycles delivered** (0 failures) | **6/6 cycles delivered** (0 failures) |
| `frames_reason` | `""` across all cycles | `""` across all cycles |
| Max chunk / decode failures | ≤64 KiB / 0 decode failures | ≤64 KiB / 0 decode failures |

Teardown across all cycles was clean: daemon PID unchanged, zero lingering
processes (`ffmpeg`/`ffplay`), port 7804 preserved.

## 4. Secondary fixes found during validation

- `_makeCallOptions({timeout})` used `timeout ?? callTimeout`, so the
  long-lived streams died with `DEADLINE_EXCEEDED` 10 s in — now
  `noTimeout: true` (verified error-free for 15 s).
- E2E harness: `pong.nonce` is fixnum `Int64` — compare `.toInt()`.
- `local_ipc_client_test.dart` leaked its subscription (`.first` no cancel);
  now subscribes + cancels.

## 5. Cross-session frame stall — RESOLVED (Slice 3B)

**Symptom (observed):** 3/8 runs on one long-lived daemon: STREAMING with
growing `access_units`, but the tap's ffmpeg emits no JPEG (`painted=0–1`,
`frames_reason` healthy). Clean daemon restarts always pass. Repro: same
daemon, stop session N, start N+1, probe `StreamFrames` 10 s.

**Root cause.** `frames.Hub` session ownership was a global `active` bool with
no correlation between an `EndSession` call and the session it belonged to.
The manager reports terminal states from a callback that runs **after** the
transition is applied and its mutex released (`manager.go`, state callback),
with no generation guard — so session N's terminal callback can land *after*
session N+1 has called `BeginSession`, and `StartSession` accepts N+1
precisely *because* N's state is already terminal. The late `EndSession()`
then cleared `active`, closed **every** subscriber channel, and permanently
killed N+1's frame window. The daemon still reported STREAMING with
`access_units` growing (the snapshot reads transport stats directly,
independent of the hub) but no JPEG could reach a subscriber; only a further
`BeginSession` repaired it — which N+1 had already spent. This matches
"restarting the daemon restores it" exactly.

**Fix (smallest responsible layer — `frames.Hub` ownership, no renderer
architecture change).**
- Session ownership is a **token**, not a flag: `BeginSession` mints a fresh
  token and returns it; `EndSession(tok)` applies only to the session that
  minted `tok`, returning `ErrSessionNotCurrent` for a superseded one — a late
  terminal callback degrades to a no-op instead of a cross-session kill.
- `BeginSession` while another is open **supersedes** it (closes its
  subscribers, mints a new token) rather than aliasing the old token.
- `EndSessionIfCurrent(tok)` performs the ownership check atomically under the
  hub lock, closing the check-then-act gap in the manager.
- `manager.go` carries the token in a dedicated mutex-guarded cell (written by
  the connect goroutine, read by the state callback; the callback must not
  take `m.mu`).

**Regression tests** (`core/pkg/frames/frames_test.go`):
`TestHubSupersededSessionCannotEndItsSuccessor` (the ordering regression:
session 1's terminal callback lands after session 2 begins),
`TestHubEndSessionIfCurrentRejectsStaleToken` (atomic ownership check),
plus the existing lifecycle/latest-wins coverage. An out-of-repo ordering
probe reproduced the bug before the fix (`BUG REPRODUCED: session 2 is live but
Subscribe cannot register a subscription; active=false`) and passed after.

**Hardware proof (POCO F1 / beryllium, one long-lived daemon, repeated
start→measure→stop cycles, 12 s measurement each):**

Six-cycle run (Slice 3C, the strongest evidence — every cycle delivered after
the preceding session ended, daemon PID unchanged throughout):

| Cycle | Session | Frames | Delivered fps | access_units | frames_reason | Status |
|---|---|---|---|---|---|---|
| 1 | `53eff29f8972` | 188 | 15.7 | 200 | `""` | OK |
| 2 | `9b42693f7d53` | 188 | 15.7 | 196 | `""` | OK |
| 3 | `6ecac810ad46` | 189 | 15.8 | 197 | `""` | OK |
| 4 | `239094407069` | 184 | 15.3 | 198 | `""` | OK |
| 5 | `277980e2dffc` | 168 | 14.0 | 196 | `""` | OK |
| 6 | `4d3961848478` | 192 | 16.0 | 200 | `""` | OK |

`RESULT: 6/6 cycles delivered frames, 0 failures`. An earlier three-cycle run
gave 187 / 182 / 141. After all cycles: one daemon process, **zero** leftover
`ffmpeg`/`ffplay`, port 7804 still held by the same PID, and no error or panic
lines in the daemon log — teardown is deterministic (`PSIGuardSink.Close` →
`TapSink.Close`, which waits on the converter's `done` channel before closing
its inner sink).

> **Harness caveat (important when reproducing).** Two runs showed a later
> cycle as "never reached STREAMING" or `STALL` with `access_units` far below
> the healthy ~200 (e.g. 15). Both were **the probe's on-screen motion loop
> expiring**, not a stall: the phone's encoder is damage-driven, so with no
> content change it emits ~0 fps and the daemon legitimately has no AUs to
> convert. The discriminator is `access_units` — a healthy cycle shows
> ~190–200 AUs per 12 s, a motion-starved cycle shows a near-zero count.
> Rerunning with continuous motion (a long-drag swipe that cannot expire
> mid-run) reproduced the intended clean 6/6.

## 7. Delivered-FPS cause — measured (Slice 3B & 3C)

The requested tuple is 30 fps; delivery measures **16.0 fps** on 60 Hz panels
(or 60 Hz composition) and **30.2–32.0 fps** on 120 Hz panels under sustained
motion. The cause is the **sender-side GOP-tail filter**, not encoder
scheduling, the receiver, the tap, ffmpeg, or Flutter.

### 7.1. POCO F1 (beryllium, SD845, 60 Hz Panel) Evidence (Slice 3B)

Evidence from the device (`adb -s <serial> logcat -s ScreenCaptureEngine`), steady state:

```text
CAPTURE_STATS: encoded=5299 (60.0 fps), delivered=1140 (16.0 fps), gop_dropped=3883, keyframes=177
```

Chain, each link measured rather than inferred:

- The Android hardware H.264 encoder ignores the requested fps and runs at
  panel refresh — **~60 fps measured** on beryllium/SD845 (`OMX.qcom.video.encoder.avc`).
- `core/pkg/frames/tap.go`'s `ffmpegArgs()` carries **no `-r`/framerate flag**,
  so the tap publishes at whatever rate AUs arrive; the tap is not the limiter.
- `GopTailFilter` (`CaptureConfig`: `KEY_FRAME_RATE=30`,
  `KEY_I_FRAME_INTERVAL=1`, `DEFAULT_KEEP_FRAMES=8`) admits a contiguous
  **prefix** of each GOP — the IDR plus the first 7 P-frames (8 AUs total) —
  and drops the tail. The prefix rule is deliberate and prediction-safe
  (DEC-021: a naive 1-of-4 stride drops P-frames whose references are needed
  later; the contiguous prefix is safe by construction, SSIM = 1.000000).

**The arithmetic closes exactly.** Differencing two consecutive sampled
`CAPTURE_STATS` lines (Δt = 2.0 s) gives `Δencoded = 120`, `Δdelivered = 32`,
`Δgop_dropped = 88`, `Δkeyframes = 4`:

```text
dt=2s: enc+120 del+32 drop+88 kf+4 | GOP~30 AUs  IDR_every=0.5s
```

- GOP ≈ 120 / 4 = **30 AUs**
- IDR actually every **0.5 s**, not the requested 1 s — the encoder ignores
  `KEY_I_FRAME_INTERVAL` wall-clock seconds and produces an IDR every 30 frames
- 8 admitted × 4 GOP per 2 s = 32 per 2 s = **16.0 fps** — matching the measured value

### 7.2. POCO F5 (marblein, SM7475, 120 Hz AMOLED) Evidence (Slice 3C)

Evidence from the POCO F5 (`89ceabd9`, `c2.qti.avc.encoder`, Android 15)
confirms the identical GOP arithmetic at both 120 Hz and 60 Hz composition.

#### A. 120 Hz Panel Refresh (`EXTRA_ANIMATE` test pattern)

Measured `CAPTURE_STATS` delta (Δt = 2.0 s):
```text
CAPTURE_STATS: encoded=12140 (120.0 fps), delivered=3237 (32.0 fps), gop_dropped=8903, keyframes=405
dt=2s: Δenc=240  Δdel=64  Δdrop=176  Δkf=8 | GOP=30 AUs  IDR_every=0.25s
```

- **Encoder FPS (measured):** 120.0 fps (runs at 120 Hz panel refresh)
- **IDR Cadence (measured):** 8 keyframes per 2 s = 4.0 IDR/s = **0.25 s interval** (250 ms)
- **GOP AU count (arithmetic):** 240 / 8 = **30 AUs** per GOP
- **Admitted AU rate (arithmetic):** 8 admitted AUs × 4.0 GOP/s = **32.0 admitted AU/s**
- **Delivered FPS (measured via probe):** 8-cycle consecutive run (12 s each):
  366, 366, 379, 364, 372, 336, 344, 372 frames = **30.2 delivered fps mean**
  (30.5, 30.5, 31.6, 30.3, 31.0, 28.0, 28.7, 31.0 fps)
- **Access units (measured):** ~390 AUs per 12 s = **32.5 AU/s**
- **Result:** `8/8 cycles delivered frames, 0 failures`, `frames_reason=""`.

This directly validates the `CaptureConfig.kt` doc comment:
`"Keep first 8 frames per GOP -> ~32 delivered AUs/sec"`.
That comment was authored on the SM7475 where the 120 Hz panel refresh drives
4 GOP/s!

#### B. 60 Hz Composition (Browser canvas motion)

Measured `CAPTURE_STATS` delta (Δt = 2.0 s):
```text
CAPTURE_STATS: encoded=4820 (59.9 fps), delivered=1285 (16.0 fps), gop_dropped=3535, keyframes=161
dt=2s: Δenc=120  Δdel=32  Δdrop=88  Δkf=4 | GOP=30 AUs  IDR_every=0.50s
```

- **Encoder FPS (measured):** 59.9 fps
- **IDR Cadence (measured):** 4 keyframes per 2 s = 2.0 IDR/s = **0.50 s interval** (500 ms)
- **GOP AU count (arithmetic):** 120 / 4 = **30 AUs** per GOP
- **Admitted AU rate (arithmetic):** 8 admitted AUs × 2.0 GOP/s = **16.0 admitted AU/s**
- **Delivered FPS (measured via probe):** 6-cycle consecutive run (12 s each):
  174, 182, 174, 169, 173 frames = **14.5 delivered fps mean** (14.1–15.2 fps)
- **Access units (measured):** ~196 AUs per 12 s = **16.3 AU/s**
- **Result:** `6/6 cycles delivered frames, 0 failures`, `frames_reason=""`.

### 7.3. Universal Qualcomm Hardware Encoder GOP Sizing Model

Both Qualcomm encoders (`c2.qti.avc.encoder` on SM7475 and
`OMX.qcom.video.encoder.avc` on SD845) compute GOP length by counting frames,
not by measuring wall-clock time:

$$\text{GOP AU count} = \text{KEY\_FRAME\_RATE} \times \text{KEY\_I\_FRAME\_INTERVAL} = 30 \times 1 = 30\text{ frames}$$

Because screen capture is damage-driven, the encoder produces frames at the
effective display composition rate ($R_{\text{comp}}$):

$$\text{GOP rate (GOP/s)} = \frac{R_{\text{comp}}}{\text{GOP AU count}} = \frac{R_{\text{comp}}}{30}$$

$$\text{IDR interval (s)} = \frac{1}{\text{GOP rate}} = \frac{30}{R_{\text{comp}}}$$

Under `DEFAULT_KEEP_FRAMES = 8`, the admitted frame rate is strictly:

$$\text{Admitted FPS} = \text{keepFrames} \times \text{GOP rate} = 8 \times \frac{R_{\text{comp}}}{30}$$

| Parameter | 60 Hz Composition / Panel (F1 or F5) | 120 Hz Panel Refresh (F5) |
|---|---|---|
| Encoder rate $R_{\text{comp}}$ | 60 fps | 120 fps |
| GOP AU count | 30 AUs | 30 AUs |
| IDR interval | 0.50 s (500 ms) | 0.25 s (250 ms) |
| GOP cadence | 2.0 GOP/s | 4.0 GOP/s |
| Tail drop per GOP | 22 AUs dropped (73.3%) | 22 AUs dropped (73.3%) |
| Admitted AU rate | **16.0 AU/s** | **32.0 AU/s** |
| Measured Delivered FPS | **14.5–16.0 fps** | **30.2–31.6 fps** |

Conclusion: the ~15–16 fps (at 60 Hz) and ~30–32 fps (at 120 Hz) results are
**expected, deterministic GopTail behavior, not defects**: the pipeline delivers
exactly what the filter admits with zero decode failures and p95 ≈ 11 ms
send→paint. Tuning `keepFrames` remains a future policy choice (deferred).

## 8. How to re-run

```bash
make -C core android-lib            # .so → jniLibs/
cd ui && flutter build apk --debug  # verify: strings … | grep psiHaveSPSPP
adb -s <serial> install -r ui/build/app/outputs/flutter-apk/app-debug.apk
make -C core daemon && ./core/build/phonebridge-daemon
# phone: consent with "Entire screen" (+ moving content for delivery)
PHONEBRIDGE_E2E=1 flutter test test/frames_e2e_test.dart
```

**Cross-session check (Slice 3B):** with the daemon running and capture live
under an *animated* source, drive three start→measure→stop cycles in a row and
confirm each cycle reports non-zero frames. Motion matters: a static screen
makes the phone encoder idle, which is not a stall.

```bash
# per cycle: StartSession → poll for SESSION_STATE_STREAMING → count frames on
# StreamFrames for ~12 s → StopSession. Success = every cycle non-zero.
```

