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

// Listed before files were judged, with an NFO this server wrote counted as the owner's:
// the next scan puts that right, once, and the game keeps its item.
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
	if _, err := f.store.DB.ExecContext(f.ctx, `UPDATE game_meta SET detect_how = '', edited = 'title', applied = '', status = 'done' WHERE item_id = ?`, it.ID); err != nil {
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
