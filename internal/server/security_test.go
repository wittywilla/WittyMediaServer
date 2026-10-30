package server

import (
	"bytes"
	"context"
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

// Everything a restricted account can reach by asking directly for an address.
func TestRestrictedAccountCantReachHiddenTitles(t *testing.T) {
	ts := newTestServer(t)
	ctx := context.Background()
	dir := t.TempDir()
	lib := ts.library(t, "movies", dir)
	hiddenLib := ts.library(t, "shows", dir)
	secret := filepath.Join(dir, "poster.png")
	if err := os.WriteFile(secret, append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 40)...), 0o644); err != nil {
		t.Fatal(err)
	}
	sh := &library.Show{LibraryID: hiddenLib, Path: dir, Title: "Hidden show", SortTitle: "Hidden show", ContentRating: "TV-MA", Genres: []string{"Horror"}, Network: "SecretNet", Poster: secret}
	if err := ts.Store.UpsertShow(ctx, sh); err != nil {
		t.Fatal(err)
	}
	ok := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: filepath.Join(dir, "ok.mp4"), ContentRating: "G", Genres: []string{"Family"}})
	bad := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: filepath.Join(dir, "bad.mp4"), ContentRating: "R", Genres: []string{"Slasher"}, Poster: secret})
	_, kid := ts.user(t, "kid", "kids", false, "")

	for _, target := range []string{
		fmt.Sprintf("/api/items/%d", bad.ID), fmt.Sprintf("/api/items/%d/poster", bad.ID), fmt.Sprintf("/api/items/%d/file", bad.ID), fmt.Sprintf("/api/items/%d/play", bad.ID),
		fmt.Sprintf("/api/items/%d/stream", bad.ID), fmt.Sprintf("/api/items/%d/disc", bad.ID), fmt.Sprintf("/api/items/%d/recommend", bad.ID), fmt.Sprintf("/api/items/%d/book", bad.ID),
		fmt.Sprintf("/api/shows/%d", sh.ID), fmt.Sprintf("/api/shows/%d/art/poster", sh.ID), fmt.Sprintf("/api/shows/%d/art/backdrop", sh.ID),
	} {
		if w := ts.do(t, "GET", target, kid, nil); w.Code == 200 {
			t.Errorf("GET %s: a Kids profile got %s", target, w.Body.String())
		}
	}
	if w := ts.do(t, "POST", fmt.Sprintf("/api/items/%d/progress", bad.ID), kid, map[string]any{"positionSec": 100}); w.Code != 404 {
		t.Errorf("progress on a hidden title: %d", w.Code)
	}
	if w := ts.do(t, "POST", fmt.Sprintf("/api/items/%d/progress", ok.ID), kid, map[string]any{"positionSec": 100}); w.Code != 200 {
		t.Errorf("progress on an allowed title: %d", w.Code)
	}
	for target, secretWord := range map[string]string{"/api/genres": "Slasher", fmt.Sprintf("/api/shows/genres?library=%d", hiddenLib): "Horror", "/api/shows/networks": "SecretNet"} {
		if w := ts.do(t, "GET", target, kid, nil); strings.Contains(w.Body.String(), secretWord) {
			t.Errorf("%s names what is hidden: %s", target, w.Body)
		}
	}
}

// Where files are on this machine is the admin's to know.
func TestServerPathsHiddenFromOtherAccounts(t *testing.T) {
	ts := newTestServer(t)
	dir := filepath.Join(t.TempDir(), "users", "owner-name", "Movies")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lib := ts.library(t, "movies", dir)
	it := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: filepath.Join(dir, "m.mp4"), ContentRating: "G", Poster: filepath.Join(dir, "poster.jpg"), Backdrop: filepath.Join(dir, "fanart.jpg")})
	_, admin := ts.user(t, "admin", "adult", true, "")
	_, adult := ts.user(t, "adult", "adult", false, "")
	for _, target := range []string{"/api/libraries", "/api/items", fmt.Sprintf("/api/items/%d", it.ID), "/api/home"} {
		if w := ts.do(t, "GET", target, adult, nil); w.Code != 200 || strings.Contains(w.Body.String(), "owner-name") {
			t.Errorf("%s (%d): %s", target, w.Code, w.Body)
		}
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d", it.ID), admin, nil); !strings.Contains(w.Body.String(), "owner-name") {
		t.Errorf("the admin needs the path: %s", w.Body)
	}
}

// A private podcast feed's address is a password.
func TestPodcastAddressesHidden(t *testing.T) {
	ts := newTestServer(t)
	ctx := context.Background()
	lib := ts.library(t, "podcasts")
	other := ts.library(t, "podcasts")
	if _, err := ts.DB.ExecContext(ctx, `INSERT INTO podcasts(library_id, url, title, created_at) VALUES(?,?,?,1),(?,?,?,1)`, lib, "https://feeds.test/private?key=SECRET", "Show", other, "https://x.test/f", "Other"); err != nil {
		t.Fatal(err)
	}
	_, adult := ts.user(t, "adult", "adult", false, "")
	if w := ts.do(t, "GET", "/api/podcasts", adult, nil); strings.Contains(w.Body.String(), "SECRET") {
		t.Errorf("feed address shown: %s", w.Body)
	}
	limited := fmt.Sprintf(`{"maxAge":-1,"allowUnrated":true,"libraries":[%d]}`, lib)
	_, u := ts.user(t, "limited", "adult", false, limited)
	if w := ts.do(t, "GET", "/api/podcasts", u, nil); strings.Contains(w.Body.String(), "Other") {
		t.Errorf("a podcast from a library this account can't open: %s", w.Body)
	}
	_, admin := ts.user(t, "admin", "adult", true, "")
	if w := ts.do(t, "GET", "/api/podcasts", admin, nil); !strings.Contains(w.Body.String(), "SECRET") {
		t.Errorf("the admin manages feeds: %s", w.Body)
	}
}

