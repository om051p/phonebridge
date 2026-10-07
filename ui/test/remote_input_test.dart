// Phase 7 Remote Input v0.1 tests (DEC-027).
//
// Tests:
// - Coordinate normalization with letterbox and pillarbox boundaries.
// - Margin rejection: clicks in letterbox/pillarbox margins return null / no events.
// - ScreenFrameView pointer down, move, up event dispatching.
// - ScreenSharingScreen navigation toolbar (Back, Home, Recents, Notifications).
// - Esc shortcut key triggering Back action.
// - Text commit dialog sending TextEvent via IPC.

import 'dart:async';
import 'dart:convert';
import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:fixnum/fixnum.dart';
import 'package:phonebridge_ui/generated/phonebridge/localipc/v1/local_ipc.pb.dart' as ipc;
import 'package:phonebridge_ui/generated/phonebridge/v1/phonebridge.pb.dart' as pb;
import 'package:phonebridge_ui/main.dart';
import 'package:phonebridge_ui/screens/screen_sharing_screen.dart';
import 'package:phonebridge_ui/services/frame_stream.dart';
import 'package:phonebridge_ui/services/linux_bridge_service.dart';
import 'package:phonebridge_ui/ui/screen_frame_view.dart';

import 'support/fake_ipc_client.dart';

/// A real 1x1 JPEG for rendering in tests.
final Uint8List tinyJpeg = base64Decode(
    '/9j/4AAQSkZJRgABAgAAAQABAAD//gAQTGF2YzYyLjExLjEwMAD/2wBDAAgGBgcGBwgICAgICAkJCQoKCgkJCQkKCgoKCgoMDAwKCgoKCgoKDAwMDA0ODQ0NDA0ODg8PDxISEREVFRUZGR//xABLAAEBAAAAAAAAAAAAAAAAAAAABwEBAAAAAAAAAAAAAAAAAAAAABABAAAAAAAAAAAAAAAAAAAAABEBAAAAAAAAAAAAAAAAAAAAAP/AABEIAAEAAQMBIgACEQADEQD/2gAMAwEAAhEDEQA/AL+AD//Z');

class _MockFrameProvider implements ProvidesFrameStream {
  final StreamController<ipc.StreamFramesResponse> ctrl =
      StreamController<ipc.StreamFramesResponse>.broadcast();

  @override
  FrameStream createFrameStream() => FrameStream(source: () => ctrl.stream);
}

