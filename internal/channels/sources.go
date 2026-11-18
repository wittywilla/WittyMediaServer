// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package channels

import "encoding/json"

// Where a channel's bumpers and commercials come from.
//
// A channel has commercials libraries and bumper libraries, each whole or only
// some of its videos. By default bumpers come only from the bumper libraries
// and commercials only from the commercials libraries; a switch on each side
// lets one also draw on the other.
//
// A bumper rule that names videos of its own plays exactly those, as rules did
// before channels had bumper libraries. Those videos are bumpers on this
// channel and not commercials too, even when they sit in a commercials library.

// empty: the rules choose nothing in particular.
func (r *Rules) empty() bool {
	b, _ := json.Marshal(r)
	return string(b) == "{}"
}

func pickRules(picks []CommercialPick) []Rules {
	var out []Rules
	for _, c := range picks {
		if r, ok := c.rules(); ok {
			out = append(out, r)
		}
	}
	return out
}

// bumperSets is what a rule that names no videos of its own plays: the bumper
// libraries, and the commercials libraries too when the owner allows it.
func (d *Definition) bumperSets() []Rules {
	out := pickRules(d.Bumpers.Libraries)
	if d.Bumpers.FromCommercials {
		out = append(out, pickRules(d.Filler.Commercials)...)
	}
	return out
}

// commercialSets is what the commercials come from: the filler rules, the
// commercials libraries, and the bumper libraries too when the owner allows it.
func (d *Definition) commercialSets() []Rules {
	var out []Rules
	if r := d.Filler.Rules; r != nil {
		out = append(out, *r)
	}
	out = append(out, pickRules(d.Filler.Commercials)...)
	if d.Filler.FromBumpers {
		out = append(out, pickRules(d.Bumpers.Libraries)...)
	}
	return out
}

// ruleSets is what one bumper rule plays.
func (d *Definition) ruleSets(r *BumperRule) []Rules {
	if r.Source.empty() {
		return d.bumperSets()
	}
	return []Rules{r.Source}
}

// loadSets loads each set and joins them, each video once, in the order given.
func loadSets(load PoolLoader, sets []Rules, minSec int64) ([]Entry, error) {
	if len(sets) == 1 {
		return load(sets[0], minSec)
	}
	var out []Entry
	seen := map[int64]bool{}
	for _, r := range sets {
		entries, err := load(r, minSec)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !seen[e.ID] {
				seen[e.ID] = true
				out = append(out, e)
			}
		}
	}
	return out, nil
}
