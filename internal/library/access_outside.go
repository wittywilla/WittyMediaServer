// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

// AllowsOutside says whether the account may see something that isn't in a library: a
// card a module's data source gives (docs/SDUI-MODULES.md). It has no ID to allow or block
// and no library, so what is held to is what it says of itself: its kind (games need the
// games switch), its genres and its content rating. A module that says nothing of a
// rating is unrated, which a profile that may not see unrated titles doesn't see.
func (a *Access) AllowsOutside(kind, contentRating string, genres []string) bool {
	if a == nil {
		return true
	}
	if kind == "game" && !a.Can(FeatureGames) {
		return false
	}
	if !a.genresOK(genres) {
		return false
	}
	return a.ageOK(AgeFor(contentRating), MediaKind(kind, kind == "episode"))
}
