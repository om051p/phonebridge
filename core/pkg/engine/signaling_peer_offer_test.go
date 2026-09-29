package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pion "github.com/pion/webrtc/v4"

	"github.com/om051p/phonebridge/core/pkg/crypto"
)

// peerOfferHarness is a signaling server with a pre-trusted client, which is
// what the phone/desktop pair looks like after pairing.
type peerOfferHarness struct {
	srv        *SignalingServer
	trust      *crypto.TrustStore
	client     *SignalingClient
	clientID   *crypto.DeviceIdentity
	endpoint   string
	peerOffer  *PeerOfferRequest
	handlerErr error
	result     PeerOfferResult
}

func newPeerOfferHarness(t *testing.T, handler func(context.Context, PeerOfferRequest) (PeerOfferResult, error)) *peerOfferHarness {
	t.Helper()
	tmpDir := t.TempDir()

	serverIdent, err := crypto.LoadOrGenerateIdentity(filepath.Join(tmpDir, "server_id.json"), "Desktop", "linux")
	if err != nil {
		t.Fatalf("server identity: %v", err)
	}
	trust, err := crypto.NewTrustStore(filepath.Join(tmpDir, "trust.json"))
	if err != nil {
		t.Fatalf("trust store: %v", err)
	}
	clientIdent, err := crypto.LoadOrGenerateIdentity(filepath.Join(tmpDir, "phone_id.json"), "Phone", "android")
	if err != nil {
		t.Fatalf("client identity: %v", err)
	}
	if err := trust.AddTrusted(crypto.TrustEntry{
		DeviceID:    clientIdent.DeviceID,
		DisplayName: clientIdent.DisplayName,
		Platform:    clientIdent.Platform,
		PublicKey:   clientIdent.PublicKey,
		PairedAt:    time.Now(),
		LastSeen:    time.Now(),
	}); err != nil {
		t.Fatalf("add trusted: %v", err)
	}

	h := &peerOfferHarness{trust: trust, clientID: clientIdent}
	srv := NewSignalingServer(SignalingServerConfig{
		Port:             0,
		Identity:         serverIdent,
		TrustStore:       trust,
		PeerOfferHandler: handler,
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start signaling server: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	h.srv = srv
	h.endpoint = fmt.Sprintf("127.0.0.1:%d", srv.Port())

	client := NewSignalingClient(3 * time.Second)
	client.SetIdentity(clientIdent)
	h.client = client
	return h
}

func testOfferSDP() pion.SessionDescription {
	return pion.SessionDescription{
		Type: pion.SDPTypeOffer,
		SDP:  "v=0\r\no=- 1 2 IN IP4 127.0.0.1\r\ns=-\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n",
	}
}

// A phone-originated session must authenticate exactly like a desktop-originated
// one: there is no weaker door for the reverse direction.
func TestSignalingServer_PeerOfferRequiresAuthentication(t *testing.T) {
	h := newPeerOfferHarness(t, func(context.Context, PeerOfferRequest) (PeerOfferResult, error) {
		t.Error("handler must not run for an unauthenticated request")
		return PeerOfferResult{}, nil
	})

	anonymous := NewSignalingClient(3 * time.Second)
	_, err := anonymous.SendPeerOffer(context.Background(), h.endpoint, testOfferSDP(), NegotiationRequest{}, 0)
	if err == nil {
		t.Fatal("expected an unauthenticated peer offer to fail")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected a 401 signal error, got %v", err)
	}
}

func TestSignalingServer_PeerOfferRejectsUntrustedAndRevokedPeers(t *testing.T) {
	h := newPeerOfferHarness(t, func(context.Context, PeerOfferRequest) (PeerOfferResult, error) {
		t.Error("handler must not run for an untrusted request")
		return PeerOfferResult{}, nil
	})

	// Untrusted: a well-formed identity this desktop has never paired with.
	untrusted, err := crypto.GenerateIdentity("Stranger", "android")
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	stranger := NewSignalingClient(3 * time.Second)
	stranger.SetIdentity(untrusted)
	if _, err := stranger.SendPeerOffer(context.Background(), h.endpoint, testOfferSDP(), NegotiationRequest{}, 0); err == nil {
		t.Fatal("expected an untrusted peer offer to fail")
	}

	// Revoked mid-trust: the same device must be turned away on the next offer.
	if err := h.trust.Revoke(h.clientID.DeviceID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := h.client.SendPeerOffer(context.Background(), h.endpoint, testOfferSDP(), NegotiationRequest{}, 0); err == nil {
		t.Fatal("expected a revoked peer offer to fail")
	}
}

func TestSignalingServer_PeerOfferReturnsTheAnswer(t *testing.T) {
	const answerSDP = "v=0\r\no=- 42 2 IN IP4 127.0.0.1\r\ns=-\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n"
	var seen PeerOfferRequest
	h := newPeerOfferHarness(t, func(_ context.Context, req PeerOfferRequest) (PeerOfferResult, error) {
		seen = req
		return PeerOfferResult{Answer: answerSDP, Code: CodeOK, SessionID: "session-abc"}, nil
	})

	res, err := h.client.SendPeerOffer(context.Background(), h.endpoint, testOfferSDP(),
		NegotiationRequest{Requested: MediaParams{Width: 1080, Height: 2400, FPS: 60}}, 7804)
	if err != nil {
		t.Fatalf("SendPeerOffer: %v", err)
	}
	if !res.Accepted || res.Code != CodeOK {
		t.Fatalf("offer not accepted: %+v", res)
	}
	if res.Answer != answerSDP {
		t.Fatalf("answer mismatch: %q", res.Answer)
	}
	if res.SessionID != "session-abc" {
		t.Fatalf("session id not carried back: %q", res.SessionID)
	}

	// The handler must see the authenticated identity, not a claimed one.
	if seen.PeerDeviceID != h.clientID.DeviceID {
		t.Fatalf("handler saw device %q, want %q", seen.PeerDeviceID, h.clientID.DeviceID)
	}
	// ...and the offer itself.
	if seen.Offer.SDP != testOfferSDP().SDP {
		t.Fatalf("handler saw a different offer: %q", seen.Offer.SDP)
	}
	if seen.Requested.Width != 1080 || seen.Requested.FPS != 60 {
		t.Fatalf("requested media not decoded: %+v", seen.Requested)
	}
}

// The reconnect target must be derived from the authenticated connection, never
// from the body: otherwise a paired device could aim this desktop at any host.
func TestSignalingServer_PeerOfferEndpointComesFromTheConnection(t *testing.T) {
	var seen PeerOfferRequest
	h := newPeerOfferHarness(t, func(_ context.Context, req PeerOfferRequest) (PeerOfferResult, error) {
		seen = req
		return PeerOfferResult{Answer: "v=0\r\n", Code: CodeOK}, nil
	})

	if _, err := h.client.SendPeerOffer(context.Background(), h.endpoint, testOfferSDP(), NegotiationRequest{}, 7804); err != nil {
		t.Fatalf("SendPeerOffer: %v", err)
	}
	if seen.Endpoint != "127.0.0.1:7804" {
		t.Fatalf("endpoint should be source host + announced port, got %q", seen.Endpoint)
	}

	// An unusable announce degrades to "no reconnect target" rather than a
	// half-built address.
	for _, port := range []uint32{0, 70000} {
		if _, err := h.client.SendPeerOffer(context.Background(), h.endpoint, testOfferSDP(), NegotiationRequest{}, port); err != nil {
			t.Fatalf("SendPeerOffer(port=%d): %v", port, err)
		}
		if seen.Endpoint != "" {
			t.Fatalf("port %d should yield no endpoint, got %q", port, seen.Endpoint)
		}
	}
}

// A busy desktop is a typed refusal the phone can act on, not a transport error.
func TestSignalingServer_PeerOfferBusyIsTyped(t *testing.T) {
	h := newPeerOfferHarness(t, func(context.Context, PeerOfferRequest) (PeerOfferResult, error) {
		return PeerOfferResult{
			Code:      CodeSessionBusy,
			Message:   "device is already in an active session (CONNECTED)",
			SessionID: "session-live",
		}, nil
	})

	res, err := h.client.SendPeerOffer(context.Background(), h.endpoint, testOfferSDP(), NegotiationRequest{}, 0)
	if err != nil {
		t.Fatalf("a busy desktop must be a typed refusal, not an error: %v", err)
	}
	if res.Accepted {
		t.Fatal("busy offer must not be accepted")
	}
	if res.Code != CodeSessionBusy {
		t.Fatalf("expected SESSION_BUSY, got %s", res.Code)
	}
	if res.SessionID != "session-live" {
		t.Fatalf("busy response should name the live session, got %q", res.SessionID)
	}
	if !strings.Contains(res.Message, "active session") {
		t.Fatalf("busy message not carried: %q", res.Message)
	}
}

func TestSignalingServer_PeerOfferRefusalsAreTyped(t *testing.T) {
	for _, tc := range []struct {
		name string
		code Code
	}{
		{"permission denied", CodePermissionDenied},
		{"consent revoked", CodeConsentRevoked},
		{"capture failed", CodeCaptureFailed},
		{"transport failed", CodeTransportFailed},
		{"unsupported media", CodeUnsupportedMediaParams},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newPeerOfferHarness(t, func(context.Context, PeerOfferRequest) (PeerOfferResult, error) {
				return PeerOfferResult{Code: tc.code, Message: tc.name}, nil
			})
			res, err := h.client.SendPeerOffer(context.Background(), h.endpoint, testOfferSDP(), NegotiationRequest{}, 0)
			if err != nil {
				t.Fatalf("typed refusal must not surface as an error: %v", err)
			}
			if res.Accepted || res.Code != tc.code {
				t.Fatalf("expected %s, got accepted=%v code=%s", tc.code, res.Accepted, res.Code)
			}
		})
	}
}

// A build with no peer-offer support (older desktop, or a test double) must say
// so in the shared vocabulary instead of returning an HTTP shape the phone has
// to special-case.
func TestSignalingServer_PeerOfferUnsupportedWithoutHandler(t *testing.T) {
	h := newPeerOfferHarness(t, nil)

	res, err := h.client.SendPeerOffer(context.Background(), h.endpoint, testOfferSDP(), NegotiationRequest{}, 0)
	if err != nil {
		t.Fatalf("unsupported must be typed, not a transport error: %v", err)
	}
	if res.Accepted {
		t.Fatal("an unsupported peer offer must not be accepted")
	}
	if res.Code != CodeUnsupportedMediaParams {
		t.Fatalf("expected UNSUPPORTED_MEDIA_PARAMS, got %s", res.Code)
	}
}

func TestSignalingServer_PeerOfferRejectsMalformedRequests(t *testing.T) {
	h := newPeerOfferHarness(t, func(context.Context, PeerOfferRequest) (PeerOfferResult, error) {
		t.Error("handler must not run for a malformed request")
		return PeerOfferResult{}, nil
	})

	post := func(t *testing.T, body []byte, path, method string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(context.Background(), method, "http://"+h.endpoint+path, bytes.NewReader(body))
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		headers, err := crypto.SignRequest(h.clientID, method, path, body)
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		return resp
	}

	for _, tc := range []struct {
		name   string
		body   string
		status int
		code   Code
	}{
		{"not json", "{", http.StatusBadRequest, CodeInvalidArgument},
		{"no offer", `{"protocol_version":1}`, http.StatusBadRequest, CodeInvalidArgument},
		{"empty sdp", `{"protocol_version":1,"offer":{"type":"offer","sdp":""}}`, http.StatusBadRequest, CodeInvalidArgument},
		{"version mismatch", `{"protocol_version":99,"offer":{"type":"offer","sdp":"v=0"}}`, http.StatusConflict, CodeIncompatibleVersion},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := post(t, []byte(tc.body), "/session/peer-offer", http.MethodPost)
			defer resp.Body.Close()
			if resp.StatusCode != tc.status {
				t.Fatalf("status %d, want %d", resp.StatusCode, tc.status)
			}
			var payload offerResponse
			if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
				t.Fatalf("decode refusal: %v", err)
			}
			if payload.Code != string(tc.code) {
				t.Fatalf("code %q, want %q", payload.Code, tc.code)
			}
		})
	}

	t.Run("wrong method", func(t *testing.T) {
		resp := post(t, nil, "/session/peer-offer", http.MethodGet)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("status %d, want 405", resp.StatusCode)
		}
	})
}

