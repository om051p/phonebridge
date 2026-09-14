# Spike 05-android-clipboard-background — Android Background Clipboard Strategy

> Status: `EXPERIMENTAL` · Phase: 0 (framework only)


## Hypothesis
Bidirectional clipboard sync is feasible with graceful limits on modern Android.

## Question
What can run in background on Android 13/14+ (restrictions around background clipboard read), and is companion IME the right fallback?

## Target environment
- Android 13/14+ physical devices (stock + OEM variants)

## Minimal prototype
- ForegroundService + clipboard listener; measure background read/write success across states (foreground/background/killed).
- Prototype companion IME input path.

## Success criteria
- Clear matrix of what works when; fallback defined and user-understandable.

## Failure criteria
- Universal background sync claimed without evidence would be a false promise.

## Perf / security concerns
- Only sync on explicit user action where required; no polling clipboard in background.

## Decision to be made
Define supported clipboard UX and fallback behavior (IME vs limited sync).

