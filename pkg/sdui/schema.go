// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import "time"

// SchemaVersion is the current wire version of Document. See the package
// documentation for the versioning policy.
const SchemaVersion = 1

// Document is one complete screen description. It is the unit that is
// authored, stored, rendered, diffed and sent to clients.
type Document struct {
	// Version is the schema version; must equal SchemaVersion.
	Version int `json:"version"`
	// Screen names the screen this document describes ("home", "library",
	// "detail", "player", "settings", or a module-defined name).
	Screen string `json:"screen"`
	// Theme optionally selects a theme and per-module token overrides.
	Theme *ThemeRef `json:"theme,omitempty"`
	// Root is the top of the component tree. Its Type must be Screen.
	Root Node `json:"root"`
	// Actions is the table of named actions that nodes refer to via Node.Action.
	Actions map[string]Action `json:"actions,omitempty"`
	// Layout says how the screen was fitted to the requesting device: its form
	// factor, safe area, type sizes, picture sizes and the limits applied.
	// Stamped by Render from RenderContext.Profile; never authored.
	Layout *Layout `json:"layout,omitempty"`
	// Meta carries non-structural information about the document.
	Meta Meta `json:"meta,omitzero"`
}

// Node is one component in the tree.
type Node struct {
	// Type is the component kind; see ComponentType and Catalog.
	Type ComponentType `json:"type"`
	// ID uniquely identifies the node within its document. It is the key
	// used by Diff/Apply, so it should be stable across renders.
	ID string `json:"id"`
	// Props are component-specific properties (title, text, src, …). Values
	// must be JSON-serialisable.
	Props map[string]any `json:"props,omitempty"`
	// Style holds layout and colour hints.
	Style *Style `json:"style,omitempty"`
	// Children are the nested components, in display order.
	Children []Node `json:"children,omitempty"`
	// Visibility, when set, must evaluate true for the node (and its subtree)
	// to be included in the rendered output. Removed by Render.
	Visibility *Condition `json:"visibility,omitempty"`
	// Bindings map a prop name to a data path that Render resolves against
	// RenderContext.Data. Removed by Render.
	Bindings map[string]Binding `json:"bindings,omitempty"`
	// Action is the name of an entry in Document.Actions to dispatch when
	// the node is activated (pressed, clicked, selected).
	Action string `json:"action,omitempty"`
	// Focus is the node's place in the focus engine (tvOS, a remote, a
	// keyboard). An author may set Default and the four guides; Render stamps
	// the rest on every interactive node. See Focus.
	Focus *Focus `json:"focus,omitempty"`
}

// Focus places a node in the focus engine. tvOS has no pointer: everything a
// person can act on must be focusable, focus must start somewhere, and moving
// it must always lead somewhere. Render guarantees the first two for every
// screen; the guides are for the places geometry alone gets wrong (jumping
// from a short row to a button that isn't straight above it).
type Focus struct {
	// Focusable is true on every node a person can act on. Stamped by Render.
	Focusable bool `json:"focusable,omitempty"`
	// Order is the node's place in the screen's focus order, from 1, in
	// reading order. It is deterministic: the same screen gives the same
	// numbers. Stamped by Render.
	Order int `json:"order,omitempty"`
	// Default marks the node focus starts on. At most one per screen; when no
	// author chose one, Render marks the first focusable node.
	Default bool `json:"default,omitempty"`
	// Group is the ID of the row or bar the node belongs to (its nearest
	// Shelf, PosterRow, Carousel, Grid, List, Tabs or Toolbar). A client
	// treats a group as one focus section and remembers the last focused
	// node in it. Stamped by Render.
	Group string `json:"group,omitempty"`
	// Up, Down, Left and Right are focus guides: the ID of the node focus
	// moves to in that direction, in place of the nearest one. Authored.
	Up    string `json:"up,omitempty"`
	Down  string `json:"down,omitempty"`
	Left  string `json:"left,omitempty"`
	Right string `json:"right,omitempty"`
}

// ComponentType identifies a kind of component.
type ComponentType string

// Component types understood by schema v1. Clients must treat unknown types
// as opaque containers: render the children, ignore the type.
const (
	Screen      ComponentType = "Screen"
	Stack       ComponentType = "Stack"
	Grid        ComponentType = "Grid"
	Shelf       ComponentType = "Shelf"
	HeroBanner  ComponentType = "HeroBanner"
	Card        ComponentType = "Card"
	PosterRow   ComponentType = "PosterRow"
	Text        ComponentType = "Text"
	Image       ComponentType = "Image"
	Button      ComponentType = "Button"
	Tabs        ComponentType = "Tabs"
	Tab         ComponentType = "Tab"
	List        ComponentType = "List"
	ListItem    ComponentType = "ListItem"
	Spacer      ComponentType = "Spacer"
	Divider     ComponentType = "Divider"
	Video       ComponentType = "Video"
	Badge       ComponentType = "Badge"
	ProgressBar ComponentType = "ProgressBar"
	Carousel    ComponentType = "Carousel"
	SearchBar   ComponentType = "SearchBar"
	Toolbar     ComponentType = "Toolbar"
	Modal       ComponentType = "Modal"
)

