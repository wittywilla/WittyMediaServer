// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// Links follow who may still see what they name.
func TestShareLinksFollowTheAlbumAndTheShow(t *testing.T) {
	ts := newTestServer(t)
	root := t.TempDir()
	photos := ts.library(t, "photos", root)
	file := func(name string) string {
		p := filepath.Join(root, name)
		if err := os.WriteFile(p, []byte("PICTURE-"+name), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	pic1 := ts.item(t, &library.Item{LibraryID: photos, Kind: "photo", Path: file("one.jpg"), Title: "one"})
	pic2 := ts.item(t, &library.Item{LibraryID: photos, Kind: "photo", Path: file("two.jpg"), Title: "two"})
	ts.user(t, "boss", "adult", true, "")
	_, alice := ts.user(t, "alice", "adult", false, "")
	_, bob := ts.user(t, "bob", "adult", false, "")
	w := ts.do(t, "POST", "/api/photos/albums", alice, map[string]any{"libraryId": photos, "name": "Family", "shared": true})
	var alb struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &alb); err != nil || alb.ID == 0 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", fmt.Sprintf("/api/photos/albums/%d/items", alb.ID), alice, map[string]any{"ids": []int64{pic1.ID}}); w.Code >= 300 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	// Someone else's album isn't anyone else's to hand out.
	if w := ts.do(t, "POST", "/api/shares", bob, map[string]any{"kind": "album", "id": alb.ID, "allowDownload": true}); w.Code != 404 {
		t.Errorf("a link to someone else's album: %d %s", w.Code, w.Body)
	}
	// One made by whoever may: an administrator, who then can't see it once it's private.
	w = ts.do(t, "POST", "/api/shares", alice, map[string]any{"kind": "album", "id": alb.ID, "allowDownload": true})
	var l struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &l); err != nil || l.Token == "" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w = ts.do(t, "GET", "/api/share/"+l.Token, "", nil)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var shared map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &shared); err != nil {
		t.Fatal(err)
	}
	if _, ok := shared["owner"]; ok || strings.Contains(w.Body.String(), "alice") {
		t.Errorf("a sign-in name goes out with a link: %s", w.Body)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/share/%s/original/%d", l.Token, pic1.ID), "", nil); w.Code != 200 {
		t.Errorf("an open link: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/share/%s/original/%d", l.Token, pic2.ID), "", nil); w.Code != 404 {
		t.Errorf("a picture not in the album: %d", w.Code)
	}
	// A link of Bob's, made while he could still see it, as old links are.
	bobID := ts.userID(t, bob)
	sh, err := ts.Store.CreateShare(context.Background(), library.ShareLink{UserID: bobID, Kind: "album", TargetID: alb.ID, Title: "Family", AllowDownload: true})
	if err != nil {
		t.Fatal(err)
	}
	if w := ts.do(t, "PUT", fmt.Sprintf("/api/photos/albums/%d", alb.ID), alice, map[string]any{"name": "Family", "shared": false}); w.Code >= 300 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", fmt.Sprintf("/api/photos/albums/%d/items", alb.ID), alice, map[string]any{"ids": []int64{pic2.ID}}); w.Code >= 300 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	for _, id := range []int64{pic1.ID, pic2.ID} {
		for _, kind := range []string{"original", "image"} {
			if w := ts.do(t, "GET", fmt.Sprintf("/api/share/%s/%s/%d", sh.Token, kind, id), "", nil); w.Code != 404 || strings.Contains(w.Body.String(), "PICTURE") {
				t.Errorf("%s %d through a link after it went private: %d", kind, id, w.Code)
			}
		}
	}
	// The maker's limits from a show's genres reach their links as well.
	tv := ts.library(t, "shows", root)
	if _, err := ts.DB.Exec(`INSERT INTO shows(id, library_id, path, title, sort_title, genres, added_at, updated_at) VALUES(77, ?, '/s', 'Show', 'show', 'Horror', 1, 1)`, tv); err != nil {
		t.Fatal(err)
	}
	show := int64(77)
	ep := ts.item(t, &library.Item{LibraryID: tv, Kind: "video", Path: file("ep.mp4"), Title: "Ep", ShowID: &show, ContentRating: "G"})
	_, carol := ts.user(t, "carol", "adult", false, "")
	carolID := ts.userID(t, carol)
	sl, err := ts.Store.CreateShare(context.Background(), library.ShareLink{UserID: carolID, Kind: "item", TargetID: ep.ID, Title: "Ep", AllowDownload: true})
	if err != nil {
		t.Fatal(err)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/share/%s/original/%d", sl.Token, ep.ID), "", nil); w.Code != 200 {
		t.Fatalf("before the limit: %d %s", w.Code, w.Body)
	}
	if _, err := ts.DB.Exec(`UPDATE users SET restrictions = '{"maxAge":-1,"allowUnrated":true,"blockGenres":["Horror"]}' WHERE id = ?`, carolID); err != nil {
		t.Fatal(err)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/share/%s/original/%d", sl.Token, ep.ID), "", nil); w.Code != 404 {
		t.Errorf("after a limit on the show's genre: %d", w.Code)
	}
}

func (ts *testServer) userID(t *testing.T, tok string) int64 {
	t.Helper()
	u, err := ts.Auth.Resolve(context.Background(), tok)
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}
