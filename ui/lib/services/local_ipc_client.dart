// Local IPC Client for PhoneBridge — Linux Desktop Transport (DEC-018)
//
// Implements UDS + gRPC client for phonebridge.localipc.v1 with:
// - Automatic discovery of UDS socket and bearer token in $XDG_RUNTIME_DIR
// - Token loading and caching with retry/backoff
// - Automatic re-authentication on UNAUTHENTICATED gRPC errors
// - StreamEvents resubscription with exponential backoff
// - Opaque pass-through handling of phonebridge.v1.Envelope messages

import 'dart:async';
import 'dart:io';

import 'package:fixnum/fixnum.dart';
import 'package:grpc/grpc.dart';

import '../generated/phonebridge/localipc/v1/local_ipc.pbgrpc.dart';
import '../generated/phonebridge/v1/phonebridge.pb.dart' as pb;

enum LocalIpcState {
  disconnected,
  connecting,
  connected,
  reconnecting,
  closed,
}

class LocalIpcClient {
  LocalIpcClient({
    String? socketPath,
    String? tokenPath,
    this.connectTimeout = const Duration(seconds: 3),
    this.callTimeout = const Duration(seconds: 10),
  })  : socketPath = socketPath ?? defaultSocketPath(),
        tokenPath = tokenPath ?? defaultTokenPath();

  final String socketPath;
  final String tokenPath;
  final Duration connectTimeout;
  final Duration callTimeout;

  ClientChannel? _channel;
  LocalEngineServiceClient? _stub;
  String? _cachedToken;

  LocalIpcState _state = LocalIpcState.disconnected;
  LocalIpcState get state => _state;

  int? negotiatedVersion;
  String? serverVersion;
  int? daemonGeneration;

  final StreamController<LocalIpcState> _stateController =
      StreamController<LocalIpcState>.broadcast();
  Stream<LocalIpcState> get onStateChanged => _stateController.stream;

  static String _resolveRuntimeDir() {
    final xdg = Platform.environment['XDG_RUNTIME_DIR'];
    if (xdg != null && xdg.isNotEmpty) {
      return xdg;
    }
    try {
      final res = Process.runSync('id', ['-u']);
      if (res.exitCode == 0) {
        final uid = res.stdout.toString().trim();
        final runUser = Directory('/run/user/$uid');
        if (runUser.existsSync()) {
          return runUser.path;
        }
        return '${Directory.systemTemp.path}/phonebridge-$uid';
      }
    } catch (_) {}
    return Directory.systemTemp.path;
  }

  static String defaultSocketPath() {
    final runtimeDir = _resolveRuntimeDir();
    return '$runtimeDir/phonebridge/engine.sock';
  }

  static String defaultTokenPath() {
    final runtimeDir = _resolveRuntimeDir();
    return '$runtimeDir/phonebridge/token';
  }

  /// Acquires the bearer token from disk, with exponential backoff if the
  /// daemon has not yet finished creating the file.
  Future<String> acquireToken({
    Duration timeout = const Duration(seconds: 5),
  }) async {
    if (_cachedToken != null && _cachedToken!.isNotEmpty) {
      return _cachedToken!;
    }

    final deadline = DateTime.now().add(timeout);
    var backoffMs = 25;

    while (DateTime.now().isBefore(deadline)) {
      final file = File(tokenPath);
      if (await file.exists()) {
        final content = (await file.readAsString()).trim();
        if (content.isNotEmpty) {
          _cachedToken = content;
          return content;
        }
      }
      await Future.delayed(Duration(milliseconds: backoffMs));
      if (backoffMs < 200) {
        backoffMs *= 2;
      }
    }
    throw TimeoutException(
      'Timed out waiting for local IPC token file at $tokenPath',
      timeout,
    );
  }

  void invalidateToken() {
    _cachedToken = null;
  }

  void connect() {
    if (_channel != null) return;
    _setState(LocalIpcState.connecting);

    _channel = ClientChannel(
      InternetAddress(socketPath, type: InternetAddressType.unix),
      port: 0,
      options: ChannelOptions(
        credentials: const ChannelCredentials.insecure(),
        connectTimeout: connectTimeout,
      ),
    );
    _stub = LocalEngineServiceClient(_channel!);
  }

