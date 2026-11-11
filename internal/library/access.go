// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/gamemeta"
	"github.com/wittywilla/wittymediaserver/internal/parental"
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
	// Games: ESRB letters typed by hand that no film rating uses
	"EC": 3, "E10+": 10, "T": 13, "AO": 18,
	// Music: the parental advisory label, for songs whose rating says so
	"EXPLICIT": 17, "CLEAN": 0,
	// The Netherlands (Kijkwijzer) and others that write a plain age are read as numbers below
	"9": 9, "14": 14,
}

// AgeFor returns the minimum age for a content rating, or nil if unknown
// (unrated). Country prefixes ("US:PG-13", "de/12") are accepted.
func AgeFor(rating string) *int {
	r := strings.ToUpper(strings.TrimSpace(rating))
	// A game's rating names its board, as boards reuse each other's letters: an ESRB M
	// isn't an Australian M.
	if sys, val, ok := strings.Cut(strings.ReplaceAll(r, ":", " "), " "); ok && gamemeta.IsSystem(sys) {
		return gamemeta.AgeFor(sys, val)
	}
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

	// Kinds: an age limit of its own for one kind of media ("film", "tv",
	// "game", "music", "book"). A kind that isn't named follows MaxAge and
	// AllowUnrated.
	Kinds map[string]KindLimit `json:"kinds,omitempty"`
	// OnlyGenres: when set, only titles with one of these genres. BlockTags:
	// tags of videos never to show.
	OnlyGenres []string `json:"onlyGenres,omitempty"`
	BlockTags  []string `json:"blockTags,omitempty"`
	// Collections whose titles are always or never allowed, like AllowItems
	// and BlockItems. Only collections that list their titles count: one made
	// from a rule has no list to go by.
	AllowCollections []int64 `json:"allowCollections,omitempty"`
	BlockCollections []int64 `json:"blockCollections,omitempty"`
	// Off names what the account may not do, of the Features below. A list of
	// what is off, so limits written before a feature was a choice leave it on.
	Off []string `json:"off,omitempty"`
	// Time: when the account may play things, and for how long each day.
	Time *parental.TimeRules `json:"time,omitempty"`

	// set are the titles the collections and tags above come to, looked up for
	// one request (Resolve). Lists made in SQL don't need it.
	set *accessSet
}

// KindLimit is the age limit for one kind of media.
type KindLimit struct {
	MaxAge       int  `json:"maxAge"` // -1 = no limit
	AllowUnrated bool `json:"allowUnrated"`
}

// MediaKind is which of the kinds a limit can name an item belongs to: "" for
// those with none of their own (photos), which follow the general limit.
func MediaKind(kind string, episode bool) string {
	switch kind {
	case "game":
		return parental.KindGame
	case "book":
		return parental.KindBook
	case "audio", "cd-image", "radio":
		return parental.KindMusic
	case "photo":
		return ""
	}
	if episode {
		return parental.KindTV
	}
	return parental.KindFilm
}

// limitFor is the age limit that applies to a kind of media.
func (a *Access) limitFor(kind string) (maxAge int, unrated bool) {
	if l, ok := a.Kinds[kind]; ok && kind != "" {
		return l.MaxAge, l.AllowUnrated || l.MaxAge < 0
	}
	return a.MaxAge, a.AllowUnrated || a.MaxAge < 0
}

// byAge reports whether any age limit applies.
func (a *Access) byAge() bool {
	if a.MaxAge >= 0 {
		return true
	}
	for _, l := range a.Kinds {
		if l.MaxAge >= 0 {
			return true
		}
	}
	return false
}

// DefaultAccess is what a new account of a kind starts with.
func DefaultAccess(kind string) Access {
	switch kind {
	case "kids":
		return Access{MaxAge: 7, LiveTV: true, LibraryChannels: true, Off: append([]string{}, kidsOff...)}
	case "guest":
		return Access{MaxAge: 13, Off: append([]string{}, kidsOff...)}
	}
	return Access{MaxAge: NoAgeLimit, AllowUnrated: true, LiveTV: true, LibraryChannels: true, TunerChannels: true}
}

// What an account can be kept from doing, apart from what it may watch.
const (
	FeatureGames       = "games"       // play games, here or on a game PC
	FeatureRecordings  = "recordings"  // record Live TV
	FeatureDownloads   = "downloads"   // keep a copy of a file
	FeatureSharing     = "sharing"     // make share links
	FeatureComments    = "comments"    // see comments, and write them
	FeatureRemote      = "remote"      // use the server away from home
	FeatureEditProfile = "editProfile" // change their own name and picture
)

// Features lists them in the order Settings shows them.
var Features = []string{FeatureGames, FeatureRecordings, FeatureDownloads, FeatureSharing, FeatureComments, FeatureRemote, FeatureEditProfile}

