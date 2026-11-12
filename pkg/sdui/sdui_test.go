// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite docs/sdui.schema.json and the catalogue section of docs/SDUI.md")

// The catalogue is consistent with itself: every name it refers to exists, every
// optional component's fallback is made of required components, and every example is a
// section the validator accepts.
func TestCatalogueIsConsistent(t *testing.T) {
	c := Catalog()
	seen := map[string]bool{}
	for _, comp := range c.Components {
		if seen[comp.Type] {
			t.Errorf("component %s is in the catalogue twice", comp.Type)
		}
		seen[comp.Type] = true
		if comp.Since < 1 || comp.Since > CatalogRevision {
			t.Errorf("%s: since %d", comp.Type, comp.Since)
		}
		if !contains(Icons, comp.Icon) {
			t.Errorf("%s: icon %q isn't in the icon set", comp.Type, comp.Icon)
		}
		for _, ch := range comp.Children {
			if _, ok := Component(ch); !ok {
				t.Errorf("%s allows a child %s that isn't a component", comp.Type, ch)
			}
		}
		for _, a := range comp.Accepts {
			if _, ok := SourceType(a); !ok {
				t.Errorf("%s accepts a source %s that isn't one", comp.Type, a)
			}
		}
		names := map[string]bool{}
		for _, p := range comp.Props {
			if names[p.Name] {
				t.Errorf("%s has the property %s twice", comp.Type, p.Name)
			}
			names[p.Name] = true
			if d, ok := p.Default.(string); ok && p.Type == PropEnum && !contains(values(p.Options), d) {
				t.Errorf("%s.%s: the default %q isn't one of its choices", comp.Type, p.Name, d)
			}
			if d, ok := p.Default.([]string); ok {
				for _, v := range d {
					if !contains(values(p.Options), v) {
						t.Errorf("%s.%s: the default %q isn't one of its choices", comp.Type, p.Name, v)
					}
				}
			}
		}
		if comp.Example != nil {
			var s Section
			if err := decodeStrict(comp.Example, &s); err != nil {
				t.Errorf("%s: its example isn't a section: %v", comp.Type, err)
				continue
			}
			kind := "custom"
			for _, k := range c.PageKinds {
				b, _ := json.Marshal(s)
				for _, v := range k.Variables {
					if strings.Contains(string(b), `"`+v+`"`) {
						kind = k.Kind
					}
				}
			}
			p := Page{ID: "example", Kind: kind, Title: "Example", Sections: []Section{s}}
			if s.Component == "FilterBar" || s.Component == "Dock" {
				p.Sections = append(p.Sections, Section{ID: "all", Component: "Grid", Source: Source{"type": "browse", "library": "$library", "filterable": true}})
				p.Kind = "library"
			}
			if issues := ValidatePage(&p); HasErrors(issues) {
				t.Errorf("%s: its example doesn't validate: %v", comp.Type, issues)
			}
		}
	}
	for _, s := range c.Sources {
		if _, ok := CardStyle(s.CardStyle); !ok {
			t.Errorf("source %s: its card style %q isn't one", s.Type, s.CardStyle)
		}
		if !contains(Icons, s.Icon) || s.Label == "" || s.Description == "" || len(s.Yields) == 0 {
			t.Errorf("source %s isn't fully described", s.Type)
		}
		var src Source
		if err := json.Unmarshal(s.Example, &src); err != nil || src.Type() != s.Type {
			t.Errorf("source %s: its example is %s", s.Type, s.Example)
			continue
		}
		kind := "custom"
		if len(s.PageKinds) > 0 {
			kind = s.PageKinds[0]
		} else if strings.Contains(string(s.Example), "$library") {
			kind = "library"
		}
		sec := Section{ID: "row", Component: "Shelf", Source: src}
		if strings.Contains(string(s.Example), "$each") {
			sec.Repeat = &Repeat{Over: "libraries"}
		}
		p := Page{ID: "example", Kind: kind, Title: "Example", Sections: []Section{sec}}
		if issues := ValidatePage(&p); HasErrors(issues) {
			t.Errorf("source %s: its example doesn't validate: %v", s.Type, issues)
		}
	}
	for _, st := range c.CardStyles {
		for _, ff := range FormFactors {
			if m, ok := c.Profiles[ff].Cards[st.Style]; !ok || (st.Shape != "chip" && m.Width["m"] <= 0) {
				t.Errorf("the %s profile has no size for %s cards", ff, st.Style)
			}
		}
	}
	for _, r := range c.Routes {
		if (r.Page == "") == (r.Native == "") {
			t.Errorf("route %s names a page and a native screen, or neither", r.Pattern)
		}
		if r.Native != "" && !contains(NativeScreens, r.Native) {
			t.Errorf("route %s: %s isn't a native screen", r.Pattern, r.Native)
		}
	}
	for _, n := range c.NativeScreens {
		if !contains(NativeScreens, n.Screen) {
			t.Errorf("native screen %s isn't in NativeScreens", n.Screen)
		}
	}
	// The built-in pages and every preset are valid, and Settings is never a page.
	for _, p := range StockPages() {
		if p.Kind == "settings" || p.ID == "settings" {
			t.Error("Settings is the app's own screen, not a page")
		}
	}
	if issues := ValidateNavigation(ptr(StockNavigation()), func(id string) bool { _, ok := StockPage(id); return ok }); HasErrors(issues) {
		t.Errorf("the built-in navigation: %v", issues)
	}
	for _, p := range Presets() {
		if p.Section != nil {
			kind := "home"
			if len(p.PageKinds) > 0 {
				kind = p.PageKinds[0]
			}
			pg := Page{ID: "x", Kind: kind, Title: "X", Sections: []Section{*p.Section}}
			if p.ID == "filter-grid" {
				continue // half of a pair; the built-in library pages hold both
			}
			if issues := ValidatePage(&pg); HasErrors(issues) {
				t.Errorf("section preset %s: %v", p.ID, issues)
			}
		}
	}
}

