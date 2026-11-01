package subtitle

import (
	"strings"
	"testing"
	"time"
)

func TestConvert(t *testing.T) {
	for _, c := range []struct {
		format, in string
		want       []string
	}{
		{"srt", "1\n00:00:01,500 --> 00:00:02,000\nHi <i>there</i> <font color=\"red\">red</font>\nSecond line\n\n2\n0:0:3,5 --> 0:0:4,0\nNext\n", []string{"WEBVTT", "00:00:01.500 --> 00:00:02.000", "Hi <i>there</i> red", "Second line", "00:00:03.500 --> 00:00:04.000", "Next"}},
		{"srt", "\xEF\xBB\xBF1\r\n00:00:01,000 --> 00:00:02,000\r\nCaf\xC3\xA9\r\n", []string{"Café"}},
		{"srt", "1\n00:00:01,000 --> 00:00:02,000\nCaf\xE9\n", []string{"Café"}},
		{"srt", "1\n00:00:01,000 --> 00:00:02,000\nnot --> a cue\n", []string{"not -&gt; a cue"}},
		{"sbv", "0:00:01.000,0:00:02.500\nHello\nthere\n\n0:00:03.000,0:00:04.000\nAgain[br]and\n", []string{"00:00:01.000 --> 00:00:02.500", "Hello", "there", "00:00:03.000 --> 00:00:04.000", "Again\nand"}},
		{"lrc", "[ti:Song]\n[offset:500]\n[00:01.00]One\n[00:03.50][00:10.00]Two\n[00:05.00]\n[00:12.00]<00:12.00>Word <00:13.00>by word\n", []string{"00:00:00.500 --> 00:00:03.000", "One", "Two", "00:00:09.500 --> 00:00:11.500", "Word by word"}},
		{"ttml", `<?xml version="1.0"?><tt xmlns="http://www.w3.org/ns/ttml" xmlns:ttp="p" ttp:frameRate="25"><body><div><p begin="00:00:01.000" end="00:00:02:12">Hello<br/>there</p><p begin="3s" dur="1.5s"><span>Next</span> one</p><p begin="4000ms" end="5s">Ms</p></div></body></tt>`, []string{"00:00:01.000 --> 00:00:02.480", "Hello\nthere", "00:00:03.000 --> 00:00:04.500", "Next", "00:00:04.000 --> 00:00:05.000"}},
		{"dfxp", `<tt xmlns="http://www.w3.org/2006/04/ttaf1" ><body><div><p begin="00:00:01.00" end="00:00:02.00">A &amp; B</p></div></body></tt>`, []string{"A &amp; B"}},
		{"vtt", "WEBVTT\n\n00:01.000 --> 00:02.000\nHi\n", []string{"WEBVTT", "Hi"}},
		{"vtt", "00:01.000 --> 00:02.000\nNo header\n", []string{"WEBVTT", "No header"}},
	} {
		got, err := Convert(c.format, []byte(c.in))
		if err != nil {
			t.Errorf("%s: %v", c.format, err)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(string(got), w) {
				t.Errorf("%s: no %q in:\n%s", c.format, w, got)
			}
		}
	}
	if _, err := Convert("nonsense", []byte("x")); err == nil {
		t.Error("an unknown format should say so")
	}
}

func TestShift(t *testing.T) {
	in := "WEBVTT\n\n00:00:05.000 --> 00:00:06.000\nGone\n\n00:00:09.000 --> 00:00:11.000 align:left\nSpans\n\n00:00:20.000 --> 00:00:21.000\nLater\n"
	got := string(Shift([]byte(in), 10))
	if strings.Contains(got, "Gone") {
		t.Errorf("a cue that ended stays:\n%s", got)
	}
	for _, w := range []string{"WEBVTT", "00:00:00.000 --> 00:00:01.000 align:left", "Spans", "00:00:10.000 --> 00:00:11.000", "Later"} {
		if !strings.Contains(got, w) {
			t.Errorf("no %q in:\n%s", w, got)
		}
	}
	if string(Shift([]byte(in), 0)) != in {
		t.Error("no shift, no change")
	}
}

func FuzzConvert(f *testing.F) {
	f.Add("srt", "1\n00:00:01,000 --> 00:00:02,000\nHi\n")
	f.Add("ttml", `<tt><p begin="1s" end="2s">x</p></tt>`)
	f.Add("lrc", "[00:01.00][99:99.99]x")
	f.Add("sbv", "0:00:01.000,0:00:02.000\nx")
	f.Fuzz(func(t *testing.T, format, in string) {
		if len(in) > 1<<16 {
			return
		}
		start := time.Now()
		out, err := Convert(format, []byte(in))
		if err == nil {
			Shift(out, 1.5)
			if len(out) > 256*len(in)+1<<16 {
				t.Errorf("%d bytes from %d", len(out), len(in))
			}
		}
		if took := time.Since(start); took > 2*time.Second {
			t.Errorf("%d bytes took %v", len(in), took)
		}
	})
}
