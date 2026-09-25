import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/controllers/session_controller.dart';
import 'package:phonebridge_ui/generated/phonebridge/localipc/v1/local_ipc.pb.dart' as ipc;
import 'package:phonebridge_ui/services/session_backend.dart';

import 'support/fake_session_backend.dart';

void main() {
  late FakeSessionBackend backend;
  late SessionController controller;

  setUp(() {
    backend = FakeSessionBackend();
    // A short poll interval keeps the telemetry tests fast; the controller takes
    // it as a parameter precisely so they do not have to wait a real second.
    controller = SessionController(
      backend: backend,
      telemetryInterval: const Duration(milliseconds: 5),
    );
  });

  tearDown(() async {
    controller.dispose();
    await backend.close();
  });

  group('unsupported platform', () {
    test('never claims a session and never polls', () async {
      final unsupported = SessionController(
        backend: const UnsupportedSessionBackend(),
      );
      unsupported.initialize();

      expect(unsupported.supportsSessions, isFalse);
      expect(unsupported.activeSessionId, isEmpty);
      expect(
        unsupported.applyEvent(
          sessionEvent(state: ipc.SessionState.SESSION_STATE_STREAMING),
        ),
        isFalse,
      );
      expect(
        unsupported.applySnapshot(
          sessionSnapshot(state: ipc.SessionState.SESSION_STATE_STREAMING),
        ),
        isFalse,
      );
      expect(await unsupported.refresh(), isFalse);
      expect(
        unsupported.status.state,
        ipc.SessionState.SESSION_STATE_DISCONNECTED,
      );
      expect(unsupported.isPollingTelemetry, isFalse);
      unsupported.dispose();
    });
  });

  group('pushed transitions', () {
    test('are applied in the order they arrive', () async {
      controller.initialize();

      await backend.push(
        sessionEvent(sessionId: 's', state: ipc.SessionState.SESSION_STATE_CONNECTING),
      );
      expect(
        controller.status.state,
        ipc.SessionState.SESSION_STATE_CONNECTING,
      );

      await backend.push(
        sessionEvent(sessionId: 's', state: ipc.SessionState.SESSION_STATE_CONNECTED),
      );
      await backend.push(
        sessionEvent(
          sessionId: 's',
          state: ipc.SessionState.SESSION_STATE_RECONNECTING,
          reason: 'transport lost',
        ),
      );

      expect(
        controller.status.state,
        ipc.SessionState.SESSION_STATE_RECONNECTING,
      );
      expect(controller.status.isRecovering, isTrue);
      expect(controller.status.sessionId, 's');
    });

    test('the session id is kept across a reconnect', () async {
      controller.initialize();
      await backend.push(
        sessionEvent(sessionId: 'sess-live', state: ipc.SessionState.SESSION_STATE_CONNECTED),
      );
      await backend.push(
        sessionEvent(
          sessionId: 'sess-live',
          state: ipc.SessionState.SESSION_STATE_RECONNECTING,
        ),
      );
      // The daemon preserves the id across a transport replacement, so the UI
      // must not treat the reconnect as a different session.
      expect(controller.status.sessionId, 'sess-live');
      expect(controller.ignoredStaleUpdates, 0);
    });

    test('an identical repeated transition does not notify again', () {
      var notifications = 0;
      controller.addListener(() => notifications++);

      expect(
        controller.applyEvent(
          sessionEvent(sessionId: 's', state: ipc.SessionState.SESSION_STATE_CONNECTED),
        ),
        isTrue,
      );
      final after = notifications;
      expect(
        controller.applyEvent(
          sessionEvent(sessionId: 's', state: ipc.SessionState.SESSION_STATE_CONNECTED),
        ),
        isFalse,
      );
      expect(notifications, after);
    });
  });

  group('stale-event protection', () {
    test('a late event from a replaced session cannot repaint the live one', () {
      controller.applyEvent(
        sessionEvent(sessionId: 'sess-A', state: ipc.SessionState.SESSION_STATE_CONNECTED),
      );

      final applied = controller.applyEvent(
        sessionEvent(
          sessionId: 'sess-B',
          state: ipc.SessionState.SESSION_STATE_FAILED,
          reason: 'reconnect budget exhausted',
          reasonCode: ipc.SessionReason.SESSION_REASON_RECONNECT_TIMEOUT,
        ),
      );

      expect(applied, isFalse);
      expect(
        controller.status.state,
        ipc.SessionState.SESSION_STATE_CONNECTED,
      );
      expect(controller.status.sessionId, 'sess-A');
      expect(controller.ignoredStaleUpdates, 1);
    });

    test('a new session id is adopted when nothing is live', () {
      controller.applyEvent(
        sessionEvent(
          sessionId: 'sess-A',
          state: ipc.SessionState.SESSION_STATE_FAILED,
          reasonCode: ipc.SessionReason.SESSION_REASON_TRANSPORT_FAILED,
        ),
      );
      expect(controller.status.isTerminal, isTrue);

      final applied = controller.applyEvent(
        sessionEvent(sessionId: 'sess-B', state: ipc.SessionState.SESSION_STATE_CONNECTING),
      );

      expect(applied, isTrue);
      expect(controller.status.sessionId, 'sess-B');
      expect(
        controller.status.state,
        ipc.SessionState.SESSION_STATE_CONNECTING,
      );
    });
  });

  group('snapshots', () {
    test('initialize hydrates the session the backend already tracks', () async {
      backend.activeSessionId = 'sess-live';
      backend.snapshot = sessionSnapshot(
        sessionId: 'sess-live',
        state: ipc.SessionState.SESSION_STATE_STREAMING,
        durationMs: 1234,
      );

      controller.initialize();
      await Future<void>.delayed(Duration.zero);

      expect(controller.activeSessionId, 'sess-live');
      expect(
        controller.status.state,
        ipc.SessionState.SESSION_STATE_STREAMING,
      );
      expect(controller.status.durationMs, 1234);
      expect(backend.snapshotIds, ['sess-live']);
      expect(controller.daemonReachable, isTrue);
    });

    test('a stale snapshot cannot resurrect a terminal session', () {
      controller.applyEvent(
        sessionEvent(
          sessionId: 'sess-A',
          state: ipc.SessionState.SESSION_STATE_FAILED,
          reasonCode: ipc.SessionReason.SESSION_REASON_RECONNECT_TIMEOUT,
        ),
      );

      final accepted = controller.applySnapshot(
        sessionSnapshot(sessionId: 'sess-A', state: ipc.SessionState.SESSION_STATE_STREAMING),
      );

      expect(accepted, isFalse);
      expect(
        controller.status.state,
        ipc.SessionState.SESSION_STATE_FAILED,
      );
      expect(controller.ignoredStaleUpdates, 1);
    });

    test('a snapshot for another session is ignored while one is live', () {
      controller.applyEvent(
        sessionEvent(sessionId: 'sess-A', state: ipc.SessionState.SESSION_STATE_STREAMING),
      );

      final accepted = controller.applySnapshot(
        sessionSnapshot(sessionId: 'sess-B', state: ipc.SessionState.SESSION_STATE_CONNECTED),
      );

      expect(accepted, isFalse);
      expect(controller.status.sessionId, 'sess-A');
      expect(controller.ignoredStaleUpdates, 1);
    });

    test('a snapshot for a new session is adopted when nothing is live', () {
      controller.applyEvent(
        sessionEvent(
          sessionId: 'sess-A',
          state: ipc.SessionState.SESSION_STATE_STOPPED,
          reasonCode: ipc.SessionReason.SESSION_REASON_USER_STOPPED,
        ),
      );

      final accepted = controller.applySnapshot(
        sessionSnapshot(sessionId: 'sess-B', state: ipc.SessionState.SESSION_STATE_CONNECTING),
      );

      expect(accepted, isTrue);
      expect(controller.status.sessionId, 'sess-B');
    });

    test('an unreachable daemon is recorded instead of thrown', () async {
      backend.activeSessionId = 'sess-x';
      backend.snapshotError = StateError('connection refused');

      controller.initialize();
      await Future<void>.delayed(Duration.zero);

      expect(controller.daemonReachable, isFalse);
      expect(
        controller.status.state,
        ipc.SessionState.SESSION_STATE_DISCONNECTED,
      );
    });
  });

  group('telemetry polling', () {
    test('polls while a session is live and stops when it ends', () async {
      backend.snapshot = sessionSnapshot(
        sessionId: 'sess-1',
        state: ipc.SessionState.SESSION_STATE_STREAMING,
      );
      controller.initialize();

      controller.applyEvent(
        sessionEvent(sessionId: 'sess-1', state: ipc.SessionState.SESSION_STATE_CONNECTED),
      );
      expect(controller.isPollingTelemetry, isTrue);

      await Future<void>.delayed(const Duration(milliseconds: 40));
      expect(backend.snapshotCalls, greaterThanOrEqualTo(2));
      expect(
        controller.status.state,
        ipc.SessionState.SESSION_STATE_STREAMING,
      );

      controller.applyEvent(
        sessionEvent(
          sessionId: 'sess-1',
          state: ipc.SessionState.SESSION_STATE_FAILED,
          reasonCode: ipc.SessionReason.SESSION_REASON_TRANSPORT_FAILED,
        ),
      );
      expect(controller.isPollingTelemetry, isFalse);

      final callsAtStop = backend.snapshotCalls;
      await Future<void>.delayed(const Duration(milliseconds: 30));
      expect(backend.snapshotCalls, callsAtStop);
    });

    test('keeps polling through a reconnect', () async {
      backend.snapshot = sessionSnapshot(
        sessionId: 'sess-1',
        state: ipc.SessionState.SESSION_STATE_RECONNECTING,
        reconnectAttempts: 2,
      );
      controller.initialize();

      controller.applyEvent(
        sessionEvent(sessionId: 'sess-1', state: ipc.SessionState.SESSION_STATE_CONNECTED),
      );
      controller.applyEvent(
        sessionEvent(
          sessionId: 'sess-1',
          state: ipc.SessionState.SESSION_STATE_RECONNECTING,
        ),
      );

      expect(controller.isPollingTelemetry, isTrue);
      await Future<void>.delayed(const Duration(milliseconds: 30));
      expect(backend.snapshotCalls, greaterThanOrEqualTo(2));
      expect(controller.status.reconnectAttempts, 2);
    });

    test('dispose stops polling and late work never notifies', () async {
      backend.snapshot = sessionSnapshot(
        sessionId: 'sess-1',
        state: ipc.SessionState.SESSION_STATE_STREAMING,
      );
      controller.initialize();
      controller.applyEvent(
        sessionEvent(sessionId: 'sess-1', state: ipc.SessionState.SESSION_STATE_CONNECTED),
      );
      await Future<void>.delayed(const Duration(milliseconds: 20));

      var notifications = 0;
      controller.addListener(() => notifications++);
      final callsBeforeDispose = backend.snapshotCalls;

      controller.dispose();
      expect(controller.isPollingTelemetry, isFalse);

      final notifiedAfterDispose = notifications;
      // A refresh that completes after the widget tree is gone must not touch a
      // disposed notifier, and the poll timer must be gone.
      expect(await controller.refresh(), isFalse);
      await Future<void>.delayed(const Duration(milliseconds: 30));

      expect(backend.snapshotCalls, callsBeforeDispose);
      expect(notifications, notifiedAfterDispose);
    });
  });

  group('clearStatus', () {
    test('drops a terminal status and stops polling', () {
      controller.applyEvent(
        sessionEvent(
          sessionId: 'sess-1',
          state: ipc.SessionState.SESSION_STATE_FAILED,
          reasonCode: ipc.SessionReason.SESSION_REASON_RECONNECT_TIMEOUT,
        ),
      );
      expect(controller.status.isTerminal, isTrue);

      controller.clearStatus();

      expect(
        controller.status.state,
        ipc.SessionState.SESSION_STATE_DISCONNECTED,
      );
      expect(controller.status.sessionId, isNull);
      expect(controller.isPollingTelemetry, isFalse);
    });
  });
}
