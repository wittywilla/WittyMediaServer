// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"testing"
)

// A custom-gamecube console, made the way the old documentation said, becomes the real GameCube
// once WittyPlay has it: its games and library move, and it goes. A second run changes nothing,
// and one WittyPlay doesn't know is left alone.
func TestCustomConsolesAreAdopted(t *testing.T) {
	ts := newTestServer(t)
	s, ctx := ts, context.Background()
	old := adoptKnown
	t.Cleanup(func() { adoptKnown = old })
	adoptKnown = func(id string) bool { return id == "gc" }
	mustExec := func(q string, a ...any) {
		t.Helper()
		if _, err := s.DB.ExecContext(ctx, q, a...); err != nil {
			t.Fatal(q, err)
		}
	}
	mustExec(`INSERT INTO libraries (id, name, type, created_at, console) VALUES (1, 'Cube', 'games', 0, 'custom-gamecube'), (2, 'Box', 'games', 0, 'custom-xbox')`)
	mustExec(`INSERT INTO items (id, library_id, kind, path, title, sort_title, console, added_at, updated_at) VALUES (1, 1, 'game', '/g/a.iso', 'A', 'a', 'custom-gamecube', 1, 1), (2, 2, 'game', '/g/b.iso', 'B', 'b', 'custom-xbox', 1, 1)`)
	mustExec(`INSERT INTO custom_consoles (id, data, created_at, updated_at) VALUES ('custom-gamecube', '{}', 0, 0), ('custom-xbox', '{}', 0, 0)`)
	for range 2 {
		s.adoptCustomConsoles(ctx)
	}
	console := func(q string) string {
		var c string
		if err := s.DB.QueryRowContext(ctx, q).Scan(&c); err != nil {
			t.Fatal(q, err)
		}
		return c
	}
	if c := console(`SELECT console FROM items WHERE id = 1`); c != "gc" {
		t.Errorf("the game is on %q", c)
	}
	if c := console(`SELECT console FROM libraries WHERE id = 1`); c != "gc" {
		t.Errorf("the library is on %q", c)
	}
	if c := console(`SELECT console FROM items WHERE id = 2`); c != "custom-xbox" {
		t.Errorf("a console WittyPlay doesn't know yet was moved: %q", c)
	}
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM custom_consoles`).Scan(&n)
	if n != 1 {
		t.Errorf("%d custom consoles left, want only custom-xbox", n)
	}
}
