import 'dart:async';

import 'package:flutter/material.dart';
import '../controllers/phonebridge_controller.dart';
import '../services/linux_bridge_service.dart' show LinuxBridgeService;
import '../services/local_ipc_client.dart' show LocalIpcState;

class DiagnosticsScreen extends StatefulWidget {
  const DiagnosticsScreen({
    super.key,
    required this.controller,
  });

  final PhoneBridgeController controller;

  @override
  State<DiagnosticsScreen> createState() => _DiagnosticsScreenState();
}

class _DiagnosticsScreenState extends State<DiagnosticsScreen> {
  late final TextEditingController _receiverController;

  // Local-IPC diagnostics (DEC-018), folded in from the retired dashboard
  // shell. These are *presentations* of the app-owned service's transport
  // facts — local UI state, never a second source of truth for connection.
  // The listeners ride the service's local broadcasts off its single daemon
  // subscription; no screen-level IPC client or event subscription exists.
  LocalIpcState _ipcState = LocalIpcState.disconnected;
  String? _ipcServerVersion;
  int? _ipcNegotiatedVersion;
  int? _ipcGeneration;
  int? _lastPingLatencyMs;
  int _receivedEventCount = 0;
  StreamSubscription<void>? _pulseSub;
  StreamSubscription<LocalIpcState>? _stateSub;

  @override
  void initState() {
    super.initState();
    _receiverController = TextEditingController(text: widget.controller.receiverUrl);
    final ipc = widget.controller.linuxService;
    if (ipc != null) {
      _stateSub = ipc.ipcStateStream.listen((st) {
        if (mounted) setState(() => _ipcState = st);
      });
      _pulseSub = ipc.eventPulse.listen((_) {
        if (mounted) setState(() => _receivedEventCount++);
      });
      _initLocalIpc(ipc);
    }
  }

  Future<void> _initLocalIpc(LinuxBridgeService ipc) async {
    try {
      final hs = await ipc.handshake();
      if (mounted) {
        setState(() {
          _ipcServerVersion = hs.serverVersion;
          _ipcNegotiatedVersion = hs.negotiatedVersion;
          _ipcGeneration = hs.daemonGeneration.toInt();
        });
      }
    } catch (_) {
      // Local daemon may not be active yet; the PING/HEALTH actions retry.
    }
  }

