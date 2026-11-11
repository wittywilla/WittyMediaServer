// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"bytes"
	"context"
	"errors"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

func testSeed(b byte) []byte { return bytes.Repeat([]byte{b}, SeedSize) }

func testID(t *testing.T, b byte) string {
	t.Helper()
	id, err := IDFromSeed(testSeed(b))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSeedsAndIDs(t *testing.T) {
	seed, err := NewSeed()
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeSeed(EncodeSeed(seed))
	if err != nil || !bytes.Equal(back, seed) {
		t.Fatal("a seed did not survive being written down")
	}
	a, _ := IDFromSeed(seed)
	b, _ := IDFromSeed(seed)
	other, _ := IDFromSeed(testSeed(1))
	if a != b || a == other || !strings.HasPrefix(a, "12D3KooW") {
		t.Fatalf("IDs: %q %q %q", a, b, other)
	}
	for _, bad := range []string{"", "hello", "QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN" /* an RSA key's hash */, a + "x"} {
		if _, err := CheckID(bad); err == nil {
			t.Errorf("%q was accepted as a connection ID", bad)
		}
	}
	if got, err := CheckID(" " + a + "\n"); err != nil || got != a {
		t.Errorf("a valid ID with spaces around it: %q, %v", got, err)
	}
	if _, err := DecodeSeed("c2hvcnQ="); err == nil {
		t.Error("a short seed was accepted")
	}
}

func TestInviteLinks(t *testing.T) {
	id := testID(t, 7)
	in := Invite{ServerID: id, Secret: "abcdefghijklmnopqrstuv", Name: "Home & garden", Addrs: []string{"/ip4/203.0.113.4/udp/4100/quic-v1", "/ip4/203.0.113.4/tcp/4100"}}
	link := in.String()
	if !strings.HasPrefix(link, "wittymedia://p2p/"+id+"?") {
		t.Fatalf("link: %s", link)
	}
	back, err := ParseInvite(link)
	if err != nil || back.ServerID != id || back.Secret != in.Secret || back.Name != in.Name || len(back.Addrs) != 2 {
		t.Fatalf("read back: %+v, %v", back, err)
	}
	for _, bad := range []string{
		"https://p2p/" + id + "?s=abcdefghijklmnopqrstuv",
		"wittymedia://other/" + id + "?s=abcdefghijklmnopqrstuv",
		"wittymedia://p2p/nonsense?s=abcdefghijklmnopqrstuv",
		"wittymedia://p2p/" + id + "?s=short",
		"wittymedia://p2p/" + id,
	} {
		if _, err := ParseInvite(bad); err == nil {
			t.Errorf("%q was read as an invite", bad)
		}
	}
	// Addresses that can't be read are dropped, and no more than a few kept.
	many := "wittymedia://p2p/" + id + "?s=abcdefghijklmnopqrstuv&a=rubbish" + strings.Repeat("&a=/ip4/203.0.113.4/tcp/1", 20)
	got, err := ParseInvite(many)
	if err != nil || len(got.Addrs) != maxInviteAddrs {
		t.Fatalf("%d addresses kept, %v", len(got.Addrs), err)
	}
}

// The address a connection is counted under must never be one that the server
// takes for this computer or the home network, whatever it really came from.
func TestPresentedIPIsNeverLocal(t *testing.T) {
	id, _ := peer.Decode(testID(t, 3))
	other, _ := peer.Decode(testID(t, 4))
	local := []string{"127.0.0.1", "::1", "192.168.1.20", "10.0.0.5", "172.16.3.3", "169.254.1.1", "fe80::1", "fd00::5", "100.64.1.1", "::ffff:127.0.0.1", "::ffff:192.168.1.1", "0.0.0.0", ""}
	for _, s := range local {
		var a netip.Addr
		if s != "" {
			a = netip.MustParseAddr(s)
		}
		for _, relayed := range []bool{false, true} {
			for _, known := range []bool{false, true} {
				got := presentedIP(a, relayed, known, id)
				if !got.IsValid() || got.IsLoopback() || got.IsPrivate() || got.IsLinkLocalUnicast() || got.IsUnspecified() ||
					netip.MustParsePrefix("100.64.0.0/10").Contains(got) {
					t.Errorf("from %q (relayed %v, known %v) the server would see %v", s, relayed, known, got)
				}
			}
		}
	}
	pub := netip.MustParseAddr("198.51.100.7")
	if got := presentedIP(pub, false, true, id); got != pub {
		t.Errorf("a direct public address should be passed on, got %v", got)
	}
	// Devices on the list are told apart through a relay; strangers are not.
	if presentedIP(pub, true, true, id) == presentedIP(pub, true, true, other) {
		t.Error("two listed devices behind one relay share an address")
	}
	if presentedIP(pub, true, false, id) != presentedIP(pub, true, false, other) || presentedIP(pub, true, false, id) != pub {
		t.Error("strangers behind one relay must be counted together, under the relay's address")
	}
	if presentedIP(netip.MustParseAddr("192.168.1.9"), false, false, id) != presentedIP(netip.MustParseAddr("10.1.1.1"), false, false, other) {
		t.Error("strangers on private networks must be counted together")
	}
}

func TestOnlyAddressesAnAppCanUseAreHandedOut(t *testing.T) {
	relay := "/p2p/" + testID(t, 5) + "/p2p-circuit"
	for a, want := range map[string]bool{
		"/ip4/203.0.113.4/udp/4100/quic-v1":                      true,
		"/ip4/203.0.113.4/tcp/4100":                              true,
		"/ip6/2001:db8::4/udp/4100/quic-v1":                      true,
		"/ip4/203.0.113.9/tcp/4001" + relay:                      true,
		"/ip4/203.0.113.9/udp/4001/quic-v1" + relay:              true,
		"/ip4/203.0.113.9/udp/4001/quic-v1/webtransport" + relay: false,
		"/ip4/203.0.113.9/udp/4001/webrtc-direct" + relay:        false,
		"/ip4/203.0.113.9/tcp/443/tls/ws" + relay:                false,
		"/ip4/203.0.113.9/tcp/8080/ws" + relay:                   false,
		"/ip4/203.0.113.9/udp/4001/quic" + relay:                 false,
		"/dns4/relay.example.org/tcp/443/wss" + relay:            false,
	} {
		m, err := ma.NewMultiaddr(a)
		if err != nil {
			t.Fatalf("%s: %v", a, err)
		}
		if spoken(m) != want {
			t.Errorf("spoken(%s) = %v", a, !want)
		}
	}
}

func TestWindow(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	w := newWindow(3, time.Minute, 2)
	w.now = func() time.Time { return now }
	for i := range 3 {
		if !w.allow("a") {
			t.Fatalf("try %d refused", i+1)
		}
	}
	if w.allow("a") {
		t.Fatal("a fourth try in the same minute was allowed")
	}
	if !w.allow("b") {
		t.Fatal("another key has its own count")
	}
	if w.allow("c") {
		t.Fatal("a full table must refuse new keys, not forget old ones")
	}
	now = now.Add(61 * time.Second)
	if !w.allow("c") || !w.allow("a") {
		t.Fatal("after the window passed, tries are allowed again")
	}
}

func TestBucketPaces(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	var slept time.Duration
	b := newBucket(800_000) // 100 kB/s
	b.now = func() time.Time { return now }
	b.sleep = func(d time.Duration) { slept += d; now = now.Add(d) }
	for range 30 {
		b.wait(10_000)
	}
	// 300 kB at 100 kB/s, less the quarter second it starts with.
	if slept < 2500*time.Millisecond || slept > 3*time.Second {
		t.Fatalf("300 kB at 100 kB/s took %v", slept)
	}
	if newBucket(0) != nil {
		t.Fatal("no limit means no bucket")
	}
}

func TestBackoff(t *testing.T) {
	b := backoff{jitter: func() float64 { return 0.5 }} // the middle: no shift
	var got []time.Duration
	for range 9 {
		got = append(got, b.next())
	}
	want := []time.Duration{1, 2, 4, 8, 16, 32, 60, 60, 60}
	for i := range want {
		if got[i] != want[i]*time.Second {
			t.Fatalf("waits = %v", got)
		}
	}
	b.reset()
	if b.next() != time.Second {
		t.Fatal("after a reset the wait starts over")
	}
	lo, hi := backoff{jitter: func() float64 { return 0 }}, backoff{jitter: func() float64 { return 1 }}
	if l, h := lo.next(), hi.next(); l != 800*time.Millisecond || h != 1200*time.Millisecond {
		t.Fatalf("jitter range %v..%v", l, h)
	}
}

// ---------------------------------------------------------------- the keeper

type stepClock struct {
	mu     sync.Mutex
	t      time.Time
	asked  chan time.Duration
	timers []chan time.Time
}

func newStepClock() *stepClock {
	return &stepClock{t: time.Unix(1_800_000_000, 0), asked: make(chan time.Duration, 32)}
}
func (c *stepClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *stepClock) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	c.mu.Lock()
	c.timers = append(c.timers, ch)
	c.mu.Unlock()
	c.asked <- d
	return ch
}

