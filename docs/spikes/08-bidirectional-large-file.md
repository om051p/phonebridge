# Spike 08-bidirectional-large-file — Bidirectional Large-File Transfer

> Status: `ABSORBED by Phase 4` (DEC-024) · Phase: 0 framework, never executed standalone
>
> This framework spike was never run as a separate spike. Its decisions
> (reliable/ordered DataChannel, chunked payloads, SHA-256, SAF on Android,
> progress/cancel, temp-file handling, no path traversal) were settled directly
> as a ratified design in **DEC-024** and implemented in `core/pkg/transfer` +
> the `transfer` WebRTC DataChannel, with measured evidence recorded in
> `docs/decisions.md`. The open question it recorded — resumability — is
> explicitly deferred: Phase 4 is restart-from-zero.


## Hypothesis
WebRTC DataChannel reliable/ordered + chunked payloads can carry large files both directions with progress/cancel/integrity.

## Question
Throughput, resumability, and integration with SAF (Android) / filesystem (Linux) for large files (hundreds of MB to GB)?

## Target environment
- Android ↔ Linux, same LAN, large files, flaky Wi-Fi simulation

## Minimal prototype
- Chunked transfer over DataChannel, SHA-256 verification, progress/cancel, SAF on Android, drag-and-drop/Share Sheet hooks stubbed.
- Simulate packet loss/latency; measure throughput vs file size.

## Success criteria
- 1 GB file completes with integrity check; cancel is prompt; no OOM.

## Failure criteria
- DataChannel backpressure/buffering makes large files impractical vs alternative transport.

## Perf / security concerns
- Backpressure, resume metadata, temp-file handling, no path traversal.

## Decision to be made
Lock file-transfer chunking/resume/integrity design and protocol fields.

