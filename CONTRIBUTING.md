# Contributing to PhoneBridge

Source of truth: [`MASTER_HANDOFF.md`](MASTER_HANDOFF.md).

## Status lexicon

Every doc/code comment that describes feature state must use one tag:

| Tag | Meaning |
|-----|---------|
| `CONFIRMED` | Implemented, tested, reviewed |
| `VALIDATED` | Prototype confirms feasibility |
| `PLANNED` | Designed, not yet implemented |
| `EXPERIMENTAL` | Hypothesis, requires spike validation |

Do not describe `PLANNED`/`EXPERIMENTAL` work as done.

## No-Cross Rule

Each agent/layer owns its directory. Do not casually modify another layer.

| Layer | Directory |
|-------|-----------|
| Go core | `core/` |
| Kotlin Android | `android/` |
| Flutter UI | `ui/` |
| Linux integration | `linux/` |
| Protocol | `proto/` |
| Signaling server | `server/` |

Cross-layer change requires canonical interface update first (protocol or documented API), then per-layer follow-ups.

## Protocol-first workflow

```
proto/phonebridge/v1/phonebridge.proto  →  buf generate  →  layer impl  →  tests
```

1. Edit `proto/phonebridge/v1/phonebridge.proto`.
2. `buf lint && buf breaking --against '.git#branch=main'`.
3. `make gen` (or `buf generate`).
4. Update Go / Dart / Kotlin consumers.
5. Add/update tests. CI checks `git diff --exit-code` for drift.

Do not hand-edit generated code.

## Dependencies

Every new dependency requires **before merge**:

- upstream URL + exact version
- `LICENSE` file inspected (record actual SPDX id)
- Apache-2.0 compatibility confirmed
- entry in `third-party/DEPENDENCIES.md`

No GPL-family dependency without explicit licensing decision (recorded in `docs/decisions.md`). KDE Connect / scrcpy are reference-only.

## Development quickstart

See [`docs/development.md`](docs/development.md) for prerequisites.

```bash
# Go core
cd core && go vet ./... && go test ./... -count=1
# Protocol
buf lint && buf generate && git diff --exit-code
# Flutter (when ui/ exists)
cd ui && flutter analyze && flutter test
# Android (when android/ exists)
cd android && ./gradlew test
```

## Branches & PRs

- Branch from `main`, PR to `main`.
- One concern per PR; keep diffs small/testable.
- CI must be green before review.
- Update relevant docs (`docs/architecture.md`, `docs/protocol.md`, etc.) when behavior changes.

## Security

See [`SECURITY.md`](SECURITY.md). Do not weaken security to make a feature "just work". No custom crypto without documented threat model + review (see `docs/security.md`).

## AI-assisted development

This repo is built for heavy AI assistance. Prefer small, testable changes. Run relevant tests before reporting completion. Update docs when architecture changes.
