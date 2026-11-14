// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import "testing"

func TestRatingBoards(t *testing.T) {
	for _, c := range []struct {
		sys, val string
		want     int // -1: unknown
	}{
		{"ESRB", "E", 0}, {"ESRB", "E10+", 10}, {"ESRB", "T", 13}, {"ESRB", "M", 17}, {"ESRB", "AO", 18}, {"ESRB", "RP", -1},
		{"ESRB", "NOT RATED", -1},
		{"PEGI", "3", 3}, {"PEGI", "PEGI 18", 18}, {"PEGI", "12+", 12},
		{"CERO", "A", 0}, {"CERO", "B", 12}, {"CERO", "Z", 18},
		{"USK", "6", 6}, {"USK", "USK 16", 16},
		{"ACB", "MA15+", 15}, {"ACB", "R18+", 18},
		{"CLASS_IND", "L", 0}, {"Class_Ind", "14", 14},
		{"GRAC", "ALL", 0}, {"GRAC", "Testing", -1},
		{"nonsense", "12", -1},
	} {
		got := BoardAge(c.sys, c.val)
		if (got == nil) != (c.want < 0) || (got != nil && *got != c.want) {
			t.Errorf("BoardAge(%s, %s) = %v, want %d", c.sys, c.val, got, c.want)
		}
	}
	if IsRatingBoard("PG") || !IsRatingBoard("esrb") {
		t.Error("IsRatingBoard")
	}
}

// A game's rating names its board, so an ESRB M is 17 where an Australian M is 15.
func TestAgeForReadsBoards(t *testing.T) {
	for rating, want := range map[string]int{"ESRB M": 17, "ESRB:T": 13, "PEGI 16": 16, "CERO Z": 18, "M": 15} {
		if got := AgeFor(rating); got == nil || *got != want {
			t.Errorf("AgeFor(%q) = %v, want %d", rating, got, want)
		}
	}
	if got := AgeFor("ESRB RP"); got != nil {
		t.Errorf("a pending rating is no rating: %v", *got)
	}
}
