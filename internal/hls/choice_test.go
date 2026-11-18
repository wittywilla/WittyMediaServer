// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package hls

import (
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/playback"
)

// A viewer's choice in the quality menu reaches FFmpeg: the picture is brought down to the
// size and the encoder held to the rate (ffmpeg's own tests cover how each hardware encoder
// is held to it).
func TestChoiceReachesFFmpeg(t *testing.T) {
	line := func(choice string, height int) string {
		tools := &ffmpeg.Tools{Encoders: map[string]bool{"libx264": true}}
		base := ffmpeg.Quality{Preset: "veryfast", CRF: 21, AudioRate: "192k"}
		c := playback.ParseChoice(choice)
		sp := Spec{InputArgs: []string{"-i", "in.mkv"}, Duration: 60, SegmentSec: 4, AudioIndex: 0, AudioChannels: 2, SubtitleIndex: -1,
			Quality: c.Quality(base), MaxHeight: height}
		s := &Session{Spec: sp, Dir: t.TempDir(), tools: tools, m: &Manager{tools: tools}}
		args, _, _, _ := s.commandLine(0)
		return strings.Join(args, " ")
	}
	got := line("h720-b4000", 720)
	for _, want := range []string{"scale=-2:'min(ih,720)'", "-c:v libx264", "-crf 21", "-maxrate 3808k -bufsize 7616k", "-b:a 192k"} {
		if !strings.Contains(got, want) {
			t.Errorf("720p at 4 Mbps with libx264: no %q in\n%s", want, got)
		}
	}
	got = line("h360-b800", 360)
	for _, want := range []string{"scale=-2:'min(ih,360)'", "-maxrate 672k", "-b:a 128k"} {
		if !strings.Contains(got, want) {
			t.Errorf("360p at 800 kbps: no %q in\n%s", want, got)
		}
	}
	// Original: nothing is capped or scaled.
	got = line("original", 0)
	if strings.Contains(got, "-maxrate") || strings.Contains(got, "min(ih,") {
		t.Errorf("Original caps nothing:\n%s", got)
	}
}
