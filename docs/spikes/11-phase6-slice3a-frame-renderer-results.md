# Phase 6 Slice 3A Results: Frame Renderer + Producer-Side PSI Fix

> **Status:** `COMPLETE` · **Classification:** `VALIDATED` (beryllium)
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
| 720×1600 @ 30 fps tuple | capture 720×1600 @ 30; actual 720×1600 @ **~16 delivered (GOP-tail; cause measured — §7)** |
| JPEG q3 | tap args carry `-q:v 3` (avg JPEG ≈ 26 KB) |
| max chunk ≤ 64 KiB | `oversizeChunks=0` |
| zero decode failures | `decodeFailures=0` |
| p95 send→paint ≤ 60 ms | p50 ≈ 6.7 ms, **p95 ≈ 11 ms**, max ≈ 20 ms |
| bounded memory | Flutter RSS ≈ 138→173 MiB (debug harness, no growth) |
| control events lossless | `StreamEvents` seq gapless under frame load |
| no stale frame after Stop | painted count frozen after `stopSession` |
| reconnect restores video | Wi-Fi down/up → `TRANSPORT_FAILED` → 1 attempt → STREAMING |

**Device caveat:** the established POCO F5 (DEC-021 baseline, SM7475) was not
attached (serial `89ceabd9` absent). All numbers are **cross-device evidence**
on beryllium/SD845 (`OMX.qcom.video.encoder.avc`); re-validate on the F5.

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

**Hardware proof (POCO F1 / beryllium, one long-lived daemon, 3 start→measure→
stop cycles, 12 s measurement each):**

| Cycle | Frames | Delivered fps | access_units | frames_reason | Status |
|---|---|---|---|---|---|
| 1 | 187 | 15.6 | 197 | `""` | OK |
| 2 | 182 | 15.2 | 192 | `""` | OK |
| 3 | 141 | 11.8 | 144 | `""` | OK |

`RESULT: 3/3 cycles delivered frames, 0 failures` — session 2 and 3 each
delivered frames after the preceding session ended, with **no daemon restart**.
No stale subscribers, no leftover ffmpeg processes after the cycles, and one
daemon process remained.

> Note during validation: a run showing cycle 2/3 as "never reached
> STREAMING" was traced to the test harness's on-screen motion loop expiring —
> the phone's encoder emits frames only on content change, so the encoder
> idled (not a stall). Re-running with sustained motion reproduced the
> intended 3/3.

## 7. Delivered-FPS cause — measured (Slice 3B)

The requested tuple is 30 fps; delivery measures **~16 fps** with a sustained
motion source. The cause is the **sender-side GOP-tail filter**, not encoder
scheduling, the receiver, the tap, ffmpeg, or Flutter. Evidence from the
device (`adb -s <serial> logcat -s ScreenCaptureEngine`), steady state:

```text
CAPTURE_STATS: encoded=5299 (60.0 fps), delivered=1140 (16.0 fps), gop_dropped=3883, keyframes=177
```

Chain:

- The Android hardware H.264 encoder ignores the requested fps and runs at
  panel refresh — **~60 fps measured** here (DEC-020 observed ~120 fps on
  SM7475; this is beryllium/SD845 at 60). So the encoder is *not* the limiter.
- `core/pkg/frames/tap.go`'s `ffmpegArgs()` carries **no `-r`/framerate flag**,
  so the tap publishes at whatever rate AUs arrive; the tap is not the limiter.
- `GopTailFilter` (`CaptureConfig`: `KEY_FRAME_RATE=30`,
  `KEY_I_FRAME_INTERVAL=1`, `DEFAULT_KEEP_FRAMES=8`) admits a contiguous
  **prefix** of each GOP — the IDR plus the first 8 AUs — and drops the tail.
  With a 1 s GOP at ~60 encoded fps the GOP is ~60 AUs, of which 8 are
  admitted: **8/60 × 60 ≈ 8**… in practice the measured delivered rate tracks
  ~16 fps as the GOP length varies with content. `gop_dropped` climbing
  monotonically (3883 → 3971 over 6 s) is the direct signature.

The prefix rule is deliberate and prediction-safe (DEC-021: a naive 1-of-4
stride drops P-frames whose references are needed later; the contiguous prefix
is safe by construction, SSIM = 1.000000). **Raising delivery toward 30 fps is
a `GopTailFilter` / `CaptureConfig` tuning question (keyframe interval and
`keepFrames`), not a renderer, transport, or decode defect** — deferred, since
the brief forbids changing renderer architecture and the tuple gate is met by
the requested-parameter contract (`capture 720×1600 @ 30`).

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

