// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/wittywilla/wittymediaserver/internal/portmap"
)

// Devices is the server's list of devices that may connect.
type Devices interface {
	// Allowed says the device with this ID is on the list.
	Allowed(id string) bool
	// Seen notes that the device connected just now, and how.
	Seen(id, path string)
}

// NodeConfig is what the server's node is made from.
type NodeConfig struct {
	Seed         []byte
	Port         int
	Layers       Layers
	StaticRelays []string
	RelayService *RelayLimits
	// DevicesOnly refuses every HTTP stream from a key that isn't on the list.
	// Off, anyone who knows the server's ID reaches the sign-in page.
	DevicesOnly bool
	// RelayBitrate limits what is sent through a relay, in bits per second
	// (0: no limit). Relays are other people's connections.
	RelayBitrate int64
	Devices      Devices
	// Enrol answers a device showing an invite. from is the address it is
	// counted under.
	Enrol func(ctx context.Context, id string, req EnrolRequest, from netip.Addr) EnrolResponse
	// Handler is the server's ordinary HTTP handler; Wrap counts the
	// connections given to it against the server's limits.
	Handler  http.Handler
	Wrap     func(net.Listener) net.Listener
	ConnInfo func(ctx context.Context, a *Addr) context.Context
	Log      *slog.Logger
	ErrorLog *log.Logger

	// For tests: see HostConfig. Mapper replaces the router.
	Bootstrap []peer.AddrInfo
	Local     bool
	Listen    []string
	Options   []libp2p.Option
	Mapper    *portmap.Holder
	// PunchTimeout: see HostConfig.
	PunchTimeout time.Duration
}

// maxStreamsPerPeer is how many HTTP connections one device may hold open: a
// player, its subtitles and artwork, a few sockets, and room to spare.
const maxStreamsPerPeer = 48

// Node is the server's end.
type Node struct {
	cfg  NodeConfig
	log  *slog.Logger
	host *Host
	ln   *listener
	http *http.Server
	maps *portmap.Holder

	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started time.Time

	enrolAll, enrolPeer, enrolAddr *window

	mu           sync.Mutex
	streams      map[peer.ID]int
	buckets      map[network.Conn]*bucket
	temp         map[peer.ID]time.Time // keys let in for a moment: the connection test
	reachability network.Reachability
	natUDP       network.NATDeviceType
	natTCP       network.NATDeviceType
	reachable    []ma.Multiaddr
	unreachable  []ma.Multiaddr
	unchecked    []ma.Multiaddr
	relayAddrs   []ma.Multiaddr
	refused      int
	refusedNote  time.Time
}

