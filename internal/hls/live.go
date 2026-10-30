package hls

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// Live sessions: a rolling stream for TV channels and tuners. A session asks
// its feed what to play now, runs FFmpeg on it, and keeps a sliding window of
// the segments that come out. Everyone on a channel reads the same files, and
// FFmpeg runs only while someone keeps asking for the playlist.

// ErrLiveBusy: the server is already converting as many streams as allowed.
var ErrLiveBusy = errors.New("the server is already converting as many streams as it is set to; try again when one has finished")

// LiveInput is one run of FFmpeg.
type LiveInput struct {
	InputArgs []string // input options and -i; a slate when Slate is set
	Seek      float64  // start this far in (files only)
	Duration  float64  // stop after this long; 0 = until the input ends
	// Stdin feeds the input (MPEG-TS from a tuner); closed when the run ends.
	Stdin       io.ReadCloser
	VideoFilter string // deinterlacing and the like
	AudioIndex  int    // ffmpeg a:N; -1 = none, silence is added
	CopyVideo   bool   // H.264 sources pass through without converting
	// Realtime: the input already arrives at viewing speed (a tuner, a live
	// address), so FFmpeg must not be paced.
	Realtime bool
	Slate    bool
	Release  func() // called when the run ends (frees a tuner)
}

// LiveFeed says what to play at a moment. An error ends the session unless
// the session is told to retry.
type LiveFeed func(ctx context.Context, at time.Time) (*LiveInput, error)

// LiveSpec describes a live session.
type LiveSpec struct {
	ID         string
	Feed       LiveFeed
	SegmentSec float64 // default 4
	WindowSec  float64 // how much stays available for pausing and rewinding; default 300
	MaxHeight  int
	Quality    ffmpeg.Quality
	AudioRate  string
	Preset     string
	IdleSec    float64 // stop this long after the last request; default 45
	// Stop, when set, is called once when the session ends for any reason.
	OnStop func()
}

type liveSeg struct {
	Seq  int
	File string
	Dur  float64
	Disc bool // a discontinuity comes before this segment
	At   time.Time
}

// LiveSession is one running channel.
type LiveSession struct {
	Spec LiveSpec
	Dir  string

	m      *Manager
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	segs   []liveSeg
	next   int // next sequence number
	disc   int // discontinuities that have left the window
	target float64
	lastAt time.Time
	err    error
	failed chan struct{} // closed when err is set
	stopd  bool
	ready  chan struct{} // closed when the first segments exist
	isRdy  bool
	runs   int
	// nextDisc: the next segment follows a change of input.
	nextDisc bool
	// runner lets tests stand in for FFmpeg.
	runner func(ctx context.Context, s *LiveSession, in *LiveInput, run int, burst bool) error
}

type liveRun struct {
	n        int
	playlist string
	seen     int
}

type liveRegistry struct {
	mu       sync.Mutex
	sessions map[string]*LiveSession
	once     sync.Once
}

func (m *Manager) liveSweep() {
	for range time.Tick(5 * time.Second) {
		m.live.mu.Lock()
		var idle []*LiveSession
		for _, s := range m.live.sessions {
			s.mu.Lock()
			if time.Since(s.lastAt).Seconds() > s.Spec.IdleSec {
				idle = append(idle, s)
			}
			s.mu.Unlock()
		}
		m.live.mu.Unlock()
		for _, s := range idle {
			m.StopLive(s.Spec.ID)
		}
	}
}

// EnsureLive returns the running session with this id, starting one if
// needed. Live sessions count against the transcode limit.
func (m *Manager) EnsureLive(spec LiveSpec) (*LiveSession, error) {
	m.live.once.Do(func() { go m.liveSweep() })
	m.live.mu.Lock()
	defer m.live.mu.Unlock()
	if m.live.sessions == nil {
		m.live.sessions = map[string]*LiveSession{}
	}
	if s, ok := m.live.sessions[spec.ID]; ok {
		s.Touch()
		return s, nil
	}
	if m.max > 0 && len(m.live.sessions) >= m.max {
		return nil, ErrLiveBusy
	}
	s := m.newLive(spec)
	m.live.sessions[spec.ID] = s
	go s.loop()
	return s, nil
}

