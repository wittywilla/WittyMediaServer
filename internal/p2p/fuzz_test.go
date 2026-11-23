// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// The handlers a stranger can reach, fed whatever a fuzzer makes up as the
// stranger's side of the conversation, on a node of this machine's own. None
// may panic, hang, answer more than its frame allows, or let anyone onto the
// list without the invite or the code.

// fakeStream is one side of a conversation: what the other end "sent" is
// read from in, and what the handler writes is kept.
type fakeStream struct {
	network.Stream // anything not below panics: a handler using more is a test to update
	in             io.Reader
	out            bytes.Buffer
	conn           *fakeConn
}

func (s *fakeStream) Read(b []byte) (int, error)                   { return s.in.Read(b) }
func (s *fakeStream) Write(b []byte) (int, error)                  { return s.out.Write(b) }
func (s *fakeStream) Close() error                                 { return nil }
func (s *fakeStream) Reset() error                                 { return nil }
func (s *fakeStream) SetDeadline(time.Time) error                  { return nil }
func (s *fakeStream) SetReadDeadline(time.Time) error              { return nil }
func (s *fakeStream) SetWriteDeadline(time.Time) error             { return nil }
func (s *fakeStream) Conn() network.Conn                           { return s.conn }
func (s *fakeStream) Stat() network.Stats                          { return network.Stats{} }
func (s *fakeStream) Scope() network.StreamScope                   { return nil }
func (s *fakeStream) ResetWithError(network.StreamErrorCode) error { return nil }

type fakeConn struct {
	network.Conn
	from peer.ID
	addr ma.Multiaddr
}

func (c *fakeConn) RemotePeer() peer.ID           { return c.from }
func (c *fakeConn) RemoteMultiaddr() ma.Multiaddr { return c.addr }
func (c *fakeConn) Stat() network.ConnStats       { return network.ConnStats{} }
func (c *fakeConn) IsClosed() bool                { return false }

const fuzzInvite = "the-only-good-invite-0001"

// fuzzWorld is one node for one fuzz target's whole run, closed after it,
// with limits that never refuse (so the fuzzer reaches the parsers, not the
// "too many tries" answer) and one code waiting whose lookup key the corpus
// knows; keep puts it back, as wrong answers give a code up.
func fuzzWorld(f *testing.F) (n *Node, keys joinKeys, keep func()) {
	n, err := StartNode(NodeConfig{
		Seed: testSeed(1), Devices: newDevList(), DevicesOnly: true, Handler: http.NotFoundHandler(), Local: true,
		Listen: []string{"/ip4/127.0.0.1/tcp/0"},
		Enrol: func(_ context.Context, id string, req EnrolRequest, _ netip.Addr) EnrolResponse {
			if req.Secret != fuzzInvite {
				return EnrolResponse{Reason: "wrong"}
			}
			return EnrolResponse{OK: true, Code: "123456"}
		},
	})
	if err != nil {
		f.Fatal(err)
	}
	f.Cleanup(n.Close)
	n.enrolAll, n.enrolPeer, n.enrolAddr = newWindow(1<<30, time.Nanosecond, 4), newWindow(1<<30, time.Nanosecond, 4), newWindow(1<<30, time.Nanosecond, 4)
	keys = deriveJoin("K7QM2XHD")
	n.joins.mu.Lock()
	n.joins.open = map[string]*pendingJoin{}
	n.joins.mu.Unlock()
	keep = func() {
		// Kept waiting whatever the fuzzer does: wrong answers would give it up.
		n.joins.mu.Lock()
		n.joins.open[hex.EncodeToString(keys.find)] = &pendingJoin{proof: keys.proof, secret: "not-the-invite-behind-it", expires: time.Now().Add(time.Hour)}
		n.joins.mu.Unlock()
	}
	keep()
	return n, keys, keep
}

