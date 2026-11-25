// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

// ClientConfig is what an app's end is made from.
type ClientConfig struct {
	Seed     []byte // this device's key
	ServerID string
	// Hints are addresses the server had last time, or that came with an
	// invite. They save a search and work when the address book can't be
	// reached; wrong ones cost a failed try.
	Hints        []string
	Layers       Layers
	StaticRelays []string
	OnState      func(State)
	Log          *slog.Logger

	// For tests: see HostConfig.
	Bootstrap []peer.AddrInfo
	Local     bool
	Listen    []string
	Options   []libp2p.Option
	Clock     Clock
	// UpgradeWait is how long a request waits for a direct connection before
	// giving up on a relay that only passes on a few kilobytes.
	UpgradeWait time.Duration
	// PunchTimeout: see HostConfig. PunchWaits replaces punchWaits.
	PunchTimeout time.Duration
	PunchWaits   []time.Duration
}

// ErrNoDirectPath: the server was found, but only through a relay that passes
// on too little to be of use, and no direct connection could be made.
var ErrNoDirectPath = errors.New("the server was found, but a direct connection through both routers couldn't be made, and no community relay is available to carry it")

// Client keeps a connection to one server and opens HTTP connections over it.
type Client struct {
	cfg    ClientConfig
	log    *slog.Logger
	host   *Host
	server peer.ID
	keeper *keeper
	tr     *http.Transport

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu       sync.Mutex
	gaveUpAt time.Time // when waiting for a direct path last ran out
	punches  int       // hole punches asked for again on this relayed spell
	punchAt  time.Time // when the last one was
	soonAt   time.Time // when punchSoon last asked
}

// NewClient starts an app's node and begins connecting. It returns at once;
// State and OnState say how the connection is doing.
func NewClient(cfg ClientConfig) (*Client, error) {
	id, err := CheckID(cfg.ServerID)
	if err != nil {
		return nil, err
	}
	server, _ := peer.Decode(id)
	lg := cfg.Log
	if lg == nil {
		lg = slog.New(slog.DiscardHandler)
	}
	cfg.Layers.PortMap, cfg.Layers.IPv6 = false, false // an app never asks a router for anything
	if cfg.UpgradeWait <= 0 {
		cfg.UpgradeWait = 15 * time.Second
	}
	h, err := NewHost(HostConfig{Seed: cfg.Seed, Layers: cfg.Layers, StaticRelays: cfg.StaticRelays, Log: lg,
		Bootstrap: cfg.Bootstrap, Local: cfg.Local, Listen: cfg.Listen, Options: cfg.Options, PunchTimeout: cfg.PunchTimeout})
	if err != nil {
		return nil, err
	}
	if h.ID() == server {
		_ = h.Close()
		return nil, errors.New("this is the server's own key")
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{cfg: cfg, log: lg, host: h, server: server, ctx: ctx, cancel: cancel}
	c.keeper = newKeeper(c, cfg.Clock, func(s State) {
		if s.Connected() && c.tr != nil {
			// Whatever idles on the old path is closed, so the next request
			// takes the new one. Requests under way finish where they are.
			c.tr.CloseIdleConnections()
		}
		lg.Info("connection to the server", "path", s.Path, "transport", s.Transport, "community", s.Community, "err", s.Error)
		if cfg.OnState != nil {
			cfg.OnState(s)
		}
	})
	c.tr = &http.Transport{
		DialContext:         c.DialContext,
		MaxIdleConns:        8,
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     60 * time.Second,
		DisableCompression:  true, // passed through as the app asked for it
	}
	h.Peerstore().AddAddrs(server, parseAddrs(cfg.Hints, server), peerstore.PermanentAddrTTL)
	h.ConnManager().Protect(server, "wittymedia")
	h.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(_ network.Network, conn network.Conn) {
			if conn.RemotePeer() == server {
				c.keeper.noteChanged()
			}
		},
		DisconnectedF: func(_ network.Network, conn network.Conn) {
			if conn.RemotePeer() == server {
				c.keeper.noteChanged()
			}
		},
	})
	sub, err := h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	if err != nil {
		cancel()
		_ = h.Close()
		return nil, err
	}
	c.wg.Add(2)
	Spawn("p2p connection", func() {
		defer c.wg.Done()
		c.keeper.run(ctx)
	})
	Spawn("p2p network watch", func() {
		defer c.wg.Done()
		defer sub.Close()
		first := true
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-sub.Out():
				if !ok {
					return
				}
				// Whenever this device learns an address of its own (its outside
				// address, as the network sees it, comes a few seconds after it
				// starts), a hole punch has something new to aim at.
				c.punchSoon()
				if first { // the node finding its own addresses at the start is no network change
					first = false
					continue
				}
				c.keeper.kick()
			}
		}
	})
	return c, nil
}

