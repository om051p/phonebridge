// Phase 6 Slice 3A: the smallest production mirror surface.
//
// Paints the newest decoded frame of the active session with the correct
// aspect ratio, and falls back to the caller's existing presentation state
// whenever no frame is available (before the first frame, after a stop, or on
// a host without a frame stream).
//
// Ownership: the view owns its [FrameStream] — it starts the dedicated
// frame-stream subscription on mount and stops/disposes it on unmount, which
// is the "clean unsubscribe on stop/dispose" requirement. It holds no session
// state of its own (the shared [SessionController] model remains the single
// source of truth), so the fallback widget is simply the existing
// presentation rendered by the screen.

import 'package:flutter/material.dart';

import '../services/frame_stream.dart';

class ScreenFrameView extends StatefulWidget {
  const ScreenFrameView({
    super.key,
    required this.provider,
    required this.fallback,
  });

  /// Structural seam: the platform service that can open the daemon's frame
  /// stream (Linux). Android does not implement it, so no view is built.
  final ProvidesFrameStream provider;

  /// Existing presentation state shown while no frame is available.
  final Widget fallback;

  @override
  State<ScreenFrameView> createState() => _ScreenFrameViewState();
}

class _ScreenFrameViewState extends State<ScreenFrameView> {
  late final FrameStream _frames;

  @override
  void initState() {
    super.initState();
    _frames = widget.provider.createFrameStream();
    _frames.start();
  }

  @override
  void dispose() {
    // Unsubscribes from the daemon stream and disposes the decoded image:
    // no stale frame can outlive this surface (or the session that fed it).
    _frames.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: _frames,
      builder: (context, _) {
        final image = _frames.image;
        if (image == null || _frames.aspectRatio <= 0) {
          return widget.fallback;
        }
        // Correct aspect ratio by construction: the box matches the frame's
        // own dimensions, so the paint can never stretch the phone screen.
        return Center(
          child: AspectRatio(
            aspectRatio: _frames.aspectRatio,
            child: RawImage(
              image: image,
              fit: BoxFit.fill,
            ),
          ),
        );
      },
    );
  }
}
