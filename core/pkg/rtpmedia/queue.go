package rtpmedia

import (
	"context"
	"sync"
)

// Access unit frame handed from the capture producer to the send side.
type Frame struct {
	Data   []byte // Annex-B access unit
	PTSUs  int64  // MediaCodec presentation timestamp, µs (monotonic, no base reset)
	Key    bool   // true for IDR access units
	PushNs int64  // optional host clock at push (push→send latency evidence)
}

// Queue is the bounded access-unit queue between the Kotlin capture path and
// the single-writer send loop (DEC-020 drop policy, validated as 256 slots in
// Spike 04; max observed depth 4–16 with zero drops under production loads).
//
// Drop policy — the producer never blocks:
//   - push of a non-key frame onto a full queue drops the *incoming* frame
//     (the cheapest thing to lose; every later P-frame depends on it anyway);
//   - push of a key frame onto a full queue evicts oldest frames until it
//     fits (an I-frame is the decoder's only re-entry point and must pass).
type Queue struct {
	mu     sync.Mutex
	cond   *sync.Cond // wake-up for PopWait (broadcast on push/close/cancel)
	items  []Frame
	cap    int
	closed bool

	Depth    int64 // high-water mark of queue length (atomic-ish under mu)
	Pushed   int64
	Dropped  int64 // non-key frames dropped on full queue
	Evicted  int64 // frames evicted (including keys) to admit a key frame
	EvictedK int64 // evicted frames that were keyframes
}

// defaultQueueDepth is the spike-validated production queue capacity.
const defaultQueueDepth = 256

// NewQueue returns a queue holding at most cap access units. A cap <= 0
// defaults to 256, the spike-validated capacity.
func NewQueue(cap int) *Queue {
	if cap < 1 {
		cap = defaultQueueDepth
	}
	q := &Queue{items: make([]Frame, 0, cap), cap: cap}
	q.cond = sync.NewCond(&q.mu)
	return q
}

// Cap returns the configured capacity.
func (q *Queue) Cap() int { return q.cap }

// Len returns the current number of queued frames.
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// Push enqueues f per the drop policy and reports whether it was admitted.
// Push on a closed queue is rejected. Push wakes one blocked PopWait.
func (q *Queue) Push(f Frame) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return false
	}
	q.Pushed++
	if len(q.items) >= q.cap {
		if !f.Key {
			q.Dropped++
			return false
		}
		for len(q.items) >= q.cap {
			oldest := q.items[0]
			q.items = q.items[1:]
			q.Evicted++
			if oldest.Key {
				q.EvictedK++
			}
		}
	}
	q.items = append(q.items, f)
	if int64(len(q.items)) > q.Depth {
		q.Depth = int64(len(q.items))
	}
	q.cond.Signal()
	return true
}

// Close marks the queue closed and wakes all blocked PopWait calls (they
// return with ok=false). Close is idempotent and safe for concurrent use.
// Frames already queued remain consumable via Pop/PopWait-drain semantics:
// PopWait returns them until the queue is empty, then reports closure.
func (q *Queue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.closed = true
	q.cond.Broadcast()
}

// Closed reports whether Close has been called.
func (q *Queue) Closed() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.closed
}

// PopWait blocks until a frame is available, the queue is closed, or ctx is
// cancelled, and reports ok=false on closure/cancellation. It is the
// replacement for the spike's 2 ms polling loop: the single-writer send loop
// sleeps in the kernel instead of spinning, at identical ordering
// guarantees. A nil ctx means wait indefinitely (until Close).
func (q *Queue) PopWait(ctx context.Context) (Frame, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	// Bridge context cancellation into the cond broadcast: cond.Wait cannot
	// select on channels. The bridge lives exactly as long as this call.
	cancelled := make(chan struct{})
	defer close(cancelled)
	go func() {
		select {
		case <-ctx.Done():
			q.cond.Broadcast()
		case <-cancelled:
		}
	}()
	for {
		if len(q.items) > 0 {
			f := q.items[0]
			q.items = q.items[1:]
			return f, true
		}
		if q.closed {
			return Frame{}, false
		}
		select {
		case <-ctx.Done():
			return Frame{}, false
		default:
		}
		q.cond.Wait()
	}
}

// Pop dequeues the oldest frame, or ok=false when empty.
func (q *Queue) Pop() (Frame, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return Frame{}, false
	}
	f := q.items[0]
	q.items = q.items[1:]
	return f, true
}
