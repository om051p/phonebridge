// The Linux bridge is where session state actually lives: it receives every
// session event on its single IPC subscription and owns the only event
// subscription in the app. Phase 5 moved the session seam here, and this file
// covers the seam, the control path, and the pre-existing mapping it must not
// disturb.

import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/generated/phonebridge/localipc/v1/local_ipc.pb.dart' as ipc;
import 'package:phonebridge_ui/services/linux_bridge_service.dart';

import 'support/fake_ipc_client.dart';

void main() {
  late FakeIpcClient client;
  late LinuxBridgeService service;

  setUp(() {
    client = FakeIpcClient();
    service = LinuxBridgeService(client: client);
  });

  tearDown(() async {
    service.dispose();
    await client.events.close();
  });

  ipc.StreamEventsResponse sessionResponse(
    ipc.SessionState state, {
    String sessionId = 'sess-live',
    String reason = '',
  }) {
    return ipc.StreamEventsResponse(
      sessionEvent: ipc.SessionEvent(
        sessionId: sessionId,
        state: state,
        reason: reason,
      ),
    );
  }

  Future<void> settle() => Future<void>.delayed(Duration.zero);

  group('state seam', () {
    test('forwards session transitions and tracks the active session id',
        () async {
      expect(service.supportsSessions, isTrue);
      expect(service.activeSessionId, isEmpty);

      final received = <ipc.SessionEvent>[];
      final sub = service.sessionEventStream.listen(received.add);

      client.events.add(
        sessionResponse(ipc.SessionState.SESSION_STATE_CONNECTING),
      );
      await settle();
      client.events.add(
        sessionResponse(
          ipc.SessionState.SESSION_STATE_STREAMING,
          sessionId: 'sess-live',
        ),
      );
      await settle();

      expect(service.activeSessionId, 'sess-live');
      expect(received.map((e) => e.state).toList(), [
        ipc.SessionState.SESSION_STATE_CONNECTING,
        ipc.SessionState.SESSION_STATE_STREAMING,
      ]);
      await sub.cancel();
    });

    test('opens exactly one subscription to the daemon event stream', () async {
      // The bridge is the app's single consumer; a screen must not add another.
      expect(client.streamEventsCalls, 1);
    });

    test('keeps the pre-existing capture/error mapping intact', () async {
      // Phase 4 derives isCapturing and lastError from the same event; the new
      // seam must not change that.
      final rawEvents = <Map<dynamic, dynamic>>[];
      final sub = service.rawEventsStream.listen(rawEvents.add);

      client.events.add(
        sessionResponse(ipc.SessionState.SESSION_STATE_STREAMING),
      );
      await settle();
      expect(rawEvents.last['isCapturing'], isTrue);

      client.events.add(
        sessionResponse(
          ipc.SessionState.SESSION_STATE_FAILED,
          reason: 'reconnect budget exhausted',
        ),
      );
      await settle();
      expect(rawEvents.last['isCapturing'], isFalse);
      expect(rawEvents.last['lastError'], 'reconnect budget exhausted');

      await sub.cancel();
    });

    test('serves the authoritative snapshot through the seam', () async {
      client.snapshot = ipc.GetSessionStateResponse(
        sessionId: 'sess-live',
        state: ipc.SessionState.SESSION_STATE_CONNECTED,
        reconnectAttempts: 2,
      );

      final snapshot = await service.getSessionSnapshot('sess-live');

      expect(snapshot.state, ipc.SessionState.SESSION_STATE_CONNECTED);
      expect(snapshot.reconnectAttempts, 2);
      expect(client.snapshotIds, ['sess-live']);
    });

    test('stops forwarding and shuts the client down on dispose', () async {
      final received = <ipc.SessionEvent>[];
      service.sessionEventStream.listen(received.add);
      client.events.add(
        sessionResponse(ipc.SessionState.SESSION_STATE_CONNECTED),
      );
      await settle();
      expect(received, hasLength(1));

      service.dispose();
      expect(client.shutdownCalls, 1);

      client.events.add(
        sessionResponse(ipc.SessionState.SESSION_STATE_FAILED),
      );
      await settle();
      expect(received, hasLength(1));
    });
  });

  group('control path', () {
    test('startCapture requests the media tuple the caller selected', () async {
      // Regression guard for a silent capability loss: the app's capture path
      // used to drop the selection at this boundary, so the phone applied its own
      // defaults while the UI showed the user's preset.
      await service.startCapture(
        receiverUrl: 'peer-auto',
        width: 1080,
        height: 2400,
        fps: 60,
        bitrateKbps: 8000,
      );

      final call = client.startCalls.single;
      expect(call.target, 'peer-auto');
      final requested = call.requested;
      expect(requested, isNotNull);
      expect(requested!.width, 1080);
      expect(requested.height, 2400);
      expect(requested.fps, 60);
      expect(requested.bitrateKbps, 8000);
      expect(requested.codec, 'h264');
    });

    test('an all-zero tuple means "device default", not zero-sized video',
        () async {
      await service.startCapture(
        receiverUrl: 'peer-auto',
        width: 0,
        height: 0,
        fps: 0,
        bitrateKbps: 0,
      );

      expect(client.startCalls.single.target, 'peer-auto');
      expect(client.startCalls.single.requested, isNull);
    });

    test('startCapture requires an explicit target (no peer-auto fallback)',
        () async {
      // Phase C: user connects always name their device. An empty target
      // fails before any RPC instead of silently picking a peer.
      expect(await service.startCapture(), isFalse);
      expect(client.startCalls, isEmpty);
    });

    test('control keeps the tracked session id in sync', () async {
      await service.startSession(target: 'poco', requested: null);
      expect(service.activeSessionId, 'sess-1');

      await service.stopSession(sessionId: 'sess-1', reason: 'test stop');
      expect(service.activeSessionId, isEmpty);
      expect(client.stopCalls.single.reason, 'test stop');
    });

    test('stopCapture stops the tracked session once', () async {
      await service.startSession(target: 'poco', requested: null);

      expect(await service.stopCapture(), isTrue);
      expect(client.stopCalls, hasLength(1));
      expect(client.stopCalls.single.sessionId, 'sess-1');

      // Nothing live any more: a second stop must not reach the daemon.
      expect(await service.stopCapture(), isTrue);
      expect(client.stopCalls, hasLength(1));
    });

    test('a failed start is reported, not thrown', () async {
      client.startError = StateError('no peer');

      expect(await service.startCapture(), isFalse);
    });
  });
}
