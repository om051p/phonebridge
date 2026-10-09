import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/controllers/notification_controller.dart';
import 'package:phonebridge_ui/models/notification_item.dart';
import 'package:phonebridge_ui/ui/notification_views.dart';

void main() {
  Widget buildTestApp(Widget child) {
    return MaterialApp(
      theme: ThemeData(useMaterial3: true),
      home: Scaffold(
        body: SingleChildScrollView(child: child),
      ),
    );
  }

  group('NotificationTile', () {
    testWidgets('renders all notification fields and ongoing badge', (tester) async {
      final now = DateTime.now();
      final item = NotificationItem(
        key: 'tile-key-1',
        packageName: 'com.whatsapp',
        appName: 'WhatsApp',
        title: 'John Doe',
        text: 'Lunch tomorrow at noon?',
        subText: 'Family Group',
        postTime: now.subtract(const Duration(minutes: 5)),
        isOngoing: true,
        category: 'msg',
        receivedAt: now,
      );

      await tester.pumpWidget(buildTestApp(NotificationTile(item: item)));
      await tester.pumpAndSettle();

      expect(find.text('WhatsApp'), findsOneWidget);
      expect(find.text('John Doe'), findsOneWidget);
      expect(find.text('Lunch tomorrow at noon?'), findsOneWidget);
      expect(find.text('Family Group'), findsOneWidget);
      expect(find.text('ONGOING'), findsOneWidget);
      expect(find.byIcon(Icons.chat_bubble_outline), findsOneWidget);
    });

    testWidgets('falls back to default icon when category is empty', (tester) async {
      final now = DateTime.now();
      final item = NotificationItem(
        key: 'tile-key-2',
        packageName: 'com.example.app',
        appName: 'Example App',
        title: 'System Alert',
        text: 'Battery low',
        postTime: now,
        isOngoing: false,
        category: '',
        receivedAt: now,
      );

      await tester.pumpWidget(buildTestApp(NotificationTile(item: item)));
      await tester.pumpAndSettle();

      expect(find.text('Example App'), findsOneWidget);
      expect(find.text('System Alert'), findsOneWidget);
      expect(find.text('Battery low'), findsOneWidget);
      expect(find.text('ONGOING'), findsNothing);
      expect(find.byIcon(Icons.notifications_none), findsOneWidget);
    });

    testWidgets('renders dismiss button when isClearable is true and onDismiss provided', (tester) async {
      final now = DateTime.now();
      final item = NotificationItem(
        key: 'tile-key-clearable',
        packageName: 'com.example.app',
        appName: 'Example App',
        title: 'Title',
        text: 'Text',
        postTime: now,
        isClearable: true,
        receivedAt: now,
      );

      var dismissed = false;
      await tester.pumpWidget(buildTestApp(NotificationTile(
        item: item,
        onDismiss: () => dismissed = true,
      )));
      await tester.pumpAndSettle();

      expect(find.byIcon(Icons.close), findsOneWidget);
      await tester.tap(find.byIcon(Icons.close));
      await tester.pumpAndSettle();
      expect(dismissed, isTrue);
    });

    testWidgets('hides dismiss button when isClearable is false', (tester) async {
      final now = DateTime.now();
      final item = NotificationItem(
        key: 'tile-key-sticky',
        packageName: 'com.example.app',
        appName: 'Example App',
        title: 'Title',
        text: 'Text',
        postTime: now,
        isClearable: false,
        receivedAt: now,
      );

      await tester.pumpWidget(buildTestApp(NotificationTile(
        item: item,
        onDismiss: () {},
      )));
      await tester.pumpAndSettle();

      expect(find.byIcon(Icons.close), findsNothing);
    });
  });

  group('NotificationsSection', () {
    late NotificationController controller;

    setUp(() {
      controller = NotificationController();
    });

    tearDown(() {
      controller.dispose();
    });

    testWidgets('renders empty card when notifications list is empty', (tester) async {
      await tester.pumpWidget(buildTestApp(NotificationsSection(controller: controller)));
      await tester.pumpAndSettle();

      expect(find.text('NOTIFICATIONS'), findsOneWidget);
      expect(find.text('No notifications'), findsOneWidget);
      expect(find.text('Clear'), findsNothing);
    });

    testWidgets('renders notifications and handles Clear button', (tester) async {
      final now = DateTime.now();
      controller.applyPosted(NotificationItem(
        key: 'k1',
        packageName: 'org.telegram',
        appName: 'Telegram',
        title: 'Bob',
        text: 'Meeting started',
        postTime: now,
        receivedAt: now,
      ));

      await tester.pumpWidget(buildTestApp(NotificationsSection(controller: controller)));
      await tester.pumpAndSettle();

      expect(find.text('NOTIFICATIONS'), findsOneWidget);
      expect(find.text('1'), findsOneWidget); // badge count
      expect(find.text('Telegram'), findsOneWidget);
      expect(find.text('Bob'), findsOneWidget);
      expect(find.text('Meeting started'), findsOneWidget);
      expect(find.text('Clear'), findsOneWidget);

      await tester.tap(find.text('Clear'));
      await tester.pumpAndSettle();

      expect(controller.isEmpty, isTrue);
      expect(find.text('No notifications'), findsOneWidget);
      expect(find.text('Clear'), findsNothing);
    });
  });
}
