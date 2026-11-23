// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	ma "github.com/multiformats/go-multiaddr"
)

// fakeWittyRelay speaks what WittyRelay (its own project) speaks, in a few
// lines: registration with a key and a device list, which decide who holds a
// place and who reaches whom, and the offer. The real one is tested in its
// own repository and, against a real server, in WSL (scripts/p2p-netsim).
type fakeWittyRelay struct {
	host.Host
	key   string
	offer RelayOffer

	mu      sync.Mutex
	servers map[peer.ID]map[peer.ID]bool
	regs    int
}

func (f *fakeWittyRelay) AllowReserve(p peer.ID, _ ma.Multiaddr) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.servers[p]
	return ok
}

func (f *fakeWittyRelay) AllowConnect(src peer.ID, _ ma.Multiaddr, dest peer.ID) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.servers[dest][src]
}

func (f *fakeWittyRelay) devices(server peer.ID) map[peer.ID]bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.servers[server]
}

func newFakeWittyRelay(t *testing.T, key string) (*fakeWittyRelay, string) {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"), libp2p.DisableMetrics(), libp2p.ForceReachabilityPublic())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	f := &fakeWittyRelay{Host: h, key: key, servers: map[peer.ID]map[peer.ID]bool{},
		offer: RelayOffer{V: 1, Software: "WittyRelay test", Name: "Test relay", Access: "key", Kbps: 1600, MaxHeight: 720, DataMiB: 4096, Minutes: 180}}
	res := relayv2.DefaultResources()
	res.Limit = nil
	if _, err := relayv2.New(h, relayv2.WithResources(res), relayv2.WithACL(f), relayv2.WithReservationAddressFilter(func(ma.Multiaddr) bool { return true })); err != nil {
		t.Fatal(err)
	}
	h.SetStreamHandler(ProtoRelay, func(s network.Stream) { s.Close() })
	h.SetStreamHandler(ProtoRelayInfo, serveOffer(f.offer))
	h.SetStreamHandler(ProtoRelayRegister, func(s network.Stream) {
		defer s.Close()
		var req RegisterRequest
		res := RegisterResponse{Offer: f.offer}
		switch err := readMessageN(s, &req, maxRegisterMessage); {
		case err != nil:
			res.Reason = "bad"
		case req.Key != f.key:
			res.Reason, res.Error = "key", "That access key isn't one this relay knows."
		default:
			devs := map[peer.ID]bool{}
			for _, d := range req.Devices {
				if id, err := peer.Decode(d); err == nil {
					devs[id] = true
				}
			}
			f.mu.Lock()
			f.servers[s.Conn().RemotePeer()] = devs
			f.regs++
			f.mu.Unlock()
			res.OK, res.TTLSec = true, 900
		}
		_ = writeMessage(s, res)
	})
	return f, h.Addrs()[0].String() + "/p2p/" + h.ID().String()
}

func ownRelay(addr, key string, devs *devList) nodeOpt {
	return func(c *NodeConfig) {
		c.Layers.Relay = true
		c.OwnRelays = []OwnRelay{{Addr: "wittyrelay://" + addr, Key: key}}
		c.DeviceIDs = func() []string {
			devs.mu.Lock()
			defer devs.mu.Unlock()
			var ids []string
			for id, ok := range devs.allowed {
				if ok {
					ids = append(ids, id)
				}
			}
			return ids
		}
		c.Options = append(c.Options, libp2p.ForceReachabilityPrivate())
	}
}

