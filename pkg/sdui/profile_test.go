// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func manyItems(n int, prefix string) []any {
	out := make([]any, n)
	for i := range out {
		id := fmt.Sprintf("%s%d", prefix, i)
		out[i] = map[string]any{"id": id, "title": "Title " + id, "imageUrl": "/api/items/" + id + "/poster", "route": "/item/" + id, "progress": 0.5}
	}
	return out
}

func homeData() map[string]any {
	var sections []any
	for i := 0; i < 25; i++ {
		sections = append(sections, map[string]any{"id": fmt.Sprintf("lib%d", i), "title": fmt.Sprintf("Recently added in Library %d", i), "items": manyItems(3, fmt.Sprintf("l%d-", i))})
	}
	return map[string]any{
		"user": map[string]any{"name": "willow"},
		"home": map[string]any{"featured": manyItems(9, "f"), "continueWatching": manyItems(50, "c"), "sections": sections},
	}
}

func walkNodes(n *Node, f func(*Node)) {
	f(n)
	for i := range n.Children {
		walkNodes(&n.Children[i], f)
	}
}

func renderHome(t *testing.T, form, class string) Document {
	t.Helper()
	out, err := Render(HomeTemplate(HomeData{}), RenderContext{Platform: form, Role: "user", Features: []string{"history"},
		Profile: ProfileFor(form, class), Data: homeData(), Now: time.Unix(0, 0)})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// An Apple TV HD gets a screen it can hold and move around: every node a
// person can act on is focusable, numbered in reading order with no gaps,
// focus starts on content, rows and their cards are cut to the profile, and
// pictures are asked for at the size they are shown.
func TestRender_TVHD(t *testing.T) {
	out := renderHome(t, FormTV, "hd")
	l := out.Layout
	if l == nil || l.FormFactor != FormTV || l.Class != "hd" || l.SafeArea != (Insets{60, 80, 60, 80}) || !l.FocusEngine || l.Canvas.Width != 1920 || l.Scale != 1 {
		t.Fatalf("layout: %+v", l)
	}
	if l.Text["body"] < 29 {
		t.Errorf("body text %d pt is too small to read from a sofa", l.Text["body"])
	}
	if !l.Truncated {
		t.Error("25 library rows were sent to a device that takes 10, and the layout doesn't say rows were left out")
	}
	rows, order, defaults := 0, 0, 0
	walkNodes(&out.Root, func(n *Node) {
		if rowScroller(n.Type) || pageScroller(n.Type) {
			rows++
			if len(n.Children) > l.MaxItems {
				t.Errorf("%s has %d cards; an Apple TV HD is sent at most %d", n.ID, len(n.Children), l.MaxItems)
			}
		}
		if Interactive(n) {
			order++
			if n.Focus == nil || !n.Focus.Focusable || n.Focus.Order != order {
				t.Fatalf("%s: focus %+v, want focusable with order %d", n.ID, n.Focus, order)
			}
			if n.Focus.Default {
				defaults++
				if n.Focus.Group == "home-toolbar" {
					t.Errorf("focus starts in the toolbar (%s), not on content", n.ID)
				}
			}
		} else if n.Focus != nil {
			t.Errorf("%s can't be acted on but has focus %+v", n.ID, n.Focus)
		}
		if n.Type == Card && n.Props["imageUrl"] != nil {
			img, _ := n.Props["image"].(map[string]any)
			if img == nil || !strings.Contains(img["url"].(string), "w=") {
				t.Fatalf("%s: no sized picture: %v", n.ID, n.Props)
			}
		}
	})
	if rows > l.MaxSections {
		t.Errorf("%d rows; at most %d", rows, l.MaxSections)
	}
	if defaults != 1 {
		t.Errorf("%d default focus nodes, want exactly 1", defaults)
	}
	cont := findNode(&out.Root, "home-continue")
	if cont == nil || len(cont.Children) != 16 || cont.Props["more"] != true || cont.Props["total"] != 50 {
		t.Fatalf("continue watching: %d cards, props %v", len(cont.Children), cont.Props)
	}
	card := cont.Children[0]
	if card.ID != "home-continue:c0" || card.Focus.Group != "home-continue" || card.Action != "openItem" || card.Props["route"] != "/item/c0" {
		t.Errorf("card: %+v props %v", card, card.Props)
	}
	// A wide card on a 1x screen is 548 points, so 548 pixels.
	if img := card.Props["image"].(map[string]any); img["width"] != 548 || img["aspect"] != "16/9" || img["url"] != "/api/items/c0/poster?w=548" {
		t.Errorf("image: %v", img)
	}
	if hero := findNode(&out.Root, "home-hero"); hero == nil || len(hero.Children) != maxHeroItems {
		t.Errorf("hero: %+v", hero)
	}
	if err := Validate(&out); err != nil {
		t.Errorf("a rendered screen should still validate: %v", err)
	}
}

// The same authored screen, for a 4K Apple TV, a phone and a browser: more
// rows where there's memory for them, pictures at each screen's scale.
func TestRender_PerFormFactor(t *testing.T) {
	want := map[string]struct {
		class        string
		width, items int
	}{FormTV: {"4k", 1096, 30}, FormPhone: {"", 720, 20}, FormTablet: {"", 640, 30}, FormDesktop: {"", 720, 40}, FormWeb: {"", 720, 40}}
	for form, w := range want {
		out := renderHome(t, form, w.class)
		cont := findNode(&out.Root, "home-continue")
		if len(cont.Children) != w.items {
			t.Errorf("%s: %d cards, want %d", form, len(cont.Children), w.items)
		}
		if img := cont.Children[0].Props["image"].(map[string]any); img["width"] != w.width {
			t.Errorf("%s: picture %v px wide, want %d", form, img["width"], w.width)
		}
		if out.Layout.FormFactor != form {
			t.Errorf("%s: layout says %s", form, out.Layout.FormFactor)
		}
	}
	// A condition on the form factor picks nodes for one of them.
	doc, _ := NewDocument("x").Root(ScreenNode("x").Child(
		TextOf("tv-only", "tv").If(Condition{FormFactor: FormTV}),
		TextOf("not-tv", "other").If(Not(Condition{FormFactor: FormTV})))).Build()
	tv, _ := Render(doc, RenderContext{Profile: ProfileFor(FormTV, "")})
	phone, _ := Render(doc, RenderContext{Profile: ProfileFor(FormPhone, "")})
	if findNode(&tv.Root, "tv-only") == nil || findNode(&tv.Root, "not-tv") != nil || findNode(&phone.Root, "tv-only") != nil || findNode(&phone.Root, "not-tv") == nil {
		t.Error("formFactor conditions didn't choose")
	}
}

// One golden per form factor: what a client developer can build against.
func TestRender_FormFactorGoldens(t *testing.T) {
	for _, c := range []struct{ form, class string }{{FormTV, "hd"}, {FormTV, "4k"}, {FormPhone, ""}, {FormWeb, ""}} {
		data := map[string]any{"user": map[string]any{"name": "willow"}, "home": map[string]any{
			"continueWatching": manyItems(2, "c"),
			"sections":         []any{map[string]any{"id": "lib1", "title": "Recently added in Films", "seeAll": "/library/1", "items": manyItems(2, "m")}},
		}}
		out, err := Render(HomeTemplate(HomeData{}), RenderContext{Platform: c.form, Role: "user", Features: []string{"history"},
			Profile: ProfileFor(c.form, c.class), Data: data, Now: time.Unix(0, 0)})
		if err != nil {
			t.Fatal(err)
		}
		got, _ := Canonical(out)
		var pretty bytes.Buffer
		_ = json.Indent(&pretty, got, "", "  ")
		name := "home." + c.form + c.class + ".golden.json"
		path := filepath.Join("testdata", name)
		if *update {
			if err := os.WriteFile(path, append(pretty.Bytes(), '\n'), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v (run with -update)", name, err)
		}
		if !bytes.Equal(bytes.TrimSpace(want), bytes.TrimSpace(pretty.Bytes())) {
			t.Errorf("%s differs from what Render makes now; run go test ./pkg/sdui -update if that's meant", name)
		}
	}
}

func TestRender_FocusDefaultsAndGuides(t *testing.T) {
	doc, err := NewDocument("x").Action("go", NavigateTo("/x")).Root(ScreenNode("x").Child(
		ToolbarOf("bar").Child(ButtonOf("b1", "One", "go")),
		ButtonOf("b2", "Two", "go").Focus(Focus{Down: "b3", Up: "gone"}),
		ButtonOf("b3", "Three", "go").Focus(Focus{Default: true}),
		ButtonOf("gone", "Hidden", "go").If(IsRole("admin")),
		ButtonOf("off", "Disabled", "go").Prop("disabled", true),
	)).Build()
	if err != nil {
		t.Fatal(err)
	}
	a, _ := Render(doc, RenderContext{})
	b, _ := Render(doc, RenderContext{})
	if ETag(a) != ETag(b) {
		t.Error("focus order isn't the same from one render to the next")
	}
	b2, b3 := findNode(&a.Root, "b2"), findNode(&a.Root, "b3")
	if !b3.Focus.Default || b2.Focus.Default || b2.Focus.Down != "b3" {
		t.Errorf("author's default and guide weren't kept: %+v %+v", b2.Focus, b3.Focus)
	}
	if b2.Focus.Up != "" {
		t.Error("a guide to a node that isn't on the screen was kept: focus would go nowhere")
	}
	if off := findNode(&a.Root, "off"); off.Focus != nil {
		t.Error("a disabled button takes focus")
	}
	// With the default's node hidden, focus still starts somewhere: the first content node.
	doc2, _ := NewDocument("x").Action("go", NavigateTo("/x")).Root(ScreenNode("x").Child(
		ToolbarOf("bar").Child(ButtonOf("b1", "One", "go")),
		ButtonOf("adm", "Admin", "go").Focus(Focus{Default: true}).If(IsRole("admin")),
		ButtonOf("b2", "Two", "go"))).Build()
	c, _ := Render(doc2, RenderContext{Role: "user"})
	if !findNode(&c.Root, "b2").Focus.Default {
		t.Error("no default focus once the author's choice was hidden")
	}
}

func TestRender_NoExternalLinksOnTV(t *testing.T) {
	doc, err := NewDocument("x").Action("help", Action{Type: External, Target: "https://example.org/help"}).Action("go", NavigateTo("/x")).
		Root(ScreenNode("x").Child(ButtonOf("help", "Help", "help"), ButtonOf("go", "Go", "go"))).Build()
	if err != nil {
		t.Fatal(err)
	}
	tv, _ := Render(doc, RenderContext{Profile: ProfileFor(FormTV, "hd")})
	if findNode(&tv.Root, "help") != nil || tv.Actions["help"].Type != "" || findNode(&tv.Root, "go") == nil {
		t.Error("a TV was sent something that opens a web page")
	}
	web, _ := Render(doc, RenderContext{Profile: ProfileFor(FormWeb, "")})
	if findNode(&web.Root, "help") == nil {
		t.Error("the browser lost its link")
	}
}

// A client built against an older catalogue is sent the type it knows.
func TestRender_DegradesNewTypes(t *testing.T) {
	catalog = append(catalog, ComponentSpec{Type: "TopTen", Since: 2, Fallback: Shelf, AllowsChildren: true, RequiredProps: []string{"title"}},
		ComponentSpec{Type: "Sparkle", Since: 3, AllowsChildren: true})
	defer func() { catalog = catalog[:len(catalog)-2] }()
	doc := Document{Version: 1, Screen: "x", Root: Node{Type: Screen, ID: "x", Children: []Node{
		{Type: "TopTen", ID: "t", Props: map[string]any{"title": "Top ten"}, Children: []Node{{Type: Card, ID: "c"}}},
		{Type: "Sparkle", ID: "s", Children: []Node{{Type: Text, ID: "tx", Props: map[string]any{"text": "hi"}}}},
	}}}
	if err := Validate(&doc); err != nil {
		t.Fatal(err)
	}
	old, _ := Render(doc, RenderContext{Catalog: 1})
	if old.Root.Children[0].Type != Shelf || old.Root.Children[1].Type != Stack || len(old.Root.Children[1].Children) != 1 {
		t.Errorf("an old client was sent %s and %s", old.Root.Children[0].Type, old.Root.Children[1].Type)
	}
	mid, _ := Render(doc, RenderContext{Catalog: 2})
	if mid.Root.Children[0].Type != "TopTen" || mid.Root.Children[1].Type != Stack {
		t.Errorf("a revision-2 client was sent %s and %s", mid.Root.Children[0].Type, mid.Root.Children[1].Type)
	}
	cur, _ := Render(doc, RenderContext{})
	if cur.Root.Children[1].Type != "Sparkle" {
		t.Error("a current client lost a current type")
	}
}

// What an administrator's edit must not be able to do to every client at once.
func TestValidate_RefusesWhatBreaksClients(t *testing.T) {
	base := func(children ...*NodeBuilder) Document {
		return NewDocument("x").Action("go", NavigateTo("/x")).Root(ScreenNode("x").Child(children...)).Document()
	}
	many := StackOf("many")
	for i := 0; i <= MaxChildren; i++ {
		many.Child(TextOf(fmt.Sprintf("t%d", i), "x"))
	}
	withAction := func(a Action) Document {
		d := base(ButtonOf("b", "B", "bad"))
		d.Actions["bad"] = a
		return d
	}
	cases := []struct {
		name string
		doc  Document
		want string
	}{
		{"a button inside a card", base(N(Card, "c").Child(TextOf("t", "x").OnPress("go"))), "focus can't reach both"},
		{"a card inside something pressable", base(StackOf("s").OnPress("go").Child(N(Card, "c"))), "focus can't reach both"},
		{"a shelf in a shelf", base(ShelfOf("a", "A").Child(ShelfOf("b", "B"))), "scrolling regions don't nest"},
		{"a shelf in a grid", base(GridOf("g", 4).Child(ShelfOf("b", "B"))), "scrolling regions don't nest"},
		{"a grid in a list via a stack", base(ShelfOf("l", "L").Child(StackOf("s").Child(GridOf("g", 2)))), "scrolling regions don't nest"},
		{"two defaults", base(ButtonOf("a", "A", "go").Focus(Focus{Default: true}), ButtonOf("b", "B", "go").Focus(Focus{Default: true})), "one default focus"},
		{"default on text", base(TextOf("t", "x").Focus(Focus{Default: true})), "default focus must be on a node a person can act on"},
		{"a guide to nothing", base(ButtonOf("a", "A", "go").Focus(Focus{Down: "nowhere"})), "names no node a person can act on"},
		{"a guide to text", base(ButtonOf("a", "A", "go").Focus(Focus{Down: "t"}), TextOf("t", "x")), "names no node a person can act on"},
		{"too many children", base(many), "at most 200"},
		{"navigate to another host", withAction(Action{Type: Navigate, Target: "https://evil.example/x"}), "path on this server"},
		{"navigate to //host", withAction(Action{Type: Navigate, Target: "//evil.example/x"}), "path on this server"},
		{"fetch from another host", withAction(Action{Type: Fetch, Target: "https://evil.example/collect"}), "path on this server"},
		{"submit to a placeholder", withAction(Action{Type: Submit, Target: "$route"}), "path on this server"},
		{"a javascript link", withAction(Action{Type: External, Target: "javascript:alert(1)"}), "http:// or https://"},
		{"a javascript picture", base(ImageOf("i", "javascript:alert(1)")), "path on this server or an http(s) address"},
		{"a data: stream", base(VideoOf("v", "data:text/html,x")), "path on this server or an http(s) address"},
		{"a huge prop", base(TextOf("t", strings.Repeat("x", MaxPropBytes+1))), "at most"},
		{"an unknown form factor", base(TextOf("t", "x").If(Condition{FormFactor: "fridge"})), "must be tv, phone, tablet, desktop or web"},
	}
	for _, c := range cases {
		err := Validate(&c.doc)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want an error with %q", c.name, err, c.want)
		}
	}
	// What is allowed: a tab's content, a modal's, and rows straight in the screen.
	ok := base(TabsOf("tabs").Child(TabOf("t1", "One").Child(ListOf("l").Child(ListItemOf("li", "Row").OnPress("go")))),
		ShelfOf("s", "S").Child(N(Card, "c").OnPress("go")), ModalOf("m", "M").Child(GridOf("g", 3)),
		ButtonOf("nav", "Nav", "go").Focus(Focus{Default: true, Down: "c"}))
	if err := Validate(&ok); err != nil {
		t.Errorf("a sound screen was refused: %v", err)
	}
}

func TestExpandSectionsAndItems(t *testing.T) {
	doc, _ := NewDocument("x").Action("openItem", NavigateTo("$route")).Action("tune", NavigateTo("$route")).Root(ScreenNode("x").Child(
		StackOf("rows").Bind("sections", "$.sections"),
		ListOf("list").Bind("items", "$.list").Prop("itemAction", "tune"),
		ShelfOf("empty", "Empty").Bind("items", "$.none"),
	)).Build()
	out, err := Render(doc, RenderContext{Data: map[string]any{
		"sections": []any{
			map[string]any{"id": "a", "title": "A", "cardShape": "square", "items": []any{map[string]any{"id": "1", "title": "One"}, map[string]any{"id": "1", "title": "Same id"}, "not an item"}},
			map[string]any{"id": "b", "title": "B", "items": []any{}},
		},
		"list": []map[string]any{{"id": "ch/7 !", "title": "Seven", "module": "wittytv"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	rows := findNode(&out.Root, "rows")
	if len(rows.Children) != 1 || rows.Children[0].ID != "rows:a" || rows.Children[0].Type != Shelf || rows.Children[0].Props["cardShape"] != "square" {
		t.Fatalf("rows: %+v", rows.Children)
	}
	cards := rows.Children[0].Children
	if len(cards) != 2 || cards[0].ID != "rows:a:1" || cards[1].ID != "rows:a:n1" {
		t.Fatalf("cards with the same id must still get ids of their own: %+v", cards)
	}
	li := findNode(&out.Root, "list").Children[0]
	if li.Type != ListItem || li.ID != "list:ch7" || li.Action != "tune" || li.Props["itemId"] != "ch/7 !" || li.Style.Module != "wittytv" {
		t.Errorf("list item: %+v", li)
	}
	if findNode(&out.Root, "empty") != nil || findNode(&out.Root, "rows:b") != nil {
		t.Error("empty rows were sent")
	}
	if err := Validate(&out); err != nil {
		t.Errorf("expanded screen doesn't validate: %v", err)
	}
}

func TestTopShelf(t *testing.T) {
	rows := []map[string]any{}
	for i := 0; i < 6; i++ {
		rows = append(rows, map[string]any{"title": fmt.Sprintf("Row %d", i), "cardShape": "wide", "items": manyItems(15, fmt.Sprintf("r%d-", i))})
	}
	rows[1]["items"] = []any{map[string]any{"id": "x", "title": "No route"}}
	ts := NewTopShelf(rows, ProfileFor(FormTV, "4k"), 900)
	if ts.Style != "sectioned" || len(ts.Sections) != MaxTopShelfSections || ts.ExpiresInSec != 900 {
		t.Fatalf("%+v", ts)
	}
	for _, s := range ts.Sections {
		if len(s.Items) != MaxTopShelfItems || s.Title == "Row 1" {
			t.Fatalf("section %q has %d items", s.Title, len(s.Items))
		}
	}
	it := ts.Sections[0].Items[0]
	// 16:9 on Top Shelf is 908×512 points; twice that on a 4K Apple TV.
	if it.Shape != "wide" || it.Image == nil || it.Image.Width != 1816 || it.Image.Height != 1024 || it.Image.URL != "/api/items/r0-0/poster?w=1816" || it.DisplayRoute != "/item/r0-0" || it.Progress != 0.5 {
		t.Errorf("%+v %+v", it, it.Image)
	}
	if hd := NewTopShelf(rows, nil, 0); hd.Sections[0].Items[0].Image.Width != 908 {
		t.Error("without a profile, Top Shelf should be sized for an Apple TV HD")
	}
}

// docs/sdui.schema.json is what client developers build against; it is made
// from the code, and this keeps it from falling behind.
func TestJSONSchemaFileIsCurrent(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "sdui.schema.json")
	got := JSONSchema()
	var parsed map[string]any
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatal(err)
	}
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./pkg/sdui -update)", err)
	}
	if !bytes.Equal(want, got) {
		t.Error("docs/sdui.schema.json is out of date: run go test ./pkg/sdui -update")
	}
	// The schema must stay open: a newer server's screen has to pass an older client's copy.
	if strings.Contains(string(got), `"additionalProperties": false`) {
		t.Error("the schema forbids properties it doesn't know, which breaks forward compatibility")
	}
}

func TestDiffAndApply_Layout(t *testing.T) {
	doc := HomeTemplate(HomeData{})
	a, _ := Render(doc, RenderContext{Profile: ProfileFor(FormTV, "hd"), Data: homeData(), Features: []string{"history"}})
	b, _ := Render(doc, RenderContext{Profile: ProfileFor(FormTV, "4k"), Data: homeData(), Features: []string{"history"}})
	got, err := Apply(a, Diff(a, b))
	if err != nil {
		t.Fatal(err)
	}
	if ETag(got) != ETag(b) {
		t.Error("applying the diff from the HD screen didn't give the 4K one")
	}
}
