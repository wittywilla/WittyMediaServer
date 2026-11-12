// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/channels"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/livetv"
	"github.com/wittywilla/wittymediaserver/internal/tuner/xmltv"
)

// WittyTV's More info (docs/plans/DESIGN-2.md, section 9): one programme in full, for the pop-up
// the guide, the watch banner and Recordings open over themselves. What it shows comes from the
// guide for a tuner channel and from the library for a library channel; cast and crew are the
// same Credits the item page shows (session 12's model), from the library when the title is
// there and the profile may see it, else from the guide's own credits. Where else it airs is this
// server's channels, among those the profile may watch.

func (s *Server) tvProgrammeRoutes(m *http.ServeMux) {
	m.Handle("GET /api/tv/{key}/programme", s.requireUser(s.handleTVProgramme))
}

type programmeChannel struct {
	Key         string        `json:"key"`
	Kind        string        `json:"kind"`
	Number      string        `json:"number"`
	Name        string        `json:"name"`
	Logo        string        `json:"logo,omitempty"`
	Brand       *channelBrand `json:"brand,omitempty"`
	CatchupDays int           `json:"catchupDays,omitempty"`
}

type programmeScore struct {
	Value  string `json:"value"`
	System string `json:"system,omitempty"` // who gave it; "library" for the library's own score
}

// programmeFacts is the programme itself, as the pop-up's header and More show it.
type programmeFacts struct {
	Start        int64            `json:"start"`
	End          int64            `json:"end"`
	Kind         string           `json:"kind"` // broadcast | show | movie | slate…: as the guide says
	Title        string           `json:"title"`
	Sub          string           `json:"sub,omitempty"`
	Desc         string           `json:"desc,omitempty"`
	Genres       []string         `json:"genres,omitempty"`
	Rating       string           `json:"rating,omitempty"` // the age rating: TV-PG, 12…
	Year         int              `json:"year,omitempty"`
	Season       *int             `json:"season,omitempty"`
	Episode      *int             `json:"episode,omitempty"`
	LengthMin    int              `json:"lengthMin"`
	Scores       []programmeScore `json:"scores,omitempty"`
	Studio       string           `json:"studio,omitempty"` // a film's studio, or a show's network
	Language     string           `json:"language,omitempty"`
	OrigLanguage string           `json:"origLanguage,omitempty"`
	Country      string           `json:"country,omitempty"`
	Subtitles    []string         `json:"subtitles,omitempty"`
	SignLanguage bool             `json:"signLanguage,omitempty"`
	Audio        string           `json:"audio,omitempty"`
	HD           bool             `json:"hd,omitempty"`
	Premiere     bool             `json:"premiere,omitempty"`
	New          bool             `json:"new,omitempty"`
	Repeat       bool             `json:"repeat,omitempty"`
	Keywords     []string         `json:"keywords,omitempty"`
}

// programmeActions is what the pop-up may offer: only what this profile can do with it now.
type programmeActions struct {
	Watch      bool `json:"watch"`      // it's on now, and the channel plays
	Record     bool `json:"record"`     // canRecord, a broadcast, not over
	Remind     bool `json:"remind"`     // it's later
	Reminded   bool `json:"reminded"`   // a reminder is already set
	FromStart  bool `json:"fromStart"`  // catch-up: it has started, within the source's archive
	WatchLater bool `json:"watchLater"` // there's a library title to put on Watch later
	InList     bool `json:"inList"`     // and it's on it already
	Open       bool `json:"open"`       // there's a library page to open
}

type programmeTrailer struct {
	ItemID      int64   `json:"itemId"`
	Title       string  `json:"title"`
	Type        string  `json:"type"` // trailer, featurette, behind-the-scenes, interview…
	DurationSec float64 `json:"durationSec"`
}

type programmeAiring struct {
	Key     string `json:"key"`
	Number  string `json:"number"`
	Name    string `json:"name"`
	Start   int64  `json:"start"`
	End     int64  `json:"end"`
	Sub     string `json:"sub,omitempty"`
	Season  *int   `json:"season,omitempty"`
	Episode *int   `json:"episode,omitempty"`
	This    bool   `json:"this,omitempty"` // the one asked about
}

type programmeLibrary struct {
	ItemID int64 `json:"itemId,omitempty"`
	ShowID int64 `json:"showId,omitempty"`
}

