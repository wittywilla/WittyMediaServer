// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"strconv"
	"strings"
)

// Form factors: the kinds of device one authored screen is fitted to.
const (
	FormTV      = "tv"
	FormPhone   = "phone"
	FormTablet  = "tablet"
	FormDesktop = "desktop"
	FormWeb     = "web"
)

// Insets are distances from each edge of the screen, in points.
type Insets struct {
	Top    int `json:"top"`
	Right  int `json:"right"`
	Bottom int `json:"bottom"`
	Left   int `json:"left"`
}

// ImageSize is the picture a client should ask for, in pixels, for one card
// shape: the size on screen in points times the screen's scale.
type ImageSize struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Aspect string `json:"aspect"` // "2/3", "16/9", "1/1"
}

// Layout is a device profile: what a screen has to fit. Render is given one
// in RenderContext.Profile, fits the screen to it, and sends it along as
// Document.Layout so the client lays out with the same numbers.
//
// The tv profiles are tvOS's rules, which are stricter than anyone else's and
// so are what the schema is designed around:
//
//   - there is no pointer, only focus: every node a person can act on is
//     focusable, in a fixed order, and one of them is where focus starts;
//   - a screen scrolls one way (down) and its rows scroll sideways; nothing
//     scrolls both ways and scrolling regions do not nest;
//   - content keeps clear of the overscan area (SafeArea);
//   - text is read from three metres away (Text, in points on a 1920×1080
//     canvas);
//   - an Apple TV HD has little memory, so a screen is cut to MaxSections rows
//     of MaxItems cards, and pictures are asked for at the size they are shown;
//   - there is no web view and no browser: nothing that opens a web page.
type Layout struct {
	// FormFactor is tv, phone, tablet, desktop or web.
	FormFactor string `json:"formFactor"`
	// Class tells devices of one form factor apart: "hd" and "4k" for tv.
	Class string `json:"class,omitempty"`
	// Canvas is the screen in points, when the form factor fixes it (tv:
	// 1920×1080). Zero: whatever the window is.
	Canvas Viewport `json:"canvas,omitzero"`
	// Scale is pixels per point: 1 on an Apple TV HD, 2 on a 4K one.
	Scale int `json:"scale"`
	// SafeArea is the margin content keeps inside, in points. Zero on devices
	// whose system reports its own.
	SafeArea Insets `json:"safeArea"`
	// Scroll is always "vertical": the screen scrolls down, rows sideways.
	Scroll string `json:"scroll"`
	// FocusEngine is true where focus is the only way to point (tv).
	FocusEngine bool `json:"focusEngine"`
	// Text is the size of each Text variant, in points.
	Text map[string]int `json:"text,omitempty"`
	// Images is the picture size to ask for, by card shape: poster, wide,
	// square, hero, and on tv the three Top Shelf shapes.
	Images map[string]ImageSize `json:"images,omitempty"`
	// GridColumns is how many columns a Grid has; 0 leaves it to the window.
	GridColumns int `json:"gridColumns,omitempty"`
	// MaxSections is the most rows a screen is sent; MaxItems the most cards
	// in one row; MaxGridItems the most in a Grid or List.
	MaxSections  int `json:"maxSections"`
	MaxItems     int `json:"maxItems"`
	MaxGridItems int `json:"maxGridItems"`
	// ExternalLinks says whether the device can open a web page. Where it
	// can't, nodes whose action is "external" are left out.
	ExternalLinks bool `json:"externalLinks"`
	// Truncated is set by Render when rows were left out to fit MaxSections.
	Truncated bool `json:"truncated,omitempty"`
}

// tvText are tvOS's own text styles (title 1, title 2, title 3, body,
// caption 1, caption 2), in points.
var tvText = map[string]int{"pageTitle": 76, "heading": 57, "title": 48, "body": 29, "hint": 25, "caption": 25, "badge": 23}

