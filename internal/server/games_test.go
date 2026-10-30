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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

type gameServer struct {
	h           http.Handler
	admin, kid  string // session tokens
	other       string
	s           *Server
	lib, itemID int64
}

func newGameServer(t *testing.T) *gameServer {
	t.Helper()
	ctx := context.Background()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := &library.Store{DB: d}
	s := &Server{Cfg: &config.Config{DataDir: t.TempDir(), Dev: true}, DB: d, Log: log, Store: store, Auth: &auth.Service{DB: d},
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
	g := &gameServer{s: s, h: s.Handler(), admin: token("admin", true), other: token("other", false), kid: token("third", false)}
	return g
}

func (g *gameServer) do(method, path, tok string, body any) *httptest.ResponseRecorder {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	w := httptest.NewRecorder()
	g.h.ServeHTTP(w, req)
	return w
}

func TestGamesAPIAccess(t *testing.T) {
	g := newGameServer(t)
	defer g.s.StopGames()
	cases := []struct {
		name, method, path, tok string
		want                    int
	}{
		{"consoles need a sign-in", "GET", "/api/games/consoles", "", 401},
		{"playing needs a sign-in", "POST", "/api/games/1/play", "", 401},
		{"session control needs a sign-in", "POST", "/api/play/abc/action", "", 401},
		{"streaming needs a sign-in", "GET", "/api/play/abc/ws", "", 401},
		{"only admins see gaming status", "GET", "/api/gaming", g.other, 403},
		{"only admins install cores", "POST", "/api/gaming/cores/fceumm/install", g.other, 403},
		{"only admins remove cores", "DELETE", "/api/gaming/cores/fceumm", g.other, 403},
		{"an unknown session does not exist", "GET", "/api/play/nope", g.other, 404},
		{"an unknown game does not exist", "POST", "/api/games/999/play", g.other, 404},
		{"a core we do not offer", "POST", "/api/gaming/cores/snes9x/install", g.admin, 404},
		{"a path is not a core name", "POST", "/api/gaming/cores/..%2Fevil/install", g.admin, 404},
	}
	for _, c := range cases {
		var body any
		if c.method == "POST" {
			body = map[string]any{}
		}
		if w := g.do(c.method, c.path, c.tok, body); w.Code != c.want {
			t.Errorf("%s: got %d, want %d: %s", c.name, w.Code, c.want, w.Body.String())
		}
	}
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

// Someone else's game, save or session stays out of reach, and a save slot can
// only be named by its number.
func TestGameSessionsBelongToTheirOwner(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no FFmpeg on PATH")
	}
	runner := filepath.Join(t.TempDir(), "wittyplay-host")
	if runtime.GOOS == "windows" {
		runner += ".exe"
	}
	build := exec.Command("go", "build", "-o", runner, "./cmd/wittyplay-host")
	build.Dir = filepath.Join("..", "..", "modules", "wittyplay")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	t.Setenv("WMS_WITTYPLAY_RUNNER", runner)

	g := newGameServer(t)
	defer g.s.StopGames()
	g.s.Tools.FFmpeg = ffmpegPath
	ctx := context.Background()
	dir := t.TempDir()
	rom := filepath.Join(dir, "Game.wittytest")
	if err := os.WriteFile(rom, []byte("game"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := g.do("POST", "/api/libraries", g.admin, map[string]any{"name": "Test", "type": "games", "console": "wittytest", "paths": []string{dir}})
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var lib library.Library
	_ = json.Unmarshal(w.Body.Bytes(), &lib)
	var item int64
	for i := 0; i < 200 && item == 0; i++ {
		items, _, err := g.s.Store.Items(ctx, library.ItemQuery{LibraryID: lib.ID})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) > 0 {
			item = items[0].ID
		}
		time.Sleep(50 * time.Millisecond)
	}
	if item == 0 {
		t.Fatal("the scan did not finish in time")
	}
	idPath := func(p string) string { return strings.ReplaceAll(p, "{id}", itoa64(item)) }

	w = g.do("POST", idPath("/api/games/{id}/play"), g.admin, map[string]any{})
	if w.Code != 201 {
		t.Fatalf("play: %d %s", w.Code, w.Body.String())
	}
	var info struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &info)
	for name, c := range map[string]struct {
		method, path string
		tok          string
		want         int
	}{
		"owner sees it":          {"GET", "/api/play/" + info.ID, g.admin, 200},
		"someone else does not":  {"GET", "/api/play/" + info.ID, g.other, 404},
		"nor can they stop it":   {"DELETE", "/api/play/" + info.ID, g.other, 404},
		"nor stream it":          {"GET", "/api/play/" + info.ID + "/ws", g.other, 404},
		"a bad slot is refused":  {"GET", idPath("/api/games/{id}/saves/10/thumb"), g.admin, 404},
		"a path is not a slot":   {"GET", idPath("/api/games/{id}/saves/..%2F..%2Fsecret/thumb"), g.admin, 404},
		"nor a negative one":     {"DELETE", idPath("/api/games/{id}/saves/-1"), g.admin, 404},
		"their saves are theirs": {"GET", idPath("/api/games/{id}/saves"), g.other, 200},
	} {
		if w := g.do(c.method, c.path, c.tok, nil); w.Code != c.want {
			t.Errorf("%s: got %d, want %d: %s", name, w.Code, c.want, w.Body.String())
		}
	}
	if w := g.do("POST", "/api/play/"+info.ID+"/webrtc", g.other, map[string]any{"sdp": "x"}); w.Code != 404 {
		t.Errorf("someone else's offer: got %d", w.Code)
	}
	if w := g.do("POST", "/api/play/"+info.ID+"/action", g.other, map[string]any{"type": "reset"}); w.Code != 404 {
		t.Errorf("someone else's action: got %d", w.Code)
	}
	if w := g.do("POST", "/api/play/"+info.ID+"/action", g.admin, map[string]any{"type": "save", "slot": 0}); w.Code != 422 {
		t.Errorf("the automatic slot isn't for saving by hand: got %d", w.Code)
	}
	if w := g.do("POST", "/api/play/"+info.ID+"/action", g.admin, map[string]any{"type": "save", "slot": 3}); w.Code != 200 {
		t.Errorf("saving: got %d: %s", w.Code, w.Body.String())
	}
	if w := g.do("DELETE", "/api/play/"+info.ID, g.admin, nil); w.Code != 200 {
		t.Errorf("stopping: got %d", w.Code)
	}
	w = g.do("GET", idPath("/api/games/{id}/saves"), g.other, nil)
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Errorf("another account must not see these saves: %s", w.Body.String())
	}
}

func itoa64(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
