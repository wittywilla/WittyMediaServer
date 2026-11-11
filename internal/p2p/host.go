// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/multiformats/go-multihash"
)

// Spawn starts a goroutine. The server points it at its own helper, which
// reports a panic as a crash; elsewhere a panic is logged and survived.
var Spawn = func(name string, fn func()) {
	go func() {
		defer func() {
			if p := recover(); p != nil {
				slog.Error("peer-to-peer: a background job failed", "job", name, "panic", fmt.Sprint(p))
			}
		}()
		fn()
	}()
}

// Layers are the ways of connecting that can be turned off one by one. With
// all of them off, only addresses an app already knows are tried.
type Layers struct {
	// DHT looks the other end up on, and announces this node to, the public
	// IPFS (Amino) network: strangers' computers that keep a shared address
	// book. They learn this node's ID and IP address; nothing else.
	DHT bool `json:"dht"`
	// Relay lets a public relay pass on (encrypted) connections, which is also
	// how two routers are talked into letting a direct one through.
	Relay bool `json:"relay"`
	// HolePunch tries to turn a relayed connection into a direct one.
	HolePunch bool `json:"holePunch"`
	// PortMap asks the home router to forward this node's port (server only).
	PortMap bool `json:"portMap"`
	// IPv6 asks the home router to let IPv6 connections in (server only).
	IPv6 bool `json:"ipv6"`
}

// AllLayers is every layer on.
func AllLayers() Layers { return Layers{DHT: true, Relay: true, HolePunch: true, PortMap: true, IPv6: true} }

// RelayLimits are what a community relay gives each connection it passes on.
type RelayLimits struct {
	Duration        time.Duration
	Data            int64 // bytes each way
	MaxReservations int
	MaxCircuits     int // per peer
}

// DefaultRelayLimits suit a home connection lending a little: two hours and
// 2 GiB per connection, for at most 16 servers at once.
func DefaultRelayLimits() RelayLimits {
	return RelayLimits{Duration: 2 * time.Hour, Data: 2 << 30, MaxReservations: 16, MaxCircuits: 4}
}

// HostConfig is what a node is made from.
type HostConfig struct {
	Seed   []byte
	Port   int  // to listen on; 0 picks one
	// Listen replaces the usual addresses to listen on (every network card,
	// UDP for QUIC and TCP, IPv4 and IPv6, on Port). For tests.
	Listen []string
	Server bool // a server keeps more connections and takes relay reservations
	Layers Layers
	// StaticRelays are relays to use whatever else is found, as multiaddrs
	// ending in /p2p/<id>: a friend's server, or one of your own elsewhere.
	StaticRelays []string
	// RelayService makes this node a community relay while it is reachable.
	RelayService *RelayLimits
	// ExtraAddrs are addresses to announce beside the ones libp2p finds by
	// itself: what the router's port mapping gave.
	ExtraAddrs func() []ma.Multiaddr
	Log        *slog.Logger

	// For tests, which must never reach the internet: bootstrap peers of their
	// own (a nil slice with Local unset means the public ones), a DHT that
	// accepts this machine's own addresses, and options added last.
	Bootstrap []peer.AddrInfo
	Local     bool
	Options   []libp2p.Option
}

// Host is a running node.
type Host struct {
	host.Host
	DHT *dht.IpfsDHT
	cfg HostConfig
	log *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	closed atomic.Bool
	// ready is closed once Host and DHT are set: libp2p starts asking for
	// relays and addresses while it is still being put together.
	ready chan struct{}
}

// relayCID is the DHT key community relays are announced under.
func relayCID() cid.Cid {
	h, _ := multihash.Sum([]byte(RelayNamespace), multihash.SHA2_256, -1)
	return cid.NewCidV1(cid.Raw, h)
}

// limits bounds what the node may use whatever the network does: libp2p's
// defaults scaled to a small share of a small machine's memory.
func limits(server bool) (network.ResourceManager, error) {
	mem, fds := int64(48<<20), 128
	if server {
		mem, fds = 96<<20, 256
	}
	l := rcmgr.DefaultLimits
	libp2p.SetDefaultServiceLimits(&l)
	return rcmgr.NewResourceManager(rcmgr.NewFixedLimiter(l.Scale(mem, fds)))
}

