# Spike 04 — Pion WebRTC Android ↔ Linux (isolated prototype)
#
# All artifact paths are inside this spike. Nothing here touches production.
#
#   make deps        — bootstrap the pinned Go toolchain (spike-local, SHA256-verified)
#   make receiver    — build the Linux receiver + signaling server
#   make lib         — build the Android c-shared library (Pion engine)
#   make apk         — build the Android APK (Gradle, isolated project)
#   make all         — receiver + lib + apk
#   make install     — adb install the APK
#   make run-session — end-to-end: receiver + capture session (env: W H FPS ... SIGNAL_HOST)
#   make run-soak    — 30-minute 720p30 soak
#   make clean

S4_DIR   := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))
GO_DIR   := $(S4_DIR)/go
ANDROID  := $(S4_DIR)/android
OUT      ?= $(S4_DIR)/results
TC       := $(HOME)/.cache/phonebridge-spike04
GO_BIN   := $(TC)/go-1.24.13/bin
GO       := $(GO_BIN)/go
NDK      := $(shell ls -d /opt/android-sdk/ndk/* 2>/dev/null | sort -V | tail -1)
LIB_SO   := $(ANDROID)/app/src/main/jniLibs/arm64-v8a/libphonebridge_spike04.so
APK      := $(ANDROID)/app/build/outputs/apk/debug/app-debug.apk
RECEIVER := $(S4_DIR)/tools/spike04-receiver

# ---- Pinned toolchain (recorded in results + README; do not "use system go") --
GO_VERSION  := 1.24.13
GO_SHA256   := $(shell curl -fsSL "https://go.dev/dl/?mode=json&include=all" 2>/dev/null | grep -A5 '"go1.24.13.linux-amd64.tar.gz"' | grep -o '"[a-f0-9]\{64\}"' | head -1 | tr -d '"')
PION_VER    := webrtc/v4 v4.2.20   # resolved from go.mod (see go.sum for the tree)

export PATH := $(GO_BIN):$(PATH)

.PHONY: all deps receiver lib apk install run-session run-soak stats clean toolchain-info

all: receiver lib apk

deps:
	@mkdir -p "$(TC)"
	@if [ -x "$(GO)" ]; then echo "go already pinned: $$($(GO) version)"; \
	else \
	  echo "downloading go$(GO_VERSION).linux-amd64.tar.gz (sha256: $(GO_SHA256))"; \
	  curl -fsSL -o "$(TC)/go1.24.13.linux-amd64.tar.gz" "https://go.dev/dl/go1.24.13.linux-amd64.tar.gz" && \
	  echo "$(GO_SHA256)  $(TC)/go1.24.13.linux-amd64.tar.gz" | sha256sum -c - && \
	  tar -C "$(TC)" -xzf "$(TC)/go1.24.13.linux-amd64.tar.gz" && \
	  mv "$(TC)/go" "$(TC)/go-1.24.13"; \
	fi

receiver: deps
	cd "$(GO_DIR)" && $(GO) build -o "$(RECEIVER)" ./receiver

lib: deps
	@test -n "$(NDK)" || { echo "ERROR: Android NDK not found under /opt/android-sdk/ndk"; exit 1; }
	cd "$(GO_DIR)" && \
	PATH="$(NDK)/toolchains/llvm/prebuilt/linux-x86_64/bin:$(GO_BIN):$$PATH" \
	CGO_ENABLED=1 GOOS=android GOARCH=arm64 CC=aarch64-linux-android35-clang \
	$(GO) build -buildmode=c-shared -ldflags="-checklinkname=0" -o "$(LIB_SO)" . && \
	ls -l "$(LIB_SO)"

apk:
	cd "$(ANDROID)" && ./gradlew --no-daemon :app:assembleDebug

install: apk
	adb install -r -t "$(APK)"

toolchain-info:
	@echo "go:      $$($(GO) version 2>/dev/null || echo missing — run make deps)"
	@echo "pion:    $(PION_VER)"
	@echo "ndk:     $(NDK)"
	@echo "libso:   $(LIB_SO) $$(stat -c%s "$(LIB_SO)" 2>/dev/null) bytes"

# Host must be on the phone's WLAN; SIGNAL_HOST overrides autodetection.
run-session:
	S4_DIR="$(S4_DIR)" OUT="$(OUT)" bash "$(S4_DIR)/tools/spike04.sh" session

run-soak:
	S4_DIR="$(S4_DIR)" OUT="$(OUT)" bash "$(S4_DIR)/tools/spike04.sh" soak

stats:
	@bash "$(S4_DIR)/tools/spike04.sh" pull
	@python3 - <<-'PY'
	import json, glob, sys
	for f in sorted(glob.glob("$(OUT)/device/*.json")):
	    try:
	        d = json.load(open(f))
	    except Exception:
	        continue
	    print(f.split('/')[-1], d.get("status"), d.get("capture", {}).get("avg_fps"),
	          d.get("capture", {}).get("avg_kbps"))
	PY

clean:
	cd "$(ANDROID)" && ./gradlew --no-daemon clean || true
	rm -rf "$(OUT)/device" "$(OUT)/receiver-run" "$(RECEIVER)"
