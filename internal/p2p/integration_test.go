// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/wittywilla/wittymediaserver/internal/portmap"
)

// Everything here runs on this machine: nodes find each other at 127.0.0.1,
// through relays and address books that are also in this process. Nothing is
// sent to the internet, and no node here could join the public network: the
// DHT in these tests speaks under a name of its own.

type devList struct {
	mu      sync.Mutex
	allowed map[string]bool
	seen    map[string]string
}

func newDevList(ids ...string) *devList {
	d := &devList{allowed: map[string]bool{}, seen: map[string]string{}}
	for _, id := range ids {
		d.allowed[id] = true
	}
	return d
}
func (d *devList) Allowed(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.allowed[id]
}
func (d *devList) Seen(id, path string) {
	d.mu.Lock()
	d.seen[id] = path
	d.mu.Unlock()
}
func (d *devList) set(id string, ok bool) {
	d.mu.Lock()
	d.allowed[id] = ok
	d.mu.Unlock()
}
func (d *devList) path(id string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.seen[id]
}

type addrKey struct{}

var bigBody = bytes.Repeat([]byte("0123456789abcdef"), 64<<10) // 1 MiB

func testHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		a, _ := r.Context().Value(addrKey{}).(*Addr)
		if a == nil {
			http.Error(w, "not marked as peer-to-peer", 500)
			return
		}
		fmt.Fprintf(w, "remote=%s ip=%s relayed=%v known=%v", r.RemoteAddr, a.IP, a.Relayed, a.Known)
	})
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "big.bin", time.Time{}, bytes.NewReader(bigBody))
	})
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for i := range 3 {
			fmt.Fprintf(w, "data: %d\n\n", i)
			w.(http.Flusher).Flush()
			select {
			case <-time.After(30 * time.Millisecond):
			case <-r.Context().Done():
				return
			}
		}
	})
	mux.HandleFunc("/upgrade", func(w http.ResponseWriter, r *http.Request) {
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer conn.Close()
		_, _ = io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: echo\r\nConnection: Upgrade\r\n\r\n")
		line, _ := buf.ReadString('\n')
		_, _ = io.WriteString(conn, "echo "+line)
	})
	return mux
}

type nodeOpt func(*NodeConfig)