// ComponentSpec describes one component type: what it is for, whether and
// which children it accepts, and which props it needs.
type ComponentSpec struct {
	// Type is the component type being described.
	Type ComponentType `json:"type"`
	// Description is a one-line human description.
	Description string `json:"description"`
	// AllowsChildren is false for leaf components.
	AllowsChildren bool `json:"allowsChildren"`
	// AllowedChildren restricts the child types; nil means any type.
	AllowedChildren []ComponentType `json:"allowedChildren,omitempty"`
	// RequiredProps must be present (as a Prop or a Binding) on every node of
	// this type.
	RequiredProps []string `json:"requiredProps,omitempty"`
	// OptionalProps are the props a client is expected to understand.
	OptionalProps []string `json:"optionalProps,omitempty"`
	// Since is the catalogue revision the type first appeared in (see
	// CatalogRevision). A client says which revision it was built against,
	// and Render sends it Fallback in place of anything newer.
	Since int `json:"since"`
	// Fallback is the older type a client that doesn't know this one is sent
	// instead. Empty: Stack, which every client draws as a plain container.
	Fallback ComponentType `json:"fallback,omitempty"`
}

// CatalogRevision counts additions to the component catalogue within one
// SchemaVersion. Every type in the first release is revision 1.
const CatalogRevision = 1

// catalog is the v1 component catalogue, in display order.
var catalog = []ComponentSpec{
	{Type: Screen, Since: 1, Description: "Root of a screen; exactly one per document.", AllowsChildren: true,
		OptionalProps: []string{"title", "scroll"}},
	{Type: Stack, Since: 1, Description: "Vertical or horizontal stack of children.", AllowsChildren: true},
	{Type: Grid, Since: 1, Description: "Responsive grid of children; Style.Columns sets the column count.", AllowsChildren: true},
	{Type: Shelf, Since: 1, Description: "A titled, horizontally scrolling row of cards (the web client's Shelf/CardRow).", AllowsChildren: true,
		RequiredProps: []string{"title"}, OptionalProps: []string{"subtitle", "seeAll", "items", "cardShape", "itemAction", "more", "total"}},
	{Type: HeroBanner, Since: 1, Description: "Full-bleed featured banner with backdrop, title and call to action.", AllowsChildren: true,
		OptionalProps: []string{"title", "subtitle", "backdropUrl", "logoUrl", "items", "interval"}},
	{Type: Card, Since: 1, Description: "A poster/tile for one item.", AllowsChildren: true,
		AllowedChildren: []ComponentType{Badge, ProgressBar, Text, Image},
		OptionalProps:   []string{"title", "subtitle", "imageUrl", "image", "kind", "progress", "badges", "itemId", "module", "route"}},
	{Type: PosterRow, Since: 1, Description: "A titled row of poster Cards.", AllowsChildren: true,
		AllowedChildren: []ComponentType{Card},
		RequiredProps:   []string{"title"}, OptionalProps: []string{"subtitle", "seeAll", "items", "cardShape"}},
	{Type: Text, Since: 1, Description: "A run of text.", RequiredProps: []string{"text"},
		OptionalProps: []string{"variant", "maxLines"}},
	{Type: Image, Since: 1, Description: "A picture.", RequiredProps: []string{"src"},
		OptionalProps: []string{"alt", "fit"}},
	{Type: Button, Since: 1, Description: "A pressable control that dispatches its Action.", RequiredProps: []string{"label"},
		OptionalProps: []string{"icon", "variant", "disabled"}},
	{Type: Tabs, Since: 1, Description: "A tab strip; children are Tab nodes.", AllowsChildren: true,
		AllowedChildren: []ComponentType{Tab}, OptionalProps: []string{"selected"}},
	{Type: Tab, Since: 1, Description: "One tab: a label and its content.", AllowsChildren: true,
		RequiredProps: []string{"label"}, OptionalProps: []string{"icon"}},
	{Type: List, Since: 1, Description: "A vertical list; children are ListItem nodes.", AllowsChildren: true,
		AllowedChildren: []ComponentType{ListItem}, OptionalProps: []string{"items", "dense"}},
	{Type: ListItem, Since: 1, Description: "One row of a List.", AllowsChildren: true,
		AllowedChildren: []ComponentType{Badge, Text, Image, ProgressBar},
		RequiredProps:   []string{"title"}, OptionalProps: []string{"subtitle", "imageUrl", "icon", "trailing", "itemId"}},
	{Type: Spacer, Since: 1, Description: "Empty space; Style.Height/Width or the size prop set its extent.",
		OptionalProps: []string{"size"}},
	{Type: Divider, Since: 1, Description: "A hairline rule.", OptionalProps: []string{"inset"}},
	{Type: Video, Since: 1, Description: "An inline or full-screen video player.", RequiredProps: []string{"src"},
		OptionalProps: []string{"poster", "autoplay", "controls", "startAt", "itemId", "title"}},
	{Type: Badge, Since: 1, Description: "A small label (NEW, 4K, LIVE, …).", RequiredProps: []string{"text"},
		OptionalProps: []string{"variant"}},
	{Type: ProgressBar, Since: 1, Description: "A progress indicator; value is 0..1 unless max is given.", RequiredProps: []string{"value"},
		OptionalProps: []string{"max", "label"}},
	{Type: Carousel, Since: 1, Description: "Paged container that rotates through its children.", AllowsChildren: true,
		OptionalProps: []string{"interval", "autoplay", "indicators"}},
	{Type: SearchBar, Since: 1, Description: "A search field; Action fires on submit.",
		OptionalProps: []string{"placeholder", "query", "scope"}},
	{Type: Toolbar, Since: 1, Description: "A horizontal bar of controls.", AllowsChildren: true,
		AllowedChildren: []ComponentType{Button, Text, Spacer, SearchBar, Badge, Image}},
	{Type: Modal, Since: 1, Description: "A dialog shown over the screen, opened by an OpenModal action.", AllowsChildren: true,
		OptionalProps: []string{"title", "dismissible", "size"}},
}

