// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package ffmpeg

import (
	"fmt"
	"slices"
	"strings"
)

// GPUPipe keeps a hardware encoder's picture on the graphics chip from decoding to encoding:
// the chip decodes the file, resizes the picture and hands it to its own encoder, with nothing
// copied through the computer's memory on the way. Decoding on the processor and scaling there
// took five times the processor time for the same film (12 s for each minute of a 1080p film
// brought to 720p for NVENC, 2.4 s this way), and on a machine already busy with other work it
// was the difference between a quarter of real time and seven times it.
type GPUPipe struct {
	// Input goes before the picture's -i: decode on the chip and keep the frames there.
	Input []string
	// family is the chip's filter suffix: cuda, qsv or vaapi; "" decodes only (VideoToolbox),
	// where FFmpeg brings each frame back itself and the rest is as without this.
	family string
	deint  string // the chip's deinterlacer, "" = none that takes the same options
	id     string // its GPUPath
}

// ID is the pipeline's GPUPath ID, "pipeline:cuda" and so on.
func (p GPUPipe) ID() string { return p.id }

// gpuDecodes are the pictures each family's decoder takes, with their pixel formats: what's
// outside these (H.264 at 10 bits, 4:2:2 and 4:4:4 pictures, older codecs) FFmpeg would decode
// on the processor and the chip's filters would refuse.
var gpuDecodes = map[string][]string{
	"cuda":         {"h264", "hevc", "vp9", "av1", "mpeg2video", "vc1"},
	"qsv":          {"h264", "hevc", "vp9", "av1", "mpeg2video"},
	"vaapi":        {"h264", "hevc", "vp9", "av1", "mpeg2video", "vc1"},
	"videotoolbox": {"h264", "hevc", "vp9", "av1", "mpeg2video"},
}

// GPUPipeFor is the pipeline for this encoder and source picture, if there is one, whether or not
// it works here (GPUPipeline also asks that).
func GPUPipeFor(encoder, codec, pixFmt string) (GPUPipe, bool) {
	family := ""
	switch encoder {
	case "h264_nvenc":
		family = "cuda"
	case "h264_qsv":
		family = "qsv"
	case "h264_vaapi":
		family = "vaapi"
	case "h264_videotoolbox":
		family = "videotoolbox"
	default:
		return GPUPipe{}, false
	}
	if !slices.Contains(gpuDecodes[family], codec) {
		return GPUPipe{}, false
	}
	switch pixFmt {
	case "yuv420p", "yuvj420p", "nv12":
	case "yuv420p10le", "p010le":
		if codec == "h264" || codec == "mpeg2video" || codec == "vc1" {
			return GPUPipe{}, false
		}
	default:
		return GPUPipe{}, false
	}
	switch family {
	case "cuda":
		return GPUPipe{id: "pipeline:cuda", family: "cuda", Input: []string{"-hwaccel", "cuda", "-hwaccel_output_format", "cuda"}, deint: "bwdif_cuda"}, true
	case "qsv":
		in := []string{"-hwaccel", "qsv", "-hwaccel_output_format", "qsv"}
		if DeviceArgs("qsv") != nil { // the device named "qs" there, on the Intel chip's node
			in = append(in, "-hwaccel_device", "qs")
		}
		return GPUPipe{id: "pipeline:qsv", family: "qsv", Input: in}, true
	case "vaapi":
		in := []string{"-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi"}
		if dev := planDevice(Plan{InputArgs: DeviceArgs("vaapi")}); dev != "" {
			in = append(in, "-hwaccel_device", dev)
		}
		return GPUPipe{id: "pipeline:vaapi", family: "vaapi", Input: in}, true
	default:
		return GPUPipe{id: "pipeline:videotoolbox", Input: []string{"-hwaccel", "videotoolbox"}}, true
	}
}

// OnChip says the frames stay on the chip, so the filters before the encoder are the chip's.
func (p GPUPipe) OnChip() bool { return p.family != "" }

// Deinterlace is the chip's version of the processor's bwdif filter with these options, "" when
// the chip has none.
func (p GPUPipe) Deinterlace(cpuFilter string) string {
	opts, ok := strings.CutPrefix(cpuFilter, "bwdif=")
	if !ok || p.deint == "" {
		return ""
	}
	return p.deint + "=" + opts
}

// Scale brings the picture to at most maxHeight lines (0 = its own height) on the chip, with
// square pixels (a widescreen DVD's 720 columns become 853), in pixFmt: nv12, or p010le to keep
// ten bits for HDR mapping. download brings the frames into the computer's memory in that
// format, for the filters only the processor has (subtitles drawn in, HDR mapping).
func (p GPUPipe) Scale(maxHeight int, pixFmt string) (scale, download string) {
	h := "trunc(ih/2)*2"
	if maxHeight > 0 {
		h = fmt.Sprintf("trunc(min(ih,%d)/2)*2", maxHeight)
	}
	scale = fmt.Sprintf("scale_%s=w='trunc(iw*sar*oh/ih/2)*2':h='%s':format=%s", p.family, h, pixFmt)
	return scale, "hwdownload,format=" + pixFmt
}

// PretendWorking marks hardware encoders ("h264_nvenc") and GPU paths ("pipeline:cuda") as
// tested and working, for tests in other packages that look at the command lines made for them.
// The returned function puts things back.
func PretendWorking(names ...string) (restore func()) {
	hw.mu.Lock()
	old, oldPaths, oldOff := hw.status, hw.paths, hw.pathsOff
	hw.status, hw.paths, hw.pathsOff = map[string]*HWStatus{}, map[string]*GPUPath{}, nil
	for _, n := range names {
		if strings.Contains(n, ":") {
			hw.paths[n] = &GPUPath{ID: n, Tested: true, Working: true}
			continue
		}
		hw.status[n] = &HWStatus{HWEncoder: HWEncoder{Name: n}, Compiled: true, Tested: true, Working: true}
	}
	hw.mu.Unlock()
	return func() {
		hw.mu.Lock()
		hw.status, hw.paths, hw.pathsOff = old, oldPaths, oldOff
		hw.mu.Unlock()
	}
}
