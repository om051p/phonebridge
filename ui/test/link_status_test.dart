import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/generated/phonebridge/localipc/v1/local_ipc.pb.dart' as ipc;
import 'package:phonebridge_ui/models/link_status.dart';
import 'package:phonebridge_ui/models/session_status.dart';
import 'package:phonebridge_ui/models/transfer_item.dart';

void main() {
  SessionStatus status(
    ipc.SessionState state, {
    ipc.SessionReason reason = ipc.SessionReason.SESSION_REASON_NONE,
    String errorMessage = '',
    String reasonDetail = '',
    int reconnectAttempts = 0,
    String sessionId = 'sess-1',
  }) {
    return SessionStatus(
      state: state,
      sessionId: sessionId,
      reasonCode: reason,
      errorMessage: errorMessage,
      reasonDetail: reasonDetail,
      reconnectAttempts: reconnectAttempts,
    );
  }

  TransferItem transfer(
    ipc.TransferState state, {
    String id = 'tx-1',
    String filename = 'holiday.mp4',
  }) {
    return TransferItem(transferId: id, state: state, filename: filename);
  }

  group('LinkStatus.compose — phase precedence', () {
    test('session exists states map to their own phase', () {
      expect(
        LinkStatus.compose(
          session: status(ipc.SessionState.SESSION_STATE_CONNECTING),
        ).phase,
        LinkPhase.connecting,
      );
      expect(
        LinkStatus.compose(
          session: status(ipc.SessionState.SESSION_STATE_DISCOVERING),
        ).phase,
        LinkPhase.connecting,
      );
      expect(
        LinkStatus.compose(
          session: status(ipc.SessionState.SESSION_STATE_CONNECTED),
        ).phase,
        LinkPhase.connected,
      );
      expect(
        LinkStatus.compose(
          session: status(ipc.SessionState.SESSION_STATE_STREAMING),
        ).phase,
        LinkPhase.streaming,
      );
      expect(
        LinkStatus.compose(
          session: status(ipc.SessionState.SESSION_STATE_RECONNECTING),
        ).phase,
        LinkPhase.reconnecting,
      );
      expect(
        LinkStatus.compose(
          session: status(ipc.SessionState.SESSION_STATE_FAILED),
        ).phase,
        LinkPhase.failed,
      );
    });

    test('a failure outranks every other session state', () {
      final link = LinkStatus.compose(
        session: status(
          ipc.SessionState.SESSION_STATE_FAILED,
          reason: ipc.SessionReason.SESSION_REASON_RECONNECT_TIMEOUT,
          errorMessage: 'reconnect budget 15s exhausted after 10 attempt(s)',
        ),
        transfer: TransferActivity(
          activeCount: 1,
          latest: transfer(ipc.TransferState.TRANSFER_STATE_ACTIVE),
        ),
      );
      expect(link.phase, LinkPhase.failed);
      expect(link.severity, LinkSeverity.critical);
      expect(link.description, contains('reconnect budget'));
      expect(link.reasonLabel, 'RECONNECT_TIMEOUT');
      expect(
        link.actionHint,
        contains('did not recover within the reconnect window'),
      );
    });

    test('recovery is not presented as an error', () {
      final link = LinkStatus.compose(
        session: status(
          ipc.SessionState.SESSION_STATE_RECONNECTING,
          reconnectAttempts: 3,
        ),
      );
      expect(link.phase, LinkPhase.reconnecting);
      expect(link.severity, LinkSeverity.warning);
      expect(link.actionHint, isEmpty);
      expect(link.description, contains('attempt 3'));
    });

    test('no session is "ready" unless the backend is known to be unreachable',
        () {
      expect(
        LinkStatus.compose(session: SessionStatus.idle).phase,
        LinkPhase.disconnected,
      );
      expect(
        LinkStatus.compose(session: SessionStatus.idle, daemonReachable: true)
            .phase,
        LinkPhase.disconnected,
      );
      expect(
        LinkStatus.compose(session: SessionStatus.idle, daemonReachable: false)
            .phase,
        LinkPhase.offline,
      );
      expect(
        LinkStatus.compose(
          session: SessionStatus.idle,
          transfer: const TransferActivity(backendUnavailable: true),
        ).phase,
        LinkPhase.offline,
      );
    });

    test('a stopped session is not a failure', () {
      final link = LinkStatus.compose(
        session: status(
          ipc.SessionState.SESSION_STATE_STOPPED,
          reason: ipc.SessionReason.SESSION_REASON_USER_STOPPED,
        ),
      );
      expect(link.phase, LinkPhase.disconnected);
      expect(link.severity, LinkSeverity.neutral);
      expect(link.reasonLabel, isEmpty);
      expect(link.actionHint, isEmpty);
    });
  });

  group('LinkStatus.compose — a transfer never decides the connection', () {
    test('a live session stays live while an unreachable transfer backend is '
        'reported', () {
      final link = LinkStatus.compose(
        session: status(ipc.SessionState.SESSION_STATE_STREAMING),
        transfer: const TransferActivity(backendUnavailable: true),
        daemonReachable: false,
      );
      expect(link.phase, LinkPhase.streaming);
      expect(link.severity, LinkSeverity.good);
    });

    test('an active transfer on a connected session reads as connected', () {
      final link = LinkStatus.compose(
        session: status(ipc.SessionState.SESSION_STATE_CONNECTED),
        transfer: TransferActivity(
          activeCount: 2,
          latest: transfer(ipc.TransferState.TRANSFER_STATE_ACTIVE),
        ),
      );
      expect(link.phase, LinkPhase.connected);
      expect(link.transfer.isActive, isTrue);
      expect(link.transferLine, contains('2 transfers in progress'));
    });

    test('an interrupted transfer under a failed session is not the failure '
        'reason', () {
      final link = LinkStatus.compose(
        session: status(
          ipc.SessionState.SESSION_STATE_FAILED,
          reason: ipc.SessionReason.SESSION_REASON_TRANSPORT_FAILED,
        ),
        transfer: TransferActivity(
          latest: transfer(
            ipc.TransferState.TRANSFER_STATE_FAILED,
            filename: 'big.iso',
          ),
        ),
      );
      expect(link.phase, LinkPhase.failed);
      expect(link.reasonLabel, 'TRANSPORT_FAILED');
      expect(link.transferLine, contains('Last transfer failed'));
      expect(link.transferLine, contains('big.iso'));
    });
  });

  group('TransferActivity', () {
    test('cancelled is not a failure; failed is', () {
      expect(
        TransferActivity(
          latest: transfer(ipc.TransferState.TRANSFER_STATE_CANCELLED),
        ).hasFailure,
        isFalse,
      );
      expect(
        TransferActivity(
          latest: transfer(ipc.TransferState.TRANSFER_STATE_FAILED),
        ).hasFailure,
        isTrue,
      );
      expect(
        TransferActivity(
          latest: transfer(ipc.TransferState.TRANSFER_STATE_COMPLETE),
        ).hasFailure,
        isFalse,
      );
      expect(TransferActivity.none.hasFailure, isFalse);
    });

    test('transferLine is empty when there is nothing to say', () {
      expect(TransferActivity.none.isActive, isFalse);
      expect(
        LinkStatus.compose(session: SessionStatus.idle).transferLine,
        isEmpty,
      );
      expect(
        LinkStatus.compose(
          session: SessionStatus.idle,
          transfer: const TransferActivity(
            backendUnavailable: true,
          ),
        ).transferLine,
        contains('unavailable'),
      );
      expect(
        LinkStatus.compose(
          session: SessionStatus.idle,
          transfer: TransferActivity(
            latest: transfer(ipc.TransferState.TRANSFER_STATE_CANCELLED),
          ),
        ).transferLine,
        contains('cancelled'),
      );
    });
  });

  group('LinkStatus presentation', () {
    test('labels are the short badge words', () {
      String label(ipc.SessionState state) =>
          LinkStatus.compose(session: status(state)).label;
      expect(label(ipc.SessionState.SESSION_STATE_STREAMING), 'Sharing');
      expect(label(ipc.SessionState.SESSION_STATE_CONNECTED), 'Connected');
      expect(label(ipc.SessionState.SESSION_STATE_RECONNECTING), 'Reconnecting');
      expect(label(ipc.SessionState.SESSION_STATE_CONNECTING), 'Connecting');
      expect(label(ipc.SessionState.SESSION_STATE_FAILED), 'Session failed');
      expect(label(ipc.SessionState.SESSION_STATE_DISCONNECTED), 'Ready');
    });

    test('hasSession is true only while a session exists', () {
      expect(
        LinkStatus.compose(
          session: status(ipc.SessionState.SESSION_STATE_STREAMING),
        ).hasSession,
        isTrue,
      );
      expect(
        LinkStatus.compose(session: status(ipc.SessionState.SESSION_STATE_FAILED))
            .hasSession,
        isFalse,
      );
      expect(LinkStatus.compose(session: SessionStatus.idle).hasSession, isFalse);
    });

    test('severity bands drive colour choices', () {
      expect(
        LinkStatus.severityFor(LinkPhase.streaming),
        LinkSeverity.good,
      );
      expect(
        LinkStatus.severityFor(LinkPhase.reconnecting),
        LinkSeverity.warning,
      );
      expect(
        LinkStatus.severityFor(LinkPhase.failed),
        LinkSeverity.critical,
      );
      expect(
        LinkStatus.severityFor(LinkPhase.disconnected),
        LinkSeverity.neutral,
      );
    });
  });
}
