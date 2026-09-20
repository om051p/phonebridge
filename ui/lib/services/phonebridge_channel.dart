import 'dart:async';
import 'package:flutter/services.dart';
import '../models/device_state.dart';
import '../models/capture_stats.dart';
import '../models/trusted_device.dart';
import '../models/clipboard_status.dart';

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

  Stream<CaptureStats> get statsStream {
    return _events.receiveBroadcastStream().map((event) {
      if (event is Map) {
        _lastStats = CaptureStats.fromMap(event, previous: _lastStats);
        return _lastStats;
      }
      return _lastStats;
    });
  }

  Stream<Map<dynamic, dynamic>> get rawEventsStream {
    return _events.receiveBroadcastStream().where((event) => event is Map).cast<Map<dynamic, dynamic>>();
  }
}
