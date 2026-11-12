// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package gameid_test

import (
	"io"
	"math/rand"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/gameid"
	"github.com/wittywilla/wittymediaserver/internal/gameid/fixture"
)

// sparse is a big file that's zeros except where parts are, so a test can stand for a
// full disc image without holding one.
type sparse struct {
	size  int64
	parts map[int][]byte
}

func (s sparse) ReadAt(b []byte, off int64) (int, error) {
	if off < 0 || off >= s.size {
		return 0, io.EOF
	}
	n := len(b)
	if int64(n) > s.size-off {
		n = int(s.size - off)
	}
	clear(b[:n])
	for at, p := range s.parts {
		lo, hi := max(int64(at), off), min(int64(at)+int64(len(p)), off+int64(n))
		if lo < hi {
			copy(b[lo-off:hi-off], p[lo-int64(at):hi-int64(at)])
		}
	}
	if n < len(b) {
		return n, io.EOF
	}
	return n, nil
}

func flat(parts map[int][]byte, size int) []byte {
	b := make([]byte, size)
	for at, p := range parts {
		copy(b[at:], p)
	}
	return b
}

func TestNewerConsolesAreProvenByTheirBytes(t *testing.T) {
	gc := fixture.NintendoDisc("GXXE01", "Made Up Racer", false)
	wii := fixture.NintendoDisc("RXXP01", "Made Up Party", true)
	for _, c := range []struct {
		name, console, title, serial, region string
		kind                                 gameid.Kind
		encrypted                            bool
		data                                 []byte
	}{
		{"gamecube disc", "gc", "Made Up Racer", "GXXE01", "USA", gameid.KindGame, false, gc},
		{"wii disc", "wii", "Made Up Party", "RXXP01", "Europe", gameid.KindGame, false, wii},
		{"gamecube ciso", "gc", "Made Up Racer", "GXXE01", "USA", gameid.KindGame, false, fixture.DolphinCISO(gc)},
		{"gamecube gcz", "gc", "Made Up Racer", "GXXE01", "USA", gameid.KindGame, false, fixture.GCZ(gc)},
		{"wii gcz", "wii", "Made Up Party", "RXXP01", "Europe", gameid.KindGame, false, fixture.GCZ(wii)},
		{"wii wia", "wii", "Made Up Party", "RXXP01", "Europe", gameid.KindGame, false, fixture.WIA(wii[:0x80], 2, false)},
		{"gamecube rvz", "gc", "Made Up Racer", "GXXE01", "USA", gameid.KindGame, false, fixture.WIA(gc[:0x80], 1, true)},
		{"rvz with no header", "wii", "", "", "", gameid.KindGame, false, fixture.WIA(nil, 2, true)},
		{"wbfs", "wii", "Made Up Party", "RXXP01", "Europe", gameid.KindGame, false, fixture.WBFS(wii[:0x100])},
		{"gamecube dol", "gc", "", "", "", gameid.KindGame, false, fixture.DOL(false)},
		{"wii dol", "wii", "", "", "", gameid.KindGame, false, fixture.DOL(true)},
		{"wiiware wad", "wii", "", "WXXE", "USA", gameid.KindGame, false, fixture.WAD(0x00010001, "WXXE")},
		{"system wad", "wii", "", "", "", gameid.KindUtility, false, fixture.WAD(0x00000001, "\x00\x00\x00\x02")},
		{"wii u disc", "wiiu", "", "AXXE", "USA", gameid.KindGame, true, fixture.WUD("AXXE")},
		{"wii u wux", "wiiu", "", "AXXJ", "Japan", gameid.KindGame, true, fixture.WUX("AXXJ")},
		{"wii u program", "wiiu", "", "", "", gameid.KindGame, false, fixture.RPX()},
		{"cemu archive", "wiiu", "", "", "", gameid.KindGame, false, fixture.WUA()},
		{"ps3 disc", "ps3", "A Made Up Game", "BLUS-30000", "USA", gameid.KindGame, false, fixture.PS3("A Made Up Game", "BLUS30000")},
		{"ps3 pkg", "ps3", "", "NPEB-00000", "Europe", gameid.KindGame, false, fixture.PKG(1, "EP0000-NPEB00000_00-MADEUPGAME000000")},
		{"xbox xbe", "xbox", "Made Up Fighter", "MS-004", "USA", gameid.KindGame, false, fixture.XBE("Made Up Fighter", 0x4D530004)},
		{"xbox xiso", "xbox", "Made Up Fighter", "MS-004", "USA", gameid.KindGame, false, flat(fixture.XISO(0, "Made Up Fighter", 0x4D530004), 0x20000)},
		{"switch nsp", "switch", "", "0100000000010000", "", gameid.KindGame, true, fixture.NSP("0100000000010000000000000000000a.tik", "00112233445566778899aabbccddeeff.nca")},
		{"switch nsz", "switch", "", "", "", gameid.KindGame, true, fixture.NSP("00112233445566778899aabbccddeeff.ncz")},
		{"switch update", "switch", "", "0100000000010800", "", gameid.KindUpdate, true, fixture.NSP("0100000000010800000000000000000a.tik", "00112233445566778899aabbccddeeff.nca")},
		{"switch dlc", "switch", "", "0100000000011001", "", gameid.KindDLC, true, fixture.NSP("0100000000011001000000000000000a.tik", "00112233445566778899aabbccddeeff.nca")},
		{"switch xci", "switch", "", "", "", gameid.KindGame, true, fixture.XCI()},
		{"switch homebrew", "switch", "Made Up Homebrew", "", "", gameid.KindGame, false, fixture.NRO("Made Up Homebrew")},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := sniff(c.data)
			if f.Console != c.console || f.Kind != c.kind {
				t.Fatalf("got %q %q, want %q %q: %s", f.Console, f.Kind, c.console, c.kind, f.Proof)
			}
			if f.Title != c.title || f.Serial != c.serial || f.Region != c.region {
				t.Errorf("got title %q serial %q region %q, want %q %q %q", f.Title, f.Serial, f.Region, c.title, c.serial, c.region)
			}
			if f.Encrypted != c.encrypted || f.NeedsKeys != c.encrypted {
				t.Errorf("encrypted %v, needs keys %v, want %v", f.Encrypted, f.NeedsKeys, c.encrypted)
			}
			if c.encrypted && c.kind == gameid.KindGame && !strings.Contains(f.Proof, "keys") {
				t.Errorf("an encrypted file should say it needs keys: %s", f.Proof)
			}
			if f.Proof == "" {
				t.Error("it should say why")
			}
		})
	}
}

