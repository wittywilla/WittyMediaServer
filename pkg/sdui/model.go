// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"strconv"
	"time"
)

// SchemaVersion is the wire version of what a client is sent (Document,
// NavDocument, TopShelf). See docs/SDUI.md, "Versioning".
const SchemaVersion = 1

// ---------------------------------------------------------------------------
// What an administrator writes: pages made of sections.
// ---------------------------------------------------------------------------

// Page is one page of the app as it is authored: an ordered list of sections,
// each a component from the catalogue, most of them filled from a data source.
// A page is rendered for one profile on one device into a Document.
type Page struct {
	// ID names the page: lower-case letters, digits, - and _. A page with the
	// ID of a built-in page replaces it.
	ID string `json:"id"`
	// Kind says what the page is for, which decides when the app opens it and
	// what its sources may refer to ($library on a library page, $item on a
	// detail page). See PageKinds.
	Kind string `json:"kind"`
	// Title is shown in the page's heading and the window title. It may hold
	// placeholders ({library}, {title}, {query}, {user}).
	Title string `json:"title"`
	// Icon is one of Icons, for a navigation entry made from the page.
	Icon string `json:"icon,omitempty"`
	// Description is for whoever edits the page; clients never see it.
	Description string `json:"description,omitempty"`
	// Match narrows a library or detail page to one library, one kind of
	// library or one kind of media. The most specific match wins.
	Match *Match `json:"match,omitempty"`
	// Scope is the theme scope the page belongs to: a module id (wittymusic)
	// or another name with colours under theme.modules. The whole page takes
	// that scope's colour overrides.
	Scope string `json:"scope,omitempty"`
	// Sections are the page, top to bottom.
	Sections []Section `json:"sections"`
	// Variants replace Sections for the first variant whose condition holds:
	// a kids home, a television home.
	Variants []Variant `json:"variants,omitempty"`
	// Personal says what a profile may change about the page for itself. nil:
	// everything in PersonalKinds.
	Personal *PersonalPolicy `json:"personal,omitempty"`
	// TTLSeconds suggests how long a client may show the page before asking
	// again. 0: the default for the page's kind.
	TTLSeconds int `json:"ttlSeconds,omitempty"`
	// Regions are what the page puts into the shell's own regions while it, or its theme scope, is
	// showing: the status bar's items and the system dock's actions (docs/SDUI.md, "Shell regions").
	Regions *PageRegions `json:"regions,omitempty"`
}

// PageRegions is a page's contribution to the shell: the content of the regions that take any.
// A page that supplies one turns that region on for as long as the page shows, unless the
// profile's own placements hide it.
type PageRegions struct {
	// StatusBar is what the status bar shows besides what the client knows (the clock, the
	// controllers' batteries): the profile's name and a line, the server's name.
	StatusBar []StatusItem `json:"statusBar,omitempty"`
	// SystemDock is the round dock of the screen's destinations.
	SystemDock []DockEntry `json:"systemDock,omitempty"`
}

// Status item kinds: "profile" (the profile's name, with Text as the line under it), "server" (the
// server's name, drawn from the brand) and "text" (Text alone).
var StatusKinds = []string{"profile", "server", "text"}

// StatusItem is one thing in the status bar.
type StatusItem struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
}

// DockEntry is one destination of the system dock: a round button, the current one labelled.
type DockEntry struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Icon  string `json:"icon"`
	// Route is the place it opens; As "layer" shows that page over the screen.
	Route string `json:"route"`
	As    string `json:"as,omitempty"`
}

// Match narrows a page. Zero fields match anything.
type Match struct {
	// Library is one library's ID.
	Library int64 `json:"library,omitempty"`
	// LibraryKind is a kind of library: movies, shows, anime, music, ...
	LibraryKind string `json:"libraryKind,omitempty"`
	// MediaKind is a kind of title on a detail page: movie, show, episode,
	// album, book, game, collection, ...
	MediaKind string `json:"mediaKind,omitempty"`
}

// Variant is another list of sections for the same page, used when its
// condition holds. Variants are tried in order; the first that holds wins.
type Variant struct {
	ID       string     `json:"id"`
	Label    string     `json:"label,omitempty"`
	When     *Condition `json:"when"`
	Sections []Section  `json:"sections"`
}

