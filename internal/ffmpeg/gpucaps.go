// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package ffmpeg

import (
	"context"
	"fmt"
	"github.com/wittywilla/wittymediaserver/internal/i18n"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// GPU capabilities: what the graphics chip really does on this machine, found by giving each
// way of using it a tiny real job (a few frames) rather than by trusting what the driver or
// FFmpeg's lists claim. The hardware encoders are HWStatus; GPUPath is everything else the chip
// can take over from the processor. Both are found by ProbeHardware, once, in the background,
// and both say why when something can't be used.
//
// For other parts of the server (WittyPlay's game streams, the stream quality menu): ask
// HardwareStatus for the encoders, GPUPaths for the rest, GPUPathWorks before using a path, and
// GPUPipeline and GPUToneMap for the FFmpeg options themselves. Everything falls back to the
// processor when a path isn't there, and a caller that sees a path fail during a job uses the
// processor for that job (see internal/hls).

// GPUPath is one way of using the graphics chip besides its encoder.
type GPUPath struct {
	ID    string `json:"id"`    // "pipeline:cuda", "tonemap:libplacebo", …
	Label string `json:"label"` // for people
	Kind  string `json:"kind"`  // "pipeline": decode and resize on the chip, feeding Encoder; "tonemap": HDR to SDR
	// Encoder is the hardware encoder a pipeline feeds; the pipeline is only tested, and only
	// used, when that encoder works.
	Encoder string `json:"encoder,omitempty"`
	Tested  bool   `json:"tested"`
	Working bool   `json:"working"`
	// Off: the owner turned it off in Settings (gpuPathsOff), whatever the test said.
	Off    bool   `json:"off"`
	Device string `json:"device,omitempty"` // the chip it ran on, when FFmpeg says
	Reason string `json:"reason,omitempty"` // why it can't be used, for people
	Error  string `json:"error,omitempty"`  // FFmpeg's last words, when it failed
}

// GPUPathList is every path known, in the order Settings shows them.
var GPUPathList = []GPUPath{
	{ID: "pipeline:cuda", Label: "NVIDIA: decode and resize on the chip (CUDA)", Kind: "pipeline", Encoder: "h264_nvenc"},
	{ID: "pipeline:qsv", Label: "Intel: decode and resize on the chip (Quick Sync)", Kind: "pipeline", Encoder: "h264_qsv"},
	{ID: "pipeline:vaapi", Label: "Intel and AMD on Linux: decode and resize on the chip (VA-API)", Kind: "pipeline", Encoder: "h264_vaapi"},
	{ID: "pipeline:videotoolbox", Label: "Apple: decode on the chip (VideoToolbox)", Kind: "pipeline", Encoder: "h264_videotoolbox"},
	{ID: "tonemap:libplacebo", Label: "HDR to an ordinary picture on the chip (Vulkan)", Kind: "tonemap"},
}

// GPUPaths reports every path (testing still in progress shows as Tested=false).
func (t *Tools) GPUPaths() []GPUPath {
	hw.mu.Lock()
	defer hw.mu.Unlock()
	out := make([]GPUPath, 0, len(GPUPathList))
	for _, p := range GPUPathList {
		if st, ok := hw.paths[p.ID]; ok {
			p = *st
		}
		p.Off = hw.pathsOff[p.ID]
		out = append(out, p)
	}
	return out
}

// GPUPathWorks says a path was tested here, works, and isn't turned off.
func GPUPathWorks(id string) bool {
	hw.mu.Lock()
	defer hw.mu.Unlock()
	st, ok := hw.paths[id]
	return ok && st.Working && !hw.pathsOff[id]
}

// SetGPUPathsOff is the owner's choice of paths not to use (the gpuPathsOff setting), for
// streams started from now on.
func SetGPUPathsOff(ids []string) {
	off := map[string]bool{}
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			off[id] = true
		}
	}
	hw.mu.Lock()
	hw.pathsOff = off
	hw.mu.Unlock()
}

// GPUPipeline is the pipeline for this encoder and source picture when it was found working
// here (see GPUPipeFor for what each chip takes).
func GPUPipeline(encoder, codec, pixFmt string) (GPUPipe, bool) {
	p, ok := GPUPipeFor(encoder, codec, pixFmt)
	if !ok || !GPUPathWorks(p.ID()) {
		return GPUPipe{}, false
	}
	return p, true
}

