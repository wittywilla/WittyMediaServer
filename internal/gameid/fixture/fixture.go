// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package fixture makes tiny files for tests that have the headers real games have and
// nothing else: no game's code or data, only the marks the checks in gameid look for.
// Everything here is written byte by byte, so no game is ever in this repository.
package fixture

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
)

func pad(b []byte, n int) []byte {
	if len(b) >= n {
		return b
	}
	return append(b, make([]byte, n-len(b))...)
}

// NES is an iNES file.
func NES() []byte {
	b := make([]byte, 16+16<<10+8<<10)
	copy(b, "NES\x1a\x01\x01")
	return b
}

// SNES is a LoROM cartridge with a good checksum pair.
func SNES(title string) []byte {
	b := make([]byte, 64<<10)
	h := b[0x7FB0:]
	copy(h[0x10:0x25], title)
	h[0x25] = 0x20 // LoROM
	h[0x27] = 8
	h[0x29] = 1
	binary.LittleEndian.PutUint16(h[0x2C:], 0x1234^0xFFFF)
	binary.LittleEndian.PutUint16(h[0x2E:], 0x1234)
	binary.LittleEndian.PutUint16(h[0x4C:], 0x8000)
	return b
}

// N64 is a cartridge in big-endian order.
func N64(title, code string) []byte {
	b := make([]byte, 1<<20)
	copy(b, []byte{0x80, 0x37, 0x12, 0x40})
	copy(b[0x20:], title)
	copy(b[0x3B:], code)
	return b
}

// N64Swapped is the same in the byte-swapped .v64 order.
func N64Swapped(title, code string) []byte {
	b := N64(title, code)
	for i := 0; i+1 < len(b); i += 2 {
		b[i], b[i+1] = b[i+1], b[i]
	}
	return b
}

var gbLogo = []byte{0xCE, 0xED, 0x66, 0x66, 0xCC, 0x0D, 0x00, 0x0B, 0x03, 0x73, 0x00, 0x83, 0x00, 0x0C, 0x00, 0x0D}

// GB is a Game Boy cartridge.
func GB(title string) []byte {
	b := make([]byte, 32<<10)
	copy(b[0x104:], gbLogo)
	copy(b[0x134:], title)
	return b
}

// GBA is a Game Boy Advance cartridge.
func GBA(title, code string) []byte {
	b := make([]byte, 1<<20)
	copy(b[4:], []byte{0x24, 0xFF, 0xAE, 0x51})
	copy(b[0xA0:], title)
	copy(b[0xAC:], code)
	b[0xB2] = 0x96
	return b
}

// NDS is a DS cartridge.
func NDS(title, code string) []byte {
	b := make([]byte, 1<<20)
	copy(b, title)
	copy(b[0x0C:], code)
	copy(b[0xC0:], []byte{0x24, 0xFF, 0xAE, 0x51})
	binary.LittleEndian.PutUint16(b[0x15C:], 0xCF56)
	return b
}

// MegaDrive is a Mega Drive cartridge.
func MegaDrive(title, serial string) []byte {
	b := make([]byte, 128<<10)
	copy(b[0x100:], "SEGA MEGA DRIVE ")
	copy(b[0x150:], title)
	copy(b[0x180:], serial)
	return b
}

// MasterSystem is a Master System or, with gameGear, Game Gear cartridge.
func MasterSystem(gameGear bool) []byte {
	b := make([]byte, 32<<10)
	copy(b[0x7FF0:], "TMR SEGA")
	b[0x7FFF] = 0x4C
	if gameGear {
		b[0x7FFF] = 0x6C
	}
	return b
}

// sector is a plain 2048-byte sector's offset.
func sector(n int) int { return n * 2048 }

func dirRecord(name string, lba, size uint32, dir bool) []byte {
	n := 33 + len(name)
	if n%2 == 1 {
		n++
	}
	r := make([]byte, n)
	r[0] = byte(n)
	binary.LittleEndian.PutUint32(r[2:], lba)
	binary.BigEndian.PutUint32(r[6:], lba)
	binary.LittleEndian.PutUint32(r[10:], size)
	binary.BigEndian.PutUint32(r[14:], size)
	if dir {
		r[25] = 2
	}
	r[32] = byte(len(name))
	copy(r[33:], name)
	return r
}