// StartNode starts the server's node. The feature is on from here on: it
// listens, announces itself as its layers allow, and asks the router for
// what its layers allow.
func StartNode(cfg NodeConfig) (*Node, error) {
	if cfg.Handler == nil || cfg.Devices == nil {
		return nil, errors.New("p2p: a node needs a handler and a device list")
	}
	lg := cfg.Log
	if lg == nil {
		lg = slog.New(slog.DiscardHandler)
	}
	ctx, cancel := context.WithCancel(context.Background())
	n := &Node{cfg: cfg, log: lg, ctx: ctx, cancel: cancel, started: time.Now(),
		streams: map[peer.ID]int{}, buckets: map[network.Conn]*bucket{}, temp: map[peer.ID]time.Time{},
		// An invite's secret is 128 bits, so these are not what keeps guessing
		// out; they keep strangers from keeping the server busy.
		enrolAll:  newWindow(60, time.Minute, 1),
		enrolPeer: newWindow(5, time.Minute, 512),
		enrolAddr: newWindow(10, time.Minute, 512),
	}
	if cfg.Layers.PortMap || cfg.Layers.IPv6 {
		n.maps = cfg.Mapper
		if n.maps == nil {
			n.maps = &portmap.Holder{Log: lg}
		}
		n.maps.IPv6 = cfg.Layers.IPv6
	}
	h, err := NewHost(HostConfig{
		Seed: cfg.Seed, Port: cfg.Port, Server: true, Layers: cfg.Layers, StaticRelays: cfg.StaticRelays,
		RelayService: cfg.RelayService, ExtraAddrs: n.mapped, Log: lg,
		Bootstrap: cfg.Bootstrap, Local: cfg.Local, Listen: cfg.Listen, Options: cfg.Options, PunchTimeout: cfg.PunchTimeout,
	})
	if err != nil {
		cancel()
		return nil, err
	}
	n.host = h
	n.ln = newListener(&Addr{Peer: h.ID().String()})
	var ln net.Listener = n.ln
	if cfg.Wrap != nil {
		ln = cfg.Wrap(ln)
	}
	n.http = &http.Server{
		Handler:           cfg.Handler,
		ErrorLog:          cfg.ErrorLog,
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			// Every connection of this server is a peer-to-peer one, whatever
			// wraps it: one whose address can't be read still says so.
			a, _ := c.RemoteAddr().(*Addr)
			if a == nil {
				a = &Addr{IP: strangerIP}
			}
			if cfg.ConnInfo != nil {
				return cfg.ConnInfo(ctx, a)
			}
			return ctx
		},
	}
	sub, err := h.EventBus().Subscribe([]any{
		new(event.EvtLocalReachabilityChanged), new(event.EvtNATDeviceTypeChanged),
		new(event.EvtHostReachableAddrsChanged), new(event.EvtAutoRelayAddrsUpdated),
	})
	if err != nil {
		_ = h.Close()
		cancel()
		return nil, err
	}
	h.SetStreamHandler(ProtoHTTP, n.handleHTTP)
	h.SetStreamHandler(ProtoEnrol, n.handleEnrol)
	h.Network().Notify(&network.NotifyBundle{DisconnectedF: func(_ network.Network, c network.Conn) {
		n.mu.Lock()
		delete(n.buckets, c)
		n.mu.Unlock()
	}})
	n.wg.Add(2)
	Spawn("p2p http", func() {
		defer n.wg.Done()
		if err := n.http.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			lg.Error("peer-to-peer connections stopped being served", "err", err)
		}
	})
	Spawn("p2p events", func() {
		defer n.wg.Done()
		defer sub.Close()
		n.watch(sub)
	})
	if n.maps != nil {
		port := n.Port()
		n.maps.Start([]portmap.Want{{Proto: portmap.UDP, Port: port}, {Proto: portmap.TCP, Port: port}})
	}
	lg.Info("peer-to-peer connections are on", "id", h.ID().String(), "port", n.Port(), "devicesOnly", cfg.DevicesOnly,
		"dht", cfg.Layers.DHT, "relay", cfg.Layers.Relay, "holePunch", cfg.Layers.HolePunch, "portMap", cfg.Layers.PortMap, "ipv6", cfg.Layers.IPv6)
	return n, nil
}

// ID is the server's connection ID.
func (n *Node) ID() string { return n.host.ID().String() }

// Host is the node underneath, for tests and the connection test.
func (n *Node) Host() *Host { return n.host }

// Port is the port the node really listens on.
func (n *Node) Port() int {
	for _, a := range n.host.Network().ListenAddresses() {
		for _, code := range []int{ma.P_UDP, ma.P_TCP} {
			if v, err := a.ValueForProtocol(code); err == nil {
				var p int
				for _, c := range v {
					p = p*10 + int(c-'0')
				}
				if p > 0 {
					return p
				}
			}
		}
	}
	return n.cfg.Port
}

