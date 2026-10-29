//go:build live

// Checks against the real online services. Not run by default (they need
// the internet and are rate-limited): go test -tags live ./internal/library
package library

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestLiveAniList(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	a := &AniList{}
	m, err := a.FindSeries(ctx, "Frieren", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.Title, "Frieren") || m.Poster == "" || m.Overview == "" || m.Genres[0] != "Anime" {
		t.Errorf("unexpected: %+v", m)
	}
	eps, err := a.Episodes(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s (%s): %d episodes with names, network %q", m.Title, m.ID, len(eps), m.Network)
	a.TitleLang = "romaji"
	r, _ := a.Series(ctx, m.ID)
	t.Logf("romaji title: %s", r.Title)
}

func TestLiveTVMaze(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	m, err := (&TVMaze{}).FindSeries(ctx, "Codename Kids Next Door", nil)
	if err != nil || m.Network != "Cartoon Network" {
		t.Fatalf("%v %+v", err, m)
	}
}
