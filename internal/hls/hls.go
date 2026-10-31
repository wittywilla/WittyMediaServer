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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// Spec describes one transcode.
type Spec struct {
	ID string
	// LogSession is the session this stream logs under when it is part of
	// something bigger (a disc's streams use the disc's); "" = its own ID.
	LogSession  string
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
	// SeekMargin is how far (seconds) before where it should start FFmpeg begins
	// reading, then cuts to the exact frame. Transport and program streams have
	// no index: a seek in them lands up to about a second late, and FFmpeg fills
	// the gap by holding the first picture. That shows at every restart as a
	// frozen moment and then a skip. It costs the decoding of this much at each
	// start, so it is only for inputs that need it: a GOP or two is enough.
	SeekMargin float64
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
	stderr   logging.Tail // FFmpeg can warn for every frame of a damaged file for hours
	lastSeg  atomic.Int64 // the newest finished segment, kept current by watch
	stopped  atomic.Bool  // ended on purpose: how it exits says nothing
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
	log      *slog.Logger // guarded by mu: TieTo can change it
	logID    string       // the session it logs under
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
	max     int // FFmpeg processes running at once
	live    liveRegistry
}

// ErrBusy: as many transcodes are running as the server allows
// (-max-transcodes), and all of them have someone watching.
var ErrBusy = errors.New("this server is busy with other streams; try again in a moment")

