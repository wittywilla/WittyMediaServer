package hls

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
		`/media/film.mkv`:            `subtitles=filename='/media/film.mkv':si=2`,
		`C:/Media/My Film.mkv`:       `subtitles=filename='C\:/Media/My Film.mkv':si=2`,
		`/media/Don't [Look] Up.mkv`: `subtitles=filename='/media/Don'\\\''t [Look] Up.mkv':si=2`,
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

// File names reach a filter graph when subtitles are burned in, and a library's file names are
// anyone's. Whatever a name holds, it stays a name: FFmpeg opens that file and adds no filter.
func TestSubtitleFilterHoldsAnyName(t *testing.T) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil || runtime.GOOS == "windows" {
		t.Skip("needs FFmpeg, and file names Windows doesn't allow")
	}
	dir := t.TempDir()
	srt := filepath.Join(dir, "base.srt")
	if err := os.WriteFile(srt, []byte("1\n00:00:00,000 --> 00:00:01,000\nhello\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, "base.mkv")
	gen := exec.Command(bin, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc=s=64x64:r=5:d=1", "-i", srt, "-c:v", "mpeg2video", "-c:s", "srt", base)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("can't make a sample: %v %s", err, out)
	}
	if out, err := exec.Command(bin, "-hide_banner", "-loglevel", "error", "-i", base, "-vf", textSubtitleFilter(base, 0, 0), "-frames:v", "1", "-f", "null", "-").CombinedOutput(); err != nil {
		t.Skipf("this FFmpeg has no subtitles filter: %v %s", err, out)
	}
	b, err := os.ReadFile(base)
	if err != nil {
		t.Fatal(err)
	}
	mark := filepath.Join(dir, "written.png")
	for _, name := range []string{
		"a:b.mkv", "a'b.mkv", `a\b.mkv`, "a,b.mkv", "a[b].mkv", "a;b.mkv", "a=b.mkv", "filename=x.mkv", "si=3.mkv", "%d.mkv", "-f.mkv",
		"x',drawbox=c=red,'.mkv", "x'[y];movie=base.srt[z].mkv", `x\',drawtext=textfile=base.srt,'.mkv`, "x':si=9,drawbox,'.mkv", "x\\\\':si=9.mkv",
	} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		// With a showinfo-free graph, an added filter or a broken quote shows as a failure, or as more
		// lines from a filter the name brought in.
		cmd := exec.Command(bin, "-hide_banner", "-loglevel", "verbose", "-i", p, "-vf", textSubtitleFilter(p, 0, 0), "-frames:v", "1", "-f", "null", "-")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("%s: %v\n%s", name, err, tail(string(out), 400))
			continue
		}
		for _, sign := range []string{"Parsed_drawbox", "Parsed_movie", "Parsed_drawtext"} {
			if strings.Contains(string(out), sign) {
				t.Errorf("%s brought in a filter: %s", name, sign)
			}
		}
		_ = os.Remove(p)
	}
	if _, err := os.Stat(mark); err == nil {
		t.Error("a name wrote a file")
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

// realFFmpeg finds an FFmpeg to test with; tests that need one are skipped
// where there is none.
func realFFmpeg(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("WMS_FFMPEG")
	if bin == "" {
		var err error
		if bin, err = exec.LookPath("ffmpeg"); err != nil {
			t.Skip("no ffmpeg here")
		}
	}
	return bin
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// logLine waits for a line of the log holding all of want. FFmpeg's ending is
// logged just after the stream hears of it.
func logLine(t *testing.T, out *lockedBuffer, want ...string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		for _, line := range strings.Split(out.String(), "\n") {
			all := line != ""
			for _, w := range want {
				all = all && strings.Contains(line, w)
			}
			if all {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no line with %q in:\n%s", want, out.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func capturingManager(t *testing.T, bin string) (*Manager, *lockedBuffer) {
	t.Helper()
	var out lockedBuffer
	log := slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))
	tools := &ffmpeg.Tools{FFmpeg: bin, Encoders: map[string]bool{"libx264": true}}
	// Not t.TempDir: FFmpeg may need a moment to let go of its files.
	root, err := os.MkdirTemp("", "wms-hls")
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{tools: tools, root: root, log: log, session: map[string]*Session{}, max: 2}
	t.Cleanup(func() {
		m.StopAll()
		m.StopAllLive()
		time.AfterFunc(time.Second, func() { _ = os.RemoveAll(root) })
	})
	return m, &out
}

func damagedFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "damaged.mkv")
	if err := os.WriteFile(p, bytes.Repeat([]byte("not a video "), 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// A damaged file: what FFmpeg said and how it ended are in the log, under the
// session of whoever is watching (here a disc's), not the stream's own ID.
func TestFailureIsTiedToTheSession(t *testing.T) {
	m, out := capturingManager(t, realFFmpeg(t))
	s := m.Ensure(Spec{ID: "dvd-abc-t1", LogSession: "dcafe", InputArgs: ffmpeg.FileInput(damagedFile(t)), Duration: 60, AudioIndex: -1, SubtitleIndex: -1, Preset: "ultrafast"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := s.Segment(ctx, 0); err == nil {
		t.Fatal("a damaged file made a segment")
	}
	logLine(t, out, "level=WARN", `msg="process ended"`, "cat=transcode", "session=dcafe", "process=ffmpeg", "reason=", "stderr=", "args=")
	logLine(t, out, "level=INFO", `msg="transcode started"`, "session=dcafe")
	if strings.Contains(out.String(), "session=dvd-abc") {
		t.Errorf("logged under the stream's own ID:\n%s", out.String())
	}
}

// A stream with no session of its own logs under its ID. Stopping it on purpose
// says nothing is wrong, and the next disc session to use it takes it over.
// A stopped stream's folder is deleted after its FFmpeg goes, which takes a while. By
// then the viewer may have opened the same film again, under the same ID.
func TestAStoppedStreamCantDeleteItsSuccessorsFiles(t *testing.T) {
	root := t.TempDir()
	m := NewManager(&ffmpeg.Tools{}, root, 2, slog.New(slog.NewTextHandler(io.Discard, nil)))
	spec := Spec{ID: "v-film-a0", SubtitleIndex: -1, Duration: 20}
	old, _ := m.ensure(spec)
	m.mu.Lock()
	delete(m.session, spec.ID) // what Stop does first
	m.mu.Unlock()
	next, created := m.ensure(spec)
	if !created || next.Dir == old.Dir {
		t.Fatalf("both use %s", next.Dir)
	}
	if !sessionDirRe.MatchString(filepath.Base(next.Dir)) {
		t.Errorf("%s wouldn't be cleared when the server next starts", next.Dir)
	}
	seg := filepath.Join(next.Dir, "seg00000.ts")
	if err := os.WriteFile(seg, []byte("a segment"), 0o644); err != nil {
		t.Fatal(err)
	}
	old.stopAndClean()
	if _, err := os.Stat(seg); err != nil {
		t.Errorf("the new stream lost a segment: %v", err)
	}
}

func TestStoppingOnPurposeIsNoFailure(t *testing.T) {
	m, out := capturingManager(t, realFFmpeg(t))
	s := m.Ensure(Spec{ID: "v-running", InputArgs: []string{"-f", "lavfi", "-i", "testsrc2=s=160x90:r=5"}, Duration: 600, AudioIndex: -1, SubtitleIndex: -1, Preset: "ultrafast"})
	if got := s.LogSession(); got != "v-running" {
		t.Errorf("logs under %q", got)
	}
	s.Warm()
	logLine(t, out, `msg="transcode started"`, "session=v-running")
	s.TieTo("dnext")
	if got := s.LogSession(); got != "dnext" {
		t.Errorf("logs under %q", got)
	}
	m.Stop("v-running")
	logLine(t, out, "level=DEBUG", `msg="process ended"`, "session=dnext")
	if strings.Contains(out.String(), "level=WARN") {
		t.Errorf("a stop on purpose was reported:\n%s", out.String())
	}
}

// A live run that fails still tells its feed what FFmpeg said (an unknown audio
// format is found this way), and the log has the same.
func TestLiveFailureIsLoggedAndHeard(t *testing.T) {
	m, out := capturingManager(t, realFFmpeg(t))
	s := m.newLive(LiveSpec{ID: "tun-7", Quality: ffmpeg.Quality{Preset: "ultrafast"}})
	var heard string
	in := &LiveInput{InputArgs: ffmpeg.FileInput(damagedFile(t)), AudioIndex: 0, Realtime: true, OnFail: func(stderr string) { heard = stderr }}
	err := runLiveFFmpeg(context.Background(), s, in, 1, false)
	if err == nil {
		t.Fatal("a damaged file played")
	}
	if heard == "" {
		t.Error("OnFail heard nothing")
	}
	logLine(t, out, "level=WARN", `msg="process ended"`, "cat=transcode", "session=tun-7", "stderr=")
}

// A live input found to be encrypted while it plays ends the session for good, with its
// reason, and FFmpeg goes with it.
func TestLiveInputRefusedWhilePlaying(t *testing.T) {
	bin := realFFmpeg(t)
	tools := &ffmpeg.Tools{FFmpeg: bin, Encoders: map[string]bool{"libx264": true}}
	m := NewManager(tools, t.TempDir(), 2, slog.New(slog.NewTextHandler(io.Discard, nil)))
	refused := make(chan struct{})
	why := errors.New("encrypted")
	feeds := 0
	s, err := m.EnsureLive(LiveSpec{ID: "tun-1", SegmentSec: 1, Feed: func(context.Context, time.Time) (*LiveInput, error) {
		feeds++
		return &LiveInput{InputArgs: []string{"-re", "-f", "lavfi", "-i", "testsrc=s=64x64:r=5"}, AudioIndex: -1, Realtime: true,
			Refused: refused, Refusal: func() error { return why }}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := s.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	close(refused)
	select {
	case <-s.failed:
	case <-ctx.Done():
		t.Fatal("it went on playing")
	}
	if !errors.Is(s.Err(), why) || feeds != 1 {
		t.Errorf("%v, after %d runs", s.Err(), feeds)
	}
}

// With a real FFmpeg (skipped where there is none): a remux in each kind of
// segment gives a playlist and segments a player can use, and leaves nothing
// outside the stream's folder.
func TestRemuxForReal(t *testing.T) {
	bin := realFFmpeg(t)
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

func TestHoleExprs(t *testing.T) {
	holes := []Hole{{At: 0, Length: 0.5}, {At: 10, Length: 4}, {At: 20, Length: 4}}
	// From the very start, every hole is ahead, also one right at the start.
	keep, shift := holeExprs(holes, 0)
	if want := "not(between(t,0.000000,0.500000)+between(t,10.500000,14.500000)+between(t,24.500000,28.500000))"; keep != want {
		t.Errorf("from the start:\n%s\nwant\n%s", keep, want)
	}
	if want := "PTS-(gte(T,0.500000)*0.500000+gte(T,14.500000)*4.000000+gte(T,28.500000)*4.000000)/TB"; shift != want {
		t.Errorf("from the start:\n%s\nwant\n%s", shift, want)
	}
	if got := holesPassed(holes, 0); got != 0 {
		t.Errorf("%v passed at the start", got)
	}
	// Further on, the clock counts from where it starts and the holes before that
	// are skipped by reading further on.
	keep, shift = holeExprs(holes, 12)
	if keep != "not(between(t,8.000000,12.000000))" || shift != "PTS-(gte(T,12.000000)*4.000000)/TB" {
		t.Errorf("from 12 s: %s %s", keep, shift)
	}
	if got := holesPassed(holes, 12); got != 4.5 {
		t.Errorf("%v passed at 12 s", got)
	}
	if keep, shift = holeExprs(holes, 21); keep != "" || shift != "" {
		t.Errorf("past them all: %s %s", keep, shift)
	}
	if keep, shift = holeExprs(nil, 0); keep != "" || shift != "" {
		t.Errorf("no holes: %s %s", keep, shift)
	}
}

// A preview of a disc's title is a session too, and its folder goes at the next start.
func TestPreviewFoldersAreCleared(t *testing.T) {
	if !sessionDirRe.MatchString("clip-3-1") {
		t.Error("left behind for good")
	}
}

// On a machine with many processor threads, a stream's decoder and libx264 get a set number,
// placed where FFmpeg applies them to the picture, and expert options still have the last word.
func TestStreamThreads(t *testing.T) {
	tools := &ffmpeg.Tools{Encoders: map[string]bool{"libx264": true}}
	line := func(sp Spec) string {
		s := &Session{Spec: sp, Dir: t.TempDir(), tools: tools, m: &Manager{tools: tools}}
		args, _, _ := s.commandLine(0)
		return strings.Join(args, " ")
	}
	base := Spec{InputArgs: []string{"-i", "in.mkv"}, Duration: 60, SegmentSec: 4, AudioIndex: 0, SubtitleIndex: -1}
	dec, enc := ffmpeg.StreamThreads(1080)
	got := line(base)
	if dec == 0 {
		if strings.Contains(got, "-threads") {
			t.Errorf("a small machine is left to FFmpeg's choice:\n%s", got)
		}
		return
	}
	if want := fmt.Sprintf("-threads %d -i in.mkv", dec); !strings.Contains(got, want) {
		t.Errorf("the decoder's threads aren't set before the input (%q):\n%s", want, got)
	}
	if want := fmt.Sprintf("-c:v libx264 -threads %d -preset", enc); !strings.Contains(got, want) {
		t.Errorf("libx264's threads aren't set (%q):\n%s", want, got)
	}
	big := base
	big.Height = 2160
	if _, enc4k := ffmpeg.StreamThreads(2160); !strings.Contains(line(big), fmt.Sprintf("-c:v libx264 -threads %d ", enc4k)) {
		t.Errorf("4K gets more threads:\n%s", line(big))
	}
	big.MaxHeight = 720 // what comes out is small, whatever went in
	if !strings.Contains(line(big), fmt.Sprintf("-c:v libx264 -threads %d ", enc)) {
		t.Errorf("a 4K film converted to 720p gets the threads of 720p:\n%s", line(big))
	}
	expert := base
	expert.Quality.VideoArgs = []string{"-preset", "fast", "-threads", "3"}
	if got := line(expert); strings.Count(got, "-threads") != 2 || !strings.Contains(got, "-c:v libx264 -preset fast -threads 3") {
		t.Errorf("expert options that set threads are left to do it (one for the decoder, theirs for libx264):\n%s", got)
	}
	copied := base
	copied.CopyVideo, copied.Growing = true, true
	hasBurst = false
	readrateOnce.Do(func() {})
	if got := line(copied); strings.Contains(got, "-threads") {
		t.Errorf("a copy decodes no picture:\n%s", got)
	}
}
