package server

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// tvServer is a whole server with no libraries, an admin and a Kids profile.
func tvServer(t *testing.T) (h http.Handler, admin, kid string, s *Server) {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	mgr := hls.NewManager(&ffmpeg.Tools{Encoders: map[string]bool{}}, t.TempDir(), 2, slog.Default())
	s = &Server{Cfg: &config.Config{DataDir: dir}, DB: d, Log: slog.Default(), Tools: &ffmpeg.Tools{}, Store: &library.Store{DB: d}, Auth: &auth.Service{DB: d}, HLS: mgr}
	h = s.Handler()
	t.Cleanup(func() {
		mgr.StopAllLive()
		if s.TV != nil {
			s.TV.Close()
		}
		d.Close()
	})
	ctx := context.Background()
	if _, err := s.Auth.CreateUser(ctx, "boss", "correct horse", true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Auth.CreateProfile(ctx, "kid", "", auth.KindKids, false, ""); err != nil {
		t.Fatal(err)
	}
	admin, _, err = s.Auth.Login(ctx, "boss", "correct horse", "t", false)
	if err != nil {
		t.Fatal(err)
	}
	kid, _, err = s.Auth.Login(ctx, "kid", "", "t", true)
	if err != nil {
		t.Fatal(err)
	}
	return h, admin, kid, s
}

func tvCall(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = "192.168.1.50:4000"
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestTVRoutesNeedTheRightAccount(t *testing.T) {
	h, admin, kid, _ := tvServer(t)
	adminOnly := []struct{ method, path string }{
		{"POST", "/api/channels"}, {"GET", "/api/channels/1"}, {"PUT", "/api/channels/1"}, {"DELETE", "/api/channels/1"},
		{"POST", "/api/channels/preview"}, {"GET", "/api/channels/suggestions"}, {"GET", "/api/channels/options"},
		{"POST", "/api/channels/from-network"}, {"POST", "/api/channels/networks/refresh"}, {"POST", "/api/channels/1/rebuild"},
		{"GET", "/api/channels/1/history"}, {"POST", "/api/channels/1/restore"}, {"PUT", "/api/channels/1/logo"}, {"DELETE", "/api/channels/1/logo"},
		{"GET", "/api/tuners"}, {"POST", "/api/tuners"}, {"PUT", "/api/tuners/1"}, {"DELETE", "/api/tuners/1"}, {"GET", "/api/tuners/drivers"}, {"GET", "/api/tuners/discover"},
		{"POST", "/api/tuners/1/scan"}, {"POST", "/api/tuners/1/refresh"}, {"GET", "/api/tuners/1/channels"}, {"PUT", "/api/tuner-channels/1"},
		{"GET", "/api/epg/sources"}, {"POST", "/api/epg/sources"}, {"DELETE", "/api/epg/sources/1"}, {"GET", "/api/tv/settings"}, {"PUT", "/api/tv/settings"},
		{"PUT", "/api/libraries/1/main"},
	}
	for _, c := range adminOnly {
		if w := tvCall(h, c.method, c.path, kid, "{}"); w.Code != 403 {
			t.Errorf("%s %s as a Kids profile: %d", c.method, c.path, w.Code)
		}
		if w := tvCall(h, c.method, c.path, "", "{}"); w.Code != 401 {
			t.Errorf("%s %s signed out: %d", c.method, c.path, w.Code)
		}
	}
	for _, p := range []string{"/api/guide", "/api/channels", "/api/tv/lib-1/tune", "/api/tv/lib-1/live/playlist.m3u8", "/api/tv/lib-1/live/seg/x.ts", "/api/recordings"} {
		if w := tvCall(h, "GET", p, "", ""); w.Code != 401 {
			t.Errorf("GET %s signed out: %d", p, w.Code)
		}
	}
	// Recordings are for accounts that may use tuner channels.
	if w := tvCall(h, "GET", "/api/recordings", kid, ""); w.Code != 403 {
		t.Errorf("a Kids profile's recordings: %d", w.Code)
	}
	if w := tvCall(h, "GET", "/api/recordings", admin, ""); w.Code != 200 {
		t.Errorf("admin recordings: %d", w.Code)
	}
}

func TestChannelInputIsChecked(t *testing.T) {
	h, admin, _, _ := tvServer(t)
	bad := []string{
		`{"name":"","definition":{}}`,
		`{"name":"x","number":"abc","definition":{}}`,
		`{"name":"x","definition":{"mode":"nope"}}`,
		`{"name":"x","definition":{"alignMin":7}}`,
		`{"name":"x","definition":{"mode":"rules","blocks":[{"start":"25:00","end":"26:00"}]}}`,
		`{"name":"x","definition":{"rules":{"kinds":["'; DROP TABLE items;--"]}}}`,
		`not json`,
	}
	for _, b := range bad {
		if w := tvCall(h, "POST", "/api/channels", admin, b); w.Code != 400 {
			t.Errorf("%s: %d %s", b, w.Code, w.Body)
		}
	}
	// Rule text goes through bound arguments, whatever it says.
	w := tvCall(h, "POST", "/api/channels", admin, `{"name":"Tricky","definition":{"rules":{"genres":["'; DROP TABLE items;--"],"search":"%' OR 1=1 --"}}}`)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := tvCall(h, "GET", "/api/guide", admin, ""); w.Code != 200 || !strings.Contains(w.Body.String(), "Tricky") {
		t.Fatalf("guide after odd rules: %d %s", w.Code, w.Body)
	}
	for _, p := range []string{"/api/tv/nope/tune", "/api/tv/lib-x/tune", "/api/tv/lib-0/tune", "/api/tv/other-1/tune", "/api/tv/lib-999/tune", "/api/tv/tun-999/tune"} {
		if w := tvCall(h, "GET", p, admin, ""); w.Code != 400 && w.Code != 404 {
			t.Errorf("%s: %d", p, w.Code)
		}
	}
	if w := tvCall(h, "GET", "/api/guide?hours=9999", admin, ""); w.Code != 400 {
		t.Errorf("hours: %d", w.Code)
	}
	if w := tvCall(h, "PUT", "/api/tv/settings", admin, `{"nope":"1"}`); w.Code != 400 {
		t.Errorf("unknown setting: %d", w.Code)
	}
	if w := tvCall(h, "PUT", "/api/tv/settings", admin, `{"dvrFolder":"relative/path"}`); w.Code != 400 {
		t.Errorf("relative folder: %d", w.Code)
	}
	if w := tvCall(h, "POST", "/api/recordings", admin, `{"channel":"lib-1","title":"x","start":1,"stop":2}`); w.Code != 400 || !strings.Contains(w.Body.String(), "already in your library") {
		t.Errorf("library channels are not recorded: %d %s", w.Code, w.Body)
	}
	if w := tvCall(h, "POST", "/api/tuners", admin, `{"driver":"command","name":"x","options":{"command":"/bin/sh"}}`); w.Code != 400 {
		t.Errorf("a web session must not be able to add a command: %d", w.Code)
	}
	if w := tvCall(h, "POST", "/api/tuners", admin, `{"driver":"nope","name":"x"}`); w.Code != 400 {
		t.Errorf("unknown driver: %d", w.Code)
	}
	if w := tvCall(h, "POST", "/api/epg/sources", admin, `{"url":"ftp://x.test/a"}`); w.Code != 400 {
		t.Errorf("guide scheme: %d", w.Code)
	}
}

func TestChannelLogoIsAPicture(t *testing.T) {
	h, admin, _, _ := tvServer(t)
	w := tvCall(h, "POST", "/api/channels", admin, `{"name":"Logo","definition":{}}`)
	var c struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &c); err != nil || c.ID == 0 {
		t.Fatalf("%v %s", err, w.Body)
	}
	path := "/api/channels/" + itoa(c.ID) + "/logo"
	for _, b := range []string{`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`, "not an image", ""} {
		if w := tvCall(h, "PUT", path, admin, b); w.Code != 400 {
			t.Errorf("%q: %d", b, w.Code)
		}
	}
	var png1 bytes.Buffer
	if err := png.Encode(&png1, image.NewRGBA(image.Rect(0, 0, 32, 32))); err != nil {
		t.Fatal(err)
	}
	if w := tvCall(h, "PUT", path, admin, png1.String()); w.Code != 200 {
		t.Fatalf("a real logo: %d %s", w.Code, w.Body)
	}
	if w := tvCall(h, "GET", path, admin, ""); w.Code != 200 || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("reading it back: %d %s", w.Code, w.Header().Get("Content-Type"))
	}
}

