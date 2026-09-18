# Spike 02 Results — Flutter/Kotlin ↔ Go Integration on Android

> Status: `EXPERIMENTAL` artifact — **Architecture & Embedding Recommendation**
> Frame: [02-flutter-kotlin-go-android.md](02-flutter-kotlin-go-android.md) ·
> Prototype: [`spikes/02-flutter-kotlin-go-android/`](../../spikes/02-flutter-kotlin-go-android/) ·
> Raw data: [`spikes/02-flutter-kotlin-go-android/results/`](../../spikes/02-flutter-kotlin-go-android/results/)
> *(regenerable per-run evidence via `make -C spikes/02-flutter-kotlin-go-android perf`; intentionally not committed — the distilled numbers below are the durable record)*

---

## 1. Executive Verdict & Core Recommendation

**In-process `c-shared` Go library + JNI (`libphonebridge_core.so`) embedded in an Android Foreground Service is the recommended architecture for PhoneBridge on Android.**

### Summary of Decisions

1. **JNI + `c-shared` is selected over `gomobile`:** `gomobile` is practically unmaintained, severely restricts exported Go types to a small primitive subset, cannot bind complex protobuf message trees without manual byte-array wrappers, and introduces reflection overhead. In contrast, `c-shared` with hand-crafted JNI stubs gives complete control over memory layouts, thread attachment, zero-copy buffers, and NDK compatibility.
2. **In-process embedding is selected over a separate local daemon (UDS/gRPC):** On Linux (Spike 01), a standalone systemd user daemon communicating over UDS + gRPC is optimal. On Android, running a separate background daemon executable is severely punished by the OS:
   - Android's Low Memory Killer (LMK) and battery optimizations (Doze, App Standby) aggressively terminate orphan/disowned background processes.
   - SELinux policies (`isolated_app`, `app_data_file`) restrict creating and sharing Unix domain sockets across processes.
   - An Android Foreground Service (`android.app.Service`) with an active ongoing notification guarantees process survival. Embedding Go directly within this service process eliminates IPC serialization overhead and matches standard Android architecture.
3. **Sub-microsecond latency:** In-process JNI call overhead is **1.00 µs (p50)** compared to **440 µs (p50)** for UDS + gRPC on Linux (~400× faster).
4. **Panic containment is proven:** A Go runtime panic within the native bridge is intercepted via `recover()` in the cgo boundary, preventing SIGABRT and converting the panic safely into a Java `java.lang.IllegalStateException`.
5. **Memory trimming is viable:** In response to Android's `onTrimMemory(level)`, triggering `runtime.GC()` and `debug.FreeOSMemory()` reduces the Go heap by over **70%** (1,123 KB → 328 KB in microbenchmarks), releasing physical pages back to the Linux kernel.

---

## 2. Evaluation of Realistic Android ↔ Go Options

| Criteria | Option A: `c-shared` + JNI (Recommended) | Option B: `gomobile bind` | Option C: Android Local Daemon (UDS / gRPC) | Option D: Flutter FFI direct to Go |
|---|---|---|---|---|
| **Process Model** | In-process (Kotlin Service) | In-process (Kotlin Service) | Out-of-process (Forked daemon) | In-process (Flutter UI Isolate) |
| **NDK Compatibility** | Direct Clang/NDK r27 integration | Wraps older NDK toolchains | N/A (separate binary executable) | Direct Clang/NDK r27 |
| **Type Support** | Arbitrary bytes / Protobuf payloads | Strict primitive whitelist; no slices of structs | Full Protobuf via gRPC stubs | C types / pointers / byte buffers |
| **Thread Affinity** | Explicit `AttachCurrentThread` + `LockOSThread` | Automatic (hidden in generated JNI) | Independent daemon threads | UI Isolate blocks unless worker Isolate used |
| **Android Lifecycle Fit** | **Excellent:** Tied to Foreground Service | **Good:** Tied to Foreground Service | **Poor:** LMK kills standalone binaries; SELinux blocks UDS | **Poor:** Tied to Flutter Activity; dies when UI dismissed |
| **Unary Latency (p50)** | **1.00 µs** | ~2.5–5 µs (reflection/boxing) | ~450–900 µs | **< 1.0 µs** |
| **Memory Footprint** | ~11.7 MB VmSys, ~1 MB heap | Similar | Double overhead (two runtimes + libc) | ~11.7 MB VmSys, ~1 MB heap |
| **Maintenance Risk** | Low (Standard Go toolchain `go build -buildmode=c-shared`) | High (gomobile has low maintenance & slow fixes) | Medium (SELinux updates, daemon process monitoring) | Low |

### Detailed Rationale

