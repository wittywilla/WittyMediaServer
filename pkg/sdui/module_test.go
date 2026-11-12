// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func exampleModule(t *testing.T) *ModuleUI {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "module-boardgames.json"))
	if err != nil {
		t.Fatal(err)
	}
	ui, err := ParseModuleUI(b)
	if err != nil {
		t.Fatal(err)
	}
	return ui
}

var hexColour = func(s string) bool { return len(s) == 7 && s[0] == '#' }

func TestTheWorkedExampleModuleIsValid(t *testing.T) {
	ui := exampleModule(t)
	issues := ValidateModuleUI(ui, "wittyboardgames", ModuleChecks{ColorToken: func(s string) bool { return s == "accent" }, Color: hexColour})
	for _, is := range issues {
		t.Errorf("%s %s: %s (%s)", is.Severity, is.Path, is.Message, is.Fix)
	}
}

func TestModuleUIIsParsedStrictly(t *testing.T) {
	for name, in := range map[string]string{
		"unknown field": `{"format":"wittymodule-sdui","formatVersion":1,"module":"x1","catalog":2,"theme":{"scope":"x1"},"script":"alert(1)"}`,
		"trailing":      `{"format":"wittymodule-sdui","formatVersion":1,"module":"x1","catalog":2,"theme":{"scope":"x1"}} {}`,
		"not json":      `{`,
	} {
		if _, err := ParseModuleUI([]byte(in)); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
	if _, err := ParseModuleUI(make([]byte, MaxModuleBytes+1)); err == nil {
		t.Error("an oversized file parsed")
	}
	ui, err := ParseModuleUI([]byte(`{"format":"wittymodule-sdui","formatVersion":1,"module":"wittyx","catalog":2,"theme":{"scope":"wittyx"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if issues := ValidateModuleUI(ui, "wittyx", ModuleChecks{}); len(issues) != 0 {
		t.Errorf("the smallest file has issues: %v", issues)
	}
}

func TestModuleUIRefusals(t *testing.T) {
	cases := []struct {
		name, path string
		change     func(ui *ModuleUI)
	}{
		{"another module's file", "/module", func(ui *ModuleUI) { ui.Module = "wittyother" }},
		{"a newer catalogue", "/catalog", func(ui *ModuleUI) { ui.Catalog = CatalogRevision + 1 }},
		{"a scope not its own", "/theme/scope", func(ui *ModuleUI) { ui.Theme.Scope = "wittytv" }},
		{"a colour that isn't one", "/theme/tokens/accent", func(ui *ModuleUI) { ui.Theme.Tokens["accent"] = "red; background:url(x)" }},
		{"a source not named after it", "/sources/0/type", func(ui *ModuleUI) { ui.Sources[0].Type = "recentlyAdded" }},
		{"a source without what it yields", "/sources/0/yields", func(ui *ModuleUI) { ui.Sources[0].Yields = nil }},
		{"a component not named after it", "/components/0/type", func(ui *ModuleUI) { ui.Components[0].Type = "wittyother.Thing" }},
		{"a template placeholder with no property", "/components/0/section", func(ui *ModuleUI) { ui.Components[0].Section.Props["subtitle"] = "{{nothing}}" }},
		{"a page not named after it", "/pages/0/id", func(ui *ModuleUI) { ui.Pages[0].ID = "games" }},
		{"a home page", "/pages/0/kind", func(ui *ModuleUI) { ui.Pages[0].Kind = "home" }},
		{"a library page for every library", "/pages/0/match", func(ui *ModuleUI) { ui.Pages[0].Kind = "library" }},
		{"a page using another module's source", "/pages/0/sections/1/source/type", func(ui *ModuleUI) { ui.Pages[0].Sections[1].Source["type"] = "wittyother.things" }},
		{"an insert into a page that isn't built in", "/inserts/0/page", func(ui *ModuleUI) { ui.Inserts[0].Page = "nowhere" }},
		{"an insert after a section the page hasn't", "/inserts/0", func(ui *ModuleUI) { ui.Inserts[0].After = "nothing" }},
		{"an entry for Settings", "/navigation/0/screen", func(ui *ModuleUI) {
			ui.Navigation[0] = NavItem{ID: "wittyboardgames", Type: "native", Screen: "settings"}
		}},
		{"an entry for a page that isn't there", "/items/0/page", func(ui *ModuleUI) { ui.Navigation[0].Page = "wittyboardgames-nothing" }},
		{"a setting kind that isn't one", "/settings/0/groups/0/rows/0/kind", func(ui *ModuleUI) { ui.Settings[0].Groups[0].Rows[0].Kind = "colour" }},
		{"a default out of range", "/settings/0/groups/0/rows/2/default", func(ui *ModuleUI) { ui.Settings[0].Groups[0].Rows[2].Default = 40 }},
		{"a secret for a profile", "/settings/0/groups/0/rows/1/scope", func(ui *ModuleUI) { ui.Settings[0].Groups[0].Rows[1].Scope = "profile" }},
		{"two settings with one key", "/settings/0/groups/1/rows/0/key", func(ui *ModuleUI) { ui.Settings[0].Groups[1].Rows[0].Key = "household" }},
	}
	for _, c := range cases {
		ui := exampleModule(t)
		c.change(ui)
		issues := ValidateModuleUI(ui, "wittyboardgames", ModuleChecks{ColorToken: func(s string) bool { return s == "accent" }, Color: hexColour})
		found := false
		for _, is := range issues {
			if is.Severity == "error" && strings.HasPrefix(is.Path, c.path) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: no error at %s; got %v", c.name, c.path, issues)
		}
	}
}

func TestModuleComponentsExpand(t *testing.T) {
	set := NewModuleSet([]ModuleUI{*exampleModule(t)})
	page, ok := set.Page("wittyboardgames")
	if !ok {
		t.Fatal("no page")
	}
	out := set.ExpandPage(page)
	night := out.Sections[0]
	if night.ID != "night" || night.Component != "Shelf" || night.Scope != "wittyboardgames" {
		t.Fatalf("expanded to %+v", night)
	}
	if got := night.Props["title"]; got != "Game night for 4" {
		t.Errorf("title %v", got)
	}
	if got := night.Source["players"]; got != float64(4) {
		t.Errorf("players %#v: a value standing alone keeps its type", got)
	}
	// The set's own copy is untouched.
	again, _ := set.Page("wittyboardgames")
	if again.Sections[0].Component != "wittyboardgames.GameNight" {
		t.Error("expanding changed the module's page")
	}
}

func TestModuleInsertsGoIntoBuiltInPages(t *testing.T) {
	set := NewModuleSet([]ModuleUI{*exampleModule(t)})
	home, _ := StockPage("home")
	got := set.WithInserts(home)
	for i, s := range got.Sections {
		if s.ID == "wittyboardgames-tonight" {
			if got.Sections[i-1].ID != "jump" || s.Scope != "wittyboardgames" {
				t.Errorf("inserted after %q with scope %q", got.Sections[i-1].ID, s.Scope)
			}
			if issues := ValidatePageWith(&got, set); HasErrors(issues) {
				t.Errorf("home with the insert doesn't validate: %v", issues)
			}
			return
		}
	}
	t.Fatal("not inserted")
}

func TestPagesUsingAModuleNotInUseAreWarnedAndLeftOut(t *testing.T) {
	p := Page{ID: "mine", Kind: "custom", Title: "Mine", Sections: []Section{
		{ID: "a", Component: "wittyboardgames.GameNight"},
		{ID: "b", Component: "Shelf", Source: Source{"type": "wittyboardgames.shelf"}},
		{ID: "c", Component: "Shelf", Source: Source{"type": "recentlyAdded"}},
	}}
	issues := ValidatePage(&p)
	if HasErrors(issues) {
		t.Fatalf("a missing module is an error: %v", issues)
	}
	if len(issues) < 2 {
		t.Fatalf("no warnings: %v", issues)
	}
	doc, err := Render(p, &RenderContext{Resolver: oneItem{}, Vars: map[string]string{}, Flags: map[string]bool{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Root.Children) != 1 || doc.Root.Children[0].ID != "c" {
		t.Errorf("rendered %+v", doc.Root.Children)
	}
	// With the module in use, all three are there, and the module's source
	// is asked by its own name.
	set := NewModuleSet([]ModuleUI{*exampleModule(t)})
	if issues := ValidatePageWith(&p, set); len(issues) != 0 {
		t.Errorf("issues with the module in use: %v", issues)
	}
	rv := &asked{}
	doc, err = Render(p, &RenderContext{Resolver: rv, UI: set, Vars: map[string]string{}, Flags: map[string]bool{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Root.Children) != 3 {
		t.Fatalf("rendered %d sections", len(doc.Root.Children))
	}
	if doc.Root.Children[0].Props["cardStyle"] != "square" {
		t.Errorf("a module source's card style: %v", doc.Root.Children[0].Props["cardStyle"])
	}
	if !strings.Contains(strings.Join(rv.types, ","), "wittyboardgames.shelf") {
		t.Errorf("asked %v", rv.types)
	}
}

func TestCatalogWithModules(t *testing.T) {
	set := NewModuleSet([]ModuleUI{*exampleModule(t)})
	c := CatalogWith(set)
	if c == Catalog() {
		t.Fatal("the same catalogue")
	}
	var comp *ComponentSpec
	for i := range c.Components {
		if c.Components[i].Type == "wittyboardgames.GameNight" {
			comp = &c.Components[i]
		}
		if c.Components[i].Type == "NativeView" {
			found := false
			for _, p := range c.Components[i].Props {
				for _, o := range p.Options {
					found = found || o.Value == "wittyboardgames.scoreboard"
				}
			}
			if !found {
				t.Error("NativeView doesn't offer the module's view")
			}
		}
	}
	if comp == nil || comp.Module != "wittyboardgames" || comp.Expands != "Shelf" || comp.Category != "module" {
		t.Fatalf("component %+v", comp)
	}
	found := false
	for _, s := range c.Sources {
		found = found || (s.Type == "wittyboardgames.shelf" && s.Module == "wittyboardgames")
	}
	if !found {
		t.Error("no module source")
	}
	if len(Catalog().Components) == len(c.Components) {
		t.Error("the shared catalogue was changed")
	}
	if b, err := json.Marshal(c); err != nil || len(b) == 0 {
		t.Fatal(err)
	}
}

type oneItem struct{}

func (oneItem) Items(req SourceRequest) (SourceResult, error) {
	return SourceResult{Items: []Item{{ID: "1", Kind: "movie", Title: "One"}}, Total: 1}, nil
}
func (oneItem) Each(string, []string, int) ([]Each, error) { return nil, nil }
func (oneItem) Subject() (*Item, error)                    { return nil, ErrNotFound }
func (oneItem) Seasons(string) ([]Item, error)             { return nil, nil }
func (oneItem) Facets(string, []string) ([]Facet, error)   { return nil, nil }

type asked struct {
	oneItem
	types []string
}

func (a *asked) Items(req SourceRequest) (SourceResult, error) {
	a.types = append(a.types, req.Source.Type())
	return SourceResult{Items: []Item{{ID: "g1", Kind: "game", Title: "Go"}}, Total: 1}, nil
}
