// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
)

// NavDocument is the main menu rendered for one profile on one device.
type NavDocument struct {
	Version int `json:"version"`
	// FormFactor is the device the placements were chosen for.
	FormFactor string `json:"formFactor"`
	// Items are in order. A client puts "primary" entries in its bar (the
	// tab bar of a phone, the top menu of a television, the sidebar of the
	// web app), "more" entries behind More or in the sidebar's lower group,
	// and "end" entries at the bar's far end.
	Items []NavEntry `json:"items"`
	// Personal is what the profile may change about the menu and what it has
	// changed: every entry it could have, in its order, hidden ones included,
	// so an app can offer "Arrange the menu" without knowing the navigation.
	// Settings is listed like the rest and is never marked hidden. Left out
	// for a profile that may not rearrange.
	Personal *PersonalInfo `json:"personal,omitempty"`
	// Shell is how the app draws its frame with the experience in force: the menu's
	// style on this device, and whether it lies over the page.
	Shell *Shell `json:"shell,omitempty"`
	// Experiences are the experiences this profile may choose from on this device, the
	// one in force marked, for an app's "Layout" setting. Left out for a profile kept
	// from rearranging.
	Experiences *ExperienceChoice `json:"experiences,omitempty"`
	ETag        string            `json:"etag,omitempty"`
}

// ExperienceChoice is what a profile may choose for its layout on a device.
type ExperienceChoice struct {
	Current string `json:"current"`
	// Chosen is the profile's own choice for the device, "" when it follows the server.
	Chosen  string       `json:"chosen,omitempty"`
	Options []EnumOption `json:"options"`
	URL     string       `json:"url"`
	// Required says an administrator requires the experience: who, when and for whom. The
	// person can't choose another; Options is then empty.
	Required *RequiredPart `json:"required,omitempty"`
}

// NavEntry is one rendered entry of the main menu.
type NavEntry struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Icon  string `json:"icon"`
	// Kind is "page" (an SDUI page: ask for Page with Params) or "native" (a
	// screen the app draws itself, named by Screen).
	Kind   string            `json:"kind"`
	Page   string            `json:"page,omitempty"`
	Params map[string]string `json:"params,omitempty"`
	Screen string            `json:"screen,omitempty"`
	// Route is the entry's place in the app.
	Route     string `json:"route"`
	Placement string `json:"placement"`
	// Drawer is the second column a library opens beside the rail (experiences with
	// navExpand "drawer", shell.drawer points wide): headed by the library's name, its icon
	// and its item count, listing its views. Back closes the drawer, then the rail.
	Drawer *NavDrawer `json:"drawer,omitempty"`
}

// NavDrawer is a library's drawer.
type NavDrawer struct {
	Title string `json:"title"`
	Icon  string `json:"icon"`
	// Count is how many titles the library holds, 0 when not known.
	Count int       `json:"count,omitempty"`
	Views []NavView `json:"views"`
}

// NavView is one view of a library, opened as the library's page with Params.
type NavView struct {
	ID     string            `json:"id"`
	Label  string            `json:"label"`
	Icon   string            `json:"icon"`
	Params map[string]string `json:"params,omitempty"`
	Route  string            `json:"route"`
}

// libraryViews are the views a kind of library offers in its drawer: what its page can show
// by its own parameters (sort, tab). A module's own views come with its pages.
func libraryViews(kind string, id string) []NavView {
	base := "/library/" + id
	v := func(vid, label, icon, query string, params map[string]string) NavView {
		route := base
		if query != "" {
			route += "?" + query
		}
		p := map[string]string{"library": id}
		for k, x := range params {
			p[k] = x
		}
		return NavView{ID: vid, Label: label, Icon: icon, Params: p, Route: route}
	}
	switch kind {
	case "music", "audiobooks", "podcasts":
		return []NavView{v("recent", "Recently added", "plus", "sort=added", map[string]string{"sort": "added"}),
			v("albums", "Albums", "album", "tab=albums", map[string]string{"tab": "albums"}),
			v("artists", "Artists", "artist", "tab=artists", map[string]string{"tab": "artists"}),
			v("all", "All", "list", "", nil)}
	case "photos", "books", "games", "channels":
		return []NavView{v("recent", "Recently added", "plus", "sort=added", map[string]string{"sort": "added"}), v("all", "All", "list", "", nil)}
	}
	return []NavView{v("recent", "Recently added", "plus", "sort=added", map[string]string{"sort": "added"}),
		v("rating", "Top rated", "star", "sort=rating", map[string]string{"sort": "rating"}),
		v("newest", "Newest releases", "calendar", "sort=newest", map[string]string{"sort": "newest"}),
		v("collections", "Collections", "collection", "", nil),
		v("all", "All", "list", "sort=title", map[string]string{"sort": "title"})}
}

