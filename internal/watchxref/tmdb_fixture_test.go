// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package watchxref

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// The whole path against an answer shaped like TMDB's own (testdata, in the shape of
// GET /3/movie/603/watch/providers), served from this machine: nothing here reaches the network.
func TestTMDBWatchProvidersFromARecordedAnswer(t *testing.T) {
	body, err := os.ReadFile("testdata/tmdb-movie-603-watch-providers.json")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	var lastQuery atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		lastQuery.Store(r.URL.RawQuery + " " + r.URL.Path)
		if r.URL.Path != "/movie/603/watch/providers" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	client := &library.TMDB{Key: "test-key", Base: srv.URL, Client: srv.Client()}
	svc := &Service{Providers: []Provider{TMDB{Client: func() *library.TMDB { return client }}}, Cache: memCache{}, MaxAge: time.Hour}
	local := &Local{ItemID: 7, Title: "The Matrix", Versions: []Version{{ItemID: 7, Quality: "1080p"}}, Quality: "1080p"}

	a := svc.Lookup(context.Background(), local, Ref{Kind: "movie", ID: "603"}, "NL")
	if !a.InLibrary || a.ItemID == nil || *a.ItemID != 7 || a.Local.Quality != "1080p" {
		t.Errorf("what the library has: %+v", a)
	}
	if a.Reason != "" || a.Source != "tmdb" || a.Attribution == "" || a.Cached || a.Region != "NL" {
		t.Fatalf("answer: %+v", a)
	}
	if a.Link != "https://www.themoviedb.org/movie/603-the-matrix/watch?locale=NL" {
		t.Errorf("link: %q", a.Link)
	}
	var got []string
	for _, g := range a.Groups {
		for _, p := range g.Providers {
			got = append(got, g.Type+":"+p.Name)
		}
	}
	if want := "stream:Netflix stream:HBO Max rent:Apple TV rent:Amazon Video buy:Apple TV"; strings.Join(got, " ") != want {
		t.Errorf("offers in the Netherlands: %v", got)
	}
	for _, p := range a.Providers {
		if p.Name == "Amazon Video" && p.Logo != "" {
			t.Errorf("a logo path that climbs out was kept: %q", p.Logo)
		}
		if p.Name == "Netflix" && p.Logo != "https://image.tmdb.org/t/p/w92/pbpMk2JmcoNnQwx5JGpXngfoWtp.jpg" {
			t.Errorf("Netflix's logo: %q", p.Logo)
		}
	}
	// Only the title's id and the key were sent: nothing about the library or the viewer.
	if q, _ := lastQuery.Load().(string); q != "api_key=test-key /movie/603/watch/providers" {
		t.Errorf("what was sent to TMDB: %q", q)
	}

	// The same question again is answered from what was kept.
	again := svc.Lookup(context.Background(), local, Ref{Kind: "movie", ID: "603"}, "NL")
	if !again.Cached || calls.Load() != 1 || len(again.Providers) != len(a.Providers) {
		t.Errorf("second ask: cached %v after %d calls", again.Cached, calls.Load())
	}
	// Another region is its own question; a link that isn't TMDB's is dropped.
	us := svc.Lookup(context.Background(), nil, Ref{Kind: "movie", ID: "603"}, "US")
	if us.InLibrary || us.Link != "" || len(us.Groups) != 2 || us.Groups[0].Type != TypeStream || us.Groups[1].Type != TypeAds {
		t.Errorf("the United States: %+v", us)
	}
	// A region TMDB has nothing for: no services, and no error.
	if de := svc.Lookup(context.Background(), nil, Ref{Kind: "movie", ID: "603"}, "DE"); de.Reason != "" || len(de.Providers) != 0 || de.Source != "tmdb" {
		t.Errorf("Germany: %+v", de)
	}
	// TMDB failing: what's here is still answered, and the reason says why there's no more.
	if down := svc.Lookup(context.Background(), local, Ref{Kind: "movie", ID: "999"}, "NL"); !down.InLibrary || down.Reason != ReasonUnavailable || len(down.Providers) != 0 {
		t.Errorf("TMDB answering 404: %+v", down)
	}
	// No key: no request at all.
	before := calls.Load()
	noKey := &Service{Providers: []Provider{TMDB{Client: func() *library.TMDB { return nil }}}}
	if a := noKey.Lookup(context.Background(), local, Ref{Kind: "movie", ID: "603"}, "NL"); !a.InLibrary || a.Reason != ReasonNoAPIKey || calls.Load() != before {
		t.Errorf("without a key: %+v", a)
	}
}
