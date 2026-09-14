# PhoneBridge

Open-source Android ↔ Linux device integration — pairing, LAN discovery,
clipboard sync, file sharing, notification forwarding, device status,
screen mirroring, and remote interaction.

**Status:** Pre-development. Implementation not started.
**Source of truth:** [`MASTER_HANDOFF.md`](MASTER_HANDOFF.md)

## Stack

| Layer   | Tech    |
|---------|---------|
| UI      | Flutter |
| Core    | Go      |
| Android | Kotlin  |
| Linux   | Go daemon |

## Rules

- Protocol-first: `proto/phonebridge/v1/phonebridge.proto` changes before layer code.
- No-cross: each agent works its own layer (`core/`, `android/`, `ui/`, `linux/`, `proto/`, `server/`).
- KDE Connect / scrcpy are reference-only. No copied code, no GPL imports.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
