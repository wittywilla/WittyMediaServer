// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
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
	ts.p2pLoadPending(context.Background())
}

var testFrom = netip.MustParseAddr("198.51.100.7")

// Invites and codes outlive a restart (#683): a link shown before it puts a
// device on the list after it, its enrol key is on the relay list again, and a
// code shown beside a link still answers. What is saved holds no secret and no
// working sign-in.
func TestInvitesAndCodesOutliveARestart(t *testing.T) {
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

	saved := ts.DB.GetSetting(context.Background(), p2pPendingKey, "")
	for _, secret := range []string{inv.Secret, link.Secret, o.Code, admin, kate} {
		if strings.Contains(saved, secret) {
			t.Fatalf("what is saved for a restart holds a secret or a sign-in: %q in %s", secret, saved)
		}
	}

	restartPairing(t, ts)
	if !slices.Contains(relayList(), enrolID) {
		t.Fatal("after a restart, the open invite's enrol key isn't on the relay list")
	}
	res := p2pApp(t, ts, 4)
	got, err := res.Enrol(context.Background(), inv.Secret, "Phone", "Android")
	if err != nil || !got.OK || len(got.Code) != 6 {
		t.Fatalf("an invite made before a restart, used after it: %+v %v", got, err)
	}
	d := newCodeDevice(t, o.Code)
	code, a := d.hello(t, ts)
	if code != 200 {
		t.Fatalf("a code made before a restart: %d", code)
	}
	// The link and the code are still one way in after it: the code used, the link is spent.
	if code, out := d.answer(t, ts, a, d.proof); code != 200 {
		t.Fatalf("the code after a restart: %d %v", code, out)
	}
	restartPairing(t, ts)
	if r := ts.p2pEnrol(context.Background(), testP2PID(t, 7), p2p.EnrolRequest{V: 1, Secret: link.Secret}, testFrom); r.OK || r.Reason != "used" {
		t.Fatalf("the link beside a used code, after a restart: %+v", r)
	}
	// Used invites stay used across a restart, for any other device.
	if r := ts.p2pEnrol(context.Background(), testP2PID(t, 8), p2p.EnrolRequest{V: 1, Secret: inv.Secret}, testFrom); r.OK || r.Reason != "used" {
		t.Fatalf("a used invite after a restart: %+v", r)
	}

	// Signing out still ends them, after a restart too.
	o = makeOffer(t, ts, kate, nil)
	link, _ = p2p.ParseInvite(o.Link)
	restartPairing(t, ts)
	ts.Auth.Logout(context.Background(), kate)
	if r := ts.p2pEnrol(context.Background(), testP2PID(t, 9), p2p.EnrolRequest{V: 1, Secret: link.Secret}, testFrom); r.OK {
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

// A code a TV showed, typed into the server before a restart, still finds the
// server after it: the new node is offered it again from what was saved.
func TestP2PJoinCodeOutlivesARestart(t *testing.T) {
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
	if saved := ts.DB.GetSetting(context.Background(), p2pPendingKey, ""); strings.Contains(saved, code) || !strings.Contains(saved, `"joins"`) {
		t.Fatalf("what is saved of the code: %s", saved)
	}
	ts.StopP2P()
	restartPairing(t, ts)
	ts.StartP2P(context.Background(), ts.h)
	node := ts.p2pNode()
	if node == nil {
		t.Fatal("no node after the restart")
	}
	find, _, _ := p2p.JoinKeysOf(code)
	if !node.JoinWaiting(find) {
		t.Fatal("the new node isn't waiting for the code typed in before the restart")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	res, err := p2p.Join(ctx, p2p.JoinConfig{Seed: p2pSeedFor(2), Code: code, Name: "Living room", Platform: "tvOS", Local: true,
		Bootstrap: []peer.AddrInfo{book}, Listen: p2pLoopback, Every: 100 * time.Millisecond})
	if err != nil || !res.OK || res.ServerID != node.ID() || len(res.Code) != 6 {
		t.Fatalf("the TV joining after the restart: %+v %v", res, err)
	}
	// Taken: no longer saved.
	ts.p2pSavePending()
	if saved := ts.DB.GetSetting(context.Background(), p2pPendingKey, ""); strings.Contains(saved, find) {
		t.Fatalf("a code its device took is still saved: %s", saved)
	}
}
