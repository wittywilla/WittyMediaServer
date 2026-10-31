// Package seamcheck measures what a stream does to the disc underneath it:
// which pictures and how much sound it loses, repeats or moves at each chapter
// boundary. It needs no person watching: both sides are decoded to small
// fingerprints (a tiny grey picture per frame, low-rate mono sound) and lined up
// against each other, so it works across a re-encode.
package seamcheck

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
)

const (
	thumbW, thumbH = 32, 18
	thumbSize      = thumbW * thumbH
	// AudioRate is the rate sound is compared at: an eighth of a millisecond.
	AudioRate = 8000
)

// Pictures is every frame of a stream as tiny grey pictures, in order.
type Pictures struct {
	FPS    float64
	Frames [][thumbSize]byte
}

// Decode reads the pictures and sound of an input into fingerprints. Each
// frame keeps its place by its timestamps at fps (passthrough would only tell
// their order), so repeating or leaving out a frame shows. filter runs first;
// the reference side uses the same one as the stream so both look alike.
// audio maps the sound, e.g. "0:a:0" or "0:i:0x80"; "" leaves it out.
func Decode(ctx context.Context, t *ffmpeg.Tools, input []string, fps float64, filter, audio, tmp string) (*Pictures, []int16, error) {
	// Averaged, it barely changes with a scale and a re-encode in between.
	vf := "scale=" + strconv.Itoa(thumbW) + ":" + strconv.Itoa(thumbH) + ":flags=area,format=gray"
	if filter != "" {
		vf = filter + "," + vf
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-max_error_rate", "1"}
	args = append(args, input...)
	args = append(args, "-map", "0:v:0", "-vf", vf, "-fps_mode", "passthrough", "-f", "rawvideo", "-pix_fmt", "gray", "pipe:1")
	pcm := ""
	if audio != "" {
		// Placed by its timestamps to within 10 ms, as a player places it, or
		// small steps would add up unseen until they passed FFmpeg's usual 100 ms.
		pcm = filepath.Join(tmp, "sound.pcm")
		args = append(args, "-map", audio+"?", "-af", "aresample=async=1:min_hard_comp=0.01:first_pts=0", "-ac", "1", "-ar", strconv.Itoa(AudioRate), "-f", "s16le", pcm)
	}
	cmd := ffmpeg.Command(ctx, t.FFmpeg, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	p := &Pictures{FPS: fps}
	r := bufio.NewReaderSize(out, 1<<20)
	for {
		var f [thumbSize]byte
		if _, err := io.ReadFull(r, f[:]); err != nil {
			break
		}
		p.Frames = append(p.Frames, f)
	}
	if err := cmd.Wait(); err != nil {
		return nil, nil, fmt.Errorf("ffmpeg: %w: %s", err, lastLines(stderr.String()))
	}
	if pcm == "" {
		return p, nil, nil
	}
	defer os.Remove(pcm)
	b, err := os.ReadFile(pcm) // no such stream: no file, or an empty one
	if err != nil {
		return nil, nil, err
	}
	s := make([]int16, len(b)/2)
	for i := range s {
		s[i] = int16(binary.LittleEndian.Uint16(b[i*2:]))
	}
	return p, s, nil
}

func lastLines(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 600 {
		return s[len(s)-600:]
	}
	return s
}

// FirstFrameSec is where an input's first picture sits on the timeline FFmpeg
// gives it, which starts at its earliest stream; 0 when it can't be told. It goes
// by the first packets, as a list of disc cells often reports no video start time.
func FirstFrameSec(ctx context.Context, t *ffmpeg.Tools, input []string) float64 {
	args := []string{"-v", "error", "-read_intervals", "%+#2000", "-show_entries", "packet=codec_type,pts_time", "-of", "csv=p=0"}
	for i, a := range input { // options after the input are for the output
		args = append(args, a)
		if i > 0 && input[i-1] == "-i" {
			break
		}
	}
	out, err := ffmpeg.Command(ctx, t.FFprobe, args...).Output()
	if err != nil {
		return 0
	}
	first, video := math.Inf(1), math.Inf(1)
	for _, line := range strings.Fields(string(out)) {
		kind, at, ok := strings.Cut(strings.TrimSuffix(line, ","), ",")
		v, err := strconv.ParseFloat(at, 64)
		if !ok || err != nil {
			continue // a picture between its neighbours states no time
		}
		first = min(first, v)
		if kind == "video" {
			video = min(video, v)
		}
	}
	if math.IsInf(video, 1) {
		return 0
	}
	return video - first
}

// Match follows the output through the reference: Map[i] is the reference
// frame that output frame i shows.
type Match struct {
	Map []int
}

func diff(a, b *[thumbSize]byte) int {
	n := 0
	for i := range a {
		d := int(a[i]) - int(b[i])
		if d < 0 {
			d = -d
		}
		n += d
	}
	return n
}

// matchWindow is how far from where it should be (in frames) a picture is
// looked for: enough to see any plausible seam, not enough to confuse scenes.
const matchWindow = 120

// Align finds, for each output frame, the reference frame it shows. An output
// frame stays on the frame the walk expects unless another one nearby is clearly
// a better likeness: stills and fades, where many frames look alike, then don't
// scatter the result, and a frame that matches nothing well moves nothing.
func Align(ref, out *Pictures) *Match {
	m := &Match{Map: make([]int, len(out.Frames))}
	pos := 0
	for i := range out.Frames {
		f := &out.Frames[i]
		if pos >= len(ref.Frames) {
			pos = len(ref.Frames) - 1
		}
		d0 := diff(f, &ref.Frames[pos])
		exp := pos
		// One grey level per pixel on average is what a re-encode changes.
		if d0 > thumbSize {
			best, bestD := pos, d0
			for k := max(0, pos-matchWindow); k < min(len(ref.Frames), pos+matchWindow); k++ {
				d := diff(f, &ref.Frames[k])
				if d < bestD || (d == bestD && abs(k-pos) < abs(best-pos)) {
					best, bestD = k, d
				}
			}
			if bestD*3 < d0*2 {
				exp = best
			}
		}
		m.Map[i] = exp
		pos = exp + 1
	}
	return m
}

// Event is a place where the output leaves the reference's order.
type Event struct {
	Sec            float64 // on the output's timeline
	Lost, Repeated int     // frames skipped over, or shown again
}

// Events lists every place in the stream where frames are left out or repeated,
// chapter or no chapter. Adjacent ones add up.
func Events(ref, out *Pictures) []Event {
	m := Align(ref, out)
	var ev []Event
	for i := 1; i < len(m.Map); i++ {
		step := m.Map[i] - m.Map[i-1]
		if step == 1 {
			continue
		}
		sec := float64(i) / out.FPS
		if n := len(ev); n > 0 && sec-ev[n-1].Sec < 1 {
			if step > 1 {
				ev[n-1].Lost += step - 1
			} else {
				ev[n-1].Repeated += 1 - step
			}
			continue
		}
		e := Event{Sec: sec}
		if step > 1 {
			e.Lost = step - 1
		} else {
			e.Repeated = 1 - step
		}
		ev = append(ev, e)
	}
	return ev
}

// SoundJump is a place where the sound moves against the reference.
type SoundJump struct {
	Sec, FromMs, ToMs, Confidence float64
}

// SoundJumps walks the whole stream and lists where its sound moves against the
// reference. Quiet or ambiguous stretches are passed over.
func SoundJumps(ref, out []int16, every, atLeastMs float64) []SoundJump {
	var jumps []SoundJump
	have := false
	last := 0.0
	end := float64(len(out))/AudioRate - 1.5
	for sec := 1.0; sec < end; sec += every {
		ms, conf := soundLag(ref, out, sec, sec)
		if conf < 0.8 {
			continue
		}
		if have && math.Abs(ms-last) >= atLeastMs {
			jumps = append(jumps, SoundJump{Sec: sec, FromMs: last, ToMs: ms, Confidence: conf})
		}
		last, have = ms, true
	}
	return jumps
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// Boundary is what happened at one chapter or cell boundary.
type Boundary struct {
	Label string
	Sec   float64
	// MarkMs is how far from the chapter's announced time the chapter's first
	// picture shows (+ = late). It is what a chapter jump lands next to.
	MarkMs float64
	// Lost and Repeated count frames within WindowSec of the boundary.
	Lost, Repeated int
	// ShiftMs is how much the picture moved on the timeline across the boundary.
	ShiftMs float64
	// Sound: where it sits against the reference on each side (+ = late), the
	// amount lost or added across the boundary (Sound lost = after minus before,
	// negative = sound missing), and how sure the comparison is (0 to 1).
	SoundBeforeMs, SoundAfterMs, SoundJumpMs, SoundConfidence float64
	// SyncMs is the sound against the picture just after the boundary (+ = sound late).
	SyncMs float64
	// HoleMs is the longest drop-out in the sound within half a second of the
	// boundary: too short to move what comes after, and heard as a click or a gap.
	HoleMs float64
}

// WindowSec is how far either side of a boundary frames are counted.
const WindowSec = 2.0

// Input to Measure.
type Input struct {
	Ref, Out           *Pictures
	RefSound, OutSound []int16
	// Boundaries are times where the reference frame of the same time should
	// show (reference frame i and sound sample s/AudioRate share a timeline);
	// Labels name them.
	Boundaries []float64
	Labels     []string
}

// Measure compares the output with the reference at each boundary.
func Measure(in Input) []Boundary {
	m := Align(in.Ref, in.Out)
	inv := make([]int, len(in.Ref.Frames))
	for i := range inv {
		inv[i] = -1
	}
	for i, k := range m.Map {
		if k >= 0 && k < len(inv) && inv[k] < 0 {
			inv[k] = i
		}
	}
	fps := in.Out.FPS
	w := int(WindowSec * fps)
	out := make([]Boundary, 0, len(in.Boundaries))
	for i, sec := range in.Boundaries {
		b := Boundary{Sec: sec}
		if i < len(in.Labels) {
			b.Label = in.Labels[i]
		}
		rf := int(math.Round(sec * in.Ref.FPS))
		for k := max(0, rf-w); k < min(len(inv), rf+w); k++ {
			if inv[k] < 0 {
				b.Lost++
			}
		}
		of := max(0, rf-w)
		for j := of + 1; j < min(len(m.Map), rf+w+int(fps)); j++ {
			if m.Map[j] == m.Map[j-1] && j >= rf-w && j < rf+w {
				b.Repeated++
			}
		}
		before, after := medianOffset(m, inv, rf-w, rf-2), medianOffset(m, inv, rf+2, rf+w)
		b.ShiftMs = float64(after-before) / fps * 1000
		if rf >= 0 && rf < len(inv) && inv[rf] >= 0 {
			b.MarkMs = (float64(inv[rf])/fps - sec) * 1000
		}
		if in.RefSound != nil && in.OutSound != nil {
			var c1, c2 float64
			b.SoundBeforeMs, c1 = soundLag(in.RefSound, in.OutSound, sec-1.1, sec-1.1)
			b.SoundAfterMs, c2 = soundLag(in.RefSound, in.OutSound, sec+0.1, sec+0.1)
			b.SoundJumpMs = b.SoundAfterMs - b.SoundBeforeMs
			b.SoundConfidence = min(c1, c2)
			b.SyncMs = b.SoundAfterMs - float64(after)/fps*1000
			if b.SoundConfidence > 0.5 {
				b.HoleMs = soundHole(in.RefSound, in.OutSound, sec, b.SoundBeforeMs, b.SoundAfterMs)
			}
		}
		out = append(out, b)
	}
	return out
}

// medianOffset is how many frames late the reference frames from..to show.
func medianOffset(m *Match, inv []int, from, to int) int {
	var v []int
	for k := max(0, from); k < min(len(inv), to); k++ {
		if inv[k] >= 0 {
			v = append(v, inv[k]-k)
		}
	}
	if len(v) == 0 {
		return 0
	}
	sort.Ints(v)
	return v[len(v)/2]
}

// soundLag finds where a second of the output's sound at outSec lies in the
// reference near refSec, by correlation: first coarsely, then to the sample.
// It gives the lag in ms (+ = the output is late) and the correlation (0 to 1).
func soundLag(ref, out []int16, refSec, outSec float64) (ms, conf float64) {
	const span = AudioRate
	const search = AudioRate // one second either way
	o0 := int(outSec * AudioRate)
	r0 := int(refSec * AudioRate)
	if o0 < 0 || o0+span > len(out) || r0 < 0 {
		return 0, 0
	}
	win := out[o0 : o0+span]
	corr := func(lag, step int) float64 {
		a := r0 - lag
		if a < 0 || a+span > len(ref) {
			return -2
		}
		var sxy, sxx, syy float64
		for i := 0; i < span; i += step {
			x, y := float64(win[i]), float64(ref[a+i])
			sxy += x * y
			sxx += x * x
			syy += y * y
		}
		if sxx == 0 || syy == 0 {
			return 0
		}
		return sxy / math.Sqrt(sxx*syy)
	}
	best, bestC := 0, -2.0
	for lag := -search; lag <= search; lag += 4 {
		if c := corr(lag, 4); c > bestC {
			best, bestC = lag, c
		}
	}
	for lag := best - 8; lag <= best+8; lag++ {
		if c := corr(lag, 1); c > bestC || lag == best-8 {
			best, bestC = lag, c
		}
	}
	return float64(best) / AudioRate * 1000, max(0, bestC)
}

// soundHole finds the longest stretch near sec where the output is all but silent
// and the reference isn't. The lags say where each side lines up.
func soundHole(ref, out []int16, sec, beforeMs, afterMs float64) float64 {
	const block = AudioRate / 200 // 5 ms
	rms := func(s []int16) float64 {
		var v float64
		for _, x := range s {
			v += float64(x) * float64(x)
		}
		return math.Sqrt(v / float64(len(s)))
	}
	longest, run := 0, 0
	for t := sec - 0.5; t < sec+0.5; t += 0.005 {
		lag := beforeMs
		if t >= sec {
			lag = afterMs
		}
		o := int(t * AudioRate)
		r := int((t - lag/1000) * AudioRate)
		if o < 0 || r < 0 || o+block > len(out) || r+block > len(ref) {
			run = 0
			continue
		}
		if a, b := rms(out[o:o+block]), rms(ref[r:r+block]); b > 300 && a < b/5 {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	return float64(longest) * 5
}

// Table prints the measurements, one boundary per line.
func Table(rows []Boundary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-14s %10s %6s %5s %9s %9s %10s %10s %9s %7s %8s %8s\n", "boundary", "at", "lost", "rep", "shift ms", "mark ms", "snd<- ms", "snd-> ms", "jump ms", "conf", "sync ms", "hole ms")
	var lost, rep int
	var jump, shift, hole float64
	for _, r := range rows {
		fmt.Fprintf(&b, "%-14s %10.2f %6d %5d %9.0f %9.0f %10.1f %10.1f %9.1f %7.2f %8.0f %8.0f\n", r.Label, r.Sec, r.Lost, r.Repeated, r.ShiftMs, r.MarkMs, r.SoundBeforeMs, r.SoundAfterMs, r.SoundJumpMs, r.SoundConfidence, r.SyncMs, r.HoleMs)
		lost += r.Lost
		rep += r.Repeated
		shift += math.Abs(r.ShiftMs)
		hole = max(hole, r.HoleMs)
		if r.SoundConfidence > 0.5 {
			jump += math.Abs(r.SoundJumpMs)
		}
	}
	fmt.Fprintf(&b, "total: %d frames lost, %d repeated, %.0f ms of picture movement, %.1f ms of sound jumps, longest sound drop-out %.0f ms, across %d boundaries\n", lost, rep, shift, jump, hole, len(rows))
	return b.String()
}

// Reference decodes runs of a disc that were each read straight through, with
// nothing cut, and lays them end to end: frame i sits at i/fps and the sound
// next to it. A run's sound is moved so it starts with the run's first picture,
// and is padded or cut to the run's length (a run's sound may start early or end
// late, around the picture).
func Reference(ctx context.Context, t *ffmpeg.Tools, runs [][]string, fps float64, filter, audio, tmp string) (*Pictures, []int16, error) {
	out := &Pictures{FPS: fps}
	var sound []int16
	for _, in := range runs {
		p, s, err := Decode(ctx, t, in, fps, filter, audio, tmp)
		if err != nil && audio != "" { // a short cell may have no sound at all
			p, s, err = Decode(ctx, t, in, fps, filter, "", tmp)
		}
		if err != nil {
			return nil, nil, err
		}
		out.Frames = append(out.Frames, p.Frames...)
		if audio != "" {
			skip := min(int(FirstFrameSec(ctx, t, in)*AudioRate), len(s))
			s = s[skip:]
			want := max(0, int(float64(len(out.Frames))/fps*AudioRate)-len(sound))
			if len(s) > want {
				s = s[:want]
			}
			sound = append(sound, s...)
			sound = append(sound, make([]int16, want-len(s))...)
		}
	}
	return out, sound, nil
}

// Join fetches every segment of a stream in order into one file, as a player
// does. paced behaves like someone watching in real time: the server gets far
// enough ahead that it rests, and starts FFmpeg again when the viewer nears the
// end of what is ready, so the file holds those restarts too.
func Join(ctx context.Context, h *hls.Session, to string, paced bool) error {
	f, err := os.Create(to)
	if err != nil {
		return err
	}
	defer f.Close()
	for n := 0; n < h.SegmentCount(); n++ {
		p, err := h.Segment(ctx, n)
		if err != nil {
			return fmt.Errorf("segment %d: %w", n, err)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if _, err := f.Write(b); err != nil {
			return err
		}
		if paced {
			ahead := filepath.Join(h.Dir, fmt.Sprintf("seg%05d%s", n+95, h.SegmentExt()))
			for h.Active() {
				if _, err := os.Stat(ahead); err == nil {
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
		}
	}
	return nil
}

// Whole reports everything the stream does wrong, anywhere.
func Whole(ref, out *Pictures, refSound, outSound []int16) string {
	var b strings.Builder
	ev := Events(ref, out)
	lost, rep := 0, 0
	for _, e := range ev {
		lost += e.Lost
		rep += e.Repeated
		if e.Lost > 2 || e.Repeated > 2 {
			fmt.Fprintf(&b, "picture at %9.2f: %3d lost, %3d repeated\n", e.Sec, e.Lost, e.Repeated)
		}
	}
	fmt.Fprintf(&b, "whole stream: %d frames (of %d) lost, %d repeated, in %d places\n", lost, len(ref.Frames), rep, len(ev))
	if refSound != nil && outSound != nil {
		js := SoundJumps(refSound, outSound, 4, 5)
		sum := 0.0
		for _, j := range js {
			fmt.Fprintf(&b, "sound at %9.2f: %+7.1f ms (%.1f -> %.1f, correlation %.2f)\n", j.Sec, j.ToMs-j.FromMs, j.FromMs, j.ToMs, j.Confidence)
			sum += math.Abs(j.ToMs - j.FromMs)
		}
		fmt.Fprintf(&b, "whole stream: %d sound jumps of 5 ms or more, %.1f ms in all\n", len(js), sum)
	}
	return b.String()
}
