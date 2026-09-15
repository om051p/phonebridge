# Spike 01 Results — Flutter ↔ Go local IPC on Linux

> Status: `EXPERIMENTAL` — spike artifact. Evidence for the transport decision
> that gates freezing the IPC contract; results feed a future DEC entry in
> `docs/decisions.md` once ratified.

Frame: [01-flutter-go-ipc-linux.md](01-flutter-go-ipc-linux.md) ·
Prototype: [`spikes/01-flutter-go-ipc-linux/`](../../spikes/01-flutter-go-ipc-linux/) ·
Raw data: [`spikes/01-flutter-go-ipc-linux/results/`](../../spikes/01-flutter-go-ipc-linux/results/)
*(regenerable per-run evidence via `make perf`; intentionally not committed —
the distilled numbers below are the durable record)*

## Verdict

**UDS + gRPC is the recommended transport for the Flutter ↔ Go boundary on
Linux.** Feasibility is proven with zero native/FFI code on the Dart side, the
latency budget is met with ~40× headroom against a 60 fps frame, push works as
a first-class primitive, and the security boundary (kernel-verified peer uid +
bearer token) exists and is tested. FFI is rejected as the primary boundary:
its raw call latency is ~500× lower, but it has no auth boundary, no push
model, no crash isolation, and its safe use requires an isolate+port harness
that costs more complexity than the transport saves.

## What was prototyped

Everything lives under `spikes/01-flutter-go-ipc-linux/`; the product schema
(`proto/phonebridge/v1`), product codegen paths, `core/`, and `ui/` were not
touched (CI drift gate would catch that).

- `proto/` — spike-local `phonebridge.spike.localipc.v1` (Ping / Subscribe /
  WhoAmI) + buf config with locally pinned plugins (Go 1.36.12 / grpc-go
  1.5.1 / dart 25.1.0). Generated Go + Dart (incl. gRPC stubs) into the spike
  tree only.
- `go/cmd/spiked` — the "engine" stand-in: UDS listener (0700 dir, 0600
  socket, umask hardened), stale-socket cleanup, live-socket refusal, SIGTERM →
  `GracefulStop`, `READY`/`STATS` stdout lines, stats counters.
- `go/internal/spikegrpc` — gRPC server with a custom
  `credentials.TransportCredentials` that reads **SO_PEERCRED on every
  connection**, plus uid allowlist (`--expect-uid`) and bearer-token
  (`--token`, constant-time compare) gates.
- `go/cshared` — the FFI alternative: `PBSpikePing/EchoRaw/PollEvent/
  StartTicker/BlockMicros` with caller-owned buffers (no C allocs).
- `harness/` — Flutter test package driving the real daemon binary and the
  real `.so` from Dart: feasibility, security, lifecycle, and performance
  suites (27 tests, all passing) + a JSON evidence writer.

## Method and caveats

- The harness spawns the actual `spiked` binary and connects with the actual
  grpc-dart `ClientChannel` over a Unix socket; FFI tests `dlopen` the actual
  `libphonebridge_spike.so`.
- Latency = wall-clock `Stopwatch` around each awaited call, after 200 warmup
  calls, 2000 measured iterations unless stated otherwise. Percentiles over
  per-call samples.
- **Caveat:** the Flutter test VM is JIT/debug, so absolute numbers are
  pessimistic versus an AOT release build. Cross-branch ratios (UDS vs FFI)
  and order-of-magnitude conclusions are the meaningful output. Re-run
  `make -C spikes/01-flutter-go-ipc-linux perf` on target hardware before the
  contract freeze if exact budgets matter.
- Single-user test environment: no second uid available, so the uid gate is
  exercised via the `--expect-uid` mismatch proxy (documented in the security
  suite) instead of a real cross-uid connect.

## Results

### Feasibility

| Question | Answer |
|---|---|
| Can grpc-dart reach the Go daemon over a Unix socket without native code? | **Yes.** `ClientChannel(InternetAddress(path, type: InternetAddressType.unix), port: 0)` — no plugin, no CMake change, no FFI. |
| Does gRPC server-streaming work for Flutter ← Go push? | **Yes.** Ordered events with ~0.5 ms delivery latency (see below). |
| Does the FFI branch build and call? | **Yes.** 1.8 MB `.so`, dlopen ≈ 0.9 ms, sub-µs calls. |
| Lifecycle (spawn/READY/stop/crash/restart) | **Yes.** READY ≈ 10 ms after spawn; SIGTERM → socket removed + `STOPPED`; SIGKILL → stale socket cleaned by next start (also the systemd `Restart=on-failure` recovery path); double-start refused. |

### Performance (debug/JIT harness; see caveat above)

UDS + gRPC:

| Metric | Value | Notes |
|---|---|---|
| Unary Ping p50 / p99 | **0.44 ms / 1.3 ms** | n=2000, sequential ≈ 2000 rps |
| Payload sweep p50 | 0.35–0.42 ms up to 16 KiB; 0.85 ms @ 64 KiB | 64 KiB ≈ +2× — future chunk-sizing evidence |
| 64 in-flight calls | **≈ 3300 rps** | mean 0.30 ms/call |
| Push (server stream) delivery | p50 ≈ 0.48 ms | same-host clock; emission → Dart receive |
| Push inter-arrival | 20.5 ms for a 20 ms cadence | timer jitter ≈ +0.5 ms |
| Cold channel first call | 62 ms | one-time per app launch, not per screen |
| Stream jitter under concurrent load | 11–32 ms events spaced 100 ms | jitter, not loss |

