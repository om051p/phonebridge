# PhoneBridge Protocol

> Packages: `phonebridge.v1` (device-to-device, `PLANNED` — Phase 0 envelope only) ·
> `phonebridge.localipc.v1` (UI ↔ engine local IPC, `CONFIRMED` — DEC-018)
> Schemas: `proto/phonebridge/v1/phonebridge.proto` ·
> `proto/phonebridge/localipc/v1/local_ipc.proto`

## Principles

- PhoneBridge-owned protocol. No KDE Connect/scrcpy wire-format reuse.
- Protobuf v3, versioned package `phonebridge.v1`.
- Protocol-first: proto changes before layer code; `buf breaking` enforced.

## Envelope (Phase 0)

`Envelope` is the top-level framing for every application message:

- `version` — protocol version (monotonic uint32)
- `device_id` — stable device identity (fingerprint of Ed25519 pubkey)
- `session_id` — per-connection session
- `sequence` — per-session monotonic sequence for replay protection
- `timestamp_ms` — sender wall clock (not trusted for ordering)
- `nonce` — per-message uniqueness for replay window
- `capabilities` — `CapabilitySet` advertised by sender
- `oneof payload` — exactly one feature payload or control message

Unknown `payload` variants must be ignored gracefully (forward compat).

## Capability & version negotiation (`PLANNED`)

- Handshake: `DeviceHello` advertises `min_version`/`max_version` + `CapabilitySet`.
- Intersection: receiver selects highest mutually supported `version` and capability subset.
- Incompatible peer → `Error.Code.INCOMPATIBLE_VERSION`.

Feature capabilities (examples): `CLIPBOARD`, `FILES`, `NOTIFICATIONS`, `SCREEN`, `INPUT`, `DEVICE_STATUS`.

## Sequence / replay (`PLANNED`)

- `sequence` strictly increasing per `session_id`; receiver tracks window and rejects duplicates/old nonces.
- Clock skew does not affect ordering; `sequence` is authoritative.

## Error model

```proto
enum Code { OK, INVALID_ARGUMENT, UNAUTHENTICATED, PERMISSION_DENIED,
            NOT_FOUND, ALREADY_EXISTS, INCOMPATIBLE_VERSION, RESOURCE_EXHAUSTED,
            INTERNAL, UNAVAILABLE }
message Error { Code code = 1; string message = 2; map<string,string> details = 3; }
```

Transport errors vs application errors are distinguished by `Code`; `details` is extensible.

## Compatibility & extensibility

- New fields: always optional / defaulted; never reuse field numbers.
- Removed fields: `reserved` number + name.
- New `oneof` branches: safe to add; old peers ignore unknown branch.
- Package versioning: breaking wire change → new package `phonebridge.v2` alongside `v1`.
- `buf breaking --against '.git#branch=main'` is CI gate.

## Phase 0 scope vs later

- Phase 0 locks envelope, `Capability`, `Error`, `VersionNegotiation`, `Ping`/`Pong`, and stubs for `DeviceHello`/`Pair*` — nothing more.
- Feature payloads (clipboard, file chunk, notification, screen/input) are empty stubs marked `// PLANNED` and filled in their respective phases.

## Local IPC (`phonebridge.localipc.v1` — CONFIRMED, DEC-018)

The UI ↔ engine boundary is a **separate package with a separate trust model**
from the device protocol; `phonebridge.v1` remains device-to-device only and
was not modified for local IPC.

- Schema: `proto/phonebridge/localipc/v1/local_ipc.proto`
- Transport: gRPC over the Unix domain socket
  `$XDG_RUNTIME_DIR/phonebridge/engine.sock` (dir `0700`, socket `0600`).
- Service `LocalEngineService`: `Handshake`, `Ping`, `StreamEvents`
  (server-stream), `Health`.
- `StreamEventsResponse` relays one `phonebridge.v1.Envelope` **verbatim**
  (pass-through framing, DEC-018). The daemon is the only component that
  speaks both packages; `LocalEngineService` is never a device peer.
- Isolation rules: the local contract MUST NOT be exposed on a network
  listener and MUST NOT gain device-peer semantics; new device payloads go in
  `phonebridge.v1` and are relayed through `StreamEventsResponse.envelope`.

### Authentication (both gates required)

1. **SO_PEERCRED uid gate (primary, kernel-verified)** — transport
   credentials checked on every accepted connection; peers whose UID differs
   from the daemon's effective UID are rejected pre-auth (Spike 01 pattern).
2. **Bearer token (defense-in-depth)** — gRPC metadata
   `authorization: Bearer <token>` on every call, constant-time compared.
   Provisioning: at startup the daemon generates ≥ 256 bits from a CSPRNG and
   writes `$XDG_RUNTIME_DIR/phonebridge/token` (mode `0600`, owner = daemon
   user). A legitimate client — any process running as the same OS user,
   exactly the privilege the uid gate enforces — reads that file and attaches
   the metadata to every call. The token rotates on each daemon start;
   clients re-read it after `UNAUTHENTICATED` and reconnect.

### Lifecycle & payloads

- Daemon: foreground process (systemd user service `Type=simple`,
  `Restart=on-failure`); SIGTERM → graceful stop, streams end cleanly, socket
  removed; stale sockets are cleaned at startup (verified, Spike 01).
- Clients: reconnect with backoff; detect daemon replacement via
  `HandshakeResponse.daemon_generation` and stream gaps via
  `StreamEventsResponse.seq`; re-read the token after `UNAUTHENTICATED`.
- Payloads: gRPC's default 4 MiB message ceiling applies; bulk content (file
  bytes, large clipboard bodies) MUST additionally be chunked ≤ 64 KiB per
  message (Spike 01: ~2× message cost at 64 KiB).

See `proto/README.md` for generation.
