// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// CatalogueMarkdown writes the reference part of docs/SDUI.md from the
// catalogue: every component, card style, data source, condition, page kind,
// route and device profile. A test keeps the document equal to this, so the
// reference can't say something the code doesn't.
func CatalogueMarkdown() string {
	c := Catalog()
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	cell := func(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "|", "\\|"), "\n", " ") }
	def := func(v any) string {
		switch x := v.(type) {
		case nil:
			return ""
		case string:
			return "`" + x + "`"
		case []string:
			return "`" + strings.Join(x, ", ") + "`"
		}
		return fmt.Sprintf("`%v`", v)
	}
	propType := func(p *PropSpec) string {
		t := p.Type
		switch {
		case len(p.Options) > 0 && len(p.Options) <= 16:
			t += ": " + strings.Join(values(p.Options), ", ")
		case p.Min != nil && p.Max != nil:
			t += fmt.Sprintf(" %d to %d", *p.Min, *p.Max)
		case p.MaxLength > 0:
			t += fmt.Sprintf(", at most %d", p.MaxLength)
		}
		return t
	}
	props := func(list []PropSpec) {
		if len(list) == 0 {
			return
		}
		w("| Property | Type | Default | What it is |\n|---|---|---|---|\n")
		for i := range list {
			p := &list[i]
			name := "`" + p.Name + "`"
			if p.Required {
				name += " (required)"
			}
			help := p.Label
			if p.Help != "" {
				help += ". " + p.Help
			}
			if len(p.FormFactors) > 0 {
				help += " Only on " + strings.Join(p.FormFactors, ", ") + "."
			}
			w("| %s | %s | %s | %s |\n", name, cell(propType(p)), def(p.Default), cell(help))
		}
		w("\n")
	}
	pretty := func(raw json.RawMessage) string {
		var buf bytes.Buffer
		if json.Indent(&buf, raw, "", "  ") != nil {
			return string(raw)
		}
		return buf.String()
	}
	renders := func(m map[string]string, order []string) {
		for _, k := range order {
			if v := m[k]; v != "" {
				label := map[string]string{"all": "Everywhere", "tv": "Television", "phone": "Phone", "tablet": "Tablet", "desktop": "Desktop and web"}[k]
				w("- **%s.** %s\n", label, v)
			}
		}
	}

	w("### Components\n\n")
	w("%d components. **Required** ones are drawn by every renderer; the others may be left out by a renderer, which then names the ones it has (`components=`), and is sent the fallback instead.\n\n", len(c.Components))
	w("| Component | Label | Category | Required | Takes a source | Focus | Fallback |\n|---|---|---|---|---|---|---|\n")
	for _, comp := range c.Components {
		req, fb := "", comp.Fallback
		if comp.Required {
			req = "yes"
		} else if fb == "" {
			fb = "left out"
		}
		w("| [`%s`](#%s) | %s | %s | %s | %s | %s | %s |\n", comp.Type, strings.ToLower(comp.Type), comp.Label, comp.Category, req, comp.Data, comp.Focus, fb)
	}
	w("\n")
	for _, comp := range c.Components {
		w("#### %s\n\n", comp.Type)
		w("*%s.* %s", comp.Label, comp.Description)
		if !comp.Authored {
			w(" Made by the server; not something an administrator places.")
		}
		w("\n\n")
		if comp.Container {
			kids := "any component"
			if len(comp.Children) > 0 {
				kids = "`" + strings.Join(comp.Children, "`, `") + "`"
			}
			w("Holds: %s.", kids)
			w("\n\n")
		}
		if len(comp.Accepts) > 0 {
			w("Sources: `%s` only.\n\n", strings.Join(comp.Accepts, "`, `"))
		}
		props(comp.Props)
		if len(comp.Renders) > 0 {
			w("How it is drawn:\n\n")
			renders(comp.Renders, []string{"all", "tv", "phone", "tablet", "desktop"})
			w("\n")
		}
		if len(comp.States) > 0 {
			w("States:\n\n")
			keys := make([]string, 0, len(comp.States))
			for k := range comp.States {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				w("- **%s.** %s\n", k, comp.States[k])
			}
			w("\n")
		}
		w("Focus: %s. ", map[string]string{"none": "takes none", "item": "the node takes focus when it can be acted on", "group": "its children take focus, and it is their group", "stop": "one stop in the page's focus order; it moves focus among its own parts"}[comp.Focus])
		w("Accessibility: %s\n\n", comp.A11y)
		if !comp.Required {
			w("Fallback: %s\n\n", comp.FallbackNote)
		}
		if comp.Example != nil {
			w("```json\n%s\n```\n\n", pretty(comp.Example))
		}
	}

	w("### Card styles\n\n")
	w("A `Card` has a `style` and a `shape`. A renderer that doesn't know a style draws its shape: the picture, with the title and subtitle under it. `Shows` is what the style displays of what the card carries, in reading order, which is also the order of its accessibility label.\n\n")
	w("| Style | Shape | Aspect | Picture, in order of preference | Shows | Title lines | Subtitle lines | For |\n|---|---|---|---|---|---|---|---|\n")
	for _, s := range c.CardStyles {
		w("| `%s` | %s | %s | %s | %s | %d | %d | %s |\n", s.Style, s.Shape, s.Aspect, strings.Join(s.Image, ", "), strings.Join(s.Shows, ", "), s.TitleLines, s.SubtitleLines, cell(s.Description))
	}
	w("\nThe hero's slides are cards of style `hero`: shape landscape, a 16/9 backdrop the hero's width, with `facts`, `synopsis`, `logo` and `actions`.\n\n")

	w("### Card sizes\n\n")
	w("The width of a medium (`m`) card in points; small is 80%% and large 125%%, rounded down to an even number. The picture's height follows from the aspect. A television's grid has a fixed number of columns for medium cards (one more for small, one fewer for large).\n\n")
	w("| Style | tv | tv columns | phone | tablet | desktop and web |\n|---|---|---|---|---|---|\n")
	for _, s := range c.CardStyles {
		if s.Shape == "chip" {
			continue
		}
		w("| `%s` | %d | %d | %d | %d | %d |\n", s.Style, c.Profiles["tv"].Cards[s.Style].Width["m"], c.Profiles["tv"].Cards[s.Style].Columns,
			c.Profiles["phone"].Cards[s.Style].Width["m"], c.Profiles["tablet"].Cards[s.Style].Width["m"], c.Profiles["desktop"].Cards[s.Style].Width["m"])
	}
	w("\n")

	b.WriteString(tokensMarkdown(c))
	w("### Device profiles\n\n")
	w("| | tv (hd) | tv (4k) | phone | tablet | desktop | web |\n|---|---|---|---|---|---|---|\n")
	order := []string{"tv", "tv-4k", "phone", "tablet", "desktop", "web"}
	row := func(label string, f func(l *Layout) string) {
		w("| %s |", label)
		for _, k := range order {
			w(" %s |", f(c.Profiles[k]))
		}
		w("\n")
	}
	num := func(n int) string { return fmt.Sprint(n) }
	row("Scale (pixels per point)", func(l *Layout) string { return num(l.Scale) })
	row("Canvas", func(l *Layout) string {
		if l.Canvas.Width == 0 {
			return "the window"
		}
		return fmt.Sprintf("%d×%d", l.Canvas.Width, l.Canvas.Height)
	})
	row("Breakpoint when the window isn't known", func(l *Layout) string { return l.Breakpoint })
	row("Safe area (top, right, bottom, left)", func(l *Layout) string {
		if l.SafeArea == (Insets{}) {
			return "the system's"
		}
		return fmt.Sprintf("%d, %d, %d, %d", l.SafeArea.Top, l.SafeArea.Right, l.SafeArea.Bottom, l.SafeArea.Left)
	})
	row("Focus is the only pointer", func(l *Layout) string { return map[bool]string{true: "yes", false: "no"}[l.FocusEngine] })
	row("Opens web links", func(l *Layout) string { return map[bool]string{true: "yes", false: "no"}[l.ExternalLinks] })
	row("Spacing xs, s, m, l, xl", func(l *Layout) string {
		return fmt.Sprintf("%d, %d, %d, %d, %d", l.Spacing["xs"], l.Spacing["s"], l.Spacing["m"], l.Spacing["l"], l.Spacing["xl"])
	})
	row("Text pageTitle, heading, title, body, caption", func(l *Layout) string {
		return fmt.Sprintf("%d, %d, %d, %d, %d", l.Text["pageTitle"], l.Text["heading"], l.Text["title"], l.Text["body"], l.Text["caption"])
	})
	row("Hero", func(l *Layout) string { return fmt.Sprintf("%d×%d", l.Hero.Width, l.Hero.Height) })
	row("Most sections a page", func(l *Layout) string { return num(l.Budget.MaxSections) })
	row("Sections that arrive filled", func(l *Layout) string { return num(l.Budget.EagerSections) })
	row("Cards in a row's first page", func(l *Layout) string { return num(l.Budget.ShelfItems) })
	row("Cards in a grid's first page", func(l *Layout) string { return num(l.Budget.GridItems) })
	row("Rows in a list's first page", func(l *Layout) string { return num(l.Budget.ListItems) })
	row("Cards in each page after", func(l *Layout) string { return num(l.Budget.PageSize) })
	row("Titles in a hero", func(l *Layout) string { return num(l.Budget.HeroItems) })
	row("Copies of a repeated section", func(l *Layout) string { return num(l.Budget.Repeats) })
	row("Characters: title, subtitle, synopsis, overview", func(l *Layout) string {
		return fmt.Sprintf("%d, %d, %d, %d", l.TextLimits.Title, l.TextLimits.Subtitle, l.TextLimits.Synopsis, l.TextLimits.Overview)
	})
	w("\nPictures are asked for at one of these widths, in pixels: %s.\n\n", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(c.Profiles["tv"].ImageWidths)), ", "), "[]"))

	w("### Data sources\n\n")
	w("| Source | Label | Category | Gives | Usual card style | Pages by cursor | On pages |\n|---|---|---|---|---|---|---|\n")
	for _, s := range c.Sources {
		on := "any"
		if len(s.PageKinds) > 0 {
			on = strings.Join(s.PageKinds, ", ")
		}
		w("| [`%s`](#%s) | %s | %s | %s | %s | %s | %s |\n", s.Type, strings.ToLower(s.Type), s.Label, s.Category, strings.Join(s.Yields, ", "), s.CardStyle, map[bool]string{true: "yes", false: ""}[s.Paged], on)
	}
	w("\n")
	for _, s := range c.Sources {
		w("#### %s\n\n*%s.* %s", s.Type, s.Label, s.Description)
		if s.SeeAll != "" {
			w(" See all opens `%s`.", s.SeeAll)
		}
		w("\n\n")
		props(s.Params)
		w("```json\n%s\n```\n\n", string(s.Example))
	}
	w("Every source also takes `map`: `{\"subtitle\": \"studio\"}` puts another field of the item into a card's `title`, `subtitle` or `synopsis`. The fields are %s.\n\n", "`"+strings.Join(MapFields, "`, `")+"`")

	w("### Conditions\n\n")
	w("| Field | Type | True when |\n|---|---|---|\n")
	w("| `all` | list of conditions | every one holds |\n| `any` | list of conditions | at least one holds |\n| `not` | condition | it doesn't hold; `{\"not\": {}}` never holds |\n")
	for _, cs := range c.Conditions {
		help := cs.Label
		if cs.Help != "" {
			help += ". " + cs.Help
		}
		w("| `%s` | %s | %s |\n", cs.Field, cell(propType(&cs.PropSpec)), cell(help))
	}
	w("\n")

	w("### Page kinds\n\n")
	w("| Kind | What it is | Route | Query parameters | Variables for sources | Narrowed by |\n|---|---|---|---|---|---|\n")
	for _, k := range c.PageKinds {
		w("| `%s` | %s | `%s` | %s | %s | %s |\n", k.Kind, cell(k.Description), k.Route, strings.Join(k.Params, ", "), strings.Join(k.Variables, ", "), strings.Join(k.Match, ", "))
	}
	w("\n")

	w("### Routes\n\n")
	w("What shows each place in the app: an SDUI page, asked for with the parameters given (`{name}` is from the route, `?name` from its query), or a screen the app draws itself.\n\n")
	w("| Route | Shown by | Parameters |\n|---|---|---|\n")
	for _, r := range c.Routes {
		by := "native `" + r.Native + "`"
		if r.Page != "" {
			by = "page `" + r.Page + "`"
		}
		keys := make([]string, 0, len(r.Params))
		for k, v := range r.Params {
			keys = append(keys, k+"="+v)
		}
		sort.Strings(keys)
		w("| `%s` | %s | %s |\n", r.Pattern, by, strings.Join(keys, ", "))
	}
	w("\n")

	w("### Native screens\n\n")
	w("| Screen | What it is | Route |\n|---|---|---|\n")
	for _, n := range c.NativeScreens {
		w("| `%s` | %s | %s |\n", n.Screen, cell(n.Description), def(n.Route))
	}
	w("\n")

	w("### Commands\n\n")
	w("| `op` | What it does | Request |\n|---|---|---|\n")
	for _, cmd := range c.Commands {
		reqLine := ""
		if cmd.Path != "" {
			reqLine = "`" + cmd.Method + " " + cmd.Path + "`"
			if cmd.Body != "" {
				reqLine += " with `" + cmd.Body + "`"
			}
		}
		w("| `%s` | %s | %s |\n", cmd.Op, cell(cmd.Description), reqLine)
	}
	w("\n")

	w("### Layout options\n\nThe menu's style on each form factor, and what the server changes about every page for the layout. Menu styles: `%s`.\n\n", strings.Join(c.NavStyles, "`, `"))
	w("| Layout | What it is | TV | Phone | Tablet | Desktop | Web | Pages |\n|---|---|---|---|---|---|---|---|\n")
	for _, l := range c.Layouts {
		var pages []string
		if l.Pages.GridsAsRows {
			pages = append(pages, "grids as rows")
		}
		if l.Pages.RowsAsLists {
			pages = append(pages, "rows and grids as lists")
		}
		if l.Pages.Hero != "" {
			pages = append(pages, "spotlight: "+l.Pages.Hero)
		}
		if l.Pages.CardSize != "" {
			pages = append(pages, "cards "+l.Pages.CardSize)
		}
		if l.Pages.LandscapeCards {
			pages = append(pages, "wide cards for films and shows")
		}
		if l.Pages.Density != "" {
			pages = append(pages, l.Pages.Density)
		}
		if l.Overlay {
			pages = append(pages, "menu over the page")
		}
		if len(pages) == 0 {
			pages = []string{"as written"}
		}
		w("| `%s` | **%s.** %s | %s | %s | %s | %s | %s | %s |\n", l.ID, l.Label, cell(l.Description), l.Nav[FormTV], l.Nav[FormPhone], l.Nav[FormTablet], l.Nav[FormDesktop], l.Nav[FormWeb], strings.Join(pages, ", "))
	}
	w("\n")

	w("### Icons\n\n%s\n\n", "`"+strings.Join(c.Icons, "`, `")+"`")

	w("### Placeholders\n\n| Placeholder | Becomes |\n|---|---|\n")
	for _, p := range c.Placeholders {
		w("| `%s` | %s |\n", p.Value, p.Label)
	}
	w("\n### Limits\n\n| Limit | Value |\n|---|---|\n")
	keys := make([]string, 0, len(c.Limits))
	for k := range c.Limits {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		w("| `%s` | %d |\n", k, c.Limits[k])
	}
	w("\n### Built-in pages\n\n| Page | Kind | For | Sections |\n|---|---|---|---|\n")
	for _, p := range StockPages() {
		forWhat := "every " + p.Kind + " page"
		if m := p.Match; m != nil {
			forWhat = strings.TrimSpace(m.LibraryKind + " " + m.MediaKind)
		}
		var secs []string
		var walk func(list []Section)
		walk = func(list []Section) {
			for _, s := range list {
				name := s.Component
				if t := s.Source.Type(); t != "" {
					name += " of " + t
				}
				if s.Repeat != nil {
					name += ", per " + strings.TrimSuffix(s.Repeat.Over, "s")
				}
				secs = append(secs, "`"+s.ID+"` ("+name+")")
				walk(s.Children)
			}
		}
		walk(p.Sections)
		w("| `%s` | %s | %s | %s |\n", p.ID, p.Kind, forWhat, strings.Join(secs, ", "))
	}
	w("\n### Presets\n\n| Preset | Kind | What it is |\n|---|---|---|\n")
	for _, p := range Presets() {
		w("| `%s` | %s | **%s.** %s |\n", p.ID, p.Kind, p.Label, cell(p.Description))
	}
	w("\n")
	return b.String()
}