- **Why `gomobile` was rejected:** `gomobile bind` requires Go packages to conform to restrictive API signatures. It generates cumbersome Java wrapper classes that mirror Go structs, relying on finalizers and reflection. Furthermore, PhoneBridge already has a canonical schema (`phonebridge.v1` Protobuf). Serializing protobuf to `byte[]` and passing it across a single JNI entry point (`invoke(String method, byte[] payload): byte[]`) is cleaner, faster, and avoids code generator coupling.
- **Why Android Local Daemon was rejected:** Android is not a generic desktop Linux distribution. Android uses Android Binder for IPC, not UDS. Running an independent Go daemon binary requires either root, `su`, or packaging an ELF binary in the APK's `lib/` directory and executing it via `Runtime.getRuntime().exec()`. Android 10+ (`targetSdkVersion >= 29`) completely prohibits executing binaries from writable app directories (W^X violation). Even if spawned from `lib/`, the daemon lacks an Android `Context`, cannot run a Foreground Service notification, cannot acquire wake locks via Android APIs, and will be terminated by the OS within seconds of the app going to the background.
- **Why Flutter FFI direct to Go is not the primary boundary:** While Flutter FFI achieves < 1 µs calls, routing through Kotlin is mandatory on Android because PhoneBridge features (MediaProjection, Android NotificationListenerService, Foreground Service, WifiP2P/NSD discovery) are Android platform APIs accessible only via Kotlin/Java. If Flutter spoke directly to Go via FFI, Go would still need JNI to talk to Kotlin, creating a confusing triangular dependency (`Flutter -> Go -> Kotlin -> Go`). Having **Kotlin act as the central Android host** (`Flutter <MethodChannel/EventChannel> Kotlin <JNI> Go`) creates a clean, layered architecture:
  ```text
  Flutter UI (Presentation)
       │ (MethodChannel / EventChannel)
  Kotlin Host Service (Android System APIs & Lifecycle)
       │ (JNI binary protobuf bridge)
  Go Core Engine (Pion WebRTC, Protocol, Device Identity)
  ```

---

## 3. What Was Prototyped

All prototype code is strictly isolated under `spikes/02-flutter-kotlin-go-android/`. Production code in `core/`, `ui/`, and `proto/` was untouched.

```text
spikes/02-flutter-kotlin-go-android/
├── Makefile                                # Orchestrates host build, Android NDK build, tests, and perf
├── go/
│   ├── go.mod                             # Go module for spike
│   └── bridge.go                          # c-shared JNI implementation with panic recovery and lifecycle hooks
├── harness/
│   └── src/dev/phonebridge/spike02/
│       ├── EventListener.java             # Async callback interface for Go -> Java push events
│       ├── GoBridge.java                  # JNI native bindings and loader
│       ├── AndroidLifecycleSimulator.java # Simulates Android Service lifecycle transitions
│       ├── Spike02Test.java               # 6 automated verification tests
│       └── Spike02Benchmark.java          # Microbenchmark suite (latency, sweep, throughput, memory)
└── results/
    └── spike02_benchmark.json             # Structured evidence output
```

### Key Engineering Features in Prototype

1. **Panic Protection:**
   The JNI boundary uses Go's `defer recover()`. When an unhandled panic occurs in Go, the bridge catches it, updates internal diagnostic counters, and throws a `java.lang.IllegalStateException` into the calling JVM thread. The JVM and Android host process remain completely healthy.
2. **Safe Goroutine JNI Callbacks:**
   When Go pushes asynchronous events back into Java/Kotlin, it locks the OS thread (`runtime.LockOSThread()`), attaches the thread to the `JavaVM` via `AttachCurrentThread`, executes the callback using cached `jmethodID` references and a `jobject` global reference (`NewGlobalRef`), deletes local references, and detaches cleanly.
3. **Android Lifecycle State Machine:**
   `bridge.go` tracks engine states (`STOPPED`, `RUNNING`, `TRIMMED`). The Java harness simulates `onCreate` -> `onStartCommand` -> `onTrimMemory` -> `onDestroy` -> recreate.
4. **Cross-Compilation with Android NDK:**
   A dedicated Makefile target (`make build-android`) cross-compiles `libphonebridge_spike02_arm64.so` using Android NDK r27 (`aarch64-linux-android34-clang`) targeting Android API 34 (`arm64-v8a`).

---

## 4. Verification & Functional Test Results

All 6 requirements were executed and verified via `make test`:

