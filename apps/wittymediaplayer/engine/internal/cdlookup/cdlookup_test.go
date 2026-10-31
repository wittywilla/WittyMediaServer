package cdlookup

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const answer = `{"releases":[
 {"id":"11111111-1111-1111-1111-111111111111","title":"Only Fits","date":"1999","artist-credit":[{"name":"Someone"}],
  "cover-art-archive":{"front":true},
  "media":[{"position":1,"track-count":2,"discs":[{"id":"other"}],"tracks":[{"position":1,"title":"A"},{"position":2,"title":"B"}]}]},
 {"id":"22222222-2222-2222-2222-222222222222","title":"The One","date":"2001-05-01","country":"GB",
  "artist-credit":[{"name":"First","joinphrase":" & "},{"name":"Second"}],
  "media":[{"position":1,"track-count":9,"discs":[{"id":"x"}],"tracks":[]},
           {"position":2,"track-count":2,"discs":[{"id":"abc_-."}],"tracks":[
    {"position":1,"title":"Song","artist-credit":[{"name":"First","joinphrase":" & "},{"name":"Second"}]},
    {"position":2,"title":"Guest","artist-credit":[{"name":"Guest Singer"}]}]}]},
 {"id":"not-a-release-id","title":"Bad","media":[{"track-count":2}]},
 {"id":"33333333-3333-3333-3333-333333333333","title":"Too Few","media":[{"track-count":7,"tracks":[]}]}
]}`

func serve(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	return &Client{HTTP: s.Client(), Base: s.URL, CoverBase: s.URL}
}

func TestLookup(t *testing.T) {
	var asked string
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.String()
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "WittyMediaPlayer/") {
			t.Errorf("agent %q", r.Header.Get("User-Agent"))
		}
		w.Write([]byte(answer))
	})
	got, err := c.Lookup(context.Background(), "abc_-.", "1+2+1000+150+500", 2)
	if err != nil {
		t.Fatal(err)
	}
	// What's sent is the disc's numbers and what to include, and nothing else.
	if !strings.HasPrefix(asked, "/discid/abc_-.?") || !strings.HasSuffix(asked, "&toc=1+2+1000+150+500") {
		t.Errorf("asked %s", asked)
	}
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	m := got[0]
	if m.Title != "The One" || !m.Exact || m.Artist != "First & Second" || m.Disc != 2 || m.Discs != 2 || m.Date != "2001-05-01" {
		t.Errorf("%+v", m)
	}
	if len(m.Tracks) != 2 || m.Tracks[0].Artist != "" || m.Tracks[1].Artist != "Guest Singer" || m.Tracks[1].Title != "Guest" {
		t.Errorf("%+v", m.Tracks)
	}
	if got[1].Exact || !got[1].HasCover {
		t.Errorf("%+v", got[1])
	}
}

func TestUnknownDiscAndTrouble(t *testing.T) {
	c := serve(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	got, err := c.Lookup(context.Background(), "abc", "1", 2)
	if err != nil || len(got) != 0 || got == nil {
		t.Errorf("%v %v", got, err)
	}
	c = serve(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	if _, err := c.Lookup(context.Background(), "abc", "1", 2); !errors.Is(err, ErrBusy) {
		t.Error(err)
	}
	c = serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>")) })
	if _, err := c.Lookup(context.Background(), "abc", "1", 2); !errors.Is(err, ErrBusy) {
		t.Error(err)
	}
}

// Only a disc's numbers can go out, and only a real release's cover is asked for.
func TestInputsAreChecked(t *testing.T) {
	c := serve(t, func(w http.ResponseWriter, r *http.Request) { t.Errorf("asked %s", r.URL) })
	for _, bad := range [][2]string{{"../secret", "1"}, {"abc", "1&x=y"}, {"abc", "C:/music"}, {"", "1"}, {"abc", ""}} {
		if _, err := c.Lookup(context.Background(), bad[0], bad[1], 2); err == nil {
			t.Errorf("%q went out", bad)
		}
	}
	if _, err := c.Cover(context.Background(), "../../etc"); err == nil {
		t.Error("that isn't a release")
	}
}

func TestCover(t *testing.T) {
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/release/11111111-1111-1111-1111-111111111111/front-500" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte("jpeg"))
	})
	b, err := c.Cover(context.Background(), "11111111-1111-1111-1111-111111111111")
	if err != nil || string(b) != "jpeg" {
		t.Errorf("%q %v", b, err)
	}
	if _, err := c.Cover(context.Background(), "22222222-2222-2222-2222-222222222222"); err == nil {
		t.Error("no cover there")
	}
}
