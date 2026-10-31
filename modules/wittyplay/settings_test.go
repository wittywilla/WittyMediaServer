package wittyplay

import (
	"encoding/json"
	"strings"
	"testing"
)

func layerOf(values map[string]any, core map[string]string) *Layer {
	return &Layer{Values: values, CoreOptions: core}
}

func TestEverySettingIsSound(t *testing.T) {
	seen := map[string]bool{}
	for i := range Settings {
		s := &Settings[i]
		if seen[s.Key] {
			t.Errorf("%s is listed twice", s.Key)
		}
		seen[s.Key] = true
		if s.Label == "" || s.Group == "" || s.Kind == "" {
			t.Errorf("%s has no label, group or kind", s.Key)
		}
		if _, err := CleanValue(s, s.Default); err != nil {
			t.Errorf("%s: its own default is refused: %v", s.Key, err)
		}
		if s.Motion && s.Caution == "" {
			t.Errorf("%s animates, so it needs a caution", s.Key)
		}
	}
	// These could hurt someone or surprise them, so they start off.
	for _, off := range []string{"display.flicker", "osd.on", "emulation.rewind"} {
		if SettingFor(off).Default == true || SettingFor(off).Default == 1.0 {
			t.Errorf("%s must start off", off)
		}
	}
	for _, p := range BuiltinPresets {
		for k, v := range p.Values {
			s := SettingFor(k)
			if s == nil {
				t.Errorf("preset %s sets unknown %s", p.Name, k)
				continue
			}
			if _, err := CleanValue(s, v); err != nil {
				t.Errorf("preset %s: %s: %v", p.Name, k, err)
			}
		}
	}
}

func TestTheMostSpecificLevelWins(t *testing.T) {
	r := Resolve([]NamedLayer{
		{LevelServer, layerOf(map[string]any{"display.scanlines": 0.1, "display.curve": 0.1, "audio.volume": 0.1, "osd.on": true}, map[string]string{"opt": "server", "other": "server"})},
		{LevelServerConsole, layerOf(map[string]any{"display.scanlines": 0.2, "display.curve": 0.2, "audio.volume": 0.2}, nil)},
		{LevelPerson, layerOf(map[string]any{"display.scanlines": 0.3, "display.curve": 0.3}, map[string]string{"opt": "person"})},
		{LevelConsole, layerOf(map[string]any{"display.scanlines": 0.4}, nil)},
		{LevelGame, layerOf(nil, map[string]string{"opt": "game"})},
	})
	check := func(key string, want any, source string) {
		t.Helper()
		if r.Values[key] != want || r.Sources[key] != source {
			t.Errorf("%s = %v from %s, want %v from %s", key, r.Values[key], r.Sources[key], want, source)
		}
	}
	check("display.scanlines", 0.4, LevelConsole)
	check("display.curve", 0.3, LevelPerson)
	check("audio.volume", 0.2, LevelServerConsole)
	check("osd.on", true, LevelServer)
	check("display.mask", 0.0, LevelBuiltin)
	if r.CoreOptions["opt"] != "game" || r.CoreSources["opt"] != LevelGame {
		t.Errorf("core option: %v %v", r.CoreOptions["opt"], r.CoreSources["opt"])
	}
	if r.CoreOptions["other"] != "server" {
		t.Errorf("a core option from a broader level was lost: %v", r.CoreOptions)
	}
}

// What's stored may be older than today's rules. Bad values fall back; a game still starts.
func TestStaleValuesAreIgnoredWhenResolved(t *testing.T) {
	r := Resolve([]NamedLayer{{LevelGame, layerOf(map[string]any{"display.scanlines": 99.0, "display.scaling": "fancy", "gone.setting": 1.0, "osd.on": "yes"}, map[string]string{"../evil": "x", "ok": strings.Repeat("v", 1000)})}})
	if r.Values["display.scanlines"] != 0.0 || r.Values["display.scaling"] != "sharp" || r.Values["osd.on"] != false {
		t.Fatalf("bad values got through: %v %v %v", r.Values["display.scanlines"], r.Values["display.scaling"], r.Values["osd.on"])
	}
	if _, ok := r.Values["gone.setting"]; ok {
		t.Fatal("an unknown setting got through")
	}
	if len(r.CoreOptions) != 0 {
		t.Fatalf("bad core options got through: %v", r.CoreOptions)
	}
}

