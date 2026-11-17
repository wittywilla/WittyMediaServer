// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package channels

import (
	"encoding/json"
	"testing"
	"time"
)

// sourceWorld: library 60 holds commercials (ids 700…), library 70 holds
// bumpers (ids 750…), anything else is the channel's programmes.
func sourceWorld() PoolLoader {
	shows := eps(1, 8, 22*60)
	var ads, bumpers []Entry
	for i := range 12 {
		ads = append(ads, Entry{ID: 700 + int64(i), Dur: 30, Title: "Ad"})
	}
	for i := range 3 {
		bumpers = append(bumpers, Entry{ID: 750 + int64(i), Dur: 5, Title: "Bumper"})
	}
	pick := func(from []Entry, ids []int64) []Entry {
		var out []Entry
		for _, e := range from {
			for _, id := range ids {
				if e.ID == id {
					out = append(out, e)
				}
			}
		}
		return out
	}
	return func(r Rules, _ int64) ([]Entry, error) {
		switch {
		case len(r.Libraries) == 1 && r.Libraries[0] == 60:
			if len(r.Items) > 0 {
				return pick(ads, r.Items), nil
			}
			return ads, nil
		case len(r.Libraries) == 1 && r.Libraries[0] == 70:
			if len(r.Items) > 0 {
				return pick(bumpers, r.Items), nil
			}
			return bumpers, nil
		case len(r.Items) > 0:
			return pick(append(ads, bumpers...), r.Items), nil
		}
		return shows, nil
	}
}

func sourceDay(t *testing.T, def *Definition) []Programme {
	t.Helper()
	if err := def.Validate(); err != nil {
		t.Fatal(err)
	}
	progs, _, err := GenerateDay(def, 5, 20000, time.UTC, State{}, sourceWorld(), map[string][]Entry{})
	if err != nil {
		t.Fatal(err)
	}
	noOverlap(t, progs)
	return progs
}

func isAd(id int64) bool     { return id >= 700 && id < 750 }
func isBumper(id int64) bool { return id >= 750 && id < 800 }

// sourceCount counts what played as bumpers (before and after programmes) and
// as commercials, by the library it came from.
type sourceCount struct{ bumperFromAds, bumperFromBumpers, adFromAds, adFromBumpers, showsAsBumpers int }

func countSources(progs []Programme) sourceCount {
	var c sourceCount
	for _, p := range progs {
		switch p.Role {
		case RolePre, RolePost:
			switch {
			case isAd(p.ItemID):
				c.bumperFromAds++
			case isBumper(p.ItemID):
				c.bumperFromBumpers++
			default:
				c.showsAsBumpers++
			}
		case RoleAd:
			switch {
			case isAd(p.ItemID):
				c.adFromAds++
			case isBumper(p.ItemID):
				c.adFromBumpers++
			}
		}
	}
	return c
}

func sourceChannel() Definition {
	return Definition{Mode: "simple", Order: Order{Mode: "rotate"},
		Filler: Filler{Commercials: []CommercialPick{{Library: 60}}, Between: 3},
		Bumpers: BumperPlan{
			Libraries: []CommercialPick{{Library: 70}},
			Rules: []BumperRule{
				{ID: "in", When: WhenBefore, Count: 2},
				{ID: "out", When: WhenAfter},
			},
		}}
}

// By default bumpers come only from the bumper libraries and commercials only
// from the commercials libraries.
func TestBumpersAndCommercialsKeepToTheirOwnLibraries(t *testing.T) {
	def := sourceChannel()
	c := countSources(sourceDay(t, &def))
	if c.bumperFromBumpers == 0 || c.adFromAds == 0 {
		t.Fatalf("nothing played: %+v", c)
	}
	if c.bumperFromAds != 0 || c.adFromBumpers != 0 || c.showsAsBumpers != 0 {
		t.Errorf("a video played from the wrong library: %+v", c)
	}
}

// "Bumpers may also come from the commercials": a rule with no videos of its
// own draws on both, and the commercials stay commercials too.
func TestBumpersFromCommercials(t *testing.T) {
	def := sourceChannel()
	def.Bumpers.FromCommercials = true
	c := countSources(sourceDay(t, &def))
	if c.bumperFromAds == 0 || c.bumperFromBumpers == 0 {
		t.Errorf("bumpers should come from both libraries: %+v", c)
	}
	if c.adFromAds == 0 || c.adFromBumpers != 0 {
		t.Errorf("commercials should still come only from the commercials: %+v", c)
	}
}

// "Commercials may also come from the bumper libraries".
func TestCommercialsFromBumpers(t *testing.T) {
	def := sourceChannel()
	def.Filler.FromBumpers = true
	c := countSources(sourceDay(t, &def))
	if c.adFromAds == 0 || c.adFromBumpers == 0 {
		t.Errorf("commercials should come from both libraries: %+v", c)
	}
	if c.bumperFromAds != 0 {
		t.Errorf("bumpers should still come only from the bumper libraries: %+v", c)
	}
}

// Only chosen videos of a bumper library, and of a commercials library.
func TestChosenVideosOfALibrary(t *testing.T) {
	def := sourceChannel()
	def.Bumpers.Libraries = []CommercialPick{{Library: 70, Only: true, Items: []int64{751}}}
	def.Filler.Commercials = []CommercialPick{{Library: 60, Only: true, Items: []int64{703, 704}}}
	for _, p := range sourceDay(t, &def) {
		switch p.Role {
		case RolePre, RolePost:
			if p.ItemID != 751 {
				t.Fatalf("a bumper that wasn't chosen played: %+v", p)
			}
		case RoleAd:
			if p.ItemID != 703 && p.ItemID != 704 {
				t.Fatalf("a commercial that wasn't chosen played: %+v", p)
			}
		}
	}
	// "Only these" with none chosen plays nothing from that library.
	def.Bumpers.Libraries = []CommercialPick{{Library: 70, Only: true}}
	if c := countSources(sourceDay(t, &def)); c.bumperFromBumpers+c.bumperFromAds+c.showsAsBumpers != 0 {
		t.Errorf("no bumper was chosen, yet some played: %+v", c)
	}
}

