#!/usr/bin/env bash
set -euo pipefail

export WAYLAND_DISPLAY=wayland-1
export XDG_RUNTIME_DIR=/run/user/1000

PROBE="./06-cosmic-clipboard/bin/data_control_probe"
LOG_DIR="./06-cosmic-clipboard/results"
mkdir -p "$LOG_DIR"
OUT="$LOG_DIR/section_d_change_notification.log"
rm -f "$OUT"

exec > >(tee -a "$OUT") 2>&1

echo "=== SECTION D: CHANGE NOTIFICATION & LATENCY VALIDATION ==="
echo "Compositor: cosmic-comp (Smithay)"
echo "Wayland Socket: $XDG_RUNTIME_DIR/$WAYLAND_DISPLAY"
echo "Date: $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
echo

# D.1: Write-to-Notification and Notification-to-Read Latency (10 Iterations)
echo "--- D.1: Latency Profile Across 10 Independent Writes ---"
echo "Collecting 10 write-to-notification and notification-to-read data points..."

python3 - << 'EOF'
import subprocess
import time
import re

PROBE = "./06-cosmic-clipboard/bin/data_control_probe"

write_to_notif_times = []
notif_to_read_times = []

for i in range(10):
    # Start listener with auto_read (default)
    listener = subprocess.Popen([PROBE, "--mode", "listen"],
                                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    time.sleep(0.3)

    writer = subprocess.Popen([PROBE, "--mode", "write", "--text", f"LATENCY_TEST_SAMPLE_{i}"],
                              stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)

    # Read writer line for STATUS=selection_set time_us=...
    write_time_us = None
    for line in writer.stdout:
        m = re.search(r"STATUS=selection_set time_us=(\d+)", line)
        if m:
            write_time_us = int(m.group(1))
            break

    # Read listener lines
    notif_time_us = None
    read_duration_us = None
    for line in listener.stdout:
        m_notif = re.search(r"EVENT=selection_offer time_us=(\d+)", line)
        if m_notif:
            notif_time_us = int(m_notif.group(1))
        m_read = re.search(r"EVENT=read_done .*duration_us=(\d+)", line)
        if m_read:
            read_duration_us = int(m_read.group(1))
            break

    listener.terminate()
    writer.terminate()
    listener.wait()
    writer.wait()

    if write_time_us and notif_time_us and read_duration_us is not None:
        w_to_n = notif_time_us - write_time_us
        write_to_notif_times.append(w_to_n)
        notif_to_read_times.append(read_duration_us)
        print(f"Sample {i+1:02d}: write_to_notif = {w_to_n} us, notif_to_read = {read_duration_us} us")
    else:
        print(f"Sample {i+1:02d}: capture failed (w={write_time_us}, n={notif_time_us}, r={read_duration_us})")

def stats(arr, name):
    if not arr:
        return
    arr_sorted = sorted(arr)
    n = len(arr)
    avg = sum(arr) / n
    median = arr_sorted[n//2]
    print(f"\n{name} Stats (N={n}):")
    print(f"  Min:    {min(arr)} us")
    print(f"  Max:    {max(arr)} us")
    print(f"  Mean:   {avg:.1f} us")
    print(f"  Median: {median} us")

stats(write_to_notif_times, "Write-to-Notification Latency")
stats(notif_to_read_times, "Notification-to-Read (Pipe Transfer) Latency")
EOF
echo

# D.2: Rapid Burst Behavior (10 Writes in Rapid Succession)
echo "--- D.2: Rapid Burst Behavior (10 Consecutive Writes) ---"
echo "Launching listener for 10 offers..."
$PROBE --mode listen --max-offers 10 --no-read > /tmp/d2_listener.log 2>&1 &
L_PID=$!
sleep 0.5

echo "Firing 10 rapid burst writes with 5ms interval..."
$PROBE --mode write --burst 10 --burst-interval-ms 5 > /tmp/d2_writer.log 2>&1 &
W_PID=$!

# Wait for burst writes to finish and listener to capture
sleep 1.5

kill -9 $W_PID $L_PID 2>/dev/null || true
wait $W_PID $L_PID 2>/dev/null || true

OFFERS_COUNT=$(grep -c "EVENT=selection_offer" /tmp/d2_listener.log || true)
WRITES_COUNT=$(grep -c "EVENT=burst_step" /tmp/d2_writer.log || true)

echo "Burst writes fired: $WRITES_COUNT"
echo "Selection offers received by listener: $OFFERS_COUNT"
echo
echo "Listener Event Log Summary:"
cat /tmp/d2_listener.log

if [ "$OFFERS_COUNT" -eq 10 ]; then
    echo "PASS: Exactly 10/10 burst notifications delivered without coalescing or loss"
elif [ "$OFFERS_COUNT" -gt 0 ]; then
    echo "CONDITIONAL: Delivered $OFFERS_COUNT/10 notifications (coalesced or rate-governed)"
else
    echo "FAIL: Zero notifications delivered during burst"
    exit 1
fi
echo

# D.3: Zero-Delay Rapid Burst Behavior (10 Writes at 0ms Interval)
echo "--- D.3: Zero-Delay Rapid Burst Behavior (0ms Interval) ---"
$PROBE --mode listen --max-offers 10 --no-read > /tmp/d3_listener.log 2>&1 &
L3_PID=$!
sleep 0.5

echo "Firing 10 immediate burst writes (0ms interval)..."
$PROBE --mode write --burst 10 --burst-interval-ms 0 > /tmp/d3_writer.log 2>&1 &
W3_PID=$!
sleep 1.0

kill -9 $W3_PID $L3_PID 2>/dev/null || true
wait $W3_PID $L3_PID 2>/dev/null || true

OFFERS_COUNT3=$(grep -c "EVENT=selection_offer" /tmp/d3_listener.log || true)
echo "Burst writes fired: 10"
echo "Selection offers received: $OFFERS_COUNT3"
if [ "$OFFERS_COUNT3" -ge 1 ]; then
    echo "PASS: Zero-delay burst handled without compositor crash (received $OFFERS_COUNT3 events)"
fi
echo

# D.4: Duplicate Notification Behavior (Writing Same Content Twice)
echo "--- D.4: Duplicate Notification Behavior ---"
echo "Starting continuous listener..."
$PROBE --mode listen --max-offers 2 --no-read > /tmp/d4_listener.log 2>&1 &
L4_PID=$!
sleep 0.5

echo "Writing IDENTICAL_PAYLOAD (First instance)..."
$PROBE --mode write --text "IDENTICAL_PAYLOAD" > /tmp/d4_w1.log 2>&1 &
W4_1_PID=$!
sleep 0.5

echo "Writing IDENTICAL_PAYLOAD (Second instance)..."
$PROBE --mode write --text "IDENTICAL_PAYLOAD" > /tmp/d4_w2.log 2>&1 &
W4_2_PID=$!
sleep 0.5

kill -9 $W4_1_PID $W4_2_PID $L4_PID 2>/dev/null || true
wait $W4_1_PID $W4_2_PID $L4_PID 2>/dev/null || true

DUP_OFFERS=$(grep -c "EVENT=selection_offer" /tmp/d4_listener.log || true)
echo "Duplicate offers recorded: $DUP_OFFERS"
cat /tmp/d4_listener.log

if [ "$DUP_OFFERS" -eq 2 ]; then
    echo "PASS: Wayland compositor emits selection_offer on every set_selection regardless of identical payload (no deduplication)"
else
    echo "INFO: Received $DUP_OFFERS offers"
fi

echo "=== SECTION D COMPLETE ==="
