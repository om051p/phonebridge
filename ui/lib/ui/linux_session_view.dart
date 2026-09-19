import 'dart:async';
import 'package:flutter/material.dart';
import '../services/local_ipc_client.dart';
import '../models/session_status.dart';
import '../generated/phonebridge/localipc/v1/local_ipc.pb.dart';
import '../generated/phonebridge/v1/phonebridge.pb.dart' as pb;

/// Media tuple presets the user can ask the phone for. Zero fields mean
/// "device default", which the phone reports back as unreported rather than
/// guessing (DEC-022).
class _RequestedMedia {
  const _RequestedMedia._(this.label, this.width, this.height, this.fps, this.bitrateKbps);

  final String label;
  final int width;
  final int height;
  final int fps;
  final int bitrateKbps;

  static const deviceDefault = _RequestedMedia._('Device default', 0, 0, 0, 0);
  static const hd720p30 = _RequestedMedia._('720p 30 fps', 720, 1600, 30, 4000);
  static const hd1080p30 = _RequestedMedia._('1080p 30 fps', 1080, 2400, 30, 6000);
  static const hd1080p60 = _RequestedMedia._('1080p 60 fps', 1080, 2400, 60, 8000);

  static const values = <_RequestedMedia>[deviceDefault, hd720p30, hd1080p30, hd1080p60];

  pb.MediaParams? toParams() {
    if (width == 0 && height == 0 && fps == 0 && bitrateKbps == 0) return null;
    return pb.MediaParams(
      width: width,
      height: height,
      fps: fps,
      bitrateKbps: bitrateKbps,
      codec: 'h264',
    );
  }
}

/// Linux desktop session view for PhoneBridge.
/// Connects to local daemon over UDS + gRPC to:
/// - Discover PhoneBridge devices over mDNS
/// - Initiate and stop WebRTC sessions
/// - Monitor session state machine and live stream telemetry
/// - Coordinate with Go receiver display pipeline (ffplay)
class LinuxSessionView extends StatefulWidget {
  const LinuxSessionView({
    super.key,
    this.client,
  });

  final LocalIpcClient? client;

  @override
  State<LinuxSessionView> createState() => _LinuxSessionViewState();
}

class _LinuxSessionViewState extends State<LinuxSessionView> {
  late final LocalIpcClient _ipcClient;
  bool _ownsClient = false;

  LocalIpcState _ipcState = LocalIpcState.disconnected;
  String? _serverVersion;
  int? _daemonGeneration;
  int? _lastPingLatencyMs;

  List<DiscoveredDevice> _devices = [];
  List<TrustedDevice> _trustedDevices = [];
  String? _selectedDeviceId;
  final TextEditingController _targetIdController = TextEditingController();

  /// The single source of truth for session presentation: state, typed reason,
  /// the requested tuple and the tuple the phone reported applying.
  SessionStatus _status = SessionStatus.idle;

  /// What the next session will ask the phone for. Settled before the offer is
  /// created (DEC-022), so it is chosen here rather than applied afterwards.
  _RequestedMedia _requestedMedia = _RequestedMedia.deviceDefault;

  String? get _activeSessionId => _status.sessionId;
  StreamStats? _streamStats;

  bool _isLoading = false;
  bool _isScanning = false;

  StreamSubscription<LocalIpcState>? _stateSub;
  StreamSubscription<SessionEvent>? _sessionEventSub;
  Timer? _scanTimer;
  Timer? _telemetryTimer;

  @override
  void initState() {
    super.initState();
    if (widget.client != null) {
      _ipcClient = widget.client!;
    } else {
      _ipcClient = LocalIpcClient();
      _ownsClient = true;
    }

    _stateSub = _ipcClient.onStateChanged.listen((st) {
      if (mounted) setState(() => _ipcState = st);
    });

    _initIpc();
  }

  @override
  void dispose() {
    _scanTimer?.cancel();
    _telemetryTimer?.cancel();
    _stateSub?.cancel();
    _sessionEventSub?.cancel();
    _targetIdController.dispose();
    if (_ownsClient) {
      _ipcClient.shutdown();
    }
    super.dispose();
  }

  Future<void> _initIpc() async {
    setState(() => _isLoading = true);
    try {
      final hs = await _ipcClient.handshake();
      if (mounted) {
        setState(() {
          _serverVersion = hs.serverVersion;
          _daemonGeneration = hs.daemonGeneration.toInt();
          _ipcState = LocalIpcState.connected;
        });
      }
      _subscribeSessionEvents();
      await _refreshDevices();
      await _refreshTrustedDevices();
      _startPeriodicScan();
    } catch (e) {
      if (mounted) {
        setState(() => _ipcState = LocalIpcState.disconnected);
      }
    } finally {
      if (mounted) {
        setState(() => _isLoading = false);
      }
    }
  }

