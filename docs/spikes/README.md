# Technical Spikes — Phase 0 Framework

Per `MASTER_HANDOFF.md` §Immediate Next Action, ten validation spikes block freezing production architecture. Each doc below defines the framework; implementation is a small prototype, not a full feature.

> Status: `EXPERIMENTAL` — docs only in Phase 0. Results will update `docs/decisions.md`.

| # | Spike | File |
|---|-------|------|
| 01 | Flutter ↔ Go local IPC on Linux | [01-flutter-go-ipc-linux.md](01-flutter-go-ipc-linux.md) — **results: [01-flutter-go-ipc-linux-results.md](01-flutter-go-ipc-linux-results.md)** (`EXPERIMENTAL` — decision ratified as DEC-018: UDS+gRPC) |
| 02 | Flutter/Kotlin ↔ Go on Android | [02-flutter-kotlin-go-android.md](02-flutter-kotlin-go-android.md) |
| 03 | Android MediaProjection → encoder | [03-android-mediaprojection-encoder.md](03-android-mediaprojection-encoder.md) — **results: [03-android-mediaprojection-encoder-results.md](03-android-mediaprojection-encoder-results.md)** (`EXPERIMENTAL` — hardware H.264 validated; decision proposed, see §12) |
| 04 | Pion WebRTC Android ↔ Linux | [04-pion-webrtc-android-linux.md](04-pion-webrtc-android-linux.md) — **results: [../../spikes/04-pion-webrtc-android-linux/](../../spikes/04-pion-webrtc-android-linux/)** (VALIDATION.md · GOP-RETUNE.md · BURST-AND-PSI.md) (`EXPERIMENTAL` — transport ratified as DEC-021; DEC-006 confirmed) |
| 05 | Android background clipboard | [05-android-clipboard-background.md](05-android-clipboard-background.md) |
| 06 | COSMIC clipboard access | [06-cosmic-clipboard.md](06-cosmic-clipboard.md) |
| 07 | COSMIC remote-input strategy | [07-cosmic-input.md](07-cosmic-input.md) |
| 08 | Bidirectional large-file transfer | [08-bidirectional-large-file.md](08-bidirectional-large-file.md) |
| 09 | LAN discovery | [09-lan-discovery.md](09-lan-discovery.md) |
| 10 | Remote WebRTC P2P + TURN fallback | [10-remote-p2p-turn.md](10-remote-p2p-turn.md) |

Each spike doc follows: Hypothesis | Question | Target env | Minimal prototype | Success criteria | Failure criteria | Perf/security concerns | Decision to be made.
