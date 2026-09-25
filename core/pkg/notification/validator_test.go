package notification

import (
	"strings"
	"testing"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

func TestValidateNotificationFrame(t *testing.T) {
	if err := ValidateNotificationFrame(nil); err != ErrNilFrame {
		t.Fatalf("expected ErrNilFrame, got %v", err)
	}

	emptyFrame := &phonebridgev1.NotificationFrame{}
	if err := ValidateNotificationFrame(emptyFrame); err != ErrNoEvent {
		t.Fatalf("expected ErrNoEvent, got %v", err)
	}

	validPosted := &phonebridgev1.NotificationFrame{
		Version:     1,
		TimestampMs: 12345678,
		Event: &phonebridgev1.NotificationFrame_Posted{
			Posted: &phonebridgev1.NotificationPosted{
				Key:         "0|com.example.app|1|tag|1000",
				PackageName: "com.example.app",
				AppName:     "Example",
				Title:       "Test Notification",
				Text:        "Hello world",
				PostTimeMs:  12345678,
				IsClearable: true,
			},
		},
	}
	if err := ValidateNotificationFrame(validPosted); err != nil {
		t.Fatalf("expected valid frame, got error: %v", err)
	}

	// Test bounds violations
	oversizedTitle := strings.Repeat("A", MaxTitleBytes+1)
	p := validPosted.GetPosted()
	p.Title = oversizedTitle
	if err := ValidateNotificationFrame(validPosted); err != ErrTitleTooLarge {
		t.Fatalf("expected ErrTitleTooLarge, got %v", err)
	}
	p.Title = "Test"

	oversizedText := strings.Repeat("B", MaxTextBytes+1)
	p.Text = oversizedText
	if err := ValidateNotificationFrame(validPosted); err != ErrTextTooLarge {
		t.Fatalf("expected ErrTextTooLarge, got %v", err)
	}
	p.Text = "Hello"

	emptyKey := validPosted
	p.Key = ""
	if err := ValidateNotificationFrame(emptyKey); err != ErrKeyEmpty {
		t.Fatalf("expected ErrKeyEmpty, got %v", err)
	}
}

func TestValidateNotificationRemoved(t *testing.T) {
	rem := &phonebridgev1.NotificationFrame{
		Version:     1,
		TimestampMs: 12345678,
		Event: &phonebridgev1.NotificationFrame_Removed{
			Removed: &phonebridgev1.NotificationRemoved{
				Key:         "0|com.example.app|1|tag|1000",
				PackageName: "com.example.app",
				Reason:      1,
			},
		},
	}
	if err := ValidateNotificationFrame(rem); err != nil {
		t.Fatalf("expected valid removed frame, got %v", err)
	}

	rem.GetRemoved().Key = ""
	if err := ValidateNotificationFrame(rem); err != ErrKeyEmpty {
		t.Fatalf("expected ErrKeyEmpty, got %v", err)
	}
}

func TestSanitizeString(t *testing.T) {
	s, err := SanitizeString("Hello\x00World")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != "HelloWorld" {
		t.Fatalf("expected HelloWorld, got %q", s)
	}

	_, err = SanitizeString("\xff\xfe\xfd")
	if err != ErrInvalidUTF8 {
		t.Fatalf("expected ErrInvalidUTF8, got %v", err)
	}
}