func TestHiddenCommercialsLibraries(t *testing.T) {
	h, admin, kid, s := tvServer(t)
	ctx := context.Background()
	lib, err := s.Store.CreateLibrary(ctx, "Ads", library.CommercialsType, []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	names := func(tok string) string { return tvCall(h, "GET", "/api/libraries", tok, "").Body.String() }
	if !strings.Contains(names(admin), `"Ads"`) {
		t.Error("administrators need it in Settings")
	}
	if strings.Contains(names(kid), `"Ads"`) {
		t.Error("a hidden library must stay out of other people's menus")
	}
	path := "/api/libraries/" + itoa(lib.ID) + "/main"
	if w := tvCall(h, "PUT", path, admin, `{"show":true}`); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	if !strings.Contains(names(kid), `"Ads"`) {
		t.Error("switched on, it shows")
	}
	other, _ := s.Store.CreateLibrary(ctx, "Films", "movies", []string{t.TempDir()})
	if w := tvCall(h, "PUT", "/api/libraries/"+itoa(other.ID)+"/main", admin, `{"show":false}`); w.Code != 404 {
		t.Errorf("only commercials libraries have the switch: %d", w.Code)
	}
}

func TestNoAudioDecoderReadsFFmpeg(t *testing.T) {
	for _, m := range []string{
		"Decoder (codec ac4) not found for input stream #0:1",
		"Decoding requested, but no decoder found for: ac4",
	} {
		if !noAudioDecoder(m) {
			t.Errorf("%q should mean no AC-4 decoder", m)
		}
	}
	if noAudioDecoder("Connection reset by peer") {
		t.Error("other failures are not about sound")
	}
}

func TestEncryptedHelpComesFromTheAPI(t *testing.T) {
	h, _, kid, _ := tvServer(t)
	w := tvCall(h, "GET", "/api/tv/encrypted-help", kid, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "ATSC 1.0") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := tvCall(h, "GET", "/api/tv/encrypted-help", "", ""); w.Code != 401 {
		t.Errorf("signed out: %d", w.Code)
	}
}
