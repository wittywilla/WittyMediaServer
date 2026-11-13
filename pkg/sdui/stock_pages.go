// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

// The pages every app draws from the server's own data and no menu lists: opened from a button, a row or
// a route (docs/SDUI.md, "Routes"). They are pages like any other: an administrator may restyle one, and
// every client draws them with the same renderer. Settings stays hand-built; none of these replaces it.

func buildUtilityPages() []Page {
	grid := func(id, title, empty string, src Source) Section {
		return sec(id, "Grid", props{"title": title, "empty": "hide", "emptyText": empty}, src)
	}
	// A studio, a network or a category: everything under it across the libraries the profile can open, split
	// into films and shows, each with how many.
	brand := func(id, label, param, icon string) Page {
		films := Source{"type": "browse", "titles": "video", "limit": 500}
		shows := Source{"type": "browse", "titles": "show", "limit": 500}
		films[param], shows[param] = "$name", "$name"
		return Page{ID: id, Kind: "custom", Title: "{title}", Icon: icon,
			Description: label + ": everything from it, opened at /" + label + "/{name}.",
			Sections: []Section{
				grid("films", "Movies ({count})", "", films),
				grid("shows", "Shows ({count})", "", shows),
			}}
	}
	versions := Page{ID: "versions", Kind: "custom", Title: "{title}", Icon: "video",
		Description: "The files a title can play from (opened with item): choose which one plays. Opened at /versions/{item}.",
		Sections: []Section{
			sec("lead", "Text", props{"text": "Choose which file plays. The same title in more than one version.", "role": "caption"}, nil),
			sec("files", "List", props{"title": "Versions", "empty": "message", "emptyText": "This title has one file."}, Source{"type": "versions", "item": "$item"}),
		}}
	saveTo := Page{ID: "save-to", Kind: "custom", Title: "Save to a list", Icon: "collection",
		Description: "The profile's lists, each a switch for whether the title is in it (opened with item). Opened at /save-to/{item}.",
		Sections: []Section{
			sec("lead", "Text", props{"text": "Tick the lists {title} should be in.", "role": "caption"}, nil),
			sec("lists", "List", props{"empty": "message", "emptyText": "You have no lists yet. Make one below."}, Source{"type": "listsFor", "item": "$item"}),
			with(sec("new", "Form", props{"title": "A new list", "op": "collection.create", "submitLabel": "Make the list"}, nil),
				sec("name", "List", props{}, Source{"type": "newList"})),
		}}
	devices := Page{ID: "devices", Kind: "custom", Title: "Your devices", Icon: "devices",
		Description: "Where this profile is signed in, with when each was last used and a Sign out. Opened at /devices.",
		Sections: []Section{
			sec("lead", "Text", props{"text": "Last used, and sign out. A device you sign out has to sign in again.", "role": "caption"}, nil),
			sec("list", "List", props{"empty": "message", "emptyText": "No devices."}, Source{"type": "devices"}),
			sec("others", "Button", props{"label": "Sign out the others", "icon": "devices", "op": "sessions.endOthers", "confirm": "Sign out every other device?", "variant": "secondary"}, nil),
		}}
	year := Page{ID: "year", Kind: "custom", Title: "Your year", Icon: "chart",
		Description: "Totals, streak, busiest day, months and top titles from the profile's own play log. Opened at /year.",
		Sections: []Section{
			sec("totals", "List", props{"title": "Totals", "empty": "message", "emptyText": "Nothing watched yet this year."}, Source{"type": "year"}),
			sec("months", "List", props{"title": "Months", "dense": true}, Source{"type": "year", "part": "months"}),
			sec("titles", "List", props{"title": "Top titles"}, Source{"type": "year", "part": "titles", "limit": 10}),
			sec("shows", "List", props{"title": "Top shows"}, Source{"type": "year", "part": "shows", "limit": 10}),
			sec("artists", "List", props{"title": "Top artists"}, Source{"type": "year", "part": "artists", "limit": 10}),
			sec("genres", "List", props{"title": "Top genres", "dense": true}, Source{"type": "year", "part": "genres", "limit": 10}),
			sec("clear", "Button", props{"label": "Clear my history", "icon": "history", "op": "history.clear", "confirm": "Forget everything you have watched and listened to? This cannot be undone.", "variant": "ghost"}, nil),
		}}
	activity := Page{ID: "activity", Kind: "custom", Title: "Activity", Icon: "plug",
		Description: "What the server is doing (scanning, making karaoke, backing up) and what it has just done. Opened at /activity.",
		Sections: []Section{
			with(sec("tabs", "Tabs", props{"style": "segments", "default": "now"}, nil),
				with(sec("now", "Tab", props{"label": "Now"}, nil), sec("running", "List", props{"empty": "message", "emptyText": "Nothing is running."}, Source{"type": "activity", "part": "now"})),
				with(sec("history", "Tab", props{"label": "History"}, nil), sec("done", "List", props{"empty": "message", "emptyText": "Nothing has finished lately."}, Source{"type": "activity", "part": "history"}))),
		}}
	notices := Page{ID: "notifications", Kind: "custom", Title: "Notifications", Icon: "info",
		Description: "Reminders, and for an administrator a new version. Opened at /notifications.",
		Sections: []Section{
			sec("recent", "List", props{"title": "Recent", "empty": "message", "emptyText": "You are all caught up."}, Source{"type": "notices"}),
		}}
	creators := Page{ID: "creators", Kind: "custom", Title: "Creators", Icon: "users",
		Description: "The creators of the libraries the profile can open, with a Subscribe switch. Opened at /creators.",
		Sections: []Section{
			with(sec("tabs", "Tabs", props{"style": "segments", "default": "all"}, nil),
				with(sec("all", "Tab", props{"label": "Creators"}, nil), sec("everyone", "List", props{"empty": "message", "emptyText": "No creators yet."}, Source{"type": "creators"})),
				with(sec("subscribed", "Tab", props{"label": "Subscriptions"}, nil), sec("mine", "List", props{"empty": "message", "emptyText": "You are not subscribed to anyone yet."}, Source{"type": "creators", "subscribed": true}))),
		}}
	creator := Page{ID: "creator", Kind: "custom", Title: "{title}", Icon: "user",
		Description: "One creator: their row with Subscribe and what they say about themselves. Opened at /creator/{item}.",
		Sections:    []Section{sec("who", "List", props{"empty": "message", "emptyText": "That creator isn't here."}, Source{"type": "creators", "item": "$item"})}}
	comments := Page{ID: "comments", Kind: "custom", Title: "Comments", Icon: "info",
		Description: "What people wrote about a title (opened with item); adults may write, everyone may read. Opened at /comments/{item}.",
		Sections: []Section{
			sec("all", "List", props{"empty": "message", "emptyText": "No comments yet."}, Source{"type": "comments", "item": "$item"}),
			when(with(sec("write", "Form", props{"title": "Write a comment", "op": "comment.add", "item": "$item", "submitLabel": "Post"}, nil),
				sec("text", "List", props{}, Source{"type": "formField", "name": "text", "label": "Comment", "placeholder": "Say something kind", "multiline": true, "maxLength": 2000})),
				Condition{Can: "comments", Not: &Condition{ProfileKind: []string{"kids", "guest"}}}),
			when(sec("readonly", "Text", props{"text": "Comments are read-only for this profile.", "role": "caption"}, nil), Condition{ProfileKind: []string{"kids", "guest"}}),
		}}
	markers := Page{ID: "markers", Kind: "custom", Title: "{title}", Icon: "play",
		Description: "Where the intro, recap, credits and breaks of a title are (opened with item). Opened at /markers/{item}.",
		Sections: []Section{
			sec("lead", "Text", props{"text": "Skipping markers: what Skip uses while this plays.", "role": "caption"}, nil),
			sec("marks", "List", props{"empty": "message", "emptyText": "Nothing is marked."}, Source{"type": "markers", "item": "$item"}),
			sec("pick", "Button", props{"label": "Pick in player", "icon": "play", "native": "player", "item": "$item", "variant": "primary"}, nil),
		}}
	canvas := Page{ID: "canvas", Kind: "custom", Title: "{title}", Icon: "video",
		Description: "Whether a title has a looping silent video behind its page (opened with item). Opened at /canvas/{item}.",
		Sections: []Section{
			sec("lead", "Text", props{"text": "It plays behind this title's page, and behind an album's page and the full player.", "role": "caption"}, nil),
			sec("state", "List", props{"empty": "message", "emptyText": "Not available."}, Source{"type": "canvas", "item": "$item"}),
		}}
	smart := Page{ID: "smart-list", Kind: "custom", Title: "A smart playlist", Icon: "filter",
		Description: "A playlist that fills itself from a rule: choose the rule and save it. Opened at /smart-list.",
		Sections: []Section{
			sec("lead", "Text", props{"text": "It finds what matches, whenever you open it.", "role": "caption"}, nil),
			with(sec("rule", "Form", props{"title": "Rules", "op": "collection.createSmart", "submitLabel": "Save"}, nil),
				sec("fields", "List", props{}, Source{"type": "smartRule"})),
		}}
	// Administrators' pages, from the admin routes: the libraries, the titles whose match needs a look, and
	// one title's possible matches. Their sources are empty for anyone else, and their commands refused.
	admin := true
	onlyAdmin := func(x Section) Section { return when(x, Condition{Admin: &admin}) }
	notAdmin := when(sec("not", "Notice", props{"tone": "info", "text": "Only an administrator can manage this."}, nil), Condition{Admin: new(bool)})
	manageLibraries := Page{ID: "manage-libraries", Kind: "custom", Title: "Libraries", Icon: "library",
		Description: "Administrators: the libraries with their scan state, a row scanning each again, and a form adding one. Opened at /manage/libraries.",
		Sections: []Section{notAdmin,
			onlyAdmin(sec("all", "List", props{"empty": "message", "emptyText": "No libraries yet. Add one below."}, Source{"type": "libraryAdmin"})),
			onlyAdmin(sec("forget", "Button", props{"label": "Remove downloaded details", "icon": "history", "op": "libraries.forgetMetadata", "variant": "ghost",
				"confirm": "Remove what was downloaded for every library and download it again? History, lists and locked fields stay."}, nil)),
			onlyAdmin(with(sec("add", "Form", props{"title": "Add a library", "op": "library.create", "submitLabel": "Add and scan"}, nil),
				sec("fields", "List", props{}, Source{"type": "newLibrary"}))),
		}}
	fixMatches := Page{ID: "fix-matches", Kind: "custom", Title: "Fix matches", Icon: "filter",
		Description: "Administrators: titles that are unmatched or matched with little certainty, each opening its possible matches. Opened at /manage/matches.",
		Sections: []Section{notAdmin,
			onlyAdmin(with(sec("tabs", "Tabs", props{"style": "segments", "default": "check"}, nil),
				with(sec("check", "Tab", props{"label": "To check"}, nil), sec("queue", "List", props{"empty": "message", "emptyText": "Every title is matched."}, Source{"type": "matchQueue", "state": "attention"})),
				with(sec("byhand", "Tab", props{"label": "Matched by hand"}, nil), sec("manual", "List", props{"empty": "message", "emptyText": "Nothing was matched by hand."}, Source{"type": "matchQueue", "state": "manual"})),
				with(sec("left", "Tab", props{"label": "Left unmatched"}, nil), sec("none", "List", props{"empty": "message", "emptyText": "Nothing is left unmatched."}, Source{"type": "matchQueue", "state": "none"})))),
		}}
	match := Page{ID: "match", Kind: "custom", Title: "Fix the match", Icon: "search",
		Description: "Administrators: one title's possible matches (opened with item), each matching it after asking. Opened at /manage/match/{item}.",
		Sections: []Section{notAdmin,
			onlyAdmin(sec("lead", "Text", props{"text": "Choose what this is. The details, artwork and people come from the one you choose.", "role": "caption"}, nil)),
			onlyAdmin(sec("candidates", "List", props{"empty": "message", "emptyText": "No source suggests anything for this title."}, Source{"type": "matchCandidates", "item": "$item"})),
		}}
	return []Page{manageLibraries, fixMatches, match, creators, creator, comments, markers, canvas, smart, brand("studio", "studio", "studio", "globe"), brand("network", "network", "network", "tv"), brand("category", "category", "genre", "filter"),
		versions, saveTo, devices, year, activity, notices}
}
