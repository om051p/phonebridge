// Phase 6 consolidation: the session control card that lived in the retired
// desktop session view now renders on the production Screen tab.
//
// What is pinned here:
//  * the requested-quality presets — including DEC-022's "device default"
//    zero tuple — reach the shared control path exactly as picked,
//  * start/stop run through the shared controller stack (one injected IPC
//    client, one daemon subscription) and the shared session model follows
//    the daemon's pushed rungs,
//  * a failed start is surfaced where the control is instead of vanishing.

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:phonebridge_ui/generated/phonebridge/localipc/v1/local_ipc.pb.dart'
    as ipc;
import 'package:phonebridge_ui/main.dart';
import 'package:phonebridge_ui/screens/screen_sharing_screen.dart';
import 'package:phonebridge_ui/services/linux_bridge_service.dart';

import 'support/fake_ipc_client.dart';

void main() {
  // One daemon-side transition, exactly as the single subscription delivers it.
  ipc.StreamEventsResponse event(ipc.SessionState state) {
    return ipc.StreamEventsResponse(
      sessionEvent: ipc.SessionEvent(sessionId: 'sess-1', state: state),
    );
  }

  // While a session is active the shared model arms its 1 Hz telemetry poll,
  // so after an active rung only zero-duration pumps are safe.
  Future<void> settle(WidgetTester tester) async {
    await tester.pump();
    await tester.pump();
  }

  late FakeIpcClient client;
  late PhoneBridgeController controller;

  Future<void> mountScreen(WidgetTester tester) async {
    tester.view.physicalSize = const Size(1080, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    // The app-shaped stack: one shared controller over the injected single
    // client. The screen builds none of it — that is the ownership under test.
    client = FakeIpcClient();
    controller = PhoneBridgeController(
      service: LinuxBridgeService(client: client),
    );
    controller.initialize();
    addTearDown(controller.dispose);
    addTearDown(client.events.close);

    await tester.pumpWidget(PhoneBridgeApp(
      home: ListenableBuilder(
        listenable: controller,
        // Mirrors the production shell, where the tab body lives inside the
        // app scaffold (which is what presents control feedback).
        builder: (context, _) => Scaffold(
          body: ScreenSharingScreen(controller: controller),
        ),
      ),
    ));
    await tester.pumpAndSettle();
  }

  testWidgets(
      'quality presets — including device default — reach the control path untouched',
      (tester) async {
    await mountScreen(tester);

    // Explicit target for this tab (Phase C): the Screen tab is a session
    // control surface, not device selection — the manual override stands in
    // for the Devices-tab selection in these tests.
    controller.setReceiverUrl('test-peer');

    // The DEC-022 device-default option migrated from the retired view: it
    // must request *no* tuple, so the phone reports what it actually applies.
    expect(find.text('Device Default (Phone Chooses)'), findsOneWidget);
    await tester.tap(find.text('Device Default (Phone Chooses)'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('MIRROR PHONE SCREEN'));
    await settle(tester);
    expect(client.startCalls, hasLength(1));
    // The explicit target travels untouched (no peer-auto guessing).
    expect(client.startCalls.single.target, 'test-peer');
    // The zero selection travelled as "unset", never as a zero-sized video.
    expect(client.startCalls.single.requested, isNull);

    // A concrete preset must travel as the exact tuple the user picked: a
    // dropped preset here is the silent capability loss this slice exists to
    // prevent.
    await tester.tap(find.text('1080p High Frame Rate'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('MIRROR PHONE SCREEN'));
    await settle(tester);

    expect(client.startCalls, hasLength(2));
    final requested = client.startCalls.last.requested;
    expect(requested, isNotNull);
    expect(requested!.width, 1080);
    expect(requested.height, 2400);
    expect(requested.fps, 60);
    expect(requested.bitrateKbps, 8000);
    expect(requested.codec, 'h264');

    // Still exactly one daemon subscription — the screen owns no IPC.
    expect(client.streamEventsCalls, 1);
  });

  testWidgets(
      'start and stop run through the shared controller and its session model',
      (tester) async {
    await mountScreen(tester);
    controller.setReceiverUrl('test-peer');

    // Start: the control reaches the shared seam exactly once, and until the
    // daemon pushes a rung the shared model has not moved — the screen holds
    // no optimistic copy of its own.
    await tester.tap(find.text('MIRROR PHONE SCREEN'));
    await settle(tester);
    expect(client.startCalls, hasLength(1));
    expect(controller.session.status.isActive, isFalse);

    client.events.add(event(ipc.SessionState.SESSION_STATE_CONNECTING));
    await settle(tester);
    expect(controller.session.status.isActive, isTrue);
    expect(find.text('STOP RECEIVER SESSION'), findsOneWidget);
    expect(find.textContaining('Session ID: sess-1'), findsOneWidget);

    // Stop: back through the shared controller to the same session id.
    await tester.tap(find.text('STOP RECEIVER SESSION'));
    await settle(tester);
    expect(client.stopCalls, hasLength(1));
    expect(client.stopCalls.single.sessionId, 'sess-1');

    // The daemon confirms the transition the same way it always does: pushed.
    client.events.add(event(ipc.SessionState.SESSION_STATE_DISCONNECTED));
    await settle(tester);
    expect(controller.session.status.isActive, isFalse);
    expect(find.text('MIRROR PHONE SCREEN'), findsOneWidget);

    // One subscription across the whole start/stop ladder.
    expect(client.streamEventsCalls, 1);
  });

  testWidgets('a failed start surfaces on the Screen tab instead of vanishing',
      (tester) async {
    await mountScreen(tester);
    controller.setReceiverUrl('test-peer');

    client.startError = StateError('peer refused the offer');

    await tester.tap(find.text('MIRROR PHONE SCREEN'));
    await settle(tester);

    expect(client.startCalls, hasLength(1));
    expect(controller.session.status.isActive, isFalse);
    // The shared controller carries the failure; the tab shows it where the
    // control is, rather than swallowing it the way no surface did before.
    expect(find.text('Failed to initiate screen capture request'), findsOneWidget);
    expect(find.byType(SnackBar), findsOneWidget);
    expect(client.streamEventsCalls, 1);

    // Fully dismiss it first: the auto-dismiss timer is armed only once the
    // entrance animation completes on a pumped frame, so dismissal needs
    // clock advance, not just frames (same pattern as diagnostics_ipc_test).
    await tester.pump(const Duration(seconds: 2));
    await tester.pump(const Duration(seconds: 2));
    await tester.pump(const Duration(seconds: 2));
    await tester.pumpAndSettle();
    expect(find.byType(SnackBar), findsNothing);
  });
}