// NavLibrary is a library the profile can open, for the "libraries" entry.
type NavLibrary struct {
	ID    int64
	Name  string
	Kind  string
	Count int
}

// LibraryIcon is the icon of a kind of library.
func LibraryIcon(kind string) string {
	switch kind {
	case "movies":
		return "movie"
	case "shows", "anime":
		return "show"
	case "music", "radio":
		return "music"
	case "musicvideos", "videos", "shorts", "channels", "commercials":
		return "video"
	case "audiobooks":
		return "audiobook"
	case "podcasts":
		return "podcast"
	case "books":
		return "book"
	case "photos":
		return "photo"
	case "games":
		return "game"
	case "discs":
		return "disc"
	}
	return "library"
}

// maxPrimary is how many entries a device's bar holds before the rest go
// behind More. A sidebar and a television's top menu scroll, so they hold
// more.
func maxPrimary(ff string) int {
	switch ff {
	case FormPhone:
		return 5
	case FormTV:
		return 7
	case FormTablet, FormHeadset:
		return 8
	}
	return 24
}

// RenderNavigation makes the main menu for ctx: entries whose conditions
// don't hold are left out, "libraries" becomes one entry per library, pages
// take their own title and icon, and the profile's own order and hidden
// entries are applied (ctx.Personal). Settings is always there: when the
// navigation as authored lacks it, it is added at the end.
//
// pages looks a page up by ID; an entry for a page that isn't there is left
// out.
func RenderNavigation(nav Navigation, pages func(id string) (Page, bool), libs []NavLibrary, ctx *RenderContext) NavDocument {
	if ctx.Profile == nil {
		ctx.Profile = ProfileFor(FormWeb, "")
	}
	ff := ctx.Profile.FormFactor
	place := func(it *NavItem, def string) string {
		if p := it.Placement[ff]; p != "" {
			return p
		}
		if ff == FormWeb {
			if p := it.Placement[FormDesktop]; p != "" {
				return p
			}
		}
		return def
	}
	var out []NavEntry
	hasSettings := false
	for i := range nav.Items {
		it := &nav.Items[i]
		if !(it.Type == "native" && it.Screen == "settings") && !Evaluate(it.When, ctx) {
			continue
		}
		p := place(it, "primary")
		if p == "hidden" {
			continue
		}
		switch it.Type {
		case "page":
			pg, ok := pages(it.Page)
			if !ok {
				continue
			}
			e := NavEntry{ID: it.ID, Label: firstNonEmpty(it.Label, pg.Title), Icon: firstNonEmpty(it.Icon, pg.Icon, "grid"), Kind: "page", Page: pg.ID, Placement: p}
			e.Route = "/page/" + pg.ID
			if k := pageKind(pg.Kind); k != nil && pg.Kind != "custom" && pg.Kind != "library" && pg.Kind != "detail" {
				e.Route = k.Route
			}
			out = append(out, e)
		case "libraries":
			for _, l := range libs {
				id := strconv.FormatInt(l.ID, 10)
				e := NavEntry{ID: "lib-" + id, Label: l.Name, Icon: LibraryIcon(l.Kind), Kind: "page", Page: "library",
					Params: map[string]string{"library": id}, Route: "/library/" + id, Placement: place(it, "more")}
				e.Drawer = drawerFor(ctx, l)
				out = append(out, e)
			}
		case "library":
			for _, l := range libs {
				if l.ID == it.Library {
					id := strconv.FormatInt(l.ID, 10)
					e := NavEntry{ID: it.ID, Label: firstNonEmpty(it.Label, l.Name), Icon: firstNonEmpty(it.Icon, LibraryIcon(l.Kind)), Kind: "page", Page: "library",
						Params: map[string]string{"library": id}, Route: "/library/" + id, Placement: p}
					e.Drawer = drawerFor(ctx, l)
					out = append(out, e)
				}
			}
		case "native":
			spec := nativeScreen(it.Screen)
			if spec == nil {
				continue
			}
			hasSettings = hasSettings || it.Screen == "settings"
			out = append(out, NavEntry{ID: it.ID, Label: firstNonEmpty(it.Label, spec.Label), Icon: firstNonEmpty(it.Icon, spec.Icon), Kind: "native", Screen: it.Screen, Route: spec.Route, Placement: p})
		case "route":
			out = append(out, NavEntry{ID: it.ID, Label: it.Label, Icon: firstNonEmpty(it.Icon, "grid"), Kind: "page", Route: it.Route, Placement: p})
		}
	}
	if !hasSettings {
		out = append(out, NavEntry{ID: "settings", Label: "Settings", Icon: "settings", Kind: "native", Screen: "settings", Route: "/settings", Placement: "end"})
	}
	// What "Arrange the menu" lists: every entry, in the profile's order,
	// with the hidden ones marked. Worked out before the hidden ones go.
	var info *PersonalInfo
	if ctx.CanPersonalise {
		info = &PersonalInfo{Editable: true, Allow: []string{"reorder", "hide"}, URL: "/api/v1/sdui/navigation/personal"}
		all := out
		if p := ctx.Personal; p != nil {
			pos := map[string]int{}
			for i, id := range p.Order {
				if _, dup := pos[id]; !dup {
					pos[id] = i
				}
			}
			var placed, rest []NavEntry
			for _, e := range out {
				if _, ok := pos[e.ID]; ok {
					placed = append(placed, e)
				} else {
					rest = append(rest, e)
				}
			}
			for i := 1; i < len(placed); i++ {
				for j := i; j > 0 && pos[placed[j].ID] < pos[placed[j-1].ID]; j-- {
					placed[j], placed[j-1] = placed[j-1], placed[j]
				}
			}
			all = append(placed, rest...)
		}
		for _, e := range all {
			sec := PersonalSection{ID: e.ID, Title: e.Label}
			if p := ctx.Personal; p != nil && e.Screen != "settings" && contains(p.Hidden, e.ID) {
				sec.Hidden = true
			}
			info.Sections = append(info.Sections, sec)
		}
	}
	// The profile's own order and hidden entries. Settings can't be hidden.
	if p := ctx.Personal; p != nil && ctx.CanPersonalise {
		hidden := map[string]bool{}
		for _, id := range p.Hidden {
			hidden[id] = true
		}
		pos := map[string]int{}
		for i, id := range p.Order {
			if _, dup := pos[id]; !dup {
				pos[id] = i
			}
		}
		var placed, rest []NavEntry
		for _, e := range out {
			if hidden[e.ID] && e.Screen != "settings" {
				continue
			}
			if _, ok := pos[e.ID]; ok {
				placed = append(placed, e)
			} else {
				rest = append(rest, e)
			}
		}
		for i := 1; i < len(placed); i++ {
			for j := i; j > 0 && pos[placed[j].ID] < pos[placed[j-1].ID]; j-- {
				placed[j], placed[j-1] = placed[j-1], placed[j]
			}
		}
		out = append(placed, rest...)
	}
	// A bar holds only so many: the primary entries past that go behind More.
	n := 0
	for i := range out {
		if out[i].Placement == "primary" {
			if n++; n > maxPrimary(ff) {
				out[i].Placement = "more"
			}
		}
	}
	if out == nil {
		out = []NavEntry{}
	}
	doc := NavDocument{Version: SchemaVersion, FormFactor: ff, Items: out, Personal: info}
	b, _ := json.Marshal(doc)
	sum := sha256.Sum256(b)
	doc.ETag = `W/"nav-` + hex.EncodeToString(sum[:])[:32] + `"`
	return doc
}

func nativeScreen(s string) *NativeScreenSpec {
	list := Catalog().NativeScreens
	for i := range list {
		if list[i].Screen == s {
			return &list[i]
		}
	}
	return nil
}

// drawerFor is a library's drawer when the experience has one (rail), else nil.
func drawerFor(ctx *RenderContext, l NavLibrary) *NavDrawer {
	if ctx.Experience == nil || ctx.Experience.NavExpand != "drawer" {
		return nil
	}
	return &NavDrawer{Title: l.Name, Icon: LibraryIcon(l.Kind), Count: l.Count, Views: libraryViews(l.Kind, strconv.FormatInt(l.ID, 10))}
}
