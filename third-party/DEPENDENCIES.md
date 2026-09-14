# Dependencies

> Every new dependency requires upstream, exact version, LICENSE inspection, and Apache-2.0 compatibility before merge. See `CONTRIBUTING.md`.

## Phase 0 — declared or planned

| Dependency | Version | License | Apache-2.0 compatible | Notes |
|------------|---------|---------|------------------------|-------|
| `google.golang.org/protobuf` | v1.34.2 (pin) | BSD-3 | Yes | Go protobuf runtime |
| `google.golang.org/grpc` | v1.64.0 (pin) | Apache-2.0 | Yes | gRPC Go (local IPC) |
| `google.golang.org/genproto` | (transitive) | Apache-2.0 | Yes | via grpc |
| `golang.org/x/net`, `x/sys`, `x/text` | (transitive) | BSD-3 | Yes | via grpc/protobuf |
| `bufbuild/buf` (tooling) | ≥1.32 | Apache-2.0 | Yes | lint/breaking/generate; not a runtime dep |
| `buf.build/protocolbuffers/go` (remote plugin) | latest | BSD-3 | Yes | Go codegen |
| `buf.build/grpc/go` (remote plugin) | latest | Apache-2.0 | Yes | gRPC Go codegen |
| Flutter SDK | 3.47+ | BSD-3 | Yes | attribution required (bundled LICENSE) |
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
