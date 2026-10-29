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
