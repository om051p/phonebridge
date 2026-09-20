import 'dart:async';
import 'dart:io';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import '../models/device_state.dart';
import '../models/capture_stats.dart';
import '../services/phonebridge_channel.dart';
import '../services/local_ipc_client.dart';
import '../generated/phonebridge/localipc/v1/local_ipc.pb.dart';

class DashboardScreen extends StatefulWidget {
  const DashboardScreen({super.key});

  @override
  State<DashboardScreen> createState() => _DashboardScreenState();
}

class _DashboardScreenState extends State<DashboardScreen> {
  final PhoneBridgeChannel _channel = PhoneBridgeChannel();
  final TextEditingController _receiverController =
      TextEditingController(text: 'http://127.0.0.1:7804');

  DeviceState _deviceState = DeviceState.initial;
  CaptureStats _stats = CaptureStats.initial;
  StreamSubscription<CaptureStats>? _statsSub;

  bool _isLoading = false;
  int _selectedWidth = 720;
  int _selectedHeight = 1600;
  int _selectedFps = 30;
  final int _selectedBitrateKbps = 2500;

  // Local IPC state (DEC-018)
  LocalIpcClient? _ipcClient;
  LocalIpcState _ipcState = LocalIpcState.disconnected;
  String? _ipcServerVersion;
  int? _ipcNegotiatedVersion;
  int? _ipcGeneration;
  int? _lastPingLatencyMs;
  int _receivedEventCount = 0;
  StreamSubscription<StreamEventsResponse>? _eventStreamSub;

  @override
  void initState() {
    super.initState();
    _refreshDeviceState();
    _subscribeEvents();
    if (!kIsWeb && Platform.isLinux) {
      _initLocalIpc();
    }
  }

  @override
  void dispose() {
    _statsSub?.cancel();
    _eventStreamSub?.cancel();
    _ipcClient?.shutdown();
    _receiverController.dispose();
    super.dispose();
  }

  Future<void> _initLocalIpc() async {
    final client = LocalIpcClient();
    _ipcClient = client;
    client.onStateChanged.listen((st) {
      if (mounted) setState(() => _ipcState = st);
    });

    try {
      final hs = await client.handshake();
      if (mounted) {
        setState(() {
          _ipcServerVersion = hs.serverVersion;
          _ipcNegotiatedVersion = hs.negotiatedVersion;
          _ipcGeneration = hs.daemonGeneration.toInt();
        });
      }
      _subscribeIpcEvents();
    } catch (e) {
      // Local daemon may not be active yet; user can retry or check health
    }
  }

  void _subscribeIpcEvents() {
    _eventStreamSub?.cancel();
    _eventStreamSub = _ipcClient?.streamEvents().listen(
      (event) {
        if (mounted) {
          setState(() {
            _receivedEventCount++;
          });
        }
      },
      onError: (_) {},
    );
  }

  Future<void> _pingDaemon() async {
    if (_ipcClient == null) return;
    final sw = Stopwatch()..start();
    try {
      await _ipcClient!.ping(nonce: DateTime.now().millisecondsSinceEpoch);
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
      if (mounted) _showError('Ping failed: $e');
    }
  }

  Future<void> _checkDaemonHealth() async {
    if (_ipcClient == null) return;
    try {
      final h = await _ipcClient!.health();
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
      if (mounted) _showError('Health check failed: $e');
    }
  }

  Future<void> _refreshDeviceState() async {
    final state = await _channel.getDeviceState();
    if (mounted) {
      setState(() {
        _deviceState = state;
      });
    }
  }

  void _subscribeEvents() {
    _statsSub = _channel.statsStream.listen(
      (stats) {
        if (!mounted) return;
        setState(() {
          _stats = stats;
        });
        if (stats.lastError != null && stats.lastError!.isNotEmpty) {
          _showError(stats.lastError!);
        }
      },
      onError: (err) {
        if (!mounted) return;
        _showError('Telemetry stream error: $err');
      },
    );
  }

  Future<void> _startCapture() async {
    setState(() => _isLoading = true);
    try {
      final success = await _channel.startCapture(
        receiverUrl: _receiverController.text.trim(),
        width: _selectedWidth,
        height: _selectedHeight,
        fps: _selectedFps,
        bitrateKbps: _selectedBitrateKbps,
      );
      if (!success && mounted) {
        _showError('Failed to initiate capture request');
      }
      await _refreshDeviceState();
    } catch (e) {
      if (mounted) {
        _showError('Start capture error: $e');
      }
    } finally {
      if (mounted) {
        setState(() => _isLoading = false);
      }
    }
  }

