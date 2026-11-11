// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package playback

import (
	"reflect"
	"strings"
	"testing"
)

func TestChooseStreamMode(t *testing.T) {
	for _, c := range []struct {
		server, device string
		away           bool
		mode, from     string
	}{
		// The default: nothing changes for anyone.
		{"", "", false, StreamStandard, StreamFromDefault},
		{"", "", true, StreamStandard, StreamFromDefault},
		{StreamStandard, "", true, StreamStandard, StreamFromDefault},
		// The server's choice is for viewers away from home only.
		{StreamRobust, "", true, StreamRobust, StreamFromServer},
		{StreamRobust, "", false, StreamStandard, StreamFromDefault},
		// A device's own choice wins, wherever it is.
		{StreamStandard, StreamRobust, false, StreamRobust, StreamFromDevice},
		{StreamRobust, StreamStandard, true, StreamStandard, StreamFromDevice},
		// Anything else a device sends is no choice.
		{StreamRobust, "fast", true, StreamRobust, StreamFromServer},
	} {
		mode, from := ChooseStreamMode(c.server, c.device, c.away)
		if mode != c.mode || from != c.from {
			t.Errorf("ChooseStreamMode(%q, %q, %v) = %s, %s; want %s, %s", c.server, c.device, c.away, mode, from, c.mode, c.from)
		}
	}
}

func TestRobustLadder(t *testing.T) {
	for _, c := range []struct {
		name                         string
		src, maxH, capKbps, measured int
		want                         []Rung
		start                        int
	}{
		{"1080p film, fast connection: starts at the top", 1080, 0, 0, 20000,
			[]Rung{{1080, 5000}, {720, 2800}, {480, 1400}, {360, 800}}, 0},
		{"1080p film, 3 Mb/s: 480p, one rung above to climb to", 1080, 0, 0, 3000,
			[]Rung{{720, 2800}, {480, 1400}, {360, 800}, {240, 400}}, 1},
		{"not measured: starts in the middle", 1080, 0, 0, 0,
			[]Rung{{720, 2800}, {480, 1400}, {360, 800}, {240, 400}}, 1},
		{"a very slow connection starts at the bottom", 1080, 0, 0, 300,
			[]Rung{{360, 800}, {240, 400}}, 1},
		{"never taller than the picture", 480, 0, 0, 20000,
			[]Rung{{480, 1400}, {360, 800}, {240, 400}}, 0},
		{"the server's height limit", 2160, 720, 0, 20000,
			[]Rung{{720, 2800}, {480, 1400}, {360, 800}, {240, 400}}, 0},
		{"the limit for viewers away from home", 1080, 0, 2000, 20000,
			[]Rung{{480, 1400}, {360, 800}, {240, 400}}, 0},
		{"a tiny picture still gets one rung, at its own height", 144, 0, 0, 20000,
			[]Rung{{144, 400}}, 0},
		{"a limit below every rung", 1080, 0, 250, 20000,
			[]Rung{{240, 250}}, 0},
	} {
		got := RobustLadder(c.src, c.maxH, c.capKbps, c.measured)
		if !reflect.DeepEqual(got.Rungs, c.want) || got.Start != c.start {
			t.Errorf("%s: got %v start %d, want %v start %d", c.name, got.Rungs, got.Start, c.want, c.start)
		}
		if len(got.Rungs) > maxRungs {
			t.Errorf("%s: %d rungs, more than %d", c.name, len(got.Rungs), maxRungs)
		}
	}
}

func TestFitsRobust(t *testing.T) {
	if !FitsRobust(2000, 5000) {
		t.Error("a 2 Mb/s file on a 5 Mb/s connection should be sent as it is")
	}
	if FitsRobust(3000, 5000) {
		t.Error("a 3 Mb/s file on a 5 Mb/s connection leaves too little room")
	}
	if FitsRobust(1000, 0) || FitsRobust(0, 5000) {
		t.Error("without a measurement, or a known bitrate, a file is converted")
	}
}

// Robust converts a file that would fit everything else, and says why.
func TestDecideRobust(t *testing.T) {
	web, _ := Find("web")
	src := Source{Container: "mp4", HasVideo: true, VideoCodec: "h264", Height: 1080, BitDepth: 8, BitrateKbps: 8000, Audio: []Audio{{Codec: "aac", Channels: 2}}}
	q := Request{SubtitleIndex: -1, AllowDirect: true}
	if d := Decide(web, src, q); d.Mode != Direct {
		t.Fatalf("standard: %s, want direct", d.Mode)
	}
	q.Robust = true
	d := Decide(web, src, q)
	if d.Mode != Transcode || !strings.Contains(strings.Join(d.Reasons, ","), ReasonRobust) {
		t.Fatalf("robust: %s %v, want a transcode for %s", d.Mode, d.Reasons, ReasonRobust)
	}
	if ReasonText(ReasonRobust) == ReasonRobust {
		t.Error("the robust reason has no words")
	}
}

func TestHints(t *testing.T) {
	std, rob := Hints(StreamStandard), Hints(StreamRobust)
	if rob.BufferSec <= std.BufferSec || rob.Retries <= std.Retries || !rob.ResumeOnDrop || !rob.StepDownOnStall {
		t.Errorf("robust hints should ask for more than standard: %+v vs %+v", rob, std)
	}
	if std.ResumeOnDrop || std.StepDownOnStall {
		t.Errorf("standard hints should leave the player as it was: %+v", std)
	}
}

func TestExplainStreaming(t *testing.T) {
	if s := ExplainStreaming(StreamStandard, StreamFromDefault, nil, 0); s != "" {
		t.Errorf("standard needs no words, got %q", s)
	}
	s := ExplainStreaming(StreamRobust, StreamFromDevice, []Rung{{480, 1400}, {720, 2800}, {360, 800}, {240, 400}}, 3000)
	for _, want := range []string{"this device", "3 Mb/s", "480p", "1.4 Mb/s", "240p"} {
		if !strings.Contains(s, want) {
			t.Errorf("%q should mention %q", s, want)
		}
	}
	if s := ExplainStreaming(StreamRobust, StreamFromServer, nil, 9000); !strings.Contains(s, "as it is") {
		t.Errorf("a file sent as it is should say so: %q", s)
	}
}