// PersonalPolicy lists what profiles may change: any of PersonalKinds. An
// empty list means nothing.
type PersonalPolicy struct {
	Allow []string `json:"allow"`
}

// Section is one component on a page.
type Section struct {
	// ID is unique on the page (across variants too): it is how a profile's
	// own order refers to the section and how a client asks for more of it.
	ID string `json:"id"`
	// Component is a type from the catalogue: Shelf, Hero, Grid, ...
	Component string `json:"component"`
	// Props are the component's properties, as the catalogue describes them.
	Props map[string]any `json:"props,omitempty"`
	// Source says where the component's items come from.
	Source Source `json:"source,omitempty"`
	// When must hold for the section to be shown.
	When *Condition `json:"when,omitempty"`
	// Repeat makes one copy of the section for each library, pinned view or
	// collection, with $each standing for it in Source and {each} in titles.
	Repeat *Repeat `json:"repeat,omitempty"`
	// Scope is a theme scope for this section alone (see Page.Scope).
	Scope string `json:"scope,omitempty"`
	// Locked keeps profiles from hiding or moving the section.
	Locked bool `json:"locked,omitempty"`
	// Children are the sections inside a layout component (Stack, Split, Tab).
	Children []Section `json:"children,omitempty"`
}

// Repeat makes a section once per thing.
type Repeat struct {
	// Over is "libraries", "pinnedViews", "collections", or the series or authors of
	// the page's books library: "bookSeries", "bookAuthors"; or the parts of a person's work on
	// their page: "personParts".
	Over string `json:"over"`
	// Kinds keeps only libraries of these kinds (Over "libraries").
	Kinds []string `json:"kinds,omitempty"`
	// Limit is the most copies made; 0 is the device's limit.
	Limit int `json:"limit,omitempty"`
}

// Source is a data source and its parameters, written flat:
// {"type": "recentlyAdded", "library": 3, "limit": 20}. The catalogue lists
// every type and its parameters. A string parameter beginning with $ stands
// for something the page was opened with ($library, $item, $show, $query) or
// for the thing a repeated section is repeated over ($each).
type Source map[string]any

// Type is the source's type, "" when the section has no source.
func (s Source) Type() string {
	t, _ := s["type"].(string)
	return t
}

// Str, Int and Bool read a parameter, with "" , 0 and false for one that isn't
// there or isn't of that type.
func (s Source) Str(k string) string {
	switch v := s[k].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	}
	return ""
}

func (s Source) Int(k string) int {
	switch v := s[k].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}

func (s Source) Bool(k string) bool {
	b, _ := s[k].(bool)
	return b
}

// Condition says when a section, variant or navigation entry is shown. Every
// field that is set must hold; All, Any and Not combine conditions. The zero
// Condition always holds.
type Condition struct {
	All []Condition `json:"all,omitempty"`
	Any []Condition `json:"any,omitempty"`
	Not *Condition  `json:"not,omitempty"`
	// FormFactor: the device is one of these (tv, phone, tablet, desktop, web).
	FormFactor []string `json:"formFactor,omitempty"`
	// Platform: the app is one of these (web, tvos, ios, android, androidtv, desktop).
	Platform []string `json:"platform,omitempty"`
	// ProfileKind: the profile is one of these (adult, kids, guest).
	ProfileKind []string `json:"profileKind,omitempty"`
	// Admin: the profile is (true) or isn't (false) an administrator.
	Admin *bool `json:"admin,omitempty"`
	// Feature: the server has this feature (/capabilities features).
	Feature string `json:"feature,omitempty"`
	// Module: this module is installed and on.
	Module string `json:"module,omitempty"`
	// Can: the profile may use this (games, recordings, downloads, sharing,
	// comments, syncplay, remoteControl, remote, liveTv, editLayout): its
	// parental feature switches.
	Can string `json:"can,omitempty"`
	// HasLibrary: the profile can open at least one library of this kind.
	HasLibrary string `json:"hasLibrary,omitempty"`
	// LibraryKind: the page's own library is of this kind (library pages).
	LibraryKind string `json:"libraryKind,omitempty"`
	// Capability: the client said it has this (capabilities=hdr,touch).
	Capability string `json:"capability,omitempty"`
	// Flag: something the server knows is so: setupIncomplete, scanning,
	// librariesEmpty, hasQuery, screenTimeUp.
	Flag string `json:"flag,omitempty"`
	// MinWidth and MaxWidth bound the window's width in points, inclusive.
	// They pass when the client didn't say how wide it is.
	MinWidth int `json:"minWidth,omitempty"`
	MaxWidth int `json:"maxWidth,omitempty"`
	// TimeFrom and TimeTo bound the time of day on the server, "HH:MM",
	// from inclusive and to exclusive; a range may pass midnight.
	TimeFrom string `json:"timeFrom,omitempty"`
	TimeTo   string `json:"timeTo,omitempty"`
	// Weekday: today is one of these (mon, tue, wed, thu, fri, sat, sun).
	Weekday []string `json:"weekday,omitempty"`
	// Locale: the client's language is this or a kind of it (en matches en-GB).
	Locale string `json:"locale,omitempty"`
}

