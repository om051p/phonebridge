# Proto

Canonical schema: `phonebridge/v1/phonebridge.proto` (package `phonebridge.v1`).

## Toolchain

- `buf` ≥ 1.32 — lint, breaking, generate. Install: https://buf.build/docs/installation
- Plugins declared in `buf.gen.yaml` (Go `protocolbuffers/go` + `grpc/go`; Dart when vetted).

## Commands

```bash
buf lint
buf breaking --against '.git#branch=main'
buf generate
git diff --exit-code  # drift check (CI also runs this)
```

From repo root via Make:

```bash
make -C core gen
make -C core check-generated
```

## Outputs

- Go: `core/pkg/protocol/`
- Dart: `ui/lib/generated/` (when Dart plugin is enabled)

Never hand-edit generated files.
