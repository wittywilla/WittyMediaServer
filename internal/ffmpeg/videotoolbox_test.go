// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package ffmpeg

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// What an Intel Mac gets, and what Apple silicon gets, are both plain options.
func TestVideoToolboxOptionsForEachChip(t *testing.T) {
	tools := &Tools{Encoders: map[string]bool{}}
	defer func(was bool) { VideoToolboxQuality = was }(VideoToolboxQuality)

	VideoToolboxQuality = true
	if args := tools.planFor("h264_videotoolbox", Quality{CRF: 21}).Args; !slices.Equal(args, []string{"-q:v", "60", "-realtime", "1"}) {
		t.Errorf("Apple silicon: %v", args)
	}
	VideoToolboxQuality = false
	args := tools.planFor("h264_videotoolbox", Quality{CRF: 21}).Args
	if slices.Contains(args, "-q:v") || !slices.Contains(args, "-b:v") {
		t.Errorf("Intel: FFmpeg refuses -q:v there, so it needs a bitrate: %v", args)
	}
	// A limit still wins over the bitrate.
	if args := tools.planFor("h264_videotoolbox", Quality{CapRate: "4000k"}).Args; !slices.Equal(args, []string{"-b:v", "3200k", "-realtime", "1", "-maxrate", "4000k", "-bufsize", "8000k"}) {
		t.Errorf("with a limit: %v", args)
	}
}

// A real stream through VideoToolbox, with the options the server gives it, on a Mac whose
// FFmpeg has it. Both ways of setting it are tried where the chip allows them.
func TestVideoToolboxReallyEncodes(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("VideoToolbox is the Mac's")
	}
	tools := realTools(t)
	out, err := exec.Command(tools.FFmpeg, "-hide_banner", "-encoders").Output()
	if err != nil || !strings.Contains(string(out), "h264_videotoolbox") {
		t.Skip("this FFmpeg has no VideoToolbox")
	}
	tools.Encoders = map[string]bool{"h264_videotoolbox": true}
	defer func(was bool) { VideoToolboxQuality = was }(VideoToolboxQuality)
	modes := []bool{false}
	if runtime.GOARCH == "arm64" {
		modes = append(modes, true)
	}
	for _, quality := range modes {
		VideoToolboxQuality = quality
		for _, q := range []Quality{{CRF: 21}, {CapRate: "2000k"}} {
			plan := tools.planFor("h264_videotoolbox", q)
			dir := t.TempDir()
			args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
				"-f", "lavfi", "-i", "testsrc=s=1280x720:r=30:d=2",
				"-vf", "setsar=1,format=" + plan.PixFmt, "-c:v", plan.Name}
			args = append(args, plan.Args...)
			args = append(args, "-force_key_frames", "expr:gte(t,n_forced*1)", "-g", "30",
				"-f", "hls", "-hls_time", "1", "-hls_segment_type", "fmp4", "-hls_fmp4_init_filename", "init.mp4",
				"-hls_list_size", "0", "-hls_segment_filename", filepath.Join(dir, "seg%05d.m4s"), filepath.Join(dir, "out.m3u8"))
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			cmd := exec.CommandContext(ctx, tools.FFmpeg, args...)
			cmd.Dir = dir
			msg, err := cmd.CombinedOutput()
			cancel()
			if err != nil {
				t.Fatalf("quality %v, %+v: %v\n%s\n%v", quality, q, err, msg, plan.Args)
			}
			probe, err := exec.Command(tools.FFprobe, "-v", "error", "-show_entries", "stream=codec_name,width,height", "-of", "csv=p=0",
				filepath.Join(dir, "init.mp4")).Output()
			if err != nil || strings.TrimSpace(string(probe)) != "h264,1280,720" {
				t.Errorf("quality %v, %+v: %q %v", quality, q, probe, err)
			}
			if segs, _ := filepath.Glob(filepath.Join(dir, "seg*.m4s")); len(segs) < 2 {
				t.Errorf("quality %v, %+v: %d segments", quality, q, len(segs))
			}
			if _, err := os.Stat(filepath.Join(dir, "out.m3u8")); err != nil {
				t.Error(err)
			}
		}
	}
}