func scaled(scale int, m map[string][3]int) map[string]ImageSize {
	out := make(map[string]ImageSize, len(m))
	for k, v := range m {
		out[k] = ImageSize{Width: v[0] * scale, Height: v[1] * scale, Aspect: strconv.Itoa(v[2]/100) + "/" + strconv.Itoa(v[2]%100)}
	}
	return out
}

// Shapes in points, with the aspect as w*100+h.
var (
	tvShapes = map[string][3]int{
		"poster": {250, 375, 203}, "wide": {548, 308, 1609}, "square": {308, 308, 101}, "hero": {1920, 1080, 1609},
		// What tvOS asks of Top Shelf pictures (TVTopShelfSectionedItem's three shapes).
		"topShelfPoster": {404, 608, 203}, "topShelfWide": {908, 512, 1609}, "topShelfSquare": {608, 608, 101},
	}
	phoneShapes  = map[string][3]int{"poster": {120, 180, 203}, "wide": {240, 135, 1609}, "square": {150, 150, 101}, "hero": {430, 242, 1609}}
	tabletShapes = map[string][3]int{"poster": {160, 240, 203}, "wide": {320, 180, 1609}, "square": {200, 200, 101}, "hero": {1024, 576, 1609}}
	deskShapes   = map[string][3]int{"poster": {180, 270, 203}, "wide": {360, 202, 1609}, "square": {220, 220, 101}, "hero": {1280, 720, 1609}}
)

// ProfileFor is the device profile for a form factor and class. An unknown
// form factor gets the web profile; an unknown tv class the HD one, the
// smaller of the two.
func ProfileFor(formFactor, class string) *Layout {
	switch formFactor {
	case FormTV:
		l := &Layout{FormFactor: FormTV, Class: "hd", Canvas: Viewport{Width: 1920, Height: 1080}, Scale: 1,
			SafeArea: Insets{Top: 60, Right: 80, Bottom: 60, Left: 80}, Scroll: "vertical", FocusEngine: true,
			Text: copyInts(tvText), GridColumns: 6, MaxSections: 10, MaxItems: 16, MaxGridItems: 60}
		if class == "4k" {
			l.Class, l.Scale, l.MaxSections, l.MaxItems, l.MaxGridItems = "4k", 2, 20, 30, 120
		}
		l.Images = scaled(l.Scale, tvShapes)
		return l
	case FormPhone:
		return &Layout{FormFactor: FormPhone, Scale: 3, Scroll: "vertical",
			Text:   map[string]int{"pageTitle": 28, "heading": 22, "title": 17, "body": 17, "hint": 13, "caption": 12, "badge": 11},
			Images: scaled(3, phoneShapes), GridColumns: 3, MaxSections: 16, MaxItems: 20, MaxGridItems: 60, ExternalLinks: true}
	case FormTablet:
		return &Layout{FormFactor: FormTablet, Scale: 2, Scroll: "vertical",
			Text:   map[string]int{"pageTitle": 34, "heading": 24, "title": 20, "body": 17, "hint": 15, "caption": 13, "badge": 12},
			Images: scaled(2, tabletShapes), GridColumns: 5, MaxSections: 20, MaxItems: 30, MaxGridItems: 100, ExternalLinks: true}
	case FormDesktop:
		return &Layout{FormFactor: FormDesktop, Scale: 2, Scroll: "vertical",
			Text:   map[string]int{"pageTitle": 28, "heading": 20, "title": 16, "body": 14, "hint": 13, "caption": 12, "badge": 11},
			Images: scaled(2, deskShapes), MaxSections: 30, MaxItems: 40, MaxGridItems: 200, ExternalLinks: true}
	}
	return &Layout{FormFactor: FormWeb, Scale: 2, Scroll: "vertical",
		Text:   map[string]int{"pageTitle": 28, "heading": 20, "title": 16, "body": 14, "hint": 13, "caption": 12, "badge": 11},
		Images: scaled(2, deskShapes), MaxSections: 30, MaxItems: 40, MaxGridItems: 200, ExternalLinks: true}
}

