package seamcheck

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittydvd"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
)

// These tests make a disc-like stream with FFmpeg, cut it where a disc cuts it,
// and play it through the real stream code. Nothing copyrighted is needed, only
// FFmpeg (WMS_FFMPEG, or on PATH); they are skipped without it.

const (
	testFPS = 25.0
	testSec = 36
)

func tools(t *testing.T) *ffmpeg.Tools {
	t.Helper()
	ff, err := ffmpeg.Locate(os.Getenv("WMS_FFMPEG"), "")
	if err != nil {
		t.Skip(err)
	}
	if !ff.Encoders["libx264"] || !ff.Encoders["mpeg2video"] || !ff.Encoders["ac3"] {
		t.Skip("this FFmpeg lacks the encoders for the test stream")
	}
	return ff
}

func run(t *testing.T, ff *ffmpeg.Tools, args ...string) {
	t.Helper()
	cmd := ffmpeg.Command(context.Background(), ff.FFmpeg, append([]string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg %v: %s", err, out)
	}
}

// picture and sound of the test film: every frame and moment is unlike the others.
func source(sec int) []string {
	return []string{
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=320x180:rate=%d:duration=%d", int(testFPS), sec),
		"-f", "lavfi", "-i", fmt.Sprintf("anoisesrc=d=%d:c=pink:r=48000:s=7:a=0.5,aformat=channel_layouts=stereo", sec),
	}
}

// makeVOB writes an MPEG program stream as a DVD holds one: 2048-byte packs,
// sound stored ahead of its picture, a sequence header at each group of pictures.
func makeVOB(t *testing.T, ff *ffmpeg.Tools, dir string) string {
	p := filepath.Join(dir, "film.vob")
	run(t, ff, append(source(testSec), "-c:v", "mpeg2video", "-flags", "+cgop", "-sc_threshold", "1000000000", "-g", "12", "-bf", "2", "-b:v", "1500k", "-maxrate", "4M", "-bufsize", "1800k",
		"-c:a", "ac3", "-b:a", "192k", "-packetsize", "2048", "-f", "vob", p)...)
	return p
}

// cutPoints finds the packs where a group of pictures starts, nearest to each
// of the given times: the places a disc may start a cell.
func cutPoints(t *testing.T, ff *ffmpeg.Tools, path string, at []float64) []int64 {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cuts []int64
	var starts []int64
	for off := 0; off+2048 <= len(b); off += 2048 {
		if bytes.Contains(b[off:off+2048], []byte{0, 0, 1, 0xb3}) {
			starts = append(starts, int64(off))
		}
	}
	if len(starts) < 4 {
		t.Fatal("no groups of pictures in the test stream")
	}
	size := float64(len(b))
	for _, sec := range at {
		// Near enough: the bit rate is even, and the exact place doesn't matter.
		want := int64(size * sec / testSec)
		best := starts[0]
		for _, s := range starts {
			if math.Abs(float64(s-want)) < math.Abs(float64(best-want)) {
				best = s
			}
		}
		cuts = append(cuts, best)
	}
	return cuts
}

func firstPTS(t *testing.T, ff *ffmpeg.Tools, url string) (first, fileStart float64) {
	t.Helper()
	out, err := ffmpeg.Command(context.Background(), ff.FFprobe, "-v", "error", "-protocol_whitelist", "file,subfile",
		"-select_streams", "v:0", "-show_entries", "stream=start_time:format=start_time", "-of", "csv=p=0", url).Output()
	if err != nil {
		t.Fatal(err)
	}
	f := strings.Fields(strings.ReplaceAll(string(out), ",", " "))
	if len(f) < 2 {
		t.Fatalf("no times in %q", out)
	}
	first, _ = strconv.ParseFloat(f[0], 64)
	fileStart, _ = strconv.ParseFloat(f[len(f)-1], 64)
	return first, fileStart
}

type testCell struct {
	seg    ffmpeg.Segment
	start  float64 // of its first picture, from the start of the film
	lateBy float64 // how far its sound lies ahead of its first picture
}

// cutCells cuts a program stream into cells at those offsets.
func cutCells(t *testing.T, ff *ffmpeg.Tools, path string, cuts []int64, frames int) []testCell {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	var cells []testCell
	bounds := append(append([]int64{0}, cuts...), st.Size())
	var t0 float64
	for i := 0; i+1 < len(bounds); i++ {
		e := wittydvd.FileExtent{File: path, Offset: bounds[i], Length: bounds[i+1] - bounds[i]}
		first, fileStart := firstPTS(t, ff, ffmpeg.SubfileURL(e))
		if i == 0 {
			t0 = first
		}
		cells = append(cells, testCell{seg: ffmpeg.Segment{Extents: []wittydvd.FileExtent{e}, Lead: first - fileStart}, start: first - t0, lateBy: first - fileStart})
	}
	for i := range cells {
		end := float64(frames) / testFPS
		if i+1 < len(cells) {
			end = cells[i+1].start
		}
		cells[i].seg.Duration = end - cells[i].start
	}
	return cells
}

func straightRead(path string) []string { return []string{"-i", path} }

// playThrough runs segments through the real stream code and returns what comes out.
func playThrough(t *testing.T, ff *ffmpeg.Tools, dir, name string, segs []ffmpeg.Segment, ids []int, duration float64, tune func(*hls.Spec)) (*Pictures, []int16, *hls.Session) {
	t.Helper()
	ctx := context.Background()
	list, err := ffmpeg.WriteConcat(dir, name, segs, ids...)
	if err != nil {
		t.Fatal(err)
	}
	m := hls.NewManager(ff, filepath.Join(dir, name+"-hls"), 2, slog.New(slog.NewTextHandler(io.Discard, nil)))
	spec := hls.Spec{ID: "v-" + name, InputArgs: ffmpeg.ConcatInputArgs(list), Duration: duration, AudioIndex: 0, AudioChannels: 2, SubtitleIndex: -1, Preset: "ultrafast"}
	if tune != nil {
		tune(&spec)
	}
	h := m.Ensure(spec)
	t.Cleanup(m.StopAll)
	joined := filepath.Join(dir, name+".ts")
	if err := Join(ctx, h, joined, false); err != nil {
		t.Fatal(err)
	}
	out, sound, err := Decode(ctx, ff, []string{"-i", joined}, testFPS, "", "0:a:0", dir)
	if err != nil {
		t.Fatal(err)
	}
	return out, sound, h
}

// noSeams says what a join has to be: nothing lost or moved, picture or sound.
func noSeams(t *testing.T, rows []Boundary) {
	t.Helper()
	for _, r := range rows {
		if r.Lost != 0 || r.Repeated != 0 {
			t.Errorf("%s: %d frames lost, %d repeated", r.Label, r.Lost, r.Repeated)
		}
		if math.Abs(r.ShiftMs) > 1 || math.Abs(r.MarkMs) > 1000/testFPS {
			t.Errorf("%s: picture moved %.0f ms, the chapter shows %.0f ms off", r.Label, r.ShiftMs, r.MarkMs)
		}
		if r.SoundConfidence > 0.5 && math.Abs(r.SoundJumpMs) > 5 {
			t.Errorf("%s: sound jumps %.1f ms", r.Label, r.SoundJumpMs)
		}
	}
}

func boundaries(cells []testCell) (at []float64, labels []string) {
	for i, c := range cells[1:] {
		at, labels = append(at, c.start), append(labels, fmt.Sprintf("cell %d", i+2))
	}
	return
}

// DVD: cells of one recording, cut where the sound stored ahead of the first
// picture makes them start early, so measuring from the earliest packet moves the
// picture. The old way is checked too: if it shows nothing wrong, neither would
// the new way, and this test would prove nothing.
func TestCellSeams(t *testing.T) {
	ff := tools(t)
	dir := t.TempDir()
	vob := makeVOB(t, ff, dir)
	cuts := cutPoints(t, ff, vob, []float64{8, 16, 24})
	ctx := context.Background()
	ref, refSound, err := Reference(ctx, ff, [][]string{straightRead(vob)}, testFPS, "", "0:a:0", dir)
	if err != nil {
		t.Fatal(err)
	}
	cells := cutCells(t, ff, vob, cuts, len(ref.Frames))
	var lead float64
	for _, c := range cells[1:] {
		lead += c.lateBy
	}
	if lead == 0 {
		t.Fatal("the test stream has no sound ahead of its pictures: it can't show this bug")
	}
	at, labels := boundaries(cells)
	duration := float64(len(ref.Frames)) / testFPS
	ids := []int{0x1e0, 0x80}

	// The join as it is now: placed by their first pictures.
	var segs []ffmpeg.Segment
	for _, c := range cells {
		segs = append(segs, c.seg)
	}
	out, sound, _ := playThrough(t, ff, dir, "anchored", segs, ids, duration, nil)
	rows := Measure(Input{Ref: ref, Out: out, RefSound: refSound, OutSound: sound, Boundaries: at, Labels: labels})
	t.Log("placed by the first picture:\n" + Table(rows))
	noSeams(t, rows)

	// The same cells placed by their earliest packet, as they used to be.
	for i := range segs {
		segs[i].Lead = 0
	}
	old, oldSound, _ := playThrough(t, ff, dir, "unanchored", segs, ids, duration, nil)
	before := Measure(Input{Ref: ref, Out: old, RefSound: refSound, OutSound: oldSound, Boundaries: at, Labels: labels})
	t.Log("placed by the earliest packet:\n" + Table(before))
	moved := false
	for _, r := range before {
		moved = moved || r.MarkMs > 1000/testFPS
	}
	if !moved {
		t.Error("the old way shows no late picture: this test can't see the bug any more")
	}
}

// A recording in the library: the probe asks for the margin, and a restart
// anywhere in the file goes on with the right pictures.
func TestRestartInARecordingIsExact(t *testing.T) {
	ff := tools(t)
	dir := t.TempDir()
	clip := filepath.Join(dir, "recording.ts")
	run(t, ff, append(source(testSec), "-c:v", "libx264", "-preset", "veryfast", "-g", "48", "-keyint_min", "48", "-sc_threshold", "0", "-bf", "3", "-b:v", "1500k",
		"-c:a", "ac3", "-b:a", "192k", "-f", "mpegts", clip)...)
	ctx := context.Background()
	pr, err := ff.ProbeFile(ctx, clip)
	if err != nil {
		t.Fatal(err)
	}
	if pr.SeekMargin() == 0 {
		t.Fatalf("no margin for %q", pr.Format.FormatName)
	}
	ref, _, err := Reference(ctx, ff, [][]string{{"-i", clip}}, testFPS, "", "", dir)
	if err != nil {
		t.Fatal(err)
	}
	m := hls.NewManager(ff, filepath.Join(dir, "hls"), 2, slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(m.StopAll)
	for _, n := range []int{1, 3, 5} {
		m.Stop("v-rec")
		h := m.Ensure(hls.Spec{ID: "v-rec", InputArgs: ffmpeg.FileInput(clip), Duration: pr.Duration(), SeekMargin: pr.SeekMargin(), AudioIndex: 0, AudioChannels: 2, SubtitleIndex: -1, Preset: "veryfast"})
		p, err := h.Segment(ctx, n)
		if err != nil {
			t.Fatal(err)
		}
		got, _, err := Decode(ctx, ff, []string{"-i", p}, testFPS, "", "", dir)
		if err != nil {
			t.Fatal(err)
		}
		from := n * 4 * int(testFPS)
		if len(got.Frames) != 4*int(testFPS) {
			t.Errorf("segment %d: %d frames", n, len(got.Frames))
		}
		for i := range got.Frames {
			if d := diff(&got.Frames[i], &ref.Frames[from+i]); d > thumbSize*3 {
				t.Errorf("segment %d: frame %d isn't the film's frame %d (difference %d)", n, i, from+i, d)
				break
			}
		}
	}
}

// A straight read that starts mid-film has its sound ahead of its first picture.
// The reference has to allow for that, or sound looks out of step when it isn't.
func TestFirstFrameSecCountsEarlySound(t *testing.T) {
	ff := tools(t)
	dir := t.TempDir()
	vob := makeVOB(t, ff, dir)
	cells := cutCells(t, ff, vob, cutPoints(t, ff, vob, []float64{8}), int(testSec*testFPS))
	c := cells[1]
	if c.lateBy == 0 {
		t.Fatal("the test stream has no sound ahead of its pictures: it can't show this")
	}
	list, err := ffmpeg.WriteConcat(dir, "cell", []ffmpeg.Segment{{Extents: c.seg.Extents}}, 0x1e0, 0x80)
	if err != nil {
		t.Fatal(err)
	}
	got := FirstFrameSec(context.Background(), ff, ffmpeg.ConcatInputArgs(list))
	if math.Abs(got-c.lateBy) > 1/testFPS {
		t.Errorf("first picture at %.3f s, want %.3f", got, c.lateBy)
	}
}

// Cells that follow on read as one input have no join at all.
func TestOneReadHasNoSeams(t *testing.T) {
	ff := tools(t)
	dir := t.TempDir()
	vob := makeVOB(t, ff, dir)
	ctx := context.Background()
	ref, refSound, err := Reference(ctx, ff, [][]string{straightRead(vob)}, testFPS, "", "0:a:0", dir)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(vob)
	e := wittydvd.FileExtent{File: vob, Length: st.Size()}
	duration := float64(len(ref.Frames)) / testFPS
	out, sound, _ := playThrough(t, ff, dir, "one", []ffmpeg.Segment{{Extents: []wittydvd.FileExtent{e}, Duration: duration}}, []int{0x1e0, 0x80}, duration, nil)
	at := []float64{4, 8, 12, 16, 20, 24, 28, 32}
	rows := Measure(Input{Ref: ref, Out: out, RefSound: refSound, OutSound: sound, Boundaries: at})
	noSeams(t, rows)
}

// A seek lands on a segment by itself: FFmpeg starts there with -ss. Around a
// join, it must show the same pictures at the same times as playing up to it.
func TestSeekNearACellSeam(t *testing.T) {
	ff := tools(t)
	dir := t.TempDir()
	vob := makeVOB(t, ff, dir)
	cuts := cutPoints(t, ff, vob, []float64{9, 17, 25})
	ctx := context.Background()
	ref, _, err := Reference(ctx, ff, [][]string{straightRead(vob)}, testFPS, "", "", dir)
	if err != nil {
		t.Fatal(err)
	}
	cells := cutCells(t, ff, vob, cuts, len(ref.Frames))
	var segs []ffmpeg.Segment
	for _, c := range cells {
		segs = append(segs, c.seg)
	}
	duration := float64(len(ref.Frames)) / testFPS
	list, err := ffmpeg.WriteConcat(dir, "seek", segs, 0x1e0, 0x80)
	if err != nil {
		t.Fatal(err)
	}
	m := hls.NewManager(ff, filepath.Join(dir, "seek-hls"), 2, slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(m.StopAll)
	h := m.Ensure(hls.Spec{ID: "v-seek", InputArgs: ffmpeg.ConcatInputArgs(list), Duration: duration, AudioIndex: 0, AudioChannels: 2, SubtitleIndex: -1, Preset: "ultrafast"})
	for _, c := range cells[1:] {
		n := int(c.start / 4) // the segment holding the join: a fresh FFmpeg each time
		m.Stop("v-seek")
		h = m.Ensure(hls.Spec{ID: "v-seek", InputArgs: ffmpeg.ConcatInputArgs(list), Duration: duration, AudioIndex: 0, AudioChannels: 2, SubtitleIndex: -1, Preset: "ultrafast"})
		joined := filepath.Join(dir, fmt.Sprintf("seek%d.ts", n))
		f, err := os.Create(joined)
		if err != nil {
			t.Fatal(err)
		}
		for k := n; k < n+2 && k < h.SegmentCount(); k++ {
			p, err := h.Segment(ctx, k)
			if err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.Write(b); err != nil {
				t.Fatal(err)
			}
		}
		f.Close()
		got, _, err := Decode(ctx, ff, []string{"-i", joined}, testFPS, "", "", dir)
		if err != nil {
			t.Fatal(err)
		}
		from := n * 4 * int(testFPS)
		bad := 0
		for i := range got.Frames {
			if from+i < len(ref.Frames) && diff(&got.Frames[i], &ref.Frames[from+i]) > thumbSize*3 {
				bad++
			}
		}
		if bad > 0 {
			t.Errorf("seek to segment %d (join at %.2f s): %d of %d frames aren't the right ones", n, c.start, bad, len(got.Frames))
		}
	}
}

// A copied stream goes through FFmpeg's own cutting, with no joins of ours: all
// the pictures come through, also when it starts just before a chapter.
func TestCopyStreamKeepsEveryFrame(t *testing.T) {
	ff := tools(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "film.mkv")
	run(t, ff, append(source(testSec), "-c:v", "libx264", "-preset", "ultrafast", "-g", "48", "-bf", "2", "-c:a", "aac", src)...)
	ctx := context.Background()
	ref, _, err := Decode(ctx, ff, []string{"-i", src}, testFPS, "", "", dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, start := range []float64{0, 11.5} {
		m := hls.NewManager(ff, filepath.Join(dir, fmt.Sprintf("copy-hls-%g", start)), 2, slog.New(slog.NewTextHandler(io.Discard, nil)))
		t.Cleanup(m.StopAll)
		h := m.Ensure(hls.Spec{ID: fmt.Sprintf("v-copy-%d", int(start)), InputArgs: ffmpeg.FileInput(src), Duration: testSec, AudioIndex: 0, SubtitleIndex: -1, SeekMargin: 3, CopyVideo: true, CopyAudio: true, Growing: true, StartSec: start, SegmentSec: 4})
		if _, err := h.GrowingPlaylist(ctx, func(n int) string { return fmt.Sprintf("seg%05d.ts", n) }); err != nil {
			t.Fatal(err)
		}
		for h.Active() { // a copy is quick; wait for all of it
			time.Sleep(50 * time.Millisecond)
		}
		b, err := os.ReadFile(filepath.Join(h.Dir, "ffmpeg-0.m3u8"))
		if err != nil {
			t.Fatal(err)
		}
		joined := filepath.Join(dir, fmt.Sprintf("copy-%g.ts", start))
		f, err := os.Create(joined)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			seg, err := os.ReadFile(filepath.Join(h.Dir, line))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.Write(seg); err != nil {
				t.Fatal(err)
			}
		}
		f.Close()
		got, _, err := Decode(ctx, ff, []string{"-i", joined}, testFPS, "", "", dir)
		if err != nil {
			t.Fatal(err)
		}
		// A copy starts at the keyframe before the start it was asked for.
		key := int(start*testFPS) / 48 * 48
		if want := len(ref.Frames) - key; len(got.Frames) != want {
			t.Errorf("from %g s: %d frames, want %d", start, len(got.Frames), want)
		}
		for i := range got.Frames {
			if key+i < len(ref.Frames) && diff(&got.Frames[i], &ref.Frames[key+i]) > thumbSize {
				t.Errorf("from %g s: frame %d is not the film's frame %d", start, i, key+i)
				break
			}
		}
	}
}

// Two runs of FFmpeg must agree: a restart (the server rested while the viewer
// caught up, or someone seeked) goes on with the same pictures at the same times.
// x264's first decode times are below zero, which used to move the first run.
func TestRestartsAgreeWithTheFirstRun(t *testing.T) {
	ff := tools(t)
	dir := t.TempDir()
	vob := makeVOB(t, ff, dir)
	st, _ := os.Stat(vob)
	e := wittydvd.FileExtent{File: vob, Length: st.Size()}
	list, err := ffmpeg.WriteConcat(dir, "restart", []ffmpeg.Segment{{Extents: []wittydvd.FileExtent{e}, Duration: testSec}}, 0x1e0, 0x80)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	m := hls.NewManager(ff, filepath.Join(dir, "hls"), 2, slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(m.StopAll)
	spec := hls.Spec{ID: "v-restart", InputArgs: ffmpeg.ConcatInputArgs(list), Duration: testSec, AudioIndex: 0, AudioChannels: 2, SubtitleIndex: -1, Preset: "veryfast"}
	starts := func(n int) (video, audio float64) {
		h := m.Ensure(spec)
		p, err := h.Segment(ctx, n)
		if err != nil {
			t.Fatal(err)
		}
		pts := func(stream string) float64 {
			out, err := ffmpeg.Command(ctx, ff.FFprobe, "-v", "error", "-select_streams", stream, "-show_entries", "packet=pts_time", "-of", "csv=p=0", p).Output()
			if err != nil {
				t.Fatal(err)
			}
			min := math.Inf(1)
			for _, f := range strings.Fields(strings.ReplaceAll(string(out), ",", " ")) {
				if v, err := strconv.ParseFloat(f, 64); err == nil && v < min {
					min = v
				}
			}
			return min
		}
		return pts("v:0"), pts("a:0")
	}
	starts(0)
	v3, a3 := starts(3) // the first run: from the start, on to this one
	m.Stop("v-restart")
	v3b, a3b := starts(3) // a fresh FFmpeg, starting at that segment
	if math.Abs(v3-v3b) > 0.001 {
		t.Errorf("picture: the first run has segment 3 at %.3f, a restart at %.3f", v3, v3b)
	}
	// Which sound frame comes first in the segment may differ. What can't is
	// where the frames lie: within half a frame of the same places.
	const frame = 1024.0 / 48000
	if d := math.Abs(math.Remainder(a3-a3b, frame)); d > frame/2+0.001 {
		t.Errorf("sound: the first run has segment 3 at %.3f, a restart at %.3f", a3, a3b)
	}
}

// A transport stream has no index, so a seek lands late, by up to a second or so.
// A restart must still begin on the right frame, or the viewer sees the first
// picture held for that long and then the film further on.
func TestRestartInATransportStreamIsExact(t *testing.T) {
	ff := tools(t)
	dir := t.TempDir()
	clip := filepath.Join(dir, "clip.m2t")
	run(t, ff, append(source(testSec), "-c:v", "libx264", "-preset", "veryfast", "-g", "48", "-keyint_min", "48", "-sc_threshold", "0", "-bf", "3", "-b:v", "1500k",
		"-c:a", "ac3", "-b:a", "192k", "-streamid", "0:0x1011", "-streamid", "1:0x1100", "-f", "mpegts", clip)...)
	ctx := context.Background()
	ref, _, err := Reference(ctx, ff, [][]string{{"-i", clip}}, testFPS, "", "", dir)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(clip)
	list, err := ffmpeg.WriteConcat(dir, "clip", []ffmpeg.Segment{{Extents: []wittydvd.FileExtent{{File: clip, Length: st.Size()}}, Duration: testSec}}, 0x1011, 0x1100)
	if err != nil {
		t.Fatal(err)
	}
	m := hls.NewManager(ff, filepath.Join(dir, "hls"), 2, slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(m.StopAll)
	for _, n := range []int{1, 3, 5, 6} {
		m.Stop("v-clip")
		h := m.Ensure(hls.Spec{ID: "v-clip", InputArgs: ffmpeg.ConcatInputArgs(list), Duration: testSec, AudioIndex: 0, AudioChannels: 2, SubtitleIndex: -1, Preset: "veryfast", SeekMargin: 3})
		p, err := h.Segment(ctx, n)
		if err != nil {
			t.Fatal(err)
		}
		got, _, err := Decode(ctx, ff, []string{"-i", p}, testFPS, "", "", dir)
		if err != nil {
			t.Fatal(err)
		}
		from := n * 4 * int(testFPS)
		if len(got.Frames) != 4*int(testFPS) {
			t.Errorf("segment %d: %d frames", n, len(got.Frames))
		}
		for i := range got.Frames {
			if d := diff(&got.Frames[i], &ref.Frames[from+i]); d > thumbSize*3 {
				t.Errorf("segment %d: frame %d isn't the film's frame %d (difference %d)", n, i, from+i, d)
				break
			}
		}
	}
}
