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
func TestShareLinksFollowTheSetAndTheShow(t *testing.T) {
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
	for _, p := range []*library.Item{pic1, pic2} { // what a scan reads from each picture
		if _, err := ts.DB.Exec(`INSERT INTO photo_meta(item_id, library_id, taken_at, ym) VALUES(?, ?, 1700000000, 202311)`, p.ID, photos); err != nil {
			t.Fatal(err)
		}
	}
	ts.user(t, "boss", "adult", true, "")
	_, alice := ts.user(t, "alice", "adult", false, "")
	_, bob := ts.user(t, "bob", "adult", false, "")
	// A set has a title, and only pictures its maker may see.
	for _, bad := range []map[string]any{
		{"kind": "photos", "ids": []int64{pic1.ID}},
		{"kind": "photos", "ids": []int64{}, "title": "Family"},
		{"kind": "photos", "ids": []int64{pic1.ID}, "title": "two\nlines"},
		{"kind": "album", "id": 1},
	} {
		if w := ts.do(t, "POST", "/api/shares", alice, bad); w.Code != 400 {
			t.Errorf("%v: %d %s", bad, w.Code, w.Body)
		}
	}
	if w := ts.do(t, "POST", "/api/shares", alice, map[string]any{"kind": "photos", "ids": []int64{pic1.ID, 99999}, "title": "Family"}); w.Code != 404 {
		t.Errorf("a set naming what isn't there: %d %s", w.Code, w.Body)
	}
	w := ts.do(t, "POST", "/api/shares", alice, map[string]any{"kind": "photos", "ids": []int64{pic1.ID, pic1.ID}, "title": "Family", "allowDownload": true})
	var l struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &l); err != nil || l.Token == "" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w = ts.do(t, "GET", "/api/share/"+l.Token, "", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"title":"Family"`) || !strings.Contains(w.Body.String(), `"kind":"photos"`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var shared map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &shared); err != nil {
		t.Fatal(err)
	}
	if items, _ := shared["items"].([]any); len(items) != 1 {
		t.Errorf("the set: %s", w.Body)
	}
	if _, ok := shared["owner"]; ok || strings.Contains(w.Body.String(), "alice") {
		t.Errorf("a sign-in name goes out with a link: %s", w.Body)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/share/%s/original/%d", l.Token, pic1.ID), "", nil); w.Code != 200 {
		t.Errorf("an open link: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/share/%s/original/%d", l.Token, pic2.ID), "", nil); w.Code != 404 {
		t.Errorf("a picture not in the set: %d", w.Code)
	}
	// A link of Bob's to a set, made while he could see it: once he can't, it shows nothing.
	bobID := ts.userID(t, bob)
	sh, err := ts.Store.CreateShare(context.Background(), library.ShareLink{UserID: bobID, Kind: "photos", Items: []int64{pic1.ID, pic2.ID}, Title: "Family", AllowDownload: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ts.DB.Exec(`UPDATE users SET restrictions = ? WHERE id = ?`, fmt.Sprintf(`{"maxAge":-1,"allowUnrated":true,"blockItems":[%d,%d]}`, pic1.ID, pic2.ID), bobID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{pic1.ID, pic2.ID} {
		for _, kind := range []string{"original", "image"} {
			if w := ts.do(t, "GET", fmt.Sprintf("/api/share/%s/%s/%d", sh.Token, kind, id), "", nil); w.Code != 404 || strings.Contains(w.Body.String(), "PICTURE") {
				t.Errorf("%s %d through a link after its maker lost it: %d", kind, id, w.Code)
			}
		}
	}
	if w := ts.do(t, "GET", "/api/share/"+sh.Token, "", nil); w.Code != 200 || strings.Contains(w.Body.String(), `"one"`) {
		t.Errorf("the set after its maker lost it: %d %s", w.Code, w.Body)
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
