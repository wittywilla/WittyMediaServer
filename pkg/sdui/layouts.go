// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"slices"
	"strconv"
	"strings"
)

// Layout options: named arrangements of a whole app, chosen by the administrator for the
// server and by each profile for each of its devices (docs/SDUI.md, "Layout options").
// One is the app's shell (where the menu is and how it is drawn), sent with the
// navigation and with every page, and a set of changes to pages that the server makes
// itself, so a renderer only follows the document it is sent.

// Layout option IDs.
const (
	LayoutSidebar     = "sidebar"
	LayoutTopTabs     = "topTabs"
	LayoutTVRows      = "tvRows"
	LayoutCompactList = "compactList"
	LayoutStreaming   = "streaming"
)

// How an app draws its menu.
const (
	NavSidebar    = "sidebar"    // a column at the side, labels and icons
	NavRail       = "rail"       // a narrow column of icons, labels under them
	NavBottomTabs = "bottomTabs" // a tab bar at the bottom (a phone's)
	NavTopTabs    = "topTabs"    // a row of tabs across the top
	NavTVTopMenu  = "tvTopMenu"  // a television's menu across the top, shown when focus goes up
	NavTVSideMenu = "tvSideMenu" // a television's menu down the side, opened by going left
)

// NavStyles are every way an app draws its menu.
var NavStyles = []string{NavSidebar, NavRail, NavBottomTabs, NavTopTabs, NavTVTopMenu, NavTVSideMenu}

// LayoutSpec describes one layout option.
type LayoutSpec struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	// Nav is how the menu is drawn on each form factor.
	Nav map[string]string `json:"nav"`
	// Overlay: the menu is drawn over the top of the page, transparent until the
	// page scrolls, where it is drawn across the top.
	Overlay bool `json:"overlay,omitempty"`
	// Pages are the changes the server makes to every page for this layout.
	Pages PageStyle `json:"pages"`
}

// PageStyle is what a layout changes about pages. Renderers needn't know it: the server
// has made the changes in the document. It is in the catalogue for editors.
type PageStyle struct {
	// GridsAsRows draws grids as rows of cards (a television's way of browsing), except a
	// grid a filter bar drives.
	GridsAsRows bool `json:"gridsAsRows,omitempty"`
	// RowsAsLists draws rows and grids as lists, a line per title.
	RowsAsLists bool `json:"rowsAsLists,omitempty"`
	// Hero is the spotlight's style: "" as authored, billboard, banner, or "row" (a row of
	// large cards).
	Hero string `json:"hero,omitempty"`
	// CardSize is the size of cards whose section doesn't say: s, m or l; "" as authored.
	CardSize string `json:"cardSize,omitempty"`
	// LandscapeCards draws films', shows' and episodes' cards wide where a section leaves
	// the style to its source.
	LandscapeCards bool `json:"landscapeCards,omitempty"`
	// Density is the page's spacing where the profile chose none; "" the device's.
	Density string `json:"density,omitempty"`
}

// Shell is the layout a document was made for, as an app draws its frame: which layout,
// how the menu is drawn on this device, and whether it lies over the page.
type Shell struct {
	Layout  string `json:"layout"`
	Nav     string `json:"nav"`
	Overlay bool   `json:"overlay,omitempty"`
	// Size is the menu's breadth in points: a sidebar's or rail's width, a bar's
	// height (a phone's tab bar without the system's own inset). A renderer lays its
	// frame out with it, so the page keeps the same room on every app.
	Size int `json:"size"`
	// Open is how wide a television's side menu is while it has focus; 0 elsewhere.
	Open int `json:"open,omitempty"`
}

// navSizes are each menu style's breadth on each form factor, in points.
var navSizes = map[string]map[string]int{
	NavSidebar:    {FormWeb: 240, FormDesktop: 240, FormTablet: 220, FormPhone: 240, FormTV: 400},
	NavRail:       {FormWeb: 80, FormDesktop: 80, FormTablet: 72, FormPhone: 72, FormTV: 120},
	NavBottomTabs: {FormWeb: 56, FormDesktop: 56, FormTablet: 64, FormPhone: 56, FormTV: 100},
	NavTopTabs:    {FormWeb: 64, FormDesktop: 64, FormTablet: 56, FormPhone: 48, FormTV: 100},
	NavTVTopMenu:  {FormTV: 88}, // themes/tokens.json tv.topBar.height
	NavTVSideMenu: {FormTV: 120},
}

