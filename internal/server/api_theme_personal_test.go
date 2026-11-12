// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/brandname"
	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymediaserver/internal/themes"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
	builtin "github.com/wittywilla/wittymediaserver/themes"
)

func getTheme(t *testing.T, ts *testServer, token string) ResolvedTheme {
	t.Helper()
	w := ts.do(t, "GET", "/api/v1/theme", token, nil)
	if w.Code != 200 {
		t.Fatalf("theme: %d %s", w.Code, w.Body)
	}
	return decode[ResolvedTheme](t, w)
}

func putPersonal(t *testing.T, ts *testServer, token string, body any) themePersonalView {
	t.Helper()
	w := ts.do(t, "PUT", "/api/v1/theme/personal", token, body)
	if w.Code != 200 {
		t.Fatalf("personal: %d %s", w.Code, w.Body)
	}
	return decode[themePersonalView](t, w)
}

// The layers, lowest first: the pack, the administrator's global colours, the module's own,
// the administrator's for the module, the profile's own, the profile's own for the module.
func TestThemePrecedence(t *testing.T) {
	const (
		global      = "#FF8800"
		moduleOwn   = "#7B1FA2" // comicsUI's
		moduleAdmin = "#1565C0"
		personal    = "#D02060"
		personalMod = "#2E7D32"
	)
	for _, c := range []struct {
		name                            string
		admin, adminMod, mine, mineMod  bool
		wantTokens, wantModule          string
		wantModuleTint, wantTokensFocus bool
	}{
		{name: "the module's own colours over the pack", wantTokens: "#7955FF", wantModule: moduleOwn},
		{name: "global under the module", admin: true, wantTokens: global, wantModule: moduleOwn},
		{name: "the administrator's for the module over its own", admin: true, adminMod: true, wantTokens: global, wantModule: moduleAdmin},
		{name: "the profile's own over everything the administrator set", admin: true, adminMod: true, mine: true, wantTokens: personal, wantModule: personal},
		{name: "the profile's own for the module over its own", admin: true, adminMod: true, mine: true, mineMod: true, wantTokens: personal, wantModule: personalMod},
		{name: "the profile's own for the module alone", mineMod: true, wantTokens: "#7955FF", wantModule: personalMod},
	} {
		t.Run(c.name, func(t *testing.T) {
			ts, admin, adult, _ := sduiAccounts(t)
			if err := ts.Themes.Install(builtin.Builtin); err != nil {
				t.Fatal(err)
			}
			ts.Modules = installedBundleWith(t, `export default 1`, `a{}`, modmgr.SDUIAPI, map[string][]byte{sdui.ModuleFile: []byte(comicsUI)})
			if c.admin {
				ts.do(t, "PUT", "/api/theme/global", admin, map[string]any{"tokens": map[string]string{"accent": global}})
			}
			if c.adminMod {
				ts.do(t, "PUT", "/api/theme/modules/wittycomics", admin, map[string]any{"tokens": map[string]string{"accent": moduleAdmin}})
			}
			body := map[string]any{}
			if c.mine {
				body["tokens"] = map[string]string{"accent": personal}
			}
			if c.mineMod {
				body["modules"] = map[string]any{"wittycomics": map[string]string{"accent": personalMod}}
			}
			if len(body) > 0 {
				putPersonal(t, ts, adult, body)
			}
			th := getTheme(t, ts, adult)
			mod := th.Modules["wittycomics"].Tokens
			if th.Tokens["accent"] != c.wantTokens || mod["accent"] != c.wantModule {
				t.Fatalf("accent %s, module %s; want %s, %s", th.Tokens["accent"], mod["accent"], c.wantTokens, c.wantModule)
			}
			// Every layer brings its derived colours: the tint follows whichever accent won.
			if th.Tokens["accentSoft"] != c.wantTokens+"24" || mod["accentSoft"] != c.wantModule+"24" {
				t.Errorf("tints: %s / %s", th.Tokens["accentSoft"], mod["accentSoft"])
			}
			if mod["focus"] != mod["accentText"] || th.Tokens["focus"] != th.Tokens["accentText"] {
				t.Errorf("focus doesn't follow the accent as text")
			}
			// Server-driven pages carry the scope's palette as it differs from the account's own.
			doc := sduiDoc(t, ts.do(t, "GET", "/api/v1/sdui/pages/wittycomics?formFactor=desktop", adult, nil))
			want := map[string]string{}
			for k, v := range mod {
				if th.Tokens[k] != v {
					want[k] = v
				}
			}
			if doc.Theme == nil || (len(want) > 0 || len(doc.Theme.Tokens) > 0) && !reflect.DeepEqual(doc.Theme.Tokens, want) {
				t.Errorf("page tokens %v, want %v", doc.Theme, want)
			}
			// Someone else sees none of this profile's colours.
			_, other := ts.user(t, "sam", "adult", false, "")
			if o := getTheme(t, ts, other); len(o.Personal) != 0 || o.Modules["wittycomics"].Personal != nil ||
				(c.mine && o.Tokens["accent"] == personal) || (c.mineMod && o.Modules["wittycomics"].Tokens["accent"] == personalMod) {
				t.Errorf("another profile got this one's colours: %+v", o.Personal)
			}
		})
	}
}

