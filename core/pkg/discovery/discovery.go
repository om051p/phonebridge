package discovery

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/pion/mdns/v2"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const (
	// ServiceType is the canonical DNS-SD service identifier for PhoneBridge.
	ServiceType = "_phonebridge._tcp"
	// ServiceDomain is the default mDNS domain.
	ServiceDomain = "local"
)

// Config configures the Discovery manager.
type Config struct {
	InstanceName  string
	Port          uint16
	DeviceID      string
	DeviceName    string
	Model         string
	Version       string
	Capabilities  []string
	State         string
	SweepInterval time.Duration
	StaleTimeout  time.Duration
	LostTimeout   time.Duration
	// RefreshInterval controls how often the browse session is restarted to
	// force peers to re-emit their service records. The pion/mdns browse
	// session deduplicates identical answers per session lifetime, so a
	// steady-state peer that merely answers repeated queries identically
	// (Android NSD responders do exactly this) would otherwise never refresh
	// the registry's LastSeen and be swept stale after StaleTimeout.
	RefreshInterval time.Duration
	IncludeLoopback bool
	// CustomPacketConn allows injecting a test or custom packet connection.
	CustomPacketConn4 *ipv4.PacketConn
	CustomPacketConn6 *ipv6.PacketConn
}

// Discovery manages mDNS service advertisement, browsing, and peer tracking.
type Discovery struct {
	cfg      Config
	registry *DeviceRegistry
	server   *mdns.Conn
	events   chan Event

	mu             sync.Mutex
	stopChan       chan struct{}
	closed         bool
	browseLifetime context.Context
	browseCancel   context.CancelFunc
}

// NewDiscovery creates a discovery manager and initializes the device registry.
func NewDiscovery(cfg Config) (*Discovery, error) {
	if cfg.Version == "" {
		cfg.Version = "1"
	}
	if cfg.SweepInterval <= 0 {
		cfg.SweepInterval = 2 * time.Second
	}
	if cfg.StaleTimeout <= 0 {
		cfg.StaleTimeout = 10 * time.Second
	}
	if cfg.LostTimeout <= 0 {
		cfg.LostTimeout = 30 * time.Second
	}
	if cfg.RefreshInterval <= 0 {
		// Default comfortably inside StaleTimeout (10s) so that at least one
		// refresh cycle lands before a quiet peer would be marked stale.
		cfg.RefreshInterval = 7 * time.Second
	}

	eventCh := make(chan Event, 64)
	reg := NewDeviceRegistry(RegistryConfig{
		StaleTimeout: cfg.StaleTimeout,
		LostTimeout:  cfg.LostTimeout,
	}, func(e Event) {
		select {
		case eventCh <- e:
		default:
			// Non-blocking drop if consumer buffer full
		}
	})

	return &Discovery{
		cfg:      cfg,
		registry: reg,
		events:   eventCh,
		stopChan: make(chan struct{}),
	}, nil
}

// Events returns the channel receiving device discovery events.
func (d *Discovery) Events() <-chan Event {
	return d.events
}

// Registry returns the underlying device registry.
func (d *Discovery) Registry() *DeviceRegistry {
	return d.registry
}

// Start launches the mDNS server, registers local advertisement (if configured),
// and begins browsing for remote _phonebridge._tcp peers.
func (d *Discovery) Start(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return fmt.Errorf("discovery manager is closed")
	}
	if d.server != nil {
		return fmt.Errorf("discovery already started")
	}

	var p4 *ipv4.PacketConn
	var p6 *ipv6.PacketConn

	if d.cfg.CustomPacketConn4 != nil || d.cfg.CustomPacketConn6 != nil {
		p4 = d.cfg.CustomPacketConn4
		p6 = d.cfg.CustomPacketConn6
	} else {
		addr4, err := net.ResolveUDPAddr("udp4", mdns.DefaultAddressIPv4)
		if err == nil {
			if l4, err := net.ListenUDP("udp4", addr4); err == nil {
				p4 = ipv4.NewPacketConn(l4)
				_ = p4.SetMulticastLoopback(true)
			}
		}

		addr6, err := net.ResolveUDPAddr("udp6", mdns.DefaultAddressIPv6)
		if err == nil {
			if l6, err := net.ListenUDP("udp6", addr6); err == nil {
				p6 = ipv6.NewPacketConn(l6)
				_ = p6.SetMulticastLoopback(true)
			}
		}
	}

	if p4 == nil && p6 == nil {
		return fmt.Errorf("failed to bind any multicast UDP connection (IPv4 or IPv6)")
	}

	opts := []mdns.ServerOption{}
	if d.cfg.IncludeLoopback {
		opts = append(opts, mdns.WithIncludeLoopback(true))
	}

	srv, err := mdns.NewServer(p4, p6, opts...)
	if err != nil {
		return fmt.Errorf("mdns: create server: %w", err)
	}
	d.server = srv

	// Register local service if port and device ID are configured
	if d.cfg.Port > 0 && d.cfg.DeviceID != "" {
		instanceName := d.cfg.InstanceName
		if instanceName == "" {
			deviceIDShort := d.cfg.DeviceID
			if len(deviceIDShort) > 16 {
				deviceIDShort = deviceIDShort[:16]
			}
			instanceName = fmt.Sprintf("PhoneBridge-%s", deviceIDShort)
		}
		if len(instanceName) > 63 {
			instanceName = instanceName[:63]
		}

		txt := []mdns.TXTEntry{
			mdns.NewTXTString("id", d.cfg.DeviceID),
			mdns.NewTXTString("name", d.cfg.DeviceName),
			mdns.NewTXTString("model", d.cfg.Model),
			mdns.NewTXTString("v", d.cfg.Version),
			mdns.NewTXTString("caps", strings.Join(d.cfg.Capabilities, ",")),
			mdns.NewTXTString("state", d.cfg.State),
		}

		inst := mdns.ServiceInstance{
			Instance: instanceName,
			Service:  ServiceType,
			Domain:   ServiceDomain,
			Port:     d.cfg.Port,
			Text:     txt,
		}

		if err := srv.Register(inst); err != nil {
			_ = srv.Close()
			return fmt.Errorf("mdns: register service: %w", err)
		}
	}

	// Listen for discovered peers
	srv.OnServiceDiscovered(func(ev mdns.ServiceEvent) {
		d.handleDiscoveredService(ev)
	})

	// Track the browse lifetime so the refresh loop can restart the session.
	// The initial browse runs on a cancelable child of ctx so RestartBrowse
	// can retire exactly one session per restart (no leaked browse loops).
	d.browseLifetime = ctx
	sessionCtx, cancel := context.WithCancel(ctx)
	d.browseCancel = cancel

	// Begin browsing
	if err := srv.Browse(sessionCtx, ServiceType); err != nil {
		cancel()
		_ = srv.Close()
		return fmt.Errorf("mdns: browse: %w", err)
	}

	// Start background sweep for stale devices and the browse refresh loop.
	go d.sweepLoop()
	go d.refreshLoop()

	return nil
}

