# Spike 03 Results — Android MediaProjection → Surface → Hardware H.264 Encoder

> Status: `EXPERIMENTAL` artifact — **measurable, device-backed answer set for the capture pipeline**
> Frame: [03-android-mediaprojection-encoder.md](03-android-mediaprojection-encoder.md) ·
> Prototype: [`spikes/03-android-mediaprojection-encoder/`](../../spikes/03-android-mediaprojection-encoder/) ·
> Raw data: `spikes/03-android-mediaprojection-encoder/results/` *(regenerable per-run evidence via
> `make -C spikes/03-android-mediaprojection-encoder …`; intentionally not committed — the distilled
> numbers below are the durable record)*

---

## 1. Executive Verdict & Core Recommendation

**MediaProjection → VirtualDisplay → Surface → MediaCodec hardware H.264 works, and it works well
enough to build screen mirroring on.** On a mid-range Snapdragon (SM7475) running Android 15 the
pipeline sustained the panel's full **119–120 fps at 1080×2400**, produced **no encoder errors in any
run** (including 20 back-to-back encoder restarts), survived `FLAG_SECURE` and screen-off cycles
without crashing, and added only **~11 % of one CPU core** in the codec process.

The spike also overturned four assumptions that would have cost real time later:

1. **Capture does not honour a requested frame rate.** Frames arrive at the panel refresh rate
   (119–120 fps here) whenever content changes; `MediaFormat.KEY_FRAME_RATE` and even
   `Surface.setFrameRate()` do not throttle it. Throttling must happen in our own code.
2. **Frame delivery is damage-driven, not clock-driven.** Static content produced **0 frames/s**;
   screen-off produced **0 frames/s for 6 consecutive seconds**; waking produced a **39.6 Mbps
   one-second burst**. The stream is bursty-by-nature and must be treated as such.
3. **`KEY_I_FRAME_INTERVAL` is in units of configured-fps frames, not seconds.** With
   `KEY_FRAME_RATE=30` and `KEY_I_FRAME_INTERVAL=1` at a 119 fps input, keyframes arrived every
   **250 ms** (4/s) instead of every second.
4. **On Android 15 the foreground service must start *after* consent.** Starting an FGS with
   `type=mediaProjection` before the consent dialog has granted the projection appop throws
   `SecurityException` and kills the process — on this ROM at **both targetSdk 34 and 35**.

### Summary of decisions this spike supports

1. **H.264-first with a hardware encoder, selected by capability query, not by codec name.**
   `c2.qti.avc.encoder` (hardware, CBR+VBR, 128–4096 px, ≤480 fps, 16 instances) is the primary
   choice; HEVC hardware is available as a secondary. **AV1 is not viable**: the only AV1 encoder on
   this device class is `c2.android.av1.encoder` (software-only, ≤1920 px).
2. **Byte-buffer (`dequeueOutputBuffer`) output, not an output Surface.** The bytes are what the Go
   core needs; measured frame sizes (1.5–9.2 KB) make the JNI handoff cost negligible (§10).
3. **Fixed capture geometry per session; adaptive bitrate/framerate in-session; resolution changes via
   projection restart.** `vd.resize()` and `setSurface()` are permitted, but re-binding a *new*
   encoder to a live VirtualDisplay did not resume delivery in practice (§8.4).
4. **One consent per session is a hard platform rule.** Stopped projections cannot be reused
   (`SecurityException`), and a second VirtualDisplay on the same projection is refused. The product
   UX must therefore present mirroring as an explicit, user-approved session.
5. **Explicitly drive the Android 15 capture scope.** The dialog defaults to **"A single app"**;
   full-screen mirroring requires selecting **"Entire screen"** (§9.2).

---

## 2. Test Environment