// Navigation is the app's main menu as authored: the tab bar of a phone, the
// sidebar of the web app, the top menu of a television.
type Navigation struct {
	Items []NavItem `json:"items"`
}

// NavItem is one entry of the main menu.
type NavItem struct {
	ID string `json:"id"`
	// Label and Icon may be left out for a page (its title and icon are used)
	// and for the libraries (each library's name and its kind's icon).
	Label string `json:"label,omitempty"`
	Icon  string `json:"icon,omitempty"`
	// Type is "page" (Page names it), "libraries" (one entry per library the
	// profile can open), "library" (Library is its ID), "native" (Screen is a
	// screen the app draws itself: settings, guide, profiles, downloads,
	// remote) or "route" (Route is a place in the app).
	Type    string `json:"type"`
	Page    string `json:"page,omitempty"`
	Library int64  `json:"library,omitempty"`
	Screen  string `json:"screen,omitempty"`
	Route   string `json:"route,omitempty"`
	// When must hold for the entry to be shown. The settings entry has none.
	When *Condition `json:"when,omitempty"`
	// Placement says where the entry sits on each form factor: "primary" (the
	// bar itself), "more" (behind More or in the sidebar's lower group), "end"
	// (pinned to the far end) or "hidden". A form factor left out is primary.
	Placement map[string]string `json:"placement,omitempty"`
}

// Personal is what one profile changed about one page for itself.
type Personal struct {
	// Order is the profile's order of the page's top-level sections, by ID.
	// Sections it doesn't name keep their place after the ones it does.
	Order []string `json:"order,omitempty"`
	// Hidden sections aren't shown. Pinned ones come first, in Pinned's order.
	Hidden []string `json:"hidden,omitempty"`
	Pinned []string `json:"pinned,omitempty"`
	// Density is compact, comfortable or spacious; "" follows the device.
	Density string `json:"density,omitempty"`
	// CardSize is s, m or l for a section, by ID.
	CardSize map[string]string `json:"cardSize,omitempty"`
	// DefaultTab is the tab a Tabs section opens on, by the Tabs section's ID.
	DefaultTab map[string]string `json:"defaultTab,omitempty"`
}

// IsZero reports whether the profile changed nothing.
func (p *Personal) IsZero() bool {
	return p == nil || (len(p.Order) == 0 && len(p.Hidden) == 0 && len(p.Pinned) == 0 && p.Density == "" && len(p.CardSize) == 0 && len(p.DefaultTab) == 0)
}

// ---------------------------------------------------------------------------
// What a client is sent: a document, a tree of nodes.
// ---------------------------------------------------------------------------

// Document is one page rendered for one profile on one device.
type Document struct {
	// Version is SchemaVersion.
	Version int `json:"version"`
	// Catalog is the catalogue revision the tree was written for: the
	// client's own when it is older than the server's.
	Catalog int `json:"catalog"`
	// Page says which page this is.
	Page PageInfo `json:"page"`
	// Layout is the device profile the page was fitted to.
	Layout *Layout `json:"layout,omitempty"`
	// Theme is the page's theme scope and the colours it overrides.
	Theme *ThemeScope `json:"theme,omitempty"`
	// Root is the tree. Its type is Page.
	Root Node `json:"root"`
	// Personal says what the profile may change about this page and what it
	// has changed, so an app can offer "Customise" without knowing the page.
	Personal *PersonalInfo `json:"personal,omitempty"`
	// Regions is what the page puts into the shell's regions (Page.Regions), with the regions it
	// turns on in Shell.
	Regions *PageRegions `json:"regions,omitempty"`
	// ScreenTime is present when the profile's screen time is over: nothing
	// in the page offers to play, and this says why and until when.
	ScreenTime *ScreenTime `json:"screenTime,omitempty"`
	Meta       Meta        `json:"meta,omitzero"`
}

