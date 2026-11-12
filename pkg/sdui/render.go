// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ErrNotFound is what a Resolver answers when the page's subject (the
// library, the title) isn't there or isn't this profile's to see, and what
// Render answers when a page can't be shown because of it.
var ErrNotFound = errors.New("sdui: not found")

// Resolver is where a page's data comes from. The server's own resolver
// reads the library with the profile's limits applied; the conformance kit's
// answers from canned data. A resolver belongs to one request.
type Resolver interface {
	// Items answers a data source: up to req.Limit items from req.Offset.
	Items(req SourceRequest) (SourceResult, error)
	// Each lists what a section is repeated over.
	Each(over string, kinds []string, limit int) ([]Each, error)
	// Subject is the title a detail page is about. ErrNotFound when there is
	// none or the profile may not see it.
	Subject() (*Item, error)
	// Seasons lists a show's seasons as items (Kind season, Season, Title,
	// Count, Unwatched).
	Seasons(show string) ([]Item, error)
	// Facets gives the choices of each named filter for a library.
	Facets(library string, fields []string) ([]Facet, error)
}

// SourceRequest is one question to a Resolver.
type SourceRequest struct {
	// Source is the source with every $variable replaced.
	Source Source
	// Filter holds the page's filter and sort parameters, for a filterable
	// source.
	Filter map[string]string
	// Offset and Limit page the answer.
	Offset, Limit int
}

// SourceResult is a Resolver's answer.
type SourceResult struct {
	Items []Item
	// More says there are items after these; Total how many there are in
	// all, -1 when that isn't known.
	More  bool
	Total int
	// Selected is the season an "episodes" source chose when none was named.
	Selected string
}

// Each is one thing a section is repeated over.
type Each struct {
	// Key replaces $each in the section's source and ends the copy's ID.
	Key string
	// Label replaces {each} in its title.
	Label string
	// Kind is the library's kind, for Repeat.Kinds.
	Kind string
	// ID ends the copy's ID in place of Key, when Key is text (a series' name) that two
	// copies could share once cut to what an ID may hold. "" uses Key.
	ID string
}

// copyID is the ID of a repeated section's copy for e.
func copyID(section string, e *Each) string {
	k := e.Key
	if e.ID != "" {
		k = e.ID
	}
	return clipID(section + "-" + safeID(k))
}

// RenderContext is who a page is rendered for, on what, with what.
type RenderContext struct {
	// Platform is web, tvos, ios, android, androidtv or desktop.
	Platform string
	// ProfileKind is adult, kids or guest; Admin whether it administers.
	ProfileKind string
	Admin       bool
	// UserName and ServerName fill {user} and {server}.
	UserName, ServerName string
	// Features are the server's; Modules those installed and on; Cannot the
	// parental feature switches that are off for the profile; LibraryKinds
	// the kinds of library it can open; Capabilities what the client has.
	Features, Modules, Cannot, LibraryKinds, Capabilities []string
	// Flags are things the server knows: see Flags.
	Flags map[string]bool
	// Viewport is the window in points, zero when unknown. Locale is a
	// language tag. Now is the server's clock; the zero value means now.
	Viewport Viewport
	Locale   string
	Now      time.Time
	// Profile is the device profile. nil is the web profile.
	Profile *Layout
	// NativeViews are the views the client has for NativeView.
	NativeViews []string
	// Vars are what the page was opened with: library, libraryKind,
	// libraryName, item, show, season, collection, artist, album, query.
	Vars map[string]string
	// Params are the page's filter, sort and tab parameters.
	Params map[string]string
	// Scopes are the theme scopes' colour overrides: scope → token → colour.
	Scopes map[string]map[string]string
	// Personal is the profile's own arrangement of the page; CanPersonalise
	// whether it may have one.
	Personal       *Personal
	CanPersonalise bool
	// ScreenTime, when the profile's time is over, is sent with the document.
	ScreenTime *ScreenTime
	// Resolver answers the page's data sources, the modules' included.
	Resolver Resolver
	// UI are the modules in use: their components are expanded before the
	// page is drawn, and their sources are asked like any other. nil: none.
	UI *ModuleSet
	// Layout is the layout option the page is drawn for (LayoutByID): its changes to
	// pages are made here, and its shell is sent in document.layout.shell. nil: none,
	// the page as written.
	Layout *LayoutSpec
	// BasePath is where the SDUI routes are: "/api/v1/sdui".
	BasePath string
	// Source and ConfigRevision are stamped into Meta.
	Source         string
	ConfigRevision int64
}

// renderer carries one render's state.
type renderer struct {
	ctx    *RenderContext
	page   *Page
	l      *Layout
	eager  int  // data sections that arrived filled so far
	asked  int  // data sections whose source was asked, filled or empty
	clock  bool // a condition looked at the time
	ids    map[string]bool
	totals map[string]int // section → total its source reported
	err    error
}

// instance is one section to draw: the authored section and, for a copy of a
// repeated one, what it is a copy for.
type instance struct {
	sec  *Section
	id   string
	each *Each
}

// Render draws page for ctx. It chooses the variant, drops the sections whose
// conditions don't hold, makes the copies of repeated sections, applies the
// profile's own arrangement, asks the Resolver for each section's items
// within the device's budget, and stamps focus and the ETag.
//
// The page should have passed ValidatePage; Render never panics on one that
// hasn't, but leaves out what it can't make sense of.
func Render(page Page, ctx *RenderContext) (Document, error) {
	page = zoned(styled(ctx.UI.ExpandPage(page), ctx.Layout), ctx.deviceFormFactor())
	r := newRenderer(&page, ctx)
	sections, variant := r.sections()
	top := r.expand(sections, true)
	top, info := r.personalise(top)
	// A title's page on a phone or a television offers no Customise: there it is a row
	// under the title nobody comes for. The profile's arrangement, if it has one, still
	// applies.
	if page.Kind == "detail" && (r.l.FormFactor == FormPhone || r.l.FormFactor == FormTV) {
		info = nil
	}

	root := Node{Type: "Page", ID: "page", Props: map[string]any{"title": r.fill(page.Title, nil, nil)}}
	if r.l.Density != "comfortable" {
		root.Props["density"] = r.l.Density
	}
	if len(top) > r.l.Budget.MaxSections {
		top = top[:r.l.Budget.MaxSections]
		r.l.Truncated = true
	}
	shown := map[string]bool{}
	for _, in := range top {
		n, ok := r.build(in, 1)
		if errors.Is(r.err, ErrNotFound) {
			return Document{}, ErrNotFound
		}
		if ok {
			root.Children = append(root.Children, n...)
			shown[in.id] = true
		}
	}
	r.finishFilters(&root)
	r.finishDocks(&root)
	stampFocus(&root)
	if info != nil {
		for i := range info.Sections {
			if s := &info.Sections[i]; !s.Hidden && !shown[s.ID] {
				s.Empty = true
			}
		}
	}

	doc := Document{Version: SchemaVersion, Catalog: CatalogRevision, Layout: r.l, Root: root, Personal: info,
		Page: PageInfo{ID: page.ID, Kind: page.Kind, Title: r.fill(page.Title, nil, nil), Icon: page.Icon, Route: r.route(), Variant: variant, Params: r.pageParams()},
		Meta: Meta{Source: ctx.Source, ConfigRevision: ctx.ConfigRevision, TTLSeconds: r.ttl()}}
	if page.Scope != "" {
		doc.Theme = &ThemeScope{Scope: page.Scope, Tokens: copyTokens(ctx.Scopes[page.Scope])}
	}
	doc.ScreenTime = ctx.ScreenTime
	doc.Meta.GeneratedAt = ctx.Now.UTC()
	doc.Meta.ETag = ETag(doc)
	return doc, nil
}

func newRenderer(page *Page, ctx *RenderContext) *renderer {
	if ctx.Now.IsZero() {
		ctx.Now = time.Now()
	}
	if ctx.Profile == nil {
		ctx.Profile = ProfileFor(FormWeb, "")
	}
	if ctx.BasePath == "" {
		ctx.BasePath = "/api/v1/sdui"
	}
	l := ctx.Profile.WithViewport(ctx.Viewport)
	if d := layoutDensity(ctx.Layout); d != "" {
		l = l.WithDensity(d)
	}
	if ctx.Personal != nil && ctx.CanPersonalise && allows(page, "density") && ctx.Personal.Density != "" {
		l = l.WithDensity(ctx.Personal.Density)
	}
	if ctx.Layout != nil {
		l = l.clone()
		l.Shell = ShellFor(ctx.Layout.ID, l.FormFactor)
	}
	if contains(ctx.Capabilities, "reducedMotion") {
		l = l.clone()
		l.ReducedMotion = true
	}
	return &renderer{ctx: ctx, page: page, l: l.clone(), ids: map[string]bool{}, totals: map[string]int{}}
}

