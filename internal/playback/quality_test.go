// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package playback

import (
	"slices"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

func TestParseChoice(t *testing.T) {
	for in, want := range map[string]string{
		"":                         "",
		"auto":                     "",
		"original":                 "original",
		"h720-b4000":               "h720-b4000",
		"b4000,h720":               "h720-b4000", // the same choice is named the same way
		"h720;b4000;vh264;a128;c2": "h720-b4000-vh264-a128-c2",
		"h99999-b5":                "",     // out of range: dropped, never an error
		"h720-vav1-a100-c3":        "h720", // AV1 isn't a codec conversions make; 100k isn't offered; 3 channels isn't a layout
		"x720-h-b":                 "",
	} {
		if got := ParseChoice(in).String(); got != want {
			t.Errorf("ParseChoice(%q) = %q, want %q", in, got, want)
		}
	}
	if ParseChoice("auto").Set || !ParseChoice("original").Set {
		t.Error("Auto is no choice; Original is one")
	}
}

// Only sizes at or below the picture's own are offered, a wide film counting by its width.
func TestPresetsFor(t *testing.T) {
	names := func(w, h int) []string {
		var out []string
		for _, p := range PresetsFor(w, h) {
			out = append(out, p.Name)
		}
		return out
	}
	for _, c := range []struct {
		w, h int
		want []string
	}{
		{3840, 2160, []string{"4K", "1080p", "720p", "480p", "360p"}},
		{1920, 1080, []string{"1080p", "720p", "480p", "360p"}},
		{1920, 800, []string{"1080p", "720p", "480p", "360p"}}, // 2.40:1
		{1920, 1072, []string{"1080p", "720p", "480p", "360p"}},
		{720, 576, []string{"480p", "360p"}},                    // PAL DVD
		{1080, 1920, []string{"1080p", "720p", "480p", "360p"}}, // an upright phone video
		{0, 0, []string{"4K", "1080p", "720p", "480p", "360p"}}, // not known: everything
	} {
		if got := names(c.w, c.h); !slices.Equal(got, c.want) {
			t.Errorf("%dx%d: %v, want %v", c.w, c.h, got, c.want)
		}
	}
}

// A size is a 16:9 box: a wide film comes out as wide as any picture of that size.
func TestFitHeight(t *testing.T) {
	for _, c := range []struct{ max, w, h, want int }{
		{720, 1920, 1080, 720},
		{720, 1920, 800, 532}, // 1280x532, as wide as any 720p picture
		{720, 720, 576, 720},  // narrower than 16:9: by height
		{720, 1080, 1920, 720},
		{0, 1920, 800, 0},
	} {
		if got := FitHeight(c.max, c.w, c.h); got != c.want {
			t.Errorf("FitHeight(%d, %dx%d) = %d, want %d", c.max, c.w, c.h, got, c.want)
		}
	}
}

// What a viewer asks for never goes over what they are allowed.
func TestWithinLimits(t *testing.T) {
	c := ParseChoice("original").Within(Limits{MaxKbps: 4000}, Limits{MaxHeight: 720})
	if c.MaxHeight != 720 || c.MaxKbps != 4000 {
		t.Errorf("Original away from home with limits: %+v", c)
	}
	c = ParseChoice("h480-b1500").Within(Limits{MaxHeight: 720, MaxKbps: 4000})
	if c.MaxHeight != 480 || c.MaxKbps != 1500 {
		t.Errorf("a lower choice stays: %+v", c)
	}
}

// The rate becomes a hard cap on the picture, with the sound inside it, and the sound rate
// the viewer picked is kept when the picture still gets most of the total.
func TestChoiceQuality(t *testing.T) {
	base := ffmpeg.Quality{Preset: "veryfast", CRF: 21, AudioRate: "192k", HW: []string{"h264_nvenc"}}
	q := ParseChoice("h1080-b8000").Quality(base)
	if q.CapRate != "7808k" || q.AudioRate != "192k" || q.CRF != 21 || len(q.HW) != 1 {
		t.Errorf("8 Mbps: %+v", q)
	}
	q = ParseChoice("h360-b800").Quality(base)
	if q.CapRate != "672k" || q.AudioRate != "128k" {
		t.Errorf("800 kbps: sound is held to 128k, the picture gets the rest: %+v", q)
	}
	q = ParseChoice("b8000-a320").Quality(base)
	if q.AudioRate != "320k" || q.CapRate != "7808k" {
		t.Errorf("a sound rate the viewer chose stays: %+v", q)
	}
	// A lower cap already there (away from home) stays lower.
	away := base.WithCap("2M")
	if q = ParseChoice("b8000").Quality(away); q.CapRate != away.CapRate {
		t.Errorf("the away cap %s was lifted to %s", away.CapRate, q.CapRate)
	}
	if q = ParseChoice("original").Quality(base); q.CapRate != "" {
		t.Errorf("Original caps nothing: %+v", q)
	}
}

// The direct-play decision with a choice: a file within it plays as it is; one over it is
// converted, and says why.
func TestChoiceDecision(t *testing.T) {
	web, _ := Find("web")
	web.VideoCodecs = append(web.VideoCodecs, "hevc")
	src := Source{Container: "mp4", HasVideo: true, VideoCodec: "h264", Height: 1080, BitDepth: 8, Range: "sdr", BitrateKbps: 6000,
		Audio: []Audio{{Codec: "aac", Channels: 2}}}
	decide := func(choice string, s Source) Decision {
		c := ParseChoice(choice)
		return Decide(c.Profile(web), s, c.Request(Request{AllowDirect: true, SubtitleIndex: -1}))
	}
	if d := decide("h1080-b8000", src); d.Mode != Direct {
		t.Errorf("1080p at 6 Mbps is within 1080p and 8 Mbps: %+v", d)
	}
	if d := decide("original", src); d.Mode != Direct {
		t.Errorf("Original: %+v", d)
	}
	d := decide("h720-b4000", src)
	if d.Mode != Transcode || !slices.Contains(d.Reasons, ReasonVideoHeight) || !slices.Contains(d.Reasons, ReasonVideoBitrate) {
		t.Errorf("720p and 4 Mbps from a 1080p 6 Mbps file: %+v", d)
	}
	if d := decide("b4000", src); d.Mode != Transcode || slices.Contains(d.Reasons, ReasonVideoHeight) {
		t.Errorf("only the rate is over: %+v", d)
	}
	hevc := src
	hevc.VideoCodec = "hevc"
	if d := decide("", hevc); d.Mode != Direct {
		t.Errorf("HEVC plays as it is where the device plays it: %+v", d)
	}
	if d := decide("vh264", hevc); d.Mode != Transcode || !slices.Contains(d.Reasons, ReasonVideoCodec) {
		t.Errorf("H.264 preferred: HEVC is converted: %+v", d)
	}
	if d := decide("vh264", src); d.Mode != Direct {
		t.Errorf("H.264 preferred: H.264 plays as it is: %+v", d)
	}
	surround := src
	surround.Audio = []Audio{{Codec: "aac", Channels: 6}}
	if d := decide("c2", surround); d.Mode == Direct || d.AudioChannels != 2 {
		t.Errorf("stereo chosen: 5.1 sound is folded down: %+v", d)
	}
}

// A 1080p film of 6.4 Mbps isn't offered "1080p at 8 Mbps" (#470); smaller sizes are held to its rate.
func TestPresetsForSource(t *testing.T) {
	got := PresetsForSource(1920, 1080, 6400)
	if len(got) == 0 || got[0].Name != "720p" || got[0].Kbps != 4000 {
		t.Fatalf("1080p at 6.4 Mbps: %+v", got)
	}
	got = PresetsForSource(1920, 1080, 3000)
	if got[0].Name != "720p" || got[0].Kbps != 3000 {
		t.Fatalf("1080p at 3 Mbps: 720p should be held to 3000: %+v", got)
	}
	got = PresetsForSource(1920, 1080, 30000)
	if got[0].Name != "1080p" || got[0].Kbps != 8000 {
		t.Fatalf("a 30 Mbps film keeps 1080p at 8 Mbps: %+v", got)
	}
	if n := len(PresetsForSource(0, 0, 0)); n != len(Presets) {
		t.Fatalf("unknown source: %d presets", n)
	}
}
