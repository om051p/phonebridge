# Graph Report - Phonebridge  (2026-09-14)

## Corpus Check
- cluster-only mode — file stats not available

## Summary
- 42 nodes · 27 edges · 18 communities (4 shown, 5 thin omitted)
- Extraction: 93% EXTRACTED · 7% INFERRED · 0% AMBIGUOUS · INFERRED: 2 edges (avg confidence: 0.88)
- Token cost: 3,677 input · 160 output

## Graph Freshness
- Built from commit: `b51b590b`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- Core System Architecture
- Project Documentation and Decisions
- Flutter Application Entry Point
- Flutter UI Smoke Tests
- Linux Daemon Entry Point
- Protocol Envelope Specification
- Security Threat Model
- Core Module Package Definition
- Protobuf Linter Configuration

## God Nodes (most connected - your core abstractions)
1. `Architecture Blueprint` - 5 edges
2. `Decision Log` - 5 edges
3. `Master Project Handoff` - 4 edges
4. `Go Core Engine` - 3 edges
5. `Contributing Guidelines` - 3 edges
6. `PhoneBridgeApp` - 2 edges
7. `main()` - 2 edges
8. `xdgRuntimeDir()` - 2 edges
9. `Flutter UI Layer` - 2 edges
10. `build` - 1 edges

## Surprising Connections (you probably didn't know these)
- `Buf Generation Configuration` --references--> `Go Core Engine`  [INFERRED]
  proto/buf.gen.yaml → docs/architecture.md
- `Signaling Server` --conceptually_related_to--> `Go Core Engine`  [INFERRED]
  server/README.md → docs/architecture.md
- `Architecture Blueprint` --references--> `Master Project Handoff`  [EXTRACTED]
  docs/architecture.md → MASTER_HANDOFF.md
- `Flutter Pubspec Configuration` --implements--> `Flutter UI Layer`  [EXTRACTED]
  ui/pubspec.yaml → docs/architecture.md
- `Contributing Guidelines` --references--> `Decision Log`  [EXTRACTED]
  CONTRIBUTING.md → docs/decisions.md

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **Protocol-First Code Generation Pipeline** — proto_buf_config, proto_buf_gen_config, docs_protocol_envelope, core_go_core [EXTRACTED 0.95]
- **Multi-Layer Architecture** — ui_flutter_ui, core_go_core, android_kotlin_host, linux_daemon [EXTRACTED 1.00]

## Communities (18 total, 5 thin omitted)

### Community 0 - "Core System Architecture"
Cohesion: 0.25
Nodes (8): Android Kotlin Host, Go Core Engine, Architecture Blueprint, Linux Go Daemon, Buf Generation Configuration, Signaling Server, Flutter UI Layer, Flutter Pubspec Configuration

### Community 1 - "Project Documentation and Decisions"
Cohesion: 0.29
Nodes (8): Contributing Guidelines, DEC-001: Independent Product, DEC-010: KDE Connect Reference Only, DEC-011: scrcpy Reference Only, Decision Log, Master Project Handoff, README, Security Policy

### Community 2 - "Flutter Application Entry Point"
Cohesion: 0.33
Nodes (5): package:flutter/material.dart, StatelessWidget, build, main, PhoneBridgeApp

### Community 3 - "Flutter UI Smoke Tests"
Cohesion: 0.50
Nodes (3): package:flutter_test/flutter_test.dart, package:phonebridge_ui/main.dart, main

## Knowledge Gaps
- **17 isolated node(s):** `github.com/om051p/phonebridge/core`, `build`, `main`, `main`, `Buf Generation Configuration` (+12 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 30 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **5 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Architecture Blueprint` connect `Core System Architecture` to `Project Documentation and Decisions`?**
  _High betweenness centrality (0.089) - this node is a cross-community bridge._
- **Why does `Master Project Handoff` connect `Project Documentation and Decisions` to `Core System Architecture`?**
  _High betweenness centrality (0.076) - this node is a cross-community bridge._
- **Are the 2 inferred relationships involving `Go Core Engine` (e.g. with `Buf Generation Configuration` and `Signaling Server`) actually correct?**
  _`Go Core Engine` has 2 INFERRED edges - model-reasoned connections that need verification._
- **What connects `github.com/om051p/phonebridge/core`, `build`, `main` to the rest of the system?**
  _17 weakly-connected nodes found - possible documentation gaps or missing edges._