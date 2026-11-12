// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package themes

import (
	"encoding/json"
	"io/fs"
	"strconv"
	"strings"

	builtin "github.com/wittywilla/wittymediaserver/themes"
)

// Shape is a theme's corners and focus ring, in points (CSS pixels), for apps that draw
// their own controls. The web's stylesheet works every corner out from the theme's one
// --radius (web/src/styles.css "shape"); ShapeRoles says which step each part uses, and
// themes/tokens.json carries the same table.
type Shape struct {
	// Radius is the theme's --radius, the medium corner.
	Radius float64 `json:"radius"`
	Card   float64 `json:"card"`   // cards and their pictures
	Button float64 `json:"button"` // buttons
	Field  float64 `json:"field"`  // text fields, selects, search boxes
	Row    float64 `json:"row"`    // list rows
	Thumb  float64 `json:"thumb"`  // small pictures in a row
	Chip   float64 `json:"chip"`   // chips and filter pills: fully round
	Badge  float64 `json:"badge"`  // badges and small labels
	Image  float64 `json:"image"`  // pictures and banners placed on a page, the TV spotlight
	Sheet  float64 `json:"sheet"`  // menus, popovers, sheets
	Dialog float64 `json:"dialog"` // dialogs
	Pill   float64 `json:"pill"`   // fully round, whatever the size: a circle on a square
	// The focus ring: line width and gap, in points, and how much a focused card grows.
	// Ten-foot screens use themes/tokens.json tenFoot.focus instead.
	FocusWidth  float64 `json:"focusWidth"`
	FocusOffset float64 `json:"focusOffset"`
	FocusScale  float64 `json:"focusScale"`
}

// ShapeRoles are the steps of the corner scale each part of Shape takes, as multiples of
// Radius (xs .4, sm .6, md 1, lg 1.4, xl 2) or "pill".
var ShapeRoles = map[string]string{
	"card": "md", "button": "md", "field": "md", "row": "md", "thumb": "sm", "chip": "pill",
	"badge": "xs", "image": "lg", "sheet": "lg", "dialog": "xl",
}

// radiusSteps are web/src/styles.css --r-xs … --r-xl.
var radiusSteps = map[string]float64{"xs": 0.4, "sm": 0.6, "md": 1, "lg": 1.4, "xl": 2}

// PillRadius is --r-full.
const PillRadius = 999

// ShapeFor works a Shape out from a radius, with the stylesheet's focus ring.
func ShapeFor(radius float64) Shape {
	if radius < 0 {
		radius = 0
	}
	step := func(part string) float64 {
		if ShapeRoles[part] == "pill" {
			return PillRadius
		}
		return round2(radius * radiusSteps[ShapeRoles[part]])
	}
	return Shape{Radius: radius, Card: step("card"), Button: step("button"), Field: step("field"), Row: step("row"),
		Thumb: step("thumb"), Chip: step("chip"), Badge: step("badge"), Image: step("image"), Sheet: step("sheet"),
		Dialog: step("dialog"), Pill: PillRadius, FocusWidth: 2, FocusOffset: 2, FocusScale: 1}
}

func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }

// TypeTokens are a theme's typefaces and weights. Sizes are the device's: the rendered
// document's layout.text, by text role.
type TypeTokens struct {
	// Display is for page titles and headings, Body for everything else, Mono for code;
	// each is a list of families, best first, ending in a generic one.
	Display []string `json:"display"`
	Body    []string `json:"body"`
	Mono    []string `json:"mono"`
	// Weights: body, medium, strong and heading, 100 to 900.
	Weights map[string]int `json:"weights"`
}

// The stylesheet's own type, for themes that don't set it.
var (
	defaultFonts = []string{"Inter", "Segoe UI Variable Text", "system-ui", "-apple-system", "Segoe UI", "Roboto", "Helvetica", "Arial", "sans-serif"}
	defaultMono  = []string{"ui-monospace", "Cascadia Mono", "SFMono-Regular", "Menlo", "Consolas", "monospace"}
)

// DefaultType is the stylesheet's type.
func DefaultType() TypeTokens {
	return TypeTokens{Display: append([]string(nil), defaultFonts...), Body: append([]string(nil), defaultFonts...),
		Mono: append([]string(nil), defaultMono...), Weights: map[string]int{"body": 400, "medium": 500, "strong": 600, "heading": 650}}
}

