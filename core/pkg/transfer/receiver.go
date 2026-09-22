package transfer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

// handleOffer validates an inbound offer and, when it is acceptable, creates the
// destination writer and answers FileAccept. Every refusal is typed and travels
// back over the wire, because a silent drop would leave the sender guessing.
func (e *Engine) handleOffer(offer *phonebridgev1.FileOffer) {
	if offer == nil || offer.TransferId == "" {
		return // nothing to attribute; a frame without an id cannot be answered
	}
	if e.cfg.Destination == nil {
		e.refuseOffer(offer, phonebridgev1.Code_CODE_UNAVAILABLE, ReasonUnsupportedPeer,
			"this device does not accept inbound files", true)
		return
	}
	if rec := e.activeInbound(); rec != nil {
		e.refuseOffer(offer, phonebridgev1.Code_CODE_TRANSFER_BUSY, ReasonBusy,
			"a transfer is already in flight", true)
		return
	}
	if e.knownTransfer(offer.TransferId) {
		// Replayed or duplicated offer: refuse without adding a second history
		// entry for an id that already exists (DEC-024).
		e.refuseOffer(offer, phonebridgev1.Code_CODE_INVALID_ARGUMENT, ReasonProtocolError,
			"transfer id already known", false)
		return
	}

	filename, err := SanitizeFilename(offer.Filename)
	if err != nil {
		f, _ := IsFailure(err)
		e.refuseOffer(offer, f.Code, f.Reason, f.Message, true)
		return
	}
	if offer.SizeBytes > e.cfg.MaxFileSize {
		e.refuseOffer(offer, phonebridgev1.Code_CODE_FILE_TOO_LARGE, ReasonTooLarge,
			fmt.Sprintf("file of %d bytes exceeds the %d-byte policy", offer.SizeBytes, e.cfg.MaxFileSize), true)
		return
	}
	if offer.ChunkSize == 0 || offer.ChunkSize > MaxReceiveChunkSize {
		e.refuseOffer(offer, phonebridgev1.Code_CODE_INVALID_ARGUMENT, ReasonProtocolError,
			fmt.Sprintf("chunk size %d is outside 1..%d", offer.ChunkSize, MaxReceiveChunkSize), true)
		return
	}
	if len(offer.Sha256Digest) != 0 && len(offer.Sha256Digest) != sha256.Size {
		e.refuseOffer(offer, phonebridgev1.Code_CODE_INVALID_ARGUMENT, ReasonProtocolError,
			fmt.Sprintf("declared digest is %d bytes, want %d", len(offer.Sha256Digest), sha256.Size), true)
		return
	}

	committer, err := e.cfg.Destination.Begin(Meta{
		TransferID: offer.TransferId,
		Filename:   filename,
		MimeType:   offer.MimeType,
		SizeBytes:  offer.SizeBytes,
	})
	if err != nil {
		f, ok := IsFailure(err)
		if !ok {
			f = newFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, ReasonStorageFailed, "%v", err)
		}
		e.refuseOffer(offer, f.Code, f.Reason, f.Message, true)
		return
	}

	recCtx, cancel := context.WithCancel(e.baseCtx)
	rec := &record{
		lastChunkMs: e.nowMs(),
		info: Info{
			TransferID:   offer.TransferId,
			Direction:    DirectionInbound,
			State:        StatePending,
			PeerDeviceID: e.peerIDSnapshot(),
			Filename:     filename,
			MimeType:     offer.MimeType,
			SizeBytes:    offer.SizeBytes,
			StartedAtMs:  e.nowMs(),
		},
		ctx:       recCtx,
		cancel:    cancel,
		committer: committer,
		hasher:    sha256.New(),
		offer:     offer,
		chunkSize: offer.ChunkSize,
	}
	e.mu.Lock()
	e.active[offer.TransferId] = rec
	e.mu.Unlock()

	e.emit(rec)
	rec.setState(StateActive)
	e.emit(rec)
	go e.watchStall(rec)

	if err := e.send(AcceptFrame(&phonebridgev1.FileAccept{
		TransferId: offer.TransferId,
		Accept:     true,
		Code:       phonebridgev1.Code_CODE_OK,
	})); err != nil {
		e.abortInbound(rec, ReasonInterrupted, err.Error(), false)
	}
}

