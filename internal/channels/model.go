// Package channels builds always-on TV channels out of the libraries: what a
// channel may play (rules), when (blocks), and the schedule that follows.
//
// Schedules are made a day at a time, only when something asks for that day,
// and are deterministic: the same definition, seed and library give the same
// day. See docs/CHANNELS.md.
package channels

import (
	"errors"
	"fmt"
	"regexp"
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
	ExcludeItems    []int64  `json:"excludeItems,omitempty"`
	SeasonFrom      int      `json:"seasonFrom,omitempty"`
	SeasonTo        int      `json:"seasonTo,omitempty"`
	IncludeSpecials bool     `json:"includeSpecials,omitempty"`
	Watched         string   `json:"watched,omitempty"`   // "" | watched | unwatched
	WatchedBy       int64    `json:"watchedBy,omitempty"` // whose watch history; channels are the same for everyone
	MinMinutes      int      `json:"minMinutes,omitempty"`
	MaxMinutes      int      `json:"maxMinutes,omitempty"`
	Search          string   `json:"search,omitempty"`
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
}

// Filler is what plays between programmes: the owner's own files.
type Filler struct {
	Rules *Rules `json:"rules,omitempty"`
	// Between is how many bumpers play after each programme.
	Between int `json:"between,omitempty"`
	// MaxSeconds keeps longer files out of the filler (0 = no limit).
	MaxSeconds int `json:"maxSeconds,omitempty"`
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
)

const (
	maxListLen = 2000
	maxBlocks  = 2000
	maxTextLen = 200
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
	}
	if d.Filler.Rules != nil {
		if err := d.Filler.Rules.validate(); err != nil {
			return fmt.Errorf("filler: %w", err)
		}
	}
	if d.Filler.Between < 0 || d.Filler.Between > 5 || d.Filler.MaxSeconds < 0 {
		return errors.New("filler: between 0 and 5 bumpers")
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
	for _, l := range [][]int64{r.Libraries, r.Shows, r.ExcludeShows, r.Items, r.ExcludeItems} {
		if len(l) > maxListLen {
			return errors.New("too many choices in a rule")
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
