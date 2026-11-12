// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"encoding/json"
	"strings"
	"sync"
)

// The catalogue itself. Everything an editor shows and the validator checks
// comes from these tables, and docs/SDUI.md's reference section is written
// from them by a test, so the three can't disagree.

// Form factors, platforms and the other fixed vocabularies.
const (
	FormTV      = "tv"
	FormPhone   = "phone"
	FormTablet  = "tablet"
	FormDesktop = "desktop"
	FormWeb     = "web"
)

var (
	// FormFactors are the kinds of device a page is fitted to.
	FormFactors = []string{FormTV, FormPhone, FormTablet, FormDesktop, FormWeb}
	// Platforms are the apps a condition can name.
	Platforms = []string{"web", "tvos", "ios", "android", "androidtv", "desktop"}
	// ProfileKinds are the kinds of profile.
	ProfileKinds = []string{"adult", "kids", "guest"}
	// LibraryKinds are the kinds of library a page or repeat can name.
	LibraryKinds = []string{"movies", "discs", "shows", "anime", "videos", "shorts", "channels", "musicvideos", "commercials", "music", "audiobooks", "podcasts", "radio", "books", "photos", "games"}
	// MediaKinds are the kinds of title a detail page can match.
	MediaKinds = []string{"movie", "show", "episode", "video", "disc", "album", "artist", "track", "book", "audiobook", "podcast", "game", "photo", "collection"}
	// SpacingSteps are the abstract distances a layout component may use; a
	// device profile says how many points each is.
	SpacingSteps = []string{"none", "xs", "s", "m", "l", "xl"}
	// CardSizes are the three sizes of a card.
	CardSizes = []string{"s", "m", "l"}
	// Densities are how tightly a page is packed.
	Densities = []string{"compact", "comfortable", "spacious"}
	// PersonalKinds are what a profile may change about a page.
	PersonalKinds = []string{"reorder", "hide", "pin", "density", "cardSize", "defaultTab"}
	// CanFeatures are what a "can" condition may ask about.
	CanFeatures = []string{"games", "recordings", "downloads", "sharing", "comments", "syncplay", "remoteControl", "remote", "editProfile", "editLayout", "liveTv"}
	// Flags are what a "flag" condition may ask about.
	Flags = []string{"setupIncomplete", "scanning", "librariesEmpty", "hasQuery", "screenTimeUp"}
	// NativeScreens are the screens every app draws itself and SDUI only
	// links to. Settings is one of them on purpose: it is never an SDUI page.
	NativeScreens = []string{"settings", "guide", "profiles", "downloads", "remote", "nowPlaying", "pair", "collections", "player", "livePlayer", "reader", "game", "download", "share", "playOn", "watchParty"}
	// NativeViews are parts of a page an app may draw itself (NativeView).
	NativeViews = []string{"guide", "photoTimeline", "photoMap", "coverFlow"}
	// RepeatKinds are what a section can be repeated over.
	RepeatKinds = []string{"libraries", "pinnedViews", "collections"}
	// Weekdays as a condition names them, Sunday first as time.Weekday counts.
	Weekdays = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}
	// Placements are where a navigation entry can sit.
	Placements = []string{"primary", "more", "end", "hidden"}
)

// Icons is the fixed icon set: a name every renderer has a glyph for.
var Icons = []string{
	"home", "library", "movie", "show", "tv", "live", "guide", "record", "music", "album", "artist", "mic", "radio",
	"podcast", "book", "audiobook", "comic", "photo", "video", "disc", "game", "collection", "list", "folder",
	"search", "settings", "user", "users", "kids", "star", "heart", "clock", "history", "calendar", "sparkles",
	"fire", "shuffle", "play", "download", "share", "globe", "info", "warning", "check", "plus", "grid", "filter",
	"sort", "plug", "chart", "devices", "more",
}

func ip(n int) *int { return &n }

// opts turns "value|Label" strings into options; a bare "value" is its own label, capitalised.
func opts(list ...string) []EnumOption {
	out := make([]EnumOption, 0, len(list))
	for _, s := range list {
		v, l, ok := strings.Cut(s, "|")
		if !ok {
			l = strings.ToUpper(v[:1]) + v[1:]
		}
		out = append(out, EnumOption{Value: v, Label: l})
	}
	return out
}

func values(o []EnumOption) []string {
	out := make([]string, len(o))
	for i := range o {
		out[i] = o[i].Value
	}
	return out
}

func pStr(name, label, help string, max int) PropSpec {
	return PropSpec{Name: name, Type: PropString, Label: label, Help: help, MaxLength: max, Since: 2}
}
func pText(name, label, help string, max int) PropSpec {
	return PropSpec{Name: name, Type: PropText, Label: label, Help: help, MaxLength: max, Since: 2}
}
func pBool(name, label, help string, def bool) PropSpec {
	return PropSpec{Name: name, Type: PropBool, Label: label, Help: help, Default: def, Since: 2}
}
func pInt(name, label, help string, def, min, max int) PropSpec {
	p := PropSpec{Name: name, Type: PropInt, Label: label, Help: help, Min: ip(min), Max: ip(max), Since: 2}
	if def != 0 {
		p.Default = def
	}
	return p
}
func pEnum(name, label, help, def string, o ...string) PropSpec {
	p := PropSpec{Name: name, Type: PropEnum, Label: label, Help: help, Options: opts(o...), Since: 2}
	if def != "" {
		p.Default = def
	}
	return p
}
func pList(name, label, help string, def []string, o ...string) PropSpec {
	return PropSpec{Name: name, Type: PropEnumList, Label: label, Help: help, Default: def, Options: opts(o...), Since: 2}
}
func pOf(typ, name, label, help string) PropSpec {
	return PropSpec{Name: name, Type: typ, Label: label, Help: help, Since: 2}
}
func req(p PropSpec) PropSpec { p.Required = true; return p }
func on(p PropSpec, ff ...string) PropSpec {
	p.FormFactors = ff
	return p
}

func example(v string) json.RawMessage {
	var buf any
	if err := json.Unmarshal([]byte(v), &buf); err != nil {
		panic("sdui: a catalogue example is not JSON: " + v)
	}
	return json.RawMessage(v)
}

// The card styles a section can ask for, plus "auto": the source's own.
var cardStyleOptions = []string{
	"auto|The source's usual style", "poster|Poster", "landscape|Landscape", "progress|Landscape with progress", "episode|Episode",
	"square|Square", "circle|Circle", "album|Album", "person|Person", "book|Book cover", "game|Game box", "photo|Photo",
	"channel|Channel", "tile|Tile", "chip|Chip",
}

var (
	propTitle     = pText("title", "Title", "The heading above the items. Placeholders such as {library} and {each} are filled in.", 120)
	propSubtitle  = pText("subtitle", "Subtitle", "A smaller line under the title.", 160)
	propCardStyle = pEnum("cardStyle", "Card style", "How each item is drawn. Auto uses the style that suits the source.", "auto", cardStyleOptions...)
	propCardSize  = pEnum("cardSize", "Card size", "Small, medium or large cards. A profile may choose its own.", "m", "s|Small", "m|Medium", "l|Large")
	propEmpty     = pEnum("empty", "When there is nothing to show", "Leave the section out, or keep it with a message.", "hide", "hide|Leave it out", "message|Show a message")
	propEmptyText = pText("emptyText", "Message when empty", "Shown in place of the items when there are none.", 200)
	propTo        = pOf(PropRoute, "to", "Opens", "A place in the app, such as /library/3 or /page/family-night.")
	propNative    = pEnum("native", "Opens a screen of the app", "A screen the app draws itself.", "", "settings|Settings", "guide|TV guide", "profiles|Who's watching", "downloads|Downloads", "remote|Remote control", "nowPlaying|Now playing", "pair|Pair a device", "collections|Collections")
	propURL       = on(pOf(PropURL, "url", "Opens a web page", "An http or https address, opened in the browser. Left out on a television."), FormPhone, FormTablet, FormDesktop, FormWeb)
	propIcon      = pOf(PropIcon, "icon", "Icon", "One of the built-in icons.")
	propGap       = pEnum("gap", "Space between", "The distance between the things inside.", "m", "none|None", "xs|Extra small", "s|Small", "m|Medium", "l|Large", "xl|Extra large")
)

var statesData = map[string]string{
	"loading":  "Its heading, and as many grey placeholder cards of the card's size as fit, pulsing unless the device asks for reduced motion.",
	"empty":    "Left out of the page; with empty \"message\", the heading and emptyText in the caption style where the items would be.",
	"error":    "The heading and one line, \"Couldn't load this\", with a Retry button that asks props.more.url again.",
	"deferred": "Drawn as loading until the client has asked props.more.url, which it does when the section is within one screen of being seen.",
}