func waitOwnRelay(t *testing.T, n *Node, state string) OwnRelayStatus {
	t.Helper()
	deadline := time.Now().Add(patience)
	for {
		st := n.Status()
		if len(st.OwnRelays) == 1 && st.OwnRelays[0].State == state {
			return st.OwnRelays[0]
		}
		if time.Now().After(deadline) {
			t.Fatalf("the WittyRelay never became %q: %+v", state, st.OwnRelays)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestYourOwnWittyRelayCarriesYourDevicesOnly(t *testing.T) {
	f, relay := newFakeWittyRelay(t, "the-key-0123456789")
	devs := newDevList(testID(t, 2))
	n := startNode(t, 1, devs, ownRelay(relay, "the-key-0123456789", devs), func(c *NodeConfig) { c.RelayBitrate = 0 })

	// Registered, with what the relay allows, in words, before anything uses it.
	st := waitOwnRelay(t, n, "ok")
	if st.Offer == nil || st.Offer.MaxHeight != 720 || !strings.Contains(st.Allows, "1.6 Mbit/s per connection") || !strings.Contains(st.Allows, "720p") {
		t.Fatalf("status: %+v", st)
	}
	if !f.devices(n.Host().ID())[mustPeer(t, 2)] {
		t.Fatal("the relay wasn't told which devices may come through it")
	}

	// A listed device, knowing only the relayed address, gets in through it,
	// and is told what the relay allows.
	c := startClient(t, 2, n, func(c *ClientConfig) {
		c.Hints = []string{viaRelay(relay)}
		c.Layers.Relay = true
	})
	s := waitPathWith(t, c, func(s State) bool { return s.Path == PathRelayed && s.RelayMaxHeight != 0 })
	if !s.Community || s.RelayKbps != 1600 || s.RelayMaxHeight != 720 || s.RelayAllows == "" {
		t.Fatalf("state: %+v", s)
	}
	code, body, err := get(t, c, "/hello")
	if err != nil || code != 200 || !strings.Contains(body, "relayed=true") {
		t.Fatalf("%d %q %v", code, body, err)
	}
	// Paced to the relay's own limit (1.6 Mbit/s = 200 kB/s) though the
	// server set none of its own: 600 kB takes over two seconds.
	start := time.Now()
	if code, body, err = get(t, c, "/big", "Range", "bytes=0-599999"); err != nil || code != 206 || len(body) != 600000 {
		t.Fatalf("%d, %d bytes, %v", code, len(body), err)
	}
	if d := time.Since(start); d < 2*time.Second {
		t.Errorf("600 kB through a relay that allows 200 kB/s took only %v", d)
	}
	if lim := n.RelayStreamLimit(f.ID().String()); lim != 1_600_000 {
		t.Errorf("stream limit through the relay: %d", lim)
	}

	// A device that isn't on the list doesn't get through the relay at all.
	stranger := startClient(t, 3, n, func(c *ClientConfig) {
		c.Hints = []string{viaRelay(relay)}
		c.Layers.Relay = true
	})
	time.Sleep(time.Second)
	if stranger.State().Connected() {
		t.Fatalf("a device the server never listed got through its relay: %+v", stranger.State())
	}

	// Listing a device tells the relay at once.
	devs.set(testID(t, 3), true)
	n.DevicesChanged()
	deadline := time.Now().Add(patience)
	for !f.devices(n.Host().ID())[mustPeer(t, 3)] {
		if time.Now().After(deadline) {
			t.Fatal("the relay never heard about the newly listed device")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestAWrongRelayKeySaysSo(t *testing.T) {
	_, relay := newFakeWittyRelay(t, "the-key-0123456789")
	devs := newDevList()
	n := startNode(t, 1, devs, ownRelay(relay, "not-the-key-000000", devs))
	st := waitOwnRelay(t, n, "refused")
	if !strings.Contains(st.Error, "access key") {
		t.Fatalf("%+v", st)
	}
}

func TestCheckingARelayBeforeUsingIt(t *testing.T) {
	_, relay := newFakeWittyRelay(t, "k")
	ctx := context.Background()
	o, err := CheckRelay(ctx, relay)
	if err != nil || o.Name != "Test relay" || o.MaxHeight != 720 {
		t.Fatalf("%+v %v", o, err)
	}
	// A community relay answers what it allows but takes no registrations.
	devs := newDevList()
	n := startNode(t, 4, devs, func(c *NodeConfig) {
		l := DefaultRelayLimits()
		l.TotalKbps, l.MaxHeight = 3000, 480
		c.RelayService = &l
		c.Layers.Relay = true
	})
	addr := n.Host().Addrs()[0].String() + "/p2p/" + n.ID()
	o, err = CheckRelay(ctx, addr)
	if err == nil || !strings.Contains(err.Error(), "community relay") || o.MaxHeight != 480 || o.TotalKbps != 3000 {
		t.Fatalf("%+v %v", o, err)
	}
	if !strings.Contains(o.Words(), "3 Mbit/s for everyone together") || !strings.Contains(o.Words(), "480p") {
		t.Fatalf("words: %s", o.Words())
	}
	// Nothing listening there.
	if _, err := CheckRelay(ctx, "/ip4/127.0.0.1/tcp/1/p2p/"+n.ID()); err == nil {
		t.Fatal("a relay that isn't there was checked")
	}
	if _, err := CheckRelay(ctx, "relay.example.com"); err == nil {
		t.Fatal("not an address")
	}
}

func TestACommunityRelayPacesEverythingTogether(t *testing.T) {
	b := newBucket(800_000) // 100 kB/s
	p := pacer{b}
	start := time.Now()
	for range 30 {
		p.BytesTransferred(10_000)
	}
	if d := time.Since(start); d < 2*time.Second {
		t.Fatalf("300 kB at 100 kB/s took %v", d)
	}
}

func TestRelayOffers(t *testing.T) {
	o := RelayOffer{Kbps: 4000, TotalKbps: 50, MaxHeight: 99999, DataMiB: 2048, Minutes: 120, Name: "a\x00b" + strings.Repeat("é", 50), Access: "other"}.Clean()
	if o.TotalKbps != 100 || o.MaxHeight != 4320 || o.Access != "open" || strings.ContainsRune(o.Name, 0) || len(o.Name) > 60 {
		t.Fatalf("%+v", o)
	}
	if w := o.Words(); w != "Up to 4 Mbit/s per connection, 100 kbit/s for everyone together, pictures up to 4320p, 2 GiB and 2 hours per connection before it starts a new one." {
		t.Fatalf("%q", w)
	}
	for in, want := range map[RelayHealth]*RelayHealth{
		{State: "busy", Load: 87}:   {State: "busy", Load: 90},
		{State: "ok", Load: -4}:     {State: "ok", Load: 0},
		{State: "full", Load: 400}:  {State: "full", Load: 100},
		{State: "on fire", Load: 0}: nil,
	} {
		h := in
		got := RelayOffer{Health: &h}.Clean().Health
		if (got == nil) != (want == nil) || (got != nil && *got != *want) {
			t.Fatalf("health %+v cleaned to %+v", in, got)
		}
	}
	if w := (RelayOffer{}).Words(); w != "No speed limit per connection." {
		t.Fatalf("%q", w)
	}
	if got := offerOf(DefaultRelayLimits()); got.Kbps != 4000 || got.MaxHeight != 720 || got.DataMiB != 2048 || got.Minutes != 120 || got.Servers != 16 {
		t.Fatalf("%+v", got)
	}
}

func TestRelayAddresses(t *testing.T) {
	id := testID(t, 7)
	for _, ok := range []string{"/dns4/relay.example.com/udp/4001/quic-v1/p2p/" + id, "wittyrelay:///ip4/203.0.113.4/tcp/4001/p2p/" + id, " /ip6/2001:db8::1/tcp/4001/p2p/" + id + " "} {
		if _, err := ParseRelayAddr(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "relay.example.com:4001", "/ip4/1.2.3.4/tcp/4001", "/p2p/" + id, "/ip4/1.2.3.4/tcp/4001/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N", strings.Repeat("/ip4/1.2.3.4", 40)} {
		if _, err := ParseRelayAddr(bad); err == nil {
			t.Errorf("%q was taken for a relay's address", bad)
		}
	}
}

// Many devices at once, each with requests in parallel, all answered whole.
func TestManyDevicesAtOnce(t *testing.T) {
	const devices, each = 6, 8
	var ids []string
	for i := range devices {
		ids = append(ids, testID(t, byte(10+i)))
	}
	n := startNode(t, 1, newDevList(ids...))
	var wg sync.WaitGroup
	errs := make(chan error, devices*each)
	for i := range devices {
		c := startClient(t, byte(10+i), n)
		for j := range each {
			wg.Add(1)
			go func() {
				defer wg.Done()
				start := (i*each + j) * 1000
				code, body, err := get(t, c, "/big", "Range", fmt.Sprintf("bytes=%d-%d", start, start+99_999))
				if err != nil || code != 206 || body != string(bigBody[start:start+100_000]) {
					errs <- fmt.Errorf("device %d request %d: %d, %d bytes, %v", i, j, code, len(body), err)
				}
			}()
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if got := len(n.Status().Devices); got < devices {
		t.Errorf("%d devices in the status, want %d", got, devices)
	}
}

func waitPathWith(t *testing.T, c *Client, ok func(State) bool) State {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), patience)
	defer cancel()
	s, err := c.keeper.wait(ctx, ok)
	if err != nil {
		t.Fatalf("the state never got there: %+v", s)
	}
	return s
}

// ---------------------------------------------------------------- fuzzing

func FuzzReadMessage(f *testing.F) {
	f.Add([]byte("\x00\x00\x00\x02{}"))
	f.Add([]byte("\x00\x00\x00\x0b{\"v\":1,\"s\":1}"))
	f.Add([]byte("\xff\xff\xff\xff"))
	f.Add([]byte("\x00\x00\x00\x00"))
	f.Fuzz(func(t *testing.T, b []byte) {
		var e EnrolRequest
		_ = readMessage(bytes.NewReader(b), &e)
		var r RegisterResponse
		if readMessageN(bytes.NewReader(b), &r, maxEnrolMessage) == nil {
			o := r.Offer.Clean()
			if len(o.Note) > 280 || len(o.Name) > 60 || (o.Access != "open" && o.Access != "key") {
				t.Fatalf("an offer was let through as %+v", o)
			}
			_ = o.Words()
		}
		var j joinHello
		_ = readMessage(bytes.NewReader(b), &j)
		// Nothing longer than the limit is read, whatever the length says.
		if len(b) >= 4 && int(uint32(b[0])<<24|uint32(b[1])<<16|uint32(b[2])<<8|uint32(b[3])) > maxEnrolMessage {
			if readMessage(bytes.NewReader(b), &e) == nil {
				t.Fatal("an over-long message was read")
			}
		}
	})
}

func FuzzParseInvite(f *testing.F) {
	f.Add("wittymedia://p2p/12D3KooWD3eckifWpRn9wQpMG9R9hX3sD158z7EqHWmweQAJU5SA?s=0123456789abcdef0123&n=Home&a=/ip4/1.2.3.4/tcp/4001")
	f.Add("wittymedia://p2p/?s=")
	f.Add("wittymedia://p2p/%zz")
	f.Fuzz(func(t *testing.T, link string) {
		in, err := ParseInvite(link)
		if err != nil {
			return
		}
		if _, err := CheckID(in.ServerID); err != nil || len(in.Secret) < 16 || len(in.Secret) > 64 || len(in.Name) > 80 || len(in.Addrs) > maxInviteAddrs {
			t.Fatalf("%q read as %+v", link, in)
		}
		// What is read can be written and read back the same.
		again, err := ParseInvite(in.String())
		if err != nil || again.ServerID != in.ServerID || again.Secret != in.Secret {
			t.Fatalf("%+v round-tripped as %+v, %v", in, again, err)
		}
	})
}

func FuzzParseRelayAddr(f *testing.F) {
	f.Add("/dns4/relay.example.com/udp/4001/quic-v1/p2p/12D3KooWD3eckifWpRn9wQpMG9R9hX3sD158z7EqHWmweQAJU5SA")
	f.Add("wittyrelay:///ip4/1.2.3.4/tcp/1/p2p/12D3KooWD3eckifWpRn9wQpMG9R9hX3sD158z7EqHWmweQAJU5SA")
	f.Fuzz(func(t *testing.T, s string) {
		ai, err := ParseRelayAddr(s)
		if err == nil && (len(ai.Addrs) == 0 || ai.ID == "") {
			t.Fatalf("%q read as %+v", s, ai)
		}
		_, _ = CheckID(s)
		_ = parseAddrs([]string{s}, mustPeerF(f))
		if ip, err := netip.ParseAddr(s); err == nil {
			_ = presentedIP(ip, false, false, mustPeerF(f))
		}
	})
}

var fuzzPeer peer.ID

func mustPeerF(f *testing.F) peer.ID {
	if fuzzPeer == "" {
		id, err := IDFromSeed(testSeed(5))
		if err != nil {
			f.Fatal(err)
		}
		fuzzPeer, _ = peer.Decode(id)
	}
	return fuzzPeer
}

var _ = io.EOF
