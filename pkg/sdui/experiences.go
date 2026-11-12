// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"bytes"
	"embed"
	"encoding/json"
	"io/fs"
	"slices"
	"strconv"
	"strings"
)

// Experiences: complete looks, chosen by the administrator for the server and by each
// profile for each of its devices (docs/SDUI.md, "Experiences"). An experience is the
// app's shell (where the menu is and how it is drawn), the shape, finish, type set and
// density it wears, and a set of changes to pages that the server makes itself, so a
// renderer only follows the document it is sent. It is not a theme: colour is chosen
// separately, and each experience only suggests one (docs/plans/DESIGN-2.md section 8).

// How an app draws its menu.
const (
	NavSidebar       = "sidebar"          // a column at the side, labels and icons
	NavRail          = "rail"             // a narrow column of icons, labels under them
	NavRailDrawer    = "railDrawer"       // a rail that opens into a full menu and a library drawer
	NavBottomTabs    = "bottomTabs"       // a tab bar at the bottom (a phone's)
	NavTopTabs       = "topTabs"          // a row of tabs across the top
	NavFloatingGlass = "floatingGlassBar" // a glossy glass bar floating across the top
	NavCableBar      = "cableBar"         // flat blue rows with yellow highlights
	NavBevelBar      = "bevelBar"         // a silver bevelled bar
	NavTeleKeys      = "teleKeys"         // colour-coded keys, as teletext has
	NavCrossbar      = "crossbar"         // a horizontal bar of categories over a vertical list
	NavOrnament      = "ornamentTabs"     // a headset's ornament tabs beside the window
	NavTVTopMenu     = "tvTopMenu"        // a television's menu across the top, shown when focus goes up
	NavTVSideMenu    = "tvSideMenu"       // a television's menu down the side, opened by going left
)

// NavStyles are every way an app draws its menu.
var NavStyles = []string{NavSidebar, NavRail, NavRailDrawer, NavBottomTabs, NavTopTabs, NavFloatingGlass, NavCableBar, NavBevelBar,
	NavTeleKeys, NavCrossbar, NavOrnament, NavTVTopMenu, NavTVSideMenu}

// Levels of a finish effect.
const (
	FxNone   = "none"
	FxSubtle = "subtle"
	FxFull   = "full"
)

// Effects are the finish an experience puts on surfaces and controls: each none, subtle or
// full. Renderers draw the recipes in the theme's finish tokens at that level, and lower a
// level on modest hardware and with reduced transparency.
type Effects struct {
	Glass   string `json:"glass"`
	Gloss   string `json:"gloss"`
	Glow    string `json:"glow"`
	Texture string `json:"texture"`
	Bevel   string `json:"bevel"`
}

// ExperienceShape is the corner and card set an experience wears.
type ExperienceShape struct {
	// RadiusMultiple multiplies the theme's radius.
	RadiusMultiple float64 `json:"radiusMultiple"`
	// CardSet names the card styles it prefers: poster, plate, landscape, tile, block, list,
	// row or carousel.
	CardSet string `json:"cardSet"`
}

// ExperienceMotion is how focus and screens move: focus ring, ring-lift, fill or glow;
// screens fade, slide or cut. Overshoot adds a small settle (never on modest hardware).
type ExperienceMotion struct {
	Focus     string `json:"focus"`
	Screen    string `json:"screen"`
	Overshoot bool   `json:"overshoot,omitempty"`
}

// ExperienceHome is what an experience's Home starts with: the spotlight's style and the
// arrangement of rows.
type ExperienceHome struct {
	Hero string `json:"hero"`
	Rows string `json:"rows"`
}

// Preview is the swatch the chooser draws.
type Preview struct {
	BG      string `json:"bg"`
	Surface string `json:"surface"`
	Accent  string `json:"accent"`
}

// Placement is where one region of the shell is, and whether it is shown.
type Placement struct {
	Position string `json:"position,omitempty"`
	Visible  *bool  `json:"visible,omitempty"`
}

