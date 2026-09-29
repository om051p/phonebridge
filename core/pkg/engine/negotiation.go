package engine

import "fmt"

// This file holds the session negotiation vocabulary shared by the session
// state machine, the LAN signaling client (DEC-022) and the local IPC surface:
// media parameters, typed failure codes, and the typed session reason taxonomy.
//
// It is deliberately the only definition of each of these concepts in the core,
// so a requested/actual mismatch cannot drift between layers.

// MediaParams is the parameter tuple for a screen stream. It mirrors
// phonebridge.v1.MediaParams field for field: the same shape is carried by the
// device protocol, the LAN signaling exchange and the local IPC session
// snapshot. A zero field means "no preference / not reported" — never a request
// for a zero-sized stream.
type MediaParams struct {
	Width       int
	Height      int
	FPS         int
	BitrateKbps int
	Codec       string
}

// IsZero reports whether no parameter is set at all.
func (p MediaParams) IsZero() bool {
	return p.Width == 0 && p.Height == 0 && p.FPS == 0 && p.BitrateKbps == 0 && p.Codec == ""
}

// Equal reports whether two tuples are identical. Used to detect a reported
// downgrade (requested != actual) without inventing a separate flag.
func (p MediaParams) Equal(o MediaParams) bool {
	return p.Width == o.Width && p.Height == o.Height && p.FPS == o.FPS &&
		p.BitrateKbps == o.BitrateKbps && p.Codec == o.Codec
}

// WithDefaults fills zero fields from fallback. It is only used to turn a
// caller's partial request into the concrete tuple the capture device receives,
// so the device never has to infer intent from absent fields.
func (p MediaParams) WithDefaults(fallback MediaParams) MediaParams {
	out := p
	if out.Width == 0 {
		out.Width = fallback.Width
	}
	if out.Height == 0 {
		out.Height = fallback.Height
	}
	if out.FPS == 0 {
		out.FPS = fallback.FPS
	}
	if out.BitrateKbps == 0 {
		out.BitrateKbps = fallback.BitrateKbps
	}
	if out.Codec == "" {
		out.Codec = fallback.Codec
	}
	return out
}

func (p MediaParams) String() string {
	return fmt.Sprintf("%dx%d@%dfps %dkbps %s", p.Width, p.Height, p.FPS, p.BitrateKbps, codecOrDefault(p.Codec))
}

func codecOrDefault(codec string) string {
	if codec == "" {
		return "(default)"
	}
	return codec
}

// MediaCapability advertises what a capture device can do, mirroring
// phonebridge.v1.MediaCapabilities.
type MediaCapability struct {
	Codecs         []string
	MaxWidth       int
	MaxHeight      int
	MaxFPS         int
	SupportsScreen bool
}

// Code is a typed negotiation/failure outcome. The values match the names of
// phonebridge.v1.Code so the LAN signaling JSON and the device protocol agree
// without a translation table.
type Code string

const (
	CodeOK Code = "OK"
	// CodeUnsupportedMediaParams: the capture device cannot satisfy the request.
	CodeUnsupportedMediaParams Code = "UNSUPPORTED_MEDIA_PARAMS"
	// CodeConsentRevoked: MediaProjection consent was withdrawn. The link is
	// healthy; there is simply no longer a screen to send (DEC-020).
	CodeConsentRevoked Code = "CONSENT_REVOKED"
	// CodeCaptureFailed: capture/encode failed on the sending device.
	CodeCaptureFailed Code = "CAPTURE_FAILED"
	// CodeTransportFailed: ICE/DTLS failed.
	CodeTransportFailed Code = "TRANSPORT_FAILED"
	// CodeReconnectTimeout: the bounded reconnect window elapsed.
	CodeReconnectTimeout Code = "RECONNECT_TIMEOUT"
	// CodeSessionBusy: the peer already has an active session.
	CodeSessionBusy Code = "SESSION_BUSY"
	// CodeIncompatibleVersion: no protocol version in common.
	CodeIncompatibleVersion Code = "INCOMPATIBLE_VERSION"
	// CodeSignalingFailed: the LAN signaling exchange itself failed
	// (HTTP/auth/timeout). Always retryable; local classification only.
	CodeSignalingFailed Code = "SIGNALING_FAILED"
	// CodeInvalidArgument: a malformed request.
	CodeInvalidArgument Code = "INVALID_ARGUMENT"
	// CodePermissionDenied: the peer refused the request (auth/trust).
	CodePermissionDenied Code = "PERMISSION_DENIED"
)

