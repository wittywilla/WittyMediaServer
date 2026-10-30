package server

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

func TestCSRF(t *testing.T) {
	var s Server
	h := s.csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	cases := []struct {
		name   string
		method string
		hdr    map[string]string
		want   int
	}{
		{"read from anywhere", "GET", map[string]string{"Origin": "http://evil.test"}, 200},
		{"same origin by fetch metadata", "POST", map[string]string{"Sec-Fetch-Site": "same-origin"}, 200},
		{"another site", "POST", map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
		{"another port on this host", "POST", map[string]string{"Sec-Fetch-Site": "same-site"}, 403},
		{"old browser, other origin", "PUT", map[string]string{"Origin": "http://media.local:8080"}, 403},
		{"old browser, same origin", "PUT", map[string]string{"Origin": "http://media.local:9797"}, 200},
		{"sandboxed page", "POST", map[string]string{"Origin": "null"}, 403},
		{"no browser at all", "DELETE", nil, 200},
		{"app with a bearer token", "POST", map[string]string{"Sec-Fetch-Site": "cross-site", "Authorization": "Bearer x"}, 200},
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, "http://media.local:9797/api/users", nil)
		for k, v := range c.hdr {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s: got %d, want %d", c.name, w.Code, c.want)
		}
	}
}

func TestCORSHasNoCredentials(t *testing.T) {
	var s Server
	h := s.cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	r := httptest.NewRequest("GET", "/api/me", nil)
	r.Header.Set("Origin", "http://evil.test")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("cookies must never be honoured across origins")
	}
}

func TestRedactQuery(t *testing.T) {
	got := redactQuery("start=10&token=abcdef0123")
	if strings.Contains(got, "abcdef") || !strings.Contains(got, "start=10") {
		t.Fatalf("redactQuery = %q", got)
	}
	if got := tokenRe.ReplaceAllString(`GET /api/x?token=abc123&f=1 failed`, "token=REDACTED"); strings.Contains(got, "abc123") {
		t.Fatalf("client log not redacted: %q", got)
	}
}

func TestImageType(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 40)...)
	if ct := imageType(bytes.NewReader(png), "x.png"); ct != "image/png" {
		t.Errorf("png: %q", ct)
	}
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	if ct := imageType(bytes.NewReader(svg), "poster.jpg"); ct != "" {
		t.Errorf("an SVG named .jpg was accepted as %q", ct)
	}
	if ct := imageType(bytes.NewReader([]byte("<html><script>x</script>")), "a.png"); ct != "" {
		t.Errorf("HTML accepted as %q", ct)
	}
}

// A podcast feed decides what its enclosure address serves. A web page there
// must not run on this server's site when someone opens the stream's address.
func TestEpisodeProxyNeverServesPages(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "admin", "adult", true, "")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<script>alert(document.domain)</script>"))
	}))
	defer upstream.Close()
	old := library.SafeClient
	library.SafeClient = upstream.Client() // the real one refuses this machine
	defer func() { library.SafeClient = old }()
	lib := ts.library(t, "podcasts")
	it := ts.item(t, &library.Item{LibraryID: lib, Kind: "audio", Path: upstream.URL + "/ep.mp3", Provider: "podcast"})
	w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/stream", it.ID), tok, nil)
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); strings.Contains(ct, "html") {
		t.Errorf("served a feed's page as %q", ct)
	}
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") {
		t.Errorf("no sandbox on a proxied stream: %q", csp)
	}
}

func TestEpisodeType(t *testing.T) {
	for in, want := range map[string]string{"audio/mpeg": "audio/mpeg", "video/mp4; x=1": "video/mp4", "application/ogg": "application/ogg", "text/html": "application/octet-stream",
		"image/svg+xml": "application/octet-stream", "": "application/octet-stream", "application/xhtml+xml": "application/octet-stream"} {
		if got := episodeType(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

// A library file's type comes from its extension, never from its contents.
func TestFileNeverSniffedAsPage(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "admin", "adult", true, "")
	dir := t.TempDir()
	p := filepath.Join(dir, "movie.xyz")
	if err := os.WriteFile(p, []byte("<!doctype html><script>alert(1)</script>"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := ts.library(t, "videos", dir)
	it := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: p})
	w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/file", it.ID), tok, nil)
	if ct := w.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("Content-Type %q", ct)
	}
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") {
		t.Errorf("CSP %q", csp)
	}
}

func TestSecurityHeaders(t *testing.T) {
	ts := newTestServer(t)
	for _, target := range []string{"/", "/api/status", "/api/nothing"} {
		w := ts.do(t, "GET", target, "", nil)
		for _, h := range []string{"X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy", "Content-Security-Policy"} {
			if w.Header().Get(h) == "" {
				t.Errorf("%s: no %s", target, h)
			}
		}
	}
	if csp := ts.do(t, "GET", "/", "", nil).Header().Get("Content-Security-Policy"); strings.Contains(csp, "connect-src 'self' https:") {
		t.Errorf("the app may talk only to this server: %q", csp)
	}
}
