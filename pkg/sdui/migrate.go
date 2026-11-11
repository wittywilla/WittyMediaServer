// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

// Migration from the first, unreleased, format: screens written as trees of
// low-level nodes (Screen, Stack, PosterRow, Card, ...) with bindings to data
// paths. Development servers wrote every stock screen into ui_config.json on
// first start, so most such files hold nothing an administrator made.
//
// A screen that is still exactly a stock screen of that format is dropped:
// the built-in page takes its place and keeps improving with the server. A
// screen someone changed, or added, is converted node by node. What the new
// model has no place for is left out and named in the page's description, so
// nothing disappears silently.

// legacyStock are the SHA-256 of each stock screen of the first format, as
// canonical JSON (keys sorted, no spaces).
var legacyStock = map[string]string{
	"detail":    "eff7ce738e22feabbfdf46ec9d4eb1398e93e496d521f4745f4ddad994658410",
	"home":      "2d9e3a95af07ea08bd4448a9551f5c7a5b79f089afe0b897e508cd11b1b0e956",
	"libraries": "5237070b8f9959c00307bd64093948ad40fee6372fd6028733499024aadc65dd",
	"library":   "d76dc3ddc7a09c6324e8546776dbf9e120ac3347955239eb5f0a9d941037a8ba",
	"livetv":    "4ec327c5540478ddf4a5746147a95880ed2b3e63e777938421a2e2601b7dc234",
	"music":     "5ed62e4c0d66fcd0eaef609274cb2673fbaa51b3e0f18806fd1c1f44309c3409",
	"player":    "71e2ba69521aec67a21e057290796ebc5e0a0f4cb8295e78518b9ed0a010f80e",
	"search":    "dce47601df193d02cb6d17b3905e29c3b52da6fd1aab212647d30f7b2c6ff8ac",
	"settings":  "2c77b8a562eadc54cb8c6da094d1060790997cbec96a18761f7d8c51e6eb2763",
}

// legacySources maps the data paths the first format bound rows to onto data
// sources.
var legacySources = map[string]Source{
	"$.home.continueWatching":   {"type": "continueWatching", "media": "all"},
	"$.home.nextUp":             {"type": "nextUp"},
	"$.home.featured":           {"type": "featured"},
	"$.home.recentlyAdded":      {"type": "recentlyAdded"},
	"$.home.recommended":        {"type": "recommended"},
	"$.home.games":              {"type": "continuePlaying"},
	"$.home.channels":           {"type": "liveNow"},
	"$.home.music":              {"type": "recentlyAdded", "kind": "music"},
	"$.libraries.items":         {"type": "libraries"},
	"$.library.items":           {"type": "recentlyAdded", "library": "$library"},
	"$.item.children":           {"type": "episodes", "show": "$show"},
	"$.item.related":            {"type": "similar", "item": "$item"},
	"$.livetv.channels":         {"type": "liveNow"},
	"$.livetv.favourites":       {"type": "liveNow", "favourites": true},
	"$.music.continueListening": {"type": "continueWatching", "media": "listen"},
}

// legacyTexts maps bound text onto placeholders.
var legacyTexts = map[string]string{
	"$.user.name": "{user}", "$.server.name": "{server}", "$.library.title": "{library}", "$.item.title": "{title}", "$.search.query": "{query}",
}

// LegacyResult is what migrating one screen of the first format gave.
type LegacyResult struct {
	// Page is the converted page; nil when the screen was dropped.
	Page *Page
	// Stock is true when the screen was an unchanged stock screen, so the
	// built-in page replaces it. Dropped names why else it was dropped.
	Stock   bool
	Dropped string
	// Notes say what could not be carried over.
	Notes []string
}

