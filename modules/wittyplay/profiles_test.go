package wittyplay

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func anyConsole(string) bool { return true }

func TestResolveOrder(t *testing.T) {
	b, err := os.ReadFile("testdata/resolve.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Profiles []Profile `json:"profiles"`
		Cases    []struct {
			Why        string `json:"why"`
			Kind       string `json:"kind"`
			Controller string `json:"controller"`
			Console    string `json:"console"`
			Game       int64  `json:"game"`
			Raw        bool   `json:"raw"`
			Want       int64  `json:"want"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.Cases {
		got := ResolveProfile(f.Profiles, c.Kind, c.Controller, c.Console, c.Game, c.Raw)
		var id int64
		if got != nil {
			id = got.ID
		}
		if id != c.Want {
			t.Errorf("%s: got profile %d, want %d", c.Why, id, c.Want)
		}
	}
}

func TestEffectiveMap(t *testing.T) {
	five, one := 5, 1
	p := &Profile{Kind: ProfileGamepad, Map: map[int]*Binding{
		RetroB:      {Button: &five},
		RetroSelect: nil, // nothing presses it
		StickUpL:    {Axis: &one, Dir: -1},
		StickUpR:    {Button: &five}, // not on this console
	}}
	m := EffectiveMap("snes", p)
	if b := m[RetroB]; b.Button == nil || *b.Button != 5 {
		t.Errorf("the change is missing: %+v", b)
	}
	if _, ok := m[RetroSelect]; ok {
		t.Error("a cleared button should be gone")
	}
	if b := m[RetroA]; b.Button == nil || *b.Button != 1 {
		t.Errorf("what isn't changed should stay at its default: %+v", b)
	}
	if _, ok := m[StickUpR]; ok {
		t.Error("a console with no stick got one")
	}
	if len(EffectiveMap("snes", &Profile{Raw: true, Map: map[int]*Binding{RetroB: {Button: &five}}})) != 1 {
		t.Error("a profile of raw numbers mustn't mix with standard defaults")
	}
}

func TestEffectiveKeys(t *testing.T) {
	k := EffectiveKeys("snes", &Profile{Keys: map[int]string{RetroB: "KeyX", StickUpR: "KeyQ"}})
	if k[RetroB] != "KeyX" {
		t.Errorf("%v", k)
	}
	if _, ok := k[RetroA]; ok {
		t.Error("KeyX was A by default and now means B; it should do just one thing")
	}
	if _, ok := k[StickUpR]; ok {
		t.Error("a console with no stick got a key for one")
	}
}

func TestConsoleDefaults(t *testing.T) {
	// Every console's map uses things that exist, and passes our own check.
	for _, c := range Consoles {
		targets := map[int]bool{}
		for _, tg := range Targets(c.ID) {
			if tg.ID < 0 || tg.ID >= targetCount {
				t.Errorf("%s: bad target %+v", c.ID, tg)
			}
			if targets[tg.ID] {
				t.Errorf("%s: target %d twice", c.ID, tg.ID)
			}
			targets[tg.ID] = true
		}
		for id := range DefaultGamepadFor(c.ID) {
			if !targets[id] {
				t.Errorf("%s: default map has %d, which isn't there", c.ID, id)
			}
		}
		keys := map[string]int{}
		for id, k := range DefaultKeyboardFor(c.ID) {
			if !targets[id] {
				t.Errorf("%s: default keys have %d, which isn't there", c.ID, id)
			}
			if other, dup := keys[k]; dup {
				t.Errorf("%s: %s is used for both %d and %d", c.ID, k, id, other)
			}
			keys[k] = id
		}
		for _, tg := range Targets(c.ID) {
			if _, ok := DefaultGamepadFor(c.ID)[tg.ID]; !ok {
				t.Errorf("%s: nothing presses %s by default", c.ID, tg.Label)
			}
		}
		b, _ := json.Marshal(DefaultGamepadFor(c.ID))
		if _, err := CleanControl(c.ID, KindGamepad, b); err != nil {
			t.Errorf("%s: %v", c.ID, err)
		}
	}

	// Stick consoles don't steer their D-pad with it; others do.
	if *DefaultOptions("ps1").StickDpad || !*DefaultOptions("nes").StickDpad {
		t.Error("the stick should only steer the D-pad where there's no stick to use")
	}
	// Two buttons on a two-button console, with the names they have.
	if f := Pad("nes").Face; len(f) != 2 || f[0].ID != RetroB || f[1].ID != RetroA {
		t.Errorf("NES: %+v", f)
	}
	// The N64's C buttons are the right stick, and its stick comes first.
	n := Pad("n64")
	if !n.StickFirst || len(n.Right) != 4 || n.Right[0].Label != "C left" {
		t.Errorf("N64: %+v", n)
	}
	if k := DefaultKeyboardFor("n64"); k[StickUpL] != "ArrowUp" || k[RetroUp] == "ArrowUp" {
		t.Errorf("N64 arrows should steer the stick: %v", k)
	}
	if !strings.Contains(strings.Join(Pad("ps1").PreferDevices, " "), "dualshock") {
		t.Error("PlayStation should ask for a DualShock")
	}
}

func TestCleanProfile(t *testing.T) {
	five := 5
	good := []Profile{
		{Name: "Pad", Kind: ProfileGamepad, Map: map[int]*Binding{RetroB: {Button: &five}, RetroA: nil}},
		{Name: "On a model", Kind: ProfileGamepad, Controller: "054c:0ce6", Console: "snes"},
		{Name: "By name", Kind: ProfileGamepad, Controller: "id:USB Gamepad", Raw: true, Map: map[int]*Binding{StickUpL: {Hat: &five, Dir: 1}}},
		{Name: "Trigger", Kind: ProfileGamepad, Map: map[int]*Binding{RetroL2: {Axis: &five, Dir: 1, Rest: -1}}},
		{Name: "One game", Kind: ProfileGamepad, Console: "snes", Game: 4},
		{Name: "Keys", Kind: ProfileKeyboard, Keys: map[int]string{RetroB: "KeyK"}},
		{Name: "Everything", Kind: ProfileGamepad, Options: ProfileOptions{Deadzone: 0.2, Threshold: 0.4, TurboHz: 20, Turbo: []int{RetroB, RetroB}, Calibration: map[int][3]float64{0: {-0.9, 0.01, 0.95}}}},
	}
	for _, p := range good {
		if _, err := CleanProfile(p, anyConsole); err != nil {
			t.Errorf("%s: %v", p.Name, err)
		}
	}
	long := strings.Repeat("x", 61)
	nine := 999
	bad := []struct {
		why string
		p   Profile
	}{
		{"no name", Profile{Kind: ProfileGamepad}},
		{"only invisible name", Profile{Name: " ​\u0007 ", Kind: ProfileGamepad}},
		{"long name", Profile{Name: long, Kind: ProfileGamepad}},
		{"unknown kind", Profile{Name: "x", Kind: "wheel"}},
		{"keys on a gamepad", Profile{Name: "x", Kind: ProfileGamepad, Keys: map[int]string{0: "KeyA"}}},
		{"buttons on a keyboard", Profile{Name: "x", Kind: ProfileKeyboard, Map: map[int]*Binding{0: {Button: &five}}}},
		{"keyboard with a pad's model", Profile{Name: "x", Kind: ProfileKeyboard, Controller: "054c:0ce6"}},
		{"odd model", Profile{Name: "x", Kind: ProfileGamepad, Controller: "../../etc"}},
		{"model with capitals", Profile{Name: "x", Kind: ProfileGamepad, Controller: "054C:0CE6"}},
		{"control character in a model", Profile{Name: "x", Kind: ProfileGamepad, Controller: "id:a\x07b"}},
		{"long model", Profile{Name: "x", Kind: ProfileGamepad, Controller: "id:" + strings.Repeat("y", 121)}},
		{"raw with no model", Profile{Name: "x", Kind: ProfileGamepad, Raw: true}},
		{"a target that isn't", Profile{Name: "x", Kind: ProfileGamepad, Map: map[int]*Binding{99: {Button: &five}}}},
		{"a negative target", Profile{Name: "x", Kind: ProfileGamepad, Map: map[int]*Binding{-1: {Button: &five}}}},
		{"a huge button", Profile{Name: "x", Kind: ProfileGamepad, Map: map[int]*Binding{0: {Button: &nine}}}},
		{"both button and axis", Profile{Name: "x", Kind: ProfileGamepad, Map: map[int]*Binding{0: {Button: &five, Axis: &five, Dir: 1}}}},
		{"empty binding", Profile{Name: "x", Kind: ProfileGamepad, Map: map[int]*Binding{0: {}}}},
		{"axis with no direction", Profile{Name: "x", Kind: ProfileGamepad, Map: map[int]*Binding{0: {Axis: &five}}}},
		{"hat with a bad direction", Profile{Name: "x", Kind: ProfileGamepad, Map: map[int]*Binding{0: {Hat: &five, Dir: 9}}}},
		{"rest out of range", Profile{Name: "x", Kind: ProfileGamepad, Map: map[int]*Binding{0: {Axis: &five, Dir: 1, Rest: 7}}}},
		{"key with control characters", Profile{Name: "x", Kind: ProfileKeyboard, Keys: map[int]string{0: "a\x07"}}},
		{"game with no console", Profile{Name: "x", Kind: ProfileGamepad, Game: 3}},
		{"negative game", Profile{Name: "x", Kind: ProfileGamepad, Console: "snes", Game: -3}},
		{"huge dead zone", Profile{Name: "x", Kind: ProfileGamepad, Options: ProfileOptions{Deadzone: 0.99}}},
		{"negative dead zone", Profile{Name: "x", Kind: ProfileGamepad, Options: ProfileOptions{Deadzone: -1}}},
		{"silly turbo", Profile{Name: "x", Kind: ProfileGamepad, Options: ProfileOptions{TurboHz: 5000}}},
		{"turbo on a stick", Profile{Name: "x", Kind: ProfileGamepad, Options: ProfileOptions{Turbo: []int{StickUpL}}}},
		{"calibration backwards", Profile{Name: "x", Kind: ProfileGamepad, Options: ProfileOptions{Calibration: map[int][3]float64{0: {1, 0, -1}}}}},
		{"calibration out of range", Profile{Name: "x", Kind: ProfileGamepad, Options: ProfileOptions{Calibration: map[int][3]float64{0: {-9, 0, 9}}}}},
		{"calibrating no axis", Profile{Name: "x", Kind: ProfileGamepad, Options: ProfileOptions{Calibration: map[int][3]float64{999: {-1, 0, 1}}}}},
	}
	for _, c := range bad {
		if _, err := CleanProfile(c.p, anyConsole); err == nil {
			t.Errorf("%s should be refused", c.why)
		}
	}
	if _, err := CleanProfile(Profile{Name: "x", Kind: ProfileGamepad, Console: "nosuch"}, func(string) bool { return false }); err == nil {
		t.Error("an unknown console should be refused")
	}
	// What can't be seen, or doesn't belong, doesn't survive.
	var raw Profile
	if err := json.Unmarshal([]byte(`{"name":" Mine‮ ","kind":"gamepad","evil":"<script>","map":{"0":{"b":1,"evil":2}},"gameTitle":"forged","updatedAt":99}`), &raw); err != nil {
		t.Fatal(err)
	}
	out, err := CleanProfile(raw, anyConsole)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(out)
	if s := string(b); strings.Contains(s, "evil") || strings.Contains(s, "forged") || strings.Contains(s, "\\u202e") || out.Name != "Mine" || out.UpdatedAt != 0 {
		t.Errorf("%s", s)
	}
	if k, _ := CleanProfile(Profile{Name: "x", Kind: ProfileKeyboard}, anyConsole); k.Controller != ControllerKeyboard {
		t.Errorf("a keyboard profile's controller: %q", k.Controller)
	}
}

func TestParseProfileFile(t *testing.T) {
	ok := `{"format":"` + ProfileFormat + `","version":1,"profiles":[{"name":"a","kind":"gamepad"}]}`
	if l, err := ParseProfileFile([]byte(ok)); err != nil || len(l) != 1 {
		t.Fatalf("%v %v", l, err)
	}
	many := make([]Profile, MaxImportEntries+1)
	mb, _ := json.Marshal(ProfileFile{Format: ProfileFormat, Version: 1, Profiles: many})
	for why, b := range map[string][]byte{
		"not JSON":      []byte("{"),
		"HTML":          []byte("<html><script>alert(1)</script>"),
		"another file":  []byte(`{"format":"theme","version":1,"profiles":[{}]}`),
		"newer version": []byte(`{"format":"` + ProfileFormat + `","version":2,"profiles":[{}]}`),
		"empty":         []byte(`{"format":"` + ProfileFormat + `","version":1,"profiles":[]}`),
		"too many":      mb,
		"too large":     []byte(strings.Repeat(" ", MaxImportSize+1) + ok),
	} {
		if _, err := ParseProfileFile(b); err == nil {
			t.Errorf("%s should be refused", why)
		}
	}
}

func TestKnownControllersAreWellFormed(t *testing.T) {
	d := KnownControllers()
	seen := map[string]bool{}
	for _, p := range d.Pads {
		if !modelRe.MatchString(p.ID) {
			t.Errorf("%q is not vendor:product", p.ID)
		}
		if seen[p.ID] {
			t.Errorf("%s is listed twice", p.ID)
		}
		seen[p.ID] = true
		if p.Name == "" || (p.Style != "xbox" && p.Style != "playstation" && p.Style != "nintendo" && p.Style != "generic") {
			t.Errorf("%s: %+v", p.ID, p)
		}
		for _, l := range []string{p.Layout, p.LinuxLayout} {
			if _, ok := d.Layouts[l]; l != "" && !ok {
				t.Errorf("%s: no layout %q", p.ID, l)
			}
		}
	}
	for name, l := range d.Layouts {
		if len(l.Buttons) == 0 || len(l.Buttons) > 17 {
			t.Errorf("%s has %d buttons", name, len(l.Buttons))
		}
		seen := map[int]bool{}
		for i, b := range l.Buttons {
			if b == nil {
				continue
			}
			if err := b.check(); err != nil {
				t.Errorf("%s button %d: %v", name, i, err)
			}
			if b.Button != nil {
				if seen[*b.Button] {
					t.Errorf("%s uses button %d twice", name, *b.Button)
				}
				seen[*b.Button] = true
			}
		}
	}
}