func copyInts(m map[string]int) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneLayout(l *Layout) *Layout {
	if l == nil {
		return nil
	}
	c := *l
	c.Text = copyInts(l.Text)
	c.Images = make(map[string]ImageSize, len(l.Images))
	for k, v := range l.Images {
		c.Images[k] = v
	}
	return &c
}

// ---- which nodes are what ----

// Interactive says whether a person can act on the node: a control, or
// anything with an action. Disabled buttons are not.
func Interactive(n *Node) bool {
	if n.Type == Button {
		if d, _ := n.Props["disabled"].(bool); d {
			return false
		}
	}
	switch n.Type {
	case Button, Card, ListItem, Tab, SearchBar:
		return true
	}
	return n.Action != ""
}

// rowScroller: scrolls sideways. pageScroller: scrolls down inside the screen.
func rowScroller(t ComponentType) bool { return t == Shelf || t == PosterRow || t == Carousel }
func pageScroller(t ComponentType) bool {
	return t == Grid || t == List
}

// focusGroup: the containers a client treats as one focus section.
func focusGroup(t ComponentType) bool {
	return rowScroller(t) || pageScroller(t) || t == Tabs || t == Toolbar || t == HeroBanner
}

// page: a region that scrolls by itself, so the no-nesting rule starts again.
func page(t ComponentType) bool { return t == Screen || t == Tab || t == Modal }

// itemHolder: containers whose "items" prop Render turns into children.
func itemHolder(t ComponentType) bool {
	return rowScroller(t) || pageScroller(t) || t == HeroBanner
}

// ---- items → children ----

// maxHeroItems is the most a HeroBanner rotates through, on any device.
const maxHeroItems = 5

// defaultMaxItems bounds a row when Render is given no profile.
const defaultMaxItems = 100

// itemProps are the keys of an item that become props of its Card or ListItem.
var itemProps = []string{"title", "subtitle", "imageUrl", "kind", "progress", "badges", "module", "route", "shape"}

// expandItems turns a container's resolved "items" (a list of objects, from a
// binding or written out) into child nodes, one Card (or ListItem in a List)
// each, so that every item is a node: it has an ID to diff by, takes focus,
// and counts toward the device's limits. The children replace any written
// into the template. An item's node is "<container>:<item id>", and its
// action is the container's "itemAction" prop, or "openItem" when the
// document defines that.
func expandItems(n *Node, doc *Document, l *Layout) {
	expandSections(n)
	raw, ok := n.Props["items"]
	if !ok || !itemHolder(n.Type) {
		return
	}
	delete(n.Props, "items")
	list := anyList(raw)
	limit := defaultMaxItems
	if l != nil {
		limit = l.MaxItems
		if pageScroller(n.Type) {
			limit = l.MaxGridItems
		}
	}
	if n.Type == HeroBanner {
		limit = min(limit, maxHeroItems)
	}
	if total := len(list); total > limit {
		list = list[:limit]
		n.Props["more"], n.Props["total"] = true, total
	}
	action, _ := n.Props["itemAction"].(string)
	if action == "" {
		action = "openItem"
	}
	if _, defined := doc.Actions[action]; !defined {
		action = ""
	}
	child := Card
	if n.Type == List {
		child = ListItem
	}
	seen := map[string]bool{}
	kids := make([]Node, 0, len(list))
	for i, v := range list {
		it, ok := v.(map[string]any)
		if !ok {
			continue
		}
		rawID := stringify(it["id"])
		if rawID == "" {
			rawID = stringify(it["itemId"])
		}
		id := safeID(rawID)
		if id == "" || seen[id] {
			id = "n" + strconv.Itoa(i)
		}
		seen[id] = true
		c := Node{Type: child, ID: clipID(n.ID + ":" + id), Props: map[string]any{}, Action: action}
		for _, k := range itemProps {
			if val, ok := it[k]; ok && val != nil && val != "" {
				c.Props[k] = cloneAny(val)
			}
		}
		if rawID != "" {
			c.Props["itemId"] = rawID
		}
		if _, ok := c.Props["title"]; !ok {
			c.Props["title"] = ""
		}
		if mod, _ := it["module"].(string); mod != "" && validModuleID(mod) {
			c.Style = &Style{Module: mod}
		}
		kids = append(kids, c)
	}
	n.Children = kids
	if len(kids) == 0 {
		n.Children = nil
	}
}

