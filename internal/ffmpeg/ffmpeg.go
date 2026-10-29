// Package ffmpeg locates and drives the bundled FFmpeg binaries.
package ffmpeg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/disc/dvd"
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
	candidates = append(candidates, ".tools/ffmpeg-master-latest-win64-gpl/bin")
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
	if out, err := exec.CommandContext(ctx, t.FFmpeg, "-hide_banner", "-version").Output(); err == nil {
		if line, _, ok := bytes.Cut(out, []byte("\n")); ok {
			t.Version = strings.TrimSpace(string(line))
		}
	}
	if out, err := exec.CommandContext(ctx, t.FFmpeg, "-hide_banner", "-encoders").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			f := strings.Fields(line)
			if len(f) >= 2 && len(f[0]) == 6 && f[0][0] == 'V' || len(f) >= 2 && len(f[0]) == 6 && f[0][0] == 'A' {
				t.Encoders[f[1]] = true
			}
		}
	}
	return t, nil
}

// H264Encoder picks the best available H.264 encoder.
func (t *Tools) H264Encoder(preferHW bool) (name string, args []string) {
	if preferHW {
		switch {
		case t.Encoders["h264_nvenc"]:
			return "h264_nvenc", []string{"-preset", "p4", "-tune", "ll", "-rc", "vbr", "-cq", "23", "-b:v", "0"}
		case t.Encoders["h264_qsv"]:
			return "h264_qsv", []string{"-preset", "veryfast", "-global_quality", "23"}
		case t.Encoders["h264_videotoolbox"]:
			return "h264_videotoolbox", []string{"-q:v", "60", "-realtime", "1"}
		case t.Encoders["h264_vaapi"]:
			return "h264_vaapi", []string{"-qp", "23"}
		case t.Encoders["h264_amf"]:
			return "h264_amf", []string{"-quality", "speed", "-rc", "cqp", "-qp_i", "22", "-qp_p", "24"}
		}
	}
	if t.Encoders["libx264"] {
		return "libx264", []string{"-preset", "veryfast", "-crf", "21", "-profile:v", "high", "-level", "4.1", "-pix_fmt", "yuv420p", "-x264-params", "scenecut=0:open_gop=0"}
	}
	if t.Encoders["libopenh264"] {
		return "libopenh264", []string{"-b:v", "6M", "-pix_fmt", "yuv420p", "-profile:v", "high"}
	}
	return "mpeg4", []string{"-q:v", "4"}
}

// ---------------------------------------------------------------------------
// Probing
// ---------------------------------------------------------------------------

type Stream struct {
	Index       int               `json:"index"`
	ID          string            `json:"id"`
	CodecType   string            `json:"codec_type"`
	CodecName   string            `json:"codec_name"`
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
}

type Format struct {
	Filename   string            `json:"filename"`
	FormatName string            `json:"format_name"`
	Duration   string            `json:"duration"`
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

// ProbeFile runs ffprobe on a regular media file.
func (t *Tools) ProbeFile(ctx context.Context, path string) (*Probe, error) {
	return t.probe(ctx, []string{path})
}

// ProbeInput runs ffprobe on arbitrary input args (e.g. a concat list).
func (t *Tools) ProbeInput(ctx context.Context, inputArgs []string) (*Probe, error) {
	return t.probe(ctx, inputArgs)
}

func (t *Tools) probe(ctx context.Context, input []string) (*Probe, error) {
	args := []string{"-v", "error", "-print_format", "json", "-show_format", "-show_streams", "-show_chapters"}
	args = append(args, input...)
	cmd := exec.CommandContext(ctx, t.FFprobe, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %w: %s", err, strings.TrimSpace(stderr.String()))
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
	Extents  []dvd.FileExtent
	Duration float64 // seconds, 0 = unknown
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
func WriteConcat(dir, name string, segs []Segment) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("ffconcat version 1.0\n")
	for i, s := range segs {
		var url string
		if len(s.Extents) == 1 {
			url = SubfileURL(s.Extents[0])
		} else {
			side := filepath.Join(dir, fmt.Sprintf("%s-seg%d.txt", name, i+1))
			var lines []string
			for _, e := range s.Extents {
				lines = append(lines, SubfileURL(e))
			}
			if err := os.WriteFile(side, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
				return "", err
			}
			url = "concatf:" + filepath.ToSlash(side)
		}
		fmt.Fprintf(&b, "file %s\n", quoteConcat(url))
		if s.Duration > 0 {
			fmt.Fprintf(&b, "duration %.3f\n", s.Duration)
		}
	}
	p := filepath.Join(dir, name+".ffconcat")
	return p, os.WriteFile(p, []byte(b.String()), 0o644)
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
	args = append(args, "-an", "-sn", "-frames:v", "1", "-vf", vf, "-q:v", "3", out)
	cmd := exec.CommandContext(ctx, t.FFmpeg, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg still: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
