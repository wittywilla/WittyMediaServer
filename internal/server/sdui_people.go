// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// The pages' cast and crew (docs/SDUI.md "Credits"): the credits source, a person's titles
// (filmography), people in search, the person a person's page is about, and the parts of their
// work it has a row for. Every one is read as the REST routes read them (api_people.go): the
// title first, through the account's limits; a person only when they are in something the
// account may see.

// creditGroup is the SDUI group a credit is in: cast, guests or the department's key.
func creditGroup(c library.Credit) string {
	switch c.Role {
	case library.RoleCast:
		return "cast"
	case library.RoleGuest:
		return "guests"
	}
	return c.Department
}

// creditItem is a person card.
func creditItem(c library.Credit, group string) sdui.Item {
	id := itoa(c.PersonID)
	it := sdui.Item{ID: "person-" + id + "-" + group, Kind: "person", Title: c.Name, Group: group, Route: "/person/" + id, Images: map[string]string{}}
	switch {
	case c.Role == library.RoleCrew:
		it.Subtitle = c.Job
	case c.Character != "":
		it.Subtitle = c.Character
	}
	if c.Episodes > 1 {
		it.Subtitle = strings.TrimPrefix(it.Subtitle+" · "+strconv.Itoa(c.Episodes)+" episodes", " · ")
	}
	if c.HasPhoto {
		it.Images["square"] = "/api/people/" + id + "/photo"
	}
	return it
}

// credits answers the credits source: the title's cast, guest stars and crew, in that order.
func (rv *sduiResolver) credits(src sdui.Source, req sdui.SourceRequest) (sdui.SourceResult, error) {
	s := rv.s
	var c *library.Credits
	var err error
	if ref := src.Str("programme"); ref != "" {
		// A WittyTV programme's: see programmeCredits (api_tvprogramme.go).
		if c, err = s.programmeCredits(rv.r, ref); err != nil {
			return sdui.SourceResult{}, nil
		}
	} else if ref := src.Str("item"); ref != "" && !strings.HasPrefix(ref, "show-") {
		id, perr := strconv.ParseInt(ref, 10, 64)
		if perr != nil {
			return sdui.SourceResult{}, nil
		}
		it, verr := s.visibleItem(rv.r, id)
		if verr != nil {
			return sdui.SourceResult{}, nil
		}
		c, err = s.Store.ItemCredits(rv.ctx, it)
	} else {
		ref = firstNonEmpty(src.Str("show"), ref)
		id, perr := strconv.ParseInt(strings.TrimPrefix(ref, "show-"), 10, 64)
		if perr != nil {
			return sdui.SourceResult{}, nil
		}
		if _, verr := s.visibleShow(rv.r, id); verr != nil {
			return sdui.SourceResult{}, nil
		}
		season := -1
		if v := src.Str("season"); v != "" {
			if n, perr := strconv.Atoi(v); perr == nil && n >= 0 {
				season = n
			}
		}
		c, err = s.Store.ShowCredits(rv.ctx, id, season, rv.acc)
	}
	if err != nil {
		return sdui.SourceResult{}, err
	}
	part := src.Str("part")
	var out []sdui.Item
	add := func(group string, list []library.Credit) {
		if part != "" && part != "all" && part != group {
			return
		}
		for _, x := range list {
			out = append(out, creditItem(x, group))
		}
	}
	add("cast", c.Cast)
	add("guests", c.Guests)
	for _, g := range c.Crew {
		add(g.Department, g.People)
	}
	return page(out, req), nil
}

// filmography answers a person's titles: one part of their work, or all of it, newest first.
func (rv *sduiResolver) filmography(src sdui.Source, req sdui.SourceRequest) (sdui.SourceResult, error) {
	id, err := strconv.ParseInt(strings.TrimPrefix(src.Str("person"), "person-"), 10, 64)
	if err != nil || !rv.s.Store.PersonVisible(rv.ctx, id, rv.acc) {
		return sdui.SourceResult{}, nil
	}
	films, err := rv.s.Store.Filmography(rv.ctx, id, rv.acc)
	if err != nil {
		return sdui.SourceResult{}, err
	}
	part := src.Str("part")
	type entry struct {
		t   library.FilmTitle
		job bool
	}
	var list []entry
	seen := map[string]bool{}
	for _, g := range films.Groups {
		if part != "" && part != "all" && part != g.Key {
			continue
		}
		for _, t := range g.Titles {
			k := t.Target + itoa(t.ID)
			if seen[k] {
				continue // all of their work: each title once, in its first part
			}
			seen[k] = true
			list = append(list, entry{t, g.Key != library.FilmActing})
		}
	}
	window := list[min(req.Offset, len(list)):]
	more := len(window) > req.Limit
	if more {
		window = window[:req.Limit]
	}
	out := make([]sdui.Item, 0, len(window))
	for _, e := range window {
		var it sdui.Item
		if e.t.Target == "show" {
			sh, err := rv.s.Store.ShowFacts(rv.ctx, e.t.ID)
			if err != nil || !rv.acc.AllowsShow(sh) {
				continue
			}
			it = rv.show(sh)
		} else {
			raw, err := rv.s.visibleItem(rv.r, e.t.ID)
			if err != nil {
				continue
			}
			it = rv.item(raw)
		}
		var line []string
		if e.t.Year > 0 {
			line = append(line, strconv.Itoa(e.t.Year))
		}
		switch {
		case e.job && e.t.Job != "":
			line = append(line, e.t.Job)
		case e.t.Character != "":
			line = append(line, "as "+e.t.Character)
		}
		if e.t.Episodes > 0 {
			noun := " episodes"
			if e.t.Episodes == 1 {
				noun = " episode"
			}
			line = append(line, strconv.Itoa(e.t.Episodes)+noun)
		}
		it.Subtitle = strings.Join(line, " · ")
		out = append(out, it)
	}
	return sdui.SourceResult{Items: out, More: more, Total: len(list)}, nil
}

