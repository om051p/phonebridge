// In-session presentation, extracted from the retired desktop session view so
// the mounted Screen tab (Phase 6) renders the exact same validated session
// information without keeping its own copy of anything.
//
// Every widget here is a *pure read*: it takes the shared [SessionStatus] /
// [StreamStats] as input and renders it. No widget subscribes, polls, or holds
// session state — that is [SessionController]'s single job, which is what keeps
// every surface that renders a session from ever disagreeing about it.

import 'package:flutter/material.dart';

import '../generated/phonebridge/localipc/v1/local_ipc.pb.dart';
import '../models/session_status.dart';

/// The session banner: state label, duration, session id, status detail,
/// recovery progress, the requested-vs-applied negotiation, the typed reason
/// badge, and the failure/error lines.
class SessionStateBanner extends StatelessWidget {
  const SessionStateBanner({
    super.key,
    required this.status,
    required this.sessionId,
  });

  /// The shared session status to render — never a local copy.
  final SessionStatus status;

  /// Live session id, or '' when the daemon reports none.
  final String sessionId;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final color = _stateColor(status.state);
    final label = status.label;
    final icon = _stateIcon(status.state);
    final negotiation = status.negotiationSummary();
    final failure = status.failureSummary;
    final recovery = status.recoveringDetail;
    final badge = status.reasonBadge;

    return Card(
      elevation: 0,
      color: color.withValues(alpha: 0.12),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(12),
        side: BorderSide(color: color.withValues(alpha: 0.4)),
      ),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(icon, color: color, size: 24),
                const SizedBox(width: 12),
                Expanded(
                  child: Text(
                    label,
                    style: theme.textTheme.titleMedium?.copyWith(
                      fontWeight: FontWeight.bold,
                      color: color,
                    ),
                  ),
                ),
                if (status.durationMs > 0)
                  Text(
                    'Duration: ${_formatDuration(status.durationMs)}',
                    style: theme.textTheme.bodyMedium?.copyWith(
                      fontWeight: FontWeight.w600,
                    ),
                  ),
              ],
            ),
            if (sessionId.isNotEmpty) ...[
              const SizedBox(height: 6),
              Text(
                'Session ID: $sessionId',
                style: theme.textTheme.bodySmall?.copyWith(fontFamily: 'monospace'),
              ),
            ],
            if (status.reasonDetail.isNotEmpty) ...[
              const SizedBox(height: 4),
              Text(
                'Status detail: ${status.reasonDetail}',
                style: theme.textTheme.bodySmall?.copyWith(color: theme.colorScheme.onSurfaceVariant),
              ),
            ],
            // Automatic recovery: presented as progress, not as an error.
            if (recovery.isNotEmpty) ...[
              const SizedBox(height: 6),
              Text(
                recovery,
                style: theme.textTheme.bodySmall?.copyWith(
                  color: color,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ],
            // What was asked for versus what the phone said it applies. The
            // requested tuple is never shown as if it had been confirmed.
            if (negotiation.isNotEmpty) ...[
              const SizedBox(height: 6),
              Text(
                negotiation,
                style: theme.textTheme.bodySmall?.copyWith(fontFamily: 'monospace'),
              ),
            ],
            if (badge.isNotEmpty) ...[
              const SizedBox(height: 6),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                decoration: BoxDecoration(
                  borderRadius: BorderRadius.circular(6),
                  border: Border.all(color: color.withValues(alpha: 0.6)),
                ),
                child: Text(
                  badge,
                  style: theme.textTheme.labelSmall?.copyWith(
                    color: color,
                    fontWeight: FontWeight.bold,
                    letterSpacing: 0.5,
                  ),
                ),
              ),
            ],
            if (failure.isNotEmpty) ...[
              const SizedBox(height: 6),
              Text(
                failure,
                style: theme.textTheme.bodySmall?.copyWith(
                  color: theme.colorScheme.error,
                  fontWeight: FontWeight.bold,
                ),
              ),
            ],
            if (status.errorMessage.isNotEmpty) ...[
              const SizedBox(height: 4),
              Text(
                'Detail: ${status.errorMessage}',
                style: theme.textTheme.bodySmall?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }
}

/// The receiver's display-sink state, as reported by the session snapshot.
///
/// This used to be static copy claiming "Active (ffplay)" — untrue on a
/// headless daemon, which silently runs a null sink. The widget now renders
/// the daemon-reported [SinkKind] and its liveness; callers gate it on
/// [SessionStatus.hasReportedSink] so an unclassified session shows nothing
/// rather than a guess.
class VideoDisplayBanner extends StatelessWidget {
  const VideoDisplayBanner({
    super.key,
    required this.sinkKind,
    required this.sinkActive,
  });

  final SinkKind sinkKind;
  final bool sinkActive;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final displayActive = sinkKind == SinkKind.SINK_KIND_DISPLAY && sinkActive;
    final (title, detail, icon, tint) = switch (sinkKind) {
      SinkKind.SINK_KIND_DISPLAY => displayActive
          ? (
              'Display Sink: Active (ffplay)',
              'Hardware H.264 Annex-B stream is piped to low-latency Linux display window via Receiver pipeline.',
              Icons.desktop_windows,
              theme.colorScheme.primary,
            )
          : (
              'Display Sink: Inactive',
              'The display window is not accepting frames; video is received but nothing is rendering it.',
              Icons.desktop_windows_outlined,
              Colors.orange,
            ),
      SinkKind.SINK_KIND_NULL => (
          'Display Sink: None (Headless)',
          'This daemon runs without a graphical session; frames are received and discarded (null sink).',
          Icons.desktop_access_disabled,
          Colors.orange,
        ),
      SinkKind.SINK_KIND_PIPE => (
          'Display Sink: Pipe (Forwarding)',
          'Frames are piped to a downstream process instead of a local window.',
          Icons.plumbing,
          theme.colorScheme.primary,
        ),
      SinkKind.SINK_KIND_FILE => (
          'Display Sink: File (Recording)',
          'Frames are being recorded to the configured capture files.',
          Icons.video_file,
          theme.colorScheme.primary,
        ),
      _ => (
          'Display Sink: Unknown',
          'The local engine has not classified the sink for this session.',
          Icons.desktop_windows,
          theme.colorScheme.onSurfaceVariant,
        ),
    };
    return Card(
      color: theme.colorScheme.primaryContainer
          .withValues(alpha: displayActive ? 0.25 : 0.15),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(12),
        side: BorderSide(color: tint.withValues(alpha: 0.4)),
      ),
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Row(
          children: [
            Icon(icon, size: 36, color: tint),
            const SizedBox(width: 16),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    style: theme.textTheme.titleSmall?.copyWith(fontWeight: FontWeight.bold),
                  ),
                  const SizedBox(height: 4),
                  Text(
                    detail,
                    style: theme.textTheme.bodySmall?.copyWith(color: theme.colorScheme.onSurfaceVariant),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Live stream telemetry, read from the shared controller's single poll.
/// Renders zeros until the first snapshot lands — the card never invents data.
class StreamTelemetryCard extends StatelessWidget {
  const StreamTelemetryCard({super.key, required this.stats});

  /// Latest stats from [SessionController]; null before the first snapshot.
  final StreamStats? stats;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final s = stats;

    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(Icons.analytics, color: theme.colorScheme.primary),
                const SizedBox(width: 8),
                Text('Live Stream Telemetry', style: theme.textTheme.titleMedium),
              ],
            ),
            const Divider(),
            Row(
              children: [
                _metricBox(
                  theme,
                  'RTP Packets',
                  s != null ? s.packets.toString() : '0',
                  Icons.send,
                  Colors.blue,
                ),
                const SizedBox(width: 12),
                _metricBox(
                  theme,
                  'Access Units',
                  s != null ? s.accessUnits.toString() : '0',
                  Icons.video_library,
                  Colors.purple,
                ),
              ],
            ),
            const SizedBox(height: 12),
            Row(
              children: [
                _metricBox(
                  theme,
                  'H.264 Data',
                  s != null ? _formatBytes(s.bytesH264.toInt()) : '0 B',
                  Icons.data_usage,
                  Colors.green,
                ),
                const SizedBox(width: 12),
                _metricBox(
                  theme,
                  'Keyframes (IDR)',
                  s != null ? s.keyframes.toString() : '0',
                  Icons.key,
                  Colors.amber,
                ),
              ],
            ),
            const SizedBox(height: 12),
            _infoRow(
              'Dropped AUs',
              '${s?.droppedAus ?? 0}',
              valueColor: (s?.droppedAus.toInt() ?? 0) > 0 ? Colors.orange : null,
            ),
            // Transport counters from the receiver's depacketizer: link-quality
            // regressions are visible here without reading daemon logs.
            _infoRow(
              'RTP Seq Gaps',
              '${s?.seqGaps ?? 0}',
              valueColor: (s?.seqGaps.toInt() ?? 0) > 0 ? Colors.orange : null,
            ),
            _infoRow(
              'RTP Duplicates',
              '${s?.dupSeq ?? 0}',
              valueColor: (s?.dupSeq.toInt() ?? 0) > 0 ? Colors.orange : null,
            ),
            _infoRow(
              'RTP Late Packets',
              '${s?.latePackets ?? 0}',
              valueColor: (s?.latePackets.toInt() ?? 0) > 0 ? Colors.orange : null,
            ),
            _infoRow(
              'TS Backward',
              '${s?.tsBackward ?? 0}',
              valueColor: (s?.tsBackward.toInt() ?? 0) > 0 ? Colors.orange : null,
            ),
            _infoRow(
              'RTP Total Payload',
              s != null ? _formatBytes(s.bytesRtp.toInt()) : '0 B',
            ),
          ],
        ),
      ),
    );
  }
}

