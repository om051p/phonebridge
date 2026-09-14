# PhoneBridge Protocol

> Package: `phonebridge.v1` · Schema: `proto/phonebridge/v1/phonebridge.proto` · Status: `PLANNED` (Phase 0 envelope only)

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

See `proto/README.md` for generation.
