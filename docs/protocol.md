# PhoneBridge Protocol

> Packages: `phonebridge.v1` (device-to-device; `CONFIRMED` for the handshake and
> screen-session negotiation — DEC-022, clipboard — DEC-023, and file transfer —
> DEC-024; notification/input/device-status payloads still `PLANNED`) ·
> `phonebridge.localipc.v1` (UI ↔ engine local IPC, `CONFIRMED` — DEC-018,
> session lifecycle extended in Phase 2, clipboard in Phase 3, transfer in
> Phase 4)
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

**Framing of feature DataChannels (DEC-024 clarification).** Envelope frames
*session-level control* messages and the local-IPC relay. A feature DataChannel
carries its own feature message directly: the shipped `clipboard` DataChannel
transports a bare `ClipboardUpdate` (Phase 3), and the `transfer` DataChannel
transports a bare `TransferFrame` (Phase 4). Each feature message is
self-describing (`TransferFrame` is a `oneof`), so no Envelope is needed on the
DC. The Envelope feature branches (`clipboard_update`, `file_*`) are retained for
wire compatibility and are not sent by current peers.

## Capability & version negotiation (`CONFIRMED` — Phase 2, DEC-022)

- Handshake: `DeviceHello` advertises `min_version`/`max_version`, `CapabilitySet`
  and `MediaCapabilities` (codecs, max width/height/fps, `supports_screen`).
- Intersection: receiver selects highest mutually supported `version` and capability subset.
- Incompatible peer → `Error.Code.INCOMPATIBLE_VERSION`, and **no partial session**
  is created: the handshake completes before any media parameter is exchanged.
- One active session per device pair; a second request fails typed (`SESSION_BUSY`).

Feature capabilities: `CLIPBOARD`, `FILES`, `NOTIFICATIONS`, `SCREEN`, `INPUT`, `DEVICE_STATUS`.
Only `SCREEN` must be honoured in Phase 2; the rest are advertised ahead of their phases.

## Sequence / replay (`PLANNED`)

- `sequence` strictly increasing per `session_id`; receiver tracks window and rejects duplicates/old nonces.
- Clock skew does not affect ordering; `sequence` is authoritative.

## Error model

```proto
enum Code { OK, INVALID_ARGUMENT, UNAUTHENTICATED, PERMISSION_DENIED,
            NOT_FOUND, ALREADY_EXISTS, INCOMPATIBLE_VERSION, RESOURCE_EXHAUSTED,
            INTERNAL, UNAVAILABLE,
            UNSUPPORTED_MEDIA_PARAMS, CONSENT_REVOKED, CAPTURE_FAILED,
            TRANSPORT_FAILED, RECONNECT_TIMEOUT, SESSION_BUSY }
message Error { Code code = 1; string message = 2; map<string,string> details = 3; }
```

Transport errors vs application errors are distinguished by `Code`; `details` is extensible.
The Phase 2 additions (DEC-022) exist so callers never infer cause from prose:
`CONSENT_REVOKED` and `CAPTURE_FAILED` are sender-side conditions on a healthy
link, while `TRANSPORT_FAILED` and `RECONNECT_TIMEOUT` are link-side.

## Screen session negotiation (`CONFIRMED` — Phase 2, DEC-022)

One `MediaParams` tuple (`width`, `height`, `fps`, `bitrate_kbps`, `codec`) is
shared by the device protocol (`ScreenStart`), the LAN signaling exchange and
the local IPC session snapshot, so the same shape flows end to end.

1. The initiator sends `ScreenStart.requested` **before** the offer is composed.
   Android needs a MediaProjection consent before capture exists (DEC-020), so
   parameters are settled up front and never mid-stream.
2. The capture device always answers with `accepted` + `actual`. **It is
   authoritative for `actual`** (DEC-020): a reply that differs from the request
   is a *reported downgrade, never a silent substitution*.
3. A request that cannot be met within the device's `MediaCapabilities` fails
   typed (`CODE_UNSUPPORTED_MEDIA_PARAMS`) instead of substituting a stream.
4. Resolution changes are never negotiated in-session: a new geometry needs a
   new consent and a new session (DEC-020).

Transport for this exchange is the ratified LAN signaling endpoint (DEC-022);
`ScreenStop` carries the typed reason for teardown.

## File transfer (`CONFIRMED` — Phase 4, DEC-024)

One dedicated `transfer` DataChannel per session (reliable, ordered, created by
the same side that creates `control`/`clipboard`) carries exactly one
`TransferFrame` per DataChannel message:

```
FileOffer → FileAccept → FileChunk×N → FileComplete → FileResult
                 \___ FileCancel (either side, after the offer) ___/
```

- **Transfer id**: random 128-bit hex, in every frame. A second offer for a live
  or known id is a protocol violation (the replay/duplicate guard).
- **Chunking**: `chunk_size` (1…65536, default 65536) declared in the offer;
  `FileChunk.chunk_index` must be exactly next-expected and `offset` must equal
  `chunk_index × chunk_size` — a gap, duplicate or reorder aborts the transfer.
  The reliable ordered DC is the ordering guarantee; validation catches bugs.
