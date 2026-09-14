# PhoneBridge

Open-source Android ↔ Linux device integration — pairing, LAN discovery, clipboard sync, file sharing, notification forwarding, device status, screen mirroring, and remote interaction.

**Status:** `PLANNED` — Phase 0 scaffolding. No MVP features implemented.
**Source of truth:** [`MASTER_HANDOFF.md`](MASTER_HANDOFF.md) · Architecture: [`docs/architecture.md`](docs/architecture.md)

## Stack

| Layer   | Tech    | Dir |
|---------|---------|-----|
| UI      | Flutter | `ui/` |
| Core    | Go      | `core/` |
| Android | Kotlin  | `android/` |
| Linux   | Go daemon | `linux/` + `core/cmd/daemon` |
| Protocol | Protobuf v3 | `proto/phonebridge/v1/` |

## Structure

```
proto/phonebridge/v1/   protobuf (buf.yaml, buf.gen.yaml, phonebridge.proto)
core/pkg/*, core/cmd/*  Go core + daemon (pkg: crypto, discovery, webrtc, protocol, transfer, clipboard, engine)
android/                Kotlin + Gradle (thin platform layer)
linux/                  D-Bus, portals, systemd unit
server/                 signaling rendezvous (Phase 4)
ui/                     Flutter
docs/                   architecture, security, protocol, development, spikes
tests/                  cross-layer / network-sim harnesses (later)
third-party/            dependency ledger
```

## Quickstart

```bash
# Proto (requires buf ≥1.32)
buf lint --config proto/buf.yaml
buf generate --config proto/buf.yaml --template proto/buf.gen.yaml
buf breaking --config proto/buf.yaml --against '.git#branch=main'

# Go core
cd core && go vet ./... && go test ./... -count=1 -race

# Flutter
cd ui && flutter pub get && flutter analyze && flutter test

# Android (when wired)
cd android && ./gradlew test
```

Details: [`docs/development.md`](docs/development.md).

## Rules

- **Protocol-first:** `proto/phonebridge/v1/phonebridge.proto` changes before layer code → `buf generate` → impl → tests.
- **No-Cross:** each agent owns its layer (`core/`, `android/`, `ui/`, `linux/`, `proto/`, `server/`).
- KDE Connect / scrcpy are reference-only. No copied code, no GPL imports without explicit decision.

See [`CONTRIBUTING.md`](CONTRIBUTING.md), [`SECURITY.md`](SECURITY.md), [`NOTICE`](NOTICE).

## Status lexicon

`CONFIRMED` · `VALIDATED` · `PLANNED` · `EXPERIMENTAL` — see `CONTRIBUTING.md` and `docs/decisions.md`.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
