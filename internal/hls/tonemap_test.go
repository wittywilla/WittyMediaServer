package hls

import (
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// HDR is mapped once the picture is the size it's sent at: the mapping brings it down to size
// itself, after deinterlacing and the like, and before subtitles are drawn on the result.
func TestToneMapAtTheFinalSize(t *testing.T) {
	tools := &ffmpeg.Tools{Encoders: map[string]bool{"libx264": true}}
	vf := func(sp Spec) string {
		s := &Session{Spec: sp, Dir: t.TempDir(), tools: tools, m: &Manager{tools: tools}}
		args, _, _ := s.commandLine(0)
		for i, a := range args {
			if a == "-vf" {
				return args[i+1]
			}
		}
		t.Fatalf("no -vf in %q", args)
		return ""
	}
	sp := Spec{InputArgs: []string{"-i", "in.mkv"}, Duration: 60, SegmentSec: 4, AudioIndex: 0, SubtitleIndex: 0, SubtitleText: true, Source: "in.mkv",
		VideoFilter: "bwdif=mode=send_frame", ToneMap: "hdr10", MaxHeight: 1080}
	got := vf(sp)
	mapped := strings.Index(got, ffmpeg.ToneMapTo("hdr10", 1080))
	if mapped < 0 {
		t.Fatalf("not mapped at 1080 lines:\n%s", got)
	}
	if d, sar := strings.Index(got, "bwdif"), strings.Index(got, "scale=trunc(iw*sar/2)*2:ih"); d < 0 || d > sar || sar > mapped {
		t.Errorf("deinterlacing and square pixels come first:\n%s", got)
	}
	if subs := strings.Index(got, "subtitles="); subs < mapped {
		t.Errorf("subtitles are drawn on the mapped picture:\n%s", got)
	}
	if strings.Contains(got, "scale=-2:'min(ih,1080)'") {
		t.Errorf("brought down to size twice:\n%s", got)
	}
	sp.MaxHeight = 0
	if got := vf(sp); !strings.Contains(got, ffmpeg.ToneMapChain("hdr10")) {
		t.Errorf("at its own size, the chain is the plain one:\n%s", got)
	}
	sp.ToneMap, sp.MaxHeight = "", 720
	if got := vf(sp); !strings.Contains(got, "scale=-2:'min(ih,720)'") || strings.Contains(got, "zscale") {
		t.Errorf("an ordinary picture is scaled as before:\n%s", got)
	}
}
