#!/usr/bin/env bash
# Spike 05 — companion-IME clipboard exemption harness.
#
# Answers the spike's decisive architectural question: can an input method we
# own read and observe the clipboard while ANOTHER app owns the window, and what
# does that cost / how durable is it?
#
#   ./spike05-ime.sh setup     — install + enable + select the probe IME
#   ./spike05-ime.sh shown     — IME visible over a third-party app: read + change
#   ./spike05-ime.sh hidden    — IME selected but NOT shown: read + change
#   ./spike05-ime.sh loop      — naive echo sync: prove it oscillates unboundedly
#   ./spike05-ime.sh suppress  — echo sync + digest suppression: prove it bounds
#   ./spike05-ime.sh lifecycle — process residency / restart after kill
#   ./spike05-ime.sh cost      — idle + event-driven CPU cost
#   ./spike05-ime.sh all       — everything, one evidence file
#   ./spike05-ime.sh restore   — restore the user's original IME
#
# Nothing here touches production code. The IME is a separate APK with a
# separate applicationId (dev.phonebridge.spike05ime).
set -uo pipefail

S5_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${OUT:-$S5_DIR/results}"
IME_PKG="dev.phonebridge.spike05ime"
IME_COMP="${IME_PKG}/.ProbeImeService"
IME_CTL="${IME_PKG}/.ImeControlActivity"
IME_APK="$S5_DIR/android/ime/build/outputs/apk/debug/ime-debug.apk"
SETTER="dev.phonebridge.spike05setter"
SETTER_ACT="$SETTER/.SetterActivity"
TAG="Spike05Ime"
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
reset_log() { "${ADB[@]}" logcat -c >/dev/null 2>&1; sleep 0.3; }
ime_log()  { "${ADB[@]}" logcat -d -s "$TAG" 2>/dev/null; }
set_log()  { "${ADB[@]}" logcat -d -s "$STAG" 2>/dev/null; }
ime_pid()  { "${ADB[@]}" shell pidof "$IME_PKG" 2>/dev/null | tr -d '\r'; }

# Select the probe IME (idempotent). Re-selecting after a force-stop is required
# on this build: force-stop also clears the default-IME setting.
select_ime() {
  "${ADB[@]}" shell ime enable "$IME_COMP" >/dev/null 2>&1
  "${ADB[@]}" shell ime set "$IME_COMP" >/dev/null 2>&1
  sleep 1.5
}

# Have the peer app write a payload WITHOUT showing an editable field, so the
# IME stays in the "selected but not shown" state throughout.
peer_write() { # peer_write <label> <text>
  "${ADB[@]}" shell am start -n "$SETTER_ACT" \
    --es op write --es label "$1" --es text "$2" >/dev/null 2>&1
}

# Focus the peer's editable field, which makes the selected IME show over it.
peer_edit() { # peer_edit <label>
  "${ADB[@]}" shell am force-stop "$SETTER" >/dev/null 2>&1
  sleep 0.4
  "${ADB[@]}" shell am start -W -n "$SETTER_ACT" --es op edit --es label "$1" >/dev/null 2>&1
}

peer_hide() { # peer_hide <label>
  "${ADB[@]}" shell am start -n "$SETTER_ACT" --es op hide --es label "$1" >/dev/null 2>&1
}

set_periodic() { # set_periodic <true|false> [period_ms]
  local on="$1" ms="${2:-2000}"
  "${ADB[@]}" shell am start -n "$IME_CTL" --ez periodic "$on" --ei period_ms "$ms" >/dev/null 2>&1
  sleep 0.5
}

# ------------------------------------------------------------------ scenarios

set_ctl() { # set_ctl <extra am args...>
  "${ADB[@]}" shell am start -n "$IME_CTL" "$@" >/dev/null 2>&1
  sleep 0.5
}