// kidsOff is what Kids and Guest profiles start without: their names were
// always for whoever manages them to choose. (Writing comments and making share
// links are for grown-up accounts whatever the limits say; reading comments is
// on until a parent turns it off.)
var kidsOff = []string{FeatureEditProfile}

// Can reports whether the account may use a feature.
func (a *Access) Can(feature string) bool {
	if a == nil {
		return true
	}
	for _, f := range a.Off {
		if f == feature {
			return false
		}
	}
	return true
}

// Limited reports whether the account can't see everything. Someone who is
// limited can't set limits for others: a Kids profile they make with no limits
// and a password they know would be a way around their own.
func (a *Access) Limited() bool {
	if a == nil {
		return false
	}
	return a.byAge() || len(a.Libraries) > 0 || len(a.BlockGenres) > 0 || len(a.BlockItems) > 0 || len(a.BlockShows) > 0 ||
		!a.LiveTV || !a.LibraryChannels || !a.TunerChannels ||
		len(a.OnlyGenres) > 0 || len(a.BlockTags) > 0 || len(a.BlockCollections) > 0 ||
		len(a.Off) > 0 || !a.Time.Empty()
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

func (a *Access) ageOK(age *int, kind string) bool {
	max, unrated := a.limitFor(kind)
	n := -1
	if age != nil {
		n = *age
	}
	return parental.Allows(max, unrated, n)
}

// sqlAge writes the age limit as SQL: age is the column holding a title's
// minimum age, and which says which kind of media a row is ("" when every row
// is of the kind given).
func (a *Access) sqlAge(age, which, kind string) string {
	if len(a.Kinds) == 0 {
		// One limit for everything, as it was before kinds had their own.
		if a.MaxAge < 0 {
			return "1=1"
		}
		ok := age + " <= " + strconv.Itoa(a.MaxAge)
		if a.AllowUnrated {
			ok = "(" + ok + " OR " + age + " IS NULL)"
		}
		return ok
	}
	one := func(k string) (string, string) {
		max, unrated := a.limitFor(k)
		if max < 0 {
			max = 999
		}
		if unrated {
			return strconv.Itoa(max), "1"
		}
		return strconv.Itoa(max), "0"
	}
	if which == "" {
		max, unrated := one(kind)
		return "(" + age + " <= " + max + " OR (" + age + " IS NULL AND " + unrated + " = 1))"
	}
	maxCase, unratedCase := "CASE", "CASE"
	for _, k := range []struct{ when, kind string }{
		{which + " = 'game'", parental.KindGame}, {which + " = 'book'", parental.KindBook},
		{which + " IN ('audio','cd-image','radio')", parental.KindMusic}, {which + " = 'photo'", ""},
		{"i.show_id IS NOT NULL", parental.KindTV},
	} {
		max, unrated := one(k.kind)
		maxCase += " WHEN " + k.when + " THEN " + max
		unratedCase += " WHEN " + k.when + " THEN " + unrated
	}
	max, unrated := one(parental.KindFilm)
	maxCase += " ELSE " + max + " END"
	unratedCase += " ELSE " + unrated + " END"
	return "(" + age + " <= " + maxCase + " OR (" + age + " IS NULL AND " + unratedCase + " = 1))"
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
	cond := []string{a.sqlAge("COALESCE(i.min_age, (SELECT sh.min_age FROM shows sh WHERE sh.id = i.show_id))", "i.kind", "")}
	if len(a.Libraries) > 0 {
		cond = append(cond, "i.library_id IN ("+ids(a.Libraries)+")")
	}
	// An episode counts its show's genres too, as AllowsItem does.
	const genres = "('|' || IFNULL(i.genres,'') || '|' || IFNULL((SELECT sh.genres FROM shows sh WHERE sh.id = i.show_id),'') || '|')"
	for _, g := range a.BlockGenres {
		cond = append(cond, genres+" NOT LIKE ?")
		args = append(args, "%|"+g+"|%")
	}
	if len(a.OnlyGenres) > 0 {
		var any []string
		for _, g := range a.OnlyGenres {
			any = append(any, genres+" LIKE ?")
			args = append(args, "%|"+g+"|%")
		}
		cond = append(cond, "("+strings.Join(any, " OR ")+")")
	}
	for _, t := range a.BlockTags {
		cond = append(cond, "NOT EXISTS (SELECT 1 FROM video_meta vm WHERE vm.item_id = i.id AND ('|' || vm.tags || '|') LIKE ?)")
		args = append(args, "%|"+t+"|%")
	}
	never := "i.id NOT IN (" + ids(a.BlockItems) + ") AND IFNULL(i.show_id, 0) NOT IN (" + ids(a.BlockShows) + ")"
	if !a.Can(FeatureGames) {
		never += " AND i.kind <> 'game'" // an account that may not play games doesn't see them listed
	}
	if len(a.BlockCollections) > 0 {
		c := ids(a.BlockCollections)
		never += " AND i.id NOT IN (SELECT ci.item_id FROM collection_items ci WHERE ci.collection_id IN (" + c + "))" +
			" AND IFNULL(i.show_id, 0) NOT IN (SELECT cs.show_id FROM collection_shows cs WHERE cs.collection_id IN (" + c + "))"
	}
	always := "i.id IN (" + ids(a.AllowItems) + ") OR IFNULL(i.show_id, 0) IN (" + ids(a.AllowShows) + ")"
	if len(a.AllowCollections) > 0 {
		c := ids(a.AllowCollections)
		always += " OR i.id IN (SELECT ci.item_id FROM collection_items ci WHERE ci.collection_id IN (" + c + "))" +
			" OR IFNULL(i.show_id, 0) IN (SELECT cs.show_id FROM collection_shows cs WHERE cs.collection_id IN (" + c + "))"
	}
	return "(" + never + " AND (" + always + " OR (" + strings.Join(cond, " AND ") + ")))", args
}

// ShowClause is the same for shows aliased "s".
func (a *Access) ShowClause() (string, []any) {
	if a == nil {
		return "1=1", nil
	}
	var args []any
	cond := []string{a.sqlAge("s.min_age", "", parental.KindTV)}
	if len(a.Libraries) > 0 {
		cond = append(cond, "s.library_id IN ("+ids(a.Libraries)+")")
	}
	for _, g := range a.BlockGenres {
		cond = append(cond, "('|' || IFNULL(s.genres,'') || '|') NOT LIKE ?")
		args = append(args, "%|"+g+"|%")
	}
	if len(a.OnlyGenres) > 0 {
		var any []string
		for _, g := range a.OnlyGenres {
			any = append(any, "('|' || IFNULL(s.genres,'') || '|') LIKE ?")
			args = append(args, "%|"+g+"|%")
		}
		cond = append(cond, "("+strings.Join(any, " OR ")+")")
	}
	never := "s.id NOT IN (" + ids(a.BlockShows) + ")"
	if len(a.BlockCollections) > 0 {
		never += " AND s.id NOT IN (SELECT cs.show_id FROM collection_shows cs WHERE cs.collection_id IN (" + ids(a.BlockCollections) + "))"
	}
	always := "s.id IN (" + ids(a.AllowShows) + ")"
	if len(a.AllowCollections) > 0 {
		always += " OR s.id IN (SELECT cs.show_id FROM collection_shows cs WHERE cs.collection_id IN (" + ids(a.AllowCollections) + "))"
	}
	return "(" + never + " AND (" + always + " OR (" + strings.Join(cond, " AND ") + ")))", args
}

// AllowsLibrary reports whether the account may open a library at all.
func (a *Access) AllowsLibrary(id int64) bool {
	return a == nil || len(a.Libraries) == 0 || has(a.Libraries, id)
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
	if a.needsSet() && a.set == nil {
		return false // rules nobody looked up can't be said to allow anything
	}
	if has(a.BlockItems, it.ID) || (showID != 0 && has(a.BlockShows, showID)) || a.set.blocks(it.ID, showID) {
		return false
	}
	if it.Kind == "game" && !a.Can(FeatureGames) {
		return false
	}
	if has(a.AllowItems, it.ID) || (showID != 0 && has(a.AllowShows, showID)) || a.set.allows(it.ID, showID) {
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
	if !a.genresOK(genres) || a.set.tagged(it.ID) {
		return false
	}
	return a.ageOK(age, MediaKind(it.Kind, showID != 0))
}

// genresOK: none of the hidden genres, and one of the only ones when there are any.
func (a *Access) genresOK(genres []string) bool {
	for _, g := range a.BlockGenres {
		for _, x := range genres {
			if strings.EqualFold(g, x) {
				return false
			}
		}
	}
	if len(a.OnlyGenres) == 0 {
		return true
	}
	for _, g := range a.OnlyGenres {
		for _, x := range genres {
			if strings.EqualFold(g, x) {
				return true
			}
		}
	}
	return false
}

// AllowsShow checks one show.
func (a *Access) AllowsShow(sh *Show) bool {
	if a == nil {
		return true
	}
	if a.needsSet() && a.set == nil {
		return false
	}
	if has(a.BlockShows, sh.ID) || a.set.blocks(0, sh.ID) {
		return false
	}
	if has(a.AllowShows, sh.ID) || a.set.allows(0, sh.ID) {
		return true
	}
	if len(a.Libraries) > 0 && !has(a.Libraries, sh.LibraryID) {
		return false
	}
	if !a.genresOK(sh.Genres) {
		return false
	}
	return a.ageOK(AgeFor(sh.ContentRating), parental.KindTV)
}
