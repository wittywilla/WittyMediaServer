// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymodule"
	"github.com/wittywilla/wittymodule/service"
)

// heard is what the fake module's service was sent.
type heard struct {
	Path   string
	Query  string
	Header http.Header
}

// withModule stands up a module service that answers anything with what it was sent, and has
// the server use an integration for it, as a version of API 5 in use would.
func withModule(t *testing.T, ts *testServer, in *wittymodule.Integration) *[]heard {
	t.Helper()
	var got []heard
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(service.TokenHeader) != "start-secret" {
			w.WriteHeader(403)
			return
		}
		got = append(got, heard{r.URL.Path, r.URL.RawQuery, r.Header.Clone()})
		w.Header().Set("Set-Cookie", "module=1")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(svc.Close)
	base, _ := url.Parse(svc.URL)
	old := service.Dial
	service.Dial = func(id string) (*service.Conn, error) {
		if id != in.Module {
			return nil, &modmgr.ErrMissing{ID: id, Name: id}
		}
		return &service.Conn{Base: base, Token: "start-secret", Client: svc.Client()}, nil
	}
	t.Cleanup(func() { service.Dial = old })
	gen := uint64(1)
	ts.integrations = func() ([]modmgr.InUse, uint64) {
		return []modmgr.InUse{{ID: in.Module, Name: "WittyPlay", Version: "0.5.0", Integration: in}}, gen
	}
	return &got
}

func playIntegration() *wittymodule.Integration {
	return &wittymodule.Integration{Format: wittymodule.IntegrationFormat, FormatVersion: 1, Module: "wittyplay", Feature: "games",
		HTTP: []wittymodule.Route{
			{Path: "/echo", Methods: []string{"GET"}, Auth: "user"},
			{Path: "/secret", Methods: []string{"GET"}, Auth: "admin"},
			{Path: "/careful", Methods: []string{"PUT"}, Auth: "admin", Confirm: true},
			{Path: "/upload", Methods: []string{"POST"}, Auth: "user", Limits: &wittymodule.Limits{Body: "64MB"}},
			{Path: "/games/{id}/play", Methods: []string{"POST"}, Auth: "user", Item: "id", Plays: true},
			{Path: "/desk/{sid}/{rest...}", Methods: []string{"GET"}, Auth: "ticket", CSP: "sandboxed-page"},
		}}
}

