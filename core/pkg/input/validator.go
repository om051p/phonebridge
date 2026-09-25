// Package input implements validation, rate-limiting, and dispatch for Phase 7 Remote Input.
package input

import (
	"errors"
	"fmt"
	"math"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

const (
	// MaxTextLengthBytes is the strict upper bound on TextEvent.text payload size (1024 bytes).
	MaxTextLengthBytes = 1024
)

var (
	ErrNilFrame          = errors.New("input: nil frame")
	ErrNoEvent           = errors.New("input: frame carries no event")
	ErrInvalidCoordinate = errors.New("input: coordinate must be within [0.0, 1.0] and not NaN/Inf")
	ErrTextTooLarge      = fmt.Errorf("input: text payload exceeds %d bytes", MaxTextLengthBytes)
)

// ValidateInputFrame enforces coordinate bounds, float validity, and text length limits.
func ValidateInputFrame(frame *phonebridgev1.InputFrame) error {
	if frame == nil {
		return ErrNilFrame
	}
	switch ev := frame.Event.(type) {
	case *phonebridgev1.InputFrame_Touch:
		if ev.Touch == nil {
			return ErrNoEvent
		}
		if err := validateNormalizedCoord(ev.Touch.NormalizedX, ev.Touch.NormalizedY); err != nil {
			return err
		}
	case *phonebridgev1.InputFrame_Key:
		if ev.Key == nil {
			return ErrNoEvent
		}
	case *phonebridgev1.InputFrame_Text:
		if ev.Text == nil {
			return ErrNoEvent
		}
		if len(ev.Text.Text) > MaxTextLengthBytes {
			return ErrTextTooLarge
		}
	case *phonebridgev1.InputFrame_Scroll:
		if ev.Scroll == nil {
			return ErrNoEvent
		}
		if err := validateNormalizedCoord(ev.Scroll.NormalizedX, ev.Scroll.NormalizedY); err != nil {
			return err
		}
	case *phonebridgev1.InputFrame_Action:
		if ev.Action == nil {
			return ErrNoEvent
		}
	default:
		return ErrNoEvent
	}
	return nil
}

func validateNormalizedCoord(x, y float32) error {
	fx := float64(x)
	fy := float64(y)
	if math.IsNaN(fx) || math.IsInf(fx, 0) || math.IsNaN(fy) || math.IsInf(fy, 0) {
		return ErrInvalidCoordinate
	}
	if fx < 0.0 || fx > 1.0 || fy < 0.0 || fy > 1.0 {
		return ErrInvalidCoordinate
	}
	return nil
}

// RedactedKind returns a safe, non-sensitive string label for the input frame kind
// to satisfy the Zero Logging Rule (never logs coordinates, text, or key codes).
func RedactedKind(frame *phonebridgev1.InputFrame) string {
	if frame == nil {
		return "nil"
	}
	switch ev := frame.Event.(type) {
	case *phonebridgev1.InputFrame_Touch:
		if ev.Touch == nil {
			return "touch_nil"
		}
		return fmt.Sprintf("touch_%s", ev.Touch.Action.String())
	case *phonebridgev1.InputFrame_Key:
		if ev.Key == nil {
			return "key_nil"
		}
		return fmt.Sprintf("key_%s", ev.Key.Action.String())
	case *phonebridgev1.InputFrame_Text:
		return "text_commit"
	case *phonebridgev1.InputFrame_Scroll:
		return "scroll"
	case *phonebridgev1.InputFrame_Action:
		if ev.Action == nil {
			return "action_nil"
		}
		return fmt.Sprintf("global_action_%s", ev.Action.Type.String())
	default:
		return "unknown"
	}
}
