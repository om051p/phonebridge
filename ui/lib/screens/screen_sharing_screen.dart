import 'package:flutter/material.dart';
import '../controllers/phonebridge_controller.dart';
import 'diagnostics_screen.dart';

class ScreenSharingScreen extends StatelessWidget {
  const ScreenSharingScreen({
    super.key,
    required this.controller,
  });

  final PhoneBridgeController controller;

  String _formatDuration(int us) {
    if (us <= 0) return '00:00';
    final totalSec = us ~/ 1000000;
    final min = (totalSec ~/ 60).toString().padLeft(2, '0');
    final sec = (totalSec % 60).toString().padLeft(2, '0');
    return '$min:$sec';
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final isCapturing = controller.isCapturing;
    final stats = controller.captureStats;
    final peer = controller.activePeer;

    final isLinux = controller.service.isLinux;

    return ListView(
      padding: const EdgeInsets.all(16.0),
      children: [
        _buildActiveStatusCard(theme, isCapturing, stats),
        const SizedBox(height: 16),
        _buildTargetDeviceCard(theme, peer),
        const SizedBox(height: 16),
        _buildQualityPresetsCard(theme, isCapturing),
        const SizedBox(height: 16),
        _buildPermissionGuidanceCard(theme, isLinux),
        const SizedBox(height: 20),
        SizedBox(
          width: double.infinity,
          height: 52,
          child: FilledButton.icon(
            style: FilledButton.styleFrom(
              backgroundColor: isCapturing ? theme.colorScheme.error : null,
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(14),
              ),
            ),
            onPressed: controller.isLoading
                ? null
                : () {
                    if (isCapturing) {
                      controller.stopScreenSharing();
                    } else {
                      controller.startScreenSharing();
                    }
                  },
            icon: Icon(isCapturing
                ? Icons.stop
                : (isLinux ? Icons.phone_android : Icons.screen_share)),
            label: Text(
              isCapturing
                  ? (isLinux ? 'STOP RECEIVER SESSION' : 'STOP SCREEN SHARING')
                  : (isLinux ? 'MIRROR PHONE SCREEN' : 'START SCREEN SHARING'),
              style: const TextStyle(fontWeight: FontWeight.bold, fontSize: 16),
            ),
          ),
        ),
        const SizedBox(height: 16),
        Center(
          child: TextButton.icon(
            onPressed: () {
              Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => DiagnosticsScreen(controller: controller),
                ),
              );
            },
            icon: const Icon(Icons.analytics_outlined, size: 16),
            label: const Text('View Stream Diagnostics & Telemetry'),
          ),
        ),
      ],
    );
  }

  Widget _buildActiveStatusCard(ThemeData theme, bool isCapturing, dynamic stats) {
    final color = isCapturing ? Colors.green : Colors.grey;

    return Card(
      elevation: 0,
      color: color.withValues(alpha: 0.1),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(color: color.withValues(alpha: 0.3)),
      ),
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Row(
          children: [
            Container(
              width: 12,
              height: 12,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                color: color,
              ),
            ),
            const SizedBox(width: 12),
            Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  isCapturing ? 'STREAMING ACTIVE' : 'SHARING IDLE',
                  style: theme.textTheme.titleSmall?.copyWith(
                    fontWeight: FontWeight.bold,
                    color: isCapturing ? Colors.green.shade700 : Colors.grey.shade700,
                  ),
                ),
                Text(
                  isCapturing
                      ? 'Duration: ${_formatDuration(stats.durationUs)}'
                      : 'Ready to mirror display',
                  style: theme.textTheme.bodySmall?.copyWith(
                    color: theme.colorScheme.onSurfaceVariant,
                  ),
                ),
              ],
            ),
            const Spacer(),
            if (isCapturing && stats.currentFps != null)
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                decoration: BoxDecoration(
                  color: theme.colorScheme.surfaceContainerHighest,
                  borderRadius: BorderRadius.circular(8),
                ),
                child: Text(
                  '${stats.currentFps!.toStringAsFixed(1)} FPS',
                  style: const TextStyle(fontWeight: FontWeight.bold, fontSize: 12),
                ),
              ),
          ],
        ),
      ),
    );
  }

  Widget _buildTargetDeviceCard(ThemeData theme, dynamic peer) {
    return Card(
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.35),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(color: theme.colorScheme.outlineVariant.withValues(alpha: 0.4)),
      ),
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Row(
          children: [
            Icon(Icons.computer, color: theme.colorScheme.primary),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'Target PC',
                    style: theme.textTheme.labelMedium?.copyWith(
                      color: theme.colorScheme.onSurfaceVariant,
                    ),
                  ),
                  Text(
                    peer != null ? peer.displayName : 'Local Network (Auto-Discovery)',
                    style: const TextStyle(fontWeight: FontWeight.bold, fontSize: 14),
                  ),
                ],
              ),
            ),
            if (peer != null)
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                decoration: BoxDecoration(
                  color: Colors.green.withValues(alpha: 0.15),
                  borderRadius: BorderRadius.circular(8),
                ),
                child: const Text(
                  'Trusted',
                  style: TextStyle(color: Colors.green, fontSize: 11, fontWeight: FontWeight.bold),
                ),
              ),
          ],
        ),
      ),
    );
  }

  Widget _buildQualityPresetsCard(ThemeData theme, bool isCapturing) {
    return Card(
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.35),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(color: theme.colorScheme.outlineVariant.withValues(alpha: 0.4)),
      ),
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'Streaming Resolution & Quality',
              style: theme.textTheme.titleSmall?.copyWith(
                fontWeight: FontWeight.bold,
              ),
            ),
            const SizedBox(height: 12),
            _buildPresetOption(
              theme,
              title: '720p HD (Balanced)',
              subtitle: '720x1600 @ 30 fps · Low latency, optimal stability',
              width: 720,
              height: 1600,
              fps: 30,
              bitrate: 2500,
              disabled: isCapturing,
            ),
            const Divider(),
            _buildPresetOption(
              theme,
              title: '1080p FHD (Sharp Text)',
              subtitle: '1080x2400 @ 30 fps · Crisp details for reading & docs',
              width: 1080,
              height: 2400,
              fps: 30,
              bitrate: 5000,
              disabled: isCapturing,
            ),
            const Divider(),
            _buildPresetOption(
              theme,
              title: '1080p 60fps (Smooth Motion)',
              subtitle: '1080x2400 @ 60 fps · High frame rate streaming',
              width: 1080,
              height: 2400,
              fps: 60,
              bitrate: 8000,
              disabled: isCapturing,
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildPresetOption(
    ThemeData theme, {
    required String title,
    required String subtitle,
    required int width,
    required int height,
    required int fps,
    required int bitrate,
    required bool disabled,
  }) {
    final isSelected = controller.selectedWidth == width &&
        controller.selectedHeight == height &&
        controller.selectedFps == fps;

    return InkWell(
      onTap: disabled
          ? null
          : () {
              controller.setResolution(width, height);
              controller.setFps(fps);
              controller.setBitrate(bitrate);
            },
      borderRadius: BorderRadius.circular(10),
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 8.0, horizontal: 4.0),
        child: Row(
          children: [
            Icon(
              isSelected ? Icons.radio_button_checked : Icons.radio_button_unchecked,
              color: isSelected ? theme.colorScheme.primary : theme.colorScheme.outline,
              size: 22,
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    style: TextStyle(
                      fontWeight: FontWeight.bold,
                      color: disabled ? theme.disabledColor : null,
                    ),
                  ),
                  Text(
                    subtitle,
                    style: TextStyle(
                      fontSize: 12,
                      color: theme.colorScheme.onSurfaceVariant,
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildPermissionGuidanceCard(ThemeData theme, bool isLinux) {
    return Card(
      elevation: 0,
      color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.2),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(color: theme.colorScheme.outlineVariant.withValues(alpha: 0.3)),
      ),
      child: Padding(
        padding: const EdgeInsets.all(14.0),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(Icons.info_outline, size: 20, color: theme.colorScheme.primary),
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                isLinux
                    ? 'On Linux, PhoneBridge functions as a high-performance WebRTC receiver for Android screen mirroring, with bidirectional clipboard sync.'
                    : 'Android will show a system permission dialog asking "Start recording or casting with PhoneBridge?" to protect your privacy.',
                style: theme.textTheme.bodySmall?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                  height: 1.3,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
