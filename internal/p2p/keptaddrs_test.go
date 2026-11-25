// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"fmt"
	"strings"
	"testing"

	ma "github.com/multiformats/go-multiaddr"
)

// A server with many addresses must not push the invite's relay circuit out
// of the hints kept for next time: from away it is often the only way in.
func TestKeptAddrsKeepsTheInvitesCircuit(t *testing.T) {
	const relay = "/ip4/203.0.113.9/tcp/4001/p2p/12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo/p2p-circuit"
	var known []ma.Multiaddr
	for i := range 10 {
		known = append(known, ma.StringCast(fmt.Sprintf("/ip4/10.77.%d.2/tcp/9796", i)))
	}
	known = append(known,
		ma.StringCast("/ip4/127.0.0.1/tcp/9796"),
		ma.StringCast("/ip4/172.17.0.2/udp/9796/quic-v1"),
		ma.StringCast("/ip4/192.168.1.5/tcp/9796"),
		ma.StringCast("/ip4/198.51.100.7/udp/9796/quic-v1"),
		ma.StringCast("/dns4/home.example.net/tcp/9796"),
	)
	got := addrStrings(keptAddrs(known, []ma.Multiaddr{ma.StringCast(relay), ma.StringCast("/ip4/10.77.0.2/tcp/9796")}, maxKeptAddrs))
	want := []string{
		relay,
		"/ip4/198.51.100.7/udp/9796/quic-v1",
		"/dns4/home.example.net/tcp/9796",
		"/ip4/192.168.1.5/tcp/9796",
		"/ip4/10.77.0.2/tcp/9796",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got  %v, want %v", got, want)
	}
}

func TestKeptAddrsCaps(t *testing.T) {
	var known []ma.Multiaddr
	for i := range 6 {
		known = append(known, ma.StringCast(fmt.Sprintf("/ip4/198.51.100.%d/tcp/4001/p2p/12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo/p2p-circuit", i+1)))
	}
	for i := range 12 {
		known = append(known, ma.StringCast(fmt.Sprintf("/ip4/203.0.113.%d/tcp/9796", i+1)))
	}
	known = append(known, ma.StringCast("/ip4/127.0.0.1/tcp/9796"))
	got := addrStrings(keptAddrs(known, nil, maxKeptAddrs))
	if len(got) != maxKeptAddrs || strings.Count(strings.Join(got, " "), "p2p-circuit") != keptCircuits {
		t.Fatalf("got %d with %d circuits: %v", len(got), strings.Count(strings.Join(got, " "), "p2p-circuit"), got)
	}
	if strings.Contains(strings.Join(got, " "), "127.0.0.1") {
		t.Fatalf("loopback kept ahead of public addresses: %v", got)
	}
}

func TestPrivateRank(t *testing.T) {
	for addr, want := range map[string]int{
		"/ip4/192.168.0.9/tcp/1": 0, "/ip4/10.1.2.3/tcp/1": 1, "/ip4/172.20.0.2/tcp/1": 2,
		"/ip4/100.64.1.1/tcp/1": 3, "/ip6/fd00::1/tcp/1": 3, "/ip6/fe80::1/tcp/1": 4,
		"/ip4/127.0.0.1/tcp/1": 5, "/ip4/172.32.0.1/tcp/1": -1, "/ip4/8.8.8.8/tcp/1": -1,
		"/dns4/example.net/tcp/1": -1,
	} {
		if got := privateRank(ma.StringCast(addr)); got != want {
			t.Errorf("%s: %d, want %d", addr, got, want)
		}
	}
}