| Item | Value |
|---|---|
| Device | Xiaomi **23049PCD8I** (`POCO/marblein/marblein:15/AQ3A.250226.002/OS3.0.2.0.VMRINXM`, region `in`) |
| SoC | **SM7475** (`ro.board.platform=taro`), `ro.hardware=qcom`, arm64-v8a |
| OS | **Android 15 / API 35**, security patch 2026-05-01 (HyperOS 3.0.2.0) |
| Display | 1080×2400, 440 dpi, modes 60/90/**120 Hz** (active `renderFrameRate 120.00001`) |
| Encoder used | `c2.qti.avc.encoder` (hardware, vendor, CBR) |
| Codec process | `media.hwcodec` (where the vendor C2 service runs) |
| Prototype | `dev.phonebridge.spike03`, `compileSdk 35`, `minSdk 26`, `targetSdk 35` (and a 34 variant) |
| Evidence | 25 run files (23 in §3 + 2 pre-fix exploratory sessions excluded from §5) |
| Build | AGP 8.13.1, Kotlin 2.1.0, Gradle 8.14, JDK 17, no third-party dependencies |
| Harness | foreground service (`type=mediaProjection`) + adb-driven scenarios; no UI interaction except consent |

The prototype is fully isolated: its own Gradle project under
`spikes/03-android-mediaprojection-encoder/`, no references to `android/`, `core/`, or `proto/`, and
no dependency additions to production builds. **No production file was modified by this spike.**

### Method

Frame accounting happens in a dedicated thread doing only `dequeueOutputBuffer` →
`releaseOutputBuffer`, per second buckets. Reported metrics:

- **fps** = encoded output buffers per second (wall clock).
- **bitrate** = encoded bytes per second.
- **latency proxy** = `System.nanoTime()` at dequeue − buffer `presentationTimeUs`
  (`CLOCK_MONOTONIC`, the composer's frame timestamp). This is *capture→encoded-buffer-available*
  (composition + encoder queue + encode + buffer availability). It is **not** glass-to-glass: no
  network, decode, or display stage is included and no loopback decode was performed.
- **app CPU** = `Process.getElapsedCpuTime()` delta (one-core percentages of an 8-core SoC).
- A **baseline** run (`make baseline`) measures the same animated content with no projection and no
  encoder, so the capture pipeline's cost can be subtracted.

An early defect is worth recording because it shows the harness validated itself: the first latency
numbers were ~1.46 × 10⁹ ms, because the composer timestamps are `CLOCK_MONOTONIC` while the first
version sampled `elapsedRealtimeNanos()` (`CLOCK_BOOTTIME`). The ~6.2-day constant offset was the
device's accumulated suspend time. Comparing against `System.nanoTime()` fixed it; the corrected
numbers below agree with the frame-gap distribution.

---

## 3. What Was Tested

| # | Scenario | Command | Runs |
|---|---|---|---|
| 1 | Encoder inventory + size/rate probe matrix | `make caps` | 1 |
| 2 | Control run (content only, no capture) | `make baseline` | 1 |
| 3 | Resolution / fps matrix (8 configs from 540×1200 to 2400×1080, 30/60 fps, 1.5–12 Mbps) | `make matrix` | 8 |
| 4 | Full lifecycle + probe run | `make lifecycle` | 2 |
| 5 | Frame-rate throttle attempts (`ONLY_IF_SEAMLESS`, `ALWAYS`) | `make frhint`, `make frhint-always` | 3 |
| 6 | Isolated dynamic-bitrate probe | `make adapt` | 1 |
| 7 | VBR vs CBR at identical settings | `make vbr` | 1 |
| 8 | Landscape 1280×720 (rotated display) | `make landscape W=1280 H=720` | 1 |
| 9 | Screen-off → screen-on during capture | `make screenoff` | 1 |
| 10 | Negative probe: FGS before consent | `make fgsfirst` | 3 (1 exploratory + 2 fresh-install reproductions) |
| 11 | targetSdk 34 variant: FGS order + capture | `make build34` + runs | 2 |

23 runs in the table; the two remaining files are the earliest pre-instrumentation-fix sessions
(kept as evidence for the clock-timebase defect described below, excluded from the §5 numbers).

Probes inside a session: sync-frame request (~40 % of run), `FLAG_SECURE` on/off (3 s window),
content freeze (3 s window), dynamic bitrate change (70 % of run), `moveTaskToBack` (80 % of run),
encoder start/stop cycles, second encoder instance, second VirtualDisplay, `setSurface` (same and
different size), `vd.resize`, encoder restart + retry, `projection.stop()` → `onStop`, post-stop token
reuse.

---

## 4. Encoder / Device Capabilities

All video encoders reported by `MediaCodecList(REGULAR_CODECS)` on the test device:

| Codec | MIME | HW | SW | Size range | FPS range | Rate modes | Max inst. | Align |
|---|---|---|---|---|---|---|---|---|
| `c2.qti.avc.encoder` | video/avc | ✅ | – | 128–4096 | 1–480 | CBR, VBR | 16 | 2×2 |
| `OMX.qcom.video.encoder.avc` | video/avc | ✅ | – | 128–4096 | 1–480 | CBR, VBR | 16 | 2×2 |
| `c2.android.avc.encoder` | video/avc | – | ✅ | 16–2048 | 0–960 | CBR, VBR | 32 | 2×2 |
| `OMX.google.h264.encoder` | video/avc | – | ✅ | 16–2048 | 0–960 | CBR, VBR | 32 | 2×2 |
| `c2.qti.hevc.encoder` | video/hevc | ✅ | – | 128–4096 | 1–480 | CBR, VBR | 16 | 2×2 |
| `OMX.qcom.video.encoder.hevc` | video/hevc | ✅ | – | 128–4096 | 1–480 | CBR, VBR | 16 | 2×2 |
| `c2.qti.hevc.encoder.cq` | video/hevc | ✅ | – | 128–512 | 1–20 | CQ | 16 | 2×2 |
| `c2.android.hevc.encoder` | video/hevc | – | ✅ | 2–512 | 1–300 | CBR, VBR, CQ | 32 | 2×2 |
| `c2.android.av1.encoder` | video/av01 | – | ✅ | 2–1920 | 0–960 | CBR, VBR, CQ | 32 | 1×1 |
| `c2.android.vp8.encoder`, `OMX.google.vp8.encoder` | vp8 | – | ✅ | 2–2048 | 0–960 | CBR, VBR | 32 | 1×1 |
| `c2.android.vp9.encoder`, `OMX.google.vp9.encoder` | vp9 | – | ✅ | 2–2048 | 0–960 | CBR, VBR | 32 | 1×1 |

Notes:

- **Hardware H.264 and HEVC are both available**, each with two vendor variants (C2 and legacy OMX),
  16 concurrent instances, and advertised support to 4096 px / 480 fps.
- **AV1 has no hardware encoder on this device class** — only the software `c2.android.av1.encoder`
  capped at 1920 px. AV1 should not be part of the Android capture plan for these devices.
- Selection logic picks `c2.qti.avc.encoder` for every probe configuration (1080×2400, 2400×1080,
  1920×1080, 1280×720, 2560×1440, 540×1200, 3840×2160 at 30/60 fps) — i.e. `areSizeAndRateSupported`
  was satisfied everywhere, including 2160p.
- The encoder advertises **CBR and VBR**; the harness requested CBR by default (report §6 shows what
  "CBR" actually delivered).
- Format actually produced (from `INFO_OUTPUT_FORMAT_CHANGED`): `video/avc`, profile 8 (High),
  1080×2400, `bitrate-mode=2` (CBR), `frame-rate=30`, `bitrate=6000000`.

---

## 5. Resolution / FPS Results

10–12 s sessions, CBR, `KEY_I_FRAME_INTERVAL=1`. Latencies are the encode-pipeline proxy (§2).

| Requested | Measured fps | Requested kbps | Measured kbps | Bytes/frame | lat p50 | lat p99 | Keyframe interval | First frame | App CPU |
|---|---|---|---|---|---|---|---|---|---|
| 1080×2400@30 | **119.1** | 6000 | 5985 | 6283 | 24.7 ms | 42.1 ms | 249 ms | 139 ms | 68.7 % |
| 1080×2400@30 | **119.4** | 6000 | 6001 | 6284 | 25.7 ms | 39.6 ms | 250 ms | 108 ms | 83.6 % |
| 1080×2400@60 | **119.3** | 12000 | 8756 | 9177 | 25.2 ms | 38.5 ms | 501 ms | 111 ms | 80.9 % |
| 720×1600@30 | **119.3** | 3000 | 2928 | 3067 | 21.9 ms | 34.6 ms | 250 ms | 117 ms | 82.1 % |
| 540×1200@30 | **119.6** | 1500 | 1499 | 1567 | 17.8 ms | 58.6 ms | 250 ms | 146 ms | 86.2 % |
| 1280×720@30 (portrait source, letterboxed) | **119.3** | 4000 | 2208 | 2315 | 20.0 ms | 30.5 ms | 250 ms | 123 ms | 80.1 % |
| 1920×1080@30 | **119.2** | 8000 | 3644 | 3820 | 28.0 ms | 54.2 ms | 249 ms | 167 ms | 82.8 % |
| 1920×1080@60 | **119.3** | 12000 | 3422 | 3587 | 27.8 ms | 40.8 ms | 498 ms | 129 ms | 82.2 % |
| 2400×1080@30 | **119.4** | 8000 | 3952 | 4139 | 24.6 ms | 42.2 ms | 249 ms | 151 ms | 82.8 % |
| 1280×720@30 (**landscape display**) | **119.3** | 6000 | 3755 | 3933 | 19.2 ms | 29.8 ms | 252 ms | 144 ms | 81.6 % |
| 1080×2400@30, **VBR** | **119.3** | 6000 | 4122 | 4318 | 25.1 ms | 40.8 ms | 250 ms | 141 ms | 80.0 % |

Baseline (identical animated content, **no** capture): **69.1 % of one core**.

### Findings

1. **The requested frame rate is decorative.** Every configuration — 30 fps and 60 fps alike —
   delivered **119.1–119.6 fps**, matching the 120 Hz panel and the timestamp-implied input rate
   (119.9 fps measured from `presentationTimeUs` deltas). `KEY_FRAME_RATE` did not limit input; it
   only changed the keyframe cadence (§6/§8.1).
2. **The spike's 720p@30 criterion is comfortably met** — and so is 1080p (and 2400×1080) at the
   panel's full refresh rate, with no encoder errors, across every configuration tested.
3. **Latency scales with pixel rate, not with the requested fps**: p50 17.8 ms at 540×1200 →
   19.2 ms at 1280×720 landscape → 24.7 ms at 1080×2400 → 28.0 ms at 1920×1080. p99 stayed under
   ~55 ms in all runs (one 540×1200 run hit 58.6 ms).
4. **First encoded frame after VirtualDisplay creation: 106–249 ms** (median ≈ 135 ms); encoder
   configure+start is only 25–90 ms of that. This is the user-visible "mirror appeared" delay floor,
   before network and decode.
5. **App CPU during capture (80.1–86.2 % of one core) is only ~11–17 points above the baseline**; the
   rest is the harness's synthetic 120 Hz full-screen animation, which also drives SurfaceFlinger
   (~62 % of a core during 1080×2400 capture) and `media.hwcodec` (~11 %). Capsule: **the encoder is
   cheap; composing a 120 Hz 1080p virtual display is the expensive part.** Reducing capture fps or
   resolution is the effective lever for battery, not codec choice.

---

## 6. Bitrate Behaviour

| Requested | Mode | Measured | Fidelity |
|---|---|---|---|
| 6000 kbps | CBR | 5985–6001 kbps | **99.8–100 %** |
| 3000 kbps | CBR | 2928 kbps | 97.6 % |
| 1500 kbps | CBR | 1499 kbps | **100 %** |
| 4000 kbps | CBR | 2208 kbps | 55 % |
| 8000 kbps | CBR | 3644–3952 kbps | 46–49 % |
| 12000 kbps | CBR | 3422–8756 kbps | 29–73 % |
| 6000 kbps | VBR | 4122 kbps | 69 % |

- **"CBR" on this encoder behaves as a ceiling, not a guarantee.** It hit the target exactly when the
  content was complex enough (full-height 1080×2400, 540×1200) but undershot badly on easier content
  (letterboxed 1080p came in at 29–49 % of target). Any capacity planning must use the *measured*
  rate, not the configured one.
- **VBR delivered 69 % of the CBR target** at identical geometry with more per-second variance
  (3.2–4.7 Mbps), i.e. real content adaptation.
- **Dynamic bitrate reconfiguration works but is loose**: `setParameters(PARAMETER_KEY_VIDEO_BITRATE)`
  6000 → 2000 kbps returned success and the measured rate fell from ~6.2 Mbps to a **3.4–3.7 Mbps
  steady state within ~1–2 s** — still ~1.8× the requested value. Rate control converges slowly and
  imprecisely; the sender-side estimator must drive it iteratively and verify against actual output.
- Encoders also accept `PARAMETER_KEY_REQUEST_SYNC_FRAME`: a requested keyframe arrived **6–29 ms**
  later (p50 ≈ 22 ms over 13 retained samples) while frames were flowing (§8.1) — the correct tool for
  on-demand keyframes.

---

## 7. CPU, Memory, Thermal

| Metric | Baseline (no capture) | During capture (1080×2400, CBR 6 Mbps) |
|---|---|---|
| App CPU (in-app counter, reliable) | **69.1 %** of one core | **80.1–83.6 %** of one core |
| `media.hwcodec` CPU (host `dumpsys cpuinfo`) | ~0 % in idle windows | **7.6–18 %** of one core |
| `surfaceflinger` CPU (host `dumpsys cpuinfo`) | ~60 % (120 Hz animated content) | ~62 % |
| App RSS | 133.8→156.9 MB | +2.2 → +6.1 MB during a 10–16 s session (peak ≈ 168–172 MB) |
| App PSS | 39.2→93.1 MB (graphics grows with the animation) | +1.0 → +6.7 MB during capture; **~59 MB after shutdown** (graphics PSS 1.8 MB once the activity is gone) |
| Threads | 27 | 38 |
| CPU thermal zones | 55.5–60.6 °C (before) | **50.8–55.1 °C** (after ~16 s — the device was cooling from earlier load) |
| Battery temperature | — | 37.2–37.9 °C |

- **Encoder cost is small; composition cost dominates.** The vendor codec contributed 7.6–18 % of
  one core, and SurfaceFlinger was already at ~60 % *without* any capture (the harness's 120 Hz
  animated content) — i.e. the mirror's marginal composition cost was too small to isolate this way.
  The dominant cost is `SurfaceFlinger` + the app's content rendering, not `MediaCodec`.
- **Caveat on host numbers:** `dumpsys cpuinfo` reports over a long, overlapping window, so its
  per-process percentages can span adjacent runs (visible in `results/host-*.txt`, where consecutive
  snapshots repeat). The in-app CPU counters (baseline vs capture, measured over the run itself) are
  the authoritative figures; the `cpuinfo` numbers are corroborating and are reported as such.
- **No thermal throttling was observed** in sessions up to 26 s; a soak test is still required for
  production claims (§13).
- Memory growth during a session is codec/buffer related and modest (≤ 6.7 MB PSS); teardown returns
  most of it (`onTrimMemory` → `runtime.GC()`/`FreeMemory` routing from DEC-019 remains the right
  pattern).

---

## 8. Lifecycle Findings

### 8.1 Encoder lifecycle

| Probe | Result |
|---|---|
| configure → start → stop → release, **20 cycles** | **20/20 succeeded**, p50 **32.6 ms**, p99 **80.0 ms**, min 26.8 ms, max 101.1 ms |
| Encoder configure+start (in session) | 25–90 ms |
| VirtualDisplay creation | 40–82 ms |
| `projection.attach` (`getMediaProjection` + `registerCallback`) | 2–7 ms |
| Second concurrent encoder instance | **Allowed** (2 × `c2.qti.avc.encoder` simultaneously) |
| On-demand keyframe (`REQUEST_SYNC_FRAME`) with frames flowing | **6–29 ms** across 13 retained samples (p50 ≈ 22 ms) |
| On-demand keyframe while content is static | **1461 ms** — bounded by the next frame, not the encoder |
| `codec.stop()` + `release()` + `VirtualDisplay.release()` | 14 ms + 14 ms |
| `projection.stop()` → `MediaProjection.Callback.onStop()` | **1 ms** (fires reliably when the callback stays registered) |

### 8.2 Stop / restart

- **In-session encoder restart does not resume delivery.** After stopping/releasing the codec, creating
  a fresh encoder and calling `vd.setSurface(newInputSurface)`, the VirtualDisplay reported success
  but the drain produced **0 frames in 3 s**; a second `setSurface` + 3 s drain produced **0 frames**
  again. (The same `setSurface` call had worked earlier at equal size for an `ImageReader` consumer.)
  **Conclusion: re-binding a new encoder to a live projection is not a supported adaptation path on
  this platform.**
- **`vd.resize(1080×2400 → 540×1200)` succeeded**, as did `setSurface` with a differently-sized
  `ImageReader` surface — but with the encoder's input surface detached, capture did not resume
  (0 frames). Resizing without a working way to re-attach an encoder leaves the stream dead until the
  session is recreated.
- **A stopped projection cannot be revived**: `createVirtualDisplay` on it → `IllegalStateException`
  (`Cannot create VirtualDisplay with non-current MediaProjection`). Requesting a *new* projection from
  the same consent `Intent` returned an object, but creating a display on it → `SecurityException`
  (`Don't re-use the resultData to retrieve the same projection instance…`).
- **A second VirtualDisplay on the same projection is refused** with that same `SecurityException`
  (`… Don't take multiple captures by invoking MediaProjection#createVirtualDisplay multiple times on
  the same instance`). One projection = one capture.
- **Consequence for the product:** changing capture resolution requires a **new session with a new
  user consent**. Design mirroring as a session that starts/ends explicitly, and treat geometry as
  fixed for the session; if adaptive resolution is required, add an EGL/GL scale stage between the
  VirtualDisplay surface and the encoder rather than re-binding codecs.

### 8.3 Backgrounding

`moveTaskToBack` during capture: frames continued briefly then fell to ~2 fps — because the launcher
is static, not because capture stopped. The foreground service and the projection stayed alive.

### 8.4 Damage-driven frame delivery

Content freeze probe (3 s window, 1080×2400@CBR 6 Mbps):

| Window | fps | kbps | Bytes/frame |
|---|---|---|---|
| Before freeze | 110.2 | 5330 | 6045 |
| **Frozen (static screen)** | **7.7** (two buckets at **0 frames**) | 448 | 7312 |
| After resume | 118.6 | 1039 | 1094 |

**The mirror produces frames only when screen content changes.** A static screen yields ~0 fps — which
is excellent for bandwidth and catastrophic for naive assumptions (e.g. "we always have a fresh
frame", fixed-rate pacing, or keyframe timers driven by frame counts).

### 8.5 Screen off / on

26 s session with the screen switched off mid-run:

| Period | fps | kbps |
|---|---|---|
| Screen on (baseline) | 119–122 | 5.7–6.7 Mbps |
| Screen-off transition | 30.9 | 210 |
| **Screen off (6 consecutive seconds)** | **0** | **0** |
| Wake transition | 1–15 | 2–1646 |
| **First second after wake** | 92.4 | **39 563 kbps (39.6 Mbps)** |
| Settled again | 116–122 | 3.5–3.7 Mbps |

- Nothing crashes and nothing is logged as an error: with the screen off, the display simply stops
  composing, so the encoder receives no frames. Capture resumes automatically on wake.
- **The wake burst is the headline risk**: a single second carried ~39.6 Mbps (a keyframe plus
  accumulated re-encoding of changed content). A WebRTC sender without pacing will spike the LAN/WAN
  and can trip queue limits and bandwidth estimators. **Mitigation: send-side pacing/leaky-bucket and
  a bounded keyframe size for large-resolution captures.**

### 8.6 Encoder selection stability

Across every capture run the same hardware encoder was selected and configured without failure, and
no `capture.errors` entry was recorded in any run file. The **only** non-OK outcomes in the whole
series were: the intentional FGS-before-consent probes (process killed by `SecurityException`, 4×,
including 3 fresh-install reproductions — targetSdk 35 ×2 and targetSdk 34 ×1), the projection-restart probes above (expected platform
refusals), and 2 early runs whose consent interaction was rejected/aborted before the driver's scope
handling existed. **No encoder-related failure occurred in any run.**

---

## 9. Security & Privacy Considerations

### 9.1 Consent and foreground service ordering (Android 15)

Starting an FGS with `type=mediaProjection` **before** the consent dialog has granted the projection
appop fails hard, on a **fresh install**, at **both targetSdk 34 and 35** — each target level reproduced
separately on a clean install (`results/fgs-before-consent-crash.txt` → `targetSDK=35`;
`results/fgs-before-consent-crash-t34.txt` → `targetSDK=34`):

```
java.lang.SecurityException: Starting FGS with type mediaProjection
callerApp=ProcessRecord{… dev.phonebridge.spike03/…} targetSDK=35 requires permissions:
all of the permissions allOf=true [android.permission.FOREGROUND_SERVICE_MEDIA_PROJECTION]
any of the permissions allOf=false [android.permission.CAPTURE_VIDEO_OUTPUT, android:project_media]
```

`CAPTURE_VIDEO_OUTPUT` is signature|privileged and unavailable to normal apps, so the practical
requirement is the **`android:project_media` appop**, which is granted by the consent dialog. The
correct order on Android 15 (and the one the prototype ships with) is:

```
1. Activity: createScreenCaptureIntent() → user consent
2. Activity: start foreground service (type=mediaProjection)   ← only AFTER consent
3. Service:  getMediaProjection(resultCode, data) + registerCallback()
4. Service:  createVirtualDisplay(...)
```

On a device where the appop has already been granted, the legacy (FGS-first) order no longer throws —
which is exactly how this becomes a "works on my machine" bug. Evidence: the raw crash buffer is saved
in `results/fgs-before-consent-crash.txt`.

Note that this contradicts the Android 14 guidance ordering, so the capture bootstrap must be written
against the runtime API level, not a single global order.

### 9.2 Capture scope: the Android 15 dialog defaults to *partial* capture

The dialogs actually encountered (hierarchy dumps are saved per run) offer exactly two scopes:

```
Scope:  [ A single app  ▾ ]      ← default
        A single app
        Entire screen
```

- With **"A single app"** the user shares one app's window; with **"Entire screen"** the full display.
- The driver selects "Entire screen" before pressing Start; `dumpsys media_projection` then reports
  `(dev.phonebridge.spike03, uid=…) : TYPE_SCREEN_CAPTURE` for the session.
- **Product consequence:** mirroring must let the user choose the scope (or state clearly that it
  needs full-screen), must not assume full-screen, and must handle a single-app projection as a valid
  state rather than a bug.

### 9.3 `FLAG_SECURE` is handled gracefully

With our own window flag set (the only content on screen), during the 3 s secure window:

| Window | fps | kbps | Bytes/frame |
|---|---|---|---|
| Before | 119.7 | 6409 | 6695 |
| **FLAG_SECURE on** | **119.9** | **1076** | **1123** |
| After | 96.9 | 5844 | 7534 |

- The stream **keeps flowing at full frame rate** with no error, no exception and no stall; the encoded
  payload collapses to ~17 % of normal size (uniform black frames under a rate cap).
- Release of `FLAG_SECURE` restores content immediately.
- Honest limitation: this is an **indirect** check — the harness did not decode frames to sample
  pixels. The bytes/frame collapse is consistent with Android's documented behaviour of replacing
  protected windows with black in the mirror, but a decode-side pixel check belongs in Spike 04.

### 9.4 Privacy-relevant properties

- The projection runs in-process under the app's UID and SELinux domain; no additional privileges,
  hidden APIs, or root are used.
- Consent is per session and **cannot be reused** (§8.2), matching the platform's intent that mirroring
  is user-initiated and visible. The ongoing notification is mandatory and was present for every run.
- A capture session carried no credentials, no clipboard, and no user data; the harness encoded only
  its own synthetic content. Nothing was transmitted off-device — the prototype has no network code.
- Residual risk to design for: while mirroring, **the user's entire screen is captured by our process**
  (including notification content). This is the strongest privacy argument for (a) an unmissable
  ongoing notification, (b) a visible "mirroring" state in the UI, (c) never persisting frames, and
  (d) preferring the single-app scope when the user only needs one app.

---

## 10. Go / WebRTC Handoff Implications

Not implemented in this spike (no WebRTC, by scope). What the measurements imply:

1. **Output mode: byte buffers.** `dequeueOutputBuffer` gives Kotlin the encoded H.264 for each frame:
   mean 1567 B (540×1200), 2315–3933 B (720p/720p-landscape), 6283 B (1080×2400 @ 6 Mbps),
   9177 B (1080×2400 @ 12 Mbps); 119 fps for dynamic content.
2. **The JNI boundary is not a bottleneck.** Spike 02 measured `invoke(method, byte[])` copies at
   1.44 µs (1 KiB), 4.75 µs (16 KiB), 27.33 µs (64 KiB). At 6.3 KB × 119 fps = 750 KB/s, the copy is
   ≈ 3 µs per frame → **≈ 0.36 ms of CPU per second (~0.04 % of one core)**. Even a 12 Mbps stream is
   negligible. Zero-copy Surface/interop machinery is therefore *not* justified for bandwidth reasons.
3. **Bitstream handoff is only partially characterised — this is the main open item for Spike 04.**
   Proven: exactly **one output buffer per session** carried `BUFFER_FLAG_CODEC_CONFIG` (with
   `size > 0`), and `INFO_OUTPUT_FORMAT_CHANGED` exposed **`csd-0`/`csd-1`** keys — so SPS/PPS is
   available both in-band at start and via the output format. **Not proven: the byte-level framing**
   (Annex-B start codes vs AVCC length-prefixed). The harness never inspected payload bytes
   (`formatToMap` recorded the CSD *keys* only — the values serialise as `null` because the value
   extraction handled int/long/float/string and not `ByteArray`). A payload-capturing harness — or
   Spike 04 — must dump the SPS/PPS and first NALs and confirm framing before any RTP payloader is
   written. Timestamps are `CLOCK_MONOTONIC` µs and map directly to RTP's 90 kHz clock.
4. **An Android `Surface` cannot be handed to Go.** Go/Pion has no way to consume an Android
   Surface/EGL stream, so the coherent design is exactly this one: **Kotlin owns
   MediaProjection/MediaCodec, and pushes encoded byte buffers across the DEC-019 JNI boundary**; Go
   owns packetization, pacing, RTP/WebRTC, and the protocol. (The libwebrtc-style "encoder output
   surface + Java track" pattern is only available inside libwebrtc, which PhoneBridge is not using.)
5. **Throttling must be implemented in Kotlin** (target fps 30/60 with frame dropping), because the
   VirtualDisplay cannot be throttle-controlled (§11). The steady-state sender therefore sees a
   variable rate it controls, not the panel rate.
6. **Pacing is mandatory**, because the natural stream is bursty: 0 fps on static screens, and a
   measured **39.6 Mbps single-second burst** on wake (§8.5). The Go layer needs a leaky bucket /
   pacer and a keyframe-size cap, and must treat "no frames" as normal rather than as a stall.
7. **Bitrate control loop:** drive `PARAMETER_KEY_VIDEO_BITRATE` from the Go-side estimator, expect
   slow (~1 s) and loose (~1.8× overshoot in the 6→2 Mbps test) convergence, and verify against
   measured output rather than trusting the request. On-demand keyframes via `REQUEST_SYNC_FRAME`
   respond in **6–29 ms** when frames are flowing (and take as long as the next frame when they
   aren't).
8. **Latency budget contribution (phone side):** p50 **17.8–28.0 ms** capture→encoded-buffer, plus
   packetization/network/decode/display measured in Spike 04. First-frame after session start:
   ~110–250 ms.

---

## 11. Pass / Fail Against the Spike Criteria

| Criterion (from the spike frame) | Result | Evidence |
|---|---|---|
| MediaProjection permission flow | **PASS** | consent dialog captured, driven programmatically, scope selected; `TYPE_SCREEN_CAPTURE` confirmed by `dumpsys media_projection` |
| VirtualDisplay creation | **PASS** | 40–82 ms, displayIds assigned, `onResumed`/`onStopped` callbacks observed |
| Surface input (VD → codec input surface) | **PASS** | every session delivered frames; 0 negative-latency samples |
| Hardware encoder availability | **PASS** | 2 hardware H.264 encoders (+2 HEVC); software fallbacks present; selection verified at 9 configurations |
| H.264 encoding | **PASS** | High profile, 3985–8756 kbps measured, 119 fps sustained |
| Frame delivery | **PASS** | 1311 frames/16 s typical; per-frame timestamps monotonic; gap p50 6 ms |
| Resolution/FPS configuration | **PASS (with a caveat)** | 9 distinct geometry/fps combinations across 11 measurement rows (540×1200 → 2400×1080, 30/60 fps, 1.5–12 Mbps); **fps is not configurable via the encoder — only resolution and bitrate are** |
| Keyframe generation | **PASS (caveat)** | `KEY_I_FRAME_INTERVAL` is measured in *configured-fps frames*, not seconds (1 s → 250 ms at 120 fps input); on-demand sync frames work (6–29 ms) |
| Encoder lifecycle | **PASS** | 20/20 configure/start/stop/release cycles, 0 failures; second instance allowed; clean release in ~14 ms |
| Stop/restart behaviour | **PASS (as a finding)** | clean stop with 1 ms `onStop`; **restart requires a new consent**; in-session encoder re-bind does not resume frames |
| Basic CPU/memory usage | **PASS** | codec 11 % / SurfaceFlinger 62 % / app 80 % of one core; ≤ 6.7 MB PSS growth; recovered after shutdown |
| Achievable latency | **PASS** | encode-pipeline p50 17.8–28.0 ms, p99 ≤ 58.6 ms (glass-to-glass not measured) |
| Achievable frame rate | **PASS** | 119–120 fps at 1080×2400 on this device; bounded by content damage, not by the encoder |
| **Success: 720p@30 on a mid-range device, no persistent encoder errors, graceful FLAG_SECURE fallback** | **PASS** | mid-range SM7475; 720p and far beyond; zero encoder errors across 25 runs; FLAG_SECURE stream continues at full fps with ~17 % payload |
| Failure triggers (encoder unavailable / inconsistent) | **Not triggered** | 4 AVC candidates, deterministic selection |
| Failure trigger: "Flutter texture integration requires deep native work" | **Not applicable / avoided** | the byte-buffer path never needs Flutter texture integration on Android |
| AV1 (experimental in the frame) | **Not viable** | software-only `c2.android.av1.encoder`, ≤ 1920 px on this class |

---

## 12. Architecture Decision — DEC-020 (ratified)

> **DEC-020 — Android screen capture pipeline** (`CONFIRMED`, recorded in [`docs/decisions.md`](../decisions.md)).
> Android screen mirroring uses **MediaProjection → VirtualDisplay → codec input Surface →
> hardware H.264 MediaCodec in byte-buffer output mode**, with encoded frames handed to the Go core
> over the DEC-019 JNI boundary. Specifics:
> **(a) Session order (validated):** user consent → foreground service with `type=mediaProjection` →
> `getMediaProjection` + `registerCallback` → `createVirtualDisplay` (required on Android 15;
> the FGS-before-consent order throws `SecurityException` at targetSdk 34 and 35).
> **(b) Scope (validated):** request and surface the Android 15 capture scope explicitly; request
> full-screen only when the user asks for mirroring (the platform default is "A single app").
> **(c) Codec (validated, except HEVC):** select by capability (`isHardwareAccelerated` +
> `areSizeAndRateSupported`), H.264 High primary, CBR configured but **measured** rate treated as
> authoritative; **AV1 excluded** (no hardware encoder on target devices). HEVC is *permitted* as a
> secondary because hardware encoders are present, but it was **capability-probed only, never encoded
> with** — it stays unvalidated until measured.
> **(d) Frame rate (necessity validated, mechanism prescribed):** capture is damage-driven and runs at
> panel refresh; the Kotlin layer **throttles to the negotiated stream rate (30/60 fps)**, since
> neither `KEY_FRAME_RATE` nor `Surface.setFrameRate()` limits composition. This spike proves that
> throttling is required; **the throttling implementation itself was never written or measured, so it
> is a Spike 04 prerequisite, not a measured result.**
> **(e) Adaptation within a session (validated):** bitrate via `PARAMETER_KEY_VIDEO_BITRATE` (loose —
> a 6000 → 2000 kbps change settled at ~3.4–3.7 Mbps, ≈1.8× the request, within ~1–2 s) and keyframes
> via `REQUEST_SYNC_FRAME` (6–29 ms with frames flowing); **resolution changes require ending the
> session and re-requesting consent**
> (in-session encoder re-binding was measured as unable to resume delivery). If adaptive resolution is
> later required, introduce an EGL scale stage between the VirtualDisplay surface and the encoder.
> **(f) Keyframes (validated):** configure `KEY_FRAME_RATE` to the *actual* capture rate (or set
> `KEY_I_FRAME_INTERVAL` in frames) so the keyframe period is not silently 4× shorter than intended.
> **(g) Transport (prescribed):** the Go layer paces the bursty stream (pacing/leaky bucket + bounded
> keyframe size) and must tolerate 0-fps periods (static screen, screen off) as a normal state. Go-side
> pacing is not implemented here; the burst evidence (39.6 Mbps in one second) is why it is required.
> **(h) Bitstream handoff (partially validated):** SPS/PPS is available in-band (`CODEC_CONFIG`, one
> buffer per session) and via `csd-0`/`csd-1`; **byte-level framing must be confirmed in Spike 04**
> before writing the RTP payloader (§10).

---

## 13. Limitations & Follow-Up Work

Not covered by this spike:

- **True glass-to-glass latency** — no loopback decode, no Linux receiver. The reported latency is the
  phone-side encode pipeline only.
- **`FLAG_SECURE` pixel verification** — inferred from payload collapse, not decoded pixel inspection.
- **Bitstream framing / CSD payloads** — SPS/PPS presence is established (`CODEC_CONFIG` ×1 per session,
  `csd-0`/`csd-1` keys) but the bytes were never dumped, so Annex-B vs AVCC framing is unverified
  (§10.3). This is the first thing Spike 04 should capture.
- **Kotlin frame-rate throttling was never implemented** — only the need for it was measured. The
  30/60 fps operating points quoted in this document are therefore *targets*, not results.
- **One device / one SoC.** Device-variability claims ("consistent across devices") remain open; a
  second SoC (ideally a different vendor) should be added to the harness before freezing performance
  targets. The harness is device-agnostic and driven entirely over adb.
- **Long-duration soak / thermal throttling** — sessions were ≤ 26 s. Multi-minute runs are needed
  before making battery or sustained-quality claims.
- **4K and AV1 encoding** — capability was probed (4096 px, 30 fps advertised) but not exercised.
- **The app-CPU baseline includes the harness's own 120 Hz synthetic animation** (69 % of one core);
  a real screen's content cost will differ — the *delta* (~11–17 points) is the meaningful number.
- **Keyframe payload distribution** was not recorded separately (only intervals and mean frame size),
  so the burst composition (I-frame vs P-frames) is inferred.
- The MediaProjection **consent-scope defaults may differ on non-Android-15 ROMs**; the automation
  records the dialogs it meets, so this remains testable per device.

Follow-up candidates: Spike 04 (Pion WebRTC Android ↔ Linux) should consume these bytes and measure
decode/display latency plus paced delivery; a Kotlin throttle implementation; a soak test for
thermal/battery; a second device family.

---

## 14. Reproducing

```bash
cd spikes/03-android-mediaprojection-encoder
make build install device caps baseline
make matrix                       # resolution/fps sweep (one consent per config)
make lifecycle DURATION=16        # lifecycle + FLAG_SECURE + static + bitrate probes
make frhint-always; make adapt; make vbr; make landscape W=1280 H=720; make screenoff
make build34 install && make fgsfirst    # targetSdk 34 FGS-order check (negative probe)
```

Per-run evidence lands in `results/` (JSON + logcat + `dumpsys` snapshots + UI dumps) and is
deliberately gitignored; the numbers in this document were distilled from those files. Building at
targetSdk 34 is a single Gradle property (`-PtargetSdkOverride=34`), so the same prototype can be used
to re-check the FGS ordering constraint on other ROMs.