func buildComponents() []ComponentSpec {
	return []ComponentSpec{
		{Type: "Page", Label: "Page", Icon: "grid", Category: "structure", Since: 2, Required: true, Data: "none", Container: true, Focus: "none",
			Description: "The root of every rendered page. It scrolls down; nothing inside it scrolls down by itself.",
			Props:       []PropSpec{pStr("title", "Title", "The page's heading.", 120), pEnum("density", "Density", "How tightly the page is packed.", "comfortable", "compact", "comfortable", "spacious")},
			Renders: map[string]string{
				"tv":      "A full-screen column inside the safe area (60 points top and bottom, 80 at the sides), sections 40 points apart. The title isn't drawn: the top menu says where you are.",
				"phone":   "A scrolling column under a large title that collapses into the navigation bar; sections 24 points apart, rows bleed to the screen's edges.",
				"tablet":  "As a phone, with 16-point side margins (the m spacing, as on every form factor but the television).",
				"desktop": "A scrolling column beside the sidebar, the title as a heading, content at most 1600 points wide.",
			},
			A11y: "Role: main. Label: the title."},

		{Type: "Hero", Label: "Spotlight", Icon: "sparkles", Category: "media", Since: 2, Authored: true, Data: "required", Focus: "stop",
			Fallback: "Shelf", FallbackNote: "A Shelf of landscape cards of the same items, titled with the hero's title or \"Featured\".",
			Description: "A large rotating spotlight of a few titles: backdrop, title or logo, a line of facts, a synopsis and buttons.",
			Props: []PropSpec{
				pText("title", "Title", "Named to assistive technology and used by the fallback; not drawn.", 120),
				pEnum("style", "Style", "A full billboard, or a shorter banner.", "billboard", "billboard|Billboard", "banner|Banner"),
				pInt("autoAdvanceSec", "Seconds per title", "How long each title stays before the next one. 0 never moves by itself.", 8, 0, 60),
				pBool("showSynopsis", "Show the synopsis", "Two or three lines about the title.", true),
				pBool("showLogo", "Use the title's logo", "The title's logo art in place of its name, when it has one.", true),
				pList("actions", "Buttons", "The buttons on each title, in order.", []string{"play", "details"}, "play|Play", "details|More info", "watchLater|Watch later"),
				pInt("maxItems", "Most titles", "How many titles it rotates through.", 5, 1, 8),
			},
			Renders: map[string]string{
				"tv":      "The top 620 points of the screen: backdrop full width behind a left-to-right scrim, the logo or title at the heading size bottom-left, facts, at most three lines of synopsis, then the buttons in a row. Left and right on the buttons' row past its ends change title; it never advances while one of its buttons has focus.",
				"phone":   "A 16/9 backdrop the width of the screen (2/3 with style billboard), title and facts over a bottom scrim, buttons below; swipe sideways to change title, page dots under it.",
				"tablet":  "A 21/9 backdrop with the text block at the bottom-left third.",
				"desktop": "A 21/9 backdrop at most 620 points tall, text block bottom-left, arrows at the edges on hover, page dots bottom-right.",
			},
			States:  map[string]string{"loading": "A grey block of the hero's size.", "empty": "Left out.", "error": "Left out."},
			A11y:    "Role: region, labelled by the title prop or \"Featured\". Each slide is a group labelled \"<title>, <facts joined by comma>\", announced politely when it changes. With reduced motion it does not advance by itself and cross-fades are cuts.",
			Example: example(`{"id":"hero","component":"Hero","source":{"type":"featured","limit":5},"props":{"style":"billboard"}}`)},

		{Type: "Shelf", Label: "Row", Icon: "list", Category: "media", Since: 2, Required: true, Authored: true, Data: "required", Focus: "group",
			Description: "A titled row of cards that scrolls sideways. The workhorse of a home page.",
			Props: []PropSpec{propTitle, propSubtitle, propCardStyle, propCardSize,
				pBool("showTitles", "Show titles under cards", "Off draws the pictures alone; the titles are still read out.", true),
				pEnum("seeAll", "See all", "A link to everything the row is drawn from, when the source has such a place.", "auto", "auto|When there is more", "always|Always", "never|Never"),
				pBool("numbered", "Number the cards", "Large rank numbers beside the cards, for a top ten.", false),
				pInt("maxItems", "Most cards at first", "How many cards are sent before the row asks for more. 0 uses the device's own limit.", 0, 0, 60),
				propEmpty, propEmptyText,
			},
			Renders: map[string]string{
				"tv":      "A heading at the title size, then one row of cards 40 points apart that scrolls sideways under focus, the focused card kept a safe-area's width from the left edge and raised with the system's focus effect. The row is one focus section and remembers its last card. \"See all\" is a final card of the row's shape.",
				"phone":   "A heading with a chevron when there is a See all, then a row that scrolls by touch and bleeds off both edges; cards 12 points apart.",
				"tablet":  "As a phone with 16-point gaps.",
				"desktop": "A heading with a \"See all\" link at its end, a row that scrolls by wheel, drag or the arrow buttons that appear at its ends on hover; arrow keys move focus card to card.",
			},
			States:  statesData,
			A11y:    "Role: list, labelled by the title. Each card is a list item. When numbered, a card's label starts with \"Number <n>\".",
			Example: example(`{"id":"recent","component":"Shelf","props":{"title":"Recently added in {each}"},"repeat":{"over":"libraries"},"source":{"type":"recentlyAdded","library":"$each"}}`)},

		{Type: "Grid", Label: "Grid", Icon: "grid", Category: "media", Since: 2, Required: true, Authored: true, Data: "required", Focus: "group",
			Description: "Cards in rows and columns, as many columns as fit. It pages as the person scrolls: the body of a library.",
			Props: []PropSpec{propTitle, propCardStyle, propCardSize,
				pBool("showTitles", "Show titles under cards", "Off draws the pictures alone.", true),
				pInt("maxItems", "Most cards at first", "How many cards the first page holds. 0 uses the device's own limit.", 0, 0, 200),
				propEmpty, propEmptyText,
			},
			Renders: map[string]string{
				"tv":      "layout.gridColumns columns for the card's shape (6 posters, 4 landscape cards), 40 points apart, focus moving by row and column; the next page is asked for when focus is within two rows of the end.",
				"phone":   "As many columns of the card's width as fit, at least 2 (3 for posters at size s or m), 12 points apart.",
				"tablet":  "As many columns as fit, 16 points apart.",
				"desktop": "As many columns as fit: floor((width + gap) / (card width + gap)), cards stretched to fill the row; the next page is asked for when the end is within one screen.",
			},
			States:  statesData,
			A11y:    "Role: grid (list on touch devices), labelled by the title or the page's title.",
			Example: example(`{"id":"all","component":"Grid","source":{"type":"browse","library":"$library","filterable":true}}`)},

		{Type: "Card", Label: "Card", Icon: "photo", Category: "media", Since: 2, Required: true, Data: "none", Focus: "item",
			Description: "One item in a Shelf, Grid, Hero or SeasonList. Made by the server from the source's items; see the card styles for what each style shows.",
			Props: []PropSpec{
				pOf(PropString, "style", "Style", "The card style."), pOf(PropString, "shape", "Shape", "poster, landscape, square, circle or chip: what a renderer that doesn't know the style draws."),
				pOf(PropString, "title", "Title", "Always sent, and always in the label, even where the style doesn't draw it."), pOf(PropString, "subtitle", "Subtitle", ""),
				pOf(PropString, "synopsis", "Synopsis", "Episode cards and hero slides."), pOf(PropString, "remaining", "Time left", "Progress cards: \"42 min left\"."),
				pOf(PropString, "itemId", "Item", "The item's ID."), pOf(PropString, "kind", "Kind", "The item's kind."), pOf(PropString, "playId", "Plays", "What a play affordance on the card starts, when the item can be played."),
				pOf(PropObject, "image", "Picture", "{url, width, height, aspect, kind}: the address with the width to ask for."),
				pOf(PropNumber, "progress", "Progress", "0 to 1."), pOf(PropBool, "watched", "Watched", ""), pOf(PropInt, "unwatched", "Unwatched count", ""),
				pOf(PropList, "badges", "Badges", "At most three short labels."), pOf(PropInt, "rank", "Rank", "In a numbered row."),
				pOf(PropString, "number", "Channel number", ""), pOf(PropObject, "now", "On now", "{title, start, end}"), pOf(PropObject, "next", "On next", "{title, start, end}"),
				pOf(PropIcon, "icon", "Icon", "For tiles and chips without a picture."), pOf(PropInt, "count", "Count", ""),
				pOf(PropList, "facts", "Facts", "Hero slides: [{id, value}]."), pOf(PropList, "actions", "Buttons", "Hero slides: [{id, label, icon, primary, action}]."), pOf(PropObject, "logo", "Logo", "Hero slides: the title's logo art."),
			},
			Renders: map[string]string{
				"tv":      "The picture at the row's card size with the system focus effect (lift, parallax on tvOS; scale 1.08 and a 3-point ring in token accent elsewhere); the title appears under the focused card only, unless showTitles.",
				"phone":   "Picture, then title and subtitle under it in the title and caption sizes. A long press opens the app's own item menu.",
				"desktop": "Picture with a hover scrim and play button when it can be played, title and subtitle under it; a context menu on right click.",
			},
			States: map[string]string{"loading": "A grey rounded rectangle of the card's shape.", "error": "The picture's place filled with token bg3 and the item kind's icon."},
			A11y:   "Role: button (link on the web). Label: the props its style shows, in the style's order, joined by \", \": progress as \"<n> percent watched\", watched as \"Watched\", unwatched as \"<n> unwatched\", badges as written, a channel as \"Channel <number>, <title>, now <now.title>, next <next.title>\"."},

		{Type: "Tabs", Label: "Tabs", Icon: "more", Category: "navigation", Since: 2, Authored: true, Data: "none", Container: true, Children: []string{"Tab"}, Focus: "group",
			Fallback: "Stack", FallbackNote: "The selected tab's content alone, in a column.",
			Description: "A strip of tabs, each with its own sections. Only the selected tab's content is sent; choosing another asks for the page again with tab=<id>.",
			Props: []PropSpec{
				pEnum("style", "Style", "Tabs for sections of a page, segments for views of the same thing.", "tabs", "tabs|Tabs", "segments|Segments"),
				pOf(PropSection, "default", "Opens on", "The tab shown first. A profile may choose its own."),
			},
			Renders: map[string]string{
				"tv":      "A row of pills centred above the content; left and right move between them and the content changes as focus moves; down enters the content.",
				"phone":   "A segmented control (segments) or a scrolling row of text tabs with an underline (tabs); swiping the content sideways changes tab.",
				"desktop": "Text tabs with an underline in token accent, left-aligned; arrow keys move between them, Enter selects.",
			},
			A11y:    "Role: tablist; each Tab is a tab with selected state; the content is its tabpanel.",
			Example: example(`{"id":"views","component":"Tabs","children":[{"id":"albums","component":"Tab","props":{"label":"Albums"},"children":[{"id":"albums-grid","component":"Grid","source":{"type":"albums","library":"$library","sort":"title"}}]},{"id":"artists","component":"Tab","props":{"label":"Artists"},"children":[{"id":"artists-grid","component":"Grid","source":{"type":"artists","library":"$library"}}]}]}`)},

		{Type: "Tab", Label: "Tab", Icon: "more", Category: "navigation", Since: 2, Authored: true, Data: "none", Container: true, Focus: "item",
			Fallback: "Stack", FallbackNote: "Its content in a column.",
			Description: "One tab of a Tabs section: a label and the sections shown when it is chosen.",
			Props:       []PropSpec{req(pText("label", "Label", "The tab's name.", 40)), propIcon},
			A11y:        "Role: tab. Label: the label."},

		{Type: "FilterBar", Label: "Filter and sort", Icon: "filter", Category: "navigation", Since: 2, Authored: true, Data: "none", Focus: "stop",
			Fallback: "", FallbackNote: "Left out: the grid shows everything in its default order.",
			Description: "The controls that filter and sort a Grid on the same page. The server fills in each filter's choices from what the library holds; a change asks for the page again with the choice as a query parameter.",
			Props: []PropSpec{
				req(pOf(PropSection, "target", "Filters", "The Grid this bar filters. Its source must be filterable.")),
				pList("fields", "Filters", "Which filters are offered, in order. One with no choices in this library is left out.", []string{"genre", "decade", "watched"},
					"genre|Genre", "decade|Decade", "year|Year", "content|Rating", "resolution|Resolution", "watched|Watched", "studio|Studio", "network|Network", "tag|Tag", "service|Streaming on"),
				pList("sorts", "Sort by", "The orders offered; the first is the default.", []string{"title", "added", "newest", "rating"},
					"title|Title", "added|Date added", "newest|Newest release", "oldest|Oldest release", "rating|Rating", "shuffle|Shuffle"),
				pBool("showCount", "Show how many", "The number of titles found.", true),
			},
			Renders: map[string]string{
				"tv":      "One row of pills: Sort first, then each filter showing its current choice; OK on a pill opens a full-height list from the right edge to choose from.",
				"phone":   "A row of chips that scrolls sideways; a chip opens a bottom sheet of choices. The count sits above it in the caption style.",
				"desktop": "A row of drop-downs with the count at the far end and a Clear button when anything is chosen.",
			},
			A11y:    "Role: toolbar, labelled \"Filter and sort\". Each field is a pop-up button labelled \"<label>: <current choice or Any>\".",
			Example: example(`{"id":"filters","component":"FilterBar","props":{"target":"all","fields":["genre","decade","watched"]}}`)},

		{Type: "DetailHeader", Label: "Title header", Icon: "info", Category: "detail", Since: 2, Authored: true, Data: "none", Focus: "stop",
			Fallback: "Stack", FallbackNote: "A column: the backdrop as an Image, the title and overview as Text, and the actions as Buttons in a row.",
			Description: "The top of a title's page: its art, name, facts, score, synopsis and the buttons that play it and act on it. It shows the title the page was opened for.",
			Props: []PropSpec{
				pEnum("layout", "Layout", "Backdrop behind the text, poster beside it, or a compact strip.", "backdrop", "backdrop|Backdrop", "poster|Poster beside", "compact|Compact"),
				pList("actions", "Buttons", "The buttons offered, in order. One the title or the profile can't use is left out.", []string{"play", "restart", "watchLater", "watched", "download", "share"},
					"play|Play or Resume", "restart|Play from the start", "watchLater|Watch later", "watched|Mark watched", "download|Download", "share|Share", "playOn|Play on another device", "watchParty|Watch party"),
				pList("facts", "Facts", "The line of facts under the title, in order.", []string{"year", "runtime", "contentRating", "rating", "genres"},
					"year|Year", "runtime|Running time", "contentRating|Certificate", "rating|Score", "genres|Genres", "studio|Studio", "count|How many it holds", "season|Season and episode"),
				pBool("showOverview", "Show the synopsis", "", true),
				pBool("showTagline", "Show the tagline", "", true),
			},
			Renders: map[string]string{
				"tv":      "Backdrop across the whole screen under a scrim; the logo or title at the page title size at the left, the facts on one line, at most four lines of synopsis, the buttons in a row with Play first and focused. Sections below scroll up over the backdrop.",
				"phone":   "Backdrop 16/9 at the top, title and facts under it, a full-width primary button, the other actions as a row of icon buttons with captions, then the synopsis, cut to four lines with More.",
				"tablet":  "As a phone with the poster beside the text block.",
				"desktop": "Backdrop behind; poster at the left (layout poster or backdrop), text block beside it, buttons in a row: primary filled, the rest outlined.",
			},
			States:  map[string]string{"loading": "A grey backdrop and three grey text lines.", "error": "A Notice: \"This title isn't available\" with a button back to Home."},
			A11y:    "Role: region labelled by the title. The title is a level-1 heading. Each button's label is its label, the primary button's \"Resume from <time>\" when it resumes.",
			Example: example(`{"id":"header","component":"DetailHeader","props":{"layout":"backdrop"}}`)},

		{Type: "SeasonList", Label: "Seasons and episodes", Icon: "show", Category: "detail", Since: 2, Authored: true, Data: "required", Accepts: []string{"episodes"}, Focus: "group",
			Fallback: "Shelf", FallbackNote: "A Shelf of the selected season's episodes as landscape cards, titled with the season's name.",
			Description: "A show's seasons with the chosen season's episodes. Choosing a season asks for its episodes through the season's own URL.",
			Props: []PropSpec{
				pEnum("style", "Episodes as", "Cards with a still and synopsis, or a compact list.", "cards", "cards|Cards", "list|List"),
				pBool("showSynopsis", "Show each synopsis", "", true),
			},
			Renders: map[string]string{
				"tv":      "A row of season pills, then the episodes as a row of episode cards (still, \"<n>. <title>\", runtime, two lines of synopsis) that scrolls sideways; focus starts on the next episode to watch.",
				"phone":   "A season menu button, then the episodes as a vertical list: still at the left with a progress bar, title, runtime and two lines of synopsis.",
				"desktop": "Season tabs, then a vertical list (style list) or a wrapping grid of episode cards (style cards).",
			},
			States:  statesData,
			A11y:    "Role: region labelled \"Episodes\". The season picker is a pop-up button \"Season <n>\"; each episode's label is \"Episode <n>, <title>, <runtime>, <n> percent watched\".",
			Example: example(`{"id":"episodes","component":"SeasonList","source":{"type":"episodes","show":"$show"}}`)},

		{Type: "TrackList", Label: "Track list", Icon: "music", Category: "detail", Since: 2, Authored: true, Data: "required", Accepts: []string{"tracks"}, Focus: "group",
			Fallback: "List", FallbackNote: "A List of ListItems: the track's title, artist as subtitle and length as trailing text.",
			Description: "An album's songs, an audiobook's chapters or a podcast's episodes, numbered, with their lengths.",
			Props: []PropSpec{
				pBool("showNumbers", "Show track numbers", "", true),
				pBool("showArtist", "Show each track's artist", "For compilations.", false),
				pBool("showDuration", "Show lengths", "", true),
			},
			Renders: map[string]string{
				"tv":      "A list down the right two thirds of the screen, one row per track: number, title, length; the playing track in token accent.",
				"phone":   "Full-width rows 56 points tall: number, title over artist, length; swipe actions are the app's own.",
				"desktop": "A table: #, Title, Artist (when shown), Length; double-click plays.",
			},
			States:  statesData,
			A11y:    "Role: list. Each track: \"Track <n>, <title>, <artist>, <minutes> minutes <seconds> seconds\".",
			Example: example(`{"id":"tracks","component":"TrackList","source":{"type":"tracks","library":"$library","artist":"$artist","album":"$album"}}`)},

		{Type: "Track", Label: "Track", Icon: "music", Category: "detail", Since: 2, Data: "none", Focus: "item",
			Fallback: "ListItem", FallbackNote: "title, subtitle (the artist) and trailing (the length).",
			Description: "One row of a TrackList. Made by the server.",
			Props: []PropSpec{pOf(PropString, "title", "Title", ""), pOf(PropString, "artist", "Artist", ""), pOf(PropInt, "number", "Number", ""), pOf(PropInt, "durationSec", "Length in seconds", ""),
				pOf(PropString, "itemId", "Item", ""), pOf(PropNumber, "progress", "Progress", "0 to 1.")},
			A11y: "Role: button. Label as TrackList says."},

		{Type: "Banner", Label: "Banner", Icon: "star", Category: "media", Since: 2, Authored: true, Data: "optional", Focus: "item",
			Fallback: "Notice", FallbackNote: "A Notice with the banner's title, text and button.",
			Description: "One wide promotional block: a picture, a title, a line of text and one button. Written by hand (a collection's banner, a way into the TV guide), or drawn from the first item of a source (a random pick).",
			Props: []PropSpec{
				pText("title", "Title", "With a source, {title} is the item's title.", 120), pText("text", "Text", "With a source, {title}, {subtitle} and {overview} are the item's.", 300),
				pOf(PropImage, "image", "Picture", "A picture on this server. With a source, the item's backdrop is used."), propIcon,
				pEnum("style", "Style", "", "promo", "promo|Promo: picture behind", "compact|Compact: picture beside"),
				pText("buttonLabel", "Button", "The button's label.", 40),
				pEnum("action", "The button", "With a source: open the item's page, or play it.", "open", "open|Opens it", "play|Plays it"),
				propTo, propNative,
			},
			Renders: map[string]string{
				"tv":      "A 1760 by 300 point block: the picture behind a scrim, the title at the heading size and text at the left, one button. The block is one focus stop.",
				"phone":   "A full-width 16/9 card (promo) or a 96-point row with a square picture (compact); the whole banner is the button.",
				"desktop": "A block at most 360 points tall, picture behind a left-to-right scrim, text and button bottom-left.",
			},
			States:  map[string]string{"loading": "A grey block.", "empty": "Left out.", "error": "Left out."},
			A11y:    "Role: button (link on the web). Label: \"<title>, <text>, <button label>\".",
			Example: example(`{"id":"shuffle","component":"Banner","props":{"title":"Can't decide?","text":"{title}","buttonLabel":"Play something","action":"play"},"source":{"type":"random","limit":1}}`)},

		{Type: "Text", Label: "Text", Icon: "info", Category: "text", Since: 2, Required: true, Authored: true, Data: "none", Focus: "none",
			Description: "A heading or a block of text, plain or in the small Markdown subset (bold, italic, links to places in the app, lists, headings).",
			Props: []PropSpec{
				req(pOf(PropMarkdown, "text", "Text", "Placeholders such as {user}, {greeting}, {server} and {library} are filled in.")),
				pEnum("format", "Format", "", "plain", "plain|Plain", "markdown|Markdown"),
				pEnum("role", "Size", "Sizes come from the device, not from numbers.", "body", "pageTitle|Page title", "heading|Heading", "title|Title", "body|Body", "caption|Caption"),
				pEnum("align", "Align", "", "start", "start|Start", "center|Centre", "end|End"),
				pInt("maxLines", "Most lines", "Cut with an ellipsis after this many lines. 0 doesn't cut.", 0, 0, 20),
			},
			Renders: map[string]string{"all": "Text in token fg (caption in fg2) at layout.text[role] points, at most 70 characters wide for body text. Markdown links are drawn in token accentText and are focusable on a television only when the page has nothing else to focus."},
			A11y:    "Role: heading for pageTitle (level 1), heading (2) and title (3); otherwise text.",
			Example: example(`{"id":"hello","component":"Text","props":{"text":"{greeting}, {user}","role":"pageTitle"}}`)},

		{Type: "Notice", Label: "Notice", Icon: "info", Category: "text", Since: 2, Required: true, Authored: true, Data: "none", Focus: "item",
			Description: "A message that stands out: something to know, a warning, or a nudge with one button.",
			Props: []PropSpec{
				pEnum("tone", "Tone", "", "info", "info|Information", "success|Good news", "warning|Warning", "danger|Problem"),
				pText("title", "Title", "", 120), req(pText("text", "Text", "", 400)), propIcon,
				pText("buttonLabel", "Button", "", 40), propTo, propNative, propURL,
			},
			Renders: map[string]string{"all": "A rounded block in token bg2 with a 4-point bar at its leading edge in the tone's colour (accent, ok, warn, danger), the icon, title in the title size, text in body, and the button at the end (below on a phone). It takes focus only when it has a button."},
			A11y:    "Role: status (alert for warning and danger). Label: \"<title>. <text>\".",
			Example: example(`{"id":"setup","component":"Notice","when":{"admin":true,"flag":"setupIncomplete"},"props":{"tone":"info","title":"Finish setting up","text":"A few things are left to choose.","buttonLabel":"Open settings","native":"settings"}}`)},

		{Type: "Divider", Label: "Divider", Icon: "more", Category: "layout", Since: 2, Required: true, Authored: true, Data: "none", Focus: "none",
			Description: "A hairline between sections.",
			Props:       []PropSpec{pBool("inset", "Inset", "Start the line at the content's edge rather than the screen's.", false)},
			Renders:     map[string]string{"all": "A 1-point line in token border across the content width."},
			A11y:        "Role: separator; not announced.",
			Example:     example(`{"id":"rule","component":"Divider"}`)},

		{Type: "Spacer", Label: "Space", Icon: "more", Category: "layout", Since: 2, Required: true, Authored: true, Data: "none", Focus: "none",
			Description: "Empty space between sections.",
			Props:       []PropSpec{pEnum("size", "Size", "", "m", "xs|Extra small", "s|Small", "m|Medium", "l|Large", "xl|Extra large")},
			Renders:     map[string]string{"all": "layout.spacing[size] points of nothing, in the direction of its container."},
			A11y:        "Hidden from assistive technology.",
			Example:     example(`{"id":"gap","component":"Spacer","props":{"size":"l"}}`)},

		{Type: "Stack", Label: "Stack", Icon: "list", Category: "layout", Since: 2, Required: true, Authored: true, Data: "none", Container: true, Focus: "none",
			Description: "Sections one after another, down or across. The plain container for a custom page.",
			Props: []PropSpec{
				pEnum("direction", "Direction", "", "column", "column|Down", "row|Across"), propGap,
				pEnum("align", "Align", "Across the stack's direction.", "stretch", "start|Start", "center|Centre", "end|End", "stretch|Stretch"),
				pBool("wrap", "Wrap", "A row that doesn't fit continues on the next line.", true),
			},
			Renders: map[string]string{"all": "A flex column or row with layout.spacing[gap] between children. A row of Buttons is one focus group."},
			A11y:    "No role of its own.",
			Example: example(`{"id":"links","component":"Stack","props":{"direction":"row","gap":"s"},"children":[{"id":"guide","component":"Button","props":{"label":"TV guide","icon":"guide","native":"guide"}}]}`)},

		{Type: "Split", Label: "Two columns", Icon: "grid", Category: "layout", Since: 2, Authored: true, Data: "none", Container: true, Focus: "none",
			Fallback: "Stack", FallbackNote: "Its two children in a column.",
			Description: "Two sections side by side on a wide screen, one above the other on a narrow one.",
			Props: []PropSpec{
				pEnum("ratio", "Widths", "", "1:1", "1:1|Equal", "1:2|Narrow then wide", "2:1|Wide then narrow"), propGap,
				pEnum("stackBelow", "One column below", "The window width under which the two stack.", "medium", "compact|600 points", "medium|840 points", "expanded|1200 points"),
			},
			Renders: map[string]string{"tv": "Always side by side.", "phone": "Always stacked.", "desktop": "Side by side at or above the stackBelow breakpoint, stacked under it."},
			A11y:    "No role of its own.",
			Example: example(`{"id":"two","component":"Split","props":{"ratio":"2:1"},"children":[{"id":"left","component":"Text","props":{"text":"About this server"}},{"id":"right","component":"Button","props":{"label":"Settings","native":"settings"}}]}`)},

		{Type: "Button", Label: "Button", Icon: "plus", Category: "navigation", Since: 2, Required: true, Authored: true, Data: "none", Focus: "item",
			Description: "A button that opens a place in the app, one of the app's own screens, or a web page.",
			Props: []PropSpec{
				req(pText("label", "Label", "", 60)), propIcon,
				pEnum("variant", "Look", "", "secondary", "primary|Primary", "secondary|Secondary", "ghost|Plain"),
				propTo, propNative, propURL,
			},
			Renders: map[string]string{"all": "The platform's own button: filled in token accent with accentFg text (primary), outlined (secondary) or text only (ghost); at least 44 points tall on touch devices, 66 on a television."},
			A11y:    "Role: button (link on the web when it navigates). Label: the label.",
			Example: example(`{"id":"open-settings","component":"Button","props":{"label":"Settings","icon":"settings","native":"settings"}}`)},

		{Type: "Image", Label: "Picture", Icon: "photo", Category: "text", Since: 2, Required: true, Authored: true, Data: "none", Focus: "none",
			Description: "A picture from this server, optionally a link.",
			Props: []PropSpec{
				req(pOf(PropImage, "src", "Picture", "A path on this server beginning /api/.")), pText("alt", "Description", "For people who can't see the picture. Leave empty when it is decoration.", 200),
				pEnum("aspect", "Shape", "", "16/9", "16/9|Wide", "21/9|Extra wide", "4/3|Classic", "1/1|Square", "2/3|Poster"),
				pEnum("fit", "Fit", "", "cover", "cover|Fill and crop", "contain|Show all of it"),
				pBool("rounded", "Rounded corners", "", true), propTo,
			},
			Renders: map[string]string{"all": "The picture at the content width (at most 960 points) in its aspect, asked for at the width in props.image. Focusable only with a to."},
			A11y:    "Role: img with alt as its label; hidden when alt is empty and it isn't a link.",
			Example: example(`{"id":"pic","component":"Image","props":{"src":"/api/collections/4/cover","alt":"Family films","aspect":"21/9"}}`)},

		{Type: "SearchBox", Label: "Search box", Icon: "search", Category: "navigation", Since: 2, Authored: true, Data: "none", Focus: "item",
			Fallback: "Button", FallbackNote: "A Button labelled \"Search\" that opens /search.",
			Description: "The field people type a search into. On the search page it holds the current search; anywhere else it opens the search page.",
			Props:       []PropSpec{pText("placeholder", "Prompt", "", 80), pBool("autofocus", "Start here", "Focus starts in the field.", false)},
			Renders: map[string]string{
				"tv":      "The system's search field and keyboard (UISearchController on tvOS, the leanback search bar on Android TV); results appear under it as they are typed.",
				"phone":   "A search field under the title; typing asks for the page again with q after 300 ms without a keystroke.",
				"desktop": "A search field at most 560 points wide; Enter or 300 ms without a keystroke searches.",
			},
			A11y:    "Role: searchbox labelled \"Search\".",
			Example: example(`{"id":"q","component":"SearchBox","props":{"placeholder":"Films, shows, music…","autofocus":true}}`)},

		{Type: "List", Label: "List", Icon: "list", Category: "media", Since: 2, Required: true, Authored: true, Data: "optional", Container: true, Children: []string{"ListItem"}, Focus: "group",
			Description: "Rows of text with an icon or small picture, written by hand or drawn from a source.",
			Props:       []PropSpec{propTitle, pBool("dense", "Compact rows", "", false), propEmpty, propEmptyText},
			Renders:     map[string]string{"all": "Full-width rows (56 points, 44 dense, 90 on a television): picture or icon, title over subtitle, trailing text, and a chevron when the row opens something."},
			States:      statesData,
			A11y:        "Role: list labelled by the title.",
			Example:     example(`{"id":"lists","component":"List","props":{"title":"Your collections"},"source":{"type":"collections"}}`)},

		{Type: "ListItem", Label: "List row", Icon: "list", Category: "media", Since: 2, Required: true, Authored: true, Data: "none", Focus: "item",
			Description: "One row of a List.",
			Props:       []PropSpec{req(pText("title", "Title", "", 120)), pText("subtitle", "Subtitle", "", 160), propIcon, pText("trailing", "Text at the end", "", 40), propTo, propNative, propURL},
			A11y:        "Role: button when it opens something, else text. Label: \"<title>, <subtitle>, <trailing>\"."},

		{Type: "NativeView", Label: "The app's own view", Icon: "plug", Category: "structure", Since: 2, Authored: true, Data: "none", Container: true, Focus: "stop",
			Fallback: "Stack", FallbackNote: "Its children in a column: what to show where the app has no such view.",
			Description: "A place where the app draws one of its own rich views: the TV guide's grid, the photo timeline. An app that has the view says so; any other is sent the sections inside instead.",
			Props: []PropSpec{
				req(pEnum("view", "View", "", "", "guide|TV guide grid", "photoTimeline|Photo timeline", "photoMap|Photo map", "coverFlow|Album cover flow")),
				pOf(PropLibrary, "library", "Library", "The library the view shows, where it shows one."),
			},
			Renders: map[string]string{"all": "The app's own view fills the rest of the page; nothing may follow a NativeView in a page."},
			A11y:    "Whatever the app's own view provides.",
			Example: example(`{"id":"timeline","component":"NativeView","props":{"view":"photoTimeline","library":"$library"},"children":[{"id":"photos","component":"Grid","props":{"cardStyle":"photo"},"source":{"type":"recentlyAdded","library":"$library"}}]}`)},
	}
}

