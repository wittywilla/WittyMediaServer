package libtranscode

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

// atomic32 is a counter the running-controls test shares with the engine.
type atomic32 = atomic.Int32

func testEnv() Env {
	return Env{
		Encoders:  map[string]bool{"libx264": true, "libx265": true, "libsvtav1": true, "aac": true, "ac3": true, "eac3": true, "libopus": true, "hevc_nvenc": true},
		HasFilter: func(string) bool { return true }, ToneMap: func(string) string { return "tonemap-chain" }, Sidecars: true,
	}
}

func h264(height int) VideoStream {
	return VideoStream{Index: 0, Codec: "h264", PixFmt: "yuv420p", Width: height * 16 / 9, Height: height, Depth: 8, Range: "sdr", FPS: 24, Default: true}
}

func stepsOf(pl *Plan, kind string) []Step {
	var out []Step
	for _, st := range pl.Steps {
		if st.Kind == kind {
			out = append(out, st)
		}
	}
	return out
}

func TestPresetsAreValid(t *testing.T) {
	for _, p := range Presets() {
		pol := p.Policy
		if err := pol.Validate(); err != nil {
			t.Errorf("%s: %v", p.ID, err)
		}
		if p.Name == "" || p.Summary == "" || p.Cost == "" || len(p.Details) == 0 {
			t.Errorf("%s says too little", p.ID)
		}
	}
}

func TestValidateRefusesBadValues(t *testing.T) {
	good := func() Policy { p, _ := PresetPolicy(PresetCompatible); return p }
	bad := map[string]func(*Policy){
		"unknown preset":   func(p *Policy) { p.Preset = "x" },
		"container":        func(p *Policy) { p.Container = "avi -i /etc/passwd" },
		"video codec":      func(p *Policy) { p.Video.Codec = "-vf" },
		"quality":          func(p *Policy) { p.Video.Quality = 99 },
		"height":           func(p *Policy) { p.Video.MaxHeight = -1 },
		"audio codec":      func(p *Policy) { p.Audio.Codec = "copy;rm" },
		"audio language":   func(p *Policy) { p.Audio.Languages = []string{"en', evil"} },
		"sub language":     func(p *Policy) { p.Subtitles.Languages = []string{strings.Repeat("a", 40)} },
		"codec list":       func(p *Policy) { p.Audio.Accept = []string{"aa c"} },
		"channels":         func(p *Policy) { p.Audio.MaxChannels = 3 },
		"nothing to keep":  func(p *Policy) { p.Audio.Keep, p.Audio.KeepOriginal, p.Audio.Languages = KeepLanguages, false, nil },
		"no sub languages": func(p *Policy) { p.Subtitles.Keep, p.Subtitles.Languages = KeepLanguages, nil },
	}
	for name, change := range bad {
		p := good()
		change(&p)
		if p.Validate() == nil {
			t.Errorf("%s: allowed", name)
		}
	}
	p := good()
	p.Audio.Keep, p.Audio.Languages = KeepLanguages, []string{"eng", "en", "ja-JP", "FRE"}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.Audio.Languages, []string{"en", "ja", "fr"}) {
		t.Fatalf("languages weren't tidied: %v", p.Audio.Languages)
	}
}

func TestAlreadyFittingFileIsSkipped(t *testing.T) {
	p, _ := PresetPolicy(PresetCompatible)
	src := &Source{Path: "/m/a.mp4", Ext: "mp4", Duration: 60, Video: []VideoStream{h264(1080)},
		Audio: []AudioStream{{Index: 1, Codec: "aac", Channels: 2, Language: "en", Default: true}}}
	if pl := Decide(src, p, testEnv(), PlanOptions{}); pl.Verdict != VerdictSkip {
		t.Fatalf("%s: %s %+v", pl.Verdict, pl.Reason, pl.Steps)
	}
}