  void _subscribeSessionEvents() {
    _sessionEventSub?.cancel();
    _sessionEventSub = _ipcClient.onSessionEvents.listen(
      (evt) {
        if (!mounted) return;
        final next = SessionStatus.fromEvent(evt, previous: _status);
        setState(() => _status = next);

        // A reconnect keeps the same session (the daemon preserves its ID) and
        // only replaces the transport, so telemetry keeps polling through it:
        // dropping it here would blank the panel during exactly the moment the
        // user wants to watch it recover.
        if (next.shouldKeepTelemetry) {
          _startTelemetryPolling();
        } else if (next.isTerminal) {
          _stopTelemetryPolling();
        }
      },
      onError: (e) {
        // Event stream reconnection is handled by LocalIpcClient
      },
    );
  }

  void _startPeriodicScan() {
    _scanTimer?.cancel();
    _scanTimer = Timer.periodic(const Duration(seconds: 4), (_) {
      if (_ipcState == LocalIpcState.connected) {
        _refreshDevices(silent: true);
      }
    });
  }

  void _startTelemetryPolling() {
    _telemetryTimer?.cancel();
    _telemetryTimer = Timer.periodic(const Duration(seconds: 1), (_) async {
      if (_activeSessionId == null || _activeSessionId!.isEmpty) return;
      try {
        final snap = await _ipcClient.getSessionState(sessionId: _activeSessionId!);
        if (mounted) {
          setState(() {
            // The snapshot is authoritative for the negotiation, so it replaces
            // the previous view of it rather than merging into it.
            _status = SessionStatus.fromSnapshot(snap, previous: _status);
            if (snap.hasStats()) {
              _streamStats = snap.stats;
            }
          });
          if (!_status.shouldKeepTelemetry) {
            _stopTelemetryPolling();
          }
        }
      } catch (_) {}
    });
  }

  void _stopTelemetryPolling() {
    _telemetryTimer?.cancel();
    _telemetryTimer = null;
  }

  Future<void> _refreshDevices({bool silent = false}) async {
    if (!silent && mounted) setState(() => _isScanning = true);
    try {
      final resp = await _ipcClient.listDevices();
      if (mounted) {
        setState(() {
          _devices = resp.devices;
          if (_selectedDeviceId == null && _devices.isNotEmpty) {
            final active = _devices.where((d) => !d.isStale).toList();
            if (active.isNotEmpty) {
              _selectedDeviceId = active.first.id;
              _targetIdController.text = active.first.id;
            } else {
              _selectedDeviceId = _devices.first.id;
              _targetIdController.text = _devices.first.id;
            }
          }
        });
      }
    } catch (e) {
      if (!silent && mounted) {
        _showError('Device discovery scan failed: $e');
      }
    } finally {
      if (!silent && mounted) setState(() => _isScanning = false);
    }
  }

  Future<void> _refreshTrustedDevices() async {
    try {
      final resp = await _ipcClient.listTrustedDevices();
      if (mounted) {
        setState(() {
          _trustedDevices = resp.devices;
        });
      }
    } catch (_) {}
  }

  bool _isDeviceTrusted(String deviceId) {
    final matches = _trustedDevices.where((d) => d.deviceId == deviceId).toList();
    return matches.isNotEmpty && !matches.first.revoked;
  }