FFI (in-process):

| Metric | Value | Notes |
|---|---|---|
| Direct call p50 | **< 1 µs** (≈ 4M rps) | the raw-speed selling point |
| Echo 64 KiB | ≈ 1 µs p50 | memcpy-level |
| dlopen + symbols | ≈ 0.9 ms | one-time |
| Isolate hop (per-call spawn) | p50 99 µs / p99 1.2 ms | upper bound; a long-lived worker isolate sits between this and direct |

Headroom: a 60 fps frame is 16.7 ms. Even p99 (~1.3 ms) leaves **~40× headroom**
for real engine work inside the budget; the cold-start 62 ms is the only number
outside per-frame budgets and is a one-time cost.

### Security

| Control | UDS + gRPC | FFI |
|---|---|---|
| Auth boundary | Kernel-verified: SO_PEERCRED read on **every** connection via `credentials.TransportCredentials`; the client cannot spoof uid | **None** — the core is in-process; any code that can run Dart can do anything |
| Bearer token | Yes (`--token`, constant-time compare) | N/A (nothing to authenticate) |
| uid allowlist | Yes (`--expect-uid`; tested via the mismatch proxy — no second uid in this env) | N/A |
| Socket exposure | dir 0700 + socket 0600 verified; a 0000 socket is un-connectable; connect needs write perm | No socket; compromise of the UI process = compromise of the core |
| Malicious-client containment | gRPC rejects pre-auth (UNAUTHENTICATED) before the uid gate (PERMISSION_DENIED) | Core shares the UI's fate: a crash or exploit on either side takes down both |

### Lifecycle & operations

- Process model proven: foreground daemon, SIGTERM → graceful stop with socket
  removal, STATS line as shutdown evidence. Maps directly to systemd
  `Type=simple`, `Restart=on-failure`.
- Stale-socket cleanup is exercised and is the normal recovery path after a
  crash.
- No fd leak across 15 connect/ping/close cycles (8 → 8 fds); RSS grows ~1.4 MB
  (Go runtime warm-up, not per-cycle).
- Client reconnect after a daemon restart works; channel recreation verified.

### FFI branch findings (why it loses despite winning on speed)

1. **No auth boundary** — everything in the security table is structurally
   impossible in-process.
2. **All exports block the calling isolate.** Measured: a 10 ms timer fired at
   **51 ms** while a 50 ms call ran on the same isolate. Every synchronous FFI
   call from the UI isolate freezes frame production for its full duration.
3. **No push path without a Dart API DL shim.** Poll-only demo: the "core"
   produced an event every 2 ms; a 50 ms poller saw 8 distinct values for
   **191** events (183 invisible) with **51 ms** first-detection latency.
   Doing this properly means a Dart native-port callback queue (C shim, Dart
   API DL, lifecycle handling) — a third language boundary.
4. **Isolate ergonomics:** closures capturing a `DynamicLibrary` cannot be sent
   to `Isolate.run` (hit during the spike); a worker isolate + port
   architecture is mandatory for any real use, so the "simple" branch is not
   simple once UI-safety and push are requirements.
5. **Crash coupling:** a Go panic in a cgo export aborts the host process, and
   a Dart crash kills the core; with UDS the daemon survives and the UI
   reconnects.

## Decision to be made (recommendation)

Adopt **UDS + gRPC (single transport) for the Flutter ↔ Go boundary on Linux**:

1. Freeze the IPC contract as a proto file (a product `phonebridge.v1`
   extension — the spike schema `phonebridge.spike.localipc.v1` is throwaway
   and must not be promoted).
2. Keep the daemon process model proven here: foreground + systemd
   `Type=simple`, `Restart=on-failure`; socket dir 0700 / socket 0600 under
   `XDG_RUNTIME_DIR`; SIGTERM → GracefulStop.
3. Port the security-gate design to the product: SO_PEERCRED
   `TransportCredentials` + uid allowlist + constant-time token, rejected
   pre-auth.
4. Treat FFI as a documented rejected alternative; revisit only for a future
   hot path that protos cannot express.
5. Perf guidance: stream events are never the cadence bottleneck; jitter under
   load (11–32 ms) is the number to watch if an event class ever needs strict
   isochrony; keep bulk payload chunks well under 64 KiB per message.

## Artifacts & how to re-run

```
# one-time toolchain (staged at ~/.local/spike01-toolchain)
export PATH=$HOME/.local/spike01-toolchain/go/bin:$HOME/.local/spike01-toolchain/bin:$HOME/.pub-cache/bin:$PATH
export GOTOOLCHAIN=local

make -C spikes/01-flutter-go-ipc-linux proto     # buf lint + generate (spike-local)
make -C spikes/01-flutter-go-ipc-linux build     # spiked + libphonebridge_spike.so
make -C spikes/01-flutter-go-ipc-linux test      # functional suites (17 tests)
make -C spikes/01-flutter-go-ipc-linux perf      # perf suites + JSON evidence
cd spikes/01-flutter-go-ipc-linux/harness && flutter analyze && flutter test
```

- JSON evidence: `results/uds_grpc.json`, `results/ffi.json` (percentiles,
  sweeps, lifecycle/hazard sections; rewritten on every perf run).
- The spike is fully isolated from product code; nothing in `core/`, `ui/`,
  `proto/phonebridge/v1`, or CI codegen paths was modified. **Not committed /
  not pushed** — review, then commit as a spike artifact.