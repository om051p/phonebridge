package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	pion "github.com/pion/webrtc/v4"

	"github.com/om051p/phonebridge/core/pkg/crypto"
)

// LAN signaling contract version (DEC-022). Bumped only for an incompatible
// change to the /session/* shape; the device protocol carries its own version
// in DeviceHello and is negotiated independently.
const signalingVersion uint32 = 1

// signalingCapabilities is what this (Linux) side advertises in the handshake.
// It receives screen streams and drives the pairing/trust flow.
var signalingCapabilities = []string{"SCREEN"}

type sdpPayload struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

// mediaParamsJSON is the wire projection of MediaParams. Zero values are
// omitted so "no preference" is distinguishable from an explicit value by
// absence rather than by a magic number.
type mediaParamsJSON struct {
	Width       uint32 `json:"width,omitempty"`
	Height      uint32 `json:"height,omitempty"`
	FPS         uint32 `json:"fps,omitempty"`
	BitrateKbps uint32 `json:"bitrate_kbps,omitempty"`
	Codec       string `json:"codec,omitempty"`
}

func toMediaParamsJSON(p MediaParams) mediaParamsJSON {
	return mediaParamsJSON{
		Width:       uint32(max0(p.Width)),
		Height:      uint32(max0(p.Height)),
		FPS:         uint32(max0(p.FPS)),
		BitrateKbps: uint32(max0(p.BitrateKbps)),
		Codec:       p.Codec,
	}
}

func (j mediaParamsJSON) toMediaParams() MediaParams {
	return MediaParams{
		Width:       int(j.Width),
		Height:      int(j.Height),
		FPS:         int(j.FPS),
		BitrateKbps: int(j.BitrateKbps),
		Codec:       j.Codec,
	}
}

type mediaCapabilityJSON struct {
	Codecs         []string `json:"codecs,omitempty"`
	MaxWidth       uint32   `json:"max_width,omitempty"`
	MaxHeight      uint32   `json:"max_height,omitempty"`
	MaxFPS         uint32   `json:"max_fps,omitempty"`
	SupportsScreen bool     `json:"supports_screen,omitempty"`
}

// versionAdvert is the VersionNegotiation projection.
type versionAdvert struct {
	Min uint32 `json:"min"`
	Max uint32 `json:"max"`
}

// offerRequest is the POST /session/offer body: the DEC-022 handshake plus the
// requested media parameters. Parameters must be settled here, before the offer
// exists, because Android needs a MediaProjection consent before capture exists
// (DEC-020).
type offerRequest struct {
	ProtocolVersion uint32          `json:"protocol_version"`
	Version         versionAdvert   `json:"version"`
	Capabilities    []string        `json:"capabilities,omitempty"`
	Requested       mediaParamsJSON `json:"requested"`
}

// offerResponse is the phone's answer: the SDP offer plus the typed negotiation
// result. `accepted` is a pointer so that a legacy peer which reports no
// negotiation fields is distinguishable from one that explicitly rejected:
// absent means "not reported", false means "rejected".
type offerResponse struct {
	Type            string                `json:"type"`
	SDP             string                `json:"sdp"`
	ProtocolVersion uint32                `json:"protocol_version,omitempty"`
	Code            string                `json:"code,omitempty"`
	Message         string                `json:"message,omitempty"`
	Accepted        *bool                 `json:"accepted,omitempty"`
	RejectReason    string                `json:"reject_reason,omitempty"`
	Actual          *mediaParamsJSON      `json:"actual,omitempty"`
	Capabilities    []mediaCapabilityJSON `json:"capabilities,omitempty"`
	// SessionID identifies the session this exchange created. It is additive and
	// omitted by peers that predate it, so it changes no existing shape.
	SessionID string `json:"session_id,omitempty"`
}