// handleDiscoveredService parses incoming DNS-SD ServiceEvent records into a Device.
func (d *Discovery) handleDiscoveredService(ev mdns.ServiceEvent) {
	meta := make(map[string]string)
	for _, entry := range ev.Instance.Text {
		if entry.Key != "" {
			meta[entry.Key] = string(entry.Value)
		}
	}

	deviceID := meta["id"]
	if deviceID == "" {
		// Fallback to instance name if id tag is missing
		deviceID = ev.Instance.Instance
	}

	// Filter out own advertisement if seen
	if d.cfg.DeviceID != "" && deviceID == d.cfg.DeviceID {
		return
	}

	deviceName := meta["name"]
	if deviceName == "" {
		deviceName = ev.Instance.Instance
	}
	model := meta["model"]
	version := meta["v"]
	state := meta["state"]

	var caps []string
	if cStr := meta["caps"]; cStr != "" {
		for _, c := range strings.Split(cStr, ",") {
			cTrim := strings.TrimSpace(c)
			if cTrim != "" {
				caps = append(caps, cTrim)
			}
		}
	}

	var addrs []netip.Addr
	if ev.Addr.IsValid() {
		// Keep the zone pion/mdns attached for link-local v6 — without it the
		// address is not dialable. Zone carries through Device.Addresses.
		addrs = append(addrs, ev.Addr)
	}

	device := Device{
		ID:           deviceID,
		Name:         deviceName,
		Model:        model,
		Addresses:    addrs,
		Port:         ev.Instance.Port,
		Version:      version,
		Capabilities: caps,
		State:        state,
		Metadata:     meta,
	}

	d.registry.Upsert(device)
}

// refreshLoop periodically restarts the mDNS browse session so that peers
// re-emit their service records into the registry.
//
// Why this is needed: pion/mdns's browseSession keeps a per-session "seen"
// map and only emits a service event when a record is new or has changed.
// A steady-state peer (e.g. an Android NSD responder, which only announces
// on registration and answers identical responses to identical queries)
// therefore produces no further events after the first resolution, and any
// consumer relying on events to keep LastSeen fresh sees the peer go stale
// even though it is alive and reachable.
//
// Restarting the browse session (cancel the session context, then Browse
// again) starts with an empty "seen" map, so the next query round re-emits
// every peer and refreshes the registry. This is an active-query strategy
// (RFC 6762 §5.2: queriers SHOULD re-query to maintain known records) and
// changes nothing about the advertisement, protocol, or security model.
func (d *Discovery) refreshLoop() {
	ticker := time.NewTicker(d.cfg.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.stopChan:
			return
		case <-ticker.C:
			if err := d.RestartBrowse(); err != nil {
				// Transient errors (e.g. connection closed during shutdown)
				// are expected; keep looping until stopChan fires.
				continue
			}
		}
	}
}

// RestartBrowse cancels the current browse session and starts a new one,
// forcing known peers to be re-emitted (refreshing LastSeen in the registry).
func (d *Discovery) RestartBrowse() error {
	d.mu.Lock()
	if d.closed || d.server == nil {
		d.mu.Unlock()
		return fmt.Errorf("discovery: cannot restart browse: server not running")
	}
	prevCancel := d.browseCancel
	ctx := d.browseLifetime
	d.mu.Unlock()

	// Stop the old session first so its browseLoop exits and unregisters.
	if prevCancel != nil {
		prevCancel()
	}

	d.mu.Lock()
	if d.closed || d.server == nil {
		d.mu.Unlock()
		return fmt.Errorf("discovery: cannot restart browse: server not running")
	}
	newCtx, newCancel := context.WithCancel(ctx)
	d.browseCancel = newCancel
	srv := d.server
	d.mu.Unlock()

	return srv.Browse(newCtx, ServiceType)
}

// sweepLoop periodically sweeps the registry to flag stale or lost devices.
func (d *Discovery) sweepLoop() {
	ticker := time.NewTicker(d.cfg.SweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.stopChan:
			return
		case now := <-ticker.C:
			d.registry.Sweep(now)
		}
	}
}

// Close stops browsing, unregisters services, and terminates the background sweep.
func (d *Discovery) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil
	}
	d.closed = true
	close(d.stopChan)

	var err error
	if d.browseCancel != nil {
		d.browseCancel()
	}
	if d.server != nil {
		err = d.server.Close()
		d.server = nil
	}
	return err
}
