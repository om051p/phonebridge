# Handover — Phase 4 Step 5: Flutter Transfer UI (RESUME POINT)

Branch: `feat/phase4-file-transfer` (unpushed). Steps 0–4 are complete and verified but **uncommitted** — commit them first before starting Step 5.
Flutter toolchain: `/usr/local/bin/flutter` / `/usr/local/bin/dart`. App module: `ui/`.

## Status of Step 5

Inspection is **DONE** (full read-only pass). **No Step 5 code has been written yet.**
Everything below was verified directly from the repo — re-verify nothing unless a file changed.

## What Step 5 must build (from the task)

Send-file action, transfer list (filename/direction/peer/state/progress/bytes/reason/timestamp),
live progress via TransferEvent (no polling as primary), cancel for active transfers, history
hydration from ListTransfers merged with events, typed error/empty states, Material 3 UI matching
the existing design, integrated into existing navigation, plus the listed widget/unit tests.
Run `flutter analyze` + full `flutter test` at the end. Do NOT change proto/engine/JNI/backends.

## Inspection findings (all verified)

### 1. Where the transfer UI integrates
- `lib/screens/app_scaffold.dart` — `NavigationBar` with exactly 5 destinations:
  Home, Devices, Screen, Clipboard, Activity (`_currentIndex.clamp(0, 4)` in `onNavigateTab`).
  Screens array built at line ~128: `[HomeScreen, DevicesScreen, ScreenSharingScreen, ClipboardScreen, ActivityScreen]`.
- **Decision: integrate transfers into `ActivityScreen`** (it is the history surface; no 6th tab,
  no new route). Add a transfers section/segment + "Send file" action there, and optionally a
  small transfers card on HomeScreen. If a separate full screen is wanted, push it as a route
  from Activity (like `SettingsScreen`/`DiagnosticsScreen` are pushed) — do NOT add a 6th tab
  (that would break `clamp(0, 4)` and the native `onNavigateTab` contract).

### 2. Who owns SendFile/CancelTransfer/ListTransfers
- **Linux**: `lib/services/local_ipc_client.dart` (LocalIpcClient, UDS+gRPC, token/retry handled)
  currently has `sendFile`/`cancelTransfer`/`listTransfers` **missing** — add three methods:
  - `Future<SendFileResponse> sendFile({String deviceId = '', required String localPath, String filename = ''})`
    → `_callWithAuth((opts) => _service.sendFile(SendFileRequest()..deviceId=..localPath=..filename=.., options: opts))`
  - `Future<CancelTransferResponse> cancelTransfer(String transferId)`
  - `Future<ListTransfersResponse> listTransfers()`
  Generated stubs already exist in `lib/generated/phonebridge/localipc/v1/local_ipc.pbgrpc.dart`
  (SendFileRequest/CancelTransferRequest/ListTransfersRequest at lines ~160-180, ~256-270).
- **Platform abstraction**: `lib/services/platform_bridge_service.dart` (abstract) → add
  transfer methods + a transfer events stream; implement in:
  - `linux_bridge_service.dart` (has `_client`, `_initStream()` handles StreamEventsResponse:
    currently reads `hasSessionEvent()`/`hasClipboardEvent()` — **add `hasTransferEvent()` branch**
    pushing a typed map or the proto object onto a new broadcast controller).
  - `android_bridge_service.dart` (MethodChannel `dev.phonebridge/control` via
    `phonebridge_channel.dart`). **Note:** the Android platform channel (MainActivity) does NOT yet
    expose transfer methods or a transfer event push (GoBridge.transferSend/List/Stats exist in
    Kotlin but are not wired to the channel). Either add pass-through channel methods (Kotlin edit,
    small) or scope the Android send-file UI behind graceful "not yet available on this device"
    handling for this step. Prefer: implement the service methods calling new channel methods
    `sendFile`/`cancelTransfer`/`listTransfers`, and in the test mocks supply them; the Kotlin
    channel wiring can be a follow-up without blocking Step 5 tests.

### 3. StreamEvents exposure to Flutter
- Linux: `LocalIpcClient.streamEvents()` → broadcast; `LinuxBridgeService._initStream()` maps
  `StreamEventsResponse` into `Map<dynamic,dynamic>` raw events + `statsStream`. TransferEvent
  arrives on the SAME stream as `resp.transferEvent` (field `transfer` of `TransferInfo`).
  Add: `Stream<TransferEvent> get transferStream` (broadcast) on LinuxBridgeService and on
  PlatformBridgeService; on Android, surface it via the existing `rawEventsStream` map shape
  (e.g. key `'transfer'`) or a dedicated EventChannel later.
- TransferInfo proto getters (generated, verified): `transferId, direction, state, peerDeviceId,
  filename, mimeType, sizeBytes (Int64), bytesTransferred (Int64), startedAtMs, finishedAtMs,
  reasonCode, errorMessage, savedName`.
- Enums in `lib/generated/phonebridge/localipc/v1/local_ipc.pbenum.dart`:
  `TransferDirection{UNSPECIFIED,OUTBOUND,INBOUND}`,
  `TransferState{UNSPECIFIED,PENDING,ACTIVE,VERIFYING,COMPLETE,CANCELLED,FAILED}`,
  `TransferReason{UNSPECIFIED,NONE,NO_SESSION,UNSUPPORTED_PEER,BUSY,UNSAFE_FILENAME,TOO_LARGE,
  CHECKSUM_MISMATCH,STORAGE_FAILED,INTERRUPTED,CANCELLED_BY_PEER,CANCELLED_BY_USER,PROTOCOL_ERROR,
  INCOMPATIBLE_VERSION}`.

