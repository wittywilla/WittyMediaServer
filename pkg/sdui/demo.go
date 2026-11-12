// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import "strconv"

// DemoResolver answers every source with a few generated sample items and no pictures, so a
// page can be drawn before any library exists: the first-run chooser draws every experience
// with the client's own renderer from it (GET /sdui/preview). It touches no library and
// holds no real title.
type DemoResolver struct{}

var demoLibraries = []struct{ name, kind string }{{"Films", "movies"}, {"Shows", "shows"}, {"Music", "music"}, {"Photos", "photos"}}

// Items makes up to req.Limit samples of the kind the source yields.
func (DemoResolver) Items(req SourceRequest) (SourceResult, error) {
	spec, ok := SourceType(req.Source.Type())
	if !ok {
		return SourceResult{}, nil
	}
	kind := "movie"
	if len(spec.Yields) > 0 {
		kind = spec.Yields[0]
	}
	if kind == "status" || kind == "library" {
		return SourceResult{}, nil
	}
	const total = 10
	var items []Item
	for i := req.Offset; i < total && len(items) < req.Limit; i++ {
		n := strconv.Itoa(i + 1)
		it := Item{ID: "demo-" + kind + "-" + n, Kind: kind, Title: "Sample " + kind + " " + n, Subtitle: "Sample", Year: 2000 + i, Route: "/library", Icon: "play"}
		if kind == "channel" {
			it.Title, it.Number = "Sample channel "+n, n
		}
		if spec.Type == "continueWatching" {
			it.Progress = 0.2 + 0.07*float64(i)
		}
		items = append(items, it)
	}
	return SourceResult{Items: items, More: req.Offset+len(items) < total, Total: total}, nil
}

// Each lists four sample libraries.
func (DemoResolver) Each(over string, kinds []string, limit int) ([]Each, error) {
	if over != "libraries" {
		return nil, nil
	}
	var out []Each
	for i, l := range demoLibraries {
		if len(kinds) > 0 && !contains(kinds, l.kind) {
			continue
		}
		out = append(out, Each{Key: strconv.Itoa(i + 1), Label: l.name, Kind: l.kind})
	}
	return out, nil
}

// Subject has nothing: there is no detail page in a preview.
func (DemoResolver) Subject() (*Item, error) { return nil, ErrNotFound }

// Seasons and Facets have nothing.
func (DemoResolver) Seasons(string) ([]Item, error)           { return nil, nil }
func (DemoResolver) Facets(string, []string) ([]Facet, error) { return nil, nil }
