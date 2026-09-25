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
| 720×1600 @ 30 fps tuple | capture 720×1600 @ 30; actual 720×1600 @ 15 delivered (GOP-tail) |
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

## 5. Known open issue — intermittent cross-session frame stall

3/8 runs on one long-lived daemon: STREAMING with growing `access_units`, but
the tap's ffmpeg emits no JPEG (`painted=0–1`, `frames_reason` healthy).
Clean daemon restarts always pass. Root cause NOT isolated; suspect
cross-session lifecycle around `frames.Hub.Begin/EndSession`. Repro: same
daemon, stop session N, start N+1, probe `StreamFrames` 10 s.

## 6. How to re-run

```bash
make -C core android-lib            # .so → jniLibs/
cd ui && flutter build apk --debug  # verify: strings … | grep psiHaveSPSPP
adb -s <serial> install -r ui/build/app/outputs/flutter-apk/app-debug.apk
make -C core daemon && ./core/build/phonebridge-daemon
# phone: consent with "Entire screen" (+ moving content for delivery)
PHONEBRIDGE_E2E=1 flutter test test/frames_e2e_test.dart
```

