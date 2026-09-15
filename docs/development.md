# Development

## Prerequisites

| Tool | Version (Phase 0 pin) | Notes |
|------|-----------------------|-------|
| Go | **1.23+** | `google.golang.org/protobuf v1.36.12` declares `go 1.23`; CI pins `1.23.x` |
| buf | **1.73.0** (pinned) | CI pins via `bufbuild/buf-setup-action`; `version: v2` config |
| protoc plugins | pinned inline in `proto/buf.gen.yaml` | Go `protocolbuffers/go:v1.36.12`, `grpc/go:v1.5.1` |
| Flutter | 3.47+ | `flutter analyze` / `flutter test` |
| Dart | 3.13+ | via Flutter |
| JDK | 17 | Android Gradle (when `android/` wired) |

Install `buf`: https://buf.build/docs/installation (or `go install github.com/bufbuild/buf/cmd/buf@latest`).

## Quickstart

```bash
# Protocol — run buf with CWD=proto/ (module root; `out` is resolved relative to CWD)
cd proto
buf lint
buf generate
cd ..
git status --porcelain -- core/pkg/protocol ui/lib/generated   # drift check: must be empty

# buf breaking must run from the repo root (baseline module root must match the input)
buf breaking proto --against '.git#branch=main,subdir=proto'

# Go core
cd core && go vet ./... && go build ./... && go test ./... -count=1 -race

# Flutter
cd ui && flutter pub get && flutter analyze && flutter test

# Android — NOT WIRED YET: there is no gradle wrapper and ui/android/ does not exist,
# so `flutter build apk` fails. See docs/architecture.md (Android host layout is an
# open Phase 0 decision; spike 02 decides it).
```

## Code generation

Canonical schema: `proto/phonebridge/v1/phonebridge.proto`.

```bash
# lint + generate: CWD must be proto/ (buf module root; `out` is CWD-relative)
cd proto && buf lint && buf generate

# breaking: run from the repo root (baseline module root must match the input)
buf breaking proto --against '.git#branch=main,subdir=proto'

# Make aliases (they cd into proto/ internally)
make -C core gen
make -C core check-generated   # gen, then fail on stale/uncommitted generated code
```

Generated outputs (committed — see `docs/decisions.md` DEC-015):

- Go: `core/pkg/protocol/phonebridgev1/phonebridge.pb.go` (directory matches `option go_package`)
- Dart: `ui/lib/generated/` (when the Dart plugin is enabled, after spike 01)

Never hand-edit generated files. CI fails on drift — including newly generated
(untracked) files, which `git diff --exit-code` alone cannot detect.

## Testing

| Layer | Command |
|-------|---------|
| Go | `go test ./...` (from `core/`, needs Go ≥ 1.23) |
| Flutter | `flutter test` |
| Android | **not wired**: no gradle wrapper; see "Quickstart" above |
| Proto | `cd proto && buf lint` + `buf breaking` from repo root |

Every subsystem needs deterministic tests before it is `CONFIRMED`. Note: the Go
tree currently has **zero** test files, so `go test` passes vacuously.

## CI

Workflow: `.github/workflows/ci.yml` — jobs `proto`, `go`, `flutter`, `docs`. Triggers on `push`/`pull_request` to `main`.

The `proto` job pins buf, lints and generates with CWD=`proto/`, fails on any
generated-code drift, and runs `buf breaking` from the repo root with
`fetch-depth: 0`. The `go` job pins Go `1.23.x` and caches on `core/go.sum`.

## Repository rules

Protocol-first and No-Cross — see `CONTRIBUTING.md`.