// ScreenTime says a profile's time for playing is over.
type ScreenTime struct {
	// Reason is the rule that ended it; Message says so in words to show.
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message"`
	// NextAt is when there is time again, Unix seconds; 0 when not known.
	NextAt int64 `json:"nextAt,omitempty"`
}

// PageInfo identifies a rendered page.
type PageInfo struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Icon  string `json:"icon,omitempty"`
	// Route is the page's place in the app: what a deep link to it holds.
	Route string `json:"route,omitempty"`
	// Variant is the ID of the variant used, if one was.
	Variant string `json:"variant,omitempty"`
	// Params are what the page was opened with (library, item, q, ...).
	Params map[string]string `json:"params,omitempty"`
}

// ThemeScope is a theme scope and its colour overrides: token → colour.
type ThemeScope struct {
	Scope  string            `json:"scope"`
	Tokens map[string]string `json:"tokens,omitempty"`
}

// PersonalInfo is the profile's own arrangement of a page.
type PersonalInfo struct {
	// Editable is false when the profile may not change layouts at all.
	Editable bool `json:"editable"`
	// Allow lists what may be changed (PersonalKinds).
	Allow []string `json:"allow,omitempty"`
	// Sections are the page's top-level sections in the profile's order,
	// hidden ones included, so they can be shown again.
	Sections []PersonalSection `json:"sections,omitempty"`
	// Density is the profile's choice, "" when it made none.
	Density string `json:"density,omitempty"`
	// URL is where the arrangement is read and written.
	URL string `json:"url,omitempty"`
}

// PersonalSection is one row of a "Customise" list.
type PersonalSection struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Hidden bool   `json:"hidden,omitempty"`
	Pinned bool   `json:"pinned,omitempty"`
	Locked bool   `json:"locked,omitempty"`
	// Empty: there is nothing to show in it now, so it isn't on the page.
	Empty bool `json:"empty,omitempty"`
}

// Node is one component in a rendered tree.
type Node struct {
	// Type is the component: see the catalogue. A client draws a type it
	// doesn't know as a plain column of its children.
	Type string `json:"type"`
	// ID is unique in the document and stable between renders: patches name
	// nodes by it.
	ID string `json:"id"`
	// Props are the component's properties, resolved.
	Props map[string]any `json:"props,omitempty"`
	// Children are in display order.
	Children []Node `json:"children,omitempty"`
	// Action is what activating the node does.
	Action *Action `json:"action,omitempty"`
	// Focus is the node's place in the focus order.
	Focus *Focus `json:"focus,omitempty"`
	// State is "" (ready), "deferred" (ask props.more.url for the items when
	// the node nears the screen), "empty" or "error".
	State string `json:"state,omitempty"`
	// Scope is the theme scope of the node and everything in it, and Tokens
	// the colours that scope overrides.
	Scope  string            `json:"scope,omitempty"`
	Tokens map[string]string `json:"tokens,omitempty"`
}

// Action is what a client does when a node is activated.
type Action struct {
	// Type is navigate, play, native, external, back or command.
	Type string `json:"type"`
	// To is a route in the app (navigate) or a native screen (native).
	To string `json:"to,omitempty"`
	// Item is the ID of the title to play (play) or act on (command).
	Item string `json:"item,omitempty"`
	// URL is a web address (external).
	URL string `json:"url,omitempty"`
	// Op is the command: see Commands.
	Op string `json:"op,omitempty"`
	// Params are extra, per type: {"resume": true} for play.
	Params map[string]any `json:"params,omitempty"`
	// As says how a navigate action's page is shown: "layer" over the current screen (a sheet, a
	// quick menu) instead of replacing it, which a client that has no layers does as a page.
	// Left out: the page replaces the screen.
	As string `json:"as,omitempty"`
}

// PresentLayer is Action.As for a page shown over the screen.
const PresentLayer = "layer"

// Action types.
const (
	ActNavigate = "navigate"
	ActPlay     = "play"
	ActNative   = "native"
	ActExternal = "external"
	ActBack     = "back"
	ActCommand  = "command"
)

