// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package ffmpeg locates and drives the bundled FFmpeg binaries.
package ffmpeg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/dvd"
	"github.com/wittywilla/wittymediaserver/internal/layout"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/pace"
)

// Tools holds resolved binary paths and encoder capabilities.
type Tools struct {
	FFmpeg   string
	FFprobe  string
	Encoders map[string]bool
	Version  string
}

func exeName(n string) string {
	if runtime.GOOS == "windows" {
		return n + ".exe"
	}
	return n
}

// Locate finds ffmpeg/ffprobe: explicit paths, then beside the executable
// (bin/, ffmpeg/), then PATH.
func Locate(ffmpegPath, ffprobePath string) (*Tools, error) {
	t := &Tools{Encoders: map[string]bool{}}
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, layout.Current().ProgramDirs()...)
		dir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(dir, "ffmpeg"), filepath.Join(dir, "..", "ffmpeg"))
	}
	// The copy for working on this project, only in a source checkout: FFmpeg
	// must not be run from whatever folder the server was started in.
	if st, err := os.Stat("gomod.go"); err == nil && !st.IsDir() {
		candidates = append(candidates, ".tools/ffmpeg-master-latest-win64-gpl/bin")
	}
	find := func(explicit, name string) string {
		if explicit != "" {
			return explicit
		}
		for _, d := range candidates {
			p := filepath.Join(d, exeName(name))
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
		return ""
	}
	t.FFmpeg = find(ffmpegPath, "ffmpeg")
	t.FFprobe = find(ffprobePath, "ffprobe")
	if t.FFmpeg == "" {
		return nil, errors.New("ffmpeg not found: put it in the program's bin folder or set WMS_FFMPEG")
	}
	if t.FFprobe == "" {
		t.FFprobe = filepath.Join(filepath.Dir(t.FFmpeg), exeName("ffprobe"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, banner, err := t.encoderList(ctx)
	if err != nil {
		return t, nil
	}
	if line, _, _ := strings.Cut(banner, "\n"); strings.HasPrefix(line, "ffmpeg version") {
		t.Version = strings.TrimSpace(line)
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && len(f[0]) == 6 && f[0][0] == 'V' || len(f) >= 2 && len(f[0]) == 6 && f[0][0] == 'A' {
			t.Encoders[f[1]] = true
		}
	}
	return t, nil
}

// encoderList asks FFmpeg for its encoders, and gets its version too: without -hide_banner it
// names itself first on standard error. That's one start instead of two before the server can
// listen. A start takes a tenth of a second on Linux, and from half a second to well over one
// on a busy Windows machine, where the 170 MB program is scanned each time.
func (t *Tools) encoderList(ctx context.Context) (list []byte, banner string, err error) {
	cmd := Command(ctx, t.FFmpeg, "-nostdin", "-encoders")
	var stdout bytes.Buffer
	stderr := &logging.Tail{} // the banner is at the start, which it keeps
	cmd.Stdout, cmd.Stderr = &stdout, stderr
	started := time.Now()
	if err := cmd.Run(); err != nil {
		logging.ChildExit(slog.Default().With("cat", "transcode"), "ffmpeg", err, false, stderr, started, "args", "-encoders")
		return nil, "", err
	}
	return stdout.Bytes(), stderr.String(), nil
}

// ---------------------------------------------------------------------------
// Probing
// ---------------------------------------------------------------------------

type Stream struct {
	Index       int               `json:"index"`
	ID          string            `json:"id"`
	CodecType   string            `json:"codec_type"`
	CodecName   string            `json:"codec_name"`
	PixFmt      string            `json:"pix_fmt"`
	Width       int               `json:"width"`
	Height      int               `json:"height"`
	Channels    int               `json:"channels"`
	SampleRate  string            `json:"sample_rate"`
	FieldOrder  string            `json:"field_order"`
	SAR         string            `json:"sample_aspect_ratio"`
	DAR         string            `json:"display_aspect_ratio"`
	Tags        map[string]string `json:"tags"`
	Disposition map[string]int    `json:"disposition"`
	Duration    string            `json:"duration"`
	// For playback decisions.
	Profile       string           `json:"profile"`
	Level         int              `json:"level"`
	ColorTransfer string           `json:"color_transfer"`
	BitRate       string           `json:"bit_rate"`
	BitsPerSample string           `json:"bits_per_raw_sample"`
	CodecTag      string           `json:"codec_tag_string"`
	SideData      []map[string]any `json:"side_data_list"`
	// Frame rates as fractions, and whether captions are carried inside the picture.
	RFrameRate     string `json:"r_frame_rate"`
	AvgFrameRate   string `json:"avg_frame_rate"`
	ClosedCaptions int    `json:"closed_captions"`
}

// DisplaySize is the picture's size as shown: phones store upright video as a
// sideways frame with a rotation to apply.
func (s *Stream) DisplaySize() (w, h int) {
	w, h = s.Width, s.Height
	rot := s.Tags["rotate"]
	turned := rot == "90" || rot == "270" || rot == "-90"
	for _, d := range s.SideData {
		if f, ok := d["rotation"].(float64); ok {
			if r := int(f) % 180; r == 90 || r == -90 {
				turned = true
			}
		}
	}
	if turned {
		w, h = h, w
	}
	return w, h
}

type Format struct {
	Filename   string            `json:"filename"`
	FormatName string            `json:"format_name"`
	Duration   string            `json:"duration"`
	StartTime  string            `json:"start_time"`
	Size       string            `json:"size"`
	BitRate    string            `json:"bit_rate"`
	Tags       map[string]string `json:"tags"`
}

type Probe struct {
	Streams  []Stream `json:"streams"`
	Format   Format   `json:"format"`
	Chapters []struct {
		Start string            `json:"start_time"`
		End   string            `json:"end_time"`
		Tags  map[string]string `json:"tags"`
	} `json:"chapters"`
}

// SeekMargin is how far ahead of a start to begin reading this file, for
// hls.Spec.SeekMargin: transport and program streams have no index to seek by.
func (p *Probe) SeekMargin() float64 {
	for _, f := range strings.Split(p.Format.FormatName, ",") {
		if f == "mpegts" || f == "mpeg" {
			return 3
		}
	}
	return 0
}

func (p *Probe) Duration() float64 {
	d, _ := strconv.ParseFloat(p.Format.Duration, 64)
	return d
}

// Video is the file's picture: its first video stream that really is one. A file may
// also carry cover art, stored as a one-frame video stream, ahead of it.
func (p *Probe) Video() *Stream {
	for i := range p.Streams {
		if p.Streams[i].CodecType == "video" && !p.Streams[i].stillPicture() {
			return &p.Streams[i]
		}
	}
	return nil
}

func (s *Stream) stillPicture() bool {
	return s.Disposition["attached_pic"] == 1 || s.Disposition["still_image"] == 1 || s.Disposition["timed_thumbnails"] == 1
}

// VideoIndex is the number among the file's video streams (v:N) of the one Video
// returns. Picking "the first video stream" would pick the cover art of some files.
func (p *Probe) VideoIndex() int {
	n := 0
	for i := range p.Streams {
		if p.Streams[i].CodecType != "video" {
			continue
		}
		if !p.Streams[i].stillPicture() {
			return n
		}
		n++
	}
	return 0
}

// VideoStreams lists the streams that are pictures, without cover art.
func (p *Probe) VideoStreams() []Stream {
	var out []Stream
	for i := range p.Streams {
		if p.Streams[i].CodecType == "video" && !p.Streams[i].stillPicture() {
			out = append(out, p.Streams[i])
		}
	}
	return out
}

func (p *Probe) Audio() []Stream {
	var out []Stream
	for _, s := range p.Streams {
		if s.CodecType == "audio" {
			out = append(out, s)
		}
	}
	return out
}

func (p *Probe) Subtitles() []Stream {
	var out []Stream
	for _, s := range p.Streams {
		if s.CodecType == "subtitle" {
			out = append(out, s)
		}
	}
	return out
}

// FileInput opens a file from a library as input. Only local file reading
// is allowed, and only as a real media format (see libraryFormats): a downloaded
// "video" that is really a playlist must not make the server fetch addresses on
// the network, or read other files.
func FileInput(path string) []string {
	if literalPicture(path) {
		return []string{"-format_whitelist", "image2", "-f", "image2", "-pattern_type", "none", "-protocol_whitelist", "file,pipe", "-i", path}
	}
	return []string{"-format_whitelist", libraryFormatsFor(path), "-protocol_whitelist", "file,pipe", "-i", path}
}

// ProbeFile runs ffprobe on a regular media file.
func (t *Tools) ProbeFile(ctx context.Context, path string) (*Probe, error) {
	return t.probe(ctx, FileInput(path))
}

// ProbeInput runs ffprobe on input options built by the caller (a TV stream).
func (t *Tools) ProbeInput(ctx context.Context, input []string) (*Probe, error) {
	return t.probe(ctx, input)
}

// probeSlots limits how many ffprobe processes run at once: anyone signed in
// can ask for a title's details as often as they like. How many depends on the
// machine: each is a process with a processor to itself while it runs.
var probeSlots = sync.OnceValue(func() chan struct{} { return make(chan struct{}, pace.Slots(pace.Probes)) })

func (t *Tools) probe(ctx context.Context, input []string) (*Probe, error) {
	// A scan's probe gives way to someone's stream on a small machine. Before the
	// slot, so a waiting scan holds nothing a person's own request needs.
	if err := pace.Yield(ctx); err != nil {
		return nil, err
	}
	slots := probeSlots()
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	args := []string{"-v", "error", "-print_format", "json", "-show_format", "-show_streams", "-show_chapters"}
	args = append(args, input...)
	out, stderr, err := Run(ctx, t.FFprobe, args)
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %w: %s", err, shortFormats(stderr.String()))
	}
	var p Probe
	if err := json.Unmarshal(out, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// KeyframeNear finds where a copy that starts at this time really starts: the first
// keyframe FFmpeg reaches after seeking there. No keyframe found says so.
func (t *Tools) KeyframeNear(ctx context.Context, path string, videoIndex int, sec float64) (float64, bool) {
	args := []string{"-v", "error", "-select_streams", "v:" + strconv.Itoa(videoIndex), "-read_intervals", strconv.FormatFloat(sec, 'f', 3, 64) + "%+#600",
		"-show_entries", "packet=pts_time,flags", "-of", "csv=p=0"}
	args = append(args, FileInput(path)...)
	out, _, err := Run(ctx, t.FFprobe, args)
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(out), "\n") {
		pts, flags, ok := strings.Cut(strings.TrimSpace(line), ",")
		if !ok || !strings.HasPrefix(flags, "K") {
			continue
		}
		if v, err := strconv.ParseFloat(pts, 64); err == nil && v >= 0 {
			return v, true
		}
	}
	return 0, false
}

// measureTime finds FFmpeg's last report of where it had got to.
var measureTime = regexp.MustCompile(`time=(\d+):(\d+):(\d+(?:\.\d+)?)`)

// MeasureDuration reads a file through without decoding it, for what doesn't say how
// long it is: a bare stream with no container. 0 means it couldn't be found out.
func (t *Tools) MeasureDuration(ctx context.Context, path string) float64 {
	args := append([]string{"-hide_banner", "-nostdin", "-stats"}, FileInput(path)...)
	args = append(args, "-map", "0:v:0?", "-map", "0:a:0?", "-c", "copy", "-f", "null", "-")
	cmd := Command(ctx, t.FFmpeg, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &limitWriter{w: &stderr, n: 1 << 20}
	if err := cmd.Run(); err != nil {
		return 0
	}
	if all := measureTime.FindAllStringSubmatch(stderr.String(), -1); len(all) > 0 {
		m := all[len(all)-1]
		h, _ := strconv.ParseFloat(m[1], 64)
		mi, _ := strconv.ParseFloat(m[2], 64)
		sec, _ := strconv.ParseFloat(m[3], 64)
		return h*3600 + mi*60 + sec
	}
	// A raw stream has no times at all. Its frames and frame rate say the same.
	if m := measureFrames.FindAllStringSubmatch(stderr.String(), -1); len(m) > 0 {
		frames, _ := strconv.ParseFloat(m[len(m)-1][1], 64)
		if rate := measureRate.FindStringSubmatch(stderr.String()); rate != nil {
			if fps, _ := strconv.ParseFloat(rate[1], 64); fps > 0 && fps < 1000 {
				return frames / fps
			}
		}
	}
	return 0
}

var (
	measureFrames = regexp.MustCompile(`frame=\s*(\d+)`)
	measureRate   = regexp.MustCompile(`([0-9.]+) fps`)
)

// limitWriter keeps the last part of what's written to it, up to n bytes in all.
type limitWriter struct {
	w *bytes.Buffer
	n int
}

func (l *limitWriter) Write(b []byte) (int, error) {
	if l.w.Len()+len(b) > l.n {
		rest := l.w.Bytes()
		keep := rest[min(len(rest), len(b)):]
		l.w = bytes.NewBuffer(append(append(make([]byte, 0, l.n), keep...), b...))
		return len(b), nil
	}
	return l.w.Write(b)
}

// ---------------------------------------------------------------------------
// Disc inputs: ffconcat lists of subfile byte ranges
// ---------------------------------------------------------------------------

// Segment is one playable run of bytes with a known duration.
type Segment struct {
	Extents  []dvd.FileExtent
	Duration float64 // seconds, 0 = unknown
	// Lead is how long before its first picture a segment's earliest packet
	// lies (seconds): usually sound, stored ahead of its picture, up to a
	// quarter of a second on a DVD, a different amount in every segment. The
	// concat demuxer puts a segment on the timeline by that earliest packet, so
	// without this the picture lands late by that much, and each join repeats
	// or drops frames and sound. (The demuxer's inpoint would say the same thing
	// directly, but it seeks to get there, and that can land a whole group of
	// pictures in.)
	Lead float64
	// Outpoint is the time on the segment's own clock (seconds) to stop reading at,
	// when its bytes run on further than it needs; 0 reads them all. FFmpeg stops at
	// the first packet of any stream that is decoded at this time or later, so it
	// doesn't end a segment on a chosen frame.
	Outpoint float64
}

// SubfileURL builds a subfile protocol URL for one extent.
func SubfileURL(e dvd.FileExtent) string {
	return fmt.Sprintf("subfile,,start,%d,end,%d,,:%s", e.Offset, e.Offset+e.Length, filepath.ToSlash(e.File))
}

func quoteConcat(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// WriteConcat writes an ffconcat list for the segments into dir and returns
// the list path. Multi-extent segments get a concatf side file.
//
// streamIDs, when given, pin the output streams to these MPEG-PS stream ids
// (in this order). Without them the concat demuxer pairs streams by position,
// and DVD VOBs do not list their streams in a stable order: an interleaved
// cell may announce subtitles before audio, which then silently swaps the
// audio track for a subtitle stream for the length of that cell.
func WriteConcat(dir, name string, segs []Segment, streamIDs ...int) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := noListInCells(segs); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("ffconcat version 1.0\n")
	for _, id := range streamIDs {
		fmt.Fprintf(&b, "stream\nexact_stream_id 0x%x\n", id)
	}
	for i, s := range segs {
		// Lists hold one address per line: a folder name with a line break in
		// it (possible on Linux) would add an address of its own.
		for _, e := range s.Extents {
			if strings.ContainsAny(e.File, "\n\r\x00") {
				return "", fmt.Errorf("unusable file name: %q", e.File)
			}
		}
		pieces := splitExtents(s.Extents)
		var url string
		if len(pieces) == 1 && !misleadingName(pieces[0].File) {
			url = SubfileURL(pieces[0])
		} else {
			side := filepath.Join(dir, fmt.Sprintf("%s-seg%d.txt", name, i+1))
			var lines []string
			for _, e := range pieces {
				lines = append(lines, SubfileURL(e))
			}
			if err := os.WriteFile(side, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
				return "", err
			}
			url = "concatf:" + filepath.ToSlash(side)
		}
		fmt.Fprintf(&b, "file %s\n", quoteConcat(url))
		if s.Duration > 0 {
			// The next segment starts where this one's duration says. Moving that
			// by the difference in lead puts its first picture where it belongs.
			d := s.Duration + s.Lead
			if i+1 < len(segs) {
				d -= segs[i+1].Lead
			}
			fmt.Fprintf(&b, "duration %.6f\n", max(d, 0.001))
		}
		if s.Outpoint > 0 {
			fmt.Fprintf(&b, "outpoint %.6f\n", s.Outpoint)
		}
	}
	p := filepath.Join(dir, name+".ffconcat")
	return p, os.WriteFile(p, []byte(b.String()), 0o644)
}

// misleadingName: FFmpeg picks how to read an input partly from the name of the file.
// To it an .img is a GEM picture, so a disc image of that name goes through a list of
// addresses, whose own name says nothing.
func misleadingName(p string) bool { return strings.EqualFold(filepath.Ext(p), ".img") }

// maxSubfile is the most one subfile address reads. Past 2 GiB in one FFmpeg
// misjudges a program stream's end, and then loses about a second of its start.
const maxSubfile = 1 << 30

// splitExtents cuts extents into pieces that FFmpeg reads reliably.
func splitExtents(ext []dvd.FileExtent) []dvd.FileExtent {
	var out []dvd.FileExtent
	for _, e := range ext {
		for e.Length > maxSubfile {
			out = append(out, dvd.FileExtent{File: e.File, Offset: e.Offset, Length: maxSubfile})
			e.Offset += maxSubfile
			e.Length -= maxSubfile
		}
		out = append(out, e)
	}
	return out
}

// noListInCells: the concat format has to stay on the list of formats FFmpeg may use here, so a
// part of a disc whose bytes are themselves a list has to be stopped another way. FFmpeg would read
// that list's entries, including a link beside the image. Only the start of a part can be taken for one.
func noListInCells(segs []Segment) error {
	files := map[string]*os.File{}
	defer func() {
		for _, f := range files {
			if f != nil {
				f.Close()
			}
		}
	}()
	head := make([]byte, 8)
	for _, s := range segs {
		if len(s.Extents) == 0 {
			continue
		}
		e := s.Extents[0]
		f, seen := files[e.File]
		if !seen {
			f, _ = os.Open(e.File)
			files[e.File] = f
		}
		if f == nil {
			continue // it's FFmpeg's to say a file is missing
		}
		if n, _ := f.ReadAt(head, e.Offset); n == len(head) && string(head) == "ffconcat" {
			return errors.New("a part of this disc isn't video")
		}
	}
	return nil
}

// PartsInput opens the files of a split film as one input. Only files can be read,
// and the list is written here from names that were checked. Each part is held to the formats
// a file from a library may be, and the list format has to be among them for this one to open,
// so a part whose bytes are a list is stopped by looking, as a disc's cells are.
func PartsInput(dir, name string, parts []string) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("ffconcat version 1.0\n")
	segs := make([]Segment, 0, len(parts))
	for _, p := range parts {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		if strings.ContainsAny(abs, "\n\r\x00") {
			return nil, fmt.Errorf("unusable file name: %q", p)
		}
		fmt.Fprintf(&b, "file %s\n", quoteConcat(filepath.ToSlash(abs)))
		segs = append(segs, Segment{Extents: []dvd.FileExtent{{File: abs}}})
	}
	if err := noListInCells(segs); err != nil {
		return nil, errors.New("a part of this film isn't video")
	}
	list := filepath.Join(dir, name+".ffconcat")
	if err := os.WriteFile(list, []byte(b.String()), 0o644); err != nil {
		return nil, err
	}
	return []string{"-f", "concat", "-safe", "0", "-format_whitelist", "concat," + libraryFormats, "-protocol_whitelist", "file,pipe", "-i", filepath.ToSlash(list)}, nil
}

// ConcatInputArgs are the ffmpeg input flags for an ffconcat list of subfiles.
func ConcatInputArgs(list string) []string {
	return []string{
		"-f", "concat", "-safe", "0",
		"-format_whitelist", discFormats,
		"-protocol_whitelist", "file,subfile,concat,concatf",
		"-probesize", "50M", "-analyzeduration", "20M",
		"-i", filepath.ToSlash(list),
	}
}

// ---------------------------------------------------------------------------
// Thumbnails / stills
// ---------------------------------------------------------------------------

// Still grabs one frame as JPEG/PNG (by extension of out). inputArgs is
// everything up to and including -i; atSec seeks (input-side) when > 0.
func (t *Tools) Still(ctx context.Context, inputArgs []string, atSec float64, width int, out string) error {
	if err := outFolder(out); err != nil {
		return err
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
	if atSec > 0 {
		args = append(args, "-ss", strconv.FormatFloat(atSec, 'f', 3, 64))
	}
	args = append(args, inputArgs...)
	vf := "scale=iw*sar:ih,setsar=1"
	if width > 0 {
		vf = fmt.Sprintf("scale=iw*sar:ih,scale=%d:-2,setsar=1", width)
	}
	if strings.HasSuffix(strings.ToLower(out), ".jpg") {
		// The JPEG encoder only takes full-range YUV; convert explicitly
		// instead of relying on format negotiation (which fails on some
		// discs with recent ffmpeg builds).
		vf += ",format=yuvj420p"
	}
	args = append(args, "-an", "-sn", "-frames:v", "1", "-vf", vf, "-q:v", "3", out)
	if _, stderr, err := Run(ctx, t.FFmpeg, args); err != nil {
		return fmt.Errorf("ffmpeg still: %w: %s", err, shortFormats(stderr.String()))
	}
	return nil
}

// Poster makes a title's picture from about atSec into it: the keyframe before that point, read on
// one thread. A title's picture doesn't need the exact frame, and going straight to a keyframe takes
// a tenth of the work and a third of the memory of decoding up to the moment on every core (measured
// on a 7 GB film: 110 ms and 77 MB against 2560 ms and 219 MB). If that finds nothing, it's Still.
func (t *Tools) Poster(ctx context.Context, inputArgs []string, atSec float64, width int, out string) error {
	if err := outFolder(out); err != nil {
		return err
	}
	if atSec <= 0 {
		return t.Still(ctx, inputArgs, atSec, width, out)
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-threads", "1", "-skip_frame", "nokey", "-noaccurate_seek", "-ss", strconv.FormatFloat(atSec, 'f', 3, 64)}
	args = append(args, inputArgs...)
	vf := "scale=iw*sar:ih,setsar=1"
	if width > 0 {
		vf = fmt.Sprintf("scale=iw*sar:ih,scale=%d:-2,setsar=1", width)
	}
	if strings.HasSuffix(strings.ToLower(out), ".jpg") {
		vf += ",format=yuvj420p" // see Still
	}
	args = append(args, "-an", "-sn", "-frames:v", "1", "-vf", vf, "-q:v", "3", "-threads", "1", out)
	if _, _, err := Run(ctx, t.FFmpeg, args); err == nil {
		if st, err := os.Stat(out); err == nil && st.Size() > 0 {
			return nil
		}
	}
	return t.Still(ctx, inputArgs, atSec, width, out)
}

// ImageThumb makes a JPEG of a picture, at most width pixels along its longer
// side, upright according to its EXIF orientation (1 to 8; 0 means as stored).
// shrink halves a JPEG that many times while decoding (0 to 3). Rotation is
// always done here rather than by FFmpeg, whose handling differs by version.
func (t *Tools) ImageThumb(ctx context.Context, path string, orientation, width, shrink int, out string) error {
	if err := outFolder(out); err != nil {
		return err
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-noautorotate", "-threads", "1"}
	if shrink > 0 {
		args = append(args, "-lowres", strconv.Itoa(shrink))
	}
	args = append(args, FileInput(path)...)
	vf := ""
	switch orientation {
	case 2:
		vf = "hflip,"
	case 3:
		vf = "hflip,vflip,"
	case 4:
		vf = "vflip,"
	case 5:
		vf = "transpose=0,"
	case 6:
		vf = "transpose=1,"
	case 7:
		vf = "transpose=3,"
	case 8:
		vf = "transpose=2,"
	}
	vf += fmt.Sprintf("scale='min(%d,iw)':'min(%d,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2,format=yuvj420p", width, width)
	args = append(args, "-an", "-sn", "-frames:v", "1", "-vf", vf, "-q:v", "4", "-update", "1", out)
	if _, stderr, err := Run(ctx, t.FFmpeg, args); err != nil {
		return fmt.Errorf("ffmpeg image: %w: %s", err, shortFormats(stderr.String()))
	}
	return nil
}

// outFolder makes the folder a picture is written to: a cache folder can be removed while
// the server runs (a reset, someone tidying up), and FFmpeg won't make it.
func outFolder(out string) error {
	return os.MkdirAll(filepath.Dir(out), 0o755)
}

// Command prepares an ffmpeg/ffprobe process. On Windows it never opens a
// console window (the server often runs without one, from the tray).
func Command(ctx context.Context, path string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, path, args...)
	hideWindow(cmd)
	return cmd
}

// RunLow starts another program the way background FFmpeg jobs are started, at a lower
// priority so it has the processor only when nothing else wants it, and waits for it. For
// the helper programs an owner installs (a stem separator), which can run for minutes.
func RunLow(cmd *exec.Cmd) error {
	hideWindow(cmd)
	lowPriority(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	lowPriorityStarted(cmd)
	return cmd.Wait()
}

// Run runs a short FFmpeg or ffprobe job and waits for it, returning its output
// and what it said. It leaves a record of how the job ended, tied to the session
// in ctx (see logging.WithSession); attrs say what it was for.
func Run(ctx context.Context, path string, args []string, attrs ...any) ([]byte, *logging.Tail, error) {
	cmd := Command(ctx, path, args...)
	stderr := &logging.Tail{}
	cmd.Stderr = stderr
	started := time.Now()
	// As cmd.Output, with a moment between starting and waiting: work nobody is
	// waiting for runs at a lower priority, so it has the processor only when
	// nothing else wants it.
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if pace.IsBackground(ctx) {
		lowPriority(cmd)
	}
	err := cmd.Start()
	if err == nil {
		if pace.IsBackground(ctx) {
			lowPriorityStarted(cmd)
		}
		err = cmd.Wait()
	}
	out := stdout.Bytes()
	// A job killed for its context says only "exit status 1". The context says why.
	why := err
	if err != nil && ctx.Err() != nil {
		why = ctx.Err()
	}
	log := slog.Default().With("cat", "transcode")
	if id := logging.SessionFrom(ctx); id != "" {
		log = log.With("session", id)
	}
	if id := logging.RequestFrom(ctx); id != "" {
		attrs = append(attrs, "request", id)
	}
	logging.ChildExit(log, strings.TrimSuffix(filepath.Base(path), ".exe"), why, errors.Is(ctx.Err(), context.Canceled), stderr, started,
		append([]any{"args", RedactArgs(args)}, attrs...)...)
	return out, stderr, err
}

// Hidden from the log: a stream address can carry a login.
var urlCreds = regexp.MustCompile(`([a-z0-9+]+://)[^/@\s]+@`)

// RedactArgs is a command line for the log, without the headers and logins a
// stream's input options can carry.
func RedactArgs(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if i > 0 && args[i-1] == "-headers" {
			a = "(hidden)"
		}
		if i > 0 && args[i-1] == "-format_whitelist" && len(a) > 80 {
			a = "(media formats)" // a long list, on every line of the log
		}
		out[i] = urlCreds.ReplaceAllString(a, "$1(hidden)@")
	}
	return strings.Join(out, " ")
}
