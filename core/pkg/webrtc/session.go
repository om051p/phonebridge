package webrtc // import "github.com/om051p/phonebridge/core/pkg/webrtc"

import (
	"errors"
	"fmt"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	pion "github.com/pion/webrtc/v4"

	"github.com/om051p/phonebridge/core/pkg/transfer"
	"github.com/om051p/phonebridge/core/pkg/transfer/rtcchannel"
)

// Pion PeerConnection session for the Android→Linux H.264 transport
// (DEC-021; Spike 04 validated architecture, production re-derivation).
//
// Integration strategy (R1, settled against pion/webrtc v4.2.20 source):
// the public TrackLocalStaticRTP is used unmodified. Its WriteRTP overwrites
// only SSRC and PayloadType from the negotiated binding and forwards
// SequenceNumber, Timestamp, Marker and payload verbatim, performing no
// pacing, reordering or repacketization. The Sender therefore emits exactly
// the DEC-021 packet stream (per-packet shaped, FU-A ≤1200 B, marker on the
// last packet of each AU, SPS/PPS-prepended IDRs) while Pion retains
// ownership of SSRC/PT and the whole SDP/ICE/DTLS/SRTP stack. A custom
// TrackLocal implementation is deliberately NOT used: it would duplicate
// binding bookkeeping for zero benefit. Sample-level WriteSample was
// rejected: Pion's H264Payloader repacketizes there (STAP-A aggregation, its
// own marker rules) and offers no per-packet hook, which would surrender
// shaping and DEC-021 packetization semantics.
//
// ICE configuration is intentionally empty (LAN-only first vertical slice
// per the approved plan; STUN/TURN is Spike 10 / DEC-008). The interceptor
// registry is bare: no NACK/TWCC buffering of pre-encoded RTP, matching the
// spike-validated send path (no retransmission obligations at this layer).

// Defaults for the track identity and H.264 profile.
const (
	DefaultTrackID   = "phonebridge-video"
	DefaultStreamID  = "phonebridge"
	defaultFmtpLine  = "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"
	defaultControlDC = "control"
	// transferDC carries phonebridge.v1.TransferFrame messages for file transfer
	// (DEC-024). It is created by the offerer, like control and clipboard, and
	// stays independent of the clipboard channel.
	transferDC = "transfer"
	// controlMaxRetrans = 0 means unordered-safe, no-retransmit control
	// messages: freshest stats wins, stale samples are worthless.
	controlMaxRetrans = 0
)

// SessionConfig configures NewSession. Zero values are production defaults.
type SessionConfig struct {
	TrackID  string // 0 → DefaultTrackID
	StreamID string // 0 → DefaultStreamID
	FmtpLine string // 0 → packetization-mode=1, baseline-constrained profile
	PortMin  uint16 // 0 → OS-assigned ephemeral port (spike used 40000–40100)
	PortMax  uint16
	// IncludeLoopback adds 127.x ICE candidates. Test-only (loopback
	// integration tests); production LAN paths use host candidates, which
	// Pion includes by default.
	IncludeLoopback bool
	// OnStateChange receives PeerConnection state transitions (from the
	// Pion callback goroutine — must not block).
	OnStateChange func(pion.PeerConnectionState)
	// OnControlMessage receives DataChannel control messages (from the
	// Pion receive goroutine — must not block).
	OnControlMessage func([]byte)
	// OnClipboardMessage receives DataChannel clipboard messages (from the
	// Pion receive goroutine — must not block).
	OnClipboardMessage func([]byte)
	// OnClipboardOpen is called when the reliable ordered "clipboard" DataChannel
	// transitions to open state.
	OnClipboardOpen func()
	// OnTransferMessage receives transfer frames (from the Pion receive
	// goroutine — must not block).
	OnTransferMessage func([]byte)
	// OnTransferOpen is called when the reliable ordered "transfer" DataChannel
	// transitions to open state.
	OnTransferOpen func()
	// OnTransferClose is called when it closes, so in-flight transfers are
	// interrupted instead of waiting out their timeouts (DEC-024 has no resume).
	OnTransferClose func()
}

// Session couples one Sender to one Pion PeerConnection + H.264 track.
// Lifecycle: NewSession → CreateOffer/SetRemoteAnswer (or SetRemoteOffer/
// CreateAnswer) → Start → … → Stop. Single-use; construct a new Session per
// stream (DEC-020: resolution changes end the session and need new consent).
type Session struct {
	pc     *pion.PeerConnection
	track  *pion.TrackLocalStaticRTP
	dc     *pion.DataChannel
	cbDC   *pion.DataChannel
	trDC   *pion.DataChannel
	trCh   *rtcchannel.Channel
	sender *Sender
}

