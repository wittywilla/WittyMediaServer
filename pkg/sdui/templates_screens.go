// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import "strconv"

// DetailTemplate builds an item detail screen: a detail hero with backdrop,
// title, metadata and a Play button, the overview, child items (seasons,
// episodes, tracks) and a related row. Text and image props are bound under
// $.item so one template serves every item.
func DetailTemplate(d DetailData) Document {
	title := d.Item.Title
	if title == "" {
		title = "Details"
	}
	b := commonActions(NewDocument("detail")).
		Theme("default").
		Action(actOpenItem, NavigateTo("/item")).
		Action(actPlayItem, PlayItem(firstNonEmpty(d.Item.ID, "$itemId"))).
		Action("resume", Action{Type: Play, Target: firstNonEmpty(d.Item.ID, "$itemId"), Params: map[string]any{"resume": true}}).
		Action("openArtwork", OpenModalAction("detail-artwork-modal")).
		Action("closeModal", CloseModalAction()).
		Meta(Meta{Title: title, Source: "template:detail", TTLSeconds: 120})

	root := ScreenNode("detail").Prop("title", title).Prop("scroll", true).
		Bind("title", "$.item.title").
		Style(func(s *Style) { s.Direction = "column"; s.Gap = 24 })

	hero := Hero("detail-hero").
		Prop("title", title).
		Bind("title", "$.item.title").
		BindWith("subtitle", Binding{Path: "$.item.subtitle", Default: d.Item.Subtitle}).
		BindWith("backdropUrl", Binding{Path: "$.item.backdrop", Default: d.Backdrop}).
		Style(func(s *Style) { s.Class = "detail-hero"; s.AspectRatio = "16/9" }).
		Child(
			Row("detail-meta").Style(func(s *Style) { s.Gap = 12; s.Align = "center" }).Child(
				BadgeOf("detail-year", strconv.Itoa(d.Year)).
					BindWith("text", Binding{Path: "$.item.year", Default: yearOrEmpty(d.Year)}).
					If(Not(HasFlag("hideYear"))),
				BadgeOf("detail-duration", "").
					BindWith("text", Binding{Path: "$.item.duration", Default: d.Duration, Format: "duration"}),
				BadgeOf("detail-rating", firstNonEmpty(d.Rating, "NR")).
					BindWith("text", Binding{Path: "$.item.rating", Default: firstNonEmpty(d.Rating, "NR")}),
			),
			Row("detail-actions").Style(func(s *Style) { s.Gap = 12 }).Child(
				ButtonOf("detail-play", "Play", actPlayItem).Prop("icon", "play").Prop("variant", "primary").
					If(HasFeature("playback")),
				ButtonOf("detail-resume", "Resume", "resume").Prop("icon", "resume").
					If(All(HasFeature("playback"), HasFlag("hasProgress"))),
				ButtonOf("detail-artwork", "Artwork", "openArtwork").Prop("variant", "ghost").
					If(IsRole("admin")),
			),
			Progress("detail-progress", d.Progress).Bind("value", "$.item.progress").If(HasFlag("hasProgress")),
		)
	root.Child(hero)

	root.Child(TextOf("detail-overview", d.Overview).
		BindWith("text", Binding{Path: "$.item.overview", Default: firstNonEmpty(d.Overview, "No overview yet.")}).
		Prop("variant", "body").Prop("maxLines", 6))

	if len(d.Genres) > 0 {
		genres := Row("detail-genres").Style(func(s *Style) { s.Gap = 8 })
		for i, g := range d.Genres {
			genres.Child(BadgeOf("detail-genre-"+strconv.Itoa(i), g).Prop("variant", "outline"))
		}
		root.Child(genres)
	}

	children := ListOf("detail-children").Bind("items", "$.item.children")
	for i, c := range d.Children {
		li := ListItemOf("detail-child-"+strconv.Itoa(i), c.Title).Prop("itemId", c.ID).OnPress(actOpenItem)
		if c.Subtitle != "" {
			li.Prop("subtitle", c.Subtitle)
		}
		if c.ImageURL != "" {
			li.Prop("imageUrl", c.ImageURL)
		}
		children.Child(li)
	}
	root.Child(children.If(Any(HasFlag("hasChildren"), MinWidth(0))))

	root.Child(posterRow("detail-related", "More like this", "$.item.related", d.Related).
		If(HasFeature("recommendations")))

	root.Child(ModalOf("detail-artwork-modal", "Artwork").Prop("dismissible", true).
		If(IsRole("admin")).
		Child(
			TextOf("detail-artwork-hint", "Choose a poster or backdrop for this title."),
			ButtonOf("detail-artwork-close", "Close", "closeModal"),
		))

	doc, err := b.Root(root).Build()
	if err != nil {
		panic("sdui: DetailTemplate is invalid: " + err.Error())
	}
	return doc
}