// ID is this device's connection ID: what the server lists it under.
func (c *Client) ID() string { return c.host.ID().String() }

// State is how the server is reached right now.
func (c *Client) State() State { return c.keeper.get() }

// NetworkChanged tells the client the device moved to another network, for
// systems that say so (phones). It checks the connection now and reconnects
// without waiting.
func (c *Client) NetworkChanged() { c.keeper.kick() }

// KnownAddrs are the server's addresses as last learnt, to keep as hints for
// next time: relay circuits first, then public addresses, then a few private.
func (c *Client) KnownAddrs() []string {
	return addrStrings(keptAddrs(c.host.Peerstore().Addrs(c.server), parseAddrs(c.cfg.Hints, c.server), maxKeptAddrs))
}

// The hints kept for next time: at most maxKeptAddrs, of which up to
// keptCircuits relay circuits and keptPrivate private addresses.
const (
	maxKeptAddrs = 12
	keptCircuits = 4
	keptPrivate  = 2
)

// keptAddrs picks the hints to keep. A server with many addresses (Docker
// bridges, VPNs, several networks) would fill the list with private ones that
// are no use away from home and push out the relay circuit, often the only
// way in from there. So circuits come first: those that came with the hints
// (the owner's WittyRelay, from the invite) ahead of the ones learnt since, as
// the address book may have dropped them. Then public addresses, then at most
// keptPrivate private ones, a home network's ranges ahead of container ones
// and loopback last. At home an app finds the server by its own address too.
func keptAddrs(known, hinted []ma.Multiaddr, limit int) []ma.Multiaddr {
	seen := map[string]bool{}
	var circuits, public, private []ma.Multiaddr
	add := func(a ma.Multiaddr) {
		k := a.String()
		if seen[k] {
			return
		}
		seen[k] = true
		switch {
		case isRelayed(a):
			circuits = append(circuits, a)
		case privateRank(a) >= 0:
			private = append(private, a)
		default:
			public = append(public, a)
		}
	}
	for _, a := range hinted {
		if isRelayed(a) {
			add(a)
		}
	}
	for _, a := range known {
		add(a)
	}
	slices.SortStableFunc(private, func(x, y ma.Multiaddr) int { return privateRank(x) - privateRank(y) })
	out := append(firstN(circuits, keptCircuits), public...)
	out = append(out, firstN(private, keptPrivate)...)
	return firstN(out, limit)
}

func firstN(list []ma.Multiaddr, n int) []ma.Multiaddr {
	if len(list) > n {
		return list[:max(n, 0)]
	}
	return list
}

// privateRank orders the addresses that reach no further than a local network,
// most likely a home network's first; -1 for a public address or a name.
func privateRank(a ma.Multiaddr) int {
	ip, err := manet.ToIP(a)
	if err != nil {
		return -1 // a DNS name: kept with the public ones
	}
	v4 := ip.To4()
	switch {
	case v4 != nil && v4[0] == 192 && v4[1] == 168:
		return 0
	case v4 != nil && v4[0] == 10:
		return 1
	case v4 != nil && v4[0] == 172 && v4[1]&0xf0 == 16:
		return 2 // Docker's own networks are here
	case ip.IsPrivate(), v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 64:
		return 3 // IPv6 unique local, and shared address space (CGNAT, VPNs)
	case ip.IsLinkLocalUnicast():
		return 4
	case ip.IsLoopback():
		return 5 // reaches only a server on the same computer
	case ip.IsUnspecified(), ip.IsMulticast():
		return 6
	}
	return -1
}

// Close ends the connection and the node.
func (c *Client) Close() {
	c.cancel()
	c.tr.CloseIdleConnections()
	_ = c.host.Close()
	c.wg.Wait()
}

