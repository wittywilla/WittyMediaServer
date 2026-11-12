// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package conformance

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// Normalised is a page as a renderer lays it out, described without pixels:
// what is on the page in what order, how many cards each row holds and how
// big they are, which picture each card asks for, the text each thing shows
// and how many lines it may take, what assistive technology is told, and
// where focus starts and goes. Two renderers that agree on this for every
// fixture draw the same page.
type Normalised struct {
	// Kit is the version of the conformance kit this description follows.
	Kit int `json:"kit"`
	// Page is the page's heading and the profile it was laid out with.
	Page NormalisedPage `json:"page"`
	// Visible is every component drawn, depth first in display order.
	Visible []Element `json:"visible"`
	// Focus is the focus order: node IDs in order, the one focus starts on,
	// and each group's members in order.
	Focus NormalisedFocus `json:"focus"`
}

// NormalisedPage is the frame of a normalised page.
type NormalisedPage struct {
	Title        string `json:"title"`
	FormFactor   string `json:"formFactor"`
	Breakpoint   string `json:"breakpoint"`
	Density      string `json:"density"`
	ContentWidth int    `json:"contentWidth"`
	// SectionGap is the distance between the page's sections, in points: the
	// profile's l spacing (m on a compact window).
	SectionGap int `json:"sectionGap"`
	// Scope is the page's theme scope, "" for none.
	Scope string `json:"scope,omitempty"`
	// Shell is the frame the app draws around the page: the layout option, the
	// menu's style and breadth (document.layout.shell). Absent: none was sent.
	Shell *sdui.Shell `json:"shell,omitempty"`
}

// Element is one drawn component.
type Element struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Depth int    `json:"depth"`
	// Role and Label are what assistive technology is told. A Label of ""
	// with Role "none" means the element is hidden from it.
	Role  string `json:"role"`
	Label string `json:"label"`
	// State is "" (ready), "deferred", "empty" or "error".
	State string `json:"state,omitempty"`
	// Text is what the element itself shows, by part: title, subtitle,
	// synopsis, text, button. Lines is the most lines each part may take
	// before it is cut with an ellipsis; a part not listed isn't limited.
	Text  map[string]string `json:"text,omitempty"`
	Lines map[string]int    `json:"lines,omitempty"`
	// TextSize is the size of the element's main text, in points.
	TextSize int `json:"textSize,omitempty"`
	// Cards is how many cards (or rows, tracks, tabs, slides) it holds now.
	// Placeholders is how many grey placeholders it shows while its items
	// are on their way (state deferred).
	Cards        *int `json:"cards,omitempty"`
	Placeholders int  `json:"placeholders,omitempty"`
	// CardStyle, CardWidth and CardHeight describe its cards, in points.
	// Columns is a Grid's column count.
	CardStyle  string `json:"cardStyle,omitempty"`
	CardWidth  int    `json:"cardWidth,omitempty"`
	CardHeight int    `json:"cardHeight,omitempty"`
	Columns    int    `json:"columns,omitempty"`
	// HasMore says it asks for more items as it is scrolled; SeeAll is the
	// route its "See all" opens.
	HasMore bool   `json:"hasMore,omitempty"`
	SeeAll  string `json:"seeAll,omitempty"`
	// Image is the picture it asks for: its kind and width in pixels.
	Image string `json:"image,omitempty"`
	// Shows lists the parts of a card that are drawn, in reading order.
	Shows []string `json:"shows,omitempty"`
	// Progress is a progress bar's fill, in whole percent.
	Progress *int `json:"progress,omitempty"`
	// Buttons are the labels of its buttons, in order, the primary one
	// marked with a leading "*".
	Buttons []string `json:"buttons,omitempty"`
	// Action is what activating it does: "navigate /item/3", "play 3",
	// "native settings", "command watched.set 3", "external https://…".
	Action string `json:"action,omitempty"`
	// Scope is its theme scope, "" for the page's.
	Scope string `json:"scope,omitempty"`
}

// NormalisedFocus is the focus order of a page.
type NormalisedFocus struct {
	Order   []string            `json:"order"`
	Default string              `json:"default"`
	Groups  map[string][]string `json:"groups,omitempty"`
}

