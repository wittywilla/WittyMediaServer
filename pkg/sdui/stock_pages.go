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
	return []Page{brand("studio", "studio", "studio", "globe"), brand("network", "network", "network", "tv"), brand("category", "category", "genre", "filter"),
		versions, saveTo, devices, year, activity, notices}
}
