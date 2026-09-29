package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/interceptor"
	pion "github.com/pion/webrtc/v4"
	"google.golang.org/protobuf/proto"

	"github.com/om051p/phonebridge/core/pkg/clipboard"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"github.com/om051p/phonebridge/core/pkg/transfer"
	"github.com/om051p/phonebridge/core/pkg/transfer/rtcchannel"
)

const (
	// DefaultInboundAnswerTimeout bounds how long an unanswered inbound offer may
	// stay allocated.
	//
	// A peer that dials in and then disappears (crash, sleep, network drop, or a
	// process killed before it could POST /session/stop) used to leave this
	// session allocated forever, and because a live inbound session answers the
	// next offer with SESSION_BUSY, that one dead peer permanently wedged inbound
	// sessions until the daemon restarted. The budget reuses the session layer's
	// own connect timeout so both directions of a session fail at the same speed.
	DefaultInboundAnswerTimeout = 20 * time.Second

	// DefaultInboundDisconnectGrace is how long a transport may sit in
	// DISCONNECTED before the session is torn down.
	//
	// DISCONNECTED is reported for both a transport that is re-establishing ICE
	// and one that is gone. Tearing down immediately would drop recoverable
	// sessions, so the state is given a grace period; CONNECTED cancels it.
	// FAILED and CLOSED are terminal and tear down at once.
	DefaultInboundDisconnectGrace = 5 * time.Second
)

// InboundSessionConfig configures an inbound WebRTC responder session.
type InboundSessionConfig struct {
	PortMin         uint16
	PortMax         uint16
	IncludeLoopback bool
	PeerDeviceID    string
	ClipboardEngine *clipboard.Engine
	TransferEngine  *transfer.Engine
	OnStateChange   func(pion.PeerConnectionState)
	// OnDead is invoked exactly once, after the session has closed itself because
	// its transport died or the peer never answered. The owner uses it to drop
	// its reference; without it the session still closes, only the owner's pointer
	// would linger.
	OnDead func(sess *InboundSession, reason string)
	// AnswerTimeout how long the peer may take to answer the offer before the
	// session tears itself down. Zero uses DefaultInboundAnswerTimeout.
	AnswerTimeout time.Duration
	// DisconnectGrace is the DISCONNECTED grace period. Zero uses
	// DefaultInboundDisconnectGrace.
	DisconnectGrace time.Duration
}

// InboundSession manages the responder-side WebRTC PeerConnection and DataChannels (DEC-022, DEC-023).
type InboundSession struct {
	mu              sync.Mutex
	cfg             InboundSessionConfig
	pc              *pion.PeerConnection
	cbDC            *pion.DataChannel
	trDC            *pion.DataChannel
	trCh            *rtcchannel.Channel
	ctrlDC          *pion.DataChannel
	clipboardEngine *clipboard.Engine
	transferEngine  *transfer.Engine
	ctx             context.Context
	cancel          context.CancelFunc
	closed          bool

	// dead is a latch rather than a sync.Once because teardown re-enters this
	// type: closing the PeerConnection reports CLOSED back to the state handler,
	// and a sync.Once would deadlock against its own in-progress call.
	dead atomic.Bool
	// answered records whether a remote answer was ever applied, so teardown can
	// report the right reason, and so the answer deadline stops on its own.
	answered atomic.Bool

	offerDeadline   *time.Timer
	disconnectTimer *time.Timer
	timerMu         sync.Mutex
}

