// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sduistore

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

func textPage(id, title string) sdui.Page {
	return sdui.Page{ID: id, Kind: "custom", Title: title, Sections: []sdui.Section{{ID: "t", Component: "Text", Props: map[string]any{"text": title}}}}
}

func navTo(ids ...string) *sdui.Navigation {
	n := &sdui.Navigation{}
	for _, id := range ids {
		n.Items = append(n.Items, sdui.NavItem{ID: id, Type: "page", Page: id})
	}
	n.Items = append(n.Items, sdui.NavItem{ID: "settings", Type: "native", Screen: "settings"})
	return n
}

// Publishing a draft keeps what was published after the draft was begun, applies what the
// draft changed, and refuses (naming the items) when both changed the same thing.
func TestPublishMergesWithWhatWasPublishedSince(t *testing.T) {
	type edit func(c *Config)
	setPage := func(id, title string) edit {
		return func(c *Config) { c.Pages[id] = textPage(id, title) }
	}
	delPage := func(id string) edit { return func(c *Config) { delete(c.Pages, id) } }
	setNav := func(ids ...string) edit { return func(c *Config) { c.Navigation = navTo(ids...) } }
	setAccent := func(col string) edit {
		return func(c *Config) { c.Theme.Tokens = map[string]string{"accent": col} }
	}
	personal := func(off bool) edit { return func(c *Config) { c.Personalisation.Disabled = off } }
	setLayout := func(phone string) edit {
		return func(c *Config) { c.Layouts = &sdui.LayoutPolicy{Default: map[string]string{"phone": phone}} }
	}

	cases := []struct {
		name       string
		start      []edit // published before the draft is begun
		draft      []edit // the draft's changes
		published  []edit // published after the draft was begun
		prefer     string
		conflicts  []string
		wantTitles map[string]string // page → title; "" means absent
		wantAccent string
		wantNav    int // entries; 0: no navigation
		wantOff    bool
		wantLayout string // the phone's default layout; "" for none
	}{
		{name: "nothing published since: the draft as it is",
			draft: []edit{setPage("a", "A")}, wantTitles: map[string]string{"a": "A"}},
		{name: "different pages on each side: both kept",
			draft: []edit{setPage("a", "A")}, published: []edit{setPage("b", "B")}, wantTitles: map[string]string{"a": "A", "b": "B"}},
		{name: "a page the draft didn't touch keeps its published change",
			start: []edit{setPage("b", "B0")}, draft: []edit{setPage("a", "A")}, published: []edit{setPage("b", "B1")}, wantTitles: map[string]string{"a": "A", "b": "B1"}},
		{name: "a page deleted since stays deleted",
			start: []edit{setPage("b", "B")}, draft: []edit{setPage("a", "A")}, published: []edit{delPage("b")}, wantTitles: map[string]string{"a": "A", "b": ""}},
		{name: "the draft deletes a page published since unchanged",
			start: []edit{setPage("b", "B")}, draft: []edit{delPage("b")}, published: []edit{setPage("c", "C")}, wantTitles: map[string]string{"b": "", "c": "C"}},
		{name: "both made the same change: no conflict",
			draft: []edit{setPage("a", "A")}, published: []edit{setPage("a", "A")}, wantTitles: map[string]string{"a": "A"}},
		{name: "the same page changed both ways",
			start: []edit{setPage("a", "A0")}, draft: []edit{setPage("a", "Mine")}, published: []edit{setPage("a", "Theirs")}, conflicts: []string{"/pages/a"}},
		{name: "deleted on one side, changed on the other",
			start: []edit{setPage("a", "A0")}, draft: []edit{delPage("a")}, published: []edit{setPage("a", "Theirs")}, conflicts: []string{"/pages/a"}},
		{name: "navigation, theme and personalisation each conflict on their own",
			draft:     []edit{setNav("x1"), setAccent("#ff0000"), personal(true), setPage("x1", "X")},
			published: []edit{setNav("y1", "y2"), setAccent("#00ff00"), setPage("y1", "Y"), setPage("y2", "Y2"), setPage("x1", "X")},
			conflicts: []string{"/navigation", "/theme"}},
		{name: "a theme published since is kept beside the draft's navigation",
			draft: []edit{setPage("x1", "X"), setNav("x1")}, published: []edit{setAccent("#00ff00"), personal(true)},
			wantTitles: map[string]string{"x1": "X"}, wantAccent: "#00ff00", wantNav: 2, wantOff: true},
		{name: "layouts published since are kept beside the draft's page",
			draft: []edit{setPage("a", "A")}, published: []edit{setLayout("streaming")}, wantTitles: map[string]string{"a": "A"}, wantLayout: "streaming"},
		{name: "the draft's layouts are published",
			draft: []edit{setLayout("compactList")}, wantLayout: "compactList"},
		{name: "layouts changed both ways conflict",
			start: []edit{setLayout("sidebar")}, draft: []edit{setLayout("compactList")}, published: []edit{setLayout("streaming")}, conflicts: []string{"/layouts"}},
		{name: "prefer draft settles a conflict the draft's way",
			start: []edit{setPage("a", "A0")}, draft: []edit{setPage("a", "Mine")}, published: []edit{setPage("a", "Theirs"), setPage("b", "B")},
			prefer: "draft", wantTitles: map[string]string{"a": "Mine", "b": "B"}},
		{name: "prefer published settles it the published way and keeps the draft's other changes",
			start: []edit{setPage("a", "A0")}, draft: []edit{setPage("a", "Mine"), setPage("c", "C")}, published: []edit{setPage("a", "Theirs")},
			prefer: "published", wantTitles: map[string]string{"a": "Theirs", "c": "C"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := open(t, t.TempDir())
			apply := func(edits []edit) {
				t.Helper()
				if len(edits) == 0 {
					return
				}
				if _, err := s.Update("", "owner", func(c *Config) error {
					for _, e := range edits {
						e(c)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			apply(tc.start)
			began := s.Current().Revision
			d := s.Current().Clone()
			for _, e := range tc.draft {
				e(d)
			}
			saved, err := s.SaveDraft(d, "", "owner")
			if err != nil {
				t.Fatal(err)
			}
			if saved.BaseRevision != began {
				t.Fatalf("the draft remembers revision %d, want %d", saved.BaseRevision, began)
			}
			// Saved again, it still remembers where it began.
			if again, err := s.SaveDraft(saved.Clone(), saved.ETag, "owner"); err != nil || again.BaseRevision != began {
				t.Fatalf("saved again: base %d, %v", again.BaseRevision, err)
			}
			apply(tc.published)
			before := s.Current()
			pub, err := s.PublishPreferring(s.Draft().ETag, "owner", tc.prefer)
			if tc.conflicts != nil {
				var mc *MergeConflictError
				if !errors.As(err, &mc) || !slices.Equal(mc.Items, tc.conflicts) || mc.Base != began || mc.Current != before.Revision {
					t.Fatalf("want a conflict on %v, got %v (%+v)", tc.conflicts, err, mc)
				}
				if s.Current() != before || s.Draft() == nil {
					t.Error("a refused publish changed something")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for id, title := range tc.wantTitles {
				p, ok := pub.Pages[id]
				if title == "" && ok {
					t.Errorf("page %s should be gone", id)
				}
				if title != "" && p.Title != title {
					t.Errorf("page %s: %q, want %q", id, p.Title, title)
				}
			}
			if tc.wantAccent != "" && pub.Theme.Tokens["accent"] != tc.wantAccent {
				t.Errorf("accent %q", pub.Theme.Tokens["accent"])
			}
			if tc.wantNav > 0 && (pub.Navigation == nil || len(pub.Navigation.Items) != tc.wantNav) {
				t.Errorf("navigation %+v", pub.Navigation)
			}
			got := ""
			if pub.Layouts != nil {
				got = pub.Layouts.Default["phone"]
			}
			if got != tc.wantLayout && !(tc.wantLayout == "" && got == "sidebar") {
				t.Errorf("phone layout %q", got)
			}
			if pub.Personalisation.Disabled != tc.wantOff {
				t.Errorf("personalisation off %v", pub.Personalisation.Disabled)
			}
			if pub.BaseRevision != 0 || s.Draft() != nil {
				t.Error("the published configuration carries a base, or the draft is still there")
			}
		})
	}
}

func TestDraftBaseSurvivesARestartAndOldDraftsArePublishedAsBefore(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	d := s.Current().Clone()
	d.Pages["a"] = textPage("a", "A")
	if _, err := s.SaveDraft(d, "", "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update("", "owner", func(c *Config) error { c.Pages["b"] = textPage("b", "B"); return nil }); err != nil {
		t.Fatal(err)
	}
	s2 := open(t, dir)
	if s2.Draft().BaseRevision != 1 {
		t.Fatalf("base after a restart: %d", s2.Draft().BaseRevision)
	}
	pub, err := s2.Publish("", "owner")
	if err != nil || len(pub.Pages) != 2 {
		t.Fatalf("%v, %d pages", err, len(pub.Pages))
	}
	// A draft written before drafts had a base is taken to be made from what is published.
	raw := []byte(`{"version":2,"revision":1,"theme":{},"pages":{"c":{"id":"c","kind":"custom","title":"C","sections":[]}}}`)
	if err := os.WriteFile(filepath.Join(dir, DraftName), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	s3 := open(t, dir)
	if s3.Draft().BaseRevision != s3.Current().Revision {
		t.Fatalf("an old draft's base: %d", s3.Draft().BaseRevision)
	}
	if pub, err := s3.Publish("", "owner"); err != nil || len(pub.Pages) != 1 {
		t.Fatalf("an old draft replaces as before: %v", err)
	}
}

func TestPublishRefusesWhenTheBaseIsNoLongerKept(t *testing.T) {
	s := open(t, t.TempDir())
	d := s.Current().Clone()
	d.Pages["a"] = textPage("a", "A")
	if _, err := s.SaveDraft(d, "", "owner"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= MaxHistory; i++ {
		if _, err := s.Update("", "owner", func(c *Config) error { c.Pages["b"] = textPage("b", string(rune('A'+i))); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	var mc *MergeConflictError
	if _, err := s.Publish("", "owner"); !errors.As(err, &mc) || mc.Items[0] != "/" {
		t.Fatalf("%v", err)
	}
	if pub, err := s.PublishPreferring("", "owner", "draft"); err != nil || pub.Pages["a"].Title != "A" {
		t.Fatalf("prefer draft: %v", err)
	}
	if _, err := s.PublishPreferring("", "owner", "mine"); err == nil {
		t.Error("an unknown preference")
	}
}