func TestThemePersonalRoutes(t *testing.T) {
	ts, admin, adult, kid := sduiAccounts(t)
	if err := ts.Themes.Install(builtin.Builtin); err != nil {
		t.Fatal(err)
	}
	if w := ts.do(t, "GET", "/api/v1/theme/personal", "", nil); w.Code != 401 {
		t.Errorf("signed out: %d", w.Code)
	}
	empty := decode[themePersonalView](t, ts.do(t, "GET", "/api/v1/theme/personal", adult, nil))
	if len(empty.Tokens) != 0 || len(empty.Modules) != 0 || empty.Warnings == nil {
		t.Errorf("nothing yet: %+v", empty)
	}
	// The profile's interface choices are kept as they were.
	ts.do(t, "PUT", "/api/me/ui", adult, map[string]any{"theme": "midnight", "home": map[string]any{"v": 2}})
	before := getTheme(t, ts, adult)

	v := putPersonal(t, ts, adult, map[string]any{"tokens": map[string]string{"accent": "#d02060"}, "modules": map[string]any{"wittyplay": map[string]string{"accent": "rgb(46, 125, 50)"}, "empty": map[string]string{}}})
	if v.Tokens["accent"] != "#D02060" || v.Modules["wittyplay"]["accent"] != "#2E7D32" || len(v.Modules) != 1 {
		t.Errorf("normalised: %+v", v)
	}
	after := getTheme(t, ts, adult)
	if after.ETag == before.ETag || v.ETag != after.ETag {
		t.Errorf("ETag: before %s after %s answer %s", before.ETag, after.ETag, v.ETag)
	}
	if w := ts.do(t, "GET", "/api/v1/theme", adult, nil, "If-None-Match", before.ETag); w.Code != 200 {
		t.Errorf("an old ETag: %d", w.Code)
	}
	if after.ID != "midnight" || after.Personal["accent"] != "#D02060" || after.Modules["wittyplay"].Personal["accent"] != "#2E7D32" {
		t.Errorf("resolved: %+v", after)
	}
	ui := map[string]any{}
	_ = json.Unmarshal(ts.do(t, "GET", "/api/me/ui", adult, nil).Body.Bytes(), &ui)
	if ui["theme"] != "midnight" || ui["home"] == nil || ui[themePersonalKey] == nil {
		t.Errorf("me/ui: %v", ui)
	}

	for name, body := range map[string]any{
		"unknown token":    map[string]any{"tokens": map[string]string{"sparkle": "#ffffff"}},
		"not a colour":     map[string]any{"tokens": map[string]string{"accent": "#000;}body{display:none"}},
		"url":              map[string]any{"tokens": map[string]string{"bg": "url(https://x)"}},
		"bad scope":        map[string]any{"modules": map[string]any{"Witty Play": map[string]string{"accent": "#ffffff"}}},
		"bad module token": map[string]any{"modules": map[string]any{"wittyplay": map[string]string{"nope": "#ffffff"}}},
		"unknown field":    map[string]any{"tokens": map[string]string{}, "profile": 3},
		"not json":         []byte("{"),
		"a list":           []byte(`["accent"]`),
		"low contrast":     map[string]any{"tokens": map[string]string{"fg": "#333333"}},
		"low on a module":  map[string]any{"modules": map[string]any{"wittyplay": map[string]string{"accent": "#ffff00", "accentFg": "#ffffff"}}},
	} {
		if w := ts.do(t, "PUT", "/api/v1/theme/personal", adult, body); w.Code != 400 {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	// A refusal changes nothing.
	if th := getTheme(t, ts, adult); th.ETag != after.ETag {
		t.Error("a refused change was kept")
	}
	// The refusal names the pairs that fail.
	w := ts.do(t, "PUT", "/api/v1/theme/personal", adult, map[string]any{"tokens": map[string]string{"fg": "#333333"}})
	var refusal struct {
		Error    string         `json:"error"`
		Contrast []ThemeWarning `json:"contrast"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &refusal)
	found := false
	for _, c := range refusal.Contrast {
		if c.Scope == "personal" && c.Text == "fg" && c.Surface == "bg" && c.Ratio < themes.ContrastAA {
			found = true
		}
	}
	if !found || !strings.Contains(refusal.Error, "fg on bg") {
		t.Errorf("refusal: %s", w.Body)
	}
	// The automatic theme: refused when either look fails.
	ts.do(t, "PUT", "/api/theme/global", admin, map[string]any{"id": "auto"})
	if w := ts.do(t, "PUT", "/api/v1/theme/personal", kid, map[string]any{"tokens": map[string]string{"fg": "#f4f4f4"}}); w.Code != 400 || !strings.Contains(w.Body.String(), `"variant":"light"`) {
		t.Errorf("auto, near-white text: %d %s", w.Code, w.Body)
	}
	// The administrator's colours keep warning instead of refusing.
	if w := ts.do(t, "PUT", "/api/theme/global", admin, map[string]any{"tokens": map[string]string{"fg": "#333333"}}); w.Code != 200 {
		t.Errorf("admin: %d", w.Code)
	}
	// Taking them away.
	w = ts.do(t, "DELETE", "/api/v1/theme/personal", adult, nil)
	if d := decode[themePersonalView](t, w); w.Code != 200 || len(d.Tokens) != 0 || len(d.Modules) != 0 {
		t.Errorf("delete: %d %+v", w.Code, d)
	}
	ui = map[string]any{}
	_ = json.Unmarshal(ts.do(t, "GET", "/api/me/ui", adult, nil).Body.Bytes(), &ui)
	if ui["theme"] != "midnight" || ui[themePersonalKey] != nil {
		t.Errorf("me/ui after delete: %v", ui)
	}
}

// The theme carries its corners and type, in every look.
func TestThemeShapeAndType(t *testing.T) {
	ts, admin, adult, _ := sduiAccounts(t)
	if err := ts.Themes.Install(builtin.Builtin); err != nil {
		t.Fatal(err)
	}
	ts.do(t, "PUT", "/api/theme/global", admin, map[string]any{"id": "auto"})
	th := getTheme(t, ts, adult)
	want := themes.ShapeFor(10)
	if th.Shape != want || th.Type.Weights["heading"] != 650 || len(th.Type.Body) == 0 || th.Type.Display[0] != "Inter" {
		t.Errorf("shape %+v type %+v", th.Shape, th.Type)
	}
	for scheme, v := range th.Variants {
		if v.Shape != want || len(v.Type.Body) == 0 {
			t.Errorf("%s: %+v", scheme, v.Shape)
		}
	}
}

// Design 2: the theme carries its ten-foot colours and units, the elevation table in its own
// colours and the density steps, in every look, and the brand is untouched.
func TestThemeCarriesDesign2Tokens(t *testing.T) {
	ts, admin, adult, _ := sduiAccounts(t)
	if err := ts.Themes.Install(builtin.Builtin); err != nil {
		t.Fatal(err)
	}
	ts.do(t, "PUT", "/api/theme/global", admin, map[string]any{"id": "auto"})
	th := getTheme(t, ts, adult)
	if th.TV.RingColor == "" || th.TV.TVText == "" || th.TV.HeroScrim <= 0 || len(th.TV.Units) == 0 {
		t.Errorf("tv: %+v", th.TV)
	}
	if th.Elevation["1"].Fill != th.Tokens["bg2"] || th.Elevation["3"].Line != th.Tokens["borderStrong"] || th.Density.Default != "comfortable" {
		t.Errorf("elevation %+v density %+v", th.Elevation, th.Density)
	}
	dark, light := th.Variants["dark"], th.Variants["light"]
	if dark.Elevation["0"].Fill == light.Elevation["0"].Fill || dark.TV.TVText == light.TV.TVText {
		t.Errorf("each look has its own: %+v / %+v", dark.TV, light.TV)
	}
	if th.Brand.Token != "brandAccent" || th.Brand.Color == "" {
		t.Errorf("brand: %+v", th.Brand)
	}
	// A profile's text colour re-derives the ten-foot colours instead of keeping the theme's.
	ts.do(t, "PUT", "/api/theme/global", admin, map[string]any{"id": "default"})
	if w := ts.do(t, "PUT", "/api/theme/personal", adult, map[string]any{"tokens": map[string]string{"fg": "#FFCC00"}}); w.Code != 200 {
		t.Fatalf("personal: %d %s", w.Code, w.Body)
	}
	if got := getTheme(t, ts, adult); got.TV.RingColor != "#FFCC00" {
		t.Errorf("ring after a text override: %+v", got.TV)
	}
}

// The server's name keeps its colouring through every layer: the parts are the settings'
// and the coloured ones take brandAccent, resolved with the other colours.
func TestBrandColouringSurvivesTheThemeEngine(t *testing.T) {
	ts, admin, adult, _ := sduiAccounts(t)
	if err := ts.Themes.Install(builtin.Builtin); err != nil {
		t.Fatal(err)
	}
	if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"brandName": "MyHomeTheater", "brandColouring": "pick:2-6"}); w.Code != 200 {
		t.Fatalf("settings: %d %s", w.Code, w.Body)
	}
	want := brandname.Parts("MyHomeTheater", "pick:2-6")
	status := func() []brandname.Part {
		var s struct {
			BrandParts []brandname.Part `json:"brandParts"`
		}
		_ = json.Unmarshal(ts.do(t, "GET", "/api/status", "", nil).Body.Bytes(), &s)
		return s.BrandParts
	}
	list := func() []brandname.Part {
		var s struct {
			BrandParts     []brandname.Part `json:"brandParts"`
			BrandColouring string           `json:"brandColouring"`
		}
		_ = json.Unmarshal(ts.do(t, "GET", "/api/themes", "", nil).Body.Bytes(), &s)
		if s.BrandColouring != "pick:2-6" {
			t.Errorf("brandColouring %q", s.BrandColouring)
		}
		return s.BrandParts
	}
	check := func(stage, color string) {
		t.Helper()
		th := getTheme(t, ts, adult)
		if !reflect.DeepEqual(status(), want) || !reflect.DeepEqual(list(), want) || !reflect.DeepEqual(th.Brand.Parts, want) {
			t.Errorf("%s: the parts changed: %v / %v / %v", stage, status(), list(), th.Brand.Parts)
		}
		if th.Brand.Token != "brandAccent" || th.Brand.Color != th.Tokens["brandAccent"] || th.Brand.Color != color {
			t.Errorf("%s: brand colour %+v, tokens %s, want %s", stage, th.Brand, th.Tokens["brandAccent"], color)
		}
		for _, w := range th.Warnings {
			if w.Text == "brandAccent" {
				t.Errorf("%s: the name is hard to read: %+v", stage, w)
			}
		}
	}
	check("the theme's", "#9174FF")
	// A global accent: the name follows the accent as text worked out from it.
	ts.do(t, "PUT", "/api/theme/global", admin, map[string]any{"tokens": map[string]string{"accent": "#2E7D32"}})
	g := getTheme(t, ts, adult)
	if g.Tokens["brandAccent"] == "#9174FF" || g.Tokens["brandAccent"] != g.Tokens["accentText"] {
		t.Fatalf("brandAccent didn't follow the accent: %s / %s", g.Tokens["brandAccent"], g.Tokens["accentText"])
	}
	check("global accent", g.Tokens["accentText"])
	// The profile's own colour for it wins.
	putPersonal(t, ts, adult, map[string]any{"tokens": map[string]string{"brandAccent": "#FFB74D"}})
	check("own brandAccent", "#FFB74D")
	// A colour for it that can't be read on the top bar is refused like any text.
	if w := ts.do(t, "PUT", "/api/v1/theme/personal", adult, map[string]any{"tokens": map[string]string{"brandAccent": "#20242C"}}); w.Code != 400 {
		t.Errorf("unreadable name: %d", w.Code)
	}
	check("after a refusal", "#FFB74D")
}

// The name's per-part colours, its mark and the contrast warnings come with the theme.
func TestBrandCarriesColoursMarkAndWarnings(t *testing.T) {
	ts, admin, adult, _ := sduiAccounts(t)
	if err := ts.Themes.Install(builtin.Builtin); err != nil {
		t.Fatal(err)
	}
	set := map[string]string{"brandName": "WittyMediaServer", "brandMark": `{"kind":"initials"}`,
		"brandSpans": `[{"from":5,"to":10,"color":{"kind":"static","hex":"#2a2a2a"}}]`}
	if w := ts.do(t, "PUT", "/api/settings", admin, set); w.Code != 200 {
		t.Fatalf("settings: %d %s", w.Code, w.Body)
	}
	th := getTheme(t, ts, adult)
	if th.Brand.Mark.Kind != "initials" || th.Brand.Mark.Shown != "WMS" {
		t.Errorf("mark: %+v", th.Brand.Mark)
	}
	if len(th.Brand.Parts) != 3 || th.Brand.Parts[1].Color == nil || th.Brand.Parts[1].Color.Hex != "#2a2a2a" {
		t.Errorf("parts: %+v", th.Brand.Parts)
	}
	if len(th.Brand.Warnings) == 0 {
		t.Errorf("a dark grey word on a dark theme should warn: %+v", th.Brand.Warnings)
	}
	if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"brandSpans": `[{"from":0,"to":3,"color":{"kind":"static","hex":"red"}}]`}); w.Code != 400 {
		t.Errorf("a colour that isn't one: %d", w.Code)
	}
}