// Catalog returns the v1 component catalogue. The returned slice is a copy
// and may be modified by the caller.
func Catalog() []ComponentSpec {
	out := make([]ComponentSpec, len(catalog))
	for i, s := range catalog {
		c := s
		c.AllowedChildren = append([]ComponentType(nil), s.AllowedChildren...)
		c.RequiredProps = append([]string(nil), s.RequiredProps...)
		c.OptionalProps = append([]string(nil), s.OptionalProps...)
		out[i] = c
	}
	return out
}

// Spec returns the ComponentSpec for t, and whether t is a known type.
func Spec(t ComponentType) (ComponentSpec, bool) {
	for _, s := range catalog {
		if s.Type == t {
			return s, true
		}
	}
	return ComponentSpec{}, false
}

// Style carries layout and colour hints for a node. Colour fields accept a
// token reference ("token:accent", naming a theme colour such as those in
// themes/*/theme.json) or a literal CSS colour; see ValidateColor.
type Style struct {
	// Direction is "row" or "column" for Stack-like containers.
	Direction string `json:"direction,omitempty"`
	// Gap is the space between children, in CSS pixels.
	Gap int `json:"gap,omitempty"`
	// Padding is a CSS padding shorthand ("16px", "8px 16px").
	Padding string `json:"padding,omitempty"`
	// Margin is a CSS margin shorthand.
	Margin string `json:"margin,omitempty"`
	// Align is the cross-axis alignment ("start", "center", "end", "stretch").
	Align string `json:"align,omitempty"`
	// Justify is the main-axis distribution ("start", "center", "end",
	// "between", "around").
	Justify string `json:"justify,omitempty"`
	// Width and Height are CSS lengths ("100%", "240px", "auto").
	Width  string `json:"width,omitempty"`
	Height string `json:"height,omitempty"`
	// Columns is the column count for Grid.
	Columns int `json:"columns,omitempty"`
	// AspectRatio is "w/h" ("2/3" poster, "16/9" wide, "1/1" album).
	AspectRatio string `json:"aspectRatio,omitempty"`

	// Background, Foreground, Accent and Border are colours.
	Background string `json:"background,omitempty"`
	Foreground string `json:"foreground,omitempty"`
	Accent     string `json:"accent,omitempty"`
	Border     string `json:"border,omitempty"`

	// Radius is the corner radius in CSS pixels; 0 means the theme default.
	Radius int `json:"radius,omitempty"`
	// Elevation is a shadow depth hint (0 = flat).
	Elevation int `json:"elevation,omitempty"`
	// Opacity is 0..1; 0 means unset (fully opaque).
	Opacity float64 `json:"opacity,omitempty"`
	// Hidden hides the node without removing it from the tree. Unlike
	// Visibility it is evaluated by the client, not by Render.
	Hidden bool `json:"hidden,omitempty"`
	// Module scopes the node to a module ("wittyplay", "wittytv", …) so that
	// per-module token overrides from the theme apply to it.
	Module string `json:"module,omitempty"`
	// Class is a free-form hook for client-side styling.
	Class string `json:"class,omitempty"`
	// Tokens are resolved token→colour overrides, filled in by Render from
	// the ThemeRef and RenderContext module overrides when Module matches.
	Tokens map[string]string `json:"tokens,omitempty"`
}

