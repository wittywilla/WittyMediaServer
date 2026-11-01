package server

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// A file in a theme, opened on its own, is on this site: it mustn't be a form that
// sends what's typed into it elsewhere, which a policy without a sandbox allows.
func TestThemeFileIsSandboxed(t *testing.T) {
	ts := newTestServer(t)
	if err := os.MkdirAll(ts.Themes.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for name, body := range map[string]string{"theme.json": `{"name":"x"}`, "theme.css": "a{}", "logo.svg": `<svg xmlns="http://www.w3.org/2000/svg"/>`} {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(body))
	}
	zw.Close()
	if err := os.WriteFile(filepath.Join(ts.Themes.Dir, "mine.zip"), b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	w := ts.do(t, "GET", "/api/themes/mine/logo.svg", "", nil)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("%d, policy %q", w.Code, w.Header().Get("Content-Security-Policy"))
	}
}

// script-src 'self' means anything this site serves with a script's type can be run by a
// page here. What comes out of a book or off a station never has that type.
func TestBookFileIsNeverAScript(t *testing.T) {
	ts := newTestServer(t)
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, name := range []string{"evil.js", "evil.mjs", "chapter.xhtml", "style.css"} {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte("x"))
	}
	zw.Close()
	zr, err := zip.NewReader(bytes.NewReader(b.Bytes()), int64(b.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"evil.js", "evil.mjs"} {
		w := httptest.NewRecorder()
		ts.serveArchiveFile(w, zr, name)
		if ct := w.Header().Get("Content-Type"); scriptType(ct) || ct != "application/octet-stream" {
			t.Errorf("%s went out as %q", name, ct)
		}
	}
	w := httptest.NewRecorder()
	ts.serveArchiveFile(w, zr, "chapter.xhtml")
	if ct := w.Header().Get("Content-Type"); ct != "application/xhtml+xml" {
		t.Errorf("a chapter went out as %q", ct)
	}
}

func TestStationCantServeAScript(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "owner", "adult", true, "")
	station := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		fmt.Fprint(w, "alert(1)")
	}))
	defer station.Close()
	real := library.SafeClient
	library.SafeClient = station.Client()
	defer func() { library.SafeClient = real }()
	lib := ts.library(t, "radio")
	it, err := ts.Store.AddRadioStation(context.Background(), lib, library.RadioStation{Name: "x", URL: station.URL + "/s"})
	if err != nil {
		t.Fatal(err)
	}
	w := ts.do(t, "GET", fmt.Sprintf("/api/radio/%d/stream", it.ID), tok, nil)
	if w.Code == 200 && strings.Contains(w.Header().Get("Content-Type"), "javascript") {
		t.Errorf("relayed as %q", w.Header().Get("Content-Type"))
	}
	if w.Code != 200 {
		t.Logf("refused: %d", w.Code)
	}
}

// A shared cache in front of the server: a file sent to one account's cookie must not be
// kept for the next person to ask for that address.
func TestSignedInAnswersAreNotForSharedCaches(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "owner", "adult", true, "")
	dir := t.TempDir()
	p := filepath.Join(dir, "film.mp4")
	if err := os.WriteFile(p, []byte("film"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := ts.library(t, "videos", dir)
	it := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: p})
	w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/file", it.ID), tok, nil)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Cache-Control"), "private") {
		t.Errorf("%d, Cache-Control %q", w.Code, w.Header().Get("Cache-Control"))
	}
	// Those that say something of their own still do.
	if cc := ts.do(t, "GET", "/api/me", tok, nil).Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("JSON: %q", cc)
	}
}

// The redirect to HTTPS stays on the server's own address, whatever the request line says.
func TestHTTPSRedirectStaysHere(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	putRemote(t, h, tok, map[string]any{"enabled": true, "publicUrl": "https://media.example.com"})
	send := func(u *url.URL) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/", nil)
		r.URL = u
		r.RemoteAddr = public + ":5000"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, c := range []struct {
		u    *url.URL
		want string
	}{
		{&url.URL{Path: "/library", RawQuery: "a=1"}, "https://media.example.com/library?a=1"},
		{&url.URL{Path: "//evil.example/"}, "https://media.example.com//evil.example/"},
		// GET x:@evil.example/ HTTP/1.1. A browser wouldn't send it. A proxy or a script might.
		{&url.URL{Scheme: "x", Opaque: "@evil.example/"}, "https://media.example.com"},
	} {
		w := send(c.u)
		if w.Code != http.StatusPermanentRedirect || w.Header().Get("Location") != c.want {
			t.Errorf("%v: %d to %q", c.u, w.Code, w.Header().Get("Location"))
		}
	}
}

func TestHSTSBehindATrustedProxy(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	putRemote(t, h, tok, map[string]any{"enabled": true, "trustedProxies": "10.0.0.2"})
	const proxy = "10.0.0.2"
	if r := call(h, "GET", "/", proxy, nil, "X-Forwarded-For: "+public, "X-Forwarded-Proto: https"); r.h.Get("Strict-Transport-Security") == "" {
		t.Error("the proxy says HTTPS: it should stay on once used")
	}
	if r := call(h, "GET", "/", proxy, nil, "X-Forwarded-For: "+lan, "X-Forwarded-Proto: http"); r.h.Get("Strict-Transport-Security") != "" {
		t.Error("never over plain HTTP")
	}
	// Not the proxy: the header is only someone's word.
	if r := call(h, "GET", "/", lan, nil, "X-Forwarded-Proto: https"); r.h.Get("Strict-Transport-Security") != "" {
		t.Error("believed a visitor's own header")
	}
}
