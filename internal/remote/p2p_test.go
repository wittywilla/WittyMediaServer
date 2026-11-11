// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package remote

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

// A request that came over a peer-to-peer connection is away from home, and
// nothing about it can say otherwise: not the address its connection really
// has, not the address it is counted under, not a forwarded header, and not
// the owner's own lists of home networks and trusted proxies.
func TestP2PIsNeverAtHome(t *testing.T) {
	everything, _ := ParseNets("10.0.0.0/8, 127.0.0.0/8, 192.168.0.0/16, 198.51.100.0/24, 192.0.2.0/24, 2001:db8::/32, ::1")
	policies := map[string]*Policy{
		"none":                     nil,
		"plain":                    {},
		"everything is home":       {Home: everything},
		"everything is a proxy":    {Trusted: everything},
		"home and a trusted proxy": {Home: everything, Trusted: everything},
	}
	marks := []P2PConn{
		{}, // no address at all
		{Addr: netip.MustParseAddr("127.0.0.1")},
		{Addr: netip.MustParseAddr("::1")},
		{Addr: netip.MustParseAddr("::ffff:127.0.0.1")},
		{Addr: netip.MustParseAddr("192.168.1.10")},
		{Addr: netip.MustParseAddr("10.2.3.4")},
		{Addr: netip.MustParseAddr("100.100.1.1")}, // a VPN's range
		{Addr: netip.MustParseAddr("fe80::1")},
		{Addr: netip.MustParseAddr("198.51.100.7"), Device: "12D3KooWdevice", Known: true},
		{Addr: netip.MustParseAddr("2001:db8:1:2::1"), Relayed: true},
	}
	remotes := []string{"127.0.0.1:5000", "[::1]:5000", "192.168.1.10:5000", "198.51.100.7:0", "nonsense", ""}
	for name, p := range policies {
		for _, m := range marks {
			for _, ra := range remotes {
				for _, xff := range []string{"", "127.0.0.1", "192.168.1.2, 127.0.0.1"} {
					r := httptest.NewRequest("GET", "/api/status", nil)
					r.RemoteAddr = ra
					if xff != "" {
						r.Header.Set("X-Forwarded-For", xff)
						r.Header.Set("X-Forwarded-Host", "home.example")
					}
					r = r.WithContext(WithP2P(r.Context(), m))
					c := p.Resolve(r)
					if c.Where != Internet || c.Where.AtHome() || !c.P2P {
						t.Fatalf("policy %q, mark %+v, remote %q, forwarded %q: counted as %s", name, m, ra, xff, c.Where)
					}
					if c.Proxied || c.ForwardedHost != "" {
						t.Fatalf("policy %q: a forwarded header was believed on a peer-to-peer connection", name)
					}
					// The address it is counted under is never one a later check
					// could take for home, even with no policy at all.
					if (*Policy)(nil).Classify(c.IP) != Internet || !c.IP.IsValid() {
						t.Fatalf("mark %+v is counted under %v, which reads as %s", m, c.IP, (*Policy)(nil).Classify(c.IP))
					}
					if !c.Secure {
						t.Fatal("a peer-to-peer connection is encrypted, and must not be refused as plain")
					}
				}
			}
		}
	}
	// Without the mark, the same request from this machine is this machine:
	// the mark, and only the mark, makes the difference.
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:5000"
	if c := (&Policy{}).Resolve(r); c.Where != Machine || c.P2P {
		t.Fatalf("an ordinary local request: %+v", c)
	}
	m := P2PConn{Addr: netip.MustParseAddr("198.51.100.7"), Device: "dev", Relayed: true}
	c := (&Policy{}).Resolve(r.WithContext(WithP2P(r.Context(), m)))
	if c.IP != m.Addr || c.P2PDevice != "dev" || !c.P2PRelayed {
		t.Fatalf("a public address and the device should be passed on: %+v", c)
	}
}
