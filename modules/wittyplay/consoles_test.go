package wittyplay

import "testing"

func TestDetect(t *testing.T) {
	for _, c := range []struct{ path, want string }{
		{"game.sfc", "snes"},
		{"game.gba", "gba"},
		{"SNES/game.sfc", "snes"},
		{"Sega - Mega Drive/game.bin", "md"},
		{"PlayStation/Some Game/game.cue", "ps1"},
		{"Atari 2600/roms/combat.bin", "a2600"},
		{"NES/game.zip", "nes"},
		{"Super Nintendo (SNES)/game.7z", "snes"},
		{"Nintendo 64 (N64)/game.zip", "n64"},
		{"Playstation (PSX)/game.7z", "ps1"},
		{"Nintendo Entertainment System (NES)/game.nes", "nes"},
		{"Atari 2600/game.tar.gz", "a2600"},
		{"SNES/game.rar", "snes"},
		{"game.7z", ""}, // says nothing until it's looked inside
		{"game.bin", ""}, // used by several consoles
		{"game.cue", ""}, // likewise
		{"Movies/game.iso", ""},
		{"SNES/readme.txt", ""},
		{"game.wittytest", TestConsole},
	} {
		if got := Detect(c.path); got != c.want {
			t.Errorf("Detect(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestExtensionsBelongToOneConsole(t *testing.T) {
	seen := map[string]string{}
	for i := range Consoles {
		for _, e := range Consoles[i].Extensions {
			if other, ok := seen[e]; ok {
				t.Errorf("%s is claimed by both %s and %s; list it as Shared", e, other, Consoles[i].ID)
			}
			seen[e] = Consoles[i].ID
		}
	}
}

func TestValidLibraryConsole(t *testing.T) {
	for _, c := range []struct {
		id      string
		testing bool
		want    bool
	}{{"snes", false, true}, {Auto, false, true}, {"", false, false}, {"nope", false, false}, {TestConsole, false, false}, {TestConsole, true, true}} {
		if got := ValidLibraryConsole(c.id, c.testing); got != c.want {
			t.Errorf("ValidLibraryConsole(%q, %v) = %v", c.id, c.testing, got)
		}
	}
}
