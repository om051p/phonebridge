SHELL := /bin/bash
GO ?= go
JAVAC ?= javac
JAVA ?= java
NDK_CLANG ?= /opt/android-sdk/ndk/27.0.12077973/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android34-clang
JAVA_HOME ?= /usr/lib/jvm/java-17-openjdk-amd64

JNI_CFLAGS := -I$(JAVA_HOME)/include -I$(JAVA_HOME)/include/linux

.PHONY: all build build-host build-android test perf clean

all: build test

build: build-host build-android

build-host:
	@mkdir -p build/classes results
	@echo "==> Building Go JNI library for host JVM..."
	cd go && CGO_CFLAGS="$(JNI_CFLAGS)" $(GO) build -buildmode=c-shared -o ../build/libphonebridge_spike02.so bridge.go
	@echo "==> Compiling Java test harness..."
	$(JAVAC) -d build/classes harness/src/dev/phonebridge/spike02/*.java

build-android:
	@mkdir -p build
	@echo "==> Cross-compiling Go JNI library for Android arm64-v8a with NDK..."
	@if [ -x "$(NDK_CLANG)" ]; then \
		(cd go && CC="$(NDK_CLANG)" CGO_ENABLED=1 GOOS=android GOARCH=arm64 $(GO) build -buildmode=c-shared -o ../build/libphonebridge_spike02_arm64.so bridge.go) && \
		echo "==> Android arm64 shared library built successfully:" && \
		ls -lh build/libphonebridge_spike02_arm64.so; \
	else \
		echo "Warning: NDK compiler $(NDK_CLANG) not found or not executable. Skipping arm64 build."; \
	fi

test: build-host
	@echo "==> Running Spike 02 automated verification tests..."
	$(JAVA) -cp build/classes -Djava.library.path=build dev.phonebridge.spike02.Spike02Test build/libphonebridge_spike02.so

perf: build-host
	@echo "==> Running Spike 02 benchmark suite..."
	$(JAVA) -cp build/classes -Djava.library.path=build dev.phonebridge.spike02.Spike02Benchmark build/libphonebridge_spike02.so results

clean:
	rm -rf build results