// maxDataSections bounds the rows one "sections" prop can make, before the
// device's own limit is applied.
const maxDataSections = 64

// expandSections turns a Stack's "sections" prop (a list of {id, title,
// subtitle, cardShape, seeAll, items}) into one Shelf each, for screens whose
// rows depend on the data: a row per library on Home, per kind in Search.
func expandSections(n *Node) {
	raw, ok := n.Props["sections"]
	if !ok || n.Type != Stack {
		return
	}
	delete(n.Props, "sections")
	seen := map[string]bool{}
	for i, v := range anyList(raw) {
		sec, ok := v.(map[string]any)
		if !ok || i >= maxDataSections {
			continue
		}
		id := safeID(stringify(sec["id"]))
		if id == "" || seen[id] {
			id = "s" + strconv.Itoa(i)
		}
		seen[id] = true
		row := Node{Type: Shelf, ID: clipID(n.ID + ":" + id), Props: map[string]any{"title": stringify(sec["title"])}}
		for _, k := range []string{"subtitle", "cardShape", "seeAll", "items", "itemAction"} {
			if val, ok := sec[k]; ok && val != nil && val != "" {
				row.Props[k] = cloneAny(val)
			}
		}
		if mod, _ := sec["module"].(string); mod != "" && validModuleID(mod) {
			row.Style = &Style{Module: mod}
		}
		n.Children = append(n.Children, row)
	}
}

func anyList(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	case []map[string]any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = x[i]
		}
		return out
	}
	return nil
}

// safeID keeps the characters a node ID may have.
func safeID(s string) string {
	var b strings.Builder
	for i := 0; i < len(s) && b.Len() < 64; i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '.', c == '-':
			b.WriteByte(c)
		}
	}
	return b.String()
}

func clipID(s string) string {
	if len(s) > MaxIDLength {
		return s[:MaxIDLength]
	}
	return s
}

// ---- fitting a rendered tree to a device ----

// fit applies the device's rules to a tree whose conditions and bindings are
// already resolved. It never fails: what doesn't fit is left out.
func fit(doc *Document, l *Layout, catalog int) {
	// A client built against an older catalogue gets the type it knows.
	if catalog > 0 {
		degrade(&doc.Root, catalog)
	}
	if l != nil && !l.ExternalLinks {
		dropExternal(doc)
	}
	if l != nil {
		// Rows past the device's limit are left out, last first.
		sections := 0
		l.Truncated = capSections(&doc.Root, l.MaxSections, &sections)
		stampImages(&doc.Root, l, "")
		setColumns(&doc.Root, l)
	}
	stampFocus(doc)
}

func degrade(n *Node, catalog int) {
	if spec, ok := Spec(n.Type); ok && spec.Since > catalog {
		n.Type = spec.Fallback
		if n.Type == "" {
			n.Type = Stack
		}
	}
	for i := range n.Children {
		degrade(&n.Children[i], catalog)
	}
}

func dropExternal(doc *Document) {
	ext := map[string]bool{}
	for name, a := range doc.Actions {
		if a.Type == External {
			ext[name] = true
			delete(doc.Actions, name)
		}
	}
	if len(ext) == 0 {
		return
	}
	var walk func(n *Node)
	walk = func(n *Node) {
		kept := n.Children[:0]
		for _, c := range n.Children {
			if ext[c.Action] {
				continue
			}
			walk(&c)
			kept = append(kept, c)
		}
		n.Children = kept
		if len(kept) == 0 {
			n.Children = nil
		}
	}
	walk(&doc.Root)
	if ext[doc.Root.Action] {
		doc.Root.Action = ""
	}
}