// VulkanDevice are the global options that give the filters a Vulkan device, for GPUToneMap.
var VulkanDevice = []string{"-init_hw_device", "vulkan=vk", "-filter_hw_device", "vk"}

// GPUToneMap maps HDR of this range (as Stream.Range says: "pq" or "hlg") to an ordinary
// picture on the chip with libplacebo, bringing it to at most maxHeight lines on the way (0 =
// as it is, for a picture already brought to size), when that was found working here. It takes
// frames in the computer's memory (any format) and gives them back as nv12; the command needs
// VulkanDevice among its global options. Mapping on the processor takes floating point on
// every pixel: 83 processor seconds for 30 seconds of a 4K HDR film brought to 1080p, against
// 2.3 seconds this way, and quicker as well.
func GPUToneMap(rng string, maxHeight int) (string, bool) {
	if !GPUPathWorks("tonemap:libplacebo") {
		return "", false
	}
	return placeboChain(rng, maxHeight), true
}

func placeboChain(rng string, maxHeight int) string {
	trc := "smpte2084"
	if rng == "hlg" {
		trc = "arib-std-b67"
	}
	size := ""
	if maxHeight > 0 {
		size = fmt.Sprintf("w='trunc(iw*sar*oh/ih/2)*2':h='trunc(min(ih,%d)/2)*2':", maxHeight)
	}
	// Told what the frames are first, as for the processor's chain (a file may only say so in its
	// container), and that what comes out is an ordinary picture afterwards.
	return "setparams=color_primaries=bt2020:color_trc=" + trc + ":colorspace=bt2020nc," +
		"libplacebo=" + size + "tonemapping=hable:colorspace=bt709:color_primaries=bt709:color_trc=bt709:range=tv:format=nv12," +
		"hwdownload,format=nv12,setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709"
}

// vulkanSelected finds the device FFmpeg chose in its verbose output; cpu says it is a software
// one (llvmpipe), which would do the work on the processor after all.
var vulkanSelected = regexp.MustCompile(`Device \d+ selected: ([^\n]+?) \((integrated|discrete|virtual|cpu|other)\)`)

// probePaths tests every path whose encoder (if it has one) works. A pipeline is given a tiny
// file to decode, resize and encode; the tone map a few frames of HDR test picture.
func (t *Tools) probePaths(ctx context.Context, log *slog.Logger) {
	dir, err := os.MkdirTemp("", "wms-gpu-probe-")
	if err != nil {
		log.Warn("graphics chip tests: no temporary folder", "err", err)
		return
	}
	defer os.RemoveAll(dir)
	sample := ""
	for _, p := range GPUPathList {
		st := p
		switch {
		case p.Kind == "pipeline" && !hwWorking(p.Encoder):
			st.Reason = "Its encoder isn't working here."
		case p.Kind == "pipeline" && !t.HasFilter("scale_"+strings.TrimPrefix(p.ID, "pipeline:")) && p.ID != "pipeline:videotoolbox":
			st.Reason = "This FFmpeg has no scale_" + strings.TrimPrefix(p.ID, "pipeline:") + " filter."
		case p.Kind == "tonemap" && !t.HasFilter("libplacebo"):
			st.Reason = "This FFmpeg has no libplacebo filter."
		case p.Kind == "pipeline":
			if sample == "" {
				if sample, err = t.probeSample(ctx, dir); err != nil {
					st.Reason, st.Error = "The test picture couldn't be made.", err.Error()
					break
				}
			}
			t.testPath(ctx, log, &st, t.pipelineTest(p, sample))
		default:
			t.testPath(ctx, log, &st, append(slices.Clone(VulkanDevice),
				"-f", "lavfi", "-i", "testsrc2=s=320x240:r=25:d=0.2,format=yuv420p10le",
				"-vf", placeboChain("pq", 0), "-c:v", "rawvideo", "-f", "null", "-"))
		}
		hw.mu.Lock()
		hw.paths[p.ID] = &st
		hw.mu.Unlock()
	}
}

