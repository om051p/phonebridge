//go:build android || jni

package main

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"github.com/om051p/phonebridge/core/pkg/discovery"
	"github.com/om051p/phonebridge/core/pkg/engine"
)

// PeerInfo is the wire shape the Android UI consumes for one discovered peer.
//
// It mirrors the subset of discovery.Device the Devices tab renders, with the
// address pre-resolved to a single dialable host so the Kotlin/Dart side never
// has to reason about address families, zones, or bracketing.
type PeerInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Model   string `json:"model"`
	Version string `json:"version"`
	Host    string `json:"host"`
	Port    uint16 `json:"port"`
	IsStale bool   `json:"is_stale"`
}

// DiscoveryBridge browses the LAN for `_phonebridge._tcp` peers from the phone.
//
// Why the phone browses at all: Android's NsdAdvertiser only registers this
// device (DEC-007 ships advertise-on-Android / browse-on-Linux), so before this
// bridge existed the Devices tab could never list the PC — there was nothing to
// list from. The browse logic itself is the same discovery package the daemon
// ships, including its active browse-refresh loop, so a steady-state peer is
// not swept stale while it is still answering queries.
//
// The bridge is deliberately browse-only: Port stays 0 and DeviceID empty, so
// discovery.Start skips the advertisement. The phone's advertisement must stay
// the platform NSD registration (NsdAdvertiser) — a second advertisement via
// pion/mdns would publish a duplicate instance for the same device.
type DiscoveryBridge struct {
	mu      sync.Mutex
	disc    *discovery.Discovery
	cancel  context.CancelFunc
	started bool

	// snapshot returns the current peer set. It defaults to the live mDNS
	// registry once Start succeeds; tests inject a stub so the wire shape and
	// ordering are verifiable without joining the multicast group.
	snapshot func() []discovery.Device
}

// globalDiscovery is the process-wide bridge (the JNI entry points delegate to
// it), mirroring globalClipboard/transfer.
var globalDiscovery discoveryBridgeSlot

// discoveryBridgeSlot keeps the single bridge instance behind a mutex-free
// pointer store; the bridge itself locks internally.
type discoveryBridgeSlot struct {
	mu   sync.Mutex
	inst *DiscoveryBridge
}

func currentDiscoveryBridge() *DiscoveryBridge {
	globalDiscovery.mu.Lock()
	defer globalDiscovery.mu.Unlock()
	if globalDiscovery.inst == nil {
		globalDiscovery.inst = &DiscoveryBridge{}
	}
	return globalDiscovery.inst
}

// Start begins browsing. Idempotent: a second call while a browse session is
// live is a no-op.
func (b *DiscoveryBridge) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.started {
		return nil
	}

	disc, err := discovery.NewDiscovery(discovery.Config{
		// Browse-only: no Port and no DeviceID means discovery.Start will not
		// register an advertisement (see the type comment).
		Version:    "1",
		DeviceName: "",
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := disc.Start(ctx); err != nil {
		cancel()
		_ = disc.Close()
		return err
	}

	b.disc = disc
	b.cancel = cancel
	b.started = true
	b.snapshot = func() []discovery.Device { return disc.Registry().List() }
	return nil
}

// Stop ends browsing and releases the multicast sockets.
func (b *DiscoveryBridge) Stop() {
	b.mu.Lock()
	disc := b.disc
	cancel := b.cancel
	b.disc = nil
	b.cancel = nil
	b.started = false
	b.snapshot = nil
	b.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if disc != nil {
		_ = disc.Close()
	}
}

// Running reports whether a browse session is live (diagnostics/tests).
func (b *DiscoveryBridge) Running() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.started
}

// Peers returns the current peer snapshot in a deterministic order.
func (b *DiscoveryBridge) Peers() []PeerInfo {
	b.mu.Lock()
	snapshot := b.snapshot
	b.mu.Unlock()

	if snapshot == nil {
		return []PeerInfo{}
	}
	return peersFromDevices(snapshot())
}

// PeersJSON renders the snapshot as a JSON array (never null, so the Kotlin
// side never has to special-case an absent list).
func (b *DiscoveryBridge) PeersJSON() []byte {
	out, err := json.Marshal(b.Peers())
	if err != nil {
		return []byte("[]")
	}
	return out
}

// peersFromDevices is the pure mapping from discovery records to the UI shape:
// a peer with no dialable address is omitted (it could never be connected to),
// and the order is stable so repeated polls do not reshuffle the list.
func peersFromDevices(devices []discovery.Device) []PeerInfo {
	peers := make([]PeerInfo, 0, len(devices))
	for _, d := range devices {
		endpoint, ok := engine.Endpoint(d.Addresses, d.Port)
		if !ok || d.Port == 0 {
			continue
		}
		// engine.Endpoint renders "host:port" (IPv6 literals bracketed). The UI
		// keeps host and port as separate fields because it composes the
		// signaling URL itself, so strip the trailing ":port" from the rendered
		// endpoint instead of re-deriving the address form — that keeps the
		// bracket+zone handling in one place (engine.DialHost).
		host := endpoint[:strings.LastIndex(endpoint, ":")]
		if host == "" {
			continue
		}
		name := d.Name
		if name == "" {
			name = d.ID
		}
		peers = append(peers, PeerInfo{
			ID:      d.ID,
			Name:    name,
			Model:   d.Model,
			Version: d.Version,
			Host:    host,
			Port:    d.Port,
			IsStale: d.IsStale,
		})
	}

	sort.Slice(peers, func(i, j int) bool {
		if peers[i].Name != peers[j].Name {
			return peers[i].Name < peers[j].Name
		}
		return peers[i].ID < peers[j].ID
	})
	return peers
}

// invokeDiscovery routes the generic "discovery:*" control-plane methods. It
// mirrors invokeTransfer: a nil result is rendered as an empty object by the
// JNI boundary, and an unrecognised verb is not handled here.
func invokeDiscovery(method string, payload []byte) ([]byte, bool) {
	const prefix = "discovery:"
	if len(method) <= len(prefix) || method[:len(prefix)] != prefix {
		return nil, false
	}

	bridge := currentDiscoveryBridge()
	switch method[len(prefix):] {
	case "list":
		// Lazy start: the browse costs nothing until the UI asks, and a bridge
		// whose sockets failed to bind still answers with an empty list rather
		// than failing the UI's refresh.
		if err := bridge.Start(); err != nil {
			return []byte("[]"), true
		}
		return bridge.PeersJSON(), true
	case "stop":
		bridge.Stop()
		return []byte("{}"), true
	}
	return nil, false
}
