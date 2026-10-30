// Package hls runs FFmpeg transcodes that produce HLS segments on demand.
//
// The server writes the playlist itself from the known duration, forces
// keyframes on segment boundaries and (re)starts FFmpeg at whichever segment a
// client asks for. That gives instant seeking anywhere without transcoding
// the whole file first.
package hls

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// Spec describes one transcode.
type Spec struct {
	ID            string
	InputArgs     []string // input options and -i
	Duration      float64  // total seconds (required)
	VideoFilter   string   // extra -vf chain (deinterlace etc.); aspect normalisation is added automatically
	AudioIndex    int      // ffmpeg audio stream index (a:N), -1 = no audio
	AudioChannels int      // 0 = keep (max 6)
	SubtitleIndex int      // s:N to burn in, -1 = none
	AudioCodec    string   // aac (default) | mp3
	SegmentSec    float64
	MaxHeight     int    // 0 = source
	VideoBitrate  string // e.g. "8M"; empty = quality mode
	// Quality is the encoder choice and quality profile from Settings.
	Quality    ffmpeg.Quality
	LowLatency bool // menus: tiny segments, zerolatency
	CopyVideo  bool // remux (h264 sources); keyframe alignment is approximate
	// Preset overrides the libx264 preset (e.g. "superfast" for DVD menus or
	// low-power machines). Ignored by hardware encoders.
	Preset string
	// HoldLastFrame pads video (and audio) to Duration by repeating the last
	// frame. DVD menus and transitions are often a single still picture
	// shown for many seconds; without this the stream would be one frame
	// long and players stall waiting for more.
	HoldLastFrame bool
}

type job struct {
	startSeg int
	cmd      *exec.Cmd
	done     chan struct{}
	err      error
	stderr   tailBuffer
	lastSeg  atomic.Int64 // the newest finished segment, kept current by watch
	stopped  atomic.Bool  // ended on purpose: how it exits says nothing
}

// tailBuffer keeps the end of what is written to it: FFmpeg can warn for
// every frame of a damaged file for hours, and only the end explains a failure.
type tailBuffer struct {
	mu sync.Mutex
	b  []byte
}

const stderrTail = 16 << 10

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if len(t.b) > 2*stderrTail {
		t.b = append(t.b[:0], t.b[len(t.b)-stderrTail:]...)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.b)
}

// Session is a running or idle transcode.
type Session struct {
	Spec     Spec
	Dir      string
	mu       sync.Mutex
	job      *job
	lastUsed time.Time
	tools    *ffmpeg.Tools
	log      *slog.Logger
	m        *Manager
	// lastAsked is the newest segment a viewer asked for; FFmpeg rests once it
	// is far ahead of this.
	lastAsked atomic.Int64
	prunedTo  int
	lastPrune time.Time
	// viewers: who has been watching lately (several accounts can share one
	// stream), so one of them leaving doesn't end it for the others.
	viewers map[string]time.Time
}

// Manager owns sessions and the temp directory.
type Manager struct {
	tools   *ffmpeg.Tools
	root    string
	log     *slog.Logger
	mu      sync.Mutex
	session map[string]*Session
	max     int
}

// ErrBusy: as many transcodes are running as the server allows
// (-max-transcodes), and all of them have someone watching.
var ErrBusy = errors.New("this server is busy with other streams; try again in a moment")

const (
	// A viewer who has asked for nothing for this long gives up their place
	// when someone else needs to start a transcode (they restart on return).
	yieldAfter = 20 * time.Second
	pauseAfter = 90 * time.Second
	// Finished segments stay this long after the last request, for a viewer
	// who comes back; the session itself is kept longer (it costs next to
	// nothing), so a film paused for hours still plays when resumed.
	cleanAfter  = 10 * time.Minute
	forgetAfter = 24 * time.Hour
	// A fast machine can convert a film at many times its speed, taking every
	// core and disk space for parts nobody may watch. FFmpeg stops when it is
	// this many segments ahead of the viewer and starts again as they near it.
	maxAheadSegs = 90
	wakeSegs     = 30
	// Segments the viewer passed this long ago are removed.
	keepBehindSegs = 150
	pruneEvery     = 30 * time.Second
)

// sessionDirRe matches the folders this package makes (see the IDs in
// api_play.go, dvdsession and bdsession).
var sessionDirRe = regexp.MustCompile(`^(v|dvd|bd)-[0-9a-zA-Z-]+$`)

