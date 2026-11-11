// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"net"
	"net/netip"
	"testing"
)

// Addresses that lead back to this machine by way of IPv6's old ways of writing IPv4.
func TestInternetAddrOldForms(t *testing.T) {
	for _, s := range []string{"::127.0.0.1", "::ffff:0:7f00:1", "::192.168.1.1", "::ffff:0:c0a8:101", "::8.8.8.8"} {
		if internetAddr(net.ParseIP(s)) {
			t.Errorf("%s counts as the internet", s)
		}
	}
	if !internetAddr(net.ParseIP("2606:4700:4700::1111")) {
		t.Error("a real address was refused")
	}
}

// A feed's host learns this server's IPv6 address, and so its whole network: the router, and the
// server itself. Those are global addresses, but they aren't the internet.
func TestSafeClientRefusesThisMachinesNetworks(t *testing.T) {
	old := machineNets
	machineNets = func() []netip.Prefix {
		return []netip.Prefix{netip.MustParsePrefix("2a02:1234:5678:1::/64"), netip.MustParsePrefix("203.0.113.50/32"), netip.MustParsePrefix("198.51.100.0/24")}
	}
	defer func() { machineNets = old }()
	for addr, want := range map[string]bool{
		"[2a02:1234:5678:1::1]:80":      false,
		"[2a02:1234:5678:1:ffff::9]:80": false,
		"[2a02:1234:5678:2::1]:80":      true,
		"[2606:4700:4700::1111]:80":     true,
		"8.8.8.8:80":                    true,
		"[::ffff:8.8.4.4]:80":           true,
	} {
		err := safeDialer.Control("tcp", addr, nil)
		if (err == nil) != want {
			t.Errorf("%s: %v", addr, err)
		}
	}
}

func TestMachineNetsFromCards(t *testing.T) {
	got := netsOfCards([]net.Addr{
		&net.IPNet{IP: net.ParseIP("2a02:1234:5678:1::5"), Mask: net.CIDRMask(64, 128)},
		&net.IPNet{IP: net.ParseIP("192.168.1.5"), Mask: net.CIDRMask(24, 32)},
		&net.IPNet{IP: net.ParseIP("45.33.32.156"), Mask: net.CIDRMask(24, 32)},
		&net.IPNet{IP: net.ParseIP("8.8.8.8"), Mask: net.CIDRMask(8, 32)},        // a network too wide to be a neighbourhood
		&net.IPNet{IP: net.ParseIP("2606:4700::1"), Mask: net.CIDRMask(16, 128)}, // the same
		&net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
	})
	has := func(s string) bool {
		a := netip.MustParseAddr(s)
		for _, p := range got {
			if p.Contains(a) {
				return true
			}
		}
		return false
	}
	for s, want := range map[string]bool{"2a02:1234:5678:1::99": true, "45.33.32.99": true, "8.8.8.8": true, "8.8.4.4": false, "2606:4700::1": true, "2606:4700::2": false} {
		if has(s) != want {
			t.Errorf("%s: %v in %v", s, has(s), got)
		}
	}
}
