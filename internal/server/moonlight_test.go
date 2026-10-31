package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymoonlight/gamestream"
	"github.com/wittywilla/wittymoonlight/gamestream/fake"
)

type moonHost = moonlightHostView

// addMoonlight adds the fake as someone's PC. Tests reach it on this machine, which an ordinary
// account never may, so that one rule is opened up for the test that asks.
func addMoonlight(t *testing.T, ts *testServer, f *fake.Sunshine, token string) moonHost {
	t.Helper()
	w := ts.do(t, "POST", "/api/moonlight/hosts", token, map[string]any{"address": fmt.Sprintf("127.0.0.1:%d", f.Port())})
	if w.Code != 201 {
		t.Fatalf("add: %d %s", w.Code, w.Body)
	}
	return rigJSON[moonHost](t, w.Code, w.Body.Bytes())
}

func pairMoonlight(t *testing.T, ts *testServer, f *fake.Sunshine, token string, id int64) {
	t.Helper()
	url := fmt.Sprintf("/api/moonlight/hosts/%d/pair", id)
	w := ts.do(t, "POST", url, token, nil)
	if w.Code != 200 {
		t.Fatalf("pair: %d %s", w.Code, w.Body)
	}
	p := rigJSON[rigPairing](t, w.Code, w.Body.Bytes())
	if !f.WaitForPairing(5 * time.Second) {
		t.Fatal("nothing started pairing")
	}
	f.TypePIN(p.PIN)
	deadline := time.Now().Add(10 * time.Second)
	for {
		st := rigJSON[rigPairing](t, 200, ts.do(t, "GET", url, token, nil).Body.Bytes())
		if st.State == "paired" {
			return
		}
		if st.State == "failed" || time.Now().After(deadline) {
			t.Fatalf("pairing: %+v", st)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// testAllowLoopback lets ordinary accounts reach the fake, for tests of everything else.
func testAllowLoopback(t *testing.T) {
	t.Helper()
	moonlightAllowLoopback = true
	t.Cleanup(func() { moonlightAllowLoopback = false })
}

func TestMoonlightWhoMayUseIt(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, adult := ts.user(t, "adult1", "adult", false, "")
	_, kid := ts.user(t, "kid1", "kids", false, "")
	_, guest := ts.user(t, "guest1", "guest", false, "")
	limits, _ := json.Marshal(library.Access{MaxAge: 13})
	_, limited := ts.user(t, "adult2", "adult", false, string(limits))

	allowed := func(tok string) bool {
		var v struct {
			Allowed bool `json:"allowed"`
		}
		if err := json.Unmarshal(ts.do(t, "GET", "/api/moonlight", tok, nil).Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		return v.Allowed
	}
	if !allowed(admin) || !allowed(adult) {
		t.Error("admins and adults start with it on")
	}
	if allowed(kid) || allowed(guest) || allowed(limited) {
		t.Error("kids, guests and limited accounts start without it")
	}
	if w := ts.do(t, "GET", "/api/moonlight", "", nil); w.Code != 401 {
		t.Errorf("signed out: %d", w.Code)
	}
	for _, tok := range []string{kid, guest, limited} {
		for _, c := range [][2]string{{"GET", "/api/moonlight/hosts"}, {"POST", "/api/moonlight/hosts"}, {"GET", "/api/moonlight/find"}, {"POST", "/api/moonlight/hosts/1/pair"}, {"POST", "/api/moonlight/hosts/1/apps/1/play"}} {
			if w := ts.do(t, c[0], c[1], tok, map[string]any{"address": "192.168.1.2"}); w.Code != 403 {
				t.Errorf("%s %s: %d", c[0], c[1], w.Code)
			}
		}
	}
	// The admin's setting changes who.
	if err := ts.DB.SetSetting(context.Background(), "moonlightAccess", "admins"); err != nil {
		t.Fatal(err)
	}
	if allowed(adult) || !allowed(admin) {
		t.Error("admins only")
	}
	if err := ts.DB.SetSetting(context.Background(), "moonlightAccess", "everyone"); err != nil {
		t.Fatal(err)
	}
	if !allowed(kid) {
		t.Error("everyone")
	}
}

func TestMoonlightAddressRules(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, adult := ts.user(t, "adult1", "adult", false, "")
	ctx := context.Background()
	add := func(tok, addr string) (int, string) {
		w := ts.do(t, "POST", "/api/moonlight/hosts", tok, map[string]any{"address": addr})
		return w.Code, w.Body.String()
	}
	// Never, for anyone.
	for _, a := range []string{"169.254.169.254", "224.0.0.251", "0.0.0.0", "fe80::1", "http://192.168.1.2", "192.168.1.2/admin", "user@pc", "pc?x=1", "::ffff:169.254.169.254"} {
		if code, body := add(admin, a); code != 400 {
			t.Errorf("admin %q: %d %s", a, code, body)
		}
		if code, body := add(adult, a); code != 400 {
			t.Errorf("adult %q: %d %s", a, code, body)
		}
	}
	// This machine: only an admin. It's the way to whatever else listens here.
	for _, a := range []string{"127.0.0.1", "::1", "localhost", "127.0.0.2"} {
		if code, body := add(adult, a); code != 400 || !strings.Contains(body, "this server") {
			t.Errorf("%q: %d %s", a, code, body)
		}
	}
	// Other ports are off unless an admin allows them, as they make a port scanner.
	if code, body := add(adult, "192.168.1.2:22"); code != 400 || !strings.Contains(body, "usual port") {
		t.Errorf("another port: %d %s", code, body)
	}
	// An admin can turn off either kind of network.
	_ = ts.DB.SetSetting(ctx, "moonlightPrivate", "0")
	if code, body := add(adult, "192.168.1.2"); code != 400 || !strings.Contains(body, "own network") {
		t.Errorf("private off: %d %s", code, body)
	}
	_ = ts.DB.SetSetting(ctx, "moonlightPrivate", "1")
	_ = ts.DB.SetSetting(ctx, "moonlightInternet", "0")
	if code, body := add(adult, "8.8.8.8"); code != 400 || !strings.Contains(body, "internet") {
		t.Errorf("internet off: %d %s", code, body)
	}
	// Nothing was stored, and the connection policy agrees.
	var n int
	_ = ts.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM wittymoonlight_hosts`).Scan(&n)
	if n != 0 {
		t.Errorf("%d stored", n)
	}
	rig := &gamestream.Rig{Address: "127.0.0.1", Port: 1, Policy: gamestream.Policy{DenyLoopback: true}}
	if _, err := rig.Hello(ctx); err == nil || !strings.Contains(err.Error(), "isn't allowed") {
		t.Errorf("connecting to this machine: %v", err)
	}
	rig = &gamestream.Rig{Address: "192.168.1.2", Port: 1, Policy: gamestream.Policy{DenyPrivate: true}}
	if _, err := rig.Hello(ctx); err == nil || !strings.Contains(err.Error(), "isn't allowed") {
		t.Errorf("connecting to a private address: %v", err)
	}
}

func TestMoonlightAnswersGiveNothingAway(t *testing.T) {
	testAllowLoopback(t)
	ts := newTestServer(t)
	_, adult := ts.user(t, "adult1", "adult", false, "")
	_ = ts.DB.SetSetting(context.Background(), "moonlightCustomPorts", "1")
	// Something listening that isn't Sunshine, and nothing listening at all, read the same.
	f := rigFake(t)
	f.Garbage = true
	wa := ts.do(t, "POST", "/api/moonlight/hosts", adult, map[string]any{"address": fmt.Sprintf("127.0.0.1:%d", f.Port())})
	f.Garbage = false
	f.Close()
	wb := ts.do(t, "POST", "/api/moonlight/hosts", adult, map[string]any{"address": fmt.Sprintf("127.0.0.1:%d", f.Port())})
	if wa.Code != 502 || wb.Code != 502 || wa.Body.String() != wb.Body.String() {
		t.Errorf("a printer: %d %s; nothing: %d %s", wa.Code, wa.Body, wb.Code, wb.Body)
	}
	if strings.Contains(wa.Body.String(), "printer") || strings.Contains(wa.Body.String(), "<html") {
		t.Errorf("a far end's words came back: %s", wa.Body)
	}
}

func TestMoonlightRateLimit(t *testing.T) {
	testAllowLoopback(t)
	ts := newTestServer(t)
	_, adult := ts.user(t, "adult1", "adult", false, "")
	_ = ts.DB.SetSetting(context.Background(), "moonlightCustomPorts", "1")
	f := rigFake(t)
	f.Garbage = true
	limited := false
	for range moonlightAttemptsLimit + 3 {
		w := ts.do(t, "POST", "/api/moonlight/hosts", adult, map[string]any{"address": fmt.Sprintf("127.0.0.1:%d", f.Port())})
		if w.Code == 429 {
			limited = true
			if w.Header().Get("Retry-After") == "" {
				t.Error("no Retry-After")
			}
			break
		}
	}
	if !limited {
		t.Error("attempts were never limited")
	}
	// It's per person.
	_, other := ts.user(t, "adult2", "adult", false, "")
	if w := ts.do(t, "POST", "/api/moonlight/hosts", other, map[string]any{"address": fmt.Sprintf("127.0.0.1:%d", f.Port())}); w.Code == 429 {
		t.Error("another account was limited too")
	}
}

func TestMoonlightEachPersonsOwn(t *testing.T) {
	testAllowLoopback(t)
	ts := newTestServer(t)
	_ = ts.DB.SetSetting(context.Background(), "moonlightCustomPorts", "1")
	_, a := ts.user(t, "adult1", "adult", false, "")
	ua, _ := ts.Auth.User(context.Background(), 1)
	_, b := ts.user(t, "adult2", "adult", false, "")
	f := rigFake(t)
	h := addMoonlight(t, ts, f, a)
	pairMoonlight(t, ts, f, a, h.ID)

	// The owner sees their PC, with its games, and no secrets.
	w := ts.do(t, "GET", "/api/moonlight/hosts", a, nil)
	list := rigJSON[[]moonHost](t, w.Code, w.Body.Bytes())
	if len(list) != 1 || !list[0].Paired || list[0].State != "online" {
		t.Fatalf("list %+v", list)
	}
	if strings.Contains(w.Body.String(), "BEGIN") || strings.Contains(w.Body.String(), f.MAC) {
		t.Errorf("a secret: %s", w.Body)
	}
	base := fmt.Sprintf("/api/moonlight/hosts/%d", h.ID)
	w = ts.do(t, "GET", base+"/apps", a, nil)
	apps := rigJSON[[]moonlightApp](t, w.Code, w.Body.Bytes())
	if len(apps) != 3 {
		t.Fatalf("apps %+v", apps)
	}
	for _, app := range apps {
		switch app.Title {
		case "Desktop":
			if !app.IsDesktop || app.HasArt {
				t.Errorf("%+v", app)
			}
		case "Space Game":
			if app.IsDesktop || !app.HasArt {
				t.Errorf("%+v", app)
			}
		}
	}
	w = ts.do(t, "GET", fmt.Sprintf("%s/apps/7/art", base), a, nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("art: %d %v", w.Code, w.Header())
	}

	// Nobody else can see or use it, and it looks as if it isn't there.
	if got := rigJSON[[]moonHost](t, 200, ts.do(t, "GET", "/api/moonlight/hosts", b, nil).Body.Bytes()); len(got) != 0 {
		t.Errorf("another account sees %+v", got)
	}
	for _, c := range [][2]string{{"GET", base + "/apps"}, {"GET", base + "/apps/7/art"}, {"POST", base + "/apps/7/play"}, {"POST", base + "/pair"}, {"GET", base + "/pair"}, {"DELETE", base}, {"PATCH", base}, {"POST", base + "/wake"}, {"POST", base + "/quit"}} {
		if w := ts.do(t, c[0], c[1], b, map[string]any{"name": "mine"}); w.Code != 404 {
			t.Errorf("%s %s by another account: %d", c[0], c[1], w.Code)
		}
	}
	// Each person's certificate is different, so one pairing can't be used by anyone else. The
	// other account adding the same PC isn't paired with it.
	hb := addMoonlight(t, ts, f, b)
	if hb.Paired || hb.State != "not-paired" {
		t.Errorf("the same PC for another account: %+v", hb)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/moonlight/hosts/%d/apps", hb.ID), b, nil); w.Code != 502 {
		t.Errorf("apps with no pairing: %d", w.Code)
	}
	ia, _ := ts.moonlightIdentity(1)
	ib, _ := ts.moonlightIdentity(2)
	if ia.ID == ib.ID || string(ia.CertPEM) == string(ib.CertPEM) {
		t.Error("two accounts share a certificate")
	}
	_ = ua

	// Removing it takes it away.
	if w := ts.do(t, "DELETE", base, a, nil); w.Code != 200 || !strings.Contains(w.Body.String(), "paired devices") {
		t.Errorf("remove: %d %s", w.Code, w.Body)
	}
}

func TestMoonlightNoProgramSaysSo(t *testing.T) {
	testAllowLoopback(t)
	ts := newTestServer(t)
	_ = ts.DB.SetSetting(context.Background(), "moonlightCustomPorts", "1")
	t.Setenv("WMS_WITTYMOONLIGHT_HOST", "")
	t.Setenv("PATH", t.TempDir())
	_, a := ts.user(t, "adult1", "adult", false, "")
	f := rigFake(t)
	h := addMoonlight(t, ts, f, a)
	pairMoonlight(t, ts, f, a, h.ID)
	w := ts.do(t, "POST", fmt.Sprintf("/api/moonlight/hosts/%d/apps/7/play", h.ID), a, map[string]any{})
	if w.Code != 503 || !strings.Contains(w.Body.String(), `"needProgram":true`) {
		t.Errorf("%d %s", w.Code, w.Body)
	}
	if launches, _ := f.Counts(); launches != 0 {
		t.Error("a game was started with nothing to show it")
	}
}

func TestMoonlightStreamsThroughTheProgram(t *testing.T) {
	testAllowLoopback(t)
	ts := newTestServer(t)
	_ = ts.DB.SetSetting(context.Background(), "moonlightCustomPorts", "1")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WMS_WITTYMOONLIGHT_HOST", self)
	prepareMoonlightHost = fakeHostEnv
	t.Cleanup(func() { prepareMoonlightHost = realPrepareMoonlightHost; ts.StopMoonlight() })
	_, a := ts.user(t, "adult1", "adult", false, "")
	_, b := ts.user(t, "adult2", "adult", false, "")
	f := rigFake(t)
	h := addMoonlight(t, ts, f, a)
	pairMoonlight(t, ts, f, a, h.ID)
	play := fmt.Sprintf("/api/moonlight/hosts/%d/apps/7/play", h.ID)

	// Numbers from a browser are kept to something sensible on their way to a PC's encoder.
	w := ts.do(t, "POST", play, a, map[string]any{"width": 99999, "height": 1, "fps": 100000, "bitrate": 1})
	if w.Code != 200 {
		t.Fatalf("play: %d %s", w.Code, w.Body)
	}
	st := rigJSON[moonlightSessionView](t, w.Code, w.Body.Bytes())
	if st.ID == "" || st.Title != "Space Game" {
		t.Errorf("%+v", st)
	}
	if f.Running() != 7 {
		t.Errorf("running %d", f.Running())
	}
	if !strings.Contains(f.LastLaunch(), "mode=3840x360x120") {
		t.Errorf("sent %s", f.LastLaunch())
	}
	// A stream is its owner's alone.
	url := "/api/moonlight/sessions/" + st.ID
	for _, c := range [][2]string{{"GET", url}, {"POST", url + "/webrtc"}, {"DELETE", url}} {
		if w := ts.do(t, c[0], c[1], b, map[string]any{"sdp": "x"}); w.Code != 404 {
			t.Errorf("%s by another account: %d", c[0], w.Code)
		}
	}
	if w := ts.do(t, "GET", url, a, nil); w.Code != 200 {
		t.Errorf("own stream: %d", w.Code)
	}
	if w := ts.do(t, "POST", url+"/webrtc", a, map[string]any{"sdp": "not an offer"}); w.Code != 502 {
		t.Errorf("a bad offer: %d %s", w.Code, w.Body)
	}
	// Starting it again resumes, as it's already running. Starting something else is refused.
	if w := ts.do(t, "POST", play, a, map[string]any{}); w.Code != 200 {
		t.Errorf("again: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", fmt.Sprintf("/api/moonlight/hosts/%d/apps/%d/play", h.ID, f.Apps[1].ID), a, map[string]any{}); w.Code != 409 {
		t.Errorf("another game: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", fmt.Sprintf("/api/moonlight/hosts/%d/apps/12345/play", h.ID), a, map[string]any{}); w.Code != 404 {
		t.Errorf("no such app: %d", w.Code)
	}
	// Ending it, and the game with it.
	g := ts.moonlight()
	g.mu.Lock()
	var sid string
	for id := range g.sessions {
		sid = id
	}
	g.mu.Unlock()
	if w := ts.do(t, "DELETE", "/api/moonlight/sessions/"+sid+"?quit=1", a, nil); w.Code != 200 {
		t.Errorf("end: %d %s", w.Code, w.Body)
	}
	if f.Running() != 0 {
		t.Error("the game is still running")
	}
}

func TestSessionAddressIsRebuilt(t *testing.T) {
	good, err := cleanSessionURL("rtspenc://10.0.0.9:48010", "192.168.1.2")
	if err != nil || good != "rtspenc://192.168.1.2:48010" {
		t.Errorf("%q %v", good, err)
	}
	if v, err := cleanSessionURL("rtsp://[fd00::1]:48010", "fd00::1"); err != nil || v != "rtsp://[fd00::1]:48010" {
		t.Errorf("%q %v", v, err)
	}
	for _, bad := range []string{"", "http://x:48010", "rtsp://x", "rtsp://x:22", "rtsp://x:99999", "rtsp://x:48010/../..", "rtsp://x:48010?y=1", "file:///etc/passwd", "rtsp://x:48010\nHost: evil"} {
		if v, err := cleanSessionURL(bad, "192.168.1.2"); err == nil {
			t.Errorf("%q became %q", bad, v)
		}
	}
	ctx := context.Background()
	if _, err := resolveAllowed(ctx, "169.254.169.254", gamestream.Policy{AllowPublic: true}); err == nil {
		t.Error("the metadata address")
	}
	if _, err := resolveAllowed(ctx, "8.8.8.8", gamestream.Policy{}); err == nil {
		t.Error("the internet, when it isn't allowed")
	}
	if ip, err := resolveAllowed(ctx, "192.168.1.2", gamestream.Policy{}); err != nil || ip != "192.168.1.2" {
		t.Errorf("%q %v", ip, err)
	}
}

func TestSunshineIsOffUntilAsked(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, adult := ts.user(t, "adult1", "adult", false, "")
	if w := ts.do(t, "GET", "/api/sunshine", adult, nil); w.Code != 403 {
		t.Errorf("as an adult: %d", w.Code)
	}
	if w := ts.do(t, "PUT", "/api/sunshine", adult, map[string]any{"enabled": true}); w.Code != 403 {
		t.Errorf("turning it on as an adult: %d", w.Code)
	}
	w := ts.do(t, "GET", "/api/sunshine", admin, nil)
	var st struct {
		Enabled bool   `json:"enabled"`
		License string `json:"license"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil || st.Enabled || st.License != "GPL-3.0" {
		t.Errorf("%s %v", w.Body, err)
	}
	if w := ts.do(t, "PUT", "/api/sunshine", admin, map[string]any{"enabled": true}); w.Code != 200 || !strings.Contains(w.Body.String(), `"enabled":true`) {
		t.Errorf("on: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "PUT", "/api/sunshine", admin, map[string]any{"enabled": false}); w.Code != 200 || !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Errorf("off: %d %s", w.Code, w.Body)
	}
}
