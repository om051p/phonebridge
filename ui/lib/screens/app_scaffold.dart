import 'package:flutter/material.dart';
import '../controllers/phonebridge_controller.dart';
import 'package:flutter/services.dart';
import '../services/phonebridge_channel.dart';
import 'home_screen.dart';
import 'devices_screen.dart';
import 'screen_sharing_screen.dart';
import 'clipboard_screen.dart';
import 'activity_screen.dart';
import 'settings_screen.dart';
import 'diagnostics_screen.dart';
import 'about_screen.dart';

class AppScaffold extends StatefulWidget {
  const AppScaffold({
    super.key,
    required this.controller,
    this.initialIndex = 0,
  });

  final PhoneBridgeController controller;
  final int initialIndex;

  @override
  State<AppScaffold> createState() => _AppScaffoldState();
}

class _AppScaffoldState extends State<AppScaffold> {
  late int _currentIndex;

  @override
  void initState() {
    super.initState();
    _currentIndex = widget.initialIndex;
    widget.controller.initialize();
    PhoneBridgeChannel.setMethodCallHandler(_handleNativeCall);
  }

  Future<dynamic> _handleNativeCall(MethodCall call) async {
    if (!mounted) return;
    switch (call.method) {
      case 'onNavigateTab':
        final tab = call.arguments['tab'] as int? ?? 0;
        setState(() {
          _currentIndex = tab.clamp(0, 4);
        });
        break;
      case 'onNavigateRoute':
        final route = call.arguments['route'] as String? ?? '/';
        if (route == '/settings') {
          _openSettings();
        } else if (route == '/diagnostics') {
          Navigator.of(context).push(
            MaterialPageRoute(
              builder: (_) => DiagnosticsScreen(controller: widget.controller),
            ),
          );
        } else if (route == '/about') {
          Navigator.of(context).push(
            MaterialPageRoute(
              builder: (_) => const AboutScreen(),
            ),
          );
        }
        break;
      case 'onTriggerAction':
        final cmd = call.arguments['cmd'] as String? ?? '';
        final args = call.arguments['args'] as String?;
        if (cmd == 'startSharing') {
          widget.controller.startScreenSharing();
        } else if (cmd == 'stopSharing') {
          widget.controller.stopScreenSharing();
        } else if (cmd == 'syncClipboard') {
          widget.controller.triggerClipboardPull();
        } else if (cmd == 'revokeDevice' && args != null) {
          widget.controller.revokeDevice(args);
        } else if (cmd == 'removeDevice' && args != null) {
          widget.controller.removeDevice(args);
        } else if (cmd == 'pop') {
          if (Navigator.of(context).canPop()) {
            Navigator.of(context).pop();
          }
        }
        break;
    }
  }

  void _onDestinationSelected(int index) {
    setState(() {
      _currentIndex = index;
    });
  }

  void _openSettings() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => SettingsScreen(controller: widget.controller),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: widget.controller,
      builder: (context, _) {
        final controller = widget.controller;
        final isCapturing = controller.isCapturing;
        final activePeer = controller.activePeer;

        final screens = [
          HomeScreen(
            controller: controller,
            onNavigateToTab: _onDestinationSelected,
          ),
          DevicesScreen(controller: controller),
          ScreenSharingScreen(controller: controller),
          ClipboardScreen(controller: controller),
          ActivityScreen(controller: controller),
        ];

        return Scaffold(
          appBar: AppBar(
            title: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                const Text(
                  'PhoneBridge',
                  style: TextStyle(fontWeight: FontWeight.bold, letterSpacing: -0.5),
                ),
                const SizedBox(width: 10),
                _buildStatusBadge(context, isCapturing, activePeer != null),
              ],
            ),
            actions: [
              IconButton(
                icon: const Icon(Icons.refresh),
                tooltip: 'Refresh Status',
                onPressed: controller.isLoading ? null : () => controller.refreshAll(),
              ),
              IconButton(
                icon: const Icon(Icons.settings_outlined),
                tooltip: 'Settings',
                onPressed: _openSettings,
              ),
            ],
          ),
          body: IndexedStack(
            index: _currentIndex,
            children: screens,
          ),
          bottomNavigationBar: NavigationBar(
            selectedIndex: _currentIndex,
            onDestinationSelected: _onDestinationSelected,
            destinations: const [
              NavigationDestination(
                icon: Icon(Icons.home_outlined),
                selectedIcon: Icon(Icons.home),
                label: 'Home',
              ),
              NavigationDestination(
                icon: Icon(Icons.devices_outlined),
                selectedIcon: Icon(Icons.devices),
                label: 'Devices',
              ),
              NavigationDestination(
                icon: Icon(Icons.screen_share_outlined),
                selectedIcon: Icon(Icons.screen_share),
                label: 'Screen',
              ),
              NavigationDestination(
                icon: Icon(Icons.content_paste_outlined),
                selectedIcon: Icon(Icons.content_paste),
                label: 'Clipboard',
              ),
              NavigationDestination(
                icon: Icon(Icons.history_outlined),
                selectedIcon: Icon(Icons.history),
                label: 'Activity',
              ),
            ],
          ),
        );
      },
    );
  }

  Widget _buildStatusBadge(BuildContext context, bool isCapturing, bool hasPeer) {
    final theme = Theme.of(context);
    final Color badgeColor;
    final String label;

    if (isCapturing) {
      badgeColor = Colors.green;
      label = 'Sharing';
    } else if (hasPeer) {
      badgeColor = Colors.blue;
      label = 'Paired';
    } else {
      badgeColor = Colors.grey;
      label = 'Ready';
    }

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: badgeColor.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: badgeColor.withValues(alpha: 0.4)),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 7,
            height: 7,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: badgeColor,
            ),
          ),
          const SizedBox(width: 5),
          Text(
            label,
            style: theme.textTheme.labelSmall?.copyWith(
              fontWeight: FontWeight.bold,
              color: badgeColor,
            ),
          ),
        ],
      ),
    );
  }
}
