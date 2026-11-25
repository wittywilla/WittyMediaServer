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
	known = append(known, ma.StringCast("/ip4/127.0.0.1/tcp/9796"), ma.StringCast("/ip4/192.168.1.5/tcp/9796"))
	got := addrStrings(keptAddrs(known, []ma.Multiaddr{ma.StringCast(relay), ma.StringCast("/ip4/10.77.0.2/tcp/9796")}, maxKeptAddrs))

	if len(got) != maxKeptAddrs {
		t.Fatalf("kept %d, want %d: %v", len(got), maxKeptAddrs, got)
	}
	if got[len(got)-1] != relay {
		t.Fatalf("the invite's circuit isn't kept: %v", got)
	}
	for _, a := range got {
		if strings.HasPrefix(a, "/ip4/127.") {
			t.Fatalf("loopback kept ahead of other addresses: %v", got)
		}
	}
	if strings.Count(strings.Join(got, " "), "10.77.0.2/") != 1 {
		t.Fatalf("an address is kept twice: %v", got)
	}
}

func TestKeptAddrsCapsCircuits(t *testing.T) {
	var known []ma.Multiaddr
	for i := range 4 {
		known = append(known, ma.StringCast(fmt.Sprintf("/ip4/198.51.100.%d/tcp/4001/p2p/12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo/p2p-circuit", i+1)))
	}
	known = append(known, ma.StringCast("/ip4/127.0.0.1/tcp/9796"))
	got := addrStrings(keptAddrs(known, nil, maxKeptAddrs))
	if len(got) != 1+keptCircuits || got[0] != "/ip4/127.0.0.1/tcp/9796" {
		t.Fatalf("got %v", got)
	}
}
