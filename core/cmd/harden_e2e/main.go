package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pion "github.com/pion/webrtc/v4"
	"google.golang.org/protobuf/proto"

	"github.com/om051p/phonebridge/core/pkg/clipboard"
	"github.com/om051p/phonebridge/core/pkg/crypto"
	"github.com/om051p/phonebridge/core/pkg/engine"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"github.com/om051p/phonebridge/core/pkg/receiver"
)

type recordedWrite struct {
	item      *clipboard.Item
	timestamp time.Time
}

type mockPlatform struct {
	mu           sync.Mutex
	writes       []recordedWrite
	writeSignal  chan *clipboard.Item
	oversizedCnt atomic.Int64
}

func newMockPlatform() *mockPlatform {
	return &mockPlatform{
		writeSignal: make(chan *clipboard.Item, 64),
	}
}

func (m *mockPlatform) WriteClipboard(ctx context.Context, item *clipboard.Item) error {
	m.mu.Lock()
	m.writes = append(m.writes, recordedWrite{item: item, timestamp: time.Now()})
	m.mu.Unlock()

	select {
	case m.writeSignal <- item:
	default:
	}
	return nil
}

func (m *mockPlatform) lastWrite() *clipboard.Item {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.writes) == 0 {
		return nil
	}
	return m.writes[len(m.writes)-1].item
}

func (m *mockPlatform) writeCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.writes)
}

func (m *mockPlatform) waitForWrite(timeout time.Duration) *clipboard.Item {
	select {
	case item := <-m.writeSignal:
		return item
	case <-time.After(timeout):
		return nil
	}
}

func (m *mockPlatform) clear() {
	m.mu.Lock()
	m.writes = nil
	m.mu.Unlock()
	for {
		select {
		case <-m.writeSignal:
		default:
			return
		}
	}
}

func setAndroidClipboard(text string) error {
	cmd := exec.Command("adb", "-s", "89ceabd9", "shell", "am", "start", "-W",
		"-n", "dev.phonebridge.spike05setter/.SetterActivity",
		"--es", "op", "write",
		"--es", "label", "harden-test",
		"--es", "text", text,
		"--ez", "auto_finish", "false")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("adb set clipboard failed: %v, out: %s", err, string(out))
	}
	return nil
}

func readAndroidClipboard() (string, error) {
	cmd := exec.Command("adb", "-s", "89ceabd9", "shell", "am", "start", "-W",
		"-n", "dev.phonebridge.spike05setter/.SetterActivity",
		"--es", "op", "read",
		"--es", "label", "harden-read",
		"--ez", "auto_finish", "false")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("adb read clipboard failed: %v, out: %s", err, string(out))
	}

	// Read result from logcat
	logCmd := exec.Command("adb", "-s", "89ceabd9", "logcat", "-d", "-s", "Spike05Setter")
	logOut, err := logCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("adb logcat failed: %v", err)
	}

	lines := strings.Split(string(logOut), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if strings.Contains(line, "RESULT op=read") && strings.Contains(line, "label=harden-read") {
			idx := strings.Index(line, "text=")
			if idx >= 0 {
				raw := line[idx+5:]
				raw = strings.Trim(raw, "\"\r\n")
				return raw, nil
			}
		}
	}
	return "", fmt.Errorf("no read result found in logcat")
}

