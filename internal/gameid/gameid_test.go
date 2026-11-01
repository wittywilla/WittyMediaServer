package gameid_test

import (
	"bytes"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/gameid"
	"github.com/wittywilla/wittymediaserver/internal/gameid/fixture"
)

func sniff(b []byte) gameid.Finding {
	return gameid.Sniff(bytes.NewReader(b), int64(len(b)))
}

func TestCartridgesAreProvenByTheirHeaders(t *testing.T) {
	for _, c := range []struct {
		name, console, title, serial string
		data                         []byte
	}{
		{"nes", "nes", "", "", fixture.NES()},
		{"snes", "snes", "MADE UP QUEST", "", fixture.SNES("MADE UP QUEST")},
		{"n64", "n64", "MADE UP RACER", "NXXE", fixture.N64("MADE UP RACER", "NXXE")},
		{"n64 swapped", "n64", "MADE UP RACER", "NXXE", fixture.N64Swapped("MADE UP RACER", "NXXE")},
		{"gb", "gb", "MADE UP", "", fixture.GB("MADE UP")},
		{"gba", "gba", "MADE UP", "AXXE", fixture.GBA("MADE UP", "AXXE")},
		{"nds", "nds", "MADE UP", "AYYE", fixture.NDS("MADE UP", "AYYE")},
		{"md", "md", "MADE UP FIGHTER", "GM 000-00", fixture.MegaDrive("MADE UP FIGHTER", "GM 000-00")},
		{"sms", "sms", "", "", fixture.MasterSystem(false)},
		{"gg", "gg", "", "", fixture.MasterSystem(true)},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := sniff(c.data)
			if f.Console != c.console || f.Kind != gameid.KindGame {
				t.Fatalf("got %q %q, want %q: %s", f.Console, f.Kind, c.console, f.Proof)
			}
			if f.Title != c.title || f.Serial != c.serial {
				t.Errorf("got title %q serial %q, want %q %q", f.Title, f.Serial, c.title, c.serial)
			}
			if f.Proof == "" {
				t.Error("it should say why")
			}
		})
	}
}

func TestDiscsAreToldApartByWhatsOnThem(t *testing.T) {
	ps1, ps2 := fixture.PS1("SLUS_123.45"), fixture.PS2("SLUS_543.21")
	psp := fixture.PSP("A Made Up Game", "ULUS10000")
	for _, c := range []struct {
		name, console, serial string
		data                  []byte
	}{
		{"ps1 iso", "ps1", "SLUS-12345", ps1},
		{"ps1 raw", "ps1", "SLUS-12345", fixture.Raw(ps1)},
		{"ps2 iso", "ps2", "SLUS-54321", ps2},
		{"ps2 raw", "ps2", "SLUS-54321", fixture.Raw(ps2)},
		{"psp iso", "psp", "ULUS-10000", psp},
		{"ps2 cso", "ps2", "SLUS-54321", fixture.CSO(ps2)},
		{"psp cso", "psp", "ULUS-10000", fixture.CSO(psp)},
		{"dreamcast", "dc", "", fixture.Boot("SEGA SEGAKATANA")},
		{"saturn", gameid.Saturn, "", fixture.Boot("SEGA SEGASATURN")},
		{"sega cd", gameid.SegaCD, "", fixture.Boot("SEGADISCSYSTEM")},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := sniff(c.data)
			if f.Console != c.console {
				t.Fatalf("got %q, want %q: %s", f.Console, c.console, f.Proof)
			}
			if f.Serial != c.serial {
				t.Errorf("serial %q, want %q", f.Serial, c.serial)
			}
		})
	}
	if f := sniff(fixture.PSP("A Made Up Game", "ULUS10000")); f.Title != "A Made Up Game" {
		t.Errorf("PSP title: %q", f.Title)
	}
	// A plain data disc proves nothing, and says so.
	f := sniff(fixture.ISO("LINUX"))
	if f.Proven() || f.Proof == "" {
		t.Errorf("a plain disc: %+v", f)
	}
	// A film disc isn't a game.
	if f := sniff(fixture.ISO("", fixture.DiscFile{Name: "VIDEO_TS", Data: []byte("x")})); f.Kind != gameid.KindOther {
		t.Errorf("a film disc: %+v", f)
	}
}

