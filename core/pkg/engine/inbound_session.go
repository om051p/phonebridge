package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/interceptor"
	pion "github.com/pion/webrtc/v4"
	"google.golang.org/protobuf/proto"

	"github.com/om051p/phonebridge/core/pkg/clipboard"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

// InboundSessionConfig configures an inbound WebRTC responder session.
type InboundSessionConfig struct {
	PortMin         uint16
	PortMax         uint16
	IncludeLoopback bool
	PeerDeviceID    string
	ClipboardEngine *clipboard.Engine
	OnStateChange   func(pion.PeerConnectionState)
}

// InboundSession manages the responder-side WebRTC PeerConnection and DataChannels (DEC-022, DEC-023).
type InboundSession struct {
	mu              sync.Mutex
	cfg             InboundSessionConfig
	pc              *pion.PeerConnection
	cbDC            *pion.DataChannel
	ctrlDC          *pion.DataChannel
	clipboardEngine *clipboard.Engine
	ctx             context.Context
	cancel          context.CancelFunc
	closed          bool
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
	sess := &InboundSession{
		cfg:             cfg,
		pc:              pc,
		cbDC:            cbDC,
		ctrlDC:          ctrlDC,
		clipboardEngine: cfg.ClipboardEngine,
		ctx:             ctx,
		cancel:          cancel,
	}

	if cfg.OnStateChange != nil {
		pc.OnConnectionStateChange(cfg.OnStateChange)
	}

	cbDC.OnOpen(func() {
		sess.handleClipboardOpen()
	})

	cbDC.OnMessage(func(msg pion.DataChannelMessage) {
		sess.handleClipboardMessage(msg.Data)
	})

	return sess, nil
}

// CreateOffer generates a WebRTC SDP offer and waits for ICE gathering to complete.
func (s *InboundSession) CreateOffer() (pion.SessionDescription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return pion.SessionDescription{}, errors.New("inbound: session is closed")
	}

	offer, err := s.pc.CreateOffer(nil)
	if err != nil {
		return pion.SessionDescription{}, fmt.Errorf("inbound: create offer: %w", err)
	}
	if err := s.pc.SetLocalDescription(offer); err != nil {
		return pion.SessionDescription{}, fmt.Errorf("inbound: set local offer: %w", err)
	}

	s.gatherComplete(2 * time.Second)
	return *s.pc.LocalDescription(), nil
}

// SetRemoteAnswer applies the remote SDP answer.
func (s *InboundSession) SetRemoteAnswer(answer pion.SessionDescription) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return errors.New("inbound: session is closed")
	}
	return s.pc.SetRemoteDescription(answer)
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

// IsClosed returns whether the session has been closed.
func (s *InboundSession) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// Close terminates the session and its underlying PeerConnection.
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
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if eng != nil {
		eng.SetTransport(nil)
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

func (s *InboundSession) gatherComplete(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s.pc.ICEGatheringState() == pion.ICEGatheringStateComplete {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}
