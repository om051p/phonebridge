// App-level session state (Phase 5, first slice).
//
// Ownership move, not new behaviour: [SessionStatus] and its transitions
// already existed, but they were owned by a single screen, so the rest of the
// app could not see them and inferred "connected" from a capture boolean. This
// controller is now the only owner. It reuses the existing rules
// ([SessionStatus.fromEvent] / [SessionStatus.fromSnapshot]) unchanged — no new
// protocol meaning is introduced here.
//
// Event ordering is the hard part of any pushed-state model, so the three
// protections are explicit and tested:
//
//  * **Correlation.** A transition that belongs to a *different* session than
//    the live one is a late event from a session that has already been
//    replaced; applying it would paint the old session's failure over the new
//    session's state. It is dropped.
//  * **No resurrection.** A snapshot that would move a terminal session back
//    into an active state is stale by construction (the daemon issues a new id
//    for every new session), so it is dropped. This mirrors the transfer
//    controller's "a snapshot never downgrades a terminal row" rule.
//  * **No noise.** A transition identical to the current status does not
//    notify, so identical repeats cannot rebuild the widget tree every second.

import 'dart:async';

import 'package:flutter/foundation.dart';

import '../generated/phonebridge/localipc/v1/local_ipc.pb.dart' as ipc;
import '../generated/phonebridge/v1/phonebridge.pb.dart' as pb;
import '../models/media_capabilities.dart';
import '../models/session_status.dart';
import '../services/session_backend.dart';

class SessionController extends ChangeNotifier {
  SessionController({
    required SessionBackend backend,
    Duration telemetryInterval = const Duration(seconds: 1),
  })  : _backend = backend,
        _telemetryInterval = telemetryInterval;

  final SessionBackend _backend;
  final Duration _telemetryInterval;

  SessionStatus _status = SessionStatus.idle;
  StreamSubscription<ipc.SessionEvent>? _eventSub;
  Timer? _telemetryTimer;
  bool _disposed = false;

  /// Observation of the backend's reachability, set only by calls this
  /// controller actually makes: null until something has been observed, false
  /// once a snapshot call fails. Nothing here probes the daemon speculatively.
  bool? _daemonReachable;

  int _ignoredStaleUpdates = 0;

  /// Latest telemetry the backend reported for the live session. Owned here so
  /// exactly one component polls (the controller), and screens read it.
  ipc.StreamStats? _streamStats;

  /// Last control failure (start/stop), for an inline message. Mirrors
  /// [TransferController.lastSendError].
  String? _lastError;

  /// The single session status the UI renders.
  SessionStatus get status => _status;

  bool get supportsSessions => _backend.supportsSessions;

  String get activeSessionId => _status.sessionId ?? '';

  /// Authoritative active peer: the daemon session's target device while the
  /// session is live, null otherwise. This is the ONLY active-device source
  /// the UI may use (Phase B) — never trust ordering.
  String? get activeDeviceId {
    if (!supportsSessions) return null;
    if (!_status.isActive) return null;
    final id = _status.deviceId;
    return (id == null || id.isEmpty) ? null : id;
  }

  bool? get daemonReachable => _daemonReachable;

  /// How many transitions were dropped as stale/late (diagnostics and tests).
  int get ignoredStaleUpdates => _ignoredStaleUpdates;

  bool get isPollingTelemetry => _telemetryTimer != null;

  ipc.StreamStats? get streamStats => _streamStats;

  String? get lastError => _lastError;

  void clearLastError() {
    if (_lastError == null) return;
    _lastError = null;
    _notify();
  }

  /// Subscribes to pushed transitions and hydrates the current session, if the
  /// backend is already tracking one.
  void initialize() {
    if (!_backend.supportsSessions) return;
    _eventSub?.cancel();
    _eventSub = _backend.sessionEventStream.listen(
      applyEvent,
      onError: (Object _) {
        // The event stream reconnects underneath the backend; a stream error is
        // not a session failure and must not be reported as one.
      },
    );
    final id = _backend.activeSessionId;
    if (id.isNotEmpty) {
      _status = _status.copyWith(sessionId: id);
    }
    unawaited(refresh());
  }

  /// Applies one pushed transition. Returns true when the status changed.
  bool applyEvent(ipc.SessionEvent event) {
    if (!_backend.supportsSessions) return false;

    final id = event.sessionId;
    if (_status.isActive &&
        id.isNotEmpty &&
        _status.sessionId != null &&
        id != _status.sessionId) {
      // A different session while one is live: late event from the replaced one.
      _ignoredStaleUpdates++;
      return false;
    }

    final next = SessionStatus.fromEvent(event, previous: _status);
    if (_isSameAs(next)) return false;
    _setStatus(next);
    return true;
  }

  /// Applies one authoritative snapshot. Returns true when it was accepted.
  bool applySnapshot(ipc.GetSessionStateResponse snapshot) {
    if (!_backend.supportsSessions) return false;

    final id = snapshot.sessionId;
    if (id.isNotEmpty && _status.sessionId != null && id != _status.sessionId) {
      if (_status.isActive) {
        _ignoredStaleUpdates++;
        return false;
      }
      // Not active: a genuinely new session is being reported, so adopt its id.
    }

    if (_status.isTerminal &&
        _isActiveState(snapshot.state) &&
        id == _status.sessionId) {
      _ignoredStaleUpdates++;
      return false;
    }

    final next = SessionStatus.fromSnapshot(snapshot, previous: _status);
    if (_isSameAs(next)) return false;
    _setStatus(next);
    return true;
  }