// IsLegacyStock reports whether raw is, exactly, a stock screen of the first
// format.
func IsLegacyStock(name string, raw json.RawMessage) bool {
	var g any
	if json.Unmarshal(raw, &g) != nil {
		return false
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if enc.Encode(g) != nil {
		return false
	}
	sum := sha256.Sum256(bytes.TrimRight(buf.Bytes(), "\n"))
	return legacyStock[name] == hex.EncodeToString(sum[:])
}

type legacyNode struct {
	Type       string                    `json:"type"`
	ID         string                    `json:"id"`
	Props      map[string]any            `json:"props"`
	Style      map[string]any            `json:"style"`
	Children   []legacyNode              `json:"children"`
	Visibility *legacyCond               `json:"visibility"`
	Bindings   map[string]map[string]any `json:"bindings"`
	Action     string                    `json:"action"`
}

type legacyCond struct {
	All        []legacyCond `json:"all"`
	Any        []legacyCond `json:"any"`
	Not        *legacyCond  `json:"not"`
	Feature    string       `json:"feature"`
	Role       string       `json:"role"`
	Platform   string       `json:"platform"`
	FormFactor string       `json:"formFactor"`
	Module     string       `json:"module"`
	MinWidth   int          `json:"minWidth"`
	MaxWidth   int          `json:"maxWidth"`
	Locale     string       `json:"locale"`
	Flag       string       `json:"flag"`
}

type legacyDoc struct {
	Screen  string                    `json:"screen"`
	Root    legacyNode                `json:"root"`
	Actions map[string]map[string]any `json:"actions"`
	Meta    map[string]any            `json:"meta"`
}

type legacy struct {
	doc   legacyDoc
	kind  string
	notes map[string]bool
	ids   map[string]bool
	head  bool // a DetailHeader has been placed
}

// MigrateLegacyScreen converts one screen of the first format into a page.
func MigrateLegacyScreen(name string, raw json.RawMessage) LegacyResult {
	if IsLegacyStock(name, raw) {
		return LegacyResult{Stock: true}
	}
	m := &legacy{notes: map[string]bool{}, ids: map[string]bool{}}
	if err := json.Unmarshal(raw, &m.doc); err != nil {
		return LegacyResult{Dropped: "it could not be read: " + err.Error()}
	}
	switch name {
	case "player", "settings":
		return LegacyResult{Dropped: "the " + name + " is the app's own screen now, not a page"}
	case "home", "libraries", "library", "detail", "search", "livetv", "music":
		m.kind = name
	default:
		m.kind = "custom"
	}
	id := strings.ToLower(name)
	if !ValidID(id) {
		id = "page-" + safeID(id)
		id = strings.ToLower(id)
	}
	title, _ := m.doc.Root.Props["title"].(string)
	if title == "" {
		title, _ = m.doc.Meta["title"].(string)
	}
	if title == "" {
		title = "Page"
		if name != "" {
			title = strings.ToUpper(name[:1]) + name[1:]
		}
	}
	title = Truncate(title, 120)
	p := &Page{ID: id, Kind: m.kind, Title: title, Sections: m.children(m.doc.Root.Children, "")}
	var notes []string
	for n := range m.notes {
		notes = append(notes, n)
	}
	sort.Strings(notes)
	p.Description = "Converted from the earlier screen format."
	if len(notes) > 0 {
		p.Description += " Not carried over: " + strings.Join(notes, "; ") + "."
	}
	if len(p.Description) > 500 {
		p.Description = p.Description[:497] + "..."
	}
	// What still doesn't validate is taken out section by section, so one bad
	// node doesn't lose the page.
	for range 50 {
		issues := ValidatePage(p)
		if !HasErrors(issues) {
			break
		}
		if !m.prune(p, issues) {
			return LegacyResult{Dropped: "it could not be converted: " + AsError(issues).Error(), Notes: notes}
		}
		notes = append(notes, "a section the new format refuses")
	}
	if len(p.Sections) == 0 {
		return LegacyResult{Dropped: "nothing in it has a place in the new format", Notes: notes}
	}
	cp := ClonePage(*p)
	return LegacyResult{Page: &cp, Notes: notes}
}

// prune removes the first top-level section an error points into.
func (m *legacy) prune(p *Page, issues []Issue) bool {
	for _, is := range issues {
		if is.Severity == "warning" || !strings.HasPrefix(is.Path, "/sections/") {
			continue
		}
		rest := strings.TrimPrefix(is.Path, "/sections/")
		num, _, _ := strings.Cut(rest, "/")
		i := Source{"n": num}.Int("n")
		if i < 0 || i >= len(p.Sections) {
			return false
		}
		p.Sections = append(p.Sections[:i], p.Sections[i+1:]...)
		return true
	}
	return false
}

func (m *legacy) id(want string) string {
	id := strings.ToLower(safeID(want))
	id = strings.ReplaceAll(id, ".", "-")
	if !ValidID(id) {
		id = "s-" + strings.TrimLeft(id, "-_0123456789")
		if !ValidID(id) {
			id = "s"
		}
	}
	base := id
	for n := 2; m.ids[id]; n++ {
		id = base + "-" + Source{"n": n}.Str("n")
	}
	m.ids[id] = true
	return id
}

func (m *legacy) children(nodes []legacyNode, parent string) []Section {
	var out []Section
	for i := range nodes {
		out = append(out, m.node(&nodes[i], parent)...)
	}
	return out
}

func bindingPath(n *legacyNode, prop string) string {
	p, _ := n.Bindings[prop]["path"].(string)
	return p
}

func legacyStyle(shape string) string {
	switch shape {
	case "wide":
		return "landscape"
	case "square":
		return "square"
	case "poster":
		return "poster"
	}
	return ""
}

// step turns a distance in CSS pixels into the nearest spacing step.
func step(px float64) string {
	switch {
	case px <= 0:
		return "none"
	case px <= 6:
		return "xs"
	case px <= 12:
		return "s"
	case px <= 20:
		return "m"
	case px <= 32:
		return "l"
	}
	return "xl"
}

func (m *legacy) node(n *legacyNode, parent string) []Section {
	s := Section{ID: m.id(n.ID), Props: map[string]any{}, When: m.cond(n.Visibility)}
	if mod, _ := n.Style["module"].(string); mod != "" && scopePattern.MatchString(mod) {
		s.Scope = mod
	}
	str := func(k string) string { v, _ := n.Props[k].(string); return v }
	title := func() {
		if t := str("title"); t != "" {
			s.Props["title"] = t
		}
	}
	switch n.Type {
	case "Shelf", "PosterRow", "Carousel", "Grid", "List", "HeroBanner":
		src, known := legacySources[bindingPath(n, "items")]
		switch {
		case known && (n.Type == "HeroBanner" || n.Type == "Carousel"):
			s.Component, s.Source = "Hero", src
			title()
		case known:
			s.Component, s.Source = map[string]string{"Grid": "Grid", "List": "List"}[n.Type], src
			if s.Component == "" {
				s.Component = "Shelf"
			}
			title()
			if st := legacyStyle(str("cardShape")); st != "" && s.Component != "List" {
				s.Props["cardStyle"] = st
			}
			if spec, _ := SourceType(src.Type()); spec != nil && len(spec.PageKinds) > 0 && !contains(spec.PageKinds, m.kind) {
				m.notes["a row of "+src.Type()+" on a page where it has no meaning"] = true
				return nil
			}
		case n.Type == "HeroBanner" && (str("title") != "" || str("subtitle") != ""):
			s.Component = "Banner"
			title()
			if t := str("subtitle"); t != "" {
				s.Props["text"] = t
			}
			if b := str("backdropUrl"); strings.HasPrefix(b, "/api/") {
				s.Props["image"] = b
			}
		case len(n.Children) > 0:
			// Cards written out by hand become a list of links.
			s.Component = "List"
			title()
			for i := range n.Children {
				c := &n.Children[i]
				t, _ := c.Props["title"].(string)
				if t == "" {
					continue
				}
				row := Section{ID: m.id(c.ID), Component: "ListItem", Props: map[string]any{"title": t}}
				if sub, _ := c.Props["subtitle"].(string); sub != "" {
					row.Props["subtitle"] = sub
				}
				if to := m.target(c); to != "" {
					row.Props["to"] = to
				}
				s.Children = append(s.Children, row)
			}
			m.notes["pictures on cards written out by hand"] = true
			if len(s.Children) == 0 {
				return nil
			}
		default:
			if p := bindingPath(n, "items"); p != "" {
				m.notes["a row bound to "+p] = true
			}
			return nil
		}
	case "Stack", "Toolbar":
		switch bindingPath(n, "sections") {
		case "$.home.sections":
			s.Component, s.Props = "Shelf", map[string]any{"title": "Recently added in {each}", "seeAll": "always"}
			s.Source, s.Repeat = Source{"type": "recentlyAdded", "library": "$each"}, &Repeat{Over: "libraries"}
			return []Section{s}
		case "$.search.sections":
			s.Component, s.Props = "Shelf", map[string]any{"title": "{each}", "seeAll": "never"}
			s.Source, s.Repeat = Source{"type": "search", "query": "$query", "library": "$each"}, &Repeat{Over: "libraries"}
			return []Section{s}
		case "$.music.sections":
			s.Component, s.Props = "Shelf", map[string]any{"title": "Recently added in {each}"}
			s.Source, s.Repeat = Source{"type": "albums", "library": "$each", "sort": "added"}, &Repeat{Over: "libraries", Kinds: []string{"music"}}
			return []Section{s}
		}
		s.Component = "Stack"
		row := n.Type == "Toolbar" || n.Style["direction"] == "row"
		if row {
			s.Props["direction"] = "row"
		}
		if g, ok := n.Style["gap"].(float64); ok {
			s.Props["gap"] = step(g)
		}
		kids := m.children(n.Children, "Stack")
		if row {
			var keep []Section
			for _, k := range kids {
				if contains([]string{"Button", "Text", "Image", "Spacer", "Divider", "SearchBox"}, k.Component) {
					keep = append(keep, k)
				} else {
					m.notes["a "+k.Component+" inside a row"] = true
				}
			}
			kids = keep
		}
		s.Children = kids
		if len(kids) == 0 {
			return nil
		}
	case "Text":
		text := str("text")
		if ph, ok := legacyTexts[bindingPath(n, "text")]; ok {
			text = ph
		} else if p := bindingPath(n, "text"); strings.HasPrefix(p, "$.item.") {
			return m.header(s)
		}
		if text == "" {
			return nil
		}
		s.Component, s.Props["text"] = "Text", text
		if role := map[string]string{"pageTitle": "pageTitle", "heading": "heading", "title": "title", "body": "body", "hint": "caption", "caption": "caption"}[str("variant")]; role != "" {
			s.Props["role"] = role
		}
	case "Image":
		if p := bindingPath(n, "src"); strings.HasPrefix(p, "$.item.") {
			return m.header(s)
		}
		if !strings.HasPrefix(str("src"), "/api/") {
			m.notes["a picture from outside this server"] = true
			return nil
		}
		s.Component, s.Props["src"] = "Image", str("src")
		if a := str("alt"); a != "" {
			s.Props["alt"] = a
		}
	case "Button":
		act := m.doc.Actions[n.Action]
		typ, _ := act["type"].(string)
		target, _ := act["target"].(string)
		if typ == "play" && m.kind == "detail" {
			return m.header(s)
		}
		s.Component, s.Props["label"] = "Button", str("label")
		if ic := str("icon"); contains(Icons, ic) {
			s.Props["icon"] = ic
		}
		if v := map[string]string{"primary": "primary", "ghost": "ghost"}[str("variant")]; v != "" {
			s.Props["variant"] = v
		}
		switch {
		case typ == "navigate" && strings.HasPrefix(target, "/settings"):
			s.Props["native"] = "settings"
		case typ == "navigate" && ValidRoute(target):
			s.Props["to"] = target
		case typ == "external" && ValidURL(target):
			s.Props["url"] = target
		default:
			m.notes["a button whose action ("+firstNonEmpty(typ, "none")+") has no equivalent"] = true
			return nil
		}
		if str("label") == "" {
			return nil
		}
	case "Tabs":
		s.Component = "Tabs"
		for i := range n.Children {
			c := &n.Children[i]
			label, _ := c.Props["label"].(string)
			tab := Section{ID: m.id(c.ID), Component: "Tab", Props: map[string]any{"label": label}, When: m.cond(c.Visibility), Children: m.children(c.Children, "Tab")}
			if label != "" && len(tab.Children) > 0 {
				s.Children = append(s.Children, tab)
			}
		}
		if len(s.Children) == 0 {
			return nil
		}
	case "ListItem":
		if parent != "List" {
			return nil
		}
		s.Component, s.Props["title"] = "ListItem", str("title")
		if sub := str("subtitle"); sub != "" {
			s.Props["subtitle"] = sub
		}
		if to := m.target(n); to != "" {
			s.Props["to"] = to
		}
	case "Spacer":
		size, _ := n.Props["size"].(float64)
		if size <= 0 {
			return nil
		}
		s.Component, s.Props["size"] = "Spacer", step(size)
	case "Divider":
		s.Component = "Divider"
	case "SearchBar":
		s.Component = "SearchBox"
		if ph := str("placeholder"); ph != "" {
			s.Props["placeholder"] = ph
		}
	case "Badge":
		if str("text") == "" {
			return nil
		}
		s.Component, s.Props = "Text", map[string]any{"text": str("text"), "role": "caption"}
	case "Card":
		if strings.HasPrefix(bindingPath(n, "title"), "$.item.") || m.kind == "detail" {
			return m.header(s)
		}
		m.notes["a card on its own"] = true
		return nil
	case "Video", "Modal", "ProgressBar":
		m.notes["a "+n.Type+" (the app draws players and dialogs itself)"] = true
		return nil
	default:
		m.notes["a node of type "+n.Type] = true
		return nil
	}
	for k := range n.Style {
		if k != "module" && k != "direction" && k != "gap" {
			m.notes["colours and sizes written into a node's style (the theme decides them now)"] = true
			break
		}
	}
	if len(s.Props) == 0 {
		s.Props = nil
	}
	return []Section{s}
}

// header places the page's one DetailHeader where the first node about the
// title was.
func (m *legacy) header(s Section) []Section {
	if m.kind != "detail" || m.head {
		return nil
	}
	m.head = true
	return []Section{{ID: s.ID, Component: "DetailHeader", When: nil}}
}

// target is the route a node's navigate action led to, when it was one.
func (m *legacy) target(n *legacyNode) string {
	act := m.doc.Actions[n.Action]
	if t, _ := act["type"].(string); t != "navigate" {
		return ""
	}
	target, _ := act["target"].(string)
	if target == "$route" {
		target, _ = n.Props["route"].(string)
	}
	if !ValidRoute(target) {
		return ""
	}
	return target
}

func (m *legacy) cond(c *legacyCond) *Condition {
	if c == nil {
		return nil
	}
	out := Condition{Feature: c.Feature, Module: c.Module, MinWidth: c.MinWidth, MaxWidth: c.MaxWidth, Locale: c.Locale}
	// history and playback were always on; a condition on them said nothing.
	if out.Feature == "history" || out.Feature == "playback" {
		out.Feature = ""
	}
	if !wordPattern.MatchString(out.Module) {
		out.Module = ""
	}
	for i := range c.All {
		if x := m.cond(&c.All[i]); x != nil {
			out.All = append(out.All, *x)
		}
	}
	for i := range c.Any {
		if x := m.cond(&c.Any[i]); x != nil {
			out.Any = append(out.Any, *x)
		}
	}
	out.Not = m.cond(c.Not)
	switch c.Role {
	case "":
	case "admin":
		out.Admin = &yes
	case "adult", "kids", "guest":
		out.ProfileKind = []string{c.Role}
	default:
		m.notes["a condition on the role "+c.Role] = true
	}
	switch c.Platform {
	case "":
	case "tv":
		out.FormFactor = []string{FormTV}
	case "web", "ios", "android", "desktop":
		out.Platform = []string{c.Platform}
	default:
		// A platform no app ever was: the condition never held.
		return &Condition{Not: &Condition{}}
	}
	if contains(FormFactors, c.FormFactor) {
		out.FormFactor = []string{c.FormFactor}
	}
	switch {
	case c.Flag == "":
	case contains(Flags, c.Flag):
		out.Flag = c.Flag
	case strings.HasPrefix(c.Flag, "formFactor:") && contains(FormFactors, strings.TrimPrefix(c.Flag, "formFactor:")):
		out.FormFactor = []string{strings.TrimPrefix(c.Flag, "formFactor:")}
	case c.Flag == "searchIdle":
		out.Not = &Condition{Flag: "hasQuery"}
	case wordPattern.MatchString(c.Flag) && !contains([]string{"libraryEmpty", "librariesEmpty", "hasProgress", "notPlayable", "searchEmpty", "livetvEmpty", "musicEmpty"}, c.Flag):
		out.Capability = c.Flag
	default:
		m.notes["a condition on the flag "+c.Flag+" (empty and error states are each component's own now)"] = true
		return &Condition{Not: &Condition{}} // never: those nodes were empty-state messages
	}
	if isZeroCondition(&out) {
		return nil
	}
	return &out
}