// KitVersion is the version of the fixtures and of Normalised.
const KitVersion = 1

func intp(n int) *int { return &n }

func str(p map[string]any, k string) string {
	s, _ := p[k].(string)
	return s
}

func num(p map[string]any, k string) (float64, bool) {
	switch v := p[k].(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	}
	return 0, false
}

func obj(p map[string]any, k string) map[string]any {
	m, _ := p[k].(map[string]any)
	return m
}

func list(p map[string]any, k string) []any {
	l, _ := p[k].([]any)
	return l
}

// Normalise describes a rendered document the way a renderer lays it out. It
// reads only the document: a renderer's own normaliser, working from the
// views it built, must give the same answer.
func Normalise(doc sdui.Document) Normalised {
	doc = roundTrip(doc)
	l := doc.Layout
	if l == nil {
		l = sdui.ProfileFor(sdui.FormWeb, "")
	}
	gap := l.Spacing["l"]
	if l.Breakpoint == "compact" {
		gap = l.Spacing["m"]
	}
	out := Normalised{Kit: KitVersion, Visible: []Element{},
		Page:  NormalisedPage{Title: doc.Page.Title, FormFactor: l.FormFactor, Breakpoint: l.Breakpoint, Density: l.Density, ContentWidth: l.ContentWidth(), SectionGap: gap, Shell: l.Shell},
		Focus: NormalisedFocus{Order: []string{}}}
	if doc.Theme != nil {
		out.Page.Scope = doc.Theme.Scope
	}
	n := &normaliser{l: l, out: &out, page: doc.Page.Title}
	n.walk(&doc.Root, 0, nil)
	// Focus: by Order, which is reading order.
	byOrder := map[int]*sdui.Node{}
	var collect func(x *sdui.Node)
	collect = func(x *sdui.Node) {
		if x.Focus != nil {
			byOrder[x.Focus.Order] = x
		}
		for i := range x.Children {
			collect(&x.Children[i])
		}
	}
	collect(&doc.Root)
	for i := 1; i <= len(byOrder); i++ {
		x := byOrder[i]
		if x == nil {
			continue
		}
		out.Focus.Order = append(out.Focus.Order, x.ID)
		if x.Focus.Default {
			out.Focus.Default = x.ID
		}
		if g := x.Focus.Group; g != "" {
			if out.Focus.Groups == nil {
				out.Focus.Groups = map[string][]string{}
			}
			out.Focus.Groups[g] = append(out.Focus.Groups[g], x.ID)
		}
	}
	return out
}

type normaliser struct {
	l    *sdui.Layout
	out  *Normalised
	page string
}

func (n *normaliser) walk(x *sdui.Node, depth int, parent *sdui.Node) {
	e := n.element(x, depth, parent)
	n.out.Visible = append(n.out.Visible, e)
	for i := range x.Children {
		n.walk(&x.Children[i], depth+1, x)
	}
}

func action(a *sdui.Action) string {
	if a == nil {
		return ""
	}
	switch a.Type {
	case sdui.ActNavigate, sdui.ActNative:
		return a.Type + " " + a.To
	case sdui.ActPlay:
		s := "play " + a.Item
		if a.Params["resume"] == false {
			s += " from the start"
		}
		return s
	case sdui.ActExternal:
		return "external " + a.URL
	case sdui.ActCommand:
		s := "command " + a.Op
		if a.Item != "" {
			s += " " + a.Item
		}
		if t, _ := a.Params["tab"].(string); t != "" {
			s += " " + t
		}
		return s
	}
	return a.Type
}

func image(p map[string]any, k string) string {
	m := obj(p, k)
	if m == nil {
		return ""
	}
	w, _ := num(m, "width")
	return str(m, "kind") + " " + strconv.Itoa(int(w))
}

func percent(p map[string]any) *int {
	f, ok := num(p, "progress")
	if !ok {
		return nil
	}
	return intp(int(f*100 + 0.5))
}

func join(parts ...string) string {
	var keep []string
	for _, s := range parts {
		if s != "" {
			keep = append(keep, s)
		}
	}
	return strings.Join(keep, ", ")
}