func NewManager(tools *ffmpeg.Tools, root string, max int, log *slog.Logger) *Manager {
	m := &Manager{tools: tools, root: root, log: log, session: map[string]*Session{}, max: max}
	cleanRoot(root)
	_ = os.MkdirAll(root, 0o755)
	go m.sweep()
	return m
}

// cleanRoot removes what an earlier run left: only folders of the kind this
// package makes. The transcode folder is the owner's choice and could hold
// anything else.
func cleanRoot(root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && sessionDirRe.MatchString(e.Name()) {
			_ = os.RemoveAll(filepath.Join(root, e.Name()))
		}
	}
}

func (m *Manager) sweep() {
	for range time.Tick(30 * time.Second) {
		m.sweepOnce()
	}
}

func (m *Manager) sweepOnce() {
	var clean []*Session
	m.mu.Lock()
	for id, s := range m.session {
		s.mu.Lock()
		idle := time.Since(s.lastUsed)
		s.mu.Unlock()
		switch {
		case idle > forgetAfter:
			clean = append(clean, s)
			delete(m.session, id)
		case idle > cleanAfter:
			clean = append(clean, s)
		case idle > pauseAfter:
			s.pause()
		}
	}
	m.mu.Unlock()
	for _, s := range clean { // waiting for FFmpeg to go must not hold up other streams
		s.stopAndClean()
	}
}

// makeRoom is called before starting FFmpeg for session s. When the limit is
// reached, the transcode nobody has asked anything of for the longest time
// gives way; if everyone is watching, s has to wait.
func (m *Manager) makeRoom(s *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	running := 0
	var oldest *Session
	var oldestUsed time.Time
	for _, o := range m.session {
		if o == s {
			continue
		}
		o.mu.Lock()
		active, used := o.job != nil && !o.job.finished(), o.lastUsed
		o.mu.Unlock()
		if active {
			running++
			if oldest == nil || used.Before(oldestUsed) {
				oldest, oldestUsed = o, used
			}
		}
	}
	if running < m.max {
		return nil
	}
	if time.Since(oldestUsed) > yieldAfter {
		m.log.Info("transcode limit reached: pausing an idle stream", "cat", "ffmpeg", "hls", oldest.Spec.ID, "idle", time.Since(oldestUsed).Round(time.Second))
		oldest.stop()
		return nil
	}
	return ErrBusy
}

// Get returns a session by id.
func (m *Manager) Get(id string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.session[id]
}

// Ensure returns the session with this spec id, creating it if needed.
func (m *Manager) Ensure(spec Spec) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.session[spec.ID]; ok {
		return s
	}
	if spec.SegmentSec <= 0 {
		spec.SegmentSec = 4
		if spec.LowLatency {
			spec.SegmentSec = 2
		}
	}
	s := &Session{Spec: spec, Dir: filepath.Join(m.root, spec.ID), lastUsed: time.Now(), tools: m.tools, log: m.log.With("hls", spec.ID), m: m, viewers: map[string]time.Time{}}
	_ = os.MkdirAll(s.Dir, 0o755)
	m.session[spec.ID] = s
	return s
}

// Stop ends and deletes a session.
func (m *Manager) Stop(id string) {
	m.mu.Lock()
	s, ok := m.session[id]
	delete(m.session, id)
	m.mu.Unlock()
	if ok {
		s.stopAndClean()
	}
}

// Leave says one viewer (any string that tells devices apart) is done with a
// stream; it stops when nobody else has used it lately.
func (m *Manager) Leave(id, viewer string) {
	s := m.Get(id)
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.viewers, viewer)
	others := false
	for _, seen := range s.viewers {
		others = others || time.Since(seen) < pauseAfter
	}
	s.mu.Unlock()
	if !others {
		m.Stop(id)
	}
}

// StopAll ends every session (shutdown).
func (m *Manager) StopAll() {
	m.mu.Lock()
	all := m.session
	m.session = map[string]*Session{}
	m.mu.Unlock()
	for _, s := range all {
		s.stopAndClean()
	}
}

// See records who is using a stream.
func (s *Session) See(viewer string) {
	s.mu.Lock()
	s.viewers[viewer] = time.Now()
	s.lastUsed = time.Now()
	s.mu.Unlock()
}

// SegmentCount is the number of segments for the duration.
func (s *Session) SegmentCount() int {
	n := int(s.Spec.Duration / s.Spec.SegmentSec)
	if float64(n)*s.Spec.SegmentSec < s.Spec.Duration-0.05 {
		n++
	}
	if n < 1 {
		n = 1
	}
	return n
}

