// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/themes"
)

// The colour tokens the table names are the theme engine's.
func TestTokenNamesAreTheThemes(t *testing.T) {
	got := append([]string(nil), ColorTokenNames...)
	sort.Strings(got)
	if want := themes.ColorTokens(); !slices.Equal(got, want) {
		t.Errorf("ColorTokenNames %v, themes have %v", got, want)
	}
	// Shape tokens are fields of the theme's shape.
	fields := map[string]bool{}
	st := reflect.TypeOf(themes.Shape{})
	for i := range st.NumField() {
		fields[strings.Split(st.Field(i).Tag.Get("json"), ",")[0]] = true
	}
	for _, s := range shapeTokens {
		if !fields[s.Value] {
			t.Errorf("shape token %s isn't in GET /theme shape", s.Value)
		}
	}
	// Text roles are the device profiles' text roles.
	for _, ff := range FormFactors {
		roles := ProfileFor(ff, "").Text
		if len(roles) != len(typeRoles) {
			t.Errorf("%s has text roles %v, the table %v", ff, roles, typeRoles)
		}
		for r := range typeRoles {
			if _, ok := roles[r]; !ok {
				t.Errorf("%s has no text role %s", ff, r)
			}
		}
	}
	weights := themes.DefaultType().Weights
	for r, tr := range typeRoles {
		if (tr.Family != "display" && tr.Family != "body") || weights[tr.Weight] == 0 {
			t.Errorf("type role %s: %+v", r, tr)
		}
	}
}

// Every component and card style says what it is drawn with, in the theme's words only.
func TestEveryComponentHasTokens(t *testing.T) {
	c := Catalog()
	if c.Theme == nil || len(c.Theme.ColorRoles) == 0 || len(c.Theme.TypeRoles) == 0 || len(c.Theme.ShapeTokens) == 0 {
		t.Fatal("the catalogue has no theme vocabulary")
	}
	roles, colours, shapes := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, r := range colorRoles {
		roles[r.Value] = true
	}
	for _, n := range ColorTokenNames {
		colours[n] = true
	}
	for _, s := range shapeTokens {
		shapes[s.Value] = true
	}
	check := func(what string, tr *TokenRoles) {
		if tr == nil || tr.Colors == nil {
			t.Errorf("%s has no tokens", what)
			return
		}
		for role, tok := range tr.Colors {
			if !roles[role] || !colours[tok] {
				t.Errorf("%s colour %s → %s: unknown role or token", what, role, tok)
			}
		}
		for part, role := range tr.Type {
			if _, ok := typeRoles[role]; !ok {
				t.Errorf("%s text %s → %s: not a text role", what, part, role)
			}
		}
		for part, tok := range tr.Shape {
			if !shapes[tok] {
				t.Errorf("%s shape %s → %s: not a shape token", what, part, tok)
			}
		}
	}
	for _, comp := range c.Components {
		check(comp.Type, comp.Tokens)
	}
	for _, s := range c.CardStyles {
		check("card style "+s.Style, s.Tokens)
	}
	if len(componentTokens) != len(c.Components) {
		t.Errorf("the table has %d components, the catalogue %d", len(componentTokens), len(c.Components))
	}
	// Anything with text has its text colour, and anything that takes focus a ring.
	for _, comp := range c.Components {
		if len(comp.Tokens.Type) > 0 && comp.Tokens.Colors["text"] == "" {
			t.Errorf("%s has text but no text colour", comp.Type)
		}
		if comp.Focus != "none" && comp.Tokens.Colors["focus"] == "" {
			t.Errorf("%s takes focus but has no focus colour", comp.Type)
		}
	}
	if s, _ := CardStyle("circle"); s.Tokens.Shape["corner"] != "pill" {
		t.Errorf("a circle card: %+v", s.Tokens.Shape)
	}
}

// A module's component is drawn as the one it expands into.
func TestModuleComponentTakesItsBasesTokens(t *testing.T) {
	spec := moduleComponentSpec(&ModuleComponent{Type: "wittyplay.ConsoleShelf", Label: "Consoles", Section: Section{ID: "x", Component: "Shelf"}})
	base, _ := Component("Shelf")
	if spec.Tokens == nil || !reflect.DeepEqual(spec.Tokens, base.Tokens) {
		t.Errorf("module component tokens: %+v", spec.Tokens)
	}
}
