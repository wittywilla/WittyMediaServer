package library

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/gamemeta"
)

// An NFO this server wrote is an old answer, for a better one to replace. What a person
// wrote is theirs. A wrong match saved beside a game once mustn't stick to it for good.
func TestSavedAnswersDontBindAGame(t *testing.T) {
	src := &fakeSource{id: "a", answer: func(gamemeta.Query) (*gamemeta.Info, error) {
		return &gamemeta.Info{ID: "1", Title: "Right Name", Method: gamemeta.MethodChecksum, Confidence: 100, Description: "Right words", Developer: "Right maker"}, nil
	}}
	f := newMetaFixture(t, src)
	saved := nfoMarker + `<movie><title>Wrong Name</title><plot>Wrong words</plot><studio>Wrong maker</studio></movie>`
	if err := os.WriteFile(filepath.Join(f.root, "Game.nfo"), []byte(saved), 0o644); err != nil {
		t.Fatal(err)
	}
	it := f.scanFile(t, "Game.sfc")
	f.work(t)
	got := f.item(t, it.ID)
	if got.Title != "Right Name" || got.Overview != "Right words" || got.Studio != "Right maker" {
		t.Fatalf("an old answer stuck: %+v", got)
	}
}

// Listed before files were judged, with an NFO this server wrote counted as the owner's. A
// source gave that title, so the next scan puts it right, once, and the game keeps its item.
func TestAGameFromBeforeIsGoneOverOnce(t *testing.T) {
	src := &fakeSource{id: "a", answer: func(gamemeta.Query) (*gamemeta.Info, error) {
		return &gamemeta.Info{ID: "1", Title: "Right Name", Method: gamemeta.MethodChecksum, Confidence: 100}, nil
	}}
	f := newMetaFixture(t, src)
	saved := nfoMarker + `<movie><title>Wrong Name</title></movie>`
	if err := os.WriteFile(filepath.Join(f.root, "Game.nfo"), []byte(saved), 0o644); err != nil {
		t.Fatal(err)
	}
	it := f.scanFile(t, "Game.sfc")
	if _, err := f.store.DB.ExecContext(f.ctx, `UPDATE game_meta SET detect_how = '', edited = 'title', applied = 'title', status = 'done' WHERE item_id = ?`, it.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.DB.ExecContext(f.ctx, `UPDATE items SET title = 'Wrong Name' WHERE id = ?`, it.ID); err != nil {
		t.Fatal(err)
	}
	sc := NewScanner(f.store, nil, t.TempDir(), func() string { return "" }, discardLog())
	sc.GameMeta = f.g
	if err := sc.scan(f.ctx, f.lib.ID); err != nil {
		t.Fatal(err)
	}
	f.work(t)
	if got := f.item(t, it.ID); got.Title != "Right Name" {
		t.Fatalf("%+v", got)
	}
}

// A title someone typed in an earlier version has to survive the first scan of a new one,
// which goes over every game again, whatever is or isn't beside the file.
func TestATypedTitleSurvivesTheFirstScanAfterAnUpgrade(t *testing.T) {
	const typed = "Typed By Hand"
	for _, c := range []struct{ name, nfo string }{
		{name: "nothing beside it"},
		{name: "saved beside it by this server", nfo: nfoMarker + "<movie><title>" + typed + "</title></movie>"},
		{name: "an older answer saved beside it", nfo: nfoMarker + "<movie><title>Wrong Name</title></movie>"},
		{name: "a file of the owner's beside it", nfo: "<movie><title>Their Own Name</title></movie>"},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := &fakeSource{id: "a", answer: func(gamemeta.Query) (*gamemeta.Info, error) {
				return &gamemeta.Info{ID: "1", Title: "Right Name", Method: gamemeta.MethodChecksum, Confidence: 100}, nil
			}}
			f := newMetaFixture(t, src)
			it := f.scanFile(t, "Game.sfc")
			f.work(t)
			if c.nfo != "" {
				if err := os.WriteFile(filepath.Join(f.root, "Game.nfo"), []byte(c.nfo), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			// As an earlier version left it: typed by hand, and not yet judged.
			got := f.item(t, it.ID)
			got.Title, got.SortTitle = typed, SortTitle(typed)
			if err := f.store.upsertItem(f.ctx, got); err != nil {
				t.Fatal(err)
			}
			if err := f.store.MarkGameEdited(f.ctx, it.ID, "title"); err != nil {
				t.Fatal(err)
			}
			if _, err := f.store.DB.ExecContext(f.ctx, `UPDATE game_meta SET detect_how = '' WHERE item_id = ?`, it.ID); err != nil {
				t.Fatal(err)
			}
			for i := 1; i <= 2; i++ {
				if got := f.scanFile2(t, it.ID); got.Title != typed {
					t.Fatalf("scan %d made it %q", i, got.Title)
				}
				f.work(t)
				if got := f.item(t, it.ID); got.Title != typed {
					t.Fatalf("what was found after scan %d made it %q", i, got.Title)
				}
			}
			if !f.store.gameFieldTyped(f.ctx, it.ID, "title") {
				t.Fatal("it's no longer marked as typed, so a later scan could lose it")
			}
		})
	}
}

// Saved beside the file because someone typed it, and then the file changes.
func TestATypedTitleSavedBesideAGameSurvivesAChangedFile(t *testing.T) {
	src := &fakeSource{id: "a", answer: func(gamemeta.Query) (*gamemeta.Info, error) {
		return &gamemeta.Info{ID: "1", Title: "Right Name", Method: gamemeta.MethodChecksum, Confidence: 100}, nil
	}}
	f := newMetaFixture(t, src)
	it := f.scanFile(t, "Game.sfc")
	f.work(t)
	got := f.item(t, it.ID)
	got.Title, got.SortTitle = "Typed By Hand", SortTitle("Typed By Hand")
	if err := f.store.upsertItem(f.ctx, got); err != nil {
		t.Fatal(err)
	}
	if err := f.store.MarkGameEdited(f.ctx, it.ID, "title"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.root, "Game.nfo"), []byte(nfoMarker+"<movie><title>Typed By Hand</title></movie>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(got.Path, gameBytes("another dump"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.scanFile2(t, it.ID)
	f.work(t)
	if got := f.item(t, it.ID); got.Title != "Typed By Hand" {
		t.Fatalf("it became %q", got.Title)
	}
}
