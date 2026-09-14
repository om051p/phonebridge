# PhoneBridge --- Master Project Handoff

**Project:** PhoneBridge\
**Repository:** `phonebridge`\
**Status:** Pre-development research complete; implementation not
started\
**Document role:** Master project handoff / source-of-truth for
implementation agents\
**Primary target:** Android ↔ Linux (Wayland/COSMIC)\
**License target:** Apache License 2.0\
**Core stack:** Flutter + Go + Kotlin\
**Research baseline:**
`PhoneBridge: Pre-Development Technical Due Diligence & Architecture Blueprint`

------------------------------------------------------------------------

## 1. Project Mission

PhoneBridge is an open-source Android ↔ Linux device-integration
application inspired by the seamless experience of Microsoft Phone Link.

The goal is to provide a unified experience for:

-   device pairing
-   automatic local discovery
-   seamless clipboard synchronization
-   bidirectional file sharing
-   Android notification forwarding
-   notification actions/replies
-   device status
-   screen mirroring
-   remote touch/interaction where Android permits it
-   future SMS/call integration where technically and
    distribution-policy feasible
-   automatic local-network vs Internet connectivity

### Product principle

PhoneBridge is **not** a KDE Connect fork, scrcpy fork, or wrapper.

KDE Connect and scrcpy are prior art and may be studied for
architecture, protocol behavior, security considerations, and
implementation concepts. Their source code must not be copied into
PhoneBridge.

PhoneBridge must have:

-   its own source code
-   its own UI
-   its own protocol
-   its own device identity model
-   its own product branding
-   its own architecture

------------------------------------------------------------------------

# 2. Current Architectural Direction

The research baseline recommends:

``` text
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
                    PhoneBridge Protocol
                           │
                      WebRTC / P2P
                           │
             ┌─────────────┴─────────────┐
             │                           │
           LAN                         WAN
        direct/P2P                  P2P / TURN
```

### Important architectural rule

Do not assume every detail of this diagram is final.

The research identifies several areas that require prototype validation
before production architecture is frozen, especially:

-   Flutter ↔ embedded Go on Android
-   Android background clipboard strategy
-   COSMIC/Wayland clipboard access
-   COSMIC native input/portal/EIS behavior
-   screen capture/rendering pipeline
-   remote input limitations
-   long-running Android service behavior
-   WebRTC media performance across target hardware

------------------------------------------------------------------------

# 3. Technology Stack

## UI

### Flutter

Responsibilities:

-   shared UI
-   pairing UI
-   device dashboard
-   notifications UI
-   file-transfer UI
-   settings
-   screen-mirroring surface
-   connection status
-   transfer progress
-   error presentation

Flutter should remain primarily a presentation/state-management layer.

------------------------------------------------------------------------

## Core

### Go

Responsibilities:

-   protocol engine
-   cryptography
-   device identity
-   connection state machine
-   discovery
-   WebRTC
-   ICE/STUN/TURN integration
-   DataChannels
-   file transfer
-   clipboard transport
-   network state
-   local IPC
-   Linux daemon functionality

The Go core should be independently testable without Flutter.

------------------------------------------------------------------------

## Android platform layer

### Kotlin

Kotlin is responsible for Android-specific framework APIs.

Expected components include:

-   Foreground Service
-   CompanionDeviceManager integration
-   NotificationListenerService
-   AccessibilityService
-   MediaProjection
-   MediaCodec
-   Storage Access Framework
-   InputMethodService where validated/required
-   Android lifecycle and permission handling

Keep Kotlin thin. Do not duplicate business logic that belongs in Go.

------------------------------------------------------------------------

## Linux platform layer

Target modern Linux/Wayland, especially COSMIC.

Expected integration areas:

-   D-Bus
-   `org.freedesktop.Notifications`
-   Wayland clipboard protocols
-   XDG Desktop Portals
-   PipeWire
-   `/dev/uinput` or native portal/EIS where validated
-   systemd user services
-   filesystem APIs

Do not assume X11 behavior.

------------------------------------------------------------------------

