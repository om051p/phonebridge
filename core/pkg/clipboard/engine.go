package clipboard

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"google.golang.org/protobuf/proto"
)

// EngineConfig configures the clipboard synchronization engine.
type EngineConfig struct {
	// Role identifies whether this node is Desktop (Linux) or Mobile (Android).
	// Used for deterministic reconnect conflict arbitration tie-breaking.
	Role Role

	// Platform is the platform adapter for writing to the host clipboard.
	// Can be nil in tests or headless mode.
	Platform PlatformAdapter

	// Transport sends clipboard updates over the communication channel.
	// Can be nil in tests or offline mode.
	Transport Transport

	// Clock provides time operations. If nil, real system time is used.
	Clock Clock

	// SuppressionCapacity overrides the default echo filter capacity (32).
	SuppressionCapacity int

	// SuppressionTTL overrides the default echo filter TTL (5,000 ms).
	SuppressionTTL time.Duration

	// OnOversizedPayload is an optional hook called when a clipboard payload
	// exceeds the 768 KiB ceiling, allowing a UI/local-IPC layer to offer DEC-012 file transfer.
	OnOversizedPayload func(size int)
}

// Engine coordinates clipboard synchronization between the local host platform
// and the remote peer over WebRTC DataChannel (DEC-023).
//
// Concurrency model:
//   - Safe for concurrent local, remote, reconnect, and lookup calls.
//   - Locks are strictly NEVER held while invoking Platform or Transport callbacks,
//     preventing lock cycles with platform event loops.
//   - Single current item retained; unbounded history is never stored.
type Engine struct {
	mu          sync.RWMutex
	role        Role
	platform    PlatformAdapter
	transport   Transport
	clock       Clock
	echoFilter  *EchoFilter
	currentItem *Item
	onOversized func(size int)
	syncPending bool
}

// NewEngine creates a new clipboard synchronization engine.
func NewEngine(cfg EngineConfig) (*Engine, error) {
	clock := cfg.Clock
	if clock == nil {
		clock = realClock{}
	}

	cap := cfg.SuppressionCapacity
	if cap <= 0 {
		cap = DefaultSuppressionCapacity
	}

	ttl := cfg.SuppressionTTL
	if ttl <= 0 {
		ttl = DefaultSuppressionTTL
	}

	return &Engine{
		role:        cfg.Role,
		platform:    cfg.Platform,
		transport:   cfg.Transport,
		clock:       clock,
		echoFilter:  NewEchoFilter(cap, ttl, clock),
		onOversized: cfg.OnOversizedPayload,
	}, nil
}

// Role returns the configured device role.
func (e *Engine) Role() Role {
	return e.role
}

// EchoFilter returns the underlying echo filter.
func (e *Engine) EchoFilter() *EchoFilter {
	return e.echoFilter
}

// CurrentItem returns the currently active local clipboard state, or nil if none.
func (e *Engine) CurrentItem() *Item {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.currentItem
}

// SetTransport sets or updates the transport interface used to send updates to peer.
func (e *Engine) SetTransport(t Transport) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.transport = t
}

// SetPlatform sets or updates the platform adapter used to write to host clipboard.
func (e *Engine) SetPlatform(p PlatformAdapter) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.platform = p
}

// OnDataChannelOpen is called when the underlying WebRTC "clipboard" DataChannel
// transitions to open (on initial connection or reconnect).
//
// Per DEC-023:
//   - Marks reconnect sync as pending.
//   - If an active local clipboard item exists, transmits it to the peer via ClipboardUpdate.
func (e *Engine) OnDataChannelOpen(ctx context.Context) error {
	e.mu.Lock()
	e.syncPending = true
	item := e.currentItem
	transport := e.transport
	var toSend *phonebridgev1.ClipboardUpdate
	if item != nil {
		toSend = item.ToProto()
	}
	e.mu.Unlock()

	if toSend != nil && transport != nil {
		return transport.SendClipboardUpdate(ctx, toSend)
	}
	return nil
}

// OnLocalCopy is a convenience method for platform adapters. It validates the MIME type,
// computes the SHA-256 digest, enforces the 768 KiB payload ceiling, and handles
// the event through OnLocalClipboard.
func (e *Engine) OnLocalCopy(ctx context.Context, mimeType string, payload []byte, copiedAtMs uint64) (*Item, error) {
	if len(payload) > MaxPayloadSize {
		if e.onOversized != nil {
			e.onOversized(len(payload))
		}
		return nil, &OversizedPayloadError{Size: len(payload), MaxSize: MaxPayloadSize}
	}

	item, err := NewItem(mimeType, payload, copiedAtMs)
	if err != nil {
		return nil, err
	}

	if err := e.OnLocalClipboard(ctx, item); err != nil {
		return nil, err
	}

	return item, nil
}

