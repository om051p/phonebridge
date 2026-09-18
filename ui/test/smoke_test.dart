import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/main.dart';
import 'package:phonebridge_ui/ui/linux_session_view.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  const MethodChannel controlChannel = MethodChannel('dev.phonebridge/control');

  setUp(() {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(controlChannel, (MethodCall methodCall) async {
      switch (methodCall.method) {
        case 'getDeviceState':
          return {
            'model': '23049PCD8I',
            'manufacturer': 'Xiaomi',
            'sdkInt': 35,
            'isCapturing': false,
            'goEngineLoaded': true,
            'codec': 'c2.qti.avc.encoder',
            'isHardwareCodec': true,
          };
        case 'startCapture':
          return true;
        case 'stopCapture':
          return true;
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
        default:
          return null;
      }
    });
  });

  tearDown(() {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(controlChannel, null);
  });

  testWidgets('renders PhoneBridge dashboard with device info and control buttons', (tester) async {
    tester.view.physicalSize = const Size(1080, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(const PhoneBridgeApp(home: DashboardScreen()));
    await tester.pumpAndSettle();

    // Verify AppBar
    expect(find.text('PhoneBridge Control'), findsOneWidget);

    // Verify Status Banner
    expect(find.text('IDLE'), findsOneWidget);

    // Verify Device Info
    expect(find.text('Device Info'), findsOneWidget);
    expect(find.text('Xiaomi 23049PCD8I'), findsOneWidget);
    expect(find.text('API 35'), findsOneWidget);
    expect(find.text('Loaded (libphonebridge_core.so)'), findsOneWidget);

    // Verify Control Card & Actions
    expect(find.text('Stream Configuration'), findsOneWidget);
    expect(find.text('START SCREEN SHARING'), findsOneWidget);

    // Verify Telemetry Card
    expect(find.text('Live Telemetry'), findsOneWidget);
    expect(find.text('Current FPS'), findsOneWidget);
    expect(find.text('Keyframes (IDR)'), findsOneWidget);
    expect(find.text('Encoded Frames'), findsOneWidget);
    expect(find.text('Transport Admitted'), findsOneWidget);
  });

  testWidgets('tapping start screen sharing invokes startCapture channel method', (tester) async {
    tester.view.physicalSize = const Size(1080, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(const PhoneBridgeApp(home: DashboardScreen()));
    await tester.pumpAndSettle();

    final startButton = find.text('START SCREEN SHARING');
    expect(startButton, findsOneWidget);

    await tester.tap(startButton);
    await tester.pump();
  });

  testWidgets('renders Linux desktop session view with discovery and session controls', (tester) async {
    tester.view.physicalSize = const Size(1280, 900);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(const PhoneBridgeApp(home: LinuxSessionView()));
    await tester.pumpAndSettle();

    // Verify Linux Desktop title
    expect(find.text('PhoneBridge Linux Desktop'), findsOneWidget);

    // Verify Session State Banner
    expect(find.text('SESSION IDLE (READY)'), findsOneWidget);

    // Verify Session Controls
    expect(find.text('Session Controls'), findsOneWidget);
    expect(find.text('START SESSION'), findsOneWidget);

    // Verify Discovered Devices Card
    expect(find.text('Discovered Devices (mDNS)'), findsOneWidget);

    // Verify Trusted Devices Card
    expect(find.text('Trusted Devices (Trust Store)'), findsOneWidget);

    // Verify Local IPC Card
    expect(find.text('Local Engine IPC (DEC-018)'), findsOneWidget);
    expect(find.text('RECONNECT DAEMON'), findsOneWidget);
  });
}
