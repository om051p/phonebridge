// A native trust mutation (pairing commit, revoke, remove) must refresh the
// controller's trusted-device list immediately: the Devices screen showed
// UNPAIRED with "Trusted Devices (0)" while both trust stores were already
// mutually trusted, and only an app restart repaired the view.
import 'package:flutter_test/flutter_test.dart';

import 'package:phonebridge_ui/main.dart';
import 'package:phonebridge_ui/models/trusted_device.dart';

import 'support/fake_bridge_service.dart';

void main() {
  test('trustChanged event refreshes trusted devices without restart',
      () async {
    final service = FakeBridgeService();
    final controller = PhoneBridgeController(service: service);
    controller.initialize();
    await Future<void>.delayed(Duration.zero);
    await Future<void>.delayed(Duration.zero);
    expect(controller.trustedDevices, isEmpty);
    final callsAfterInit = service.getTrustedDevicesCalls;
    expect(callsAfterInit, greaterThanOrEqualTo(1));

    // Background pairing commits on the native side; the bridge pushes
    // a trustChanged event through the raw event stream.
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
    service.rawEvents.add({'trustChanged': true});
    await Future<void>.delayed(const Duration(milliseconds: 200));

    expect(
      service.getTrustedDevicesCalls,
      greaterThan(callsAfterInit),
      reason: 'trustChanged must trigger a trust-store refresh',
    );
    expect(
      controller.trustedDevices.map((d) => d.deviceId),
      contains('aa67e88a629d12dd'),
    );

    controller.dispose();
    await service.rawEvents.close();
  });
}
