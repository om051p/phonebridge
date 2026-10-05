package engine

import (
	"context"
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	pion "github.com/pion/webrtc/v4"

	"github.com/om051p/phonebridge/core/pkg/crypto"
)

// DefaultSignalingPort is the canonical LAN signaling port (DEC-022).
const DefaultSignalingPort = 7804

// pendingPairingTTL bounds how long a pairing token stays valid. An expired
// token is rejected and removed at confirm time; the user simply pairs again
// (idempotent: trust commits upsert by device ID).
const pendingPairingTTL = 5 * time.Minute

// confirmPollIntervalDefault paces requester confirm polls while the receiver
// reports 202 pending. Every poll is its own short HTTP exchange, so the 10 s
// transport budgets are never the bound — the token TTL is.
const confirmPollIntervalDefault = 3 * time.Second

// pendingPairingMaxPollElapsed caps the whole confirm-poll wait at the
// pairing-token TTL: polling past it can only ever meet an expired token.
const pendingPairingMaxPollElapsed = 5 * time.Minute

type serverPendingPairing struct {
	token          string
	remoteName     string
	remotePlatform string
	remotePub      []byte
	sas            string
	createdAt      time.Time
	// approved is the receiving user's explicit decision: nil while the
	// request is still awaiting them, true/false once they accept/reject.
	// Trust is committed only after approved==true AND a valid confirm.
	approved *bool
}

// InboundPairing is a receiver-side view of one pending pairing request,
// surfaced to the local UI so the user can accept or reject it.
type InboundPairing struct {
	Token          string
	RemoteName     string
	RemotePlatform string
	SAS            string
	CreatedAt      time.Time
}

// SignalingServerConfig configures the inbound HTTP LAN signaling server.
type SignalingServerConfig struct {
	// Port to listen on. If 0, an ephemeral port is allocated.
	Port int

	// Identity of the local device for signing/pairing.
	Identity *crypto.DeviceIdentity

	// TrustStore for verifying incoming authenticated requests and storing new pairings.
	TrustStore *crypto.TrustStore

	// OfferHandler handles incoming POST /session/offer.
	OfferHandler func(req NegotiationRequest) (NegotiationResponse, error)

	// AnswerHandler handles incoming POST /session/answer.
	AnswerHandler func(answer pion.SessionDescription) error

	// StopHandler handles incoming POST /session/stop.
	//
	// peerDeviceID is the AUTHENTICATED device that asked for the stop (its
	// signature was verified against the trust store), so the manager can stop
	// exactly the session that device is part of and nothing else.
	StopHandler func(peerDeviceID, reason string, code Code) error

	// PeerOfferHandler handles incoming POST /session/peer-offer, where the peer
	// is itself the capture device and supplies its own SDP offer. The returned
	// answer is carried back to the peer in this same response. Nil means the
	// host does not accept peer-started sessions.
	PeerOfferHandler func(ctx context.Context, req PeerOfferRequest) (PeerOfferResult, error)
}

// SignalingServer serves the LAN signaling protocol over HTTP with Ed25519 authentication.
type SignalingServer struct {
	cfg        SignalingServerConfig
	listener   net.Listener
	httpServer *http.Server
	nonces     *crypto.NonceCache

	mu              sync.Mutex
	pendingPairings map[string]serverPendingPairing
	actualPort      int
}

// NewSignalingServer constructs a new SignalingServer.
func NewSignalingServer(cfg SignalingServerConfig) *SignalingServer {
	return &SignalingServer{
		cfg:             cfg,
		nonces:          crypto.NewNonceCache(),
		pendingPairings: make(map[string]serverPendingPairing),
	}
}

