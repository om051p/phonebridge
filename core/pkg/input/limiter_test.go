package input_test

import (
	"testing"

	"github.com/om051p/phonebridge/core/pkg/input"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

func TestLimiter_CoalescesMovesWhenExhausted(t *testing.T) {
	// 10 Hz rate, 2 token burst
	limiter := input.NewLimiter(10, 2)

	down := &phonebridgev1.InputFrame{
		Event: &phonebridgev1.InputFrame_Touch{
			Touch: &phonebridgev1.TouchEvent{
				Action: phonebridgev1.TouchEvent_ACTION_DOWN,
			},
		},
	}
	move := &phonebridgev1.InputFrame{
		Event: &phonebridgev1.InputFrame_Touch{
			Touch: &phonebridgev1.TouchEvent{
				Action: phonebridgev1.TouchEvent_ACTION_MOVE,
			},
		},
	}

	// First 2 should be allowed (burst)
	if !limiter.Allow(down) {
		t.Fatal("expected first event to be allowed")
	}
	if !limiter.Allow(move) {
		t.Fatal("expected second event to be allowed")
	}

	// Third event immediate (tokens <= 0) should drop move
	dropped := 0
	for i := 0; i < 5; i++ {
		if !limiter.Allow(move) {
			dropped++
		}
	}
	if dropped == 0 {
		t.Errorf("expected moves to be dropped under rate limit exhaustion")
	}

	accepted, droppedMoves := limiter.Stats()
	if droppedMoves == 0 {
		t.Errorf("expected droppedMoves > 0, got %d", droppedMoves)
	}
	if accepted == 0 {
		t.Errorf("expected accepted > 0, got %d", accepted)
	}
}
