// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package conformance is the SDUI conformance kit: fixtures that pin down,
// case by case, the tree the server sends for a page and the layout a
// renderer makes of it, and the reference implementation of both steps.
//
// A fixture is plain JSON (fixtures/*.json), read the same from Go, Swift,
// Kotlin and TypeScript:
//
//	input.page      the authored page
//	input.request   who asks, on what device, knowing which components
//	input.data      canned answers for the page's data sources
//	expected.document  the rendered tree the server sends
//	expected.layout    the normalised layout a renderer makes of that tree
//
// The server's side is input → expected.document (Run). A renderer's side is
// expected.document → expected.layout (Normalise): its test suite feeds the
// document to its own renderer, describes what the renderer built in the
// Normalised shape, and compares. README.md says how.
package conformance

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// Fixture is one conformance case.
type Fixture struct {
	// Kit is KitVersion; Schema and Catalog the versions the case is for.
	Kit     int `json:"kit"`
	Schema  int `json:"schemaVersion"`
	Catalog int `json:"catalogRevision"`
	// ID names the case and its file; Title and Checks say what it is about.
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Checks []string `json:"checks"`
	Input  Input    `json:"input"`
	// Expected is what a correct server and a correct renderer produce.
	Expected Expected `json:"expected"`
}

// Input is everything a render depends on.
type Input struct {
	Page    sdui.Page `json:"page"`
	Request Request   `json:"request"`
	Data    Data      `json:"data"`
	// ModuleUI are the SDUI files of the modules in use (docs/SDUI-MODULES.md):
	// their components are expanded and their sources asked like any other.
	// A renderer never sees them; they are here for the server's half.
	ModuleUI []sdui.ModuleUI `json:"moduleUI,omitempty"`
}

// Request is who asks and on what. Its fields are the query parameters of
// GET /api/v1/sdui/pages/{page} and what the server knows of the profile.
type Request struct {
	FormFactor  string   `json:"formFactor"`
	DeviceClass string   `json:"deviceClass,omitempty"`
	Platform    string   `json:"platform,omitempty"`
	Viewport    string   `json:"viewport,omitempty"` // "1440x900"
	Locale      string   `json:"locale,omitempty"`
	NativeViews []string `json:"nativeViews,omitempty"`
	// Capabilities are the client's (reducedMotion, touch, hdr).
	Capabilities []string `json:"capabilities,omitempty"`
	// Experience is the look the page is drawn with (docs/SDUI.md, "Experiences"); "" for none.
	Experience string `json:"experience,omitempty"`
	// Now is the server's clock, RFC 3339.
	Now string `json:"now"`
	// The profile: its kind, whether it administers, its name, the parental
	// feature switches that are off, the kinds of library it can open, and
	// its own arrangement of the page.
	ProfileKind    string         `json:"profileKind"`
	Admin          bool           `json:"admin,omitempty"`
	UserName       string         `json:"userName"`
	Cannot         []string       `json:"cannot,omitempty"`
	LibraryKinds   []string       `json:"libraryKinds,omitempty"`
	Personal       *sdui.Personal `json:"personal,omitempty"`
	CanPersonalise bool           `json:"canPersonalise"`
	// The server: its name, features, modules and flags, and the theme
	// scopes' colours.
	ServerName string                       `json:"serverName"`
	Features   []string                     `json:"features,omitempty"`
	Modules    []string                     `json:"modules,omitempty"`
	Flags      map[string]bool              `json:"flags,omitempty"`
	Scopes     map[string]map[string]string `json:"scopes,omitempty"`
	// Vars are what the page was opened with (library, item, show, query,
	// ...); Params its filter, sort and tab parameters.
	Vars   map[string]string `json:"vars,omitempty"`
	Params map[string]string `json:"params,omitempty"`
}

