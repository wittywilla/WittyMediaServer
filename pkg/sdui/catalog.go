// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"encoding/json"
	"sync"
)

// CatalogRevision counts changes to the catalogue within one SchemaVersion.
// Revision 1 was the first, unreleased, catalogue of low-level nodes; 2 is the
// media components described here. A client sends the revision it was built
// against and is never sent a component newer than that.
const CatalogRevision = 2

// Property types, as an editor needs them to build a form.
const (
	PropString   = "string"   // one line of text
	PropText     = "text"     // text that may hold placeholders ({user}, {library}, ...)
	PropMarkdown = "markdown" // the small Markdown subset (see docs/SDUI.md)
	PropInt      = "int"
	PropBool     = "bool"
	PropEnum     = "enum"     // one of Options
	PropEnumList = "enumList" // an ordered list of Options, none twice
	PropIcon     = "icon"     // one of Icons
	PropRoute    = "route"    // a place in the app: /library/3, /page/family-night
	PropImage    = "image"    // a picture on this server: a path beginning /api/
	PropURL      = "url"      // an http(s) address, opened outside the app
	PropSection  = "section"  // the ID of another section on the page
	PropLibrary  = "library"  // a library's ID, or $library / $each
	PropRef      = "ref"      // the ID of a collection, saved view or title, or a $ variable
	PropScope    = "scope"    // a theme scope
	// The rest are only on components the server makes (Card, Track).
	PropNumber = "number"
	PropObject = "object"
	PropList   = "list"
)

// PropSpec describes one property of a component or parameter of a source,
// with everything an editor needs to offer it and a validator to check it.
type PropSpec struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Label string `json:"label"`
	Help  string `json:"help,omitempty"`
	// Default is what a renderer and the server assume when it is left out.
	Default  any  `json:"default,omitempty"`
	Required bool `json:"required,omitempty"`
	// Options are the choices of an enum or enumList.
	Options []EnumOption `json:"options,omitempty"`
	// Min and Max bound an int; MaxLength a string.
	Min       *int `json:"min,omitempty"`
	Max       *int `json:"max,omitempty"`
	MaxLength int  `json:"maxLength,omitempty"`
	// FormFactors lists where the property has an effect; empty means all.
	FormFactors []string `json:"formFactors,omitempty"`
	// Since is the catalogue revision the property appeared in.
	Since int `json:"since"`
}

// ComponentSpec describes one component.
type ComponentSpec struct {
	Type        string `json:"type"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	// Category groups the editor's palette: media, detail, navigation, text,
	// layout, structure.
	Category string `json:"category"`
	// Since is the catalogue revision the component appeared in.
	Since int `json:"since"`
	// Required components are drawn by every renderer. The others may be left
	// out by a renderer, which then says so, and is sent Fallback instead.
	Required bool `json:"required"`
	// Fallback names what a client that can't draw the component is sent, and
	// FallbackNote says how it is made from the component.
	Fallback     string `json:"fallback,omitempty"`
	FallbackNote string `json:"fallbackNote,omitempty"`
	// Authored is false for components only the server makes (Page, Card,
	// Track): they aren't in an editor's palette.
	Authored bool `json:"authored"`
	// Data is "none", "optional" or "required": whether the component takes
	// a data source. Accepts lists the kinds of source it takes; empty: any.
	Data    string   `json:"data"`
	Accepts []string `json:"accepts,omitempty"`
	// Container components hold other sections; Children lists which types,
	// empty meaning any authored component.
	Container bool     `json:"container"`
	Children  []string `json:"children,omitempty"`
	// Focus is "none", "item" (the node takes focus itself), "group" (its
	// children take focus and it is their group) or "stop" (it is one stop in
	// the page's focus order and moves focus among its own parts).
	Focus string `json:"focus"`
	// FormFactors lists where the component is shown; empty means everywhere.
	FormFactors []string   `json:"formFactors,omitempty"`
	Props       []PropSpec `json:"props,omitempty"`
	// Renders says in words how each form factor draws it.
	Renders map[string]string `json:"renders,omitempty"`
	// States says what is drawn while loading, when empty and on an error.
	States map[string]string `json:"states,omitempty"`
	// A11y says how the accessibility role and label are derived.
	A11y string `json:"a11y,omitempty"`
	// Example is a section using the component.
	Example json.RawMessage `json:"example,omitempty"`
}

// CardStyleSpec describes one way a Card is drawn.
type CardStyleSpec struct {
	Style       string `json:"style"`
	Label       string `json:"label"`
	Description string `json:"description"`
	// Shape is the basic shape every renderer knows: poster, landscape,
	// square, circle or chip. A renderer that doesn't know Style draws Shape.
	Shape  string `json:"shape"`
	Aspect string `json:"aspect"`
	// Image is the order in which an item's pictures are tried.
	Image []string `json:"image,omitempty"`
	// Shows lists the card props the style displays, in reading order; it is
	// also the order of the accessibility label.
	Shows []string `json:"shows"`
	// TitleLines and SubtitleLines are the most lines each may take before
	// it is cut with an ellipsis.
	TitleLines    int `json:"titleLines"`
	SubtitleLines int `json:"subtitleLines"`
	// Kinds are the item kinds the style is the natural one for.
	Kinds []string `json:"kinds,omitempty"`
}

// SourceSpec describes one data source.
type SourceSpec struct {
	Type        string `json:"type"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	// Category groups the editor's list: personal, library, discover, live,
	// music, context, structure.
	Category string     `json:"category"`
	Params   []PropSpec `json:"params,omitempty"`
	// Yields are the item kinds it gives; CardStyle the style "auto" means.
	Yields    []string `json:"yields"`
	CardStyle string   `json:"cardStyle"`
	// PageKinds lists the kinds of page it can be used on; empty means any.
	PageKinds []string `json:"pageKinds,omitempty"`
	// Paged sources give more through a section's cursor URL.
	Paged bool `json:"paged"`
	// SeeAll is the route "See all" opens, with {param} placeholders; "" when
	// the source has no page of its own.
	SeeAll string `json:"seeAll,omitempty"`
	// Title is a good default title for a section using it.
	Title   string          `json:"title,omitempty"`
	Since   int             `json:"since"`
	Example json.RawMessage `json:"example,omitempty"`
}

