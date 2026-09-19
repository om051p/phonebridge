package clipboard

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestIPCWriteCommand(t *testing.T) {
	t.Run("ClearSelection", func(t *testing.T) {
		var buf bytes.Buffer
		err := WriteCommand(&buf, CmdClearSelection, nil, nil)
		if err != nil {
			t.Fatalf("WriteCommand failed: %v", err)
		}
		expected := "CMD=CLEAR_SELECTION\n"
		if buf.String() != expected {
			t.Errorf("got %q, want %q", buf.String(), expected)
		}
	})

	t.Run("Shutdown", func(t *testing.T) {
		var buf bytes.Buffer
		err := WriteCommand(&buf, CmdShutdown, nil, nil)
		if err != nil {
			t.Fatalf("WriteCommand failed: %v", err)
		}
		expected := "CMD=SHUTDOWN\n"
		if buf.String() != expected {
			t.Errorf("got %q, want %q", buf.String(), expected)
		}
	})

	t.Run("SetSelectionWithPayload", func(t *testing.T) {
		var buf bytes.Buffer
		payload := []byte("Hello\nWorld!\x00Binary")
		params := map[string]string{
			"mime": "text/plain;charset=utf-8",
		}
		err := WriteCommand(&buf, CmdSetSelection, params, payload)
		if err != nil {
			t.Fatalf("WriteCommand failed: %v", err)
		}

		// Header must contain len=19 and mime
		str := buf.String()
		if !strings.HasPrefix(str, "CMD=SET_SELECTION") {
			t.Errorf("expected CMD=SET_SELECTION prefix, got %q", str)
		}
		if !strings.Contains(str, "len=19") {
			t.Errorf("expected len=19 in header, got %q", str)
		}
		if !strings.Contains(str, "mime=text/plain;charset=utf-8") {
			t.Errorf("expected mime param in header, got %q", str)
		}
		if !strings.HasSuffix(str, string(payload)+"\n") {
			t.Errorf("expected payload with trailing newline at end")
		}
	})

	t.Run("SetSelectionPayloadCeiling", func(t *testing.T) {
		var buf bytes.Buffer
		huge := make([]byte, MaxPayloadSize+1)
		err := WriteCommand(&buf, CmdSetSelection, nil, huge)
		if !errors.Is(err, ErrPayloadTooLarge) {
			t.Errorf("got %v, want ErrPayloadTooLarge", err)
		}
	})
}

func TestIPCReadMessage(t *testing.T) {
	t.Run("StatusReady", func(t *testing.T) {
		input := "STATUS=READY compositor=COSMIC data_control=v2\n"
		r := bufio.NewReader(strings.NewReader(input))
		msg, err := ReadIPCMessage(r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if msg.Type != "STATUS" || msg.Name != StatusReady {
			t.Errorf("got Type=%q Name=%q, want STATUS READY", msg.Type, msg.Name)
		}
		if msg.Params["compositor"] != "COSMIC" {
			t.Errorf("got compositor=%q, want COSMIC", msg.Params["compositor"])
		}
		if msg.Params["data_control"] != "v2" {
			t.Errorf("got data_control=%q, want v2", msg.Params["data_control"])
		}
	})

	t.Run("StatusErrors", func(t *testing.T) {
		tests := []struct {
			input    string
			wantName string
		}{
			{"STATUS=ERR_WAYLAND_CONNECT detail=socket_error\n", StatusErrWaylandConnect},
			{"STATUS=ERR_NO_DATA_CONTROL detail=not_advertised\n", StatusErrNoDataControl},
			{"STATUS=ERR_COSMIC_FLAG_REQUIRED detail=env_var_missing\n", StatusErrCosmicFlagRequired},
			{"STATUS=ERR_NO_SEAT detail=missing\n", StatusErrNoSeat},
			{"STATUS=ERR_COMPOSITOR_DISCONNECTED detail=socket_closed\n", StatusErrCompositorDisconn},
		}

		for _, tc := range tests {
			r := bufio.NewReader(strings.NewReader(tc.input))
			msg, err := ReadIPCMessage(r)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if msg.Type != "STATUS" || msg.Name != tc.wantName {
				t.Errorf("got Type=%q Name=%q, want STATUS %s", msg.Type, msg.Name, tc.wantName)
			}
		}
	})

	t.Run("EventSelectionOffer", func(t *testing.T) {
		input := "EVENT=SELECTION_OFFER mime_count=2 mimes=text/plain;charset=utf-8,text/plain\n"
		r := bufio.NewReader(strings.NewReader(input))
		msg, err := ReadIPCMessage(r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if msg.Type != "EVENT" || msg.Name != EventSelectionOffer {
			t.Errorf("got Type=%q Name=%q, want EVENT SELECTION_OFFER", msg.Type, msg.Name)
		}
		if msg.Params["mime_count"] != "2" {
			t.Errorf("got mime_count=%q, want 2", msg.Params["mime_count"])
		}
		if msg.Params["mimes"] != "text/plain;charset=utf-8,text/plain" {
			t.Errorf("got mimes=%q", msg.Params["mimes"])
		}
	})

	t.Run("EventReadDataExactPayload", func(t *testing.T) {
		payload := "Arbitrary\ntext with \x00 null bytes and \r\n newlines\nEVENT=FAKE"
		header := fmt.Sprintf("EVENT=READ_DATA mime=text/plain;charset=utf-8 len=%d\n", len(payload))
		fullInput := header + payload + "\n"

		r := bufio.NewReader(strings.NewReader(fullInput))
		msg, err := ReadIPCMessage(r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if msg.Type != "EVENT" || msg.Name != EventReadData {
			t.Errorf("got Type=%q Name=%q, want EVENT READ_DATA", msg.Type, msg.Name)
		}
		if msg.Params["mime"] != "text/plain;charset=utf-8" {
			t.Errorf("got mime=%q", msg.Params["mime"])
		}
		if string(msg.Payload) != payload {
			t.Errorf("payload mismatch: got %q, want %q", string(msg.Payload), payload)
		}
	})

	t.Run("EventReadOversized", func(t *testing.T) {
		input := "EVENT=READ_OVERSIZED mime=text/plain size=1048576\n"
		r := bufio.NewReader(strings.NewReader(input))
		msg, err := ReadIPCMessage(r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if msg.Type != "EVENT" || msg.Name != EventReadOversized {
			t.Errorf("got Type=%q Name=%q, want EVENT READ_OVERSIZED", msg.Type, msg.Name)
		}
		if msg.Params["size"] != "1048576" {
			t.Errorf("got size=%q, want 1048576", msg.Params["size"])
		}
	})

	t.Run("MalformedMessages", func(t *testing.T) {
		malformed := []string{
			"",
			"\n",
			"INVALID_NO_EQUALS\n",
			"STATUS=\n",
			"EVENT=READ_DATA\n", // missing len
			"EVENT=READ_DATA mime=text/plain len=abc\n",
			"EVENT=READ_DATA mime=text/plain len=-5\n",
		}

		for _, input := range malformed {
			r := bufio.NewReader(strings.NewReader(input))
			_, err := ReadIPCMessage(r)
			if err == nil {
				t.Errorf("expected error for malformed input %q, got nil", input)
			}
		}
	})
}