// ---------------------------------------------------------------- the link

func (c *Client) allow(ctx context.Context) context.Context {
	return network.WithAllowLimitedConn(ctx, "wittymedia")
}

// current reads the path off the connections that are open.
func (c *Client) current() State {
	var relayed network.Conn
	for _, conn := range c.host.Network().ConnsToPeer(c.server) {
		if conn.IsClosed() {
			continue
		}
		ra := conn.RemoteMultiaddr()
		if conn.Stat().Limited || isRelayed(ra) {
			if relayed == nil {
				relayed = conn
			}
			continue
		}
		s := State{Path: PathDirect, Addr: ra.String(), Transport: transportOf(ra)}
		if ip := ipOf(ra); ip.IsValid() && !publicIP(ip) {
			s.Path = PathLocal
		}
		return s
	}
	if relayed != nil {
		r := relayOf(relayed.RemoteMultiaddr())
		s := State{Path: PathRelayed, Relay: r.String(), Community: c.host.IsCommunityRelay(r)}
		if s.Community {
			if o, ok := c.host.RelayOfferOf(r); ok {
				s.RelayKbps, s.RelayMaxHeight, s.RelayAllows = o.Kbps, o.MaxHeight, o.Words()
			} else {
				c.host.learnOffer(r, c.keeper.noteChanged)
			}
		}
		return s
	}
	return State{}
}

// connect tries what is known first, then asks the address book, then the
// relays the owner named.
func (c *Client) connect(ctx context.Context) error {
	// libp2p keeps its own list of addresses that failed lately and won't dial
	// them again for up to five minutes. The keeper has its own, shorter
	// back-off, so a server that comes back (a restart, a router reboot) would
	// otherwise wait out libp2p's as well.
	c.clearDialBackoff()
	var last error
	try := func(addrs []ma.Multiaddr, d time.Duration) bool {
		if len(addrs) == 0 && len(c.host.Peerstore().Addrs(c.server)) == 0 {
			return false
		}
		cctx, cancel := context.WithTimeout(c.allow(ctx), d)
		defer cancel()
		err := c.host.Connect(cctx, peer.AddrInfo{ID: c.server, Addrs: addrs})
		if err != nil {
			last = err
			return false
		}
		return true
	}
	if try(nil, 20*time.Second) {
		return nil
	}
	if c.host.DHT != nil && ctx.Err() == nil {
		fctx, cancel := context.WithTimeout(ctx, 45*time.Second)
		ai, err := c.host.DHT.FindPeer(fctx, c.server)
		cancel()
		if err == nil && try(ai.Addrs, 30*time.Second) {
			return nil
		}
		if err != nil && last == nil {
			last = fmt.Errorf("the server wasn't found in the public address book: %w", err)
		}
	}
	if c.cfg.Layers.Relay && ctx.Err() == nil {
		var via []ma.Multiaddr
		for _, r := range c.host.StaticRelayInfos() {
			for _, a := range r.Addrs {
				if circuit, err := ma.NewMultiaddr(a.String() + "/p2p/" + r.ID.String() + "/p2p-circuit"); err == nil {
					via = append(via, circuit)
				}
			}
		}
		if len(via) > 0 && try(via, 30*time.Second) {
			return nil
		}
	}
	if last == nil {
		return errors.New("no address is known for the server yet")
	}
	return plainDialError(last)
}

// clearDialBackoff forgets libp2p's record of failed dials to the server.
func (c *Client) clearDialBackoff() {
	if sw, ok := c.host.Network().(*swarm.Swarm); ok {
		sw.Backoff().Clear(c.server)
	}
}

// plainDialError shortens libp2p's account of every address it tried to what
// a person can use.
func plainDialError(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("the server didn't answer in time")
	case errors.Is(err, context.Canceled):
		return err
	}
	return errors.New("the server couldn't be reached at any of its addresses")
}