  /// Asks a peer for a session and makes the returned session the live one.
  ///
  /// [requested] is passed to the backend untouched: the media tuple a caller
  /// asks for must reach the daemon exactly as given (a dropped tuple is a
  /// silent capability loss — the phone would apply its own defaults instead).
  Future<bool> start(
      {required String target, pb.MediaParams? requested}) async {
    if (!_backend.supportsSessions) {
      _lastError = 'This platform has no local session state machine.';
      _notify();
      return false;
    }
    final deviceId = target.trim();
    if (deviceId.isEmpty) {
      _lastError = 'Select or enter a target device id.';
      _notify();
      return false;
    }

    _lastError = null;
    _notify();
    try {
      final resp = await _backend.startSession(
        target: deviceId,
        requested: requested,
      );
      if (_disposed) return false;
      _streamStats = null;
      // A new session starts from a clean slate: the previous failure and the
      // previous negotiated tuple must not leak into it. The requested target
      // is the authoritative peer from this moment until a snapshot says
      // otherwise (the daemon snapshot carries the same device_id).
      _setStatus(SessionStatus(
        state: resp.state,
        sessionId: resp.sessionId,
        deviceId: deviceId,
        reasonDetail: 'session initiated',
        requested: requested,
        actual: null,
      ));
      return resp.sessionId.isNotEmpty;
    } catch (error) {
      if (_disposed) return false;
      _lastError = error.toString();
      _notify();
      return false;
    }
  }

  /// Terminates the live session. Returns true when there was nothing to stop or
  /// the backend confirmed it, so a caller can treat "already stopped" as fine.
  Future<bool> stop(
      {String reason = 'user requested stop from desktop UI'}) async {
    final id = activeSessionId;
    if (id.isEmpty) return true;
    if (!_backend.supportsSessions) return false;

    _lastError = null;
    try {
      final resp = await _backend.stopSession(sessionId: id, reason: reason);
      if (_disposed) return false;
      _setStatus(_status.copyWith(
        state: resp.state,
        sessionId:
            resp.sessionId.isNotEmpty ? resp.sessionId : _status.sessionId,
        reasonCode: ipc.SessionReason.SESSION_REASON_USER_STOPPED,
        reasonDetail: 'stopped by user',
      ));
      return true;
    } catch (error) {
      if (_disposed) return false;
      _lastError = error.toString();
      _notify();
      return false;
    }
  }

  /// Reads the authoritative snapshot for the live session. Safe to call at any
  /// time; a failure is recorded as unreachable rather than thrown.
  Future<bool> refresh() async {
    if (!_backend.supportsSessions || _disposed) return false;
    final id = activeSessionId;
    try {
      final snapshot = await _backend.getSessionSnapshot(id);
      if (_disposed) return false;
      _setDaemonReachable(true);
      if (snapshot.hasStats()) {
        _streamStats = snapshot.stats;
      }
      return applySnapshot(snapshot);
    } catch (_) {
      if (_disposed) return false;
      _setDaemonReachable(false);
      return false;
    }
  }

  /// Clears the terminal status so the UI stops showing a stale failure. The
  /// daemon's own state is untouched.
  void clearStatus() {
    if (_status == SessionStatus.idle) return;
    _status = SessionStatus.idle;
    _streamStats = null;
    _stopTelemetryPolling();
    _notify();
  }

  void _setStatus(SessionStatus next) {
    _status = next;
    if (next.shouldKeepTelemetry) {
      _startTelemetryPolling();
    } else {
      _stopTelemetryPolling();
    }
    _notify();
  }

  void _setDaemonReachable(bool reachable) {
    if (_daemonReachable == reachable) return;
    _daemonReachable = reachable;
    _notify();
  }

  void _startTelemetryPolling() {
    if (_disposed || _telemetryTimer != null) return;
    _telemetryTimer = Timer.periodic(_telemetryInterval, (_) {
      // A reconnect keeps the session and only replaces the transport, so
      // telemetry deliberately keeps polling through it: dropping it here would
      // blank the panel during the exact moment the user wants to watch it
      // recover (DEC-022).
      unawaited(refresh());
    });
  }

  void _stopTelemetryPolling() {
    _telemetryTimer?.cancel();
    _telemetryTimer = null;
  }

  static bool _isActiveState(ipc.SessionState state) =>
      state != ipc.SessionState.SESSION_STATE_DISCONNECTED &&
      state != ipc.SessionState.SESSION_STATE_STOPPED &&
      state != ipc.SessionState.SESSION_STATE_FAILED;

  bool _isSameAs(SessionStatus other) =>
      other.state == _status.state &&
      other.sessionId == _status.sessionId &&
      other.deviceId == _status.deviceId &&
      other.reasonCode == _status.reasonCode &&
      other.reasonDetail == _status.reasonDetail &&
      other.errorMessage == _status.errorMessage &&
      other.reconnectAttempts == _status.reconnectAttempts &&
      other.durationMs == _status.durationMs &&
      other.requested == _status.requested &&
      other.sinkKind == _status.sinkKind &&
      other.sinkActive == _status.sinkActive &&
      other.actual == _status.actual &&
      mediaCapabilitiesEqual(other.capabilities, _status.capabilities);

  @override
  void dispose() {
    // Idempotent: a controller that owns a timer must tolerate being disposed
    // twice (owner teardown plus an explicit call) without throwing.
    if (_disposed) return;
    _disposed = true;
    _eventSub?.cancel();
    _eventSub = null;
    _stopTelemetryPolling();
    super.dispose();
  }

  /// [ChangeNotifier.notifyListeners] that ignores calls arriving after
  /// [dispose] (late refresh completions).
  void _notify() {
    if (_disposed) return;
    notifyListeners();
  }
}
