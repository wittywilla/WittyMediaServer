// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"database/sql"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/search"
)

// searchGroup is the group a library's titles are found in: its own kind when the
// search has one of that name, else videos.
func searchGroup(libType string) string {
	switch libType {
	case "movies", "discs":
		return "films"
	case "shows", "anime":
		return "episodes"
	case "music":
		return "songs"
	case "shorts", "channels", "musicvideos", "commercials":
		return "videos"
	}
	if _, ok := search.GroupLabels[libType]; ok {
		return libType
	}
	return "videos"
}

// SearchDocs lists what a search can find in one library: its titles, and in a
// show library its shows, in a music library its albums and artists, in an
// audiobook or podcast library its books and podcasts. Rows a search never shows
// (missing files, extras attached to a title) are left out.
func (s *Store) SearchDocs(ctx context.Context, lib *Library) ([]search.Doc, error) {
	var out []search.Doc
	group := searchGroup(lib.Type)
	albumsOnly := lib.Type == "audiobooks" || lib.Type == "podcasts"
	if !albumsOnly {
		rows, err := s.DB.QueryContext(ctx, `SELECT i.id, i.title, IFNULL(i.year, 0), IFNULL(i.show_id, 0), IFNULL(i.artist, ''), IFNULL(i.album, ''),
			IFNULL(json_extract(i.alt_titles, '$.native'), ''), IFNULL(json_extract(i.alt_titles, '$.english'), ''), LOWER(SUBSTR(i.path, -4))
			FROM items i WHERE i.library_id = ? AND i.missing = 0 AND `+hideLinked, lib.ID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var d search.Doc
			var show int64
			var artist, album, native, english, ext string
			if err := rows.Scan(&d.ID, &d.Title, &d.Year, &show, &artist, &album, &native, &english, &ext); err != nil {
				rows.Close()
				return nil, err
			}
			d.Lib, d.Group = lib.ID, group
			switch {
			case group == "episodes" && show == 0:
				d.Group = "films" // a film kept in a show library
			case group == "songs":
				d.Alt = artist + "\n" + album
			case group == "books" && (ext == ".cbz" || ext == ".cbr" || ext == ".cb7" || ext == ".cbt"):
				d.Group = "comics"
			}
			if native != "" || english != "" {
				d.Alt = strings.TrimSpace(d.Alt + "\n" + native + "\n" + english)
			}
			out = append(out, d)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	switch lib.Type {
	case "shows", "anime":
		rows, err := s.DB.QueryContext(ctx, `SELECT id, title, IFNULL(year, 0) FROM shows WHERE library_id = ? AND missing = 0`, lib.ID)
		if err != nil {
			return nil, err
		}
		err = scanDocs(rows, func(d *search.Doc) { d.Lib, d.Group = lib.ID, "shows" }, &out)
		if err != nil {
			return nil, err
		}
	case "music", "audiobooks", "podcasts":
		// Albums as the library's page lists them (groupWhere, AlbumsFiltered): an album
		// artist's album, its cover the first track with a picture, else its first track.
		ag := "albums"
		if albumsOnly {
			ag = lib.Type
		}
		rows, err := s.DB.QueryContext(ctx, `SELECT IFNULL(MIN(CASE WHEN IFNULL(i.poster, '') <> '' THEN i.id END), MIN(i.id)), IFNULL(i.album, '') AS al, IFNULL(MIN(i.year), 0),
			`+albumArtist+` AS a, COUNT(*)
			FROM items i WHERE i.library_id = ? AND i.missing = 0 AND i.kind IN ('audio', 'book') AND IFNULL(i.album, '') <> ''
			GROUP BY a COLLATE NOCASE, al COLLATE NOCASE`, lib.ID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			d := search.Doc{Lib: lib.ID, Group: ag}
			if err := rows.Scan(&d.ID, &d.Title, &d.Year, &d.Alt, &d.Count); err != nil {
				rows.Close()
				return nil, err
			}
			d.Key = d.Alt // the album artist, which the album's page is found by
			out = append(out, d)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if lib.Type != "music" {
			break
		}
		rows, err = s.DB.QueryContext(ctx, `SELECT IFNULL(MIN(CASE WHEN IFNULL(i.poster, '') <> '' THEN i.id END), MIN(i.id)), `+albumArtist+` AS a, COUNT(DISTINCT IFNULL(i.album, ''))
			FROM items i WHERE i.library_id = ? AND i.missing = 0 AND i.kind = 'audio' AND `+albumArtist+` <> ''
			GROUP BY a COLLATE NOCASE`, lib.ID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			d := search.Doc{Lib: lib.ID, Group: "artists"}
			if err := rows.Scan(&d.ID, &d.Title, &d.Count); err != nil {
				rows.Close()
				return nil, err
			}
			d.Key = d.Title
			out = append(out, d)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func scanDocs(rows *sql.Rows, fill func(*search.Doc), out *[]search.Doc) error {
	defer rows.Close()
	for rows.Next() {
		var d search.Doc
		if err := rows.Scan(&d.ID, &d.Title, &d.Year); err != nil {
			return err
		}
		fill(&d)
		*out = append(*out, d)
	}
	return rows.Err()
}

// PeopleDocs lists everyone credited on a title, for search. Count is how many
// titles they're credited on, so the best known come first among equal matches.
func (s *Store) PeopleDocs(ctx context.Context) ([]search.Doc, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT p.id, p.name, (SELECT COUNT(*) FROM credits c WHERE c.person_id = p.id) FROM people p`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []search.Doc
	for rows.Next() {
		d := search.Doc{Group: "people"}
		if err := rows.Scan(&d.ID, &d.Title, &d.Count); err != nil {
			return nil, err
		}
		if d.Count > 0 {
			out = append(out, d)
		}
	}
	return out, rows.Err()
}

// SearchStamp is what changes when a library's titles do: how many rows it has and
// when the last changed, for its shows too. People are library 0.
type SearchStamp struct{ Rows, Changed int64 }

// SearchStamps reads every library's stamp in one pass over the items' and shows' indexes.
func (s *Store) SearchStamps(ctx context.Context) (map[int64]SearchStamp, error) {
	out := map[int64]SearchStamp{}
	for _, q := range []string{
		`SELECT library_id, COUNT(*), MAX(updated_at) FROM items GROUP BY library_id`,
		`SELECT library_id, COUNT(*), MAX(updated_at) FROM shows GROUP BY library_id`,
		`SELECT 0, COUNT(*), IFNULL(MAX(updated_at), 0) FROM people`,
	} {
		rows, err := s.DB.QueryContext(ctx, q)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var lib int64
			var st SearchStamp
			var changed sql.NullInt64
			if err := rows.Scan(&lib, &st.Rows, &changed); err != nil {
				rows.Close()
				return nil, err
			}
			o := out[lib]
			o.Rows += st.Rows
			o.Changed = max(o.Changed, changed.Int64)
			out[lib] = o
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// VisiblePeople keeps the people of ids with a title the account may see, with how many.
func (s *Store) VisiblePeople(ctx context.Context, ids []int64, access *Access) ([]FoundPerson, error) {
	out := []FoundPerson{}
	if len(ids) == 0 {
		return out, nil
	}
	q, args := visibleCreditsQuery("SELECT COUNT(DISTINCT v.target || v.tid)", access)
	for _, id := range ids {
		p := FoundPerson{ID: id}
		if err := s.DB.QueryRowContext(ctx, `SELECT name, photo <> '', known_for FROM people WHERE id = ?`, id).Scan(&p.Name, &p.HasPhoto, &p.KnownFor); err != nil {
			continue
		}
		if err := s.DB.QueryRowContext(ctx, q, args(id)...).Scan(&p.Titles); err != nil {
			return nil, err
		}
		if p.Titles > 0 {
			out = append(out, p)
		}
	}
	return out, nil
}
