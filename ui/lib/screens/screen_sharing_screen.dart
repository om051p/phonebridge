import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../controllers/phonebridge_controller.dart';
import '../generated/phonebridge/v1/phonebridge.pb.dart' as pb;
import '../models/link_status.dart';
import '../models/media_capabilities.dart';
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

    return Focus(
      autofocus: true,
      onKeyEvent: (node, event) {
        if (event is KeyDownEvent &&
            event.logicalKey == LogicalKeyboardKey.escape &&
            inSession) {
          controller.sendGlobalAction(
            pb.GlobalActionEvent_Type.TYPE_GLOBAL_ACTION_BACK,
          );
          return KeyEventResult.handled;
        }
        return KeyEventResult.ignored;
      },
      child: ListView(
        padding: const EdgeInsets.fromLTRB(16, 16, 16, 32),
        children: [
          _buildActiveStatusCard(theme, link, stats),
          const SizedBox(height: 16),
          if (inSession) ...[
            // The live mirror (Linux only): newest-frame-wins JPEG surface with
            // decode-on-arrival. While no frame has arrived it renders the
            // existing presentation state below it — the banners, telemetry and
            // controls are untouched and keep answering for the session.
            if (frameProvider != null) ...[
              _buildMirrorCard(context, theme, frameProvider, session.status.label),
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
    ),
  );
}

  Widget _buildMirrorCard(
    BuildContext context,
    ThemeData theme,
    ProvidesFrameStream provider,
    String stateLabel,
  ) {
    final isControlActive = controller.remoteControlEnabled && controller.isRemoteControlAvailable;

    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        // Top status and control bar
        Material(
          color: Colors.transparent,
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
            decoration: BoxDecoration(
              color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.5),
              borderRadius: BorderRadius.circular(12),
              border: Border.all(
                color: theme.colorScheme.outlineVariant.withValues(alpha: 0.3),
              ),
            ),
            child: Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                // Status Pill
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
                  decoration: BoxDecoration(
                    color: (isControlActive ? Colors.green : Colors.amber).withValues(alpha: 0.15),
                    borderRadius: BorderRadius.circular(8),
                    border: Border.all(
                      color: (isControlActive ? Colors.green : Colors.amber).withValues(alpha: 0.4),
                    ),
                  ),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Container(
                        width: 8,
                        height: 8,
                        decoration: BoxDecoration(
                          shape: BoxShape.circle,
                          color: isControlActive ? Colors.green : Colors.amber,
                        ),
                      ),
                      const SizedBox(width: 6),
                      Text(
                        isControlActive ? 'REMOTE CONTROL ACTIVE' : 'VIEW ONLY',
                        style: TextStyle(
                          fontSize: 11,
                          fontWeight: FontWeight.bold,
                          letterSpacing: 0.5,
                          color: isControlActive ? Colors.green : Colors.amber.shade800,
                        ),
                      ),
                    ],
                  ),
                ),
                // Control Toggle
                Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(
                      'Remote Control',
                      style: theme.textTheme.labelMedium?.copyWith(
                        fontWeight: FontWeight.bold,
                      ),
                    ),
                    const SizedBox(width: 8),
                    Switch(
                      value: controller.remoteControlEnabled,
                      onChanged: (val) => controller.setRemoteControlEnabled(val),
                    ),
                  ],
                ),
              ],
            ),
          ),
        ),
        const SizedBox(height: 8),
        Card(
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
            height: 520,
            child: ScreenFrameView(
              provider: provider,
              enabled: isControlActive,
              onInput: (frame) => controller.sendInput(frame),
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
        ),
        const SizedBox(height: 8),
        _buildNavigationToolbar(context, theme),
      ],
    );
  }

  Widget _buildNavigationToolbar(BuildContext context, ThemeData theme) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
      decoration: BoxDecoration(
        color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.5),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: theme.colorScheme.outlineVariant.withValues(alpha: 0.3),
        ),
      ),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceEvenly,
        children: [
          IconButton(
            tooltip: 'Back (Esc)',
            icon: const Icon(Icons.arrow_back),
            onPressed: () => controller.sendGlobalAction(
              pb.GlobalActionEvent_Type.TYPE_GLOBAL_ACTION_BACK,
            ),
          ),
          IconButton(
            tooltip: 'Home',
            icon: const Icon(Icons.circle_outlined),
            onPressed: () => controller.sendGlobalAction(
              pb.GlobalActionEvent_Type.TYPE_GLOBAL_ACTION_HOME,
            ),
          ),
          IconButton(
            tooltip: 'Recents',
            icon: const Icon(Icons.crop_square),
            onPressed: () => controller.sendGlobalAction(
              pb.GlobalActionEvent_Type.TYPE_GLOBAL_ACTION_RECENTS,
            ),
          ),
          IconButton(
            tooltip: 'Notifications',
            icon: const Icon(Icons.notifications_none),
            onPressed: () => controller.sendGlobalAction(
              pb.GlobalActionEvent_Type.TYPE_GLOBAL_ACTION_NOTIFICATIONS,
            ),
          ),
          IconButton(
            tooltip: 'Send Text',
            icon: const Icon(Icons.keyboard_outlined),
            onPressed: () => _showTextInputDialog(context),
          ),
        ],
      ),
    );
  }

  void _showTextInputDialog(BuildContext context) {
    final textController = TextEditingController();
    showDialog(
      context: context,
      builder: (dialogCtx) => AlertDialog(
        title: const Text('Send Text to Device'),
        content: TextField(
          controller: textController,
          autofocus: true,
          decoration: const InputDecoration(
            hintText: 'Enter text to commit into active field...',
            border: OutlineInputBorder(),
          ),
          onSubmitted: (value) {
            if (value.isNotEmpty) {
              controller.sendText(value);
            }
            Navigator.of(dialogCtx).pop();
          },
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogCtx).pop(),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () {
              final val = textController.text;
              if (val.isNotEmpty) {
                controller.sendText(val);
              }
              Navigator.of(dialogCtx).pop();
            },
            child: const Text('Send'),
          ),
        ],
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
    // Human-readable identity: name + platform + trust/connection/path, never
    // an internal "Target PC x1". Trust (shield) and connection (dot) stay
    // separate so Trusted + Offline/Available/Connected read distinctly.
    final hasPeer = peer != null;
    // Read the unified link, not the raw session: on Android the phone owns
    // capture and has no local session model, so the raw session is always
    // inactive even while streaming (the badge would say Ready).
    final sessionActive = controller.linkStatus.hasSession;
    final connectionLabel =
        sessionActive ? controller.linkStatus.label : 'Disconnected';
    final connectionColor =
        linkPhaseColor(theme, controller.linkStatus.phase);
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
                    hasPeer ? peer.displayName : 'Paired device',
                    style: const TextStyle(fontWeight: FontWeight.bold, fontSize: 14),
                    overflow: TextOverflow.ellipsis,
                    maxLines: 1,
                  ),
                  const SizedBox(height: 2),
                  Text(
                    hasPeer
                        ? '${peer.platform} · LAN · Direct'
                        : 'Local Network (Auto-Discovery) · LAN · Direct',
                    style: theme.textTheme.labelMedium?.copyWith(
                      color: theme.colorScheme.onSurfaceVariant,
                    ),
                    overflow: TextOverflow.ellipsis,
                    maxLines: 1,
                  ),
                  const SizedBox(height: 6),
                  Wrap(
                    spacing: 6,
                    runSpacing: 6,
                    children: [
                      if (hasPeer)
                        Container(
                          padding: const EdgeInsets.symmetric(
                              horizontal: 8, vertical: 4),
                          decoration: BoxDecoration(
                            color: Colors.green.withValues(alpha: 0.15),
                            borderRadius: BorderRadius.circular(8),
                          ),
                          child: const Row(
                            mainAxisSize: MainAxisSize.min,
                            children: [
                              Icon(Icons.shield_outlined,
                                  size: 12, color: Colors.green),
                              SizedBox(width: 4),
                              Text(
                                'Trusted',
                                style: TextStyle(
                                    color: Colors.green,
                                    fontSize: 11,
                                    fontWeight: FontWeight.bold),
                              ),
                            ],
                          ),
                        ),
                      Container(
                        padding: const EdgeInsets.symmetric(
                            horizontal: 8, vertical: 4),
                        decoration: BoxDecoration(
                          color: connectionColor.withValues(alpha: 0.12),
                          borderRadius: BorderRadius.circular(8),
                          border: Border.all(
                              color: connectionColor.withValues(alpha: 0.3)),
                        ),
                        child: Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Container(
                              width: 7,
                              height: 7,
                              decoration: BoxDecoration(
                                shape: BoxShape.circle,
                                color: connectionColor,
                              ),
                            ),
                            const SizedBox(width: 5),
                            Text(
                              connectionLabel,
                              style: TextStyle(
                                color: connectionColor,
                                fontSize: 11,
                                fontWeight: FontWeight.bold,
                              ),
                            ),
                          ],
                        ),
                      ),
                    ],
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildQualityPresetsCard(ThemeData theme, bool isCapturing) {
    // Runtime-gated presets: each entry is checked against the advertised
    // capabilities (Go-retained DEC-022 advertisement on Linux, encoder-read
    // advertisement on Android). A passing gate means "worth requesting" —
    // the phone's applied tuple, shown above, remains the final verdict.
    final actual = controller.session.status.actual;
    final actualLabel = actual == null
        ? 'Applied: not reported by the phone yet'
        : 'Applied: ${actual.width}x${actual.height} @${actual.fps} fps';
    final caps = controller.effectiveCapabilities;
    // DEC-022 device default is Linux-only: on Android the capture pipeline
    // is configured from this side and never sends a session request tuple.
    final presets = controller.service.isLinux
        ? kScreenPresets
        : kScreenPresets.where((p) => !p.isDeviceDefault).toList();
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
            const SizedBox(height: 4),
            Text(
              caps == null
                  ? 'Checking device capabilities… presets unlock as the phone reports what it applies ($actualLabel).'
                  : 'Requests only — the phone reports what it applies. See session details above ($actualLabel).',
              style: theme.textTheme.bodySmall?.copyWith(
                color: theme.colorScheme.onSurfaceVariant,
              ),
            ),
            const SizedBox(height: 12),
            for (var i = 0; i < presets.length; i++) ...[
              if (i > 0) const Divider(),
              _buildPresetOption(
                theme,
                preset: presets[i],
                capabilities: caps,
                sessionActive: isCapturing,
              ),
            ],
          ],
        ),
      ),
    );
  }

  Widget _buildPresetOption(
    ThemeData theme, {
    required ScreenPreset preset,
    required pb.MediaCapabilities? capabilities,
    required bool sessionActive,
  }) {
    final isSelected = controller.selectedWidth == preset.width &&
        controller.selectedHeight == preset.height &&
        controller.selectedFps == preset.fps;
    final availability = availabilityOf(preset, capabilities);
    // An unsupported mode is not offered: the tap is disabled and the reason
    // states what the encoder actually supports. Unknown stays tappable —
    // the peer accepts unstated limits, and its typed answer decides.
    final disabled =
        sessionActive || availability == CapabilityAvailability.unsupported;
    final String badge;
    final Color badgeColor;
    switch (availability) {
      case CapabilityAvailability.supported:
        badge = preset.guidance;
        badgeColor = theme.colorScheme.primary;
        break;
      case CapabilityAvailability.unknown:
        badge = 'Checking…';
        badgeColor = theme.colorScheme.onSurfaceVariant;
        break;
      case CapabilityAvailability.unsupported:
        badge = 'Unavailable';
        badgeColor = theme.colorScheme.error;
        break;
    }
    final reason = availabilityReason(preset, capabilities);
    final subtitle = reason.isNotEmpty &&
            availability == CapabilityAvailability.unsupported
        ? '${preset.subtitle}\n$reason'
        : preset.subtitle;

    return InkWell(
      onTap: disabled
          ? null
          : () {
              controller.setResolution(preset.width, preset.height);
              controller.setFps(preset.fps);
              controller.setBitrate(preset.bitrateKbps);
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
                  Wrap(
                    spacing: 8,
                    runSpacing: 4,
                    crossAxisAlignment: WrapCrossAlignment.center,
                    children: [
                      Text(
                        preset.title,
                        style: TextStyle(
                          fontWeight: FontWeight.bold,
                          color: disabled ? theme.disabledColor : null,
                        ),
                      ),
                      Container(
                        padding: const EdgeInsets.symmetric(
                            horizontal: 6, vertical: 2),
                        decoration: BoxDecoration(
                          color: badgeColor
                              .withValues(alpha: 0.1),
                          borderRadius: BorderRadius.circular(6),
                        ),
                        child: Text(
                          isSelected ? 'Currently active' : badge,
                          style: TextStyle(
                            fontSize: 10,
                            fontWeight: FontWeight.bold,
                            color: badgeColor,
                          ),
                        ),
                      ),
                    ],
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
