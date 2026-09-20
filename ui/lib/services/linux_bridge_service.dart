import 'dart:async';
import '../generated/phonebridge/localipc/v1/local_ipc.pbgrpc.dart' as ipc;
import '../models/capture_stats.dart';
import '../models/clipboard_status.dart';
import '../models/device_state.dart';
import '../models/discovered_device.dart';
import '../models/trusted_device.dart';
import 'local_ipc_client.dart';
import 'platform_bridge_service.dart';

class LinuxBridgeService implements PlatformBridgeService {
  LinuxBridgeService({LocalIpcClient? client})
      : _client = client ?? LocalIpcClient() {
    _initStream();
  }

  final LocalIpcClient _client;
  final StreamController<Map<dynamic, dynamic>> _rawEventsController =
      StreamController<Map<dynamic, dynamic>>.broadcast();
  final StreamController<CaptureStats> _statsController =
      StreamController<CaptureStats>.broadcast();

  StreamSubscription<ipc.StreamEventsResponse>? _streamSub;
  String? _activeSessionId;
  CaptureStats _lastStats = CaptureStats.initial;
  String _lastClipboardState = 'STOPPED';

  @override
  bool get isAndroid => false;

  @override
  bool get isLinux => true;

  void _initStream() {
    _streamSub = _client.streamEvents().listen(
      (resp) {
        final map = <dynamic, dynamic>{};

        if (resp.hasSessionEvent()) {
          final se = resp.sessionEvent;
          _activeSessionId = se.sessionId;
          final isStreaming = se.state == ipc.SessionState.SESSION_STATE_STREAMING;
          final isStopped = se.state == ipc.SessionState.SESSION_STATE_STOPPED ||
              se.state == ipc.SessionState.SESSION_STATE_FAILED;

          map['isCapturing'] = isStreaming;
          if (isStopped) {
            map['isCapturing'] = false;
          }
          if (se.state == ipc.SessionState.SESSION_STATE_FAILED) {
            map['lastError'] = se.reason;
          }

          _lastStats = _lastStats.copyWith(
            isCapturing: isStreaming,
          );
          _statsController.add(_lastStats);
        }

        if (resp.hasClipboardEvent()) {
          final ce = resp.clipboardEvent;
          _lastClipboardState = ce.state;
          map['clipboardState'] = ce.state;
        }

        if (map.isNotEmpty) {
          _rawEventsController.add(map);
        }
      },
      onError: (err) {
        _rawEventsController.add({
          'lastError': err.toString(),
        });
      },
    );
  }

  @override
  Future<DeviceState> getDeviceState() async {
    try {
      final h = await _client.health();
      return DeviceState(
        model: 'Linux Desktop',
        manufacturer: 'Linux (Daemon ${h.serverVersion})',
        sdkInt: 0,
        isCapturing: _lastStats.isCapturing,
        goEngineLoaded: true,
        codec: 'H.264',
        isHardwareCodec: false,
        deviceId: 'linux-desktop',
        displayName: 'Linux Desktop',
        clipboardState: _lastClipboardState,
        imeSelected: false,
      );
    } catch (_) {
      return const DeviceState(
        model: 'Linux Desktop',
        manufacturer: 'Linux (Daemon Offline)',
        sdkInt: 0,
        isCapturing: false,
        goEngineLoaded: false,
        codec: 'H.264',
        isHardwareCodec: false,
        deviceId: 'linux-desktop',
        displayName: 'Linux Desktop',
        clipboardState: 'UNAVAILABLE',
        imeSelected: false,
      );
    }
  }

  @override
  Future<CaptureStats> getMediaStats() async {
    try {
      if (_activeSessionId != null && _activeSessionId!.isNotEmpty) {
        final snap = await _client.getSessionState(sessionId: _activeSessionId!);
        final isStreaming = snap.state == ipc.SessionState.SESSION_STATE_STREAMING;
        final fps = snap.hasActual() && snap.actual.fps > 0 ? snap.actual.fps.toDouble() : null;
        _lastStats = _lastStats.copyWith(
          isCapturing: isStreaming,
          durationUs: (snap.connectedDurationMs * 1000).toInt(),
          currentFps: fps,
        );
      }
    } catch (_) {}
    return _lastStats;
  }

