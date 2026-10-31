package bdsession

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
)

func testManager(t *testing.T) *Manager {
	t.Helper()
	tools := &ffmpeg.Tools{FFmpeg: "ffmpeg", FFprobe: "ffprobe", Encoders: map[string]bool{}}
	dir := t.TempDir()
	log := slog.New(slog.DiscardHandler)
	return &Manager{HLS: hls.NewManager(tools, dir+"/hls", 2, log), Tools: tools, CacheDir: dir, BaseURL: "/api", sessions: map[string]*Session{}}
}

// TestRealDisc walks an unencrypted Blu-ray when WMS_TEST_BD points at one
// (skipped otherwise): first play to a menu or title, its buttons, the
// feature, a menu call and back, and the feature's subtitles.
func TestRealDisc(t *testing.T) {
	p := os.Getenv("WMS_TEST_BD")
	if p == "" {
		t.Skip("set WMS_TEST_BD to an unencrypted Blu-ray image or folder")
	}
	m := testManager(t)
	s, err := m.Open(1, p, Prefs{AudioLanguage: "en"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	cur := s.Start(ctx)
	for i := 0; i < 8 && cur.Kind == "menu" && len(cur.Menu.Buttons) == 0; i++ {
		cur = s.Do(ctx, Action{Type: "ended"})
	}
	t.Logf("after first play: %s %q", cur.Kind, cur.Reason)
	if cur.Kind == "stop" {
		t.Fatalf("stopped: %s", cur.Reason)
	}
	if cur.Kind == "menu" {
		if cur.Media.Loop {
			t.Log("the menu loops")
		}
		if b, ok := s.OverlayPNG(s.serial*1000 + s.artSeq); !ok || len(b) == 0 {
			t.Errorf("no overlay for the menu")
		}
		// Moving the selection tells the disc's registers.
		for _, k := range []string{"down", "up"} {
			cur = s.Do(ctx, Action{Type: "key", Key: k})
		}
		if got := s.nav.Registers().PSR[wittybluray.PSRButton]; int(got)+1 != cur.Menu.Selected {
			t.Errorf("PSR10 = %d, selected %d", got, cur.Menu.Selected)
		}
	}
	main := s.disc.MainPlaylist()
	cur = s.Do(ctx, Action{Type: "title", TTN: main.Number})
	if cur.Kind != "title" || cur.Title == nil {
		t.Fatalf("feature: %+v", cur)
	}
	if cur.Title.SubStream != -1 {
		t.Errorf("subtitles on by themselves: stream %d", cur.Title.SubStream)
	}
	if cur.Title.AudioStream < 0 || len(cur.Title.Audio) == 0 {
		t.Errorf("audio: %+v", cur.Title)
	}
	evs, err := s.Subtitles(main.Number, 0, 600, 900)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d subtitle events between 10 and 15 minutes", len(evs))
	for _, ev := range evs {
		if ev.End < ev.Start {
			t.Errorf("event %+v", ev)
		}
	}
	// To the menu and back resumes where the film was.
	cur = s.Do(ctx, Action{Type: "menu", PositionSec: 1234})
	t.Logf("menu call: %s", cur.Kind)
	if cur.Kind == "stop" {
		t.Fatalf("stopped: %s", cur.Reason)
	}
}