func TestThingsThatAreNotGames(t *testing.T) {
	for _, c := range []struct {
		name string
		data []byte
		kind gameid.Kind
	}{
		{"a readme called .md", []byte("# Notes\n\nMade up text about a game.\n"), gameid.KindOther},
		{"a Windows program", append([]byte("MZ"), make([]byte, 4096)...), gameid.KindOther},
		{"a picture", append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...), gameid.KindOther},
		{"chiptune", append([]byte("NESM\x1a"), make([]byte, 256)...), gameid.KindAudio},
	} {
		t.Run(c.name, func(t *testing.T) {
			if f := sniff(c.data); f.Kind != c.kind || f.Proven() {
				t.Fatalf("%+v", f)
			}
		})
	}
	// Random bytes prove nothing at all.
	b := make([]byte, 64<<10)
	rand.New(rand.NewSource(1)).Read(b)
	if f := sniff(b); f.Proven() {
		t.Fatalf("noise proved %s: %s", f.Console, f.Proof)
	}
	// Nor do zeros, or a very short file.
	if f := sniff(make([]byte, 64<<10)); f.Proven() {
		t.Fatalf("zeros proved %s: %s", f.Console, f.Proof)
	}
	if f := sniff([]byte("x")); f.Proven() {
		t.Fatalf("one byte proved %s", f.Console)
	}
}

func Test3DSEncryptionIsReadFromTheBytes(t *testing.T) {
	const size = 1 << 16
	for _, c := range []struct {
		name      string
		data      []byte
		encrypted bool
		kind      gameid.Kind
		newOnly   bool
	}{
		{"plain program", fixture.NCCH(true, false, size), false, gameid.KindGame, false},
		{"encrypted program", fixture.NCCH(false, false, size), true, gameid.KindGame, false},
		{"plain New 3DS program", fixture.NCCH(true, true, size), false, gameid.KindGame, true},
		{"clear package", fixture.CIA(fixture.NCCH(true, false, size), 0, 0x00040000), false, gameid.KindGame, false},
		// Marked encrypted, but the program is plainly readable: the bytes win.
		{"wrongly flagged package", fixture.CIA(fixture.NCCH(true, false, size), 1, 0x00040000), false, gameid.KindGame, false},
		{"encrypted package", fixture.CIA(fixture.Scrambled(size), 1, 0x00040000), true, gameid.KindGame, false},
		{"scrambled, marked clear", fixture.CIA(fixture.Scrambled(size), 0, 0x00040000), true, gameid.KindGame, false},
		{"encrypted program in a clear package", fixture.CIA(fixture.NCCH(false, false, size), 0, 0x00040000), true, gameid.KindGame, false},
		{"update", fixture.CIA(fixture.NCCH(true, false, size), 0, 0x0004000E), false, gameid.KindUpdate, false},
		{"extra content", fixture.CIA(fixture.NCCH(true, false, size), 0, 0x0004008C), false, gameid.KindDLC, false},
		{"system software", fixture.CIA(fixture.NCCH(true, false, size), 0, 0x00040010), false, gameid.KindUtility, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := sniff(c.data)
			if f.Console != "3ds" {
				t.Fatalf("console %q", f.Console)
			}
			if f.Encrypted != c.encrypted || f.Kind != c.kind || f.NewOnly != c.newOnly {
				t.Fatalf("%+v", f)
			}
		})
	}
}

func TestMangledFilesDoNotPanicOrProveAnything(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	seeds := [][]byte{fixture.PS1("SLUS_123.45"), fixture.PSP("Made Up", "ULUS10000"), fixture.CSO(fixture.PS2("SLUS_543.21")),
		fixture.CIA(fixture.NCCH(true, false, 1<<14), 0, 0x00040000), fixture.SFO([2]string{"TITLE", "x"})}
	for _, seed := range seeds {
		for i := 0; i < 300; i++ {
			b := append([]byte(nil), seed...)
			for j := 0; j < 12; j++ {
				b[rng.Intn(len(b))] = byte(rng.Intn(256))
			}
			if i%5 == 0 {
				b = b[:rng.Intn(len(b))]
			}
			_ = sniff(b)
		}
	}
}

