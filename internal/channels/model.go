// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package channels builds always-on TV channels out of the libraries: what a
// channel may play (rules), when (blocks), and the schedule that follows.
//
// Schedules are made a day at a time, only when something asks for that day,
// and are deterministic: the same definition, seed and library give the same
// day. See docs/CHANNELS.md.
package channels

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Definition is everything an owner chose for a channel. Simple, Rules and
// Full timetable are views of this one structure, so moving between them
// loses nothing.
type Definition struct {
	Mode   string  `json:"mode"` // simple | rules | timetable
	Rules  Rules   `json:"rules"`
	Order  Order   `json:"order"`
	Blocks []Block `json:"blocks"`
	Filler Filler  `json:"filler"`
	// AlignMin snaps programme starts to a grid of this many minutes
	// (0 = play back to back); the gaps get filler or a slate.
	AlignMin int `json:"alignMin"`
	// Bumpers are what plays around programmes, and what may give way to keep
	// to the timetable.
	Bumpers BumperPlan `json:"bumpers,omitzero"`
}

// Rules choose titles. Empty fields do not limit anything; lists mean "any of".
type Rules struct {
	Libraries       []int64  `json:"libraries,omitempty"`
	Kinds           []string `json:"kinds,omitempty"` // episodes | movies | discs
	Genres          []string `json:"genres,omitempty"`
	ExcludeGenres   []string `json:"excludeGenres,omitempty"`
	Networks        []string `json:"networks,omitempty"` // a show's network, or a film's studio
	Studios         []string `json:"studios,omitempty"`
	YearFrom        int      `json:"yearFrom,omitempty"` // by air date, else the release year
	YearTo          int      `json:"yearTo,omitempty"`
	Decades         []int    `json:"decades,omitempty"` // 1990 = the 1990s
	MaxAge          *int     `json:"maxAge,omitempty"`  // highest minimum age allowed
	AllowUnrated    bool     `json:"allowUnrated,omitempty"`
	Ratings         []string `json:"ratings,omitempty"`  // certificates: TV-PG, PG-13…
	MinScore        float64  `json:"minScore,omitempty"` // 0–10
	Shows           []int64  `json:"shows,omitempty"`
	ExcludeShows    []int64  `json:"excludeShows,omitempty"`
	Items           []int64  `json:"items,omitempty"` // hand-picked titles: a collection of the owner's own
	Collections     []int64  `json:"collections,omitempty"`
	ExcludeItems    []int64  `json:"excludeItems,omitempty"`
	SeasonFrom      int      `json:"seasonFrom,omitempty"`
	SeasonTo        int      `json:"seasonTo,omitempty"`
	IncludeSpecials bool     `json:"includeSpecials,omitempty"`
	Watched         string   `json:"watched,omitempty"`   // "" | watched | unwatched
	WatchedBy       int64    `json:"watchedBy,omitempty"` // whose watch history; channels are the same for everyone
	MinMinutes      int      `json:"minMinutes,omitempty"`
	MaxMinutes      int      `json:"maxMinutes,omitempty"`
	Search          string   `json:"search,omitempty"`
	// Clips are parts of discs, each a title of its own in the pool. With no
	// other rules they are all that plays; otherwise they add to what the
	// rules choose.
	Clips []DiscClip `json:"clips,omitempty"`
}

// DiscClip is a disc's main feature, or a scene from any of its titles.
type DiscClip struct {
	ID    string  `json:"id"`
	Item  int64   `json:"item"`
	Title int     `json:"title,omitempty"` // the disc's title number; 0 = its main feature
	Start float64 `json:"start,omitempty"` // seconds into the title
	End   float64 `json:"end,omitempty"`   // 0 = to the end of the title
	// The chapters it was picked from, for showing it again in the editor.
	FromChapter int    `json:"fromChapter,omitempty"`
	ToChapter   int    `json:"toChapter,omitempty"`
	Label       string `json:"label,omitempty"`
}

// onlyClips: nothing but clips was chosen, so nothing else may play.
func (r *Rules) onlyClips() bool {
	if len(r.Clips) == 0 {
		return false
	}
	c := *r
	c.Clips = nil
	b, _ := json.Marshal(c)
	return string(b) == "{}"
}

// clipItems lists the discs clips come from.
func (r *Rules) clipItems() []int64 {
	var out []int64
	for _, c := range r.Clips {
		if !slices.Contains(out, c.Item) {
			out = append(out, c.Item)
		}
	}
	return out
}

// Order is how a pool plays.
type Order struct {
	Mode string `json:"mode"` // rotate (default) | sequential | aired | shuffle
	// KeepEpisodeOrder: when shuffling, shows are shuffled but each show's
	// episodes stay in order.
	KeepEpisodeOrder bool `json:"keepEpisodeOrder,omitempty"`
}

