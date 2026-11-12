// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import "sort"

// Every colour, typeface and corner a renderer draws comes from the theme: GET
// /api/v1/theme gives the colours (tokens, by scope), the corners (shape) and the
// typefaces and weights (type); the rendered document's layout.text gives the sizes.
// This table says which of them each part of each component and card style uses, so a
// renderer has no colour, type or shape of its own, and every app draws a component
// alike. It is in the catalogue as each component's and card style's "tokens".

// TokenRoles are the theme tokens a component or card style is drawn with.
type TokenRoles struct {
	// Colors maps each coloured part (a ColorRoles name: background, text, accent, ...)
	// to the colour token drawn there (bg, fg, accent, ...: the names in GET
	// /api/v1/theme tokens). A node's scope tokens override the colours, never the roles.
	Colors map[string]string `json:"colors"`
	// Type maps each piece of text to its text role: its size is layout.text[role] and
	// its family and weight are ThemeRoles.TypeRoles[role].
	Type map[string]string `json:"type,omitempty"`
	// Shape maps each rounded part to a shape token: a name in GET /api/v1/theme shape
	// (card, button, field, chip, badge, image, ...), whose value is its corner radius in
	// points; focusWidth and focusOffset are the focus ring's.
	Shape map[string]string `json:"shape,omitempty"`
}

// ThemeRoles is the vocabulary of TokenRoles, in the catalogue as "theme".
type ThemeRoles struct {
	// ColorRoles are the coloured parts a component can have.
	ColorRoles []EnumOption `json:"colorRoles"`
	// ColorTokens are the colour tokens a role may name: GET /api/v1/theme tokens.
	ColorTokens []string `json:"colorTokens"`
	// ShapeTokens are the names in GET /api/v1/theme shape.
	ShapeTokens []EnumOption `json:"shapeTokens"`
	// TypeRoles say the family (display or body: GET /api/v1/theme type) and weight
	// (a name in type.weights) of each text role; the size is layout.text[role].
	TypeRoles map[string]TypeRole `json:"typeRoles"`
}

// TypeRole is how one text role is set.
type TypeRole struct {
	Family string `json:"family"`
	Weight string `json:"weight"`
}

// ColorTokenNames are the colour tokens of every theme (the names in themes/*/theme.json
// "colors"); a test holds them to internal/themes.
var ColorTokenNames = []string{
	"accent", "accentFg", "accentHover", "accentLine", "accentSoft", "accentText", "bg", "bg2", "bg3", "bg4",
	"border", "borderStrong", "brandAccent", "controlLine", "danger", "dangerSoft", "fg", "fg2", "fg3", "focus",
	"hover", "ok", "okSoft", "onImage", "overlay", "pressed", "scrim", "warn", "warnSoft",
}

var colorRoles = []EnumOption{
	{"background", "What the component fills its own area with"},
	{"surface", "A raised area inside it, and the place of a picture that hasn't loaded"},
	{"text", "Its main text"},
	{"secondaryText", "Subtitles, facts, inactive tabs"},
	{"hintText", "Numbers, placeholders, hints"},
	{"link", "Links and highlighted words: the accent as text"},
	{"icon", "Icons beside text"},
	{"accent", "Filled things that stand out: a primary button, a selected tab's underline"},
	{"onAccent", "Text and icons on accent"},
	{"accentHover", "accent while hovered or pressed"},
	{"selected", "The tint behind a selected row or chip"},
	{"selectedBorder", "The outline of a selected chip"},
	{"selectedText", "The text of a selected tab, chip or row"},
	{"hover", "The tint over a hovered row (pointer devices)"},
	{"pressed", "The tint over a pressed row"},
	{"border", "Hairlines"},
	{"outline", "Outlines of secondary buttons and chips"},
	{"fieldOutline", "Outlines of text fields and other controls: 3:1 on every surface"},
	{"focus", "The focus ring"},
	{"onImage", "Text and icons over a picture"},
	{"scrim", "The plate behind text over a picture"},
	{"badge", "A badge's fill (over a picture)"},
	{"badgeText", "A badge's text"},
	{"progress", "A progress bar's filled part"},
	{"progressTrack", "A progress bar's empty part"},
	{"watched", "The watched mark's fill"},
	{"onWatched", "The watched mark's tick"},
	{"playing", "The track or item playing now"},
	{"info", "The information tone's bar and icon"},
	{"success", "The good-news tone's bar and icon"},
	{"warning", "The warning tone's bar and icon"},
	{"danger", "The problem tone's bar, icon, and a destructive button"},
	{"dangerTint", "The tint behind a hovered destructive button"},
	{"brand", "The server's name, its coloured parts (brandParts with accent)"},
}

