// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package p2p connects an app to a server at home with nothing set up on the
// router and no machine rented anywhere. Both ends are libp2p nodes: the
// server is known by the public half of a key it keeps (its "connection ID"),
// apps dial that ID, and whatever path the two find (the same network, IPv6,
// a port the router mapped, a hole punched through both routers, or a relay)
// is encrypted to that key from end to end. The server's ordinary HTTP API
// rides inside, so every route works as it does at home.
//
// docs/P2P.md is the design: the layers, who is contacted, the threat model.
package p2p

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

// The protocols spoken between an app and a server. A change that an older
// end couldn't follow gets a new number, and both are answered for a while.
const (
	// ProtoHTTP carries HTTP/1.1, one stream per connection, to the server's
	// ordinary handler.
	ProtoHTTP protocol.ID = "/wittymedia/http/1.0.0"
	// ProtoEnrol is how a device that holds an invite gets its key onto the
	// server's list of devices.
	ProtoEnrol protocol.ID = "/wittymedia/enrol/1.0.0"
	// ProtoRelay marks a node as a community relay: a WittyMediaServer whose
	// owner offered to pass on other people's (encrypted) connections.
	ProtoRelay protocol.ID = "/wittymedia/relay/1.0.0"

	// RelayNamespace is what community relays announce themselves under.
	RelayNamespace = "wittymedia-community-relay-v1"

	// UserAgent is what other libp2p nodes are told this is.
	UserAgent = "wittymedia/1"
)

// SeedSize is the length of a key's seed.
const SeedSize = ed25519.SeedSize

// NewSeed makes the secret half of a new identity.
func NewSeed() ([]byte, error) {
	seed := make([]byte, SeedSize)
	if _, err := rand.Read(seed); err != nil {
		return nil, err
	}
	return seed, nil
}

// EncodeSeed and DecodeSeed are how a seed is kept in settings and files.
func EncodeSeed(seed []byte) string { return base64.StdEncoding.EncodeToString(seed) }

// DecodeSeed reads what EncodeSeed wrote.
func DecodeSeed(s string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != SeedSize {
		return nil, errors.New("not a connection key")
	}
	return b, nil
}

func keyFromSeed(seed []byte) (crypto.PrivKey, error) {
	if len(seed) != SeedSize {
		return nil, errors.New("not a connection key")
	}
	return crypto.UnmarshalEd25519PrivateKey(ed25519.NewKeyFromSeed(seed))
}

// IDFromSeed is the public name of a key: what the other end dials or lists.
func IDFromSeed(seed []byte) (string, error) {
	k, err := keyFromSeed(seed)
	if err != nil {
		return "", err
	}
	id, err := peer.IDFromPrivateKey(k)
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// CheckID says whether s can be a device's or a server's ID, and returns it
// in its usual spelling. Only Ed25519 identities are accepted: that is all
// this software makes, and their IDs hold the whole public key.
func CheckID(s string) (string, error) {
	id, err := peer.Decode(strings.TrimSpace(s))
	if err != nil {
		return "", errors.New("not a connection ID")
	}
	pub, err := id.ExtractPublicKey()
	if err != nil || pub == nil || pub.Type() != crypto.Ed25519 {
		return "", errors.New("not a connection ID")
	}
	return id.String(), nil
}

// ShortID is the end of an ID, for logs and lists: enough to tell two apart.
func ShortID(id string) string {
	if len(id) > 8 {
		return id[len(id)-8:]
	}
	return id
}

// ---------------------------------------------------------------- invites

// InviteScheme starts an invite link, which the apps register to open.
const InviteScheme = "wittymedia"

// Invite is what an invite link or QR code holds: which server, a secret that
// works once, and where the server was last reachable, to save a search.
type Invite struct {
	ServerID string
	Secret   string
	Name     string   // the server's name, to show before connecting
	Addrs    []string // multiaddrs without the /p2p/ part
}

// maxInviteAddrs keeps a link short enough for a QR code a phone can read.
const maxInviteAddrs = 6

// String is the link: wittymedia://p2p/<server ID>?s=<secret>&n=<name>&a=<address>...
func (in Invite) String() string {
	q := url.Values{}
	q.Set("s", in.Secret)
	if in.Name != "" {
		q.Set("n", in.Name)
	}
	for i, a := range in.Addrs {
		if i >= maxInviteAddrs {
			break
		}
		q.Add("a", a)
	}
	return InviteScheme + "://p2p/" + in.ServerID + "?" + q.Encode()
}

// ParseInvite reads a link. Nothing in it is trusted further than this: the
// ID is what gets dialled and proved, the addresses are only places to try.
func ParseInvite(link string) (Invite, error) {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || u.Scheme != InviteScheme || u.Host != "p2p" {
		return Invite{}, errors.New("not an invite link")
	}
	id, err := CheckID(strings.Trim(u.Path, "/"))
	if err != nil {
		return Invite{}, errors.New("not an invite link")
	}
	q := u.Query()
	in := Invite{ServerID: id, Secret: q.Get("s"), Name: q.Get("n")}
	if len(in.Secret) < 16 || len(in.Secret) > 64 {
		return Invite{}, errors.New("not an invite link")
	}
	if len(in.Name) > 80 {
		in.Name = in.Name[:80]
	}
	for _, a := range q["a"] {
		if len(in.Addrs) >= maxInviteAddrs {
			break
		}
		if _, err := ma.NewMultiaddr(a); err == nil {
			in.Addrs = append(in.Addrs, a)
		}
	}
	return in, nil
}

// ---------------------------------------------------------------- addresses

// parseAddrs reads multiaddrs, leaving out the unreadable ones. A trailing
// /p2p/<id> is accepted when it names id and dropped; another ID is refused.
func parseAddrs(list []string, id peer.ID) []ma.Multiaddr {
	var out []ma.Multiaddr
	for _, s := range list {
		a, err := ma.NewMultiaddr(strings.TrimSpace(s))
		if err != nil {
			continue
		}
		if ai, err := peer.AddrInfoFromP2pAddr(a); err == nil {
			if ai.ID != id || len(ai.Addrs) == 0 {
				continue
			}
			a = ai.Addrs[0]
		}
		out = append(out, a)
	}
	return out
}

func addrStrings(list []ma.Multiaddr) []string {
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, a.String())
	}
	return out
}

