// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"slices"
	"sort"
	"strings"
)

// Placement: everything movable, by data (docs/plans/DESIGN-2.md 7.3). The shell is made of
// regions; each has a position and may be hidden. The experience sets them, the
// administrator overrides them for everyone and a profile for its own device, and the
// server sends the merged result in shell.placements: a renderer only draws it.

// Region describes one region of the shell.
type Region struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Positions are where it may be; empty: it has none to choose, only to show or hide.
	Positions []string `json:"positions,omitempty"`
	// Locked regions can't be hidden: the way to Settings, to You and out of a screen.
	Locked bool `json:"locked,omitempty"`
	// Styles are the choices of a region with a style (the hero's).
	Styles []string `json:"styles,omitempty"`
	// Off regions are hidden until an experience, a module's page or a placement turns them on.
	Off bool `json:"off,omitempty"`
}

// Regions are the regions of the shell, in the order an editor lists them.
var Regions = []Region{
	{ID: "brand", Label: "Server name", Positions: []string{"start", "center", "top"}},
	{ID: "nav", Label: "Menu", Positions: []string{"start", "end", "top", "bottom"}, Locked: true},
	{ID: "search", Label: "Search", Positions: []string{"nav", "top", "floating"}},
	{ID: "profile", Label: "You", Positions: []string{"nav", "top"}, Locked: true},
	{ID: "clock", Label: "Clock", Positions: []string{"top", "dock"}},
	{ID: "dock", Label: "Dock", Positions: []string{"top", "bottom", "side"}},
	{ID: "legend", Label: "Key legend"},
	{ID: "hero", Label: "Spotlight", Styles: []string{"billboard", "banner", "carousel", "row"}},
	// The status bar (the profile, the server's name, the clock, the controllers' batteries) and the
	// system dock (a round dock of the screen's destinations) are WittyPlay Online's: off in every
	// other look, turned on by a page's regions (Page.Regions) or a placement.
	{ID: "statusBar", Label: "Status bar", Positions: []string{"top"}, Off: true},
	{ID: "systemDock", Label: "System dock", Positions: []string{"bottom", "side"}, Off: true},
}

// RegionByID is a region, nil for an ID that isn't one.
func RegionByID(id string) *Region {
	for i := range Regions {
		if Regions[i].ID == id {
			return &Regions[i]
		}
	}
	return nil
}

// RegionPlacements is a set of placements by region.
type RegionPlacements map[string]Placement

// Validate checks placements: known regions, positions the region has, locked regions
// shown.
func (p RegionPlacements) Validate(path string) []Issue {
	v := &validator{page: &Page{Kind: "custom"}}
	ids := make([]string, 0, len(p))
	for id := range p {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var names []string
	for _, r := range Regions {
		names = append(names, r.ID)
	}
	for _, id := range ids {
		pl, r := p[id], RegionByID(id)
		if r == nil {
			v.err(path+"/"+id, suggest(id, names), "%q is not a region of the shell; the regions are %s", id, strings.Join(names, ", "))
			continue
		}
		if pl.Position != "" && !slices.Contains(r.Positions, pl.Position) {
			v.err(path+"/"+id+"/position", "Choose one of "+strings.Join(r.Positions, ", ")+".", "the %s can't be at %q", r.Label, pl.Position)
		}
		if pl.Style != "" && !slices.Contains(r.Styles, pl.Style) {
			v.err(path+"/"+id+"/style", "Choose one of "+strings.Join(r.Styles, ", ")+".", "the %s has no style %q", r.Label, pl.Style)
		}
		if r.Locked && pl.Visible != nil && !*pl.Visible {
			v.err(path+"/"+id+"/visible", "The way to Settings, to You and out of a screen can't be hidden.", "the %s can't be hidden", r.Label)
		}
	}
	return v.issues
}

// MergePlacements lays each set over the one before: a region's position, visibility and
// style set later replace those set earlier. The result has every region.
func MergePlacements(layers ...RegionPlacements) RegionPlacements {
	out := RegionPlacements{}
	for _, r := range Regions {
		out[r.ID] = Placement{}
	}
	for _, l := range layers {
		for id, pl := range l {
			if RegionByID(id) == nil {
				continue
			}
			cur := out[id]
			if pl.Position != "" {
				cur.Position = pl.Position
			}
			if pl.Visible != nil {
				cur.Visible = pl.Visible
			}
			if pl.Style != "" {
				cur.Style = pl.Style
			}
			out[id] = cur
		}
	}
	for id, pl := range out {
		if pl.Visible == nil {
			t := !RegionByID(id).Off
			pl.Visible = &t
			out[id] = pl
		}
	}
	return out
}
