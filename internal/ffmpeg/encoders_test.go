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
