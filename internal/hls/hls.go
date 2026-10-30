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
	// SubtitleText: that stream is text (SubRip, ASS), which the picture-overlay
	// way of burning in draws nothing for; it goes through the subtitles filter,
	// which reads Source itself.
	SubtitleText bool
	AudioCodec   string // aac (default) | mp3
	SegmentSec   float64
	MaxHeight    int    // 0 = source
	VideoBitrate string // e.g. "8M"; empty = quality mode
	// Quality is the encoder choice and quality profile from Settings.
	Quality    ffmpeg.Quality
	LowLatency bool // menus: tiny segments, zerolatency
	CopyVideo  bool // remux; see Growing
	CopyAudio  bool // with CopyVideo: the sound goes through unchanged too
	// FMP4 stores segments as fragmented MP4 instead of MPEG-TS: what HEVC
	// needs in HLS. VideoTag sets the MP4 sample tag ("hvc1" for Apple players).
	FMP4     bool
	VideoTag string
	// Growing serves FFmpeg's own playlist, which grows while it runs, instead
	// of one written ahead from the duration. A copied stream can only be cut
	// at the file's keyframes, so its segment lengths aren't known before it
	// has been read. It starts at StartSec and can't be entered part-way:
	// clients seek past what exists by opening a new stream.
	Growing  bool
	StartSec float64
	// Source and Variant describe the stream in its master playlist, for
	// clients that get subtitles as HLS renditions.
	Source  string
	Variant *Variant
	// Preset overrides the libx264 preset (e.g. "superfast" for DVD menus or
	// low-power machines). Ignored by hardware encoders.
	Preset string
	// HoldLastFrame pads video (and audio) to Duration by repeating the last
	// frame. DVD menus and transitions are often a single still picture
	// shown for many seconds; without this the stream would be one frame
	// long and players stall waiting for more.
	HoldLastFrame bool
}

// Variant is the single stream a master playlist points at.
type Variant struct {
	BandwidthKbps int
	Codecs        string // RFC 6381; empty when not known for sure
	Width, Height int
	Range         string // SDR | PQ | HLG; empty for SDR
	Subtitles     []Rendition
}

// Rendition is a WebVTT subtitle track of a stream.
type Rendition struct {
	Stream   int // subtitle stream number in the source (s:N)
	Name     string
	Language string
	Default  bool
	Forced   bool
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
	tools    *ffmpeg.Tools
	log      *slog.Logger
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

func NewManager(tools *ffmpeg.Tools, root string, max int, log *slog.Logger) *Manager {
	m := &Manager{tools: tools, root: root, log: log, session: map[string]*Session{}, max: max}
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
			} else if idle > 90*time.Second && !s.Spec.Growing {
				// A growing stream can't continue where it was stopped; it is
				// paced, so it may run on until the session expires.
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
	if spec.SegmentSec <= 0 {
		spec.SegmentSec = 4
		if spec.LowLatency {
			spec.SegmentSec = 2
		}
	}
	s := &Session{Spec: spec, Dir: filepath.Join(m.root, spec.ID), lastUsed: time.Now(), tools: m.tools, log: m.log.With("hls", spec.ID)}
	_ = os.MkdirAll(s.Dir, 0o755)
	if spec.Growing {
		m.evictGrowing(maxGrowing - 1)
	}
	m.session[spec.ID] = s
	if spec.Growing {
		s.Warm()
	}
	return s
}

// maxGrowing limits growing streams at once: each runs FFmpeg until it finishes
// or expires, needs no request to keep going, and a new one starts at every
// seek past what exists, so they must not be able to pile up.
const maxGrowing = 8

// evictGrowing ends the longest-unused growing streams, leaving at most keep.
// A viewer still on one gets a 404 and opens it again. The caller holds m.mu.
func (m *Manager) evictGrowing(keep int) {
	for {
		n := 0
		var oldest string
		var at time.Time
		for id, s := range m.session {
			if !s.Spec.Growing {
				continue
			}
			n++
			s.mu.Lock()
			used := s.lastUsed
			s.mu.Unlock()
			if oldest == "" || used.Before(at) {
				oldest, at = id, used
			}
		}
		if n <= keep {
			return
		}
		s := m.session[oldest]
		s.stop()
		_ = os.RemoveAll(s.Dir)
		delete(m.session, oldest)
	}
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
	s.mu.Lock()
	s.lastUsed = time.Now()
	s.mu.Unlock()
}

// SegmentExt is the file extension of this stream's segments.
func (s *Session) SegmentExt() string {
	if s.Spec.FMP4 {
		return ".m4s"
	}
	return ".ts"
}

func (s *Session) segPath(n int) string {
	return filepath.Join(s.Dir, fmt.Sprintf("seg%05d%s", n, s.SegmentExt()))
}

// growingName is the file FFmpeg writes a growing stream's playlist to.
const growingName = "ffmpeg-0.m3u8"

// growingTarget is the least target duration a growing playlist announces.
const growingTarget = 10

// GrowingPlaylist is FFmpeg's playlist as it stands, with segments renamed by
// segURL (a segment number, or -1 for the fMP4 init segment). It waits until
// the first segments exist: players give up on a nearly empty playlist.
func (s *Session) GrowingPlaylist(ctx context.Context, segURL func(n int) string) ([]byte, error) {
	s.touch()
	p := filepath.Join(s.Dir, growingName)
	deadline := time.Now().Add(45 * time.Second)
	for {
		s.mu.Lock()
		j := s.job
		s.mu.Unlock()
		b, err := os.ReadFile(p)
		if err == nil && (bytes.Count(b, []byte("#EXTINF")) >= 3 || (j != nil && j.finished())) {
			return rewriteGrowing(b, segURL), nil
		}
		if j != nil && j.finished() {
			return nil, fmt.Errorf("remux ended before the stream started: %s", tail(j.stderr.String(), 800))
		}
		if time.Now().After(deadline) {
			return nil, errors.New("timed out waiting for the stream")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
}

// rewriteGrowing renames the segments in FFmpeg's playlist and makes players
// start at its beginning (a playlist that is still growing otherwise starts
// near its end, as if it were live). FFmpeg raises the target duration when a
// long gap between keyframes comes along, which players may refuse in a playlist
// they already know, so it starts high enough to cover the usual cases.
func rewriteGrowing(b []byte, segURL func(n int) string) []byte {
	var out strings.Builder
	for _, line := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			fmt.Fprintf(&out, "#EXT-X-MAP:URI=\"%s\"\n", segURL(-1))
		case strings.HasPrefix(line, "#EXT-X-TARGETDURATION:"):
			n, _ := strconv.Atoi(strings.TrimPrefix(line, "#EXT-X-TARGETDURATION:"))
			fmt.Fprintf(&out, "#EXT-X-TARGETDURATION:%d\n", max(n, growingTarget))
		case strings.HasPrefix(line, "#EXT-X-VERSION:"):
			out.WriteString(line + "\n#EXT-X-START:TIME-OFFSET=0,PRECISE=YES\n")
		case strings.HasPrefix(line, "#"):
			out.WriteString(line + "\n")
		default:
			name := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(line, "seg"), ".ts"), ".m4s")
			if n, err := strconv.Atoi(name); err == nil {
				out.WriteString(segURL(n) + "\n")
			}
		}
	}
	return []byte(out.String())
}

