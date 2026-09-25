// Phase 6 preparation: the production navigation shell walked end-to-end on
// the single app-owned IPC stack. Pins the Phase 5 ownership invariants in
// their real context — every tab, the Settings route, and the Diagnostics
// route render from one LocalIpcClient with exactly one daemon event
// subscription, and no screen tears the stack down on unmount.

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:phonebridge_ui/main.dart';
import 'package:phonebridge_ui/services/linux_bridge_service.dart';

import 'support/fake_ipc_client.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  testWidgets(
      'five tabs and the Settings/Diagnostics routes run on one controller '
      'with one daemon subscription', (tester) async {
    tester.view.physicalSize = const Size(1080, 2400);
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
      home: AppScaffold(controller: controller),
    ));
    await tester.pumpAndSettle();

    // Construction wires exactly one daemon subscription; the scaffold's
    // initialize() (refresh, session hydrate, transfer history) adds none.
    expect(client.streamEventsCalls, 1);

    // Home opens first, no-peer on the fake daemon.
    expect(find.text('No Paired PC'), findsOneWidget);

    // Devices tab: empty trust store + empty discovery, from the service.
    await tester.tap(find.text('Devices'));
    await tester.pumpAndSettle();
    expect(find.text('Trusted Devices (0)'), findsOneWidget);
    expect(find.text('Discovered Devices (0)'), findsOneWidget);
    expect(find.text('This Device (Linux Desktop)'), findsOneWidget);

    // Screen tab: the Linux service path (isLinux) owns the mirror action.
    await tester.tap(find.text('Screen'));
    await tester.pumpAndSettle();
    expect(find.text('MIRROR PHONE SCREEN'), findsOneWidget);
    expect(find.text('720p HD (Balanced)'), findsOneWidget);

    // Clipboard tab.
    await tester.tap(find.text('Clipboard'));
    await tester.pumpAndSettle();
    expect(find.text('Clipboard Synchronization'), findsOneWidget);
    expect(find.text('SYNC CURRENT CLIPBOARD NOW'), findsOneWidget);

    // Activity tab (last — its filter chips share labels with nav entries).
    await tester.tap(find.text('Activity'));
    await tester.pumpAndSettle();
    expect(find.text('All'), findsOneWidget);

    // Settings route from the app bar gear.
    await tester.tap(find.byTooltip('Settings'));
    await tester.pumpAndSettle();
    expect(find.text('About PhoneBridge'), findsOneWidget);
    expect(find.text('Diagnostics & Developer'), findsOneWidget);

    // Diagnostics route, pushed from Settings.
    await tester.tap(find.widgetWithText(ListTile, 'Diagnostics & Developer'));
    await tester.pumpAndSettle();
    expect(find.text('Live Telemetry (DEC-020/021)'), findsOneWidget);
    expect(find.text('Local Engine IPC (DEC-018)'), findsOneWidget);
    // Handshake runs once on mount, through the app-owned service.
    expect(client.handshakeCalls, 1);

    // The whole walk — five tabs, two pushed routes — left exactly the
    // service's own subscription. No surface added a second streamEvents().
    expect(client.streamEventsCalls, 1);

    // Ownership: unmounting everything never tears down the injected stack...
    await tester.pumpWidget(const SizedBox());
    await tester.pump();
    expect(client.shutdownCalls, 0);

    // ...and the explicit owner's dispose cascades exactly once.
    controller.dispose();
    expect(client.shutdownCalls, 1);
  });
}
