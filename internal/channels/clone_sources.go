// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package channels

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/tuner/xmltv"
)

// Reading a station's schedule from the sources a clone can use. Fetching is the server's job
// (it knows the owner's key and which addresses may be reached); these only read what came back.

// Station is one channel a guide has.
type Station struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Country string `json:"country,omitempty"`
	TZ      string `json:"tz,omitempty"`
	Count   int    `json:"count,omitempty"` // programmes in the days asked for
}

// Window is the days a clone reads: from midnight of From, on the station's clock, for Days.
type Window struct {
	From time.Time
	Days int
}

func (w Window) contains(t time.Time) bool {
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	from := time.Date(w.From.Year(), w.From.Month(), w.From.Day(), 0, 0, 0, 0, time.UTC)
	return !day.Before(from) && day.Before(from.AddDate(0, 0, w.Days))
}

// maxStations bounds a station list; guides of whole countries name thousands.
const maxStations = 5000

// XMLTVStations lists a guide's channels with how many programmes each has in the window.
func XMLTVStations(r io.Reader, w Window) ([]Station, error) {
	byID := map[string]*Station{}
	var order []string
	err := xmltv.Parse(r, func(c xmltv.Channel) bool {
		if _, ok := byID[c.ID]; !ok && len(order) < maxStations {
			name := c.ID
			if len(c.Names) > 0 {
				name = c.Names[0]
			}
			byID[c.ID] = &Station{ID: c.ID, Name: name}
			order = append(order, c.ID)
		}
		return true
	}, func(p xmltv.Programme) bool {
		if s := byID[p.Channel]; s != nil && w.contains(p.Start) {
			s.Count++
		}
		return true
	})
	out := make([]Station, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, err
}

// XMLTVSlots reads one channel's programmes in the window. The guide's own offsets give the
// station's clock.
func XMLTVSlots(r io.Reader, channel string, w Window) ([]CloneSlot, error) {
	var out []CloneSlot
	err := xmltv.Parse(r, nil, func(p xmltv.Programme) bool {
		if p.Channel != channel || !w.contains(p.Start) || !p.Stop.After(p.Start) {
			return true
		}
		s := CloneSlot{Start: p.Start.Unix(), Dur: int(p.Stop.Sub(p.Start).Seconds()), Wall: p.Start.Format("2006-01-02 15:04"),
			Title: p.Title, Sub: p.Sub, Season: p.Season, Episode: p.Episode, Year: p.Year, Rating: p.Rating,
			IMDb: p.Details.IMDb, TMDb: p.Details.TMDb}
		s.Genres = append(s.Genres, p.Details.Categories...)
		if len(s.Genres) == 0 && p.Category != "" {
			s.Genres = []string{p.Category}
		}
		s.Movie = isMovie(s.Genres) || strings.HasPrefix(p.Details.TMDb, "movie/")
		if s.Year == 0 {
			s.Year = p.Details.Year
		}
		out = append(out, s)
		return len(out) < maxCloneSlots*2
	})
	return Tidy(out), err
}

func isMovie(genres []string) bool {
	for _, g := range genres {
		switch strings.ToLower(g) {
		case "movie", "movies", "film", "feature film", "tvmovie", "tv movie":
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// The server's own guide (from tuners and IPTV playlists)
// ---------------------------------------------------------------------------

// GuideStations lists the channels the server's guide has, with how many programmes each has in
// the window.
func (s *Store) GuideStations(ctx context.Context, from, to time.Time) ([]Station, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT e.epg_id, MIN(e.name), (SELECT COUNT(*) FROM epg_programmes p WHERE p.channel = e.epg_id AND p.start >= ? AND p.start < ?)
		FROM epg_channels e GROUP BY e.epg_id ORDER BY MIN(e.name) COLLATE NOCASE LIMIT ?`, from.Unix(), to.Unix(), maxStations)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Station{}
	for rows.Next() {
		var st Station
		if err := rows.Scan(&st.ID, &st.Name, &st.Count); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// GuideSlots reads one channel of the server's guide, on this server's clock.
func (s *Store) GuideSlots(ctx context.Context, channel string, from, to time.Time) ([]CloneSlot, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT start, stop, title, sub, category, rating, season, episode, extra FROM epg_programmes
		WHERE channel = ? AND start >= ? AND start < ? ORDER BY start LIMIT ?`, channel, from.Unix(), to.Unix(), maxCloneSlots*2)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CloneSlot
	loc := s.loc()
	for rows.Next() {
		var start, stop int64
		var cs CloneSlot
		var cat, extra string
		var season, episode sql.NullInt64
		if err := rows.Scan(&start, &stop, &cs.Title, &cs.Sub, &cat, &cs.Rating, &season, &episode, &extra); err != nil {
			return nil, err
		}
		if stop <= start {
			continue
		}
		cs.Start, cs.Dur, cs.Wall = start, int(stop-start), time.Unix(start, 0).In(loc).Format("2006-01-02 15:04")
		if season.Valid {
			v := int(season.Int64)
			cs.Season = &v
		}
		if episode.Valid {
			v := int(episode.Int64)
			cs.Episode = &v
		}
		var d xmltv.Details
		if extra != "" && json.Unmarshal([]byte(extra), &d) == nil {
			cs.Genres, cs.Year, cs.IMDb, cs.TMDb = d.Categories, d.Year, d.IMDb, d.TMDb
		}
		if len(cs.Genres) == 0 && cat != "" {
			cs.Genres = []string{cat}
		}
		cs.Movie = isMovie(cs.Genres) || strings.HasPrefix(cs.TMDb, "movie/")
		out = append(out, cs)
	}
	return Tidy(out), rows.Err()
}

// ---------------------------------------------------------------------------
// TVmaze's public schedule (api.tvmaze.com/schedule: no key, CC BY-SA)
// ---------------------------------------------------------------------------

// TVmazeCredit is the attribution its licence asks for.
const TVmazeCredit = "Schedule data from TVmaze (tvmaze.com), CC BY-SA 4.0"

// TVmazeEntry is one episode in TVmaze's schedule.
type TVmazeEntry struct {
	Name     string `json:"name"`
	Season   *int   `json:"season"`
	Number   *int   `json:"number"`
	Airdate  string `json:"airdate"`
	Airtime  string `json:"airtime"`
	Airstamp string `json:"airstamp"`
	Runtime  *int   `json:"runtime"`
	Show     struct {
		ID        int      `json:"id"`
		Name      string   `json:"name"`
		Type      string   `json:"type"`
		Genres    []string `json:"genres"`
		Premiered string   `json:"premiered"`
		Runtime   *int     `json:"runtime"`
		Network   *struct {
			ID      int    `json:"id"`
			Name    string `json:"name"`
			Country *struct {
				Code     string `json:"code"`
				Timezone string `json:"timezone"`
			} `json:"country"`
		} `json:"network"`
		Externals struct {
			TheTVDB *int   `json:"thetvdb"`
			IMDb    string `json:"imdb"`
		} `json:"externals"`
	} `json:"show"`
}

// ParseTVmaze reads one day of /schedule.
func ParseTVmaze(body []byte) ([]TVmazeEntry, error) {
	var list []TVmazeEntry
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, errors.New("TVmaze sent something that isn't a schedule")
	}
	return list, nil
}

// TVmazeStations lists the networks in a day of TVmaze's schedule.
func TVmazeStations(days ...[]TVmazeEntry) []Station {
	byID := map[int]*Station{}
	for _, list := range days {
		for i := range list {
			n := list[i].Show.Network
			if n == nil {
				continue
			}
			st := byID[n.ID]
			if st == nil {
				st = &Station{ID: strconv.Itoa(n.ID), Name: n.Name}
				if n.Country != nil {
					st.Country, st.TZ = n.Country.Code, n.Country.Timezone
				}
				byID[n.ID] = st
			}
			st.Count++
		}
	}
	out := make([]Station, 0, len(byID))
	for _, st := range byID {
		out = append(out, *st)
	}
	slices.SortFunc(out, func(a, b Station) int { return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return out
}

// TVmazeSlots is one network's programmes from days of TVmaze's schedule.
func TVmazeSlots(network string, days ...[]TVmazeEntry) []CloneSlot {
	var out []CloneSlot
	for _, list := range days {
		for i := range list {
			e := &list[i]
			n := e.Show.Network
			if n == nil || strconv.Itoa(n.ID) != network {
				continue
			}
			at, err := time.Parse(time.RFC3339, e.Airstamp)
			if err != nil {
				continue
			}
			mins := 30
			switch {
			case e.Runtime != nil && *e.Runtime > 0:
				mins = *e.Runtime
			case e.Show.Runtime != nil && *e.Show.Runtime > 0:
				mins = *e.Show.Runtime
			}
			wall := e.Airdate + " " + e.Airtime
			if _, err := time.Parse("2006-01-02 15:04", wall); err != nil {
				wall = at.Format("2006-01-02 15:04")
			}
			s := CloneSlot{Start: at.Unix(), Dur: mins * 60, Wall: wall, Title: e.Show.Name, Sub: e.Name,
				Season: e.Season, Episode: e.Number, Aired: e.Airdate, Genres: e.Show.Genres, Network: n.Name,
				IMDb: e.Show.Externals.IMDb, TVmaze: strconv.Itoa(e.Show.ID)}
			if e.Show.Externals.TheTVDB != nil {
				s.TVDB = strconv.Itoa(*e.Show.Externals.TheTVDB)
			}
			if len(e.Show.Premiered) >= 4 {
				s.Year, _ = strconv.Atoi(e.Show.Premiered[:4])
			}
			out = append(out, s)
		}
	}
	return Tidy(out)
}

// ---------------------------------------------------------------------------
// Tidying a guide
// ---------------------------------------------------------------------------

const (
	minSlotSec = 90       // shorter entries join the one before
	minGapSec  = 5 * 60   // shorter gaps go to the programme before
	maxGapSec  = 6 * 3600 // a guide silent this long is not a gap to fill but the end of it
)

// Tidy puts a guide in order: overlaps cut, very short entries joined to the one before, short
// gaps closed and longer ones marked, so that the slots run end to end.
func Tidy(in []CloneSlot) []CloneSlot {
	slices.SortStableFunc(in, func(a, b CloneSlot) int { return cmp.Compare(a.Start, b.Start) })
	var out []CloneSlot
	for _, s := range in {
		if n := len(out); n > 0 {
			prev := &out[n-1]
			end := prev.Start + int64(prev.Dur)
			if s.Start == prev.Start && s.Dur >= minSlotSec && prev.Dur >= minSlotSec {
				// Two at once: an hour-long premiere listed as its two episodes. The second
				// follows the first.
				s.Start, s.Wall = end, wallAfter(prev.Wall, prev.Dur)
			}
			if s.Start < end {
				// Overlap: the earlier one stops where this starts.
				prev.Dur = int(s.Start - prev.Start)
				if prev.Dur < minSlotSec {
					out = out[:n-1]
				}
			} else if gap := s.Start - end; gap > 0 && gap < minGapSec {
				prev.Dur += int(gap)
			} else if gap >= minGapSec && gap <= maxGapSec {
				wall := wallAfter(prev.Wall, int(end-prev.Start))
				out = append(out, CloneSlot{Start: end, Dur: int(gap), Wall: wall, Gap: true})
			}
		}
		if s.Dur < minSlotSec {
			if n := len(out); n > 0 && out[n-1].Start+int64(out[n-1].Dur) >= s.Start {
				out[n-1].Dur += max(0, int(s.Start+int64(s.Dur)-(out[n-1].Start+int64(out[n-1].Dur))))
			}
			continue
		}
		out = append(out, s)
		if len(out) >= maxCloneSlots {
			break
		}
	}
	return out
}

// wallAfter moves a station clock time on by secs.
func wallAfter(wall string, secs int) string {
	t, err := time.Parse("2006-01-02 15:04", wall)
	if err != nil {
		return wall
	}
	return t.Add(time.Duration(secs) * time.Second).Format("2006-01-02 15:04")
}

// ---------------------------------------------------------------------------
// The library, as a clone sees it
// ---------------------------------------------------------------------------

// maxCloneTitles bounds what one import loads, so a large library stays light.
const maxCloneTitles = 60000

// CloneLibrary loads what a clone may play: every playable series episode and title, from the
// given libraries or, with none, from all but commercials.
func (s *Store) CloneLibrary(ctx context.Context, libs []int64) (*CloneLibrary, error) {
	where := playable
	var args []any
	if len(libs) > 0 {
		where += " AND i.library_id IN (" + placeholders(len(libs)) + ")"
		args = anyArgs(libs)
	} else {
		where += " AND " + notCommercial
	}
	where += " AND (i.show_id IS NULL OR IFNULL(i.season, 1) <> 0)"
	rows, err := s.DB.QueryContext(ctx, `SELECT i.id, IFNULL(i.show_id, 0), i.title, i.season, i.episode, IFNULL(i.aired, ''), IFNULL(i.runtime_sec, 0),
		IFNULL(i.year, 0), IFNULL(i.genres, ''), IFNULL(i.content_rating, ''), IFNULL(i.studio, ''), IFNULL(i.provider, ''), IFNULL(i.provider_id, ''),
		IFNULL(s.title, ''), IFNULL(s.year, 0), IFNULL(s.network, ''), IFNULL(s.genres, ''), IFNULL(s.content_rating, ''), IFNULL(s.provider, ''), IFNULL(s.provider_id, '')
		FROM items i LEFT JOIN shows s ON s.id = i.show_id WHERE `+where+`
		ORDER BY i.show_id IS NULL, s.sort_title COLLATE NOCASE, s.id, IFNULL(i.season, 0), IFNULL(i.episode, 0), i.sort_title COLLATE NOCASE, i.id LIMIT `+strconv.Itoa(maxCloneTitles), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lib := &CloneLibrary{}
	shows := map[int64]*LibShow{}
	for rows.Next() {
		var id, showID int64
		var title, aired, genres, rating, studio, prov, provID string
		var sTitle, sNet, sGenres, sRating, sProv, sProvID string
		var season, episode sql.NullInt64
		var runtime float64
		var year, sYear int
		if err := rows.Scan(&id, &showID, &title, &season, &episode, &aired, &runtime, &year, &genres, &rating, &studio, &prov, &provID,
			&sTitle, &sYear, &sNet, &sGenres, &sRating, &sProv, &sProvID); err != nil {
			return nil, err
		}
		dur := int(runtime + 0.999)
		if dur < 30 {
			continue
		}
		if showID == 0 {
			lib.Titles = append(lib.Titles, &LibTitle{Item: id, Title: title, Year: year, Genres: splitGenres(genres), Rating: rating, Studio: studio, Provider: prov, ProviderID: provID, Dur: dur})
			continue
		}
		sh := shows[showID]
		if sh == nil {
			sh = &LibShow{ID: showID, Title: sTitle, Year: sYear, Network: sNet, Genres: splitGenres(sGenres), Rating: sRating, Provider: sProv, ProviderID: sProvID}
			shows[showID] = sh
			lib.Shows = append(lib.Shows, sh)
		}
		e := LibEpisode{Item: id, Title: title, Season: -1, Aired: aired, Dur: dur}
		if season.Valid {
			e.Season = int(season.Int64)
		}
		if episode.Valid {
			e.Episode = int(episode.Int64)
		}
		sh.Episodes = append(sh.Episodes, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	lib.Prepare()
	return lib, nil
}

func splitGenres(s string) []string {
	var out []string
	for _, g := range strings.Split(s, "|") {
		if g = strings.TrimSpace(g); g != "" {
			out = append(out, g)
		}
	}
	return out
}

// CommercialLibraries lists the commercials libraries, which a new clone uses by default.
func (s *Store) CommercialLibraries(ctx context.Context) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM libraries WHERE type = ? ORDER BY id`, library.CommercialsType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// CarryPins keeps what was chosen by hand in an earlier import, for the same programme at the
// same time of the same weekday in a new one.
func CarryPins(old, fresh []CloneSlot) {
	key := func(s *CloneSlot) string {
		t, err := time.Parse("2006-01-02 15:04", s.Wall)
		if err != nil {
			return ""
		}
		return t.Weekday().String() + t.Format("15:04") + normTitle(s.Title)
	}
	pins := map[string]*CloneSlot{}
	for i := range old {
		if old[i].Pinned && old[i].Item > 0 {
			pins[key(&old[i])] = &old[i]
		}
	}
	for i := range fresh {
		if p := pins[key(&fresh[i])]; p != nil {
			fresh[i].Item, fresh[i].Rule, fresh[i].Confidence, fresh[i].Why, fresh[i].Pinned = p.Item, p.Rule, p.Confidence, p.Why, true
			fresh[i].ItemTitle, fresh[i].ItemSub, fresh[i].ItemDur = p.ItemTitle, p.ItemSub, p.ItemDur
		}
	}
}