// mapped is what the router's port mapping adds to this node's addresses.
// Nothing is announced unless the router's outside address is a public one:
// behind a second router or a shared address the mapping leads nowhere.
func (n *Node) mapped() []ma.Multiaddr {
	if n.maps == nil {
		return nil
	}
	st := n.maps.Status()
	if !st.Usable() {
		return nil
	}
	ip, err := netip.ParseAddr(st.ExternalIP)
	if err != nil {
		return nil
	}
	return mappedAddrs(ip, st.Mappings[0].ExternalPort)
}

func (n *Node) watch(sub event.Subscription) {
	for {
		select {
		case <-n.ctx.Done():
			return
		case e, ok := <-sub.Out():
			if !ok {
				return
			}
			n.mu.Lock()
			switch ev := e.(type) {
			case event.EvtLocalReachabilityChanged:
				if n.reachability != ev.Reachability {
					n.log.Info("peer-to-peer: reachability from outside changed", "now", reachWord(ev.Reachability))
				}
				n.reachability = ev.Reachability
			case event.EvtNATDeviceTypeChanged:
				if ev.TransportProtocol == network.NATTransportUDP {
					n.natUDP = ev.NatDeviceType
				} else {
					n.natTCP = ev.NatDeviceType
				}
			case event.EvtHostReachableAddrsChanged:
				n.reachable, n.unreachable, n.unchecked = ev.Reachable, ev.Unreachable, ev.Unknown
			case event.EvtAutoRelayAddrsUpdated:
				n.relayAddrs = ev.RelayAddrs
			}
			n.mu.Unlock()
		}
	}
}

// AllowBriefly lets one key open streams for a little while without being on
// the list: the connection test dials in as a device nobody enrolled.
func (n *Node) AllowBriefly(id string, d time.Duration) {
	p, err := peer.Decode(id)
	if err != nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	now := time.Now()
	for k, until := range n.temp {
		if now.After(until) {
			delete(n.temp, k)
		}
	}
	if len(n.temp) < 8 {
		n.temp[p] = now.Add(d)
	}
}

func (n *Node) briefly(p peer.ID) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	until, ok := n.temp[p]
	return ok && time.Now().Before(until)
}

// remoteOf describes the other end of a stream.
func (n *Node) remoteOf(s network.Stream, known bool) *Addr {
	c := s.Conn()
	ra := c.RemoteMultiaddr()
	relayed := c.Stat().Limited || isRelayed(ra)
	ip := ipOf(ra)
	if relayed {
		// The far end's own address isn't known through a relay: the relay's
		// is, from this node's connection to it.
		ip = netip.Addr{}
		if r := relayOf(ra); r != "" {
			for _, rc := range n.host.Network().ConnsToPeer(r) {
				ip = ipOf(rc.RemoteMultiaddr())
				break
			}
		}
	}
	return &Addr{IP: presentedIP(ip, relayed, known, c.RemotePeer()), Peer: c.RemotePeer().String(), Relayed: relayed, Known: known}
}

func (n *Node) noteRefused(s network.Stream, why string) {
	n.mu.Lock()
	n.refused++
	note := time.Since(n.refusedNote) >= time.Minute
	count := n.refused
	if note {
		n.refusedNote, n.refused = time.Now(), 0
	}
	n.mu.Unlock()
	if note {
		// At most once a minute: someone knocking shouldn't fill the log too.
		n.log.Warn("peer-to-peer: connection refused", "why", why, "device", ShortID(s.Conn().RemotePeer().String()),
			"from", n.remoteOf(s, false).IP.String(), "refused", count)
	}
}

