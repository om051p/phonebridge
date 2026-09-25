// Shared fakes for the file-transfer tests (DEC-024, Phase 4).
//
// Not a test file itself: `flutter test` only picks up `*_test.dart`.

import 'dart:async';

import 'package:phonebridge_ui/generated/phonebridge/localipc/v1/local_ipc.pb.dart' as ipc;
import 'package:phonebridge_ui/generated/phonebridge/v1/phonebridge.pb.dart' as pb;
import 'package:phonebridge_ui/models/capture_stats.dart';
import 'package:phonebridge_ui/models/clipboard_status.dart';
import 'package:phonebridge_ui/models/device_state.dart';
import 'package:phonebridge_ui/models/discovered_device.dart';
import 'package:phonebridge_ui/models/transfer_item.dart';
import 'package:phonebridge_ui/models/trusted_device.dart';
import 'package:phonebridge_ui/services/platform_bridge_service.dart';
import 'package:phonebridge_ui/services/session_backend.dart';
import 'package:phonebridge_ui/services/transfer_backend.dart';

import 'fake_session_backend.dart';

/// In-memory [TransferBackend] that records every call and lets each failure mode
/// be switched on, so the controller and widget tests can drive exact states.
class FakeTransferBackend implements TransferBackend {
  FakeTransferBackend({this.supportsFileTransfer = true});

  @override
  final bool supportsFileTransfer;

  final StreamController<TransferItem> streamController =
      StreamController<TransferItem>.broadcast();

  /// What listTransfers() returns; assigned by the test before hydrating.
  List<TransferItem> history = const [];

  /// When set, listTransfers() throws it (backend unavailable state).
  Object? listError;

  int listCalls = 0;

  /// Result/exception of the next sendFile call.
  TransferSendResult sendResult =
      const TransferSendResult(transferId: 'tx-sent');
  Object? sendError;
  final List<Map<String, String>> sendCalls = <Map<String, String>>[];

  bool cancelResult = true;
  Object? cancelError;

  /// When set, cancelTransfer waits on it: lets a test observe the in-flight
  /// (button disabled) state before the daemon answers.
  Completer<bool>? cancelGate;

  final List<String> cancelCalls = <String>[];

  @override
  Stream<TransferItem> get transferStream => streamController.stream;

  @override
  Future<List<TransferItem>> listTransfers() async {
    listCalls++;
    if (listError != null) throw listError!;
    return List<TransferItem>.of(history);
  }

  @override
  Future<TransferSendResult> sendFile({
    required String localPath,
    String filename = '',
    String deviceId = '',
  }) async {
    sendCalls.add({
      'localPath': localPath,
      'filename': filename,
      'deviceId': deviceId,
    });
    if (sendError != null) throw sendError!;
    return sendResult;
  }

  @override
  Future<bool> cancelTransfer(String transferId) async {
    cancelCalls.add(transferId);
    if (cancelError != null) throw cancelError!;
    final gate = cancelGate;
    if (gate != null) return gate.future;
    return cancelResult;
  }

  Future<void> close() => streamController.close();
}

/// Minimal [PlatformBridgeService] whose device/clipboard surface is inert and
/// whose transfer/session surfaces are fakes.
///
/// It also implements [SessionBackend], which is how a platform that drives
/// sessions locally looks to [PhoneBridgeController].
class FakeBridgeService implements PlatformBridgeService, SessionBackend {
  FakeBridgeService({
    bool supportsFileTransfer = true,
    bool supportsSessions = true,
  })  : transfers =
            FakeTransferBackend(supportsFileTransfer: supportsFileTransfer),
        sessions = FakeSessionBackend(supportsSessions: supportsSessions);

  final FakeTransferBackend transfers;
  final FakeSessionBackend sessions;

  final StreamController<CaptureStats> _statsCtrl =
      StreamController<CaptureStats>.broadcast();
  final StreamController<Map<dynamic, dynamic>> _rawEventsCtrl =
      StreamController<Map<dynamic, dynamic>>.broadcast();

  @override
  bool get isAndroid => false;

  @override
  bool get isLinux => true;

  @override
  Future<DeviceState> getDeviceState() async => DeviceState.initial;

  @override
  Future<CaptureStats> getMediaStats() async => CaptureStats.initial;

  @override
  Future<List<TrustedDevice>> getTrustedDevices() async => const [];

  @override
  Future<bool> revokeDevice(String deviceId) async => true;

  @override
  Future<bool> removeDevice(String deviceId) async => true;

  @override
  Future<ClipboardStatus> getClipboardStatus() async => ClipboardStatus.initial;

  @override
  Future<bool> triggerClipboardPull() async => true;

  @override
  Future<bool> startCapture({
    String? receiverUrl,
    int width = 720,
    int height = 1600,
    int fps = 30,
    int bitrateKbps = 2500,
  }) async {
    return true;
  }

  @override
  Future<bool> stopCapture() async => true;

  @override
  Future<List<DiscoveredDevice>> listDiscoveredDevices() async => const [];

  @override
  Future<PairingResult?> pairDevice(String deviceId) async => null;

  @override
  Future<bool> confirmPairing({
    required String deviceId,
    required bool confirmed,
  }) async {
    return false;
  }

  @override
  Stream<CaptureStats> get statsStream => _statsCtrl.stream;

  @override
  Stream<Map<dynamic, dynamic>> get rawEventsStream => _rawEventsCtrl.stream;

  @override
  bool get supportsFileTransfer => transfers.supportsFileTransfer;

  @override
  Stream<TransferItem> get transferStream => transfers.transferStream;

  @override
  Future<List<TransferItem>> listTransfers() => transfers.listTransfers();

  @override
  Future<TransferSendResult> sendFile({
    required String localPath,
    String filename = '',
    String deviceId = '',
  }) {
    return transfers.sendFile(
      localPath: localPath,
      filename: filename,
      deviceId: deviceId,
    );
  }

  @override
  Future<bool> cancelTransfer(String transferId) =>
      transfers.cancelTransfer(transferId);

  // ---------------------------------------------------------------- sessions

  @override
  bool get supportsSessions => sessions.supportsSessions;

  @override
  String get activeSessionId => sessions.activeSessionId;

  @override
  Stream<ipc.SessionEvent> get sessionEventStream => sessions.sessionEventStream;

  @override
  Future<ipc.GetSessionStateResponse> getSessionSnapshot(String sessionId) =>
      sessions.getSessionSnapshot(sessionId);

  @override
  Future<ipc.StartSessionResponse> startSession({
    required String target,
    pb.MediaParams? requested,
  }) =>
      sessions.startSession(target: target, requested: requested);

  @override
  Future<ipc.StopSessionResponse> stopSession({
    required String sessionId,
    String reason = 'user stopped',
  }) =>
      sessions.stopSession(sessionId: sessionId, reason: reason);

  @override
  void setNativeCallHandler(Future<dynamic> Function(dynamic call)? handler) {}

  @override
  void dispose() {
    _statsCtrl.close();
    _rawEventsCtrl.close();
    transfers.close();
    sessions.close();
  }
}