// refuseOffer answers an unacceptable offer with a typed refusal and (unless the
// id is a known duplicate) records it so the user can see why nothing arrived.
func (e *Engine) refuseOffer(offer *phonebridgev1.FileOffer, code phonebridgev1.Code, reason Reason, message string, record bool) {
	if code == 0 {
		code = CodeForReason(reason)
	}
	_ = e.send(AcceptFrame(&phonebridgev1.FileAccept{
		TransferId: offer.TransferId,
		Accept:     false,
		Code:       code,
		Reason:     message,
	}))
	if !record {
		return
	}
	info := Info{
		TransferID:   offer.TransferId,
		Direction:    DirectionInbound,
		State:        StateFailed,
		Filename:     offer.Filename,
		MimeType:     offer.MimeType,
		SizeBytes:    offer.SizeBytes,
		StartedAtMs:  e.nowMs(),
		FinishedAtMs: e.nowMs(),
		ReasonCode:   reason,
		ErrorMessage: message,
	}
	e.mu.Lock()
	e.history = append([]Info{info}, e.history...)
	if len(e.history) > e.cfg.HistoryLimit {
		e.history = e.history[:e.cfg.HistoryLimit]
	}
	e.mu.Unlock()
	e.publish(info)
}

// handleChunk validates one data frame and streams it into the destination.
// The reliable ordered DataChannel already provides ordering; this is the
// validation that turns a bug (or a fabricated stream) into a typed abort
// instead of a corrupt file.
func (e *Engine) handleChunk(chunk *phonebridgev1.FileChunk) {
	if chunk == nil || chunk.TransferId == "" {
		return
	}
	rec := e.findRecord(chunk.TransferId)
	if rec == nil {
		return // late frame after a cancel/finish, or an id we never accepted
	}
	if rec.direction() != DirectionInbound {
		e.abortTransfer(rec, ReasonProtocolError, "peer sent a chunk for an outbound transfer")
		return
	}

	rec.mu.Lock()
	if rec.finished {
		rec.mu.Unlock()
		return
	}
	if rec.info.State != StateActive {
		rec.mu.Unlock()
		e.abortInbound(rec, ReasonProtocolError, "chunk arrived before the offer was accepted", true)
		return
	}
	expectedIndex := rec.expectedIndex
	chunkSize := uint64(rec.chunkSize)
	total := rec.offer.SizeBytes
	received := rec.received
	rec.mu.Unlock()

	switch {
	case chunk.ChunkIndex != expectedIndex:
		e.abortInbound(rec, ReasonProtocolError,
			fmt.Sprintf("chunk index %d arrived where %d was expected", chunk.ChunkIndex, expectedIndex), true)
		return
	case chunk.Offset != expectedIndex*chunkSize:
		e.abortInbound(rec, ReasonProtocolError,
			fmt.Sprintf("chunk offset %d does not match index %d × chunk size %d", chunk.Offset, expectedIndex, chunkSize), true)
		return
	case len(chunk.Data) == 0:
		e.abortInbound(rec, ReasonProtocolError, "empty chunk", true)
		return
	case uint64(len(chunk.Data)) > chunkSize:
		e.abortInbound(rec, ReasonProtocolError,
			fmt.Sprintf("chunk of %d bytes exceeds the declared chunk size %d", len(chunk.Data), chunkSize), true)
		return
	case received+uint64(len(chunk.Data)) > total:
		e.abortInbound(rec, ReasonProtocolError, "chunk overruns the declared file size", true)
		return
	case uint64(len(chunk.Data)) != chunkSize && received+uint64(len(chunk.Data)) != total:
		e.abortInbound(rec, ReasonProtocolError, "only the final chunk may be shorter than the chunk size", true)
		return
	}

	rec.mu.Lock()
	if rec.finished || rec.committer == nil || rec.info.State != StateActive {
		rec.mu.Unlock()
		return
	}
	n, werr := rec.committer.Write(chunk.Data)
	if werr != nil {
		rec.mu.Unlock()
		f, _ := IsFailure(werr)
		if f == nil {
			f = newFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, ReasonStorageFailed, "%v", werr)
		}
		e.abortInbound(rec, f.Reason, f.Message, true)
		return
	}
	if n > 0 {
		_, _ = rec.hasher.Write(chunk.Data[:n])
		rec.received += uint64(n)
		rec.expectedIndex++
		rec.info.BytesTransferred = rec.received
		rec.lastChunkMs = e.nowMs()
	}
	rec.mu.Unlock()

	e.emitThrottled(rec)
}

