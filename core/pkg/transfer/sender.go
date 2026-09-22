package transfer

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

// runSender drives one outbound transfer on its own lifecycle context:
//
//	PENDING → (FileAccept) → ACTIVE → FileChunk×N → FileComplete → (FileResult) → terminal
//
// The source is read exactly once: each 64 KiB block is hashed while it is sent,
// and the digest travels in FileComplete (DEC-024), so there is no pre-hash pass
// and no staging copy. Memory is bounded by the chunk buffer plus the transport
// watermark, never by the file size.
func (e *Engine) runSender(rec *record) {
	if rec.file != nil {
		defer func() { _ = rec.file.Close() }()
	}
	defer rec.cancel()

	chunkSize := e.cfg.ChunkSize
	offer := &phonebridgev1.FileOffer{
		TransferId:  rec.info.TransferID,
		Filename:    rec.info.Filename,
		MimeType:    rec.info.MimeType,
		SizeBytes:   rec.info.SizeBytes,
		ChunkSize:   uint32(chunkSize),
		CreatedAtMs: e.nowMs(),
	}
	if err := e.send(OfferFrame(offer)); err != nil {
		e.finish(rec, StateFailed, ReasonInterrupted, err.Error(), "")
		return
	}

	// Wait for the accept. Every way out of this wait is terminal: a refused
	// offer, a local cancel, a lost channel, or a peer that never answers.
	select {
	case accept := <-rec.acceptCh:
		if accept == nil || !accept.Accept {
			reason := ReasonUnspecified
			message := "peer refused the transfer"
			if accept != nil {
				reason = ReasonForCode(accept.Code)
				if reason == ReasonUnspecified {
					reason = ReasonUnsupportedPeer
				}
				if accept.Reason != "" {
					message = accept.Reason
				}
			}
			e.finish(rec, StateFailed, reason, message, "")
			return
		}
	case <-rec.ctx.Done():
		e.finishCancelled(rec)
		return
	case <-e.channelDone():
		e.finish(rec, StateFailed, ReasonInterrupted, "transfer channel closed before the peer accepted", "")
		return
	case <-time.After(e.cfg.OfferTimeout):
		e.finish(rec, StateFailed, ReasonInterrupted, fmt.Sprintf("no FileAccept within %v", e.cfg.OfferTimeout), "")
		return
	}

	rec.setState(StateActive)
	e.emit(rec)

	hasher := sha256.New()
	buf := make([]byte, chunkSize)
	var index, sent uint64

	for {
		if rec.isFinished() {
			return
		}
		if rec.ctx.Err() != nil {
			e.finishCancelled(rec)
			return
		}

		n, rerr := io.ReadFull(rec.file, buf)
		if rerr != nil && !errors.Is(rerr, io.EOF) && !errors.Is(rerr, io.ErrUnexpectedEOF) {
			e.finish(rec, StateFailed, ReasonStorageFailed, fmt.Sprintf("read %s: %v", rec.path, rerr), "")
			return
		}
		if n > 0 {
			if err := e.awaitDrain(rec); err != nil {
				if rec.ctx.Err() != nil {
					e.finishCancelled(rec)
					return
				}
				e.finish(rec, StateFailed, ReasonInterrupted, err.Error(), "")
				return
			}
			_, _ = hasher.Write(buf[:n])
			chunk := &phonebridgev1.FileChunk{
				TransferId: rec.info.TransferID,
				ChunkIndex: index,
				Offset:     index * uint64(chunkSize),
				Data:       buf[:n], // marshalled into the frame before SendFrame returns
			}
			if err := e.send(ChunkFrame(chunk)); err != nil {
				e.finish(rec, StateFailed, ReasonInterrupted, err.Error(), "")
				return
			}
			sent += uint64(n)
			index++
			rec.setProgress(sent)
			e.emitThrottled(rec)
		}
		if n < len(buf) {
			break // short read = last chunk (or an empty file)
		}
	}

	complete := &phonebridgev1.FileComplete{
		TransferId:   rec.info.TransferID,
		SizeBytes:    sent,
		Sha256Digest: hasher.Sum(nil),
	}
	if err := e.send(CompleteFrame(complete)); err != nil {
		e.finish(rec, StateFailed, ReasonInterrupted, err.Error(), "")
		return
	}
	rec.setProgress(sent)
	e.emit(rec)

	// The receiver's verdict is the completion ack: "no answer" is never
	// success, and the wait scales with the file size because verifying and
	// promoting a large file is not instantaneous.
	select {
	case result := <-rec.resultCh:
		if result != nil && result.Committed {
			e.finish(rec, StateComplete, ReasonNone, "", result.SavedName)
			return
		}
		reason := ReasonUnspecified
		message := "peer did not commit the transfer"
		if result != nil {
			reason = ReasonForCode(result.Code)
			if reason == ReasonUnspecified {
				reason = ReasonStorageFailed
			}
			if result.Reason != "" {
				message = result.Reason
			}
		}
		e.finish(rec, StateFailed, reason, message, "")
	case <-rec.ctx.Done():
		e.finishCancelled(rec)
	case <-e.channelDone():
		e.finish(rec, StateFailed, ReasonInterrupted, "transfer channel closed before the peer confirmed the file", "")
	case <-time.After(e.cfg.resultTimeout(sent)):
		e.finish(rec, StateFailed, ReasonInterrupted,
			fmt.Sprintf("no FileResult within %v", e.cfg.resultTimeout(sent)), "")
	}
}

// finishCancelled ends a sender whose context was cancelled, honouring the
// recorded cause (local cancel, peer cancel, channel loss).
func (e *Engine) finishCancelled(rec *record) {
	reason, message := rec.cancelCause()
	state := StateFailed
	if reason == ReasonCancelledByUser || reason == ReasonCancelledByPeer {
		state = StateCancelled
	}
	e.finish(rec, state, reason, message, "")
}

// channelDone returns the bound channel's done channel, or a nil channel (never
// ready in a select) when nothing is bound.
func (e *Engine) channelDone() <-chan struct{} {
	e.mu.Lock()
	ch := e.ch
	e.mu.Unlock()
	if ch == nil {
		return nil
	}
	return ch.Done()
}
