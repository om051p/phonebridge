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
    // Canonical rows: no active session, so no connected hero; the trusted
    // peer is available, the two untrusted browse entries pair-able.
    expect(find.text('Connected device'), findsOneWidget);
    expect(find.text('Your devices (1)'), findsOneWidget);
    expect(find.text('Available to pair (2)'), findsOneWidget);

    // Badges: the trusted row reads TRUSTED once (single canonical row, not
    // once per source list); both untrusted rows read UNPAIRED; the stale
    // browse entry reads STALE.
    expect(find.text('TRUSTED'), findsOneWidget);
    expect(find.text('UNPAIRED'), findsNWidgets(2));
    expect(find.text('STALE'), findsOneWidget);

    // Pairing is offered for the fresh untrusted device; the trusted row
    // connects (re-pair lives in its detail sheet).
    expect(find.widgetWithText(FilledButton, 'PAIR'), findsOneWidget);
    expect(find.widgetWithText(FilledButton, 'CONNECT'), findsOneWidget);

    // The screen added no daemon subscription of its own.
    expect(client.streamEventsCalls, 1);
  });

  testWidgets('re-pairing triggers SAS confirmation flow for locally-trusted device',
      (tester) async {
    await mountDevices(tester);

    // Re-pair lives in the trusted row's detail sheet now (canonical row:
    // CONNECT on the tile, RE-PAIR in details).
    await tester.tap(find.text('Trusted POCO'));
    await tester.pumpAndSettle();
    await tester.tap(find.widgetWithText(OutlinedButton, 'RE-PAIR'));
    await tester.pumpAndSettle();

    expect(client.startPairingCalls, 1);
    expect(find.text('Confirm Pairing Code'), findsOneWidget);
    expect(find.text('736500'), findsOneWidget);

    await tester.tap(find.text('CONFIRM MATCH'));
    await tester.pumpAndSettle();

    expect(client.completePairingCalls, 1);
    expect(client.lastConfirmedPairing!.deviceId, 'trusted-1');
    expect(client.lastConfirmedPairing!.userConfirmed, isTrue);
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

  testWidgets(
      'undiscovered trusted or revoked device displays NOT DISCOVERED and explanation',
      (tester) async {
    await mountDevices(tester);

    client.trustedDevices.add(
      ipc.TrustedDevice(
        deviceId: 'offline-phone',
        displayName: 'Offline Phone',
        platform: 'android',
        pairedAtMs: Int64(1726000000000),
        lastSeenMs: Int64(1726000010000),
        revoked: true,
      ),
    );
    await controller.refreshAll();
    await tester.pumpAndSettle();

    expect(find.text('Offline Phone'), findsOneWidget);
    expect(find.text('NOT DISCOVERED'), findsOneWidget);
    expect(
      find.textContaining('Not currently discovered'),
      findsOneWidget,
    );
  });

  // -------------------------------------------------------------------------
  // Phase 2 bidirectional pairing: the receiving side.
  // -------------------------------------------------------------------------

  void seedInbound() {
    client.inboundPairings = [
      ipc.InboundPairingRequest(
        pairingToken: 'tok-inbound-1',
        remoteName: 'Linux Desktop',
        remotePlatform: 'linux',
        sas: '482913',
        createdAtMs: Int64(1726000100000),
      ),
    ];
  }

  testWidgets(
      'inbound pairing request renders a card and the review dialog shows the SAS',
      (tester) async {
  await mountDevices(tester);
  seedInbound();

  await controller.refreshInboundPairings();
  await tester.pumpAndSettle();

  expect(find.text('Pairing Request · Linux Desktop'), findsOneWidget);
  expect(find.text('REVIEW'), findsOneWidget);

  await tester.tap(find.text('REVIEW'));
  await tester.pumpAndSettle();

  expect(find.text('Pairing Request'), findsOneWidget);
  expect(find.text('482913'), findsOneWidget);
  expect(find.text('ACCEPT'), findsOneWidget);
  expect(find.text('REJECT'), findsOneWidget);

  // The pending sweep timer is live while a request exists; dispose in-body
  // so the test binding's no-pending-timers invariant holds (addTearDown
  // dispose runs after it). Idempotent with the teardown.
  controller.dispose();
});

testWidgets('accepting an inbound request records the approval on the daemon path',
    (tester) async {
  await mountDevices(tester);
  seedInbound();

  await controller.refreshInboundPairings();
  await tester.pumpAndSettle();
  await tester.tap(find.text('REVIEW'));
  await tester.pumpAndSettle();
  await tester.tap(find.text('ACCEPT'));
  await tester.pumpAndSettle();

  expect(client.respondInboundPairingCallCount, 1);
  expect(client.respondInboundPairingCalls.last.pairingToken, 'tok-inbound-1');
  expect(client.respondInboundPairingCalls.last.approved, isTrue);
  // The dialog closes once the decision is recorded.
  expect(find.text('ACCEPT'), findsNothing);
  controller.dispose();
});

testWidgets('rejecting an inbound request records the refusal',
    (tester) async {
  await mountDevices(tester);
  seedInbound();

  await controller.refreshInboundPairings();
  await tester.pumpAndSettle();
  await tester.tap(find.text('REVIEW'));
  await tester.pumpAndSettle();
  await tester.tap(find.text('REJECT'));
  await tester.pumpAndSettle();

  expect(client.respondInboundPairingCallCount, 1);
  expect(client.respondInboundPairingCalls.last.approved, isFalse);
  controller.dispose();
});

testWidgets('an inbound request that leaves the snapshot closes the open dialog',
    (tester) async {
  await mountDevices(tester);
  seedInbound();

  await controller.refreshInboundPairings();
  await tester.pumpAndSettle();
  await tester.tap(find.text('REVIEW'));
  await tester.pumpAndSettle();
  expect(find.text('ACCEPT'), findsOneWidget);

  // The request expired/was withdrawn natively: the next refresh empties the
  // snapshot and the dialog must close itself instead of offering a decision
  // the native side can no longer record.
  client.inboundPairings = [];
  await controller.refreshInboundPairings();
  await tester.pumpAndSettle();

  expect(find.text('ACCEPT'), findsNothing);
});

testWidgets('a pairing event pushed by the daemon refreshes the inbound snapshot',
    (tester) async {
  await mountDevices(tester);

  client.events.add(ipc.StreamEventsResponse(
    pairingEvent: ipc.PairingEvent(
      type: ipc.PairingEventType.PAIRING_EVENT_TYPE_ARRIVED,
      pairingToken: 'tok-push',
    ),
  ));
  await tester.pumpAndSettle();

  expect(client.listInboundPairingsCalls, greaterThanOrEqualTo(1));
});

testWidgets('an outbound pairing awaiting SAS confirmation shows PAIRING, not just UNPAIRED',
    (tester) async {
  await mountDevices(tester);

  await tester.tap(find.widgetWithText(FilledButton, 'PAIR'));
  await tester.pumpAndSettle();

  // The SAS dialog is open; behind it the tile reports the in-flight state.
  expect(find.text('PAIRING'), findsOneWidget);
  expect(find.text('Confirm Pairing Code'), findsOneWidget);

  // Rejecting clears the in-flight state.
  await tester.tap(find.text('REJECT'));
  await tester.pumpAndSettle();
  expect(find.text('PAIRING'), findsNothing);
});

testWidgets('session in connecting state renders Connecting hero card and cancel button',
    (tester) async {
  await mountDevices(tester);

  client.trustedDevices = [
    ipc.TrustedDevice(
      deviceId: 'trusted-1',
      displayName: 'Trusted POCO',
      platform: 'android',
    ),
  ];
  client.snapshot = ipc.GetSessionStateResponse(
    sessionId: 'sess-connecting-1',
    deviceId: 'trusted-1',
    state: ipc.SessionState.SESSION_STATE_CONNECTING,
  );
  await controller.refreshAll();
  await controller.session.refresh();
  await tester.pump();

  expect(find.text('Connecting to Trusted POCO…'), findsOneWidget);
  expect(find.text('Negotiating WebRTC session…'), findsOneWidget);
  expect(find.text('CANCEL'), findsOneWidget);
  controller.dispose();
});

testWidgets('clear all revoked devices button opens dialog and triggers controller clear',
    (tester) async {
  await mountDevices(tester);

  client.trustedDevices = [
    ipc.TrustedDevice(
      deviceId: 'rev-1',
      displayName: 'Old Device 1',
      platform: 'android',
      revoked: true,
    ),
    ipc.TrustedDevice(
      deviceId: 'rev-2',
      displayName: 'Old Device 2',
      platform: 'android',
      revoked: true,
    ),
  ];
  await controller.refreshAll();
  await tester.pumpAndSettle();

  expect(find.text('CLEAR ALL'), findsOneWidget);
  await tester.tap(find.text('CLEAR ALL'));
  await tester.pumpAndSettle();

  expect(find.text('Clear All Revoked Devices?'), findsOneWidget);
  await tester.tap(find.widgetWithText(FilledButton, 'CLEAR ALL'));
  await tester.pumpAndSettle();

  expect(client.removeDeviceCalls, containsAll(['rev-1', 'rev-2']));
});

testWidgets('inline forget button on revoked tile opens confirmation and forgets device',
    (tester) async {
  await mountDevices(tester);

  client.trustedDevices = [
    ipc.TrustedDevice(
      deviceId: 'rev-single',
      displayName: 'Single Past Device',
      platform: 'android',
      revoked: true,
    ),
  ];
  await controller.refreshAll();
  await tester.pumpAndSettle();

  expect(find.byTooltip('Forget device'), findsOneWidget);
  await tester.tap(find.byTooltip('Forget device'));
  await tester.pumpAndSettle();

  expect(find.text('Forget Device?'), findsOneWidget);
  await tester.tap(find.widgetWithText(FilledButton, 'FORGET'));
  await tester.pumpAndSettle();

  expect(client.removeDeviceCalls, equals(['rev-single']));
});
}
