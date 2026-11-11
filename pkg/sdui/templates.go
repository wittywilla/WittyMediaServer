// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import "strconv"

// Item is one media entry shown on a Card or ListItem.
type Item struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle,omitempty"`
	ImageURL string   `json:"imageUrl,omitempty"`
	Kind     string   `json:"kind,omitempty"` // movie, episode, album, game, channel, …
	Module   string   `json:"module,omitempty"`
	Progress float64  `json:"progress,omitempty"` // 0..1
	Badges   []string `json:"badges,omitempty"`
}

// ShelfData is one extra, data-driven shelf on the home screen.
type ShelfData struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Module string `json:"module,omitempty"` // gates the shelf on ModuleEnabled
	Items  []Item `json:"items"`
}

// HomeData feeds HomeTemplate.
type HomeData struct {
	UserName         string
	ContinueWatching []Item
	RecentlyAdded    []Item
	Recommended      []Item
	Shelves          []ShelfData
	Modules          []string
}

// LibraryData feeds LibraryTemplate.
type LibraryData struct {
	LibraryID string
	Title     string
	Kind      string // movies, shows, music, games, books, photos
	Items     []Item
	Sort      string
	Filters   []string
}

// DetailData feeds DetailTemplate.
type DetailData struct {
	Item     Item
	Overview string
	Year     int
	Duration int // seconds
	Rating   string
	Genres   []string
	Backdrop string
	Related  []Item
	Children []Item // seasons/episodes/tracks
	CanPlay  bool
	Progress float64
}

// PlayerData feeds PlayerTemplate.
type PlayerData struct {
	Item      Item
	StreamURL string
	Poster    string
	StartAt   int // seconds
	Next      *Item
	Subtitles []string
}

// SettingsSection is one group of settings rows.
type SettingsSection struct {
	ID    string
	Title string
	Rows  []Item
	Admin bool // shown only to role admin
}

// SettingsData feeds SettingsTemplate.
type SettingsData struct {
	Sections []SettingsSection
	Version  string
}

const (
	actGoLibrary  = "goLibrary"
	actGoSearch   = "goSearch"
	actGoSettings = "goSettings"
	actOpenItem   = "openItem"
	actPlayItem   = "playItem"
	actBack       = "back"
	actGoHome     = "goHome"
)

func commonActions(b *DocumentBuilder) *DocumentBuilder {
	return b.
		Action(actGoHome, NavigateTo("/")).
		Action(actGoLibrary, NavigateTo("/library")).
		Action(actGoSearch, NavigateTo("/search")).
		Action(actGoSettings, NavigateTo("/settings")).
		Action(actBack, BackAction())
}

// cards builds Card children for items under prefix.
func cards(prefix string, items []Item) []*NodeBuilder {
	out := make([]*NodeBuilder, 0, len(items))
	for i, it := range items {
		id := prefix + "-" + strconv.Itoa(i)
		c := CardOf(id, it)
		if it.ID != "" {
			c.OnPress(actOpenItem).Prop("itemId", it.ID)
		}
		if it.Progress > 0 {
			c.Child(Progress(id+"-progress", it.Progress))
		}
		out = append(out, c)
	}
	return out
}

func posterRow(id, title, bindPath string, items []Item) *NodeBuilder {
	row := PosterRowOf(id, title).
		Bind("items", bindPath).
		Child(cards(id+"-card", items)...)
	return row
}

