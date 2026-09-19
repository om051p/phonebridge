// Package clipboard provides the platform-independent clipboard synchronization
// engine for PhoneBridge (DEC-023, Phase 3).
//
// The engine owns:
//   - Local and remote clipboard state management
//   - V1 MIME normalization (text/plain and text/plain;charset=utf-8)
//   - Strict payload size enforcement (768 KiB / 786,432 bytes)
//   - SHA-256 payload digest generation and verification
//   - Thread-safe, bounded LRU echo suppression (capacity 32, TTL 5,000 ms)
//   - Reconnect conflict arbitration (timestamp difference >1000 ms newer wins;
//     difference <=1000 ms Linux/Desktop tie-breaker)
//   - Bidirectional conversion with phonebridge.v1.ClipboardUpdate protobuf
//   - Decoupled interfaces for platform adapters and WebRTC DataChannel transport
//
// Platform adapters (Wayland helper on Linux, companion IME on Android) and
// WebRTC DataChannel transport are decoupled via interfaces. No locks are held
// during platform or transport callback invocation.
package clipboard