// Block is a day part or a timetable slot. Without Rules it plays the
// channel's; End at or before Start runs past midnight.
type Block struct {
	ID    string `json:"id"`
	Name  string `json:"name,omitempty"`
	Days  []int  `json:"days,omitempty"` // 0 = Sunday … 6; none = every day
	Start string `json:"start"`          // HH:MM
	End   string `json:"end"`            // HH:MM
	Rules *Rules `json:"rules,omitempty"`
	Order *Order `json:"order,omitempty"`
	Fit   string `json:"fit,omitempty"` // fill (default) | overrun | cut
	// FromDate and ToDate limit it to part of every year, as MM-DD: a holiday
	// season. ToDate before FromDate runs over New Year. Empty = all year.
	FromDate string `json:"fromDate,omitempty"`
	ToDate   string `json:"toDate,omitempty"`
	// Marathon: programmes run straight on, with no commercials, no optional
	// bumpers and no waiting for the time grid.
	Marathon bool `json:"marathon,omitempty"`
}

// BumperPlan is one channel's bumpers and how it keeps to its timetable.
type BumperPlan struct {
	Rules []BumperRule `json:"rules,omitempty"`
	Trim  Trim         `json:"trim,omitzero"`
}

// When a bumper rule plays.
const (
	WhenBefore  = "before"  // before a programme
	WhenAfter   = "after"   // after a programme
	WhenBetween = "between" // between commercials in a break
	WhenEvery   = "every"   // a break inside a programme, every N minutes
)

// BumperRule says which short videos play around which programmes.
type BumperRule struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	Off  bool   `json:"off,omitempty"` // kept, but not used
	When string `json:"when"`
	// EveryMin: for WhenEvery, the minutes of programme between breaks.
	EveryMin int `json:"everyMin,omitempty"`
	// Source is what may play. Any of it: the videos take turns.
	Source Rules `json:"source"`
	// Count is how many play each time (default 1).
	Count int `json:"count,omitempty"`
	// Mandatory bumpers are never cut or dropped to keep to the timetable.
	Mandatory bool `json:"mandatory,omitempty"`
	// Only limits the rule to programmes these rules choose: particular films,
	// shows, genres or collections. Nil = every programme.
	Only *Rules `json:"only,omitempty"`
}

// What may give way when a programme runs long.
const (
	TrimCut  = "cut"  // shortened, then left out when too little would remain
	TrimDrop = "drop" // left out whole, never shortened
	TrimKeep = "keep" // always plays in full
)

// Trim is "cut commercials short": what may shrink so that the next programme
// starts on time. Mandatory bumpers and programmes are safe unless TailSec says.
type Trim struct {
	On      bool   `json:"on,omitempty"`
	Ads     string `json:"ads,omitempty"`     // cut (default) | drop | keep
	Bumpers string `json:"bumpers,omitempty"` // optional bumpers: drop (default) | cut | keep
	// MinKeepSec: a shortened video with less than this left is left out (default 5).
	MinKeepSec int `json:"minKeepSec,omitempty"`
	// TailSec: a programme may lose up to this much from its end, usually the
	// credits, when that is what it takes to start the next one on time.
	TailSec int `json:"tailSec,omitempty"`
}

func (t Trim) adsPolicy() string {
	if t.Ads == "" {
		return TrimCut
	}
	return t.Ads
}

func (t Trim) bumperPolicy() string {
	if t.Bumpers == "" {
		return TrimDrop
	}
	return t.Bumpers
}

func (t Trim) minKeep() int64 {
	if t.MinKeepSec <= 0 {
		return 5
	}
	return int64(t.MinKeepSec)
}

// Filler is what plays between programmes: the owner's own files.
type Filler struct {
	Rules *Rules `json:"rules,omitempty"`
	// Between is how many bumpers play after each programme.
	Between int `json:"between,omitempty"`
	// MaxSeconds keeps longer files out of the filler (0 = no limit).
	MaxSeconds int `json:"maxSeconds,omitempty"`
	// BreakEveryMin puts a commercial break inside a programme every so many
	// minutes (0 = none), of BreakAds commercials. They give way like any others.
	BreakEveryMin int `json:"breakEveryMin,omitempty"`
	BreakAds      int `json:"breakAds,omitempty"`
	// Commercials are libraries of commercials this channel uses: any number
	// of them, each whole or only the videos chosen here.
	Commercials []CommercialPick `json:"commercials,omitempty"`
}

// CommercialPick is one commercials library on one channel.
type CommercialPick struct {
	Library int64 `json:"library"`
	// Only: just the Items below, rather than the whole library.
	Only  bool    `json:"only,omitempty"`
	Items []int64 `json:"items,omitempty"`
}