// isRelayed says an address goes through a relay.
func isRelayed(a ma.Multiaddr) bool {
	_, err := a.ValueForProtocol(ma.P_CIRCUIT)
	return err == nil
}

// relayOf names the relay in a relayed address ("" if it isn't one).
func relayOf(a ma.Multiaddr) peer.ID {
	if !isRelayed(a) {
		return ""
	}
	relay, _ := ma.SplitFunc(a, func(c ma.Component) bool { return c.Protocol().Code == ma.P_CIRCUIT })
	if relay == nil {
		return ""
	}
	if ai, err := peer.AddrInfoFromP2pAddr(relay); err == nil {
		return ai.ID
	}
	return ""
}

// ipOf is the IP address at the start of a multiaddr.
func ipOf(a ma.Multiaddr) netip.Addr {
	ip, err := manet.ToIP(a)
	if err != nil {
		return netip.Addr{}
	}
	out, _ := netip.AddrFromSlice(ip)
	return out.Unmap()
}

// publicIP: an address the internet routes to.
func publicIP(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsValid() && a.IsGlobalUnicast() && !a.IsPrivate() && !a.IsLoopback() && !a.IsLinkLocalUnicast() &&
		!netip.MustParsePrefix("100.64.0.0/10").Contains(a) && !netip.MustParsePrefix("192.0.2.0/24").Contains(a) &&
		!netip.MustParsePrefix("2001:db8::/32").Contains(a)
}

func transportOf(a ma.Multiaddr) string {
	switch {
	case hasProto(a, ma.P_QUIC_V1):
		return "quic"
	case hasProto(a, ma.P_TCP):
		return "tcp"
	}
	return "other"
}

func hasProto(a ma.Multiaddr, code int) bool {
	_, err := a.ValueForProtocol(code)
	return err == nil
}

// spoken says an address uses a transport these nodes have: QUIC or plain
// TCP. A relay on the public network lists more (WebSocket, WebTransport,
// WebRTC), and an address only they could use is no use to an app.
func spoken(a ma.Multiaddr) bool {
	for _, code := range []int{ma.P_WS, ma.P_WSS, ma.P_TLS, ma.P_WEBTRANSPORT, ma.P_WEBRTC_DIRECT, ma.P_WEBRTC, ma.P_QUIC} {
		if hasProto(a, code) {
			return false
		}
	}
	return hasProto(a, ma.P_QUIC_V1) || hasProto(a, ma.P_TCP)
}

// mappedAddrs are the addresses a port mapping gives: ip:port for UDP (QUIC)
// and TCP.
func mappedAddrs(ip netip.Addr, port int) []ma.Multiaddr {
	if !ip.IsValid() || port <= 0 {
		return nil
	}
	fam := "ip4"
	if ip.Is6() {
		fam = "ip6"
	}
	var out []ma.Multiaddr
	for _, s := range []string{
		fmt.Sprintf("/%s/%s/udp/%d/quic-v1", fam, ip, port),
		fmt.Sprintf("/%s/%s/tcp/%d", fam, ip, port),
	} {
		if a, err := ma.NewMultiaddr(s); err == nil {
			out = append(out, a)
		}
	}
	return out
}
