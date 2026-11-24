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
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
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
	// OwnRelays are WittyRelays the owner named: registered with, and held a
	// place on before any other relay. DeviceIDs lists the devices they are
	// told may come through them; Name is what their operators see.
	OwnRelays []OwnRelay
	DeviceIDs func() []string
	Name      string
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
	// TalkWait replaces how long an invite's or a code's conversation may
	// take, so tests of slow peers don't wait.
	TalkWait time.Duration
}

// enrolWait and joinWait are the most an invite's or a code's whole
// conversation may take, however slowly the other end sends: a stream that
// trickles a byte at a time is cut off then.
const (
	enrolWait = 15 * time.Second
	joinWait  = 20 * time.Second
)

// talkWait is how long one of those conversations may take on this node.
func (n *Node) talkWait(d time.Duration) time.Duration {
	if n.cfg.TalkWait > 0 {
		return n.cfg.TalkWait
	}
	return d
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
	count                          counters
	joins                          joins // codes typed into the server, waiting for their devices: join.go

	mu             sync.Mutex
	streams        map[peer.ID]int
	buckets        map[network.Conn]*bucket
	temp           map[peer.ID]time.Time // keys let in for a moment: the connection test
	tempNow        func() time.Time      // the time temp is judged by (tests); nil: now
	reachability   network.Reachability
	natUDP         network.NATDeviceType
	natTCP         network.NATDeviceType
	reachable      []ma.Multiaddr
	unreachable    []ma.Multiaddr
	unchecked      []ma.Multiaddr
	relayAddrs     []ma.Multiaddr
	paths          map[peer.ID]string // each connected device's way here, to log when it changes
	ownRelays      []OwnRelayStatus
	ownCircuits    map[int][]ma.Multiaddr // this server's addresses through each WittyRelay it holds a place on
	devicesChanged chan struct{}
	registered     chan struct{} // closed once each WittyRelay has been tried once
	refused        int
	refusedNote    time.Time
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
		streams: map[peer.ID]int{}, buckets: map[network.Conn]*bucket{}, temp: map[peer.ID]time.Time{}, paths: map[peer.ID]string{},
		devicesChanged: make(chan struct{}, 1), registered: make(chan struct{}),
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
	// The owner's WittyRelays come first among the relays a place is held on.
	static := []string{}
	for _, r := range cfg.OwnRelays {
		n.ownRelays = append(n.ownRelays, OwnRelayStatus{Addr: r.Addr, State: "wait"})
		if ai, err := ParseRelayAddr(r.Addr); err == nil {
			for _, a := range ai.Addrs {
				static = append(static, a.String()+"/p2p/"+ai.ID.String())
			}
		}
	}
	static = append(static, cfg.StaticRelays...)
	h, err := NewHost(HostConfig{
		Seed: cfg.Seed, Port: cfg.Port, Server: true, Layers: cfg.Layers, StaticRelays: static,
		RelayService: cfg.RelayService, ExtraAddrs: n.extraAddrs, Log: lg,
		Bootstrap: cfg.Bootstrap, Local: cfg.Local, Listen: cfg.Listen, Options: cfg.Options, PunchTimeout: cfg.PunchTimeout,
		BeforeRelays: func(ctx context.Context) {
			select {
			case <-n.registered:
			case <-ctx.Done():
			case <-time.After(45 * time.Second): // a relay that doesn't answer holds nobody up for long
			}
		},
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
	h.SetStreamHandler(ProtoJoin, n.handleJoin)
	h.Network().Notify(&network.NotifyBundle{DisconnectedF: func(nw network.Network, c network.Conn) {
		p := c.RemotePeer()
		n.mu.Lock()
		delete(n.buckets, c)
		path, had := n.paths[p]
		gone := had && nw.Connectedness(p) != network.Connected
		if gone {
			delete(n.paths, p)
		}
		n.mu.Unlock()
		if gone {
			n.log.Info("peer-to-peer: device disconnected", "session", ConnSession(p.String()), "device", ShortID(p.String()), "path", path)
		}
	}})
	n.wg.Add(3)
	Spawn("p2p wittyrelay", n.keepRegistered)
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

// RelayOffer is what the relay a connection came through said it allows.
func (n *Node) RelayOffer(relay string) (RelayOffer, bool) {
	p, err := peer.Decode(relay)
	if err != nil {
		return RelayOffer{}, false
	}
	return n.host.RelayOfferOf(p)
}

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

// extraAddrs are what this node announces besides what libp2p finds: the
// router's mapping, and its places on the owner's WittyRelays.
func (n *Node) extraAddrs() []ma.Multiaddr {
	out := n.mapped()
	n.mu.Lock()
	for _, list := range n.ownCircuits {
		out = append(out, list...)
	}
	n.mu.Unlock()
	return out
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
				proto, was := "tcp", &n.natTCP
				if ev.TransportProtocol == network.NATTransportUDP {
					proto, was = "udp", &n.natUDP
				}
				if *was != ev.NatDeviceType {
					// open: a hole can be punched; strict: only a relay reaches this server from away.
					n.log.Info("peer-to-peer: the router's NAT type is known", "proto", proto, "nat", natWord(ev.NatDeviceType), "was", natWord(*was))
				}
				*was = ev.NatDeviceType
			case event.EvtHostReachableAddrsChanged:
				n.reachable, n.unreachable, n.unchecked = ev.Reachable, ev.Unreachable, ev.Unknown
				n.log.Debug("peer-to-peer: addresses checked from outside", "reachable", len(ev.Reachable), "unreachable", len(ev.Unreachable), "unchecked", len(ev.Unknown))
			case event.EvtAutoRelayAddrsUpdated:
				before, after := relayIDs(n.relayAddrs), relayIDs(ev.RelayAddrs)
				n.relayAddrs = ev.RelayAddrs
				for _, a := range ev.RelayAddrs {
					if r := relayOf(a); r != "" {
						// What it allows: shown in Settings, and kept to.
						n.host.learnOffer(r, nil)
					}
				}
				if before != after {
					if after == "" {
						n.log.Info("peer-to-peer: no relay holds a place for this server now")
					} else {
						n.log.Info("peer-to-peer: relays holding a place for this server", "relays", after)
					}
				}
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
	now := n.tempTime()
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
	return ok && n.tempTime().Before(until)
}

// tempTime is the time keys let in for a moment are judged by. Call with mu held.
func (n *Node) tempTime() time.Time {
	if n.tempNow != nil {
		return n.tempNow()
	}
	return time.Now()
}

// remoteOf describes the other end of a stream.
func (n *Node) remoteOf(s network.Stream, known bool) *Addr {
	c := s.Conn()
	ra := c.RemoteMultiaddr()
	relayed := c.Stat().Limited || isRelayed(ra)
	ip := ipOf(ra)
	var relay string
	if relayed {
		relay = relayOf(ra).String()
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
	return &Addr{IP: presentedIP(ip, relayed, known, c.RemotePeer()), Peer: c.RemotePeer().String(), Relayed: relayed, Relay: relay, Known: known}
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
		n.count.refusedNotListed.Add(1)
		n.noteRefused(s, "not on the list of devices")
		_ = s.Reset()
		return
	}
	n.mu.Lock()
	if n.streams[p] >= maxStreamsPerPeer {
		n.mu.Unlock()
		n.count.refusedTooMany.Add(1)
		n.noteRefused(s, "too many connections from one device")
		_ = s.Reset()
		return
	}
	n.streams[p]++
	n.mu.Unlock()
	var b *bucket
	addr := n.remoteOf(s, known)
	if addr.Relayed {
		if limit := n.RelayStreamLimit(addr.Relay); limit > 0 {
			n.mu.Lock()
			if b = n.buckets[s.Conn()]; b == nil {
				b = newBucket(limit)
				n.buckets[s.Conn()] = b
			}
			n.mu.Unlock()
		}
	}
	if known {
		// Noted before the connection is handed over, so that whatever the
		// device asks first is already counted as coming from it, this way.
		n.host.ConnManager().Protect(p, "wittymedia")
		path := "direct"
		if addr.Relayed {
			path = "relayed"
		}
		n.notePath(p, path)
		n.cfg.Devices.Seen(p.String(), path)
	}
	var sent *atomic.Int64
	if addr.Relayed {
		n.count.streamsRelayed.Add(1)
		sent = &n.count.relayedBytes
	} else {
		n.count.streamsDirect.Add(1)
	}
	conn := &streamConn{Stream: s, local: n.ln.addr, remote: addr, bucket: b, sent: sent, onClose: func() {
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
	}
}

// notePath logs a device's first connection and every change in how it reaches
// this server, direct or through a relay, under the device's connection session.
func (n *Node) notePath(p peer.ID, path string) {
	n.mu.Lock()
	was, had := n.paths[p]
	n.paths[p] = path
	n.mu.Unlock()
	switch {
	case !had:
		n.log.Info("peer-to-peer: device connected", "session", ConnSession(p.String()), "device", ShortID(p.String()), "path", path)
	case was != path:
		n.count.pathChanges.Add(1)
		n.log.Info("peer-to-peer: path changed", "session", ConnSession(p.String()), "device", ShortID(p.String()), "from", was, "to", path)
	}
}

// relayIDs is the relays in a list of addresses, by their short IDs, for the log.
func relayIDs(list []ma.Multiaddr) string {
	var ids []string
	for _, a := range list {
		if r := relayOf(a); r != "" {
			if id := ShortID(r.String()); !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
	}
	slices.Sort(ids)
	return strings.Join(ids, ",")
}

func (n *Node) handleEnrol(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(n.talkWait(enrolWait)))
	id := s.Conn().RemotePeer().String()
	addr := n.remoteOf(s, false)
	answer := func(r EnrolResponse) { _ = writeMessage(s, r) }
	if !n.allowTry(id, addr.IP) {
		n.count.enrolsRefused.Add(1)
		n.noteRefused(s, "too many tries at an invite")
		answer(EnrolResponse{Reason: "too-many-tries", Error: "Too many tries. Wait a minute and try again."})
		return
	}
	n.count.enrols.Add(1)
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
		res.Addrs = n.Addrs()
	}
	answer(res)
}

// allowTry counts one try at an invite or a code. The narrowest limit is
// asked first, and a try it refuses goes no further: whoever is over their own
// share can't use up the share of every other key, or everyone's together.
// Making new keys costs nothing, so the address is what holds a stranger.
func (n *Node) allowTry(id string, ip netip.Addr) bool {
	return n.enrolAddr.allow(tryKey(ip)) && n.enrolPeer.allow(id) && n.enrolAll.allow("all")
}

// tryKey is what tries are counted under for an address: an IPv6 address by
// its /64, as one home or one machine is given a whole /64 to pick from.
func tryKey(ip netip.Addr) string {
	ip = ip.Unmap()
	if ip.Is6() {
		p, err := ip.Prefix(64)
		if err == nil {
			return p.String()
		}
	}
	return ip.String()
}

// Kick closes every connection from a device: what revoking it does at once.
func (n *Node) Kick(id string) {
	if p, err := peer.Decode(id); err == nil {
		n.host.ConnManager().Unprotect(p, "wittymedia")
		_ = n.host.Network().ClosePeer(p)
	}
}

// Addrs are the node's current addresses, for apps to remember: those an app
// can use, which leaves out a relay's WebSocket and WebRTC addresses.
func (n *Node) Addrs() []string { return addrStrings(ma.FilterAddrs(n.host.Addrs(), spoken)) }

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
	// Offer is what the relay said it allows, and Allows the same in words;
	// both empty for a relay that didn't say (a public one).
	Offer  *RelayOffer `json:"offer,omitempty"`
	Allows string      `json:"allows,omitempty"`
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
	// Offer is what this server tells others it allows, as a community relay.
	Offer     *RelayOffer      `json:"offer,omitempty"`
	OwnRelays []OwnRelayStatus `json:"ownRelays"`
	Devices   []DeviceConn     `json:"devices"`
	Counters  Counters         `json:"counters"`
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
	if n.cfg.RelayService != nil {
		o := offerOf(*n.cfg.RelayService)
		st.Offer = &o
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
	st.OwnRelays = append([]OwnRelayStatus{}, n.ownRelays...)
	st.Counters = n.count.snapshot()
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
		rs := RelayStatus{ID: r.String(), Addr: a.String(), Community: h.IsCommunityRelay(r)}
		if o, ok := h.RelayOfferOf(r); ok {
			rs.Offer, rs.Allows = &o, o.Words()
		}
		st.Relays = append(st.Relays, rs)
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
