// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

func pageNode(n *sdui.Node, id string) *sdui.Node {
	if n.ID == id {
		return n
	}
	for i := range n.Children {
		if f := pageNode(&n.Children[i], id); f != nil {
			return f
		}
	}
	return nil
}

func pageHasType(n *sdui.Node, typ string) bool {
	if n.Type == typ {
		return true
	}
	for i := range n.Children {
		if pageHasType(&n.Children[i], typ) {
			return true
		}
	}
	return false
}

// The pages every app draws from the server's data: a studio (with the profile's limits), a title's versions,
// the lists it can be saved to, devices, and the rest, each served as an ordinary SDUI page.
func TestUtilityPages(t *testing.T) {
	f := newCreditsFixture(t)
	ts := f.ts
	ctx := t.Context()
	root := t.TempDir()
	lib := ts.library(t, "movies", root)
	y := 2022
	for i, rating := range []string{"G", "R"} {
		it := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: filepath.Join(root, fmt.Sprintf("%d.mkv", i)), Title: fmt.Sprintf("Harbour %d", i), Year: &y, ContentRating: rating})
		if _, err := ts.DB.ExecContext(ctx, `INSERT INTO title_brands(kind, key, name, library_id, item_id) VALUES('studio', ?, 'Harbour Pictures', ?, ?)`, library.BrandKey("Harbour Pictures"), lib, it.ID); err != nil {
			t.Fatal(err)
		}
	}
	// A studio: the kind of profile decides what is in it, and the count is in the heading.
	adult := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/studio?name=Harbour+Pictures", f.adult, nil))
	films := pageNode(&adult.Root, "films")
	if films == nil || films.Props["title"] != "Movies (2)" || len(films.Children) != 2 || adult.Page.Title != "Harbour Pictures" {
		t.Fatalf("an adult's studio page: %+v %q", films, adult.Page.Title)
	}
	kid := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/studio?name=Harbour+Pictures", f.kid, nil))
	if films := pageNode(&kid.Root, "films"); films == nil || len(films.Children) != 1 || films.Props["title"] != "Movies (1)" {
		t.Errorf("a kid sees %+v of the studio", films)
	}
	// Versions: one file here, and its row plays it.
	v := sduiDoc(t, ts.do(t, "GET", fmt.Sprintf("/api/sdui/pages/versions?item=%d", f.film.ID), f.adult, nil))
	files := pageNode(&v.Root, "files")
	if files == nil || len(files.Children) != 1 || files.Children[0].Action == nil || files.Children[0].Action.Type != sdui.ActPlay || v.Page.Title != "Robot Friend" {
		t.Fatalf("versions: %+v %q", files, v.Page.Title)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/sdui/pages/versions?item=%d", f.grownUp.ID), f.kid, nil); w.Code != 200 {
		t.Errorf("a title a kid can't see: %d", w.Code)
	} else if d := sduiDoc(t, w); pageNode(&d.Root, "files") != nil && len(pageNode(&d.Root, "files").Children) > 0 {
		t.Errorf("a kid is shown the versions of a title it may not see")
	}
	// Save to a list: a list of the profile's own as a switch that adds the title.
	if w := ts.do(t, "POST", "/api/collections", f.adult, map[string]any{"name": "Friday night", "kind": "manual"}); w.Code != 201 {
		t.Fatalf("a list: %d %s", w.Code, w.Body)
	}
	sv := sduiDoc(t, ts.do(t, "GET", fmt.Sprintf("/api/sdui/pages/save-to?item=%d", f.film.ID), f.adult, nil))
	lists := pageNode(&sv.Root, "lists")
	if lists == nil || len(lists.Children) == 0 {
		t.Fatalf("save to: no lists: %+v", sv.Root)
	}
	row := lists.Children[0]
	c, _ := row.Props["control"].(map[string]any)
	if c == nil || c["type"] != sdui.ControlSwitch || c["value"] != false || row.Action == nil || row.Action.Op != "collection.add" || row.Action.Item != fmt.Sprint(f.film.ID) {
		t.Errorf("save to row: %+v %+v", row.Props, row.Action)
	}
	if form := pageNode(&sv.Root, "new"); form == nil || form.Action == nil || form.Action.Op != "collection.create" || !pageHasType(form, "ListItem") {
		t.Errorf("save to has no form for a new list: %+v", form)
	}
	// Devices: this device first, with nothing to sign out.
	dv := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/devices", f.adult, nil))
	if list := pageNode(&dv.Root, "list"); list == nil || len(list.Children) == 0 || list.Children[0].Action != nil {
		t.Errorf("devices: %+v", list)
	}
	if b := pageNode(&dv.Root, "others"); b == nil || b.Action == nil || b.Action.Op != "sessions.endOthers" || b.Action.Confirm == "" {
		t.Errorf("sign out the others: %+v", b)
	}
	for _, page := range []string{"year", "activity", "notifications"} {
		if w := ts.do(t, "GET", "/api/sdui/pages/"+page, f.adult, nil); w.Code != 200 {
			t.Errorf("%s: %d %s", page, w.Code, w.Body)
		}
	}
	// Search is scoped by tabs; every page on a television carries the key legend in a Dock.
	se := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/search?q=Robot", f.adult, nil))
	if tabs := pageNode(&se.Root, "scope"); tabs == nil || len(tabs.Children) < 5 {
		t.Errorf("search has no scope tabs: %+v", tabs)
	}
	for _, page := range []string{"home", fmt.Sprintf("detail?item=%d", f.film.ID), "devices"} {
		sep := "?"
		if len(page) > 6 && page[:6] == "detail" {
			sep = "&"
		}
		d := sduiDoc(t, ts.do(t, "GET", "/api/sdui/pages/"+page+sep+"platform=tvos", f.adult, nil))
		dock := pageNode(&d.Root, "dock")
		if dock == nil && !pageHasType(&d.Root, "Dock") {
			t.Errorf("%s on a television has no Dock", page)
		}
		if !pageHasType(&d.Root, "KeyLegend") {
			t.Errorf("%s on a television has no key legend", page)
		}
	}
}

