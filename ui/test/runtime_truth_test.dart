// Runtime truth: media capabilities, service liveness, feature availability.
//
// Every test pins a mapping from an existing runtime owner to its honest
// presentation — never a value Flutter invents:
//   * capabilities: Go-retained DEC-022 advertisement (Linux snapshot) or the
//     encoder-read channel map (Android), gated by availabilityOf;
//   * liveness: the Android service singletons via getPermissionsStatus;
//   * features: pure functions over those models (feature_availability.dart).

import 'package:flutter/material.dart' hide ClipboardStatus;
import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/controllers/phonebridge_controller.dart';
import 'package:phonebridge_ui/controllers/session_controller.dart';
import 'package:phonebridge_ui/generated/phonebridge/localipc/v1/local_ipc.pb.dart'
    as ipc;
import 'package:phonebridge_ui/generated/phonebridge/v1/phonebridge.pb.dart'
    as pb;
import 'package:phonebridge_ui/models/clipboard_status.dart';
import 'package:phonebridge_ui/models/feature_availability.dart';
import 'package:phonebridge_ui/models/media_capabilities.dart';
import 'package:phonebridge_ui/models/session_status.dart';
import 'package:phonebridge_ui/services/android_bridge_service.dart';
import 'package:phonebridge_ui/services/linux_bridge_service.dart';
import 'package:phonebridge_ui/services/phonebridge_channel.dart';
import 'package:phonebridge_ui/ui/permission_onboarding_card.dart';

import 'support/fake_ipc_client.dart';
import 'support/fake_session_backend.dart';

pb.MediaCapabilities caps({
  List<String> codecs = const ['h264'],
  int maxWidth = 1080,
  int maxHeight = 2400,
  int maxFps = 30,
  bool supportsScreen = true,
}) =>
    pb.MediaCapabilities(
      codecs: codecs,
      maxWidth: maxWidth,
      maxHeight: maxHeight,
      maxFps: maxFps,
      supportsScreen: supportsScreen,
    );

ScreenPreset preset(int width, int height, int fps) => ScreenPreset(
      width: width,
      height: height,
      fps: fps,
      bitrateKbps: 2500,
      title: 'probe',
      subtitle: 'probe',
      guidance: 'Available',
    );

/// Channel fake that answers capability + permission maps deterministically.
class _CapsChannel extends PhoneBridgeChannel {
  Map<String, dynamic>? capsMap;
  int capsCalls = 0;
  Map<String, dynamic> permissions = const {
    'postNotifications': true,
    'notificationListener': true,
    'accessibility': true,
  };

  @override
  Future<Map<String, dynamic>?> getMediaCapabilities() async {
    capsCalls++;
    return capsMap;
  }

  @override
  Future<Map<String, dynamic>> getPermissionsStatus() async => permissions;

  @override
  Stream<Map<dynamic, dynamic>> get rawEventsStream => const Stream.empty();
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('media capability gating', () {
    test('720p30 and 1080p30 pass a 1080p30 h264 advertisement', () {
      final c = caps();
      expect(availabilityOf(preset(720, 1600, 30), c),
          CapabilityAvailability.supported);
      expect(availabilityOf(preset(1080, 2400, 30), c),
          CapabilityAvailability.supported);
      expect(availabilityReason(preset(1080, 2400, 30), c), isEmpty);
    });

    test('1080p presets fail honestly against a 720p encoder', () {
      final c = caps(maxWidth: 720, maxHeight: 1600);
      expect(availabilityOf(preset(1080, 2400, 30), c),
          CapabilityAvailability.unsupported);
      expect(
        availabilityReason(preset(1080, 2400, 30), c),
        contains('720px wide'),
      );
      // 720p itself still passes.
      expect(availabilityOf(preset(720, 1600, 30), c),
          CapabilityAvailability.supported);
    });

    test('frame rate is advisory: 60fps passes a 30fps-max advertisement', () {
      // DEC-020: the platform ignores the requested rate, so the gate must
      // not refuse 60fps where the geometry fits — the applied rate is what
      // the phone reports back.
      final c = caps(maxFps: 30);
      expect(availabilityOf(preset(1080, 2400, 60), c),
          CapabilityAvailability.supported);
    });

    test('absurd frame rates stay refused by the contract maximum', () {
      expect(
        availabilityOf(preset(1080, 2400, 300), caps()),
        CapabilityAvailability.unsupported,
      );
      expect(
        availabilityReason(preset(1080, 2400, 300), caps()),
        contains('240'),
      );
    });

    test('no screen support refuses every preset including device default', () {
      final c = caps(supportsScreen: false);
      for (final p in kScreenPresets) {
        expect(availabilityOf(p, c), CapabilityAvailability.unsupported,
            reason: p.title);
      }
    });

    test('device default always passes a supporting advertisement', () {
      expect(availabilityOf(kScreenPresets.first, caps()),
          CapabilityAvailability.supported);
      expect(kScreenPresets.first.isDeviceDefault, isTrue);
    });

    test('no advertisement is unknown, never a guess', () {
      for (final p in kScreenPresets) {
        expect(availabilityOf(p, null), CapabilityAvailability.unknown,
            reason: p.title);
      }
      expect(availabilityReason(preset(720, 1600, 30), null), isNotEmpty);
    });

    test('undisclosed bounds pass: the peer stated no limit', () {
      final c = caps(maxWidth: 0, maxHeight: 0, maxFps: 0);
      expect(availabilityOf(preset(1080, 2400, 60), c),
          CapabilityAvailability.supported);
    });

    test('non-h264 encoder refuses the h264 presets honestly', () {
      final c = caps(codecs: ['av1']);
      expect(availabilityOf(preset(720, 1600, 30), c),
          CapabilityAvailability.unsupported);
      expect(availabilityReason(preset(720, 1600, 30), c), contains('H.264'));
    });

    test('advertisement equality is by value, not identity', () {
      expect(mediaCapabilitiesEqual(null, null), isTrue);
      expect(mediaCapabilitiesEqual(caps(), null), isFalse);
      expect(mediaCapabilitiesEqual(null, caps()), isFalse);
      expect(mediaCapabilitiesEqual(caps(), caps()), isTrue);
      expect(mediaCapabilitiesEqual(caps(maxWidth: 720), caps()), isFalse);
    });
  });

