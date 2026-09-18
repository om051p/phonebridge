package receiver

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/interceptor"
	pion "github.com/pion/webrtc/v4"

	"github.com/om051p/phonebridge/core/pkg/rtpmedia"
)

const (
	defaultFmtpLine = "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"
	defaultQueueCap = 128
)

// Config configures a production Linux Receiver.
type Config struct {
	PortMin         uint16
	PortMax         uint16
	IncludeLoopback bool
	QueueDepth      int
	Sink            FrameSink
	// KeepSinkOpen makes Close leave the sink open, so the caller owns its
	// lifetime. The session sets this: its sink must outlive an individual
	// transport, because a reconnect repairs the transport and must not
	// restart the display path (DEC-022).
	KeepSinkOpen   bool
	OnStateChange  func(pion.PeerConnectionState)
	OnConnected    func()
	OnDisconnected func()
	// OnSessionError receives a typed failure reported by the sending device
	// over the control channel (for example CONSENT_REVOKED or
	// CAPTURE_FAILED). The message is the device's human-readable detail.
	OnSessionError func(code string, message string)
}

// Receiver coordinates the WebRTC peer connection, RFC 6184 RTP depacketization,
// and delivery to a FrameSink (display, file, or verification decoder).
type Receiver struct {
	cfg          Config
	pc           *pion.PeerConnection
	depacketizer *rtpmedia.Depacketizer
	sink         FrameSink
	auChan       chan rtpmedia.AccessUnit
	workerWg     sync.WaitGroup

	closed     atomic.Bool
	droppedAUs atomic.Int64
	rttUs      atomic.Int64

	mu        sync.Mutex
	readDone  chan struct{}
	trackSeen chan struct{}
}

// NewReceiver builds and initializes a Receiver.
func NewReceiver(cfg Config) (*Receiver, error) {
	if cfg.QueueDepth <= 0 {
		cfg.QueueDepth = defaultQueueCap
	}
	if cfg.Sink == nil {
		cfg.Sink = NewNullSink()
	}

	se := &pion.SettingEngine{}
	if cfg.PortMin > 0 && cfg.PortMax >= cfg.PortMin {
		_ = se.SetEphemeralUDPPortRange(cfg.PortMin, cfg.PortMax)
	}
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	if cfg.IncludeLoopback {
		se.SetIncludeLoopbackCandidate(true)
	}

	api := pion.NewAPI(
		pion.WithSettingEngine(*se),
		pion.WithInterceptorRegistry(&interceptor.Registry{}),
	)

	pc, err := api.NewPeerConnection(pion.Configuration{ICEServers: []pion.ICEServer{}})
	if err != nil {
		return nil, fmt.Errorf("receiver: peer connection: %w", err)
	}

	r := &Receiver{
		cfg:          cfg,
		pc:           pc,
		depacketizer: rtpmedia.NewDepacketizer(),
		sink:         cfg.Sink,
		auChan:       make(chan rtpmedia.AccessUnit, cfg.QueueDepth),
		readDone:     make(chan struct{}),
		trackSeen:    make(chan struct{}),
	}

	// DataChannel handler for control and RTT ping-pong
	pc.OnDataChannel(func(dc *pion.DataChannel) {
		dc.OnMessage(func(msg pion.DataChannelMessage) {
			var m map[string]any
			if json.Unmarshal(msg.Data, &m) == nil {
				switch m["type"] {
				case "ping":
					// Echo pong with the sender's transmit timestamp
					resp, _ := json.Marshal(map[string]any{
						"type":        "pong",
						"tx_epoch_ms": m["tx_epoch_ms"],
						"rx_epoch_ms": time.Now().UnixMilli(),
					})
					_ = dc.SendText(string(resp))
				case "session_error":
					// Typed sender-side failure (DEC-022). The device tells us what
					// actually went wrong so the session can classify it rather than
					// infer a cause from a stalled stream.
					code, _ := m["code"].(string)
					msg, _ := m["message"].(string)
					if cfg.OnSessionError != nil {
						cfg.OnSessionError(code, msg)
					}
				}
			}
		})
	})

	// Connection state observer
	pc.OnConnectionStateChange(func(st pion.PeerConnectionState) {
		if cfg.OnStateChange != nil {
			cfg.OnStateChange(st)
		}
		if st == pion.PeerConnectionStateConnected && cfg.OnConnected != nil {
			cfg.OnConnected()
		}
		if (st == pion.PeerConnectionStateFailed || st == pion.PeerConnectionStateClosed) && cfg.OnDisconnected != nil {
			cfg.OnDisconnected()
		}
	})

	// Register remote video track handler
	pc.OnTrack(func(track *pion.TrackRemote, _ *pion.RTPReceiver) {
		r.mu.Lock()
		select {
		case <-r.trackSeen:
		default:
			close(r.trackSeen)
		}
		r.mu.Unlock()

		r.readLoop(track)
	})

	// Start asynchronous AU delivery worker
	r.workerWg.Add(1)
	go r.sinkWorker()

	return r, nil
}

