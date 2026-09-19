package clipboard

import (
	"container/list"
	"sync"
	"time"
)

type echoEntry struct {
	digest  [32]byte
	addedAt time.Time
}

// EchoFilter implements the DEC-023 loop and echo suppression mechanism.
// It tracks recent SHA-256 digests in a thread-safe, bounded-memory LRU cache
// with a fixed capacity (default 32) and time-to-live (default 5,000 ms).
//
// Because neither Wayland nor Android platform protocols expose writer identity,
// echo cycles (Android → Linux → Android and Linux → Android → Linux) are prevented
// by dropping events whose SHA-256 digest matches a recently recorded digest within TTL.
type EchoFilter struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	clock    Clock
	entries  map[[32]byte]*list.Element
	lru      *list.List
}

// NewEchoFilter constructs an EchoFilter with the specified capacity, TTL, and clock.
// If capacity <= 0, DefaultSuppressionCapacity (32) is used.
// If ttl <= 0, DefaultSuppressionTTL (5000 ms) is used.
// If clock is nil, the system wall clock is used.
func NewEchoFilter(capacity int, ttl time.Duration, clock Clock) *EchoFilter {
	if capacity <= 0 {
		capacity = DefaultSuppressionCapacity
	}
	if ttl <= 0 {
		ttl = DefaultSuppressionTTL
	}
	if clock == nil {
		clock = realClock{}
	}

	return &EchoFilter{
		capacity: capacity,
		ttl:      ttl,
		clock:    clock,
		entries:  make(map[[32]byte]*list.Element, capacity),
		lru:      list.New(),
	}
}

// Record stores a digest in the suppression filter with the current timestamp.
// If the digest already exists, its timestamp is refreshed and it is moved to the
// front of the LRU queue. If the filter is at capacity, the oldest entry is evicted.
func (f *EchoFilter) Record(digest [32]byte) {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := f.clock.Now()
	f.purgeExpiredLocked(now)

	if elem, ok := f.entries[digest]; ok {
		elem.Value.(*echoEntry).addedAt = now
		f.lru.MoveToFront(elem)
		return
	}

	if f.lru.Len() >= f.capacity {
		oldest := f.lru.Back()
		if oldest != nil {
			oldDigest := oldest.Value.(*echoEntry).digest
			delete(f.entries, oldDigest)
			f.lru.Remove(oldest)
		}
	}

	elem := f.lru.PushFront(&echoEntry{digest: digest, addedAt: now})
	f.entries[digest] = elem
}

// IsEcho reports whether the given digest was recorded within the TTL window.
// If an entry exists but has exceeded its TTL, it is pruned and IsEcho returns false.
func (f *EchoFilter) IsEcho(digest [32]byte) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	elem, ok := f.entries[digest]
	if !ok {
		return false
	}

	now := f.clock.Now()
	entry := elem.Value.(*echoEntry)
	if now.Sub(entry.addedAt) > f.ttl {
		delete(f.entries, digest)
		f.lru.Remove(elem)
		return false
	}

	return true
}

// Len returns the current number of unexpired entries in the filter.
func (f *EchoFilter) Len() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := f.clock.Now()
	f.purgeExpiredLocked(now)
	return f.lru.Len()
}

// Clear removes all entries from the filter.
func (f *EchoFilter) Clear() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.entries = make(map[[32]byte]*list.Element, f.capacity)
	f.lru.Init()
}

// purgeExpiredLocked removes all entries older than TTL.
// Must be called with f.mu held.
func (f *EchoFilter) purgeExpiredLocked(now time.Time) {
	var next *list.Element
	for e := f.lru.Back(); e != nil; e = next {
		entry := e.Value.(*echoEntry)
		if now.Sub(entry.addedAt) > f.ttl {
			next = e.Prev()
			delete(f.entries, entry.digest)
			f.lru.Remove(e)
		} else {
			// Because non-refreshed entries preserve arrival order, we can check towards front.
			// But since MoveToFront can place refreshed items at front, we check all up to 32 entries.
			next = e.Prev()
		}
	}
}