// Replaying a captured offer must not create a second session.
func TestSignalingServer_PeerOfferReplayIsRejected(t *testing.T) {
	h := newPeerOfferHarness(t, func(context.Context, PeerOfferRequest) (PeerOfferResult, error) {
		return PeerOfferResult{Answer: "v=0\r\n", Code: CodeOK}, nil
	})

	body, err := json.Marshal(peerOfferRequest{
		ProtocolVersion: signalingVersion,
		Version:         versionAdvert{Min: signalingVersion, Max: signalingVersion},
		Offer:           sdpPayload{Type: "offer", SDP: testOfferSDP().SDP},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	headers, err := crypto.SignRequest(h.clientID, http.MethodPost, "/session/peer-offer", body)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	send := func() int {
		req, err := http.NewRequest(http.MethodPost, "http://"+h.endpoint+"/session/peer-offer", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	if got := send(); got != http.StatusOK {
		t.Fatalf("first offer status %d, want 200", got)
	}
	if got := send(); got != http.StatusUnauthorized {
		t.Fatalf("replayed offer status %d, want 401", got)
	}
}

// A body with a bogus port must never become a connect target, and the retry
// path must still be able to reach the peer's own signaling server.
func TestSignalingServer_PeerOfferDoesNotTrustBodyForReachability(t *testing.T) {
	var seen PeerOfferRequest
	h := newPeerOfferHarness(t, func(_ context.Context, req PeerOfferRequest) (PeerOfferResult, error) {
		seen = req
		return PeerOfferResult{Answer: "v=0\r\n", Code: CodeOK}, nil
	})

	body, err := json.Marshal(map[string]any{
		"protocol_version": signalingVersion,
		"offer":            map[string]string{"type": "offer", "sdp": testOfferSDP().SDP},
		"signaling_port":   7804,
		"endpoint":         "example.com:9999",
		"redirect":         "http://attacker.invalid",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	headers, err := crypto.SignRequest(h.clientID, http.MethodPost, "/session/peer-offer", body)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, "http://"+h.endpoint+"/session/peer-offer", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	if seen.Endpoint != "127.0.0.1:7804" {
		t.Fatalf("endpoint must ignore body-supplied hosts, got %q", seen.Endpoint)
	}
}
