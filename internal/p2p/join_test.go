// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestJoinCodes(t *testing.T) {
	seen := map[string]bool{}
	for range 50 {
		code, err := NewJoinCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != JoinCodeLen+1 || code[JoinCodeLen/2] != '-' || strings.ContainsAny(code, "ILOU") {
			t.Fatalf("a code a person couldn't read across a room: %q", code)
		}
		clean, err := CleanJoinCode(code)
		if err != nil || FormatJoinCode(clean) != code {
			t.Fatalf("%q: %q %v", code, clean, err)
		}
		seen[code] = true
	}
	if len(seen) < 50 {
		t.Fatal("codes repeat")
	}
	// As a person types it: either case, no dash, and the letters that look like digits.
	for typed, want := range map[string]string{
		"k7qm-2xhd": "K7QM2XHD", " K7QM 2XHD ": "K7QM2XHD", "OIL0-1234": "01101234", "oooo-llll": "00001111",
	} {
		if got, err := CleanJoinCode(typed); err != nil || got != want {
			t.Errorf("%q: %q %v, want %q", typed, got, err, want)
		}
	}
	for _, bad := range []string{"", "K7QM", "K7QM-2XHD-9", "K7QM-2XHU", "K7QM-2XH!", "K7QM-2XHé"} {
		if _, err := CleanJoinCode(bad); err == nil {
			t.Errorf("%q was taken for a code", bad)
		}
	}
	// The two keys a code becomes are different, and another code's are others.
	a, b := deriveJoin("K7QM2XHD"), deriveJoin("K7QM2XHE")
	if string(a.find) == string(a.proof) || string(a.find) == string(b.find) || string(a.proof) == string(b.proof) {
		t.Fatal("the keys aren't independent")
	}
}

// joinWorld is a server in an address book of this machine's own, taking
// invites, and what a device needs to look for it.
func joinWorld(t *testing.T, invite string) (n *Node, devs *devList, cfg func(seed byte, code string) JoinConfig, enrolled *atomic.Int32) {
	t.Helper()
	book := newBook(t)
	devs = newDevList()
	enrolled = &atomic.Int32{}
	n = startNode(t, 1, devs, func(c *NodeConfig) {
		c.Layers.DHT = true
		c.Bootstrap = []peer.AddrInfo{book}
		c.Enrol = func(_ context.Context, id string, req EnrolRequest, _ netip.Addr) EnrolResponse {
			if req.Secret != invite {
				return EnrolResponse{Reason: "wrong", Error: "no"}
			}
			enrolled.Add(1)
			devs.set(id, true)
			return EnrolResponse{OK: true, Code: "123456", ServerName: "Home"}
		}
	})
	for i := 0; len(n.Host().Network().ConnsToPeer(book.ID)) == 0; i++ {
		if i > 200 {
			t.Fatal("the server never reached the address book")
		}
		time.Sleep(50 * time.Millisecond)
	}
	cfg = func(seed byte, code string) JoinConfig {
		return JoinConfig{Seed: testSeed(seed), Code: code, Name: "Living room", Platform: "tvOS", Local: true,
			Bootstrap: []peer.AddrInfo{book}, Listen: []string{"/ip4/127.0.0.1/udp/0/quic-v1", "/ip4/127.0.0.1/tcp/0"}, Every: 100 * time.Millisecond}
	}
	return n, devs, cfg, enrolled
}