// Start begins listening on the configured port.
func (s *SignalingServer) Start(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", s.cfg.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("signaling server listen on %s: %w", addr, err)
	}
	s.listener = ln
	s.actualPort = ln.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/pairing/request", s.handlePairingRequest)
	mux.HandleFunc("/pairing/confirm", s.handlePairingConfirm)
	mux.HandleFunc("/session/offer", s.handleSessionOffer)
	mux.HandleFunc("/session/peer-offer", s.handleSessionPeerOffer)
	mux.HandleFunc("/session/answer", s.handleSessionAnswer)
	mux.HandleFunc("/session/stop", s.handleSessionStop)

	s.httpServer = &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// server stopped
		}
	}()

	return nil
}

// Port returns the actual listening TCP port.
func (s *SignalingServer) Port() int {
	return s.actualPort
}

// Close gracefully terminates the signaling server.
func (s *SignalingServer) Close() error {
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(ctx)
	}
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

func (s *SignalingServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *SignalingServer) handlePairingRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.Identity == nil {
		http.Error(w, `{"error":"device identity not configured"}`, http.StatusServiceUnavailable)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}

	var req crypto.PairingRequestPayload
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"error":"malformed json"}`, http.StatusBadRequest)
		return
	}

	remotePub, err := hex.DecodeString(req.PublicKey)
	if err != nil || len(remotePub) != ed25519.PublicKeySize {
		http.Error(w, `{"error":"invalid public key length"}`, http.StatusBadRequest)
		return
	}

	sas := crypto.CalculateSAS(s.cfg.Identity.PublicKey, remotePub, req.PairingToken)

	// Re-pair of an already-trusted key is redundant, never a new request:
	// answer 409 so the requester can say "already trusted" instead of
	// opening an approval dialog for a peer both sides already trust. A
	// revoked key re-pairs through the normal approval flow below.
	if s.cfg.TrustStore != nil {
		if entry, ok := s.cfg.TrustStore.FindByPublicKey(remotePub); ok && !entry.Revoked {
			http.Error(w, `{"error":"already trusted"}`, http.StatusConflict)
			return
		}
	}

	s.mu.Lock()
	// Duplicate-request protection: one pending request per remote key. A
	// retry (new token, same peer) supersedes the older one instead of
	// stacking dialogs on the receiver.
	for tok, p := range s.pendingPairings {
		if subtle.ConstantTimeCompare(p.remotePub, remotePub) == 1 {
			delete(s.pendingPairings, tok)
		}
	}
	s.pendingPairings[req.PairingToken] = serverPendingPairing{
		token:          req.PairingToken,
		remoteName:     req.DisplayName,
		remotePlatform: req.Platform,
		remotePub:      remotePub,
		sas:            sas,
		createdAt:      time.Now(),
	}
	s.mu.Unlock()

	resp := crypto.PairingAcceptPayload{
		DisplayName: s.cfg.Identity.DisplayName,
		Platform:    s.cfg.Identity.Platform,
		PublicKey:   hex.EncodeToString(s.cfg.Identity.PublicKey),
		SAS:         sas,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *SignalingServer) handlePairingConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}

	var req crypto.PairingConfirmPayload
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"error":"malformed json"}`, http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	pending, exists := s.pendingPairings[req.PairingToken]
	s.mu.Unlock()

	if exists && time.Since(pending.createdAt) > pendingPairingTTL {
		s.mu.Lock()
		delete(s.pendingPairings, req.PairingToken)
		s.mu.Unlock()
		http.Error(w, `{"error":"pairing token expired"}`, http.StatusBadRequest)
		return
	}

	if !exists || subtle.ConstantTimeCompare([]byte(pending.sas), []byte(req.SAS)) != 1 {
		if exists {
			s.mu.Lock()
			delete(s.pendingPairings, req.PairingToken)
			s.mu.Unlock()
		}
		http.Error(w, `{"error":"invalid or expired pairing token/sas"}`, http.StatusBadRequest)
		return
	}

	if !req.Confirmed {
		// Requester-side rejection withdraws the request: the receiver's
		// pending dialog disappears on its next refresh instead of asking
		// about a peer that already walked away.
		s.mu.Lock()
		delete(s.pendingPairings, req.PairingToken)
		s.mu.Unlock()
		http.Error(w, `{"error":"pairing rejected by user"}`, http.StatusBadRequest)
		return
	}

	// The receiving user has not decided yet: hold the token (202) so the
	// requester polls. Trust is committed only after an explicit approval.
	if pending.approved == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(crypto.PairingStatusPayload{Status: "pending"})
		return
	}
	if !*pending.approved {
		s.mu.Lock()
		delete(s.pendingPairings, req.PairingToken)
		s.mu.Unlock()
		http.Error(w, `{"error":"pairing rejected by user"}`, http.StatusBadRequest)
		return
	}

	sigBytes, err := hex.DecodeString(req.Signature)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		http.Error(w, `{"error":"invalid confirmation signature format"}`, http.StatusBadRequest)
		return
	}

	sigMaterial := fmt.Sprintf("%s:%s", req.PairingToken, req.SAS)
	if !crypto.Verify(pending.remotePub, []byte(sigMaterial), sigBytes) {
		s.mu.Lock()
		delete(s.pendingPairings, req.PairingToken)
		s.mu.Unlock()
		http.Error(w, `{"error":"invalid confirmation signature"}`, http.StatusUnauthorized)
		return
	}

	// The requester identifies itself with req.DeviceID. Never trust it blindly:
	// the authenticated identity is pending.remotePub (the key this token was
	// issued for and the signature was verified against). A mismatched ID
	// would create a second logical trust record for the same key, which is
	// exactly the duplicate-device accumulation the connection audit found.
	if req.DeviceID != crypto.Fingerprint(pending.remotePub) {
		s.mu.Lock()
		delete(s.pendingPairings, req.PairingToken)
		s.mu.Unlock()
		http.Error(w, `{"error":"device_id does not match authenticated public key"}`, http.StatusBadRequest)
		return
	}

	// Single-use token: a successful pairing consumes it, so a replayed
	// confirm can never commit trust twice.
	s.mu.Lock()
	delete(s.pendingPairings, req.PairingToken)
	s.mu.Unlock()

	if s.cfg.TrustStore != nil {
		_ = s.cfg.TrustStore.UpsertCanonical(crypto.TrustEntry{
			DeviceID:    req.DeviceID,
			DisplayName: pending.remoteName,
			Platform:    pending.remotePlatform,
			PublicKey:   pending.remotePub,
			PairedAt:    time.Now(),
			LastSeen:    time.Now(),
			Revoked:     false,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(crypto.PairingStatusPayload{Status: "paired"})
}

// ApprovePairing records the receiving user's explicit decision for one
// pending inbound pairing request. It returns false when the token is
// unknown or already expired (expired entries are swept). Approval holds the
// token until the requester's signed confirm commits trust; rejection is
// terminal for the receiver, so the token is dropped immediately and the
// requester's next confirm poll learns the outcome as 400.
func (s *SignalingServer) ApprovePairing(token string, approved bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	pending, ok := s.pendingPairings[token]
	if !ok {
		return false
	}
	if time.Since(pending.createdAt) > pendingPairingTTL {
		delete(s.pendingPairings, token)
		return false
	}
	if !approved {
		// A rejected request must not linger in PendingPairings: the UI
		// would keep asking the receiver about a decision they already
		// made until the TTL sweep.
		delete(s.pendingPairings, token)
		return true
	}
	pending.approved = &approved
	s.pendingPairings[token] = pending
	return true
}

// PendingPairings snapshots the inbound pairing requests still awaiting the
// receiving user's decision. Expired entries are swept and omitted, so the UI
// auto-dismisses stale dialogs by refreshing this list.
func (s *SignalingServer) PendingPairings() []InboundPairing {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]InboundPairing, 0, len(s.pendingPairings))
	for token, p := range s.pendingPairings {
		if time.Since(p.createdAt) > pendingPairingTTL {
			delete(s.pendingPairings, token)
			continue
		}
		out = append(out, InboundPairing{
			Token:          token,
			RemoteName:     p.remoteName,
			RemotePlatform: p.remotePlatform,
			SAS:            p.sas,
			CreatedAt:      p.createdAt,
		})
	}
	return out
}

