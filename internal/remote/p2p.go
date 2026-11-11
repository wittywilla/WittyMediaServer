// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package remote

import (
	"context"
	"net/netip"
)

// A request that arrives over a peer-to-peer connection comes out of this
// very process, and the connection underneath may have been made from the
// home network, from this computer, or through a relay. None of that says
// where the person is. So where such a request comes from is never read off
// an address: the connection is marked when it is accepted, and the mark
// decides. It always decides "the internet".

// P2PConn is what is known about a peer-to-peer connection.
type P2PConn struct {
	// Addr is the address the connection is counted under: the other end's
	// public address when it is direct, otherwise one made up for it. It is
	// never an address of this computer or of the home network.
	Addr netip.Addr
	// Device is the other end's key, as its ID.
	Device  string
	Relayed bool
	// Known: the device is on the server's list.
	Known bool
}

type p2pKey struct{}

// WithP2P marks a connection's context as peer-to-peer.
func WithP2P(ctx context.Context, c P2PConn) context.Context {
	return context.WithValue(ctx, p2pKey{}, c)
}

// P2PFrom reads the mark.
func P2PFrom(ctx context.Context) (P2PConn, bool) {
	c, ok := ctx.Value(p2pKey{}).(P2PConn)
	return c, ok
}

// p2pFallback stands in when a marked connection has no usable address: an
// address reserved for documentation, which is nobody's and nobody's home.
var p2pFallback = netip.MustParseAddr("192.0.2.1")

// p2pClient is the Client for a marked connection. Forwarded headers are not
// read at all: nothing in front of a peer-to-peer connection is a proxy of
// the owner's. It counts as encrypted, which it is, to the server's own key.
func p2pClient(c P2PConn) Client {
	a := clean(c.Addr)
	if !a.IsValid() || (*Policy)(nil).Classify(a) != Internet {
		a = p2pFallback
	}
	return Client{IP: a, Peer: a, Where: Internet, Secure: true, P2P: true, P2PDevice: c.Device, P2PRelayed: c.Relayed}
}
