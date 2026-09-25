package notification

import (
	"testing"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

func TestStoreOperations(t *testing.T) {
	s := NewStore()

	n1 := &phonebridgev1.NotificationPosted{
		Key:         "key1",
		PackageName: "com.test",
		Title:       "Title 1",
		PostTimeMs:  1000,
	}
	n2 := &phonebridgev1.NotificationPosted{
		Key:         "key2",
		PackageName: "com.test",
		Title:       "Title 2",
		PostTimeMs:  2000,
	}

	s.Put(n1)
	s.Put(n2)

	if s.Count() != 2 {
		t.Fatalf("expected count 2, got %d", s.Count())
	}

	list := s.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 items, got %d", len(list))
	}
	// Sorted newest first -> n2 (2000) then n1 (1000)
	if list[0].Key != "key2" || list[1].Key != "key1" {
		t.Fatalf("unexpected order: %v, %v", list[0].Key, list[1].Key)
	}

	// In-place update
	n1Updated := &phonebridgev1.NotificationPosted{
		Key:         "key1",
		PackageName: "com.test",
		Title:       "Title 1 Updated",
		PostTimeMs:  3000,
	}
	s.Put(n1Updated)
	if s.Count() != 2 {
		t.Fatalf("expected count 2 after update, got %d", s.Count())
	}
	got, ok := s.Get("key1")
	if !ok || got.Title != "Title 1 Updated" {
		t.Fatalf("expected updated title, got %v", got)
	}

	// Remove
	s.Remove("key1")
	if s.Count() != 1 {
		t.Fatalf("expected count 1 after remove, got %d", s.Count())
	}

	// Clear
	s.Clear()
	if s.Count() != 0 {
		t.Fatalf("expected count 0 after clear, got %d", s.Count())
	}
}