// Layouts are the layout options, in the order they are offered.
var Layouts = []LayoutSpec{
	{ID: LayoutSidebar, Label: "Sidebar", Icon: "list",
		Description: "The menu down the side where there is room, a tab bar on a phone and the top menu on a television. Pages as they are written.",
		Nav:         map[string]string{FormWeb: NavSidebar, FormDesktop: NavSidebar, FormTablet: NavSidebar, FormPhone: NavBottomTabs, FormTV: NavTVTopMenu}},
	{ID: LayoutTopTabs, Label: "Top tabs", Icon: "more",
		Description: "The menu as a row of tabs across the top on every device, leaving the whole width to the page.",
		Nav:         map[string]string{FormWeb: NavTopTabs, FormDesktop: NavTopTabs, FormTablet: NavTopTabs, FormPhone: NavTopTabs, FormTV: NavTVTopMenu}},
	{ID: LayoutTVRows, Label: "TV rows", Icon: "tv",
		Description: "Everything in rows of large cards, as on a television: grids become rows, and the menu is a narrow rail.",
		Nav:         map[string]string{FormWeb: NavRail, FormDesktop: NavRail, FormTablet: NavRail, FormPhone: NavBottomTabs, FormTV: NavTVTopMenu},
		Pages:       PageStyle{GridsAsRows: true, CardSize: "l", Hero: "billboard"}},
	{ID: LayoutCompactList, Label: "Compact list", Icon: "list",
		Description: "A line per title instead of rows of pictures: the most on the screen at once, for small screens and long libraries.",
		Nav:         map[string]string{FormWeb: NavRail, FormDesktop: NavRail, FormTablet: NavRail, FormPhone: NavBottomTabs, FormTV: NavTVSideMenu},
		Pages:       PageStyle{RowsAsLists: true, Hero: "row", Density: "compact"}},
	{ID: LayoutStreaming, Label: "Streaming style", Icon: "play",
		Description: "A big spotlight under a see-through menu, then rows of wide cards, like a streaming service.",
		Nav:         map[string]string{FormWeb: NavTopTabs, FormDesktop: NavTopTabs, FormTablet: NavTopTabs, FormPhone: NavBottomTabs, FormTV: NavTVTopMenu},
		Overlay:     true, Pages: PageStyle{Hero: "billboard", LandscapeCards: true, Density: "comfortable"}},
}

// LayoutIDs are the layout options' IDs.
func LayoutIDs() []string {
	out := make([]string, len(Layouts))
	for i, l := range Layouts {
		out[i] = l.ID
	}
	return out
}

// LayoutByID is a layout option, nil for an ID that isn't one.
func LayoutByID(id string) *LayoutSpec {
	for i := range Layouts {
		if Layouts[i].ID == id {
			return &Layouts[i]
		}
	}
	return nil
}

// DefaultLayout is the layout a form factor has when nobody chose one: the apps as they
// have always looked (on a television that is the top menu and pages as written).
func DefaultLayout(formFactor string) string { return LayoutSidebar }

// ShellFor is the shell a layout makes on a form factor.
func ShellFor(id, formFactor string) *Shell {
	l := LayoutByID(id)
	if l == nil {
		l = LayoutByID(DefaultLayout(formFactor))
	}
	nav := l.Nav[formFactor]
	if nav == "" {
		nav = l.Nav[FormWeb]
	}
	sh := &Shell{Layout: l.ID, Nav: nav, Overlay: l.Overlay && formFactor != FormTV, Size: navSizes[nav][formFactor]}
	if sh.Size == 0 {
		sh.Size = navSizes[nav][FormWeb]
	}
	if nav == NavTVSideMenu {
		sh.Open = 400
	}
	return sh
}

// ChooseLayout picks the layout for a request: the profile's choice for the device, then
// the administrator's for the form factor, then the default; a choice the administrator
// doesn't allow, or that isn't a layout, is passed over.
func ChooseLayout(personal, server string, allowed []string, formFactor string) string {
	ok := func(id string) bool {
		return id != "" && LayoutByID(id) != nil && (len(allowed) == 0 || slices.Contains(allowed, id))
	}
	switch {
	case ok(personal):
		return personal
	case ok(server):
		return server
	case ok(DefaultLayout(formFactor)):
		return DefaultLayout(formFactor)
	}
	if len(allowed) > 0 && ok(allowed[0]) {
		return allowed[0]
	}
	return DefaultLayout(formFactor)
}

// styled is a page changed for a layout: what PageStyle says, made with the catalogue's
// own components, so a renderer draws it like any other page.
func styled(p Page, l *LayoutSpec) Page {
	if l == nil || l.Pages == (PageStyle{}) {
		return p
	}
	p = ClonePage(p)
	st := l.Pages
	p.Sections = styleList(p.Sections, st, filterTargets(p.Sections))
	for i := range p.Variants {
		p.Variants[i].Sections = styleList(p.Variants[i].Sections, st, filterTargets(p.Variants[i].Sections))
	}
	return p
}

