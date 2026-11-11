// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package playback

import (
	"slices"
	"strings"
	"testing"
)

func h264MP4() Source {
	return Source{Container: "mp4", HasVideo: true, VideoCodec: "h264", VideoTag: "avc1", Height: 1080, BitDepth: 8, Range: "sdr", BitrateKbps: 8000,
		Audio: []Audio{{Codec: "aac", Channels: 2, Language: "eng"}}}
}

func req() Request { return Request{SubtitleIndex: -1, AllowDirect: true} }

func prof(t *testing.T, id string) Profile {
	t.Helper()
	p, ok := Find(id)
	if !ok {
		t.Fatalf("no profile %s", id)
	}
	return p
}

// What a browser that says nothing about itself gets. The rule the whole table is about:
// the simplest way that plays. Direct if it can, then rewrapped, then with only the
// sound converted, and only then with the picture converted.
func TestWebProfile(t *testing.T) {
	web := prof(t, "web")
	cases := []struct {
		name      string
		src       func(*Source)
		q         func(*Request)
		want      string
		copyAudio bool
	}{
		{"h264 mp4 aac", nil, nil, Direct, false},
		{"vp9 webm opus", func(s *Source) {
			s.Container, s.VideoCodec = "webm", "vp9"
			s.Audio[0].Codec = "opus"
		}, nil, Direct, false},
		{"av1 mov mp3", func(s *Source) {
			s.Container, s.VideoCodec = "mov", "av1"
			s.Audio[0].Codec = "mp3"
		}, nil, Direct, false},
		{"no sound", func(s *Source) { s.Audio = nil }, nil, Direct, false},
		{"text subtitle needs no conversion", func(s *Source) { s.Subtitles = []Subtitle{{Codec: "subrip"}} }, func(q *Request) { q.SubtitleIndex = 0 }, Direct, false},
		{"under the height cap", nil, func(q *Request) { q.MaxHeight = 1080 }, Direct, false},
		{"only the container is wrong", func(s *Source) { s.Container = "mkv" }, nil, Remux, true},
		{"split across files", func(s *Source) { s.Parts = 2 }, nil, Remux, true},
		{"second sound track", func(s *Source) { s.Audio = append(s.Audio, Audio{Codec: "aac"}) }, func(q *Request) { q.AudioIndex = 1 }, Remux, true},
		{"another video stream", nil, func(q *Request) { q.VideoIndex = 1 }, Remux, true},
		{"direct play off", nil, func(q *Request) { q.AllowDirect = false }, Remux, true},
		{"only the sound is wrong", func(s *Source) { s.Audio[0].Codec = "ac3" }, nil, Remux, false},
		{"dts in mkv", func(s *Source) { s.Container = "mkv"; s.Audio[0].Codec = "dts" }, nil, Remux, false},
		{"truehd in mkv", func(s *Source) { s.Container = "mkv"; s.Audio[0].Codec = "truehd" }, nil, Remux, false},
		{"hevc can't be decoded", func(s *Source) { s.VideoCodec = "hevc" }, nil, Transcode, false},
		{"mpeg-2 can't be decoded", func(s *Source) { s.VideoCodec = "mpeg2video"; s.Container = "mpg" }, nil, Transcode, false},
		{"asked to convert", nil, func(q *Request) { q.ForceTranscode = true }, Transcode, false},
		{"over the height cap", nil, func(q *Request) { q.MaxHeight = 720 }, Transcode, false},
		{"picture subtitles", func(s *Source) { s.Subtitles = []Subtitle{{Codec: "hdmv_pgs_subtitle"}} }, func(q *Request) { q.SubtitleIndex = 0 }, Transcode, false},
		{"styled subtitles asked for", func(s *Source) { s.Subtitles = []Subtitle{{Codec: "ass"}} }, func(q *Request) { q.SubtitleIndex = 0; q.BurnSubtitle = true }, Transcode, false},
		{"hdr on an ordinary screen", func(s *Source) {
			s.Range = "hdr10"
			s.VideoCodec = "vp9"
			s.Container = "webm"
			s.Audio[0].Codec = "opus"
			s.BitDepth = 10
		}, nil, Transcode, false},
		{"interlaced", func(s *Source) { s.Interlaced = true }, nil, Transcode, false},
		{"10-bit h264", func(s *Source) { s.BitDepth = 10 }, nil, Transcode, false},
		{"12-bit vp9 plays", func(s *Source) {
			s.VideoCodec = "vp9"
			s.Container = "webm"
			s.Audio[0].Codec = "opus"
			s.BitDepth = 10
		}, nil, Direct, false},
		{"3D in 2D", func(s *Source) { s.Stereo = "sbs" }, func(q *Request) { q.Flatten3D = true }, Transcode, false},
		{"3D as it is", func(s *Source) { s.Stereo = "sbs" }, nil, Direct, false},
		{"over the limit away from home", nil, func(q *Request) { q.OverLimit = true }, Transcode, false},
		{"sound only goes through a stream", func(s *Source) { s.HasVideo = false; s.VideoCodec = "" }, nil, Transcode, false},
	}
	for _, c := range cases {
		s, q := h264MP4(), req()
		if c.src != nil {
			c.src(&s)
		}
		if c.q != nil {
			c.q(&q)
		}
		got := Decide(web, s, q)
		if got.Mode != c.want {
			t.Errorf("%s: got %s %v, want %s", c.name, got.Mode, got.Reasons, c.want)
		}
		if got.Mode == Remux && got.CopyAudio != c.copyAudio {
			t.Errorf("%s: copies the sound: %v", c.name, got.CopyAudio)
		}
	}
}