func (m *Manager) newLive(spec LiveSpec) *LiveSession {
	if spec.SegmentSec <= 0 {
		spec.SegmentSec = 4
	}
	if spec.WindowSec <= 0 {
		spec.WindowSec = 300
	}
	if spec.IdleSec <= 0 {
		spec.IdleSec = 45
	}
	ctx, cancel := context.WithCancel(context.Background())
	dir := filepath.Join(m.root, "live-"+safeName(spec.ID))
	_ = os.RemoveAll(dir)
	_ = os.MkdirAll(dir, 0o755)
	return &LiveSession{Spec: spec, Dir: dir, m: m, ctx: ctx, cancel: cancel, lastAt: time.Now(), target: spec.SegmentSec,
		failed: make(chan struct{}), ready: make(chan struct{})}
}

// GetLive returns a running session, or nil.
func (m *Manager) GetLive(id string) *LiveSession {
	m.live.mu.Lock()
	defer m.live.mu.Unlock()
	return m.live.sessions[id]
}

// StopLive ends a session and deletes its segments.
func (m *Manager) StopLive(id string) {
	m.live.mu.Lock()
	s := m.live.sessions[id]
	delete(m.live.sessions, id)
	m.live.mu.Unlock()
	if s != nil {
		s.stop()
	}
}

// StopAllLive ends every live session (shutdown).
func (m *Manager) StopAllLive() {
	m.live.mu.Lock()
	all := m.live.sessions
	m.live.sessions = nil
	m.live.mu.Unlock()
	for _, s := range all {
		s.stop()
	}
}

// LiveCount is the number of channels running now.
func (m *Manager) LiveCount() int {
	m.live.mu.Lock()
	defer m.live.mu.Unlock()
	return len(m.live.sessions)
}

func safeName(id string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, id)
}

func (s *LiveSession) stop() {
	s.mu.Lock()
	if s.stopd {
		s.mu.Unlock()
		return
	}
	s.stopd = true
	s.mu.Unlock()
	s.cancel()
	if s.Spec.OnStop != nil {
		s.Spec.OnStop()
	}
	// FFmpeg may need a moment to let go of its files, on Windows above all.
	go func() {
		for i := 0; i < 20; i++ {
			if os.RemoveAll(s.Dir) == nil {
				return
			}
			time.Sleep(250 * time.Millisecond)
		}
	}()
}

// Touch records that someone is still watching.
func (s *LiveSession) Touch() {
	s.mu.Lock()
	s.lastAt = time.Now()
	s.mu.Unlock()
}

// Err is why the session failed, if it did.
func (s *LiveSession) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *LiveSession) fail(err error) {
	s.mu.Lock()
	if s.err == nil {
		s.err = err
		close(s.failed)
	}
	s.mu.Unlock()
}

