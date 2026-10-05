//go:build android || jni

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/clipboard"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"google.golang.org/protobuf/proto"
)

type mockClipboardHost struct {
	mu             sync.Mutex
	platformWrites []mockPlatformWrite
	sentUpdates    [][]byte
	oversizedSizes []int

	writeReturn bool
	sendReturn  bool
}

type mockPlatformWrite struct {
	mimeType string
	payload  []byte
}

func newMockClipboardHost() *mockClipboardHost {
	return &mockClipboardHost{
		writeReturn: true,
		sendReturn:  true,
	}
}

func (m *mockClipboardHost) WritePlatformClipboard(mimeType string, payload []byte) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := make([]byte, len(payload))
	copy(p, payload)
	m.platformWrites = append(m.platformWrites, mockPlatformWrite{mimeType: mimeType, payload: p})
	return m.writeReturn
}

func (m *mockClipboardHost) SendClipboardUpdate(wireBytes []byte) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := make([]byte, len(wireBytes))
	copy(b, wireBytes)
	m.sentUpdates = append(m.sentUpdates, b)
	return m.sendReturn
}

func (m *mockClipboardHost) OnOversizedPayload(size int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.oversizedSizes = append(m.oversizedSizes, size)
}

func (m *mockClipboardHost) getPlatformWrites() []mockPlatformWrite {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([]mockPlatformWrite, len(m.platformWrites))
	copy(res, m.platformWrites)
	return res
}

func (m *mockClipboardHost) getSentUpdates() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([][]byte, len(m.sentUpdates))
	copy(res, m.sentUpdates)
	return res
}

func (m *mockClipboardHost) getOversized() []int {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([]int, len(m.oversizedSizes))
	copy(res, m.oversizedSizes)
	return res
}

func TestClipboardBridgeLifecycle(t *testing.T) {
	bridge := &ClipboardBridge{}
	host := newMockClipboardHost()

	// Call before Init should fail
	err := bridge.OnLocalCopy("text/plain", []byte("hello"), time.Now().UnixMilli())
	if err == nil {
		t.Fatal("expected error calling OnLocalCopy before Init")
	}

	err = bridge.OnRemoteBytes([]byte{1, 2, 3})
	if err == nil {
		t.Fatal("expected error calling OnRemoteBytes before Init")
	}

	// Init
	if err := bridge.Init(host); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Idempotent Init
	if err := bridge.Init(host); err != nil {
		t.Fatalf("Second Init failed: %v", err)
	}

	stats := bridge.Stats()
	if !stats.Initialized {
		t.Fatal("expected stats.Initialized to be true")
	}
	if stats.Role != "Mobile" {
		t.Fatalf("expected role 'Mobile', got %q", stats.Role)
	}

	// Stop
	bridge.Stop()
	stats = bridge.Stats()
	if stats.Initialized {
		t.Fatal("expected stats.Initialized to be false after Stop")
	}

	// Call after Stop should fail
	err = bridge.OnLocalCopy("text/plain", []byte("hello"), time.Now().UnixMilli())
	if err == nil {
		t.Fatal("expected error calling OnLocalCopy after Stop")
	}
}

func TestClipboardBridgeLocalCopyAndOutboundTransport(t *testing.T) {
	bridge := &ClipboardBridge{}
	host := newMockClipboardHost()
	if err := bridge.Init(host); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer bridge.Stop()

	payload := []byte("Hello from Android device!")
	copiedAt := time.Now().UnixMilli()
	if err := bridge.OnLocalCopy("text/plain;charset=utf-8", payload, copiedAt); err != nil {
		t.Fatalf("OnLocalCopy failed: %v", err)
	}

	sent := host.getSentUpdates()
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent update, got %d", len(sent))
	}

	var update phonebridgev1.ClipboardUpdate
	if err := proto.Unmarshal(sent[0], &update); err != nil {
		t.Fatalf("failed to unmarshal sent update: %v", err)
	}

	if update.MimeType != "text/plain;charset=utf-8" {
		t.Fatalf("unexpected MIME: %s", update.MimeType)
	}
	if string(update.Payload) != string(payload) {
		t.Fatalf("unexpected payload: %s", string(update.Payload))
	}
	if update.CopiedAtMs != uint64(copiedAt) {
		t.Fatalf("unexpected timestamp: %d != %d", update.CopiedAtMs, copiedAt)
	}

	expectedDigest := sha256.Sum256(payload)
	if hex.EncodeToString(update.Sha256Digest) != hex.EncodeToString(expectedDigest[:]) {
		t.Fatalf("digest mismatch: %x != %x", update.Sha256Digest, expectedDigest)
	}

	stats := bridge.Stats()
	if stats.LocalCopies != 1 {
		t.Fatalf("expected LocalCopies 1, got %d", stats.LocalCopies)
	}
}

