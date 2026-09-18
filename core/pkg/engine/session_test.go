package engine

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/crypto"
	"github.com/om051p/phonebridge/core/pkg/discovery"
)

func TestSession_ValidLifecycleTransitions(t *testing.T) {
	var transitions []string
	cb := func(oldState, newState SessionState, reason string) {
		transitions = append(transitions, oldState.String()+"->"+newState.String())
	}

	reg := discovery.NewDeviceRegistry(discovery.DefaultRegistryConfig(), nil)
	sess := NewSession("sess-1", DefaultSessionConfig(), reg, cb)

	if sess.State() != StateDisconnected {
		t.Fatalf("expected initial StateDisconnected, got %v", sess.State())
	}

	// 1. Disconnected -> Discovering
	if err := sess.Transition(StateDiscovering, "start discovery"); err != nil {
		t.Fatal(err)
	}

	// 2. Discovering -> Connecting
	if err := sess.Transition(StateConnecting, "device found"); err != nil {
		t.Fatal(err)
	}

	// 3. Connecting -> Connected
	if err := sess.Transition(StateConnected, "webrtc connected"); err != nil {
		t.Fatal(err)
	}

	// 4. Connected -> Streaming
	if err := sess.Transition(StateStreaming, "frames arriving"); err != nil {
		t.Fatal(err)
	}

	// 5. Streaming -> Reconnecting
	if err := sess.Transition(StateReconnecting, "ice disconnect"); err != nil {
		t.Fatal(err)
	}

	// 6. Reconnecting -> Streaming
	if err := sess.Transition(StateStreaming, "ice recovered"); err != nil {
		t.Fatal(err)
	}

	// 7. Streaming -> Stopped
	if err := sess.Stop("user stop"); err != nil {
		t.Fatal(err)
	}

	expected := []string{
		"DISCONNECTED->DISCOVERING",
		"DISCOVERING->CONNECTING",
		"CONNECTING->CONNECTED",
		"CONNECTED->STREAMING",
		"STREAMING->RECONNECTING",
		"RECONNECTING->STREAMING",
		"STREAMING->STOPPED",
	}

	if len(transitions) != len(expected) {
		t.Fatalf("expected %d transitions, got %d: %v", len(expected), len(transitions), transitions)
	}
	for i, exp := range expected {
		if transitions[i] != exp {
			t.Errorf("transition %d: expected %s, got %s", i, exp, transitions[i])
		}
	}
}

func TestSession_InvalidTransitionsRejected(t *testing.T) {
	reg := discovery.NewDeviceRegistry(discovery.DefaultRegistryConfig(), nil)
	sess := NewSession("sess-2", DefaultSessionConfig(), reg, nil)

	// Cannot jump directly from Disconnected to Streaming
	if err := sess.Transition(StateStreaming, "illegal jump"); err == nil {
		t.Fatal("expected error jumping Disconnected -> Streaming, got nil")
	}

	// Disconnected -> Connecting is allowed
	if err := sess.Transition(StateConnecting, "direct connect"); err != nil {
		t.Fatal(err)
	}

	// Cannot jump directly from Connecting to Streaming (must connect first)
	if err := sess.Transition(StateStreaming, "illegal jump"); err == nil {
		t.Fatal("expected error jumping Connecting -> Streaming, got nil")
	}
}

func TestSession_FailureTransition(t *testing.T) {
	var lastErr string
	cb := func(oldState, newState SessionState, reason string) {
		if newState == StateFailed {
			lastErr = reason
		}
	}

	reg := discovery.NewDeviceRegistry(discovery.DefaultRegistryConfig(), nil)
	sess := NewSession("sess-3", DefaultSessionConfig(), reg, cb)

	_ = sess.Transition(StateConnecting, "signaling")
	sess.Fail(errors.New("handshake timeout"))

	if sess.State() != StateFailed {
		t.Fatalf("expected StateFailed, got %v", sess.State())
	}
	if lastErr != "handshake timeout" {
		t.Fatalf("expected 'handshake timeout' reason, got %q", lastErr)
	}

	snap := sess.Snapshot()
	if snap.ErrorMessage != "handshake timeout" {
		t.Fatalf("snapshot error message mismatch: %q", snap.ErrorMessage)
	}
}

