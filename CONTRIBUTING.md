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
proto/phonebridge/v1/phonebridge.proto  →  cd proto && buf generate  →  layer impl  →  tests
```

1. Edit `proto/phonebridge/v1/phonebridge.proto`.
2. `cd proto && buf lint`, then from the repo root `buf breaking proto --against '.git#branch=main,subdir=proto'`.
3. Generate: `cd proto && buf generate` (or `make -C core gen`).
4. Update Go / Dart / Kotlin consumers.
5. Add/update tests. **Commit the regenerated files** — CI fails on any drift, including newly
   generated files that are not yet tracked (`git status --porcelain -- core/pkg/protocol ui/lib/generated`
   must be empty). See `docs/decisions.md` DEC-015.

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
# Go core (requires Go >= 1.23 — docs/decisions.md DEC-017)
cd core && go vet ./... && go build ./... && go test ./... -count=1
# Protocol (buf module root is proto/)
cd proto && buf lint && buf generate && cd ..
git status --porcelain -- core/pkg/protocol ui/lib/generated   # must be empty
# Flutter (when ui/ exists)
cd ui && flutter analyze && flutter test
# Android: NOT WIRED — no gradle wrapper yet, so `./gradlew test` cannot run.
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
