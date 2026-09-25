// Shared fake for the Linux IPC tests.
//
// Not a test file itself: `flutter test` only picks up `*_test.dart`.
//
// Serves two purposes: it lets the bridge/view tests drive the event stream
// exactly, and it counts how many times an event subscription is opened — which
// is how the "one consumer of the daemon's stream" rule is asserted instead of
// assumed.

import 'dart:async';

import 'package:fixnum/fixnum.dart';
import 'package:phonebridge_ui/generated/phonebridge/localipc/v1/local_ipc.pb.dart' as ipc;
import 'package:phonebridge_ui/generated/phonebridge/v1/phonebridge.pb.dart' as pb;
import 'package:phonebridge_ui/services/local_ipc_client.dart';

/// One recorded [LocalIpcClient.startSession] call.
class RecordedStartSession {
  const RecordedStartSession({required this.target, this.requested});

  final String target;
  final pb.MediaParams? requested;
}

/// One recorded [LocalIpcClient.stopSession] call.
class RecordedStopSession {
  const RecordedStopSession({required this.sessionId, required this.reason});

  final String sessionId;
  final String reason;
}

/// One recorded pairing confirmation.
class RecordedConfirmPairing {
  const RecordedConfirmPairing({required this.deviceId, required this.userConfirmed});

  final String deviceId;
  final bool userConfirmed;
}

class FakeIpcClient extends LocalIpcClient {
  FakeIpcClient({this.connected = true})
      : super(socketPath: '/nonexistent.sock', tokenPath: '/nonexistent.token');

  /// When false, handshake throws — the "daemon is not running" case.
  final bool connected;

  final StreamController<ipc.StreamEventsResponse> events =
      StreamController<ipc.StreamEventsResponse>.broadcast();

  /// How many event subscriptions were opened against this client. The bridge
  /// service opens exactly one; a screen must open none.
  int streamEventsCalls = 0;

  ipc.GetSessionStateResponse? snapshot;
  Object? snapshotError;
  final List<String> snapshotIds = <String>[];

  ipc.HandshakeResponse handshakeResponse = ipc.HandshakeResponse(
    negotiatedVersion: 1,
    serverVersion: '0.1.0',
    daemonGeneration: Int64(1),
  );
  Object? handshakeError;

  /// Call counters for the ownership tests: they assert which operations ran
  /// through this client and how often.
  int handshakeCalls = 0;
  int listDevicesCalls = 0;
  int listTrustedDevicesCalls = 0;
  int startPairingCalls = 0;
  int completePairingCalls = 0;
  int pingCalls = 0;
  int healthCalls = 0;

  ipc.HealthResponse healthResponse = ipc.HealthResponse(
    serverVersion: '0.1.0',
    daemonGeneration: Int64(1),
  );

  /// Controller-path stubs (used when a [PhoneBridgeController] rides this
  /// client through the real [LinuxBridgeService]).
  ipc.ListTransfersResponse listTransfersResponse =
      ipc.ListTransfersResponse();
  ipc.GetClipboardStatusResponse clipboardStatusResponse =
      ipc.GetClipboardStatusResponse(state: 'RUNNING');
  ipc.TriggerClipboardPullResponse clipboardPullResponse =
      ipc.TriggerClipboardPullResponse(success: true);

  /// Pairing outcomes.
  ipc.PairDeviceResponse pairResponse = ipc.PairDeviceResponse(
    deviceId: 'poco-f5',
    displayName: 'POCO F5',
    sas: '736500',
  );
  Object? pairError;
  ipc.ConfirmPairingResponse confirmPairingResponse =
      ipc.ConfirmPairingResponse(success: true);
  Object? confirmPairingError;

  RecordedConfirmPairing? get lastConfirmedPairing =>
      confirmPairingCalls.isEmpty ? null : confirmPairingCalls.last;

  final List<RecordedConfirmPairing> confirmPairingCalls =
      <RecordedConfirmPairing>[];

  /// Devices the daemon reports as discovered.
  List<ipc.DiscoveredDevice> devices = <ipc.DiscoveredDevice>[];

  ipc.StartSessionResponse startResponse = ipc.StartSessionResponse(
    sessionId: 'sess-1',
    state: ipc.SessionState.SESSION_STATE_DISCOVERING,
  );
  Object? startError;

  ipc.StopSessionResponse stopResponse = ipc.StopSessionResponse(
    sessionId: 'sess-1',
    state: ipc.SessionState.SESSION_STATE_STOPPED,
  );
  Object? stopError;

