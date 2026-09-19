package clipboard

import (
	"mime"
	"strings"
)

// Supported MIME type constants for PhoneBridge V1 (DEC-023).
const (
	// MIMETextPlain is the plain text MIME type without charset.
	MIMETextPlain = "text/plain"

	// MIMETextPlainUTF8 is the canonical normalized plain text MIME type with explicit UTF-8 charset.
	MIMETextPlainUTF8 = "text/plain;charset=utf-8"

	// CanonicalMIME is the default canonical MIME type for all text clipboard payloads in V1.
	CanonicalMIME = MIMETextPlainUTF8
)

// NormalizeMIME parses and validates a MIME type for PhoneBridge V1.
// Equivalent representations of UTF-8 plain text (e.g. "text/plain", "text/plain; charset=utf-8",
// "TEXT/PLAIN; CHARSET=UTF-8") normalize consistently to "text/plain;charset=utf-8".
// Any unsupported MIME types (e.g. "text/html", "text/uri-list", "image/png") or non-UTF-8
// charsets are rejected with ErrUnsupportedMIME.
func NormalizeMIME(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ErrUnsupportedMIME
	}

	mediatype, params, err := mime.ParseMediaType(trimmed)
	if err != nil {
		return "", ErrUnsupportedMIME
	}

	if mediatype != MIMETextPlain {
		return "", ErrUnsupportedMIME
	}

	// Check parameters: only "charset" is permitted, and it must be utf-8.
	for k, v := range params {
		if strings.ToLower(k) == "charset" {
			val := strings.ToLower(strings.TrimSpace(v))
			if val != "utf-8" && val != "utf8" {
				return "", ErrUnsupportedMIME
			}
		} else {
			// Reject unexpected parameters
			return "", ErrUnsupportedMIME
		}
	}

	return CanonicalMIME, nil
}

// IsSupportedMIME returns true if the given MIME type is supported under V1 rules.
func IsSupportedMIME(raw string) bool {
	_, err := NormalizeMIME(raw)
	return err == nil
}