func (s *SignalingServer) verifyAuth(r *http.Request, body []byte) (string, error) {
	if s.cfg.TrustStore == nil {
		return "", nil // No auth enforced if trust store not provided (e.g. tests)
	}
	return crypto.VerifyRequest(s.cfg.TrustStore, r.Method, r.URL.Path, body, r.Header.Get, s.nonces, 0)
}

func (s *SignalingServer) handleSessionOffer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}

	deviceID, err := s.verifyAuth(r, body)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusUnauthorized)
		return
	}
	if s.cfg.TrustStore != nil && deviceID != "" {
		s.cfg.TrustStore.TouchLastSeen(deviceID)
	}

	var req offerRequest
	_ = json.Unmarshal(body, &req)

	if s.cfg.OfferHandler == nil {
		resp := offerResponse{
			Code:         "UNSUPPORTED_MEDIA_PARAMS",
			Message:      "session offer not supported by host",
			RejectReason: "host has no capture or offer handler",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	negReq := NegotiationRequest{
		Requested:    req.Requested.toMediaParams(),
		PeerDeviceID: deviceID,
	}
	ans, err := s.cfg.OfferHandler(negReq)
	if err != nil {
		resp := offerResponse{
			Code:         "INTERNAL",
			Message:      err.Error(),
			RejectReason: err.Error(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	accepted := ans.Accepted
	var actual *mediaParamsJSON
	if ans.ActualKnown {
		j := toMediaParamsJSON(ans.Actual)
		actual = &j
	}

	resp := offerResponse{
		Type:            "offer",
		SDP:             ans.Offer,
		ProtocolVersion: signalingVersion,
		Code:            string(ans.Code),
		Message:         ans.Message,
		Accepted:        &accepted,
		RejectReason:    ans.RejectReason,
		Actual:          actual,
	}
	for _, c := range ans.Capabilities {
		resp.Capabilities = append(resp.Capabilities, mediaCapabilityJSON{
			Codecs:         c.Codecs,
			MaxWidth:       uint32(c.MaxWidth),
			MaxHeight:      uint32(c.MaxHeight),
			MaxFPS:         uint32(c.MaxFPS),
			SupportsScreen: c.SupportsScreen,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if !accepted {
		w.WriteHeader(http.StatusConflict)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// peerSignalingEndpoint derives where to reach a peer again from the
// AUTHENTICATED connection plus the port the peer announced.
//
// The host half comes from r.RemoteAddr, never from the request body: otherwise
// any paired device could make this daemon POST to an arbitrary address. The
// port is validated as a real TCP port, and an unusable announce simply yields
// an empty endpoint — the session then has no reconnect target, which is a
// degradation, not a security hole (the initiator path still validates trust on
// every attempt).
func peerSignalingEndpoint(r *http.Request, announcedPort uint32) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || host == "" {
		return ""
	}
	if announcedPort == 0 || announcedPort > 65535 {
		return ""
	}
	return net.JoinHostPort(host, strconv.Itoa(int(announcedPort)))
}

// handleSessionPeerOffer answers a session offer from a peer that is the capture
// device (DEC-022). It is the counterpart of handleSessionOffer: same auth, same
// vocabulary, same error shape — the SDP direction is simply reversed.
func (s *SignalingServer) handleSessionPeerOffer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Bounded read: an SDP offer with a full candidate list is tens of KiB, so
	// 256 KiB is generous while keeping a hostile body from being buffered whole.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 256<<10))
	if err != nil {
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}

	deviceID, err := s.verifyAuth(r, body)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusUnauthorized)
		return
	}
	if s.cfg.TrustStore != nil && deviceID != "" {
		s.cfg.TrustStore.TouchLastSeen(deviceID)
	}

	// sessionID is carried on refusals too: a SESSION_BUSY answer that names the
	// live session lets the caller tell "someone else is connected" from "this
	// device thinks I am still connected", which need opposite remedies.
	writePeerOfferErr := func(status int, code Code, msg, sessionID string) {
		accepted := false
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(offerResponse{
			ProtocolVersion: signalingVersion,
			Code:            string(code),
			Message:         msg,
			Accepted:        &accepted,
			RejectReason:    msg,
			SessionID:       sessionID,
		})
	}

	var req peerOfferRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writePeerOfferErr(http.StatusBadRequest, CodeInvalidArgument, "invalid peer-offer body", "")
		return
	}
	if req.Offer.SDP == "" {
		writePeerOfferErr(http.StatusBadRequest, CodeInvalidArgument, "peer offer carries no sdp", "")
		return
	}
	if req.ProtocolVersion != 0 && req.ProtocolVersion != signalingVersion {
		writePeerOfferErr(http.StatusConflict, CodeIncompatibleVersion, fmt.Sprintf(
			"peer speaks signaling version %d, this build speaks %d", req.ProtocolVersion, signalingVersion), "")
		return
	}

	if s.cfg.PeerOfferHandler == nil {
		writePeerOfferErr(http.StatusConflict, CodeUnsupportedMediaParams,
			"host does not accept peer-started sessions", "")
		return
	}

	res, err := s.cfg.PeerOfferHandler(r.Context(), PeerOfferRequest{
		PeerDeviceID: deviceID,
		// Derived from the authenticated connection plus the announced port: a
		// body-supplied host is ignored on purpose (see peerSignalingEndpoint).
		Endpoint: peerSignalingEndpoint(r, req.SignalingPort),
		Offer: pion.SessionDescription{
			Type: pion.SDPTypeOffer,
			SDP:  req.Offer.SDP,
		},
		Requested: req.Requested.toMediaParams(),
	})
	if err != nil {
		writePeerOfferErr(http.StatusInternalServerError, CodeTransportFailed, err.Error(), "")
		return
	}

	if res.Code != CodeOK || res.Answer == "" {
		code := res.Code
		if code == "" {
			code = CodeTransportFailed
		}
		msg := res.Message
		if msg == "" {
			msg = "peer offer could not be answered"
		}
		// SESSION_BUSY and PERMISSION_DENIED are outcomes the peer must be able
		// to tell apart from a malformed request; both are reported as conflicts
		// with a typed code, matching how the phone answers the same conditions.
		status := http.StatusConflict
		if code == CodeInvalidArgument {
			status = http.StatusBadRequest
		}
		writePeerOfferErr(status, code, msg, res.SessionID)
		return
	}

	accepted := true
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(offerResponse{
		Type:            "answer",
		SDP:             res.Answer,
		ProtocolVersion: signalingVersion,
		Code:            string(CodeOK),
		Message:         res.Message,
		Accepted:        &accepted,
		SessionID:       res.SessionID,
	})
}

func (s *SignalingServer) handleSessionAnswer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}

	deviceID, err := s.verifyAuth(r, body)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusUnauthorized)
		return
	}
	if s.cfg.TrustStore != nil && deviceID != "" {
		s.cfg.TrustStore.TouchLastSeen(deviceID)
	}

	var payload sdpPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, `{"error":"invalid sdp payload"}`, http.StatusBadRequest)
		return
	}

	if s.cfg.AnswerHandler != nil {
		err := s.cfg.AnswerHandler(pion.SessionDescription{
			Type: pion.SDPTypeAnswer,
			SDP:  payload.SDP,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *SignalingServer) handleSessionStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}

	deviceID, err := s.verifyAuth(r, body)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusUnauthorized)
		return
	}
	if s.cfg.TrustStore != nil && deviceID != "" {
		s.cfg.TrustStore.TouchLastSeen(deviceID)
	}

	var payload stopPayload
	_ = json.Unmarshal(body, &payload)

	if s.cfg.StopHandler != nil {
		code, _ := ParseCode(payload.ReasonCode)
		_ = s.cfg.StopHandler(deviceID, payload.Reason, code)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
