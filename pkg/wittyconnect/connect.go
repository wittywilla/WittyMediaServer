// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package wittyconnect is the app's side of peer-to-peer connections, ready
// to use: it connects to a server by its connection ID and offers what an app
// already knows how to talk to, an HTTP address on this device. Whatever is
// sent to that address comes out at the server, encrypted to its key on the
// way, over whichever path the two could find.
//
// The address is http://127.0.0.1:<port>, on a port picked at random, and it
// answers only requests that carry the session's secret (in a header or a
// cookie): other programs on the device can connect to the port, but not use
// it. The desktop app runs this as a helper program (cmd/wittyconnect); the
// phone and TV apps link it in (the mobile package beside this one).
//
// docs/P2P.md describes the whole design.
package wittyconnect

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/p2p"
)

// The names an app passes the secret under. The header suits an app's own
// HTTP client; the cookie suits a web view and media players, which send
// cookies by themselves with every request, sockets included.
const (
	SecretHeader = "X-Witty-Connect"
	SecretCookie = "wittyconnect"
)

// State is how the server is reached right now: see p2p.State.
type State = p2p.State

// Config is what a connection is made from.
type Config struct {
	// Key is this device's key, as NewKey made it. The app keeps it secret and
	// keeps it: the server knows the device by it.
	Key string `json:"key"`
	// ServerID is the server's connection ID.
	ServerID string `json:"serverId"`
	// Hints are addresses the server had last time, or that an invite named.
	Hints []string `json:"hints"`
	// NoDHT, NoRelay and NoHolePunch turn layers off, for a person who wants
	// fewer outside parties involved: see docs/P2P.md for what each costs.
	NoDHT       bool `json:"noDht"`
	NoRelay     bool `json:"noRelay"`
	NoHolePunch bool `json:"noHolePunch"`
	// StaticRelays are relays to use as well, as multiaddrs ending in /p2p/<id>.
	StaticRelays []string `json:"staticRelays"`
	// Port is the port to open the local address on, if it is free: the one
	// used last time, so that what a web view stored for the address is still
	// there. Otherwise, and when 0, one is picked.
	Port int `json:"port"`

	OnState func(State)  `json:"-"`
	Log     *slog.Logger `json:"-"`
	// Tune changes the node's configuration before it starts, for tests.
	Tune func(*p2p.ClientConfig) `json:"-"`
}

// NewKey makes a device's key. ID says what the server will call it.
func NewKey() (key string, err error) {
	seed, err := p2p.NewSeed()
	if err != nil {
		return "", err
	}
	return p2p.EncodeSeed(seed), nil
}

// DeviceID is the public name of a key.
func DeviceID(key string) (string, error) {
	seed, err := p2p.DecodeSeed(key)
	if err != nil {
		return "", err
	}
	return p2p.IDFromSeed(seed)
}

// ParseInvite reads an invite link.
func ParseInvite(link string) (p2p.Invite, error) { return p2p.ParseInvite(link) }

// NewJoinCode makes the code a device with no keyboard shows, for someone
// signed in to type into the server: see Join.
func NewJoinCode() (string, error) { return p2p.NewJoinCode() }

// JoinConfig is what a device that shows a code looks for its server with.
type JoinConfig struct {
	// Key is this device's key, and Code what NewJoinCode made.
	Key  string `json:"key"`
	Code string `json:"code"`
	// Name and Platform are what the server's list will call the device.
	Name     string `json:"name"`
	Platform string `json:"platform"`
	// NoRelay and NoHolePunch turn layers off. The address book can't be: it
	// is how a device that knows only a code finds the server.
	NoRelay      bool     `json:"noRelay"`
	NoHolePunch  bool     `json:"noHolePunch"`
	StaticRelays []string `json:"staticRelays"`

	// Tune changes the search before it starts, for tests.
	Tune func(*p2p.JoinConfig) `json:"-"`
}

// JoinResult is the server that took the code, and its answer: on OK, the
// server's connection ID and addresses to start a connection with, and a
// sign-in code to redeem over it.
type JoinResult = p2p.JoinResult