func (n *Node) handleHTTP(s network.Stream) {
	p := s.Conn().RemotePeer()
	known := n.cfg.Devices.Allowed(p.String())
	if n.cfg.DevicesOnly && !known && !n.briefly(p) {
		n.noteRefused(s, "not on the list of devices")
		_ = s.Reset()
		return
	}
	n.mu.Lock()
	if n.streams[p] >= maxStreamsPerPeer {
		n.mu.Unlock()
		n.noteRefused(s, "too many connections from one device")
		_ = s.Reset()
		return
	}
	n.streams[p]++
	n.mu.Unlock()
	var b *bucket
	addr := n.remoteOf(s, known)
	if addr.Relayed && n.cfg.RelayBitrate > 0 {
		n.mu.Lock()
		if b = n.buckets[s.Conn()]; b == nil {
			b = newBucket(n.cfg.RelayBitrate)
			n.buckets[s.Conn()] = b
		}
		n.mu.Unlock()
	}
	conn := &streamConn{Stream: s, local: n.ln.addr, remote: addr, bucket: b, onClose: func() {
		n.mu.Lock()
		if n.streams[p] <= 1 {
			delete(n.streams, p)
		} else {
			n.streams[p]--
		}
		n.mu.Unlock()
	}}
	if err := n.ln.offer(conn); err != nil {
		_ = s.Reset()
		conn.once.Do(conn.onClose)
		return
	}
	if known {
		n.host.ConnManager().Protect(p, "wittymedia")
		path := "direct"
		if addr.Relayed {
			path = "relayed"
		}
		n.cfg.Devices.Seen(p.String(), path)
	}
}

func (n *Node) handleEnrol(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(15 * time.Second))
	id := s.Conn().RemotePeer().String()
	addr := n.remoteOf(s, false)
	answer := func(r EnrolResponse) { _ = writeMessage(s, r) }
	// Each is counted, so no "or": a stranger over one limit still uses up
	// their share of the others.
	okAll, okPeer, okAddr := n.enrolAll.allow("all"), n.enrolPeer.allow(id), n.enrolAddr.allow(addr.IP.String())
	if !okAll || !okPeer || !okAddr {
		n.noteRefused(s, "too many tries at an invite")
		answer(EnrolResponse{Reason: "too-many-tries", Error: "Too many tries. Wait a minute and try again."})
		return
	}
	var req EnrolRequest
	if err := readMessage(s, &req); err != nil || req.V != 1 {
		answer(EnrolResponse{Reason: "wrong", Error: "That isn't an invite this server understands."})
		return
	}
	if n.cfg.Enrol == nil {
		answer(EnrolResponse{Reason: "off", Error: "This server takes no invites."})
		return
	}
	ctx, cancel := context.WithTimeout(n.ctx, 10*time.Second)
	defer cancel()
	res := n.cfg.Enrol(ctx, id, req, addr.IP)
	if res.OK {
		res.Addrs = addrStrings(n.host.Addrs())
	}
	answer(res)
}

// Kick closes every connection from a device: what revoking it does at once.
func (n *Node) Kick(id string) {
	if p, err := peer.Decode(id); err == nil {
		n.host.ConnManager().Unprotect(p, "wittymedia")
		_ = n.host.Network().ClosePeer(p)
	}
}

// Addrs are the node's current addresses, for apps to remember.
func (n *Node) Addrs() []string { return addrStrings(n.host.Addrs()) }

// Close stops the node: streams end, mappings on the router are removed.
func (n *Node) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_ = n.http.Shutdown(ctx)
	cancel()
	_ = n.http.Close()
	_ = n.ln.Close()
	if n.maps != nil {
		n.maps.Stop()
	}
	n.cancel()
	_ = n.host.Close()
	n.wg.Wait()
	n.log.Info("peer-to-peer connections are off")
}

// ---------------------------------------------------------------- status

// DeviceConn is one device connected now.
type DeviceConn struct {
	ID      string `json:"id"`
	Path    string `json:"path"` // direct | relayed
	Addr    string `json:"addr,omitempty"`
	Streams int    `json:"streams"`
	Since   int64  `json:"since"`
}

// RelayStatus is one relay this node holds a place on.
type RelayStatus struct {
	ID        string `json:"id"`
	Addr      string `json:"addr"`
	Community bool   `json:"community"`
}

