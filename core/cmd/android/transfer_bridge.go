//go:build android || jni

package main

// Transfer bridge (DEC-024): the Android counterpart of the Linux daemon's
// transfer wiring. It owns one transfer.Engine for the app, attaches the
// dedicated "transfer" DataChannel when it opens, and exposes a flat surface to
// Kotlin (send/cancel/list/stats) plus a host seam for storage.
//
// The JNI wiring of TransferHost into this bridge lives in main.go
// (jniTransferHost + the nativeTransfer* exports), following the clipboard
// plane's split: transport-agnostic logic here, JNI glue in main.go.
//
// Like the clipboard bridge, this file is pure Go so the whole contract is
// host-testable under `go test -tags jni -race`.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"github.com/om051p/phonebridge/core/pkg/transfer"
)

// TransferStats is the diagnostic snapshot Kotlin renders in the UI and logs.
type TransferStats struct {
	Initialized    bool   `json:"initialized"`
	ChannelReady   bool   `json:"channel_ready"`
	ActiveCount    int    `json:"active_count"`
	HistoryCount   int    `json:"history_count"`
	SentFiles      uint64 `json:"sent_files"`
	ReceivedFiles  uint64 `json:"received_files"`
	FailedFiles    uint64 `json:"failed_files"`
	CancelledFiles uint64 `json:"cancelled_files"`
	LastError      string `json:"last_error,omitempty"`
}

// TransferBridge coordinates the Kotlin host and the Go transfer engine.
type TransferBridge struct {
	mu          sync.Mutex
	engine      *transfer.Engine
	host        TransferHost
	destination *PlatformDestination
	initialized atomic.Bool

	lastError atomic.Pointer[string]

	sentFiles      atomic.Uint64
	receivedFiles  atomic.Uint64
	failedFiles    atomic.Uint64
	cancelledFiles atomic.Uint64
}

var globalTransfer atomic.Pointer[TransferBridge]

func currentTransferBridge() *TransferBridge {
	if b := globalTransfer.Load(); b != nil {
		return b
	}
	b := &TransferBridge{}
	globalTransfer.Store(b)
	return b
}

// Init builds the transfer engine with the platform destination. Idempotent, so
// a second Init (a service restart) only refreshes the host reference.
//
// localPeerID is the Kotlin-owned device identity: the Go side has no identity
// of its own on Android, and the engine records the local device on every event
// for the activity UI.
func (b *TransferBridge) Init(host TransferHost, localPeerID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.initialized.Load() {
		b.host = host
		if b.destination != nil {
			b.destination.host = host
		}
		return nil
	}
	if host == nil {
		return errors.New("transfer: storage host is required")
	}

	b.host = host
	b.destination = NewPlatformDestination(host)

	eng, err := transfer.NewEngine(transfer.Config{
		LocalPeerID: localPeerID,
		Destination: b.destination,
		// The event stream is what the UI renders: progress, terminal states and
		// typed failures all arrive here, and the bridge mirrors the counters.
		OnEvent: func(evt transfer.Event) {
			b.observe(evt)
		},
	})
	if err != nil {
		return fmt.Errorf("transfer: failed to create engine: %w", err)
	}
	b.engine = eng
	b.initialized.Store(true)
	return nil
}

// observe mirrors terminal states into the diagnostic counters.
func (b *TransferBridge) observe(evt transfer.Event) {
	switch evt.Info.State {
	case transfer.StateComplete:
		if evt.Info.Direction == transfer.DirectionOutbound {
			b.sentFiles.Add(1)
		} else {
			b.receivedFiles.Add(1)
		}
	case transfer.StateFailed:
		b.failedFiles.Add(1)
		if evt.Info.ErrorMessage != "" {
			msg := evt.Info.ErrorMessage
			b.lastError.Store(&msg)
		}
	case transfer.StateCancelled:
		b.cancelledFiles.Add(1)
	}
}

// OnChannelOpen binds the dedicated "transfer" DataChannel and sets the peer
// attribution for the transfer history.
func (b *TransferBridge) OnChannelOpen(ch transfer.Channel, peerDeviceID string) {
	b.mu.Lock()
	eng := b.engine
	b.mu.Unlock()
	if eng == nil {
		return
	}
	eng.SetPeerDeviceID(peerDeviceID)
	eng.AttachChannel(ch)
}

// OnChannelClose interrupts every in-flight transfer: DEC-024 has no resume, so
// a lost channel fails the transfer and the user retries.
func (b *TransferBridge) OnChannelClose(ch transfer.Channel) {
	b.mu.Lock()
	eng := b.engine
	b.mu.Unlock()
	if eng == nil {
		return
	}
	eng.DetachChannelIf(ch, transfer.ReasonInterrupted, "transfer channel closed")
}