// cardStylesOnce is the card style table, built once: the device profiles need
// it before the catalogue, which holds the profiles, exists.
var cardStylesOnce = sync.OnceValue(buildCardStyles)

func buildCardStyles() []CardStyleSpec {
	return []CardStyleSpec{
		{Style: "poster", Label: "Poster", Shape: "poster", Aspect: "2/3", Image: []string{"poster"}, Shows: []string{"title", "subtitle", "progress", "watched", "unwatched", "badges"}, TitleLines: 1, SubtitleLines: 1,
			Description: "Upright film and show art with the title and year under it.", Kinds: []string{"movie", "show", "disc"}},
		{Style: "landscape", Label: "Landscape", Shape: "landscape", Aspect: "16/9", Image: []string{"backdrop", "poster"}, Shows: []string{"title", "subtitle", "badges"}, TitleLines: 1, SubtitleLines: 1,
			Description: "A wide still or backdrop with the title under it.", Kinds: []string{"video", "recording"}},
		{Style: "progress", Label: "Landscape with progress", Shape: "landscape", Aspect: "16/9", Image: []string{"backdrop", "poster"}, Shows: []string{"title", "subtitle", "progress", "remaining"}, TitleLines: 1, SubtitleLines: 1,
			Description: "A wide card with a progress bar along its bottom edge and how long is left: Continue watching."},
		{Style: "episode", Label: "Episode", Shape: "landscape", Aspect: "16/9", Image: []string{"backdrop", "poster"}, Shows: []string{"title", "subtitle", "synopsis", "progress", "watched"}, TitleLines: 1, SubtitleLines: 1,
			Description: "An episode's still with \"<n>. <title>\", its running time and two lines of synopsis.", Kinds: []string{"episode"}},
		{Style: "square", Label: "Square", Shape: "square", Aspect: "1/1", Image: []string{"square", "poster"}, Shows: []string{"title", "subtitle"}, TitleLines: 1, SubtitleLines: 1,
			Description: "A square picture with a title and subtitle.", Kinds: []string{"podcast", "audiobook", "track"}},
		{Style: "album", Label: "Album", Shape: "square", Aspect: "1/1", Image: []string{"square", "poster"}, Shows: []string{"title", "subtitle"}, TitleLines: 1, SubtitleLines: 1,
			Description: "An album cover with the album and artist under it.", Kinds: []string{"album"}},
		{Style: "circle", Label: "Circle", Shape: "circle", Aspect: "1/1", Image: []string{"square", "poster"}, Shows: []string{"title"}, TitleLines: 2, SubtitleLines: 0,
			Description: "A round picture with a centred name."},
		{Style: "person", Label: "Person", Shape: "circle", Aspect: "1/1", Image: []string{"square", "poster"}, Shows: []string{"title", "subtitle"}, TitleLines: 1, SubtitleLines: 1,
			Description: "A round portrait with a name and a line under it: an artist, an author.", Kinds: []string{"artist"}},
		{Style: "book", Label: "Book cover", Shape: "poster", Aspect: "2/3", Image: []string{"poster"}, Shows: []string{"title", "subtitle", "progress"}, TitleLines: 2, SubtitleLines: 1,
			Description: "A book or comic cover with the title and author, and how far it has been read.", Kinds: []string{"book"}},
		{Style: "game", Label: "Game box", Shape: "poster", Aspect: "3/4", Image: []string{"poster"}, Shows: []string{"title", "subtitle", "badges"}, TitleLines: 1, SubtitleLines: 1,
			Description: "Box art with the game's name and console.", Kinds: []string{"game"}},
		{Style: "photo", Label: "Photo", Shape: "square", Aspect: "1/1", Image: []string{"poster", "square"}, Shows: []string{}, TitleLines: 0, SubtitleLines: 0,
			Description: "The picture alone, cropped square; its name is only read out.", Kinds: []string{"photo"}},
		{Style: "channel", Label: "Channel", Shape: "landscape", Aspect: "16/9", Image: []string{"logo"}, Shows: []string{"number", "title", "now", "next", "progress"}, TitleLines: 1, SubtitleLines: 2,
			Description: "A channel's logo on a plain tile, its number and name, what is on now with how far through it is, and what is next.", Kinds: []string{"channel"}},
		{Style: "tile", Label: "Tile", Shape: "landscape", Aspect: "16/9", Image: []string{"backdrop", "poster"}, Shows: []string{"title", "count"}, TitleLines: 1, SubtitleLines: 0,
			Description: "A wide tile with its name over the picture, or over a plain tile with an icon: a library, a collection.", Kinds: []string{"library", "collection", "view"}},
		{Style: "chip", Label: "Chip", Shape: "chip", Aspect: "", Shows: []string{"title"}, TitleLines: 1, SubtitleLines: 0,
			Description: "A pill with a name alone: a genre.", Kinds: []string{"genre"}},
	}
}

