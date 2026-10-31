package bdsession

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/seamcheck"
)

// TestRealDiscChapterSeams plays a playlist of a real disc through the real
// stream and compares what comes out with a straight read of its clips:
// pictures and sound, at every chapter and every join between play items. It
// needs a decrypted Blu-ray (WMS_SEAM_BD) and FFmpeg (WMS_FFMPEG = its full path,
// or on PATH); it takes many minutes. WMS_SEAM_PLAYLIST picks a playlist (default:
// the main one) and WMS_SEAM_OUT receives the table.
func TestRealDiscChapterSeams(t *testing.T) {
	path := os.Getenv("WMS_SEAM_BD")
	if path == "" {
		t.Skip("set WMS_SEAM_BD to a decrypted Blu-ray image or folder")
	}
	ff, err := ffmpeg.Locate(os.Getenv("WMS_FFMPEG"), "")
	if err != nil {
		t.Skip(err)
	}
	ctx := context.Background()
	tmp := t.TempDir()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := hls.NewManager(ff, filepath.Join(tmp, "hls"), 2, log)
	m := NewManager(h, ff, filepath.Join(tmp, "cache"), "/api", log)
	s, err := m.Open(1, path, Prefs{MaxHeight: 144})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	pl := s.disc.MainPlaylist()
	if v, err := strconv.Atoi(os.Getenv("WMS_SEAM_PLAYLIST")); err == nil {
		pl = s.disc.Playlist(v)
	}
	if pl == nil {
		t.Fatal("no such playlist")
	}
	fps := frameRate(pl.Items[0].Video[0].Rate)
	laid := s.layout(pl, -1)
	it := &pl.Items[0]
	ids := []int{int(it.Video[0].PID)}
	audioMap := ""
	if len(it.Audio) > 0 {
		ids, audioMap = append(ids, int(it.Audio[0].PID)), "0:a:0"
	}

	// The straight read: each item from its first entry point to the end of its
	// clip, stopping at the item's end time. Nothing is cut at a seam here, and what
	// lies before the item's start is only taken off after decoding.
	t.Logf("playlist %d: %.1f s, %d items, %d chapters, %.3f fps", pl.Number, laid.duration, len(pl.Items), len(laid.chapters), fps)
	ref := &seamcheck.Pictures{FPS: fps}
	var refSound []int16
	for i, pi := range laid.items {
		c := s.disc.Clip(pi.clip)
		seg, err := s.clipSegment(pi.clip, pi.spn[0], c.Packets, 0)
		if err != nil {
			t.Fatal(err)
		}
		list, err := ffmpeg.WriteConcat(tmp, fmt.Sprintf("ref%d", i), []ffmpeg.Segment{seg}, ids...)
		if err != nil {
			t.Fatal(err)
		}
		in := append(ffmpeg.ConcatInputArgs(list), "-t", strconv.FormatFloat(pi.preroll+pi.duration, 'f', 3, 64))
		run, sound, err := seamcheck.Reference(ctx, ff, [][]string{in}, fps, s.videoFilter(it), audioMap, tmp)
		if err != nil {
			t.Fatal(err)
		}
		skip := min(int(pi.preroll*fps+0.5), len(run.Frames))
		ref.Frames = append(ref.Frames, run.Frames[skip:]...)
		refSound = append(refSound, sound[min(int(pi.preroll*seamcheck.AudioRate), len(sound)):]...)
	}
	t.Logf("reference: %d frames, %d s of sound", len(ref.Frames), len(refSound)/seamcheck.AudioRate)
	s.nav.SetAudio(1)
	p := s.show(ctx, s.nav.PlayPlaylist(pl, 0, 0))
	if p.Media == nil || p.Media.Type != "hls" {
		t.Fatalf("not a stream: %+v", p)
	}
	id := strings.TrimSuffix(strings.TrimPrefix(p.Media.URL, "/api/hls/"), "/playlist.m3u8")
	joined := filepath.Join(tmp, "out.ts")
	if err := seamcheck.Join(ctx, h.Get(id), joined, os.Getenv("WMS_SEAM_PACED") != ""); err != nil {
		t.Fatal(err)
	}
	out, outSound, err := seamcheck.Decode(ctx, ff, []string{"-i", joined}, fps, "", "0:a:0", tmp)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("output: %d frames, %d s of sound", len(out.Frames), len(outSound)/seamcheck.AudioRate)

	var at []float64
	var labels []string
	for i, c := range laid.chapters {
		if c > 0 {
			at, labels = append(at, c), append(labels, fmt.Sprintf("chapter %d", i+1))
		}
	}
	for i, pi := range laid.items {
		if i > 0 {
			at, labels = append(at, pi.start), append(labels, fmt.Sprintf("item %d", i+1))
		}
	}
	table := seamcheck.Table(seamcheck.Measure(seamcheck.Input{Ref: ref, Out: out, RefSound: refSound, OutSound: outSound, Boundaries: at, Labels: labels})) +
		seamcheck.Whole(ref, out, refSound, outSound)
	t.Log("\n" + table)
	if f := os.Getenv("WMS_SEAM_OUT"); f != "" {
		_ = os.WriteFile(f, []byte(table), 0o644)
	}
}

func frameRate(code int) float64 {
	switch code {
	case 1:
		return 24000.0 / 1001
	case 2:
		return 24
	case 3:
		return 25
	case 4:
		return 30000.0 / 1001
	case 6:
		return 50
	case 7:
		return 60000.0 / 1001
	}
	return 24000.0 / 1001
}