// WaitReady blocks until there is something to play, the session fails or
// ctx ends.
func (s *LiveSession) WaitReady(ctx context.Context) error {
	select {
	case <-s.ready:
		return nil
	case <-s.failed:
		return s.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// startBurst is how many seconds a first run may run ahead of real time, so
// the viewer gets a picture right away and has something to buffer.
const startBurst = 12

// loop runs FFmpeg input after input until the session is stopped.
func (s *LiveSession) loop() {
	defer s.m.forgetFailed(s)
	retries := 0
	burst := true
	for s.ctx.Err() == nil {
		in, err := s.Spec.Feed(s.ctx, time.Now())
		if err != nil {
			if s.ctx.Err() == nil {
				s.fail(err)
			}
			return
		}
		s.mu.Lock()
		s.runs++
		run := s.runs
		s.mu.Unlock()
		start := time.Now()
		runner := s.runner
		if runner == nil {
			runner = runLiveFFmpeg
		}
		err = runner(s.ctx, s, in, run, burst && !in.Realtime)
		if in.Stdin != nil {
			_ = in.Stdin.Close()
		}
		if in.Release != nil {
			in.Release()
		}
		if s.ctx.Err() != nil {
			return
		}
		burst = false
		s.mu.Lock()
		s.nextDisc = true
		s.mu.Unlock()
		switch {
		case isFatal(err):
			s.fail(err)
			return
		case err != nil || (time.Since(start) < 2*time.Second && !in.Slate && (in.Duration == 0 || in.Duration >= 3)):
			// Failed, or ended at once. Give up after a few tries in a row: a
			// stream that does not work must not keep FFmpeg spinning.
			retries++
			if retries > 5 {
				if err == nil {
					err = errors.New("the stream ended as soon as it started")
				}
				s.fail(err)
				return
			}
			select {
			case <-s.ctx.Done():
				return
			case <-time.After(time.Duration(retries) * time.Second):
			}
		default:
			retries = 0
		}
	}
}

// FatalError: retrying cannot help (an encrypted channel, say).
type FatalError struct{ Err error }

func (e *FatalError) Error() string { return e.Err.Error() }
func (e *FatalError) Unwrap() error { return e.Err }

func isFatal(err error) bool {
	var f *FatalError
	return errors.As(err, &f)
}

// forgetFailed takes a session that ended on its own off the list after a
// while, leaving time for viewers to read why.
func (m *Manager) forgetFailed(s *LiveSession) {
	if s.Err() == nil {
		return
	}
	time.AfterFunc(10*time.Second, func() {
		m.live.mu.Lock()
		if m.live.sessions[s.Spec.ID] == s {
			delete(m.live.sessions, s.Spec.ID)
		}
		m.live.mu.Unlock()
		s.stop()
	})
}

var (
	readrateOnce sync.Once
	hasBurst     bool
)

func (m *Manager) burstSupported() bool {
	readrateOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if out, err := ffmpeg.Command(ctx, m.tools.FFmpeg, "-hide_banner", "-h", "long").Output(); err == nil {
			hasBurst = strings.Contains(string(out), "readrate_initial_burst")
		}
	})
	return hasBurst
}

