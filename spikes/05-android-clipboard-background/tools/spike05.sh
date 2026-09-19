#!/usr/bin/env bash
# Spike 05 — Android clipboard behaviour harness.
#
# Drives the spike app through every app state, captures the RESULT lines from
# logcat, and writes a durable JSON/markdown evidence file under $OUT.
#
#   ./spike05.sh matrix      — foreground + background + FGS matrices
#   ./spike05.sh lifecycle   — focus-transition / listener reachability
#   ./spike05.sh autoclr    — clipboard auto-clear behaviour
#   ./spike05.sh listener   — listener-only reachability in each state
#   ./spike05.sh all        — everything above, one evidence file
#   ./spike05.sh pull       — pull any on-device evidence files
#
# Nothing here touches production code. The spike app is a separate APK with a
# separate applicationId (dev.phonebridge.spike05).
set -uo pipefail

S5_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${OUT:-$S5_DIR/results}"
PKG="dev.phonebridge.spike05"
SETTER="dev.phonebridge.spike05setter"
ACT="$PKG/.MainActivity"
SETTER_ACT="$SETTER/.SetterActivity"
TAG="Spike05"
STAG="Spike05Setter"

mkdir -p "$OUT"

SERIAL="${ANDROID_SERIAL:-}"
if [ -z "$SERIAL" ]; then
  SERIAL="$(adb devices | awk 'NR>1 && $2=="device" {print $1; exit}')"
fi
[ -n "$SERIAL" ] || { echo "ERROR: no authorized device. adb devices:"; adb devices -l; exit 1; }
ADB=(adb -s "$SERIAL")
echo "device: $SERIAL"

log() { printf '\n=== %s ===\n' "$*"; }

# ---------------------------------------------------------------- primitives

# Clear logcat and wait for the buffer to actually be empty.
reset_log() {
  "${ADB[@]}" logcat -c >/dev/null 2>&1
  sleep 0.3
}

# Dump everything this spike logged since the last reset.
dump_log() {
  "${ADB[@]}" logcat -d -s "$TAG" "$STAG" 2>/dev/null
}

# Launch the spike app in a given scenario and wait for it to settle.
# force-stop first: the activity is singleTask, so a relaunch would otherwise
# reuse the live instance and skip onCreate (the scenario would never re-run).
launch() { # launch <scenario> <label> [extra am args...]
  local scenario="$1" label="$2"; shift 2
  "${ADB[@]}" shell am force-stop "$PKG" >/dev/null 2>&1
  sleep 0.5
  "${ADB[@]}" shell am start -W -n "$ACT" \
    --es scenario "$scenario" --es label "$label" "$@" >/dev/null 2>&1
}

# Peer app writes to the clipboard as an independent owner.
peer_write() { # peer_write <label> <text>
  "${ADB[@]}" shell am start -W -n "$SETTER_ACT" \
    --es op write --es label "$1" --es text "$2" --ez auto_finish true >/dev/null 2>&1
}

peer_read() { # peer_read <label>
  "${ADB[@]}" shell am start -W -n "$SETTER_ACT" \
    --es op read --es label "$1" --ez auto_finish true >/dev/null 2>&1
}

# Bring the *launcher* (a third-party, non-spike app) to the front so that
# "background" is a genuine focus loss rather than a same-app transition.
goto_home() {
  "${ADB[@]}" shell input keyevent KEYCODE_HOME >/dev/null 2>&1
  sleep 1
}

foreground_spike() {
  "${ADB[@]}" shell am start -W -n "$ACT" >/dev/null 2>&1
  sleep 1
}

# ------------------------------------------------------------------ scenarios

scenario_matrix() {
  local payload="spike05-$(date +%s)"
  log "scenario: foreground matrix"
  reset_log
  launch matrix fg "$(printf -- '--es text %s' "$payload")" --ei iterations 3
  sleep 6
  dump_log

  log "scenario: background matrix (activity moved to back, no FGS)"
  reset_log
  launch background bg --es text "$payload" --ei iterations 3
  sleep 8
  dump_log
  foreground_spike

  log "scenario: foreground-service matrix (FGS, task backgrounded)"
  reset_log
  launch fgs fgs --es text "$payload" --ei iterations 3 --ez auto_background true
  sleep 9
  dump_log
  # Tear the service down so it does not linger between runs.
  "${ADB[@]}" shell am stopservice -n "$PKG/.ProbeService" >/dev/null 2>&1
  foreground_spike
}

