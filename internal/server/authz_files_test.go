// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// A file already in a library, swapped for a link to something else, is read by nothing that
// serves or plays it: this includes a link anyone can open with no account.
func TestLinkSwappedInAfterAScanIsNeverServed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs links")
	}
	ts := newTestServer(t)
	root := t.TempDir()
	lib := ts.library(t, "photos", root)
	secret := filepath.Join(t.TempDir(), "wms.db")
	if err := os.WriteFile(secret, []byte("TOP-SECRET-DATABASE"), 0o600); err != nil {
		t.Fatal(err)
	}
	photo := filepath.Join(root, "a.jpg")
	clip := filepath.Join(root, "b.mp4")
	if err := os.Symlink(secret, photo); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, clip); err != nil {
		t.Fatal(err)
	}
	p := ts.item(t, &library.Item{LibraryID: lib, Kind: "photo", Path: photo, Title: "a"})
	v := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: clip, Title: "b", VideoCodec: "h264"})
	ts.user(t, "boss", "adult", true, "")
	_, adult := ts.user(t, "bob", "adult", false, "")

	for _, u := range []string{
		fmt.Sprintf("/api/items/%d/original", p.ID),
		fmt.Sprintf("/api/items/%d/image", p.ID),
		fmt.Sprintf("/api/items/%d/image?w=320", p.ID),
		fmt.Sprintf("/api/items/%d/original", v.ID),
		fmt.Sprintf("/api/items/%d/file", v.ID),
		fmt.Sprintf("/api/items/%d/play", v.ID),
	} {
		w := ts.do(t, "GET", u, adult, nil)
		if w.Code != 404 || strings.Contains(w.Body.String(), "TOP-SECRET") {
			t.Errorf("%s: %d %.80s", u, w.Code, w.Body)
		}
	}
	if w := ts.do(t, "POST", fmt.Sprintf("/api/items/%d/playback", v.ID), adult, map[string]any{"profile": "web"}); w.Code != 404 {
		t.Errorf("playback: %d %.80s", w.Code, w.Body)
	}
	w := ts.do(t, "POST", "/api/shares", adult, map[string]any{"kind": "item", "id": v.ID, "allowDownload": true})
	var l struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &l); err != nil || l.Token == "" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	for _, u := range []string{"original", "file", "image"} {
		w := ts.do(t, "GET", fmt.Sprintf("/api/share/%s/%s/%d", l.Token, u, v.ID), "", nil)
		if strings.Contains(w.Body.String(), "TOP-SECRET") || w.Code < 400 {
			t.Errorf("share %s: %d %.80s", u, w.Code, w.Body)
		}
	}

	// A real file in the same library still comes through.
	real := filepath.Join(root, "c.jpg")
	if err := os.WriteFile(real, []byte("picture"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := ts.item(t, &library.Item{LibraryID: lib, Kind: "photo", Path: real, Title: "c"})
	if w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/original", c.ID), adult, nil); w.Code != 200 || w.Body.String() != "picture" {
		t.Errorf("a plain file: %d %s", w.Code, w.Body)
	}
}

// The clip of a live photo goes through the same check.
func TestMotionClipLinkIsRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs links")
	}
	ts := newTestServer(t)
	root := t.TempDir()
	lib := ts.library(t, "photos", root)
	secret := filepath.Join(t.TempDir(), "wms.db")
	if err := os.WriteFile(secret, []byte("TOP-SECRET-DATABASE"), 0o600); err != nil {
		t.Fatal(err)
	}
	photo := filepath.Join(root, "a.jpg")
	if err := os.WriteFile(photo, []byte("picture"), 0o644); err != nil {
		t.Fatal(err)
	}
	clip := filepath.Join(root, "a.mov")
	if err := os.Symlink(secret, clip); err != nil {
		t.Fatal(err)
	}
	p := ts.item(t, &library.Item{LibraryID: lib, Kind: "photo", Path: photo, Title: "a"})
	if _, err := ts.DB.Exec(`INSERT INTO photo_meta(item_id, library_id, taken_at, ym, motion) VALUES(?, ?, 1, 1, ?) ON CONFLICT(item_id) DO UPDATE SET motion = excluded.motion`, p.ID, lib, clip); err != nil {
		t.Fatal(err)
	}
	ts.user(t, "boss", "adult", true, "")
	_, adult := ts.user(t, "bob", "adult", false, "")
	w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/motion", p.ID), adult, nil)
	if w.Code != 404 || strings.Contains(w.Body.String(), "TOP-SECRET") {
		t.Errorf("%d %.80s", w.Code, w.Body)
	}
}

// What a viewer away from home is held to covers the file itself as well.
func TestOriginalIsHeldToTheLimitAwayFromHome(t *testing.T) {
	ts := newTestServer(t)
	root := t.TempDir()
	lib := ts.library(t, "photos", root)
	film := filepath.Join(root, "film.mp4")
	if err := os.WriteFile(film, make([]byte, 640<<10), 0o644); err != nil {
		t.Fatal(err)
	}
	it := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: film, Title: "film"})
	_, admin := ts.user(t, "boss", "adult", true, "")
	if w := ts.do(t, "PUT", "/api/remote", admin, map[string]any{"enabled": true, "maxBitrate": "1M"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	r := httptest.NewRequest("GET", fmt.Sprintf("/api/items/%d/original", it.ID), nil)
	r.Host = "media.example.com"
	r.RemoteAddr = "100.64.0.9:5000" // a VPN: away from home, encrypted by itself
	r.Header.Set("Authorization", "Bearer "+admin)
	w := httptest.NewRecorder()
	start := time.Now()
	ts.h.ServeHTTP(w, r)
	if w.Code != http.StatusOK || w.Body.Len() != 640<<10 {
		t.Fatalf("%d %d", w.Code, w.Body.Len())
	}
	// 640 KB at 1.25 Mbit/s, less the two seconds' head start, takes about two seconds.
	if d := time.Since(start); d < 800*time.Millisecond {
		t.Errorf("sent in %v: nothing held it back", d)
	}
}
