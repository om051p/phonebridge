package crypto

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// PairingRequestPayload is sent over HTTP to request pairing.
type PairingRequestPayload struct {
	DisplayName  string `json:"display_name"`
	Platform     string `json:"platform"`
	PublicKey    string `json:"public_key"` // hex
	PairingToken string `json:"pairing_token"`
}

// PairingAcceptPayload is returned by the responder when receiving a valid pair request.
type PairingAcceptPayload struct {
	DisplayName string `json:"display_name"`
	Platform    string `json:"platform"`
	PublicKey   string `json:"public_key"` // hex
	SAS         string `json:"sas"`
}

// PairingConfirmPayload is sent to confirm mutual SAS verification.
type PairingConfirmPayload struct {
	DeviceID     string `json:"device_id"`
	PairingToken string `json:"pairing_token"`
	SAS          string `json:"sas"`
	Confirmed    bool   `json:"confirmed"`
	Signature    string `json:"signature"` // hex signature over token:sas
}

// PairingStatusPayload is returned after pairing confirmation.
type PairingStatusPayload struct {
	Status string `json:"status"` // "paired", "rejected", "pending"
	Error  string `json:"error,omitempty"`
}

// PairingClient orchestrates client-side device pairing over LAN.
type PairingClient struct {
	client *http.Client
}

// NewPairingClient creates a pairing client with the specified timeout.
func NewPairingClient(timeout time.Duration) *PairingClient {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &PairingClient{
		client: &http.Client{Timeout: timeout},
	}
}

// Pair performs a complete mutual pairing handshake with the target device.
func (c *PairingClient) Pair(
	ctx context.Context,
	endpoint string,
	identity *DeviceIdentity,
	store *TrustStore,
	confirmSAS func(remoteName, sas string) bool,
) (*TrustEntry, error) {
	if identity == nil {
		return nil, fmt.Errorf("local device identity is nil")
	}

	// 1. Generate ephemeral pairing token
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generate pairing token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	reqPayload := PairingRequestPayload{
		DisplayName:  identity.DisplayName,
		Platform:     identity.Platform,
		PublicKey:    hex.EncodeToString(identity.PublicKey),
		PairingToken: token,
	}
	reqData, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal pair request: %w", err)
	}

	// 2. Send PairRequest to target device
	reqURL := fmt.Sprintf("http://%s/pairing/request", endpoint)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(reqData))
	if err != nil {
		return nil, fmt.Errorf("create pair request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send pair request to %s: %w", reqURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("pairing request rejected (%d): %s", resp.StatusCode, string(body))
	}

	var acceptPayload PairingAcceptPayload
	if err := json.NewDecoder(resp.Body).Decode(&acceptPayload); err != nil {
		return nil, fmt.Errorf("decode pair accept response: %w", err)
	}

	remotePub, err := hex.DecodeString(acceptPayload.PublicKey)
	if err != nil || len(remotePub) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid remote public key")
	}

	// 3. Compute local SAS and verify remote matches
	expectedSAS := CalculateSAS(identity.PublicKey, remotePub, token)
	if acceptPayload.SAS != expectedSAS {
		return nil, fmt.Errorf("SAS verification failed: remote %s != expected %s", acceptPayload.SAS, expectedSAS)
	}

	// 4. Prompt user confirmation callback
	userApproved := true
	if confirmSAS != nil {
		userApproved = confirmSAS(acceptPayload.DisplayName, expectedSAS)
	}

	// 5. Send PairingConfirmPayload
	confirmSigMaterial := fmt.Sprintf("%s:%s", token, expectedSAS)
	confirmSig := Sign(identity.PrivateKey, []byte(confirmSigMaterial))

	confirmPayload := PairingConfirmPayload{
		DeviceID:     identity.DeviceID,
		PairingToken: token,
		SAS:          expectedSAS,
		Confirmed:    userApproved,
		Signature:    hex.EncodeToString(confirmSig),
	}
	confirmData, err := json.Marshal(confirmPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal pair confirm: %w", err)
	}

	confirmURL := fmt.Sprintf("http://%s/pairing/confirm", endpoint)
	confirmReq, err := http.NewRequestWithContext(ctx, http.MethodPost, confirmURL, bytes.NewReader(confirmData))
	if err != nil {
		return nil, fmt.Errorf("create pair confirm request: %w", err)
	}
	confirmReq.Header.Set("Content-Type", "application/json")

	confirmResp, err := c.client.Do(confirmReq)
	if err != nil {
		return nil, fmt.Errorf("send pair confirm to %s: %w", confirmURL, err)
	}
	defer confirmResp.Body.Close()

	if !userApproved {
		return nil, fmt.Errorf("user rejected pairing SAS")
	}

	if confirmResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(confirmResp.Body)
		return nil, fmt.Errorf("pairing confirm rejected (%d): %s", confirmResp.StatusCode, string(body))
	}

	// 6. Commit trusted peer to trust store
	remoteDeviceID := Fingerprint(remotePub)
	entry := TrustEntry{
		DeviceID:    remoteDeviceID,
		DisplayName: acceptPayload.DisplayName,
		Platform:    acceptPayload.Platform,
		PublicKey:   remotePub,
		PairedAt:    time.Now(),
		LastSeen:    time.Now(),
		Revoked:     false,
	}

	if store != nil {
		if err := store.AddTrusted(entry); err != nil {
			return nil, fmt.Errorf("save trusted device: %w", err)
		}
	}

	return &entry, nil
}
