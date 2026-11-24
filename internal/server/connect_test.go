// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/p2p"
)

type offerMade struct {
	Code          string   `json:"code"`
	Link          string   `json:"link"`
	QRLink        string   `json:"qrLink"`
	ExpiresInSec  int      `json:"expiresInSec"`
	Away          bool     `json:"away"`
	HomeAddresses []string `json:"homeAddresses"`
	User          struct{ Name string }
	Reason        string `json:"reason"`
}

func makeOffer(t *testing.T, ts *testServer, token string, body map[string]any) offerMade {
	t.Helper()
	if body == nil {
		body = map[string]any{}
	}
	w := ts.do(t, "POST", "/api/connect/offer", token, body)
	if w.Code != 200 {
		t.Fatalf("making a code: %d %s", w.Code, w.Body)
	}
	var o offerMade
	_ = json.Unmarshal(w.Body.Bytes(), &o)
	return o
}

// codeDevice is a device's side of a code typed on it, over the web port, as the apps do it.
type codeDevice struct {
	find, proof []byte
	nonce       []byte
}

func newCodeDevice(t *testing.T, code string) *codeDevice {
	t.Helper()
	find, proof, err := p2p.DeriveWebJoin(code)
	if err != nil {
		t.Fatal(err)
	}
	n := make([]byte, 16)
	_, _ = rand.Read(n)
	return &codeDevice{find: find, proof: proof, nonce: n}
}

type helloAnswer struct {
	Nonce, PublicKey, Proof, Reason string
}

func (d *codeDevice) hello(t *testing.T, ts *testServer, hdr ...string) (int, helloAnswer) {
	t.Helper()
	w := ts.do(t, "POST", "/api/connect/hello", "", map[string]any{"v": 1, "find": hex.EncodeToString(d.find), "nonce": base64.StdEncoding.EncodeToString(d.nonce)}, hdr...)
	var a helloAnswer
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	return w.Code, a
}

// serverKnows is the check the device makes before it says anything more.
func (d *codeDevice) serverKnows(a helloAnswer, identityKey string) bool {
	sn, _ := base64.StdEncoding.DecodeString(a.Nonce)
	got, _ := base64.StdEncoding.DecodeString(a.Proof)
	return a.PublicKey == identityKey && hmac.Equal(got, p2p.WebJoinProof(d.proof, "server", d.nonce, sn, identityKey))
}

