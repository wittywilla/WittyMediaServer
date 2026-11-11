// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package parental

// Rating systems, as a parent knows them. A limit is kept as an age in years,
// because that is the one thing every board's ratings can be turned into; these
// tables let someone choose "PG-13" or "FSK 12" and see what else that allows.
// The ages match the ones the library gives each rating when a title is saved
// (library.AgeFor and gamemeta.AgeFor); a test keeps the two in step.

// Rating is one step of a system.
type Rating struct {
	Label string `json:"label"`
	Age   int    `json:"age"`
}

// System is one board's ratings, youngest first.
type System struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Country string   `json:"country"` // "" = used in many
	Kinds   []string `json:"kinds"`   // the kinds of media it rates
	Ratings []Rating `json:"ratings"`
}

// The kinds of media a limit can be set for apart from the rest.
const (
	KindFilm  = "film"
	KindTV    = "tv"
	KindGame  = "game"
	KindMusic = "music"
	KindBook  = "book"
)

// Kinds lists them in the order Settings shows them.
var Kinds = []string{KindFilm, KindTV, KindGame, KindMusic, KindBook}

// ValidKind reports whether a name is one of them.
func ValidKind(k string) bool {
	for _, x := range Kinds {
		if x == k {
			return true
		}
	}
	return false
}

// Systems are the boards Settings offers.
var Systems = []System{
	{ID: "mpa", Name: "MPA", Country: "US", Kinds: []string{KindFilm}, Ratings: []Rating{{"G", 0}, {"PG", 10}, {"PG-13", 13}, {"R", 17}, {"NC-17", 18}}},
	{ID: "tvpg", Name: "TV Parental Guidelines", Country: "US", Kinds: []string{KindTV}, Ratings: []Rating{{"TV-Y", 0}, {"TV-G", 0}, {"TV-Y7", 7}, {"TV-PG", 10}, {"TV-14", 14}, {"TV-MA", 17}}},
	{ID: "bbfc", Name: "BBFC", Country: "GB", Kinds: []string{KindFilm, KindTV}, Ratings: []Rating{{"U", 0}, {"PG", 10}, {"12A", 12}, {"12", 12}, {"15", 15}, {"18", 18}}},
	{ID: "fsk", Name: "FSK", Country: "DE", Kinds: []string{KindFilm, KindTV}, Ratings: []Rating{{"FSK 0", 0}, {"FSK 6", 6}, {"FSK 12", 12}, {"FSK 16", 16}, {"FSK 18", 18}}},
	{ID: "kijkwijzer", Name: "Kijkwijzer", Country: "NL", Kinds: []string{KindFilm, KindTV}, Ratings: []Rating{{"AL", 0}, {"6", 6}, {"9", 9}, {"12", 12}, {"14", 14}, {"16", 16}, {"18", 18}}},
	{ID: "esrb", Name: "ESRB", Country: "US", Kinds: []string{KindGame}, Ratings: []Rating{{"ESRB E", 0}, {"ESRB E10+", 10}, {"ESRB T", 13}, {"ESRB M", 17}, {"ESRB AO", 18}}},
	{ID: "pegi", Name: "PEGI", Country: "", Kinds: []string{KindGame}, Ratings: []Rating{{"PEGI 3", 3}, {"PEGI 7", 7}, {"PEGI 12", 12}, {"PEGI 16", 16}, {"PEGI 18", 18}}},
	{ID: "usk", Name: "USK", Country: "DE", Kinds: []string{KindGame}, Ratings: []Rating{{"USK 0", 0}, {"USK 6", 6}, {"USK 12", 12}, {"USK 16", 16}, {"USK 18", 18}}},
	{ID: "advisory", Name: "Parental advisory", Country: "", Kinds: []string{KindMusic}, Ratings: []Rating{{"Clean", 0}, {"Explicit", 17}}},
}

// Allowed is the ratings of a system that a limit of maxAge lets through.
// maxAge < 0 is no limit.
func (s System) Allowed(maxAge int) []string {
	out := []string{}
	for _, r := range s.Ratings {
		if maxAge < 0 || r.Age <= maxAge {
			out = append(out, r.Label)
		}
	}
	return out
}

// Allows reports whether a title of this age passes a limit. age < 0 means the
// title has no rating we know, which unrated decides.
func Allows(maxAge int, unrated bool, age int) bool {
	if maxAge < 0 {
		return true
	}
	if age < 0 {
		return unrated
	}
	return age <= maxAge
}
