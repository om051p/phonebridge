//go:build android || jni

package main

import (
	"encoding/json"
	"net/netip"
	"testing"

	"github.com/om051p/phonebridge/core/pkg/discovery"
)

// The phone's Devices tab is fed entirely by this JSON, so its shape (the
// camel/snake keys the Kotlin and Dart layers read) and its ordering are part
// of the contract, not incidental detail.
func TestDiscoveryBridge_PeersJSONWireShape(t *testing.T) {
	bridge := &DiscoveryBridge{
		snapshot: func() []discovery.Device {
			return []discovery.Device{
				{
					ID:        "pc-id",
					Name:      "x1",
					Model:     "Linux",
					Version:   "1",
					Addresses: []netip.Addr{netip.MustParseAddr("192.168.1.20")},
					Port:      7804,
				},
			}
		},
	}

	var got []map[string]any
	if err := json.Unmarshal(bridge.PeersJSON(), &got); err != nil {
		t.Fatalf("PeersJSON is not valid JSON: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(got))
	}
	want := map[string]any{
		"id":       "pc-id",
		"name":     "x1",
		"model":    "Linux",
		"version":  "1",
		"host":     "192.168.1.20",
		"port":     float64(7804),
		"is_stale": false,
	}
	for k, v := range want {
		if got[0][k] != v {
			t.Errorf("field %q = %#v, want %#v", k, got[0][k], v)
		}
	}
}

func TestDiscoveryBridge_PeersJSONIsAlwaysAnArray(t *testing.T) {
	// A bridge that never started (or whose sockets failed to bind) must still
	// answer with an array: the Kotlin side decodes it into a list, and a JSON
	// null would surface as an empty/absent device list with no error.
	bridge := &DiscoveryBridge{}
	if string(bridge.PeersJSON()) != "[]" {
		t.Fatalf("empty bridge PeersJSON() = %s, want []", bridge.PeersJSON())
	}
}

// A peer with no dialable address or no port is not connectable, so it must not
// reach the UI at all — listing it would offer a PAIR/CONNECT button that can
// only fail.
func TestDiscoveryBridge_SkipsUndialablePeers(t *testing.T) {
	bridge := &DiscoveryBridge{
		snapshot: func() []discovery.Device {
			return []discovery.Device{
				{ID: "no-address", Name: "ghost", Port: 7804},
				{ID: "no-port", Name: "portless", Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.5")}},
				{
					ID:        "good",
					Name:      "x1",
					Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.9")},
					Port:      7804,
				},
			}
		},
	}

	peers := bridge.Peers()
	if len(peers) != 1 || peers[0].ID != "good" {
		t.Fatalf("expected only the dialable peer, got %+v", peers)
	}
}

// The registry merges the v4 and link-local v6 records of one peer; the UI needs
// the routable one, and re-bracketed IPv6 so the Dart side can build a URL.
func TestDiscoveryBridge_PrefersIPv4AndKeepsBrackets(t *testing.T) {
	v6Only := &DiscoveryBridge{
		snapshot: func() []discovery.Device {
			return []discovery.Device{{
				ID:        "v6",
				Name:      "pc",
				Addresses: []netip.Addr{netip.MustParseAddr("fe80::1%wlan0")},
				Port:      7804,
			}}
		},
	}
	if got := v6Only.Peers()[0].Host; got != "[fe80::1%wlan0]" {
		t.Errorf("v6 host = %q, want [fe80::1%%wlan0]", got)
	}

	both := &DiscoveryBridge{
		snapshot: func() []discovery.Device {
			return []discovery.Device{{
				ID:   "both",
				Name: "pc",
				Addresses: []netip.Addr{
					netip.MustParseAddr("fe80::1%wlan0"),
					netip.MustParseAddr("192.168.1.20"),
				},
				Port: 7804,
			}}
		},
	}
	if got := both.Peers()[0].Host; got != "192.168.1.20" {
		t.Errorf("dual-stack host = %q, want 192.168.1.20", got)
	}
}

func TestDiscoveryBridge_PeersAreOrdered(t *testing.T) {
	bridge := &DiscoveryBridge{
		snapshot: func() []discovery.Device {
			return []discovery.Device{
				{ID: "b", Name: "zephyr", Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")}, Port: 7804},
				{ID: "a", Name: "alpha", Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")}, Port: 7804},
			}
		},
	}
	peers := bridge.Peers()
	if peers[0].Name != "alpha" || peers[1].Name != "zephyr" {
		t.Fatalf("peers not ordered by name: %+v", peers)
	}
}

func TestInvokeDiscovery_RoutesListAndRejectsUnknownVerbs(t *testing.T) {
	if _, handled := invokeDiscovery("transfer:list", nil); handled {
		t.Fatal("invokeDiscovery must not claim the transfer: prefix")
	}
	if _, handled := invokeDiscovery("discovery:", nil); handled {
		t.Fatal("an empty discovery verb must not be handled")
	}
	if _, handled := invokeDiscovery("discovery:bogus", nil); handled {
		t.Fatal("an unknown discovery verb must not be handled")
	}

	// discovery:list lazy-starts the browse on the global bridge, so the answer
	// has to decode as a (possibly empty) array even when nothing was found or
	// the multicast sockets could not be bound.
	defer currentDiscoveryBridge().Stop()
	out, handled := invokeDiscovery("discovery:list", nil)
	if !handled {
		t.Fatal("discovery:list must be handled")
	}
	var peers []PeerInfo
	if err := json.Unmarshal(out, &peers); err != nil {
		t.Fatalf("discovery:list returned invalid JSON %s: %v", out, err)
	}
}