# 4. Core Features

## MVP

### Pairing

-   QR-based pairing
-   device identity
-   mutual authentication
-   Short Authentication String verification
-   trusted-device storage
-   device revocation

### LAN

-   mDNS discovery
-   direct local connection
-   fallback discovery if required
-   automatic reconnect

### Clipboard

Bidirectional:

``` text
Android ⇄ Linux
```

Initial scope:

-   text
-   common image clipboard formats where practical

Android background clipboard behavior must be validated against current
Android versions before final implementation.

### File Sharing

Bidirectional:

``` text
Android → Linux
Linux → Android
```

Required UX:

-   single file
-   multiple files
-   drag and drop on Linux
-   Android Share Sheet integration
-   transfer progress
-   cancellation
-   integrity verification
-   configurable destination
-   resumable transfer as a target
-   large-file support

Research baseline proposes:

-   WebRTC DataChannel
-   reliable/ordered transfer
-   chunked payloads
-   SHA-256 integrity verification
-   Android Storage Access Framework

Do not hard-code performance claims until benchmarked.

### Notifications

Android → Linux:

-   title
-   body
-   application identity
-   icon where available
-   actions where available

Linux → Android:

-   dismiss
-   supported notification actions
-   inline reply where the original Android notification exposes
    RemoteInput

### Device Status

Potential initial information:

-   battery percentage
-   charging state
-   network state
-   Wi-Fi information where permitted

------------------------------------------------------------------------

# 5. V1 Features

## Screen Mirroring

Target architecture:

``` text
Android
  │
  ▼
MediaProjection
  │
  ▼
MediaCodec hardware encoder
  │
  ▼
WebRTC media track
  │
  ▼
Linux
  │
  ▼
hardware/software decoder
  │
  ▼
Flutter rendering surface
```

Primary codec candidate:

-   H.264

Secondary/experimental:

-   AV1 where hardware support is appropriate

Screen mirroring must support:

-   start/stop
-   resolution negotiation
-   frame-rate negotiation
-   adaptive bitrate
-   network adaptation
-   graceful failure for protected/`FLAG_SECURE` content

Do not claim universal 60 FPS until hardware benchmarks prove it.

------------------------------------------------------------------------

## Remote Interaction

Standard Android tier:

-   AccessibilityService for supported gestures
-   companion IME for text input

Enhanced optional tier:

-   Shizuku / Wireless ADB integration

Do not claim unrestricted Android input injection for a normal unrooted
application.

------------------------------------------------------------------------

# 6. Remote Connectivity

PhoneBridge should automatically select the best path.

``` text
                 Connection Manager
                        │
             ┌──────────┴──────────┐
             │                     │
          Same LAN             Different LAN
             │                     │
       Direct connection       WebRTC ICE
                                   │
                         ┌─────────┴─────────┐
                         │                   │
                       P2P                 TURN
                    preferred             fallback
```

The signaling server should primarily provide:

-   rendezvous
-   device/session coordination
-   SDP/ICE exchange
-   authentication metadata

It should not normally receive plaintext application payloads.

TURN is a transport relay and therefore introduces operational bandwidth
costs even though application data remains encrypted.

------------------------------------------------------------------------

# 7. Security Model

## Device Identity

Research baseline:

-   Ed25519 identity key
-   X25519 key exchange
-   secure platform storage where available

Storage targets:

### Android

Android Keystore / hardware-backed storage where available.

### Linux

Secret Service / desktop credential storage where available.

------------------------------------------------------------------------

## Pairing

Preferred flow:

``` text
Linux displays QR
       ↓
Android scans
       ↓
Exchange identity information
       ↓
Mutual authentication
       ↓
Display SAS
       ↓
User verifies
       ↓
Device becomes trusted
```

Never silently trust an unknown device.

------------------------------------------------------------------------

## Transport Security

WebRTC provides encrypted transport.

Application-level cryptography may be used for additional end-to-end
protection where justified by the final threat model.

Do not implement custom cryptography without a documented threat model
and review.

