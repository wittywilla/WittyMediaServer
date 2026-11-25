// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package hls

import (
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// A stream converted on the processor says why not on the graphics chip, in words an owner can act on.
func TestWhyNotTheGraphicsChip(t *testing.T) {
	m := testManager(t, 4)
	m.tools = &ffmpeg.Tools{Encoders: map[string]bool{"libx264": true, "h264_qsv": true, "h264_vaapi": true}}
	qsv := ffmpeg.HWEncoder{Name: "h264_qsv", Label: "Intel Quick Sync"}
	vaapi := ffmpeg.HWEncoder{Name: "h264_vaapi", Label: "VAAPI"}
	defer ffmpeg.PretendTested(
		ffmpeg.HWStatus{HWEncoder: qsv, Compiled: true, Reason: "No graphics device was passed to the container"},
		ffmpeg.HWStatus{HWEncoder: vaapi, Compiled: true, Reason: "isn't in group 105, which owns /dev/dri/renderD128"},
	)()
	sp := Spec{Duration: 60, SegmentSec: 4, Quality: ffmpeg.Quality{HW: []string{"h264_qsv", "h264_vaapi"}}}
	why := m.whyProcessor(sp, m.chooseHW(sp.Quality, "", levelGPU, hwSourceOf(sp)))
	if !strings.Contains(why, "Intel Quick Sync: No graphics device") || !strings.Contains(why, "group 105") {
		t.Fatalf("each encoder's reason: %q", why)
	}
	if why := m.whyProcessor(Spec{}, m.chooseHW(ffmpeg.Quality{}, "", levelGPU, hwSource{})); !strings.Contains(why, "Settings") {
		t.Fatalf("none turned on: %q", why)
	}
	if why := m.whyProcessor(Spec{CopyVideo: true}, hwChoice{}); why != "" {
		t.Fatalf("a copy converts nothing: %q", why)
	}
	if why := m.whyProcessor(sp, hwChoice{plan: ffmpeg.Plan{Name: "libx264"}, level: levelSoftware}); !strings.Contains(why, "failed this stream earlier") {
		t.Fatalf("stepped down: %q", why)
	}
	defer ffmpeg.PretendWorking("h264_qsv")()
	if why := m.whyProcessor(sp, m.chooseHW(sp.Quality, "", levelGPU, hwSourceOf(sp))); why != "" {
		t.Fatalf("Quick Sync works: %q", why)
	}
}
