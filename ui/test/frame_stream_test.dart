// Phase 6 Slice 3A: the production frame consumer's contract.
//
// Covers the four requirements the prototype validated by hand:
//  * reassembly of <=64 KiB chunks back into whole frames
//  * decode-on-arrival (decode immediately when idle, retain ONLY the newest
//    frame while a decode is in flight — the replacement for the 33 ms tick)
//  * bounded behaviour: incomplete frames abandoned, gaps counted, oversize
//    chunks rejected
//  * clean unsubscribe and no stale frame after stop/dispose
// plus the mirror surface (fallback until the first frame) and the screen's
// use of the dedicated frame stream next to the single event stream.

import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:phonebridge_ui/generated/phonebridge/localipc/v1/local_ipc.pb.dart'
    as ipc;
import 'package:phonebridge_ui/main.dart';
import 'package:phonebridge_ui/screens/screen_sharing_screen.dart';
import 'package:phonebridge_ui/services/frame_stream.dart';
import 'package:phonebridge_ui/services/linux_bridge_service.dart';
import 'package:phonebridge_ui/ui/screen_frame_view.dart';

import 'support/fake_ipc_client.dart';

/// A real 1x1 JPEG (ffmpeg-generated), so the dart:ui decode path is exercised
/// end to end without depending on files outside the test.
final Uint8List tinyJpeg = base64Decode(
    '/9j/4AAQSkZJRgABAgAAAQABAAD//gAQTGF2YzYyLjExLjEwMAD/2wBDAAgGBgcGBwgICAgICAkJCQoKCgkJCQkKCgoKCgoMDAwKCgoKCgoKDAwMDA0ODQ0NDA0ODg8PDxISEREVFRUZGR//xABLAAEBAAAAAAAAAAAAAAAAAAAABwEBAAAAAAAAAAAAAAAAAAAAABABAAAAAAAAAAAAAAAAAAAAABEBAAAAAAAAAAAAAAAAAAAAAP/AABEIAAEAAQMBIgACEQADEQD/2gAMAwEAAhEDEQA/AL+AD//Z');

ipc.StreamFramesResponse chunk({
  required int id,
  required int index,
  required int count,
  List<int> data = const [],
  bool last = false,
}) {
  return ipc.StreamFramesResponse(
    frameId: Int64(id),
    chunkIndex: index,
    chunkCount: count,
    lastChunk: last,
    width: 4,
    height: 2,
    jpeg: Uint8List.fromList(data),
  );
}

ipc.StreamFramesResponse wholeFrame(int id, Uint8List jpeg) => chunk(
      id: id,
      index: 0,
      count: 1,
      data: jpeg,
      last: true,
    );

