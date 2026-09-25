import 'package:flutter/material.dart';
import '../controllers/phonebridge_controller.dart';
import 'diagnostics_screen.dart';
import 'about_screen.dart';

class SettingsScreen extends StatelessWidget {
  const SettingsScreen({
    super.key,
    required this.controller,
  });

  final PhoneBridgeController controller;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final clip = controller.clipboardStatus;

    return Scaffold(
      appBar: AppBar(
        title: const Text('Settings'),
      ),
      body: ListView(
        children: [
          _sectionHeader(theme, 'Screen Sharing'),
          ListTile(
            title: const Text('Streaming Resolution'),
            // Zero means the DEC-022 "device default" request, not 0x0.
            subtitle: Text(controller.selectedWidth == 0
                ? 'Device default (phone chooses)'
                : '${controller.selectedWidth}x${controller.selectedHeight}'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () => _showResolutionPicker(context),
          ),
          ListTile(
            title: const Text('Target Frame Rate'),
            subtitle: Text(controller.selectedFps == 0
                ? 'Device default (phone chooses)'
                : '${controller.selectedFps} fps'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () => _showFpsPicker(context),
          ),
          const Divider(),
          _sectionHeader(theme, 'Clipboard'),
          SwitchListTile(
            title: const Text('Automatic Synchronization'),
            subtitle: const Text('Sync copied clips across paired devices'),
            value: clip.isEnabled,
            onChanged: (val) => controller.setClipboardEnabled(val),
          ),
          ListTile(
            title: const Text('Companion Input Method'),
            subtitle: Text(clip.imeSelected ? 'Enabled and active' : 'Not configured (Tap for guidance)'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () {
              ScaffoldMessenger.of(context).showSnackBar(
                const SnackBar(
                  content: Text('Enable PhoneBridge Keyboard in Android Settings > System > Languages & Input'),
                  behavior: SnackBarBehavior.floating,
                ),
              );
            },
          ),
          const Divider(),
          _sectionHeader(theme, 'Connection & LAN'),
          const ListTile(
            title: Text('Signaling Port'),
            subtitle: Text('Port 7804 (LAN mDNS auto-advertised)'),
          ),
          const ListTile(
            title: Text('Security & Transport'),
            subtitle: Text('Mutual Ed25519 authentication · WebRTC media transport'),
          ),
          const Divider(),
          _sectionHeader(theme, 'System & Diagnostics'),
          ListTile(
            leading: const Icon(Icons.bug_report_outlined),
            title: const Text('Diagnostics & Developer'),
            subtitle: const Text('Live frame telemetry, encoder status, and Go core state'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () {
              Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => DiagnosticsScreen(controller: controller),
                ),
              );
            },
          ),
          ListTile(
            leading: const Icon(Icons.info_outline),
            title: const Text('About PhoneBridge'),
            subtitle: const Text('Version, licenses, and architecture overview'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () {
              Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => const AboutScreen(),
                ),
              );
            },
          ),
        ],
      ),
    );
  }

  Widget _sectionHeader(ThemeData theme, String title) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 8),
      child: Text(
        title,
        style: TextStyle(
          color: theme.colorScheme.primary,
          fontWeight: FontWeight.bold,
          fontSize: 13,
          letterSpacing: 0.5,
        ),
      ),
    );
  }

  void _showResolutionPicker(BuildContext context) {
    showDialog(
      context: context,
      builder: (ctx) => SimpleDialog(
        title: const Text('Select Resolution'),
        children: [
          SimpleDialogOption(
            onPressed: () {
              controller.setResolution(720, 1600);
              Navigator.pop(ctx);
            },
            child: const Text('720p HD (720 x 1600) — Recommended'),
          ),
          SimpleDialogOption(
            onPressed: () {
              controller.setResolution(1080, 2400);
              Navigator.pop(ctx);
            },
            child: const Text('1080p FHD (1080 x 2400)'),
          ),
        ],
      ),
    );
  }

  void _showFpsPicker(BuildContext context) {
    showDialog(
      context: context,
      builder: (ctx) => SimpleDialog(
        title: const Text('Select Frame Rate'),
        children: [
          SimpleDialogOption(
            onPressed: () {
              controller.setFps(30);
              Navigator.pop(ctx);
            },
            child: const Text('30 fps (Optimal battery & network)'),
          ),
          SimpleDialogOption(
            onPressed: () {
              controller.setFps(60);
              Navigator.pop(ctx);
            },
            child: const Text('60 fps (High motion)'),
          ),
        ],
      ),
    );
  }
}