const (
	// maxSessions caps known streams; each has a folder, usually with segments.
	maxSessions = 200
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
	if max < 1 {
		max = 1
	}
	m := &Manager{tools: tools, root: root, log: log, session: map[string]*Session{}, max: max}
	cleanRoot(root)
	if err := os.MkdirAll(root, 0o755); err != nil {
		log.Error("could not make the transcode folder", "cat", "transcode", "dir", root, "err", err)
	}
	logging.Loop("hls sweep", m.sweep, "cat", "transcode")
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
		// A growing stream can't continue where it was stopped; it is paced, so
		// it may run on until its session expires, and then it is opened again.
		case idle > forgetAfter, idle > cleanAfter && s.Spec.Growing:
			clean = append(clean, s)
			delete(m.session, id)
		case idle > cleanAfter:
			clean = append(clean, s)
		case idle > pauseAfter && !s.Spec.Growing:
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
	if s.Spec.Growing { // growing streams copy, and have their own limit
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	running := 0
	var oldest *Session
	var oldestUsed time.Time
	for _, o := range m.session {
		if o == s || o.Spec.Growing { // growing streams copy, and have their own limit
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
		oldest.logger().Info("transcode limit reached: pausing an idle stream", "idle", time.Since(oldestUsed).Round(time.Second).String())
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
	s, created := m.ensure(spec)
	if created && spec.Growing {
		s.Warm() // without the manager's lock: starting FFmpeg takes time, and other streams wait on it
	}
	return s
}

func (m *Manager) ensure(spec Spec) (s *Session, created bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.session[spec.ID]; ok {
		return s, false
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
	logID := spec.LogSession
	if logID == "" {
		logID = spec.ID
	}
	s = &Session{Spec: spec, Dir: filepath.Join(m.root, spec.ID), lastUsed: time.Now(), tools: m.tools, log: m.sessionLog(logID), logID: logID, m: m, viewers: map[string]time.Time{}}
	s.used.Store(time.Now().UnixNano())
	_ = os.MkdirAll(s.Dir, 0o755) // tried again before FFmpeg starts, which logs a failure
	if spec.Growing {
		m.evictGrowing(maxGrowing - 1)
	}
	m.session[spec.ID] = s
	return s, true
}

func (m *Manager) sessionLog(id string) *slog.Logger {
	return m.log.With("cat", "transcode", "session", id)
}

// logger is what this stream logs through.
func (s *Session) logger() *slog.Logger {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.log
}

// LogSession is the session this stream logs under.
func (s *Session) LogSession() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.logID
}

// TieTo moves the stream's logging to another session: a stream outlives the
// disc session that made it, and the next one to open that disc takes it over.
func (s *Session) TieTo(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != "" && id != s.logID {
		s.log, s.logID = s.m.sessionLog(id), id
	}
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
	s.used.Store(s.lastUsed.UnixNano())
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
	now := time.Now()
	s.used.Store(now.UnixNano())
	s.mu.Lock()
	s.lastUsed = now
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
				s.logFailedStart("could not resume a transcode", err)
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
		s.logFailedStart("could not start a transcode ahead of the viewer", err)
	}
}

// logFailedStart is for starts nobody is waiting on, so nobody receives the
// error. A server at its limit will try again when the viewer gets there.
func (s *Session) logFailedStart(msg string, err error) {
	if errors.Is(err, ErrBusy) {
		s.logger().Debug(msg, "err", err)
		return
	}
	s.logger().Warn(msg, "err", err)
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
	// The segments of an idle session may have been cleaned up.
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		s.log.Warn("could not make the stream's folder", "dir", s.Dir, "err", err)
	}
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
	if sp.Growing {
		if sp.StartSec > 0 {
			args = append(args, "-ss", strconv.FormatFloat(sp.StartSec, 'f', 3, 64))
		}
		// A copy runs as fast as the disk reads; a few times real time leaves
		// plenty ahead of the viewer without filling the disk for nothing.
		args = append(args, "-readrate", "4")
	}
	if start > 0 && sp.AudioInput > 0 {
		for _, a := range sp.InputArgs {
			if a == "-i" {
				args = append(args, "-ss", strconv.FormatFloat(start, 'f', 3, 64))
			}
			args = append(args, a)
		}
	} else {
		margin := sp.SeekMargin
		if sp.SubtitleIndex >= 0 || sp.CopyVideo { // burning in lines up with the input's own clock; a copy starts at a keyframe
			margin = 0
		}
		read := max(0, start-margin)
		if read > 0 {
			args = append(args, "-ss", strconv.FormatFloat(read, 'f', 3, 64))
		}
		args = append(args, sp.InputArgs...)
		if cut := start - read; cut > 0 {
			args = append(args, "-ss", strconv.FormatFloat(cut, 'f', 3, 64))
		}
	}
	args = append(args, "-map", "0:v:0")
	if sp.AudioIndex >= 0 {
		args = append(args, "-map", fmt.Sprintf("%d:a:%d?", sp.AudioInput, sp.AudioIndex))
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
		if rate := firstNonEmpty(sp.VideoBitrate, sp.Quality.MaxRate); rate != "" && len(sp.Quality.VideoArgs) == 0 && sp.Quality.CapRate == "" {
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
		if !sp.CopyVideo {
			// The encoder's first frames have decode times below zero when it
			// starts at zero, and by default the muxers move the whole run later
			// to avoid them (by two frames, or so). A restart further on has
			// nothing to avoid, so the first restart would jump back by that much
			// and the player would lose those frames. Both muxers need telling.
			args = append(args, "-avoid_negative_ts", "disabled", "-hls_segment_options", "avoid_negative_ts=disabled")
		}
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
	j := &job{startSeg: fromSeg, cmd: cmd, done: make(chan struct{})}
	j.lastSeg.Store(int64(fromSeg) - 1)
	cmd.Stderr = &j.stderr
	cmdline := ffmpeg.RedactArgs(args)
	encoder := plan.Name
	if sp.CopyVideo {
		encoder = "copy"
	}
	s.log.Info("transcode started", "segment", fromSeg, "encoder", encoder, "height", sp.MaxHeight)
	s.log.Debug("ffmpeg command", "args", cmdline)
	started := time.Now()
	if err := cmd.Start(); err != nil {
		s.log.Error("ffmpeg could not start", "reason", logging.ExitReason(err), "err", err, "args", cmdline)
		return nil, err
	}
	s.job = j
	logID := s.logID
	logging.Go("ffmpeg wait", func() {
		j.err = cmd.Wait()
		close(j.done)
		logging.ChildExit(s.logger(), "ffmpeg", j.err, j.stopped.Load(), &j.stderr, started, "segment", fromSeg, "args", cmdline)
	}, "cat", "transcode", "session", logID)
	logging.Go("ffmpeg watch", func() { s.watch(j) }, "cat", "transcode", "session", logID)
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
				s.log.Debug("resting: far ahead of the viewer", "at", j.lastSeg.Load())
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
