package webrtc

import (
	"sync"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"github.com/om051p/phonebridge/core/pkg/receiver"
	pion "github.com/pion/webrtc/v4"
	"google.golang.org/protobuf/proto"
)

func TestLoopbackInputDataChannel(t *testing.T) {
	var (
		recvRecvMsg []byte
		recvMu      sync.Mutex
		recvOpened  = make(chan struct{})
		sessOpened  = make(chan struct{})
	)

	recvCfg := receiver.Config{
		IncludeLoopback: true,
		Sink:            receiver.NewNullSink(),
		OnInputOpen: func() {
			close(recvOpened)
		},
		OnInputMessage: func(data []byte) {
			recvMu.Lock()
			recvRecvMsg = append([]byte(nil), data...)
			recvMu.Unlock()
		},
	}
	recv, err := receiver.NewReceiver(recvCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer recv.Close()

	sessCfg := SessionConfig{
		IncludeLoopback: true,
		OnInputOpen: func() {
			close(sessOpened)
		},
	}
	sess, err := NewSession(sessCfg, NewSender(nil, SenderConfig{}))
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

	select {
	case <-sessOpened:
	case <-time.After(5 * time.Second):
		t.Fatal("session input channel did not open")
	}

	select {
	case <-recvOpened:
	case <-time.After(5 * time.Second):
		t.Fatal("receiver input channel did not open")
	}

	// 1. Send from Session to Receiver
	frame := &phonebridgev1.InputFrame{
		TimestampMs: uint64(time.Now().UnixMilli()),
		Event: &phonebridgev1.InputFrame_Touch{
			Touch: &phonebridgev1.TouchEvent{
				Action:      phonebridgev1.TouchEvent_ACTION_DOWN,
				NormalizedX: 0.25,
				NormalizedY: 0.75,
				Pressure:    1.0,
			},
		},
	}
	data, err := proto.Marshal(frame)
	if err != nil {
		t.Fatal(err)
	}

	if err := sess.SendInput(data); err != nil {
		t.Fatalf("sess.SendInput failed: %v", err)
	}

	// Wait for message receipt on Receiver
	deadline := time.Now().Add(5 * time.Second)
	var gotMsg []byte
	for time.Now().Before(deadline) {
		recvMu.Lock()
		if len(recvRecvMsg) > 0 {
			gotMsg = append([]byte(nil), recvRecvMsg...)
			recvMu.Unlock()
			break
		}
		recvMu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}

	if len(gotMsg) == 0 {
		t.Fatal("receiver never received input frame")
	}

	var parsed phonebridgev1.InputFrame
	if err := proto.Unmarshal(gotMsg, &parsed); err != nil {
		t.Fatalf("failed to unmarshal input frame: %v", err)
	}
	touch := parsed.GetTouch()
	if touch == nil || touch.NormalizedX != 0.25 || touch.NormalizedY != 0.75 {
		t.Fatalf("unexpected touch event: %v", touch)
	}

	// 2. Send from Receiver to Session
	var sessRecvMsg []byte
	var sessMu sync.Mutex
	sess.inputDC.OnMessage(func(msg pion.DataChannelMessage) {
		sessMu.Lock()
		sessRecvMsg = append([]byte(nil), msg.Data...)
		sessMu.Unlock()
	})

	textFrame := &phonebridgev1.InputFrame{
		TimestampMs: uint64(time.Now().UnixMilli()),
		Event: &phonebridgev1.InputFrame_Text{
			Text: &phonebridgev1.TextEvent{
				Text: "Hello from receiver",
			},
		},
	}
	textData, err := proto.Marshal(textFrame)
	if err != nil {
		t.Fatal(err)
	}

	if err := recv.SendInput(textData); err != nil {
		t.Fatalf("recv.SendInput failed: %v", err)
	}

	deadline = time.Now().Add(5 * time.Second)
	gotMsg = nil
	for time.Now().Before(deadline) {
		sessMu.Lock()
		if len(sessRecvMsg) > 0 {
			gotMsg = append([]byte(nil), sessRecvMsg...)
			sessMu.Unlock()
			break
		}
		sessMu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}

	if len(gotMsg) == 0 {
		t.Fatal("session never received input frame from receiver")
	}

	var parsedText phonebridgev1.InputFrame
	if err := proto.Unmarshal(gotMsg, &parsedText); err != nil {
		t.Fatalf("failed to unmarshal text frame: %v", err)
	}
	if parsedText.GetText() == nil || parsedText.GetText().Text != "Hello from receiver" {
		t.Fatalf("unexpected text event: %v", parsedText.GetText())
	}
}
