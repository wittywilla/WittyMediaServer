// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package channels

import (
	"errors"
	"fmt"
	"time"
)

// Broadcast cloning: a channel that follows a real station's schedule with the owner's own
// files. A guide (an XMLTV file, the server's own guide, or a public schedule) is imported
// once, each slot is matched to the library, and what isn't there is filled in by the nearest
// thing that is. The slots become timetable blocks that play one programme each, with
// commercials in the time it leaves, so the channel keeps to the station's clock.
//
// Nothing is recorded or decrypted: only the guide's words are used. See docs/CHANNELS.md,
// "Clone a station".

// How a slot got what it plays, from the best to the least like it.
const (
	RuleExact   = "exact"   // the same episode: season and episode, or a film by title and year
	RuleTitle   = "title"   // the same episode, by its title
	RuleAired   = "aired"   // the same episode, by its first air date
	RuleNear    = "near"    // the same series, the closest episode
	RuleSeries  = "series"  // the same series, another episode
	RuleSimilar = "similar" // a show like it: genre, network, era, rating
	RuleFiller  = "filler"  // whatever fits the slot's length best
	RuleManual  = "manual"  // chosen by hand
	RuleNone    = "none"    // nothing at all: the channel's own rules play
)

// Ad densities: how much of a slot's spare time is spread through the programme.
const (
	DensityLow    = "low"
	DensityNormal = "normal"
	DensityHigh   = "high"
)

// Repeat modes: which days the cloned schedule plays on.
const (
	RepeatAuto   = ""       // one day: every day; more: each on its weekday
	RepeatDaily  = "daily"  // the first cloned day, every day
	RepeatWeekly = "weekly" // each cloned day on its weekday, every week
	RepeatOnce   = "once"   // only on the dates cloned (each year)
)

// Clone is a channel's imported schedule and how it was made.
type Clone struct {
	Source  CloneSource  `json:"source"`
	Station string       `json:"station"`          // the station's name, as the source gives it
	From    string       `json:"from"`             // YYYY-MM-DD: the first day cloned
	Days    int          `json:"days"`             // how many days, 1–7
	Credit  string       `json:"credit,omitempty"` // whose guide it is, for the attribution line
	Options CloneOptions `json:"options"`
	Slots   []CloneSlot  `json:"slots"`
	// ImportedAt is when the guide was read, Unix seconds; RefreshedFrom the first day of the
	// last weekly re-import.
	ImportedAt int64 `json:"importedAt,omitempty"`
}

// CloneSource is where the guide came from, enough to read it again.
type CloneSource struct {
	Kind    string `json:"kind"`              // xmltv | guide | tvmaze
	URL     string `json:"url,omitempty"`     // xmltv: the address ("{key}" stands for the owner's guide key) or "upload:<id>"
	Channel string `json:"channel,omitempty"` // xmltv and guide: the channel id; tvmaze: the network id
	Country string `json:"country,omitempty"` // tvmaze: two letters
	TZ      string `json:"tz,omitempty"`      // the station's own time zone, when the source says
}

// CloneOptions are the advanced choices. Their zero values are the defaults.
type CloneOptions struct {
	Repeat string `json:"repeat,omitempty"` // see Repeat*
	// RealTime plays each programme at the same moment it airs, in this server's time zone.
	// Otherwise (the default) it plays at the same clock time as on the station.
	RealTime bool `json:"realTime,omitempty"`
	ShiftMin int  `json:"shiftMin,omitempty"` // moves everything, -720 to 720
	// Which stand-ins may be used. All are allowed by default.
	NoNear    bool `json:"noNear,omitempty"`
	NoSeries  bool `json:"noSeries,omitempty"`
	NoSimilar bool `json:"noSimilar,omitempty"`
	NoFiller  bool `json:"noFiller,omitempty"`
	// Libraries limits what may play to these; none = every library but commercials.
	Libraries []int64 `json:"libraries,omitempty"`
	// Commercials are on unless AdsOff. Density says how many breaks; NoEra stops preferring
	// commercials from the programme's own years.
	AdsOff  bool   `json:"adsOff,omitempty"`
	Density string `json:"density,omitempty"`
	NoEra   bool   `json:"noEra,omitempty"`
	// Refresh reads the guide again each week, so the clone follows the station.
	Refresh bool `json:"refresh,omitempty"`
}

