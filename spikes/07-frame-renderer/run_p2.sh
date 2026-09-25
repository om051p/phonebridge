#!/usr/bin/env bash
# Spike 07 / Phase 6 Slice 3 — P2 runner (local-IPC frame transport + Flutter decode/paint).
# Orchestrates: Go p2server (real localipc gRPC path) ⇄ dart p2_bench (real LocalIpcClient).
# Logs in results/p2/ (gitignored).
set -euo pipefail
cd "$(dirname "$0")"
ROOT="$PWD"
export PATH="${HOME}/go-sdk/go/bin:${PATH}"
mkdir -p results/p2

(cd go && go build -o bin/p2server ./p2server)

SOCK="$ROOT/results/p2/engine.sock"
TOK="$ROOT/results/p2/token"
FRAMES="$ROOT/results/frames"
[ -d "$FRAMES" ] || { echo "run run_p1.sh first (JPEG corpus)"; exit 1; }

(cd dart && flutter pub get >/dev/null)

round() {
  local phase="$1"; shift
  rm -f "$SOCK" "$TOK"
  echo "=== round $phase ==="
  go/bin/p2server -socket "$SOCK" -token "$TOK" "$@" > "results/p2/$phase.server.log" 2>&1 &
  local spid=$!
  for _ in $(seq 1 200); do [ -S "$SOCK" ] && [ -f "$TOK" ] && break; sleep 0.1; done
  set +e
  (cd dart && P2_PHASE="$phase" P2_SOCKET="$SOCK" P2_TOKEN="$TOK" \
    flutter test test/p2_bench_test.dart -r expanded) 2>&1 | tee "results/p2/$phase.client.log"
  set -e
  kill -TERM "$spid" 2>/dev/null || true
  wait "$spid" 2>/dev/null || true
  grep -h "RESULT\|INFO stall" "results/p2/$phase.server.log" "results/p2/$phase.client.log" 2>/dev/null || true
}

round flood   -mode flood -bytes 65536   -delay 25s -duration 25s
round flood1m -mode flood -bytes 1048576 -delay 25s -duration 25s
round flood17 -mode flood -bytes 1728000 -delay 25s -duration 25s
round decode  -mode paced -fps 30 -jpeg "$FRAMES" -delay 25s -duration 25s
round stall   -mode paced -fps 30 -jpeg "$FRAMES" -delay 25s -duration 25s

echo "=== P2 complete — see results/p2/*.log ==="
