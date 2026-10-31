package hls

import (
	"context"
	"errors"
	"fmt"
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

func testManager(t *testing.T, max int) *Manager {
	t.Helper()
	return &Manager{tools: &ffmpeg.Tools{}, root: t.TempDir(), log: slog.New(slog.NewTextHandler(io.Discard, nil)), session: map[string]*Session{}, max: max}
}

// running gives a session a job that looks like a live FFmpeg.
func running(s *Session, used time.Time) {
	s.job = &job{cmd: &exec.Cmd{}, done: make(chan struct{})}
	s.lastUsed = used
}

func TestCleanRootOnlyTakesItsOwnFolders(t *testing.T) {
	root := t.TempDir()
	mine := []string{"v-0123abcd-a0-s-1-h0-c2-aac-abc123", "dvd-0123-m-VMGM-0-1-1-a0", "bd-0123-p00001-a0-g1"}
	for _, d := range mine {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Someone may point the transcode folder at a folder that holds other things.
	theirs := []string{"Movies", "video-project"}
	for _, d := range theirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(root, "v-notes.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cleanRoot(root)
	for _, d := range mine {
		if _, err := os.Stat(filepath.Join(root, d)); err == nil {
			t.Errorf("%s was left behind", d)
		}
	}
	for _, d := range theirs {
		if _, err := os.Stat(filepath.Join(root, d)); err != nil {
			t.Errorf("%s was deleted", d)
		}
	}
	if _, err := os.Stat(file); err != nil {
		t.Error("a file of someone else's was deleted")
	}
}

func TestSessionIDsMatchCleanup(t *testing.T) {
	m := testManager(t, 4)
	s := m.Ensure(Spec{ID: "v-" + Key("/a/b.mkv") + "-a0-s-1-h0-c2-aac-abc123", Duration: 10})
	if !sessionDirRe.MatchString(filepath.Base(s.Dir)) {
		t.Fatalf("cleanRoot would not recognise %s", s.Dir)
	}
}

func TestTailBufferKeepsTheEnd(t *testing.T) {
	var b tailBuffer
	for i := 0; i < 2000; i++ {
		_, _ = b.Write([]byte(strings.Repeat("x", 1000)))
	}
	_, _ = b.Write([]byte("the last words"))
	if !strings.HasSuffix(b.String(), "the last words") {
		t.Error("the end is gone")
	}
	if len(b.b) > 2*stderrTail+1024 {
		t.Errorf("kept %d bytes", len(b.b))
	}
}

func TestLimitOnRunningTranscodes(t *testing.T) {
	m := testManager(t, 1)
	watching := m.Ensure(Spec{ID: "v-watching", Duration: 10})
	waiting := m.Ensure(Spec{ID: "v-waiting", Duration: 10})
	running(watching, time.Now())
	if err := m.makeRoom(waiting); !errors.Is(err, ErrBusy) {
		t.Fatalf("over the limit with everyone watching: %v", err)
	}
	if watching.job == nil {
		t.Fatal("a stream in use was stopped")
	}
	// The stream was left alone for a while (paused): it makes way.
	watching.lastUsed = time.Now().Add(-2 * yieldAfter)
	if err := m.makeRoom(waiting); err != nil {
		t.Fatal(err)
	}
	if watching.job != nil {
		t.Fatal("the idle stream is still running")
	}
	// Its own job doesn't count against a session.
	running(waiting, time.Now())
	if err := m.makeRoom(waiting); err != nil {
		t.Fatal(err)
	}
}

func TestLeaveKeepsAStreamOthersWatch(t *testing.T) {
	m := testManager(t, 4)
	s := m.Ensure(Spec{ID: "v-shared", Duration: 10})
	s.See("phone")
	s.See("tv")
	m.Leave("v-shared", "phone")
	if m.Get("v-shared") == nil {
		t.Fatal("one viewer leaving ended the stream for the other")
	}
	m.Leave("v-shared", "tv")
	if m.Get("v-shared") != nil {
		t.Fatal("the last viewer left and the stream goes on")
	}
}

// A film paused for a long time plays on: its segments are cleaned up, but
// the stream still exists for the viewer's next request.
func TestSweepKeepsIdleSessionsPlayable(t *testing.T) {
	m := testManager(t, 4)
	s := m.Ensure(Spec{ID: "v-paused", Duration: 10})
	seg := s.segPath(0)
	if err := os.WriteFile(seg, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	running(s, time.Now().Add(-pauseAfter-time.Second))
	m.sweepOnce()
	if s.job != nil {
		t.Fatal("not paused")
	}
	if !fileReady(seg) {
		t.Fatal("segments of a paused stream are gone")
	}
	s.lastUsed = time.Now().Add(-cleanAfter - time.Second)
	m.sweepOnce()
	if fileReady(seg) {
		t.Fatal("the segments of an idle stream stay on disk")
	}
	if m.Get("v-paused") == nil {
		t.Fatal("the stream is gone")
	}
	s.lastUsed = time.Now().Add(-forgetAfter - time.Second)
	m.sweepOnce()
	if m.Get("v-paused") != nil {
		t.Fatal("never forgotten")
	}
}

func TestPruneKeepsWhatIsNear(t *testing.T) {
	m := testManager(t, 4)
	s := m.Ensure(Spec{ID: "v-long", Duration: 4000})
	for _, n := range []int{0, 10, 349, 350, 500} {
		if err := os.WriteFile(s.segPath(n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s.prune(500)
	for n, want := range map[int]bool{0: false, 10: false, 349: false, 350: true, 500: true} {
		if fileReady(s.segPath(n)) != want {
			t.Errorf("segment %d: want present=%v", n, want)
		}
	}
}

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

func TestSessionsAreBounded(t *testing.T) {
	m := testManager(t, 2)
	for i := 0; i < maxSessions+50; i++ {
		m.Ensure(Spec{ID: fmt.Sprint("s", i), Duration: 10})
	}
	if len(m.session) > maxSessions {
		t.Fatalf("%d sessions kept", len(m.session))
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

func TestHoleFilter(t *testing.T) {
	holes := []Hole{{At: 0, Length: 0.5}, {At: 10, Length: 4}, {At: 20, Length: 4}}
	// From the very start, every hole is ahead, also one right at the start.
	want := "select='not(between(t,0.000000,0.500000)+between(t,10.500000,14.500000)+between(t,24.500000,28.500000))'," +
		"setpts='PTS-(gte(T,0.500000)*0.500000+gte(T,14.500000)*4.000000+gte(T,28.500000)*4.000000)/TB'"
	if got := holeFilter(holes, 0, "select", "setpts"); got != want {
		t.Errorf("from the start:\n%s\nwant\n%s", got, want)
	}
	if got := holesPassed(holes, 0); got != 0 {
		t.Errorf("%v passed at the start", got)
	}
	// Further on, the clock counts from where it starts and the holes before that
	// are skipped by reading further on.
	want = "aselect='not(between(t,8.000000,12.000000))',asetpts='PTS-(gte(T,12.000000)*4.000000)/TB'"
	if got := holeFilter(holes, 12, "aselect", "asetpts"); got != want {
		t.Errorf("from 12 s:\n%s\nwant\n%s", got, want)
	}
	if got := holesPassed(holes, 12); got != 4.5 {
		t.Errorf("%v passed at 12 s", got)
	}
	if got := holeFilter(holes, 21, "select", "setpts"); got != "" {
		t.Errorf("past them all: %s", got)
	}
	if got := holeFilter(nil, 0, "select", "setpts"); got != "" {
		t.Errorf("no holes: %s", got)
	}
}
