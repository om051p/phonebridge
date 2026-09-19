#!/usr/bin/env bash
set -euo pipefail

export WAYLAND_DISPLAY=wayland-1
export XDG_RUNTIME_DIR=/run/user/1000

PROBE="./06-cosmic-clipboard/bin/data_control_probe"
LOG_DIR="./06-cosmic-clipboard/results"
mkdir -p "$LOG_DIR"
OUT="$LOG_DIR/section_a_data_control.log"
rm -f "$OUT"

exec > >(tee -a "$OUT") 2>&1

echo "=== SECTION A: WAYLAND DATA-CONTROL VALIDATION ==="
echo "Compositor: cosmic-comp (Smithay)"
echo "Wayland Socket: $XDG_RUNTIME_DIR/$WAYLAND_DISPLAY"
echo "Date: $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
echo

# A.1: Verify Global Advertising
echo "--- A.1: Global Protocol Advertising ---"
if wayland-info | grep -q 'zwlr_data_control_manager_v1'; then
    echo "PASS: zwlr_data_control_manager_v1 is advertised by compositor"
    wayland-info | grep -C 1 'zwlr_data_control_manager_v1'
else
    echo "FAIL: zwlr_data_control_manager_v1 is NOT advertised"
    exit 1
fi
echo

# A.2: Windowless Background Client Write
echo "--- A.2: Windowless Background Client Write ---"
$PROBE --mode write --text "DATA_CONTROL_TEST_A2_$(date +%s)" > /tmp/a2_writer.log 2>&1 &
A2_PID=$!
sleep 0.5
if ps -p $A2_PID > /dev/null; then
    echo "PASS: Background writer registered and active (PID $A2_PID)"
else
    echo "FAIL: Background writer died"
    cat /tmp/a2_writer.log
    exit 1
fi

# A.3: Windowless Background Client Read
echo "--- A.3: Windowless Background Client Read ---"
A3_OUT=$($PROBE --mode read_once)
echo "$A3_OUT"
if echo "$A3_OUT" | grep -q 'DATA_CONTROL_TEST_A2_'; then
    echo "PASS: Background reader read exact payload written by background writer"
else
    echo "FAIL: Reader did not get expected payload"
    kill -9 $A2_PID 2>/dev/null || true
    exit 1
fi
kill -9 $A2_PID 2>/dev/null || true
echo

# A.4: Change Notification
echo "--- A.4: Change Notification to Background Listener ---"
# Max 3 events: initial state (cleared or existing), then write 1, then write 2
$PROBE --mode listen --max-events 3 > /tmp/a4_listener.log 2>&1 &
A4_LIS_PID=$!
sleep 0.5

# Perform external write 1
echo "Triggering write 1..."
$PROBE --mode write --text "NOTIFY_PAYLOAD_ALPHA" > /tmp/a4_w1.log 2>&1 &
W1_PID=$!
sleep 0.8
kill -9 $W1_PID 2>/dev/null || true
sleep 0.3

# Perform external write 2
echo "Triggering write 2..."
$PROBE --mode write --text "NOTIFY_PAYLOAD_BETA" > /tmp/a4_w2.log 2>&1 &
W2_PID=$!
sleep 0.8

wait $A4_LIS_PID || true
kill -9 $W2_PID 2>/dev/null || true
echo "Listener output:"
cat /tmp/a4_listener.log
if grep -q "NOTIFY_PAYLOAD_ALPHA" /tmp/a4_listener.log && grep -q "NOTIFY_PAYLOAD_BETA" /tmp/a4_listener.log; then
    echo "PASS: Both change events detected and read in real-time"
else
    echo "FAIL: Missed change notifications"
    exit 1
fi
echo

# A.5: Ownership Replacement & Cancelled Event
echo "--- A.5: Ownership Replacement & Source Cancellation ---"
$PROBE --mode write --text "OWNER_ONE" > /tmp/a5_w1.log 2>&1 &
A5_W1_PID=$!
sleep 0.5

$PROBE --mode write --text "OWNER_TWO" > /tmp/a5_w2.log 2>&1 &
A5_W2_PID=$!
sleep 0.5

# Check if OWNER_ONE received cancelled
sleep 0.2
cat /tmp/a5_w1.log
if grep -q "EVENT=source_cancelled" /tmp/a5_w1.log; then
    echo "PASS: Owner 1 received source_cancelled when Owner 2 claimed selection"
else
    echo "FAIL: Owner 1 did not receive source_cancelled"
fi
kill -9 $A5_W1_PID $A5_W2_PID 2>/dev/null || true
echo

# A.6: Multiple Concurrent Data-Control Listeners
echo "--- A.6: Multiple Concurrent Data-Control Listeners ---"
$PROBE --mode listen --max-events 2 > /tmp/a6_lis1.log 2>&1 &
LIS1_PID=$!
$PROBE --mode listen --max-events 2 > /tmp/a6_lis2.log 2>&1 &
LIS2_PID=$!
sleep 0.5

$PROBE --mode write --text "MULTI_CLIENT_BROADCAST" > /tmp/a6_w.log 2>&1 &
W_PID=$!
sleep 0.8

wait $LIS1_PID || true
wait $LIS2_PID || true
kill -9 $W_PID 2>/dev/null || true

echo "Listener 1:"
cat /tmp/a6_lis1.log
echo "Listener 2:"
cat /tmp/a6_lis2.log
if grep -q "MULTI_CLIENT_BROADCAST" /tmp/a6_lis1.log && grep -q "MULTI_CLIENT_BROADCAST" /tmp/a6_lis2.log; then
    echo "PASS: Multiple independent data-control listeners received event simultaneously"
else
    echo "FAIL: One or more listeners missed broadcast event"
fi
echo

# A.7: Owner Termination & Clipboard Persistence
echo "--- A.7: Owner Termination Behavior (Persistence Test) ---"
$PROBE --mode write --text "PERSISTENCE_TEST_DATA" > /tmp/a7_w.log 2>&1 &
A7_PID=$!
sleep 0.5
# Verify it's readable while owner is alive
echo "Reading while owner is alive:"
$PROBE --mode read_once
# Now kill owner process
echo "Killing owner process (PID $A7_PID)..."
kill -9 $A7_PID 2>/dev/null || true
sleep 0.5

echo "Reading after owner termination:"
set +e
A7_AFTER=$($PROBE --mode read_once 2>&1)
EXIT_CODE=$?
set -e
echo "$A7_AFTER"
echo "Exit code: $EXIT_CODE"
if echo "$A7_AFTER" | grep -q "selection_cleared"; then
    echo "BEHAVIOR: Selection cleared immediately on owner exit (Standard Wayland transient behavior - no compositor cache)"
elif echo "$A7_AFTER" | grep -q "PERSISTENCE_TEST_DATA"; then
    echo "BEHAVIOR: Selection persisted after owner exit (Compositor clipboard manager active)"
else
    echo "BEHAVIOR: Read returned error or empty offer"
fi
echo "=== SECTION A COMPLETE ==="
