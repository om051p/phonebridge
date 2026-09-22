package transfer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"mime"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

// transferIDBytes is the entropy behind a transfer id: 128 random bits, hex
// encoded to 32 characters. It is the replay/duplicate key (DEC-024), so it has
// to be unguessable and unique per sender.
const transferIDBytes = 16

// maxFilenameBytes bounds a proposed basename (the common filesystem limit).
const maxFilenameBytes = 255

// NewTransferID returns a fresh random transfer identifier.
func NewTransferID() (string, error) {
	b := make([]byte, transferIDBytes)
	if _, err := rand.Read(b); err != nil {
		return "", newFailure(phonebridgev1.Code_CODE_INTERNAL, ReasonUnspecified, "generate transfer id: %v", err)
	}
	return hex.EncodeToString(b), nil
}

// SanitizeFilename validates and normalizes a proposed basename. The receiver
// owns the directory, so the sender never gets to influence a path: separators,
// NUL, control characters, "." / ".." and leading dots (which would let a peer
// drop hidden files or collide with the partial staging directory) are refused
// with CODE_UNSAFE_FILENAME.
func SanitizeFilename(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", newFailure(phonebridgev1.Code_CODE_UNSAFE_FILENAME, ReasonUnsafeFilename, "empty filename")
	}
	if trimmed == "." || trimmed == ".." {
		return "", newFailure(phonebridgev1.Code_CODE_UNSAFE_FILENAME, ReasonUnsafeFilename, "reserved filename %q", trimmed)
	}
	if strings.HasPrefix(trimmed, ".") {
		return "", newFailure(phonebridgev1.Code_CODE_UNSAFE_FILENAME, ReasonUnsafeFilename, "hidden filename %q is not accepted", trimmed)
	}
	if strings.ContainsAny(trimmed, "/\\") {
		return "", newFailure(phonebridgev1.Code_CODE_UNSAFE_FILENAME, ReasonUnsafeFilename, "filename %q contains a path separator", trimmed)
	}
	if strings.ContainsRune(trimmed, 0) {
		return "", newFailure(phonebridgev1.Code_CODE_UNSAFE_FILENAME, ReasonUnsafeFilename, "filename contains NUL")
	}
	for _, r := range trimmed {
		if r < 0x20 || r == 0x7f {
			return "", newFailure(phonebridgev1.Code_CODE_UNSAFE_FILENAME, ReasonUnsafeFilename, "filename contains a control character")
		}
	}
	if len(trimmed) > maxFilenameBytes {
		return "", newFailure(phonebridgev1.Code_CODE_UNSAFE_FILENAME, ReasonUnsafeFilename, "filename is %d bytes (limit %d)", len(trimmed), maxFilenameBytes)
	}
	return trimmed, nil
}

// MimeForName is a best-effort MIME lookup from the file extension. An unknown
// extension is not a failure: FileOffer.mime_type is defined as "empty means
// unknown" (DEC-024).
func MimeForName(name string) string {
	ext := filepath.Ext(name)
	if ext == "" {
		return ""
	}
	return mime.TypeByExtension(ext)
}

// EncodeFrame marshals a TransferFrame for the DataChannel.
func EncodeFrame(frame *phonebridgev1.TransferFrame) ([]byte, error) {
	if frame == nil {
		return nil, newFailure(phonebridgev1.Code_CODE_INVALID_ARGUMENT, ReasonProtocolError, "nil transfer frame")
	}
	if frame.Version == 0 {
		frame.Version = FrameVersion
	}
	wire, err := proto.Marshal(frame)
	if err != nil {
		return nil, newFailure(phonebridgev1.Code_CODE_INTERNAL, ReasonUnspecified, "marshal transfer frame: %v", err)
	}
	return wire, nil
}