// Playlist renders the VOD playlist.
func (s *Session) Playlist(segURL func(n int) string) []byte {
	s.touch()
	var b strings.Builder
	n := s.SegmentCount()
	fmt.Fprintf(&b, "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-INDEPENDENT-SEGMENTS\n", int(s.Spec.SegmentSec+0.999))
	for i := 0; i < n; i++ {
		d := s.Spec.SegmentSec
		if i == n-1 {
			d = s.Spec.Duration - float64(i)*s.Spec.SegmentSec
			if d <= 0 {
				d = s.Spec.SegmentSec
			}
		}
		fmt.Fprintf(&b, "#EXTINF:%.3f,\n%s\n", d, segURL(i))
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	return []byte(b.String())
}

func (s *Session) touch() {
	s.mu.Lock()
	s.lastUsed = time.Now()
	s.mu.Unlock()
}

func (s *Session) segPath(n int) string { return filepath.Join(s.Dir, fmt.Sprintf("seg%05d.ts", n)) }

// Segment returns the path of segment n, starting or restarting FFmpeg as
// needed and waiting until the file is complete.
func (s *Session) Segment(ctx context.Context, n int) (string, error) {
	s.touch()
	if n < 0 || n >= s.SegmentCount() {
		return "", errors.New("segment out of range")
	}
	s.lastAsked.Store(int64(n))
	p := s.segPath(n)
	if fileReady(p) {
		s.prune(n)
		s.wake(n)
		return p, nil
	}
	// A stream that holds its last frame (DVD menus and interactive clips) is
	// never entered part-way: such clips often carry a second of video and
	// then minutes of sound over that one picture (a credits card, a game
	// screen), so a seek past the last frame leaves nothing to show and the
	// segments come out empty. It is converted from the start, which is
	// quick for these short, simple clips, and later segments wait for it.
	from := n
	if s.Spec.HoldLastFrame {
		from = 0
	}
	j, err := s.ensureJob(n, from)
	if err != nil {
		return "", err
	}

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if fileReady(p) {
			return p, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-j.done:
			if fileReady(p) {
				return p, nil
			}
			msg := strings.TrimSpace(j.stderr.String())
			if len(msg) > 2000 {
				msg = msg[len(msg)-2000:]
			}
			return "", fmt.Errorf("transcode ended before segment %d: %s", n, msg)
		case <-time.After(100 * time.Millisecond):
		}
	}
	return "", errors.New("timed out waiting for segment")
}

// wantsStart: the running job (if any) won't reach segment n soon.
func (s *Session) wantsStart(n int) bool {
	j := s.job
	if j == nil || j.finished() || n < j.startSeg {
		return true
	}
	return !s.Spec.HoldLastFrame && n > int(j.lastSeg.Load())+6
}

// ensureJob returns a job that will produce segment n, starting FFmpeg at
// segment from if there is none.
func (s *Session) ensureJob(n, from int) (*job, error) {
	s.mu.Lock()
	if !s.wantsStart(n) {
		defer s.mu.Unlock()
		return s.job, nil
	}
	s.stopLocked()
	s.mu.Unlock()
	// Without the session's lock: makeRoom looks at the other sessions.
	if err := s.m.makeRoom(s); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.wantsStart(n) { // another request for this stream got here first
		return s.job, nil
	}
	s.stopLocked()
	return s.startLocked(from)
}

// wake starts FFmpeg again when it rested (see maxAheadSegs, and pause) and
// the viewer is nearing the end of what is ready.
func (s *Session) wake(n int) {
	if s.Spec.HoldLastFrame {
		return
	}
	s.mu.Lock()
	idle := s.job == nil // a job that ended by itself reached the end, or failed
	s.mu.Unlock()
	if !idle {
		return
	}
	for i, end := n+1, min(n+wakeSegs, s.SegmentCount()); i < end; i++ {
		if !fileReady(s.segPath(i)) {
			if _, err := s.ensureJob(i, i); err != nil {
				s.log.Debug("resume failed", "err", err)
			}
			return
		}
	}
}

// prune removes segments far behind the viewer, so a long film doesn't fill a
// small disk with what was watched an hour ago.
func (s *Session) prune(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.lastPrune) < pruneEvery {
		return
	}
	s.lastPrune = time.Now()
	for ; s.prunedTo < n-keepBehindSegs; s.prunedTo++ {
		_ = os.Remove(s.segPath(s.prunedTo))
	}
}

