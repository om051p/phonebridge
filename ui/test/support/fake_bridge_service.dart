import 'dart:async';

import 'package:phonebridge_ui/models/capture_stats.dart';
import 'package:phonebridge_ui/models/clipboard_status.dart';
import 'package:phonebridge_ui/models/device_state.dart';
import 'package:phonebridge_ui/models/discovered_device.dart';
import 'package:phonebridge_ui/models/transfer_item.dart';
import 'package:phonebridge_ui/models/trusted_device.dart';
import 'package:phonebridge_ui/services/platform_bridge_service.dart';

/// Minimal controllable bridge for controller unit tests: the raw event
/// stream and trust list are driven by the test.
class FakeBridgeService implements PlatformBridgeService {
  final StreamController<Map<dynamic, dynamic>> rawEvents =
      StreamController<Map<dynamic, dynamic>>.broadcast();

  List<TrustedDevice> trustedDevices = [];
  int getTrustedDevicesCalls = 0;

  @override
  bool get isAndroid => true;

  @override
  bool get isLinux => false;

  @override
  Future<DeviceState> getDeviceState() async => DeviceState.initial;

  @override
  Future<CaptureStats> getMediaStats() async => CaptureStats.initial;

  @override
  Future<List<TrustedDevice>> getTrustedDevices() async {
    getTrustedDevicesCalls++;
    return List<TrustedDevice>.of(trustedDevices);
  }

  @override
  Future<bool> revokeDevice(String deviceId) async => false;

  @override
  Future<bool> removeDevice(String deviceId) async => false;

  @override
  Future<ClipboardStatus> getClipboardStatus() async => ClipboardStatus.initial;

  @override
  Future<bool> triggerClipboardPull() async => false;

  @override
  Future<bool> startCapture({
    String? receiverUrl,
    int width = 720,
    int height = 1600,
    int fps = 30,
    int bitrateKbps = 2500,
  }) async =>
      false;

  @override
  Future<bool> stopCapture() async => false;

  @override
  Future<List<DiscoveredDevice>> listDiscoveredDevices() async => const [];

  @override
  Future<PairingResult?> pairDevice(String deviceId) async => null;

  @override
  Future<bool> confirmPairing({
    required String deviceId,
    required bool confirmed,
  }) async =>
      false;

  @override
  Stream<CaptureStats> get statsStream => const Stream<CaptureStats>.empty();

  @override
  Stream<Map<dynamic, dynamic>> get rawEventsStream => rawEvents.stream;

  @override
  void setNativeCallHandler(Future<dynamic> Function(dynamic call)? handler) {
    nativeCallHandler = handler;
  }

  /// Last handler registered for native→Dart method calls (trustChanged, …).
  Future<dynamic> Function(dynamic call)? nativeCallHandler;

  @override
  void dispose() {}

  @override
  bool get supportsFileTransfer => false;

  @override
  Stream<TransferItem> get transferStream => const Stream<TransferItem>.empty();

  @override
  Future<List<TransferItem>> listTransfers() async => const [];

  @override
  Future<TransferSendResult> sendFile({
    required String localPath,
    String filename = '',
    String deviceId = '',
  }) async =>
      TransferSendResult.failure('unsupported');

  @override
  Future<bool> cancelTransfer(String transferId) async => false;
}