// What the web app reports changes the answer, in both directions.
func TestCapabilitiesChangeTheAnswer(t *testing.T) {
	yes := true
	hevc := h264MP4()
	hevc.VideoCodec, hevc.Container = "hevc", "mkv"
	browser := func(c Capabilities) Profile { return c.Apply(prof(t, "web")) }
	// A browser that decodes HEVC: a MKV is rewrapped into fMP4 rather than converted.
	p := browser(Capabilities{VideoCodecs: []string{"h264", "hevc"}, HLSVideo: []string{"h264", "hevc"}, FMP4Video: []string{"hevc"}})
	if d := Decide(p, hevc, req()); d.Mode != Remux || !d.FMP4 {
		t.Errorf("HEVC in MKV: %+v", d)
	}
	// In MP4 it plays as it is.
	hevc.Container = "mp4"
	if d := Decide(p, hevc, req()); d.Mode != Direct {
		t.Errorf("HEVC in MP4: %+v", d)
	}
	// An HDR screen: nothing is converted for it.
	hdr := hevc
	hdr.Range, hdr.BitDepth = "hdr10", 10
	if d := Decide(browser(Capabilities{VideoCodecs: []string{"hevc"}, HDR: []string{"hdr10", "hlg"}, TenBit: &yes}), hdr, req()); d.Mode != Direct {
		t.Errorf("HDR on an HDR screen: %+v", d)
	}
	// An ordinary one: tone mapped.
	if d := Decide(p, hdr, req()); d.Mode != Transcode || !d.ToneMap {
		t.Errorf("HDR on an ordinary screen: %+v", d)
	}
	// A browser with Dolby sound: not converted for it.
	ac3 := h264MP4()
	ac3.Audio[0].Codec = "ac3"
	if d := Decide(browser(Capabilities{AudioCodecs: []string{"aac", "ac3"}}), ac3, req()); d.Mode != Direct {
		t.Errorf("AC-3 where it's decoded: %+v", d)
	}
	// One with no H.264 in Media Source: nothing to rewrap into.
	mkv := h264MP4()
	mkv.Container = "mkv"
	no := false
	if d := Decide(browser(Capabilities{Remux: &no}), mkv, req()); d.Mode != Transcode {
		t.Errorf("no streams: %+v", d)
	}
	if d := Decide(browser(Capabilities{HiTenH264: &yes}), func() Source { s := h264MP4(); s.BitDepth = 10; return s }(), req()); d.Mode != Direct {
		t.Errorf("10-bit H.264 where it's decoded: %+v", d)
	}
}