func TestTheCanonicalFile(t *testing.T) {
	// A film as it's often found: surround from a disc, two languages, picture subtitles.
	src := &Source{Path: "/m/a.mkv", Ext: "mkv", Duration: 7200, Size: 30 << 30, Video: []VideoStream{{Index: 0, Codec: "hevc", PixFmt: "yuv420p10le", Width: 3840, Height: 2160, Depth: 10, Range: "hdr10", FPS: 24, Default: true, Kbps: 30000}},
		Audio: []AudioStream{
			{Index: 1, Codec: "truehd", Profile: "Dolby TrueHD + Dolby Atmos", Channels: 8, Language: "en", Default: true, Lossless: true, Objects: true},
			{Index: 2, Codec: "ac3", Channels: 6, Language: "en"},
			{Index: 3, Codec: "dts", Channels: 6, Language: "fr"},
			{Index: 4, Codec: "ac3", Channels: 2, Language: "en", Commentary: true, Title: "Commentary"},
		},
		Subs: []SubStream{
			{Index: 5, Codec: "hdmv_pgs_subtitle", Language: "en"},
			{Index: 6, Codec: "hdmv_pgs_subtitle", Language: "en", Forced: true},
			{Index: 7, Codec: "subrip", Language: "fr", Text: true},
		}}
	p, _ := PresetPolicy(PresetCompatible)
	pl := Decide(src, p, testEnv(), PlanOptions{})
	if pl.Verdict != VerdictConvert || pl.Container != "mp4" || pl.Ext != "mp4" {
		t.Fatalf("%+v", pl)
	}
	v := stepsOf(pl, "video")[0]
	if v.encoder != "libx264" || !slices.Contains(v.filters, "tonemap-chain") || !slices.Contains(v.filters, "scale=-2:1080:flags=bicubic") {
		t.Fatalf("%+v", v)
	}
	if !strings.Contains(v.To, "1080p") || !strings.Contains(v.To, "ordinary") {
		t.Fatalf("to: %q", v.To)
	}
	var kept []string
	for _, st := range stepsOf(pl, "audio") {
		if st.Action != ActDrop {
			kept = append(kept, st.Action+":"+st.From[:7])
		}
	}
	t.Log(kept)
	audio := stepsOf(pl, "audio")
	if audio[0].Action != ActAdd || audio[0].encoder != "aac" || audio[0].disposition != "default" {
		t.Fatalf("the first track should be a new default stereo one: %+v", audio[0])
	}
	found := false
	for _, w := range pl.Warnings {
		if strings.Contains(w, "subtitle") {
			t.Errorf("no subtitle is lost here: %s", w)
		}
	}
	for _, st := range pl.exports {
		if st.Action == ActExport && st.language == "en" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the English picture subtitle should be exported: %+v", pl.exports)
	}
	if pl.burn == nil || !strings.Contains(pl.burn.From, "forced") {
		t.Fatalf("the forced track should be burned: %+v", pl.burn)
	}
	// Objects are kept as they are, and TrueHD can't go in an MP4, so it's converted, and says so.
	said := false
	for _, w := range pl.Warnings {
		if strings.Contains(w, "Object sound won't survive") {
			said = true
		}
	}
	if !said {
		t.Fatalf("no warning about Atmos: %v", pl.Warnings)
	}
}

func TestObjectAudioKeptInMKV(t *testing.T) {
	src := &Source{Path: "/m/a.mkv", Ext: "mkv", Duration: 60, Video: []VideoStream{h264(1080)},
		Audio: []AudioStream{{Index: 1, Codec: "eac3", Profile: "Dolby Digital Plus + Dolby Atmos", Channels: 6, Language: "en", Default: true, Objects: true}}}
	p, _ := PresetPolicy(PresetSaveSpace)
	p.Video.Accept = []string{"h264"}
	pl := Decide(src, p, testEnv(), PlanOptions{})
	if pl.Verdict != VerdictSkip {
		t.Fatalf("%s %+v", pl.Verdict, pl.Steps)
	}
	if !hasObjects("Dolby TrueHD + Dolby Atmos") || !hasObjects("DTS-HD MA + DTS:X") || hasObjects("DTS-HD MA") {
		t.Fatal("objects misread")
	}
	if !isLossless("dts", "DTS-HD MA") || isLossless("dts", "DTS") || !isLossless("pcm_s24le", "") {
		t.Fatal("lossless misread")
	}
}

