// Root-widget smoke test (replaces the generated counter scaffold, which
// referenced a MyApp/counting app that does not exist in this project).
//
// Follows the smoke_test.dart conventions: mock the dev.phonebridge/control
// platform channel so the dashboard renders deterministically, then verify the
// real app root builds and shows the PhoneBridge dashboard.

import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:phonebridge_ui/main.dart';

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

  testWidgets('app root builds the PhoneBridge dashboard', (tester) async {
    tester.view.physicalSize = const Size(1080, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(const PhoneBridgeApp(home: DashboardScreen()));
    await tester.pumpAndSettle();

    expect(find.text('PhoneBridge Control'), findsOneWidget);
    expect(find.text('IDLE'), findsOneWidget);
    expect(find.text('START SCREEN SHARING'), findsOneWidget);
  });
}