void main() {
  group('FrameStream reassembly', () {
    test('reassembles a multi-chunk frame into one whole JPEG', () async {
      final ctrl = StreamController<ipc.StreamFramesResponse>();
      final decoded = <Uint8List>[];
      final fs = FrameStream(
        source: () => ctrl.stream,
        decoder: (bytes) async {
          decoded.add(bytes);
          return null; // decode result irrelevant here
        },
      );
      fs.start();

      ctrl.add(chunk(id: 1, index: 0, count: 3, data: [1, 2, 3]));
      ctrl.add(chunk(id: 1, index: 1, count: 3, data: [4, 5]));
      ctrl.add(chunk(id: 1, index: 2, count: 3, data: [6], last: true));
      await pumpEventQueue();

      expect(fs.stats.framesComplete, 1);
      expect(fs.stats.chunksReceived, 3);
      expect(decoded, hasLength(1));
      expect(decoded.single, [1, 2, 3, 4, 5, 6]);

      fs.dispose();
      await ctrl.close();
    });

    test('abandons an incomplete frame when a newer frame starts', () async {
      final ctrl = StreamController<ipc.StreamFramesResponse>();
      final decoded = <Uint8List>[];
      final fs = FrameStream(
        source: () => ctrl.stream,
        decoder: (bytes) async {
          decoded.add(bytes);
          return null;
        },
      );
      fs.start();

      ctrl.add(chunk(id: 1, index: 0, count: 2, data: [1]));
      ctrl.add(wholeFrame(2, Uint8List.fromList([9, 9])));
      await pumpEventQueue();

      expect(fs.stats.framesIncompleteDropped, 1);
      expect(fs.stats.framesComplete, 1);
      expect(decoded.single, [9, 9]);

      fs.dispose();
      await ctrl.close();
    });

    test('counts frame-id gaps as daemon-side drops', () async {
      final ctrl = StreamController<ipc.StreamFramesResponse>();
      final fs = FrameStream(
        source: () => ctrl.stream,
        decoder: (bytes) async => null,
      );
      fs.start();

      ctrl.add(wholeFrame(1, Uint8List.fromList([1])));
      ctrl.add(wholeFrame(5, Uint8List.fromList([2])));
      await pumpEventQueue();

      expect(fs.stats.frameIdGaps, 3);
      expect(fs.stats.framesComplete, 2);

      fs.dispose();
      await ctrl.close();
    });

    test('rejects an oversize chunk and recovers on the next frame', () async {
      final ctrl = StreamController<ipc.StreamFramesResponse>();
      final decoded = <Uint8List>[];
      final fs = FrameStream(
        source: () => ctrl.stream,
        decoder: (bytes) async {
          decoded.add(bytes);
          return null;
        },
      );
      fs.start();

      ctrl.add(wholeFrame(1, Uint8List(kMaxFrameChunkBytes + 1)));
      ctrl.add(wholeFrame(2, Uint8List.fromList([7])));
      await pumpEventQueue();

      expect(fs.stats.oversizeChunks, 1);
      expect(fs.stats.framesComplete, 1);
      expect(decoded.single, [7]);

      fs.dispose();
      await ctrl.close();
    });

    test('ignores a stream that joins mid-frame (chunkIndex != 0)', () async {
      final ctrl = StreamController<ipc.StreamFramesResponse>();
      final fs = FrameStream(
        source: () => ctrl.stream,
        decoder: (bytes) async => null,
      );
      fs.start();

      ctrl.add(chunk(id: 1, index: 1, count: 3, data: [4], last: true));
      ctrl.add(wholeFrame(2, Uint8List.fromList([7])));
      await pumpEventQueue();

      expect(fs.stats.framesComplete, 1);
      expect(fs.stats.decodeFailures, 1); // only frame 2 reached the decoder

      fs.dispose();
      await ctrl.close();
    });
  });

  group('decode-on-arrival', () {
    test('retains only the newest frame while a decode is in flight',
        () async {
      final ctrl = StreamController<ipc.StreamFramesResponse>();
      final gate = Completer<void>();
      final calls = <Uint8List>[];
      final fs = FrameStream(
        source: () => ctrl.stream,
        decoder: (bytes) async {
          calls.add(bytes);
          await gate.future;
          return null;
        },
      );
      fs.start();

      // Frame 1 starts a decode that stays busy; frames 2 and 3 arrive
      // while it is — newest wins, everything older is coalesced away.
      ctrl.add(wholeFrame(1, Uint8List.fromList([1])));
      await pumpEventQueue();
      ctrl.add(wholeFrame(2, Uint8List.fromList([2])));
      ctrl.add(wholeFrame(3, Uint8List.fromList([3])));
      await pumpEventQueue();

      expect(calls, hasLength(1), reason: 'no second decode while busy');
      gate.complete();
      await pumpEventQueue();

      expect(calls, hasLength(2));
      expect(calls[1], [3], reason: 'only the newest retained frame decodes');
      expect(fs.stats.framesCoalesced, 1);
      expect(fs.stats.framesComplete, 3);

      fs.dispose();
      await ctrl.close();
    });

    test('decodes a real JPEG end to end with the production decoder',
        () async {
      final ctrl = StreamController<ipc.StreamFramesResponse>();
      final fs = FrameStream(source: () => ctrl.stream);
      fs.start();

      ctrl.add(wholeFrame(1, tinyJpeg));
      await pumpEventQueue();

      expect(fs.stats.framesPainted, 1);
      expect(fs.stats.decodeFailures, 0);
      expect(fs.hasFrame, isTrue);
      expect(fs.width, 1);
      expect(fs.height, 1);
      expect(fs.aspectRatio, 1.0);

      fs.dispose();
      await ctrl.close();
    });

    test('counts undecodable frames without painting them', () async {
      final ctrl = StreamController<ipc.StreamFramesResponse>();
      final fs = FrameStream(
        source: () => ctrl.stream,
        decoder: (bytes) async => null,
      );
      fs.start();

      ctrl.add(wholeFrame(1, Uint8List.fromList([1, 2, 3])));
      await pumpEventQueue();

      expect(fs.stats.decodeFailures, 1);
      expect(fs.stats.framesPainted, 0);
      expect(fs.hasFrame, isFalse);

      fs.dispose();
      await ctrl.close();
    });
  });

  group('lifecycle', () {
    test('stop() unsubscribes, clears the image and is idempotent', () async {
      var cancelled = false;
      final ctrl = StreamController<ipc.StreamFramesResponse>(
        onCancel: () => cancelled = true,
      );
      final fs = FrameStream(source: () => ctrl.stream);
      fs.start();
      fs.start(); // idempotent: still one subscription

      ctrl.add(wholeFrame(1, tinyJpeg));
      await pumpEventQueue();
      expect(fs.hasFrame, isTrue);

      fs.stop();
      expect(cancelled, isTrue, reason: 'clean unsubscribe on stop');
      expect(fs.hasFrame, isFalse, reason: 'no stale frame after stop');
      fs.stop();

      fs.dispose(); // idempotent teardown
      fs.dispose();
      await ctrl.close();
    });

    test('a frame decoded after stop never becomes visible', () async {
      final ctrl = StreamController<ipc.StreamFramesResponse>();
      final gate = Completer<void>();
      final fs = FrameStream(
        source: () => ctrl.stream,
        decoder: (bytes) async {
          await gate.future;
          return null; // a late successful decode would surface here
        },
      );
      fs.start();

      ctrl.add(wholeFrame(1, tinyJpeg));
      await pumpEventQueue();
      fs.stop();
      gate.complete();
      await pumpEventQueue();

      expect(fs.hasFrame, isFalse);
      expect(fs.stats.framesPainted, 0);

      fs.dispose();
      await ctrl.close();
    });
  });

  group('ScreenFrameView', () {
    late _FakeFrameProvider provider;

    setUp(() => provider = _FakeFrameProvider());
    tearDown(() async {
      await provider.ctrl.close();
    });

    testWidgets('renders the fallback until the first frame arrives',
        (tester) async {
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: SizedBox(
            height: 320,
            child: ScreenFrameView(
              provider: provider,
              fallback: const Text('waiting for frames'),
            ),
          ),
        ),
      ));

      expect(find.text('waiting for frames'), findsOneWidget);
      expect(find.byType(RawImage), findsNothing);
    });

    testWidgets('paints the newest frame and unsubscribes on unmount',
        (tester) async {
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: SizedBox(
            height: 320,
            child: ScreenFrameView(
              provider: provider,
              fallback: const Text('waiting for frames'),
            ),
          ),
        ),
      ));

      provider.ctrl.add(wholeFrame(1, tinyJpeg));
      await tester.runAsync(
          () => Future<void>.delayed(const Duration(milliseconds: 50)));
      await tester.pump();

      expect(find.byType(RawImage), findsOneWidget);
      expect(find.text('waiting for frames'), findsNothing);

      // Unmount: clean unsubscribe (no listener left on the source).
      await tester.pumpWidget(const MaterialApp(home: Scaffold()));
      expect(provider.ctrl.hasListener, isFalse);
    });
  });

  group('Screen tab wiring', () {
    // Ends the live session so the controller's 1 Hz telemetry poll is
    // cancelled before the test's timer invariant is checked.
    Future<void> endSession(
        WidgetTester tester, FakeIpcClient client) async {
      client.events.add(ipc.StreamEventsResponse(
        sessionEvent: ipc.SessionEvent(
          sessionId: 'sess-1',
          state: ipc.SessionState.SESSION_STATE_STOPPED,
        ),
      ));
      await tester.pump();
      await tester.pump();
    }

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

      // Push the ladder exactly as the single subscription delivers it.
      client.events.add(ipc.StreamEventsResponse(
        sessionEvent: ipc.SessionEvent(
          sessionId: 'sess-1',
          state: ipc.SessionState.SESSION_STATE_STREAMING,
        ),
      ));
      await tester.pump();
      await tester.pump();
      return (controller, client);
    }

    testWidgets(
        'in-session Linux shows the mirror fallback and opens the dedicated '
        'frame stream alongside the single event stream', (tester) async {
      final (_, client) = await mountScreen(tester);

      // The mirror surface is present and honest before any frame arrives.
      expect(find.textContaining('waiting for live frames'), findsOneWidget);

      // One event subscription (one-consumer rule) + ONE separate frame
      // subscription: frames never ride the event stream, and the screen
      // opened neither itself — both belong to the app-owned services.
      expect(client.streamEventsCalls, 1);
      expect(client.streamFramesCalls, 1);
      expect(find.byType(RawImage), findsNothing);

      // Terminal rung: the poll must retire with the session (no stale timer).
      await endSession(tester, client);
      expect(find.textContaining('waiting for live frames'), findsNothing);
    });

    testWidgets('a live frame replaces the fallback on the Screen tab',
        (tester) async {
      final (_, client) = await mountScreen(tester);
      expect(find.textContaining('waiting for live frames'), findsOneWidget);

      client.frames.add(wholeFrame(1, tinyJpeg));
      await tester.runAsync(
          () => Future<void>.delayed(const Duration(milliseconds: 50)));
      await tester.pump();

      expect(find.byType(RawImage), findsOneWidget);
      expect(find.textContaining('waiting for live frames'), findsNothing);

      // Terminal rung: mirror surface retires with the session and the poll
      // stops — no stale frame and no pending timer.
      await endSession(tester, client);
      expect(find.byType(RawImage), findsNothing);
    });
  });
}

class _FakeFrameProvider implements ProvidesFrameStream {
  final StreamController<ipc.StreamFramesResponse> ctrl =
      StreamController<ipc.StreamFramesResponse>();

  @override
  FrameStream createFrameStream() => FrameStream(source: () => ctrl.stream);
}
