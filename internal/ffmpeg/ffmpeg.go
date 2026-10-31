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
	"time"

	"github.com/wittywilla/wittydvd"
	"github.com/wittywilla/wittymediaserver/internal/logging"
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
		dir := filepath.Dir(exe)
		candidates = append(candidates, dir, filepath.Join(dir, "bin"), filepath.Join(dir, "ffmpeg"), filepath.Join(dir, "..", "ffmpeg"), filepath.Join(dir, "..", "Resources", "ffmpeg"))
	}
	// The copy for working on this project, only in a source checkout: FFmpeg
	// must not be run from whatever folder the server was started in.
	if st, err := os.Stat("go.work"); err == nil && !st.IsDir() {
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
		return nil, errors.New("ffmpeg not found: bundle it beside the wms binary (bin/ffmpeg) or set WMS_FFMPEG")
	}
	if t.FFprobe == "" {
		t.FFprobe = filepath.Join(filepath.Dir(t.FFmpeg), exeName("ffprobe"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if out, _, err := Run(ctx, t.FFmpeg, []string{"-hide_banner", "-version"}); err == nil {
		if line, _, ok := bytes.Cut(out, []byte("\n")); ok {
			t.Version = strings.TrimSpace(string(line))
		}
	}
	if out, _, err := Run(ctx, t.FFmpeg, []string{"-hide_banner", "-encoders"}); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			f := strings.Fields(line)
			if len(f) >= 2 && len(f[0]) == 6 && f[0][0] == 'V' || len(f) >= 2 && len(f[0]) == 6 && f[0][0] == 'A' {
				t.Encoders[f[1]] = true
			}
		}
	}
	return t, nil
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

func (p *Probe) Video() *Stream {
	for i := range p.Streams {
		if p.Streams[i].CodecType == "video" && p.Streams[i].Disposition["attached_pic"] == 0 {
			return &p.Streams[i]
		}
	}
	return nil
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
// is allowed: a downloaded "video" that is really a playlist must not make
// the server fetch addresses on the network.
func FileInput(path string) []string {
	return []string{"-protocol_whitelist", "file,pipe", "-i", path}
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
// can ask for a title's details as often as they like.
var probeSlots = make(chan struct{}, max(4, runtime.NumCPU()))

func (t *Tools) probe(ctx context.Context, input []string) (*Probe, error) {
	select {
	case probeSlots <- struct{}{}:
		defer func() { <-probeSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	args := []string{"-v", "error", "-print_format", "json", "-show_format", "-show_streams", "-show_chapters"}
	args = append(args, input...)
	out, stderr, err := Run(ctx, t.FFprobe, args)
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %w: %s", err, stderr.String())
	}
	var p Probe
	if err := json.Unmarshal(out, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ---------------------------------------------------------------------------
// Disc inputs: ffconcat lists of subfile byte ranges
// ---------------------------------------------------------------------------

// Segment is one playable run of bytes with a known duration.
type Segment struct {
	Extents  []wittydvd.FileExtent
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
func SubfileURL(e wittydvd.FileExtent) string {
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
		if len(pieces) == 1 {
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

// maxSubfile is the most one subfile address reads. Past 2 GiB in one FFmpeg
// misjudges a program stream's end, and then loses about a second of its start.
const maxSubfile = 1 << 30

// splitExtents cuts extents into pieces that FFmpeg reads reliably.
func splitExtents(ext []wittydvd.FileExtent) []wittydvd.FileExtent {
	var out []wittydvd.FileExtent
	for _, e := range ext {
		for e.Length > maxSubfile {
			out = append(out, wittydvd.FileExtent{File: e.File, Offset: e.Offset, Length: maxSubfile})
			e.Offset += maxSubfile
			e.Length -= maxSubfile
		}
		out = append(out, e)
	}
	return out
}

// ConcatInputArgs are the ffmpeg input flags for an ffconcat list of subfiles.
func ConcatInputArgs(list string) []string {
	return []string{
		"-f", "concat", "-safe", "0",
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
		return fmt.Errorf("ffmpeg still: %w: %s", err, stderr.String())
	}
	return nil
}

// ImageThumb makes a JPEG of a picture, at most width pixels along its longer
// side, upright according to its EXIF orientation (1 to 8; 0 means as stored).
// shrink halves a JPEG that many times while decoding (0 to 3). Rotation is
// always done here rather than by FFmpeg, whose handling differs by version.
func (t *Tools) ImageThumb(ctx context.Context, path string, orientation, width, shrink int, out string) error {
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
		return fmt.Errorf("ffmpeg image: %w: %s", err, stderr.String())
	}
	return nil
}

// Command prepares an ffmpeg/ffprobe process. On Windows it never opens a
// console window (the server often runs without one, from the tray).
func Command(ctx context.Context, path string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, path, args...)
	hideWindow(cmd)
	return cmd
}

// Run runs a short FFmpeg or ffprobe job and waits for it, returning its output
// and what it said. It leaves a record of how the job ended, tied to the session
// in ctx (see logging.WithSession); attrs say what it was for.
func Run(ctx context.Context, path string, args []string, attrs ...any) ([]byte, *logging.Tail, error) {
	cmd := Command(ctx, path, args...)
	stderr := &logging.Tail{}
	cmd.Stderr = stderr
	started := time.Now()
	out, err := cmd.Output()
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
		out[i] = urlCreds.ReplaceAllString(a, "$1(hidden)@")
	}
	return strings.Join(out, " ")
}
