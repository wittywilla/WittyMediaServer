// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymodule"
	"github.com/wittywilla/wittymodule/host"
)

// fakeHostModules is one module, wittyplay, serving games libraries, holding the secret "tok".
type fakeHostModules struct{ in *wittymodule.Integration }

func (f fakeHostModules) HostModule(t string) (string, bool) { return "wittyplay", t == "tok" }
func (f fakeHostModules) Libraries(string) []string          { return []string{"games"} }
func (f fakeHostModules) Integration(string) *wittymodule.Integration {
	return f.in
}
func (f fakeHostModules) Name(string) string { return "WittyPlay" }

func TestHostAPI(t *testing.T) {
	ts := newTestServer(t)
	ts.hostModsForTest = fakeHostModules{in: &wittymodule.Integration{Module: "wittyplay", Data: &wittymodule.DataPart{Store: "sqlite", Import: "server-games-v1"}}}
	api := httptest.NewServer(ts.HostAPI())
	defer api.Close()
	ctx := context.Background()
	c := host.NewAt(api.URL, "tok", nil)

	pal, palTok := ts.user(t, "pal", "adult", false, "")
	open := `"maxAge":-1,"allowUnrated":true,"liveTv":true,"libraryChannels":true,"tunerChannels":true`
	kid, _ := ts.user(t, "kid", "adult", false, `{"maxAge":7,"allowUnrated":false,"off":["games"]}`)
	nog, _ := ts.user(t, "nog", "adult", false, `{`+open+`,"off":["games"]}`)
	games := ts.library(t, "games", t.TempDir())
	films := ts.library(t, "movies", t.TempDir())
	game := ts.item(t, &library.Item{LibraryID: games, Kind: "game", Path: "/g/a.sfc", Title: "A game", ContentRating: "ESRB M", Console: "snes"})
	film := ts.item(t, &library.Item{LibraryID: films, Kind: "video", Path: "/f/a.mkv", Title: "A film"})

	// Only a module's own secret gets in.
	if _, err := host.NewAt(api.URL, "forged", nil).Hello(ctx); !host.IsForbidden(err) {
		t.Fatalf("a forged secret: %v", err)
	}
	h, err := c.Hello(ctx)
	if err != nil || h.Module != "wittyplay" || h.Offers[wittymodule.HostAPI] != host.Version {
		t.Fatalf("hello: %+v %v", h, err)
	}

	// Items: of its kinds of library only, as the person asking may see them.
	it, err := c.Item(ctx, host.On{User: pal.ID}, game.ID)
	if err != nil || it.ID != game.ID || it.Facets["console"] != "snes" || len(it.Files) != 1 || it.Files[0].Path != "/g/a.sfc" {
		t.Fatalf("a game: %+v %v", it, err)
	}
	if _, err := c.Item(ctx, host.On{User: pal.ID}, film.ID); !host.IsNotFound(err) {
		t.Errorf("a film, which isn't its kind: %v", err)
	}
	if _, err := c.Item(ctx, host.On{User: kid.ID}, game.ID); !host.IsForbidden(err) {
		t.Errorf("a game too old for the person asking: %v", err)
	}
	if _, err := c.Item(ctx, host.On{User: 999999}, game.ID); !host.IsForbidden(err) {
		t.Errorf("a person who isn't there: %v", err)
	}
	list, err := c.Items(ctx, host.On{User: pal.ID}, host.Query{LibraryKind: "games"})
	if err != nil || len(list) != 1 || list[0].ID != game.ID || list[0].Files != nil {
		t.Fatalf("items: %+v %v", list, err)
	}
	if list, _ := c.Items(ctx, host.On{User: pal.ID}, host.Query{LibraryKind: "movies"}); len(list) != 0 {
		t.Errorf("films listed to a games module: %+v", list)
	}

	// Its libraries, with their folders; and its games counted by console.
	libs, err := c.Libraries(ctx)
	if err != nil || len(libs) != 1 || libs[0].ID != games || libs[0].Kind != "games" || len(libs[0].Paths) != 1 {
		t.Fatalf("libraries: %+v %v", libs, err)
	}
	if counts, err := c.Counts(ctx, host.On{User: pal.ID}, "games", "console"); err != nil || counts["snes"] != 1 {
		t.Fatalf("counts: %v %v", counts, err)
	}
	if counts, _ := c.Counts(ctx, host.On{User: kid.ID}, "games", "console"); counts["snes"] != 0 {
		t.Errorf("a game too old for the person asking was counted: %v", counts)
	}
	if _, err := c.Counts(ctx, host.On{User: pal.ID}, "movies", "console"); err == nil {
		t.Error("films were counted for a games module")
	}

	// What a person may do.
	if may, err := c.May(ctx, host.On{User: pal.ID}, "games"); !may || err != nil {
		t.Errorf("pal may play: %v %v", may, err)
	}
	if may, _ := c.May(ctx, host.On{User: nog.ID}, "games"); may {
		t.Error("someone with games off may play")
	}

	// Settings and secrets.
	if v, err := c.Settings(ctx, host.On{User: pal.ID}); err != nil || v == nil {
		t.Errorf("settings: %v %v", v, err)
	}
	_ = ts.DB.SetSetting(ctx, secretKey("wittyplay", "igdbSecret"), "s3cret")
	_ = ts.DB.SetSetting(ctx, secretKey("wittytuner", "other"), "not yours")
	if v, err := c.Secret(ctx, "igdbSecret"); v != "s3cret" || err != nil {
		t.Errorf("secret: %q %v", v, err)
	}
	if v, _ := c.Secret(ctx, "../wittytuner.secret.other"); v != "" {
		t.Errorf("another module's secret: %q", v)
	}

	// Pictures, then shown to people signed in.
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...)
	url, err := c.PutImage(ctx, game.ID, "cover", bytes.NewReader(png))
	if err != nil || !strings.HasPrefix(url, "/api/module-images/wittyplay/") {
		t.Fatalf("picture: %q %v", url, err)
	}
	if w := ts.do(t, "GET", url, palTok, nil); w.Code != 200 {
		t.Errorf("showing the picture: %d", w.Code)
	}
	if w := ts.do(t, "GET", url, "", nil); w.Code != 401 {
		t.Errorf("the picture, signed out: %d", w.Code)
	}
	if _, err := c.PutImage(ctx, 0, "x", strings.NewReader("<script>")); err == nil {
		t.Error("something that isn't a picture was kept")
	}

	// Live sessions.
	sid, err := c.StartSession(ctx, host.Session{Kind: "game", User: pal.ID, Item: game.ID, Title: "A game"})
	if err != nil || sid == "" || len(ts.modSessions.List()) != 1 {
		t.Fatalf("session: %q %v", sid, err)
	}
	if err := c.EndSession(ctx, sid); err != nil || len(ts.modSessions.List()) != 0 {
		t.Fatalf("ending it: %v", err)
	}
	if err := c.EndSession(ctx, sid); !host.IsNotFound(err) {
		t.Errorf("ending it twice: %v", err)
	}

	// Events.
	ts.hostEvents().publish("scan.finished", map[string]int64{"library": games})
	evs, next, err := c.Events(ctx, 0, 0)
	if err != nil || len(evs) == 0 || evs[len(evs)-1].Type != "scan.finished" {
		t.Fatalf("events: %+v %v", evs, err)
	}
	start := time.Now()
	go func() { time.Sleep(50 * time.Millisecond); ts.hostEvents().publish("item.added", nil) }()
	evs, _, err = c.Events(ctx, next, 5*time.Second)
	if err != nil || len(evs) != 1 || evs[0].Type != "item.added" || time.Since(start) > 4*time.Second {
		t.Fatalf("waiting for an event: %+v %v", evs, err)
	}

	// Legacy data: only the tables its import names.
	if _, err := ts.DB.ExecContext(ctx, `INSERT INTO wittyplay_controls (user_id, console, kind, key, data, updated_at) VALUES (?, 'snes', 'pad', 'k', '{}', 1)`, pal.ID); err != nil {
		t.Fatal(err)
	}
	rows, _, err := c.Legacy(ctx, "wittyplay_controls", 0, 10)
	if err != nil || len(rows) != 1 || !strings.Contains(string(rows[0]), `"console":"snes"`) {
		t.Fatalf("legacy rows: %s %v", rows, err)
	}
	if _, _, err := c.Legacy(ctx, "users", 0, 10); !host.IsForbidden(err) {
		t.Errorf("the users table: %v", err)
	}

	// A task's progress.
	if err := c.Task(ctx, "import", 1, 2, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Task(ctx, "import", 2, 2, nil); err != nil {
		t.Fatal(err)
	}
}