- **Integrity**: `FileComplete` always carries size + SHA-256 of the bytes that
  were sent, and the receiver verifies both before promoting the file. The
  offer's optional `sha256_digest` is an extra up-front declaration (Phase 4
  senders leave it empty so sending stays single-pass).
- **Limits**: one outbound + one inbound transfer per session; a second inbound
  offer is refused with `CODE_TRANSFER_BUSY`. Bounded memory only: the sender
  never buffers more than one 64 KiB chunk plus a 1 MiB SCTP high-watermark.
- **Termination**: exactly one `FileResult` or `FileCancel` ends a transfer;
  "no answer" is never success. The receiver never exposes a partial file:
  Linux writes a temp file inside the destination filesystem and `rename`s after
  verification, Android uses a MediaStore item with `IS_PENDING=1`.
- **Reconnect**: Phase 4 has no resume. A lost DataChannel/session aborts every
  in-flight transfer with `CODE_TRANSFER_INTERRUPTED`, deletes the partial, and
  the UI offers a fresh retry (new transfer id).

Typed failure codes (appended to `Code`): `TRANSFER_INTERRUPTED`,
`CHECKSUM_MISMATCH`, `STORAGE_FAILED`, `FILE_TOO_LARGE`, `UNSAFE_FILENAME`,
`TRANSFER_BUSY`, `TRANSFER_CANCELLED`; plus existing `INVALID_ARGUMENT`
(protocol violation), `NOT_FOUND` (unknown transfer id), `UNAVAILABLE` (no
`transfer` DataChannel) and `INCOMPATIBLE_VERSION` (frame version).

## Compatibility & extensibility

- New fields: always optional / defaulted; never reuse field numbers.
- Removed fields: `reserved` number + name.
- New `oneof` branches: safe to add; old peers ignore unknown branch.
- Package versioning: breaking wire change → new package `phonebridge.v2` alongside `v1`.
- `buf breaking --against '.git#branch=main'` is CI gate.

## Phase 0 scope vs later

- Phase 0 locked the envelope, `Capability`, `Error`, `VersionNegotiation`,
  `Ping`/`Pong`, and stubs for `DeviceHello`/`Pair*`.
- Phase 2 (DEC-022) promoted `DeviceHello` (now carrying `MediaCapabilities`),
  `ScreenStart` and `ScreenStop` to real messages and added the typed session
  failure codes. The handshake and screen negotiation are therefore implemented
  contracts, not stubs.
- Phase 3 (DEC-023) promoted `ClipboardUpdate`; Phase 4 (DEC-024) promoted the
  file-transfer family (`TransferFrame`, `FileOffer`, `FileAccept`,
  `FileChunk`, `FileComplete`, `FileResult`, `FileCancel`) and appended the
  transfer failure codes.
- Remaining feature payloads (notification, input, device status) are still
  empty stubs marked `// PLANNED` and are filled in their respective phases.

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
- Session lifecycle (Phase 2, DEC-022): `StartSession` accepts the requested
  `phonebridge.v1.MediaParams`, and `GetSessionState`/`SessionEvent` return the
  **requested vs actual** parameters plus a typed `SessionReason`. Sharing the
  parameter *type* across the two packages is deliberate (one definition of the
  tuple); it does not make the local contract a transport for `phonebridge.v1`.
- Isolation rules: the local contract MUST NOT be exposed on a network
  listener and MUST NOT gain device-peer semantics; new device payloads go in
  `phonebridge.v1` and are relayed through `StreamEventsResponse.envelope`.
- File transfer (Phase 4, DEC-024): `SendFile` takes a **local path** (the
  daemon reads the file itself — file bytes never cross local IPC),
  `CancelTransfer` aborts either direction, `ListTransfers` returns in-flight
  transfers plus the recent in-memory history, and `TransferEvent` is pushed on
  `StreamEventsResponse.transfer_event`. Transfer state/reason enums are local
  to this contract, mirroring the `SessionState`/`SessionReason` precedent.

### Authentication (both gates required)

1. **SO_PEERCRED uid gate (primary, kernel-verified)** — transport
   credentials checked on every accepted connection; peers whose UID differs
   from the daemon's effective UID are rejected pre-auth (Spike 01 pattern).
2. **Bearer token (defense-in-depth)** — gRPC metadata
   `authorization: Bearer <token>` on every call, constant-time compared.
   - **Provisioning:** At startup the daemon generates ≥ 256 bits from a CSPRNG
     and writes the secret to `$XDG_RUNTIME_DIR/phonebridge/token` (mode `0600`,
     owned by the daemon's effective UID).
   - **Flutter Client Acquisition:** The Flutter UI client (running as the same OS
     user) resolves `$XDG_RUNTIME_DIR/phonebridge/token`. On cold start or system boot
     when the daemon is launching, the client polls for the token file with backoff
     (e.g., 50 ms initial, doubling to a 3 s cap). Once read into memory, the client
     wires it into the gRPC stub via `CallOptions(metadata: {'authorization': 'Bearer $token'})`.
   - **Rotation & Reconnect:** The token rotates on every daemon restart. If any RPC
     fails with gRPC status `UNAUTHENTICATED` (code 16), the client clears its token cache,
     re-reads `$XDG_RUNTIME_DIR/phonebridge/token`, and reconnects with exponential backoff.

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
