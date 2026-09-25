// The Local IPC diagnostics card (folded from the retired DashboardScreen in
// the Phase 6-preparation pass) rendered through the app-owned service:
// handshake facts, the PING/HEALTH actions, and the relayed-event counter all
// ride the single daemon subscription — and unmounting never disposes a stack
// the screen does not own.

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:phonebridge_ui/generated/phonebridge/localipc/v1/local_ipc.pb.dart'
    as ipc;
import 'package:phonebridge_ui/main.dart';
import 'package:phonebridge_ui/screens/diagnostics_screen.dart';
import 'package:phonebridge_ui/services/linux_bridge_service.dart';

import 'support/fake_ipc_client.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('IPC card facts, actions, and counter ride the app-owned service',
      (tester) async {
    // The card sits low in a ListView — a tall viewport keeps every row
    // built and tappable without scroll games.
    tester.view.physicalSize = const Size(1080, 4000);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final client = FakeIpcClient();
    final controller = PhoneBridgeController(
      service: LinuxBridgeService(client: client),
    );
    addTearDown(controller.dispose);
    addTearDown(client.events.close);

    await tester.pumpWidget(PhoneBridgeApp(
      home: DiagnosticsScreen(controller: controller),
    ));
    await tester.pumpAndSettle(); // handshake resolves → version row

    // Handshake facts, through the app-owned service (one call on mount).
    expect(find.text('Local Engine IPC (DEC-018)'), findsOneWidget);
    expect(find.text('Daemon Version'), findsOneWidget);
    expect(find.textContaining('0.1.0'), findsWidgets);
    // The socket path — a fact only this card renders — rides the same
    // service getter the retired session view's daemon card used.
    expect(find.text('Socket Path'), findsOneWidget);
    expect(find.text('/nonexistent.sock'), findsOneWidget);
    expect(client.handshakeCalls, 1);

    // The service's construction-time subscription is the only one.
    expect(client.streamEventsCalls, 1);

    // The counter starts empty — it counts relayed batches, not pushes.
    expect(find.text('0 messages'), findsOneWidget);

    // PING → service → client, with a user-visible result.
    await tester.tap(find.text('PING'));
    await tester.pump();
    await tester.pump();
    expect(find.textContaining('Ping OK'), findsOneWidget);
    expect(client.pingCalls, 1);

    // Fully dismiss it first: a snackbar's auto-dismiss timer is armed only
    // once its entrance animation completes, so dismissal needs clock
    // advance, not just frames — otherwise the next snackbar queues behind a
    // still-live one and never appears.
    await tester.pump(const Duration(seconds: 2));
    await tester.pump(const Duration(seconds: 2));
    await tester.pump(const Duration(seconds: 2));
    await tester.pumpAndSettle();
    expect(find.byType(SnackBar), findsNothing);

    // HEALTH → service → client, with a user-visible result.
    await tester.tap(find.text('HEALTH'));
    await tester.pump();
    await tester.pump();
    expect(find.textContaining('Daemon ready'), findsOneWidget);
    expect(client.healthCalls, 1);

    await tester.pump(const Duration(seconds: 2));
    await tester.pump(const Duration(seconds: 2));
    await tester.pump(const Duration(seconds: 2));
    await tester.pumpAndSettle();
    expect(find.byType(SnackBar), findsNothing);

    // The relayed-event counter rides the single subscription — pushed
    // batches flow client → service → card without a second streamEvents().
    client.events.add(ipc.StreamEventsResponse());
    client.events.add(ipc.StreamEventsResponse());
    await tester.pump();
    await tester.pump();
    expect(find.text('2 messages'), findsOneWidget);
    expect(client.streamEventsCalls, 1);

    // On-demand handshake (migrated from the retired session view's daemon
    // card): re-syncs version/generation facts without an app restart.
    await tester.tap(find.text('HANDSHAKE'));
    await tester.pump();
    await tester.pump();
    expect(client.handshakeCalls, 2);
    expect(client.streamEventsCalls, 1);

    // Ownership: unmount never disposes the injected stack...
    await tester.pumpWidget(const SizedBox());
    await tester.pump();
    expect(client.shutdownCalls, 0);

    // ...and the explicit owner's dispose cascades exactly once.
    controller.dispose();
    expect(client.shutdownCalls, 1);
  });
}