// DecodeFrame parses and validates an inbound DataChannel message. It enforces
// the frame-size cap and the frame version before any body handling, so an
// oversized or future-version frame can never reach the state machine.
func DecodeFrame(data []byte) (*phonebridgev1.TransferFrame, error) {
	if len(data) == 0 {
		return nil, newFailure(phonebridgev1.Code_CODE_INVALID_ARGUMENT, ReasonProtocolError, "empty transfer frame")
	}
	if len(data) > MaxFrameBytes {
		return nil, newFailure(phonebridgev1.Code_CODE_INVALID_ARGUMENT, ReasonProtocolError, "transfer frame of %d bytes exceeds the %d-byte cap", len(data), MaxFrameBytes)
	}
	frame := &phonebridgev1.TransferFrame{}
	if err := proto.Unmarshal(data, frame); err != nil {
		return nil, newFailure(phonebridgev1.Code_CODE_INVALID_ARGUMENT, ReasonProtocolError, "malformed transfer frame: %v", err)
	}
	if frame.Version != FrameVersion {
		return nil, newFailure(phonebridgev1.Code_CODE_INCOMPATIBLE_VERSION, ReasonIncompatibleVersion,
			"transfer frame version %d is not supported (this build speaks %d)", frame.Version, FrameVersion)
	}
	if frame.GetBody() == nil {
		return nil, newFailure(phonebridgev1.Code_CODE_INVALID_ARGUMENT, ReasonProtocolError, "transfer frame carries no body")
	}
	return frame, nil
}

// Frame builders keep the oneof wiring and the frame version in one place, so a
// call site cannot pick the wrong branch or forget the version.

// OfferFrame wraps a FileOffer.
func OfferFrame(offer *phonebridgev1.FileOffer) *phonebridgev1.TransferFrame {
	return &phonebridgev1.TransferFrame{Version: FrameVersion, Body: &phonebridgev1.TransferFrame_Offer{Offer: offer}}
}

// AcceptFrame wraps a FileAccept.
func AcceptFrame(accept *phonebridgev1.FileAccept) *phonebridgev1.TransferFrame {
	return &phonebridgev1.TransferFrame{Version: FrameVersion, Body: &phonebridgev1.TransferFrame_Accept{Accept: accept}}
}

// ChunkFrame wraps a FileChunk.
func ChunkFrame(chunk *phonebridgev1.FileChunk) *phonebridgev1.TransferFrame {
	return &phonebridgev1.TransferFrame{Version: FrameVersion, Body: &phonebridgev1.TransferFrame_Chunk{Chunk: chunk}}
}

// CompleteFrame wraps a FileComplete.
func CompleteFrame(complete *phonebridgev1.FileComplete) *phonebridgev1.TransferFrame {
	return &phonebridgev1.TransferFrame{Version: FrameVersion, Body: &phonebridgev1.TransferFrame_Complete{Complete: complete}}
}

// ResultFrame wraps a FileResult.
func ResultFrame(result *phonebridgev1.FileResult) *phonebridgev1.TransferFrame {
	return &phonebridgev1.TransferFrame{Version: FrameVersion, Body: &phonebridgev1.TransferFrame_Result{Result: result}}
}

// CancelFrame wraps a FileCancel.
func CancelFrame(cancel *phonebridgev1.FileCancel) *phonebridgev1.TransferFrame {
	return &phonebridgev1.TransferFrame{Version: FrameVersion, Body: &phonebridgev1.TransferFrame_Cancel{Cancel: cancel}}
}

// Channel is the engine's view of the dedicated reliable/ordered "transfer"
// DataChannel. The engine never imports Pion: the adapter lives in
// transfer/rtcchannel, and tests use an in-memory implementation, so the
// backpressure and interruption rules are provable without a network.
type Channel interface {
	// SendFrame sends one encoded TransferFrame.
	SendFrame(ctx context.Context, frame []byte) error
	// BufferedAmount is how many bytes are queued below this layer. The sender
	// stops reading its source while this exceeds the high-watermark.
	BufferedAmount() uint64
	// AwaitDrain blocks until the buffered amount has fallen to the configured
	// low-watermark (or the context/channel ends).
	AwaitDrain(ctx context.Context) error
	// Done is closed when the underlying channel is gone (peer closed, session
	// lost). Every in-flight transfer is then interrupted, never resumed.
	Done() <-chan struct{}
}