var shapeTokens = []EnumOption{
	{"card", "Cards and their pictures"},
	{"button", "Buttons"},
	{"field", "Text fields, selects, search boxes"},
	{"row", "List rows"},
	{"thumb", "Small pictures in a row"},
	{"chip", "Chips and filter pills"},
	{"badge", "Badges and small labels"},
	{"image", "Pictures and banners placed on a page; the spotlight on a television"},
	{"sheet", "Menus, popovers and sheets"},
	{"dialog", "Dialogs"},
	{"pill", "Fully round whatever the size: a circle on a square picture"},
	{"focusWidth", "The focus ring's width"},
	{"focusOffset", "The gap between the focus ring and what it surrounds"},
}

// typeRoles: the layout.text roles.
var typeRoles = map[string]TypeRole{
	"display":   {"display", "heading"}, // revision 3: a spotlight's title; the old pageTitle size where a device has no bigger one
	"pageTitle": {"display", "heading"},
	"heading":   {"display", "heading"},
	"title":     {"body", "strong"},
	"body":      {"body", "body"},
	"caption":   {"body", "body"},
	"badge":     {"body", "strong"},
}

// Shared parts of the tables.
var (
	focusRing   = map[string]string{"focus": "focus"}
	focusShape  = map[string]string{"focus": "focusWidth", "focusGap": "focusOffset"}
	textColours = map[string]string{"text": "fg", "secondaryText": "fg2", "hintText": "fg3", "link": "accentText"}
	overImage   = map[string]string{"onImage": "onImage", "scrim": "scrim"}
	buttonParts = map[string]string{"accent": "accent", "onAccent": "accentFg", "accentHover": "accentHover",
		"surface": "bg3", "text": "fg", "outline": "borderStrong", "hover": "hover", "pressed": "pressed",
		"danger": "danger", "dangerTint": "dangerSoft"}
	progressBar = map[string]string{"progress": "accent", "progressTrack": "bg4"}
)

