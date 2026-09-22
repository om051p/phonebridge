package webrtc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	pion "github.com/pion/webrtc/v4"

	"github.com/om051p/phonebridge/core/pkg/receiver"
	"github.com/om051p/phonebridge/core/pkg/transfer"
)

// TestLoopbackTransferDataChannel drives a real file over the real "transfer"
// DataChannel between a driver Session and a Linux Receiver, with a real
// transfer.Engine on each end and the rtcchannel adapter in between. It is the
// integration proof that the adapter's backpressure contract matches what Pion
// actually does (BufferedAmount rises, OnBufferedAmountLow fires, ordered
// delivery holds), which no in-memory fake channel can establish.
func TestLoopbackTransferDataChannel(t *testing.T) {
	const (
		fileSize = 320 * 1024
		chunk    = 16 * 1024
	)

	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "loopback-payload.bin")
	payload := make([]byte, fileSize)
	rng := rand.New(rand.NewSource(7))
	for i := range payload {
		payload[i] = byte(rng.Intn(256))
	}
	if err := os.WriteFile(srcPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	wantDigest := sha256.Sum256(payload)

	// Receiver side (Linux): destination in a temp dir, its own engine.
	dstDir := t.TempDir()
	dest, err := transfer.NewFileDestination(transfer.FileDestinationConfig{Dir: dstDir})
	if err != nil {
		t.Fatal(err)
	}
	recvEngine, err := transfer.NewEngine(transfer.Config{
		LocalPeerID: "linux-receiver",
		Destination: dest,
		ChunkSize:   chunk,
		// Deliberately small watermarks so a 320 KiB file forces several drain
		// waits: the test fails if backpressure never engages.
		HighWatermark: 64 * 1024,
		LowWatermark:  16 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer recvEngine.Close()

	// The open handler needs the receiver it belongs to, so declare it first:
	// the callback cannot fire before NewReceiver returns (it needs a connected
	// transport), and Pion invokes it on the SCTP goroutine afterwards.
	var recv *receiver.Receiver
	recvCfg := receiver.Config{
		IncludeLoopback: true,
		Sink:            receiver.NewNullSink(),
		OnTransferMessage: func(data []byte) {
			recvEngine.OnFrame(data)
		},
		OnTransferOpen: func() {
			recvEngine.AttachChannel(recv.TransferChannel())
		},
	}
	recv, err = receiver.NewReceiver(recvCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer recv.Close()

	senderEngine, err := transfer.NewEngine(transfer.Config{
		LocalPeerID:   "linux-sender",
		ChunkSize:     chunk,
		HighWatermark: 64 * 1024,
		LowWatermark:  16 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer senderEngine.Close()

	var sess *Session
	sessCfg := SessionConfig{
		IncludeLoopback: true,
		OnTransferMessage: func(data []byte) {
			senderEngine.OnFrame(data)
		},
		OnTransferOpen: func() {
			senderEngine.AttachChannel(sess.TransferChannel())
		},
	}
	sess, err = NewSession(sessCfg, NewSender(nil, SenderConfig{}))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Stop()

	// WebRTC handshake: driver offers, receiver answers.
	offer, err := sess.CreateOffer()
	if err != nil {
		t.Fatal(err)
	}
	answer, err := recv.SetRemoteOffer(offer)
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.SetRemoteAnswer(answer); err != nil {
		t.Fatal(err)
	}
	if err := sess.WaitForState(pion.PeerConnectionStateConnected, 20*time.Second); err != nil {
		t.Fatal(err)
	}

	// The transfer channel must open (and be attached) on both ends before
	// anything is sent, because the engine refuses to send without a channel.
	if !waitCond(func() bool { return senderEngine.ChannelReady() }, 20*time.Second) {
		t.Fatal("sender transfer channel never became ready")
	}
	if !waitCond(func() bool { return recvEngine.ChannelReady() }, 20*time.Second) {
		t.Fatal("receiver transfer channel never became ready")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	id, err := senderEngine.SendFile(ctx, srcPath, "loopback-payload.bin")
	if err != nil {
		t.Fatalf("send file: %v", err)
	}

	info := waitTransferFinal(t, senderEngine, id, 60*time.Second)
	if info.State != transfer.StateComplete {
		t.Fatalf("sender state = %s (%s: %s)", info.State, info.ReasonCode, info.ErrorMessage)
	}
	if info.BytesTransferred != uint64(fileSize) {
		t.Fatalf("sender progress = %d, want %d", info.BytesTransferred, fileSize)
	}

	// The file must exist at the destination with the exact expected bytes: the
	// engine's own digest check is not enough to prove the bytes on disk.
	saved := filepath.Join(dstDir, info.SavedName)
	got, err := os.ReadFile(saved)
	if err != nil {
		t.Fatalf("read received file: %v", err)
	}
	if len(got) != fileSize {
		t.Fatalf("received %d bytes, want %d", len(got), fileSize)
	}
	if sha256.Sum256(got) != wantDigest {
		t.Fatal("received file digest mismatch")
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("received file content differs from source")
	}
	// The staged partial directory must be empty: nothing was left behind.
	entries, err := os.ReadDir(dest.PartialDir())
	if err != nil {
		t.Fatalf("read partial dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("partial dir still holds %d entries after commit", len(entries))
	}
	// And exactly one file landed in the destination.
	files, err := os.ReadDir(dstDir)
	if err != nil {
		t.Fatal(err)
	}
	committed := 0
	for _, f := range files {
		if !f.IsDir() {
			committed++
		}
	}
	if committed != 1 {
		t.Fatalf("destination holds %d files, want 1", committed)
	}
}

// waitTransferFinal polls the engine until the transfer reaches a final state,
// then returns its snapshot.
func waitTransferFinal(t *testing.T, eng *transfer.Engine, id string, timeout time.Duration) transfer.Info {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last transfer.Info
	for time.Now().Before(deadline) {
		if info, ok := eng.Get(id); ok {
			last = info
			switch info.State {
			case transfer.StateComplete, transfer.StateFailed, transfer.StateCancelled:
				return info
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("transfer %s did not finish (last state %s)", id, last.State)
	return transfer.Info{}
}
