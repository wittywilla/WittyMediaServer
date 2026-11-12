// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package gameid_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/gameid"
	"github.com/wittywilla/wittymediaserver/internal/gameid/fixture"
)

// A scan reads these from whatever's in a games library. Numbers inside them say where to look
// next, so what matters here is that looking ends soon.
func FuzzSniff(f *testing.F) {
	for _, seed := range [][]byte{fixture.PS1("SLUS_123.45"), fixture.PSP("Made Up", "ULUS10000"), fixture.CSO(fixture.PS2("SLUS_543.21")),
		fixture.CIA(fixture.NCCH(true, false, 1<<14), 0, 0x00040000), fixture.SFO([2]string{"TITLE", "x"}), []byte("CISO"), []byte("NCSD"),
		fixture.GCZ(fixture.NintendoDisc("GXXE01", "x", false)), fixture.WIA(nil, 2, true), fixture.WUX("AXXE"), fixture.XBE("x", 1),
		fixture.NSP("a.nca"), fixture.NRO("x"), fixture.PKG(1, "UP0000-BLUS00000_00-X")} {
		if len(seed) > 1<<16 {
			seed = seed[:1<<16]
		}
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<17 {
			return
		}
		start := time.Now()
		_ = gameid.Sniff(bytes.NewReader(b), int64(len(b)))
		_ = gameid.SniffHead(b, int64(len(b)))
		_ = gameid.Sniff(bytes.NewReader(b), 1<<45) // a file can say it's far larger than it is
		for _, console := range []string{"nes", "snes", "gb", "gba", "n64", "genesis", "ds"} {
			_, _ = gameid.CheckFor(console, bytes.NewReader(b), int64(len(b)))
		}
		if took := time.Since(start); took > 5*time.Second {
			t.Errorf("%d bytes took %v", len(b), took)
		}
	})
}

// What a sheet names has to stay in its folder, whatever it says.
func FuzzSheet(f *testing.F) {
	f.Add(".cue", `FILE "a.bin" BINARY`+"\n  TRACK 01 MODE1/2352\n")
	f.Add(".cue", `FILE "../../etc/passwd" BINARY`)
	f.Add(".gdi", "1\n1 0 4 2352 ../x.bin 0\n")
	f.Add(".m3u", "../a.chd\n/etc/passwd\n..\\b.cue\n")
	f.Add(".toc", `DATAFILE "a.bin"`)
	f.Add(".ccd", "[CloneCD]\n")
	f.Fuzz(func(t *testing.T, ext, in string) {
		if len(in) > 1<<16 {
			return
		}
		start := time.Now()
		// In this system's own form: on Windows the files come back as \lib\games\a.bin.
		root := filepath.FromSlash("/lib/games")
		s, err := gameid.ParseSheet(ext, "game"+ext, strings.NewReader(in), "/lib/games", func(string) bool { return true })
		if err == nil {
			for _, p := range s.Files {
				if !strings.HasPrefix(p, root+string(filepath.Separator)) && p != root {
					t.Errorf("%q leads to %s", in, p)
				}
			}
		}
		if took := time.Since(start); took > 5*time.Second {
			t.Errorf("%d bytes took %v", len(in), took)
		}
	})
}
