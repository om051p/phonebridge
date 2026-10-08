// Connection-architecture regression tests (Phases B–H).
//
// What is pinned here:
//  * the session peer comes from the daemon snapshot/start target — never
//    from trust ordering (C1/C2),
//  * one deviceId renders exactly one canonical row (H4/F),
//  * user connects always name their device — no peer-auto fallback (C),
//  * disconnect is a trust-preserving no-op when idle (D),
//  * switch stops A before starting B and never resurrects A (E),
//  * Forget deletes via RemoveDevice, Revoke preserves the row (H).

import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/controllers/phonebridge_controller.dart';
import 'package:phonebridge_ui/controllers/session_controller.dart';
import 'package:phonebridge_ui/generated/phonebridge/localipc/v1/local_ipc.pb.dart'
    as ipc;
import 'package:phonebridge_ui/models/device_presence.dart';
import 'package:phonebridge_ui/models/device_row.dart';
import 'package:phonebridge_ui/models/discovered_device.dart';
import 'package:phonebridge_ui/models/trusted_device.dart';
import 'package:phonebridge_ui/services/linux_bridge_service.dart';

import 'support/fake_ipc_client.dart';
import 'support/fake_session_backend.dart';

void main() {
  group('SessionController active peer (Phase B)', () {
    late FakeSessionBackend backend;
    late SessionController controller;

    setUp(() {
      backend = FakeSessionBackend();
      controller = SessionController(backend: backend);
    });

    tearDown(() async {
      controller.dispose();
      await backend.close();
    });

    test('is null when idle even before any session', () {
      expect(controller.activeDeviceId, isNull);
    });

    test('start() records the explicit target as the peer', () async {
      controller.initialize();
      final ok = await controller.start(target: 'phone-B');
      expect(ok, isTrue);
      expect(controller.activeDeviceId, equals('phone-B'));
    });

    test('snapshot adopts the authoritative daemon peer', () async {
      controller.initialize();
      await controller.start(target: 'phone-B');
      backend.snapshot = sessionSnapshot(
        sessionId: 'sess-started',
        deviceId: 'phone-B',
        state: ipc.SessionState.SESSION_STATE_STREAMING,
      );
      await controller.refresh();
      expect(controller.activeDeviceId, equals('phone-B'));
    });

    test('snapshot for another peer does not hijack a live session',
        () async {
      controller.initialize();
      await controller.start(target: 'phone-B');
      backend.snapshot = sessionSnapshot(
        sessionId: 'sess-other',
        deviceId: 'phone-C',
        state: ipc.SessionState.SESSION_STATE_STREAMING,
      );
      await controller.refresh();
      // Correlation guard: a different live session id is dropped.
      expect(controller.activeDeviceId, equals('phone-B'));
    });

    test('terminal event clears the peer', () async {
      controller.initialize();
      await controller.start(target: 'phone-B');
      await backend.push(sessionEvent(
        sessionId: 'sess-started',
        state: ipc.SessionState.SESSION_STATE_STOPPED,
      ));
      expect(controller.activeDeviceId, isNull);
    });

    test('empty target is rejected, never peer-auto', () async {
      controller.initialize();
      final ok = await controller.start(target: '   ');
      expect(ok, isFalse);
      expect(controller.activeDeviceId, isNull);
      expect(backend.startCalls, isEmpty);
    });

    test('inbound session from remote peer adopts authoritative peer deviceId',
        () async {
      backend.snapshot = sessionSnapshot(
        sessionId: 'sess-inbound',
        deviceId: 'phone-inbound',
        state: ipc.SessionState.SESSION_STATE_STREAMING,
      );
      controller.initialize();

      // Inbound event arrives over event stream without deviceId (DEC-022 wire format)
      await backend.push(sessionEvent(
        sessionId: 'sess-inbound',
        state: ipc.SessionState.SESSION_STATE_STREAMING,
      ));
      await controller.refresh();

      expect(controller.activeDeviceId, equals('phone-inbound'));
      expect(controller.status.sessionId, equals('sess-inbound'));
      expect(controller.status.state,
          equals(ipc.SessionState.SESSION_STATE_STREAMING));
    });

    test('initialize() hydrates pre-existing active session from daemon',
        () async {
      backend.snapshot = sessionSnapshot(
        sessionId: 'sess-preexisting',
        deviceId: 'phone-preexisting',
        state: ipc.SessionState.SESSION_STATE_STREAMING,
      );

      controller.initialize();
      // Allow async unawaited(refresh()) to run
      await Future<void>.delayed(Duration.zero);

      expect(controller.activeDeviceId, equals('phone-preexisting'));
      expect(controller.status.sessionId, equals('sess-preexisting'));
    });
  });

  group('DeviceRow merge (Phase F)', () {
    TrustedDevice trusted(String id, {bool revoked = false}) =>
        TrustedDevice(
          deviceId: id,
          displayName: 'Phone $id',
          platform: 'android',
          pairedAtMs: 1,
          lastSeenMs: 2,
          revoked: revoked,
        );

    DiscoveredDevice discovered(String id, {bool stale = false}) =>
        DiscoveredDevice(
          id: id,
          name: 'Phone $id',
          model: 'Model',
          version: '1',
          host: '192.168.0.2',
          port: 7804,
          isStale: stale,
        );

    test('same id in trust and discovery renders one row', () {
      final rows = mergeDeviceRows(
        trusted: [trusted('abc')],
        discovered: [discovered('abc')],
      );
      expect(rows.where((r) => r.deviceId == 'abc'), hasLength(1));
      final row = rows.singleWhere((r) => r.deviceId == 'abc');
      expect(row.trusted, isTrue);
      expect(row.discovered, isTrue);
      expect(row.group, equals(DeviceRowGroup.available));
    });

    test('session target groups as connected and alone', () {
      final rows = mergeDeviceRows(
        trusted: [trusted('a'), trusted('b')],
        discovered: [discovered('a'), discovered('b')],
        activeDeviceId: 'b',
      );
      expect(
        rows.where((r) => r.group == DeviceRowGroup.connected),
        hasLength(1),
      );
      expect(
        rows
            .singleWhere((r) => r.group == DeviceRowGroup.connected)
            .deviceId,
        equals('b'),
      );
      expect(rows.singleWhere((r) => r.deviceId == 'a').canConnect, isTrue);
      expect(
        rows.singleWhere((r) => r.deviceId == 'b').canConnect,
        isFalse,
      );
    });

    test('unpaired discovered rows group separately and cannot connect',
        () {
      final rows = mergeDeviceRows(
        trusted: const [],
        discovered: [discovered('x')],
      );
      expect(rows.single.group, equals(DeviceRowGroup.untrusted));
      expect(rows.single.canConnect, isFalse);
      expect(rows.single.trust, equals(DeviceTrust.untrusted));
    });

    test('trusted but undiscovered rows are unavailable, not connectable',
        () {
      final rows = mergeDeviceRows(
        trusted: [trusted('gone')],
        discovered: const [],
      );
      expect(rows.single.group, equals(DeviceRowGroup.unavailable));
      expect(rows.single.canConnect, isFalse);
    });

    test('revoked rows group separately even when discovered', () {
      final rows = mergeDeviceRows(
        trusted: [trusted('old', revoked: true)],
        discovered: [discovered('old')],
      );
      expect(rows.single.group, equals(DeviceRowGroup.revoked));
      expect(rows.single.canConnect, isFalse);
    });

    test('stale discovery is not connectable', () {
      final rows = mergeDeviceRows(
        trusted: [trusted('s')],
        discovered: [discovered('s', stale: true)],
      );
      expect(rows.single.isStale, isTrue);
      expect(rows.single.canConnect, isFalse);
    });
  });

  group('PhoneBridgeController connection lifecycle (Phases B-E, H)', () {
    late FakeIpcClient client;
    late PhoneBridgeController controller;

    setUp(() {
      client = FakeIpcClient();
      controller = PhoneBridgeController(
        service: LinuxBridgeService(client: client),
      );
    });

    tearDown(() {
      controller.dispose();
    });

    test('activePeer is null when idle despite trusted devices (C1)',
        () async {
      client.trustedDevices = [
        ipc.TrustedDevice(
          deviceId: 'phone-A',
          displayName: 'Phone A',
          platform: 'android',
        ),
        ipc.TrustedDevice(
          deviceId: 'phone-B',
          displayName: 'Phone B',
          platform: 'android',
        ),
      ];
      await controller.refreshAll();
      expect(controller.activeDeviceId, isNull);
      // The old bug returned the first trusted device here.
      expect(controller.activePeer, isNull);
    });

    test('explicit connect names its device and resolves the peer',
        () async {
      client.trustedDevices = [
        ipc.TrustedDevice(
          deviceId: 'phone-A',
          displayName: 'Phone A',
          platform: 'android',
        ),
      ];
      client.devices = [
        ipc.DiscoveredDevice(
          id: 'phone-A',
          name: 'Phone A',
          address: '192.168.0.2',
          port: 7804,
        ),
      ];
      client.snapshot = ipc.GetSessionStateResponse(
        sessionId: 'sess-1',
        deviceId: 'phone-A',
        state: ipc.SessionState.SESSION_STATE_STREAMING,
      );
      await controller.refreshAll();
      await controller.refreshDiscoveredDevices();

      final ok = await controller.startScreenSharing(
        targetDeviceId: 'phone-A',
      );
      expect(ok, isTrue);
      expect(client.startCalls.single.target, equals('phone-A'));
      // Production delivery: the daemon broadcasts the transition on the
      // event stream, then the snapshot carries the authoritative peer.
      controller.session.applyEvent(ipc.SessionEvent(
        sessionId: 'sess-1',
        state: ipc.SessionState.SESSION_STATE_STREAMING,
      ));
      await controller.session.refresh();
      expect(controller.activeDeviceId, equals('phone-A'));
      expect(controller.activePeer?.deviceId, equals('phone-A'));
      expect(
        controller.deviceRows
            .singleWhere((r) => r.deviceId == 'phone-A')
            .isActiveTarget,
        isTrue,
      );
    });

    test('connect without a target fails instead of peer-auto (Phase C)',
        () async {
      client.trustedDevices = [
        ipc.TrustedDevice(
          deviceId: 'phone-A',
          displayName: 'Phone A',
          platform: 'android',
        ),
      ];
      final ok = await controller.startScreenSharing();
      expect(ok, isFalse);
      expect(client.startCalls, isEmpty);
      expect(controller.activeDeviceId, isNull);
    });

    test('disconnect with no session is a no-op success (Phase D)',
        () async {
      expect(await controller.disconnectActiveDevice(), isTrue);
      expect(client.stopCalls, isEmpty);
    });

    test('switch stops A before starting B (Phase E)', () async {
      client.trustedDevices = [
        ipc.TrustedDevice(
          deviceId: 'phone-A',
          displayName: 'Phone A',
          platform: 'android',
        ),
        ipc.TrustedDevice(
          deviceId: 'phone-B',
          displayName: 'Phone B',
          platform: 'android',
        ),
      ];
      await controller.refreshAll();
      await controller.startScreenSharing(targetDeviceId: 'phone-A');
      expect(client.startCalls.single.target, equals('phone-A'));

      // Session now live for A (event + authoritative snapshot).
      controller.session.applyEvent(ipc.SessionEvent(
        sessionId: 'sess-1',
        state: ipc.SessionState.SESSION_STATE_STREAMING,
      ));
      client.snapshot = ipc.GetSessionStateResponse(
        sessionId: 'sess-1',
        deviceId: 'phone-A',
        state: ipc.SessionState.SESSION_STATE_STREAMING,
      );
      await controller.session.refresh();
      expect(controller.activeDeviceId, equals('phone-A'));

      // Daemon will report the stop; then the switch starts B.
      client.snapshot = ipc.GetSessionStateResponse(
        sessionId: 'sess-1',
        deviceId: 'phone-A',
        state: ipc.SessionState.SESSION_STATE_STOPPED,
      );
      final switched = await controller.switchToDevice(
        targetDeviceId: 'phone-B',
      );
      expect(switched, isTrue);
      expect(client.stopCalls, isNotEmpty);
      expect(client.startCalls.last.target, equals('phone-B'));
    });

    test('switch that fails on B ends truthfully idle, never revives A',
        () async {
      client.trustedDevices = [
        ipc.TrustedDevice(
          deviceId: 'phone-A',
          displayName: 'Phone A',
          platform: 'android',
        ),
        ipc.TrustedDevice(
          deviceId: 'phone-B',
          displayName: 'Phone B',
          platform: 'android',
        ),
      ];
      await controller.refreshAll();
      await controller.startScreenSharing(targetDeviceId: 'phone-A');
      controller.session.applyEvent(ipc.SessionEvent(
        sessionId: 'sess-1',
        state: ipc.SessionState.SESSION_STATE_STREAMING,
      ));
      client.snapshot = ipc.GetSessionStateResponse(
        sessionId: 'sess-1',
        deviceId: 'phone-A',
        state: ipc.SessionState.SESSION_STATE_STREAMING,
      );
      await controller.session.refresh();
      expect(controller.activeDeviceId, equals('phone-A'));

      // B refuses: the switch reports failure, A stays stopped, and no
      // ghost peer is reported.
      client.startError = StateError('peer refused the offer');
      client.snapshot = ipc.GetSessionStateResponse(
        sessionId: 'sess-1',
        deviceId: 'phone-A',
        state: ipc.SessionState.SESSION_STATE_STOPPED,
      );
      expect(
          await controller.switchToDevice(targetDeviceId: 'phone-B'), isFalse);
      expect(client.stopCalls, isNotEmpty);
      expect(controller.activeDeviceId, isNull);
      expect(controller.activePeer, isNull);
      // Retry with a healthy B recovers cleanly.
      client.startError = null;
      expect(
          await controller.switchToDevice(targetDeviceId: 'phone-B'), isTrue);
      expect(client.startCalls.last.target, equals('phone-B'));
    });

    test('removeDevice deletes via RemoveDevice, revoke preserves (Phase H)',
        () async {
      await controller.removeDevice('phone-X');
      expect(client.removeDeviceCalls, equals(['phone-X']));
      expect(client.revokeDeviceCalls, isEmpty);

      await controller.revokeDevice('phone-Y');
      expect(client.revokeDeviceCalls, equals(['phone-Y']));
    });

    test('inbound session from remote phone updates deviceRows with isActiveTarget',
        () async {
      controller.initialize();
      client.trustedDevices = [
        ipc.TrustedDevice(
          deviceId: 'phone-inbound',
          displayName: 'Inbound Phone',
          platform: 'android',
        ),
      ];
      await controller.refreshAll();
      expect(controller.deviceRows.single.isActiveTarget, isFalse);

      // Daemon now tracks inbound streaming session
      client.snapshot = ipc.GetSessionStateResponse(
        sessionId: 'sess-inbound-1',
        deviceId: 'phone-inbound',
        state: ipc.SessionState.SESSION_STATE_STREAMING,
      );

      // Event arrives over event stream
      client.events.add(ipc.StreamEventsResponse(
        sessionEvent: ipc.SessionEvent(
          sessionId: 'sess-inbound-1',
          state: ipc.SessionState.SESSION_STATE_STREAMING,
        ),
      ));

      await Future<void>.delayed(const Duration(milliseconds: 20));
      await controller.session.refresh();

      expect(controller.activeDeviceId, equals('phone-inbound'));
      final activeRow = controller.deviceRows.singleWhere(
        (r) => r.deviceId == 'phone-inbound',
      );
      expect(activeRow.isActiveTarget, isTrue);
      expect(activeRow.group, equals(DeviceRowGroup.connected));
    });

    test('clearAllRevokedDevices removes only revoked devices and preserves active trusted devices',
        () async {
      controller.initialize();
      client.trustedDevices = [
        ipc.TrustedDevice(
          deviceId: 'dev-revoked-1',
          displayName: 'Past Phone 1',
          platform: 'android',
          revoked: true,
        ),
        ipc.TrustedDevice(
          deviceId: 'dev-revoked-2',
          displayName: 'Past Phone 2',
          platform: 'android',
          revoked: true,
        ),
        ipc.TrustedDevice(
          deviceId: 'dev-trusted-active',
          displayName: 'Active Phone',
          platform: 'android',
          revoked: false,
        ),
      ];
      await controller.refreshAll();
      expect(controller.deviceRows.length, equals(3));

      final removedCount = await controller.clearAllRevokedDevices();
      expect(removedCount, equals(2));
      expect(client.removeDeviceCalls, equals(['dev-revoked-1', 'dev-revoked-2']));
    });

    test('isSessionConnecting and isConnectingTo reflect connecting session state',
        () async {
      controller.initialize();
      client.trustedDevices = [
        ipc.TrustedDevice(
          deviceId: 'phone-connecting-target',
          displayName: 'Connecting Phone',
          platform: 'android',
        ),
      ];
      await controller.refreshAll();

      expect(controller.isSessionConnecting, isFalse);
      expect(controller.isConnectingTo('phone-connecting-target'), isFalse);

      client.snapshot = ipc.GetSessionStateResponse(
        sessionId: 'sess-conn-1',
        deviceId: 'phone-connecting-target',
        state: ipc.SessionState.SESSION_STATE_CONNECTING,
      );
      await controller.session.refresh();

      expect(controller.isSessionConnecting, isTrue);
      expect(controller.connectingDeviceId, equals('phone-connecting-target'));
      expect(controller.isConnectingTo('phone-connecting-target'), isTrue);
      expect(controller.isConnectingTo('other-phone'), isFalse);
    });
  });
}
