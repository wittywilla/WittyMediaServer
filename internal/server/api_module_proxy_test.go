// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymodule/service"
)

// seen is what a stand-in WittyPlay was sent.
type seen struct {
	Path, Query, User, Admin, Token, Spoof, Cookie, Auth, SystemDir string
}

// fakeWittyPlayService stands in for WittyPlay's service: it records what each request carried,
// and answers a WebSocket upgrade on /kasm/ by echoing what it's sent.
func fakeWittyPlayService(t *testing.T) (got func() []seen) {
	t.Helper()
	var mu sync.Mutex
	var all []seen
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/call/") {
			// The server's own calls to the module, as it starts using it.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"rets":[]}`))
			return
		}
		mu.Lock()
		all = append(all, seen{Path: r.URL.Path, Query: r.URL.RawQuery, User: r.Header.Get("X-Witty-User"), Admin: r.Header.Get("X-Witty-Admin"),
			Token: r.Header.Get(service.TokenHeader), Spoof: r.Header.Get("X-Witty-Anything"), Cookie: r.Header.Get("Cookie"), Auth: r.Header.Get("Authorization"),
			SystemDir: r.Header.Get("X-Wittyplay-System-Dir")})
		mu.Unlock()
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			conn, rw, err := http.NewResponseController(w).Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			defer conn.Close()
			_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
			_ = rw.Flush()
			line, _ := rw.ReadString('\n')
			_, _ = rw.WriteString("echo " + line)
			_ = rw.Flush()
			return
		}
		if strings.HasPrefix(r.URL.Path, "/kasm/") {
			http.SetCookie(w, &http.Cookie{Name: "wittyplay_kasm", Value: "t", Path: "/kasm/abc"})
			http.SetCookie(w, &http.Cookie{Name: auth.CookieName, Value: "stolen", Path: "/"})
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/installed") {
			// A module that misbehaves: a cookie on a JSON route, and far too much to say.
			http.SetCookie(w, &http.Cookie{Name: "tracker", Value: "1", Path: "/"})
			if r.URL.Query().Get("big") == "1" {
				chunk := []byte(strings.Repeat("x", 64<<10))
				for range 80 {
					if _, err := w.Write(chunk); err != nil {
						return
					}
				}
				return
			}
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	base, _ := url.Parse(srv.URL)
	conn := &service.Conn{Base: base, Token: "svc-secret", Client: srv.Client()}
	dial := service.Dial
	service.Dial = func(id string) (*service.Conn, error) {
		if id != wittyPlayID {
			t.Errorf("dialled %q", id)
		}
		return conn, nil
	}
	t.Cleanup(func() { service.Dial = dial })
	return func() []seen {
		mu.Lock()
		defer mu.Unlock()
		return append([]seen(nil), all...)
	}
}

func TestWittyPlayDockerRoutesSayWhoIsAsking(t *testing.T) {
	got := fakeWittyPlayService(t)
	ts := newTestServer(t)
	adminU, admin := ts.user(t, "boss", auth.KindAdult, true, "")
	adultU, adult := ts.user(t, "grown", auth.KindAdult, false, "")
	spoof := []string{"X-Witty-User", "999", "X-Witty-Admin", "1", "X-Witty-Anything", "x", service.TokenHeader, "forged"}

	last := func() seen {
		t.Helper()
		all := got()
		if len(all) == 0 {
			t.Fatal("nothing reached the module")
		}
		return all[len(all)-1]
	}

	// An adult on a player's route: their own id, no admin, whatever they claimed.
	if w := ts.do(t, "DELETE", "/api/modules/wittyplay/docker/sessions/abc", adult, nil, spoof...); w.Code != 200 {
		t.Fatalf("adult stop: %d %s", w.Code, w.Body)
	}
	s := last()
	if s.Path != "/docker/sessions/abc" || s.User != itoa64(adultU.ID) || s.Admin != "" || s.Spoof != "" || s.Token != "svc-secret" || s.Auth != "" {
		t.Errorf("adult: %+v", s)
	}

	// An administrator: their id, and admin.
	if w := ts.do(t, "GET", "/api/v1/modules/wittyplay/docker/status", admin, nil, spoof...); w.Code != 200 {
		t.Fatalf("admin status: %d %s", w.Code, w.Body)
	}
	s = last()
	if s.Path != "/docker/status" || s.User != itoa64(adminU.ID) || s.Admin != "1" || s.Spoof != "" || s.Token != "svc-secret" {
		t.Errorf("admin: %+v", s)
	}

	// The server refuses administrators' routes itself: the module never hears of it.
	n := len(got())
	for _, r := range [][2]string{{"GET", "/docker/status"}, {"PUT", "/docker/settings"}, {"POST", "/docker/emulators/pcsx2/install"}, {"DELETE", "/docker/emulators/pcsx2"}} {
		if w := ts.do(t, r[0], "/api/modules/wittyplay"+r[1], adult, nil, spoof...); w.Code != 403 {
			t.Errorf("%s %s as an adult: %d", r[0], r[1], w.Code)
		}
		if w := ts.do(t, r[0], "/api/modules/wittyplay"+r[1], "", nil, spoof...); w.Code != 401 {
			t.Errorf("%s %s signed out: %d", r[0], r[1], w.Code)
		}
	}
	if len(got()) != n {
		t.Errorf("a refused request reached the module: %+v", got()[n:])
	}

}