func TestJoiningByACodeShownOnTheDevice(t *testing.T) {
	n, devs, cfg, enrolled := joinWorld(t, "the-invite-behind-it")
	code, _ := NewJoinCode()

	// The device shows its code and waits; nobody has typed it in yet.
	type outcome struct {
		res JoinResult
		err error
	}
	done := make(chan outcome, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	go func() {
		res, err := Join(ctx, cfg(2, code))
		done <- outcome{res, err}
	}()
	select {
	case got := <-done:
		t.Fatalf("joined before anyone typed the code in: %+v %v", got.res, got.err)
	case <-time.After(700 * time.Millisecond):
	}
	// Someone signed in types it, in their own way.
	if err := n.OfferJoin(strings.ToLower(strings.ReplaceAll(code, "-", " ")), "the-invite-behind-it"); err != nil {
		t.Fatal(err)
	}
	got := <-done
	if got.err != nil || !got.res.OK || got.res.ServerID != n.ID() || got.res.Code != "123456" || got.res.ServerName != "Home" || len(got.res.Addrs) == 0 {
		t.Fatalf("%+v %v", got.res, got.err)
	}
	if !devs.Allowed(testID(t, 2)) || enrolled.Load() != 1 {
		t.Fatal("the device isn't on the list")
	}
	// What came back is all a connection needs.
	c := startClient(t, 2, n, func(c *ClientConfig) {
		c.ServerID = got.res.ServerID
		c.Hints = got.res.Addrs
	})
	if status, _, err := get(t, c, "/hello"); err != nil || status != 200 {
		t.Fatalf("after joining: %d %v", status, err)
	}
	// A code works once: another device showing the same one finds nobody.
	short, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	if res, err := Join(short, cfg(3, code)); err == nil {
		t.Fatalf("the code worked twice: %+v", res)
	}
	if devs.Allowed(testID(t, 3)) || enrolled.Load() != 1 {
		t.Fatal("a second device got in on a used code")
	}
}

// Anyone can announce themselves under a code's lookup key: the address
// book's own computers see it. Such a one is shown a random number and
// nothing else, and the device goes on to find the real server.
func TestAStrangerUnderTheCodesKeyLearnsNothing(t *testing.T) {
	n, devs, cfg, _ := joinWorld(t, "the-invite-behind-it")
	code, _ := NewJoinCode()
	clean, _ := CleanJoinCode(code)
	keys := deriveJoin(clean)
	device := cfg(2, code)

	liar, err := NewHost(HostConfig{Seed: testSeed(9), Layers: Layers{DHT: true}, Local: true, Bootstrap: device.Bootstrap, Listen: device.Listen})
	if err != nil {
		t.Fatal(err)
	}
	defer liar.Close()
	var asked, toldMore atomic.Int32
	liar.SetStreamHandler(ProtoJoin, func(s network.Stream) {
		defer s.Close()
		var hello joinHello
		if readMessage(s, &hello) != nil {
			return
		}
		asked.Add(1)
		if hello.Find != hex.EncodeToString(keys.find) || hello.V != 1 {
			t.Errorf("an odd hello: %+v", hello)
		}
		// It knows the lookup key, and makes the rest up.
		_ = writeMessage(s, joinChallenge{OK: true, Nonce: base64.StdEncoding.EncodeToString(make([]byte, joinNonceLen)),
			Proof: base64.StdEncoding.EncodeToString(joinProof(keys.find, "server", nil, nil, liar.ID(), s.Conn().RemotePeer()))})
		var answer joinAnswer
		if readMessage(s, &answer) == nil {
			toldMore.Add(1)
		}
	})
	pctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	for i := 0; liar.DHT.Provide(pctx, joinCID(keys.find), true) != nil; i++ {
		if i > 100 {
			t.Fatal("the stranger never got into the address book")
		}
		time.Sleep(50 * time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	done := make(chan JoinResult, 1)
	go func() {
		res, _ := Join(ctx, device)
		done <- res
	}()
	for i := 0; asked.Load() == 0; i++ {
		if i > 400 {
			t.Fatal("the device never found the stranger, so this proves nothing")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := n.OfferJoin(code, "the-invite-behind-it"); err != nil {
		t.Fatal(err)
	}
	res := <-done
	if !res.OK || res.ServerID != n.ID() {
		t.Fatalf("the device didn't end up at the real server: %+v", res)
	}
	if toldMore.Load() != 0 {
		t.Fatal("the device answered a server that hadn't proved it knew the code")
	}
	if asked.Load() != 1 {
		t.Errorf("the stranger was asked %d times; once is enough to know", asked.Load())
	}
	if !devs.Allowed(testID(t, 2)) {
		t.Fatal("the device isn't on the list")
	}
}

// Someone who has the lookup key (it is public) and guesses at the code gets
// a few tries, and then the code is given up: the device shows another.
func TestGuessingAtACodeGivesItUp(t *testing.T) {
	n, devs, cfg, enrolled := joinWorld(t, "the-invite-behind-it")
	code, _ := NewJoinCode()
	clean, _ := CleanJoinCode(code)
	keys := deriveJoin(clean)
	if err := n.OfferJoin(code, "the-invite-behind-it"); err != nil {
		t.Fatal(err)
	}
	device := cfg(2, code)
	guesser, err := NewHost(HostConfig{Seed: testSeed(8), Local: true, Listen: device.Listen})
	if err != nil {
		t.Fatal(err)
	}
	defer guesser.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server, _ := peer.Decode(n.ID())
	if err := guesser.Connect(ctx, peer.AddrInfo{ID: server, Addrs: parseAddrs(n.Addrs(), server)}); err != nil {
		t.Fatal(err)
	}
	try := func() (joinChallenge, EnrolResponse) {
		s, err := guesser.NewStream(ctx, server, ProtoJoin)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		nonce := make([]byte, joinNonceLen)
		_ = writeMessage(s, joinHello{V: 1, Find: hex.EncodeToString(keys.find), Nonce: base64.StdEncoding.EncodeToString(nonce), Name: "x", Platform: "x"})
		var challenge joinChallenge
		if readMessage(s, &challenge) != nil || !challenge.OK {
			return challenge, EnrolResponse{}
		}
		// The server proved itself, as it must before a device says anything. The guess is wrong.
		_ = writeMessage(s, joinAnswer{Proof: base64.StdEncoding.EncodeToString(make([]byte, 32))})
		var res EnrolResponse
		_ = readMessage(s, &res)
		return challenge, res
	}
	for i := range maxJoinWrong {
		challenge, res := try()
		if !challenge.OK || res.OK || res.Reason != "wrong" {
			t.Fatalf("guess %d: %+v %+v", i+1, challenge, res)
		}
	}
	if challenge, _ := try(); challenge.OK || challenge.Reason != "wrong" {
		t.Fatalf("after %d wrong guesses the code is still waiting: %+v", maxJoinWrong, challenge)
	}
	if enrolled.Load() != 0 || devs.Allowed(guesser.ID().String()) {
		t.Fatal("a guesser got onto the list")
	}
	// The device that really shows it is out of luck too, and shows another.
	short, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	if res, err := Join(short, device); err == nil {
		t.Fatalf("a code that was given up still worked: %+v", res)
	}
	// One of a key that isn't waiting is told so, with no proof to chew on.
	s, err := guesser.NewStream(ctx, server, ProtoJoin)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_ = writeMessage(s, joinHello{V: 1, Find: strings.Repeat("ab", 32), Nonce: base64.StdEncoding.EncodeToString(make([]byte, joinNonceLen))})
	var challenge joinChallenge
	if err := readMessage(s, &challenge); err != nil || challenge.OK || challenge.Proof != "" {
		t.Fatalf("%+v %v", challenge, err)
	}
}

func TestACodeWaitsOnlySoLongAndOnlyWithAnAddressBook(t *testing.T) {
	n, _, cfg, enrolled := joinWorld(t, "the-invite-behind-it")
	code, _ := NewJoinCode()
	if err := n.OfferJoin("no code", "x"); err == nil {
		t.Fatal("anything is taken for a code")
	}
	if err := n.OfferJoin(code, "the-invite-behind-it"); err != nil {
		t.Fatal(err)
	}
	// Fifteen minutes later.
	n.joins.mu.Lock()
	for _, v := range n.joins.open {
		v.expires = time.Now().Add(-time.Second)
	}
	n.joins.mu.Unlock()
	short, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	if res, err := Join(short, cfg(2, code)); err == nil {
		t.Fatalf("a code that had run out still worked: %+v", res)
	}
	if enrolled.Load() != 0 {
		t.Fatal("a device got in on a code that had run out")
	}
	// No more than a handful wait at once.
	for i := 0; i < maxJoins; i++ {
		c, _ := NewJoinCode()
		if err := n.OfferJoin(c, "x"); err != nil {
			t.Fatalf("code %d: %v", i+1, err)
		}
	}
	extra, _ := NewJoinCode()
	if err := n.OfferJoin(extra, "x"); !errors.Is(err, ErrJoinBusy) {
		t.Fatalf("one too many: %v", err)
	}
	// With the address book turned off there is nowhere to be looked up.
	plain := startNode(t, 5, newDevList())
	if err := plain.OfferJoin(code, "x"); !errors.Is(err, ErrJoinNeedsDHT) {
		t.Fatalf("with no address book: %v", err)
	}
}