// tidy closes relayed connections that a direct one has replaced, once
// nothing is using them: a relay's time is someone else's to give.
func (c *Client) tidy() {
	conns := c.host.Network().ConnsToPeer(c.server)
	direct := false
	for _, conn := range conns {
		if !conn.Stat().Limited && !isRelayed(conn.RemoteMultiaddr()) {
			direct = true
		}
	}
	if !direct {
		c.punchAgain(conns)
		return
	}
	c.mu.Lock()
	c.punches, c.punchAt = 0, time.Time{}
	c.mu.Unlock()
	for _, conn := range conns {
		if (conn.Stat().Limited || isRelayed(conn.RemoteMultiaddr())) && len(conn.GetStreams()) == 0 {
			_ = conn.Close()
		}
	}
}

// punchWaits is how long to leave a relayed connection before asking for
// another try at a direct one. The first wait is longer than libp2p's own
// attempt can take (a direct dial, then three punches, five seconds each at
// most): asking again sooner would cut that attempt off.
var punchWaits = []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute}

// punchAgain asks for another hole punch. libp2p tries once, when the server
// sees the relayed connection arrive, and that one try is easily lost: this
// app's own end may not have been ready, or a packet went missing. The way to
// ask again is to arrive again, so an idle relayed connection is closed and
// the keeper makes a new one.
func (c *Client) punchAgain(conns []network.Conn) {
	if !c.cfg.Layers.HolePunch || len(conns) == 0 {
		return
	}
	waits := punchWaits
	if len(c.cfg.PunchWaits) > 0 {
		waits = c.cfg.PunchWaits
	}
	c.mu.Lock()
	wait := waits[min(c.punches, len(waits)-1)]
	if c.punchAt.IsZero() {
		c.punchAt = time.Now() // the spell starts now
	}
	due := time.Since(c.punchAt) >= wait
	c.mu.Unlock()
	if !due {
		return
	}
	// HTTP connections kept open for the next request are streams too; they
	// are closed first, so that only a request under way keeps the relay.
	if c.tr != nil {
		c.tr.CloseIdleConnections()
	}
	inUse := func() bool {
		for _, conn := range conns {
			for _, s := range conn.GetStreams() {
				if s.Protocol() == ProtoHTTP || s.Protocol() == ProtoEnrol {
					return true
				}
			}
		}
		return false
	}
	for i := 0; inUse(); i++ {
		if i == 10 {
			return // in use (a community relay carrying a film): leave it be
		}
		time.Sleep(50 * time.Millisecond) // a closed stream takes a moment to leave the list
	}
	c.mu.Lock()
	c.punches++
	c.punchAt = time.Now()
	c.mu.Unlock()
	c.log.Info("still on a relay; asking for a direct connection again", "try", c.punches)
	for _, conn := range conns {
		_ = conn.Close()
	}
}

// punchSoon makes the next look at a relayed connection ask for a direct one
// at once. The server tries a hole punch the moment an app arrives through a
// relay, which is often before the app has learnt its own outside address
// from the network: that try has nothing to aim at. When this device's
// addresses change, a new try has something to aim at, so it is made now
// rather than at the next wait in punchWaits.
func (c *Client) punchSoon() {
	conns := c.host.Network().ConnsToPeer(c.server)
	for _, conn := range conns {
		if !conn.Stat().Limited && !isRelayed(conn.RemoteMultiaddr()) {
			return // already direct
		}
	}
	if len(conns) == 0 || !c.cfg.Layers.HolePunch {
		return
	}
	c.mu.Lock()
	if time.Since(c.soonAt) < 5*time.Second {
		c.mu.Unlock()
		return // addresses often change several times in a row
	}
	c.soonAt = time.Now()
	c.punchAt = time.Now().Add(-time.Hour)
	c.mu.Unlock()
	c.punchAgain(conns)
}

// check pings the server over what is open, and closes it all if nothing
// comes back: a connection left on a network the device has gone from can
// take half a minute to notice by itself.
func (c *Client) check(ctx context.Context) {
	if len(c.host.Network().ConnsToPeer(c.server)) == 0 {
		return
	}
	pctx, cancel := context.WithTimeout(c.allow(ctx), 4*time.Second)
	defer cancel()
	select {
	case res := <-ping.Ping(pctx, c.host, c.server):
		if res.Error == nil {
			c.punchSoon()
			return
		}
	case <-pctx.Done():
	}
	if ctx.Err() == nil {
		c.log.Info("the connection stopped answering after a network change; reconnecting")
		_ = c.host.Network().ClosePeer(c.server)
		c.clearDialBackoff() // what failed on the old network may work on the new one
	}
}