// Look is a theme's shape and type: from its theme.json when it has them (the built-in
// themes do, written by scripts/design-tokens.py), else from its CSS variables (--radius,
// --focus-width, --focus-offset, --font, --font-mono, --weight-heading), else the
// stylesheet's. A theme that is gone, and "auto", get the default theme's.
func (m *Manager) Look(id string) (Shape, TypeTokens) {
	id = strings.ToLower(id)
	if meta := m.meta(id); meta != nil && meta.Shape != nil && meta.Type != nil {
		return fillShape(*meta.Shape, meta.Radius), fillType(*meta.Type)
	}
	if m != nil {
		if t, err := m.Tokens(id); err == nil {
			return lookFromTokens(t)
		}
	}
	if id != "default" {
		return m.Look("default")
	}
	return ShapeFor(10), DefaultType()
}

// meta is a theme's theme.json, installed or built in.
func (m *Manager) meta(id string) *Meta {
	if m != nil {
		for _, meta := range m.List() {
			if meta.ID == id {
				return &meta
			}
		}
	}
	if !idRe.MatchString(id) {
		return nil
	}
	raw, err := fs.ReadFile(builtin.Builtin, id+"/theme.json")
	if err != nil {
		return nil
	}
	var meta Meta
	if json.Unmarshal(raw, &meta) != nil {
		return nil
	}
	return &meta
}

func lookFromTokens(t *Tokens) (Shape, TypeTokens) {
	r, ok := t.Lengths["radius"]
	if !ok {
		r = 10
	}
	s := ShapeFor(r)
	if v, ok := t.Lengths["focus-width"]; ok && v > 0 && v <= 8 {
		s.FocusWidth = v
	}
	if v, ok := t.Lengths["focus-offset"]; ok && v >= 0 && v <= 8 {
		s.FocusOffset = v
	}
	ty := DefaultType()
	if len(t.Fonts) > 0 {
		ty.Display, ty.Body = append([]string(nil), t.Fonts...), append([]string(nil), t.Fonts...)
	}
	if v := t.Other["font-mono"]; v != "" {
		if f := fontFamilies(v); len(f) > 0 {
			ty.Mono = f
		}
	}
	if v, ok := t.Lengths["weight-heading"]; ok && v >= 100 && v <= 900 {
		ty.Weights["heading"] = int(v)
	} else if n, err := strconv.Atoi(t.Other["weight-heading"]); err == nil && n >= 100 && n <= 900 {
		ty.Weights["heading"] = n
	}
	return s, ty
}

// fillShape completes a theme.json shape that leaves parts out.
func fillShape(s Shape, radius float64) Shape {
	if s.Radius == 0 {
		s.Radius = radius
	}
	d := ShapeFor(s.Radius)
	for _, p := range []struct {
		got *float64
		def float64
	}{
		{&s.Card, d.Card}, {&s.Button, d.Button}, {&s.Field, d.Field}, {&s.Row, d.Row}, {&s.Thumb, d.Thumb},
		{&s.Chip, d.Chip}, {&s.Badge, d.Badge}, {&s.Image, d.Image}, {&s.Sheet, d.Sheet}, {&s.Dialog, d.Dialog},
		{&s.Pill, d.Pill}, {&s.FocusWidth, d.FocusWidth}, {&s.FocusOffset, d.FocusOffset}, {&s.FocusScale, d.FocusScale},
	} {
		if *p.got == 0 && p.def != 0 {
			*p.got = p.def
		}
	}
	return s
}

func fillType(t TypeTokens) TypeTokens {
	d := DefaultType()
	if len(t.Body) == 0 {
		t.Body = d.Body
	}
	if len(t.Display) == 0 {
		t.Display = t.Body
	}
	if len(t.Mono) == 0 {
		t.Mono = d.Mono
	}
	w := map[string]int{}
	for k, v := range d.Weights {
		w[k] = v
	}
	for k, v := range t.Weights {
		if v >= 100 && v <= 900 {
			w[k] = v
		}
	}
	t.Weights = w
	return t
}
