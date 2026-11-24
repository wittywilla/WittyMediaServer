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

func checkByID(list []reachCheck) map[string]reachCheck {
	out := map[string]reachCheck{}
	for _, c := range list {
		out[c.ID] = c
	}
	return out
}

func TestCheckP2PLines(t *testing.T) {
	cfg := defaultP2P()
	if got := checkP2P(cfg, p2p.Status{}, false, 0); len(got) != 1 || got[0].State != "fail" || got[0].Next == "" {
		t.Fatalf("off: %+v", got)
	}
	cfg.Enabled = true
	if got := checkP2P(cfg, p2p.Status{}, false, 0); len(got) != 1 || got[0].State != "fail" {
		t.Fatalf("not running: %+v", got)
	}

	// Started long ago, behind a router, nothing held: each thing that is missing says what to do.
	old := time.Now().Add(-time.Hour).Unix()
	st := p2p.Status{Running: true, Port: 4001, Started: old, Layers: p2p.AllLayers(), Reachability: "private",
		Addrs: []string{"/ip4/192.168.1.5/udp/4001/quic-v1"}, Mapping: portmap.HolderStatus{Error: "no router answered"}}
	cfg.DevicesOnly = false
	c := checkByID(checkP2P(cfg, st, true, 0))
	for id, want := range map[string]string{"on": "ok", "devicesOnly": "warn", "devices": "warn", "dht": "fail", "relay": "fail", "public": "warn", "upnp": "warn", "ipv6": "warn"} {
		if c[id].State != want || c[id].Title == "" || c[id].Detail == "" {
			t.Errorf("%s: %+v, want %s", id, c[id], want)
		}
		if (want == "fail" || id == "devicesOnly") && c[id].Next == "" {
			t.Errorf("%s says nothing to do: %+v", id, c[id])
		}
	}

	// All well: listed, relayed, reachable, mapped, and on IPv6.
	cfg.DevicesOnly = true
	st.DHTPeers, st.Relays = 40, []p2p.RelayStatus{{ID: "r"}}
	st.Addrs = append(st.Addrs, "/ip4/203.0.113.7/udp/4001/quic-v1", "/ip6/2001:db8::7/udp/4001/quic-v1")
	st.Reachable = []string{"/ip4/203.0.113.7/udp/4001/quic-v1"}
	st.Mapping = portmap.HolderStatus{On: true, Protocol: "UPnP"}
	c = checkByID(checkP2P(cfg, st, true, 2))
	for _, id := range []string{"on", "devicesOnly", "devices", "dht", "relay", "public", "upnp"} {
		if c[id].State != "ok" {
			t.Errorf("%s: %+v", id, c[id])
		}
	}
	// A public IPv6 address; a shared router address is said.
	if c["ipv6"].State != "ok" {
		t.Errorf("ipv6: %+v", c["ipv6"])
	}
	st.Mapping.Shared = true
	st.OwnRelays = []p2p.OwnRelayStatus{{State: "refused", Error: "the relay said no (key)"}}
	c = checkByID(checkP2P(cfg, st, true, 2))
	if c["upnp"].State != "warn" || c["relay"].State != "fail" || c["relay"].Next == "" {
		t.Errorf("shared and refused: %+v %+v", c["upnp"], c["relay"])
	}
	// Layers turned off are said as off, not as failures.
	st.Layers = p2p.Layers{}
	c = checkByID(checkP2P(cfg, st, true, 2))
	for _, id := range []string{"dht", "relay", "upnp", "ipv6"} {
		if c[id].State != "off" {
			t.Errorf("%s off: %+v", id, c[id])
		}
	}
	if got := addrKinds([]string{"/ip4/8.8.8.8/udp/4001/quic-v1", "/ip6/2001:db8::1/tcp/4001"}); strings.Contains(got, "8.8.8.8") || !strings.Contains(got, "IPv4 and IPv6") || !strings.Contains(got, "4001") {
		t.Errorf("addrKinds: %q", got)
	}
	if got := publicAddrs([]string{"/ip4/100.64.1.1/tcp/1", "/ip4/10.0.0.1/tcp/1", "/ip4/8.8.8.8/tcp/1", "/ip4/8.8.8.8/tcp/1/p2p/x/p2p-circuit", "/dns4/x/tcp/1"}); len(got) != 1 || got[0] != "/ip4/8.8.8.8/tcp/1" {
		t.Errorf("publicAddrs: %v", got)
	}
}

// A device that isn't listed, and one with a wrong invite, are both in the
// owner's history, with the step and the words the device itself shows.
func TestP2PCheckAndAttempts(t *testing.T) {
	ts, admin := p2pServer(t, nil)
	_, kate := ts.user(t, "kate", "adult", false, "")

	w := ts.do(t, "POST", "/api/p2p/check", admin, nil)
	var check struct {
		Checks  []reachCheck
		Verdict *p2pVerdict
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &check) != nil || len(check.Checks) < 5 || check.Verdict == nil {
		t.Fatalf("check: %d %s", w.Code, w.Body)
	}
	if c := checkByID(check.Checks); c["on"].State != "ok" || c["devicesOnly"].State != "ok" {
		t.Fatalf("check lines: %+v", check.Checks)
	}
	if w := ts.do(t, "POST", "/api/p2p/check", kate, nil); w.Code != 403 {
		t.Fatalf("check for someone who isn't the owner: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/p2p/attempts", kate, nil); w.Code != 403 {
		t.Fatalf("attempts for someone who isn't the owner: %d", w.Code)
	}

	stranger := p2pApp(t, ts, 7)
	if _, _, err := over(t, stranger, "GET", "/api/capabilities", "", nil); err == nil {
		t.Fatal("a device that isn't on the list was answered")
	}
	if res, err := stranger.Enrol(context.Background(), "AAAAAAAAAAAAAAAAAAAAAA", "Hotel TV", "android"); err != nil || res.OK {
		t.Fatalf("a wrong invite: %+v %v", res, err)
	}
	listed := p2pApp(t, ts, 8)
	enrol(t, ts, kate, listed, "Kate's phone")
	if code, _, err := over(t, listed, "GET", "/api/capabilities", "", nil); err != nil || code != 200 {
		t.Fatalf("listed: %d %v", code, err)
	}

	var got struct {
		Attempts []p2pAttempt
		Steps    []struct{ ID string }
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		w = ts.do(t, "GET", "/api/p2p/attempts?device="+stranger.ID(), admin, nil)
		_ = json.Unmarshal(w.Body.Bytes(), &got)
		if len(got.Attempts) >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(got.Steps) != 4 || got.Steps[0].ID != "read" {
		t.Fatalf("steps: %+v", got.Steps)
	}
	var notListed, wrong bool
	for _, a := range got.Attempts {
		if a.Device != stranger.ID() || a.Listed || a.OK || a.Short == "" || a.Reason == "" || a.Title == "" {
			t.Errorf("stranger's try: %+v", a)
		}
		notListed = notListed || (a.Kind == "connect" && a.Step == "signin" && a.Code == "not-listed")
		wrong = wrong || (a.Kind == "invite" && a.Code == "wrong" && a.Name == "Hotel TV")
	}
	if !notListed || !wrong {
		t.Fatalf("stranger's tries: %+v", got.Attempts)
	}

	w = ts.do(t, "GET", "/api/p2p/attempts", admin, nil)
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	var kates bool
	for _, a := range got.Attempts {
		if a.Device == listed.ID() && a.OK && a.Listed && a.Name == "Kate's phone" {
			kates = true
		}
	}
	if !kates {
		t.Fatalf("the listed device's connection: %+v", got.Attempts)
	}
}
