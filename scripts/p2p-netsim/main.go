// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Command p2p-netsim plays each part in scripts/p2p-netsim/netsim.sh, which
// builds small networks out of Linux network namespaces (home routers that
// translate addresses, strict ones, a provider's shared address, a network
// that lets no UDP out, loss and delay) and runs a server's node and an app's
// node behind them. Nothing reaches the internet: every address lives inside
// the namespaces, and the address book (DHT) is off.
//
//	p2p-netsim observers -ips 80.0.0.21,80.0.0.22,... -out observers.txt
//	p2p-netsim server -relay <addr> -key <key> -device <id> -observers observers.txt -out server.json
//	p2p-netsim client -server server.json -observers observers.txt -mb 8 -out client.json
//	p2p-netsim client ... -loop 90s   (requests every half second; reports outages)
//	p2p-netsim id -seed 2             (a client's ID, to list on the server)
//
// Observers are ordinary nodes on the "internet" that the server and app
// connect to, as they would to the public network's nodes: libp2p learns its
// own public address and kind of NAT from four or more of them.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/wittywilla/wittymediaserver/internal/p2p"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: p2p-netsim observers|server|client|id ...")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "observers":
		err = observers(os.Args[2:])
	case "server":
		err = server(os.Args[2:])
	case "client":
		err = client(os.Args[2:])
	case "id":
		fs := flag.NewFlagSet("id", flag.ExitOnError)
		seed := fs.Int("seed", 2, "")
		_ = fs.Parse(os.Args[2:])
		id, _ := p2p.IDFromSeed(seedOf(*seed))
		fmt.Println(id)
	default:
		err = fmt.Errorf("unknown part %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "p2p-netsim:", err)
		os.Exit(1)
	}
}

func seedOf(b int) []byte { return bytes.Repeat([]byte{byte(b)}, p2p.SeedSize) }

func logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func waitForSignal() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}

func writeJSON(path string, v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	tmp := path + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, path)
	}
}

// ---------------------------------------------------------------- observers

func observers(args []string) error {
	fs := flag.NewFlagSet("observers", flag.ExitOnError)
	ips := fs.String("ips", "", "comma-separated IPs, one node each")
	out := fs.String("out", "", "file to write their addresses to")
	_ = fs.Parse(args)
	var lines []string
	for i, ip := range strings.Split(*ips, ",") {
		h, err := libp2p.New(libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/%s/udp/4001/quic-v1", ip), fmt.Sprintf("/ip4/%s/tcp/4001", ip)),
			libp2p.DisableMetrics(), libp2p.ForceReachabilityPublic(), libp2p.DisableRelay())
		if err != nil {
			return fmt.Errorf("observer %d: %w", i, err)
		}
		for _, a := range h.Addrs() {
			lines = append(lines, a.String()+"/p2p/"+h.ID().String())
		}
	}
	if err := os.WriteFile(*out, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	waitForSignal()
	return nil
}

func readLines(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if l := strings.TrimSpace(sc.Text()); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// meet connects to every observer, again now and then: this is how the node
// learns its outside address, as it would from the public network.
func meet(ctx context.Context, h interface {
	Connect(context.Context, peer.AddrInfo) error
}, path string) {
	for {
		for _, l := range readLines(path) {
			a, err := ma.NewMultiaddr(l)
			if err != nil {
				continue
			}
			ai, err := peer.AddrInfoFromP2pAddr(a)
			if err != nil {
				continue
			}
			cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			_ = h.Connect(cctx, *ai)
			cancel()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(20 * time.Second):
		}
	}
}

// ---------------------------------------------------------------- server

type devices struct {
	mu  sync.Mutex
	ids map[string]bool
}

func (d *devices) Allowed(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.ids[id]
}
func (d *devices) Seen(string, string) {}

func (d *devices) list() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := []string{}
	for id := range d.ids {
		out = append(out, id)
	}
	return out
}

var blob = bytes.Repeat([]byte("WittyMediaServer"), 1<<16) // 1 MiB

func handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, "hello") })
	// /big?mb=N sends N MiB.
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		n := 1
		fmt.Sscan(r.URL.Query().Get("mb"), &n)
		w.Header().Set("Content-Length", fmt.Sprint(n*len(blob)))
		for range n {
			if _, err := w.Write(blob); err != nil {
				return
			}
		}
	})
	return mux
}