// Experience describes one look.
type Experience struct {
	ID          string  `json:"id"`
	Order       int     `json:"order"`
	Name        string  `json:"name"`
	Tagline     string  `json:"tagline"`
	Description string  `json:"description"`
	Icon        string  `json:"icon"`
	Preview     Preview `json:"preview"`
	// Nav is how the menu is drawn on each form factor; NavExpand is how a rail opens:
	// "drawer" (a library drawer beside it), "" (a plain menu).
	Nav       map[string]string `json:"nav"`
	NavExpand string            `json:"navExpand,omitempty"`
	// Overlay: the menu is drawn over the top of the page, transparent until the
	// page scrolls, where it is drawn across the top.
	Overlay bool `json:"overlay,omitempty"`
	// Pages are the changes the server makes to every page for this experience.
	Pages PageStyle `json:"pages"`
	// Theme is the theme it wears unless one is required or chosen; TVTheme the WittyTV
	// look it suggests (themes/tokens.json tvThemes).
	Theme   string          `json:"theme"`
	TVTheme string          `json:"tvTheme"`
	Effects Effects         `json:"effects"`
	Shape   ExperienceShape `json:"shape"`
	// Type is the type set: system, humanist, rounded, mono or serif.
	Type    string           `json:"type"`
	Density string           `json:"density"`
	Motion  ExperienceMotion `json:"motion"`
	Home    ExperienceHome   `json:"home"`
	// Placements are the shell's regions as the experience has them; the administrator's
	// and the profile's placements are laid over them (Placements).
	Placements map[string]Placement `json:"placements,omitempty"`
}

// PageStyle is what an experience changes about pages. Renderers needn't know it: the
// server has made the changes in the document. It is in the catalogue for editors.
type PageStyle struct {
	// GridsAsRows draws grids as rows of cards (a television's way of browsing), except a
	// grid a filter bar drives.
	GridsAsRows bool `json:"gridsAsRows,omitempty"`
	// RowsAsLists draws rows and grids as lists, a line per title.
	RowsAsLists bool `json:"rowsAsLists,omitempty"`
	// Hero is the spotlight's style: "" as authored, billboard, banner, "carousel" (a
	// centred carousel of posters) or "row" (a row of large cards).
	Hero string `json:"hero,omitempty"`
	// CardSize is the size of cards whose section doesn't say: s, m or l; "" as authored.
	CardSize string `json:"cardSize,omitempty"`
	// CardStyle is the style of the cards of films, shows and episodes where a section
	// leaves it to its source: landscape, plate, tile or block; "" as the source has it.
	CardStyle string `json:"cardStyle,omitempty"`
	// SectionStyle is how a section's heading is drawn where it says nothing: band
	// (a coloured header row) or underline; "" plain.
	SectionStyle string `json:"sectionStyle,omitempty"`
	// SeeAllCard makes See all the last card of a row where a section leaves it to the experience.
	SeeAllCard bool `json:"seeAllCard,omitempty"`
	// Crossbar turns Home into a CrossBar of the page's rows; Widgets into a grid of widgets.
	Crossbar bool `json:"crossbar,omitempty"`
	Widgets  bool `json:"widgets,omitempty"`
	// Density is the page's spacing where the profile chose none; "" the device's.
	Density string `json:"density,omitempty"`
}

// Shell is the experience a document was made for, as an app draws its frame: which one,
// how the menu is drawn on this device, and whether it lies over the page.
type Shell struct {
	Experience string `json:"experience"`
	Nav        string `json:"nav"`
	Overlay    bool   `json:"overlay,omitempty"`
	// Size is the menu's breadth in points: a sidebar's or rail's width, a bar's
	// height (a phone's tab bar without the system's own inset). A renderer lays its
	// frame out with it, so the page keeps the same room on every app.
	Size int `json:"size"`
	// Open is how wide a menu is while it is open (a television's side menu with focus, a
	// rail's full menu); 0 where it doesn't open. Drawer is the width of a rail's library
	// drawer beside it; 0 for none.
	Open   int `json:"open,omitempty"`
	Drawer int `json:"drawer,omitempty"`
}