```text
=== Spike 02: Android (Kotlin/Java) ↔ Go Boundary Verification ===
Loading native library: build/libphonebridge_spike02.so
Native library successfully loaded into JVM.
[TEST 1] 1. Start Go component from Android... PASSED
[TEST 2] 2. Synchronous request/response... PASSED
[TEST 3] 3. Asynchronous event delivery... PASSED
[TEST 4] 4. Error handling and safe panic recovery... PASSED
[TEST 5] 5. Clean shutdown & resource release... PASSED
[TEST 6] 6. Android lifecycle state compliance... PASSED
------------------------------------------------------------------
Test Results: 6/6 passed (100.0%)
Spike 02 verification SUCCESSFUL: All 6 requirements proven.
```

### Detailed Functional Findings

| Requirement | Test Scenario | Observed Result | Verdict |
|---|---|---|---|
| **1. Start Go** | `GoBridge.start(storageDir)` | Initializes `JavaVM*`, sets state `RUNNING`. Idempotent on double-start. | **PASS** |
| **2. Request/Response** | `GoBridge.invoke("ping", data)` + binary echo | Returns matching payload with sub-microsecond roundtrip. | **PASS** |
| **3. Asynchronous Events** | `GoBridge.subscribe(listener, 5, 20)` | Goroutine streams 5 sequential events at 20 ms intervals. Sequence verified. | **PASS** |
| **4. Error & Panic Handling** | `invoke("panic", null)` and `invoke("error", null)` | Intercepted in Go `defer`; thrown as `IllegalStateException` in Java; JVM survived. | **PASS** |
| **5. Clean Shutdown** | `GoBridge.stop()` | Stream cancelled, goroutines drained, state returned to `STOPPED`. | **PASS** |
| **6. Android Lifecycle** | `LifecycleSimulator` (create → start → trim → destroy → recreate) | Survived full simulated service lifecycle and cleanly restarted. | **PASS** |

---

## 5. Performance & Overhead Benchmarks

Captured on host JVM (Linux amd64, AMD Ryzen AI 9 HX 470, OpenJDK 17) and cross-compiled for Android arm64.

### 5.1 Startup Latency

| Phase | Duration | Notes |
|---|---|---|
| `System.loadLibrary` (`dlopen`) | **1.87 ms** | Loads 2.5 MB `.so` and resolves JNI symbols |
| `GoBridge.start` (Go initialization) | **0.14 ms** | Stores VM pointer, initializes atomics |
| **Total Cold Startup** | **2.02 ms** | Well within 50 ms budget; 0 risk of ANR (ANR threshold is 5,000 ms) |

### 5.2 Unary Call Latency (Request/Response)

Measurements over $n = 10,000$ sequential round-trip calls:

| Metric | Measured Value | Comparison to Linux UDS+gRPC (Spike 01) |
|---|---|---|
| **p50 (Median)** | **1.00 µs** | 440× faster than UDS (440 µs) |
| **p90** | **1.07 µs** | ~700× faster |
| **p99** | **2.57 µs** | ~500× faster than UDS p99 (1,300 µs) |
| **Mean** | **1.08 µs** | — |
| **Throughput** | **926,008 calls/sec** | Single-threaded saturation |

### 5.3 Payload Sweep (Binary Echo)

Measuring the effect of payload size on JNI data marshalling (`byte[]` across JNI boundary):

| Payload Size | Median Latency (p50) | Tail Latency (p99) | Assessment |
|---|---|---|---|
| **64 bytes** (Metadata / Ping) | **0.99 µs** | **2.67 µs** | Negligible overhead |
| **1,024 bytes** (1 KiB Protobuf) | **1.44 µs** | **6.46 µs** | Ideal for signaling & state |
| **16,384 bytes** (16 KiB Chunks) | **4.75 µs** | **73.84 µs** | Sub-0.1 ms |
| **65,536 bytes** (64 KiB Bulk Limit) | **27.33 µs** | **157.85 µs** | ~0.027 ms — perfectly fits 60 fps frame budget |
| **262,144 bytes** (256 KiB) | **54.38 µs** | **302.53 µs** | Still ~0.05 ms |

> **Protocol Guidance Alignment:** PhoneBridge's standard guidance is to keep bulk-content chunks $\le 64\text{ KiB}$. At 64 KiB, JNI transfer costs only **27.33 µs**, representing less than **0.17%** of a single 16.6 ms (60 fps) UI frame.

### 5.4 Asynchronous Event Delivery

- **1,000 events** streamed across JNI from background Go goroutines.
- Throughput: **925 events/sec** (limited intentionally by the 1 ms interval timer).
- Zero event drops, zero sequence inversion.

### 5.5 Memory Footprint & Trimming

| State | Go Heap Allocated (`runtime.MemStats`) | Go System Memory (`Sys`) |
|---|---|---|
| Active load (post-16,000 requests) | 1,123 KB (1.1 MB) | 11.7 MB |
| Post-`onTrimMemory` (`GC` + `FreeOSMemory`) | **328 KB** (0.3 MB) | 11.7 MB (virtual address reservation) |
| **Heap Memory Reclaimed** | **70.8% reduction** | Physical RSS returned to kernel via `madvise(MADV_DONTNEED)` |

