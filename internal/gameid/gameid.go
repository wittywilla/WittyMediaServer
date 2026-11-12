// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package gameid works out what a game file is from its own bytes: which console it
// belongs to, whether it's a game at all, what it calls itself, and how sure that is.
//
// A file's extension is only a hint, and on old formats a poor one: .bin and .iso mean
// half a dozen consoles, .md is usually a text file, .dmg a Mac disk image and .vb
// source code. So nothing here trusts one. Every check reads a few fixed places in the
// file, and says in plain words what it found, so that a person can see why.
//
// Files are hostile until proven otherwise. Reads are small and bounded, sizes in a
// file are checked before they're used, and nothing is ever decrypted: an encrypted
// file is only recognised as one, from what its headers say in the open.
//
// The package knows consoles only by their ids, and has no dependencies beyond the
// standard library, so anything can use it.
package gameid

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"unicode/utf8"
)

// Kind is what sort of thing a file is.
type Kind string

const (
	// KindUnknown: nothing here could say.
	KindUnknown Kind = ""
	// KindGame is something a person plays.
	KindGame Kind = "game"
	// KindBIOS is a console's or a copier's start-up software.
	KindBIOS Kind = "bios"
	// KindUpdate patches a game someone already has.
	KindUpdate Kind = "update"
	// KindDLC is extra content for a game.
	KindDLC Kind = "dlc"
	// KindUtility is a test cartridge, a cheat device, a menu: it runs, but isn't a game.
	KindUtility Kind = "utility"
	// KindAudio is a music CD, a chiptune, or one audio track of a disc.
	KindAudio Kind = "audio"
	// KindData is one piece of a game that can't stand alone.
	KindData Kind = "data"
	// KindOther is anything else: text, a PC program, a film disc.
	KindOther Kind = "other"
)

// Consoles this package recognises that the server may not be able to play. They're
// still worth naming: "this is a Saturn disc" beats "this doesn't look like a game".
const (
	Sega32X  = "32x"
	SegaCD   = "segacd"
	Saturn   = "saturn"
	GameCube = "gc"
	Wii      = "wii"
	WiiU     = "wiiu"
	PS3      = "ps3"
	Xbox     = "xbox"
	Switch   = "switch"
	ThreeDO  = "3do"
	PCFX     = "pcfx"
	NeoGeoCD = "neogeocd"
	CDI      = "cdi"
)

// Finding is what a file's bytes say about it.
type Finding struct {
	// Console is the console the bytes prove, or "".
	Console string
	Kind    Kind
	// Proof says what was seen, in words for a person.
	Proof string
	// Title and Serial are what the file calls itself, Region where it says it's from.
	Title, Serial, Region string
	// Encrypted: this file's headers say it's encrypted, or its contents can't be read
	// the way they would be if they weren't. 3DS, Switch and Wii U files can be.
	Encrypted bool
	// NeedsKeys: it's encrypted, and that's how it's meant to be played. A standalone
	// emulator decrypts it with keys the owner copied from their own console, so it's a
	// game to list, not one that can't play. Only set along with Encrypted.
	NeedsKeys bool
	// NewOnly: a 3DS game that only runs on a New 3DS.
	NewOnly bool
	// Text: this is a text file, whatever it's called.
	Text bool
	// Media is what a compressed disc image says it holds: cd, dvd or gd.
	Media string
}

// Proven says the bytes settled which console this is for.
func (f Finding) Proven() bool { return f.Console != "" }

// at reads n bytes at off, or nil if the file doesn't have them.
func at(r io.ReaderAt, off int64, n int) []byte {
	if off < 0 || n <= 0 {
		return nil
	}
	b := make([]byte, n)
	if _, err := r.ReadAt(b, off); err != nil {
		return nil
	}
	return b
}

// clean makes a field from inside a file safe to show: printable, trimmed and short.
// Bytes outside ASCII are dropped, as old consoles use their own character sets.
func clean(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	var s strings.Builder
	for _, c := range b {
		if c >= 0x20 && c < 0x7f {
			s.WriteByte(c)
		} else {
			s.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(s.String()), " ")
}

// usableTitle says whether a header's title is worth searching for. Plenty hold
// nothing, or a developer's placeholder.
func usableTitle(s string) bool {
	if len(s) < 3 {
		return false
	}
	letters := 0
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			letters++
		}
	}
	return letters >= 2
}

// looksLikeText says whether the start of a file is text: no control bytes, and
// valid UTF-8 or close to it. A ROM has zeros and vectors within its first few KB.
func looksLikeText(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	// A cut-off character at the end of the sample doesn't count against it.
	for i := 0; i < 4 && len(b) > 0 && !utf8.Valid(b); i++ {
		b = b[:len(b)-1]
	}
	if !utf8.Valid(b) {
		return false
	}
	for _, c := range b {
		if c < 0x20 && c != '\n' && c != '\r' && c != '\t' && c != '\f' {
			return false
		}
		if c == 0x7f {
			return false
		}
	}
	return true
}