var (
	paramLimit   = pInt("limit", "Most items", "How many items it gives in all. 0 has no limit of its own; the device still pages it.", 0, 0, 500)
	paramLibrary = pOf(PropLibrary, "library", "Library", "One library; leave empty for every library the profile can open. $library on a library page, $each in a row repeated over libraries.")
	paramKind    = pEnum("kind", "Kind of library", "Only libraries of this kind.", "", LibraryKinds...)
)

func buildSources() []SourceSpec {
	browseSort := pEnum("sort", "Order", "", "", "title|Title", "added|Date added", "newest|Newest release", "oldest|Oldest release", "rating|Score", "shuffle|Shuffled")
	return []SourceSpec{
		{Type: "continueWatching", Label: "Continue watching", Icon: "history", Category: "personal", Since: 2, CardStyle: "progress", Title: "Continue watching", Paged: false,
			Description: "What the profile is part-way through, most recent first.",
			Params:      []PropSpec{pEnum("media", "What", "", "all", "all|Everything", "watch|Films and episodes", "read|Books", "listen|Music, audiobooks and podcasts"), paramLimit},
			Yields:      []string{"movie", "episode", "video", "disc", "book", "track", "audiobook", "podcast"},
			Example:     example(`{"type":"continueWatching","media":"watch"}`)},
		{Type: "nextUp", Label: "Next up", Icon: "play", Category: "personal", Since: 2, CardStyle: "episode", Title: "Next up",
			Description: "The next episode of every show the profile has started and not finished.",
			Params:      []PropSpec{paramLimit}, Yields: []string{"episode"}, Example: example(`{"type":"nextUp"}`)},
		{Type: "recommended", Label: "Recommended for you", Icon: "sparkles", Category: "personal", Since: 2, CardStyle: "poster", Title: "Recommended for you",
			Description: "Films and shows picked from what this profile has watched on this server. Nothing leaves the server.",
			Params:      []PropSpec{paramLimit}, Yields: []string{"movie", "show"}, Example: example(`{"type":"recommended","limit":20}`)},
		{Type: "watchLater", Label: "Watch later", Icon: "clock", Category: "personal", Since: 2, CardStyle: "poster", Title: "Watch later", Paged: true, SeeAll: "/collections",
			Description: "The profile's own Watch later list.",
			Params:      []PropSpec{paramLimit}, Yields: []string{"movie", "show", "episode", "video"}, Example: example(`{"type":"watchLater"}`)},
		{Type: "savedView", Label: "A saved view", Icon: "filter", Category: "personal", Since: 2, CardStyle: "poster", Paged: true, Title: "{each}",
			Description: "What one of the profile's saved library views finds now. With a row repeated over pinned views, $each is each view.",
			Params:      []PropSpec{req(pOf(PropRef, "view", "View", "The saved view's ID, or $each.")), paramLimit}, Yields: []string{"movie", "show"},
			Example: example(`{"type":"savedView","view":"$each"}`)},

		{Type: "recentlyAdded", Label: "Recently added", Icon: "plus", Category: "library", Since: 2, CardStyle: "poster", Paged: true, SeeAll: "/library/{library}", Title: "Recently added",
			Description: "The newest things in a library, or across every library of a kind. Shows for a TV library, albums for music, the items themselves for the rest.",
			Params:      []PropSpec{paramLibrary, paramKind, paramLimit}, Yields: []string{"movie", "show", "album", "book", "game", "photo", "video", "podcast", "audiobook"},
			Example: example(`{"type":"recentlyAdded","library":"$each","limit":20}`)},
		{Type: "recentlyReleased", Label: "Recently released", Icon: "calendar", Category: "library", Since: 2, CardStyle: "poster", Paged: true, SeeAll: "/library/{library}?sort=newest", Title: "New releases",
			Description: "Films and shows by release date, newest first.",
			Params:      []PropSpec{paramLibrary, paramLimit}, Yields: []string{"movie", "show"}, Example: example(`{"type":"recentlyReleased"}`)},
		{Type: "topRated", Label: "Top rated", Icon: "star", Category: "discover", Since: 2, CardStyle: "poster", Paged: true, SeeAll: "/library/{library}?sort=rating", Title: "Top rated",
			Description: "Films and shows by score, best first.",
			Params:      []PropSpec{paramLibrary, pInt("minRating", "At least", "Only titles scoring this or more, out of 10.", 0, 0, 10), paramLimit}, Yields: []string{"movie", "show"},
			Example: example(`{"type":"topRated","minRating":7,"limit":10}`)},
		{Type: "random", Label: "Random picks", Icon: "shuffle", Category: "discover", Since: 2, CardStyle: "poster", Title: "Something different",
			Description: "A shuffled handful of films and shows. The shuffle changes when reshuffle says.",
			Params: []PropSpec{paramLibrary, pEnum("reshuffle", "New picks", "", "daily", "hourly|Every hour", "daily|Every day", "weekly|Every week"),
				pEnum("watched", "Watched", "", "", "no|Unwatched only", "yes|Watched only"), paramLimit},
			Yields: []string{"movie", "show"}, Example: example(`{"type":"random","limit":12,"watched":"no"}`)},
		{Type: "browse", Label: "A filter", Icon: "filter", Category: "discover", Since: 2, CardStyle: "poster", Paged: true, SeeAll: "/library/{library}", Title: "",
			Description: "Anything the library page's filters can find: the server's own filter and sort engine. With filterable on, the page's FilterBar and its query parameters are added to it.",
			Params: []PropSpec{paramLibrary, pStr("genre", "Genre", "", 100), pStr("studio", "Studio", "", 120), pStr("network", "Network", "", 120), pStr("tag", "Tag", "", 60),
				pStr("service", "Streaming on", "A streaming service, by its key: what it streams in the server's region. Needs streaming listings on.", 80),
				pStr("series", "Film series", "", 120), pInt("year", "Year", "", 0, 0, 2200), pInt("decade", "Decade", "The decade's first year: 1990.", 0, 0, 2200),
				pStr("content", "Certificate", "A content rating, such as PG.", 20), pInt("minRating", "Score at least", "Out of 10.", 0, 0, 10),
				pEnum("resolution", "Resolution", "", "", "4k|4K", "1080p", "720p", "sd|SD"), pEnum("watched", "Watched", "", "", "no|Unwatched", "yes|Watched"),
				pEnum("titles", "Only", "", "", "video|Films", "show|Shows", "disc|Discs"), browseSort, pBool("desc", "Reverse the order", "", false),
				pBool("filterable", "Follows the page's filters", "The page's FilterBar and query parameters narrow and sort this source.", false), paramLimit},
			Yields: []string{"movie", "show", "disc"}, Example: example(`{"type":"browse","library":"$library","genre":"Animation","sort":"rating"}`)},
		{Type: "genre", Label: "A genre", Icon: "filter", Category: "discover", Since: 2, CardStyle: "poster", Paged: true, SeeAll: "/library/{library}?genre={genre}", Title: "{genre}",
			Description: "Films and shows of one genre.",
			Params:      []PropSpec{req(pStr("genre", "Genre", "", 100)), paramLibrary, browseSort, paramLimit}, Yields: []string{"movie", "show"},
			Example: example(`{"type":"genre","genre":"Comedy"}`)},
		{Type: "studio", Label: "A studio or network", Icon: "globe", Category: "discover", Since: 2, CardStyle: "poster", Paged: true, Title: "{studio}",
			Description: "Films from one studio, or shows from one network.",
			Params:      []PropSpec{pStr("studio", "Studio", "", 120), pStr("network", "Network", "", 120), paramLibrary, browseSort, paramLimit}, Yields: []string{"movie", "show"},
			Example: example(`{"type":"studio","studio":"Studio Ghibli"}`)},
		{Type: "tag", Label: "A tag", Icon: "filter", Category: "discover", Since: 2, CardStyle: "poster", Paged: true, Title: "{tag}",
			Description: "Everything carrying one tag.",
			Params:      []PropSpec{req(pStr("tag", "Tag", "", 60)), paramLibrary, browseSort, paramLimit}, Yields: []string{"movie", "show"},
			Example: example(`{"type":"tag","tag":"christmas"}`)},
		{Type: "collection", Label: "A collection", Icon: "collection", Category: "discover", Since: 2, CardStyle: "poster", Paged: true, SeeAll: "/collections/{collection}", Title: "{each}",
			Description: "What one collection holds: a list someone made, or a rule. $collection on a collection's page.",
			Params:      []PropSpec{req(pOf(PropRef, "collection", "Collection", "The collection's ID, $collection or $each.")), paramLimit}, Yields: []string{"movie", "show", "episode", "video"},
			Example: example(`{"type":"collection","collection":12}`)},
		{Type: "collections", Label: "Collections", Icon: "collection", Category: "structure", Since: 2, CardStyle: "tile", SeeAll: "/collections", Title: "Collections",
			Description: "The collections the profile can see, as tiles.",
			Params:      []PropSpec{paramLimit}, Yields: []string{"collection"}, Example: example(`{"type":"collections"}`)},
		{Type: "featured", Label: "Featured", Icon: "sparkles", Category: "discover", Since: 2, CardStyle: "landscape", Title: "Featured",
			Description: "Recent films with a backdrop: what a Spotlight is usually made of.",
			Params:      []PropSpec{paramLibrary, paramLimit}, Yields: []string{"movie", "video"}, Example: example(`{"type":"featured","limit":5}`)},
		{Type: "libraries", Label: "Libraries", Icon: "library", Category: "structure", Since: 2, CardStyle: "tile", Title: "Your libraries",
			Description: "The libraries the profile can open, as tiles.",
			Params:      []PropSpec{paramKind, paramLimit}, Yields: []string{"library"}, Example: example(`{"type":"libraries"}`)},
		{Type: "genres", Label: "Genres", Icon: "filter", Category: "structure", Since: 2, CardStyle: "chip", Title: "Genres",
			Description: "A library's genres, each opening the library filtered to it.",
			Params:      []PropSpec{req(paramLibrary), paramLimit}, Yields: []string{"genre"}, Example: example(`{"type":"genres","library":"$library"}`)},

		{Type: "liveNow", Label: "On TV now", Icon: "live", Category: "live", Since: 2, CardStyle: "channel", SeeAll: "/tv", Title: "On TV now",
			Description: "The channels the profile may watch, with what is on now and next.",
			Params:      []PropSpec{pBool("favourites", "Favourite channels only", "", false), paramLimit}, Yields: []string{"channel"}, Example: example(`{"type":"liveNow","favourites":true}`)},
		{Type: "recordings", Label: "Recordings", Icon: "record", Category: "live", Since: 2, CardStyle: "landscape", SeeAll: "/tv/recordings", Title: "Recordings",
			Description: "The profile's finished Live TV recordings, newest first.",
			Params:      []PropSpec{paramLimit}, Yields: []string{"recording"}, Example: example(`{"type":"recordings"}`)},

		{Type: "albums", Label: "Albums", Icon: "album", Category: "music", Since: 2, CardStyle: "album", Paged: true, SeeAll: "/library/{library}", Title: "Albums",
			Description: "A music or audiobook library's albums.",
			Params: []PropSpec{req(paramLibrary), pEnum("sort", "Order", "", "added", "added|Date added", "played|Last played", "title|Title", "artist|Artist", "year|Year"),
				pStr("artist", "Artist", "Only this artist's albums. $artist on an artist's page.", 200), pStr("genre", "Genre", "", 100), paramLimit},
			Yields: []string{"album"}, Example: example(`{"type":"albums","library":"$each","sort":"played"}`)},
		{Type: "artists", Label: "Artists", Icon: "artist", Category: "music", Since: 2, CardStyle: "person", Paged: true, SeeAll: "/library/{library}", Title: "Artists",
			Description: "A music library's album artists, or an audiobook library's authors.",
			Params:      []PropSpec{req(paramLibrary), paramLimit}, Yields: []string{"artist"}, Example: example(`{"type":"artists","library":"$library"}`)},
		{Type: "continuePlaying", Label: "Continue playing", Icon: "game", Category: "personal", Since: 2, CardStyle: "game", Title: "Continue playing",
			Description: "Games the profile has played, most recent first.",
			Params:      []PropSpec{paramLibrary, paramLimit}, Yields: []string{"game"}, Example: example(`{"type":"continuePlaying"}`)},

		{Type: "episodes", Label: "Episodes", Icon: "show", Category: "context", Since: 2, CardStyle: "episode", PageKinds: []string{"detail"}, Paged: true, Title: "Episodes",
			Description: "One season of a show. Left to itself it picks the season of the next episode to watch.",
			Params:      []PropSpec{req(pOf(PropRef, "show", "Show", "$show on a show's or an episode's page.")), pOf(PropRef, "season", "Season", "A season number, or $season; empty picks one."), paramLimit},
			Yields:      []string{"episode"}, Example: example(`{"type":"episodes","show":"$show"}`)},
		{Type: "tracks", Label: "Tracks", Icon: "music", Category: "context", Since: 2, CardStyle: "square", PageKinds: []string{"detail"}, Title: "Tracks",
			Description: "An album's songs or a book's chapters, in order; without an album, every song by the artist, by title.",
			Params:      []PropSpec{req(paramLibrary), pStr("album", "Album", "$album on an album's page. Left out: every album's (an artist's page).", 300), pStr("artist", "Album artist", "$artist on an album's or an artist's page.", 300), paramLimit},
			Yields:      []string{"track"}, Example: example(`{"type":"tracks","library":"$library","artist":"$artist","album":"$album"}`)},
		{Type: "similar", Label: "More like this", Icon: "sparkles", Category: "context", Since: 2, CardStyle: "poster", PageKinds: []string{"detail"}, Title: "More like this",
			Description: "Titles in the same library sharing the title's first genre, best scored first, without the title itself.",
			Params:      []PropSpec{req(pOf(PropRef, "item", "Title", "$item on a film's page, $show on a show's.")), paramLimit}, Yields: []string{"movie", "show"},
			Example: example(`{"type":"similar","item":"$item","limit":12}`)},
		{Type: "search", Label: "Search results", Icon: "search", Category: "context", Since: 2, CardStyle: "poster", PageKinds: []string{"search"}, Title: "{each}",
			Description: "What a search finds in one library, or among the channels. Nothing under two characters.",
			Params: []PropSpec{req(pOf(PropRef, "query", "Search", "$query on the search page.")), paramLibrary,
				pEnum("in", "Look in", "", "library", "library|A library", "channels|TV channels"), paramLimit},
			Yields: []string{"movie", "show", "album", "book", "game", "channel"}, Example: example(`{"type":"search","query":"$query","library":"$each"}`)},
	}
}