func TestMainPicture(t *testing.T) {
	s := &Source{Video: []VideoStream{{Index: 0, Width: 320, Height: 240}, {Index: 1, Width: 1920, Height: 1080}}}
	if s.Main() != 1 {
		t.Fatal("the largest is the main one when nothing says")
	}
	s.Video[0].Default = true
	if s.Main() != 1 {
		t.Fatal("tools mark the first by themselves, so that doesn't outweigh size")
	}
	s = &Source{Video: []VideoStream{{Index: 0, Width: 1920, Height: 1080}, {Index: 1, Width: 320, Height: 240, Default: true}}}
	if s.Main() != 1 {
		t.Fatal("a later stream marked default is the main one")
	}
	s.Video[0].Default = true
	if s.Main() != 0 {
		t.Fatal("marking them all says nothing, so the largest it is")
	}
}

func TestTitlesFromFilesStaySeparateArguments(t *testing.T) {
	src := &Source{Path: "/m/-evil name.mkv", Ext: "mkv", Duration: 60, Video: []VideoStream{h264(1080)},
		Audio: []AudioStream{{Index: 1, Codec: "aac", Channels: 2, Language: "en", Default: true, Title: "-i /etc/passwd"}}}
	p, _ := PresetPolicy(PresetCompatible)
	pl := Decide(src, p, testEnv(), PlanOptions{})
	args := pl.Args(src, Output{Path: "/m/out.tmp"})
	if slices.Contains(args, "/etc/passwd") {
		t.Fatalf("a track title reached the command line: %v", args)
	}
	i := slices.Index(args, "-i")
	if args[i+1] != "/m/-evil name.mkv" || args[i-1] != "file,pipe" {
		t.Fatalf("%v", args)
	}
}

func TestCleanTitle(t *testing.T) {
	if got := cleanTitle("a\nb\x00c"); got != "abc" {
		t.Fatal(got)
	}
	if got := cleanTitle(strings.Repeat("x", 500)); len(got) != 200 {
		t.Fatal(len(got))
	}
}

func TestHours(t *testing.T) {
	s := Settings{HoursOn: true, HoursFrom: 22, HoursTo: 6}
	for h, want := range map[int]bool{21: false, 22: true, 23: true, 0: true, 5: true, 6: false, 12: false} {
		if s.inHours(h) != want {
			t.Errorf("%d: %v", h, !want)
		}
	}
	s = Settings{HoursOn: true, HoursFrom: 9, HoursTo: 17}
	for h, want := range map[int]bool{8: false, 9: true, 16: true, 17: false} {
		if s.inHours(h) != want {
			t.Errorf("%d: %v", h, !want)
		}
	}
	if !(Settings{}).inHours(3) {
		t.Fatal("no limit means any hour")
	}
}

func TestSettingsValidate(t *testing.T) {
	bad := []Settings{
		{MaxJobs: 0, MinFreeGB: 1},
		{MaxJobs: 99, MinFreeGB: 1},
		{MaxJobs: 1, MinFreeGB: 0},
		{MaxJobs: 1, MinFreeGB: 1, HoursOn: true, HoursFrom: 3, HoursTo: 3},
		{MaxJobs: 1, MinFreeGB: 1, HoursFrom: 30},
		{MaxJobs: 1, MinFreeGB: 1, Enabled: true},
	}
	for _, s := range bad {
		if s.Validate() == nil {
			t.Errorf("allowed %+v", s)
		}
	}
	good := DefaultSettings()
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	if good.Enabled {
		t.Fatal("this must start off")
	}
}

