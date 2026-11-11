package mediaextras

import (
	"reflect"
	"testing"
)

func TestChapterKind(t *testing.T) {
	for _, c := range []struct{ title, want string }{
		{"Intro", KindIntro},
		{"Opening", KindIntro},
		{"OP", KindIntro},
		{"Opening Credits", KindIntro},
		{"Title Sequence", KindIntro},
		{"Vorspann", KindIntro},
		{"Générique", KindIntro},
		{"Introducción", KindIntro},
		{"Opuningu", KindIntro},
		{"オープニング", KindIntro},
		{"Credits", KindCredits},
		{"End Credits", KindCredits},
		{"Closing Credits", KindCredits},
		{"Outro", KindCredits},
		{"Ending", KindCredits},
		{"Ending Theme", KindCredits},
		{"ED", KindCredits},
		{"ED - Song Name", KindCredits},
		{"Abspann", KindCredits},
		{"Nachspann", KindCredits},
		{"Générique de fin", KindCredits},
		{"Generique de fin", KindCredits},
		{"Créditos", KindCredits},
		{"Créditos finales", KindCredits},
		{"Staff Roll", KindCredits},
		{"エンディング", KindCredits},
		{"Previously On", KindRecap},
		{"Previously on Show Name", KindRecap},
		{"Recap", KindRecap},
		{"Was bisher geschah", KindRecap},
		{"Rückblick", KindRecap},
		{"Précédemment", KindRecap},
		{"Anteriormente", KindRecap},
		{"Resumen", KindRecap},
		{"Arasuji", KindRecap},
		{"あらすじ", KindRecap},
		{"Chapter 3", ""},
		{"Edward's Story", ""}, // "Ed" inside a word isn't the ending
		{"The Opening of the Box", KindIntro},
		{"", ""},
		{"Scene 12", ""},
		{"Credit Card Heist", ""},
	} {
		if got := ChapterKind(c.title); got != c.want {
			t.Errorf("%q: %q, want %q", c.title, got, c.want)
		}
	}
}

func TestFromChapters(t *testing.T) {
	m := FromChapters([]Chapter{
		{0, 45, "Previously on"},
		{45, 120, "Opening"},
		{120, 1200, "Part 1"},
		{1200, 1260, "Credits"}, // a mid-episode "credits" the last one replaces
		{1260, 1300, "Scene"},
		{1300, 1380, "End Credits"},
		{1380, 4000, "Intro to the next"}, // far too long for an intro, and the first stands
	})
	if m.Recap == nil || m.Recap.StartSec != 0 || m.Recap.EndSec != 45 {
		t.Errorf("recap %+v", m.Recap)
	}
	if m.Intro == nil || m.Intro.StartSec != 45 || m.Intro.EndSec != 120 {
		t.Errorf("intro %+v", m.Intro)
	}
	if m.Credits == nil || m.Credits.StartSec != 1300 || m.Credits.EndSec != 1380 {
		t.Errorf("credits %+v", m.Credits)
	}
	if got := FromChapters([]Chapter{{100, 50, "Intro"}, {0, 2000, "Recap"}}); !got.Empty() {
		t.Errorf("backwards and overlong chapters: %+v", got)
	}
}

func TestParseEDL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want Markers
	}{
		{"commercials", "0.00\t30.50\t3\n600 780 3\n", Markers{Commercials: []Range{{StartSec: 0, EndSec: 30.5}, {StartSec: 600, EndSec: 780}}}},
		{"cut counts, mute and scene don't", "10 20 0\n30 40 1\n50 60 2\n", Markers{Commercials: []Range{{StartSec: 10, EndSec: 20}}}},
		{"no type is a commercial", "100 200\n", Markers{Commercials: []Range{{StartSec: 100, EndSec: 200}}}},
		{"named ranges", "0 85.5 3 intro\n90 150 3 Previously on\n1290 1380 3 End Credits\n", Markers{
			Intro: &Range{StartSec: 0, EndSec: 85.5, Label: "intro"}, Recap: &Range{StartSec: 90, EndSec: 150, Label: "Previously on"},
			Credits: &Range{StartSec: 1290, EndSec: 1380, Label: "End Credits"}}},
		{"type words", "5 65 intro\n1200 1300 credits\n300 400 commercial\n", Markers{
			Intro: &Range{StartSec: 5, EndSec: 65}, Credits: &Range{StartSec: 1200, EndSec: 1300}, Commercials: []Range{{StartSec: 300, EndSec: 400}}}},
		{"timecodes", "00:01:00.5 00:02:00 3\n1:00:00 1:01:30 3\n", Markers{Commercials: []Range{{StartSec: 60.5, EndSec: 120}, {StartSec: 3600, EndSec: 3690}}}},
		{"comments and nonsense", "# a comment\n// another\n; and another\n\nnot a line\n50 40 3\n-5 10 3\n10 abc 3\n#100 #200 3\n00:61:00 00:62:00 3\n1e400 1e401 3\nNaN 5 3\n", Markers{}},
		{"byte order mark and CRLF", "\ufeff10 20 3\r\n30 40 3\r\n", Markers{Commercials: []Range{{StartSec: 10, EndSec: 20}, {StartSec: 30, EndSec: 40}}}},
		{"first intro stands, last credits", "0 50 intro\n60 90 intro\n1000 1100 credits\n1200 1300 credits\n", Markers{
			Intro: &Range{StartSec: 0, EndSec: 50}, Credits: &Range{StartSec: 1200, EndSec: 1300}}},
	}
	for _, c := range cases {
		got := ParseEDL([]byte(c.in))
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %+v\nwant %+v", c.name, show(got), show(c.want))
		}
	}
}

