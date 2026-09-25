//go:build android || jni

package main

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/om051p/phonebridge/core/pkg/input"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"google.golang.org/protobuf/proto"
)

// InputHost defines the Kotlin host callbacks that receive remote input.
type InputHost interface {
	OnTouch(action int32, pointerID uint32, normX, normY, pressure float32) bool
	OnKey(action int32, keyCode int32, metaState uint32) bool
	OnText(text string) bool
	OnScroll(normX, normY, deltaX, deltaY float32) bool
	OnGlobalAction(actionType int32) bool
}

// InputBridge coordinates between the Android Kotlin host and incoming remote input.
type InputBridge struct {
	mu          sync.Mutex
	host        InputHost
	initialized atomic.Bool
	limiter     *input.Limiter

	acceptedCount atomic.Uint64
	droppedCount  atomic.Uint64
}

var globalInput atomic.Pointer[InputBridge]

func currentInputBridge() *InputBridge {
	if b := globalInput.Load(); b != nil {
		return b
	}
	b := &InputBridge{
		limiter: input.NewLimiter(input.DefaultRateLimitHz, input.DefaultBurstCapacity),
	}
	globalInput.Store(b)
	return b
}

func (b *InputBridge) Init(host InputHost) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.host = host
	if b.limiter == nil {
		b.limiter = input.NewLimiter(input.DefaultRateLimitHz, input.DefaultBurstCapacity)
	}
	b.initialized.Store(true)
	return nil
}

func (b *InputBridge) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.host = nil
	b.initialized.Store(false)
}

// OnRemoteBytes receives raw protobuf InputFrame bytes from the WebRTC "input" DataChannel.
func (b *InputBridge) OnRemoteBytes(data []byte) error {
	if !b.initialized.Load() {
		return errors.New("input: bridge not initialized")
	}

	var frame phonebridgev1.InputFrame
	if err := proto.Unmarshal(data, &frame); err != nil {
		b.droppedCount.Add(1)
		return fmt.Errorf("input: unmarshal frame: %w", err)
	}

	if err := input.ValidateInputFrame(&frame); err != nil {
		b.droppedCount.Add(1)
		return err
	}

	b.mu.Lock()
	lim := b.limiter
	h := b.host
	b.mu.Unlock()

	if h == nil {
		b.droppedCount.Add(1)
		return errors.New("input: host not registered")
	}

	if lim != nil && !lim.Allow(&frame) {
		b.droppedCount.Add(1)
		return input.ErrRateLimited
	}

	b.acceptedCount.Add(1)

	switch ev := frame.Event.(type) {
	case *phonebridgev1.InputFrame_Touch:
		t := ev.Touch
		h.OnTouch(int32(t.Action), t.PointerId, t.NormalizedX, t.NormalizedY, t.Pressure)
	case *phonebridgev1.InputFrame_Key:
		k := ev.Key
		h.OnKey(int32(k.Action), k.KeyCode, k.MetaState)
	case *phonebridgev1.InputFrame_Text:
		h.OnText(ev.Text.Text)
	case *phonebridgev1.InputFrame_Scroll:
		s := ev.Scroll
		h.OnScroll(s.NormalizedX, s.NormalizedY, s.DeltaX, s.DeltaY)
	case *phonebridgev1.InputFrame_Action:
		h.OnGlobalAction(int32(ev.Action.Type))
	default:
		return input.ErrNoEvent
	}

	return nil
}
