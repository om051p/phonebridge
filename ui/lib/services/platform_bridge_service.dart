import 'dart:async';
import '../models/device_state.dart';
import '../models/capture_stats.dart';
import '../models/trusted_device.dart';
import '../models/clipboard_status.dart';
import '../models/discovered_device.dart';
import 'transfer_backend.dart';

abstract class PlatformBridgeService implements TransferBackend {
  bool get isAndroid;
  bool get isLinux;

  Future<DeviceState> getDeviceState();
  Future<CaptureStats> getMediaStats();
  Future<List<TrustedDevice>> getTrustedDevices();
  Future<bool> revokeDevice(String deviceId);
  Future<bool> removeDevice(String deviceId);
  Future<ClipboardStatus> getClipboardStatus();
  Future<bool> triggerClipboardPull();

  Future<bool> startCapture({
    String? receiverUrl,
    int width = 720,
    int height = 1600,
    int fps = 30,
    int bitrateKbps = 2500,
  });
  Future<bool> stopCapture();

  Future<List<DiscoveredDevice>> listDiscoveredDevices();
  Future<PairingResult?> pairDevice(String deviceId);
  Future<bool> confirmPairing({required String deviceId, required bool confirmed});

  Stream<CaptureStats> get statsStream;
  Stream<Map<dynamic, dynamic>> get rawEventsStream;

  void setNativeCallHandler(Future<dynamic> Function(dynamic call)? handler);

  void dispose() {}
}