func TestXboxDiscsAreFoundWhereverTheGameStarts(t *testing.T) {
	// A full disc image has its video partition first, and is far too big to make.
	const base = 0x18300000
	disc := sparse{size: base + 0x100000, parts: fixture.XISO(base, "Made Up Fighter", 0x4D530004)}
	f := gameid.Sniff(disc, disc.size)
	if f.Console != "xbox" || f.Title != "Made Up Fighter" || f.Serial != "MS-004" {
		t.Fatalf("%+v", f)
	}
	// Cut off before the game partition, it's nothing.
	short := sparse{size: base, parts: fixture.XISO(base, "x", 0)}
	if f := gameid.Sniff(short, short.size); f.Proven() {
		t.Fatalf("a cut-off image proved %s", f.Console)
	}
}

func TestNewerLookalikesProveNothing(t *testing.T) {
	for _, c := range []struct {
		name string
		data []byte
	}{
		// A PSP's install package isn't a PS3 game.
		{"psp pkg", fixture.PKG(2, "UP0000-ULUS00000_00-MADEUPGAME000000")},
		// A Switch file system with no contents.
		{"bare pfs0", fixture.NSP("main", "main.npdm")},
		// HEAD where a Switch card has it, with no card size.
		{"head with no card size", append(append(make([]byte, 0x100), "HEAD"...), make([]byte, 0x400)...)},
		// An empty .wbfs.
		{"empty wbfs", append([]byte("WBFS\x00\x00\x00\x00\x09\x15"), make([]byte, 0x400)...)},
	} {
		t.Run(c.name, func(t *testing.T) {
			if f := sniff(c.data); f.Proven() {
				t.Fatalf("proved %s: %s", f.Console, f.Proof)
			}
		})
	}
	// The PSP's .cso still reads as one, though it starts like Dolphin's .ciso.
	if f := sniff(fixture.CSO(fixture.PSP("A Made Up Game", "ULUS10000"))); f.Console != "psp" {
		t.Fatalf("a PSP cso: %+v", f)
	}
}

func TestMangledNewerFilesDoNotPanic(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	gc := fixture.NintendoDisc("GXXE01", "Made Up", false)
	seeds := [][]byte{fixture.GCZ(gc), fixture.DolphinCISO(gc), fixture.WIA(gc[:0x80], 1, true), fixture.WBFS(gc[:0x100]),
		fixture.DOL(false), fixture.WAD(0x00010001, "WXXE"), fixture.WUX("AXXE"), fixture.WUA(), fixture.PS3("Made Up", "BLUS30000"),
		fixture.PKG(1, "UP0000-BLUS00000_00-X"), fixture.XBE("Made Up", 1), flat(fixture.XISO(0, "Made Up", 1), 0x20000),
		fixture.NSP("a.nca", "0100000000010000000000000000000a.tik"), fixture.XCI(), fixture.NRO("Made Up")}
	for _, seed := range seeds {
		for i := 0; i < 300; i++ {
			b := append([]byte(nil), seed...)
			for j := 0; j < 12; j++ {
				// Mostly in the headers, where the numbers that say where to look are.
				k := rng.Intn(min(len(b), 0x400))
				if j%3 == 0 {
					k = rng.Intn(len(b))
				}
				b[k] = byte(rng.Intn(256))
			}
			if i%5 == 0 {
				b = b[:rng.Intn(len(b))]
			}
			_ = sniff(b)
			_ = gameid.Sniff(sparse{size: 1 << 40, parts: map[int][]byte{0: b}}, 1<<40)
		}
	}
}