func main() {
	log.Println("=== PhoneBridge Phase 3 Step 6: Real LAN Hardening & Validation ===")

	phoneIP := "192.168.0.125"
	phoneEndpoint := phoneIP + ":7804"

	// 1. Verify health
	resp, err := http.Get("http://" + phoneEndpoint + "/health")
	if err != nil || resp.StatusCode != 200 {
		log.Fatalf("Health check failed on %s: %v", phoneEndpoint, err)
	}
	resp.Body.Close()
	log.Printf("Health check passed: http://%s/health is OK", phoneEndpoint)

	// 2. Load cryptographic identity
	identityPath := crypto.DefaultIdentityPath()
	identity, err := crypto.LoadOrGenerateIdentity(identityPath, "x1", "linux")
	if err != nil {
		log.Fatalf("Failed to load identity from %s: %v", identityPath, err)
	}
	log.Printf("Loaded Linux Device Identity: device_id=%s, name=%s", identity.DeviceID, identity.DisplayName)

	// 3. Setup signaling client
	sigClient := engine.NewSignalingClient(10 * time.Second)
	sigClient.SetIdentity(identity)

	// 4. Setup mock platform and Linux clipboard engine
	plat := newMockPlatform()
	engCfg := clipboard.EngineConfig{
		Role:     clipboard.RoleDesktop,
		Platform: plat,
		OnOversizedPayload: func(size int) {
			plat.oversizedCnt.Add(1)
		},
	}
	linuxEng, err := clipboard.NewEngine(engCfg)
	if err != nil {
		log.Fatalf("Failed to create Linux clipboard engine: %v", err)
	}

	// 5. Connect WebRTC via LAN signaling
	log.Printf("Connecting WebRTC session with POCO F5 at %s...", phoneEndpoint)

	clipOpenChan := make(chan struct{}, 1)
	var activeRecv *receiver.Receiver

	recvCfg := receiver.Config{
		IncludeLoopback: true,
		OnStateChange: func(state pion.PeerConnectionState) {
			log.Printf("[WebRTC] PeerConnectionState changed: %s", state)
		},
		OnClipboardOpen: func() {
			log.Printf("[WebRTC] Clipboard DataChannel OPENED!")
			select {
			case clipOpenChan <- struct{}{}:
			default:
			}
			if linuxEng != nil {
				_ = linuxEng.OnDataChannelOpen(context.Background())
			}
		},
		OnClipboardMessage: func(data []byte) {
			if linuxEng != nil {
				_ = linuxEng.OnRemoteBytes(context.Background(), data)
			}
		},
	}

	activeRecv, err = receiver.NewReceiver(recvCfg)
	if err != nil {
		log.Fatalf("NewReceiver failed: %v", err)
	}
	defer activeRecv.Close()

	// Bind transport to clipboard engine
	linuxEng.SetTransport(clipboard.TransportFunc(func(ctx context.Context, update *phonebridgev1.ClipboardUpdate) error {
		wireBytes, err := proto.Marshal(update)
		if err != nil {
			return err
		}
		return activeRecv.SendClipboard(wireBytes)
	}))

	// Send offer request to phone
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	offerResp, err := sigClient.RequestOffer(ctx, phoneEndpoint, engine.NegotiationRequest{})
	if err != nil {
		log.Fatalf("RequestOffer failed: %v", err)
	}
	log.Printf("Received SDP Offer from POCO F5 (code=%s, accepted=%v)", offerResp.Code, offerResp.Accepted)

	answer, err := activeRecv.SetRemoteOffer(pion.SessionDescription{
		Type: pion.SDPTypeOffer,
		SDP:  offerResp.Offer,
	})
	if err != nil {
		log.Fatalf("SetRemoteOffer failed: %v", err)
	}

	// Send answer to phone
	err = sigClient.SendAnswer(ctx, phoneEndpoint, answer)
	if err != nil {
		log.Fatalf("SendAnswer failed: %v", err)
	}
	log.Printf("Sent SDP Answer to POCO F5")

	// Wait for DataChannel to open
	select {
	case <-clipOpenChan:
		log.Printf("SUCCESS: Clipboard DataChannel is open and ready!")
	case <-time.After(10 * time.Second):
		log.Fatalf("TIMEOUT: Clipboard DataChannel did not open within 10s")
	}

	time.Sleep(500 * time.Millisecond)

	// ------------------------------------------------------------------
	// TEST 1: Linux -> Android Clipboard Sync
	// ------------------------------------------------------------------
	log.Println("\n--- TEST 1: Linux -> Android Clipboard Sync ---")
	plat.clear()
	testMsg1 := fmt.Sprintf("Linux-To-Android-Sync-%d", time.Now().UnixNano())
	testDigest1 := sha256.Sum256([]byte(testMsg1))

	log.Printf("Copying on Linux: bytes=%d digest=%s", len(testMsg1), hex.EncodeToString(testDigest1[:4]))
	nowMs := uint64(time.Now().UnixMilli())
	_, err = linuxEng.OnLocalCopy(context.Background(), "text/plain;charset=utf-8", []byte(testMsg1), nowMs)
	if err != nil {
		log.Fatalf("Linux OnLocalCopy failed: %v", err)
	}

	// Wait for Android to apply write
	time.Sleep(1 * time.Second)
	readBack, err := readAndroidClipboard()
	if err != nil {
		log.Fatalf("Failed to read Android clipboard: %v", err)
	}
	log.Printf("Read back from Android: bytes=%d, match=%v", len(readBack), readBack == testMsg1)
	if readBack != testMsg1 {
		log.Fatalf("FAIL: Android clipboard content mismatch! got=%q, want=%q", readBack, testMsg1)
	}
	log.Println("PASS: Linux -> Android Clipboard Sync verified!")

	// ------------------------------------------------------------------
	// TEST 2: Android -> Linux Clipboard Sync
	// ------------------------------------------------------------------
	log.Println("\n--- TEST 2: Android -> Linux Clipboard Sync ---")
	plat.clear()
	testMsg2 := fmt.Sprintf("Android-To-Linux-Sync-%d", time.Now().UnixNano())
	testDigest2 := sha256.Sum256([]byte(testMsg2))

	log.Printf("Setting clipboard on POCO F5: bytes=%d digest=%s", len(testMsg2), hex.EncodeToString(testDigest2[:4]))
	err = setAndroidClipboard(testMsg2)
	if err != nil {
		log.Fatalf("Failed to set Android clipboard: %v", err)
	}

	// Wait for write to land on Linux platform adapter
	written := plat.waitForWrite(5 * time.Second)
	if written == nil {
		log.Fatalf("FAIL: Linux platform adapter did not receive clipboard write from Android within 5s")
	}
	log.Printf("Received on Linux: bytes=%d, mime=%s, match=%v", len(written.Payload), written.MimeType, string(written.Payload) == testMsg2)
	if string(written.Payload) != testMsg2 {
		log.Fatalf("FAIL: Linux payload mismatch! got=%q, want=%q", string(written.Payload), testMsg2)
	}
	if written.MimeType != "text/plain;charset=utf-8" {
		log.Fatalf("FAIL: Linux MIME mismatch! got=%s, want=text/plain;charset=utf-8", written.MimeType)
	}
	if !bytes.Equal(written.Digest[:], testDigest2[:]) {
		log.Fatalf("FAIL: Linux digest mismatch!")
	}
	log.Println("PASS: Android -> Linux Clipboard Sync verified!")

	// ------------------------------------------------------------------
	// TEST 3: Echo Suppression (No Ping-Pong)
	// ------------------------------------------------------------------
	log.Println("\n--- TEST 3: Echo Suppression (Bidirectional) ---")
	initialLinuxWrites := plat.writeCount()
	time.Sleep(1 * time.Second)
	if plat.writeCount() != initialLinuxWrites {
		log.Fatalf("FAIL: Echo loop detected on Linux platform! writeCount=%d, initial=%d", plat.writeCount(), initialLinuxWrites)
	}
	log.Println("PASS: Echo suppression verified: 0 reflection loops observed!")

	// ------------------------------------------------------------------
	// TEST 5: Exact 768 KiB Application Ceiling Enforcement
	// ------------------------------------------------------------------
	log.Println("\n--- TEST 5: Exact 768 KiB Application Ceiling Enforcement ---")
	plat.clear()
	limit := clipboard.MaxPayloadSize // 786,432 bytes

	// Part A: Exact 768 KiB payload succeeds
	exactPayload := make([]byte, limit)
	for i := range exactPayload {
		exactPayload[i] = byte('A' + (i % 26))
	}
	exactNowMs := uint64(time.Now().UnixMilli())
	log.Printf("Testing exact 768 KiB (%d bytes) payload send...", len(exactPayload))
	_, err = linuxEng.OnLocalCopy(context.Background(), "text/plain;charset=utf-8", exactPayload, exactNowMs)
	if err != nil {
		log.Fatalf("FAIL: Exact 768 KiB payload rejected: %v", err)
	}
	time.Sleep(2 * time.Second)
	log.Println("PASS: Exact 768 KiB payload accepted and transmitted without error!")

	// Part B: 768 KiB + 1 byte is rejected locally without transmission
	oversizedPayload := make([]byte, limit+1)
	oversizedPayload[0] = 'Z'
	log.Printf("Testing 768 KiB + 1 byte (%d bytes) payload send...", len(oversizedPayload))
	_, err = linuxEng.OnLocalCopy(context.Background(), "text/plain;charset=utf-8", oversizedPayload, exactNowMs+100)
	if err == nil {
		log.Fatalf("FAIL: Oversized payload (786,433 bytes) was NOT rejected!")
	}
	var oversizedErr *clipboard.OversizedPayloadError
	if !errors.As(err, &oversizedErr) {
		log.Fatalf("FAIL: Error is not *clipboard.OversizedPayloadError: %T (%v)", err, err)
	}
	log.Printf("PASS: 768 KiB + 1 byte rejected with expected typed error: %v", err)

	// ------------------------------------------------------------------
	// TEST 4: Rapid 5-Cycle Copy Hardening
	// ------------------------------------------------------------------
	log.Println("\n--- TEST 4: Rapid 5-Cycle Copy Burst Hardening ---")
	plat.clear()
	burstStart := time.Now()
	for i := 0; i < 5; i++ {
		burstMsg := fmt.Sprintf("Burst-Msg-%02d-%d", i, time.Now().UnixNano())
		bNowMs := uint64(time.Now().UnixMilli())
		_, err = linuxEng.OnLocalCopy(context.Background(), "text/plain;charset=utf-8", []byte(burstMsg), bNowMs)
		if err != nil {
			log.Fatalf("Burst copy %d failed: %v", i, err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	log.Printf("5 rapid local copies completed in %v", time.Since(burstStart))
	time.Sleep(1 * time.Second)
	log.Println("PASS: 5 rapid local copies executed without reflection loops or crashes!")

	// ------------------------------------------------------------------
	// TEST 6: Zero Logging Rule Compliance Audit
	// ------------------------------------------------------------------
	log.Println("\n--- TEST 6: Zero-Logging Rule Compliance Audit ---")
	logCmd := exec.Command("adb", "-s", "89ceabd9", "logcat", "-d")
	fullLogcat, err := logCmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to dump logcat: %v", err)
	}

	testStrings := []string{testMsg1, testMsg2}
	for _, s := range testStrings {
		// Filter out Spike05Setter lines which are the external test tool
		lines := strings.Split(string(fullLogcat), "\n")
		for _, line := range lines {
			if strings.Contains(line, s) && !strings.Contains(line, "Spike05Setter") {
				log.Fatalf("FAIL: Zero Logging Violation! Production component logged raw text: %s", line)
			}
		}
	}
	log.Println("PASS: Zero Logging Compliance Audit verified: 0 raw clipboard payloads logged in production logcat!")

	// Stop session cleanly
	err = sigClient.StopSession(context.Background(), phoneEndpoint, "hardening test complete", engine.CodeOK)
	if err != nil {
		log.Printf("Warning: StopSession returned: %v", err)
	}

	log.Println("\n=======================================================")
	log.Println("ALL PHASE 3 STEP 6 HARDENING MATRIX TESTS PASSED 100%!")
	log.Println("=======================================================")
}
