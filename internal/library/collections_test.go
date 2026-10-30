package library

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSmartPlaylists(t *testing.T) {
	s, lib := galleryStore(t, "movies")
	ctx := context.Background()
	music, err := s.CreateLibrary(ctx, "Music", "music", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	year := func(y int) *int { return &y }
	rating := func(r float64) *float64 { return &r }
	add := func(libID int64, name string, y int, r float64, genre, contentRating string) *Item {
		it := &Item{LibraryID: libID, Kind: "video", Path: "/m/" + name, Title: name, SortTitle: name, Year: year(y), Rating: rating(r), Genres: []string{genre}, RuntimeSec: 100, ContentRating: contentRating}
		if err := s.upsertItem(ctx, it); err != nil {
			t.Fatal(err)
		}
		return it
	}
	a := add(lib.ID, "Alpha", 1995, 8, "Comedy", "PG")
	b := add(lib.ID, "Beta", 1999, 6, "Drama", "R")
	c := add(lib.ID, "Gamma", 2010, 9, "Comedy", "PG")
	song := add(music.ID, "Song", 1995, 0, "Rock", "")
	find := func(r CollectionRule, access *Access) []int64 {
		t.Helper()
		col := &Collection{Kind: "smart", Rule: &r}
		items, _, err := s.CollectionItems(ctx, col, 1, access, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		out := []int64{}
		for _, it := range items {
			out = append(out, it.ID)
		}
		return out
	}
	eq := func(got []int64, want ...int64) {
		t.Helper()
		if len(got) != len(want) {
			t.Errorf("got %v, want %v", got, want)
			return
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("got %v, want %v", got, want)
				return
			}
		}
	}
	eq(find(CollectionRule{Sort: "title"}, nil), a.ID, b.ID, c.ID, song.ID)
	eq(find(CollectionRule{Sort: "title", LibraryTypes: []string{"movies"}}, nil), a.ID, b.ID, c.ID)
	eq(find(CollectionRule{Sort: "title", LibraryTypes: []string{"music"}}, nil), song.ID)
	eq(find(CollectionRule{Sort: "title", YearFrom: 1990, YearTo: 1999, LibraryTypes: []string{"movies"}}, nil), a.ID, b.ID)
	eq(find(CollectionRule{Sort: "title", Genres: []string{"Comedy"}}, nil), a.ID, c.ID)
	eq(find(CollectionRule{Sort: "rating", Desc: true, RatingMin: 7}, nil), c.ID, a.ID)
	eq(find(CollectionRule{Sort: "title", Search: "ph"}, nil), a.ID)
	eq(find(CollectionRule{Sort: "title", Limit: 2}, nil), a.ID, b.ID)
	// Watched is per account.
	_ = s.SetProgress(ctx, 1, a.ID, Progress{PositionSec: 100, DurationSec: 100, Watched: true})
	eq(find(CollectionRule{Sort: "title", Watched: "yes"}, nil), a.ID)
	eq(find(CollectionRule{Sort: "title", Watched: "no", LibraryTypes: []string{"movies"}}, nil), b.ID, c.ID)
	// A rule can't get past an account's limits.
	kids := DefaultAccess("kids")
	kids.MaxAge = 10
	eq(find(CollectionRule{Sort: "title", LibraryTypes: []string{"movies"}}, &kids), a.ID, c.ID)
	// Bad rules are refused; a value meant to break the statement is just a value.
	bad := CollectionRule{Sort: "title; DROP TABLE items"}
	if err := bad.Clean(); err == nil {
		t.Error("an unknown sort was accepted")
	}
	inj := CollectionRule{Search: "x' OR 1=1 --"}
	if got := find(inj, nil); len(got) != 0 {
		t.Errorf("a search became SQL: %v", got)
	}
	over := CollectionRule{Limit: 99999}
	if err := over.Clean(); err != nil || over.Limit != maxSmartItems {
		t.Errorf("limit: %d %v", over.Limit, err)
	}
	// Saved and listed for its owner; others only once shared.
	col := &Collection{OwnerID: 1, Name: "Comedies", Kind: "smart", Rule: &CollectionRule{Genres: []string{"Comedy"}}}
	if err := s.SaveCollection(ctx, col); err != nil {
		t.Fatal(err)
	}
	if l, _ := s.Collections(ctx, 1, nil); len(l) != 1 || l[0].Count != 2 || l[0].Rule == nil {
		t.Errorf("own list: %+v", l)
	}
	if l, _ := s.Collections(ctx, 2, nil); len(l) != 0 {
		t.Errorf("another account sees a private collection: %+v", l)
	}
	if _, err := s.CollectionByID(ctx, col.ID, 2, false); err == nil {
		t.Error("another account opened a private collection")
	}
	col.Shared = true
	if err := s.SaveCollection(ctx, col); err != nil {
		t.Fatal(err)
	}
	if l, _ := s.Collections(ctx, 2, &kids); len(l) != 1 {
		t.Errorf("a shared collection isn't listed: %+v", l)
	}
	if err := s.SaveCollection(ctx, &Collection{OwnerID: 1, Name: "x", Kind: "smart"}); err == nil {
		t.Error("a smart playlist without a rule was accepted")
	}
}

func TestManualCollections(t *testing.T) {
	s, lib := galleryStore(t, "movies")
	ctx := context.Background()
	add := func(name, rating string) *Item {
		it := &Item{LibraryID: lib.ID, Kind: "video", Path: "/m/" + name, Title: name, SortTitle: name, ContentRating: rating}
		if err := s.upsertItem(ctx, it); err != nil {
			t.Fatal(err)
		}
		return it
	}
	a, b := add("A", "G"), add("B", "R")
	res, err := s.DB.ExecContext(ctx, `INSERT INTO shows(library_id, path, title, sort_title, added_at, updated_at) VALUES(?, '/s', 'Show', 'Show', 0, 0)`, lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	show, _ := res.LastInsertId()
	// A show is listed once it has an episode.
	ep := &Item{LibraryID: lib.ID, Kind: "video", Path: "/s/e1", Title: "E1", SortTitle: "E1", ShowID: &show}
	if err := s.upsertItem(ctx, ep); err != nil {
		t.Fatal(err)
	}
	col := &Collection{OwnerID: 1, Name: "Mine", Kind: "manual"}
	if err := s.SaveCollection(ctx, col); err != nil {
		t.Fatal(err)
	}
	if err := s.AddToCollection(ctx, col.ID, []int64{a.ID, b.ID, a.ID}, []int64{show}); err != nil {
		t.Fatal(err)
	}
	items, shows, err := s.CollectionItems(ctx, col, 1, nil, 0, 0)
	if err != nil || len(items) != 2 || len(shows) != 1 {
		t.Fatalf("items %d shows %d %v", len(items), len(shows), err)
	}
	if next := s.NextInCollection(ctx, col, 1, a.ID, nil); next == nil || *next != b.ID {
		t.Errorf("next after A: %v", next)
	}
	if next := s.NextInCollection(ctx, col, 1, b.ID, nil); next != nil {
		t.Errorf("nothing follows the last: %v", *next)
	}
	kids := DefaultAccess("kids")
	if items, _, _ := s.CollectionItems(ctx, col, 1, &kids, 0, 0); len(items) != 1 || items[0].ID != a.ID {
		t.Errorf("kids see %+v", items)
	}
	holding, err := s.CollectionsHolding(ctx, 1, a.ID, 0)
	if err != nil || len(holding) != 1 {
		t.Errorf("holding %v %v", holding, err)
	}
	if err := s.RemoveFromCollection(ctx, col.ID, []int64{a.ID}, []int64{show}); err != nil {
		t.Fatal(err)
	}
	if items, shows, _ := s.CollectionItems(ctx, col, 1, nil, 0, 0); len(items) != 1 || len(shows) != 0 {
		t.Errorf("after removing: %d %d", len(items), len(shows))
	}
	w1, err := s.WatchLater(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	w2, err := s.WatchLater(ctx, 1)
	if err != nil || w1.ID != w2.ID || w2.System != "watchlater" {
		t.Errorf("Watch later: %+v %+v %v", w1, w2, err)
	}
}

func TestPlaylistParsing(t *testing.T) {
	m3u := "\xef\xbb\xbf#EXTM3U\n#EXTINF:-1 group-title=\"Jazz\",Smooth FM\nhttps://radio.example/smooth\n\n# a comment\nhttp://radio.example/plain.mp3\nnot a url\nfile:///etc/passwd\n"
	got := ParsePlaylist(m3u)
	if len(got) != 2 || got[0].Name != "Smooth FM" || got[0].Genre != "Jazz" || got[0].URL != "https://radio.example/smooth" || got[1].Name != "" {
		t.Errorf("m3u: %+v", got)
	}
	pls := "[playlist]\nNumberOfEntries=2\nFile1=http://radio.example/one\nTitle1=One\nLength1=-1\nFile2=ftp://nope.example/two\nTitle2=Two\nVersion=2\n"
	got = ParsePlaylist(pls)
	if len(got) != 1 || got[0].Name != "One" || got[0].URL != "http://radio.example/one" {
		t.Errorf("pls: %+v", got)
	}
	for _, bad := range []string{"javascript:alert(1)", "file:///etc/passwd", "ftp://x/y", "http://", "not a url"} {
		if ValidStreamURL(bad) {
			t.Errorf("%q was accepted", bad)
		}
	}
	if !ValidStreamURL("https://radio.example:8000/live") {
		t.Error("a real address was refused")
	}
}

func TestRadioStations(t *testing.T) {
	s, lib := galleryStore(t, "radio")
	ctx := context.Background()
	if _, err := s.AddRadioStation(ctx, lib.ID, RadioStation{URL: "file:///etc/passwd"}); err == nil {
		t.Error("a local file was accepted as a station")
	}
	st, err := s.AddRadioStation(ctx, lib.ID, RadioStation{Name: "Jazz", URL: "https://radio.example/jazz", Genre: "Jazz"})
	if err != nil {
		t.Fatal(err)
	}
	if again, err := s.AddRadioStation(ctx, lib.ID, RadioStation{Name: "Jazz FM", URL: "https://radio.example/jazz"}); err != nil || again.ID != st.ID {
		t.Errorf("the same address made a second station: %+v %v", again, err)
	}
	noName, err := s.AddRadioStation(ctx, lib.ID, RadioStation{URL: "https://other.example/x"})
	if err != nil || noName.Title != "other.example" {
		t.Errorf("no name: %+v %v", noName, err)
	}
	// A scan of the library must not mark stations as missing.
	if err := s.markMissing(ctx, lib.ID, map[string]bool{}); err != nil {
		t.Fatal(err)
	}
	items, _, err := s.Items(ctx, ItemQuery{LibraryID: lib.ID, Kind: "radio"})
	if err != nil || len(items) != 2 {
		t.Errorf("after a scan: %d %v", len(items), err)
	}
	if err := s.RemoveRadioStation(ctx, st.ID); err != nil {
		t.Fatal(err)
	}
	if items, _, _ := s.Items(ctx, ItemQuery{LibraryID: lib.ID, Kind: "radio"}); len(items) != 1 {
		t.Errorf("after removing: %d", len(items))
	}
}

func TestYearInReview(t *testing.T) {
	s, lib := galleryStore(t, "movies")
	ctx := context.Background()
	music, _ := s.CreateLibrary(ctx, "Music", "music", []string{t.TempDir()})
	film := &Item{LibraryID: lib.ID, Kind: "video", Path: "/m/f", Title: "Film", SortTitle: "Film", Genres: []string{"Drama"}}
	song := &Item{LibraryID: music.ID, Kind: "audio", Path: "/m/s", Title: "Song", SortTitle: "Song", Artist: "Band", Genres: []string{"Rock"}}
	for _, it := range []*Item{film, song} {
		if err := s.upsertItem(ctx, it); err != nil {
			t.Fatal(err)
		}
	}
	log := func(user, item int64, day int, secs float64) {
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO play_log(user_id, item_id, day, seconds) VALUES(?,?,?,?)`, user, item, day, secs); err != nil {
			t.Fatal(err)
		}
	}
	log(1, film.ID, 20240131, 3600)
	log(1, film.ID, 20240201, 1800)
	log(1, song.ID, 20240202, 600)
	log(1, song.ID, 20240210, 600)
	log(1, film.ID, 20231231, 7200) // last year
	log(2, film.ID, 20240101, 9999) // someone else
	r, err := s.ReviewOfYear(ctx, 1, 2024)
	if err != nil {
		t.Fatal(err)
	}
	if r.Seconds != 6600 || r.Items != 2 || r.Days != 4 {
		t.Errorf("totals: %+v", r)
	}
	if r.LongestRun != 3 {
		t.Errorf("a run over a month's end: %d", r.LongestRun)
	}
	if r.BusiestDay != 20240131 || r.ByMonth[0] != 3600 || r.ByMonth[1] != 3000 {
		t.Errorf("busiest %d, months %v", r.BusiestDay, r.ByMonth)
	}
	if len(r.TopArtists) != 1 || r.TopArtists[0].Name != "Band" {
		t.Errorf("artists: %+v", r.TopArtists)
	}
	if len(r.ByType) != 2 || r.ByType[0].Name != "movies" {
		t.Errorf("types: %+v", r.ByType)
	}
	if len(r.TopGenres) != 2 || r.TopGenres[0].Name != "Drama" {
		t.Errorf("genres: %+v", r.TopGenres)
	}
	if len(r.Years) != 2 || r.Years[0] != 2024 {
		t.Errorf("years: %v", r.Years)
	}
	empty, err := s.ReviewOfYear(ctx, 1, 2001)
	if err != nil || empty.Seconds != 0 || empty.ByType == nil {
		t.Errorf("an empty year: %+v %v", empty, err)
	}
	if err := s.ClearWatchHistory(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.ReviewOfYear(ctx, 1, 2024); r.Seconds != 0 {
		t.Errorf("clearing left %f seconds", r.Seconds)
	}
	if r, _ := s.ReviewOfYear(ctx, 2, 2024); r.Seconds == 0 {
		t.Error("clearing one account's history cleared another's")
	}
	for in, want := range map[int]int{20240131: 20240201, 20240228: 20240229, 20230228: 20230301, 20231231: 20240101, 20240115: 20240116} {
		if got := nextDay(in); got != want {
			t.Errorf("after %d: %d", in, got)
		}
	}
}

func TestRecommendations(t *testing.T) {
	s, lib := galleryStore(t, "movies")
	ctx := context.Background()
	add := func(name, genre string, r float64) *Item {
		it := &Item{LibraryID: lib.ID, Kind: "video", Path: "/m/" + name, Title: name, SortTitle: name, Genres: []string{genre}, Rating: &r}
		if err := s.upsertItem(ctx, it); err != nil {
			t.Fatal(err)
		}
		return it
	}
	seen := add("Seen", "Sci-Fi", 8)
	want := add("Unseen sci-fi", "Sci-Fi", 8)
	add("Unseen romance", "Romance", 8)
	// Nothing watched yet: no guesses.
	if items, _, _, err := s.RecommendForYou(ctx, 1, nil, 1, 10); err != nil || len(items) != 0 {
		t.Errorf("for someone new: %+v %v", items, err)
	}
	_ = s.SetProgress(ctx, 1, seen.ID, Progress{PositionSec: 100, DurationSec: 100, Watched: true})
	items, _, because, err := s.RecommendForYou(ctx, 1, nil, 1, 10)
	if err != nil || len(items) == 0 || items[0].ID != want.ID || !strings.Contains(because, "Sci-Fi") {
		t.Errorf("recommended %+v because %q, %v", items, because, err)
	}
	for _, it := range items {
		if it.ID == seen.ID {
			t.Error("recommended something already watched")
		}
	}
}

func TestMusicVideoNames(t *testing.T) {
	dir := t.TempDir()
	it := &Item{}
	cr := creatorRow{Name: "Folder", Key: "folder"}
	vm := videoMeta{}
	musicVideoNames(it, &cr, &vm, "The Band - Great Song", nil, filepath.Join(dir, "x.mp4"))
	if cr.Name != "The Band" || cr.Key != "" || it.Title != "Great Song" {
		t.Errorf("from the name: %+v %+v", cr, it)
	}
	nfo := `<?xml version="1.0"?><musicvideo><title>Live Song</title><artist>Other Band</artist><album>Live Album</album><year>1999</year><genre>Rock</genre><tag>live</tag><plot>Recorded on tour.</plot></musicvideo>`
	if err := os.WriteFile(filepath.Join(dir, "y.nfo"), []byte(nfo), 0o644); err != nil {
		t.Fatal(err)
	}
	it, cr, vm = &Item{}, creatorRow{}, videoMeta{}
	musicVideoNames(it, &cr, &vm, "y", nil, filepath.Join(dir, "y.mp4"))
	if it.Title != "Live Song" || cr.Name != "Other Band" || vm.Playlist != "Live Album" || it.Overview == "" || len(it.Genres) != 1 || *it.Year != 1999 || vm.UploadDate != "1999-01-01" || len(vm.Tags) != 1 {
		t.Errorf("from the nfo: %+v %+v %+v", it, cr, vm)
	}
	// Longer recordings are concerts.
	s, lib := galleryStore(t, "musicvideos")
	ctx := context.Background()
	short := &Item{LibraryID: lib.ID, Kind: "video", Path: "/v/s", Title: "s", SortTitle: "s", RuntimeSec: 200}
	long := &Item{LibraryID: lib.ID, Kind: "video", Path: "/v/l", Title: "l", SortTitle: "l", RuntimeSec: 3600}
	for _, x := range []*Item{short, long} {
		if err := s.upsertItem(ctx, x); err != nil {
			t.Fatal(err)
		}
		if err := s.saveVideoMeta(ctx, x, &videoMeta{UploadDate: time.Now().Format("2006-01-02")}); err != nil {
			t.Fatal(err)
		}
	}
	yes := true
	v, _, err := s.Videos(ctx, VideoQuery{LibraryID: lib.ID, Long: &yes})
	if err != nil || len(v) != 1 || v[0].ID != long.ID {
		t.Errorf("concerts: %+v %v", v, err)
	}
}