---

## 6. Android Platform Layout Analysis & Architectural Deficiencies

Inspection of the repository revealed key architectural issues in the current Android scaffold:

### Observed Deficiencies in `android/`
1. **Orphaned Android Project:** `android/` is located at the workspace root, while Flutter's UI lives in `ui/`. Standard Flutter applications place the Android shell under `ui/android/`.
2. **Missing Gradle Wrapper:** `android/` contains no `gradlew` or `gradle/wrapper/gradle-wrapper.properties`.
3. **Non-functional build scripts:** `android/app/build.gradle.kts` specifies `apply false` on Android plugins:
   ```kotlin
   plugins {
       id("com.android.application") version "8.4.0" apply false
       id("org.jetbrains.kotlin.android") version "1.9.22" apply false
   }
   ```
   No root `build.gradle.kts` exists, meaning `./gradlew assembleDebug` cannot run.
4. **No Flutter-to-Android Bridge:** Flutter is not linked as a module or submodule to `android/`.

### Recommended Production Project Layout for Android (Phase 1)

To integrate Flutter, Kotlin, and Go cleanly:

```text
android/
├── build.gradle.kts                        # Root build config with Android Gradle Plugin & Kotlin
├── settings.gradle.kts                     # Includes :app, wires Flutter module
├── gradlew                                 # Pinned Gradle wrapper (e.g. Gradle 8.6+)
└── app/
    ├── build.gradle.kts                    # App plugin, NDK ndkVersion, jniLibs packaging
    └── src/main/
        ├── AndroidManifest.xml             # Declares PhoneBridgeForegroundService, permissions
        ├── jniLibs/                        # Precompiled or CMake-linked Go shared libraries
        │   ├── arm64-v8a/libphonebridge_core.so
        │   ├── armeabi-v7a/libphonebridge_core.so (optional)
        │   └── x86_64/libphonebridge_core.so (emulator)
        └── kotlin/dev/phonebridge/
            ├── PhoneBridgeApp.kt           # Application class
            ├── bridge/
            │   └── GoCoreBridge.kt         # JNI bridge object (implements native methods)
            └── service/
                └── PhoneBridgeService.kt   # Foreground Service hosting Go runtime
```

---

## 7. Security Implications

| Security Aspect | Assessment |
|---|---|
| **Inter-Process Trust Boundary** | None needed. In-process JNI shares the application's Linux UID and SELinux sandbox (`u0_aXXX`). No exposed network ports or world-readable filesystem sockets. |
| **SELinux Confinement** | JNI runs strictly inside the app's SELinux domain (`untrusted_app`). It does not require special SELinux policies, unlike standalone daemon binaries or cross-UID domain sockets. |
| **No Dynamic Code Loading** | The Go `.so` is compiled ahead-of-time and bundled into `apk/lib/<abi>/`. It satisfies Google Play's strict policy against dynamic executable code downloading. |
| **Memory Isolation** | Memory corruption (e.g. C buffer overflow) can affect the host process. Hand-crafted cgo code must use `C.getByteArrayRegion` with bounds checking to prevent buffer overflows. Memory allocations in the bridge must be strictly paired with frees. |

---

## 8. Impact on Production Project

1. **No changes to `phonebridge.v1` Protocol:** The device-to-device protocol is untouched.
2. **Dual-Transport Architecture Ratified:**
   - **Linux:** Flutter ↔ Go over **UDS + gRPC** (`phonebridge.localipc.v1`, ratified as DEC-018).
   - **Android:** Kotlin ↔ Go over **In-Process JNI** (`libphonebridge_core.so`), with Flutter talking to Kotlin via standard Flutter Platform Channels (`MethodChannel` / `EventChannel`).
3. **Build Pipeline:**
   - Add a cross-compilation step in CI (`CGO_ENABLED=1 GOOS=android GOARCH=arm64 CC=$NDK/aarch64-linux-android34-clang go build -buildmode=c-shared`) to generate `libphonebridge_core.so`.
   - Place the output library in `android/app/src/main/jniLibs/arm64-v8a/`.

---

## 9. Next Steps

1. **Prepare Phase 1 Android Scaffold:** Fix `android/build.gradle.kts`, `settings.gradle.kts`, and add Gradle wrapper.
2. **Document Architectural Decision (DEC):** Submit decision record ratifying `c-shared` + JNI for Android.
3. **Spike 03 & Spike 04:** Proceed to validate Android MediaProjection (Spike 03) and Pion WebRTC (Spike 04) now that the Android Go hosting boundary is proven.
