# Spike 03-android-mediaprojection-encoder — Android MediaProjection → Hardware Encoder

> Status: `EXPERIMENTAL` · Phase: 0 (framework only)


## Hypothesis
Android MediaProjection can feed a hardware encoder (H.264) suitable for WebRTC media track.

## Question
Can we capture, encode (H.264 primary, AV1 experimental), and feed a WebRTC track reliably across devices, handling `FLAG_SECURE` gracefully?

## Target environment
- Physical Android devices (varied SoCs), Android 13/14+, WebRTC stack

## Minimal prototype
- MediaProjection → VirtualDisplay → MediaCodec (H.264) → WebRTC video track → loopback decode.
- Negotiate resolution/fps, handle `FLAG_SECURE` black-frame case.

## Success criteria
- 720p@30 on mid-range device, no persistent encoder errors, graceful `FLAG_SECURE` fallback.

## Failure criteria
- Encoder not available / inconsistent across devices, or Flutter texture integration requires deep native work not yet feasible.

## Perf / security concerns
- Encoder power/thermal, bitrate adaptation.
- User consent UI for projection must be shown.

## Decision to be made
Confirm H.264-first + fallback strategy and Linux decode/Flutter rendering path.

