# Proto

Canonical schema: `phonebridge/v1/phonebridge.proto` (package `phonebridge.v1`).
This directory is the buf **module root**.

## Toolchain

- `buf` — pinned (CI: `bufbuild/buf-setup-action` with `version`; local: 1.73.0).
- Plugins pinned inline in `buf.gen.yaml`: Go `protocolbuffers/go:v1.36.12`, `grpc/go:v1.5.1`.
  Unpinned remote plugins resolve to `latest`, which silently changes the required
  protobuf runtime and therefore the required Go toolchain (see `docs/decisions.md`
  DEC-014 / DEC-017).

## Commands

All buf commands run with CWD = this directory, because module resolution and plugin
`out` paths are relative to the working directory:

```bash
cd proto
buf lint
buf generate
buf format --diff --exit-code phonebridge/v1/phonebridge.proto   # optional style check
```

`buf breaking` must also run from the **repository root** — `.git#ref` resolves `.git`
relative to the working directory, and the baseline image is built with the repo root as
module root (so comparing from here reports a false `FILE_NO_DELETE`). Pin BOTH sides to
the module root with an explicit input + `subdir`; the bare root-CWD form breaks once a
proto imports another (cross-package imports resolve against the CWD as module root):

```bash
buf breaking proto --against '.git#branch=main,subdir=proto'
```

Drift check (must be empty; detects modified *and* newly generated files):

```bash
git status --porcelain -- core/pkg/protocol ui/lib/generated
```

From repo root via Make:

```bash
make -C core gen
make -C core check-generated
```

## Outputs (committed — DEC-015)

- Go: `core/pkg/protocol/phonebridgev1/phonebridge.pb.go`
  (`paths=import,module=github.com/om051p/phonebridge/core/pkg/protocol` makes the
  directory match `option go_package`, so imports resolve)
- Go (local IPC): `core/pkg/protocol/phonebridgelocalipcv1/local_ipc*.pb.go`
  (package `phonebridge.localipc.v1`; same pinned plugins, same module mapping)
- Dart: `ui/lib/generated/` (when the Dart plugin is enabled, after spike 01)

Never hand-edit generated files. CI fails on drift.

## Next protocol change

The Phase 0 schema is an unreleased draft (see `.proto` header). Renames such as
`Error` → `ProtocolError` / `Code` → `ErrorCode` are still free before the freeze.
