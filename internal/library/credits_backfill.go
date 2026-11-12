// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"errors"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/pace"
)

// After a scan, titles scanned before credits existed get theirs, a few at a time: the NFO
// beside each, then each matched title's service, then pictures and biographies of the people
// most credited. Each is asked once (credits_read remembers, found or not), nothing is asked
// of a service whose key isn't set or that is switched off, and the pass gives way to people
// watching on a small machine and stops with the scan.

// How much one pass does. A scan comes round again, and the rest wait for it.
var (
	creditsNFOBatch    = 400 // files read beside the media: cheap
	creditsOnlineBatch = 25  // titles a service is asked about
	creditsPeopleBatch = 10  // people TMDB is asked about
)

// fillCredits is the pass after a scan of a library of films or shows.
func (s *Scanner) fillCredits(ctx context.Context, lib *Library) {
	ctx = pace.Background(ctx)
	if err := s.fillNFOCredits(ctx, lib); err != nil {
		s.creditsTrouble(ctx, "could not read people from NFO files", err, lib)
		return
	}
	if err := s.fillOnlineCredits(ctx, lib); err != nil {
		s.creditsTrouble(ctx, "could not fill in the people of titles matched before", err, lib)
		return
	}
	if err := s.fillPeopleDetails(ctx); err != nil {
		s.creditsTrouble(ctx, "could not look people up", err, lib)
	}
}

func (s *Scanner) creditsTrouble(ctx context.Context, msg string, err error, lib *Library) {
	if ctx.Err() != nil {
		return // stopped with the scan
	}
	s.Log.WarnContext(ctx, msg, "library", lib.Name, "err", err)
}

// fillNFOCredits reads the people of NFOs beside films and episodes that were never read for them.
func (s *Scanner) fillNFOCredits(ctx context.Context, lib *Library) error {
	rows, err := s.Store.DB.QueryContext(ctx, `SELECT i.id FROM items i WHERE i.library_id = ? AND i.missing = 0
		AND NOT EXISTS (SELECT 1 FROM credits_read r WHERE r.target = 'item' AND r.target_id = i.id AND r.source = 'nfo')
		ORDER BY i.id LIMIT ?`, lib.ID, creditsNFOBatch)
	if err != nil {
		return err
	}
	ids, err := scanIDs(rows)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := pace.Yield(ctx); err != nil {
			return err
		}
		it, err := s.Store.Item(ctx, id)
		if err != nil {
			continue
		}
		t := &Title{Target: TargetItem, ID: id, Item: it}
		if err := s.Store.SetCredits(ctx, TargetItem, id, CreditsNFO, s.nfoCredits(ctx, t)); err != nil {
			return err
		}
	}
	return nil
}

// creditsTodo is one title a service hasn't been asked about for its people.
type creditsTodo struct {
	target, provider, providerID string
	id                           int64
}

// fillOnlineCredits asks each matched title's service for its people, for titles matched before
// credits existed.
func (s *Scanner) fillOnlineCredits(ctx context.Context, lib *Library) error {
	todo, err := s.creditsTodo(ctx, lib)
	if err != nil {
		return err
	}
	for _, td := range todo {
		if err := pace.Yield(ctx); err != nil {
			return err
		}
		err := s.fillTitleCredits(ctx, td)
		if errors.Is(err, ErrNotFound) || (err != nil && strings.Contains(err.Error(), "404")) {
			// The service doesn't know that id (any more): it names nobody, and isn't asked again.
			err = s.Store.SetCredits(ctx, td.target, td.id, creditSourceOf(td.provider), nil)
		}
		if err != nil {
			return err // most likely no network: the rest wait for the next scan
		}
	}
	return nil
}

