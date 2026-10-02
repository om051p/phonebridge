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
	"golang.org/x/net/dns/dnsmessage"
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
	// LANReady, when set, delays creation of the mDNS server until it reports
	// true. This matters because pion/mdns joins its multicast groups when the
	// server is created: a server created before a LAN interface exists (the
	// daemon starting at boot, ahead of Wi-Fi's address assignment) joins only
	// loopback and never advertises to the LAN afterwards — with no error,
	// because the bind itself succeeded. A nil LANReady binds immediately.
	LANReady func() bool
	// NetworkWaitInterval is how often LANReady is re-checked while the
	// network comes up. Defaults to 2s.
	NetworkWaitInterval time.Duration
	// Logf, when set, receives lifecycle-only diagnostics (waiting for the
	// network, mDNS started, mDNS start failure). Discovery never logs
	// clipboard or user payloads.
	Logf func(format string, args ...any)
	// Announcements sends unsolicited DNS-SD announcements for the local
	// service: the RFC 6762 §8.3 pair at startup, then one every
	// AnnounceInterval. This is what makes the node discoverable by peers that
	// are not currently asking: Android's platform resolver defers its own
	// browse query while another host asks the same question, and pion's browse
	// asks once a second forever, so a responder that only answers queries is
	// invisible to a phone (the Devices tab lists no computer). Announcing is
	// the same advertisement the responder already serves, sent unprompted.
	Announcements bool
	// AnnounceInterval is the gap between periodic announcements once the
	// startup pair has been sent. Defaults to 30s.
	AnnounceInterval time.Duration
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
	pendingStart   bool
	browseLifetime context.Context
	browseCancel   context.CancelFunc

	// p4/p6 are the sockets the mDNS server reads from. Announcements are
	// written through the same sockets so they carry the mDNS source address a
	// peer expects.
	p4 *ipv4.PacketConn
	p6 *ipv6.PacketConn

	// announceTargets, when set, replaces the LAN interface scan used to pick
	// announcement destinations (tests point announcements at their own
	// socket).
	announceTargets func() []announceTarget
}

// announceGroup returns the RFC 6762 §3 multicast destination for mDNS
// traffic: 224.0.0.251 for IPv4, ff02::fb for IPv6.
//
// pion's DefaultAddressIPv4/IPv6 are 224.0.0.0 and ff02:: — the all-hosts and
// all-nodes addresses, not the mDNS groups — so they must never be an announce
// destination: the write succeeds, nothing errors, and every responder that
// joined 224.0.0.251 (Android's platform stack included) hears nothing.
func announceGroup(addr netip.Addr) *net.UDPAddr {
	if addr.Is4() {
		return &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}
	}

	return &net.UDPAddr{IP: net.ParseIP("ff02::fb"), Port: 5353}
}

