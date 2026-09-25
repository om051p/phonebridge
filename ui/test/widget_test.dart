// Root-widget smoke test (replaces the generated counter scaffold, which
// referenced a MyApp/counting app that does not exist in this project).
//
// Verifies the real production root — PhoneBridgeApp over AppScaffold — builds
// on the single app-owned IPC stack, and pins the Phase 5/6 ownership contract
// at the root: exactly one daemon event subscription, and views never tearing
// down a stack they do not own.

import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
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
        default:
          return null;
      }
    });
  });

  tearDown(() {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(controlChannel, null);
  });

  testWidgets('app root builds the production mission-control shell',
      (tester) async {
    tester.view.physicalSize = const Size(1080, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    // Deterministic: the root runs through a controller-backed stack over a
    // fake client — on this host Platform.isLinux is true, so an unwired root
    // would otherwise handshake with the real daemon socket.
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

    // The production shell: app bar + all five primary destinations.
    expect(find.text('PhoneBridge'), findsOneWidget);
    expect(find.text('Home'), findsOneWidget);
    expect(find.text('Devices'), findsOneWidget);
    expect(find.text('Screen'), findsOneWidget);
    expect(find.text('Clipboard'), findsOneWidget);
    expect(find.text('Activity'), findsOneWidget);

    // Home opens first, in its no-peer state on the fake daemon.
    expect(find.text('No Paired PC'), findsOneWidget);
    expect(
      find.text('Pair with your Linux desktop to connect'),
      findsOneWidget,
    );

    // App construction wires exactly the single daemon subscription — the
    // scaffold's initialize() adds none of its own.
    expect(client.streamEventsCalls, 1);

    // Ownership: unmounting the root never tears down a stack the views do
    // not own...
    await tester.pumpWidget(const SizedBox());
    await tester.pump();
    expect(client.shutdownCalls, 0);

    // ...and the explicit owner's (idempotent) dispose cascades exactly once.
    controller.dispose();
    expect(client.shutdownCalls, 1);
  });
}
