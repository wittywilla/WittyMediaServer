// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type healthFixture struct {
	s          *Scanner
	films, tv  int64
	filmDir    string
	tvDir      string
	ids        map[string]int64
	ctx        context.Context
	sectionsOf func(o HealthOptions) map[string]HealthSection
}

func newHealthFixture(t *testing.T) *healthFixture {
	t.Helper()
	filmDir, tvDir := t.TempDir(), t.TempDir()
	s := testScanner(t, filmDir)
	ctx := context.Background()
	tv, err := s.Store.CreateLibrary(ctx, "TV", "shows", []string{tvDir})
	if err != nil {
		t.Fatal(err)
	}
	f := &healthFixture{s: s, films: 1, tv: tv.ID, filmDir: filmDir, tvDir: tvDir, ids: map[string]int64{}, ctx: ctx}
	exec := func(q string, args ...any) int64 {
		t.Helper()
		r, err := s.Store.DB.ExecContext(ctx, q, args...)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		id, _ := r.LastInsertId()
		return id
	}
	film := func(name, title string, year any, provider, pid string, height int, container, poster string, missing int) {
		p := filepath.Join(filmDir, name)
		if missing == 0 {
			if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		f.ids[name] = exec(`INSERT INTO items(library_id, kind, path, title, sort_title, year, provider, provider_id, height, container, poster, missing, size_bytes, added_at, updated_at)
			VALUES (1, 'video', ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, NULLIF(?, ''), ?, 100, 1, 1)`, p, title, title, year, provider, pid, height, container, poster, missing)
	}
	// Two copies of one matched film, and a third film that's fine.
	film("Harbour.1080.mkv", "Harbour Lights", 2001, "tmdb", "42", 1080, "matroska", "cache:a.jpg", 0)
	film("Harbour.2160.mkv", "Harbour Lights", 2001, "tmdb", "42", 2160, "matroska", "cache:b.jpg", 0)
	film("Treeline.mkv", "The Treeline", 1999, "tmdb", "7", 1080, "matroska", "cache:c.jpg", 0)
	// A copy with no match beside one matched by its NFO: the same film.
	film("Treeline.copy.mkv", "The Treeline", 1999, "", "", 720, "matroska", "cache:g.jpg", 0)
	// The same name and year, matched to another film: not a copy.
	film("Treeline.other.mkv", "The Treeline", 1999, "tmdb", "77", 1080, "matroska", "cache:h.jpg", 0)
	// Gone, and removed on purpose (which isn't a problem: Fix matches has it).
	film("Gone.mkv", "Gone Film", 2003, "tmdb", "8", 1080, "matroska", "cache:d.jpg", 1)
	film("Removed.mkv", "Removed Film", 2004, "tmdb", "9", 1080, "matroska", "cache:e.jpg", 1)
	exec(`INSERT INTO library_moves(path, from_library, to_library, title, moved_at) VALUES (?, 1, 0, 'Removed Film', 1)`, filepath.Join(filmDir, "Removed.mkv"))
	// No match, a release name, no poster, and a file that couldn't be read.
	film("Some.Film.2010.1080p.WEB-DL.x264-GRP.mkv", "Some.Film.2010.1080p.WEB-DL.x264-GRP", nil, "", "", 1080, "matroska", "", 0)
	film("Broken.mkv", "Broken Film", 2005, "tmdb", "10", 0, "", "cache:f.jpg", 0)
	// A show: one episode twice, one with no number, and a file loose in the library.
	show := exec(`INSERT INTO shows(library_id, path, title, sort_title, added_at, updated_at, provider, provider_id, poster) VALUES (?, ?, 'The Show', 'show', 1, 1, 'tvmaze', '1', 'cache:s.jpg')`, tv.ID, filepath.Join(tvDir, "The Show"))
	ep := func(name string, season, episode any) {
		f.ids[name] = exec(`INSERT INTO items(library_id, kind, path, title, sort_title, show_id, season, episode, container, added_at, updated_at) VALUES (?, 'video', ?, ?, ?, ?, ?, ?, 'matroska', 1, 1)`,
			tv.ID, filepath.Join(tvDir, "The Show", name), name, name, show, season, episode)
	}
	ep("S01E01.mkv", 1, 1)
	ep("S01E01.HDTV.mkv", 1, 1)
	ep("bonus thing.mkv", 1, nil)
	f.ids["loose.mkv"] = exec(`INSERT INTO items(library_id, kind, path, title, sort_title, container, added_at, updated_at) VALUES (?, 'video', ?, 'loose', 'loose', 'matroska', 1, 1)`, tv.ID, filepath.Join(tvDir, "loose.mkv"))
	f.ids["show"] = show
	f.sectionsOf = func(o HealthOptions) map[string]HealthSection {
		t.Helper()
		rep, err := s.Health(ctx, o)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]HealthSection{}
		for _, sec := range rep.Sections {
			out[sec.ID] = sec
		}
		return out
	}
	return f
}

