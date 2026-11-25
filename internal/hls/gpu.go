// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package hls

import (
	"slices"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// How much of the graphics chip a stream uses. A job that fails takes its stream a step down,
// for its later jobs, and the viewer's request is answered by the next job without an error:
//
//	levelGPU        decoded, resized and HDR-mapped on the chip where it can (ffmpeg.GPUPipe,
//	                ffmpeg.GPUToneMap), encoded by the hardware encoder
//	levelCPUDecode  decoded and filtered by the processor, encoded by the hardware encoder
//	levelSoftware   all on the processor
const (
	levelGPU int32 = iota
	levelCPUDecode
	levelSoftware
)

// hwCaps are the most jobs each hardware encoder runs at once. NVIDIA's ordinary cards open a
// limited number of encoding sessions (eight with current drivers, fewer with older ones); a job
// past that would fail to start, so it's given the next encoder, or the processor, instead.
// Older drivers that allow fewer fail the extra job, which then steps down like any failure.
var hwCaps = map[string]int{"h264_nvenc": 8}

// hwChoice is what one job uses of the chip.
type hwChoice struct {
	plan    ffmpeg.Plan
	pipe    ffmpeg.GPUPipe
	onGPU   bool   // pipe is used
	placebo bool   // HDR mapped on the chip
	level   int32  // the stream's level when it was chosen
	key     string // the kind of job, for gpuFailed, when onGPU or placebo
}

// hwSource is what a choice needs to know about the picture.
type hwSource struct {
	codec, pixFmt string
	videoFilter   string // the processor's filters before resizing (a deinterlacer)
	toneMap       string // HDR range to map, "" = none
	subs          bool   // subtitles are drawn in
	// cpuOnly: something only the processor does comes before resizing (cut stretches, a held
	// last frame, the owner's own encoder options, a copied picture).
	cpuOnly bool
}

// chooseHW picks the encoder and what the chip does for one job at this level.
func (m *Manager) chooseHW(q ffmpeg.Quality, preset string, level int32, src hwSource) hwChoice {
	if level >= levelSoftware {
		q.HW = nil
	}
	plan := m.tools.Plan(q)
	// An encoder at its limit of jobs gives way to the next one the owner allows.
	for hardware(plan.Name) && m.atCap(plan.Name) && slices.Contains(q.HW, plan.Name) {
		q.HW = slices.DeleteFunc(slices.Clone(q.HW), func(n string) bool { return n == plan.Name })
		plan = m.tools.Plan(q)
	}
	if preset != "" && plan.Name == "libx264" && len(q.VideoArgs) == 0 {
		q.Preset = preset
		plan = m.tools.Plan(q)
	}
	c := hwChoice{plan: plan, level: level}
	if level != levelGPU || src.cpuOnly || src.codec == "" && src.toneMap == "" {
		return c
	}
	c.key = strings.Join([]string{plan.Name, src.codec, src.pixFmt, src.toneMap, boolWord(src.subs)}, "|")
	m.hwMu.Lock()
	failed := m.gpuFailed[c.key]
	m.hwMu.Unlock()
	if failed {
		c.key = ""
		return c
	}
	if src.codec != "" {
		if pipe, ok := ffmpeg.GPUPipeline(plan.Name, src.codec, src.pixFmt); ok && (src.videoFilter == "" || !pipe.OnChip() || pipe.Deinterlace(src.videoFilter) != "") {
			c.pipe, c.onGPU = pipe, true
		}
	}
	// libplacebo needs the Vulkan device as the filters' device, which VA-API's upload needs too.
	if src.toneMap != "" && plan.Name != "h264_vaapi" {
		_, c.placebo = ffmpeg.GPUToneMap(src.toneMap, 0)
	}
	if !c.onGPU && !c.placebo {
		c.key = ""
	}
	return c
}

func boolWord(b bool) string {
	if b {
		return "y"
	}
	return "n"
}

// hardware: an encoder on the graphics chip, rather than the processor.
func hardware(encoder string) bool {
	return slices.ContainsFunc(ffmpeg.HWEncoders, func(e ffmpeg.HWEncoder) bool { return e.Name == encoder })
}

// atCap says an encoder runs as many jobs as it can.
func (m *Manager) atCap(encoder string) bool {
	limit, ok := hwCaps[encoder]
	if !ok {
		return false
	}
	m.hwMu.Lock()
	defer m.hwMu.Unlock()
	return m.hwRunning[encoder] >= limit
}

// hwStarted and hwEnded count an encoder's running jobs.
func (m *Manager) hwStarted(encoder string) {
	if !hardware(encoder) {
		return
	}
	m.hwMu.Lock()
	if m.hwRunning == nil {
		m.hwRunning = map[string]int{}
	}
	m.hwRunning[encoder]++
	m.hwMu.Unlock()
}

func (m *Manager) hwEnded(encoder string) {
	if !hardware(encoder) {
		return
	}
	m.hwMu.Lock()
	m.hwRunning[encoder]--
	m.hwMu.Unlock()
}

// stepDown is what a failed job using c says about its stream's next jobs: the level they
// should use (the same when nothing of the chip's was used), and whether this kind of job
// shouldn't be tried on the chip again by any stream (early: it failed before it made anything,
// and not for want of room on the chip).
func stepDown(c hwChoice, early bool, said string) (next int32, forget bool) {
	switch {
	case c.key != "":
		return max(c.level, levelCPUDecode), early && !outOfRoom(said)
	case hardware(c.plan.Name):
		return levelSoftware, false
	}
	return c.level, false
}

// outOfRoom: FFmpeg failed for want of the chip's memory or encoding sessions, which says
// nothing about whether the chip can do the job when it has room.
func outOfRoom(said string) bool {
	said = strings.ToLower(said)
	for _, w := range []string{"openencodesessionex", "out of memory", "cuda_error_out_of_memory", "cannot allocate memory", "incompatible client key", "no free encoding sessions", "resource temporarily unavailable"} {
		if strings.Contains(said, w) {
			return true
		}
	}
	return false
}

// whyProcessor says why a stream's job converts on the processor rather than the graphics chip, in
// words for the log: "" when the chip encodes, or nothing is converted. An owner whose NAS has an
// Intel or AMD chip reads this to find what is missing (the device, the render group, a driver, the
// setting) without having to know where the encoders are tested.
func (m *Manager) whyProcessor(sp Spec, hc hwChoice) string {
	switch {
	case sp.CopyVideo || sp.NoVideo || hardware(hc.plan.Name):
		return ""
	case hc.level >= levelSoftware:
		return "the graphics chip failed this stream earlier, so it goes on with the processor"
	case len(sp.Quality.VideoArgs) > 0:
		return "the expert encoder options in Settings are the processor's"
	case len(sp.Quality.HW) == 0:
		return "no hardware encoder is turned on in Settings → Live transcoding"
	}
	list, done := m.tools.HardwareStatus()
	if !done {
		return "the hardware encoders are still being tested (a minute or so after the server starts)"
	}
	var why []string
	for _, st := range list {
		if !slices.Contains(sp.Quality.HW, st.Name) {
			continue
		}
		switch {
		case !st.Compiled:
			why = append(why, st.Label+": not in this FFmpeg")
		case st.Working && m.atCap(st.Name):
			why = append(why, st.Label+": running as many streams as it can")
		case !st.Working && st.Reason != "":
			why = append(why, st.Label+": "+st.Reason)
		case !st.Working:
			why = append(why, st.Label+": failed its test")
		}
	}
	if len(why) == 0 {
		return "no hardware encoder works here"
	}
	return strings.Join(why, "; ")
}