func TestModuleOwnRoutes(t *testing.T) {
	ts := newTestServer(t)
	got := withModule(t, ts, playIntegration())
	_, admin := ts.user(t, "boss", "adult", true, "")
	pal, palTok := ts.user(t, "pal", "adult", false, "")
	open := `"maxAge":-1,"allowUnrated":true,"liveTv":true,"libraryChannels":true,"tunerChannels":true`
	_, noGames := ts.user(t, "nogames", "adult", false, `{`+open+`,"off":["games"]}`)
	games := ts.library(t, "games", t.TempDir())
	game := ts.item(t, &library.Item{LibraryID: games, Kind: "game", Path: "/g/game.sfc", Title: "A game", ContentRating: "ESRB E"})
	last := func() heard {
		t.Helper()
		if len(*got) == 0 {
			t.Fatal("the module heard nothing")
		}
		return (*got)[len(*got)-1]
	}

	// A person's own route: who they are, and nothing of their sign-in or of what they forged.
	w := ts.do(t, "GET", "/api/modules/wittyplay/echo?x=1&token=abc", palTok, nil, "X-Witty-Admin", "1", "X-Witty-User", "1", "X-Witty-Limited", "1", "Cookie", "wms_token=t; other=1")
	if w.Code != 200 {
		t.Fatalf("echo: %d %s", w.Code, w.Body)
	}
	h := last()
	if h.Path != "/http/echo" || h.Query != "x=1" {
		t.Errorf("the module was sent %s?%s", h.Path, h.Query)
	}
	if h.Header.Get("X-Witty-User") != fmt.Sprint(pal.ID) || h.Header.Get("X-Witty-Admin") != "" || h.Header.Get("Authorization") != "" {
		t.Errorf("identity: %v", h.Header)
	}
	if c := h.Header.Get("Cookie"); strings.Contains(c, "wms_token") || !strings.Contains(c, "other=1") {
		t.Errorf("cookies: %q", c)
	}
	if !strings.Contains(h.Header.Get("X-Witty-Features"), "games") {
		t.Errorf("features: %q", h.Header.Get("X-Witty-Features"))
	}
	// Where the device reached the server, said by the server alone (a forged one is dropped).
	if h.Header.Get("X-Witty-Reached") == "" || h.Header.Get("X-Witty-Limited") != "" {
		t.Errorf("reached and limited: %v", h.Header)
	}
	if w.Header().Get("Set-Cookie") != "" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("the answer's headers: %v", w.Header())
	}
	// The same under /api/v1.
	if w := ts.do(t, "GET", "/api/v1/modules/wittyplay/echo", palTok, nil); w.Code != 200 {
		t.Errorf("v1: %d", w.Code)
	}
	// Nobody signed in, and someone without games.
	if w := ts.do(t, "GET", "/api/modules/wittyplay/echo", "", nil); w.Code != 401 {
		t.Errorf("signed out: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/modules/wittyplay/echo", noGames, nil); w.Code != 403 || !strings.Contains(w.Body.String(), "feature-off") {
		t.Errorf("games off: %d %s", w.Code, w.Body)
	}
	// Administrators' routes.
	if w := ts.do(t, "GET", "/api/modules/wittyplay/secret", palTok, nil); w.Code != 403 {
		t.Errorf("admin route, not an admin: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/modules/wittyplay/secret", admin, nil); w.Code != 200 || last().Header.Get("X-Witty-Admin") != "1" {
		t.Errorf("admin route: %d", w.Code)
	}
	// What it doesn't answer, and modules that aren't here.
	if w := ts.do(t, "GET", "/api/modules/wittyplay/nothing", palTok, nil); w.Code != 404 {
		t.Errorf("no such route: %d", w.Code)
	}
	if w := ts.do(t, "DELETE", "/api/modules/wittyplay/echo", palTok, nil); w.Code == 200 {
		t.Errorf("a method it doesn't answer: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/modules/wittytuner/echo", palTok, nil); w.Code == 200 {
		t.Errorf("another module: %d", w.Code)
	}

	// An item's route: checked, and described to the module with its file.
	n := len(*got)
	if w := ts.do(t, "POST", fmt.Sprintf("/api/modules/wittyplay/games/%d/play", game.ID), palTok, map[string]any{"resume": true}); w.Code != 200 {
		t.Fatalf("play: %d %s", w.Code, w.Body)
	}
	id := service.Who(&http.Request{Header: last().Header})
	if id.Item == nil || id.Item.ID != game.ID || id.Item.Kind != "game" || len(id.Item.Files) != 1 || id.Item.Files[0].Path != "/g/game.sfc" {
		t.Errorf("the item: %+v", id.Item)
	}
	if w := ts.do(t, "POST", "/api/modules/wittyplay/games/999999/play", palTok, nil); w.Code != 404 || len(*got) != n+1 {
		t.Errorf("an item that isn't there: %d", w.Code)
	}

	// A play route is held to screen time and the games feature like the server's own.
	if w := ts.do(t, "POST", fmt.Sprintf("/api/modules/wittyplay/games/%d/play", game.ID), noGames, nil); w.Code != 403 {
		t.Errorf("games off: %d", w.Code)
	}

	// A ticket route: nobody's identity, and its page boxed in.
	w = ts.do(t, "GET", "/api/modules/wittyplay/desk/ab12/index.html", "", nil, "Cookie", "wms_token=t")
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("ticket: %d %v", w.Code, w.Header())
	}
	if h := last(); h.Header.Get("X-Witty-User") != "" || h.Header.Get("Cookie") != "" || h.Path != "/http/desk/ab12/index.html" {
		t.Errorf("ticket route sent %s %v", h.Path, h.Header)
	}
	if w := ts.do(t, "GET", "/api/modules/wittyplay/desk/ab12/../../secret", "", nil); w.Code == 200 {
		t.Errorf("a path out of the route: %d", w.Code)
	}
}

// A route with "confirm": the server checks the password, and the module hears only that it was
// right. No route ever passes the password on.
func TestModuleRouteConfirm(t *testing.T) {
	ts := newTestServer(t)
	got := withModule(t, ts, playIntegration())
	_, admin := ts.user(t, "boss", "adult", true, "")
	pw := "X-WMS-Confirm-Password"
	last := func() http.Header { return (*got)[len(*got)-1].Header }
	if w := ts.do(t, "PUT", "/api/modules/wittyplay/careful", admin, nil, pw, "correct-horse-1"); w.Code != 200 || last().Get("X-Witty-Confirmed") != "1" || last().Get(pw) != "" {
		t.Errorf("right password: %d %v", w.Code, last())
	}
	n := len(*got)
	if w := ts.do(t, "PUT", "/api/modules/wittyplay/careful", admin, nil, pw, "wrong"); w.Code != 403 || len(*got) != n {
		t.Errorf("wrong password: %d, module heard %d", w.Code, len(*got)-n)
	}
	// The refusal is in the module's own rows of the audit log.
	if rows := ts.coreAudit(context.Background(), "wittyplay", 10); len(rows) != 1 || rows[0].Action != "password-refused" || rows[0].User != "boss" || rows[0].Detail != "PUT /careful" {
		t.Errorf("audit log: %+v", rows)
	}
	if w := ts.do(t, "PUT", "/api/modules/wittyplay/careful", admin, nil); w.Code != 200 || last().Get("X-Witty-Confirmed") != "" {
		t.Errorf("no password: %d %v", w.Code, last())
	}
	if w := ts.do(t, "PUT", "/api/modules/wittyplay/careful", admin, nil, "X-Witty-Confirmed", "1"); w.Code != 200 || last().Get("X-Witty-Confirmed") != "" {
		t.Errorf("a forged confirmation: %v", last())
	}
	if w := ts.do(t, "GET", "/api/modules/wittyplay/secret", admin, nil, pw, "correct-horse-1"); w.Code != 200 || last().Get(pw) != "" || last().Get("X-Witty-Confirmed") != "" {
		t.Errorf("a route without confirm: %v", last())
	}
}

// A module that isn't here answers 424, saying which.
func TestModuleRoutesOfAMissingModule(t *testing.T) {
	ts := newTestServer(t)
	ts.integrations = func() ([]modmgr.InUse, uint64) { return nil, 1 }
	_, tok := ts.user(t, "pal", "adult", false, "")
	if w := ts.do(t, "GET", "/api/modules/wittyplay/echo", tok, nil); w.Code != 404 && w.Code != 424 {
		t.Errorf("a module route with no module: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/modules/wittyplay/echo", "", nil); w.Code != 401 {
		t.Errorf("signed out: %d", w.Code)
	}
}

func TestIntegrationCheck(t *testing.T) {
	if err := IntegrationCheck("x", &wittymodule.Integration{Feature: "games"}); err != nil {
		t.Error(err)
	}
	if err := IntegrationCheck("x", &wittymodule.Integration{Feature: "teleporting"}); err == nil {
		t.Error("an unknown feature passed")
	}
	_ = json.Valid
}

// A module's route takes the body its integration allows, past the server's general limit;
// anything else under /api/modules/ keeps that limit.
func TestModuleRouteBodyLimit(t *testing.T) {
	ts := newTestServer(t)
	withModule(t, ts, playIntegration())
	for path, big := range map[string]bool{"/api/modules/wittyplay/upload": true, "/api/v1/modules/wittyplay/upload": true,
		"/api/modules/wittyplay/echo": false, "/api/modules/wittyplay/nope": false, "/api/modules/wittycomics/upload": false, "/api/modules/audit": false} {
		r := httptest.NewRequest("POST", path, nil)
		if got := ts.bigUploadPath(r); got != big {
			t.Errorf("%s: %v", path, got)
		}
	}
}
