// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// The conformance cases, as the inputs the fixtures are written from. The
// fixtures themselves (fixtures/*.json) are the product: `go test -update`
// rewrites them from these, and the tests then check that the files alone,
// read back as JSON, give the same answers.

type kase struct {
	id, title string
	checks    []string
	in        Input
	more      []SectionAnswer // section, cursor and season to ask; the answer is filled in
}

const now = "2026-11-11T19:30:00Z" // a Wednesday evening

func request(ff string) Request {
	return Request{FormFactor: ff, Platform: map[string]string{"tv": "tvos", "phone": "ios", "tablet": "ios", "desktop": "desktop", "web": "web"}[ff], Now: now,
		ProfileKind: "adult", UserName: "Sam", ServerName: "Den", CanPersonalise: true, Locale: "en-GB",
		Features: []string{"live-tv", "recommendations", "music"}, LibraryKinds: []string{"movies", "shows", "music", "photos"}}
}

func film(i int) sdui.Item {
	id := strconv.Itoa(100 + i)
	return sdui.Item{ID: id, Kind: "movie", Title: "Film " + strconv.Itoa(i), Subtitle: strconv.Itoa(1990 + i), Year: 1990 + i, Rating: 6 + float64(i%4)*0.5,
		ContentRating: "PG", RuntimeSec: float64(5400 + 300*i), Genres: []string{"Adventure", "Family"}, Overview: "A small crew sets out to find film number " + strconv.Itoa(i) + " and learns what was at home all along.",
		Images: map[string]string{"poster": "/api/items/" + id + "/poster", "backdrop": "/api/items/" + id + "/backdrop"}, Route: "/item/" + id, PlayID: id,
		Can: []string{"play", "watchLater", "watched", "download", "share", "playOn", "watchParty"}}
}

func films(n int) []sdui.Item {
	out := make([]sdui.Item, n)
	for i := range out {
		out[i] = film(i + 1)
	}
	return out
}

func show(i int) sdui.Item {
	id := "show-" + strconv.Itoa(i)
	return sdui.Item{ID: id, Kind: "show", Title: "Show " + strconv.Itoa(i), Subtitle: strconv.Itoa(2010 + i), Year: 2010 + i, Unwatched: i, Count: 10 + i,
		Images: map[string]string{"poster": "/api/shows/" + strconv.Itoa(i) + "/art/poster", "backdrop": "/api/shows/" + strconv.Itoa(i) + "/art/backdrop"}, Route: "/show/" + strconv.Itoa(i)}
}

func shows(n int) []sdui.Item {
	out := make([]sdui.Item, n)
	for i := range out {
		out[i] = show(i + 1)
	}
	return out
}

func episode(showN, season, ep int) sdui.Item {
	id := strconv.Itoa(1000*showN + 100*season + ep)
	return sdui.Item{ID: id, Kind: "episode", Title: "Chapter " + strconv.Itoa(ep), ShowTitle: "Show " + strconv.Itoa(showN), ShowID: strconv.Itoa(showN), Season: season, Episode: ep,
		Subtitle: fmt.Sprintf("Show %d · S%d E%d", showN, season, ep), RuntimeSec: 2520, Overview: "In which chapter " + strconv.Itoa(ep) + " happens, and nobody is quite ready for it.",
		Images: map[string]string{"poster": "/api/items/" + id + "/poster", "backdrop": "/api/items/" + id + "/backdrop"}, Route: "/item/" + id, PlayID: id, Can: []string{"play", "watched"}}
}

func episodes(showN, season, n int) []sdui.Item {
	out := make([]sdui.Item, n)
	for i := range out {
		out[i] = episode(showN, season, i+1)
	}
	return out
}

func album(i int) sdui.Item {
	id := "album-3-" + strconv.Itoa(i)
	return sdui.Item{ID: id, Kind: "album", Title: "Album " + strconv.Itoa(i), Subtitle: "The Examples", Artist: "The Examples", Year: 2000 + i, Count: 10,
		Images: map[string]string{"square": "/api/items/" + strconv.Itoa(5000+i) + "/poster"}, Route: "/album/3?artist=The+Examples&album=Album+" + strconv.Itoa(i)}
}

func albums(n int) []sdui.Item {
	out := make([]sdui.Item, n)
	for i := range out {
		out[i] = album(i + 1)
	}
	return out
}

func tracks(n int) []sdui.Item {
	out := make([]sdui.Item, n)
	for i := range out {
		id := strconv.Itoa(6000 + i)
		out[i] = sdui.Item{ID: id, Kind: "track", Title: "Song " + strconv.Itoa(i+1), Subtitle: "The Examples", Artist: "The Examples", Track: i + 1, RuntimeSec: float64(180 + 7*i), PlayID: id, Can: []string{"play"}}
	}
	return out
}

func channels(n int) []sdui.Item {
	out := make([]sdui.Item, n)
	for i := range out {
		key := "lib-" + strconv.Itoa(i+1)
		out[i] = sdui.Item{ID: key, Kind: "channel", Title: "Channel " + string(rune('A'+i)), Number: strconv.Itoa(i + 1), Scope: "wittytv",
			Now:    &sdui.Programme{Title: "The Early Programme", Start: 1794423600, End: 1794427200}, // 19:00 to 20:00
			Next:   &sdui.Programme{Title: "The Late Programme", Start: 1794427200, End: 1794430800},
			Images: map[string]string{"logo": "/api/channels/" + strconv.Itoa(i+1) + "/logo"}, Route: "/tv/watch/" + key}
	}
	return out
}

func photos(n int) []sdui.Item {
	out := make([]sdui.Item, n)
	for i := range out {
		id := strconv.Itoa(7000 + i)
		out[i] = sdui.Item{ID: id, Kind: "photo", Title: "IMG_" + id, Images: map[string]string{"poster": "/api/items/" + id + "/poster"}, Route: "/item/" + id}
	}
	return out
}

