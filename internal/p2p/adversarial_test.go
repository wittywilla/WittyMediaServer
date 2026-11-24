// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/proto"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/libp2p/go-libp2p/p2p/security/insecure"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	ma "github.com/multiformats/go-multiaddr"
)

// Someone in the way, or someone pretending: each test here is one of the
// things docs/P2P.md says a stranger, a relay or a dishonest peer can't do,
// tried for real on this machine.

// marker is what the server sends and the app asks for, to look for in what
// passed through the middle. It must never be seen there.
const marker = "plaintext-marker-6f1d0c2a9b"

func markedHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/secret", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Marker") != marker+"-asked" {
			http.Error(w, "the question arrived changed", 400)
			return
		}
		_, _ = io.WriteString(w, strings.Repeat(marker+"\n", 2000))
	})
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "big.bin", time.Time{}, bytes.NewReader(bigBody))
	})
	return mux
}

func askSecret(c *Client) (int, string, error) {
	req, _ := http.NewRequest("GET", "http://server/secret", nil)
	req.Header.Set("X-Marker", marker+"-asked")
	ctx, cancel := context.WithTimeout(context.Background(), patience)
	defer cancel()
	res, err := c.Transport().RoundTrip(req.WithContext(ctx))
	if err != nil {
		return 0, "", err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	return res.StatusCode, string(b), err
}

// tcpOnly is a node that speaks only TCP, on this machine, so a proxy can stand in the way.
func tcpOnly(c *NodeConfig) { c.Listen = []string{"/ip4/127.0.0.1/tcp/0"} }

func tcpAddr(t *testing.T, n *Node) string {
	t.Helper()
	for _, a := range n.Host().Network().ListenAddresses() {
		if port, err := a.ValueForProtocol(ma.P_TCP); err == nil {
			return "127.0.0.1:" + port
		}
	}
	t.Fatal("the node listens on no TCP port")
	return ""
}

// middle is a TCP proxy that keeps everything it passes on and, if asked,
// changes one byte on the way back, once it has passed so many.
type middle struct {
	ln       net.Listener
	to       string
	flipAt   int64 // 0: change nothing
	mu       sync.Mutex
	seen     bytes.Buffer
	back     atomic.Int64
	flipped  atomic.Bool
	accepted atomic.Int32
}

func newMiddle(t *testing.T, to string, flipAt int64) *middle {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	m := &middle{ln: ln, to: to, flipAt: flipAt}
	t.Cleanup(func() { ln.Close() })
	go m.serve()
	return m
}

func (m *middle) addr() string {
	return "/ip4/127.0.0.1/tcp/" + strings.Split(m.ln.Addr().String(), ":")[1]
}

func (m *middle) serve() {
	for {
		in, err := m.ln.Accept()
		if err != nil {
			return
		}
		m.accepted.Add(1)
		out, err := net.Dial("tcp4", m.to)
		if err != nil {
			in.Close()
			continue
		}
		go m.copy(out, in, false)
		go m.copy(in, out, true)
	}
}

func (m *middle) copy(dst, src net.Conn, back bool) {
	defer dst.Close()
	defer src.Close()
	buf := make([]byte, 32<<10)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			b := buf[:n]
			m.mu.Lock()
			m.seen.Write(b)
			m.mu.Unlock()
			if back && m.flipAt > 0 {
				start := m.back.Add(int64(n)) - int64(n)
				if i := m.flipAt - start; i >= 0 && i < int64(n) && m.flipped.CompareAndSwap(false, true) {
					b[i] ^= 0x40
				}
			}
			if _, werr := dst.Write(b); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (m *middle) saw(s string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return bytes.Contains(m.seen.Bytes(), []byte(s))
}

func viaMiddle(m *middle) clientOpt {
	return func(c *ClientConfig) {
		c.Hints = []string{m.addr()}
		c.Listen = []string{"/ip4/127.0.0.1/tcp/0"}
	}
}

// Whatever carries the connection sees only ciphertext: not the question, not the answer.
func TestWhatIsInTheWaySeesOnlyCiphertext(t *testing.T) {
	devs := newDevList(testID(t, 2))
	n := startNode(t, 1, devs, tcpOnly, func(c *NodeConfig) { c.Handler = markedHandler() })
	m := newMiddle(t, tcpAddr(t, n), 0)
	c := startClient(t, 2, n, viaMiddle(m))
	code, body, err := askSecret(c)
	if err != nil || code != 200 || !strings.Contains(body, marker) {
		t.Fatalf("through the proxy: %d %v", code, err)
	}
	if m.accepted.Load() == 0 {
		t.Fatal("the connection didn't go through the proxy, so this proves nothing")
	}
	if m.saw(marker) {
		t.Fatal("the plaintext was readable on the way")
	}
}

// A byte changed on the way ends the connection; it never changes what arrives.
func TestAChangedByteBreaksTheConnectionNotTheContent(t *testing.T) {
	devs := newDevList(testID(t, 2))
	n := startNode(t, 1, devs, tcpOnly, func(c *NodeConfig) { c.Handler = markedHandler() })
	// Well past the handshake and the request: in the middle of the answer.
	m := newMiddle(t, tcpAddr(t, n), 400<<10)
	c := startClient(t, 2, n, viaMiddle(m))
	req, _ := http.NewRequest("GET", "http://server/big", nil)
	ctx, cancel := context.WithTimeout(context.Background(), patience)
	defer cancel()
	res, err := c.Transport().RoundTrip(req.WithContext(ctx))
	if err == nil {
		b, rerr := io.ReadAll(res.Body)
		res.Body.Close()
		if rerr == nil && bytes.Equal(b, bigBody) && !m.flipped.Load() {
			t.Fatal("nothing was changed on the way, so this proves nothing")
		}
		if rerr == nil {
			t.Fatalf("a changed answer was delivered as if it were whole (%d bytes, same as sent: %v)", len(b), bytes.Equal(b, bigBody))
		}
	}
	if !m.flipped.Load() {
		t.Fatal("the proxy never got as far as changing a byte")
	}
}

// The app dials the server's ID. Another key at the server's address, or at
// any address the app was given, is refused before anything is said.
func TestAnotherKeyAtTheServersAddressIsRefused(t *testing.T) {
	devs := newDevList(testID(t, 2))
	real := startNode(t, 1, devs)
	var asked atomic.Int32
	impostor := startNode(t, 7, newDevList(testID(t, 2)), func(c *NodeConfig) {
		c.DevicesOnly = false
		c.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { asked.Add(1) })
	})
	h, err := NewHost(HostConfig{Seed: testSeed(2), Local: true, Listen: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	want, _ := peer.Decode(real.ID())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err = h.Connect(ctx, peer.AddrInfo{ID: want, Addrs: parseAddrs(impostor.Addrs(), want)})
	if err == nil {
		t.Fatal("connected to another key as if it were the server")
	}
	// The app's own end, given only the impostor's addresses, never gets as far as a request.
	c := startClient(t, 2, real, func(c *ClientConfig) { c.Hints = impostor.Addrs() })
	short, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	if s, err := c.keeper.wait(short, State.Connected); err == nil {
		t.Fatalf("the app thinks it is connected: %+v", s)
	}
	if asked.Load() != 0 {
		t.Fatal("the impostor was sent a request")
	}
}

// Only Noise and QUIC's TLS are spoken: a peer that offers to talk in the
// clear (libp2p's plaintext security, for tests) gets no connection at all.
func TestNoConnectionWithoutEncryption(t *testing.T) {
	n := startNode(t, 1, newDevList(), tcpOnly, func(c *NodeConfig) { c.DevicesOnly = false })
	plain, err := libp2p.New(
		libp2p.Identity(mustKey(t, 3)),
		libp2p.NoListenAddrs,
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Security(insecure.ID, insecure.NewWithIdentity),
		libp2p.DisableRelay(),
		libp2p.DisableMetrics(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer plain.Close()
	server, _ := peer.Decode(n.ID())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := plain.Connect(ctx, peer.AddrInfo{ID: server, Addrs: parseAddrs(n.Addrs(), server)}); err == nil {
		t.Fatal("a peer that speaks only plaintext was let in")
	}
	if len(n.Host().Network().ConnsToPeer(plain.ID())) != 0 {
		t.Fatal("the server holds a connection with no encryption")
	}
}

func mustKey(t *testing.T, b byte) crypto.PrivKey {
	t.Helper()
	k, err := keyFromSeed(testSeed(b))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// ---------------------------------------------------------------- relays

// tapHost is the host a relay is given, so the test can read, or change, all
// it passes on: each relayed connection is copied between a hop stream (from
// the device) and a stop stream (to the server), and these are those streams.
type tapHost struct {
	host.Host
	tap *tap
}

type tap struct {
	mu      sync.Mutex
	seen    bytes.Buffer
	flipAt  int64
	written atomic.Int64
	flipped atomic.Bool
	streams atomic.Int32
}

type tapStream struct {
	network.Stream
	tap *tap
}

func (s *tapStream) Write(b []byte) (int, error) {
	s.tap.mu.Lock()
	s.tap.seen.Write(b)
	s.tap.mu.Unlock()
	if s.tap.flipAt > 0 {
		start := s.tap.written.Add(int64(len(b))) - int64(len(b))
		if i := s.tap.flipAt - start; i >= 0 && i < int64(len(b)) && s.tap.flipped.CompareAndSwap(false, true) {
			c := append([]byte(nil), b...)
			c[i] ^= 0x01
			return s.Stream.Write(c)
		}
	}
	return s.Stream.Write(b)
}

func (h *tapHost) SetStreamHandler(pid protocol.ID, handler network.StreamHandler) {
	if pid != proto.ProtoIDv2Hop {
		h.Host.SetStreamHandler(pid, handler)
		return
	}
	h.Host.SetStreamHandler(pid, func(s network.Stream) {
		h.tap.streams.Add(1)
		handler(&tapStream{Stream: s, tap: h.tap})
	})
}

func (h *tapHost) NewStream(ctx context.Context, p peer.ID, pids ...protocol.ID) (network.Stream, error) {
	s, err := h.Host.NewStream(ctx, p, pids...)
	if err != nil {
		return nil, err
	}
	for _, pid := range pids {
		if pid == proto.ProtoIDv2Stop {
			h.tap.streams.Add(1)
			return &tapStream{Stream: s, tap: h.tap}, nil
		}
	}
	return s, nil
}

// tappedRelay is a community relay on this machine whose every relayed byte the test sees.
func tappedRelay(t *testing.T, flipAt int64) (*tap, string) {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"), libp2p.DisableMetrics(), libp2p.ForceReachabilityPublic(), libp2p.DisableRelay())
	if err != nil {
		t.Fatal(err)
	}
	tp := &tap{flipAt: flipAt}
	r, err := relayv2.New(&tapHost{Host: h, tap: tp}, relayv2.WithInfiniteLimits())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close(); h.Close() })
	h.SetStreamHandler(ProtoRelay, func(s network.Stream) { s.Close() })
	return tp, h.Addrs()[0].String() + "/p2p/" + h.ID().String()
}

func throughRelay(relay string) clientOpt {
	return func(c *ClientConfig) {
		c.Hints = []string{viaRelay(relay)}
		c.Layers.Relay = true
	}
}

func TestARelaySeesOnlyCiphertext(t *testing.T) {
	tp, relay := tappedRelay(t, 0)
	devs := newDevList(testID(t, 2))
	n := startNode(t, 1, devs, behindRelay(relay), func(c *NodeConfig) { c.Handler = markedHandler() })
	c := startClient(t, 2, n, throughRelay(relay))
	waitPath(t, c, PathRelayed)
	code, body, err := askSecret(c)
	if err != nil || code != 200 || !strings.Contains(body, marker) {
		t.Fatalf("through the relay: %d %v", code, err)
	}
	if tp.streams.Load() == 0 {
		t.Fatal("nothing went through the relay, so this proves nothing")
	}
	tp.mu.Lock()
	seen := tp.seen.Bytes()
	tp.mu.Unlock()
	if bytes.Contains(seen, []byte(marker)) {
		t.Fatal("the relay could read what it passed on")
	}
	if len(seen) < 2000*len(marker) {
		t.Fatalf("the relay passed on only %d bytes; the answer alone is more", len(seen))
	}
}

func TestARelayThatChangesABytePassesOnNothingChanged(t *testing.T) {
	tp, relay := tappedRelay(t, 300<<10)
	devs := newDevList(testID(t, 2))
	n := startNode(t, 1, devs, behindRelay(relay), func(c *NodeConfig) { c.Handler = markedHandler() })
	c := startClient(t, 2, n, throughRelay(relay))
	waitPath(t, c, PathRelayed)
	req, _ := http.NewRequest("GET", "http://server/big", nil)
	ctx, cancel := context.WithTimeout(context.Background(), patience)
	defer cancel()
	res, err := c.Transport().RoundTrip(req.WithContext(ctx))
	if err == nil {
		b, rerr := io.ReadAll(res.Body)
		res.Body.Close()
		if rerr == nil {
			if !tp.flipped.Load() {
				t.Fatal("the relay never got as far as changing a byte, so this proves nothing")
			}
			t.Fatalf("an answer changed on the way arrived as if whole (%d bytes, same as sent: %v)", len(b), bytes.Equal(b, bigBody))
		}
	}
	if !tp.flipped.Load() {
		t.Fatal("the relay never changed a byte")
	}
}

// ---------------------------------------------------------------- dishonest peers

// A proof heard on one conversation is no use on another: a device's answer
// is bound to that conversation's two random numbers and both IDs.
func TestAJoinProofCanNotBeReplayed(t *testing.T) {
	n, devs, cfg, enrolled := joinWorld(t, "the-invite-behind-it")
	code, _ := NewJoinCode()
	clean, _ := CleanJoinCode(code)
	keys := deriveJoin(clean)
	if err := n.OfferJoin(code, "the-invite-behind-it"); err != nil {
		t.Fatal(err)
	}
	device := cfg(2, code)
	h, err := NewHost(HostConfig{Seed: testSeed(2), Local: true, Listen: device.Listen})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server, _ := peer.Decode(n.ID())
	if err := h.Connect(ctx, peer.AddrInfo{ID: server, Addrs: parseAddrs(n.Addrs(), server)}); err != nil {
		t.Fatal(err)
	}
	// begin opens a conversation and reads the server's challenge.
	begin := func() (network.Stream, []byte, []byte) {
		s, err := h.NewStream(ctx, server, ProtoJoin)
		if err != nil {
			t.Fatal(err)
		}
		mine := make([]byte, joinNonceLen)
		mine[0] = byte(time.Now().UnixNano())
		_ = writeMessage(s, joinHello{V: 1, Find: hex.EncodeToString(keys.find), Nonce: base64.StdEncoding.EncodeToString(mine), Name: "x", Platform: "x"})
		var ch joinChallenge
		if err := readMessage(s, &ch); err != nil || !ch.OK {
			t.Fatalf("challenge: %+v %v", ch, err)
		}
		theirs, _ := base64.StdEncoding.DecodeString(ch.Nonce)
		return s, mine, theirs
	}
	// The first conversation's right answer is worked out, and not sent.
	s1, mine1, theirs1 := begin()
	recorded := joinProof(keys.proof, "device", mine1, theirs1, server, h.ID())
	s1.Reset()
	// Played back on the next one, it is a wrong answer.
	s2, _, _ := begin()
	_ = writeMessage(s2, joinAnswer{Proof: base64.StdEncoding.EncodeToString(recorded)})
	var res EnrolResponse
	_ = readMessage(s2, &res)
	s2.Close()
	if res.OK || enrolled.Load() != 0 || devs.Allowed(h.ID().String()) {
		t.Fatalf("a replayed proof was taken: %+v", res)
	}
	// The same device, answering the conversation it is in, is let in.
	s3, mine3, theirs3 := begin()
	_ = writeMessage(s3, joinAnswer{Proof: base64.StdEncoding.EncodeToString(joinProof(keys.proof, "device", mine3, theirs3, server, h.ID()))})
	res = EnrolResponse{}
	_ = readMessage(s3, &res)
	s3.Close()
	if !res.OK || enrolled.Load() != 1 {
		t.Fatalf("the right answer, given in its own conversation, was refused: %+v", res)
	}
	// And the code is gone: a second right answer, even a fresh one, finds nothing.
	s4, err := h.NewStream(ctx, server, ProtoJoin)
	if err != nil {
		t.Fatal(err)
	}
	defer s4.Close()
	_ = writeMessage(s4, joinHello{V: 1, Find: hex.EncodeToString(keys.find), Nonce: base64.StdEncoding.EncodeToString(make([]byte, joinNonceLen))})
	var ch joinChallenge
	if err := readMessage(s4, &ch); err != nil || ch.OK {
		t.Fatalf("a used code answered again: %+v %v", ch, err)
	}
}

// A peer that opens an invite's conversation and then says nothing, or a
// byte at a time, is cut off when the conversation's time is up; one that
// claims a message of four gigabytes is told no without being read.
func TestSlowAndOversizedEnrolmentsAreCutOff(t *testing.T) {
	var called atomic.Int32
	n := startNode(t, 1, newDevList(), func(c *NodeConfig) {
		c.TalkWait = 500 * time.Millisecond
		c.Enrol = func(context.Context, string, EnrolRequest, netip.Addr) EnrolResponse {
			called.Add(1)
			return EnrolResponse{}
		}
	})
	h, err := NewHost(HostConfig{Seed: testSeed(2), Local: true, Listen: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server, _ := peer.Decode(n.ID())
	if err := h.Connect(ctx, peer.AddrInfo{ID: server, Addrs: parseAddrs(n.Addrs(), server)}); err != nil {
		t.Fatal(err)
	}
	ended := func(s network.Stream) time.Duration {
		start := time.Now()
		_ = s.SetReadDeadline(time.Now().Add(20 * time.Second))
		_, _ = io.Copy(io.Discard, s)
		return time.Since(start)
	}
	// Silence.
	s, err := h.NewStream(ctx, server, ProtoEnrol)
	if err != nil {
		t.Fatal(err)
	}
	if d := ended(s); d > 10*time.Second {
		t.Fatalf("a silent enrolment was held for %v", d)
	}
	// A trickle: a length, then a byte every 200 ms, until the server gives up.
	trickled, err := h.NewStream(ctx, server, ProtoEnrol)
	if err != nil {
		t.Fatal(err)
	}
	var head [4]byte
	binary.BigEndian.PutUint32(head[:], 100)
	_, _ = trickled.Write(head[:])
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		for range 100 {
			if _, err := trickled.Write([]byte{'x'}); err != nil {
				return
			}
			select {
			case <-stop:
				return
			case <-time.After(200 * time.Millisecond):
			}
		}
	}()
	d := ended(trickled)
	close(stop)
	<-stopped
	trickled.Reset()
	if d > 10*time.Second {
		t.Fatalf("a trickled enrolment was held for %v", d)
	}
	// Four gigabytes, claimed.
	s, err = h.NewStream(ctx, server, ProtoEnrol)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.Write([]byte{0xff, 0xff, 0xff, 0xff, '{'})
	var res EnrolResponse
	if err := readMessage(s, &res); err != nil || res.OK || res.Reason != "wrong" {
		t.Fatalf("an oversized enrolment was answered %+v %v", res, err)
	}
	s.Close()
	if called.Load() != 0 {
		t.Fatal("the server's invite check was asked about a message that never arrived whole")
	}
}

// One stranger trying invites as fast as they can, with new keys each time,
// uses up their own share and nobody else's.
func TestOneStrangersTriesDontLockOutEveryoneElse(t *testing.T) {
	n := &Node{enrolAll: newWindow(60, time.Minute, 1), enrolPeer: newWindow(5, time.Minute, 512), enrolAddr: newWindow(10, time.Minute, 512)}
	stranger := netip.MustParseAddr("203.0.113.9")
	allowed := 0
	for i := range 1000 {
		if n.allowTry(fmt.Sprint("stranger-key-", i), stranger) {
			allowed++
		}
	}
	if allowed != 10 {
		t.Fatalf("one address was allowed %d tries in a minute, want 10", allowed)
	}
	for i := range 5 {
		if !n.allowTry("someone-else", netip.MustParseAddr("198.51.100.7")) {
			t.Fatalf("someone else's try %d was refused after a stranger's flood", i+1)
		}
	}
	// IPv6: a whole /64 is one address here, so walking through it buys nothing.
	n = &Node{enrolAll: newWindow(60, time.Minute, 1), enrolPeer: newWindow(5, time.Minute, 512), enrolAddr: newWindow(10, time.Minute, 512)}
	allowed = 0
	for i := range 200 {
		a := netip.AddrFrom16([16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 1, 0, 2, byte(i), byte(i >> 8), 3, 4, 5, 6, 7, byte(i)})
		if n.allowTry(strings.Repeat("k", i+1), a) {
			allowed++
		}
	}
	if allowed != 10 {
		t.Fatalf("one /64 was allowed %d tries, want 10", allowed)
	}
}

func TestNamesFromElsewhereAreCleaned(t *testing.T) {
	for in, want := range map[string]string{
		"Home":                  "Home",
		"  Home\x00Server\n":    "Home Server",
		"Home\u202egnp.exe":     "Homegnp.exe",
		"\u2066Bank\u2069":      "Bank",
		"caf\xc3":               "caf",
		"\ufeffTV\u200f":        "TV",
		"tab\there":             "tab here",
		"\u0085next":            "next",
		strings.Repeat("é", 50): strings.Repeat("é", 40),
	} {
		if got := CleanName(in, 80); got != want {
			t.Errorf("CleanName(%q) = %q, want %q", in, got, want)
		}
	}
}