func rowIDs(sec HealthSection) map[int64]HealthRow {
	out := map[int64]HealthRow{}
	for _, r := range sec.Rows {
		out[r.ID] = r
	}
	return out
}

func TestHealthFindsEachKind(t *testing.T) {
	f := newHealthFixture(t)
	secs := f.sectionsOf(HealthOptions{})
	if len(secs) != len(HealthSections) {
		t.Fatalf("sections: %v", secs)
	}

	missing := rowIDs(secs[HealthMissing])
	if secs[HealthMissing].Count != 1 || missing[f.ids["Gone.mkv"]].Title != "Gone Film" {
		t.Errorf("missing: %+v", secs[HealthMissing])
	}

	// The same list as Fix matches, No match: the loose file in the shows library and the copy
	// with no match are there too.
	if u := rowIDs(secs[HealthUnmatched]); len(u) != 3 || u[f.ids["Some.Film.2010.1080p.WEB-DL.x264-GRP.mkv"]].Key == "" || u[f.ids["loose.mkv"]].Key == "" {
		t.Errorf("unmatched: %+v", secs[HealthUnmatched])
	}

	dups := secs[HealthDuplicates]
	if dups.Count != 3 {
		t.Fatalf("duplicates: %+v", dups)
	}
	var films, tree, eps *HealthRow
	for i, r := range dups.Rows {
		switch {
		case strings.HasPrefix(r.Key, "duplicates:films:") && r.Title == "Harbour Lights":
			films = &dups.Rows[i]
		case strings.HasPrefix(r.Key, "duplicates:films:"):
			tree = &dups.Rows[i]
		case strings.HasPrefix(r.Key, "duplicates:episode:"):
			eps = &dups.Rows[i]
		}
	}
	if films == nil || len(films.Members) != 2 || films.Members[0].Height != 2160 || films.ID != f.ids["Harbour.2160.mkv"] {
		t.Errorf("the better copy comes first: %+v", films)
	}
	// The copy with no match joins the matched one; the film matched to another title stays apart.
	if tree == nil || len(tree.Members) != 2 || tree.ID != f.ids["Treeline.mkv"] || tree.Members[1].ID != f.ids["Treeline.copy.mkv"] {
		t.Errorf("the copy with no match: %+v", tree)
	}
	if eps == nil || len(eps.Members) != 2 {
		t.Errorf("episodes: %+v", eps)
	}

	names := rowIDs(secs[HealthNames])
	for _, n := range []string{"bonus thing.mkv", "loose.mkv", "Some.Film.2010.1080p.WEB-DL.x264-GRP.mkv"} {
		if _, ok := names[f.ids[n]]; !ok {
			t.Errorf("odd name %s isn't listed: %+v", n, secs[HealthNames])
		}
	}
	if secs[HealthNames].Count != 3 {
		t.Errorf("names: %+v", secs[HealthNames])
	}

	if art := rowIDs(secs[HealthArtwork]); secs[HealthArtwork].Count != 1 || art[f.ids["Some.Film.2010.1080p.WEB-DL.x264-GRP.mkv"]].Fixes[0] != FixMatch {
		t.Errorf("artwork: %+v", secs[HealthArtwork])
	}

	if bad := rowIDs(secs[HealthUnreadable]); secs[HealthUnreadable].Count != 1 || bad[f.ids["Broken.mkv"]].Fixes[0] != FixProbe {
		t.Errorf("unreadable: %+v", secs[HealthUnreadable])
	}
	if secs[HealthNFO].Count != 0 || secs[HealthNFO].Checked != 0 {
		t.Errorf("nfo before a check: %+v", secs[HealthNFO])
	}
}

func TestHealthFiltersAndSetsAside(t *testing.T) {
	f := newHealthFixture(t)
	secs := f.sectionsOf(HealthOptions{Library: f.tv})
	if secs[HealthMissing].Count != 0 || secs[HealthDuplicates].Count != 1 || secs[HealthNames].Count != 2 {
		t.Fatalf("one library: %+v", secs)
	}
	key := secs[HealthDuplicates].Rows[0].Key
	secs = f.sectionsOf(HealthOptions{SetAside: map[string]bool{key: true}, Limit: 1})
	if secs[HealthDuplicates].Count != 2 || secs[HealthDuplicates].Rows[0].Key == key {
		t.Fatalf("set aside: %+v", secs[HealthDuplicates])
	}
	if len(secs[HealthNames].Rows) != 1 || secs[HealthNames].Count != 3 {
		t.Fatalf("the limit lists fewer and counts all: %+v", secs[HealthNames])
	}
	if _, err := f.s.Health(f.ctx, HealthOptions{Library: 999}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no such library: %v", err)
	}
	rep, err := f.s.Health(f.ctx, HealthOptions{Only: HealthArtwork})
	if err != nil || len(rep.Sections) != 1 {
		t.Fatalf("one section: %+v %v", rep, err)
	}
}

