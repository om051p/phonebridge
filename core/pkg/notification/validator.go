// Package notification implements validation, rate-limiting, store, and transport for Phase 8 Notifications v0.1.
package notification

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

const (
	// MaxKeyLengthBytes is the upper bound on notification key size.
	MaxKeyLengthBytes = 256
	// MaxPackageNameBytes is the upper bound on package name length.
	MaxPackageNameBytes = 256
	// MaxAppNameBytes is the upper bound on app name length.
	MaxAppNameBytes = 256
	// MaxTitleBytes is the upper bound on notification title length.
	MaxTitleBytes = 256
	// MaxTextBytes is the upper bound on notification body text length.
	MaxTextBytes = 4096
	// MaxSubTextBytes is the upper bound on notification subtext length.
	MaxSubTextBytes = 256
	// MaxCategoryBytes is the upper bound on category string length.
	MaxCategoryBytes = 64
)

var (
	ErrNilFrame            = errors.New("notification: nil frame")
	ErrNoEvent             = errors.New("notification: frame carries no event")
	ErrKeyEmpty            = errors.New("notification: key is empty")
	ErrKeyTooLarge         = fmt.Errorf("notification: key exceeds %d bytes", MaxKeyLengthBytes)
	ErrPackageNameEmpty    = errors.New("notification: package_name is empty")
	ErrPackageNameTooLarge = fmt.Errorf("notification: package_name exceeds %d bytes", MaxPackageNameBytes)
	ErrAppNameTooLarge     = fmt.Errorf("notification: app_name exceeds %d bytes", MaxAppNameBytes)
	ErrTitleTooLarge       = fmt.Errorf("notification: title exceeds %d bytes", MaxTitleBytes)
	ErrTextTooLarge        = fmt.Errorf("notification: text exceeds %d bytes", MaxTextBytes)
	ErrSubTextTooLarge     = fmt.Errorf("notification: sub_text exceeds %d bytes", MaxSubTextBytes)
	ErrCategoryTooLarge    = fmt.Errorf("notification: category exceeds %d bytes", MaxCategoryBytes)
	ErrInvalidUTF8         = errors.New("notification: string field contains invalid UTF-8")
)

// SanitizeString cleans NUL bytes and verifies valid UTF-8.
func SanitizeString(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", ErrInvalidUTF8
	}
	if strings.IndexByte(s, 0) >= 0 {
		s = strings.ReplaceAll(s, "\x00", "")
	}
	return s, nil
}

// ValidateNotificationFrame enforces field length bounds and UTF-8 validity.
func ValidateNotificationFrame(frame *phonebridgev1.NotificationFrame) error {
	if frame == nil {
		return ErrNilFrame
	}
	switch ev := frame.Event.(type) {
	case *phonebridgev1.NotificationFrame_Posted:
		if ev.Posted == nil {
			return ErrNoEvent
		}
		return ValidateNotificationPosted(ev.Posted)
	case *phonebridgev1.NotificationFrame_Removed:
		if ev.Removed == nil {
			return ErrNoEvent
		}
		return ValidateNotificationRemoved(ev.Removed)
	case *phonebridgev1.NotificationFrame_Dismiss:
		if ev.Dismiss == nil {
			return ErrNoEvent
		}
		return ValidateNotificationDismiss(ev.Dismiss)
	default:
		return ErrNoEvent
	}
}

// ValidateNotificationPosted validates a NotificationPosted payload.
func ValidateNotificationPosted(p *phonebridgev1.NotificationPosted) error {
	if p == nil {
		return ErrNoEvent
	}
	if len(p.Key) == 0 {
		return ErrKeyEmpty
	}
	if len(p.Key) > MaxKeyLengthBytes {
		return ErrKeyTooLarge
	}
	if len(p.PackageName) == 0 {
		return ErrPackageNameEmpty
	}
	if len(p.PackageName) > MaxPackageNameBytes {
		return ErrPackageNameTooLarge
	}
	if len(p.AppName) > MaxAppNameBytes {
		return ErrAppNameTooLarge
	}
	if len(p.Title) > MaxTitleBytes {
		return ErrTitleTooLarge
	}
	if len(p.Text) > MaxTextBytes {
		return ErrTextTooLarge
	}
	if len(p.SubText) > MaxSubTextBytes {
		return ErrSubTextTooLarge
	}
	if len(p.Category) > MaxCategoryBytes {
		return ErrCategoryTooLarge
	}

	for _, s := range []string{p.Key, p.PackageName, p.AppName, p.Title, p.Text, p.SubText, p.Category} {
		if !utf8.ValidString(s) {
			return ErrInvalidUTF8
		}
	}
	return nil
}

// ValidateNotificationRemoved validates a NotificationRemoved payload.
func ValidateNotificationRemoved(r *phonebridgev1.NotificationRemoved) error {
	if r == nil {
		return ErrNoEvent
	}
	if len(r.Key) == 0 {
		return ErrKeyEmpty
	}
	if len(r.Key) > MaxKeyLengthBytes {
		return ErrKeyTooLarge
	}
	if len(r.PackageName) > MaxPackageNameBytes {
		return ErrPackageNameTooLarge
	}
	if !utf8.ValidString(r.Key) || !utf8.ValidString(r.PackageName) {
		return ErrInvalidUTF8
	}
	return nil
}

// ValidateNotificationDismiss validates a NotificationDismiss payload.
func ValidateNotificationDismiss(d *phonebridgev1.NotificationDismiss) error {
	if d == nil {
		return ErrNoEvent
	}
	if len(d.Key) == 0 {
		return ErrKeyEmpty
	}
	if len(d.Key) > MaxKeyLengthBytes {
		return ErrKeyTooLarge
	}
	if !utf8.ValidString(d.Key) {
		return ErrInvalidUTF8
	}
	return nil
}