  LocalEngineServiceClient get _service {
    if (_stub == null) {
      connect();
    }
    return _stub!;
  }

  Future<CallOptions> _makeCallOptions({
    Duration? timeout,
    bool noTimeout = false,
  }) async {
    final tok = await acquireToken();
    return CallOptions(
      metadata: {'authorization': 'Bearer $tok'},
      timeout: noTimeout ? null : (timeout ?? callTimeout),
    );
  }

  void _setState(LocalIpcState newState) {
    if (_state != newState) {
      _state = newState;
      if (!_stateController.isClosed) {
        _stateController.add(_state);
      }
    }
  }

  /// Executes an RPC, retrying with a freshly read token if UNAUTHENTICATED.
  Future<T> _callWithAuth<T>(Future<T> Function(CallOptions opts) call) async {
    try {
      final opts = await _makeCallOptions();
      final res = await call(opts);
      _setState(LocalIpcState.connected);
      return res;
    } on GrpcError catch (e) {
      if (e.code == StatusCode.unauthenticated) {
        // Token might have rotated across daemon restart
        invalidateToken();
        final opts = await _makeCallOptions();
        final res = await call(opts);
        _setState(LocalIpcState.connected);
        return res;
      }
      rethrow;
    }
  }

  Future<HandshakeResponse> handshake({int clientVersion = 1}) async {
    final resp = await _callWithAuth(
      (opts) => _service.handshake(
        HandshakeRequest(clientVersion: clientVersion),
        options: opts,
      ),
    );
    negotiatedVersion = resp.negotiatedVersion;
    serverVersion = resp.serverVersion;
    daemonGeneration = resp.daemonGeneration.toInt();
    return resp;
  }

  Future<PingResponse> ping({int nonce = 1}) async {
    return _callWithAuth(
      (opts) => _service.ping(
        PingRequest(nonce: Int64(nonce)),
        options: opts,
      ),
    );
  }

  Future<HealthResponse> health() async {
    final resp = await _callWithAuth(
      (opts) => _service.health(
        HealthRequest(),
        options: opts,
      ),
    );
    serverVersion = resp.serverVersion;
    daemonGeneration = resp.daemonGeneration.toInt();
    return resp;
  }

  /// Initiates a LAN session with the target device.
  ///
  /// [requested] is the media tuple asked for. It must be settled *before* the
  /// offer exists (DEC-022), so it travels with the request rather than being
  /// applied afterwards. Omitted fields mean "device default" — which the phone
  /// reports back as unknown rather than assuming it equals the request.
  Future<StartSessionResponse> startSession(
    String deviceId, {
    pb.MediaParams? requested,
  }) async {
    return _callWithAuth(
      (opts) => _service.startSession(
        StartSessionRequest(deviceId: deviceId, requested: requested),
        options: opts,
      ),
    );
  }

  /// Terminates an active or in-progress session.
  Future<StopSessionResponse> stopSession({
    required String sessionId,
    String reason = 'user stopped',
  }) async {
    return _callWithAuth(
      (opts) => _service.stopSession(
        StopSessionRequest(sessionId: sessionId, reason: reason),
        options: opts,
      ),
    );
  }

  /// Queries the current point-in-time session snapshot.
  Future<GetSessionStateResponse> getSessionState(
      {String sessionId = ''}) async {
    return _callWithAuth(
      (opts) => _service.getSessionState(
        GetSessionStateRequest(sessionId: sessionId),
        options: opts,
      ),
    );
  }

  /// Discovers active PhoneBridge devices on LAN.
  Future<ListDevicesResponse> listDevices() async {
    return _callWithAuth(
      (opts) => _service.listDevices(
        ListDevicesRequest(),
        options: opts,
      ),
    );
  }

  /// Initiates pairing with a discovered LAN device.
  Future<PairDeviceResponse> pairDevice(String deviceId) async {
    return _callWithAuth(
      (opts) => _service.pairDevice(
        PairDeviceRequest(deviceId: deviceId),
        options: opts,
      ),
    );
  }

