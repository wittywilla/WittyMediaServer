// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package gameid

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"regexp"
)

// GameCube, Wii and Wii U files. GameCube and Wii discs start with the same header: a
// six-letter game id, a magic number that says which of the two it is, and the game's
// name. Every compressed format Dolphin reads keeps a copy of that header somewhere it
// can be found without decompressing the rest, so the same check proves them all.
//
// Wii discs are encrypted, but with a key every copy of Dolphin has, and the header
// itself is in the clear: to a person they're as playable as any disc. Wii U discs are
// encrypted with keys from the owner's own console, which only a standalone emulator
// given those keys can use.

const (
	wiiMagic  = 0x5D1C9EA3 // at 0x18
	gcMagic   = 0xC2339F3D // at 0x1C
	gczMagic  = 0xB10BC001 // little-endian, at 0
	wuxMagic  = 0x1099D02E // little-endian, at 4
	wuaMagic  = 0x169F52D6 // the last four bytes of a Cemu archive
	wuaFormat = 0x61BF3A01 // the four before them: version 1
	// discHeaderLen is how much of a disc's header is read: its name ends well before.
	discHeaderLen = 0x400
)

var (
	gameIDRe  = regexp.MustCompile(`^[A-Z0-9]{6}$`)
	wadCodeRe = regexp.MustCompile(`^[A-Z0-9]{4}$`)
)

// nintendoDisc reads a GameCube or Wii disc's header. b is its first bytes, at least
// 0x20 of them; the name is read from what there is past that.
func nintendoDisc(b []byte) (Finding, bool) {
	if len(b) < 0x20 {
		return Finding{}, false
	}
	be := binary.BigEndian
	var f Finding
	switch {
	case be.Uint32(b[0x18:]) == wiiMagic:
		f = Finding{Console: Wii, Kind: KindGame, Proof: "It has the magic number of a Wii disc."}
	case be.Uint32(b[0x1C:]) == gcMagic:
		f = Finding{Console: GameCube, Kind: KindGame, Proof: "It has the magic number of a GameCube disc."}
	default:
		return Finding{}, false
	}
	if id := string(b[:6]); gameIDRe.MatchString(id) {
		f.Serial, f.Region = id, nintendoRegion(id[3])
	}
	if len(b) > 0x20 {
		f.Title = titleIf(clean(b[0x20:min(len(b), discHeaderLen)]))
	}
	return f, true
}

// nintendoRegion reads the region letter of a GameCube, Wii or Wii U game id. Most are
// the same as a cartridge's; discs added a few for Europe and Australia.
func nintendoRegion(c byte) string {
	switch c {
	case 'U':
		return "Australia"
	case 'X', 'Y', 'Z', 'L', 'M':
		return "Europe"
	case 'W':
		return "Taiwan"
	}
	return cartRegion(c)
}

// inside marks a finding as read from inside a compressed or packed image.
func inside(f Finding, what string) Finding {
	f.Proof = "It's " + what + ". " + f.Proof
	return f
}

// sniffDolphinCISO reads Dolphin's .ciso: a block size, a map of which blocks are
// stored, and the stored blocks from 0x8000 on. A PSP .cso has a header size where the
// block size would be, which is never a power of two this big, so the two can't mix up.
func sniffDolphinCISO(r io.ReaderAt, size int64) (Finding, bool) {
	h := at(r, 0, 0x48)
	if h == nil {
		return Finding{}, false
	}
	block := binary.LittleEndian.Uint32(h[4:])
	if block < 0x800 || block > 1<<26 || block&(block-1) != 0 {
		return Finding{}, false
	}
	for _, used := range h[8:] {
		if used > 1 {
			return Finding{}, false
		}
	}
	if h[8] != 1 || size <= 0x8000 {
		return Finding{}, false
	}
	b := at(r, 0x8000, int(min(discHeaderLen, size-0x8000)))
	if f, ok := nintendoDisc(b); ok {
		return inside(f, "a compressed disc image (.ciso)"), true
	}
	return Finding{}, false
}

// sniffGCZ reads Dolphin's .gcz: a table of where each block starts, then the blocks,
// each compressed on its own. Only the first block is read, for the header.
func sniffGCZ(r io.ReaderAt, size int64) Finding {
	bad := Finding{Proof: "It starts like a compressed GameCube or Wii disc (.gcz), but its header makes no sense."}
	h := at(r, 0, 0x20)
	if h == nil {
		return bad
	}
	le := binary.LittleEndian
	compressed, block, blocks := le.Uint64(h[8:]), int64(le.Uint32(h[0x18:])), int64(le.Uint32(h[0x1C:]))
	if block <= 0 || block > maxCompressedBlock || blocks <= 0 || blocks > 1<<24 {
		return bad
	}
	dataAt := 0x20 + blocks*12 // a pointer and a checksum for each block
	if dataAt >= size {
		return bad
	}
	ptrs := at(r, 0x20, 16)
	if blocks == 1 {
		ptrs = at(r, 0x20, 8)
	}
	if ptrs == nil {
		return bad
	}
	const plainBit = 1 << 63
	start := le.Uint64(ptrs) &^ plainBit
	end := compressed
	if blocks > 1 {
		end = le.Uint64(ptrs[8:]) &^ plainBit
	}
	if end <= start || end-start > 2*maxCompressedBlock || int64(start) > size-dataAt {
		return bad
	}
	raw := at(r, dataAt+int64(start), int(end-start))
	if raw == nil {
		return bad
	}
	out := raw
	if le.Uint64(ptrs)&plainBit == 0 {
		z, err := zlib.NewReader(bytes.NewReader(raw))
		if err != nil {
			return bad
		}
		out, _ = io.ReadAll(io.LimitReader(z, discHeaderLen))
	}
	if f, ok := nintendoDisc(out); ok {
		return inside(f, "a compressed disc image (.gcz)")
	}
	return Finding{Proof: "It's a compressed GameCube or Wii disc image (.gcz), but the disc inside has no header."}
}

