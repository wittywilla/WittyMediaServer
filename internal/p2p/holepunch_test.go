// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
//
// simulated (below) is adapted from quicSimnet in go-libp2p's
// p2p/protocol/holepunch/holepunch_test.go, which is under the MIT license:
// Copyright (c) 2014 Juan Batiz-Benet. See THIRD_PARTY_LICENSES.md.

package p2p

import (
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/libp2p/go-libp2p/p2p/transport/quicreuse"
	"github.com/marcopolo/simnet"
)

// The two tests here run over a network that exists only in this process:
// simnet carries QUIC's packets between made-up public addresses, through a
// router that behaves as a home router's firewall does. A node behind it can
// send out and get answers back, but nothing reaches it uninvited. That is
// what hole punching has to get through, and no packet leaves the machine.

type sourceIP struct{ ip atomic.Pointer[net.IP] }

func (m *sourceIP) PreferredSourceIPForDestination(*net.UDPAddr) (net.IP, error) {
	return *m.ip.Load(), nil
}

// simulated puts a node's QUIC on the simulated network, reachable from
// outside or behind the firewall.
func simulated(reachable bool, router *simnet.SimpleFirewallRouter) libp2p.Option {
	m := &sourceIP{}
	return libp2p.QUICReuse(
		quicreuse.NewConnManager,
		quicreuse.OverrideSourceIPSelector(func() (quicreuse.SourceIPSelector, error) { return m, nil }),
		quicreuse.OverrideListenUDP(func(_ string, address *net.UDPAddr) (net.PacketConn, error) {
			m.ip.Store(&address.IP)
			if reachable {
				router.SetAddrPubliclyReachable(address)
			}
			c := simnet.NewSimConn(address)
			c.SetUpPacketReceiver(router)
			router.AddNode(address, c)
			return c, nil
		}))
}

// simRelay is an ordinary public relay: it passes on 128 kB per connection
// and no more, as the public network's nodes do.
func simRelay(t *testing.T, router *simnet.SimpleFirewallRouter) string {
	t.Helper()
	h, err := libp2p.New(
		simulated(true, router),
		libp2p.ListenAddrStrings("/ip4/1.2.0.1/udp/8000/quic-v1"),
		libp2p.DisableRelay(),
		libp2p.DisableMetrics(),
		libp2p.ResourceManager(&network.NullResourceManager{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := relayv2.New(h); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	return "/ip4/1.2.0.1/udp/8000/quic-v1/p2p/" + h.ID().String()
}

func TestHolePunchTurnsARelayedConnectionIntoADirectOne(t *testing.T) {
	router := &simnet.SimpleFirewallRouter{}
	relay := simRelay(t, router)
	devs := newDevList(testID(t, 2))
	n := startNode(t, 1, devs, func(c *NodeConfig) {
		c.Listen = []string{"/ip4/2.2.0.2/udp/8001/quic-v1"}
		c.Layers = Layers{Relay: true, HolePunch: true}
		c.StaticRelays = []string{relay}
		c.Options = []libp2p.Option{simulated(false, router), libp2p.ForceReachabilityPrivate()}
		c.PunchTimeout = 500 * time.Millisecond
	})
	// The server takes a place on the relay and says so in its addresses.
	var circuit string
	for i := 0; circuit == "" && i < 200; i++ {
		for _, a := range n.Addrs() {
			if strings.Contains(a, "p2p-circuit") {
				circuit = a
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if circuit == "" {
		t.Fatalf("the server never announced a relayed address: %v", n.Addrs())
	}
	if st := n.Status(); len(st.Relays) != 1 {
		t.Fatalf("relays in the status: %+v", st.Relays)
	}

	states := make(chan State, 64)
	c := startClient(t, 2, n, func(c *ClientConfig) {
		c.Hints = []string{circuit} // behind its firewall, the server can't be dialled directly
		c.Listen = []string{"/ip4/2.2.0.1/udp/8000/quic-v1"}
		c.Layers = Layers{Relay: true, HolePunch: true}
		c.Options = []libp2p.Option{simulated(false, router), libp2p.ForceReachabilityPrivate()}
		c.UpgradeWait = 20 * time.Second
		c.PunchTimeout = 500 * time.Millisecond
		// libp2p asks once, as the relayed connection arrives; if this end
		// wasn't ready for it yet, the app asks again.
		c.PunchWaits = []time.Duration{3 * time.Second}
		c.OnState = func(s State) { states <- s }
	})
	// The first request waits for the direct connection instead of being sent
	// through a relay that would cut it off.
	code, body, err := get(t, c, "/big")
	if err != nil || code != 200 || len(body) != len(bigBody) {
		t.Fatalf("a megabyte after hole punching: %d, %d bytes, %v", code, len(body), err)
	}
	s := c.State()
	if s.Path != PathDirect || s.Transport != "quic" || !strings.Contains(s.Addr, "/ip4/2.2.0.2/") {
		t.Fatalf("state after hole punching: %+v", s)
	}
	// It went relay first, then direct: that order is the point.
	var paths []string
	for len(states) > 0 {
		paths = append(paths, (<-states).Path)
	}
	joined := strings.Join(paths, " ")
	if !strings.Contains(joined, PathRelayed+" "+PathDirect) && !strings.HasSuffix(joined, PathDirect) {
		t.Fatalf("paths the app was told: %v", paths)
	}
	code, hello, err := get(t, c, "/hello")
	if err != nil || code != 200 || !strings.Contains(hello, "relayed=false") || !strings.Contains(hello, "remote=2.2.0.1:0") {
		t.Fatalf("over the punched connection the server saw: %d %q %v", code, hello, err)
	}
	// The relayed connection is let go once the direct one stands.
	c.tidy()
	for _, conn := range c.host.Network().ConnsToPeer(c.server) {
		if conn.Stat().Limited && !conn.IsClosed() {
			t.Errorf("a relayed connection was kept: %s", conn.RemoteMultiaddr())
		}
	}
}

func TestWithoutHolePunchingAPublicRelayLeadsNowhere(t *testing.T) {
	router := &simnet.SimpleFirewallRouter{}
	relay := simRelay(t, router)
	devs := newDevList(testID(t, 2))
	n := startNode(t, 1, devs, func(c *NodeConfig) {
		c.Listen = []string{"/ip4/2.2.0.2/udp/8001/quic-v1"}
		c.Layers = Layers{Relay: true}
		c.StaticRelays = []string{relay}
		c.Options = []libp2p.Option{simulated(false, router), libp2p.ForceReachabilityPrivate()}
	})
	c := startClient(t, 2, n, func(c *ClientConfig) {
		c.Hints = []string{relay + "/p2p-circuit"}
		c.Listen = []string{"/ip4/2.2.0.1/udp/8000/quic-v1"}
		c.Layers = Layers{Relay: true}
		c.Options = []libp2p.Option{simulated(false, router), libp2p.ForceReachabilityPrivate()}
	})
	waitPath(t, c, PathRelayed)
	if _, _, err := get(t, c, "/hello"); err == nil {
		t.Fatal("a request went through a relay that only passes on enough for setting up")
	}
	if s := c.State(); s.Path != PathRelayed || s.Community {
		t.Fatalf("state: %+v", s)
	}
}