// HomeTemplate builds the home screen. The greeting binds to $.user.name,
// each stock row binds its items to $.home.<row>, and module shelves are
// gated with ModuleEnabled so that a client without WittyPlay or WittyTV
// never sees the games or live-TV rows.
func HomeTemplate(d HomeData) Document {
	b := commonActions(NewDocument("home")).
		Theme("default").
		Action(actOpenItem, NavigateTo("/item")).
		Action(actPlayItem, Action{Type: Play, Target: "$itemId"}).
		Meta(Meta{Title: "Home", Source: "template:home", TTLSeconds: 60})

	root := ScreenNode("home").Prop("title", "Home").Prop("scroll", true).Style(func(s *Style) {
		s.Direction = "column"
		s.Gap = 32
		s.Background = "token:bg"
	})

	toolbar := ToolbarOf("home-toolbar").
		Child(
			TextOf("home-greeting", "Welcome back").
				BindWith("text", Binding{Path: "$.user.name", Default: "Welcome back", Format: "title"}).
				Prop("variant", "pageTitle"),
			SpacerOf("home-toolbar-gap", 0).Style(func(s *Style) { s.Width = "auto" }),
			SearchBarOf("home-search", "Search your libraries").OnPress(actGoSearch).If(MinWidth(720)),
			ButtonOf("home-settings-btn", "Settings", actGoSettings).Prop("icon", "settings").Prop("variant", "ghost"),
		)
	_ = d.UserName // the greeting is bound; UserName seeds Data in callers.

	hero := Hero("home-hero").
		Prop("title", "Featured").
		Bind("items", "$.home.featured").
		Style(func(s *Style) { s.AspectRatio = "16/9"; s.Height = "42vh" }).
		If(Not(OnPlatform("tv-compact")))

	root.Child(toolbar, hero)

	root.Child(posterRow("home-continue", "Continue watching", "$.home.continueWatching", d.ContinueWatching).
		Prop("cardShape", "wide").If(HasFeature("history")))
	root.Child(posterRow("home-recent", "Recently added", "$.home.recentlyAdded", d.RecentlyAdded))
	root.Child(posterRow("home-recommended", "Recommended for you", "$.home.recommended", d.Recommended).
		If(HasFeature("recommendations")))

	root.Child(ShelfOf("home-games", "Jump back into a game").
		Bind("items", "$.home.games").
		Module("wittyplay").
		If(ModuleEnabled("wittyplay")))
	root.Child(ShelfOf("home-livetv", "On now").
		Bind("items", "$.home.onNow").
		Module("wittytv").
		If(ModuleEnabled("wittytv")))
	root.Child(ShelfOf("home-music", "Recently played music").
		Bind("items", "$.home.music").
		Module("wittymusic").
		If(ModuleEnabled("wittymusic")))

	for i, sh := range d.Shelves {
		id := sh.ID
		if id == "" {
			id = "home-shelf-" + strconv.Itoa(i)
		}
		nb := ShelfOf(id, sh.Title).Child(cards(id+"-card", sh.Items)...)
		if sh.Module != "" {
			nb.Module(sh.Module).If(ModuleEnabled(sh.Module))
		}
		root.Child(nb)
	}

	root.Child(ButtonOf("home-admin-setup", "Finish server setup", actGoSettings).
		Prop("variant", "primary").
		If(All(IsRole("admin"), HasFlag("setupIncomplete"))))

	doc, err := b.Root(root).Build()
	if err != nil {
		panic("sdui: HomeTemplate is invalid: " + err.Error())
	}
	return doc
}

// LibraryTemplate builds a library browse screen: a toolbar with search and
// sort, then a Grid of cards bound to $.library.items. Column count follows
// the viewport via two alternative grids gated on MinWidth/MaxWidth.
func LibraryTemplate(d LibraryData) Document {
	title := d.Title
	if title == "" {
		title = "Library"
	}
	b := commonActions(NewDocument("library")).
		Theme("default").
		Action(actOpenItem, NavigateTo("/item")).
		Action("sort", Action{Type: Fetch, Target: "/ui/library?sort=$sort"}).
		Meta(Meta{Title: title, Source: "template:library", TTLSeconds: 30})

	root := ScreenNode("library").Prop("title", title).Prop("scroll", true).
		Bind("title", "$.library.title").
		Style(func(s *Style) { s.Direction = "column"; s.Gap = 24 })

	toolbar := ToolbarOf("library-toolbar").Child(
		ButtonOf("library-back", "Back", actBack).Prop("icon", "back").Prop("variant", "ghost").If(Not(OnPlatform("web"))),
		TextOf("library-title", title).Bind("text", "$.library.title").Prop("variant", "pageTitle"),
		SpacerOf("library-toolbar-gap", 0).Style(func(s *Style) { s.Width = "auto" }),
		SearchBarOf("library-search", "Search this library").Prop("scope", d.LibraryID).OnPress(actGoSearch),
		ButtonOf("library-sort", "Sort", "sort").Prop("icon", "sort").Prop("variant", "ghost"),
	)
	root.Child(toolbar)

	wide := GridOf("library-grid-wide", 8).Bind("items", "$.library.items").
		Child(cards("library-item", d.Items)...).If(MinWidth(1400))
	medium := GridOf("library-grid", 5).Bind("items", "$.library.items").
		Child(cards("library-item-m", d.Items)...).If(All(MinWidth(721), MaxWidth(1399)))
	narrow := GridOf("library-grid-narrow", 3).Bind("items", "$.library.items").
		Child(cards("library-item-n", d.Items)...).If(MaxWidth(720))
	root.Child(wide, medium, narrow)

	root.Child(TextOf("library-empty", "Nothing here yet. Add a folder to this library in Settings.").
		Prop("variant", "hint").If(HasFlag("libraryEmpty")))

	doc, err := b.Root(root).Build()
	if err != nil {
		panic("sdui: LibraryTemplate is invalid: " + err.Error())
	}
	return doc
}