// navSizes are each menu style's breadth on each form factor, in points.
var navSizes = map[string]map[string]int{
	NavSidebar:       {FormWeb: 240, FormDesktop: 240, FormTablet: 220, FormPhone: 240, FormTV: 400, FormHeadset: 240},
	NavRail:          {FormWeb: 80, FormDesktop: 80, FormTablet: 72, FormPhone: 72, FormTV: 120, FormHeadset: 80},
	NavRailDrawer:    {FormWeb: 76, FormDesktop: 76, FormTablet: 72, FormPhone: 72, FormTV: 120, FormHeadset: 76},
	NavBottomTabs:    {FormWeb: 56, FormDesktop: 56, FormTablet: 64, FormPhone: 56, FormTV: 100, FormHeadset: 64},
	NavTopTabs:       {FormWeb: 64, FormDesktop: 64, FormTablet: 56, FormPhone: 48, FormTV: 100, FormHeadset: 64},
	NavFloatingGlass: {FormWeb: 64, FormDesktop: 64, FormTablet: 56, FormPhone: 48, FormTV: 88, FormHeadset: 64},
	NavCableBar:      {FormWeb: 64, FormDesktop: 64, FormTablet: 56, FormPhone: 48, FormTV: 88, FormHeadset: 64},
	NavBevelBar:      {FormWeb: 64, FormDesktop: 64, FormTablet: 56, FormPhone: 48, FormTV: 88, FormHeadset: 64},
	NavTeleKeys:      {FormWeb: 48, FormDesktop: 48, FormTablet: 48, FormPhone: 44, FormTV: 88, FormHeadset: 56},
	NavCrossbar:      {FormWeb: 72, FormDesktop: 72, FormTablet: 64, FormPhone: 56, FormTV: 96, FormHeadset: 72},
	NavOrnament:      {FormHeadset: 72},
	NavTVTopMenu:     {FormTV: 88}, // themes/tokens.json tv.topBar.height
	NavTVSideMenu:    {FormTV: 120},
}

// navOpen and navDrawer are how wide a menu is when it opens, and its library drawer, in
// points (docs/plans/DESIGN-2.md 7.4).
var (
	navOpen   = map[string]map[string]int{NavRailDrawer: {FormWeb: 240, FormDesktop: 240, FormTablet: 220, FormTV: 400, FormHeadset: 240}, NavTVSideMenu: {FormTV: 400}}
	navDrawer = map[string]map[string]int{NavRailDrawer: {FormWeb: 360, FormDesktop: 360, FormTablet: 300, FormTV: 480, FormHeadset: 360}}
)

//go:embed experiences/*.json
var experienceFiles embed.FS

// Experiences are the looks, in the order they are offered: experiences/*.json.
var Experiences = loadExperiences()

func loadExperiences() []Experience {
	files, err := fs.ReadDir(experienceFiles, "experiences")
	if err != nil {
		panic("sdui: the experiences are missing: " + err.Error())
	}
	var out []Experience
	for _, f := range files {
		raw, _ := fs.ReadFile(experienceFiles, "experiences/"+f.Name())
		var e Experience
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&e); err != nil {
			panic("sdui: experiences/" + f.Name() + ": " + err.Error())
		}
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b Experience) int { return a.Order - b.Order })
	return out
}

// Experience IDs the code names.
const (
	ExpLumen      = "lumen"
	ExpClassic    = "classic"
	ExpStage      = "stage"
	ExpCompact    = "compact"
	ExpPaper      = "paper"
	ExpBigPicture = "bigpicture"
)

// ExperienceIDs are the layout options' IDs.
func ExperienceIDs() []string {
	out := make([]string, len(Experiences))
	for i, l := range Experiences {
		out[i] = l.ID
	}
	return out
}

// ExperienceByID is a layout option, nil for an ID that isn't one.
func ExperienceByID(id string) *Experience {
	for i := range Experiences {
		if Experiences[i].ID == id {
			return &Experiences[i]
		}
	}
	return nil
}

// DefaultExperience is the experience a form factor has when nobody chose one: Lumen.
func DefaultExperience(formFactor string) string { return ExpLumen }

// ShellFor is the shell a layout makes on a form factor.
func ShellFor(id, formFactor string) *Shell {
	l := ExperienceByID(id)
	if l == nil {
		l = ExperienceByID(DefaultExperience(formFactor))
	}
	nav := l.Nav[formFactor]
	if nav == "" {
		nav = l.Nav[FormWeb]
	}
	sh := &Shell{Experience: l.ID, Nav: nav, Overlay: l.Overlay && formFactor != FormTV, Size: navSizes[nav][formFactor]}
	if sh.Size == 0 {
		sh.Size = navSizes[nav][FormWeb]
	}
	sh.Open = navOpen[nav][formFactor]
	if l.NavExpand == "drawer" {
		sh.Drawer = navDrawer[nav][formFactor]
	}
	return sh
}

