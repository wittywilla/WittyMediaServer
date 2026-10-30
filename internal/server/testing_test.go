package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/bdsession"
	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/themes"
)

// contractServer is a real server over a temporary database, without FFmpeg.
type contractServer struct {
	*Server
	h http.Handler
}

func newContractServer(t *testing.T) *contractServer {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := hls.NewManager(nil, dir+"/hls", 2, log)
	s := &Server{
		Cfg: &config.Config{CacheDir: dir + "/cache", ImagesDir: dir + "/images"}, DB: d, Log: log, Store: &library.Store{DB: d},
		Auth: &auth.Service{DB: d}, HLS: h, DVD: dvdsession.NewManager(h, nil, dir+"/dvd", "/api", log), BD: bdsession.NewManager(h, nil, dir+"/bd", "/api", log),
		Themes: &themes.Manager{Dir: dir + "/themes"}, Web: fstest.MapFS{"index.html": {Data: []byte("<html>app</html>")}}, Version: "test",
	}
	return &contractServer{Server: s, h: s.Handler()}
}

// do makes a request; body may be nil, a string or anything JSON encodes.
func (ts *contractServer) do(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		rd = bytes.NewReader(raw)
	}
	r := httptest.NewRequest(method, path, rd)
	r.RemoteAddr = "192.168.1.50:4000"
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	ts.h.ServeHTTP(w, r)
	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("not JSON (%d): %q", w.Code, w.Body.String())
	}
	return v
}

// account makes an account and signs it in.
func (ts *contractServer) account(t *testing.T, name, kind string, admin bool) (*auth.User, string) {
	t.Helper()
	u, err := ts.Auth.CreateProfile(context.Background(), name, "password", kind, admin, "")
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := ts.Auth.Login(context.Background(), name, "password", "test", false)
	if err != nil {
		t.Fatal(err)
	}
	return u, tok
}

// item adds a library item directly.
func (ts *contractServer) item(t *testing.T, kind, path string) int64 {
	t.Helper()
	ctx := context.Background()
	libs, _ := ts.Store.Libraries(ctx)
	var lib int64
	if len(libs) > 0 {
		lib = libs[0].ID
	} else {
		l, err := ts.Store.CreateLibrary(ctx, "Test", "movies", []string{t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		lib = l.ID
	}
	res, err := ts.DB.ExecContext(ctx, `INSERT INTO items(library_id, kind, path, title, sort_title, added_at, updated_at) VALUES(?,?,?,?,?,1,1)`, lib, kind, path, path, path)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}