String _formatDuration(int ms) {
  if (ms <= 0) return '00:00';
  final totalSec = ms ~/ 1000;
  final min = (totalSec ~/ 60).toString().padLeft(2, '0');
  final sec = (totalSec % 60).toString().padLeft(2, '0');
  return '$min:$sec';
}

String _formatBytes(int bytes) {
  if (bytes < 1024) return '$bytes B';
  if (bytes < 1024 * 1024) return '${(bytes / 1024).toStringAsFixed(1)} KB';
  return '${(bytes / (1024 * 1024)).toStringAsFixed(2)} MB';
}

Color _stateColor(SessionState state) {
  switch (state) {
    case SessionState.SESSION_STATE_STREAMING:
      return Colors.green;
    case SessionState.SESSION_STATE_CONNECTED:
      return Colors.teal;
    case SessionState.SESSION_STATE_CONNECTING:
    case SessionState.SESSION_STATE_DISCOVERING:
      return Colors.amber;
    case SessionState.SESSION_STATE_RECONNECTING:
      return Colors.orange;
    case SessionState.SESSION_STATE_FAILED:
      return Colors.red;
    default:
      return Colors.grey;
  }
}

IconData _stateIcon(SessionState state) {
  switch (state) {
    case SessionState.SESSION_STATE_STREAMING:
      return Icons.play_circle_fill;
    case SessionState.SESSION_STATE_CONNECTED:
      return Icons.check_circle;
    case SessionState.SESSION_STATE_CONNECTING:
      return Icons.sync;
    case SessionState.SESSION_STATE_DISCOVERING:
      return Icons.search;
    case SessionState.SESSION_STATE_RECONNECTING:
      return Icons.autorenew;
    case SessionState.SESSION_STATE_FAILED:
      return Icons.error;
    case SessionState.SESSION_STATE_STOPPED:
      return Icons.stop_circle;
    default:
      return Icons.pause_circle_outline;
  }
}

Widget _metricBox(ThemeData theme, String label, String value, IconData icon, Color iconColor) {
  return Expanded(
    child: Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.4),
        borderRadius: BorderRadius.circular(8),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(icon, size: 18, color: iconColor),
              const SizedBox(width: 6),
              Expanded(
                child: Text(
                  label,
                  style: theme.textTheme.labelMedium?.copyWith(
                    color: theme.colorScheme.onSurfaceVariant,
                  ),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ),
            ],
          ),
          const SizedBox(height: 6),
          Text(
            value,
            style: theme.textTheme.headlineSmall?.copyWith(
              fontWeight: FontWeight.bold,
            ),
          ),
        ],
      ),
    ),
  );
}

Widget _infoRow(String label, String value, {Color? valueColor}) {
  return Padding(
    padding: const EdgeInsets.symmetric(vertical: 4.0),
    child: Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        Flexible(
          flex: 2,
          child: Text(label, style: const TextStyle(fontWeight: FontWeight.w500)),
        ),
        const SizedBox(width: 8),
        Flexible(
          flex: 3,
          child: Text(
            value,
            textAlign: TextAlign.end,
            style: TextStyle(
              fontWeight: FontWeight.bold,
              color: valueColor,
            ),
          ),
        ),
      ],
    ),
  );
}
