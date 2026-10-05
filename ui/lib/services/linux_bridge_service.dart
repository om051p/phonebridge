import 'dart:async';
import '../generated/phonebridge/localipc/v1/local_ipc.pbgrpc.dart' as ipc;
import '../models/capture_stats.dart';
import '../models/clipboard_status.dart';
import '../models/device_state.dart';
import '../models/discovered_device.dart';
import '../models/inbound_pairing.dart';
import '../models/notification_item.dart';
import '../generated/phonebridge/v1/phonebridge.pb.dart' as pb;
import '../models/transfer_item.dart';
import '../models/trusted_device.dart';
import 'frame_stream.dart';
import 'local_ipc_client.dart' show LocalIpcClient, LocalIpcState;
import 'notification_backend.dart';
import 'pairing_backend.dart';
import 'platform_bridge_service.dart';
import 'session_backend.dart';

class LinuxBridgeService
    implements
        PlatformBridgeService,
        SessionBackend,
        ProvidesFrameStream,
        NotificationBackend,
        PairingBackend {
  LinuxBridgeService({LocalIpcClient? client})
      : _client = client ?? LocalIpcClient() {
    _initStream();
  }

  final LocalIpcClient _client;
  LocalIpcClient get client => _client;
  final StreamController<Map<dynamic, dynamic>> _rawEventsController =
      StreamController<Map<dynamic, dynamic>>.broadcast();
  final StreamController<CaptureStats> _statsController =
      StreamController<CaptureStats>.broadcast();
  final StreamController<TransferItem> _transferController =
      StreamController<TransferItem>.broadcast();
  final StreamController<pb.NotificationFrame> _notificationStreamController =
      StreamController<pb.NotificationFrame>.broadcast();

  /// Session transitions, forwarded from the single IPC subscription below
  /// rather than opened as a second stream: the app-level session model must
  /// not cost an extra RPC stream to the daemon.
  final StreamController<ipc.SessionEvent> _sessionEventsController =
      StreamController<ipc.SessionEvent>.broadcast();

  /// Inbound-pairing lifecycle transitions (Phase 2), forwarded from the same
  /// single IPC subscription.
  final StreamController<ipc.PairingEvent> _pairingEventsController =
      StreamController<ipc.PairingEvent>.broadcast();

  StreamSubscription<ipc.StreamEventsResponse>? _streamSub;
  String? _activeSessionId;
  bool _disposed = false;
  CaptureStats _lastStats = CaptureStats.initial;
  String _lastClipboardState = 'STOPPED';

  @override
  bool get isAndroid => false;

  @override
  bool get isLinux => true;

  void _initStream() {
    _streamSub = _client.streamEvents().listen(
      (resp) {
        // One pulse per relayed batch, for screens that only count events.
        if (!_eventPulse.isClosed) {
          _eventPulse.add(null);
        }
        final map = <dynamic, dynamic>{};

        if (resp.hasSessionEvent()) {
          final se = resp.sessionEvent;
          _activeSessionId = se.sessionId;
          if (!_sessionEventsController.isClosed) {
            _sessionEventsController.add(se);
          }
          final isStreaming =
              se.state == ipc.SessionState.SESSION_STATE_STREAMING;
          final isStopped =
              se.state == ipc.SessionState.SESSION_STATE_STOPPED ||
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

        if (resp.hasTransferEvent()) {
          final te = resp.transferEvent;
          if (te.hasTransfer()) {
            final item = TransferItem.fromProto(te.transfer);
            map['transfer'] = item;
            if (!_transferController.isClosed) {
              _transferController.add(item);
            }
          }
        }

        if (resp.hasNotificationEvent()) {
          final ne = resp.notificationEvent;
          if (!_notificationStreamController.isClosed) {
            _notificationStreamController.add(ne);
          }
        }

        if (resp.hasPairingEvent()) {
          final pe = resp.pairingEvent;
          if (!_pairingEventsController.isClosed) {
            _pairingEventsController.add(pe);
          }
          map['pairingEvent'] = pe.type.value;
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
        final snap =
            await _client.getSessionState(sessionId: _activeSessionId!);
        final isStreaming =
            snap.state == ipc.SessionState.SESSION_STATE_STREAMING;
        final fps = snap.hasActual() && snap.actual.fps > 0
            ? snap.actual.fps.toDouble()
            : null;
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
      final resp = await _client.removeDevice(deviceId);
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
      final mapped = ClipboardSyncState.fromString(resp.state);
      // The master switch is user intent (Flutter-owned on Linux); the daemon
      // only reports adapter reality. STOPPED means the service is off;
      // setup/unavailable states keep the switch on so the UI shows what to
      // fix instead of pretending sync was turned off.
      final enabled = mapped != ClipboardSyncState.stopped;
      return ClipboardStatus(
        isEnabled: enabled,
        state: mapped,
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

  /// The app's capture control path. The negotiated-parameter screen's selection
  /// is now actually *requested* instead of being dropped at this boundary — the
  /// daemon has always accepted a requested tuple (DEC-022); only the transport of
  /// it was missing here, which meant the phone silently applied its own defaults
  /// while the UI showed the user's chosen preset.
  ///
  /// Explicit target only (Phase C): an empty target fails instead of falling
  /// back to `peer-auto`. The daemon keeps `peer-auto` for internal/test use,
  /// but user-initiated connects always name their device.
  @override
  Future<bool> startCapture({
    String? receiverUrl,
    int width = 720,
    int height = 1600,
    int fps = 30,
    int bitrateKbps = 2500,
  }) async {
    final target =
        receiverUrl != null && receiverUrl.isNotEmpty ? receiverUrl : null;
    if (target == null || target.isEmpty) return false;
    try {
      final resp = await startSession(
        target: target,
        requested: mediaParamsRequest(
          width: width,
          height: height,
          fps: fps,
          bitrateKbps: bitrateKbps,
        ),
      );
      return resp.sessionId.isNotEmpty;
    } catch (_) {
      return false;
    }
  }

  @override
  Future<bool> stopCapture() async {
    if (_activeSessionId == null || _activeSessionId!.isEmpty) return true;
    try {
      final resp = await stopSession(
        sessionId: _activeSessionId!,
        reason: 'user stopped',
      );
      return resp.sessionId.isNotEmpty;
    } catch (_) {
      return false;
    }
  }

  /// Dispatches remote input to the active session via Local IPC (DEC-027).
  Future<bool> sendInput(pb.InputFrame frame, {String sessionId = ''}) async {
    final target = sessionId.isNotEmpty ? sessionId : (_activeSessionId ?? '');
    try {
      final resp = await _client.sendInput(
        ipc.SendInputRequest(
          sessionId: target,
          frame: frame,
        ),
      );
      return resp.delivered;
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
                isStale: d.isStale,
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

  // -------------------------------------------------------------------------
  // Inbound pairing (Phase 2 bidirectional pairing): pairing requests that
  // arrive at THIS device's signaling server and await the local user's
  // explicit approval. State lives in the daemon; the UI reads and answers.
  // -------------------------------------------------------------------------

  /// Snapshots the pairing requests awaiting local approval. Expired entries
  /// are swept daemon-side; an empty list is the truth after a timeout.
  @override
  Future<List<InboundPairing>> listInboundPairings() async {
    try {
      final resp = await _client.listInboundPairings();
      return resp.requests
          .map((r) => InboundPairing(
                token: r.pairingToken,
                remoteName: r.remoteName,
                remotePlatform: r.remotePlatform,
                sas: r.sas,
                createdAtMs: r.createdAtMs.toInt(),
              ))
          .toList();
    } catch (_) {
      return [];
    }
  }

  /// Records the local user's decision. False means the token was unknown or
  /// already expired; trust is committed daemon-side on the requester's next
  /// signed confirm, never by this call.
  @override
  Future<bool> respondInboundPairing({
    required String pairingToken,
    required bool approved,
  }) async {
    try {
      final resp = await _client.respondInboundPairing(
        pairingToken: pairingToken,
        approved: approved,
      );
      return resp.recorded;
    } catch (_) {
      return false;
    }
  }

  /// Inbound-pairing lifecycle transitions (ARRIVED / WITHDRAWN) relayed from
  /// the daemon on the same single subscription as every other event.
  Stream<ipc.PairingEvent> get pairingEventStream =>
      _pairingEventsController.stream;

  @override
  bool get supportsFileTransfer => true;

  @override
  bool get supportsNotifications => true;

  @override
  Stream<pb.NotificationFrame> get notificationStream =>
      _notificationStreamController.stream;

  @override
  Future<List<NotificationItem>> listNotifications() async {
    final resp = await _client.listNotifications();
    return resp.notifications.map(NotificationItem.fromProto).toList();
  }

  /// Transfer history as the daemon sees it. Unlike the other read paths this
  /// one rethrows, because the UI must tell "no transfers yet" apart from
  /// "daemon unreachable / transfer engine disabled".
  @override
  Future<List<TransferItem>> listTransfers() async {
    final resp = await _client.listTransfers();
    return resp.transfers.map(TransferItem.fromProto).toList();
  }

  @override
  Future<TransferSendResult> sendFile({
    required String localPath,
    String filename = '',
    String deviceId = '',
  }) async {
    try {
      final resp = await _client.sendFile(
        deviceId: deviceId,
        localPath: localPath,
        filename: filename,
      );
      return TransferSendResult(
        transferId: resp.transferId,
        state: resp.state,
        reasonCode: resp.reasonCode,
        errorMessage: resp.errorMessage,
      );
    } catch (e) {
      return TransferSendResult.failure(e.toString());
    }
  }

  @override
  Future<bool> cancelTransfer(String transferId) async {
    try {
      final resp = await _client.cancelTransfer(transferId);
      return resp.cancelled;
    } catch (_) {
      return false;
    }
  }

  @override
  Stream<TransferItem> get transferStream => _transferController.stream;

  @override
  Stream<CaptureStats> get statsStream => _statsController.stream;

  @override
  Stream<Map<dynamic, dynamic>> get rawEventsStream =>
      _rawEventsController.stream;

  @override
  void setNativeCallHandler(Future<dynamic> Function(dynamic call)? handler) {
    // Desktop platform does not use Android MethodChannel handlers
  }

  // -------------------------------------------------------------------------
  // Session state seam (Phase 5): the app-level model reads session state from
  // here, so a session's state is visible to every screen instead of only the
  // one widget that happened to hold it.
  // -------------------------------------------------------------------------

  @override
  bool get supportsSessions => true;

  @override
  String get activeSessionId => _activeSessionId ?? '';

  @override
  Stream<ipc.SessionEvent> get sessionEventStream =>
      _sessionEventsController.stream;

  @override
  Future<ipc.GetSessionStateResponse> getSessionSnapshot(String sessionId) =>
      _client.getSessionState(sessionId: sessionId);

  @override
  Future<ipc.StartSessionResponse> startSession({
    required String target,
    pb.MediaParams? requested,
  }) async {
    final resp = await _client.startSession(target, requested: requested);
    _activeSessionId = resp.sessionId;
    return resp;
  }

  @override
  Future<ipc.StopSessionResponse> stopSession({
    required String sessionId,
    String reason = 'user stopped',
  }) async {
    final resp =
        await _client.stopSession(sessionId: sessionId, reason: reason);
    if (resp.sessionId.isNotEmpty) {
      _activeSessionId = null;
    }
    return resp;
  }

  // -------------------------------------------------------------------------
  // Device / diagnostics seam (Phase 5): the production surfaces (Devices,
  // Diagnostics) read discovery, trust and daemon transport facts through the
  // app-owned service instead of owning a second IPC client. Discovery
  // staleness travels on the model-typed mapping; pairing feedback rides the
  // controller's error channel.
  // -------------------------------------------------------------------------

  /// Last known transport state of the underlying client.
  LocalIpcState get ipcState => _client.state;

  /// Local transport state changes of the underlying client. This is the
  /// client's own broadcast stream — a local signal, **not** a second daemon
  /// subscription; screens may all listen without adding IPC traffic.
  Stream<LocalIpcState> get ipcStateStream => _client.onStateChanged;

  final StreamController<void> _eventPulse = StreamController<void>.broadcast();

  /// Fires once per event batch relayed by the daemon on this service's single
  /// subscription. Screens that only need to *count* relayed events listen to
  /// this pulse instead of opening their own `streamEvents()` subscription,
  /// which is how the one-consumer rule is kept without losing the feature.
  Stream<void> get eventPulse => _eventPulse.stream;

  /// Where the daemon socket lives (rendered in the Diagnostics card).
  String get socketPath => _client.socketPath;

  Future<ipc.HandshakeResponse> handshake() => _client.handshake();

  Future<ipc.PingResponse> ping({int nonce = 1}) => _client.ping(nonce: nonce);

  Future<ipc.HealthResponse> health() => _client.health();

  // -------------------------------------------------------------------------
  // Frame stream seam (Phase 6 Slice 3A): a SEPARATE daemon RPC stream from
  // the single streamEvents subscription above — frame load can never cost
  // the app a control event (one-consumer rule is about the event stream).
  // -------------------------------------------------------------------------

  @override
  FrameStream createFrameStream() =>
      FrameStream(source: () => _client.streamFrames());

  @override
  void dispose() {
    // Idempotent: the app-level owner and a standalone screen can both tear
    // the service down, and closing a StreamController twice throws.
    if (_disposed) return;
    _disposed = true;
    _streamSub?.cancel();
    _rawEventsController.close();
    _statsController.close();
    _transferController.close();
    _notificationStreamController.close();
    _sessionEventsController.close();
    _pairingEventsController.close();
    _eventPulse.close();
    _client.shutdown();
  }
}