// sections is the list of sections in force: the first variant whose
// condition holds, or the page's own.
func (r *renderer) sections() ([]Section, string) {
	for i := range r.page.Variants {
		v := &r.page.Variants[i]
		r.clock = r.clock || usesClock(v.When)
		if Evaluate(v.When, r.ctx) {
			return v.Sections, v.ID
		}
	}
	return r.page.Sections, ""
}

// expand drops the sections whose conditions don't hold and makes the copies
// of repeated ones.
func (r *renderer) expand(sections []Section, repeats bool) []instance {
	var out []instance
	for i := range sections {
		s := &sections[i]
		r.clock = r.clock || usesClock(s.When)
		if !Evaluate(s.When, r.ctx) {
			continue
		}
		if s.Repeat == nil || !repeats || r.ctx.Resolver == nil {
			if s.Repeat == nil {
				out = append(out, instance{sec: s, id: s.ID})
			}
			continue
		}
		limit := r.l.Budget.Repeats
		if s.Repeat.Limit > 0 && s.Repeat.Limit < limit {
			limit = s.Repeat.Limit
		}
		each, err := r.ctx.Resolver.Each(s.Repeat.Over, s.Repeat.Kinds, limit)
		if err != nil {
			continue
		}
		for j := range each {
			if j >= limit {
				break
			}
			e := each[j]
			out = append(out, instance{sec: s, id: copyID(s.ID, &e), each: &e})
		}
	}
	return out
}

func allows(p *Page, kind string) bool {
	if p.Personal == nil {
		return true
	}
	return contains(p.Personal.Allow, kind)
}

// personalise applies the profile's own arrangement to the top-level
// sections: hidden ones go, pinned ones come first in the profile's order of
// pinning, then the rest in the profile's order with sections it never placed
// keeping the page's order after them. A locked section is never hidden and
// never moves: it stays where the page put it relative to the other locked
// sections, and the unlocked ones flow around it.
func (r *renderer) personalise(top []instance) ([]instance, *PersonalInfo) {
	ctx, p := r.ctx, r.ctx.Personal
	if !ctx.CanPersonalise && p == nil && len(top) == 0 {
		return top, nil
	}
	info := &PersonalInfo{Editable: ctx.CanPersonalise, URL: ctx.BasePath + "/pages/" + r.page.ID + "/personal"}
	if ctx.CanPersonalise {
		if r.page.Personal == nil {
			info.Allow = append([]string(nil), PersonalKinds...)
		} else {
			for _, k := range PersonalKinds {
				if contains(r.page.Personal.Allow, k) {
					info.Allow = append(info.Allow, k)
				}
			}
		}
	}
	if !ctx.CanPersonalise || p == nil {
		p = &Personal{}
	}
	info.Density = p.Density
	hidden, pinned := map[string]bool{}, map[string]int{}
	if contains(info.Allow, "hide") {
		for _, id := range p.Hidden {
			hidden[id] = true
		}
	}
	if contains(info.Allow, "pin") {
		for i, id := range p.Pinned {
			if _, dup := pinned[id]; !dup {
				pinned[id] = i
			}
		}
	}
	place := map[string]int{}
	if contains(info.Allow, "reorder") {
		for i, id := range p.Order {
			if _, dup := place[id]; !dup {
				place[id] = i
			}
		}
	}
	// A copy of a repeated section the profile never placed sits with its
	// siblings: it takes the place of the section it is a copy of.
	rank := func(in instance, i int) (int, int, int) {
		if n, ok := pinned[in.id]; ok {
			return 0, n, i
		}
		if n, ok := place[in.id]; ok {
			return 1, n, i
		}
		if in.each != nil {
			if n, ok := place[in.sec.ID]; ok {
				return 1, n, i
			}
		}
		return 2, i, i
	}
	type slot struct {
		in      instance
		a, b, c int
	}
	var free []slot
	for i, in := range top {
		if !in.sec.Locked {
			a, b, c := rank(in, i)
			free = append(free, slot{in, a, b, c})
		}
	}
	sort.SliceStable(free, func(i, j int) bool {
		x, y := free[i], free[j]
		if x.a != y.a {
			return x.a < y.a
		}
		if x.b != y.b {
			return x.b < y.b
		}
		return x.c < y.c
	})
	out := make([]instance, 0, len(top))
	next := 0
	for _, in := range top {
		cur := in
		if !in.sec.Locked {
			cur = free[next].in
			next++
		}
		ps := PersonalSection{ID: cur.id, Title: r.sectionTitle(cur), Locked: cur.sec.Locked}
		_, ps.Pinned = pinned[cur.id]
		ps.Pinned = ps.Pinned && !cur.sec.Locked
		if hidden[cur.id] && !cur.sec.Locked {
			ps.Hidden = true
		} else {
			out = append(out, cur)
		}
		info.Sections = append(info.Sections, ps)
	}
	return out, info
}

// sectionTitle names a section in a "Customise" list: its title, else its
// source's usual title, else its component's label.
func (r *renderer) sectionTitle(in instance) string {
	if t, _ := in.sec.Props["title"].(string); t != "" {
		return r.fill(t, in.each, in.sec.Source)
	}
	if spec, ok := r.sourceSpec(in.sec.Source.Type()); ok && spec.Title != "" {
		return r.fill(spec.Title, in.each, in.sec.Source)
	}
	if spec, ok := Component(in.sec.Component); ok {
		return spec.Label
	}
	return in.sec.Component
}

// ttl is how long a client may keep the page: the page's own, else by kind,
// and never long when a condition looked at the clock.
func (r *renderer) ttl() int {
	ttl := r.page.TTLSeconds
	if ttl == 0 {
		switch r.page.Kind {
		case "home", "livetv":
			ttl = 60
		case "search":
			ttl = 0
		default:
			ttl = 300
		}
	}
	if r.clock && (ttl == 0 || ttl > 60) {
		ttl = 60
	}
	return ttl
}

// route is the page's place in the app.
func (r *renderer) route() string {
	v := r.ctx.Vars
	switch r.page.Kind {
	case "home":
		return "/"
	case "library":
		return "/library/" + v["library"]
	case "detail":
		switch {
		case v["collection"] != "":
			return "/collections/" + v["collection"]
		case v["album"] != "":
			return "/album/" + v["library"] + "?artist=" + url.QueryEscape(v["artist"]) + "&album=" + url.QueryEscape(v["album"])
		case v["artist"] != "" && v["item"] == "" && v["show"] == "":
			return "/artist/" + v["library"] + "?name=" + url.QueryEscape(v["artist"])
		case v["item"] != "":
			return "/item/" + v["item"]
		case v["show"] != "":
			return "/show/" + v["show"]
		}
	case "custom":
		return "/page/" + r.page.ID
	}
	for _, k := range Catalog().PageKinds {
		if k.Kind == r.page.Kind {
			return k.Route
		}
	}
	return "/page/" + r.page.ID
}

