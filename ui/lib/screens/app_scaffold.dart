import 'package:flutter/material.dart';
import '../controllers/phonebridge_controller.dart';
import '../models/link_status.dart';
import '../ui/link_indicator.dart';
import '../ui/pairing_request_dialog.dart';
import 'package:flutter/services.dart';
import 'home_screen.dart';
import 'devices_screen.dart';
import 'screen_sharing_screen.dart';
import 'clipboard_screen.dart';
import 'activity_screen.dart';
import 'settings_screen.dart';
import 'diagnostics_screen.dart';
import 'about_screen.dart';
import 'report_issue_screen.dart';

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
    widget.controller.service.setNativeCallHandler(_handleNativeCall);
    widget.controller.addListener(_onControllerChanged);
  }

  @override
  void dispose() {
    widget.controller.removeListener(_onControllerChanged);
    super.dispose();
  }

  /// The one-shot Pairing Request dialog (Phase 2): when a new inbound
  /// request appears, surface it exactly once. The request stays visible on
  /// the Devices tab until answered or expired; the dialog is only the
  /// immediate prompt.
  void _onControllerChanged() {
    if (!mounted) return;
    final request = widget.controller.consumePendingPairingPrompt();
    if (request == null) return;
    PairingRequestDialog.show(
      context,
      controller: widget.controller,
      request: request,
    );
  }

  Future<dynamic> _handleNativeCall(dynamic rawCall) async {
    if (!mounted || rawCall is! MethodCall) return;
    final call = rawCall;
    switch (call.method) {
      case 'trustChanged':
        // Native trust-store mutation (pairing commit, revoke, remove),
        // including background responder commits with no dialog open.
        await widget.controller.refreshAll();
        break;
      case 'pairingChanged':
        // Inbound-pairing state changed natively (Phase 2): a request
        // arrived, was answered elsewhere, or expired. Refresh the snapshot;
        // the listener below prompts the dialog for any new request.
        await widget.controller.refreshInboundPairings();
        break;
      case 'sessionChanged':
        await widget.controller.refreshAll();
        break;
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
              builder: (_) => AboutScreen(controller: widget.controller),
            ),
          );
        } else if (route == '/report') {
          Navigator.of(context).push(
            MaterialPageRoute(
              builder: (_) => ReportIssueScreen(controller: widget.controller),
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
        // One connection status for the whole app (Phase 5): the badge no longer
        // infers connection from the capture flag plus a paired peer.
        final link = controller.linkStatus;

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

        return LayoutBuilder(
          builder: (context, constraints) {
            final isWide = constraints.maxWidth >= 720;

            return Scaffold(
              appBar: AppBar(
                title: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    ClipRRect(
                      borderRadius: BorderRadius.circular(6),
                      child: Image.asset(
                        'assets/branding/logo.png',
                        width: 24,
                        height: 24,
                        fit: BoxFit.cover,
                        errorBuilder: (context, error, stackTrace) => const SizedBox.shrink(),
                      ),
                    ),
                    const SizedBox(width: 8),
                    const Flexible(
                      child: Text(
                        'PhoneBridge',
                        style: TextStyle(fontWeight: FontWeight.bold, letterSpacing: -0.5),
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                    const SizedBox(width: 8),
                    _buildStatusBadge(context, link),
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
              body: SafeArea(
                bottom: true,
                child: isWide
                    ? Row(
                        children: [
                          NavigationRail(
                            selectedIndex: _currentIndex,
                            onDestinationSelected: _onDestinationSelected,
                            labelType: NavigationRailLabelType.all,
                            destinations: const [
                              NavigationRailDestination(
                                icon: Icon(Icons.home_outlined),
                                selectedIcon: Icon(Icons.home),
                                label: Text('Home'),
                              ),
                              NavigationRailDestination(
                                icon: Icon(Icons.devices_outlined),
                                selectedIcon: Icon(Icons.devices),
                                label: Text('Devices'),
                              ),
                              NavigationRailDestination(
                                icon: Icon(Icons.screen_share_outlined),
                                selectedIcon: Icon(Icons.screen_share),
                                label: Text('Screen'),
                              ),
                              NavigationRailDestination(
                                icon: Icon(Icons.content_paste_outlined),
                                selectedIcon: Icon(Icons.content_paste),
                                label: Text('Clipboard'),
                              ),
                              NavigationRailDestination(
                                icon: Icon(Icons.history_outlined),
                                selectedIcon: Icon(Icons.history),
                                label: Text('Activity'),
                              ),
                            ],
                          ),
                          const VerticalDivider(thickness: 1, width: 1),
                          Expanded(
                            child: IndexedStack(
                              index: _currentIndex,
                              children: screens,
                            ),
                          ),
                        ],
                      )
                    : IndexedStack(
                        index: _currentIndex,
                        children: screens,
                      ),
              ),
              bottomNavigationBar: isWide
                  ? null
                  : NavigationBar(
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
      },
    );
  }

  Widget _buildStatusBadge(BuildContext context, LinkStatus link) {
    final theme = Theme.of(context);
    final badgeColor = linkPhaseColor(theme, link.phase);
    final label = link.label;
    final tooltip = [
      link.description,
      if (link.transferLine.isNotEmpty) link.transferLine,
      if (link.actionHint.isNotEmpty) link.actionHint,
    ].where((line) => line.isNotEmpty).join('\n');

    return Tooltip(
      message: tooltip,
      child: Container(
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
            // Transfer activity is reported alongside the connection, never as
            // the connection: an in-flight file must not read as a session state.
            if (link.transfer.isActive) ...[
              const SizedBox(width: 5),
              Icon(Icons.swap_vert, size: 12, color: badgeColor),
            ],
          ],
        ),
      ),
    );
  }
}
