package localipc

import (
	"context"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgelocalipcv1"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNotification_BroadcastAndList(t *testing.T) {
	sock, tok, tokVal := testSetup(t)

	notif1 := &phonebridgev1.NotificationPosted{
		Key:         "0|com.whatsapp|1|null|1001",
		PackageName: "com.whatsapp",
		AppName:     "WhatsApp",
		Title:       "Bob",
		Text:        "Hello there",
		PostTimeMs:  1700000000000,
		IsClearable: true,
	}

	orch := &mockOrchestrator{
		notifications: []*phonebridgev1.NotificationPosted{notif1},
	}

	cfg := Config{
		SocketPath:       sock,
		TokenPath:        tok,
		Token:            tokVal,
		ServerVersion:    "0.1.0-test",
		DaemonGeneration: 101,
	}

	srv, cancel, errCh := startTestServer(t, cfg)
	defer func() {
		cancel()
		<-errCh
	}()
	srv.SetOrchestrator(orch)

	client, err := Dial(context.Background(), sock, tokVal)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	// 1. Test ListNotifications RPC
	listResp, err := client.ListNotifications(context.Background(), &phonebridgelocalipcv1.ListNotificationsRequest{})
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if len(listResp.GetNotifications()) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(listResp.GetNotifications()))
	}
	gotNotif := listResp.GetNotifications()[0]
	if gotNotif.GetKey() != notif1.Key || gotNotif.GetTitle() != notif1.Title {
		t.Fatalf("unexpected notification: %+v", gotNotif)
	}

	// 2. Test StreamEvents receiving BroadcastNotificationEvent
	ctx, streamCancel := context.WithCancel(context.Background())
	defer streamCancel()

	stream, err := client.StreamEvents(ctx, &phonebridgelocalipcv1.StreamEventsRequest{})
	if err != nil {
		t.Fatalf("StreamEvents: %v", err)
	}

	frame := &phonebridgev1.NotificationFrame{
		Version:     1,
		TimestampMs: 1700000000000,
		Event: &phonebridgev1.NotificationFrame_Posted{
			Posted: notif1,
		},
	}

	// Give subscriber time to register
	time.Sleep(50 * time.Millisecond)
	srv.BroadcastNotificationEvent(frame)

	recvCh := make(chan *phonebridgelocalipcv1.StreamEventsResponse, 1)
	go func() {
		ev, err := stream.Recv()
		if err == nil {
			recvCh <- ev
		}
	}()

	select {
	case ev := <-recvCh:
		if ev.GetNotificationEvent() == nil {
			t.Fatalf("expected notification event in stream, got %+v", ev)
		}
		posted := ev.GetNotificationEvent().GetPosted()
		if posted == nil || posted.GetKey() != notif1.Key {
			t.Fatalf("unexpected posted in notification event: %+v", posted)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notification event on StreamEvents")
	}
}

func TestNotification_ListWhenClosed(t *testing.T) {
	sock, tok, tokVal := testSetup(t)

	cfg := Config{
		SocketPath:       sock,
		TokenPath:        tok,
		Token:            tokVal,
		ServerVersion:    "0.1.0-test",
		DaemonGeneration: 101,
	}

	srv, cancel, errCh := startTestServer(t, cfg)
	client, err := Dial(context.Background(), sock, tokVal)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	// Shutdown server
	cancel()
	<-errCh

	_, err = srv.ListNotifications(context.Background(), &phonebridgelocalipcv1.ListNotificationsRequest{})
	if err == nil {
		t.Fatal("expected error when server is closed")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.Unavailable {
		t.Fatalf("expected codes.Unavailable, got %v", err)
	}
}