scenario_loop() {
  # Naive sync: every clipboard change is written straight back. Measures the
  # oscillation the production design MUST avoid, and proves it cannot be
  # stopped from app-level flags (it starves the main looper).
  log "loop: naive echo sync — expected runaway oscillation"
  select_ime
  set_periodic false
  set_ctl --ez echo true --ez suppress false
  peer_edit ime-loop-focus
  sleep 3
  peer_hide ime-loop-hide
  sleep 2
  reset_log
  echo "one external write at T+0..."
  peer_write ime-loop-seed "LOOP-SEED-4001"
  sleep 12
  local n
  n="$(ime_log | grep -c 'op=ime_change')"
  echo "changes in ~12s: $n"
  echo "writes: $(ime_log | grep -c 'op=ime_write')  suppressed: $(ime_log | grep -c 'ime_echo_suppressed')"
  echo "--- attempt to disable echo via the control activity (will likely NOT stop it) ---"
  set_ctl --ez echo false
  sleep 4
  echo "changes after disable attempt: $(ime_log | grep -c 'op=ime_change')"
  echo "--- tail of the flood (counts only; full text would roll the buffer) ---"
  ime_log | grep -E 'op=ime_change|op=ime_write|ime_echo_suppressed' | tail -12
  echo "--- halting via force-stop (the only thing that stops it) ---"
  "${ADB[@]}" shell am force-stop "$IME_PKG" >/dev/null 2>&1
  sleep 1
  echo "pid after force-stop: '$(ime_pid)'  (force-stop also clears the default IME)"
}

scenario_suppress() {
  # Same naive sync PLUS the candidate loop-prevention rule (skip a change whose
  # digest equals the payload this process last wrote). Must terminate.
  log "suppress: echo sync + digest suppression — expected bounded"
  select_ime
  set_periodic false
  set_ctl --ez echo true --ez suppress true
  peer_edit ime-suppress-focus
  sleep 3
  peer_hide ime-suppress-hide
  sleep 2
  reset_log
  echo "one external write at T+0..."
  peer_write ime-suppress-seed "SUPPRESS-SEED-5001"
  sleep 10
  echo "changes: $(ime_log | grep -c 'op=ime_change')  writes: $(ime_log | grep -c 'op=ime_write')  suppressed: $(ime_log | grep -c 'ime_echo_suppressed')"
  echo "--- evidence ---"
  ime_log | grep -E 'op=ime_change|op=ime_write|ime_echo_suppressed|op=ime_read reason=on_change'
  set_ctl --ez echo false --ez suppress false
}

scenario_setup() {
  log "setup: install + enable + select probe IME"
  "${ADB[@]}" push "$IME_APK" /data/local/tmp/spike05ime.apk >/dev/null
  "${ADB[@]}" shell pm install -r -t /data/local/tmp/spike05ime.apk
  "${ADB[@]}" shell ime enable "$IME_COMP"
  echo "enabled IMEs:"; "${ADB[@]}" shell ime list -s
}

scenario_shown() {
  log "shown: IME visible over a third-party app (read + change listener)"
  select_ime
  set_periodic true 2000
  reset_log
  # Show the IME over the peer app, then write from the peer while it is shown.
  peer_edit ime-shown-focus
  sleep 4
  peer_write ime-shown-A "SHOWN-SIGNAL-A-1001"
  sleep 3
  peer_write ime-shown-B "SHOWN-SIGNAL-B-1002"
  sleep 3
  echo "--- peer writes ---"; set_log | grep -E "op=write|op=edit_target"
  echo "--- ime ---"; ime_log | grep -E "RESULT|onStartInputView|onWindow"
}

scenario_hidden() {
  log "hidden: IME SELECTED but NOT shown (peer owns focus, no editable field)"
  select_ime
  # The production shape: event-driven only, no periodic polling.
  set_periodic false
  peer_edit ime-hidden-focus
  sleep 3
  peer_hide ime-hidden-hide
  sleep 3
  reset_log
  peer_write ime-hidden-A "HIDDEN-SIGNAL-A-2001"
  sleep 4
  peer_write ime-hidden-B "HIDDEN-SIGNAL-B-2002"
  sleep 4
  echo "--- peer writes ---"; set_log | grep -E "op=write"
  echo "--- ime (event-driven; no periodic) ---"; ime_log | grep -E "RESULT|onFinishInputView|onWindowHidden"
  echo "--- ime process ---"; ime_pid
}