// rules are what this pick chooses. ok is false when it chooses nothing.
func (c CommercialPick) rules() (r Rules, ok bool) {
	r = Rules{Libraries: []int64{c.Library}}
	if c.Only {
		if len(c.Items) == 0 {
			return r, false
		}
		r.Items = c.Items
	}
	return r, true
}

// Visibility says which profiles see a channel, on top of their own limits.
type Visibility struct {
	Mode  string  `json:"mode"` // "" = everyone | adults | only
	Users []int64 `json:"users,omitempty"`
}

var (
	hhmm    = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	orders  = map[string]bool{"": true, "rotate": true, "sequential": true, "aired": true, "shuffle": true}
	kinds   = map[string]bool{"episodes": true, "movies": true, "discs": true}
	aligns  = map[int]bool{0: true, 5: true, 10: true, 15: true, 30: true, 60: true}
	fits    = map[string]bool{"": true, "fill": true, "overrun": true, "cut": true}
	watched = map[string]bool{"": true, "watched": true, "unwatched": true}
	whens   = map[string]bool{WhenBefore: true, WhenAfter: true, WhenBetween: true, WhenEvery: true}
	trims   = map[string]bool{"": true, TrimCut: true, TrimDrop: true, TrimKeep: true}
	mmdd    = regexp.MustCompile(`^(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])$`)
)

const (
	maxListLen    = 2000
	maxBlocks     = 2000
	maxTextLen    = 200
	maxBumperRule = 100
	maxBumperEach = 5
	maxTailSec    = 600
	minClipSec    = 3
)

// Validate refuses anything the editor could not have produced, with a
// message that says what to fix.
func (d *Definition) Validate() error {
	switch d.Mode {
	case "", "simple", "rules", "timetable":
	default:
		return errors.New("unknown channel mode")
	}
	if err := d.Rules.validate(); err != nil {
		return err
	}
	if !orders[d.Order.Mode] {
		return errors.New("unknown order")
	}
	if !aligns[d.AlignMin] {
		return errors.New("start times can snap to 5, 10, 15, 30 or 60 minutes")
	}
	if len(d.Blocks) > maxBlocks {
		return errors.New("too many blocks")
	}
	for i := range d.Blocks {
		b := &d.Blocks[i]
		if !hhmm.MatchString(b.Start) || !hhmm.MatchString(b.End) {
			return fmt.Errorf("block %d: times must look like 18:30", i+1)
		}
		if b.Start == b.End && len(b.Days) > 0 && len(b.Days) < 7 {
			return fmt.Errorf("block %d: a block cannot start and end at the same time", i+1)
		}
		for _, n := range b.Days {
			if n < 0 || n > 6 {
				return fmt.Errorf("block %d: bad day", i+1)
			}
		}
		if len(b.Name) > maxTextLen || len(b.ID) > 64 {
			return fmt.Errorf("block %d: name too long", i+1)
		}
		if !fits[b.Fit] {
			return fmt.Errorf("block %d: unknown fit", i+1)
		}
		if b.Rules != nil {
			if err := b.Rules.validate(); err != nil {
				return fmt.Errorf("block %d: %w", i+1, err)
			}
		}
		if b.Order != nil && !orders[b.Order.Mode] {
			return fmt.Errorf("block %d: unknown order", i+1)
		}
		if (b.FromDate == "") != (b.ToDate == "") {
			return fmt.Errorf("block %d: a season needs a first and a last date", i+1)
		}
		if b.FromDate != "" && (!mmdd.MatchString(b.FromDate) || !mmdd.MatchString(b.ToDate)) {
			return fmt.Errorf("block %d: dates must look like 12-24", i+1)
		}
	}
	if err := d.Bumpers.validate(); err != nil {
		return err
	}
	if d.Filler.Rules != nil {
		if err := d.Filler.Rules.validate(); err != nil {
			return fmt.Errorf("filler: %w", err)
		}
	}
	if d.Filler.Between < 0 || d.Filler.Between > 5 || d.Filler.MaxSeconds < 0 {
		return errors.New("filler: between 0 and 5 bumpers")
	}
	if d.Filler.BreakEveryMin != 0 && (d.Filler.BreakEveryMin < 5 || d.Filler.BreakEveryMin > 240) {
		return errors.New("commercial breaks can come every 5 to 240 minutes")
	}
	if d.Filler.BreakAds < 0 || d.Filler.BreakAds > 5 {
		return errors.New("from 0 to 5 commercials in a break")
	}
	if len(d.Filler.Commercials) > 100 {
		return errors.New("too many commercials libraries")
	}
	for _, c := range d.Filler.Commercials {
		if len(c.Items) > maxListLen {
			return errors.New("too many commercials chosen")
		}
	}
	return nil
}

