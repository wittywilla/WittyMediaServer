package bdsession

import (
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

	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/seamcheck"
)

// These tests encode clips with FFmpeg, lay them out on a disc folder with their
// real entry points, and play them through the real stream code. Nothing
// copyrighted is needed, only FFmpeg (WMS_FFMPEG, or on PATH); they are skipped
// without it.

const (
	videoPID = 0x1011
	soundPID = 0x1100
	seamFPS  = 25
	seamGOP  = 48
	// frameTicks is a frame's length at 45 kHz.
	frameTicks = 45000 / seamFPS
	// soundBehind is how much later than its pictures a clip stores its sound, in
	// the 90 kHz clock of the stream: half a second, as a disc might.
	soundBehind = 45000
)

// seamClip is a clip on the test disc, with where its first picture lies.
type seamClip struct {
	clip   *wittybluray.Clip
	first  uint32 // 45 kHz
	frames int
}

// at is the time of a frame.
func (c *seamClip) at(frame int) uint32 { return c.first + uint32(frame)*frameTicks }

// storeSoundLater moves a transport stream's sound behind its pictures, the way a
// disc holds it: a picture arrives well before it is shown, sound just before.
// Without this, FFmpeg's own muxer keeps them side by side, and an end that cuts
// sound short wouldn't show.
func storeSoundLater(b []byte) []byte {
	type packet struct {
		data []byte
		at   int64
	}
	var video, other []packet
	var headers int
	var at [2]int64
	for off := 0; off+192 <= len(b); off += 192 {
		pkt := b[off : off+192]
		p := pkt[4:]
		pid := uint16(p[1]&0x1f)<<8 | uint16(p[2])
		kind := 0
		if pid == soundPID {
			kind = 1
		}
		if (pid == videoPID || pid == soundPID) && p[1]&0x40 != 0 {
			h := 4
			if p[3]&0x20 != 0 {
				h += 1 + int(p[4])
			}
			if h+14 < len(p) && p[h] == 0 && p[h+1] == 0 && p[h+2] == 1 && p[h+7]&0x80 != 0 {
				q := p[h+9:]
				if p[h+7]&0x40 != 0 {
					q = q[5:] // the decode time, when it has its own
				}
				at[kind] = int64(q[0]>>1&7)<<30 | int64(q[1])<<22 | int64(q[2]>>1)<<15 | int64(q[3])<<7 | int64(q[4]>>1)
			}
		}
		if kind == 1 {
			other = append(other, packet{pkt, at[1] + soundBehind})
			continue
		}
		if pid != videoPID {
			headers++ // these go right before the next picture packet
		} else {
			for i := len(video) - headers; i < len(video); i++ {
				video[i].at = at[0]
			}
			headers = 0
		}
		video = append(video, packet{pkt, at[0]})
	}
	out := make([]byte, 0, len(b))
	for len(video) > 0 || len(other) > 0 {
		if len(other) == 0 || (len(video) > 0 && video[0].at <= other[0].at) {
			out = append(out, video[0].data...)
			video = video[1:]
		} else {
			out = append(out, other[0].data...)
			other = other[1:]
		}
	}
	return out
}