// peerOfferRequest is the POST /session/peer-offer body (DEC-022).
//
// It is the mirror of offerRequest: there, the initiator asks a capture device
// for an offer; here, the peer IS the capture device and brings its offer, so
// the response carries the ANSWER instead. The two directions share the same
// vocabulary, authentication and error shape rather than inventing a second
// protocol.
//
// Nothing in this body is trusted for authorization: the peer's identity comes
// from the signed request headers.
type peerOfferRequest struct {
	ProtocolVersion uint32          `json:"protocol_version"`
	Version         versionAdvert   `json:"version"`
	Capabilities    []string        `json:"capabilities,omitempty"`
	Requested       mediaParamsJSON `json:"requested"`
	// Offer is the peer's SDP offer. It must be present: this route exists
	// precisely so the capture side can drive the offer/answer exchange.
	Offer sdpPayload `json:"offer"`
	// SignalingPort is where the peer's signaling server listens, used only to
	// reach the peer again (a reconnect re-runs the standard request/answer
	// exchange). The host half is always taken from the authenticated
	// connection, never from this body.
	SignalingPort uint32 `json:"signaling_port,omitempty"`
}

// errorResponse is used for non-200 answers, which may still carry a typed code
// (for example a 409 SESSION_BUSY).
type errorResponse struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type stopPayload struct {
	Reason     string `json:"reason"`
	ReasonCode string `json:"reason_code,omitempty"`
}

// SignalingClient handles HTTP offer/answer/stop signaling with PhoneBridge devices on LAN.
type SignalingClient struct {
	client   *http.Client
	identity *crypto.DeviceIdentity
}

// NewSignalingClient creates a client with the specified request timeout.
func NewSignalingClient(timeout time.Duration) *SignalingClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &SignalingClient{
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// SetIdentity attaches a cryptographic identity for signing signaling requests.
func (c *SignalingClient) SetIdentity(identity *crypto.DeviceIdentity) {
	c.identity = identity
}

func (c *SignalingClient) applyAuth(req *http.Request, body []byte) {
	if c.identity == nil {
		return
	}
	headers, err := crypto.SignRequest(c.identity, req.Method, req.URL.Path, body)
	if err == nil {
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}
}

// NegotiationRequest bundles what the initiator asks the capture device for.
type NegotiationRequest struct {
	// Requested is the desired media tuple; zero fields mean "no preference".
	Requested MediaParams
	// PeerDeviceID is the authenticated device ID of the remote initiator (populated on the responder side).
	PeerDeviceID string
}

// RequestOffer performs the DEC-022 handshake and requests an SDP offer from
// the target device, carrying the requested media parameters. It returns the
// offer together with the device's typed negotiation answer.
//
// A non-nil error is always a signaling-level failure (transport/auth/timeout)
// and is retryable; a negotiation rejection is reported in the response, not as
// an error, so callers cannot confuse "the device said no" with "we could not
// ask".
func (c *SignalingClient) RequestOffer(ctx context.Context, endpoint string, req NegotiationRequest) (NegotiationResponse, error) {
	url := fmt.Sprintf("http://%s/session/offer", endpoint)

	body, err := json.Marshal(offerRequest{
		ProtocolVersion: signalingVersion,
		Version:         versionAdvert{Min: signalingVersion, Max: signalingVersion},
		Capabilities:    signalingCapabilities,
		Requested:       toMediaParamsJSON(req.Requested),
	})
	if err != nil {
		return NegotiationResponse{}, &SignalError{Op: "marshal offer request", Endpoint: endpoint, Err: err}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return NegotiationResponse{}, &SignalError{Op: "create offer request", Endpoint: endpoint, Err: err}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.applyAuth(httpReq, body)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return NegotiationResponse{}, &SignalError{Op: "request offer", Endpoint: endpoint, Err: err}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return NegotiationResponse{}, &SignalError{Op: "read offer response", Endpoint: endpoint, Err: err}
	}

	if resp.StatusCode != http.StatusOK {
		// A non-200 may still be a typed outcome (e.g. 409 SESSION_BUSY).
		var errBody errorResponse
		if json.Unmarshal(raw, &errBody) == nil && errBody.Code != "" {
			if code, ok := ParseCode(errBody.Code); ok {
				return NegotiationResponse{Code: code, Message: errBody.Message}, nil
			}
		}
		return NegotiationResponse{}, &SignalError{
			Op:       "request offer",
			Endpoint: endpoint,
			Err:      fmt.Errorf("unexpected status %d: %s", resp.StatusCode, truncate(string(raw), 200)),
		}
	}

	var payload offerResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return NegotiationResponse{}, &SignalError{Op: "decode offer json", Endpoint: endpoint, Err: err}
	}

	out := NegotiationResponse{
		Offer:           payload.SDP,
		ProtocolVersion: payload.ProtocolVersion,
		Message:         payload.Message,
	}
	for _, caps := range payload.Capabilities {
		out.Capabilities = append(out.Capabilities, MediaCapability{
			Codecs:         caps.Codecs,
			MaxWidth:       int(caps.MaxWidth),
			MaxHeight:      int(caps.MaxHeight),
			MaxFPS:         int(caps.MaxFPS),
			SupportsScreen: caps.SupportsScreen,
		})
	}

	// A typed non-OK code is a rejection regardless of the accepted flag.
	if payload.Code != "" {
		code, ok := ParseCode(payload.Code)
		if !ok {
			return NegotiationResponse{}, &SignalError{
				Op:       "decode offer json",
				Endpoint: endpoint,
				Err:      fmt.Errorf("unrecognised negotiation code %q", payload.Code),
			}
		}
		if code != CodeOK {
			out.Code = code
			out.Accepted = false
			out.RejectReason = payload.Message
			return out, nil
		}
	}
	out.Code = CodeOK

	switch {
	case payload.Accepted == nil:
		// Legacy/partial peer: it did not report a negotiation outcome. We do
		// not invent one — the actual tuple stays unknown and is reported as
		// such rather than assumed to equal the request.
		out.Accepted = true
	case *payload.Accepted:
		out.Accepted = true
	default:
		out.Accepted = false
		out.RejectReason = payload.RejectReason
		if out.RejectReason == "" {
			out.RejectReason = "device rejected the requested parameters"
		}
		if out.Code == CodeOK {
			out.Code = CodeUnsupportedMediaParams
		}
		return out, nil
	}

	if payload.Actual != nil {
		out.Actual = payload.Actual.toMediaParams()
		out.ActualKnown = true
	}
	return out, nil
}

