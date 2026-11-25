// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"
)

// A device an invite has just put on the list is on the relay's list before the
// server answers it (#683): RegisterNow returns once the relay has heard, with
// no polling, and says so.
func TestRegisterNowWaitsForTheRelay(t *testing.T) {
	f, relay := newFakeWittyRelay(t, "the-key-0123456789")
	devs := newDevList(testID(t, 2))
	n := startNode(t, 1, devs, ownRelay(relay, "the-key-0123456789", devs))
	waitOwnRelay(t, n, "ok")

	devs.set(testID(t, 3), true)
	ctx, cancel := context.WithTimeout(context.Background(), patience)
	defer cancel()
	if !n.RegisterNow(ctx) {
		t.Fatal("RegisterNow said the relay didn't take the list")
	}
	if !f.devices(n.Host().ID())[mustPeer(t, 3)] {
		t.Fatal("RegisterNow returned before the relay had the new device")
	}

	// A change that leaves the list as the relay has it costs the relay nothing: it allows only so many
	// registrations a minute from one address.
	f.mu.Lock()
	before := f.regs
	f.mu.Unlock()
	n.DevicesChanged()
	time.Sleep(registerSettle + time.Second)
	f.mu.Lock()
	after := f.regs
	f.mu.Unlock()
	if after != before {
		t.Fatalf("an unchanged list was registered again: %d registrations, then %d", before, after)
	}
	// RegisterNow with nothing new answers at once.
	if !n.RegisterNow(ctx) {
		t.Fatal("RegisterNow with an unchanged list")
	}
}

// A relay that refuses the list, and a server with no WittyRelay at all.
func TestRegisterNowSaysNo(t *testing.T) {
	_, relay := newFakeWittyRelay(t, "the-key-0123456789")
	devs := newDevList(testID(t, 2))
	n := startNode(t, 1, devs, ownRelay(relay, "not-the-key-0000000", devs))
	waitOwnRelay(t, n, "refused")
	ctx, cancel := context.WithTimeout(context.Background(), patience)
	defer cancel()
	if n.RegisterNow(ctx) {
		t.Fatal("RegisterNow said yes for a relay that refused the server")
	}
	// Waiting for nothing: ctx ends, it says no, and nothing is left waiting.
	short, cancel2 := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel2()
	_ = n.RegisterNow(short)

	plain := startNode(t, 4, newDevList())
	if !plain.RegisterNow(context.Background()) {
		t.Fatal("a server with no WittyRelay has nothing to wait for")
	}
}

// A try stuck on an address that never answers (here: one that takes the
// connection and says nothing) gives up at once when the network changes, or
// the server lists this device, rather than after its twenty seconds.
func TestAKickCutsATryShort(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		var held []net.Conn
		for {
			c, err := ln.Accept()
			if err != nil {
				for _, c := range held {
					c.Close()
				}
				return
			}
			held = append(held, c)
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	c, err := NewClient(ClientConfig{Seed: testSeed(2), ServerID: testID(t, 1), Local: true,
		Hints:  []string{"/ip4/127.0.0.1/tcp/" + strconv.Itoa(port)},
		Listen: []string{"/ip4/127.0.0.1/tcp/0"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	time.Sleep(500 * time.Millisecond) // the try is under way, waiting on the silent address
	start := time.Now()
	c.NetworkChanged()
	deadline := time.Now().Add(10 * time.Second)
	for c.State().Error != errLookAgain.Error() {
		if time.Now().After(deadline) {
			t.Fatalf("the try under way didn't give up on a kick: %+v", c.State())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("the try gave up only after %v", d)
	}
}
