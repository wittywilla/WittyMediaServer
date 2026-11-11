// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func baseCtx() RenderContext {
	return RenderContext{
		Platform: "web",
		Role:     "admin",
		Features: []string{"history", "playback"},
		Modules:  []string{"wittyplay", "wittytv"},
		Flags:    map[string]bool{"beta": true},
		Viewport: Viewport{Width: 1280, Height: 720},
		Locale:   "en-GB",
		Now:      time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func TestEvaluate_Predicates(t *testing.T) {
	ctx := baseCtx()
	cases := []struct {
		name string
		c    Condition
		want bool
	}{
		{"zero", Condition{}, true},
		{"feature yes", HasFeature("history"), true},
		{"feature no", HasFeature("nope"), false},
		{"role yes", IsRole("admin"), true},
		{"role no", IsRole("user"), false},
		{"platform yes", OnPlatform("web"), true},
		{"platform no", OnPlatform("tv"), false},
		{"module yes", ModuleEnabled("wittytv"), true},
		{"module no", ModuleEnabled("wittymusic"), false},
		{"minWidth ok", MinWidth(1280), true},
		{"minWidth fail", MinWidth(1281), false},
		{"maxWidth ok", MaxWidth(1280), true},
		{"maxWidth fail", MaxWidth(1279), false},
		{"minHeight ok", Condition{MinHeight: 720}, true},
		{"minHeight fail", Condition{MinHeight: 721}, false},
		{"maxHeight ok", Condition{MaxHeight: 720}, true},
		{"maxHeight fail", Condition{MaxHeight: 719}, false},
		{"locale prefix", InLocale("en"), true},
		{"locale exact", InLocale("en-GB"), true},
		{"locale case", InLocale("EN-gb"), true},
		{"locale no", InLocale("fr"), false},
		{"locale not partial word", InLocale("e"), false},
		{"flag yes", HasFlag("beta"), true},
		{"flag no", HasFlag("alpha"), false},
		{"when reserved", Condition{When: "anything"}, true},
		{"all true", All(IsRole("admin"), OnPlatform("web")), true},
		{"all one false", All(IsRole("admin"), OnPlatform("tv")), false},
		{"any one true", Any(IsRole("user"), OnPlatform("web")), true},
		{"any none", Any(IsRole("user"), OnPlatform("tv")), false},
		{"not true", Not(IsRole("admin")), false},
		{"not false", Not(IsRole("user")), true},
		{"nested", All(Any(IsRole("x"), HasFlag("beta")), Not(ModuleEnabled("wittymusic")), MinWidth(100)), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Evaluate(&tc.c, ctx); got != tc.want {
				t.Errorf("Evaluate(%+v) = %v, want %v", tc.c, got, tc.want)
			}
		})
	}
	if !Evaluate(nil, ctx) {
		t.Error("nil condition should be true")
	}
	// Unknown viewport ignores bounds.
	unknown := baseCtx()
	unknown.Viewport = Viewport{}
	if !Evaluate(&Condition{MinWidth: 5000, MaxHeight: 1}, unknown) {
		t.Error("unknown viewport should ignore bounds")
	}
}

func TestRender_PrunesHiddenSubtrees(t *testing.T) {
	doc := minimalDoc()
	doc.Root.Children = []Node{
		{Type: Stack, ID: "visible", Children: []Node{
			{Type: Spacer, ID: "inner-hidden", Visibility: &Condition{Role: "user"}},
			{Type: Spacer, ID: "inner-visible"},
		}},
		{Type: Stack, ID: "hidden", Visibility: &Condition{Platform: "tv"}, Children: []Node{
			{Type: Spacer, ID: "never"},
		}},
	}
	out, err := Render(doc, baseCtx())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Root.Children) != 1 || out.Root.Children[0].ID != "visible" {
		t.Fatalf("expected only 'visible', got %+v", out.Root.Children)
	}
	if len(out.Root.Children[0].Children) != 1 || out.Root.Children[0].Children[0].ID != "inner-visible" {
		t.Fatalf("inner pruning wrong: %+v", out.Root.Children[0].Children)
	}
	if out.Root.Children[0].Children[0].Visibility != nil {
		t.Error("visibility should be stripped")
	}
	// Input untouched.
	if len(doc.Root.Children) != 2 || doc.Root.Children[1].Visibility == nil {
		t.Error("Render mutated its input")
	}
	// Hidden root is an error.
	doc.Root.Visibility = &Condition{Role: "nobody"}
	if _, err := Render(doc, baseCtx()); err == nil {
		t.Error("hidden root should error")
	}
	// All children pruned → nil children.
	doc = minimalDoc()
	doc.Root.Children = []Node{{Type: Spacer, ID: "x", Visibility: &Condition{Role: "nobody"}}}
	out, _ = Render(doc, baseCtx())
	if out.Root.Children != nil {
		t.Error("expected nil children when all pruned")
	}
}

