#!/usr/bin/env bash
# Spike 04 host driver — Pion WebRTC Android ↔ Linux.
#
# Builds + installs the isolated prototype, starts the Linux receiver (signaling
# + media endpoint), drives capture scenarios over adb (including the MediaProjection
# consent dialog and the Android 15 capture-scope spinner), and collects evidence:
#   - device JSON (buckets, Go stats, framing forensics)  -> results/device/
#   - receiver logs + STATS lines + first-AU dump + .h264 -> results/
#
# Usage: tools/spike04.sh <command> [args]
#   build | install | device | receiver | caps | session | matrix | screenoff
#   soak | pull | logs | ui-dump | clean
#
# Env knobs: W H FPS BITRATE BITRATE_MODE THROTTLE DURATION LABEL SCOPE SERIAL ADB
#            OUT SIGNAL_HOST SIGNAL_PORT RECEIVER_BIN MATRIX_CONFIGS
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ANDROID_DIR="$HERE/android"
OUT="${OUT:-$HERE/results}"
PKG=dev.phonebridge.spike04
ACT="$PKG/.MainActivity"
REMOTE_RESULTS="/sdcard/Android/data/$PKG/files/results"
APK="$ANDROID_DIR/app/build/outputs/apk/debug/app-debug.apk"
RECEIVER_BIN="${RECEIVER_BIN:-$HERE/tools/spike04-receiver}"
RECEIVER_LOG="$OUT/receiver.log"
LOGCAT="$OUT/logcat.txt"
DRIVER_LOG="$OUT/driver.log"

ADB="${ADB:-adb}"
SERIAL_ARG=()
if [ -n "${SERIAL:-}" ]; then
  SERIAL_ARG=(-s "$SERIAL")
else
  # With more than one device/emulator attached every adb call fails with
  # "more than one device/emulator". Prefer an explicit $SERIAL, otherwise pin
  # the first *physical* device (an emulator may be attached for other work).
  mapfile -t _s4_devs < <("$ADB" devices 2>/dev/null | awk '$2=="device"{print $1}')
  if [ "${#_s4_devs[@]}" -gt 1 ]; then
    for _d in "${_s4_devs[@]}"; do
      case "$_d" in emulator-*|*:*) ;; *) SERIAL_ARG=(-s "$_d"); break ;; esac
    done
  fi
fi
# stdin is detached: adb forwards stdin to the device shell, which would otherwise
# eat the herestring driving the matrix loop (Spike 03 lesson).
adb_() { "$ADB" "${SERIAL_ARG[@]}" "$@" </dev/null; }
# Report the target so evidence files are attributable to a specific device.
S4_TARGET=$([ "${#SERIAL_ARG[@]}" -gt 0 ] && echo "${SERIAL_ARG[1]}" || echo "$( "$ADB" devices 2>/dev/null | awk '$2=="device"{print $1; exit}')")

W="${W:-720}"; H="${H:-1600}"; FPS="${FPS:-30}"; BITRATE="${BITRATE:-2500000}"
BITRATE_MODE="${BITRATE_MODE:-cbr}"; THROTTLE="${THROTTLE:-encoder}"
DURATION="${DURATION:-20}"; SIGNAL_PORT="${SIGNAL_PORT:-7804}"
SCOPE="${SCOPE:-Entire screen}"
# Encoder GOP retuning (KEY_FRAME_RATE * KEY_I_FRAME_INTERVAL = GOP access units)
# and the GOP-tail keep count. Defaults reproduce the baseline configuration.
FRAME_RATE="${FRAME_RATE:-$FPS}"; KEY_INTERVAL="${KEY_INTERVAL:-2}"
KEEP_FRAMES="${KEEP_FRAMES:-15}"
# Go-side burst shaper (0 = unshaped) and parameter-set re-injection.
SHAPE_KBPS="${SHAPE_KBPS:-0}"; SHAPE_BURST="${SHAPE_BURST:-3000}"
PSI_REINJECT="${PSI_REINJECT:-true}"
GO_EXTRA="--ei shapeKbps $SHAPE_KBPS --ei shapeBurstK $SHAPE_BURST --ez psiReinject $PSI_REINJECT"