const (
	kasmSid    = "0123456789abcdef0123456789abcdef"
	kasmTicket = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	kasmPath   = "/kasm/" + kasmSid + "/" + kasmTicket
)

// TestWittyPlayKasmIsBoxedIn: a desktop's page comes out of a container, so it is never one of
// this server's own pages. It's reached by its ticket, with nobody's sign-in or identity passed
// on; and whatever comes back is sandboxed away from this server's origin, sets no cookie and
// redirects nowhere else.
func TestWittyPlayKasmIsBoxedIn(t *testing.T) {
	got := fakeWittyPlayService(t)
	ts := newTestServer(t)
	_, adult := ts.user(t, "grown", auth.KindAdult, false, "")
	for _, base := range []string{"/api/modules/wittyplay", "/api/v1/modules/wittyplay"} {
		before := len(got())
		w := ts.do(t, "GET", base+kasmPath+"/vnc.html?x=1&token=abc", adult, nil, "Cookie", auth.CookieName+"=sess; other=1", "X-Witty-User", "999", "X-Wittyplay-System-Dir", "/etc")
		if w.Code != 200 {
			t.Fatalf("the desktop: %d %s", w.Code, w.Body)
		}
		all := got()
		if len(all) != before+1 {
			t.Fatalf("the module heard %d requests", len(all)-before)
		}
		s := all[len(all)-1]
		if s.Path != kasmPath+"/vnc.html" || s.Query != "x=1" || s.User != "" || s.Admin != "" || s.Cookie != "" || s.Auth != "" || s.SystemDir != "" || s.Token != "svc-secret" {
			t.Errorf("what the module was sent: %+v", s)
		}
		// The module tried a cookie of its own and this server's sign-in cookie: neither arrives.
		if c := w.Header().Values("Set-Cookie"); len(c) != 0 {
			t.Errorf("cookies from a desktop: %v", c)
		}
		csp := w.Header().Values("Content-Security-Policy")
		if len(csp) != 1 || csp[0] != kasmCSP {
			t.Errorf("the content policy: %q", csp)
		}
	}
	// The sandbox allows scripts and never this server's origin.
	box := kasmCSP[strings.Index(kasmCSP, "sandbox"):]
	if !strings.Contains(box, "allow-scripts") || strings.Contains(box, "allow-same-origin") || strings.Contains(box, "allow-top-navigation") || strings.Contains(box, "allow-popups") || strings.Contains(box, "allow-forms") {
		t.Errorf("the sandbox: %q", box)
	}
	// Signed out is the same as signed in: the ticket is the way in, and the module checks it.
	if w := ts.do(t, "GET", "/api/modules/wittyplay"+kasmPath+"/", "", nil); w.Code != 200 {
		t.Errorf("the desktop, signed out, with its ticket: %d", w.Code)
	}
	// Names that couldn't be a session's or a ticket's never reach the module, and nothing can be written.
	n := len(got())
	for _, p := range []string{"/kasm/abc/vnc.html", "/kasm/" + kasmSid + "/short/x", "/kasm/" + kasmSid + "/" + strings.ToUpper(kasmTicket) + "/x",
		"/kasm/" + kasmSid + "/" + strings.Repeat("g", 64) + "/x", "/kasm/..%2f..%2fdocker%2fstatus/" + kasmTicket + "/x", "/kasm/" + kasmSid + "/"} {
		// Not found, or the router's own redirect to an address that then isn't found.
		if w := ts.do(t, "GET", "/api/modules/wittyplay"+p, adult, nil); w.Code != 404 && w.Code != 307 {
			t.Errorf("GET %s: %d, want 404", p, w.Code)
		}
	}
	for _, m := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if w := ts.do(t, m, "/api/modules/wittyplay"+kasmPath+"/api/x", adult, []byte("{}")); w.Code == 200 {
			t.Errorf("%s to a desktop went through", m)
		}
	}
	if len(got()) != n {
		t.Errorf("a refused request reached the module: %+v", got()[n:])
	}
}