// capSections counts the rows of a page (through plain Stacks) and drops those
// past max. It reports whether any were dropped.
func capSections(n *Node, max int, count *int) bool {
	dropped := false
	kept := n.Children[:0]
	for _, c := range n.Children {
		switch {
		case rowScroller(c.Type) || pageScroller(c.Type):
			if *count >= max {
				dropped = true
				continue
			}
			*count++
		case c.Type == Stack:
			if capSections(&c, max, count) {
				dropped = true
			}
		}
		kept = append(kept, c)
	}
	n.Children = kept
	if len(kept) == 0 {
		n.Children = nil
	}
	return dropped
}

func setColumns(n *Node, l *Layout) {
	if n.Type == Grid && l.GridColumns > 0 {
		if n.Style == nil {
			n.Style = &Style{}
		}
		n.Style.Columns = l.GridColumns
	}
	for i := range n.Children {
		setColumns(&n.Children[i], l)
	}
}

// stampImages gives every node with a picture from this server an "image"
// prop: the address with the width to ask for, and the size it is shown at,
// so an Apple TV HD never decodes a 4K poster for a 250-point card.
func stampImages(n *Node, l *Layout, shape string) {
	if s, _ := n.Props["cardShape"].(string); s != "" {
		shape = s
	}
	own := shape
	if s, _ := n.Props["shape"].(string); s != "" {
		own = s
	}
	if own == "" {
		own = "poster"
	}
	for prop, sh := range map[string]string{"imageUrl": own, "backdropUrl": "hero"} {
		u, _ := n.Props[prop].(string)
		size, ok := l.Images[sh]
		if !ok || !strings.HasPrefix(u, "/api/") {
			continue
		}
		sep := "?"
		if strings.Contains(u, "?") {
			sep = "&"
		}
		key := "image"
		if prop == "backdropUrl" {
			key = "backdrop"
		}
		n.Props[key] = map[string]any{"url": u + sep + "w=" + strconv.Itoa(size.Width), "width": size.Width, "height": size.Height, "aspect": size.Aspect}
	}
	for i := range n.Children {
		stampImages(&n.Children[i], l, shape)
	}
}

// stampFocus numbers every interactive node in reading order, says which
// group each is in, keeps the author's default and guides when the nodes they
// name are still on the screen, and otherwise starts focus on the first
// content node (the first that isn't in a Toolbar), or the first of all.
func stampFocus(doc *Document) {
	ids := map[string]bool{}
	var collect func(n *Node)
	collect = func(n *Node) {
		if Interactive(n) {
			ids[n.ID] = true
		}
		for i := range n.Children {
			collect(&n.Children[i])
		}
	}
	collect(&doc.Root)

	order := 0
	var first, firstContent, chosen *Node
	var walk func(n *Node, group string, inBar bool)
	walk = func(n *Node, group string, inBar bool) {
		if focusGroup(n.Type) {
			group = n.ID
			inBar = n.Type == Toolbar
		}
		if Interactive(n) {
			order++
			f := Focus{}
			if n.Focus != nil {
				f = *n.Focus
			}
			f.Focusable, f.Order, f.Group = true, order, group
			for _, g := range []*string{&f.Up, &f.Down, &f.Left, &f.Right} {
				if *g != "" && !ids[*g] {
					*g = ""
				}
			}
			if f.Default && chosen == nil {
				chosen = n
			}
			f.Default = false
			n.Focus = &f
			if first == nil {
				first = n
			}
			if firstContent == nil && !inBar {
				firstContent = n
			}
		} else {
			n.Focus = nil
		}
		for i := range n.Children {
			walk(&n.Children[i], group, inBar)
		}
	}
	walk(&doc.Root, "", false)
	for _, n := range []*Node{chosen, firstContent, first} {
		if n != nil {
			n.Focus.Default = true
			return
		}
	}
}
