// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Command wittyconnect is the desktop app's helper for peer-to-peer
// connections: the app starts it, tells it which server to reach, and gets
// back a local address to load the server from.
//
// It is driven over its standard input and output, one JSON object per line,
// so nothing secret is ever on a command line where other programs can read
// it. The app writes the configuration first:
//
//	{"key":"…","serverId":"12D3KooW…","hints":["/ip4/…"]}
//
// and reads events:
//
//	{"event":"ready","port":51234,"secret":"…","deviceId":"12D3KooW…"}
//	{"event":"state","state":{"path":"direct","transport":"quic"}}
//	{"event":"addrs","addrs":["/ip4/…"]}            (after a path is found; to keep as hints)
//	{"event":"enrol","result":{"ok":true,"code":"123456"}}
//	{"event":"error","error":"…"}
//
// Further lines are commands: {"cmd":"enrol","secret":"…","name":"…","platform":"desktop"},
// {"cmd":"network-changed"}, {"cmd":"addrs"}. When its input closes, because
// the app quit or crashed, it exits: it never outlives the app.
//
// "wittyconnect keygen" prints a new device key and its ID and exits.
// "wittyconnect version" prints the version of the protocol it speaks.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/pkg/wittyconnect"
)

type out struct {
	mu sync.Mutex
	w  *json.Encoder
}

func (o *out) send(v any) {
	o.mu.Lock()
	defer o.mu.Unlock()
	_ = o.w.Encode(v)
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "keygen":
			key, err := wittyconnect.NewKey()
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			id, _ := wittyconnect.DeviceID(key)
			_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"key": key, "deviceId": id})
			return
		case "version":
			fmt.Println("wittyconnect 1")
			return
		default:
			fmt.Fprintln(os.Stderr, "wittyconnect is started by the WittyMediaClient app; see docs/P2P.md in WittyMediaServer")
			os.Exit(2)
		}
	}
	os.Exit(run(os.Stdin, os.Stdout, nil))
}

// run is the helper's life: a configuration in, events out, until the input
// closes. tune changes the configuration before starting, for tests.
func run(stdin io.Reader, stdout io.Writer, tune func(*wittyconnect.Config)) int {
	o := &out{w: json.NewEncoder(stdout)}
	in := bufio.NewScanner(stdin)
	in.Buffer(make([]byte, 0, 64<<10), 256<<10)
	if !in.Scan() {
		return 2
	}
	var cfg wittyconnect.Config
	if err := json.Unmarshal(in.Bytes(), &cfg); err != nil {
		o.send(map[string]any{"event": "error", "error": "the configuration isn't valid JSON"})
		return 2
	}
	// What it logs goes to the app, which keeps it with its own log.
	cfg.Log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	var conn *wittyconnect.Connector
	var connMu sync.Mutex
	cfg.OnState = func(s wittyconnect.State) {
		o.send(map[string]any{"event": "state", "state": s})
		if s.Connected() {
			connMu.Lock()
			c := conn
			connMu.Unlock()
			if c != nil {
				o.send(map[string]any{"event": "addrs", "addrs": c.KnownAddrs()})
			}
		}
	}
	if tune != nil {
		tune(&cfg)
	}
	c, err := wittyconnect.Start(cfg)
	if err != nil {
		o.send(map[string]any{"event": "error", "error": err.Error()})
		return 1
	}
	connMu.Lock()
	conn = c
	connMu.Unlock()
	defer c.Close()
	o.send(map[string]any{"event": "ready", "port": c.Port(), "secret": c.Secret(), "deviceId": c.DeviceID()})
	o.send(map[string]any{"event": "state", "state": c.State()})

	for in.Scan() {
		var cmd struct {
			Cmd      string `json:"cmd"`
			Secret   string `json:"secret"`
			Name     string `json:"name"`
			Platform string `json:"platform"`
		}
		if json.Unmarshal(in.Bytes(), &cmd) != nil {
			continue
		}
		switch cmd.Cmd {
		case "network-changed":
			c.NetworkChanged()
		case "addrs":
			o.send(map[string]any{"event": "addrs", "addrs": c.KnownAddrs()})
		case "enrol":
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
				defer cancel()
				res, err := c.Enrol(ctx, cmd.Secret, cmd.Name, cmd.Platform)
				if err != nil {
					o.send(map[string]any{"event": "enrol", "result": map[string]any{"ok": false, "error": err.Error()}})
					return
				}
				o.send(map[string]any{"event": "enrol", "result": res})
			}()
		case "quit":
			return 0
		}
	}
	return 0 // the app closed our input: it is gone, and so are we
}