func TestSession_LocateTargetFromRegistry(t *testing.T) {
	reg := discovery.NewDeviceRegistry(discovery.DefaultRegistryConfig(), nil)
	reg.Upsert(discovery.Device{
		ID:   "target-phone",
		Name: "POCO F5",
	})

	cfg := DefaultSessionConfig()
	cfg.TargetDeviceID = "target-phone"
	sess := NewSession("sess-4", cfg, reg, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	dev, err := sess.LocateTarget(ctx)
	if err != nil {
		t.Fatalf("failed to locate target: %v", err)
	}
	if dev.ID != "target-phone" || dev.Name != "POCO F5" {
		t.Fatalf("unexpected located device: %+v", dev)
	}
}

func TestSession_LocateTargetTimeout(t *testing.T) {
	reg := discovery.NewDeviceRegistry(discovery.DefaultRegistryConfig(), nil)
	cfg := DefaultSessionConfig()
	cfg.TargetDeviceID = "non-existent-phone"
	cfg.DiscoveryTimeout = 50 * time.Millisecond
	sess := NewSession("sess-5", cfg, reg, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := sess.LocateTarget(ctx)
	if err == nil {
		t.Fatal("expected error locating non-existent device, got nil")
	}
	if sess.State() != StateFailed {
		t.Fatalf("expected StateFailed on locate timeout, got %v", sess.State())
	}
}

func TestSession_UntrustedTargetRejection(t *testing.T) {
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "trusted_devices.json")
	ts, err := crypto.NewTrustStore(storePath)
	if err != nil {
		t.Fatalf("create trust store: %v", err)
	}

	reg := discovery.NewDeviceRegistry(discovery.DefaultRegistryConfig(), nil)
	cfg := DefaultSessionConfig()
	cfg.TargetDeviceID = "untrusted-device-id-123"
	cfg.TrustStore = ts

	sess := NewSession("sess-untrusted", cfg, reg, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = sess.Connect(ctx, "127.0.0.1:7804", nil)
	if err == nil {
		t.Fatal("expected Connect to fail for untrusted device, got nil")
	}

	if sess.State() != StateFailed {
		t.Fatalf("expected session state to be StateFailed, got %v", sess.State())
	}
}

func TestSession_TrustedDeviceConnect(t *testing.T) {
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "trusted_devices.json")
	ts, err := crypto.NewTrustStore(storePath)
	if err != nil {
		t.Fatalf("create trust store: %v", err)
	}

	targetID := "trusted-device-id-456"
	trustedDev := crypto.TrustEntry{
		DeviceID:    targetID,
		DisplayName: "Pixel 7 Pro",
		Platform:    "android",
		PublicKey:   make([]byte, 32),
		PairedAt:    time.Now(),
	}
	if err := ts.AddTrusted(trustedDev); err != nil {
		t.Fatalf("add trusted device: %v", err)
	}

	reg := discovery.NewDeviceRegistry(discovery.DefaultRegistryConfig(), nil)
	cfg := DefaultSessionConfig()
	cfg.TargetDeviceID = targetID
	cfg.TrustStore = ts
	cfg.ConnectTimeout = 100 * time.Millisecond

	sess := NewSession("sess-trusted", cfg, reg, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// It passes the trust check, and then attempts signaling (which fails on unreachable port)
	err = sess.Connect(ctx, "127.0.0.1:65534", nil)
	if err == nil {
		t.Fatal("expected network error on unreachable port")
	}
	// Verify it was NOT rejected by trust check
	if err.Error() == "device "+targetID+" is not trusted: pairing required" {
		t.Fatalf("unexpected trust rejection for trusted device: %v", err)
	}
}