func libraryTiles() []sdui.Item {
	return []sdui.Item{
		{ID: "lib1", Kind: "library", Title: "Films", Icon: "movie", Count: 120, Route: "/library/1"},
		{ID: "lib2", Kind: "library", Title: "Shows", Icon: "show", Count: 14, Route: "/library/2"},
		{ID: "lib3", Kind: "library", Title: "Music", Icon: "music", Count: 9, Route: "/library/3"},
		{ID: "lib4", Kind: "library", Title: "Photos", Icon: "photo", Count: 5, Route: "/library/4"},
	}
}

func inProgress() []sdui.Item {
	a := film(3)
	a.Progress, a.ResumeSec = 0.42, 2646
	b := episode(1, 2, 4)
	b.Progress, b.ResumeSec = 0.1, 252
	c := sdui.Item{ID: "8001", Kind: "book", Title: "A Book of Examples", Subtitle: "A. Writer", Progress: 0.65, Images: map[string]string{"poster": "/api/items/8001/poster"}, Route: "/item/8001"}
	return []sdui.Item{a, b, c}
}

func src(match map[string]any, items []sdui.Item, total int) CannedSource {
	return CannedSource{Match: match, Items: items, Total: total}
}

// homeData is a server with four libraries and something in every row.
func homeData() Data {
	return Data{
		Each: map[string][]CannedEach{
			"libraries":   {{Key: "1", Label: "Films", Kind: "movies"}, {Key: "2", Label: "Shows", Kind: "shows"}, {Key: "3", Label: "Music", Kind: "music"}, {Key: "4", Label: "Photos", Kind: "photos"}},
			"pinnedViews": {{Key: "7", Label: "Unwatched in 4K"}},
		},
		Sources: []CannedSource{
			src(map[string]any{"type": "featured"}, films(6), 0),
			src(map[string]any{"type": "libraries"}, libraryTiles(), 0),
			src(map[string]any{"type": "continueWatching", "media": "watch"}, inProgress()[:2], 0),
			src(map[string]any{"type": "continueWatching", "media": "listen"}, nil, 0),
			src(map[string]any{"type": "continueWatching"}, inProgress(), 0),
			src(map[string]any{"type": "liveNow", "favourites": true}, channels(2), 0),
			src(map[string]any{"type": "liveNow"}, channels(4), 0),
			src(map[string]any{"type": "recommended"}, films(8)[2:], 0),
			src(map[string]any{"type": "nextUp"}, []sdui.Item{episode(1, 2, 5), episode(2, 1, 2)}, 0),
			src(map[string]any{"type": "savedView", "view": "7"}, films(5), 0),
			src(map[string]any{"type": "recentlyAdded", "library": "1"}, films(26), 120),
			src(map[string]any{"type": "recentlyAdded", "library": "2"}, shows(14), 0),
			src(map[string]any{"type": "recentlyAdded", "library": "3"}, albums(9), 0),
			src(map[string]any{"type": "recentlyAdded", "library": "4"}, photos(5), 0),
			src(map[string]any{"type": "albums", "library": "3"}, albums(9), 0),
			src(map[string]any{"type": "topRated"}, films(10), 40),
			src(map[string]any{"type": "recentlyReleased"}, films(12), 0),
			src(map[string]any{"type": "random"}, films(12)[6:], 0),
			src(map[string]any{"type": "genre"}, films(7), 0),
			src(map[string]any{"type": "watchLater"}, films(2), 0),
		},
	}
}

func stock(id string) sdui.Page {
	p, ok := sdui.StockPage(id)
	if !ok {
		panic("no built-in page " + id)
	}
	return p
}

func preset(id string) sdui.Page {
	p, ok := sdui.PresetByID(id)
	if !ok || p.Page == nil {
		panic("no page preset " + id)
	}
	return sdui.ClonePage(*p.Page)
}

func page(id, kind string, sections ...sdui.Section) sdui.Page {
	return sdui.Page{ID: id, Kind: kind, Title: strings.ToUpper(id[:1]) + id[1:], Sections: sections}
}

func sec(id, component string, props map[string]any, source sdui.Source) sdui.Section {
	return sdui.Section{ID: id, Component: component, Props: props, Source: source}
}

func home(ff string, change func(in *Input)) Input {
	in := Input{Page: stock("home"), Request: request(ff), Data: homeData()}
	if change != nil {
		change(&in)
	}
	return in
}

var no = false

func subject(it sdui.Item) *sdui.Item { return &it }

func libraryData() Data {
	return Data{
		Sources: []CannedSource{
			src(map[string]any{"type": "browse", "library": "1", "filter": map[string]any{"genre": "Family", "sort": "rating"}}, films(7), 7),
			src(map[string]any{"type": "browse", "library": "1"}, films(64), 120),
			src(map[string]any{"type": "albums", "library": "3", "sort": "played"}, albums(4), 0),
			src(map[string]any{"type": "albums", "library": "3"}, albums(9), 0),
			src(map[string]any{"type": "artists", "library": "3"}, []sdui.Item{
				{ID: "artist-1", Kind: "artist", Title: "The Examples", Subtitle: "9 albums", Count: 9, Images: map[string]string{"square": "/api/music/artist-photo?library=3&name=The+Examples"}, Route: "/artist/3?name=The+Examples"},
				{ID: "artist-2", Kind: "artist", Title: "Sample & Hold", Subtitle: "1 album", Count: 1, Route: "/artist/3?name=Sample+%26+Hold"}}, 0),
			src(map[string]any{"type": "recentlyAdded", "library": "4"}, photos(12), 0),
		},
		Facets: []sdui.Facet{
			{ID: "genre", Label: "Genre", Options: []sdui.EnumOption{{Value: "Adventure", Label: "Adventure"}, {Value: "Family", Label: "Family"}}},
			{ID: "decade", Label: "Decade", Options: []sdui.EnumOption{{Value: "1990", Label: "1990s"}, {Value: "2000", Label: "2000s"}}},
			{ID: "watched", Label: "Watched", Options: []sdui.EnumOption{{Value: "no", Label: "Unwatched"}, {Value: "yes", Label: "Watched"}}},
		},
	}
}

