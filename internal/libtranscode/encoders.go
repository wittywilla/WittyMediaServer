// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package libtranscode

import (
	"slices"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// Choosing an encoder for a file kept on disk. Live playback has its own choices
// (internal/ffmpeg): it wants speed now, and this wants small files that look right.

type encoderChoice struct {
	name      string
	codec     string
	args      []string
	filters   []string
	inputArgs []string
}

// softwareFor names the software encoder of a format, in order of preference.
var softwareFor = map[string][]string{
	"h264": {"libx264"},
	"hevc": {"libx265"},
	"av1":  {"libsvtav1"},
}

// hardwareFamilies are hardware encoders by family: h264_nvenc belongs with hevc_nvenc.
func hardwareName(codec, family string) string { return codec + "_" + family }

// HardwareFamily is the part of a hardware encoder's name that says whose it is.
func HardwareFamily(name string) string {
	_, family, _ := strings.Cut(name, "_")
	return family
}

// chooseEncoder picks what to use. Hardware goes first only for an ordinary 8-bit picture:
// what hardware encoders do with HDR's information and with 10 bits differs too much from
// one driver to another to trust with a file that replaces the original.
func chooseEncoder(target string, depth int, hdr bool, p Policy, env Env, v VideoStream) (encoderChoice, string) {
	note := ""
	codec := target
	if !hasEncoderFor(codec, env, depth, hdr) && codec == "av1" {
		codec = "hevc"
		note = "This server's FFmpeg can't encode AV1, so H.265 is used instead."
	}
	if !hasEncoderFor(codec, env, depth, hdr) && codec == "hevc" && !hdr {
		codec = "h264"
		note = "This server's FFmpeg can't encode H.265, so H.264 at a stronger setting is used instead. It saves less."
	}
	q := p.Video.Quality
	if codec == "h264" && target != "h264" {
		q += 3
	}
	if depth == 8 && !hdr {
		for _, family := range hardwareOrder(env.Hardware) {
			name := hardwareName(codec, family)
			if env.Encoders[name] && slices.Contains(env.Hardware, name) {
				c := hardwareEncoder(name, codec, family, q, p.Video.Speed, outputHeight(v.Height, p.Video.MaxHeight))
				if env.Threads > 0 {
					c.args = append(c.args, "-threads", strconv.Itoa(env.Threads))
				}
				return c, note
			}
		}
	}
	for _, name := range softwareFor[codec] {
		if env.Encoders[name] {
			return softwareEncoder(name, codec, q, depth, hdr, p.Video.Speed, v, env.Threads), note
		}
	}
	return encoderChoice{}, note
}

func hasEncoderFor(codec string, env Env, depth int, hdr bool) bool {
	for _, name := range softwareFor[codec] {
		if env.Encoders[name] {
			return true
		}
	}
	if depth == 8 && !hdr {
		for _, h := range env.Hardware {
			if strings.HasPrefix(h, codec+"_") && env.Encoders[h] {
				return true
			}
		}
	}
	return false
}

// hardwareOrder is each family once, in the order of the owner's list.
func hardwareOrder(list []string) []string {
	var out []string
	for _, n := range list {
		if f := HardwareFamily(n); f != "" && !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}

func softwareEncoder(name, codec string, q, depth int, hdr bool, speed string, v VideoStream, threads int) encoderChoice {
	c := encoderChoice{name: name, codec: codec}
	pix := "yuv420p"
	if depth > 8 {
		pix = "yuv420p10le"
	}
	switch name {
	case "libx264":
		preset := map[string]string{"faster": "veryfast", "balanced": "fast", "slower": "slow"}[speed]
		c.args = []string{"-preset", orDefault(preset, "fast"), "-crf", strconv.Itoa(clampInt(q, 0, 51)), "-pix_fmt", pix}
		if depth == 8 {
			c.args = append(c.args, "-profile:v", "high")
		}
		if threads > 0 {
			c.args = append(c.args, "-threads", strconv.Itoa(threads))
		}
	case "libx265":
		preset := map[string]string{"faster": "superfast", "balanced": "faster", "slower": "medium"}[speed]
		// The same picture needs a higher number on this scale.
		c.args = []string{"-preset", orDefault(preset, "faster"), "-crf", strconv.Itoa(clampInt(q+4, 0, 51)), "-pix_fmt", pix}
		params := []string{"log-level=error"}
		if threads > 0 {
			params = append(params, "pools="+strconv.Itoa(threads), "frame-threads="+strconv.Itoa(clampInt(threads/2, 1, 4)))
		}
		if hdr {
			params = append(params, x265HDR(v)...)
		}
		c.args = append(c.args, "-x265-params", strings.Join(params, ":"))
	case "libsvtav1":
		preset := map[string]string{"faster": "10", "balanced": "8", "slower": "6"}[speed]
		c.args = []string{"-preset", orDefault(preset, "8"), "-crf", strconv.Itoa(clampInt(q+8, 0, 63)), "-pix_fmt", pix}
		if threads > 0 {
			c.args = append(c.args, "-svtav1-params", "lp="+strconv.Itoa(threads))
		}
	}
	return c
}

// x265HDR writes the HDR description into the stream itself, which is where players read
// it. The numbers come from the file, read as numbers, so nothing from the file reaches
// the command line as text.
func x265HDR(v VideoStream) []string {
	trc := "smpte2084"
	if v.Range == "hlg" {
		trc = "arib-std-b67"
	}
	out := []string{"colorprim=bt2020", "transfer=" + trc, "colormatrix=bt2020nc", "repeat-headers=1"}
	if v.Range != "hlg" {
		out = append(out, "hdr10=1", "hdr10-opt=1")
		if v.Mastering != "" {
			out = append(out, "master-display="+v.Mastering)
		}
		if v.ContentLight != "" {
			out = append(out, "max-cll="+v.ContentLight)
		}
	}
	return out
}

// hardwareEncoder: height is how high the picture comes out, 0 when that isn't known.
func hardwareEncoder(name, codec, family string, q int, speed string, height int) encoderChoice {
	c := encoderChoice{name: name, codec: codec}
	switch family {
	case "nvenc":
		preset := map[string]string{"faster": "p3", "balanced": "p5", "slower": "p6"}[speed]
		c.args = []string{"-preset", orDefault(preset, "p5"), "-tune", "hq", "-rc", "vbr", "-cq", strconv.Itoa(q + 2), "-b:v", "0", "-pix_fmt", "yuv420p"}
	case "qsv":
		c.inputArgs = ffmpeg.DeviceArgs("qsv")
		preset := map[string]string{"faster": "faster", "balanced": "medium", "slower": "slow"}[speed]
		c.args = []string{"-preset", orDefault(preset, "medium"), "-global_quality", strconv.Itoa(q + 2), "-pix_fmt", "nv12"}
	case "videotoolbox":
		if !ffmpeg.VideoToolboxQuality {
			c.args = videoToolboxRate(codec, height)
			break
		}
		c.args = []string{"-q:v", strconv.Itoa(clampInt(102-2*q, 1, 100)), "-pix_fmt", "yuv420p"}
	case "amf":
		quality := map[string]string{"faster": "speed", "balanced": "balanced", "slower": "quality"}[speed]
		c.args = []string{"-quality", orDefault(quality, "balanced"), "-rc", "cqp", "-qp_i", strconv.Itoa(q + 1), "-qp_p", strconv.Itoa(q + 3), "-pix_fmt", "yuv420p"}
	case "vaapi":
		c.inputArgs = ffmpeg.DeviceArgs("vaapi")
		c.filters = []string{"format=nv12", "hwupload"}
		c.args = []string{"-qp", strconv.Itoa(q + 2)}
	}
	return c
}

// videoToolboxRate is what VideoToolbox gets where it takes no quality setting (an Intel
// Mac: see ffmpeg.VideoToolboxQuality): a bitrate for the picture's size, about what the
// quality setting gives a film at 1080p, 6 Mb/s for H.264 and 4 for H.265.
func videoToolboxRate(codec string, height int) []string {
	if height <= 0 {
		height = 1080
	}
	kbps := 6000
	if codec != "h264" {
		kbps = 4000
	}
	kbps = max(kbps*height*height/(1080*1080), 500)
	return []string{"-b:v", strconv.Itoa(kbps) + "k", "-pix_fmt", "yuv420p"}
}

// outputHeight is how high the converted picture comes out.
func outputHeight(height, maxHeight int) int {
	if maxHeight > 0 && height > maxHeight {
		return maxHeight
	}
	return height
}

func clampInt(v, lo, hi int) int {
	return max(lo, min(hi, v))
}
