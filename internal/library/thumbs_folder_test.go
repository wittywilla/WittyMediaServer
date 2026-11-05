package library

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// Issue #66: after a reset took cache\images away, every video's thumbnail failed with
// FFmpeg's "Could not open file" until the server restarted. A thumbnail makes its folder.
func TestThumbnailAfterItsFolderWasRemoved(t *testing.T) {
	tools, err := ffmpeg.Locate(os.Getenv("WMS_FFMPEG"), "")
	if err != nil {
		t.Skip(err)
	}
	if !tools.Encoders["mpeg4"] {
		t.Skip("this FFmpeg can't make the test video")
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "A Film.mkv")
	if _, _, err := ffmpeg.Run(context.Background(), tools.FFmpeg, []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-f", "lavfi", "-i", "testsrc=d=3:s=128x72:r=10", "-c:v", "mpeg4", video}); err != nil {
		t.Fatal(err)
	}
	images := filepath.Join(dir, "cache", "images")
	s := &Scanner{Tools: tools, ImagesDir: images, CacheDir: filepath.Join(dir, "cache"), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if _, err := os.Stat(images); err == nil {
		t.Fatal("the folder is there already, so this test shows nothing")
	}
	got, err := s.thumbnail(context.Background(), &Item{Kind: "video", Path: video, RuntimeSec: 3})
	if err != nil {
		t.Fatal(err)
	}
	name := got[len("cache:"):]
	if st, err := os.Stat(filepath.Join(images, name)); err != nil || st.Size() == 0 {
		t.Fatalf("no thumbnail: %v", err)
	}
}
