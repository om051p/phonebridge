package input

import (
	"errors"
	"sync"
	"time"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

var (
	ErrRateLimited = errors.New("input: rate limit exceeded")
)

const (
	// DefaultRateLimitHz is the maximum sustained events per second (120 Hz).
	DefaultRateLimitHz = 120.0
	// DefaultBurstCapacity is the maximum burst allowance before throttling kicks in.
	DefaultBurstCapacity = 16.0
)

// Limiter provides token-bucket rate limiting with move coalescing.
// Down/Up/Key/Text/Action events consume tokens; intermediate Move events can be dropped.
type Limiter struct {
	mu            sync.Mutex
	rate          float64
	burst         float64
	tokens        float64
	lastCheck     time.Time
	droppedMoves  uint64
	acceptedCount uint64
}

// NewLimiter creates a rate limiter with the given sustained Hz and burst capacity.
func NewLimiter(rateHz, burstCapacity float64) *Limiter {
	if rateHz <= 0 {
		rateHz = DefaultRateLimitHz
	}
	if burstCapacity <= 0 {
		burstCapacity = DefaultBurstCapacity
	}
	return &Limiter{
		rate:      rateHz,
		burst:     burstCapacity,
		tokens:    burstCapacity,
		lastCheck: time.Now(),
	}
}

// Allow returns true if the input frame is admitted, or false if throttled.
// If the frame is an intermediate touch move, it is safely dropped/coalesced when throttled.
func (l *Limiter) Allow(frame *phonebridgev1.InputFrame) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(l.lastCheck).Seconds()
	l.lastCheck = now

	// Replenish tokens
	l.tokens += elapsed * l.rate
	if l.tokens > l.burst {
		l.tokens = l.burst
	}

	isMove := false
	if t, ok := frame.Event.(*phonebridgev1.InputFrame_Touch); ok && t.Touch != nil {
		if t.Touch.Action == phonebridgev1.TouchEvent_ACTION_MOVE {
			isMove = true
		}
	}

	if l.tokens >= 1.0 {
		l.tokens -= 1.0
		l.acceptedCount++
		return true
	}

	// Below 1.0 token: if it's a move, drop it silently (coalesce to next position)
	if isMove {
		l.droppedMoves++
		return false
	}

	// Critical event (down/up/text/action/key) gets a small overdraft if burst not exhausted
	if l.tokens >= 0.0 {
		l.tokens -= 1.0
		l.acceptedCount++
		return true
	}

	return false
}

// Stats returns the accepted and dropped move counters.
func (l *Limiter) Stats() (accepted, droppedMoves uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.acceptedCount, l.droppedMoves
}
