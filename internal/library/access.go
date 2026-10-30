package library

import (
	"strconv"
	"strings"
)

// Parental controls. Every content rating is turned into a minimum age when
// an item or show is saved (min_age column), so limits for Kids and Guest
// profiles become one SQL condition instead of filtering lists afterwards.
// Unrestricted profiles get no condition at all.

// ratingAges maps certificates from common rating systems to the youngest
// age they are meant for. Keys are upper-case without spaces or dashes.
var ratingAges = map[string]int{
	// United States: MPA and TV Parental Guidelines
	"G": 0, "PG": 10, "PG13": 13, "R": 17, "NC17": 18, "NR": -1, "UNRATED": -1, "NOTRATED": -1,
	"TVY": 0, "TVY7": 7, "TVY7FV": 7, "TVG": 0, "TVPG": 10, "TV14": 14, "TVMA": 17,
	// United Kingdom (BBFC)
	"U": 0, "UC": 0, "12": 12, "12A": 12, "15": 15, "18": 18, "R18": 18,
	// Germany (FSK)
	"FSK0": 0, "FSK6": 6, "FSK12": 12, "FSK16": 16, "FSK18": 18, "0": 0, "6": 6, "16": 16,
	// Canada, Australia, Ireland, France and others commonly seen in metadata
	"14A": 14, "18A": 18, "E": 0, "M": 15, "MA15": 15, "MA15+": 15, "R18+": 18, "X18": 18,
	"TOUSPUBLICS": 0, "TP": 0, "10": 10, "7": 7, "13": 13, "AL": 0, "ALL": 0,
}

// AgeFor returns the minimum age for a content rating, or nil if unknown
// (unrated). Country prefixes ("US:PG-13", "de/12") are accepted.
func AgeFor(rating string) *int {
	r := strings.ToUpper(strings.TrimSpace(rating))
	if i := strings.LastIndexAny(r, ":/"); i >= 0 {
		r = r[i+1:]
	}
	r = strings.NewReplacer(" ", "", "-", "", "_", "", ".", "").Replace(r)
	if r == "" {
		return nil
	}
	age, ok := ratingAges[r]
	if !ok {
		// "Rated 12+", "16+": a plain number is an age.
		if n, err := strconv.Atoi(strings.TrimSuffix(r, "+")); err == nil && n >= 0 && n <= 21 {
			age, ok = n, true
		}
	}
	if !ok || age < 0 {
		return nil
	}
	return &age
}

// NoAgeLimit as MaxAge: only the library and title choices apply.
const NoAgeLimit = -1

// Access are the limits of one account. A nil *Access means no limits
// (administrators, and accounts nobody restricted).
type Access struct {
	MaxAge       int      `json:"maxAge"`       // highest minimum age allowed, e.g. 7 for "Kids 7+"; -1 = no limit
	AllowUnrated bool     `json:"allowUnrated"` // titles without a known rating
	Libraries    []int64  `json:"libraries"`    // allowed libraries; empty = all
	BlockGenres  []string `json:"blockGenres"`
	AllowItems   []int64  `json:"allowItems"` // always allowed, whatever the rating
	BlockItems   []int64  `json:"blockItems"` // never allowed
	AllowShows   []int64  `json:"allowShows"`
	BlockShows   []int64  `json:"blockShows"`
	// Live TV: whether the account sees Live TV at all, the server's own
	// channels made from the libraries, and channels from tuners and IPTV.
	LiveTV          bool `json:"liveTv"`
	LibraryChannels bool `json:"libraryChannels"`
	TunerChannels   bool `json:"tunerChannels"`
}

// DefaultAccess is what a new account of a kind starts with.
func DefaultAccess(kind string) Access {
	switch kind {
	case "kids":
		return Access{MaxAge: 7, LiveTV: true, LibraryChannels: true}
	case "guest":
		return Access{MaxAge: 13}
	}
	return Access{MaxAge: NoAgeLimit, AllowUnrated: true, LiveTV: true, LibraryChannels: true, TunerChannels: true}
}

// Limited reports whether the account can't see everything. Someone who is
// limited can't set limits for others: a Kids profile they make with no limits
// and a password they know would be a way around their own.
func (a *Access) Limited() bool {
	if a == nil {
		return false
	}
	return a.MaxAge >= 0 || len(a.Libraries) > 0 || len(a.BlockGenres) > 0 || len(a.BlockItems) > 0 || len(a.BlockShows) > 0 ||
		!a.LiveTV || !a.LibraryChannels || !a.TunerChannels
}

// CanWatchChannels reports whether the account may use a kind of channel
// ("library" or "tuner").
func (a *Access) CanWatchChannels(kind string) bool {
	if a == nil {
		return true
	}
	if !a.LiveTV {
		return false
	}
	if kind == "tuner" {
		return a.TunerChannels
	}
	return a.LibraryChannels
}