func buildConditions() []ConditionSpec {
	c := func(field string, p PropSpec) ConditionSpec {
		p.Name = field
		return ConditionSpec{Field: field, PropSpec: p}
	}
	return []ConditionSpec{
		c("formFactor", pList("", "Device", "The kind of device.", nil, "tv|Television", "phone|Phone", "tablet|Tablet", "desktop|Desktop app", "web|Web browser")),
		c("platform", pList("", "App", "Which app.", nil, "web|Web", "tvos|Apple TV", "ios|iPhone and iPad", "android|Android", "androidtv|Android TV", "desktop|Desktop")),
		c("profileKind", pList("", "Kind of profile", "", nil, "adult|Adult", "kids|Kids", "guest|Guest")),
		c("admin", pBool("", "Administrator", "Only administrators (on) or only everyone else (off).", false)),
		c("feature", pStr("", "Server feature", "A name from /capabilities features, such as live-tv.", 64)),
		c("module", pStr("", "Module installed", "A module's ID, such as wittyplay.", 64)),
		c("can", pEnum("", "Profile may use", "One of the profile's parental feature switches.", "", "games|Games", "recordings|Recordings", "downloads|Downloads", "sharing|Sharing", "comments|Comments", "syncplay|Watch parties", "remoteControl|Play on other devices", "remote|Away from home", "editProfile|Edit its profile", "editLayout|Rearrange pages", "liveTv|Live TV")),
		c("hasLibrary", pEnum("", "Has a library of kind", "The profile can open at least one such library.", "", LibraryKinds...)),
		c("libraryKind", pEnum("", "This library is of kind", "On a library page.", "", LibraryKinds...)),
		c("capability", pStr("", "Device capability", "Something the client said it has: hdr, touch, pointer, keyboard.", 64)),
		c("flag", pEnum("", "The server says", "", "", "setupIncomplete|First-time setup isn't finished", "scanning|A library is being scanned", "librariesEmpty|There are no libraries", "hasQuery|Something was searched for", "screenTimeUp|The profile's screen time is over")),
		c("minWidth", pInt("", "Window at least", "Points wide.", 0, 0, 10000)),
		c("maxWidth", pInt("", "Window at most", "Points wide.", 0, 0, 10000)),
		c("timeFrom", pStr("", "From time of day", "HH:MM on the server's clock.", 5)),
		c("timeTo", pStr("", "Until time of day", "HH:MM; a range may pass midnight.", 5)),
		c("weekday", pList("", "Days", "", nil, "mon|Monday", "tue|Tuesday", "wed|Wednesday", "thu|Thursday", "fri|Friday", "sat|Saturday", "sun|Sunday")),
		c("locale", pStr("", "Language", "A language tag; en matches en-GB.", 35)),
	}
}

