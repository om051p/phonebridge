package engine

import (
	"sync"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/notification"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"google.golang.org/protobuf/proto"
)

func TestSession_NotificationHandling(t *testing.T) {
	store := notification.NewStore()
	limiter := notification.NewLimiter(20.0, 10.0, 100*time.Millisecond)

	var (
		receivedMu sync.Mutex
		received   []*phonebridgev1.NotificationFrame
	)

	cfg := DefaultSessionConfig()
	cfg.NotificationStore = store
	cfg.NotificationLimiter = limiter
	cfg.OnNotification = func(f *phonebridgev1.NotificationFrame) {
		receivedMu.Lock()
		defer receivedMu.Unlock()
		received = append(received, proto.Clone(f).(*phonebridgev1.NotificationFrame))
	}

	sess := NewSession("test-sess", cfg, nil, nil)

	// 1. Valid NotificationPosted
	notif1 := &phonebridgev1.NotificationPosted{
		Key:         "0|com.example.chat|1|tag|100",
		PackageName: "com.example.chat",
		AppName:     "ExampleChat",
		Title:       "Alice",
		Text:        "Hey there!",
		PostTimeMs:  1700000000000,
		IsClearable: true,
	}
	frame1 := &phonebridgev1.NotificationFrame{
		Version:     1,
		TimestampMs: 1700000000000,
		Event: &phonebridgev1.NotificationFrame_Posted{
			Posted: notif1,
		},
	}
	data1, err := proto.Marshal(frame1)
	if err != nil {
		t.Fatal(err)
	}

	sess.handleNotificationMessage(0, data1)

	if store.Count() != 1 {
		t.Fatalf("expected 1 notification in store, got %d", store.Count())
	}
	got1, ok := store.Get("0|com.example.chat|1|tag|100")
	if !ok || got1.Title != "Alice" {
		t.Fatalf("unexpected notification in store: %+v", got1)
	}

	receivedMu.Lock()
	if len(received) != 1 {
		t.Fatalf("expected 1 callback received, got %d", len(received))
	}
	receivedMu.Unlock()

	// 2. Immediate duplicate should be suppressed by limiter
	sess.handleNotificationMessage(0, data1)
	receivedMu.Lock()
	if len(received) != 1 {
		t.Fatalf("duplicate should not trigger callback, got count %d", len(received))
	}
	receivedMu.Unlock()

	// 3. NotificationRemoved removes from store
	frameRemoved := &phonebridgev1.NotificationFrame{
		Version:     1,
		TimestampMs: 1700000001000,
		Event: &phonebridgev1.NotificationFrame_Removed{
			Removed: &phonebridgev1.NotificationRemoved{
				Key:         "0|com.example.chat|1|tag|100",
				PackageName: "com.example.chat",
				Reason:      1,
			},
		},
	}
	dataRemoved, err := proto.Marshal(frameRemoved)
	if err != nil {
		t.Fatal(err)
	}

	sess.handleNotificationMessage(0, dataRemoved)
	if store.Count() != 0 {
		t.Fatalf("expected 0 notifications after remove, got %d", store.Count())
	}
	receivedMu.Lock()
	if len(received) != 2 {
		t.Fatalf("expected 2 callbacks total (posted + removed), got %d", len(received))
	}
	receivedMu.Unlock()

	// 4. Stale generation or malformed data is dropped
	sess.handleNotificationMessage(999, data1) // wrong gen
	sess.handleNotificationMessage(0, []byte("garbage-bytes"))
	if store.Count() != 0 {
		t.Fatalf("store count should still be 0, got %d", store.Count())
	}
}

func TestSessionManager_NotificationStoreAndTeardown(t *testing.T) {
	mgr := NewSessionManager(DefaultSessionConfig(), nil, nil, nil)

	notif := &phonebridgev1.NotificationPosted{
		Key:         "0|com.test.app|1|null|100",
		PackageName: "com.test.app",
		Title:       "Test",
		PostTimeMs:  1700000000000,
	}
	mgr.notificationStore.Put(notif)

	list := mgr.ListNotifications()
	if len(list) != 1 {
		t.Fatalf("expected 1 notification from manager, got %d", len(list))
	}

	// Purge store on session disconnect / stop
	mgr.notificationStore.Clear()
	if len(mgr.ListNotifications()) != 0 {
		t.Fatalf("expected 0 notifications after clear, got %d", len(mgr.ListNotifications()))
	}
}
