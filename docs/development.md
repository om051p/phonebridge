# Development

## Prerequisites

| Tool | Version (Phase 0 pin) | Notes |
|------|-----------------------|-------|
| Go | 1.22+ | `go vet` + `go test` |
| buf | 1.32+ | `buf lint`/`breaking`/`generate` |
| protoc plugins | via `buf.gen.yaml` | Go + Dart generation |
| Flutter | 3.47+ | `flutter analyze` / `flutter test` |
| Dart | 3.13+ | via Flutter |
| JDK | 17 | Android Gradle (when `android/` wired) |

Install `buf`: https://buf.build/docs/installation (or `go install github.com/bufbuild/buf/cmd/buf@latest`).

## Quickstart

```bash
# Generate (requires buf)
buf generate
git diff --exit-code  # drift check

# Go core
cd core && go vet ./... && go test ./... -count=1 -race

# Flutter
cd ui && flutter analyze && flutter test

# Android
cd android && ./gradlew test
```

## Code generation

Canonical schema: `proto/phonebridge/v1/phonebridge.proto`.

```bash
buf lint
buf breaking --against '.git#branch=main'
buf generate
make -C core gen        # alias to buf generate (when wired)
make -C core check-generated  # buf generate && git diff --exit-code
```

Generated outputs:

- Go: `core/pkg/protocol/` (or `core/gen/` per `buf.gen.yaml`)
- Dart: `ui/lib/generated/`

Never hand-edit generated files. CI fails on drift.

## Testing

| Layer | Command |
|-------|---------|
| Go | `go test ./...` |
| Flutter | `flutter test` |
| Android | `./gradlew test` |
| Proto | `buf lint` + `buf breaking` |

Every subsystem needs deterministic tests before it is `CONFIRMED`.

## CI

Workflow: `.github/workflows/ci.yml` — jobs `proto`, `go`, `flutter` (conditional), `docs`. Triggers on `push`/`pull_request` to `main`.

## Repository rules

Protocol-first and No-Cross — see `CONTRIBUTING.md`.
