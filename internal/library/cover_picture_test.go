package library

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

func TestPictureDecoderFromItsFirstBytes(t *testing.T) {
	for _, c := range []struct {
		name string
		head []byte
		want string
	}{
		{"JPEG", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F'}, "mjpeg"},
		{"PNG", []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), "png"},
		{"GIF", []byte("GIF89a\x01\x00\x01\x00"), "gif"},
		{"WebP", []byte("RIFF\x24\x00\x00\x00WEBPVP8 "), "webp"},
		{"BMP", []byte("BM\x36\x00\x00\x00\x00\x00\x00\x00\x36\x00\x00\x00"), "bmp"},
		{"TIFF, little-endian", []byte("II*\x00\x08\x00\x00\x00"), "tiff"},
		{"TIFF, big-endian", []byte("MM\x00*\x00\x00\x00\x08"), "tiff"},
		{"JPEG 2000", []byte{0, 0, 0, 0x0C, 'j', 'P', ' ', ' ', 0x0D, 0x0A, 0x87, 0x0A}, "jpeg2000"},
		{"JPEG 2000 codestream", []byte{0xFF, 0x4F, 0xFF, 0x51, 0, 0x2F}, "jpeg2000"},
		{"not a picture", []byte("%PDF-1.7\n"), ""},
		{"nothing", nil, ""},
		{"too short to tell", []byte{0xFF, 0xD8}, ""},
	} {
		if got := pictureDecoder(c.head); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// A comic's first page called 000.jpg that is really a PNG, as in several public-domain comics: FFmpeg read it
// as a JPEG by its name, failed ("bits 193 is invalid"), and the comic got no cover.
func TestCoverFromAPNGCalledJPG(t *testing.T) {
	tools, err := ffmpeg.Locate(os.Getenv("WMS_FFMPEG"), "")
	if err != nil {
		t.Skip(err)
	}
	pic := image.NewRGBA(image.Rect(0, 0, 600, 900))
	for y := 0; y < 900; y++ {
		for x := 0; x < 600; x++ {
			pic.Set(x, y, color.RGBA{uint8(x / 3), uint8(y / 4), 120, 255})
		}
	}
	var page bytes.Buffer
	if err := png.Encode(&page, pic); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	archive := filepath.Join(dir, "Max und Moritz 04.cbz")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("000.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(page.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	s := &Scanner{Tools: tools, ImagesDir: filepath.Join(dir, "images"), CacheDir: filepath.Join(dir, "cache"), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	check := func(what, out string) {
		t.Helper()
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatalf("%s: no cover: %v", what, err)
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("%s: the cover isn't a JPEG: %v", what, err)
		}
		if cfg.Width != 400 || cfg.Height != 600 {
			t.Fatalf("%s: the cover is %dx%d, want 400x600", what, cfg.Width, cfg.Height)
		}
	}
	ctx := context.Background()
	out := filepath.Join(dir, "images", "b-from-archive.jpg")
	if err := s.makeCover(ctx, archive, "000.jpg", out); err != nil {
		t.Fatalf("from the archive: %v", err)
	}
	check("from the archive", out)
	// The same picture already in memory, said to be a JPEG, as a book's cover can be.
	out = filepath.Join(dir, "images", "b-from-bytes.jpg")
	if err := s.coverFromBytes(ctx, page.Bytes(), ".jpg", out); err != nil {
		t.Fatalf("from memory: %v", err)
	}
	check("from memory", out)
}