scenario_lifecycle() {
  log "lifecycle: process residency after the IME is hidden"
  select_ime
  set_periodic true 2000
  peer_edit ime-life-focus
  sleep 4
  echo "T+0 shown pid=$(ime_pid)"
  peer_hide ime-life-hide
  for i in 1 2 3 4 5 6 7 8 9; do
    sleep 15
    echo "T+$((i*15))s pid=$(ime_pid)"
  done
  echo "--- ime lifecycle log ---"
  ime_log | grep -E "IME |RESULT op=ime_read" | tail -30
}

scenario_cost() {
  log "cost: idle + event-driven CPU (production shape)"
  select_ime
  set_periodic false
  sleep 3
  local pid; pid="$(ime_pid)"
  [ -n "$pid" ] || { echo "IME not resident; showing it once to start the process"; peer_edit ime-cost-focus; sleep 3; pid="$(ime_pid)"; }
  echo "ime pid=$pid"
  "${ADB[@]}" shell dumpsys batterystats --reset >/dev/null 2>&1
  local t0 t1
  t0="$("${ADB[@]}" shell cat "/proc/$pid/stat" | awk '{print $14+$15}')"
  reset_log
  echo "idle 60s with the IME selected but not shown..."
  sleep 60
  t1="$("${ADB[@]}" shell cat "/proc/$pid/stat" | awk '{print $14+$15}')"
  echo "jiffies before=$t0 after=$t1 delta=$((t1 - t0)) (100 Hz => ~$(( (t1 - t0) * 10 ))ms CPU over 60s)"
  echo "ime_read lines during idle (expected 0): $(ime_log | grep -c 'ime_read')"
  echo "--- rss ---"; "${ADB[@]}" shell dumpsys meminfo "$IME_PKG" 2>/dev/null | grep -E "TOTAL|TOTAL PSS" | head -3
}

scenario_restore() {
  log "restore: re-enable the user's original IME"
  local orig
  orig="$("${ADB[@]}" shell settings get secure default_input_method | tr -d '\r')"
  echo "current default: $orig"
  "${ADB[@]}" shell ime set com.google.android.inputmethod.latin/com.android.inputmethod.latin.LatinIME >/dev/null 2>&1
  sleep 1
  "${ADB[@]}" shell ime disable "$IME_COMP" >/dev/null 2>&1
  echo "default now: $("${ADB[@]}" shell settings get secure default_input_method | tr -d '\r')"
  echo "enabled IMEs:"; "${ADB[@]}" shell ime list -s
}

# ----------------------------------------------------------------------- main

STAMP="$(date +%Y%m%d-%H%M%S)"
run_one() { # run_one <name> <fn>
  local name="$1" fn="$2"
  local f="$OUT/ime-$name-$STAMP.log"
  echo
  echo "############ $name ############"
  "$fn" 2>&1 | tee "$f"
  echo "-> $f"
}

case "${1:-all}" in
  setup)     scenario_setup ;;
  shown)     run_one shown     scenario_shown ;;
  hidden)    run_one hidden    scenario_hidden ;;
  loop)      run_one loop      scenario_loop ;;
  suppress)  run_one suppress  scenario_suppress ;;
  lifecycle) run_one lifecycle scenario_lifecycle ;;
  cost)      run_one cost      scenario_cost ;;
  restore)   scenario_restore ;;
  all)
    run_one shown     scenario_shown
    run_one hidden    scenario_hidden
    run_one suppress  scenario_suppress
    run_one loop      scenario_loop
    run_one lifecycle scenario_lifecycle
    run_one cost      scenario_cost
    ;;
  *) echo "usage: $0 {setup|shown|hidden|loop|suppress|lifecycle|cost|all|restore}"; exit 2 ;;
esac