  final List<RecordedStartSession> startCalls = <RecordedStartSession>[];
  final List<RecordedStopSession> stopCalls = <RecordedStopSession>[];

  int shutdownCalls = 0;

  @override
  Stream<ipc.StreamEventsResponse> streamEvents({bool autoReconnect = true}) {
    streamEventsCalls++;
    return events.stream;
  }

  /// Frames ride a SEPARATE daemon stream (Phase 6 Slice 3A). The fake counts
  /// both so tests can assert the one-consumer rule (events) and the
  /// frame-stream contract (frames) at once.
  final StreamController<ipc.StreamFramesResponse> frames =
      StreamController<ipc.StreamFramesResponse>.broadcast();

  int streamFramesCalls = 0;

  @override
  Stream<ipc.StreamFramesResponse> streamFrames({
    bool autoReconnect = true,
  }) {
    streamFramesCalls++;
    return frames.stream;
  }

  @override
  Future<ipc.HandshakeResponse> handshake({int clientVersion = 1}) async {
    handshakeCalls++;
    final error = handshakeError;
    if (error != null) throw error;
    if (!connected) throw StateError('daemon not reachable');
    return handshakeResponse;
  }

  @override
  Future<ipc.ListDevicesResponse> listDevices() async {
    listDevicesCalls++;
    return ipc.ListDevicesResponse(devices: devices);
  }

  /// Trust-store entries the daemon reports.
  List<ipc.TrustedDevice> trustedDevices = <ipc.TrustedDevice>[];

  @override
  Future<ipc.ListTrustedDevicesResponse> listTrustedDevices() async {
    listTrustedDevicesCalls++;
    return ipc.ListTrustedDevicesResponse(devices: trustedDevices);
  }

  @override
  Future<ipc.GetSessionStateResponse> getSessionState({
    String sessionId = '',
  }) async {
    snapshotIds.add(sessionId);
    final error = snapshotError;
    if (error != null) throw error;
    final value = snapshot;
    if (value == null) throw StateError('no snapshot configured');
    return value;
  }

  @override
  Future<ipc.StartSessionResponse> startSession(
    String deviceId, {
    pb.MediaParams? requested,
  }) async {
    startCalls.add(RecordedStartSession(target: deviceId, requested: requested));
    final error = startError;
    if (error != null) throw error;
    return startResponse;
  }

  @override
  Future<ipc.StopSessionResponse> stopSession({
    required String sessionId,
    String reason = 'user stopped',
  }) async {
    stopCalls.add(RecordedStopSession(sessionId: sessionId, reason: reason));
    final error = stopError;
    if (error != null) throw error;
    return stopResponse;
  }

  @override
  Future<ipc.PairDeviceResponse> pairDevice(String deviceId) async {
    startPairingCalls++;
    final error = pairError;
    if (error != null) throw error;
    return pairResponse;
  }

  @override
  Future<ipc.ConfirmPairingResponse> confirmPairing({
    required String deviceId,
    required bool userConfirmed,
  }) async {
    completePairingCalls++;
    confirmPairingCalls
        .add(RecordedConfirmPairing(deviceId: deviceId, userConfirmed: userConfirmed));
    final error = confirmPairingError;
    if (error != null) throw error;
    return confirmPairingResponse;
  }

  /// Device ids whose trust was revoked through this client, in order.
  final List<String> revokeDeviceCalls = <String>[];

  @override
  Future<ipc.RevokeDeviceResponse> revokeDevice(String deviceId) async {
    revokeDeviceCalls.add(deviceId);
    return ipc.RevokeDeviceResponse(success: true);
  }

  @override
  Future<ipc.ListTransfersResponse> listTransfers() async =>
      listTransfersResponse;

  @override
  Future<ipc.GetClipboardStatusResponse> getClipboardStatus() async =>
      clipboardStatusResponse;

  @override
  Future<ipc.TriggerClipboardPullResponse> triggerClipboardPull() async =>
      clipboardPullResponse;

  @override
  Future<ipc.PingResponse> ping({int nonce = 1}) async {
    pingCalls++;
    return ipc.PingResponse(nonce: Int64(nonce));
  }

  @override
  Future<ipc.HealthResponse> health() async {
    healthCalls++;
    return healthResponse;
  }

  @override
  Future<void> shutdown() async {
    shutdownCalls++;
  }
}
