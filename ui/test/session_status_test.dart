import 'package:fixnum/fixnum.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/generated/phonebridge/localipc/v1/local_ipc.pb.dart';
import 'package:phonebridge_ui/generated/phonebridge/v1/phonebridge.pb.dart' as pb;
import 'package:phonebridge_ui/models/session_status.dart';

/// DEC-022 presentation rules.
///
/// The invariant these tests protect: the UI shows what was *asked for* and
/// what the phone *reported applying* as two different things, and a missing
/// report stays missing. Echoing the request back as the applied tuple would
/// make a downgrade invisible, which is exactly the silent substitution the
/// negotiation exists to prevent.
void main() {
  pb.MediaParams params({int w = 720, int h = 1600, int fps = 32, int kbps = 2500, String codec = 'h264'}) =>
      pb.MediaParams(width: w, height: h, fps: fps, bitrateKbps: kbps, codec: codec);

  group('SessionStatus.fromSnapshot', () {
    test('maps state, typed reason, media and reconnect attempts', () {
      final status = SessionStatus.fromSnapshot(
        GetSessionStateResponse(
          sessionId: 'sess-1',
          state: SessionState.SESSION_STATE_RECONNECTING,
          reasonCode: SessionReason.SESSION_REASON_TRANSPORT_FAILED,
          errorMessage: 'WebRTC disconnected',
          connectedDurationMs: Int64(4200),
          requested: params(w: 1080, h: 2400, fps: 60, kbps: 8000),
          actual: params(w: 720, h: 1600, fps: 32, kbps: 2500),
          reconnectAttempts: 2,
        ),
      );

      expect(status.state, SessionState.SESSION_STATE_RECONNECTING);
      expect(status.sessionId, 'sess-1');
      expect(status.reasonCode, SessionReason.SESSION_REASON_TRANSPORT_FAILED);
      expect(status.durationMs, 4200);
      expect(status.requested!.width, 1080);
      expect(status.actual!.width, 720);
      expect(status.reconnectAttempts, 2);
    });

    test('an unreported applied tuple stays unreported instead of echoing the request', () {
      final status = SessionStatus.fromSnapshot(
        GetSessionStateResponse(
          sessionId: 'sess-2',
          state: SessionState.SESSION_STATE_CONNECTED,
          requested: params(w: 1080, h: 2400),
        ),
      );

      expect(status.requested, isNotNull);
      expect(status.actual, isNull, reason: 'the phone did not report one, so it must stay unknown');
      expect(status.negotiationSummary(), contains('not reported'));
    });

    test('a stale applied tuple is cleared when the phone stops reporting one', () {
      final previous = SessionStatus(state: SessionState.SESSION_STATE_STREAMING, actual: params());
      final status = SessionStatus.fromSnapshot(
        GetSessionStateResponse(state: SessionState.SESSION_STATE_CONNECTED),
        previous: previous,
      );

      expect(status.actual, isNull);
      expect(status.sessionId, isNull);
    });

    test('a recovered session clears the previous error message', () {
      final status = SessionStatus.fromSnapshot(
        GetSessionStateResponse(
          sessionId: 'sess-3',
          state: SessionState.SESSION_STATE_STREAMING,
          errorMessage: '',
        ),
        previous: const SessionStatus(
          state: SessionState.SESSION_STATE_FAILED,
          errorMessage: 'old failure',
        ),
      );

      expect(status.errorMessage, isEmpty);
      expect(status.isTerminal, isFalse);
    });
  });

  group('SessionStatus.fromEvent', () {
    test('preserves the negotiated media, which events do not carry', () {
      final previous = SessionStatus(
        state: SessionState.SESSION_STATE_STREAMING,
        sessionId: 'sess-9',
        requested: params(w: 1080, h: 2400, fps: 60),
        actual: params(w: 720, h: 1600, fps: 32),
      );

      final status = SessionStatus.fromEvent(
        SessionEvent(
          sessionId: 'sess-9',
          state: SessionState.SESSION_STATE_RECONNECTING,
          reason: 'WebRTC disconnected',
          reasonCode: SessionReason.SESSION_REASON_TRANSPORT_FAILED,
        ),
        previous: previous,
      );

      expect(status.requested!.width, 1080);
      expect(status.actual!.width, 720);
      expect(status.state, SessionState.SESSION_STATE_RECONNECTING);
    });

    test('a terminal event keeps its typed reason', () {
      final status = SessionStatus.fromEvent(
        SessionEvent(
          sessionId: 'sess-10',
          state: SessionState.SESSION_STATE_FAILED,
          reason: 'device reported CONSENT_REVOKED',
          reasonCode: SessionReason.SESSION_REASON_CONSENT_REVOKED,
          errorMessage: 'device reported CONSENT_REVOKED',
        ),
      );

      expect(status.isTerminal, isTrue);
      expect(status.failureSummary, contains('consent withdrawn'));
    });
  });

  group('state classes', () {
    test('recovery is active, not terminal, and keeps telemetry alive', () {
      const status = SessionStatus(state: SessionState.SESSION_STATE_RECONNECTING);

      expect(status.isRecovering, isTrue);
      expect(status.isActive, isTrue);
      expect(status.isTerminal, isFalse);
      // The session is the same one; only its transport is being replaced.
      expect(status.shouldKeepTelemetry, isTrue);
      expect(status.canStop, isTrue);
    });

    test('terminal and idle states stop telemetry', () {
      expect(const SessionStatus(state: SessionState.SESSION_STATE_STOPPED).shouldKeepTelemetry, isFalse);
      expect(const SessionStatus(state: SessionState.SESSION_STATE_FAILED).shouldKeepTelemetry, isFalse);
      expect(SessionStatus.idle.shouldKeepTelemetry, isFalse);
      expect(SessionStatus.idle.isIdle, isTrue);
    });

    test('recovering detail reports the attempt count only while recovering', () {
      const recovering = SessionStatus(
        state: SessionState.SESSION_STATE_RECONNECTING,
        reconnectAttempts: 3,
      );
      expect(recovering.recoveringDetail, contains('attempt 3'));

      const attemptless = SessionStatus(state: SessionState.SESSION_STATE_RECONNECTING);
      expect(attemptless.recoveringDetail, contains('re-negotiating'));

      expect(const SessionStatus(state: SessionState.SESSION_STATE_STREAMING).recoveringDetail, isEmpty);
    });

    test('labels distinguish connecting from recovering', () {
      expect(
        const SessionStatus(state: SessionState.SESSION_STATE_CONNECTING).label,
        contains('NEGOTIATING'),
      );
      expect(
        const SessionStatus(state: SessionState.SESSION_STATE_RECONNECTING).label,
        contains('AUTOMATIC RECOVERY'),
      );
    });
  });

  group('negotiation summary', () {
    test('reports a downgrade explicitly', () {
      final status = SessionStatus(
        state: SessionState.SESSION_STATE_STREAMING,
        requested: params(w: 1080, h: 2400, fps: 60, kbps: 8000),
        actual: params(w: 720, h: 1600, fps: 32, kbps: 2500),
      );

      expect(status.isDowngraded, isTrue);
      final summary = status.negotiationSummary();
      expect(summary, contains('Requested: 1080x2400 · @60 fps'));
      expect(summary, contains('Applied: 720x1600 · @32 fps'));
      expect(summary, contains('different parameters'));
    });

    test('an identical tuple is not reported as a downgrade', () {
      final status = SessionStatus(
        state: SessionState.SESSION_STATE_STREAMING,
        requested: params(),
        actual: params(),
      );
      expect(status.isDowngraded, isFalse);
      expect(status.negotiationSummary(), isNot(contains('different')));
    });

    test('a missing applied tuple is never treated as a downgrade', () {
      final status = SessionStatus(
        state: SessionState.SESSION_STATE_CONNECTED,
        requested: params(w: 1080, h: 2400),
      );
      expect(status.isDowngraded, isFalse);
    });

    test('no negotiation at all produces no summary', () {
      expect(SessionStatus.idle.negotiationSummary(), isEmpty);
      expect(SessionStatus.idle.hasNegotiatedMedia, isFalse);
    });
  });

  group('typed failures', () {
    test('each failure class gets its own actionable message', () {
      const revoked = SessionStatus(
        state: SessionState.SESSION_STATE_FAILED,
        reasonCode: SessionReason.SESSION_REASON_CONSENT_REVOKED,
      );
      const capture = SessionStatus(
        state: SessionState.SESSION_STATE_FAILED,
        reasonCode: SessionReason.SESSION_REASON_CAPTURE_FAILED,
      );

      expect(revoked.failureSummary, contains('consent withdrawn'));
      expect(capture.failureSummary, contains('capture pipeline failed'));
      // A consent revocation is not described as a capture or transport fault.
      expect(revoked.failureSummary, isNot(contains('transport')));
      expect(revoked.failureSummary, isNot(equals(capture.failureSummary)));
    });

    test('transport failures and reconnect timeouts are distinct from capture failures', () {
      const transport = SessionStatus(
        state: SessionState.SESSION_STATE_FAILED,
        reasonCode: SessionReason.SESSION_REASON_TRANSPORT_FAILED,
      );
      const timeout = SessionStatus(
        state: SessionState.SESSION_STATE_FAILED,
        reasonCode: SessionReason.SESSION_REASON_RECONNECT_TIMEOUT,
      );

      expect(transport.failureSummary, contains('media transport failed'));
      expect(timeout.failureSummary, contains('reconnect window'));
    });

    test('a normal stop has no failure message and no badge', () {
      const stopped = SessionStatus(
        state: SessionState.SESSION_STATE_STOPPED,
        reasonCode: SessionReason.SESSION_REASON_USER_STOPPED,
      );

      expect(stopped.failureSummary, isEmpty);
      expect(stopped.reasonBadge, isEmpty);
    });

    test('the badge names the typed reason without the enum prefix', () {
      const status = SessionStatus(
        state: SessionState.SESSION_STATE_FAILED,
        reasonCode: SessionReason.SESSION_REASON_UNSUPPORTED_MEDIA_PARAMS,
      );

      expect(status.reasonBadge, 'UNSUPPORTED_MEDIA_PARAMS');
    });
  });

  group('copyWith', () {
    test('clears the error and the applied tuple when asked', () {
      final status = SessionStatus(
        state: SessionState.SESSION_STATE_FAILED,
        errorMessage: 'boom',
        actual: params(),
      );

      final cleared = status.copyWith(clearError: true, clearActual: true);
      expect(cleared.errorMessage, isEmpty);
      expect(cleared.actual, isNull);
    });

    test('keeps what it is not asked to change', () {
      final status = SessionStatus(
        state: SessionState.SESSION_STATE_CONNECTED,
        sessionId: 'sess-keep',
        requested: params(w: 1080, h: 2400),
        actual: params(w: 720, h: 1600),
      );

      final next = status.copyWith(state: SessionState.SESSION_STATE_STREAMING);
      expect(next.sessionId, 'sess-keep');
      expect(next.requested!.width, 1080);
      expect(next.actual!.width, 720);
      expect(next.state, SessionState.SESSION_STATE_STREAMING);
    });
  });

  test('media descriptions degrade gracefully for a partial tuple', () {
    final onlyGeometry = SessionStatus(
      state: SessionState.SESSION_STATE_CONNECTED,
      actual: pb.MediaParams(width: 720, height: 1600),
    );
    expect(onlyGeometry.actualSummary, '720x1600');

    final empty = SessionStatus(
      state: SessionState.SESSION_STATE_CONNECTED,
      actual: pb.MediaParams(),
    );
    expect(empty.actualSummary, 'device default');
  });
}
