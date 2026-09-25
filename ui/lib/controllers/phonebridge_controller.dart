import 'dart:async';
import 'dart:io';
import 'package:fixnum/fixnum.dart';
import 'package:flutter/foundation.dart';
import '../models/device_state.dart';
import '../models/capture_stats.dart';
import '../models/trusted_device.dart';
import '../models/clipboard_status.dart';
import '../models/activity_event.dart';
import '../generated/phonebridge/localipc/v1/local_ipc.pb.dart' as ipc;
import '../generated/phonebridge/v1/phonebridge.pb.dart' as pb;
import '../models/discovered_device.dart';
import '../models/link_status.dart';
import '../services/phonebridge_channel.dart';
import '../services/platform_bridge_service.dart';
import '../services/android_bridge_service.dart';
import '../services/linux_bridge_service.dart' show LinuxBridgeService;
import '../services/notification_backend.dart';
import '../services/session_backend.dart';
import 'session_controller.dart';
import 'transfer_controller.dart';
import 'notification_controller.dart';

class PhoneBridgeController extends ChangeNotifier {
  PhoneBridgeController({
    PlatformBridgeService? service,
    PhoneBridgeChannel? channel,
  }) : _service = service ??
            (channel != null
                ? AndroidBridgeService(channel: channel)
                : _createDefaultService());

  static PlatformBridgeService _createDefaultService() {
    if (kIsWeb || Platform.isAndroid || Platform.environment.containsKey('FLUTTER_TEST')) {
      return AndroidBridgeService();
    }
    return LinuxBridgeService();
  }

  final PlatformBridgeService _service;
  PlatformBridgeService get service => _service;

  /// The Linux IPC service, when this platform has one. Screens use this for
  /// daemon diagnostics (version, transport state) instead of reaching for a
  /// private client; null on Android, where the phone owns capture and there
  /// is no local daemon.
  LinuxBridgeService? get linuxService =>
      _service is LinuxBridgeService ? _service : null;

  /// File-transfer history and live progress (DEC-024). Shares [_service] with
  /// this controller: the service is owned and disposed here, not by the
  /// transfer controller.
  late final TransferController _transfers =
      TransferController(backend: _service);
  TransferController get transfers => _transfers;

  /// Active mirrored Android notifications on Linux desktop (DEC-028, Phase 8 v0.1).
  late final NotificationController _notifications =
      NotificationController(backend: _resolveNotificationBackend(_service));
  NotificationController get notifications => _notifications;

  /// Connection/session state for the whole app (Phase 5). Platforms without a
  /// local session state machine report "unsupported" rather than
  /// "disconnected", so the UI never claims a session is missing on a platform
  /// where sessions are not driven from this side.
  late final SessionController _session =
      SessionController(backend: _resolveSessionBackend(_service));
  SessionController get session => _session;

  /// The notification seam is optional per platform, so it is resolved structurally.
  static NotificationBackend _resolveNotificationBackend(PlatformBridgeService service) {
    final Object candidate = service;
    if (candidate is NotificationBackend) return candidate;
    return const UnsupportedNotificationBackend();
  }

  /// The session seam is optional per platform, so it is resolved structurally:
  /// a bridge that drives sessions locally also implements [SessionBackend].
  static SessionBackend _resolveSessionBackend(PlatformBridgeService service) {
    final Object candidate = service;
    if (candidate is SessionBackend) return candidate;
    return const UnsupportedSessionBackend();
  }

  /// The one connection/transfer status every surface renders.
  LinkStatus get linkStatus => LinkStatus.compose(
        session: _session.status,
        transfer: transferActivity,
        daemonReachable: _session.daemonReachable,
      );

  /// What the transfer surface currently knows, in the shape the connection
  /// model documents.
  TransferActivity get transferActivity => TransferActivity(
        activeCount: _transfers.activeCount,
        latest: _transfers.latest,
        backendUnavailable: _transfers.isBackendUnavailable,
      );

  DeviceState _deviceState = DeviceState.initial;
  CaptureStats _captureStats = CaptureStats.initial;
  ClipboardStatus _clipboardStatus = ClipboardStatus.initial;
  List<TrustedDevice> _trustedDevices = [];
  List<DiscoveredDevice> _discoveredDevices = [];
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
  List<DiscoveredDevice> get discoveredDevices => List.unmodifiable(_discoveredDevices);
  List<ActivityEvent> get activityEvents => List.unmodifiable(_activityEvents);
  bool get isLoading => _isLoading;
  String? get lastErrorMessage => _lastErrorMessage;

  int get selectedWidth => _selectedWidth;
  int get selectedHeight => _selectedHeight;
  int get selectedFps => _selectedFps;
  int get selectedBitrateKbps => _selectedBitrateKbps;
  String get receiverUrl => _receiverUrl;

  bool get isCapturing => _captureStats.isCapturing;

  /// True while the phone's screen is actually being streamed.
  ///
  /// Where the platform drives sessions from this side (Linux) the session model
  /// is the truth; elsewhere (Android, which owns capture itself) the capture
  /// telemetry is. Screens use this instead of picking one source, so the two
  /// can never disagree on screen.
  bool get isSharing => _session.supportsSessions
      ? _session.status.state == ipc.SessionState.SESSION_STATE_STREAMING
      : _captureStats.isCapturing;

  TrustedDevice? get activePeer {
    if (_trustedDevices.isEmpty) return null;
    final nonRevoked = _trustedDevices.where((d) => !d.revoked);
    return nonRevoked.isNotEmpty ? nonRevoked.first : null;
  }

