package transfer

import (
	"context"
	"hash"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

// record is one transfer in either direction. A single mutex guards both the
// UI-facing state and the local I/O (staged writes, commit/abort), which is what
// makes a local cancel race-free against an in-flight chunk: the writer finishes
// its 64 KiB write and the cancel then aborts a quiescent file.
//
// The mutex is never held across a network send or a channel select, so a
// stalled peer cannot block Cancel/DetachChannel behind a slow transfer.
type record struct {
	mu sync.Mutex

	info Info

	ctx    context.Context
	cancel context.CancelFunc

	cancelReason  Reason
	cancelMessage string
	finished      bool
	lastEmitMs    uint64
	lastChunkMs   uint64

	// Sender side.
	path     string
	file     *os.File
	acceptCh chan *phonebridgev1.FileAccept
	resultCh chan *phonebridgev1.FileResult

	// Receiver side.
	committer     Committer
	hasher        hash.Hash
	offer         *phonebridgev1.FileOffer
	chunkSize     uint32
	expectedIndex uint64
	received      uint64
}

func (r *record) snapshot() Info {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.info
}

func (r *record) direction() Direction {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.info.Direction
}

func (r *record) setState(s State) {
	r.mu.Lock()
	r.info.State = s
	r.mu.Unlock()
}

func (r *record) setProgress(n uint64) {
	r.mu.Lock()
	if n <= r.info.SizeBytes || r.info.SizeBytes == 0 {
		r.info.BytesTransferred = n
	}
	r.mu.Unlock()
}

func (r *record) markCancel(reason Reason, message string) {
	r.mu.Lock()
	if !r.finished {
		r.cancelReason = reason
		r.cancelMessage = message
	}
	r.mu.Unlock()
}

// cancelCause reports what a cancelled context means for this transfer. A
// context cancel without a recorded cause is a transport loss, because the only
// other cancel sources record their cause first.
func (r *record) cancelCause() (Reason, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancelReason == ReasonUnspecified {
		return ReasonInterrupted, "transfer cancelled without a recorded cause"
	}
	return r.cancelReason, r.cancelMessage
}

func (r *record) isFinished() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.finished
}

// emitBudget reports whether a progress event is due, and consumes the budget.
func (r *record) emitBudget(nowMs uint64, interval time.Duration) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lastEmitMs == 0 || nowMs-r.lastEmitMs >= uint64(interval.Milliseconds()) {
		r.lastEmitMs = nowMs
		return true
	}
	return false
}

// finish performs the first terminal transition for a record and reports it.
// Later calls are no-ops, so a cancel racing a completion cannot emit two
// terminal events.
func (r *record) finish(state State, reason Reason, message, savedName string, nowMs uint64) (Info, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished {
		return r.info, false
	}
	r.finished = true
	r.info.State = state
	r.info.ReasonCode = reason
	r.info.ErrorMessage = message
	r.info.SavedName = savedName
	r.info.FinishedAtMs = nowMs
	return r.info, true
}

// Engine owns file-transfer state for one session. It is transport-agnostic:
// AttachChannel supplies the dedicated "transfer" DataChannel, and every rule in
// this package is provable with a fake Channel.
type Engine struct {
	cfg        Config
	baseCtx    context.Context
	cancelBase context.CancelFunc

	mu     sync.Mutex
	ch     Channel
	peerID string
	active map[string]*record
	// history is newest-first and capped by Config.HistoryLimit.
	history []Info
}

// NewEngine builds an engine with defaults applied and the configuration
// validated.
func NewEngine(cfg Config) (*Engine, error) {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Engine{
		cfg:        cfg,
		baseCtx:    ctx,
		cancelBase: cancel,
		active:     make(map[string]*record),
	}, nil
}

// Config returns the effective configuration (defaults resolved).
func (e *Engine) Config() Config { return e.cfg }

// Close interrupts every in-flight transfer and stops accepting new ones.
func (e *Engine) Close() {
	e.DetachChannel(ReasonInterrupted, "transfer engine stopped")
	e.cancelBase()
}

