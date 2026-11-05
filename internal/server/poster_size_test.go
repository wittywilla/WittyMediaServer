package server

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// noisyJPEG writes a picture that compresses badly, as a photographed poster does.
func noisyJPEG(t *testing.T, path string, w, h int) int {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{uint8(x * 7), uint8(y * 3), uint8(x ^ y), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return buf.Len()
}

// jpegWidth is the width of a JPEG answer, or fails the test.
func jpegWidth(t *testing.T, what string, b []byte) int {
	t.Helper()
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
	return cfg.Width
}

// A grid of films asks for each poster ?w=300, and a page's backdrop ?w=1920: what each
// gets is a smaller copy, not the file beside the film, which can be megabytes.
func TestArtAskedSmallerIsSmaller(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "admin", "adult", true, "")
	dir := t.TempDir()
	lib := ts.library(t, "movies", dir)
	film := filepath.Join(dir, "Film (2001)")
	if err := os.MkdirAll(film, 0o755); err != nil {
		t.Fatal(err)
	}
	poster, backdrop := filepath.Join(film, "poster.jpg"), filepath.Join(film, "fanart.jpg")
	posterSize := noisyJPEG(t, poster, 1000, 1500)
	noisyJPEG(t, backdrop, 2400, 1350)
	it := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: filepath.Join(film, "Film (2001).mkv"), Title: "Film", Poster: poster, Backdrop: backdrop})

	whole := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/poster", it.ID), tok, nil)
	if whole.Code != 200 || whole.Body.Len() != posterSize {
		t.Fatalf("without ?w=: %d, %d bytes of %d", whole.Code, whole.Body.Len(), posterSize)
	}
	small := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/poster?w=300", it.ID), tok, nil)
	if small.Code != 200 || small.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("?w=300: %d %s", small.Code, small.Header().Get("Content-Type"))
	}
	if w := jpegWidth(t, "poster", small.Body.Bytes()); w != 320 {
		t.Errorf("poster ?w=300 is %d wide", w)
	}
	if small.Body.Len()*4 > posterSize {
		t.Errorf("%d bytes for the small copy, %d for the poster", small.Body.Len(), posterSize)
	}
	for _, c := range []struct {
		q    string
		want int
	}{{"?w=1920", 1920}, {"?w=1280", 1280}, {"?w=700", 1280}, {"", 2400}} {
		b := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/backdrop%s", it.ID, c.q), tok, nil)
		if b.Code != 200 {
			t.Fatalf("backdrop%s: %d", c.q, b.Code)
		}
		if w := jpegWidth(t, "backdrop"+c.q, b.Body.Bytes()); w != c.want {
			t.Errorf("backdrop%s is %d wide, want %d", c.q, w, c.want)
		}
	}

	// A show's poster and backdrop, the same way.
	shows := ts.library(t, "shows", dir)
	sh := &library.Show{LibraryID: shows, Path: film, Title: "Show", SortTitle: "Show", Poster: poster, Backdrop: backdrop}
	if err := ts.Store.UpsertShow(context.Background(), sh); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		kind string
		want int
	}{{"poster", 320}, {"backdrop", 320}} {
		b := ts.do(t, "GET", fmt.Sprintf("/api/shows/%d/art/%s?w=300", sh.ID, c.kind), tok, nil)
		if b.Code != 200 {
			t.Fatalf("show %s: %d", c.kind, b.Code)
		}
		if w := jpegWidth(t, "show "+c.kind, b.Body.Bytes()); w != c.want {
			t.Errorf("show %s ?w=300 is %d wide", c.kind, w)
		}
	}

	// A poster named outside the library's folders stays refused, small or not.
	outside := filepath.Join(t.TempDir(), "poster.jpg")
	noisyJPEG(t, outside, 400, 600)
	other := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: filepath.Join(film, "Other.mkv"), Title: "Other", Poster: outside})
	for _, q := range []string{"", "?w=300"} {
		if w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/poster%s", other.ID, q), tok, nil); w.Code != 404 {
			t.Errorf("outside the library%s: %d", q, w.Code)
		}
	}
}