Use established cryptographic primitives and libraries.

------------------------------------------------------------------------

# 8. Protocol

The protocol must be PhoneBridge-owned.

Canonical schema location:

``` text
proto/phonebridge/v1/phonebridge.proto
```

Recommended protocol technology:

-   Protocol Buffers v3
-   generated Go code
-   generated Dart code
-   versioned package namespace

Initial message families:

``` text
Connection / Handshake
    DEVICE_HELLO
    PAIR_REQUEST
    PAIR_ACCEPT
    PAIR_REJECT
    DEVICE_STATUS
    PING
    PONG

Clipboard
    CLIPBOARD_UPDATE
    CLIPBOARD_ACK

Notifications
    NOTIFICATION_EVENT
    NOTIFICATION_ACTION
    NOTIFICATION_DISMISSED

Files
    FILE_OFFER
    FILE_ACCEPT
    FILE_CHUNK
    FILE_COMPLETE
    FILE_CANCEL

Screen / Input
    SCREEN_START
    SCREEN_STOP
    INPUT_KEYBOARD
    INPUT_MOUSE
    INPUT_TOUCH
```

The protocol must support:

-   capability negotiation
-   version negotiation
-   sequence numbers
-   replay protection
-   backward compatibility
-   graceful unknown-message handling
-   transfer identifiers
-   resumable transfer metadata where implemented

Do not freeze the protocol based only on the research draft. Review it
before implementation.

------------------------------------------------------------------------

# 9. Local IPC

The research baseline proposes a schema-driven local service model.

Linux:

``` text
Flutter
   │
   ▼
gRPC / Protobuf
   │
Unix Domain Socket
   │
   ▼
phonebridge-daemon
```

Proposed socket:

``` text
$XDG_RUNTIME_DIR/phonebridge/engine.sock
```

Android:

The research proposes embedded Go compiled as a C-shared library and a
local gRPC/UDS boundary.

**This is a validation item, not a mandatory final design.**

Prototype alternatives before locking:

1.  embedded Go + JNI/UDS
2.  Go native library
3.  Kotlin host + Go core boundary
4.  another local IPC strategy if it is demonstrably more reliable

Choose based on actual Android lifecycle, packaging, performance, and
debugging behavior.

------------------------------------------------------------------------

# 10. Repository Structure

Initial proposal:

``` text
phonebridge/
├── .github/
│   └── workflows/
│
├── proto/
│   └── phonebridge/
│       └── v1/
│
├── core/
│   ├── go.mod
│   ├── pkg/
│   │   ├── crypto/
│   │   ├── discovery/
│   │   ├── webrtc/
│   │   ├── protocol/
│   │   ├── transfer/
│   │   ├── clipboard/
│   │   └── engine/
│   └── cmd/
│       ├── daemon/
│       └── android/
│
├── android/
│   └── app/
│       └── src/main/
│           ├── kotlin/
│           └── AndroidManifest.xml
│
├── linux/
│   ├── packaging/
│   │   ├── systemd/
│   │   ├── udev/
│   │   ├── deb/
│   │   └── flatpak/
│   └── dbus/
│
├── server/
│   └── ...
│
├── ui/
│   ├── lib/
│   │   ├── core/
│   │   ├── state/
│   │   ├── models/
│   │   ├── services/
│   │   └── views/
│   └── test/
│
├── docs/
│   ├── architecture.md
│   ├── security.md
│   ├── protocol.md
│   └── development.md
│
├── tests/
│
├── third-party/
│
├── LICENSE
├── NOTICE
├── SECURITY.md
├── CONTRIBUTING.md
└── README.md
```

The exact structure may change during Phase 0 if implementation
constraints justify it.

------------------------------------------------------------------------

# 11. Dependency and Licensing Rules

Target project license:

``` text
Apache License 2.0
```

All dependencies must be tracked.

Candidate permissive dependencies from the research include:

-   Pion WebRTC --- MIT
-   Go D-Bus --- BSD-2-Clause
-   Go uinput --- MIT
-   gRPC Go --- Apache-2.0
-   Protobuf tooling/libraries --- verify exact component licenses
-   Flutter SDK --- verify bundled attribution/license requirements