func (a *Access) ageOK(age *int) bool {
	if a.MaxAge < 0 {
		return true
	}
	if age == nil {
		return a.AllowUnrated
	}
	return *age <= a.MaxAge
}

func ids(v []int64) string {
	if len(v) == 0 {
		// Matches nothing: ids start at 1, and a missing show id is
		// written as IFNULL(show_id, 0) in the clauses below.
		return "-1"
	}
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = strconv.FormatInt(n, 10)
	}
	return strings.Join(parts, ",")
}

// ItemClause is an SQL condition for items aliased "i". Ids are numbers
// from our own JSON, never user text, so they are inlined; genres are
// passed as arguments.
func (a *Access) ItemClause() (string, []any) {
	if a == nil {
		return "1=1", nil
	}
	var args []any
	ok := "1=1"
	if a.MaxAge >= 0 {
		age := "COALESCE(i.min_age, (SELECT sh.min_age FROM shows sh WHERE sh.id = i.show_id))"
		ok = age + " <= " + strconv.Itoa(a.MaxAge)
		if a.AllowUnrated {
			ok = "(" + ok + " OR " + age + " IS NULL)"
		}
	}
	cond := []string{ok}
	if len(a.Libraries) > 0 {
		cond = append(cond, "i.library_id IN ("+ids(a.Libraries)+")")
	}
	for _, g := range a.BlockGenres {
		// An episode counts its show's genres too, as AllowsItem does.
		cond = append(cond, "('|' || IFNULL(i.genres,'') || '|') NOT LIKE ?",
			"('|' || IFNULL((SELECT sh.genres FROM shows sh WHERE sh.id = i.show_id),'') || '|') NOT LIKE ?")
		args = append(args, "%|"+g+"|%", "%|"+g+"|%")
	}
	return "(i.id NOT IN (" + ids(a.BlockItems) + ") AND IFNULL(i.show_id, 0) NOT IN (" + ids(a.BlockShows) + ") AND (" +
		"i.id IN (" + ids(a.AllowItems) + ") OR IFNULL(i.show_id, 0) IN (" + ids(a.AllowShows) + ") OR (" + strings.Join(cond, " AND ") + ")))", args
}

// ShowClause is the same for shows aliased "s".
func (a *Access) ShowClause() (string, []any) {
	if a == nil {
		return "1=1", nil
	}
	var args []any
	ok := "1=1"
	if a.MaxAge >= 0 {
		ok = "s.min_age <= " + strconv.Itoa(a.MaxAge)
		if a.AllowUnrated {
			ok = "(" + ok + " OR s.min_age IS NULL)"
		}
	}
	cond := []string{ok}
	if len(a.Libraries) > 0 {
		cond = append(cond, "s.library_id IN ("+ids(a.Libraries)+")")
	}
	for _, g := range a.BlockGenres {
		cond = append(cond, "('|' || IFNULL(s.genres,'') || '|') NOT LIKE ?")
		args = append(args, "%|"+g+"|%")
	}
	return "(s.id NOT IN (" + ids(a.BlockShows) + ") AND (s.id IN (" + ids(a.AllowShows) + ") OR (" + strings.Join(cond, " AND ") + ")))", args
}

func has(v []int64, n int64) bool {
	for _, x := range v {
		if x == n {
			return true
		}
	}
	return false
}

// AllowsItem checks one item in Go (single-item pages and playback).
// showAge is the item's show's minimum age, if it is an episode.
func (a *Access) AllowsItem(it *Item, show *Show) bool {
	if a == nil {
		return true
	}
	showID := int64(0)
	if it.ShowID != nil {
		showID = *it.ShowID
	}
	if has(a.BlockItems, it.ID) || (showID != 0 && has(a.BlockShows, showID)) {
		return false
	}
	if has(a.AllowItems, it.ID) || (showID != 0 && has(a.AllowShows, showID)) {
		return true
	}
	if len(a.Libraries) > 0 && !has(a.Libraries, it.LibraryID) {
		return false
	}
	genres := it.Genres
	age := AgeFor(it.ContentRating)
	if show != nil {
		if age == nil {
			age = AgeFor(show.ContentRating)
		}
		genres = append(append([]string{}, genres...), show.Genres...)
	}
	for _, g := range a.BlockGenres {
		for _, x := range genres {
			if strings.EqualFold(g, x) {
				return false
			}
		}
	}
	return a.ageOK(age)
}

// AllowsShow checks one show.
func (a *Access) AllowsShow(sh *Show) bool {
	if a == nil {
		return true
	}
	if has(a.BlockShows, sh.ID) {
		return false
	}
	if has(a.AllowShows, sh.ID) {
		return true
	}
	if len(a.Libraries) > 0 && !has(a.Libraries, sh.LibraryID) {
		return false
	}
	for _, g := range a.BlockGenres {
		for _, x := range sh.Genres {
			if strings.EqualFold(g, x) {
				return false
			}
		}
	}
	return a.ageOK(AgeFor(sh.ContentRating))
}
