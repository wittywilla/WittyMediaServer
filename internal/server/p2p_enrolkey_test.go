// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"net/netip"
	"slices"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/p2p"
)

// An open invite's enrol key is on the list this server gives its relays (#614), and only while the
// invite is open: not once it is used, spent by the code beside it, or ended by a new one. An invite shown
// with that key on behalf of a device lists the device, never the enrol key.
func TestInviteEnrolKeyIsListedWhileOpen(t *testing.T) {
	var relayList func() []string
	ts, admin := p2pServer(t, func(c *p2p.NodeConfig) {
		relayList = c.DeviceIDs
	})
	node := ts.p2pNode()
	_, kate := ts.user(t, "kate", "adult", false, "")

	// Settings' own invite (an administrator's takes the password again).
	w := ts.do(t, "POST", "/api/p2p/invite", admin, map[string]any{"password": "correct-horse-1"})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	inv, err := p2p.ParseInvite(decode[map[string]any](t, w)["link"].(string))
	if err != nil {
		t.Fatal(err)
	}
	enrolID, _ := p2p.EnrolID(inv.Secret)
	if !slices.Contains(relayList(), enrolID) {
		t.Fatal("the open invite's enrol key isn't on the relay list")
	}

	// Shown with it, on behalf of a device: the device is listed, the enrol key is not, and it leaves the relay list.
	dev := testP2PID(t, 5)
	sig, _ := p2p.SignEnrolFor(p2pSeedFor(5), node.ID(), enrolID)
	from := netip.MustParseAddr("198.51.100.7")
	// A signature for another device is refused, and the invite isn't used up by it.
	if res := ts.p2pEnrol(context.Background(), enrolID, p2p.EnrolRequest{V: 1, Secret: inv.Secret, Device: testP2PID(t, 6), DeviceSig: sig}, from); res.OK {
		t.Fatal("an invite was shown on behalf of a device that didn't sign for it")
	}
	res := ts.p2pEnrol(context.Background(), enrolID, p2p.EnrolRequest{V: 1, Secret: inv.Secret, Name: "Laptop", Platform: "desktop", Device: dev, DeviceSig: sig}, from)
	if !res.OK || res.Code == "" {
		t.Fatalf("%+v", res)
	}
	if !ts.p2p.devices.Allowed(dev) || ts.p2p.devices.Allowed(enrolID) {
		t.Fatal("the device should be listed, and the enrol key not")
	}
	if slices.Contains(relayList(), enrolID) || !slices.Contains(relayList(), dev) {
		t.Fatalf("the relay list after the invite was used: %v", relayList())
	}

	// Connect a device's link: listed while open, gone once the code beside it is used.
	o := makeOffer(t, ts, kate, nil)
	link, _ := p2p.ParseInvite(o.Link)
	linkID, _ := p2p.EnrolID(link.Secret)
	if !slices.Contains(relayList(), linkID) {
		t.Fatal("Connect a device's link isn't on the relay list")
	}
	d := newCodeDevice(t, o.Code)
	_, a := d.hello(t, ts)
	if code, _ := d.answer(t, ts, a, d.proof); code != 200 {
		t.Fatal("the code")
	}
	if slices.Contains(relayList(), linkID) {
		t.Fatal("the link stayed on the relay list after the code beside it was used")
	}
	// ... and when newer codes end it, past the few a person may have open (#683): not at the first new one,
	// which may be a second tab while a device is using this one.
	o = makeOffer(t, ts, kate, nil)
	link, _ = p2p.ParseInvite(o.Link)
	first, _ := p2p.EnrolID(link.Secret)
	makeOffer(t, ts, kate, nil)
	if !slices.Contains(relayList(), first) {
		t.Fatal("a second code ended the first one's link")
	}
	for range connectPerPerson {
		makeOffer(t, ts, kate, nil)
	}
	if slices.Contains(relayList(), first) {
		t.Fatal("an ended link stayed on the relay list")
	}
}
