#!/usr/bin/env bash
set -euo pipefail

export WAYLAND_DISPLAY=wayland-1
export XDG_RUNTIME_DIR=/run/user/1000

PROBE="./06-cosmic-clipboard/bin/data_control_probe"
FOCUS="./06-cosmic-clipboard/bin/focus_helper"
LOG_DIR="./06-cosmic-clipboard/results"
mkdir -p "$LOG_DIR"
OUT="$LOG_DIR/section_b_focus_independence.log"
rm -f "$OUT"

exec > >(tee -a "$OUT") 2>&1

echo "=== SECTION B: FOCUS INDEPENDENCE VALIDATION ==="
echo "Compositor: cosmic-comp (Smithay)"
echo "Wayland Socket: $XDG_RUNTIME_DIR/$WAYLAND_DISPLAY"
echo "Date: $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
echo

# B.1: Windowless Client with Zero Windows Open
echo "--- B.1: Windowless Background Client (No Windows on Desktop) ---"
$PROBE --mode write --text "WINDOWLESS_PAYLOAD" > /tmp/b1_w.log 2>&1 &
B1_W_PID=$!
sleep 0.5
B1_READ=$($PROBE --mode read_once)
echo "$B1_READ"
if echo "$B1_READ" | grep -q "WINDOWLESS_PAYLOAD"; then
    echo "PASS: Windowless background client successfully wrote and read clipboard"
else
    echo "FAIL: Windowless read/write failed"
    exit 1
fi
kill -9 $B1_W_PID 2>/dev/null || true
echo

# B.2: Unfocused Client While Independent GUI App Has Focus
echo "--- B.2: Unfocused Background Client While Another App is Focused ---"
# Launch focus_helper Window A
$FOCUS "Window-A" > /tmp/b2_win_a.log 2>&1 &
WIN_A_PID=$!
sleep 1.0

# Verify Window A is running
if ps -p $WIN_A_PID > /dev/null; then
    echo "Window A mapped (PID $WIN_A_PID)"
fi

# While Window A is active, background daemon writes clipboard
echo "Background daemon writing while Window A is active..."
$PROBE --mode write --text "UNFOCUSED_CLIENT_WRITE_TEST" > /tmp/b2_w.log 2>&1 &
B2_W_PID=$!
sleep 0.5

# Background daemon reads clipboard
echo "Background daemon reading while Window A is active..."
B2_READ=$($PROBE --mode read_once)
echo "$B2_READ"
if echo "$B2_READ" | grep -q "UNFOCUSED_CLIENT_WRITE_TEST"; then
    echo "PASS: Background daemon successfully read/wrote while independent window active"
else
    echo "FAIL: Unfocused read/write failed"
    exit 1
fi
kill -9 $B2_W_PID 2>/dev/null || true
echo

# B.3: Unfocused Client While Native COSMIC App with Editable Field Has Focus
echo "--- B.3: Unfocused Background Client While COSMIC Native Editor is Focused ---"
cosmic-edit > /tmp/b3_edit.log 2>&1 &
EDIT_PID=$!
sleep 2.0

if ps -p $EDIT_PID > /dev/null; then
    echo "cosmic-edit launched (PID $EDIT_PID)"
fi

# Background daemon writes while cosmic-edit is focused
$PROBE --mode write --text "COSMIC_EDIT_FOCUSED_PAYLOAD" > /tmp/b3_w.log 2>&1 &
B3_W_PID=$!
sleep 0.5

B3_READ=$($PROBE --mode read_once)
echo "$B3_READ"
if echo "$B3_READ" | grep -q "COSMIC_EDIT_FOCUSED_PAYLOAD"; then
    echo "PASS: Background daemon read/wrote clipboard while cosmic-edit had focus"
else
    echo "FAIL: Failed during cosmic-edit focus"
    exit 1
fi
kill -9 $B3_W_PID 2>/dev/null || true
kill -9 $EDIT_PID 2>/dev/null || true
echo

# B.4: Dynamic Focus Switching While Background Client Holds Ownership
echo "--- B.4: Dynamic Focus Switching While Background Daemon Holds Ownership ---"
$PROBE --mode write --text "STABLE_ACROSS_FOCUS_SWITCH" > /tmp/b4_w.log 2>&1 &
B4_W_PID=$!
sleep 0.5

# Launch Window 1
$FOCUS "Focus-Switch-1" > /tmp/b4_w1.log 2>&1 &
SW1_PID=$!
sleep 1.0

# Read clipboard
R1=$($PROBE --mode read_once)
echo "Read 1 (during Window 1 active):"
echo "$R1"

# Launch Window 2 (triggers focus shift)
$FOCUS "Focus-Switch-2" > /tmp/b4_w2.log 2>&1 &
SW2_PID=$!
sleep 1.0

# Read clipboard
R2=$($PROBE --mode read_once)
echo "Read 2 (after focus switch to Window 2):"
echo "$R2"

# Check if writer was cancelled
if grep -q "EVENT=source_cancelled" /tmp/b4_w.log; then
    echo "FAIL: Writer was unexpectedly cancelled by focus change!"
    exit 1
else
    echo "PASS: Writer ownership remained intact across focus changes"
fi

kill -9 $B4_W_PID $SW1_PID $SW2_PID $WIN_A_PID 2>/dev/null || true
echo "=== SECTION B COMPLETE ==="