// PlayerTemplate builds the player screen: a Video bound to $.player.src
// with an overlay toolbar and an optional "up next" card. On TV the overlay
// hides the Back button because the remote has one.
func PlayerTemplate(d PlayerData) Document {
	title := d.Item.Title
	if title == "" {
		title = "Now playing"
	}
	b := commonActions(NewDocument("player")).
		Theme("default").
		Action(actOpenItem, NavigateTo("/item")).
		Action("playNext", Action{Type: Play, Target: nextID(d.Next)}).
		Action("toggleSubtitles", Action{Type: Toggle, Target: "player-video", Params: map[string]any{"prop": "subtitles"}}).
		Meta(Meta{Title: title, Source: "template:player", TTLSeconds: 0})

	root := ScreenNode("player").Prop("title", title).Prop("scroll", false).
		Style(func(s *Style) { s.Background = "#000000"; s.Foreground = "token:onImage"; s.Class = "player" })

	video := VideoOf("player-video", firstNonEmpty(d.StreamURL, "about:blank")).
		Prop("controls", true).Prop("autoplay", true).
		BindWith("src", Binding{Path: "$.player.src", Default: firstNonEmpty(d.StreamURL, "about:blank")}).
		BindWith("poster", Binding{Path: "$.player.poster", Default: d.Poster}).
		BindWith("startAt", Binding{Path: "$.player.startAt", Default: d.StartAt}).
		BindWith("title", Binding{Path: "$.item.title", Default: title}).
		Prop("itemId", d.Item.ID)

	overlay := ToolbarOf("player-overlay").Style(func(s *Style) { s.Class = "player-overlay" }).Child(
		ButtonOf("player-back", "Back", actBack).Prop("icon", "back").Prop("variant", "ghost").
			If(Not(OnPlatform("tv"))),
		TextOf("player-title", title).Bind("text", "$.item.title").Prop("variant", "heading"),
		SpacerOf("player-overlay-gap", 0).Style(func(s *Style) { s.Width = "auto" }),
		ButtonOf("player-subs", "Subtitles", "toggleSubtitles").Prop("icon", "subtitles").Prop("variant", "ghost").
			If(HasFlag("hasSubtitles")),
	)

	root.Child(video, overlay)

	if d.Next != nil {
		root.Child(CardOf("player-next", *d.Next).Prop("kind", "upNext").OnPress("playNext").
			If(HasFeature("autoplayNext")))
	}

	doc, err := b.Root(root).Build()
	if err != nil {
		panic("sdui: PlayerTemplate is invalid: " + err.Error())
	}
	return doc
}

