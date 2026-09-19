//go:build android || jni

package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/om051p/phonebridge/core/pkg/clipboard"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"google.golang.org/protobuf/proto"
)

// ClipboardHost defines the Kotlin host callbacks that the Go clipboard engine requires.
type ClipboardHost interface {
	WritePlatformClipboard(mimeType string, payload []byte) bool
	SendClipboardUpdate(wireBytes []byte) bool
	OnOversizedPayload(size int)
}

// ClipboardStats holds runtime diagnostics counters for clipboard synchronization.
type ClipboardStats struct {
	Initialized    bool   `json:"initialized"`
	Role           string `json:"role"`
	CurrentDigest  string `json:"current_digest,omitempty"`
	CurrentMime    string `json:"current_mime,omitempty"`
	CurrentBytes   int    `json:"current_bytes"`
	LocalCopies    uint64 `json:"local_copies"`
	RemoteUpdates  uint64 `json:"remote_updates"`
	PlatformWrites uint64 `json:"platform_writes"`
	OversizedCount uint64 `json:"oversized_count"`
}

// ClipboardBridge coordinates between the Android Kotlin host and Go clipboard.Engine.
type ClipboardBridge struct {
	mu          sync.Mutex
	engine      *clipboard.Engine
	host        ClipboardHost
	initialized atomic.Bool

	localCopies    atomic.Uint64
	remoteUpdates  atomic.Uint64
	platformWrites atomic.Uint64
	oversizedCount atomic.Uint64
}

var globalClipboard atomic.Pointer[ClipboardBridge]

func currentClipboardBridge() *ClipboardBridge {
	if b := globalClipboard.Load(); b != nil {
		return b
	}
	b := &ClipboardBridge{}
	globalClipboard.Store(b)
	return b
}

// Init initializes the Go clipboard engine with RoleMobile. Idempotent.
func (b *ClipboardBridge) Init(host ClipboardHost) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.initialized.Load() {
		b.host = host
		return nil
	}

	b.host = host

	adapter := clipboard.PlatformAdapterFunc(func(ctx context.Context, item *clipboard.Item) error {
		if item == nil {
			return clipboard.ErrMalformedUpdate
		}
		if len(item.Payload) > clipboard.MaxPayloadSize {
			b.oversizedCount.Add(1)
			if b.host != nil {
				b.host.OnOversizedPayload(len(item.Payload))
			}
			return &clipboard.OversizedPayloadError{Size: len(item.Payload), MaxSize: clipboard.MaxPayloadSize}
		}

		b.platformWrites.Add(1)
		h := b.host
		if h == nil {
			return errors.New("clipboard: host callback not registered")
		}

		ok := h.WritePlatformClipboard(item.MimeType, item.Payload)
		if !ok {
			return errors.New("clipboard: platform write failed in Kotlin")
		}
		return nil
	})

	transport := clipboard.TransportFunc(func(ctx context.Context, update *phonebridgev1.ClipboardUpdate) error {
		if update == nil {
			return clipboard.ErrMalformedUpdate
		}
		wireBytes, err := proto.Marshal(update)
		if err != nil {
			return fmt.Errorf("clipboard: failed to marshal update: %w", err)
		}

		b.localCopies.Add(1)

		// Try sending directly over WebRTC MediaTransport DataChannel
		if tr := currentTransport(); tr != nil {
			if err := tr.SendClipboard(wireBytes); err == nil {
				return nil
			}
		}

		// Fallback to Kotlin host callback if registered
		h := b.host
		if h == nil {
			return nil // host not registered or offline; update safe to drop
		}

		ok := h.SendClipboardUpdate(wireBytes)
		if !ok {
			return errors.New("clipboard: send update failed in Kotlin")
		}
		return nil
	})

	cfg := clipboard.EngineConfig{
		Role:      clipboard.RoleMobile,
		Platform:  adapter,
		Transport: transport,
		OnOversizedPayload: func(size int) {
			b.oversizedCount.Add(1)
			if b.host != nil {
				b.host.OnOversizedPayload(size)
			}
		},
	}

	eng, err := clipboard.NewEngine(cfg)
	if err != nil {
		return fmt.Errorf("clipboard: failed to create engine: %w", err)
	}

	b.engine = eng
	b.initialized.Store(true)
	return nil
}

// OnLocalCopy processes a local clipboard change detected by Android (Tier 1 IME or Tier 2 pull).
func (b *ClipboardBridge) OnLocalCopy(mimeType string, payload []byte, copiedAtMs int64) error {
	if !b.initialized.Load() || b.engine == nil {
		return errors.New("clipboard: engine not initialized")
	}

	if len(payload) > clipboard.MaxPayloadSize {
		b.oversizedCount.Add(1)
		if b.host != nil {
			b.host.OnOversizedPayload(len(payload))
		}
		return &clipboard.OversizedPayloadError{Size: len(payload), MaxSize: clipboard.MaxPayloadSize}
	}

	ctx := context.Background()
	_, err := b.engine.OnLocalCopy(ctx, mimeType, payload, uint64(copiedAtMs))
	return err
}

// OnRemoteBytes processes an inbound clipboard payload received over WebRTC.
func (b *ClipboardBridge) OnRemoteBytes(data []byte) error {
	if !b.initialized.Load() || b.engine == nil {
		return errors.New("clipboard: engine not initialized")
	}

	if len(data) > clipboard.MaxPayloadSize+2048 {
		b.oversizedCount.Add(1)
		if b.host != nil {
			b.host.OnOversizedPayload(len(data))
		}
		return &clipboard.OversizedPayloadError{Size: len(data), MaxSize: clipboard.MaxPayloadSize}
	}

	b.remoteUpdates.Add(1)
	ctx := context.Background()
	return b.engine.OnRemoteBytes(ctx, data)
}

// OnDataChannelOpen is called when the WebRTC "clipboard" DataChannel transitions to open.
func (b *ClipboardBridge) OnDataChannelOpen() {
	if !b.initialized.Load() || b.engine == nil {
		return
	}
	_ = b.engine.OnDataChannelOpen(context.Background())
}

// Stop tears down the clipboard bridge and releases the underlying engine.
func (b *ClipboardBridge) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.initialized.Store(false)
	b.engine = nil
	b.host = nil
}

// Stats returns a JSON diagnostic snapshot.
func (b *ClipboardBridge) Stats() *ClipboardStats {
	stats := &ClipboardStats{
		Initialized:    b.initialized.Load(),
		Role:           clipboard.RoleMobile.String(),
		LocalCopies:    b.localCopies.Load(),
		RemoteUpdates:  b.remoteUpdates.Load(),
		PlatformWrites: b.platformWrites.Load(),
		OversizedCount: b.oversizedCount.Load(),
	}

	if b.engine != nil {
		if item := b.engine.CurrentItem(); item != nil {
			stats.CurrentDigest = hex.EncodeToString(item.Digest[:])
			stats.CurrentMime = item.MimeType
			stats.CurrentBytes = len(item.Payload)
		}
	}

	return stats
}

// StatsJSON returns the stats snapshot marshaled to JSON.
func (b *ClipboardBridge) StatsJSON() []byte {
	bytes, _ := json.Marshal(b.Stats())
	return bytes
}
