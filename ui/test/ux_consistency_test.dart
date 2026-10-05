// RCTAR UX + connection-state consistency: canonical semantics, responsive
// layout, safe-area, trust vs connection, clipboard, screen-session and
// capability-aware quality tests.

import 'package:flutter/material.dart'
    hide ConnectionState, ClipboardStatus;
import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/generated/phonebridge/localipc/v1/local_ipc.pb.dart'
    as ipc;
import 'package:phonebridge_ui/generated/phonebridge/v1/phonebridge.pb.dart'
    as pb;
import 'package:phonebridge_ui/main.dart';
import 'package:phonebridge_ui/models/clipboard_status.dart';
import 'package:phonebridge_ui/models/device_presence.dart';
import 'package:phonebridge_ui/models/discovered_device.dart';
import 'package:phonebridge_ui/models/link_status.dart';
import 'package:phonebridge_ui/models/session_status.dart';
import 'package:phonebridge_ui/models/trusted_device.dart';
import 'package:phonebridge_ui/screens/activity_screen.dart';
import 'package:phonebridge_ui/screens/devices_screen.dart';
import 'package:phonebridge_ui/services/linux_bridge_service.dart';

import 'support/fake_ipc_client.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('canonical device presence', () {
    test('trust and discovery stay distinct facts', () {
      const trusted = TrustedDevice(
        deviceId: 'd1',
        displayName: 'POCO F1',
        platform: 'android',
        pairedAtMs: 1,
        lastSeenMs: 2,
        revoked: false,
      );
      expect(trustOf(trusted: trusted), DeviceTrust.trusted);
      expect(
        trustOf(
            trusted: const TrustedDevice(
                deviceId: 'd1',
                displayName: 'x',
                platform: 'android',
                pairedAtMs: 0,
                lastSeenMs: 0,
                revoked: true)),
        DeviceTrust.revoked,
      );
      expect(trustOf(), DeviceTrust.untrusted);

      const discovered = DiscoveredDevice(
        id: 'd1',
        name: 'POCO F1',
        model: 'm',
        version: 'v',
        host: '192.168.1.2',
        port: 7804,
      );
      expect(
        discoveryOf(discovered: [discovered], deviceId: 'd1'),
        DiscoveryPresence.discovered,
      );
      expect(
        discoveryOf(discovered: const [], deviceId: 'd1'),
        DiscoveryPresence.notDiscovered,
      );
      expect(connectionPathLabel(), 'LAN · Direct');
      expect(
        deviceIdentityLine(displayName: 'POCO F1', platform: 'android'),
        'POCO F1 · android',
      );
    });

    test('connection and feature session derive from the same session', () {
      const connecting = SessionStatus(
          state: ipc.SessionState.SESSION_STATE_CONNECTING);
      expect(connectionStateOfSession(connecting),
          DeviceConnectionState.connecting);
      expect(featureSessionOf(connecting), FeatureSessionState.starting);

      const streaming =
          SessionStatus(state: ipc.SessionState.SESSION_STATE_STREAMING);
      expect(
          connectionStateOfSession(streaming), DeviceConnectionState.connected);
      expect(featureSessionOf(streaming), FeatureSessionState.active);

      const failed =
          SessionStatus(state: ipc.SessionState.SESSION_STATE_FAILED);
      expect(connectionStateOfSession(failed),
          DeviceConnectionState.connectionError);
      expect(featureSessionOf(failed), FeatureSessionState.failed);

      expect(
          connectionStateOfLink(const LinkStatus(
            phase: LinkPhase.streaming,
            label: 'Sharing',
            description: 'x',
            severity: LinkSeverity.good,
          )),
          DeviceConnectionState.connected);
    });
  });

  group('clipboard canonical states', () {
    test('Linux daemon strings map to setup/active, not generic stopped', () {
      expect(ClipboardSyncState.fromString('READY'),
          ClipboardSyncState.ambientActive);
      expect(ClipboardSyncState.fromString('AMBIENT_ACTIVE'),
          ClipboardSyncState.ambientActive);
      expect(ClipboardSyncState.fromString('WRITE_ONLY_DORMANT'),
          ClipboardSyncState.writeOnlyDormant);
      expect(ClipboardSyncState.fromString('COSMIC_FLAG_REQUIRED'),
          ClipboardSyncState.needsSetup);
      expect(ClipboardSyncState.fromString('NO_DATA_CONTROL'),
          ClipboardSyncState.needsSetup);
      expect(ClipboardSyncState.fromString('WAYLAND_UNAVAILABLE'),
          ClipboardSyncState.needsSetup);
      expect(ClipboardSyncState.fromString('STOPPED'),
          ClipboardSyncState.stopped);
    });

    test('service status distinguishes dormant from active', () {
      const dormant = ClipboardStatus(
        state: ClipboardSyncState.writeOnlyDormant,
        imeSelected: false,
        maxPayloadSize: 786432,
        isEnabled: true,
      );
      const active = ClipboardStatus(
        state: ClipboardSyncState.ambientActive,
        imeSelected: true,
        maxPayloadSize: 786432,
        isEnabled: true,
      );
      expect(clipboardServiceStatusOf(dormant), ServiceStatus.dormant);
      expect(clipboardServiceStatusOf(active), ServiceStatus.active);
      expect(
        clipboardServiceStatusOf(active.copyWith(isEnabled: false)),
        ServiceStatus.disabled,
      );
      expect(
        clipboardServiceStatusOf(const ClipboardStatus(
          state: ClipboardSyncState.needsSetup,
          imeSelected: false,
          maxPayloadSize: 786432,
        )),
        ServiceStatus.needsSetup,
      );
    });
  });

  group('responsive layout + safe area', () {
    testWidgets('discovered device card has no overflow on narrow phone',
        (tester) async {
      // Narrow width (overflow probe) but tall enough that both sections
      // build without scrolling: ListView lazily builds below-fold rows.
      tester.view.physicalSize = const Size(360, 1200);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      final client = FakeIpcClient();
      client.devices = [
        ipc.DiscoveredDevice(
          id: 'very-long-device-id-1234567890abcdef',
          name: 'Very Long Device Name That Could Overflow',
          model: 'Extra Long Model String For Overflow Testing',
          version: 'v1',
          address: '192.168.1.100',
          port: 7804,
        ),
      ];
      client.trustedDevices = [
        ipc.TrustedDevice(
          deviceId: 'very-long-device-id-1234567890abcdef',
          displayName: 'Very Long Trusted Device Name Overflow',
          platform: 'android',
        ),
      ];
      final controller = PhoneBridgeController(
        service: LinuxBridgeService(client: client),
      );
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

      // Canonical rows: one row for the single id (trusted + discovered),
      // grouped under the trusted section with separated badges.
      expect(find.textContaining('Your devices'), findsOneWidget);
      // Trust shield and connection dot are separate badges.
      expect(find.text('TRUSTED'), findsOneWidget);
      expect(find.textContaining('LAN · Direct'), findsWidgets);
      // Actions wrap instead of overflowing: CONNECT on the tile, RE-PAIR
      // in the detail sheet.
      expect(find.text('CONNECT'), findsOneWidget);
      await tester.tap(find.text('Very Long Trusted Device Name Overflow'));
      await tester.pumpAndSettle();
      expect(find.text('RE-PAIR'), findsOneWidget);
      expect(tester.takeException(), isNull);

      controller.dispose();
    });

    testWidgets('every tab keeps content above the bottom navigation',
        (tester) async {
      tester.view.physicalSize = const Size(390, 844);
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

      for (final tab in ['Devices', 'Screen', 'Clipboard', 'Activity']) {
        await tester.tap(find.text(tab));
        await tester.pumpAndSettle();
        expect(tester.takeException(), isNull);
      }
      // Home last: hero + clipboard + transfers + activity all mounted.
      await tester.tap(find.text('Home'));
      await tester.pumpAndSettle();
      expect(find.text('Screen Sharing'), findsOneWidget);
      expect(tester.takeException(), isNull);

      controller.dispose();
    });

    testWidgets('activity filters include System and scroll horizontally',
        (tester) async {
      tester.view.physicalSize = const Size(360, 740);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      final client = FakeIpcClient();
      final controller = PhoneBridgeController(
        service: LinuxBridgeService(client: client),
      );
      addTearDown(controller.dispose);
      addTearDown(client.events.close);

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(body: ActivityScreen(controller: controller)),
      ));
      await tester.pumpAndSettle();

      for (final label in ['All', 'Screen', 'Clipboard', 'Pairing', 'Connection', 'System']) {
        expect(find.text(label), findsOneWidget);
      }
      expect(tester.takeException(), isNull);
    });

    testWidgets('devices screen exposes separate trust and connection badges',
        (tester) async {
      tester.view.physicalSize = const Size(1080, 2400);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      final client = FakeIpcClient();
      client.trustedDevices = [
        ipc.TrustedDevice(
          deviceId: 'offline-1',
          displayName: 'Offline Phone',
          platform: 'android',
        ),
      ];
      final controller = PhoneBridgeController(
        service: LinuxBridgeService(client: client),
      );
      controller.initialize();
      await controller.refreshAll();
      addTearDown(controller.dispose);
      addTearDown(client.events.close);

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(body: DevicesScreen(controller: controller)),
      ));
      await tester.pumpAndSettle();

      // Trusted + Offline: shield says TRUSTED, dot says OFFLINE.
      expect(find.text('TRUSTED'), findsWidgets);
      expect(find.text('OFFLINE'), findsWidgets);
      expect(find.text('NOT DISCOVERED'), findsOneWidget);
      expect(tester.takeException(), isNull);
    });
  });

  group('screen session consistency + capability wording', () {
    testWidgets('home and screen agree while connecting (not idle vs active)',
        (tester) async {
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

      await tester.pumpWidget(PhoneBridgeApp(
        home: AppScaffold(controller: controller),
      ));
      await tester.pumpAndSettle();

      client.events.add(ipc.StreamEventsResponse(
        sessionEvent: ipc.SessionEvent(
          sessionId: 'sess-1',
          state: ipc.SessionState.SESSION_STATE_CONNECTING,
        ),
      ));
      await tester.pump();
      await tester.pump();

      // Connecting is a live session on both surfaces: STOP is offered in
      // Screen, Home shows the session phase instead of START SHARING.
      expect(controller.session.status.isActive, isTrue);
      expect(controller.isSharing, isFalse);
      expect(find.text('Connecting'), findsWidgets);

      controller.dispose();
    });

    testWidgets('quality presets are capability-aware, not universal',
        (tester) async {
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

      await tester.pumpWidget(PhoneBridgeApp(
        home: AppScaffold(controller: controller),
      ));
      await tester.pumpAndSettle();

      await tester.tap(find.text('Screen'));
      await tester.pumpAndSettle();

      // No advertisement yet: unknown, never a guess.
      expect(find.text('1080p High Frame Rate'), findsOneWidget);
      expect(find.text('Checking…'), findsWidgets);
      expect(find.textContaining('Checking device capabilities'), findsOneWidget);
      expect(tester.takeException(), isNull);

      // The phone reports a 720p30 h264 encoder: 1080p presets become
      // honestly unavailable with the encoder's reason, 720p stays guided.
      client.snapshot = ipc.GetSessionStateResponse(
        sessionId: 'sess-1',
        state: ipc.SessionState.SESSION_STATE_DISCONNECTED,
        capabilities: pb.MediaCapabilities(
          codecs: ['h264'],
          maxWidth: 720,
          maxHeight: 1600,
          maxFps: 30,
          supportsScreen: true,
        ),
      );
      client.events.add(ipc.StreamEventsResponse(
        sessionEvent: ipc.SessionEvent(
          sessionId: 'sess-1',
          state: ipc.SessionState.SESSION_STATE_DISCONNECTED,
        ),
      ));
      await tester.pump();
      await tester.pump();
      await controller.session.refresh();
      await tester.pump();
      await tester.pump();

      expect(controller.session.status.hasCapabilities, isTrue);
      expect(find.text('Unavailable'), findsWidgets);
      expect(
        find.textContaining('tops out at 720px wide'),
        findsWidgets,
      );
      // 720p still offered with its guidance; the active preset is marked.
      expect(find.text('720p HD (Balanced)'), findsOneWidget);
      expect(find.text('Currently active'), findsWidgets);
      expect(tester.takeException(), isNull);

      controller.dispose();
    });
  });
}
