// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Command biosdat writes internal/server/biosknown.txt: the BIOS files a games server can
// recognise by their checksum, wherever someone keeps them and whatever they're called.
// It reads libretro's System.dat (libretro-database, dat/System.dat), which lists firmware by
// size and checksum and nothing else: no firmware is fetched or kept.
//
//	go run ./scripts/biosdat path/to/System.dat > internal/server/biosknown.txt
//
// Only the files WittyPlay's consoles look for are kept, under the names their cores want,
// and every PlayStation 2 version, which LRPS2 takes under any name its rule accepts.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
)

// wanted is console, the name in System.dat, where it goes in the system folder, and its region.
var wanted = []struct{ console, name, dest, region string }{
	{"nes", "disksys.rom", "disksys.rom", ""},
	{"snes", "BS-X.bin", "BS-X.bin", "japan"},
	{"gba", "gba_bios.bin", "gba_bios.bin", ""},
	{"ps1", "scph5500.bin", "scph5500.bin", "japan"},
	{"ps1", "scph5501.bin", "scph5501.bin", "usa"},
	{"ps1", "scph5502.bin", "scph5502.bin", "europe"},
	{"pce", "syscard3.pce", "syscard3.pce", ""},
	{"a7800", "7800 BIOS (U).rom", "7800 BIOS (U).rom", "usa"},
	{"lynx", "lynxboot.img", "lynxboot.img", ""},
	{"nds", "bios7.bin", "bios7.bin", ""},
	{"nds", "bios9.bin", "bios9.bin", ""},
	{"dc", "dc/dc_boot.bin", "dc/dc_boot.bin", ""},
}

var (
	romLine  = regexp.MustCompile(`rom \( name ("[^"]+"|\S+) size (\d+) crc [0-9A-Fa-f]+ md5 ([0-9a-f]{32})`)
	ps2Name  = regexp.MustCompile(`^pcsx2/bios/ps2-(\d{4})([a-z])([a-z]?)-(\d{8})\.bin$`)
	ps2Where = map[string]string{"j": "japan", "a": "usa", "e": "europe", "h": "asia", "c": "china", "t": "taiwan", "r": "russia"}
)

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	byName := map[string]bool{}
	for _, w := range wanted {
		byName[w.name] = true
	}
	var out []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := romLine.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		name, size, md5 := strings.Trim(m[1], `"`), m[2], m[3]
		line := ""
		if p := ps2Name.FindStringSubmatch(name); p != nil {
			region := ps2Where[p[2]]
			if region == "" {
				region = "other"
			}
			// The rule LRPS2's BIOS is found by wants "bios" in the name; the region is in it
			// too, for the cores that choose by name.
			line = fmt.Sprintf("%s %s ps2 pcsx2/bios/ps2-bios-%s-%s %s", md5, size, region, strings.TrimPrefix(path.Base(name), "ps2-"), region)
		}
		for _, w := range wanted {
			if w.name == name {
				region := w.region
				if region == "" {
					region = "-"
				}
				line = fmt.Sprintf("%s %s %s %s %s", md5, size, w.console, strings.ReplaceAll(w.dest, " ", "%20"), region)
			}
		}
		if line != "" && !seen[md5] {
			seen[md5] = true
			out = append(out, line)
		}
	}
	sort.Strings(out)
	fmt.Println("# BIOS files recognised by checksum: md5, size, console, where it goes in the system folder")
	fmt.Println("# (%20 is a space), and its region or -. Made by scripts/biosdat from libretro's System.dat")
	fmt.Println("# (github.com/libretro/libretro-database). Nothing here is firmware, only how to know it.")
	for _, l := range out {
		fmt.Println(l)
	}
}