  void initialize() {
    refreshAll();
    // Discovery is part of startup on the desktop: the Devices tab populates
    // from the daemon's mDNS results without requiring a manual scan first
    // (the retired session view loaded its discovered list the same way, at
    // init, through this same service).
    if (_service.isLinux) {
      unawaited(refreshDiscoveredDevices());
    }
    _subscribeEvents();
    // Subscribes to the live transfer stream and loads the recent history. The
    // transfers surface has its own retry affordance, so this never blocks the
    // rest of the dashboard.
    _transfers.initialize();
    // Same contract for the session: hydrate from the backend, then follow its
    // pushed transitions. One owner for the whole app.
    _session.addListener(_onChildChanged);
    _session.initialize();
    // The composed status also depends on transfer activity, so a transfer
    // change has to repaint the connection surfaces too — otherwise an in-flight
    // file would only be visible in the transfer list.
    _transfers.addListener(_onChildChanged);
    // Mirrored notifications subscription and hydration (DEC-028).
    _notifications.addListener(_onChildChanged);
    _notifications.initialize();
  }

  void _onChildChanged() {
    if (_session.status.isTerminal) {
      _notifications.clear();
    }
    notifyListeners();
  }

  bool _disposed = false;

  @override
  void dispose() {
    // Idempotent: the owner (a screen) may dispose the controller and so may a
    // test helper, and the second call must not tear down the shared service
    // twice.
    if (_disposed) return;
    _disposed = true;
    _rawEventsSub?.cancel();
    _notifications.removeListener(_onChildChanged);
    _notifications.dispose();
    _transfers.removeListener(_onChildChanged);
    _session.removeListener(_onChildChanged);
    _session.dispose();
    _transfers.dispose();
    _service.dispose();
    super.dispose();
  }

  void _subscribeEvents() {
    _rawEventsSub?.cancel();
    _rawEventsSub = _service.rawEventsStream.listen(
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
            // A zero selection is the device-default request (DEC-022):
            // word it as the choice it is, never as "0x0".
            _selectedWidth > 0 && _selectedFps > 0
                ? 'Streaming at ${_selectedWidth}x$_selectedHeight @ ${_selectedFps}fps'
                : 'Streaming at device default quality',
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
      final state = await _service.getDeviceState();
      final stats = await _service.getMediaStats();
      final devices = await _service.getTrustedDevices();
      final clip = await _service.getClipboardStatus();

      _deviceState = state;
      _captureStats = stats;
      _trustedDevices = devices;
      _clipboardStatus = clip;
      // The connection status is part of "refresh": an explicit refresh must be
      // able to move the app out of a stale failed/recoverable reading.
      unawaited(_session.refresh());
      notifyListeners();
    } catch (e) {
      _lastErrorMessage = e.toString();
      notifyListeners();
    }
  }

  Future<void> refreshDiscoveredDevices() async {
    try {
      _discoveredDevices = await _service.listDiscoveredDevices();
      notifyListeners();
    } catch (e) {
      _lastErrorMessage = e.toString();
      notifyListeners();
    }
  }

  Future<PairingResult?> pairDevice(String deviceId) async {
    _isLoading = true;
    _lastErrorMessage = null;
    notifyListeners();
    try {
      final res = await _service.pairDevice(deviceId);
      return res;
    } catch (e) {
      _lastErrorMessage = e.toString();
      return null;
    } finally {
      _isLoading = false;
      notifyListeners();
    }
  }

  Future<bool> confirmPairing({required String deviceId, required bool confirmed}) async {
    _isLoading = true;
    _lastErrorMessage = null;
    notifyListeners();
    try {
      final ok = await _service.confirmPairing(deviceId: deviceId, confirmed: confirmed);
      if (ok) {
        if (confirmed) {
          addActivityEvent(
            ActivityCategory.pairing,
            'Pairing Successful',
            'Successfully paired device $deviceId',
            ActivityLevel.success,
          );
        }
      } else {
        // The daemon refused the confirmation (mismatched SAS, revoked peer, …).
        // The Devices surface surfaces this instead of silently closing the
        // verification dialog as if the pairing had succeeded — the feedback
        // the retired session view's pairing flow used to give.
        _lastErrorMessage = 'Pairing was rejected or could not be confirmed';
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

  Future<bool> startScreenSharing() async {
    _isLoading = true;
    _lastErrorMessage = null;
    notifyListeners();

    try {
      final ok = await _service.startCapture(
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
      final ok = await _service.stopCapture();
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
      final ok = await _service.revokeDevice(deviceId);
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
      final ok = await _service.removeDevice(deviceId);
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
      final ok = await _service.triggerClipboardPull();
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

  /// Dispatches an input frame to the active device session (DEC-027).
  Future<bool> sendInput(pb.InputFrame frame) async {
    final linux = linuxService;
    if (linux == null) return false;
    final activeId = _session.activeSessionId;
    return linux.sendInput(frame, sessionId: activeId);
  }

  /// Convenience helper to dispatch a global action (Back, Home, Recents, etc.).
  Future<bool> sendGlobalAction(pb.GlobalActionEvent_Type actionType) async {
    final frame = pb.InputFrame(
      timestampMs: Int64(DateTime.now().millisecondsSinceEpoch),
      action: pb.GlobalActionEvent(type: actionType),
    );
    return sendInput(frame);
  }

  /// Convenience helper to commit text entry through the companion IME.
  Future<bool> sendText(String text) async {
    if (text.isEmpty) return false;
    final frame = pb.InputFrame(
      timestampMs: Int64(DateTime.now().millisecondsSinceEpoch),
      text: pb.TextEvent(text: text),
    );
    return sendInput(frame);
  }
}