func TestWithoutHEVCSaveSpaceUsesH264AndSays(t *testing.T) {
	env := testEnv()
	delete(env.Encoders, "libx265")
	delete(env.Encoders, "hevc_nvenc")
	src := &Source{Path: "/m/a.mkv", Ext: "mkv", Duration: 60, Size: 2 << 30, Kbps: 20000, Video: []VideoStream{{Index: 0, Codec: "mpeg2video", PixFmt: "yuv420p", Width: 1920, Height: 1080, Depth: 8, Range: "sdr", Kbps: 19000}}}
	p, _ := PresetPolicy(PresetSaveSpace)
	pl := Decide(src, p, env, PlanOptions{})
	st := stepsOf(pl, "video")[0]
	if st.encoder != "libx264" {
		t.Fatalf("%+v", st)
	}
	if !slices.Contains(st.args, "26") {
		t.Fatalf("the setting should be stronger to save anything: %v", st.args)
	}
	said := false
	for _, n := range pl.Notes {
		if strings.Contains(n, "can't encode H.265") {
			said = true
		}
	}
	if !said {
		t.Fatalf("%v", pl.Notes)
	}
	// HDR can't fall back, and is left alone with a reason.
	src.Video[0].Range, src.Video[0].Depth, src.Video[0].PixFmt = "hdr10", 10, "yuv420p10le"
	pl = Decide(src, p, env, PlanOptions{})
	if pl.Verdict != VerdictRefuse {
		t.Fatalf("%+v", pl)
	}
}

func TestHardwareOnlyForOrdinaryPictures(t *testing.T) {
	env := testEnv()
	env.Hardware = []string{"hevc_nvenc"}
	p, _ := PresetPolicy(PresetSaveSpace)
	p.Video.SkipBelowKbps = 0
	src := &Source{Path: "/m/a.mkv", Ext: "mkv", Duration: 60, Video: []VideoStream{{Index: 0, Codec: "mpeg2video", PixFmt: "yuv420p", Width: 1920, Height: 1080, Depth: 8, Range: "sdr"}}}
	if st := stepsOf(Decide(src, p, env, PlanOptions{}), "video")[0]; st.encoder != "hevc_nvenc" {
		t.Fatalf("%+v", st)
	}
	src.Video[0].Codec, src.Video[0].Range, src.Video[0].Depth, src.Video[0].PixFmt = "vp8", "hdr10", 10, "yuv420p10le"
	if st := stepsOf(Decide(src, p, env, PlanOptions{}), "video")[0]; st.encoder != "libx265" {
		t.Fatalf("HDR should use the software encoder: %+v", st)
	}
	if !slices.Contains(stepsOf(Decide(src, p, env, PlanOptions{}), "video")[0].args, "-x265-params") {
		t.Fatal("no HDR description")
	}
}

func TestEstimatesAreSensible(t *testing.T) {
	p, _ := PresetPolicy(PresetSaveSpace)
	src := &Source{Path: "/m/a.mkv", Ext: "mkv", Duration: 7200, Size: 20 << 30, Kbps: 22000, Video: []VideoStream{{Index: 0, Codec: "h264", PixFmt: "yuv420p", Width: 1920, Height: 1080, Depth: 8, Range: "sdr", FPS: 24, Kbps: 20000}},
		Audio: []AudioStream{{Index: 1, Codec: "ac3", Channels: 6, Language: "en", Default: true}}}
	pl := Decide(src, p, testEnv(), PlanOptions{})
	if pl.Verdict != VerdictConvert {
		t.Fatal(pl.Verdict)
	}
	if pl.EstBytes <= 0 || pl.EstBytes >= src.Size {
		t.Fatalf("a bloated film should shrink: %d of %d", pl.EstBytes, src.Size)
	}
	if pl.EstSec <= 0 {
		t.Fatal("no time")
	}
	// An already tiny file isn't promised a saving.
	src.Size, src.Kbps = 1<<28, 500
	src.Video[0].Kbps = 300
	src.Audio[0].Kbps = 128
	p.Video.SkipBelowKbps = 0
	if pl = Decide(src, p, testEnv(), PlanOptions{}); pl.EstBytes > src.Size*2 {
		t.Fatalf("%d", pl.EstBytes)
	}
}