// searchPeople answers the search source's people.
func (rv *sduiResolver) searchPeople(term string, req sdui.SourceRequest) (sdui.SourceResult, error) {
	found, err := rv.s.Store.SearchPeople(rv.ctx, term, min(req.Offset+req.Limit+1, 100), rv.acc)
	if err != nil {
		return sdui.SourceResult{}, err
	}
	out := make([]sdui.Item, 0, len(found))
	for _, p := range found {
		id := itoa(p.ID)
		it := sdui.Item{ID: "person-" + id, Kind: "person", Title: p.Name, Route: "/person/" + id, Images: map[string]string{}}
		noun := " titles"
		if p.Titles == 1 {
			noun = " title"
		}
		it.Subtitle = strings.TrimPrefix(knownForLabel(p.KnownFor)+" · "+strconv.Itoa(p.Titles)+noun, " · ")
		if p.HasPhoto {
			it.Images["square"] = "/api/people/" + id + "/photo"
		}
		out = append(out, it)
	}
	return page(out, req), nil
}

// knownForLabel names what a person is known for: Acting, or a department.
func knownForLabel(key string) string {
	if key == "" {
		return ""
	}
	for _, o := range sdui.PersonParts {
		if o.Value == key {
			return o.Label
		}
	}
	return ""
}

// personSubject is the person a person's page is about.
func (rv *sduiResolver) personSubject(ref string) *sdui.Item {
	id, err := strconv.ParseInt(strings.TrimPrefix(ref, "person-"), 10, 64)
	if err != nil {
		return nil
	}
	p, err := rv.s.visiblePerson(rv.r, id)
	if err != nil {
		return nil
	}
	p = rv.s.personFresh(rv.r, p)
	sid := itoa(p.ID)
	it := sdui.Item{ID: "person-" + sid, Kind: "person", Title: p.Name, Overview: p.Biography, Route: "/person/" + sid, Images: map[string]string{},
		KnownFor: knownForLabel(p.KnownFor), Born: lifeDate(p.Birthday, p.Birthplace), Died: lifeDate(p.Deathday, "")}
	if p.HasPhoto {
		it.Images["poster"] = "/api/people/" + sid + "/photo"
		it.Images["square"] = it.Images["poster"]
	}
	return &it
}

// lifeDate writes a date as a person's page says it: "3 February 1970, Lisbon".
func lifeDate(date, place string) string {
	out := date
	for _, layout := range []string{"2006-01-02", "2006-01", "2006"} {
		if t, err := time.Parse(layout, date); err == nil {
			out = t.Format(map[string]string{"2006-01-02": "2 January 2006", "2006-01": "January 2006", "2006": "2006"}[layout])
			break
		}
	}
	if place != "" {
		if out == "" {
			return place
		}
		return out + ", " + place
	}
	return out
}

// personParts are the parts of the page's person's work that have titles the account may see.
func (rv *sduiResolver) personParts() ([]sdui.Each, error) {
	id, err := strconv.ParseInt(strings.TrimPrefix(rv.vars["person"], "person-"), 10, 64)
	if err != nil || !rv.s.Store.PersonVisible(rv.ctx, id, rv.acc) {
		return nil, nil
	}
	films, err := rv.s.Store.Filmography(rv.ctx, id, rv.acc)
	if err != nil {
		return nil, err
	}
	out := make([]sdui.Each, 0, len(films.Groups))
	for _, g := range films.Groups {
		out = append(out, sdui.Each{Key: g.Key, Label: g.Label})
	}
	return out, nil
}