func (d *codeDevice) answer(t *testing.T, ts *testServer, a helloAnswer, key []byte, hdr ...string) (int, map[string]any) {
	t.Helper()
	sn, _ := base64.StdEncoding.DecodeString(a.Nonce)
	proof := p2p.WebJoinProof(key, "device", d.nonce, sn, a.PublicKey)
	w := ts.do(t, "POST", "/api/connect/answer", "", map[string]any{"v": 1, "find": hex.EncodeToString(d.find), "nonce": base64.StdEncoding.EncodeToString(d.nonce),
		"serverNonce": a.Nonce, "proof": base64.StdEncoding.EncodeToString(proof), "device": "Desk\n\x00top", "platform": "Linux"}, hdr...)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func identityKeyOf(t *testing.T, ts *testServer) string {
	t.Helper()
	id, err := ts.Identity()
	if err != nil {
		t.Fatal(err)
	}
	return id.PublicKey()
}

// At home, with peer to peer off: the code is typed on the device, the server proves it made it, the
// device proves it holds it, and is signed in as whoever made it. Once.
func TestConnectCodeAtHome(t *testing.T) {
	ts := newTestServer(t)
	_, kate := ts.user(t, "kate", "adult", false, "")
	o := makeOffer(t, ts, kate, nil)
	if len(o.Code) != 9 || o.Code[4] != '-' || o.ExpiresInSec != 900 || o.Away || o.Link != "" || o.User.Name != "kate" {
		t.Fatalf("the offer: %+v", o)
	}
	key := identityKeyOf(t, ts)

	// The device types it loosely: lower case, a space for the dash, an O for a zero.
	typed := " " + o.Code[:4] + " " + o.Code[5:]
	d := newCodeDevice(t, typed)
	code, a := d.hello(t, ts)
	if code != 200 || !d.serverKnows(a, key) {
		t.Fatalf("hello: %d %+v", code, a)
	}
	code, out := d.answer(t, ts, a, d.proof)
	if code != 200 || out["token"] == "" || out["user"].(map[string]any)["name"] != "kate" {
		t.Fatalf("answer: %d %v", code, out)
	}
	// The token is a sign-in, and the device has the name it gave, cleaned.
	if w := ts.do(t, "GET", "/api/me", out["token"].(string), nil); w.Code != 200 {
		t.Fatalf("the sign-in: %d", w.Code)
	}
	// Once.
	d2 := newCodeDevice(t, o.Code)
	if code, _ := d2.hello(t, ts); code != 404 {
		t.Fatalf("a used code still answered: %d", code)
	}
	if code, _ := d.answer(t, ts, a, d.proof); code == 200 {
		t.Fatal("the same answer worked twice")
	}
}

// A server that didn't make the code says so and proves nothing; a device that can't prove the code
// gets three tries and then the code is gone. A challenge is answered once.
func TestConnectCodeRefusals(t *testing.T) {
	ts := newTestServer(t)
	_, kate := ts.user(t, "kate", "adult", false, "")
	other, _ := p2p.NewJoinCode()
	if code, a := newCodeDevice(t, other).hello(t, ts); code != 404 || a.Reason != "not-here" || a.Proof != "" {
		t.Fatalf("a code this server never made: %d %+v", code, a)
	}
	for _, bad := range []map[string]any{{"v": 1, "find": "x", "nonce": "AAAA"}, {"v": 2}, {"find": hex.EncodeToString(make([]byte, 32)), "nonce": "AAAA", "v": 1}} {
		if w := ts.do(t, "POST", "/api/connect/hello", "", bad); w.Code != 400 {
			t.Fatalf("%v: %d", bad, w.Code)
		}
	}
	o := makeOffer(t, ts, kate, nil)
	d := newCodeDevice(t, o.Code)
	wrong := make([]byte, 32)
	for i := 0; i < connectMaxWrong; i++ {
		code, a := d.hello(t, ts)
		if code != 200 {
			t.Fatalf("hello %d: %d", i, code)
		}
		if code, _ := d.answer(t, ts, a, wrong); code == 200 {
			t.Fatal("a wrong proof was taken")
		}
		// The challenge it was given is spent, right or wrong.
		if code, _ := d.answer(t, ts, a, d.proof); code == 200 {
			t.Fatal("a challenge was answered twice")
		}
	}
	if code, _ := d.hello(t, ts); code != 404 {
		t.Fatalf("after %d wrong proofs the code still answers: %d", connectMaxWrong, code)
	}
}

// Who may make one, and an administrator's needs the password again.
func TestConnectOfferWhoMay(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, kid := ts.user(t, "kid", "kids", false, "")
	if w := ts.do(t, "POST", "/api/connect/offer", "", map[string]any{}); w.Code != 401 {
		t.Fatalf("nobody signed in: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/connect/offer", kid, map[string]any{}); w.Code != 403 {
		t.Fatalf("a kid's profile: %d", w.Code)
	}
	w := ts.do(t, "POST", "/api/connect/offer", admin, map[string]any{})
	var refusal struct{ Reason string }
	_ = json.Unmarshal(w.Body.Bytes(), &refusal)
	if w.Code != 403 || refusal.Reason != pairNeedsPassword {
		t.Fatalf("an administrator's with no password: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/connect/offer", admin, map[string]any{"password": "nope"}); w.Code == 200 {
		t.Fatal("a wrong password was taken")
	}
	if o := makeOffer(t, ts, admin, map[string]any{"password": "correct-horse-1"}); o.Code == "" {
		t.Fatal("no code with the password")
	}
}

// A new code ends the one before; Cancel ends it; and it dies with the sign-in that made it.
func TestConnectOfferEnds(t *testing.T) {
	ts := newTestServer(t)
	_, kate := ts.user(t, "kate", "adult", false, "")
	first := makeOffer(t, ts, kate, nil)
	second := makeOffer(t, ts, kate, nil)
	if code, _ := newCodeDevice(t, first.Code).hello(t, ts); code != 404 {
		t.Fatalf("the code before a new one still answers: %d", code)
	}
	if code, _ := newCodeDevice(t, second.Code).hello(t, ts); code != 200 {
		t.Fatalf("the new one: %d", code)
	}
	if w := ts.do(t, "DELETE", "/api/connect/offer", kate, nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if code, _ := newCodeDevice(t, second.Code).hello(t, ts); code != 404 {
		t.Fatalf("a cancelled code still answers: %d", code)
	}

	third := makeOffer(t, ts, kate, nil)
	d := newCodeDevice(t, third.Code)
	_, a := d.hello(t, ts)
	ts.Auth.Logout(context.Background(), kate)
	if code, out := d.answer(t, ts, a, d.proof); code == 200 {
		t.Fatalf("a code whose maker signed out signed a device in: %v", out)
	}

	// And it expires.
	_, lee := ts.user(t, "lee", "adult", false, "")
	fourth := makeOffer(t, ts, lee, nil)
	ts.connect.mu.Lock()
	for _, x := range ts.connect.open {
		x.expires = time.Now().Add(-time.Second)
	}
	ts.connect.mu.Unlock()
	if code, _ := newCodeDevice(t, fourth.Code).hello(t, ts); code != 404 {
		t.Fatalf("an expired code still answers: %d", code)
	}
}

// With peer to peer on, the same offer is also a link: the way in from away. The link and the code are one
// way in: whichever is used first ends the other.
func TestConnectOfferIsOneWayIn(t *testing.T) {
	ts, _ := p2pServer(t, nil)
	node := ts.p2pNode()
	_, kate := ts.user(t, "kate", "adult", false, "")

	// The link first, as a device away from home uses it: it goes on the list and gets its sign-in code...
	o := makeOffer(t, ts, kate, nil)
	if !o.Away || o.Link == "" {
		t.Fatalf("with peer to peer on: %+v", o)
	}
	inv, err := p2p.ParseInvite(o.Link)
	if err != nil || inv.ServerID != node.ID() {
		t.Fatalf("the link: %v %+v", err, inv)
	}
	// The QR code's is the same way in, short enough to be drawn.
	short, err := p2p.ParseInvite(o.QRLink)
	if err != nil || short.ServerID != inv.ServerID || short.Secret != inv.Secret || len(o.QRLink) > connectQRMax {
		t.Fatalf("the QR code's link (%d bytes): %v %+v", len(o.QRLink), err, short)
	}
	app := p2pApp(t, ts, 4)
	res, err := app.Enrol(context.Background(), inv.Secret, "Laptop", "Linux")
	if err != nil || !res.OK || len(res.Code) != 6 {
		t.Fatalf("enrolling with the link: %+v %v", res, err)
	}
	if !ts.p2p.devices.Allowed(testP2PID(t, 4)) {
		t.Fatal("the device isn't on the list")
	}
	// ... and the code no longer works.
	d := newCodeDevice(t, o.Code)
	if code, a := d.hello(t, ts); code == 200 {
		if code, _ := d.answer(t, ts, a, d.proof); code == 200 {
			t.Fatal("the code worked at home after the link was used")
		}
	}

	// The code first: the link no longer works.
	o = makeOffer(t, ts, kate, nil)
	inv, _ = p2p.ParseInvite(o.Link)
	d = newCodeDevice(t, o.Code)
	_, a := d.hello(t, ts)
	if code, out := d.answer(t, ts, a, d.proof); code != 200 {
		t.Fatalf("the code at home: %d %v", code, out)
	}
	res, err = p2pApp(t, ts, 5).Enrol(context.Background(), inv.Secret, "Phone", "Android")
	if err != nil || res.OK || res.Reason != "used" {
		t.Fatalf("the link after the code was used: %+v %v", res, err)
	}
	if ts.p2p.devices.Allowed(testP2PID(t, 5)) {
		t.Fatal("a spent link put a device on the list")
	}
}

// The words the simple screen starts with.
func TestConnectStatusWords(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, kate := ts.user(t, "kate", "adult", false, "")
	var st connectStatus
	_ = json.Unmarshal(ts.do(t, "GET", "/api/connect", admin, nil).Body.Bytes(), &st)
	if st.Away != "off" || !st.CanTurnOn || st.P2P || st.Words == "" {
		t.Fatalf("off: %+v", st)
	}
	_ = json.Unmarshal(ts.do(t, "GET", "/api/connect", kate, nil).Body.Bytes(), &st)
	if st.CanTurnOn {
		t.Fatal("someone who isn't an administrator is offered to turn it on")
	}
	if w := ts.do(t, "GET", "/api/connect", "", nil); w.Code != 401 {
		t.Fatalf("nobody signed in: %d", w.Code)
	}
}

// The link a QR code holds fits it, keeps the server and the secret, and keeps the addresses most worth
// trying: a circuit through the owner's own relay first, then public, then this server's own at home; never a
// public relay's circuit or this machine's. (Checked at 210 bytes, where they have to compete, and at the real 300.)
func TestQRInviteFits(t *testing.T) {
	const tight = 210
	id := "12D3KooWQSZd5NLcWyVkrniKq1MggbNPVhYfcqs6cQP8AXLRVsdW"
	inv := p2p.Invite{ServerID: id, Secret: "abcdefghijklmnopqrstuv", Name: "The family's media server, downstairs",
		Addrs: []string{"/ip4/172.17.0.1/tcp/19893", "/ip4/10.255.255.254/udp/19893/quic-v1", "/ip4/192.168.1.20/udp/19893/quic-v1",
			"/ip4/81.2.69.160/udp/19893/quic-v1", "/ip4/127.0.0.1/tcp/19893", "/ip4/1.2.3.4/tcp/4001/p2p/" + id + "/p2p-circuit"}}
	if len(inv.String()) <= tight {
		t.Fatalf("the test needs a link too long for a QR code: %d", len(inv.String()))
	}
	link := qrInvite(inv, []string{"http://192.168.1.20:9797"}, nil, tight)
	got, err := p2p.ParseInvite(link)
	if err != nil || len(link) > tight || got.ServerID != id || got.Secret != inv.Secret {
		t.Fatalf("%d bytes: %v %+v", len(link), err, got)
	}
	if len(got.Addrs) == 0 || got.Addrs[0] != "/ip4/81.2.69.160/udp/19893/quic-v1" {
		t.Fatalf("the public address first: %v", got.Addrs)
	}
	if len(got.Addrs) > 1 && got.Addrs[1] != "/ip4/192.168.1.20/udp/19893/quic-v1" {
		t.Fatalf("then the one at home: %v", got.Addrs)
	}
	for _, a := range got.Addrs {
		if strings.Contains(a, "p2p-circuit") {
			t.Fatalf("a public relay's circuit in the QR code: %v", got.Addrs)
		}
	}
	// A short one is left as it is.
	small := p2p.Invite{ServerID: id, Secret: "abcdefghijklmnopqrstuv", Addrs: []string{"/ip4/192.168.1.20/tcp/19893"}}
	if qrInvite(small, nil, nil, connectQRMax) != small.String() {
		t.Fatal("a link that fits was changed")
	}
	// The web address at home and the code stay, addresses going first; a name too long for even that loses them.
	inv.Home, inv.Code = "http://192.168.1.20:9797", "K7QM2XHD"
	link = qrInvite(inv, []string{"http://192.168.1.20:9797"}, nil, tight)
	if got, err := p2p.ParseInvite(link); err != nil || len(link) > tight || got.Home != inv.Home || got.Code != inv.Code {
		t.Fatalf("%d bytes: %v %+v", len(link), err, got)
	}
	long := inv
	long.Home = "http://" + strings.Repeat("n", 180) + ".local"
	link = qrInvite(long, nil, nil, tight)
	if got, err := p2p.ParseInvite(link); err != nil || len(link) > tight || got.Home != "" || got.Code != "" {
		t.Fatalf("%d bytes: %v %+v", len(link), err, got)
	}
	// The owner's own WittyRelay (#683): its circuit comes first and fits beside the address at home and the code, in
	// the QR code's real 300 bytes; a phone away from home then has the one way in that works.
	relay := "12D3KooWD8PWB8C8WmfLx4KZELLzAQbUpRQxbVJVHt94e8zKt8qj"
	ownCircuit := "/ip4/80.0.0.10/udp/4001/quic-v1/p2p/" + relay + "/p2p-circuit"
	inv.Addrs = append(inv.Addrs, ownCircuit)
	link = qrInvite(inv, []string{"http://192.168.1.20:9797"}, map[string]bool{relay: true}, connectQRMax)
	got, err = p2p.ParseInvite(link)
	if err != nil || len(link) > connectQRMax || len(got.Addrs) == 0 || got.Addrs[0] != ownCircuit || got.Home != inv.Home || got.Code != inv.Code {
		t.Fatalf("%d bytes: %v %+v", len(link), err, got)
	}
	if circuitRelay(ownCircuit) != relay || circuitRelay("/ip4/1.2.3.4/tcp/1") != "" {
		t.Fatal("circuitRelay")
	}
}

// The owner's homelab (0.8.4): the server in a container on Docker's bridge network, the page opened at the
// host's address. Devices at home are pointed at that address, never the container's own, which nothing else
// reaches; the QR code holds it and the code for an app at home; the peer-to-peer port is tried at the host.
// A page opened at this computer's own name, or from the internet, gives no address at home.
func TestConnectOfferOnADockerHost(t *testing.T) {
	ts, admin := p2pServer(t, nil)
	_, kate := ts.user(t, "kate", "adult", false, "")
	const host = "192.168.213.50" // no computer running the tests has it, as no container has its host's

	// In a container, with nothing answering at the host's peer-to-peer port: Connect a device says plainly that the
	// port isn't published from Docker, and which lines to add (the owner's 0.8.4 setup).
	t.Setenv("WMS_DOCKER", "1")
	var st connectStatus
	_ = json.Unmarshal(ts.do(t, "GET", "/api/connect", admin, nil, "RemoteAddr", "192.168.213.20:5000", "Host", host+":9797").Body.Bytes(), &st)
	// (The tests' node listens on a random port for each of UDP and TCP, so the number isn't checked here; a real one
	// has one port for both.)
	if !strings.Contains(st.Hint, " isn't published from Docker") || !strings.Contains(st.Hint, "/udp\" to ports:") || st.Away == "yes" {
		t.Fatalf("the status in a container whose port isn't published: %+v", st)
	}
	w := ts.do(t, "POST", "/api/connect/offer", kate, map[string]any{}, "RemoteAddr", "192.168.213.20:5000", "Host", host+":9797")
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var o offerMade
	_ = json.Unmarshal(w.Body.Bytes(), &o)
	if len(o.HomeAddresses) != 1 || o.HomeAddresses[0] != "http://"+host+":9797" {
		t.Fatalf("the address at home: %v", o.HomeAddresses)
	}
	inv, err := p2p.ParseInvite(o.Link)
	if err != nil || len(inv.Addrs) < 2 || !strings.HasPrefix(inv.Addrs[0], "/ip4/"+host+"/udp/") || !strings.HasSuffix(inv.Addrs[0], "/quic-v1") ||
		!strings.HasPrefix(inv.Addrs[1], "/ip4/"+host+"/tcp/") {
		t.Fatalf("the peer-to-peer port on the host first: %v %v", inv.Addrs, err)
	}
	if inv.Home != "" || inv.Code != "" {
		t.Fatal("the link, which may be sent anywhere, carries the code")
	}
	qr, err := p2p.ParseInvite(o.QRLink)
	clean, _ := p2p.CleanJoinCode(o.Code)
	if err != nil || len(o.QRLink) > connectQRMax || qr.Home != "http://"+host+":9797" || qr.Code != clean || qr.Secret != inv.Secret {
		t.Fatalf("the QR code: %d bytes %+v %v", len(o.QRLink), qr, err)
	}
	_ = ts.do(t, "DELETE", "/api/connect/offer", kate, nil)

	for what, hdr := range map[string][]string{
		"this computer's own name": {"RemoteAddr", "127.0.0.1:5000", "Host", "localhost:9797"},
		"from the internet":        {"RemoteAddr", "203.0.113.9:5000", "Host", "203.0.113.4:9797"},
		"a public name":            {"RemoteAddr", "192.168.213.20:5000", "Host", "media.example.com"},
	} {
		w := ts.do(t, "POST", "/api/connect/offer", kate, map[string]any{}, hdr...)
		var o offerMade
		_ = json.Unmarshal(w.Body.Bytes(), &o)
		if w.Code != 200 {
			continue // refused outright is fine too
		}
		if qr, _ := p2p.ParseInvite(o.QRLink); qr.Home != "" || qr.Code != "" {
			t.Errorf("%s: the QR code holds %q", what, qr.Home)
		}
		for _, a := range o.HomeAddresses {
			if strings.Contains(a, "localhost") || strings.Contains(a, "example.com") || strings.Contains(a, "203.0.113") {
				t.Errorf("%s: %q given as the address at home", what, a)
			}
		}
		_ = ts.do(t, "DELETE", "/api/connect/offer", kate, nil, hdr...)
	}
}