func library(pageID, ff, lib, kind, name string, change func(in *Input)) Input {
	in := Input{Page: stock(pageID), Request: request(ff), Data: libraryData()}
	in.Request.Vars = map[string]string{"library": lib, "libraryKind": kind, "libraryName": name}
	if change != nil {
		change(&in)
	}
	return in
}

func detail(pageID, ff string, vars map[string]string, d Data, change func(in *Input)) Input {
	in := Input{Page: stock(pageID), Request: request(ff), Data: d}
	in.Request.Vars = vars
	if d.Subject != nil {
		in.Request.Vars["title"] = d.Subject.Title
	}
	if change != nil {
		change(&in)
	}
	return in
}

func movieData() Data {
	m := film(3)
	m.Tagline, m.Studio = "Home was the adventure.", "Example Pictures"
	m.Progress, m.ResumeSec = 0.42, 2646
	return Data{Subject: &m, Sources: []CannedSource{src(map[string]any{"type": "similar", "item": "103"}, films(9)[3:], 0)}}
}

func showData() Data {
	s := show(1)
	s.Overview, s.Genres, s.ContentRating, s.Rating = "Twelve chapters about a town where nothing happens twice.", []string{"Drama", "Mystery"}, "TV-14", 8.24
	s.NextID, s.Can = "1205", []string{"play", "watchLater", "share"}
	return Data{Subject: &s,
		Seasons: []sdui.Item{{ID: "s1", Kind: "season", Season: 1, Title: "Season 1", Count: 8}, {ID: "s2", Kind: "season", Season: 2, Title: "Season 2", Count: 40, Unwatched: 36}},
		Sources: []CannedSource{
			{Match: map[string]any{"type": "episodes", "show": "1", "season": "1"}, Items: episodes(1, 1, 8)},
			{Match: map[string]any{"type": "episodes", "show": "1"}, Items: episodes(1, 2, 40), Selected: "2"},
			src(map[string]any{"type": "similar"}, shows(6)[1:], 0),
		}}
}

func albumData() Data {
	a := album(2)
	a.RuntimeSec, a.Genres, a.PlayID, a.Can = 2040, []string{"Pop"}, "6000", []string{"play", "share"}
	return Data{Subject: &a, Sources: []CannedSource{
		src(map[string]any{"type": "tracks", "album": "Album 2"}, tracks(10), 0),
		src(map[string]any{"type": "albums", "artist": "The Examples"}, albums(4), 0),
	}}
}

var albumVars = map[string]string{"library": "3", "libraryKind": "music", "libraryName": "Music", "artist": "The Examples", "album": "Album 2"}

// artistData is the page an artist card opens (/artist/3?name=The+Examples): the artist,
// their albums and every song of theirs, more than one page of it.
func artistData() Data {
	a := sdui.Item{ID: "artist-0", Kind: "artist", Title: "The Examples", Artist: "The Examples", Subtitle: "4 albums", Count: 4,
		Images: map[string]string{"square": "/api/items/5001/poster"}, Route: "/artist/3?name=The+Examples", PlayID: "5001", Can: []string{"play"}}
	return Data{Subject: &a, Sources: []CannedSource{
		src(map[string]any{"type": "albums", "artist": "The Examples"}, albums(4), 0),
		src(map[string]any{"type": "tracks", "artist": "The Examples"}, tracks(40), 0),
	}}
}

var artistVars = map[string]string{"library": "3", "libraryKind": "music", "libraryName": "Music", "artist": "The Examples"}

func custom() sdui.Page {
	return sdui.Page{ID: "family-night", Kind: "custom", Title: "Family film night", Icon: "star", Scope: "family", Sections: []sdui.Section{
		sec("intro", "Text", map[string]any{"text": "## Friday, 7 o'clock\nPick **one** each. The [full list](/collections/12) is longer.", "format": "markdown"}, nil),
		{ID: "links", Component: "Stack", Props: map[string]any{"direction": "row", "gap": "s"}, Children: []sdui.Section{
			sec("guide", "Button", map[string]any{"label": "TV guide", "icon": "guide", "native": "guide"}, nil),
			sec("site", "Button", map[string]any{"label": "Reviews", "icon": "globe", "variant": "ghost", "url": "https://example.org/reviews"}, nil),
			sec("settings", "Button", map[string]any{"label": "Settings", "icon": "settings", "native": "settings"}, nil),
		}},
		sec("rule", "Divider", nil, nil),
		{ID: "two", Component: "Split", Props: map[string]any{"ratio": "2:1"}, Children: []sdui.Section{
			sec("picks", "Shelf", map[string]any{"title": "This week's picks", "cardSize": "l"}, sdui.Source{"type": "collection", "collection": 12}),
			{ID: "side", Component: "Stack", Children: []sdui.Section{
				sec("pic", "Image", map[string]any{"src": "/api/collections/12/cover", "alt": "A sofa and a bowl of popcorn", "aspect": "4/3"}, nil),
				sec("note", "Notice", map[string]any{"tone": "warning", "title": "Bedtime", "text": "Films over two hours start at six."}, nil),
			}},
		}},
		sec("gap", "Spacer", map[string]any{"size": "l"}, nil),
		{ID: "more", Component: "List", Props: map[string]any{"title": "Elsewhere"}, Children: []sdui.Section{
			sec("all-films", "ListItem", map[string]any{"title": "All films", "subtitle": "Everything in the Films library", "icon": "movie", "to": "/library/1"}, nil),
			sec("rules", "ListItem", map[string]any{"title": "House rules", "trailing": "3"}, nil),
		}},
	}}
}

