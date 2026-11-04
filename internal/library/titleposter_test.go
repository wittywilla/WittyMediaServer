package library

import (
	"context"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A title's colour is the one the web app gives a card without a poster.
func TestHueOfMatchesTheWebApp(t *testing.T) {
	// Worked out with the web app's artHue.
	for title, want := range map[string]int{"Big Buck Bunny": 321, "Ça 🎬 va": 221} {
		if got := HueOf(title); got != want {
			t.Errorf("%q: %d, want %d", title, got, want)
		}
	}
}

func TestMakeTitlePoster(t *testing.T) {
	s := &Scanner{ImagesDir: t.TempDir()}
	u, err := s.MakeTitlePoster(TitlePoster{Title: "  The   Adventures of a Very Long Title That Has To Wrap Over Several Lines  ", Line: "1925", Hue: -1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(u, "cache:made-") || SourceLabel(SourceMade) == "" {
		t.Fatalf("%q", u)
	}
	f, err := os.Open(filepath.Join(s.ImagesDir, strings.TrimPrefix(u, "cache:")))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(f)
	f.Close()
	if err != nil || cfg.Width != 1000 || cfg.Height != 1500 {
		t.Fatalf("%+v %v", cfg, err)
	}
	// The same words and look are the same file; spaces don't count.
	again, _ := s.MakeTitlePoster(TitlePoster{Title: "The Adventures of a Very Long Title That Has To Wrap Over Several Lines", Line: "1925", Hue: -1})
	if again != u {
		t.Errorf("made again: %q, %q", again, u)
	}
	other, _ := s.MakeTitlePoster(TitlePoster{Title: "The Adventures of a Very Long Title That Has To Wrap Over Several Lines", Line: "1925", Hue: 10})
	if other == u {
		t.Error("another colour should be another picture")
	}
	for _, bad := range []TitlePoster{{Title: "   "}, {Title: strings.Repeat("x", 121)}, {Title: "A", Frame: "/etc/passwd"}, {Title: "A", Frame: "cache:../x.jpg"}, {Title: "A", Frame: "cache:missing.jpg"}} {
		if _, err := s.MakeTitlePoster(bad); err == nil {
			t.Errorf("%+v was made", bad)
		}
	}

	// Over a frame: the frame's picture is under the words.
	frame := image.NewRGBA(image.Rect(0, 0, 64, 36))
	fp, _ := os.Create(filepath.Join(s.ImagesDir, "frame-x.jpg"))
	_ = jpeg.Encode(fp, frame, nil)
	fp.Close()
	if u, err := s.MakeTitlePoster(TitlePoster{Title: "Over a frame", Hue: -1, Frame: "cache:frame-x.jpg"}); err != nil || !strings.HasPrefix(u, "cache:made-") {
		t.Errorf("%q %v", u, err)
	}
}

// What a video with no poster gets follows the setting.
func TestFallbackPosterFollowsTheSetting(t *testing.T) {
	year := 1902
	it := &Item{Kind: "video", Path: filepath.Join(t.TempDir(), "a.mkv"), Title: "A Trip", Year: &year}
	mode := NoPosterTitle
	s := &Scanner{ImagesDir: t.TempDir(), NoPoster: func() string { return mode }}
	p, src, err := s.fallbackPoster(context.Background(), it)
	if err != nil || !strings.HasPrefix(p, "cache:made-") || src != SourceMade {
		t.Fatalf("title: %q %q %v", p, src, err)
	}
	mode = NoPosterNone
	if p, _, err := s.fallbackPoster(context.Background(), it); p != "" || err != nil {
		t.Errorf("none: %q %v", p, err)
	}
	// A made poster is one of ours: a real one replaces it, and it's never written beside the media.
	if !IsMadePoster("cache:made-1.jpg") || !IsMadePoster("cache:thumb-1.jpg") || IsMadePoster("cache:up-1.jpg") {
		t.Error("IsMadePoster")
	}
	tt := &Title{Item: &Item{Poster: "cache:made-1.jpg"}, Meta: &TitleMeta{Sources: map[string]string{}}}
	if got := tt.sourceOf("poster"); got != SourceMade {
		t.Errorf("source: %q", got)
	}
}
