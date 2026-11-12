// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import "sync"

// The built-in pages and navigation: what every server shows until an
// administrator changes something. They are written in the same model an
// administrator writes in, so "Reset to default" is deleting a page from the
// configuration, and a server upgrade improves every page nobody touched.
//
// They reproduce the app as it was built by hand: Home's widgets carry the
// same IDs the web app's Home used (hero, libraries, jump, onnow, foryou,
// nextup, view-<id>, recent-<library>, shuffle), so a profile's saved order
// of Home carries over.

type props = map[string]any

func sec(id, component string, p props, src Source) Section {
	return Section{ID: id, Component: component, Props: p, Source: src}
}

func when(s Section, c Condition) Section { s.When = &c; return s }
func scoped(s Section, scope string) Section {
	s.Scope = scope
	return s
}
func repeated(s Section, over string, kinds ...string) Section {
	s.Repeat = &Repeat{Over: over, Kinds: kinds}
	return s
}
func with(s Section, children ...Section) Section { s.Children = children; return s }

var yes, no = true, false

// videoKinds are the kinds of library whose page is a filtered grid.
var videoKinds = []string{"movies", "shows", "anime"}

func buildStockPages() []Page {
	notTV := Condition{Not: &Condition{FormFactor: []string{FormTV}}}
	home := Page{ID: "home", Kind: "home", Title: "Home", Icon: "home",
		Description: "The first page: a spotlight, what you were in the middle of, what's on, what's new in each library.",
		Sections: []Section{
			when(sec("setup", "Notice", props{"tone": "info", "title": "Finish setting up", "text": "A few things are left to choose before this server is ready for everyone.", "buttonLabel": "Open settings", "native": "settings"}, nil),
				Condition{Admin: &yes, Flag: "setupIncomplete"}),
			when(sec("welcome", "Notice", props{"tone": "info", "title": "Welcome, {user}", "text": "There are no libraries yet. Add one in Settings, Libraries, and what's in its folders appears here.", "buttonLabel": "Open settings", "native": "settings"}, nil),
				Condition{Flag: "librariesEmpty"}),
			sec("hero", "Hero", props{"title": "Featured"}, Source{"type": "featured"}),
			when(sec("libraries", "Shelf", props{"title": "Your libraries", "cardStyle": "tile", "seeAll": "never"}, Source{"type": "libraries"}), notTV),
			sec("jump", "Shelf", props{"title": "Jump back in"}, Source{"type": "continueWatching", "media": "all"}),
			when(scoped(sec("onnow", "Shelf", props{"title": "On TV now"}, Source{"type": "liveNow"}), "wittytv"), Condition{Can: "liveTv", Feature: "live-tv"}),
			sec("foryou", "Shelf", props{"title": "Recommended for you"}, Source{"type": "recommended", "limit": 20}),
			sec("nextup", "Shelf", props{"title": "Next up", "cardStyle": "progress"}, Source{"type": "nextUp"}),
			repeated(sec("view", "Shelf", props{"title": "{each}"}, Source{"type": "savedView", "view": "$each", "limit": 20}), "pinnedViews"),
			repeated(sec("recent", "Shelf", props{"title": "Recently added in {each}", "seeAll": "always"}, Source{"type": "recentlyAdded", "library": "$each", "limit": 20}), "libraries"),
			sec("shuffle", "Banner", props{"title": "Can't decide?", "text": "{title}", "buttonLabel": "Play something", "action": "play", "style": "compact", "icon": "shuffle"}, Source{"type": "random", "limit": 1, "reshuffle": "hourly"}),
		}}

	libraries := Page{ID: "libraries", Kind: "libraries", Title: "Libraries", Icon: "library",
		Sections: []Section{sec("all", "Grid", props{"cardStyle": "tile", "empty": "message", "emptyText": "No libraries yet."}, Source{"type": "libraries"})}}

	library := Page{ID: "library", Kind: "library", Title: "{library}", Icon: "library",
		Description: "Any library without a page of its own kind: everything in it, newest first.",
		Sections:    []Section{sec("all", "Grid", props{"empty": "message", "emptyText": "Nothing has been found in this library's folders yet."}, Source{"type": "recentlyAdded", "library": "$library"})}}

	var videoLibs []Page
	for _, kind := range videoKinds {
		videoLibs = append(videoLibs, Page{ID: "library-" + kind, Kind: "library", Title: "{library}", Icon: LibraryIcon(kind), Match: &Match{LibraryKind: kind},
			Description: "A library of films or shows: filters and sorting over a grid.",
			Sections: []Section{
				sec("filters", "FilterBar", props{"target": "all"}, nil),
				sec("all", "Grid", props{"empty": "message", "emptyText": "Nothing matches these filters."}, Source{"type": "browse", "library": "$library", "filterable": true}),
			}})
	}

	musicLib := Page{ID: "library-music", Kind: "library", Title: "{library}", Icon: "music", Match: &Match{LibraryKind: "music"}, Scope: "wittymusic",
		Sections: []Section{
			sec("played", "Shelf", props{"title": "Recently played"}, Source{"type": "albums", "library": "$library", "sort": "played", "limit": 15}),
			with(sec("views", "Tabs", props{"style": "segments"}, nil),
				with(sec("albums", "Tab", props{"label": "Albums", "icon": "album"}, nil),
					sec("albums-grid", "Grid", props{"empty": "message", "emptyText": "No albums yet."}, Source{"type": "albums", "library": "$library", "sort": "added"})),
				with(sec("artists", "Tab", props{"label": "Artists", "icon": "artist"}, nil),
					sec("artists-grid", "Grid", nil, Source{"type": "artists", "library": "$library"}))),
		}}
	var spokenLibs []Page
	for _, kind := range []string{"audiobooks", "podcasts"} {
		spokenLibs = append(spokenLibs, Page{ID: "library-" + kind, Kind: "library", Title: "{library}", Icon: LibraryIcon(kind), Match: &Match{LibraryKind: kind},
			Description: "Audiobooks or podcasts: what you were listening to, then every book or show.",
			Sections: []Section{
				sec("listening", "Shelf", props{"title": "Continue listening"}, Source{"type": "continueWatching", "media": "listen"}),
				sec("all", "Grid", props{"cardStyle": "square", "empty": "message", "emptyText": "Nothing here yet."}, Source{"type": "albums", "library": "$library", "sort": "title"}),
			}})
	}
	photosLib := Page{ID: "library-photos", Kind: "library", Title: "{library}", Icon: "photo", Match: &Match{LibraryKind: "photos"},
		Sections: []Section{with(sec("timeline", "NativeView", props{"view": "photoTimeline", "library": "$library"}, nil),
			sec("all", "Grid", props{"cardStyle": "photo", "showTitles": false, "empty": "message", "emptyText": "No photos yet."}, Source{"type": "recentlyAdded", "library": "$library"}))}}
	// The bookshelf as WittyInk's has it: every book by title, then a row for each series in
	// its own order, and for each author by series.
	booksLib := Page{ID: "library-books", Kind: "library", Title: "{library}", Icon: "book", Match: &Match{LibraryKind: "books"},
		Description: "Books and comics: what you were reading, then the bookshelf: every book, each series and each author.",
		Sections: []Section{
			sec("reading", "Shelf", props{"title": "Continue reading", "cardStyle": "book"}, Source{"type": "continueWatching", "media": "read"}),
			with(sec("shelf", "Tabs", props{"style": "segments"}, nil),
				with(sec("books", "Tab", props{"label": "Books", "icon": "book"}, nil),
					sec("all", "Grid", props{"cardStyle": "book", "empty": "message", "emptyText": "No books yet."}, Source{"type": "books", "library": "$library"})),
				with(sec("series", "Tab", props{"label": "Series", "icon": "list"}, nil),
					repeated(sec("in-series", "Shelf", props{"title": "{each}", "cardStyle": "book"}, Source{"type": "books", "library": "$library", "series": "$each"}), "bookSeries")),
				with(sec("authors", "Tab", props{"label": "Authors", "icon": "user"}, nil),
					repeated(sec("by-author", "Shelf", props{"title": "{each}", "cardStyle": "book"}, Source{"type": "books", "library": "$library", "author": "$each", "sort": "series"}), "bookAuthors"))),
		}}
	gamesLib := Page{ID: "library-games", Kind: "library", Title: "{library}", Icon: "game", Match: &Match{LibraryKind: "games"}, Scope: "wittyplay",
		Sections: []Section{
			sec("playing", "Shelf", props{"title": "Continue playing"}, Source{"type": "continuePlaying", "library": "$library"}),
			sec("all", "Grid", props{"cardStyle": "game", "empty": "message", "emptyText": "No games yet."}, Source{"type": "recentlyAdded", "library": "$library"}),
		}}

	detail := Page{ID: "detail", Kind: "detail", Title: "{title}", Icon: "info",
		Description: "A film, a video, a book, a game: its header and more like it.",
		Sections: []Section{
			sec("header", "DetailHeader", nil, nil),
			sec("similar", "Shelf", props{"title": "More like this"}, Source{"type": "similar", "item": "$item", "limit": 12}),
		}}
	detailShow := Page{ID: "detail-show", Kind: "detail", Title: "{title}", Icon: "show", Match: &Match{MediaKind: "show"},
		Sections: []Section{
			sec("header", "DetailHeader", props{"facts": []string{"year", "contentRating", "rating", "genres", "count"}, "actions": []string{"play", "watchLater", "share"}}, nil),
			sec("episodes", "SeasonList", nil, Source{"type": "episodes", "show": "$show"}),
			sec("similar", "Shelf", props{"title": "More like this"}, Source{"type": "similar", "item": "$show", "limit": 12}),
		}}
	detailEpisode := Page{ID: "detail-episode", Kind: "detail", Title: "{title}", Icon: "show", Match: &Match{MediaKind: "episode"},
		Sections: []Section{
			sec("header", "DetailHeader", props{"facts": []string{"season", "runtime", "contentRating", "year"}, "actions": []string{"play", "restart", "watched", "download", "share", "playOn", "watchParty"}}, nil),
			sec("season", "Shelf", props{"title": "More from this season", "cardStyle": "episode", "seeAll": "never"}, Source{"type": "episodes", "show": "$show", "season": "$season"}),
		}}
	detailAlbum := Page{ID: "detail-album", Kind: "detail", Title: "{title}", Icon: "album", Match: &Match{MediaKind: "album"}, Scope: "wittymusic",
		Sections: []Section{
			sec("header", "DetailHeader", props{"layout": "poster", "facts": []string{"year", "count", "runtime", "genres"}, "actions": []string{"play", "share"}}, nil),
			sec("tracks", "TrackList", nil, Source{"type": "tracks", "library": "$library", "artist": "$artist", "album": "$album"}),
			sec("more", "Shelf", props{"title": "More by this artist"}, Source{"type": "albums", "library": "$library", "artist": "$artist", "sort": "year"}),
		}}
	detailArtist := Page{ID: "detail-artist", Kind: "detail", Title: "{title}", Icon: "artist", Match: &Match{MediaKind: "artist"}, Scope: "wittymusic",
		Description: "A music artist: their albums, newest first, and every song of theirs.",
		Sections: []Section{
			sec("header", "DetailHeader", props{"layout": "poster", "facts": []string{"count"}, "actions": []string{"play"}}, nil),
			sec("albums", "Shelf", props{"title": "Albums", "cardStyle": "album", "seeAll": "never"}, Source{"type": "albums", "library": "$library", "artist": "$artist", "sort": "year"}),
			// Songs from every album, by title: an album's track numbers would repeat.
			sec("tracks", "TrackList", props{"showNumbers": false}, Source{"type": "tracks", "library": "$library", "artist": "$artist"}),
		}}
	detailCollection := Page{ID: "detail-collection", Kind: "detail", Title: "{title}", Icon: "collection", Match: &Match{MediaKind: "collection"},
		Sections: []Section{
			sec("header", "DetailHeader", props{"layout": "compact", "facts": []string{"count"}, "actions": []string{"share"}}, nil),
			sec("all", "Grid", props{"empty": "message", "emptyText": "Nothing in this collection yet."}, Source{"type": "collection", "collection": "$collection"}),
		}}

	search := Page{ID: "search", Kind: "search", Title: "Search", Icon: "search",
		Sections: []Section{
			sec("q", "SearchBox", props{"placeholder": "Films, shows, music, books, channels", "autofocus": true}, nil),
			when(sec("idle", "Text", props{"text": "Type at least two letters to search every library you can open.", "role": "caption"}, nil), Condition{Not: &Condition{Flag: "hasQuery"}}),
			repeated(sec("found", "Shelf", props{"title": "{each}", "seeAll": "never"}, Source{"type": "search", "query": "$query", "library": "$each"}), "libraries"),
			when(scoped(sec("channels", "Shelf", props{"title": "Channels", "seeAll": "never"}, Source{"type": "search", "query": "$query", "in": "channels"}), "wittytv"), Condition{Can: "liveTv"}),
		}}

	livetv := Page{ID: "livetv", Kind: "livetv", Title: "Live TV", Icon: "live", Scope: "wittytv",
		Sections: []Section{with(sec("guide", "NativeView", props{"view": "guide"}, nil),
			sec("favourites", "Shelf", props{"title": "Favourite channels", "seeAll": "never"}, Source{"type": "liveNow", "favourites": true}),
			when(sec("recordings", "Shelf", props{"title": "Recordings"}, Source{"type": "recordings"}), Condition{Can: "recordings"}),
			sec("channels", "Grid", props{"title": "All channels", "empty": "message", "emptyText": "No channels yet. An administrator can add them in Settings, Live TV."}, Source{"type": "liveNow"}),
		)}}

	music := Page{ID: "music", Kind: "music", Title: "Music", Icon: "music", Scope: "wittymusic",
		Sections: []Section{
			sec("listening", "Shelf", props{"title": "Continue listening"}, Source{"type": "continueWatching", "media": "listen"}),
			repeated(sec("played", "Shelf", props{"title": "Recently played in {each}"}, Source{"type": "albums", "library": "$each", "sort": "played", "limit": 15}), "libraries", "music"),
			repeated(sec("added", "Shelf", props{"title": "Recently added in {each}", "seeAll": "always"}, Source{"type": "albums", "library": "$each", "sort": "added", "limit": 30}), "libraries", "music"),
			repeated(sec("artists", "Shelf", props{"title": "Artists in {each}"}, Source{"type": "artists", "library": "$each"}), "libraries", "music"),
		}}
	photos := Page{ID: "photos", Kind: "photos", Title: "Photos", Icon: "photo",
		Sections: []Section{repeated(sec("recent", "Shelf", props{"title": "{each}", "cardStyle": "photo", "showTitles": false, "seeAll": "always"}, Source{"type": "recentlyAdded", "library": "$each"}), "libraries", "photos")}}
	books := Page{ID: "books", Kind: "books", Title: "Books", Icon: "book",
		Sections: []Section{
			sec("reading", "Shelf", props{"title": "Continue reading", "cardStyle": "book"}, Source{"type": "continueWatching", "media": "read"}),
			repeated(sec("recent", "Shelf", props{"title": "New in {each}", "cardStyle": "book", "seeAll": "always"}, Source{"type": "recentlyAdded", "library": "$each"}), "libraries", "books", "audiobooks"),
		}}
	games := Page{ID: "games", Kind: "games", Title: "Games", Icon: "game", Scope: "wittyplay",
		Sections: []Section{
			sec("playing", "Shelf", props{"title": "Continue playing"}, Source{"type": "continuePlaying"}),
			repeated(sec("recent", "Shelf", props{"title": "{each}", "cardStyle": "game", "seeAll": "always"}, Source{"type": "recentlyAdded", "library": "$each"}), "libraries", "games"),
		}}
	collections := Page{ID: "collections", Kind: "collections", Title: "Collections", Icon: "collection",
		Sections: []Section{
			sec("later", "Shelf", props{"title": "Watch later"}, Source{"type": "watchLater"}),
			sec("all", "Grid", props{"cardStyle": "tile", "empty": "message", "emptyText": "No collections yet."}, Source{"type": "collections"}),
		}}

	out := []Page{home, libraries, library}
	out = append(out, videoLibs...)
	out = append(out, musicLib)
	out = append(out, spokenLibs...)
	out = append(out, photosLib, booksLib, gamesLib, detail, detailShow, detailEpisode, detailAlbum, detailArtist, detailCollection, search, livetv, music, photos, books, games, collections)
	return out
}