func buildPageKinds() []PageKindSpec {
	return []PageKindSpec{
		{Kind: "home", Label: "Home", Route: "/", Description: "The first page of the app.", Params: []string{"tab"}},
		{Kind: "libraries", Label: "Libraries", Route: "/libraries", Description: "Every library the profile can open.", Params: []string{"tab"}},
		{Kind: "library", Label: "A library", Route: "/library/{library}", Description: "One library's own page.",
			Params: []string{"library", "tab", "sort", "desc", "genre", "decade", "year", "content", "resolution", "watched", "studio", "network", "tag", "service",
				"series", "kind", "disc", "minRating", "audio", "subtitle", "format", "edition", "search", "seed"},
			Variables: []string{"$library"}, Match: []string{"library", "libraryKind"}},
		{Kind: "detail", Label: "A title", Route: "/item/{item}", Description: "The page of one film, show, episode, album, artist, book, game or collection.",
			Params:    []string{"item", "show", "season", "collection", "library", "artist", "album", "tab"},
			Variables: []string{"$item", "$show", "$season", "$collection", "$library", "$artist", "$album"}, Match: []string{"mediaKind", "library", "libraryKind"}},
		{Kind: "search", Label: "Search", Route: "/search", Description: "Search across everything.", Params: []string{"q", "tab"}, Variables: []string{"$query"}},
		{Kind: "livetv", Label: "Live TV", Route: "/tv", Description: "Channels and the guide.", Params: []string{"tab"}},
		{Kind: "music", Label: "Music", Route: "/music", Description: "Every music library together.", Params: []string{"tab"}},
		{Kind: "photos", Label: "Photos", Route: "/photos", Description: "Every photo library together.", Params: []string{"tab"}},
		{Kind: "books", Label: "Books", Route: "/books", Description: "Books, comics and audiobooks together: the book libraries and the audiobook libraries.", Params: []string{"tab"}},
		{Kind: "games", Label: "Games", Route: "/games", Description: "Every games library together.", Params: []string{"tab"}},
		{Kind: "collections", Label: "Collections", Route: "/collections", Description: "The profile's collections and lists.", Params: []string{"tab"}},
		{Kind: "custom", Label: "A page of your own", Route: "/page/{id}", Description: "Any other page: a family film night, a page for one collection.", Params: []string{"tab"}},
	}
}

