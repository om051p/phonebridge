import 'package:flutter/material.dart' hide ClipboardStatus;
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/controllers/phonebridge_controller.dart';
import 'package:phonebridge_ui/models/activity_event.dart';
import 'package:phonebridge_ui/screens/app_scaffold.dart';
import 'package:phonebridge_ui/screens/home_screen.dart';
import 'package:phonebridge_ui/screens/devices_screen.dart';
import 'package:phonebridge_ui/screens/screen_sharing_screen.dart';
import 'package:phonebridge_ui/screens/clipboard_screen.dart';
import 'package:phonebridge_ui/screens/activity_screen.dart';
import 'package:phonebridge_ui/screens/settings_screen.dart';
import 'package:phonebridge_ui/screens/diagnostics_screen.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  const MethodChannel controlChannel = MethodChannel('dev.phonebridge/control');

  setUp(() {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(controlChannel, (MethodCall call) async {
      switch (call.method) {
        case 'getDeviceState':
          return {
            'model': '23049PCD8I',
            'manufacturer': 'Xiaomi',
            'sdkInt': 35,
            'isCapturing': false,
            'goEngineLoaded': true,
            'codec': 'c2.qti.avc.encoder',
            'isHardwareCodec': true,
            'deviceId': 'a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2',
            'displayName': 'POCO F5',
            'clipboardState': 'AMBIENT_ACTIVE',
            'imeSelected': true,
          };
        case 'getMediaStats':
          return {
            'isCapturing': false,
            'encodedFrames': 150,
            'keyframes': 5,
            'admittedFrames': 150,
            'droppedFrames': 0,
            'codec': 'c2.qti.avc.encoder',
            'isHardwareCodec': true,
            'durationUs': 5000000,
            'timestampMs': 1000,
          };
        case 'getTrustedDevices':
          return [
            {
              'deviceId': 'fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210',
              'displayName': 'COSMIC Workstation',
              'platform': 'linux',
              'pairedAtMs': 1700000000000,
              'lastSeenMs': 1700000050000,
              'revoked': false,
            }
          ];
        case 'getClipboardStatus':
          return {
            'state': 'AMBIENT_ACTIVE',
            'imeSelected': true,
            'maxPayloadSize': 786432,
          };
        case 'startCapture':
          return true;
        case 'stopCapture':
          return true;
        case 'revokeDevice':
          return true;
        case 'removeDevice':
          return true;
        case 'triggerClipboardPull':
          return true;
        default:
          return null;
      }
    });
  });

  tearDown(() {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(controlChannel, null);
  });

  Widget buildTestWidget(Widget child) {
    return MaterialApp(
      theme: ThemeData(useMaterial3: true),
      home: child is Scaffold ? child : Scaffold(body: child),
    );
  }

  testWidgets('AppScaffold renders M3 NavigationBar with all 5 primary destinations', (tester) async {
    tester.view.physicalSize = const Size(1080, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final controller = PhoneBridgeController();
    await tester.pumpWidget(buildTestWidget(AppScaffold(controller: controller)));
    await tester.pumpAndSettle();

    expect(find.text('PhoneBridge'), findsOneWidget);
    expect(find.text('Home'), findsOneWidget);
    expect(find.text('Devices'), findsOneWidget);
    expect(find.text('Screen'), findsOneWidget);
    expect(find.text('Clipboard'), findsOneWidget);
    expect(find.text('Activity'), findsOneWidget);

    // Switch to Devices tab
    await tester.tap(find.text('Devices'));
    await tester.pumpAndSettle();
    expect(find.text('This Device (POCO F5)'), findsOneWidget);

    // Switch to Screen tab
    await tester.tap(find.text('Screen'));
    await tester.pumpAndSettle();
    expect(find.text('START SCREEN SHARING'), findsOneWidget);

    // Switch to Clipboard tab
    await tester.tap(find.text('Clipboard'));
    await tester.pumpAndSettle();
    expect(find.text('Clipboard Synchronization'), findsOneWidget);

    // Switch to Activity tab
    await tester.tap(find.text('Activity'));
    await tester.pumpAndSettle();
    expect(find.text('All'), findsOneWidget);
  });

  testWidgets('HomeScreen displays connection overview, hero card, and triggers screen share', (tester) async {
    tester.view.physicalSize = const Size(1080, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final controller = PhoneBridgeController();
    await controller.refreshAll();

    await tester.pumpWidget(
      buildTestWidget(
        HomeScreen(
          controller: controller,
          onNavigateToTab: (_) {},
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('No active connection'), findsOneWidget);
    expect(find.text('Screen Sharing'), findsOneWidget);
    expect(find.text('START SHARING'), findsOneWidget);

    await tester.tap(find.text('START SHARING'));
    await tester.pump();
  });

  testWidgets('DevicesScreen displays trusted device and opens detail sheet', (tester) async {
    tester.view.physicalSize = const Size(1080, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final controller = PhoneBridgeController();
    await controller.refreshAll();

    await tester.pumpWidget(buildTestWidget(DevicesScreen(controller: controller)));
    await tester.pumpAndSettle();

    expect(find.text('COSMIC Workstation'), findsOneWidget);
    // Trusted but undiscovered: grouped as unavailable, never as active.
    expect(find.text('Unavailable (1)'), findsOneWidget);

    await tester.tap(find.text('COSMIC Workstation'));
    await tester.pumpAndSettle();

    expect(find.text('Revoke Trust'), findsOneWidget);
    expect(find.text('Forget Device'), findsOneWidget);
  });

  testWidgets('ScreenSharingScreen allows preset selection and start action', (tester) async {
    tester.view.physicalSize = const Size(1080, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final controller = PhoneBridgeController();
    await controller.refreshAll();

    await tester.pumpWidget(buildTestWidget(ScreenSharingScreen(controller: controller)));
    await tester.pumpAndSettle();

    expect(find.text('720p HD (Balanced)'), findsOneWidget);
    expect(find.text('1080p FHD (Sharp Text)'), findsOneWidget);
    expect(find.text('START SCREEN SHARING'), findsOneWidget);

    await tester.tap(find.text('1080p FHD (Sharp Text)'));
    await tester.pumpAndSettle();
    expect(controller.selectedWidth, 1080);
    expect(controller.selectedHeight, 2400);
  });

  testWidgets('ClipboardScreen displays ambient status and triggers manual pull', (tester) async {
    tester.view.physicalSize = const Size(1080, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final controller = PhoneBridgeController();
    await controller.refreshAll();

    await tester.pumpWidget(buildTestWidget(ClipboardScreen(controller: controller)));
    await tester.pumpAndSettle();

    expect(find.text('Clipboard Synchronization'), findsOneWidget);
    expect(find.text('Ambient Sync Active'), findsOneWidget);
    expect(find.text('SYNC CURRENT CLIPBOARD NOW'), findsOneWidget);
    expect(find.textContaining('Zero-Logging Policy'), findsOneWidget);

    await tester.tap(find.text('SYNC CURRENT CLIPBOARD NOW'));
    await tester.pumpAndSettle();
  });

  testWidgets('ActivityScreen filters events by category and clears log', (tester) async {
    tester.view.physicalSize = const Size(1080, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final controller = PhoneBridgeController();
    controller.addActivityEvent(
      ActivityCategory.screen,
      'Screen Sharing Started',
      '720p @ 30fps',
      ActivityLevel.success,
    );
    controller.addActivityEvent(
      ActivityCategory.clipboard,
      'Clipboard Forwarded',
      'Remote update applied',
      ActivityLevel.info,
    );

    await tester.pumpWidget(buildTestWidget(ActivityScreen(controller: controller)));
    await tester.pumpAndSettle();

    expect(find.text('Screen Sharing Started'), findsOneWidget);
    expect(find.text('Clipboard Forwarded'), findsOneWidget);

    // Filter by screen
    await tester.tap(find.text('Screen'));
    await tester.pumpAndSettle();
    expect(find.text('Screen Sharing Started'), findsOneWidget);
    expect(find.text('Clipboard Forwarded'), findsNothing);

    // Clear log
    controller.clearActivityLog();
    await tester.pumpAndSettle();
    expect(find.text('No Activity Recorded'), findsOneWidget);
  });

  testWidgets('SettingsScreen and DiagnosticsScreen display correctly', (tester) async {
    tester.view.physicalSize = const Size(1080, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final controller = PhoneBridgeController();
    await controller.refreshAll();

    await tester.pumpWidget(buildTestWidget(SettingsScreen(controller: controller)));
    await tester.pumpAndSettle();

    expect(find.text('General'), findsOneWidget);
    expect(find.text('Connection'), findsOneWidget);
    expect(find.text('Diagnostics & Developer'), findsOneWidget);
    expect(find.text('About PhoneBridge'), findsOneWidget);

    // Open Diagnostics
    await tester.tap(find.widgetWithText(ListTile, 'Open Diagnostics Console'));
    await tester.pumpAndSettle();

    expect(find.byType(DiagnosticsScreen), findsOneWidget);
    expect(find.text('Live Telemetry (DEC-020/021)'), findsOneWidget);
    expect(find.text('Go Core Engine & Transport'), findsOneWidget);
    expect(find.text('Clipboard Engine Internals (DEC-023)'), findsOneWidget);
  });
}
