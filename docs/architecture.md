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

## Local IPC

Proposed (subject to spike validation):

```
Linux:  Flutter ── gRPC/Protobuf ── UDS ($XDG_RUNTIME_DIR/phonebridge/engine.sock) ── phonebridge-daemon
Android: Flutter/Kotlin ── (embedded Go c-shared or gomobile) ── Go core  — EXPERIMENTAL
```

Spike 01 and 02 validate the final choice.

## Android capture pipeline

Ratified pipeline (DEC-020, spike 03): MediaProjection → VirtualDisplay → codec input `Surface` →
hardware H.264 `MediaCodec` (byte-buffer output) → Kotlin → JNI `byte[]` → Go core. The constraints
that shape the product:

- **Consent first.** Activity `createScreenCaptureIntent()` → foreground service with `type=mediaProjection` → `getMediaProjection`/`registerCallback` → `createVirtualDisplay`. Starting the FGS *before* consent throws `SecurityException` on Android 15 at both targetSdk 34 and 35 (the gate is the `project_media` appop that only consent grants).
- **One projection per consent.** Resolution changes end the session and need a new consent; in-session adaptation is limited to bitrate and sync-frame requests.
- **Capture is damage-driven at panel refresh.** Neither `KEY_FRAME_RATE` nor `Surface.setFrameRate()` throttles composition, so Kotlin throttles to the negotiated stream rate and Go paces a bursty source (0-fps periods on static content or screen-off are normal, not a stall).
- **Codec chosen by capability.** Hardware H.264 primary, HEVC optional (present but unmeasured), AV1 excluded (software-only ≤ 1920 px on target devices).

Evidence: [spike 03 results](spikes/03-android-mediaprojection-encoder-results.md). Kotlin throttling,
Go-side pacing, and H.264 byte-level framing are open prerequisites for spike 04, not measured here.

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
- WebRTC media perf / codec choice — `EXPERIMENTAL`
- Kotlin capture throttling + Go-side pacing (required by DEC-020) — `PLANNED`, not yet implemented
- TURN infra/cost — `PLANNED`, no commitment in Phase 0