// DiscFile is one file on a fixture disc.
type DiscFile struct {
	Name string
	Data []byte
}

// ISO makes an ISO 9660 image with a system id and some files in its root folder.
func ISO(system string, files ...DiscFile) []byte {
	total := 24 + len(files)*2
	b := make([]byte, sector(total))
	pvd := b[sector(16):sector(17)]
	pvd[0] = 1
	copy(pvd[1:], "CD001")
	pvd[6] = 1
	sys := pad([]byte(system), 32)
	for i, c := range sys {
		if c == 0 {
			sys[i] = ' '
		}
	}
	copy(pvd[8:], sys)
	root := bytes.Buffer{}
	root.Write(dirRecord("\x00", 18, 2048, true))
	root.Write(dirRecord("\x01", 18, 2048, true))
	lba := uint32(20)
	for _, f := range files {
		root.Write(dirRecord(f.Name+";1", lba, uint32(len(f.Data)), false))
		copy(b[sector(int(lba)):], f.Data)
		lba += 2
	}
	copy(b[sector(18):], root.Bytes())
	copy(pvd[156:], dirRecord("\x00", 18, 2048, true))
	return b
}

// PS1 is a PlayStation disc.
func PS1(serial string) []byte {
	return ISO("PLAYSTATION", DiscFile{"SYSTEM.CNF", []byte("BOOT = cdrom:\\" + serial + ";1\r\nTCB = 4\r\nEVENT = 10\r\nSTACK = 801FFFF0\r\n")})
}

// PS2 is a PlayStation 2 disc.
func PS2(serial string) []byte {
	return ISO("PLAYSTATION", DiscFile{"SYSTEM.CNF", []byte("BOOT2 = cdrom0:\\" + serial + ";1\r\nVER = 1.00\r\nVMODE = NTSC\r\n")})
}

// Raw makes a raw image out of a plain one: each 2048-byte sector gets a 16-byte header
// (mode 1 style) and the rest of a 2352-byte frame.
func Raw(plain []byte) []byte {
	var out bytes.Buffer
	for i := 0; i < len(plain); i += 2048 {
		f := make([]byte, 2352)
		f[15] = 1
		copy(f[16:], plain[i:min(i+2048, len(plain))])
		out.Write(f)
	}
	return out.Bytes()
}

// SFO makes a PARAM.SFO of text values.
func SFO(values ...[2]string) []byte {
	var keys, data bytes.Buffer
	var table bytes.Buffer
	for _, kv := range values {
		rec := make([]byte, 16)
		binary.LittleEndian.PutUint16(rec, uint16(keys.Len()))
		binary.LittleEndian.PutUint16(rec[2:], 0x0204)
		binary.LittleEndian.PutUint32(rec[4:], uint32(len(kv[1])+1))
		binary.LittleEndian.PutUint32(rec[8:], uint32(len(kv[1])+1))
		binary.LittleEndian.PutUint32(rec[12:], uint32(data.Len()))
		table.Write(rec)
		keys.WriteString(kv[0] + "\x00")
		data.WriteString(kv[1] + "\x00")
	}
	h := make([]byte, 20)
	copy(h, "\x00PSF")
	h[4] = 1
	h[5] = 1
	binary.LittleEndian.PutUint32(h[8:], uint32(20+table.Len()))
	binary.LittleEndian.PutUint32(h[12:], uint32(20+table.Len()+keys.Len()))
	binary.LittleEndian.PutUint32(h[16:], uint32(len(values)))
	return append(append(append(h, table.Bytes()...), keys.Bytes()...), data.Bytes()...)
}

