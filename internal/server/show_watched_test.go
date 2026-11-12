// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// showWorld: show 5 (G) with two G episodes and one R episode, show 6 (R) with one.
func showWorld(t *testing.T) (ts *testServer, kid, adult string, open []int64, hidden int64) {
	t.Helper()
	ts = newTestServer(t)
	root := t.TempDir()
	tv := ts.library(t, "shows", root)
	for _, q := range []string{
		`INSERT INTO shows(id, library_id, path, title, sort_title, content_rating, added_at, updated_at) VALUES(5, ?, '/s5', 'Open show', 'open show', 'G', 1, 1)`,
		`INSERT INTO shows(id, library_id, path, title, sort_title, content_rating, added_at, updated_at) VALUES(6, ?, '/s6', 'Grown-up show', 'grown-up show', 'R', 1, 1)`,
	} {
		if _, err := ts.DB.Exec(q, tv); err != nil {
			t.Fatal(err)
		}
	}
	five, six := int64(5), int64(6)
	one, two, three := 1, 2, 3
	ep := func(show *int64, n *int, rating string) int64 {
		return ts.item(t, &library.Item{LibraryID: tv, Kind: "video", Path: filepath.Join(root, fmt.Sprintf("%d-%d.mkv", *show, *n)), ShowID: show, Season: &one, Episode: n, ContentRating: rating, RuntimeSec: 1200}).ID
	}
	open = []int64{ep(&five, &one, "G"), ep(&five, &three, "G")}
	hidden = ep(&five, &two, "R")
	ep(&six, &one, "R")
	ts.user(t, "boss", "adult", true, "")
	_, kid = ts.user(t, "kid", "kids", false, "")
	_, adult = ts.user(t, "bob", "adult", false, "")
	return
}

func watchedCount(t *testing.T, ts *testServer, tok string, ids ...int64) int {
	t.Helper()
	n := 0
	for _, id := range ids {
		var w int
		_ = ts.DB.QueryRow(`SELECT watched FROM progress WHERE user_id = ? AND item_id = ?`, ts.userID(t, tok), id).Scan(&w)
		n += w
	}
	return n
}

// The commands the server-driven UI sends for a show work: watched.set and
// watched.clear on show-<n> mark the episodes the profile may see, and nothing else.
func TestShowWatchedCommands(t *testing.T) {
	ts, kid, adult, open, hidden := showWorld(t)
	cases := []struct {
		name, method, path, tok string
		body                    any
		code                    int
		wantOpen, wantHidden    int
	}{
		{"a kid marks a show watched", "POST", "/api/items/show-5/progress", kid, map[string]any{"watched": true}, 200, 2, 0},
		{"a show takes only watched:true", "POST", "/api/items/show-5/progress", kid, map[string]any{"positionSec": 3}, 400, 2, 0},
		{"a show the kid may not see", "POST", "/api/items/show-6/progress", kid, map[string]any{"watched": true}, 404, 2, 0},
		{"a show that isn't there", "POST", "/api/items/show-99/progress", kid, map[string]any{"watched": true}, 404, 2, 0},
		{"clearing a show the kid may not see", "DELETE", "/api/items/show-6/progress", kid, nil, 404, 2, 0},
		{"the kid clears the show", "DELETE", "/api/items/show-5/progress", kid, nil, 200, 0, 0},
		{"not a show: still a bad id", "POST", "/api/items/show-x/progress", kid, map[string]any{"watched": true}, 400, 0, 0},
	}
	for _, tc := range cases {
		w := ts.do(t, tc.method, tc.path, tc.tok, tc.body)
		if w.Code != tc.code {
			t.Errorf("%s: %d %s", tc.name, w.Code, w.Body)
		}
		if got := watchedCount(t, ts, kid, open...); got != tc.wantOpen {
			t.Errorf("%s: %d open episodes watched, want %d", tc.name, got, tc.wantOpen)
		}
		if got := watchedCount(t, ts, kid, hidden); got != tc.wantHidden {
			t.Errorf("%s: the episode the kid may not see was marked", tc.name)
		}
	}
	// An adult sees every episode; their marks are theirs alone.
	if w := ts.do(t, "POST", "/api/items/show-5/progress", adult, map[string]any{"watched": true}); w.Code != 200 || watchedCount(t, ts, adult, append(open, hidden)...) != 3 {
		t.Errorf("an adult: %d %s", w.Code, w.Body)
	}
	if watchedCount(t, ts, kid, open...) != 0 {
		t.Error("an adult's marks reached the kid")
	}
}

