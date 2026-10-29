package library

import (
	"context"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

func TestAnnotations(t *testing.T) {
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	s := &Store{DB: d}
	for _, name := range []string{"a", "b"} {
		if _, err := d.ExecContext(ctx, `INSERT INTO users(name, password_hash, is_admin, created_at) VALUES(?, '', 0, 0)`, name); err != nil {
			t.Fatal(err)
		}
	}
	lib, err := s.CreateLibrary(ctx, "Books", "books", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	book := &Item{LibraryID: lib.ID, Kind: "book", Path: "/b/moby.epub", Title: "Moby Dick", SortTitle: "Moby Dick"}
	if err := s.upsertItem(ctx, book); err != nil {
		t.Fatal(err)
	}

	ink := &Annotation{ItemID: book.ID, Kind: "ink", Chapter: 2, Pos: 2.5, Color: "#0a2a7a", Data: `{"t":"pen","w":0.1,"o":1,"a":12,"pts":[0,0,0.5,1,1,0.6]}`}
	if err := s.SaveAnnotation(ctx, 1, ink); err != nil {
		t.Fatal(err)
	}
	hl := &Annotation{ItemID: book.ID, Kind: "highlight", Chapter: 1, Pos: 1.2, Quote: "Call me Ishmael.", Color: "yellow", Data: "{}"}
	if err := s.SaveAnnotation(ctx, 1, hl); err != nil {
		t.Fatal(err)
	}
	if hl.Data != "" {
		t.Error("only ink keeps stroke data")
	}

	bad := []*Annotation{
		{ItemID: book.ID, Kind: "scribble"},
		{ItemID: book.ID, Kind: "highlight", Color: "red; background:url(x)"},
		{ItemID: book.ID, Kind: "ink", Color: "#000000", Data: "not json"},
		{ItemID: book.ID, Kind: "ink", Color: "#000000", Data: `"` + strings.Repeat("x", maxInk) + `"`},
	}
	for i, a := range bad {
		if s.SaveAnnotation(ctx, 1, a) == nil {
			t.Errorf("bad annotation %d was saved", i)
		}
	}

	list, err := s.Annotations(ctx, 1, book.ID)
	if err != nil || len(list) != 2 || list[0].Kind != "highlight" || list[1].Data != ink.Data {
		t.Fatalf("annotations in reading order with ink data, got %+v %v", list, err)
	}
	// Another account sees nothing and cannot change or remove them.
	if other, _ := s.Annotations(ctx, 2, book.ID); len(other) != 0 {
		t.Error("annotations leaked to another account")
	}
	if err := s.SaveAnnotation(ctx, 2, &Annotation{ID: hl.ID, Kind: "highlight", Color: "green"}); err == nil {
		t.Error("another account changed a highlight")
	}
	_ = s.DeleteAnnotation(ctx, 2, hl.ID)
	if list, _ := s.Annotations(ctx, 1, book.ID); len(list) != 2 {
		t.Error("another account removed a highlight")
	}
}
