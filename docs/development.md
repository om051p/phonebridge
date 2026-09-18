# Development

## Prerequisites

| Tool | Version (Phase 0 pin) | Notes |
|------|-----------------------|-------|
| Go | **1.24+** | Pion v4 (`webrtc/v4 v4.2.20`) declares `go 1.24.0` (DEC-006/DEC-017); CI pins `1.24.x` |
| buf | **1.73.0** (pinned) | CI pins via `bufbuild/buf-setup-action`; `version: v2` config |
| protoc plugins | pinned inline in `proto/buf.gen.yaml` | Go `protocolbuffers/go:v1.36.12`, `grpc/go:v1.5.1` |
| Flutter | 3.47+ | `flutter analyze` / `flutter test` |
| Dart | 3.13+ | via Flutter |
| JDK | 17 | Android Gradle module (`android/`), JNI headers for `core/cmd/android` |
| Android NDK | 27.0.12077973 | Cross-compiles `libphonebridge_core.so` (DEC-019); see `core/Makefile` |

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

# Android host (DEC-019) — Kotlin service + Go c-shared JNI library.
# The generated .so/.h are gitignored: build them, don't commit them.
make -C core host-lib                            # host-JVM lib for the unit tests
cd android && ./gradlew :app:testDebugUnitTest   # JVM unit tests
make -C core android-lib                         # cross-compile jniLibs/*.so (arm64-v8a, x86_64)
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
- Go: `core/pkg/protocol/phonebridgelocalipcv1/` (DEC-018 local IPC contract)
- Dart: `ui/lib/generated/` (`buf.build/protocolbuffers/dart:v25.1.0`, `opt: grpc`)

Never hand-edit generated files. CI fails on drift — including newly generated
(untracked) files, which `git diff --exit-code` alone cannot detect.

## Testing

| Layer | Command |
|-------|---------|
| Go | `go test ./...` (from `core/`, needs Go ≥ 1.24) |
| Flutter | `flutter test` |
| Android | `cd android && ./gradlew :app:testDebugUnitTest` (JVM; JNI bridge tests need `make -C core host-lib`) |
| Android (device) | `cd android && ./gradlew :app:connectedDebugAndroidTest` |
| Proto | `cd proto && buf lint` + `buf breaking` from repo root |

Every subsystem needs deterministic tests before it is `CONFIRMED`. Test suites
exist for the crypto/trust, discovery, engine, local IPC, receiver, RTP media and
WebRTC packages, the JNI bridge (`-tags jni`), and the Kotlin security/signaling/
capture layers.

## CI

Workflow: `.github/workflows/ci.yml` — jobs `proto`, `go`, `flutter`, `docs`. Triggers on `push`/`pull_request` to `main`.

The `proto` job pins buf, lints and generates with CWD=`proto/`, fails on any
generated-code drift, and runs `buf breaking` from the repo root with
`fetch-depth: 0`. The `go` job pins Go `1.24.x` and caches on `core/go.sum`.

## Repository rules

Protocol-first and No-Cross — see `CONTRIBUTING.md`.