// Focus places a node in the focus order of a device driven by a remote or a
// keyboard. Render stamps it on every node a person can act on.
type Focus struct {
	// Order is the node's place in reading order, from 1, without gaps.
	Order int `json:"order"`
	// Group is the ID of the row, grid or bar the node is in. A client treats
	// a group as one focus section and remembers where focus was in it.
	Group string `json:"group,omitempty"`
	// Default marks the one node focus starts on.
	Default bool `json:"default,omitempty"`
}

// Meta is information about a rendered document that isn't part of the page.
type Meta struct {
	// GeneratedAt and ETag are stamped by Render and left out of the ETag.
	GeneratedAt time.Time `json:"generatedAt,omitzero"`
	ETag        string    `json:"etag,omitempty"`
	// Source is "builtin", "config" or "draft".
	Source string `json:"source,omitempty"`
	// ConfigRevision is the revision of the configuration rendered.
	ConfigRevision int64 `json:"configRevision,omitempty"`
	// TTLSeconds suggests how long to show the page before asking again.
	TTLSeconds int `json:"ttlSeconds,omitempty"`
}

// ---------------------------------------------------------------------------
// Items: what data sources give and cards show.
// ---------------------------------------------------------------------------

// Item is one thing a data source found, in the one shape every card style
// reads. A source fills what it knows; a card style shows what it has room
// for (see CardStyles).
type Item struct {
	// ID is unique among the items of one source, and stable.
	ID string `json:"id"`
	// Kind is movie, episode, show, season, video, disc, album, artist, track,
	// book, audiobook, podcast, game, photo, channel, recording, collection,
	// library, genre, view or person.
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	Overview string `json:"overview,omitempty"`
	Tagline  string `json:"tagline,omitempty"`
	Year     int    `json:"year,omitempty"`
	// Rating is the community score out of 10; ContentRating the certificate.
	Rating        float64  `json:"rating,omitempty"`
	ContentRating string   `json:"contentRating,omitempty"`
	RuntimeSec    float64  `json:"runtimeSec,omitempty"`
	Genres        []string `json:"genres,omitempty"`
	Studio        string   `json:"studio,omitempty"`
	// Progress is how far through it the profile is, 0 to 1; Watched that it
	// finished; Unwatched how many episodes of a show are left.
	Progress  float64 `json:"progress,omitempty"`
	Watched   bool    `json:"watched,omitempty"`
	Unwatched int     `json:"unwatched,omitempty"`
	// InWatchLater: the title is on the profile's Watch later list.
	InWatchLater bool `json:"inWatchLater,omitempty"`
	// ResumeSec is where Play picks up, 0 to start over.
	ResumeSec float64 `json:"resumeSec,omitempty"`
	// Season and Episode number an episode; ShowTitle names its show.
	Season    int    `json:"season,omitempty"`
	Episode   int    `json:"episode,omitempty"`
	ShowTitle string `json:"showTitle,omitempty"`
	ShowID    string `json:"showId,omitempty"`
	// Artist and Track are a song's; Count is how many things an album,
	// artist, library, collection or season holds.
	Artist string `json:"artist,omitempty"`
	Track  int    `json:"track,omitempty"`
	Count  int    `json:"count,omitempty"`
	// Channel and Programme open WittyTV's More info (GET /api/tv/{key}/programme) from a
	// recording: its channel's key and "<key>@<start>". Empty when the profile may not watch it.
	Channel   string `json:"channel,omitempty"`
	Programme string `json:"programme,omitempty"`
	// Number is a channel's number. Now and Next are what is on it.
	Number string     `json:"number,omitempty"`
	Now    *Programme `json:"now,omitempty"`
	Next   *Programme `json:"next,omitempty"`
	// Images are addresses on this server by kind: poster (2/3), backdrop
	// (16/9), square (1/1), logo. Render adds the width to ask for.
	Images map[string]string `json:"images,omitempty"`
	// Route is the item's place in the app; PlayID what to play, when it can
	// be played from a card.
	Route  string `json:"route,omitempty"`
	PlayID string `json:"playId,omitempty"`
	// Badges are short labels: 4K, HDR, NEW, LIVE.
	Badges []string `json:"badges,omitempty"`
	// Icon is one of Icons, for items without pictures (libraries, genres).
	Icon string `json:"icon,omitempty"`
	// Scope is the theme scope the item belongs to (wittytv for a channel).
	Scope string `json:"scope,omitempty"`
	// Can lists what DetailHeader may offer for the item: play, download,
	// share, watchLater, watched, playOn, watchParty.
	Can []string `json:"can,omitempty"`
	// NextID is the item Play should start for a show: its next episode.
	NextID string `json:"nextId,omitempty"`
	// Group is the part of a title's credits a person card is in: cast, guests, or a crew
	// department (CreditGroups). The card's subtitle is the part played or the job.
	Group string `json:"group,omitempty"`
	// Born, Died and KnownFor are a person's, as their page's facts say them: "3 February
	// 1970, Lisbon", "2021", "Acting".
	Born     string `json:"born,omitempty"`
	Died     string `json:"died,omitempty"`
	KnownFor string `json:"knownFor,omitempty"`
	// Action is what activating the item's row does instead of opening Route or playing PlayID: a
	// command (sign a device out, add a title to a list). Trailing is the text at the row's end
	// ("2 hours ago", "22 GB"). Control draws a switch, a choice, a field or a slider in the row:
	// see Control. All three are drawn by List rows (docs/SDUI.md, "Forms and controls").
	Action   *Action  `json:"action,omitempty"`
	Trailing string   `json:"trailing,omitempty"`
	Control  *Control `json:"control,omitempty"`
	// Actions are the buttons a Shelf's detail panel shows for the item while it has focus (Resume,
	// Play online, Options), first the A action. An item with none has its route as its A action.
	Actions []ItemAction `json:"actions,omitempty"`
}

