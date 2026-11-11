// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/themes"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
	builtin "github.com/wittywilla/wittymediaserver/themes"
)

func TestThemeResolvesGlobalUserAndModule(t *testing.T) {
	ts, admin, adult, _ := sduiAccounts(t)
	if err := ts.Themes.Install(builtin.Builtin); err != nil {
		t.Fatal(err)
	}
	w := ts.do(t, "GET", "/api/v1/theme", adult, nil)
	if w.Code != 200 {
		t.Fatalf("theme: %d %s", w.Code, w.Body)
	}
	th := decode[ResolvedTheme](t, w)
	if th.ID != "default" || th.Source != "server" || th.Tokens["accent"] != "#7955FF" || th.ETag != w.Header().Get("ETag") || len(th.Warnings) != 0 {
		t.Fatalf("default theme: %+v", th)
	}
	if w := ts.do(t, "GET", "/api/theme", adult, nil, "If-None-Match", th.ETag); w.Code != 304 {
		t.Errorf("If-None-Match: %d", w.Code)
	}

	// The administrator sets the server's theme and overrides a colour.
	w = ts.do(t, "PUT", "/api/theme/global", admin, map[string]any{"id": "midnight", "tokens": map[string]string{"accent": "#ff8800"}})
	if w.Code != 200 {
		t.Fatalf("global: %d %s", w.Code, w.Body)
	}
	if ts.DB.GetSetting(t.Context(), "theme", "") != "midnight" {
		t.Error("the server theme setting wasn't changed")
	}
	th = decode[ResolvedTheme](t, ts.do(t, "GET", "/api/theme", adult, nil))
	if th.ID != "midnight" || th.Tokens["accent"] != "#FF8800" || th.Overrides["accent"] != "#FF8800" {
		t.Errorf("after global: %+v", th)
	}
	// /api/themes is unchanged and agrees about the default.
	if w := ts.do(t, "GET", "/api/themes", "", nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"default":"midnight"`) {
		t.Errorf("/api/themes: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", "/api/themes/midnight/tokens.json", "", nil); w.Code != 200 {
		t.Errorf("tokens.json: %d", w.Code)
	}

	// A module's colours, layered over the server's.
	w = ts.do(t, "PUT", "/api/theme/modules/wittyplay", admin, map[string]any{"tokens": map[string]string{"accent": "#3ad29f", "accentFg": "rgb(14, 16, 19)"}})
	if w.Code != 200 {
		t.Fatalf("module: %d %s", w.Code, w.Body)
	}
	if v := decode[themeModuleView](t, w); v.Tokens["accentFg"] != "#0E1013" || v.Module != "wittyplay" {
		t.Errorf("module answer: %+v", v)
	}
	th = decode[ResolvedTheme](t, ts.do(t, "GET", "/api/theme", adult, nil))
	if m := th.Modules["wittyplay"]; m.Tokens["accent"] != "#3AD29F" || m.Tokens["bg"] != th.Tokens["bg"] || m.Overrides["accent"] != "#3AD29F" {
		t.Errorf("module tokens: %+v", m)
	}
	// The account's own choice replaces the theme and the global overrides; modules stay.
	if w := ts.do(t, "PUT", "/api/me/ui", adult, map[string]any{"theme": "paper"}); w.Code != 200 {
		t.Fatalf("me/ui: %d", w.Code)
	}
	th = decode[ResolvedTheme](t, ts.do(t, "GET", "/api/theme", adult, nil))
	if th.ID != "paper" || th.Source != "user" || len(th.Overrides) != 0 || th.Modules["wittyplay"].Tokens["accent"] != "#3AD29F" {
		t.Errorf("own choice: %+v", th)
	}
	// A choice that isn't a theme here falls back to the server's.
	ts.do(t, "PUT", "/api/me/ui", adult, map[string]any{"theme": "gone"})
	if th := decode[ResolvedTheme](t, ts.do(t, "GET", "/api/theme", adult, nil)); th.ID != "midnight" || th.Source != "server" {
		t.Errorf("unknown own choice: %+v", th)
	}

	if w := ts.do(t, "GET", "/api/theme/modules/wittyplay", admin, nil); w.Code != 200 || decode[themeModuleView](t, w).Tokens["accent"] != "#3AD29F" {
		t.Errorf("get module: %d", w.Code)
	}
	if w := ts.do(t, "DELETE", "/api/theme/modules/wittyplay", admin, nil); w.Code != 200 || len(decode[themeModuleView](t, w).Tokens) != 0 {
		t.Errorf("delete module: %d", w.Code)
	}
	if th := decode[ResolvedTheme](t, ts.do(t, "GET", "/api/theme", admin, nil)); len(th.Modules) != 0 {
		t.Errorf("after delete: %+v", th.Modules)
	}
}

func TestThemeRejectsBadNamesAndColours(t *testing.T) {
	ts, admin, _, _ := sduiAccounts(t)
	for name, c := range map[string]struct {
		method, path string
		body         any
	}{
		"unknown token":        {"PUT", "/api/theme/modules/wittyplay", map[string]any{"tokens": map[string]string{"sparkle": "#ffffff"}}},
		"css injection":        {"PUT", "/api/theme/modules/wittyplay", map[string]any{"tokens": map[string]string{"accent": "#000;}body{display:none"}}},
		"url":                  {"PUT", "/api/theme/modules/wittyplay", map[string]any{"tokens": map[string]string{"accent": "url(https://x)"}}},
		"no tokens":            {"PUT", "/api/theme/modules/wittyplay", map[string]any{}},
		"bad module id":        {"PUT", "/api/theme/modules/Witty%20Play", map[string]any{"tokens": map[string]string{"accent": "#ffffff"}}},
		"bad module id (get)":  {"GET", "/api/theme/modules/UPPER", nil},
		"global unknown theme": {"PUT", "/api/theme/global", map[string]any{"id": "nosuchtheme"}},
		"global bad colour":    {"PUT", "/api/theme/global", map[string]any{"tokens": map[string]string{"bg": "blue-ish"}}},
		"global nothing":       {"PUT", "/api/theme/global", map[string]any{}},
		"global not json":      {"PUT", "/api/theme/global", []byte("{")},
	} {
		if w := ts.do(t, c.method, c.path, admin, c.body); w.Code != 400 {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	if w := ts.do(t, "PUT", "/api/theme/modules/wittyplay", admin, map[string]any{"tokens": map[string]string{"accent": "#ffffff"}}, "If-Match", `W/"ui-0-old"`); w.Code != 412 {
		t.Errorf("stale If-Match: %d", w.Code)
	}
}

func TestThemeContrastWarnings(t *testing.T) {
	ts, admin, _, _ := sduiAccounts(t)
	// Dark grey text on the default theme's near-black background: well under 4.5:1.
	w := ts.do(t, "PUT", "/api/theme/global", admin, map[string]any{"tokens": map[string]string{"fg": "#333333"}})
	if w.Code != 200 {
		t.Fatalf("global: %d %s", w.Code, w.Body)
	}
	th := decode[ResolvedTheme](t, w)
	found := false
	for _, x := range th.Warnings {
		if x.Scope == "global" && x.Text == "fg" && x.Surface == "bg" && x.Ratio < 4.5 && x.Need == 4.5 {
			found = true
		}
	}
	if !found {
		t.Errorf("no warning for fg on bg: %+v", th.Warnings)
	}
	w = ts.do(t, "PUT", "/api/theme/modules/wittytv", admin, map[string]any{"tokens": map[string]string{"accent": "#ffff00", "accentFg": "#ffffff"}})
	if v := decode[themeModuleView](t, w); w.Code != 200 || len(v.Warnings) == 0 || v.Warnings[0].Text != "accentFg" {
		t.Errorf("module warning: %d %+v", w.Code, v.Warnings)
	}
}

// Module colours set through /api/theme reach server-driven pages: a section's scope, a
// page's scope, and the ETag changes with them.
func TestModuleColoursReachRenderedPages(t *testing.T) {
	ts, admin, adult, _ := sduiAccounts(t)
	page := sdui.Page{ID: "game-night", Kind: "custom", Title: "Games", Scope: "wittyplay", Sections: []sdui.Section{
		{ID: "note", Component: "Notice", Scope: "wittyplay", Props: map[string]any{"text": "Game night is on Friday."}}}}
	etag := ts.do(t, "GET", "/api/sdui/config", admin, nil).Header().Get("ETag")
	if w := ts.do(t, "PUT", "/api/sdui/config/pages/game-night", admin, page, "If-Match", etag); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	before := ts.do(t, "GET", "/api/sdui/pages/game-night", adult, nil).Header().Get("ETag")
	if w := ts.do(t, "PUT", "/api/theme/modules/wittyplay", admin, map[string]any{"tokens": map[string]string{"accent": "#3ad29f"}}); w.Code != 200 {
		t.Fatalf("module: %d %s", w.Code, w.Body)
	}
	w := ts.do(t, "GET", "/api/sdui/pages/game-night", adult, nil, "If-None-Match", before)
	if w.Code != 200 {
		t.Fatalf("the page didn't change with its module's colours: %d", w.Code)
	}
	got, err := sdui.Unmarshal(w.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme == nil || got.Theme.Scope != "wittyplay" || got.Theme.Tokens["accent"] != "#3AD29F" {
		t.Errorf("the page's scope: %+v", got.Theme)
	}
	if n := got.Root.Children[0]; n.Scope != "wittyplay" || n.Tokens["accent"] != "#3AD29F" {
		t.Errorf("the section's scope: %q %v", n.Scope, n.Tokens)
	}
}

func TestContrastMatchesTheScript(t *testing.T) {
	// The values scripts/theme-contrast.py gives for these pairs.
	for _, c := range []struct {
		a, b string
		want float64
	}{{"#000000", "#ffffff", 21}, {"#777777", "#ffffff", 4.48}, {"#e8eaf0", "#0f1115", 15.71}} {
		r, ok := themes.ContrastRatio(c.a, c.b)
		if !ok || r < c.want-0.01 || r > c.want+0.01 {
			t.Errorf("%s on %s: %.2f, want %.2f", c.a, c.b, r, c.want)
		}
	}
	if _, ok := themes.ContrastRatio("#00000080", "#ffffff"); ok {
		t.Error("a translucent colour has no single contrast")
	}
}

// The automatic theme is two looks. Each is resolved with the same overrides, and an
// override that is readable on one and not the other is warned about for that one only.
func TestThemeVariants(t *testing.T) {
	ts, admin, adult, _ := sduiAccounts(t)
	if err := ts.Themes.Install(builtin.Builtin); err != nil {
		t.Fatal(err)
	}
	th := decode[ResolvedTheme](t, ts.do(t, "GET", "/api/theme", adult, nil))
	if th.Scheme != "dark" || len(th.Variants) != 1 || th.Variants["dark"].ID != "default" || th.Variants["dark"].Tokens["bg"] != th.Tokens["bg"] {
		t.Fatalf("an ordinary theme: scheme %q variants %v", th.Scheme, th.Variants)
	}
	// Near-white text: fine on the dark look, unreadable on the light one.
	w := ts.do(t, "PUT", "/api/theme/global", admin, map[string]any{"id": "auto", "tokens": map[string]string{"fg": "#f4f4f4"}})
	if w.Code != 200 {
		t.Fatalf("auto: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "PUT", "/api/theme/modules/wittytv", admin, map[string]any{"tokens": map[string]string{"accent": "#00a2ff"}}); w.Code != 200 {
		t.Fatalf("module: %d %s", w.Code, w.Body)
	}
	th = decode[ResolvedTheme](t, ts.do(t, "GET", "/api/theme", adult, nil))
	dark, light := th.Variants["dark"], th.Variants["light"]
	if th.ID != "auto" || th.Scheme != "auto" || dark.ID != "default" || light.ID != "light" {
		t.Fatalf("auto: %+v", th)
	}
	if dark.Tokens["fg"] != "#F4F4F4" || light.Tokens["fg"] != "#F4F4F4" || dark.Tokens["bg"] == light.Tokens["bg"] {
		t.Errorf("both looks get the override over their own colours: %v / %v", dark.Tokens["bg"], light.Tokens["bg"])
	}
	if dark.Modules["wittytv"]["accent"] != "#00A2FF" || light.Modules["wittytv"]["accent"] != "#00A2FF" || light.Modules["wittytv"]["bg"] != light.Tokens["bg"] {
		t.Errorf("a module's colours in each look: %v", light.Modules["wittytv"])
	}
	// The top-level colours stay the dark look's, for clients that know nothing of variants.
	if th.Tokens["bg"] != dark.Tokens["bg"] {
		t.Error("tokens aren't the dark look's")
	}
	var onLight, onDark int
	for _, w := range th.Warnings {
		if w.Text == "fg" && w.Scope == "global" {
			switch w.Variant {
			case "light":
				onLight++
			case "dark":
				onDark++
			}
		}
	}
	if onLight == 0 || onDark != 0 {
		t.Errorf("near-white text should be warned about on the light look only: %+v", th.Warnings)
	}
}
