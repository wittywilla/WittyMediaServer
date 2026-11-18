// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package hls

import "sync/atomic"

// Work says what is doing a stream's conversion, for the player's stream information: the
// encoder, and which steps run on the graphics chip. A stream steps down to the processor when
// the chip fails it (gpu.go), so this is what its latest job uses, not what was hoped for.
type Work struct {
	// Encoder is FFmpeg's video encoder ("h264_nvenc", "libx264"), or "copy" when the picture
	// isn't converted.
	Encoder string `json:"encoder"`
	// GPUEncode: a hardware encoder. GPUDecode: the chip also decodes and resizes the picture
	// (the GPU pipeline). GPUToneMap: HDR is mapped on the chip.
	GPUEncode  bool `json:"gpuEncode"`
	GPUDecode  bool `json:"gpuDecode"`
	GPUToneMap bool `json:"gpuToneMap"`
	// Running: a job is converting now. Otherwise this is what the next one would use.
	Running bool `json:"running"`
}

func (c hwChoice) work(copy bool) Work {
	if copy {
		return Work{Encoder: "copy"}
	}
	return Work{Encoder: c.plan.Name, GPUEncode: hardware(c.plan.Name), GPUDecode: c.onGPU && c.pipe.OnChip(), GPUToneMap: c.placebo}
}

// hwSourceOf is what the choice needs to know about a spec's picture, as its jobs ask.
func hwSourceOf(sp Spec) hwSource {
	return hwSource{codec: sp.VideoCodec, pixFmt: sp.PixFmt, videoFilter: sp.VideoFilter, toneMap: sp.ToneMap, subs: sp.SubtitleIndex >= 0,
		cpuOnly: sp.CopyVideo || sp.NoVideo || sp.HoldLastFrame || len(sp.Holes) > 0 || len(sp.Quality.VideoArgs) > 0}
}

// PlanWork is what a stream made to this spec would use, before any job runs.
func (m *Manager) PlanWork(sp Spec) Work {
	if s := m.Get(sp.ID); s != nil {
		return s.Work()
	}
	return m.chooseHW(sp.Quality, sp.Preset, levelGPU, hwSourceOf(sp)).work(sp.CopyVideo || sp.NoVideo)
}

// Work is what the stream's latest job uses, or what its next one would.
func (s *Session) Work() Work {
	s.mu.Lock()
	j := s.job
	s.mu.Unlock()
	if j != nil {
		w := j.hw.work(s.Spec.CopyVideo || s.Spec.NoVideo)
		w.Running = !j.finished()
		return w
	}
	return s.m.chooseHW(s.Spec.Quality, s.Spec.Preset, s.fallback.Load(), hwSourceOf(s.Spec)).work(s.Spec.CopyVideo || s.Spec.NoVideo)
}

// liveWork keeps a live stream's latest run's Work.
type liveWork struct{ v atomic.Pointer[Work] }

func (l *liveWork) set(w Work) { l.v.Store(&w) }

// Work is what the live stream's latest run uses; Encoder is "" before the first.
func (s *LiveSession) Work() Work {
	if w := s.work.v.Load(); w != nil {
		return *w
	}
	return Work{}
}
