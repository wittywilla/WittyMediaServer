package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
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
	if got := redactQuery("%74oken=abcdef0123"); strings.Contains(got, "abcdef") {
		t.Fatalf("an escaped parameter name got through: %q", got)
	}
	if got := tokenRe.ReplaceAllString(`GET /api/x?token=abc123&f=1 failed`, "token=REDACTED"); strings.Contains(got, "abc123") {
		t.Fatalf("client log not redacted: %q", got)
	}
}

// Nothing under /api may run as a page on this site; the rest of the site has
// the app's own policy.
func TestHeadersSetPolicyForAPI(t *testing.T) {
	var s Server
	h := s.headers(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<script>alert(1)</script>"))
	}))
	for path, want := range map[string]string{"/api/items/1/stream": apiCSP, "/": appCSP} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if got := w.Header().Get("Content-Security-Policy"); got != want {
			t.Errorf("%s: policy %q", path, got)
		}
	}
	if strings.Contains(apiCSP, "script-src") || !strings.Contains(apiCSP, "default-src 'none'") {
		t.Errorf("apiCSP allows scripts: %s", apiCSP)
	}
}

func TestWebFilesAndGzip(t *testing.T) {
	web := fstest.MapFS{
		"index.html":        {Data: []byte("<html>")},
		"assets/a-1.js":     {Data: []byte("console.log(1)")},
		"assets/a-1.js.gz":  {Data: []byte("gz")},
		"assets/plain-1.js": {Data: []byte("console.log(2)")},
	}
	s := &Server{Web: web}
	h := s.spa()
	get := func(path, enc string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		if enc != "" {
			r.Header.Set("Accept-Encoding", enc)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := get("/assets/a-1.js", "br, gzip")
	if w.Header().Get("Content-Encoding") != "gzip" || w.Body.String() != "gz" || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/javascript") {
		t.Errorf("gzip: %v %q", w.Header(), w.Body)
	}
	if w := get("/assets/a-1.js", "gzip;q=0"); w.Header().Get("Content-Encoding") != "" || w.Body.String() != "console.log(1)" {
		t.Errorf("a browser refusing gzip got %q", w.Body)
	}
	if w := get("/assets/plain-1.js", "gzip"); w.Header().Get("Content-Encoding") != "" || w.Body.String() != "console.log(2)" {
		t.Errorf("no compressed copy: %q", w.Body)
	}
	if w := get("/", ""); w.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("the page may be cached: %v", w.Header())
	}
	if w := get("/assets/a-1.js", ""); !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("hashed assets: %v", w.Header())
	}
	if w := get("/library/3", ""); !strings.Contains(w.Body.String(), "<html>") {
		t.Errorf("an app address gets the page: %q", w.Body)
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
