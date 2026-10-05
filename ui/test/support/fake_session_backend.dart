// Shared fake for the session/connection tests (Phase 5).
//
// Not a test file itself: `flutter test` only picks up `*_test.dart`.
//
// The fake can push transitions, answer (or refuse) snapshots, and count every
// call, so the controller's ordering and stale-event rules can be driven
// exactly instead of being inferred from sleep timings.

import 'dart:async';

import 'package:fixnum/fixnum.dart';
import 'package:phonebridge_ui/generated/phonebridge/localipc/v1/local_ipc.pb.dart' as ipc;
import 'package:phonebridge_ui/generated/phonebridge/v1/phonebridge.pb.dart' as pb;
import 'package:phonebridge_ui/services/session_backend.dart';

/// One recorded [SessionBackend.startSession] call, so a test can assert the
/// exact media tuple that left the UI (a dropped tuple is a silent capability
/// loss, which is exactly what these tests exist to prevent).
class RecordedStart {
  const RecordedStart({required this.target, this.requested});

  final String target;
  final pb.MediaParams? requested;
}

/// One recorded [SessionBackend.stopSession] call.
class RecordedStop {
  const RecordedStop({required this.sessionId, required this.reason});

  final String sessionId;
  final String reason;
}

class FakeSessionBackend implements SessionBackend {
  FakeSessionBackend({
    this.supportsSessions = true,
    this.activeSessionId = '',
  });

  @override
  final bool supportsSessions;

  /// Set to simulate a daemon that is already tracking a session (hydration).
  @override
  String activeSessionId;

  final StreamController<ipc.SessionEvent> events =
      StreamController<ipc.SessionEvent>.broadcast();

  /// Snapshot the backend answers with; when null, calls throw.
  ipc.GetSessionStateResponse? snapshot;

  /// When set, every snapshot call throws it (daemon unreachable).
  Object? snapshotError;

  int snapshotCalls = 0;

  final List<String> snapshotIds = <String>[];

  /// Control results. Defaults mirror a daemon that accepts the request.
  ipc.StartSessionResponse startResponse = ipc.StartSessionResponse(
    sessionId: 'sess-started',
    state: ipc.SessionState.SESSION_STATE_DISCOVERING,
  );
  Object? startError;

  ipc.StopSessionResponse stopResponse = ipc.StopSessionResponse(
    sessionId: 'sess-started',
    state: ipc.SessionState.SESSION_STATE_STOPPED,
  );
  Object? stopError;

  final List<RecordedStart> startCalls = <RecordedStart>[];
  final List<RecordedStop> stopCalls = <RecordedStop>[];

  @override
  Future<ipc.StartSessionResponse> startSession({
    required String target,
    pb.MediaParams? requested,
  }) async {
    startCalls.add(RecordedStart(target: target, requested: requested));
    final error = startError;
    if (error != null) throw error;
    activeSessionId = startResponse.sessionId;
    return startResponse;
  }

  @override
  Future<ipc.StopSessionResponse> stopSession({
    required String sessionId,
    String reason = 'user stopped',
  }) async {
    stopCalls.add(RecordedStop(sessionId: sessionId, reason: reason));
    final error = stopError;
    if (error != null) throw error;
    activeSessionId = '';
    return stopResponse;
  }

  @override
  Stream<ipc.SessionEvent> get sessionEventStream => events.stream;

  @override
  Future<ipc.GetSessionStateResponse> getSessionSnapshot(String sessionId) async {
    snapshotCalls++;
    snapshotIds.add(sessionId);
    final error = snapshotError;
    if (error != null) throw error;
    final value = snapshot;
    if (value == null) throw StateError('no snapshot configured');
    return value;
  }

  /// Pushes one transition and lets listeners run.
  Future<void> push(ipc.SessionEvent event) async {
    events.add(event);
    await Future<void>.delayed(Duration.zero);
  }

  Future<void> close() => events.close();
}

/// Builders so each test states only the field it is actually about.
ipc.SessionEvent sessionEvent({
  String sessionId = 'sess-1',
  ipc.SessionState state = ipc.SessionState.SESSION_STATE_CONNECTED,
  String reason = '',
  String errorMessage = '',
  ipc.SessionReason reasonCode = ipc.SessionReason.SESSION_REASON_NONE,
}) {
  return ipc.SessionEvent(
    sessionId: sessionId,
    state: state,
    reason: reason,
    errorMessage: errorMessage,
    reasonCode: reasonCode,
  );
}

ipc.GetSessionStateResponse sessionSnapshot({
  String sessionId = 'sess-1',
  String deviceId = '',
  ipc.SessionState state = ipc.SessionState.SESSION_STATE_STREAMING,
  int durationMs = 0,
  int reconnectAttempts = 0,
  String errorMessage = '',
  ipc.SessionReason reasonCode = ipc.SessionReason.SESSION_REASON_NONE,
  ipc.StreamStats? stats,
}) {
  return ipc.GetSessionStateResponse(
    sessionId: sessionId,
    deviceId: deviceId,
    state: state,
    errorMessage: errorMessage,
    reasonCode: reasonCode,
    connectedDurationMs: Int64(durationMs),
    reconnectAttempts: reconnectAttempts,
    stats: stats,
  );
}