func TestPatchChangesOnlyWhatsGood(t *testing.T) {
	l := &Layer{Values: map[string]any{"display.curve": 0.5, "display.mask": 0.5}, CoreOptions: map[string]string{"a": "1", "b": "2"}}
	two := "3"
	bad := "x\x00y"
	ignored := l.Patch(map[string]any{"display.curve": nil, "display.glow": 0.2, "display.vignette": 7.0, "nope": 1.0}, map[string]*string{"a": nil, "c": &two, "../d": &two, "e": &bad})
	if _, ok := l.Values["display.curve"]; ok {
		t.Error("null didn't remove a value")
	}
	if l.Values["display.mask"] != 0.5 || l.Values["display.glow"] != 0.2 {
		t.Errorf("values: %v", l.Values)
	}
	if _, ok := l.Values["display.vignette"]; ok {
		t.Error("an out of range value was kept")
	}
	if _, ok := l.CoreOptions["a"]; ok || l.CoreOptions["b"] != "2" || l.CoreOptions["c"] != "3" {
		t.Errorf("core: %v", l.CoreOptions)
	}
	if len(ignored) != 4 {
		t.Errorf("reported %d as ignored: %+v", len(ignored), ignored)
	}
}

func TestControlIsHeldToTheServersLimits(t *testing.T) {
	values := map[string]any{"emulation.frameSkip": "auto", "emulation.runAhead": 3.0, "emulation.rewind": true, "emulation.rewindSeconds": 60.0}
	c := ControlFor(values, Limits{MaxSpeed: 2, RewindBytes: 1 << 20, RunAhead: false})
	if *c.RunAhead != 0 {
		t.Errorf("run-ahead %d on a server that doesn't allow it", *c.RunAhead)
	}
	if *c.RewindBytes != 1<<20 {
		t.Errorf("rewind memory %d", *c.RewindBytes)
	}
	if *c.FrameSkip != -1 {
		t.Errorf("frame skip %d", *c.FrameSkip)
	}
	c = ControlFor(map[string]any{"emulation.frameSkip": "2", "emulation.rewind": false}, Limits{RewindBytes: 1 << 20, RunAhead: true})
	if *c.FrameSkip != 2 || *c.RewindBytes != 0 {
		t.Errorf("frame skip %d, rewind %d", *c.FrameSkip, *c.RewindBytes)
	}
}

func TestCheatsAreCheckedAndFilesRead(t *testing.T) {
	if _, err := CleanCheats([]Cheat{{Label: "ok", Code: "APEZPA+1234:56"}}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "<script>", "../../x", "a\nb", "`rm -rf`", strings.Repeat("a", MaxCheatCode+1)} {
		if _, err := CleanCheats([]Cheat{{Code: bad}}); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if _, err := CleanCheats(make([]Cheat, MaxCheats+1)); err == nil {
		t.Error("accepted too many")
	}
	file := []byte("cheats = 3\r\ncheat0_desc = \"Lives\"\r\ncheat0_code = \"AAAA+BBBB\"\r\ncheat0_enable = true\r\ncheat1_desc = \"Bad\"\r\ncheat1_code = \"<nope>\"\r\ncheat2_code = \"CCCC\"\r\nsomething else\r\n")
	list, skipped, err := ParseCheatFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Label != "Lives" || !list[0].Enabled || list[1].Code != "CCCC" || skipped != 1 {
		t.Fatalf("got %+v, %d skipped", list, skipped)
	}
	if _, _, err := ParseCheatFile([]byte("not a cheat file")); err == nil {
		t.Error("a file with no cheats wasn't refused")
	}
	if _, _, err := ParseCheatFile(make([]byte, MaxCheatFile+1)); err == nil {
		t.Error("an oversized file wasn't refused")
	}
	var bomb strings.Builder
	for i := 0; i < 5000; i++ {
		bomb.WriteString("cheat99999_code=AAAA\ncheat")
		bomb.WriteString(strings.Repeat("1", 5))
		bomb.WriteString("_code=BBBB\n")
	}
	if list, _, err := ParseCheatFile([]byte(bomb.String())); err == nil && len(list) > MaxCheats {
		t.Errorf("kept %d cheats", len(list))
	}
}

func TestSettingsSurviveAStorageRoundTrip(t *testing.T) {
	l := &Layer{Values: map[string]any{"display.scanlines": 0.5, "osd.on": true, "display.scaling": "pixel"}, CoreOptions: map[string]string{"a": "b"}}
	b, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	var back Layer
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	r := Resolve([]NamedLayer{{LevelGame, &back}})
	if r.Values["display.scanlines"] != 0.5 || r.Values["osd.on"] != true || r.Values["display.scaling"] != "pixel" || r.CoreOptions["a"] != "b" {
		t.Fatalf("got %+v", r)
	}
}
