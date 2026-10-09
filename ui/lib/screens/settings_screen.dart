import 'package:flutter/material.dart';
import '../controllers/phonebridge_controller.dart';
import '../models/media_capabilities.dart';
import '../ui/permission_onboarding_card.dart';
import 'diagnostics_screen.dart';
import 'about_screen.dart';
import 'report_issue_screen.dart';

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
        padding: const EdgeInsets.only(bottom: 32),
        children: [
          _sectionHeader(theme, 'General'),
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
          SwitchListTile(
            title: const Text('Automatic Clipboard Sync'),
            subtitle: const Text('Sync copied clips across paired devices'),
            value: clip.isEnabled,
            onChanged: (val) => controller.setClipboardEnabled(val),
          ),
          const Divider(),
          _sectionHeader(theme, 'Connection'),
          const ListTile(
            title: Text('Connection Path'),
            subtitle: Text('LAN · Direct (P2P-first, auto-selected)'),
          ),
          ListTile(
            title: const Text('Paired Devices'),
            subtitle: Text(controller.trustedDevices.isEmpty
                ? 'No paired devices yet'
                : '${controller.trustedDevices.length} known device(s)'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () => Navigator.of(context).pop(),
          ),
          const Divider(),
          _sectionHeader(theme, 'Permissions & Services'),
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
          if (controller.service.isAndroid) ...[
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16.0, vertical: 8.0),
              child: PermissionOnboardingCard(controller: controller),
            ),
          ],
          const Divider(),
          _sectionHeader(theme, 'Advanced'),
          const ExpansionTile(
            title: Text('Transport & Protocol Details'),
            subtitle: Text('Ports, crypto and media transport'),
            children: [
              ListTile(
                title: Text('Signaling Port'),
                subtitle: Text('Port 7804 (LAN mDNS auto-advertised)'),
              ),
              ListTile(
                title: Text('Security & Transport'),
                subtitle: Text('Mutual Ed25519 authentication · WebRTC media transport'),
              ),
            ],
          ),
          const Divider(),
          _sectionHeader(theme, 'Diagnostics & Developer'),
          ListTile(
            leading: const Icon(Icons.bug_report_outlined),
            title: const Text('Open Diagnostics Console'),
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
          const Divider(),
          _sectionHeader(theme, 'Support & Feedback'),
          ListTile(
            leading: const Icon(Icons.feedback_outlined),
            title: const Text('Report an Issue / Feedback'),
            subtitle: const Text('Send bug report or feedback to raxatechnologies@gmail.com'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () {
              Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => ReportIssueScreen(controller: controller),
                ),
              );
            },
          ),
          const Divider(),
          _sectionHeader(theme, 'About'),
          ListTile(
            leading: const Icon(Icons.info_outline),
            title: const Text('About PhoneBridge'),
            subtitle: const Text('Version, licenses, and architecture overview'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () {
              Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => AboutScreen(controller: controller),
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
    // The same runtime gate as the Screen presets: a geometry the encoder
    // cannot do is shown as unavailable with its reason, never offered.
    final caps = controller.effectiveCapabilities;
    String labelFor(int width, int height) {
      final probe = ScreenPreset(
        width: width,
        height: height,
        fps: controller.selectedFps,
        bitrateKbps: controller.selectedBitrateKbps,
        title: '',
        subtitle: '',
        guidance: '',
      );
      if (availabilityOf(probe, caps) == CapabilityAvailability.unsupported) {
        return ' (Unavailable: ${availabilityReason(probe, caps)})';
      }
      return '';
    }

    bool enabledFor(int width, int height) {
      final probe = ScreenPreset(
        width: width,
        height: height,
        fps: controller.selectedFps,
        bitrateKbps: controller.selectedBitrateKbps,
        title: '',
        subtitle: '',
        guidance: '',
      );
      return availabilityOf(probe, caps) != CapabilityAvailability.unsupported;
    }

    showDialog(
      context: context,
      builder: (ctx) => SimpleDialog(
        title: const Text('Select Resolution (request only)'),
        children: [
          SimpleDialogOption(
            onPressed: enabledFor(720, 1600)
                ? () {
                    controller.setResolution(720, 1600);
                    Navigator.pop(ctx);
                  }
                : null,
            child: Text('720p HD (720 x 1600) — Recommended${labelFor(720, 1600)}'),
          ),
          SimpleDialogOption(
            onPressed: enabledFor(1080, 2400)
                ? () {
                    controller.setResolution(1080, 2400);
                    Navigator.pop(ctx);
                  }
                : null,
            child: Text('1080p FHD (1080 x 2400)${labelFor(1080, 2400)}'),
          ),
        ],
      ),
    );
  }

  void _showFpsPicker(BuildContext context) {
    final caps = controller.effectiveCapabilities;
    String labelFor(int fps) {
      final probe = ScreenPreset(
        width: controller.selectedWidth,
        height: controller.selectedHeight,
        fps: fps,
        bitrateKbps: controller.selectedBitrateKbps,
        title: '',
        subtitle: '',
        guidance: '',
      );
      if (availabilityOf(probe, caps) == CapabilityAvailability.unsupported) {
        return ' (Unavailable: ${availabilityReason(probe, caps)})';
      }
      return '';
    }

    bool enabledFor(int fps) {
      final probe = ScreenPreset(
        width: controller.selectedWidth,
        height: controller.selectedHeight,
        fps: fps,
        bitrateKbps: controller.selectedBitrateKbps,
        title: '',
        subtitle: '',
        guidance: '',
      );
      return availabilityOf(probe, caps) != CapabilityAvailability.unsupported;
    }

    showDialog(
      context: context,
      builder: (ctx) => SimpleDialog(
        title: const Text('Select Frame Rate (request only)'),
        children: [
          SimpleDialogOption(
            onPressed: enabledFor(30)
                ? () {
                    controller.setFps(30);
                    Navigator.pop(ctx);
                  }
                : null,
            child: Text('30 fps (Recommended)${labelFor(30)}'),
          ),
          SimpleDialogOption(
            onPressed: enabledFor(60)
                ? () {
                    controller.setFps(60);
                    Navigator.pop(ctx);
                  }
                : null,
            child: Text('60 fps (Device-dependent)${labelFor(60)}'),
          ),
        ],
      ),
    );
  }
}