func TestWittyPlayKasmWebSocketPassesThrough(t *testing.T) {
	got := fakeWittyPlayService(t)
	ts := newTestServer(t)
	_, tok := ts.user(t, "player", auth.KindAdult, false, "")
	front := httptest.NewServer(ts.h)
	t.Cleanup(front.Close)
	addr := strings.TrimPrefix(front.URL, "http://")

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	// As the desktop's own page asks: from its sandboxed frame, whose origin is "null".
	req := "GET /api/modules/wittyplay" + kasmPath + "/websockify HTTP/1.1\r\nHost: " + addr + "\r\nOrigin: null\r\n" +
		"Authorization: Bearer " + tok + "\r\nX-Witty-User: 999\r\nX-Witty-Admin: 1\r\n" +
		"Connection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("upgrade: %d %s", resp.StatusCode, b)
	}
	if _, err := io.WriteString(conn, "hello\n"); err != nil {
		t.Fatal(err)
	}
	line, err := br.ReadString('\n')
	if err != nil || line != "echo hello\n" {
		t.Fatalf("echo: %q %v", line, err)
	}
	all := got()
	if len(all) != 1 || all[0].Path != kasmPath+"/websockify" || all[0].User != "" || all[0].Admin != "" || all[0].Auth != "" {
		t.Errorf("the module saw %+v", all)
	}

	// From another site, the upgrade is refused before it reaches the module.
	c2, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	_ = c2.SetDeadline(time.Now().Add(10 * time.Second))
	_, _ = io.WriteString(c2, strings.Replace(req, "Origin: null", "Origin: https://evil.example", 1))
	resp, err = http.ReadResponse(bufio.NewReader(c2), nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 403 {
		t.Errorf("cross-site upgrade: %d", resp.StatusCode)
	}
	if len(got()) != 1 {
		t.Errorf("a cross-site upgrade reached the module")
	}
}

// Without a module to dial, the routes say so, as the others do.
func TestWittyPlayDockerRoutesWithoutTheModule(t *testing.T) {
	dial := service.Dial
	service.Dial = nil
	t.Cleanup(func() { service.Dial = dial })
	ts := newTestServer(t)
	_, admin := ts.user(t, "boss", auth.KindAdult, true, "")
	w := ts.do(t, "GET", "/api/modules/wittyplay/docker/status", admin, nil)
	var b map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	if w.Code != http.StatusFailedDependency || b["error"] == nil {
		t.Errorf("%d %s", w.Code, w.Body)
	}
}