// CloneSlot is one programme in the imported guide and what plays for it.
type CloneSlot struct {
	Start int64  `json:"start"` // Unix seconds, as the guide gives it
	Dur   int    `json:"dur"`   // seconds
	Wall  string `json:"wall"`  // "2006-01-02 15:04" on the station's own clock
	Title string `json:"title"`
	Sub   string `json:"sub,omitempty"`
	// What the guide says about it, for matching.
	Season  *int     `json:"season,omitempty"`
	Episode *int     `json:"episode,omitempty"`
	Year    int      `json:"year,omitempty"`
	Aired   string   `json:"aired,omitempty"` // YYYY-MM-DD the episode first aired
	Genres  []string `json:"genres,omitempty"`
	Network string   `json:"network,omitempty"`
	Rating  string   `json:"rating,omitempty"`
	Movie   bool     `json:"movie,omitempty"`
	IMDb    string   `json:"imdb,omitempty"`
	TVDB    string   `json:"tvdb,omitempty"`
	TVmaze  string   `json:"tvmaze,omitempty"`
	TMDb    string   `json:"tmdb,omitempty"`
	Gap     bool     `json:"gap,omitempty"` // the guide had nothing here
	// The match.
	Item       int64  `json:"item,omitempty"`
	Rule       string `json:"rule"`
	Confidence int    `json:"confidence"` // 0–100
	Why        string `json:"why,omitempty"`
	ItemTitle  string `json:"itemTitle,omitempty"` // what plays, for the preview
	ItemSub    string `json:"itemSub,omitempty"`
	ItemDur    int    `json:"itemDur,omitempty"`
	Show       int64  `json:"show,omitempty"`   // the series of what plays, if any
	Pinned     bool   `json:"pinned,omitempty"` // chosen by hand: a re-import keeps it
}

// Limits on what one clone may hold.
const (
	maxCloneSlots = 1500
	maxCloneDays  = 7
)

// Exact says whether a rule found the programme itself.
func Exact(rule string) bool {
	switch rule {
	case RuleExact, RuleTitle, RuleAired, RuleManual:
		return true
	}
	return false
}

func (c *Clone) validate() error {
	switch c.Source.Kind {
	case "xmltv", "guide", "tvmaze":
	default:
		return errors.New("unknown guide source")
	}
	if len(c.Source.URL) > 2000 || len(c.Source.Channel) > 200 || len(c.Station) > maxTextLen || len(c.Credit) > 500 || len(c.Source.TZ) > 64 {
		return errors.New("the guide source is too long")
	}
	if c.Days < 1 || c.Days > maxCloneDays {
		return fmt.Errorf("clone from 1 to %d days", maxCloneDays)
	}
	if _, err := time.Parse("2006-01-02", c.From); err != nil {
		return errors.New("the first day must look like 2026-11-23")
	}
	o := &c.Options
	switch o.Repeat {
	case RepeatAuto, RepeatDaily, RepeatWeekly, RepeatOnce:
	default:
		return errors.New("unknown repeat")
	}
	switch o.Density {
	case "", DensityLow, DensityNormal, DensityHigh:
	default:
		return errors.New("unknown commercial density")
	}
	if o.ShiftMin < -720 || o.ShiftMin > 720 {
		return errors.New("shift by at most 12 hours")
	}
	if len(o.Libraries) > maxPickLibs {
		return errors.New("too many libraries")
	}
	if len(c.Slots) > maxCloneSlots {
		return errors.New("too many programmes in the guide")
	}
	for i := range c.Slots {
		s := &c.Slots[i]
		if s.Dur <= 0 || s.Dur > 24*3600 {
			return fmt.Errorf("programme %d: bad length", i+1)
		}
		if _, err := time.Parse("2006-01-02 15:04", s.Wall); err != nil {
			return fmt.Errorf("programme %d: bad time", i+1)
		}
		if len(s.Title) > maxTextLen || len(s.Sub) > maxTextLen || len(s.Why) > maxTextLen || len(s.ItemTitle) > maxTextLen || len(s.ItemSub) > maxTextLen || len(s.Genres) > 20 {
			return fmt.Errorf("programme %d: too long", i+1)
		}
		if s.Item < 0 || s.Confidence < 0 || s.Confidence > 100 {
			return fmt.Errorf("programme %d: bad match", i+1)
		}
	}
	return nil
}

