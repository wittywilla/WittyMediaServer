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
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
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

type devices struct{ ids map[string]bool }

func (d devices) Allowed(id string) bool { return d.ids[id] }
func (d devices) Seen(string, string)    {}

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
	_ = fs.Parse(args)
	ids := map[string]bool{}
	var list []string
	for _, d := range strings.Split(*device, ",") {
		if d = strings.TrimSpace(d); d != "" {
			ids[d], list = true, append(list, d)
		}
	}
	cfg := p2p.NodeConfig{
		Seed: seedOf(1), Port: *port, Layers: p2p.Layers{Relay: true, HolePunch: true}, DevicesOnly: true,
		Devices: devices{ids}, DeviceIDs: func() []string { return list }, Name: "netsim server",
		Handler: handler(), Log: logger(),
		Listen: []string{fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", *port), fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", *port)},
	}
	if *relay != "" {
		cfg.OwnRelays = []p2p.OwnRelay{{Addr: *relay, Key: *key}}
	}
	n, err := p2p.StartNode(cfg)
	if err != nil {
		return err
	}
	defer n.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if *obs != "" {
		go meet(ctx, n.Host(), *obs)
	}
	go func() {
		for {
			st := n.Status()
			writeJSON(*out, map[string]any{"id": n.ID(), "addrs": n.Addrs(), "status": st})
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
	OK           bool     `json:"ok"`
	Error        string   `json:"error,omitempty"`
	ConnectSec   float64  `json:"connectSec"`
	FirstPath    string   `json:"firstPath"`
	Path         string   `json:"path"`
	Transport    string   `json:"transport,omitempty"`
	Community    bool     `json:"community"`
	RelayAllows  string   `json:"relayAllows,omitempty"`
	DirectAfter  float64  `json:"directAfterSec,omitempty"` // when a relayed connection became direct
	MB           int      `json:"mb"`
	Mbps         float64  `json:"mbps"`
	Requests     int      `json:"requests,omitempty"`
	Failed       int      `json:"failed,omitempty"`
	LongestGapMs int64    `json:"longestOutageMs,omitempty"`
	States       []string `json:"states"`
	OwnAddrs     []string `json:"ownAddrs"`
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
	c, err := p2p.NewClient(p2p.ClientConfig{Seed: seedOf(*seed), ServerID: info.ID, Hints: info.Addrs,
		Layers: p2p.Layers{Relay: true, HolePunch: true}, Log: logger(),
		Listen: []string{"/ip4/0.0.0.0/udp/0/quic-v1", "/ip4/0.0.0.0/tcp/0"},
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
		for time.Now().Before(end) {
			res.Requests++
			if _, err := get("/hello", 5*time.Second); err != nil {
				res.Failed++
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
		if time.Since(lastOK) > 10*time.Second {
			return done(fmt.Errorf("never came back: last answer %v ago", time.Since(lastOK).Round(time.Second)))
		}
	}
	res.OK = true
	return done(nil)
}