// Status is the state of every layer, for Settings and the diagnostics.
type Status struct {
	Running      bool                 `json:"running"`
	ID           string               `json:"id"`
	Port         int                  `json:"port"`
	Started      int64                `json:"started"`
	Layers       Layers               `json:"layers"`
	Reachability string               `json:"reachability"` // public | private | unknown
	NATUDP       string               `json:"natUdp"`       // open | strict | unknown
	NATTCP       string               `json:"natTcp"`
	Addrs        []string             `json:"addrs"`
	Reachable    []string             `json:"reachable"`   // proved from outside
	Unreachable  []string             `json:"unreachable"` // tried from outside and failed
	Unchecked    []string             `json:"unchecked"`
	Mapping      portmap.HolderStatus `json:"mapping"`
	Relays       []RelayStatus        `json:"relays"`
	NetworkPeers int                  `json:"networkPeers"`
	DHTPeers     int                  `json:"dhtPeers"`
	RelayService bool                 `json:"relayService"`
	Devices      []DeviceConn         `json:"devices"`
}

func reachWord(r network.Reachability) string {
	switch r {
	case network.ReachabilityPublic:
		return "public"
	case network.ReachabilityPrivate:
		return "private"
	}
	return "unknown"
}

func natWord(t network.NATDeviceType) string {
	switch t {
	case network.NATDeviceTypeEndpointIndependent:
		return "open"
	case network.NATDeviceTypeEndpointDependent:
		return "strict"
	}
	return "unknown"
}

// Status reports the node. It asks nothing of the network.
func (n *Node) Status() Status {
	h := n.host
	st := Status{Running: true, ID: n.ID(), Port: n.Port(), Started: n.started.Unix(), Layers: n.cfg.Layers,
		Addrs: addrStrings(h.Addrs()), NetworkPeers: len(h.Network().Peers()), RelayService: n.cfg.RelayService != nil,
		Reachable: []string{}, Unreachable: []string{}, Unchecked: []string{}, Relays: []RelayStatus{}, Devices: []DeviceConn{}}
	if h.DHT != nil {
		st.DHTPeers = h.DHT.RoutingTable().Size()
	}
	if n.maps != nil {
		st.Mapping = n.maps.Status()
	}
	n.mu.Lock()
	st.Reachability, st.NATUDP, st.NATTCP = reachWord(n.reachability), natWord(n.natUDP), natWord(n.natTCP)
	st.Reachable = append(st.Reachable, addrStrings(n.reachable)...)
	st.Unreachable = append(st.Unreachable, addrStrings(n.unreachable)...)
	st.Unchecked = append(st.Unchecked, addrStrings(n.unchecked)...)
	relays := append([]ma.Multiaddr(nil), n.relayAddrs...)
	streams := map[peer.ID]int{}
	for p, c := range n.streams {
		streams[p] = c
	}
	n.mu.Unlock()
	seen := map[peer.ID]bool{}
	for _, a := range relays {
		r := relayOf(a)
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		st.Relays = append(st.Relays, RelayStatus{ID: r.String(), Addr: a.String(), Community: h.IsCommunityRelay(r)})
	}
	for _, c := range h.Network().Conns() {
		p := c.RemotePeer()
		if !n.cfg.Devices.Allowed(p.String()) {
			continue
		}
		d := DeviceConn{ID: p.String(), Path: "direct", Streams: streams[p], Since: c.Stat().Opened.Unix()}
		if c.Stat().Limited || isRelayed(c.RemoteMultiaddr()) {
			d.Path = "relayed"
		} else {
			d.Addr = ipOf(c.RemoteMultiaddr()).String()
		}
		st.Devices = append(st.Devices, d)
	}
	sort.Slice(st.Devices, func(i, j int) bool {
		if st.Devices[i].ID != st.Devices[j].ID {
			return st.Devices[i].ID < st.Devices[j].ID
		}
		return st.Devices[i].Path < st.Devices[j].Path // a direct connection before the relayed one it replaced
	})
	return st
}
