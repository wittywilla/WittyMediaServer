// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"strconv"
	"strings"
)

// Age rating boards of games, which name themselves in a rating ("ESRB T", "PEGI 16"), since
// boards reuse each other's letters: an ESRB M isn't an Australian M. What each rating means
// comes from the boards' own definitions; where a board's rating is a band, this is the bottom
// of it. Whoever describes a game (a module, through scan.describe) writes its rating this way.

var boardAges = map[string]map[string]int{
	"ESRB":     {"EC": 3, "E": 0, "KA": 0, "E10+": 10, "E10": 10, "T": 13, "M": 17, "AO": 18},
	"CERO":     {"A": 0, "B": 12, "C": 15, "D": 17, "Z": 18},
	"ACB":      {"G": 0, "PG": 8, "M": 15, "MA15+": 15, "MA15": 15, "R18+": 18, "R18": 18, "RC": 18},
	"GRAC":     {"ALL": 0, "12": 12, "15": 15, "18": 18, "19": 19, "TESTING": -1},
	"CLASSIND": {"L": 0, "AL": 0, "10": 10, "12": 12, "14": 14, "16": 16, "18": 18},
	"ELSPA":    {"3": 3, "3+": 3, "7": 7, "7+": 7, "11": 11, "11+": 11, "15": 15, "15+": 15, "18": 18, "18+": 18, "U": 0, "PG": 8},
}

// boardNames are the boards' names, by every way they are written.
var boardNames = map[string]string{"ESRB": "ESRB", "PEGI": "PEGI", "CERO": "CERO", "USK": "USK", "ACB": "ACB", "GRAC": "GRAC",
	"CLASSIND": "ClassInd", "CLASS_IND": "ClassInd", "CLASSIFICATIONIND": "ClassInd", "ELSPA": "ELSPA", "OFLC": "ACB"}

// IsRatingBoard says whether a name is a games age rating board's. Film ratings are written
// the same way, so only these names mean a game's rating.
func IsRatingBoard(name string) bool {
	_, ok := boardNames[strings.ToUpper(strings.TrimSpace(name))]
	return ok
}

// BoardAge says what a board's rating means, in years. Nil: unknown, or not decided yet
// (Rating Pending), which is treated as having no rating at all.
func BoardAge(board, value string) *int {
	sys := strings.ToUpper(strings.TrimSpace(board))
	v := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
	switch sys {
	case "CLASS_IND", "CLASSIFICATIONIND":
		sys = "CLASSIND"
	case "OFLC":
		sys = "ACB"
	}
	if t, ok := boardAges[sys]; ok {
		if a, ok := t[v]; ok {
			if a < 0 {
				return nil
			}
			return &a
		}
		return nil
	}
	if sys == "PEGI" || sys == "USK" {
		v = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(v, "PEGI"), "USK"), "+")
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 21 {
			return &n
		}
	}
	return nil
}
