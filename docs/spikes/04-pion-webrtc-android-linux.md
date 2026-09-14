# Spike 04-pion-webrtc-android-linux — Pion WebRTC Android ↔ Linux

> Status: `EXPERIMENTAL` · Phase: 0 (framework only)


## Hypothesis
Pion-based Go core can establish DataChannel + media between Android and Linux over LAN.

## Question
Does Pion interop cleanly with Android (Pion or platform WebRTC) for both DataChannel and media, with acceptable latency/bandwidth?

## Target environment
- Android device + Linux (COSMIC Wayland), same LAN, varied Wi-Fi

## Minimal prototype
- Pion Go ↔ Pion Go and Pion Go ↔ Android native WebRTC: DataChannel echo + video track loopback.
- Measure setup time, throughput, jitter.

## Success criteria
- DataChannel reliable/ordered works; media track renders on Linux/Flutter.
- NAT-free LAN connects without TURN.

## Failure criteria
- Interop failures, unacceptable latency, or missing Android Pion support requiring fork.

## Perf / security concerns
- DTLS/SRTP overhead; ANR if media on main thread.

## Decision to be made
Confirm Pion as the Go WebRTC stack and the Android peer strategy.

