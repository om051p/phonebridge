package frames

import (
	"context"
	"errors"
	"sync"
)

// perSubscriberBuffer is the number of complete frames buffered per
// StreamFrames subscription. Small on purpose: latency stays low and the
// drop-oldest policy (latest-wins) keeps memory bounded at ~2 frames of
// resident JPEG per subscriber.
const perSubscriberBuffer = 2

// ErrSessionNotCurrent is returned by EndSession for a session that is no
// longer the hub's current one. A caller seeing it must NOT treat the call as
// a failure: it is the expected outcome of a late terminal callback from a
// session that has already been superseded (see EndSession's doc comment).
var ErrSessionNotCurrent = errors.New("frames: session is not the current hub session")

// Hub broadcasts completed frames to StreamFrames subscribers using
// latest-wins backpressure, and owns frame-session lifecycle: frames only
// flow between BeginSession and EndSession, and EndSession closes every
// subscriber channel so no stale frame can outlive its session (the stream
// handler then simply waits for the next session or the client's cancel).
//
// All queues are fixed-capacity with drop-oldest semantics; Publish never
// blocks, so frame load can never propagate back into the AU path, the
// display sink, StreamEvents, clipboard, transfer or reconnect handling.
//
// Session ownership is a TOKEN, not a flag. The session layer reports
// terminal states from a callback that runs after the transition has already
// been applied and its lock released, so a session N terminal callback can
// land after session N+1 has begun — and the manager accepts that new session
// precisely because N's state is already terminal. A bare `active` bool lets
// that late EndSession close the NEW session's window and permanently stops
// frame delivery until another BeginSession happens: the daemon still reports
// STREAMING with access units flowing, but ffmpeg's output can no longer
// reach any subscriber. Correlating Begin/End by token makes each EndSession
// apply only to its own session, so a late callback degrades to a no-op
// (ErrSessionNotCurrent) instead of a cross-session kill.
type Hub struct {
	mu sync.Mutex

	current   bool   // a session is open
	token     uint64 // identifies the open session; bumped by every Begin
	nextToken uint64 // last token handed out
	nextID    uint64 // assigned to frames within the current session
	beginWait chan struct{} // closed when a session begins (fresh after End)
	subs      map[chan *Frame]struct{}
	published uint64 // frames published this process (monotonic)
	dropped   uint64 // frames dropped by subscriber backpressure

	// Subscriber presence (nobody watching vs at least one watcher) is reported
	// to an interested observer so that work which only exists to feed
	// StreamFrames (the H.264 → MJPEG converter) can be started and stopped with
	// it. lastActive makes the notification edge-triggered regardless of how
	// many subscribers come and go between two observations.
	observer    func(bool)
	observerGen uint64
	nextObsGen  uint64
	lastActive  bool
}

// NewHub creates an idle Hub (no session active).
func NewHub() *Hub {
	return &Hub{
		subs:      make(map[chan *Frame]struct{}),
		beginWait: make(chan struct{}),
	}
}

// BeginSession opens the frame window and returns the token identifying it.
// The token must be passed back to EndSession so teardown can only ever close
// its own session.
//
// A session BEGUN WHILE ANOTHER IS STILL OPEN SUPERSEDES IT: the previous
// window ends (its subscribers are closed, exactly as EndSession would) and a
// fresh token is minted, so the superseded session's later EndSession becomes
// inert. This matters because a session layer may accept a new session while
// the previous one's teardown callback has not yet run; returning the old
// token then would make the new session share the old session's identity, and
// the old session's late EndSession would kill the new window — the very
// cross-session stall this token exists to prevent.
func (h *Hub) BeginSession() uint64 {
	h.mu.Lock()
	if h.current {
		for ch := range h.subs {
			close(ch)
			delete(h.subs, ch)
		}
	}
	h.nextToken++
	h.token = h.nextToken
	h.current = true
	h.nextID = 0
	close(h.beginWait)                // release subscribers waiting for a session
	h.beginWait = make(chan struct{}) // arm the next waiters
	notify, active := h.presenceChangeLocked()
	h.mu.Unlock()
	if notify != nil {
		notify(active)
	}
	return h.token
}

// EndSession(tok) closes the frame window and every subscriber channel:
// handlers stop sending immediately and late Publish calls are dropped. It
// applies ONLY to the session that minted tok.
//
// token <= 0 means "end whatever is current" and exists solely for tests and
// token-less callers; the session layer always passes the token it was given
// by BeginSession.
//
// Returns ErrSessionNotCurrent when tok does not identify the open session
// (already ended, or superseded by a newer one). A superseded session's
// terminal callback is the normal source of that error, and its correct
// handling is to ignore it: the newer session's window stays open, which is
// exactly the cross-session stall this correlation prevents.
func (h *Hub) EndSession(tok uint64) error {
	h.mu.Lock()
	err, after := h.endSessionLocked(tok)
	h.mu.Unlock()
	if after != nil {
		after()
	}
	return err
}