void main() {
  group('ScreenFrameView coordinate normalization (letterbox/pillarbox)', () {
    test('center of symmetric container matches center of frame', () {
      const container = Size(1000, 1000);
      const frame = Size(720, 1600);

      final norm = ScreenFrameView.normalizeCoordinate(
        const Offset(500, 500),
        container,
        frame,
      );
      expect(norm, isNotNull);
      expect(norm!.dx, closeTo(0.5, 0.001));
      expect(norm.dy, closeTo(0.5, 0.001));
    });

    test('pillarbox bars (left and right margins) reject touches', () {
      // Wide container: frame aspect is 720/1600 = 0.45.
      // Container height 1000 -> fitted width is 450.
      // Left margin = (1000 - 450)/2 = 275. Active rect: [275, 725].
      const container = Size(1000, 1000);
      const frame = Size(720, 1600);

      // In left pillarbox bar
      final leftMargin = ScreenFrameView.normalizeCoordinate(
        const Offset(100, 500),
        container,
        frame,
      );
      expect(leftMargin, isNull);

      // In right pillarbox bar
      final rightMargin = ScreenFrameView.normalizeCoordinate(
        const Offset(800, 500),
        container,
        frame,
      );
      expect(rightMargin, isNull);

      // Just inside left edge of active video rect
      final leftEdge = ScreenFrameView.normalizeCoordinate(
        const Offset(276, 500),
        container,
        frame,
      );
      expect(leftEdge, isNotNull);
      expect(leftEdge!.dx, closeTo(0.002, 0.005));

      // Just inside right edge of active video rect
      final rightEdge = ScreenFrameView.normalizeCoordinate(
        const Offset(724, 500),
        container,
        frame,
      );
      expect(rightEdge, isNotNull);
      expect(rightEdge!.dx, closeTo(0.997, 0.005));
    });

    test('letterbox bars (top and bottom margins) reject touches', () {
      // Tall container: frame aspect is 1600/720 = 2.222 (landscape frame).
      // Container width 1000 -> fitted height is 450.
      // Top margin = (1000 - 450)/2 = 275. Active rect: top 275, bottom 725.
      const container = Size(1000, 1000);
      const frame = Size(1600, 720);

      // In top letterbox bar
      final topMargin = ScreenFrameView.normalizeCoordinate(
        const Offset(500, 100),
        container,
        frame,
      );
      expect(topMargin, isNull);

      // In bottom letterbox bar
      final bottomMargin = ScreenFrameView.normalizeCoordinate(
        const Offset(500, 900),
        container,
        frame,
      );
      expect(bottomMargin, isNull);

      // Center
      final center = ScreenFrameView.normalizeCoordinate(
        const Offset(500, 500),
        container,
        frame,
      );
      expect(center, isNotNull);
      expect(center!.dx, closeTo(0.5, 0.001));
      expect(center.dy, closeTo(0.5, 0.001));
    });

    test('invalid or empty dimensions return null', () {
      expect(
        ScreenFrameView.normalizeCoordinate(
          const Offset(10, 10),
          Size.zero,
          const Size(720, 1600),
        ),
        isNull,
      );
      expect(
        ScreenFrameView.normalizeCoordinate(
          const Offset(10, 10),
          const Size(100, 100),
          Size.zero,
        ),
        isNull,
      );
    });
  });

  group('ScreenFrameView pointer event dispatching', () {
    testWidgets('pointer interactions dispatch normalized touch frames',
        (tester) async {
      final provider = _MockFrameProvider();
      final receivedFrames = <pb.InputFrame>[];

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Center(
            child: SizedBox(
              width: 1000,
              height: 1000,
              child: ScreenFrameView(
                provider: provider,
                onInput: (frame) => receivedFrames.add(frame),
                fallback: const Text('fallback'),
              ),
            ),
          ),
        ),
      ));

      // Push 1 frame into stream
      provider.ctrl.add(ipc.StreamFramesResponse(
        frameId: Int64(1),
        chunkIndex: 0,
        chunkCount: 1,
        lastChunk: true,
        width: 1,
        height: 1,
        jpeg: tinyJpeg,
      ));
      await tester.runAsync(
          () => Future<void>.delayed(const Duration(milliseconds: 50)));
      await tester.pump();

      // Find the ScreenFrameView center (which is 500, 500)
      final center = tester.getCenter(find.byType(ScreenFrameView));

      // Down gesture
      final gesture = await tester.startGesture(center);
      expect(receivedFrames.length, 1);
      final downFrame = receivedFrames.last;
      expect(downFrame.hasTouch(), isTrue);
      expect(downFrame.touch.action, pb.TouchEvent_Action.ACTION_DOWN);
      expect(downFrame.touch.normalizedX, closeTo(0.5, 0.01));
      expect(downFrame.touch.normalizedY, closeTo(0.5, 0.01));

      // Move gesture
      await gesture.moveBy(const Offset(10, 10));
      expect(receivedFrames.length, 2);
      final moveFrame = receivedFrames.last;
      expect(moveFrame.hasTouch(), isTrue);
      expect(moveFrame.touch.action, pb.TouchEvent_Action.ACTION_MOVE);

      // Up gesture
      await gesture.up();
      expect(receivedFrames.length, 3);
      final upFrame = receivedFrames.last;
      expect(upFrame.hasTouch(), isTrue);
      expect(upFrame.touch.action, pb.TouchEvent_Action.ACTION_UP);

      await provider.ctrl.close();
    });
  });

  group('ScreenSharingScreen Remote Input Toolbar & Actions', () {
    Future<(PhoneBridgeController, FakeIpcClient)> mountScreen(
        WidgetTester tester) async {
      tester.view.physicalSize = const Size(1080, 2400);
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
      addTearDown(client.frames.close);

      await tester.pumpWidget(PhoneBridgeApp(
        home: ListenableBuilder(
          listenable: controller,
          builder: (context, _) => ScreenSharingScreen(controller: controller),
        ),
      ));
      await tester.pumpAndSettle();

      // Transition session to STREAMING
      client.events.add(ipc.StreamEventsResponse(
        sessionEvent: ipc.SessionEvent(
          sessionId: 'sess-input-test',
          state: ipc.SessionState.SESSION_STATE_STREAMING,
        ),
      ));
      await tester.pump();
      await tester.pump();

      // Push 1 frame to frame stream so ScreenFrameView renders the interactive mirror surface
      client.frames.add(ipc.StreamFramesResponse(
        frameId: Int64(1),
        chunkIndex: 0,
        chunkCount: 1,
        lastChunk: true,
        width: 1,
        height: 1,
        jpeg: tinyJpeg,
      ));
      await tester.runAsync(
          () => Future<void>.delayed(const Duration(milliseconds: 50)));
      await tester.pump();
      return (controller, client);
    }

    testWidgets('navigation toolbar buttons dispatch global actions',
        (tester) async {
      final (controller, client) = await mountScreen(tester);

      // Verify Back button
      final backButton = find.byTooltip('Back (Esc)');
      expect(backButton, findsOneWidget);
      await tester.tap(backButton);
      await tester.pump();

      expect(client.sendInputCalls, 1);
      final backReq = client.sentInputRequests.last;
      expect(backReq.sessionId, 'sess-input-test');
      expect(
        backReq.frame.action.type,
        pb.GlobalActionEvent_Type.TYPE_GLOBAL_ACTION_BACK,
      );

      // Verify Home button
      final homeButton = find.byTooltip('Home');
      expect(homeButton, findsOneWidget);
      await tester.tap(homeButton);
      await tester.pump();

      expect(client.sendInputCalls, 2);
      final homeReq = client.sentInputRequests.last;
      expect(
        homeReq.frame.action.type,
        pb.GlobalActionEvent_Type.TYPE_GLOBAL_ACTION_HOME,
      );

      // Verify Recents button
      final recentsButton = find.byTooltip('Recents');
      expect(recentsButton, findsOneWidget);
      await tester.tap(recentsButton);
      await tester.pump();

      expect(client.sendInputCalls, 3);
      final recentsReq = client.sentInputRequests.last;
      expect(
        recentsReq.frame.action.type,
        pb.GlobalActionEvent_Type.TYPE_GLOBAL_ACTION_RECENTS,
      );

      // Verify Notifications button
      final notifButton = find.byTooltip('Notifications');
      expect(notifButton, findsOneWidget);
      await tester.tap(notifButton);
      await tester.pump();

      expect(client.sendInputCalls, 4);
      final notifReq = client.sentInputRequests.last;
      expect(
        notifReq.frame.action.type,
        pb.GlobalActionEvent_Type.TYPE_GLOBAL_ACTION_NOTIFICATIONS,
      );

      controller.dispose();
    });

    testWidgets('Escape key triggers Back global action', (tester) async {
      final (controller, client) = await mountScreen(tester);

      await tester.sendKeyEvent(LogicalKeyboardKey.escape);
      await tester.pump();

      expect(client.sendInputCalls, 1);
      expect(
        client.sentInputRequests.last.frame.action.type,
        pb.GlobalActionEvent_Type.TYPE_GLOBAL_ACTION_BACK,
      );

      controller.dispose();
    });

    testWidgets('Text dialog commits text to device', (tester) async {
      final (controller, client) = await mountScreen(tester);

      final textButton = find.byTooltip('Send Text');
      expect(textButton, findsOneWidget);
      await tester.tap(textButton);
      await tester.pumpAndSettle();

      expect(find.text('Send Text to Device'), findsOneWidget);

      await tester.enterText(find.byType(TextField), 'Test remote input text');
      await tester.tap(find.text('Send'));
      await tester.pumpAndSettle();

      expect(client.sendInputCalls, 1);
      final req = client.sentInputRequests.last;
      expect(req.sessionId, 'sess-input-test');
      expect(req.frame.hasText(), isTrue);
      expect(req.frame.text.text, 'Test remote input text');

      controller.dispose();
    });

    testWidgets('Right click dispatches Back global action', (tester) async {
      final (controller, client) = await mountScreen(tester);

      // Find ScreenFrameView
      final frameView = find.byType(ScreenFrameView);
      expect(frameView, findsOneWidget);

      // Right click (secondary button)
      await tester.tap(frameView, buttons: kSecondaryMouseButton);
      await tester.pump();

      expect(client.sendInputCalls, 1);
      final req = client.sentInputRequests.last;
      expect(req.sessionId, 'sess-input-test');
      expect(req.frame.hasAction(), isTrue);
      expect(
        req.frame.action.type,
        pb.GlobalActionEvent_Type.TYPE_GLOBAL_ACTION_BACK,
      );

      controller.dispose();
    });

    testWidgets('Direct keyboard input sends TextEvent and KeyEvents', (tester) async {
      final (controller, client) = await mountScreen(tester);

      // Focus ScreenFrameView
      final frameView = find.byType(ScreenFrameView);
      await tester.tap(frameView);
      await tester.pump();

      // Clear calls from tap gesture
      client.sentInputRequests.clear();
      client.sendInputCalls = 0;

      // 1. Printable character typing
      await tester.sendKeyEvent(LogicalKeyboardKey.keyA, character: 'a');
      await tester.pump();

      expect(client.sendInputCalls, 1);
      var req = client.sentInputRequests.last;
      expect(req.frame.hasText(), isTrue);
      expect(req.frame.text.text, 'a');

      // 2. Backspace key
      await tester.sendKeyEvent(LogicalKeyboardKey.backspace);
      await tester.pump();

      expect(client.sendInputCalls, 2);
      req = client.sentInputRequests.last;
      expect(req.frame.hasKey(), isTrue);
      expect(req.frame.key.keyCode, 67); // KEYCODE_DEL

      // 3. Enter key
      await tester.sendKeyEvent(LogicalKeyboardKey.enter);
      await tester.pump();

      expect(client.sendInputCalls, 3);
      req = client.sentInputRequests.last;
      expect(req.frame.hasKey(), isTrue);
      expect(req.frame.key.keyCode, 66); // KEYCODE_ENTER

      // 4. Home key
      await tester.sendKeyEvent(LogicalKeyboardKey.home);
      await tester.pump();

      expect(client.sendInputCalls, 4);
      req = client.sentInputRequests.last;
      expect(req.frame.hasAction(), isTrue);
      expect(
        req.frame.action.type,
        pb.GlobalActionEvent_Type.TYPE_GLOBAL_ACTION_HOME,
      );

      controller.dispose();
    });

    testWidgets('Remote control toggle switches between ACTIVE and VIEW ONLY and gates inputs', (tester) async {
      final (controller, client) = await mountScreen(tester);

      expect(find.text('REMOTE CONTROL ACTIVE'), findsOneWidget);
      expect(controller.remoteControlEnabled, isTrue);

      // Toggle OFF via Switch
      final switchWidget = find.byType(Switch);
      expect(switchWidget, findsOneWidget);
      await tester.tap(switchWidget);
      await tester.pump();

      expect(controller.remoteControlEnabled, isFalse);
      expect(find.text('VIEW ONLY'), findsOneWidget);

      // Attempt pointer click on ScreenFrameView while disabled
      final frameView = find.byType(ScreenFrameView);
      await tester.tap(frameView);
      await tester.pump();

      // No input call should be dispatched
      expect(client.sendInputCalls, 0);

      // Toggle back ON
      await tester.tap(switchWidget);
      await tester.pump();

      expect(controller.remoteControlEnabled, isTrue);
      expect(find.text('REMOTE CONTROL ACTIVE'), findsOneWidget);

      controller.dispose();
    });
  });
}