func (p *BumperPlan) validate() error {
	if len(p.Rules) > maxBumperRule {
		return errors.New("too many bumper rules")
	}
	for i := range p.Rules {
		b := &p.Rules[i]
		if !whens[b.When] {
			return fmt.Errorf("bumper rule %d: unknown time", i+1)
		}
		if b.When == WhenEvery && (b.EveryMin < 5 || b.EveryMin > 240) {
			return fmt.Errorf("bumper rule %d: breaks can come every 5 to 240 minutes", i+1)
		}
		if b.Count < 0 || b.Count > maxBumperEach {
			return fmt.Errorf("bumper rule %d: from 1 to %d bumpers at a time", i+1, maxBumperEach)
		}
		if len(b.ID) > 64 || len(b.Name) > maxTextLen {
			return fmt.Errorf("bumper rule %d: name too long", i+1)
		}
		if err := b.Source.validate(); err != nil {
			return fmt.Errorf("bumper rule %d: %w", i+1, err)
		}
		if b.Only != nil {
			if err := b.Only.validate(); err != nil {
				return fmt.Errorf("bumper rule %d: %w", i+1, err)
			}
		}
	}
	t := &p.Trim
	if !trims[t.Ads] || !trims[t.Bumpers] {
		return errors.New("unknown trimming choice")
	}
	if t.MinKeepSec < 0 || t.MinKeepSec > 300 {
		return errors.New("keep from 0 to 300 seconds of a shortened video")
	}
	if t.TailSec < 0 || t.TailSec > maxTailSec {
		return fmt.Errorf("a programme can lose at most %d minutes from its end", maxTailSec/60)
	}
	return nil
}

func (r *Rules) validate() error {
	for _, l := range [][]string{r.Kinds, r.Genres, r.ExcludeGenres, r.Networks, r.Studios, r.Ratings} {
		if len(l) > maxListLen {
			return errors.New("too many choices in a rule")
		}
		for _, s := range l {
			if len(s) > maxTextLen {
				return errors.New("a rule's text is too long")
			}
		}
	}
	for _, l := range [][]int64{r.Libraries, r.Shows, r.ExcludeShows, r.Items, r.ExcludeItems, r.Collections} {
		if len(l) > maxListLen {
			return errors.New("too many choices in a rule")
		}
	}
	if len(r.Clips) > maxListLen {
		return errors.New("too many scenes")
	}
	for i := range r.Clips {
		c := &r.Clips[i]
		if c.Item <= 0 || c.Title < 0 || c.Start < 0 || c.End < 0 || c.FromChapter < 0 || c.ToChapter < 0 {
			return errors.New("that scene is not valid")
		}
		if c.End != 0 && c.End-c.Start < minClipSec {
			return fmt.Errorf("a scene must run for at least %d seconds", minClipSec)
		}
		if len(c.ID) > 64 || len(c.Label) > maxTextLen {
			return errors.New("a scene's name is too long")
		}
	}
	for _, k := range r.Kinds {
		if !kinds[k] {
			return errors.New("unknown kind of title")
		}
	}
	if !watched[r.Watched] {
		return errors.New("unknown watched state")
	}
	if len(r.Search) > maxTextLen || len(r.Decades) > 32 {
		return errors.New("a rule is too long")
	}
	for _, y := range append([]int{r.YearFrom, r.YearTo}, r.Decades...) {
		if y != 0 && (y < 1880 || y > 2200) {
			return errors.New("years must be between 1880 and 2200")
		}
	}
	if r.YearFrom != 0 && r.YearTo != 0 && r.YearFrom > r.YearTo {
		return errors.New("the first year is after the last")
	}
	if r.SeasonFrom < 0 || r.SeasonTo < 0 || (r.SeasonTo != 0 && r.SeasonFrom > r.SeasonTo) {
		return errors.New("the season range is the wrong way round")
	}
	if r.MinMinutes < 0 || r.MaxMinutes < 0 || (r.MaxMinutes != 0 && r.MinMinutes > r.MaxMinutes) {
		return errors.New("the length range is the wrong way round")
	}
	if r.MinScore < 0 || r.MinScore > 10 {
		return errors.New("scores go from 0 to 10")
	}
	return nil
}

func (v *Visibility) validate() error {
	switch v.Mode {
	case "", "adults":
	case "only":
		if len(v.Users) > maxListLen {
			return errors.New("too many profiles")
		}
	default:
		return errors.New("unknown visibility")
	}
	return nil
}

// minutes turns HH:MM (already validated) into minutes after midnight.
func minutes(s string) int {
	if len(s) != 5 {
		return 0
	}
	h, _ := strconv.Atoi(s[:2])
	m, _ := strconv.Atoi(s[3:])
	return h*60 + m
}

// NormalizeNetwork gives network names one spelling for comparing them.
func NormalizeNetwork(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