// ChooseExperience picks the experience for a request: what is required of the profile, then the profile's choice for the device, then
// the administrator's for the form factor, then the default; a choice the administrator
// doesn't allow, or that isn't a layout, is passed over.
func ChooseExperience(required, personal, server string, allowed []string, formFactor string) string {
	ok := func(id string) bool {
		return id != "" && ExperienceByID(id) != nil && (len(allowed) == 0 || slices.Contains(allowed, id))
	}
	switch {
	case required != "" && ExperienceByID(required) != nil:
		return required
	case ok(personal):
		return personal
	case ok(server):
		return server
	case ok(DefaultExperience(formFactor)):
		return DefaultExperience(formFactor)
	}
	if len(allowed) > 0 && ok(allowed[0]) {
		return allowed[0]
	}
	return DefaultExperience(formFactor)
}

// styled is a page changed for a layout: what PageStyle says, made with the catalogue's
// own components, so a renderer draws it like any other page.
func styled(p Page, l *Experience) Page {
	if l == nil || l.Pages == (PageStyle{}) {
		return p
	}
	p = ClonePage(p)
	st := l.Pages
	p.Sections = styleList(restructure(p.Sections, p.Kind, st), st, filterTargets(p.Sections))
	for i := range p.Variants {
		p.Variants[i].Sections = styleList(restructure(p.Variants[i].Sections, p.Kind, st), st, filterTargets(p.Variants[i].Sections))
	}
	return p
}