// liveArgs builds one run's FFmpeg command line.
func (s *LiveSession) liveArgs(in *LiveInput, run int, burst bool) []string {
	sp := s.Spec
	t := s.m.tools
	args := []string{"-hide_banner", "-loglevel", "warning", "-nostdin", "-y", "-max_error_rate", "1"}
	plan := t.Plan(sp.Quality)
	if sp.Preset != "" && plan.Name == "libx264" && len(sp.Quality.VideoArgs) == 0 {
		q := sp.Quality
		q.Preset = sp.Preset
		plan = t.Plan(q)
	}
	if !in.CopyVideo {
		args = append(args, plan.InputArgs...)
		if plan.Name == "h264_nvenc" {
			args = append(args, "-hwaccel", "auto")
		}
	}
	if in.Stdin != nil {
		args = nostdinOff(args)
	}
	if !in.Realtime {
		if burst && s.m.burstSupported() {
			args = append(args, "-readrate", "1", "-readrate_initial_burst", strconv.Itoa(startBurst))
		} else {
			args = append(args, "-re")
		}
	}
	if in.Seek > 0 {
		args = append(args, "-ss", strconv.FormatFloat(in.Seek, 'f', 3, 64))
	}
	args = append(args, in.InputArgs...)
	silence := in.AudioIndex < 0
	if silence {
		args = append(args, "-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo")
	}
	args = append(args, "-map", "0:v:0")
	if silence {
		args = append(args, "-map", "1:a:0", "-shortest")
	} else {
		args = append(args, "-map", fmt.Sprintf("0:a:%d?", in.AudioIndex))
	}
	args = append(args, "-map_metadata", "-1", "-map_chapters", "-1", "-sn", "-dn")
	if in.CopyVideo {
		args = append(args, "-c:v", "copy")
	} else {
		var vf []string
		if in.VideoFilter != "" {
			vf = append(vf, in.VideoFilter)
		}
		vf = append(vf, "scale=iw*sar:ih")
		if sp.MaxHeight > 0 {
			vf = append(vf, fmt.Sprintf("scale=-2:'min(ih,%d)'", sp.MaxHeight))
		}
		vf = append(vf, "setsar=1", "format="+plan.PixFmt)
		if plan.Upload != "" {
			vf = append(vf, plan.Upload)
		}
		args = append(args, "-vf", strings.Join(vf, ","), "-c:v", plan.Name)
		args = append(args, plan.Args...)
		if rate := sp.Quality.MaxRate; rate != "" && len(sp.Quality.VideoArgs) == 0 {
			args = append(args, "-maxrate", rate, "-bufsize", rate)
		}
		if plan.Name == "libx264" {
			args = append(args, "-tune", "zerolatency")
		}
		args = append(args, "-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%g)", sp.SegmentSec), "-g", strconv.Itoa(int(sp.SegmentSec*30)), "-sc_threshold", "0")
	}
	// The same audio throughout, so a stream stays playable across programmes
	// and devices.
	args = append(args, "-c:a", "aac", "-ac", "2", "-ar", "48000", "-b:a", firstNonEmpty(sp.AudioRate, sp.Quality.AudioRate, "160k"), "-af", "aresample=async=1:first_pts=0")
	if in.Duration > 0 {
		args = append(args, "-t", strconv.FormatFloat(in.Duration, 'f', 3, 64))
	}
	args = append(args, sp.Quality.OutputArgs...)
	args = append(args,
		"-f", "hls",
		"-hls_time", strconv.FormatFloat(sp.SegmentSec, 'f', 3, 64),
		"-hls_segment_type", "mpegts",
		"-hls_list_size", "0",
		"-hls_flags", "temp_file+independent_segments",
		"-start_number", "0",
		"-hls_segment_filename", filepath.Join(s.Dir, fmt.Sprintf("r%05d-%%05d.ts", run)),
		filepath.Join(s.Dir, fmt.Sprintf("r%05d.m3u8", run)),
	)
	return args
}

// nostdinOff: FFmpeg must read its input from standard input.
func nostdinOff(args []string) []string {
	for i, a := range args {
		if a == "-nostdin" {
			return append(args[:i:i], args[i+1:]...)
		}
	}
	return args
}

// Hidden from the log: a stream address can carry a login.
var urlCreds = regexp.MustCompile(`([a-z0-9+]+://)[^/@\s]+@`)

func redactArgs(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if i > 0 && args[i-1] == "-headers" {
			a = "(hidden)"
		}
		out[i] = urlCreds.ReplaceAllString(a, "$1(hidden)@")
	}
	return strings.Join(out, " ")
}

func runLiveFFmpeg(ctx context.Context, s *LiveSession, in *LiveInput, run int, burst bool) error {
	args := s.liveArgs(in, run, burst)
	cmd := ffmpeg.Command(ctx, s.m.tools.FFmpeg, args...)
	var stderr tailBuffer
	cmd.Stderr = &stderr
	if in.Stdin != nil {
		cmd.Stdin = in.Stdin
		// The copy to FFmpeg's input can block on a silent tuner; do not wait for it.
		cmd.WaitDelay = 3 * time.Second
	}
	s.m.log.Info("ffmpeg start", "cat", "ffmpeg", "live", s.Spec.ID, "run", run, "args", redactArgs(args))
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	r := &liveRun{n: run, playlist: filepath.Join(s.Dir, fmt.Sprintf("r%05d.m3u8", run))}
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-done:
			s.readRun(r)
			if ctx.Err() != nil {
				return nil
			}
			if err != nil {
				if f := in.fatalCause(); f != nil {
					return &FatalError{f}
				}
				return fmt.Errorf("%w: %s", err, tail(stderr.String(), 400))
			}
			if f := in.fatalCause(); f != nil {
				return &FatalError{f}
			}
			return nil
		case <-tick.C:
			s.readRun(r)
		}
	}
}