func buildRoutes() []RouteSpec {
	return []RouteSpec{
		{Pattern: "/", Page: "home"},
		{Pattern: "/libraries", Page: "libraries"},
		{Pattern: "/library/{library}", Page: "library", Params: map[string]string{"library": "{library}"}},
		{Pattern: "/item/{item}", Page: "detail", Params: map[string]string{"item": "{item}"}},
		{Pattern: "/show/{show}", Page: "detail", Params: map[string]string{"show": "{show}"}},
		{Pattern: "/album/{library}", Page: "detail", Params: map[string]string{"library": "{library}", "artist": "?artist", "album": "?album"}},
		{Pattern: "/artist/{library}", Page: "detail", Params: map[string]string{"library": "{library}", "artist": "?name"}},
		{Pattern: "/collections/{collection}", Page: "detail", Params: map[string]string{"collection": "{collection}"}},
		{Pattern: "/collections", Page: "collections"},
		{Pattern: "/search", Page: "search", Params: map[string]string{"q": "?q"}},
		{Pattern: "/tv", Page: "livetv"},
		{Pattern: "/music", Page: "music"},
		{Pattern: "/photos", Page: "photos"},
		{Pattern: "/books", Page: "books"},
		{Pattern: "/games", Page: "games"},
		{Pattern: "/page/{id}", Page: "{id}"},
		{Pattern: "/settings", Native: "settings"},
		{Pattern: "/settings/{section}", Native: "settings"},
		{Pattern: "/tv/guide", Native: "guide"},
		{Pattern: "/tv/recordings", Native: "guide"},
		{Pattern: "/tv/watch/{key}", Native: "livePlayer"},
		{Pattern: "/watch/{item}", Native: "player"},
		{Pattern: "/play/{item}", Native: "player"},
		{Pattern: "/read/{item}", Native: "reader"},
		{Pattern: "/game/{item}", Native: "game"},
		{Pattern: "/you", Native: "profiles"},
		{Pattern: "/pair", Native: "pair"},
	}
}