func frame(v any) []byte {
	var b bytes.Buffer
	_ = writeMessageN(&b, v, 1<<20)
	return b.Bytes()
}

// framesWritten reads back what a handler wrote: whole frames, each within the limit.
func framesWritten(t *testing.T, b []byte) []json.RawMessage {
	t.Helper()
	r := bytes.NewReader(b)
	var out []json.RawMessage
	for r.Len() > 0 {
		var m json.RawMessage
		if err := readMessage(r, &m); err != nil {
			t.Fatalf("the handler wrote something that isn't a whole frame within the limit: %v (%q)", err, b)
		}
		out = append(out, m)
	}
	return out
}

func strangerConn(t *testing.T) *fakeConn {
	t.Helper()
	id, _ := peer.Decode(testID(t, 9))
	return &fakeConn{from: id, addr: ma.StringCast("/ip4/203.0.113.7/tcp/4001")}
}

func FuzzEnrolConversation(f *testing.F) {
	n, _, _ := fuzzWorld(f)
	f.Add(frame(EnrolRequest{V: 1, Secret: "a-wrong-secret-0000000", Name: "x"}))
	f.Add(frame(EnrolRequest{V: 2, Secret: fuzzInvite}))
	f.Add(frame(map[string]any{"v": 1, "secret": []int{1, 2}}))
	f.Add([]byte{0, 0, 0, 3, '{', '}', '}'})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, in []byte) {
		s := &fakeStream{in: bytes.NewReader(in), conn: strangerConn(t)}
		n.handleEnrol(s)
		msgs := framesWritten(t, s.out.Bytes())
		if len(msgs) != 1 {
			t.Fatalf("%d answers to one enrolment", len(msgs))
		}
		var res EnrolResponse
		if err := json.Unmarshal(msgs[0], &res); err != nil {
			t.Fatal(err)
		}
		var req EnrolRequest
		good := readMessage(bytes.NewReader(in), &req) == nil && req.V == 1 && req.Secret == fuzzInvite
		if res.OK != good {
			t.Fatalf("answered %+v to %q", res, in)
		}
	})
}

func FuzzJoinConversation(f *testing.F) {
	n, keys, keep := fuzzWorld(f)
	find := hex.EncodeToString(keys.find)
	nonce := base64.StdEncoding.EncodeToString(make([]byte, joinNonceLen))
	hello := frame(joinHello{V: 1, Find: find, Nonce: nonce, Name: "TV", Platform: "tvOS"})
	f.Add(hello)
	f.Add(append(append([]byte{}, hello...), frame(joinAnswer{Proof: base64.StdEncoding.EncodeToString(make([]byte, 32))})...))
	f.Add(append(append([]byte{}, hello...), frame(joinAnswer{Proof: "%%%"})...))
	f.Add(frame(joinHello{V: 1, Find: find, Nonce: "short"}))
	f.Add(frame(joinHello{V: 1, Find: "00", Nonce: nonce}))
	f.Add(frame(joinHello{V: 3}))
	f.Fuzz(func(t *testing.T, in []byte) {
		keep()
		s := &fakeStream{in: bytes.NewReader(in), conn: strangerConn(t)}
		n.handleJoin(s)
		msgs := framesWritten(t, s.out.Bytes())
		if len(msgs) == 0 || len(msgs) > 2 {
			t.Fatalf("%d messages in one conversation", len(msgs))
		}
		var first joinChallenge
		if err := json.Unmarshal(msgs[0], &first); err != nil {
			t.Fatal(err)
		}
		if !first.OK && first.Proof != "" {
			t.Fatal("a refusal carried a proof to chew on")
		}
		if len(msgs) == 2 {
			var res EnrolResponse
			if err := json.Unmarshal(msgs[1], &res); err != nil {
				t.Fatal(err)
			}
			// The fuzzer doesn't know the server's random number before it
			// writes its answer, so it can't have proved anything.
			if res.OK {
				t.Fatalf("a made-up answer was taken: %q", in)
			}
		}
	})
}