### KDE Connect

Treat as:

``` text
REFERENCE ONLY
```

Do not copy source code.

Do not import GPL code into PhoneBridge unless the project's licensing
strategy is explicitly reconsidered.

### scrcpy

Treat as:

``` text
REFERENCE / POSSIBLE EXTERNAL INTEGRATION ONLY
```

Do not copy its implementation.

Any future external integration must be reviewed for:

-   license compliance
-   distribution model
-   process boundary
-   binary redistribution
-   attribution/NOTICE requirements

Every dependency must be checked against its actual upstream LICENSE
file before release.

------------------------------------------------------------------------

# 12. AI Coding Rules

This repository is intended for heavy AI-assisted development.

## No-Cross Rule

Agents working in one layer must not casually modify another layer.

``` text
Go agent       → core/
Kotlin agent   → android/
Flutter agent  → ui/
Linux agent    → linux/
Protocol agent → proto/
Server agent   → server/
```

If a cross-layer change is required, update the canonical
protocol/interface first.

## Protocol-first rule

API/state changes should begin with:

``` text
proto/phonebridge/v1/phonebridge.proto
```

Then:

``` text
generate code
→ update implementation
→ update tests
```

## Test-first rule

Every subsystem must have deterministic tests.

Go:

``` bash
go test ./...
```

Flutter:

``` bash
flutter test
```

Android:

``` bash
./gradlew test
```

Protocol:

``` text
buf lint
buf breaking
buf generate
```

Do not mark a task complete based only on compilation.

------------------------------------------------------------------------

# 13. Development Phases

## Phase 0 --- Repository & Architecture Scaffolding

Objectives:

-   create repository
-   establish license
-   establish CI
-   establish protocol directory
-   create architecture documentation
-   create contribution rules
-   create security policy
-   establish code-generation workflow

No major product feature implementation yet.

Exit criteria:

-   repository builds
-   CI runs
-   protocol generation works
-   Go and Flutter test harnesses work

------------------------------------------------------------------------

## Phase 1 --- Identity & Pairing

Implement:

-   device identity
-   secure key storage
-   QR pairing
-   authentication
-   SAS verification
-   trusted device storage
-   device revocation

Exit criteria:

Two real devices can securely pair and recognize each other.

------------------------------------------------------------------------

## Phase 2 --- Local Connectivity

Implement:

-   mDNS discovery
-   connection manager
-   local transport
-   WebRTC baseline
-   reconnect
-   capability negotiation

Exit criteria:

Android and Linux automatically discover and connect on the same LAN.

------------------------------------------------------------------------

## Phase 3 --- Phone Link Core

Implement:

-   device status
-   clipboard
-   notifications
-   notification actions
-   file transfer

File transfer must work:

``` text
Android → Linux
Linux → Android
```

Exit criteria:

The application is useful even without screen mirroring.

------------------------------------------------------------------------

## Phase 4 --- Remote Internet Connectivity

Implement:

-   signaling server
-   ICE
-   STUN
-   TURN fallback
-   remote authentication
-   connection mode selection
-   reconnect

Exit criteria:

Devices can communicate across separate networks without manual port
forwarding where NAT traversal permits.

------------------------------------------------------------------------

## Phase 5 --- Unified Flutter UX

Implement:

-   device dashboard
-   connection status
-   clipboard status
-   file transfer UI
-   notification center
-   settings
-   pairing flow
-   error handling

Exit criteria:

The product feels like one coherent application rather than separate
technical components.

------------------------------------------------------------------------

## Phase 6 --- Screen Mirroring

Implement and benchmark:

-   MediaProjection
-   MediaCodec
-   H.264
-   WebRTC media track
-   Linux decoding
-   Flutter rendering
-   bitrate adaptation
-   frame-loss recovery

Exit criteria:

Stable screen streaming on representative Android/Linux hardware.

------------------------------------------------------------------------

