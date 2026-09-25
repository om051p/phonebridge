// Unified connection/transfer status (Phase 5, first slice).
//
// Before this model there was no single answer to "are we connected?": the
// session state machine lived inside one screen, the rest of the app inferred
// connection from `CaptureStats.isCapturing`, and a failed session was
// indistinguishable from a user-stopped one. This file is the one place that
// decides what the app shows, so every surface agrees.
//
// Two rules the composition must never break, because breaking either one
// misleads the user about the thing they are looking at (DEC-022/DEC-024):
//
//  1. A *transfer* outcome never decides the *connection* phase. A transfer is
//     interrupted when its session is gone, but an interrupted transfer must
//     not be reported as "the connection failed", and a healthy connection
//     with a failed transfer must still read as connected.
//  2. A recoverable state is not an error. Bounded reconnect (DEC-022) is
//     presented as recovery, and only an exhausted reconnect window is a
//     failure — with the typed reason, never prose sniffing.

import '../generated/phonebridge/localipc/v1/local_ipc.pb.dart' as ipc;

import 'session_status.dart';
import 'transfer_item.dart';

/// User-facing connection phase. Deliberately coarser than [SessionState]: it is
/// what a person needs to tell apart, not every internal transition.
enum LinkPhase {
  /// The local daemon/backend cannot be reached at all.
  offline,

  /// Reachable, no session running (the state the UI calls "ready").
  disconnected,

  /// Negotiating with the phone (discovery/offer/answer).
  connecting,

  /// Session established, no media flowing.
  connected,

  /// Session established and streaming video.
  streaming,

  /// Transport lost, bounded reconnect in progress (not an error).
  reconnecting,

  /// Terminal failure carrying a typed reason code.
  failed,
}

/// How loudly the phase should be presented.
enum LinkSeverity { neutral, good, warning, critical }

/// The transfer facts the connection model renders. Built from
/// [TransferController] state; it carries no policy of its own.
///
/// [backendUnavailable] is the "we could not even read the transfer list" case,
/// which the UI must keep distinct from "no transfers yet".
class TransferActivity {
  const TransferActivity({
    this.activeCount = 0,
    this.latest,
    this.backendUnavailable = false,
  });

  final int activeCount;

  /// Most recent transfer, whatever its state.
  final TransferItem? latest;

  final bool backendUnavailable;

  static const TransferActivity none = TransferActivity();

  bool get isActive => activeCount > 0;

  /// True when the most recent transfer ended badly (cancelled is not a
  /// failure: the user asked for it).
  bool get hasFailure {
    final item = latest;
    if (item == null) return false;
    return item.isTerminal && (item.isFailed || !item.isComplete && !item.isCancelled);
  }
}

class LinkStatus {
  const LinkStatus({
    required this.phase,
    required this.label,
    required this.description,
    required this.severity,
    this.actionHint = '',
    this.reasonCode = ipc.SessionReason.SESSION_REASON_UNSPECIFIED,
    this.reconnectAttempts = 0,
    this.transfer = TransferActivity.none,
  });

  final LinkPhase phase;

  /// Short badge text ("Ready", "Sharing", "Reconnecting", …).
  final String label;

  /// One-line explanation of the phase.
  final String description;

  final LinkSeverity severity;

  /// What the user can do about it; empty when nothing needs doing.
  final String actionHint;

  /// Typed session reason, passed through untouched.
  final ipc.SessionReason reasonCode;

  final int reconnectAttempts;

  final TransferActivity transfer;