  Future<void> _startPairing(String deviceId) async {
    if (deviceId.isEmpty) {
      _showError('Device ID is required for pairing');
      return;
    }
    setState(() => _isLoading = true);
    try {
      final pairResp = await _ipcClient.pairDevice(deviceId);
      if (!mounted) return;
      setState(() => _isLoading = false);

      final confirmed = await showDialog<bool>(
        context: context,
        barrierDismissible: false,
        builder: (ctx) => AlertDialog(
          title: const Row(
            children: [
              Icon(Icons.security, color: Colors.blue),
              SizedBox(width: 8),
              Text('Device Pairing (SAS)'),
            ],
          ),
          content: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                'Pairing with: ${pairResp.displayName.isNotEmpty ? pairResp.displayName : pairResp.deviceId}',
                style: const TextStyle(fontWeight: FontWeight.w600),
              ),
              const SizedBox(height: 16),
              const Text(
                'Compare this 6-digit verification code with the code shown on your phone:',
                style: TextStyle(fontSize: 13),
              ),
              const SizedBox(height: 16),
              Center(
                child: Container(
                  padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 12),
                  decoration: BoxDecoration(
                    color: Theme.of(ctx).colorScheme.primaryContainer,
                    borderRadius: BorderRadius.circular(12),
                    border: Border.all(
                      color: Theme.of(ctx).colorScheme.primary,
                      width: 2,
                    ),
                  ),
                  child: Text(
                    pairResp.sas.length == 6
                        ? '${pairResp.sas.substring(0, 3)} ${pairResp.sas.substring(3)}'
                        : pairResp.sas,
                    style: Theme.of(ctx).textTheme.headlineMedium?.copyWith(
                          fontWeight: FontWeight.bold,
                          letterSpacing: 4,
                          color: Theme.of(ctx).colorScheme.onPrimaryContainer,
                        ),
                  ),
                ),
              ),
              const SizedBox(height: 16),
              const Text(
                'Do the codes match exactly on both screens?',
                style: TextStyle(fontSize: 12, color: Colors.grey),
              ),
            ],
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(ctx).pop(false),
              child: const Text('REJECT / CANCEL'),
            ),
            FilledButton(
              onPressed: () => Navigator.of(ctx).pop(true),
              child: const Text('CONFIRM & TRUST'),
            ),
          ],
        ),
      );

      if (confirmed == null) return;

      setState(() => _isLoading = true);
      final confResp = await _ipcClient.confirmPairing(
        deviceId: deviceId,
        userConfirmed: confirmed,
      );

      if (confResp.success) {
        await _refreshTrustedDevices();
        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(
              content: Text('Device paired and trusted: $deviceId'),
              backgroundColor: Colors.green,
            ),
          );
        }
      } else {
        _showError(confResp.errorMessage.isNotEmpty
            ? 'Pairing failed: ${confResp.errorMessage}'
            : 'Pairing rejected or cancelled');
      }
    } catch (e) {
      if (mounted) _showError('Pairing failed: $e');
    } finally {
      if (mounted) setState(() => _isLoading = false);
    }
  }

  Future<void> _revokeDevice(String deviceId) async {
    final confirm = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Revoke Trust'),
        content: Text('Are you sure you want to revoke trust for device $deviceId? You will need to pair again to connect.'),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(false),
            child: const Text('CANCEL'),
          ),
          FilledButton(
            style: FilledButton.styleFrom(backgroundColor: Theme.of(ctx).colorScheme.error),
            onPressed: () => Navigator.of(ctx).pop(true),
            child: const Text('REVOKE'),
          ),
        ],
      ),
    );

    if (confirm != true) return;

    setState(() => _isLoading = true);
    try {
      final resp = await _ipcClient.revokeDevice(deviceId);
      if (resp.success) {
        await _refreshTrustedDevices();
        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(content: Text('Revoked trust for $deviceId')),
          );
        }
      }
    } catch (e) {
      if (mounted) _showError('Revoke failed: $e');
    } finally {
      if (mounted) setState(() => _isLoading = false);
    }
  }

  Future<void> _startSession() async {
    final targetId = _targetIdController.text.trim();
    if (targetId.isEmpty) {
      _showError('Please select or enter a target device ID');
      return;
    }

    setState(() => _isLoading = true);
    try {
      final requested = _requestedMedia.toParams();
      final resp = await _ipcClient.startSession(targetId, requested: requested);
      if (mounted) {
        setState(() {
          // A new session starts from a clean slate: the previous failure and
          // negotiated tuple must not leak into it.
          _status = SessionStatus(
            state: resp.state,
            sessionId: resp.sessionId,
            reasonDetail: 'session initiated',
            requested: requested,
            actual: null,
          );
        });
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text('Session started: ${resp.sessionId}'),
            duration: const Duration(seconds: 2),
          ),
        );
      }
    } catch (e) {
      if (mounted) _showError('Start session error: $e');
    } finally {
      if (mounted) setState(() => _isLoading = false);
    }
  }

  Future<void> _stopSession() async {
    if (_activeSessionId == null || _activeSessionId!.isEmpty) return;

    setState(() => _isLoading = true);
    try {
      final resp = await _ipcClient.stopSession(
        sessionId: _activeSessionId!,
        reason: 'user requested stop from desktop UI',
      );
      if (mounted) {
        setState(() {
          _status = _status.copyWith(
            state: resp.state,
            sessionId: resp.sessionId.isNotEmpty ? resp.sessionId : _status.sessionId,
            reasonCode: SessionReason.SESSION_REASON_USER_STOPPED,
            reasonDetail: 'stopped by user',
          );
        });
        _stopTelemetryPolling();
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text('Session stopped successfully'),
            duration: Duration(seconds: 2),
          ),
        );
      }
    } catch (e) {
      if (mounted) _showError('Stop session error: $e');
    } finally {
      if (mounted) setState(() => _isLoading = false);
    }
  }

  Future<void> _pingDaemon() async {
    final sw = Stopwatch()..start();
    try {
      await _ipcClient.ping(nonce: DateTime.now().millisecondsSinceEpoch);
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

  Future<void> _checkHealth() async {
    try {
      final h = await _ipcClient.health();
      if (mounted) {
        setState(() {
          _serverVersion = h.serverVersion;
          _daemonGeneration = h.daemonGeneration.toInt();
        });
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text('Daemon ready: ${h.ready} (uptime: ${h.uptimeMs} ms)'),
            duration: const Duration(seconds: 2),
          ),
        );
      }
    } catch (e) {
      if (mounted) _showError('Health check failed: $e');
    }
  }

  void _showError(String msg) {
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(msg),
        backgroundColor: Theme.of(context).colorScheme.error,
        behavior: SnackBarBehavior.floating,
      ),
    );
  }

  String _formatDuration(int ms) {
    if (ms <= 0) return '00:00';
    final totalSec = ms ~/ 1000;
    final min = (totalSec ~/ 60).toString().padLeft(2, '0');
    final sec = (totalSec % 60).toString().padLeft(2, '0');
    return '$min:$sec';
  }

  String _formatBytes(int bytes) {
    if (bytes < 1024) return '$bytes B';
    if (bytes < 1024 * 1024) return '${(bytes / 1024).toStringAsFixed(1)} KB';
    return '${(bytes / (1024 * 1024)).toStringAsFixed(2)} MB';
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final isConnected = _ipcState == LocalIpcState.connected;
    final isSessionActive = _status.isActive;

    return Scaffold(
      appBar: AppBar(
        title: const Text('PhoneBridge Linux Desktop'),
        actions: [
          IconButton(
            icon: _isScanning
                ? const SizedBox(
                    width: 20,
                    height: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : const Icon(Icons.refresh),
            tooltip: 'Scan LAN Devices',
            onPressed: isConnected && !_isScanning ? () => _refreshDevices() : null,
          ),
          IconButton(
            icon: const Icon(Icons.network_ping),
            tooltip: 'Ping Daemon',
            onPressed: isConnected ? _pingDaemon : null,
          ),
          IconButton(
            icon: const Icon(Icons.health_and_safety),
            tooltip: 'Check Daemon Health',
            onPressed: isConnected ? _checkHealth : null,
          ),
        ],
      ),
      body: ListView(
        padding: const EdgeInsets.all(16.0),
        children: [
          _buildSessionStateBanner(theme),
          const SizedBox(height: 16),
          if (_status.state == SessionState.SESSION_STATE_STREAMING) ...[
            _buildVideoDisplayBanner(theme),
            const SizedBox(height: 16),
          ],
          _buildControlCard(theme, isConnected, isSessionActive),
          const SizedBox(height: 16),
          _buildDiscoveredDevicesCard(theme, isConnected),
          const SizedBox(height: 16),
          _buildTrustedDevicesCard(theme, isConnected),
          if (_streamStats != null || isSessionActive) ...[
            const SizedBox(height: 16),
            _buildTelemetryCard(theme),
          ],
          const SizedBox(height: 16),
          _buildDaemonStatusCard(theme),
        ],
      ),
    );
  }

  Color _stateColor(SessionState state) {
    switch (state) {
      case SessionState.SESSION_STATE_STREAMING:
        return Colors.green;
      case SessionState.SESSION_STATE_CONNECTED:
        return Colors.teal;
      case SessionState.SESSION_STATE_CONNECTING:
      case SessionState.SESSION_STATE_DISCOVERING:
        return Colors.amber;
      case SessionState.SESSION_STATE_RECONNECTING:
        return Colors.orange;
      case SessionState.SESSION_STATE_FAILED:
        return Colors.red;
      default:
        return Colors.grey;
    }
  }

  IconData _stateIcon(SessionState state) {
    switch (state) {
      case SessionState.SESSION_STATE_STREAMING:
        return Icons.play_circle_fill;
      case SessionState.SESSION_STATE_CONNECTED:
        return Icons.check_circle;
      case SessionState.SESSION_STATE_CONNECTING:
        return Icons.sync;
      case SessionState.SESSION_STATE_DISCOVERING:
        return Icons.search;
      case SessionState.SESSION_STATE_RECONNECTING:
        return Icons.autorenew;
      case SessionState.SESSION_STATE_FAILED:
        return Icons.error;
      case SessionState.SESSION_STATE_STOPPED:
        return Icons.stop_circle;
      default:
        return Icons.pause_circle_outline;
    }
  }

  Widget _buildSessionStateBanner(ThemeData theme) {
    final color = _stateColor(_status.state);
    final label = _status.label;
    final icon = _stateIcon(_status.state);
    final negotiation = _status.negotiationSummary();
    final failure = _status.failureSummary;
    final recovery = _status.recoveringDetail;
    final badge = _status.reasonBadge;

    return Card(
      elevation: 0,
      color: color.withValues(alpha: 0.12),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(12),
        side: BorderSide(color: color.withValues(alpha: 0.4)),
      ),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(icon, color: color, size: 24),
                const SizedBox(width: 12),
                Expanded(
                  child: Text(
                    label,
                    style: theme.textTheme.titleMedium?.copyWith(
                      fontWeight: FontWeight.bold,
                      color: color,
                    ),
                  ),
                ),
                if (_status.durationMs > 0)
                  Text(
                    'Duration: ${_formatDuration(_status.durationMs)}',
                    style: theme.textTheme.bodyMedium?.copyWith(
                      fontWeight: FontWeight.w600,
                    ),
                  ),
              ],
            ),
            if (_activeSessionId != null && _activeSessionId!.isNotEmpty) ...[
              const SizedBox(height: 6),
              Text(
                'Session ID: $_activeSessionId',
                style: theme.textTheme.bodySmall?.copyWith(fontFamily: 'monospace'),
              ),
            ],
            if (_status.reasonDetail.isNotEmpty) ...[
              const SizedBox(height: 4),
              Text(
                'Status detail: ${_status.reasonDetail}',
                style: theme.textTheme.bodySmall?.copyWith(color: theme.colorScheme.onSurfaceVariant),
              ),
            ],
            // Automatic recovery: presented as progress, not as an error.
            if (recovery.isNotEmpty) ...[
              const SizedBox(height: 6),
              Text(
                recovery,
                style: theme.textTheme.bodySmall?.copyWith(
                  color: color,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ],
            // What was asked for versus what the phone said it applies. The
            // requested tuple is never shown as if it had been confirmed.
            if (negotiation.isNotEmpty) ...[
              const SizedBox(height: 6),
              Text(
                negotiation,
                style: theme.textTheme.bodySmall?.copyWith(fontFamily: 'monospace'),
              ),
            ],
            if (badge.isNotEmpty) ...[
              const SizedBox(height: 6),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                decoration: BoxDecoration(
                  borderRadius: BorderRadius.circular(6),
                  border: Border.all(color: color.withValues(alpha: 0.6)),
                ),
                child: Text(
                  badge,
                  style: theme.textTheme.labelSmall?.copyWith(
                    color: color,
                    fontWeight: FontWeight.bold,
                    letterSpacing: 0.5,
                  ),
                ),
              ),
            ],
            if (failure.isNotEmpty) ...[
              const SizedBox(height: 6),
              Text(
                failure,
                style: theme.textTheme.bodySmall?.copyWith(
                  color: theme.colorScheme.error,
                  fontWeight: FontWeight.bold,
                ),
              ),
            ],
            if (_status.errorMessage.isNotEmpty) ...[
              const SizedBox(height: 4),
              Text(
                'Detail: ${_status.errorMessage}',
                style: theme.textTheme.bodySmall?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }

  Widget _buildVideoDisplayBanner(ThemeData theme) {
    return Card(
      color: theme.colorScheme.primaryContainer.withValues(alpha: 0.25),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(12),
        side: BorderSide(color: theme.colorScheme.primary.withValues(alpha: 0.4)),
      ),
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Row(
          children: [
            Icon(Icons.desktop_windows, size: 36, color: theme.colorScheme.primary),
            const SizedBox(width: 16),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'Display Sink: Active (ffplay)',
                    style: theme.textTheme.titleSmall?.copyWith(fontWeight: FontWeight.bold),
                  ),
                  const SizedBox(height: 4),
                  Text(
                    'Hardware H.264 Annex-B stream is piped to low-latency Linux display window via Receiver pipeline.',
                    style: theme.textTheme.bodySmall?.copyWith(color: theme.colorScheme.onSurfaceVariant),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildControlCard(ThemeData theme, bool isConnected, bool isSessionActive) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(Icons.tune, color: theme.colorScheme.primary),
                const SizedBox(width: 8),
                Text('Session Controls', style: theme.textTheme.titleMedium),
              ],
            ),
            const Divider(),
            TextField(
              controller: _targetIdController,
              decoration: InputDecoration(
                labelText: 'Target Device ID',
                hintText: 'e.g. android-device-1 or select below',
                prefixIcon: const Icon(Icons.phone_android),
                border: const OutlineInputBorder(),
                isDense: true,
                suffixIcon: _selectedDeviceId != null
                    ? IconButton(
                        icon: const Icon(Icons.clear, size: 18),
                        onPressed: isSessionActive
                            ? null
                            : () {
                                setState(() {
                                  _selectedDeviceId = null;
                                  _targetIdController.clear();
                                });
                              },
                      )
                    : null,
              ),
              enabled: !isSessionActive && !_isLoading && isConnected,
            ),
            const SizedBox(height: 12),
            DropdownButtonFormField<_RequestedMedia>(
              initialValue: _requestedMedia,
              decoration: const InputDecoration(
                labelText: 'Requested quality',
                helperText: 'Negotiated with the phone before the offer is created',
                prefixIcon: Icon(Icons.high_quality),
                border: OutlineInputBorder(),
                isDense: true,
              ),
              items: [
                for (final preset in _RequestedMedia.values)
                  DropdownMenuItem(value: preset, child: Text(preset.label)),
              ],
              onChanged: isSessionActive || _isLoading
                  ? null
                  : (value) {
                      if (value != null) setState(() => _requestedMedia = value);
                    },
            ),
            const SizedBox(height: 16),
            if (_targetIdController.text.trim().isNotEmpty &&
                !_isDeviceTrusted(_targetIdController.text.trim()) &&
                !isSessionActive) ...[
              Container(
                margin: const EdgeInsets.only(bottom: 12),
                padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
                decoration: BoxDecoration(
                  color: Colors.amber.withValues(alpha: 0.15),
                  borderRadius: BorderRadius.circular(8),
                  border: Border.all(color: Colors.amber.withValues(alpha: 0.5)),
                ),
                child: Row(
                  children: [
                    const Icon(Icons.lock_open, size: 18, color: Colors.amber),
                    const SizedBox(width: 8),
                    const Expanded(
                      child: Text(
                        'Device is not paired. Pair first to authenticate WebRTC streaming.',
                        style: TextStyle(fontSize: 12, color: Colors.amber),
                      ),
                    ),
                    TextButton.icon(
                      icon: const Icon(Icons.link, size: 16),
                      label: const Text('PAIR NOW'),
                      onPressed: _isLoading || !isConnected
                          ? null
                          : () => _startPairing(_targetIdController.text.trim()),
                    ),
                  ],
                ),
              ),
            ],
            SizedBox(
              width: double.infinity,
              height: 48,
              child: isSessionActive
                  ? FilledButton.icon(
                      style: FilledButton.styleFrom(
                        backgroundColor: theme.colorScheme.error,
                      ),
                      onPressed: _isLoading || !isConnected ? null : _stopSession,
                      icon: const Icon(Icons.stop),
                      label: const Text('STOP SESSION'),
                    )
                  : FilledButton.icon(
                      onPressed: _isLoading || !isConnected ? null : _startSession,
                      icon: const Icon(Icons.play_arrow),
                      label: const Text('START SESSION'),
                    ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildDiscoveredDevicesCard(ThemeData theme, bool isConnected) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(Icons.wifi_tethering, color: theme.colorScheme.primary),
                const SizedBox(width: 8),
                Text('Discovered Devices (mDNS)', style: theme.textTheme.titleMedium),
                const Spacer(),
                Text(
                  '${_devices.length} found',
                  style: theme.textTheme.bodySmall?.copyWith(color: theme.colorScheme.onSurfaceVariant),
                ),
              ],
            ),
            const Divider(),
            if (_devices.isEmpty)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 24.0),
                child: Center(
                  child: Column(
                    children: [
                      Icon(Icons.devices_other, size: 40, color: theme.colorScheme.outline),
                      const SizedBox(height: 8),
                      Text(
                        'No PhoneBridge devices discovered yet.',
                        style: theme.textTheme.bodyMedium?.copyWith(
                          color: theme.colorScheme.onSurfaceVariant,
                        ),
                      ),
                      const SizedBox(height: 4),
                      Text(
                        'Ensure the Android PhoneBridge app is advertising on the same LAN or enter device ID manually above.',
                        textAlign: TextAlign.center,
                        style: theme.textTheme.bodySmall?.copyWith(
                          color: theme.colorScheme.outline,
                        ),
                      ),
                    ],
                  ),
                ),
              )
            else
              ...List.generate(_devices.length, (index) {
                final d = _devices[index];
                final isSelected = _selectedDeviceId == d.id;
                final isStale = d.isStale;
                final isTrusted = _isDeviceTrusted(d.id);

                return Container(
                  margin: const EdgeInsets.only(bottom: 8),
                  decoration: BoxDecoration(
                    borderRadius: BorderRadius.circular(8),
                    border: Border.all(
                      color: isSelected ? theme.colorScheme.primary : theme.colorScheme.outlineVariant,
                      width: isSelected ? 2 : 1,
                    ),
                    color: isSelected
                        ? theme.colorScheme.primary.withValues(alpha: 0.08)
                        : (isStale ? theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.3) : null),
                  ),
                  child: ListTile(
                    dense: true,
                    leading: Icon(
                      Icons.smartphone,
                      color: isStale ? Colors.grey : (isSelected ? theme.colorScheme.primary : null),
                    ),
                    title: Row(
                      children: [
                        Text(
                          d.name.isNotEmpty ? d.name : d.id,
                          style: TextStyle(
                            fontWeight: FontWeight.bold,
                            color: isStale ? Colors.grey : null,
                          ),
                        ),
                        const SizedBox(width: 8),
                        if (isTrusted)
                          Container(
                            padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                            decoration: BoxDecoration(
                              color: Colors.green.withValues(alpha: 0.2),
                              borderRadius: BorderRadius.circular(4),
                            ),
                            child: const Row(
                              mainAxisSize: MainAxisSize.min,
                              children: [
                                Icon(Icons.verified, size: 12, color: Colors.green),
                                SizedBox(width: 4),
                                Text('PAIRED', style: TextStyle(fontSize: 10, fontWeight: FontWeight.bold, color: Colors.green)),
                              ],
                            ),
                          )
                        else
                          Container(
                            padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                            decoration: BoxDecoration(
                              color: Colors.amber.withValues(alpha: 0.2),
                              borderRadius: BorderRadius.circular(4),
                            ),
                            child: const Row(
                              mainAxisSize: MainAxisSize.min,
                              children: [
                                Icon(Icons.lock_open, size: 12, color: Colors.amber),
                                SizedBox(width: 4),
                                Text('UNPAIRED', style: TextStyle(fontSize: 10, fontWeight: FontWeight.bold, color: Colors.amber)),
                              ],
                            ),
                          ),
                        if (isStale) ...[
                          const SizedBox(width: 8),
                          Container(
                            padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                            decoration: BoxDecoration(
                              color: Colors.orange.withValues(alpha: 0.2),
                              borderRadius: BorderRadius.circular(4),
                            ),
                            child: const Text('STALE', style: TextStyle(fontSize: 10, color: Colors.orange)),
                          ),
                        ],
                      ],
                    ),
                    subtitle: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text('ID: ${d.id} • ${d.model} (${d.version})'),
                        if (d.address.isNotEmpty) Text('Address: ${d.address}:${d.port}'),
                      ],
                    ),
                    trailing: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        if (!isTrusted && !isStale)
                          IconButton(
                            icon: const Icon(Icons.link, size: 20),
                            tooltip: 'Pair Device',
                            onPressed: isConnected && !_isLoading ? () => _startPairing(d.id) : null,
                          ),
                        Icon(
                          isSelected ? Icons.radio_button_checked : Icons.radio_button_unchecked,
                          color: isSelected ? theme.colorScheme.primary : theme.colorScheme.outline,
                        ),
                      ],
                    ),
                    onTap: () {
                      setState(() {
                        _selectedDeviceId = d.id;
                        _targetIdController.text = d.id;
                      });
                    },
                  ),
                );
              }),
          ],
        ),
      ),
    );
  }

  Widget _buildTrustedDevicesCard(ThemeData theme, bool isConnected) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(Icons.verified_user, color: theme.colorScheme.primary),
                const SizedBox(width: 8),
                Text('Trusted Devices (Trust Store)', style: theme.textTheme.titleMedium),
                const Spacer(),
                Text(
                  '${_trustedDevices.length} trusted',
                  style: theme.textTheme.bodySmall?.copyWith(color: theme.colorScheme.onSurfaceVariant),
                ),
              ],
            ),
            const Divider(),
            if (_trustedDevices.isEmpty)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 16.0),
                child: Center(
                  child: Text(
                    'No paired devices. Pair with a LAN device to establish cryptographic trust.',
                    style: theme.textTheme.bodyMedium?.copyWith(
                      color: theme.colorScheme.onSurfaceVariant,
                    ),
                  ),
                ),
              )
            else
              ...List.generate(_trustedDevices.length, (index) {
                final dev = _trustedDevices[index];
                return ListTile(
                  dense: true,
                  leading: Icon(
                    dev.revoked ? Icons.block : Icons.lock,
                    color: dev.revoked ? Colors.red : Colors.green,
                  ),
                  title: Row(
                    children: [
                      Text(
                        dev.displayName.isNotEmpty ? dev.displayName : dev.deviceId,
                        style: const TextStyle(fontWeight: FontWeight.bold),
                      ),
                      const SizedBox(width: 8),
                      Container(
                        padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                        decoration: BoxDecoration(
                          color: dev.revoked
                              ? Colors.red.withValues(alpha: 0.2)
                              : Colors.green.withValues(alpha: 0.2),
                          borderRadius: BorderRadius.circular(4),
                        ),
                        child: Text(
                          dev.revoked ? 'REVOKED' : 'TRUSTED',
                          style: TextStyle(
                            fontSize: 10,
                            fontWeight: FontWeight.bold,
                            color: dev.revoked ? Colors.red : Colors.green,
                          ),
                        ),
                      ),
                    ],
                  ),
                  subtitle: Text('ID: ${dev.deviceId} • Platform: ${dev.platform}'),
                  trailing: dev.revoked
                      ? null
                      : TextButton.icon(
                          icon: const Icon(Icons.delete_outline, size: 16, color: Colors.red),
                          label: const Text('REVOKE', style: TextStyle(color: Colors.red, fontSize: 12)),
                          onPressed: isConnected && !_isLoading ? () => _revokeDevice(dev.deviceId) : null,
                        ),
                );
              }),
          ],
        ),
      ),
    );
  }

  Widget _buildTelemetryCard(ThemeData theme) {
    final s = _streamStats;

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
                Text('Live Stream Telemetry', style: theme.textTheme.titleMedium),
              ],
            ),
            const Divider(),
            Row(
              children: [
                _metricBox(
                  theme,
                  'RTP Packets',
                  s != null ? s.packets.toString() : '0',
                  Icons.send,
                  Colors.blue,
                ),
                const SizedBox(width: 12),
                _metricBox(
                  theme,
                  'Access Units',
                  s != null ? s.accessUnits.toString() : '0',
                  Icons.video_library,
                  Colors.purple,
                ),
              ],
            ),
            const SizedBox(height: 12),
            Row(
              children: [
                _metricBox(
                  theme,
                  'H.264 Data',
                  s != null ? _formatBytes(s.bytesH264.toInt()) : '0 B',
                  Icons.data_usage,
                  Colors.green,
                ),
                const SizedBox(width: 12),
                _metricBox(
                  theme,
                  'Keyframes (IDR)',
                  s != null ? s.keyframes.toString() : '0',
                  Icons.key,
                  Colors.amber,
                ),
              ],
            ),
            const SizedBox(height: 12),
            _infoRow(
              'Dropped AUs',
              '${s?.droppedAus ?? 0}',
              valueColor: (s?.droppedAus.toInt() ?? 0) > 0 ? Colors.orange : null,
            ),
            _infoRow(
              'RTP Total Payload',
              s != null ? _formatBytes(s.bytesRtp.toInt()) : '0 B',
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildDaemonStatusCard(ThemeData theme) {
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
            _infoRow('Socket Path', _ipcClient.socketPath),
            _infoRow('Daemon Version', _serverVersion ?? 'Unavailable'),
            _infoRow('Daemon Generation', _daemonGeneration != null ? '$_daemonGeneration' : '—'),
            _infoRow('Ping Round-Trip', _lastPingLatencyMs != null ? '$_lastPingLatencyMs ms' : '—'),
            const SizedBox(height: 12),
            Row(
              children: [
                Expanded(
                  child: OutlinedButton.icon(
                    onPressed: _isLoading ? null : _initIpc,
                    icon: const Icon(Icons.sync, size: 18),
                    label: const Text('RECONNECT DAEMON'),
                  ),
                ),
              ],
            ),
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
