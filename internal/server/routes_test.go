package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// Open to anyone, on purpose. Adding a route here should be a decision: the
// sign-in screen needs these, and nothing behind them is private.
var publicRoutes = map[string]bool{
	"GET /api/status":                true,
	"POST /api/setup":                true,
	"POST /api/login":                true,
	"POST /api/logout":               true,
	"GET /api/profiles":              true,
	"GET /api/themes":                true,
	"GET /api/themes/{id}/download":  true,
	"GET /api/themes/{id}/{file...}": true,
	"GET /api/health":                true,
	"GET /api/branding/logo":         true,
}

var routeRe = regexp.MustCompile(`m\.Handle(?:Func)?\("((?:GET|POST|PUT|DELETE) (/api/[^"]*))",\s*(\S*)`)

func testServer(t *testing.T) (*Server, *auth.Service) {
	t.Helper()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	a := &auth.Service{DB: d}
	return &Server{DB: d, Auth: a, Store: &library.Store{DB: d}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}, a
}

// Every route in server.go, as the code declares it: signed-out requests get
// 401 unless the route is listed above, and administrators' routes refuse
// everyone else. A new route is covered by being added.
func TestEveryRouteChecksWhoIsAsking(t *testing.T) {
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	s, a := testServer(t)
	ctx := context.Background()
	if _, err := a.CreateProfile(ctx, "grown", "password", auth.KindAdult, false, ""); err != nil {
		t.Fatal(err)
	}
	adult, _, err := a.Login(ctx, "grown", "password", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateProfile(ctx, "small", "password", auth.KindKids, false, ""); err != nil {
		t.Fatal(err)
	}
	kid, _, err := a.Login(ctx, "small", "password", "", false)
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	call := func(method, path, token string) int {
		r := httptest.NewRequest(method, path, strings.NewReader("{}"))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	fill := strings.NewReplacer("{id}", "1", "{aid}", "1", "{sid}", "x", "{ttn}", "1", "{n}", "1", "{serial}", "1", "{name}", "x", "{kind}", "poster", "{file...}", "x")
	matches := routeRe.FindAllSubmatch(src, -1)
	if len(matches) < 90 {
		t.Fatalf("found only %d routes: has the way they are declared changed?", len(matches))
	}
	for _, m := range matches {
		route, handler := string(m[1]), string(m[3])
		method, pattern, _ := strings.Cut(route, " ")
		path := fill.Replace(pattern)
		if publicRoutes[route] {
			if !strings.HasPrefix(handler, "s.handle") && !strings.HasPrefix(handler, "http.HandlerFunc") && !strings.HasPrefix(handler, "func") {
				t.Errorf("%s is listed as public but goes through %s", route, handler)
			}
			continue
		}
		if code := call(method, path, ""); code != http.StatusUnauthorized {
			t.Errorf("%s without signing in: %d, want 401", route, code)
		}
		if call(method, path, "not-a-token") != http.StatusUnauthorized {
			t.Errorf("%s with a made-up token was let through", route)
		}
		if strings.HasPrefix(handler, "s.requireAdmin") {
			for _, tok := range []string{adult, kid} {
				if code := call(method, path, tok); code != http.StatusForbidden {
					t.Errorf("%s as a non-admin: %d, want 403", route, code)
				}
			}
		}
	}
}

// Accounts can't manage each other beyond what they are allowed.
func TestAccountManagementLimits(t *testing.T) {
	s, a := testServer(t)
	ctx := context.Background()
	admin, err := a.CreateUser(ctx, "boss", "password", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateProfile(ctx, "grown", "password", auth.KindAdult, false, ""); err != nil {
		t.Fatal(err)
	}
	kidUser, err := a.CreateProfile(ctx, "small", "password", auth.KindKids, false, "")
	if err != nil {
		t.Fatal(err)
	}
	adult, _, _ := a.Login(ctx, "grown", "password", "", false)
	kid, _, _ := a.Login(ctx, "small", "password", "", false)
	h := s.Handler()
	call := func(method, path, token, body string) int {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	id := func(n int64) string { return itoa(n) }
	cases := []struct {
		name, method, path, token, body string
		want                            int
	}{
		{"an adult can't make an administrator", "POST", "/api/users", adult, `{"name":"x","password":"password","kind":"adult","isAdmin":true}`, 403},
		{"a kid can't add a profile", "POST", "/api/users", kid, `{"name":"y","password":"password","kind":"kids"}`, 403},
		{"a kid can't lift their own limits", "PUT", "/api/users/" + id(kidUser.ID) + "/access", kid, `{"access":{"maxAge":-1}}`, 403},
		{"a kid can't change their password", "PUT", "/api/users/" + id(kidUser.ID) + "/password", kid, `{"password":"newpassword"}`, 403},
		{"an adult can't change the admin's password", "PUT", "/api/users/" + id(admin.ID) + "/password", adult, `{"password":"newpassword"}`, 403},
		{"an adult can't delete the admin", "DELETE", "/api/users/" + id(admin.ID), adult, ``, 403},
		{"an adult needs their current password", "PUT", "/api/users/2/password", adult, `{"password":"newpassword","current":"wrong"}`, 403},
		{"an adult may limit a kid", "PUT", "/api/users/" + id(kidUser.ID) + "/access", adult, `{"access":{"maxAge":3}}`, 200},
	}
	for _, c := range cases {
		if code := call(c.method, c.path, c.token, c.body); code != c.want {
			t.Errorf("%s: %d, want %d", c.name, code, c.want)
		}
	}
	u, err := a.User(ctx, admin.ID)
	if err != nil || !u.IsAdmin {
		t.Fatalf("the administrator changed: %+v %v", u, err)
	}
}

// A podcast host decides what comes back. Whatever it says, it must not run
// as a page on this site.
func TestPodcastProxyServesOnlySound(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", r.URL.Query().Get("type"))
		_, _ = w.Write([]byte("<script>alert(document.domain)</script>"))
	}))
	defer remote.Close()
	saved := library.SafeClient
	library.SafeClient = remote.Client() // the test server is on this machine, which the real client refuses
	defer func() { library.SafeClient = saved }()
	var s Server
	for typ, want := range map[string]string{"text/html": "application/octet-stream", "image/svg+xml": "application/octet-stream", "audio/mpeg": "audio/mpeg", "": "application/octet-stream"} {
		w := httptest.NewRecorder()
		h := s.headers(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.proxyEpisode(w, r, &library.Item{Path: remote.URL + "/ep?type=" + typ})
		}))
		h.ServeHTTP(w, httptest.NewRequest("GET", "/api/items/1/stream", nil))
		if got := w.Header().Get("Content-Type"); got != want {
			t.Errorf("%q came back as %q", typ, got)
		}
		if w.Header().Get("Content-Security-Policy") != apiCSP {
			t.Errorf("%q: no policy", typ)
		}
	}
}
