import 'package:flutter/material.dart';

import '../models/link_status.dart';

/// Presentation of the unified link status, in one place so the app bar badge
/// and the session card cannot drift into telling the user two different
/// stories about the same connection.
///
/// The mapping is severity-first: [LinkStatus.severity] is the model's judgement
/// about how loud the phase should be, and the UI only picks colours.

Color linkSeverityColor(ThemeData theme, LinkSeverity severity) {
  switch (severity) {
    case LinkSeverity.good:
      return Colors.green;
    case LinkSeverity.warning:
      return Colors.orange;
    case LinkSeverity.critical:
      return theme.colorScheme.error;
    case LinkSeverity.neutral:
      return Colors.grey;
  }
}

/// A phase's colour, with the one exception the severity band would flatten:
/// an in-progress negotiation is neutral but should still read as "working",
/// not as "nothing happening".
Color linkPhaseColor(ThemeData theme, LinkPhase phase) {
  if (phase == LinkPhase.connecting) return theme.colorScheme.primary;
  return linkSeverityColor(theme, LinkStatus.severityFor(phase));
}

IconData linkPhaseIcon(LinkPhase phase) {
  switch (phase) {
    case LinkPhase.offline:
      return Icons.cloud_off;
    case LinkPhase.disconnected:
      return Icons.radio_button_unchecked;
    case LinkPhase.connecting:
      return Icons.sync;
    case LinkPhase.connected:
      return Icons.link;
    case LinkPhase.streaming:
      return Icons.screen_share;
    case LinkPhase.reconnecting:
      return Icons.autorenew;
    case LinkPhase.failed:
      return Icons.error_outline;
  }
}