func TestClipboardBridgeRemoteUpdateAndPlatformWrite(t *testing.T) {
	bridge := &ClipboardBridge{}
	host := newMockClipboardHost()
	if err := bridge.Init(host); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer bridge.Stop()

	remotePayload := []byte("Remote desktop copied text")
	remoteDigest := sha256.Sum256(remotePayload)
	update := &phonebridgev1.ClipboardUpdate{
		MimeType:     "text/plain;charset=utf-8",
		Payload:      remotePayload,
		CopiedAtMs:   uint64(time.Now().UnixMilli()),
		Sha256Digest: remoteDigest[:],
	}

	wireBytes, err := proto.Marshal(update)
	if err != nil {
		t.Fatalf("marshal update failed: %v", err)
	}

	if err := bridge.OnRemoteBytes(wireBytes); err != nil {
		t.Fatalf("OnRemoteBytes failed: %v", err)
	}

	writes := host.getPlatformWrites()
	if len(writes) != 1 {
		t.Fatalf("expected 1 platform write, got %d", len(writes))
	}
	if writes[0].mimeType != "text/plain;charset=utf-8" {
		t.Fatalf("unexpected write mime: %s", writes[0].mimeType)
	}
	if string(writes[0].payload) != string(remotePayload) {
		t.Fatalf("unexpected write payload: %s", string(writes[0].payload))
	}

	stats := bridge.Stats()
	if stats.RemoteUpdates != 1 {
		t.Fatalf("expected RemoteUpdates 1, got %d", stats.RemoteUpdates)
	}
	if stats.PlatformWrites != 1 {
		t.Fatalf("expected PlatformWrites 1, got %d", stats.PlatformWrites)
	}
	if stats.CurrentBytes != len(remotePayload) {
		t.Fatalf("expected CurrentBytes %d, got %d", len(remotePayload), stats.CurrentBytes)
	}
}

func TestClipboardBridgeEchoSuppression(t *testing.T) {
	bridge := &ClipboardBridge{}
	host := newMockClipboardHost()
	if err := bridge.Init(host); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer bridge.Stop()

	text := []byte("Echo suppression test string")
	copiedAt := time.Now().UnixMilli()

	// 1. Android receives remote update
	digest := sha256.Sum256(text)
	update := &phonebridgev1.ClipboardUpdate{
		MimeType:     "text/plain;charset=utf-8",
		Payload:      text,
		CopiedAtMs:   uint64(copiedAt),
		Sha256Digest: digest[:],
	}
	wireBytes, _ := proto.Marshal(update)
	if err := bridge.OnRemoteBytes(wireBytes); err != nil {
		t.Fatalf("OnRemoteBytes failed: %v", err)
	}

	// Platform write occurred
	writes := host.getPlatformWrites()
	if len(writes) != 1 {
		t.Fatalf("expected 1 platform write, got %d", len(writes))
	}

	// 2. Android platform clipboard listener fires for the write that was just applied
	// (Simulates the platform echo callback)
	if err := bridge.OnLocalCopy("text/plain;charset=utf-8", text, copiedAt+100); err != nil {
		t.Fatalf("OnLocalCopy returned error on echo: %v", err)
	}

	// It must NOT send an outbound clipboard update back across transport!
	sent := host.getSentUpdates()
	if len(sent) != 0 {
		t.Fatalf("expected 0 sent updates due to echo suppression, got %d", len(sent))
	}
}

func TestClipboardBridgePayloadCeilingEnforcement(t *testing.T) {
	bridge := &ClipboardBridge{}
	host := newMockClipboardHost()
	if err := bridge.Init(host); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer bridge.Stop()

	// Exactly 768 KiB is allowed
	allowedPayload := make([]byte, clipboard.MaxPayloadSize)
	for i := range allowedPayload {
		allowedPayload[i] = 'a'
	}
	if err := bridge.OnLocalCopy("text/plain", allowedPayload, time.Now().UnixMilli()); err != nil {
		t.Fatalf("expected 768 KiB to succeed, got: %v", err)
	}

	// 768 KiB + 1 byte must be rejected
	oversizedPayload := make([]byte, clipboard.MaxPayloadSize+1)
	err := bridge.OnLocalCopy("text/plain", oversizedPayload, time.Now().UnixMilli())
	if err == nil {
		t.Fatal("expected error on oversized payload (768 KiB + 1 byte)")
	}

	oversizedSizes := host.getOversized()
	if len(oversizedSizes) != 1 || oversizedSizes[0] != clipboard.MaxPayloadSize+1 {
		t.Fatalf("expected OnOversizedPayload called with %d, got: %v", clipboard.MaxPayloadSize+1, oversizedSizes)
	}

	stats := bridge.Stats()
	if stats.OversizedCount != 1 {
		t.Fatalf("expected OversizedCount 1, got %d", stats.OversizedCount)
	}

	// Verify JSON marshaling works
	jsonBytes := bridge.StatsJSON()
	var unmarshaled ClipboardStats
	if err := json.Unmarshal(jsonBytes, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal StatsJSON: %v", err)
	}
	if unmarshaled.OversizedCount != 1 {
		t.Fatalf("expected OversizedCount 1 in json, got %d", unmarshaled.OversizedCount)
	}
}

// With no WebRTC session and no registered host there is no send path; the
// bridge must report that as an error so the pending-slot flush retains the
// item for retry. A silent nil here loses the item while the sender believes
// it was delivered (observed on device: a held clip was logged as "sent to
// the peer" and never reached the desktop).
func TestClipboardBridgeLocalCopyWithoutTransportFailsLoudly(t *testing.T) {
	bridge := &ClipboardBridge{}
	if err := bridge.Init(nil); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer bridge.Stop()

	err := bridge.OnLocalCopy("text/plain;charset=utf-8", []byte("must survive"), time.Now().UnixMilli())
	if err == nil {
		t.Fatal("OnLocalCopy returned nil with no transport and no host: silent drop")
	}
}