## Phase 7 --- Remote Interaction

Implement:

-   AccessibilityService gestures
-   companion IME text input
-   Linux input capture
-   Android input routing

Optional:

-   Shizuku
-   Wireless ADB

Exit criteria:

Clearly documented standard vs enhanced capabilities.

------------------------------------------------------------------------

## Phase 8 --- Packaging & Release

Linux:

-   `.deb`
-   Flatpak
-   systemd user service
-   udev rules only where actually required

Android:

-   signed APK
-   Play Store feasibility review
-   F-Droid feasibility review

Release requirements:

-   LICENSE
-   NOTICE
-   SECURITY.md
-   privacy policy
-   documentation
-   reproducible CI builds where practical

------------------------------------------------------------------------

# 14. Testing Strategy

Testing must occur at four levels.

## Unit

Examples:

-   crypto
-   protocol
-   transfer state
-   capability negotiation
-   connection state machine

## Integration

Examples:

-   Android ↔ Go
-   Flutter ↔ local engine
-   Go ↔ D-Bus
-   Go ↔ WebRTC
-   Android services ↔ Go core

## Network simulation

Test:

-   packet loss
-   latency
-   jitter
-   NAT
-   reconnection
-   bandwidth changes
-   TURN fallback

## Real-device testing

Required before release.

Test at minimum:

-   modern Android device
-   Ubuntu/COSMIC Wayland
-   another mainstream Wayland compositor
-   same-LAN scenario
-   separate-network scenario
-   mobile data ↔ home network
-   large file transfer
-   screen mirroring
-   clipboard
-   notifications

------------------------------------------------------------------------

# 15. Major Risks

## Android background execution

Risk:

Android lifecycle and OEM behavior can interrupt long-running
connectivity.

Mitigation:

-   CompanionDeviceManager
-   correct foreground-service types
-   OEM-specific guidance where necessary
-   real-device validation

------------------------------------------------------------------------

## Clipboard

Risk:

Android and Wayland restrict background clipboard access.

Mitigation:

-   validate the companion IME approach
-   validate COSMIC data-control/portal behavior
-   provide graceful fallback
-   never claim universal background clipboard synchronization without
    testing

------------------------------------------------------------------------

## Wayland input

Risk:

Wayland deliberately restricts arbitrary input injection.

Mitigation:

-   investigate native portal/EIS/libei
-   evaluate `/dev/uinput`
-   test COSMIC specifically
-   isolate privileged/input functionality

------------------------------------------------------------------------

## Screen mirroring

Risk:

-   protected content may appear black
-   encoder support differs by device
-   decoder support differs by Linux GPU
-   Flutter texture integration may require native platform code

Mitigation:

-   H.264 first
-   capability negotiation
-   hardware detection
-   fallback decoding
-   prototype before committing to performance targets

------------------------------------------------------------------------

## TURN cost

Relayed screen/file traffic can become expensive.

Mitigation:

-   P2P first
-   TURN only when required
-   bandwidth policies
-   self-hosted TURN option
-   documented relay economics

------------------------------------------------------------------------

# 16. Product Boundaries

PhoneBridge should NOT attempt to implement everything immediately.

Do not begin with:

-   SMS
-   calling
-   audio forwarding
-   unrestricted remote input
-   advanced ADB control
-   AI features
-   cloud storage
-   account systems

First prove the core:

``` text
PAIR
  ↓
CONNECT
  ↓
CLIPBOARD
  ↓
FILES
  ↓
NOTIFICATIONS
  ↓
REMOTE
  ↓
SCREEN
  ↓
INPUT
```

This sequence gives the project usable milestones and prevents
screen-mirroring complexity from blocking the basic Phone Link
experience.

------------------------------------------------------------------------

# 17. Definition of Done

A feature is not complete when code compiles.

A feature is complete only when:

1.  Architecture is documented.
2.  Interface/protocol is defined.
3.  Unit tests exist.
4.  Integration tests exist where appropriate.
5.  Error paths are handled.
6.  Security implications are reviewed.
7.  Android/Linux permissions are documented.
8.  CI passes.
9.  Real-device testing is performed when platform behavior is involved.
10. The implementation does not violate the project's
    clean-room/dependency rules.

