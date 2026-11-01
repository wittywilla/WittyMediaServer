package library

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAgentLookup(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/poster.png":
			w.Write(png)
		case "/lookup":
			if r.Header.Get("X-Key") != "s3cret" {
				w.WriteHeader(401)
				return
			}
			if r.URL.Query().Get("title") != "The Treeline" || r.URL.Query().Get("year") != "2012" || r.URL.Query().Get("type") != "movie" {
				w.WriteHeader(404)
				return
			}
			io.WriteString(w, `{"results":[{"id":"lx","title":"The Treeline","overview":"Trees.","genres":["Family"],"rating":6.4,"runtimeMinutes":86,"poster":"`+"http://"+r.Host+`/poster.png"}]}`)
		}
	}))
	defer srv.Close()
	a := Agent{ID: "home", Name: "Home catalogue", URL: srv.URL + "/lookup?title={title}&year={year}&type={type}", Header: "X-Key: s3cret", Kinds: []string{"movies"}, Enabled: true}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	s := &Scanner{ImagesDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Agents: func() []Agent { return []Agent{a} }}
	y := 2012
	it := &Item{Title: "The Treeline", Year: &y}
	s.applyAgents(context.Background(), it, "movies")
	if it.Overview != "Trees." || len(it.Genres) != 1 || it.Rating == nil || it.RuntimeSec != 86*60 || it.Provider != "agent:home" {
		t.Fatalf("not applied: %+v", it)
	}
	if len(it.Poster) < 7 || it.Poster[:6] != "cache:" {
		t.Fatalf("poster not stored: %q", it.Poster)
	}
	// Other kinds of library are not its business.
	other := &Item{Title: "The Treeline"}
	s.applyAgents(context.Background(), other, "books")
	if other.Overview != "" {
		t.Fatal("agent ran for a library type it doesn't serve")
	}
	// A bad address is refused.
	bad := Agent{ID: "x", Name: "x", URL: "file:///etc/passwd?{title}", Kinds: []string{"movies"}}
	if bad.Validate() == nil {
		t.Fatal("file: address accepted")
	}
}
