package discovery

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/pion/mdns/v2"
	"golang.org/x/net/ipv4"
)

func TestRegistry_UpsertAndGet(t *testing.T) {
	var events []Event
	reg := NewDeviceRegistry(DefaultRegistryConfig(), func(e Event) {
		events = append(events, e)
	})

	dev1 := Device{
		ID:           "dev-01",
		Name:         "POCO F5",
		Model:        "23049PCD8I",
		Addresses:    []net.IP{net.ParseIP("192.168.0.125")},
		Port:         7804,
		Version:      "1",
		Capabilities: []string{"screen", "files"},
		State:        "ready",
	}

	kind, stored := reg.Upsert(dev1)
	if kind != DeviceDiscovered {
		t.Fatalf("expected DeviceDiscovered, got %v", kind)
	}
	if stored.ID != "dev-01" || stored.Name != "POCO F5" {
		t.Fatalf("unexpected stored device: %+v", stored)
	}
	if len(events) != 1 || events[0].Kind != DeviceDiscovered {
		t.Fatalf("expected 1 DeviceDiscovered event, got %v", events)
	}

	// Verify Get
	got, ok := reg.Get("dev-01")
	if !ok || got.Name != "POCO F5" {
		t.Fatalf("failed to retrieve device: %+v, ok=%v", got, ok)
	}
}

func TestRegistry_RediscoveryAndUpdate(t *testing.T) {
	var events []Event
	reg := NewDeviceRegistry(DefaultRegistryConfig(), func(e Event) {
		events = append(events, e)
	})

	dev1 := Device{
		ID:    "dev-01",
		Name:  "POCO F5",
		Model: "23049PCD8I",
		Port:  7804,
		State: "idle",
	}
	reg.Upsert(dev1)

	time.Sleep(5 * time.Millisecond)

	// Device refreshes advertisement with new state
	dev1Update := Device{
		ID:    "dev-01",
		Name:  "POCO F5 (Renamed)",
		Model: "23049PCD8I",
		Port:  7804,
		State: "streaming",
	}
	kind, updated := reg.Upsert(dev1Update)
	if kind != DeviceUpdated {
		t.Fatalf("expected DeviceUpdated, got %v", kind)
	}
	if updated.Name != "POCO F5 (Renamed)" || updated.State != "streaming" {
		t.Fatalf("updated device did not reflect new attributes: %+v", updated)
	}
	if len(events) != 2 || events[1].Kind != DeviceUpdated {
		t.Fatalf("expected DeviceUpdated event, got %v", events)
	}
}

func TestRegistry_StaleAndEvictionSweep(t *testing.T) {
	var events []Event
	cfg := RegistryConfig{
		StaleTimeout: 50 * time.Millisecond,
		LostTimeout:  150 * time.Millisecond,
	}
	reg := NewDeviceRegistry(cfg, func(e Event) {
		events = append(events, e)
	})

	t0 := time.Now()
	dev := Device{ID: "dev-stale", Name: "Test Device"}
	reg.Upsert(dev)

	// At t0 + 20ms: device is fresh
	stale, lost := reg.Sweep(t0.Add(20 * time.Millisecond))
	if len(stale) != 0 || len(lost) != 0 {
		t.Fatalf("expected no stale or lost devices at 20ms, got stale=%v lost=%v", stale, lost)
	}

	// At t0 + 80ms: device passes stale threshold (50ms)
	stale, lost = reg.Sweep(t0.Add(80 * time.Millisecond))
	if len(stale) != 1 || stale[0].ID != "dev-stale" {
		t.Fatalf("expected 1 stale device, got %v", stale)
	}
	if len(lost) != 0 {
		t.Fatalf("expected 0 lost devices, got %v", lost)
	}

	// Verify device is marked stale in registry
	stored, ok := reg.Get("dev-stale")
	if !ok || !stored.IsStale {
		t.Fatalf("expected stored device to be stale, got %+v", stored)
	}

	// At t0 + 100ms: second sweep while still stale shouldn't re-emit stale
	stale2, _ := reg.Sweep(t0.Add(100 * time.Millisecond))
	if len(stale2) != 0 {
		t.Fatalf("expected 0 stale events on repeat sweep, got %v", stale2)
	}

	// At t0 + 200ms: device passes lost threshold (150ms)
	stale, lost = reg.Sweep(t0.Add(200 * time.Millisecond))
	if len(lost) != 1 || lost[0].ID != "dev-stale" {
		t.Fatalf("expected 1 lost device, got %v", lost)
	}

	// Verify device removed from registry
	if _, ok := reg.Get("dev-stale"); ok {
		t.Fatalf("expected dev-stale to be removed from registry")
	}
}

func TestDiscovery_LoopbackMdnsEndToEnd(t *testing.T) {
	// Create two discovery instances over loopback
	addr1, err := net.ResolveUDPAddr("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l1, err := net.ListenUDP("udp4", addr1)
	if err != nil {
		t.Fatal(err)
	}
	p1 := ipv4.NewPacketConn(l1)
	_ = p1.SetMulticastLoopback(true)

	addr2, err := net.ResolveUDPAddr("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l2, err := net.ListenUDP("udp4", addr2)
	if err != nil {
		t.Fatal(err)
	}
	p2 := ipv4.NewPacketConn(l2)
	_ = p2.SetMulticastLoopback(true)

	d1, err := NewDiscovery(Config{
		DeviceID:          "advertiser-phone",
		DeviceName:        "POCO F5 Loopback",
		Model:             "23049PCD8I",
		Port:              7804,
		Capabilities:      []string{"screen", "clipboard"},
		CustomPacketConn4: p1,
		IncludeLoopback:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer d1.Close()

	d2, err := NewDiscovery(Config{
		DeviceID:          "browser-linux",
		DeviceName:        "Linux Host",
		CustomPacketConn4: p2,
		IncludeLoopback:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := d1.Start(ctx); err != nil {
		t.Skipf("mDNS loopback bind not supported in this environment: %v", err)
	}
	if err := d2.Start(ctx); err != nil {
		t.Skipf("mDNS loopback bind not supported in this environment: %v", err)
	}

	// Simulate discovery event delivery into d2 handler directly to verify parsing
	ev := mdns.ServiceEvent{
		Instance: mdns.ServiceInstance{
			Instance: "PhoneBridge-advertiser-phone",
			Service:  ServiceType,
			Domain:   ServiceDomain,
			Port:     7804,
			Text: []mdns.TXTEntry{
				mdns.NewTXTString("id", "advertiser-phone"),
				mdns.NewTXTString("name", "POCO F5 Loopback"),
				mdns.NewTXTString("model", "23049PCD8I"),
				mdns.NewTXTString("v", "1"),
				mdns.NewTXTString("caps", "screen,clipboard"),
				mdns.NewTXTString("state", "ready"),
			},
		},
		Addr: netip.MustParseAddr("127.0.0.1"),
	}

	d2.handleDiscoveredService(ev)

	dev, ok := d2.Registry().Get("advertiser-phone")
	if !ok {
		t.Fatalf("expected advertiser-phone in d2 registry")
	}
	if dev.Name != "POCO F5 Loopback" || dev.Port != 7804 || dev.Model != "23049PCD8I" {
		t.Fatalf("unexpected parsed device: %+v", dev)
	}
	if len(dev.Capabilities) != 2 || dev.Capabilities[0] != "screen" {
		t.Fatalf("unexpected capabilities: %v", dev.Capabilities)
	}
}
