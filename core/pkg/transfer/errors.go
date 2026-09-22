package transfer

import (
	"errors"
	"fmt"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

// Failure is a typed transfer failure. It always carries both the wire code
// (what the peer is told) and the local reason (what the UI shows), so a caller
// never has to infer a cause from a string.
type Failure struct {
	Code    phonebridgev1.Code
	Reason  Reason
	Message string
	Err     error
}

func (f *Failure) Error() string {
	if f == nil {
		return "<nil>"
	}
	if f.Err != nil && f.Message != "" {
		return fmt.Sprintf("transfer: %s: %v", f.Message, f.Err)
	}
	if f.Err != nil {
		return fmt.Sprintf("transfer: %v", f.Err)
	}
	if f.Message != "" {
		return fmt.Sprintf("transfer: %s", f.Message)
	}
	return fmt.Sprintf("transfer: %s", f.Code)
}

func (f *Failure) Unwrap() error { return f.Err }

// NewFailure builds a typed Failure with a formatted message. It is exported so
// callers outside this package (the session manager and the platform bridges)
// report the same typed outcome the engine would, instead of inventing their
// own error strings that the UI would have to parse.
func NewFailure(code phonebridgev1.Code, reason Reason, format string, args ...any) *Failure {
	return &Failure{Code: code, Reason: reason, Message: fmt.Sprintf(format, args...)}
}

// newFailure is the in-package shorthand for NewFailure.
func newFailure(code phonebridgev1.Code, reason Reason, format string, args ...any) *Failure {
	return NewFailure(code, reason, format, args...)
}

// IsFailure reports whether err is (or wraps) a typed transfer Failure.
func IsFailure(err error) (*Failure, bool) {
	var f *Failure
	if errors.As(err, &f) {
		return f, true
	}
	return nil, false
}

// CodeForReason maps a local reason onto the wire code the peer is told. It is
// the single mapping point, so the two enumerations cannot drift.
func CodeForReason(r Reason) phonebridgev1.Code {
	switch r {
	case ReasonNone:
		return phonebridgev1.Code_CODE_OK
	case ReasonNoSession, ReasonUnsupportedPeer:
		return phonebridgev1.Code_CODE_UNAVAILABLE
	case ReasonBusy:
		return phonebridgev1.Code_CODE_TRANSFER_BUSY
	case ReasonUnsafeFilename:
		return phonebridgev1.Code_CODE_UNSAFE_FILENAME
	case ReasonTooLarge:
		return phonebridgev1.Code_CODE_FILE_TOO_LARGE
	case ReasonChecksumMismatch:
		return phonebridgev1.Code_CODE_CHECKSUM_MISMATCH
	case ReasonStorageFailed:
		return phonebridgev1.Code_CODE_STORAGE_FAILED
	case ReasonInterrupted:
		return phonebridgev1.Code_CODE_TRANSFER_INTERRUPTED
	case ReasonCancelledByPeer, ReasonCancelledByUser:
		return phonebridgev1.Code_CODE_TRANSFER_CANCELLED
	case ReasonProtocolError:
		return phonebridgev1.Code_CODE_INVALID_ARGUMENT
	case ReasonIncompatibleVersion:
		return phonebridgev1.Code_CODE_INCOMPATIBLE_VERSION
	default:
		return phonebridgev1.Code_CODE_INTERNAL
	}
}

// ReasonForCode maps a wire code onto the local reason (the same mapping,
// inverted, for frames received from the peer).
func ReasonForCode(c phonebridgev1.Code) Reason {
	switch c {
	case phonebridgev1.Code_CODE_OK:
		return ReasonNone
	case phonebridgev1.Code_CODE_UNAVAILABLE:
		return ReasonNoSession
	case phonebridgev1.Code_CODE_TRANSFER_BUSY:
		return ReasonBusy
	case phonebridgev1.Code_CODE_UNSAFE_FILENAME:
		return ReasonUnsafeFilename
	case phonebridgev1.Code_CODE_FILE_TOO_LARGE:
		return ReasonTooLarge
	case phonebridgev1.Code_CODE_CHECKSUM_MISMATCH:
		return ReasonChecksumMismatch
	case phonebridgev1.Code_CODE_STORAGE_FAILED:
		return ReasonStorageFailed
	case phonebridgev1.Code_CODE_TRANSFER_INTERRUPTED:
		return ReasonInterrupted
	case phonebridgev1.Code_CODE_TRANSFER_CANCELLED:
		return ReasonCancelledByPeer
	case phonebridgev1.Code_CODE_INVALID_ARGUMENT:
		return ReasonProtocolError
	case phonebridgev1.Code_CODE_INCOMPATIBLE_VERSION:
		return ReasonIncompatibleVersion
	default:
		return ReasonUnspecified
	}
}