var (
	mdLinks  = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	mdMarks  = regexp.MustCompile("[*_`#>]+")
	mdSpaces = regexp.MustCompile(`\s+`)
)

// PlainText is Markdown as assistive technology reads it: links by their
// text, marks removed, white space folded.
func PlainText(s string) string {
	s = mdLinks.ReplaceAllString(s, "$1")
	s = mdMarks.ReplaceAllString(s, "")
	return strings.TrimSpace(mdSpaces.ReplaceAllString(s, " "))
}

// buttons lists the labels of a props.actions list, the primary one starred.
func buttons(p map[string]any) []string {
	var out []string
	for _, a := range list(p, "actions") {
		m, _ := a.(map[string]any)
		label := str(m, "label")
		if m["primary"] == true {
			label = "*" + label
		}
		out = append(out, label)
	}
	return out
}

func facts(p map[string]any, sep string) string {
	var parts []string
	for _, f := range list(p, "facts") {
		m, _ := f.(map[string]any)
		parts = append(parts, str(m, "value"))
	}
	return strings.Join(parts, sep)
}

// CardLabel is a card's accessibility label: the parts its style shows, in
// the style's order.
func CardLabel(p map[string]any) string {
	style, _ := sdui.CardStyle(str(p, "style"))
	var parts []string
	if r, ok := num(p, "rank"); ok {
		parts = append(parts, "Number "+strconv.Itoa(int(r)))
	}
	shows := []string{"title", "subtitle"}
	if style != nil {
		shows = style.Shows
	}
	if !has(shows, "title") {
		parts = append(parts, str(p, "title")) // a photo's name is read, though not drawn
	}
	for _, s := range shows {
		switch s {
		case "title", "subtitle", "remaining":
			parts = append(parts, str(p, s))
		case "number":
			if v := str(p, s); v != "" {
				parts = append(parts, "Channel "+v)
			}
		case "now", "next":
			if m := obj(p, s); m != nil {
				parts = append(parts, s+" "+str(m, "title"))
			}
		case "progress":
			if pc := percent(p); pc != nil && obj(p, "now") == nil {
				parts = append(parts, strconv.Itoa(*pc)+" percent watched")
			}
		case "watched":
			if p["watched"] == true {
				parts = append(parts, "Watched")
			}
		case "unwatched":
			if c, ok := num(p, "unwatched"); ok {
				parts = append(parts, strconv.Itoa(int(c))+" unwatched")
			}
		case "count":
			if c, ok := num(p, "count"); ok {
				parts = append(parts, strconv.Itoa(int(c))+" items")
			}
		case "badges":
			for _, b := range list(p, "badges") {
				if s, ok := b.(string); ok {
					parts = append(parts, s)
				}
			}
		}
	}
	return join(parts...)
}