mkdir -p "$OUT"
log() { echo "[driver] $*" | tee -a "$DRIVER_LOG"; }

# ------------------------------------------------------------- signaling host

# The app's network_security_config.xml allows cleartext only for these literals.
# If the signaling host is not listed, Android rejects the POST with
# "Cleartext HTTP traffic to <host> not permitted" and the run silently produces
# zero RTP. Keep this list in sync with android/app/src/main/res/xml/.
CLEARTEXT_ALLOWED="192.168.0.236 192.168.0.125 10.0.2.2 127.0.0.1 localhost"

assert_cleartext_allowed() {
  local h=$1 ok=1 a
  for a in $CLEARTEXT_ALLOWED; do [ "$a" = "$h" ] && ok=0; done
  if [ $ok -ne 0 ]; then
    # stderr on purpose: this function is called from a $(...) capture.
    printf '[driver] ERROR: signaling host %s is not in the app cleartext allowlist.\n' "$h" >&2
    printf '[driver]        Add <domain>%s</domain> to spike_network_security_config.xml,\n' "$h" >&2
    printf '[driver]        rebuild, and add it to CLEARTEXT_ALLOWED here.\n' >&2
    printf '[driver]        Aborting before a silent 0-packet run.\n' >&2
    return 1
  fi
  return 0
}

# Resolve the signaling host and refuse to start a run that Android would reject.
# Emits only the host on stdout (safe to $(...)-capture); all diagnostics on stderr.
resolve_sig() {
  local h
  h=$(detect_signal_host || true)
  if [ -z "$h" ]; then
    printf '[driver] ERROR: no signaling host (set SIGNAL_HOST)\n' >&2
    return 1
  fi
  assert_cleartext_allowed "$h" >&2 || return 1
  printf '%s\n' "$h"
}

detect_signal_host() {
  # Prefer an explicit host; else pick the host IP on the phone's /24.
  if [ -n "${SIGNAL_HOST:-}" ]; then echo "$SIGNAL_HOST"; return 0; fi
  local phone_ip host_ip
  phone_ip=$(adb_ shell ip -f inet addr show wlan0 2>/dev/null \
    | awk '/inet /{print $2; exit}' | cut -d/ -f1 | tr -d '\r')
  [ -z "$phone_ip" ] && { echo ""; return 1; }
  local prefix
  prefix=$(echo "$phone_ip" | cut -d. -f1-3)
  host_ip=$(ip -4 -o addr show 2>/dev/null | awk -v p="$prefix." '$4 ~ p {sub(/\/.*/, "", $4); print $4; exit}')
  [ -z "$host_ip" ] && { echo ""; return 1; }
  echo "$host_ip"
}

