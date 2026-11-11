// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import "sync"

// Preset is something ready-made an administrator applies in one step and
// then changes: a whole page, or one section to drop into a page.
type Preset struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	// Kind is "page" or "section".
	Kind string `json:"kind"`
	// PageKind is the kind of page a page preset makes, or the kinds a
	// section preset suits (empty: any).
	PageKinds []string `json:"pageKinds,omitempty"`
	Page      *Page    `json:"page,omitempty"`
	Section   *Section `json:"section,omitempty"`
}

func pagePreset(id, label, icon, description string, p Page) Preset {
	return Preset{ID: id, Label: label, Icon: icon, Description: description, Kind: "page", PageKinds: []string{p.Kind}, Page: &p}
}

func sectionPreset(id, label, icon, description string, s Section, kinds ...string) Preset {
	return Preset{ID: id, Label: label, Icon: icon, Description: description, Kind: "section", PageKinds: kinds, Section: &s}
}

func buildPresets() []Preset {
	kids := Condition{ProfileKind: []string{"kids"}}
	recent := repeated(sec("recent", "Shelf", props{"title": "Recently added in {each}", "seeAll": "always"}, Source{"type": "recentlyAdded", "library": "$each", "limit": 20}), "libraries")
	jump := sec("jump", "Shelf", props{"title": "Continue watching"}, Source{"type": "continueWatching", "media": "all"})
	nextup := sec("nextup", "Shelf", props{"title": "Next up", "cardStyle": "progress"}, Source{"type": "nextUp"})
	hero := sec("hero", "Hero", props{"title": "Featured"}, Source{"type": "featured"})
	stock, _ := StockPage("home")

	streaming := Page{ID: "home", Kind: "home", Title: "Home", Icon: "home", Sections: []Section{
		hero, jump, nextup,
		sec("foryou", "Shelf", props{"title": "Recommended for you"}, Source{"type": "recommended", "limit": 20}),
		sec("top", "Shelf", props{"title": "Top 10 on this server", "numbered": true, "cardSize": "l", "seeAll": "never"}, Source{"type": "topRated", "limit": 10}),
		sec("new", "Shelf", props{"title": "New releases"}, Source{"type": "recentlyReleased", "limit": 20}),
		when(scoped(sec("onnow", "Shelf", props{"title": "On TV now"}, Source{"type": "liveNow"}), "wittytv"), Condition{Can: "liveTv", Feature: "live-tv"}),
		sec("later", "Shelf", props{"title": "Watch later"}, Source{"type": "watchLater"}),
		recent,
		sec("random", "Shelf", props{"title": "Something different", "seeAll": "never"}, Source{"type": "random", "limit": 12, "watched": "no"}),
	}}
	minimal := Page{ID: "home", Kind: "home", Title: "Home", Icon: "home", Sections: []Section{
		sec("hello", "Text", props{"text": "{greeting}, {user}", "role": "pageTitle"}, nil),
		jump,
		sec("libraries", "Grid", props{"cardStyle": "tile", "cardSize": "l"}, Source{"type": "libraries"}),
	}}
	kidsHome := Page{ID: "home", Kind: "home", Title: "Home", Icon: "home", Sections: stock.Sections,
		Variants: []Variant{{ID: "kids", Label: "Kids profiles", When: &kids, Sections: []Section{
			sec("hello", "Text", props{"text": "Hi {user}!", "role": "pageTitle"}, nil),
			sec("jump", "Shelf", props{"title": "Keep watching", "cardSize": "l"}, Source{"type": "continueWatching", "media": "watch"}),
			sec("nextup", "Shelf", props{"title": "Next episode", "cardStyle": "progress", "cardSize": "l"}, Source{"type": "nextUp"}),
			repeated(sec("recent", "Shelf", props{"title": "New in {each}", "cardSize": "l", "seeAll": "always"}, Source{"type": "recentlyAdded", "library": "$each", "limit": 16}), "libraries"),
			sec("animation", "Shelf", props{"title": "Cartoons", "cardSize": "l"}, Source{"type": "genre", "genre": "Animation", "sort": "shuffle"}),
		}}}}
	musicFirst := Page{ID: "home", Kind: "home", Title: "Home", Icon: "home", Sections: []Section{
		scoped(sec("listening", "Shelf", props{"title": "Continue listening"}, Source{"type": "continueWatching", "media": "listen"}), "wittymusic"),
		scoped(repeated(sec("played", "Shelf", props{"title": "Recently played", "cardSize": "l"}, Source{"type": "albums", "library": "$each", "sort": "played", "limit": 15}), "libraries", "music"), "wittymusic"),
		scoped(repeated(sec("added", "Shelf", props{"title": "New in {each}", "seeAll": "always"}, Source{"type": "albums", "library": "$each", "sort": "added", "limit": 30}), "libraries", "music"), "wittymusic"),
		scoped(repeated(sec("artists", "Shelf", props{"title": "Artists"}, Source{"type": "artists", "library": "$each"}), "libraries", "music"), "wittymusic"),
		sec("jump", "Shelf", props{"title": "Continue watching"}, Source{"type": "continueWatching", "media": "watch"}),
		repeated(sec("recent", "Shelf", props{"title": "Recently added in {each}", "seeAll": "always"}, Source{"type": "recentlyAdded", "library": "$each", "limit": 20}), "libraries", "movies", "shows", "anime", "videos"),
	}}
	tvFirst := Page{ID: "home", Kind: "home", Title: "Home", Icon: "home", Scope: "wittytv", Sections: []Section{
		when(sec("favourites", "Shelf", props{"title": "Your channels", "cardSize": "l", "seeAll": "always"}, Source{"type": "liveNow", "favourites": true}), Condition{Can: "liveTv"}),
		when(sec("onnow", "Shelf", props{"title": "On now"}, Source{"type": "liveNow"}), Condition{Can: "liveTv"}),
		when(sec("guide", "Banner", props{"title": "TV guide", "text": "Everything on now and for the next two weeks.", "buttonLabel": "Open the guide", "native": "guide", "icon": "guide", "style": "compact"}, nil), Condition{Can: "liveTv"}),
		when(sec("recordings", "Shelf", props{"title": "Recordings"}, Source{"type": "recordings"}), Condition{Can: "recordings"}),
		jump, nextup, recent,
	}}
	photoFrame := Page{ID: "photo-frame", Kind: "custom", Title: "Photos", Icon: "photo", Description: "A page of pictures for a screen on the wall: a slow spotlight, then the newest photos.",
		Personal: &PersonalPolicy{Allow: []string{}}, Sections: []Section{
			repeated(sec("show", "Hero", props{"title": "Photos", "style": "billboard", "autoAdvanceSec": 30, "showSynopsis": false, "showLogo": false, "actions": []string{}, "maxItems": 8},
				Source{"type": "recentlyAdded", "library": "$each"}), "libraries", "photos"),
			repeated(sec("recent", "Shelf", props{"title": "{each}", "cardStyle": "photo", "cardSize": "l", "showTitles": false, "seeAll": "always"}, Source{"type": "recentlyAdded", "library": "$each"}), "libraries", "photos"),
		}}

	out := []Preset{
		pagePreset("home-default", "The built-in Home", "home", "Home as it ships: spotlight, libraries, jump back in, on TV now, recommendations, next up, pinned views, a row per library and a shuffle.", stock),
		pagePreset("home-streaming", "Streaming-style Home", "sparkles", "A big spotlight, continue watching, recommendations, a numbered top ten, new releases and a row per library.", streaming),
		pagePreset("home-minimal", "Minimal Home", "grid", "A greeting, what you were watching, and large tiles for the libraries. Nothing else.", minimal),
		pagePreset("home-kids", "Home with a kids' variant", "kids", "The built-in Home for everyone, and a simpler one with large cards for Kids profiles.", kidsHome),
		pagePreset("home-music", "Music-first Home", "music", "Music on top: continue listening, recently played and new albums, artists; films and shows below.", musicFirst),
		pagePreset("home-tv", "TV-first Home", "live", "Live TV on top: favourite channels, what's on now, the guide and recordings; the libraries below.", tvFirst),
		pagePreset("page-photo-frame", "Photo frame", "photo", "A page of its own that cycles slowly through the newest photos, for a screen left on. Add it to the navigation, or open /page/photo-frame.", photoFrame),

		sectionPreset("continue-watching", "Continue watching", "history", "Everything the profile is part-way through.", jump),
		sectionPreset("next-up", "Next up", "play", "The next episode of each show in progress.", nextup),
		sectionPreset("spotlight", "Spotlight", "sparkles", "A rotating spotlight of recent films.", hero),
		sectionPreset("recent-per-library", "Recently added, a row per library", "plus", "One row for each library the profile can open.", recent),
		sectionPreset("recommended", "Recommended for you", "sparkles", "Picked from what the profile has watched here.", sec("foryou", "Shelf", props{"title": "Recommended for you"}, Source{"type": "recommended", "limit": 20})),
		sectionPreset("top-ten", "Top 10", "star", "The ten best-scored titles, numbered.", sec("top", "Shelf", props{"title": "Top 10", "numbered": true, "cardSize": "l", "seeAll": "never"}, Source{"type": "topRated", "limit": 10})),
		sectionPreset("genre-row", "A genre", "filter", "One genre as a row. Change the genre to taste.", sec("genre", "Shelf", props{"title": "{genre}"}, Source{"type": "genre", "genre": "Comedy"})),
		sectionPreset("collection-row", "A collection", "collection", "What one collection holds. Set the collection's ID.", sec("collection", "Shelf", props{"title": "Collection"}, Source{"type": "collection", "collection": 1})),
		sectionPreset("pinned-views", "Pinned saved views", "filter", "A row for each saved view the profile pinned to Home.", repeated(sec("view", "Shelf", props{"title": "{each}"}, Source{"type": "savedView", "view": "$each", "limit": 20}), "pinnedViews")),
		sectionPreset("watch-later", "Watch later", "clock", "The profile's Watch later list.", sec("later", "Shelf", props{"title": "Watch later"}, Source{"type": "watchLater"})),
		sectionPreset("random-pick", "Can't decide?", "shuffle", "A banner offering one random title to play.", sec("shuffle", "Banner", props{"title": "Can't decide?", "text": "{title}", "buttonLabel": "Play something", "action": "play", "style": "compact", "icon": "shuffle"}, Source{"type": "random", "limit": 1, "reshuffle": "hourly"})),
		sectionPreset("on-tv-now", "On TV now", "live", "Channels with what is on now and next.", when(scoped(sec("onnow", "Shelf", props{"title": "On TV now"}, Source{"type": "liveNow"}), "wittytv"), Condition{Can: "liveTv"})),
		sectionPreset("guide-entry", "A way into the TV guide", "guide", "A banner that opens the app's TV guide.", sec("guide", "Banner", props{"title": "TV guide", "text": "Everything on now and for the next two weeks.", "buttonLabel": "Open the guide", "native": "guide", "icon": "guide", "style": "compact"}, nil)),
		sectionPreset("library-tiles", "Library tiles", "library", "The libraries as a row of tiles.", sec("libraries", "Shelf", props{"title": "Your libraries", "cardStyle": "tile", "seeAll": "never"}, Source{"type": "libraries"})),
		sectionPreset("genre-chips", "Genre chips", "filter", "A library's genres as chips, each opening the library filtered to it.", sec("genres", "Shelf", props{"title": "Genres", "seeAll": "never"}, Source{"type": "genres", "library": "$library"}), "library"),
		sectionPreset("greeting", "A greeting", "user", "\"Good evening, Sam\" as the page's heading.", sec("hello", "Text", props{"text": "{greeting}, {user}", "role": "pageTitle"}, nil)),
		sectionPreset("search-box", "Search box", "search", "A search field that opens Search.", sec("q", "SearchBox", props{"placeholder": "Search"}, nil)),
		sectionPreset("filter-grid", "Filters over a grid", "filter", "The body of a film or show library: a filter bar and the grid it filters. Add both sections.", sec("all", "Grid", nil, Source{"type": "browse", "library": "$library", "filterable": true}), "library"),
		sectionPreset("title-header", "Title header", "info", "The top of a title's page.", sec("header", "DetailHeader", nil, nil), "detail"),
		sectionPreset("seasons", "Seasons and episodes", "show", "A show's seasons and episodes.", sec("episodes", "SeasonList", nil, Source{"type": "episodes", "show": "$show"}), "detail"),
		sectionPreset("track-list", "Track list", "music", "An album's tracks.", sec("tracks", "TrackList", nil, Source{"type": "tracks", "library": "$library", "artist": "$artist", "album": "$album"}), "detail"),
		sectionPreset("more-like-this", "More like this", "sparkles", "Titles like the one the page is about.", sec("similar", "Shelf", props{"title": "More like this"}, Source{"type": "similar", "item": "$item", "limit": 12}), "detail"),
		sectionPreset("kids-notice", "A note for a kind of profile", "info", "A notice only Kids profiles see. Change the words and the condition.", when(sec("note", "Notice", props{"tone": "info", "text": "Ask a grown-up if you can't find something."}, nil), kids)),
		sectionPreset("evening-row", "A row for the evening", "clock", "A row shown only between 18:00 and 23:00. Change the times and the source.", when(sec("evening", "Shelf", props{"title": "Tonight"}, Source{"type": "random", "limit": 12, "reshuffle": "daily"}), Condition{TimeFrom: "18:00", TimeTo: "23:00"})),
	}
	for i := range out {
		if p := out[i].Page; p != nil {
			cp := ClonePage(*p)
			out[i].Page = &cp
			if err := AsError(ValidatePage(&cp)); err != nil {
				panic("sdui: the preset " + out[i].ID + " is invalid: " + err.Error())
			}
		}
	}
	return out
}

var presetsOnce = sync.OnceValue(buildPresets)

// Presets returns the gallery of page and section presets. Shared: don't
// change it.
func Presets() []Preset { return presetsOnce() }

// PresetByID returns one preset.
func PresetByID(id string) (Preset, bool) {
	for _, p := range presetsOnce() {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}
