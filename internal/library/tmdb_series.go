// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"path/filepath"
	"strings"
)

// tmdbSeries is TMDB's shows as the TV scan asks for them.
type tmdbSeries struct{ *tmdbShows }

func (t *tmdbSeries) Series(ctx context.Context, id string) (*SeriesMeta, error) {
	d, err := t.Details(ctx, id)
	if err != nil {
		return nil, err
	}
	return &SeriesMeta{ID: d.ID, Title: d.Title, Year: d.Year, Overview: d.Overview, Genres: d.Genres, Rating: d.Rating, ContentRating: d.ContentRating,
		Network: d.Network, Status: d.Status, Poster: d.Poster, Backdrop: d.Backdrop, SeasonPosters: d.SeasonPosters, Posters: d.Posters, Backdrops: d.Backdrops,
		Credits: d.Credits, CreditsRead: d.CreditsRead, EpisodeCredits: d.EpisodeCredits}, nil
}

func (t *tmdbSeries) FindSeries(ctx context.Context, name string, year *int) (*SeriesMeta, error) {
	c, err := t.Search(ctx, name, year)
	if err != nil {
		return nil, err
	}
	if len(c) == 0 {
		return nil, ErrNotFound
	}
	best := c[0]
	if year != nil {
		for _, x := range c {
			if x.Year != nil && *x.Year == *year {
				best = x
				break
			}
		}
	}
	return t.Series(ctx, best.ID)
}

// markRuleExtra marks a file a rule says is a bonus feature, by leaving it for someone to
// attach: a rule can say what it is, but not which film it belongs to, unless the film
// is alone in its folder.
func (s *Store) markRuleExtra(ctx context.Context, it *Item, extra string) {
	if s.LinkOf(ctx, it.ID) != nil {
		return
	}
	var parent int64
	var n int
	row := s.DB.QueryRowContext(ctx, `SELECT COUNT(*), IFNULL(MIN(id), 0) FROM items WHERE library_id = ? AND missing = 0 AND id <> ? AND show_id IS NULL AND
		(path LIKE ? ESCAPE '\') AND id NOT IN (SELECT item_id FROM item_links)`, it.LibraryID, it.ID, likeFolder(it.Path))
	if row.Scan(&n, &parent) != nil || n != 1 {
		return
	}
	_ = s.AttachItem(ctx, it.ID, LinkExtra, extra, "", &parent, nil)
}

// likeFolder is a LIKE pattern for files in the same folder as path.
func likeFolder(path string) string {
	dir := filepath.Dir(path)
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(dir) + string(filepath.Separator) + "%"
}