func server(args []string) error {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	relay := fs.String("relay", "", "a WittyRelay's address")
	key := fs.String("key", "", "its access key")
	device := fs.String("device", "", "comma-separated device IDs that may connect")
	obs := fs.String("observers", "", "file of observers' addresses")
	out := fs.String("out", "server.json", "where to write the status")
	port := fs.Int("port", 4002, "")
	revokeAfter := fs.Duration("revoke-after", 0, "take every device off the list after this long, as Settings does")
	invite := fs.String("invite", "", "an open invite's secret: the first device to show it is put on the list, once")
	_ = fs.Parse(args)
	devs := &devices{ids: map[string]bool{}}
	for _, d := range strings.Split(*device, ",") {
		if d = strings.TrimSpace(d); d != "" {
			devs.ids[d] = true
		}
	}
	cfg := p2p.NodeConfig{
		Seed: seedOf(1), Port: *port, Layers: p2p.Layers{Relay: true, HolePunch: true}, DevicesOnly: true,
		Devices: devs, DeviceIDs: devs.list, Name: "netsim server",
		Handler: handler(), Log: logger(),
		Listen: []string{fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", *port), fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", *port)},
	}
	if *relay != "" {
		cfg.OwnRelays = []p2p.OwnRelay{{Addr: *relay, Key: *key}}
	}
	// An invite, as the server keeps one (#614): while it is open its enrol key is on the list
	// the relay is given, and the first device to show it (from that key, signing for its own)
	// goes on the list in its place.
	var node atomic.Pointer[p2p.Node]
	if *invite != "" {
		var open atomic.Bool
		open.Store(true)
		until := time.Now().Add(15 * time.Minute)
		enrolID, err := p2p.EnrolID(*invite)
		if err != nil {
			return err
		}
		cfg.DeviceIDs = func() []string {
			ids := devs.list()
			if open.Load() && time.Now().Before(until) {
				ids = append(ids, enrolID)
			}
			return ids
		}
		cfg.Enrol = func(_ context.Context, id string, req p2p.EnrolRequest, from netip.Addr) p2p.EnrolResponse {
			n := node.Load()
			if n == nil {
				return p2p.EnrolResponse{Reason: "off", Error: "not yet"}
			}
			listID, err := p2p.CheckEnrolFor(req, id, n.ID())
			if err != nil || req.Secret != *invite {
				slog.Info("netsim: an invite was refused", "why", "wrong", "device", p2p.ShortID(id), "from", from.String())
				return p2p.EnrolResponse{Reason: "wrong", Error: "That invite isn't right."}
			}
			if !open.CompareAndSwap(true, false) {
				slog.Info("netsim: an invite was refused", "why", "used", "device", p2p.ShortID(id), "from", from.String())
				return p2p.EnrolResponse{Reason: "used", Error: "That invite has already been used."}
			}
			devs.mu.Lock()
			devs.ids[listID] = true
			devs.mu.Unlock()
			n.DevicesChanged()
			slog.Info("netsim: a device joined by invite", "device", p2p.ShortID(listID), "through", p2p.ShortID(id), "from", from.String())
			return p2p.EnrolResponse{OK: true, ServerName: "netsim server"}
		}
	}
	n, err := p2p.StartNode(cfg)
	if err != nil {
		return err
	}
	defer n.Close()
	node.Store(n)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if *obs != "" {
		go meet(ctx, n.Host(), *obs)
	}
	var revokedAt atomic.Int64
	if *revokeAfter > 0 {
		go func() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(*revokeAfter):
			}
			// What taking a device off the list does on a real server: the
			// list, the WittyRelay told at once, and every connection closed.
			gone := devs.list()
			devs.mu.Lock()
			devs.ids = map[string]bool{}
			devs.mu.Unlock()
			revokedAt.Store(time.Now().UnixMilli())
			n.DevicesChanged()
			for _, id := range gone {
				n.Kick(id)
			}
			slog.Info("netsim: every device was taken off the list", "devices", len(gone))
		}()
	}
	go func() {
		for {
			st := n.Status()
			writeJSON(*out, map[string]any{"id": n.ID(), "addrs": n.Addrs(), "status": st, "revokedAt": revokedAt.Load()})
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}()
	waitForSignal()
	return nil
}

// ---------------------------------------------------------------- client