// Retryable reports whether the condition can plausibly heal by re-running the
// offer/answer exchange. Sender-side conditions (consent, capture, bad
// parameters, busy, version) are terminal: retrying would only delay the
// error the user needs to see.
func (c Code) Retryable() bool {
	switch c {
	case CodeTransportFailed, CodeReconnectTimeout, CodeSignalingFailed:
		return true
	default:
		return false
	}
}

// Reason maps a code onto the typed session reason taxonomy.
func (c Code) Reason() SessionReason {
	switch c {
	case CodeUnsupportedMediaParams:
		return ReasonUnsupportedMediaParams
	case CodeConsentRevoked:
		return ReasonConsentRevoked
	case CodeCaptureFailed:
		return ReasonCaptureFailed
	case CodeTransportFailed:
		return ReasonTransportFailed
	case CodeReconnectTimeout:
		return ReasonReconnectTimeout
	case CodeSessionBusy:
		return ReasonSessionBusy
	case CodeIncompatibleVersion:
		return ReasonProtocolVersionMismatch
	case CodeSignalingFailed:
		return ReasonSignalingFailed
	case CodePermissionDenied:
		return ReasonDeviceNotTrusted
	case CodeInvalidArgument:
		return ReasonUnsupportedMediaParams
	default:
		return ReasonUnspecified
	}
}

// codeForReason maps a typed session reason back onto its wire code. It is the
// inverse of Code.Reason and exists so an outcome the local session classified
// internally (for example a trust refusal while answering a peer-supplied
// offer) is reported to the peer with the same code the initiator path would
// have used for the same condition.
func codeForReason(r SessionReason) Code {
	switch r {
	case ReasonProtocolVersionMismatch:
		return CodeIncompatibleVersion
	case ReasonUnsupportedMediaParams:
		return CodeUnsupportedMediaParams
	case ReasonDeviceNotTrusted:
		return CodePermissionDenied
	case ReasonSessionBusy:
		return CodeSessionBusy
	case ReasonConsentRevoked:
		return CodeConsentRevoked
	case ReasonCaptureFailed:
		return CodeCaptureFailed
	case ReasonTransportFailed:
		return CodeTransportFailed
	case ReasonReconnectTimeout:
		return CodeReconnectTimeout
	case ReasonSignalingFailed:
		return CodeSignalingFailed
	default:
		return CodeInvalidArgument
	}
}

// ParseCode accepts an enum name from the wire. An unknown or empty value is
// reported as CodeOK only when it is empty; unknown non-empty values are
// surfaced as a signaling failure so a newer peer's code is never silently
// treated as success.
func ParseCode(s string) (Code, bool) {
	if s == "" {
		return CodeOK, true
	}
	known := map[Code]bool{
		CodeOK: true, CodeUnsupportedMediaParams: true, CodeConsentRevoked: true,
		CodeCaptureFailed: true, CodeTransportFailed: true, CodeReconnectTimeout: true,
		CodeSessionBusy: true, CodeIncompatibleVersion: true, CodeSignalingFailed: true,
		CodeInvalidArgument: true, CodePermissionDenied: true,
	}
	c := Code(s)
	if known[c] {
		return c, true
	}
	return c, false
}

