import 'dart:async';
import 'package:flutter/services.dart';
import '../models/device_state.dart';
import '../models/capture_stats.dart';

class PhoneBridgeChannel {
  static const MethodChannel _control = MethodChannel('dev.phonebridge/control');
  static const EventChannel _events = EventChannel('dev.phonebridge/events');

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

  Stream<CaptureStats> get statsStream {
    return _events.receiveBroadcastStream().map((event) {
      if (event is Map) {
        _lastStats = CaptureStats.fromMap(event, previous: _lastStats);
        return _lastStats;
      }
      return _lastStats;
    });
  }
}