// fire lets the newest timer run out: the one the keeper is waiting on (older
// ones belong to waits it has already left).
func (c *stepClock) fire(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	now := c.t
	ch := c.timers[len(c.timers)-1]
	c.mu.Unlock()
	ch <- now
}

type fakeLink struct {
	mu       sync.Mutex
	state    State
	err      error
	connects int
	tidies   int
	checks   int
	onCheck  func()
	result   State // what a successful connect leads to
}

func (f *fakeLink) current() State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}
func (f *fakeLink) connect(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connects++
	if f.err != nil {
		return f.err
	}
	f.state = f.result
	return nil
}
func (f *fakeLink) tidy() {
	f.mu.Lock()
	f.tidies++
	f.mu.Unlock()
}
func (f *fakeLink) check(context.Context) {
	f.mu.Lock()
	f.checks++
	fn := f.onCheck
	f.mu.Unlock()
	if fn != nil {
		fn()
	}
}
func (f *fakeLink) set(s State, err error) {
	f.mu.Lock()
	f.state, f.err = s, err
	f.mu.Unlock()
}

func expectState(t *testing.T, states <-chan State, path string) State {
	t.Helper()
	select {
	case s := <-states:
		if s.Path != path {
			t.Fatalf("state %q, want %q (%+v)", s.Path, path, s)
		}
		return s
	case <-time.After(5 * time.Second):
		t.Fatalf("no change to %q", path)
	}
	return State{}
}

