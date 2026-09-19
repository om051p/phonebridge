#!/usr/bin/env bash
set -euo pipefail

export WAYLAND_DISPLAY=wayland-1
export XDG_RUNTIME_DIR=/run/user/1000

PROBE="./06-cosmic-clipboard/bin/data_control_probe"
LOG_DIR="./06-cosmic-clipboard/results"
mkdir -p "$LOG_DIR"
OUT="$LOG_DIR/section_c_mime_types.log"
rm -f "$OUT"

exec > >(tee -a "$OUT") 2>&1

echo "=== SECTION C: MIME TYPES & PAYLOAD CAPACITY VALIDATION ==="
echo "Compositor: cosmic-comp (Smithay)"
echo "Wayland Socket: $XDG_RUNTIME_DIR/$WAYLAND_DISPLAY"
echo "Date: $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
echo

# C.1: text/plain & text/plain;charset=utf-8
echo "--- C.1: text/plain and text/plain;charset=utf-8 ---"
$PROBE --mode write --text "TEXT_PLAIN_UTF8_VERIFICATION_€_🚀_日本語" > /tmp/c1_w.log 2>&1 &
C1_W_PID=$!
sleep 0.5

R_UTF8=$($PROBE --mode read_once --mime "text/plain;charset=utf-8")
echo "Read text/plain;charset=utf-8:"
echo "$R_UTF8"

R_PLAIN=$($PROBE --mode read_once --mime "text/plain")
echo "Read text/plain:"
echo "$R_PLAIN"

kill -9 $C1_W_PID 2>/dev/null || true
wait $C1_W_PID 2>/dev/null || true
echo

# C.2: text/html
echo "--- C.2: text/html ---"
HTML_SNIPPET="<div style='color:red;'><h1>PhoneBridge COSMIC</h1><p>Rich text test</p></div>"
$PROBE --mode write --text "Plain text fallback" --html "$HTML_SNIPPET" > /tmp/c2_w.log 2>&1 &
C2_W_PID=$!
sleep 0.5

R_HTML=$($PROBE --mode read_once --mime "text/html")
echo "Read text/html:"
echo "$R_HTML"
if echo "$R_HTML" | grep -q "PhoneBridge COSMIC"; then
    echo "PASS: text/html read successfully with HTML tags intact"
else
    echo "FAIL: text/html read failed"
    exit 1
fi
kill -9 $C2_W_PID 2>/dev/null || true
wait $C2_W_PID 2>/dev/null || true
echo

# C.3: text/uri-list
echo "--- C.3: text/uri-list ---"
URI_LIST="file:///home/pop-os/document.pdf\r\nfile:///home/pop-os/image.png\r\n"
$PROBE --mode write --text "URI fallback text" --uri-list "$URI_LIST" > /tmp/c3_w.log 2>&1 &
C3_W_PID=$!
sleep 0.5

R_URI=$($PROBE --mode read_once --mime "text/uri-list")
echo "Read text/uri-list:"
echo "$R_URI"
if echo "$R_URI" | grep -q "document.pdf"; then
    echo "PASS: text/uri-list read successfully"
else
    echo "FAIL: text/uri-list read failed"
    exit 1
fi
kill -9 $C3_W_PID 2>/dev/null || true
wait $C3_W_PID 2>/dev/null || true
echo

# C.4: Multi-Format Offer (Text + HTML simultaneously)
echo "--- C.4: Multi-Format Offer (Text + HTML Simultaneous Availability) ---"
$PROBE --mode write --text "MULTI_PLAIN_REPRESENTATION" --html "<b>MULTI_HTML_REPRESENTATION</b>" > /tmp/c4_w.log 2>&1 &
C4_W_PID=$!
sleep 0.5

MF_PLAIN=$($PROBE --mode read_once --mime "text/plain")
MF_HTML=$($PROBE --mode read_once --mime "text/html")

echo "Plain format query: $MF_PLAIN"
echo "HTML format query: $MF_HTML"

if echo "$MF_PLAIN" | grep -q "MULTI_PLAIN_REPRESENTATION" && echo "$MF_HTML" | grep -q "MULTI_HTML_REPRESENTATION"; then
    echo "PASS: Single clipboard selection concurrently served distinct MIME formats"
else
    echo "FAIL: Multi-format offer failed"
    exit 1
fi
kill -9 $C4_W_PID 2>/dev/null || true
wait $C4_W_PID 2>/dev/null || true
echo

# C.5: Custom / Binary MIME Type (application/x-phonebridge-test)
echo "--- C.5: Custom & Binary MIME Type ---"
$PROBE --mode write --custom-mime "application/x-phonebridge-envelope" --binary > /tmp/c5_w.log 2>&1 &
C5_W_PID=$!
sleep 0.5

R_BIN=$($PROBE --mode read_once --mime "application/x-phonebridge-envelope")
echo "Binary read output:"
echo "$R_BIN"
if echo "$R_BIN" | grep -q "application/x-phonebridge-envelope"; then
    echo "PASS: Custom binary MIME type successfully registered and transferred across Wayland data-control"
else
    echo "FAIL: Custom binary MIME type failed"
    exit 1
fi
kill -9 $C5_W_PID 2>/dev/null || true
wait $C5_W_PID 2>/dev/null || true
echo

# C.6: Payload Scalability & Throughput (100KB, 1MB, 5MB, 10MB, 25MB, 50MB)
echo "--- C.6: Maximum Payload Size & Pipe Throughput ---"
for S_MB in 1 5 10 25 50; do
    echo "Testing payload size: ${S_MB} MB..."
    $PROBE --mode write --size-mb $S_MB > /tmp/c6_w_${S_MB}.log 2>&1 &
    C6_PID=$!
    sleep 0.8

    R_BIG=$($PROBE --mode read_once --mime "text/plain;charset=utf-8")
    echo "$R_BIG"
    kill -9 $C6_PID 2>/dev/null || true
    wait $C6_PID 2>/dev/null || true
    sleep 0.5
done

echo "=== SECTION C COMPLETE ==="