func ptr[T any](v T) *T { return &v }

func TestValidatePageSaysWhatIsWrongAndHowToFixIt(t *testing.T) {
	cases := []struct {
		name string
		page string
		path string
		want string // in the message or the fix
	}{
		{"a misspelt component", `{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Shelff"}]}`, "/sections/0/component", `Did you mean "Shelf"?`},
		{"a misspelt source", `{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Shelf","source":{"type":"recentlyAded"}}]}`, "/sections/0/source/type", `Did you mean "recentlyAdded"?`},
		{"a misspelt parameter", `{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Shelf","source":{"type":"genre","genere":"Comedy"}}]}`, "/sections/0/source/genere", `Did you mean "genre"?`},
		{"a missing source", `{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Shelf"}]}`, "/sections/0/source", "needs a data source"},
		{"a source where none belongs", `{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Text","props":{"text":"x"},"source":{"type":"nextUp"}}]}`, "/sections/0/source", "doesn't take a data source"},
		{"a wrong type", `{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Shelf","props":{"numbered":"yes"},"source":{"type":"nextUp"}}]}`, "/sections/0/props/numbered", "true or false"},
		{"out of range", `{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Hero","props":{"maxItems":50},"source":{"type":"featured"}}]}`, "/sections/0/props/maxItems", "between 1 and 8"},
		{"a duplicate ID", `{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Divider"},{"id":"a","component":"Divider"}]}`, "/sections/1/id", "already used at /sections/0"},
		{"a variable from another kind of page", `{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Shelf","source":{"type":"similar","item":"$item"}}]}`, "/sections/0/source/item", "$item means nothing on a home page"},
		{"a link off the server", `{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Button","props":{"label":"x","to":"//evil.example/x"}}]}`, "/sections/0/props/to", "is not a place in the app"},
		{"a script in a picture", `{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Image","props":{"src":"javascript:alert(1)"}}]}`, "/sections/0/props/src", "is not a picture on this server"},
		{"a script in Markdown", `{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Text","props":{"format":"markdown","text":"[x](javascript:alert(1))"}}]}`, "/sections/0/props/text", "doesn't lead anywhere"},
		{"a row inside a row", `{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Stack","props":{"direction":"row"},"children":[{"id":"b","component":"Shelf","source":{"type":"nextUp"}}]}]}`, "/sections/0/children/0/component", "goes across holds only"},
		{"children where none belong", `{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Shelf","source":{"type":"nextUp"},"children":[{"id":"b","component":"Divider"}]}]}`, "/sections/0/children", "can't hold other sections"},
		{"a tab outside tabs", `{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Tab","props":{"label":"x"}}]}`, "/sections/0/component", "belongs inside Tabs"},
		{"a filter bar with no grid", `{"id":"p","kind":"library","title":"P","sections":[{"id":"a","component":"FilterBar","props":{"target":"nope"}}]}`, "/sections/0/props/target", "is not a Grid on this page"},
		{"a filter bar on a grid that doesn't filter", `{"id":"p","kind":"library","title":"P","sections":[{"id":"a","component":"FilterBar","props":{"target":"g"}},{"id":"g","component":"Grid","source":{"type":"recentlyAdded"}}]}`, "/sections/0/props/target", "filterable"},
		{"a variant with no condition", `{"id":"p","kind":"home","title":"P","sections":[],"variants":[{"id":"v","when":{},"sections":[]}]}`, "/variants/0/when", "needs a condition"},
		{"a bad time", `{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Divider","when":{"timeFrom":"6pm"}}]}`, "/sections/0/when/timeFrom", "HH:MM"},
		{"a bad form factor", `{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Divider","when":{"formFactor":["watch"]}}]}`, "/sections/0/when/formFactor/0", "choices are tv, phone"},
		{"a repeat that isn't used", `{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Shelf","repeat":{"over":"libraries"},"source":{"type":"nextUp"}}]}`, "/sections/0/source", "never uses $each"},
		{"the wrong kind of source", `{"id":"p","kind":"detail","title":"P","sections":[{"id":"a","component":"TrackList","source":{"type":"episodes","show":"$show"}}]}`, "/sections/0/source/type", "shows tracks, not episodes"},
		{"a source for another kind of page", `{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Shelf","source":{"type":"search","query":"x"}}]}`, "/sections/0/source/type", "only works on a search page"},
		{"something after the app's own view", `{"id":"p","kind":"livetv","title":"P","sections":[{"id":"a","component":"NativeView","props":{"view":"guide"}},{"id":"b","component":"Divider"}]}`, "/sections/0", "nothing can follow it"},
		{"a map to a field that isn't one", `{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Shelf","source":{"type":"nextUp","map":{"subtitle":"director"}}}]}`, "/sections/0/source/map/subtitle", "is not an item field"},
		{"a page with no title", `{"id":"p","kind":"home","title":" ","sections":[]}`, "/title", "needs a title"},
		{"a page of no kind", `{"id":"p","kind":"homee","title":"P","sections":[]}`, "/kind", `Did you mean "home"?`},
		{"a match that kind can't have", `{"id":"p","kind":"home","title":"P","match":{"libraryKind":"music"},"sections":[]}`, "/match/libraryKind", "can't be matched by libraryKind"},
		{"a button that opens nothing", `{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Button","props":{"label":"x"}}]}`, "/sections/0/props", "doesn't open anything"},
		{"a button that opens two things", `{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Button","props":{"label":"x","to":"/a","native":"guide"}}]}`, "/sections/0/props", "opens one thing"},
		{"Settings as a page", `{"id":"p","kind":"settings","title":"P","sections":[]}`, "/kind", "is not a kind of page"},
	}
	for _, c := range cases {
		p, err := DecodePage([]byte(c.page))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		found := false
		issues := ValidatePage(&p)
		for _, is := range issues {
			if is.Path == c.path && (strings.Contains(is.Message, c.want) || strings.Contains(is.Fix, c.want)) {
				found = true
				if is.Fix == "" && is.Severity == "error" && !strings.Contains(c.want, "choices") && !strings.Contains(c.want, "between") {
					t.Errorf("%s: no fix is suggested for %q", c.name, is.Message)
				}
			}
		}
		if !found {
			t.Errorf("%s: want an issue at %s saying %q, got %v", c.name, c.path, c.want, issues)
		}
	}
	// Unknown fields are refused when a page is decoded, before it is validated.
	if _, err := DecodePage([]byte(`{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Divider","style":{"color":"red"}}]}`)); err == nil {
		t.Error("a section with a style was decoded")
	}
	if _, err := DecodePage(bytes.Repeat([]byte(" "), MaxPageBytes+1)); err == nil {
		t.Error("a page over the size limit was decoded")
	}
	// Limits.
	deep := Section{ID: "s0", Component: "Stack"}
	cur := &deep
	for i := 1; i < 10; i++ {
		cur.Children = []Section{{ID: "s" + string(rune('0'+i)), Component: "Stack"}}
		cur = &cur.Children[0]
	}
	if issues := ValidatePage(&Page{ID: "p", Kind: "home", Title: "P", Sections: []Section{deep}}); !HasErrors(issues) {
		t.Error("ten levels of nesting passed")
	}
	var many []Section
	for i := range MaxChildren + 1 {
		many = append(many, Section{ID: "d" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + string(rune('a'+i/26)), Component: "Divider"})
	}
	if issues := ValidatePage(&Page{ID: "p", Kind: "home", Title: "P", Sections: many}); !HasErrors(issues) {
		t.Error("too many sections passed")
	}
}