// SendAnswer sends the local SDP answer to the target device.
func (c *SignalingClient) SendAnswer(ctx context.Context, endpoint string, answer pion.SessionDescription) error {
	url := fmt.Sprintf("http://%s/session/answer", endpoint)
	data, err := json.Marshal(sdpPayload{
		Type: answer.Type.String(),
		SDP:  answer.SDP,
	})
	if err != nil {
		return &SignalError{Op: "marshal answer json", Endpoint: endpoint, Err: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return &SignalError{Op: "create answer request", Endpoint: endpoint, Err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	c.applyAuth(req, data)

	resp, err := c.client.Do(req)
	if err != nil {
		return &SignalError{Op: "send answer", Endpoint: endpoint, Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return &SignalError{
			Op:       "send answer",
			Endpoint: endpoint,
			Err:      fmt.Errorf("unexpected status %d: %s", resp.StatusCode, truncate(string(b), 200)),
		}
	}
	return nil
}

// PeerOfferResponse is the typed outcome of POST /session/peer-offer.
//
// Code is always populated: a refusal (SESSION_BUSY, PERMISSION_DENIED, ...)
// comes back as a code with Accepted false rather than as a transport error, so
// the caller can tell "the peer said no" from "we could not ask".
type PeerOfferResponse struct {
	// Answer is the peer's SDP answer. Empty unless Accepted.
	Answer    string
	SessionID string
	Code      Code
	Message   string
	Accepted  bool
}

// SendPeerOffer offers a session to a peer that will answer it, as the capture
// device.
//
// It is the mirror of RequestOffer: there, this side asks a capture device for
// an offer; here, this side brings its own offer (it owns the capture pipeline)
// and the peer answers. Both directions use the same authenticated DEC-022
// vocabulary, so a peer only ever has to implement one session handshake.
func (c *SignalingClient) SendPeerOffer(
	ctx context.Context,
	endpoint string,
	offer pion.SessionDescription,
	req NegotiationRequest,
	signalingPort uint32,
) (PeerOfferResponse, error) {
	url := fmt.Sprintf("http://%s/session/peer-offer", endpoint)

	body, err := json.Marshal(peerOfferRequest{
		ProtocolVersion: signalingVersion,
		Version:         versionAdvert{Min: signalingVersion, Max: signalingVersion},
		Capabilities:    signalingCapabilities,
		Requested:       toMediaParamsJSON(req.Requested),
		Offer: sdpPayload{
			Type: offer.Type.String(),
			SDP:  offer.SDP,
		},
		SignalingPort: signalingPort,
	})
	if err != nil {
		return PeerOfferResponse{}, &SignalError{Op: "marshal peer offer", Endpoint: endpoint, Err: err}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return PeerOfferResponse{}, &SignalError{Op: "create peer offer request", Endpoint: endpoint, Err: err}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.applyAuth(httpReq, body)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return PeerOfferResponse{}, &SignalError{Op: "send peer offer", Endpoint: endpoint, Err: err}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return PeerOfferResponse{}, &SignalError{Op: "read peer offer response", Endpoint: endpoint, Err: err}
	}

	// A non-200 without a typed code is not a refusal, it is a failure to ask
	// (unauthenticated, wrong method, broken peer). Keeping the two apart is
	// what lets the caller decide between "retry later" and "do not retry".
	var payload offerResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return PeerOfferResponse{}, &SignalError{
			Op:       "decode peer offer response",
			Endpoint: endpoint,
			Err:      fmt.Errorf("status %d: %w", resp.StatusCode, err),
		}
	}
	if resp.StatusCode != http.StatusOK && payload.Code == "" {
		return PeerOfferResponse{}, &SignalError{
			Op:       "send peer offer",
			Endpoint: endpoint,
			Err:      fmt.Errorf("unexpected status %d: %s", resp.StatusCode, truncate(string(raw), 200)),
		}
	}

	out := PeerOfferResponse{
		Answer:    payload.SDP,
		SessionID: payload.SessionID,
		Message:   payload.Message,
	}
	if payload.Code == "" {
		out.Code = CodeOK
	} else {
		code, ok := ParseCode(payload.Code)
		if !ok {
			return PeerOfferResponse{}, &SignalError{
				Op:       "decode peer offer response",
				Endpoint: endpoint,
				Err:      fmt.Errorf("unrecognised negotiation code %q", payload.Code),
			}
		}
		out.Code = code
	}

	if resp.StatusCode != http.StatusOK || out.Code != CodeOK {
		if out.Message == "" {
			out.Message = fmt.Sprintf("peer refused the session (status %d)", resp.StatusCode)
		}
		out.Accepted = false
		return out, nil
	}
	if out.Answer == "" {
		return PeerOfferResponse{}, &SignalError{
			Op:       "decode peer offer response",
			Endpoint: endpoint,
			Err:      fmt.Errorf("peer accepted the session but returned no sdp answer"),
		}
	}
	out.Accepted = true
	return out, nil
}

// StopSession notifies the target device that the session has ended, carrying
// the typed reason so the device can distinguish a user stop from a failure.
func (c *SignalingClient) StopSession(ctx context.Context, endpoint string, reason string, code Code) error {
	url := fmt.Sprintf("http://%s/session/stop", endpoint)
	data, _ := json.Marshal(stopPayload{Reason: reason, ReasonCode: string(code)})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return &SignalError{Op: "create stop request", Endpoint: endpoint, Err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	c.applyAuth(req, data)

	resp, err := c.client.Do(req)
	if err != nil {
		return &SignalError{Op: "stop session", Endpoint: endpoint, Err: err}
	}
	defer resp.Body.Close()
	return nil
}

func max0(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