// NewHost starts a node. It listens straight away; finding the rest of the
// network happens in the background and failing at it is not an error.
func NewHost(cfg HostConfig) (*Host, error) {
	key, err := keyFromSeed(cfg.Seed)
	if err != nil {
		return nil, err
	}
	log := cfg.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h := &Host{cfg: cfg, log: log, ctx: ctx, cancel: cancel, ready: make(chan struct{})}

	lo, hi := 8, 24
	if cfg.Server {
		lo, hi = 16, 48
	}
	cm, err := connmgr.NewConnManager(lo, hi, connmgr.WithGracePeriod(time.Minute))
	if err != nil {
		cancel()
		return nil, err
	}
	rm, err := limits(cfg.Server)
	if err != nil {
		cancel()
		return nil, err
	}
	listen := cfg.Listen
	if listen == nil {
		listen = []string{
			fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", cfg.Port),
			fmt.Sprintf("/ip6/::/udp/%d/quic-v1", cfg.Port),
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", cfg.Port),
			fmt.Sprintf("/ip6/::/tcp/%d", cfg.Port),
		}
	}
	opts := []libp2p.Option{
		libp2p.Identity(key),
		libp2p.UserAgent(UserAgent),
		libp2p.ListenAddrStrings(listen...),
		// QUIC first: one UDP port, quick to set up, and what hole punching
		// works best with. TCP for networks that let no UDP out.
		libp2p.Transport(libp2pquic.NewTransport),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Security(noise.ID, noise.New),
		libp2p.Muxer(yamux.ID, yamux.DefaultTransport),
		libp2p.ConnectionManager(cm),
		libp2p.ResourceManager(rm),
		libp2p.DisableMetrics(),
		libp2p.EnableAutoNATv2(),
		libp2p.WithDialTimeout(30 * time.Second),
		libp2p.AddrsFactory(h.announce),
	}
	if cfg.Layers.Relay {
		opts = append(opts, libp2p.EnableRelay())
		if cfg.Server {
			// Places on two relays, chosen once two candidates are known or after
			// twenty seconds with fewer: a server's first minutes matter less
			// than not settling for the first relay that happens to answer.
			ar := []autorelay.Option{autorelay.WithNumRelays(2), autorelay.WithMinCandidates(2), autorelay.WithBootDelay(20 * time.Second)}
			if cfg.Local {
				ar = []autorelay.Option{autorelay.WithNumRelays(2), autorelay.WithMinCandidates(1), autorelay.WithBootDelay(0), autorelay.WithMinInterval(200 * time.Millisecond)}
			}
			opts = append(opts, libp2p.EnableAutoRelayWithPeerSource(h.relaySource, ar...))
		}
	} else {
		opts = append(opts, libp2p.DisableRelay())
	}
	if cfg.Layers.HolePunch && cfg.Layers.Relay {
		opts = append(opts, libp2p.EnableHolePunching())
	}
	if cfg.RelayService != nil {
		rl := *cfg.RelayService
		res := relayv2.DefaultResources()
		res.Limit = &relayv2.RelayLimit{Duration: rl.Duration, Data: rl.Data}
		res.MaxReservations, res.MaxCircuits = rl.MaxReservations, rl.MaxCircuits
		res.MaxReservationsPerIP = 2
		opts = append(opts, libp2p.EnableRelayService(relayv2.WithResources(res)))
	}
	if cfg.Layers.DHT {
		opts = append(opts, libp2p.Routing(func(bh host.Host) (routing.PeerRouting, error) {
			d, err := newDHT(bh, cfg)
			h.DHT = d
			return d, err
		}))
	}
	opts = append(opts, cfg.Options...)
	lh, err := libp2p.New(opts...)
	if err != nil {
		cancel()
		return nil, err
	}
	h.Host = lh
	close(h.ready)
	if cfg.RelayService != nil {
		// Only a marker: an app asks whether a relay is one of these before it
		// sends anything large through it.
		lh.SetStreamHandler(ProtoRelay, func(s network.Stream) { _ = s.Close() })
	}
	if h.DHT != nil {
		// Bootstrap only starts the DHT's own upkeep; it keeps trying by itself
		// while the network can't be reached.
		if err := h.DHT.Bootstrap(ctx); err != nil {
			log.Warn("the public address book could not be started", "err", err)
		}
		if cfg.RelayService != nil {
			h.wg.Add(1)
			Spawn("p2p relay announce", h.announceRelay)
		}
	}
	return h, nil
}

func newDHT(h host.Host, cfg HostConfig) (*dht.IpfsDHT, error) {
	opts := []dht.Option{dht.Mode(dht.ModeClient)}
	if cfg.Local {
		// A network of this machine's own, for tests: its own protocol name, so
		// it can never join the public one, and no demand for public addresses.
		opts = append(opts, dht.ProtocolPrefix("/wittytest"), dht.BootstrapPeers(cfg.Bootstrap...))
	} else {
		boot := cfg.Bootstrap
		if boot == nil {
			boot = dht.GetDefaultBootstrapPeerAddrInfos()
		}
		opts = append(opts, dht.BootstrapPeers(boot...),
			dht.QueryFilter(dht.PublicQueryFilter),
			dht.RoutingTableFilter(dht.PublicRoutingTableFilter),
			dht.AddressFilter(func(addrs []ma.Multiaddr) []ma.Multiaddr { return ma.FilterAddrs(addrs, manet.IsPublicAddr) }))
	}
	return dht.New(h, opts...)
}

