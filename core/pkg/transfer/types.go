package transfer

// Direction is which way the bytes flow for this device.
type Direction int

const (
	DirectionUnspecified Direction = iota
	DirectionOutbound
	DirectionInbound
)

func (d Direction) String() string {
	switch d {
	case DirectionOutbound:
		return "OUTBOUND"
	case DirectionInbound:
		return "INBOUND"
	default:
		return "UNSPECIFIED"
	}
}

// State is the lifecycle phase of one transfer.
type State int

const (
	StateUnspecified State = iota
	// Pending: offer sent/received, awaiting FileAccept.
	StatePending
	// Active: chunks are flowing.
	StateActive
	// Verifying: all chunks received, size/digest being checked and the file
	// being promoted.
	StateVerifying
	// Complete: verified and committed at the destination.
	StateComplete
	// Cancelled: cancelled locally or by the peer.
	StateCancelled
	// Failed: any typed failure (integrity, storage, interruption, protocol).
	StateFailed
)

func (s State) String() string {
	switch s {
	case StatePending:
		return "PENDING"
	case StateActive:
		return "ACTIVE"
	case StateVerifying:
		return "VERIFYING"
	case StateComplete:
		return "COMPLETE"
	case StateCancelled:
		return "CANCELLED"
	case StateFailed:
		return "FAILED"
	default:
		return "UNSPECIFIED"
	}
}

// Terminal reports whether the state ends a transfer's lifecycle.
func (s State) Terminal() bool {
	switch s {
	case StateComplete, StateCancelled, StateFailed:
		return true
	default:
		return false
	}
}

// Reason is the typed classification of a non-success outcome. It exists so the
// UI never parses prose (the SessionReason precedent); the wire carries
// phonebridge.v1.Code instead, and the two are mapped at the boundary.
type Reason int

const (
	// ReasonUnspecified means the engine did not classify the outcome.
	ReasonUnspecified Reason = iota
	// ReasonNone is an explicitly normal outcome.
	ReasonNone
	// ReasonNoSession: no transfer channel (no active session) is attached.
	ReasonNoSession
	// ReasonUnsupportedPeer: the peer has no "transfer" DataChannel.
	ReasonUnsupportedPeer
	// ReasonBusy: a transfer is already active in that direction.
	ReasonBusy
	// ReasonUnsafeFilename: the proposed name is not a plain basename.
	ReasonUnsafeFilename
	// ReasonTooLarge: the file exceeds the configured size policy.
	ReasonTooLarge
	// ReasonChecksumMismatch: size or SHA-256 did not match.
	ReasonChecksumMismatch
	// ReasonStorageFailed: destination write/promotion failed.
	ReasonStorageFailed
	// ReasonInterrupted: the transport/session went away mid-transfer.
	ReasonInterrupted
	// ReasonCancelledByPeer: the peer sent FileCancel.
	ReasonCancelledByPeer
	// ReasonCancelledByUser: a local cancel.
	ReasonCancelledByUser
	// ReasonProtocolError: a wire frame violated the protocol.
	ReasonProtocolError
	// ReasonIncompatibleVersion: unknown transfer frame version.
	ReasonIncompatibleVersion
)

func (r Reason) String() string {
	switch r {
	case ReasonNone:
		return "NONE"
	case ReasonNoSession:
		return "NO_SESSION"
	case ReasonUnsupportedPeer:
		return "UNSUPPORTED_PEER"
	case ReasonBusy:
		return "BUSY"
	case ReasonUnsafeFilename:
		return "UNSAFE_FILENAME"
	case ReasonTooLarge:
		return "TOO_LARGE"
	case ReasonChecksumMismatch:
		return "CHECKSUM_MISMATCH"
	case ReasonStorageFailed:
		return "STORAGE_FAILED"
	case ReasonInterrupted:
		return "INTERRUPTED"
	case ReasonCancelledByPeer:
		return "CANCELLED_BY_PEER"
	case ReasonCancelledByUser:
		return "CANCELLED_BY_USER"
	case ReasonProtocolError:
		return "PROTOCOL_ERROR"
	case ReasonIncompatibleVersion:
		return "INCOMPATIBLE_VERSION"
	default:
		return "UNSPECIFIED"
	}
}

// Info is the UI-facing snapshot of one transfer. It deliberately carries no
// payload bytes and no destination path: the receiver's directory layout stays
// private, and only the stored basename is reported (DEC-024).
type Info struct {
	TransferID       string
	Direction        Direction
	State            State
	PeerDeviceID     string
	Filename         string
	MimeType         string
	SizeBytes        uint64
	BytesTransferred uint64
	StartedAtMs      uint64
	FinishedAtMs     uint64
	ReasonCode       Reason
	ErrorMessage     string
	SavedName        string
}

// Event is one transfer transition delivered to the local UI.
type Event struct {
	Info Info
}
