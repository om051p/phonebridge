package notification

import (
	"sort"
	"sync"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"google.golang.org/protobuf/proto"
)

// Store maintains thread-safe in-memory mirrored notifications for the active session (DEC-028).
// Ephemeral only: zero disk persistence.
type Store struct {
	mu    sync.RWMutex
	items map[string]*phonebridgev1.NotificationPosted
}

// NewStore creates a fresh in-memory notification store.
func NewStore() *Store {
	return &Store{
		items: make(map[string]*phonebridgev1.NotificationPosted),
	}
}

// Put adds or updates a notification in-place.
func (s *Store) Put(n *phonebridgev1.NotificationPosted) {
	if n == nil || n.Key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Store clone to prevent external mutation
	s.items[n.Key] = proto.Clone(n).(*phonebridgev1.NotificationPosted)
}

// Remove deletes a notification by key.
func (s *Store) Remove(key string) {
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, key)
}

// Get retrieves a notification by key.
func (s *Store) Get(key string) (*phonebridgev1.NotificationPosted, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[key]
	if !ok {
		return nil, false
	}
	return proto.Clone(item).(*phonebridgev1.NotificationPosted), true
}

// List returns all active notifications sorted newest first by PostTimeMs.
func (s *Store) List() []*phonebridgev1.NotificationPosted {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]*phonebridgev1.NotificationPosted, 0, len(s.items))
	for _, item := range s.items {
		res = append(res, proto.Clone(item).(*phonebridgev1.NotificationPosted))
	}

	sort.Slice(res, func(i, j int) bool {
		return res[i].PostTimeMs > res[j].PostTimeMs
	})

	return res
}

// Clear evicts all notifications (called on session disconnect or reset).
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = make(map[string]*phonebridgev1.NotificationPosted)
}

// Count returns the number of active notifications.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}