func TestExplanations(t *testing.T) {
	web := prof(t, "web")
	s := h264MP4()
	e := Explain(Decide(web, s, req()), s, req())
	if e.Load != "none" || !strings.Contains(e.Summary, "as it is") || len(e.Reasons) != 0 {
		t.Errorf("direct: %+v", e)
	}
	s.Container = "mkv"
	e = Explain(Decide(web, s, req()), s, req())
	if e.Load != "light" || !strings.Contains(e.Summary, "Rewrapping") || !strings.Contains(e.Source, "Matroska") || len(e.Reasons) != 1 {
		t.Errorf("remux: %+v", e)
	}
	s.Audio[0].Codec = "dts"
	e = Explain(Decide(web, s, req()), s, req())
	if !strings.Contains(e.Summary, "only the sound") || !strings.Contains(e.Sound, "AAC") || !strings.Contains(e.Picture, "untouched") {
		t.Errorf("sound only: %+v", e)
	}
	s.VideoCodec = "mpeg2video"
	e = Explain(Decide(web, s, req()), s, req())
	if e.Load != "heavy" || !strings.Contains(e.Picture, "Converted") {
		t.Errorf("transcode: %+v", e)
	}
	// Every reason a decision can give has words.
	for code := range reasonWords {
		if ReasonText(code) == code {
			t.Errorf("%s has no words", code)
		}
	}
	for _, r := range []string{ReasonForced, ReasonDirectOff, ReasonContainer, ReasonVideoCodec, ReasonVideoTag, ReasonVideoHeight, ReasonVideoBitrate, ReasonVideoDepth, ReasonVideoRange, ReasonInterlaced, ReasonAudioCodec, ReasonAudioChans, ReasonAudioTrack, ReasonSubtitle, ReasonNoVideo, ReasonBurnAsked, ReasonHLSVideo, ReasonRemuxDisable, ReasonSplit, ReasonVideoTrack, ReasonFlatten3D} {
		if ReasonText(r) == r {
			t.Errorf("%s has no words", r)
		}
	}
	if a := ExplainAudio(false, "ape", "ape"); a.Load != "light" || !strings.Contains(a.Source, "Monkey") {
		t.Errorf("%+v", a)
	}
}

func TestStereoForBrowsersSurroundForTVs(t *testing.T) {
	s := h264MP4()
	s.Audio[0].Channels, s.Audio[0].Codec = 6, "dts"
	if d := Decide(prof(t, "web"), s, req()); d.AudioChannels != 2 {
		t.Errorf("browser: %d channels", d.AudioChannels)
	}
	if d := Decide(prof(t, "appletv"), s, req()); d.AudioChannels != 6 {
		t.Errorf("TV: %d channels", d.AudioChannels)
	}
}

