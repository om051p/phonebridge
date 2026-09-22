// Package transfer implements PhoneBridge bidirectional file transfer
// (DEC-012 expanded into DEC-024, Phase 4).
//
// The engine is transport-agnostic, platform-agnostic pure Go: it owns
//
//   - TransferFrame framing and validation (one frame per DataChannel message)
//   - the offer/accept/chunk/complete/result/cancel state machine
//   - strict chunk ordering/offset validation and duplicate/replay refusal
//   - streaming SHA-256 integrity over exactly the bytes that were transferred
//   - bounded-memory streaming I/O (never a whole file in memory)
//   - temp-file staging with verification-before-promotion (Destination seam)
//   - typed terminal outcomes and UI-facing progress events
//   - cancellation, transport-loss interruption (restart-from-zero, no resume)
//
// The WebRTC side is injected through Channel, so the engine never imports Pion
// and every rule above is provable with a fake channel. The Pion adapter lives
// in transfer/rtcchannel; the Linux/plain-file destination in this package and
// the Android MediaStore destination behind the same Destination seam.
//
// Scope limits ratified in DEC-024: one outbound and one inbound transfer per
// session; 64 KiB chunks; no resume (an interrupted transfer fails typed and the
// user retries from zero); the receiver never exposes a partial file.
package transfer