// SetPeerDeviceID records which device the currently bound channel leads to, so
// transfers are attributed to a device in the activity history (DEC-023 "who"
// rule applied to transfers). It is safe to call before or after AttachChannel.
func (e *Engine) SetPeerDeviceID(peerID string) {
	e.mu.Lock()
	e.peerID = peerID
	e.mu.Unlock()
}

// AttachChannel binds the dedicated "transfer" DataChannel. A previously bound
// channel is treated as lost first: replacing the channel means every in-flight
// transfer is interrupted, never silently continued on a new transport (DEC-024
// has no resume).
func (e *Engine) AttachChannel(ch Channel) {
	if ch == nil {
		return
	}
	e.DetachChannel(ReasonInterrupted, "transfer channel replaced")
	e.mu.Lock()
	e.ch = ch
	e.mu.Unlock()
}

// DetachChannel marks the channel gone (DataChannel closed, session lost, or
// reconnect) and interrupts every in-flight transfer with a typed reason.
func (e *Engine) DetachChannel(reason Reason, message string) {
	e.mu.Lock()
	e.ch = nil
	records := make([]*record, 0, len(e.active))
	for _, rec := range e.active {
		records = append(records, rec)
	}
	e.mu.Unlock()

	if len(records) == 0 {
		return
	}
	if reason == ReasonUnspecified {
		reason = ReasonInterrupted
	}
	if message == "" {
		message = "transfer channel closed"
	}
	for _, rec := range records {
		if rec.direction() == DirectionInbound {
			e.abortInbound(rec, reason, message, false)
			continue
		}
		rec.markCancel(reason, message)
		rec.cancel()
		e.finish(rec, StateFailed, reason, message, "")
	}
}

// DetachChannelIf detaches only if ch is still the bound channel. A closing
// session uses it so tearing down an already-replaced transport cannot
// interrupt transfers running on its successor (the same generation discipline
// the media plane uses for transport callbacks, DEC-022).
func (e *Engine) DetachChannelIf(ch Channel, reason Reason, message string) {
	e.mu.Lock()
	bound := e.ch == ch
	e.mu.Unlock()
	if !bound {
		return
	}
	e.DetachChannel(reason, message)
}

// ChannelReady reports whether a transfer channel is bound.
func (e *Engine) ChannelReady() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.ch != nil
}

// peerIDSnapshot reads the peer attribution under the lock; records are built
// outside it, so this is the only safe way to read it there.
func (e *Engine) peerIDSnapshot() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.peerID
}

// SendFile offers a local regular file to the peer and returns the new transfer
// id immediately; the transfer itself runs on its own lifecycle context, so a
// request-scoped caller context (a gRPC handler, for example) cannot kill it
// when the call returns.
func (e *Engine) SendFile(ctx context.Context, path, name string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", newFailure(phonebridgev1.Code_CODE_UNAVAILABLE, ReasonNoSession, "send file: %v", err)
	}

	e.mu.Lock()
	ch := e.ch
	outbound := e.activeLocked(DirectionOutbound)
	e.mu.Unlock()
	if ch == nil {
		return "", newFailure(phonebridgev1.Code_CODE_UNAVAILABLE, ReasonNoSession, "no active session with file-transfer support")
	}
	if outbound != nil {
		return "", newFailure(phonebridgev1.Code_CODE_TRANSFER_BUSY, ReasonBusy, "a transfer is already in flight (id=%s)", outbound.snapshot().TransferID)
	}

	file, info, err := openRegularReadonly(path)
	if err != nil {
		return "", err
	}
	defer func() {
		if file != nil {
			_ = file.Close()
		}
	}()

	size := uint64(info.Size())
	if size > e.cfg.MaxFileSize {
		return "", newFailure(phonebridgev1.Code_CODE_FILE_TOO_LARGE, ReasonTooLarge,
			"file of %d bytes exceeds the %d-byte policy", size, e.cfg.MaxFileSize)
	}

	proposed := name
	if proposed == "" {
		proposed = filepath.Base(path)
	}
	filename, err := SanitizeFilename(proposed)
	if err != nil {
		return "", err
	}
	id, err := NewTransferID()
	if err != nil {
		return "", err
	}

	recCtx, cancel := context.WithCancel(e.baseCtx)
	rec := &record{
		info: Info{
			TransferID:   id,
			Direction:    DirectionOutbound,
			State:        StatePending,
			PeerDeviceID: e.peerIDSnapshot(),
			Filename:     filename,
			MimeType:     MimeForName(filename),
			SizeBytes:    size,
			StartedAtMs:  e.nowMs(),
		},
		ctx:      recCtx,
		cancel:   cancel,
		path:     path,
		file:     file,
		acceptCh: make(chan *phonebridgev1.FileAccept, 1),
		resultCh: make(chan *phonebridgev1.FileResult, 1),
	}
	file = nil // ownership passes to the sender goroutine

	e.mu.Lock()
	e.active[id] = rec
	e.mu.Unlock()

	e.emit(rec)
	go e.runSender(rec)
	return id, nil
}

