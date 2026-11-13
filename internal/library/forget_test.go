// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func putFile(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

var appleDouble = append([]byte{0x00, 0x05, 0x16, 0x07}, make([]byte, 60)...)

// What the server saved beside the media goes, and only that: a person's NFO comes back from its
// .nfo.bak, their own pictures stay, a "._" file goes only when it really is one, nothing is
// reached through a link out of the library, and a read-only library is cleaned all the same.
func TestForgetFilesRemovesOnlyTheServersOwn(t *testing.T) {
	ctx := context.Background()
	root, outside := t.TempDir(), t.TempDir()
	s := testScanner(t, root)
	lib, err := s.Store.Library(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}

	// A film in its own folder, with a hand-written NFO and a picture the owner chose.
	filmDir := filepath.Join(root, "Harbour Lights (2001)")
	media := filepath.Join(filmDir, "Harbour Lights (2001).mkv")
	putFile(t, media, []byte("film"))
	userNFO := filepath.Join(filmDir, "Harbour Lights (2001).nfo")
	putFile(t, userNFO, []byte("<movie><title>Mine</title></movie>"))
	picked := filepath.Join(root, "picked.jpg")
	putFile(t, picked, pngHead)
	it := &Item{Kind: "video", Path: media, Title: "Harbour Lights", Overview: "Boats.", Poster: picked, Backdrop: picked}
	if err := s.SaveItemMetadata(ctx, it); err != nil {
		t.Fatal(err)
	}
	poster, fanart := filepath.Join(filmDir, "poster.jpg"), filepath.Join(filmDir, "fanart.jpg")
	for _, p := range []string{poster, fanart, userNFO + ".bak"} {
		if !exists(p) {
			t.Fatalf("%s wasn't written", p)
		}
	}
	// Every file the server wrote is on record; the person's .bak isn't the server's.
	var n int
	_ = s.Store.DB.QueryRow(`SELECT COUNT(*) FROM saved_files`).Scan(&n)
	if n != 3 {
		t.Fatalf("%d files on record, want poster, fanart and the NFO", n)
	}
	var bak int
	_ = s.Store.DB.QueryRow(`SELECT COUNT(*) FROM saved_files WHERE path LIKE '%.bak'`).Scan(&bak)
	if bak != 0 {
		t.Fatal("the person's NFO kept as .bak is on record as the server's")
	}

	// macOS companions: one real, one only named like one.
	putFile(t, filepath.Join(filmDir, "._poster.jpg"), appleDouble)
	notAD := filepath.Join(filmDir, "._fanart.jpg")
	putFile(t, notAD, []byte("someone's own file"))
	putFile(t, filepath.Join(filmDir, "._Harbour Lights (2001).nfo.bak"), appleDouble)

	// An NFO the server wrote before it kept a record, and a person's picture beside it.
	oldDir := filepath.Join(root, "Old Film (1990)")
	oldNFO := filepath.Join(oldDir, "Old Film (1990).nfo")
	putFile(t, oldNFO, []byte("<?xml version=\"1.0\"?>\n"+nfoMarker+"\n<movie><title>Old</title></movie>"))
	theirs := filepath.Join(oldDir, "poster.jpg")
	putFile(t, theirs, pngHead)
	theirNFO := filepath.Join(oldDir, "movie.nfo")
	putFile(t, theirNFO, []byte("<movie><title>Their own</title></movie>"))

	// A picture the server wrote and someone has since replaced with their own.
	replaced := filepath.Join(root, "Replaced (2005)", "poster.jpg")
	putFile(t, replaced, pngHead)
	s.Store.savedBeside(ctx, replaced)
	putFile(t, replaced, append(pngHead, []byte("a bigger picture of their own")...))

	// A folder in the library that is a link out of it, holding what looks like the server's.
	outNFO := filepath.Join(outside, "elsewhere.nfo")
	putFile(t, outNFO, []byte(nfoMarker))
	outPic := filepath.Join(outside, "poster.jpg")
	putFile(t, outPic, pngHead)
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.DB.Exec(`INSERT INTO saved_files(path, written_at, size) VALUES(?, 9999999999, ?)`, filepath.Join(root, "linked", "poster.jpg"), len(pngHead)); err != nil {
		t.Fatal(err)
	}

	// Read-only stops the server adding to the folders, not taking its own leftovers away.
	if _, err := s.Store.DB.Exec(`UPDATE libraries SET read_only = 1 WHERE id = ?`, lib.ID); err != nil {
		t.Fatal(err)
	}
	lib.ReadOnly = true
	res := &ForgetResult{}
	if err := s.Store.ForgetFiles(ctx, lib, ForgetOptions{Files: true, ImagesDir: s.ImagesDir}, res); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{poster, fanart, oldNFO, filepath.Join(filmDir, "._poster.jpg"), userNFO + ".bak", filepath.Join(filmDir, "._Harbour Lights (2001).nfo.bak")} {
		if exists(p) {
			t.Errorf("%s is still there", p)
		}
	}
	for _, p := range []string{picked, notAD, theirs, theirNFO, replaced, outNFO, outPic, media, filepath.Join(filmDir, "._Harbour Lights (2001).nfo")} {
		if !exists(p) {
			t.Errorf("%s was removed", p)
		}
	}
	if b, _ := os.ReadFile(userNFO); string(b) != "<movie><title>Mine</title></movie>" {
		t.Errorf("the person's NFO wasn't put back: %q", b)
	}
	if res.Removed != 4 || res.Restored != 1 || res.Companions != 1 || res.Skipped < 1 {
		t.Errorf("counts: %+v", res)
	}
	_ = s.Store.DB.QueryRow(`SELECT COUNT(*) FROM saved_files WHERE path NOT LIKE ?`, filepath.Join(root, "linked")+"%").Scan(&n)
	if n != 0 {
		t.Errorf("%d files still on record", n)
	}
}

