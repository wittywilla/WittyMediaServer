// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/hex"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/wittywilla/wittymediaserver/internal/p2p"
)

// restartPairing forgets every open invite and code in memory, as stopping the
// server does, and starts again from what was saved.
func restartPairing(t *testing.T, ts *testServer) {
	t.Helper()
	ts.p2p.invites.mu.Lock()
	ts.p2p.invites.open = nil
	ts.p2p.invites.mu.Unlock()
	ts.connect.mu.Lock()
	ts.connect.open = nil
	ts.connect.mu.Unlock()
	ts.p2pForgetSavedJoins()
	ts.p2p.pending.jmu.Lock()
	ts.p2p.pending.codesBefore = nil
	ts.p2p.pending.jmu.Unlock()
	ts.p2pLoadPending(context.Background())
}

var testFrom = netip.MustParseAddr("198.51.100.7")

// Invite links outlive a restart (#683, #692): one shown before it puts a
// device on the list after it, and its enrol key is on the relay list again.
// Codes don't: a code, or the keys made from it, is never written down. A
// device typing one is told the server restarted, and the person who showed it
// is told so too, for their screen to show a new one. What is saved holds no
// secret, no code and no working sign-in.
func TestInvitesOutliveARestartAndCodesSaySo(t *testing.T) {
	var relayList func() []string
	ts, admin := p2pServer(t, func(c *p2p.NodeConfig) { relayList = c.DeviceIDs })
	_, kate := ts.user(t, "kate", "adult", false, "")

	w := ts.do(t, "POST", "/api/p2p/invite", admin, map[string]any{"password": "correct-horse-1"})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	inv, _ := p2p.ParseInvite(decode[map[string]any](t, w)["link"].(string))
	enrolID, _ := p2p.EnrolID(inv.Secret)
	o := makeOffer(t, ts, kate, nil)
	link, _ := p2p.ParseInvite(o.Link)
	d := newCodeDevice(t, o.Code)
	if st := decode[map[string]any](t, ts.do(t, "GET", "/api/connect/offer", kate, nil)); st["open"] != true || st["restarted"] != false {
		t.Fatalf("an open code: %v", st)
	}

	saved := ts.DB.GetSetting(context.Background(), p2pPendingKey, "")
	cleanCode, _ := p2p.CleanJoinCode(o.Code)
	findHex, _, _ := p2p.JoinKeysOf(o.Code)
	for _, secret := range []string{inv.Secret, link.Secret, o.Code, cleanCode, hex.EncodeToString(d.find), hex.EncodeToString(d.proof), findHex, admin, kate} {
		if secret != "" && strings.Contains(saved, secret) {
			t.Fatalf("what is saved for a restart holds a secret, a code, a code's key or a sign-in: %q in %s", secret, saved)
		}
	}

	restartPairing(t, ts)
	if !slices.Contains(relayList(), enrolID) || !slices.Contains(relayList(), func() string { id, _ := p2p.EnrolID(link.Secret); return id }()) {
		t.Fatal("after a restart, the open invites' enrol keys aren't on the relay list")
	}
	// The code is gone, and says why: to the device typing it, and to the person who showed it.
	code, a := d.hello(t, ts)
	if code != 404 || a.Reason != "restarted" || a.Proof != "" {
		t.Fatalf("a code from before a restart: %d %+v", code, a)
	}
	if st := decode[map[string]any](t, ts.do(t, "GET", "/api/connect/offer", kate, nil)); st["open"] != false || st["restarted"] != true {
		t.Fatalf("the person whose code a restart ended: %v", st)
	}
	if st := decode[map[string]any](t, ts.do(t, "GET", "/api/connect/offer", admin, nil)); st["restarted"] != false {
		t.Fatalf("someone who had no code open: %v", st)
	}
	// The invite made before the restart, used after it.
	got, err := p2pApp(t, ts, 4).Enrol(context.Background(), inv.Secret, "Phone", "Android")
	if err != nil || !got.OK || len(got.Code) != 6 {
		t.Fatalf("an invite made before a restart, used after it: %+v %v", got, err)
	}
	// The link beside the ended code still works: its QR code still on the screen is good until it runs out.
	if r := ts.p2pEnrol(context.Background(), testP2PID(t, 7), p2p.EnrolRequest{V: 1, Secret: link.Secret}, testFrom); !r.OK {
		t.Fatalf("the link beside a code a restart ended: %+v", r)
	}
	// Used invites stay used across a restart, for any other device.
	restartPairing(t, ts)
	if r := ts.p2pEnrol(context.Background(), testP2PID(t, 8), p2p.EnrolRequest{V: 1, Secret: inv.Secret}, testFrom); r.OK || r.Reason != "used" {
		t.Fatalf("a used invite after a restart: %+v", r)
	}
	// A new code after the restart: the screen is told it is open again.
	makeOffer(t, ts, kate, nil)
	if st := decode[map[string]any](t, ts.do(t, "GET", "/api/connect/offer", kate, nil)); st["open"] != true || st["restarted"] != false {
		t.Fatalf("a new code after the restart: %v", st)
	}

	// The link beside a code that was used is used after a restart too: one way in.
	o = makeOffer(t, ts, kate, nil)
	link, _ = p2p.ParseInvite(o.Link)
	d = newCodeDevice(t, o.Code)
	_, a = d.hello(t, ts)
	if code, out := d.answer(t, ts, a, d.proof); code != 200 {
		t.Fatalf("the code: %d %v", code, out)
	}
	restartPairing(t, ts)
	if r := ts.p2pEnrol(context.Background(), testP2PID(t, 9), p2p.EnrolRequest{V: 1, Secret: link.Secret}, testFrom); r.OK || r.Reason != "used" {
		t.Fatalf("the link beside a used code, after a restart: %+v", r)
	}

	// Signing out still ends an invite, after a restart too.
	o = makeOffer(t, ts, kate, nil)
	link, _ = p2p.ParseInvite(o.Link)
	restartPairing(t, ts)
	ts.Auth.Logout(context.Background(), kate)
	if r := ts.p2pEnrol(context.Background(), testP2PID(t, 10), p2p.EnrolRequest{V: 1, Secret: link.Secret}, testFrom); r.OK {
		t.Fatalf("an invite whose maker signed out, after a restart: %+v", r)
	}
}

