package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// A file that has gone missing doesn't tell a viewer where it was.
func TestErrorsDontNameFolders(t *testing.T) {
	ts := newTestServer(t)
	root := t.TempDir()
	lib := ts.library(t, "movies", root)
	d := ts.item(t, &library.Item{LibraryID: lib, Kind: "dvd-iso", Path: filepath.Join(root, "alice-private", "gone.iso"), Title: "Gone", ContentRating: "G"})
	b := ts.item(t, &library.Item{LibraryID: lib, Kind: "bluray-iso", Path: filepath.Join(root, "alice-private", "gone-bd.iso"), Title: "GoneBD", ContentRating: "G"})
	v := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: filepath.Join(root, "alice-private", "gone.mkv"), Title: "GoneVid", ContentRating: "G"})
	ts.user(t, "boss", "adult", true, "")
	_, kid := ts.user(t, "kid", "kids", false, "")
	for _, c := range []struct {
		m, u string
		body any
	}{
		{"GET", fmt.Sprintf("/api/items/%d/disc", d.ID), nil},
		{"POST", fmt.Sprintf("/api/items/%d/dvd", d.ID), map[string]any{}},
		{"GET", fmt.Sprintf("/api/items/%d/disc", b.ID), nil},
		{"POST", fmt.Sprintf("/api/items/%d/dvd", b.ID), map[string]any{}},
		{"GET", fmt.Sprintf("/api/items/%d/play", v.ID), nil},
		{"POST", fmt.Sprintf("/api/items/%d/playback", v.ID), map[string]any{"profile": "web"}},
	} {
		w := ts.do(t, c.m, c.u, kid, c.body)
		if w.Code < 400 {
			t.Errorf("%s: %d", c.u, w.Code)
		}
		if strings.Contains(w.Body.String(), "alice-private") || strings.Contains(w.Body.String(), root) {
			t.Errorf("%s: %s", c.u, w.Body)
		}
	}
}

func TestViewerErrorKeepsWhatHelps(t *testing.T) {
	ts := newTestServer(t)
	root := t.TempDir()
	ts.library(t, "movies", root)
	_, kid := ts.user(t, "kid", "kids", false, "")
	_, admin := ts.user(t, "boss", "adult", true, "")
	check := func(tok string, err error, want string) {
		t.Helper()
		r := httptest.NewRequest("GET", "/", nil)
		u, rerr := ts.Auth.Resolve(r.Context(), tok)
		if rerr != nil {
			t.Fatal(rerr)
		}
		r = r.WithContext(context.WithValue(r.Context(), userKey, u))
		if got := ts.viewerError(r, err); got != want {
			t.Errorf("%v: got %q", err, got)
		}
	}
	path := filepath.Join(root, "x", "a.iso")
	stat := &os.PathError{Op: "stat", Path: "/some/other/place", Err: os.ErrNotExist}
	check(kid, stat, fileFailed)
	check(kid, fmt.Errorf("ffprobe: exit status 1: %s: Invalid data", path), fileFailed)
	check(kid, errors.New("this disc is encrypted and can't be played"), "this disc is encrypted and can't be played")
	check(admin, stat, stat.Error())
}

func TestCapabilitiesHideTheVersionFromTheInternet(t *testing.T) {
	s, h := remoteServer(t, nil)
	s.Version = "9.9.9"
	tok := adminToken(t, s, h)
	putRemote(t, h, tok, map[string]any{"enabled": true})
	var c struct {
		Version    string `json:"version"`
		ApiVersion int    `json:"apiVersion"`
	}
	r := call(h, "GET", "/api/capabilities", lan, nil)
	if err := json.Unmarshal([]byte(r.body), &c); err != nil || c.Version != "9.9.9" {
		t.Errorf("at home: %+v %v", c, err)
	}
	c.Version = ""
	r = call(h, "GET", "/api/capabilities", public, nil, "TLS: yes")
	if err := json.Unmarshal([]byte(r.body), &c); err != nil || c.Version != "" || c.ApiVersion == 0 {
		t.Errorf("internet, signed out: %s %v", r.body, err)
	}
	r = call(h, "GET", "/api/capabilities", public, nil, "TLS: yes", "Authorization: Bearer "+tok)
	if err := json.Unmarshal([]byte(r.body), &c); err != nil || c.Version != "9.9.9" {
		t.Errorf("internet, signed in: %s %v", r.body, err)
	}
}
