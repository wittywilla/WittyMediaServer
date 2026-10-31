package lookup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wittywilla/wittycd"
)

const rel = "11111111-2222-3333-4444-555555555555"

func TestFindSendsOnlyTheTableOfContents(t *testing.T) {
	toc := wittycd.TOC{First: 1, Last: 2, LeadOut: 600, Offsets: []int{150, 375}}
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.String())
		if got := r.Header.Get("User-Agent"); got != "Test/1.0 ( https://example.org )" {
			t.Errorf("user agent = %q", got)
		}
		json.NewEncoder(w).Encode(map[string]any{"releases": []any{
			map[string]any{"id": rel, "title": "Album", "date": "1999-03-02", "artist-credit": []any{map[string]any{"name": "Band"}},
				"cover-art-archive": map[string]any{"front": true},
				"media": []any{map[string]any{"position": 1, "track-count": 2, "discs": []any{map[string]any{"id": toc.MusicBrainzID()}},
					"tracks": []any{map[string]any{"position": 1, "title": "One"}, map[string]any{"position": 2, "title": "Two", "artist-credit": []any{map[string]any{"name": "Guest"}}}}}}},
			map[string]any{"id": "not an id", "title": "Ignored"},
		}})
	}))
	defer srv.Close()
	c := &Client{UserAgent: "Test/1.0 ( https://example.org )", Base: srv.URL}
	ms, err := c.Find(context.Background(), toc)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 1 || !ms[0].Exact || ms[0].Title != "Album" || ms[0].Year() != 1999 || ms[0].Tracks[0].Artist != "" || ms[0].Tracks[1].Artist != "Guest" {
		t.Fatalf("matches = %+v", ms)
	}
	if !strings.Contains(asked[0], "toc=1+2+600+150+375") || !strings.Contains(asked[0], toc.MusicBrainzID()) {
		t.Fatalf("asked %v", asked)
	}
}

func TestUnknownDiscAndABusyService(t *testing.T) {
	code := http.StatusNotFound
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) }))
	defer srv.Close()
	c := &Client{Base: srv.URL}
	toc := wittycd.TOC{First: 1, Last: 1, LeadOut: 600, Offsets: []int{150}}
	if ms, err := c.Find(context.Background(), toc); err != nil || ms == nil || len(ms) != 0 {
		t.Fatalf("%v, %v", ms, err)
	}
	code = http.StatusServiceUnavailable
	if _, err := c.Find(context.Background(), toc); err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("err = %v", err)
	}
}

func TestSameAlbum(t *testing.T) {
	a := Match{Title: "Same", Artist: "Band", Exact: true, Tracks: []TrackName{{Number: 1, Title: "One"}}}
	b := a
	if !SameAlbum([]Match{a, b}) {
		t.Fatal("two pressings of one album should need no asking")
	}
	c := a
	c.Exact = false
	if SameAlbum([]Match{a, c}) {
		t.Fatal("a close match isn't good enough")
	}
	d := a
	d.Tracks = []TrackName{{Number: 1, Title: "Different"}}
	if SameAlbum([]Match{a, d}) {
		t.Fatal("different tracks are a different album")
	}
	if SameAlbum(nil) {
		t.Fatal("nothing isn't an album")
	}
}

func TestBadIDs(t *testing.T) {
	c := &Client{}
	if _, err := c.Cover(context.Background(), "../../etc", 250); err == nil {
		t.Fatal("a bad ID should never reach the network")
	}
	if _, err := c.Release(context.Background(), "x", wittycd.TOC{}); err == nil {
		t.Fatal("a bad ID should never reach the network")
	}
}