// OnFrame dispatches one decoded DataChannel message. It is called from the
// transport's receive goroutine, which is why the receive path does its I/O
// inline: SCTP flow control then turns a slow disk into backpressure instead of
// an unbounded queue.
func (e *Engine) OnFrame(data []byte) {
	frame, err := DecodeFrame(data)
	if err != nil {
		f, ok := IsFailure(err)
		if !ok {
			f = newFailure(phonebridgev1.Code_CODE_INVALID_ARGUMENT, ReasonProtocolError, "%v", err)
		}
		// A frame we cannot even parse is only attributable when exactly one
		// inbound transfer is in flight; abandoning it now fails the sender
		// fast instead of letting it hang until its timeout.
		if rec := e.soleInbound(); rec != nil {
			e.abortInbound(rec, f.Reason, f.Message, true)
		}
		return
	}

	switch body := frame.Body.(type) {
	case *phonebridgev1.TransferFrame_Offer:
		e.handleOffer(body.Offer)
	case *phonebridgev1.TransferFrame_Accept:
		e.handleAccept(body.Accept)
	case *phonebridgev1.TransferFrame_Chunk:
		e.handleChunk(body.Chunk)
	case *phonebridgev1.TransferFrame_Complete:
		e.handleComplete(body.Complete)
	case *phonebridgev1.TransferFrame_Result:
		e.handleResult(body.Result)
	case *phonebridgev1.TransferFrame_Cancel:
		e.handleCancelFrame(body.Cancel)
	}
}

// Cancel aborts an in-flight transfer in either direction.
func (e *Engine) Cancel(ctx context.Context, transferID string) error {
	if err := ctx.Err(); err != nil {
		return newFailure(phonebridgev1.Code_CODE_UNAVAILABLE, ReasonNoSession, "cancel: %v", err)
	}
	e.mu.Lock()
	rec := e.active[transferID]
	e.mu.Unlock()
	if rec == nil {
		return newFailure(phonebridgev1.Code_CODE_NOT_FOUND, ReasonUnspecified, "no in-flight transfer %q", transferID)
	}
	if rec.isFinished() {
		return nil
	}

	reason := ReasonCancelledByUser
	message := "cancelled locally"

	// Tell the peer first (best effort): if the channel is already gone there is
	// nothing to tell, and the local cleanup still has to happen.
	_ = e.send(CancelFrame(&phonebridgev1.FileCancel{TransferId: transferID,
		Code:   CodeForReason(reason),
		Reason: message,
	}))

	if rec.direction() == DirectionInbound {
		e.abortInbound(rec, reason, message, false)
		return nil
	}
	rec.markCancel(reason, message)
	rec.cancel()
	return nil
}

// List returns in-flight transfers followed by the finished history, newest
// first.
func (e *Engine) List() []Info {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Info, 0, len(e.active)+len(e.history))
	for _, rec := range e.active {
		out = append(out, rec.snapshot())
	}
	out = append(out, e.history...)
	return out
}