func customData() Data {
	return Data{Sources: []CannedSource{src(map[string]any{"type": "collection", "collection": 12}, films(4), 0)}}
}

func styles() sdui.Page {
	p := page("styles", "custom")
	for _, s := range sdui.Catalog().CardStyles {
		p.Sections = append(p.Sections, sec("row-"+s.Style, "Shelf", map[string]any{"title": s.Label, "cardStyle": s.Style}, sdui.Source{"type": "browse", "tag": s.Style}))
	}
	return p
}

func stylesData() Data {
	var d Data
	watched := film(2)
	watched.Watched, watched.Badges = true, []string{"4K", "HDR", "NEW", "EXTRA"}
	prog := film(3)
	prog.Progress = 0.5
	genre := sdui.Item{ID: "genre-comedy", Kind: "genre", Title: "Comedy", Route: "/library/1?genre=Comedy"}
	game := sdui.Item{ID: "9001", Kind: "game", Title: "Example Quest", Subtitle: "Example Console", Badges: []string{"2 players"}, Images: map[string]string{"poster": "/api/items/9001/poster"}, Route: "/game/9001"}
	for _, s := range sdui.Catalog().CardStyles {
		items := []sdui.Item{film(1), watched, prog, show(3)}
		switch s.Style {
		case "episode", "progress":
			e := episode(1, 1, 3)
			e.Progress = 0.25
			items = []sdui.Item{e, episode(1, 1, 4), prog}
		case "channel":
			items = channels(2)
		case "album", "square":
			items = albums(2)
		case "person", "circle":
			items = []sdui.Item{{ID: "artist-1", Kind: "artist", Title: "The Examples", Subtitle: "9 albums", Images: map[string]string{"square": "/api/music/artist-photo?library=3&name=The+Examples"}, Route: "/artist/3?name=The+Examples"}}
		case "book":
			items = inProgress()[2:]
		case "game":
			items = []sdui.Item{game}
		case "photo":
			items = photos(2)
		case "tile":
			items = libraryTiles()[:2]
		case "chip":
			items = []sdui.Item{genre}
		}
		d.Sources = append(d.Sources, src(map[string]any{"type": "browse", "tag": s.Style}, items, 0))
	}
	return d
}

