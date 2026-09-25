// Phase 6 consolidation: discovered-device and pairing behaviour migrated
// from the retired desktop session view onto the production Devices surface.
//
// What is pinned here:
//  * discovery loads at startup through the shared controller (no Scan tap),
//  * daemon-reported staleness and trust state are rendered, and pairing is
//    only offered for a fresh, untrusted result,
//  * pairing runs through the existing controller → service → client path —
//    the screen owns no IPC and opens no subscription,
//  * a rejected confirmation is surfaced instead of closing silently,
//  * revoking trust stays behind an explicit confirmation gate.

import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:phonebridge_ui/generated/phonebridge/localipc/v1/local_ipc.pb.dart'
    as ipc;
import 'package:phonebridge_ui/main.dart';
import 'package:phonebridge_ui/screens/devices_screen.dart';
import 'package:phonebridge_ui/services/linux_bridge_service.dart';

import 'support/fake_ipc_client.dart';

void main() {
  late FakeIpcClient client;
  late PhoneBridgeController controller;

  Future<void> mountDevices(WidgetTester tester) async {
    tester.view.physicalSize = const Size(1080, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    client = FakeIpcClient()
      ..devices = [
        ipc.DiscoveredDevice(
          id: 'poco-f5',
          name: 'POCO F5',
          model: '23049PCD8I',
          version: '1.0',
          address: '192.168.0.125',
          port: 7804,
        ),
        ipc.DiscoveredDevice(
          id: 'stale-phone',
          name: 'Old Pixel',
          model: 'Pixel 6',
          version: '1.0',
          address: '192.168.0.99',
          port: 7804,
          isStale: true,
        ),
        ipc.DiscoveredDevice(
          id: 'trusted-1',
          name: 'Trusted POCO',
          model: '23049PCD8I',
          version: '1.0',
          address: '192.168.0.50',
          port: 7804,
        ),
      ]
      ..trustedDevices = [
        ipc.TrustedDevice(
          deviceId: 'trusted-1',
          displayName: 'Trusted POCO',
          platform: 'android',
          pairedAtMs: Int64(1726000000000),
          lastSeenMs: Int64(1726000010000),
          revoked: false,
        ),
      ];

    controller = PhoneBridgeController(
      service: LinuxBridgeService(client: client),
    );
    // The app-level entry point the scaffold calls: it now includes the
    // startup discovery load that used to be private to the session view.
    controller.initialize();
    addTearDown(controller.dispose);
    addTearDown(client.events.close);

    await tester.pumpWidget(PhoneBridgeApp(
      home: ListenableBuilder(
        listenable: controller,
        builder: (context, _) =>
            Scaffold(body: DevicesScreen(controller: controller)),
      ),
    ));
    await tester.pumpAndSettle();
  }

  testWidgets(
      'discovery loads at startup and gates pairing on staleness and trust',
      (tester) async {
    await mountDevices(tester);

    // Startup load: the list populated without anyone tapping Scan.
    expect(client.listDevicesCalls, greaterThanOrEqualTo(1));
    expect(find.text('Discovered Devices (3)'), findsOneWidget);
    expect(find.text('Trusted Devices (1)'), findsOneWidget);

    // Badges: fresh+untrusted reads UNPAIRED, the stale browse entry reads
    // STALE (plus UNPAIRED — it is also untrusted), and the peer that is
    // both discovered and in the trust store reads PAIRED.
    expect(find.text('PAIRED'), findsOneWidget);
    expect(find.text('UNPAIRED'), findsNWidgets(2));
    expect(find.text('STALE'), findsOneWidget);

    // Pairing is offered only for the fresh, untrusted device.
    expect(find.widgetWithText(FilledButton, 'PAIR'), findsOneWidget);

    // The screen added no daemon subscription of its own.
    expect(client.streamEventsCalls, 1);
  });

  testWidgets('pairing runs through the existing controller → service → client path',
      (tester) async {
    await mountDevices(tester);

    await tester.tap(find.widgetWithText(FilledButton, 'PAIR'));
    await tester.pumpAndSettle();
    expect(client.startPairingCalls, 1);
    expect(find.text('Confirm Pairing Code'), findsOneWidget);
    expect(find.text('736500'), findsOneWidget);

    await tester.tap(find.text('CONFIRM MATCH'));
    await tester.pumpAndSettle();

    // The user's confirmation reached the daemon through the shared path,
    // with the device id and the choice intact.
    expect(client.completePairingCalls, 1);
    expect(client.lastConfirmedPairing!.deviceId, 'poco-f5');
    expect(client.lastConfirmedPairing!.userConfirmed, isTrue);
    expect(client.streamEventsCalls, 1);
  });

  testWidgets('a rejected confirmation is surfaced instead of closing silently',
      (tester) async {
    await mountDevices(tester);
    client.confirmPairingResponse =
        ipc.ConfirmPairingResponse(success: false, errorMessage: 'SAS mismatch');

    await tester.tap(find.widgetWithText(FilledButton, 'PAIR'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('CONFIRM MATCH'));
    await tester.pumpAndSettle();

    expect(client.completePairingCalls, 1);
    expect(
      find.text('Pairing was rejected or could not be confirmed'),
      findsOneWidget,
    );

    // Fully dismiss it first: the auto-dismiss timer is armed only once the
    // entrance animation completes on a pumped frame (same pattern as
    // diagnostics_ipc_test).
    await tester.pump(const Duration(seconds: 2));
    await tester.pump(const Duration(seconds: 2));
    await tester.pump(const Duration(seconds: 2));
    await tester.pumpAndSettle();
    expect(find.byType(SnackBar), findsNothing);
  });

  testWidgets('revoking trust requires confirmation and reaches the service path',
      (tester) async {
    await mountDevices(tester);

    // The trust-store tile renders after the discovered list, so the last
    // match is the one under test.
    await tester.tap(find.text('Trusted POCO').last);
    await tester.pumpAndSettle();
    expect(find.text('Revoke Trust'), findsOneWidget);

    await tester.tap(find.text('Revoke Trust'));
    await tester.pumpAndSettle();

    // Not yet: the confirmation gate stands between the tap and the daemon.
    expect(client.revokeDeviceCalls, isEmpty);
    expect(find.text('REVOKE'), findsOneWidget);

    await tester.tap(find.text('CANCEL'));
    await tester.pumpAndSettle();
    expect(client.revokeDeviceCalls, isEmpty);

    // Reopen and confirm for real.
    await tester.tap(find.text('Trusted POCO').last);
    await tester.pumpAndSettle();
    await tester.tap(find.text('Revoke Trust'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('REVOKE'));
    await tester.pumpAndSettle();

    expect(client.revokeDeviceCalls, ['trusted-1']);
    expect(client.streamEventsCalls, 1);
  });
}