// NewInboundSession constructs a new InboundSession with control and clipboard DataChannels.
func NewInboundSession(cfg InboundSessionConfig) (*InboundSession, error) {
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
		return nil, fmt.Errorf("inbound: peer connection: %w", err)
	}

	ordered := true
	maxRetransmits := uint16(0)
	ctrlDC, err := pc.CreateDataChannel("control", &pion.DataChannelInit{
		Ordered:        &ordered,
		MaxRetransmits: &maxRetransmits,
	})
	if err != nil {
		_ = pc.Close()
		return nil, fmt.Errorf("inbound: control datachannel: %w", err)
	}

	cbOrdered := true
	cbDC, err := pc.CreateDataChannel("clipboard", &pion.DataChannelInit{
		Ordered: &cbOrdered,
	})
	if err != nil {
		_ = pc.Close()
		return nil, fmt.Errorf("inbound: clipboard datachannel: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Dedicated file-transfer DataChannel (DEC-024): reliable, ordered, and
	// independent of clipboard and control.
	trDC, err := pc.CreateDataChannel("transfer", &pion.DataChannelInit{
		Ordered: &cbOrdered,
	})
	if err != nil {
		cancel()
		_ = pc.Close()
		return nil, fmt.Errorf("inbound: transfer datachannel: %w", err)
	}
	trCh := rtcchannel.New(trDC, transfer.DefaultLowWatermark)

	sess := &InboundSession{
		cfg:             cfg,
		pc:              pc,
		cbDC:            cbDC,
		trDC:            trDC,
		trCh:            trCh,
		ctrlDC:          ctrlDC,
		clipboardEngine: cfg.ClipboardEngine,
		transferEngine:  cfg.TransferEngine,
		ctx:             ctx,
		cancel:          cancel,
	}

	// One internal handler owns lifecycle decisions and forwards to the caller's
	// callback, so a pure observer can never accidentally suppress teardown.
	pc.OnConnectionStateChange(func(state pion.PeerConnectionState) {
		sess.handleConnectionState(state)
		if cfg.OnStateChange != nil {
			cfg.OnStateChange(state)
		}
	})

	cbDC.OnOpen(func() {
		sess.handleClipboardOpen()
	})

	cbDC.OnMessage(func(msg pion.DataChannelMessage) {
		sess.handleClipboardMessage(msg.Data)
	})

	trCh2 := trCh
	trDC.OnOpen(func() {
		sess.handleTransferOpen(trCh2)
	})

	trDC.OnMessage(func(msg pion.DataChannelMessage) {
		sess.handleTransferMessage(msg.Data)
	})

	trDC.OnClose(func() {
		trCh2.Close()
		sess.handleTransferClose()
	})

	return sess, nil
}

// CreateOffer generates a WebRTC SDP offer and waits for ICE gathering to complete.
//
// The session mutex is deliberately NOT held across the pion calls: offer
// creation plus ICE gathering can take up to ~2 s, and holding it would block
// Close (and therefore the whole inbound teardown path) for that whole window.
func (s *InboundSession) CreateOffer() (pion.SessionDescription, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return pion.SessionDescription{}, errors.New("inbound: session is closed")
	}
	pc := s.pc
	s.mu.Unlock()

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return pion.SessionDescription{}, fmt.Errorf("inbound: create offer: %w", err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		return pion.SessionDescription{}, fmt.Errorf("inbound: set local offer: %w", err)
	}

	s.waitForICEGathering(2 * time.Second)

	local := pc.LocalDescription()
	if local == nil {
		return pion.SessionDescription{}, errors.New("inbound: no local description after gathering")
	}

	// The peer's clock on our offer starts now: from here on the session is
	// allocated and would wedge the next offer if the peer walked away.
	s.startAnswerDeadline()

	return *local, nil
}

// SetRemoteAnswer applies the remote SDP answer.
func (s *InboundSession) SetRemoteAnswer(answer pion.SessionDescription) error {
	s.mu.Lock()
	closed := s.closed
	pc := s.pc
	s.mu.Unlock()

	if closed {
		return errors.New("inbound: session is closed")
	}
	if err := pc.SetRemoteDescription(answer); err != nil {
		return err
	}
	// A peer that answered is alive, even if ICE has not finished yet: the
	// deadline exists to catch a peer that vanished, not a slow handshake.
	s.answered.Store(true)
	s.stopTimer(&s.offerDeadline)
	return nil
}

// startAnswerDeadline arms the watchdog that tears down a session the peer
// never answered.
func (s *InboundSession) startAnswerDeadline() {
	timeout := s.cfg.AnswerTimeout
	if timeout <= 0 {
		timeout = DefaultInboundAnswerTimeout
	}
	s.timerMu.Lock()
	defer s.timerMu.Unlock()
	if s.offerDeadline != nil {
		return
	}
	s.offerDeadline = time.AfterFunc(timeout, func() {
		if s.answered.Load() || s.IsClosed() {
			return
		}
		s.markDead(fmt.Sprintf("peer did not answer the inbound offer within %v", timeout))
	})
}

// handleConnectionState turns PeerConnection state transitions into lifecycle
// decisions. It runs on pion's callback goroutine, so it never takes s.mu.
func (s *InboundSession) handleConnectionState(state pion.PeerConnectionState) {
	switch state {
	case pion.PeerConnectionStateConnected:
		// Connected ends both watchdogs: the peer is demonstrably there.
		s.answered.Store(true)
		s.stopTimer(&s.offerDeadline)
		s.stopTimer(&s.disconnectTimer)
	case pion.PeerConnectionStateFailed, pion.PeerConnectionStateClosed:
		s.stopTimer(&s.offerDeadline)
		s.stopTimer(&s.disconnectTimer)
		s.markDead("inbound transport " + state.String())
	case pion.PeerConnectionStateDisconnected:
		// Transient for a re-establishing transport, terminal for a gone peer.
		// Give it a grace period instead of guessing.
		s.stopTimer(&s.disconnectTimer)
		grace := s.cfg.DisconnectGrace
		if grace <= 0 {
			grace = DefaultInboundDisconnectGrace
		}
		s.timerMu.Lock()
		s.disconnectTimer = time.AfterFunc(grace, func() {
			if s.IsClosed() {
				return
			}
			if err := s.pc.ConnectionState(); err != pion.PeerConnectionStateConnected {
				s.markDead(fmt.Sprintf("inbound transport stayed %s for %v", pion.PeerConnectionStateDisconnected, grace))
			}
		})
		s.timerMu.Unlock()
	}
}

