#!/usr/bin/env bash
# Spike 03 host driver.
#
# Builds + installs the isolated prototype, drives scenarios on a connected
# device over adb, drives the MediaProjection consent dialog (including the
# Android 15 capture-scope spinner), and pulls the JSON evidence off the device.
#
# Usage: tools/spike03.sh <command> [args]
#   build | install | device | caps | baseline | session | lifecycle | matrix
#   screenoff | frhint | vbr | target34 | pull | logs | ui-dump | clean
#
# Env knobs: W H FPS BITRATE BITRATE_MODE DURATION CYCLES LABEL SCOPE SERIAL ADB
#            OUT MATRIX_CONFIGS
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ANDROID_DIR="$HERE/android"
OUT="${OUT:-$HERE/results}"
PKG=dev.phonebridge.spike03
ACT="$PKG/.MainActivity"
REMOTE_RESULTS="/sdcard/Android/data/$PKG/files/results"
APK="$ANDROID_DIR/app/build/outputs/apk/debug/app-debug.apk"
LOGCAT="$OUT/logcat.txt"
DRIVER_LOG="$OUT/driver.log"

ADB="${ADB:-adb}"
SERIAL_ARG=()
if [ -n "${SERIAL:-}" ]; then SERIAL_ARG=(-s "$SERIAL"); fi
# stdin is detached: adb forwards stdin to the device shell, which would otherwise
# eat the herestring driving the matrix loop.
adb_() { "$ADB" "${SERIAL_ARG[@]}" "$@" </dev/null; }

W="${W:-1080}"; H="${H:-2400}"; FPS="${FPS:-30}"; BITRATE="${BITRATE:-6000000}"
BITRATE_MODE="${BITRATE_MODE:-cbr}"; DURATION="${DURATION:-10}"; CYCLES="${CYCLES:-20}"
SCOPE="${SCOPE:-Entire screen}"

DEFAULT_MATRIX_CONFIGS="1080 2400 30 6000000 10
720 1600 30 3000000 10
1280 720 30 4000000 10
1920 1080 30 8000000 10
1920 1080 60 12000000 10
540 1200 30 1500000 10
1080 2400 60 12000000 10
2400 1080 30 8000000 10"

mkdir -p "$OUT"
log() { echo "[driver] $*" | tee -a "$DRIVER_LOG"; }

# ---------------------------------------------------------------- lifecycle

cmd_build() {
  ( cd "$ANDROID_DIR" && ./gradlew --no-daemon :app:assembleDebug "$@" ) || return 1
  ls -l "$APK"
}

cmd_build_target34() {
  ( cd "$ANDROID_DIR" && ./gradlew --no-daemon :app:assembleDebug -PtargetSdkOverride=34 ) || return 1
  log "built variant with targetSdk=34 ($APK)"
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
    adb_ shell wm density | tr -d '\r'
    adb_ shell dumpsys display 2>/dev/null | grep -E "mDisplayId=0|fps|refreshRate" | head -6 | tr -d '\r'
    echo "== battery =="
    adb_ shell dumpsys battery | grep -Ei "level|temperature|voltage|status" | tr -d '\r'
    echo "== thermal =="
    adb_ shell dumpsys thermalservice 2>/dev/null | head -25 | tr -d '\r'
    echo "== installed package =="
    adb_ shell dumpsys package "$PKG" | grep -E "targetSdk|versionName" | head -4 | tr -d '\r'
  } | tee "$OUT/device.txt"
}