### 4. File picker
- **No file_picker dependency exists** (pubspec deps: flutter, cupertino_icons, protobuf, grpc,
  fixnum only). No existing picker abstraction.
- Options: (a) add `file_picker` to pubspec (new dep — acceptable, it's UI-only), or (b) Linux-only
  minimal path: a text field for an absolute path + Send (works with daemon SendFile which takes
  absolute local paths). Recommend (a) `file_picker` on both platforms with (b) as fallback UI on
  Linux if the plugin misbehaves on the desktop build. Keep it in the UI layer only.

### 5. Controller pattern to follow
- `lib/controllers/phonebridge_controller.dart` — `ChangeNotifier`, `ListenableBuilder` in screens,
  `initialize()` → `refreshAll()` + `_subscribeEvents()` (single subscription to
  `service.rawEventsStream`, guarded by `_rawEventsSub?.cancel()` first). Activity events live in
  `List<ActivityEvent> _activityEvents` surfaced via `List.unmodifiable`.
- **Add** `lib/controllers/transfer_controller.dart` (or extend PhoneBridgeController): hold
  `LinkedHashMap<String, TransferItem>` (id → item, newest first), `hydrate()` from listTransfers,
  `applyEvent(TransferInfo)` merge-by-id, `inFlight` set for cancel button disabling, subscription
  lifecycle cancel in `dispose()`.

### 6. UI patterns/design tokens
- Material 3 (`ThemeData(useMaterial3: true)` in tests); screens use `Theme.of(context)`,
  `Card` with `surfaceContainerHighest.withValues(alpha:0.35)` + `outlineVariant` borders,
  `colorScheme.error` for failures, section-card methods like `_buildStatusCard` (see
  `clipboard_screen.dart` lines ~49-90). Activity tiles use `ListTile`-style rows with dividers
  (activity_screen.dart `_buildEventTile`). Follow these exactly; no new visual system.

### 7. Test conventions
- `ui/test/`: `screens_test.dart` (widget tests: mock MethodChannel via
  `TestDefaultBinaryMessengerBinding...setMockMethodCallHandler`, `pumpWidget` inside
  `MaterialApp(theme: ThemeData(useMaterial3: true))`, `tester.view.physicalSize` for responsive
  checks), `local_ipc_client_test.dart`, `desktop_bridge_test.dart`, `smoke_test.dart`.
- Write `test/transfer_controller_test.dart` (pure Dart: hydration/merge/dedup/cancel-state) and
  `test/transfer_ui_test.dart` (widget: empty state, rows, progress rendering, failure text,
  responsive wide layout, navigation away/back without duplicate subscriptions).

## Suggested file plan (minimal)

1. `lib/models/transfer_item.dart` — plain model wrapping TransferInfo (direction/state/progress
   getters, human-readable reason text mapping the typed enum values; do NOT alter proto).
2. `lib/services/local_ipc_client.dart` — add `sendFile`, `cancelTransfer`, `listTransfers`.
3. `lib/services/platform_bridge_service.dart` + `linux_bridge_service.dart`
   (+ `android_bridge_service.dart` pass-through) — `Future<List<TransferItem>> listTransfers()`,
   `Future<TransferSendResult> sendFile(path, name, deviceId)`, `Future<bool> cancelTransfer(id)`,
   `Stream<TransferItem> get transferStream`.
4. `lib/controllers/transfer_controller.dart` — hydration, event merge with duplicate-id
   suppression, cancel-request dedup (`_cancelRequested` set), dispose-safe subscription.
5. `lib/screens/activity_screen.dart` — add "Transfers" filter/segment + transfer rows +
   Send-file button + empty/error states (no paired device / no session / no transfers /
   backend unavailable).
6. `lib/screens/home_screen.dart` — optional small transfers summary card (reuse components).
7. Tests per conventions above.

## Key state mapping for the UI (from DEC-024 enums)

- Active/cancellable: PENDING, ACTIVE (VERIFYING = not cancellable locally, show "Verifying…").
- Terminal: COMPLETE (green), CANCELLED (grey), FAILED (red + reason text).
- Reason text: NO_SESSION→"No active session", UNSUPPORTED_PEER→"Peer does not support transfers",
  BUSY→"Another transfer is in flight", UNSAFE_FILENAME→"Unsafe filename",
  TOO_LARGE→"File exceeds the size limit", CHECKSUM_MISMATCH→"Integrity check failed",
  STORAGE_FAILED→"Storage error", INTERRUPTED→"Connection interrupted", CANCELLED_BY_PEER→
  "Cancelled by peer", CANCELLED_BY_USER→"Cancelled", PROTOCOL_ERROR→"Protocol error",
  INCOMPATIBLE_VERSION→"Incompatible device".
- Progress = bytesTransferred/sizeBytes (Int64 → use `.toInt()`); only show when sizeBytes > 0.
  No speed/ETA (backend does not provide it — do not fabricate).

## Verification commands

```bash
cd ui && flutter analyze
cd ui && flutter test
```

Existing suite must stay green: smoke_test, screens_test, desktop_bridge_test,
local_ipc_client_test, session_status_test.

## Constraints reminder

No changes to: proto files, Go engine, Android TransferHost/JNI, Linux backend, chunk limits,
pairing. UI + service/controller glue + tests only. Do not push commits.