// NewSession builds the PeerConnection, adds the H.264 track, wires the
// sender's sink to the track and registers state/control callbacks. The
// sender must not be started yet (NewSession installs its sink).
func NewSession(cfg SessionConfig, sender *Sender) (*Session, error) {
	if cfg.TrackID == "" {
		cfg.TrackID = DefaultTrackID
	}
	if cfg.StreamID == "" {
		cfg.StreamID = DefaultStreamID
	}
	if cfg.FmtpLine == "" {
		cfg.FmtpLine = defaultFmtpLine
	}

	se := &pion.SettingEngine{}
	if cfg.PortMin > 0 && cfg.PortMax >= cfg.PortMin {
		_ = se.SetEphemeralUDPPortRange(cfg.PortMin, cfg.PortMax)
	}
	// mDNS-obfuscated candidates need multicast to resolve; plain host IPs
	// are correct on the LAN path and work in containers/CI.
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	if cfg.IncludeLoopback {
		se.SetIncludeLoopbackCandidate(true)
	}
	// Bare interceptor registry: see package doc (no NACK/TWCC buffering of
	// pre-encoded RTP).
	api := pion.NewAPI(
		pion.WithSettingEngine(*se),
		pion.WithInterceptorRegistry(&interceptor.Registry{}),
	)
	pc, err := api.NewPeerConnection(pion.Configuration{ICEServers: []pion.ICEServer{}})
	if err != nil {
		return nil, fmt.Errorf("webrtc: peer connection: %w", err)
	}

	codec := pion.RTPCodecCapability{
		MimeType:    pion.MimeTypeH264,
		ClockRate:   90000,
		SDPFmtpLine: cfg.FmtpLine,
	}
	track, err := pion.NewTrackLocalStaticRTP(codec, cfg.TrackID, cfg.StreamID)
	if err != nil {
		_ = pc.Close()
		return nil, fmt.Errorf("webrtc: track: %w", err)
	}
	if _, err := pc.AddTrack(track); err != nil {
		_ = pc.Close()
		return nil, fmt.Errorf("webrtc: add track: %w", err)
	}

	ordered := true
	maxRetransmits := uint16(controlMaxRetrans)
	dc, err := pc.CreateDataChannel(defaultControlDC, &pion.DataChannelInit{
		Ordered:        &ordered,
		MaxRetransmits: &maxRetransmits,
	})
	if err != nil {
		_ = pc.Close()
		return nil, fmt.Errorf("webrtc: control datachannel: %w", err)
	}
	if cfg.OnControlMessage != nil {
		dc.OnMessage(func(msg pion.DataChannelMessage) { cfg.OnControlMessage(msg.Data) })
	}

	cbOrdered := true
	cbDC, err := pc.CreateDataChannel("clipboard", &pion.DataChannelInit{
		Ordered: &cbOrdered,
	})
	if err != nil {
		_ = pc.Close()
		return nil, fmt.Errorf("webrtc: clipboard datachannel: %w", err)
	}
	if cfg.OnClipboardMessage != nil {
		cbDC.OnMessage(func(msg pion.DataChannelMessage) { cfg.OnClipboardMessage(msg.Data) })
	}
	if cfg.OnClipboardOpen != nil {
		cbDC.OnOpen(cfg.OnClipboardOpen)
	}

	// Dedicated file-transfer DataChannel (DEC-024): reliable, ordered, and
	// independent of the clipboard channel. The engine attaches to it through
	// TransferChannel() once it opens.
	trOrdered := true
	trDC, err := pc.CreateDataChannel(transferDC, &pion.DataChannelInit{
		Ordered: &trOrdered,
	})
	if err != nil {
		_ = pc.Close()
		return nil, fmt.Errorf("webrtc: transfer datachannel: %w", err)
	}
	trCh := rtcchannel.New(trDC, transfer.DefaultLowWatermark)
	if cfg.OnTransferMessage != nil {
		trDC.OnMessage(func(msg pion.DataChannelMessage) { cfg.OnTransferMessage(msg.Data) })
	}
	if cfg.OnTransferOpen != nil {
		trDC.OnOpen(cfg.OnTransferOpen)
	}
	if cfg.OnTransferClose != nil {
		trDC.OnClose(cfg.OnTransferClose)
	}

	if cfg.OnStateChange != nil {
		pc.OnConnectionStateChange(cfg.OnStateChange)
	}

	// Emission point: per-packet WriteRTP (see package doc for why this
	// preserves DEC-021 semantics without taking SSRC/PT ownership).
	sender.SetSink(SinkFunc(func(pkt rtp.Packet) error {
		return track.WriteRTP(&pkt)
	}))

	return &Session{pc: pc, track: track, dc: dc, cbDC: cbDC, trDC: trDC, trCh: trCh, sender: sender}, nil
}