type result struct {
	OK           bool    `json:"ok"`
	Error        string  `json:"error,omitempty"`
	ConnectSec   float64 `json:"connectSec"`
	FirstPath    string  `json:"firstPath"`
	Path         string  `json:"path"`
	Transport    string  `json:"transport,omitempty"`
	Community    bool    `json:"community"`
	RelayAllows  string  `json:"relayAllows,omitempty"`
	DirectAfter  float64 `json:"directAfterSec,omitempty"` // when a relayed connection became direct
	MB           int     `json:"mb"`
	Mbps         float64 `json:"mbps"`
	Requests     int     `json:"requests,omitempty"`
	Failed       int     `json:"failed,omitempty"`
	LongestGapMs int64   `json:"longestOutageMs,omitempty"`
	// For a device that must not get in (a stranger) or must stop getting in
	// (revoked): what it got anyway.
	Answered        int `json:"answered,omitempty"`
	AnsweredRevoked int `json:"answeredAfterRevoke,omitempty"`
	// With -invite: whether showing it put the device on the list.
	Enrolled   bool     `json:"enrolled,omitempty"`
	EnrolError string   `json:"enrolError,omitempty"`
	States     []string `json:"states"`
	OwnAddrs   []string `json:"ownAddrs"`
}

func client(args []string) error {
	fs := flag.NewFlagSet("client", flag.ExitOnError)
	srv := fs.String("server", "server.json", "the server's status file")
	obs := fs.String("observers", "", "file of observers' addresses")
	out := fs.String("out", "client.json", "")
	mb := fs.Int("mb", 8, "MiB to fetch")
	wait := fs.Duration("wait", 90*time.Second, "how long to wait for a connection")
	upgrade := fs.Duration("upgrade", 90*time.Second, "how long to wait for a relayed connection to become direct")
	loop := fs.Duration("loop", 0, "keep sending requests this long, reporting outages")
	seed := fs.Int("seed", 2, "")
	refused := fs.Bool("expect-refused", false, "this device isn't on the list: ok means nothing ever answers it")
	revoked := fs.Bool("expect-revoked", false, "the server takes this device off its list while -loop runs: ok means nothing answers after that")
	invite := fs.String("invite", "", "show the server this invite's secret first, as an app opening an invite link does")
	_ = fs.Parse(args)

	var info struct {
		ID    string   `json:"id"`
		Addrs []string `json:"addrs"`
	}
	b, err := os.ReadFile(*srv)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &info); err != nil {
		return err
	}
	res := result{MB: *mb, States: []string{}}
	start := time.Now()
	stateCh := make(chan p2p.State, 256)
	cc := p2p.ClientConfig{Seed: seedOf(*seed), ServerID: info.ID, Hints: info.Addrs,
		Layers: p2p.Layers{Relay: true, HolePunch: true}, Log: logger(),
		Listen: []string{"/ip4/0.0.0.0/udp/0/quic-v1", "/ip4/0.0.0.0/tcp/0"}}
	c, err := p2p.NewClient(p2p.ClientConfig{Seed: cc.Seed, ServerID: cc.ServerID, Hints: cc.Hints, Layers: cc.Layers, Log: cc.Log, Listen: cc.Listen,
		OnState: func(s p2p.State) {
			select {
			case stateCh <- s:
			default:
			}
		}})
	if err != nil {
		return err
	}
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if *obs != "" {
		go meet(ctx, c.Host(), *obs)
	}
	var directAt time.Time
	go func() {
		for s := range stateCh {
			line := fmt.Sprintf("%.1fs %s", time.Since(start).Seconds(), s.Path)
			if s.Transport != "" {
				line += " " + s.Transport
			}
			if s.Error != "" {
				line += " (" + s.Error + ")"
			}
			res.States = append(res.States, line)
			if (s.Path == p2p.PathDirect || s.Path == p2p.PathLocal) && directAt.IsZero() {
				directAt = time.Now()
			}
		}
	}()
	done := func(err error) error {
		if err != nil {
			res.Error = err.Error()
		}
		st := c.State()
		res.Path, res.Transport, res.Community, res.RelayAllows = st.Path, st.Transport, st.Community, st.RelayAllows
		for _, a := range c.Host().Addrs() {
			res.OwnAddrs = append(res.OwnAddrs, a.String())
		}
		time.Sleep(100 * time.Millisecond)
		writeJSON(*out, res)
		return nil
	}

	get := func(path string, timeout time.Duration) (int64, error) {
		rctx, rcancel := context.WithTimeout(ctx, timeout)
		defer rcancel()
		req, _ := http.NewRequestWithContext(rctx, "GET", "http://server"+path, nil)
		r, err := c.Transport().RoundTrip(req)
		if err != nil {
			return 0, err
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			return 0, fmt.Errorf("status %d", r.StatusCode)
		}
		return io.Copy(io.Discard, r.Body)
	}

	if *invite != "" {
		// What an app does with an invite link (wittyconnect's Connector.Enrol): show it with this
		// device's own key if that gets through, else from the invite's own enrol key, which the
		// server lists with its relay while the invite is open (#614).
		ectx, ecancel := context.WithTimeout(ctx, 90*time.Second)
		wctx, wcancel := context.WithTimeout(ectx, 20*time.Second)
		direct := c.WaitConnected(wctx)
		wcancel()
		var er p2p.EnrolResponse
		if direct {
			er, err = c.Enrol(ectx, *invite, "netsim device", "desktop")
		} else {
			er, err = p2p.EnrolThroughRelay(ectx, cc, *invite, "netsim device", "desktop")
			if err == nil && er.OK {
				c.NetworkChanged()
			}
		}
		ecancel()
		res.Enrolled = err == nil && er.OK
		switch {
		case err != nil:
			res.EnrolError = err.Error()
		case !er.OK:
			res.EnrolError = er.Reason + ": " + er.Error
		}
		if !res.Enrolled && !*refused {
			return done(fmt.Errorf("enrolling with the invite: %s", res.EnrolError))
		}
		if res.Enrolled && *refused {
			return done(errors.New("a device with a wrong or used invite was put on the list"))
		}
	}

	if *refused {
		// A stranger: it may find the server, and may even connect (a
		// connection says nothing), but no request of its gets an answer, by
		// any path, for as long as it keeps trying.
		end := time.Now().Add(*wait)
		for time.Now().Before(end) {
			res.Requests++
			if _, err := get("/hello", 5*time.Second); err == nil {
				res.Answered++
			}
			time.Sleep(time.Second)
		}
		if res.Answered > 0 {
			return done(fmt.Errorf("a device that isn't on the list was answered %d times", res.Answered))
		}
		res.OK = true
		return done(nil)
	}

	// Connected, and usable (a public relay that only introduces doesn't count).
	deadline := time.Now().Add(*wait)
	for {
		st := c.State()
		if st.Connected() && (st.Path != p2p.PathRelayed || st.Community) {
			res.FirstPath = st.Path
			break
		}
		if time.Now().After(deadline) {
			return done(fmt.Errorf("no usable connection after %v: %+v", *wait, st))
		}
		time.Sleep(100 * time.Millisecond)
	}
	res.ConnectSec = time.Since(start).Seconds()
	if _, err := get("/hello", 30*time.Second); err != nil {
		return done(fmt.Errorf("/hello: %w", err))
	}
	t0 := time.Now()
	n, err := get(fmt.Sprintf("/big?mb=%d", *mb), 10*time.Minute)
	if err != nil {
		return done(fmt.Errorf("/big: %w", err))
	}
	res.Mbps = float64(n) * 8 / 1e6 / time.Since(t0).Seconds()

	// A relayed connection should become direct where the routers allow it.
	if res.FirstPath == p2p.PathRelayed {
		until := time.Now().Add(*upgrade)
		for time.Now().Before(until) && directAt.IsZero() {
			time.Sleep(250 * time.Millisecond)
		}
		if !directAt.IsZero() {
			res.DirectAfter = directAt.Sub(start).Seconds()
		}
	}

	if *loop > 0 {
		end := time.Now().Add(*loop)
		var lastOK = time.Now()
		var longest time.Duration
		revokedAt := func() int64 {
			var st struct {
				RevokedAt int64 `json:"revokedAt"`
			}
			b, _ := os.ReadFile(*srv)
			_ = json.Unmarshal(b, &st)
			return st.RevokedAt
		}
		for time.Now().Before(end) {
			res.Requests++
			asked := time.Now()
			if _, err := get("/hello", 5*time.Second); err != nil {
				res.Failed++
			} else if at := revokedAt(); *revoked && at > 0 && asked.UnixMilli() > at+1000 {
				// Asked a second after the server took it off the list, and answered.
				res.AnsweredRevoked++
			} else {
				if gap := time.Since(lastOK); gap > longest {
					longest = gap
				}
				lastOK = time.Now()
			}
			time.Sleep(500 * time.Millisecond)
		}
		if gap := time.Since(lastOK); gap > longest {
			longest = gap
		}
		res.LongestGapMs = longest.Milliseconds()
		if *revoked {
			if revokedAt() == 0 {
				return done(errors.New("the server never took the device off its list, so this proves nothing"))
			}
			if res.AnsweredRevoked > 0 {
				return done(fmt.Errorf("answered %d times after being taken off the list", res.AnsweredRevoked))
			}
			res.OK = true
			return done(nil)
		}
		if time.Since(lastOK) > 10*time.Second {
			return done(fmt.Errorf("never came back: last answer %v ago", time.Since(lastOK).Round(time.Second)))
		}
	}
	res.OK = true
	return done(nil)
}
