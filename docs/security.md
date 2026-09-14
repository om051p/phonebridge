# PhoneBridge Security

> Status tags: `CONFIRMED` `VALIDATED` `PLANNED` `EXPERIMENTAL` · Source: `MASTER_HANDOFF.md` §7

## Assets (`PLANNED`)

- Ed25519 identity keys + X25519 exchange state
- Paired-device trust store
- Clipboard / file / notification / media payloads
- WebRTC/ICE session state

## Pairing flow (`PLANNED`)

```
Linux displays QR → Android scans → exchange identities → mutual auth → SAS → user verifies → trusted
```

Never silently trust unknown device. SAS verification is mandatory before trust.

## Key storage (`PLANNED`)

| Platform | Target |
|----------|--------|
| Android | Android Keystore / hardware-backed |
| Linux | Secret Service / libsecret (desktop credential store) |

Fallback handling is documented per platform; keys must not be written in plaintext to app-private files when a secure store is available.

## Transport (`PLANNED`)

- WebRTC encrypted transport is baseline.
- App-level E2E may be added where justified by threat model — no custom crypto without documented threat model + review.
- Signaling server sees only rendezvous + SDP/ICE + auth metadata, not plaintext payloads.
- TURN is transport relay only; app data remains encrypted. Relay cost is ops cost, not trust cost.

## Permissions (to be enumerated per subsystem)

Document Android (`FOREGROUND_SERVICE_*`, `POST_NOTIFICATIONS`, `CAMERA` for QR, etc.) and Linux (D-Bus, portal, PipeWire, uinput) permissions alongside each feature PR. No undocumented privilege escalation.

## Cryptography rules

- Use established libraries only (Go `crypto/*`, audited WebRTC, platform keystore).
- No custom primitives, no home-rolled KDF/nonce schemes without review.

## Reporting

See [`SECURITY.md`](../SECURITY.md) for disclosure process.