// SettingsTemplate builds the settings screen: one Tab per section, with
// admin-only sections gated on IsRole("admin") and module sections gated on
// ModuleEnabled.
func SettingsTemplate(d SettingsData) Document {
	b := commonActions(NewDocument("settings")).
		Theme("default").
		Action("openSetting", NavigateTo("/settings/$itemId")).
		Action("signOut", Action{Type: Submit, Target: "/auth/signout"}).
		Action("about", OpenModalAction("settings-about")).
		Action("closeModal", CloseModalAction()).
		Meta(Meta{Title: "Settings", Source: "template:settings", TTLSeconds: 300})

	root := ScreenNode("settings").Prop("title", "Settings").Prop("scroll", true).
		Style(func(s *Style) { s.Direction = "column"; s.Gap = 24 })

	root.Child(ToolbarOf("settings-toolbar").Child(
		ButtonOf("settings-back", "Back", actBack).Prop("icon", "back").Prop("variant", "ghost").If(Not(OnPlatform("web"))),
		TextOf("settings-title", "Settings").Prop("variant", "pageTitle"),
		SpacerOf("settings-toolbar-gap", 0).Style(func(s *Style) { s.Width = "auto" }),
		ButtonOf("settings-about-btn", "About", "about").Prop("variant", "ghost"),
	))

	tabs := TabsOf("settings-tabs").Prop("selected", 0)
	sections := d.Sections
	if len(sections) == 0 {
		sections = defaultSettingsSections()
	}
	for i, sec := range sections {
		id := sec.ID
		if id == "" {
			id = "settings-sec-" + strconv.Itoa(i)
		}
		tab := TabOf(id, sec.Title)
		if sec.Admin {
			tab.If(IsRole("admin"))
		}
		list := ListOf(id + "-list")
		for j, row := range sec.Rows {
			li := ListItemOf(id+"-row-"+strconv.Itoa(j), row.Title).
				Prop("itemId", row.ID).Prop("trailing", "chevron").OnPress("openSetting")
			if row.Subtitle != "" {
				li.Prop("subtitle", row.Subtitle)
			}
			if row.Module != "" {
				li.Module(row.Module).If(ModuleEnabled(row.Module))
			}
			list.Child(li)
		}
		tab.Child(list)
		tabs.Child(tab)
	}
	root.Child(tabs)

	root.Child(ButtonOf("settings-signout", "Sign out", "signOut").Prop("variant", "danger").
		If(Not(IsRole("guest"))))

	root.Child(ModalOf("settings-about", "About WittyMediaServer").Prop("dismissible", true).Child(
		TextOf("settings-version", "Version "+firstNonEmpty(d.Version, "unknown")).
			BindWith("text", Binding{Path: "$.server.version", Default: "Version " + firstNonEmpty(d.Version, "unknown")}),
		ButtonOf("settings-about-close", "Close", "closeModal"),
	))

	doc, err := b.Root(root).Build()
	if err != nil {
		panic("sdui: SettingsTemplate is invalid: " + err.Error())
	}
	return doc
}

func defaultSettingsSections() []SettingsSection {
	return []SettingsSection{
		{ID: "settings-general", Title: "General", Rows: []Item{
			{ID: "appearance", Title: "Appearance", Subtitle: "Theme, text size, TV look"},
			{ID: "playback", Title: "Playback", Subtitle: "Quality, subtitles, autoplay"},
			{ID: "profiles", Title: "Profiles", Subtitle: "Who is watching"},
		}},
		{ID: "settings-libraries", Title: "Libraries", Admin: true, Rows: []Item{
			{ID: "libraries", Title: "Libraries", Subtitle: "Folders, scanning, metadata"},
			{ID: "transcoding", Title: "Transcoding", Subtitle: "Hardware acceleration"},
			{ID: "fixmatches", Title: "Fix matches", Subtitle: "Titles that need a hand"},
		}},
		{ID: "settings-modules", Title: "Modules", Admin: true, Rows: []Item{
			{ID: "modules", Title: "Module store", Subtitle: "Install and update modules"},
			{ID: "wittyplay", Title: "WittyPlay", Subtitle: "Consoles and cores", Module: "wittyplay"},
			{ID: "wittytv", Title: "WittyTV", Subtitle: "Tuners, guide, DVR", Module: "wittytv"},
			{ID: "wittydvd", Title: "WittyDVD", Subtitle: "Disc drives and ripping", Module: "wittydvd"},
			{ID: "wittybluray", Title: "WittyBluray", Subtitle: "Blu-ray playback", Module: "wittybluray"},
		}},
		{ID: "settings-server", Title: "Server", Admin: true, Rows: []Item{
			{ID: "remote", Title: "Remote access", Subtitle: "Reach your server away from home"},
			{ID: "backup", Title: "Backup", Subtitle: "Snapshots and restore"},
			{ID: "logs", Title: "Logs", Subtitle: "Diagnostics"},
		}},
	}
}

// DefaultScreens returns the five stock screens built from empty data, keyed
// by screen name. These are the documents a server seeds its UI
// configuration with before an administrator customises anything.
func DefaultScreens() map[string]Document {
	return map[string]Document{
		"home":     HomeTemplate(HomeData{}),
		"library":  LibraryTemplate(LibraryData{}),
		"detail":   DetailTemplate(DetailData{}),
		"player":   PlayerTemplate(PlayerData{}),
		"settings": SettingsTemplate(SettingsData{}),
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func yearOrEmpty(y int) string {
	if y <= 0 {
		return ""
	}
	return strconv.Itoa(y)
}

func nextID(it *Item) string {
	if it == nil || it.ID == "" {
		return "$next"
	}
	return it.ID
}