// filterTargets are the grids a filter bar drives: they stay grids.
func filterTargets(list []Section) map[string]bool {
	out := map[string]bool{}
	var walk func(list []Section)
	walk = func(list []Section) {
		for _, s := range list {
			if s.Component == "FilterBar" {
				if t := propStr(s.Props, "target", ""); t != "" {
					out[t] = true
				}
			}
			walk(s.Children)
		}
	}
	walk(list)
	return out
}

// landscapeKinds are the kinds of item a wide card suits.
var landscapeKinds = []string{"movie", "show", "episode", "video", "recording"}

func styleList(list []Section, st PageStyle, keep map[string]bool) []Section {
	for i := range list {
		s := &list[i]
		if s.Props == nil {
			s.Props = map[string]any{}
		}
		switch s.Component {
		case "Grid":
			if st.RowsAsLists && !keep[s.ID] {
				toList(s)
			} else if st.GridsAsRows && !keep[s.ID] {
				s.Component = "Shelf"
				delete(s.Props, "columns")
				if _, set := s.Props["seeAll"]; !set {
					s.Props["seeAll"] = "auto"
				}
			}
		case "Shelf":
			if st.RowsAsLists {
				toList(s)
			}
		case "Hero":
			switch st.Hero {
			case "billboard", "banner":
				s.Props["style"] = st.Hero
			case "row":
				s.Component = "Shelf"
				title := propStr(s.Props, "title", "Featured")
				s.Props = map[string]any{"title": title, "seeAll": "never"}
				if st.RowsAsLists {
					toList(s)
				}
			}
		}
		if s.Component == "Shelf" || s.Component == "Grid" {
			if _, set := s.Props["cardSize"]; !set && st.CardSize != "" {
				s.Props["cardSize"] = st.CardSize
			}
		}
		if len(s.Props) == 0 {
			s.Props = nil
		}
		s.Children = styleList(s.Children, st, keep)
	}
	return list
}

// toList makes a row or grid a list of the same items: a line each, the title its
// heading.
func toList(s *Section) {
	keep := map[string]any{}
	if t := propStr(s.Props, "title", ""); t != "" {
		keep["title"] = t
	}
	keep["dense"] = true
	s.Component, s.Props = "List", keep
}

func overlaps(a, b []string) bool {
	for _, x := range a {
		if slices.Contains(b, x) {
			return true
		}
	}
	return false
}

// layoutDensity is the density a layout asks for, "" for none.
func layoutDensity(l *LayoutSpec) string {
	if l == nil {
		return ""
	}
	return strings.TrimSpace(l.Pages.Density)
}

// LayoutPolicy is the administrator's choice of layouts for the server: the layout each
// form factor starts with, and the layouts profiles may choose from.
type LayoutPolicy struct {
	// Default is the layout each form factor has unless a profile chose another; a form
	// factor left out has DefaultLayout.
	Default map[string]string `json:"default,omitempty"`
	// Allowed are the layouts profiles may choose; empty: every one.
	Allowed []string `json:"allowed,omitempty"`
}

// ValidateLayoutPolicy checks the administrator's layouts.
func ValidateLayoutPolicy(p *LayoutPolicy) []Issue {
	v := &validator{page: &Page{Kind: "custom"}}
	if p == nil {
		return nil
	}
	ids := LayoutIDs()
	for ff, id := range p.Default {
		if !contains(FormFactors, ff) {
			v.err("/default/"+ff, suggest(ff, FormFactors), "%q is not a form factor", ff)
		}
		if LayoutByID(id) == nil {
			v.err("/default/"+ff, suggest(id, ids), "%q is not a layout; the layouts are %s", id, strings.Join(ids, ", "))
		} else if len(p.Allowed) > 0 && !slices.Contains(p.Allowed, id) {
			v.err("/default/"+ff, "Allow it, or choose an allowed layout.", "%q is the default on %s but isn't among the allowed layouts", id, ff)
		}
	}
	seen := map[string]bool{}
	for i, id := range p.Allowed {
		path := "/allowed/" + strconv.Itoa(i)
		if LayoutByID(id) == nil {
			v.err(path, suggest(id, ids), "%q is not a layout; the layouts are %s", id, strings.Join(ids, ", "))
		} else if seen[id] {
			v.err(path, "Remove the repeat.", "%q is listed twice", id)
		}
		seen[id] = true
	}
	return v.issues
}

// LayoutOptions are the layouts a profile may choose from under a policy, as choices.
func LayoutOptions(p *LayoutPolicy) []EnumOption {
	var out []EnumOption
	for _, l := range Layouts {
		if p == nil || len(p.Allowed) == 0 || slices.Contains(p.Allowed, l.ID) {
			out = append(out, EnumOption{Value: l.ID, Label: l.Label})
		}
	}
	return out
}
