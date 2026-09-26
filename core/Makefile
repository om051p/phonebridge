BUF ?= buf
GO ?= go
PROTO_DIR ?= ../proto

# JNI library build (Step 3, DEC-019 c-shared boundary).
# host-lib:      builds the jni-tagged c-shared library for the HOST JVM —
#                loaded by android/app/src/test GoBridgeMediaTest to prove AUs
#                cross JNI into the production transport without an emulator.
# android-lib:   cross-compiles libphonebridge_core.so for the ABIs the app
#                ships (arm64-v8a, x86_64) into android jniLibs.
JAVA_HOME ?= /usr/lib/jvm/java-17-openjdk-amd64
NDK ?= /opt/android-sdk/ndk/27.0.12077973
JNI_CFLAGS := -I$(JAVA_HOME)/include -I$(JAVA_HOME)/include/linux
BUILD_DIR := build
NDK_TC := $(NDK)/toolchains/llvm/prebuilt/linux-x86_64/bin

# Run from repo root:  make -C core gen
# Run from core/:      make gen
#
# buf is always invoked with CWD=proto/ (the module root). `out: ../core/pkg/protocol`
# in proto/buf.gen.yaml is resolved relative to the working directory, so running buf
# from core/ or from the repo root fails or writes to the wrong place.

VERSION ?= 0.1.0
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(GIT_COMMIT) -X main.buildDate=$(BUILD_DATE)

.PHONY: gen lint breaking check-generated vet test build fmt daemon ipcdrv receiver host-lib android-arm64 android-x86_64 android-lib

gen:
	cd $(PROTO_DIR) && $(BUF) generate

lint:
	cd $(PROTO_DIR) && $(BUF) lint

# Must run from the repo root so the baseline module root matches the input.
breaking:
	cd .. && $(BUF) breaking proto --against '.git#branch=main,subdir=proto'

# Fails on modified *and* newly generated (untracked) files: `git diff --exit-code`
# alone cannot see untracked output.
check-generated: gen
	@if [ -n "$$(git status --porcelain -- . ../ui/lib/generated)" ]; then \
		echo "generated code is stale or uncommitted — commit it:"; \
		git status --porcelain -- . ../ui/lib/generated; \
		exit 1; \
	fi

vet:
	$(GO) vet ./...

test:
	$(GO) test ./... -count=1 -race

build:
	$(GO) build ./...

fmt:
	@test -z "$$($(GO) fmt ./... 2>&1)" || (echo "unformatted files"; $(GO) fmt ./...; exit 1)

receiver:
	mkdir -p $(BUILD_DIR)
	$(GO) build -o $(BUILD_DIR)/phonebridge-receiver ./cmd/receiver

daemon:
	mkdir -p $(BUILD_DIR)
	$(GO) build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/phonebridge-daemon ./cmd/daemon

ipcdrv:
	mkdir -p $(BUILD_DIR)
	$(GO) build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/ipcdrv ./cmd/ipcdrv

host-lib:
	mkdir -p $(BUILD_DIR)
	CGO_CFLAGS="$(JNI_CFLAGS)" $(GO) build -tags jni -buildmode=c-shared -o $(BUILD_DIR)/libphonebridge_core.so ./cmd/android

android-arm64:
	mkdir -p ../android/app/src/main/jniLibs/arm64-v8a
	CC=$(NDK_TC)/aarch64-linux-android34-clang CGO_ENABLED=1 GOOS=android GOARCH=arm64 \
		$(GO) build -buildmode=c-shared -ldflags="-checklinkname=0" \
		-o ../android/app/src/main/jniLibs/arm64-v8a/libphonebridge_core.so ./cmd/android

android-x86_64:
	mkdir -p ../android/app/src/main/jniLibs/x86_64
	CC=$(NDK_TC)/x86_64-linux-android34-clang CGO_ENABLED=1 GOOS=android GOARCH=amd64 \
		$(GO) build -buildmode=c-shared -ldflags="-checklinkname=0" \
		-o ../android/app/src/main/jniLibs/x86_64/libphonebridge_core.so ./cmd/android

android-lib: android-arm64 android-x86_64