func FuzzCleanJoinCode(f *testing.F) {
	f.Add("K7QM-2XHD")
	f.Add("k7qm 2xhd")
	f.Add("OIL0-1234")
	f.Add("ＫＱ７Ｍ２ＸＨＤ")
	f.Fuzz(func(t *testing.T, typed string) {
		code, err := CleanJoinCode(typed)
		if err != nil {
			return
		}
		if len(code) != JoinCodeLen {
			t.Fatalf("%q cleaned to %q", typed, code)
		}
		for _, r := range code {
			if !bytes.ContainsRune([]byte(joinAlphabet), r) {
				t.Fatalf("%q cleaned to %q, which has %q", typed, code, r)
			}
		}
		again, err := CleanJoinCode(FormatJoinCode(code))
		if err != nil || again != code {
			t.Fatalf("%q doesn't read back: %q %v", code, again, err)
		}
	})
}

func FuzzCleanName(f *testing.F) {
	f.Add("Home", 80)
	f.Add("a\u202eb", 3)
	f.Add("\xff\xfe", 1)
	f.Fuzz(func(t *testing.T, s string, max int) {
		if max < 0 || max > 1000 {
			return
		}
		got := CleanName(s, max)
		if len(got) > max {
			t.Fatalf("%q is longer than %d", got, max)
		}
		if !utf8Valid(got) {
			t.Fatalf("%q isn't valid UTF-8", got)
		}
		for _, r := range got {
			if r < 0x20 || r == 0x7f || (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) || r == 0xfeff {
				t.Fatalf("%q kept %U", got, r)
			}
		}
		if CleanName(got, max) != got {
			t.Fatalf("cleaning %q again changed it", got)
		}
	})
}

func FuzzRelayOffer(f *testing.F) {
	f.Add([]byte(`{"v":1,"name":"Mine","access":"key","kbps":4000,"maxHeight":720,"dataMiB":4096,"minutes":180}`))
	f.Add([]byte(`{"kbps":-1,"totalKbps":99999999999,"dataMiB":-5,"note":"\u202eevil"}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		var o RelayOffer
		if json.Unmarshal(b, &o) != nil {
			return
		}
		c := o.Clean()
		if c.Kbps < 0 || c.TotalKbps < 0 || c.DataMiB < 0 || c.Minutes < 0 || (c.MaxHeight != 0 && (c.MaxHeight < 144 || c.MaxHeight > 4320)) {
			t.Fatalf("%+v cleaned to %+v", o, c)
		}
		if c.Clean() != c {
			t.Fatalf("cleaning twice changed %+v", c)
		}
		if w := c.Words(); w == "" {
			t.Fatal("no words")
		}
	})
}

func FuzzSeedsAndIDs(f *testing.F) {
	f.Add("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	f.Add("12D3KooWD3eckifWpRn9wQpMG9R9hX3sD158z7EqHWmweQAJU5SA")
	f.Add("QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
	f.Fuzz(func(t *testing.T, s string) {
		if seed, err := DecodeSeed(s); err == nil {
			if len(seed) != SeedSize {
				t.Fatalf("a seed of %d bytes", len(seed))
			}
			id, err := IDFromSeed(seed)
			if err != nil {
				t.Fatal(err)
			}
			if again, err := CheckID(id); err != nil || again != id {
				t.Fatalf("%s doesn't check as itself", id)
			}
		}
		if id, err := CheckID(s); err == nil {
			// Only Ed25519 keys, whose ID holds the whole key.
			p, _ := peer.Decode(id)
			if k, err := p.ExtractPublicKey(); err != nil || k == nil {
				t.Fatalf("%q passed without a key in it", s)
			}
		}
	})
}

func utf8Valid(s string) bool { return bytes.Equal([]byte(s), bytes.ToValidUTF8([]byte(s), nil)) }
