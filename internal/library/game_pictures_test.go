// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/wittyplayfile"
)

func picturesStore(t *testing.T) (*Store, *Item, string) {
	t.Helper()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	s := &Store{DB: database}
	ctx := context.Background()
	root := t.TempDir()
	lib, err := s.CreateLibrary(ctx, "Games", "games", []string{root})
	if err != nil {
		t.Fatal(err)
	}
	game := filepath.Join(root, "Made Up Quest (USA).sfc")
	if err := os.WriteFile(game, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	it := &Item{LibraryID: lib.ID, Kind: "game", Path: game, Title: "Made Up Quest", SortTitle: "Made Up Quest", Console: "snes"}
	if err := s.UpdateItem(ctx, it); err != nil {
		t.Fatal(err)
	}
	return s, it, root
}

// A .wittyplay file beside a game is read, by either name, and forgotten when it goes; a broken one
// counts as none. Nothing is ever written beside the game.
func TestSidecarLook(t *testing.T) {
	s, it, root := picturesStore(t)
	ctx := context.Background()
	side := filepath.Join(root, "Made Up Quest (USA).wittyplay")
	if err := os.WriteFile(side, []byte(`{"wittyplay":1,"default":{"picture":"case"},"consoles":{"snes":{"case":"cart-super","label":"title"}}}`), 0o444); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadDir(root)
	if err := s.noteSidecar(ctx, it); err != nil {
		t.Fatal(err)
	}
	all, err := s.GamePictures(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := all[it.ID]; got.Picture != "case" || got.Case != "cart-super" || got.Label != "title" {
		t.Fatalf("sidecar look = %+v", got)
	}
	// An administrator's choice wins, field by field.
	if err := s.SetGamePicture(ctx, it.ID, PictureByAdmin, wittyplayfile.Look{Case: "generic-box"}); err != nil {
		t.Fatal(err)
	}
	all, _ = s.GamePictures(ctx)
	if got := all[it.ID]; got.Case != "generic-box" || got.Picture != "case" {
		t.Fatalf("admin over file = %+v", got)
	}
	after, _ := os.ReadDir(root)
	if len(after) != len(before) {
		t.Fatalf("something was written beside the game: %v", after)
	}
	// The other name, and a broken file.
	os.Remove(side)
	if err := os.WriteFile(it.Path+".wittyplay", []byte(`{"wittyplay":1,"default":{"picture":"filled"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = s.noteSidecar(ctx, it)
	if l, _ := s.GamePictureFrom(ctx, it.ID, PictureByFile); l.Picture != "filled" {
		t.Fatalf("game.sfc.wittyplay = %+v", l)
	}
	if err := os.WriteFile(it.Path+".wittyplay", []byte(`{"wittyplay":1,"default":{"picture":"sideways"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = s.noteSidecar(ctx, it)
	if l, _ := s.GamePictureFrom(ctx, it.ID, PictureByFile); !l.Empty() {
		t.Fatalf("a broken file counts: %+v", l)
	}
	big := `{"wittyplay":1,"x":"` + strings.Repeat("y", wittyplayfile.MaxSize) + `"}`
	if err := os.WriteFile(it.Path+".wittyplay", []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := sidecarLook(it.Path, "snes", it.Title); ok {
		t.Fatal("a file over the limit was read")
	}
}

func TestGameMatchAndServerDoc(t *testing.T) {
	s, it, _ := picturesStore(t)
	ctx := context.Background()
	for _, m := range []wittyplayfile.Match{{Item: it.ID}, {File: "made up quest (usa).SFC"}, {Title: "made up quest", Console: "snes"}} {
		ids, err := s.GameMatch(ctx, m, 10)
		if err != nil || len(ids) != 1 || ids[0] != it.ID {
			t.Fatalf("%+v matched %v, %v", m, ids, err)
		}
	}
	for _, m := range []wittyplayfile.Match{{Item: it.ID + 99}, {Title: "Made Up Quest", Console: "nes"}, {File: "%"}, {File: "_ade Up Quest (USA).sfc"}} {
		if ids, _ := s.GameMatch(ctx, m, 10); len(ids) != 0 {
			t.Fatalf("%+v matched %v", m, ids)
		}
	}
	d := &wittyplayfile.Doc{Version: 1, Default: &wittyplayfile.Look{Picture: "case"}, Consoles: map[string]wittyplayfile.Look{"nes": {Case: "cart-nes"}},
		Games: []wittyplayfile.Game{{Match: wittyplayfile.Match{Item: 1}}}}
	if err := s.SetServerPictures(ctx, d); err != nil {
		t.Fatal(err)
	}
	got := s.ServerPictures(ctx)
	if got.Default.Picture != "case" || got.Consoles["nes"].Case != "cart-nes" || len(got.Games) != 0 {
		t.Fatalf("server doc = %+v", got)
	}
	if err := s.SetServerPictures(ctx, &wittyplayfile.Doc{Default: &wittyplayfile.Look{Picture: "nope"}}); err == nil {
		t.Fatal("a bad look was kept")
	}
}