func cases() []kase {
	long := film(1)
	long.Title = "The Extraordinarily Long and Winding Title of a Film That Never Seems to Reach the End of Its Own Name, Part One: The Beginning"
	long.Overview = strings.Repeat("A sentence about the film that goes on a little. ", 30)
	long.Tagline = strings.Repeat("Longer than a tagline should be. ", 8)

	return append([]kase{
		{id: "home-tv-hd", title: "The built-in Home on an Apple TV HD", in: home("tv", nil),
			checks: []string{"the tv profile: safe area, text sizes, 250-point posters", "four data sections arrive filled; the rest are deferred with a cursor URL", "the libraries row is left out on a television", "focus starts on the hero, not on a row", "a row repeated over libraries has one copy per library, IDs recent-1, recent-2, ..."},
			more:   []SectionAnswer{{Section: "recent-1", Cursor: 0}, {Section: "recent-1", Cursor: 12}, {Section: "view-7", Cursor: 0}}},
		{id: "home-tv-4k", title: "The built-in Home on an Apple TV 4K", in: home("tv", func(in *Input) { in.Request.DeviceClass = "4k" }),
			checks: []string{"scale 2: pictures are asked for at twice the width", "a larger budget: six filled sections, twenty cards a row"}},
		{id: "home-phone", title: "The built-in Home on a phone", in: home("phone", nil),
			checks: []string{"compact breakpoint, scale 3", "the libraries row is shown", "the shuffle banner plays its item"}},
		{id: "home-tablet", title: "The built-in Home on a tablet", in: home("tablet", nil), checks: []string{"the tablet profile"}},
		{id: "home-desktop", title: "The built-in Home in a 1440-point window", in: home("desktop", func(in *Input) { in.Request.Viewport = "1440x900" }),
			checks: []string{"large breakpoint from the viewport", "eight filled sections"}},
		{id: "home-web-narrow", title: "The built-in Home in a narrow browser window", in: home("web", func(in *Input) { in.Request.Viewport = "520x800" }),
			checks: []string{"a web client 520 points wide is at the compact breakpoint; card sizes stay the desktop's"}},
		{id: "home-no-libraries", title: "Home on a server with no libraries", in: home("desktop", func(in *Input) {
			in.Data = Data{}
			in.Request.Flags = map[string]bool{"librariesEmpty": true}
		}), checks: []string{"every data section is left out when empty", "the welcome notice is all there is, and takes focus by its button"}},
		{id: "home-admin-setup", title: "Home for an administrator who hasn't finished setting up", in: home("desktop", func(in *Input) {
			in.Request.Admin = true
			in.Request.Flags = map[string]bool{"setupIncomplete": true}
		}), checks: []string{"a condition on admin and a flag", "the notice has a button that opens the native Settings"}},
		{id: "home-kids-variant", title: "A page with a kids' variant, for a Kids profile", in: Input{Page: preset("home-kids"), Request: func() Request {
			r := request("tablet")
			r.ProfileKind, r.UserName = "kids", "Robin"
			return r
		}(), Data: homeData()}, checks: []string{"the first variant whose condition holds replaces the page's sections", "page.variant names it", "large cards"}},
		{id: "home-kids-variant-adult", title: "The same page for an adult", in: Input{Page: preset("home-kids"), Request: request("tablet"), Data: homeData()},
			checks: []string{"no variant holds: the page's own sections"}},
		{id: "home-personal-order", title: "Home as one profile arranged it", in: home("desktop", func(in *Input) {
			in.Request.Personal = &sdui.Personal{Order: []string{"nextup", "jump", "recent-2", "hero"}, Hidden: []string{"shuffle", "foryou", "recent-4"}, Pinned: []string{"onnow"}}
		}), checks: []string{"pinned sections first, then the profile's order, then the rest in the page's order", "hidden sections are left out but listed in personal.sections", "a copy of a repeated section is placed and hidden by its own ID"}},
		{id: "home-personal-locked", title: "A locked section the profile tried to hide and move", in: home("desktop", func(in *Input) {
			in.Page.Sections[2].Locked = true // the hero
			in.Request.Personal = &sdui.Personal{Order: []string{"jump", "hero"}, Hidden: []string{"hero"}}
		}), checks: []string{"a locked section is never hidden and keeps its place"}},
		{id: "home-personal-not-allowed", title: "A profile that may not rearrange pages", in: home("phone", func(in *Input) {
			in.Request.CanPersonalise = false
			in.Request.ProfileKind = "kids"
			in.Request.Cannot = []string{"editLayout"}
			in.Request.Personal = &sdui.Personal{Hidden: []string{"hero"}, Density: "compact"}
		}), checks: []string{"what it stored is ignored", "personal.editable is false and nothing is allowed"}},
		{id: "home-density-card-size", title: "A profile's density and card sizes", in: home("desktop", func(in *Input) {
			in.Request.Personal = &sdui.Personal{Density: "compact", CardSize: map[string]string{"jump": "l", "recent-1": "s"}}
		}), checks: []string{"compact density: spacing three quarters of the profile's", "a section's card size is the profile's choice"}},
		{id: "home-required-components-only", title: "Home for a client that draws only the required components", in: home("phone", func(in *Input) { in.Request.AllOptional = &no }),
			checks: []string{"Hero becomes a Shelf of large landscape cards", "Banner becomes a Notice", "nothing optional is in the tree"}},
		{id: "home-old-catalogue", title: "Home for a client built against catalogue revision 1", in: home("tv", func(in *Input) { in.Request.Catalog = 1 }),
			checks: []string{"document.catalog is the client's revision", "every component newer than it is sent as its fallback"}},
		{id: "home-source-fails", title: "A row whose source fails", in: home("desktop", func(in *Input) {
			in.Data.Sources = append([]CannedSource{{Match: map[string]any{"type": "recommended"}, Fails: true}}, in.Data.Sources...)
		}), checks: []string{"the row stays with state error and a URL to try again", "the rest of the page is unaffected"}},
		{id: "home-live-tv-off", title: "Home for a profile that may not watch Live TV", in: home("desktop", func(in *Input) { in.Request.Cannot = []string{"liveTv"} }),
			checks: []string{"a \"can\" condition leaves the row out"}},
		{id: "home-reduced-motion", title: "Home with reduced motion", in: home("tv", func(in *Input) { in.Request.Capabilities = []string{"reducedMotion"} }),
			checks: []string{"the hero has no autoAdvanceSec", "layout.reducedMotion is true"}},
		{id: "home-theme-scopes", title: "Theme scopes on a page and a row", in: home("desktop", func(in *Input) {
			in.Page.Scope = "evening"
			in.Request.Scopes = map[string]map[string]string{"evening": {"accent": "#B24A00"}, "wittytv": {"accent": "#2A6FDB", "accentFg": "#FFFFFF"}}
		}), checks: []string{"document.theme carries the page's scope and its colours", "the On TV now row and its channel cards carry scope wittytv and its tokens"}},
		{id: "home-evening-row", title: "A row shown only in the evening, in the evening", in: Input{Page: page("home", "home",
			sdui.Section{ID: "tonight", Component: "Shelf", Props: map[string]any{"title": "Tonight"}, Source: sdui.Source{"type": "random", "limit": 6}, When: &sdui.Condition{TimeFrom: "18:00", TimeTo: "23:00"}},
			sec("jump", "Shelf", map[string]any{"title": "Continue watching"}, sdui.Source{"type": "continueWatching"})), Request: request("phone"), Data: homeData()},
			checks: []string{"19:30 is inside 18:00 to 23:00", "meta.ttlSeconds is at most 60 when a condition looks at the clock"}},
		{id: "home-evening-row-morning", title: "The same page in the morning", in: func() Input {
			in := Input{Page: page("home", "home",
				sdui.Section{ID: "tonight", Component: "Shelf", Props: map[string]any{"title": "Tonight"}, Source: sdui.Source{"type": "random", "limit": 6}, When: &sdui.Condition{TimeFrom: "18:00", TimeTo: "23:00"}},
				sec("jump", "Shelf", map[string]any{"title": "Continue watching"}, sdui.Source{"type": "continueWatching"})), Request: request("phone"), Data: homeData()}
			in.Request.Now = "2026-11-11T08:15:00Z"
			return in
		}(), checks: []string{"08:15 is outside the range: the row is left out"}},
		{id: "home-streaming-preset-tv", title: "The streaming-style Home preset on a television", in: Input{Page: preset("home-streaming"), Request: request("tv"), Data: homeData()},
			checks: []string{"a numbered top ten: cards carry rank", "a source's limit caps the row and removes \"more\""}},
		{id: "home-music-preset", title: "The music-first Home preset", in: Input{Page: preset("home-music"), Request: request("desktop"), Data: homeData()},
			checks: []string{"a repeat over libraries of one kind", "album cards are square", "sections carry scope wittymusic"}},

		{id: "library-films-desktop", title: "A film library in a desktop window", in: library("library-movies", "desktop", "1", "movies", "Films", func(in *Input) { in.Request.Viewport = "1440x900" }),
			checks: []string{"the filter bar's choices come from the server", "its count is the grid's total", "the grid has as many columns as fit: floor((1408 + 16) / (180 + 16)) = 7", "the grid pages: more.url"},
			more:   []SectionAnswer{{Section: "all", Cursor: 60}}},
		{id: "library-films-tv", title: "A film library on a television", in: library("library-movies", "tv", "1", "movies", "Films", nil),
			checks: []string{"six columns of posters", "36 cards in the first page", "focus starts in the grid, not on the filter bar"}},
		{id: "library-films-phone", title: "A film library on a phone", in: library("library-movies", "phone", "1", "movies", "Films", nil), checks: []string{"three columns of 120-point posters"}},
		{id: "library-films-filtered", title: "A film library filtered and sorted", in: library("library-movies", "desktop", "1", "movies", "Films", func(in *Input) {
			in.Request.Params = map[string]string{"genre": "Family", "sort": "rating"}
		}), checks: []string{"the page's query parameters reach the filterable source", "the filter bar shows what is chosen", "page.params and every cursor URL carry the filters"}},
		{id: "library-music", title: "A music library", in: library("library-music", "desktop", "3", "music", "Music", nil),
			checks: []string{"only the selected tab has content", "each tab's action is selectTab", "the page's scope is wittymusic"}},
		{id: "library-music-artists-tab", title: "A music library with tab=artists", in: library("library-music", "tablet", "3", "music", "Music", func(in *Input) { in.Request.Params = map[string]string{"tab": "artists"} }),
			checks: []string{"the tab parameter selects the tab", "person cards are circles; one without a picture carries no image"}},
		{id: "library-music-default-tab", title: "A profile whose music library opens on Artists", in: library("library-music", "tv", "3", "music", "Music", func(in *Input) {
			in.Request.Personal = &sdui.Personal{DefaultTab: map[string]string{"views": "artists"}}
		}), checks: []string{"the profile's default tab wins over the page's"}},
		{id: "library-music-no-tabs", title: "A music library for a client without Tabs", in: library("library-music", "phone", "3", "music", "Music", func(in *Input) {
			in.Request.Components = []string{"Hero", "Banner"}
		}), checks: []string{"Tabs becomes a Stack holding the selected tab's content"}},
		{id: "library-photos-native", title: "A photo library on a client with its own timeline", in: library("library-photos", "phone", "4", "photos", "Photos", func(in *Input) { in.Request.NativeViews = []string{"photoTimeline"} }),
			checks: []string{"a NativeView node with the library, and none of its children"}},
		{id: "library-photos-grid", title: "A photo library on a client without one", in: library("library-photos", "desktop", "4", "photos", "Photos", nil),
			checks: []string{"the NativeView's children in a Stack: a grid of photo cards", "photo cards draw no title but are labelled with it"}},
		{id: "library-empty", title: "An empty library", in: library("library", "phone", "9", "videos", "Home videos", nil),
			checks: []string{"empty \"message\": the grid stays with state empty and its text"}},

		{id: "detail-film-tv", title: "A film's page on a television", in: detail("detail", "tv", map[string]string{"item": "103"}, movieData(), nil),
			checks: []string{"the header's facts, buttons and pictures", "Resume is primary; Play from the start follows it", "focus starts on the header"}},
		{id: "detail-film-phone", title: "A film's page on a phone", in: detail("detail", "phone", map[string]string{"item": "103"}, movieData(), nil), checks: []string{"the overview is cut to the phone's limit"}},
		{id: "detail-film-kids", title: "A film's page for a profile without downloads or sharing", in: detail("detail", "tablet", map[string]string{"item": "103"}, movieData(), func(in *Input) {
			in.Data.Subject.Can = []string{"play", "watched"}
			in.Data.Subject.Progress, in.Data.Subject.ResumeSec = 0, 0
		}), checks: []string{"buttons the item can't do are left out", "Play, not Resume, without progress"}},
		{id: "detail-show", title: "A show's page", in: detail("detail-show", "desktop", map[string]string{"show": "1"}, showData(), nil),
			checks: []string{"the season the source chose is selected", "each season has a URL of its own", "episode cards: \"<n>. <title>\", running time, synopsis", "Play starts the next episode (nextId)"},
			more:   []SectionAnswer{{Section: "episodes", Cursor: 0, Season: "1"}, {Section: "episodes", Cursor: 50, Season: "2"}}},
		{id: "detail-show-tv", title: "A show's page on a television", in: detail("detail-show", "tv", map[string]string{"show": "1"}, showData(), nil), checks: []string{"thirty episodes in the first page, the rest by cursor"}},
		{id: "detail-show-fallbacks", title: "A show's page for a client with neither DetailHeader nor SeasonList", in: detail("detail-show", "phone", map[string]string{"show": "1"}, showData(), func(in *Input) { in.Request.AllOptional = &no }),
			checks: []string{"the header as Image, Text and a row of Buttons", "the episodes as a Shelf of landscape cards"}},
		{id: "detail-album", title: "An album's page", in: detail("detail-album", "desktop", albumVars, albumData(), nil),
			checks: []string{"the header uses the square cover", "tracks numbered, with lengths", "a track's label: Track 1, Song 1, 3 minutes"}},
		{id: "detail-album-fallback", title: "An album's page for a client without TrackList", in: detail("detail-album", "tv", albumVars, albumData(), func(in *Input) { in.Request.Components = []string{"DetailHeader"} }),
			checks: []string{"the tracks as a List of ListItems that play"}},
		{id: "detail-artist", title: "An artist's page", in: detail("detail-artist", "phone", artistVars, artistData(), nil),
			checks: []string{"the route is /artist/3?name=The+Examples", "a poster header with the square picture, the album count and Play", "the artist's albums as album cards", "every song as a TrackList without track numbers"}},
		{id: "detail-artist-tv", title: "An artist's page on a television", in: detail("detail-artist", "tv", artistVars, artistData(), nil),
			checks: []string{"the same sections, sized for the television", "Play is focused first"}},
		{id: "detail-collection", title: "A collection's page", in: detail("detail-collection", "tablet", map[string]string{"collection": "12"},
			Data{Subject: subject(sdui.Item{ID: "collection-12", Kind: "collection", Title: "Family film night", Overview: "Picked by everyone, vetoed by no one.", Count: 4, Can: []string{"share"}}),
				Sources: []CannedSource{src(map[string]any{"type": "collection", "collection": "12"}, films(4), 0)}}, nil), checks: []string{"a compact header with a count", "the grid of what it holds"}},
		{id: "detail-missing", title: "A title that isn't there, or isn't this profile's to see", in: detail("detail", "phone", map[string]string{"item": "404"}, Data{}, nil),
			checks: []string{"the server answers 404; there is no document"}},

		{id: "search-idle", title: "Search before anything is typed", in: Input{Page: stock("search"), Request: request("tv"), Data: homeData()},
			checks: []string{"the search box has focus (autofocus)", "the hint is shown; no rows"}},
		{id: "search-results", title: "Search with results", in: func() Input {
			in := Input{Page: stock("search"), Request: request("desktop"), Data: Data{Each: homeData().Each, Sources: []CannedSource{
				src(map[string]any{"type": "search", "library": "1"}, films(3), 0),
				src(map[string]any{"type": "search", "library": "3"}, albums(1), 0),
				src(map[string]any{"type": "search", "in": "channels"}, channels(1), 0)}}}
			in.Request.Vars = map[string]string{"query": "exam"}
			in.Request.Flags = map[string]bool{"hasQuery": true}
			return in
		}(), checks: []string{"a row per library with matches, titled with the library's name", "libraries without matches are left out", "the box holds the query"}},

		{id: "livetv-native-guide", title: "Live TV on a client with its own guide", in: func() Input {
			in := Input{Page: stock("livetv"), Request: request("tv"), Data: homeData()}
			in.Request.NativeViews = []string{"guide"}
			return in
		}(), checks: []string{"one NativeView node: guide"}},
		{id: "livetv-channels", title: "Live TV on a client without one", in: Input{Page: stock("livetv"), Request: request("phone"), Data: homeData()},
			checks: []string{"channel cards: number, name, now, next and progress through the programme (19:30 of 19:00 to 20:00 is 50 percent)", "the recordings row is empty and left out"}},

		{id: "custom-page-desktop", title: "A hand-made page with every layout component", in: Input{Page: custom(), Request: func() Request {
			r := request("desktop")
			r.Viewport = "1280x800"
			r.Scopes = map[string]map[string]string{"family": {"accent": "#C2185B"}}
			return r
		}(), Data: customData()}, checks: []string{"Markdown text is sent as written; its label is the plain text", "a row of buttons is one focus group", "a ListItem without a destination is text, not a button", "Split holds exactly two children"}},
		{id: "custom-page-tv", title: "The same page on a television", in: Input{Page: custom(), Request: request("tv"), Data: customData()},
			checks: []string{"a button that opens a web page is left out where there is no browser"}},
		{id: "card-styles", title: "Every card style", in: Input{Page: styles(), Request: func() Request {
			r := request("desktop")
			r.Viewport = "1440x900"
			return r
		}(), Data: stylesData()}, checks: []string{"what each style shows of an item, and in which order", "at most three badges", "the picture each style asks for: kind and width bucket", "a tile without a picture carries an icon", "a chip has no picture"}},
		{id: "card-styles-tv-4k", title: "Every card style on an Apple TV 4K", in: Input{Page: styles(), Request: func() Request {
			r := request("tv")
			r.DeviceClass = "4k"
			return r
		}(), Data: stylesData()}, checks: []string{"picture buckets at scale 2", "rows past the budget are deferred with placeholders"}},
		{id: "long-text", title: "Text longer than a device shows", in: Input{Page: page("long", "detail", sec("header", "DetailHeader", nil, nil),
			sec("row", "Shelf", map[string]any{"title": strings.Repeat("A very long row title ", 5)}, sdui.Source{"type": "similar", "item": "$item"})), Request: func() Request {
			r := request("phone")
			r.Vars = map[string]string{"item": "101", "title": long.Title}
			return r
		}(), Data: Data{Subject: &long, Sources: []CannedSource{src(map[string]any{"type": "similar"}, []sdui.Item{long}, 0)}}},
			checks: []string{"titles are cut to 80 characters at a word, with an ellipsis", "the overview to 400", "nothing is cut mid-word when a space is near"}},
		{id: "map-override", title: "A source that maps another field into the subtitle", in: Input{Page: page("mapped", "custom",
			sec("row", "Shelf", map[string]any{"title": "By studio"}, sdui.Source{"type": "browse", "sort": "title", "map": map[string]any{"subtitle": "genres"}})), Request: request("desktop"),
			Data: Data{Sources: []CannedSource{src(map[string]any{"type": "browse"}, films(3), 0)}}}, checks: []string{"the subtitle is the item's genres, not its year"}},
		{id: "odd-ids", title: "Items with awkward and repeated IDs", in: Input{Page: page("odd", "custom", sec("row", "Shelf", map[string]any{"title": "Odd"}, sdui.Source{"type": "browse"})), Request: request("phone"),
			Data: Data{Sources: []CannedSource{src(map[string]any{"type": "browse"}, []sdui.Item{
				{ID: "a b/c", Kind: "movie", Title: "Spaces and a slash", Route: "/item/1"}, {ID: "abc", Kind: "movie", Title: "The same after cleaning", Route: "/item/2"},
				{ID: "abc", Kind: "movie", Title: "A true repeat", Route: "/item/3"}, {ID: "", Kind: "movie", Title: "No ID at all"}}, 0)}}},
			checks: []string{"node IDs keep only letters, digits, . _ and -", "a repeated ID gets ~2, ~3", "an item without a route is drawn but has no action and takes no focus"}},
		{id: "banners", title: "Banners written by hand and from a source", in: Input{Page: page("banners", "custom",
			sec("guide", "Banner", map[string]any{"title": "TV guide", "text": "Everything on now.", "buttonLabel": "Open the guide", "native": "guide", "icon": "guide", "style": "compact"}, nil),
			sec("pick", "Banner", map[string]any{"title": "Tonight: {title}", "text": "{overview}", "buttonLabel": "Details"}, sdui.Source{"type": "random", "limit": 1}),
			sec("none", "Banner", map[string]any{"title": "Never shown"}, sdui.Source{"type": "watchLater", "limit": 1})), Request: request("tablet"),
			Data: Data{Sources: []CannedSource{src(map[string]any{"type": "random"}, films(1), 0)}}},
			checks: []string{"a banner written by hand opens a native screen", "a banner from a source takes its title, text, picture and destination from the first item", "a banner whose source is empty is left out"}},
	}, moduleCases()...)
}