// Data is the canned content of the server: what each source answers.
type Data struct {
	// Sources are tried in order: the first whose Match is contained in the
	// (resolved) source a section asks for answers it. A source nothing
	// matches is empty.
	Sources []CannedSource `json:"sources,omitempty"`
	// Each lists what "repeat" goes over, by its "over".
	Each map[string][]CannedEach `json:"each,omitempty"`
	// Subject is the title a detail page is about.
	Subject *sdui.Item `json:"subject,omitempty"`
	// Seasons are a show's seasons; Facets a library's filter choices.
	Seasons []sdui.Item  `json:"seasons,omitempty"`
	Facets  []sdui.Facet `json:"facets,omitempty"`
}

// CannedSource is the answer to every source that Match describes.
type CannedSource struct {
	Match map[string]any `json:"match"`
	Items []sdui.Item    `json:"items"`
	// Total is how many there are in all when that is more than Items.
	Total int `json:"total,omitempty"`
	// Fails makes the source answer with an error.
	Fails bool `json:"fails,omitempty"`
	// Selected is the season an "episodes" source chose.
	Selected string `json:"selected,omitempty"`
	// Reason is why the source chose what it did ("Because you watch Sci-Fi").
	Reason string `json:"reason,omitempty"`
}

// CannedEach is one thing a repeat goes over.
type CannedEach struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Kind  string `json:"kind,omitempty"`
	// ID ends the copy's ID in place of Key (a series' name is no ID).
	ID string `json:"id,omitempty"`
}

// Expected is the two halves of a case's answer.
type Expected struct {
	// NotFound is true when the page can't be shown (its subject is missing):
	// the server answers 404 and there is no document.
	NotFound bool            `json:"notFound,omitempty"`
	Document *sdui.Document  `json:"document,omitempty"`
	Layout   *Normalised     `json:"layout,omitempty"`
	More     []SectionAnswer `json:"more,omitempty"`
}

// SectionAnswer is what a section's cursor URL answers: the next page of a
// row, a deferred row's first page, another season.
type SectionAnswer struct {
	Section string            `json:"section"`
	Cursor  int               `json:"cursor"`
	Season  string            `json:"season,omitempty"`
	Answer  sdui.SectionItems `json:"answer"`
}

// canned is a Resolver over Data.
type canned struct{ d *Data }

func sameValue(a, b any) bool {
	as, _ := json.Marshal(a)
	bs, _ := json.Marshal(b)
	if string(as) == string(bs) {
		return true
	}
	// 3 and "3" are the same ID.
	return sdui.Source{"v": a}.Str("v") == sdui.Source{"v": b}.Str("v") && sdui.Source{"v": a}.Str("v") != ""
}

func (c canned) Items(req sdui.SourceRequest) (sdui.SourceResult, error) {
	for i := range c.d.Sources {
		s := &c.d.Sources[i]
		ok := true
		for k, v := range s.Match {
			if k == "filter" {
				// {"filter": {"genre": "Drama"}} matches the page's filters.
				want, _ := v.(map[string]any)
				for fk, fv := range want {
					if fs, _ := fv.(string); req.Filter[fk] != fs {
						ok = false
					}
				}
				continue
			}
			if !sameValue(req.Source[k], v) {
				ok = false
			}
		}
		if !ok {
			continue
		}
		if s.Fails {
			return sdui.SourceResult{}, errors.New("the source failed")
		}
		total := max(s.Total, len(s.Items))
		items := s.Items
		if req.Offset >= len(items) {
			return sdui.SourceResult{Total: total, Selected: s.Selected, Reason: s.Reason}, nil
		}
		items = items[req.Offset:]
		more := false
		if len(items) > req.Limit {
			items, more = items[:req.Limit], true
		}
		return sdui.SourceResult{Items: append([]sdui.Item(nil), items...), More: more || req.Offset+len(items) < total, Total: total, Selected: s.Selected, Reason: s.Reason}, nil
	}
	return sdui.SourceResult{}, nil
}

