// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/modules/modtest"
)

// A server with games libraries: what the server keeps of games (their scan, review and
// pictures). Playing them is WittyPlay's.
type gameServer struct {
	h          http.Handler
	admin, kid string // session tokens
	other      string
	s          *Server
}

func newGameServer(t *testing.T) *gameServer {
	t.Helper()
	modtest.Need(t, "wittyplay")
	ctx := context.Background()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := &library.Store{DB: d}
	s := &Server{Cfg: &config.Config{DataDir: t.TempDir(), CacheDir: t.TempDir(), Dev: true}, DB: d, Log: log, Store: store, Auth: &auth.Service{DB: d},
		Tools: &ffmpeg.Tools{FFmpeg: "ffmpeg", Encoders: map[string]bool{}}, Scanner: library.NewScanner(store, nil, t.TempDir(), func() string { return "" }, log)}
	token := func(name string, admin bool) string {
		if _, err := s.Auth.CreateUser(ctx, name, "a long enough password", admin); err != nil {
			t.Fatal(err)
		}
		tok, _, err := s.Auth.Login(ctx, name, "a long enough password", "test", false)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	t.Cleanup(s.Close)
	return &gameServer{s: s, h: s.Handler(), admin: token("admin", true), other: token("other", false), kid: token("third", false)}
}

func (g *gameServer) do(method, path, tok string, body any) *httptest.ResponseRecorder {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	req.RemoteAddr = "192.168.1.50:4000"
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	w := httptest.NewRecorder()
	g.h.ServeHTTP(w, req)
	return w
}

func jsonBody(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("%v: %s", err, w.Body.String())
	}
}

func itoa64(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// runnerGameItem scans a library of the test console's one game and gives the game.
func runnerGameItem(t *testing.T, g *gameServer) int64 {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Game.wittytest"), []byte("game"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := g.do("POST", "/api/libraries", g.admin, map[string]any{"name": "Test", "type": "games", "console": "wittytest", "paths": []string{dir}})
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var lib library.Library
	_ = json.Unmarshal(w.Body.Bytes(), &lib)
	for range 200 {
		items, _, err := g.s.Store.Items(context.Background(), library.ItemQuery{LibraryID: lib.ID})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) > 0 {
			return items[0].ID
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the scan did not finish in time")
	return 0
}

func TestLibraryConsoleValidation(t *testing.T) {
	g := newGameServer(t)
	dir := t.TempDir()
	mk := func(console string) int {
		return g.do("POST", "/api/libraries", g.admin, map[string]any{"name": "Games", "type": "games", "console": console, "paths": []string{dir}}).Code
	}
	if c := mk("nope"); c != 400 {
		t.Fatalf("an unknown console: got %d", c)
	}
	// Two libraries of one console, with their own names.
	if c := mk("snes"); c != 201 {
		t.Fatalf("snes: got %d", c)
	}
	if c := mk("snes"); c != 201 {
		t.Fatalf("a second snes library: got %d", c)
	}
	w := g.do("GET", "/api/libraries", g.admin, nil)
	var libs []library.Library
	_ = json.Unmarshal(w.Body.Bytes(), &libs)
	if len(libs) != 2 || libs[0].Console != "snes" || libs[1].Console != "snes" {
		t.Fatalf("got %+v", libs)
	}
}
