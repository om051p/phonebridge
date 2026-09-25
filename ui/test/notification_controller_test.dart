import 'package:fixnum/fixnum.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/controllers/notification_controller.dart';
import 'package:phonebridge_ui/generated/phonebridge/v1/phonebridge.pb.dart' as pb;
import 'package:phonebridge_ui/models/notification_item.dart';

void main() {
  group('NotificationController', () {
    late NotificationController controller;

    setUp(() {
      controller = NotificationController();
    });

    tearDown(() {
      controller.dispose();
    });

    test('initial state is empty', () {
      expect(controller.notifications, isEmpty);
      expect(controller.count, 0);
      expect(controller.isEmpty, isTrue);
      expect(controller.isNotEmpty, isFalse);
      expect(controller.latest, isNull);
    });

    test('applyPosted inserts new item at front (newest first)', () {
      final now = DateTime.now();
      final item1 = NotificationItem(
        key: '0|pkg.test|1|tag|1000',
        packageName: 'pkg.test',
        appName: 'Test App',
        title: 'First Notification',
        text: 'Hello 1',
        postTime: now.subtract(const Duration(seconds: 10)),
        receivedAt: now,
      );

      final item2 = NotificationItem(
        key: '0|pkg.test|2|tag|1000',
        packageName: 'pkg.test',
        appName: 'Test App',
        title: 'Second Notification',
        text: 'Hello 2',
        postTime: now,
        receivedAt: now,
      );

      var notified = 0;
      controller.addListener(() => notified++);

      controller.applyPosted(item1);
      expect(controller.count, 1);
      expect(controller.latest?.key, item1.key);
      expect(notified, 1);

      controller.applyPosted(item2);
      expect(controller.count, 2);
      expect(controller.latest?.key, item2.key);
      expect(controller.notifications.first.key, item2.key);
      expect(controller.notifications.last.key, item1.key);
      expect(notified, 2);
    });

    test('applyPosted updates in-place when key matches', () {
      final now = DateTime.now();
      final original = NotificationItem(
        key: '0|pkg.test|1|tag|1000',
        packageName: 'pkg.test',
        appName: 'Test App',
        title: 'Downloading...',
        text: '20%',
        postTime: now,
        receivedAt: now,
      );

      final updated = NotificationItem(
        key: '0|pkg.test|1|tag|1000',
        packageName: 'pkg.test',
        appName: 'Test App',
        title: 'Downloading...',
        text: '80%',
        postTime: now,
        receivedAt: now,
      );

      controller.applyPosted(original);
      expect(controller.count, 1);
      expect(controller.notifications.first.text, '20%');

      controller.applyPosted(updated);
      expect(controller.count, 1);
      expect(controller.notifications.first.text, '80%');
    });

    test('applyRemoved removes item by key and notifies listeners', () {
      final now = DateTime.now();
      final item = NotificationItem(
        key: '0|pkg.test|1|tag|1000',
        packageName: 'pkg.test',
        appName: 'Test App',
        title: 'Dismiss Me',
        text: 'Body',
        postTime: now,
        receivedAt: now,
      );

      controller.applyPosted(item);
      expect(controller.count, 1);

      var notified = 0;
      controller.addListener(() => notified++);

      controller.applyRemoved(item.key);
      expect(controller.count, 0);
      expect(controller.isEmpty, isTrue);
      expect(notified, 1);

      // Removing non-existent key does not notify
      controller.applyRemoved('non-existent');
      expect(notified, 1);
    });

    test('clear purges all notifications', () {
      final now = DateTime.now();
      controller.applyPosted(NotificationItem(
        key: 'k1',
        packageName: 'pkg',
        appName: 'App',
        title: 'T1',
        text: 'B1',
        postTime: now,
        receivedAt: now,
      ));
      controller.applyPosted(NotificationItem(
        key: 'k2',
        packageName: 'pkg',
        appName: 'App',
        title: 'T2',
        text: 'B2',
        postTime: now,
        receivedAt: now,
      ));

      expect(controller.count, 2);
      controller.clear();
      expect(controller.count, 0);
      expect(controller.notifications, isEmpty);
    });

    test('handleNotificationEvent handles posted and removed frames', () {
      final postedFrame = pb.NotificationFrame(
        version: 1,
        timestampMs: Int64(1700000000000),
        posted: pb.NotificationPosted(
          key: 'frame-key-1',
          packageName: 'com.example.chat',
          appName: 'Chat App',
          title: 'Alice',
          text: 'Hey there!',
          subText: 'Work Group',
          postTimeMs: Int64(1700000000000),
          isOngoing: false,
          isClearable: true,
          category: 'msg',
        ),
      );

      controller.handleNotificationEvent(postedFrame);
      expect(controller.count, 1);
      final item = controller.latest!;
      expect(item.key, 'frame-key-1');
      expect(item.appName, 'Chat App');
      expect(item.title, 'Alice');
      expect(item.text, 'Hey there!');
      expect(item.subText, 'Work Group');
      expect(item.category, 'msg');

      final removedFrame = pb.NotificationFrame(
        version: 1,
        timestampMs: Int64(1700000001000),
        removed: pb.NotificationRemoved(
          key: 'frame-key-1',
          packageName: 'com.example.chat',
          reason: 1,
        ),
      );

      controller.handleNotificationEvent(removedFrame);
      expect(controller.count, 0);
    });
  });
}
