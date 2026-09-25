//go:build android || jni

package main

import (
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/input"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"google.golang.org/protobuf/proto"
)

type mockInputHost struct {
	mu           sync.Mutex
	touches      []struct{ action int32; pointerID uint32; x, y, p float32 }
	keys         []struct{ action, keyCode int32; meta uint32 }
	texts        []string
	scrolls      []struct{ x, y, dx, dy float32 }
	actions      []int32
}

func (m *mockInputHost) OnTouch(action int32, pointerID uint32, normX, normY, pressure float32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.touches = append(m.touches, struct{ action int32; pointerID uint32; x, y, p float32 }{action, pointerID, normX, normY, pressure})
	return true
}

func (m *mockInputHost) OnKey(action int32, keyCode int32, metaState uint32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keys = append(m.keys, struct{ action, keyCode int32; meta uint32 }{action, keyCode, metaState})
	return true
}

func (m *mockInputHost) OnText(text string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.texts = append(m.texts, text)
	return true
}

func (m *mockInputHost) OnScroll(normX, normY, deltaX, deltaY float32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scrolls = append(m.scrolls, struct{ x, y, dx, dy float32 }{normX, normY, deltaX, deltaY})
	return true
}

func (m *mockInputHost) OnGlobalAction(actionType int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.actions = append(m.actions, actionType)
	return true
}

func TestInputBridge_LifecycleAndDispatch(t *testing.T) {
	b := &InputBridge{}
	host := &mockInputHost{}

	// 1. Before init -> must fail
	frame := &phonebridgev1.InputFrame{
		TimestampMs: uint64(time.Now().UnixMilli()),
		Event: &phonebridgev1.InputFrame_Touch{
			Touch: &phonebridgev1.TouchEvent{
				Action:      phonebridgev1.TouchEvent_ACTION_DOWN,
				NormalizedX: 0.3,
				NormalizedY: 0.7,
				Pressure:    1.0,
			},
		},
	}
	data, _ := proto.Marshal(frame)
	if err := b.OnRemoteBytes(data); err == nil {
		t.Fatal("expected error before Init")
	}

	// 2. Init
	if err := b.Init(host); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// 3. Dispatch touch
	if err := b.OnRemoteBytes(data); err != nil {
		t.Fatalf("OnRemoteBytes failed: %v", err)
	}
	if len(host.touches) != 1 || host.touches[0].x != 0.3 || host.touches[0].y != 0.7 {
		t.Fatalf("unexpected touch event dispatched: %v", host.touches)
	}

	// 4. Dispatch text
	textFrame := &phonebridgev1.InputFrame{
		Event: &phonebridgev1.InputFrame_Text{
			Text: &phonebridgev1.TextEvent{Text: "Testing 123"},
		},
	}
	textData, _ := proto.Marshal(textFrame)
	if err := b.OnRemoteBytes(textData); err != nil {
		t.Fatalf("OnRemoteBytes text failed: %v", err)
	}
	if len(host.texts) != 1 || host.texts[0] != "Testing 123" {
		t.Fatalf("unexpected text dispatched: %v", host.texts)
	}

	// 5. Dispatch global action
	actionFrame := &phonebridgev1.InputFrame{
		Event: &phonebridgev1.InputFrame_Action{
			Action: &phonebridgev1.GlobalActionEvent{
				Type: phonebridgev1.GlobalActionEvent_TYPE_GLOBAL_ACTION_BACK,
			},
		},
	}
	actionData, _ := proto.Marshal(actionFrame)
	if err := b.OnRemoteBytes(actionData); err != nil {
		t.Fatalf("OnRemoteBytes action failed: %v", err)
	}
	if len(host.actions) != 1 || host.actions[0] != int32(phonebridgev1.GlobalActionEvent_TYPE_GLOBAL_ACTION_BACK) {
		t.Fatalf("unexpected action dispatched: %v", host.actions)
	}

	// 6. Stop
	b.Stop()
	if err := b.OnRemoteBytes(data); err == nil {
		t.Fatal("expected error after Stop")
	}
}

func TestInputBridge_ValidationRejections(t *testing.T) {
	b := &InputBridge{}
	host := &mockInputHost{}
	_ = b.Init(host)

	// Malformed protobuf bytes
	if err := b.OnRemoteBytes([]byte("garbage non-protobuf bytes")); err == nil {
		t.Error("expected unmarshal error for garbage bytes")
	}

	// Coordinate out of bounds
	badCoord := &phonebridgev1.InputFrame{
		Event: &phonebridgev1.InputFrame_Touch{
			Touch: &phonebridgev1.TouchEvent{
				NormalizedX: -0.1,
				NormalizedY: 0.5,
			},
		},
	}
	d, _ := proto.Marshal(badCoord)
	if err := b.OnRemoteBytes(d); err != input.ErrInvalidCoordinate {
		t.Errorf("expected ErrInvalidCoordinate, got %v", err)
	}

	// NaN coordinate
	nanCoord := &phonebridgev1.InputFrame{
		Event: &phonebridgev1.InputFrame_Touch{
			Touch: &phonebridgev1.TouchEvent{
				NormalizedX: float32(math.NaN()),
				NormalizedY: 0.5,
			},
		},
	}
	d, _ = proto.Marshal(nanCoord)
	if err := b.OnRemoteBytes(d); err != input.ErrInvalidCoordinate {
		t.Errorf("expected ErrInvalidCoordinate for NaN, got %v", err)
	}

	// Text length cap
	bigText := &phonebridgev1.InputFrame{
		Event: &phonebridgev1.InputFrame_Text{
			Text: &phonebridgev1.TextEvent{
				Text: strings.Repeat("z", input.MaxTextLengthBytes+1),
			},
		},
	}
	d, _ = proto.Marshal(bigText)
	if err := b.OnRemoteBytes(d); err == nil {
		t.Error("expected error for oversized text")
	}
}