// encodeClip writes a film to where a disc holds a clip, and reads its entry points
// back from the stream: its keyframes, with times rounded as a table has them.
func encodeClip(t *testing.T, ff *ffmpeg.Tools, path string, seed, sec int) *seamClip {
	t.Helper()
	ctx := context.Background()
	// Each film is unlike the other: another stretch of the picture, other noise.
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=320x180:rate=%d:duration=%d", seamFPS, sec+seed*20),
		"-f", "lavfi", "-i", fmt.Sprintf("anoisesrc=d=%d:c=pink:r=48000:s=%d:a=0.5,aformat=channel_layouts=stereo", sec+seed*20, seed+1),
		"-ss", strconv.Itoa(seed * 20),
		"-c:v", "libx264", "-preset", "veryfast", "-g", strconv.Itoa(seamGOP), "-keyint_min", strconv.Itoa(seamGOP), "-sc_threshold", "0", "-bf", "3", "-b:v", "1500k",
		"-c:a", "ac3", "-b:a", "192k", "-streamid", "0:0x1011", "-streamid", "1:0x1100",
		"-mpegts_m2ts_mode", "1", "-mpegts_flags", "+pat_pmt_at_frames", "-f", "mpegts", path}
	if out, err := ffmpeg.Command(ctx, ff.FFmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b = storeSoundLater(b)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := ffmpeg.Command(ctx, ff.FFprobe, "-v", "error", "-select_streams", "v:0", "-show_entries", "packet=pts,pos,flags", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	m := wittybluray.EPMap{PID: videoPID}
	c := &seamClip{}
	for i, line := range strings.Fields(string(out)) {
		f := strings.Split(line, ",")
		if len(f) < 3 {
			continue
		}
		c.frames++
		if !strings.HasPrefix(f[2], "K") {
			continue
		}
		pts, _ := strconv.ParseUint(f[0], 10, 64)
		pos, _ := strconv.ParseUint(f[1], 10, 64)
		if i == 0 {
			c.first = uint32(pts / 2)
		}
		spn := uint32(pos / 192)
		// The headers just before a keyframe belong to it.
		for spn > 0 {
			p := b[int(spn-1)*192+4:]
			if pid := uint16(p[1]&0x1f)<<8 | uint16(p[2]); pid == videoPID || pid == soundPID {
				break
			}
			spn--
		}
		m.Entries = append(m.Entries, wittybluray.EPEntry{PTS: uint32(pts/2) &^ (epRounding - 1), SPN: spn})
	}
	if len(m.Entries) < 4 {
		t.Fatalf("%d keyframes in the test clip", len(m.Entries))
	}
	c.clip = &wittybluray.Clip{
		Packets:  uint32(len(b) / 192),
		Programs: []wittybluray.Program{{Streams: []wittybluray.ClipStream{{PID: videoPID, Coding: 0x1b}, {PID: soundPID, Coding: 0x81}}}},
		EPMaps:   []wittybluray.EPMap{m},
		STC:      []wittybluray.STCSequence{{ID: 0, Start: c.first, End: c.at(c.frames)}},
	}
	return c
}

// seamDisc is a session over two such clips, and each one's film as a straight read.
type seamDisc struct {
	ff     *ffmpeg.Tools
	s      *Session
	tmp    string
	clips  map[string]*seamClip
	frames map[string]*seamcheck.Pictures
	sounds map[string][]int16
}

func newSeamDisc(t *testing.T) *seamDisc {
	t.Helper()
	ff, err := ffmpeg.Locate(os.Getenv("WMS_FFMPEG"), "")
	if err != nil {
		t.Skip(err)
	}
	if !ff.Encoders["libx264"] || !ff.Encoders["ac3"] {
		t.Skip("this FFmpeg lacks the encoders for the test clips")
	}
	d := &seamDisc{ff: ff, s: seamSession(t), tmp: t.TempDir(), clips: map[string]*seamClip{}, frames: map[string]*seamcheck.Pictures{}, sounds: map[string][]int16{}}
	for i, name := range []string{"00001", "00002"} {
		ext, err := d.s.disc.ClipExtents(name)
		if err != nil {
			t.Fatal(err)
		}
		c := encodeClip(t, ff, ext[0].Path, i, 20)
		c.clip.Name = name
		d.clips[name] = c
		d.s.disc.Clips[name] = c.clip
		seg, err := d.s.clipSegment(name, 0, c.clip.Packets, 0)
		if err != nil {
			t.Fatal(err)
		}
		list, err := ffmpeg.WriteConcat(d.tmp, "ref"+name, []ffmpeg.Segment{seg}, videoPID, soundPID)
		if err != nil {
			t.Fatal(err)
		}
		d.frames[name], d.sounds[name], err = seamcheck.Reference(context.Background(), ff, [][]string{ffmpeg.ConcatInputArgs(list)}, seamFPS, "", "0:a:0", d.tmp)
		if err != nil {
			t.Fatal(err)
		}
		if len(d.frames[name].Frames) != c.frames {
			t.Fatalf("clip %s: %d frames decoded, %d stored", name, len(d.frames[name].Frames), c.frames)
		}
	}
	return d
}

// span is one play item: frames of a clip.
type span struct {
	clip     string
	from, to int
}

func (d *seamDisc) playlist(spans []span) *wittybluray.Playlist {
	pl := &wittybluray.Playlist{Number: 1}
	for _, sp := range spans {
		c := d.clips[sp.clip]
		pl.Items = append(pl.Items, wittybluray.PlayItem{Clip: sp.clip, In: c.at(sp.from), Out: c.at(sp.to), Video: []wittybluray.Stream{{PID: videoPID}}})
	}
	return pl
}

// reference is what the items should show: exactly those frames, and their sound.
func (d *seamDisc) reference(spans []span) (*seamcheck.Pictures, []int16, []float64, []string) {
	ref := &seamcheck.Pictures{FPS: seamFPS}
	var sound []int16
	var at []float64
	var labels []string
	for i, sp := range spans {
		if i > 0 {
			at, labels = append(at, float64(len(ref.Frames))/seamFPS), append(labels, fmt.Sprintf("item %d", i+1))
		}
		ref.Frames = append(ref.Frames, d.frames[sp.clip].Frames[sp.from:sp.to]...)
		s := d.sounds[sp.clip]
		sound = append(sound, s[sp.from*seamcheck.AudioRate/seamFPS:min(sp.to*seamcheck.AudioRate/seamFPS, len(s))]...)
	}
	return ref, sound, at, labels
}

func (d *seamDisc) stream(t *testing.T, name string, segs []ffmpeg.Segment, holes []hls.Hole, duration float64) *hls.Manager {
	t.Helper()
	list, err := ffmpeg.WriteConcat(d.tmp, name, segs, videoPID, soundPID)
	if err != nil {
		t.Fatal(err)
	}
	m := hls.NewManager(d.ff, filepath.Join(d.tmp, name+"-hls"), 2, slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(m.StopAll)
	m.Ensure(hls.Spec{ID: "v-" + name, InputArgs: ffmpeg.ConcatInputArgs(list), Duration: duration, AudioIndex: 0, AudioChannels: 2, SubtitleIndex: -1, Preset: "ultrafast", SeekMargin: seekMargin, Holes: holes})
	return m
}

// play runs inputs through the real stream code and measures it against ref.
func (d *seamDisc) play(t *testing.T, name string, segs []ffmpeg.Segment, holes []hls.Hole, duration float64, ref *seamcheck.Pictures, refSound []int16, at []float64, labels []string) (*seamcheck.Pictures, []seamcheck.Boundary) {
	t.Helper()
	ctx := context.Background()
	h := d.stream(t, name, segs, holes, duration).Get("v-" + name)
	joined := filepath.Join(d.tmp, name+".ts")
	if err := seamcheck.Join(ctx, h, joined, false); err != nil {
		t.Fatal(err)
	}
	out, sound, err := seamcheck.Decode(ctx, d.ff, []string{"-i", joined}, seamFPS, "", "0:a:0", d.tmp)
	if err != nil {
		t.Fatal(err)
	}
	rows := seamcheck.Measure(seamcheck.Input{Ref: ref, Out: out, RefSound: refSound, OutSound: sound, Boundaries: at, Labels: labels})
	t.Logf("%s: %d frames of %d\n%s", name, len(out.Frames), len(ref.Frames), seamcheck.Table(rows))
	return out, rows
}

// exact says what a stream has to be: every frame in its place, and at the joins
// the sound in step, with no drop-out.
func exact(t *testing.T, ref, out *seamcheck.Pictures, rows []seamcheck.Boundary) {
	t.Helper()
	if len(out.Frames) != len(ref.Frames) {
		t.Errorf("%d frames, want %d", len(out.Frames), len(ref.Frames))
	}
	for i, k := range seamcheck.Align(ref, out).Map {
		if i != k {
			t.Errorf("frame %d shows the film's frame %d", i, k)
			break
		}
	}
	for _, r := range rows {
		if r.Lost != 0 || r.Repeated != 0 || r.ShiftMs != 0 {
			t.Errorf("%s: %d frames lost, %d repeated, picture moved %.0f ms", r.Label, r.Lost, r.Repeated, r.ShiftMs)
		}
		if r.SoundConfidence < 0.5 {
			t.Errorf("%s: the sound can't be followed: confidence %.2f", r.Label, r.SoundConfidence)
		}
		if math.Abs(r.SoundJumpMs) > 33 || r.HoleMs > 33 {
			t.Errorf("%s: sound jumps %.1f ms, drops out for %.0f ms", r.Label, r.SoundJumpMs, r.HoleMs)
		}
	}
}

// An item that ends part-way through a clip has to show all its pictures and sound
// and nothing after them, whatever the next item is. Both ways of cutting that
// aren't good enough are measured too: if they show nothing wrong, these clips
// can't show the bug and this test proves nothing.
func TestItemsEndAndStartExactly(t *testing.T) {
	d := newSeamDisc(t)
	a, b := "00001", "00002"
	endA, endB := d.clips[a].frames, d.clips[b].frames
	const midGOP = seamGOP*4 + 29 // 29 frames past a keyframe
	cases := []struct {
		name  string
		spans []span
	}{
		{"ends-mid-clip", []span{{a, 0, midGOP}, {b, 0, endB}}},
		{"ends-on-a-keyframe", []span{{a, 0, seamGOP * 4}, {b, 0, endB}}},
		{"last-ends-mid-clip", []span{{a, 0, endA}, {b, 0, midGOP}}},
		{"starts-mid-clip", []span{{a, 0, endA}, {b, seamGOP*2 + 13, endB}}},
		{"first-starts-mid-clip", []span{{a, seamGOP + 21, midGOP}, {b, seamGOP*2 + 13, seamGOP*5 + 7}}},
		{"same-clip-carries-on", []span{{a, 0, midGOP}, {a, midGOP, seamGOP*7 + 5}, {b, 0, endB}}},
		{"same-clip-jumps-back", []span{{a, seamGOP, midGOP}, {a, seamGOP * 2, seamGOP*3 + 11}}},
		{"one-item", []span{{a, seamGOP + 5, midGOP}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			laid := d.s.layout(d.playlist(c.spans), -1)
			segs, holes, err := d.s.segments(laid, videoPID, soundPID)
			if err != nil {
				t.Fatal(err)
			}
			ref, sound, at, labels := d.reference(c.spans)
			if math.Abs(laid.duration-float64(len(ref.Frames))/seamFPS) > 0.001 {
				t.Errorf("laid out as %.3f s, want %d frames", laid.duration, len(ref.Frames))
			}
			out, rows := d.play(t, c.name, segs, holes, laid.duration, ref, sound, at, labels)
			exact(t, ref, out, rows)
		})
	}

	spans := []span{{a, 0, midGOP}, {b, 0, endB}}
	ref, sound, at, labels := d.reference(spans)
	laid := d.s.layout(d.playlist(spans), -1)
	duration := laid.duration
	segs, _, err := d.s.segments(laid, videoPID, soundPID)
	if err != nil {
		t.Fatal(err)
	}

	// The old cut: stop reading at the entry point at or before the item's end.
	e, _ := d.clips[a].clip.EntryAt(videoPID, 0, d.clips[a].at(midGOP))
	seg, err := d.s.clipSegment(a, 0, e.SPN, laid.items[0].duration)
	if err != nil {
		t.Fatal(err)
	}
	seg.Lead = segs[0].Lead
	_, rows := d.play(t, "old-cut", []ffmpeg.Segment{seg, segs[1]}, nil, duration, ref, sound, at, labels)
	if rows[0].Lost < 20 {
		t.Errorf("the old cut lost %d frames, not the 29 it should: this test can't see the bug any more", rows[0].Lost)
	}

	// Stopping by time alone: pictures decoded before the end but shown after it
	// get through, and take the place of the next item's first ones.
	seg = segs[0]
	seg.Duration, seg.Outpoint = laid.items[0].duration, float64(d.clips[a].at(midGOP))/45000
	out, rows := d.play(t, "stop-by-time", []ffmpeg.Segment{seg, segs[1]}, nil, duration, ref, sound, at, labels)
	if len(out.Frames) == len(ref.Frames) && rows[0].ShiftMs == 0 && rows[0].Lost == 0 && rows[0].HoleMs == 0 {
		t.Error("stopping by time alone shows nothing wrong: holes are no longer needed, or this test can't see why they are")
	}
}

// A restart begins by itself, wherever the viewer is: a seek, or the server
// starting again after resting. It must show the same frames as playing up to there.
func TestRestartAroundAnItemEnd(t *testing.T) {
	d := newSeamDisc(t)
	a, b := "00001", "00002"
	const midGOP = seamGOP*4 + 29
	spans := []span{{a, seamGOP + 21, midGOP}, {b, seamGOP*2 + 13, d.clips[b].frames}}
	laid := d.s.layout(d.playlist(spans), -1)
	segs, holes, err := d.s.segments(laid, videoPID, soundPID)
	if err != nil {
		t.Fatal(err)
	}
	ref, _, _, _ := d.reference(spans)
	ctx := context.Background()
	m := d.stream(t, "restart", segs, holes, laid.duration)
	for _, n := range []int{0, 1, 2, 3, 4} { // the join is 6.4 s in, within segment 1
		m.Stop("v-restart")
		m = d.stream(t, "restart", segs, holes, laid.duration)
		p, err := m.Get("v-restart").Segment(ctx, n)
		if err != nil {
			t.Fatal(err)
		}
		got, _, err := seamcheck.Decode(ctx, d.ff, []string{"-i", p}, seamFPS, "", "", d.tmp)
		if err != nil {
			t.Fatal(err)
		}
		from := n * 4 * seamFPS
		if len(got.Frames) != 4*seamFPS {
			t.Errorf("segment %d: %d frames", n, len(got.Frames))
		}
		part := &seamcheck.Pictures{FPS: seamFPS, Frames: ref.Frames[from:]}
		for i, k := range seamcheck.Align(part, got).Map {
			if i != k {
				t.Errorf("segment %d: its frame %d shows the film's frame %d, not %d", n, i, from+k, from+i)
				break
			}
		}
	}
}
