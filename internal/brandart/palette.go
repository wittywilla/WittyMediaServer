// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package brandart

import (
	"encoding/json"

	"github.com/wittywilla/wittymediaserver/themes"
)

// Palette is the family's colours, read from the design tokens (themes/*/theme.json, written
// by scripts/design-tokens.py from the CSS), so the icons change when the house theme does.
type Palette struct {
	Night      Colour // the house theme's background
	Slate      Colour // its raised surface
	Raised     Colour // the surface above that
	Accent     Colour // violet
	AccentLite Colour // the accent as text: lighter violet
	AccentDeep Colour // the accent pressed: deeper violet
	Mint       Colour // "ok"
	Amber      Colour // "warn"
	Coral      Colour // "danger"
	Sky        Colour // the Midnight theme's accent text
	Cream      Colour // the light theme's background: the faces
	Ink        Colour // deepest: the features on the faces
	Paper      Colour // the light theme's background, for light-mode pictures
	Text       Colour // the house theme's text
}

type themeColours struct {
	Colors map[string]string `json:"colors"`
}

func themeColour(id, name string) Colour {
	data, err := themes.Builtin.ReadFile(id + "/theme.json")
	if err != nil {
		panic("brandart: no built-in theme " + id)
	}
	var t themeColours
	if err := json.Unmarshal(data, &t); err != nil {
		panic(err)
	}
	v, ok := t.Colors[name]
	if !ok {
		panic("brandart: theme " + id + " has no colour " + name)
	}
	return Hex(v)
}

// HousePalette is the palette from the built-in themes.
func HousePalette() Palette {
	accent := themeColour("default", "accent")
	paper := themeColour("light", "bg")
	return Palette{
		Night:      themeColour("default", "bg"),
		Slate:      themeColour("default", "bg3"),
		Raised:     themeColour("default", "bg4"),
		Accent:     accent,
		AccentLite: themeColour("default", "accentText"),
		AccentDeep: Mix(themeColour("default", "accentHover"), themeColour("default", "bg"), 0.28),
		Mint:       themeColour("default", "ok"),
		Amber:      themeColour("default", "warn"),
		Coral:      themeColour("default", "danger"),
		Sky:        themeColour("midnight", "accentText"),
		Cream:      Mix(paper, Hex("#ffffff"), 0.55),
		Ink:        Mix(themeColour("default", "bg"), accent, 0.18),
		Paper:      paper,
		Text:       themeColour("default", "fg"),
	}
}
