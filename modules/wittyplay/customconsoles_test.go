package wittyplay

import (
	"strings"
	"testing"
)

func validConsole(t *testing.T, raw string, existing ...Console) *Console {
	t.Helper()
	in, err := ParseCustomConsole([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	c, err := in.Validate("", existing)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCustomConsoleFromJSON(t *testing.T) {
	c := validConsole(t, `{"name":"Fantasy Box","short":"FBox","extensions":["fbx",".FBX2",".sfc"],"aliases":["Fantasy-Box 3000"],
		"bios":[{"file":"boot/fbx.rom","for":"start-up","required":true}],"padLike":"snes"}`)
	if c.ID != "custom-fbox" || c.Name != "Fantasy Box" || c.Short != "FBox" {
		t.Errorf("got %+v", c)
	}
	if strings.Join(c.Extensions, ",") != ".fbx,.fbx2" {
		t.Errorf("extensions = %v", c.Extensions)
	}
	if strings.Join(c.Shared, ",") != ".sfc" {
		t.Errorf("an extension the table uses must only be shared: %v", c.Shared)
	}
	for _, a := range []string{"fantasy3000", "fantasybox3000", "fantasybox", "fbox"} {
		if a != "fantasy3000" && !contains(c.Aliases, a) {
			t.Errorf("aliases %v lack %q", c.Aliases, a)
		}
	}
	if len(c.BIOS) != 1 || !c.BIOS[0].Required {
		t.Errorf("bios = %+v", c.BIOS)
	}
}

func TestCustomConsoleRefusals(t *testing.T) {
	for _, c := range []struct{ name, raw, says string }{
		{"not JSON", `{`, "isn't a console"},
		{"unknown field", `{"name":"A","short":"A","extensions":[".a"],"core":"x"}`, "isn't a console"},
		{"two objects", `{"name":"A","short":"A","extensions":[".a"]}{"name":"B"}`, "more than one"},
		{"no name", `{"short":"A","extensions":[".a"]}`, "name is empty"},
		{"no short name", `{"name":"A","extensions":[".a"]}`, "short name is empty"},
		{"too long", `{"name":"` + strings.Repeat("a", 81) + `","short":"A","extensions":[".a"]}`, "longer"},
		{"control characters", `{"name":"A\u0007B","short":"A","extensions":[".a"]}`, "can't be used"},
		{"invisible direction change", `{"name":"A\u202eB","short":"A","extensions":[".a"]}`, "can't be used"},
		{"angle brackets", `{"name":"<img src=x onerror=alert(1)>","short":"A","extensions":[".a"]}`, "can't be used"},
		{"only symbols", `{"name":"!!!","short":"???","extensions":[".a"]}`, "letters or numbers"},
		{"a name from the table", `{"name":"Super Nintendo","short":"X","extensions":[".a"]}`, "already a console"},
		{"no extensions", `{"name":"A","short":"A","extensions":[]}`, "at least one"},
		{"an extension with a path", `{"name":"A","short":"A","extensions":["../../etc"]}`, "isn't a file extension"},
		{"an extension with spaces", `{"name":"A","short":"A","extensions":[". a"]}`, "isn't a file extension"},
		{"an archive", `{"name":"A","short":"A","extensions":[".zip"]}`, "archive"},
		{"too many extensions", `{"name":"A","short":"A","extensions":[` + strings.Repeat(`".a",`, 64) + `".a"]}`, "too many"},
		{"a folder name the table uses", `{"name":"A","short":"A","extensions":[".a"],"aliases":["SNES"]}`, "already means"},
		{"a folder name with no letters", `{"name":"A","short":"A","extensions":[".a"],"aliases":["---"]}`, "letters or numbers"},
		{"BIOS climbing out", `{"name":"A","short":"A","extensions":[".a"],"bios":[{"file":"../../secret","for":"x"}]}`, "BIOS"},
		{"BIOS with a drive", `{"name":"A","short":"A","extensions":[".a"],"bios":[{"file":"C:\\Windows\\x","for":"x"}]}`, "BIOS"},
		{"BIOS absolute", `{"name":"A","short":"A","extensions":[".a"],"bios":[{"file":"/etc/passwd","for":"x"}]}`, "BIOS"},
		{"BIOS named as a device", `{"name":"A","short":"A","extensions":[".a"],"bios":[{"file":"a/..","for":"x"}]}`, "BIOS"},
		{"a controller that doesn't exist", `{"name":"A","short":"A","extensions":[".a"],"padLike":"nope"}`, "controller"},
		{"the hidden test controller", `{"name":"A","short":"A","extensions":[".a"],"padLike":"wittytest"}`, "controller"},
		{"an ID without the prefix", `{"id":"snes","name":"A","short":"A","extensions":[".a"]}`, "must look like"},
		{"an ID with a path", `{"id":"custom-../x","name":"A","short":"A","extensions":[".a"]}`, "must look like"},
		{"a core that can't be a name", `{"name":"A","short":"A","extensions":[".a"],"cores":["../evil"]}`, "name of a core"},
	} {
		t.Run(c.name, func(t *testing.T) {
			in, err := ParseCustomConsole([]byte(c.raw))
			if err == nil {
				_, err = in.Validate("", nil)
			}
			if err == nil || !strings.Contains(err.Error(), c.says) {
				t.Fatalf("got %v, want %q", err, c.says)
			}
		})
	}
	t.Run("far too much JSON", func(t *testing.T) {
		if _, err := ParseCustomConsole(make([]byte, MaxCustomConsoleJSON+1)); err == nil {
			t.Fatal("it should be refused")
		}
	})
}

func TestCustomConsolesAreFoundAndStaySeparate(t *testing.T) {
	first := validConsole(t, `{"name":"Fantasy Box","short":"FBox","extensions":[".fbx",".iso"],"aliases":["fbox3000"]}`)
	second := validConsole(t, `{"name":"Other Box","short":"OBox","extensions":[".fbx",".obx"]}`, *first)
	if contains(second.Extensions, ".fbx") || !contains(second.Shared, ".fbx") {
		t.Errorf("the second console to claim an extension only shares it: %+v", second)
	}
	SetCustomConsoles([]Console{*first, *second})
	t.Cleanup(func() { SetCustomConsoles(nil) })

	if c := ByID(first.ID); c == nil || c.Name != "Fantasy Box" {
		t.Errorf("ByID = %+v", c)
	}
	if !ValidLibraryConsole(first.ID, false) {
		t.Error("it should be usable for a library")
	}
	if len(CustomConsoles()) != 2 {
		t.Errorf("CustomConsoles = %v", CustomConsoles())
	}
	for path, want := range map[string]string{
		"game.fbx":             first.ID,
		"Fantasy Box/game.iso": first.ID,
		"fbox3000/game.iso":    first.ID,
		"OBox/game.fbx":        second.ID,
		"game.obx":             second.ID,
		"game.iso":             "",
		"SNES/game.sfc":        "snes", // the table wins for its own folders
		"Fantasy Box/game.sfc": "snes",
		"game.sfc":             "snes",
	} {
		if got := Detect(path); got != want {
			t.Errorf("Detect(%q) = %q, want %q", path, got, want)
		}
	}
	if len(Consoles) != len(All())-2 {
		t.Error("the table itself must not change")
	}
	if _, ok := CoreLicense("nothing"); ok {
		t.Error("custom consoles add nothing to what we vouch for")
	}

	withPad := validConsole(t, `{"name":"Pad Box","short":"PBox","extensions":[".pbx"],"padLike":"nes"}`)
	SetCustomConsoles([]Console{*withPad})
	if p := Pad(withPad.ID); len(p.Face) != 2 {
		t.Errorf("Pad = %+v", p)
	}
	if p := Pad("custom-unknown"); !p.Sticks {
		t.Errorf("no layout chosen means a whole RetroPad: %+v", p)
	}
}

func TestCustomConsoleEditKeepsItsID(t *testing.T) {
	first := validConsole(t, `{"name":"Fantasy Box","short":"FBox","extensions":[".fbx"]}`)
	in, _ := ParseCustomConsole([]byte(`{"name":"Fantasy Box 2","short":"FB2","extensions":[".fbx"]}`))
	c, err := in.Validate(first.ID, []Console{*first})
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != first.ID {
		t.Errorf("ID became %q", c.ID)
	}
	if !contains(c.Extensions, ".fbx") {
		t.Errorf("its own extension isn't someone else's: %+v", c)
	}
}

func TestNonCommercialNames(t *testing.T) {
	for _, n := range []string{"Snes9x", "snes9x 2010", "Genesis Plus GX", "PicoDrive", "FinalBurn Neo", "MAME 2003-Plus"} {
		if !NonCommercialName(n) {
			t.Errorf("%q should be recognised", n)
		}
	}
	for _, n := range []string{"", "mGBA", "Gambatte", "My Own Core"} {
		if NonCommercialName(n) {
			t.Errorf("%q isn't one", n)
		}
	}
	for _, l := range []string{"Non-commercial", "CC BY-NC 4.0", "noncommercial only"} {
		if !NonCommercialLicense(l) {
			t.Errorf("%q should count", l)
		}
	}
	if NonCommercialLicense("GPLv3") {
		t.Error("GPLv3 doesn't forbid it")
	}
}
