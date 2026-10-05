import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/main.dart';
import 'package:phonebridge_ui/services/linux_bridge_service.dart';

import 'support/fake_ipc_client.dart';

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

  testWidgets('renders the mission-control shell with home surfaces',
      (tester) async {
    tester.view.physicalSize = const Size(1080, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    // Deterministic: the shell runs through a controller-backed stack over a
    // fake client, not a real daemon socket.
    final client = FakeIpcClient();
    final controller = PhoneBridgeController(
      service: LinuxBridgeService(client: client),
    );
    addTearDown(controller.dispose);
    addTearDown(client.events.close);
    await tester.pumpWidget(PhoneBridgeApp(
      home: AppScaffold(controller: controller),
    ));
    await tester.pumpAndSettle();

    // App bar + the home hero in its no-peer state (empty trust store) and
    // its pairing action.
    expect(find.text('PhoneBridge'), findsOneWidget);
    expect(find.text('No paired device'), findsOneWidget);
    expect(
      find.text('Pair with your other device to connect'),
      findsOneWidget,
    );
    expect(find.text('Pair'), findsOneWidget);

    // One subscription for the whole shell.
    expect(client.streamEventsCalls, 1);
  });

  testWidgets('mirroring from the Screen tab reaches the shared session seam',
      (tester) async {
    tester.view.physicalSize = const Size(1080, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    // Same controller-backed path as the render test. The tap must reach the
    // service's session seam (the shared capture path).
    final client = FakeIpcClient();
    final controller = PhoneBridgeController(
      service: LinuxBridgeService(client: client),
    );
    // Explicit target for the tab's start control (Phase C).
    controller.setReceiverUrl('test-peer');
    addTearDown(controller.dispose);
    addTearDown(client.events.close);
    await tester.pumpWidget(PhoneBridgeApp(
      home: AppScaffold(controller: controller),
    ));
    await tester.pumpAndSettle();

    // Screen tab: the Linux service path labels the action "mirror", and the
    // quality presets render.
    await tester.tap(find.text('Screen'));
    await tester.pumpAndSettle();
    final mirrorButton = find.text('MIRROR PHONE SCREEN');
    expect(mirrorButton, findsOneWidget);
    expect(find.text('720p HD (Balanced)'), findsOneWidget);

    await tester.tap(mirrorButton);
    await tester.pump();
    await tester.pump();

    // The tap reached the shared session seam exactly once — the same path
    // every surface uses (no screen-level capture channel survives).
    expect(client.startCalls, hasLength(1));
  });

  // The standalone Linux session view was retired in Phase 6 consolidation:
  // its session presentation, controls, discovery/pairing and daemon
  // diagnostics now live on the Screen, Devices and Diagnostics surfaces,
  // each covered by its own test (screen_session_control_test,
  // devices_pairing_test, diagnostics_ipc_test, screen_in_session_test).
}