// Get returns one transfer snapshot.
func (e *Engine) Get(transferID string) (Info, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if rec, ok := e.active[transferID]; ok {
		return rec.snapshot(), true
	}
	for _, info := range e.history {
		if info.TransferID == transferID {
			return info, true
		}
	}
	return Info{}, false
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func (e *Engine) nowMs() uint64 { return uint64(e.cfg.Now().UnixMilli()) }

// publish delivers an event without engine locks held.
func (e *Engine) publish(info Info) {
	if e.cfg.OnEvent != nil {
		e.cfg.OnEvent(Event{Info: info})
	}
}

// emit publishes a non-terminal snapshot.
func (e *Engine) emit(rec *record) {
	e.publish(rec.snapshot())
}

// emitThrottled publishes at most one progress event per ProgressInterval.
func (e *Engine) emitThrottled(rec *record) {
	if rec.emitBudget(e.nowMs(), e.cfg.ProgressInterval) {
		e.emit(rec)
	}
}

// finish performs the terminal transition, moves the record to the history and
// publishes exactly one terminal event. It also releases the record's lifecycle
// context, which is what stops the inbound stall watchdog.
func (e *Engine) finish(rec *record, state State, reason Reason, message, savedName string) {
	info, ok := rec.finish(state, reason, message, savedName, e.nowMs())
	if !ok {
		return
	}
	if rec.cancel != nil {
		rec.cancel()
	}
	e.mu.Lock()
	delete(e.active, info.TransferID)
	e.history = append([]Info{info}, e.history...)
	if len(e.history) > e.cfg.HistoryLimit {
		e.history = e.history[:e.cfg.HistoryLimit]
	}
	e.mu.Unlock()

	e.publish(info)
}

// send encodes and transmits one frame on the bound channel.
func (e *Engine) send(frame *phonebridgev1.TransferFrame) error {
	e.mu.Lock()
	ch := e.ch
	e.mu.Unlock()
	if ch == nil {
		return newFailure(phonebridgev1.Code_CODE_UNAVAILABLE, ReasonNoSession, "no transfer channel")
	}
	wire, err := EncodeFrame(frame)
	if err != nil {
		return err
	}
	if err := ch.SendFrame(e.baseCtx, wire); err != nil {
		return newFailure(phonebridgev1.Code_CODE_TRANSFER_INTERRUPTED, ReasonInterrupted, "send transfer frame: %v", err)
	}
	return nil
}

// awaitDrain applies outbound backpressure: the sender stops reading its source
// while the transport buffer is above the high-watermark (DEC-024).
func (e *Engine) awaitDrain(rec *record) error {
	e.mu.Lock()
	ch := e.ch
	e.mu.Unlock()
	if ch == nil {
		return newFailure(phonebridgev1.Code_CODE_UNAVAILABLE, ReasonNoSession, "no transfer channel")
	}
	if ch.BufferedAmount() <= e.cfg.HighWatermark {
		return nil
	}
	if err := ch.AwaitDrain(rec.ctx); err != nil {
		return err
	}
	return nil
}

// activeLocked returns the in-flight transfer in the given direction (e.mu held).
func (e *Engine) activeLocked(d Direction) *record {
	for _, rec := range e.active {
		if rec.info.Direction == d {
			return rec
		}
	}
	return nil
}

func (e *Engine) activeInbound() *record {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.activeLocked(DirectionInbound)
}

// soleInbound is the only inbound transfer, or nil when there is none. Used to
// attribute an unparseable frame to the transfer the peer is most plausibly
// corrupting.
func (e *Engine) soleInbound() *record {
	e.mu.Lock()
	defer e.mu.Unlock()
	var found *record
	for _, rec := range e.active {
		if rec.info.Direction != DirectionInbound {
			continue
		}
		if found != nil {
			return nil
		}
		found = rec
	}
	return found
}

// knownTransfer reports whether a transfer id is in flight or remembered, which
// is what refuses a replayed offer (DEC-024).
func (e *Engine) knownTransfer(transferID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.active[transferID]; ok {
		return true
	}
	for _, info := range e.history {
		if info.TransferID == transferID {
			return true
		}
	}
	return false
}

// findRecord returns the in-flight transfer with this id.
func (e *Engine) findRecord(transferID string) *record {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.active[transferID]
}
