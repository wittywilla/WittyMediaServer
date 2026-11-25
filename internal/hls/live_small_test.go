// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package hls

import (
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// A small machine converts a live picture with the fastest preset, at most 1080 lines, in 2-second
// segments, so a tune-in isn't a picture standing still while the processor can't keep up. A
// larger machine is as before.
func TestLiveOnASmallMachine(t *testing.T) {
	file := &LiveInput{InputArgs: ffmpeg.FileInput("/m/hdr.mkv"), Seek: 300, Duration: 60, AudioIndex: 0, Height: 2160, VideoCodec: "hevc", PixFmt: "yuv420p10le"}

	m := liveTestManager(t)
	m.smallMachine = func() bool { return true }
	s := m.newLive(LiveSpec{ID: "small"})
	if s.Spec.SegmentSec != smallLiveSegmentSec {
		t.Errorf("segments of %v s on a small machine, want %d", s.Spec.SegmentSec, smallLiveSegmentSec)
	}
	args := strings.Join(s.liveArgs(file, 1, true), " ")
	for _, want := range []string{"-preset " + smallLivePreset, "scale=-2:'min(ih,1080)'", "-hls_time 2.000"} {
		if !strings.Contains(args, want) {
			t.Errorf("a small machine's live conversion lacks %q:\n%s", want, args)
		}
	}
	// A preset asked for (the owner's low power setting) is kept.
	asked := strings.Join(m.newLive(LiveSpec{ID: "asked", Preset: "superfast"}).liveArgs(file, 1, true), " ")
	if !strings.Contains(asked, "-preset superfast") {
		t.Errorf("the preset asked for is kept:\n%s", asked)
	}
	// A copy isn't converted, so nothing about it changes.
	copied := strings.Join(s.liveArgs(&LiveInput{InputArgs: ffmpeg.FileInput("/m/a.mkv"), AudioIndex: 0, Height: 2160, CopyVideo: true}, 2, false), " ")
	if strings.Contains(copied, "min(ih,1080)") || strings.Contains(copied, "-preset") {
		t.Errorf("a copy is left as it is:\n%s", copied)
	}

	big := liveTestManager(t)
	big.smallMachine = func() bool { return false }
	b := big.newLive(LiveSpec{ID: "big"})
	if b.Spec.SegmentSec != 4 {
		t.Errorf("segments of %v s on a larger machine, want 4", b.Spec.SegmentSec)
	}
	bargs := strings.Join(b.liveArgs(file, 1, true), " ")
	if strings.Contains(bargs, "ultrafast") || strings.Contains(bargs, "min(ih,1080)") {
		t.Errorf("a larger machine is as before:\n%s", bargs)
	}
}
