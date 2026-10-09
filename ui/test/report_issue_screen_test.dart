import 'package:flutter/material.dart' hide ClipboardStatus;
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/controllers/phonebridge_controller.dart';
import 'package:phonebridge_ui/screens/about_screen.dart';
import 'package:phonebridge_ui/screens/report_issue_screen.dart';
import 'package:phonebridge_ui/screens/settings_screen.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  const MethodChannel controlChannel = MethodChannel('dev.phonebridge/control');

  setUp(() {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(controlChannel, (MethodCall call) async {
      switch (call.method) {
        case 'getDeviceState':
          return {
            'model': 'Pixel 4 XL',
            'manufacturer': 'Google',
            'sdkInt': 33,
            'isCapturing': false,
            'goEngineLoaded': true,
            'codec': 'c2.android.avc.encoder',
            'isHardwareCodec': true,
            'deviceId': 'test-dev-id',
            'displayName': 'Pixel 4 XL',
            'clipboardState': 'AMBIENT_ACTIVE',
            'imeSelected': true,
          };
        case 'getMediaStats':
          return {
            'isCapturing': false,
            'encodedFrames': 0,
            'keyframes': 0,
            'admittedFrames': 0,
            'droppedFrames': 0,
            'codec': 'c2.android.avc.encoder',
            'isHardwareCodec': true,
            'durationUs': 0,
            'timestampMs': 1000,
          };
        case 'getTrustedDevices':
          return [];
        case 'getClipboardStatus':
          return {
            'state': 'AMBIENT_ACTIVE',
            'imeSelected': true,
            'enabled': true,
            'maxPayloadSize': 1048576,
          };
        case 'getDiscoveredDevices':
          return [];
        case 'getPermissionsStatus':
          return {
            'postNotifications': true,
            'notificationListener': true,
            'accessibility': true,
            'notificationServiceActive': true,
            'accessibilityServiceActive': true,
            'foregroundServiceActive': true,
            'sdkInt': 33,
          };
        case 'listTransfers':
          return [];
        case 'openEmailClient':
          return true;
        default:
          return null;
      }
    });

    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(SystemChannels.platform, (MethodCall call) async {
      if (call.method == 'Clipboard.setData') {
        return null;
      }
      return null;
    });
  });

  tearDown(() {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(controlChannel, null);
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(SystemChannels.platform, null);
  });

  testWidgets('ReportIssueScreen renders developer contact and controls', (tester) async {
    tester.view.physicalSize = const Size(1080, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final controller = PhoneBridgeController();
    controller.initialize();

    await tester.pumpWidget(
      MaterialApp(
        home: ReportIssueScreen(controller: controller),
      ),
    );
    await tester.pumpAndSettle();

    // Verify Developer Info is prominently displayed
    expect(find.text('Raxa Technologies'), findsOneWidget);
    expect(find.text('raxatechnologies@gmail.com'), findsOneWidget);

    // Verify categories
    expect(find.text('Connection & Pairing'), findsOneWidget);
    expect(find.text('Screen Mirroring'), findsOneWidget);
    expect(find.text('Clipboard Sync'), findsOneWidget);

    // Switch category
    await tester.tap(find.text('Screen Mirroring'));
    await tester.pumpAndSettle();

    // Enter text in title and description
    await tester.enterText(find.byType(TextField).first, 'Test Bug Title');
    await tester.enterText(find.byType(TextField).last, 'Steps to reproduce the bug');
    await tester.pumpAndSettle();

    // Verify diagnostics preview is expandable
    expect(find.text('Include Privacy-Safe System Diagnostics'), findsOneWidget);
    expect(find.text('Diagnostic Snapshot Preview'), findsOneWidget);

    // Tap copy report in AppBar
    final copyBtn = find.byTooltip('Copy Report to Clipboard');
    expect(copyBtn, findsOneWidget);
    await tester.tap(copyBtn);
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 300));

    expect(find.textContaining('Bug report copied to clipboard'), findsOneWidget);
  });

  testWidgets('AboutScreen renders developer support card and opens ReportIssueScreen', (tester) async {
    tester.view.physicalSize = const Size(1080, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final controller = PhoneBridgeController();
    controller.initialize();

    await tester.pumpWidget(
      MaterialApp(
        home: AboutScreen(controller: controller),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('About PhoneBridge'), findsOneWidget);
    expect(find.text('Raxa Technologies'), findsOneWidget);
    expect(find.text('raxatechnologies@gmail.com'), findsOneWidget);

    // Tap developer card to navigate to report screen
    await tester.tap(find.text('Raxa Technologies'));
    await tester.pumpAndSettle();

    expect(find.text('Report an Issue / Feedback'), findsOneWidget);
    expect(find.text('Send Email Report'), findsOneWidget);
  });

  testWidgets('SettingsScreen has Support & Feedback section navigating to ReportIssueScreen', (tester) async {
    tester.view.physicalSize = const Size(1080, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final controller = PhoneBridgeController();
    controller.initialize();

    await tester.pumpWidget(
      MaterialApp(
        home: SettingsScreen(controller: controller),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Support & Feedback'), findsOneWidget);
    expect(find.text('Report an Issue / Feedback'), findsOneWidget);

    await tester.tap(find.text('Report an Issue / Feedback'));
    await tester.pumpAndSettle();

    expect(find.text('Report an Issue / Feedback'), findsOneWidget);
    expect(find.text('Raxa Technologies'), findsOneWidget);
  });
}
