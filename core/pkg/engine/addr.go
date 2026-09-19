package engine

import (
	"fmt"
	"net/netip"
)

// BestDialAddr picks the most dialable address for a discovered device.
//
// Preference order (first valid wins):
//  1. IPv4 — universally routable on the LAN, no bracket/zone concerns;
//  2. global unicast IPv6;
//  3. zoned IPv6 link-local (fe80::/10) — dialable only because the mDNS
//     library preserves the source interface as the %zone.
//
// This exists because discovery records typically carry one IPv4 and one
// link-local v6 address, and picking the first entry blindly (the previous
// behaviour) could select an address the dialer cannot use.
func BestDialAddr(addrs []netip.Addr) (netip.Addr, bool) {
	var v6global, v6link netip.Addr
	for _, a := range addrs {
		if !a.IsValid() {
			continue
		}
		if a.Is4() {
			return a, true
		}
		switch {
		case a.Is6() && a.Zone() == "" && a.IsGlobalUnicast():
			if !v6global.IsValid() {
				v6global = a
			}
		case a.Is6() && a.Zone() != "":
			if !v6link.IsValid() {
				v6link = a
			}
		}
	}
	if v6global.IsValid() {
		return v6global, true
	}
	if v6link.IsValid() {
		return v6link, true
	}
	return netip.Addr{}, false
}

// DialHost renders an address for use in a host:port URL. IPv6 literals must
// be bracketed or url.Parse and net.Dial both fail ("too many colons" /
// "invalid argument"). netip.Addr.String() already emits the %zone form that
// Go dialers accept inside brackets for link-locals.
func DialHost(a netip.Addr) string {
	if a.Is4() || a.Is4In6() {
		return a.String()
	}
	return "[" + a.String() + "]"
}

// Endpoint formats host:port from a discovered address list.
func Endpoint(addrs []netip.Addr, port uint16) (string, bool) {
	a, ok := BestDialAddr(addrs)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%s:%d", DialHost(a), port), true
}