type programmeInfo struct {
	Channel   programmeChannel   `json:"channel"`
	Programme programmeFacts     `json:"programme"`
	Actions   programmeActions   `json:"actions"`
	Library   *programmeLibrary  `json:"library,omitempty"`
	Credits   *library.Credits   `json:"credits"`
	Trailers  []programmeTrailer `json:"trailers"`
	Schedule  []programmeAiring  `json:"schedule"`
	// IDs name the title elsewhere, for GET /discover/where-to-watch when it isn't in the library.
	IDs map[string]string `json:"ids,omitempty"`
}

const (
	scheduleDays  = 14
	scheduleLimit = 20
	maxTrailers   = 10
)

var errProgrammeHidden = errors.New("what is on then is not available to this profile")

// GET /api/tv/{key}/programme?start=<a moment in it>: one programme in full.
func (s *Server) handleTVProgramme(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	kind, id, ok := parseKey(key)
	if !ok {
		writeErr(w, 400, "bad channel")
		return
	}
	at, err := strconv.ParseInt(r.URL.Query().Get("start"), 10, 64)
	now := time.Now()
	if err != nil || at <= 0 {
		writeErr(w, 400, "start is a moment in the programme, in seconds since 1970")
		return
	}
	if at < now.AddDate(0, 0, -31).Unix() || at > now.AddDate(0, 0, 15).Unix() {
		writeErr(w, 404, "the guide has no programme then")
		return
	}
	if !s.canWatchLive(r, kind, id) {
		writeErr(w, 404, "channel not found")
		return
	}
	var info *programmeInfo
	if kind == "tun" {
		info, err = s.tunerProgrammeInfo(r, id, at, now)
	} else {
		info, err = s.libraryProgrammeInfo(r, id, at, now)
	}
	switch {
	case errors.Is(err, livetv.ErrNoProgramme), errors.Is(err, livetv.ErrNotFound), errors.Is(err, library.ErrNotFound):
		writeErr(w, 404, "the guide has no programme then")
		return
	case errors.Is(err, errProgrammeHidden):
		writeErr(w, 403, err.Error())
		return
	case err != nil:
		writeErr(w, 500, err.Error())
		return
	}
	// The lists are always lists, so an app can draw an empty tab without asking first.
	if info.Trailers == nil {
		info.Trailers = []programmeTrailer{}
	}
	if info.Schedule == nil {
		info.Schedule = []programmeAiring{}
	}
	writeJSON(w, 200, info)
}

func (s *Server) tunerProgrammeInfo(r *http.Request, id, at int64, now time.Time) (*programmeInfo, error) {
	ctx := r.Context()
	c, err := s.TV.Channel(ctx, id)
	if err != nil {
		return nil, err
	}
	p, d, err := s.TV.ProgrammeAt(ctx, c, at)
	if err != nil {
		return nil, err
	}
	logo, brand := tunerLogo(c)
	info := &programmeInfo{
		Channel: programmeChannel{Key: "tun-" + strconv.FormatInt(c.ID, 10), Kind: "tuner", Number: c.Number, Name: c.Name, Logo: logo, Brand: brand, CatchupDays: c.CatchupDays},
		Programme: programmeFacts{Start: p.Start, End: p.End, Kind: "broadcast", Title: p.Title, Sub: p.Sub, Desc: p.Desc, Rating: p.Rating,
			Season: p.Season, Episode: p.Episode, Year: d.Year, LengthMin: d.LengthMin, Language: d.Language, OrigLanguage: d.OrigLanguage,
			Country: d.Country, Subtitles: d.Subtitles, SignLanguage: d.SignLanguage, Audio: d.Audio, HD: d.HD || c.HD, Premiere: d.Premiere,
			New: d.New, Repeat: d.Repeat, Keywords: d.Keywords},
	}
	if p.Category != "" {
		info.Programme.Genres = append([]string{p.Category}, d.Categories...)
	}
	for _, st := range d.Stars {
		info.Programme.Scores = append(info.Programme.Scores, programmeScore(st))
	}
	if info.Programme.LengthMin == 0 {
		info.Programme.LengthMin = int((p.End - p.Start) / 60)
	}
	if d.IMDb != "" || d.TMDb != "" {
		info.IDs = map[string]string{}
		if d.IMDb != "" {
			info.IDs["imdb"] = d.IMDb
		}
		if d.TMDb != "" {
			info.IDs["tmdb"] = d.TMDb
		}
	}
	nowS := now.Unix()
	info.Actions = programmeActions{
		Watch:     p.Start <= nowS && p.End > nowS && c.Encrypted == "",
		Record:    s.canRecord(r, c) && p.End > nowS,
		Remind:    p.Start > nowS,
		FromStart: c.CatchupDays > 0 && p.Start <= nowS && p.Start >= nowS-int64(c.CatchupDays)*86400 && c.Encrypted == "",
	}
	info.Actions.Reminded = s.reminded(ctx, r, info.Channel.Key, p.Start)
	// The same title in the library, when the profile may see it: its credits, trailers and page.
	if lib := s.libraryMatch(r, p.Title, d.Year, p.Season != nil); lib != nil {
		s.fillFromLibrary(r, info, lib)
	}
	if info.Credits == nil {
		info.Credits = guideCredits(d.Credits)
	}
	info.Schedule = s.programmeSchedule(r, p.Title, info.Channel.Key, p.Start, now)
	return info, nil
}