  Future<void> _pingDaemon() async {
    final ipc = widget.controller.linuxService;
    if (ipc == null) return;
    final sw = Stopwatch()..start();
    try {
      await ipc.ping(nonce: DateTime.now().millisecondsSinceEpoch);
      sw.stop();
      if (mounted) {
        setState(() => _lastPingLatencyMs = sw.elapsedMilliseconds);
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text('Ping OK (${sw.elapsedMilliseconds} ms)'),
            duration: const Duration(seconds: 1),
          ),
        );
      }
    } catch (e) {
      if (mounted) _showIpcError('Ping failed: $e');
    }
  }

  Future<void> _checkDaemonHealth() async {
    final ipc = widget.controller.linuxService;
    if (ipc == null) return;
    try {
      final h = await ipc.health();
      if (mounted) {
        setState(() {
          _ipcServerVersion = h.serverVersion;
          _ipcGeneration = h.daemonGeneration.toInt();
        });
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text('Daemon ready: ${h.ready}, uptime: ${h.uptimeMs} ms'),
            duration: const Duration(seconds: 2),
          ),
        );
      }
    } catch (e) {
      if (mounted) _showIpcError('Health check failed: $e');
    }
  }

  void _showIpcError(String message) {
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(message),
        backgroundColor: Theme.of(context).colorScheme.error,
        behavior: SnackBarBehavior.floating,
      ),
    );
  }

  @override
  void dispose() {
    _stateSub?.cancel();
    _pulseSub?.cancel();
    _receiverController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final controller = widget.controller;

    return ListenableBuilder(
      listenable: controller,
      builder: (context, _) {
        final device = controller.deviceState;
        final stats = controller.captureStats;
        final clip = controller.clipboardStatus;
        final isCapturing = stats.isCapturing;

        return Scaffold(
          appBar: AppBar(
            title: const Text('Diagnostics & Developer'),
            actions: [
              IconButton(
                icon: const Icon(Icons.refresh),
                tooltip: 'Refresh Telemetry',
                onPressed: () => controller.refreshAll(),
              ),
            ],
          ),
          body: ListView(
            padding: const EdgeInsets.all(16.0),
            children: [
              _buildTelemetryCard(theme, stats, isCapturing),
              const SizedBox(height: 16),
              _buildGoEngineCard(theme, device, stats),
              if (controller.linuxService != null) ...[
                const SizedBox(height: 16),
                _buildLocalIpcCard(theme),
              ],
              const SizedBox(height: 16),
              _buildClipboardInternalsCard(theme, clip),
              const SizedBox(height: 16),
              _buildEndpointOverrideCard(theme, controller, isCapturing),
              const SizedBox(height: 16),
              _buildDeviceHardwareCard(theme, device),
            ],
          ),
        );
      },
    );
  }

  Widget _buildTelemetryCard(ThemeData theme, dynamic stats, bool isCapturing) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(Icons.analytics, color: theme.colorScheme.primary),
                const SizedBox(width: 8),
                Text('Live Telemetry (DEC-020/021)', style: theme.textTheme.titleMedium),
                const Spacer(),
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
                  decoration: BoxDecoration(
                    color: isCapturing
                        ? Colors.green.withValues(alpha: 0.15)
                        : Colors.grey.withValues(alpha: 0.15),
                    borderRadius: BorderRadius.circular(8),
                  ),
                  child: Text(
                    isCapturing ? 'Active' : 'Idle',
                    style: TextStyle(
                      color: isCapturing ? Colors.green : Colors.grey,
                      fontWeight: FontWeight.bold,
                      fontSize: 12,
                    ),
                  ),
                ),
              ],
            ),
            const Divider(),
            Row(
              children: [
                _metricBox(
                  theme,
                  'Current FPS',
                  stats.currentFps != null
                      ? stats.currentFps!.toStringAsFixed(1)
                      : (isCapturing ? '—' : '0.0'),
                  Icons.speed,
                  Colors.blue,
                ),
                const SizedBox(width: 12),
                _metricBox(
                  theme,
                  'Keyframes (IDR)',
                  stats.keyframes.toString(),
                  Icons.key,
                  Colors.amber,
                ),
              ],
            ),
            const SizedBox(height: 12),
            Row(
              children: [
                _metricBox(
                  theme,
                  'Encoded Frames',
                  stats.encodedFrames.toString(),
                  Icons.video_library,
                  Colors.purple,
                ),
                const SizedBox(width: 12),
                _metricBox(
                  theme,
                  'Admitted Frames',
                  stats.admittedFrames.toString(),
                  Icons.send,
                  Colors.green,
                ),
              ],
            ),
            const SizedBox(height: 12),
            _infoRow(
              'GOP Drops (Backpressure)',
              '${stats.droppedFrames} frames',
              valueColor: stats.droppedFrames > 0 ? Colors.orange : null,
            ),
            _infoRow('Active Codec', stats.codec),
          ],
        ),
      ),
    );
  }

  Widget _buildGoEngineCard(ThemeData theme, dynamic device, dynamic stats) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(Icons.memory, color: theme.colorScheme.primary),
                const SizedBox(width: 8),
                Text('Go Core Engine & Transport', style: theme.textTheme.titleMedium),
              ],
            ),
            const Divider(),
            _infoRow(
              'JNI Binary',
              device.goEngineLoaded ? 'Loaded (libphonebridge_core.so)' : 'Unavailable',
              valueColor: device.goEngineLoaded ? Colors.green : Colors.orange,
            ),
            _infoRow('Selected Encoder', stats.isCapturing ? stats.codec : device.codec),
            _infoRow(
              'Hardware Accelerated',
              (stats.isCapturing ? stats.isHardwareCodec : device.isHardwareCodec)
                  ? 'Yes (Qualcomm HW Surface)'
                  : 'Software',
            ),
            _infoRow('Signaling Transport', 'Pion WebRTC / LAN HTTP 7804'),
          ],
        ),
      ),
    );
  }

  Widget _buildLocalIpcCard(ThemeData theme) {
    Color stateColor;
    String stateLabel;
    switch (_ipcState) {
      case LocalIpcState.connected:
        stateColor = Colors.green;
        stateLabel = 'Connected';
        break;
      case LocalIpcState.connecting:
        stateColor = Colors.amber;
        stateLabel = 'Connecting';
        break;
      case LocalIpcState.reconnecting:
        stateColor = Colors.orange;
        stateLabel = 'Reconnecting';
        break;
      case LocalIpcState.disconnected:
      case LocalIpcState.closed:
        stateColor = Colors.grey;
        stateLabel = 'Disconnected';
        break;
    }

    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(Icons.hub, color: theme.colorScheme.primary),
                const SizedBox(width: 8),
                Expanded(
                  child: Text('Local Engine IPC (DEC-018)',
                      style: theme.textTheme.titleMedium),
                ),
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                  decoration: BoxDecoration(
                    color: stateColor.withValues(alpha: 0.15),
                    borderRadius: BorderRadius.circular(8),
                    border: Border.all(color: stateColor.withValues(alpha: 0.5)),
                  ),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Container(
                        width: 8,
                        height: 8,
                        decoration: BoxDecoration(
                          shape: BoxShape.circle,
                          color: stateColor,
                        ),
                      ),
                      const SizedBox(width: 6),
                      Text(
                        stateLabel,
                        style: TextStyle(
                          fontSize: 12,
                          fontWeight: FontWeight.bold,
                          color: stateColor,
                        ),
                      ),
                    ],
                  ),
                ),
              ],
            ),
            const Divider(),
            _infoRow('Transport', 'UDS + gRPC (SO_PEERCRED + Bearer Token)'),
            _infoRow(
              'Socket Path',
              widget.controller.linuxService?.socketPath ?? '—',
            ),
            _infoRow('Daemon Version', _ipcServerVersion ?? 'Not connected'),
            _infoRow(
              'Negotiated Version',
              _ipcNegotiatedVersion != null ? 'v$_ipcNegotiatedVersion' : '—',
            ),
            _infoRow(
              'Daemon Generation',
              _ipcGeneration != null ? '$_ipcGeneration' : '—',
            ),
            _infoRow('Relayed Events', '$_receivedEventCount messages'),
            _infoRow(
              'Ping Latency',
              _lastPingLatencyMs != null ? '$_lastPingLatencyMs ms' : '—',
            ),
            const SizedBox(height: 12),
            Row(
              children: [
                Expanded(
                  child: OutlinedButton.icon(
                    onPressed: _pingDaemon,
                    icon: const Icon(Icons.network_ping, size: 18),
                    label: const Text('PING'),
                  ),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: OutlinedButton.icon(
                    onPressed: _checkDaemonHealth,
                    icon: const Icon(Icons.health_and_safety, size: 18),
                    label: const Text('HEALTH'),
                  ),
                ),
                const SizedBox(width: 8),
                Expanded(
                  // Re-runs the handshake on demand so a daemon that came up
                  // after mount still reports its negotiated version and
                  // generation — the on-demand refresh the retired session
                  // view's daemon card offered, now beside the other two
                  // daemon probes.
                  child: OutlinedButton.icon(
                    onPressed: widget.controller.linuxService == null
                        ? null
                        : () => _initLocalIpc(widget.controller.linuxService!),
                    icon: const Icon(Icons.handshake, size: 18),
                    label: const Text('HANDSHAKE'),
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildClipboardInternalsCard(ThemeData theme, dynamic clip) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(Icons.content_paste_go, color: theme.colorScheme.primary),
                const SizedBox(width: 8),
                Text('Clipboard Engine Internals (DEC-023)', style: theme.textTheme.titleMedium),
              ],
            ),
            const Divider(),
            _infoRow('Adapter State', clip.state.name),
            _infoRow('Companion IME Selected', clip.imeSelected ? 'Yes' : 'No'),
            _infoRow('Application Payload Ceiling', '${clip.maxPayloadSize} bytes (768 KiB)'),
            _infoRow('Zero-Logging Guarantee', 'Strict (Payload text never exposed)'),
          ],
        ),
      ),
    );
  }

  Widget _buildEndpointOverrideCard(
    ThemeData theme,
    PhoneBridgeController controller,
    bool isCapturing,
  ) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(Icons.settings_ethernet, color: theme.colorScheme.primary),
                const SizedBox(width: 8),
                Text('Receiver URL Override (Optional)', style: theme.textTheme.titleMedium),
              ],
            ),
            const SizedBox(height: 8),
            Text(
              'By default, PhoneBridge discovers receivers automatically. Specify a custom URL for headless or direct network testing.',
              style: theme.textTheme.bodySmall?.copyWith(color: theme.colorScheme.onSurfaceVariant),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _receiverController,
              decoration: const InputDecoration(
                labelText: 'Receiver Endpoint',
                hintText: 'http://127.0.0.1:7804',
                prefixIcon: Icon(Icons.link),
                border: OutlineInputBorder(),
                isDense: true,
              ),
              enabled: !isCapturing,
              onChanged: (val) => controller.setReceiverUrl(val),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildDeviceHardwareCard(ThemeData theme, dynamic device) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(Icons.phone_android, color: theme.colorScheme.primary),
                const SizedBox(width: 8),
                Text('Device Host Information', style: theme.textTheme.titleMedium),
              ],
            ),
            const Divider(),
            _infoRow('Model', '${device.manufacturer} ${device.model}'),
            _infoRow('Android Platform', 'API level ${device.sdkInt}'),
            _infoRow('Device Fingerprint', device.deviceId),
          ],
        ),
      ),
    );
  }

  Widget _metricBox(ThemeData theme, String label, String value, IconData icon, Color iconColor) {
    return Expanded(
      child: Container(
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.4),
          borderRadius: BorderRadius.circular(8),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(icon, size: 18, color: iconColor),
                const SizedBox(width: 6),
                Expanded(
                  child: Text(
                    label,
                    style: theme.textTheme.labelMedium?.copyWith(
                      color: theme.colorScheme.onSurfaceVariant,
                    ),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 6),
            Text(
              value,
              style: theme.textTheme.headlineSmall?.copyWith(
                fontWeight: FontWeight.bold,
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _infoRow(String label, String value, {Color? valueColor}) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4.0),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Flexible(
            flex: 2,
            child: Text(label, style: const TextStyle(fontWeight: FontWeight.w500)),
          ),
          const SizedBox(width: 8),
          Flexible(
            flex: 3,
            child: Text(
              value,
              textAlign: TextAlign.end,
              style: TextStyle(
                fontWeight: FontWeight.bold,
                color: valueColor,
              ),
            ),
          ),
        ],
      ),
    );
  }
}