// SetRemoteOffer applies a remote SDP offer, creates a local SDP answer, and waits
// for local ICE gathering to complete before returning the answer.
func (r *Receiver) SetRemoteOffer(offer pion.SessionDescription) (pion.SessionDescription, error) {
	if err := r.pc.SetRemoteDescription(offer); err != nil {
		return pion.SessionDescription{}, fmt.Errorf("receiver: set remote offer: %w", err)
	}

	answer, err := r.pc.CreateAnswer(nil)
	if err != nil {
		return pion.SessionDescription{}, fmt.Errorf("receiver: create answer: %w", err)
	}

	if err := r.pc.SetLocalDescription(answer); err != nil {
		return pion.SessionDescription{}, fmt.Errorf("receiver: set local answer: %w", err)
	}

	r.gatherComplete(2 * time.Second)
	return *r.pc.LocalDescription(), nil
}

// ConnectionState returns the underlying PeerConnection state.
func (r *Receiver) ConnectionState() pion.PeerConnectionState {
	return r.pc.ConnectionState()
}

// Stats returns a snapshot of stream statistics and queue metrics.
func (r *Receiver) Stats() (rtpmedia.StreamStats, int64) {
	return r.depacketizer.Stats(), r.droppedAUs.Load()
}

// WaitForTrack blocks until OnTrack is invoked or the timeout elapses.
func (r *Receiver) WaitForTrack(timeout time.Duration) error {
	select {
	case <-r.trackSeen:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("receiver: timeout waiting for remote track")
	}
}

// WaitForState polls until the connection reaches want or times out.
func (r *Receiver) WaitForState(want pion.PeerConnectionState, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if r.pc.ConnectionState() == want {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("receiver: timeout waiting for %s (current %s)", want, r.pc.ConnectionState())
}

// Close terminates the receiver, closes network sockets, flushes the depacketizer,
// drains worker queues, and closes the FrameSink.
func (r *Receiver) Close() error {
	if r.closed.Swap(true) {
		return nil
	}

	var firstErr error
	if err := r.pc.Close(); err != nil && firstErr == nil {
		firstErr = err
	}

	// Flush any pending trailing AU from the depacketizer
	if trailingAU := r.depacketizer.Flush(); trailingAU != nil {
		r.enqueueAU(*trailingAU)
	}

	close(r.auChan)
	r.workerWg.Wait()

	if !r.cfg.KeepSinkOpen {
		if err := r.sink.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func (r *Receiver) readLoop(track *pion.TrackRemote) {
	defer close(r.readDone)

	for {
		if r.closed.Load() {
			return
		}

		pkt, _, err := track.ReadRTP()
		if err != nil {
			return
		}

		au, err := r.depacketizer.Push(pkt)
		if err != nil {
			continue
		}
		if au != nil {
			r.enqueueAU(*au)
		}
	}
}

func (r *Receiver) enqueueAU(au rtpmedia.AccessUnit) {
	select {
	case r.auChan <- au:
		return
	default:
		// Queue full: drop oldest frame to preserve WebRTC reader responsiveness
		select {
		case <-r.auChan:
			r.droppedAUs.Add(1)
		default:
		}
		select {
		case r.auChan <- au:
		default:
			r.droppedAUs.Add(1)
		}
	}
}

func (r *Receiver) sinkWorker() {
	defer r.workerWg.Done()

	for au := range r.auChan {
		_ = r.sink.WriteAU(au)
	}
}

func (r *Receiver) gatherComplete(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if r.pc.ICEGatheringState() == pion.ICEGatheringStateComplete {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}
