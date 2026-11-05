package server

import (
	"crypto/md5"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The table that ships: every line read, and the firmware people most often have in it.
func TestTheBIOSTableIsRead(t *testing.T) {
	if n := len(biosKnown.byMD5); n < 70 {
		t.Fatalf("%d known BIOS files", n)
	}
	for md5, want := range map[string]string{
		"490f666e1afb15b7362b406ed1cea246": "scph5501.bin",                               // PlayStation, US
		"a860e8c0b6d573d191e4ec7db1b1e4f6": "gba_bios.bin",                               // Game Boy Advance
		"d5ce2c7d119f563ce04bc04dbc3a323e": "pcsx2/bios/ps2-bios-usa-0160a-20020207.bin", // PlayStation 2, SCPH-39001
		"0763f1ffb006ddbe32e52d497ee848ae": "7800 BIOS (U).rom",                          // a space in the name
	} {
		if k := biosKnown.byMD5[md5]; k.Dest != want {
			t.Errorf("%s: %+v, want %s", md5, k, want)
		}
	}
	for _, k := range biosKnown.byMD5 {
		if k.Console == "ps2" && (!strings.Contains(k.Dest, "bios") || k.Region == "") {
			t.Errorf("a PlayStation 2 BIOS LRPS2's rule wouldn't take, or without a region: %+v", k)
		}
	}
}

func fakeBIOS(t *testing.T, table biosTable, contents, console, dest, region string) []byte {
	t.Helper()
	b := []byte(contents)
	sum := md5.Sum(b)
	k := knownBIOS{MD5: hex.EncodeToString(sum[:]), Size: int64(len(b)), Console: console, Dest: dest, Region: region}
	table.byMD5[k.MD5], table.sizes[k.Size] = k, true
	return b
}

func write(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A BIOS kept beside the games under any name, in folders of its own, is found by what it is,
// copied where its core looks, and said to have come from there. Anything else is left alone.
func TestBIOSFoundByChecksum(t *testing.T) {
	table := biosTable{byMD5: map[string]knownBIOS{}, sizes: map[int64]bool{}}
	ps1 := fakeBIOS(t, table, "a PlayStation BIOS of a known checksum", "ps1", "scph5501.bin", "usa")
	ps2 := fakeBIOS(t, table, "a PlayStation 2 BIOS, by another name", "ps2", "pcsx2/bios/ps2-bios-usa-0160a.bin", "usa")
	g := &gameService{dir: t.TempDir()}
	lib := t.TempDir()
	write(t, filepath.Join(lib, "PlayStation", "Bios", "PSX BIOS (USA) v4.1.bin"), ps1)
	write(t, filepath.Join(lib, "PlayStation 2", "BIOS", "SCPH-39001 (NTSC)", "whatever.BIN"), ps2)
	write(t, filepath.Join(lib, "PlayStation 2", "BIOS", "notes.txt"), []byte("not firmware"))
	// The right name, the wrong contents: never taken for the real thing.
	write(t, filepath.Join(lib, "PlayStation", "Bios", "scph5502.bin"), []byte("a PlayStation BIOS of another checksum"))
	write(t, filepath.Join(lib, "PlayStation", "Game.cue"), []byte("not in a BIOS folder"))

	folders := biosFoldersInLibrary([]string{lib})
	if len(folders) != 2 {
		t.Fatalf("BIOS folders: %v", folders)
	}
	if n := g.findBIOS(table, folders); n != 2 {
		t.Fatalf("copied %d", n)
	}
	sys := g.systemDir()
	for dest, want := range map[string][]byte{"scph5501.bin": ps1, "pcsx2/bios/ps2-bios-usa-0160a.bin": ps2} {
		got, err := os.ReadFile(filepath.Join(sys, filepath.FromSlash(dest)))
		if err != nil || string(got) != string(want) {
			t.Errorf("%s: %q %v", dest, got, err)
		}
	}
	if _, err := os.Stat(filepath.Join(sys, "scph5502.bin")); err == nil {
		t.Error("a file was taken by its name")
	}
	rec := g.biosRecords()["scph5501.bin"]
	if rec.Console != "ps1" || !strings.HasSuffix(rec.From, filepath.Join("Bios", "PSX BIOS (USA) v4.1.bin")) || rec.Region != "usa" {
		t.Errorf("where it came from: %+v", rec)
	}

	// Looked at again: nothing more is copied.
	if n := g.findBIOS(table, folders); n != 0 {
		t.Errorf("copied %d again", n)
	}
	// The same firmware already there under another name, as WittyPlay's own collector names it,
	// isn't copied a second time, and is the one recorded.
	g2 := &gameService{dir: t.TempDir()}
	write(t, filepath.Join(g2.systemDir(), "pcsx2", "bios", "SCPH-39001_BIOS_V7_USA_160.BIN"), ps2)
	g2.findBIOS(table, folders)
	if _, err := os.Stat(filepath.Join(g2.systemDir(), "pcsx2", "bios", "ps2-bios-usa-0160a.bin")); err == nil {
		t.Error("copied beside the same firmware under another name")
	}
	if rec := g2.biosRecords()["pcsx2/bios/SCPH-39001_BIOS_V7_USA_160.BIN"]; rec.Console != "ps2" {
		t.Errorf("the copy already there wasn't recorded: %+v", g2.biosRecords())
	}
	// Someone's own different file under the name is left alone.
	g3 := &gameService{dir: t.TempDir()}
	write(t, filepath.Join(g3.systemDir(), "scph5501.bin"), []byte("theirs"))
	g3.findBIOS(table, folders)
	if got, _ := os.ReadFile(filepath.Join(g3.systemDir(), "scph5501.bin")); string(got) != "theirs" {
		t.Errorf("replaced someone's own file: %q", got)
	}
}

func TestBIOSFoldersBesideAGame(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "PS2", "BIOS", "x.bin"), []byte("x"))
	write(t, filepath.Join(root, "PS2", "Some Game", "bios", "y.bin"), []byte("y"))
	got := biosFoldersNear(filepath.Join(root, "PS2", "Some Game", "game.iso"))
	if len(got) != 2 {
		t.Errorf("%v", got)
	}
}