func TestHealthMakeVersions(t *testing.T) {
	f := newHealthFixture(t)
	main, other := f.ids["Harbour.2160.mkv"], f.ids["Harbour.1080.mkv"]
	if err := f.s.Store.MakeVersions(f.ctx, []int64{main}); err == nil {
		t.Fatal("one copy isn't a group")
	}
	if err := f.s.Store.MakeVersions(f.ctx, []int64{main, f.ids["S01E01.mkv"]}); err == nil {
		t.Fatal("a film and an episode in another library")
	}
	if err := f.s.Store.MakeVersions(f.ctx, []int64{main, other}); err != nil {
		t.Fatal(err)
	}
	if l := f.s.Store.LinkOf(f.ctx, other); l == nil || l.Link != LinkVersion {
		t.Fatalf("link: %+v", l)
	}
	if secs := f.sectionsOf(HealthOptions{}); secs[HealthDuplicates].Count != 2 {
		t.Fatalf("still a duplicate: %+v", secs[HealthDuplicates])
	}
}

func TestHealthForgetOnlyWhatIsGone(t *testing.T) {
	f := newHealthFixture(t)
	// Not missing: refused.
	if err := f.s.Store.ForgetGone(f.ctx, TargetItem, f.ids["Treeline.mkv"]); !errors.Is(err, ErrStillThere) {
		t.Fatalf("forgot a title that's there: %v", err)
	}
	// Marked missing, but the file is back: refused.
	back := f.ids["Harbour.1080.mkv"]
	if _, err := f.s.Store.DB.ExecContext(f.ctx, `UPDATE items SET missing = 1 WHERE id = ?`, back); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Store.ForgetGone(f.ctx, TargetItem, back); !errors.Is(err, ErrStillThere) {
		t.Fatalf("forgot a title whose file is back: %v", err)
	}
	// The drive is away (its folder empty): refused, the history stays.
	gone := f.ids["Gone.mkv"]
	empty := t.TempDir()
	if _, err := f.s.Store.DB.ExecContext(f.ctx, `UPDATE library_paths SET path = ? WHERE library_id = 1`, empty); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Store.DB.ExecContext(f.ctx, `UPDATE items SET path = ? WHERE id = ?`, filepath.Join(empty, "Gone.mkv"), gone); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Store.ForgetGone(f.ctx, TargetItem, gone); !errors.Is(err, ErrStillThere) {
		t.Fatalf("forgot a title on a drive that's away: %v", err)
	}
	// The drive is there: forgotten.
	if err := os.WriteFile(filepath.Join(empty, "other.mkv"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Store.ForgetGone(f.ctx, TargetItem, gone); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Store.Item(f.ctx, gone); !errors.Is(err, ErrNotFound) {
		t.Fatalf("still there: %v", err)
	}
	if err := f.s.Store.ForgetGone(f.ctx, TargetItem, gone); !errors.Is(err, ErrNotFound) {
		t.Fatalf("twice: %v", err)
	}
}

func TestHealthSidecars(t *testing.T) {
	f := newHealthFixture(t)
	bad := filepath.Join(f.filmDir, "Treeline.nfo")
	if err := os.WriteFile(bad, []byte("<movie><title>Fish & Chips</title></movie>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.filmDir, "Harbour.1080.nfo"), []byte("<movie><title>ok</title></movie>"), 0o644); err != nil {
		t.Fatal(err)
	}
	problem := func(p string, b []byte) string {
		if strings.Contains(string(b), "&") {
			return "line 1: a stray &"
		}
		return ""
	}
	res, err := f.s.Store.CheckSidecars(f.ctx, SidecarLimits{Files: 1000, Deadline: time.Now().Add(time.Minute)}, problem, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 || res.Rows[0].Path != bad || res.Rows[0].ID != f.ids["Treeline.mkv"] || res.Stopped {
		t.Fatalf("%+v", res)
	}
	secs := f.sectionsOf(HealthOptions{Sidecars: res})
	if secs[HealthNFO].Count != 1 || secs[HealthNFO].Checked == 0 {
		t.Fatalf("nfo: %+v", secs[HealthNFO])
	}
	if secs := f.sectionsOf(HealthOptions{Sidecars: res, Library: f.tv}); secs[HealthNFO].Count != 0 {
		t.Fatalf("the other library's: %+v", secs[HealthNFO])
	}
	res, err = f.s.Store.CheckSidecars(f.ctx, SidecarLimits{Files: 0}, problem, nil)
	if err != nil || !res.Stopped {
		t.Fatalf("a limit stops it: %+v %v", res, err)
	}
}

func TestReadAgainRefusesWhatIsntAFile(t *testing.T) {
	f := newHealthFixture(t)
	if _, err := f.s.ReadAgain(f.ctx, f.ids["Gone.mkv"]); err == nil {
		t.Fatal("read a file that's gone")
	}
	if _, err := f.s.ReadAgain(f.ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no such item: %v", err)
	}
}
