//go:build android || jni

package main

// MediaTransport couples the production DEC-021 pipeline to the Android JNI
// surface. This file is pure Go (no cgo) so the lifecycle state machine is
// host-testable under `go test -tags jni -race`; media.go carries the JNI
// exports and delegates here.
//
// Lifecycle and error semantics (contract for GoBridge + PhoneBridgeService):
//
//	idle --Init--> initialized --CreateOffer/SetAnswer--> negotiated
//	     --Start--> streaming --Stop--> stopped --(Release)--> idle
//
//   - Every call before readiness is a safe no-op with a documented return:
//     OnFrame returns false (frame not admitted); Stop/Release are idempotent.
//     Misuse that indicates a Kotlin bug (double Init, SetAnswer without
//     Init) returns an error that the JNI boundary escalates to
//     IllegalStateException (DEC-019 rule).
//   - Backpressure: Push never blocks (DEC-020/021 hot path). The bounded
//     queue (default 256) drops non-key frames when full and evicts oldest
//     frames to admit keyframes; OnFrame reports false for every dropped
//     frame so the caller can count.
//   - Frames may be pushed before Start (they queue); the writer drains
//     them once streaming begins. Pushed frames before the PeerConnection
//     binds are silently not sent (Pion TrackLocalStaticRTP pre-bind no-op)
//     — production therefore orders Start after the answer is applied and
//     the connection is established.
//   - Stop discards queued frames by design (teardown owes nothing,
//     DEC-020/021), stops the single writer, and closes the PeerConnection.
//     The engine-level stop (nativeStop) tears the transport down first:
//     the transport is subordinate to the DEC-019 engine lifecycle.

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	pion "github.com/pion/webrtc/v4"

	"github.com/om051p/phonebridge/core/pkg/rtpmedia"
	"github.com/om051p/phonebridge/core/pkg/webrtc"
)

// transport states (exposed in stats).
const (
	trIdle = iota
	trInitialized
	trNegotiated
	trStreaming
	trStopped
)

// MediaTransport owns one Sender/Session pair (single-use per Init; Release
// returns to idle so a new session can be built — DEC-020: resolution
// changes end the session and require a new one).
//
// The PSI cache (rtpmedia.Cache) is the ONE piece of media state that
// deliberately outlives Init/Release cycles: the encoder emits SPS/PPS exactly
// once per codec lifetime (the CSD AU), while every offer — and every
// DEC-022 reconnect — rebuilds the transport with mediaRelease+mediaInit and
// discards queued frames. A per-Sender cache would therefore be empty for the
// whole capture (the Slice 3A PSI root cause: bare IDRs on the wire, nothing
// decodable). The transport owns the cache, learns parameter sets at ADMISSION
// (before any queue can discard the CSD AU), and hands the same cache to each
// new Sender, so re-injection works from the first IDR of every rebuilt
// transport. Stale parameter sets cannot survive a capture restart: the new
// codec's CSD AU replaces them (Cache.Learn swaps changed bytes).
type MediaTransport struct {
	mu       sync.Mutex
	state    atomic.Int32
	sender   *webrtc.Sender
	retired  *webrtc.Sender // last stopped sender; counters stay readable for diagnostics
	session  *webrtc.Session
	lastPC   atomic.Pointer[string] // last PeerConnection state (diagnostics)
	peerID   string                 // signaling target, for transfer attribution
	queueAU  int
	shaperKb int
	shaperBk int
	psi      *rtpmedia.Cache // capture-scoped parameter-set cache (see above)
}

// sdpBlob is the JSON-serializable SDP exchange shape (same wire form the
// spike used: {"type":"offer","sdp":"..."}).
type sdpBlob struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

// mediaTransport is the process-wide instance (JNI entry points delegate to
// it); engine stop tears it down first.
var mediaTransport atomic.Pointer[MediaTransport]

func currentTransport() *MediaTransport {
	if t := mediaTransport.Load(); t != nil {
		return t
	}
	t := newMediaTransport(0, 0, 0) // DEC-021 defaults: 256 / 4000 / 3000
	mediaTransport.Store(t)
	return t
}