// Warm starts FFmpeg on the first segment in the background (if nothing has
// been produced yet) so a stream the viewer is about to see is ready sooner.
func (s *Session) Warm() {
	s.mu.Lock()
	busy := s.job != nil || fileReady(s.segPath(0))
	s.mu.Unlock()
	if busy {
		return
	}
	if _, err := s.ensureJob(0, 0); err != nil {
		s.log.Debug("warm failed", "err", err)
	}
}

func fileReady(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Size() > 0
}

func (j *job) finished() bool {
	select {
	case <-j.done:
		return true
	default:
		return false
	}
}

func (s *Session) startLocked(fromSeg int) (*job, error) {
	sp := s.Spec
	_ = os.MkdirAll(s.Dir, 0o755) // the segments of an idle session may have been cleaned up
	start := float64(fromSeg) * sp.SegmentSec
	// Damaged discs and rips often carry runs of broken audio frames; ffmpeg
	// would abort once 2/3 of the frames failed to decode and leave the
	// stream unfinished, so decode errors never end a transcode.
	args := []string{"-hide_banner", "-loglevel", "warning", "-nostdin", "-y", "-max_error_rate", "1"}
	plan := s.tools.Plan(sp.Quality)
	if sp.Preset != "" && plan.Name == "libx264" && len(sp.Quality.VideoArgs) == 0 {
		q := sp.Quality
		q.Preset = sp.Preset
		plan = s.tools.Plan(q)
	}
	if !sp.CopyVideo {
		args = append(args, plan.InputArgs...)
		if plan.Name == "h264_nvenc" {
			args = append(args, "-hwaccel", "auto")
		}
	}
	if start > 0 {
		args = append(args, "-ss", strconv.FormatFloat(start, 'f', 3, 64))
	}
	args = append(args, sp.InputArgs...)
	args = append(args, "-map", "0:v:0")
	if sp.AudioIndex >= 0 {
		args = append(args, "-map", fmt.Sprintf("0:a:%d?", sp.AudioIndex))
	}
	args = append(args, "-map_metadata", "-1", "-map_chapters", "-1")

	// Video
	if sp.CopyVideo {
		args = append(args, "-c:v", "copy")
	} else {
		var vf []string
		if sp.VideoFilter != "" {
			vf = append(vf, sp.VideoFilter)
		}
		vf = append(vf, "scale=iw*sar:ih")
		if sp.MaxHeight > 0 {
			vf = append(vf, fmt.Sprintf("scale=-2:'min(ih,%d)'", sp.MaxHeight))
		}
		if sp.HoldLastFrame {
			vf = append(vf, fmt.Sprintf("tpad=stop_mode=clone:stop_duration=%.3f", sp.Duration))
		}
		vf = append(vf, "setsar=1", "format="+plan.PixFmt)
		if plan.Upload != "" {
			vf = append(vf, plan.Upload)
		}
		if sp.SubtitleIndex >= 0 {
			// Burn bitmap/text subtitles in via a filter graph.
			graph := fmt.Sprintf("[0:v:0]%s[v0];[0:s:%d]scale=iw:ih[s0];[v0][s0]overlay=shortest=0:eof_action=pass[vout]", strings.Join(vf, ","), sp.SubtitleIndex)
			args = append(args, "-filter_complex", graph, "-map", "-0:v:0", "-map", "[vout]")
		} else {
			args = append(args, "-vf", strings.Join(vf, ","))
		}
		enc := plan.Name
		args = append(args, "-c:v", enc)
		args = append(args, plan.Args...)
		if rate := firstNonEmpty(sp.VideoBitrate, sp.Quality.MaxRate); rate != "" && len(sp.Quality.VideoArgs) == 0 {
			args = append(args, "-maxrate", rate, "-bufsize", rate)
		}
		if sp.LowLatency && enc == "libx264" {
			args = append(args, "-tune", "zerolatency")
		}
		args = append(args, "-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%g)", sp.SegmentSec), "-g", strconv.Itoa(int(sp.SegmentSec*30)), "-sc_threshold", "0")
	}
	// Audio
	if sp.AudioIndex >= 0 {
		ch := sp.AudioChannels
		codec := "aac"
		if sp.AudioCodec == "mp3" {
			codec = "libmp3lame"
			ch = 2
		}
		args = append(args, "-c:a", codec)
		af := "aresample=async=1:first_pts=0"
		if sp.HoldLastFrame {
			af += ",apad"
		}
		switch {
		case ch == 2 || ch == 0:
			args = append(args, "-ac", "2", "-b:a", firstNonEmpty(sp.Quality.AudioRate, "192k"), "-af", af)
		default:
			args = append(args, "-ac", strconv.Itoa(ch), "-b:a", "384k", "-af", af)
		}
	} else {
		args = append(args, "-an")
	}
	if sp.HoldLastFrame && sp.Duration > start {
		args = append(args, "-t", strconv.FormatFloat(sp.Duration-start, 'f', 3, 64))
	}
	args = append(args, sp.Quality.OutputArgs...)
	args = append(args,
		"-sn",
		"-output_ts_offset", strconv.FormatFloat(start, 'f', 3, 64),
		"-f", "hls",
		"-hls_time", strconv.FormatFloat(sp.SegmentSec, 'f', 3, 64),
		"-hls_segment_type", "mpegts",
		"-hls_list_size", "0",
		"-hls_flags", "temp_file+independent_segments",
		"-start_number", strconv.Itoa(fromSeg),
		"-hls_segment_filename", filepath.Join(s.Dir, "seg%05d.ts"),
		filepath.Join(s.Dir, fmt.Sprintf("ffmpeg-%d.m3u8", fromSeg)),
	)
	cmd := ffmpeg.Command(context.Background(), s.tools.FFmpeg, args...)
	j := &job{startSeg: fromSeg, cmd: cmd, done: make(chan struct{})}
	j.lastSeg.Store(int64(fromSeg) - 1)
	cmd.Stderr = &j.stderr
	s.log.Info("ffmpeg start", "cat", "ffmpeg", "hls", s.Spec.ID, "from", fromSeg, "args", strings.Join(args, " "))
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	s.job = j
	go func() {
		j.err = cmd.Wait()
		close(j.done)
		if j.err != nil && !j.stopped.Load() {
			s.log.Warn("ffmpeg exited", "err", j.err, "stderr", tail(j.stderr.String(), 800))
		}
	}()
	go s.watch(j)
	return j, nil
}