func TestConditions(t *testing.T) {
	yes, no := true, false
	at := func(hhmm string) time.Time {
		tm, _ := time.Parse("2006-01-02 15:04", "2026-11-11 "+hhmm) // a Wednesday
		return tm
	}
	ctx := &RenderContext{Platform: "tvos", ProfileKind: "kids", Features: []string{"live-tv"}, Modules: []string{"wittyplay"}, Cannot: []string{"downloads"},
		LibraryKinds: []string{"movies"}, Capabilities: []string{"hdr"}, Flags: map[string]bool{"scanning": true}, Viewport: Viewport{Width: 1920}, Locale: "en-GB",
		Profile: ProfileFor(FormTV, ""), Vars: map[string]string{"libraryKind": "movies"}, Now: at("19:30")}
	for name, c := range map[string]struct {
		c    Condition
		want bool
	}{
		"nothing":            {Condition{}, true},
		"form factor":        {Condition{FormFactor: []string{"phone", "tv"}}, true},
		"another device":     {Condition{FormFactor: []string{"phone"}}, false},
		"platform":           {Condition{Platform: []string{"tvos"}}, true},
		"kids":               {Condition{ProfileKind: []string{"kids", "guest"}}, true},
		"not admin":          {Condition{Admin: &no}, true},
		"admin":              {Condition{Admin: &yes}, false},
		"feature":            {Condition{Feature: "live-tv"}, true},
		"no feature":         {Condition{Feature: "karaoke"}, false},
		"module":             {Condition{Module: "wittyplay"}, true},
		"can":                {Condition{Can: "games"}, true},
		"cannot":             {Condition{Can: "downloads"}, false},
		"has library":        {Condition{HasLibrary: "movies"}, true},
		"has no library":     {Condition{HasLibrary: "music"}, false},
		"library kind":       {Condition{LibraryKind: "movies"}, true},
		"capability":         {Condition{Capability: "hdr"}, true},
		"flag":               {Condition{Flag: "scanning"}, true},
		"flag off":           {Condition{Flag: "setupIncomplete"}, false},
		"wide":               {Condition{MinWidth: 1200}, true},
		"narrow":             {Condition{MaxWidth: 600}, false},
		"evening":            {Condition{TimeFrom: "18:00", TimeTo: "23:00"}, true},
		"morning":            {Condition{TimeFrom: "06:00", TimeTo: "12:00"}, false},
		"until":              {Condition{TimeTo: "19:30"}, false},
		"from":               {Condition{TimeFrom: "19:30"}, true},
		"overnight":          {Condition{TimeFrom: "19:00", TimeTo: "02:00"}, true},
		"overnight, outside": {Condition{TimeFrom: "22:00", TimeTo: "06:00"}, false},
		"wednesday":          {Condition{Weekday: []string{"wed"}}, true},
		"weekend":            {Condition{Weekday: []string{"sat", "sun"}}, false},
		"language":           {Condition{Locale: "en"}, true},
		"another language":   {Condition{Locale: "e"}, false},
		"all":                {Condition{All: []Condition{{Feature: "live-tv"}, {Can: "games"}}}, true},
		"all, one fails":     {Condition{All: []Condition{{Feature: "live-tv"}, {Can: "downloads"}}}, false},
		"any":                {Condition{Any: []Condition{{Admin: &yes}, {ProfileKind: []string{"kids"}}}}, true},
		"any, none":          {Condition{Any: []Condition{{Admin: &yes}, {ProfileKind: []string{"adult"}}}}, false},
		"not":                {Condition{Not: &Condition{Admin: &yes}}, true},
		"never":              {Condition{Not: &Condition{}}, false},
		"fields are anded":   {Condition{Feature: "live-tv", Can: "downloads"}, false},
	} {
		if got := Evaluate(&c.c, ctx); got != c.want {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
	if !Evaluate(nil, ctx) {
		t.Error("no condition must hold")
	}
	// Width bounds pass when the client didn't say how wide it is.
	ctx.Viewport = Viewport{}
	if !Evaluate(&Condition{MaxWidth: 600}, ctx) {
		t.Error("a width bound with no viewport")
	}
}

func TestTruncateAndRuntime(t *testing.T) {
	for _, c := range []struct {
		in    string
		limit int
		want  string
	}{
		{"short", 10, "short"}, {"exactly ten", 11, "exactly ten"}, {"one two three four five", 12, "one two…"}, {"one two, three", 9, "one two…"},
		{"abcdefghijklmnopqrstuvwxyz", 10, "abcdefghi…"}, {"  padded  ", 20, "padded"}, {"ünïcödé text that is long", 10, "ünïcödé…"}, {"no limit", 0, "no limit"},
	} {
		if got := Truncate(c.in, c.limit); got != c.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", c.in, c.limit, got, c.want)
		}
		if c.limit > 0 && len([]rune(Truncate(c.in, c.limit))) > c.limit {
			t.Errorf("Truncate(%q, %d) is longer than the limit", c.in, c.limit)
		}
	}
	for sec, want := range map[float64]string{0: "", 45: "45 sec", 59.6: "1 min", 2520: "42 min", 3600: "1 hr", 7080: "1 hr 58 min", 5370: "1 hr 30 min"} {
		if got := FormatRuntime(sec); got != want {
			t.Errorf("FormatRuntime(%v) = %q, want %q", sec, got, want)
		}
	}
	l := ProfileFor(FormTV, "4k")
	if l.ImageWidth(250) != 640 || ProfileFor(FormTV, "").ImageWidth(250) != 320 || ProfileFor(FormPhone, "").ImageWidth(120) != 480 || l.ImageWidth(5000) != 3840 {
		t.Error("picture widths")
	}
	if c := ProfileFor(FormTV, "").GridColumns("poster", "m"); c != 6 {
		t.Errorf("tv poster columns: %d", c)
	}
	if c := ProfileFor(FormPhone, "").GridColumns("poster", "m"); c != 3 {
		t.Errorf("phone poster columns: %d", c)
	}
	if c := ProfileFor(FormDesktop, "").WithViewport(Viewport{Width: 1440}).GridColumns("poster", "m"); c != 7 {
		t.Errorf("desktop poster columns at 1440: %d", c)
	}
}

func TestSelectPage(t *testing.T) {
	pages := []Page{
		{ID: "mine", Kind: "library", Match: &Match{Library: 7}},
		{ID: "library", Kind: "library"}, {ID: "library-music", Kind: "library", Match: &Match{LibraryKind: "music"}},
		{ID: "detail", Kind: "detail"}, {ID: "detail-show", Kind: "detail", Match: &Match{MediaKind: "show"}},
		{ID: "detail-anime-show", Kind: "detail", Match: &Match{MediaKind: "show", LibraryKind: "anime"}},
	}
	for _, c := range []struct {
		kind           string
		lib            int64
		libKind, media string
		want           string
	}{
		{"library", 7, "music", "", "mine"}, {"library", 3, "music", "", "library-music"}, {"library", 3, "photos", "", "library"},
		{"detail", 3, "shows", "show", "detail-show"}, {"detail", 3, "anime", "show", "detail-anime-show"}, {"detail", 3, "movies", "movie", "detail"},
	} {
		if got, ok := SelectPage(pages, c.kind, c.lib, c.libKind, c.media); !ok || got.ID != c.want {
			t.Errorf("%s for library %d (%s) and %q: %s, want %s", c.kind, c.lib, c.libKind, c.media, got.ID, c.want)
		}
	}
	if _, ok := SelectPage(pages, "home", 0, "", ""); ok {
		t.Error("a page of a kind there is none of")
	}
}

func TestDiffAndApply(t *testing.T) {
	node := func(id string, title string, kids ...Node) Node {
		return Node{Type: "Shelf", ID: id, Props: map[string]any{"title": title}, Children: kids}
	}
	card := func(id string) Node {
		return Node{Type: "Card", ID: id, Props: map[string]any{"title": id}, Action: &Action{Type: ActNavigate, To: "/item/" + id}}
	}
	doc := func(title string, kids ...Node) Document {
		d := Document{Version: 1, Catalog: CatalogRevision, Page: PageInfo{ID: "home", Kind: "home", Title: title}, Layout: ProfileFor(FormTV, ""), Root: Node{Type: "Page", ID: "page", Children: kids}}
		stampFocus(&d.Root)
		d.Meta.ETag = ETag(d)
		return d
	}
	a := doc("Home", node("one", "One", card("a"), card("b"), card("c")), node("two", "Two", card("d")))
	b := doc("Home!", node("two", "Two", card("d"), card("e")), node("three", "Three"), node("one", "First", card("c"), card("a")))
	patches := Diff(a, b)
	if len(patches) == 0 {
		t.Fatal("no patches")
	}
	// As a client gets them: through JSON.
	raw, _ := json.Marshal(patches)
	var wire []Patch
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	got, err := Apply(a, wire)
	if err != nil {
		t.Fatal(err)
	}
	if ETag(got) != ETag(b) {
		x, _ := json.Marshal(got)
		y, _ := json.Marshal(b)
		t.Errorf("applying the patches doesn't give the new document:\n%s\n%s", x, y)
	}
	if len(Diff(b, b)) != 0 {
		t.Error("a document differs from itself")
	}
	if _, err := Apply(a, []Patch{{Op: PatchRemove, Path: "#nope"}}); err == nil {
		t.Error("a patch for a node that isn't there applied")
	}
	if a.Root.Children[0].Props["title"] != "One" {
		t.Error("Apply changed the document it was given")
	}
	// A different root is one patch.
	c := b
	c.Root = Node{Type: "Page", ID: "other"}
	if p := Diff(a, c); p[len(p)-1].Path != "/root" {
		t.Errorf("a new root: %+v", p)
	}
}

func TestJSONSchemaAndDocsAreCurrent(t *testing.T) {
	schema := JSONSchema()
	var v map[string]any
	if err := json.Unmarshal(schema, &v); err != nil {
		t.Fatal(err)
	}
	check := func(path string, want []byte) {
		t.Helper()
		if *update {
			if err := os.WriteFile(path, want, 0o644); err != nil {
				t.Fatal(err)
			}
			return
		}
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), want) {
			t.Errorf("%s is out of date: run go test ./pkg/sdui -run TestJSONSchemaAndDocsAreCurrent -update", path)
		}
	}
	check(filepath.Join("..", "..", "docs", "sdui.schema.json"), schema)
	check(filepath.Join("..", "..", "docs", "sdui-module.schema.json"), ModuleJSONSchema())
	// The reference section of docs/SDUI.md is written from the catalogue.
	docPath := filepath.Join("..", "..", "docs", "SDUI.md")
	doc, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatal(err)
	}
	const begin, end = "<!-- catalogue:begin (written by go test ./pkg/sdui -update; edit pkg/sdui/catalog_data.go) -->\n", "<!-- catalogue:end -->"
	text := strings.ReplaceAll(string(doc), "\r\n", "\n")
	i, j := strings.Index(text, begin), strings.Index(text, end)
	if i < 0 || j < i {
		t.Fatalf("docs/SDUI.md has no catalogue section between %q and %q", strings.TrimSpace(begin), end)
	}
	check(docPath, []byte(text[:i]+begin+CatalogueMarkdown()+text[j:]))
}

func FuzzDecodeAndValidatePage(f *testing.F) {
	for _, p := range StockPages() {
		b, _ := json.Marshal(p)
		f.Add(b)
	}
	f.Add([]byte(`{"id":"p","kind":"home","title":"P","sections":[{"id":"a","component":"Shelf","source":{"type":"genre","genre":5,"map":7},"when":{"not":{"any":[{}]}},"repeat":{"over":"x"}}]}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		p, err := DecodePage(b)
		if err != nil {
			return
		}
		issues := ValidatePage(&p)
		for _, is := range issues {
			if is.Message == "" || (is.Severity != "error" && is.Severity != "warning") {
				t.Fatalf("an issue with no message or severity: %+v", is)
			}
		}
		// Whatever decodes renders without panicking, valid or not, with or without data.
		ctx := &RenderContext{Profile: ProfileFor(FormTV, ""), Vars: map[string]string{}, Flags: map[string]bool{}, CanPersonalise: true,
			Personal: &Personal{Order: []string{"a"}, Hidden: []string{"b"}}}
		doc, err := Render(p, ctx)
		if err == nil && !HasErrors(issues) {
			if _, err := Marshal(doc); err != nil {
				t.Fatal(err)
			}
		}
	})
}