port_open() {
  (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null && { exec 3>&- 3<&-; return 0; }
  return 1
}

ensure_receiver() {
  # ensure_receiver [session_secs] — the receiver is given a bounded lifetime
  # (session + 120s slack for consent/setup) so it prints its STATS line and
  # exits on its own instead of running until the driver is killed.
  local secs="${1:-0}" host dump h264 budget
  if port_open "$SIGNAL_PORT"; then
    # A receiver left over from an interrupted run would silently serve this run
    # with the previous log/artifacts (and its own budget already partly spent).
    # Terminate it and start a fresh one so every capture is attributable.
    local stale=""
    [ -f "$OUT/.receiver.pid" ] && stale=$(cat "$OUT/.receiver.pid" 2>/dev/null)
    if [ -n "$stale" ] && kill -0 "$stale" 2>/dev/null; then
      log "terminating stale receiver pid $stale from a previous run"
      kill "$stale" 2>/dev/null || true
      sleep 1.5
    fi
    if port_open "$SIGNAL_PORT"; then
      # A killed driver leaves the receiver orphaned; the pid file may be gone,
      # so fall back to whoever actually holds the port.
      stale=$(ss -ltnp 2>/dev/null | grep ":$SIGNAL_PORT " \
        | grep -o 'pid=[0-9]*' | head -1 | cut -d= -f2)
      if [ -n "$stale" ]; then
        log "terminating orphaned receiver pid $stale holding :$SIGNAL_PORT"
        kill "$stale" 2>/dev/null || true
        sleep 1.5
      fi
    fi
    rm -f "$OUT/.receiver.pid"
    if port_open "$SIGNAL_PORT"; then
      log "ERROR: port $SIGNAL_PORT still busy; refusing to reuse an unknown listener"
      return 1
    fi
  fi
  if [ ! -x "$RECEIVER_BIN" ]; then
    log "ERROR: receiver binary missing: $RECEIVER_BIN (run: make receiver)"
    return 1
  fi
  # secs=0 (interactive `receiver` command) means unbounded. For a scenario the
  # budget is only a safety net: consent can block for minutes, so the driver
  # stops the receiver explicitly once the session is done (SIGTERM flushes STATS).
  budget=0
  [ "$secs" -gt 0 ] 2>/dev/null && budget=$((secs + 900))
  mkdir -p "$OUT/receiver-run"
  dump="$OUT/receiver-run/first-au.bin"
  h264="$OUT/receiver-run/received.h264"
  rm -f "$dump" "$h264" "$OUT/receiver-run/aus.idx"
  # `exec` matters: without it the subshell survives as the job and $! is the
  # SUBSHELL's pid, so `kill $(cat .receiver.pid)` signals the subshell and
  # orphans the receiver -- the receiver then never flushes STATS and keeps
  # holding the port (observed as pid 179166 in the file vs 179167 listening).
  ( cd "$HERE" && exec "$RECEIVER_BIN" -mode serve -http ":$SIGNAL_PORT" -duration "$budget" \
      -dump "$dump" -h264 "$h264" -auidx "$OUT/receiver-run/aus.idx" ) \
      >"$RECEIVER_LOG" 2>&1 &
  echo $! > "$OUT/.receiver.pid"
  sleep 1
  if port_open "$SIGNAL_PORT"; then
    log "receiver started (pid $(cat "$OUT/.receiver.pid"), budget ${budget}s, log $RECEIVER_LOG)"
    return 0
  fi
  log "ERROR: receiver did not start; log:"
  tail -5 "$RECEIVER_LOG" | tee -a "$DRIVER_LOG"
  return 1
}

stop_receiver() {
  local pid=""
  [ -f "$OUT/.receiver.pid" ] && pid=$(cat "$OUT/.receiver.pid" 2>/dev/null)
  if [ -z "$pid" ]; then
    pid=$(ss -ltnp 2>/dev/null | grep ":$SIGNAL_PORT " \
      | grep -o 'pid=[0-9]*' | head -1 | cut -d= -f2)
  fi
  [ -z "$pid" ] && return 0
  # SIGTERM is what makes the receiver flush its final STATS line.
  kill "$pid" 2>/dev/null || true
  local i=0
  while [ "$i" -lt 20 ]; do
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.25
    i=$((i + 1))
  done
  if kill -0 "$pid" 2>/dev/null; then
    log "receiver pid $pid ignored SIGTERM; sending SIGKILL"
    kill -9 "$pid" 2>/dev/null || true
    sleep 0.5
  fi
  rm -f "$OUT/.receiver.pid"
}

# Block until the receiver has printed STATS and exited (bounded).
await_receiver_stats() {
  local timeout_s="${1:-180}" label="$2" i=0
  while [ "$i" -lt $((timeout_s * 2)) ]; do
    if grep -aq '^STATS ' "$RECEIVER_LOG" 2>/dev/null; then
      log "receiver STATS captured"
      receiver_stats "$label"
      return 0
    fi
    if [ -f "$OUT/.receiver.pid" ] && ! kill -0 "$(cat "$OUT/.receiver.pid")" 2>/dev/null; then
      # The final STATS line may still be in flight when the process disappears;
      # give the log a moment before declaring it missing.
      sleep 1
      if grep -aq '^STATS ' "$RECEIVER_LOG" 2>/dev/null; then
        log "receiver STATS captured"
        receiver_stats "$label"
        return 0
      fi
      log "receiver exited without a STATS line"
      receiver_stats "$label"
      return 1
    fi
    sleep 0.5
    i=$((i + 1))
  done
  log "WARN: receiver produced no STATS within ${timeout_s}s; stopping it"
  stop_receiver
  receiver_stats
  return 1
}

receiver_stats() {
  # Keep the STATS line + last health lines with the run evidence. The label
  # argument also writes a run-specific copy: a single fixed filename meant a
  # later run (e.g. the soak) silently overwrote earlier runs' receiver stats.
  local label="$1"
  if [ -f "$RECEIVER_LOG" ]; then
    grep -a "^STATS " "$RECEIVER_LOG" | tail -1 > "$OUT/receiver-stats.txt" 2>/dev/null || true
    tail -30 "$RECEIVER_LOG" > "$OUT/receiver-tail.txt" 2>/dev/null || true
    if [ -n "$label" ]; then
      cp "$OUT/receiver-stats.txt" "$OUT/receiver-stats-$label.txt" 2>/dev/null || true
      grep -aE " t=[0-9]+s packets=" "$RECEIVER_LOG" > "$OUT/receiver-health-$label.txt" 2>/dev/null || true
      cp "$RECEIVER_LOG" "$OUT/receiver-$label.log" 2>/dev/null || true
    fi
  fi
}

# Snapshot the receiver artifacts under a label-specific name so the throttling
# comparison keeps every strategy's exact byte stream (reproducible evidence).
snapshot_capture() {
  local label="$1"
  mkdir -p "$OUT/captures"
  [ -f "$OUT/receiver-run/received.h264" ] && \
    cp "$OUT/receiver-run/received.h264" "$OUT/captures/$label.h264" 2>/dev/null || true
  [ -f "$OUT/receiver-run/aus.idx" ] && \
    cp "$OUT/receiver-run/aus.idx" "$OUT/captures/$label.idx" 2>/dev/null || true
  if [ -f "$OUT/captures/$label.h264" ]; then
    log "capture saved: $OUT/captures/$label.h264 ($(stat -c%s "$OUT/captures/$label.h264") bytes)"
  fi
}

# ------------------------------------------------------------------ lifecycle

cmd_build() {
  ( cd "$ANDROID_DIR" && ./gradlew --no-daemon :app:assembleDebug "$@" ) || return 1
  ls -l "$APK"
}

cmd_install() {
  log "device display: $(adb_ shell wm size | tr -d '\r')"
  adb_ install -r -t "$APK" || return 1
  adb_ shell pm grant "$PKG" android.permission.POST_NOTIFICATIONS 2>/dev/null || true
  adb_ shell am force-stop "$PKG"
}

cmd_device() {
  {
    echo "== getprop =="
    for p in ro.product.model ro.product.manufacturer ro.build.version.release \
             ro.build.version.sdk ro.product.cpu.abi ro.hardware ro.build.fingerprint \
             ro.build.version.security_patch ro.board.platform ro.soc.model ro.miui.build.region; do
      printf '%s=%s\n' "$p" "$(adb_ shell getprop "$p" | tr -d '\r')"
    done
    echo "== display =="
    adb_ shell wm size | tr -d '\r'
    echo "== phone wlan0 =="
    adb_ shell ip -f inet addr show wlan0 2>/dev/null | tr -d '\r'
    echo "== installed package =="
    adb_ shell dumpsys package "$PKG" | grep -E "targetSdk|versionName" | head -4 | tr -d '\r'
  } | tee "$OUT/device.txt"
  echo "== host candidate =="
  detect_signal_host || log "WARN: no host IP on the phone's subnet — set SIGNAL_HOST"
}

cmd_receiver() {
  ensure_receiver || return 1
  log "receiver logs at $RECEIVER_LOG — Ctrl-C the driver or run 'stop' via kill"
}

start_logcat() {
  stop_logcat
  adb_ logcat -c >/dev/null 2>&1 || true
  : > "$LOGCAT"
  adb_ logcat -v brief -s Spike04:I Spike04Go:I > "$LOGCAT" 2>&1 &
  echo $! > "$OUT/.logcat.pid"
  sleep 0.5
}

stop_logcat() {
  if [ -f "$OUT/.logcat.pid" ]; then
    kill "$(cat "$OUT/.logcat.pid")" 2>/dev/null || true
    rm -f "$OUT/.logcat.pid"
  fi
}

wait_marker() {
  local marker="$1" timeout_s="$2" i=0
  while [ "$i" -lt $((timeout_s * 2)) ]; do
    if grep -q "$marker" "$LOGCAT" 2>/dev/null; then return 0; fi
    sleep 0.5
    i=$((i + 1))
  done
  return 1
}

dump_ui() {
  local tag="$1"
  adb_ shell uiautomator dump --compressed /sdcard/spike04-ui.xml >/dev/null 2>&1 || return 1
  adb_ shell ls /sdcard/spike04-ui.xml >/dev/null 2>&1 || return 1
  adb_ exec-out cat /sdcard/spike04-ui.xml > "$OUT/.ui-current.xml" 2>/dev/null || return 1
  cp "$OUT/.ui-current.xml" "$OUT/ui-dump-$tag-$(date +%H%M%S).xml" 2>/dev/null || true
  return 0
}

# tap_match <regex> <dump-file>
tap_match() {
  local coords
  if coords=$(python3 "$HERE/tools/consent_tap.py" "$2" "$1" 2>/dev/null); then
    log "tap [$1] -> $coords"
    # shellcheck disable=SC2086
    adb_ shell input tap $coords
    return 0
  fi
  return 1
}

accept_consent() {
  local tries="${1:-25}" i=0
  while [ "$i" -lt "$tries" ]; do
    i=$((i + 1))
    if grep -aq "SPIKE04_CONSENT_RESULT" "$LOGCAT" 2>/dev/null; then return 0; fi
    # Drive the whole dialog from Python: it waits for a SETTLED dump, reads the
    # scope spinner's real value, retries selection (tap, then keyboard), and only
    # presses Start once the requested scope is confirmed. The old bash version
    # scraped coordinates from a mid-animation dump and silently mis-tapped.
    local cst=0
    python3 "$HERE/tools/consent_auto.py" --scope "$SCOPE" --attempts 6 >>"$DRIVER_LOG" 2>&1 || cst=$?
    [ "$cst" -ne 0 ] && log "consent automation returned $cst (attempt $i)"
    sleep 1.5
    if grep -aq "SPIKE04_CONSENT_RESULT" "$LOGCAT" 2>/dev/null; then return 0; fi
    sleep 1
  done
  return 1
}

host_snapshot() {
  local tag="$1"
  {
    echo "== load =="
    adb_ shell dumpsys cpuinfo 2>/dev/null | grep -a "^Load:" | tr -d '\r'
    echo "== cpuinfo: relevant processes =="
    adb_ shell dumpsys cpuinfo 2>/dev/null | grep -aiE "spike04|media\.hwcodec|media\.codec|media\.swcodec|surfaceflinger|system_server|Load:" | tr -d '\r'
    echo "== battery =="
    adb_ shell dumpsys battery 2>/dev/null | grep -Ei "level|temperature|voltage" | tr -d '\r'
    echo "== thermal status =="
    adb_ shell dumpsys thermalservice 2>/dev/null | grep -Ei "Temperature|status" | head -6 | tr -d '\r'
    echo "== media_projection sessions =="
    adb_ shell dumpsys media_projection 2>/dev/null | head -25 | tr -d '\r'
  } > "$OUT/host-$tag.txt" 2>/dev/null || true
}

pull_results() {
  adb_ pull "$REMOTE_RESULTS" "$OUT/device" >/dev/null 2>&1 || true
}

# ------------------------------------------------------------- scenario runs

# run_scenario <scenario> <label> <w> <h> <fps> <bitrate> <seconds> [extra am args...]
run_scenario() {
  local scenario="$1" label="$2" w="$3" h="$4" fps="$5" br="$6" secs="$7"
  shift 7
  local extra=("$@")
  local sig
  sig=$(resolve_sig) || { log "skipping $label (no usable signaling host)"; return 1; }
  local sig_url="http://$sig:$SIGNAL_PORT/offer"
  log "scenario=$scenario label=$label ${w}x${h}@${fps} ${br}bps ${secs}s throttle=$THROTTLE -> $sig_url ${extra[*]:-}"
  ensure_receiver "$secs" || return 1
  start_logcat
  host_snapshot "before-$label"
  adb_ shell am force-stop "$PKG" >/dev/null 2>&1 || true
  sleep 0.5
  adb_ shell am start -n "$ACT" \
    --es scenario "$scenario" --es label "$label" \
    --ei width "$w" --ei height "$h" --ei fps "$fps" --ei bitrate "$br" \
    --es bitrateMode "$BITRATE_MODE" --es throttle "$THROTTLE" \
    --ei frameRate "$FRAME_RATE" --ei keyInterval "$KEY_INTERVAL" \
    --ei keepFrames "$KEEP_FRAMES" \
    $GO_EXTRA \
    --es signaling "$sig_url" --ei seconds "$secs" "${extra[@]}" >/dev/null 2>&1
  if ! accept_consent 25; then
    log "WARNING: consent not accepted for $label (see $LOGCAT)"
  fi
  sleep 3
  host_snapshot "during-$label"
  if wait_marker "SPIKE04_DONE" $((secs + 150)); then
    log "done: $(grep -a SPIKE04_DONE "$LOGCAT" | tail -1)"
  else
    log "ERROR: no SPIKE04_DONE marker for $label within timeout"
    tail -25 "$LOGCAT" | tee -a "$DRIVER_LOG"
  fi
  host_snapshot "after-$label"
  stop_logcat
  cp "$LOGCAT" "$OUT/logcat-$label.txt" 2>/dev/null || true
  # Tell the receiver the session is over: SIGTERM makes it flush STATS and exit.
  stop_receiver
  await_receiver_stats 60 "$label" || true
  snapshot_capture "$label"
  pull_results
}

cmd_caps() {
  log "enumerating encoders (no consent/signaling required)"
  start_logcat
  adb_ shell am force-stop "$PKG" >/dev/null 2>&1 || true
  adb_ shell am start -n "$ACT" --es scenario caps --es label caps >/dev/null 2>&1
  if wait_marker "SPIKE04_DONE" 90; then
    log "done: $(grep -a SPIKE04_DONE "$LOGCAT" | tail -1)"
  else
    log "ERROR: caps did not finish"
    tail -25 "$LOGCAT" | tee -a "$DRIVER_LOG"
  fi
  stop_logcat
  cp "$LOGCAT" "$OUT/logcat-caps.txt" 2>/dev/null || true
  pull_results
}

cmd_session() {
  run_scenario session "${LABEL:-${W}x${H}t${THROTTLE}}" "$W" "$H" "$FPS" "$BITRATE" "$DURATION" "$@"
}

# MATRIX_CONFIGS: lines of "W H FPS BITRATE THROTTLE SECONDS"
cmd_matrix() {
  local src="${MATRIX_CONFIGS:-$DEFAULT_MATRIX_CONFIGS}"
  local line
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    # shellcheck disable=SC2086
    set -- $line
    THROTTLE="$5" run_scenario session "$1x$2@$3-$5" "$1" "$2" "$3" "$4" "$6"
    sleep 2
  done <<< "$src"
}

DEFAULT_MATRIX_CONFIGS="720 1600 30 2500000 encoder 15
720 1600 30 2500000 gop 15
1080 2400 30 6000000 encoder 15
1280 720 30 4000000 encoder 15"

cmd_screenoff() {
  # 0-fps window + wake burst through the live WebRTC path. LABEL differentiates
  # A/B trials (unshaped vs shaped): every artifact is keyed by it.
  local label="${LABEL:-screenoff-${W}x${H}}"
  # Exercise the real candidate path (GOP 30 + GOP-tail) so the 0-fps window and
  # the wake burst are measured through the same pipeline that would ship.
  THROTTLE="${THROTTLE:-tail}" run_scenario_probe "$label" 60 <<'PROBE'
  sleep 15
  log "switching screen off for 12s"
  adb_ shell input keyevent 26 >/dev/null 2>&1 || true
  host_snapshot "screenoff-$label-off"
  sleep 12
  log "waking screen"
  adb_ shell input keyevent 224 >/dev/null 2>&1 || true
  sleep 1
  adb_ shell wm dismiss-keyguard >/dev/null 2>&1 || true
  host_snapshot "screenoff-$label-on"
  sleep 20
PROBE
}

# run_scenario_probe <label> <secs> — session with a host-driven probe script on stdin
run_scenario_probe() {
  local label="$1" secs="$2"
  local sig probe_script
  # Read the probe script FIRST: anything that runs adb before this point (in
  # particular the consent automation) forwards its stdin to the device shell
  # and would consume the heredoc, leaving the probe body empty.
  probe_script=$(cat)
  sig=$(resolve_sig) || return 1
  ensure_receiver "$secs" || return 1
  start_logcat
  host_snapshot "before-$label"
  adb_ shell am force-stop "$PKG" >/dev/null 2>&1 || true
  sleep 0.5
  adb_ shell am start -n "$ACT" \
    --es scenario session --es label "$label" \
    --ei width "$W" --ei height "$H" --ei fps "$FPS" --ei bitrate "$BITRATE" \
    --es bitrateMode "$BITRATE_MODE" --es throttle "$THROTTLE" \
    --ei frameRate "$FRAME_RATE" --ei keyInterval "$KEY_INTERVAL" \
    --ei keepFrames "$KEEP_FRAMES" \
    $GO_EXTRA \
    --es signaling "http://$sig:$SIGNAL_PORT/offer" --ei seconds "$secs" >/dev/null 2>&1
  accept_consent 25 || log "WARNING: consent not accepted"
  sleep 3
  if [ -z "$probe_script" ]; then
    log "ERROR: probe script is empty for $label (stdin consumed before read?)"
    return 1
  fi
  log "running probe script for $label ($(wc -l <<<"$probe_script") lines)"
  eval "$probe_script"
  if wait_marker "SPIKE04_DONE" $((secs + 150)); then
    log "done: $(grep -a SPIKE04_DONE "$LOGCAT" | tail -1)"
  else
    log "ERROR: no SPIKE04_DONE marker for $label"
  fi
  host_snapshot "after-$label"
  stop_logcat
  cp "$LOGCAT" "$OUT/logcat-$label.txt" 2>/dev/null || true
  # Tell the receiver the session is over: SIGTERM makes it flush STATS and exit.
  stop_receiver
  await_receiver_stats 60 "$label" || true
  snapshot_capture "$label"
  pull_results
}

cmd_soak() {
  # Minimum 30-minute 720p30 soak, monitoring every 5 minutes.
  local label="${LABEL:-soak-720p30}" secs="${SOAK_SECS:-1800}" i=0
  local sig
  sig=$(resolve_sig) || return 1
  ensure_receiver "$secs" || return 1
  start_logcat
  host_snapshot "before-$label"
  adb_ shell am force-stop "$PKG" >/dev/null 2>&1 || true
  sleep 0.5
  # GOP 30 + GOP-tail keep 8 is the candidate under validation (GOP-RETUNE.md).
  adb_ shell am start -n "$ACT" \
    --es scenario session --es label "$label" \
    --ei width 720 --ei height 1600 --ei fps 30 --ei bitrate "$BITRATE" \
    --es bitrateMode "$BITRATE_MODE" --es throttle "$THROTTLE" \
    --ei frameRate "$FRAME_RATE" --ei keyInterval "$KEY_INTERVAL" \
    --ei keepFrames "$KEEP_FRAMES" \
    $GO_EXTRA \
    --es signaling "http://$sig:$SIGNAL_PORT/offer" --ei seconds "$secs" >/dev/null 2>&1
  accept_consent 25 || log "WARNING: consent not accepted"
  while [ "$i" -lt "$secs" ]; do
    sleep 300
    i=$((i + 300))
    host_snapshot "soak-${label}-t${i}s"
    if [ -f "$RECEIVER_LOG" ]; then
      grep -a "pc state:" "$RECEIVER_LOG" | tail -1 >> "$OUT/soak-$label-pcstate.txt" 2>/dev/null || true
      grep -a "t=" "$RECEIVER_LOG" | tail -1 | sed "s/^/t=${i}s /" >> "$OUT/soak-$label-health.txt" 2>/dev/null || true
    fi
    log "soak t=${i}s: $(grep -a 't=' "$RECEIVER_LOG" 2>/dev/null | tail -1)"
  done
  if wait_marker "SPIKE04_DONE" 300; then
    log "done: $(grep -a SPIKE04_DONE "$LOGCAT" | tail -1)"
  else
    log "WARNING: no SPIKE04_DONE after soak window"
  fi
  host_snapshot "after-$label"
  stop_logcat
  cp "$LOGCAT" "$OUT/logcat-$label.txt" 2>/dev/null || true
  # Tell the receiver the session is over: SIGTERM makes it flush STATS and exit.
  stop_receiver
  await_receiver_stats 60 "$label" || true
  snapshot_capture "$label"
  pull_results
}

cmd_pull() { pull_results; log "results pulled to $OUT/device"; }
cmd_logs() { adb_ logcat -d -v brief -s Spike04:I Spike04Go:I | tail -100; }

cmd_consent_dump() {
  adb_ shell uiautomator dump --compressed /sdcard/spike04-ui.xml
  adb_ exec-out cat /sdcard/spike04-ui.xml > "$OUT/ui-dump-manual.xml"
  log "saved $OUT/ui-dump-manual.xml"
}

cmd_clean() {
  rm -rf "$ANDROID_DIR/app/build" "$ANDROID_DIR/build" "$ANDROID_DIR/.gradle" "$OUT/device" "$OUT/receiver-run"
  log "cleaned build + device results"
}

case "${1:-}" in
  build) shift; cmd_build "$@" ;;
  install) cmd_install ;;
  device) cmd_device ;;
  receiver) cmd_receiver ;;
  caps) cmd_caps ;;
  session) shift; cmd_session "$@" ;;
  matrix) cmd_matrix ;;
  screenoff) shift; cmd_screenoff "$@" ;;
  soak) cmd_soak ;;
  pull) cmd_pull ;;
  logs) cmd_logs ;;
  ui-dump) cmd_consent_dump ;;
  clean) cmd_clean ;;
  *)
    echo "usage: $0 {build|install|device|receiver|caps|session|matrix|screenoff|soak|pull|logs|ui-dump|clean}" >&2
    exit 2
    ;;
esac
