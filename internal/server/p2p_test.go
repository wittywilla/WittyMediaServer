// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/p2p"
	"github.com/wittywilla/wittymediaserver/internal/portmap"
	"github.com/wittywilla/wittymediaserver/internal/remote"
)

// Everything here stays on this machine: the server's node and the apps' nodes
// listen on 127.0.0.1 and know only each other. No address book, no relay and
// no router is asked for anything, and none of it could reach the internet.

var p2pLoopback = []string{"/ip4/127.0.0.1/udp/0/quic-v1", "/ip4/127.0.0.1/tcp/0"}

// p2pServer is a test server with peer-to-peer connections turned on.
func p2pServer(t *testing.T, tune func(*p2p.NodeConfig)) (*testServer, string) {
	t.Helper()
	ts := newTestServer(t)
	ts.p2p.tune = func(c *p2p.NodeConfig) {
		c.Local, c.Listen = true, p2pLoopback
		c.Layers = p2p.Layers{}
		if tune != nil {
			tune(c)
		}
	}
	_, admin := ts.user(t, "owner", "adult", true, "")
	ts.StartP2P(context.Background(), ts.h)
	t.Cleanup(ts.StopP2P)
	if w := ts.do(t, "PUT", "/api/p2p/settings", admin, map[string]any{"enabled": true, "accept": true}); w.Code != 200 {
		t.Fatalf("turning it on: %d %s", w.Code, w.Body)
	}
	if ts.p2pNode() == nil {
		t.Fatal("turned on, but no node runs")
	}
	return ts, admin
}

func p2pSeedFor(b byte) []byte { return bytes.Repeat([]byte{b}, p2p.SeedSize) }

