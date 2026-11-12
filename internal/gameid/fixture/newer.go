// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package fixture

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"unicode/utf16"
)

// Files for the consoles from the GameCube on. As with the rest, only the marks and
// headers are real; everything else is zeros.

// NintendoDisc is the start of a GameCube disc, or with wii a Wii one.
func NintendoDisc(id, title string, wii bool) []byte {
	b := make([]byte, 0x10000)
	copy(b, id)
	if wii {
		binary.BigEndian.PutUint32(b[0x18:], 0x5D1C9EA3)
	} else {
		binary.BigEndian.PutUint32(b[0x1C:], 0xC2339F3D)
	}
	copy(b[0x20:], title)
	return b
}

// DolphinCISO packs a disc the way Dolphin's .ciso does: every block stored.
func DolphinCISO(disc []byte) []byte {
	const block = 0x8000
	b := make([]byte, 0x8000)
	copy(b, "CISO")
	binary.LittleEndian.PutUint32(b[4:], block)
	for i := 0; i*block < len(disc); i++ {
		b[8+i] = 1
	}
	return append(b, disc...)
}

// GCZ compresses a disc the way Dolphin's .gcz does: one zlib stream per block.
func GCZ(disc []byte) []byte {
	const block = 0x4000
	n := (len(disc) + block - 1) / block
	var data bytes.Buffer
	ptrs := make([]byte, n*8)
	for i := 0; i < n; i++ {
		binary.LittleEndian.PutUint64(ptrs[i*8:], uint64(data.Len()))
		w := zlib.NewWriter(&data)
		_, _ = w.Write(disc[i*block : min((i+1)*block, len(disc))])
		_ = w.Close()
	}
	h := make([]byte, 0x20)
	binary.LittleEndian.PutUint32(h, 0xB10BC001)
	binary.LittleEndian.PutUint64(h[8:], uint64(data.Len()))
	binary.LittleEndian.PutUint64(h[0x10:], uint64(len(disc)))
	binary.LittleEndian.PutUint32(h[0x18:], block)
	binary.LittleEndian.PutUint32(h[0x1C:], uint32(n))
	out := append(h, ptrs...)
	out = append(out, make([]byte, n*4)...) // checksums
	return append(out, data.Bytes()...)
}

// WIA makes the headers of a .wia, or with rvz a .rvz, holding a disc whose first bytes
// are header. discType is 1 for a GameCube disc and 2 for a Wii one.
func WIA(header []byte, discType uint32, rvz bool) []byte {
	b := make([]byte, 0x200)
	copy(b, "WIA\x01")
	if rvz {
		copy(b, "RVZ\x01")
	}
	binary.BigEndian.PutUint32(b[0x48:], discType)
	copy(b[0x58:0xD8], header)
	return b
}

// WBFS puts a Wii disc's header into a .wbfs with 512-byte sectors.
func WBFS(header []byte) []byte {
	b := make([]byte, 0x10000)
	copy(b, "WBFS")
	b[8], b[9], b[12] = 9, 21, 1
	copy(b[0x200:0x300], header)
	return b
}

// DOL is a GameCube program with one text section, or with wii one that loads into the
// Wii's second bank of memory.
func DOL(wii bool) []byte {
	b := make([]byte, 0x1000)
	addr := uint32(0x80003100)
	if wii {
		addr = 0x90000000
	}
	binary.BigEndian.PutUint32(b[0x00:], 0x100)
	binary.BigEndian.PutUint32(b[0x48:], addr)
	binary.BigEndian.PutUint32(b[0x90:], 0x800)
	binary.BigEndian.PutUint32(b[0xE0:], addr)
	return b
}

// WAD is a Wii install package whose title has this type (the high half of its id) and
// four-letter code.
func WAD(titleType uint32, code string) []byte {
	const certs, tik, tmd = 0xA00, 0x2A4, 0x208
	b := make([]byte, 0x4000)
	binary.BigEndian.PutUint32(b, 0x20)
	copy(b[4:], "Is")
	binary.BigEndian.PutUint32(b[8:], certs)
	binary.BigEndian.PutUint32(b[0x10:], tik)
	binary.BigEndian.PutUint32(b[0x14:], tmd)
	tmdAt := 0x40 + certs + align64(tik)
	binary.BigEndian.PutUint32(b[tmdAt+0x18C:], titleType)
	copy(b[tmdAt+0x190:], code)
	return b
}

// WUD is the start of a Wii U disc image.
func WUD(code string) []byte {
	b := make([]byte, 0x8000)
	copy(b, "WUP-P-"+code+"-0-00")
	return b
}

// WUX compresses that the way a .wux does, with the disc's first sector stored second.
func WUX(code string) []byte {
	const sector = 0x8000
	h := make([]byte, sector)
	copy(h, "WUX0")
	binary.LittleEndian.PutUint32(h[4:], 0x1099D02E)
	binary.LittleEndian.PutUint32(h[8:], sector)
	binary.LittleEndian.PutUint64(h[0x10:], 2*sector)
	binary.LittleEndian.PutUint32(h[0x20:], 1) // disc sector 0 is stored sector 1
	binary.LittleEndian.PutUint32(h[0x24:], 0)
	out := append(h, make([]byte, sector)...)
	return append(out, WUD(code)...)
}

// RPX is the header of a Wii U program.
func RPX() []byte {
	b := make([]byte, 0x400)
	copy(b, "\x7fELF\x01\x02\x01\xca\xfe")
	binary.BigEndian.PutUint16(b[0x10:], 0xFE01)
	return b
}