// Marks a protected file carries, made by hand, because no tool here can make them.
func TestProtectionMarks(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	t.Run("matroska", func(t *testing.T) {
		head := []byte{0x1A, 0x45, 0xDF, 0xA3, 0x80, 0x18, 0x53, 0x80, 0x67, 0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
		tracks := func(body ...byte) []byte {
			return append([]byte{0x16, 0x54, 0xAE, 0x6B, 0x80 | byte(len(body))}, body...)
		}
		enc := append(append([]byte{}, head...), tracks(0x6D, 0x80, 0x88, 0x50, 0x33, 0x81, 0x01, 0x50, 0x35, 0x80)...)
		if !Detect(write("enc.mkv", enc), nil) {
			t.Error("encryption not found")
		}
		// Compression, which is also a content encoding, isn't protection.
		comp := append(append([]byte{}, head...), tracks(0x6D, 0x80, 0x84, 0x50, 0x33, 0x81, 0x00)...)
		if Detect(write("comp.mkv", comp), nil) {
			t.Error("compression called protection")
		}
		// The marks in picture data, after the track list, mean nothing.
		data := append(append([]byte{}, head...), tracks(0x83, 0x81, 0x01)...)
		data = append(data, 0x6D, 0x80, 0x50, 0x35, 0x50, 0x33, 0x81, 0x01)
		if Detect(write("data.mkv", data), nil) {
			t.Error("picture data called protection")
		}
		// As a muxer writes them: a seek index naming Tracks first, then the segment's
		// details, whose random IDs can hold the marks, then the real track list.
		el := func(id []byte, body ...byte) []byte {
			return append(append(append([]byte{}, id...), 0x80|byte(len(body))), body...)
		}
		var (
			idTracks   = []byte{0x16, 0x54, 0xAE, 0x6B}
			seek       = el([]byte{0x11, 0x4D, 0x9B, 0x74}, el([]byte{0x4D, 0xBB}, el([]byte{0x53, 0xAB}, idTracks...)...)...)
			info       = el([]byte{0x15, 0x49, 0xA9, 0x66}, el([]byte{0x73, 0xA4}, 0x6D, 0x80, 0x50, 0x35, 0x50, 0x33, 0x81, 0x01, 0, 0, 0, 0, 0, 0, 0, 0)...)
			uidMarks   = el([]byte{0x73, 0xC5}, 0x6D, 0x80, 0x50, 0x35, 0, 0, 0, 1)
			encrypted  = el([]byte{0x6D, 0x80}, el([]byte{0x62, 0x40}, append(el([]byte{0x50, 0x33}, 0x01), el([]byte{0x50, 0x35})...)...)...)
			compressed = el([]byte{0x6D, 0x80}, el([]byte{0x62, 0x40}, el([]byte{0x50, 0x33}, 0x00)...)...)
		)
		segment := func(entry ...byte) []byte {
			body := append(append(append([]byte{}, seek...), info...), el(idTracks, el([]byte{0xAE}, entry...)...)...)
			return append(append(append([]byte{}, head[:5]...), 0x18, 0x53, 0x80, 0x67, 0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF), body...)
		}
		if Detect(write("clear.mkv", segment(uidMarks...)), nil) {
			t.Error("the marks in a seek index, the segment's IDs and a track's ID called protection")
		}
		if !Detect(write("track.mkv", segment(append(append([]byte{}, uidMarks...), encrypted...)...)), nil) {
			t.Error("an encrypted track not found")
		}
		if Detect(write("zipped.mkv", segment(compressed...)), nil) {
			t.Error("a compressed track called protection")
		}
	})
	t.Run("asf", func(t *testing.T) {
		enc := append(append([]byte{}, asfHeaderGUID...), make([]byte, 30)...)
		enc = append(enc, asfEncryption...)
		if !Detect(write("enc.wmv", enc), nil) {
			t.Error("encryption not found")
		}
		if Detect(write("plain.wmv", append(append([]byte{}, asfHeaderGUID...), make([]byte, 64)...)), nil) {
			t.Error("plain ASF called protection")
		}
	})
	t.Run("old iTunes tracks", func(t *testing.T) {
		box := func(kind string, body ...byte) []byte {
			n := 8 + len(body)
			return append(append([]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}, kind...), body...)
		}
		ftyp := box("ftyp", 'M', '4', 'P', ' ', 0, 0, 0, 0)
		entry := box("drms", make([]byte, 28)...)
		stsd := box("stsd", append([]byte{0, 0, 0, 0, 0, 0, 0, 1}, entry...)...)
		moov := box("moov", box("trak", box("mdia", box("minf", box("stbl", stsd...)...)...)...)...)
		if !Detect(write("old.m4p", append(ftyp, moov...)), nil) {
			t.Error("not found")
		}
	})
	t.Run("a TiVo file", func(t *testing.T) {
		if !Detect(write("rec.tivo", append([]byte("TiVo"), make([]byte, 64)...)), nil) {
			t.Error("not found")
		}
	})
}