// MasterPlaylist points at the stream and its subtitle renditions. subs maps
// a position in Variant.Subtitles to its media playlist.
func (s *Session) MasterPlaylist(media string, subs func(i int) string) []byte {
	v := s.Spec.Variant
	if v == nil {
		v = &Variant{}
	}
	var b strings.Builder
	version := 4
	if s.Spec.FMP4 {
		version = 7
	}
	fmt.Fprintf(&b, "#EXTM3U\n#EXT-X-VERSION:%d\n#EXT-X-INDEPENDENT-SEGMENTS\n", version)
	yn := func(on bool) string {
		if on {
			return "YES"
		}
		return "NO"
	}
	seen := map[string]int{}
	for i, r := range v.Subtitles {
		name := strings.NewReplacer(`"`, "'", "\n", " ", "\r", " ").Replace(r.Name)
		// Names must differ within the group.
		if seen[name]++; seen[name] > 1 {
			name = fmt.Sprintf("%s (%d)", name, seen[name])
		}
		fmt.Fprintf(&b, `#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="%s",DEFAULT=%s,AUTOSELECT=%s,FORCED=%s`, name, yn(r.Default), yn(r.Default || r.Forced), yn(r.Forced))
		if r.Language != "" {
			fmt.Fprintf(&b, `,LANGUAGE="%s"`, r.Language)
		}
		fmt.Fprintf(&b, ",URI=\"%s\"\n", subs(i))
	}
	kbps := v.BandwidthKbps
	if kbps <= 0 {
		kbps = 8000
	}
	fmt.Fprintf(&b, "#EXT-X-STREAM-INF:BANDWIDTH=%d,AVERAGE-BANDWIDTH=%d", kbps*1000, kbps*1000)
	if v.Codecs != "" {
		fmt.Fprintf(&b, `,CODECS="%s"`, v.Codecs)
	}
	if v.Width > 0 && v.Height > 0 {
		fmt.Fprintf(&b, ",RESOLUTION=%dx%d", v.Width, v.Height)
	}
	if v.Range != "" {
		fmt.Fprintf(&b, ",VIDEO-RANGE=%s", v.Range)
	}
	if len(v.Subtitles) > 0 {
		b.WriteString(`,SUBTITLES="subs"`)
	}
	fmt.Fprintf(&b, "\n%s\n", media)
	return []byte(b.String())
}

// SubtitlePlaylist is the media playlist of one subtitle rendition: the whole
// track as a single WebVTT file, which is allowed for video on demand.
func (s *Session) SubtitlePlaylist(vtt string) []byte {
	s.touch()
	d := s.Spec.Duration - s.Spec.StartSec
	if d < 1 {
		d = 1
	}
	return []byte(fmt.Sprintf("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXTINF:%.3f,\n%s\n#EXT-X-ENDLIST\n",
		int(d+0.999), d, vtt))
}