// sniffWIA reads a .wia or .rvz. Its second header keeps the disc's first 0x80 bytes as
// they are, uncompressed, at 0x58, and says itself whether the disc is a GameCube's or
// a Wii's.
func sniffWIA(r io.ReaderAt, head []byte) Finding {
	what := "a compressed disc image (.wia)"
	if bytes.HasPrefix(head, []byte("RVZ")) {
		what = "a compressed disc image (.rvz)"
	}
	b := at(r, 0x48, 0x10+0x80)
	if b == nil {
		return Finding{Proof: "It starts like " + what + ", but it's cut short."}
	}
	if f, ok := nintendoDisc(b[0x10:]); ok {
		return inside(f, what)
	}
	switch binary.BigEndian.Uint32(b) {
	case 1:
		return Finding{Console: GameCube, Kind: KindGame, Proof: "It's " + what + " that says it holds a GameCube disc."}
	case 2:
		return Finding{Console: Wii, Kind: KindGame, Proof: "It's " + what + " that says it holds a Wii disc."}
	}
	return Finding{Proof: "It's " + what + ", with nothing in it that says which console it's for."}
}

// sniffWBFS reads a .wbfs, the format Wii loaders keep games in. Its first sector says
// how big its sectors are; the second holds a copy of the first game's disc header.
func sniffWBFS(r io.ReaderAt) Finding {
	h := at(r, 0, 16)
	if h == nil {
		return Finding{Proof: "It starts like a Wii game (.wbfs), but it's cut short."}
	}
	shift := h[8]
	if shift < 9 || shift > 16 {
		return Finding{Proof: "It starts like a Wii game (.wbfs), but its header makes no sense."}
	}
	if h[12] == 0 {
		return Finding{Kind: KindOther, Proof: "It's an empty Wii game container (.wbfs)."}
	}
	if f, ok := nintendoDisc(at(r, 1<<shift, 0x100)); ok {
		return inside(f, "a Wii game container (.wbfs)")
	}
	return Finding{Proof: "It's a Wii game container (.wbfs), but the game's header in it makes no sense."}
}

// sniffDOL reads a GameCube or Wii program (.dol), which homebrew comes as. Its header is
// nothing but where each of its sections goes, so what proves one is that every section
// fits in the file and loads into the console's memory. Programs that load into the
// Wii's second bank of memory are Wii programs; the rest run on either.
func sniffDOL(r io.ReaderAt, size int64) (Finding, bool) {
	if size < 0x200 || size > 64<<20 {
		return Finding{}, false
	}
	h := at(r, 0, 0x100)
	if h == nil {
		return Finding{}, false
	}
	for _, c := range h[0xE4:] {
		if c != 0 {
			return Finding{}, false
		}
	}
	be := binary.BigEndian
	inMem1 := func(a uint32) bool { return a >= 0x80000000 && a < 0x81800000 }
	inMem2 := func(a uint32) bool { return a >= 0x90000000 && a < 0x94000000 }
	entry := be.Uint32(h[0xE0:])
	wii, text, entryFound := false, 0, false
	for i := 0; i < 18; i++ {
		off, addr, n := be.Uint32(h[i*4:]), be.Uint32(h[0x48+i*4:]), be.Uint32(h[0x90+i*4:])
		if n == 0 {
			continue
		}
		if off < 0x100 || int64(off)+int64(n) > size || !(inMem1(addr) || inMem2(addr)) || uint64(addr)+uint64(n) > 0x94000000 {
			return Finding{}, false
		}
		wii = wii || inMem2(addr)
		if i < 7 {
			text++
			entryFound = entryFound || (entry >= addr && entry < addr+n)
		}
	}
	if text == 0 || !entryFound {
		return Finding{}, false
	}
	if wii {
		return Finding{Console: Wii, Kind: KindGame, Proof: "It's a Wii program (.dol): its sections fit the file and load into the Wii's memory."}, true
	}
	return Finding{Console: GameCube, Kind: KindGame, Proof: "It's a GameCube program (.dol): its sections fit the file and load into the console's memory. Wii homebrew comes like this too."}, true
}

