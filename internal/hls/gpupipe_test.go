// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package hls

import (
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

var discard = slog.New(slog.DiscardHandler)

// With a hardware encoder and a picture its chip decodes, the picture stays on the chip: decoded
// there, brought to size there and encoded there. What the chip can't do keeps the old way.
func TestGPUPipeline(t *testing.T) {
	defer ffmpeg.PretendWorking("h264_nvenc", "h264_qsv", "h264_vaapi")()
	line := func(enc string, sp Spec) (string, string) {
		tools := &ffmpeg.Tools{Encoders: map[string]bool{"libx264": true, enc: true}}
		sp.Quality.HW = []string{enc}
		s := &Session{Spec: sp, log: discard, Dir: t.TempDir(), tools: tools, m: &Manager{tools: tools}}
		args, _, _, gpu := s.commandLine(0)
		return strings.Join(args, " "), gpu
	}
	base := Spec{InputArgs: []string{"-i", "in.mkv"}, Duration: 60, SegmentSec: 4, AudioIndex: 0, SubtitleIndex: -1, VideoCodec: "h264", PixFmt: "yuv420p", MaxHeight: 720}

	got, gpu := line("h264_nvenc", base)
	for _, want := range []string{"-hwaccel cuda -hwaccel_output_format cuda", "scale_cuda=w='trunc(iw*sar*oh/ih/2)*2':h='trunc(min(ih,720)/2)*2':format=nv12,setsar=1 -c:v h264_nvenc"} {
		if !strings.Contains(got, want) {
			t.Errorf("NVENC: want %q in\n%s", want, got)
		}
	}
	if gpu == "" || strings.Contains(got, "hwdownload") || strings.Contains(got, "format=yuv420p") || strings.Contains(got, "-hwaccel auto") {
		t.Errorf("NVENC: the picture should stay on the chip (gpu %q):\n%s", gpu, got)
	}

	if got, _ := line("h264_qsv", base); !strings.Contains(got, "-hwaccel qsv -hwaccel_output_format qsv") || !strings.Contains(got, "scale_qsv=") {
		t.Errorf("Quick Sync:\n%s", got)
	}
	if got, _ := line("h264_vaapi", base); !strings.Contains(got, "-hwaccel vaapi -hwaccel_output_format vaapi") || !strings.Contains(got, "scale_vaapi=") || strings.Contains(got, "hwupload") {
		t.Errorf("VA-API: decoded, scaled and encoded on the chip, with nothing to upload:\n%s", got)
	}

	// Interlaced: the chip's own deinterlacer, before the picture is resized.
	inter := base
	inter.VideoFilter = "bwdif=mode=send_frame:parity=auto:deint=interlaced"
	if got, _ := line("h264_nvenc", inter); !strings.Contains(got, "bwdif_cuda=mode=send_frame:parity=auto:deint=interlaced,scale_cuda=") {
		t.Errorf("NVENC deinterlaces on the chip:\n%s", got)
	}
	if got, gpu := line("h264_qsv", inter); gpu != "" || !strings.Contains(got, "bwdif=") {
		t.Errorf("Quick Sync has no bwdif of its own here, so the processor does it all:\n%s", got)
	}

	// HDR mapping and subtitles are the processor's: resized on the chip in ten bits, then brought out.
	hdr := base
	hdr.VideoCodec, hdr.PixFmt, hdr.ToneMap, hdr.MaxHeight = "hevc", "yuv420p10le", "pq", 1080
	got, _ = line("h264_nvenc", hdr)
	if !strings.Contains(got, "format=p010le,hwdownload,format=p010le,setparams=") || !strings.Contains(got, "format=yuv420p -c:v h264_nvenc") {
		t.Errorf("HDR: scaled on the chip, mapped by the processor:\n%s", got)
	}
	if strings.Contains(got, "zscale=w=") {
		t.Errorf("HDR: already brought to size, the mapping mustn't resize again:\n%s", got)
	}
	subs := base
	subs.SubtitleIndex, subs.SubtitleText, subs.Source = 0, true, "in.mkv"
	if got, _ := line("h264_nvenc", subs); !strings.Contains(got, "format=nv12,hwdownload,format=nv12,subtitles=") {
		t.Errorf("subtitles are drawn by the processor, after the chip:\n%s", got)
	}

	// What the chip doesn't decode, or nothing known about the picture: as before.
	for name, sp := range map[string]Spec{
		"10-bit H.264":    func() Spec { s := base; s.PixFmt = "yuv420p10le"; return s }(),
		"4:2:2":           func() Spec { s := base; s.PixFmt = "yuv422p"; return s }(),
		"MPEG-4 part 2":   func() Spec { s := base; s.VideoCodec = "mpeg4"; return s }(),
		"not known":       func() Spec { s := base; s.VideoCodec = ""; return s }(),
		"cut stretches":   func() Spec { s := base; s.Holes = []Hole{{At: 1, Length: 2}}; return s }(),
		"held last frame": func() Spec { s := base; s.HoldLastFrame = true; return s }(),
	} {
		if got, gpu := line("h264_nvenc", sp); gpu != "" || !strings.Contains(got, "-hwaccel auto") {
			t.Errorf("%s: decoded as before:\n%s", name, got)
		}
	}
	if got, gpu := line("libx264", base); gpu != "" || strings.Contains(got, "-hwaccel") {
		t.Errorf("software encoding is unchanged:\n%s", got)
	}
}

// A GPU pipeline that fails before its first segment turns itself off, for that stream and for
// others of the same kind, and the stream is made again on the processor.
func TestGPUPipelineFallsBack(t *testing.T) {
	defer ffmpeg.PretendWorking("h264_nvenc")()
	tools := &ffmpeg.Tools{Encoders: map[string]bool{"libx264": true, "h264_nvenc": true}}
	m := &Manager{tools: tools}
	sp := Spec{InputArgs: []string{"-i", "in.mkv"}, Duration: 60, SegmentSec: 4, AudioIndex: 0, SubtitleIndex: -1, VideoCodec: "hevc", PixFmt: "yuv420p", Quality: ffmpeg.Quality{HW: []string{"h264_nvenc"}}}
	s := &Session{Spec: sp, log: discard, Dir: t.TempDir(), tools: tools, m: m}
	_, _, _, gpu := s.commandLine(0)
	if gpu == "" {
		t.Fatal("expected the GPU pipeline")
	}
	s.gpuExit(&job{gpu: gpu, err: errors.New("exit status 1")})
	if args, _, _, again := s.commandLine(0); again != "" || !strings.Contains(strings.Join(args, " "), "-hwaccel auto") {
		t.Errorf("after a failure the stream decodes on the processor:\n%s", strings.Join(args, " "))
	}
	other := &Session{Spec: sp, log: discard, Dir: t.TempDir(), tools: tools, m: m}
	if _, _, _, g := other.commandLine(0); g != "" {
		t.Error("another stream of the same kind doesn't try the failed pipeline again")
	}
	diff := sp
	diff.VideoCodec = "h264"
	if _, _, _, g := (&Session{Spec: diff, log: discard, Dir: t.TempDir(), tools: tools, m: m}).commandLine(0); g == "" {
		t.Error("a different kind of picture still tries the chip")
	}
	// Stopped on purpose, or failing after segments were made: says nothing about the chip.
	fresh := &Session{Spec: diff, log: discard, Dir: t.TempDir(), tools: tools, m: &Manager{tools: tools}}
	j := &job{gpu: "x", err: errors.New("killed")}
	j.stopped.Store(true)
	fresh.gpuExit(j)
	if fresh.noGPU.Load() {
		t.Error("a job stopped on purpose isn't a failure of the chip")
	}
}
