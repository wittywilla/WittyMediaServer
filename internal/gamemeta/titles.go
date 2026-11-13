// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package gamemeta

import "unicode"

// A game's other names. Most sources call a game what it was sold as in English, or write
// a Japanese name in Latin letters (A Ressha de Ikou). Some also know the name in its own
// writing (A列車で行こう) and the English release's (A-Train), which a profile may rather see.
// Info.Native and Info.English carry those; Info.Title keeps its meaning.

// NonLatin says whether a name is written, at least partly, in letters other than Latin
// ones: Japanese, Chinese, Korean, Cyrillic, Greek and so on. Digits and punctuation don't count.
func NonLatin(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) && !unicode.In(r, unicode.Latin) {
			return true
		}
	}
	return false
}

// nativeName is a name worth keeping as a game's own: written in a script of its own.
func nativeName(s string) string {
	s = Line(s)
	if !NonLatin(s) {
		return ""
	}
	return s
}

// englishName is a name worth keeping as a game's English one: Latin letters only.
func englishName(s string) string {
	s = Line(s)
	if s == "" || NonLatin(s) {
		return ""
	}
	return s
}
