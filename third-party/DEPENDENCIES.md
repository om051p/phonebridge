# Dependencies

> Every new dependency requires upstream, exact version, LICENSE inspection, and Apache-2.0 compatibility before merge. See `CONTRIBUTING.md`.

## Phase 0 — declared or planned

| Dependency | Version | License | Apache-2.0 compatible | Notes |
|------------|---------|---------|------------------------|-------|
| `google.golang.org/protobuf` | **v1.36.12** | BSD-3 | Yes | Go protobuf runtime; **declares `go 1.23`** → sets the toolchain floor (DEC-017) |
| `github.com/google/go-cmp` | v0.7.0 | BSD-3 | Yes | transitive in `core/go.sum` (dependency's test helper; not linked into PhoneBridge binaries) |
| `google.golang.org/grpc` | v1.64.0 (planned pin) | Apache-2.0 | Yes | gRPC Go (local IPC) — **not yet imported**; arrives with the local IPC contract (spike 01) |
| `golang.org/x/net`, `x/sys`, `x/text` | (transitive, when grpc lands) | BSD-3 | Yes | via grpc |
| `bufbuild/buf` (tooling) | **1.73.0** (pinned) | Apache-2.0 | Yes | lint/breaking/generate; not a runtime dep |
| `buf.build/protocolbuffers/go` (remote plugin) | **v1.36.12** (pinned inline) | BSD-3 | Yes | Go codegen — pin to keep runtime/toolchain stable |
| `buf.build/grpc/go` (remote plugin) | **v1.5.1** (pinned inline) | Apache-2.0 | Yes | gRPC Go codegen (emits nothing until a `service` exists) |
| Flutter SDK | 3.47.2 (local); 3.47+ | BSD-3 | Yes | attribution required (bundled LICENSE); CI channel is not yet pinned |
| Dart `protobuf`, `grpc` | TBD (when wired) | BSD-3 | Yes | verify per package before pin |
| `flutter_lints` | ^4.0.0 | BSD-3 | Yes | dev only |
| `cupertino_icons` | ^1.0.6 | MIT | Yes | Flutter sample dep; replace with real UI deps later |

## Deferred (not added in Phase 0; check before adding)

| Candidate | License | Notes |
|-----------|---------|-------|
| `pion/webrtc` | MIT | Phase 2; validate via spike 04 |
| `godbus/dbus` | BSD-2 | Linux D-Bus; when needed |
| Go `uinput` | MIT | Linux input; spike 07 decides |
| mDNS (e.g. `hashicorp/mdns`, `grandcat/zeroconf`) | MIT/ BSD | spike 09 decides |

## Forbidden without explicit decision

- Any GPL/LGPL/AGPL dependency or KDE Connect / scrcpy source — reference-only.
- CI sanity checks `go.mod`/`pubspec.yaml` for `GPL` strings.
