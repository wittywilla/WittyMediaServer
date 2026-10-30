package hls

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

func TestRewriteGrowing(t *testing.T) {
	in := "#EXTM3U\r\n#EXT-X-VERSION:7\r\n#EXT-X-TARGETDURATION:4\r\n#EXT-X-PLAYLIST-TYPE:EVENT\r\n#EXT-X-MAP:URI=\"init.mp4\"\r\n#EXTINF:4.000,\r\nseg00000.m4s\r\n#EXTINF:3.500,\r\nseg00012.m4s\r\n"
	got := string(rewriteGrowing([]byte(in), func(n int) string {
		if n < 0 {
			return "seg/init.mp4?st=x"
		}
		return "seg/" + string(rune('a'+n)) + ".m4s?st=x"
	}))
	for _, want := range []string{
		"#EXT-X-VERSION:7\n#EXT-X-START:TIME-OFFSET=0,PRECISE=YES\n",
		"#EXT-X-TARGETDURATION:10\n",
		`#EXT-X-MAP:URI="seg/init.mp4?st=x"`,
		"seg/a.m4s?st=x\n",
		"seg/m.m4s?st=x\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "seg00000") || strings.Contains(got, "\r") {
		t.Errorf("not rewritten:\n%s", got)
	}
	if !strings.Contains(string(rewriteGrowing([]byte("#EXT-X-TARGETDURATION:17\n"), nil)), ":17\n") {
		t.Error("a longer target duration has to stay")
	}
}

func TestMasterPlaylist(t *testing.T) {
	s := &Session{Spec: Spec{FMP4: true, Variant: &Variant{BandwidthKbps: 5000, Codecs: "hvc1.2.4.L150.B0,mp4a.40.2", Width: 3840, Height: 2160, Range: "PQ", Subtitles: []Rendition{
		{Stream: 2, Name: `The "big" one`, Language: "en", Default: true},
		{Stream: 3, Name: "The 'big' one", Language: "en", Forced: true},
		{Stream: 4, Name: "Unknown"},
	}}}}
	got := string(s.MasterPlaylist("playlist.m3u8?st=x", func(i int) string { return "subs/" + string(rune('0'+i)) + ".m3u8?st=x" }))
	for _, want := range []string{
		"#EXT-X-VERSION:7\n",
		`NAME="The 'big' one",DEFAULT=YES,AUTOSELECT=YES,FORCED=NO,LANGUAGE="en",URI="subs/0.m3u8?st=x"`,
		`NAME="The 'big' one (2)",DEFAULT=NO,AUTOSELECT=YES,FORCED=YES,LANGUAGE="en",URI="subs/1.m3u8?st=x"`,
		`NAME="Unknown",DEFAULT=NO,AUTOSELECT=NO,FORCED=NO,URI="subs/2.m3u8?st=x"`,
		`BANDWIDTH=5000000,AVERAGE-BANDWIDTH=5000000,CODECS="hvc1.2.4.L150.B0,mp4a.40.2",RESOLUTION=3840x2160,VIDEO-RANGE=PQ,SUBTITLES="subs"`,
		"\nplaylist.m3u8?st=x\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in:\n%s", want, got)
		}
	}
	// Without subtitles there's no group to point at.
	plain := string((&Session{Spec: Spec{}}).MasterPlaylist("p", nil))
	if strings.Contains(plain, "SUBTITLES") || !strings.Contains(plain, "#EXT-X-VERSION:4") {
		t.Errorf("plain master:\n%s", plain)
	}
}

func TestSubtitlePlaylistCoversTheStream(t *testing.T) {
	s := &Session{Spec: Spec{Duration: 100.5, StartSec: 40}}
	got := string(s.SubtitlePlaylist("0.vtt"))
	for _, want := range []string{"#EXT-X-TARGETDURATION:61\n", "#EXTINF:60.500,\n0.vtt\n", "#EXT-X-PLAYLIST-TYPE:VOD\n", "#EXT-X-ENDLIST\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in:\n%s", want, got)
		}
	}
}