// ActionType identifies what an Action does.
type ActionType string

// Action types understood by schema v1.
const (
	// Navigate opens the screen or route named by Target.
	Navigate ActionType = "navigate"
	// Play starts playback of the item named by Target.
	Play ActionType = "play"
	// OpenModal shows the Modal node whose ID is Target.
	OpenModal ActionType = "openModal"
	// CloseModal dismisses the top-most modal.
	CloseModal ActionType = "closeModal"
	// Fetch requests the server resource at Target and replaces/patches the
	// screen with the response.
	Fetch ActionType = "fetch"
	// Toggle flips a boolean on the node whose ID is Target (Params.prop
	// names the prop).
	Toggle ActionType = "toggle"
	// Submit posts the form/field values to Target.
	Submit ActionType = "submit"
	// External opens Target in the platform browser.
	External ActionType = "external"
	// Back returns to the previous screen.
	Back ActionType = "back"
)

// Action is something a client does when a node is activated.
type Action struct {
	// Type is the kind of action.
	Type ActionType `json:"type"`
	// Target is the action's object: a route, item ID, node ID or URL,
	// depending on Type. Required for all types except CloseModal and Back.
	Target string `json:"target,omitempty"`
	// Params are extra, action-specific parameters.
	Params map[string]any `json:"params,omitempty"`
}

// Condition is a composable visibility predicate. Every set field must hold
// for the condition to be true (an implicit AND); All, Any and Not compose
// nested conditions. The zero Condition is always true.
type Condition struct {
	// All is true when every element is true (and when empty).
	All []Condition `json:"all,omitempty"`
	// Any is true when at least one element is true (false when empty).
	Any []Condition `json:"any,omitempty"`
	// Not inverts a nested condition.
	Not *Condition `json:"not,omitempty"`
	// Feature requires RenderContext.Features to contain the value.
	Feature string `json:"feature,omitempty"`
	// Role requires RenderContext.Role to equal the value.
	Role string `json:"role,omitempty"`
	// Platform requires RenderContext.Platform to equal the value
	// ("web", "tv", "ios", "android", …).
	Platform string `json:"platform,omitempty"`
	// FormFactor requires the requester's form factor (RenderContext.Profile)
	// to equal the value: "tv", "phone", "tablet", "desktop" or "web".
	FormFactor string `json:"formFactor,omitempty"`
	// Module requires RenderContext.Modules to contain the value.
	Module string `json:"module,omitempty"`
	// MinWidth/MaxWidth/MinHeight/MaxHeight bound the viewport, inclusive,
	// in CSS pixels. Zero means unbounded.
	MinWidth  int `json:"minWidth,omitempty"`
	MaxWidth  int `json:"maxWidth,omitempty"`
	MinHeight int `json:"minHeight,omitempty"`
	MaxHeight int `json:"maxHeight,omitempty"`
	// Locale requires RenderContext.Locale to have the value as a prefix
	// ("en" matches "en-GB").
	Locale string `json:"locale,omitempty"`
	// Flag requires RenderContext.Flags[value] to be true.
	Flag string `json:"flag,omitempty"`
	// When is reserved for a future expression language; it is carried but
	// currently evaluates to true.
	When string `json:"when,omitempty"`
}

// Binding connects a prop to a value in RenderContext.Data.
type Binding struct {
	// Path is a JSONPath-lite expression: "$" followed by ".ident" and "[n]"
	// segments, e.g. "$.user.name" or "$.items[0].title".
	Path string `json:"path"`
	// Default is used when the path does not resolve.
	Default any `json:"default,omitempty"`
	// Format is an optional verb applied to the resolved value: "upper",
	// "lower", "title", "duration" or "date:<Go layout>".
	Format string `json:"format,omitempty"`
}

// ThemeRef selects a theme and per-module colour overrides.
type ThemeRef struct {
	// ID is the theme id ("default", "midnight", …).
	ID string `json:"id"`
	// ModuleOverrides maps module id → token name → colour, e.g.
	// {"wittyplay": {"accent": "#3ad29f"}}.
	ModuleOverrides map[string]map[string]string `json:"moduleOverrides,omitempty"`
}

// Meta is non-structural information about a document.
type Meta struct {
	// Title and Description are shown by clients in window titles and
	// previews.
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	// GeneratedAt is stamped by Render.
	GeneratedAt time.Time `json:"generatedAt,omitzero"`
	// ETag is stamped by Render; see ETag.
	ETag string `json:"etag,omitempty"`
	// Source records where the document came from ("template:home",
	// "ui_config.json", …).
	Source string `json:"source,omitempty"`
	// TTLSeconds suggests how long a client may cache the rendered screen.
	TTLSeconds int `json:"ttlSeconds,omitempty"`
}
