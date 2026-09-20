import 'dart:async';
import 'package:flutter/services.dart';
import '../models/capture_stats.dart';
import '../models/clipboard_status.dart';
import '../models/device_state.dart';
import '../models/discovered_device.dart';
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

  @override
  Future<List<DiscoveredDevice>> listDiscoveredDevices() async => [];

  @override
  Future<PairingResult?> pairDevice(String deviceId) async => null;

  @override
  Future<bool> confirmPairing({required String deviceId, required bool confirmed}) async => false;

  @override
  Stream<CaptureStats> get statsStream => _channel.statsStream;

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

  @override
  void dispose() {}
}
