// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package gameid

import (
	"bytes"
	"encoding/binary"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Switch files. Install packages (.nsp, and .nsz with their contents compressed) are a
// plain list of files; game card images (.xci, .xcz) have a header in the clear. What's
// inside both is encrypted with keys from the owner's own console, which only a
// standalone emulator given those keys can use. Nothing here reads past the plain lists
// and headers. Homebrew (.nro) isn't encrypted, and carries its name.

const switchKeys = " Its contents are encrypted with keys from a Switch console, so it plays in a standalone emulator given the keys from your own console."

// A ticket is named for its rights id, which starts with the title's id.
var ticketRe = regexp.MustCompile(`^([0-9a-fA-F]{16})[0-9a-fA-F]{16}\.tik$`)

// sniffPFS0 reads an install package's list of files: the contents (.nca, or .ncz when
// compressed), and a ticket whose name gives the title's id. A title id ending in 000 is a
// game, 800 its update, and anything else extra content.
func sniffPFS0(r io.ReaderAt, size int64) (Finding, bool) {
	h := at(r, 0, 16)
	if h == nil {
		return Finding{}, false
	}
	le := binary.LittleEndian
	n, names := int64(le.Uint32(h[4:])), int64(le.Uint32(h[8:]))
	table := 16 + n*24 + names
	if n <= 0 || n > 1024 || names <= 0 || table > 32<<10 || table > size {
		return Finding{Proof: "It starts like a Switch package, but its list of files makes no sense."}, true
	}
	b := at(r, 0, int(table))
	if b == nil {
		return Finding{Proof: "It starts like a Switch package, but it's cut short."}, true
	}
	strs := b[16+n*24:]
	var nca, ncz int
	titleID := ""
	for i := int64(0); i < n; i++ {
		off := int64(le.Uint32(b[16+i*24+16:]))
		if off >= int64(len(strs)) {
			continue
		}
		name := strs[off:]
		if end := bytes.IndexByte(name, 0); end >= 0 {
			name = name[:end]
		}
		s := strings.ToLower(string(name))
		switch {
		case strings.HasSuffix(s, ".nca"):
			nca++
		case strings.HasSuffix(s, ".ncz"):
			ncz++
		}
		if m := ticketRe.FindStringSubmatch(s); m != nil && titleID == "" {
			titleID = m[1]
		}
	}
	if nca+ncz == 0 {
		return Finding{Proof: "It's a Switch file system with no game contents in it."}, true
	}
	f := Finding{Console: Switch, Kind: KindGame, Encrypted: true, NeedsKeys: true, Proof: "It's a Switch install package (.nsp)." + switchKeys}
	if ncz > 0 {
		f.Proof = "It's a compressed Switch install package (.nsz)." + switchKeys
	}
	if titleID != "" {
		f.Serial = strings.ToUpper(titleID)
		if id, err := strconv.ParseUint(titleID, 16, 64); err == nil {
			switch {
			case id&0xFFF == 0x800:
				f.Kind, f.Proof = KindUpdate, "It's a Switch install package of an update for a game, not a game: its title id ends in 800."
			case id&0xFFF != 0:
				f.Kind, f.Proof = KindDLC, "It's a Switch install package of extra content for a game, not a game."
			}
		}
	}
	return f, true
}

// sniffSwitchHead recognises a game card image by its header, and homebrew by its mark.
func sniffSwitchHead(r io.ReaderAt, size int64, head []byte) (Finding, bool) {
	if len(head) >= 0x110 && string(head[0x100:0x104]) == "HEAD" {
		// The card's size code: only these are real cards.
		switch head[0x10D] {
		case 0xFA, 0xF8, 0xF0, 0xE0, 0xE1, 0xE2, 0xE3:
			return Finding{Console: Switch, Kind: KindGame, Encrypted: true, NeedsKeys: true, Proof: "It's a Switch game card image (.xci or .xcz)." + switchKeys}, true
		}
	}
	if len(head) >= 0x20 && string(head[0x10:0x14]) == "NRO0" {
		f := Finding{Console: Switch, Kind: KindGame, Proof: "It's a Switch homebrew program (.nro)."}
		f.Title = nroTitle(r, size, int64(binary.LittleEndian.Uint32(head[0x18:])))
		return f, true
	}
	return Finding{}, false
}

// nroTitle reads the name homebrew gives itself, in the assets after the program: its
// first entry, which is in English.
func nroTitle(r io.ReaderAt, size, program int64) string {
	if program <= 0 || program+0x38 > size {
		return ""
	}
	a := at(r, program, 0x38)
	if a == nil || string(a[:4]) != "ASET" {
		return ""
	}
	le := binary.LittleEndian
	off, n := int64(le.Uint64(a[0x18:])), int64(le.Uint64(a[0x20:]))
	if off <= 0 || n < 0x300 || off > size || program+off+0x200 > size {
		return ""
	}
	return titleIf(clean(at(r, program+off, 0x200)))
}
