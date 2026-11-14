// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymodule"
	"github.com/wittywilla/wittymodule/host"
)

// fakeScanModule is a module that scans games libraries (scan hooks), as the scanner sees it.
type fakeScanModule struct {
	mu    sync.Mutex
	id    string
	calls []string
}

func (f *fakeScanModule) ScanModule(kind string) (library.ScanModule, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if kind != "games" {
		return library.ScanModule{}, false
	}
	return library.ScanModule{ID: f.id, Name: "WittyPlay", Version: "0.7.0", On: true, Describe: true, Facet: "console"}, true
}

func (f *fakeScanModule) Call(ctx context.Context, module, call string, args, out any) error {
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()
	var answer any = struct{}{}
	switch call {
	case wittymodule.CallScanChoices:
		answer = wittymodule.ScanChoices{Facet: "console", Values: []wittymodule.ScanChoice{{ID: "nes", Name: "Nintendo Entertainment System", Short: "NES"}}}
	case wittymodule.CallScanIdentify:
		answer = wittymodule.ScanIdentified{}
	case wittymodule.CallScanDescribe:
		answer = wittymodule.ScanDescribed{}
	}
	b, _ := json.Marshal(answer)
	return json.Unmarshal(b, out)
}

// A module sets an item's fields, and asks for a scan, only in libraries of a kind it scans;
// a field a person typed stays, and a picture must be one it kept through PutImage.
func TestHostScanHooks(t *testing.T) {
	ts := newTestServer(t)
	ts.hostModsForTest = fakeHostModules{in: &wittymodule.Integration{Module: "wittyplay"}}
	scans := &fakeScanModule{id: "wittyplay"}
	ts.Scanner.Modules = scans
	api := httptest.NewServer(ts.HostAPI())
	defer api.Close()
	ctx := context.Background()
	c := host.NewAt(api.URL, "tok", nil)

	if h, err := c.Hello(ctx); err != nil || h.Offers[wittymodule.HostAPI] != 2 {
		t.Fatalf("host API 2: %+v %v", h, err)
	}
	games := ts.library(t, "games", t.TempDir())
	films := ts.library(t, "movies", t.TempDir())
	game := ts.item(t, &library.Item{LibraryID: games, Kind: "game", Path: "/g/a.nes", Title: "a", Console: "nes"})
	film := ts.item(t, &library.Item{LibraryID: films, Kind: "video", Path: "/f/a.mkv", Title: "A film"})

	if err := c.SetItem(ctx, wittymodule.ItemFields{ID: game.ID, Title: "A Game", Overview: "Found later.", Year: 1988}); err != nil {
		t.Fatal(err)
	}
	got, _ := ts.Store.Item(ctx, game.ID)
	if got.Title != "A Game" || got.Overview != "Found later." || got.Year == nil || *got.Year != 1988 {
		t.Errorf("%+v", got)
	}
	// What a person types in the item editor is theirs.
	_, admin := ts.user(t, "hooks-admin", "adult", true, "")
	if w := ts.do(t, "PUT", "/api/items/"+itoa64(game.ID), admin, map[string]any{"title": "Typed"}); w.Code != 200 {
		t.Fatalf("edit: %d %s", w.Code, w.Body)
	}
	if err := c.SetItem(ctx, wittymodule.ItemFields{ID: game.ID, Title: "Module's", Tagline: "USA"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := ts.Store.Item(ctx, game.ID); got.Title != "Typed" || got.Tagline != "USA" {
		t.Errorf("%q %q", got.Title, got.Tagline)
	}
	// The editor's Refresh asks the module that scans the library.
	if w := ts.do(t, "PUT", "/api/items/"+itoa64(game.ID), admin, map[string]any{"refresh": true}); w.Code != 200 {
		t.Fatalf("refresh: %d %s", w.Code, w.Body)
	}
	if !strings.Contains(strings.Join(scans.calls, " "), wittymodule.CallScanDescribe) {
		t.Errorf("refresh didn't ask the module: %v", scans.calls)
	}

	// Not an item of a kind of library it scans.
	if err := c.SetItem(ctx, wittymodule.ItemFields{ID: film.ID, Title: "Mine"}); !host.IsNotFound(err) {
		t.Errorf("a film: %v", err)
	}
	if err := c.SetItem(ctx, wittymodule.ItemFields{ID: 999999, Title: "Mine"}); !host.IsNotFound(err) {
		t.Errorf("no item: %v", err)
	}
	// Pictures: one it kept, not anything else.
	url, err := c.PutImage(ctx, game.ID, "poster", bytes.NewReader(append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetItem(ctx, wittymodule.ItemFields{ID: game.ID, Poster: url}); err != nil {
		t.Fatal(err)
	}
	if got, _ := ts.Store.Item(ctx, game.ID); !strings.HasPrefix(got.Poster, "cache:mod-wittyplay-") {
		t.Errorf("poster %q", got.Poster)
	}
	for _, bad := range []string{"/etc/passwd", "cache:x.png", strings.Replace(url, "/wittyplay/", "/wittytuner/", 1), "https://example.com/a.png"} {
		if err := c.SetItem(ctx, wittymodule.ItemFields{ID: game.ID, Poster: bad}); err == nil || !strings.Contains(err.Error(), "PutImage") {
			t.Errorf("%s: %v", bad, err)
		}
	}

	// A rescan of one of its libraries; not of another kind.
	empty := ts.library(t, "games", t.TempDir())
	if err := c.Rescan(ctx, empty); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(10 * time.Second); ts.Scanner.Running(empty) && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
	}
	if err := c.Rescan(ctx, films); !host.IsNotFound(err) {
		t.Errorf("a films library: %v", err)
	}

	// Another module scans games now: this one may touch none of them.
	scans.mu.Lock()
	scans.id = "wittyother"
	scans.mu.Unlock()
	if err := c.SetItem(ctx, wittymodule.ItemFields{ID: game.ID, Overview: "x"}); !host.IsNotFound(err) {
		t.Errorf("a module that doesn't scan games: %v", err)
	}
	if err := c.Rescan(ctx, games); !host.IsNotFound(err) {
		t.Errorf("a module that doesn't scan games: %v", err)
	}
}

// A games library needs a module that scans games, and its console is one of the module's.
func TestGamesLibraryNeedsItsModule(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "lib-admin", "adult", true, "")
	dir := t.TempDir()
	w := ts.do(t, "POST", "/api/libraries", admin, map[string]any{"name": "Games", "type": "games", "paths": []string{dir}})
	if w.Code != 409 || !strings.Contains(w.Body.String(), `"module-missing"`) {
		t.Fatalf("without the module: %d %s", w.Code, w.Body)
	}
	ts.Scanner.Modules = &fakeScanModule{id: "wittyplay"}
	if w := ts.do(t, "POST", "/api/libraries", admin, map[string]any{"name": "Games", "type": "games", "paths": []string{dir}, "console": "psx9"}); w.Code != 400 {
		t.Errorf("a console the module doesn't have: %d %s", w.Code, w.Body)
	}
	w = ts.do(t, "POST", "/api/libraries", admin, map[string]any{"name": "Games", "type": "games", "paths": []string{dir}, "console": "nes"})
	if w.Code != 201 || !strings.Contains(w.Body.String(), `"console":"nes"`) {
		t.Fatalf("with the module: %d %s", w.Code, w.Body)
	}
	var lib library.Library
	_ = json.Unmarshal(w.Body.Bytes(), &lib)
	for deadline := time.Now().Add(10 * time.Second); ts.Scanner.Running(lib.ID) && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
	}
	if w := ts.do(t, "PUT", "/api/libraries/"+itoa64(lib.ID), admin, map[string]any{"name": "Games", "type": "games", "paths": []string{dir}}); w.Code != 200 || !strings.Contains(w.Body.String(), `"console":"auto"`) {
		t.Errorf("auto: %d %s", w.Code, w.Body)
	}
	for deadline := time.Now().Add(10 * time.Second); ts.Scanner.Running(lib.ID) && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
	}
	// Cards name the console as the module does.
	ts.item(t, &library.Item{LibraryID: lib.ID, Kind: "game", Path: filepath.Join(dir, "a.nes"), Title: "A", Console: "nes"})
	if got := ts.consoleShort(context.Background(), "nes"); got != "NES" {
		t.Errorf("short name %q", got)
	}
	// Films need no module.
	if w := ts.do(t, "POST", "/api/libraries", admin, map[string]any{"name": "Films", "type": "movies", "paths": []string{t.TempDir()}}); w.Code != 201 {
		t.Errorf("films: %d %s", w.Code, w.Body)
	}
}

