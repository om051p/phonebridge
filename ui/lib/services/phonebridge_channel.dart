import 'dart:async';
import 'package:flutter/services.dart';
import '../models/device_state.dart';
import '../models/capture_stats.dart';
import '../models/inbound_pairing.dart';
import '../models/trusted_device.dart';
import '../models/clipboard_status.dart';
import '../models/transfer_item.dart';

class PhoneBridgeChannel {
  static const MethodChannel _control = MethodChannel('dev.phonebridge/control');
  static const EventChannel _events = EventChannel('dev.phonebridge/events');

  static void setMethodCallHandler(Future<dynamic> Function(MethodCall call)? handler) {
    _control.setMethodCallHandler(handler);
  }

  CaptureStats _lastStats = CaptureStats.initial;

  Future<DeviceState> getDeviceState() async {
    try {
      final res = await _control.invokeMethod<Map<dynamic, dynamic>>('getDeviceState');
      if (res != null) {
        return DeviceState.fromMap(res);
      }
    } on PlatformException {
      // Fallback for non-Android host or unattached engine
    }
    return DeviceState.initial;
  }

  Future<bool> startCapture({
    String? receiverUrl,
    int width = 720,
    int height = 1600,
    int fps = 30,
    int bitrateKbps = 2500,
  }) async {
    try {
      final res = await _control.invokeMethod<bool>('startCapture', {
        if (receiverUrl != null && receiverUrl.isNotEmpty) 'receiverUrl': receiverUrl,
        'width': width,
        'height': height,
        'fps': fps,
        'bitrateKbps': bitrateKbps,
      });
      return res ?? false;
    } on PlatformException {
      rethrow;
    }
  }

  Future<bool> stopCapture() async {
    try {
      final res = await _control.invokeMethod<bool>('stopCapture');
      return res ?? false;
    } on PlatformException {
      rethrow;
    }
  }

  Future<CaptureStats> getMediaStats() async {
    try {
      final res = await _control.invokeMethod<Map<dynamic, dynamic>>('getMediaStats');
      if (res != null) {
        _lastStats = CaptureStats.fromMap(res, previous: _lastStats);
        return _lastStats;
      }
    } on PlatformException {
      // Fallback
    }
    return _lastStats;
  }

  Future<List<TrustedDevice>> getTrustedDevices() async {
    try {
      final res = await _control.invokeMethod<List<dynamic>>('getTrustedDevices');
      if (res != null) {
        return res
            .whereType<Map<dynamic, dynamic>>()
            .map((m) => TrustedDevice.fromMap(m))
            .toList();
      }
    } on PlatformException {
      // Fallback
    }
    return [];
  }

  Future<bool> revokeDevice(String deviceId) async {
    try {
      final res = await _control.invokeMethod<bool>('revokeDevice', {'deviceId': deviceId});
      return res ?? false;
    } on PlatformException {
      return false;
    }
  }

  Future<bool> removeDevice(String deviceId) async {
    try {
      final res = await _control.invokeMethod<bool>('removeDevice', {'deviceId': deviceId});
      return res ?? false;
    } on PlatformException {
      return false;
    }
  }

  Future<ClipboardStatus> getClipboardStatus() async {
    try {
      final res = await _control.invokeMethod<Map<dynamic, dynamic>>('getClipboardStatus');
      if (res != null) {
        return ClipboardStatus.fromMap(res);
      }
    } on PlatformException {
      // Fallback
    }
    return ClipboardStatus.initial;
  }

  Future<bool> triggerClipboardPull() async {
    try {
      final res = await _control.invokeMethod<bool>('triggerClipboardPull');
      return res ?? false;
    } on PlatformException {
      return false;
    }
  }

  /// Lists LAN peers found by the Go core's mDNS browse (DEC-007).
  ///
  /// The Android app is the browsing side of the pair here: it discovers the
  /// Linux desktop, which is what makes it possible to pair and connect from the
  /// phone at all. An empty list is a valid answer (nothing on the LAN yet), so
  /// failures degrade to an empty list rather than propagating.
  Future<List<Map<dynamic, dynamic>>> getDiscoveredDevices() async {
    try {
      final res = await _control.invokeMethod<List<dynamic>>('getDiscoveredDevices');
      if (res != null) {
        return res.whereType<Map<dynamic, dynamic>>().toList();
      }
    } on MissingPluginException {
      return const [];
    } on PlatformException {
      return const [];
    }
    return const [];
  }

  /// Reads the encoder-selected screen capabilities (Android
  /// `CodecSelector` via `getMediaCapabilities`). Null when the platform
  /// cannot answer (missing plugin, old build, read failure) — Flutter
  /// renders "Checking…" instead of guessing.
  Future<Map<String, dynamic>?> getMediaCapabilities() async {
    try {
      final res = await _control.invokeMethod<Map<dynamic, dynamic>>('getMediaCapabilities');
      if (res != null) {
        return res.cast<String, dynamic>();
      }
    } on MissingPluginException {
      return null;
    } on PlatformException {
      return null;
    }
    return null;
  }

  /// Opens the platform screen where the companion IME is enabled. Ambient
  /// clipboard observation is impossible without it (Spike 05 / DEC-023).
  Future<bool> openInputMethodSettings() async {
    try {
      final res = await _control.invokeMethod<bool>('openInputMethodSettings');
      return res ?? false;
    } on PlatformException {
      return false;
    }
  }

  /// Opens the system keyboard picker so the companion IME can be selected.
  Future<bool> showInputMethodPicker() async {
    try {
      final res = await _control.invokeMethod<bool>('showInputMethodPicker');
      return res ?? false;
    } on PlatformException {
      return false;
    }
  }

  /// Enables or disables clipboard synchronization in the native adapter.
  Future<bool> setClipboardSyncEnabled(bool enabled) async {
    try {
      final res = await _control.invokeMethod<bool>('setClipboardSyncEnabled', {
        'enabled': enabled,
      });
      return res ?? false;
    } on PlatformException {
      return false;
    }
  }

  // One subscription to the native events channel, fanned out to every
  // consumer. Calling receiveBroadcastStream() per getter/access registers a
  // fresh binary-message handler per subscription: the last subscriber wins
  // the native sink and the first cancellation removes it for everyone, so
  // session/stats events were silently lost while a transfer subscription was
  // live (measured on device: the screen tab froze at "READY — No session
  // running" during an active stream). The controller below keeps exactly one
  // native subscription: it opens on the first consumer, closes when the last
  // consumer leaves (native sink released — no leaks), drops events while
  // nobody listens (no queue growth), and every consumer receives its own
  // copy of each event.
  StreamController<Map<dynamic, dynamic>>? _fanout;
  StreamSubscription<Map<dynamic, dynamic>>? _nativeEventsSub;

  Stream<Map<dynamic, dynamic>> get _eventFanout {
    final existing = _fanout;
    if (existing != null) {
      return existing.stream;
    }
    final controller = StreamController<Map<dynamic, dynamic>>.broadcast(
      onCancel: () {
        _nativeEventsSub?.cancel();
        _nativeEventsSub = null;
        _fanout = null;
      },
    );
    _fanout = controller;
    _nativeEventsSub = _events
        .receiveBroadcastStream()
        .where((event) => event is Map)
        .cast<Map<dynamic, dynamic>>()
        .listen(controller.add, onError: controller.addError);
    return controller.stream;
  }

  Stream<CaptureStats> get statsStream {
    CaptureStats previous = _lastStats;
    return _eventFanout.map((event) {
      previous = CaptureStats.fromMap(event, previous: previous);
      _lastStats = previous;
      return previous;
    });
  }

  Stream<Map<dynamic, dynamic>> get rawEventsStream => _eventFanout;

  // -------------------------------------------------------------------------
  // File transfers (DEC-024, Phase 4)
  // -------------------------------------------------------------------------
  //
  // Pass-through to the control channel methods MainActivity forwards to Go
  // (sendFile/listTransfers/cancelTransfer). Missing or failing native handlers
  // still fall through to the graceful "unavailable" results.

  /// Native-pushed transfer transitions. MainActivity emits the same event-map
  /// shape as the other events (key `transfer`) on dev.phonebridge/events; the
  /// stream stays silent while no transfer activity is being reported.
  Stream<TransferItem> get transferStream => _eventFanout
      .where((event) => event['transfer'] != null)
      .map((event) => _toTransferItem(event['transfer']))
      .where((item) => item != null)
      .cast<TransferItem>();

  Future<List<TransferItem>> listTransfers() async {
    try {
      final res = await _control.invokeMethod<List<dynamic>>('listTransfers');
      return _toTransferItems(res);
    } on MissingPluginException {
      return const <TransferItem>[];
    } on PlatformException {
      return const <TransferItem>[];
    }
  }

  Future<TransferSendResult> sendFile({
    required String localPath,
    String filename = '',
    String deviceId = '',
  }) async {
    try {
      final res = await _control.invokeMethod<Map<dynamic, dynamic>>('sendFile', {
        'localPath': localPath,
        if (filename.isNotEmpty) 'filename': filename,
        if (deviceId.isNotEmpty) 'deviceId': deviceId,
      });
      if (res != null) {
        return TransferSendResult.fromMap(res);
      }
      return TransferSendResult.failure(kTransferUnavailableMessage);
    } on MissingPluginException {
      return TransferSendResult.failure(kTransferUnavailableMessage);
    } on PlatformException catch (e) {
      return TransferSendResult.failure(e.message ?? e.code);
    }
  }

  Future<bool> cancelTransfer(String transferId) async {
    try {
      final res = await _control.invokeMethod<bool>('cancelTransfer', {
        'transferId': transferId,
      });
      return res ?? false;
    } on MissingPluginException {
      return false;
    } on PlatformException {
      return false;
    }
  }

  // -------------------------------------------------------------------------
  // Bidirectional pairing (Phase 2)
  // -------------------------------------------------------------------------

  /// Requests pairing with the peer at [endpoint] ("host:port"). Returns the
  /// native outcome map: `{status: ready, token, deviceName, sas}` leads to
  /// the SAS dialog; `{status: alreadyTrusted}` means the peer already trusts
  /// this device; `{status: failed, message}` is a refusal.
  Future<Map<dynamic, dynamic>?> requestPairing({required String endpoint}) async {
    try {
      return await _control.invokeMethod<Map<dynamic, dynamic>>(
        'pairDevice',
        {'endpoint': endpoint},
      );
    } on MissingPluginException {
      return {'status': 'failed', 'message': 'Pairing is unavailable on this build'};
    } on PlatformException catch (e) {
      return {'status': 'failed', 'message': e.message ?? e.code};
    }
  }

  /// Confirms (or rejects) a pending outbound pairing identified by [token].
  /// Runs the full confirm/poll exchange natively; true only on 200 paired.
  Future<bool> confirmPairingToken({required String token, required bool confirmed}) async {
    try {
      final res = await _control.invokeMethod<bool>(
        'confirmPairing',
        {'token': token, 'confirmed': confirmed},
      );
      return res ?? false;
    } on MissingPluginException {
      return false;
    } on PlatformException {
      return false;
    }
  }

  /// Pairing requests awaiting THIS device's approval. Empty on any failure —
  /// the list is transient state, not truth worth an error banner.
  Future<List<InboundPairing>> listInboundPairings() async {
    try {
      final res = await _control.invokeMethod<List<dynamic>>('listInboundPairings');
      if (res != null) {
        return res
            .whereType<Map<dynamic, dynamic>>()
            .map(InboundPairing.fromMap)
            .toList();
      }
    } on MissingPluginException {
      // Old build or unit-test host: no inbound surface.
    } on PlatformException {
      // Fall through to empty.
    }
    return [];
  }

  /// Records the local user's decision on an inbound pairing request.
  Future<bool> respondInboundPairing({required String token, required bool approved}) async {
    try {
      final res = await _control.invokeMethod<bool>('respondInboundPairing', {
        'token': token,
        'approved': approved,
      });
      return res ?? false;
    } on MissingPluginException {
      return false;
    } on PlatformException {
      return false;
    }
  }

  // -------------------------------------------------------------------------
  // Onboarding & Permissions (M-BETA-1)
  // -------------------------------------------------------------------------

  Future<Map<String, dynamic>> getPermissionsStatus() async {
    try {
      final res = await _control.invokeMethod<Map<dynamic, dynamic>>('getPermissionsStatus');
      if (res != null) {
        return res.cast<String, dynamic>();
      }
    } on MissingPluginException {
      return const {};
    } on PlatformException {
      return const {};
    }
    return const {};
  }

  Future<bool> requestNotificationPermission() async {
    try {
      final res = await _control.invokeMethod<bool>('requestNotificationPermission');
      return res ?? false;
    } on MissingPluginException {
      return false;
    } on PlatformException {
      return false;
    }
  }

  Future<bool> openNotificationListenerSettings() async {
    try {
      final res = await _control.invokeMethod<bool>('openNotificationListenerSettings');
      return res ?? false;
    } on MissingPluginException {
      return false;
    } on PlatformException {
      return false;
    }
  }

  Future<bool> openAccessibilitySettings() async {
    try {
      final res = await _control.invokeMethod<bool>('openAccessibilitySettings');
      return res ?? false;
    } on MissingPluginException {
      return false;
    } on PlatformException {
      return false;
    }
  }

  Future<bool> openAppDetailsSettings() async {
    try {
      final res = await _control.invokeMethod<bool>('openAppDetailsSettings');
      return res ?? false;
    } on MissingPluginException {
      return false;
    } on PlatformException {
      return false;
    }
  }
}

TransferItem? _toTransferItem(Object? raw) {
  if (raw is TransferItem) return raw;
  if (raw is Map) return TransferItem.fromMap(raw);
  return null;
}

List<TransferItem> _toTransferItems(List<dynamic>? raw) {
  if (raw == null) return const <TransferItem>[];
  return raw.whereType<Map<dynamic, dynamic>>().map(TransferItem.fromMap).toList();
}
