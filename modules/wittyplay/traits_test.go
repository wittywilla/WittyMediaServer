package wittyplay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectBracketedFolders(t *testing.T) {
	for _, c := range []struct{ path, want string }{
		{"Nintendo 64 (N64)/game.z64", "n64"},
		{"Nintendo 64 (N64)/game.zip", "n64"},
		{"Playstation 2 (PS2)/game.iso", "ps2"},
		{"Playstation 2 (PS2)/game.bin", "ps2"},
		{"PlayStation/game.iso", "ps1"},
		{"Nintendo 3DS/game.3ds", "3ds"},
		{"game.nds", "nds"},
	} {
		if got := Detect(c.path); got != c.want {
			t.Errorf("Detect(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestEveryConsolePlayable(t *testing.T) {
	for i := range Consoles {
		c := &Consoles[i]
		if c.Hidden {
			continue
		}
		if c.Unavailable != "" {
			t.Errorf("%s is still marked unavailable: %s", c.ID, c.Unavailable)
		}
		if len(c.Cores) == 0 {
			t.Errorf("%s has no cores", c.ID)
		}
	}
}

func TestRunnerArgs(t *testing.T) {
	for id, want := range map[string]string{"fceumm": "", "melonds": "-normal-priority", "mupen64plus_next": "-gpu -normal-priority", "someone_elses_core": "-gpu -normal-priority"} {
		if got := strings.Join(RunnerArgs(id), " "); got != want {
			t.Errorf("RunnerArgs(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestStartOptions(t *testing.T) {
	o := StartOptions("n64", "mupen64plus_next", "", t.TempDir())
	if o["mupen64plus-43screensize"] != "1280x960" {
		t.Errorf("automatic N64 resolution = %q", o["mupen64plus-43screensize"])
	}
	if o := StartOptions("n64", "mupen64plus_next", "1", t.TempDir()); o["mupen64plus-43screensize"] != "640x480" {
		t.Errorf("native N64 resolution = %q", o["mupen64plus-43screensize"])
	}
	if o := StartOptions("n64", "mupen64plus_next", "nonsense", t.TempDir()); o["mupen64plus-43screensize"] != "1280x960" {
		t.Errorf("a bad setting should mean automatic, got %q", o["mupen64plus-43screensize"])
	}
	if o := StartOptions("nes", "fceumm", "", t.TempDir()); len(o) != len(CoreOptions("fceumm")) {
		t.Errorf("a software-rendered core got extra options: %v", o)
	}
}

func TestTraitsGPU(t *testing.T) {
	for id, want := range map[string]bool{"mupen64plus_next": true, "pcsx2": true, "azahar": true, "melonds": false, "fceumm": false, "someone_elses_core": true} {
		if got := Traits(id).GPU; got != want {
			t.Errorf("Traits(%q).GPU = %v, want %v", id, got, want)
		}
	}
}

func fakeBIOS(t *testing.T, path string, size int64) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

// BIOS files are found the way people really keep them: one folder per console
// model, beside the games, with small companion files.
func TestCollectBIOS(t *testing.T) {
	root := t.TempDir()
	games := filepath.Join(root, "Playstation 2 (PS2)")
	game := filepath.Join(games, "game.iso")
	fakeBIOS(t, game, 16)
	const image = 4 << 20
	fakeBIOS(t, filepath.Join(games, "BIOS", "SCPH-10000_JAP", "SCPH-10000_BIOS_V1_JAP_100.BIN"), image)
	fakeBIOS(t, filepath.Join(games, "BIOS", "SCPH-39001_USA", "SCPH-39001_BIOS_V7_USA_160.BIN"), image)
	fakeBIOS(t, filepath.Join(games, "BIOS", "SCPH-39001_USA", "SCPH-39001_BIOS_V7_USA_160.NVM"), 1024)
	fakeBIOS(t, filepath.Join(games, "BIOS", "SCPH-39001_USA", "readme.txt"), 10)
	fakeBIOS(t, filepath.Join(games, "BIOS", "wrong size", "SCPH-1.BIN"), 1024)
	system := filepath.Join(root, "system")

	if MissingBIOS("pcsx2", system) == "" {
		t.Fatal("an empty system folder should be missing a BIOS")
	}
	if BIOSHelp("pcsx2", "PlayStation 2") == "" {
		t.Fatal("there should be words for a missing BIOS")
	}
	if err := CollectBIOS(ByID("ps2"), "pcsx2", game, system); err != nil {
		t.Fatal(err)
	}
	if got := MissingBIOS("pcsx2", system); got != "" {
		t.Fatalf("still missing %q", got)
	}
	bios := filepath.Join(system, "pcsx2", "bios")
	for _, want := range []string{"SCPH-39001_BIOS_V7_USA_160.BIN", "SCPH-39001_BIOS_V7_USA_160.NVM", "SCPH-10000_BIOS_V1_JAP_100.BIN"} {
		if _, err := os.Stat(filepath.Join(bios, want)); err != nil {
			t.Errorf("%s wasn't copied: %v", want, err)
		}
	}
	for _, not := range []string{"readme.txt", "SCPH-1.BIN"} {
		if _, err := os.Stat(filepath.Join(bios, not)); err == nil {
			t.Errorf("%s shouldn't have been copied", not)
		}
	}
	if got := StartOptions("ps2", "pcsx2", "", system)["pcsx2_bios"]; got != "SCPH-39001_BIOS_V7_USA_160.BIN" {
		t.Errorf("chose %q from a set", got)
	}
	// A second run changes nothing, and the originals stay where they were.
	if err := CollectBIOS(ByID("ps2"), "pcsx2", game, system); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(games, "BIOS", "SCPH-39001_USA", "SCPH-39001_BIOS_V7_USA_160.BIN")); err != nil {
		t.Error("the original BIOS should stay where it was")
	}
}

func TestCollectBIOSOptionalFiles(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "Dreamcast", "game.cdi")
	fakeBIOS(t, game, 16)
	fakeBIOS(t, filepath.Join(root, "Dreamcast", "bios", "dc_boot.bin"), 1024)
	system := filepath.Join(root, "system")
	if err := CollectBIOS(ByID("dc"), "flycast", game, system); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(system, "dc", "dc_boot.bin")); err != nil {
		t.Errorf("the Dreamcast BIOS should land where the core looks: %v", err)
	}
}

func TestIsBIOSFile(t *testing.T) {
	for _, c := range []struct {
		console, name string
		want          bool
	}{
		{"ps2", "SCPH-39001_BIOS_V7_USA_160.BIN", true},
		{"ps2", "ps2-bios.bin", true},
		{"ps2", "Biosphere Rising.bin", false},
		{"ps2", "Some Game.iso", false},
		{"ps1", "scph5501.bin", true},
		{"ps1", "Some Game.bin", false},
	} {
		if got := IsBIOSFile(c.console, c.name); got != c.want {
			t.Errorf("IsBIOSFile(%q, %q) = %v", c.console, c.name, got)
		}
	}
}
