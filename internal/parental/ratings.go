// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package parental

// Rating systems, as a parent knows them. A limit is kept as an age in years,
// because that is the one thing every board's ratings can be turned into; these
// tables let someone choose "PG-13" or "FSK 12" and see what else that allows.
// The ages match the ones the library gives each rating when a title is saved
// (library.AgeFor, and library.BoardAge for games' boards); a test keeps the two in step.

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
	// Film and television boards of more countries. Their certificates are read by the country
	// the server keeps them for (the ratingCountry setting), as letters mean different ages in
	// different places.
	{ID: "acb", Name: "Australian Classification", Country: "AU", Kinds: []string{KindFilm, KindTV}, Ratings: []Rating{{"G", 0}, {"PG", 10}, {"M", 15}, {"MA15+", 15}, {"R18+", 18}}},
	{ID: "nzcl", Name: "NZ Classification Office", Country: "NZ", Kinds: []string{KindFilm, KindTV}, Ratings: []Rating{{"G", 0}, {"PG", 10}, {"R13", 13}, {"R15", 15}, {"M", 16}, {"R16", 16}, {"R18", 18}}},
	{ID: "chvrs", Name: "Canadian Home Video Rating", Country: "CA", Kinds: []string{KindFilm, KindTV}, Ratings: []Rating{{"G", 0}, {"PG", 10}, {"14A", 14}, {"18A", 18}, {"R", 18}}},
	{ID: "cnc", Name: "CNC", Country: "FR", Kinds: []string{KindFilm, KindTV}, Ratings: []Rating{{"U", 0}, {"10", 10}, {"12", 12}, {"16", 16}, {"18", 18}}},
	{ID: "ifco", Name: "IFCO", Country: "IE", Kinds: []string{KindFilm, KindTV}, Ratings: []Rating{{"G", 0}, {"PG", 10}, {"12A", 12}, {"15A", 15}, {"16", 16}, {"18", 18}}},
	{ID: "icaa", Name: "ICAA", Country: "ES", Kinds: []string{KindFilm, KindTV}, Ratings: []Rating{{"A", 0}, {"7", 7}, {"12", 12}, {"16", 16}, {"18", 18}}},
	{ID: "mic", Name: "Italian ratings", Country: "IT", Kinds: []string{KindFilm, KindTV}, Ratings: []Rating{{"T", 0}, {"6+", 6}, {"VM14", 14}, {"VM18", 18}}},
	{ID: "classind", Name: "ClassInd", Country: "BR", Kinds: []string{KindFilm, KindTV}, Ratings: []Rating{{"L", 0}, {"10", 10}, {"12", 12}, {"14", 14}, {"16", 16}, {"18", 18}}},
	{ID: "eirin", Name: "Eirin", Country: "JP", Kinds: []string{KindFilm, KindTV}, Ratings: []Rating{{"G", 0}, {"PG12", 12}, {"R15+", 15}, {"R18+", 18}}},
	{ID: "kmrb", Name: "KMRB", Country: "KR", Kinds: []string{KindFilm, KindTV}, Ratings: []Rating{{"ALL", 0}, {"12", 12}, {"15", 15}, {"18", 18}}},
	{ID: "smr", Name: "Statens medieråd", Country: "SE", Kinds: []string{KindFilm, KindTV}, Ratings: []Rating{{"Btl", 0}, {"7", 7}, {"11", 11}, {"15", 15}}},
	{ID: "medietilsynet", Name: "Medietilsynet", Country: "NO", Kinds: []string{KindFilm, KindTV}, Ratings: []Rating{{"A", 0}, {"6", 6}, {"9", 9}, {"12", 12}, {"15", 15}, {"18", 18}}},
	{ID: "medieraadet", Name: "Medierådet", Country: "DK", Kinds: []string{KindFilm, KindTV}, Ratings: []Rating{{"A", 0}, {"7", 7}, {"11", 11}, {"15", 15}}},
	{ID: "kavi", Name: "KAVI", Country: "FI", Kinds: []string{KindFilm, KindTV}, Ratings: []Rating{{"S", 0}, {"K7", 7}, {"K12", 12}, {"K16", 16}, {"K18", 18}}},
	{ID: "cbfc", Name: "CBFC", Country: "IN", Kinds: []string{KindFilm, KindTV}, Ratings: []Rating{{"U", 0}, {"UA 7+", 7}, {"UA 13+", 13}, {"UA 16+", 16}, {"A", 18}}},
	{ID: "rtc", Name: "RTC", Country: "MX", Kinds: []string{KindFilm, KindTV}, Ratings: []Rating{{"AA", 0}, {"A", 0}, {"B", 12}, {"B15", 15}, {"C", 18}}},
	{ID: "cce", Name: "CCE", Country: "PT", Kinds: []string{KindFilm, KindTV}, Ratings: []Rating{{"M/3", 3}, {"M/6", 6}, {"M/12", 12}, {"M/14", 14}, {"M/16", 16}, {"M/18", 18}}},
	// Games boards of more countries.
	{ID: "cero", Name: "CERO", Country: "JP", Kinds: []string{KindGame}, Ratings: []Rating{{"CERO A", 0}, {"CERO B", 12}, {"CERO C", 15}, {"CERO D", 17}, {"CERO Z", 18}}},
	{ID: "acb-games", Name: "Australian Classification (games)", Country: "AU", Kinds: []string{KindGame}, Ratings: []Rating{{"ACB G", 0}, {"ACB PG", 8}, {"ACB M", 15}, {"ACB MA15+", 15}, {"ACB R18+", 18}}},
	{ID: "grac", Name: "GRAC", Country: "KR", Kinds: []string{KindGame}, Ratings: []Rating{{"GRAC ALL", 0}, {"GRAC 12", 12}, {"GRAC 15", 15}, {"GRAC 18", 18}}},
	{ID: "classind-games", Name: "ClassInd (games)", Country: "BR", Kinds: []string{KindGame}, Ratings: []Rating{{"ClassInd L", 0}, {"ClassInd 10", 10}, {"ClassInd 12", 12}, {"ClassInd 14", 14}, {"ClassInd 16", 16}, {"ClassInd 18", 18}}},
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