  Future<void> _stopCapture() async {
    setState(() => _isLoading = true);
    try {
      await _channel.stopCapture();
      await _refreshDeviceState();
      final freshStats = await _channel.getMediaStats();
      if (mounted) {
        setState(() {
          _stats = freshStats;
        });
      }
    } catch (e) {
      if (mounted) {
        _showError('Stop capture error: $e');
      }
    } finally {
      if (mounted) {
        setState(() => _isLoading = false);
      }
    }
  }

  void _showError(String message) {
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(message),
        backgroundColor: Theme.of(context).colorScheme.error,
        behavior: SnackBarBehavior.floating,
      ),
    );
  }

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
    final isCapturing = _stats.isCapturing;

    return Scaffold(
      appBar: AppBar(
        title: const Text('PhoneBridge Control'),
        actions: [
          IconButton(
            icon: const Icon(Icons.refresh),
            tooltip: 'Refresh Device State',
            onPressed: () {
              _refreshDeviceState();
              _channel.getMediaStats().then((s) {
                if (mounted) setState(() => _stats = s);
              });
            },
          ),
        ],
      ),
      body: ListView(
        padding: const EdgeInsets.all(16.0),
        children: [
          _buildStatusBanner(theme, isCapturing),
          if (!kIsWeb && Platform.isLinux) ...[
            const SizedBox(height: 16),
            _buildLocalIpcCard(theme),
          ],
          const SizedBox(height: 16),
          _buildDeviceStateCard(theme),
          const SizedBox(height: 16),
          _buildControlCard(theme, isCapturing),
          const SizedBox(height: 16),
          _buildTelemetryCard(theme, isCapturing),
        ],
      ),
    );
  }

  Widget _buildStatusBanner(ThemeData theme, bool isCapturing) {
    final color = isCapturing ? Colors.green : Colors.grey;
    final label = isCapturing ? 'STREAMING ACTIVE' : 'IDLE';

    return Card(
      elevation: 0,
      color: color.withValues(alpha: 0.12),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(12),
        side: BorderSide(color: color.withValues(alpha: 0.4)),
      ),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
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
            Text(
              label,
              style: theme.textTheme.titleMedium?.copyWith(
                fontWeight: FontWeight.bold,
                color: isCapturing ? Colors.green.shade700 : Colors.grey.shade700,
              ),
            ),
            const Spacer(),
            if (isCapturing)
              Text(
                'Duration: ${_formatDuration(_stats.durationUs)}',
                style: theme.textTheme.bodyMedium?.copyWith(
                  fontWeight: FontWeight.w600,
                ),
              ),
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
                Text('Local Engine IPC (DEC-018)', style: theme.textTheme.titleMedium),
                const Spacer(),
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
            _infoRow('Daemon Version', _ipcServerVersion ?? 'Not connected'),
            _infoRow(
              'Negotiated Version',
              _ipcNegotiatedVersion != null ? 'v$_ipcNegotiatedVersion' : '—',
            ),
            _infoRow(
              'Daemon Generation',
              _ipcGeneration != null ? '$_ipcGeneration' : '—',
            ),
            _infoRow(
              'Relayed Events',
              '$_receivedEventCount messages',
            ),
            _infoRow(
              'Ping Latency',
              _lastPingLatencyMs != null ? '$_lastPingLatencyMs ms' : '—',
            ),
            const SizedBox(height: 12),
            Row(
              children: [
                Expanded(
                  child: OutlinedButton.icon(
                    onPressed: _ipcClient != null ? _pingDaemon : null,
                    icon: const Icon(Icons.network_ping, size: 18),
                    label: const Text('PING'),
                  ),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: OutlinedButton.icon(
                    onPressed: _ipcClient != null ? _checkDaemonHealth : null,
                    icon: const Icon(Icons.health_and_safety, size: 18),
                    label: const Text('HEALTH'),
                  ),
                ),
                const SizedBox(width: 8),
                IconButton.outlined(
                  tooltip: 'Reconnect IPC',
                  icon: const Icon(Icons.sync, size: 18),
                  onPressed: _initLocalIpc,
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildDeviceStateCard(ThemeData theme) {
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
                Text('Device Info', style: theme.textTheme.titleMedium),
              ],
            ),
            const Divider(),
            _infoRow('Hardware Model', '${_deviceState.manufacturer} ${_deviceState.model}'),
            _infoRow('Android SDK', 'API ${_deviceState.sdkInt}'),
            _infoRow(
              'Go Core Engine',
              _deviceState.goEngineLoaded ? 'Loaded (libphonebridge_core.so)' : 'Unavailable',
              valueColor: _deviceState.goEngineLoaded ? Colors.green : Colors.orange,
            ),
            _infoRow('Encoder', _stats.isCapturing ? _stats.codec : _deviceState.codec),
            _infoRow(
              'Hardware Accelerated',
              (_stats.isCapturing ? _stats.isHardwareCodec : _deviceState.isHardwareCodec)
                  ? 'Yes (Qualcomm HW)'
                  : 'Software',
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildControlCard(ThemeData theme, bool isCapturing) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(Icons.settings_input_antenna, color: theme.colorScheme.primary),
                const SizedBox(width: 8),
                Text('Stream Configuration', style: theme.textTheme.titleMedium),
              ],
            ),
            const Divider(),
            TextField(
              controller: _receiverController,
              decoration: const InputDecoration(
                labelText: 'Linux Receiver URL (Optional)',
                hintText: 'http://127.0.0.1:7804',
                prefixIcon: Icon(Icons.computer),
                border: OutlineInputBorder(),
                isDense: true,
              ),
              enabled: !isCapturing && !_isLoading,
            ),
            const SizedBox(height: 12),
            Row(
              children: [
                Expanded(
                  child: DropdownButtonFormField<String>(
                    isExpanded: true,
                    initialValue: '$_selectedWidth x $_selectedHeight',
                    decoration: const InputDecoration(
                      labelText: 'Resolution',
                      border: OutlineInputBorder(),
                      isDense: true,
                    ),
                    items: const [
                      DropdownMenuItem(value: '720 x 1600', child: Text('720p (DEC-020)')),
                      DropdownMenuItem(value: '1080 x 2400', child: Text('1080p (FHD+)')),
                    ],
                    onChanged: isCapturing
                        ? null
                        : (val) {
                            if (val == '720 x 1600') {
                              setState(() {
                                _selectedWidth = 720;
                                _selectedHeight = 1600;
                              });
                            } else if (val == '1080 x 2400') {
                              setState(() {
                                _selectedWidth = 1080;
                                _selectedHeight = 2400;
                              });
                            }
                          },
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: DropdownButtonFormField<int>(
                    isExpanded: true,
                    initialValue: _selectedFps,
                    decoration: const InputDecoration(
                      labelText: 'Target FPS',
                      border: OutlineInputBorder(),
                      isDense: true,
                    ),
                    items: const [
                      DropdownMenuItem(value: 30, child: Text('30 fps')),
                      DropdownMenuItem(value: 60, child: Text('60 fps')),
                    ],
                    onChanged: isCapturing
                        ? null
                        : (val) {
                            if (val != null) setState(() => _selectedFps = val);
                          },
                  ),
                ),
              ],
            ),
            const SizedBox(height: 16),
            SizedBox(
              width: double.infinity,
              height: 48,
              child: isCapturing
                  ? FilledButton.icon(
                      style: FilledButton.styleFrom(backgroundColor: theme.colorScheme.error),
                      onPressed: _isLoading ? null : _stopCapture,
                      icon: const Icon(Icons.stop),
                      label: const Text('STOP SCREEN SHARING'),
                    )
                  : FilledButton.icon(
                      onPressed: _isLoading ? null : _startCapture,
                      icon: const Icon(Icons.screen_share),
                      label: const Text('START SCREEN SHARING'),
                    ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildTelemetryCard(ThemeData theme, bool isCapturing) {
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
                Text('Live Telemetry', style: theme.textTheme.titleMedium),
              ],
            ),
            const Divider(),
            Row(
              children: [
                _metricBox(
                  theme,
                  'Current FPS',
                  _stats.currentFps != null ? _stats.currentFps!.toStringAsFixed(1) : (isCapturing ? '—' : '0.0'),
                  Icons.speed,
                  Colors.blue,
                ),
                const SizedBox(width: 12),
                _metricBox(
                  theme,
                  'Keyframes (IDR)',
                  _stats.keyframes.toString(),
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
                  _stats.encodedFrames.toString(),
                  Icons.video_library,
                  Colors.purple,
                ),
                const SizedBox(width: 12),
                _metricBox(
                  theme,
                  'Transport Admitted',
                  _stats.admittedFrames.toString(),
                  Icons.send,
                  Colors.green,
                ),
              ],
            ),
            const SizedBox(height: 12),
            _infoRow(
              'GOP Drops (Backpressure)',
              '${_stats.droppedFrames} frames',
              valueColor: _stats.droppedFrames > 0 ? Colors.orange : null,
            ),
            _infoRow('Active Codec', _stats.codec),
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
