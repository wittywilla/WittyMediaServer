package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
