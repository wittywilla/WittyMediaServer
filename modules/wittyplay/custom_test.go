package wittyplay

import (
	"fmt"
	"strings"
	"testing"
)

var testOffered = []OptionInfo{
	{Key: "fceumm_sndquality", Label: "Sound quality", Choices: []string{"Low", "High", "Very High"}, Default: "Low"},
	{Key: "fceumm_nospritelimit", Label: "No sprite limit", Choices: []string{"disabled", "enabled"}, Default: "disabled"},
}

func TestACompleteFileIsRead(t *testing.T) {
	l, rep := ParseCustom([]byte(`{
  "wittyplay": 1,
  "name": "My NES",
  "settings": {"display.scanlines": 0.4, "display.rotate": 90, "osd.on": true},
  "coreOptions": {"fceumm_sndquality": "High", "fceumm_nospritelimit": true},
  "controls": [
    {"label": "Sound", "type": "choice", "option": "fceumm_sndquality", "choices": ["Low", {"value": "High", "label": "Hi-fi"}]},
    {"label": "Sprites", "type": "toggle", "option": "fceumm_nospritelimit"},
    {"label": "Volume", "type": "slider", "option": "fceumm_sndquality", "min": 0, "max": 10, "step": 2}
  ],
  "presets": [{"name": "Crisp", "settings": {"display.scaling": "pixel"}, "coreOptions": {"fceumm_sndquality": "Low"}}]
}`), CustomInput{Offered: testOffered})
	if !rep.OK() {
		t.Fatalf("errors: %+v", rep.Errors)
	}
	if len(rep.Ignored) != 0 {
		t.Fatalf("ignored: %+v", rep.Ignored)
	}
	if l.Values["display.scanlines"] != 0.4 || l.Values["display.rotate"] != "90" || l.Values["osd.on"] != true {
		t.Errorf("values: %v", l.Values)
	}
	if l.CoreOptions["fceumm_nospritelimit"] != "enabled" {
		t.Errorf("true should read as enabled: %v", l.CoreOptions)
	}
	if len(l.Controls) != 3 || l.Controls[0].Choices[1].Label != "Hi-fi" || l.Controls[1].On != "enabled" || len(l.Controls[2].Choices) != 6 || l.Controls[2].Choices[5].Value != "10" {
		t.Errorf("controls: %+v", l.Controls)
	}
	if len(l.Presets) != 1 || l.Presets[0].Values["display.scaling"] != "pixel" {
		t.Errorf("presets: %+v", l.Presets)
	}
}

// Going through the file and back gives the same thing, so the editor can show a level
// and save it unchanged.
func TestALayerSurvivesBecomingAFile(t *testing.T) {
	src := `{"wittyplay":1,"settings":{"display.curve":0.25},"coreOptions":{"a":"b"},"controls":[{"label":"X","type":"toggle","option":"a","on":"b","off":"c"}],"presets":[{"name":"P","settings":{"osd.on":true}}]}`
	l, rep := ParseCustom([]byte(src), CustomInput{})
	if !rep.OK() {
		t.Fatal(rep.Errors)
	}
	again, rep := ParseCustom(l.Document(), CustomInput{})
	if !rep.OK() {
		t.Fatalf("its own document was refused: %+v\n%s", rep.Errors, l.Document())
	}
	if len(rep.Ignored) != 0 {
		t.Fatalf("parts of its own document were ignored: %+v\n%s", rep.Ignored, l.Document())
	}
	if again.Values["display.curve"] != 0.25 || again.CoreOptions["a"] != "b" || again.Controls[0].Off != "c" || again.Presets[0].Name != "P" {
		t.Fatalf("got %+v", again)
	}
}

func issueAt(list []Issue, line int) *Issue {
	for i := range list {
		if list[i].Line == line {
			return &list[i]
		}
	}
	return nil
}