// ---------------------------------------------------------------- HTTP

// usable says whether requests may be sent over a path. A public relay passes
// on about a hundred kilobytes and then cuts off: enough to arrange a direct
// connection, useless for a page, let alone a film.
func usable(s State) bool {
	return s.Path == PathLocal || s.Path == PathDirect || (s.Path == PathRelayed && s.Community)
}

// ready waits until requests can be sent, within ctx.
func (c *Client) ready(ctx context.Context) error {
	wctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	s, err := c.keeper.wait(wctx, State.Connected)
	cancel()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if s.Error != "" {
			return errors.New(s.Error)
		}
		return errors.New("still looking for the server")
	}
	if usable(s) {
		return nil
	}
	// On a public relay: a direct connection is being arranged right now.
	// Wait for it, but not on every request once it has failed.
	c.mu.Lock()
	recently := time.Since(c.gaveUpAt) < 30*time.Second
	c.mu.Unlock()
	if recently {
		return ErrNoDirectPath
	}
	wctx, cancel = context.WithTimeout(ctx, c.cfg.UpgradeWait)
	_, err = c.keeper.wait(wctx, usable)
	cancel()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		c.mu.Lock()
		c.gaveUpAt = time.Now()
		c.mu.Unlock()
		return ErrNoDirectPath
	}
	return nil
}

// DialContext opens one HTTP connection to the server. The address asked for
// is ignored: there is only one place this leads.
func (c *Client) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	var s network.Stream
	var err error
	// A connection can be closing just as a stream is asked for on it (the
	// network changed, or the server closed it): nothing has been sent yet,
	// so another try on a fresh connection is safe for any request.
	for try := 0; try < 3; try++ {
		if err = c.ready(ctx); err != nil {
			return nil, err
		}
		s, err = c.host.NewStream(c.allow(ctx), c.server, ProtoHTTP)
		if err == nil || ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Duration(try+1) * 100 * time.Millisecond):
		}
	}
	if err != nil {
		return nil, fmt.Errorf("the server refused the connection (is this device still on its list?): %w", err)
	}
	a := &Addr{Peer: c.server.String()}
	// A request that fails part-way (a timeout, a reset) is the first sign of a
	// connection that died without saying so: the network changed under it and
	// the system didn't tell (a router given a new outside address). The keeper
	// is asked to check it at once rather than wait for the transport to notice.
	return &streamConn{Stream: s, local: a, remote: a, onError: c.keeper.kick}, nil
}

// Transport sends HTTP requests to the server, whatever host they name.
func (c *Client) Transport() *http.Transport { return c.tr }

// WaitConnected waits until requests can be sent, and says whether they can.
func (c *Client) WaitConnected(ctx context.Context) bool {
	_, err := c.keeper.wait(ctx, State.Connected)
	return err == nil
}

// Enrol shows the server an invite, which puts this device on its list. It
// works over any relay: it is two small messages.
func (c *Client) Enrol(ctx context.Context, secret, name, platform string) (EnrolResponse, error) {
	wctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if s, err := c.keeper.wait(wctx, State.Connected); err != nil {
		if s.Error != "" {
			return EnrolResponse{}, errors.New(s.Error)
		}
		return EnrolResponse{}, errors.New("the server wasn't found")
	}
	s, err := c.host.NewStream(c.allow(wctx), c.server, ProtoEnrol)
	if err != nil {
		return EnrolResponse{}, errors.New("the server takes no invites")
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(20 * time.Second))
	if err := writeMessage(s, EnrolRequest{V: 1, Secret: secret, Name: name, Platform: platform}); err != nil {
		return EnrolResponse{}, err
	}
	var res EnrolResponse
	if err := readMessage(s, &res); err != nil {
		return EnrolResponse{}, errors.New("the server didn't answer the invite")
	}
	if res.OK {
		c.host.Peerstore().AddAddrs(c.server, parseAddrs(res.Addrs, c.server), peerstore.PermanentAddrTTL)
	}
	return res, nil
}

// Host is the node underneath, for tools that simulate networks.
func (c *Client) Host() *Host { return c.host }