func (s *Server) libraryProgrammeInfo(r *http.Request, id, at int64, now time.Time) (*programmeInfo, error) {
	ctx := r.Context()
	c, err := s.visibleChannel(r, id)
	if err != nil {
		return nil, err
	}
	list, err := s.Channels.Window(ctx, c, time.Unix(at, 0).Add(-12*time.Hour), time.Unix(at, 0).Add(time.Hour))
	if err != nil {
		return nil, err
	}
	var p *channels.Programme
	for _, x := range channels.GuideCells(list) {
		if x.Start <= at && x.End > at {
			x := x
			p = &x
			break
		}
	}
	if p == nil {
		return nil, livetv.ErrNoProgramme
	}
	key := "lib-" + strconv.FormatInt(c.ID, 10)
	g := fromSchedule(*p)
	info := &programmeInfo{
		Channel: programmeChannel{Key: key, Kind: "library", Number: c.Number, Name: c.Name, Brand: brandOf(c)},
		Programme: programmeFacts{Start: p.Start, End: p.End, Kind: p.Kind, Title: g.Title, Sub: p.Sub, Rating: p.Rating, Year: p.Year,
			Season: p.Season, Episode: p.Episode, LengthMin: int((p.End - p.Start) / 60)},
	}
	if c.Logo != "" {
		info.Channel.Logo = "/api/channels/" + strconv.FormatInt(c.ID, 10) + "/logo?v=" + c.Logo
	}
	if p.Genre != "" {
		info.Programme.Genres = []string{p.Genre}
	}
	nowS := now.Unix()
	info.Actions = programmeActions{Watch: p.Start <= nowS && p.End > nowS, Remind: p.Start > nowS}
	info.Actions.Reminded = s.reminded(ctx, r, key, p.Start)
	if p.ItemID != 0 {
		it, err := s.visibleItem(r, p.ItemID)
		if errors.Is(err, library.ErrNotFound) {
			return nil, errProgrammeHidden
		} else if err != nil {
			return nil, err
		}
		lib := &programmeLibrary{ItemID: it.ID}
		if it.ShowID != nil {
			lib.ShowID = *it.ShowID
		}
		s.fillFromLibrary(r, info, lib)
	}
	if info.Credits == nil {
		info.Credits = &library.Credits{Cast: []library.Credit{}, Guests: []library.Credit{}, Crew: []library.CrewGroup{}}
	}
	info.Schedule = s.programmeSchedule(r, p.Title, key, p.Start, now)
	return info, nil
}

// reminded says whether this account has a reminder for that programme.
func (s *Server) reminded(ctx context.Context, r *http.Request, key string, start int64) bool {
	list, err := s.Channels.Reminders(ctx, userFrom(r).ID, start-1)
	if err != nil {
		return false
	}
	for _, x := range list {
		if x.Channel == key && x.Start == start {
			return true
		}
	}
	return false
}

