import 'dart:async';
import 'dart:io';
import 'package:fixnum/fixnum.dart';
import 'package:flutter/foundation.dart';
import '../models/device_state.dart';
import '../models/capture_stats.dart';
import '../models/trusted_device.dart';
import '../models/device_row.dart';
import '../models/clipboard_status.dart';
import '../models/activity_event.dart';
import '../generated/phonebridge/localipc/v1/local_ipc.pb.dart' as ipc;
import '../generated/phonebridge/v1/phonebridge.pb.dart' as pb;
import '../models/discovered_device.dart';
import '../models/inbound_pairing.dart';
import '../models/link_status.dart';
import '../services/phonebridge_channel.dart';
import '../services/platform_bridge_service.dart';
import '../services/android_bridge_service.dart';
import '../services/linux_bridge_service.dart' show LinuxBridgeService;
import '../services/notification_backend.dart';
import '../services/pairing_backend.dart';
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
    if (kIsWeb ||
        Platform.isAndroid ||
        Platform.environment.containsKey('FLUTTER_TEST')) {
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

  /// Inbound-pairing seam (Phase 2): resolved structurally — null where the
  /// platform carries no inbound-pairing state (and no test fake needs to).
  PairingBackend? get pairingBackend {
    final Object candidate = _service;
    if (candidate is PairingBackend) return candidate;
    return null;
  }

  /// Connection/session state for the whole app (Phase 5). Platforms without a
  /// local session state machine report "unsupported" rather than
  /// "disconnected", so the UI never claims a session is missing on a platform
  /// where sessions are not driven from this side.
  late final SessionController _session =
      SessionController(backend: _resolveSessionBackend(_service));
  SessionController get session => _session;

  /// The notification seam is optional per platform, so it is resolved structurally.
  static NotificationBackend _resolveNotificationBackend(
      PlatformBridgeService service) {
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
  List<InboundPairing> _inboundPairings = [];
  final List<ActivityEvent> _activityEvents = [];
  Map<String, dynamic> _permissionsStatus = const {};

  /// Device id whose outbound pairing is awaiting SAS confirmation, or null.
  /// Drives the "PAIRING" badge so an in-flight handshake is never rendered
  /// as plain UNPAIRED.
  String? _outboundPairingDeviceId;

  /// Device id currently undergoing connection negotiation, if any.
  String? _connectingDeviceId;

  /// Inbound request tokens this device has already surfaced a prompt for.
  /// Tracks the one-shot dialog so the same request never re-prompts on every
  /// notifyListeners.
  final Set<String> _promptedInboundTokens = {};

  /// Encoder-selected screen capabilities on Android (see
  /// `AndroidBridgeService.getMediaCapabilities`). Null until fetched, and
  /// null again when the platform cannot answer — the UI renders
  /// "Checking…" instead of guessing. On Linux the capabilities ride the
  /// session snapshot instead (Go retains the DEC-022 advertisement).
  pb.MediaCapabilities? _androidCapabilities;
  pb.MediaCapabilities? get androidCapabilities => _androidCapabilities;

  /// The capabilities the quality presets gate on: the live session snapshot
  /// where sessions are driven locally (Linux), the channel-read encoder
  /// advertisement where capture is owned locally (Android).
  pb.MediaCapabilities? get effectiveCapabilities =>
      _service.isLinux ? _session.status.capabilities : _androidCapabilities;

  bool _isLoading = false;
  String? _lastErrorMessage;

  int _selectedWidth = 720;
  int _selectedHeight = 1600;
  int _selectedFps = 30;
  int _selectedBitrateKbps = 2500;
  String _receiverUrl = '';

  StreamSubscription<Map<dynamic, dynamic>>? _rawEventsSub;

  /// Inbound-pairing lifecycle events where the platform pushes them
  /// (Linux: daemon PairingEvent on the one IPC subscription).
  StreamSubscription<dynamic>? _pairingEventsSub;

  /// Sweep while requests are pending: with no events flowing (Android
  /// foreground path, or an event gap) the dialog still expires on time.
  Timer? _pairingSweepTimer;

  DeviceState get deviceState => _deviceState;
  CaptureStats get captureStats => _captureStats;
  ClipboardStatus get clipboardStatus => _clipboardStatus;
  List<TrustedDevice> get trustedDevices => List.unmodifiable(_trustedDevices);
  List<DiscoveredDevice> get discoveredDevices =>
      List.unmodifiable(_discoveredDevices);

  /// Pairing requests awaiting THIS device's approval (Phase 2). Empty where
  /// the platform carries no inbound-pairing state.
  List<InboundPairing> get pairingRequests =>
      List.unmodifiable(_inboundPairings);

  /// The oldest inbound request not yet surfaced to the user, or null. The
  /// app scaffold consumes this to show the one-shot Pairing Request dialog;
  /// the token is marked prompted on read.
  InboundPairing? consumePendingPairingPrompt() {
    for (final r in _inboundPairings) {
      if (_promptedInboundTokens.add(r.token)) return r;
    }
    return null;
  }

  /// Device id whose pairing handshake is awaiting SAS confirmation.
  String? get outboundPairingDeviceId => _outboundPairingDeviceId;
  List<ActivityEvent> get activityEvents => List.unmodifiable(_activityEvents);
  Map<String, dynamic> get permissionsStatus => _permissionsStatus;
  bool get postNotificationsGranted =>
      _permissionsStatus['postNotifications'] == true;
  bool get notificationListenerEnabled =>
      _permissionsStatus['notificationListener'] == true;
  bool get accessibilityEnabled => _permissionsStatus['accessibility'] == true;

  /// Runtime liveness, distinct from the granted/configured flags above.
  /// Null means the native side did not report it (old build) — the UI shows
  /// no runtime line rather than guessing.
  bool? get notificationServiceActive =>
      _permissionsStatus['notificationServiceActive'] as bool?;
  bool? get accessibilityServiceActive =>
      _permissionsStatus['accessibilityServiceActive'] as bool?;
  bool? get foregroundServiceActive =>
      _permissionsStatus['foregroundServiceActive'] as bool?;
  bool get hasMissingPermissions {
    if (!_service.isAndroid) return false;
    return !postNotificationsGranted ||
        !notificationListenerEnabled ||
        !accessibilityEnabled;
  }

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

  /// Authoritative active device: the daemon session's target while a
  /// session is live, null otherwise (Phase B).
  ///
  /// Linux: the session controller's peer (daemon snapshot device_id).
  /// Android (no local session state machine; the phone owns capture):
  /// the explicitly selected target while capture is live. Never trust
  /// ordering, never discovery ordering, never a guess.
  String? get activeDeviceId {
    if (_service.isLinux) return _session.activeDeviceId;
    if (!isSharing) return null;
    return _androidTargetDeviceId;
  }

  /// Device id currently being connected to (either in-flight start/switch,
  /// or daemon session state while connecting/discovering).
  String? get connectingDeviceId {
    if (_connectingDeviceId != null && _connectingDeviceId!.isNotEmpty) {
      return _connectingDeviceId;
    }
    if (_session.supportsSessions && _session.status.isConnecting) {
      return _session.status.deviceId;
    }
    return null;
  }

  /// True when a connection is actively being negotiated (connecting or discovering),
  /// but has not yet reached a stable connected/streaming state.
  bool get isSessionConnecting =>
      _connectingDeviceId != null ||
      (_session.supportsSessions && _session.status.isConnecting) ||
      (_isLoading && _androidTargetDeviceId != null && !isSharing);

  /// Whether the given [deviceId] is the target currently being connected to.
  bool isConnectingTo(String deviceId) {
    if (deviceId.isEmpty) return false;
    final target = connectingDeviceId;
    return isSessionConnecting && target == deviceId;
  }

  /// The trusted record for the authoritative active device, or null when
  /// there is no live session peer. This REPLACES the old "first trusted"
  /// meaning: callers must treat null as "no active device", not as
  /// "pick another one".
  TrustedDevice? get activePeer {
    final id = activeDeviceId;
    if (id == null || id.isEmpty) return null;
    for (final d in _trustedDevices) {
      if (d.deviceId == id) return d;
    }
    return null;
  }

  /// Canonical logical devices (Phase F): one row per deviceId merging
  /// trust + discovery + the authoritative session target. Screens must
  /// render rows, never the two raw lists side by side.
  List<DeviceRow> get deviceRows => mergeDeviceRows(
        trusted: _trustedDevices,
        discovered: _discoveredDevices,
        activeDeviceId: activeDeviceId,
      );

  /// Generation guard for atomic switch (Phase E): every switchToDevice call
  /// takes a generation; late completions from a superseded switch are
  /// dropped instead of overwriting the newer attempt's state.
  int _switchGeneration = 0;

  /// Last explicitly selected Android target (device id). Set on connect,
  /// cleared on disconnect/stop. The phone has no daemon session snapshot,
  /// so this explicit selection is the only honest Android peer.
  String? _androidTargetDeviceId;

  void initialize() {
    refreshAll();
    if (_service.isAndroid) {
      unawaited(refreshPermissions());
      unawaited(refreshCapabilities());
    }
    // Discovery is part of startup on both platforms: the Devices tab
    // populates from the mDNS results without requiring a manual scan first.
    // On Linux the daemon browses; on Android the phone browses for the PC
    // through the Go core, which is the only way it can ever list a desktop.
    unawaited(refreshDiscoveredDevices());
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
    // Inbound pairing (Phase 2): hydrate once, then follow pushed transitions
    // where the platform provides them, plus a bounded sweep while anything
    // is pending so expiry/withdrawal is seen even without events.
    if (pairingBackend != null) {
      unawaited(refreshInboundPairings());
      final LinuxBridgeService? linux =
          _service is LinuxBridgeService ? _service : null;
      _pairingEventsSub = linux?.pairingEventStream.listen((_) {
        unawaited(refreshInboundPairings());
      });
    }
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
    _pairingEventsSub?.cancel();
    _pairingSweepTimer?.cancel();
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
        // Native trust-store mutations (pairing commit, revoke, remove) can
        // land with no pairing dialog open — e.g. the background responder
        // path. Refresh so the Devices surface never shows stale UNPAIRED.
        if (event['trustChanged'] == true) {
          unawaited(refreshAll());
        }

        // Inbound pairing surfaced over the platform event channel (Android):
        // the request set changed — arrived, answered, or swept. Refresh the
        // snapshot; the dialog consumption is driven by the state change.
        if (event['pairingChanged'] == true) {
          unawaited(refreshInboundPairings());
        }

        final prevCapturing = _captureStats.isCapturing;
        _captureStats = CaptureStats.fromMap(event, previous: _captureStats);

        final rawState = event['clipboardState'] as String?;
        final ime = event['imeSelected'] as bool?;
        final clipEnabled = event['enabled'] as bool?;
        if (rawState != null || ime != null || clipEnabled != null) {
          _clipboardStatus = _clipboardStatus.copyWith(
            state: rawState != null
                ? ClipboardSyncState.fromString(rawState)
                : null,
            imeSelected: ime,
            isEnabled: clipEnabled,
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
            err != null && err.isNotEmpty
                ? 'Stopped: $err'
                : 'Session ended normally',
            err != null && err.isNotEmpty
                ? ActivityLevel.warning
                : ActivityLevel.info,
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
      if (_service.isAndroid) {
        unawaited(refreshPermissions());
        unawaited(refreshCapabilities());
      }
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

  /// Re-reads the inbound-pairing snapshot (Phase 2). Called on pushed
  /// transitions, on the pending sweep, and by explicit refreshes.
  Future<void> refreshInboundPairings() async {
    final backend = pairingBackend;
    if (backend == null) return;
    try {
      final prevTokens = _inboundPairings.map((r) => r.token).toSet();
      _inboundPairings = await backend.listInboundPairings();
      // A token that left the snapshot was withdrawn or expired: re-prompting
      // for it later would be wrong even if the same peer asks again — that
      // arrives as a NEW token and prompts normally.
      _promptedInboundTokens.removeWhere(
        (t) => !_inboundPairings.any((r) => r.token == t),
      );
      _updatePairingSweepTimer(prevTokens);
      notifyListeners();
    } catch (_) {
      // Pairing requests are transient state; a failed read leaves the last
      // snapshot rather than clearing visible requests on a transport blip.
    }
  }

  void _updatePairingSweepTimer(Set<String> prevTokens) {
    final hadPending = prevTokens.isNotEmpty;
    final hasPending = _inboundPairings.isNotEmpty;
    if (hasPending && _pairingSweepTimer == null) {
      _pairingSweepTimer = Timer.periodic(const Duration(seconds: 5), (_) {
        unawaited(refreshInboundPairings());
      });
    } else if (!hasPending && hadPending) {
      _pairingSweepTimer?.cancel();
      _pairingSweepTimer = null;
    }
  }

  /// Records the local user's accept/reject for one inbound pairing request.
  /// Trust is committed natively on the requester's next signed confirm; this
  /// only records the decision and refreshes what the UI knows.
  Future<bool> respondPairing({
    required InboundPairing request,
    required bool approved,
  }) async {
    final backend = pairingBackend;
    if (backend == null) return false;
    final ok = await backend.respondInboundPairing(
      pairingToken: request.token,
      approved: approved,
    );
    addActivityEvent(
      ActivityCategory.pairing,
      ok
          ? (approved ? 'Pairing Request Accepted' : 'Pairing Request Rejected')
          : 'Pairing Request Already Gone',
      ok
          ? '${approved ? 'Accepted' : 'Rejected'} pairing request from ${request.remoteName}'
          : 'Request from ${request.remoteName} expired or was withdrawn',
      ok ? ActivityLevel.success : ActivityLevel.warning,
    );
    await refreshInboundPairings();
    // On approval the requester's confirm lands shortly; trustChanged (and
    // the next refresh) picks up the committed trust either way.
    return ok;
  }

  Future<void> refreshPermissions() async {
    final s = _service;
    if (s is AndroidBridgeService) {
      try {
        _permissionsStatus = await s.getPermissionsStatus();
        notifyListeners();
      } catch (_) {}
    }
  }

  /// Reads the encoder-selected screen capabilities on Android. Safe to call
  /// on any platform (no-op elsewhere); a null answer means unknown.
  Future<void> refreshCapabilities() async {
    final s = _service;
    if (s is AndroidBridgeService) {
      try {
        _androidCapabilities = await s.getMediaCapabilities();
        notifyListeners();
      } catch (_) {}
    }
  }

  Future<bool> requestNotificationPermission() async {
    final s = _service;
    if (s is AndroidBridgeService) {
      final res = await s.requestNotificationPermission();
      await refreshPermissions();
      return res;
    }
    return false;
  }

  Future<bool> openNotificationListenerSettings() async {
    final s = _service;
    if (s is AndroidBridgeService) {
      return s.openNotificationListenerSettings();
    }
    return false;
  }

  Future<bool> openAccessibilitySettings() async {
    final s = _service;
    if (s is AndroidBridgeService) {
      return s.openAccessibilitySettings();
    }
    return false;
  }

  Future<bool> openAppDetailsSettings() async {
    final s = _service;
    if (s is AndroidBridgeService) {
      return s.openAppDetailsSettings();
    }
    return false;
  }

  Future<PairingResult?> pairDevice(String deviceId) async {
    _isLoading = true;
    _lastErrorMessage = null;
    notifyListeners();
    try {
      final res = await _service.pairDevice(deviceId);
      if (res != null && !res.alreadyTrusted) {
        // The request was answered by the peer's signaling server; the SAS is
        // now awaiting the local user's confirm. The tile shows PAIRING, not
        // UNPAIRED, for as long as that handshake is alive. An
        // already-trusted answer opens no handshake — the badge must not
        // come on (and a stale one from a previous flow is cleared).
        _outboundPairingDeviceId = deviceId;
        addActivityEvent(
          ActivityCategory.pairing,
          'Pairing Request Sent',
          'Waiting for SAS confirmation with ${res.deviceName}',
          ActivityLevel.info,
        );
      } else if (res != null && res.alreadyTrusted) {
        _outboundPairingDeviceId = null;
      }
      return res;
    } catch (e) {
      _lastErrorMessage = e.toString();
      return null;
    } finally {
      _isLoading = false;
      notifyListeners();
    }
  }

  Future<bool> confirmPairing(
      {required String deviceId, required bool confirmed}) async {
    _isLoading = true;
    _lastErrorMessage = null;
    notifyListeners();
    try {
      final ok = await _service.confirmPairing(
          deviceId: deviceId, confirmed: confirmed);
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
      _outboundPairingDeviceId = null;
      _isLoading = false;
      notifyListeners();
    }
  }

  /// Starts a screen-sharing session.
  ///
  /// [targetDeviceId] is the Linux contract (the daemon resolves the id to an
  /// endpoint). [receiverUrl] is the Android contract: the phone owns the
  /// session and dials the peer's signaling server directly, so it needs the
  /// discovered `http://host:port` rather than a device id.
  ///
  /// Explicit selection only (Phase C): a user CONNECT always names its
  /// device. There is no peer-auto fallback and no "first trusted" fallback —
  /// an empty target fails before capture/consent, never after. The manual
  /// Diagnostics override ([_receiverUrl]) still counts as explicit input.
  ///
  /// On Android a device id is therefore never passed as a receiver URL: an
  /// explicit URL goes through untouched (Kotlin validates it), anything else
  /// is resolved through the discovery records, and an unresolvable peer
  /// aborts cleanly *before* capture — never after consent.
  Future<bool> startScreenSharing(
      {String? targetDeviceId, String? receiverUrl}) async {
    _connectingDeviceId = targetDeviceId;
    _isLoading = true;
    _lastErrorMessage = null;
    notifyListeners();

    try {
      final String? target;
      if (_service.isAndroid) {
        target = _resolveAndroidReceiver(
          receiverUrl: receiverUrl,
          targetDeviceId: targetDeviceId,
        );
        if (target == null) {
          _lastErrorMessage = 'Select a device to connect. '
              'The selected device is not currently reachable — '
              'make sure it is discovered on the LAN, then try again.';
          return false;
        }
      } else {
        final explicit = (targetDeviceId != null && targetDeviceId.isNotEmpty)
            ? targetDeviceId
            : ((_receiverUrl.trim().isNotEmpty)
                ? _receiverUrl.trim()
                : ((receiverUrl != null && receiverUrl.isNotEmpty)
                    ? receiverUrl
                    : null));
        if (explicit == null || explicit.isEmpty) {
          _lastErrorMessage = 'Select a device to connect.';
          return false;
        }
        target = explicit;
      }
      final ok = await _service.startCapture(
        receiverUrl: target,
        width: _selectedWidth,
        height: _selectedHeight,
        fps: _selectedFps,
        bitrateKbps: _selectedBitrateKbps,
      );

      if (!ok) {
        _lastErrorMessage = 'Failed to initiate screen capture request';
      } else if (_service.isAndroid) {
        // Remember the explicit peer while capture lives so activeDeviceId
        // stays honest without guessing (cleared on stop).
        final id = (targetDeviceId != null && targetDeviceId.isNotEmpty)
            ? targetDeviceId
            : _deviceIdForSignalingUrl(target);
        _androidTargetDeviceId = id;
      }
      await refreshAll();
      return ok;
    } catch (e) {
      _lastErrorMessage = e.toString();
      return false;
    } finally {
      _connectingDeviceId = null;
      _isLoading = false;
      notifyListeners();
    }
  }

  /// Maps a resolved Android signaling URL back to its discovery device id,
  /// or null when the URL matches no current record.
  String? _deviceIdForSignalingUrl(String url) {
    final uri = Uri.tryParse(url);
    if (uri == null) return null;
    final host = uri.host;
    final port = uri.hasPort ? uri.port : -1;
    for (final dev in _discoveredDevices) {
      if (!dev.isStale && dev.host == host && (port <= 0 || dev.port == port)) {
        return dev.id;
      }
    }
    return null;
  }

  /// Resolves what Android's `startCapture` must dial. An explicit URL is
  /// returned untouched (endpoint validation owns it); an explicit device id
  /// is resolved through the canonical discovery records. Null means no
  /// usable endpoint: the caller aborts instead of launching consent for a
  /// session that cannot establish. There is deliberately no fallback to a
  /// remembered or "first" peer (Phase C).
  String? _resolveAndroidReceiver({
    String? receiverUrl,
    String? targetDeviceId,
  }) {
    if (receiverUrl != null && receiverUrl.isNotEmpty) return receiverUrl;
    final id = (targetDeviceId != null && targetDeviceId.isNotEmpty)
        ? targetDeviceId
        : null;
    if (id != null && id.isNotEmpty) {
      for (final dev in _discoveredDevices) {
        if (dev.id == id && !dev.isStale && dev.signalingUrl.isNotEmpty) {
          return dev.signalingUrl;
        }
      }
      // An explicit device id with no discovery record is not an endpoint.
      return null;
    }
    // Manual receiver override (Diagnostics) is passed through untouched —
    // endpoint validation owns it. Empty means unresolvable: abort.
    if (_receiverUrl.trim().isNotEmpty) return _receiverUrl.trim();
    return null;
  }

  /// Disconnects the live session, if any (Phase D).
  ///
  /// Trust, discovery, and pairing state are untouched: the device remains
  /// trusted and discoverable. Safe to call with no session (no-op success),
  /// while failed, or during reconnect — all covered by regression tests.
  Future<bool> disconnectActiveDevice(
      {String reason = 'user requested disconnect from devices UI'}) async {
    if (_service.isAndroid) {
      _androidTargetDeviceId = null;
    }
    final ok = await stopScreenSharing();
    if (_service.isAndroid) {
      _androidTargetDeviceId = null;
    }
    return ok;
  }

  /// Atomically switches from the current peer to [targetDeviceId]
  /// (Phase E): stops the live session, waits until it is terminal,
  /// then starts the new peer explicitly.
  ///
  /// Never starts B while A is still live (that path is SESSION_BUSY by
  /// design). If B fails after A stopped, the result is honestly "no active
  /// device" — A is not resurrected. Superseded switches (a newer switch
  /// started while this one was stopping) drop out instead of overwriting
  /// the newer attempt's state.
  Future<bool> switchToDevice({
    String? targetDeviceId,
    String? receiverUrl,
    Duration settleTimeout = const Duration(seconds: 8),
  }) async {
    final generation = ++_switchGeneration;
    bool superseded() => generation != _switchGeneration;

    _connectingDeviceId = targetDeviceId;
    notifyListeners();

    try {
      final needsStop =
          _session.status.isActive || activeDeviceId != null || isSharing;
      if (needsStop) {
        final stopped = await disconnectActiveDevice(
            reason: 'user switched devices from devices UI');
        if (superseded()) return false;
        if (!stopped) {
          _lastErrorMessage ??= 'Could not stop the current session.';
          return false;
        }
        final deadline = DateTime.now().add(settleTimeout);
        while (DateTime.now().isBefore(deadline)) {
          if (superseded()) return false;
          if (!_session.status.isActive && activeDeviceId == null && !isSharing) {
            break;
          }
          await Future<void>.delayed(const Duration(milliseconds: 150));
          if (superseded()) return false;
        }
        if (_session.status.isActive) {
          _lastErrorMessage =
              'The previous session did not stop in time. Try disconnecting first.';
          return false;
        }
      }
      if (superseded()) return false;
      return await startScreenSharing(
        targetDeviceId: targetDeviceId,
        receiverUrl: receiverUrl,
      );
    } finally {
      if (!superseded()) {
        _connectingDeviceId = null;
        notifyListeners();
      }
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

  /// Permanently removes all revoked device records from the trust store.
  ///
  /// Returns the number of removed devices.
  Future<int> clearAllRevokedDevices() async {
    final revoked = _trustedDevices.where((d) => d.revoked).toList();
    if (revoked.isEmpty) return 0;

    _isLoading = true;
    _lastErrorMessage = null;
    notifyListeners();
    int removedCount = 0;
    try {
      for (final dev in revoked) {
        final ok = await _service.removeDevice(dev.deviceId);
        if (ok) {
          removedCount++;
        }
      }
      if (removedCount > 0) {
        addActivityEvent(
          ActivityCategory.pairing,
          'Revoked Devices Cleared',
          'Removed $removedCount revoked device records from trust store',
          ActivityLevel.info,
        );
      }
      await refreshAll();
      return removedCount;
    } catch (e) {
      _lastErrorMessage = e.toString();
      return removedCount;
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

  /// Applies the clipboard master switch.
  ///
  /// The switch used to only flip a local flag, so it looked like it worked
  /// while sync kept running. On Android the adapter owns the flag, so the
  /// native side is told first and the UI reflects what it accepted.
  Future<void> setClipboardEnabled(bool enabled) async {
    final s = _service;
    if (s is AndroidBridgeService) {
      final ok = await s.setClipboardSyncEnabled(enabled);
      if (!ok) {
        _lastErrorMessage = 'Clipboard synchronization could not be changed';
        notifyListeners();
        return;
      }
    }
    _clipboardStatus = _clipboardStatus.copyWith(isEnabled: enabled);
    addActivityEvent(
      ActivityCategory.clipboard,
      enabled ? 'Clipboard Sync Enabled' : 'Clipboard Sync Disabled',
      'User changed clipboard toggle',
      ActivityLevel.info,
    );
    notifyListeners();
  }

  /// Opens the Android screen where the companion IME is enabled (DEC-023).
  Future<bool> openInputMethodSettings() async {
    final s = _service;
    if (s is AndroidBridgeService) {
      return s.openInputMethodSettings();
    }
    return false;
  }

  /// Opens the system keyboard picker so the companion IME can be selected.
  Future<bool> showInputMethodPicker() async {
    final s = _service;
    if (s is AndroidBridgeService) {
      return s.showInputMethodPicker();
    }
    return false;
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

  bool _remoteControlEnabled = true;
  bool get remoteControlEnabled => _remoteControlEnabled;

  void setRemoteControlEnabled(bool value) {
    if (_remoteControlEnabled != value) {
      _remoteControlEnabled = value;
      notifyListeners();
    }
  }

  void toggleRemoteControl() {
    setRemoteControlEnabled(!_remoteControlEnabled);
  }

  bool get isRemoteControlAvailable {
    if (!service.isLinux) return false;
    if (!_session.status.isActive) return false;
    final peer = activePeer;
    if (peer != null && !peer.isTrusted) return false;
    return true;
  }

  /// Dispatches an input frame to the active device session (DEC-027).
  Future<bool> sendInput(pb.InputFrame frame) async {
    final linux = linuxService;
    if (linux == null) return false;
    if (!_remoteControlEnabled) return false;
    if (!_session.status.isActive) return false;
    final peer = activePeer;
    if (peer != null && !peer.isTrusted) return false;
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

  /// Dispatches a discrete key event (e.g. Backspace, Enter).
  Future<bool> sendKeyCode(int keyCode, {pb.KeyEvent_Action action = pb.KeyEvent_Action.ACTION_DOWN}) async {
    final frame = pb.InputFrame(
      timestampMs: Int64(DateTime.now().millisecondsSinceEpoch),
      key: pb.KeyEvent(
        action: action,
        keyCode: keyCode,
      ),
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