func TestAGameStartsOnABIOSOfItsRegion(t *testing.T) {
	files := []string{"SCPH-10000_JAP.BIN", "SCPH-39001_USA.BIN", "SCPH-39004_EUR.BIN", "SCPH-77004_EUR.BIN"}
	regionOf := func(n string) string { return biosRegion(n, biosRecord{}, false) }
	for _, c := range []struct{ region, want string }{
		{"usa", "SCPH-39001_USA.BIN"},
		{"europe", "SCPH-39004_EUR.BIN"},
		{"japan", "SCPH-10000_JAP.BIN"},
		{"", "SCPH-39001_USA.BIN"},       // not known: what the core would take anyway
		{"brazil", "SCPH-39001_USA.BIN"}, // none of its own: the same
	} {
		if got := pickRegionBIOS(files, "SCPH-39001_USA.BIN", c.region, regionOf); got != c.want {
			t.Errorf("%q: %s, want %s", c.region, got, c.want)
		}
	}
	for serial, want := range map[string]string{"SLUS_203.12": "usa", "SCES-50490": "europe", "SLPM_654.01": "japan", "": ""} {
		if got := gameRegion(serial, "Game.iso"); got != want {
			t.Errorf("%q: %q", serial, got)
		}
	}
	if got := gameRegion("", "Gran Turismo 4 (Europe).iso"); got != "europe" {
		t.Errorf("by name: %q", got)
	}
	// What the table recognised it as comes before what its name says.
	if got := biosRegion("SCPH-39001_USA.BIN", biosRecord{Region: "europe"}, true); got != "europe" {
		t.Error(got)
	}
}
