// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Wikidata's, Wikipedia's and Commons' answers for The Cabinet of Dr. Caligari (1920), recorded from the real
// services on 2026-11-05 and cut down to what's read. Its film poster (P3383) is a public-domain file; its
// image (P18) is a still.

func commonsFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "commons", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fakeWikis answers as Wikidata (/wd), Commons (/c) and Wikipedia's summaries do. license replaces every
// file's licence when set. calls counts the questions asked of Commons.
func fakeWikis(t *testing.T, license string) (*Wikidata, *int) {
	t.Helper()
	commonsCalls := 0
	info := commonsFixture(t, "imageinfo.json")
	if license != "" {
		var d map[string]any
		_ = json.Unmarshal(info, &d)
		for _, p := range d["query"].(map[string]any)["pages"].(map[string]any) {
			meta := p.(map[string]any)["imageinfo"].([]any)[0].(map[string]any)["extmetadata"].(map[string]any)
			meta["LicenseShortName"] = map[string]any{"value": license}
		}
		info, _ = json.Marshal(d)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/summary":
			w.Write(commonsFixture(t, "summary.json"))
		case strings.HasPrefix(r.URL.Path, "/c"):
			commonsCalls++
			w.Write(info)
		case q.Get("action") == "wbsearchentities" && !strings.Contains(q.Get("search"), "Dr."):
			// As Wikidata's name search does, a name without the stop finds nothing.
			w.Write([]byte(`{"search":[]}`))
		case q.Get("action") == "wbsearchentities":
			w.Write(commonsFixture(t, "search.json"))
		case q.Get("action") == "wbgetentities" && q.Get("props") == "labels":
			w.Write(commonsFixture(t, "labels.json"))
		case q.Get("action") == "wbgetentities":
			w.Write(commonsFixture(t, "entities.json"))
		case q.Get("action") == "query":
			// Wikidata's full-text search, recorded for "The Cabinet of Dr Caligari", films and series only.
			w.Write([]byte(`{"query":{"search":[{"title":"Q153886"},{"title":"Q7720893"},{"title":"Q7720891"}]}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return &Wikidata{Client: &http.Client{Transport: wikipediaTo{srv.URL}}, API: srv.URL + "/wd", CommonsAPI: srv.URL + "/c"}, &commonsCalls
}

// wikipediaTo sends Wikipedia's summaries, whose address is fixed, to the fake server.
type wikipediaTo struct{ base string }

func (rt wikipediaTo) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host == "en.wikipedia.org" {
		r2 := r.Clone(r.Context())
		u, _ := r.URL.Parse(rt.base + "/summary")
		r2.URL, r2.Host = u, ""
		return http.DefaultTransport.RoundTrip(r2)
	}
	return http.DefaultTransport.RoundTrip(r)
}

const caligariPoster = "https://upload.wikimedia.org/wikipedia/commons/7/74/CABINETOFDRCALIGARI-poster.jpg"

func TestPosterFileIsTheFilmPosterOrAnImageThatSaysSo(t *testing.T) {
	file := func(v string) []struct{ Mainsnak wdSnak } {
		var s wdSnak
		s.Datavalue.Value, _ = json.Marshal(v)
		return []struct{ Mainsnak wdSnak }{{s}}
	}
	for _, c := range []struct {
		name   string
		claims map[string][]struct{ Mainsnak wdSnak }
		want   string
	}{
		{"its film poster, before its image", map[string][]struct{ Mainsnak wdSnak }{wdFilmPoster: file("CABINETOFDRCALIGARI-poster.jpg"), wdImage: file("Das Cabinet des Dr. Caligari.JPG")}, "CABINETOFDRCALIGARI-poster.jpg"},
		{"an image named as a poster", map[string][]struct{ Mainsnak wdSnak }{wdImage: file("Plan 9 from Outer Space (1959) theatrical poster.jpg")}, "Plan 9 from Outer Space (1959) theatrical poster.jpg"},
		{"key art", map[string][]struct{ Mainsnak wdSnak }{wdImage: file("Some Film Key Art.png")}, "Some Film Key Art.png"},
		{"a still is not a poster", map[string][]struct{ Mainsnak wdSnak }{wdImage: file("Night Of The Living Dead trailer screenshot.jpg")}, ""},
		{"nothing", nil, ""},
	} {
		if got := posterFile(wdEntity{Claims: c.claims}); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestCommonsFileOfAnAddress(t *testing.T) {
	for raw, want := range map[string]string{
		"https://upload.wikimedia.org/wikipedia/commons/7/74/CABINETOFDRCALIGARI-poster.jpg":                                            "CABINETOFDRCALIGARI-poster.jpg",
		"https://upload.wikimedia.org/wikipedia/commons/thumb/8/89/Plan_nine_from_outer_space.jpg/500px-Plan_nine_from_outer_space.jpg": "Plan nine from outer space.jpg",
		"https://thumb.wikimedia.org/wikipedia/commons/thumb/8/89/Plan_nine_from_outer_space.jpg/500px-Plan_nine_from_outer_space.jpg":  "Plan nine from outer space.jpg",
		"https://upload.wikimedia.org/wikipedia/commons/a/a5/Caf%C3%A9_poster.jpg":                                                      "Café poster.jpg",
		"https://image.tmdb.org/t/p/w500/abc.jpg":                     "",
		"http://upload.wikimedia.org/wikipedia/commons/7/74/A.jpg":    "",
		"https://upload.wikimedia.org/wikipedia/en/7/74/Non-free.jpg": "",
		"cache:r-0123": "",
	} {
		if got := CommonsFileOf(raw); got != want {
			t.Errorf("%s: %q, want %q", raw, got, want)
		}
	}
}

func TestCommonsCreditIsTheAuthorInWords(t *testing.T) {
	for in, want := range map[string]string{
		`Unknown author<span style="display: none;">Unknown author</span>`:   "Unknown author",
		`<a href="//commons.wikimedia.org/wiki/User:X" title="User:X">X</a>`: "X",
		"Distributors Corporation of America":                                "Distributors Corporation of America",
		"Studio &amp; Sons\n  Ltd":                                           "Studio & Sons Ltd",
	} {
		if got := commonsCredit(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// A scan with no TMDB key: the film's facts from Wikidata, and its poster from Commons, at an address with
// nothing of Commons' statistics in it.
func TestEnrichGivesAFilmItsFreePosterFromCommons(t *testing.T) {
	w, _ := fakeWikis(t, "")
	year := 1920
	it := &Item{Title: "The Cabinet of Dr. Caligari", Year: &year}
	if err := w.Enrich(context.Background(), it); err != nil {
		t.Fatal(err)
	}
	if it.Poster != caligariPoster {
		t.Errorf("poster %q, want %q", it.Poster, caligariPoster)
	}
	if it.ProviderID != "Q153886" || it.Overview == "" || !strings.Contains(strings.Join(it.Genres, "|"), "Horror") {
		t.Errorf("facts: %q %q %v", it.ProviderID, it.Overview, it.Genres)
	}
}

func TestEnrichKeepsAPosterTheFilmHas(t *testing.T) {
	w, calls := fakeWikis(t, "")
	year := 1920
	it := &Item{Title: "The Cabinet of Dr. Caligari", Year: &year, Poster: "/films/Caligari/poster.jpg"}
	if err := w.Enrich(context.Background(), it); err != nil {
		t.Fatal(err)
	}
	if it.Poster != "/films/Caligari/poster.jpg" || *calls != 0 {
		t.Errorf("poster %q after %d questions to Commons: the film's own picture stays, and Commons isn't asked", it.Poster, *calls)
	}
}

// Commons holds free files only, but a file it marks non-free, or under a licence that isn't plainly free,
// is never shown.
func TestNoPosterUnderALicenceThatIsNotFree(t *testing.T) {
	for _, license := range []string{"Fair use", "CC BY-NC 4.0", "CC BY-ND 2.0", "All rights reserved"} {
		w, _ := fakeWikis(t, license)
		year := 1920
		it := &Item{Title: "The Cabinet of Dr. Caligari", Year: &year}
		if err := w.Enrich(context.Background(), it); err != nil {
			t.Fatal(err)
		}
		if it.Poster != "" {
			t.Errorf("%q: a poster %q", license, it.Poster)
		}
	}
}

// The match screen's candidates and a chosen match's details carry the poster too.
func TestWikidataCandidatesAndDetailsHaveThePoster(t *testing.T) {
	w, _ := fakeWikis(t, "")
	src := &wikidataSource{*w}
	ctx := context.Background()
	list, err := src.Search(ctx, "The Cabinet of Dr. Caligari", nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range list {
		got[c.ID] = c.Poster
	}
	if got["Q153886"] != caligariPoster {
		t.Errorf("the 1920 film's candidate: %q", got["Q153886"])
	}
	if p, ok := got["Q7720893"]; !ok || p != "" {
		t.Errorf("the 2005 film, which has no poster file: %q (listed %v)", p, ok)
	}
	d, err := src.Details(ctx, "Q153886")
	if err != nil {
		t.Fatal(err)
	}
	if d.Poster != caligariPoster || len(d.Posters) != 1 {
		t.Errorf("details: %q %v", d.Poster, d.Posters)
	}
}

// The credits: each address, where its file is on Commons, its licence and its author. Commons is asked once,
// then remembered.
func TestCommonsCreditsForAddresses(t *testing.T) {
	w, calls := fakeWikis(t, "")
	commonsCache.Lock()
	commonsCache.at, commonsCache.pics = map[string]time.Time{}, map[string]CommonsPicture{}
	commonsCache.Unlock()
	plan9 := "https://thumb.wikimedia.org/wikipedia/commons/thumb/8/89/Plan_nine_from_outer_space.jpg/500px-Plan_nine_from_outer_space.jpg"
	addresses := []string{caligariPoster, plan9, "https://image.tmdb.org/t/p/w500/abc.jpg", ""}
	got := CommonsCredits(context.Background(), w, addresses)
	if len(got) != 2 {
		t.Fatalf("%d credits: %+v", len(got), got)
	}
	c := got[caligariPoster]
	if c.Page != "https://commons.wikimedia.org/wiki/File:CABINETOFDRCALIGARI-poster.jpg" || c.License != "Public domain" || c.Credit != "Unknown author" {
		t.Errorf("Caligari: %+v", c)
	}
	if got[plan9].Credit != "Distributors Corporation of America" {
		t.Errorf("Plan 9: %+v", got[plan9])
	}
	before := *calls
	again := CommonsCredits(context.Background(), w, addresses)
	if *calls != before || len(again) != 2 {
		t.Errorf("asked Commons again (%d then %d), %d credits", before, *calls, len(again))
	}
}

// The scan's name for a film can lose a stop ("Dr Caligari"), which Wikidata's name search doesn't
// forgive: the full-text search finds it, and only a film is taken from it.
func TestEnrichFindsAFilmWhoseNameLostItsStop(t *testing.T) {
	w, _ := fakeWikis(t, "")
	year := 1920
	it := &Item{Title: "The Cabinet of Dr Caligari", Year: &year}
	if err := w.Enrich(context.Background(), it); err != nil {
		t.Fatal(err)
	}
	if it.ProviderID != "Q153886" || it.Poster != caligariPoster {
		t.Errorf("matched %q, poster %q", it.ProviderID, it.Poster)
	}
}

// A film can have several release dates; any of them counts against the year a file names.
func TestReleasedNearAnyOfAFilmsDates(t *testing.T) {
	date := func(d string) struct{ Mainsnak wdSnak } {
		var s wdSnak
		s.Datavalue.Value, _ = json.Marshal(map[string]string{"time": d})
		return struct{ Mainsnak wdSnak }{s}
	}
	// As Wikidata has Plan 9 from Outer Space: its premiere, its release, a later one.
	plan9 := wdEntity{Claims: map[string][]struct{ Mainsnak wdSnak }{wdPublication: {date("+1957-03-15T00:00:00Z"), date("+1959-07-22T00:00:00Z"), date("+1995-06-02T00:00:00Z")}}}
	for year, want := range map[int]bool{1959: true, 1960: true, 1957: true, 1995: true, 1970: false, 1954: false} {
		if got := releasedNear(plan9, year); got != want {
			t.Errorf("%d: %v", year, got)
		}
	}
	if releasedNear(wdEntity{}, 1959) {
		t.Error("a film with no date")
	}
}