func buildCatalogue() *Catalogue {
	c := &Catalogue{
		SchemaVersion: SchemaVersion, Revision: CatalogRevision,
		Components: buildComponents(), CardStyles: cardStylesOnce(), Sources: buildSources(), Conditions: buildConditions(),
		PageKinds: buildPageKinds(), Routes: buildRoutes(),
		Commands: []CommandSpec{
			{Op: "watchLater.add", Label: "Watch later", Description: "Adds the title to the profile's Watch later list.", Method: "POST", Path: "/api/v1/collections/watch-later/items", Body: `{"items":["{item}"]}`},
			{Op: "watched.set", Label: "Mark watched", Description: "Marks the title watched; for a show, every episode.", Method: "POST", Path: "/api/v1/items/{item}/progress", Body: `{"watched":true}`},
			{Op: "watched.clear", Label: "Mark unwatched", Description: "Forgets the profile's progress in the title; for a show, in every episode.", Method: "DELETE", Path: "/api/v1/items/{item}/progress"},
			{Op: "selectTab", Label: "Choose a tab", Description: "Asks for the page again with tab=<params.tab>, or shows the tab's content when the client already has it.", Method: "GET", Path: ""},
			{Op: "refresh", Label: "Refresh", Description: "Asks for the page again.", Method: "GET", Path: ""},
		},
		NativeScreens: append([]NativeScreenSpec(nil), nativeScreenSpecs...),
		NativeViews:   opts("guide|TV guide grid", "photoTimeline|Photo timeline", "photoMap|Photo map", "coverFlow|Album cover flow"),
		Icons:         append([]string(nil), Icons...),
		Placeholders: opts("{user}|The profile's name", "{server}|The server's name", "{greeting}|Good morning, afternoon or evening", "{library}|The page's library",
			"{title}|The page's title, or a source's first item", "{query}|What was searched for", "{each}|What a repeated section is repeated over", "{count}|How many items a source gave",
			"{genre}|The source's genre", "{studio}|The source's studio", "{tag}|The source's tag"),
		Repeats:     opts("libraries|Each library", "pinnedViews|Each saved view pinned to Home", "collections|Each collection"),
		FormFactors: FormFactors, Platforms: Platforms, ProfileKinds: ProfileKinds, LibraryKinds: LibraryKinds, MediaKinds: MediaKinds,
		PersonalKinds: opts("reorder|Move sections", "hide|Hide sections", "pin|Pin sections to the top", "density|Choose a density", "cardSize|Choose card sizes", "defaultTab|Choose the opening tab"),
		Placements:    opts("primary|In the bar", "more|Under More", "end|At the far end", "hidden|Not shown"),
		Spacing:       SpacingSteps,
		Limits: map[string]int{"maxPageBytes": MaxPageBytes, "maxSections": MaxSections, "maxDepth": MaxDepth, "maxChildren": MaxChildren, "maxVariants": MaxVariants,
			"maxNavItems": MaxNavItems, "maxPages": MaxPages, "maxIdLength": MaxIDLength, "maxPersonalBytes": MaxPersonalBytes, "maxSourceLimit": 500},
		Profiles: map[string]*Layout{}, Layouts: Layouts, NavStyles: NavStyles,
	}
	for _, ff := range FormFactors {
		c.Profiles[ff] = ProfileFor(ff, "")
	}
	c.Profiles["tv-4k"] = ProfileFor(FormTV, "4k")
	return c
}

// nativeScreenSpecs are the native screens, in the catalogue's order. Menu marks
// those a navigation entry may open: a place of its own that needs nothing filled
// in. The player, the reader, a game, a channel and the per-title sheets need a
// title (or a channel), so they are opened by actions, never from the menu.
var nativeScreenSpecs = []NativeScreenSpec{
	{Screen: "settings", Menu: true, Label: "Settings", Route: "/settings", Icon: "settings", Description: "The app's own Settings. Always in the navigation; never an SDUI page."},
	{Screen: "guide", Menu: true, Label: "TV guide", Route: "/tv/guide", Icon: "guide", Description: "The guide's grid of channels and times."},
	{Screen: "profiles", Menu: true, Label: "Who's watching", Route: "/you", Icon: "user", Description: "The profile's own page and the profile switcher."},
	{Screen: "downloads", Menu: true, Label: "Downloads", Route: "/downloads", Icon: "download", Description: "What is kept on this device."},
	{Screen: "remote", Menu: true, Label: "Remote", Route: "/remote", Icon: "devices", Description: "Remote control of another device."},
	{Screen: "nowPlaying", Menu: true, Label: "Now playing", Route: "/now-playing", Icon: "music", Description: "The music player."},
	{Screen: "pair", Menu: true, Label: "Pair a device", Route: "/pair", Icon: "devices", Description: "Sign another device in."},
	{Screen: "collections", Menu: true, Label: "Collections", Route: "/collections", Icon: "collection", Description: "The app's own collections manager, where it has one; else the collections page."},
	{Screen: "player", Label: "Player", Route: "/watch/{item}", Icon: "play", Description: "The video player."},
	{Screen: "livePlayer", Label: "Live TV player", Route: "/tv/watch/{key}", Icon: "live", Description: "A channel, playing."},
	{Screen: "reader", Label: "Reader", Route: "/read/{item}", Icon: "book", Description: "The book and comic reader."},
	{Screen: "game", Label: "Game", Route: "/game/{item}", Icon: "game", Description: "A game's page and player."},
	{Screen: "download", Label: "Download", Route: "", Icon: "download", Description: "Keep a copy of a title on this device (action params: item)."},
	{Screen: "share", Label: "Share", Route: "", Icon: "share", Description: "The share sheet for a title (action params: item)."},
	{Screen: "playOn", Label: "Play on…", Route: "", Icon: "devices", Description: "The device picker for a title (action params: item)."},
	{Screen: "watchParty", Label: "Watch party", Route: "", Icon: "users", Description: "Start a watch party for a title (action params: item)."},
}

// menuScreens are the native screens a navigation entry may open.
func menuScreens() []string {
	var out []string
	for _, n := range nativeScreenSpecs {
		if n.Menu {
			out = append(out, n.Screen)
		}
	}
	return out
}
