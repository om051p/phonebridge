// Package rtcchannel adapts the dedicated "transfer" Pion DataChannel to the
// transport-agnostic transfer.Channel interface (DEC-024).
//
// The adapter is deliberately thin: it maps SendFrame onto DataChannel.Send,
// BufferedAmount onto Pion's buffered-bytes counter, and AwaitDrain onto Pion's
// OnBufferedAmountLow threshold callback with a poll fallback. It is the only
// place where the transfer engine's backpressure contract meets Pion, so the
// engine itself stays pure Go and unit-testable without a network.
package rtcchannel

import (
	"context"
	"errors"
	"sync"
	"time"

	pion "github.com/pion/webrtc/v4"

	"github.com/om051p/phonebridge/core/pkg/transfer"
)

// drainPollInterval bounds how long AwaitDrain waits between re-checks. Pion's
// OnBufferedAmountLow fires only on a threshold crossing, so the poll is the
// safety net that keeps a drain wait from parking forever on a missed edge.
const drainPollInterval = 10 * time.Millisecond

// Channel is the transfer.Channel implementation over one Pion DataChannel.
type Channel struct {
	dc    *pion.DataChannel
	low   uint64
	done  chan struct{}
	once  sync.Once
	drain chan struct{}
}

var _ transfer.Channel = (*Channel)(nil)

// New wires a DataChannel as a transfer channel. lowWatermark is the buffered
// amount at which AwaitDrain reports "drained"; it must match the engine's
// LowWatermark so the sender's high/low hysteresis is coherent.
func New(dc *pion.DataChannel, lowWatermark uint64) *Channel {
	c := &Channel{
		dc:    dc,
		low:   lowWatermark,
		done:  make(chan struct{}),
		drain: make(chan struct{}, 1),
	}
	if lowWatermark > 0 {
		dc.SetBufferedAmountLowThreshold(lowWatermark)
	}
	dc.OnBufferedAmountLow(c.signal)
	dc.OnClose(c.Close)
	return c
}

func (c *Channel) signal() {
	select {
	case c.drain <- struct{}{}:
	default:
	}
}

// SendFrame sends one encoded TransferFrame.
func (c *Channel) SendFrame(ctx context.Context, frame []byte) error {
	select {
	case <-c.done:
		return errors.New("rtcchannel: transfer datachannel is closed")
	default:
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.dc.ReadyState() != pion.DataChannelStateOpen {
		return errors.New("rtcchannel: transfer datachannel is not open")
	}
	return c.dc.Send(frame)
}

// BufferedAmount reports bytes queued below this layer.
func (c *Channel) BufferedAmount() uint64 { return c.dc.BufferedAmount() }

// AwaitDrain blocks until the buffered amount has fallen to the low-watermark,
// the context ends, or the channel closes.
func (c *Channel) AwaitDrain(ctx context.Context) error {
	for {
		if c.dc.BufferedAmount() <= c.low {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.done:
			return errors.New("rtcchannel: transfer datachannel closed")
		case <-c.drain:
		case <-time.After(drainPollInterval):
		}
	}
}

// Done is closed when the channel closes (peer closed it or the session died).
func (c *Channel) Done() <-chan struct{} { return c.done }

// Close marks the channel done. Safe to call repeatedly and from any goroutine.
func (c *Channel) Close() {
	c.once.Do(func() { close(c.done) })
}