// ConditionSpec describes one field of Condition.
type ConditionSpec struct {
	Field string `json:"field"`
	PropSpec
}

// PageKindSpec describes a kind of page.
type PageKindSpec struct {
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Description string `json:"description"`
	// Route is the page's place in the app, and Params the query parameters
	// GET /sdui/pages/{kind} takes for it.
	Route  string   `json:"route"`
	Params []string `json:"params,omitempty"`
	// Variables are the $names a source on such a page may use.
	Variables []string `json:"variables,omitempty"`
	// Match lists the Match fields that narrow such a page.
	Match []string `json:"match,omitempty"`
}

// RouteSpec maps a route in the app to what shows it: an SDUI page (Page, with
// Params taken from the route) or a screen the app draws itself (Native).
type RouteSpec struct {
	Pattern string            `json:"pattern"`
	Page    string            `json:"page,omitempty"`
	Params  map[string]string `json:"params,omitempty"`
	Native  string            `json:"native,omitempty"`
}

// CommandSpec describes one thing a "command" action can ask for.
type CommandSpec struct {
	Op          string `json:"op"`
	Label       string `json:"label"`
	Description string `json:"description"`
	// Method and Path are the request that carries it out; {item} is the
	// action's item.
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   string `json:"body,omitempty"`
}

// NativeScreenSpec describes a screen every app draws itself.
type NativeScreenSpec struct {
	Screen      string `json:"screen"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Route       string `json:"route"`
	Icon        string `json:"icon"`
}

// Catalogue is everything an editor or a renderer needs to know about the
// schema, as data: GET /api/v1/sdui/catalog answers it.
type Catalogue struct {
	SchemaVersion int                `json:"schemaVersion"`
	Revision      int                `json:"revision"`
	Components    []ComponentSpec    `json:"components"`
	CardStyles    []CardStyleSpec    `json:"cardStyles"`
	Sources       []SourceSpec       `json:"sources"`
	Conditions    []ConditionSpec    `json:"conditions"`
	PageKinds     []PageKindSpec     `json:"pageKinds"`
	Routes        []RouteSpec        `json:"routes"`
	Commands      []CommandSpec      `json:"commands"`
	NativeScreens []NativeScreenSpec `json:"nativeScreens"`
	NativeViews   []EnumOption       `json:"nativeViews"`
	Icons         []string           `json:"icons"`
	Placeholders  []EnumOption       `json:"placeholders"`
	Repeats       []EnumOption       `json:"repeats"`
	FormFactors   []string           `json:"formFactors"`
	Platforms     []string           `json:"platforms"`
	ProfileKinds  []string           `json:"profileKinds"`
	LibraryKinds  []string           `json:"libraryKinds"`
	MediaKinds    []string           `json:"mediaKinds"`
	PersonalKinds []EnumOption       `json:"personalKinds"`
	Placements    []EnumOption       `json:"placements"`
	Spacing       []string           `json:"spacing"`
	Limits        map[string]int     `json:"limits"`
	// Profiles are the device profiles, so an editor can preview a page's
	// budgets and a renderer can lay out a lazily loaded page of cards.
	Profiles map[string]*Layout `json:"profiles"`
}

var catalogueOnce = sync.OnceValue(buildCatalogue)

// Catalog returns the catalogue. It is shared: don't change it.
func Catalog() *Catalogue { return catalogueOnce() }

// Component returns the spec of a component type.
func Component(t string) (*ComponentSpec, bool) {
	c := componentIndex()[t]
	return c, c != nil
}

// SourceType returns the spec of a data source type.
func SourceType(t string) (*SourceSpec, bool) {
	s := sourceIndex()[t]
	return s, s != nil
}

// CardStyle returns the spec of a card style.
func CardStyle(s string) (*CardStyleSpec, bool) {
	c := styleIndex()[s]
	return c, c != nil
}

var (
	componentIndex = sync.OnceValue(func() map[string]*ComponentSpec {
		m := map[string]*ComponentSpec{}
		c := Catalog()
		for i := range c.Components {
			m[c.Components[i].Type] = &c.Components[i]
		}
		return m
	})
	sourceIndex = sync.OnceValue(func() map[string]*SourceSpec {
		m := map[string]*SourceSpec{}
		c := Catalog()
		for i := range c.Sources {
			m[c.Sources[i].Type] = &c.Sources[i]
		}
		return m
	})
	styleIndex = sync.OnceValue(func() map[string]*CardStyleSpec {
		m := map[string]*CardStyleSpec{}
		c := Catalog()
		for i := range c.CardStyles {
			m[c.CardStyles[i].Style] = &c.CardStyles[i]
		}
		return m
	})
)

// prop returns the spec of one property of a component.
func (c *ComponentSpec) prop(name string) *PropSpec {
	for i := range c.Props {
		if c.Props[i].Name == name {
			return &c.Props[i]
		}
	}
	return nil
}

func (s *SourceSpec) param(name string) *PropSpec {
	for i := range s.Params {
		if s.Params[i].Name == name {
			return &s.Params[i]
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