// Downloaded details go and titles go back to their file names, keeping every ID, what was
// watched, and what a person locked.
func TestForgetDetailsKeepsIDsHistoryAndLocks(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s := testScanner(t, root)
	st := s.Store
	lib, err := st.Library(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	year, rating := 1999, 8.1
	film := &Item{LibraryID: lib.ID, Kind: "video", Path: filepath.Join(root, "Harbour.Lights.2001.1080p.mkv"), Title: "Harbour Lights: The Matched Name",
		SortTitle: "Harbour Lights", Year: &year, Overview: "From a service.", Tagline: "Mine, typed", Genres: []string{"Drama"}, Rating: &rating,
		ContentRating: "PG", Studio: "Big Studio", Poster: "https://images.example/p.jpg", Backdrop: "https://images.example/b.jpg",
		Provider: "tmdb", ProviderID: "55", MTime: 1234, SizeBytes: 10}
	if err := st.upsertItem(ctx, film); err != nil {
		t.Fatal(err)
	}
	m := newTitleMeta()
	m.Locks["tagline"] = true
	m.Sources["tagline"], m.Sources["overview"] = SourceYou, "tmdb"
	m.MatchState, m.Director = MatchManual, "A Director"
	if err := st.saveTitleMeta(ctx, TargetItem, film.ID, m); err != nil {
		t.Fatal(err)
	}
	if err := st.setTitleBrands(ctx, TargetItem, film.ID, lib.ID, LogoStudio, []string{"Big Studio"}); err != nil {
		t.Fatal(err)
	}
	res, err := st.DB.Exec(`INSERT INTO users(name, password_hash, is_admin, created_at) VALUES('Sam', '', 0, 1)`)
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := res.LastInsertId()
	if _, err := st.DB.Exec(`INSERT INTO progress(user_id, item_id, position_sec, duration_sec, watched, updated_at) VALUES(?, ?, 600, 6000, 0, 1)`, uid, film.ID); err != nil {
		t.Fatal(err)
	}
	pres, err := st.DB.Exec(`INSERT INTO people(name, sort_name, key, photo) VALUES('An Actor', 'Actor, An', 'an actor', 'https://images.example/face.jpg')`)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := pres.LastInsertId()
	if _, err := st.DB.Exec(`INSERT INTO credits(person_id, target, target_id, source, role) VALUES(?, 'item', ?, 'tmdb', 'cast')`, pid, film.ID); err != nil {
		t.Fatal(err)
	}
	cached := []string{RemoteArtPath(s.ImagesDir, film.Poster), RemoteArtPath(s.ImagesDir, "https://images.example/face.jpg")}
	for _, p := range cached {
		putFile(t, p, pngHead)
	}

	// A show and an episode in a shows library.
	tvRoot := t.TempDir()
	tv, err := st.CreateLibrary(ctx, "TV", "shows", []string{tvRoot})
	if err != nil {
		t.Fatal(err)
	}
	sh := &Show{LibraryID: tv.ID, Path: filepath.Join(tvRoot, "Night Ferry (2010)"), Title: "The Night Ferry (US)", Overview: "Online.", Network: "A Network",
		Provider: "tvdb", ProviderID: "77", Poster: "https://images.example/show.jpg"}
	if err := st.UpsertShow(ctx, sh); err != nil {
		t.Fatal(err)
	}
	ep := &Item{LibraryID: tv.ID, Kind: "video", Path: filepath.Join(sh.Path, "Season 01", "Night Ferry - S01E02 - Crossing.mkv"), Title: "The Real Episode Name",
		Overview: "Online.", Provider: "tvdb", ProviderID: "9", ShowID: &sh.ID, MTime: 5}
	one, two := 1, 2
	ep.Season, ep.Episode = &one, &two
	if err := st.upsertItem(ctx, ep); err != nil {
		t.Fatal(err)
	}

	out := &ForgetResult{}
	for _, l := range []*Library{lib, tv} {
		if err := st.ForgetDetails(ctx, l, ForgetOptions{Details: true, ImagesDir: s.ImagesDir}, out); err != nil {
			t.Fatal(err)
		}
	}
	got, err := st.Item(ctx, film.ID)
	if err != nil {
		t.Fatalf("the film's ID changed: %v", err)
	}
	if got.Title != "Harbour Lights" || got.Year == nil || *got.Year != 2001 || got.Overview != "" || len(got.Genres) != 0 || got.Rating != nil ||
		got.Studio != "" || got.Poster != "" || got.Backdrop != "" || got.Provider != "" || got.ProviderID != "" || got.MTime != 0 {
		t.Errorf("film not reset: %+v", got)
	}
	if got.Tagline != "Mine, typed" {
		t.Errorf("the locked tagline went: %q", got.Tagline)
	}
	meta := st.TitleMeta(ctx, TargetItem, film.ID)
	if meta.MatchState != MatchAuto || !meta.Locks["tagline"] || meta.Sources["tagline"] != SourceYou || meta.Sources["overview"] != "" || meta.Director != "" {
		t.Errorf("title meta: %+v", meta)
	}
	if b := st.TitleBrands(ctx, TargetItem, film.ID, LogoStudio); len(b) != 0 {
		t.Errorf("studios kept: %v", b)
	}
	var pos float64
	if err := st.DB.QueryRow(`SELECT position_sec FROM progress WHERE user_id = ? AND item_id = ?`, uid, film.ID).Scan(&pos); err != nil || pos != 600 {
		t.Errorf("watch progress lost: %v %v", pos, err)
	}
	var people int
	_ = st.DB.QueryRow(`SELECT COUNT(*) FROM credits`).Scan(&people)
	if people != 0 {
		t.Error("credits kept")
	}
	_ = st.DB.QueryRow(`SELECT COUNT(*) FROM people`).Scan(&people)
	if people != 0 {
		t.Error("a person no title names any more was kept")
	}
	for _, p := range cached {
		if exists(p) {
			t.Errorf("cached picture %s kept", filepath.Base(p))
		}
	}
	show, err := st.Show(ctx, sh.ID, 0)
	if err != nil {
		t.Fatalf("the show's ID changed: %v", err)
	}
	if show.Title != "Night Ferry" || show.Year == nil || *show.Year != 2010 || show.Overview != "" || show.Network != "" || show.Provider != "" || show.Poster != "" {
		t.Errorf("show not reset: %+v", show)
	}
	gotEp, err := st.Item(ctx, ep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotEp.Title, "Crossing") || gotEp.Provider != "" || gotEp.Overview != "" || gotEp.ShowID == nil || *gotEp.ShowID != sh.ID ||
		gotEp.Season == nil || *gotEp.Season != 1 || gotEp.Episode == nil || *gotEp.Episode != 2 {
		t.Errorf("episode not reset: %+v", gotEp)
	}
	if out.Titles != 2 || out.Shows != 1 || out.LockedKept < 1 {
		t.Errorf("counts: %+v", out)
	}
}

// Libraries whose details aren't looked up by file name are named, not changed.
func TestForgetDetailsLeavesMusicAlone(t *testing.T) {
	ctx := context.Background()
	s := testScanner(t, t.TempDir())
	music, err := s.Store.CreateLibrary(ctx, "Music", "music", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	res := &ForgetResult{}
	if err := s.Store.ForgetDetails(ctx, music, ForgetOptions{Details: true}, res); err != nil {
		t.Fatal(err)
	}
	if len(res.NotReset) != 1 || res.NotReset[0] != "Music" || res.Titles != 0 {
		t.Errorf("%+v", res)
	}
}
