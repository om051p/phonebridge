# Spike 03 — Android MediaProjection → Surface → MediaCodec

Isolated prototype validating Android screen capture into a hardware H.264 encoder,
per [`docs/spikes/03-android-mediaprojection-encoder.md`](../../docs/spikes/03-android-mediaprojection-encoder.md).

> Status: `EXPERIMENTAL` — evidence artifact, not production code.
> Results write-up: [03-android-mediaprojection-encoder-results.md](../../docs/spikes/03-android-mediaprojection-encoder-results.md)

## What this is

A standalone Android app (own Gradle project, **no production code references, no
third-party dependencies**) that runs one capture pipeline:

```
MediaProjection  →  VirtualDisplay  →  Surface  →  MediaCodec (hardware H.264)  →  byte buffers
     (consent)         (composer)      (input)        (c2.qti.avc.encoder)          (measured)
```

and measures it: frame delivery, achievable fps, bitrate fidelity, encode-pipeline
latency, keyframe behaviour, encoder lifecycle, stop/restart semantics, CPU/memory/
thermal cost, and the failure modes that matter for PhoneBridge (FLAG_SECURE, screen
off, static content, consent scopes, foreground-service ordering).

Nothing is implemented beyond the encoder: **no WebRTC, no remote control, no pairing,
no clipboard, no file transfer, no product UI.** Encoded frames stop at the harness.

## Requirements

- Android SDK (compileSdk 35), JDK 17
- A **physical device** (encoders differ wildly on emulators)
- `adb` with the device authorised; a MediaProjection consent dialog will be driven
  automatically (see "Consent automation")

## Quick start

```bash
make build install     # build + install the prototype APK
make device            # dump device / SoC / display / thermal context
make caps              # enumerate video encoders (no consent needed)
make baseline          # control run: content only, no capture
make session           # one capture session (10 s default)
make lifecycle         # capture + lifecycle/restart/FLAG_SECURE/static/bitrate probes
make matrix            # resolution + fps sweep (one consent per config)
make landscape W=1280 H=720
make frhint            # Surface.setFrameRate hint (seamless strategy)
make frhint-always     # Surface.setFrameRate hint (always strategy)
make adapt             # isolated dynamic-bitrate probe
make vbr               # same session with VBR instead of CBR
make screenoff         # capture across a screen-off/screen-on cycle
make fgsfirst          # NEGATIVE probe: mediaProjection FGS before consent
make build34           # rebuild the same prototype with targetSdk 34
make pull logs clean
```

Overrides: `W H FPS BITRATE BITRATE_MODE DURATION CYCLES LABEL SCOPE OUT MATRIX_CONFIGS`
(`SCOPE` selects the Android 15 capture scope, default `Entire screen`).

Everything app-side is driven over adb with intent extras, so runs are reproducible
without a UI: the activity only requests consent and draws deterministic high-motion
content; the foreground service owns the session.

## Layout

```
spikes/03-android-mediaprojection-encoder/
├── Makefile                     # thin wrapper over the host driver
├── tools/spike03.sh             # host driver: build, install, run, auto-consent, pull
├── tools/consent_tap.py         # uiautomator-XML → tap coordinates (dialog + scope spinner)
├── android/                     # self-contained Gradle project (dev.phonebridge.spike03)
│   └── app/src/main/kotlin/dev/phonebridge/spike03/
│       ├── MainActivity.kt      # consent gateway + animated content + caps/baseline modes
│       ├── CaptureService.kt    # foreground service (type mediaProjection) owning the session
│       ├── Session.kt           # projection → VirtualDisplay → codec, metrics, lifecycle probes
│       ├── Caps.kt              # encoder inventory + selection
│       └── Support.kt           # JSON writer, device/proc/thermal sampling, UI hooks
└── results/                     # per-run evidence (gitignored, regenerable)
    ├── device/results/*.json    # app-written JSON per run
    ├── host-*.txt               # dumpsys cpuinfo/battery/thermal/media_projection snapshots
    ├── logcat-*.txt             # per-run app log (SPIKE03_* markers)
    ├── ui-dump-*.xml            # consent-dialog hierarchies actually encountered
    └── aggregate.txt            # summariser output used for the results doc
```

## Evidence model

Every run writes JSON containing the config, the selected codec, per-step timings and
outcomes, per-second buckets (frames, fps, kbps, bytes/frame, app CPU, RSS), latency
percentiles, keyframe intervals, and probe results. The host driver additionally saves
`dumpsys` snapshots and the logcat for the run, so each claim in the results doc can be
traced to a file.

Markers the driver keys off: `SPIKE03_CONSENT_PROMPT`, `SPIKE03_CONSENT_RESULT`,
`SPIKE03_STEP`, `SPIKE03_PROBE`, `SPIKE03_RESULT_FILE`, `SPIKE03_DONE`.

## Consent automation

The MediaProjection dialog is SystemUI-owned and, on Android 15, offers a **capture
scope spinner that defaults to "A single app"**. `tools/spike03.sh` dumps the hierarchy,
picks the requested scope (`SCOPE`, default "Entire screen"), then presses Start; the
saved `ui-dump-*.xml` files are the record of what the dialog actually looked like.

The app pauses its animated content while the dialog is up: uiautomator only dumps an
idle hierarchy, and the content view is never idle while animating.

## Notes / caveats

- On Xiaomi HyperOS the first `adb install` after uninstalling may need the on-device
  confirmation ("Install via USB"); `adb install` then fails with
  `INSTALL_FAILED_USER_RESTRICTED`.
- Measurements include the harness's own synthetic 120 Hz animated content; `make baseline`
  exists to subtract it.
- Latency is an **encode-pipeline proxy** (composer frame timestamp → encoded buffer in
  Kotlin). Network, decode and display are out of scope for this spike.