// Track exposes the underlying track (stats/diagnostics).
func (s *Session) Track() *pion.TrackLocalStaticRTP { return s.track }

// ConnectionState returns the PeerConnection state (delegates to Pion's
// internally synchronized view).
func (s *Session) ConnectionState() pion.PeerConnectionState {
	return s.pc.ConnectionState()
}

// gatherComplete waits for ICE candidate gathering to finish (bounded), so
// the returned descriptions carry full candidate lists for non-trickle
// signaling — the LAN vertical slice exchanges one SDP blob per side.
func gatherComplete(pc *pion.PeerConnection, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pc.ICEGatheringState() == pion.ICEGatheringStateComplete {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// CreateOffer creates the local offer and applies it. The returned
// description is complete (gathering waited out — no trickle dependency).
func (s *Session) CreateOffer() (pion.SessionDescription, error) {
	offer, err := s.pc.CreateOffer(nil)
	if err != nil {
		return pion.SessionDescription{}, fmt.Errorf("webrtc: create offer: %w", err)
	}
	if err := s.pc.SetLocalDescription(offer); err != nil {
		return pion.SessionDescription{}, fmt.Errorf("webrtc: set local offer: %w", err)
	}
	gatherComplete(s.pc, 2*time.Second)
	return *s.pc.LocalDescription(), nil
}

// SetRemoteOffer applies a remote offer and returns the local answer
// (receiver-side flow).
func (s *Session) SetRemoteOffer(offer pion.SessionDescription) (pion.SessionDescription, error) {
	if err := s.pc.SetRemoteDescription(offer); err != nil {
		return pion.SessionDescription{}, fmt.Errorf("webrtc: set remote offer: %w", err)
	}
	answer, err := s.pc.CreateAnswer(nil)
	if err != nil {
		return pion.SessionDescription{}, fmt.Errorf("webrtc: create answer: %w", err)
	}
	if err := s.pc.SetLocalDescription(answer); err != nil {
		return pion.SessionDescription{}, fmt.Errorf("webrtc: set local answer: %w", err)
	}
	gatherComplete(s.pc, 2*time.Second)
	return *s.pc.LocalDescription(), nil
}

// SetRemoteAnswer applies the remote answer, completing negotiation
// (sender-side flow).
func (s *Session) SetRemoteAnswer(answer pion.SessionDescription) error {
	if err := s.pc.SetRemoteDescription(answer); err != nil {
		return fmt.Errorf("webrtc: set remote answer: %w", err)
	}
	return nil
}

// Start begins streaming queued AUs through the track.
func (s *Session) Start() error { return s.sender.Start() }

// SendControl sends a message on the control DataChannel (statistics,
// pacing probes — payload semantics are protocol work, not defined here).
func (s *Session) SendControl(data []byte) error { return s.dc.Send(data) }

// SendClipboard sends a message on the reliable ordered "clipboard" DataChannel.
func (s *Session) SendClipboard(data []byte) error {
	if s.cbDC == nil {
		return errors.New("webrtc: clipboard datachannel not available")
	}
	return s.cbDC.Send(data)
}

// SendTransfer sends one encoded phonebridge.v1.TransferFrame on the reliable
// ordered "transfer" DataChannel.
func (s *Session) SendTransfer(data []byte) error {
	if s.trDC == nil {
		return errors.New("webrtc: transfer datachannel not available")
	}
	return s.trDC.Send(data)
}

// TransferChannel exposes the transfer channel adapter for the transfer engine
// (DEC-024). It is non-nil for the lifetime of the session, because this is the
// side that created the channel.
func (s *Session) TransferChannel() *rtcchannel.Channel { return s.trCh }

// WaitForState polls until the PeerConnection reaches want or the timeout
// elapses (diagnostic convenience; polling is fine at handshake scale).
func (s *Session) WaitForState(want pion.PeerConnectionState, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s.pc.ConnectionState() == want {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("webrtc: timeout waiting for %s (current %s)", want, s.pc.ConnectionState())
}

// Stop tears down the sender and closes the PeerConnection. Safe from any
// goroutine and idempotent.
func (s *Session) Stop() {
	s.sender.Stop()
	_ = s.pc.Close()
}