func newMediaTransport(queueAU, shaperKbps, shaperBurstKbit int) *MediaTransport {
	return &MediaTransport{
		queueAU:  queueAU,
		shaperKb: shaperKbps,
		shaperBk: shaperBurstKbit,
		psi:      &rtpmedia.Cache{},
	}
}

// MediaInit builds the sender and PeerConnection (offerer role, Spike 04).
func (t *MediaTransport) MediaInit() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state.Load() != trIdle {
		return fmt.Errorf("media transport already initialized (state %d)", t.state.Load())
	}
	sender := webrtc.NewSender(nil, webrtc.SenderConfig{
		QueueDepth:   t.queueAU,
		ShaperKbps:   t.shaperKb,
		ShaperBurstK: t.shaperBk,
		PSIReinject:  true,  // DEC-021 obligation 2
		PSICache:     t.psi, // survives this Init (Slice 3A)
	})
	cfg := webrtc.SessionConfig{
		OnStateChange: func(st pion.PeerConnectionState) {
			s := st.String()
			t.lastPC.Store(&s)
		},
		OnClipboardMessage: func(data []byte) {
			if cb := currentClipboardBridge(); cb != nil {
				_ = cb.OnRemoteBytes(data)
			}
		},
		OnClipboardOpen: func() {
			if cb := currentClipboardBridge(); cb != nil {
				cb.OnDataChannelOpen()
			}
		},
		// File transfer (DEC-024) rides its own DataChannel, independent of the
		// clipboard channel and of the media plane.
		OnTransferMessage: func(data []byte) {
			if tb := currentTransferBridge(); tb != nil {
				_ = tb.OnRemoteBytes(data)
			}
		},
		OnTransferOpen: func() {
			if tb := currentTransferBridge(); tb != nil {
				if s := t.currentSession(); s != nil {
					tb.OnChannelOpen(s.TransferChannel(), t.peerDeviceID())
				}
			}
		},
		OnTransferClose: func() {
			if tb := currentTransferBridge(); tb != nil {
				if s := t.currentSession(); s != nil {
					tb.OnChannelClose(s.TransferChannel())
				}
			}
		},
		// Remote input (DEC-027, Phase 7) rides its own dedicated DataChannel.
		OnInputMessage: func(data []byte) {
			if ib := currentInputBridge(); ib != nil {
				_ = ib.OnRemoteBytes(data)
			}
		},
		// Dedicated notifications channel (DEC-028, Phase 8): reliable and ordered.
		OnNotificationOpen: func() {
			if nb := currentNotificationBridge(); nb != nil {
				nb.OnChannelOpen()
			}
		},
		OnNotificationClose: func() {
			if nb := currentNotificationBridge(); nb != nil {
				nb.OnChannelClose()
			}
		},
	}
	sess, err := webrtc.NewSession(cfg, sender)
	if err != nil {
		return fmt.Errorf("media session: %w", err)
	}
	t.sender, t.session = sender, sess
	t.state.Store(trInitialized)
	return nil
}

// currentSession snapshots the live session under the transport lock, so the
// Pion callbacks (which fire on SCTP goroutines) never race MediaStop.
func (t *MediaTransport) currentSession() *webrtc.Session {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.session
}

// SetPeerDeviceID records which device this session targets, so transfer history
// attributes files to a device (the Android side learns it from signaling).
func (t *MediaTransport) SetPeerDeviceID(deviceID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.peerID = deviceID
}

func (t *MediaTransport) peerDeviceID() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.peerID
}

// SendClipboard sends a clipboard update message over the active WebRTC clipboard DataChannel.
func (t *MediaTransport) SendClipboard(data []byte) error {
	t.mu.Lock()
	sess := t.session
	t.mu.Unlock()
	if sess == nil {
		return errors.New("media transport not initialized")
	}
	return sess.SendClipboard(data)
}