// restructure turns a Home page into the experience's own arrangement of it: a CrossBar
// with a category for each row, or a grid of widgets. Other pages are left as they are.
func restructure(list []Section, kind string, st PageStyle) []Section {
	if kind != "home" || (!st.Crossbar && !st.Widgets) {
		return list
	}
	var keep []Section // notices and banners stay on top
	for _, s := range list {
		if s.Component == "Notice" || s.Component == "Banner" {
			keep = append(keep, s)
		}
	}
	if st.Widgets {
		admin := true
		w := func(id, kind string, x, y, wd, h int, when *Condition) Section {
			return Section{ID: "widget-" + id, Component: "Widget", When: when, Props: map[string]any{"kind": kind, "x": x, "y": y, "w": wd, "h": h}}
		}
		grid := Section{ID: "widgets", Component: "WidgetGrid", Props: map[string]any{"columns": 4}, Children: []Section{
			w("continue", "continue", 0, 0, 2, 1, nil),
			w("livenow", "liveNow", 2, 0, 2, 1, &Condition{Can: "liveTv", Feature: "live-tv"}),
			w("stats", "stats", 0, 1, 2, 1, nil),
			w("status", "serverStatus", 2, 1, 1, 1, &Condition{Admin: &admin}),
			w("clock", "clock", 3, 1, 1, 1, nil),
			w("recordings", "recordings", 0, 2, 2, 1, &Condition{Can: "recordings"}),
		}}
		return append(keep, grid)
	}
	bar := Section{ID: "crossbar", Component: "CrossBar"}
	for _, s := range list {
		if len(bar.Children) >= 8 || (s.Component != "Shelf" && s.Component != "Grid" && s.Component != "Hero") || s.Source.Type() == "" {
			continue
		}
		label := propStr(s.Props, "title", "")
		if label == "" {
			label = "Featured"
		}
		row := s
		row.Component, row.Props = "List", map[string]any{"dense": true}
		bar.Children = append(bar.Children, Section{ID: s.ID + "-tab", Component: "Tab", When: s.When, Props: map[string]any{"label": label, "icon": "list"}, Children: []Section{row}})
	}
	if len(bar.Children) == 0 {
		return list
	}
	return append(keep, bar)
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
			case "carousel":
				s.Component = "Carousel"
				s.Props = map[string]any{"title": propStr(s.Props, "title", "Featured"), "cardSize": "l"}
			case "row":
				s.Component = "Shelf"
				title := propStr(s.Props, "title", "Featured")
				s.Props = map[string]any{"title": title, "seeAll": "never"}
				if st.RowsAsLists {
					toList(s)
				}
			}
		}
		if s.Component == "Shelf" || s.Component == "Grid" || s.Component == "Carousel" {
			if _, set := s.Props["cardSize"]; !set && st.CardSize != "" {
				s.Props["cardSize"] = st.CardSize
			}
		}
		switch s.Component {
		case "Shelf", "Grid", "List", "Carousel", "NumberedRow":
			if _, set := s.Props["headingStyle"]; !set && st.SectionStyle != "" {
				s.Props["headingStyle"] = st.SectionStyle
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
func layoutDensity(l *Experience) string {
	if l == nil {
		return ""
	}
	return strings.TrimSpace(l.Pages.Density)
}

// ExperiencePolicy is the administrator's choice of layouts for the server: the layout each
// form factor starts with, and the layouts profiles may choose from.
type ExperiencePolicy struct {
	// Default is the layout each form factor has unless a profile chose another; a form
	// factor left out has DefaultExperience.
	Default map[string]string `json:"default,omitempty"`
	// Allowed are the layouts profiles may choose; empty: every one.
	Allowed []string `json:"allowed,omitempty"`
	// Required is a look (an experience, a theme or both) the administrator requires of
	// everyone, of kids profiles or of named profiles (look.go).
	Required *RequiredPolicy `json:"required,omitempty"`
	// ProfileThemes lets profiles import themes of their own.
	ProfileThemes bool `json:"profileThemes,omitempty"`
}

// ValidateExperiencePolicy checks the administrator's layouts.
func ValidateExperiencePolicy(p *ExperiencePolicy) []Issue {
	v := &validator{page: &Page{Kind: "custom"}}
	if p == nil {
		return nil
	}
	ids := ExperienceIDs()
	for ff, id := range p.Default {
		if !contains(FormFactors, ff) {
			v.err("/default/"+ff, suggest(ff, FormFactors), "%q is not a form factor", ff)
		}
		if ExperienceByID(id) == nil {
			v.err("/default/"+ff, suggest(id, ids), "%q is not a layout; the layouts are %s", id, strings.Join(ids, ", "))
		} else if len(p.Allowed) > 0 && !slices.Contains(p.Allowed, id) {
			v.err("/default/"+ff, "Allow it, or choose an allowed layout.", "%q is the default on %s but isn't among the allowed layouts", id, ff)
		}
	}
	seen := map[string]bool{}
	for i, id := range p.Allowed {
		path := "/allowed/" + strconv.Itoa(i)
		if ExperienceByID(id) == nil {
			v.err(path, suggest(id, ids), "%q is not a layout; the layouts are %s", id, strings.Join(ids, ", "))
		} else if seen[id] {
			v.err(path, "Remove the repeat.", "%q is listed twice", id)
		}
		seen[id] = true
	}
	v.issues = append(v.issues, ValidateRequired(p.Required)...)
	return v.issues
}

// ExperienceOptions are the layouts a profile may choose from under a policy, as choices.
func ExperienceOptions(p *ExperiencePolicy) []EnumOption {
	var out []EnumOption
	for _, l := range Experiences {
		if p == nil || len(p.Allowed) == 0 || slices.Contains(p.Allowed, l.ID) {
			out = append(out, EnumOption{Value: l.ID, Label: l.Name})
		}
	}
	return out
}

// zoned gives a television's page its Dock (docs/plans/DESIGN-2.md section 4): a page
// with a filter bar over a grid gets the sort, filters, view and jump-to-letter in one
// row at the foot of the screen, unless the page already has a Dock. The Dock is locked,
// so a profile's arrangement can't move or hide it.
func zoned(p Page, formFactor string) Page {
	if formFactor != FormTV {
		return p
	}
	p = ClonePage(p)
	p.Sections = withDock(p.Sections)
	for i := range p.Variants {
		p.Variants[i].Sections = withDock(p.Variants[i].Sections)
	}
	return p
}

func withDock(list []Section) []Section {
	var target string
	var hasDock bool
	var walk func(list []Section)
	walk = func(list []Section) {
		for _, s := range list {
			switch s.Component {
			case "Dock":
				hasDock = true
			case "FilterBar":
				if target == "" {
					target = propStr(s.Props, "target", "")
				}
			}
			walk(s.Children)
		}
	}
	walk(list)
	if hasDock || target == "" || (len(list) > 0 && list[len(list)-1].Component == "NativeView") {
		return list
	}
	id := "dock"
	for taken := true; taken; {
		taken = false
		for _, s := range flatten(list) {
			if s.ID == id {
				id += "-zone"
				taken = true
				break
			}
		}
	}
	return append(list, Section{ID: id, Component: "Dock", Locked: true,
		Props: map[string]any{"target": target, "controls": []any{"sort", "filter", "view", "jump"}}})
}

// flatten lists every section of a tree, containers included.
func flatten(list []Section) []Section {
	var out []Section
	for _, s := range list {
		out = append(out, s)
		out = append(out, flatten(s.Children)...)
	}
	return out
}
