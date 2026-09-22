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
| Flutter (`ui/`) | pairing/device/file/notification/screen UI, state | `PLANNED` — mission control shipped; transfer UI in Phase 4 |
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

## LAN session contract (DEC-022, Phase 2)

The phone's LAN HTTP signaling endpoint is the first ratified production signaling
interface, and it carries parameter negotiation rather than only SDP:

```
Linux daemon ── DeviceHello (version + capabilities + MediaCapabilities) ──▶ phone
             ── POST /session/offer   { requested: MediaParams }        ──▶ phone
             ◀─ offer + { accepted, reject_reason, actual: MediaParams } ──
             ── POST /session/answer  { answer }                       ──▶ phone
             ◀────────────── RTP (DEC-021 transport obligations) ──────────
             ── POST /session/stop    { reason_code }                  ──▶ phone
```

- **Parameters are settled before the offer** because Android needs a
  MediaProjection consent before capture exists (DEC-020); the phone therefore
  stays the SDP offerer.
- **The capture device is authoritative** for the parameters it applies. A
  reply that differs from the request is a reported downgrade, never a silent
  substitution; an unsatisfiable request fails typed instead.
- **Reconnect is transport-only.** A dropped ICE/DTLS connection re-runs the
  same offer/answer exchange under the same session id while reusing the live
  MediaProjection/VirtualDisplay/encoder — DEC-020 records that re-binding a
  new encoder to a live VirtualDisplay does not resume delivery and that a
  resolution change needs a new consent, so capture is never restarted.
  SPS/PPS re-injection (DEC-021) makes the next IDR decodable immediately.
  Recovery is bounded (500 ms → 1 s → 2 s → 4 s, cap 5 s, 15 s window) and
  exhaustion is classified as `RECONNECT_TIMEOUT`, distinct from a transport
  failure. Consent withdrawal is reported as `CONSENT_REVOKED`, not a stall.
- **Scope:** LAN-only. Remote/NAT traversal (Spike 10) and server-based
  rendezvous (Phase 4) are out of scope; the `server/` component stays a stub.

## Clipboard synchronization architecture (DEC-023, Phase 3)

Ratified clipboard synchronization architecture across Linux (COSMIC/Wayland) and Android (API 35+):

```
+───────────────────────────────────────────────────────────────────────────────────+
|                                    PHONEBRIDGE                                    |
|                                                                                   |
|    LINUX HOST                                      ANDROID HOST                   |
|  ┌───────────────────────────────┐               ┌──────────────────────────────┐ |
|  | Wayland Compositor            |               | Foreground Window (App)      | |
|  | (cosmic-comp, Smithay)        |               | (mVisibleBound=true session) | |
|  └───────────────▲───────────────┘               └──────────────▲───────────────┘ |
|                  │ zwlr_data_control_v1                         │ IMMS binding    |
|  ┌───────────────▼───────────────┐               ┌──────────────▼───────────────┐ |
|  | phonebridge-wayland-helper    |               | PhoneBridge Companion IME    | |
|  | (isolated C binary, ~35KB)    |               | (InputMethodService)         | |
|  └───────────────▲───────────────┘               └──────────────▲───────────────┘ |
|                  │ stdio pipes (EVENT=/CMD=)                    │ JNI (DEC-019)   |
|  ┌───────────────▼───────────────┐               ┌──────────────▼───────────────┐ |
|  | Linux Daemon (core/cmd/daemon)|               | PhoneBridgeForegroundService | |
|  | ┌───────────────────────────┐ |               | ┌──────────────────────────┐ | |
|  | | Go Core Clipboard Engine  | |               | | Go Core Clipboard Engine | | |
|  | | - SHA-256 LRU Ring (32)   | |               | | - SHA-256 LRU Ring (32)  | | |
|  | | - Asymmetric 768 KiB cap  | |               | | - Asymmetric 768 KiB cap | | |
|  | | - In-memory resident cache| |               | └──────────────────────────┘ | |
|  | └─────────────▲─────────────┘ |               └──────────────▲───────────────┘ |
|  └───────────────┼───────────────┘                              │                 |
|                  │                                              │                 |
|                  └────────── WebRTC DataChannel ────────────────┘                 |
|                              (phonebridge.v1.ClipboardUpdate)                     |
+───────────────────────────────────────────────────────────────────────────────────+
```

- **Linux platform mechanism (Spike 06):** Wayland `zwlr_data_control_unstable_v1` v2 driven by an isolated C helper binary (`phonebridge-wayland-helper`). Provides 100% focus-independent read/write/observation (~215–333 µs) without seat focus, X11 fallback, or window mapping.
- **COSMIC configuration dependency:** `cosmic-comp` (Smithay) gates `zwlr_data_control_manager_v1` behind `COSMIC_DATA_CONTROL_ENABLED=1`. Daemon pre-flight verifies this flag and surfaces `CLIPBOARD_STATUS_COSMIC_FLAG_REQUIRED` with remediation instructions when unset.
- **Portal excluded:** Upstream `xdg-desktop-portal` and `xdg-desktop-portal-cosmic` provide NO clipboard D-Bus interface (`org.freedesktop.portal.Clipboard` absent). Protocol-level access is required.
- **Go integration boundary:** Pure Go core (`CGO_ENABLED=0`). Standalone helper binary (~35 KB, 2.4 MB RSS, 0.0% CPU, 304 µs spawn) driven over stdin/stdout pipes, isolating Go supervisor from compositor crashes with bounded backoff restarts (500 ms → 1 s → 2 s → 4 s, cap 5 s).
- **Android platform mechanism (Spike 05):** Two-tier hybrid architecture:
  1. *Tier 1 (Ambient Sync):* Default companion IME (`InputMethodService`) bypasses background read prohibitions (1.0–2.2 ms read, 5.4 ms mean notification latency) with hidden keyboard (`mInputShown=false`), **conditional on `mVisibleBound=true`** (active `InputConnection` in foreground window).
  2. *Tier 2 (Dormant Fallback):* When `mVisibleBound=false`, Android `ClipboardService` suppresses background notifications. Fallback via Quick Settings Tile, Notification Action, or In-App pull on resume. Background writes are unconditional (0.7–4.6 ms).