// OnLocalClipboard handles a clipboard change detected by the local platform adapter.
//
// Processing flow:
//  1. Rejects nil items or oversized payloads.
//  2. Checks the echo filter; if this digest was recently received/sent, suppresses it.
//  3. If identical to current local item, treats as a no-op.
//  4. Updates local state and records digest in the echo filter.
//  5. Releases lock, then invokes Transport.SendClipboardUpdate.
func (e *Engine) OnLocalClipboard(ctx context.Context, item *Item) error {
	if item == nil {
		return ErrMalformedUpdate
	}

	if len(item.Payload) > MaxPayloadSize {
		if e.onOversized != nil {
			e.onOversized(len(item.Payload))
		}
		return &OversizedPayloadError{Size: len(item.Payload), MaxSize: MaxPayloadSize}
	}

	e.mu.Lock()

	// A new local copy supersedes any pending reconnect sync
	e.syncPending = false

	// Check echo suppression
	if e.echoFilter.IsEcho(item.Digest) {
		e.mu.Unlock()
		return nil
	}

	// Check identical to current state
	if e.currentItem != nil && e.currentItem.Digest == item.Digest {
		e.mu.Unlock()
		return nil
	}

	e.currentItem = item
	e.echoFilter.Record(item.Digest)
	transport := e.transport
	protoMsg := item.ToProto()

	e.mu.Unlock()

	// Transport call outside the lock
	if transport != nil {
		return transport.SendClipboardUpdate(ctx, protoMsg)
	}

	return nil
}

// OnRemoteClipboard handles an inbound ClipboardUpdate received from the remote peer.
//
// Processing flow:
//  1. Validates and converts protobuf to Item (rejecting oversized, bad MIME, bad SHA-256).
//  2. If a reconnect sync is pending, delegates directly to OnReconnectSync for DEC-023 arbitration.
//  3. Checks echo filter; drops echoes at the boundary.
//  4. If identical to current local item, treats as no-op.
//  5. Updates local state and records digest in echo filter (so subsequent local platform
//     change events are recognized as echoes and suppressed).
//  6. Releases lock, then invokes PlatformAdapter.WriteClipboard.
func (e *Engine) OnRemoteClipboard(ctx context.Context, update *phonebridgev1.ClipboardUpdate) error {
	e.mu.Lock()
	pending := e.syncPending
	e.mu.Unlock()

	if pending {
		_, err := e.OnReconnectSync(ctx, update)
		return err
	}

	item, err := ItemFromProto(update)
	if err != nil {
		var oversized *OversizedPayloadError
		if errors.As(err, &oversized) && e.onOversized != nil {
			e.onOversized(oversized.Size)
		}
		return err
	}

	e.mu.Lock()

	// Check echo suppression
	if e.echoFilter.IsEcho(item.Digest) {
		e.mu.Unlock()
		return nil
	}

	// Check identical to current state
	if e.currentItem != nil && e.currentItem.Digest == item.Digest {
		e.mu.Unlock()
		return nil
	}

	e.currentItem = item
	e.echoFilter.Record(item.Digest)
	platform := e.platform

	e.mu.Unlock()

	// Platform write outside the lock
	if platform != nil {
		return platform.WriteClipboard(ctx, item)
	}

	return nil
}

// OnRemoteBytes decodes raw wire bytes (either ClipboardUpdate or Envelope framing)
// and processes the update via OnRemoteClipboard.
func (e *Engine) OnRemoteBytes(ctx context.Context, data []byte) error {
	if len(data) > MaxPayloadSize+2048 {
		return &OversizedPayloadError{Size: len(data), MaxSize: MaxPayloadSize}
	}

	var update phonebridgev1.ClipboardUpdate
	if err := proto.Unmarshal(data, &update); err == nil && len(update.Payload) > 0 {
		return e.OnRemoteClipboard(ctx, &update)
	}

	// Try Envelope framing
	var env phonebridgev1.Envelope
	if err := proto.Unmarshal(data, &env); err == nil && env.GetClipboardUpdate() != nil {
		return e.OnRemoteClipboard(ctx, env.GetClipboardUpdate())
	}

	return ErrMalformedUpdate
}

// OnReconnectSync performs initial clipboard synchronization when the DataChannel opens
// or reconnects (DEC-023).
//
// It evaluates local and remote states using Arbitrate:
//   - WinnerNone: no-op, returns WinnerNone.
//   - WinnerLocal: transmits the local state to remote, returns WinnerLocal.
//   - WinnerRemote: applies remote state locally, returns WinnerRemote.
//
// All callbacks to Transport and PlatformAdapter are executed outside the lock.
func (e *Engine) OnReconnectSync(ctx context.Context, remoteUpdate *phonebridgev1.ClipboardUpdate) (Winner, error) {
	var remoteItem *Item
	if remoteUpdate != nil {
		var err error
		remoteItem, err = ItemFromProto(remoteUpdate)
		if err != nil {
			return WinnerNone, err
		}
	}

	e.mu.Lock()
	e.syncPending = false

	winner := Arbitrate(e.currentItem, remoteItem, e.role)

	var transport Transport
	var platform PlatformAdapter
	var toSend *phonebridgev1.ClipboardUpdate
	var toWrite *Item

	switch winner {
	case WinnerNone:
		// No-op
	case WinnerLocal:
		if e.currentItem != nil {
			toSend = e.currentItem.ToProto()
			transport = e.transport
		}
	case WinnerRemote:
		if remoteItem != nil {
			e.currentItem = remoteItem
			e.echoFilter.Record(remoteItem.Digest)
			toWrite = remoteItem
			platform = e.platform
		}
	}

	e.mu.Unlock()

	// Invoke callbacks outside the lock
	if toSend != nil && transport != nil {
		if err := transport.SendClipboardUpdate(ctx, toSend); err != nil {
			return winner, err
		}
	}

	if toWrite != nil && platform != nil {
		if err := platform.WriteClipboard(ctx, toWrite); err != nil {
			return winner, err
		}
	}

	return winner, nil
}