// probeSample makes the little file the pipelines decode: H.264 when this FFmpeg can make it,
// which is what most files are, else MPEG-2, which every FFmpeg makes and every chip decodes.
func (t *Tools) probeSample(ctx context.Context, dir string) (string, error) {
	out := filepath.Join(dir, "sample.mkv")
	enc := []string{"-c:v", "mpeg2video"}
	if t.Encoders["libx264"] {
		enc = []string{"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p"}
	}
	args := append([]string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-f", "lavfi", "-i", "testsrc2=s=640x360:r=25:d=0.4"}, enc...)
	cctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if _, stderr, err := Run(cctx, t.FFmpeg, append(args, out)); err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// pipelineTest is a pipeline's whole job on the sample: decoded, resized and encoded on the chip.
func (t *Tools) pipelineTest(p GPUPath, sample string) []string {
	plan := t.planFor(p.Encoder, Quality{})
	pipe, _ := GPUPipeFor(p.Encoder, "h264", "yuv420p")
	args := append(slices.Clone(plan.InputArgs), pipe.Input...)
	args = append(args, "-i", sample)
	vf := "scale=320:-2,format=" + plan.PixFmt
	if pipe.OnChip() {
		scale, _ := pipe.Scale(240, "nv12")
		vf = scale + ",setsar=1"
	}
	args = append(args, "-vf", vf, "-c:v", plan.Name)
	args = append(args, plan.Args...)
	return append(args, "-f", "null", "-")
}

// testPath runs one test, given the options after the global ones, with the encoders' patience
// for a busy machine (see testEncoder).
func (t *Tools) testPath(ctx context.Context, log *slog.Logger, st *GPUPath, job []string) {
	args := append([]string{"-hide_banner", "-loglevel", "verbose", "-nostdin"}, job...)
	var (
		err    error
		stderr *logging.Tail
	)
	for _, limit := range []time.Duration{20 * time.Second, time.Minute, 3 * time.Minute} {
		stderr = &logging.Tail{}
		cctx, cancel := context.WithTimeout(ctx, limit)
		cmd := Command(cctx, t.FFmpeg, args...)
		cmd.Stderr = stderr
		err = cmd.Run()
		timedOut := cctx.Err() == context.DeadlineExceeded && ctx.Err() == nil
		cancel()
		if !timedOut {
			break
		}
		err = fmt.Errorf("the test took longer than %s: the machine may be too busy", limit)
	}
	said := stderr.String()
	if m := vulkanSelected.FindStringSubmatch(said); m != nil {
		st.Device = strings.TrimSpace(m[1])
		if strings.EqualFold(m[2], "cpu") || strings.Contains(strings.ToLower(m[1]), "llvmpipe") {
			st.Tested, st.Reason = true, i18n.F("The only Vulkan device here is a software one ({device}), which would do the work on the processor.", "device", st.Device)
			log.Info("graphics chip path can't be used", "path", st.ID, "reason", st.Reason)
			return
		}
	}
	st.Tested, st.Working = true, err == nil
	if err == nil {
		log.Info("graphics chip path works", "path", st.ID, "device", st.Device)
		return
	}
	msg := lastLines(said, 300)
	st.Reason, st.Error = "It failed its test on this machine.", msg
	if strings.HasPrefix(err.Error(), "the test took longer") {
		st.Reason = "The test took too long: the machine may have been too busy. It's tested again when the server restarts."
	}
	log.Info("graphics chip path can't be used", "path", st.ID, "reason", st.Reason, "err", msg)
}

// lastLines is the end of FFmpeg's output that says what went wrong: its error lines, or failing
// that its last line, at most max bytes.
func lastLines(said string, max int) string {
	var bad []string
	for _, l := range strings.Split(said, "\n") {
		if lo := strings.ToLower(l); strings.Contains(lo, "error") || strings.Contains(lo, "failed") || strings.Contains(lo, "cannot") {
			bad = append(bad, strings.TrimSpace(l))
		}
	}
	out := strings.Join(bad, " / ")
	if out == "" {
		out = strings.TrimSpace(said)
		if i := strings.LastIndex(out, "\n"); i >= 0 {
			out = out[i+1:]
		}
	}
	if len(out) > max {
		out = out[len(out)-max:]
	}
	return out
}
