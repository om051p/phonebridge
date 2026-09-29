import 'dart:async';
import 'package:flutter/services.dart';
import '../models/capture_stats.dart';
import '../models/clipboard_status.dart';
import '../models/device_state.dart';
import '../models/discovered_device.dart';
import '../models/transfer_item.dart';
import '../models/trusted_device.dart';
import 'phonebridge_channel.dart';
import 'platform_bridge_service.dart';

class AndroidBridgeService implements PlatformBridgeService {
  AndroidBridgeService({PhoneBridgeChannel? channel})
      : _channel = channel ?? PhoneBridgeChannel();

  final PhoneBridgeChannel _channel;

  @override
  bool get isAndroid => true;

  @override
  bool get isLinux => false;

  @override
  Future<DeviceState> getDeviceState() => _channel.getDeviceState();

  @override
  Future<CaptureStats> getMediaStats() => _channel.getMediaStats();

  @override
  Future<List<TrustedDevice>> getTrustedDevices() => _channel.getTrustedDevices();

  @override
  Future<bool> revokeDevice(String deviceId) => _channel.revokeDevice(deviceId);

  @override
  Future<bool> removeDevice(String deviceId) => _channel.removeDevice(deviceId);

  @override
  Future<ClipboardStatus> getClipboardStatus() => _channel.getClipboardStatus();

  @override
  Future<bool> triggerClipboardPull() => _channel.triggerClipboardPull();

  @override
  Future<bool> startCapture({
    String? receiverUrl,
    int width = 720,
    int height = 1600,
    int fps = 30,
    int bitrateKbps = 2500,
  }) {
    return _channel.startCapture(
      receiverUrl: receiverUrl,
      width: width,
      height: height,
      fps: fps,
      bitrateKbps: bitrateKbps,
    );
  }

  @override
  Future<bool> stopCapture() => _channel.stopCapture();

  /// LAN peers found by the Go core's mDNS browse.
  ///
  /// The phone is the browsing side of the pair: it discovers the Linux
  /// desktop, so the Devices tab can offer the PC for pairing and connection.
  /// A malformed or unaddressable entry is dropped here rather than rendered,
  /// because a row without a dialable host could only offer an action that
  /// fails (see [DiscoveredDevice.signalingUrl]).
  @override
  Future<List<DiscoveredDevice>> listDiscoveredDevices() async {
    final rows = await _channel.getDiscoveredDevices();
    return rows
        .map(DiscoveredDevice.fromMap)
        .where((d) => d.id.isNotEmpty && d.host.isNotEmpty && d.port > 0)
        .toList();
  }

  @override
  Future<PairingResult?> pairDevice(String deviceId) async => null;

  @override
  Future<bool> confirmPairing({required String deviceId, required bool confirmed}) async => false;

  /// Opens the screen where the companion IME is enabled (Android only).
  Future<bool> openInputMethodSettings() => _channel.openInputMethodSettings();

  /// Opens the system keyboard picker so the companion IME can be selected.
  Future<bool> showInputMethodPicker() => _channel.showInputMethodPicker();

  /// Enables or disables clipboard synchronization in the native adapter.
  Future<bool> setClipboardSyncEnabled(bool enabled) =>
      _channel.setClipboardSyncEnabled(enabled);

  @override
  Stream<CaptureStats> get statsStream => _channel.statsStream;

  // -------------------------------------------------------------------------
  // File transfers (DEC-024, Phase 4)
  // -------------------------------------------------------------------------
  //
  // MainActivity now forwards sendFile/listTransfers/cancelTransfer to Go and
  // pushes {"transfer": ...} events on dev.phonebridge/events, so the Android
  // backend reports transfer support. The pass-through below still degrades
  // gracefully (unavailable results, empty history) when the native side cannot
  // answer — a mocked channel exercises exactly that path in the tests.
  @override
  bool get supportsFileTransfer => true;

  @override
  Stream<TransferItem> get transferStream => _channel.transferStream;

  @override
  Future<List<TransferItem>> listTransfers() => _channel.listTransfers();

  @override
  Future<TransferSendResult> sendFile({
    required String localPath,
    String filename = '',
    String deviceId = '',
  }) {
    return _channel.sendFile(
      localPath: localPath,
      filename: filename,
      deviceId: deviceId,
    );
  }

  @override
  Future<bool> cancelTransfer(String transferId) => _channel.cancelTransfer(transferId);

  @override
  Stream<Map<dynamic, dynamic>> get rawEventsStream => _channel.rawEventsStream;

  @override
  void setNativeCallHandler(Future<dynamic> Function(dynamic call)? handler) {
    if (handler == null) {
      PhoneBridgeChannel.setMethodCallHandler(null);
    } else {
      PhoneBridgeChannel.setMethodCallHandler((MethodCall call) => handler(call));
    }
  }

  // -------------------------------------------------------------------------
  // Onboarding & Permissions (M-BETA-1)
  // -------------------------------------------------------------------------

  Future<Map<String, dynamic>> getPermissionsStatus() => _channel.getPermissionsStatus();
  Future<bool> requestNotificationPermission() => _channel.requestNotificationPermission();
  Future<bool> openNotificationListenerSettings() => _channel.openNotificationListenerSettings();
  Future<bool> openAccessibilitySettings() => _channel.openAccessibilitySettings();
  Future<bool> openAppDetailsSettings() => _channel.openAppDetailsSettings();

  @override
  void dispose() {}
}
