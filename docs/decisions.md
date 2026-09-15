# Decision Log

> Source: `MASTER_HANDOFF.md` §18. New Phase 0 decisions appended.

| ID | Decision | Direction | Status |
|----|----------|-----------|--------|
| DEC-001 | Product direction | Independent PhoneBridge product | `CONFIRMED` |
| DEC-002 | UI | Flutter | `CONFIRMED` |
| DEC-003 | Core | Go | `CONFIRMED` |
| DEC-004 | Android platform | Kotlin | `CONFIRMED` |
| DEC-005 | Protocol | Protobuf v3 | `CONFIRMED` — review in Phase 0 |
| DEC-006 | Network | WebRTC/Pion | `PLANNED` — needs spike 04 |
| DEC-007 | Discovery | mDNS + fallback | `PLANNED` — needs spike 09 |
| DEC-008 | Remote networking | ICE/STUN/TURN (P2P preferred) | `PLANNED` — needs spike 10 |
| DEC-009 | License | Apache-2.0 | `CONFIRMED` |
| DEC-010 | KDE Connect | Reference only, no source copy | `CONFIRMED` |
| DEC-011 | scrcpy | Reference only, no source copy | `CONFIRMED` |
| DEC-012 | File transfer | MVP, bidirectional | `PLANNED` |
| DEC-013 | Module path | `github.com/om051p/phonebridge` (from `origin`) — Go module `core/` uses `github.com/om051p/phonebridge/core` | `CONFIRMED` (Phase 0) |
| DEC-014 | Codegen toolchain | `buf` **pinned** (`bufbuild/buf-setup-action` with `version`); remote plugins **pinned inline** in `proto/buf.gen.yaml` (`protocolbuffers/go:v1.36.12`, `grpc/go:v1.5.1`). An unpinned plugin resolves to `latest`, which silently changes the required protobuf runtime and the required Go toolchain | `CONFIRMED` (Phase 0) |
| DEC-015 | Generated output | Generated Go **is committed** under `core/pkg/protocol/` (and Dart under `ui/lib/generated/` once enabled). CI drift gate fails on modified **or newly generated (untracked)** output. Canonical invocation is `cd proto && buf generate` (`out` is CWD-relative; from the repo root it writes outside the repository). Output uses `paths=import,module=github.com/om051p/phonebridge/core/pkg/protocol` so the on-disk directory matches `option go_package` (`.../protocol/phonebridgev1`) and imports resolve | `CONFIRMED` (Phase 0) |
| DEC-016 | Legacy handling | No legacy code in current `main`; if legacy appears, isolate on `archive/legacy-python` branch or `legacy/` dir — never imported by new layers | `CONFIRMED` (Phase 0) |
| DEC-017 | Go toolchain floor | `core/` requires **Go ≥ 1.23** (`google.golang.org/protobuf v1.36.12` declares `go 1.23`). CI pins `1.23.x`. `core/go.sum` is committed and is the CI cache key | `CONFIRMED` (Phase 0) |
| DEC-018 | Local IPC transport | **UDS + gRPC** (Spike 01 ratified; FFI rejected as primary). Contract `phonebridge.localipc.v1` (`proto/phonebridge/localipc/v1/local_ipc.proto`): `LocalEngineService` (`Handshake`/`Ping`/`StreamEvents`/`Health`) on `$XDG_RUNTIME_DIR/phonebridge/engine.sock`; `StreamEventsResponse` relays a pass-through `phonebridge.v1.Envelope` (UI decodes device payloads directly; daemon never becomes a device peer). Auth: SO_PEERCRED uid gate (primary) + per-start CSPRNG bearer token at `$XDG_RUNTIME_DIR/phonebridge/token` (0600) — clients read the file, attach `authorization: Bearer` metadata. Bulk content chunked ≤ 64 KiB. Spike schema `phonebridge.spike.localipc.v1` was never promoted | `CONFIRMED` (Phase 0) |

Update this file when a spike validates or overturns a `PLANNED`/`EXPERIMENTAL` choice.