// repeat is the mode actually used.
func (c *Clone) repeat() string {
	if c.Options.Repeat != RepeatAuto {
		return c.Options.Repeat
	}
	if c.Days <= 1 {
		return RepeatDaily
	}
	return RepeatWeekly
}

// slotTime is when a slot plays on this server's clock.
func (c *Clone) slotTime(s *CloneSlot, loc *time.Location) time.Time {
	var t time.Time
	if c.Options.RealTime {
		t = time.Unix(s.Start, 0).In(loc)
	} else {
		w, _ := time.ParseInLocation("2006-01-02 15:04", s.Wall, loc)
		t = w
	}
	return t.Add(time.Duration(c.Options.ShiftMin) * time.Minute)
}

// CompileClone turns a cloned schedule into timetable blocks, one a programme, replacing any
// blocks the definition had. It changes nothing else the owner set.
func (d *Definition) CompileClone(loc *time.Location) {
	c := d.Clone
	if c == nil {
		return
	}
	d.Mode = "clone"
	d.AlignMin = 0
	d.Blocks = d.Blocks[:0]
	repeat := c.repeat()
	first := ""
	if len(c.Slots) > 0 {
		first = c.Slots[0].Wall[:10]
	}
	for i := range c.Slots {
		s := &c.Slots[i]
		// Daily: the first cloned day, every day. A programme of it that runs past midnight
		// is still that day's.
		if repeat == RepeatDaily && s.Wall[:10] != first {
			continue
		}
		at := c.slotTime(s, loc)
		mins := (s.Dur + 30) / 60
		if mins < 1 {
			continue
		}
		mins = min(mins, 24*60-1)
		end := at.Add(time.Duration(mins) * time.Minute)
		b := Block{
			ID:    fmt.Sprintf("clone-%d", i+1),
			Name:  clip(s.Title, maxTextLen),
			Start: at.Format("15:04"),
			End:   end.Format("15:04"),
			Fit:   "cut",
			Once:  true,
		}
		if b.Start == b.End {
			continue
		}
		switch repeat {
		case RepeatWeekly:
			b.Days = []int{int(at.Weekday())}
		case RepeatOnce:
			b.Days = []int{int(at.Weekday())}
			b.FromDate, b.ToDate = at.Format("01-02"), at.Format("01-02")
		}
		switch {
		case s.Item > 0 && s.Show > 0 && !Exact(s.Rule) && longSlot(s):
			// A stand-in far shorter than its slot: its series plays on, an episode after
			// another, rather than leave the rest of the slot to commercials.
			b.Rules = &Rules{Shows: []int64{s.Show}}
			b.Order = &Order{Mode: "sequential"}
			b.Once, b.Fit = false, "fill"
		case s.Item > 0:
			b.Rules = &Rules{Items: []int64{s.Item}}
			b.Order = &Order{Mode: "sequential"}
		}
		b.StandIn = !Exact(s.Rule) && s.Item > 0
		d.Blocks = append(d.Blocks, b)
		if len(d.Blocks) >= maxBlocks {
			break
		}
	}
	if len(c.Options.Libraries) > 0 {
		d.Rules.Libraries = c.Options.Libraries
	}
	if c.Options.AdsOff {
		d.Filler.BreakEveryMin, d.Filler.BreakAds = 0, 0
	}
}

// longSlot: what plays leaves at least a quarter of an hour, and a third of the slot, unfilled.
func longSlot(s *CloneSlot) bool {
	spare := s.Dur - s.ItemDur
	return s.ItemDur > 0 && spare >= 15*60 && spare*3 >= s.Dur
}

// breaksPerHalfHour is how many breaks a density puts in each half hour of a slot.
func breaksPerHalfHour(density string) int {
	switch density {
	case DensityLow:
		return 1
	case DensityHigh:
		return 3
	}
	return 2
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8Start(s[n]) {
		n--
	}
	return s[:n]
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
