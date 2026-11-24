// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"context"
	"encoding/hex"
	"net/netip"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/control"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// behindRouter is a server's router, as far as these tests need one: nothing
// reaches the server directly from outside, and it dials nobody but the peers
// named. Connections through a relay are all a device gets.
type behindRouter struct{ allowed map[peer.ID]bool }

func (g behindRouter) InterceptPeerDial(p peer.ID) bool             { return g.allowed[p] }
func (g behindRouter) InterceptAddrDial(peer.ID, ma.Multiaddr) bool { return true }
func (g behindRouter) InterceptAccept(cm network.ConnMultiaddrs) bool {
	return isRelayed(cm.RemoteMultiaddr())
}
func (g behindRouter) InterceptSecured(network.Direction, peer.ID, network.ConnMultiaddrs) bool {
	return true
}
func (g behindRouter) InterceptUpgraded(network.Conn) (bool, control.DisconnectReason) {
	return true, 0
}

// A TV away from home whose server's only way in is the owner's WittyRelay
// joins by its code (#683, F12): it finds the server in the address book, its
// own key is refused by the relay (nobody listed it), and it shows the code
// from the code's own key instead, which the server lists with its relay while
// the code waits. The device's own key is what goes on the list.
func TestJoiningThroughTheServersOwnRelay(t *testing.T) {
	book := newBook(t)
	f, relay := newFakeWittyRelay(t, "the-key-0123456789")
	devs := newDevList()
	var n *Node
	n = startNode(t, 1, devs, ownRelay(relay, "the-key-0123456789", devs), func(c *NodeConfig) {
		c.Layers.DHT = true
		c.Bootstrap = []peer.AddrInfo{book}
		c.Enrol = func(_ context.Context, id string, req EnrolRequest, _ netip.Addr) EnrolResponse {
			if req.Secret != "the-invite-behind-it" || !req.FromJoin {
				return EnrolResponse{Reason: "wrong", Error: "no"}
			}
			devs.set(id, true)
			n.DevicesChanged() // as the server does when it lists a device
			return EnrolResponse{OK: true, Code: "123456", ServerName: "Home"}
		}
		c.Options = append(c.Options, libp2p.ConnectionGater(behindRouter{allowed: map[peer.ID]bool{f.ID(): true, book.ID: true}}))
		// TCP only: a refusal at accept is a refusal there, where QUIC would have finished its handshake first.
		c.Listen = []string{"/ip4/127.0.0.1/tcp/0"}
	})
	waitOwnRelay(t, n, "ok")
	for i := 0; len(n.Host().Network().ConnsToPeer(book.ID)) == 0; i++ {
		if i > 200 {
			t.Fatal("the server never reached the address book")
		}
		time.Sleep(50 * time.Millisecond)
	}
	code, _ := NewJoinCode()
	if err := n.OfferJoin(code, "the-invite-behind-it"); err != nil {
		t.Fatal(err)
	}
	find, _, _ := JoinKeysOf(code)
	findBytes, _ := hex.DecodeString(find)
	codeKey, _ := peer.Decode(joinEnrolID(findBytes))
	deadline := time.Now().Add(patience)
	for !f.devices(n.Host().ID())[codeKey] {
		if time.Now().After(deadline) {
			t.Fatal("the waiting code's own key never reached the relay's list")
		}
		time.Sleep(50 * time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(context.Background(), patience)
	defer cancel()
	res, err := Join(ctx, JoinConfig{Seed: testSeed(2), Code: code, Name: "Living room", Platform: "tvOS", Local: true,
		Layers:    Layers{Relay: true},
		Bootstrap: []peer.AddrInfo{book}, Listen: []string{"/ip4/127.0.0.1/tcp/0"}, Every: 100 * time.Millisecond})
	if err != nil || !res.OK || res.ServerID != n.ID() || res.Code != "123456" {
		t.Fatalf("the TV joining through the relay: %+v %v", res, err)
	}
	if !devs.Allowed(testID(t, 2)) || devs.Allowed(codeKey.String()) {
		t.Fatal("the device's own key should be listed, and the code's not")
	}
	// Taken: the code's key leaves the relay's list, and the device's own arrives.
	deadline = time.Now().Add(patience)
	for f.devices(n.Host().ID())[codeKey] || !f.devices(n.Host().ID())[mustPeer(t, 2)] {
		if time.Now().After(deadline) {
			t.Fatalf("the relay's list after the code was taken: %v", f.devices(n.Host().ID()))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// A code shown from its own key on behalf of a device that didn't sign for it
// lists nobody.
func TestACodesKeySpeaksOnlyForADeviceThatSigned(t *testing.T) {
	code, _ := NewJoinCode()
	find, _, _ := JoinKeysOf(code)
	findBytes, _ := hex.DecodeString(find)
	codeID := joinEnrolID(findBytes)
	server := testID(t, 1)
	sig, _ := SignEnrolFor(testSeed(2), server, codeID)
	hello := joinHello{Find: find, Device: testID(t, 2), DeviceSig: sig}
	if got, err := checkJoinFor(hello, codeID, server); err != nil || got != testID(t, 2) {
		t.Fatalf("a signed hello: %q %v", got, err)
	}
	for name, h := range map[string]struct {
		hello  joinHello
		connID string
	}{
		"another device's name":           {joinHello{Find: find, Device: testID(t, 3), DeviceSig: sig}, codeID},
		"shown from a key not the code's": {hello, testID(t, 4)},
		"another code's key":              {joinHello{Find: hex.EncodeToString(make([]byte, 32)), Device: testID(t, 2), DeviceSig: sig}, codeID},
	} {
		if _, err := checkJoinFor(h.hello, h.connID, server); err == nil {
			t.Errorf("%s: taken", name)
		}
	}
	if _, err := checkJoinFor(hello, codeID, testID(t, 5)); err == nil {
		t.Error("a signature for another server: taken")
	}
}

// A waiting code is announced again when the server's relayed addresses change, and only then.
func TestRelayedKeyIsTheRelayedAddressesOnly(t *testing.T) {
	a := relayedKey([]string{"/ip4/192.0.2.1/tcp/1", "/ip4/198.51.100.1/tcp/2/p2p/" + testID(t, 9) + "/p2p-circuit"})
	b := relayedKey([]string{"/ip4/192.0.2.7/tcp/1", "/ip4/198.51.100.1/tcp/2/p2p/" + testID(t, 9) + "/p2p-circuit"})
	if a == "" || a != b {
		t.Fatalf("a change of direct address changed the key: %q %q", a, b)
	}
	if relayedKey([]string{"/ip4/192.0.2.1/tcp/1"}) != "" {
		t.Fatal("no relay, yet a key")
	}
	if c := relayedKey([]string{"/ip4/198.51.100.2/tcp/2/p2p/" + testID(t, 8) + "/p2p-circuit"}); c == a {
		t.Fatal("another relay, the same key")
	}
}