// p2pApp is an app's end, connecting to the test server from this machine.
func p2pApp(t *testing.T, ts *testServer, seed byte) *p2p.Client {
	t.Helper()
	node := ts.p2pNode()
	c, err := p2p.NewClient(p2p.ClientConfig{Seed: p2pSeedFor(seed), ServerID: node.ID(), Hints: node.Addrs(), Local: true, Listen: p2pLoopback, UpgradeWait: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

// over sends a request through an app's peer-to-peer connection.
func over(t *testing.T, c *p2p.Client, method, path, token string, body any, hdr ...string) (int, string, error) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, "http://127.0.0.1:9797"+path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	res, err := c.Transport().RoundTrip(req.WithContext(ctx))
	if err != nil {
		return 0, "", err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b), nil
}

func enrol(t *testing.T, ts *testServer, token string, c *p2p.Client, name string) {
	t.Helper()
	if w := ts.do(t, "POST", "/api/p2p/devices", token, map[string]any{"id": c.ID(), "name": name, "platform": "desktop"}); w.Code != 200 {
		t.Fatalf("enrolling %s: %d %s", name, w.Code, w.Body)
	}
}

// The test that matters most. The app here really is on this computer: its
// connection to the server's node comes from 127.0.0.1. If anything took that
// address at its word, a device away from home would be treated as the owner
// sitting at the server. Nothing may.
func TestP2PRequestsCountAsAwayFromHome(t *testing.T) {
	ts, admin := p2pServer(t, nil)
	app := p2pApp(t, ts, 2)
	enrol(t, ts, admin, app, "Laptop")

	// What this machine itself may see without signing in: how remote access
	// stands (the tray asks), and which version runs.
	if w := ts.do(t, "GET", "/api/remote/status", "", nil); w.Code != 200 {
		t.Fatalf("from this machine: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/capabilities", "", nil); !strings.Contains(w.Body.String(), `"version":"test"`) {
		t.Fatalf("from this machine the version is shown: %s", w.Body)
	}

	// The same two questions over the peer-to-peer connection, with every
	// header a proxy on this machine might add.
	for _, hdr := range [][]string{nil, {"X-Forwarded-For", "127.0.0.1"}, {"X-Forwarded-For", "192.168.1.5", "X-Forwarded-Proto", "https", "X-Forwarded-Host", "localhost"}, {"Forwarded", "for=127.0.0.1"}, {"X-Real-IP", "127.0.0.1"}} {
		code, body, err := over(t, app, "GET", "/api/remote/status", "", nil, hdr...)
		if err != nil || code != 401 {
			t.Fatalf("headers %v: a peer-to-peer request was answered as if from this computer: %d %s %v", hdr, code, body, err)
		}
		code, body, err = over(t, app, "GET", "/api/capabilities", "", nil, hdr...)
		if err != nil || code != 200 || !strings.Contains(body, `"version":""`) {
			t.Fatalf("headers %v: the version was shown to a stranger from away: %d %s %v", hdr, code, body, err)
		}
	}

	// Signed in over it, the session is listed as from the internet, by a
	// peer-to-peer connection, under an address that isn't this machine's.
	code, body, err := over(t, app, "GET", "/api/sessions", admin, nil)
	if err != nil || code != 200 {
		t.Fatalf("%d %s %v", code, body, err)
	}
	var sessions []struct{ IP, Where, Via string }
	_ = json.Unmarshal([]byte(body), &sessions)
	if len(sessions) != 1 || sessions[0].Where != "internet" || sessions[0].Via != "p2p" {
		t.Fatalf("the session after a peer-to-peer request: %+v", sessions)
	}
	ip, err := netip.ParseAddr(sessions[0].IP)
	if err != nil || ip.IsLoopback() || ip.IsPrivate() || (*remote.Policy)(nil).Classify(ip) != remote.Internet {
		t.Fatalf("it is counted under %q", sessions[0].IP)
	}

	// The limits for viewers away from home apply to it.
	if w := ts.do(t, "PUT", "/api/remote", admin, map[string]any{"maxHeight": 720, "maxBitrate": "4M"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	r := httptest.NewRequest("GET", "/api/items/1/file", nil)
	r.RemoteAddr = "127.0.0.1:5000"
	marked := r.WithContext(remote.WithP2P(r.Context(), remote.P2PConn{Addr: netip.MustParseAddr("127.0.0.1")}))
	if h, rate := ts.remoteCaps(marked); h != 720 || rate != "4M" {
		t.Fatalf("limits for a peer-to-peer viewer: %d %q", h, rate)
	}
	if h, rate := ts.remoteCaps(r); h != 0 || rate != "" {
		t.Fatalf("the same request without the mark is at home: %d %q", h, rate)
	}
}

// Remote Access itself is off in these tests. A request marked as peer-to-peer
// is let through the gate only while peer-to-peer connections are on.
func TestP2PGateFollowsItsOwnSwitch(t *testing.T) {
	ts, admin := p2pServer(t, nil)
	marked := func() int {
		r := httptest.NewRequest("GET", "/api/capabilities", nil)
		r.RemoteAddr = "127.0.0.1:5000"
		r = r.WithContext(remote.WithP2P(r.Context(), remote.P2PConn{Addr: netip.MustParseAddr("198.51.100.9")}))
		w := httptest.NewRecorder()
		ts.h.ServeHTTP(w, r)
		return w.Code
	}
	if code := marked(); code != 200 {
		t.Fatalf("on: %d", code)
	}
	// Someone on the internet who isn't peer-to-peer is still kept out.
	if w := ts.do(t, "GET", "/api/capabilities", "", nil, "RemoteAddr", "198.51.100.9:4000"); w.Code != 403 {
		t.Fatalf("an ordinary request from the internet with Remote Access off: %d", w.Code)
	}
	if w := ts.do(t, "PUT", "/api/p2p/settings", admin, map[string]any{"enabled": false}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if ts.p2pNode() != nil || ts.p2pOn() {
		t.Fatal("turned off, and still running")
	}
	if code := marked(); code != 403 {
		t.Fatalf("off: a marked request got %d", code)
	}
}

func TestP2POnlyListedDevicesGetIn(t *testing.T) {
	ts, admin := p2pServer(t, nil)
	_, kate := ts.user(t, "kate", "adult", false, "")
	app := p2pApp(t, ts, 2)

	// Not on the list: no answer at all, not even the sign-in page.
	if _, _, err := over(t, app, "GET", "/api/capabilities", "", nil); err == nil {
		t.Fatal("a device that isn't on the list was answered")
	}
	enrol(t, ts, kate, app, "Kate's laptop")
	if code, _, err := over(t, app, "GET", "/api/capabilities", "", nil); err != nil || code != 200 {
		t.Fatalf("after enrolling: %d %v", code, err)
	}

	// The lists: Kate sees hers, the admin everyone's, and Kate can't ask for everyone's.
	var mine, all []p2pDevice
	_ = json.Unmarshal(ts.do(t, "GET", "/api/p2p/devices", kate, nil).Body.Bytes(), &mine)
	_ = json.Unmarshal(ts.do(t, "GET", "/api/p2p/devices?all=1", admin, nil).Body.Bytes(), &all)
	if len(mine) != 1 || !mine[0].Mine || mine[0].Name != "Kate's laptop" || mine[0].LastSeen == 0 || !mine[0].Connected || mine[0].Path != "direct" {
		t.Fatalf("Kate's list: %+v", mine)
	}
	if len(all) != 1 || all[0].User != "kate" || all[0].Mine {
		t.Fatalf("the admin's list: %+v", all)
	}
	if w := ts.do(t, "GET", "/api/p2p/devices?all=1", kate, nil); w.Code != 403 {
		t.Fatalf("everyone's devices for a non-admin: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/p2p/devices", admin, nil); strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("the admin's own devices: %s", w.Body)
	}

	// Nobody takes over a device by naming its key, or removes someone else's.
	_, bob := ts.user(t, "bob", "adult", false, "")
	if w := ts.do(t, "POST", "/api/p2p/devices", bob, map[string]any{"id": app.ID(), "name": "mine now"}); w.Code != 400 {
		t.Fatalf("claiming someone else's device: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "DELETE", "/api/p2p/devices/"+app.ID(), bob, nil); w.Code != 404 {
		t.Fatalf("removing someone else's device: %d", w.Code)
	}
	for _, bad := range []string{"", "not-an-id", ts.p2pNode().ID()} {
		if w := ts.do(t, "POST", "/api/p2p/devices", bob, map[string]any{"id": bad}); w.Code != 400 {
			t.Fatalf("enrolling %q: %d", bad, w.Code)
		}
	}
	if w := ts.do(t, "POST", "/api/p2p/devices", "", map[string]any{"id": app.ID()}); w.Code != 401 {
		t.Fatalf("enrolling without signing in: %d", w.Code)
	}

	// Revoked: its open connection is closed, and it doesn't get back in.
	if w := ts.do(t, "DELETE", "/api/p2p/devices/"+app.ID(), admin, nil); w.Code != 200 {
		t.Fatalf("the admin removing it: %d %s", w.Code, w.Body)
	}
	if _, _, err := over(t, app, "GET", "/api/capabilities", "", nil); err == nil {
		t.Fatal("a revoked device was answered")
	}
}

func TestP2PDeviceLimitAndDeletedAccounts(t *testing.T) {
	ts, admin := p2pServer(t, nil)
	u, kate := ts.user(t, "kate", "adult", false, "")
	for i := range p2pMaxDevicesEach {
		id, _ := p2p.IDFromSeed(p2pSeedFor(byte(100 + i)))
		if w := ts.do(t, "POST", "/api/p2p/devices", kate, map[string]any{"id": id, "name": "d"}); w.Code != 200 {
			t.Fatalf("device %d: %d %s", i, w.Code, w.Body)
		}
	}
	extra, _ := p2p.IDFromSeed(p2pSeedFor(99))
	if w := ts.do(t, "POST", "/api/p2p/devices", kate, map[string]any{"id": extra}); w.Code != 409 {
		t.Fatalf("one device too many: %d", w.Code)
	}
	// Enrolling the same device again only renames it.
	first, _ := p2p.IDFromSeed(p2pSeedFor(100))
	if w := ts.do(t, "POST", "/api/p2p/devices", kate, map[string]any{"id": first, "name": "renamed"}); w.Code != 200 || !strings.Contains(w.Body.String(), "renamed") {
		t.Fatalf("enrolling again: %d %s", w.Code, w.Body)
	}
	// The account goes, and its devices with it.
	if w := ts.do(t, "DELETE", "/api/users/"+itoa64(u.ID), admin, map[string]any{"current": "correct-horse-1"}); w.Code != 200 {
		t.Fatalf("deleting the account: %d %s", w.Code, w.Body)
	}
	if ts.p2p.devices.Allowed(first) {
		t.Fatal("a deleted account's device is still let in")
	}
}

func TestP2PInviteEnrolsAndSignsIn(t *testing.T) {
	ts, admin := p2pServer(t, nil)
	_, kid := ts.user(t, "kid", "kids", false, "")
	if w := ts.do(t, "POST", "/api/p2p/invite", kid, map[string]any{}); w.Code != 403 {
		t.Fatalf("a kid's profile making an invite: %d", w.Code)
	}
	// An invite signs a device in as the administrator wherever it is: that takes the
	// administrator's password again, as approving a device at home does.
	for _, c := range []struct {
		name string
		body map[string]any
		code int
	}{
		{"no password", map[string]any{}, 403},
		{"a wrong password", map[string]any{"password": "not the password"}, 403},
		{"another account's managed profile", map[string]any{"userId": 999999, "password": "correct-horse-1"}, 403},
	} {
		w := ts.do(t, "POST", "/api/p2p/invite", admin, c.body)
		if w.Code != c.code {
			t.Fatalf("%s: %d %s", c.name, w.Code, w.Body)
		}
		if c.name == "no password" && !strings.Contains(w.Body.String(), `"reason":"`+pairNeedsPassword+`"`) {
			t.Fatalf("%s: %s", c.name, w.Body)
		}
	}
	ts.p2p.invites.mu.Lock()
	n := len(ts.p2p.invites.open)
	ts.p2p.invites.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d invites were made without the password", n)
	}
	w := ts.do(t, "POST", "/api/p2p/invite", admin, map[string]any{"password": "correct-horse-1"})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var made struct {
		Link         string
		ExpiresInSec int
	}
	_ = json.Unmarshal(w.Body.Bytes(), &made)
	in, err := p2p.ParseInvite(made.Link)
	if err != nil || in.ServerID != ts.p2pNode().ID() || made.ExpiresInSec != 900 {
		t.Fatalf("the invite: %+v %v", made, err)
	}

	app := p2pApp(t, ts, 2)
	ctx := context.Background()
	// A guess at the secret gets nowhere, and doesn't use the invite up.
	if res, err := app.Enrol(ctx, "AAAAAAAAAAAAAAAAAAAAAA", "Phone", "android"); err != nil || res.OK || res.Reason != "wrong" {
		t.Fatalf("a wrong secret: %+v %v", res, err)
	}
	res, err := app.Enrol(ctx, in.Secret, "Phone\n\x00 of mine", "android")
	if err != nil || !res.OK || len(res.Code) != 6 {
		t.Fatalf("the right secret: %+v %v", res, err)
	}
	// The code it was given signs it in, over the new connection, as the
	// account the invite was made for.
	code, body, err := over(t, app, "POST", "/api/pair/redeem", "", map[string]any{"code": res.Code, "device": "Phone"})
	if err != nil || code != 200 {
		t.Fatalf("redeeming the code: %d %s %v", code, body, err)
	}
	var signed struct {
		Token string
		User  struct{ Name string }
	}
	_ = json.Unmarshal([]byte(body), &signed)
	if signed.Token == "" || signed.User.Name != "owner" {
		t.Fatalf("signed in as: %s", body)
	}
	if code, _, err := over(t, app, "GET", "/api/p2p", signed.Token, nil); err != nil || code != 200 {
		t.Fatalf("a signed-in request: %d %v", code, err)
	}
	var list []p2pDevice
	_ = json.Unmarshal(ts.do(t, "GET", "/api/p2p/devices", admin, nil).Body.Bytes(), &list)
	if len(list) != 1 || list[0].How != "invite" || list[0].Platform != "android" || strings.ContainsAny(list[0].Name, "\n\x00") {
		t.Fatalf("the device on the list: %+v", list)
	}

	// The invite works once.
	other := p2pApp(t, ts, 3)
	if res, _ := other.Enrol(ctx, in.Secret, "Other", "desktop"); res.OK || res.Reason != "used" {
		t.Fatalf("a second use: %+v", res)
	}
	// An invite dies with the sign-in that made it.
	_, kate := ts.user(t, "kate", "adult", false, "")
	w = ts.do(t, "POST", "/api/p2p/invite", kate, map[string]any{})
	_ = json.Unmarshal(w.Body.Bytes(), &made)
	in2, _ := p2p.ParseInvite(made.Link)
	ts.Auth.Logout(ctx, kate)
	if res, _ := other.Enrol(ctx, in2.Secret, "Other", "desktop"); res.OK || res.Reason != "expired" {
		t.Fatalf("an invite whose maker signed out: %+v", res)
	}
	if ts.p2p.devices.Allowed(other.ID()) {
		t.Fatal("a refused invite still put the device on the list")
	}
}

func TestP2PSettingsAreForTheOwner(t *testing.T) {
	ts := newTestServer(t)
	router := &portmap.FakeRouter{External: netip.MustParseAddr("198.51.100.20")}
	ts.p2p.tune = func(c *p2p.NodeConfig) {
		c.Local, c.Listen = true, p2pLoopback
		c.Layers = p2p.Layers{PortMap: c.Layers.PortMap}
		c.Mapper = portmap.NewFakeHolder(router)
	}
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, kate := ts.user(t, "kate", "adult", false, "")
	ts.StartP2P(context.Background(), ts.h)
	t.Cleanup(ts.StopP2P)

	// Off until turned on: no node, no key made, the router asked for nothing.
	if ts.p2pNode() != nil || ts.DB.GetSetting(context.Background(), p2pSeedKey, "") != "" {
		t.Fatal("something was started with the feature off")
	}
	if w := ts.do(t, "GET", "/api/p2p", kate, nil); strings.TrimSpace(w.Body.String()) != `{"enabled":false}` {
		t.Fatalf("off, an app is told: %s", w.Body)
	}
	if w := ts.do(t, "POST", "/api/p2p/invite", admin, map[string]any{}); w.Code != 409 {
		t.Fatalf("an invite with it off: %d", w.Code)
	}
	time.Sleep(100 * time.Millisecond)
	if len(router.Open()) != 0 {
		t.Fatalf("the router was asked for a mapping with the feature off: %v", router.Open())
	}

	for _, route := range [][2]string{{"GET", "/api/p2p/settings"}, {"PUT", "/api/p2p/settings"}, {"GET", "/api/p2p/status"}, {"POST", "/api/p2p/test"}, {"POST", "/api/p2p/rotate"}} {
		if w := ts.do(t, route[0], route[1], kate, map[string]any{"enabled": true, "accept": true}); w.Code != 403 {
			t.Errorf("%s %s for a non-admin: %d", route[0], route[1], w.Code)
		}
		if w := ts.do(t, route[0], route[1], "", map[string]any{}); w.Code != 401 {
			t.Errorf("%s %s signed out: %d", route[0], route[1], w.Code)
		}
	}
	if w := ts.do(t, "GET", "/api/p2p", "", nil); w.Code != 401 {
		t.Fatalf("the server's ID is for signed-in accounts: %d", w.Code)
	}

	// Turning it on takes saying so.
	if w := ts.do(t, "PUT", "/api/p2p/settings", admin, map[string]any{"enabled": true}); w.Code != 400 || ts.p2pNode() != nil {
		t.Fatalf("turned on without accepting: %d", w.Code)
	}
	for _, bad := range []map[string]any{
		{"enabled": true, "accept": true, "port": 80},
		{"enabled": true, "accept": true, "relayKbps": 10},
		{"enabled": true, "accept": true, "staticRelays": []string{"/ip4/203.0.113.4/tcp/4001"}},
		{"enabled": true, "accept": true, "staticRelays": []string{"/ip4/203.0.113.4/tcp/4001/p2p/nonsense"}},
	} {
		if w := ts.do(t, "PUT", "/api/p2p/settings", admin, bad); w.Code != 400 {
			t.Errorf("%v was accepted: %d", bad, w.Code)
		}
	}
	w := ts.do(t, "PUT", "/api/p2p/settings", admin, map[string]any{"enabled": true, "accept": true, "layers": map[string]any{"portMap": true}})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var v p2pView
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	if !v.Running || v.ServerID == "" || v.Settings.Port < 20000 || !v.Settings.DevicesOnly || len(v.ThirdParties) == 0 || len(v.Steps) == 0 {
		t.Fatalf("after turning on: %+v", v)
	}
	if strings.Contains(w.Body.String(), ts.DB.GetSetting(context.Background(), p2pSeedKey, "x")) {
		t.Fatal("the answer holds the server's key")
	}
	// Now, and only now, the router was asked.
	for i := 0; i < 100 && len(router.Open()) != 2; i++ {
		time.Sleep(30 * time.Millisecond)
	}
	if len(router.Open()) != 2 {
		t.Fatalf("mapped on the router: %v", router.Open())
	}
	// Letting any device in takes saying so as well.
	if w := ts.do(t, "PUT", "/api/p2p/settings", admin, map[string]any{"devicesOnly": false}); w.Code != 400 {
		t.Fatalf("opened to any device without accepting: %d", w.Code)
	}
	// The key is replaced only with the owner's password, and the ID changes.
	if w := ts.do(t, "POST", "/api/p2p/rotate", admin, map[string]any{"password": "wrong"}); w.Code == 200 {
		t.Fatal("the key was replaced with a wrong password")
	}
	w = ts.do(t, "POST", "/api/p2p/rotate", admin, map[string]any{"password": "correct-horse-1"})
	var v2 p2pView
	_ = json.Unmarshal(w.Body.Bytes(), &v2)
	if w.Code != 200 || !v2.Running || v2.ServerID == v.ServerID || v2.Settings.Port != v.Settings.Port {
		t.Fatalf("after a new key: %d %+v", w.Code, v2)
	}
	// Turned off, what was asked of the router is taken back.
	if w := ts.do(t, "PUT", "/api/p2p/settings", admin, map[string]any{"enabled": false}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if len(router.Open()) != 0 {
		t.Fatalf("left on the router after turning off: %v", router.Open())
	}
}

func TestP2PConnectionTest(t *testing.T) {
	ts, admin := p2pServer(t, nil)
	w := ts.do(t, "POST", "/api/p2p/test", admin, nil)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var out struct{ Steps []checkStep }
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if len(out.Steps) != 3 {
		t.Fatalf("steps: %+v", out.Steps)
	}
	for _, s := range out.Steps {
		if s.State != "ok" {
			t.Errorf("%s: %s: %s", s.Name, s.State, s.Detail)
		}
	}
	if !strings.Contains(out.Steps[2].Detail, "away from home") {
		t.Errorf("the request step: %s", out.Steps[2].Detail)
	}
	// The test's own key was let in for the test only, and is on no list.
	if strings.TrimSpace(ts.do(t, "GET", "/api/p2p/devices?all=1", admin, nil).Body.String()) != "[]" {
		t.Fatal("the test left a device on the list")
	}
}

func TestDescribeP2P(t *testing.T) {
	find := func(steps []checkStep, name string) checkStep {
		for _, s := range steps {
			if s.Name == name {
				return s
			}
		}
		t.Fatalf("no step %q in %+v", name, steps)
		return checkStep{}
	}
	all := p2p.AllLayers()
	cases := []struct {
		name  string
		st    p2p.Status
		step  string
		state string
		says  string
	}{
		{"just started", p2p.Status{Layers: all, Reachability: "unknown"}, "Reachable from outside", "wait", "few minutes"},
		{"proved from outside", p2p.Status{Layers: all, Reachable: []string{"/ip4/198.51.100.2/udp/4100/quic-v1"}}, "Reachable from outside", "ok", "198.51.100.2"},
		{"behind a router", p2p.Status{Layers: all, Reachability: "private"}, "Reachable from outside", "warn", "hole punching"},
		{"cgnat", p2p.Status{Layers: all, Mapping: portmap.HolderStatus{On: true, Shared: true, ExternalIP: "100.70.1.2", Mappings: []portmap.MappingStatus{{Working: true}}}}, "Port on your router", "warn", "CGNAT"},
		{"mapped", p2p.Status{Layers: all, Port: 4100, Mapping: portmap.HolderStatus{On: true, Protocol: "UPnP", ExternalIP: "198.51.100.2", Mappings: []portmap.MappingStatus{{Working: true}}}}, "Port on your router", "ok", "UPnP"},
		{"no upnp", p2p.Status{Layers: all, Mapping: portmap.HolderStatus{On: true, Error: "no router answered"}}, "Port on your router", "warn", "no router answered"},
		{"mapping off", p2p.Status{Layers: p2p.Layers{}}, "Port on your router", "off", "isn't asked"},
		{"symmetric", p2p.Status{Layers: all, NATUDP: "strict", NATTCP: "strict"}, "Hole punching", "warn", "symmetric"},
		{"cone", p2p.Status{Layers: all, NATUDP: "open"}, "Hole punching", "ok", "same outside port"},
		{"dht down", p2p.Status{Layers: all}, "Public address book", "warn", "hasn't been reached"},
		{"dht up", p2p.Status{Layers: all, DHTPeers: 40}, "Public address book", "ok", "40"},
		{"public relays only", p2p.Status{Layers: all, Relays: []p2p.RelayStatus{{ID: "a"}}}, "Relays", "ok", "No community relay"},
		{"community relay", p2p.Status{Layers: all, Relays: []p2p.RelayStatus{{ID: "a", Community: true}}}, "Relays", "ok", "encrypted"},
		{"relays off", p2p.Status{Layers: p2p.Layers{}}, "Relays", "off", "no hole punching"},
		{"no ipv6", p2p.Status{Layers: all, Addrs: []string{"/ip4/192.168.1.2/tcp/1"}}, "IPv6", "off", "no IPv6 address"},
		{"ipv6 shut", p2p.Status{Layers: all, Addrs: []string{"/ip6/2001:db8::2/tcp/1"}, Mapping: portmap.HolderStatus{Pinholes: []portmap.PinholeStatus{{Error: "refused"}}}}, "IPv6", "warn", "refused"},
		{"ipv6 open", p2p.Status{Layers: all, Addrs: []string{"/ip6/2001:db8::2/tcp/1"}, Mapping: portmap.HolderStatus{Pinholes: []portmap.PinholeStatus{{Working: true}}}}, "IPv6", "ok", "agreed"},
	}
	for _, c := range cases {
		got := find(describeP2P(c.st, 1), c.step)
		if got.State != c.state || !strings.Contains(got.Detail, c.says) {
			t.Errorf("%s: %s is %q (%s), want %q mentioning %q", c.name, c.step, got.State, got.Detail, c.state, c.says)
		}
	}
	if s := find(describeP2P(p2p.Status{Layers: all}, 0), "Devices"); s.State != "warn" {
		t.Errorf("no devices: %+v", s)
	}
}

func TestP2PSettingsDefaultsAndValidation(t *testing.T) {
	d := defaultP2P()
	if d.Enabled || !d.DevicesOnly || d.RelayForOthers {
		t.Fatalf("the defaults must be off, devices only, and no relaying for others: %+v", d)
	}
	c := defaultP2P()
	c.Layers.Relay = false
	if err := c.validate(); err != nil || c.Layers.HolePunch {
		t.Fatalf("hole punching without relays: %+v %v", c.Layers, err)
	}
	if got := inviteAddrs([]string{"/ip4/127.0.0.1/tcp/1", "/ip4/1.2.3.4/tcp/1/p2p/x/p2p-circuit", "/ip4/198.51.100.2/tcp/1", "/ip6/::1/tcp/1"}); len(got) != 2 || got[0] != "/ip4/198.51.100.2/tcp/1" {
		t.Fatalf("addresses for an invite: %v", got)
	}
}