func TestMistakesAreShownAtTheirLine(t *testing.T) {
	cases := []struct {
		name, src string
		line      int
		says      string
	}{
		{"out of range", "{\n  \"settings\": {\n    \"display.scanlines\": 5\n  }\n}", 3, "from 0 to 1"},
		{"wrong type", "{\n  \"settings\": {\n    \"osd.on\": \"yes\"\n  }\n}", 3, "true or false"},
		{"not a choice", "{\n\n\n  \"settings\": {\"display.scaling\": \"wobbly\"}\n}", 4, "must be one of"},
		{"bad option name", "{\"coreOptions\": {\n \"../../etc/passwd\": \"x\"\n}}", 2, "letters, numbers"},
		{"control missing type", "{\"controls\": [\n  {\"label\": \"X\", \"option\": \"a\"}\n]}", 2, "type"},
		{"control bad type", "{\"controls\": [\n {\"label\":\"X\",\"option\":\"a\",\n \"type\":\"script\"}\n]}", 3, "toggle"},
		{"duplicate key", "{\n  \"settings\": {},\n  \"settings\": {}\n}", 3, "twice"},
		{"bad version", "{\n \"wittyplay\": 2\n}", 2, "version"},
		{"syntax", "{\n  \"settings\": {\n    \"osd.on\": tru\n  }\n}", 3, "valid JSON"},
		{"missing comma", "{\n  \"a\": 1\n  \"b\": 2\n}", 3, "valid JSON"},
		{"trailing comma", "{\n  \"settings\": {},\n}", 2, "valid JSON"},
		{"not closed", "{\n  \"settings\": {\n", 2, "ends before"},
		{"after the end", "{}\n{}", 2, "after the end"},
		{"not an object", "[1]", 1, "object"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, rep := ParseCustom([]byte(c.src), CustomInput{})
			if rep.OK() {
				t.Fatal("no error")
			}
			got := issueAt(rep.Errors, c.line)
			if got == nil {
				t.Fatalf("nothing at line %d: %+v", c.line, rep.Errors)
			}
			if !strings.Contains(got.Message, c.says) {
				t.Fatalf("message %q doesn't say %q", got.Message, c.says)
			}
		})
	}
}

// Unknown things are listed, never silently dropped, and never errors.
func TestUnknownThingsAreReportedAsIgnored(t *testing.T) {
	src := "{\n  \"settings\": {\n    \"display.nope\": 1\n  },\n  \"exec\": \"rm -rf /\",\n  \"coreOptions\": {\n    \"made_up\": \"1\",\n    \"fceumm_sndquality\": \"Ultra\"\n  },\n  \"controls\": [\n    {\"label\":\"X\",\"type\":\"toggle\",\"option\":\"no_such\",\"run\":\"x\"}\n  ]\n}"
	l, rep := ParseCustom([]byte(src), CustomInput{Offered: testOffered})
	if !rep.OK() {
		t.Fatalf("errors: %+v", rep.Errors)
	}
	for _, line := range []int{3, 5, 7, 8, 11} {
		if issueAt(rep.Ignored, line) == nil {
			t.Errorf("nothing reported at line %d: %+v", line, rep.Ignored)
		}
	}
	if len(l.Values) != 0 {
		t.Errorf("an unknown setting came through: %v", l.Values)
	}
}

