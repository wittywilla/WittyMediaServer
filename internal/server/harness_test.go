package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/bdsession"
	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/themes"
)

// testServer is a real server with its own data folder, no FFmpeg, and a
// handler to send requests to.
type testServer struct {
	*Server
	h http.Handler
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	tools := &ffmpeg.Tools{Encoders: map[string]bool{}}
	h := hls.NewManager(tools, filepath.Join(dir, "transcode"), 2, log)
	store := &library.Store{DB: d}
	s := &Server{
		Cfg: &config.Config{DataDir: dir, ImagesDir: filepath.Join(dir, "images"), CacheDir: filepath.Join(dir, "cache"), MaxTranscode: 2}, DB: d, Log: log, Tools: tools, Store: store,
		Scanner: library.NewScanner(store, tools, filepath.Join(dir, "cache"), func() string { return "" }, log),
		Auth:    &auth.Service{DB: d}, HLS: h, DVD: dvdsession.NewManager(h, tools, dir, "/api", log), BD: bdsession.NewManager(h, tools, dir, "/api", log),
		Themes: &themes.Manager{Dir: filepath.Join(dir, "themes")}, Web: fstest.MapFS{"index.html": {Data: []byte("<html></html>")}}, Version: "test",
	}
	return &testServer{Server: s, h: s.Handler()}
}

// user makes an account and returns a session token for it.
func (ts *testServer) user(t *testing.T, name, kind string, admin bool, restrictions string) (*auth.User, string) {
	t.Helper()
	ctx := context.Background()
	u, err := ts.Auth.CreateProfile(ctx, name, "correct-horse-1", kind, admin, restrictions)
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := ts.Auth.Login(ctx, name, "correct-horse-1", "test", false)
	if err != nil {
		t.Fatal(err)
	}
	return u, tok
}

// do sends a request from this machine, signed in with token when given.
func (ts *testServer) do(t *testing.T, method, target, token string, body any, hdr ...string) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case []byte:
		rd = bytes.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		rd = bytes.NewReader(raw)
	}
	r := httptest.NewRequest(method, target, rd)
	r.Host = "127.0.0.1:9797"
	r.RemoteAddr = "127.0.0.1:50000"
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		if hdr[i] == "Host" {
			r.Host = hdr[i+1]
		} else if hdr[i] == "RemoteAddr" {
			r.RemoteAddr = hdr[i+1]
		} else {
			r.Header.Set(hdr[i], hdr[i+1])
		}
	}
	w := httptest.NewRecorder()
	ts.h.ServeHTTP(w, r)
	return w
}

// library makes a library on a folder and returns its id.
func (ts *testServer) library(t *testing.T, typ string, paths ...string) int64 {
	t.Helper()
	l, err := ts.Store.CreateLibrary(context.Background(), typ, typ, paths)
	if err != nil {
		t.Fatal(err)
	}
	return l.ID
}

// item stores an item directly, without scanning.
func (ts *testServer) item(t *testing.T, it *library.Item) *library.Item {
	t.Helper()
	if it.Title == "" {
		it.Title = "title"
	}
	it.SortTitle = it.Title
	if err := ts.Store.UpdateItem(context.Background(), it); err != nil {
		t.Fatal(err)
	}
	return it
}
