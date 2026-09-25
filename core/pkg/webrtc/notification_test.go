package webrtc

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"github.com/om051p/phonebridge/core/pkg/receiver"
	pion "github.com/pion/webrtc/v4"
	"google.golang.org/protobuf/proto"
)

func TestLoopbackNotificationDataChannel(t *testing.T) {
	var (
		recvRecvMsg []byte
		recvMu      sync.Mutex
		recvOpened  = make(chan struct{})
		sessOpened  = make(chan struct{})
	)

	recvCfg := receiver.Config{
		IncludeLoopback: true,
		Sink:            receiver.NewNullSink(),
		OnNotificationOpen: func() {
			close(recvOpened)
		},
		OnNotificationMessage: func(data []byte) {
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
		OnNotificationOpen: func() {
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

	// Wait for connected state and DataChannels to open.
	if err := sess.WaitForState(pion.PeerConnectionStateConnected, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case <-recvOpened:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for receiver notification datachannel to open")
	}
	select {
	case <-sessOpened:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for session notification datachannel to open")
	}

	// Send NotificationFrame from session to receiver.
	frame := &phonebridgev1.NotificationFrame{
		Version:     1,
		TimestampMs: uint64(time.Now().UnixMilli()),
		Event: &phonebridgev1.NotificationFrame_Posted{
			Posted: &phonebridgev1.NotificationPosted{
				Key:         "0|org.telegram.messenger|42|tag|1000",
				PackageName: "org.telegram.messenger",
				AppName:     "Telegram",
				Title:       "Alice",
				Text:        "Hey there!",
				PostTimeMs:  time.Now().UnixMilli(),
				IsClearable: true,
			},
		},
	}
	data, err := proto.Marshal(frame)
	if err != nil {
		t.Fatal(err)
	}

	if err := sess.SendNotification(data); err != nil {
		t.Fatalf("sess.SendNotification failed: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		recvMu.Lock()
		got := recvRecvMsg
		recvMu.Unlock()
		if len(got) > 0 {
			if !bytes.Equal(got, data) {
				t.Fatalf("received data mismatch: got %v, want %v", got, data)
			}
			var decoded phonebridgev1.NotificationFrame
			if err := proto.Unmarshal(got, &decoded); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			posted := decoded.GetPosted()
			if posted == nil || posted.Title != "Alice" || posted.Key != "0|org.telegram.messenger|42|tag|1000" {
				t.Fatalf("decoded content mismatch: %+v", posted)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timeout waiting for notification data on receiver")
}
