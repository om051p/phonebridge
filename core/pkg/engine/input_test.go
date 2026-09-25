package engine

import (
	"context"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/input"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"github.com/om051p/phonebridge/core/pkg/rtpmedia"
	"github.com/om051p/phonebridge/core/pkg/transfer"
	pion "github.com/pion/webrtc/v4"
	"google.golang.org/protobuf/proto"
)

type mockInputTransport struct {
	mu        sync.Mutex
	sentBytes [][]byte
	closed    bool
}

func (m *mockInputTransport) SetRemoteOffer(offer pion.SessionDescription) (pion.SessionDescription, error) {
	return pion.SessionDescription{Type: pion.SDPTypeAnswer, SDP: "v=0\r\nfake\r\n"}, nil
}

func (m *mockInputTransport) WaitForTrack(timeout time.Duration) error {
	return nil
}

func (m *mockInputTransport) Stats() (rtpmedia.StreamStats, int64) {
	return rtpmedia.StreamStats{}, 0
}

func (m *mockInputTransport) TransferChannel() transfer.Channel {
	return nil
}

func (m *mockInputTransport) Close() error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}

func (m *mockInputTransport) SendInput(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentBytes = append(m.sentBytes, data)
	return nil
}

func TestSession_SendInput_LifecycleGating(t *testing.T) {
	sess := NewSession("sess-input-1", DefaultSessionConfig(), nil, nil)
	tr := &mockInputTransport{}
	sess.tr = tr

	frame := &phonebridgev1.InputFrame{
		Event: &phonebridgev1.InputFrame_Touch{
			Touch: &phonebridgev1.TouchEvent{
				Action:      phonebridgev1.TouchEvent_ACTION_DOWN,
				NormalizedX: 0.5,
				NormalizedY: 0.5,
			},
		},
	}

	// 1. Initial StateDisconnected -> must fail
	if err := sess.SendInput(frame); err == nil {
		t.Fatal("expected error in StateDisconnected")
	}

	// 2. StateConnecting -> must fail
	_ = sess.Transition(StateConnecting, "test")
	if err := sess.SendInput(frame); err == nil {
		t.Fatal("expected error in StateConnecting")
	}

	// 3. StateConnected -> must fail
	_ = sess.Transition(StateConnected, "test")
	if err := sess.SendInput(frame); err == nil {
		t.Fatal("expected error in StateConnected")
	}

	// 4. StateStreaming -> must succeed
	_ = sess.Transition(StateStreaming, "test")
	if err := sess.SendInput(frame); err != nil {
		t.Fatalf("expected success in StateStreaming, got %v", err)
	}

	tr.mu.Lock()
	count := len(tr.sentBytes)
	tr.mu.Unlock()
	if count != 1 {
		t.Fatalf("expected 1 wire message sent, got %d", count)
	}

	// Verify wire message is valid proto
	var received phonebridgev1.InputFrame
	if err := proto.Unmarshal(tr.sentBytes[0], &received); err != nil {
		t.Fatalf("failed to unmarshal sent frame: %v", err)
	}
	touch := received.GetTouch()
	if touch == nil || touch.GetNormalizedX() != 0.5 || touch.GetNormalizedY() != 0.5 {
		t.Errorf("unmarshaled touch event mismatch: %v", touch)
	}
}

func TestSession_SendInput_ValidationAndSecurity(t *testing.T) {
	sess := NewSession("sess-input-2", DefaultSessionConfig(), nil, nil)
	tr := &mockInputTransport{}
	sess.tr = tr
	sess.state = StateStreaming

	// Nil frame
	if err := sess.SendInput(nil); err != input.ErrNilFrame {
		t.Errorf("expected ErrNilFrame, got %v", err)
	}

	// Invalid coordinates: out of range
	invalidX := &phonebridgev1.InputFrame{
		Event: &phonebridgev1.InputFrame_Touch{
			Touch: &phonebridgev1.TouchEvent{
				Action:      phonebridgev1.TouchEvent_ACTION_MOVE,
				NormalizedX: 1.05,
				NormalizedY: 0.5,
			},
		},
	}
	if err := sess.SendInput(invalidX); err != input.ErrInvalidCoordinate {
		t.Errorf("expected ErrInvalidCoordinate for X > 1.0, got %v", err)
	}

	// Invalid coordinates: NaN
	nanY := &phonebridgev1.InputFrame{
		Event: &phonebridgev1.InputFrame_Touch{
			Touch: &phonebridgev1.TouchEvent{
				Action:      phonebridgev1.TouchEvent_ACTION_MOVE,
				NormalizedX: 0.5,
				NormalizedY: float32(math.NaN()),
			},
		},
	}
	if err := sess.SendInput(nanY); err != input.ErrInvalidCoordinate {
		t.Errorf("expected ErrInvalidCoordinate for NaN Y, got %v", err)
	}

	// Text length cap: > 1024 bytes
	oversizedText := &phonebridgev1.InputFrame{
		Event: &phonebridgev1.InputFrame_Text{
			Text: &phonebridgev1.TextEvent{
				Text: strings.Repeat("x", input.MaxTextLengthBytes+1),
			},
		},
	}
	if err := sess.SendInput(oversizedText); err == nil {
		t.Error("expected error for oversized text")
	}

	// Global actions
	backAction := &phonebridgev1.InputFrame{
		Event: &phonebridgev1.InputFrame_Action{
			Action: &phonebridgev1.GlobalActionEvent{
				Type: phonebridgev1.GlobalActionEvent_TYPE_GLOBAL_ACTION_BACK,
			},
		},
	}
	if err := sess.SendInput(backAction); err != nil {
		t.Errorf("expected success for global action, got %v", err)
	}
}

func TestSessionManager_SendInput_Scoping(t *testing.T) {
	mgr := NewSessionManager(DefaultSessionConfig(), nil, nil, nil)
	ctx := context.Background()

	frame := &phonebridgev1.InputFrame{
		Event: &phonebridgev1.InputFrame_Touch{
			Touch: &phonebridgev1.TouchEvent{
				Action:      phonebridgev1.TouchEvent_ACTION_DOWN,
				NormalizedX: 0.5,
				NormalizedY: 0.5,
			},
		},
	}

	// 1. No active session
	if err := mgr.SendInput(ctx, "sess-1", frame); err == nil {
		t.Fatal("expected error with no active session")
	}

	// 2. Active session with mismatched ID
	sess := NewSession("sess-active", DefaultSessionConfig(), nil, nil)
	tr := &mockInputTransport{}
	sess.tr = tr
	sess.state = StateStreaming

	mgr.mu.Lock()
	mgr.activeSess = sess
	mgr.mu.Unlock()

	if err := mgr.SendInput(ctx, "wrong-sess-id", frame); err == nil {
		t.Fatal("expected error for mismatched session ID")
	}

	// 3. Active session with matching ID
	if err := mgr.SendInput(ctx, "sess-active", frame); err != nil {
		t.Fatalf("expected success with matching session ID, got %v", err)
	}

	// 4. Empty session ID accepts active session
	if err := mgr.SendInput(ctx, "", frame); err != nil {
		t.Fatalf("expected success with empty session ID defaulting to active, got %v", err)
	}
}
