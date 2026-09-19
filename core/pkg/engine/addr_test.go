package engine

import (
	"net/netip"
	"testing"
)

func TestBestDialAddrPrefersIPv4(t *testing.T) {
	link := netip.MustParseAddr("fe80::284c:a6ff:fe0c:8d8c%wlan0")
	v4 := netip.MustParseAddr("192.168.0.125")
	got, ok := BestDialAddr([]netip.Addr{link, v4})
	if !ok || got != v4 {
		t.Fatalf("want %v, got %v ok=%v", v4, got, ok)
	}
}

func TestBestDialAddrPrefersGlobalV6OverLinkLocal(t *testing.T) {
	link := netip.MustParseAddr("fe80::1%eth0")
	global := netip.MustParseAddr("fd00::5")
	got, ok := BestDialAddr([]netip.Addr{link, global})
	if !ok || got != global {
		t.Fatalf("want %v, got %v ok=%v", global, got, ok)
	}
}

func TestBestDialAddrFallsBackToZonedLinkLocal(t *testing.T) {
	link := netip.MustParseAddr("fe80::284c:a6ff:fe0c:8d8c%wlan0")
	got, ok := BestDialAddr([]netip.Addr{link})
	if !ok || got != link {
		t.Fatalf("want zoned link-local %v, got %v ok=%v", link, got, ok)
	}
}

func TestBestDialAddrRejectsUnscopedLinkLocal(t *testing.T) {
	// A link-local without a zone is not dialable — it must never be chosen.
	unscoped := netip.MustParseAddr("fe80::284c:a6ff:fe0c:8d8c")
	if _, ok := BestDialAddr([]netip.Addr{unscoped}); ok {
		t.Fatal("unscoped link-local must not be dialable")
	}
}

func TestBestDialAddrEmpty(t *testing.T) {
	if _, ok := BestDialAddr(nil); ok {
		t.Fatal("empty list must not yield an address")
	}
}

func TestDialHostBracketsIPv6(t *testing.T) {
	cases := []struct {
		in   netip.Addr
		want string
	}{
		{netip.MustParseAddr("192.168.0.125"), "192.168.0.125"},
		{netip.MustParseAddr("fe80::1%wlan0"), "[fe80::1%wlan0]"},
		{netip.MustParseAddr("fd00::5"), "[fd00::5]"},
	}
	for _, c := range cases {
		if got := DialHost(c.in); got != c.want {
			t.Errorf("DialHost(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestEndpointFromDiscoveryStyleAddrs(t *testing.T) {
	// Regression for the Phase 2 acceptance finding: a device advertising a
	// link-local v6 packet source plus an A-record v4 must dial the v4.
	addrs := []netip.Addr{
		netip.MustParseAddr("fe80::284c:a6ff:fe0c:8d8c%wlan0"),
		netip.MustParseAddr("192.168.0.125"),
	}
	ep, ok := Endpoint(addrs, 7804)
	if !ok {
		t.Fatal("expected an endpoint")
	}
	if ep != "192.168.0.125:7804" {
		t.Fatalf("endpoint = %q, want 192.168.0.125:7804", ep)
	}
}
