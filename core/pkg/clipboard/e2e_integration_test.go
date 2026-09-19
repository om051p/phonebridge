package clipboard_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pion "github.com/pion/webrtc/v4"
	"google.golang.org/protobuf/proto"

	"github.com/om051p/phonebridge/core/pkg/clipboard"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"github.com/om051p/phonebridge/core/pkg/receiver"
	"github.com/om051p/phonebridge/core/pkg/webrtc"
)

// mockPlatform tracks platform writes and allows simulating platform echo events.
type mockPlatform struct {
	mu           sync.Mutex
	writes       []*clipboard.Item
	writeSignal  chan *clipboard.Item
	oversizedCnt atomic.Int64
}

func newMockPlatform() *mockPlatform {
	return &mockPlatform{
		writeSignal: make(chan *clipboard.Item, 16),
	}
}

func (m *mockPlatform) WriteClipboard(ctx context.Context, item *clipboard.Item) error {
	m.mu.Lock()
	m.writes = append(m.writes, item)
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
	return m.writes[len(m.writes)-1]
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

// loopbackPair encapsulates a connected WebRTC session and receiver pair with clipboard engines.
type loopbackPair struct {
	sess         *webrtc.Session
	recv         *receiver.Receiver
	androidEng   *clipboard.Engine
	androidPlat  *mockPlatform
	androidOpens atomic.Int32
	linuxEng     *clipboard.Engine
	linuxPlat    *mockPlatform
	linuxOpens   atomic.Int32
}

func setupLoopbackPair(t *testing.T, androidEngCfg *clipboard.EngineConfig, linuxEngCfg *clipboard.EngineConfig) *loopbackPair {
	t.Helper()

	pair := &loopbackPair{
		androidPlat: newMockPlatform(),
		linuxPlat:   newMockPlatform(),
	}

	androidOpenChan := make(chan struct{}, 1)
	linuxOpenChan := make(chan struct{}, 1)

	// Configure sender session (Android / Offerer)
	sender := webrtc.NewSender(nil, webrtc.SenderConfig{
		PSIReinject: true,
		ShaperKbps:  0,
	})

	sessCfg := webrtc.SessionConfig{
		IncludeLoopback: true,
		OnClipboardOpen: func() {
			pair.androidOpens.Add(1)
			select {
			case androidOpenChan <- struct{}{}:
			default:
			}
			if pair.androidEng != nil {
				_ = pair.androidEng.OnDataChannelOpen(context.Background())
			}
		},
		OnClipboardMessage: func(data []byte) {
			if pair.androidEng != nil {
				_ = pair.androidEng.OnRemoteBytes(context.Background(), data)
			}
		},
	}

	sess, err := webrtc.NewSession(sessCfg, sender)
	if err != nil {
		t.Fatalf("webrtc.NewSession: %v", err)
	}
	pair.sess = sess

	// Configure receiver (Linux / Answerer)
	recvCfg := receiver.Config{
		IncludeLoopback: true,
		OnClipboardOpen: func() {
			pair.linuxOpens.Add(1)
			select {
			case linuxOpenChan <- struct{}{}:
			default:
			}
			if pair.linuxEng != nil {
				_ = pair.linuxEng.OnDataChannelOpen(context.Background())
			}
		},
		OnClipboardMessage: func(data []byte) {
			if pair.linuxEng != nil {
				_ = pair.linuxEng.OnRemoteBytes(context.Background(), data)
			}
		},
	}

	recv, err := receiver.NewReceiver(recvCfg)
	if err != nil {
		sess.Stop()
		t.Fatalf("receiver.NewReceiver: %v", err)
	}
	pair.recv = recv

	// Build Android Clipboard Engine (RoleMobile)
	aCfg := clipboard.EngineConfig{
		Role:     clipboard.RoleMobile,
		Platform: pair.androidPlat,
		Transport: clipboard.TransportFunc(func(ctx context.Context, update *phonebridgev1.ClipboardUpdate) error {
			wire, err := proto.Marshal(update)
			if err != nil {
				return err
			}
			return sess.SendClipboard(wire)
		}),
		OnOversizedPayload: func(size int) {
			pair.androidPlat.oversizedCnt.Add(1)
		},
	}
	if androidEngCfg != nil {
		if androidEngCfg.Clock != nil {
			aCfg.Clock = androidEngCfg.Clock
		}
		if androidEngCfg.SuppressionTTL > 0 {
			aCfg.SuppressionTTL = androidEngCfg.SuppressionTTL
		}
	}
	androidEng, err := clipboard.NewEngine(aCfg)
	if err != nil {
		sess.Stop()
		_ = recv.Close()
		t.Fatalf("NewEngine(android): %v", err)
	}
	pair.androidEng = androidEng

	// Build Linux Clipboard Engine (RoleDesktop)
	lCfg := clipboard.EngineConfig{
		Role:     clipboard.RoleDesktop,
		Platform: pair.linuxPlat,
		Transport: clipboard.TransportFunc(func(ctx context.Context, update *phonebridgev1.ClipboardUpdate) error {
			wire, err := proto.Marshal(update)
			if err != nil {
				return err
			}
			return recv.SendClipboard(wire)
		}),
		OnOversizedPayload: func(size int) {
			pair.linuxPlat.oversizedCnt.Add(1)
		},
	}
	if linuxEngCfg != nil {
		if linuxEngCfg.Clock != nil {
			lCfg.Clock = linuxEngCfg.Clock
		}
		if linuxEngCfg.SuppressionTTL > 0 {
			lCfg.SuppressionTTL = linuxEngCfg.SuppressionTTL
		}
	}
	linuxEng, err := clipboard.NewEngine(lCfg)
	if err != nil {
		sess.Stop()
		_ = recv.Close()
		t.Fatalf("NewEngine(linux): %v", err)
	}
	pair.linuxEng = linuxEng

	// Perform WebRTC SDP Offer/Answer handshake
	offer, err := sess.CreateOffer()
	if err != nil {
		sess.Stop()
		_ = recv.Close()
		t.Fatalf("CreateOffer: %v", err)
	}

	answer, err := recv.SetRemoteOffer(offer)
	if err != nil {
		sess.Stop()
		_ = recv.Close()
		t.Fatalf("SetRemoteOffer: %v", err)
	}

	if err := sess.SetRemoteAnswer(answer); err != nil {
		sess.Stop()
		_ = recv.Close()
		t.Fatalf("SetRemoteAnswer: %v", err)
	}

	// Wait for WebRTC connection
	if err := recv.WaitForState(pion.PeerConnectionStateConnected, 5*time.Second); err != nil {
		sess.Stop()
		_ = recv.Close()
		t.Fatalf("WaitForState: %v", err)
	}

	// Wait for "clipboard" DataChannels to open on both ends
	select {
	case <-androidOpenChan:
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout waiting for Android clipboard datachannel open")
	}

	select {
	case <-linuxOpenChan:
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout waiting for Linux clipboard datachannel open")
	}

	return pair
}

func (p *loopbackPair) close() {
	if p.sess != nil {
		p.sess.Stop()
	}
	if p.recv != nil {
		_ = p.recv.Close()
	}
}

// ------------------------------------------------------------------
// E2E Test 1: Android -> Linux Sync over WebRTC DataChannel
// ------------------------------------------------------------------

func TestE2E_AndroidToLinuxSync(t *testing.T) {
	pair := setupLoopbackPair(t, nil, nil)
	defer pair.close()

	ctx := context.Background()
	text := "Hello COSMIC from Android POCO F5!"
	payload := []byte(text)
	nowMs := uint64(time.Now().UnixMilli())

	item, err := pair.androidEng.OnLocalCopy(ctx, "text/plain;charset=utf-8", payload, nowMs)
	if err != nil {
		t.Fatalf("OnLocalCopy failed: %v", err)
	}

	// Wait for Linux platform adapter to receive write
	written := pair.linuxPlat.waitForWrite(3 * time.Second)
	if written == nil {
		t.Fatalf("Linux platform adapter did not receive clipboard write within 3s")
	}

	if !bytes.Equal(written.Payload, payload) {
		t.Fatalf("Payload mismatch: got %q, want %q", string(written.Payload), text)
	}
	if written.MimeType != "text/plain;charset=utf-8" {
		t.Fatalf("MIME mismatch: got %q, want %q", written.MimeType, "text/plain;charset=utf-8")
	}
	if written.Digest != item.Digest {
		t.Fatalf("Digest mismatch: got %x, want %x", written.Digest, item.Digest)
	}
}

// ------------------------------------------------------------------
// E2E Test 2: Linux -> Android Sync over WebRTC DataChannel
// ------------------------------------------------------------------

func TestE2E_LinuxToAndroidSync(t *testing.T) {
	pair := setupLoopbackPair(t, nil, nil)
	defer pair.close()

	ctx := context.Background()
	text := "Hello Android from Linux COSMIC desktop!"
	payload := []byte(text)
	nowMs := uint64(time.Now().UnixMilli())

	item, err := pair.linuxEng.OnLocalCopy(ctx, "text/plain;charset=utf-8", payload, nowMs)
	if err != nil {
		t.Fatalf("OnLocalCopy failed: %v", err)
	}

	// Wait for Android platform adapter to receive write
	written := pair.androidPlat.waitForWrite(3 * time.Second)
	if written == nil {
		t.Fatalf("Android platform adapter did not receive clipboard write within 3s")
	}

	if !bytes.Equal(written.Payload, payload) {
		t.Fatalf("Payload mismatch: got %q, want %q", string(written.Payload), text)
	}
	if written.MimeType != "text/plain;charset=utf-8" {
		t.Fatalf("MIME mismatch: got %q, want %q", written.MimeType, "text/plain;charset=utf-8")
	}
	if written.Digest != item.Digest {
		t.Fatalf("Digest mismatch: got %x, want %x", written.Digest, item.Digest)
	}
}

// ------------------------------------------------------------------
// E2E Test 3: Echo Suppression across WebRTC DataChannel
// ------------------------------------------------------------------

func TestE2E_EchoSuppressionBidirectional(t *testing.T) {
	pair := setupLoopbackPair(t, nil, nil)
	defer pair.close()

	ctx := context.Background()

	// 1. Android copies text -> transferred to Linux
	text := "Echo suppression test across WebRTC DataChannel"
	payload := []byte(text)
	nowMs := uint64(time.Now().UnixMilli())

	_, err := pair.androidEng.OnLocalCopy(ctx, "text/plain;charset=utf-8", payload, nowMs)
	if err != nil {
		t.Fatalf("OnLocalCopy failed: %v", err)
	}

	written := pair.linuxPlat.waitForWrite(3 * time.Second)
	if written == nil {
		t.Fatalf("Linux platform adapter did not receive initial write")
	}

	// 2. Simulate Linux platform listener echoing the write back into Linux engine
	initialAndroidWrites := pair.androidPlat.writeCount()
	_, err = pair.linuxEng.OnLocalCopy(ctx, "text/plain;charset=utf-8", payload, nowMs+50)
	if err != nil {
		t.Fatalf("Simulated Linux echo OnLocalCopy failed: %v", err)
	}

	// Ensure no new write lands on Android
	time.Sleep(300 * time.Millisecond)
	if pair.androidPlat.writeCount() != initialAndroidWrites {
		t.Fatalf("Echo suppression failed: Android received reflected write (count=%d, want=%d)",
			pair.androidPlat.writeCount(), initialAndroidWrites)
	}

	// 3. Now reverse direction: Linux copies new text -> transferred to Android
	revText = "Reverse echo suppression verification"
	revPayload = []byte(revText)
	revNowMs = nowMs + 1000

	_, err = pair.linuxEng.OnLocalCopy(ctx, "text/plain;charset=utf-8", revPayload, revNowMs)
	if err != nil {
		t.Fatalf("Linux OnLocalCopy failed: %v", err)
	}

	revWritten := pair.androidPlat.waitForWrite(3 * time.Second)
	if revWritten == nil {
		t.Fatalf("Android platform adapter did not receive reverse write")
	}

	// 4. Simulate Android platform listener echoing the write back into Android engine
	initialLinuxWrites := pair.linuxPlat.writeCount()
	_, err = pair.androidEng.OnLocalCopy(ctx, "text/plain;charset=utf-8", revPayload, revNowMs+50)
	if err != nil {
		t.Fatalf("Simulated Android echo OnLocalCopy failed: %v", err)
	}

	// Ensure no new write lands on Linux
	time.Sleep(300 * time.Millisecond)
	if pair.linuxPlat.writeCount() != initialLinuxWrites {
		t.Fatalf("Echo suppression failed: Linux received reflected write (count=%d, want=%d)",
			pair.linuxPlat.writeCount(), initialLinuxWrites)
	}
}

var revText string
var revPayload []byte
var revNowMs uint64

// ------------------------------------------------------------------
// E2E Test 4: Exact 768 KiB Application Ceiling Enforcement
// ------------------------------------------------------------------

func TestE2E_PayloadCeilingEnforcement(t *testing.T) {
	pair := setupLoopbackPair(t, nil, nil)
	defer pair.close()

	ctx := context.Background()
	limit := clipboard.MaxPayloadSize // 786,432 bytes

	// Part A: Exactly 768 KiB payload succeeds end-to-end
	exactPayload := make([]byte, limit)
	for i := range exactPayload {
		exactPayload[i] = byte('A' + (i % 26))
	}
	nowMs := uint64(time.Now().UnixMilli())

	_, err := pair.androidEng.OnLocalCopy(ctx, "text/plain", exactPayload, nowMs)
	if err != nil {
		t.Fatalf("768 KiB exact copy rejected unexpectedly: %v", err)
	}

	written := pair.linuxPlat.waitForWrite(5 * time.Second)
	if written == nil {
		t.Fatalf("Linux platform did not receive 768 KiB payload")
	}
	if len(written.Payload) != limit {
		t.Fatalf("Received payload length mismatch: got %d, want %d", len(written.Payload), limit)
	}
	if !bytes.Equal(written.Payload, exactPayload) {
		t.Fatalf("768 KiB payload corrupted in transit")
	}

	// Part B: 768 KiB + 1 byte is rejected locally without network transmission
	oversizedPayload := make([]byte, limit+1)
	oversizedPayload[0] = 'Z'

	linuxWriteCountBefore := pair.linuxPlat.writeCount()
	_, err = pair.androidEng.OnLocalCopy(ctx, "text/plain", oversizedPayload, nowMs+100)
	if err == nil {
		t.Fatalf("Expected OversizedPayloadError for 768 KiB + 1 byte, got nil")
	}

	var oversizedErr *clipboard.OversizedPayloadError
	if !errorsAsOversized(err, &oversizedErr) {
		t.Fatalf("Expected *clipboard.OversizedPayloadError, got: %T (%v)", err, err)
	}
	if pair.androidPlat.oversizedCnt.Load() == 0 {
		t.Fatalf("OnOversizedPayload callback was not triggered on Android")
	}

	// Verify nothing was transmitted across WebRTC to Linux
	time.Sleep(300 * time.Millisecond)
	if pair.linuxPlat.writeCount() != linuxWriteCountBefore {
		t.Fatalf("Oversized payload leaked across WebRTC to Linux platform")
	}
}

func errorsAsOversized(err error, target **clipboard.OversizedPayloadError) bool {
	if oe, ok := err.(*clipboard.OversizedPayloadError); ok {
		*target = oe
		return true
	}
	return false
}

// ------------------------------------------------------------------
// E2E Test 5: Reconnect Conflict Arbitration (DEC-023)
// ------------------------------------------------------------------

func TestE2E_ReconnectArbitrationScenarios(t *testing.T) {
	ctx := context.Background()

	// Scenario 1: Identical clips on both peers -> WinnerNone (zero writes)
	t.Run("IdenticalClips_WinnerNone", func(t *testing.T) {
		pair := setupLoopbackPair(t, nil, nil)
		defer pair.close()

		identicalPayload := []byte("Synchronized clip state")
		digest := sha256.Sum256(identicalPayload)
		nowMs := uint64(time.Now().UnixMilli())

		update := &phonebridgev1.ClipboardUpdate{
			MimeType:     "text/plain;charset=utf-8",
			Payload:      identicalPayload,
			Sha256Digest: digest[:],
			CopiedAtMs:   nowMs,
		}

		// Pre-populate both engines with identical item
		_, _ = pair.androidEng.OnLocalCopy(ctx, "text/plain;charset=utf-8", identicalPayload, nowMs)
		_ = pair.linuxPlat.waitForWrite(2 * time.Second)

		// Reconnect sync with identical remote
		winner, err := pair.androidEng.OnReconnectSync(ctx, update)
		if err != nil {
			t.Fatalf("OnReconnectSync failed: %v", err)
		}
		if winner != clipboard.WinnerNone {
			t.Fatalf("Expected WinnerNone for identical digest, got: %v", winner)
		}
	})

	// Scenario 2: Timestamp difference > 1000 ms -> strictly newer timestamp wins
	t.Run("TimestampDiff_NewerWins", func(t *testing.T) {
		pair := setupLoopbackPair(t, nil, nil)
		defer pair.close()

		oldPayload := []byte("Old local clip")
		newPayload := []byte("New remote clip from Android")
		newDigest := sha256.Sum256(newPayload)

		baseMs := uint64(100000)
		newerMs := baseMs + 5000 // 5 seconds newer (> 1000ms threshold)

		// Linux has old clip
		_, _ = pair.linuxEng.OnLocalCopy(ctx, "text/plain", oldPayload, baseMs)
		time.Sleep(50 * time.Millisecond)

		// Linux receives newer remote update from Android during reconnect
		remoteUpdate := &phonebridgev1.ClipboardUpdate{
			MimeType:     "text/plain;charset=utf-8",
			Payload:      newPayload,
			Sha256Digest: newDigest[:],
			CopiedAtMs:   newerMs,
		}

		winner, err := pair.linuxEng.OnReconnectSync(ctx, remoteUpdate)
		if err != nil {
			t.Fatalf("OnReconnectSync failed: %v", err)
		}
		if winner != clipboard.WinnerRemote {
			t.Fatalf("Expected WinnerRemote when remote is >1s newer, got: %v", winner)
		}

		written := pair.linuxPlat.lastWrite()
		if written == nil || !bytes.Equal(written.Payload, newPayload) {
			t.Fatalf("Linux platform write not applied for newer remote winner")
		}
	})

	// Scenario 3: Timestamp difference <= 1000 ms -> Desktop wins tie-break
	t.Run("TieBreak_DesktopWins", func(t *testing.T) {
		pair := setupLoopbackPair(t, nil, nil)
		defer pair.close()

		androidPayload := []byte("Android clip during near-simultaneous copy")
		androidDigest := sha256.Sum256(androidPayload)
		linuxPayload := []byte("Linux desktop clip during near-simultaneous copy")
		linuxDigest := sha256.Sum256(linuxPayload)

		baseMs := uint64(100000)
		androidMs := baseMs + 100 // 100ms apart (<= 1000ms threshold)
		linuxMs := baseMs

		// 1. Evaluate from Mobile (Android) perspective: Desktop (remote) must win
		_, _ = pair.androidEng.OnLocalCopy(ctx, "text/plain", androidPayload, androidMs)
		time.Sleep(50 * time.Millisecond)

		linuxRemoteUpdate := &phonebridgev1.ClipboardUpdate{
			MimeType:     "text/plain;charset=utf-8",
			Payload:      linuxPayload,
			Sha256Digest: linuxDigest[:],
			CopiedAtMs:   linuxMs,
		}

		aWinner, err := pair.androidEng.OnReconnectSync(ctx, linuxRemoteUpdate)
		if err != nil {
			t.Fatalf("Android OnReconnectSync failed: %v", err)
		}
		if aWinner != clipboard.WinnerRemote {
			t.Fatalf("Expected WinnerRemote on Mobile when within 1000ms tie-break, got: %v", aWinner)
		}

		// 2. Evaluate from Desktop (Linux) perspective: Desktop (local) must win
		_, _ = pair.linuxEng.OnLocalCopy(ctx, "text/plain", linuxPayload, linuxMs)
		time.Sleep(50 * time.Millisecond)

		androidRemoteUpdate := &phonebridgev1.ClipboardUpdate{
			MimeType:     "text/plain;charset=utf-8",
			Payload:      androidPayload,
			Sha256Digest: androidDigest[:],
			CopiedAtMs:   androidMs,
		}

		lWinner, err := pair.linuxEng.OnReconnectSync(ctx, androidRemoteUpdate)
		if err != nil {
			t.Fatalf("Linux OnReconnectSync failed: %v", err)
		}
		if lWinner != clipboard.WinnerLocal {
			t.Fatalf("Expected WinnerLocal on Desktop when within 1000ms tie-break, got: %v", lWinner)
		}
	})
}
