package frames

import (
	"context"
	"sync"
)

// perSubscriberBuffer is the number of complete frames buffered per
// StreamFrames subscription. Small on purpose: latency stays low and the
// drop-oldest policy (latest-wins) keeps memory bounded at ~2 frames of
// resident JPEG per subscriber.
const perSubscriberBuffer = 2

// Hub broadcasts completed frames to StreamFrames subscribers using
// latest-wins backpressure, and owns frame-session lifecycle: frames only
// flow between BeginSession and EndSession, and EndSession closes every
// subscriber channel so no stale frame can outlive its session (the stream
// handler then simply waits for the next session or the client's cancel).
//
// All queues are fixed-capacity with drop-oldest semantics; Publish never
// blocks, so frame load can never propagate back into the AU path, the
// display sink, StreamEvents, clipboard, transfer or reconnect handling.
type Hub struct {
	mu sync.Mutex

	active    bool
	nextID    uint64        // assigned to frames within the current session
	beginWait chan struct{} // closed when a session begins (fresh after End)
	subs      map[chan *Frame]struct{}
	published uint64 // frames published this process (monotonic)
	dropped   uint64 // frames dropped by subscriber backpressure
}

// NewHub creates an idle Hub (no session active).
func NewHub() *Hub {
	return &Hub{
		subs:      make(map[chan *Frame]struct{}),
		beginWait: make(chan struct{}),
	}
}

// BeginSession opens the frame window (called when a session's guarded sink
// chain is installed). Idempotent per session.
func (h *Hub) BeginSession() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.active {
		return
	}
	h.active = true
	h.nextID = 0
	close(h.beginWait) // release subscribers waiting for a session
}

// EndSession closes the frame window and every subscriber channel: handlers
// stop sending immediately and late Publish calls are dropped. Idempotent.
func (h *Hub) EndSession() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.active {
		return
	}
	h.active = false
	for ch := range h.subs {
		close(ch)
		delete(h.subs, ch)
	}
	h.beginWait = make(chan struct{}) // arm the next session's waiters
}

// Active reports whether a frame session is open.
func (h *Hub) Active() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.active
}

// Subscribe registers a per-session frame channel. It blocks until a session
// begins or ctx is done. The channel is closed when that session ends; the
// caller then either resubscribes (next session) or observes ctx cancellation.
// Receiving yields *Frame values shared across subscribers (immutable).
func (h *Hub) Subscribe(ctx context.Context) (<-chan *Frame, error) {
	for {
		h.mu.Lock()
		if h.active {
			ch := make(chan *Frame, perSubscriberBuffer)
			h.subs[ch] = struct{}{}
			h.mu.Unlock()
			return ch, nil
		}
		wait := h.beginWait
		h.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-wait:
			// Session began (or was re-armed); retry registration.
		}
	}
}

// Unsubscribe removes a channel without closing it (the handler owns close
// semantics only via EndSession; cancellation paths call this).
func (h *Hub) Unsubscribe(ch <-chan *Frame) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subs {
		if sub == ch {
			delete(h.subs, sub)
			return
		}
	}
}

// Publish fans a completed frame out with latest-wins backpressure: if a
// subscriber's buffer is full the OLDEST buffered frame is dropped (visible
// to that client as a frame_id gap). Never blocks.
func (h *Hub) Publish(f *Frame) {
	if f == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.published++
	if !h.active {
		return
	}
	f.ID = h.nextID + 1
	h.nextID = f.ID
	for ch := range h.subs {
		select {
		case ch <- f:
		default:
			// Drop oldest, then retry once (buffer cap >= 1).
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- f:
			default:
			}
			h.dropped++
		}
	}
}

// Stats returns cumulative publish/drop counters (diagnostics/tests).
func (h *Hub) Stats() (published, dropped uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.published, h.dropped
}
