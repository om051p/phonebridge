package clipboard

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// IPC Protocol Constants
const (
	CmdSetSelection   = "SET_SELECTION"
	CmdClearSelection = "CLEAR_SELECTION"
	CmdShutdown       = "SHUTDOWN"

	StatusReady                 = "READY"
	StatusErrWaylandConnect     = "ERR_WAYLAND_CONNECT"
	StatusErrNoDataControl      = "ERR_NO_DATA_CONTROL"
	StatusErrCosmicFlagRequired = "ERR_COSMIC_FLAG_REQUIRED"
	StatusErrNoSeat             = "ERR_NO_SEAT"
	StatusErrCompositorDisconn  = "ERR_COMPOSITOR_DISCONNECTED"
	StatusOk                    = "OK"
	StatusError                 = "ERROR"

	EventSelectionOffer   = "SELECTION_OFFER"
	EventReadData         = "READ_DATA"
	EventReadOversized    = "READ_OVERSIZED"
	EventSelectionCleared = "SELECTION_CLEARED"
	EventSourceCancelled  = "SOURCE_CANCELLED"
	EventUnsupportedOffer = "UNSUPPORTED_OFFER"
	EventSourceSend       = "SOURCE_SEND"
)

var (
	ErrMalformedIPCMessage = errors.New("clipboard: malformed IPC message")
)

// IPCMessage represents a parsed IPC line from or to the helper.
type IPCMessage struct {
	Type    string // "STATUS", "EVENT", "CMD"
	Name    string // e.g. "READY", "READ_DATA", "SET_SELECTION"
	Params  map[string]string
	Payload []byte
}

// WriteCommand writes a command to the helper's stdin.
// If payload is provided, it uses length-prefixed framing:
// CMD=SET_SELECTION mime=<mime> len=<len>\n<payload>\n
func WriteCommand(w io.Writer, cmd string, params map[string]string, payload []byte) error {
	var sb strings.Builder
	sb.WriteString("CMD=")
	sb.WriteString(cmd)

	for k, v := range params {
		sb.WriteString(" ")
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(v)
	}

	if payload != nil {
		if len(payload) > MaxPayloadSize {
			return ErrPayloadTooLarge
		}
		if _, hasLen := params["len"]; !hasLen {
			sb.WriteString(" len=")
			sb.WriteString(strconv.Itoa(len(payload)))
		}
	}
	sb.WriteString("\n")

	if _, err := io.WriteString(w, sb.String()); err != nil {
		return err
	}

	if payload != nil {
		if _, err := w.Write(payload); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
	}

	return nil
}

// ReadIPCMessage reads a single message from the helper's stdout.
// If the message is EVENT=READ_DATA, it reads the length-prefixed payload.
func ReadIPCMessage(r *bufio.Reader) (*IPCMessage, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}

	line = strings.TrimRight(line, "\r\n")
	if len(line) == 0 {
		return nil, ErrMalformedIPCMessage
	}

	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return nil, ErrMalformedIPCMessage
	}

	msgType := parts[0]
	rest := parts[1]

	tokens := strings.Fields(rest)
	if len(tokens) == 0 {
		return nil, ErrMalformedIPCMessage
	}

	msg := &IPCMessage{
		Type:   msgType,
		Name:   tokens[0],
		Params: make(map[string]string),
	}

	for _, token := range tokens[1:] {
		kv := strings.SplitN(token, "=", 2)
		if len(kv) == 2 {
			msg.Params[kv[0]] = kv[1]
		}
	}

	// If this is READ_DATA or SET_SELECTION, read length-prefixed payload
	if (msg.Type == "EVENT" && msg.Name == EventReadData) ||
		(msg.Type == "CMD" && msg.Name == CmdSetSelection) {
		lenStr, ok := msg.Params["len"]
		if !ok {
			return nil, fmt.Errorf("%w: missing len in %s", ErrMalformedIPCMessage, msg.Name)
		}

		length, err := strconv.Atoi(lenStr)
		if err != nil || length < 0 {
			return nil, fmt.Errorf("%w: invalid len in %s: %v", ErrMalformedIPCMessage, msg.Name, lenStr)
		}

		if length > MaxPayloadSize {
			return nil, ErrPayloadTooLarge
		}

		payload := make([]byte, length)
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, fmt.Errorf("clipboard: failed to read payload: %w", err)
		}

		// Read and discard trailing newline delimiter
		trailing, err := r.ReadByte()
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("clipboard: failed to read trailing delimiter: %w", err)
		}
		if trailing != '\n' && trailing != '\r' && trailing != 0 {
			// Delimiter is expected to be newline
		}

		msg.Payload = payload
	}

	return msg, nil
}
