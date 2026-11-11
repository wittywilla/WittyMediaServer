// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package gameregion

import (
	"slices"
	"testing"
)

func TestRankFollowsTheOrder(t *testing.T) {
	t.Cleanup(func() { Set("") })
	Set("")
	if Rank("USA") >= Rank("Europe") || Rank("Europe") >= Rank("Japan") {
		t.Fatalf("usual order: usa %d, europe %d, japan %d", Rank("USA"), Rank("Europe"), Rank("Japan"))
	}
	Set("japan,europe")
	if got := Order(); !slices.Equal(got, []string{Japan, Europe, USA, Australia, Asia, Brazil}) {
		t.Fatalf("order %v", got)
	}
	if Rank("Japan") != 0 || Rank("Europe") != 1 || Rank("USA") != 2 {
		t.Fatalf("chosen order: japan %d, europe %d, usa %d", Rank("Japan"), Rank("Europe"), Rank("USA"))
	}
	if Rank("USA, Japan") != 0 {
		t.Fatal("a version for two regions counts for the better one")
	}
	if Rank("World") != 0 {
		t.Fatal("a version for everywhere is as good as the first choice")
	}
}

func TestRankOnlyReadsWholeWords(t *testing.T) {
	t.Cleanup(func() { Set("") })
	Set("")
	for _, s := range []string{"", "Thousand", "Rev 1", "Busan", "Japanese"} {
		if got := Rank(s); got != Unknown() {
			t.Errorf("Rank(%q) = %d", s, got)
		}
	}
	if Rank("Germany") != Rank("Europe") || Rank("Canada") != Rank("USA") || Rank("Korea") != Rank("Asia") {
		t.Error("a country counts as its region")
	}
}

func TestRankFile(t *testing.T) {
	t.Cleanup(func() { Set("") })
	Set("japan,usa")
	for name, want := range map[string]int{
		"Game (Japan).sfc":             0,
		"Game (J).sfc":                 0,
		"Game (JU) [!].sfc":            0,
		"Game (USA).sfc":               1,
		"Game (U) [!].sfc":             1,
		"Game (USA, Europe) (Rev 1).z": 1,
		"Game (W).sfc":                 0,
		"Game (World).sfc":             0,
		"America (Beta).sfc":           Unknown(),
		"Japan.sfc":                    Unknown(),
		"Game (En,Ja).sfc":             Unknown(),
		"Game (unfinished":             Unknown(),
	} {
		if got := RankFile(name); got != want {
			t.Errorf("RankFile(%q) = %d, want %d", name, got, want)
		}
	}
}

func TestSettingIsChecked(t *testing.T) {
	for v, want := range map[string]bool{
		"japan,usa":                 true,
		"usa":                       true,
		"japan, usa":                true,
		"japan,japan":               false,
		"mars":                      false,
		"usa,../../etc":             false,
		"usa,europe,australia,asia": true,
	} {
		if got := Valid(v); got != want {
			t.Errorf("Valid(%q) = %v", v, got)
		}
	}
	if got := Parse("mars,japan,japan"); !slices.Equal(got, []string{Japan, USA, Europe, Australia, Asia, Brazil}) {
		t.Errorf("Parse: %v", got)
	}
}
