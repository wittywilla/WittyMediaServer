// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// Where files live, and addresses that can hold a key, are for administrators on every list.
func TestListsDontGiveAwayPathsOrAddresses(t *testing.T) {
	ts := newTestServer(t)
	root := t.TempDir()
	lib := ts.library(t, "movies", root)
	radio := ts.library(t, "radio")
	pods := ts.library(t, "podcasts")
	a := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: filepath.Join(root, "alice-private", "seen.mkv"), Title: "Seen", Genres: []string{"Comedy"}, ContentRating: "G"})
	ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: filepath.Join(root, "alice-private", "unseen.mkv"), Title: "Unseen", Genres: []string{"Comedy"}, ContentRating: "G", Poster: filepath.Join(root, "alice-private", "poster.jpg")})
	ts.item(t, &library.Item{LibraryID: radio, Kind: "radio", Path: "https://stream.example.net/live?key=PRIVATEKEY", Title: "Station", ContentRating: "G"})
	ts.item(t, &library.Item{LibraryID: pods, Kind: "audio", Path: "https://feeds.example.net/ep1.mp3?auth=EPISODESECRET", Title: "Episode 1", Album: "A Show"})
	_, admin := ts.user(t, "boss", "adult", true, "")
	_, kid := ts.user(t, "kid", "kids", false, "")
	_, adult := ts.user(t, "bob", "adult", false, "")

	clean := func(name, body string) {
		t.Helper()
		for _, secret := range []string{"alice-private", "PRIVATEKEY", "EPISODESECRET"} {
			if strings.Contains(body, secret) {
				t.Errorf("%s holds %s: %.400s", name, secret, body)
			}
		}
	}
	w := ts.do(t, "POST", "/api/collections/preview", kid, map[string]any{"rule": map[string]any{"libraries": []int64{lib}}})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Unseen") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	clean("a Kids preview", w.Body.String())
	w = ts.do(t, "POST", "/api/collections/preview", adult, map[string]any{"rule": map[string]any{"libraryTypes": []string{"radio"}}})
	clean("an adult's preview of radio", w.Body.String())

	if w := ts.do(t, "POST", fmt.Sprintf("/api/items/%d/progress", a.ID), kid, map[string]any{"positionSec": 1000, "watched": true}); w.Code >= 300 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w = ts.do(t, "GET", "/api/me/recommended", kid, nil)
	clean("recommended", w.Body.String())
	for _, u := range []string{fmt.Sprintf("/api/items?library=%d", pods), "/api/home", "/api/music/tracks?library=" + fmt.Sprint(pods)} {
		w := ts.do(t, "GET", u, adult, nil)
		clean(u, w.Body.String())
	}

	// An administrator still sees what they need to manage them.
	if w := ts.do(t, "GET", fmt.Sprintf("/api/items?library=%d", pods), admin, nil); !strings.Contains(w.Body.String(), "EPISODESECRET") {
		t.Errorf("admin: %s", w.Body)
	}
}
