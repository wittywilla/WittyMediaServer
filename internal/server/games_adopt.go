// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	wittyplay "github.com/wittywilla/wittymediaserver/internal/play"
)

// Before GameCube, Wii, Wii U, PS3, Xbox and Switch were consoles of WittyPlay's own, the way to
// play them in a standalone emulator was a custom console named for them: custom-gamecube and so
// on (WittyPlay's docs/DOCKER-BACKEND.md said so). Those are adopted into the real consoles once
// WittyPlay has them: their games, libraries, settings and preferences move over, and the custom
// console goes. Nothing is lost: a custom console's own aliases and notes were only ever there to
// make up for the console not existing.

// adoptedConsoles maps the custom consoles people were told to make to the real ones.
var adoptedConsoles = map[string]string{
	"custom-gamecube": "gc", "custom-gc": "gc", "custom-ngc": "gc",
	"custom-wii":  "wii",
	"custom-wiiu": "wiiu",
	"custom-ps3":  "ps3", "custom-playstation3": "ps3",
	"custom-xbox":   "xbox",
	"custom-switch": "switch", "custom-nintendoswitch": "switch",
}

// adoptKnown says WittyPlay has a console of its own by this id; a test stands in for it.
var adoptKnown = func(id string) bool {
	c := wittyplay.ByID(id)
	return c != nil && !wittyplay.IsCustomConsole(c.ID)
}

// adoptCustomConsoles moves what's kept under each of those to the real console, in one
// transaction, when WittyPlay knows the real one. It does nothing a second time.
func (s *Server) adoptCustomConsoles(ctx context.Context) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM custom_consoles`)
	if err != nil {
		return
	}
	var found []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			if _, ok := adoptedConsoles[id]; ok {
				found = append(found, id)
			}
		}
	}
	rows.Close()
	for _, from := range found {
		to := adoptedConsoles[from]
		if !adoptKnown(to) {
			continue // a WittyPlay that doesn't have it yet: leave it be
		}
		n, err := s.adoptConsole(ctx, from, to)
		if err != nil {
			s.Log.Warn("games: moving a custom console's games to the real console", "from", from, "to", to, "err", err)
			continue
		}
		s.Log.Info("games: a custom console became the real one", "from", from, "to", to, "games", n)
	}
}

func (s *Server) adoptConsole(ctx context.Context, from, to string) (int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck // a no-op once committed
	res, err := tx.ExecContext(ctx, `UPDATE items SET console = ? WHERE console = ?`, to, from)
	if err != nil {
		return 0, err
	}
	games, _ := res.RowsAffected()
	for _, q := range []string{
		`UPDATE libraries SET console = ? WHERE console = ?`,
		// Per-person settings, controls and profiles: where the real console already has a row
		// of its own, that one stays and the custom console's is dropped.
		`UPDATE OR IGNORE wittyplay_controls SET console = ? WHERE console = ?`,
		`UPDATE OR IGNORE wittyplay_settings SET console = ? WHERE console = ?`,
		`UPDATE OR IGNORE wittyplay_profiles SET console = ? WHERE console = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, to, from); err != nil {
			return 0, fmt.Errorf("%s: %w", q, err)
		}
	}
	for _, q := range []string{`DELETE FROM wittyplay_controls WHERE console = ?`, `DELETE FROM wittyplay_settings WHERE console = ?`,
		`DELETE FROM wittyplay_profiles WHERE console = ?`, `DELETE FROM custom_consoles WHERE id = ?`} {
		if _, err := tx.ExecContext(ctx, q, from); err != nil {
			return 0, fmt.Errorf("%s: %w", q, err)
		}
	}
	// Custom cores assigned to it, and the preferred core, follow it.
	crows, err := tx.QueryContext(ctx, `SELECT id, consoles FROM custom_cores`)
	if err != nil {
		return 0, err
	}
	type upd struct{ id, consoles string }
	var ups []upd
	for crows.Next() {
		var id, raw string
		if crows.Scan(&id, &raw) != nil {
			continue
		}
		var list []string
		if json.Unmarshal([]byte(raw), &list) != nil || !slices.Contains(list, from) {
			continue
		}
		list = slices.DeleteFunc(list, func(c string) bool { return c == from })
		if !slices.Contains(list, to) {
			list = append(list, to)
		}
		b, _ := json.Marshal(list)
		ups = append(ups, upd{id, string(b)})
	}
	crows.Close()
	for _, u := range ups {
		if _, err := tx.ExecContext(ctx, `UPDATE custom_cores SET consoles = ? WHERE id = ?`, u.consoles, u.id); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	prefs := s.preferredCores(ctx)
	if core, ok := prefs[from]; ok {
		delete(prefs, from)
		if _, has := prefs[to]; !has {
			prefs[to] = core
		}
		_ = s.DB.SetSetting(ctx, "gamingCores", toJSON(prefs))
	}
	return games, nil
}