  group('requested vs actual stays distinct', () {
    test('a downgrade reports both tuples, never the request as fact', () {
      final status = SessionStatus(
        state: ipc.SessionState.SESSION_STATE_STREAMING,
        requested: pb.MediaParams(width: 1080, height: 2400, fps: 60),
        actual: pb.MediaParams(width: 720, height: 1600, fps: 30),
      );
      expect(status.isDowngraded, isTrue);
      expect(status.requestedSummary, contains('1080x2400'));
      expect(status.actualSummary, contains('720x1600'));
      expect(status.negotiationSummary(), contains('different parameters'));
    });

    test('an unreported tuple is unknown, not echoed from the request', () {
      final status = SessionStatus(
        state: ipc.SessionState.SESSION_STATE_CONNECTED,
        requested: pb.MediaParams(width: 1080, height: 2400, fps: 60),
      );
      expect(status.isDowngraded, isFalse);
      expect(status.negotiationSummary(), contains('not reported'));
    });

    test('snapshot capabilities flow into the shared status', () {
      final snap = ipc.GetSessionStateResponse(
        sessionId: 's1',
        state: ipc.SessionState.SESSION_STATE_STREAMING,
        capabilities: caps(),
      );
      final status = SessionStatus.fromSnapshot(snap);
      expect(status.hasCapabilities, isTrue);
      expect(status.capabilities!.maxWidth, 1080);

      final bare = ipc.GetSessionStateResponse(
        sessionId: 's1',
        state: ipc.SessionState.SESSION_STATE_STREAMING,
      );
      expect(SessionStatus.fromSnapshot(bare).hasCapabilities, isFalse);
    });

    test('events preserve capabilities; terminal copyWith keeps them', () {
      const live = SessionStatus(
        state: ipc.SessionState.SESSION_STATE_STREAMING,
      );
      final withCaps = live.copyWith(capabilities: caps());
      final stopped = SessionStatus.fromEvent(
        ipc.SessionEvent(
          sessionId: 's1',
          state: ipc.SessionState.SESSION_STATE_STOPPED,
        ),
        previous: withCaps,
      );
      // Device facts outlive the session; the sink must not.
      expect(stopped.hasCapabilities, isTrue);
      expect(stopped.hasReportedSink, isFalse);
    });
  });

