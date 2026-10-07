// Phase 6 Slice 3A & Phase 7 Remote Input v0.1 (DEC-027).
//
// Paints the newest decoded frame of the active session with the correct
// aspect ratio, and falls back to the caller's existing presentation state
// whenever no frame is available.
//
// Remote Input (DEC-027):
// - Intercepts pointer events (down, move, up, cancel, scroll).
// - Transforms coordinates relative to the letterboxed/pillarboxed video content rect.
// - Normalizes coordinates strictly in [0.0, 1.0].
// - Discards events falling inside letterbox/pillarbox margins.
// - ZERO-LOGGING: Never logs coordinates or text payloads.

import 'package:fixnum/fixnum.dart';
import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../generated/phonebridge/v1/phonebridge.pb.dart' as pb;
import '../services/frame_stream.dart';

class ScreenFrameView extends StatefulWidget {
  const ScreenFrameView({
    super.key,
    required this.provider,
    required this.fallback,
    this.onInput,
    this.enabled = true,
  });

  /// Structural seam: the platform service that can open the daemon's frame
  /// stream (Linux). Android does not implement it, so no view is built.
  final ProvidesFrameStream provider;

  /// Existing presentation state shown while no frame is available.
  final Widget fallback;

  /// Optional callback invoked when a normalized remote input event occurs.
  final void Function(pb.InputFrame frame)? onInput;

  /// Whether interactive remote control is currently enabled.
  final bool enabled;

  /// Normalizes an interaction coordinate inside a letterboxed/pillarboxed container.
  /// Returns null if [localPosition] falls in letterbox/pillarbox bars.
  static Offset? normalizeCoordinate(
    Offset localPosition,
    Size containerSize,
    Size frameSize,
  ) {
    if (containerSize.width <= 0 ||
        containerSize.height <= 0 ||
        frameSize.width <= 0 ||
        frameSize.height <= 0) {
      return null;
    }
    final fitted = applyBoxFit(BoxFit.contain, frameSize, containerSize);
    final dest = fitted.destination;
    final left = (containerSize.width - dest.width) / 2.0;
    final top = (containerSize.height - dest.height) / 2.0;
    final rect = Rect.fromLTWH(left, top, dest.width, dest.height);

    if (!rect.contains(localPosition)) {
      return null; // Outside active video image
    }

    final normX = ((localPosition.dx - rect.left) / rect.width).clamp(0.0, 1.0);
    final normY = ((localPosition.dy - rect.top) / rect.height).clamp(0.0, 1.0);
    return Offset(normX, normY);
  }

  @override
  State<ScreenFrameView> createState() => _ScreenFrameViewState();
}

class _ScreenFrameViewState extends State<ScreenFrameView> {
  late final FrameStream _frames;
  final FocusNode _focusNode = FocusNode();
  bool _isRightClick = false;

  @override
  void initState() {
    super.initState();
    _frames = widget.provider.createFrameStream();
    _frames.start();
  }

  @override
  void dispose() {
    _focusNode.dispose();
    _frames.dispose();
    super.dispose();
  }

  void _handlePointer(
    PointerEvent event,
    Size containerSize,
    Size frameSize,
    pb.TouchEvent_Action action,
  ) {
    if (!widget.enabled || widget.onInput == null) return;

    if (action == pb.TouchEvent_Action.ACTION_DOWN) {
      _focusNode.requestFocus();
      if ((event.buttons & kSecondaryMouseButton) != 0) {
        _isRightClick = true;
        // Right click triggers Android Back immediately
        widget.onInput!(pb.InputFrame(
          timestampMs: Int64(DateTime.now().millisecondsSinceEpoch),
          action: pb.GlobalActionEvent(
            type: pb.GlobalActionEvent_Type.TYPE_GLOBAL_ACTION_BACK,
          ),
        ));
        return;
      }
      _isRightClick = false;
    } else if (action == pb.TouchEvent_Action.ACTION_MOVE && _isRightClick) {
      return;
    } else if (action == pb.TouchEvent_Action.ACTION_UP && _isRightClick) {
      _isRightClick = false;
      return;
    }

    final norm = ScreenFrameView.normalizeCoordinate(
      event.localPosition,
      containerSize,
      frameSize,
    );
    if (norm == null) return;

    final frame = pb.InputFrame(
      timestampMs: Int64(DateTime.now().millisecondsSinceEpoch),
      touch: pb.TouchEvent(
        action: action,
        pointerId: event.pointer,
        normalizedX: norm.dx,
        normalizedY: norm.dy,
        pressure: event.pressure > 0 ? event.pressure : 1.0,
      ),
    );
    widget.onInput!(frame);
  }