func TestTextSubtitleFilter(t *testing.T) {
	cases := map[string]string{
		`/media/film.mkv`:            `subtitles='/media/film.mkv':si=2`,
		`C:/Media/My Film.mkv`:       `subtitles='C\:/Media/My Film.mkv':si=2`,
		`/media/Don't [Look] Up.mkv`: `subtitles='/media/Don'\\\''t [Look] Up.mkv':si=2`,
	}
	for in, want := range cases {
		if got := textSubtitleFilter(in, 2, 0); got != want {
			t.Errorf("%s:\n got %s\nwant %s", in, got, want)
		}
	}
	got := textSubtitleFilter("/a.mkv", 0, 90.5)
	if !strings.HasPrefix(got, "setpts=PTS+90.500/TB,subtitles=") || !strings.HasSuffix(got, ",setpts=PTS-STARTPTS") {
		t.Errorf("after a seek: %s", got)
	}
}

func TestGrowingStreamsCannotPileUp(t *testing.T) {
	m := &Manager{root: t.TempDir(), session: map[string]*Session{}}
	now := time.Now()
	m.session["plain"] = &Session{Dir: t.TempDir(), lastUsed: now.Add(-time.Hour)}
	for i := range maxGrowing + 3 {
		id := string(rune('a' + i))
		m.session[id] = &Session{Spec: Spec{Growing: true}, Dir: t.TempDir(), lastUsed: now.Add(time.Duration(i) * time.Minute)}
	}
	m.evictGrowing(maxGrowing)
	if len(m.session) != maxGrowing+1 {
		t.Fatalf("%d sessions left", len(m.session))
	}
	if m.session["plain"] == nil {
		t.Error("ordinary transcodes aren't affected")
	}
	for _, id := range []string{"a", "b", "c"} {
		if m.session[id] != nil {
			t.Errorf("%s, among the longest unused, should have gone", id)
		}
	}
}

// With a real FFmpeg (skipped where there is none): a remux in each kind of
// segment gives a playlist and segments a player can use, and leaves nothing
// outside the stream's folder.
func TestRemuxForReal(t *testing.T) {
	bin := os.Getenv("WMS_FFMPEG")
	if bin == "" {
		var err error
		if bin, err = exec.LookPath("ffmpeg"); err != nil {
			t.Skip("no ffmpeg here")
		}
	}
	tools := &ffmpeg.Tools{FFmpeg: bin, Encoders: map[string]bool{"libx264": true}}
	dir := t.TempDir()
	clip := filepath.Join(dir, "clip.mkv")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	gen := exec.CommandContext(ctx, bin, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=s=320x180:r=25:d=20", "-f", "lavfi", "-i", "sine=f=440:d=20",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "25", "-c:a", "aac", clip)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("can't make a clip: %v %s", err, out)
	}
	for _, fmp4 := range []bool{false, true} {
		root := filepath.Join(dir, "hls")
		m := NewManager(tools, root, 2, slog.New(slog.NewTextHandler(io.Discard, nil)))
		spec := Spec{ID: "t", InputArgs: ffmpeg.FileInput(clip), Duration: 20, AudioIndex: 0, SubtitleIndex: -1, CopyVideo: true, CopyAudio: true, Growing: true, FMP4: fmp4}
		s := m.Ensure(spec)
		pl, err := s.GrowingPlaylist(ctx, func(n int) string { return "seg/" + strings.Repeat("x", n+1) })
		if err != nil {
			t.Fatalf("fmp4=%v: %v", fmp4, err)
		}
		if !strings.Contains(string(pl), "#EXT-X-PLAYLIST-TYPE:EVENT") || !strings.Contains(string(pl), "#EXT-X-START") {
			t.Errorf("fmp4=%v playlist:\n%s", fmp4, pl)
		}
		seg, err := s.Segment(ctx, 0)
		if err != nil || !strings.HasSuffix(seg, s.SegmentExt()) {
			t.Errorf("fmp4=%v segment: %q %v", fmp4, seg, err)
		}
		if fmp4 {
			if !strings.Contains(string(pl), `#EXT-X-MAP:URI="seg/"`) {
				t.Errorf("no map:\n%s", pl)
			}
			if _, err := s.InitSegment(ctx); err != nil {
				t.Errorf("init segment: %v", err)
			}
		}
		m.StopAll()
	}
	if _, err := os.Stat("init.mp4"); err == nil {
		os.Remove("init.mp4")
		t.Error("FFmpeg wrote the init segment outside the stream's folder")
	}
}
