# Spike 02-flutter-kotlin-go-android — Flutter/Kotlin ↔ Go Integration on Android

> Status: `EXPERIMENTAL` · Phase: 0 (framework only)


## Hypothesis
Go core can be embedded in the Android app and survive lifecycle without excessive battery/ANR cost.

## Question
Which embedding is most reliable: embedded Go as `c-shared` + JNI/UDS vs `gomobile` bind vs Kotlin host + Go core boundary vs other IPC?

## Target environment
- Android 13/14+ on physical device + emulator
- Flutter stable, Go 1.22+, NDK

## Minimal prototype
- Build Go as `c-shared` and as `gomobile` lib; expose `Ping`/`Echo`.
- Call from Kotlin and from Flutter (MethodChannel → Kotlin → Go).
- Exercise lifecycle: app backgrounded, killed, restarted; Work/ForegroundService cases.

## Success criteria
- < 50 ms call overhead; stable across background/foreground transitions.
- Debuggable, reproducible builds; no ANR on cold start.

## Failure criteria
- Packaging/lifecycle fragility (e.g. JNI crashes, class-loader issues, excessive binary size).
- Background kills that break the chosen IPC.

## Perf / security concerns
- Binary size, startup time, memory.
- Same-process vs cross-process trust boundary.

## Decision to be made
Lock the canonical Android embedding/Bridge and document build/debug flow.