// InitSegment is the fMP4 header of a growing stream, once FFmpeg has written it.
func (s *Session) InitSegment(ctx context.Context) (string, error) {
	return s.waitFile(ctx, filepath.Join(s.Dir, "init.mp4"))
}

// waitFile waits for a file of a running growing stream.
func (s *Session) waitFile(ctx context.Context, p string) (string, error) {
	s.touch()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if fileReady(p) {
			return p, nil
		}
		s.mu.Lock()
		j := s.job
		s.mu.Unlock()
		if j == nil || j.finished() {
			if fileReady(p) {
				return p, nil
			}
			return "", errors.New("this stream has no such segment; open it again")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return "", errors.New("timed out waiting for segment")
}

// Segment returns the path of segment n, starting or restarting FFmpeg as
// needed and waiting until the file is complete.
func (s *Session) Segment(ctx context.Context, n int) (string, error) {
	s.touch()
	if s.Spec.Growing {
		if n < 0 {
			return "", errors.New("segment out of range")
		}
		return s.waitFile(ctx, s.segPath(n))
	}
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
	if start > 0 {
		args = append(args, "-ss", strconv.FormatFloat(start, 'f', 3, 64))
	}
	if sp.Growing {
		if sp.StartSec > 0 {
			args = append(args, "-ss", strconv.FormatFloat(sp.StartSec, 'f', 3, 64))
		}
		// A copy runs as fast as the disk reads; a few times real time leaves
		// plenty ahead of the viewer without filling the disk for nothing.
		args = append(args, "-readrate", "4")
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
		if sp.VideoTag != "" {
			args = append(args, "-tag:v", sp.VideoTag)
		}
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
		if sp.SubtitleIndex >= 0 && sp.SubtitleText {
			vf = append(vf, textSubtitleFilter(sp.Source, sp.SubtitleIndex, start))
		}
		vf = append(vf, "setsar=1", "format="+plan.PixFmt)
		if plan.Upload != "" {
			vf = append(vf, plan.Upload)
		}
		if sp.SubtitleIndex >= 0 && !sp.SubtitleText {
			// Burn bitmap subtitles in via a filter graph.
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
	if sp.AudioIndex >= 0 && sp.CopyAudio {
		args = append(args, "-c:a", "copy")
	} else if sp.AudioIndex >= 0 {
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
	)
	dir := s.Dir
	if sp.FMP4 {
		// FFmpeg writes the init segment relative to where it runs, whatever
		// the other paths say, so it runs in the stream's folder.
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
		args = append(args, "-hls_segment_type", "fmp4", "-hls_fmp4_init_filename", "init.mp4")
	} else {
		args = append(args, "-hls_segment_type", "mpegts")
	}
	if sp.Growing {
		args = append(args, "-hls_playlist_type", "event")
	}
	args = append(args,
		"-hls_list_size", "0",
		"-hls_flags", "temp_file+independent_segments",
		"-start_number", strconv.Itoa(fromSeg),
		"-hls_segment_filename", filepath.Join(dir, "seg%05d"+s.SegmentExt()),
		filepath.Join(dir, fmt.Sprintf("ffmpeg-%d.m3u8", fromSeg)),
	)
	cmd := ffmpeg.Command(context.Background(), s.tools.FFmpeg, args...)
	if sp.FMP4 {
		cmd.Dir = dir
	}
	j := &job{startSeg: fromSeg, cmd: cmd, done: make(chan struct{}), lastSeg: fromSeg - 1}
	cmd.Stderr = &j.stderr
	s.log.Info("ffmpeg start", "cat", "ffmpeg", "hls", s.Spec.ID, "from", fromSeg, "args", strings.Join(args, " "))
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	s.job = j
	go func() {
		j.err = cmd.Wait()
		close(j.done)
		if j.err != nil && !strings.Contains(j.err.Error(), "killed") && !strings.Contains(j.err.Error(), "exit status 255") {
			s.log.Warn("ffmpeg exited", "err", j.err, "stderr", tail(j.stderr.String(), 800))
		}
	}()
	go s.watch(j)
	return j, nil
}

// textSubtitleFilter draws text subtitles from the file itself. The filter
// counts from the start of the file while the picture, after a seek, counts from
// where it started, so the clock is put right around it.
func textSubtitleFilter(path string, stream int, start float64) string {
	f := fmt.Sprintf("subtitles='%s':si=%d", escapeFilterPath(path), stream)
	if start > 0 {
		f = fmt.Sprintf("setpts=PTS+%.3f/TB,%s,setpts=PTS-STARTPTS", start, f)
	}
	return f
}

// escapeFilterPath writes a path for a quoted value in a filter graph, which is
// read twice: by the graph, then by the option.
func escapeFilterPath(p string) string {
	return strings.NewReplacer(`\`, `\\\\`, `:`, `\:`, `'`, `'\\\''`).Replace(filepath.ToSlash(p))
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
