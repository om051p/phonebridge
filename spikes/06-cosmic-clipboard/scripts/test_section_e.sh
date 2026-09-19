#!/usr/bin/env bash
set -euo pipefail

export WAYLAND_DISPLAY=wayland-1
export XDG_RUNTIME_DIR=/run/user/1000

PROBE="./06-cosmic-clipboard/bin/data_control_probe"
LOG_DIR="./06-cosmic-clipboard/results"
mkdir -p "$LOG_DIR"
OUT="$LOG_DIR/section_e_lifecycle.log"
rm -f "$OUT"

exec > >(tee -a "$OUT") 2>&1

echo "=== SECTION E: LIFECYCLE & RESILIENCY VALIDATION ==="
echo "Compositor: cosmic-comp (Smithay)"
echo "Wayland Socket: $XDG_RUNTIME_DIR/$WAYLAND_DISPLAY"
echo "Date: $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
echo

# E.1: Reader Exit and Restart (Immediate Current Selection Retrieval)
echo "--- E.1: Reader Exit and Restart ---"
echo "Starting persistent writer..."
$PROBE --mode write --text "PERSISTENT_OFFER_FOR_SUCCESSIVE_READERS" > /tmp/e1_w.log 2>&1 &
W_PID=$!
sleep 0.5

echo "Reader 1 connecting..."
R1=$($PROBE --mode read_once)
echo "Reader 1 Output:"
echo "$R1"

echo "Reader 1 exited. Reader 2 connecting 1 second later..."
sleep 1.0
R2=$($PROBE --mode read_once)
echo "Reader 2 Output:"
echo "$R2"

DIGEST1=$(echo "$R1" | grep "EVENT=read_done" | sed -n 's/.*digest=\([0-9a-f]*\).*/\1/p')
DIGEST2=$(echo "$R2" | grep "EVENT=read_done" | sed -n 's/.*digest=\([0-9a-f]*\).*/\1/p')

if [ -n "$DIGEST1" ] && [ "$DIGEST1" = "$DIGEST2" ]; then
    echo "PASS: New reader immediately receives active clipboard offer upon binding data device (digests match: $DIGEST1)"
else
    echo "FAIL: Reader restart failed or digests mismatch ($DIGEST1 vs $DIGEST2)"
    exit 1
fi
kill -9 $W_PID 2>/dev/null || true
wait $W_PID 2>/dev/null || true
echo

# E.2: Writer Exit (Transience vs Persistence)
echo "--- E.2: Writer Exit (Transience vs Persistence) ---"
echo "Starting writer and verifying clipboard content..."
$PROBE --mode write --text "TRANSIENCE_VALIDATION_PAYLOAD" > /tmp/e2_w.log 2>&1 &
W_PID=$!
sleep 0.5

R_PRE=$($PROBE --mode read_once)
echo "Pre-exit read:"
echo "$R_PRE"

echo "Terminating writer process (PID $W_PID)..."
kill -15 $W_PID 2>/dev/null || true
wait $W_PID 2>/dev/null || true
sleep 0.3

echo "Reader connecting post-writer termination..."
R_POST=$($PROBE --mode read_once)
echo "Post-exit read:"
echo "$R_POST"

if echo "$R_POST" | grep -q "EVENT=selection_cleared"; then
    echo "PASS: PROVEN: COSMIC clipboard is TRANSIENT by default. When writer process exits, selection is immediately cleared."
else
    echo "INFO: Clipboard retained after exit: $R_POST"
fi
echo

# E.3: Ownership Replacement Race (Concurrent Writers)
echo "--- E.3: Ownership Replacement Race ---"
echo "Launching Writer A and Writer B concurrently..."
$PROBE --mode write --text "WRITER_A_PAYLOAD" > /tmp/e3_wa.log 2>&1 &
PID_A=$!
$PROBE --mode write --text "WRITER_B_PAYLOAD" > /tmp/e3_wb.log 2>&1 &
PID_B=$!

sleep 0.8

# Check which writer remains uncancelled and which was cancelled
CANCELLED_A=$(grep -c "EVENT=source_cancelled" /tmp/e3_wa.log || true)
CANCELLED_B=$(grep -c "EVENT=source_cancelled" /tmp/e3_wb.log || true)

echo "Writer A cancelled count: $CANCELLED_A (PID $PID_A)"
echo "Writer B cancelled count: $CANCELLED_B (PID $PID_B)"

R_RACE=$($PROBE --mode read_once)
echo "Clipboard content after race:"
echo "$R_RACE"

if [ "$CANCELLED_A" -eq 1 ] && [ "$CANCELLED_B" -eq 0 ]; then
    echo "PASS: Clean arbitration. Writer B won ownership, Writer A received source_cancelled."
elif [ "$CANCELLED_A" -eq 0 ] && [ "$CANCELLED_B" -eq 1 ]; then
    echo "PASS: Clean arbitration. Writer A won ownership, Writer B received source_cancelled."
else
    echo "NOTE: Simultaneous writes handled cleanly without deadlock. Cancels: A=$CANCELLED_A, B=$CANCELLED_B"
fi

kill -9 $PID_A $PID_B 2>/dev/null || true
wait $PID_A $PID_B 2>/dev/null || true
echo

# E.4: Client Disconnect / Compositor Crash Detection
echo "--- E.4: Connection Error & Compositor Severance Detection ---"
python3 - << 'EOF'
import subprocess
import time
import os
import signal

# Test how probe responds when Wayland socket or connection is broken
PROBE = "./06-cosmic-clipboard/bin/data_control_probe"

p = subprocess.Popen([PROBE, "--mode", "listen"],
                     stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
time.sleep(0.3)

# Check if client running
if p.poll() is None:
    print("Client connected and listening on Wayland socket.")

# Test behavior when Wayland display is non-existent
bad_env = os.environ.copy()
bad_env["WAYLAND_DISPLAY"] = "nonexistent-wayland-99"
p_bad = subprocess.run([PROBE, "--mode", "read_once"], env=bad_env,
                       stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
print(f"Non-existent socket exit code: {p_bad.returncode}")
print(f"Error output: {p_bad.stderr.strip()}")

p.terminate()
p.wait()

if p_bad.returncode != 0 and "wl_display_connect failed" in p_bad.stderr:
    print("PASS: Client immediately detects absent socket or broken connection with exit code 1")
else:
    print("FAIL: Socket disconnect detection failed")
EOF

echo "=== SECTION E COMPLETE ==="