// Sniff reads a file and says what it is. It costs a handful of small reads, plus one
// or two compressed blocks for a compressed disc.
func Sniff(r io.ReaderAt, size int64) Finding {
	head := make([]byte, min(size, 0x200))
	if n, _ := r.ReadAt(head, 0); int64(n) < int64(len(head)) {
		head = head[:n]
	}
	if len(head) == 0 {
		return Finding{Kind: KindOther, Proof: "The file is empty."}
	}
	// Containers first: what's inside them decides.
	switch {
	case bytes.HasPrefix(head, []byte("CISO")):
		// Dolphin's .ciso and the PSP's .cso share a mark, and differ in what follows it.
		if f, ok := sniffDolphinCISO(r, size); ok {
			return f
		}
		return sniffCompressedISO(r, size)
	case bytes.HasPrefix(head, []byte("ZISO")):
		return sniffCompressedISO(r, size)
	case bytes.HasPrefix(head, []byte("\x00PBP")):
		return sniffPBP(r, size)
	case bytes.HasPrefix(head, []byte("MComprHD")):
		return sniffCHD(r, size)
	}
	if f, ok := sniffNewer(r, size, head); ok {
		return f
	}
	if f, ok := sniff3DS(r, size); ok {
		return f
	}
	if f, ok := sniffCartridge(r, size, head); ok {
		return f
	}
	if f, ok := sniffDisc(r, size); ok {
		return f
	}
	if f, ok := sniffSNES(r, size); ok {
		return f
	}
	if f, ok := sniffBIOS(r, size); ok {
		return f
	}
	if f, ok := sniffDOL(r, size); ok {
		return f
	}
	sample := make([]byte, min(size, 4096))
	n, _ := r.ReadAt(sample, 0)
	if looksLikeText(sample[:n]) {
		return Finding{Kind: KindOther, Text: true, Proof: "It's a text file."}
	}
	if f, ok := sniffNotGame(sample[:n]); ok {
		return f
	}
	return Finding{}
}

// sniffNewer recognises the files of consoles from the GameCube on: compressed and
// packed disc images, install packages and programs. Each has a mark at a fixed place,
// so this costs a read or two of a few hundred bytes.
func sniffNewer(r io.ReaderAt, size int64, head []byte) (Finding, bool) {
	switch {
	case bytes.HasPrefix(head, []byte("WIA\x01")), bytes.HasPrefix(head, []byte("RVZ\x01")):
		return sniffWIA(r, head), true
	case len(head) >= 4 && binary.LittleEndian.Uint32(head) == gczMagic:
		return sniffGCZ(r, size), true
	case bytes.HasPrefix(head, []byte("WBFS")):
		return sniffWBFS(r), true
	case bytes.HasPrefix(head, []byte("WUX0")) && len(head) >= 8 && binary.LittleEndian.Uint32(head[4:]) == wuxMagic:
		return sniffWUX(r, size), true
	case bytes.HasPrefix(head, []byte("XBEH")):
		return sniffXBE(r, size), true
	case bytes.HasPrefix(head, []byte("PFS0")):
		return sniffPFS0(r, size)
	case bytes.HasPrefix(head, []byte("\x7fPKG")):
		return sniffPKG(head)
	}
	if f, ok := sniffWiiU(head); ok {
		return f, true
	}
	if f, ok := sniffWAD(r, size, head); ok {
		return f, true
	}
	if f, ok := sniffSwitchHead(r, size, head); ok {
		return f, true
	}
	if f, ok := sniffWUA(r, size); ok {
		return f, true
	}
	return Finding{}, false
}

// SniffHead is Sniff for the first bytes of a file that can't be read at will, such as
// one inside an archive. What's past the end of b counts as not there, so size is what
// the file really is, not len(b).
func SniffHead(b []byte, size int64) Finding {
	return Sniff(prefixReader(b), max(size, int64(len(b))))
}

// prefixReader is the start of a file, with the rest unreadable.
type prefixReader []byte

// ReadAt implements io.ReaderAt.
func (p prefixReader) ReadAt(b []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(p)) {
		return 0, io.EOF
	}
	n := copy(b, p[off:])
	if n < len(b) {
		return n, io.EOF
	}
	return n, nil
}

// sniffNotGame names common files that end up in game folders. A person wants to know
// a setup program isn't a game more than that nothing could be said about it.
func sniffNotGame(b []byte) (Finding, bool) {
	other := func(what string) (Finding, bool) {
		return Finding{Kind: KindOther, Proof: "It's " + what + "."}, true
	}
	switch {
	case bytes.HasPrefix(b, []byte("MZ")):
		return other("a Windows program")
	case bytes.HasPrefix(b, []byte("%PDF")):
		return other("a PDF document")
	case bytes.HasPrefix(b, []byte("\x89PNG")), bytes.HasPrefix(b, []byte("\xff\xd8\xff")), bytes.HasPrefix(b, []byte("GIF8")):
		return other("a picture")
	case bytes.HasPrefix(b, []byte("fLaC")), bytes.HasPrefix(b, []byte("ID3")), bytes.HasPrefix(b, []byte("OggS")):
		return Finding{Kind: KindAudio, Proof: "It's a sound file."}, true
	case bytes.HasPrefix(b, []byte("RIFF")) && len(b) > 12 && string(b[8:12]) == "WAVE":
		return Finding{Kind: KindAudio, Proof: "It's a sound file."}, true
	case bytes.HasPrefix(b, []byte("\xcf\xfa\xed\xfe")), bytes.HasPrefix(b, []byte("\xca\xfe\xba\xbe")):
		return other("a program for a Mac")
	}
	return Finding{}, false
}

// serialRegion says where a PlayStation product code is from: SLUS is the Americas,
// SLES Europe, SLPS Japan, and so on.
func serialRegion(serial string) string {
	if len(serial) < 3 {
		return ""
	}
	switch serial[2] {
	case 'U':
		return "USA"
	case 'E':
		return "Europe"
	case 'P', 'J':
		return "Japan"
	case 'K':
		return "Korea"
	case 'A':
		return "Asia"
	}
	return ""
}
