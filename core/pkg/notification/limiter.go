package notification

import (
	"crypto/sha256"
	"errors"
	"sync"
	"time"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

var (
	ErrRateLimited = errors.New("notification: rate limit exceeded")
)

const (
	// DefaultRateLimitHz is the maximum sustained events per second (20 Hz).
	DefaultRateLimitHz = 20.0
	// DefaultBurstCapacity is the maximum burst allowance.
	DefaultBurstCapacity = 10.0
	// DefaultDedupWindow is the sliding window within which identical updates are suppressed.
	DefaultDedupWindow = 500 * time.Millisecond
)

type dedupEntry struct {
	hash      [32]byte
	timestamp time.Time
}

// Limiter provides token-bucket rate limiting and duplicate payload suppression.
type Limiter struct {
	mu             sync.Mutex
	rate           float64
	burst          float64
	tokens         float64
	lastCheck      time.Time
	dedupWindow    time.Duration
	recentHashes   map[string]dedupEntry // key -> last seen hash & time
	droppedCount   uint64
	acceptedCount  uint64
	dedupDropCount uint64
}

// NewLimiter creates a notification rate limiter with the specified sustained rate and burst capacity.
func NewLimiter(rateHz, burstCapacity float64, dedupWindow time.Duration) *Limiter {
	if rateHz <= 0 {
		rateHz = DefaultRateLimitHz
	}
	if burstCapacity <= 0 {
		burstCapacity = DefaultBurstCapacity
	}
	if dedupWindow <= 0 {
		dedupWindow = DefaultDedupWindow
	}
	return &Limiter{
		rate:         rateHz,
		burst:        burstCapacity,
		tokens:       burstCapacity,
		lastCheck:    time.Now(),
		dedupWindow:  dedupWindow,
		recentHashes: make(map[string]dedupEntry),
	}
}

// Allow checks if the frame is admitted.
// Returns true if allowed, false if dropped due to rate limit or duplicate suppression.
func (l *Limiter) Allow(frame *phonebridgev1.NotificationFrame) bool {
	if frame == nil {
		return false
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	// 1. Deduplication check for posted notifications
	if posted := frame.GetPosted(); posted != nil {
		h := computeContentHash(posted)
		if entry, exists := l.recentHashes[posted.Key]; exists {
			if entry.hash == h && now.Sub(entry.timestamp) < l.dedupWindow {
				l.dedupDropCount++
				return false
			}
		}
		l.recentHashes[posted.Key] = dedupEntry{hash: h, timestamp: now}
		// Periodically prune stale entries
		if len(l.recentHashes) > 256 {
			l.pruneStaleLocked(now)
		}
	} else if removed := frame.GetRemoved(); removed != nil {
		delete(l.recentHashes, removed.Key)
	}

	// 2. Token bucket replenishment
	elapsed := now.Sub(l.lastCheck).Seconds()
	l.lastCheck = now

	l.tokens += elapsed * l.rate
	if l.tokens > l.burst {
		l.tokens = l.burst
	}

	// 3. Token consumption
	if l.tokens < 1.0 {
		l.droppedCount++
		return false
	}

	l.tokens -= 1.0
	l.acceptedCount++
	return true
}

func computeContentHash(p *phonebridgev1.NotificationPosted) [32]byte {
	h := sha256.New()
	h.Write([]byte(p.Key))
	h.Write([]byte{0})
	h.Write([]byte(p.Title))
	h.Write([]byte{0})
	h.Write([]byte(p.Text))
	h.Write([]byte{0})
	h.Write([]byte(p.SubText))
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

func (l *Limiter) pruneStaleLocked(now time.Time) {
	cutoff := now.Add(-l.dedupWindow * 2)
	for k, v := range l.recentHashes {
		if v.timestamp.Before(cutoff) {
			delete(l.recentHashes, k)
		}
	}
}

// Stats returns the limiter counters.
func (l *Limiter) Stats() (accepted, dropped, dedupDropped uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.acceptedCount, l.droppedCount, l.dedupDropCount
}
