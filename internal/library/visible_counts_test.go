// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// A library's count, as a profile with limits is told it, is of what that
// profile may see: the number must not give away that hidden titles exist.
func TestVisibleCountsFollowTheLimits(t *testing.T) {
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	s := &Store{DB: d}
	films, err := s.CreateLibrary(ctx, "Films", "movies", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateLibrary(ctx, "Other", "movies", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	add := func(name, rating string, lib int64) {
		t.Helper()
		if err := s.upsertItem(ctx, &Item{LibraryID: lib, Kind: "video", Path: "/m/" + name, Title: name, SortTitle: name, ContentRating: rating}); err != nil {
			t.Fatal(err)
		}
	}
	add("g", "G", films.ID)
	add("pg13", "PG-13", films.ID)
	add("r", "R", films.ID)
	add("unrated", "", films.ID)
	add("elsewhere", "G", other.ID)

	counts := func(a *Access) map[string]int {
		t.Helper()
		libs, err := s.Libraries(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.VisibleCounts(ctx, libs, a); err != nil {
			t.Fatal(err)
		}
		out := map[string]int{}
		for _, l := range libs {
			out[l.Name] = l.ItemCount
		}
		return out
	}
	open := func() *Access { return &Access{MaxAge: -1, LiveTV: true, LibraryChannels: true, TunerChannels: true} }
	for _, c := range []struct {
		name  string
		a     *Access
		films int
		other int
	}{
		{"no account limits", nil, 4, 1},
		{"an adult with no limits", open(), 4, 1},
		{"kids 7, unrated hidden", &Access{MaxAge: 7, LiveTV: true, LibraryChannels: true, TunerChannels: true}, 1, 1},
		{"kids 7, unrated shown", &Access{MaxAge: 7, AllowUnrated: true, LiveTV: true, LibraryChannels: true, TunerChannels: true}, 2, 1},
		{"teens 13", &Access{MaxAge: 13, LiveTV: true, LibraryChannels: true, TunerChannels: true}, 2, 1},
		{"one library only", func() *Access { a := open(); a.Libraries = []int64{films.ID}; return a }(), 4, 0},
	} {
		got := counts(c.a)
		if got["Films"] != c.films || got["Other"] != c.other {
			t.Errorf("%s: Films %d, Other %d; want %d and %d", c.name, got["Films"], got["Other"], c.films, c.other)
		}
	}
}