// Join is for a device away from home that has never met its server and
// can't take an invite link in: a TV. It shows a code; this waits until
// someone signed in has typed that code into the server (POST
// /api/v1/p2p/join), finds the server through the address book, and has this
// device's key put on its list. Give ctx the fifteen minutes a code lasts.
// docs/P2P.md, "Joining by a code", is the design.
func Join(ctx context.Context, cfg JoinConfig) (JoinResult, error) {
	seed, err := p2p.DecodeSeed(cfg.Key)
	if err != nil {
		return JoinResult{}, errors.New("this device has no connection key")
	}
	jc := p2p.JoinConfig{Seed: seed, Code: cfg.Code, Name: cfg.Name, Platform: cfg.Platform, StaticRelays: cfg.StaticRelays,
		Layers: p2p.Layers{DHT: true, Relay: !cfg.NoRelay, HolePunch: !cfg.NoHolePunch && !cfg.NoRelay}}
	if cfg.Tune != nil {
		cfg.Tune(&jc)
	}
	return p2p.Join(ctx, jc)
}

// Connector is a running connection and its local address.
type Connector struct {
	client *p2p.Client
	ln     net.Listener
	srv    *http.Server
	secret string
	host   string // 127.0.0.1:<port>
	log    *slog.Logger
	once   sync.Once
	done   chan struct{}
}

// Start connects and opens the local address. It returns as soon as the
// address is listening; requests wait while the server is being found.
func Start(cfg Config) (*Connector, error) {
	seed, err := p2p.DecodeSeed(cfg.Key)
	if err != nil {
		return nil, errors.New("this device has no connection key")
	}
	lg := cfg.Log
	if lg == nil {
		lg = slog.New(slog.DiscardHandler)
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	cc := p2p.ClientConfig{
		Seed: seed, ServerID: cfg.ServerID, Hints: cfg.Hints, StaticRelays: cfg.StaticRelays, OnState: cfg.OnState, Log: lg,
		Layers: p2p.Layers{DHT: !cfg.NoDHT, Relay: !cfg.NoRelay, HolePunch: !cfg.NoHolePunch && !cfg.NoRelay},
	}
	if cfg.Tune != nil {
		cfg.Tune(&cc)
	}
	// Only this device: never a port other machines on the network can reach.
	var ln net.Listener
	if cfg.Port >= 1024 && cfg.Port <= 65535 {
		ln, _ = net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", cfg.Port))
	}
	if ln == nil {
		if ln, err = net.Listen("tcp4", "127.0.0.1:0"); err != nil {
			return nil, err
		}
	}
	client, err := p2p.NewClient(cc)
	if err != nil {
		_ = ln.Close()
		return nil, err
	}
	c := &Connector{client: client, ln: ln, secret: base64.RawURLEncoding.EncodeToString(raw), host: ln.Addr().String(), log: lg, done: make(chan struct{})}
	target, _ := url.Parse("http://server")
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			// Forwarded headers are dropped before this is called, and none is
			// added: the server reads none on these connections anyway.
			r.SetURL(target)
			r.Out.Host = r.In.Host // the page's own address, which the server compares with Origin
			stripSecret(r.Out)
		},
		Transport:     client.Transport(),
		FlushInterval: -1, // events and streams go out as they come
		ErrorHandler:  c.proxyError,
	}
	c.srv = &http.Server{
		Handler:           c.guard(proxy),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	p2p.Spawn("wittyconnect local address", func() {
		defer close(c.done)
		_ = c.srv.Serve(ln)
	})
	return c, nil
}

// stripSecret takes the session's secret out of a request before it leaves:
// it is between the app and this connector, and none of the server's business.
func stripSecret(r *http.Request) {
	r.Header.Del(SecretHeader)
	cookies := r.Cookies()
	if len(cookies) == 0 {
		return
	}
	r.Header.Del("Cookie")
	for _, ck := range cookies {
		if ck.Name != SecretCookie {
			r.AddCookie(&http.Cookie{Name: ck.Name, Value: ck.Value})
		}
	}
}

