package input_test

import (
	"math"
	"strings"
	"testing"

	"github.com/om051p/phonebridge/core/pkg/input"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

func TestValidateInputFrame_Coordinates(t *testing.T) {
	tests := []struct {
		name    string
		x, y    float32
		wantErr bool
	}{
		{"valid_center", 0.5, 0.5, false},
		{"valid_origin", 0.0, 0.0, false},
		{"valid_corner", 1.0, 1.0, false},
		{"negative_x", -0.01, 0.5, true},
		{"negative_y", 0.5, -0.01, true},
		{"oversize_x", 1.01, 0.5, true},
		{"oversize_y", 0.5, 1.01, true},
		{"nan_x", float32(math.NaN()), 0.5, true},
		{"nan_y", 0.5, float32(math.NaN()), true},
		{"inf_x", float32(math.Inf(1)), 0.5, true},
		{"inf_y", 0.5, float32(math.Inf(-1)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := &phonebridgev1.InputFrame{
				Event: &phonebridgev1.InputFrame_Touch{
					Touch: &phonebridgev1.TouchEvent{
						Action:      phonebridgev1.TouchEvent_ACTION_DOWN,
						NormalizedX: tt.x,
						NormalizedY: tt.y,
					},
				},
			}
			err := input.ValidateInputFrame(frame)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateInputFrame() err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateInputFrame_Text(t *testing.T) {
	validText := &phonebridgev1.InputFrame{
		Event: &phonebridgev1.InputFrame_Text{
			Text: &phonebridgev1.TextEvent{
				Text: "Hello, PhoneBridge!",
			},
		},
	}
	if err := input.ValidateInputFrame(validText); err != nil {
		t.Errorf("expected valid text, got %v", err)
	}

	tooLargeText := &phonebridgev1.InputFrame{
		Event: &phonebridgev1.InputFrame_Text{
			Text: &phonebridgev1.TextEvent{
				Text: strings.Repeat("a", input.MaxTextLengthBytes+1),
			},
		},
	}
	if err := input.ValidateInputFrame(tooLargeText); err == nil {
		t.Errorf("expected error for text exceeding %d bytes", input.MaxTextLengthBytes)
	}
}

func TestValidateInputFrame_Scroll(t *testing.T) {
	validScroll := &phonebridgev1.InputFrame{
		Event: &phonebridgev1.InputFrame_Scroll{
			Scroll: &phonebridgev1.ScrollEvent{
				NormalizedX: 0.5,
				NormalizedY: 0.5,
				DeltaX:      0.0,
				DeltaY:      10.0,
			},
		},
	}
	if err := input.ValidateInputFrame(validScroll); err != nil {
		t.Errorf("expected valid scroll, got %v", err)
	}

	invalidScroll := &phonebridgev1.InputFrame{
		Event: &phonebridgev1.InputFrame_Scroll{
			Scroll: &phonebridgev1.ScrollEvent{
				NormalizedX: 1.5,
				NormalizedY: 0.5,
			},
		},
	}
	if err := input.ValidateInputFrame(invalidScroll); err == nil {
		t.Errorf("expected invalid coordinate error for scroll")
	}
}

func TestRedactedKind(t *testing.T) {
	frame := &phonebridgev1.InputFrame{
		Event: &phonebridgev1.InputFrame_Text{
			Text: &phonebridgev1.TextEvent{Text: "SECRET_PASSWORD_123"},
		},
	}
	kind := input.RedactedKind(frame)
	if strings.Contains(kind, "SECRET") || strings.Contains(kind, "PASSWORD") {
		t.Errorf("RedactedKind leaked sensitive text: %s", kind)
	}
	if kind != "text_commit" {
		t.Errorf("expected 'text_commit', got '%s'", kind)
	}
}