// Watch later takes every form a title comes in, so a command template's body works for
// a show as for a film.
func TestWatchLaterTakesSDUIIDs(t *testing.T) {
	ts, kid, _, open, _ := showWorld(t)
	cases := []struct {
		name, body string
		code       int
	}{
		{"a show by its SDUI ID", `{"items":["show-5"]}`, 200},
		{"a show as a number under shows", `{"shows":[5]}`, 200},
		{"a show as a string under shows", `{"shows":["show-5"]}`, 200},
		{"an episode as a string", fmt.Sprintf(`{"items":["%d"]}`, open[0]), 200},
		{"an episode as a number", fmt.Sprintf(`{"items":[%d]}`, open[1]), 200},
		{"nonsense", `{"items":["abc"]}`, 400},
		{"an empty show ID", `{"items":["show-"]}`, 400},
		{"an object", `{"items":[{"id":5}]}`, 400},
		{"nothing", `{"items":[]}`, 400},
	}
	for _, tc := range cases {
		if w := ts.do(t, "POST", "/api/collections/watch-later/items", kid, []byte(tc.body)); w.Code != tc.code {
			t.Errorf("%s: %d %s", tc.name, w.Code, w.Body)
		}
	}
	if held := decode[[]int64](t, ts.do(t, "GET", "/api/collections/holding?show=5", kid, nil)); len(held) != 1 {
		t.Errorf("the lists holding the show: %v", held)
	}
	// The catalogue's own template, filled in for a show, is accepted as it is.
	for _, cmd := range sdui.Catalog().Commands {
		if cmd.Op != "watchLater.add" {
			continue
		}
		body, _ := json.Marshal("show-5")
		filled := strings.ReplaceAll(cmd.Body, `"{item}"`, string(body))
		path := strings.TrimPrefix(cmd.Path, "/api/v1")
		if w := ts.do(t, cmd.Method, "/api"+strings.ReplaceAll(path, "{item}", url.PathEscape("show-5")), kid, []byte(filled)); w.Code != 200 {
			t.Errorf("the catalogue's template for a show: %s → %d %s", filled, w.Code, w.Body)
		}
	}
}

// A title's page says whether it is on Watch later, and its button takes it off again: for a
// show and for a film, with the catalogue's own command templates, the page asked for again at
// once as an app does after a command.
func TestWatchLaterIsAToggleOnATitlesPage(t *testing.T) {
	ts, _, adult, _, _ := showWorld(t)
	films := t.TempDir()
	film := ts.item(t, &library.Item{LibraryID: ts.library(t, "movies", films), Kind: "video", Title: "A film", Path: filepath.Join(films, "film.mkv"), ContentRating: "G", RuntimeSec: 5400}).ID
	button := func(page string) map[string]any {
		t.Helper()
		doc := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/detail?"+page, adult, nil))
		var find func(n *sdui.Node) *sdui.Node
		find = func(n *sdui.Node) *sdui.Node {
			if n.Type == "DetailHeader" {
				return n
			}
			for i := range n.Children {
				if got := find(&n.Children[i]); got != nil {
					return got
				}
			}
			return nil
		}
		h := find(&doc.Root)
		if h == nil {
			t.Fatalf("%s: no DetailHeader", page)
		}
		acts, _ := h.Props["actions"].([]any)
		for _, a := range acts {
			if m, _ := a.(map[string]any); m["id"] == "watchLater" {
				return m
			}
		}
		t.Fatalf("%s: no Watch later button in %v", page, acts)
		return nil
	}
	op := func(b map[string]any) string {
		a, _ := b["action"].(map[string]any)
		op, _ := a["op"].(string)
		return op
	}
	run := func(name, item string) {
		t.Helper()
		for _, cmd := range sdui.Catalog().Commands {
			if cmd.Op != name {
				continue
			}
			body, _ := json.Marshal(item)
			filled := strings.ReplaceAll(cmd.Body, `"{item}"`, string(body))
			if w := ts.do(t, cmd.Method, "/api"+strings.TrimPrefix(cmd.Path, "/api/v1"), adult, []byte(filled)); w.Code != 200 {
				t.Fatalf("%s %s: %d %s", name, item, w.Code, w.Body)
			}
			return
		}
		t.Fatalf("no %s command in the catalogue", name)
	}
	for _, c := range []struct{ page, item string }{{"show=5", "show-5"}, {fmt.Sprintf("item=%d", film), fmt.Sprint(film)}} {
		if b := button(c.page); op(b) != "watchLater.add" || b["pressed"] != false || b["label"] != "Watch later" {
			t.Errorf("%s, not on the list: %v", c.page, b)
		}
		run("watchLater.add", c.item)
		if b := button(c.page); op(b) != "watchLater.remove" || b["pressed"] != true || b["label"] != "In Watch later" {
			t.Errorf("%s, on the list: %v", c.page, b)
		}
		run("watchLater.remove", c.item)
		if b := button(c.page); op(b) != "watchLater.add" || b["pressed"] != false {
			t.Errorf("%s, taken off: %v", c.page, b)
		}
		// Taking off what isn't there is no error.
		run("watchLater.remove", c.item)
	}
	if held := decode[[]int64](t, ts.do(t, "GET", "/api/collections/holding?show=5", adult, nil)); len(held) != 0 {
		t.Errorf("still held: %v", held)
	}
	// A guest keeps no lists, either way.
	_, guest := ts.user(t, "visitor", "guest", false, "")
	if w := ts.do(t, "DELETE", "/api/collections/watch-later/items", guest, []byte(`{"items":["show-5"]}`)); w.Code != 403 {
		t.Errorf("a guest: %d", w.Code)
	}
}
