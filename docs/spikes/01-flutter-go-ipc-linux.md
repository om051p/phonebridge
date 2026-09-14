# Spike 01-flutter-go-ipc-linux — Flutter ↔ Go Local IPC on Linux

> Status: `EXPERIMENTAL` · Phase: 0 (framework only)


## Hypothesis
A Flutter Linux app can reliably talk to a Go daemon over a local, authenticated channel with low latency and clean lifecycle.

## Question
What is the best local IPC between Flutter and `phonebridge-daemon` on Wayland/COSMIC: UDS + gRPC/Protobuf vs FFI vs other?

## Target environment
- Ubuntu/COSMIC Wayland (primary)
- Another mainstream Wayland compositor (e.g. GNOME Wayland)
- Flutter stable, Go 1.23+, systemd user service

## Minimal prototype
- Go daemon listening on `$XDG_RUNTIME_DIR/phonebridge/engine.sock` with a trivial gRPC service (`Ping`).
- Flutter Linux calls it via `grpc` Dart over UDS (or FFI alternative branch).
- Lifecycle: daemon start/stop, socket ownership/permissions, reconnect after daemon restart.

## Success criteria
- Round-trip < 10 ms p50 on local machine; no leaks on reconnect.
- Works under systemd user service and manual launch.
- Auth boundary: only same-user processes can connect (socket perms).

## Failure criteria
- UDS unavailable/sandboxed, or Flutter–UDS integration requires fragile native code.
- Latency/jitter unacceptable vs FFI alternative.

## Perf / security concerns
- Socket permission/ownership; no world-readable socket.
- No plaintext secrets on socket — still enforce auth token/handshake.

## Decision to be made
Choose UDS+gRPC vs FFI (or other) as the canonical Linux local IPC and lock `proto` + generation for it.

