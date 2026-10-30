package library

import (
	"context"
	"fmt"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// A large home library, to keep the queries behind the main pages honest:
//
//	go test ./internal/library -run '^$' -bench Store -benchtime 20x
const (
	benchMovies   = 5000
	benchShows    = 400
	benchEpisodes = 50 // per show
)

func benchStore(b *testing.B) (*Store, int64, int64, int64) {
	b.Helper()
	d, err := db.Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { d.Close() })
	ctx := context.Background()
	st := &Store{DB: d}
	movies, err := st.CreateLibrary(ctx, "Movies", "movies", []string{"/m"})
	if err != nil {
		b.Fatal(err)
	}
	tv, err := st.CreateLibrary(ctx, "TV", "shows", []string{"/t"})
	if err != nil {
		b.Fatal(err)
	}
	res, err := d.ExecContext(ctx, `INSERT INTO users(name, password_hash, is_admin, created_at) VALUES('u', 'x', 0, 1)`)
	if err != nil {
		b.Fatal(err)
	}
	user, _ := res.LastInsertId()
	tx, err := d.Begin()
	if err != nil {
		b.Fatal(err)
	}
	genres := []string{"Comedy|Drama", "Action", "Horror|Thriller", "Family|Animation"}
	id := int64(0)
	for i := 0; i < benchMovies; i++ {
		id++
		if _, err := tx.Exec(`INSERT INTO items(id, library_id, kind, path, title, sort_title, year, overview, genres, added_at, updated_at, runtime_sec)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, id, movies.ID, "video", fmt.Sprintf("/m/%d.mkv", i), fmt.Sprintf("Movie %d", i), fmt.Sprintf("Movie %d", i),
			1950+i%70, "An overview that is about as long as one is, which is to say a couple of sentences of text.", genres[i%4], int64(i), int64(i), 5400); err != nil {
			b.Fatal(err)
		}
		if i%2 == 0 {
			if _, err := tx.Exec(`INSERT INTO progress(user_id, item_id, position_sec, duration_sec, watched, updated_at) VALUES(?,?,?,?,?,?)`, user, id, 120+i, 5400, i%4/2, int64(i)); err != nil {
				b.Fatal(err)
			}
		}
	}
	firstShow := int64(0)
	for s := 0; s < benchShows; s++ {
		r, err := tx.Exec(`INSERT INTO shows(library_id, path, title, sort_title, genres, added_at, updated_at) VALUES(?,?,?,?,?,?,?)`,
			tv.ID, fmt.Sprintf("/t/%d", s), fmt.Sprintf("Show %d", s), fmt.Sprintf("Show %d", s), genres[s%4], int64(s), int64(s))
		if err != nil {
			b.Fatal(err)
		}
		show, _ := r.LastInsertId()
		if firstShow == 0 {
			firstShow = show
		}
		for e := 0; e < benchEpisodes; e++ {
			id++
			if _, err := tx.Exec(`INSERT INTO items(id, library_id, kind, path, title, sort_title, overview, added_at, updated_at, show_id, season, episode, runtime_sec)
				VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, tv.ID, "video", fmt.Sprintf("/t/%d/%d.mkv", s, e), fmt.Sprintf("Episode %d", e), fmt.Sprintf("Episode %d", e),
				"An overview of the episode.", int64(e), int64(e), show, 1+e/10, 1+e%10, 1400); err != nil {
				b.Fatal(err)
			}
			if s%10 == 0 && e < 25 { // a tenth of the shows are part-way through
				if _, err := tx.Exec(`INSERT INTO progress(user_id, item_id, position_sec, duration_sec, watched, updated_at) VALUES(?,?,?,?,1,?)`, user, id, 1400, 1400, int64(e)); err != nil {
					b.Fatal(err)
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	return st, movies.ID, firstShow, user
}

func BenchmarkStore(b *testing.B) {
	st, lib, show, user := benchStore(b)
	ctx := context.Background()
	check := func(b *testing.B, err error) {
		if err != nil {
			b.Fatal(err)
		}
	}
	access := &Access{MaxAge: 7}
	b.Run("Libraries", func(b *testing.B) {
		for b.Loop() {
			_, err := st.Libraries(ctx)
			check(b, err)
		}
	})
	b.Run("Library", func(b *testing.B) {
		for b.Loop() {
			_, err := st.Library(ctx, lib)
			check(b, err)
		}
	})
	b.Run("ItemsPageByTitle", func(b *testing.B) {
		for b.Loop() {
			_, _, err := st.Items(ctx, ItemQuery{LibraryID: lib, Limit: 60})
			check(b, err)
		}
	})
	b.Run("ItemsRecentlyAdded", func(b *testing.B) {
		for b.Loop() {
			_, _, err := st.Items(ctx, ItemQuery{LibraryID: lib, Sort: "added", Desc: true, Limit: 20})
			check(b, err)
		}
	})
	b.Run("ItemsFeatured", func(b *testing.B) {
		for b.Loop() {
			_, _, err := st.Items(ctx, ItemQuery{Sort: "added", Desc: true, Limit: 40, NoEpisodes: true})
			check(b, err)
		}
	})
	b.Run("ItemsKidsProfile", func(b *testing.B) {
		for b.Loop() {
			_, _, err := st.Items(ctx, ItemQuery{Limit: 60, Access: access})
			check(b, err)
		}
	})
	b.Run("ShowsAll", func(b *testing.B) {
		for b.Loop() {
			_, err := st.Shows(ctx, ShowQuery{LibraryID: lib + 1, UserID: user})
			check(b, err)
		}
	})
	b.Run("ShowsRecentlyAdded", func(b *testing.B) {
		for b.Loop() {
			_, err := st.Shows(ctx, ShowQuery{LibraryID: lib + 1, Sort: "added", Desc: true, Limit: 20, UserID: user})
			check(b, err)
		}
	})
	b.Run("Show", func(b *testing.B) {
		for b.Loop() {
			_, err := st.Show(ctx, show, user)
			check(b, err)
		}
	})
	b.Run("Episodes", func(b *testing.B) {
		for b.Loop() {
			_, err := st.Episodes(ctx, show, nil)
			check(b, err)
		}
	})
	b.Run("NextEpisode", func(b *testing.B) {
		sh := show
		it := &Item{ID: 5001, ShowID: &sh}
		for b.Loop() {
			st.NextEpisode(ctx, it)
		}
	})
	b.Run("ContinueWatching", func(b *testing.B) {
		for b.Loop() {
			_, err := st.ContinueWatching(ctx, user, 20)
			check(b, err)
		}
	})
	b.Run("AttachProgress20", func(b *testing.B) {
		items := make([]*Item, 20)
		for i := range items {
			items[i] = &Item{ID: int64(i + 1)}
		}
		for b.Loop() {
			check(b, st.AttachProgress(ctx, user, items))
		}
	})
	b.Run("Genres", func(b *testing.B) {
		for b.Loop() {
			_, err := st.Genres(ctx, 0, nil)
			check(b, err)
		}
	})
	b.Run("Albums", func(b *testing.B) {
		for b.Loop() {
			_, err := st.Albums(ctx, lib, "", "", "", nil)
			check(b, err)
		}
	})
}

// BenchmarkHome is every query behind the home page, as the server runs them.
func BenchmarkHomeQueries(b *testing.B) {
	st, lib, _, user := benchStore(b)
	ctx := context.Background()
	for b.Loop() {
		cont, _ := st.ContinueWatching(ctx, user, 20)
		_ = cont
		libs, err := st.Libraries(ctx)
		if err != nil {
			b.Fatal(err)
		}
		// Next up: the started shows of each TV library.
		for _, l := range libs {
			if l.Type != "shows" {
				continue
			}
			shows, err := st.Shows(ctx, ShowQuery{LibraryID: l.ID, UserID: user, Started: true})
			if err != nil {
				b.Fatal(err)
			}
			var started []int64
			for _, sh := range shows {
				if sh.Unwatched != 0 && sh.Unwatched != sh.EpisodeCount {
					started = append(started, sh.ID)
				}
			}
			if _, err := st.NextUpItems(ctx, user, started, nil, 20); err != nil {
				b.Fatal(err)
			}
			if _, err := st.Shows(ctx, ShowQuery{LibraryID: l.ID, Sort: "added", Desc: true, Limit: 20, UserID: user}); err != nil {
				b.Fatal(err)
			}
		}
		items, _, err := st.Items(ctx, ItemQuery{LibraryID: lib, Sort: "added", Desc: true, Limit: 20, NoTotal: true})
		if err != nil {
			b.Fatal(err)
		}
		_ = st.AttachProgress(ctx, user, items)
		all, _, _ := st.Items(ctx, ItemQuery{Sort: "added", Desc: true, Limit: 40, NoEpisodes: true, NoTotal: true})
		_ = st.AttachProgress(ctx, user, all)
	}
}
