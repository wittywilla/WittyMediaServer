// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/search"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

type searchAnswer struct {
	Query  string  `json:"query"`
	TookMs float64 `json:"tookMs"`
	Groups []struct {
		Key   string      `json:"key"`
		Label string      `json:"label"`
		Items []sdui.Item `json:"items"`
	} `json:"groups"`
}

func (a searchAnswer) titles() []string {
	var out []string
	for _, g := range a.Groups {
		for _, it := range g.Items {
			out = append(out, g.Key+":"+it.Title)
		}
	}
	return out
}

func searchFor(t *testing.T, ts *testServer, token, query string) searchAnswer {
	t.Helper()
	w := ts.do(t, "GET", "/api/search?q="+strings.ReplaceAll(query, " ", "+"), token, nil)
	if w.Code != 200 {
		t.Fatalf("search %q: %d %s", query, w.Code, w.Body)
	}
	var a searchAnswer
	if err := json.Unmarshal(w.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	return a
}

// The one search finds a title with a letter wrong, and only what the profile may see.
func TestSearchAcrossLibraries(t *testing.T) {
	f := newSDUIFixture(t)
	ts := f.ts
	if got := searchFor(t, ts, f.admin, "grwon-up").titles(); !slices.Equal(got, []string{"films:Grown-up Film"}) {
		t.Errorf("two letters swapped: %v", got)
	}
	if got := searchFor(t, ts, f.admin, "film").titles(); len(got) != 3 {
		t.Errorf("the administrator sees every film: %v", got)
	}
	if got := searchFor(t, ts, f.oneLibrary, "film").titles(); slices.Contains(got, "films:Secret Film") || len(got) != 2 {
		t.Errorf("a library the profile can't open is never searched: %v", got)
	}
	if got := searchFor(t, ts, f.kid, "film").titles(); slices.Contains(got, "films:Grown-up Film") || !slices.Contains(got, "films:Family Film") {
		t.Errorf("a title rated above a kid's limit is never found: %v", got)
	}
	if got := searchFor(t, ts, f.admin, "f").titles(); len(got) != 0 {
		t.Errorf("one letter finds nothing: %v", got)
	}

	// A title added later is found once its library's part of the index is rebuilt (after a
	// scan, or when the stamps show a change).
	root := t.TempDir()
	p := filepath.Join(root, "Interstellar.mkv")
	put(t, p, []byte("film"))
	ts.item(t, &library.Item{LibraryID: f.open, Kind: "video", Path: p, Title: "Interstellar", ContentRating: "PG"})
	if err := ts.refreshSearch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := searchFor(t, ts, f.admin, "intersteller").titles(); !slices.Equal(got, []string{"films:Interstellar"}) {
		t.Errorf("a new title, misspelt: %v", got)
	}
}

// Recent searches belong to the profile; typing a search on doesn't fill the list.
func TestRecentSearches(t *testing.T) {
	f := newSDUIFixture(t)
	ts := f.ts
	for _, q := range []string{"fam", "family", "grown", "grown"} {
		ts.do(t, "GET", "/api/search?remember=1&q="+q, f.admin, nil)
	}
	ts.do(t, "GET", "/api/search?q=secret", f.admin, nil) // not asked to be remembered
	recent := func(token string) []string {
		var r struct{ Recent []string }
		_ = json.Unmarshal(ts.do(t, "GET", "/api/search/recent", token, nil).Body.Bytes(), &r)
		return r.Recent
	}
	if got := recent(f.admin); !slices.Equal(got, []string{"grown", "family"}) {
		t.Errorf("recent: %v", got)
	}
	if got := recent(f.kid); len(got) != 0 {
		t.Errorf("another profile's searches: %v", got)
	}
	// The search page, with nothing typed, lists them; with a search, keeps it.
	page := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/search?q=Secret", f.admin, nil))
	_ = page
	idle := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/search", f.admin, nil))
	if n := findNode(&idle.Root, "recent"); n == nil || len(n.Children) != 3 || n.Children[0].Props["title"] != "Secret" {
		t.Errorf("the idle page's recent searches: %+v", n)
	}
	if w := ts.do(t, "DELETE", "/api/search/recent?q=family", f.admin, nil); w.Code != 204 {
		t.Fatalf("forget one: %d", w.Code)
	}
	if got := recent(f.admin); slices.Contains(got, "family") || len(got) != 2 {
		t.Errorf("one forgotten: %v", got)
	}
	if findNode(&idle.Root, "forget") == nil {
		t.Errorf("a button clears them")
	}
	ts.do(t, "DELETE", "/api/v1/search/recent", f.admin, nil) // the page's command, search.forgetRecent
	if got := recent(f.admin); len(got) != 0 {
		t.Errorf("all forgotten: %v", got)
	}
	idle = sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/search", f.admin, nil))
	if findNode(&idle.Root, "recent") != nil || findNode(&idle.Root, "forget") != nil {
		t.Errorf("nothing to list or clear: %+v", idle.Root.Children)
	}
}

// The groups the server's index makes are the ones the page catalogue names.
func TestSearchGroupsMatchCatalogue(t *testing.T) {
	if !slices.Equal(search.Groups, sdui.SearchGroups) {
		t.Errorf("internal/search Groups %v and sdui.SearchGroups %v differ", search.Groups, sdui.SearchGroups)
	}
	for _, g := range search.Groups {
		if search.GroupLabels[g] == "" {
			t.Errorf("group %s has no label", g)
		}
	}
}