// A limited adult who could create Kids profiles could give one no limits and
// sign in as it.
func TestLimitedAdultCantMakeUnlimitedProfiles(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "teen", "adult", false, `{"maxAge":13,"allowUnrated":false}`)
	body := map[string]any{"name": "backdoor", "password": "1234", "kind": "kids", "access": map[string]any{"maxAge": -1, "allowUnrated": true}}
	if w := ts.do(t, "POST", "/api/users", tok, body); w.Code != 403 {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	_, full := ts.user(t, "parent", "adult", false, "")
	if w := ts.do(t, "POST", "/api/users", full, body); w.Code != 201 {
		t.Fatalf("an unrestricted adult manages Kids profiles: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/users", full, map[string]any{"name": "x", "password": "long enough pw", "kind": "adult", "isAdmin": true}); w.Code != 403 {
		t.Fatalf("an adult made an admin: %d", w.Code)
	}
}

// Every route that changes something or shows server internals needs the
// right account. Walks the routes so a new one can't be forgotten here.
func TestAdminRoutes(t *testing.T) {
	ts := newTestServer(t)
	_, adult := ts.user(t, "adult", "adult", false, "")
	_, kid := ts.user(t, "kid", "kids", false, "")
	for _, rt := range []struct{ m, p string }{
		{"POST", "/api/libraries"}, {"PUT", "/api/libraries/1"}, {"DELETE", "/api/libraries/1"}, {"POST", "/api/libraries/1/scan"},
		{"PUT", "/api/items/1"}, {"GET", "/api/items/1/artwork"}, {"PUT", "/api/items/1/disc/labels"}, {"GET", "/api/artwork/preview?src=x.png"},
		{"POST", "/api/themes"}, {"DELETE", "/api/themes/x"}, {"PUT", "/api/shows/1"}, {"PUT", "/api/shows/1/view"}, {"GET", "/api/shows/1/artwork"},
		{"GET", "/api/podcasts/search?q=ab"}, {"POST", "/api/podcasts"}, {"PUT", "/api/podcasts/1"}, {"POST", "/api/podcasts/1/refresh"}, {"DELETE", "/api/podcasts/1"}, {"POST", "/api/items/1/download"},
		{"GET", "/api/settings"}, {"PUT", "/api/settings"}, {"PUT", "/api/branding/logo"}, {"DELETE", "/api/branding/logo"}, {"GET", "/api/system"},
		{"GET", "/api/fs?path=/"}, {"POST", "/api/fs/pick"}, {"POST", "/api/metadata/test"}, {"GET", "/api/agents"}, {"PUT", "/api/agents"}, {"POST", "/api/agents/test"},
		{"GET", "/api/logs"}, {"PUT", "/api/logs"}, {"GET", "/api/logs/files/wms-x.log"}, {"POST", "/api/logs/open"}, {"GET", "/api/update"}, {"POST", "/api/update/check"}, {"POST", "/api/update/apply"},
	} {
		for who, tok := range map[string]string{"adult": adult, "kids": kid} {
			if w := ts.do(t, rt.m, rt.p, tok, nil); w.Code != 403 {
				t.Errorf("%s %s as %s: %d", rt.m, rt.p, who, w.Code)
			}
		}
		if w := ts.do(t, rt.m, rt.p, "", nil); w.Code != 401 {
			t.Errorf("%s %s signed out: %d", rt.m, rt.p, w.Code)
		}
	}
}

func TestSettingsRefuseFFmpegEscapes(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "admin", "adult", true, "")
	for k, v := range map[string]string{
		"ffmpegOutputArgs": "-y /etc/cron.d/x", "ffmpegVideoArgs": "-i /etc/passwd", "x264Preset": "-y", "maxBitrate": "../../x", "audioBitrate": "1 2", "audioCodec": "../../../x",
	} {
		if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{k: v}); w.Code != 400 {
			t.Errorf("%s=%q: status %d", k, v, w.Code)
		}
		if got := ts.DB.GetSetting(context.Background(), k, ""); got == v {
			t.Errorf("%s=%q was saved", k, v)
		}
	}
	if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"ffmpegVideoArgs": "-preset slow -crf 20 -tune film", "maxBitrate": "8M"}); w.Code != 200 {
		t.Errorf("normal settings: %d %s", w.Code, w.Body)
	}
	// A value saved before this check is not used.
	_ = ts.DB.SetSetting(context.Background(), "qualityMode", "expert")
	_ = ts.DB.SetSetting(context.Background(), "ffmpegOutputArgs", "-y /tmp/evil")
	q, _ := ts.quality(httptest.NewRequest("GET", "/", nil))
	if len(q.OutputArgs) != 0 {
		t.Errorf("old expert options used: %v", q.OutputArgs)
	}
}

func TestViewerCantRaiseMaxHeight(t *testing.T) {
	ts := newTestServer(t)
	_ = ts.DB.SetSetting(context.Background(), "maxHeight", "720")
	for target, want := range map[string]int{"/?maxHeight=2160": 720, "/?maxHeight=480": 480, "/?maxHeight=-5": 720, "/?maxHeight=99999999": 720, "/": 720} {
		if _, h := ts.quality(httptest.NewRequest("GET", target, nil)); h != want {
			t.Errorf("%s: %d, want %d", target, h, want)
		}
	}
}
