// The native trustChanged method call (primary push path: the EventChannel
// sink is shared by several broadcast subscriptions, so delivery of a
// trustChanged *event* is not guaranteed) must refresh the trust list.
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:phonebridge_ui/main.dart';
import 'package:phonebridge_ui/models/trusted_device.dart';

import 'support/fake_bridge_service.dart';

void main() {
  testWidgets('native trustChanged call refreshes trusted devices',
      (tester) async {
    final service = FakeBridgeService();
    final controller = PhoneBridgeController(service: service);
    addTearDown(controller.dispose);

    await tester.pumpWidget(
      MaterialApp(home: AppScaffold(controller: controller)),
    );
    await tester.pumpAndSettle();

    expect(controller.trustedDevices, isEmpty);
    final handler = service.nativeCallHandler;
    expect(handler, isNotNull, reason: 'scaffold must register a native handler');
    final callsAfterInit = service.getTrustedDevicesCalls;

    service.trustedDevices = const [
      TrustedDevice(
        deviceId: 'aa67e88a629d12dd',
        displayName: 'x1',
        platform: 'linux',
        pairedAtMs: 1791038847434,
        lastSeenMs: 1791038847434,
        revoked: false,
      ),
    ];
    await handler!(const MethodCall('trustChanged'));
    await tester.pumpAndSettle();

    expect(
      service.getTrustedDevicesCalls,
      greaterThan(callsAfterInit),
      reason: 'native trustChanged must trigger a trust-store refresh',
    );
    expect(
      controller.trustedDevices.map((d) => d.deviceId),
      contains('aa67e88a629d12dd'),
    );
    await tester.tap(find.text('Devices'));
    await tester.pumpAndSettle();
    expect(find.text('Trusted Devices (1)'), findsOneWidget);

    await service.rawEvents.close();
  });
}