// The device an invite put on the list may show it again (its answer was lost
// on the way) and is answered again, a few times; any other device is told it
// was used.
func TestAnInviteAnswersItsOwnDeviceAgain(t *testing.T) {
	ts, admin := p2pServer(t, nil)
	w := ts.do(t, "POST", "/api/p2p/invite", admin, map[string]any{"password": "correct-horse-1"})
	inv, _ := p2p.ParseInvite(decode[map[string]any](t, w)["link"].(string))
	dev := testP2PID(t, 4)
	for i := 0; i <= p2pMaxReshows; i++ {
		r := ts.p2pEnrol(context.Background(), dev, p2p.EnrolRequest{V: 1, Secret: inv.Secret, Name: "Phone"}, testFrom)
		if !r.OK || len(r.Code) != 6 {
			t.Fatalf("showing it the %d time: %+v", i+1, r)
		}
	}
	if r := ts.p2pEnrol(context.Background(), dev, p2p.EnrolRequest{V: 1, Secret: inv.Secret}, testFrom); r.OK {
		t.Fatal("an invite answered its device without end")
	}
	if r := ts.p2pEnrol(context.Background(), testP2PID(t, 5), p2p.EnrolRequest{V: 1, Secret: inv.Secret}, testFrom); r.OK || r.Reason != "used" {
		t.Fatalf("another device with a used invite: %+v", r)
	}
	if n := len(ts.p2p.devices.list()); n != 1 {
		t.Fatalf("%d devices listed", n)
	}
}

