package ffmpeg

import (
	"reflect"
	"testing"
)

func TestSplitArgs(t *testing.T) {
	got := SplitArgs(`-preset slow  -x264-params "aq-mode=3:psy-rd=1.0" -tune 'film grain' ""`)
	want := []string{"-preset", "slow", "-x264-params", "aq-mode=3:psy-rd=1.0", "-tune", "film grain", ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
}

func TestPlanFallsBackToSoftware(t *testing.T) {
	tools := &Tools{Encoders: map[string]bool{"libx264": true, "h264_nvenc": true}}
	// NVENC is compiled in but was never found working: libx264 must be used.
	p := tools.Plan(Quality{HW: []string{"h264_nvenc"}, Preset: "medium", CRF: 19})
	if p.Name != "libx264" {
		t.Fatalf("expected libx264, got %s", p.Name)
	}
	if !reflect.DeepEqual(p.Args[:4], []string{"-preset", "medium", "-crf", "19"}) {
		t.Fatalf("args %q", p.Args)
	}
	hw.mu.Lock()
	hw.status["h264_nvenc"] = &HWStatus{Tested: true, Working: true}
	hw.mu.Unlock()
	defer func() { hw.mu.Lock(); delete(hw.status, "h264_nvenc"); hw.mu.Unlock() }()
	if p := tools.Plan(Quality{HW: []string{"h264_nvenc"}, Preset: "medium"}); p.Name != "h264_nvenc" || p.Args[1] != "p5" {
		t.Fatalf("nvenc plan %+v", p)
	}
	// Expert options replace the encoder options.
	if p := tools.Plan(Quality{VideoArgs: []string{"-crf", "30"}}); !reflect.DeepEqual(p.Args, []string{"-crf", "30"}) {
		t.Fatalf("expert args %q", p.Args)
	}
}

func TestCheckExpertArgs(t *testing.T) {
	for _, ok := range []string{
		"", "-preset slow -crf 19 -tune film -x264-params aq-mode=3", "-preset p6 -tune hq -rc vbr -cq 21 -b:v 0 -spatial-aq 1 -temporal-aq 1",
		"-preset slower -global_quality 20 -look_ahead 1", "-qp 20 -quality 1", "-threads 4", `-x264-params "aq-mode=3:psy-rd=1.0"`, "-movflags +faststart", "-bf -1",
	} {
		if err := CheckExpertArgs(SplitArgs(ok)); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"-y /etc/cron.d/x", "/tmp/out.mp4", "-i /etc/passwd", "-i http://169.254.169.254/ -c copy", "-filter_complex movie=/etc/passwd", "-vf drawtext=textfile=/etc/passwd",
		"-attach /etc/passwd", "-dump_attachment:t /tmp/x", "-passlogfile /tmp/x", "-f hls", "-hls_segment_filename /tmp/x", "-protocol_whitelist file,http",
		"-x264-params stats=/tmp/x", "-x264-params dump-yuv=x.yuv", "-x265-params csv=x.csv", "-x264-params aq-mode=3:stats=x", "-preset ../x", `-preset C:\\x`, "-crf -i", "-threads", "-tune -y",
	} {
		if err := CheckExpertArgs(SplitArgs(bad)); err == nil {
			t.Errorf("%q was allowed", bad)
		}
	}
}

func TestCapEveryEncoder(t *testing.T) {
	tools := &Tools{Encoders: map[string]bool{}}
	q := Quality{CapRate: "4000k", Preset: "veryfast", CRF: 21}
	has := func(args []string, want ...string) bool {
		for i := 0; i+len(want) <= len(args); i++ {
			match := true
			for j, w := range want {
				match = match && args[i+j] == w
			}
			if match {
				return true
			}
		}
		return false
	}
	constant := map[string][]string{"h264_qsv": {"-global_quality"}, "h264_amf": {"-qp_i", "cqp"}, "h264_vaapi": {"-qp"}, "h264_videotoolbox": {"-q:v"}}
	for _, name := range []string{"libx264", "h264_nvenc", "h264_qsv", "h264_amf", "h264_vaapi", "h264_videotoolbox", "libopenh264"} {
		args := tools.planFor(name, q).Args
		if !has(args, "-maxrate", "4000k") || !has(args, "-bufsize", "8000k") {
			t.Errorf("%s has no limit: %v", name, args)
		}
		for _, bad := range constant[name] {
			if has(args, bad) {
				t.Errorf("%s stays in a constant quality mode that ignores the limit: %v", name, args)
			}
		}
	}
	// Expert options can't lift it: the limit comes last.
	expert := q
	expert.VideoArgs = []string{"-preset", "slow", "-maxrate", "50M"}
	args := tools.planFor("libx264", expert).Args
	if args[len(args)-4] != "-maxrate" || args[len(args)-3] != "4000k" {
		t.Errorf("expert options: %v", args)
	}
	if args := tools.Plan(q).Args; !has(args, "-maxrate", "4000k") {
		t.Errorf("the last-resort encoder: %v", args)
	}
	if args := tools.planFor("libx264", Quality{}).Args; has(args, "-maxrate") {
		t.Errorf("no limit, no limit options: %v", args)
	}
}

func TestParseRate(t *testing.T) {
	for in, want := range map[string]int64{"8M": 8000000, "1500k": 1500000, "2000000": 2000000, "1.5m": 1500000, "": 0, "x": 0, "-4M": 0, "M": 0} {
		if got := ParseRate(in); got != want {
			t.Errorf("%q: got %d", in, got)
		}
	}
}

func TestValidRateAndPreset(t *testing.T) {
	for v, want := range map[string]bool{"": true, "8M": true, "192k": true, "1.5M": true, "-y": false, "8M /tmp/x": false, "../x": false} {
		if ValidRate(v) != want {
			t.Errorf("ValidRate(%q) = %v", v, !want)
		}
	}
	if ValidPreset("-y") || !ValidPreset("veryfast") {
		t.Error("ValidPreset")
	}
}

func TestWithCap(t *testing.T) {
	q := Quality{AudioRate: "256k"}.WithCap("4M")
	if q.CapRate != "3808k" || q.AudioRate != "192k" {
		t.Errorf("sound and picture must fit in the total: %+v", q)
	}
	q = Quality{}.WithCap("1M")
	if q.CapRate != "872k" || q.AudioRate != "128k" {
		t.Errorf("small limits give sound less: %+v", q)
	}
	q = Quality{MaxRate: "2M"}.WithCap("8M")
	if q.CapRate != "2000k" {
		t.Errorf("a lower limit in Settings still counts: %+v", q)
	}
	q = Quality{}.WithCap("")
	if q.CapRate != "" {
		t.Errorf("no limit: %+v", q)
	}
}

func TestParseDecoders(t *testing.T) {
	out := "Decoders:\n V..... = Video\n ------\n VFS..D hevc                 HEVC\n A....D ac3                  AC-3\n"
	got := parseDecoders(out)
	if !got["hevc"] || !got["ac3"] || got["ac4"] || got["="] {
		t.Fatalf("%v", got)
	}
}
