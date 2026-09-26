import 'dart:async';
import 'package:flutter/material.dart' hide ClipboardStatus;
import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/controllers/phonebridge_controller.dart';
import 'package:phonebridge_ui/models/capture_stats.dart';
import 'package:phonebridge_ui/models/clipboard_status.dart';
import 'package:phonebridge_ui/models/device_state.dart';
import 'package:phonebridge_ui/models/transfer_item.dart';
import 'package:phonebridge_ui/models/trusted_device.dart';
import 'package:phonebridge_ui/services/android_bridge_service.dart';
import 'package:phonebridge_ui/services/phonebridge_channel.dart';
import 'package:phonebridge_ui/ui/permission_onboarding_card.dart';

class _FakeChannel extends PhoneBridgeChannel {
  Map<String, dynamic> status = {
    'postNotifications': false,
    'notificationListener': false,
    'accessibility': false,
  };

  bool requestNotifCalled = false;
  bool openNotifListenerCalled = false;
  bool openAccessibilityCalled = false;
  bool openAppDetailsCalled = false;

  @override
  Future<Map<String, dynamic>> getPermissionsStatus() async => status;

  @override
  Future<bool> requestNotificationPermission() async {
    requestNotifCalled = true;
    status['postNotifications'] = true;
    return true;
  }

  @override
  Future<bool> openNotificationListenerSettings() async {
    openNotifListenerCalled = true;
    return true;
  }

  @override
  Future<bool> openAccessibilitySettings() async {
    openAccessibilityCalled = true;
    return true;
  }

  @override
  Future<bool> openAppDetailsSettings() async {
    openAppDetailsCalled = true;
    return true;
  }

  @override
  Future<DeviceState> getDeviceState() async => DeviceState.initial;
  @override
  Future<CaptureStats> getMediaStats() async => CaptureStats.initial;
  @override
  Future<List<TrustedDevice>> getTrustedDevices() async => [];
  @override
  Future<ClipboardStatus> getClipboardStatus() async => ClipboardStatus.initial;
  @override
  Stream<Map<dynamic, dynamic>> get rawEventsStream => const Stream.empty();
  @override
  Stream<CaptureStats> get statsStream => const Stream.empty();
  @override
  Stream<TransferItem> get transferStream => const Stream.empty();
  @override
  Future<List<TransferItem>> listTransfers() async => [];
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  late _FakeChannel channel;
  late AndroidBridgeService service;
  late PhoneBridgeController controller;

  setUp(() {
    channel = _FakeChannel();
    service = AndroidBridgeService(channel: channel);
    controller = PhoneBridgeController(service: service);
  });

  tearDown(() {
    controller.dispose();
  });

  test('PhoneBridgeController initial permissions state and refresh', () async {
    expect(controller.permissionsStatus, isEmpty);
    expect(controller.postNotificationsGranted, isFalse);
    expect(controller.notificationListenerEnabled, isFalse);
    expect(controller.accessibilityEnabled, isFalse);

    await controller.refreshPermissions();

    expect(controller.postNotificationsGranted, isFalse);
    expect(controller.notificationListenerEnabled, isFalse);
    expect(controller.accessibilityEnabled, isFalse);
    expect(controller.hasMissingPermissions, isTrue);

    await controller.requestNotificationPermission();
    expect(channel.requestNotifCalled, isTrue);
    expect(controller.postNotificationsGranted, isTrue);
  });

  testWidgets('PermissionOnboardingCard renders action buttons when permissions missing',
      (tester) async {
    await controller.refreshPermissions();

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: PermissionOnboardingCard(controller: controller),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Android Permissions Setup (0/3)'), findsOneWidget);
    expect(find.text('Allow'), findsOneWidget);
    expect(find.text('Open Settings'), findsNWidgets(2));
    expect(find.text('Android 13+ Restricted Settings Guidance'), findsOneWidget);
    expect(find.text('Open App Info'), findsOneWidget);

    // Tap Allow
    await tester.tap(find.text('Allow'));
    await tester.pumpAndSettle();
    expect(channel.requestNotifCalled, isTrue);
    expect(controller.postNotificationsGranted, isTrue);

    // Tap Open App Info
    await tester.tap(find.text('Open App Info'));
    await tester.pumpAndSettle();
    expect(channel.openAppDetailsCalled, isTrue);
  });

  testWidgets('PermissionOnboardingCard shows configured state when all permissions active',
      (tester) async {
    channel.status = {
      'postNotifications': true,
      'notificationListener': true,
      'accessibility': true,
    };
    await controller.refreshPermissions();
    expect(controller.hasMissingPermissions, isFalse);

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: PermissionOnboardingCard(controller: controller),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Permissions Configured'), findsOneWidget);
    expect(find.text('Active'), findsNWidgets(3));
    expect(find.text('Android 13+ Restricted Settings Guidance'), findsNothing);
  });
}