  /// Composes the one status the whole app renders.
  ///
  /// Precedence, highest first: failed session → recovering session → streaming
  /// → connected → connecting → (offline) → ready. Transfer activity is
  /// *parallel* information and never changes the phase.
  factory LinkStatus.compose({
    required SessionStatus session,
    TransferActivity transfer = TransferActivity.none,
    bool? daemonReachable,
  }) {
    final LinkPhase phase;
    switch (session.state) {
      case ipc.SessionState.SESSION_STATE_FAILED:
        phase = LinkPhase.failed;
        break;
      case ipc.SessionState.SESSION_STATE_RECONNECTING:
        phase = LinkPhase.reconnecting;
        break;
      case ipc.SessionState.SESSION_STATE_STREAMING:
        phase = LinkPhase.streaming;
        break;
      case ipc.SessionState.SESSION_STATE_CONNECTED:
        phase = LinkPhase.connected;
        break;
      case ipc.SessionState.SESSION_STATE_CONNECTING:
      case ipc.SessionState.SESSION_STATE_DISCOVERING:
        phase = LinkPhase.connecting;
        break;
      case ipc.SessionState.SESSION_STATE_STOPPED:
      case ipc.SessionState.SESSION_STATE_DISCONNECTED:
      default:
        // No session: only now can "we cannot reach the backend" describe the
        // phase. Reporting offline while a session is live would hide the
        // session the user is watching.
        phase = (daemonReachable == false || transfer.backendUnavailable)
            ? LinkPhase.offline
            : LinkPhase.disconnected;
        break;
    }

    return LinkStatus(
      phase: phase,
      label: _labelFor(phase),
      description: _describe(phase, session, transfer),
      severity: severityFor(phase),
      actionHint: phase == LinkPhase.failed ? session.failureSummary : '',
      reasonCode: session.reasonCode,
      reconnectAttempts: session.reconnectAttempts,
      transfer: transfer,
    );
  }

  static String _labelFor(LinkPhase phase) {
    switch (phase) {
      case LinkPhase.offline:
        return 'Offline';
      case LinkPhase.disconnected:
        return 'Ready';
      case LinkPhase.connecting:
        return 'Connecting';
      case LinkPhase.connected:
        return 'Connected';
      case LinkPhase.streaming:
        return 'Sharing';
      case LinkPhase.reconnecting:
        return 'Reconnecting';
      case LinkPhase.failed:
        return 'Session failed';
    }
  }

  /// How loudly a phase should be presented. Public so the UI maps severity to
  /// colour in one place instead of re-deriving it per surface.
  static LinkSeverity severityFor(LinkPhase phase) {
    switch (phase) {
      case LinkPhase.streaming:
      case LinkPhase.connected:
        return LinkSeverity.good;
      case LinkPhase.reconnecting:
        return LinkSeverity.warning;
      case LinkPhase.failed:
        return LinkSeverity.critical;
      case LinkPhase.offline:
      case LinkPhase.connecting:
      case LinkPhase.disconnected:
        return LinkSeverity.neutral;
    }
  }

  static String _describe(
    LinkPhase phase,
    SessionStatus session,
    TransferActivity transfer,
  ) {
    switch (phase) {
      case LinkPhase.offline:
        return 'The local PhoneBridge service is not reachable.';
      case LinkPhase.disconnected:
        return 'No session running.';
      case LinkPhase.connecting:
        return session.label;
      case LinkPhase.connected:
        return 'Session established; video is not flowing yet.';
      case LinkPhase.streaming:
        return 'Streaming the phone screen.';
      case LinkPhase.reconnecting:
        return session.recoveringDetail.isEmpty
            ? 'Transport lost; re-negotiating with the phone.'
            : session.recoveringDetail;
      case LinkPhase.failed:
        return session.errorMessage.isNotEmpty
            ? session.errorMessage
            : 'The session ended with a failure.';
    }
  }

  /// Whether a session exists in any form (used to gate session controls).
  bool get hasSession => phase == LinkPhase.connecting ||
      phase == LinkPhase.connected ||
      phase == LinkPhase.streaming ||
      phase == LinkPhase.reconnecting;

  /// The transfer line shown under the connection status. Empty when there is
  /// nothing transfer-related worth saying.
  String get transferLine {
    if (transfer.backendUnavailable) {
      return 'File transfer is unavailable: the local service did not answer.';
    }
    if (transfer.isActive) {
      final name = transfer.latest?.displayName ?? '';
      final count = transfer.activeCount == 1
          ? '1 transfer in progress'
          : '${transfer.activeCount} transfers in progress';
      return name.isEmpty ? '$count…' : '$count: $name';
    }
    final item = transfer.latest;
    if (item != null && item.isFailed) {
      return 'Last transfer failed: ${item.displayName}';
    }
    if (item != null && item.isCancelled) {
      return 'Last transfer cancelled: ${item.displayName}';
    }
    return '';
  }

  /// A typed reason label for the badge, when there is one worth showing.
  String get reasonLabel {
    switch (reasonCode) {
      case ipc.SessionReason.SESSION_REASON_NONE:
      case ipc.SessionReason.SESSION_REASON_UNSPECIFIED:
      case ipc.SessionReason.SESSION_REASON_USER_STOPPED:
        return '';
      default:
        return reasonCode.toString().replaceFirst('SESSION_REASON_', '');
    }
  }
}
