// Runtime screen-capability presentation (Phase B follow-up).
//
// Detection stays where it lives: the Android encoder bounds are read by
// Kotlin `CodecSelector` (served on the wire by the signaling server and on
// the channel by `getMediaCapabilities`), and the Linux side learns them from
// the DEC-022 offer exchange (retained by the Go session, served on
// `GetSessionStateResponse.capabilities`). This file only decides how a
// preset READS against an advertisement — it detects nothing.
//
// Rules mirror the negotiation contract (`SessionNegotiation` / Go `engine`):
//   * no advertisement → unknown ("Checking…"), never a guess;
//   * `supportsScreen == false` → every preset unsupported;
//   * geometry is consent-bound: a preset exceeding a STATED bound is
//     unsupported. A zero bound means "no stated limit", never "unsupported";
//   * frame rate and bitrate are advisory (DEC-020): they never gate, except
//     above the contract maximum (240 fps), which the peer always refuses;
//   * codec gates only when the advertisement names codecs without H.264.
// A passing gate means "worth requesting" — the daemon's typed answer
// (requested vs `actual`) remains the final verdict.

import '../generated/phonebridge/v1/phonebridge.pb.dart' as pb;

/// Highest frame rate the session contract will even consider
/// (mirrors `SessionNegotiation.MAX_REASONABLE_FPS`).
const int kMaxReasonableFps = 240;

/// One screen-stream request preset shown in the UI.
class ScreenPreset {
  const ScreenPreset({
    required this.width,
    required this.height,
    required this.fps,
    required this.bitrateKbps,
    required this.title,
    required this.subtitle,
    required this.guidance,
  });

  final int width;
  final int height;
  final int fps;
  final int bitrateKbps;
  final String title;
  final String subtitle;

  /// Static guidance shown when the capability gate passes
  /// (e.g. "Recommended"). Never a capability verdict by itself.
  final String guidance;

  /// True for the all-zero "device default" request (DEC-022).
  bool get isDeviceDefault =>
      width <= 0 && height <= 0 && fps <= 0 && bitrateKbps <= 0;
}

/// The presets the UI may offer, in display order. Sourced here so every
/// surface offers the same tuples; the Screen tab filters the device-default
/// entry on Android (the capture pipeline is configured locally there).
const List<ScreenPreset> kScreenPresets = [
  ScreenPreset(
    width: 0,
    height: 0,
    fps: 0,
    bitrateKbps: 0,
    title: 'Device Default (Phone Chooses)',
    subtitle: 'No requested tuple · the phone reports what it applies',
    guidance: 'Recommended',
  ),
  ScreenPreset(
    width: 720,
    height: 1600,
    fps: 30,
    bitrateKbps: 2500,
    title: '720p HD (Balanced)',
    subtitle: '720x1600 @ 30 fps · Low latency, optimal stability',
    guidance: 'Recommended',
  ),
  ScreenPreset(
    width: 1080,
    height: 2400,
    fps: 30,
    bitrateKbps: 5000,
    title: '1080p FHD (Sharp Text)',
    subtitle: '1080x2400 @ 30 fps · Crisp details, device-dependent',
    guidance: 'Available',
  ),
  ScreenPreset(
    width: 1080,
    height: 2400,
    fps: 60,
    bitrateKbps: 8000,
    title: '1080p High Frame Rate',
    subtitle: '1080x2400 @ 60 fps · Only if the device encoder supports it',
    guidance: 'Device-dependent',
  ),
];

/// Whether a preset may honestly be offered against an advertisement.
enum CapabilityAvailability { supported, unsupported, unknown }

/// Why a preset is (un)available. Empty when supported.
String availabilityReason(
  ScreenPreset preset,
  pb.MediaCapabilities? caps,
) {
  if (caps == null) return 'Capability report not available yet';
  if (!caps.supportsScreen) return 'This device cannot capture its screen';
  if (preset.isDeviceDefault) return '';
  if (caps.maxWidth > 0 && preset.width > caps.maxWidth) {
    return 'Hardware encoder tops out at ${caps.maxWidth}px wide';
  }
  if (caps.maxHeight > 0 && preset.height > caps.maxHeight) {
    return 'Hardware encoder tops out at ${caps.maxHeight}px tall';
  }
  if (preset.fps > kMaxReasonableFps) {
    return 'Exceeds the session maximum of $kMaxReasonableFps fps';
  }
  if (caps.codecs.isNotEmpty &&
      caps.codecs.every((c) => _normalizeCodec(c) != 'h264')) {
    return 'Device does not encode H.264';
  }
  return '';
}

/// Gates one preset against an advertisement. Zero bounds are "no stated
/// limit" and pass — mirroring the negotiation, which accepts geometry the
/// peer stated no limit for. The daemon's typed answer stays the final
/// verdict.
CapabilityAvailability availabilityOf(
  ScreenPreset preset,
  pb.MediaCapabilities? caps,
) {
  if (caps == null) return CapabilityAvailability.unknown;
  return availabilityReason(preset, caps).isEmpty
      ? CapabilityAvailability.supported
      : CapabilityAvailability.unsupported;
}

String _normalizeCodec(String codec) {
  final n = codec.toLowerCase().trim();
  return switch (n) {
    'video/avc' || 'avc' || 'h.264' => 'h264',
    _ => n,
  };
}

/// Field-wise equality for capability advertisements. Protobuf messages use
/// identity equality, which would treat every fresh snapshot as a change and
/// rebuild the tree on each telemetry poll.
bool mediaCapabilitiesEqual(
  pb.MediaCapabilities? a,
  pb.MediaCapabilities? b,
) {
  if (identical(a, b)) return true;
  if (a == null || b == null) return false;
  return a.supportsScreen == b.supportsScreen &&
      a.maxWidth == b.maxWidth &&
      a.maxHeight == b.maxHeight &&
      a.maxFps == b.maxFps &&
      _stringListsEqual(a.codecs, b.codecs);
}

bool _stringListsEqual(List<String> a, List<String> b) {
  if (a.length != b.length) return false;
  for (var i = 0; i < a.length; i++) {
    if (a[i] != b[i]) return false;
  }
  return true;
}