// markDead closes the session exactly once and reports why. The latch makes it
// safe to call from inside teardown itself, which is what happens when closing
// the PeerConnection reports CLOSED back to handleConnectionState.
func (s *InboundSession) markDead(reason string) {
	if !s.dead.CompareAndSwap(false, true) {
		return
	}
	_ = s.Close()
	if cb := s.cfg.OnDead; cb != nil {
		cb(s, reason)
	}
}

func (s *InboundSession) stopTimer(t **time.Timer) {
	s.timerMu.Lock()
	defer s.timerMu.Unlock()
	if *t != nil {
		(*t).Stop()
		*t = nil
	}
}

// SendClipboard transmits wire bytes over the reliable ordered "clipboard" DataChannel.
func (s *InboundSession) SendClipboard(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || s.cbDC == nil {
		return errors.New("inbound: clipboard datachannel not available")
	}
	return s.cbDC.Send(data)
}

// SendTransfer transmits one encoded TransferFrame over the reliable ordered
// "transfer" DataChannel.
func (s *InboundSession) SendTransfer(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || s.trDC == nil {
		return errors.New("inbound: transfer datachannel not available")
	}
	return s.trDC.Send(data)
}

// TransferChannel exposes the transfer channel adapter for the local transfer
// engine (DEC-024).
func (s *InboundSession) TransferChannel() *rtcchannel.Channel { return s.trCh }

// IsClosed returns whether the session has been closed.
func (s *InboundSession) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// Close terminates the session and its underlying PeerConnection. It is
// idempotent and safe to call from pion callbacks.
func (s *InboundSession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	cancel := s.cancel
	pc := s.pc
	eng := s.clipboardEngine
	trEng := s.transferEngine
	ch := s.trCh
	s.mu.Unlock()

	// Stop the watchdogs first: they would otherwise fire against a session that
	// is already going away and report a second, misleading reason.
	s.stopTimer(&s.offerDeadline)
	s.stopTimer(&s.disconnectTimer)

	if cancel != nil {
		cancel()
	}
	if eng != nil {
		eng.SetTransport(nil)
	}
	if ch != nil {
		ch.Close()
	}
	if trEng != nil {
		// No resume (DEC-024): an interrupted transfer fails and is reported to
		// the user rather than silently waiting. DetachChannelIf keeps a session
		// being torn down from interrupting its successor's transfers.
		trEng.DetachChannelIf(ch, transfer.ReasonInterrupted, "session closed")
	}
	if pc != nil {
		return pc.Close()
	}
	return nil
}

func (s *InboundSession) handleClipboardOpen() {
	s.mu.Lock()
	eng := s.clipboardEngine
	ctx := s.ctx
	s.mu.Unlock()

	if eng == nil {
		return
	}

	eng.SetTransport(clipboard.TransportFunc(func(c context.Context, update *phonebridgev1.ClipboardUpdate) error {
		wireBytes, err := proto.Marshal(update)
		if err != nil {
			return err
		}
		return s.SendClipboard(wireBytes)
	}))

	_ = eng.OnDataChannelOpen(ctx)
}

func (s *InboundSession) handleClipboardMessage(data []byte) {
	s.mu.Lock()
	eng := s.clipboardEngine
	ctx := s.ctx
	s.mu.Unlock()

	if eng == nil {
		return
	}

	_ = eng.OnRemoteBytes(ctx, data)
}

// handleTransferOpen binds the freshly opened transfer DataChannel to the local
// transfer engine for the lifetime of this session.
func (s *InboundSession) handleTransferOpen(ch *rtcchannel.Channel) {
	s.mu.Lock()
	eng := s.transferEngine
	peer := s.cfg.PeerDeviceID
	s.mu.Unlock()

	if eng == nil || ch == nil {
		return
	}

	eng.SetPeerDeviceID(peer)
	eng.AttachChannel(ch)
}

func (s *InboundSession) handleTransferMessage(data []byte) {
	s.mu.Lock()
	eng := s.transferEngine
	s.mu.Unlock()

	if eng == nil {
		return
	}
	eng.OnFrame(data)
}

func (s *InboundSession) handleTransferClose() {
	s.mu.Lock()
	eng := s.transferEngine
	ch := s.trCh
	s.mu.Unlock()

	if eng == nil {
		return
	}
	eng.DetachChannelIf(ch, transfer.ReasonInterrupted, "transfer channel closed")
}

// waitForICEGathering blocks until ICE gathering completes or the timeout
// expires.
//
// It waits on pion's gathering-state callback instead of polling: the state is
// an event, and a 5 ms spin for up to 2 s per offer is pure waste that also
// makes the caller's latency unpredictable.
func (s *InboundSession) waitForICEGathering(timeout time.Duration) {
	done := make(chan struct{})
	var once sync.Once
	s.pc.OnICEGatheringStateChange(func(state pion.ICEGatheringState) {
		if state == pion.ICEGatheringStateComplete {
			once.Do(func() { close(done) })
		}
	})
	// The state may already be complete (no candidates to gather), in which case
	// the callback never fires.
	if s.pc.ICEGatheringState() == pion.ICEGatheringStateComplete {
		once.Do(func() { close(done) })
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
}
