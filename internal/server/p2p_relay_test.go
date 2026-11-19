// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/p2p"
	"github.com/wittywilla/wittymediaserver/internal/portmap"
)

// The headline on the page: one answer for each kind of network, with what to do.
func TestP2PVerdictInPlainWords(t *testing.T) {
	base := p2p.Status{Running: true, Layers: p2p.AllLayers(), Started: time.Now().Unix(), DHTPeers: 40}
	cases := []struct {
		name  string
		edit  func(*p2p.Status)
		state string
		title string
	}{
		{"reached from outside", func(s *p2p.Status) { s.Reachable = []string{"/ip4/198.51.100.2/udp/1/quic-v1"} }, "ok", "Direct connection works"},
		{"hole punching", func(s *p2p.Status) { s.NATUDP = "open" }, "ok", "Direct connection works"},
		{"symmetric, no relay", func(s *p2p.Status) { s.NATUDP = "strict" }, "warn", "Your network blocks direct connections; we'll use a relay when WittyRelay is available"},
		{"shared address, no relay", func(s *p2p.Status) { s.Mapping = portmap.HolderStatus{Shared: true} }, "warn", "Your network blocks direct connections; we'll use a relay when WittyRelay is available"},
		{"symmetric, own relay", func(s *p2p.Status) {
			s.NATUDP = "strict"
			s.OwnRelays = []p2p.OwnRelayStatus{{State: "ok"}}
		}, "ok", "Connections will go through your relay"},
		{"symmetric, community relay", func(s *p2p.Status) {
			s.NATUDP = "strict"
			s.Relays = []p2p.RelayStatus{{Community: true, Allows: "Up to 4 Mbit/s per connection."}}
		}, "ok", "Connections will go through your relay"},
		{"nothing outside answers", func(s *p2p.Status) { s.DHTPeers, s.Started = 0, time.Now().Add(-10*time.Minute).Unix() }, "fail", "This server can't reach the internet for peer-to-peer connections"},
		{"just turned on", func(s *p2p.Status) {}, "wait", "Still finding out"},
	}
	for _, c := range cases {
		st := base
		c.edit(&st)
		v := verdictP2P(st)
		if v.State != c.state || v.Title != c.title || v.Detail == "" || v.Todo == nil {
			t.Errorf("%s: %+v", c.name, v)
		}
		if c.state == "warn" && len(v.Todo) == 0 {
			t.Errorf("%s: a warning says what to do: %+v", c.name, v)
		}
	}
	// A WittyRelay that refused says why, whatever else is true.
	st := base
	st.NATUDP = "open"
	st.OwnRelays = []p2p.OwnRelayStatus{{State: "refused", Error: "That access key isn't one this relay knows."}}
	if v := verdictP2P(st); !strings.Contains(strings.Join(v.Todo, " "), "access key") {
		t.Fatalf("%+v", v)
	}
}

// A WittyRelay's access key is kept on the server: never sent to the page, kept when
// the page sends the relay back without it, and checked like every other setting.
func TestWittyRelayKeysStayOnTheServer(t *testing.T) {
	ts := newTestServer(t)
	ts.p2p.tune = func(c *p2p.NodeConfig) { c.Local, c.Listen, c.Layers = true, p2pLoopback, p2p.Layers{} }
	_, admin := ts.user(t, "owner", "adult", true, "")
	addr := "/dns4/relay.example.com/udp/4001/quic-v1/p2p/" + mustID(t, 7)
	w := ts.do(t, "PUT", "/api/p2p/settings", admin, map[string]any{"ownRelays": []map[string]any{{"addr": addr, "key": "a-secret-relay-key-01234"}}})
	if w.Code != 200 || strings.Contains(w.Body.String(), "a-secret-relay-key") || !strings.Contains(w.Body.String(), `"hasKey":true`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	// Saved again from the page, which never had the key: it stays.
	w = ts.do(t, "PUT", "/api/p2p/settings", admin, map[string]any{"ownRelays": []map[string]any{{"addr": addr, "hasKey": true}}, "shareKbps": 4000, "shareMaxHeight": 480})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	cfg := ts.loadP2P(context.Background())
	if len(cfg.OwnRelays) != 1 || cfg.OwnRelays[0].Key != "a-secret-relay-key-01234" || cfg.ShareKbps != 4000 || cfg.ShareMaxHeight != 480 {
		t.Fatalf("%+v", cfg)
	}
	var v p2pView
	_ = json.Unmarshal(ts.do(t, "GET", "/api/p2p/settings", admin, nil).Body.Bytes(), &v)
	if v.Settings.OwnRelays[0].Key != "" || !v.Settings.OwnRelays[0].HasKey {
		t.Fatalf("%+v", v.Settings.OwnRelays)
	}
	for _, bad := range []map[string]any{
		{"ownRelays": []map[string]any{{"addr": "relay.example.com:4001"}}},
		{"ownRelays": []map[string]any{{"addr": addr}, {"addr": addr}, {"addr": addr}, {"addr": addr}, {"addr": addr}}},
		{"shareKbps": 100},
		{"shareMaxHeight": 99999},
	} {
		if w := ts.do(t, "PUT", "/api/p2p/settings", admin, bad); w.Code != 400 {
			t.Errorf("%v was taken: %d", bad, w.Code)
		}
	}
	// Checking a relay is for the owner, and needs an address.
	_, kate := ts.user(t, "kate", "adult", false, "")
	if w := ts.do(t, "POST", "/api/p2p/relay-check", kate, map[string]any{"addr": addr}); w.Code != 403 {
		t.Fatalf("a non-admin checked a relay: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/p2p/relay-check", admin, map[string]any{"addr": "nonsense"}); w.Code != 400 {
		t.Fatalf("%d", w.Code)
	}
}

// Making invites and typing in codes is limited per account.
func TestP2PPairingIsRateLimited(t *testing.T) {
	ts, admin := p2pServer(t, nil)
	codes := 0
	for range 12 {
		w := ts.do(t, "POST", "/api/p2p/invite", admin, map[string]any{})
		if w.Code == 429 {
			codes++
		}
	}
	if codes == 0 {
		t.Fatal("twelve invites in a moment were all made")
	}
}

func mustID(t *testing.T, b byte) string {
	t.Helper()
	id, err := p2p.IDFromSeed(p2pSeedFor(b))
	if err != nil {
		t.Fatal(err)
	}
	return id
}