// ItemAction is one button of an item's detail panel.
type ItemAction struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Glyph is the key the button is drawn with: a, b, x or y, the shared pad map (A selects, B goes
	// back, X and Y are the secondary and options keys).
	Glyph  string  `json:"glyph,omitempty"`
	Action *Action `json:"action"`
}

// Glyphs are the keys an ItemAction may carry.
var Glyphs = []string{"a", "b", "x", "y"}

// Control types.
const (
	ControlSwitch = "switch" // on or off; the row's Action is what flipping it does
	ControlSelect = "select" // one of Options
	ControlField  = "field"  // a line (or lines) of text, or a number
	ControlSlider = "slider" // a number from Min to Max in steps of Step
)

// Control is a control drawn in a list row, so a page can hold settings-like rows: Settings itself stays
// hand-built, but a page the server serves (the profile's games privacy, a smart playlist's rules) is
// made of these. A switch says its state in Value (a bool) and its row's Action is what flipping it
// does: the server has already put the new value in the action's params, so a client never works out
// the opposite. A select, field or slider carries the Action to run with the chosen value added to its
// params as "value"; inside a Form the control has no Action of its own and the Form's is run with every
// named control's value (see the Form component).
type Control struct {
	Type string `json:"type"`
	// Name is the control's name inside a Form: the key its value goes under in the Form's command.
	Name string `json:"name,omitempty"`
	// Value is a bool (switch), a string (select, field) or a number (slider).
	Value any `json:"value,omitempty"`
	// Options are a select's choices.
	Options []EnumOption `json:"options,omitempty"`
	// Min, Max, Step and Unit are a slider's; Unit is a suffix ("%", " min").
	Min  float64 `json:"min,omitempty"`
	Max  float64 `json:"max,omitempty"`
	Step float64 `json:"step,omitempty"`
	Unit string  `json:"unit,omitempty"`
	// Placeholder, MaxLength, Multiline and Input (text, number, url, search) are a field's.
	Placeholder string `json:"placeholder,omitempty"`
	MaxLength   int    `json:"maxLength,omitempty"`
	Multiline   bool   `json:"multiline,omitempty"`
	Input       string `json:"input,omitempty"`
	// Disabled draws it dimmed and leaves it out of focus; Why says why, in a line under it.
	Disabled bool   `json:"disabled,omitempty"`
	Why      string `json:"why,omitempty"`
}

// Programme is something on a channel.
type Programme struct {
	Title string `json:"title"`
	// Start and End are Unix seconds.
	Start int64 `json:"start,omitempty"`
	End   int64 `json:"end,omitempty"`
}

// Facet is one thing a FilterBar can filter or sort by, with its choices.
type Facet struct {
	ID      string       `json:"id"`
	Label   string       `json:"label"`
	Options []EnumOption `json:"options"`
}

// EnumOption is one choice of a property or facet.
type EnumOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