// WUA is an empty Cemu archive: only its footer.
func WUA() []byte {
	b := make([]byte, 0x400)
	be := binary.BigEndian
	be.PutUint64(b[len(b)-16:], uint64(len(b)))
	be.PutUint32(b[len(b)-8:], 0x61BF3A01)
	be.PutUint32(b[len(b)-4:], 0x169F52D6)
	return b
}

// PS3 is a PlayStation 3 disc: PS3_DISC.SFB at the top, and PS3_GAME/PARAM.SFO.
func PS3(title, id string) []byte {
	sfo := SFO([2]string{"TITLE", title}, [2]string{"TITLE_ID", id}, [2]string{"CATEGORY", "DG"})
	b := make([]byte, sector(40))
	pvd := b[sector(16):sector(17)]
	pvd[0] = 1
	copy(pvd[1:], "CD001")
	pvd[6] = 1
	copy(pvd[8:], "PS3VOLUME")
	root := bytes.Buffer{}
	root.Write(dirRecord("\x00", 18, 2048, true))
	root.Write(dirRecord("\x01", 18, 2048, true))
	root.Write(dirRecord("PS3_DISC.SFB;1", 24, 16, false))
	root.Write(dirRecord("PS3_GAME", 20, 2048, true))
	copy(b[sector(18):], root.Bytes())
	sub := bytes.Buffer{}
	sub.Write(dirRecord("\x00", 20, 2048, true))
	sub.Write(dirRecord("\x01", 18, 2048, true))
	sub.Write(dirRecord("PARAM.SFO;1", 22, uint32(len(sfo)), false))
	copy(b[sector(20):], sub.Bytes())
	copy(b[sector(22):], sfo)
	copy(b[sector(24):], ".SFB")
	copy(pvd[156:], dirRecord("\x00", 18, 2048, true))
	return b
}

// PKG is the header of a PlayStation install package: kind 1 for a PS3's, 2 for a PSP's.
func PKG(kind uint16, contentID string) []byte {
	b := make([]byte, 0x400)
	copy(b, "\x7fPKG")
	binary.BigEndian.PutUint16(b[4:], 0x8000)
	binary.BigEndian.PutUint16(b[6:], kind)
	copy(b[0x30:], contentID)
	return b
}

// XISO is an Xbox disc whose top folder holds default.xbe. The game's file system
// starts at base: 0 for an .xiso, or where a full disc image has it.
func XISO(base int, title string, titleID uint32) map[int][]byte {
	vol := make([]byte, 0x20)
	copy(vol, "MICROSOFT*XBOX*MEDIA")
	binary.LittleEndian.PutUint32(vol[0x14:], 40) // root folder's sector
	binary.LittleEndian.PutUint32(vol[0x18:], 2048)
	dir := make([]byte, 2048)
	for i := range dir {
		dir[i] = 0xFF
	}
	// One other file sorts before it, so finding it takes a step right.
	entry := func(at int, right uint16, start uint32, name string) {
		binary.LittleEndian.PutUint16(dir[at:], 0)
		binary.LittleEndian.PutUint16(dir[at+2:], right)
		binary.LittleEndian.PutUint32(dir[at+4:], start)
		binary.LittleEndian.PutUint32(dir[at+8:], 0x1000)
		dir[at+12] = 0x20
		dir[at+13] = byte(len(name))
		copy(dir[at+14:], name)
	}
	entry(0, 8, 0, "Audio.dat") // right points to byte 32
	entry(32, 0, 50, "default.xbe")
	return map[int][]byte{base + 0x10000: vol, base + 40*2048: dir, base + 50*2048: XBE(title, titleID)}
}

// XBE is an Xbox program with a certificate naming it.
func XBE(title string, titleID uint32) []byte {
	b := make([]byte, 0x1000)
	copy(b, "XBEH")
	binary.LittleEndian.PutUint32(b[0x104:], 0x10000)
	binary.LittleEndian.PutUint32(b[0x118:], 0x10000+0x180)
	c := b[0x180:]
	binary.LittleEndian.PutUint32(c[0x08:], titleID)
	for i, u := range utf16.Encode([]rune(title)) {
		binary.LittleEndian.PutUint16(c[0x0C+i*2:], u)
	}
	binary.LittleEndian.PutUint32(c[0xA0:], 1)
	return b
}

// NSP is a Switch install package listing these files.
func NSP(names ...string) []byte {
	var strs bytes.Buffer
	table := make([]byte, len(names)*24)
	for i, n := range names {
		binary.LittleEndian.PutUint32(table[i*24+16:], uint32(strs.Len()))
		strs.WriteString(n + "\x00")
	}
	h := make([]byte, 16)
	copy(h, "PFS0")
	binary.LittleEndian.PutUint32(h[4:], uint32(len(names)))
	binary.LittleEndian.PutUint32(h[8:], uint32(strs.Len()))
	return pad(append(append(h, table...), strs.Bytes()...), 0x1000)
}

// XCI is the start of a Switch game card image.
func XCI() []byte {
	b := make([]byte, 0x1000)
	copy(b[0x100:], "HEAD")
	b[0x10D] = 0xF8
	return b
}

// NRO is a Switch homebrew program with its name in its assets.
func NRO(title string) []byte {
	const program = 0x1000
	b := make([]byte, program+0x1000)
	copy(b[0x10:], "NRO0")
	binary.LittleEndian.PutUint32(b[0x18:], program)
	a := b[program:]
	copy(a, "ASET")
	binary.LittleEndian.PutUint64(a[0x18:], 0x100)
	binary.LittleEndian.PutUint64(a[0x20:], 0x4000)
	copy(a[0x100:], title)
	return b
}