- **Loop/echo suppression:** Neither Wayland nor Android protocol exposes writer identity. Go core (`core/pkg/clipboard`) maintains a thread-safe LRU hash ring buffer (capacity 32, TTL 5,000 ms) of recent outbound SHA-256 digests; incoming events matching cached digests within TTL are dropped at the boundary (0 echo cycles proven).
- **Payload & MIME policy:** PhoneBridge application safety limit: strictly enforces **768 KiB ceiling (786,432 bytes)** on `ClipboardUpdate.payload` bytes, an application-level safety limit informed by Android Binder transaction buffer measurements (where payloads exceeding ~800 KiB risk fatal `TransactionTooLargeException` process crashes). If payload > 786,432 bytes, `ClipboardUpdate` MUST NOT be transmitted; the Go clipboard engine rejects the oversized payload and informs the local UI that the clipboard exceeds the sync limit. Large content may be transferred via DEC-012 File Transfer, but this requires explicit user action—clipboard sync NEVER silently converts into a file transfer. V1 MIME scope: `text/plain;charset=utf-8` and `text/plain`; V1.1: `text/html` and `text/uri-list`.
- **Lifecycle, transience & reconnect synchronization:** Because Wayland selections are transient and clear immediately on writer process exit, the Linux Go daemon maintains an in-memory resident clipboard cache to serve desktop paste requests. Reconnect resumes on the same session ID (DEC-022) over a dedicated, reliable, ordered WebRTC DataChannel (`"clipboard"`, gated by `CAPABILITY_CLIPBOARD`). When the DataChannel opens, both peers exchange their active clip via `ClipboardUpdate`. If the 32-byte `sha256_digest` matches, no write occurs and sync is complete. If digests differ, peers compare `copied_at_ms`: if one timestamp is > 1000 ms newer, the newer clipboard wins; if timestamps differ by <= 1000 ms (or are equal), Linux/Desktop wins as a deterministic tie-breaker. This application-level arbitration rule resolves divergent offline edits without assuming synchronized clocks and requires no separate sync message.
- **Flatpak constraint:** Sandboxed Flatpak access succeeds under default Pop!_OS 24.04 but is blocked if `COSMIC_ENABLE_WAYLAND_SECURITY=1` is enforced; native packaging (`.deb` / systemd user service) is primary distribution target.

## File transfer architecture (DEC-024, Phase 4)

Bidirectional file transfer rides the **existing** authenticated session on one
dedicated reliable/ordered DataChannel — no second transport:

```
  sender Go core (core/pkg/transfer)                        receiver Go core
  ┌───────────────────────────────┐                     ┌────────────────────────────┐
  │ framing / limits / state      │   "transfer" DC     │ validation / hashing       │
  │ one 64 KiB read buffer        │  (one TransferFrame │ incremental temp writes    │
  │ backpressure: 1 MiB HWM /     │   per DC message)   │ verify size + SHA-256      │
  │ 256 KiB LWM via BufferedAmount│ ──────────────────▶ │ promote (Linux: rename;    │
  │ SHA-256 in the same pass      │ ◀────────────────── │  Android: MediaStore       │
  │ local events → IPC / JNI      │  accept/result/…    │  IS_PENDING=1 → clear)     │
  └───────────────────────────────┘                     └────────────────────────────┘
```

- **Channel:** `transfer` (reliable, ordered) created by the same offerer that
  creates `control`/`clipboard`; the clipboard channel is untouched. Presence of
  the channel is the capability; a session is still required (piggyback, like
  clipboard) — capture-free/data-only sessions remain a follow-up.
- **Protocol:** `TransferFrame` oneof (`FileOffer/FileAccept/FileChunk/
  FileComplete/FileResult/FileCancel`) sent bare on the DC, documented in
  `docs/protocol.md`; `Envelope.file_*` branches stay for wire compatibility.
- **Limits:** 64 KiB chunks (128 KiB received-frame cap), 16 GiB max file,
  free-space preflight, one outbound + one inbound transfer per session,
  bounded buffers only (no whole-file buffering), stall/complete timeouts.
- **No partial exposure:** Linux writes a hidden temp inside the destination
  filesystem and renames after verification; Android writes through a MediaStore
  pending item that becomes visible only on commit.
- **Reconnect:** no resume in Phase 4 — interrupted transfers fail typed
  (`CODE_TRANSFER_INTERRUPTED`), partials are deleted, the user retries from zero.

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
- Wayland clipboard — architecture ratified (DEC-023, Phase 3); remote input (COSMIC EIS / uinput) remains `EXPERIMENTAL` (Spike 07)
- WebRTC media perf / codec choice — transport validated (DEC-021, Spike 04); HEVC path and cross-device behaviour — `EXPERIMENTAL`
- LAN session negotiation + bounded reconnect — contract ratified (DEC-022, Phase 2); remote/TURN and server-based rendezvous — `PLANNED`
- File transfer transport/protocol/storage — ratified (DEC-024, Phase 4); transfer **resume**, capture-free data-only sessions, SAF "Save as…" and Linux drag-and-drop — `PLANNED` follow-ups
- Kotlin capture throttling + Go-side pacing — measured & ratified (DEC-020 amendment + DEC-021, Spike 04)
- TURN infra/cost — `PLANNED`, no commitment in Phase 0 (remote NAT/TURN remains Spike 10)