func expectWait(t *testing.T, c *stepClock, want time.Duration) {
	t.Helper()
	select {
	case d := <-c.asked:
		if d != want {
			t.Fatalf("waits %v, want %v", d, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("never waited %v", want)
	}
}

func TestKeeperBacksOffReconnectsAndUpgrades(t *testing.T) {
	clock := newStepClock()
	link := &fakeLink{err: errors.New("no route"), result: State{Path: PathRelayed, Relay: "r", Community: true}}
	states := make(chan State, 32)
	k := newKeeper(link, clock, func(s State) { states <- s })
	k.bo.jitter = func() float64 { return 0.5 }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { k.run(ctx); close(done) }()

	// Three failures: each wait twice the last, and the state says when the next try is.
	for i, want := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second} {
		s := expectState(t, states, PathOffline)
		if s.Error != "no route" || s.Attempts != i+1 || s.NextTry != clock.Now().Add(want).Unix() {
			t.Fatalf("offline state %d: %+v", i, s)
		}
		expectWait(t, clock, want)
		if i == 2 {
			link.set(State{}, nil) // the server is back
		}
		clock.fire(want)
		expectState(t, states, PathConnecting)
	}
	s := expectState(t, states, PathRelayed)
	if !s.Community || s.Since == 0 || s.Error != "" {
		t.Fatalf("relayed state: %+v", s)
	}
	expectWait(t, clock, tidyEvery)

	// A direct connection appears: the path is upgraded without a reconnect.
	link.set(State{Path: PathDirect, Addr: "/ip4/203.0.113.4/udp/1/quic-v1"}, nil)
	k.noteChanged()
	expectState(t, states, PathDirect)
	expectWait(t, clock, tidyEvery)
	if link.connects != 4 {
		t.Fatalf("an upgrade must not reconnect: %d connects", link.connects)
	}

	// Now and then the standing connection is tidied.
	clock.fire(tidyEvery)
	expectWait(t, clock, tidyEvery)
	if link.tidies != 1 {
		t.Fatalf("tidied %d times", link.tidies)
	}

	// The network changes and the connection turns out dead: checked, dropped,
	// and tried again at once, with the back-off starting over.
	link.mu.Lock()
	link.onCheck = func() { link.set(State{}, errors.New("no route")) }
	link.mu.Unlock()
	k.kick()
	expectState(t, states, PathConnecting)
	s = expectState(t, states, PathOffline)
	if s.Attempts != 1 {
		t.Fatalf("after a working spell the count starts over: %+v", s)
	}
	expectWait(t, clock, time.Second)
	// Another network change while waiting: no waiting out the timer.
	link.set(State{}, nil)
	k.kick()
	expectState(t, states, PathConnecting)
	expectState(t, states, PathRelayed)

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the keeper did not stop with its context")
	}
}

func TestKeeperWait(t *testing.T) {
	link := &fakeLink{err: errors.New("down")}
	k := newKeeper(link, newStepClock(), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := k.wait(ctx, State.Connected); err == nil {
		t.Fatal("waited for a connection that never came, and said it had")
	}
	go func() {
		time.Sleep(20 * time.Millisecond)
		k.set(State{Path: PathDirect})
	}()
	s, err := k.wait(context.Background(), State.Connected)
	if err != nil || s.Path != PathDirect {
		t.Fatalf("%+v, %v", s, err)
	}
}

func TestUsablePaths(t *testing.T) {
	for s, want := range map[State]bool{
		{Path: PathDirect}:                   true,
		{Path: PathLocal}:                    true,
		{Path: PathRelayed, Community: true}: true,
		{Path: PathRelayed}:                  false, // a public relay: setup only
		{Path: PathOffline}:                  false,
		{Path: PathConnecting}:               false,
	} {
		if usable(s) != want {
			t.Errorf("usable(%+v) = %v", s, !want)
		}
	}
}
