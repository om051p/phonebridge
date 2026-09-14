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
| DEC-014 | Codegen | `buf` pinned ≥1.32; `buf.gen.yaml` for Go+Dart; `make check-generated` drift gate | `CONFIRMED` (Phase 0) |
| DEC-015 | Generated output | Committed under `core/pkg/protocol/` + `ui/lib/generated/` with drift gate (TBD final if generated size warrants gitignore) | `PLANNED` |
| DEC-016 | Legacy handling | No legacy code in current `main`; if legacy appears, isolate on `archive/legacy-python` branch or `legacy/` dir — never imported by new layers | `CONFIRMED` (Phase 0) |

Update this file when a spike validates or overturns a `PLANNED`/`EXPERIMENTAL` choice.