  /// Completes pairing by confirming or rejecting the SAS code.
  Future<ConfirmPairingResponse> confirmPairing({
    required String deviceId,
    required bool userConfirmed,
  }) async {
    return _callWithAuth(
      (opts) => _service.confirmPairing(
        ConfirmPairingRequest(
          deviceId: deviceId,
          userConfirmed: userConfirmed,
        ),
        options: opts,
      ),
    );
  }

  /// Lists all trusted/paired devices from the daemon's trust store.
  Future<ListTrustedDevicesResponse> listTrustedDevices() async {
    return _callWithAuth(
      (opts) => _service.listTrustedDevices(
        ListTrustedDevicesRequest(),
        options: opts,
      ),
    );
  }

  /// Revokes trust for a previously paired device.
  Future<RevokeDeviceResponse> revokeDevice(String deviceId) async {
    return _callWithAuth(
      (opts) => _service.revokeDevice(
        RevokeDeviceRequest(deviceId: deviceId),
        options: opts,
      ),
    );
  }

  /// Gets current clipboard sync and adapter status from daemon.
  Future<GetClipboardStatusResponse> getClipboardStatus() async {
    return _callWithAuth(
      (opts) => _service.getClipboardStatus(
        GetClipboardStatusRequest(),
        options: opts,
      ),
    );
  }

  /// Triggers a manual clipboard sync cycle from daemon.
  Future<TriggerClipboardPullResponse> triggerClipboardPull() async {
    return _callWithAuth(
      (opts) => _service.triggerClipboardPull(
        TriggerClipboardPullRequest(),
        options: opts,
      ),
    );
  }

  /// Pushed stream of session lifecycle events.
  Stream<SessionEvent> get onSessionEvents => streamEvents()
      .where((event) => event.hasSessionEvent())
      .map((event) => event.sessionEvent);

  /// Pushed stream of clipboard status events.
  Stream<ClipboardStatusEvent> get onClipboardEvents => streamEvents()
      .where((event) => event.hasClipboardEvent())
      .map((event) => event.clipboardEvent);

  /// Pushed stream of file-transfer transitions (DEC-024). Every state and
  /// progress change for a transfer arrives here, so the UI never polls.
  Stream<TransferEvent> get onTransferEvents => streamEvents()
      .where((event) => event.hasTransferEvent())
      .map((event) => event.transferEvent);

  /// Pushed stream of notification events (DEC-028, Phase 8 v0.1).
  Stream<pb.NotificationFrame> get onNotificationEvents => streamEvents()
      .where((event) => event.hasNotificationEvent())
      .map((event) => event.notificationEvent);

  /// Offers a local file to the active peer over the session's "transfer"
  /// DataChannel. [deviceId] empty means "the active session's peer"; the
  /// daemon reads [localPath] itself, so file bytes never cross local IPC.
  Future<SendFileResponse> sendFile({
    String deviceId = '',
    required String localPath,
    String filename = '',
  }) async {
    return _callWithAuth(
      (opts) => _service.sendFile(
        SendFileRequest(
          deviceId: deviceId,
          localPath: localPath,
          filename: filename,
        ),
        options: opts,
      ),
    );
  }

  /// Aborts an in-flight transfer in either direction.
  Future<CancelTransferResponse> cancelTransfer(String transferId) async {
    return _callWithAuth(
      (opts) => _service.cancelTransfer(
        CancelTransferRequest(transferId: transferId),
        options: opts,
      ),
    );
  }

  /// In-flight transfers plus the recent in-memory history, newest first.
  Future<ListTransfersResponse> listTransfers() async {
    return _callWithAuth(
      (opts) => _service.listTransfers(
        ListTransfersRequest(),
        options: opts,
      ),
    );
  }

  /// Dispatches an input event to the active session via unary SendInput (DEC-027).
  Future<SendInputResponse> sendInput(SendInputRequest request) async {
    return _callWithAuth(
      (opts) => _service.sendInput(
        request,
        options: opts,
      ),
    );
  }

  /// Lists active notifications currently held in daemon memory (DEC-028).
  Future<ListNotificationsResponse> listNotifications() async {
    return _callWithAuth(
      (opts) => _service.listNotifications(
        ListNotificationsRequest(),
        options: opts,
      ),
    );
  }