  void _handleScroll(
    PointerScrollEvent event,
    Size containerSize,
    Size frameSize,
  ) {
    if (!widget.enabled || widget.onInput == null) return;
    final norm = ScreenFrameView.normalizeCoordinate(
      event.localPosition,
      containerSize,
      frameSize,
    );
    if (norm == null) return;

    final fitted = applyBoxFit(BoxFit.contain, frameSize, containerSize);
    final dest = fitted.destination;
    final deltaX = dest.width > 0 ? (event.scrollDelta.dx / dest.width) : 0.0;
    final deltaY = dest.height > 0 ? (event.scrollDelta.dy / dest.height) : 0.0;

    final frame = pb.InputFrame(
      timestampMs: Int64(DateTime.now().millisecondsSinceEpoch),
      scroll: pb.ScrollEvent(
        normalizedX: norm.dx,
        normalizedY: norm.dy,
        deltaX: deltaX,
        deltaY: deltaY,
      ),
    );
    widget.onInput!(frame);
  }

  KeyEventResult _handleKeyEvent(FocusNode node, KeyEvent event) {
    if (!widget.enabled || widget.onInput == null) return KeyEventResult.ignored;
    if (event is! KeyDownEvent) return KeyEventResult.ignored;

    // Special keys mapping
    if (event.logicalKey == LogicalKeyboardKey.escape ||
        event.logicalKey == LogicalKeyboardKey.browserBack ||
        event.logicalKey == LogicalKeyboardKey.goBack) {
      widget.onInput!(pb.InputFrame(
        timestampMs: Int64(DateTime.now().millisecondsSinceEpoch),
        action: pb.GlobalActionEvent(
          type: pb.GlobalActionEvent_Type.TYPE_GLOBAL_ACTION_BACK,
        ),
      ));
      return KeyEventResult.handled;
    }

    if (event.logicalKey == LogicalKeyboardKey.home) {
      widget.onInput!(pb.InputFrame(
        timestampMs: Int64(DateTime.now().millisecondsSinceEpoch),
        action: pb.GlobalActionEvent(
          type: pb.GlobalActionEvent_Type.TYPE_GLOBAL_ACTION_HOME,
        ),
      ));
      return KeyEventResult.handled;
    }

    if (event.logicalKey == LogicalKeyboardKey.backspace) {
      widget.onInput!(pb.InputFrame(
        timestampMs: Int64(DateTime.now().millisecondsSinceEpoch),
        key: pb.KeyEvent(
          action: pb.KeyEvent_Action.ACTION_DOWN,
          keyCode: 67, // KEYCODE_DEL
        ),
      ));
      return KeyEventResult.handled;
    }

    if (event.logicalKey == LogicalKeyboardKey.enter ||
        event.logicalKey == LogicalKeyboardKey.numpadEnter) {
      widget.onInput!(pb.InputFrame(
        timestampMs: Int64(DateTime.now().millisecondsSinceEpoch),
        key: pb.KeyEvent(
          action: pb.KeyEvent_Action.ACTION_DOWN,
          keyCode: 66, // KEYCODE_ENTER
        ),
      ));
      return KeyEventResult.handled;
    }

    // Direct printable characters text entry
    final char = event.character;
    if (char != null && char.isNotEmpty && !char.contains(RegExp(r'[\x00-\x1F\x7F]'))) {
      widget.onInput!(pb.InputFrame(
        timestampMs: Int64(DateTime.now().millisecondsSinceEpoch),
        text: pb.TextEvent(text: char),
      ));
      return KeyEventResult.handled;
    }

    return KeyEventResult.ignored;
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

        return LayoutBuilder(
          builder: (context, constraints) {
            final containerSize = Size(constraints.maxWidth, constraints.maxHeight);
            final frameSize = Size(image.width.toDouble(), image.height.toDouble());

            return Focus(
              focusNode: _focusNode,
              autofocus: true,
              onKeyEvent: _handleKeyEvent,
              child: GestureDetector(
                behavior: HitTestBehavior.opaque,
                onPanDown: (_) {},
                onPanStart: (_) {},
                onPanUpdate: (_) {},
                onPanEnd: (_) {},
                child: Listener(
                  behavior: HitTestBehavior.opaque,
                  onPointerDown: (e) => _handlePointer(e, containerSize, frameSize, pb.TouchEvent_Action.ACTION_DOWN),
                  onPointerMove: (e) => _handlePointer(e, containerSize, frameSize, pb.TouchEvent_Action.ACTION_MOVE),
                  onPointerUp: (e) => _handlePointer(e, containerSize, frameSize, pb.TouchEvent_Action.ACTION_UP),
                  onPointerCancel: (e) => _handlePointer(e, containerSize, frameSize, pb.TouchEvent_Action.ACTION_CANCEL),
                  onPointerSignal: (signal) {
                    if (signal is PointerScrollEvent) {
                      _handleScroll(signal, containerSize, frameSize);
                    }
                  },
                  child: Center(
                    child: AspectRatio(
                      aspectRatio: _frames.aspectRatio,
                      child: RawImage(
                        image: image,
                        fit: BoxFit.fill,
                      ),
                    ),
                  ),
                ),
              ),
            );
          },
        );
      },
    );
  }
}
