// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"encoding/json"
	"strings"
)

// An item's other names (a game's, today, from the module that describes it). The title stays
// what it always was (and sorts the same); a profile may choose to see the name in its own
// script, or its English release's, instead (the web app's "gameTitles" preference). They are
// kept in items.alt_titles as JSON.

// ItemTitles are an item's names, each only when known.
type ItemTitles struct {
	// Native is the name in its own script, such as Japanese.
	Native string `json:"native,omitempty"`
	// English is the English release's name.
	English string `json:"english,omitempty"`
	// Romanized is the title as it's listed: the name in Latin letters, the default.
	Romanized string `json:"romanized,omitempty"`
}

type storedTitles struct {
	Native  string `json:"native,omitempty"`
	English string `json:"english,omitempty"`
}

// altTitlesLike matches an item's other names with one LIKE argument, for searches.
// searchNames: who made it and what it's part of (an artist, an album or a book's series), for search.
const searchNames = "(IFNULL(i.artist, '') || char(10) || IFNULL(i.album_artist, '') || char(10) || IFNULL(i.album, ''))"

const altTitlesLike = "(IFNULL(json_extract(i.alt_titles, '$.native'), '') || char(10) || IFNULL(json_extract(i.alt_titles, '$.english'), '')) LIKE ?"

// readItemTitles reads what alt_titles holds. Nothing known: nil.
func readItemTitles(raw, title string) *ItemTitles {
	if raw == "" {
		return nil
	}
	var s storedTitles
	if json.Unmarshal([]byte(raw), &s) != nil || s.Native == "" && s.English == "" {
		return nil
	}
	return &ItemTitles{Native: s.Native, English: s.English, Romanized: title}
}

// itemTitlesFor is what an item's other names come to: nothing that only repeats the title.
// An empty string when there's nothing to keep.
func itemTitlesFor(native, english, title string) string {
	clean := func(s string) string {
		s = strings.TrimSpace(s)
		if strings.EqualFold(s, title) || len(s) > 300 {
			return ""
		}
		return s
	}
	s := storedTitles{Native: clean(native), English: clean(english)}
	if s.Native == "" && s.English == "" {
		return ""
	}
	b, _ := json.Marshal(s)
	return string(b)
}

// setItemTitles keeps an item's other names, or forgets them ("").
func (s *Store) setItemTitles(ctx context.Context, id int64, stored string) error {
	var v any
	if stored != "" {
		v = stored
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE items SET alt_titles = ? WHERE id = ?`, v, id)
	return err
}
