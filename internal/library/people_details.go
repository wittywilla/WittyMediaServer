// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
)

// What TMDB says about one person (GET /person/{id}): their picture, dates, birthplace and
// biography. Asked when someone opens the person's page and nobody has asked for a month, and by
// the paced background pass after a scan, never for every name in every film at once.

type tmdbPerson struct {
	Name         string `json:"name"`
	Biography    string `json:"biography"`
	Birthday     string `json:"birthday"`
	Deathday     string `json:"deathday"`
	PlaceOfBirth string `json:"place_of_birth"`
	ProfilePath  string `json:"profile_path"`
	KnownFor     string `json:"known_for_department"`
	IMDBID       string `json:"imdb_id"`
}

// Person reads one person from TMDB.
func (t *TMDB) Person(ctx context.Context, id string) (*PersonDetails, error) {
	id = strings.TrimSpace(id)
	if id == "" || strings.ContainsAny(id, "/?#") {
		return nil, errors.New("tmdb: no person id")
	}
	var p tmdbPerson
	if err := t.get(ctx, "/person/"+url.PathEscape(id), url.Values{}, &p); err != nil {
		return nil, err
	}
	d := &PersonDetails{Birthday: p.Birthday, Deathday: p.Deathday, Birthplace: p.PlaceOfBirth, Biography: p.Biography,
		KnownFor: knownForKey(p.KnownFor), IDs: PersonIDs{TMDB: id, IMDB: p.IMDBID}}
	if p.ProfilePath != "" {
		d.Photo = tmdbImage + "h632" + p.ProfilePath
	}
	return d, nil
}

// knownForKey is a department as a person's page names it: acting, or a department's key.
func knownForKey(dept string) string {
	switch strings.TrimSpace(dept) {
	case "":
		return ""
	case "Acting", "Actors":
		return FilmActing
	}
	return DepartmentOf(dept, "")
}

// LookUpPerson fills in a person from TMDB when they have an id there, a key is set, and nobody
// asked in the last month. It reports whether it asked.
func (s *Scanner) LookUpPerson(ctx context.Context, p *Person) (bool, error) {
	if p == nil || p.IDs.TMDB == "" || s.TMDBKey == nil || time.Now().Unix()-p.DetailsAt < 30*86400 {
		return false, nil
	}
	key := s.TMDBKey()
	if key == "" {
		return false, nil
	}
	t := &TMDB{Key: key, Base: s.TMDBBase}
	d, err := t.Person(ctx, p.IDs.TMDB)
	if err != nil {
		// Asked and not answered: wait a month before asking about them again, unless it
		// failed for a reason of this server's own (no network), which is worth another try.
		if strings.Contains(err.Error(), "404") {
			_ = s.Store.MarkPersonLookedUp(ctx, p.ID)
		}
		return true, err
	}
	return true, s.Store.SetPersonDetails(ctx, p.ID, *d)
}