// Hostile files: none of these may hurt, and none may get anything through.
func TestHostileFiles(t *testing.T) {
	deep := strings.Repeat(`{"a":`, 5000) + "1" + strings.Repeat("}", 5000)
	wide := "{\"settings\":{" + func() string {
		var b strings.Builder
		for i := 0; i < MaxCustomNodes+10; i++ {
			fmt.Fprintf(&b, "%q:1,", fmt.Sprint("k", i))
		}
		return b.String()
	}() + `"z":1}}`
	cases := map[string]string{
		"deep":        deep,
		"deep arrays": strings.Repeat("[", 100000),
		"wide":        wide,
		"long key":    `{"` + strings.Repeat("a", MaxCustomText+1) + `":1}`,
		"long string": `{"name":"` + strings.Repeat("a", MaxCustomText+1) + `"}`,
		"huge number": `{"settings":{"display.curve":1e99999}}`,
		"too large":   `{"name":"` + strings.Repeat("a", MaxCustomSize) + `"}`,
		"bad utf8":    "{\"wittyplay\xff\": 1,",
		"nul":         "{\"a\":\"\x00\"}",
		"empty":       "   ",
		"controls": `{"controls":[` + func() string {
			var b strings.Builder
			for i := 0; i <= MaxCustomControls; i++ {
				b.WriteString(`{"label":"x","type":"toggle","option":"a"},`)
			}
			return b.String()
		}() + `{"label":"x","type":"toggle","option":"a"}]}`,
		"slider bomb":        `{"controls":[{"label":"x","type":"slider","option":"a","min":0,"max":1e15,"step":0.001}]}`,
		"zero step":          `{"controls":[{"label":"x","type":"slider","option":"a","min":0,"max":10,"step":0}]}`,
		"negative range":     `{"controls":[{"label":"x","type":"slider","option":"a","min":10,"max":0}]}`,
		"path in option":     `{"controls":[{"label":"x","type":"toggle","option":"C:\\Windows\\system32"}]}`,
		"control character":  `{"controls":[{"label":"a\u0007b","type":"toggle","option":"a"}]}`,
		"value with newline": `{"coreOptions":{"a":"b\nc"}}`,
		"long value":         `{"coreOptions":{"a":"` + strings.Repeat("v", MaxOptionValue+1) + `"}}`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			l, rep := ParseCustom([]byte(src), CustomInput{Offered: testOffered})
			if rep.OK() {
				t.Fatalf("accepted: %+v", l)
			}
			if l != nil {
				t.Fatalf("errors, yet a layer came back: %+v", l)
			}
			if len(rep.Errors) == 0 || rep.Errors[0].Message == "" {
				t.Fatal("no message")
			}
		})
	}
}

// Whatever's in a file, only known settings and well-formed option names come out.
func TestNothingButKnownThingsComeOut(t *testing.T) {
	l, rep := ParseCustom([]byte(`{"settings":{"osd.on":true,"__proto__":1,"constructor":{"prototype":{}}},"coreOptions":{"ok":"1"},"script":"alert(1)","file":"/etc/passwd","url":"http://x"}`), CustomInput{})
	if !rep.OK() {
		t.Fatalf("errors: %+v", rep.Errors)
	}
	if len(l.Values) != 1 || l.Values["osd.on"] != true || len(l.CoreOptions) != 1 {
		t.Fatalf("got %+v", l)
	}
	if len(rep.Ignored) != 5 {
		t.Fatalf("expected five things reported as ignored: %+v", rep.Ignored)
	}
}

func TestPresetsAreLimitedAndNamed(t *testing.T) {
	_, rep := ParseCustom([]byte(`{"presets":[{"name":"A"},{"name":"A"}]}`), CustomInput{})
	if rep.OK() {
		t.Fatal("two presets of one name were accepted")
	}
	_, rep = ParseCustom([]byte(`{"presets":[{"settings":{}}]}`), CustomInput{})
	if rep.OK() {
		t.Fatal("a preset with no name was accepted")
	}
	var b strings.Builder
	b.WriteString(`{"presets":[`)
	for i := 0; i <= MaxPresets; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"name":"p%d"}`, i)
	}
	b.WriteString(`]}`)
	_, rep = ParseCustom([]byte(b.String()), CustomInput{})
	if rep.OK() {
		t.Fatal("too many presets were accepted")
	}
}

func FuzzParseCustom(f *testing.F) {
	f.Add([]byte(`{"wittyplay":1,"settings":{"osd.on":true},"controls":[{"label":"a","type":"slider","option":"b","min":1,"max":2}]}`))
	f.Add([]byte(`{"presets":[{"name":"a","coreOptions":{"b":1}}]}`))
	f.Add([]byte("{\n\"a\": [1,2,{\"b\":null}]\n}"))
	f.Fuzz(func(t *testing.T, b []byte) {
		l, rep := ParseCustom(b, CustomInput{Offered: testOffered})
		if rep.OK() == (l == nil) {
			t.Fatalf("ok %v with layer %v", rep.OK(), l)
		}
		if l != nil {
			if _, rep2 := ParseCustom(l.Document(), CustomInput{}); !rep2.OK() {
				t.Fatalf("its own document was refused: %+v\n%s", rep2.Errors, l.Document())
			}
		}
	})
}