  group('android service liveness', () {
    test('denied permissions offer setup and no runtime lines', () async {
      final channel = _CapsChannel()
        ..permissions = const {
          'postNotifications': false,
          'notificationListener': false,
          'accessibility': false,
        };
      final controller = PhoneBridgeController(
        service: AndroidBridgeService(channel: channel),
      );
      addTearDown(controller.dispose);
      await controller.refreshPermissions();

      expect(controller.hasMissingPermissions, isTrue);
      expect(controller.notificationServiceActive, isNull);
      expect(controller.accessibilityServiceActive, isNull);
      expect(controller.foregroundServiceActive, isNull);
    });

    test('granted but stopped services read as not running, not active',
        () async {
      final channel = _CapsChannel()
        ..permissions = const {
          'postNotifications': true,
          'notificationListener': true,
          'accessibility': true,
          'notificationServiceActive': false,
          'accessibilityServiceActive': false,
          'foregroundServiceActive': false,
        };
      final controller = PhoneBridgeController(
        service: AndroidBridgeService(channel: channel),
      );
      addTearDown(controller.dispose);
      await controller.refreshPermissions();

      expect(controller.hasMissingPermissions, isFalse);
      expect(controller.notificationServiceActive, isFalse);
      expect(controller.accessibilityServiceActive, isFalse);
      expect(controller.foregroundServiceActive, isFalse);

      final notif = notificationAvailability(
          granted: true, serviceActive: controller.notificationServiceActive);
      expect(notif.state, FeatureState.needsSetup);
      expect(notif.label, 'Granted');
      expect(notif.detail, contains('not running'));
    });

    test('bound services read as active', () async {
      final channel = _CapsChannel()
        ..permissions = const {
          'postNotifications': true,
          'notificationListener': true,
          'accessibility': true,
          'notificationServiceActive': true,
          'accessibilityServiceActive': true,
          'foregroundServiceActive': true,
        };
      final controller = PhoneBridgeController(
        service: AndroidBridgeService(channel: channel),
      );
      addTearDown(controller.dispose);
      await controller.refreshPermissions();

      expect(
          notificationAvailability(granted: true, serviceActive: true).state,
          FeatureState.active);
      expect(
          remoteInputAvailability(granted: true, serviceActive: true).state,
          FeatureState.active);
    });

    test('unknown liveness shows no runtime claim', () {
      final unknown =
          notificationAvailability(granted: true, serviceActive: null);
      expect(unknown.state, FeatureState.granted);
      final input =
          remoteInputAvailability(granted: true, serviceActive: null);
      expect(input.state, FeatureState.granted);
    });

    test('denied access needs setup; restricted input is restricted', () {
      expect(
          notificationAvailability(granted: false, serviceActive: null).state,
          FeatureState.needsSetup);
      expect(
          remoteInputAvailability(
                  granted: true, serviceActive: null, restricted: true)
              .state,
          FeatureState.restricted);
      expect(
          remoteInputAvailability(granted: false, serviceActive: null).state,
          FeatureState.needsSetup);
    });

    testWidgets('card shows runtime lines only when liveness is known',
        (tester) async {
      final channel = _CapsChannel()
        ..permissions = const {
          'postNotifications': true,
          'notificationListener': true,
          'accessibility': true,
          'notificationServiceActive': true,
          'accessibilityServiceActive': false,
          'foregroundServiceActive': true,
        };
      final controller = PhoneBridgeController(
        service: AndroidBridgeService(channel: channel),
      );
      addTearDown(controller.dispose);
      await controller.refreshPermissions();

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(body: PermissionOnboardingCard(controller: controller)),
      ));
      await tester.pumpAndSettle();

      expect(find.text('Granted'), findsNWidgets(3));
      expect(find.textContaining('listener service is bound'), findsOneWidget);
      expect(find.textContaining('not running'), findsOneWidget);
      expect(find.text('Foreground service running'), findsOneWidget);
    });

    testWidgets('card shows no runtime lines without liveness reports',
        (tester) async {
      final channel = _CapsChannel(); // no runtime keys: old build
      final controller = PhoneBridgeController(
        service: AndroidBridgeService(channel: channel),
      );
      addTearDown(controller.dispose);
      await controller.refreshPermissions();

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(body: PermissionOnboardingCard(controller: controller)),
      ));
      await tester.pumpAndSettle();