// The modules' cases (docs/SDUI-MODULES.md): the worked example module's page,
// its row on Home, and a page that names a module that isn't installed. What a
// module adds reaches a renderer as the catalogue's own components.

func boardGamesUI() sdui.ModuleUI {
	b, err := os.ReadFile(filepath.Join("..", "testdata", "module-boardgames.json"))
	if err != nil {
		panic(err)
	}
	ui, err := sdui.ParseModuleUI(b)
	if err != nil {
		panic(err)
	}
	return *ui
}

func boardGame(i int) sdui.Item {
	id := "bg-" + strconv.Itoa(i)
	names := []string{"Harbour Lights", "Seven Lanterns", "Paper Rivers", "The Quiet Orchard", "Copper Kites", "Moss and Stone", "Night Market", "Tidewater"}
	return sdui.Item{ID: id, Kind: "game", Title: names[(i-1)%len(names)], Subtitle: strconv.Itoa(2+i%4) + "–" + strconv.Itoa(4+i%3) + " players",
		Images: map[string]string{"square": "/api/v1/sdui/modules/wittyboardgames/images/" + id + ".png"}, Route: "/page/wittyboardgames", Scope: "wittyboardgames"}
}

func boardGames(n int) []sdui.Item {
	out := make([]sdui.Item, n)
	for i := range out {
		out[i] = boardGame(i + 1)
	}
	return out
}

