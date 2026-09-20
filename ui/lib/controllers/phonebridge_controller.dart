import 'dart:async';
import 'package:flutter/foundation.dart';
import '../models/device_state.dart';
import '../models/capture_stats.dart';
import '../models/trusted_device.dart';
import '../models/clipboard_status.dart';
import '../models/activity_event.dart';
import '../services/phonebridge_channel.dart';

class PhoneBridgeController extends ChangeNotifier {
  PhoneBridgeController({PhoneBridgeChannel? channel})
      : _channel = channel ?? PhoneBridgeChannel();

  final PhoneBridgeChannel _channel;

  DeviceState _deviceState = DeviceState.initial;
  CaptureStats _captureStats = CaptureStats.initial;
  ClipboardStatus _clipboardStatus = ClipboardStatus.initial;
  List<TrustedDevice> _trustedDevices = [];
  final List<ActivityEvent> _activityEvents = [];

  bool _isLoading = false;
  String? _lastErrorMessage;

  int _selectedWidth = 720;
  int _selectedHeight = 1600;
  int _selectedFps = 30;
  int _selectedBitrateKbps = 2500;
  String _receiverUrl = '';

  StreamSubscription<Map<dynamic, dynamic>>? _rawEventsSub;

  DeviceState get deviceState => _deviceState;
  CaptureStats get captureStats => _captureStats;
  ClipboardStatus get clipboardStatus => _clipboardStatus;
  List<TrustedDevice> get trustedDevices => List.unmodifiable(_trustedDevices);
  List<ActivityEvent> get activityEvents => List.unmodifiable(_activityEvents);
  bool get isLoading => _isLoading;
  String? get lastErrorMessage => _lastErrorMessage;

  int get selectedWidth => _selectedWidth;
  int get selectedHeight => _selectedHeight;
  int get selectedFps => _selectedFps;
  int get selectedBitrateKbps => _selectedBitrateKbps;
  String get receiverUrl => _receiverUrl;

  bool get isCapturing => _captureStats.isCapturing;

  TrustedDevice? get activePeer {
    if (_trustedDevices.isEmpty) return null;
    final nonRevoked = _trustedDevices.where((d) => !d.revoked);
    return nonRevoked.isNotEmpty ? nonRevoked.first : null;
  }

  void initialize() {
    refreshAll();
    _subscribeEvents();
  }

  @override
  void dispose() {
    _rawEventsSub?.cancel();
    super.dispose();
  }

  void _subscribeEvents() {
    _rawEventsSub?.cancel();
    _rawEventsSub = _channel.rawEventsStream.listen(
      (event) {
        final prevCapturing = _captureStats.isCapturing;
        _captureStats = CaptureStats.fromMap(event, previous: _captureStats);

        final rawState = event['clipboardState'] as String?;
        final ime = event['imeSelected'] as bool?;
        if (rawState != null || ime != null) {
          _clipboardStatus = _clipboardStatus.copyWith(
            state: rawState != null ? ClipboardSyncState.fromString(rawState) : null,
            imeSelected: ime,
          );
        }

        final err = event['lastError'] as String?;
        if (err != null && err.isNotEmpty && err != _lastErrorMessage) {
          _lastErrorMessage = err;
          addActivityEvent(
            ActivityCategory.screen,
            'Screen Share Error',
            err,
            ActivityLevel.error,
          );
        }

        // Detect state transitions for activity log
        if (!prevCapturing && _captureStats.isCapturing) {
          addActivityEvent(
            ActivityCategory.screen,
            'Screen Sharing Started',
            'Streaming at ${_selectedWidth}x$_selectedHeight @ ${_selectedFps}fps',
            ActivityLevel.success,
          );
        } else if (prevCapturing && !_captureStats.isCapturing) {
          addActivityEvent(
            ActivityCategory.screen,
            'Screen Sharing Stopped',
            err != null && err.isNotEmpty ? 'Stopped: $err' : 'Session ended normally',
            err != null && err.isNotEmpty ? ActivityLevel.warning : ActivityLevel.info,
          );
        }

        notifyListeners();
      },
      onError: (err) {
        _lastErrorMessage = err.toString();
        notifyListeners();
      },
    );
  }

  Future<void> refreshAll() async {
    try {
      final state = await _channel.getDeviceState();
      final stats = await _channel.getMediaStats();
      final devices = await _channel.getTrustedDevices();
      final clip = await _channel.getClipboardStatus();

      _deviceState = state;
      _captureStats = stats;
      _trustedDevices = devices;
      _clipboardStatus = clip;
      notifyListeners();
    } catch (e) {
      _lastErrorMessage = e.toString();
      notifyListeners();
    }
  }