  /// Subscribes to relayed device events via server streaming.
  /// Automatically attempts resubscription with backoff on disconnect.
  Stream<StreamEventsResponse> streamEvents({
    bool autoReconnect = true,
  }) {
    late StreamController<StreamEventsResponse> controller;
    StreamSubscription<StreamEventsResponse>? subscription;
    bool isClosed = false;

    void startSubscription() async {
      if (isClosed) return;
      try {
        final opts = await _makeCallOptions(noTimeout: true);
        final stream = _service.streamEvents(
          StreamEventsRequest(),
          options: opts,
        );

        subscription = stream.listen(
          (event) {
            _setState(LocalIpcState.connected);
            controller.add(event);
          },
          onError: (err) {
            if (isClosed) return;
            _setState(LocalIpcState.reconnecting);
            controller.addError(err);
            if (autoReconnect) {
              Future.delayed(const Duration(milliseconds: 500), () {
                if (!isClosed) startSubscription();
              });
            }
          },
          onDone: () {
            if (isClosed) return;
            _setState(LocalIpcState.reconnecting);
            if (autoReconnect) {
              Future.delayed(const Duration(milliseconds: 500), () {
                if (!isClosed) startSubscription();
              });
            } else {
              controller.close();
            }
          },
          cancelOnError: true,
        );
      } catch (e) {
        if (isClosed) return;
        _setState(LocalIpcState.reconnecting);
        controller.addError(e);
        if (autoReconnect) {
          Future.delayed(const Duration(milliseconds: 1000), () {
            if (!isClosed) startSubscription();
          });
        }
      }
    }

    controller = StreamController<StreamEventsResponse>(
      onListen: () {
        startSubscription();
      },
      onCancel: () {
        isClosed = true;
        subscription?.cancel();
      },
    );

    return controller.stream;
  }

  /// Subscribes to the daemon's frame stream (Phase 6 Slice 3A): one
  /// [StreamFramesResponse] per <=64 KiB JPEG chunk of the active session.
  ///
  /// This is a SEPARATE RPC from [streamEvents] by contract — frame
  /// backpressure can never cost a client a control event (one-consumer rule:
  /// this method must never be folded into the event subscription).
  /// Resubscribes with the same backoff as the event stream, so a daemon
  /// restart heals both streams independently.
  Stream<StreamFramesResponse> streamFrames({
    bool autoReconnect = true,
  }) {
    late StreamController<StreamFramesResponse> controller;
    StreamSubscription<StreamFramesResponse>? subscription;
    bool isClosed = false;

    void startSubscription() async {
      if (isClosed) return;
      try {
        final opts = await _makeCallOptions(noTimeout: true);
        final stream = _service.streamFrames(
          StreamFramesRequest(),
          options: opts,
        );

        subscription = stream.listen(
          (frame) {
            if (!controller.isClosed) controller.add(frame);
          },
          onError: (err) {
            if (isClosed) return;
            if (!controller.isClosed) controller.addError(err);
            if (autoReconnect) {
              Future.delayed(const Duration(milliseconds: 500), () {
                if (!isClosed) startSubscription();
              });
            }
          },
          onDone: () {
            if (isClosed) return;
            if (autoReconnect) {
              Future.delayed(const Duration(milliseconds: 500), () {
                if (!isClosed) startSubscription();
              });
            } else {
              controller.close();
            }
          },
          cancelOnError: true,
        );
      } catch (e) {
        if (isClosed) return;
        if (!controller.isClosed) controller.addError(e);
        if (autoReconnect) {
          Future.delayed(const Duration(seconds: 1), () {
            if (!isClosed) startSubscription();
          });
        }
      }
    }

    controller = StreamController<StreamFramesResponse>(
      onListen: startSubscription,
      onCancel: () {
        isClosed = true;
        subscription?.cancel();
      },
    );

    return controller.stream;
  }

  Future<void> shutdown() async {
    _setState(LocalIpcState.closed);
    final ch = _channel;
    _channel = null;
    _stub = null;
    if (ch != null) {
      await ch.shutdown();
    }
    await _stateController.close();
  }
}