start_logcat() {
  stop_logcat
  adb_ logcat -c >/dev/null 2>&1 || true
  : > "$LOGCAT"
  adb_ logcat -v brief -s Spike03:I > "$LOGCAT" 2>&1 &
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
  adb_ shell uiautomator dump --compressed /sdcard/spike03-ui.xml >/dev/null 2>&1 || return 1
  adb_ shell ls /sdcard/spike03-ui.xml >/dev/null 2>&1 || return 1
  adb_ exec-out cat /sdcard/spike03-ui.xml > "$OUT/.ui-current.xml" 2>/dev/null || return 1
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
    if grep -aq "SPIKE03_CONSENT_RESULT" "$LOGCAT" 2>/dev/null; then return 0; fi
    if dump_ui consent; then
      # Android 15: the dialog defaults to a single-app capture scope; pick the
      # requested scope explicitly before pressing Start.
      if grep -q "screen_share_mode_spinner" "$OUT/.ui-current.xml" && [ -n "$SCOPE" ]; then
        if tap_match "screen_share_mode_spinner" "$OUT/.ui-current.xml"; then
          sleep 1
          dump_ui scope-options
          if ! tap_match "$SCOPE" "$OUT/.ui-current.xml"; then
            log "scope option '$SCOPE' not offered; keeping device default"
          fi
          sleep 1
          dump_ui after-scope
        fi
      fi
      tap_match "button1|^\s*(Start|Allow|Share|Cast)\s*$" "$OUT/.ui-current.xml" || true
      sleep 1.5
      if grep -aq "SPIKE03_CONSENT_RESULT" "$LOGCAT" 2>/dev/null; then return 0; fi
    fi
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
    # media.hwcodec = vendor hardware codec service (where c2.qti.* actually runs)
    adb_ shell dumpsys cpuinfo 2>/dev/null | grep -aiE "spike03|media\.hwcodec|media\.codec|media\.swcodec|surfaceflinger|system_server|Load:" | tr -d '\r'
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
  log "scenario=$scenario label=$label ${w}x${h}@${fps} ${br}bps ${secs}s ${extra[*]:-}"
  start_logcat
  host_snapshot "before-$label"
  adb_ shell am force-stop "$PKG" >/dev/null 2>&1 || true
  sleep 0.5
  adb_ shell am start -n "$ACT" \
    --es scenario "$scenario" --es label "$label" \
    --ei width "$w" --ei height "$h" --ei fps "$fps" --ei bitrate "$br" \
    --es bitrateMode "$BITRATE_MODE" --ei seconds "$secs" "${extra[@]}" >/dev/null 2>&1
  if ! accept_consent 25; then
    log "WARNING: consent not accepted for $label (see $LOGCAT)"
  fi
  sleep 3
  host_snapshot "during-$label"
  if wait_marker "SPIKE03_DONE" $((secs + 150)); then
    log "done: $(grep -a SPIKE03_DONE "$LOGCAT" | tail -1)"
  else
    log "ERROR: no SPIKE03_DONE marker for $label within timeout"
    tail -25 "$LOGCAT" | tee -a "$DRIVER_LOG"
  fi
  host_snapshot "after-$label"
  stop_logcat
  cp "$LOGCAT" "$OUT/logcat-$label.txt" 2>/dev/null || true
  pull_results
}

cmd_caps() {
  log "enumerating encoders (no consent required)"
  start_logcat
  adb_ shell am force-stop "$PKG" >/dev/null 2>&1 || true
  adb_ shell am start -n "$ACT" --es scenario caps --es label caps >/dev/null 2>&1
  if wait_marker "SPIKE03_DONE" 90; then
    log "done: $(grep -a SPIKE03_DONE "$LOGCAT" | tail -1)"
  else
    log "ERROR: caps did not finish"
    tail -25 "$LOGCAT" | tee -a "$DRIVER_LOG"
  fi
  stop_logcat
  cp "$LOGCAT" "$OUT/logcat-caps.txt" 2>/dev/null || true
  pull_results
}

cmd_baseline() {
  local label="baseline-${W}x${H}"
  log "baseline: animated content, no capture, ${DURATION}s"
  start_logcat
  host_snapshot "before-$label"
  adb_ shell am force-stop "$PKG" >/dev/null 2>&1 || true
  adb_ shell am start -n "$ACT" --es scenario baseline --es label "$label" \
    --ei seconds "$DURATION" >/dev/null 2>&1
  if wait_marker "SPIKE03_DONE" $((DURATION + 60)); then
    log "done: $(grep -a SPIKE03_DONE "$LOGCAT" | tail -1)"
  else
    log "ERROR: baseline did not finish"
  fi
  host_snapshot "after-$label"
  stop_logcat
  cp "$LOGCAT" "$OUT/logcat-$label.txt" 2>/dev/null || true
  pull_results
}

cmd_session() {
  run_scenario session "${LABEL:-${W}x${H}}" "$W" "$H" "$FPS" "$BITRATE" "$DURATION" "$@"
}

cmd_vbr() {
  local prev="$BITRATE_MODE"
  BITRATE_MODE=vbr run_scenario session "vbr-${W}x${H}" "$W" "$H" "$FPS" "$BITRATE" "$DURATION" "$@"
  BITRATE_MODE="$prev"
}

cmd_lifecycle() {
  run_scenario lifecycle "${LABEL:-lifecycle-${W}x${H}}" "$W" "$H" "$FPS" "$BITRATE" "$DURATION" \
    --ei cycles "$CYCLES" --ez syncProbe true --ez secureProbe true --ez backgroundProbe true \
    --ez staticProbe true --ez bitrateProbe true "$@"
}

