// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"strconv"
	"strings"
	"testing"
)

// A books library's series as a repeat: names that are the same once cut down to what an ID
// may hold ("Ōoku" and "Ōku" are both "x" to safeID) still give copies of their own, through
// Each.ID, and a copy's next page is found again by its ID.
type bookShelf struct {
	oneItem
	asked []Source
}

func (b *bookShelf) Each(over string, _ []string, _ int) ([]Each, error) {
	if over != "bookSeries" {
		return nil, nil
	}
	return []Each{{Key: "Ōoku", Label: "Ōoku", ID: "a1"}, {Key: "Ōku", Label: "Ōku", ID: "b2"}, {Key: BooksNone, Label: "No series", ID: "c3"}}, nil
}

func (b *bookShelf) Items(req SourceRequest) (SourceResult, error) {
	b.asked = append(b.asked, req.Source)
	items := make([]Item, 0, req.Limit)
	for i := req.Offset; i < 40 && len(items) < req.Limit; i++ {
		items = append(items, Item{ID: "b" + strconv.Itoa(i), Kind: "book", Title: "Book", Route: "/item/" + strconv.Itoa(i)})
	}
	return SourceResult{Items: items, More: req.Offset+len(items) < 40, Total: 40}, nil
}

func TestBookSeriesRepeatsGiveACopyEach(t *testing.T) {
	page, ok := StockPage("library-books")
	if !ok {
		t.Fatal("no library-books page")
	}
	if issues := ValidatePage(&page); len(issues) > 0 {
		t.Fatalf("the built-in books page has issues: %v", issues)
	}
	res := &bookShelf{}
	ctx := &RenderContext{Profile: ProfileFor(FormTV, ""), Resolver: res, Vars: map[string]string{"library": "5"}, Params: map[string]string{"tab": "series"}}
	doc, err := Render(page, ctx)
	if err != nil {
		t.Fatal(err)
	}
	var shelves []Node
	var walk func(n *Node)
	walk = func(n *Node) {
		if n.Type == "Shelf" && strings.HasPrefix(n.ID, "in-series-") {
			shelves = append(shelves, *n)
		}
		for i := range n.Children {
			walk(&n.Children[i])
		}
	}
	walk(&doc.Root)
	if len(shelves) != 3 {
		t.Fatalf("got %d series rows, want 3", len(shelves))
	}
	seen := map[string]bool{}
	for i, want := range []string{"Ōoku", "Ōku", "No series"} {
		if got := shelves[i].Props["title"]; got != want {
			t.Errorf("row %d is titled %v, want %s", i, got, want)
		}
		if seen[shelves[i].ID] {
			t.Errorf("two rows share the ID %s", shelves[i].ID)
		}
		seen[shelves[i].ID] = true
	}
	var series []string
	for _, s := range res.asked {
		if s.Type() == "books" {
			series = append(series, s.Str("series"))
		}
	}
	if strings.Join(series, "|") != "Ōoku|Ōku|"+BooksNone {
		t.Errorf("the books source was asked for the series %q", series)
	}
	// The second row's next page is that row's, not the first's.
	res.asked = nil
	if _, err := RenderSection(page, shelves[1].ID, 20, "", ctx); err != nil {
		t.Fatalf("the second row's next page: %v", err)
	}
	if len(res.asked) != 1 || res.asked[0].Str("series") != "Ōku" {
		t.Errorf("the second row's next page asked for %v", res.asked)
	}
}