// announceTarget is one destination an announcement is written to: the
// multicast group of a usable interface, carrying that interface's own address
// in the A/AAAA record.
type announceTarget struct {
	addr    *net.UDPAddr
	ifIndex int
	source  netip.Addr
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
	if cfg.NetworkWaitInterval <= 0 {
		cfg.NetworkWaitInterval = 2 * time.Second
	}
	if cfg.AnnounceInterval <= 0 {
		cfg.AnnounceInterval = 30 * time.Second
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
//
// When Config.LANReady is set and reports false, Start returns successfully
// without creating the server and keeps re-checking in the background: the
// server is created once a usable LAN interface exists. See Config.LANReady
// for why creating it early would silently freeze discovery on loopback.
func (d *Discovery) Start(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return fmt.Errorf("discovery manager is closed")
	}
	if d.server != nil || d.pendingStart {
		return fmt.Errorf("discovery already started")
	}

	// Remember the browse lifetime before any deferral: the deferred start and
	// RestartBrowse both parent their browse sessions on it.
	d.browseLifetime = ctx

	if d.cfg.LANReady != nil && !d.cfg.LANReady() {
		d.pendingStart = true
		d.logf("discovery: no usable LAN interface yet; deferring mDNS start so the server joins the LAN multicast groups once the network is up")
		go d.bindWhenNetworkReady(ctx)
		return nil
	}

	return d.startLocked(ctx)
}

// bindWhenNetworkReady waits for Config.LANReady and then starts the mDNS
// server. It exits when the network never arrives and the manager closes.
func (d *Discovery) bindWhenNetworkReady(ctx context.Context) {
	ticker := time.NewTicker(d.cfg.NetworkWaitInterval)
	defer ticker.Stop()

	for {
		if d.cfg.LANReady() {
			d.mu.Lock()
			if d.closed {
				d.mu.Unlock()
				return
			}
			err := d.startLocked(ctx)
			d.pendingStart = false
			d.mu.Unlock()
			if err == nil {
				d.logf("discovery: LAN interface detected; mDNS advertisement and browse started")
				return
			}
			d.logf("discovery: mDNS start after network came up failed: %v", err)
		}

		select {
		case <-d.stopChan:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// startLocked creates the mDNS server, registers the local advertisement and
// begins browsing. d.mu must be held.
func (d *Discovery) startLocked(ctx context.Context) error {
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

	// A DNS-SD answer carries an SRV record whose target is the service host,
	// and pion/mdns fills that target from the server's configured local names
	// (ServiceInstance.Host defaults to localNames[0]). Registering without one
	// leaves the target empty, so every PTR/SRV answer fails to pack
	// ("SRVResource.Target: name is not in canonical format") and no reply is
	// ever sent: the node listens, browses and reaches its peers, but no peer
	// can browse it back. Advertised hostname is therefore mandatory when
	// registering.
	registering := d.cfg.Port > 0 && d.cfg.DeviceID != ""
	if registering {
		opts = append(opts, mdns.WithLocalNames(d.localHostname()))
	}

	srv, err := mdns.NewServer(p4, p6, opts...)
	if err != nil {
		return fmt.Errorf("mdns: create server: %w", err)
	}
	d.server = srv
	d.p4 = p4
	d.p6 = p6

	// Register local service if port and device ID are configured
	if registering {
		txt := make([]mdns.TXTEntry, 0, 6)
		for _, entry := range d.txtStrings() {
			key, value, _ := strings.Cut(entry, "=")
			txt = append(txt, mdns.NewTXTString(key, value))
		}

		inst := mdns.ServiceInstance{
			Instance: d.serviceInstanceName(),
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

	// The initial browse runs on a cancelable child of ctx so RestartBrowse
	// can retire exactly one session per restart (no leaked browse loops).
	// browseLifetime was set by Start before any deferral.
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

	if d.cfg.Announcements {
		d.logf("discovery: announcing %s unprompted every %s so peers that are not querying can find this node", d.serviceInstanceName(), d.cfg.AnnounceInterval)
		go d.announceLoop(ctx)
	}

	return nil
}

// serviceInstanceName is the DNS-SD instance name this node advertises.
func (d *Discovery) serviceInstanceName() string {
	if d.cfg.InstanceName != "" {
		return d.cfg.InstanceName
	}

	deviceIDShort := d.cfg.DeviceID
	if len(deviceIDShort) > 16 {
		deviceIDShort = deviceIDShort[:16]
	}
	name := fmt.Sprintf("PhoneBridge-%s", deviceIDShort)
	if len(name) > 63 {
		name = name[:63]
	}

	return name
}

// txtStrings is the TXT record of the local advertisement, in key=value form.
// Register and the unsolicited announcements share it so a peer sees the same
// data whichever way it learns the service.
func (d *Discovery) txtStrings() []string {
	return []string{
		"id=" + d.cfg.DeviceID,
		"name=" + d.cfg.DeviceName,
		"model=" + d.cfg.Model,
		"v=" + d.cfg.Version,
		"caps=" + strings.Join(d.cfg.Capabilities, ","),
		"state=" + d.cfg.State,
	}
}

// announceLoop sends the local advertisement unprompted: the RFC 6762 §8.3
// startup pair one second apart, then one announcement per AnnounceInterval so
// a peer that never asks keeps the records fresh instead of ageing them out.
func (d *Discovery) announceLoop(ctx context.Context) {
	d.sendAnnouncements()

	timer := time.NewTimer(announceRepeatDelay)
	defer timer.Stop()
	select {
	case <-d.stopChan:
		return
	case <-ctx.Done():
		return
	case <-timer.C:
	}
	d.sendAnnouncements()

	ticker := time.NewTicker(d.cfg.AnnounceInterval)
	defer ticker.Stop()
	for {
		select {
		case <-d.stopChan:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.sendAnnouncements()
		}
	}
}

// sendAnnouncements writes one announcement per usable interface address.
// Errors are not fatal: a node that cannot announce still answers queries.
func (d *Discovery) sendAnnouncements() {
	targets := d.resolveAnnounceTargets()
	if len(targets) == 0 {
		d.logf("discovery: no interface to announce on yet")
		return
	}

	for _, target := range targets {
		raw, err := d.buildAnnouncementPacket(target.source)
		if err != nil {
			d.logf("discovery: announcement for %s could not be packed: %v", target.source, err)
			continue
		}
		if err := d.writeAnnouncement(target, raw); err != nil {
			d.logf("discovery: announcement to %s failed: %v", target.addr, err)
		}
	}
	d.logf("discovery: announced %s on %d interface(s)", d.serviceInstanceName(), len(targets))
}

// resolveAnnounceTargets picks where announcements go: the overlay when a test
// supplied one, otherwise the multicast group of every usable LAN interface.
func (d *Discovery) resolveAnnounceTargets() []announceTarget {
	if d.announceTargets != nil {
		return d.announceTargets()
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var targets []announceTarget
	for _, iface := range ifaces {
		if iface.Name == "lo" || iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			addr, ok := routableLANAddr(a)
			if !ok {
				continue
			}
			targets = append(targets, announceTarget{
				addr:    announceGroup(addr),
				ifIndex: iface.Index,
				source:  addr,
			})
		}
	}

	return targets
}

// writeAnnouncement sends one prepared announcement to one destination.
func (d *Discovery) writeAnnouncement(target announceTarget, raw []byte) error {
	if target.addr == nil {
		return fmt.Errorf("no destination")
	}

	if target.addr.IP.To4() != nil {
		if d.p4 == nil {
			return fmt.Errorf("no IPv4 mDNS socket")
		}
		if err := d.setAnnounceInterface4(target.ifIndex); err != nil {
			return err
		}
		_, err := d.p4.WriteTo(raw, nil, target.addr)
		return err
	}

	if d.p6 == nil {
		return fmt.Errorf("no IPv6 mDNS socket")
	}
	if err := d.setAnnounceInterface6(target.ifIndex); err != nil {
		return err
	}
	_, err := d.p6.WriteTo(raw, nil, target.addr)
	return err
}

// setAnnounceInterface4 selects the outgoing interface on the mDNS socket.
//
// The socket is bound to the multicast group address, so without this the
// kernel picks the source address for itself and the announcement leaves with
// an address no peer can use — silent, because the write still succeeds. pion
// selects the interface the same way before every answer it sends.
func (d *Discovery) setAnnounceInterface4(ifIndex int) error {
	if ifIndex <= 0 {
		return nil
	}
	ifi, err := net.InterfaceByIndex(ifIndex)
	if err != nil {
		return fmt.Errorf("interface %d: %w", ifIndex, err)
	}
	if err := d.p4.SetMulticastInterface(ifi); err != nil {
		return fmt.Errorf("set multicast interface %d: %w", ifIndex, err)
	}
	return nil
}

// setAnnounceInterface6 is the IPv6 counterpart of setAnnounceInterface4.
func (d *Discovery) setAnnounceInterface6(ifIndex int) error {
	if ifIndex <= 0 {
		return nil
	}
	ifi, err := net.InterfaceByIndex(ifIndex)
	if err != nil {
		return fmt.Errorf("interface %d: %w", ifIndex, err)
	}
	if err := d.p6.SetMulticastInterface(ifi); err != nil {
		return fmt.Errorf("set multicast interface %d: %w", ifIndex, err)
	}
	return nil
}

// buildAnnouncementPacket packs an unsolicited DNS-SD announcement for the
// local service, resolving to announceAddr (RFC 6762 §8.3). TTLs mirror the
// ones the pion responder puts on the same records (RFC 6762 §10): 4500 for
// the shared PTR and the TXT, 120 for the host-scoped SRV and address records.
func (d *Discovery) buildAnnouncementPacket(announceAddr netip.Addr) ([]byte, error) {
	if !announceAddr.IsValid() {
		return nil, fmt.Errorf("announce: no address to advertise")
	}

	service, err := dnsmessage.NewName(ServiceType + "." + ServiceDomain + ".")
	if err != nil {
		return nil, err
	}
	instance, err := dnsmessage.NewName(d.serviceInstanceName() + "." + ServiceType + "." + ServiceDomain + ".")
	if err != nil {
		return nil, err
	}
	host, err := dnsmessage.NewName(d.localHostname() + ".")
	if err != nil {
		return nil, err
	}

	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{Response: true, Authoritative: true})
	b.EnableCompression()

	if err := b.StartAnswers(); err != nil {
		return nil, err
	}
	if err := b.PTRResource(
		dnsmessage.ResourceHeader{Name: service, Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET, TTL: announceBrowseTTL},
		dnsmessage.PTRResource{PTR: instance},
	); err != nil {
		return nil, err
	}

	if err := b.StartAdditionals(); err != nil {
		return nil, err
	}
	if err := b.SRVResource(
		dnsmessage.ResourceHeader{Name: instance, Type: dnsmessage.TypeSRV, Class: announceFlushClass, TTL: announceResponseTTL},
		dnsmessage.SRVResource{Port: d.cfg.Port, Target: host},
	); err != nil {
		return nil, err
	}
	if err := b.TXTResource(
		dnsmessage.ResourceHeader{Name: instance, Type: dnsmessage.TypeTXT, Class: announceFlushClass, TTL: announceBrowseTTL},
		dnsmessage.TXTResource{TXT: d.txtStrings()},
	); err != nil {
		return nil, err
	}

	if announceAddr.Is4() {
		if err := b.AResource(
			dnsmessage.ResourceHeader{Name: host, Type: dnsmessage.TypeA, Class: announceFlushClass, TTL: announceResponseTTL},
			dnsmessage.AResource{A: announceAddr.As4()},
		); err != nil {
			return nil, err
		}
	} else {
		if err := b.AAAAResource(
			dnsmessage.ResourceHeader{Name: host, Type: dnsmessage.TypeAAAA, Class: announceFlushClass, TTL: announceResponseTTL},
			dnsmessage.AAAAResource{AAAA: announceAddr.As16()},
		); err != nil {
			return nil, err
		}
	}

	return b.Finish()
}

// logf forwards a lifecycle diagnostic when a sink is configured.
func (d *Discovery) logf(format string, args ...any) {
	if d.cfg.Logf != nil {
		d.cfg.Logf(format, args...)
	}
}

const (
	// announceBrowseTTL matches pion's browseTTL: shared browse-scoped records
	// are cached for 75 minutes (RFC 6762 §10).
	announceBrowseTTL = 4500
	// announceResponseTTL matches pion's responseTTL: host-scoped records are
	// cached for two minutes (RFC 6762 §10).
	announceResponseTTL = 120
	// announceRepeatDelay is the RFC 6762 §8.3 gap between the first two
	// announcements.
	announceRepeatDelay = time.Second
	// announceFlushClass is DNS class IN with the cache-flush bit set, which is
	// how a responder marks the records it alone owns (RFC 6762 §10.2).
	announceFlushClass = dnsmessage.ClassINET | 0x8000
)

// interfaceSnapshot captures the interface properties that decide whether
// mDNS can reach the LAN through it.
type interfaceSnapshot struct {
	Name      string
	Up        bool
	Multicast bool
	Addresses int
}

// hasUsableLANInterface reports whether any interface in the snapshot can
// carry mDNS traffic: it must be up, multicast-capable, carry at least one
// routable (non-loopback, non-link-local) address, and not be loopback.
func hasUsableLANInterface(snaps []interfaceSnapshot) bool {
	for _, snap := range snaps {
		if snap.Name == "lo" || !snap.Up || !snap.Multicast || snap.Addresses == 0 {
			continue
		}
		return true
	}
	return false
}

// LANInterfaceAvailable reports whether the machine currently has an
// interface that mDNS can use to reach a LAN. Daemons pass it as
// Config.LANReady so a start-at-boot process does not create its mDNS server
// before the network is up.
func LANInterfaceAvailable() bool {
	return hasUsableLANInterface(snapshotInterfaces())
}

// snapshotInterfaces reads the current interfaces. An unreadable interface
// list is reported as no interfaces, so the gate keeps waiting instead of
// binding to a network it cannot see.
func snapshotInterfaces() []interfaceSnapshot {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	snaps := make([]interfaceSnapshot, 0, len(ifaces))
	for _, iface := range ifaces {
		snap := interfaceSnapshot{
			Name:      iface.Name,
			Up:        iface.Flags&net.FlagUp != 0,
			Multicast: iface.Flags&net.FlagMulticast != 0,
		}
		if addrs, err := iface.Addrs(); err == nil {
			for _, addr := range addrs {
				if _, ok := routableLANAddr(addr); ok {
					snap.Addresses++
				}
			}
		}
		snaps = append(snaps, snap)
	}
	return snaps
}

// routableLANAddr reports whether an interface address can carry LAN traffic,
// returning the parsed address when it can. Loopback and IPv6 link-local
// addresses are excluded: a WLAN that has not finished DHCP already carries
// fe80:: addresses, and treating those as "ready" would bind the server into
// exactly the pre-DHCP window the gate exists to avoid.
func routableLANAddr(addr net.Addr) (netip.Addr, bool) {
	host := addr.String()
	if i := strings.IndexByte(host, '/'); i >= 0 {
		host = host[:i]
	}
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}

	ip, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsMulticast() || ip.IsUnspecified() {
		return netip.Addr{}, false
	}

	return ip, true
}

// localHostname is the canonical mDNS hostname (RFC 6762 §3, so it carries a
// trailing dot by the time it reaches the wire) that this node's advertised
// service instance resolves to. It is derived from the device identity rather
// than the OS hostname so a PhoneBridge advertisement can never collide with a
// system responder (avahi, systemd-resolved) claiming the machine's own name.
func (d *Discovery) localHostname() string {
	short := d.cfg.DeviceID
	if len(short) > 16 {
		short = short[:16]
	}

	return fmt.Sprintf("phonebridge-%s.%s", short, ServiceDomain)
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
