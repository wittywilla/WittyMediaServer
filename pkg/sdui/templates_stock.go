// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

// The stock screens that are all data: the template is the frame, and the
// server fills it for whoever asks (see docs/SDUI.md, "What the server puts
// in Data"). None of them takes arguments, because nothing in them is known
// before the request.

func stockToolbar(id, title string) *NodeBuilder {
	return ToolbarOf(id+"-toolbar").Child(
		ButtonOf(id+"-back", "Back", actBack).Prop("icon", "back").Prop("variant", "ghost").
			If(All(Not(OnPlatform("web")), Not(Condition{FormFactor: FormTV}))), // a TV remote has its own Back
		TextOf(id+"-title", title).Prop("variant", "pageTitle"),
	)
}

// LibrariesTemplate builds the list of libraries the account may open, bound
// to $.libraries.items.
func LibrariesTemplate() Document {
	b := commonActions(NewDocument("libraries")).Theme("default").
		Action(actOpenItem, NavigateTo("$route")).
		Meta(Meta{Title: "Libraries", Source: "template:libraries", TTLSeconds: 300})
	root := ScreenNode("libraries").Prop("title", "Libraries").Prop("scroll", true).
		Style(func(s *Style) { s.Direction = "column"; s.Gap = 24 })
	root.Child(stockToolbar("libraries", "Libraries"))
	root.Child(GridOf("libraries-grid", 0).Prop("cardShape", "wide").Bind("items", "$.libraries.items"))
	root.Child(TextOf("libraries-empty", "There are no libraries you can open yet.").Prop("variant", "hint").If(HasFlag("librariesEmpty")))
	return mustBuild(b, root)
}

// SearchTemplate builds search: the field, then one row per kind of result,
// bound to $.search.sections. The field submits to this same screen with ?q=.
func SearchTemplate() Document {
	b := commonActions(NewDocument("search")).Theme("default").
		Action(actOpenItem, NavigateTo("$route")).
		Action("search", Action{Type: Fetch, Target: "/api/v1/sdui/screens/search?q=$query"}).
		Meta(Meta{Title: "Search", Source: "template:search", TTLSeconds: 0})
	root := ScreenNode("search").Prop("title", "Search").Prop("scroll", true).
		Style(func(s *Style) { s.Direction = "column"; s.Gap = 24 })
	root.Child(ToolbarOf("search-toolbar").Child(
		SearchBarOf("search-field", "Films, shows, music, channels").Bind("query", "$.search.query").OnPress("search").
			Focus(Focus{Default: true}),
	))
	root.Child(StackOf("search-results").Bind("sections", "$.search.sections"))
	root.Child(TextOf("search-none", "Nothing matches that.").Prop("variant", "hint").If(HasFlag("searchEmpty")))
	root.Child(TextOf("search-hint", "Type at least two letters.").Prop("variant", "hint").If(HasFlag("searchIdle")))
	return mustBuild(b, root)
}

// LiveTVTemplate builds live TV: favourite channels, then every channel the
// account may watch, bound to $.livetv.favourites and $.livetv.channels.
func LiveTVTemplate() Document {
	b := commonActions(NewDocument("livetv")).Theme("default").
		Action(actOpenItem, NavigateTo("$route")).
		Meta(Meta{Title: "Live TV", Source: "template:livetv", TTLSeconds: 60})
	root := ScreenNode("livetv").Prop("title", "Live TV").Prop("scroll", true).
		Style(func(s *Style) { s.Direction = "column"; s.Gap = 24; s.Module = "wittytv" })
	root.Child(stockToolbar("livetv", "Live TV"))
	root.Child(ShelfOf("livetv-favourites", "Favourites").Prop("cardShape", "wide").Bind("items", "$.livetv.favourites").Module("wittytv"))
	root.Child(GridOf("livetv-channels", 0).Prop("cardShape", "wide").Bind("items", "$.livetv.channels").Module("wittytv"))
	root.Child(TextOf("livetv-empty", "There are no channels yet.").Prop("variant", "hint").If(HasFlag("livetvEmpty")))
	return mustBuild(b, root)
}

// MusicTemplate builds the music home: rows of albums, bound to
// $.music.sections (recently added, by library).
func MusicTemplate() Document {
	b := commonActions(NewDocument("music")).Theme("default").
		Action(actOpenItem, NavigateTo("$route")).
		Meta(Meta{Title: "Music", Source: "template:music", TTLSeconds: 120})
	root := ScreenNode("music").Prop("title", "Music").Prop("scroll", true).
		Style(func(s *Style) { s.Direction = "column"; s.Gap = 24; s.Module = "wittymusic" })
	root.Child(stockToolbar("music", "Music"))
	root.Child(ShelfOf("music-continue", "Continue listening").Prop("cardShape", "square").Bind("items", "$.music.continueListening").Module("wittymusic"))
	root.Child(StackOf("music-rows").Bind("sections", "$.music.sections"))
	root.Child(TextOf("music-empty", "There is no music you can open yet.").Prop("variant", "hint").If(HasFlag("musicEmpty")))
	return mustBuild(b, root)
}

func mustBuild(b *DocumentBuilder, root *NodeBuilder) Document {
	doc, err := b.Root(root).Build()
	if err != nil {
		panic("sdui: stock template is invalid: " + err.Error())
	}
	return doc
}
