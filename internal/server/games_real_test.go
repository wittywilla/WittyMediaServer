package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/wittywilla/wittymediaserver/internal/modules/modtest"
)

// TestRealGameThroughTheServer plays one real game through everything a browser's game goes
// through: the library's scan, the play route and its checks, the module, the sandboxed
// runner, the encoder and the WebSocket. It needs a core and a game, so it only runs when told:
//
//	WMS_TEST_CATALOG=<catalogue> WMS_WITTYPLAY_RUNNER=<wittyplay-game-runner> \
//	WMS_TEST_REAL_GAME="<console>|<core id>|<core file>|<game folder>" go test ./internal/server -run TestRealGameThroughTheServer
//
// The folder holds the game and nothing else, and is only read. Nothing here needs the game
// to be any particular one.
func TestRealGameThroughTheServer(t *testing.T) {
	spec := os.Getenv("WMS_TEST_REAL_GAME")
	if spec == "" {
		t.Skip("WMS_TEST_REAL_GAME isn't set")
	}
	modtest.Need(t, "wittyplay")
	parts := strings.Split(spec, "|")
	if len(parts) != 4 {
		t.Fatalf("WMS_TEST_REAL_GAME is %q", spec)
	}
	console, coreID, coreFile, folder := parts[0], parts[1], parts[2], parts[3]
	g := newGameServer(t)
	g.s.Tools.Encoders["libx264"] = true
	dir := g.s.games().cores.Dir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(coreFile, filepath.Join(dir, coreID+"_libretro.so")); err != nil {
		t.Fatal(err)
	}
	it := addGame(t, g, console, folder)
	w := g.do("POST", "/api/games/"+itoa64(it.ID)+"/play", g.admin, map[string]any{"client": "browser"})
	if w.Code != 201 {
		t.Fatalf("play: %d %s", w.Code, w.Body.String())
	}
	var info struct {
		ID   string `json:"id"`
		Core string `json:"core"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	t.Logf("%q is playing on %s", it.Title, info.Core)
	srv := httptest.NewServer(g.h)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/api/play/"+info.ID+"/ws?video=h264",
		&websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + g.admin}}})
	if err != nil {
		if resp != nil {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("connecting: %v: %s", err, b)
		}
		t.Fatal(err)
	}
	defer c.CloseNow()
	c.SetReadLimit(64 << 20)
	pictures, sound, different := 0, 0, map[string]bool{}
	began := time.Now()
	for pictures < 300 {
		kind, b, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("after %d pictures in %s: %v", pictures, time.Since(began).Round(time.Second), err)
		}
		if kind != websocket.MessageBinary || len(b) < 2 {
			continue
		}
		switch b[0] {
		case 1, 2:
			pictures++
			different[string(b[2:min(len(b), 64)])+itoa64(int64(len(b)))] = true
		case 3:
			sound++
		}
	}
	took := time.Since(began).Seconds()
	t.Logf("%d pictures in %.1f s (%.0f a second), %d of them looking different, and %d lumps of sound", pictures, took, float64(pictures)/took, len(different), sound)
	if len(different) < 2 {
		t.Error("every picture was the same")
	}
	if n := g.do("DELETE", "/api/play/"+info.ID, g.admin, nil); n.Code != 200 {
		t.Errorf("stopping: %d", n.Code)
	}
}