// OnRemoteBytes feeds one decoded DataChannel message into the engine.
func (b *TransferBridge) OnRemoteBytes(data []byte) error {
	b.mu.Lock()
	eng := b.engine
	b.mu.Unlock()
	if eng == nil {
		return errors.New("transfer: engine not initialized")
	}
	eng.OnFrame(data)
	return nil
}

// SendFile offers a local file to the peer and returns the new transfer id.
// The source is read from the app-private path the Kotlin picker resolved.
func (b *TransferBridge) SendFile(localPath, name string) (string, error) {
	b.mu.Lock()
	eng := b.engine
	b.mu.Unlock()
	if eng == nil {
		return "", errors.New("transfer: engine not initialized")
	}
	if localPath == "" {
		return "", transfer.NewFailure(phonebridgev1.Code_CODE_UNAVAILABLE, transfer.ReasonUnsupportedPeer,
			"no local path to send")
	}
	if name == "" {
		name = filepath.Base(localPath)
	}
	return eng.SendFile(context.Background(), localPath, name)
}

// Cancel aborts an in-flight transfer in either direction.
func (b *TransferBridge) Cancel(transferID string) error {
	b.mu.Lock()
	eng := b.engine
	b.mu.Unlock()
	if eng == nil {
		return errors.New("transfer: engine not initialized")
	}
	return eng.Cancel(context.Background(), transferID)
}

// List returns the engine's snapshots (in-flight plus recent history).
func (b *TransferBridge) List() []transfer.Info {
	b.mu.Lock()
	eng := b.engine
	b.mu.Unlock()
	if eng == nil {
		return nil
	}
	return eng.List()
}

// ChannelReady reports whether a transfer channel is bound to the engine.
func (b *TransferBridge) ChannelReady() bool {
	b.mu.Lock()
	eng := b.engine
	b.mu.Unlock()
	return eng != nil && eng.ChannelReady()
}

// Stop interrupts in-flight transfers and releases the engine. Idempotent.
func (b *TransferBridge) Stop() {
	b.mu.Lock()
	eng := b.engine
	b.engine = nil
	b.destination = nil
	b.host = nil
	b.mu.Unlock()

	if eng != nil {
		eng.Close()
	}
	b.initialized.Store(false)
}

// Stats returns the diagnostic snapshot.
func (b *TransferBridge) Stats() *TransferStats {
	b.mu.Lock()
	eng := b.engine
	b.mu.Unlock()

	stats := &TransferStats{
		Initialized:    b.initialized.Load(),
		SentFiles:      b.sentFiles.Load(),
		ReceivedFiles:  b.receivedFiles.Load(),
		FailedFiles:    b.failedFiles.Load(),
		CancelledFiles: b.cancelledFiles.Load(),
	}
	if msg := b.lastError.Load(); msg != nil {
		stats.LastError = *msg
	}
	if eng != nil {
		stats.ChannelReady = eng.ChannelReady()
		for _, info := range eng.List() {
			switch info.State {
			case transfer.StatePending, transfer.StateActive, transfer.StateVerifying:
				stats.ActiveCount++
			default:
				stats.HistoryCount++
			}
		}
	}
	return stats
}

// StatsJSON returns the stats snapshot marshaled to JSON.
func (b *TransferBridge) StatsJSON() []byte {
	out, _ := json.Marshal(b.Stats())
	return out
}

// ListJSON returns the transfer list marshaled to JSON for the Kotlin UI.
func (b *TransferBridge) ListJSON() []byte {
	type row struct {
		TransferID       string `json:"transfer_id"`
		Direction        string `json:"direction"`
		State            string `json:"state"`
		PeerDeviceID     string `json:"peer_device_id,omitempty"`
		Filename         string `json:"filename"`
		MimeType         string `json:"mime_type,omitempty"`
		SizeBytes        uint64 `json:"size_bytes"`
		BytesTransferred uint64 `json:"bytes_transferred"`
		StartedAtMs      uint64 `json:"started_at_ms"`
		FinishedAtMs     uint64 `json:"finished_at_ms,omitempty"`
		Reason           string `json:"reason,omitempty"`
		ErrorMessage     string `json:"error_message,omitempty"`
		SavedName        string `json:"saved_name,omitempty"`
	}
	rows := make([]row, 0)
	for _, info := range b.List() {
		rows = append(rows, row{
			TransferID:       info.TransferID,
			Direction:        info.Direction.String(),
			State:            info.State.String(),
			PeerDeviceID:     info.PeerDeviceID,
			Filename:         info.Filename,
			MimeType:         info.MimeType,
			SizeBytes:        info.SizeBytes,
			BytesTransferred: info.BytesTransferred,
			StartedAtMs:      info.StartedAtMs,
			FinishedAtMs:     info.FinishedAtMs,
			Reason:           info.ReasonCode.String(),
			ErrorMessage:     info.ErrorMessage,
			SavedName:        info.SavedName,
		})
	}
	out, _ := json.Marshal(rows)
	return out
}