// fatalCause lets a tuner's stream say why it ended, when it refused to go on.
func (in *LiveInput) fatalCause() error {
	if c, ok := in.Stdin.(interface{ Fatal() error }); ok {
		return c.Fatal()
	}
	return nil
}

type tailBuffer struct {
	mu sync.Mutex
	b  []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if len(t.b) > 8192 {
		t.b = t.b[len(t.b)-4096:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.b)
}

// readRun picks up the segments FFmpeg has finished.
func (s *LiveSession) readRun(r *liveRun) {
	f, err := os.Open(r.playlist)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	n := 0
	var dur float64
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "#EXTINF:"):
			v := strings.TrimSuffix(strings.TrimPrefix(line, "#EXTINF:"), ",")
			dur, _ = strconv.ParseFloat(v, 64)
		case line != "" && !strings.HasPrefix(line, "#"):
			if n >= r.seen && dur > 0 {
				s.addSegment(filepath.Base(line), dur)
				r.seen = n + 1
			}
			n++
			dur = 0
		}
	}
}

func (s *LiveSession) addSegment(file string, dur float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.segs = append(s.segs, liveSeg{Seq: s.next, File: file, Dur: dur, Disc: s.nextDisc, At: time.Now()})
	s.nextDisc = false
	s.next++
	if d := math.Ceil(dur); d > s.target {
		s.target = d
	}
	// Slide the window.
	var total float64
	for _, x := range s.segs {
		total += x.Dur
	}
	drop := 0
	for total > s.Spec.WindowSec && drop < len(s.segs)-3 {
		total -= s.segs[drop].Dur
		_ = os.Remove(filepath.Join(s.Dir, s.segs[drop].File))
		drop++
		// The first segment of a playlist carries no discontinuity tag, so
		// the count moves on as soon as the tag has nothing before it.
		if s.segs[drop].Disc {
			s.segs[drop].Disc = false
			s.disc++
		}
	}
	s.segs = s.segs[drop:]
	if !s.isRdy && len(s.segs) >= 2 {
		s.isRdy = true
		close(s.ready)
	}
}

// Playlist renders the live playlist. Nothing marks it as ended: the player
// keeps asking, and each request keeps the session alive.
func (s *LiveSession) Playlist(segURL func(file string) string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastAt = time.Now()
	var b strings.Builder
	first := s.next
	if len(s.segs) > 0 {
		first = s.segs[0].Seq
	}
	fmt.Fprintf(&b, "#EXTM3U\n#EXT-X-VERSION:6\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:%d\n#EXT-X-DISCONTINUITY-SEQUENCE:%d\n#EXT-X-INDEPENDENT-SEGMENTS\n",
		int(math.Ceil(s.target)), first, s.disc)
	for i, x := range s.segs {
		if x.Disc && i > 0 {
			b.WriteString("#EXT-X-DISCONTINUITY\n")
		}
		fmt.Fprintf(&b, "#EXTINF:%.3f,\n%s\n", x.Dur, segURL(x.File))
	}
	return []byte(b.String())
}

// SegmentPath finds a segment still in the window, by name.
func (s *LiveSession) SegmentPath(file string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastAt = time.Now()
	for _, x := range s.segs {
		if x.File == file {
			return filepath.Join(s.Dir, file), true
		}
	}
	return "", false
}

// WindowSeconds is how much can be rewound right now.
func (s *LiveSession) WindowSeconds() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var total float64
	for _, x := range s.segs {
		total += x.Dur
	}
	return total
}

// SlateInput is a plain card with silence, for the gaps between programmes.
// The player draws what to say over it.
func SlateInput() []string {
	return []string{"-f", "lavfi", "-i", "color=c=0x0b1f5c:s=1280x720:r=25"}
}