func has(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func (n *normaliser) element(x *sdui.Node, depth int, parent *sdui.Node) Element {
	p := x.Props
	if p == nil {
		p = map[string]any{}
	}
	e := Element{ID: x.ID, Type: x.Type, Depth: depth, State: x.State, Scope: x.Scope, Action: action(x.Action), Role: "none"}
	text := func(k, v string) {
		if v == "" {
			return
		}
		if e.Text == nil {
			e.Text = map[string]string{}
		}
		e.Text[k] = v
	}
	lines := func(k string, v int) {
		if v <= 0 {
			return
		}
		if e.Lines == nil {
			e.Lines = map[string]int{}
		}
		e.Lines[k] = v
	}
	collection := func() {
		e.Cards = intp(len(x.Children))
		if c := obj(p, "card"); c != nil {
			w, _ := num(c, "width")
			h, _ := num(c, "height")
			e.CardStyle, e.CardWidth, e.CardHeight = str(p, "cardStyle"), int(w), int(h)
			if x.State == "deferred" {
				e.Placeholders = n.l.Placeholders(int(w))
			}
		}
		if cols, ok := num(p, "columns"); ok {
			e.Columns = int(cols)
		}
		e.HasMore = obj(p, "more") != nil && x.State != "deferred" && x.State != "error"
		if sa := obj(p, "seeAll"); sa != nil {
			e.SeeAll = str(sa, "route")
		}
		text("title", str(p, "title"))
		text("subtitle", str(p, "subtitle"))
		text("empty", str(p, "emptyText"))
		lines("title", 1)
		e.TextSize = n.l.Text["title"]
	}
	switch x.Type {
	case "Page":
		e.Role, e.Label = "main", str(p, "title")
	case "Shelf":
		collection()
		e.Role, e.Label = "list", str(p, "title")
	case "Grid":
		collection()
		e.Role, e.Label = "grid", firstNonEmpty(str(p, "title"), n.page)
		if !n.l.FocusEngine && n.l.FormFactor != sdui.FormDesktop && n.l.FormFactor != sdui.FormWeb {
			e.Role = "list"
		}
	case "List":
		e.Cards = intp(len(x.Children))
		e.Role, e.Label = "list", str(p, "title")
		text("title", str(p, "title"))
		text("empty", str(p, "emptyText"))
		e.HasMore = obj(p, "more") != nil
	case "Card":
		n.card(&e, x, parent)
	case "Hero":
		e.Role, e.Label = "region", firstNonEmpty(str(p, "title"), "Featured")
		e.Cards = intp(len(x.Children))
		if size := obj(p, "size"); size != nil {
			w, _ := num(size, "width")
			h, _ := num(size, "height")
			e.CardWidth, e.CardHeight = int(w), int(h)
		}
		if adv, ok := num(p, "autoAdvanceSec"); ok {
			text("autoAdvance", strconv.Itoa(int(adv))+" s")
		}
	case "Tabs":
		e.Role, e.Label = "tablist", ""
		e.Cards = intp(len(x.Children))
	case "Tab":
		e.Role, e.Label = "tab", str(p, "label")
		if p["selected"] == true {
			e.Label += ", selected"
		}
		text("title", str(p, "label"))
		lines("title", 1)
	case "FilterBar":
		e.Role, e.Label = "toolbar", "Filter and sort"
		if s := obj(p, "sort"); s != nil {
			e.Buttons = append(e.Buttons, "Sort: "+optionLabel(s, str(s, "value")))
		}
		for _, f := range list(p, "fields") {
			m, _ := f.(map[string]any)
			e.Buttons = append(e.Buttons, str(m, "label")+": "+firstNonEmpty(optionLabel(m, str(m, "value")), "Any"))
		}
		if c, ok := num(p, "count"); ok {
			text("count", strconv.Itoa(int(c))+" titles")
		}
	case "DetailHeader":
		e.Role, e.Label = "region", str(p, "title")
		text("title", str(p, "title"))
		text("tagline", str(p, "tagline"))
		text("facts", facts(p, " · "))
		text("overview", str(p, "overview"))
		lines("title", 2)
		lines("overview", 4)
		e.TextSize = n.l.Text["pageTitle"]
		e.Buttons = buttons(p)
		e.Progress = percent(p)
		if im := obj(p, "images"); im != nil {
			e.Image = join(image(im, "backdrop"), image(im, "poster"))
		}
	case "SeasonList":
		e.Role, e.Label = "region", "Episodes"
		e.Cards = intp(len(x.Children))
		for _, s := range list(p, "seasons") {
			m, _ := s.(map[string]any)
			label := str(m, "title")
			if m["selected"] == true {
				label = "*" + label
			}
			e.Buttons = append(e.Buttons, label)
		}
		e.HasMore = obj(p, "more") != nil
		if c := obj(p, "card"); c != nil {
			w, _ := num(c, "width")
			h, _ := num(c, "height")
			e.CardStyle, e.CardWidth, e.CardHeight = "episode", int(w), int(h)
		}
	case "TrackList":
		e.Role, e.Label = "list", "Tracks"
		e.Cards = intp(len(x.Children))
	case "Credits":
		// The cast's and the guest stars' cards in rows, then each department's people; the
		// parts in order with how many of each were sent, and Show all when there are more.
		e.Role, e.Label = "region", str(p, "title")
		e.Cards = intp(len(x.Children))
		text("title", str(p, "title"))
		lines("title", 1)
		e.TextSize = n.l.Text["heading"]
		var parts []string
		for _, g := range list(p, "groups") {
			m, _ := g.(map[string]any)
			shown, _ := num(m, "shown")
			total, _ := num(m, "total")
			part := str(m, "label") + " " + strconv.Itoa(int(shown))
			if total > shown {
				part += " of " + strconv.Itoa(int(total))
			}
			parts = append(parts, part)
		}
		text("groups", strings.Join(parts, " · "))
		if c := obj(p, "card"); c != nil {
			w, _ := num(c, "width")
			h, _ := num(c, "height")
			e.CardStyle, e.CardWidth, e.CardHeight = "person", int(w), int(h)
		}
		if m := obj(p, "more"); m != nil {
			e.HasMore = true
			e.Buttons = []string{firstNonEmpty(str(m, "label"), "Show all")}
		}
	case "Track":
		e.Role = "button"
		parts := []string{}
		if t, ok := num(p, "number"); ok && (parent == nil || parent.Props["showNumbers"] != false) {
			parts = append(parts, "Track "+strconv.Itoa(int(t)))
		}
		parts = append(parts, str(p, "title"))
		text("title", str(p, "title"))
		lines("title", 1)
		if parent != nil && parent.Props["showArtist"] == true {
			parts = append(parts, str(p, "artist"))
			text("subtitle", str(p, "artist"))
		}
		if d, ok := num(p, "durationSec"); ok && (parent == nil || parent.Props["showDuration"] != false) {
			parts = append(parts, spokenDuration(int(d)))
			text("duration", clock(int(d)))
		}
		e.Label = join(parts...)
		e.Progress = percent(p)
	case "Banner":
		e.Role, e.Label = "button", join(str(p, "title"), str(p, "text"), str(p, "buttonLabel"))
		if x.Action == nil {
			e.Role = "group"
		}
		text("title", str(p, "title"))
		text("text", str(p, "text"))
		text("button", str(p, "buttonLabel"))
		lines("title", 2)
		lines("text", 2)
		e.Image = image(p, "image")
	case "Text":
		role := firstNonEmpty(str(p, "role"), "body")
		e.TextSize = n.l.Text[role]
		e.Role = map[string]string{"pageTitle": "heading 1", "heading": "heading 2", "title": "heading 3"}[role]
		if e.Role == "" {
			e.Role = "text"
		}
		shown := str(p, "text")
		if str(p, "format") != "markdown" {
			e.Label = shown
		} else {
			e.Label = PlainText(shown)
		}
		text("text", shown)
		if ml, ok := num(p, "maxLines"); ok {
			lines("text", int(ml))
		}
	case "Notice":
		e.Role = "status"
		if t := str(p, "tone"); t == "warning" || t == "danger" {
			e.Role = "alert"
		}
		e.Label = strings.TrimPrefix(str(p, "title")+". "+str(p, "text"), ". ")
		text("title", str(p, "title"))
		text("text", str(p, "text"))
		text("button", str(p, "buttonLabel"))
	case "Button":
		e.Role, e.Label = "button", str(p, "label")
		text("title", str(p, "label"))
		lines("title", 1)
	case "Image":
		e.Image = image(p, "image")
		if alt := str(p, "alt"); alt != "" || x.Action != nil {
			e.Role, e.Label = "img", alt
		}
	case "SearchBox":
		e.Role, e.Label = "searchbox", "Search"
		text("placeholder", str(p, "placeholder"))
		text("query", str(p, "query"))
	case "ListItem":
		e.Role = "text"
		if x.Action != nil {
			e.Role = "button"
		}
		e.Label = join(str(p, "title"), str(p, "subtitle"), str(p, "trailing"))
		text("title", str(p, "title"))
		text("subtitle", str(p, "subtitle"))
		text("trailing", str(p, "trailing"))
		lines("title", 1)
		lines("subtitle", 1)
		e.Image = image(p, "image")
	case "Divider":
		e.Role = "separator"
	case "NativeView":
		e.Role, e.Label = "group", str(p, "view")
	case "Stack", "Split", "Spacer":
	default:
		// A type this catalogue doesn't have is drawn as a plain column.
		e.Type = "Stack"
	}
	return e
}

func optionLabel(m map[string]any, value string) string {
	for _, o := range list(m, "options") {
		om, _ := o.(map[string]any)
		if str(om, "value") == value {
			return str(om, "label")
		}
	}
	return ""
}

func spokenDuration(sec int) string {
	m, s := sec/60, sec%60
	switch {
	case m == 0:
		return fmt.Sprintf("%d seconds", s)
	case s == 0:
		return fmt.Sprintf("%d minutes", m)
	}
	return fmt.Sprintf("%d minutes %d seconds", m, s)
}

func clock(sec int) string {
	if sec >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", sec/3600, sec%3600/60, sec%60)
	}
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// card describes one Card: what its style shows of what it was given.
func (n *normaliser) card(e *Element, x *sdui.Node, parent *sdui.Node) {
	p := x.Props
	e.Role = "button"
	e.Image = image(p, "image")
	if parent != nil && parent.Type == "Hero" {
		// A slide of a Hero: a group, not a button; its buttons are listed.
		e.Role = "group"
		e.Label = join(str(p, "title"), facts(p, ", "))
		e.Text = map[string]string{"title": str(p, "title")}
		if f := facts(p, " · "); f != "" {
			e.Text["facts"] = f
		}
		if s := str(p, "synopsis"); s != "" {
			e.Text["synopsis"] = s
		}
		e.Lines = map[string]int{"title": 2, "synopsis": 3}
		e.Buttons = buttons(p)
		e.Action = ""
		return
	}
	e.Label = CardLabel(p)
	style, _ := sdui.CardStyle(str(p, "style"))
	if style == nil {
		// A style this catalogue doesn't have is drawn as its shape: the
		// title and subtitle under the picture.
		style = &sdui.CardStyleSpec{Shows: []string{"title", "subtitle"}, TitleLines: 1, SubtitleLines: 1}
	}
	titles := parent == nil || parent.Props["showTitles"] != false
	e.Text, e.Lines = map[string]string{}, map[string]int{}
	for _, s := range style.Shows {
		switch s {
		case "title", "subtitle":
			if !titles {
				continue
			}
			if v := str(p, s); v != "" {
				e.Text[s] = v
				e.Shows = append(e.Shows, s)
				if s == "title" {
					e.Lines[s] = style.TitleLines
				} else {
					e.Lines[s] = style.SubtitleLines
				}
			}
		case "synopsis":
			if v := str(p, s); v != "" {
				e.Text[s], e.Lines[s] = v, 2
				e.Shows = append(e.Shows, s)
			}
		case "remaining", "number":
			if v := str(p, s); v != "" {
				e.Text[s] = v
				e.Shows = append(e.Shows, s)
			}
		case "now", "next":
			if m := obj(p, s); m != nil {
				e.Text[s], e.Lines[s] = str(m, "title"), 1
				e.Shows = append(e.Shows, s)
			}
		case "progress":
			if pc := percent(p); pc != nil {
				e.Progress = pc
				e.Shows = append(e.Shows, s)
			}
		case "watched":
			if p["watched"] == true {
				e.Shows = append(e.Shows, s)
			}
		case "unwatched", "count":
			if c, ok := num(p, s); ok {
				e.Text[s] = strconv.Itoa(int(c))
				e.Shows = append(e.Shows, s)
			}
		case "badges":
			var b []string
			for _, v := range list(p, s) {
				if sv, ok := v.(string); ok {
					b = append(b, sv)
				}
			}
			if len(b) > 0 {
				e.Text[s] = strings.Join(b, " ")
				e.Shows = append(e.Shows, s)
			}
		}
	}
	if r, ok := num(p, "rank"); ok {
		e.Text["rank"] = strconv.Itoa(int(r))
		e.Shows = append([]string{"rank"}, e.Shows...)
	}
	if e.Image == "" && str(p, "icon") != "" {
		e.Image = "icon " + str(p, "icon")
	}
	if len(e.Text) == 0 {
		e.Text = nil
	}
	if len(e.Lines) == 0 {
		e.Lines = nil
	}
}