// join merges maps; later ones win.
func join(ms ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range ms {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// componentTokens is the table: component type → roles.
var componentTokens = map[string]TokenRoles{
	"Page": {Colors: map[string]string{"background": "bg", "text": "fg", "secondaryText": "fg2", "brand": "brandAccent"},
		Type: map[string]string{"title": "pageTitle"}},
	"Hero": {Colors: join(overImage, focusRing, map[string]string{"background": "bg", "surface": "bg3", "text": "onImage", "secondaryText": "onImage",
		"accent": "accent", "onAccent": "accentFg", "accentHover": "accentHover", "outline": "onImage", "badge": "scrim", "badgeText": "onImage"}),
		Type:  map[string]string{"title": "display", "facts": "caption", "synopsis": "body", "button": "body", "badge": "badge"},
		Shape: join(focusShape, map[string]string{"corner": "image", "button": "button", "badge": "badge", "pageDots": "pill"})},
	"Shelf": {Colors: join(focusRing, map[string]string{"text": "fg", "secondaryText": "fg2", "link": "accentText", "surface": "bg3"}),
		Type:  map[string]string{"title": "heading", "subtitle": "caption", "seeAll": "body"},
		Shape: join(focusShape, map[string]string{"scrollButton": "pill"})},
	"Grid": {Colors: join(focusRing, map[string]string{"text": "fg", "secondaryText": "fg2", "link": "accentText", "surface": "bg3"}),
		Type: map[string]string{"title": "heading", "subtitle": "caption", "seeAll": "body"}, Shape: focusShape},
	"Card": {Colors: join(focusRing, progressBar, map[string]string{"surface": "bg3", "text": "fg", "secondaryText": "fg2", "hintText": "fg3",
		"border": "border", "badge": "scrim", "badgeText": "onImage", "watched": "accent", "onWatched": "accentFg", "onImage": "onImage", "scrim": "scrim"}),
		Type:  map[string]string{"title": "title", "subtitle": "caption", "badge": "badge"},
		Shape: join(focusShape, map[string]string{"corner": "card", "badge": "badge", "progress": "pill"})},
	"Tabs": {Colors: join(focusRing, map[string]string{"text": "fg2", "selectedText": "fg", "accent": "accent", "border": "border",
		"selected": "accentSoft", "surface": "bg3", "hover": "hover"}),
		Type: map[string]string{"label": "title"}, Shape: join(focusShape, map[string]string{"segment": "chip"})},
	"CrossBar": {Colors: join(focusRing, map[string]string{"text": "fg2", "selectedText": "fg", "accent": "accent", "border": "border", "selected": "accentSoft", "surface": "bg3", "hover": "hover", "icon": "fg2"}),
		Type: map[string]string{"label": "title", "item": "heading"}, Shape: join(focusShape, map[string]string{"segment": "chip", "row": "row"})},
	"Carousel": {Colors: join(focusRing, map[string]string{"text": "fg", "secondaryText": "fg2", "link": "accentText", "surface": "bg3", "accent": "accent", "onAccent": "accentFg"}),
		Type: map[string]string{"title": "heading", "subtitle": "caption", "focused": "display"}, Shape: join(focusShape, map[string]string{"scrollButton": "pill", "button": "button"})},
	"WidgetGrid": {Colors: join(focusRing, map[string]string{"background": "bg", "text": "fg"}), Shape: focusShape},
	"Widget": {Colors: join(focusRing, map[string]string{"background": "bg2", "surface": "bg3", "border": "border", "text": "fg", "secondaryText": "fg2", "hintText": "fg3", "link": "accentText", "progress": "accent", "progressTrack": "bg4"}),
		Type: map[string]string{"title": "heading", "figure": "display", "text": "body", "caption": "caption"}, Shape: join(focusShape, map[string]string{"corner": "card"})},
	"NumberedRow": {Colors: join(focusRing, map[string]string{"text": "fg", "secondaryText": "fg2", "hintText": "fg3", "playing": "accentText", "border": "border", "hover": "hover", "selected": "accentSoft", "surface": "bg3"}),
		Type: map[string]string{"title": "heading", "row": "body", "subtitle": "caption", "number": "title"}, Shape: join(focusShape, map[string]string{"row": "row", "art": "thumb"})},
	"Tab": {Colors: join(focusRing, map[string]string{"text": "fg2", "selectedText": "fg", "accent": "accent", "selected": "accentSoft", "hover": "hover"}),
		Type: map[string]string{"label": "title"}, Shape: join(focusShape, map[string]string{"segment": "chip"})},
	"FilterBar": {Colors: join(focusRing, map[string]string{"surface": "bg3", "text": "fg", "secondaryText": "fg2", "hintText": "fg3",
		"fieldOutline": "controlLine", "outline": "borderStrong", "selected": "accentSoft", "selectedBorder": "accentLine", "selectedText": "fg", "hover": "hover"}),
		Type: map[string]string{"label": "body", "count": "caption"}, Shape: join(focusShape, map[string]string{"field": "field", "chip": "chip", "menu": "sheet"})},
	// The Dock is elevation 1 (a panel: tone bg2 and a hairline) with its buttons drawn as Buttons are.
	"Dock": {Colors: join(focusRing, buttonParts, map[string]string{"background": "bg2", "border": "border", "text": "fg", "secondaryText": "fg2",
		"selected": "accentSoft", "selectedText": "fg"}),
		Type: map[string]string{"label": "body"}, Shape: join(focusShape, map[string]string{"button": "button"})},
	"KeyLegend": {Colors: map[string]string{"text": "fg2", "outline": "borderStrong", "surface": "bg3"},
		Type: map[string]string{"label": "caption"}, Shape: map[string]string{"keycap": "badge"}},
	"DetailHeader": {Colors: join(overImage, focusRing, progressBar, buttonParts, map[string]string{"background": "bg", "text": "fg", "secondaryText": "fg2",
		"link": "accentText", "badge": "scrim", "badgeText": "onImage", "border": "borderStrong"}),
		Type:  map[string]string{"title": "pageTitle", "facts": "caption", "synopsis": "body", "button": "body", "badge": "badge"},
		Shape: join(focusShape, map[string]string{"poster": "card", "button": "button", "badge": "badge", "progress": "pill"})},
	"SeasonList": {Colors: join(focusRing, progressBar, map[string]string{"text": "fg", "secondaryText": "fg2", "hintText": "fg3", "outline": "borderStrong",
		"selected": "accentSoft", "selectedBorder": "accentLine", "selectedText": "fg", "surface": "bg3", "watched": "accent", "onWatched": "accentFg", "hover": "hover"}),
		Type:  map[string]string{"season": "body", "title": "title", "synopsis": "caption"},
		Shape: join(focusShape, map[string]string{"season": "chip", "still": "card", "row": "row"})},
	"TrackList": {Colors: join(focusRing, map[string]string{"text": "fg", "secondaryText": "fg2", "hintText": "fg3", "playing": "accentText",
		"border": "border", "hover": "hover", "pressed": "pressed", "selected": "accentSoft"}),
		Type: map[string]string{"title": "body", "artist": "caption", "number": "caption", "length": "caption"}, Shape: join(focusShape, map[string]string{"row": "row"})},
	"Credits": {Colors: join(focusRing, map[string]string{"text": "fg", "secondaryText": "fg2", "hintText": "fg3", "link": "accentText",
		"surface": "bg3", "border": "border", "hover": "hover", "pressed": "pressed"}),
		Type:  map[string]string{"title": "heading", "department": "title", "name": "body", "part": "caption", "showAll": "body"},
		Shape: join(focusShape, map[string]string{"portrait": "pill", "chip": "chip", "scrollButton": "pill"})},
	"Track": {Colors: join(focusRing, map[string]string{"text": "fg", "secondaryText": "fg2", "hintText": "fg3", "playing": "accentText",
		"hover": "hover", "pressed": "pressed", "selected": "accentSoft"}),
		Type: map[string]string{"title": "body", "artist": "caption", "number": "caption", "length": "caption"}, Shape: join(focusShape, map[string]string{"row": "row"})},
	"Banner": {Colors: join(overImage, focusRing, buttonParts, map[string]string{"background": "bg3", "text": "fg", "secondaryText": "fg2", "border": "border"}),
		Type:  map[string]string{"title": "heading", "text": "body", "button": "body"},
		Shape: join(focusShape, map[string]string{"corner": "image", "thumb": "card", "button": "button"})},
	"Text": {Colors: join(textColours, map[string]string{"surface": "bg3"}),
		Type:  map[string]string{"pageTitle": "pageTitle", "heading": "heading", "title": "title", "body": "body", "caption": "caption"},
		Shape: map[string]string{"code": "badge"}},
	"Notice": {Colors: join(focusRing, buttonParts, map[string]string{"background": "bg2", "text": "fg", "secondaryText": "fg2", "icon": "fg2",
		"info": "accentText", "success": "ok", "warning": "warn", "danger": "danger", "border": "border"}),
		Type:  map[string]string{"title": "title", "text": "body", "button": "body"},
		Shape: join(focusShape, map[string]string{"corner": "card", "button": "button"})},
	"Divider": {Colors: map[string]string{"border": "border"}},
	"Spacer":  {Colors: map[string]string{}},
	"Stack":   {Colors: map[string]string{"text": "fg", "secondaryText": "fg2"}, Type: map[string]string{"title": "heading"}},
	"Split":   {Colors: map[string]string{}},
	"Button": {Colors: join(focusRing, buttonParts),
		Type: map[string]string{"label": "body"}, Shape: join(focusShape, map[string]string{"corner": "button", "iconOnly": "pill"})},
	"Image": {Colors: join(focusRing, map[string]string{"surface": "bg3"}),
		Shape: join(focusShape, map[string]string{"corner": "image"})},
	"SearchBox": {Colors: join(focusRing, map[string]string{"surface": "bg3", "text": "fg", "hintText": "fg3", "icon": "fg2", "fieldOutline": "controlLine"}),
		Type: map[string]string{"text": "body"}, Shape: join(focusShape, map[string]string{"corner": "field"})},
	"List": {Colors: join(focusRing, map[string]string{"text": "fg", "secondaryText": "fg2", "border": "border"}),
		Type: map[string]string{"title": "heading"}, Shape: focusShape},
	"ListItem": {Colors: join(focusRing, map[string]string{"text": "fg", "secondaryText": "fg2", "hintText": "fg3", "icon": "fg2", "surface": "bg3",
		"hover": "hover", "pressed": "pressed", "selected": "accentSoft", "selectedText": "fg"}),
		Type:  map[string]string{"title": "body", "subtitle": "caption", "trailing": "caption"},
		Shape: join(focusShape, map[string]string{"corner": "row", "art": "thumb", "icon": "row"})},
	"NativeView": {Colors: join(focusRing, map[string]string{"background": "bg", "surface": "bg2", "text": "fg", "secondaryText": "fg2", "outline": "borderStrong"}),
		Type: map[string]string{"text": "body"}, Shape: join(focusShape, map[string]string{"corner": "image"})},
}

// cardStyleTokens is the table for card styles: the Card's own roles with each style's
// differences.
func cardStyleTokens(style, shape string) TokenRoles {
	base := componentTokens["Card"]
	t := TokenRoles{Colors: join(base.Colors), Type: join(base.Type), Shape: join(base.Shape)}
	switch shape {
	case "circle":
		t.Shape["corner"] = "pill"
	case "chip":
		t.Colors = join(focusRing, map[string]string{"text": "fg2", "outline": "borderStrong", "hover": "hover",
			"selected": "accentSoft", "selectedBorder": "accentLine", "selectedText": "fg"})
		t.Type = map[string]string{"title": "body"}
		t.Shape = join(focusShape, map[string]string{"corner": "chip"})
	}
	switch style {
	case "tile":
		// The name sits over the picture, or over a plain tile with an icon.
		t.Colors = join(t.Colors, overImage, map[string]string{"text": "onImage", "icon": "fg2"})
		t.Type["title"] = "heading"
	case "plate":
		// The title sits over the picture, on a scrim.
		t.Colors = join(t.Colors, overImage, map[string]string{"text": "onImage"})
	case "block":
		t.Type = map[string]string{"title": "title", "subtitle": "caption"}
	case "logo", "channel":
		t.Colors = join(t.Colors, map[string]string{"hintText": "fg2", "surface": "bg3"})
		t.Type["number"], t.Type["now"], t.Type["next"] = "caption", "caption", "caption"
	case "episode":
		t.Type["synopsis"] = "caption"
	case "photo":
		t.Type = nil
	}
	return t
}

// themeRoles is the catalogue's "theme".
func themeRoles() *ThemeRoles {
	tr := &ThemeRoles{ColorRoles: append([]EnumOption(nil), colorRoles...), ColorTokens: append([]string(nil), ColorTokenNames...),
		ShapeTokens: append([]EnumOption(nil), shapeTokens...), TypeRoles: map[string]TypeRole{}}
	for k, v := range typeRoles {
		tr.TypeRoles[k] = v
	}
	sort.Strings(tr.ColorTokens)
	return tr
}

// withTokens fills in each component's and card style's tokens and the catalogue's
// theme vocabulary. Components a module adds take those of the component they expand
// into (moduleComponentSpec).
func withTokens(c *Catalogue) *Catalogue {
	for i := range c.Components {
		if t, ok := componentTokens[c.Components[i].Type]; ok {
			cp := TokenRoles{Colors: join(t.Colors)}
			if len(t.Type) > 0 {
				cp.Type = join(t.Type)
			}
			if len(t.Shape) > 0 {
				cp.Shape = join(t.Shape)
			}
			c.Components[i].Tokens = &cp
		}
	}
	for i := range c.CardStyles {
		t := cardStyleTokens(c.CardStyles[i].Style, c.CardStyles[i].Shape)
		c.CardStyles[i].Tokens = &t
	}
	c.Theme = themeRoles()
	return c
}