// sniffWAD reads a Wii install package (.wad), as WiiWare and Virtual Console games come.
// Its header gives the size of each part; the title's id, in the part after the ticket,
// says what sort of title it is and, for a game, its four-letter code.
func sniffWAD(r io.ReaderAt, size int64, head []byte) (Finding, bool) {
	if len(head) < 0x20 {
		return Finding{}, false
	}
	be := binary.BigEndian
	if be.Uint32(head) != 0x20 || !(bytes.Equal(head[4:6], []byte("Is")) || bytes.Equal(head[4:6], []byte("ib"))) {
		return Finding{}, false
	}
	certs, crl, tik, tmd := int64(be.Uint32(head[8:])), int64(be.Uint32(head[0x0C:])), int64(be.Uint32(head[0x10:])), int64(be.Uint32(head[0x14:]))
	if certs < 0x100 || certs > 0x10000 || crl > 0x10000 || tik != 0x2A4 || tmd < 0x1E4 || tmd > 0x10000 {
		return Finding{}, false
	}
	f := Finding{Console: Wii, Kind: KindGame, Proof: "It's a Wii install package (.wad)."}
	tmdAt := align64(0x20) + align64(certs) + align64(crl) + align64(tik)
	if tmdAt+0x194 > size {
		return f, true
	}
	id := at(r, tmdAt+0x18C, 8)
	if id == nil {
		return f, true
	}
	switch be.Uint32(id) {
	case 0x00010001: // channels: WiiWare and Virtual Console
		if code := string(id[4:]); wadCodeRe.MatchString(code) {
			f.Serial, f.Region = code, nintendoRegion(code[3])
		}
	case 0x00010005:
		f.Kind, f.Proof = KindDLC, "It's a Wii install package of extra content for a game, not a game."
	default:
		f.Kind, f.Proof = KindUtility, "It's a Wii install package of system software, not a game."
	}
	return f, true
}

var wiiUCodeRe = regexp.MustCompile(`^WUP-[A-Z]-([A-Z0-9]{4})`)

// sniffWiiU recognises a Wii U disc image (.wud), which starts with its product code,
// and a Wii U program (.rpx or .rpl), an ELF file marked with Cafe OS's ABI.
func sniffWiiU(head []byte) (Finding, bool) {
	if m := wiiUCodeRe.FindSubmatch(head); m != nil {
		code := string(m[1])
		return Finding{Console: WiiU, Kind: KindGame, Serial: code, Region: nintendoRegion(code[3]), Encrypted: true, NeedsKeys: true,
			Proof: fmt.Sprintf("It's a Wii U disc image, %s. Its contents are encrypted with keys from a Wii U console, so it plays in a standalone emulator given the keys from your own console.", head[:10])}, true
	}
	if len(head) >= 0x14 && bytes.HasPrefix(head, []byte("\x7fELF\x01\x02\x01")) && head[7] == 0xCA && head[8] == 0xFE {
		return Finding{Console: WiiU, Kind: KindGame, Proof: "It's a Wii U program (.rpx): its header carries the Wii U's own mark."}, true
	}
	return Finding{}, false
}

// sniffWUX reads a compressed Wii U disc image (.wux): a header, a table of which stored
// sector each disc sector is, then the sectors. The disc's first sector is found
// through the table, for its product code.
func sniffWUX(r io.ReaderAt, size int64) Finding {
	f := Finding{Console: WiiU, Kind: KindGame, Encrypted: true, NeedsKeys: true,
		Proof: "It's a compressed Wii U disc image (.wux). Its contents are encrypted with keys from a Wii U console, so it plays in a standalone emulator given the keys from your own console."}
	h := at(r, 0, 0x24)
	if h == nil {
		return f
	}
	le := binary.LittleEndian
	sector, disc := int64(le.Uint32(h[8:])), int64(le.Uint64(h[0x10:]))
	if sector < 0x100 || sector > 1<<20 || sector&(sector-1) != 0 || disc <= 0 || disc > 1<<40 {
		return f
	}
	n := (disc + sector - 1) / sector
	data := (0x20 + n*4 + sector - 1) &^ (sector - 1)
	first := int64(le.Uint32(h[0x20:]))
	if first >= n || data+first*sector >= size {
		return f
	}
	if m := wiiUCodeRe.FindSubmatch(at(r, data+first*sector, 16)); m != nil {
		f.Serial = string(m[1])
		f.Region = nintendoRegion(f.Serial[3])
	}
	return f
}

// sniffWUA recognises a Cemu archive (.wua), which holds a Wii U game already decrypted.
// It's marked at its very end.
func sniffWUA(r io.ReaderAt, size int64) (Finding, bool) {
	if size < 0x90 {
		return Finding{}, false
	}
	t := at(r, size-16, 16)
	if t == nil {
		return Finding{}, false
	}
	be := binary.BigEndian
	if be.Uint32(t[12:]) != wuaMagic || be.Uint32(t[8:]) != wuaFormat || int64(be.Uint64(t)) != size {
		return Finding{}, false
	}
	return Finding{Console: WiiU, Kind: KindGame, Proof: "It's a Cemu archive (.wua) of a Wii U game, which is kept decrypted."}, true
}