func TestSheets(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write("Game (Track 1).bin", fixture.Raw(fixture.PS1("SLUS_123.45")))
	write("Game (Track 2).bin", make([]byte, 2352))
	good := write("Game.cue", []byte("FILE \"Game (Track 1).bin\" BINARY\n  TRACK 01 MODE2/2352\n    INDEX 01 00:00:00\nFILE \"Game (Track 2).bin\" BINARY\n  TRACK 02 AUDIO\n    INDEX 01 00:00:00\n"))
	f, err := gameid.SniffFile(good)
	if err != nil || f.Console != "ps1" || f.Serial != "SLUS-12345" {
		t.Fatalf("%+v %v", f, err)
	}
	music := write("Album.cue", []byte("FILE \"Game (Track 2).bin\" BINARY\n  TRACK 01 AUDIO\n    INDEX 01 00:00:00\n"))
	if f, _ := gameid.SniffFile(music); f.Kind != gameid.KindAudio {
		t.Fatalf("a music CD: %+v", f)
	}
	missing := write("Lost.cue", []byte("FILE \"Gone.bin\" BINARY\n  TRACK 01 MODE2/2352\n"))
	if f, _ := gameid.SniffFile(missing); f.Kind != gameid.KindData || !strings.Contains(f.Proof, "Gone.bin") {
		t.Fatalf("a sheet with its file missing: %+v", f)
	}
	// A name can't lead out of the folder, however it's written.
	escape := write("Bad.cue", []byte("FILE \"../outside.bin\" BINARY\n  TRACK 01 MODE2/2352\nFILE \"..\\outside.bin\" BINARY\n"))
	s, err := gameid.ReadSheet(escape)
	if err != nil || len(s.Files) != 0 || s.Data != "" {
		t.Fatalf("%+v %v", s, err)
	}
	// A different case in the name still finds the file.
	cased := write("Cased.cue", []byte("FILE \"GAME (TRACK 1).BIN\" BINARY\n  TRACK 01 MODE2/2352\n"))
	if f, _ := gameid.SniffFile(cased); f.Console != "ps1" {
		t.Fatalf("a name in another case: %+v", f)
	}
}

func TestNames(t *testing.T) {
	for _, c := range []struct {
		name string
		kind gameid.Kind
	}{
		{"Super Made Up BIOS (V1.0) [!]", gameid.KindBIOS},
		{"[BIOS] Made Up (USA)", gameid.KindBIOS},
		{"SCPH-39001", gameid.KindBIOS},
		{"Bios Quest (Japan)", gameid.KindUnknown}, // a game with the word in its name
		{"Game Genie (USA) (Unl)", gameid.KindUtility},
		{"Pro Action Replay Mk2", gameid.KindUtility},
		{"Controller Test Cartridge (J) [!]", gameid.KindUtility},
		{"Super Game Boy (JU) (V1.1) [!]", gameid.KindUtility},
		{"Made Up (USA) (Update 1.02)", gameid.KindUpdate},
		{"Made Up [Update]", gameid.KindUpdate},
		{"Made Up (USA) (DLC)", gameid.KindDLC},
		{"Made Up: The Update (USA)", gameid.KindUnknown}, // only tags say so
		{"Made Up (USA) (Rev 1)", gameid.KindUnknown},
	} {
		if got, _ := gameid.ClassifyName(c.name); got != c.kind {
			t.Errorf("%q: %q, want %q", c.name, got, c.kind)
		}
	}
	for _, c := range []struct {
		in          string
		disc, total int
		rest        string
	}{
		{"Mega Game (Disc 2 of 3)", 2, 3, "Mega Game"},
		{"Mega Game Disc 1", 1, 0, "Mega Game"},
		{"mega-game-disc1of3-u-scus-94163-", 1, 3, "mega-game u-scus-94163-"},
		{"Mega Game [CD2]", 2, 0, "Mega Game"},
		{"Mega Game", 0, 0, "Mega Game"},
		{"Sega CD Classics", 0, 0, "Sega CD Classics"},
	} {
		d, n, r := gameid.DiscIn(c.in)
		if d != c.disc || n != c.total || r != c.rest {
			t.Errorf("DiscIn(%q) = %d, %d, %q; want %d, %d, %q", c.in, d, n, r, c.disc, c.total, c.rest)
		}
	}
	if s := gameid.SerialIn("mega-game-disc1of3-u-scus-94163-"); s != "SCUS-94163" {
		t.Errorf("serial %q", s)
	}
	if s := gameid.Unslug("mega-game-u-scus-94163-"); s != "mega game" {
		t.Errorf("unslug %q", s)
	}
	for _, c := range []struct {
		in   string
		want gameid.Quality
	}{
		{"Game (U) [!].smc", gameid.QualityClean},
		{"Game (U).smc", gameid.QualityClean},
		{"Game (Beta).smc", gameid.QualityPrerelease},
		{"Game (U) [b1].smc", gameid.QualityAltered},
		{"Game (U) [T+Fre1.00_Someone].smc", gameid.QualityTranslated},
		{"Game (SMW1 Hack).smc", gameid.QualityHack},
		{"Intro by Someone (PD).smc", gameid.QualityPD},
	} {
		if got := gameid.QualityOf(c.in); got != c.want {
			t.Errorf("%q: %v, want %v", c.in, got, c.want)
		}
	}
}