  @override
  Future<List<TrustedDevice>> getTrustedDevices() async {
    try {
      final resp = await _client.listTrustedDevices();
      return resp.devices
          .map((d) => TrustedDevice(
                deviceId: d.deviceId,
                displayName: d.displayName,
                platform: d.platform,
                pairedAtMs: d.pairedAtMs.toInt(),
                lastSeenMs: d.lastSeenMs.toInt(),
                revoked: d.revoked,
              ))
          .toList();
    } catch (_) {
      return [];
    }
  }

  @override
  Future<bool> revokeDevice(String deviceId) async {
    try {
      final resp = await _client.revokeDevice(deviceId);
      return resp.success;
    } catch (_) {
      return false;
    }
  }

  @override
  Future<bool> removeDevice(String deviceId) async {
    try {
      final resp = await _client.revokeDevice(deviceId);
      return resp.success;
    } catch (_) {
      return false;
    }
  }

  @override
  Future<ClipboardStatus> getClipboardStatus() async {
    try {
      final resp = await _client.getClipboardStatus();
      _lastClipboardState = resp.state;
      return ClipboardStatus(
        isEnabled: resp.state != 'STOPPED' && resp.state != 'UNAVAILABLE',
        state: ClipboardSyncState.fromString(resp.state),
        maxPayloadSize: resp.maxPayloadSize > 0 ? resp.maxPayloadSize : 786432,
        syncCount: 0,
        lastSyncTimestampMs: resp.lastSyncMs.toInt(),
        lastDirection: null,
        imeSelected: false,
      );
    } catch (_) {
      return ClipboardStatus.initial;
    }
  }

  @override
  Future<bool> triggerClipboardPull() async {
    try {
      final resp = await _client.triggerClipboardPull();
      return resp.success;
    } catch (_) {
      return false;
    }
  }

  @override
  Future<bool> startCapture({
    String? receiverUrl,
    int width = 720,
    int height = 1600,
    int fps = 30,
    int bitrateKbps = 2500,
  }) async {
    try {
      final target = receiverUrl != null && receiverUrl.isNotEmpty
          ? receiverUrl
          : 'peer-auto';
      final resp = await _client.startSession(target);
      _activeSessionId = resp.sessionId;
      return resp.sessionId.isNotEmpty;
    } catch (_) {
      return false;
    }
  }

  @override
  Future<bool> stopCapture() async {
    try {
      if (_activeSessionId != null && _activeSessionId!.isNotEmpty) {
        final resp = await _client.stopSession(sessionId: _activeSessionId!);
        _activeSessionId = null;
        return resp.sessionId.isNotEmpty;
      }
      return true;
    } catch (_) {
      return false;
    }
  }

  @override
  Future<List<DiscoveredDevice>> listDiscoveredDevices() async {
    try {
      final resp = await _client.listDevices();
      return resp.devices
          .map((d) => DiscoveredDevice(
                id: d.id,
                name: d.name,
                model: d.model,
                version: d.version,
                host: d.address,
                port: d.port,
              ))
          .toList();
    } catch (_) {
      return [];
    }
  }

  @override
  Future<PairingResult?> pairDevice(String deviceId) async {
    try {
      final resp = await _client.pairDevice(deviceId);
      return PairingResult(
        deviceName: resp.displayName,
        sasCode: resp.sas,
      );
    } catch (_) {
      return null;
    }
  }

  @override
  Future<bool> confirmPairing({
    required String deviceId,
    required bool confirmed,
  }) async {
    try {
      final resp = await _client.confirmPairing(
        deviceId: deviceId,
        userConfirmed: confirmed,
      );
      return resp.success;
    } catch (_) {
      return false;
    }
  }

  @override
  Stream<CaptureStats> get statsStream => _statsController.stream;

  @override
  Stream<Map<dynamic, dynamic>> get rawEventsStream => _rawEventsController.stream;

  @override
  void setNativeCallHandler(Future<dynamic> Function(dynamic call)? handler) {
    // Desktop platform does not use Android MethodChannel handlers
  }

  @override
  void dispose() {
    _streamSub?.cancel();
    _rawEventsController.close();
    _statsController.close();
    _client.shutdown();
  }
}