// libraryMatch finds a guide title in the library among what the profile may see: a show by its
// name, or else a film by its name and, when the guide gives one, its year.
func (s *Server) libraryMatch(r *http.Request, title string, year int, episode bool) *programmeLibrary {
	ctx := r.Context()
	title = strings.TrimSpace(title)
	if title == "" {
		return nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM shows WHERE missing = 0 AND title = ? COLLATE NOCASE ORDER BY id LIMIT 5`, title)
	if err == nil {
		var ids []int64
		for rows.Next() {
			var id int64
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}
		rows.Close()
		for _, id := range ids {
			if _, err := s.visibleShow(r, id); err == nil {
				return &programmeLibrary{ShowID: id}
			}
		}
	}
	if episode {
		return nil
	}
	q := `SELECT id FROM items WHERE missing = 0 AND show_id IS NULL AND title = ? COLLATE NOCASE`
	args := []any{title}
	if year > 0 {
		q += ` AND (year IS NULL OR year = ?)`
		args = append(args, year)
	}
	rows, err = s.DB.QueryContext(ctx, q+` AND id NOT IN (SELECT item_id FROM item_links) ORDER BY id LIMIT 5`, args...)
	if err != nil {
		return nil
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		if _, err := s.visibleItem(r, id); err == nil {
			return &programmeLibrary{ItemID: id}
		}
	}
	return nil
}

// fillFromLibrary adds what the library knows: the description, facts and score where the guide
// had none, the item page's credits, its trailers and bonus features, and Watch later.
func (s *Server) fillFromLibrary(r *http.Request, info *programmeInfo, lib *programmeLibrary) {
	ctx := r.Context()
	u := userFrom(r)
	f := &info.Programme
	info.Library = lib
	info.Actions.Open, info.Actions.WatchLater = true, true
	if in, err := s.Store.InWatchLater(ctx, u.ID, lib.ItemID, lib.ShowID); err == nil {
		info.Actions.InList = in
	}
	var overview, rating, studio string
	var genres []string
	var score *float64
	var year *int
	if lib.ItemID != 0 {
		if it, err := s.visibleItem(r, lib.ItemID); err == nil {
			overview, rating, studio, genres, score, year = it.Overview, it.ContentRating, it.Studio, it.Genres, it.Rating, it.Year
			if c, err := s.Store.ItemCredits(ctx, it); err == nil && !c.Empty() {
				info.Credits = c
			}
		}
	}
	if lib.ShowID != 0 {
		if sh, err := s.visibleShow(r, lib.ShowID); err == nil {
			if overview == "" {
				overview = sh.Overview
			}
			rating, studio = firstNonEmpty(rating, sh.ContentRating), firstNonEmpty(studio, sh.Network)
			if len(genres) == 0 {
				genres = sh.Genres
			}
			if score == nil {
				score = sh.Rating
			}
			if year == nil {
				year = sh.Year
			}
			if info.Credits == nil {
				season := -1
				if f.Season != nil {
					season = *f.Season
				}
				if c, err := s.Store.ShowCredits(ctx, lib.ShowID, season, s.access(r)); err == nil && !c.Empty() {
					info.Credits = c
				}
			}
		}
	}
	if f.Desc == "" {
		f.Desc = overview
	}
	if f.Rating == "" {
		f.Rating = rating
	}
	if f.Studio == "" {
		f.Studio = studio
	}
	if len(f.Genres) == 0 {
		f.Genres = genres
	}
	if f.Year == 0 && year != nil {
		f.Year = *year
	}
	if score != nil {
		f.Scores = append(f.Scores, programmeScore{Value: strconv.FormatFloat(*score, 'f', 1, 64) + "/10", System: "library"})
	}
	info.Trailers = s.libraryExtras(r, lib)
}

// libraryExtras are the trailers and other bonus features attached to a title, trailers first,
// each one the profile may see.
func (s *Server) libraryExtras(r *http.Request, lib *programmeLibrary) []programmeTrailer {
	out := []programmeTrailer{}
	rows, err := s.DB.QueryContext(r.Context(), `SELECT item_id, extra_type, label FROM item_links
		WHERE link = ? AND ((parent_item IS NOT NULL AND parent_item = ?) OR (parent_show IS NOT NULL AND parent_show = ?))
		ORDER BY extra_type != 'trailer', item_id LIMIT ?`, library.LinkExtra, lib.ItemID, lib.ShowID, maxTrailers*2)
	if err != nil {
		return out
	}
	type extra struct {
		id          int64
		kind, label string
	}
	var found []extra
	for rows.Next() {
		var e extra
		if rows.Scan(&e.id, &e.kind, &e.label) == nil {
			found = append(found, e)
		}
	}
	rows.Close()
	for _, e := range found {
		if len(out) >= maxTrailers {
			break
		}
		it, err := s.visibleItem(r, e.id)
		if err != nil {
			continue
		}
		out = append(out, programmeTrailer{ItemID: it.ID, Title: firstNonEmpty(e.label, it.Title), Type: firstNonEmpty(e.kind, "other"), DurationSec: it.RuntimeSec})
	}
	return out
}

// programmeSchedule is where and when the title airs from now on, on channels the profile may
// watch, with the one asked about marked: the pop-up's Schedule, and its Also on.
func (s *Server) programmeSchedule(r *http.Request, title, key string, start int64, now time.Time) []programmeAiring {
	out := []programmeAiring{}
	if s.TV == nil || !s.access(r).CanWatchChannels("tuner") {
		return out
	}
	list, err := s.TV.Airings(r.Context(), title, now.Add(-12*time.Hour), now.AddDate(0, 0, scheduleDays), scheduleLimit)
	if err != nil {
		return out
	}
	names := map[int64]*livetv.Channel{}
	for _, a := range list {
		c, ok := names[a.ChannelID]
		if !ok {
			c, _ = s.TV.Channel(r.Context(), a.ChannelID)
			names[a.ChannelID] = c
		}
		if c == nil || a.End <= now.Unix() && "tun-"+strconv.FormatInt(c.ID, 10) != key {
			continue
		}
		k := "tun-" + strconv.FormatInt(c.ID, 10)
		out = append(out, programmeAiring{Key: k, Number: c.Number, Name: c.Name, Start: a.Start, End: a.End, Sub: a.Sub,
			Season: a.Season, Episode: a.Episode, This: k == key && a.Start <= start && a.End > start})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

// guideCredits are a guide's own credits as the item page's Credits: the cast with their parts,
// guests, and the crew by department. Nobody here has a page or a picture.
func guideCredits(list []xmltv.Credit) *library.Credits {
	c := &library.Credits{Cast: []library.Credit{}, Guests: []library.Credit{}, Crew: []library.CrewGroup{}}
	depts := map[string]int{}
	add := func(dept, job, name string) {
		i, ok := depts[dept]
		if !ok {
			i = len(c.Crew)
			depts[dept] = i
			label := "Crew"
			for _, d := range library.Departments {
				if d.Key == dept {
					label = d.Label
				}
			}
			c.Crew = append(c.Crew, library.CrewGroup{Department: dept, Label: label})
		}
		g := &c.Crew[i]
		g.People = append(g.People, library.Credit{Name: name, Role: "crew", Department: dept, Job: job, Order: len(g.People), Source: "guide"})
	}
	for _, x := range list {
		switch x.Role {
		case "actor", "presenter", "commentator", "guest":
			cr := library.Credit{Name: x.Name, Role: "cast", Character: x.Character, Source: "guide"}
			if x.Guest || x.Role == "guest" {
				cr.Role, cr.Order = "guest", len(c.Guests)
				c.Guests = append(c.Guests, cr)
				continue
			}
			if x.Role != "actor" && cr.Character == "" {
				cr.Character = map[string]string{"presenter": "Presenter", "commentator": "Commentator"}[x.Role]
			}
			cr.Order = len(c.Cast)
			c.Cast = append(c.Cast, cr)
		case "director":
			add("directing", "Director", x.Name)
		case "writer", "adapter":
			add("writing", map[string]string{"writer": "Writer", "adapter": "Adaptation"}[x.Role], x.Name)
		case "producer":
			add("production", "Producer", x.Name)
		case "composer":
			add("sound", "Composer", x.Name)
		case "editor":
			add("editing", "Editor", x.Name)
		}
	}
	// Departments in the order the item page lists them.
	order := map[string]int{}
	for i, d := range library.Departments {
		order[d.Key] = i
	}
	sort.SliceStable(c.Crew, func(i, j int) bool { return order[c.Crew[i].Department] < order[c.Crew[j].Department] })
	return c
}

// programmeCredits are a WittyTV programme's credits, for the credits source's programme: a
// channel's key and a moment in it ("tun-12@1767225600"). The same answer as /programme's.
func (s *Server) programmeCredits(r *http.Request, ref string) (*library.Credits, error) {
	key, at, ok := strings.Cut(ref, "@")
	kind, id, kok := parseKey(key)
	start, err := strconv.ParseInt(at, 10, 64)
	if !ok || !kok || err != nil || !s.canWatchLive(r, kind, id) {
		return nil, livetv.ErrNoProgramme
	}
	now := time.Now()
	var info *programmeInfo
	if kind == "tun" {
		info, err = s.tunerProgrammeInfo(r, id, start, now)
	} else {
		info, err = s.libraryProgrammeInfo(r, id, start, now)
	}
	if err != nil {
		return nil, err
	}
	return info.Credits, nil
}