scenario_listener() {
  log "scenario: listener reachability, foreground"
  reset_log
  launch listener lsn-fg
  sleep 2
  peer_write "peer-for-listener" "listener-payload-$(date +%s)"
  sleep 4
  dump_log

  log "scenario: listener reachability, background (no FGS)"
  reset_log
  launch background lsn-bg --ei iterations 0
  sleep 7
  peer_write "peer-bg-listener" "listener-bg-payload-$(date +%s)"
  sleep 4
  dump_log
  foreground_spike

  log "scenario: listener reachability, FGS backgrounded"
  reset_log
  launch fgs lsn-fgs --ei iterations 0 --ez auto_background true
  sleep 4
  peer_write "peer-fgs-listener" "listener-fgs-payload-$(date +%s)"
  sleep 4
  dump_log
  "${ADB[@]}" shell am stopservice -n "$PKG/.ProbeService" >/dev/null 2>&1
}

scenario_lifecycle() {
  log "scenario: focus transitions with a live listener"
  reset_log
  launch lifecycle lifecycle
  sleep 2
  # While the app is in the foreground, have the peer write.
  peer_write "peer-fg" "fg-payload-$(date +%s)"
  sleep 3
  # The app backgrounds itself mid-probe; write again to test background detection.
  peer_write "peer-bg" "bg-payload-$(date +%s)"
  sleep 6
  dump_log
  foreground_spike
}

scenario_autoclear() {
  # Android 13+ auto-clears the clipboard after a period; HyperOS may differ.
  # Write, then poll the *peer* app's read at intervals to see when content dies.
  local payload="autoclear-$(date +%s)"
  log "scenario: clipboard auto-clear (poll every 30s, up to 6 min)"
  reset_log
  foreground_spike
  peer_write "autoclear-seed" "$payload"
  sleep 2
  local waited=0
  while [ "$waited" -lt 360 ]; do
    local mark="$waited"
    peer_read "autoclear-t${mark}s"
    printf '  t=%ss: %s\n' "$waited" "$(dump_log | grep 'op=read' | tail -1 | sed 's/.*RESULT //')"
    sleep 30
    waited=$((waited + 30))
  done
  dump_log
}

# ------------------------------------------------------------------- evidence

emit_evidence() {
  local mode="$1"
  local stamp; stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  local raw="$OUT/raw-$mode-$stamp.log"
  cat > "$raw"

  python3 - "$raw" "$OUT" "$mode" "$stamp" <<'PY'
import json, re, sys, os, collections

raw, out, mode, stamp = sys.argv[1:5]
text = open(raw, encoding="utf-8", errors="replace").read()

def kv(line):
    # RESULT op=<x> label=<y> status=<z> k=v ...
    d = {}
    for m in re.finditer(r'(\w+)=("[^"]*"|\S+)', line):
        k, v = m.group(1), m.group(2).strip('"')
        d[k] = v
    return d

results = []
for line in text.splitlines():
    i = line.find("RESULT ")
    if i < 0:
        continue
    results.append(kv(line[i:]))

# Aggregate per (op, state): how often did it succeed?
agg = collections.defaultdict(lambda: collections.Counter())
for r in results:
    state = r.get("state") or r.get("scenario") or "n/a"
    agg[(r.get("op", "?"), state)][r.get("status", "?")] += 1

summary = {
    "mode": mode,
    "stamp": stamp,
    "raw_log": os.path.basename(raw),
    "result_lines": len(results),
    "matrix": {f"{op}|{state}": dict(c) for (op, state), c in sorted(agg.items())},
}
with open(os.path.join(out, f"summary-{mode}-{stamp}.json"), "w") as f:
    json.dump(summary, f, indent=2)

print(f"\n--- {mode}: {len(results)} result lines -> {out}/summary-{mode}-{stamp}.json")
for (op, state), c in sorted(agg.items()):
    tally = " ".join(f"{k}={v}" for k, v in sorted(c.items()))
    print(f"  {op:<20} {state:<20} {tally}")
PY
  echo "raw log: $raw"
}

# ----------------------------------------------------------------------- main

case "${1:-all}" in
  matrix)    scenario_matrix    | emit_evidence matrix ;;
  listener)  scenario_listener  | emit_evidence listener ;;
  lifecycle) scenario_lifecycle | emit_evidence lifecycle ;;
  autoclear) scenario_autoclear | emit_evidence autoclear ;;
  pull)      "${ADB[@]}" pull /sdcard/Android/data/"$PKG"/files/ "$OUT/device/" 2>/dev/null || echo "nothing to pull" ;;
  all)
    { scenario_matrix; scenario_listener; scenario_lifecycle; } | emit_evidence all
    ;;
  *) echo "usage: $0 {matrix|listener|lifecycle|autoclear|all|pull}"; exit 2 ;;
esac