func TestBookRepeatsNeedALibraryPage(t *testing.T) {
	row := Section{ID: "rows", Component: "Shelf", Repeat: &Repeat{Over: "bookAuthors"}, Source: Source{"type": "books", "library": "$library", "author": "$each"}}
	home := Page{ID: "p", Kind: "home", Title: "P", Sections: []Section{row}}
	if !hasError(ValidatePage(&home), "/sections/0/repeat/over") {
		t.Error("a repeat over a library's authors was allowed on Home")
	}
	lib := Page{ID: "p", Kind: "library", Title: "P", Sections: []Section{row}}
	if issues := ValidatePage(&lib); len(issues) > 0 {
		t.Errorf("on a library page: %v", issues)
	}
	// Directly inside a Tab of a Tabs at the top is a place for a repeat; deeper isn't.
	inTab := Page{ID: "p", Kind: "library", Title: "P", Sections: []Section{{ID: "t", Component: "Tabs", Children: []Section{{ID: "a", Component: "Tab", Props: map[string]any{"label": "A"}, Children: []Section{row}}}}}}
	if issues := ValidatePage(&inTab); len(issues) > 0 {
		t.Errorf("inside a Tab: %v", issues)
	}
	deep := Page{ID: "p", Kind: "library", Title: "P", Sections: []Section{{ID: "s", Component: "Stack", Children: []Section{{ID: "s2", Component: "Stack", Children: []Section{row}}}}}}
	if !hasError(ValidatePage(&deep), "/sections/0/children/0/children/0/repeat") {
		t.Error("a repeat three Stacks deep was allowed")
	}
}

func hasError(issues []Issue, path string) bool {
	for _, is := range issues {
		if is.Severity == "error" && is.Path == path {
			return true
		}
	}
	return false
}

func TestModuleSourcesOnTopShelf(t *testing.T) {
	checks := ModuleChecks{ColorToken: func(s string) bool { return s == "accent" }, Color: hexColour}
	ui := exampleModule(t)
	ui.Sources[0].TopShelf = true
	if issues := ValidateModuleUI(ui, "wittyboardgames", checks); len(issues) > 0 {
		t.Fatalf("a source with no required parameters may be offered: %v", issues)
	}
	set := NewModuleSet([]ModuleUI{*ui})
	got := set.TopShelfSources()
	if len(got) != 1 || got[0].Type != "wittyboardgames.shelf" {
		t.Errorf("TopShelfSources = %+v", got)
	}
	if NewModuleSet([]ModuleUI{*exampleModule(t)}).TopShelfSources() != nil {
		t.Error("a source the module didn't mark was offered")
	}
	var none *ModuleSet
	if none.TopShelfSources() != nil {
		t.Error("no modules offered something")
	}

	needs := exampleModule(t)
	needs.Sources[0].TopShelf = true
	needs.Sources[0].Params[0].Required = true
	if !hasError(ValidateModuleUI(needs, "wittyboardgames", checks), "/sources/0/params/0") {
		t.Error("a source with a required parameter was offered on Top Shelf")
	}
	paged := exampleModule(t)
	paged.Sources[0].TopShelf = true
	paged.Sources[0].PageKinds = []string{"detail"}
	if !hasError(ValidateModuleUI(paged, "wittyboardgames", checks), "/sources/0/topShelf") {
		t.Error("a source that needs a page was offered on Top Shelf")
	}
	// The file's own spelling.
	b := []byte(`{"format":"wittymodule-sdui","formatVersion":1,"module":"wittyx","catalog":2,"theme":{"scope":"wittyx"},
		"sources":[{"type":"wittyx.picks","label":"Picks","yields":["movie"],"cardStyle":"poster","topShelf":true}]}`)
	parsed, err := ParseModuleUI(b)
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.Sources[0].TopShelf {
		t.Error("topShelf wasn't read")
	}
}

// The new sources are in the catalogue with what an editor needs, and their parameters
// take what the server's resolver reads.
func TestCatalogueHasTheShelfStatisticsAndLikedSources(t *testing.T) {
	for typ, params := range map[string][]string{
		"books":      {"library", "series", "author", "format", "sort", "desc", "limit"},
		"statistics": {"stat", "who", "period", "library", "limit"},
		"liked":      {"library", "media", "limit"},
	} {
		spec, ok := SourceType(typ)
		if !ok {
			t.Errorf("%s isn't in the catalogue", typ)
			continue
		}
		for _, p := range params {
			if spec.param(p) == nil {
				t.Errorf("%s has no parameter %s", typ, p)
			}
		}
	}
	if !contains(RepeatKinds, "bookSeries") || !contains(RepeatKinds, "bookAuthors") {
		t.Error("a section can't be repeated over a library's series and authors")
	}
}
