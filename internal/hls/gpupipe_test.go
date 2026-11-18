// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package hls

import (
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

var discard = slog.New(slog.DiscardHandler)

var allPaths = []string{"h264_nvenc", "h264_qsv", "h264_vaapi", "pipeline:cuda", "pipeline:qsv", "pipeline:vaapi", "tonemap:libplacebo"}

// line is the command line a new stream of sp gets with encoder enc allowed, and what it uses of the chip.
func line(t *testing.T, m *Manager, enc string, sp Spec) (string, hwChoice) {
	t.Helper()
	sp.Quality.HW = []string{enc}
	s := &Session{Spec: sp, log: discard, Dir: t.TempDir(), tools: m.tools, m: m}
	args, _, _, hc := s.commandLine(0)
	return strings.Join(args, " "), hc
}

func gpuManager() *Manager {
	tools := &ffmpeg.Tools{Encoders: map[string]bool{"libx264": true, "h264_nvenc": true, "h264_qsv": true, "h264_vaapi": true}}
	return &Manager{tools: tools, log: discard, root: os.TempDir()}
}

var gpuBase = Spec{InputArgs: []string{"-i", "in.mkv"}, Duration: 60, SegmentSec: 4, AudioIndex: 0, SubtitleIndex: -1, VideoCodec: "h264", PixFmt: "yuv420p", MaxHeight: 720}

// With a hardware encoder and a picture its chip decodes, the picture stays on the chip: decoded
// there, brought to size there and encoded there. What the chip can't do keeps the old way.
func TestGPUPipeline(t *testing.T) {
	defer ffmpeg.PretendWorking(allPaths...)()
	m := gpuManager()
	base := gpuBase

	got, hc := line(t, m, "h264_nvenc", base)
	for _, want := range []string{"-hwaccel cuda -hwaccel_output_format cuda", "scale_cuda=w='trunc(iw*sar*oh/ih/2)*2':h='trunc(min(ih,720)/2)*2':format=nv12,setsar=1 -c:v h264_nvenc"} {
		if !strings.Contains(got, want) {
			t.Errorf("NVENC: want %q in\n%s", want, got)
		}
	}
	if !hc.onGPU || strings.Contains(got, "hwdownload") || strings.Contains(got, "format=yuv420p") || strings.Contains(got, "-hwaccel auto") {
		t.Errorf("NVENC: the picture should stay on the chip:\n%s", got)
	}
	if got, _ := line(t, m, "h264_qsv", base); !strings.Contains(got, "-hwaccel qsv -hwaccel_output_format qsv") || !strings.Contains(got, "scale_qsv=") {
		t.Errorf("Quick Sync:\n%s", got)
	}
	if got, _ := line(t, m, "h264_vaapi", base); !strings.Contains(got, "-hwaccel vaapi -hwaccel_output_format vaapi") || !strings.Contains(got, "scale_vaapi=") || strings.Contains(got, "hwupload") {
		t.Errorf("VA-API: decoded, scaled and encoded on the chip, with nothing to upload:\n%s", got)
	}

	// Interlaced: the chip's own deinterlacer, before the picture is resized.
	inter := base
	inter.VideoFilter = "bwdif=mode=send_frame:parity=auto:deint=interlaced"
	if got, _ := line(t, m, "h264_nvenc", inter); !strings.Contains(got, "bwdif_cuda=mode=send_frame:parity=auto:deint=interlaced,scale_cuda=") {
		t.Errorf("NVENC deinterlaces on the chip:\n%s", got)
	}
	if got, hc := line(t, m, "h264_qsv", inter); hc.onGPU || !strings.Contains(got, "bwdif=") {
		t.Errorf("Quick Sync has no bwdif of its own here, so the processor does it all:\n%s", got)
	}

	// HDR: resized on the chip in ten bits, brought out and mapped on the chip by libplacebo.
	hdr := base
	hdr.VideoCodec, hdr.PixFmt, hdr.ToneMap, hdr.MaxHeight = "hevc", "yuv420p10le", "pq", 1080
	got, hc = line(t, m, "h264_nvenc", hdr)
	if !hc.placebo || !strings.Contains(got, "-init_hw_device vulkan=vk -filter_hw_device vk") ||
		!strings.Contains(got, "format=p010le,hwdownload,format=p010le,setparams=") || !strings.Contains(got, "libplacebo=tonemapping=hable") {
		t.Errorf("HDR: scaled on the chip, mapped by libplacebo:\n%s", got)
	}
	if strings.Contains(got, "zscale") || strings.Contains(got, "libplacebo=w=") {
		t.Errorf("HDR: already brought to size, and not by the processor:\n%s", got)
	}
	// libx264 still has its HDR mapped on the chip, resizing on the way.
	if got, hc := line(t, m, "libx264", hdr); !hc.placebo || hc.onGPU || !strings.Contains(got, "libplacebo=w='trunc(iw*sar*oh/ih/2)*2':h='trunc(min(ih,1080)/2)*2':") || strings.Contains(got, "zscale") {
		t.Errorf("software encoding maps HDR on the chip:\n%s", got)
	}
	// Without a working Vulkan device: the processor's chain.
	func() {
		defer ffmpeg.PretendWorking("h264_nvenc", "pipeline:cuda")()
		if got, hc := line(t, m, "h264_nvenc", hdr); hc.placebo || !strings.Contains(got, "zscale=t=linear") || strings.Contains(got, "vulkan") {
			t.Errorf("no libplacebo: the processor maps:\n%s", got)
		}
	}()

	subs := base
	subs.SubtitleIndex, subs.SubtitleText, subs.Source = 0, true, "in.mkv"
	if got, _ := line(t, m, "h264_nvenc", subs); !strings.Contains(got, "format=nv12,hwdownload,format=nv12,subtitles=") {
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
		if got, hc := line(t, m, "h264_nvenc", sp); hc.onGPU || !strings.Contains(got, "-hwaccel auto") {
			t.Errorf("%s: decoded as before:\n%s", name, got)
		}
	}
	if got, hc := line(t, m, "libx264", base); hc.onGPU || strings.Contains(got, "-hwaccel") {
		t.Errorf("software encoding is unchanged:\n%s", got)
	}
	// A path that wasn't found working, or that the owner turned off, isn't used.
	func() {
		defer ffmpeg.PretendWorking("h264_nvenc")()
		if _, hc := line(t, m, "h264_nvenc", base); hc.onGPU {
			t.Error("an untested pipeline is used")
		}
	}()
	ffmpeg.SetGPUPathsOff([]string{"pipeline:cuda"})
	if _, hc := line(t, m, "h264_nvenc", base); hc.onGPU {
		t.Error("a pipeline the owner turned off is used")
	}
	ffmpeg.SetGPUPathsOff(nil)
}

// A job that fails using the chip takes its stream a step down: first the processor decodes, then
// the processor encodes too. A failure before anything was made stops other streams of that kind
// trying the chip; running out of the chip's room doesn't.
func TestGPUFallsBack(t *testing.T) {
	defer ffmpeg.PretendWorking(allPaths...)()
	m := gpuManager()
	sp := gpuBase
	sp.VideoCodec, sp.Quality.HW = "hevc", []string{"h264_nvenc"}
	s := &Session{Spec: sp, log: discard, Dir: t.TempDir(), tools: m.tools, m: m}
	_, _, _, hc := s.commandLine(0)
	if !hc.onGPU {
		t.Fatal("expected the GPU pipeline")
	}
	s.hwExit(&job{hw: hc, err: errors.New("exit status 1")})
	args, _, _, hc2 := s.commandLine(0)
	if got := strings.Join(args, " "); hc2.onGPU || strings.Contains(got, "-hwaccel") || !strings.Contains(got, "-c:v h264_nvenc") {
		t.Errorf("after a failure the processor decodes, for NVENC:\n%s", got)
	}
	if _, hc := line(t, m, "h264_nvenc", sp); hc.onGPU {
		t.Error("another stream of the same kind doesn't try the failed pipeline again")
	}
	diff := sp
	diff.VideoCodec = "h264"
	if _, hc := line(t, m, "h264_nvenc", diff); !hc.onGPU {
		t.Error("a different kind of picture still tries the chip")
	}
	// The second failure: the processor does it all.
	s.hwExit(&job{hw: hc2, err: errors.New("exit status 1")})
	if args, _, _, _ := s.commandLine(0); !strings.Contains(strings.Join(args, " "), "-c:v libx264") {
		t.Errorf("after the encoder failed too, libx264:\n%s", strings.Join(args, " "))
	}

	// Out of the chip's room: this stream steps down, others still try.
	m2 := gpuManager()
	s2 := &Session{Spec: sp, log: discard, Dir: t.TempDir(), tools: m2.tools, m: m2}
	_, _, _, hc = s2.commandLine(0)
	full := &job{hw: hc, err: errors.New("exit status 1")}
	full.stderr.Write([]byte("[h264_nvenc] OpenEncodeSessionEx failed: incompatible client key (21)"))
	s2.hwExit(full)
	if s2.fallback.Load() != levelCPUDecode {
		t.Error("the stream steps down")
	}
	if _, hc := line(t, m2, "h264_nvenc", sp); !hc.onGPU {
		t.Error("a chip that was only full is still tried by other streams")
	}

	// Stopped on purpose: says nothing about the chip.
	s3 := &Session{Spec: diff, log: discard, Dir: t.TempDir(), tools: m2.tools, m: m2}
	j := &job{hw: hwChoice{key: "x", onGPU: true}, err: errors.New("killed")}
	j.stopped.Store(true)
	s3.hwExit(j)
	if s3.fallback.Load() != levelGPU {
		t.Error("a job stopped on purpose isn't a failure of the chip")
	}
}

// NVENC's sessions are limited: past the limit a new job takes the next encoder allowed, or the
// processor.
func TestEncoderCap(t *testing.T) {
	defer ffmpeg.PretendWorking(allPaths...)()
	m := gpuManager()
	for range hwCaps["h264_nvenc"] {
		m.hwStarted("h264_nvenc")
	}
	sp := gpuBase
	sp.Quality.HW = []string{"h264_nvenc", "h264_qsv"}
	s := &Session{Spec: sp, log: discard, Dir: t.TempDir(), tools: m.tools, m: m}
	if _, enc, _, _ := s.commandLine(0); enc != "h264_qsv" {
		t.Errorf("NVENC full: the next encoder, not %s", enc)
	}
	sp.Quality.HW = []string{"h264_nvenc"}
	s = &Session{Spec: sp, log: discard, Dir: t.TempDir(), tools: m.tools, m: m}
	if _, enc, _, _ := s.commandLine(0); enc != "libx264" {
		t.Errorf("NVENC full and nothing else allowed: the processor, not %s", enc)
	}
	m.hwEnded("h264_nvenc")
	if _, enc, _, _ := s.commandLine(0); enc != "h264_nvenc" {
		t.Errorf("room again: NVENC, not %s", enc)
	}
}

// A channel's run of a library file decodes, deinterlaces and resizes on the chip too; one that
// failed there steps the channel down.
func TestLiveGPUPipeline(t *testing.T) {
	defer ffmpeg.PretendWorking(allPaths...)()
	m := gpuManager()
	s := m.newLive(LiveSpec{ID: "g", SegmentSec: 4, MaxHeight: 720, Quality: ffmpeg.Quality{HW: []string{"h264_nvenc"}}})
	in := &LiveInput{InputArgs: ffmpeg.FileInput("/m/a.ts"), AudioIndex: 0, VideoCodec: "mpeg2video", PixFmt: "yuv420p", VideoFilter: "bwdif=mode=send_frame:parity=auto:deint=interlaced"}
	args, hc := s.liveArgsHW(in, 1, false)
	got := strings.Join(args, " ")
	if !hc.onGPU || !strings.Contains(got, "-hwaccel cuda -hwaccel_output_format cuda") || !strings.Contains(got, "bwdif_cuda=mode=send_frame:parity=auto:deint=interlaced,scale_cuda=") {
		t.Errorf("a channel's file on the chip:\n%s", got)
	}
	s.hwFailed(hc, true, "Error while filtering")
	if args, hc := s.liveArgsHW(in, 2, false); hc.onGPU || strings.Contains(strings.Join(args, " "), "-hwaccel") {
		t.Errorf("after a failure the next run decodes on the processor:\n%s", strings.Join(args, " "))
	}
	tuner := &LiveInput{InputArgs: []string{"-f", "mpegts", "-i", "pipe:0"}, AudioIndex: 0, VideoFilter: "bwdif=mode=send_frame:parity=auto:deint=interlaced"}
	if _, hc := m.newLive(LiveSpec{ID: "h", SegmentSec: 4, Quality: ffmpeg.Quality{HW: []string{"h264_nvenc"}}}).liveArgsHW(tuner, 1, false); hc.onGPU {
		t.Error("a tuner's picture isn't known ahead, so it isn't sent to the chip's decoder")
	}
}
