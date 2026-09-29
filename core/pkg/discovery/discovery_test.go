package discovery

import (
	"context"
	"net"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/mdns/v2"
	"golang.org/x/net/dns/dnsmessage"
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
		Addresses:    []netip.Addr{netip.MustParseAddr("192.168.0.125")},
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

func TestDiscovery_LongDeviceID_Registration(t *testing.T) {
	addr, err := net.ResolveUDPAddr("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l, err := net.ListenUDP("udp4", addr)
	if err != nil {
		t.Fatal(err)
	}
	p := ipv4.NewPacketConn(l)
	_ = p.SetMulticastLoopback(true)

	// 64-char hex device ID
	longID := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	d, err := NewDiscovery(Config{
		DeviceID:          longID,
		DeviceName:        "Linux Host",
		Port:              7804,
		CustomPacketConn4: p,
		IncludeLoopback:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := d.Start(ctx); err != nil {
		t.Fatalf("d.Start failed with 64-char DeviceID: %v", err)
	}
}

// TestRegistry_RefreshClearsStale verifies the registry contract the browse
// refresh relies on: an Upsert of a previously-stale peer clears IsStale and
// advances LastSeen (delivered as DeviceUpdated), so periodic re-emission
// keeps a steady-state peer discoverable past StaleTimeout.
func TestRegistry_RefreshClearsStale(t *testing.T) {
	var events []Event
	cfg := RegistryConfig{StaleTimeout: 50 * time.Millisecond, LostTimeout: 1 * time.Second}
	reg := NewDeviceRegistry(cfg, func(e Event) { events = append(events, e) })

	dev := Device{ID: "dev-refresh", Name: "POCO F5", Port: 7804}
	reg.Upsert(dev)

	stale, _ := reg.Sweep(time.Now().Add(80 * time.Millisecond))
	if len(stale) != 1 {
		t.Fatalf("expected peer to go stale before refresh, got stale=%v", stale)
	}

	kind, refreshed := reg.Upsert(dev)
	if kind != DeviceUpdated {
		t.Fatalf("expected DeviceUpdated on refresh, got %v", kind)
	}
	if refreshed.IsStale {
		t.Fatalf("expected refreshed peer to be fresh, got IsStale=true")
	}
	if time.Since(refreshed.LastSeen) > time.Second {
		t.Fatalf("expected LastSeen advanced by refresh, got %v", refreshed.LastSeen)
	}
}

// TestDiscovery_BrowseRestartRefreshesPeerPastStaleWindow reproduces the
// Android NSD staleness defect deterministically and proves the fix.
//
// A fake responder streams byte-identical DNS-SD responses (PTR+SRV+TXT+A for
// PhoneBridge-stale-phone) to the browser — exactly the steady-state behavior
// of an Android NSD responder, which announces only on registration and
// answers every query with an identical response. pion/mdns's browse session
// keeps a per-session "seen" map and only emits records that are new or
// changed, so identical answers never reach the registry: LastSeen freezes
// and the peer is swept stale while answers are still flowing (the reported
// defect). Restarting the browse session re-queries with fresh session
// state, the identical response is re-emitted, and the peer refreshes with
// zero advertiser-side change.
func TestDiscovery_BrowseRestartRefreshesPeerPastStaleWindow(t *testing.T) {
	// Browser socket on loopback; the fake responder streams responses from
	// a second socket straight to the browser's address. Source ports do not
	// affect pion's answer processing, and no multicast routing is involved.
	sock, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	responderSock, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		sock.Close()
		t.Fatal(err)
	}
	defer responderSock.Close()
	browserAddr := sock.LocalAddr().(*net.UDPAddr)

	// One response, packed once, sent identically forever — Android NSD semantics.
	svcName, _ := dnsmessage.NewName("_phonebridge._tcp.local.")
	instName, _ := dnsmessage.NewName("PhoneBridge-stale-phone._phonebridge._tcp.local.")
	hostName, _ := dnsmessage.NewName("android-phone.local.")
	resp := dnsmessage.Message{
		Header: dnsmessage.Header{Response: true, Authoritative: true},
		Answers: []dnsmessage.Resource{
			{
				Header: dnsmessage.ResourceHeader{Name: svcName, Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET, TTL: 120},
				Body:   &dnsmessage.PTRResource{PTR: instName},
			},
			{
				Header: dnsmessage.ResourceHeader{Name: instName, Type: dnsmessage.TypeSRV, Class: dnsmessage.ClassINET, TTL: 120},
				Body:   &dnsmessage.SRVResource{Target: hostName, Port: 7804},
			},
			{
				Header: dnsmessage.ResourceHeader{Name: instName, Type: dnsmessage.TypeTXT, Class: dnsmessage.ClassINET, TTL: 120},
				Body: &dnsmessage.TXTResource{TXT: []string{
					"id=stale-phone", "name=POCO F5", "model=23049PCD8I", "v=1", "caps=screen,files", "state=ready",
				}},
			},
			{
				Header: dnsmessage.ResourceHeader{Name: hostName, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 120},
				Body:   &dnsmessage.AResource{A: [4]byte{127, 0, 0, 1}},
			},
		},
	}
	raw, err := resp.Pack()
	if err != nil {
		t.Fatal(err)
	}

	var responding atomic.Bool
	responding.Store(true)
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if responding.Load() {
					_, _ = responderSock.WriteToUDP(raw, browserAddr)
				}
			}
		}
	}()

	browser, err := NewDiscovery(Config{
		DeviceID:          "linux-host",
		DeviceName:        "Linux Host",
		CustomPacketConn4: ipv4.NewPacketConn(sock),
		IncludeLoopback:   true,
		StaleTimeout:      1500 * time.Millisecond,
		LostTimeout:       30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := browser.Start(ctx); err != nil {
		t.Fatalf("browser.Start: %v", err)
	}

	// Phase 1: initial discovery through the real answer -> parse -> emit path.
	deadline := time.Now().Add(5 * time.Second)
	var first Device
	for {
		if dev, ok := browser.Registry().Get("stale-phone"); ok {
			first = dev
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("initial discovery failed: responder never discovered")
		}
		time.Sleep(25 * time.Millisecond)
	}
	if first.Port != 7804 || first.Name != "POCO F5" || first.Model != "23049PCD8I" || len(first.Addresses) != 1 {
		t.Fatalf("unexpected first discovery record: %+v", first)
	}

	// Phase 2: the responder keeps answering identically (Android steady
	// state), yet the peer must go stale — pion's seen-dedup swallows the
	// identical answers so the registry never refreshes. This is the defect.
	time.Sleep(1800 * time.Millisecond)
	// Note: the background sweepLoop may flag the stale before our explicit
	// Sweep call, and stale events emit once — so assert on stored state.
	stored, ok := browser.Registry().Get("stale-phone")
	if !ok {
		t.Fatalf("peer vanished from registry during defect reproduction")
	}
	if !stored.IsStale {
		t.Fatalf("defect not reproduced: peer stayed fresh despite identical answers flowing for 1.8s: %+v", stored)
	}
	if !stored.LastSeen.Equal(first.LastSeen) {
		t.Fatalf("expected LastSeen frozen at first emission (dedup proof), got %v -> %v", first.LastSeen, stored.LastSeen)
	}

	// Phase 3: fix — restart the browse session. Fresh session state
	// re-emits the identical response, LastSeen refreshes, and the peer
	// leaves the stale set with zero advertiser-side change.
	before := first.LastSeen
	if err := browser.RestartBrowse(); err != nil {
		t.Fatalf("RestartBrowse: %v", err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for {
		dev, ok := browser.Registry().Get("stale-phone")
		if ok && !dev.IsStale && dev.LastSeen.After(before) {
			return // refreshed from identical data: fix proven
		}
		if time.Now().After(deadline) {
			t.Fatalf("peer not refreshed after RestartBrowse: last_seen=%v stale=%v", dev.LastSeen, dev.IsStale)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// packBrowseQuery builds an mDNS PTR query for a DNS-SD service type.
func packBrowseQuery(t *testing.T, serviceType, domain string) []byte {
	t.Helper()

	name, err := dnsmessage.NewName(serviceType + "." + domain + ".")
	if err != nil {
		t.Fatalf("NewName: %v", err)
	}

	msg := dnsmessage.Message{
		Questions: []dnsmessage.Question{{
			Name:  name,
			Type:  dnsmessage.TypePTR,
			Class: dnsmessage.ClassINET,
		}},
	}
	raw, err := msg.Pack()
	if err != nil {
		t.Fatalf("pack query: %v", err)
	}

	return raw
}

// TestDiscovery_AnswersBrowseQuery covers the advertising half of discovery: a
// node that registers a service must actually ANSWER a browse query.
//
// Regression test: a DNS-SD answer carries an SRV record whose target is the
// service host, and pion/mdns fills that target from the server's configured
// local names. A registering node that never configures one leaves the target
// empty, so every PTR/SRV answer fails to pack ("SRVResource.Target: name is
// not in canonical format") and no reply is sent. The node then stays invisible
// to every browsing peer — the Android Devices tab lists no PC — even though it
// is listening, browsing, and reaching its own peers.
func TestDiscovery_AnswersBrowseQuery(t *testing.T) {
	serverConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	packetConn := ipv4.NewPacketConn(serverConn)
	_ = packetConn.SetMulticastLoopback(true)

	node, err := NewDiscovery(Config{
		DeviceID:          "advertiser-linux",
		DeviceName:        "Linux Host",
		Model:             "x1",
		Port:              7804,
		Capabilities:      []string{"screen"},
		State:             "ready",
		CustomPacketConn4: packetConn,
		IncludeLoopback:   true,
	})
	if err != nil {
		t.Fatalf("NewDiscovery: %v", err)
	}
	defer node.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := node.Start(ctx); err != nil {
		t.Skipf("mDNS bind not supported in this environment: %v", err)
	}

	querier, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen querier: %v", err)
	}
	defer querier.Close()

	// A direct unicast query to the node's socket exercises the responder
	// without depending on a multicast-capable network.
	if _, err := querier.WriteToUDP(packBrowseQuery(t, ServiceType, ServiceDomain), serverConn.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatalf("write browse query: %v", err)
	}
	if err := querier.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}

	buf := make([]byte, 9000)
	n, _, err := querier.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("registered node never answered the browse query: %v", err)
	}

	var reply dnsmessage.Message
	if err := reply.Unpack(buf[:n]); err != nil {
		t.Fatalf("unpack reply: %v", err)
	}
	if !reply.Header.Response {
		t.Fatalf("expected an mDNS response, got header %+v", reply.Header)
	}

	var gotPTR bool
	for _, answer := range reply.Answers {
		ptr, ok := answer.Body.(*dnsmessage.PTRResource)
		if !ok {
			continue
		}
		if ptr.PTR.String() == "PhoneBridge-advertiser-linux."+ServiceType+"."+ServiceDomain+"." {
			gotPTR = true
		}
	}
	if !gotPTR {
		t.Fatalf("browse answer carried no PTR for the registered instance: %+v", reply.Answers)
	}

	var gotSRV, gotAddr bool
	for _, additional := range reply.Additionals {
		switch body := additional.Body.(type) {
		case *dnsmessage.SRVResource:
			gotSRV = body.Port == 7804
		case *dnsmessage.AResource:
			gotAddr = true
		}
	}
	if !gotSRV {
		t.Fatalf("browse answer carried no SRV record for port 7804: %+v", reply.Additionals)
	}
	if !gotAddr {
		t.Fatalf("browse answer carried no address record: %+v", reply.Additionals)
	}
}
