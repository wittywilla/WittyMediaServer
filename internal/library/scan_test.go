package library

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

func scanFixture(t *testing.T) (*Scanner, *Library, string) {
	t.Helper()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	root := filepath.Join(t.TempDir(), "books")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	st := &Store{DB: d}
	lib, err := st.CreateLibrary(context.Background(), "Books", "books", []string{root})
	if err != nil {
		t.Fatal(err)
	}
	s := &Scanner{Store: st, ImagesDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	return s, lib, root
}

func visible(t *testing.T, s *Scanner, lib int64) map[string]*Item {
	t.Helper()
	items, _, err := s.Store.Items(context.Background(), ItemQuery{LibraryID: lib})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]*Item{}
	for _, it := range items {
		out[filepath.Base(it.Path)] = it
	}
	return out
}

// A drive that is away is not the same as every file having been deleted: its
// items stay, and when it is back nothing has been lost.
func TestLibraryFolderAwayKeepsItems(t *testing.T) {
	s, lib, root := scanFixture(t)
	ctx := context.Background()
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte("%PDF-"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("one.pdf")
	write("two.pdf")
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	got := visible(t, s, lib.ID)
	if len(got) != 2 {
		t.Fatalf("found %d books", len(got))
	}
	// Someone edits a title by hand.
	one := got["one.pdf"]
	one.Title = "My Title"
	if err := s.Store.UpdateItem(ctx, one); err != nil {
		t.Fatal(err)
	}

	// The folder goes away.
	away := root + "-away"
	if err := os.Rename(root, away); err != nil {
		t.Fatal(err)
	}
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	if n := len(visible(t, s, lib.ID)); n != 2 {
		t.Fatalf("%d books left while the folder is away", n)
	}
	// Mounted again, but empty: an unmounted mount point looks like this.
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	if n := len(visible(t, s, lib.ID)); n != 2 {
		t.Fatalf("%d books left under an empty mount point", n)
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(away, root); err != nil {
		t.Fatal(err)
	}

	// A file really deleted is hidden; put back unchanged, it returns with its
	// edit, and without being read again.
	saved, err := os.ReadFile(filepath.Join(root, "one.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(root, "one.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "one.pdf")); err != nil {
		t.Fatal(err)
	}
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	got = visible(t, s, lib.ID)
	if len(got) != 1 || got["two.pdf"] == nil {
		t.Fatalf("after deleting a file: %v", got)
	}
	if err := os.WriteFile(filepath.Join(root, "one.pdf"), saved, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(root, "one.pdf"), st.ModTime(), st.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := s.scan(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	got = visible(t, s, lib.ID)
	if got["one.pdf"] == nil || got["one.pdf"].Title != "My Title" {
		t.Fatalf("the file came back without its edit: %+v", got["one.pdf"])
	}
}

// A scan that changes nothing leaves a show alone: the address of its pictures
// carries the time it last changed, and browsers would fetch them all again.
func TestUnchangedShowIsNotSavedAgain(t *testing.T) {
	s, _, _ := scanFixture(t)
	ctx := context.Background()
	lib, err := s.Store.CreateLibrary(ctx, "TV", "shows", []string{"/t"})
	if err != nil {
		t.Fatal(err)
	}
	sh := &Show{LibraryID: lib.ID, Path: "/t/a", Title: "A", Overview: "x", Genres: []string{"Drama"}}
	if err := s.Store.UpsertShow(ctx, sh); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.DB.ExecContext(ctx, `UPDATE shows SET updated_at = 1 WHERE id = ?`, sh.ID); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Store.Show(ctx, sh.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	before := *loaded
	if err := s.saveShow(ctx, loaded, &before); err != nil {
		t.Fatal(err)
	}
	if again, _ := s.Store.Show(ctx, sh.ID, 0); again.UpdatedAt != 1 {
		t.Fatal("an unchanged show was written")
	}
	loaded.Overview = "changed"
	if err := s.saveShow(ctx, loaded, &before); err != nil {
		t.Fatal(err)
	}
	if again, _ := s.Store.Show(ctx, sh.ID, 0); again.UpdatedAt == 1 {
		t.Fatal("a changed show was not written")
	}
}
