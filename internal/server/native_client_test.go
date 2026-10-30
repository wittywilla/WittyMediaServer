package server

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
)

// Native apps (TV, phone) send no Origin and keep no cookies: only the token
// from the sign-in answer.
func TestNativeClientNeedsNoCookies(t *testing.T) {
	s, h := remoteServer(t, nil)
	admin := adminToken(t, s, h)
	r := call(h, "POST", "/api/login", lan, map[string]string{"name": "Owner", "password": pass, "device": "Living room TV"}, "User-Agent: SomeTV/1.0")
	var out struct {
		Token string `json:"token"`
	}
	r.json(t, &out)
	if r.code != 200 || out.Token == "" {
		t.Fatalf("sign-in: %d %s", r.code, r.body)
	}
	bearer := "Authorization: Bearer " + out.Token
	for name, hdr := range map[string]string{"bearer": bearer, "header": "X-WMS-Token: " + out.Token} {
		if r := call(h, "GET", "/api/me", lan, nil, hdr); r.code != 200 {
			t.Errorf("%s: %d", name, r.code)
		}
	}
	// Changing things needs no browser headers either.
	if r := call(h, "PUT", "/api/me/ui", lan, map[string]any{"x": 1}, bearer); r.code != 200 {
		t.Errorf("a change with a token: %d %s", r.code, r.body)
	}
	// It shows up by the name it gave, and can be signed out like any other.
	var list []auth.Session
	call(h, "GET", "/api/sessions", lan, nil, bearer).json(t, &list)
	var tv auth.Session
	for _, v := range list {
		if v.Current {
			tv = v
		}
	}
	if tv.Device != "Living room TV" || tv.Where != "home" {
		t.Errorf("session: %+v", tv)
	}
	if r := call(h, "DELETE", "/api/sessions/"+tv.ID, lan, nil, "Authorization: Bearer "+admin); r.code != 200 {
		t.Fatalf("revoke: %d", r.code)
	}
	if r := call(h, "GET", "/api/me", lan, nil, bearer); r.code != 401 {
		t.Errorf("a revoked token: %d", r.code)
	}
}

// Native players often can't add headers: the playlist must carry a ?token= on
// to its segments, a stream token rather than the session's own, and never
// otherwise put a token in an address.
func TestPlaylistCarriesTokenOnlyWhenSignedInByIt(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	s.HLS = hls.NewManager(&ffmpeg.Tools{}, t.TempDir(), 1, slog.New(slog.DiscardHandler))
	s.HLS.Ensure(hls.Spec{ID: "v1", Duration: 8, SegmentSec: 4})

	r := call(h, "GET", "/api/hls/v1/playlist.m3u8?token="+tok, lan, nil)
	if r.code != 200 || !strings.Contains(r.body, "seg/0.ts?token=st") || !strings.Contains(r.body, "seg/1.ts?token=st") || strings.Contains(r.body, tok) {
		t.Errorf("by ?token=: %d\n%s", r.code, r.body)
	}
	r = call(h, "GET", "/api/hls/v1/playlist.m3u8", lan, nil, "Authorization: Bearer "+tok)
	if r.code != 200 || strings.Contains(r.body, "token") || !strings.Contains(r.body, "seg/0.ts\n") {
		t.Errorf("by header: %d\n%s", r.code, r.body)
	}
	// Whatever else the address says is not repeated.
	r = call(h, "GET", "/api/hls/v1/playlist.m3u8?token=made-up", lan, nil, "Authorization: Bearer "+tok)
	if r.code != 200 || strings.Contains(r.body, "made-up") {
		t.Errorf("a token that didn't sign in: %d\n%s", r.code, r.body)
	}
	if r := call(h, "GET", "/api/hls/v1/playlist.m3u8?token=made-up", lan, nil); r.code != 401 {
		t.Errorf("no valid token: %d", r.code)
	}
}
