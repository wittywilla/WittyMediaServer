package server

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// A grid of films asks for each poster ?w=300: what it gets is a small copy, not the
// file beside the film, which can be megabytes.
func TestPosterAskedSmallerIsSmaller(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "admin", "adult", true, "")
	dir := t.TempDir()
	lib := ts.library(t, "movies", dir)
	film := filepath.Join(dir, "Film (2001)")
	if err := os.MkdirAll(film, 0o755); err != nil {
		t.Fatal(err)
	}
	poster := filepath.Join(film, "poster.jpg")
	img := image.NewRGBA(image.Rect(0, 0, 1000, 1500))
	for y := range 1500 {
		for x := range 1000 {
			img.Set(x, y, color.RGBA{uint8(x * 7), uint8(y * 3), uint8(x ^ y), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(poster, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	it := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: filepath.Join(film, "Film (2001).mkv"), Title: "Film", Poster: poster})

	whole := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/poster", it.ID), tok, nil)
	if whole.Code != 200 || whole.Body.Len() != buf.Len() {
		t.Fatalf("without ?w=: %d, %d bytes of %d", whole.Code, whole.Body.Len(), buf.Len())
	}
	small := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/poster?w=300", it.ID), tok, nil)
	if small.Code != 200 || small.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("?w=300: %d %s", small.Code, small.Header().Get("Content-Type"))
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(small.Body.Bytes()))
	if err != nil || cfg.Width != 320 || cfg.Height != 480 {
		t.Fatalf("%v %+v", err, cfg)
	}
	if small.Body.Len()*4 > buf.Len() {
		t.Errorf("%d bytes for the small copy, %d for the poster", small.Body.Len(), buf.Len())
	}
	t.Logf("poster %d KiB, ?w=300 %d KiB", buf.Len()>>10, small.Body.Len()>>10)

	// A poster named outside the library's folders stays refused, small or not.
	outside := filepath.Join(t.TempDir(), "poster.jpg")
	if err := os.WriteFile(outside, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	other := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: filepath.Join(film, "Other.mkv"), Title: "Other", Poster: outside})
	for _, q := range []string{"", "?w=300"} {
		if w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/poster%s", other.ID, q), tok, nil); w.Code != 404 {
			t.Errorf("outside the library%s: %d", q, w.Code)
		}
	}
}