cmd_matrix() {
  local src="${MATRIX_CONFIGS:-$DEFAULT_MATRIX_CONFIGS}"
  local prev="$SCOPE"
  local line
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    # shellcheck disable=SC2086
    set -- $line
    run_scenario session "$1x$2@$3" "$1" "$2" "$3" "$4" "$5" --ez syncProbe true
    sleep 2
  done <<< "$src"
  SCOPE="$prev"
}

cmd_probe_frame_rate() {
  run_scenario session "frhint-${W}x${H}" "$W" "$H" "$FPS" "$BITRATE" "$DURATION" --ez frameRateHint true
}

cmd_probe_frame_rate_always() {
  run_scenario session "frhint-always-${W}x${H}" "$W" "$H" "$FPS" "$BITRATE" "$DURATION" \
    --ez frameRateHint true --ez frameRateHintAlways true
}

cmd_adapt() {
  # Isolated dynamic-bitrate probe: no FLAG_SECURE/static/background interference.
  run_scenario session "adapt-${W}x${H}" "$W" "$H" "$FPS" "$BITRATE" "$DURATION" \
    --ez bitrateProbe true --ez syncProbe true
}

cmd_landscape() {
  run_scenario session "landscape-${W}x${H}" "$W" "$H" "$FPS" "$BITRATE" "$DURATION" \
    --ez landscape true --ez syncProbe true
}

cmd_screenoff() {
  local label="screenoff-${W}x${H}"
  start_logcat
  host_snapshot "before-$label"
  adb_ shell am force-stop "$PKG" >/dev/null 2>&1 || true
  adb_ shell am start -n "$ACT" --es scenario session --es label "$label" \
    --ei width "$W" --ei height "$H" --ei fps "$FPS" --ei bitrate "$BITRATE" \
    --es bitrateMode "$BITRATE_MODE" --ei seconds 26 >/dev/null 2>&1
  accept_consent 25 || log "WARNING: consent not accepted"
  sleep 8
  log "switching screen off for 10s"
  adb_ shell input keyevent 26 >/dev/null 2>&1 || true
  host_snapshot "screenoff-$label"
  sleep 10
  log "waking screen"
  adb_ shell input keyevent 224 >/dev/null 2>&1 || true
  sleep 1
  adb_ shell wm dismiss-keyguard >/dev/null 2>&1 || true
  wait_marker "SPIKE03_DONE" 180 || log "ERROR: screenoff run did not finish"
  host_snapshot "after-$label"
  stop_logcat
  cp "$LOGCAT" "$OUT/logcat-$label.txt" 2>/dev/null || true
  pull_results
}

cmd_fgs_first() {
  # Negative probe: starting the mediaProjection FGS before consent (the Android 14 order).
  run_scenario session "fgsfirst-${W}x${H}" "$W" "$H" "$FPS" "$BITRATE" "$DURATION" --ez fgsBeforeConsent true
}

cmd_pull() { pull_results; log "results pulled to $OUT/device"; }
cmd_logs() { adb_ logcat -d -v brief -s Spike03:I | tail -80; }

cmd_consent_dump() {
  adb_ shell uiautomator dump --compressed /sdcard/spike03-ui.xml
  adb_ exec-out cat /sdcard/spike03-ui.xml > "$OUT/ui-dump-manual.xml"
  log "saved $OUT/ui-dump-manual.xml"
}

cmd_clean() {
  rm -rf "$ANDROID_DIR/app/build" "$ANDROID_DIR/build" "$ANDROID_DIR/.gradle" "$OUT/device"
  log "cleaned build + device results"
}

case "${1:-}" in
  build) shift; cmd_build "$@" ;;
  build34) cmd_build_target34 ;;
  install) cmd_install ;;
  device) cmd_device ;;
  caps) cmd_caps ;;
  baseline) cmd_baseline ;;
  session) shift; cmd_session "$@" ;;
  vbr) shift; cmd_vbr "$@" ;;
  lifecycle) shift; cmd_lifecycle "$@" ;;
  matrix) cmd_matrix ;;
  screenoff) shift; cmd_screenoff "$@" ;;
  frhint) shift; cmd_probe_frame_rate "$@" ;;
  frhint-always) shift; cmd_probe_frame_rate_always "$@" ;;
  adapt) shift; cmd_adapt "$@" ;;
  landscape) shift; cmd_landscape "$@" ;;
  fgsfirst) shift; cmd_fgs_first "$@" ;;
  pull) cmd_pull ;;
  logs) cmd_logs ;;
  ui-dump) cmd_consent_dump ;;
  clean) cmd_clean ;;
  *)
    echo "usage: $0 {build|build34|install|device|caps|baseline|session|vbr|lifecycle|matrix|screenoff|frhint|frhint-always|adapt|landscape|fgsfirst|pull|logs|ui-dump|clean}" >&2
    exit 2
    ;;
esac