// PSP is a PSP disc. The PSP_GAME folder holds a PARAM.SFO.
func PSP(title, id string) []byte {
	sfo := SFO([2]string{"TITLE", title}, [2]string{"DISC_ID", id}, [2]string{"CATEGORY", "UG"})
	b := make([]byte, sector(40))
	pvd := b[sector(16):sector(17)]
	pvd[0] = 1
	copy(pvd[1:], "CD001")
	pvd[6] = 1
	copy(pvd[8:], "PSP GAME")
	root := bytes.Buffer{}
	root.Write(dirRecord("\x00", 18, 2048, true))
	root.Write(dirRecord("\x01", 18, 2048, true))
	root.Write(dirRecord("PSP_GAME", 20, 2048, true))
	copy(b[sector(18):], root.Bytes())
	sub := bytes.Buffer{}
	sub.Write(dirRecord("\x00", 20, 2048, true))
	sub.Write(dirRecord("\x01", 18, 2048, true))
	sub.Write(dirRecord("PARAM.SFO;1", 22, uint32(len(sfo)), false))
	copy(b[sector(20):], sub.Bytes())
	copy(b[sector(22):], sfo)
	copy(pvd[156:], dirRecord("\x00", 18, 2048, true))
	return b
}

// CSO compresses a disc the way a .cso does: raw deflate, block by block.
func CSO(plain []byte) []byte {
	const block = 2048
	blocks := (len(plain) + block - 1) / block
	head := make([]byte, 0x18)
	copy(head, "CISO")
	binary.LittleEndian.PutUint32(head[4:], 0x18)
	binary.LittleEndian.PutUint64(head[8:], uint64(len(plain)))
	binary.LittleEndian.PutUint32(head[16:], block)
	head[20] = 1
	index := make([]byte, (blocks+1)*4)
	var body bytes.Buffer
	at := len(head) + len(index)
	for i := 0; i < blocks; i++ {
		binary.LittleEndian.PutUint32(index[i*4:], uint32(at+body.Len()))
		w, _ := flate.NewWriter(&body, flate.DefaultCompression)
		_, _ = w.Write(plain[i*block : min((i+1)*block, len(plain))])
		_ = w.Close()
	}
	binary.LittleEndian.PutUint32(index[blocks*4:], uint32(at+body.Len()))
	return append(append(head, index...), body.Bytes()...)
}

// Boot makes a disc whose first sector starts with a console's mark.
func Boot(mark string) []byte {
	b := make([]byte, 1<<20)
	copy(b, mark)
	return b
}

// NCCH is a 3DS program: plain, or marked as encrypted. newOnly marks it for the New 3DS.
func NCCH(plain, newOnly bool, size int) []byte {
	b := make([]byte, size)
	copy(b[0x100:], "NCCH")
	copy(b[0x150:], "CTR-P-TEST")
	if plain {
		b[0x18F] = 0x04
	}
	b[0x18C] = 1
	if newOnly {
		b[0x18C] = 2
	}
	return b
}

// Scrambled stands for a program under a package's own encryption: it has no mark.
func Scrambled(size int) []byte {
	b := make([]byte, size)
	for i := range b {
		b[i] = byte(i*131 + 7)
	}
	return b
}

func align64(n int) int { return (n + 63) &^ 63 }

// CIA wraps a program in an install package. flags go in the table of contents, and
// titleHigh is the high half of its title id.
func CIA(program []byte, flags uint16, titleHigh uint32) []byte {
	const certLen, tikLen = 0x100, 0x100
	sigSkip := 4 + 0x100 + 0x3C
	tmdLen := sigSkip + 0xC4 + 64*0x24 + 0x30
	h := make([]byte, 0x2020)
	binary.LittleEndian.PutUint32(h, 0x2020)
	binary.LittleEndian.PutUint32(h[8:], certLen)
	binary.LittleEndian.PutUint32(h[12:], tikLen)
	binary.LittleEndian.PutUint32(h[16:], uint32(tmdLen))
	binary.LittleEndian.PutUint64(h[24:], uint64(len(program)))
	tmdAt := align64(align64(align64(0x2020)+certLen) + tikLen)
	contentsAt := align64(tmdAt + tmdLen)
	out := make([]byte, contentsAt+len(program))
	copy(out, h)
	binary.BigEndian.PutUint32(out[tmdAt:], 0x10004)
	head := tmdAt + sigSkip
	binary.BigEndian.PutUint32(out[head+0x4C:], titleHigh)
	binary.BigEndian.PutUint16(out[head+0x9E:], 1)
	rec := head + 0xC4 + 64*0x24
	binary.BigEndian.PutUint16(out[rec+6:], flags)
	binary.BigEndian.PutUint64(out[rec+8:], uint64(len(program)))
	copy(out[contentsAt:], program)
	return out
}