// handleComplete verifies the streamed size and digest, promotes the file and
// answers FileResult — the sender's only completion ack.
func (e *Engine) handleComplete(complete *phonebridgev1.FileComplete) {
	if complete == nil || complete.TransferId == "" {
		return
	}
	rec := e.findRecord(complete.TransferId)
	if rec == nil {
		return
	}
	if rec.direction() != DirectionInbound {
		e.abortTransfer(rec, ReasonProtocolError, "peer sent a completion for an outbound transfer")
		return
	}

	rec.mu.Lock()
	if rec.finished {
		rec.mu.Unlock()
		return
	}
	if rec.info.State != StateActive {
		rec.mu.Unlock()
		e.abortInbound(rec, ReasonProtocolError, "completion arrived before the offer was accepted", true)
		return
	}
	computed := rec.hasher.Sum(nil)
	received := rec.received
	total := rec.offer.SizeBytes
	declared := rec.offer.Sha256Digest
	committer := rec.committer
	rec.mu.Unlock()

	switch {
	case complete.SizeBytes != received || complete.SizeBytes != total:
		e.abortInbound(rec, ReasonChecksumMismatch,
			fmt.Sprintf("size mismatch: transferred %d of %d bytes, completion declared %d", received, total, complete.SizeBytes), true)
		return
	case len(complete.Sha256Digest) != sha256.Size:
		e.abortInbound(rec, ReasonChecksumMismatch,
			fmt.Sprintf("completion digest is %d bytes, want %d", len(complete.Sha256Digest), sha256.Size), true)
		return
	case !bytes.Equal(complete.Sha256Digest, computed):
		e.abortInbound(rec, ReasonChecksumMismatch, "SHA-256 of the received bytes does not match the completion digest", true)
		return
	case len(declared) == sha256.Size && !bytes.Equal(declared, computed):
		e.abortInbound(rec, ReasonChecksumMismatch, "SHA-256 of the received bytes does not match the digest declared in the offer", true)
		return
	}

	rec.setState(StateVerifying)
	e.emit(rec)

	savedName, err := committer.Commit()
	if err != nil {
		f, ok := IsFailure(err)
		if !ok {
			f = newFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, ReasonStorageFailed, "%v", err)
		}
		rec.mu.Lock()
		rec.committer = nil
		rec.mu.Unlock()
		_ = e.send(ResultFrame(&phonebridgev1.FileResult{
			TransferId: rec.info.TransferID,
			Committed:  false,
			Code:       f.Code,
			Reason:     f.Message,
		}))
		e.finish(rec, StateFailed, f.Reason, f.Message, "")
		return
	}

	rec.mu.Lock()
	rec.committer = nil
	rec.mu.Unlock()

	_ = e.send(ResultFrame(&phonebridgev1.FileResult{
		TransferId: rec.info.TransferID,
		Committed:  true,
		Code:       phonebridgev1.Code_CODE_OK,
		SavedName:  savedName,
	}))
	e.finish(rec, StateComplete, ReasonNone, "", savedName)
}