  Future<bool> startScreenSharing() async {
    _isLoading = true;
    _lastErrorMessage = null;
    notifyListeners();

    try {
      final ok = await _channel.startCapture(
        receiverUrl: _receiverUrl.trim().isEmpty ? null : _receiverUrl.trim(),
        width: _selectedWidth,
        height: _selectedHeight,
        fps: _selectedFps,
        bitrateKbps: _selectedBitrateKbps,
      );

      if (!ok) {
        _lastErrorMessage = 'Failed to initiate screen capture request';
      }
      await refreshAll();
      return ok;
    } catch (e) {
      _lastErrorMessage = e.toString();
      return false;
    } finally {
      _isLoading = false;
      notifyListeners();
    }
  }

  Future<bool> stopScreenSharing() async {
    _isLoading = true;
    _lastErrorMessage = null;
    notifyListeners();

    try {
      final ok = await _channel.stopCapture();
      await refreshAll();
      return ok;
    } catch (e) {
      _lastErrorMessage = e.toString();
      return false;
    } finally {
      _isLoading = false;
      notifyListeners();
    }
  }

  Future<bool> revokeDevice(String deviceId) async {
    _isLoading = true;
    notifyListeners();
    try {
      final ok = await _channel.revokeDevice(deviceId);
      if (ok) {
        addActivityEvent(
          ActivityCategory.pairing,
          'Trust Revoked',
          'Revoked trust for device $deviceId',
          ActivityLevel.warning,
        );
      }
      await refreshAll();
      return ok;
    } catch (e) {
      _lastErrorMessage = e.toString();
      return false;
    } finally {
      _isLoading = false;
      notifyListeners();
    }
  }

  Future<bool> removeDevice(String deviceId) async {
    _isLoading = true;
    notifyListeners();
    try {
      final ok = await _channel.removeDevice(deviceId);
      if (ok) {
        addActivityEvent(
          ActivityCategory.pairing,
          'Device Removed',
          'Removed device $deviceId from trust store',
          ActivityLevel.info,
        );
      }
      await refreshAll();
      return ok;
    } catch (e) {
      _lastErrorMessage = e.toString();
      return false;
    } finally {
      _isLoading = false;
      notifyListeners();
    }
  }

  Future<bool> triggerClipboardPull() async {
    try {
      final ok = await _channel.triggerClipboardPull();
      if (ok) {
        _clipboardStatus = _clipboardStatus.copyWith(
          lastSyncTimestampMs: DateTime.now().millisecondsSinceEpoch,
          syncCount: _clipboardStatus.syncCount + 1,
          lastDirection: 'outbound',
        );
        addActivityEvent(
          ActivityCategory.clipboard,
          'Manual Clipboard Sync',
          'Clipboard item forwarded to Go sync engine',
          ActivityLevel.success,
        );
        notifyListeners();
      }
      return ok;
    } catch (e) {
      _lastErrorMessage = e.toString();
      notifyListeners();
      return false;
    }
  }

  void setClipboardEnabled(bool enabled) {
    _clipboardStatus = _clipboardStatus.copyWith(isEnabled: enabled);
    addActivityEvent(
      ActivityCategory.clipboard,
      enabled ? 'Clipboard Sync Enabled' : 'Clipboard Sync Disabled',
      'User changed clipboard toggle',
      ActivityLevel.info,
    );
    notifyListeners();
  }

  void setResolution(int width, int height) {
    _selectedWidth = width;
    _selectedHeight = height;
    notifyListeners();
  }

  void setFps(int fps) {
    _selectedFps = fps;
    notifyListeners();
  }

  void setBitrate(int kbps) {
    _selectedBitrateKbps = kbps;
    notifyListeners();
  }

  void setReceiverUrl(String url) {
    _receiverUrl = url;
    notifyListeners();
  }

  void clearErrorMessage() {
    _lastErrorMessage = null;
    notifyListeners();
  }

  void addActivityEvent(
    ActivityCategory category,
    String title,
    String detail, [
    ActivityLevel level = ActivityLevel.info,
  ]) {
    final event = ActivityEvent(
      id: '${DateTime.now().millisecondsSinceEpoch}_${_activityEvents.length}',
      timestamp: DateTime.now(),
      category: category,
      title: title,
      detail: detail,
      level: level,
    );
    _activityEvents.insert(0, event);
    if (_activityEvents.length > 100) {
      _activityEvents.removeLast();
    }
    notifyListeners();
  }

  void clearActivityLog() {
    _activityEvents.clear();
    notifyListeners();
  }
}