func moduleData() Data {
	return Data{Sources: []CannedSource{
		src(map[string]any{"type": "wittyboardgames.shelf", "sort": "shuffle"}, boardGames(6), 0),
		src(map[string]any{"type": "wittyboardgames.shelf"}, boardGames(8), 14),
		src(map[string]any{"type": "continueWatching"}, films(3), 0),
	}}
}

func moduleRequest(ff string) Request {
	r := request(ff)
	r.Modules = []string{"wittyboardgames"}
	r.Scopes = map[string]map[string]string{"wittyboardgames": {"accent": "#2E7D32"}}
	return r
}

func moduleCases() []kase {
	ui := boardGamesUI()
	set := sdui.NewModuleSet([]sdui.ModuleUI{ui})
	pg, _ := set.Page("wittyboardgames")
	home, _ := sdui.StockPage("home")
	home = set.WithInserts(home)
	friday := moduleRequest("phone")
	friday.Now = "2026-11-13T19:30:00Z"
	web := moduleRequest("web")
	web.Viewport = "1440x900"
	web.NativeViews = []string{"wittyboardgames.scoreboard"}
	missing := page("mine", "custom",
		sec("night", "wittyboardgames.GameNight", nil, nil),
		sec("shelf", "Shelf", map[string]any{"title": "Board games"}, sdui.Source{"type": "wittyboardgames.shelf"}),
		sec("jump", "Shelf", map[string]any{"title": "Continue watching"}, sdui.Source{"type": "continueWatching", "media": "all"}))
	return []kase{
		{id: "module-page-tv", title: "A module's own page on an Apple TV", in: Input{Page: pg, Request: moduleRequest("tv"), Data: moduleData(), ModuleUI: []sdui.ModuleUI{ui}},
			checks: []string{"a module's component reaches the app as the catalogue component it stands for (a Shelf), with its properties filled in", "a module's source gives cards like any other, in the source's card style", "a module's page takes the module's theme scope and colours", "a NativeView of a module's view the app doesn't have is sent its sections instead"}},
		{id: "module-page-web", title: "A module's own page in a browser that has the module's view", in: Input{Page: pg, Request: web, Data: moduleData(), ModuleUI: []sdui.ModuleUI{ui}},
			checks: []string{"an app that says it has a module's view (views=wittyboardgames.scoreboard) is sent the NativeView", "the rest is the same as on any device"}},
		{id: "module-home-insert", title: "A module's row on Home, on a Friday, on a phone", in: Input{Page: home, Request: friday, Data: moduleData(), ModuleUI: []sdui.ModuleUI{ui}},
			checks: []string{"a module's insert sits where it asked to (after jump)", "an inserted section carries the module's scope and colours", "its condition is the module's, combined with the section's"}},
		{id: "module-not-installed", title: "A page that uses a module that isn't installed", in: Input{Page: missing, Request: request("phone"), Data: moduleData()},
			checks: []string{"a module's component or source with no module behind it is left out, never sent", "the rest of the page is drawn"}},
	}
}