// handleAccept routes an answer to the waiting sender.
func (e *Engine) handleAccept(accept *phonebridgev1.FileAccept) {
	if accept == nil {
		return
	}
	rec := e.findRecord(accept.TransferId)
	if rec == nil || rec.direction() != DirectionOutbound {
		return
	}
	if rec.isFinished() {
		return
	}
	select {
	case rec.acceptCh <- accept:
	default:
		// A second accept for the same offer is a protocol violation; ignoring
		// it is safe because the first one already decided the transfer.
	}
}

// handleResult routes the receiver's verdict to the waiting sender.
func (e *Engine) handleResult(result *phonebridgev1.FileResult) {
	if result == nil {
		return
	}
	rec := e.findRecord(result.TransferId)
	if rec == nil || rec.direction() != DirectionOutbound {
		return
	}
	if rec.isFinished() {
		return
	}
	select {
	case rec.resultCh <- result:
	default:
	}
}

// handleCancelFrame aborts the addressed transfer because the peer asked.
func (e *Engine) handleCancelFrame(cancel *phonebridgev1.FileCancel) {
	if cancel == nil || cancel.TransferId == "" {
		return
	}
	rec := e.findRecord(cancel.TransferId)
	if rec == nil {
		return
	}
	message := cancel.Reason
	if message == "" {
		message = "peer cancelled the transfer"
	}
	if rec.direction() == DirectionInbound {
		e.abortInbound(rec, ReasonCancelledByPeer, message, false)
		return
	}
	rec.markCancel(ReasonCancelledByPeer, message)
	rec.cancel()
	e.finish(rec, StateCancelled, ReasonCancelledByPeer, message, "")
}

// abortInbound discards a staged partial and ends the transfer. When sendResult
// is set the peer is told the verdict, so its sender never waits out a timeout.
func (e *Engine) abortInbound(rec *record, reason Reason, message string, sendResult bool) {
	rec.mu.Lock()
	committer := rec.committer
	rec.committer = nil
	rec.mu.Unlock()

	if committer != nil {
		if err := committer.Abort(); err != nil && message == "" {
			message = err.Error()
		}
	}
	if sendResult && !rec.isFinished() {
		_ = e.send(ResultFrame(&phonebridgev1.FileResult{
			TransferId: rec.info.TransferID,
			Committed:  false,
			Code:       CodeForReason(reason),
			Reason:     message,
		}))
	}
	state := StateFailed
	switch reason {
	case ReasonCancelledByUser, ReasonCancelledByPeer:
		state = StateCancelled
	case ReasonNone:
		state = StateComplete
	}
	e.finish(rec, state, reason, message, "")
}

// watchStall fails an inbound transfer that stops making progress while bytes
// remain (DEC-024's stall timeout). Without it, a peer that dies without closing
// the channel would leave a staged partial and a PENDING UI row forever.
func (e *Engine) watchStall(rec *record) {
	interval := e.cfg.StallTimeout / 4
	if interval <= 0 || interval > time.Second {
		interval = time.Second
	}
	if interval < 50*time.Millisecond {
		interval = 50 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	stallMs := uint64(e.cfg.StallTimeout.Milliseconds())
	for {
		select {
		case <-rec.ctx.Done():
			return
		case <-ticker.C:
		}
		rec.mu.Lock()
		finished := rec.finished
		state := rec.info.State
		last := rec.lastChunkMs
		rec.mu.Unlock()
		if finished {
			return
		}
		if state != StateActive {
			continue
		}
		now := e.nowMs()
		if now >= last && now-last > stallMs {
			e.abortInbound(rec, ReasonInterrupted,
				fmt.Sprintf("no chunk received for %v", e.cfg.StallTimeout), true)
			return
		}
	}
}

// abortTransfer ends an outbound transfer discovered to be invalid by the peer
// side of the state machine (it has no staged file of its own).
func (e *Engine) abortTransfer(rec *record, reason Reason, message string) {
	rec.markCancel(reason, message)
	rec.cancel()
	e.finish(rec, StateFailed, reason, message, "")
}
