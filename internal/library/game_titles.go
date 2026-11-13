// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/gamemeta"
)

// A game's other names. The title stays what it always was (and sorts the same); a profile
// may choose to see the game's name in its own script, or its English release's, instead
// (the web app's "gameTitles" preference). They are kept in items.alt_titles as JSON.

// GameTitles are a game's names, each only when known.
type GameTitles struct {
	// Native is the name in the game's own script, such as Japanese.
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

// altTitlesLike matches a game's other names with one LIKE argument, for searches.
const altTitlesLike = "(IFNULL(json_extract(i.alt_titles, '$.native'), '') || char(10) || IFNULL(json_extract(i.alt_titles, '$.english'), '')) LIKE ?"

// readGameTitles reads what alt_titles holds. Nothing known: nil.
func readGameTitles(raw, title string) *GameTitles {
	if raw == "" {
		return nil
	}
	var s storedTitles
	if json.Unmarshal([]byte(raw), &s) != nil || s.Native == "" && s.English == "" {
		return nil
	}
	return &GameTitles{Native: s.Native, English: s.English, Romanized: title}
}

// gameTitlesFor is what a game's other names come to, from what the sources said: tags left
// off as from the title, and nothing that only repeats the title. An empty string when
// there's nothing to keep.
func gameTitlesFor(native, english, title string) string {
	clean := func(s string) string {
		s, _ = gamemeta.CleanName(s)
		s = strings.TrimSpace(s)
		if strings.EqualFold(s, title) {
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

// setGameTitles keeps a game's other names, or forgets them ("").
func (s *Store) setGameTitles(ctx context.Context, id int64, stored string) error {
	var v any
	if stored != "" {
		v = stored
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE items SET alt_titles = ? WHERE id = ?`, v, id)
	return err
}