func TestRender_Bindings(t *testing.T) {
	ctx := baseCtx()
	ctx.Data = map[string]any{
		"user": map[string]any{"name": "willow poortenga", "since": "2024-03-09T10:00:00Z"},
		"items": []any{
			map[string]any{"title": "First", "runtime": float64(5400)},
			map[string]any{"title": "Second"},
		},
		"tags":  []string{"a", "b"},
		"maps":  []map[string]any{{"k": "v"}},
		"epoch": float64(0),
		"none":  nil,
	}
	cases := []struct {
		name string
		b    Binding
		want any
	}{
		{"plain", Binding{Path: "$.user.name"}, "willow poortenga"},
		{"upper", Binding{Path: "$.user.name", Format: "upper"}, "WILLOW POORTENGA"},
		{"lower", Binding{Path: "$.user.name", Format: "lower"}, "willow poortenga"},
		{"title", Binding{Path: "$.user.name", Format: "title"}, "Willow Poortenga"},
		{"index", Binding{Path: "$.items[1].title"}, "Second"},
		{"duration", Binding{Path: "$.items[0].runtime", Format: "duration"}, "1:30:00"},
		{"duration short", Binding{Path: "$.x", Default: 125, Format: "duration"}, "2:05"},
		{"duration string", Binding{Path: "$.x", Default: "61", Format: "duration"}, "1:01"},
		{"duration non-number", Binding{Path: "$.user.name", Format: "duration"}, "willow poortenga"},
		{"date string", Binding{Path: "$.user.since", Format: "date:2006-01-02"}, "2024-03-09"},
		{"date epoch", Binding{Path: "$.epoch", Format: "date:2006"}, "1970"},
		{"date default time", Binding{Path: "$.x", Default: time.Date(2020, 5, 6, 0, 0, 0, 0, time.UTC), Format: "date:Jan 2"}, "May 6"},
		{"date unparsable", Binding{Path: "$.user.name", Format: "date:2006"}, "willow poortenga"},
		{"string slice", Binding{Path: "$.tags[1]"}, "b"},
		{"map slice", Binding{Path: "$.maps[0].k"}, "v"},
		{"missing default", Binding{Path: "$.user.missing", Default: "fallback"}, "fallback"},
		{"nil value default", Binding{Path: "$.none", Default: "fallback"}, "fallback"},
		{"index oob default", Binding{Path: "$.items[9].title", Default: "x"}, "x"},
		{"string slice oob", Binding{Path: "$.tags[9]", Default: "x"}, "x"},
		{"map slice oob", Binding{Path: "$.maps[9]", Default: "x"}, "x"},
		{"index into map", Binding{Path: "$.user[0]", Default: "x"}, "x"},
		{"key into slice", Binding{Path: "$.items.title", Default: "x"}, "x"},
		{"missing no default", Binding{Path: "$.user.missing"}, nil},
		{"root", Binding{Path: "$.tags"}, []string{"a", "b"}},
		{"unknown format passthrough", Binding{Path: "$.epoch", Format: "weird"}, float64(0)},
		{"upper number", Binding{Path: "$.epoch", Format: "upper"}, "0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveBinding(tc.b, ctx.Data)
			if err != nil {
				t.Fatal(err)
			}
			if !jsonEqual(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
	if _, err := ResolveBinding(Binding{Path: "bad"}, nil); err == nil {
		t.Error("bad path should error")
	}
	if v, ok := Lookup(nil, []Segment{{Kind: KeySegment, Key: "a"}}); ok || v != nil {
		t.Error("lookup on nil data should miss")
	}

	// Through Render: props set, missing removed, bindings stripped.
	doc := minimalDoc()
	doc.Root.Children = []Node{
		{Type: Text, ID: "t", Props: map[string]any{"text": "static", "variant": "h1"},
			Bindings: map[string]Binding{"text": {Path: "$.user.name", Format: "title"}, "variant": {Path: "$.nope"}}},
		{Type: Text, ID: "t2", Bindings: map[string]Binding{"text": {Path: "$.nope"}}},
	}
	out, err := Render(doc, ctx)
	if err != nil {
		t.Fatal(err)
	}
	tn := out.Root.Children[0]
	if tn.Props["text"] != "Willow Poortenga" || tn.Bindings != nil {
		t.Errorf("binding not applied: %+v", tn)
	}
	if tn.Props["variant"] != "h1" {
		t.Error("unresolved binding without default should keep the static prop")
	}
	if out.Root.Children[1].Props != nil {
		t.Error("expected nil props when nothing resolved")
	}
	if doc.Root.Children[0].Props["text"] != "static" {
		t.Error("input mutated")
	}

	// Malformed path surfaces as a render error with a path.
	doc.Root.Children[0].Bindings["text"] = Binding{Path: "nope"}
	if _, err := Render(doc, ctx); err == nil || !strings.Contains(err.Error(), "/root/children/0/bindings/text") {
		t.Errorf("expected path in error, got %v", err)
	}
}

func TestRender_ModuleOverrides(t *testing.T) {
	doc := minimalDoc()
	doc.Theme = &ThemeRef{ID: "default", ModuleOverrides: map[string]map[string]string{
		"wittyplay": {"accent": "#111111", "bg": "#000000"},
	}}
	doc.Root.Children = []Node{
		{Type: Stack, ID: "games", Style: &Style{Module: "wittyplay", Tokens: map[string]string{"fg": "#eeeeee"}}},
		{Type: Stack, ID: "tv", Style: &Style{Module: "wittytv"}},
		{Type: Stack, ID: "plain", Style: &Style{Gap: 1}},
		{Type: Stack, ID: "nostyle"},
	}
	ctx := baseCtx()
	ctx.ThemeOverrides = map[string]map[string]string{
		"wittyplay": {"accent": "#222222"}, // context wins over document
		"wittytv":   {"accent": "#333333"},
	}
	out, err := Render(doc, ctx)
	if err != nil {
		t.Fatal(err)
	}
	games := out.Root.Children[0].Style.Tokens
	if games["accent"] != "#222222" || games["bg"] != "#000000" || games["fg"] != "#eeeeee" {
		t.Errorf("games tokens wrong: %v", games)
	}
	if out.Root.Children[1].Style.Tokens["accent"] != "#333333" {
		t.Errorf("tv tokens wrong: %v", out.Root.Children[1].Style.Tokens)
	}
	if out.Root.Children[2].Style.Tokens != nil || out.Root.Children[3].Style != nil {
		t.Error("unscoped nodes should get no tokens")
	}
	if doc.Root.Children[0].Style.Tokens["accent"] != "" {
		t.Error("input style mutated")
	}
	if out.Meta.GeneratedAt != ctx.Now || out.Meta.ETag == "" {
		t.Errorf("meta not stamped: %+v", out.Meta)
	}
}

func TestRender_Deterministic(t *testing.T) {
	data := map[string]any{"user": map[string]any{"name": "w"}, "home": map[string]any{
		"recentlyAdded": []any{map[string]any{"title": "a"}, map[string]any{"title": "b"}},
	}}
	for name, doc := range DefaultScreens() {
		ctx := baseCtx()
		ctx.Data = data
		a, err := Render(doc, ctx)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		ctx.Now = ctx.Now.Add(time.Hour) // GeneratedAt differs; canonical must not
		b, err := Render(doc, ctx)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		ca, _ := Canonical(a)
		cb, _ := Canonical(b)
		if !bytes.Equal(ca, cb) {
			t.Errorf("%s: render is not deterministic", name)
		}
		if a.Meta.ETag != b.Meta.ETag || a.Meta.ETag != ETag(b) {
			t.Errorf("%s: etag unstable", name)
		}
		if err := Validate(&a); err != nil {
			t.Errorf("%s: rendered output should still validate: %v", name, err)
		}
		if ETag(a) == ETag(minimalDoc()) {
			t.Errorf("%s: etag collides with an unrelated document", name)
		}
	}
	// Zero Now uses wall clock.
	out, err := Render(minimalDoc(), RenderContext{})
	if err != nil || out.Meta.GeneratedAt.IsZero() {
		t.Errorf("expected GeneratedAt to be stamped, got %v %v", out.Meta.GeneratedAt, err)
	}
}

func TestFormatHelpers(t *testing.T) {
	if formatDuration(-5) != "0:00" {
		t.Error("negative duration")
	}
	if formatDuration(3599.6) != "1:00:00" {
		t.Errorf("rounding: %s", formatDuration(3599.6))
	}
	if titleCase("hello-world foo") != "Hello-World Foo" {
		t.Errorf("titleCase: %s", titleCase("hello-world foo"))
	}
	if stringify(nil) != "" || stringify(true) != "true" || stringify(int64(3)) != "3" || stringify(float32(1.5)) != "1.5" || stringify(7) != "7" {
		t.Error("stringify")
	}
	if stringify(time.Duration(1)) != "1ns" {
		t.Error("stringify Stringer")
	}
	if stringify(struct{ A int }{1}) == "" {
		t.Error("stringify fallback")
	}
	if _, ok := toFloat("x"); ok {
		t.Error("toFloat bad string")
	}
	if f, ok := toFloat(int64(2)); !ok || f != 2 {
		t.Error("toFloat int64")
	}
	if f, ok := toFloat(float32(2)); !ok || f != 2 {
		t.Error("toFloat float32")
	}
	if _, ok := toFloat(true); ok {
		t.Error("toFloat bool")
	}
	if _, ok := toTime(true); ok {
		t.Error("toTime bool")
	}
	if tm, ok := toTime("2020-01-02"); !ok || tm.Year() != 2020 {
		t.Error("toTime date")
	}
	if tm, ok := toTime(int64(86400)); !ok || tm.Day() != 2 {
		t.Error("toTime int64")
	}
}

func TestClone_DeepCopiesEverything(t *testing.T) {
	doc := HomeTemplate(HomeData{Shelves: []ShelfData{{ID: "s", Title: "S", Items: []Item{{ID: "i", Title: "I", Badges: []string{"a"}}}}}})
	doc.Actions["x"] = Action{Type: Navigate, Target: "/", Params: map[string]any{"a": []any{1}, "m": map[string]any{"k": "v"}, "mm": []map[string]any{{"q": 1}}}}
	doc.Root.Visibility = &Condition{All: []Condition{{Role: "a"}}, Any: []Condition{{Role: "b"}}, Not: &Condition{Role: "c"}}
	c := cloneDocument(doc)
	ca, _ := Canonical(c)
	cb, _ := Canonical(doc)
	if !bytes.Equal(ca, cb) {
		t.Fatal("clone differs from original")
	}
	c.Actions["x"].Params["m"].(map[string]any)["k"] = "changed"
	c.Actions["x"].Params["a"].([]any)[0] = 2
	c.Actions["x"].Params["mm"].([]map[string]any)[0]["q"] = 2
	c.Theme.ModuleOverrides = map[string]map[string]string{"z": {}}
	c.Root.Visibility.All[0].Role = "zzz"
	c.Root.Visibility.Not.Role = "zzz"
	if doc.Actions["x"].Params["m"].(map[string]any)["k"] != "v" || doc.Actions["x"].Params["a"].([]any)[0] != 1 ||
		doc.Actions["x"].Params["mm"].([]map[string]any)[0]["q"] != 1 || doc.Theme.ModuleOverrides != nil ||
		doc.Root.Visibility.All[0].Role != "a" || doc.Root.Visibility.Not.Role != "c" {
		t.Error("clone shares memory with original")
	}
	if cloneStringMap(nil) != nil || cloneStringMapMap(nil) != nil || cloneAnyMap(nil) != nil {
		t.Error("nil maps should clone to nil")
	}
}
