package discovery

import (
	"net/netip"
	"sync"
	"time"
)

// Device represents a PhoneBridge device discovered on the local network.
type Device struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Model string `json:"model"`
	// Addresses keeps the resolved addresses with their IPv6 zones intact:
	// mDNS link-local v6 (fe80::/10) is only dialable with its %interface
	// zone, and stripping it (net.IP cannot carry one) made such peers
	// undialable. Consumers should dial via engine.DialHost/Endpoint.
	Addresses    []netip.Addr      `json:"addresses"`
	Port         uint16            `json:"port"`
	Version      string            `json:"version"`
	Capabilities []string          `json:"capabilities"`
	State        string            `json:"state"`
	LastSeen     time.Time         `json:"last_seen"`
	IsStale      bool              `json:"is_stale"`
	Metadata     map[string]string `json:"metadata"`
}

// EventKind describes changes in device discovery state.
type EventKind int

const (
	// DeviceDiscovered indicates a new device was seen for the first time.
	DeviceDiscovered EventKind = iota
	// DeviceUpdated indicates an existing device refreshed its announcement or attributes.
	DeviceUpdated
	// DeviceStale indicates a device has not sent a heartbeat within the stale threshold.
	DeviceStale
	// DeviceLost indicates a device missed its TTL and was evicted.
	DeviceLost
)

func (k EventKind) String() string {
	switch k {
	case DeviceDiscovered:
		return "DISCOVERED"
	case DeviceUpdated:
		return "UPDATED"
	case DeviceStale:
		return "STALE"
	case DeviceLost:
		return "LOST"
	default:
		return "UNKNOWN"
	}
}

// Event represents a discovery state notification.
type Event struct {
	Kind   EventKind
	Device Device
}

// RegistryConfig configures device retention and stale detection.
type RegistryConfig struct {
	StaleTimeout time.Duration
	LostTimeout  time.Duration
}

// DefaultRegistryConfig returns production defaults for discovery timeouts.
func DefaultRegistryConfig() RegistryConfig {
	return RegistryConfig{
		StaleTimeout: 10 * time.Second,
		LostTimeout:  30 * time.Second,
	}
}

// DeviceRegistry maintains an in-memory, thread-safe table of discovered devices.
type DeviceRegistry struct {
	mu      sync.RWMutex
	devices map[string]*Device
	cfg     RegistryConfig
	onEvent func(Event)
}

// NewDeviceRegistry creates a registry with the provided configuration and event callback.
func NewDeviceRegistry(cfg RegistryConfig, onEvent func(Event)) *DeviceRegistry {
	if cfg.StaleTimeout <= 0 {
		cfg.StaleTimeout = 10 * time.Second
	}
	if cfg.LostTimeout <= 0 {
		cfg.LostTimeout = 30 * time.Second
	}
	return &DeviceRegistry{
		devices: make(map[string]*Device),
		cfg:     cfg,
		onEvent: onEvent,
	}
}

// Upsert adds a new device or updates an existing device record (rediscovery).
func (r *DeviceRegistry) Upsert(dev Device) (EventKind, Device) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	dev.LastSeen = now
	dev.IsStale = false

	existing, found := r.devices[dev.ID]
	if !found {
		// New device discovered
		stored := dev
		r.devices[dev.ID] = &stored
		event := Event{Kind: DeviceDiscovered, Device: stored}
		if r.onEvent != nil {
			r.onEvent(event)
		}
		return DeviceDiscovered, stored
	}

	// Rediscovery / Attribute update
	existing.Name = dev.Name
	existing.Model = dev.Model
	if len(dev.Addresses) > 0 {
		// Merge instead of replace: mDNS emits one event per packet/source, so a
		// replace loses the v4 record whenever a v6 event arrives (or vice
		// versa). Keeping the union lets BestDialAddr pick a dialable family.
		seen := make(map[string]bool, len(existing.Addresses)+len(dev.Addresses))
		merged := existing.Addresses[:0]
		for _, a := range append(append([]netip.Addr{}, existing.Addresses...), dev.Addresses...) {
			k := a.String()
			if !seen[k] {
				seen[k] = true
				merged = append(merged, a)
			}
		}
		if len(merged) > 4 { // cap: two families plus headroom
			merged = merged[len(merged)-4:]
		}
		existing.Addresses = merged
	}
	if dev.Port > 0 {
		existing.Port = dev.Port
	}
	if dev.Version != "" {
		existing.Version = dev.Version
	}
	if len(dev.Capabilities) > 0 {
		existing.Capabilities = dev.Capabilities
	}
	if dev.State != "" {
		existing.State = dev.State
	}
	existing.LastSeen = now
	existing.IsStale = false
	existing.Metadata = dev.Metadata

	stored := *existing
	event := Event{Kind: DeviceUpdated, Device: stored}
	if r.onEvent != nil {
		r.onEvent(event)
	}
	return DeviceUpdated, stored
}

// Get returns a copy of the device by ID if present.
func (r *DeviceRegistry) Get(id string) (Device, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	dev, ok := r.devices[id]
	if !ok {
		return Device{}, false
	}
	return *dev, true
}

// List returns a snapshot of all active and stale devices.
func (r *DeviceRegistry) List() []Device {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]Device, 0, len(r.devices))
	for _, d := range r.devices {
		res = append(res, *d)
	}
	return res
}

// Remove explicitly unregisters a device (e.g. on goodbye message).
func (r *DeviceRegistry) Remove(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	dev, ok := r.devices[id]
	if !ok {
		return false
	}
	deleted := *dev
	delete(r.devices, id)
	if r.onEvent != nil {
		r.onEvent(Event{Kind: DeviceLost, Device: deleted})
	}
	return true
}

// Sweep checks for stale and expired devices. Returns lists of stale and removed devices.
func (r *DeviceRegistry) Sweep(now time.Time) (stale []Device, lost []Device) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, dev := range r.devices {
		age := now.Sub(dev.LastSeen)
		if age > r.cfg.LostTimeout {
			lost = append(lost, *dev)
			delete(r.devices, id)
			if r.onEvent != nil {
				r.onEvent(Event{Kind: DeviceLost, Device: *dev})
			}
		} else if age > r.cfg.StaleTimeout && !dev.IsStale {
			dev.IsStale = true
			stale = append(stale, *dev)
			if r.onEvent != nil {
				r.onEvent(Event{Kind: DeviceStale, Device: *dev})
			}
		}
	}
	return stale, lost
}
