// Package hls runs FFmpeg transcodes that produce HLS segments on demand.
//
// The server writes the playlist itself from the known duration, forces
// keyframes on segment boundaries and (re)starts FFmpeg at whichever segment a
// client asks for. That gives instant seeking anywhere without transcoding
// the whole file first.
package hls

import (
	"bytes"
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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// Spec describes one transcode.
type Spec struct {
	ID          string
	InputArgs   []string // input options and -i
	Duration    float64  // total seconds (required)
	VideoFilter string   // extra -vf chain (deinterlace etc.); aspect normalisation is added automatically
	AudioIndex  int      // ffmpeg audio stream index (a:N), -1 = no audio
	// AudioInput is the input (of those InputArgs opens) that has the audio;
	// 0 = the same one as the video. With a second input every input seeks,
	// and Duration limits the output, since the picture may be an endless
	// generated one or the sound may loop.
	AudioInput    int
	AudioChannels int    // 0 = keep (max 6)
	AudioFilter   string // extra -af chain, before the output's own (karaoke: leave out the vocals)
	SubtitleIndex int    // s:N to burn in, -1 = none
	AudioCodec    string // aac (default) | mp3
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
	stderr   bytes.Buffer
	lastSeg  int
}

// Session is a running or idle transcode.
type Session struct {
	Spec     Spec
	Dir      string
	mu       sync.Mutex
	job      *job
	lastUsed time.Time
	used     atomic.Int64 // lastUsed as Unix nanoseconds, for the manager to read without taking mu
	tools    *ffmpeg.Tools
	log      *slog.Logger
	m        *Manager
}

// Manager owns sessions and the temp directory.
type Manager struct {
	tools   *ffmpeg.Tools
	root    string
	log     *slog.Logger
	mu      sync.Mutex
	session map[string]*Session
	max     int // FFmpeg processes running at once

	// running is every FFmpeg process. jobMu is taken after a session's mu,
	// never before it.
	jobMu   sync.Mutex
	running map[*job]*Session
}

// ErrBusy: as many transcodes as the server allows are all being watched.
var ErrBusy = errors.New("the server is busy with other streams")

const (
	// maxSessions caps known streams; each has a folder, usually with segments.
	maxSessions = 200
	// A stream nobody asked anything of for this long gives way to a new one.
	// (A player asks for a segment every few seconds, and FFmpeg picks up from
	// wherever it asks next.)
	evictAfter = 8 * time.Second
)

func NewManager(tools *ffmpeg.Tools, root string, max int, log *slog.Logger) *Manager {
	if max < 1 {
		max = 1
	}
	m := &Manager{tools: tools, root: root, log: log, session: map[string]*Session{}, max: max, running: map[*job]*Session{}}
	_ = os.RemoveAll(root)
	_ = os.MkdirAll(root, 0o755)
	go m.sweep()
	return m
}

func (m *Manager) sweep() {
	for range time.Tick(30 * time.Second) {
		m.mu.Lock()
		for id, s := range m.session {
			s.mu.Lock()
			idle := time.Since(s.lastUsed)
			s.mu.Unlock()
			if idle > 10*time.Minute {
				s.stop()
				_ = os.RemoveAll(s.Dir)
				delete(m.session, id)
			} else if idle > 90*time.Second {
				s.pause()
			}
		}
		m.mu.Unlock()
	}
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
	if len(m.session) >= maxSessions {
		m.dropOldestLocked()
	}
	if spec.SegmentSec <= 0 {
		spec.SegmentSec = 4
		if spec.LowLatency {
			spec.SegmentSec = 2
		}
	}
	s := &Session{Spec: spec, Dir: filepath.Join(m.root, spec.ID), lastUsed: time.Now(), tools: m.tools, log: m.log.With("hls", spec.ID), m: m}
	s.used.Store(time.Now().UnixNano())
	_ = os.MkdirAll(s.Dir, 0o755)
	m.session[spec.ID] = s
	return s
}

// dropOldestLocked ends the least recently used stream. Call with m.mu held.
func (m *Manager) dropOldestLocked() {
	var oldest *Session
	var id string
	for k, s := range m.session {
		if oldest == nil || s.used.Load() < oldest.used.Load() {
			oldest, id = s, k
		}
	}
	if oldest != nil {
		oldest.stop()
		_ = os.RemoveAll(oldest.Dir)
		delete(m.session, id)
	}
}

// admitLocked makes room for one more FFmpeg process, stopping the one that
// has gone unused longest if it must. Call with m.jobMu held.
func (m *Manager) admitLocked(self *Session) bool {
	if len(m.running) < m.max {
		return true
	}
	var oldest *job
	var at int64
	now := time.Now().UnixNano()
	for j, s := range m.running {
		used := s.used.Load()
		if s != self && now-used > int64(evictAfter) && (oldest == nil || used < at) {
			oldest, at = j, used
		}
	}
	if oldest == nil {
		return false
	}
	if oldest.cmd.Process != nil {
		_ = oldest.cmd.Process.Kill()
	}
	delete(m.running, oldest)
	return true
}

// Stop ends and deletes a session.
func (m *Manager) Stop(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.session[id]; ok {
		s.stop()
		_ = os.RemoveAll(s.Dir)
		delete(m.session, id)
	}
}

// StopAll ends every session (shutdown).
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.session {
		s.stop()
		_ = os.RemoveAll(s.Dir)
		delete(m.session, id)
	}
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
	now := time.Now()
	s.used.Store(now.UnixNano())
	s.mu.Lock()
	s.lastUsed = now
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
	p := s.segPath(n)
	if fileReady(p) {
		return p, nil
	}
	s.mu.Lock()
	j := s.job
	// A stream that holds its last frame (DVD menus and interactive clips) is
	// never entered part-way: such clips often carry a second of video and
	// then minutes of sound over that one picture (a credits card, a game
	// screen), so a seek past the last frame leaves nothing to show and the
	// segments come out empty. It is converted from the start, which is
	// quick for these short, simple clips, and later segments wait for it.
	from := n
	ahead := n > j.lastProducedOr(-1)+6
	if s.Spec.HoldLastFrame {
		from, ahead = 0, false
	}
	needStart := j == nil || j.finished() || n < j.startSeg || ahead
	if needStart {
		if j != nil {
			s.stopLocked()
		}
		var err error
		j, err = s.startLocked(from)
		if err != nil {
			s.mu.Unlock()
			return "", err
		}
	}
	s.mu.Unlock()

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

// Warm starts FFmpeg on the first segment in the background (if nothing has
// been produced yet) so a stream the viewer is about to see is ready sooner.
func (s *Session) Warm() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job != nil || fileReady(s.segPath(0)) {
		return
	}
	if _, err := s.startLocked(0); err != nil {
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

func (j *job) lastProduced() int { return j.lastSeg }

// lastProducedOr is lastProduced for a job that may not exist.
func (j *job) lastProducedOr(none int) int {
	if j == nil {
		return none
	}
	return j.lastSeg
}

func (s *Session) startLocked(fromSeg int) (*job, error) {
	sp := s.Spec
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
	if start > 0 && sp.AudioInput > 0 {
		for _, a := range sp.InputArgs {
			if a == "-i" {
				args = append(args, "-ss", strconv.FormatFloat(start, 'f', 3, 64))
			}
			args = append(args, a)
		}
	} else {
		if start > 0 {
			args = append(args, "-ss", strconv.FormatFloat(start, 'f', 3, 64))
		}
		args = append(args, sp.InputArgs...)
	}
	args = append(args, "-map", "0:v:0")
	if sp.AudioIndex >= 0 {
		args = append(args, "-map", fmt.Sprintf("%d:a:%d?", sp.AudioInput, sp.AudioIndex))
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
		if sp.AudioFilter != "" {
			af = sp.AudioFilter + "," + af
		}
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
	if (sp.HoldLastFrame || sp.AudioInput > 0) && sp.Duration > start {
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
	j := &job{startSeg: fromSeg, cmd: cmd, done: make(chan struct{}), lastSeg: fromSeg - 1}
	cmd.Stderr = &j.stderr
	s.log.Info("ffmpeg start", "cat", "ffmpeg", "hls", s.Spec.ID, "from", fromSeg, "args", strings.Join(args, " "))
	s.m.jobMu.Lock()
	if !s.m.admitLocked(s) {
		s.m.jobMu.Unlock()
		return nil, ErrBusy
	}
	err := cmd.Start()
	if err == nil {
		s.m.running[j] = s
	}
	s.m.jobMu.Unlock()
	if err != nil {
		return nil, err
	}
	s.job = j
	go func() {
		j.err = cmd.Wait()
		s.m.jobMu.Lock()
		delete(s.m.running, j)
		s.m.jobMu.Unlock()
		close(j.done)
		if j.err != nil && !strings.Contains(j.err.Error(), "killed") && !strings.Contains(j.err.Error(), "exit status 255") {
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
		for n := j.lastSeg + 1; n < s.SegmentCount(); n++ {
			if fileReady(s.segPath(n)) {
				j.lastSeg = n
			} else {
				break
			}
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
	s.m.jobMu.Lock()
	delete(s.m.running, j)
	s.m.jobMu.Unlock()
	if !j.finished() && j.cmd.Process != nil {
		_ = j.cmd.Process.Kill()
	}
}

func (s *Session) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
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
