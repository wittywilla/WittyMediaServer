// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite golden files")

func sampleItems() []Item {
	return []Item{
		{ID: "m1", Title: "Metropolis", Subtitle: "1927", ImageURL: "/img/m1.jpg", Kind: "movie", Progress: 0.4, Badges: []string{"HD"}},
		{ID: "g1", Title: "Dragon Blaster", Kind: "game", Module: "wittyplay"},
	}
}

func TestTemplates_Validate(t *testing.T) {
	cases := map[string]Document{
		"home-empty":        HomeTemplate(HomeData{}),
		"home-full":         HomeTemplate(HomeData{UserName: "Willow", ContinueWatching: sampleItems(), RecentlyAdded: sampleItems(), Recommended: sampleItems(), Shelves: []ShelfData{{Title: "Untitled id", Items: sampleItems()}, {ID: "s2", Title: "Games", Module: "wittyplay", Items: sampleItems()}}, Modules: []string{"wittyplay"}}),
		"library-empty":     LibraryTemplate(LibraryData{}),
		"library-full":      LibraryTemplate(LibraryData{LibraryID: "lib1", Title: "Films", Kind: "movies", Items: sampleItems(), Sort: "title"}),
		"detail-empty":      DetailTemplate(DetailData{}),
		"detail-full":       DetailTemplate(DetailData{Item: sampleItems()[0], Overview: "A city.", Year: 1927, Duration: 9000, Rating: "PG", Genres: []string{"Sci-fi", "Drama"}, Backdrop: "/bd.jpg", Related: sampleItems(), Children: sampleItems(), CanPlay: true, Progress: 0.4}),
		"player-empty":      PlayerTemplate(PlayerData{}),
		"player-full":       PlayerTemplate(PlayerData{Item: sampleItems()[0], StreamURL: "/stream/m1.m3u8", Poster: "/p.jpg", StartAt: 120, Next: &sampleItems()[1], Subtitles: []string{"en"}}),
		"player-next-no-id": PlayerTemplate(PlayerData{Next: &Item{Title: "x"}}),
		"settings-empty":    SettingsTemplate(SettingsData{}),
		"settings-full": SettingsTemplate(SettingsData{Version: "0.8.0", Sections: []SettingsSection{
			{Title: "No id", Rows: []Item{{ID: "a", Title: "A", Subtitle: "sub"}}},
			{ID: "adm", Title: "Admin", Admin: true, Rows: []Item{{ID: "b", Title: "B", Module: "wittytv"}}},
		}}),
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Validate(&doc); err != nil {
				t.Fatalf("%v", err)
			}
			// Must survive a JSON round trip through strict decoding.
			b, err := Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			back, err := Unmarshal(b)
			if err != nil {
				t.Fatal(err)
			}
			c1, _ := Canonical(doc)
			c2, _ := Canonical(back)
			if !bytes.Equal(c1, c2) {
				t.Error("template changed across JSON round trip")
			}
			// Renders for a bare guest and a loaded admin without error.
			for _, ctx := range []RenderContext{{}, baseCtx()} {
				if _, err := Render(doc, ctx); err != nil {
					t.Errorf("render: %v", err)
				}
			}
		})
	}
}

func TestTemplates_AreRealSDUI(t *testing.T) {
	var conds, binds int
	var walk func(Node)
	walk = func(n Node) {
		if n.Visibility != nil {
			conds++
		}
		binds += len(n.Bindings)
		for _, c := range n.Children {
			walk(c)
		}
	}
	for name, d := range DefaultScreens() {
		conds, binds = 0, 0
		walk(d.Root)
		if conds == 0 || binds == 0 {
			t.Errorf("%s: expected visibility conditions and bindings, got %d/%d", name, conds, binds)
		}
	}
}

func TestDefaultScreens(t *testing.T) {
	screens := DefaultScreens()
	for _, want := range []string{"home", "library", "detail", "player", "settings"} {
		d, ok := screens[want]
		if !ok {
			t.Fatalf("missing screen %q", want)
		}
		if d.Screen != want {
			t.Errorf("screen %q has Screen=%q", want, d.Screen)
		}
		if err := Validate(&d); err != nil {
			t.Errorf("%s: %v", want, err)
		}
	}
}

func TestHomeTemplate_ModuleGating(t *testing.T) {
	doc := HomeTemplate(HomeData{})
	find := func(d Document, id string) bool { return findNode(&d.Root, id) != nil }

	guest, _ := Render(doc, RenderContext{Platform: "web", Role: "guest"})
	if find(guest, "home-games") || find(guest, "home-livetv") || find(guest, "home-continue") || find(guest, "home-admin-setup") {
		t.Error("guest without modules should not see gated shelves")
	}
	admin, _ := Render(doc, RenderContext{Platform: "web", Role: "admin", Modules: []string{"wittyplay", "wittytv"},
		Features: []string{"history"}, Flags: map[string]bool{"setupIncomplete": true}})
	if !find(admin, "home-games") || !find(admin, "home-livetv") || !find(admin, "home-continue") || !find(admin, "home-admin-setup") {
		t.Error("admin with modules should see gated shelves")
	}
	if find(admin, "home-music") {
		t.Error("music shelf should be hidden without wittymusic")
	}
	g := findNode(&admin.Root, "home-games")
	if g.Style == nil || g.Style.Module != "wittyplay" {
		t.Error("games shelf should be scoped to wittyplay")
	}
}

func TestSettingsTemplate_AdminGating(t *testing.T) {
	doc := SettingsTemplate(SettingsData{})
	user, _ := Render(doc, RenderContext{Role: "user"})
	tabs := findNode(&user.Root, "settings-tabs")
	if tabs == nil || len(tabs.Children) != 1 {
		t.Fatalf("user should see exactly the General tab, got %+v", tabs)
	}
	admin, _ := Render(doc, RenderContext{Role: "admin", Modules: []string{"wittyplay"}})
	tabs = findNode(&admin.Root, "settings-tabs")
	if len(tabs.Children) != 4 {
		t.Fatalf("admin should see 4 tabs, got %d", len(tabs.Children))
	}
	if findNode(&admin.Root, "settings-modules-row-2") != nil || findNode(&admin.Root, "settings-modules-row-1") == nil {
		t.Error("module rows should be gated on enabled modules")
	}
	guest, _ := Render(doc, RenderContext{Role: "guest"})
	if findNode(&guest.Root, "settings-signout") != nil {
		t.Error("guest should not see sign out")
	}
}

func TestHomeTemplate_Golden(t *testing.T) {
	doc := HomeTemplate(HomeData{
		UserName:         "Willow",
		ContinueWatching: sampleItems()[:1],
		RecentlyAdded:    sampleItems(),
		Shelves:          []ShelfData{{ID: "home-retro", Title: "Retro picks", Module: "wittyplay", Items: sampleItems()[1:]}},
	})
	ctx := RenderContext{
		Platform: "web", Role: "admin",
		Features: []string{"history", "recommendations"},
		Modules:  []string{"wittyplay", "wittytv"},
		Viewport: Viewport{Width: 1440, Height: 900},
		Locale:   "en-US",
		Now:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Data: map[string]any{
			"user": map[string]any{"name": "willow"},
			"home": map[string]any{
				"featured":      []any{map[string]any{"title": "Metropolis", "itemId": "m1"}},
				"recentlyAdded": []any{map[string]any{"title": "Metropolis"}},
				"games":         []any{map[string]any{"title": "Dragon Blaster"}},
			},
		},
		ThemeOverrides: map[string]map[string]string{"wittyplay": {"accent": "#3ad29f"}},
	}
	rendered, err := Render(doc, ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Canonical(rendered)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "home.golden.json")
	if *update {
		pretty, _ := MarshalIndent(rendered)
		if err := os.WriteFile(path, append(pretty, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	want, err := Unmarshal(raw)
	if err != nil {
		t.Fatalf("golden is not a valid document: %v", err)
	}
	wantC, _ := Canonical(want)
	if !bytes.Equal(got, wantC) {
		t.Errorf("rendered home differs from golden (re-run with -update if intentional)\n got: %s\nwant: %s", got, wantC)
	}
}