// TestWittyPlayRoutesAreHeldToWhoMayCallThem goes through every route passed to WittyPlay: the
// administrators' ones refuse everyone else before the module hears anything, the rest need a
// sign-in, and what does go through is held to a size.
func TestWittyPlayRoutesAreHeldToWhoMayCallThem(t *testing.T) {
	got := fakeWittyPlayService(t)
	ts := newTestServer(t)
	_, admin := ts.user(t, "boss", auth.KindAdult, true, "")
	_, adult := ts.user(t, "grown", auth.KindAdult, false, "")
	_, kid := ts.user(t, "small", auth.KindKids, false, "")
	adminOnly := [][2]string{
		{"GET", "/docker/status"}, {"GET", "/docker/diagnostics"}, {"GET", "/docker/settings"}, {"PUT", "/docker/settings"},
		{"GET", "/docker/emulators"}, {"GET", "/docker/installed"}, {"POST", "/docker/emulators/pcsx2/install"},
		{"GET", "/docker/emulators/pcsx2/install"}, {"DELETE", "/docker/emulators/pcsx2/install"}, {"DELETE", "/docker/emulators/pcsx2"},
	}
	anyone := [][2]string{
		{"GET", "/docker/emulators/pcsx2/settings"}, {"PUT", "/docker/emulators/pcsx2/settings"}, {"GET", "/docker/sessions"},
		{"POST", "/docker/sessions/abc/connect"}, {"POST", "/docker/sessions/abc/pause"}, {"POST", "/docker/sessions/abc/resume"},
		{"DELETE", "/docker/sessions/abc"},
	}
	for _, base := range []string{"/api/modules/wittyplay", "/api/v1/modules/wittyplay"} {
		for _, r := range adminOnly {
			for who, tok := range map[string]string{"an adult": adult, "a kid": kid} {
				if w := ts.do(t, r[0], base+r[1], tok, nil, "X-Witty-Admin", "1"); w.Code != 403 {
					t.Errorf("%s %s%s as %s: %d, want 403", r[0], base, r[1], who, w.Code)
				}
			}
			if w := ts.do(t, r[0], base+r[1], "", nil, "X-Witty-User", "1", "X-Witty-Admin", "1"); w.Code != 401 {
				t.Errorf("%s %s%s signed out: %d, want 401", r[0], base, r[1], w.Code)
			}
		}
		for _, r := range anyone {
			if w := ts.do(t, r[0], base+r[1], "", nil, "X-Witty-User", "1"); w.Code != 401 {
				t.Errorf("%s %s%s signed out: %d, want 401", r[0], base, r[1], w.Code)
			}
		}
	}
	if n := len(got()); n != 0 {
		t.Fatalf("%d refused requests reached the module: %+v", n, got())
	}
	// Starting a game isn't passed through at all: only the server's own route starts one.
	for _, p := range []string{"/docker/sessions", "/docker/launch", "/call/docker.Launch", "/docker/play"} {
		if w := ts.do(t, "POST", "/api/modules/wittyplay"+p, admin, map[string]string{"ROM": "/etc/passwd"}); w.Code == 200 || w.Code == 201 {
			t.Errorf("POST %s reached something: %d", p, w.Code)
		}
	}
	if n := len(got()); n != 0 {
		t.Fatalf("a route that isn't listed reached the module: %+v", got())
	}
	// Signed in, the rest go through, as the caller.
	for _, r := range adminOnly {
		if w := ts.do(t, r[0], "/api/modules/wittyplay"+r[1], admin, nil); w.Code != 200 {
			t.Errorf("%s %s as an admin: %d", r[0], r[1], w.Code)
		}
	}
	for _, r := range anyone {
		if w := ts.do(t, r[0], "/api/modules/wittyplay"+r[1], kid, nil); w.Code != 200 {
			t.Errorf("%s %s signed in: %d", r[0], r[1], w.Code)
		}
	}
	// The BIOS folder is the server's to say: a client's word for it is replaced.
	ts.do(t, "GET", "/api/modules/wittyplay/docker/emulators", admin, nil, "X-Wittyplay-System-Dir", "/etc")
	all := got()
	if dir := all[len(all)-1].SystemDir; dir == "" || dir == "/etc" || !strings.Contains(dir, "gaming") {
		t.Errorf("the BIOS folder WittyPlay was told: %q", dir)
	}

	// Sizes. A request larger than any of these routes takes is refused before it's passed on.
	n := len(got())
	big := []byte(`{"values":{"x":"` + strings.Repeat("a", 300<<10) + `"}}`)
	if w := ts.do(t, "PUT", "/api/modules/wittyplay/docker/emulators/pcsx2/settings", adult, big); w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("a 300 KB request: %d", w.Code)
	}
	if len(got()) != n {
		t.Error("an oversize request reached the module")
	}
	// An answer larger than any of them gives is cut off, and a module's cookie never arrives.
	w := ts.do(t, "GET", "/api/modules/wittyplay/docker/installed?big=1", admin, nil)
	if w.Body.Len() > wittyPlayMaxResponse {
		t.Errorf("an oversize answer came through: %d bytes", w.Body.Len())
	}
	w = ts.do(t, "GET", "/api/modules/wittyplay/docker/installed", admin, nil)
	if w.Code != 200 || len(w.Result().Cookies()) != 0 || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("a JSON route's answer: %d, cookies %v", w.Code, w.Header().Values("Set-Cookie"))
	}
}
