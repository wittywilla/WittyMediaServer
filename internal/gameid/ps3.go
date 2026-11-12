// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package gameid

import (
	"encoding/binary"
	"regexp"
)

// PlayStation 3 files. A disc is ISO 9660 with PS3_DISC.SFB in its top folder, and a
// PARAM.SFO in PS3_GAME naming the game; that's read in sniffISO with the other Sony
// discs. A store download (.pkg) is an install package: RPCS3 installs it, and the game
// is then played from where it was installed. Its header names its content in the clear.

// A content id is a publisher code, then the title id: UP0001-BLUS30443_00-....
var contentIDRe = regexp.MustCompile(`^[A-Z]{2}[0-9]{4}-([A-Z]{4}[0-9]{5})_`)

func sniffPKG(head []byte) (Finding, bool) {
	if len(head) < 0x54 {
		return Finding{Proof: "It starts like a PlayStation install package, but it's cut short."}, true
	}
	if binary.BigEndian.Uint16(head[6:]) != 1 {
		return Finding{Proof: "It's a PlayStation Portable or Vita install package."}, true
	}
	f := Finding{Console: PS3, Kind: KindGame, Proof: "It's a PlayStation 3 install package (.pkg). RPCS3 installs it, and plays the game from there."}
	if m := contentIDRe.FindSubmatch(head[0x30:0x54]); m != nil {
		f.Serial = normalPSP(string(m[1]))
		f.Region = serialRegion(f.Serial)
	}
	return f, true
}

// applyPS3SFO fills in what a PlayStation 3 disc's PARAM.SFO says.
func applyPS3SFO(f *Finding, b []byte) {
	sfo := parseSFO(b)
	if t := sfo["TITLE"]; usableTitle(t) {
		f.Title = t
	}
	if id := sfo["TITLE_ID"]; id != "" {
		f.Serial = normalPSP(id)
		f.Region = serialRegion(f.Serial)
	}
}
