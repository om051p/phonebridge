#!/usr/bin/env bash
# Spike 07 / Phase 6 Slice 3 — P1 runner (daemon-side decode/encode cost).
# Reproducible: generates Annex-B reference streams, then runs every p1 mode.
# All artifacts land in results/ (gitignored). Output is also tee'd to results/p1.log.
set -euo pipefail
cd "$(dirname "$0")"

export PATH="${HOME}/go-sdk/go/bin:${PATH}"
mkdir -p results go/bin

if [ ! -x go/bin/p1 ]; then
  (cd go && go build -o bin/p1 ./p1)
fi

P1=go/bin/p1
LOG=results/p1.log
: > "$LOG"

gen() { # gen <file> <WxH> <rate> <seconds> <keyint> <bitrate>
  local f="$1" size="$2" rate="$3" dur="$4" keyint="$5" br="$6"
  if [ ! -s "results/$f" ]; then
    echo "== generating results/$f (${size}@${rate}, ${dur}s, GOP ${keyint}, ${br})"
    ffmpeg -hide_banner -loglevel error -y \
      -f lavfi -i "testsrc2=size=${size}:rate=${rate}" -t "$dur" \
      -c:v libx264 -profile:v main -preset medium \
      -x264-params "keyint=${keyint}:min-keyint=${keyint}:scenecut=0:repeat-headers=1:bframes=0" \
      -b:v "$br" -maxrate "$br" -bufsize "$(( ${br%k} * 2 ))k" -pix_fmt yuv420p \
      -f h264 "results/$f"
  fi
}

gen 720p30.h264  720x1600  30 30 30 2500k
gen 1080p60.h264 1080x2400 60 20 60 6000k

run() { echo "+ $*" | tee -a "$LOG"; "$@" 2>&1 | tee -a "$LOG"; }

echo "=== P1: decode floor (shared with ffplay: same libavcodec input path) ==="
run "$P1" decode -i results/720p30.h264
run "$P1" decode -i results/1080p60.h264

echo "=== P1: MJPEG transcode (flood = capacity, not realtime) ==="
for q in 2 3 5; do
  run "$P1" mjpeg -i results/720p30.h264 -q "$q"
done
for q in 2 3; do
  run "$P1" mjpeg -i results/1080p60.h264 -q "$q"
done

echo "=== P1: rawvideo I420 variant (no re-encode) ==="
run "$P1" i420 -i results/720p30.h264 -size 720x1600
run "$P1" i420 -i results/1080p60.h264 -size 1080x2400

echo "=== P1: paced per-frame latency (decoder residency = framecrc) ==="
run "$P1" rtt -i results/720p30.h264 -proto framecrc -fps 30 -n 300
run "$P1" rtt -i results/720p30.h264 -proto framecrc -fps 30 -n 300 -threads 1

echo "=== P1: paced per-frame latency (full transcode AU -> JPEG) ==="
run "$P1" rtt -i results/720p30.h264 -proto mjpeg -q 3 -fps 30 -n 300
run "$P1" rtt -i results/720p30.h264 -proto mjpeg -q 3 -fps 30 -n 300 -threads 1
run "$P1" rtt -i results/720p30.h264 -proto mjpeg -q 3 -fps 60 -n 300
run "$P1" rtt -i results/1080p60.h264 -proto mjpeg -q 3 -fps 60 -n 600
run "$P1" rtt -i results/1080p60.h264 -proto mjpeg -q 2 -fps 60 -n 600

echo "=== P1: keyframe re-join recovery (DEC-021 re-injection model) ==="
run "$P1" recovery -i results/720p30.h264

echo "=== P1: thread sweep (decoder threads = THE latency knob) ==="
run "$P1" decode -i results/720p30.h264
for t in 1 2 4; do
  run "$P1" mjpeg -i results/720p30.h264 -q 3 -threads "$t"
done
run "$P1" mjpeg -i results/1080p60.h264 -q 3 -threads 1
run "$P1" mjpeg -i results/1080p60.h264 -q 3 -threads 4
run "$P1" rtt -i results/720p30.h264 -proto framecrc -fps 30 -n 300 -threads 2
run "$P1" rtt -i results/720p30.h264 -proto framecrc -fps 30 -n 300 -threads 4
run "$P1" rtt -i results/720p30.h264 -proto mjpeg -q 3 -fps 60 -n 300 -threads 1
run "$P1" rtt -i results/1080p60.h264 -proto mjpeg -q 3 -fps 60 -n 300 -threads 1
run "$P1" rtt -i results/1080p60.h264 -proto mjpeg -q 3 -fps 60 -n 600 -threads 4

echo "=== P1 complete — see results/p1.log ==="