// pageParams are what the page was opened with, as a client sends them back.
func (r *renderer) pageParams() map[string]string {
	out := map[string]string{}
	for _, k := range []string{"library", "item", "show", "season", "collection", "artist", "album"} {
		if v := r.ctx.Vars[k]; v != "" {
			out[k] = v
		}
	}
	if q := r.ctx.Vars["query"]; q != "" {
		out["q"] = q
	}
	for k, v := range r.ctx.Params {
		if v != "" {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// query is pageParams as a query string, keys sorted, for cursor URLs.
func (r *renderer) query(extra ...string) string {
	q := url.Values{}
	for k, v := range r.pageParams() {
		if k != "tab" {
			q.Set(k, v)
		}
	}
	for i := 0; i+1 < len(extra); i += 2 {
		if extra[i+1] != "" {
			q.Set(extra[i], extra[i+1])
		}
	}
	return q.Encode()
}

func (r *renderer) moreURL(id string, offset int, extra ...string) string {
	return r.ctx.BasePath + "/pages/" + r.page.ID + "/sections/" + url.PathEscape(id) + "/items?" + r.query(append(extra, "cursor", strconv.Itoa(offset))...)
}

// fill replaces the placeholders in authored text.
func (r *renderer) fill(s string, each *Each, src Source) string {
	if !strings.Contains(s, "{") {
		return s
	}
	v := r.ctx.Vars
	pairs := []string{"{user}", r.ctx.UserName, "{server}", r.ctx.ServerName, "{greeting}", greeting(r.ctx.Now),
		"{library}", v["libraryName"], "{title}", v["title"], "{query}", v["query"]}
	if each != nil {
		pairs = append(pairs, "{each}", each.Label)
	} else {
		pairs = append(pairs, "{each}", "")
	}
	for _, k := range []string{"genre", "studio", "tag", "network"} {
		pairs = append(pairs, "{"+k+"}", r.resolveVar(src.Str(k), each))
	}
	return strings.TrimSpace(strings.NewReplacer(pairs...).Replace(s))
}

func greeting(t time.Time) string {
	switch h := t.Hour(); {
	case h < 5:
		return "Good evening"
	case h < 12:
		return "Good morning"
	case h < 18:
		return "Good afternoon"
	}
	return "Good evening"
}

// resolveVar replaces a "$name" value with what it stands for.
func (r *renderer) resolveVar(s string, each *Each) string {
	if !strings.HasPrefix(s, "$") {
		return s
	}
	if s == "$each" {
		if each != nil {
			return each.Key
		}
		return ""
	}
	return r.ctx.Vars[s[1:]]
}

// resolved is the source with every $variable replaced. ok is false when a
// variable the source needs has nothing behind it: the section is left out.
func (r *renderer) resolved(src Source, each *Each) (Source, bool) {
	out := make(Source, len(src))
	spec, _ := r.sourceSpec(src.Type())
	for k, v := range src {
		s, isStr := v.(string)
		if isStr && strings.HasPrefix(s, "$") {
			val := r.resolveVar(s, each)
			if val == "" {
				if p := specParam(spec, k); p != nil && p.Required {
					return nil, false
				}
				continue
			}
			out[k] = val
			continue
		}
		out[k] = v
	}
	return out, true
}

func specParam(s *SourceSpec, name string) *PropSpec {
	if s == nil {
		return nil
	}
	return s.param(name)
}

// filterKeys are the page parameters a filterable source takes.
// The ones after "tag" are the rest of a client's own filters and saved views (the same as
// GET /browse takes); a FilterBar offers only the first ones.
var filterKeys = []string{"sort", "desc", "genre", "decade", "year", "content", "resolution", "watched", "studio", "network", "tag", "service",
	"series", "kind", "disc", "minRating", "audio", "subtitle", "format", "edition", "search", "seed"}

func (r *renderer) filter(src Source) map[string]string {
	if !src.Bool("filterable") {
		return nil
	}
	out := map[string]string{}
	for _, k := range filterKeys {
		if v := r.ctx.Params[k]; v != "" {
			out[k] = v
		}
	}
	return out
}

func (r *renderer) uniqueID(id string) string {
	id = clipID(id)
	for n := 2; r.ids[id]; n++ {
		id = clipID(id) + "~" + strconv.Itoa(n)
	}
	r.ids[id] = true
	return id
}

func (r *renderer) scope(n *Node, scope string) {
	if scope == "" {
		return
	}
	n.Scope = scope
	n.Tokens = copyTokens(r.ctx.Scopes[scope])
}

func copyTokens(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func propStr(p map[string]any, k, def string) string {
	if s, ok := p[k].(string); ok && s != "" {
		return s
	}
	return def
}

func propBool(p map[string]any, k string, def bool) bool {
	if b, ok := p[k].(bool); ok {
		return b
	}
	return def
}

func propInt(p map[string]any, k string, def int) int {
	switch v := p[k].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return def
}

func propList(p map[string]any, k string, def []string) []string {
	switch v := p[k].(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return def
}

// defaultList is a list property's default from the catalogue.
func defaultList(component, prop string) []string {
	if c, ok := Component(component); ok {
		if p := c.prop(prop); p != nil {
			if d, ok := p.Default.([]string); ok {
				return d
			}
		}
	}
	return nil
}

// build draws one section. It answers the nodes the section becomes (one,
// except for a container replaced by its children) and whether it is shown.
func (r *renderer) build(in instance, depth int) ([]Node, bool) {
	s := in.sec
	if depth > MaxDepth {
		return nil, false
	}
	if _, ok := Component(s.Component); !ok {
		return nil, false
	}
	n := Node{Type: s.Component, ID: r.uniqueID(in.id), Props: map[string]any{}}
	r.scope(&n, s.Scope)
	p := s.Props
	ok := true
	switch s.Component {
	case "Shelf", "Grid":
		ok = r.collection(&n, in)
	case "List":
		if s.Source.Type() != "" {
			ok = r.collection(&n, in)
		} else {
			if t := r.fill(propStr(p, "title", ""), in.each, nil); t != "" {
				n.Props["title"] = t
			}
			n.Children = r.children(in, depth)
			ok = len(n.Children) > 0
		}
		if propBool(p, "dense", false) {
			n.Props["dense"] = true
		}
	case "Hero":
		ok = r.hero(&n, in)
	case "SeasonList":
		ok = r.seasons(&n, in)
	case "TrackList":
		ok = r.tracks(&n, in)
	case "Banner":
		ok = r.banner(&n, in)
	case "DetailHeader":
		ok = r.header(&n, in)
	case "Tabs":
		ok = r.tabs(&n, in, depth)
	case "FilterBar":
		ok = r.filterBar(&n, in)
	case "Dock":
		ok = r.dock(&n, in, depth)
	case "KeyLegend":
		ok = r.keyLegend(&n, in)
	case "Text":
		n.Props["text"] = r.fill(propStr(p, "text", ""), in.each, nil)
		copyProps(n.Props, p, "format", "role", "align", "maxLines")
		ok = n.Props["text"] != ""
	case "Notice":
		copyProps(n.Props, p, "tone", "icon")
		n.Props["text"] = r.fill(propStr(p, "text", ""), in.each, nil)
		if t := r.fill(propStr(p, "title", ""), in.each, nil); t != "" {
			n.Props["title"] = t
		}
		if a := r.link(p); a != nil && propStr(p, "buttonLabel", "") != "" {
			n.Props["buttonLabel"] = propStr(p, "buttonLabel", "")
			n.Action = a
		}
	case "Button":
		n.Props["label"] = r.fill(propStr(p, "label", ""), in.each, nil)
		copyProps(n.Props, p, "icon", "variant")
		n.Action = r.link(p)
		ok = n.Action != nil
	case "ListItem":
		n.Props["title"] = r.fill(propStr(p, "title", ""), in.each, nil)
		copyProps(n.Props, p, "subtitle", "icon", "trailing")
		n.Action = r.link(p)
	case "Image":
		src := propStr(p, "src", "")
		aspect := propStr(p, "aspect", "16/9")
		copyProps(n.Props, p, "alt", "fit", "rounded")
		n.Props["aspect"] = aspect
		n.Props["image"] = r.picture(src, min(960, max(r.l.Canvas.Width, 430)), aspect, "image")
		n.Action = r.link(p)
		ok = src != ""
	case "SearchBox":
		copyProps(n.Props, p, "placeholder", "autofocus")
		if q := r.ctx.Vars["query"]; q != "" {
			n.Props["query"] = q
		}
		n.Action = &Action{Type: ActNavigate, To: "/search"}
	case "Divider":
		copyProps(n.Props, p, "inset")
	case "Spacer":
		n.Props["size"] = propStr(p, "size", "m")
	case "Stack", "Split", "Tab":
		copyProps(n.Props, p, "direction", "gap", "align", "wrap", "ratio", "stackBelow")
		n.Children = r.children(in, depth)
		ok = len(n.Children) > 0
	case "NativeView":
		view := propStr(p, "view", "")
		if !contains(r.ctx.NativeViews, view) {
			// The client has no view of its own for it: the page's own sections stand in.
			n.Type = "Stack"
			n.Props = map[string]any{}
			n.Children = r.children(in, depth)
			ok = len(n.Children) > 0
			break
		}
		n.Props["view"] = view
		if lib := r.resolveVar(propAny(p, "library"), in.each); lib != "" {
			n.Props["library"] = lib
		}
	default:
		return nil, false
	}
	if !ok {
		delete(r.ids, n.ID)
		return nil, false
	}
	if len(n.Props) == 0 {
		n.Props = nil
	}
	return []Node{n}, true
}

func propAny(p map[string]any, k string) string { return Source(p).Str(k) }

func copyProps(dst, src map[string]any, keys ...string) {
	for _, k := range keys {
		if v, ok := src[k]; ok && v != nil && v != "" {
			dst[k] = v
		}
	}
}

// children draws the sections inside a container.
func (r *renderer) children(in instance, depth int) []Node {
	var out []Node
	for _, c := range r.expand(in.sec.Children, true) {
		if c.each == nil {
			c.each = in.each
		}
		if n, ok := r.build(c, depth+1); ok {
			out = append(out, n...)
		}
	}
	return out
}

// link is the action of a button-like section: to, native or url.
func (r *renderer) link(p map[string]any) *Action {
	if to := propStr(p, "to", ""); to != "" {
		return &Action{Type: ActNavigate, To: to}
	}
	if s := propStr(p, "native", ""); s != "" {
		return &Action{Type: ActNative, To: s}
	}
	if u := propStr(p, "url", ""); u != "" && r.l.ExternalLinks {
		return &Action{Type: ActExternal, URL: u}
	}
	return nil
}

// picture is the props.image of a node: the address with the width to ask
// for, the size it is asked at and what kind of picture it is.
func (r *renderer) picture(src string, points int, aspect, kind string) map[string]any {
	if src == "" {
		return nil
	}
	return r.pictureAt(src, r.l.ImageWidth(points), aspect, kind)
}

// backdrop is a 16/9 picture spanning a window this wide: never more than twice its width.
func (r *renderer) backdrop(src string, points int) map[string]any {
	if src == "" {
		return nil
	}
	return r.pictureAt(src, r.l.BackdropWidth(points), "16/9", "backdrop")
}

func (r *renderer) pictureAt(src string, w int, aspect, kind string) map[string]any {
	u := src
	if strings.HasPrefix(src, "/api/") {
		sep := "?"
		if strings.Contains(src, "?") {
			sep = "&"
		}
		u = src + sep + "w=" + strconv.Itoa(w)
	}
	return map[string]any{"url": u, "width": w, "height": aspectHeight(w, aspect), "aspect": aspect, "kind": kind}
}

// limits works out a collection's first page: how many items to ask for.
func (r *renderer) firstPage(component string, p map[string]any, src Source) int {
	n := r.l.Budget.ShelfItems
	switch component {
	case "Grid":
		n = r.l.Budget.GridItems
	case "List", "TrackList", "SeasonList":
		n = r.l.Budget.ListItems
	}
	if m := propInt(p, "maxItems", 0); m > 0 && m < n {
		n = m
	}
	if lim := src.Int("limit"); lim > 0 && lim < n {
		n = lim
	}
	return n
}

// ask puts a question to the Resolver.
func (r *renderer) ask(src Source, each *Each, offset, limit int) (SourceResult, Source, error) {
	if r.ctx.Resolver == nil {
		return SourceResult{}, nil, errors.New("no resolver")
	}
	rs, ok := r.resolved(src, each)
	if !ok {
		return SourceResult{Total: 0}, rs, nil
	}
	// A source of a module that isn't in use has nothing in it: the section
	// is left out, as an empty one is.
	if t := src.Type(); t != "" {
		if _, known := r.sourceSpec(t); !known {
			return SourceResult{Total: 0}, rs, nil
		}
	}
	if lim := src.Int("limit"); lim > 0 {
		if offset >= lim {
			return SourceResult{Total: lim}, rs, nil
		}
		limit = min(limit, lim-offset)
	}
	res, err := r.ctx.Resolver.Items(SourceRequest{Source: rs, Filter: r.filter(src), Offset: offset, Limit: limit})
	if err != nil {
		return res, rs, err
	}
	if len(res.Items) > limit {
		res.Items, res.More = res.Items[:limit], true
	}
	if lim := src.Int("limit"); lim > 0 && offset+len(res.Items) >= lim {
		res.More = false
	}
	return res, rs, nil
}

// cardStyle is the style a section's cards are drawn in: its own choice, or
// its source's usual style.
func (r *renderer) cardStyleFor(p map[string]any, src Source, each *Each) string {
	style := propStr(p, "cardStyle", "auto")
	if _, ok := CardStyle(style); ok {
		return style
	}
	spec, ok := r.sourceSpec(src.Type())
	if !ok {
		return "poster"
	}
	// A source that gives whatever a library holds draws it in the style of what that
	// library holds: an album's cover square, a book's cover, a game's box.
	if spec.CardStyle == "poster" {
		kind := ""
		switch lib := src.Str("library"); {
		case each != nil && lib == "$each":
			kind = each.Kind
		case lib == "$library":
			kind = r.ctx.Vars["libraryKind"]
		}
		if s := libraryCardStyle[kind]; s != "" {
			return s
		}
		// A layout with wide cards draws films' and shows' rows wide.
		if r.ctx.Layout != nil && r.ctx.Layout.Pages.LandscapeCards && overlaps(spec.Yields, landscapeKinds) {
			return "landscape"
		}
	}
	return spec.CardStyle
}

// libraryCardStyle is the card style for what a kind of library holds, where it isn't a
// poster.
var libraryCardStyle = map[string]string{"music": "album", "audiobooks": "album", "podcasts": "album", "books": "book", "games": "game", "photos": "photo"}

// collection fills a Shelf, Grid or List from its source.
func (r *renderer) collection(n *Node, in instance) bool {
	s, p := in.sec, in.sec.Props
	return r.collectionPage(n, in, 0, r.firstPage(s.Component, p, s.Source), true)
}

func (r *renderer) collectionPage(n *Node, in instance, offset, limit int, allowDefer bool) bool {
	s, p := in.sec, in.sec.Props
	style := r.cardStyleFor(p, s.Source, in.each)
	// A phone's row of libraries is a row of chips, each the library's icon and name: a
	// wide tile with an icon in it takes a screen's width for four words.
	if style == "tile" && s.Component == "Shelf" && s.Source.Type() == "libraries" && r.l.FormFactor == FormPhone {
		style = "chip"
	}
	size := propStr(p, "cardSize", "m")
	if r.ctx.Personal != nil && r.ctx.CanPersonalise && allows(r.page, "cardSize") {
		if ps := r.ctx.Personal.CardSize[in.id]; contains(CardSizes, ps) {
			size = ps
		}
	}
	title := r.fill(propStr(p, "title", ""), in.each, s.Source)
	if title != "" {
		n.Props["title"] = Truncate(title, r.l.TextLimits.Title)
	}
	if st := r.fill(propStr(p, "subtitle", ""), in.each, s.Source); st != "" {
		n.Props["subtitle"] = st
	}
	if s.Component != "List" {
		m := r.l.Cards[style]
		n.Props["cardStyle"], n.Props["cardSize"] = style, size
		card := map[string]any{"width": m.Width[size], "shape": m.Shape}
		if m.Aspect != "" {
			card["height"], card["aspect"] = aspectHeight(m.Width[size], m.Aspect), m.Aspect
		}
		n.Props["card"] = card
		if !propBool(p, "showTitles", true) {
			n.Props["showTitles"] = false
		}
		if s.Component == "Grid" {
			n.Props["columns"] = r.l.GridColumns(style, size)
		}
		if propBool(p, "numbered", false) {
			n.Props["numbered"] = true
		}
	}
	spec, _ := r.sourceSpec(s.Source.Type())
	// Past the device's budget of sections that arrive filled, a row is sent
	// empty with where to ask for it.
	// So is one that comes after many empty ones: a page of rows with nothing
	// in them must not cost a small server a query for every row.
	if allowDefer && (r.eager >= r.l.Budget.EagerSections || r.asked >= 3*r.l.Budget.EagerSections) && s.Component != "List" {
		n.State = "deferred"
		n.Props["more"] = map[string]any{"url": r.moreURL(n.ID, 0)}
		return true
	}
	r.asked++
	res, rs, err := r.ask(s.Source, in.each, offset, limit)
	if errors.Is(err, ErrNotFound) {
		r.err = err
		return false
	}
	if err != nil {
		n.State = "error"
		n.Props["more"] = map[string]any{"url": r.moreURL(n.ID, offset)}
		return true
	}
	if len(res.Items) == 0 && offset == 0 {
		if propStr(p, "empty", "hide") != "message" {
			return false
		}
		n.State = "empty"
		n.Props["emptyText"] = r.fill(propStr(p, "emptyText", "Nothing here yet"), in.each, s.Source)
		return true
	}
	r.eager++
	for i := range res.Items {
		it := &res.Items[i]
		if s.Component == "List" {
			n.Children = append(n.Children, r.listItem(n.ID, it))
			continue
		}
		c := r.card(n.ID, style, size, it, s.Source)
		if propBool(p, "numbered", false) {
			c.Props["rank"] = offset + i + 1
		}
		n.Children = append(n.Children, c)
	}
	if res.Total > 0 {
		n.Props["total"] = res.Total
		r.totals[in.id] = res.Total
	}
	if res.More && spec != nil && spec.Paged {
		n.Props["more"] = map[string]any{"url": r.moreURL(n.ID, offset+len(res.Items))}
	}
	if s.Component == "Shelf" {
		if all := r.seeAll(spec, rs, propStr(p, "seeAll", "auto"), res.More); all != "" {
			n.Props["seeAll"] = map[string]any{"route": all, "label": "See all"}
			if r.seeAllCard(p) {
				n.Props["seeAllCard"] = true
			}
		}
		if !propBool(p, "peek", true) {
			n.Props["peek"] = false
		}
	}
	return true
}

// seeAll is the route a row's "See all" opens, "" for none.
func (r *renderer) seeAll(spec *SourceSpec, rs Source, policy string, more bool) string {
	if spec == nil || spec.SeeAll == "" || policy == "never" || (policy != "always" && !more) {
		return ""
	}
	route := spec.SeeAll
	for strings.Contains(route, "{") {
		i := strings.Index(route, "{")
		j := strings.Index(route[i:], "}")
		if j < 0 {
			return ""
		}
		val := rs.Str(route[i+1 : i+j])
		if val == "" {
			return ""
		}
		if i > 0 && route[i-1] == '=' {
			val = url.QueryEscape(val)
		}
		route = route[:i] + val + route[i+j+1:]
	}
	return route
}

func (r *renderer) listItem(parent string, it *Item) Node {
	n := Node{Type: "ListItem", ID: r.uniqueID(parent + ":" + safeID(it.ID)), Props: map[string]any{"title": Truncate(it.Title, r.l.TextLimits.Title), "itemId": it.ID, "kind": it.Kind}}
	if it.Subtitle != "" {
		n.Props["subtitle"] = Truncate(it.Subtitle, r.l.TextLimits.Subtitle)
	}
	if it.Icon != "" {
		n.Props["icon"] = it.Icon
	}
	switch {
	case it.Count > 0:
		n.Props["trailing"] = strconv.Itoa(it.Count)
	case it.Kind == "track" && it.RuntimeSec > 0:
		n.Props["trailing"] = FormatRuntime(it.RuntimeSec)
	}
	// The picture keeps the item's own shape, at the row's height: a poster upright, a
	// film still wide, a cover square.
	switch {
	case slices.Contains([]string{"episode", "video", "recording", "channel"}, it.Kind) && firstImage(it, []string{"backdrop"}) != "":
		n.Props["image"] = r.picture(firstImage(it, []string{"backdrop"}), 100, "16/9", "backdrop")
	case slices.Contains([]string{"movie", "show", "season", "book", "game"}, it.Kind) && firstImage(it, []string{"poster"}) != "":
		n.Props["image"] = r.picture(firstImage(it, []string{"poster"}), 38, "2/3", "poster")
	default:
		if src := firstImage(it, []string{"square", "poster"}); src != "" {
			n.Props["image"] = r.picture(src, 56, "1/1", "square")
		}
	}
	if it.Progress > 0 && it.Progress < 1 {
		n.Props["progress"] = round3(it.Progress)
	}
	if len(it.Badges) > 0 {
		n.Props["badges"] = it.Badges[:min(len(it.Badges), 2)]
	}
	switch {
	case it.Route != "":
		n.Action = &Action{Type: ActNavigate, To: it.Route}
	case it.PlayID != "":
		n.Action = &Action{Type: ActPlay, Item: it.PlayID}
	}
	return n
}

// hero fills a Hero: each title a Card of style "hero" holding its backdrop,
// facts, synopsis and buttons.
func (r *renderer) hero(n *Node, in instance) bool {
	s, p := in.sec, in.sec.Props
	limit := min(propInt(p, "maxItems", 5), r.l.Budget.HeroItems)
	res, _, err := r.ask(s.Source, in.each, 0, limit)
	if errors.Is(err, ErrNotFound) {
		r.err = err
	}
	if err != nil || len(res.Items) == 0 {
		return false
	}
	r.eager++
	style := propStr(p, "style", "billboard")
	n.Props["style"] = style
	if t := r.fill(propStr(p, "title", ""), in.each, s.Source); t != "" {
		n.Props["title"] = t
	}
	if adv := propInt(p, "autoAdvanceSec", 8); adv > 0 && !r.l.ReducedMotion {
		n.Props["autoAdvanceSec"] = adv
	}
	n.Props["size"] = map[string]any{"width": r.l.Hero.Width, "height": r.l.Hero.Height}
	// The next slide's edge shows on a phone or a tablet, where the spotlight is swiped; a
	// television's buttons and a desktop's arrows say there is more already.
	if propBool(p, "peek", false) && (r.l.FormFactor == FormPhone || r.l.FormFactor == FormTablet) {
		n.Props["peek"] = true
	}
	actions := propList(p, "actions", defaultList("Hero", "actions"))
	for i := range res.Items {
		it := &res.Items[i]
		c := Node{Type: "Card", ID: r.uniqueID(n.ID + ":" + safeID(it.ID)), Props: map[string]any{
			"style": "hero", "shape": "landscape", "title": Truncate(it.Title, r.l.TextLimits.Title), "itemId": it.ID, "kind": it.Kind}}
		// Only a real backdrop: a poster stretched into a 16/9 slide is never sent. A slide
		// without one has no image, and is drawn on the theme's colours.
		if src := firstImage(it, []string{"backdrop"}); src != "" {
			c.Props["image"] = r.backdrop(src, min(r.l.Hero.Width, r.l.WindowWidth()))
		}
		if logo := it.Images["logo"]; logo != "" && propBool(p, "showLogo", true) {
			c.Props["logo"] = r.picture(logo, r.l.Hero.Width/3, "", "logo")
		}
		if f := factsLine(it, []string{"year", "runtime", "contentRating", "genres"}); len(f) > 0 {
			c.Props["facts"] = f
		}
		if propBool(p, "showSynopsis", true) && it.Overview != "" {
			c.Props["synopsis"] = Truncate(it.Overview, r.l.TextLimits.Synopsis*2)
		}
		var acts []any
		for _, a := range actions {
			switch {
			case a == "play" && it.PlayID != "" && contains(it.Can, "play"):
				label := "Play"
				if it.ResumeSec > 0 {
					label = "Resume"
				}
				acts = append(acts, button("play", label, "play", true, &Action{Type: ActPlay, Item: it.PlayID, Params: map[string]any{"resume": true}}))
			case a == "details" && it.Route != "":
				acts = append(acts, button("details", "More info", "info", false, &Action{Type: ActNavigate, To: it.Route}))
			case a == "watchLater" && contains(it.Can, "watchLater"):
				acts = append(acts, button("watchLater", "Watch later", "clock", false, &Action{Type: ActCommand, Op: "watchLater.add", Item: it.ID}))
			}
		}
		if len(acts) > 0 {
			acts[0].(map[string]any)["primary"] = true
			c.Props["actions"] = acts
		}
		if it.Route != "" {
			c.Action = &Action{Type: ActNavigate, To: it.Route}
		}
		n.Children = append(n.Children, c)
	}
	return true
}

func button(id, label, icon string, primary bool, a *Action) map[string]any {
	m := map[string]any{"id": id, "label": label, "icon": icon, "action": a}
	if primary {
		m["primary"] = true
	}
	return m
}

// seasons fills a SeasonList: the show's seasons, and the chosen one's
// episodes as cards.
func (r *renderer) seasons(n *Node, in instance) bool {
	s, p := in.sec, in.sec.Props
	rs, ok := r.resolved(s.Source, in.each)
	if !ok || r.ctx.Resolver == nil {
		return false
	}
	show := rs.Str("show")
	list, err := r.ctx.Resolver.Seasons(show)
	if err != nil || len(list) == 0 {
		if errors.Is(err, ErrNotFound) {
			r.err = err
		}
		return false
	}
	res, _, err := r.ask(s.Source, in.each, 0, r.firstPage("SeasonList", p, s.Source))
	if err != nil {
		return false
	}
	r.eager++
	selected := firstNonEmpty(rs.Str("season"), res.Selected, strconv.Itoa(list[0].Season))
	var seasons []any
	for i := range list {
		se := &list[i]
		num := strconv.Itoa(se.Season)
		m := map[string]any{"number": se.Season, "title": firstNonEmpty(se.Title, "Season "+num), "count": se.Count,
			"url": r.moreURL(n.ID, 0, "season", num)}
		if se.Unwatched > 0 {
			m["unwatched"] = se.Unwatched
		}
		if num == selected {
			m["selected"] = true
		}
		seasons = append(seasons, m)
	}
	n.Props["seasons"] = seasons
	n.Props["style"] = propStr(p, "style", "cards")
	n.Props["title"] = "Episodes"
	m := r.l.Cards["episode"]
	n.Props["card"] = map[string]any{"width": m.Width["m"], "height": aspectHeight(m.Width["m"], m.Aspect), "aspect": m.Aspect, "shape": m.Shape}
	for i := range res.Items {
		c := r.card(n.ID, "episode", "m", &res.Items[i], s.Source)
		if !propBool(p, "showSynopsis", true) {
			delete(c.Props, "synopsis")
		}
		n.Children = append(n.Children, c)
	}
	if res.More {
		n.Props["more"] = map[string]any{"url": r.moreURL(n.ID, len(res.Items), "season", selected)}
	}
	return true
}

// tracks fills a TrackList.
func (r *renderer) tracks(n *Node, in instance) bool {
	s, p := in.sec, in.sec.Props
	res, _, err := r.ask(s.Source, in.each, 0, 500)
	if errors.Is(err, ErrNotFound) {
		r.err = err
	}
	if err != nil || len(res.Items) == 0 {
		return false
	}
	r.eager++
	copyProps(n.Props, p, "showNumbers", "showArtist", "showDuration")
	var total float64
	for i := range res.Items {
		n.Children = append(n.Children, r.track(n.ID, &res.Items[i], i))
		total += res.Items[i].RuntimeSec
	}
	n.Props["count"] = len(res.Items)
	if total > 0 {
		n.Props["durationSec"] = int(total + 0.5)
	}
	return true
}

func (r *renderer) track(parent string, it *Item, i int) Node {
	n := Node{Type: "Track", ID: r.uniqueID(parent + ":" + safeID(it.ID)), Props: map[string]any{"title": Truncate(it.Title, r.l.TextLimits.Title), "itemId": it.ID}}
	num := it.Track
	if num == 0 {
		num = i + 1
	}
	n.Props["number"] = num
	if it.Artist != "" {
		n.Props["artist"] = it.Artist
	}
	if it.RuntimeSec > 0 {
		n.Props["durationSec"] = int(it.RuntimeSec + 0.5)
	}
	if it.Progress > 0 && it.Progress < 1 {
		n.Props["progress"] = round3(it.Progress)
	}
	if it.PlayID != "" {
		n.Action = &Action{Type: ActPlay, Item: it.PlayID}
	}
	return n
}

// banner fills a Banner: written by hand, or from its source's first item.
func (r *renderer) banner(n *Node, in instance) bool {
	s, p := in.sec, in.sec.Props
	title, text := propStr(p, "title", ""), propStr(p, "text", "")
	image := propStr(p, "image", "")
	copyProps(n.Props, p, "icon")
	n.Props["style"] = propStr(p, "style", "promo")
	n.Action = r.link(p)
	if s.Source.Type() != "" {
		res, _, err := r.ask(s.Source, in.each, 0, 1)
		if errors.Is(err, ErrNotFound) {
			r.err = err
		}
		if err != nil || len(res.Items) == 0 {
			return false
		}
		r.eager++
		it := &res.Items[0]
		rep := strings.NewReplacer("{title}", it.Title, "{subtitle}", it.Subtitle, "{overview}", Truncate(it.Overview, r.l.TextLimits.Synopsis))
		title, text = rep.Replace(firstNonEmpty(title, "{title}")), rep.Replace(text)
		if image == "" {
			image = firstImage(it, []string{"backdrop"}) // never a poster stretched to 16/9
		}
		n.Props["itemId"] = it.ID
		if n.Action == nil {
			if propStr(p, "action", "open") == "play" && it.PlayID != "" && contains(it.Can, "play") {
				n.Action = &Action{Type: ActPlay, Item: it.PlayID, Params: map[string]any{"resume": true}}
			} else if it.Route != "" {
				n.Action = &Action{Type: ActNavigate, To: it.Route}
			}
		}
	}
	if t := r.fill(title, in.each, s.Source); t != "" {
		n.Props["title"] = Truncate(t, r.l.TextLimits.Title)
	}
	if t := r.fill(text, in.each, s.Source); t != "" {
		n.Props["text"] = t
	}
	if image != "" {
		n.Props["image"] = r.picture(image, min(r.l.Hero.Width, 1280), "16/9", "backdrop")
	}
	if l := propStr(p, "buttonLabel", ""); l != "" && n.Action != nil {
		n.Props["buttonLabel"] = l
	}
	return n.Props["title"] != nil || n.Props["text"] != nil
}

// header fills a DetailHeader from the page's subject.
func (r *renderer) header(n *Node, in instance) bool {
	p := in.sec.Props
	if r.ctx.Resolver == nil {
		return false
	}
	it, err := r.ctx.Resolver.Subject()
	if err != nil || it == nil {
		r.err = ErrNotFound
		return false
	}
	n.Props["layout"] = propStr(p, "layout", "backdrop")
	n.Props["title"] = Truncate(it.Title, r.l.TextLimits.Title)
	n.Props["itemId"], n.Props["kind"] = it.ID, it.Kind
	if it.Tagline != "" && propBool(p, "showTagline", true) {
		n.Props["tagline"] = Truncate(it.Tagline, r.l.TextLimits.Subtitle)
	}
	if it.Overview != "" && propBool(p, "showOverview", true) {
		n.Props["overview"] = Truncate(it.Overview, r.l.TextLimits.Overview)
	}
	if f := factsLine(it, propList(p, "facts", defaultList("DetailHeader", "facts"))); len(f) > 0 {
		n.Props["facts"] = f
	}
	if it.Progress > 0 && it.Progress < 1 {
		n.Props["progress"] = round3(it.Progress)
	}
	images := map[string]any{}
	if b := firstImage(it, []string{"backdrop"}); b != "" {
		// A backdrop spans the window: chosen for its width, as every other picture is for its card's.
		images["backdrop"] = r.backdrop(b, r.l.WindowWidth())
	}
	posterStyle := "poster"
	if it.Kind == "album" || it.Kind == "track" || it.Kind == "artist" || it.Kind == "podcast" || it.Kind == "audiobook" {
		posterStyle = "square"
	}
	pm := r.l.Cards[posterStyle]
	if src := firstImage(it, []string{posterStyle, "poster", "square"}); src != "" {
		images["poster"] = r.picture(src, pm.Width["l"], pm.Aspect, posterStyle)
	}
	if logo := it.Images["logo"]; logo != "" {
		images["logo"] = r.picture(logo, r.l.Hero.Width/3, "", "logo")
	}
	if len(images) > 0 {
		n.Props["images"] = images
	}
	if acts := headerActions(it, propList(p, "actions", defaultList("DetailHeader", "actions"))); len(acts) > 0 {
		n.Props["actions"] = acts
	}
	return true
}

// headerActions are the buttons a DetailHeader offers for an item, in the
// order asked for: only those the item can do. The first is the primary one.
func headerActions(it *Item, want []string) []any {
	var out []any
	play := firstNonEmpty(it.NextID, it.PlayID)
	native := func(id, label, icon, screen string) {
		out = append(out, button(id, label, icon, false, &Action{Type: ActNative, To: screen, Params: map[string]any{"item": it.ID}}))
	}
	for _, a := range want {
		if !contains(it.Can, a) && !(a == "restart" && contains(it.Can, "play")) {
			continue
		}
		switch a {
		case "play":
			if play == "" {
				continue
			}
			label := "Play"
			if it.ResumeSec > 0 {
				label = "Resume"
			}
			out = append(out, button("play", label, "play", false, &Action{Type: ActPlay, Item: play, Params: map[string]any{"resume": true}}))
		case "restart":
			if it.ResumeSec > 0 && play != "" {
				out = append(out, button("restart", "Play from the start", "play", false, &Action{Type: ActPlay, Item: play, Params: map[string]any{"resume": false}}))
			}
		case "watchLater":
			out = append(out, button("watchLater", "Watch later", "clock", false, &Action{Type: ActCommand, Op: "watchLater.add", Item: it.ID}))
		case "watched":
			if it.Watched {
				out = append(out, button("watched", "Mark unwatched", "check", false, &Action{Type: ActCommand, Op: "watched.clear", Item: it.ID}))
			} else {
				out = append(out, button("watched", "Mark watched", "check", false, &Action{Type: ActCommand, Op: "watched.set", Item: it.ID}))
			}
		case "download":
			native("download", "Download", "download", "download")
		case "share":
			native("share", "Share", "share", "share")
		case "playOn":
			native("playOn", "Play on…", "devices", "playOn")
		case "watchParty":
			native("watchParty", "Watch party", "users", "watchParty")
		}
	}
	if len(out) > 0 {
		out[0].(map[string]any)["primary"] = true
	}
	return out
}

// factsLine is the line of facts about an item: [{id, value}] in the order
// asked for, leaving out what the item doesn't have.
func factsLine(it *Item, want []string) []any {
	var out []any
	add := func(id, v string) {
		if v != "" {
			out = append(out, map[string]any{"id": id, "value": v})
		}
	}
	for _, f := range want {
		switch f {
		case "year":
			if it.Year > 0 {
				add(f, strconv.Itoa(it.Year))
			}
		case "runtime":
			add(f, FormatRuntime(it.RuntimeSec))
		case "contentRating":
			add(f, it.ContentRating)
		case "rating":
			if it.Rating > 0 {
				add(f, strconv.FormatFloat(float64(int(it.Rating*10+0.5))/10, 'f', 1, 64))
			}
		case "genres":
			add(f, strings.Join(it.Genres[:min(len(it.Genres), 3)], ", "))
		case "studio":
			add(f, it.Studio)
		case "count":
			add(f, countText(it))
		case "season":
			if it.Season > 0 || it.Episode > 0 {
				add(f, fmt.Sprintf("S%d E%d", it.Season, it.Episode))
			}
		}
	}
	return out
}

func countText(it *Item) string {
	if it.Count <= 0 {
		return ""
	}
	noun := "titles"
	switch it.Kind {
	case "show", "season", "podcast":
		noun = "episodes"
	case "album", "audiobook":
		noun = "tracks"
	case "artist":
		noun = "albums"
	}
	if it.Count == 1 {
		noun = strings.TrimSuffix(noun, "s")
	}
	return strconv.Itoa(it.Count) + " " + noun
}

// tabs fills a Tabs: every tab's label, and the chosen tab's content.
func (r *renderer) tabs(n *Node, in instance, depth int) bool {
	p := in.sec.Props
	tabs := r.expand(in.sec.Children, false)
	if len(tabs) == 0 {
		return false
	}
	want := []string{r.ctx.Params["tab"]}
	if r.ctx.Personal != nil && r.ctx.CanPersonalise && allows(r.page, "defaultTab") {
		want = append(want, r.ctx.Personal.DefaultTab[in.id])
	}
	want = append(want, propStr(p, "default", ""))
	selected := tabs[0].id
	for _, w := range want {
		found := false
		for _, t := range tabs {
			if w != "" && t.id == w {
				selected, found = w, true
			}
		}
		if found {
			break
		}
	}
	n.Props["style"], n.Props["selected"] = propStr(p, "style", "tabs"), selected
	for _, t := range tabs {
		if t.sec.Component != "Tab" {
			continue
		}
		tn := Node{Type: "Tab", ID: r.uniqueID(t.id), Props: map[string]any{"label": r.fill(propStr(t.sec.Props, "label", ""), in.each, nil)}}
		copyProps(tn.Props, t.sec.Props, "icon")
		tn.Action = &Action{Type: ActCommand, Op: "selectTab", Params: map[string]any{"tab": t.id}}
		if t.id == selected {
			tn.Props["selected"] = true
			t.each = in.each
			tn.Children = r.children(t, depth+1)
		}
		n.Children = append(n.Children, tn)
	}
	return len(n.Children) > 0
}

// filterBar fills a FilterBar: each filter's choices from the Resolver and
// what is chosen now from the page's parameters.
func (r *renderer) filterBar(n *Node, in instance) bool {
	p := in.sec.Props
	n.Props["target"] = propStr(p, "target", "")
	fields := propList(p, "fields", defaultList("FilterBar", "fields"))
	var out []any
	if r.ctx.Resolver != nil && len(fields) > 0 {
		facets, _ := r.ctx.Resolver.Facets(r.ctx.Vars["library"], fields)
		for _, f := range facets {
			if len(f.Options) == 0 {
				continue
			}
			m := map[string]any{"id": f.ID, "label": f.Label, "options": f.Options}
			if v := r.ctx.Params[f.ID]; v != "" {
				m["value"] = v
			}
			out = append(out, m)
		}
	}
	if len(out) > 0 {
		n.Props["fields"] = out
	}
	sorts := propList(p, "sorts", defaultList("FilterBar", "sorts"))
	if len(sorts) > 0 {
		spec, _ := Component("FilterBar")
		var options []EnumOption
		for _, s := range sorts {
			for _, o := range spec.prop("sorts").Options {
				if o.Value == s {
					options = append(options, o)
				}
			}
		}
		sortBy := r.ctx.Params["sort"]
		if !contains(sorts, sortBy) {
			sortBy = sorts[0]
		}
		m := map[string]any{"value": sortBy, "options": options}
		if r.ctx.Params["desc"] == "1" {
			m["desc"] = true
		}
		n.Props["sort"] = m
	}
	if propBool(p, "showCount", true) {
		n.Props["showCount"] = true
	}
	return len(out) > 0 || len(sorts) > 1
}

// seeAllCard says whether a row's See all is its last card: the section's own choice, else
// on a television and in the layouts that ask for it (TV rows, Streaming style).
func (r *renderer) seeAllCard(p map[string]any) bool {
	switch propStr(p, "seeAllCard", "auto") {
	case "always":
		return true
	case "never":
		return false
	}
	return r.l.FormFactor == FormTV || (r.ctx.Layout != nil && r.ctx.Layout.Pages.SeeAllCard)
}

// keyHints are the labels of a KeyLegend's actions.
var keyHints = []EnumOption{{"select", "Select"}, {"back", "Back"}, {"options", "Options"}, {"search", "Search"},
	{"tabs", "Tabs"}, {"page", "Page"}, {"top", "Top"}, {"dock", "Dock"}, {"help", "Keys"}}

func (r *renderer) keyLegend(n *Node, in instance) bool {
	actions := propList(in.sec.Props, "actions", defaultList("KeyLegend", "actions"))
	var entries []any
	for _, a := range actions {
		for _, h := range keyHints {
			if h.Value == a {
				entries = append(entries, map[string]any{"action": h.Value, "label": h.Label})
			}
		}
	}
	if len(entries) == 0 {
		return false
	}
	n.Props["entries"] = entries
	return true
}

// dock draws a Dock's own children; its generated controls are added once the page's
// filter bars are made (finishDocks).
func (r *renderer) dock(n *Node, in instance, depth int) bool {
	p := in.sec.Props
	n.Props["title"] = firstNonEmpty(r.fill(propStr(p, "title", ""), in.each, nil), "Options")
	n.Props["placement"] = r.l.Dock.Placement
	if t := propStr(p, "target", ""); t != "" {
		n.Props["target"] = t
	}
	if c := propList(p, "controls", nil); len(c) > 0 {
		n.Props["controls"] = anyList(c)
	}
	if propBool(p, "legend", true) {
		n.Props["legend"] = true
	}
	n.Children = r.children(in, depth)
	return true
}

func anyList(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

// finishDocks makes each Dock's generated controls from the filter bar that drives its
// target, adds its key legend, and takes away a Dock with nothing in it. Sort and filters
// are the filter bar's; view and jump are the Grid's.
func (r *renderer) finishDocks(root *Node) {
	var bar *Node
	var find func(n *Node)
	find = func(n *Node) {
		if n.Type == "FilterBar" && bar == nil {
			bar = n
		}
		for i := range n.Children {
			find(&n.Children[i])
		}
	}
	find(root)
	var walk func(n *Node)
	walk = func(n *Node) {
		kept := n.Children[:0]
		for i := range n.Children {
			c := &n.Children[i]
			if c.Type == "Dock" {
				if !r.fillDock(c, bar) {
					delete(r.ids, c.ID)
					continue
				}
			} else {
				walk(c)
			}
			kept = append(kept, *c)
		}
		n.Children = kept
	}
	walk(root)
}

// fillDock reports whether the Dock has anything to show.
func (r *renderer) fillDock(d *Node, bar *Node) bool {
	target := propStr(d.Props, "target", "")
	controls, _ := d.Props["controls"].([]any)
	delete(d.Props, "controls")
	var made []Node
	add := func(key, label, icon string) {
		made = append(made, Node{Type: "Button", ID: r.uniqueID(d.ID + ":" + key), Props: map[string]any{"label": label, "icon": icon, "variant": "secondary"},
			Action: &Action{Type: ActCommand, Op: "control.open", Params: map[string]any{"control": key, "target": target}}})
	}
	if target != "" && bar != nil && propStr(bar.Props, "target", "") == target {
		for _, c := range controls {
			switch c {
			case "sort":
				if s := obj(bar.Props, "sort"); s != nil {
					add("sort", "Sort: "+optionName(s, propStr(s, "value", "")), "sort")
				}
			case "filter":
				if fs, _ := bar.Props["fields"].([]any); len(fs) > 0 {
					label := "Filter"
					if n := activeFilters(fs); n > 0 {
						label += ": " + strconv.Itoa(n)
					}
					add("filter", label, "filter")
				}
			case "view":
				add("view", "View", "grid")
			case "jump":
				if s := obj(bar.Props, "sort"); s != nil && propStr(s, "value", "") == "title" {
					add("jump", "Jump to a letter", "list")
				}
			}
		}
	}
	buttons := len(d.Children)
	// Authored buttons keep their place after the generated controls.
	d.Children = append(made, d.Children...)
	buttons += len(made)
	if d.Props["legend"] == true {
		delete(d.Props, "legend")
		hasLegend := false
		for _, c := range d.Children {
			hasLegend = hasLegend || c.Type == "KeyLegend"
		}
		// A phone's dock is a sheet: no keys to show.
		if !hasLegend && r.l.Dock.Placement == "bottom" && buttons > 0 {
			legend := Node{Type: "KeyLegend", ID: r.uniqueID(d.ID + ":keys"), Props: map[string]any{}}
			if r.keyLegend(&legend, instance{sec: &Section{ID: legend.ID, Component: "KeyLegend"}, id: legend.ID}) {
				d.Children = append(d.Children, legend)
			}
		}
	} else {
		delete(d.Props, "legend")
	}
	return buttons > 0
}

func activeFilters(fields []any) int {
	n := 0
	for _, f := range fields {
		if m, _ := f.(map[string]any); m != nil && propStr(m, "value", "") != "" {
			n++
		}
	}
	return n
}

// optionName is the label of a choice among options ([]EnumOption or the maps a filter
// bar holds).
func optionName(m map[string]any, value string) string {
	switch opts := m["options"].(type) {
	case []EnumOption:
		for _, o := range opts {
			if o.Value == value {
				return o.Label
			}
		}
	case []any:
		for _, o := range opts {
			if om, _ := o.(map[string]any); om != nil && propStr(om, "value", "") == value {
				return propStr(om, "label", "")
			}
		}
	}
	return value
}

// finishFilters gives each FilterBar the count of what its Grid found.
func (r *renderer) finishFilters(root *Node) {
	var walk func(n *Node)
	walk = func(n *Node) {
		if n.Type == "FilterBar" {
			if t, ok := r.totals[propStr(n.Props, "target", "")]; ok && n.Props["showCount"] == true {
				n.Props["count"] = t
			}
			delete(n.Props, "showCount")
		}
		for i := range n.Children {
			walk(&n.Children[i])
		}
	}
	walk(root)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func round3(f float64) float64 { return float64(int(f*1000+0.5)) / 1000 }

// safeID keeps the characters a node ID may have.
func safeID(s string) string {
	var b strings.Builder
	for i := 0; i < len(s) && b.Len() < 48; i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '.', c == '-':
			b.WriteByte(c)
		}
	}
	if b.Len() == 0 {
		return "x"
	}
	return b.String()
}

func clipID(s string) string {
	if len(s) > MaxNodeIDLength {
		return s[:MaxNodeIDLength]
	}
	return s
}

// SectionItems is a page of one section's items: what a client gets from a
// section's props.more.url, and from a season's url.
type SectionItems struct {
	Version int `json:"version"`
	// Section is the node the items belong to.
	Section string `json:"section"`
	// State is "" or "empty": a deferred section that turned out to have
	// nothing in it, which the client then takes off the page.
	State string `json:"state,omitempty"`
	// Props are the section's own props as they are now (title, total,
	// seeAll, seasons), for a section that arrived deferred.
	Props map[string]any `json:"props,omitempty"`
	// Items are the cards to add after the ones the client has; with
	// Replace, in place of them (another season).
	Items   []Node `json:"items"`
	Replace bool   `json:"replace,omitempty"`
	// Next is where the page after this one is; "" when this is the last.
	Next string `json:"next,omitempty"`
}

// RenderSection answers one page of a section's items. nodeID is the ID the
// section had in the rendered page (for a copy of a repeated section, the
// section's ID, "-" and the key); offset is the cursor; season chooses a
// SeasonList's season.
func RenderSection(page Page, nodeID string, offset int, season string, ctx *RenderContext) (SectionItems, error) {
	page = styled(ctx.UI.ExpandPage(page), ctx.Layout)
	r := newRenderer(&page, ctx)
	sections, _ := r.sections()
	in, ok := r.find(sections, nodeID)
	if !ok {
		return SectionItems{}, ErrNotFound
	}
	out := SectionItems{Version: SchemaVersion, Section: nodeID, Items: []Node{}}
	n := Node{Type: in.sec.Component, ID: nodeID, Props: map[string]any{}}
	r.ids[nodeID] = true
	shown := false
	switch in.sec.Component {
	case "Shelf", "Grid", "List":
		limit := r.l.Budget.PageSize
		if offset == 0 {
			limit = r.firstPage(in.sec.Component, in.sec.Props, in.sec.Source)
		}
		shown = r.collectionPage(&n, in, offset, limit, false)
	case "SeasonList":
		if season != "" {
			src := Source{}
			for k, v := range in.sec.Source {
				src[k] = v
			}
			src["season"] = season
			sec := *in.sec
			sec.Source = src
			in.sec = &sec
		}
		if offset == 0 {
			shown = r.seasons(&n, in)
			out.Replace = true
		} else {
			res, _, err := r.ask(in.sec.Source, in.each, offset, r.l.Budget.PageSize)
			if err != nil {
				return out, err
			}
			for i := range res.Items {
				n.Children = append(n.Children, r.card(nodeID, "episode", "m", &res.Items[i], in.sec.Source))
			}
			if res.More {
				n.Props["more"] = map[string]any{"url": r.moreURL(nodeID, offset+len(res.Items), "season", season)}
			}
			shown = true
		}
	default:
		return SectionItems{}, ErrNotFound
	}
	if r.err != nil {
		return SectionItems{}, r.err
	}
	if n.State == "error" {
		return SectionItems{}, errors.New("sdui: the section's source failed")
	}
	if !shown || n.State == "empty" {
		out.State = "empty"
	}
	if more, _ := n.Props["more"].(map[string]any); more != nil {
		out.Next, _ = more["url"].(string)
	}
	delete(n.Props, "more")
	if offset == 0 {
		out.Props = n.Props
	}
	if n.Children != nil {
		out.Items = n.Children
	}
	// Cards that arrive later take no part in the page's focus order: the
	// client appends them to their group in the order given.
	return out, nil
}

// find locates the section a rendered node ID came from.
func (r *renderer) find(sections []Section, nodeID string) (instance, bool) {
	for i := range sections {
		s := &sections[i]
		if !Evaluate(s.When, r.ctx) {
			continue
		}
		if s.Repeat == nil && s.ID == nodeID {
			return instance{sec: s, id: nodeID}, true
		}
		if s.Repeat != nil && strings.HasPrefix(nodeID, s.ID+"-") && r.ctx.Resolver != nil {
			each, _ := r.ctx.Resolver.Each(s.Repeat.Over, s.Repeat.Kinds, r.l.Budget.Repeats)
			for j := range each {
				if copyID(s.ID, &each[j]) == nodeID {
					return instance{sec: s, id: nodeID, each: &each[j]}, true
				}
			}
		}
		if in, ok := r.find(s.Children, nodeID); ok {
			return in, true
		}
	}
	return instance{}, false
}

// sourceSpec is a source's spec: the catalogue's, or a module's in use.
func (r *renderer) sourceSpec(t string) (*SourceSpec, bool) {
	if s := r.ctx.UI.Source(t); s != nil {
		return s, true
	}
	return SourceType(t)
}

// obj is a map-valued prop, nil when it isn't one.
func obj(m map[string]any, k string) map[string]any {
	v, _ := m[k].(map[string]any)
	return v
}

// deviceFormFactor is the form factor of the device asking.
func (ctx *RenderContext) deviceFormFactor() string {
	if ctx.Profile == nil {
		return FormWeb
	}
	return ctx.Profile.FormFactor
}
