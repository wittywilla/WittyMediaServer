// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
)

// The apps' contract while modules move out of the server (docs/plans/MODULAR-MODULES.md,
// "Client apps"). The Apple and Android apps call these addresses; what each answers (its
// status, and the shape of its JSON: every key and the kind of every value) is kept in
// testdata/contract/<state>.json and must not change when a module's feature moves from the
// server's own copy into the module. Each state is a server as an app might meet it:
//
//	absent   the module isn't on the server (an app hides what it can't use; 424 says why)
//	legacy   a version of API 3 or 4 is in use: the server's own copy answers (WMS_TEST_CATALOG)
//	api5     a version of API 5 answers through its routes and aliases (WMS_TEST_CATALOG)
//
// WMS_UPDATE_CONTRACT=1 writes the files from what the server answers now: only ever on purpose,
// with the change agreed with the apps.

type contractCall struct {
	name, method, path string
	body               any
	admin              bool
}

// contractCalls are the apps' calls, from WittyMediaClient (apple/WittyKit GamesAPI.swift,
// GameStreamV2.swift; android net/*), with {game} and {lib} filled in from the seeded library.
var contractCalls = []contractCall{
	{name: "capabilities", method: "GET", path: "/api/v1/capabilities"},
	{name: "items games", method: "GET", path: "/api/v1/items?library={lib}&kind=game&sort=title&limit=50"},
	{name: "items games console", method: "GET", path: "/api/v1/items?library={lib}&kind=game&console=snes"},
	{name: "games home", method: "GET", path: "/api/v1/games/home?library={lib}"},
	{name: "games consoles", method: "GET", path: "/api/v1/games/consoles"},
	{name: "games check", method: "GET", path: "/api/v1/games/{game}/check"},
	{name: "games metadata", method: "GET", path: "/api/v1/games/{game}/metadata"},
	{name: "games versions", method: "GET", path: "/api/v1/games/{game}/versions"},
	{name: "games saves", method: "GET", path: "/api/v1/games/{game}/saves"},
	{name: "games settings", method: "GET", path: "/api/v1/games/settings?item={game}"},
	{name: "games meta", method: "GET", path: "/api/v1/games/{game}/meta"},
	{name: "games prepare", method: "POST", path: "/api/v1/games/{game}/prepare", body: map[string]any{}},
	{name: "games play", method: "POST", path: "/api/v1/games/{game}/play", body: map[string]any{"resume": false, "client": "native"}},
	{name: "console controls", method: "GET", path: "/api/v1/consoles/snes/controls?device=phone&game={game}"},
	{name: "profiles resolve", method: "GET", path: "/api/v1/game-profiles/resolve?kind=gamepad&controller=x&console=snes&game={game}"},
	{name: "game background", method: "GET", path: "/api/v1/game-background"},
	{name: "play session", method: "GET", path: "/api/v1/play/0123456789abcdef"},
	{name: "couch", method: "GET", path: "/api/v1/couch"},
	{name: "rigs", method: "GET", path: "/api/v1/rigs"},
	{name: "sdui games page", method: "GET", path: "/api/v1/sdui/pages/games"},
	{name: "module settings", method: "GET", path: "/api/v1/sdui/modules/settings"},
}

// shapeOf is a JSON value with every value replaced by its kind, and every list by the shape
// of its first member: what an app's decoder depends on, without what changes from run to run.
func shapeOf(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			out[k] = shapeOf(e)
		}
		return out
	case []any:
		if len(x) == 0 {
			return []any{}
		}
		return []any{shapeOf(x[0])}
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "bool"
	case nil:
		return "null"
	}
	return fmt.Sprintf("%T", v)
}

type contractAnswer struct {
	Status int    `json:"status"`
	Type   string `json:"type,omitempty"`
	Shape  any    `json:"shape,omitempty"`
}

func runContract(t *testing.T, ts *testServer, state string) {
	t.Helper()
	_, admin := ts.user(t, "boss", "adult", true, "")
	_, pal := ts.user(t, "pal", "adult", false, "")
	lib := ts.library(t, "games", t.TempDir())
	game := ts.item(t, &library.Item{LibraryID: lib, Kind: "game", Path: "/g/a.sfc", Title: "A game", Console: "snes", ContentRating: "ESRB E"})
	got := map[string]contractAnswer{}
	for _, c := range contractCalls {
		path := strings.NewReplacer("{lib}", fmt.Sprint(lib), "{game}", fmt.Sprint(game.ID)).Replace(c.path)
		tok := pal
		if c.admin {
			tok = admin
		}
		w := ts.do(t, c.method, path, tok, c.body)
		a := contractAnswer{Status: w.Code, Type: strings.Split(w.Header().Get("Content-Type"), ";")[0]}
		var v any
		if a.Type == "application/json" && json.Unmarshal(w.Body.Bytes(), &v) == nil {
			a.Shape = shapeOf(v)
		}
		got[c.name] = a
	}
	file := filepath.Join("testdata", "contract", state+".json")
	if os.Getenv("WMS_UPDATE_CONTRACT") == "1" {
		b, _ := json.MarshalIndent(got, "", "  ")
		_ = os.MkdirAll(filepath.Dir(file), 0o755)
		if err := os.WriteFile(file, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", file)
		return
	}
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("no %s: WMS_UPDATE_CONTRACT=1 writes it", file)
	}
	var want map[string]contractAnswer
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(contractCalls))
	for _, c := range contractCalls {
		names = append(names, c.name)
	}
	sort.Strings(names)
	for _, n := range names {
		w, ok := want[n]
		if !ok {
			t.Errorf("%s: not in %s (WMS_UPDATE_CONTRACT=1 adds it)", n, file)
			continue
		}
		g := got[n]
		gb, _ := json.Marshal(g)
		wb, _ := json.Marshal(w)
		if string(gb) != string(wb) {
			t.Errorf("%s answers differently from what the apps were promised (%s):\n got %s\nwant %s", n, state, gb, wb)
		}
	}
}

// With the module absent, every games address an app calls answers as it does today.
func TestAppContractModuleAbsent(t *testing.T) {
	ts := newTestServer(t)
	ts.Modules = modmgr.New(modmgr.Config{Dir: t.TempDir(), Server: "0.9.0-alpha", Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ts.h = ts.Handler()
	runContract(t, ts, "absent")
}