// A book in more than one format lists the others beside it, with sizes and never paths; the bookshelf's
// reading filter keeps what has been started.
func TestBookFormatsAndReadingFilter(t *testing.T) {
	f := newCreditsFixture(t)
	ts := f.ts
	root := t.TempDir()
	lib := ts.library(t, "books", root)
	write := func(name string, n int) string {
		p := filepath.Join(root, name)
		if err := os.WriteFile(p, make([]byte, n), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a := ts.item(t, &library.Item{LibraryID: lib, Kind: "book", Path: write("Tide.epub", 2048), Title: "Tide", Container: "epub"})
	write("Tide.pdf", 4096)
	ts.item(t, &library.Item{LibraryID: lib, Kind: "book", Path: write("Orchard.epub", 1024), Title: "Orchard", Container: "epub"})
	_, admin := ts.user(t, "shelf-admin", "adult", true, "")
	etag := ts.do(t, "GET", "/api/sdui/config", admin, nil).Header().Get("ETag")
	pg := sdui.Page{ID: "book-formats", Kind: "custom", Title: "{title}", Sections: []sdui.Section{{ID: "fmt", Component: "List", Source: sdui.Source{"type": "formats", "item": "$item"}}}}
	if w := ts.do(t, "PUT", "/api/sdui/config/pages/book-formats", admin, pg, "If-Match", etag); w.Code != 200 {
		t.Fatal(w.Body)
	}
	d := sduiDoc(t, ts.do(t, "GET", fmt.Sprintf("/api/sdui/pages/book-formats?item=%d", a.ID), f.adult, nil))
	rows := pageNode(&d.Root, "fmt")
	if rows == nil || len(rows.Children) != 2 || rows.Children[0].Props["title"] != "EPUB" || rows.Children[1].Props["title"] != "PDF" {
		t.Fatalf("a book's formats: %+v", rows)
	}
	if strings.Contains(fmt.Sprint(rows.Children[0].Props, rows.Children[1].Props), root) {
		t.Errorf("a path leaked into the formats")
	}
	// The bookshelf's filters: nothing is being read, so Reading is empty and Not started holds both books.
	shelf := func(q string) *sdui.Node {
		d := sduiDoc(t, ts.do(t, "GET", fmt.Sprintf("/api/sdui/pages/library?library=%d&%s", lib, q), f.adult, nil))
		return pageNode(&d.Root, "all")
	}
	if g := shelf("reading=unread"); g == nil || len(g.Children) != 2 {
		t.Errorf("not started: %+v", g)
	}
	if g := shelf("reading=reading"); g != nil && len(g.Children) != 0 {
		t.Errorf("reading: %+v", g)
	}
	if g := shelf("format=pdf"); g != nil && len(g.Children) != 0 {
		t.Errorf("pdf: %+v", g)
	}
}