// MediaCreateOffer creates the SDP offer (blocks ≤ ~2 s for ICE gathering;
// non-trickle, spike-validated shape).
func (t *MediaTransport) MediaCreateOffer() ([]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state.Load() != trInitialized {
		return nil, fmt.Errorf("create offer in state %d", t.state.Load())
	}
	offer, err := t.session.CreateOffer()
	if err != nil {
		return nil, err
	}
	return json.Marshal(sdpBlob{Type: offer.Type.String(), SDP: offer.SDP})
}

// MediaSetAnswer applies the remote answer (sender-side negotiation
// completion). State advances to negotiated.
func (t *MediaTransport) MediaSetAnswer(sdpJSON []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state.Load() != trInitialized {
		return fmt.Errorf("set answer in state %d", t.state.Load())
	}
	var b sdpBlob
	if err := json.Unmarshal(sdpJSON, &b); err != nil {
		return fmt.Errorf("answer json: %w", err)
	}
	typ := pion.SDPTypeAnswer
	if b.Type == pion.SDPTypeRollback.String() {
		return fmt.Errorf("rollback not supported")
	}
	if err := t.session.SetRemoteAnswer(pion.SessionDescription{Type: typ, SDP: b.SDP}); err != nil {
		return err
	}
	t.state.Store(trNegotiated)
	return nil
}

// MediaStart starts the single writer once the connection is established.
// Idempotent while streaming.
func (t *MediaTransport) MediaStart() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch t.state.Load() {
	case trNegotiated:
		if err := t.session.Start(); err != nil {
			return err
		}
		t.state.Store(trStreaming)
		return nil
	case trStreaming:
		return nil
	default:
		return fmt.Errorf("start in state %d (negotiate first)", t.state.Load())
	}
}

// LearnPSI caches any H.264 parameter sets (SPS/PPS) carried by one Annex-B
// access unit, with no transport admission and no state requirement (Slice
// 3A). It is the single learning point of the phone-side pipeline: MediaOnFrame
// calls it on every AU, and the JNI boundary calls it when the engine gate
// refuses a frame before MediaOnFrame is reachable. Learning only caches bytes
// — an AU that must be rejected is still allowed to teach this capture's
// parameter sets.
func (t *MediaTransport) LearnPSI(au []byte) {
	t.psi.Learn(rtpmedia.SplitAnnexB(au))
}

// MediaOnFrame is the hot path: never blocks, returns whether the AU was
// admitted. Safe (returns false) in every non-initialized state.
func (t *MediaTransport) MediaOnFrame(au []byte, ptsUs int64, key bool) bool {
	// Admission-side PSI learning (Slice 3A) — BEFORE the state gate: the CSD
	// AU is emitted exactly once per codec lifetime, seconds before the first
	// offer, and the transport may not be initialized yet when it arrives.
	// Learning only caches bytes, so it must succeed even when the frame
	// itself is rejected; otherwise the cache stays empty for the whole
	// capture and every rebuilt transport re-injects nothing (the live E2E
	// reproduced exactly that: PARAM_SETS_MISSING with the fix behind the
	// gate). Cheap when no PSI is present (classify-only fast path).
	t.LearnPSI(au)
	if t.state.Load() < trInitialized {
		return false
	}
	t.mu.Lock()
	s := t.sender
	t.mu.Unlock()
	if s == nil {
		return false
	}
	return s.Push(au, ptsUs, key)
}

// MediaStop tears the session down (idempotent). Queued frames are
// discarded by design (DEC-020/021 teardown semantics). The sender's counters
// remain readable through MediaStatsJSON (moved to `retired`).
func (t *MediaTransport) MediaStop() {
	t.mu.Lock()
	s := t.session
	sender := t.sender
	t.session = nil
	t.sender = nil
	if sender != nil {
		t.retired = sender
	}
	t.mu.Unlock()
	if s != nil {
		s.Stop()
	}
	t.state.Store(trStopped)
}

// MediaRelease returns to idle (stopping first if needed), enabling a fresh
// Init for the next session.
func (t *MediaTransport) MediaRelease() {
	if t.state.Load() != trIdle {
		t.MediaStop()
	}
	t.state.Store(trIdle)
}