------------------------------------------------------------------------

# 18. Current Decision Log

### DEC-001 --- Product direction

**Decision:** Build an independent PhoneBridge product.

### DEC-002 --- UI

**Decision:** Flutter.

### DEC-003 --- Core

**Decision:** Go.

### DEC-004 --- Android platform integration

**Decision:** Kotlin.

### DEC-005 --- Protocol

**Direction:** Protobuf v3, subject to Phase 0 review.

### DEC-006 --- Network

**Direction:** WebRTC/Pion, subject to prototype validation.

### DEC-007 --- Local discovery

**Direction:** mDNS with fallback strategy.

### DEC-008 --- Remote networking

**Direction:** ICE/STUN/P2P with TURN fallback.

### DEC-009 --- License

**Target:** Apache-2.0.

### DEC-010 --- KDE Connect

**Decision:** Prior art/reference; no source-code copying.

### DEC-011 --- scrcpy

**Decision:** Prior art/reference; no source-code copying.

### DEC-012 --- File transfer

**Decision:** First-class MVP feature, bidirectional.

------------------------------------------------------------------------

# 19. Current Research Status

The available research concludes **GO** and estimates approximately 22
weeks for a production-oriented v1 under the proposed scope.

However, the estimate is not a commitment.

Before production implementation, validate the highest-risk assumptions
through small prototypes.

The research specifically identifies COSMIC Wayland remote-desktop/input
behavior and TURN infrastructure cost as areas requiring additional
validation.

------------------------------------------------------------------------

# 20. Immediate Next Action

Do **not** start implementing the full application yet.

Start with:

``` text
PHASE 0
    │
    ├── repository scaffolding
    ├── license/NOTICE
    ├── protocol draft
    ├── CI
    ├── architecture docs
    └── technical spike plan
```

Then build small proof-of-concept spikes for:

1.  Flutter ↔ Go local IPC on Linux
2.  Flutter/Kotlin ↔ Go integration on Android
3.  Android MediaProjection → encoder
4.  Pion WebRTC Android ↔ Linux
5.  Android clipboard strategy
6.  COSMIC clipboard access
7.  COSMIC input strategy
8.  bidirectional large-file transfer
9.  LAN discovery
10. remote WebRTC P2P + TURN fallback

Only after these are validated should the production architecture be
frozen.

------------------------------------------------------------------------

# 21. Agent Operating Instruction

Any AI coding agent receiving this repository must:

1.  Read this file first.
2.  Inspect the current repository before changing anything.
3.  Never assume the architecture is implemented merely because it is
    documented.
4.  Distinguish confirmed implementation from planned architecture.
5.  Never copy KDE Connect or scrcpy source code.
6.  Never introduce a dependency without checking its license.
7.  Never weaken security to make a feature "just work."
8.  Never bypass Android security restrictions through undocumented
    hacks for the standard product.
9.  Prefer small, testable changes.
10. Run relevant tests before reporting completion.
11. Update documentation when architectural behavior changes.
12. Ask for clarification before making a large cross-layer
    architectural change.
13. Preserve backward compatibility once the protocol reaches a released
    version.

------------------------------------------------------------------------

# 22. Master Principle

PhoneBridge should optimize for:

``` text
OPEN SOURCE
      +
SECURITY
      +
LOCAL-FIRST
      +
P2P-FIRST
      +
CROSS-PLATFORM
      +
AI-MAINTAINABLE ARCHITECTURE
      +
REAL-WORLD RELIABILITY
```

The product should feel simple to the user even though the underlying
system is distributed and technically complex.

**User experience goal:**

> Install PhoneBridge → pair phone once → devices recognize each other →
> clipboard, files and notifications simply work → screen mirroring and
> interaction are available when supported → LAN is preferred
> automatically → Internet connectivity works without manual networking
> configuration whenever NAT traversal permits.