// An administrator gives a module secrets: never read back, only which are set, and written
// to the audit log without the value.
func TestModuleSecrets(t *testing.T) {
	ts := newTestServer(t)
	ts.Modules = modmgr.New(modmgr.Config{Dir: filepath.Join(t.TempDir(), "modules"), Server: "0.4.0", Known: true, Builtins: modmgr.ServerManifests()})
	_, admin := ts.user(t, "sec-admin", "adult", true, "")
	_, adult := ts.user(t, "sec-adult", "adult", false, "")
	const value = "s3cret-igdb-value"
	if w := ts.do(t, "PUT", "/api/modules/wittyplay/secrets/igdbClientSecret", adult, map[string]string{"value": value}); w.Code != 403 {
		t.Errorf("not an administrator: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/modules/wittyplay/secrets", adult, nil); w.Code != 403 {
		t.Errorf("not an administrator: %d", w.Code)
	}
	w := ts.do(t, "PUT", "/api/modules/wittyplay/secrets/igdbClientSecret", admin, map[string]string{"value": value})
	if w.Code != 200 || strings.Contains(w.Body.String(), value) {
		t.Fatalf("set: %d %s", w.Code, w.Body)
	}
	if got := ts.DB.GetSetting(context.Background(), secretKey("wittyplay", "igdbClientSecret"), ""); got != value {
		t.Errorf("kept %q", got)
	}
	w = ts.do(t, "GET", "/api/modules/wittyplay/secrets", admin, nil)
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"set":["igdbClientSecret"]}` {
		t.Errorf("list: %d %s", w.Code, w.Body)
	}
	// Another module's secrets are its own.
	if w := ts.do(t, "GET", "/api/modules/wittytuner/secrets", admin, nil); w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"set":[]}` {
		t.Errorf("another module: %d %s", w.Code, w.Body)
	}
	for _, bad := range []string{"/api/modules/nosuchmodule/secrets/x", "/api/modules/wittyplay/secrets/bad%20name"} {
		if w := ts.do(t, "PUT", bad, admin, map[string]string{"value": "v"}); w.Code != 404 && w.Code != 400 {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}
	var n int
	_ = ts.DB.QueryRow(`SELECT COUNT(*) FROM game_core_audit WHERE module = 'wittyplay' AND action = 'secret set' AND detail = 'igdbClientSecret'`).Scan(&n)
	var leaked int
	_ = ts.DB.QueryRow(`SELECT COUNT(*) FROM game_core_audit WHERE detail LIKE ? OR core LIKE ?`, "%"+value+"%", "%"+value+"%").Scan(&leaked)
	if n != 1 || leaked != 0 {
		t.Errorf("audit rows %d, with the value %d", n, leaked)
	}
	// "" removes it.
	if w := ts.do(t, "PUT", "/api/modules/wittyplay/secrets/igdbClientSecret", admin, map[string]string{"value": ""}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := ts.do(t, "GET", "/api/modules/wittyplay/secrets", admin, nil); strings.TrimSpace(w.Body.String()) != `{"set":[]}` {
		t.Errorf("after removing: %s", w.Body)
	}
}

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }
