import 'dart:async';
import 'package:flutter/material.dart' hide ClipboardStatus;
import 'package:flutter_test/flutter_test.dart';
import 'package:phonebridge_ui/controllers/phonebridge_controller.dart';
import 'package:phonebridge_ui/models/capture_stats.dart';
import 'package:phonebridge_ui/models/clipboard_status.dart';
import 'package:phonebridge_ui/models/device_state.dart';
import 'package:phonebridge_ui/models/discovered_device.dart';
import 'package:phonebridge_ui/models/trusted_device.dart';
import 'package:phonebridge_ui/screens/app_scaffold.dart';
import 'package:phonebridge_ui/screens/clipboard_screen.dart';
import 'package:phonebridge_ui/screens/devices_screen.dart';
import 'package:phonebridge_ui/screens/screen_sharing_screen.dart';
import 'package:phonebridge_ui/services/platform_bridge_service.dart';

class MockPlatformBridgeService implements PlatformBridgeService {
  MockPlatformBridgeService({
    this.isLinuxMode = true,
  });

  final bool isLinuxMode;

  @override
  bool get isAndroid => !isLinuxMode;

  @override
  bool get isLinux => isLinuxMode;

  DeviceState deviceState = const DeviceState(
    model: 'Linux Desktop',
    manufacturer: 'Linux (Daemon 1.0.0)',
    sdkInt: 0,
    isCapturing: false,
    goEngineLoaded: true,
    codec: 'H.264',
    isHardwareCodec: false,
    deviceId: 'linux-desktop-id',
    displayName: 'Arch Linux Workstation',
    clipboardState: 'AMBIENT_ACTIVE',
    imeSelected: false,
  );

  CaptureStats captureStats = CaptureStats.initial;

  ClipboardStatus clipboardStatus = const ClipboardStatus(
    state: ClipboardSyncState.ambientActive,
    imeSelected: false,
    maxPayloadSize: 786432,
    isEnabled: true,
    syncCount: 3,
  );

  List<TrustedDevice> trustedDevices = [
    const TrustedDevice(
      deviceId: 'poco-f5-1234567890abcdef',
      displayName: 'POCO F5',
      platform: 'android',
      pairedAtMs: 1726000000000,
      lastSeenMs: 1726000010000,
      revoked: false,
    ),
  ];

  List<DiscoveredDevice> discoveredDevices = [
    const DiscoveredDevice(
      id: 'discovered-phone-1',
      name: 'Pixel 8',
      model: 'Pixel 8',
      version: '1.0',
      host: '192.168.1.100',
      port: 7804,
    ),
  ];

  bool captureStarted = false;
  bool clipboardPullTriggered = false;
  bool pairCalled = false;
  bool confirmCalled = false;

  final StreamController<CaptureStats> _statsCtrl = StreamController<CaptureStats>.broadcast();
  final StreamController<Map<dynamic, dynamic>> _rawEventsCtrl =
      StreamController<Map<dynamic, dynamic>>.broadcast();

  @override
  Future<DeviceState> getDeviceState() async => deviceState;

  @override
  Future<CaptureStats> getMediaStats() async => captureStats;

  @override
  Future<List<TrustedDevice>> getTrustedDevices() async => trustedDevices;

  @override
  Future<ClipboardStatus> getClipboardStatus() async => clipboardStatus;

  @override
  Future<bool> triggerClipboardPull() async {
    clipboardPullTriggered = true;
    return true;
  }

  @override
  Future<bool> startCapture({
    String? receiverUrl,
    int width = 720,
    int height = 1600,
    int fps = 30,
    int bitrateKbps = 2500,
  }) async {
    captureStarted = true;
    return true;
  }

  @override
  Future<bool> stopCapture() async {
    captureStarted = false;
    return true;
  }

  @override
  Future<bool> revokeDevice(String deviceId) async => true;

  @override
  Future<bool> removeDevice(String deviceId) async => true;

  @override
  Future<List<DiscoveredDevice>> listDiscoveredDevices() async => discoveredDevices;

  @override
  Future<PairingResult?> pairDevice(String deviceId) async {
    pairCalled = true;
    return const PairingResult(deviceName: 'Pixel 8', sasCode: '492817');
  }

  @override
  Future<bool> confirmPairing({required String deviceId, required bool confirmed}) async {
    confirmCalled = true;
    return true;
  }

  @override
  Stream<CaptureStats> get statsStream => _statsCtrl.stream;

  @override
  Stream<Map<dynamic, dynamic>> get rawEventsStream => _rawEventsCtrl.stream;

  @override
  void setNativeCallHandler(Future<dynamic> Function(dynamic call)? handler) {}

  @override
  void dispose() {
    _statsCtrl.close();
    _rawEventsCtrl.close();
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  Widget buildTestWidget(Widget child) {
    return MaterialApp(
      theme: ThemeData(useMaterial3: true),
      home: child is Scaffold ? child : Scaffold(body: child),
    );
  }

  group('Desktop Mission Control & Platform Bridge', () {
    testWidgets('AppScaffold renders NavigationRail on wide desktop displays (width >= 720)', (tester) async {
      tester.view.physicalSize = const Size(1280, 800);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      final mock = MockPlatformBridgeService(isLinuxMode: true);
      final controller = PhoneBridgeController(service: mock);

      await tester.pumpWidget(buildTestWidget(AppScaffold(controller: controller)));
      await tester.pumpAndSettle();

      expect(find.byType(NavigationRail), findsOneWidget);
      expect(find.byType(NavigationBar), findsNothing);
      expect(find.text('Home'), findsOneWidget);
      expect(find.text('Devices'), findsOneWidget);
      expect(find.text('Screen'), findsOneWidget);
      expect(find.text('Clipboard'), findsOneWidget);
      expect(find.text('Activity'), findsOneWidget);
    });

    testWidgets('AppScaffold renders NavigationBar on narrow mobile displays (width < 720)', (tester) async {
      tester.view.physicalSize = const Size(600, 1000);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      final mock = MockPlatformBridgeService(isLinuxMode: true);
      final controller = PhoneBridgeController(service: mock);

      await tester.pumpWidget(buildTestWidget(AppScaffold(controller: controller)));
      await tester.pumpAndSettle();

      expect(find.byType(NavigationBar), findsOneWidget);
      expect(find.byType(NavigationRail), findsNothing);
    });

    testWidgets('ScreenSharingScreen adapts controls for Linux desktop receiver', (tester) async {
      tester.view.physicalSize = const Size(1080, 1920);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      final mock = MockPlatformBridgeService(isLinuxMode: true);
      final controller = PhoneBridgeController(service: mock);

      await tester.pumpWidget(buildTestWidget(ScreenSharingScreen(controller: controller)));
      await tester.pumpAndSettle();

      expect(find.text('MIRROR PHONE SCREEN'), findsOneWidget);
      expect(find.textContaining('PhoneBridge functions as a high-performance WebRTC receiver'), findsOneWidget);

      await tester.ensureVisible(find.text('MIRROR PHONE SCREEN'));
      await tester.tap(find.text('MIRROR PHONE SCREEN'));
      await tester.pumpAndSettle();
      expect(mock.captureStarted, isTrue);
    });

    testWidgets('ClipboardScreen displays Wayland data-control status on Linux', (tester) async {
      tester.view.physicalSize = const Size(1080, 1920);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      final mock = MockPlatformBridgeService(isLinuxMode: true);
      final controller = PhoneBridgeController(service: mock);

      await tester.pumpWidget(buildTestWidget(ClipboardScreen(controller: controller)));
      await tester.pumpAndSettle();

      expect(find.text('Wayland Data Control'), findsOneWidget);
      expect(find.textContaining('Wayland data-control helper is active'), findsOneWidget);
      expect(find.text('Companion Input Method'), findsNothing);

      await tester.ensureVisible(find.text('SYNC CURRENT CLIPBOARD NOW'));
      await tester.tap(find.text('SYNC CURRENT CLIPBOARD NOW'));
      await tester.pumpAndSettle();
      expect(mock.clipboardPullTriggered, isTrue);
    });

    testWidgets('DevicesScreen displays discovered devices and SAS pairing dialog on Linux', (tester) async {
      tester.view.physicalSize = const Size(1080, 1920);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      final mock = MockPlatformBridgeService(isLinuxMode: true);
      final controller = PhoneBridgeController(service: mock);
      await controller.refreshAll();
      await controller.refreshDiscoveredDevices();

      await tester.pumpWidget(buildTestWidget(DevicesScreen(controller: controller)));
      await tester.pumpAndSettle();

      expect(find.text('This Device (Arch Linux Workstation)'), findsOneWidget);
      expect(find.text('Discovered Devices (1)'), findsOneWidget);
      expect(find.text('Pixel 8'), findsOneWidget);

      await tester.tap(find.text('PAIR'));
      await tester.pumpAndSettle();

      expect(find.text('Confirm Pairing Code'), findsOneWidget);
      expect(find.text('492817'), findsOneWidget);

      await tester.tap(find.text('CONFIRM MATCH'));
      await tester.pumpAndSettle();

      expect(mock.confirmCalled, isTrue);
    });
  });
}