// creditsTodo lists the library's films and shows matched to a service that is on, whose
// people that service was never asked for.
func (s *Scanner) creditsTodo(ctx context.Context, lib *Library) ([]creditsTodo, error) {
	tmdbOn := s.TMDBKey != nil && s.TMDBKey() != ""
	tvdbOn := false
	if s.TVDBKey != nil {
		key, _ := s.TVDBKey()
		tvdbOn = key != ""
	}
	tvmazeOn := s.TVMazeOn == nil || s.TVMazeOn()
	var providers []string
	for _, p := range []struct {
		name string
		on   bool
	}{{"tmdb", tmdbOn}, {"tmdb-tv", tmdbOn}, {"tvdb", tvdbOn}, {"tvmaze", tvmazeOn}} {
		if p.on {
			providers = append(providers, "'"+p.name+"'")
		}
	}
	if len(providers) == 0 {
		return nil, nil
	}
	in := strings.Join(providers, ",")
	source := `CASE t.provider WHEN 'tmdb-tv' THEN 'tmdb' ELSE t.provider END`
	rows, err := s.Store.DB.QueryContext(ctx, `SELECT * FROM (
		SELECT 'item', t.id, t.provider, t.provider_id FROM items t WHERE t.library_id = ?1 AND t.missing = 0 AND t.show_id IS NULL
		  AND t.provider IN (`+in+`) AND t.provider <> 'tmdb-tv' AND t.provider_id <> ''
		  AND NOT EXISTS (SELECT 1 FROM credits_read r WHERE r.target = 'item' AND r.target_id = t.id AND r.source = `+source+`)
		UNION ALL
		SELECT 'show', t.id, t.provider, t.provider_id FROM shows t WHERE t.library_id = ?1 AND t.missing = 0
		  AND t.provider IN (`+in+`) AND t.provider <> 'tmdb' AND t.provider_id <> ''
		  AND (NOT EXISTS (SELECT 1 FROM credits_read r WHERE r.target = 'show' AND r.target_id = t.id AND r.source = `+source+`)
		    -- episodes added since the show was read: the services that name episodes' people
		    OR (t.provider IN ('tvdb', 'tmdb-tv') AND EXISTS (SELECT 1 FROM items e WHERE e.show_id = t.id AND e.missing = 0
		      AND e.provider = t.provider AND e.provider_id <> ''
		      AND NOT EXISTS (SELECT 1 FROM credits_read r WHERE r.target = 'item' AND r.target_id = e.id AND r.source = `+source+`))))
		) LIMIT ?2`, lib.ID, creditsOnlineBatch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []creditsTodo
	for rows.Next() {
		var td creditsTodo
		if err := rows.Scan(&td.target, &td.id, &td.provider, &td.providerID); err != nil {
			return nil, err
		}
		out = append(out, td)
	}
	return out, rows.Err()
}

// fillTitleCredits asks one title's service for its people, and for a show its episodes'.
func (s *Scanner) fillTitleCredits(ctx context.Context, td creditsTodo) error {
	switch td.provider {
	case "tmdb":
		list, err := (&TMDB{Key: s.TMDBKey(), Base: s.TMDBBase}).MovieCredits(ctx, td.providerID)
		if err != nil {
			return err
		}
		return s.Store.SetCredits(ctx, TargetItem, td.id, CreditsTMDB, list)
	case "tmdb-tv":
		src := &tmdbShows{TMDB{Key: s.TMDBKey(), Base: s.TMDBBase}}
		d, err := src.Details(ctx, td.providerID)
		if err != nil {
			return err
		}
		eps, err := src.Episodes(ctx, td.providerID)
		if err != nil {
			return err
		}
		if err := s.setEpisodeCredits(ctx, td, eps); err != nil {
			return err
		}
		return s.Store.SetCredits(ctx, TargetShow, td.id, CreditsTMDB, d.Credits)
	case "tvdb":
		key, pin := s.TVDBKey()
		show, byEpisode, err := (&TVDB{Key: key, PIN: pin}).seriesPeople(ctx, td.providerID)
		if err != nil {
			return err
		}
		if err := s.setEpisodeCreditsByID(ctx, td, byEpisode); err != nil {
			return err
		}
		return s.Store.SetCredits(ctx, TargetShow, td.id, CreditsTVDB, show)
	case "tvmaze":
		list, err := (&TVMaze{}).ShowCredits(ctx, td.providerID)
		if err != nil {
			return err
		}
		return s.Store.SetCredits(ctx, TargetShow, td.id, CreditsTVMaze, list)
	}
	return nil
}

type episodeRow struct {
	id                   int64
	season, episode      int
	provider, providerID string
}

func (s *Scanner) showEpisodes(ctx context.Context, showID int64) ([]episodeRow, error) {
	rows, err := s.Store.DB.QueryContext(ctx, `SELECT id, IFNULL(season, -1), IFNULL(episode, -1), provider, provider_id FROM items WHERE show_id = ? AND missing = 0`, showID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []episodeRow
	for rows.Next() {
		var e episodeRow
		if err := rows.Scan(&e.id, &e.season, &e.episode, &e.provider, &e.providerID); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// setEpisodeCredits stores the people of a show's episodes that its service knows, by number.
func (s *Scanner) setEpisodeCredits(ctx context.Context, td creditsTodo, eps map[[2]int]EpisodeMeta) error {
	list, err := s.showEpisodes(ctx, td.id)
	if err != nil {
		return err
	}
	for _, e := range list {
		if e.provider != td.provider {
			continue
		}
		// An episode the service no longer knows names nobody, and isn't asked about again.
		m := eps[[2]int{e.season, e.episode}]
		if err := s.Store.SetCredits(ctx, TargetItem, e.id, creditSourceOf(td.provider), m.Credits); err != nil {
			return err
		}
	}
	return nil
}

// setEpisodeCreditsByID stores the people a show's service names by its episode ids.
func (s *Scanner) setEpisodeCreditsByID(ctx context.Context, td creditsTodo, by map[string][]CreditIn) error {
	list, err := s.showEpisodes(ctx, td.id)
	if err != nil {
		return err
	}
	for _, e := range list {
		if e.provider != td.provider || e.providerID == "" {
			continue
		}
		if err := s.Store.SetCredits(ctx, TargetItem, e.id, creditSourceOf(td.provider), by[e.providerID]); err != nil {
			return err
		}
	}
	return nil
}

// fillPeopleDetails asks TMDB about the most credited people nobody has asked about yet.
func (s *Scanner) fillPeopleDetails(ctx context.Context) error {
	if s.TMDBKey == nil || s.TMDBKey() == "" {
		return nil
	}
	people, err := s.Store.PeopleNeedingDetails(ctx, creditsPeopleBatch)
	if err != nil {
		return err
	}
	for _, p := range people {
		if err := pace.Yield(ctx); err != nil {
			return err
		}
		if _, err := s.LookUpPerson(ctx, p); err != nil && !strings.Contains(err.Error(), "404") {
			return err
		}
	}
	return nil
}

// scanIDs reads a column of ids and closes the rows.
func scanIDs(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close() error
}) ([]int64, error) {
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