// announce is what this node says its addresses are: what libp2p found, plus
// what the router mapped.
func (h *Host) announce(addrs []ma.Multiaddr) []ma.Multiaddr {
	if h.cfg.ExtraAddrs == nil {
		return addrs
	}
	seen := map[string]bool{}
	for _, a := range addrs {
		seen[string(a.Bytes())] = true
	}
	for _, a := range h.cfg.ExtraAddrs() {
		if !seen[string(a.Bytes())] {
			seen[string(a.Bytes())] = true
			addrs = append(addrs, a)
		}
	}
	return addrs
}

// StaticRelayInfos reads the configured relays, leaving out the unreadable.
func (h *Host) StaticRelayInfos() []peer.AddrInfo { return staticRelays(h.cfg.StaticRelays) }

func staticRelays(list []string) []peer.AddrInfo {
	var out []peer.AddrInfo
	for _, s := range list {
		a, err := ma.NewMultiaddr(s)
		if err != nil {
			continue
		}
		ai, err := peer.AddrInfoFromP2pAddr(a)
		if err != nil {
			continue
		}
		out = append(out, *ai)
	}
	return out
}

// relaySource offers relays to reserve a place on, best first: the owner's
// own choices, community relays (which pass on whole films), then any public
// node of the network (which pass on only enough to set up a direct
// connection). It is asked again every so often while places are missing.
func (h *Host) relaySource(ctx context.Context, num int) <-chan peer.AddrInfo {
	out := make(chan peer.AddrInfo, num)
	Spawn("p2p relay search", func() {
		defer close(out)
		select {
		case <-h.ready:
		case <-ctx.Done():
			return
		case <-h.ctx.Done():
			return
		}
		seen := map[peer.ID]bool{h.ID(): true}
		left := num
		send := func(ai peer.AddrInfo) bool {
			if left <= 0 {
				return false
			}
			if seen[ai.ID] || len(ai.Addrs) == 0 {
				return true
			}
			seen[ai.ID] = true
			select {
			case out <- ai:
				left--
				return true
			case <-ctx.Done():
				return false
			}
		}
		for _, ai := range h.StaticRelayInfos() {
			if !send(ai) {
				return
			}
		}
		if h.DHT != nil {
			fctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			for ai := range h.DHT.FindProvidersAsync(fctx, relayCID(), num) {
				if !send(ai) {
					break
				}
			}
			cancel()
		}
		for _, p := range h.Network().Peers() {
			addrs := h.Peerstore().Addrs(p)
			if !h.cfg.Local {
				addrs = ma.FilterAddrs(addrs, manet.IsPublicAddr)
			}
			if !send(peer.AddrInfo{ID: p, Addrs: addrs}) {
				return
			}
		}
	})
	return out
}

// announceRelay tells the address book this node is a community relay, now
// and twice a day (records there are dropped after about two days).
func (h *Host) announceRelay() {
	defer h.wg.Done()
	wait := time.Minute // let the node find the network first
	for {
		select {
		case <-h.ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = 12 * time.Hour
		if len(h.Network().Peers()) == 0 {
			wait = 5 * time.Minute
			continue
		}
		ctx, cancel := context.WithTimeout(h.ctx, 2*time.Minute)
		err := h.DHT.Provide(ctx, relayCID(), true)
		cancel()
		if err != nil && h.ctx.Err() == nil {
			h.log.Info("could not announce this server as a community relay; trying again later", "err", err)
			wait = 30 * time.Minute
		}
	}
}

// IsCommunityRelay says a relay this node is connected to is one that passes
// on whole connections, not only the few kilobytes a public node allows.
func (h *Host) IsCommunityRelay(relay peer.ID) bool {
	if relay == "" {
		return false
	}
	for _, ai := range h.StaticRelayInfos() {
		if ai.ID == relay {
			return true
		}
	}
	got, err := h.Peerstore().SupportsProtocols(relay, ProtoRelay)
	return err == nil && len(got) > 0
}

// Close stops the node and waits for its own goroutines.
func (h *Host) Close() error {
	if !h.closed.CompareAndSwap(false, true) {
		return nil
	}
	h.cancel()
	var err error
	if h.DHT != nil {
		err = h.DHT.Close()
	}
	if e := h.Host.Close(); e != nil && err == nil {
		err = e
	}
	h.wg.Wait()
	return err
}
