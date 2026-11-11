// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/wittywilla/wittymediaserver/internal/p2p"
)

// p2pBook is an address book of this machine's own: a DHT in server mode,
// under the protocol name the tests' nodes speak, so none of it can join the
// public one.
func p2pBook(t *testing.T) peer.AddrInfo {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"), libp2p.DisableMetrics())
	if err != nil {
		t.Fatal(err)
	}
	d, err := dht.New(h, dht.Mode(dht.ModeServer), dht.ProtocolPrefix("/wittytest"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close(); h.Close() })
	return peer.AddrInfo{ID: h.ID(), Addrs: h.Addrs()}
}

// A TV away from home shows a code; someone signed in types it into the
// server; the TV finds the server by it, is put on the list and signed in.
func TestP2PJoinByACodeShownOnTheDevice(t *testing.T) {
	book := p2pBook(t)
	ts, admin := p2pServer(t, func(c *p2p.NodeConfig) {
		c.Layers.DHT = true
		c.Bootstrap = []peer.AddrInfo{book}
	})
	node := ts.p2pNode()
	for i := 0; len(node.Host().Network().ConnsToPeer(book.ID)) == 0; i++ {
		if i > 200 {
			t.Fatal("the server never reached the address book")
		}
		time.Sleep(50 * time.Millisecond)
	}
	_, kate := ts.user(t, "kate", "adult", false, "")
	_, kid := ts.user(t, "kid", "kids", false, "")
	code, _ := p2p.NewJoinCode()

	// Who may, and with what.
	if w := ts.do(t, "POST", "/api/p2p/join", kid, map[string]any{"code": code}); w.Code != 403 {
		t.Fatalf("a kid's profile typing a code in: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/p2p/join", "", map[string]any{"code": code}); w.Code != 401 {
		t.Fatalf("nobody signed in: %d", w.Code)
	}
	for _, bad := range []string{"", "K7QM", "K7QM-2XHU", "wittymedia://p2p/x"} {
		if w := ts.do(t, "POST", "/api/p2p/join", kate, map[string]any{"code": bad}); w.Code != 400 {
			t.Fatalf("%q taken for a code: %d %s", bad, w.Code, w.Body)
		}
	}
	// A device signed in as the administrator can do everything: that takes the password again.
	w := ts.do(t, "POST", "/api/p2p/join", admin, map[string]any{"code": code})
	var refusal struct{ Reason string }
	_ = json.Unmarshal(w.Body.Bytes(), &refusal)
	if w.Code != 403 || refusal.Reason != pairNeedsPassword {
		t.Fatalf("an administrator's code with no password: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/p2p/join", admin, map[string]any{"code": code, "password": "not the password"}); w.Code == 200 {
		t.Fatalf("a wrong password was taken: %s", w.Body)
	}
	// Nothing so far has made the server findable by the code.
	device := func(seed byte, code string) p2p.JoinConfig {
		return p2p.JoinConfig{Seed: p2pSeedFor(seed), Code: code, Name: "Living room\n\x00", Platform: "tvOS", Local: true,
			Bootstrap: []peer.AddrInfo{book}, Listen: p2pLoopback, Every: 100 * time.Millisecond}
	}
	short, stop := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	if res, err := p2p.Join(short, device(2, code)); err == nil {
		t.Fatalf("a device joined on a code nobody was allowed to type in: %+v", res)
	}
	stop()

	// Kate types in what her TV shows, in her own way.
	w = ts.do(t, "POST", "/api/p2p/join", kate, map[string]any{"code": " " + code[:4] + " " + code[5:] + " "})
	var took struct {
		OK           bool
		ExpiresInSec int
		User         struct{ Name string }
	}
	_ = json.Unmarshal(w.Body.Bytes(), &took)
	if w.Code != 200 || !took.OK || took.ExpiresInSec != 900 || took.User.Name != "kate" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	res, err := p2p.Join(ctx, device(2, code))
	if err != nil || !res.OK || res.ServerID != node.ID() || len(res.Code) != 6 || len(res.Addrs) == 0 {
		t.Fatalf("the TV joining: %+v %v", res, err)
	}
	// On the list under Kate's account, and what came back signs it in as her, over the connection.
	app, err := p2p.NewClient(p2p.ClientConfig{Seed: p2pSeedFor(2), ServerID: res.ServerID, Hints: res.Addrs, Local: true, Listen: p2pLoopback, UpgradeWait: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	status, body, err := over(t, app, "POST", "/api/pair/redeem", "", map[string]any{"code": res.Code, "device": "Living room"})
	if err != nil || status != 200 {
		t.Fatalf("redeeming the code: %d %s %v", status, body, err)
	}
	var signed struct {
		Token string
		User  struct{ Name string }
	}
	_ = json.Unmarshal([]byte(body), &signed)
	if signed.Token == "" || signed.User.Name != "kate" {
		t.Fatalf("signed in as: %s", body)
	}
	var list []p2pDevice
	_ = json.Unmarshal(ts.do(t, "GET", "/api/p2p/devices", kate, nil).Body.Bytes(), &list)
	if len(list) != 1 || list[0].ID != app.ID() || list[0].Platform != "tvOS" || list[0].Name != "Living room" {
		t.Fatalf("the device on the list: %+v", list)
	}

	// A code dies with the sign-in that typed it in, as an invite does.
	_, lee := ts.user(t, "lee", "adult", false, "")
	second, _ := p2p.NewJoinCode()
	if w := ts.do(t, "POST", "/api/p2p/join", lee, map[string]any{"code": second}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	ts.Auth.Logout(ctx, lee)
	res, err = p2p.Join(ctx, device(3, second))
	if err != nil || res.OK || res.Reason != "expired" {
		t.Fatalf("a code whose typist signed out: %+v %v", res, err)
	}
	if ts.p2p.devices.Allowed(testP2PID(t, 3)) {
		t.Fatal("a refused code still put the device on the list")
	}
}

func testP2PID(t *testing.T, seed byte) string {
	t.Helper()
	id, err := p2p.IDFromSeed(p2pSeedFor(seed))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// With the address book off there is nowhere for a device that knows only a
// code to look, and the server says so instead of taking the code.
func TestP2PJoinNeedsTheAddressBookAndTheFeature(t *testing.T) {
	ts, _ := p2pServer(t, nil)
	_, kate := ts.user(t, "kate", "adult", false, "")
	code, _ := p2p.NewJoinCode()
	if w := ts.do(t, "POST", "/api/p2p/join", kate, map[string]any{"code": code}); w.Code != 409 {
		t.Fatalf("with no address book: %d %s", w.Code, w.Body)
	}
	plain := newTestServer(t)
	_, sam := plain.user(t, "sam", "adult", false, "")
	if w := plain.do(t, "POST", "/api/p2p/join", sam, map[string]any{"code": code}); w.Code != 409 {
		t.Fatalf("with peer-to-peer connections off: %d %s", w.Code, w.Body)
	}
}