// Promos chosen as bumpers out of the commercials library the channel also
// uses whole: they play as bumpers, and not as commercials too, unless the
// owner lets commercials come from the bumper libraries.
func TestBumpersChosenFromTheCommercialsLibrary(t *testing.T) {
	def := sourceChannel()
	promos := map[int64]bool{702: true, 703: true}
	def.Bumpers.Libraries = []CommercialPick{{Library: 60, Only: true, Items: []int64{702, 703}}}
	asAds, asBumpers := 0, 0
	for _, p := range sourceDay(t, &def) {
		switch {
		case p.Role == RoleAd && promos[p.ItemID]:
			asAds++
		case (p.Role == RolePre || p.Role == RolePost) && promos[p.ItemID]:
			asBumpers++
		case p.Role == RolePre || p.Role == RolePost:
			t.Fatalf("a bumper that wasn't chosen played: %+v", p)
		}
	}
	if asBumpers == 0 || asAds != 0 {
		t.Errorf("chosen promos: %d as bumpers, %d as commercials", asBumpers, asAds)
	}
	def.Filler.FromBumpers = true
	asAds = 0
	for _, p := range sourceDay(t, &def) {
		if p.Role == RoleAd && promos[p.ItemID] {
			asAds++
		}
	}
	if asAds == 0 {
		t.Error("with commercials from the bumper libraries, the promos should be commercials too")
	}
}

// A rule with no videos of its own on a channel with no bumper libraries plays
// nothing: never the channel's programmes as bumpers.
func TestEmptyRuleWithoutBumperLibraries(t *testing.T) {
	def := sourceChannel()
	def.Bumpers.Libraries = nil
	c := countSources(sourceDay(t, &def))
	if c.showsAsBumpers != 0 || c.bumperFromAds != 0 || c.bumperFromBumpers != 0 {
		t.Errorf("an empty rule played something: %+v", c)
	}
	if c.adFromAds == 0 {
		t.Errorf("the commercials should still play: %+v", c)
	}
}

// Channels made before bumper libraries keep working as they did: a rule's own
// videos play, even out of a commercials library, and are not commercials too.
func TestChannelsFromBeforeBumperLibraries(t *testing.T) {
	const old = `{"mode":"simple","rules":{},"order":{"mode":"rotate"},"blocks":null,"filler":{"between":3,"commercials":[{"library":60}]},"alignMin":0,
		"bumpers":{"rules":[{"id":"in","when":"before","source":{"items":[700,701]},"mandatory":true}]}}`
	var def Definition
	if err := json.Unmarshal([]byte(old), &def); err != nil {
		t.Fatal(err)
	}
	if def.Bumpers.FromCommercials || def.Filler.FromBumpers || len(def.Bumpers.Libraries) != 0 {
		t.Fatalf("an old channel gained settings: %+v", def)
	}
	b, _ := json.Marshal(def.Bumpers)
	if string(b) != `{"rules":[{"id":"in","when":"before","source":{"items":[700,701]},"mandatory":true}]}` {
		t.Errorf("an old channel saves differently: %s", b)
	}
	pre := 0
	for _, p := range sourceDay(t, &def) {
		switch p.Role {
		case RolePre:
			pre++
			if p.ItemID != 700 && p.ItemID != 701 {
				t.Fatalf("the rule's own videos should lead in: %+v", p)
			}
		case RoleAd:
			if p.ItemID == 700 || p.ItemID == 701 {
				t.Fatalf("a rule's own video played as a commercial: %+v", p)
			}
		}
	}
	if pre == 0 {
		t.Error("the old rule played nothing")
	}
}

// A rule's own videos still win when the channel has bumper libraries.
func TestRuleWithItsOwnVideos(t *testing.T) {
	def := sourceChannel()
	def.Bumpers.Rules = []BumperRule{{ID: "in", When: WhenBefore, Source: Rules{Items: []int64{705}}}}
	for _, p := range sourceDay(t, &def) {
		if p.Role == RolePre && p.ItemID != 705 {
			t.Fatalf("the rule's own video should play, not the library's: %+v", p)
		}
	}
}

func TestSourceValidation(t *testing.T) {
	def := sourceChannel()
	def.Bumpers.Libraries = make([]CommercialPick, maxPickLibs+1)
	if def.Validate() == nil {
		t.Error("too many bumper libraries were accepted")
	}
	def = sourceChannel()
	def.Bumpers.Libraries = []CommercialPick{{Library: 70, Only: true, Items: make([]int64, maxListLen+1)}}
	if def.Validate() == nil {
		t.Error("too many chosen bumpers were accepted")
	}
}

// Who may see a channel counts its bumper libraries, and not an empty rule as
// "everything".
func TestAllRulesCountsBumperLibraries(t *testing.T) {
	def := sourceChannel()
	var libs, empty int
	for _, r := range def.allRules() {
		if len(r.Libraries) == 1 && r.Libraries[0] == 70 {
			libs++
		}
		if r.empty() {
			empty++
		}
	}
	if libs != 1 {
		t.Errorf("the bumper library should count once, got %d", libs)
	}
	// d.Rules is the channel's own rules: empty here, which is fine. No bumper
	// rule should add another.
	if empty != 1 {
		t.Errorf("empty bumper rules were counted as rules: %d empty sets", empty)
	}
}