func (c canned) Each(over string, kinds []string, limit int) ([]sdui.Each, error) {
	var out []sdui.Each
	for _, e := range c.d.Each[over] {
		if len(kinds) > 0 && !has(kinds, e.Kind) {
			continue
		}
		out = append(out, sdui.Each{Key: e.Key, Label: e.Label, Kind: e.Kind, ID: e.ID})
	}
	return out, nil
}

func (c canned) Subject() (*sdui.Item, error) {
	if c.d.Subject == nil {
		return nil, sdui.ErrNotFound
	}
	s := *c.d.Subject
	return &s, nil
}

func (c canned) Seasons(string) ([]sdui.Item, error) { return c.d.Seasons, nil }

func (c canned) Facets(_ string, fields []string) ([]sdui.Facet, error) {
	var out []sdui.Facet
	for _, f := range fields {
		for _, have := range c.d.Facets {
			if have.ID == f {
				out = append(out, have)
			}
		}
	}
	return out, nil
}

// Context builds the render context a fixture's input describes.
func (in *Input) Context() *sdui.RenderContext {
	r := &in.Request
	now, _ := time.Parse(time.RFC3339, r.Now)
	ctx := &sdui.RenderContext{
		Platform: r.Platform, ProfileKind: r.ProfileKind, Admin: r.Admin, UserName: r.UserName, ServerName: r.ServerName,
		Features: r.Features, Modules: r.Modules, Cannot: r.Cannot, LibraryKinds: r.LibraryKinds, Capabilities: r.Capabilities,
		Flags: r.Flags, Locale: r.Locale, Now: now, Profile: sdui.ProfileFor(r.FormFactor, r.DeviceClass),
		NativeViews: r.NativeViews, Vars: r.Vars, Params: r.Params, Scopes: r.Scopes,
		Personal: r.Personal, CanPersonalise: r.CanPersonalise, Resolver: canned{&in.Data}, BasePath: "/api/v1/sdui", Source: "builtin",
	}
	if r.Experience != "" {
		ctx.Experience = sdui.ExperienceByID(r.Experience)
	}
	if len(in.ModuleUI) > 0 {
		ctx.UI = sdui.NewModuleSet(in.ModuleUI)
	}
	if ctx.Flags == nil {
		ctx.Flags = map[string]bool{}
	}
	if ctx.Vars == nil {
		ctx.Vars = map[string]string{}
	}
	if w, h, ok := cut(r.Viewport); ok {
		ctx.Viewport = sdui.Viewport{Width: w, Height: h}
	}
	return ctx
}

func cut(s string) (int, int, bool) {
	for i := range s {
		if s[i] == 'x' {
			w, err1 := strconv.Atoi(s[:i])
			h, err2 := strconv.Atoi(s[i+1:])
			return w, h, err1 == nil && err2 == nil
		}
	}
	return 0, 0, false
}

// Run renders a fixture's input: the server's half of the case.
func Run(in Input) (Expected, error) {
	doc, err := sdui.Render(sdui.ClonePage(in.Page), in.Context())
	if errors.Is(err, sdui.ErrNotFound) {
		return Expected{NotFound: true}, nil
	}
	if err != nil {
		return Expected{}, err
	}
	doc = roundTrip(doc)
	layout := Normalise(doc)
	return Expected{Document: &doc, Layout: &layout}, nil
}

// RunSection answers one section's cursor URL for a fixture's input.
func RunSection(in Input, section string, cursor int, season string) (sdui.SectionItems, error) {
	out, err := sdui.RenderSection(sdui.ClonePage(in.Page), section, cursor, season, in.Context())
	if err != nil {
		return out, err
	}
	b, _ := json.Marshal(out)
	var rt sdui.SectionItems
	_ = json.Unmarshal(b, &rt)
	return rt, nil
}

// roundTrip puts a document through JSON, so that it is exactly what a client
// decodes.
func roundTrip(doc sdui.Document) sdui.Document {
	b, err := json.Marshal(doc)
	if err != nil {
		return doc
	}
	var out sdui.Document
	if json.Unmarshal(b, &out) != nil {
		return doc
	}
	return out
}