func TestStreamArgs(t *testing.T) {
	got := streamArgs([]string{"-preset", "fast", "-profile:v", "high", "-q:v", "60", "-b:v", "0", "-x265-params", "a=1"}, "v:1")
	want := []string{"-preset:v:1", "fast", "-profile:v:1", "high", "-q:v:1", "60", "-b:v:1", "0", "-x265-params:v:1", "a=1"}
	if !slices.Equal(got, want) {
		t.Fatalf("%v", got)
	}
}

func TestLengthOK(t *testing.T) {
	cases := []struct {
		a, b float64
		ok   bool
	}{{100, 100, true}, {100, 101.5, true}, {100, 103, false}, {7200, 7260, true}, {7200, 7400, false}, {0, 5, true}, {0, 0, false}}
	for _, c := range cases {
		if lengthOK(c.a, c.b) != c.ok {
			t.Errorf("%v", c)
		}
	}
}

func TestFilterPathEscaping(t *testing.T) {
	got := escapeFilterPath("/m/It's [odd].mkv")
	if !strings.Contains(got, `\'`) {
		t.Fatal(got)
	}
}

// A conversion reads a library's files, some of which aren't what they say, so every input is held
// to the same list of formats as playing one.
func TestEveryInputKeepsToMediaFormats(t *testing.T) {
	check := func(what string, args []string) {
		t.Helper()
		seen := 0
		for i, a := range args {
			if a != "-i" {
				continue
			}
			seen++
			if !slices.Contains(args[:i], "-format_whitelist") {
				t.Errorf("%s: %q has no list of formats", what, args[i+1])
			}
		}
		if seen == 0 {
			t.Errorf("%s: no input: %v", what, args)
		}
	}
	check("a cover", CoverArgs("/lib/film.mkv", Step{index: 2}, "/tmp/cover.jpg"))
	pl := &Plan{}
	check("a conversion", pl.Args(&Source{Path: "/lib/film.mkv"}, Output{Path: "/tmp/out.mkv"}))
	pl.embeds = append(pl.embeds, Step{path: "/lib/film.en.srt"})
	args := pl.Args(&Source{Path: "/lib/film.mkv"}, Output{Path: "/tmp/out.mkv"})
	n := 0
	for _, a := range args {
		if a == "-format_whitelist" {
			n++

		}
	}
	if n != 2 {
		t.Errorf("each input needs its own list of formats: %v", args)
	}
}
