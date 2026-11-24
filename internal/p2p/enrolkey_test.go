// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"context"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// A device that has never met the server, whose only way in is the server's own WittyRelay, joins with an
// invite (#614): the invite's enrol key is listed with the relay while the invite is open, the device
// shows the invite with it on behalf of its own key, and then gets through the relay with its own key.
// Nobody without the invite gets anywhere, and the enrol key stops being listed once the invite is used.
func TestInviteThroughYourOwnWittyRelay(t *testing.T) {
	const secret = "the-invite-secret-0001"
	f, relay := newFakeWittyRelay(t, "the-key-0123456789")
	devs := newDevList()
	var mu sync.Mutex
	open := true // the invite, until it is used
	var n *Node
	n = startNode(t, 1, devs, ownRelay(relay, "the-key-0123456789", devs), func(c *NodeConfig) {
		listed := c.DeviceIDs
		c.DeviceIDs = func() []string {
			ids := listed()
			mu.Lock()
			defer mu.Unlock()
			if open {
				id, _ := EnrolID(secret)
				ids = append(ids, id)
			}
			return ids
		}
		// As the server's p2pEnrol: who may be listed is checked before the invite is used.
		c.Enrol = func(_ context.Context, id string, req EnrolRequest, _ netip.Addr) EnrolResponse {
			listID, err := CheckEnrolFor(req, id, n.ID())
			mu.Lock()
			ok := open && req.Secret == secret
			if err == nil && ok {
				open = false
			}
			mu.Unlock()
			if err != nil || !ok {
				return EnrolResponse{Reason: "wrong", Error: "no"}
			}
			devs.set(listID, true)
			n.DevicesChanged()
			return EnrolResponse{OK: true, Code: "123456"}
		}
	})
	waitOwnRelay(t, n, "ok")
	enrolID, _ := EnrolID(secret)
	until := func(what string, ok func() bool) {
		t.Helper()
		deadline := time.Now().Add(patience)
		for !ok() {
			if time.Now().After(deadline) {
				t.Fatal(what)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	until("the relay never heard of the open invite's enrol key", func() bool { return f.devices(n.Host().ID())[peerOf(t, enrolID)] })

	relayOnly := func(c *ClientConfig) { c.Hints = []string{viaRelay(relay)}; c.Layers.Relay = true }
	// The device's own key isn't listed: it gets nowhere through the relay by itself.
	dev := startClient(t, 5, n, relayOnly)
	time.Sleep(time.Second)
	if dev.State().Connected() {
		t.Fatalf("an unlisted device got through the relay: %+v", dev.State())
	}
	cfg := ClientConfig{Seed: testSeed(5), ServerID: n.ID(), Local: true, UpgradeWait: 400 * time.Millisecond,
		Listen: []string{"/ip4/127.0.0.1/udp/0/quic-v1", "/ip4/127.0.0.1/tcp/0"}}
	relayOnly(&cfg)

	// A wrong secret has a different enrol key, which isn't listed: nothing.
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	if res, err := EnrolThroughRelay(ctx, cfg, "not-the-invite-secret-1", "Stranger", "desktop"); err == nil && res.OK {
		t.Fatal("a wrong secret got a device listed")
	}
	cancel()

	// The invite: shown with its enrol key, on behalf of the device's own key.
	ctx, cancel = context.WithTimeout(context.Background(), patience)
	defer cancel()
	res, err := EnrolThroughRelay(ctx, cfg, secret, "Laptop", "desktop")
	if err != nil || !res.OK || res.Code != "123456" {
		t.Fatalf("enrolling through the relay: %+v %v", res, err)
	}
	if !devs.Allowed(testID(t, 5)) || devs.Allowed(enrolID) {
		t.Fatal("the device's own key should be listed, and the enrol key not")
	}
	until("the relay still lists the used invite's enrol key", func() bool { return !f.devices(n.Host().ID())[peerOf(t, enrolID)] })
	// ... and the device gets through with its own key.
	dev.NetworkChanged()
	s := waitPathWith(t, dev, func(s State) bool { return s.Connected() })
	if s.Path != PathRelayed {
		t.Fatalf("expected the relay: %+v", s)
	}
	if code, body, err := get(t, dev, "/hello"); err != nil || code != 200 || !strings.Contains(body, "relayed=true") {
		t.Fatalf("%d %q %v", code, body, err)
	}

	// The invite is spent: using it again, even with a valid signature, lists nobody.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel2()
	cfg.Seed = testSeed(6)
	if res, err := EnrolThroughRelay(ctx2, cfg, secret, "Again", "desktop"); err == nil && res.OK {
		t.Fatal("a used invite listed another device")
	}
	if devs.Allowed(testID(t, 6)) {
		t.Fatal("a used invite listed another device")
	}
}

// Only the invite's own enrol key may name another device, and only with that device's signature over
// this server and that enrol key.
func TestCheckEnrolFor(t *testing.T) {
	const secret = "the-invite-secret-0001"
	server := testID(t, 1)
	enrolID, _ := EnrolID(secret)
	dev := testID(t, 5)
	sig, err := SignEnrolFor(testSeed(5), server, enrolID)
	if err != nil {
		t.Fatal(err)
	}
	good := EnrolRequest{V: 1, Secret: secret, Device: dev, DeviceSig: sig}
	if id, err := CheckEnrolFor(good, enrolID, server); err != nil || id != dev {
		t.Fatalf("the right request: %q %v", id, err)
	}
	// Plain enrolment: the key that shows the invite is the one listed.
	if id, err := CheckEnrolFor(EnrolRequest{V: 1, Secret: secret}, dev, server); err != nil || id != dev {
		t.Fatalf("plain: %q %v", id, err)
	}
	otherSig, _ := SignEnrolFor(testSeed(5), testID(t, 9), enrolID)
	bad := map[string]struct {
		req  EnrolRequest
		conn string
	}{
		"from a key that isn't the invite's": {good, testID(t, 7)},
		"another secret":                     {EnrolRequest{V: 1, Secret: "another-secret-000000001", Device: dev, DeviceSig: sig}, enrolID},
		"signed by another key":              {EnrolRequest{V: 1, Secret: secret, Device: testID(t, 8), DeviceSig: sig}, enrolID},
		"signed for another server":          {EnrolRequest{V: 1, Secret: secret, Device: dev, DeviceSig: otherSig}, enrolID},
		"no signature":                       {EnrolRequest{V: 1, Secret: secret, Device: dev}, enrolID},
		"not an ID":                          {EnrolRequest{V: 1, Secret: secret, Device: "nope", DeviceSig: sig}, enrolID},
		"the enrol key itself":               {EnrolRequest{V: 1, Secret: secret, Device: enrolID, DeviceSig: sig}, enrolID},
	}
	for what, c := range bad {
		if id, err := CheckEnrolFor(c.req, c.conn, server); err == nil {
			t.Errorf("%s: listed %q", what, id)
		}
	}
}

func peerOf(t *testing.T, id string) peer.ID {
	t.Helper()
	p, err := peer.Decode(id)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