func TestLanguageWords(t *testing.T) {
	for in, want := range map[string]string{"en": "en", "eng": "en", "English": "en", "pt-BR": "pt", "nld": "nl", "dutch": "nl", "xx": "", "forced": "", "hi": "hi", "a": ""} {
		if got := LanguageFromWord(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestDecide(t *testing.T) {
	type want struct {
		mode      string
		copyAudio bool
		fmp4      bool
		sub       string
		reason    string
	}
	cases := []struct {
		name    string
		profile string
		src     func(*Source)
		q       func(*Request)
		want    want
	}{
		{"apple: mp4 plays directly", "appletv", nil, nil, want{mode: Direct, sub: SubNone}},
		{"apple: mkv h264 rewraps, sound kept", "appletv", func(s *Source) { s.Container = "mkv" }, nil, want{mode: Remux, copyAudio: true, sub: SubNone, reason: ReasonContainer}},
		{"apple: mkv hevc rewraps in fmp4", "appletv", func(s *Source) { s.Container, s.VideoCodec, s.VideoTag = "mkv", "hevc", "hev1" }, nil, want{mode: Remux, copyAudio: true, fmp4: true, sub: SubNone, reason: ReasonContainer}},
		{"apple: hev1 mp4 is retagged", "appletv", func(s *Source) { s.VideoCodec, s.VideoTag = "hevc", "hev1" }, nil, want{mode: Remux, copyAudio: true, fmp4: true, sub: SubNone, reason: ReasonVideoTag}},
		{"apple: hvc1 mp4 direct", "appletv", func(s *Source) { s.VideoCodec, s.VideoTag = "hevc", "hvc1" }, nil, want{mode: Direct, sub: SubNone}},
		{"apple: dts sound converted, picture copied", "appletv", func(s *Source) {
			s.Container = "mkv"
			s.Audio[0].Codec = "dts"
		}, nil, want{mode: Remux, sub: SubNone, reason: ReasonAudioCodec}},
		{"apple: ac3 kept in hls", "appletv", func(s *Source) {
			s.Container = "mkv"
			s.Audio[0].Codec = "ac3"
		}, nil, want{mode: Remux, copyAudio: true, sub: SubNone}},
		{"apple: mpeg2 converts", "appletv", func(s *Source) { s.Container, s.VideoCodec = "mkv", "mpeg2video" }, nil, want{mode: Transcode, sub: SubNone, reason: ReasonHLSVideo}},
		{"apple: text sub becomes a rendition", "appletv", func(s *Source) { s.Subtitles = []Subtitle{{Codec: "subrip"}} }, func(q *Request) { q.SubtitleIndex = 0 }, want{mode: Remux, copyAudio: true, sub: SubHLS, reason: ReasonSubtitle}},
		{"apple: mov_text stays embedded", "appletv", func(s *Source) { s.Subtitles = []Subtitle{{Codec: "mov_text"}} }, func(q *Request) { q.SubtitleIndex = 0 }, want{mode: Direct, sub: SubEmbedded}},
		{"apple: pgs burns in", "appletv", func(s *Source) { s.Subtitles = []Subtitle{{Codec: "hdmv_pgs_subtitle"}} }, func(q *Request) { q.SubtitleIndex = 0 }, want{mode: Transcode, sub: SubBurn, reason: ReasonSubtitle}},
		{"apple: other sound track needs no new stream", "appletv", func(s *Source) { s.Audio = append(s.Audio, Audio{Codec: "aac"}) }, func(q *Request) { q.AudioIndex = 1 }, want{mode: Direct, sub: SubNone}},
		{"apple hd: hevc converts", "appletv-hd", func(s *Source) { s.VideoCodec, s.VideoTag = "hevc", "hvc1" }, nil, want{mode: Transcode, sub: SubNone, reason: ReasonVideoCodec}},
		{"apple hd: 4k converts", "appletv-hd", func(s *Source) { s.Height = 2160 }, nil, want{mode: Transcode, sub: SubNone, reason: ReasonVideoHeight}},
		{"hdr on an sdr display tone maps", "appletv-hd", func(s *Source) { s.Range = "hdr10" }, nil, want{mode: Transcode, sub: SubNone, reason: ReasonVideoRange}},

		{"android: mkv plays directly", "androidtv", func(s *Source) { s.Container = "mkv" }, nil, want{mode: Direct, sub: SubNone}},
		{"android: ac3 is rewrapped with converted sound", "androidtv", func(s *Source) {
			s.Container = "mkv"
			s.Audio[0].Codec = "ac3"
		}, nil, want{mode: Remux, sub: SubNone, reason: ReasonAudioCodec}},
		{"android: pgs is drawn by the player", "androidtv", func(s *Source) {
			s.Container = "mkv"
			s.Subtitles = []Subtitle{{Codec: "hdmv_pgs_subtitle"}}
		}, func(q *Request) { q.SubtitleIndex = 0 }, want{mode: Direct, sub: SubEmbedded}},
		{"android: ass embedded", "androidtv", func(s *Source) {
			s.Container = "mkv"
			s.Subtitles = []Subtitle{{Codec: "ass"}}
		}, func(q *Request) { q.SubtitleIndex = 0 }, want{mode: Direct, sub: SubEmbedded}},
		{"android: microdvd goes sidecar", "androidtv", func(s *Source) {
			s.Container = "mkv"
			s.Subtitles = []Subtitle{{Codec: "microdvd"}}
		}, func(q *Request) { q.SubtitleIndex = 0 }, want{mode: Direct, sub: SubSidecar}},
		{"asked to burn styled subtitles", "androidtv", func(s *Source) {
			s.Container = "mkv"
			s.Subtitles = []Subtitle{{Codec: "ass"}}
		}, func(q *Request) { q.SubtitleIndex, q.BurnSubtitle = 0, true }, want{mode: Transcode, sub: SubBurn, reason: ReasonBurnAsked}},
		{"10-bit h264 converts", "androidtv", func(s *Source) { s.BitDepth = 10 }, nil, want{mode: Transcode, sub: SubNone, reason: ReasonVideoDepth}},
		{"interlaced converts", "androidtv", func(s *Source) { s.Interlaced = true }, nil, want{mode: Transcode, sub: SubNone, reason: ReasonInterlaced}},
		{"too much data converts", "androidtv", nil, func(q *Request) { q.MaxBitrateKbps = 2000 }, want{mode: Transcode, sub: SubNone, reason: ReasonVideoBitrate}},
		{"forced conversion wins", "androidtv", nil, func(q *Request) { q.ForceTranscode = true }, want{mode: Transcode, sub: SubNone, reason: ReasonForced}},
	}
	for _, c := range cases {
		s, q := h264MP4(), req()
		if c.src != nil {
			c.src(&s)
		}
		if c.q != nil {
			c.q(&q)
		}
		got := Decide(prof(t, c.profile), s, q)
		if got.Mode != c.want.mode {
			t.Errorf("%s: mode %s %v, want %s", c.name, got.Mode, got.Reasons, c.want.mode)
			continue
		}
		if got.Mode == Remux && (got.CopyAudio != c.want.copyAudio || got.FMP4 != c.want.fmp4 || !got.CopyVideo) {
			t.Errorf("%s: %+v", c.name, got)
		}
		if got.Subtitle != c.want.sub {
			t.Errorf("%s: subtitle %s, want %s", c.name, got.Subtitle, c.want.sub)
		}
		if c.want.reason != "" && !slices.Contains(got.Reasons, c.want.reason) {
			t.Errorf("%s: reasons %v lack %s", c.name, got.Reasons, c.want.reason)
		}
		if got.Mode == Direct && len(got.Reasons) != 0 {
			t.Errorf("%s: direct with reasons %v", c.name, got.Reasons)
		}
	}
}

func TestToneMapOnlyWhenNeeded(t *testing.T) {
	s := h264MP4()
	s.VideoCodec, s.VideoTag, s.Range, s.BitDepth = "hevc", "hvc1", "hdr10", 10
	if d := Decide(prof(t, "appletv"), s, req()); d.Mode != Direct || d.ToneMap {
		t.Errorf("apple tv 4k shows hdr10: %+v", d)
	}
	if d := Decide(prof(t, "androidtv"), s, req()); d.Mode != Transcode || !d.ToneMap {
		t.Errorf("android without hdr tone maps: %+v", d)
	}
	p := (&Capabilities{HDR: []string{"HDR10"}}).Apply(prof(t, "androidtv"))
	if d := Decide(p, s, req()); d.Mode != Direct {
		t.Errorf("an app reporting hdr10 plays it: %+v", d)
	}
}

func TestCapabilitiesAreCleaned(t *testing.T) {
	big := 99999999
	p := (&Capabilities{
		VideoCodecs: []string{" AV1 ", "h264", "../../etc", "", "<script>"},
		MaxHeight:   &big,
	}).Apply(prof(t, "androidtv"))
	if !slices.Equal(p.VideoCodecs, []string{"av1", "h264"}) {
		t.Errorf("codecs: %v", p.VideoCodecs)
	}
	if p.MaxHeight != 2160 {
		t.Errorf("an absurd height is ignored, got %d", p.MaxHeight)
	}
	many := make([]string, 100)
	for i := range many {
		many[i] = "x"
	}
	if p := (&Capabilities{AudioCodecs: many}).Apply(prof(t, "web")); len(p.AudioCodecs) != 32 {
		t.Errorf("kept %d names", len(p.AudioCodecs))
	}
	if q := (*Capabilities)(nil).Apply(prof(t, "web")); q.ID != "web" {
		t.Error("nil capabilities change nothing")
	}
}

func TestChooseTracks(t *testing.T) {
	src := Source{
		Audio: []Audio{{Language: "eng", Title: "Commentary", Comment: true}, {Language: "jpn"}, {Language: "eng", Default: true}},
		Subtitles: []Subtitle{
			{Language: "eng", Forced: true},
			{Language: "eng", SDH: true},
			{Language: "eng"},
			{Language: "nld"},
		},
	}
	cases := []struct {
		name       string
		audio, sub string
		want       Chosen
	}{
		{"defaults: the marked track, its forced subtitles", "", "", Chosen{Audio: 2, Subtitle: 0}},
		{"language wanted, tags differ", "ja-JP", "", Chosen{Audio: 1, Subtitle: -1}},
		{"regular subtitles before forced and SDH", "", "en", Chosen{Audio: 2, Subtitle: 2}},
		{"three letter and two letter codes match", "eng", "nl", Chosen{Audio: 2, Subtitle: 3}},
		{"language not there", "fr", "de", Chosen{Audio: 2, Subtitle: 0}},
	}
	for _, c := range cases {
		if got := ChooseTracks(src, c.audio, c.sub); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
	if got := ChooseTracks(Source{}, "en", "en"); got != (Chosen{Audio: 0, Subtitle: -1}) {
		t.Errorf("an empty file: %+v", got)
	}
	only := Source{Audio: []Audio{{Language: "eng", Comment: true}}}
	if got := ChooseTracks(only, "", ""); got.Audio != 0 {
		t.Errorf("only commentary: %+v", got)
	}
}

func TestSameLanguage(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"eng", "en", true}, {"en-US", "eng", true}, {"dut", "nl", true}, {"nld", "dut", true}, {"und", "und", false}, {"", "", false}, {"fr", "de", false}} {
		if SameLanguage(c.a, c.b) != c.want {
			t.Errorf("%q %q: want %v", c.a, c.b, c.want)
		}
	}
}