// What could still refuse a device is asked before the invite is used: a
// refused device leaves it good for the next.
func TestAnInviteIsSpentOnlyOnADeviceItTakes(t *testing.T) {
	ts, admin := p2pServer(t, nil)
	_, kate := ts.user(t, "kate", "adult", false, "")
	katesID := userIDOf(t, ts, kate)
	taken := testP2PID(t, 6)
	if _, err := ts.p2pAddDevice(context.Background(), katesID, taken, "Kate's phone", "android", "signed-in"); err != nil {
		t.Fatal(err)
	}
	w := ts.do(t, "POST", "/api/p2p/invite", admin, map[string]any{"password": "correct-horse-1"})
	inv, _ := p2p.ParseInvite(decode[map[string]any](t, w)["link"].(string))
	// Kate's device can't go on the owner's list...
	if r := ts.p2pEnrol(context.Background(), taken, p2p.EnrolRequest{V: 1, Secret: inv.Secret}, testFrom); r.OK || r.Reason != "refused" {
		t.Fatalf("a device on another account's list: %+v", r)
	}
	// ... and the invite is still good for one that can.
	if r := ts.p2pEnrol(context.Background(), testP2PID(t, 7), p2p.EnrolRequest{V: 1, Secret: inv.Secret}, testFrom); !r.OK {
		t.Fatalf("the invite after a device it couldn't take: %+v", r)
	}
}

// Two devices joining by two invites from the same person each keep the
// sign-in code they were given: the second doesn't end the first's.
func TestTwoDevicesKeepTheirSignInCodes(t *testing.T) {
	ts, admin := p2pServer(t, nil)
	var codes []string
	for i, seed := range []byte{4, 5} {
		w := ts.do(t, "POST", "/api/p2p/invite", admin, map[string]any{"password": "correct-horse-1"})
		inv, _ := p2p.ParseInvite(decode[map[string]any](t, w)["link"].(string))
		r := ts.p2pEnrol(context.Background(), testP2PID(t, seed), p2p.EnrolRequest{V: 1, Secret: inv.Secret}, testFrom)
		if !r.OK || r.Code == "" {
			t.Fatalf("device %d: %+v", i, r)
		}
		codes = append(codes, r.Code)
	}
	for i, c := range codes {
		if w := ts.do(t, "POST", "/api/pair/redeem", "", map[string]any{"code": c, "device": "Phone"}); w.Code != 200 {
			t.Fatalf("device %d's sign-in code: %d %s", i, w.Code, w.Body)
		}
	}
}

// A code a TV showed, typed into the server, still finds it after peer-to-peer
// connections start again (a change of settings): the new node is offered it
// from memory. It is never written down.
func TestP2PJoinCodeOutlivesANodeRestart(t *testing.T) {
	book := p2pBook(t)
	ts, _ := p2pServer(t, func(c *p2p.NodeConfig) {
		c.Layers.DHT = true
		c.Bootstrap = []peer.AddrInfo{book}
	})
	_, kate := ts.user(t, "kate", "adult", false, "")
	code, _ := p2p.NewJoinCode()
	if w := ts.do(t, "POST", "/api/p2p/join", kate, map[string]any{"code": code}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	find, _, _ := p2p.JoinKeysOf(code)
	if saved := ts.DB.GetSetting(context.Background(), p2pPendingKey, ""); strings.Contains(saved, find) || strings.Contains(saved, code) {
		t.Fatalf("a code typed in was written down: %s", saved)
	}
	ts.StopP2P()
	ts.StartP2P(context.Background(), ts.h)
	node := ts.p2pNode()
	if node == nil {
		t.Fatal("no node after starting again")
	}
	if !node.JoinWaiting(find) {
		t.Fatal("the new node isn't waiting for the code typed in before it started")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	res, err := p2p.Join(ctx, p2p.JoinConfig{Seed: p2pSeedFor(2), Code: code, Name: "Living room", Platform: "tvOS", Local: true,
		Bootstrap: []peer.AddrInfo{book}, Listen: p2pLoopback, Every: 100 * time.Millisecond})
	if err != nil || !res.OK || res.ServerID != node.ID() || len(res.Code) != 6 {
		t.Fatalf("the TV joining after the node started again: %+v %v", res, err)
	}
}
