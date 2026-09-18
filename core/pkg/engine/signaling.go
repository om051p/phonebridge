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

type sdpPayload struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

type stopPayload struct {
	Reason string `json:"reason"`
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

// RequestOffer requests an SDP offer from the target device.
func (c *SignalingClient) RequestOffer(ctx context.Context, endpoint string) (pion.SessionDescription, error) {
	url := fmt.Sprintf("http://%s/session/offer", endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return pion.SessionDescription{}, fmt.Errorf("create offer request: %w", err)
	}
	c.applyAuth(req, nil)

	resp, err := c.client.Do(req)
	if err != nil {
		return pion.SessionDescription{}, fmt.Errorf("do offer request to %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return pion.SessionDescription{}, fmt.Errorf("offer request failed (%d): %s", resp.StatusCode, string(b))
	}

	var payload sdpPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return pion.SessionDescription{}, fmt.Errorf("decode offer json: %w", err)
	}

	return pion.SessionDescription{
		Type: pion.SDPTypeOffer,
		SDP:  payload.SDP,
	}, nil
}

// SendAnswer sends the local SDP answer to the target device.
func (c *SignalingClient) SendAnswer(ctx context.Context, endpoint string, answer pion.SessionDescription) error {
	url := fmt.Sprintf("http://%s/session/answer", endpoint)
	data, err := json.Marshal(sdpPayload{
		Type: answer.Type.String(),
		SDP:  answer.SDP,
	})
	if err != nil {
		return fmt.Errorf("marshal answer json: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create answer request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.applyAuth(req, data)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("do answer request to %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("answer request failed (%d): %s", resp.StatusCode, string(b))
	}
	return nil
}

// StopSession notifies the target device that the session has ended.
func (c *SignalingClient) StopSession(ctx context.Context, endpoint string, reason string) error {
	url := fmt.Sprintf("http://%s/session/stop", endpoint)
	data, _ := json.Marshal(stopPayload{Reason: reason})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.applyAuth(req, data)

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
