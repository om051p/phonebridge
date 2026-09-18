# PhoneBridge Architecture

> Source of truth: [`MASTER_HANDOFF.md`](../MASTER_HANDOFF.md) · Status tags: `CONFIRMED` `VALIDATED` `PLANNED` `EXPERIMENTAL`

## Product principle

PhoneBridge is an independent product — own source, UI, protocol, identity model, branding, architecture. KDE Connect / scrcpy are reference-only.

## Target

Android ↔ Linux (Wayland, especially COSMIC). Primary: same-LAN direct/P2P, with WAN via P2P/TURN fallback.

## Layer map

```
                         PHONEBRIDGE
                              │
              ┌───────────────┴────────────────┐
              │                                │
           ANDROID                           LINUX
              │                                │
      ┌───────▼────────┐              ┌────────▼───────┐
      │ Flutter UI     │              │ Flutter UI     │
      └───────┬────────┘              └────────┬───────┘
              │                                │
      ┌───────▼────────┐              ┌────────▼───────┐
      │ Kotlin Host    │              │ Go Daemon      │
      │ Android APIs   │              │ Linux APIs     │
      └───────┬────────┘              └────────┬───────┘
              │                                │
              └────────────┬───────────────────┘
                           │
                       Go Core
                           │
                    PhoneBridge Protocol (proto/phonebridge/v1)
                           │
                      WebRTC / P2P (Pion)
                           │
             ┌─────────────┴─────────────┐
             │                           │
           LAN                         WAN
        direct/P2P                  P2P / TURN
```

## Intended boundaries

```
Flutter (ui/)  →  platform/local interface (MethodChannel / gRPC / UDS)
               →  Go core (core/pkg/*)
               →  PhoneBridge protocol (proto/phonebridge/v1)
               →  transport (WebRTC/ICE)

Android: Flutter → Kotlin (android/) → Go core (gomobile / c-shared)
Linux:   Flutter → Linux integration (linux/) → Go core (core/cmd/daemon)
```

### Rules

- Go core independently testable (`go test ./...` with no Flutter/Kotlin).
- No business logic in Flutter widgets.
- No duplicated Go logic in Kotlin.
- No Linux-specific assumptions in the protocol.
- Cross-layer changes are protocol-first.

## Responsibilities

| Layer | Owns | Status |
|-------|------|--------|
| Flutter (`ui/`) | pairing/device/file/notification/screen UI, state | `PLANNED` |
| Go core (`core/`) | protocol engine, crypto, identity, discovery, WebRTC, DataChannels, file/clipboard transport, engine | `PLANNED` — Phase 0 scaffold only |
| Kotlin (`android/`) | ForegroundService, CDM, NLS, AccessibilityService, MediaProjection/MediaCodec, SAF, IME | `PLANNED` |
| Linux (`linux/`) | D-Bus, Notifications, Wayland clipboard, XDG portals, PipeWire, uinput/EIS, systemd | `PLANNED` |
| Protocol (`proto/`) | Protobuf v3, package `phonebridge.v1`, codegen Go+Dart | `PLANNED` — envelope in Phase 0 |
| Server (`server/`) | signaling rendezvous, SDP/ICE exchange | `PLANNED` (Phase 4) |

## Local IPC & Platform Boundaries

Ratified transport boundaries:

- **Linux (DEC-018):** Flutter ── gRPC/Protobuf ── UDS (`$XDG_RUNTIME_DIR/phonebridge/engine.sock`) ── `phonebridge-daemon`. Authenticated via kernel `SO_PEERCRED` UID check + bearer token.
- **Android (DEC-019):** Flutter ── Platform Channels (`MethodChannel`/`EventChannel`) ── Kotlin `PhoneBridgeForegroundService` ── JNI (binary Protobuf `byte[]`) ── Go Core Engine (`libphonebridge_core.so`). The Kotlin Foreground Service owns the process lifecycle and Go runtime lifecycle, and forwards `onTrimMemory` to Go (`runtime.GC()` + `debug.FreeOSMemory()`). Panics are intercepted via `defer recover()` and safely mapped to `IllegalStateException`. Embedded in-process for sub-microsecond latency (< 1.0 µs p50) and robust background execution under Android LMK and Doze. (`gomobile`, out-of-process UDS/gRPC daemon, and Flutter direct FFI rejected).