// EndSessionIfCurrent ends the open session only if it is still the one the
// caller began, atomically under the hub lock. Use it when the caller has to
// validate ownership against external state (e.g. "is my session still the
// manager's active one?"): doing that check outside the lock would leave a
// window in which a superseding BeginSession lands between the check and the
// EndSession, letting the caller close a window it no longer owns.
func (h *Hub) EndSessionIfCurrent(tok uint64) error {
	h.mu.Lock()
	if !h.current || tok == 0 || tok != h.token {
		h.mu.Unlock()
		return ErrSessionNotCurrent
	}
	err, after := h.endSessionLocked(tok)
	h.mu.Unlock()
	if after != nil {
		after()
	}
	return err
}

// endSessionLocked closes the window and returns a function the caller must run
// AFTER releasing h.mu: closing subscriber channels can change subscriber
// presence, and the observer is external code that must never run under the hub
// lock.
func (h *Hub) endSessionLocked(tok uint64) (error, func()) {
	if !h.current {
		return nil, nil // already ended: idempotent
	}
	if tok > 0 && tok != h.token {
		return ErrSessionNotCurrent, nil
	}
	h.current = false
	for ch := range h.subs {
		close(ch)
		delete(h.subs, ch)
	}
	h.beginWait = make(chan struct{}) // arm the next session's waiters
	notify, active := h.presenceChangeLocked()
	if notify == nil {
		return nil, nil
	}
	return nil, func() { notify(active) }
}

// SetSubscriberObserver registers fn to be told when StreamFrames subscriber
// presence changes between "nobody is watching" and "at least one watcher".
//
// It exists because converting H.264 to MJPEG is work nobody asked for when no
// client is watching: the tap uses this signal to run its converter only while
// frames have somewhere to go. fn must not call back into the Hub.
//
// One observer is supported (there is one active session at a time); a newer
// registration replaces the older one and is primed with the current presence.
// The returned function detaches this registration if it is still the active
// one, so a closing session can never be signalled after it is gone.
func (h *Hub) SetSubscriberObserver(fn func(bool)) func() {
	h.mu.Lock()
	h.nextObsGen++
	gen := h.nextObsGen
	h.observer = fn
	h.observerGen = gen
	active := len(h.subs) > 0
	h.lastActive = active
	h.mu.Unlock()

	if fn != nil && active {
		fn(true)
	}

	return func() {
		h.mu.Lock()
		if h.observerGen == gen {
			h.observer = nil
			h.observerGen = 0
		}
		h.mu.Unlock()
	}
}

// presenceChangeLocked reports the observer to notify and the new presence when
// presence changed since the last notification. h.mu must be held; the returned
// function must be invoked only after it is released.
func (h *Hub) presenceChangeLocked() (func(bool), bool) {
	active := len(h.subs) > 0
	if active == h.lastActive {
		return nil, active
	}
	h.lastActive = active
	return h.observer, active
}

// Active reports whether a frame session is open.
func (h *Hub) Active() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.current
}

// Subscribe registers a per-session frame channel. It blocks until a session
// begins or ctx is done. The channel is closed when that session ends; the
// caller then either resubscribes (next session) or observes ctx cancellation.
// Receiving yields *Frame values shared across subscribers (immutable).
func (h *Hub) Subscribe(ctx context.Context) (<-chan *Frame, error) {
	for {
		h.mu.Lock()
		if h.current {
			ch := make(chan *Frame, perSubscriberBuffer)
			h.subs[ch] = struct{}{}
			notify, active := h.presenceChangeLocked()
			h.mu.Unlock()
			if notify != nil {
				notify(active)
			}
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
	for sub := range h.subs {
		if sub == ch {
			delete(h.subs, sub)
			notify, active := h.presenceChangeLocked()
			h.mu.Unlock()
			if notify != nil {
				notify(active)
			}
			return
		}
	}
	h.mu.Unlock()
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
	if !h.current {
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

// SubscriberCount reports how many StreamFrames subscriptions are registered.
//
// It is here for lifecycle decisions and their tests: a subscription that is
// never released keeps the hub buffering frames into a channel nobody reads, so
// "nobody is watching" has to be observable.
func (h *Hub) SubscriberCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// Stats returns cumulative publish/drop counters (diagnostics/tests).
func (h *Hub) Stats() (published, dropped uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.published, h.dropped
}