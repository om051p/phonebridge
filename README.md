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
# Proto (buf pinned; module root is proto/, so lint/generate run with CWD=proto/)
cd proto && buf lint && buf generate && cd ..
buf breaking --config proto/buf.yaml --against '.git#branch=main'   # from repo root
git status --porcelain -- core/pkg/protocol ui/lib/generated         # drift check: must be empty

# Go core (requires Go >= 1.23 — see docs/decisions.md DEC-017)
cd core && go vet ./... && go build ./... && go test ./... -count=1 -race

# Flutter
cd ui && flutter pub get && flutter analyze && flutter test

# Android: NOT WIRED — no gradle wrapper; `flutter build apk` fails until the
# Android host project layout is decided (see docs/development.md).
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
