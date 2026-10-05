// Phase 6, first UI slice: the mounted Screen tab's in-session surface.
//
// The Screen tab branches on the shared SessionController: disconnected it is
// the existing start experience, and while a session is live it renders the
// validated session banner, the streaming display banner and the telemetry —
// all reads of the one shared model over the app's single IPC subscription.
// The ladder below is pushed exactly the way the daemon pushes it.

import 'package:fixnum/fixnum.dart';
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
  ipc.StreamEventsResponse event(
    ipc.SessionState state, {
    String sessionId = 'sess-1',
    ipc.SessionReason reasonCode = ipc.SessionReason.SESSION_REASON_NONE,
    String reason = '',
  }) {
    return ipc.StreamEventsResponse(
      sessionEvent: ipc.SessionEvent(
        sessionId: sessionId,
        state: state,
        reason: reason,
        reasonCode: reasonCode,
      ),
    );
  }

  // Once a session is active the controller arms its 1 Hz telemetry poll, so
  // after the first rung only zero-duration pumps are safe (pumpAndSettle
  // would advance the clock into the timer forever).
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

    // The app-shaped stack: one shared controller over the single fake client.
    // The screen builds none of it — that is the ownership under test.
    client = FakeIpcClient();
    controller = PhoneBridgeController(
      service: LinuxBridgeService(client: client),
    );
    // Explicit target for the tab's start control (Phase C): the Screen tab
    // is a session control surface, not device selection — the manual
    // override stands in for the Devices-tab selection in these tests.
    controller.setReceiverUrl('test-peer');
    controller.initialize();
    addTearDown(controller.dispose);
    addTearDown(client.events.close);

    // Production wires rebuilds at the shell level (AppScaffold's
    // ListenableBuilder over the shared controller), with the screen itself a
    // pure read — mirror that here so pushed transitions repaint the tab.
    await tester.pumpWidget(PhoneBridgeApp(
      home: ListenableBuilder(
        listenable: controller,
        builder: (context, _) => ScreenSharingScreen(controller: controller),
      ),
    ));
    await tester.pumpAndSettle();
  }

  testWidgets('disconnected Screen tab keeps the existing start experience',
      (tester) async {
    await mountScreen(tester);

    expect(find.text('MIRROR PHONE SCREEN'), findsOneWidget);
    expect(find.text('720p HD (Balanced)'), findsOneWidget);
    // Human-readable device identity (name + platform + trust/connection),
    // never an internal "Target PC" label.
    expect(find.text('Paired device'), findsOneWidget);

    // Nothing in-session: no banner, no telemetry, no stop control.
    expect(find.textContaining('Session ID:'), findsNothing);
    expect(find.textContaining('Display Sink'), findsNothing);
    expect(find.text('Live Stream Telemetry'), findsNothing);
    expect(find.text('STOP RECEIVER SESSION'), findsNothing);

    // Exactly one daemon subscription — the service's own; the screen adds none.
    expect(client.streamEventsCalls, 1);
  });

  testWidgets(
      'start uses the shared controller and pushed rungs drive the mounted screen',
      (tester) async {
    await mountScreen(tester);

    // Start: the existing control reaches the shared session seam exactly once.
    await tester.tap(find.text('MIRROR PHONE SCREEN'));
    await settle(tester);
    expect(client.startCalls, hasLength(1));

    // The model only moves on daemon pushes — CONNECTING opens the in-session
    // surface: validated banner with the session id, stop control instead of
    // start (the old UI kept offering START while negotiating).
    client.events.add(event(ipc.SessionState.SESSION_STATE_CONNECTING));
    await settle(tester);
    expect(
      find.text('CONNECTING (NEGOTIATING PARAMETERS)'),
      findsWidgets, // banner label + the unified card's description line
    );
    expect(find.textContaining('Session ID: sess-1'), findsOneWidget);
    expect(find.text('STOP RECEIVER SESSION'), findsOneWidget);
    expect(find.text('MIRROR PHONE SCREEN'), findsNothing);
    expect(find.text('Live Stream Telemetry'), findsOneWidget);

    client.events.add(event(ipc.SessionState.SESSION_STATE_CONNECTED));
    await settle(tester);
    expect(find.text('CONNECTED (WEBRTC PEER)'), findsOneWidget);
    expect(find.textContaining('Session ID: sess-1'), findsOneWidget);

    // STREAMING: display banner appears with the media-flowing state.
    client.snapshot = ipc.GetSessionStateResponse(
      sessionId: 'sess-1',
      state: ipc.SessionState.SESSION_STATE_STREAMING,
      connectedDurationMs: Int64(5000),
      // The daemon reports the sink it actually chose and the receiver's
      // transport counters — both reach the screen through the one poll.
      sinkKind: ipc.SinkKind.SINK_KIND_DISPLAY,
      sinkActive: true,
      stats: ipc.StreamStats(
        packets: Int64(4242),
        bytesRtp: Int64(1000000),
        bytesH264: Int64(500000),
        accessUnits: Int64(150),
        keyframes: Int64(30),
        droppedAus: Int64(0),
        seqGaps: Int64(13),
        dupSeq: Int64(11),
        latePackets: Int64(17),
        tsBackward: Int64(4),
      ),
    );
    client.events.add(event(ipc.SessionState.SESSION_STATE_STREAMING));
    await settle(tester);
    expect(find.text('STREAMING (VIDEO ACTIVE)'), findsOneWidget);
    expect(find.text('Live Stream Telemetry'), findsOneWidget);
    expect(find.text('STOP RECEIVER SESSION'), findsOneWidget);
    // Before the first snapshot lands the sink is unreported: the banner
    // makes no claim instead of guessing ffplay.
    expect(find.textContaining('Display Sink'), findsNothing);

    // One clock second: the controller's single telemetry poll reads the
    // snapshot and the cards render *its* values — the screen holds no copy.
    await tester.pump(const Duration(seconds: 1));
    await tester.pump();
    // Real daemon-reported sink state: display pipe, attached.
    expect(find.text('Display Sink: Active (ffplay)'), findsOneWidget);
    // Transport counters rendered from the same snapshot.
    expect(find.text('RTP Seq Gaps'), findsOneWidget);
    expect(find.text('13'), findsOneWidget);
    expect(find.text('4242'), findsOneWidget); // RTP packets metric
    expect(find.text('150'), findsOneWidget); // access units metric
    expect(
      find.text('Duration: 00:05'),
      findsOneWidget, // banner duration from the shared snapshot
    );
    // The rendered value IS the shared model's value — one source, not two.
    expect(controller.session.streamStats?.packets, Int64(4242));
    expect(controller.session.status.durationMs, 5000);

    // RECONNECTING: presented as recovery; the banner updates in place.
    client.events.add(event(ipc.SessionState.SESSION_STATE_RECONNECTING));
    await settle(tester);
    expect(find.text('RECONNECTING (AUTOMATIC RECOVERY)'), findsOneWidget);
    expect(find.textContaining('Transport lost'), findsWidgets);

    // FAILED: terminal, so the in-session surface retires and the start
    // experience returns — now carrying the typed failure in the unified card.
    client.events.add(event(
      ipc.SessionState.SESSION_STATE_FAILED,
      reasonCode: ipc.SessionReason.SESSION_REASON_RECONNECT_TIMEOUT,
      reason: 'reconnect budget exhausted',
    ));
    await settle(tester);
    expect(find.text('SESSION FAILED'), findsOneWidget);
    // Stale receiver state cannot survive a terminal session: the sink
    // classification went with it, so no banner lingers.
    expect(find.textContaining('Display Sink'), findsNothing);
    expect(find.textContaining('Session ID:'), findsNothing);
    expect(find.text('Live Stream Telemetry'), findsNothing);
    expect(find.text('MIRROR PHONE SCREEN'), findsOneWidget); // retry affordance

    // Still one subscription across the whole ladder.
    expect(client.streamEventsCalls, 1);
  });

  testWidgets('stop returns the tab to the disconnected start experience',
      (tester) async {
    await mountScreen(tester);

    await tester.tap(find.text('MIRROR PHONE SCREEN'));
    await settle(tester);
    expect(client.startCalls, hasLength(1));

    // The stop path reads a snapshot (refreshAll after stopCapture), so the
    // daemon must be able to answer: give it a live classified sink first,
    // then the idle answer the real daemon gives after stop.
    client.snapshot = ipc.GetSessionStateResponse(
      sessionId: 'sess-1',
      state: ipc.SessionState.SESSION_STATE_STREAMING,
      sinkKind: ipc.SinkKind.SINK_KIND_DISPLAY,
      sinkActive: true,
    );
    client.events.add(event(ipc.SessionState.SESSION_STATE_CONNECTING));
    await settle(tester);
    await tester.pump(const Duration(seconds: 1));
    await tester.pump();
    expect(controller.session.status.hasReportedSink, isTrue);
    expect(find.text('STOP RECEIVER SESSION'), findsOneWidget);

    // The daemon answers the post-stop refresh with idle and no sink — the
    // engine cleared both on the terminal transition itself.
    client.snapshot = ipc.GetSessionStateResponse(
      sessionId: 'sess-1',
      state: ipc.SessionState.SESSION_STATE_DISCONNECTED,
    );
    await tester.tap(find.text('STOP RECEIVER SESSION'));
    await settle(tester);
    expect(client.stopCalls, hasLength(1));

    // The daemon confirms the transition the same way it always does: pushed.
    client.events.add(event(ipc.SessionState.SESSION_STATE_DISCONNECTED));
    await settle(tester);

    expect(find.text('MIRROR PHONE SCREEN'), findsOneWidget);
    expect(find.text('STOP RECEIVER SESSION'), findsNothing);
    expect(find.textContaining('Session ID:'), findsNothing);
    expect(find.textContaining('Display Sink'), findsNothing);
    expect(find.text('Live Stream Telemetry'), findsNothing);
    // The stop went through an idle copyWith/refresh: the sink
    // classification is cleared with the session it belonged to.
    expect(controller.session.status.hasReportedSink, isFalse);
    expect(find.text('READY'), findsOneWidget); // unified card back to Ready
    expect(client.streamEventsCalls, 1);
  });

  testWidgets('a headless daemon renders its real null sink, never ffplay',
      (tester) async {
    await mountScreen(tester);

    await tester.tap(find.text('MIRROR PHONE SCREEN'));
    await settle(tester);

    // The engine chose a null sink (this daemon has no display): the snapshot
    // says so, and the banner must report exactly that instead of ffplay.
    client.snapshot = ipc.GetSessionStateResponse(
      sessionId: 'sess-1',
      state: ipc.SessionState.SESSION_STATE_STREAMING,
      sinkKind: ipc.SinkKind.SINK_KIND_NULL,
      sinkActive: true,
    );
    client.events.add(event(ipc.SessionState.SESSION_STATE_STREAMING));
    await settle(tester);
    await tester.pump(const Duration(seconds: 1));
    await tester.pump();

    expect(find.text('Display Sink: None (Headless)'), findsOneWidget);
    expect(find.text('Display Sink: Active (ffplay)'), findsNothing);
    expect(client.streamEventsCalls, 1);

    // End in an active state with a live telemetry poll: dispose the adopted
    // controller inside the body (idempotent with the registered teardown).
    controller.dispose();
  });

  testWidgets(
      'the screen creates no second IPC client or subscription and never '
      'disposes the adopted stack', (tester) async {
    await mountScreen(tester);

    expect(client.streamEventsCalls, 1);

    // Even driven into an active session, the screen adds no subscription of
    // its own — everything it renders comes off the service's single stream.
    client.events.add(event(ipc.SessionState.SESSION_STATE_CONNECTING));
    await settle(tester);
    expect(client.streamEventsCalls, 1);
    expect(controller.session.status.isActive, isTrue);

    // Ownership: unmounting the screen never tears down the injected stack...
    await tester.pumpWidget(const SizedBox());
    await tester.pump();
    expect(client.shutdownCalls, 0);

    // ...and the explicit owner's dispose cascades exactly once (this also
    // cancels the active session's telemetry poll before the test ends).
    controller.dispose();
    expect(client.shutdownCalls, 1);
  });
}
