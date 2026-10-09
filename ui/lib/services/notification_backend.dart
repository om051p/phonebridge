import '../generated/phonebridge/v1/phonebridge.pb.dart' as pb;
import '../models/notification_item.dart';

/// The narrow notification surface the UI depends on (DEC-028, Phase 8 v0.1).
abstract class NotificationBackend {
  /// True when the platform supports notification mirroring (e.g. Linux desktop).
  bool get supportsNotifications;

  /// Pushed notification events (posted, updated, removed) from paired phone.
  Stream<pb.NotificationFrame> get notificationStream;

  /// In-memory active notifications from daemon.
  Future<List<NotificationItem>> listNotifications();

  /// Requests dismissal of a mirrored notification on Android.
  Future<bool> dismissNotification(String key);
}

/// Fallback for platforms where notifications are not mirrored (e.g. Android).
class UnsupportedNotificationBackend implements NotificationBackend {
  const UnsupportedNotificationBackend();

  @override
  bool get supportsNotifications => false;

  @override
  Stream<pb.NotificationFrame> get notificationStream => const Stream.empty();

  @override
  Future<List<NotificationItem>> listNotifications() async => const [];

  @override
  Future<bool> dismissNotification(String key) async => false;
}