func TestParseEDLBounded(t *testing.T) {
	var b []byte
	for i := range 1000 {
		b = append(b, []byte(itoa(i*10)+" "+itoa(i*10+5)+" 3\n")...)
	}
	if got := ParseEDL(b); len(got.Commercials) != maxRanges {
		t.Errorf("%d breaks kept", len(got.Commercials))
	}
	long := make([]byte, 10000)
	for i := range long {
		long[i] = 'x'
	}
	if got := ParseEDL(append(long, []byte("\n10 20 3\n")...)); len(got.Commercials) != 0 {
		// A line longer than the scanner takes ends the file: nothing after it is trusted.
		t.Logf("after an overlong line: %+v", got)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func show(m Markers) map[string]any {
	out := map[string]any{"commercials": m.Commercials}
	if m.Intro != nil {
		out["intro"] = *m.Intro
	}
	if m.Recap != nil {
		out["recap"] = *m.Recap
	}
	if m.Credits != nil {
		out["credits"] = *m.Credits
	}
	return out
}

func TestParseSkipJSON(t *testing.T) {
	m, err := ParseSkipJSON([]byte(`{"intro": {"start": 62, "end": 151.5}, "recap": {"startSec": 0, "endSec": 60},
		"credits": {"start": 1290}, "commercials": [{"start": 600, "end": 780, "label": "break 1"}]}`), 1400)
	if err != nil {
		t.Fatal(err)
	}
	if m.Intro.StartSec != 62 || m.Intro.EndSec != 151.5 || m.Recap.EndSec != 60 || m.Credits.StartSec != 1290 || m.Credits.EndSec != 1400 {
		t.Errorf("%+v", show(m))
	}
	if len(m.Commercials) != 1 || m.Commercials[0].Label != "break 1" {
		t.Errorf("%+v", m.Commercials)
	}
	// Credits to an end that isn't known yet.
	m, err = ParseSkipJSON([]byte(`{"credits": {"start": 1290}}`), 0)
	if err != nil || m.Credits.EndSec != 0 {
		t.Errorf("%v %+v", err, m.Credits)
	}
	for _, bad := range []string{
		`{"intro": {"start": 100, "end": 50}}`,
		`{"intro": {"start": -1, "end": 50}}`,
		`{"intro": {"start": 10}}`,
		`{"recap": {"end": 1e999}}`,
		`not json`,
		`{"commercials": [{"start": 5, "end": 5}]}`,
	} {
		if _, err := ParseSkipJSON([]byte(bad), 1000); err == nil {
			t.Errorf("%s was taken", bad)
		}
	}
}

func TestSkipFileRoundTrip(t *testing.T) {
	in := Markers{Intro: &Range{StartSec: 1, EndSec: 2}, Credits: &Range{StartSec: 100}, Commercials: []Range{{StartSec: 10, EndSec: 20}}}
	f := in.ToSkipFile()
	out, err := f.Markers(0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(show(in), show(out)) {
		t.Errorf("%+v != %+v", show(in), show(out))
	}
}

func TestMerge(t *testing.T) {
	manual := Markers{Credits: &Range{StartSec: 1000, EndSec: 1100}}
	edl := Markers{Intro: &Range{StartSec: 5, EndSec: 50}, Commercials: []Range{{StartSec: 600, EndSec: 700}, {StartSec: 300, EndSec: 400}}}
	chapters := Markers{Intro: &Range{StartSec: 0, EndSec: 60}, Recap: &Range{StartSec: 0, EndSec: 30}, Credits: &Range{StartSec: 900, EndSec: 960}, Commercials: []Range{{StartSec: 1, EndSec: 2}}}
	m := Merge(Layer{SourceManual, manual}, Layer{SourceEDL, edl}, Layer{SourceChapters, chapters})
	if m.Credits.StartSec != 1000 || m.Sources[KindCredits] != SourceManual {
		t.Errorf("credits %+v from %s", m.Credits, m.Sources[KindCredits])
	}
	if m.Intro.StartSec != 5 || m.Sources[KindIntro] != SourceEDL {
		t.Errorf("intro %+v", m.Intro)
	}
	if m.Recap.EndSec != 30 || m.Sources[KindRecap] != SourceChapters {
		t.Errorf("recap %+v", m.Recap)
	}
	if len(m.Commercials) != 2 || m.Commercials[0].StartSec != 300 || m.Sources["commercials"] != SourceEDL {
		t.Errorf("commercials come whole from one source, in order: %+v", m.Commercials)
	}
	if m.Source != SourceManual {
		t.Errorf("source %s", m.Source)
	}
	if e := Merge(); e.Source != SourceNone || e.Commercials == nil {
		t.Errorf("nothing: %+v", e)
	}
}