// watch tracks the newest finished segment so Segment() can decide whether a
// request is "coming soon" or needs a restart.
func (s *Session) watch(j *job) {
	for {
		select {
		case <-j.done:
			return
		case <-time.After(250 * time.Millisecond):
		}
		for n := int(j.lastSeg.Load()) + 1; n < s.SegmentCount(); n++ {
			if fileReady(s.segPath(n)) {
				j.lastSeg.Store(int64(n))
			} else {
				break
			}
		}
		// Far enough ahead of the viewer: rest (wake starts it again).
		if !s.Spec.HoldLastFrame && j.lastSeg.Load()-s.lastAsked.Load() > maxAheadSegs {
			s.mu.Lock()
			if s.job == j {
				s.log.Debug("resting: far ahead of the viewer", "cat", "ffmpeg", "at", j.lastSeg.Load())
				s.stopLocked()
			}
			s.mu.Unlock()
			return
		}
	}
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

func (s *Session) stopLocked() {
	if s.job == nil {
		return
	}
	j := s.job
	s.job = nil
	if !j.finished() && j.cmd.Process != nil {
		j.stopped.Store(true)
		_ = j.cmd.Process.Kill()
	}
}

func (s *Session) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
}

// stopAndClean ends FFmpeg and removes the segments. It waits for the process
// to go: an open file can't be deleted on Windows, and a process that is
// still writing would put files back in the folder.
func (s *Session) stopAndClean() {
	s.mu.Lock()
	j := s.job
	s.stopLocked()
	s.prunedTo = 0
	s.mu.Unlock()
	if j != nil {
		select {
		case <-j.done:
		case <-time.After(3 * time.Second):
		}
	}
	_ = os.RemoveAll(s.Dir)
}

// pause stops FFmpeg but keeps produced segments so playback can resume.
func (s *Session) pause() { s.stop() }

// Active reports whether FFmpeg is currently running for this session.
func (s *Session) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.job != nil && !s.job.finished()
}

// keySecret is chosen afresh at every start: stream IDs built from it can't
// be guessed from a file's path, so only someone who was handed a stream
// (after the access checks) can fetch it.
var keySecret = func() []byte {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return b
}()

// Key names a source file in stream IDs: the same file gives the same key
// (so viewers share one transcode) while the server runs.
func Key(path string) string {
	m := hmac.New(sha256.New, keySecret)
	m.Write([]byte(path))
	return hex.EncodeToString(m.Sum(nil)[:12])
}