## Android capture pipeline

Ratified pipeline (DEC-020, spike 03): MediaProjection → VirtualDisplay → codec input `Surface` →
hardware H.264 `MediaCodec` (byte-buffer output) → Kotlin → JNI `byte[]` → Go core. The constraints
that shape the product:

- **Consent first.** Activity `createScreenCaptureIntent()` → foreground service with `type=mediaProjection` → `getMediaProjection`/`registerCallback` → `createVirtualDisplay`. Starting the FGS *before* consent throws `SecurityException` on Android 15 at both targetSdk 34 and 35 (the gate is the `project_media` appop that only consent grants).
- **One projection per consent.** Resolution changes end the session and need a new consent; in-session adaptation is limited to bitrate and sync-frame requests.
- **Capture is damage-driven at panel refresh.** Neither `KEY_FRAME_RATE` nor `Surface.setFrameRate()` throttles composition, so Kotlin throttles to the negotiated stream rate and Go paces a bursty source (0-fps periods on static content or screen-off are normal, not a stall).
- **Codec chosen by capability.** Hardware H.264 primary, HEVC optional (present but unmeasured), AV1 excluded (software-only ≤ 1920 px on target devices).

Evidence: [spike 03 results](spikes/03-android-mediaprojection-encoder-results.md) and [spike 04 evidence](../spikes/04-pion-webrtc-android-linux/) (VALIDATION.md, GOP-RETUNE.md, BURST-AND-PSI.md). Spike 04 measured what spike 03 left open: `KEY_MAX_FPS_TO_ENCODER` is accepted but ignored (~119.7 fps actual), framing is Annex-B, parameter sets appear once per stream (1 of 57,464 AUs), and the safe throttle is GOP-tail dropping (SSIM 1.0 vs 0.9504 for naive striding).

## Android transport pipeline (DEC-021, spike 04)

Ratified transport: Kotlin AUs → bounded queue (key-protected drop policy) → single-writer send loop → RFC 6184 packetization (single NALU / FU-A ≤ 1200 B) → RTP (90 kHz timestamps = MediaCodec PTS, monotonic, no base reset) → Pion ICE/DTLS/SRTP. Obligations with measured provenance:

- **Burst shaping.** Token bucket at the RTP emission point: 4 Mbps ceiling, 3000 kbit bucket, 500 ms idle-credit cap. Wake bursts measured 3.0–6.0 Mbps unshaped (0–44 packets lost, decoder corruption observed); shaped runs lost nothing with p50/p90 delay unchanged and a bounded ≈0.9 s worst-case hold.
- **SPS/PPS re-injection.** Cache CSD and prepend to every forwarded IDR lacking them; parameter sets otherwise occur once per stream, so late join/reconnect/first-AU loss would be undecodable. ≈0.3% bandwidth overhead at 2.5 Mbps.
- **Throttling.** Encoder frame-rate control does not work on the tested SoC; Kotlin applies GOP-tail dropping (GOP 30 candidate: ~32 AU/s delivered, ~192 ms freeze per 250 ms GOP) and Go paces the send path.

Limitations: single device (SM7475/Android 15), absolute one-way latency unmeasured (RTP timestamps are sender-monotonic; no clock anchoring), signaling was spike-local HTTP.

## Repository layout

```
proto/phonebridge/v1/   protobuf, buf.yaml/buf.gen.yaml
core/pkg/*, core/cmd/*  Go core + daemon
android/                Kotlin + Gradle
linux/packaging/, dbus/ Linux integration
server/                 signaling (later)
ui/                     Flutter
docs/                   architecture, security, protocol, development, spikes
tests/                  cross-layer / network sim (later)
third-party/            dependency ledger
```

## Deferred / experimental

- Flutter ↔ Go on Android embedding — `EXPERIMENTAL`
- Wayland clipboard/input (COSMIC portal/EIS) — `EXPERIMENTAL`
- WebRTC media perf / codec choice — transport validated (DEC-021, Spike 04); HEVC path and cross-device behaviour — `EXPERIMENTAL`
- Kotlin capture throttling + Go-side pacing — measured & ratified (DEC-020 amendment + DEC-021, Spike 04)
- TURN infra/cost — `PLANNED`, no commitment in Phase 0 (remote NAT/TURN remains Spike 10)
