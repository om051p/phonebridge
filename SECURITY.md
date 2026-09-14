# Security Policy

## Supported versions

| Version | Supported |
|---------|-----------|
| `main` (pre-release) | Best-effort triage; no stable release yet |

Stable version support will be declared at first tagged release.

## Reporting a vulnerability

- **Do not** open a public issue for a suspected vulnerability.
- Email: see GitHub Security Advisories for this repo (`Security` → `Report a vulnerability`), or contact a maintainer listed in `CODEOWNERS`.
- Include: affected version/commit, reproduction steps, impact assessment.
- Expect acknowledgement within 72 hours.

We follow coordinated disclosure. Please allow time to fix and release before public disclosure.

## Threat model (PLANNED — not yet fully implemented)

Primary assets: device identity keys (Ed25519), paired-device trust store, clipboard/file/notification payloads, WebRTC media.

Trust boundaries:

- Device ↔ device (protocol + WebRTC encrypted transport)
- App ↔ OS credential storage (Android Keystore, Linux Secret Service / libsecret)
- Flutter UI ↔ Go core (local IPC — UDS/gRPC — authenticated, local-only)
- Signaling server: rendezvous + SDP/ICE only; must not see plaintext payloads (TURN is transport relay only, app data remains encrypted)

See [`docs/security.md`](docs/security.md) for detailed model, pairing/SAS flow, and key-storage plan.

## Cryptography rules

- Use established primitives/libraries only.
- No custom cryptography without a documented threat model and review.
- Never silently trust an unknown device.

## Permissions

Android and Linux permissions required by each subsystem are documented in `docs/security.md` and in the relevant `android/` / `linux/` integration docs. Review them before adding new platform capabilities.