      expect(find.text('Granted'), findsNWidgets(3));
      expect(find.textContaining('Service active'), findsNothing);
      expect(find.textContaining('not running'), findsNothing);
    });
  });

  group('capability channel parsing', () {
    test('full map parses; empty and missing parse as unknown', () async {
      final channel = _CapsChannel()
        ..capsMap = {
          'codecs': ['h264'],
          'maxWidth': 1080,
          'maxHeight': 2400,
          'maxFps': 30,
          'supportsScreen': true,
        };
      final service = AndroidBridgeService(channel: channel);
      final got = await service.getMediaCapabilities();
      expect(got, isNotNull);
      expect(got!.maxWidth, 1080);
      expect(got.supportsScreen, isTrue);

      channel.capsMap = {};
      expect(await service.getMediaCapabilities(), isNull);

      channel.capsMap = null;
      expect(await service.getMediaCapabilities(), isNull);
      expect(channel.capsCalls, 3);
    });

    test('controller refreshes android caps and exposes them', () async {
      final channel = _CapsChannel()
        ..capsMap = {
          'codecs': ['h264'],
          'maxWidth': 720,
          'maxHeight': 1600,
          'maxFps': 30,
          'supportsScreen': true,
        };
      final controller = PhoneBridgeController(
        service: AndroidBridgeService(channel: channel),
      );
      addTearDown(controller.dispose);

      expect(controller.effectiveCapabilities, isNull);
      await controller.refreshCapabilities();
      expect(controller.effectiveCapabilities, isNotNull);
      expect(controller.effectiveCapabilities!.maxWidth, 720);
    });

    test('linux effective capabilities ride the session snapshot', () async {
      final client = FakeIpcClient();
      final controller = PhoneBridgeController(
        service: LinuxBridgeService(client: client),
      );
      addTearDown(controller.dispose);
      addTearDown(client.events.close);

      expect(controller.effectiveCapabilities, isNull);
      // Drive the session model directly (no stream timing involved).
      controller.session.applyEvent(ipc.SessionEvent(
        sessionId: 'sess-1',
        state: ipc.SessionState.SESSION_STATE_CONNECTED,
      ));
      client.snapshot = ipc.GetSessionStateResponse(
        sessionId: 'sess-1',
        state: ipc.SessionState.SESSION_STATE_CONNECTED,
        capabilities: caps(maxWidth: 720),
      );
      await controller.session.refresh();
      expect(controller.effectiveCapabilities?.maxWidth, 720);
    });

    test('session change notifications fire when caps arrive', () async {
      final backend = FakeSessionBackend();
      final sc = SessionController(backend: backend);
      addTearDown(sc.dispose);
      sc.applyEvent(ipc.SessionEvent(
        sessionId: 's1',
        state: ipc.SessionState.SESSION_STATE_CONNECTED,
      ));
      backend.snapshot = ipc.GetSessionStateResponse(
        sessionId: 's1',
        state: ipc.SessionState.SESSION_STATE_CONNECTED,
      );
      await sc.refresh();
      var notified = 0;
      sc.addListener(() => notified++);
      backend.snapshot = ipc.GetSessionStateResponse(
        sessionId: 's1',
        state: ipc.SessionState.SESSION_STATE_CONNECTED,
        capabilities: caps(),
      );
      await sc.refresh();
      expect(notified, greaterThan(0));
    });
  });

  group('feature mapping', () {
    test('clipboard states map to feature vocabulary', () {
      ClipboardStatus s(ClipboardSyncState st, {bool enabled = true}) =>
          ClipboardStatus(
              state: st, imeSelected: false, maxPayloadSize: 1, isEnabled: enabled);

      expect(clipboardAvailability(s(ClipboardSyncState.ambientActive)).state,
          FeatureState.active);
      expect(
          clipboardAvailability(s(ClipboardSyncState.writeOnlyDormant)).state,
          FeatureState.manual);
      expect(clipboardAvailability(s(ClipboardSyncState.needsSetup)).state,
          FeatureState.needsSetup);
      expect(clipboardAvailability(s(ClipboardSyncState.restricted)).state,
          FeatureState.restricted);
      expect(clipboardAvailability(s(ClipboardSyncState.unavailable)).state,
          FeatureState.unavailable);
      expect(clipboardAvailability(s(ClipboardSyncState.stopped)).state,
          FeatureState.unavailable);
      expect(
          clipboardAvailability(
              s(ClipboardSyncState.ambientActive, enabled: false)),
          isA<FeatureStatus>().having((f) => f.state, 'state', FeatureState.disabled));
    });

    test('screen availability follows session and advertisement', () {
      SessionStatus st(ipc.SessionState state) =>
          SessionStatus(state: state);

      expect(
          screenAvailability(
                  session: st(ipc.SessionState.SESSION_STATE_STREAMING),
                  capabilities: caps())
              .state,
          FeatureState.active);
      expect(
          screenAvailability(
                  session: st(ipc.SessionState.SESSION_STATE_CONNECTING),
                  capabilities: null)
              .state,
          FeatureState.starting);
      expect(
          screenAvailability(
                  session: st(ipc.SessionState.SESSION_STATE_DISCONNECTED),
                  capabilities: null)
              .state,
          FeatureState.available);
      expect(
          screenAvailability(
              session: st(ipc.SessionState.SESSION_STATE_DISCONNECTED),
              capabilities: caps(supportsScreen: false)).state,
          FeatureState.unavailable);
      const failed = SessionStatus(
        state: ipc.SessionState.SESSION_STATE_FAILED,
        reasonCode: ipc.SessionReason.SESSION_REASON_RECONNECT_TIMEOUT,
      );
      final unavail =
          screenAvailability(session: failed, capabilities: caps());
      expect(unavail.state, FeatureState.unavailable);
      expect(unavail.detail, contains('reconnect'));
    });
  });
}