// guard lets a request through only if it is meant for this address and
// carries the secret.
func (c *Connector) guard(next http.Handler) http.Handler {
	_, port, _ := net.SplitHostPort(c.host)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A web page elsewhere can make a browser send requests to 127.0.0.1
		// under a name of its own. It can't make the Host say 127.0.0.1, or a
		// name under .localhost, which never leaves the device (an app uses one
		// per server, so that each server's cookies stay its own).
		if !localHost(r.Host, port) {
			http.Error(w, "This address is for the WittyMediaServer app on this device only.", http.StatusForbidden)
			return
		}
		got := r.Header.Get(SecretHeader)
		if got == "" {
			if ck, err := r.Cookie(SecretCookie); err == nil {
				got = ck.Value
			}
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(c.secret)) != 1 {
			http.Error(w, "This address is for the WittyMediaServer app on this device only.", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// localHost says a Host header names this device, on this port.
func localHost(hostport, port string) bool {
	host, p, err := net.SplitHostPort(hostport)
	if err != nil || p != port {
		return false
	}
	if host == "127.0.0.1" || host == "localhost" {
		return true
	}
	name, ok := strings.CutSuffix(host, ".localhost")
	if !ok || name == "" || len(name) > 63 {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z') && !(c >= '0' && c <= '9') && c != '-' {
			return false
		}
	}
	return true
}

// proxyError tells the app, in words fit to show, why a request didn't get
// through. A page gets a page; anything else gets the server's usual JSON.
func (c *Connector) proxyError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, context.Canceled) {
		return // the app gave up; nobody is listening
	}
	msg := err.Error()
	// Go's own wrapping ("dial tcp: ...") is no use to a person.
	if i := strings.LastIndex(msg, ": "); i >= 0 && !errors.Is(err, p2p.ErrNoDirectPath) && strings.Contains(msg[:i], "dial") {
		msg = msg[i+2:]
	}
	if errors.Is(err, p2p.ErrNoDirectPath) {
		msg = "Your server was found, but a direct connection through both routers couldn't be made, and no community relay is available to carry it. It often works from another network."
	}
	msg = strings.ToUpper(msg[:1]) + msg[1:]
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Witty-Connect-Error", "1")
	if r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Not connected</title>`+
			`<style>body{font:16px system-ui,sans-serif;margin:15vh auto;max-width:34em;padding:0 1.5em;color:#ddd;background:#111}h1{font-size:1.3em}</style></head>`+
			`<body><h1>Can't reach your server right now</h1><p>%s</p><p>The app keeps trying. Reload this page in a moment.</p></body></html>`, html.EscapeString(msg))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	fmt.Fprintf(w, `{"error":%q}`, msg)
}

// URL is the local address: http://127.0.0.1:<port>.
func (c *Connector) URL() string { return "http://" + c.host }

// Port is its port.
func (c *Connector) Port() int { return c.ln.Addr().(*net.TCPAddr).Port }

// Secret is what requests to the address must carry, under SecretHeader or
// in a cookie named SecretCookie. It is new each time.
func (c *Connector) Secret() string { return c.secret }

// State is how the server is reached right now.
func (c *Connector) State() State { return c.client.State() }

// DeviceID is this device's ID, to enrol with the server.
func (c *Connector) DeviceID() string { return c.client.ID() }

// KnownAddrs are the server's addresses as last learnt, to keep as hints.
func (c *Connector) KnownAddrs() []string { return c.client.KnownAddrs() }

// NetworkChanged tells the connector the device moved to another network.
func (c *Connector) NetworkChanged() { c.client.NetworkChanged() }

// Enrol shows the server an invite's secret, which puts this device on its
// list. The answer may hold a sign-in code to redeem over the local address.
func (c *Connector) Enrol(ctx context.Context, secret, name, platform string) (p2p.EnrolResponse, error) {
	return c.client.Enrol(ctx, secret, name, platform)
}

// Close ends the connection and closes the local address.
func (c *Connector) Close() {
	c.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = c.srv.Shutdown(ctx)
		cancel()
		_ = c.srv.Close()
		c.client.Close()
		<-c.done
	})
}
