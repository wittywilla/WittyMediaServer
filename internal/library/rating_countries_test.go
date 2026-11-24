// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db/dbtest"
)

// The same letters mean different ages in different countries: a certificate is read by the
// country written in front of it, or else by the country the server keeps certificates for.
func TestCertificatesAreReadByTheirCountry(t *testing.T) {
	defer SetRatingCountry("US")
	cases := []struct {
		country, rating string
		want            int // -1 = unrated
	}{
		{"US", "M", 15}, // no US meaning: the general table
		{"AU", "M", 15},
		{"NZ", "M", 16},
		{"IN", "A", 18},
		{"ES", "A", 0},
		{"CA", "R", 18},
		{"US", "R", 17},
		{"US", "A", -1}, // means nothing in the US: unrated, so a limited profile doesn't see it
		{"FI", "K12", 12},
		{"MX", "B15", 15},
		{"US", "NZ:M", 16}, // the country in front wins
		{"NZ", "AU:M", 15},
		{"ES", "in/A", 18},
		{"JP", "CERO Z", 18}, // a games board's own rating, wherever
		{"NZ", "US:ESRB M", 17},
		{"PT", "M/12", 12},
	}
	for _, c := range cases {
		SetRatingCountry(c.country)
		got := AgeFor(c.rating)
		switch {
		case c.want < 0 && got != nil:
			t.Errorf("in %s, %q = %d, want unrated", c.country, c.rating, *got)
		case c.want >= 0 && (got == nil || *got != c.want):
			t.Errorf("in %s, %q = %v, want %d", c.country, c.rating, got, c.want)
		}
	}
	SetRatingCountry("XX") // not a country we know: the US
	if ratingCountry() != "US" {
		t.Errorf("an unknown country should mean the US, got %s", ratingCountry())
	}
}

func TestPickCertificateTakesTheChosenCountryThenTheUS(t *testing.T) {
	defer SetRatingCountry("US")
	certs := map[string]string{"US": "PG-13", "GB": "12A", "DE": ""}
	get := func(iso string) string { return certs[iso] }
	for country, want := range map[string]string{"US": "PG-13", "GB": "12A", "DE": "PG-13", "FR": "PG-13"} {
		SetRatingCountry(country)
		if got := PickCertificate(get); got != want {
			t.Errorf("in %s: %q, want %q", country, got, want)
		}
	}
	SetRatingCountry("GB")
	if got := PickCertificate(func(string) string { return "" }); got != "" {
		t.Errorf("no certificate anywhere: %q", got)
	}
}

// Changing the country changes what titles already saved mean.
func TestRecomputeAgesAfterTheCountryChanges(t *testing.T) {
	defer SetRatingCountry("US")
	d, err := dbtest.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	st := &Store{DB: d}
	lib, err := st.CreateLibrary(ctx, "Films", "movies", []string{"/x"})
	if err != nil {
		t.Fatal(err)
	}
	it := &Item{LibraryID: lib.ID, Kind: "video", Title: "Letters", SortTitle: "Letters", Path: "/x/letters.mkv", ContentRating: "M"}
	if err := st.upsertItem(ctx, it); err != nil {
		t.Fatal(err)
	}
	age := func() *int {
		var a *int
		if err := st.DB.QueryRowContext(ctx, `SELECT min_age FROM items WHERE id = ?`, it.ID).Scan(&a); err != nil {
			t.Fatal(err)
		}
		return a
	}
	if a := age(); a == nil || *a != 15 {
		t.Fatalf("saved in the US: %v, want 15", a)
	}
	SetRatingCountry("NZ")
	if err := st.RecomputeAges(ctx); err != nil {
		t.Fatal(err)
	}
	if a := age(); a == nil || *a != 16 {
		t.Fatalf("read in New Zealand: %v, want 16", a)
	}
}

// A song's parental advisory, however its file writes it, becomes a rating parental controls know.
func TestSongsAdvisoryIsRead(t *testing.T) {
	cases := []struct {
		tags map[string]string
		path string
		want string
	}{
		{map[string]string{"ITUNESADVISORY": "1"}, "/m/a.flac", "Explicit"},
		{map[string]string{"iTunesAdvisory": "2"}, "/m/a.mp3", "Clean"},
		{map[string]string{"rating": "1"}, "/m/a.m4a", "Explicit"},
		{map[string]string{"rating": "4"}, "/m/a.m4a", "Explicit"},
		{map[string]string{"rating": "2"}, "/m/a.m4a", "Clean"},
		{map[string]string{"RATING": "1"}, "/m/a.flac", ""}, // a star rating elsewhere
		{map[string]string{"rating": "0"}, "/m/a.m4a", ""},
		{map[string]string{"CONTENTRATING": "Explicit"}, "/m/a.ogg", "Explicit"},
		{map[string]string{}, "/m/a.mp3", ""},
	}
	for _, c := range cases {
		if got := advisoryTag([]map[string]string{c.tags}, c.path); got != c.want {
			t.Errorf("%v %s: %q, want %q", c.tags, c.path, got, c.want)
		}
	}
	if a := AgeFor("Explicit"); a == nil || *a != 17 {
		t.Errorf("Explicit is %v", a)
	}
}