// SessionReason is a typed classification of a session state change. The values
// mirror phonebridge.localipc.v1.SessionReason one-for-one.
//
// It exists so failure handling is never inferred from prose: reason strings
// remain human-readable diagnostics, while the reason code drives recovery
// policy (retry vs terminal) and what the UI reports to the user.
type SessionReason int

const (
	// ReasonUnspecified: the transition carries no classification.
	ReasonUnspecified SessionReason = iota
	// ReasonNone: explicitly "no failure" (connecting, streaming, user stop).
	ReasonNone
	ReasonProtocolVersionMismatch
	ReasonUnsupportedMediaParams
	ReasonDeviceNotTrusted
	ReasonDeviceNotFound
	ReasonSessionBusy
	// ReasonConsentRevoked: the user withdrew MediaProjection consent.
	ReasonConsentRevoked
	ReasonCaptureFailed
	ReasonTransportFailed
	ReasonReconnectTimeout
	ReasonSignalingFailed
	ReasonUserStopped
)

// terminal reports whether a reason means the session cannot recover without a
// new user action. A transport reason is recoverable by re-running the
// offer/answer exchange; a capture or consent reason is not.
func (r SessionReason) terminal() bool {
	switch r {
	case ReasonTransportFailed, ReasonReconnectTimeout, ReasonSignalingFailed,
		ReasonUnspecified, ReasonNone:
		return false
	default:
		return true
	}
}

func (r SessionReason) String() string {
	switch r {
	case ReasonUnspecified:
		return "UNSPECIFIED"
	case ReasonNone:
		return "NONE"
	case ReasonProtocolVersionMismatch:
		return "PROTOCOL_VERSION_MISMATCH"
	case ReasonUnsupportedMediaParams:
		return "UNSUPPORTED_MEDIA_PARAMS"
	case ReasonDeviceNotTrusted:
		return "DEVICE_NOT_TRUSTED"
	case ReasonDeviceNotFound:
		return "DEVICE_NOT_FOUND"
	case ReasonSessionBusy:
		return "SESSION_BUSY"
	case ReasonConsentRevoked:
		return "CONSENT_REVOKED"
	case ReasonCaptureFailed:
		return "CAPTURE_FAILED"
	case ReasonTransportFailed:
		return "TRANSPORT_FAILED"
	case ReasonReconnectTimeout:
		return "RECONNECT_TIMEOUT"
	case ReasonSignalingFailed:
		return "SIGNALING_FAILED"
	case ReasonUserStopped:
		return "USER_STOPPED"
	default:
		return "UNKNOWN"
	}
}

// SignalError is a transport-level signaling failure (HTTP/auth/timeout). It is
// always retryable and always classified as SIGNALING_FAILED, which keeps it
// distinct from a negotiation rejection coming back from the device.
type SignalError struct {
	Op       string
	Endpoint string
	Err      error
}

func (e *SignalError) Error() string {
	return fmt.Sprintf("signaling %s to %s failed: %v", e.Op, e.Endpoint, e.Err)
}

func (e *SignalError) Unwrap() error { return e.Err }

// NegotiationResponse is the capture device's answer to a session request.
type NegotiationResponse struct {
	// Offer is the SDP offer the phone composed for this session.
	Offer string
	// Accepted is true when the device agreed to the exact requested tuple.
	Accepted bool
	// RejectReason explains a rejection (human-readable detail).
	RejectReason string
	// Actual is the tuple the device applied. It is authoritative (DEC-020)
	// and is only meaningful when ActualKnown is true.
	Actual MediaParams
	// ActualKnown is false when the peer did not report the applied tuple. An
	// unknown tuple is never back-filled from the request: assuming "probably
	// the same" is exactly the silent substitution DEC-022 forbids.
	ActualKnown bool
	// Code is the typed outcome; CodeOK on success.
	Code Code
	// Message is the device's human-readable detail for Code.
	Message string
	// Capabilities is what the device advertised in this exchange (optional).
	Capabilities []MediaCapability
	// ProtocolVersion is the version the device selected.
	ProtocolVersion uint32
}
