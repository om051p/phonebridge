import 'dart:async';
import 'dart:collection';

import 'package:flutter/foundation.dart';

import '../generated/phonebridge/v1/phonebridge.pb.dart' as pb;
import '../models/notification_item.dart';
import '../services/notification_backend.dart';

/// Manages mirrored Android notifications on Linux desktop (DEC-028, Phase 8 v0.1).
///
/// Lifecycle and policy guarantees:
/// 1. Ephemeral In-Memory Only: Zero disk persistence. Notifications live only in this controller.
/// 2. Identity & Ordering: Keyed by sbn.key (`key`).
///    - New notifications are inserted at the front (newest first).
///    - In-place updates on matching key preserve position.
///    - Dismissals remove the matching item.
/// 3. Teardown: Disconnection or session reset purges all active records via [clear].
/// 4. Zero Logging: Never logs notification title, text, or subtext to console.
class NotificationController extends ChangeNotifier {
  NotificationController({NotificationBackend? backend})
      : _backend = backend ?? const UnsupportedNotificationBackend();

  final NotificationBackend _backend;
  final LinkedHashMap<String, NotificationItem> _items =
      LinkedHashMap<String, NotificationItem>();

  StreamSubscription<pb.NotificationFrame>? _streamSub;
  bool _isLoading = false;
  String? _errorMessage;

  List<NotificationItem> get notifications => List.unmodifiable(_items.values);

  int get count => _items.length;

  bool get isEmpty => _items.isEmpty;

  bool get isNotEmpty => _items.isNotEmpty;

  bool get isLoading => _isLoading;

  String? get errorMessage => _errorMessage;

  NotificationItem? get latest => _items.isEmpty ? null : _items.values.first;

  void initialize() {
    _subscribe();
    refresh();
  }

  void _subscribe() {
    if (!_backend.supportsNotifications) return;
    _streamSub?.cancel();
    _streamSub = _backend.notificationStream.listen(
      handleNotificationEvent,
      onError: (Object error) {
        _errorMessage = error.toString();
        notifyListeners();
      },
    );
  }

  Future<void> refresh() async {
    if (!_backend.supportsNotifications) return;

    _isLoading = true;
    _errorMessage = null;
    notifyListeners();

    try {
      final items = await _backend.listNotifications();
      _items.clear();
      for (final item in items) {
        _items[item.key] = item;
      }
      _errorMessage = null;
    } catch (e) {
      _errorMessage = e.toString();
    } finally {
      _isLoading = false;
      notifyListeners();
    }
  }

  void handleNotificationEvent(pb.NotificationFrame frame) {
    if (frame.hasPosted()) {
      final posted = frame.posted;
      final item = NotificationItem.fromProto(posted);
      applyPosted(item);
    } else if (frame.hasRemoved()) {
      final removed = frame.removed;
      applyRemoved(removed.key);
    }
  }

  void applyPosted(NotificationItem item) {
    if (_items.containsKey(item.key)) {
      // In-place update (e.g. progress update or new message in thread)
      _items[item.key] = item;
    } else {
      // Insert at the front (newest first)
      final updated = <String, NotificationItem>{};
      updated[item.key] = item;
      updated.addAll(_items);
      _items
        ..clear()
        ..addAll(updated);
    }
    notifyListeners();
  }

  void applyRemoved(String key) {
    if (_items.remove(key) != null) {
      notifyListeners();
    }
  }

  /// Requests dismissal of a notification on Android and removes it locally upon success.
  Future<bool> dismiss(String key) async {
    if (!_backend.supportsNotifications) return false;
    final success = await _backend.dismissNotification(key);
    if (success) {
      applyRemoved(key);
    }
    return success;
  }

  /// Purges all notifications from memory on disconnect or reset.
  void clear() {
    if (_items.isNotEmpty) {
      _items.clear();
      notifyListeners();
    }
  }

  @override
  void dispose() {
    _streamSub?.cancel();
    _streamSub = null;
    super.dispose();
  }
}