var stockPagesOnce = sync.OnceValue(func() []Page {
	pages := buildStockPages()
	for i := range pages {
		pages[i] = ClonePage(pages[i]) // so every prop is what JSON would hold
		if err := AsError(ValidatePage(&pages[i])); err != nil {
			panic("sdui: the built-in page " + pages[i].ID + " is invalid: " + err.Error())
		}
	}
	return pages
})

// StockPages returns the built-in pages, in the order an editor lists them.
// The result is a copy.
func StockPages() []Page {
	src := stockPagesOnce()
	out := make([]Page, len(src))
	for i := range src {
		out[i] = ClonePage(src[i])
	}
	return out
}

// StockPage returns one built-in page by ID.
func StockPage(id string) (Page, bool) {
	for _, p := range stockPagesOnce() {
		if p.ID == id {
			return ClonePage(p), true
		}
	}
	return Page{}, false
}

// StockNavigation is the built-in main menu: Home, Search, Live TV and the
// libraries, with the app's own profile screen and Settings at the end. On a
// phone the bar holds Home, Search and Live TV and the libraries sit behind
// More; a television has a Libraries entry in place of one per library.
func StockNavigation() Navigation {
	tvOnly := Condition{FormFactor: []string{FormTV}}
	return Navigation{Items: []NavItem{
		{ID: "home", Type: "page", Page: "home"},
		{ID: "search", Type: "page", Page: "search", Placement: map[string]string{FormDesktop: "end", FormTV: "primary"}},
		{ID: "livetv", Type: "page", Page: "livetv", When: &Condition{Can: "liveTv", Feature: "live-tv"}},
		{ID: "all-libraries", Type: "page", Page: "libraries", When: &tvOnly},
		{ID: "libraries", Type: "libraries", Placement: map[string]string{FormTV: "hidden", FormPhone: "more", FormTablet: "primary", FormDesktop: "primary"}},
		{ID: "collections", Type: "page", Page: "collections", Placement: map[string]string{FormPhone: "more", FormTV: "more"}},
		{ID: "you", Type: "native", Screen: "profiles", Label: "You", Placement: map[string]string{FormTV: "end", FormPhone: "primary", FormDesktop: "end", FormTablet: "end"}},
		{ID: "settings", Type: "native", Screen: "settings", Placement: map[string]string{FormTV: "end", FormPhone: "more", FormDesktop: "end", FormTablet: "end"}},
	}}
}

// SelectPage picks the page for a request from every page of a kind: the
// one matching the library's ID, else its kind, else the media kind, else the
// page with no match at all. Among equals the one named first wins, so a
// configured page (listed before the built-in ones) beats a built-in page.
func SelectPage(pages []Page, kind string, library int64, libraryKind, mediaKind string) (Page, bool) {
	best, bestScore := -1, -1
	for i := range pages {
		p := &pages[i]
		if p.Kind != kind {
			continue
		}
		score := 0
		if m := p.Match; m != nil {
			if (m.Library != 0 && m.Library != library) || (m.LibraryKind != "" && m.LibraryKind != libraryKind) || (m.MediaKind != "" && m.MediaKind != mediaKind) {
				continue
			}
			if m.Library != 0 {
				score += 4
			}
			if m.MediaKind != "" {
				score += 2
			}
			if m.LibraryKind != "" {
				score++
			}
		}
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	if best < 0 {
		return Page{}, false
	}
	return pages[best], true
}
