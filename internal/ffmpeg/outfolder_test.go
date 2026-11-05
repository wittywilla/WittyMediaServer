package ffmpeg

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Issue #66: a picture is written into a folder that may have gone since the server
// started. Every picture FFmpeg writes for the server makes its folder first.
func TestPicturesMakeTheirFolder(t *testing.T) {
	tools, err := Locate(os.Getenv("WMS_FFMPEG"), "")
	if err != nil {
		t.Skip(err)
	}
	ctx := context.Background()
	src := []string{"-f", "lavfi", "-i", "color=c=red:s=64x48:d=1"}
	root := t.TempDir()
	for name, write := range map[string]func(out string) error{
		"still":  func(out string) error { return tools.Still(ctx, src, 0, 32, out) },
		"poster": func(out string) error { return tools.Poster(ctx, src, 0.5, 32, out) },
	} {
		out := filepath.Join(root, name, "cache", "images", "thumb-test.jpg")
		if err := write(out); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if st, err := os.Stat(out); err != nil || st.Size() == 0 {
			t.Fatalf("%s: nothing written: %v", name, err)
		}
	}
	// And a photo's thumbnail, from a picture on disk.
	pic := filepath.Join(root, "photo.png")
	if err := tools.Still(ctx, src, 0, 64, pic); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "gone", "thumbs", "photo.jpg")
	if err := tools.ImageThumb(ctx, pic, 1, 32, 0, out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal(err)
	}
}