// sessionErrorCodes are the typed failures this device may report to the peer
// over the control channel (DEC-022). Unknown codes are refused rather than
// forwarded, so a typo cannot invent a failure class the receiver will not
// recognise.
var sessionErrorCodes = map[string]bool{
	"CONSENT_REVOKED":  true,
	"CAPTURE_FAILED":   true,
	"TRANSPORT_FAILED": true,
}

// sessionErrorPayload builds the control-channel message carrying a typed
// sender-side failure. Kept separate from the send path so the wire shape is
// unit-testable.
func sessionErrorPayload(code, message string) ([]byte, error) {
	if !sessionErrorCodes[code] {
		return nil, fmt.Errorf("unknown session error code %q", code)
	}
	return json.Marshal(map[string]any{
		"type":    "session_error",
		"code":    code,
		"message": message,
	})
}

// ReportSessionError tells the peer that this device hit a typed, sender-side
// failure (consent withdrawn, capture failed) instead of leaving it to guess
// from a stream that stops. It is only meaningful while streaming; before that
// there is no control channel to send on, and the failure belongs in the
// signaling answer.
func (t *MediaTransport) ReportSessionError(code, message string) error {
	payload, err := sessionErrorPayload(code, message)
	if err != nil {
		return err
	}
	t.mu.Lock()
	sess := t.session
	state := t.state.Load()
	t.mu.Unlock()
	if sess == nil || state < trNegotiated {
		return fmt.Errorf("no negotiated transport to report %s on (state %d)", code, state)
	}
	if err := sess.SendControl(payload); err != nil {
		return fmt.Errorf("report %s: %w", code, err)
	}
	return nil
}

// MediaStatsJSON serializes counters + connection state for diagnostics.
// Counters survive Stop (reported from the retired sender).
func (t *MediaTransport) MediaStatsJSON() []byte {
	t.mu.Lock()
	s := t.sender
	if s == nil {
		s = t.retired
	}
	pcState := ""
	if t.session != nil {
		pcState = t.session.ConnectionState().String()
	}
	t.mu.Unlock()
	if v := t.lastPC.Load(); v != nil && pcState == "" {
		pcState = *v
	}
	// PSI cache counters (Slice 3A): the only on-device proof that parameter
	// sets were learned and re-injected. A capture that reports
	// psiHaveSPSPP=false with psiIDRsNoCache>0 is sending undecodable IDRs —
	// exactly the PARAM_SETS_MISSING condition the receiver diagnoses.
	psi := t.psi.Stats()
	stats := map[string]any{
		"pcState":         pcState,
		"transportState":  t.state.Load(),
		"psiHaveSPSPP":    psi.HaveSPSPP,
		"psiSPSBytes":     psi.CachedSPS,
		"psiPPSBytes":     psi.CachedPPS,
		"psiSPSUpdates":   psi.SPSUpdates,
		"psiPPSUpdates":   psi.PPSUpdates,
		"psiInjectedIDRs": psi.InjectedIDRs,
		"psiInBandIDRs":   psi.InBandIDRs,
		"psiIDRsNoCache":  psi.IDRsNoCache,
	}
	if s != nil {
		stats["pushedAUs"] = s.PushedAUs.Load()
		stats["droppedAUs"] = s.DroppedAUs.Load()
		stats["sentAUs"] = s.SentAUs.Load()
		stats["sentPackets"] = s.SentPackets.Load()
		stats["sentBytes"] = s.SentBytes.Load()
		stats["reinjectedIDRs"] = s.ReinjectedIDRs.Load()
		stats["sendErrors"] = s.SendErrors.Load()
		stats["maxSendLatencyNs"] = s.MaxSendLatency.Load()
	}
	b, _ := json.Marshal(stats)
	return b
}

// SendNotification sends wire bytes over the reliable ordered "notifications" DataChannel (Phase 8, DEC-028).
func (t *MediaTransport) SendNotification(wireBytes []byte) error {
	t.mu.Lock()
	sess := t.session
	t.mu.Unlock()
	if sess == nil {
		return errors.New("media: no active session")
	}
	return sess.SendNotification(wireBytes)
}
