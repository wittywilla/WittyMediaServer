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
	"sort"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"
	ma "github.com/multiformats/go-multiaddr"
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
		Bootstrap: cfg.Bootstrap, Local: cfg.Local, Listen: cfg.Listen, Options: cfg.Options})
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
				if first { // the node finding its own addresses at the start is no change
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
// next time: the direct ones first.
func (c *Client) KnownAddrs() []string {
	addrs := c.host.Peerstore().Addrs(c.server)
	sort.SliceStable(addrs, func(i, j int) bool { return !isRelayed(addrs[i]) && isRelayed(addrs[j]) })
	if len(addrs) > 12 {
		addrs = addrs[:12]
	}
	return addrStrings(addrs)
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
		return State{Path: PathRelayed, Relay: r.String(), Community: c.host.IsCommunityRelay(r)}
	}
	return State{}
}

// connect tries what is known first, then asks the address book, then the
// relays the owner named.
func (c *Client) connect(ctx context.Context) error {
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
		return
	}
	for _, conn := range conns {
		if (conn.Stat().Limited || isRelayed(conn.RemoteMultiaddr())) && len(conn.GetStreams()) == 0 {
			_ = conn.Close()
		}
	}
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
			return
		}
	case <-pctx.Done():
	}
	if ctx.Err() == nil {
		c.log.Info("the connection stopped answering after a network change; reconnecting")
		_ = c.host.Network().ClosePeer(c.server)
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
	_, err = c.keeper.wait(wctx, func(s State) bool { return usable(s) || !s.Connected() })
	cancel()
	if err != nil {
		c.mu.Lock()
		c.gaveUpAt = time.Now()
		c.mu.Unlock()
		return ErrNoDirectPath
	}
	if !usable(c.keeper.get()) {
		return errors.New("the connection was lost")
	}
	return nil
}

// DialContext opens one HTTP connection to the server. The address asked for
// is ignored: there is only one place this leads.
func (c *Client) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	if err := c.ready(ctx); err != nil {
		return nil, err
	}
	s, err := c.host.NewStream(c.allow(ctx), c.server, ProtoHTTP)
	if err != nil {
		return nil, fmt.Errorf("the server refused the connection (is this device still on its list?): %w", err)
	}
	a := &Addr{Peer: c.server.String()}
	return &streamConn{Stream: s, local: a, remote: a}, nil
}

// Transport sends HTTP requests to the server, whatever host they name.
func (c *Client) Transport() *http.Transport { return c.tr }

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
