// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymodule"
	"github.com/wittywilla/wittymodule/host"
)

// rigHostModules is wittyrigs, built on wittycouch, which is built on the program wittycouchhost.
type rigHostModules struct {
	fakeHostModules
	installed bool
}

func (f rigHostModules) HostModule(t string) (string, bool) { return "wittyrigs", t == "tok" }
func (f rigHostModules) Requires(id string) []string {
	return map[string][]string{"wittyrigs": {"wittycouch", "wittyplay"}, "wittycouch": {"wittycouchhost"}}[id]
}
func (f rigHostModules) ProgramCommand(id string) (string, []string, string, bool) {
	return "/bin/" + id, []string{"--sandboxed"}, "1.0.0", f.installed && id == "wittycouchhost"
}

// A module holding libraries (WittyRigs): it makes a games library of the console "rig", adds,
// pictures and removes its games, and can touch no other library or item.
func TestHostHeldLibraries(t *testing.T) {
	ts := newTestServer(t)
	ts.hostModsForTest = rigHostModules{fakeHostModules: fakeHostModules{in: &wittymodule.Integration{Module: "wittyrigs"}}}
	api := httptest.NewServer(ts.HostAPI())
	defer api.Close()
	ctx := context.Background()
	c := host.NewAt(api.URL, "tok", nil)
	var lib heldLibraryView
	if err := c.Do(ctx, host.On{}, "POST", "/libraries", map[string]string{"name": "Den PC"}, &lib); err != nil || lib.Console != library.RigConsole || lib.Type != "games" {
		t.Fatalf("a library: %+v %v", lib, err)
	}
	base := "/libraries/" + strconv.FormatInt(lib.ID, 10)
	var saved struct {
		ID  int64 `json:"id"`
		New bool  `json:"new"`
	}
	if err := c.Do(ctx, host.On{}, "PUT", base+"/items", map[string]string{"path": "rig://1/7", "title": "Racer", "tagline": "Plays on Den PC"}, &saved); err != nil || !saved.New {
		t.Fatalf("a game: %+v %v", saved, err)
	}
	first := saved.ID
	if err := c.Do(ctx, host.On{}, "PUT", base+"/items", map[string]string{"path": "rig://1/7", "title": "Racer 2"}, &saved); err != nil || saved.New || saved.ID != first {
		t.Fatalf("the same game again: %+v %v", saved, err)
	}
	if err := c.Do(ctx, host.On{}, "PUT", base+"/items", map[string]string{"path": "/etc/passwd", "title": "x"}, nil); err == nil {
		t.Error("a game with a path on disk was taken")
	}
	it, _ := ts.Store.Item(ctx, first)
	if it.Title != "Racer 2" || it.Console != library.RigConsole {
		t.Fatalf("the game: %+v", it)
	}

	// Its picture from the PC, which a picture chosen by hand outlives.
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	poster := "/items/" + strconv.FormatInt(first, 10) + "/poster"
	if err := c.Do(ctx, host.On{}, "PUT", poster, bytes.NewReader(png), nil); err != nil {
		t.Fatal(err)
	}
	it, _ = ts.Store.Item(ctx, first)
	name, ok := strings.CutPrefix(it.Poster, "cache:")
	if !ok || !strings.HasPrefix(it.Poster, library.RigArtPrefix) {
		t.Fatalf("poster %q", it.Poster)
	}
	if _, err := os.Stat(filepath.Join(ts.Cfg.ImagesDir, name)); err != nil {
		t.Fatal(err)
	}
	if err := c.Do(ctx, host.On{}, "DELETE", poster, nil, nil); err != nil {
		t.Fatal(err)
	}
	if it, _ = ts.Store.Item(ctx, first); it.Poster != "" {
		t.Errorf("the PC's picture stayed: %q", it.Poster)
	}
	it.Poster = "cache:chosen.jpg"
	if _, err := ts.DB.Exec(`UPDATE items SET poster = ? WHERE id = ?`, it.Poster, first); err != nil {
		t.Fatal(err)
	}
	_ = c.Do(ctx, host.On{}, "PUT", poster, bytes.NewReader(png), nil)
	if it, _ = ts.Store.Item(ctx, first); it.Poster != "cache:chosen.jpg" {
		t.Errorf("a picture chosen by hand was replaced: %q", it.Poster)
	}

	// Nothing of anyone else's.
	other := ts.library(t, "games", t.TempDir())
	game := ts.item(t, &library.Item{LibraryID: other, Kind: "game", Path: "/g/a.sfc", Title: "A game", Console: "snes"})
	if err := c.Do(ctx, host.On{}, "DELETE", "/libraries/"+strconv.FormatInt(other, 10), nil, nil); !host.IsNotFound(err) {
		t.Errorf("another library: %v", err)
	}
	if err := c.Do(ctx, host.On{}, "DELETE", "/items/"+strconv.FormatInt(game.ID, 10), nil, nil); !host.IsNotFound(err) {
		t.Errorf("another library's game: %v", err)
	}
	if err := c.Do(ctx, host.On{}, "PATCH", base, map[string]string{"name": "Study PC"}, &lib); err != nil || lib.Name != "Study PC" {
		t.Errorf("rename: %+v %v", lib, err)
	}
	if err := c.Do(ctx, host.On{}, "DELETE", "/items/"+strconv.FormatInt(first, 10), nil, nil); err != nil {
		t.Error(err)
	}
	if err := c.Do(ctx, host.On{}, "DELETE", base, nil, nil); err != nil {
		t.Error(err)
	}
	if _, err := ts.Store.Library(ctx, lib.ID); err == nil {
		t.Error("the library stayed")
	}
}

// A module runs only the programs of modules it is built on, and only when installed here.
func TestHostPrograms(t *testing.T) {
	ts := newTestServer(t)
	ts.hostModsForTest = rigHostModules{fakeHostModules: fakeHostModules{in: &wittymodule.Integration{Module: "wittyrigs"}}}
	api := httptest.NewServer(ts.HostAPI())
	defer api.Close()
	ctx := context.Background()
	c := host.NewAt(api.URL, "tok", nil)
	var out struct {
		Path string   `json:"path"`
		Args []string `json:"args"`
	}
	if err := c.Do(ctx, host.On{}, "GET", "/programs/wittycouchhost", nil, &out); !host.IsNotFound(err) {
		t.Errorf("not installed: %v", err)
	}
	if err := c.Do(ctx, host.On{}, "GET", "/programs/wittydvd", nil, &out); !host.IsForbidden(err) {
		t.Errorf("a module it isn't built on: %v", err)
	}
	ts.hostModsForTest = rigHostModules{fakeHostModules: fakeHostModules{in: &wittymodule.Integration{Module: "wittyrigs"}}, installed: true}
	if err := c.Do(ctx, host.On{}, "GET", "/programs/wittycouchhost", nil, &out); err != nil || out.Path != "/bin/wittycouchhost" || len(out.Args) != 1 {
		t.Errorf("installed: %+v %v", out, err)
	}
}
