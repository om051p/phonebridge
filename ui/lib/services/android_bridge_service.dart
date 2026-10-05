import 'dart:async';
import 'package:flutter/services.dart';
import '../generated/phonebridge/v1/phonebridge.pb.dart' as pb;
import '../models/capture_stats.dart';
import '../models/clipboard_status.dart';
import '../models/device_state.dart';
import '../models/discovered_device.dart';
import '../models/inbound_pairing.dart';
import '../models/transfer_item.dart';
import '../models/trusted_device.dart';
import 'pairing_backend.dart';
import 'phonebridge_channel.dart';
import 'platform_bridge_service.dart';

class AndroidBridgeService implements PlatformBridgeService, PairingBackend {
  AndroidBridgeService({PhoneBridgeChannel? channel})
      : _channel = channel ?? PhoneBridgeChannel();

  final PhoneBridgeChannel _channel;

  /// Outbound pairing tokens keyed by device id: pairDevice is addressed by
  /// id (the UI's contract, same as Linux) while the native confirm is
  /// addressed by the token the peer issued. One token per device; a retry
  /// replaces it.
  final Map<String, String> _pendingTokens = {};

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
  Future<PairingResult?> pairDevice(String deviceId) async {
    // The phone dials the peer's signaling server directly, so the id must
    // resolve to a dialable endpoint from the current discovery records —
    // the same canonical resolution the session path uses. Bounded: a hung
    // discovery read must disable pairing, not freeze the pair button.
    final devices = await listDiscoveredDevices().timeout(
      const Duration(seconds: 5),
      onTimeout: () => const <DiscoveredDevice>[],
    );
    DiscoveredDevice? target;
    for (final d in devices) {
      if (d.id == deviceId && !d.isStale && d.signalingUrl.isNotEmpty) {
        target = d;
        break;
      }
    }
    if (target == null) return null;

    final endpoint = '${target.host}:${target.port}';
    final res = await _channel.requestPairing(endpoint: endpoint);
    if (res == null) return null;
    switch (res['status'] as String?) {
      case 'ready':
        final token = res['token'] as String?;
        if (token == null || token.isEmpty) return null;
        _pendingTokens[deviceId] = token;
        return PairingResult(
          deviceName: (res['deviceName'] as String?) ?? target.name,
          sasCode: (res['sas'] as String?) ?? '',
        );
      case 'alreadyTrusted':
        return const PairingResult(deviceName: '', sasCode: '', alreadyTrusted: true);
      default:
        return null;
    }
  }

  @override
  Future<bool> confirmPairing({required String deviceId, required bool confirmed}) async {
    final token = _pendingTokens.remove(deviceId);
    if (token == null) return false;
    return _channel.confirmPairingToken(token: token, confirmed: confirmed);
  }

  // -------------------------------------------------------------------------
  // Inbound pairing (Phase 2): this device's signaling server holds the
  // pending requests; the UI reads them and records the user's decision.
  // -------------------------------------------------------------------------

  @override
  Future<List<InboundPairing>> listInboundPairings() => _channel.listInboundPairings();

  @override
  Future<bool> respondInboundPairing({
    required String pairingToken,
    required bool approved,
  }) =>
      _channel.respondInboundPairing(token: pairingToken, approved: approved);

  /// Opens the screen where the companion IME is enabled (Android only).
  Future<bool> openInputMethodSettings() => _channel.openInputMethodSettings();

  /// Opens the system keyboard picker so the companion IME can be selected.
  Future<bool> showInputMethodPicker() => _channel.showInputMethodPicker();

  /// Encoder-selected screen capabilities for gating quality presets.
  ///
  /// Null means unknown (old build, missing plugin, read failure) — the UI
  /// renders "Checking…" rather than guessing. A present value with
  /// `supportsScreen == false` is an honest "this device cannot capture".
  Future<pb.MediaCapabilities?> getMediaCapabilities() async {
    final map = await _channel.getMediaCapabilities();
    if (map == null || map.isEmpty) return null;
    List<String> codecsOf(Object? raw) {
      if (raw is List) return raw.whereType<String>().toList();
      if (raw is String && raw.isNotEmpty) return [raw];
      return const [];
    }

    int intOf(Object? raw) => (raw as num?)?.toInt() ?? 0;
    return pb.MediaCapabilities(
      codecs: codecsOf(map['codecs']),
      maxWidth: intOf(map['maxWidth']),
      maxHeight: intOf(map['maxHeight']),
      maxFps: intOf(map['maxFps']),
      supportsScreen: map['supportsScreen'] as bool? ?? true,
    );
  }

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
