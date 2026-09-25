import 'package:flutter/material.dart';
import '../controllers/phonebridge_controller.dart';
import '../models/link_status.dart';
import '../services/frame_stream.dart';
import '../ui/link_indicator.dart';
import '../ui/screen_frame_view.dart';
import '../ui/session_presentation.dart';
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
    // The connection card renders the unified status; the capture telemetry now
    // only supplies stream numbers. "Is the phone streaming?" and "is the app
    // connected?" are answered by one model instead of a capture flag.
    final link = controller.linkStatus;
    final isSharing = controller.isSharing;
    final stats = controller.captureStats;
    final peer = controller.activePeer;

    final isLinux = controller.service.isLinux;

    // The shared session model decides which surface this is: with no live
    // session the tab stays the existing start experience; while one is live
    // it swaps in the validated in-session banner, telemetry and stop control.
    // Every value below is a read — the screen holds no session state of its
    // own, so it can never disagree with the badge or any other surface.
    final session = controller.session;
    final inSession = session.status.isActive;
    // Frame mirror seam (Phase 6 Slice 3A): only a platform that can open the
    // daemon's frame stream gets the live surface; every other platform keeps
    // the exact existing presentation.
    final Object service = controller.service;
    final frameProvider =
        service is ProvidesFrameStream ? service : null;
    // A live session (or Android's own capture) means STOP; otherwise START.
    // On Android inSession is always false, so this collapses exactly to the
    // previous isSharing behaviour.
    final showStop = inSession || isSharing;

    return ListView(
      padding: const EdgeInsets.all(16.0),
      children: [
        _buildActiveStatusCard(theme, link, stats),
        const SizedBox(height: 16),
        if (inSession) ...[
          // The live mirror (Linux only): newest-frame-wins JPEG surface with
          // decode-on-arrival. While no frame has arrived it renders the
          // existing presentation state below it — the banners, telemetry and
          // controls are untouched and keep answering for the session.
          if (frameProvider != null) ...[
            _buildMirrorCard(theme, frameProvider, session.status.label),
            const SizedBox(height: 16),
          ],
          // The validated session information (DEC-022): id, negotiation,
          // recovery and typed failure detail — read from the shared model.
          SessionStateBanner(
            status: session.status,
            sessionId: session.activeSessionId,
          ),
          const SizedBox(height: 16),
          if (session.status.hasReportedSink) ...[
            // The daemon reports which sink it actually chose (ffplay,
            // headless null, pipe, file) — the banner can no longer claim
            // ffplay on a host that never opened a window.
            VideoDisplayBanner(
              sinkKind: session.status.sinkKind,
              sinkActive: session.status.sinkActive,
            ),
            const SizedBox(height: 16),
          ],
        ],
        _buildTargetDeviceCard(theme, peer),
        const SizedBox(height: 16),
        _buildQualityPresetsCard(theme, showStop),
        const SizedBox(height: 16),
        if (inSession) ...[
          // Telemetry from the controller's one poll; the card renders zeros
          // until the first snapshot rather than guessing numbers.
          StreamTelemetryCard(stats: session.streamStats),
          const SizedBox(height: 16),
        ],
        _buildPermissionGuidanceCard(theme, isLinux),
        const SizedBox(height: 20),
        SizedBox(
          width: double.infinity,
          height: 52,
          child: FilledButton.icon(
            style: FilledButton.styleFrom(
              backgroundColor: showStop ? theme.colorScheme.error : null,
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(14),
              ),
            ),
            onPressed: controller.isLoading
                ? null
                : () async {
                    final ok = showStop
                        ? await controller.stopScreenSharing()
                        : await controller.startScreenSharing();
                    if (ok || !context.mounted) return;
                    // A control failure must surface where the control lives
                    // (migrated from the retired session view): the status
                    // card only shows failures that actually transitioned the
                    // shared session model, so a rejected start/stop would
                    // otherwise vanish without a trace.
                    final reason = controller.lastErrorMessage ??
                        controller.session.lastError;
                    ScaffoldMessenger.of(context).showSnackBar(
                      SnackBar(
                        content: Text(
                          reason ??
                              (showStop
                                  ? 'Stop session failed'
                                  : 'Start session failed'),
                        ),
                        backgroundColor: Theme.of(context).colorScheme.error,
                        behavior: SnackBarBehavior.floating,
                      ),
                    );
                  },
            icon: Icon(showStop
                ? Icons.stop
                : (isLinux ? Icons.phone_android : Icons.screen_share)),
            label: Text(
              showStop
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

  Widget _buildMirrorCard(
    ThemeData theme,
    ProvidesFrameStream provider,
    String stateLabel,
  ) {
    return Card(
      elevation: 0,
      color: Colors.black,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(
          color: theme.colorScheme.outlineVariant.withValues(alpha: 0.4),
        ),
      ),
      clipBehavior: Clip.antiAlias,
      child: SizedBox(
        height: 320,
        child: ScreenFrameView(
          provider: provider,
          // Fallback: the existing presentation state (shared session model)
          // until the first frame arrives or after the stream stops.
          fallback: Center(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(
                  Icons.phone_android,
                  size: 36,
                  color: theme.colorScheme.onSurfaceVariant,
                ),
                const SizedBox(height: 8),
                Text(
                  '$stateLabel · waiting for live frames',
                  style: theme.textTheme.bodySmall?.copyWith(
                    color: theme.colorScheme.onSurfaceVariant,
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildActiveStatusCard(ThemeData theme, LinkStatus link, dynamic stats) {
    final color = linkPhaseColor(theme, link.phase);
    final String subtitle;
    if (link.phase == LinkPhase.streaming) {
      subtitle = 'Duration: ${_formatDuration(stats.durationUs)}';
    } else if (link.actionHint.isNotEmpty) {
      // A failure must carry what the user can do about it, not just the state.
      subtitle = link.actionHint;
    } else {
      subtitle = link.description;
    }

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
            // Expanded, not Spacer: a failure line is a whole sentence, and the
            // status text must be able to wrap instead of overflowing the row.
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    link.label.toUpperCase(),
                    style: theme.textTheme.titleSmall?.copyWith(
                      fontWeight: FontWeight.bold,
                      color: color,
                    ),
                  ),
                  Text(
                    subtitle,
                    maxLines: 3,
                    overflow: TextOverflow.ellipsis,
                    style: theme.textTheme.bodySmall?.copyWith(
                      color: theme.colorScheme.onSurfaceVariant,
                    ),
                  ),
                ],
              ),
            ),
            const SizedBox(width: 8),
            if (link.phase == LinkPhase.streaming && stats.currentFps != null)
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
            // DEC-022, migrated from the retired desktop session view: an
            // all-zero tuple means "device default" — the phone answers with
            // what it actually applies instead of a request the UI could
            // mistake for a confirmed parameter. Desktop-only: on Android the
            // capture pipeline is configured from this side and never sends a
            // session request tuple.
            if (controller.service.isLinux) ...[
              _buildPresetOption(
                theme,
                title: 'Device Default (Phone Chooses)',
                subtitle: 'No requested tuple · the phone reports what it applies',
                width: 0,
                height: 0,
                fps: 0,
                bitrate: 0,
                disabled: isCapturing,
              ),
              const Divider(),
            ],
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
