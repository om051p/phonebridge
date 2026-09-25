//go:build android || jni

package main

import (
	"sync"
	"testing"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"google.golang.org/protobuf/proto"
)

type mockNotificationSender struct {
	mu   sync.Mutex
	sent [][]byte
}

func (m *mockNotificationSender) SendNotification(wireBytes []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, append([]byte(nil), wireBytes...))
	return nil
}

func TestNotificationBridge_LifecycleAndDispatch(t *testing.T) {
	b := &NotificationBridge{}
	sender := &mockNotificationSender{}
	b.SetSender(sender)

	// 1. Before init -> must fail
	err := b.PostNotification("k1", "pkg", "App", "Title", "Text", "", 1000, false, true, "msg")
	if err == nil {
		t.Fatal("expected error before Init")
	}

	// 2. Init
	if err := b.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// 3. Channel closed -> drops quietly
	err = b.PostNotification("k1", "pkg", "App", "Title", "Text", "", 1000, false, true, "msg")
	if err != nil {
		t.Fatalf("expected nil when channel closed (dropped), got: %v", err)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("expected 0 sent when channel closed, got %d", len(sender.sent))
	}

	// 4. Open channel and PostNotification
	b.OnChannelOpen()
	err = b.PostNotification("0|com.example|1|null|10", "com.example", "Example", "Title", "Body", "Sub", 123456789, true, true, "msg")
	if err != nil {
		t.Fatalf("PostNotification failed: %v", err)
	}

	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 frame sent, got %d", len(sender.sent))
	}

	var frame phonebridgev1.NotificationFrame
	if err := proto.Unmarshal(sender.sent[0], &frame); err != nil {
		t.Fatalf("unmarshal frame: %v", err)
	}
	posted := frame.GetPosted()
	if posted == nil {
		t.Fatal("expected posted event in frame")
	}
	if posted.Key != "0|com.example|1|null|10" || posted.PackageName != "com.example" || posted.Title != "Title" || posted.Text != "Body" {
		t.Fatalf("unexpected posted fields: %+v", posted)
	}
	if !posted.IsOngoing || !posted.IsClearable {
		t.Fatalf("unexpected flags: ongoing=%v, clearable=%v", posted.IsOngoing, posted.IsClearable)
	}

	// 5. RemoveNotification
	err = b.RemoveNotification("0|com.example|1|null|10", "com.example", 1)
	if err != nil {
		t.Fatalf("RemoveNotification failed: %v", err)
	}
	if len(sender.sent) != 2 {
		t.Fatalf("expected 2 frames sent, got %d", len(sender.sent))
	}

	var frame2 phonebridgev1.NotificationFrame
	if err := proto.Unmarshal(sender.sent[1], &frame2); err != nil {
		t.Fatalf("unmarshal frame2: %v", err)
	}
	removed := frame2.GetRemoved()
	if removed == nil {
		t.Fatal("expected removed event in frame")
	}
	if removed.Key != "0|com.example|1|null|10" || removed.Reason != 1 {
		t.Fatalf("unexpected removed fields: %+v", removed)
	}

	// 6. Validation errors (empty key or package name)
	if err := b.PostNotification("", "com.example", "App", "Title", "Text", "", 1000, false, true, ""); err == nil {
		t.Fatal("expected validation error for empty key")
	}
	if err := b.RemoveNotification("", "com.example", 1); err == nil {
		t.Fatal("expected validation error for empty key")
	}

	// 7. Stop resets state
	b.Stop()
	if err := b.PostNotification("k1", "pkg", "App", "Title", "Text", "", 1000, false, true, "msg"); err == nil {
		t.Fatal("expected error after Stop")
	}
}
