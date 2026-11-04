package hls

import (
	"context"
	"io"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// With a real FFmpeg (skipped where there is none): a channel goes from a
// programme into a break of commercials unlike it and each other, and back.
// One is silent, one is two seconds long with an odd width, an odd frame rate
// and mono audio at another rate. Every run is its own stretch of the playlist,
// and every segment has a picture and sound a player can decode.
func TestLiveAcrossABreakForReal(t *testing.T) {
	if testing.Short() {
		t.Skip("plays about 15 seconds in real time")
	}
	bin := realFFmpeg(t)
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	mk := func(name string, args ...string) string {
		p := filepath.Join(dir, name)
		cmd := exec.CommandContext(ctx, bin, append(append([]string{"-hide_banner", "-loglevel", "error", "-y"}, args...), p)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("can't make %s: %v %s", name, err, out)
		}
		return p
	}
	show := mk("show.mkv", "-f", "lavfi", "-i", "testsrc2=s=640x360:r=25:d=8", "-f", "lavfi", "-i", "sine=f=440:d=8:r=48000",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "25", "-c:a", "aac")
	silent := mk("silent.mp4", "-f", "lavfi", "-i", "testsrc=s=320x240:r=30000/1001:d=3", "-c:v", "libx264", "-preset", "ultrafast")
	short := mk("short.mkv", "-f", "lavfi", "-i", "smptebars=s=427x240:r=24000/1001:d=2", "-f", "lavfi", "-i", "sine=f=880:d=2:r=44100",
		"-c:v", "mpeg4", "-c:a", "aac", "-ac", "1")

	inputs := []*LiveInput{
		{InputArgs: ffmpeg.FileInput(show), Duration: 4, AudioIndex: 0},
		{InputArgs: ffmpeg.FileInput(silent), Duration: 3, AudioIndex: -1},
		{InputArgs: ffmpeg.FileInput(short), Duration: 2, AudioIndex: 0},
		{InputArgs: ffmpeg.FileInput(show), Seek: 4, Duration: 4, AudioIndex: 0},
	}
	done := make(chan struct{})
	tools := &ffmpeg.Tools{FFmpeg: bin, Encoders: map[string]bool{"libx264": true}}
	m := NewManager(tools, t.TempDir(), 2, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer m.StopAllLive()
	var n atomic.Int32
	s, err := m.EnsureLive(LiveSpec{ID: "lib-1", SegmentSec: 2, Quality: ffmpeg.Quality{Preset: "ultrafast"}, Feed: func(ctx context.Context, _ time.Time) (*LiveInput, error) {
		i := int(n.Load())
		if i == len(inputs) {
			close(done)
			<-ctx.Done()
			return nil, ctx.Err()
		}
		n.Add(1)
		return inputs[i], nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-s.failed:
		t.Fatal(s.Err())
	case <-ctx.Done():
		t.Fatalf("still playing input %d", n.Load())
	}

	pl := string(s.Playlist(func(f string) string { return f }))
	if got := strings.Count(pl, "#EXT-X-DISCONTINUITY\n"); got != len(inputs)-1 {
		t.Errorf("%d discontinuities, want one before each run after the first:\n%s", got, pl)
	}
	s.mu.Lock()
	segs := append([]liveSeg(nil), s.segs...)
	s.mu.Unlock()
	var total float64
	for _, x := range segs {
		total += x.Dur
		// Both streams, decoded from start to end without a complaint.
		cmd := exec.CommandContext(ctx, bin, "-hide_banner", "-v", "error", "-i", filepath.Join(s.Dir, x.File), "-map", "0:v:0", "-map", "0:a:0", "-f", "null", "-")
		if out, err := cmd.CombinedOutput(); err != nil || len(out) > 0 {
			t.Errorf("%s: %v %s", x.File, err, out)
		}
	}
	// Each run plays as long as it was asked to, give or take a frame.
	if want := 4.0 + 3 + 2 + 4; total < want-0.5 || total > want+0.5 {
		t.Errorf("the playlist runs %.2f s, want %.0f:\n%s", total, want, pl)
	}
}