func startNode(t *testing.T, seed byte, devs *devList, opts ...nodeOpt) *Node {
	t.Helper()
	cfg := NodeConfig{
		Seed: testSeed(seed), Devices: devs, DevicesOnly: true, Handler: testHandler(), Local: true,
		Listen: []string{"/ip4/127.0.0.1/udp/0/quic-v1", "/ip4/127.0.0.1/tcp/0"},
		ConnInfo: func(ctx context.Context, a *Addr) context.Context {
			return context.WithValue(ctx, addrKey{}, a)
		},
	}
	for _, o := range opts {
		o(&cfg)
	}
	n, err := StartNode(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(n.Close)
	return n
}

type clientOpt func(*ClientConfig)

func startClient(t *testing.T, seed byte, n *Node, opts ...clientOpt) *Client {
	t.Helper()
	cfg := ClientConfig{Seed: testSeed(seed), ServerID: n.ID(), Hints: n.Addrs(), Local: true, UpgradeWait: 400 * time.Millisecond,
		Listen: []string{"/ip4/127.0.0.1/udp/0/quic-v1", "/ip4/127.0.0.1/tcp/0"}}
	for _, o := range opts {
		o(&cfg)
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func get(t *testing.T, c *Client, path string, header ...string) (int, string, error) {
	t.Helper()
	req, _ := http.NewRequest("GET", "http://server"+path, nil)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	res, err := c.Transport().RoundTrip(req.WithContext(ctx))
	if err != nil {
		return 0, "", err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	return res.StatusCode, string(b), err
}

func waitPath(t *testing.T, c *Client, want ...string) State {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	s, err := c.keeper.wait(ctx, func(s State) bool {
		for _, w := range want {
			if s.Path == w {
				return true
			}
		}
		return false
	})
	if err != nil {
		t.Fatalf("the path never became %v: %+v", want, s)
	}
	return s
}

func TestHTTPOverADirectConnection(t *testing.T) {
	devs := newDevList(testID(t, 2))
	n := startNode(t, 1, devs)
	c := startClient(t, 2, n)
	s := waitPath(t, c, PathLocal)
	if s.Transport != "quic" && s.Transport != "tcp" {
		t.Errorf("transport %q", s.Transport)
	}

	code, body, err := get(t, c, "/hello")
	if err != nil || code != 200 {
		t.Fatalf("GET /hello: %d %q %v", code, body, err)
	}
	// The classic tunnel mistake: the connection really comes from 127.0.0.1,
	// and that is exactly what the server must not be told.
	if strings.Contains(body, "127.0.0.1") || !strings.Contains(body, "remote=[2001:db8:") || !strings.Contains(body, "known=true") {
		t.Fatalf("the server saw: %s", body)
	}
	if devs.path(c.ID()) != "direct" {
		t.Errorf("last seen path %q", devs.path(c.ID()))
	}

	// A range request, as a video player makes.
	code, body, err = get(t, c, "/big", "Range", "bytes=1048560-1048575")
	if err != nil || code != 206 || body != "0123456789abcdef" {
		t.Fatalf("range: %d %q %v", code, body, err)
	}
	// A whole megabyte arrives whole.
	code, body, err = get(t, c, "/big")
	if err != nil || code != 200 || len(body) != len(bigBody) || body != string(bigBody) {
		t.Fatalf("big: %d, %d bytes, %v", code, len(body), err)
	}

	// Server-sent events arrive as they are sent, not when the response ends.
	req, _ := http.NewRequest("GET", "http://server/events", nil)
	res, err := c.Transport().RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(res.Body)
	start := time.Now()
	line, err := r.ReadString('\n')
	if err != nil || line != "data: 0\n" || time.Since(start) > 2*time.Second {
		t.Fatalf("first event %q after %v: %v", line, time.Since(start), err)
	}
	rest, _ := io.ReadAll(r)
	res.Body.Close()
	if !strings.Contains(string(rest), "data: 2") {
		t.Fatalf("events: %q", rest)
	}

	// An upgraded connection (what a WebSocket is underneath) passes both ways.
	conn, err := c.DialContext(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	_, _ = io.WriteString(conn, "GET /upgrade HTTP/1.1\r\nHost: server\r\nConnection: Upgrade\r\nUpgrade: echo\r\n\r\n")
	br := bufio.NewReader(conn)
	status, _ := br.ReadString('\n')
	if !strings.Contains(status, "101") {
		t.Fatalf("upgrade answered %q", status)
	}
	for {
		l, err := br.ReadString('\n')
		if err != nil || l == "\r\n" {
			break
		}
	}
	_, _ = io.WriteString(conn, "ping\n")
	if l, _ := br.ReadString('\n'); l != "echo ping\n" {
		t.Fatalf("echo: %q", l)
	}

	st := n.Status()
	if !st.Running || st.ID != n.ID() || len(st.Devices) == 0 || st.Devices[0].ID != c.ID() || st.Devices[0].Path != "direct" {
		t.Fatalf("status: %+v", st)
	}
}

func TestOnlyListedDevicesMayOpenStreams(t *testing.T) {
	devs := newDevList()
	n := startNode(t, 1, devs)
	c := startClient(t, 2, n)
	waitPath(t, c, PathLocal) // the connection itself is made: it is the stream that is refused
	if _, _, err := get(t, c, "/hello"); err == nil {
		t.Fatal("a device that isn't on the list got an answer")
	}

	// Put on the list, it gets in; taken off and kicked, it is out again, at once.
	devs.set(c.ID(), true)
	if code, _, err := get(t, c, "/hello"); err != nil || code != 200 {
		t.Fatalf("after being listed: %d %v", code, err)
	}
	devs.set(c.ID(), false)
	n.Kick(c.ID())
	if _, _, err := get(t, c, "/hello"); err == nil {
		t.Fatal("a revoked device got an answer")
	}

	// A key let in for a moment (the connection test) is let in for that moment only.
	n.AllowBriefly(c.ID(), 300*time.Millisecond)
	if code, body, err := get(t, c, "/hello"); err != nil || code != 200 || !strings.Contains(body, "known=false") {
		t.Fatalf("briefly allowed: %d %q %v", code, body, err)
	}
	time.Sleep(350 * time.Millisecond)
	c.Transport().CloseIdleConnections()
	if _, _, err := get(t, c, "/hello"); err == nil {
		t.Fatal("the moment passed, and it was still let in")
	}
}

func TestOpenModeReachesTheServerAsAStranger(t *testing.T) {
	n := startNode(t, 1, newDevList(), func(c *NodeConfig) { c.DevicesOnly = false })
	c := startClient(t, 2, n)
	code, body, err := get(t, c, "/hello")
	if err != nil || code != 200 {
		t.Fatalf("%d %v", code, err)
	}
	// Not on the list and from a private address: counted with every other
	// stranger, under an address that is nobody's.
	if !strings.Contains(body, "remote=192.0.2.1:0") || !strings.Contains(body, "known=false") {
		t.Fatalf("the server saw: %s", body)
	}
}

func TestStreamLimitPerDevice(t *testing.T) {
	devs := newDevList(testID(t, 2))
	n := startNode(t, 1, devs)
	c := startClient(t, 2, n)
	waitPath(t, c, PathLocal)
	var conns []net.Conn
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()
	// Open connections and leave them idle. The ones over the limit are reset.
	refused := 0
	for range maxStreamsPerPeer + 6 {
		conn, err := c.DialContext(context.Background(), "", "")
		if err != nil {
			refused++
			continue
		}
		conns = append(conns, conn)
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		if _, err := io.WriteString(conn, "GET /hello HTTP/1.1\r\nHost: s\r\n\r\n"); err != nil {
			refused++
			continue
		}
		if _, err := bufio.NewReader(conn).ReadString('\n'); err != nil {
			refused++
		}
	}
	if refused != 6 {
		t.Fatalf("%d connections over the limit of %d were refused, want 6", refused, maxStreamsPerPeer)
	}
}

func TestEnrolment(t *testing.T) {
	devs := newDevList()
	var from atomic.Pointer[netip.Addr]
	n := startNode(t, 1, devs, func(c *NodeConfig) {
		c.Enrol = func(_ context.Context, id string, req EnrolRequest, addr netip.Addr) EnrolResponse {
			from.Store(&addr)
			if req.Secret != "the-right-secret-0001" {
				return EnrolResponse{Reason: "wrong", Error: "no"}
			}
			devs.set(id, true)
			return EnrolResponse{OK: true, Code: "123456", ServerName: "Home"}
		}
	})
	c := startClient(t, 2, n)
	ctx := context.Background()
	res, err := c.Enrol(ctx, "a-wrong-secret-000000", "Laptop", "desktop")
	if err != nil || res.OK || res.Reason != "wrong" {
		t.Fatalf("wrong secret: %+v %v", res, err)
	}
	if _, _, err := get(t, c, "/hello"); err == nil {
		t.Fatal("a wrong invite opened the door")
	}
	res, err = c.Enrol(ctx, "the-right-secret-0001", "Laptop", "desktop")
	if err != nil || !res.OK || res.Code != "123456" || len(res.Addrs) == 0 {
		t.Fatalf("right secret: %+v %v", res, err)
	}
	if a := from.Load(); a == nil || *a != strangerIP {
		t.Errorf("an enrolling stranger from this machine was counted under %v", a)
	}
	if code, _, err := get(t, c, "/hello"); err != nil || code != 200 {
		t.Fatalf("after enrolling: %d %v", code, err)
	}
	// Guessing is slowed: the sixth try within a minute isn't looked at.
	var last EnrolResponse
	for range 5 {
		last, _ = c.Enrol(ctx, "a-wrong-secret-000000", "Laptop", "desktop")
	}
	if last.Reason != "too-many-tries" {
		t.Fatalf("after many tries: %+v", last)
	}
}

// newRelay is a relay on this machine. community marks it as one that passes
// on whole connections.
func newRelay(t *testing.T, community bool) (host.Host, string) {
	t.Helper()
	opts := []libp2p.Option{
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.DisableMetrics(),
		libp2p.ForceReachabilityPublic(),
		libp2p.EnableRelayService(relayv2.WithInfiniteLimits()),
	}
	h, err := libp2p.New(opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	if community {
		h.SetStreamHandler(ProtoRelay, func(s network.Stream) { s.Close() })
	}
	return h, h.Addrs()[0].String() + "/p2p/" + h.ID().String()
}

func behindRelay(relay string) nodeOpt {
	return func(c *NodeConfig) {
		c.Layers.Relay = true
		c.StaticRelays = []string{relay}
		c.Options = append(c.Options, libp2p.ForceReachabilityPrivate())
	}
}

// viaRelay is the server's address through a relay on this machine. (libp2p
// only announces relayed addresses by itself for relays with public
// addresses, which nothing in these tests has.) The app keeps trying it until
// the server has its place on the relay.
func viaRelay(relay string) string { return relay + "/p2p-circuit" }

func TestRelayedPathThroughACommunityRelay(t *testing.T) {
	_, relay := newRelay(t, true)
	devs := newDevList(testID(t, 2))
	n := startNode(t, 1, devs, behindRelay(relay), func(c *NodeConfig) { c.RelayBitrate = 1_600_000 }) // 200 kB/s
	circuit := viaRelay(relay)
	// The app knows only the relayed address, and hole punching is off, so the
	// relay is the only way.
	c := startClient(t, 2, n, func(c *ClientConfig) {
		c.Hints = []string{circuit}
		c.Layers.Relay = true
	})
	s := waitPath(t, c, PathRelayed)
	if !s.Community || s.Relay == "" {
		t.Fatalf("state: %+v", s)
	}
	code, body, err := get(t, c, "/hello")
	if err != nil || code != 200 || !strings.Contains(body, "relayed=true") || !strings.Contains(body, "remote=[2001:db8:") {
		t.Fatalf("%d %q %v", code, body, err)
	}
	if devs.path(c.ID()) != "relayed" {
		t.Errorf("last seen path %q", devs.path(c.ID()))
	}
	// What goes through a relay is paced: 600 kB at 200 kB/s is over two seconds.
	start := time.Now()
	code, body, err = get(t, c, "/big", "Range", "bytes=0-599999")
	if err != nil || code != 206 || len(body) != 600000 {
		t.Fatalf("%d, %d bytes, %v", code, len(body), err)
	}
	if d := time.Since(start); d < 2*time.Second {
		t.Errorf("600 kB through a relay limited to 200 kB/s took only %v", d)
	}
	st := n.Status()
	if len(st.Devices) != 1 || st.Devices[0].Path != "relayed" {
		t.Fatalf("devices in the status: %+v", st.Devices)
	}
}

func TestAPublicRelayIsForSetupOnly(t *testing.T) {
	_, relay := newRelay(t, false)
	devs := newDevList()
	n := startNode(t, 1, devs, behindRelay(relay), func(c *NodeConfig) {
		c.Enrol = func(_ context.Context, id string, _ EnrolRequest, _ netip.Addr) EnrolResponse {
			devs.set(id, true)
			return EnrolResponse{OK: true}
		}
	})
	circuit := viaRelay(relay)
	c := startClient(t, 2, n, func(c *ClientConfig) {
		c.Hints = []string{circuit}
		c.Layers.Relay = true
	})
	s := waitPath(t, c, PathRelayed)
	if s.Community {
		t.Fatalf("an ordinary relay was taken for a community one: %+v", s)
	}
	// An invite is small enough to go through.
	if res, err := c.Enrol(context.Background(), "the-right-secret-0001", "Phone", "android"); err != nil || !res.OK {
		t.Fatalf("enrol over a public relay: %+v %v", res, err)
	}
	// Pages and films are not sent through it: the app is told plainly, after
	// waiting a moment for a direct connection.
	_, _, err := get(t, c, "/hello")
	if !errors.Is(err, ErrNoDirectPath) {
		t.Fatalf("want ErrNoDirectPath, got %v", err)
	}
	start := time.Now()
	if _, _, err := get(t, c, "/hello"); !errors.Is(err, ErrNoDirectPath) || time.Since(start) > 200*time.Millisecond {
		t.Fatalf("the second request should fail at once: %v after %v", err, time.Since(start))
	}
}

// newBook is an address book (a DHT in server mode) on this machine.
func newBook(t *testing.T) peer.AddrInfo {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"), libp2p.DisableMetrics())
	if err != nil {
		t.Fatal(err)
	}
	d, err := dht.New(h, dht.Mode(dht.ModeServer), dht.ProtocolPrefix("/wittytest"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close(); h.Close() })
	return peer.AddrInfo{ID: h.ID(), Addrs: h.Addrs()}
}

func TestFindingTheServerThroughTheAddressBook(t *testing.T) {
	book := newBook(t)
	devs := newDevList(testID(t, 2))
	n := startNode(t, 1, devs, func(c *NodeConfig) {
		c.Layers.DHT = true
		c.Bootstrap = []peer.AddrInfo{book}
	})
	// Wait until the server is in the book.
	for i := 0; len(n.Host().Network().ConnsToPeer(book.ID)) == 0; i++ {
		if i > 200 {
			t.Fatal("the server never reached the address book")
		}
		time.Sleep(50 * time.Millisecond)
	}
	// The app has the server's ID and nothing else.
	c := startClient(t, 2, n, func(c *ClientConfig) {
		c.Hints = nil
		c.Layers.DHT = true
		c.Bootstrap = []peer.AddrInfo{book}
	})
	waitPath(t, c, PathLocal)
	if code, _, err := get(t, c, "/hello"); err != nil || code != 200 {
		t.Fatalf("%d %v", code, err)
	}
	if len(c.KnownAddrs()) == 0 {
		t.Fatal("the app learnt no addresses to keep for next time")
	}
}

func TestDegradesWithoutTheAddressBook(t *testing.T) {
	// The book is gone (its address refuses connections): hints still work,
	// and starting up doesn't wait for it.
	dead := peer.AddrInfo{ID: mustPeer(t, 9), Addrs: []ma.Multiaddr{ma.StringCast("/ip4/127.0.0.1/tcp/1")}}
	devs := newDevList(testID(t, 2))
	start := time.Now()
	n := startNode(t, 1, devs, func(c *NodeConfig) {
		c.Layers.DHT = true
		c.Bootstrap = []peer.AddrInfo{dead}
	})
	c := startClient(t, 2, n, func(c *ClientConfig) {
		c.Layers.DHT = true
		c.Bootstrap = []peer.AddrInfo{dead}
	})
	if code, _, err := get(t, c, "/hello"); err != nil || code != 200 {
		t.Fatalf("%d %v", code, err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("connecting with known addresses took %v with the address book down", d)
	}
}

func mustPeer(t *testing.T, seed byte) peer.ID {
	t.Helper()
	p, err := peer.Decode(testID(t, seed))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReconnectsWhenTheServerComesBack(t *testing.T) {
	devs := newDevList(testID(t, 2))
	n := startNode(t, 1, devs)
	port := n.Port()
	listen := []string{fmt.Sprintf("/ip4/127.0.0.1/udp/%d/quic-v1", port), fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", port)}
	states := make(chan State, 64)
	c := startClient(t, 2, n, func(c *ClientConfig) { c.OnState = func(s State) { states <- s } })
	waitPath(t, c, PathLocal)
	n.Close()
	s := waitPath(t, c, PathOffline, PathConnecting)
	_ = s
	if _, _, err := get(t, c, "/hello"); err == nil {
		t.Fatal("got an answer from a server that is off")
	}
	// The same server, back on the same port (the same listen addresses may
	// take the system a moment to give back).
	var n2 *Node
	for i := 0; ; i++ {
		var err error
		n2, err = StartNode(NodeConfig{Seed: testSeed(1), Devices: devs, DevicesOnly: true, Handler: testHandler(), Local: true, Listen: listen,
			ConnInfo: func(ctx context.Context, a *Addr) context.Context { return context.WithValue(ctx, addrKey{}, a) }})
		if err == nil && len(n2.Host().Network().ListenAddresses()) == 2 {
			break
		}
		if n2 != nil {
			n2.Close()
		}
		if i > 50 {
			t.Fatalf("could not start the server again: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	defer n2.Close()
	waitPath(t, c, PathLocal)
	if code, _, err := get(t, c, "/hello"); err != nil || code != 200 {
		t.Fatalf("after the server came back: %d %v", code, err)
	}
	// The app was told about each change, in order.
	var paths []string
	for len(states) > 0 {
		paths = append(paths, (<-states).Path)
	}
	joined := strings.Join(paths, " ")
	if !strings.Contains(joined, PathLocal+" ") || !strings.Contains(joined, PathOffline) || paths[len(paths)-1] != PathLocal {
		t.Fatalf("states told to the app: %v", paths)
	}
}

func TestPortMappingIsAnnouncedOnlyWhenItLeadsSomewhere(t *testing.T) {
	devs := newDevList()
	router := &portmap.FakeRouter{External: netip.MustParseAddr("198.51.100.20")}
	n := startNode(t, 1, devs, onFreePort(t), func(c *NodeConfig) {
		c.Layers.PortMap = true
		c.Mapper = portmap.NewFakeHolder(router)
	})
	port := n.Port()
	want := fmt.Sprintf("/ip4/198.51.100.20/udp/%d/quic-v1", port)
	found := false
	for i := 0; i < 200 && !found; i++ {
		for _, a := range n.Addrs() {
			found = found || a == want
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !found {
		t.Fatalf("the mapped address %s was never announced: %v", want, n.Addrs())
	}
	if st := n.Status(); !st.Mapping.Usable() || len(st.Mapping.Mappings) != 2 {
		t.Fatalf("mapping status: %+v", st.Mapping)
	}
	if len(router.Open()) != 2 {
		t.Fatalf("on the router: %v", router.Open())
	}
	n.Close()
	if len(router.Open()) != 0 {
		t.Fatalf("turning it off left mappings on the router: %v", router.Open())
	}

	// A router whose own outside address is another private one (a second
	// router, or an internet provider sharing addresses): mapped, but nothing
	// is announced, and the status says why.
	shared := &portmap.FakeRouter{External: netip.MustParseAddr("100.70.1.2")}
	n2 := startNode(t, 3, devs, onFreePort(t), func(c *NodeConfig) {
		c.Layers.PortMap = true
		c.Mapper = portmap.NewFakeHolder(shared)
	})
	for i := 0; i < 200 && n2.Status().Mapping.ExternalIP == ""; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	if st := n2.Status(); !st.Mapping.Shared || st.Mapping.Usable() {
		t.Fatalf("status behind a shared address: %+v", st.Mapping)
	}
	for _, a := range n2.Addrs() {
		if strings.Contains(a, "100.70.1.2") {
			t.Fatalf("an address that leads nowhere was announced: %s", a)
		}
	}
}

// onFreePort makes a node listen on one port for UDP and TCP, as a real
// server does.
func onFreePort(t *testing.T) nodeOpt {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return func(c *NodeConfig) {
		c.Listen = []string{fmt.Sprintf("/ip4/127.0.0.1/udp/%d/quic-v1", port), fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", port)}
	}
}

func TestNoMappingUnlessTheLayerIsOn(t *testing.T) {
	router := &portmap.FakeRouter{External: netip.MustParseAddr("198.51.100.20")}
	n := startNode(t, 1, newDevList(), func(c *NodeConfig) { c.Mapper = portmap.NewFakeHolder(router) })
	time.Sleep(200 * time.Millisecond)
	if len(router.Open()) != 0 || n.Status().Mapping.On {
		t.Fatal("the router was asked for a mapping with the layer off")
	}
}

func TestCloseIsPromptAndFinal(t *testing.T) {
	devs := newDevList(testID(t, 2))
	n := startNode(t, 1, devs)
	c := startClient(t, 2, n)
	// A request in flight when the node closes ends; nothing hangs.
	done := make(chan error, 1)
	go func() {
		req, _ := http.NewRequest("GET", "http://server/events", nil)
		res, err := c.Transport().RoundTrip(req)
		if err == nil {
			_, err = io.ReadAll(res.Body)
			res.Body.Close()
		}
		done <- err
	}()
	time.Sleep(40 * time.Millisecond)
	start := time.Now()
	n.Close()
	if d := time.Since(start); d > 8*time.Second {
		t.Errorf("closing took %v", d)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("a request outlived the node")
	}
}

// TestThroughputOverLoopback measures what the transport itself costs: 64 MiB
// through one stream between two nodes on this machine, once over QUIC and
// once over TCP. It only reports (go test -v -run Throughput); docs/P2P.md
// quotes the numbers.
func TestThroughputOverLoopback(t *testing.T) {
	if testing.Short() {
		t.Skip("a measurement, not a check")
	}
	const size = 64 << 20
	for _, tr := range []struct{ name, addr string }{
		{"QUIC", "/ip4/127.0.0.1/udp/0/quic-v1"},
		{"TCP (Noise, yamux)", "/ip4/127.0.0.1/tcp/0"},
	} {
		devs := newDevList(testID(t, 2))
		n := startNode(t, 1, devs, func(c *NodeConfig) {
			c.Listen = []string{tr.addr}
			c.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				buf := make([]byte, 256<<10)
				for sent := 0; sent < size; sent += len(buf) {
					if _, err := w.Write(buf); err != nil {
						return
					}
				}
			})
		})
		c := startClient(t, 2, n, func(c *ClientConfig) { c.Listen = []string{tr.addr} })
		waitPath(t, c, PathLocal)
		req, _ := http.NewRequest("GET", "http://server/", nil)
		start := time.Now()
		res, err := c.Transport().RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.Copy(io.Discard, res.Body)
		res.Body.Close()
		d := time.Since(start)
		if err != nil || got != size {
			t.Fatalf("%s: %d bytes, %v", tr.name, got, err)
		}
		t.Logf("%s: %d MiB in %v = %.0f MiB/s (%.0f Mbit/s)", tr.name, size>>20, d.Round(time.Millisecond), float64(size>>20)/d.Seconds(), float64(size)*8/1e6/d.Seconds())
		c.Close()
		n.Close()
	}
}
