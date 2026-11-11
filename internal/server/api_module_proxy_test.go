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
	Path, Query, User, Admin, Token, Spoof, Cookie, Auth string
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
			Token: r.Header.Get(service.TokenHeader), Spoof: r.Header.Get("X-Witty-Anything"), Cookie: r.Header.Get("Cookie"), Auth: r.Header.Get("Authorization")})
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
	if w := ts.do(t, "GET", "/api/modules/wittyplay/kasm/abc/", "", nil, spoof...); w.Code != 401 {
		t.Errorf("kasm signed out: %d", w.Code)
	}
	if len(got()) != n {
		t.Errorf("a refused request reached the module: %+v", got()[n:])
	}

	// The desktop: the ticket goes through, sign-in doesn't, and cookies land under the outside path.
	w := ts.do(t, "GET", "/api/modules/wittyplay/kasm/abc/vnc.html?ticket=T", adult, nil, "Cookie", auth.CookieName+"=sess; other=1")
	if w.Code != 200 {
		t.Fatalf("kasm: %d %s", w.Code, w.Body)
	}
	s = last()
	if s.Path != "/kasm/abc/vnc.html" || s.Query != "ticket=T" || s.User != itoa64(adultU.ID) || strings.Contains(s.Cookie, auth.CookieName) || !strings.Contains(s.Cookie, "other=1") {
		t.Errorf("kasm: %+v", s)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "wittyplay_kasm" || cookies[0].Path != "/api/modules/wittyplay/kasm/abc" {
		t.Errorf("cookies: %v", w.Header().Values("Set-Cookie"))
	}
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") {
		t.Errorf("csp: %q", csp)
	}
}

func TestWittyPlayKasmWebSocketPassesThrough(t *testing.T) {
	got := fakeWittyPlayService(t)
	ts := newTestServer(t)
	u, tok := ts.user(t, "player", auth.KindAdult, false, "")
	front := httptest.NewServer(ts.h)
	t.Cleanup(front.Close)
	addr := strings.TrimPrefix(front.URL, "http://")

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	req := "GET /api/modules/wittyplay/kasm/abc/websockify HTTP/1.1\r\nHost: " + addr + "\r\n" +
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
	if len(all) != 1 || all[0].Path != "/kasm/abc/websockify" || all[0].User != itoa64(u.ID) || all[0].Admin != "" {
		t.Errorf("the module saw %+v", all)
	}

	// From another site, the upgrade is refused before it reaches the module.
	c2, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	_ = c2.SetDeadline(time.Now().Add(10 * time.Second))
	_, _ = io.WriteString(c2, strings.Replace(req, "Connection:", "Origin: https://evil.example\r\nConnection:", 1))
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
