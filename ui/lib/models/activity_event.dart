enum ActivityCategory {
  connection,
  screen,
  clipboard,
  pairing,
  system;

  String get label {
    switch (this) {
      case ActivityCategory.connection:
        return 'Connection';
      case ActivityCategory.screen:
        return 'Screen Share';
      case ActivityCategory.clipboard:
        return 'Clipboard';
      case ActivityCategory.pairing:
        return 'Pairing';
      case ActivityCategory.system:
        return 'System';
    }
  }
}

enum ActivityLevel {
  info,
  success,
  warning,
  error;
}

class ActivityEvent {
  final String id;
  final DateTime timestamp;
  final ActivityCategory category;
  final String title;
  final String detail;
  final ActivityLevel level;

  const ActivityEvent({
    required this.id,
    required this.timestamp,
    required this.category,
    required this.title,
    required this.detail,
    this.level = ActivityLevel.info,
  });

  String get timeAgo {
    final diff = DateTime.now().difference(timestamp);
    if (diff.inSeconds < 45) return 'Just now';
    if (diff.inMinutes < 60) return '${diff.inMinutes}m ago';
    if (diff.inHours < 24) return '${diff.inHours}h ago';
    return '${diff.inDays}d ago';
  }
}
