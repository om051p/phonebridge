// Phase 5 cross-screen state-consistency test.
//
// One shared PhoneBridgeController over one fake IPC stack, with the app's real
// surfaces mounted: the AppScaffold badge, Home's hero card and Screen
// Sharing's status card. The daemon-side session ladder is pushed through the
// single subscription and every surface must agree at each rung:
//
//   disconnected -> connecting -> connected -> streaming
//     -> reconnecting -> failed -> (clear) -> streaming (recovery)
//
// Transfer activity is asserted *in parallel* to the ladder: it must decorate
// the description without ever changing the connection phase.

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/generated/phonebridge/localipc/v1/local_ipc.pb.dart' as ipc;
import 'package:phonebridge_ui/main.dart';
import 'package:phonebridge_ui/models/link_status.dart';
import 'package:phonebridge_ui/models/transfer_item.dart';
import 'package:phonebridge_ui/services/linux_bridge_service.dart';

import 'support/fake_ipc_client.dart';

void main() {
  // The ladder, the one daemon would push for a real session. Failures carry
  // the typed reason code (prose `reason` is only supplementary detail).
  ipc.StreamEventsResponse event(
    ipc.SessionState state, {
    String reason = '',
    ipc.SessionReason reasonCode = ipc.SessionReason.SESSION_REASON_NONE,
  }) {
    return ipc.StreamEventsResponse(
      sessionEvent: ipc.SessionEvent(
        sessionId: 'sess-ladder',
        state: state,
        reason: reason,
        reasonCode: reasonCode,
      ),
    );
  }

  Future<void> settle(WidgetTester tester) async {
    await tester.pump();
    await tester.pump();
  }

  testWidgets('full session ladder renders consistently across badge, home and screen sharing',
      (tester) async {
    tester.view.physicalSize = const Size(1600, 1000);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final client = FakeIpcClient();
    final controller = PhoneBridgeController(
      service: LinuxBridgeService(client: client),
    );
    controller.initialize();
    addTearDown(controller.dispose);
    addTearDown(client.events.close);

    await tester.pumpWidget(PhoneBridgeApp(
      home: AppScaffold(controller: controller),
    ));
    await tester.pumpAndSettle();

    // Rung 0: no session -> Ready everywhere.
    expect(controller.linkStatus.phase, LinkPhase.disconnected);
    expect(find.text('Ready'), findsWidgets);

    // Rung 1: connecting.
    client.events.add(event(ipc.SessionState.SESSION_STATE_CONNECTING));
    await settle(tester);
    expect(controller.linkStatus.phase, LinkPhase.connecting);
    expect(find.text('Connecting'), findsWidgets);
    expect(controller.isSharing, isFalse);

    // Rung 2: connected (no media yet).
    client.events.add(event(ipc.SessionState.SESSION_STATE_CONNECTED));
    await settle(tester);
    expect(controller.linkStatus.phase, LinkPhase.connected);
    expect(find.text('Connected'), findsWidgets);
    expect(controller.isSharing, isFalse);

    // Rung 3: streaming — every surface flips together.
    client.events.add(event(ipc.SessionState.SESSION_STATE_STREAMING));
    await settle(tester);
    expect(controller.linkStatus.phase, LinkPhase.streaming);
    expect(controller.isSharing, isTrue);
    expect(find.text('Sharing'), findsWidgets);

    // Rung 4: transport lost, bounded reconnect — recovery, not failure.
    client.events.add(event(ipc.SessionState.SESSION_STATE_RECONNECTING));
    await settle(tester);
    expect(controller.linkStatus.phase, LinkPhase.reconnecting);
    expect(find.text('Reconnecting'), findsWidgets);
    expect(controller.linkStatus.severity, LinkSeverity.warning);
    expect(controller.isSharing, isFalse);

    // Rung 5: reconnect budget exhausted -> terminal failure, critical.
    client.events.add(
      event(
        ipc.SessionState.SESSION_STATE_FAILED,
        reason: 'reconnect budget exhausted',
        reasonCode: ipc.SessionReason.SESSION_REASON_RECONNECT_TIMEOUT,
      ),
    );
    await settle(tester);
    expect(controller.linkStatus.phase, LinkPhase.failed);
    expect(controller.linkStatus.severity, LinkSeverity.critical);
    expect(controller.linkStatus.reasonLabel, 'RECONNECT_TIMEOUT');
    expect(controller.isSharing, isFalse);

    // Recovery: a fresh session after the failure must not be poisoned by it —
    // clear + new transitions bring the app back to streaming.
    client.events.add(event(ipc.SessionState.SESSION_STATE_DISCONNECTED));
    await settle(tester);
    client.events.add(event(ipc.SessionState.SESSION_STATE_CONNECTING));
    await settle(tester);
    expect(controller.linkStatus.phase, LinkPhase.connecting);
    client.events.add(event(ipc.SessionState.SESSION_STATE_STREAMING));
    await settle(tester);
    expect(controller.linkStatus.phase, LinkPhase.streaming);
    expect(find.text('Sharing'), findsWidgets);

    // Dispose in-body: the test ends mid-stream, and the 1 Hz telemetry poll
    // must not dangle past it (tearDown would run too late).
    controller.dispose();
  });

  testWidgets('transfer activity decorates every phase without changing it',
      (tester) async {
    tester.view.physicalSize = const Size(1600, 1000);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final client = FakeIpcClient();
    final controller = PhoneBridgeController(
      service: LinuxBridgeService(client: client),
    );
    controller.initialize();
    addTearDown(controller.dispose);
    addTearDown(client.events.close);

    // Ladder rungs are driven straight through the model; the surfaces under
    // test here are the badge and the Home hero, both of which render
    // linkStatus.
    await tester.pumpWidget(PhoneBridgeApp(
      home: AppScaffold(controller: controller),
    ));
    await tester.pumpAndSettle();

    TransferItem push(ipc.TransferState state) {
      final t = TransferItem(
        transferId: 't1',
        filename: 'evidence.bin',
        state: state,
        sizeBytes: 1024,
        bytesTransferred:
            state == ipc.TransferState.TRANSFER_STATE_COMPLETE ? 1024 : 0,
      );
      // Same entry point the daemon's transfer events use.
      controller.transfers.applyEvent(t);
      return t;
    }

    // No session (Ready) + an active transfer: phase must stay Ready, with a
    // transfer line.
    push(ipc.TransferState.TRANSFER_STATE_ACTIVE);
    await settle(tester);
    expect(controller.linkStatus.phase, LinkPhase.disconnected);
    expect(controller.linkStatus.transferLine, contains('transfer in progress'));

    // Streaming + active transfer: still streaming, transfer is parallel info.
    client.events.add(event(ipc.SessionState.SESSION_STATE_STREAMING));
    await settle(tester);
    expect(controller.linkStatus.phase, LinkPhase.streaming);
    expect(controller.linkStatus.transferLine, isNotEmpty);

    // Reconnecting + active transfer: the transfer is interrupted when the
    // session dies, but the phase must stay "reconnecting" (recovery, not
    // failure) and the interrupted transfer must not reclassify it.
    client.events.add(event(ipc.SessionState.SESSION_STATE_RECONNECTING));
    await settle(tester);
    // Interruption surfaces as a terminal FAILED transfer (no separate
    // interrupted state exists; the session ladder above is the recovery view).
    push(ipc.TransferState.TRANSFER_STATE_FAILED);
    await settle(tester);
    expect(controller.linkStatus.phase, LinkPhase.reconnecting);
    expect(controller.linkStatus.severity, LinkSeverity.warning);

    // Failed session + failed transfer: the session decides the phase; the
    // transfer failure is the parallel line.
    client.events.add(event(ipc.SessionState.SESSION_STATE_FAILED));
    await settle(tester);
    push(ipc.TransferState.TRANSFER_STATE_FAILED);
    await settle(tester);
    expect(controller.linkStatus.phase, LinkPhase.failed);
    expect(controller.linkStatus.transferLine, contains('Last transfer failed'));

    // Completion does not downgrade a failed connection.
    push(ipc.TransferState.TRANSFER_STATE_COMPLETE);
    await settle(tester);
    expect(controller.linkStatus.phase, LinkPhase.failed);
  });
}
