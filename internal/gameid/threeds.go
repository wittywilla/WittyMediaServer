// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package gameid

import (
	"encoding/binary"
	"fmt"
	"io"
)

// 3DS files. This only ever reads the plain headers that say what a file is. Nothing
// here decrypts anything or goes looking for keys.
//
// There are two layers of encryption, each with its own proof:
//
//   - An install package (.cia) marks each of its contents as encrypted or not in its
//     table of contents. When it's encrypted, the program inside doesn't start with
//     "NCCH" where it should, because those bytes are scrambled too. Bytes count for
//     more than the mark: "NCCH" where it belongs can't happen by accident.
//   - A program marks itself as unencrypted with its NoCrypto flag.

const (
	ncchMagicAt = 0x100
	ncchFlagsAt = 0x188
	ncchNoCrypt = 0x04
	ncchNew3DS  = 0x02 // platform byte, flags[4]
	mediaUnit   = 0x200
)

// CIA title types: the high half of a title id says what sort of thing it is.
const (
	ciaApplication = 0x00040000
	ciaUpdate      = 0x0004000E
	ciaDLC         = 0x0004008C
	ciaDemo        = 0x00040002
)

func sniff3DS(r io.ReaderAt, size int64) (Finding, bool) {
	if size < 0x200 {
		return Finding{}, false
	}
	magic := at(r, ncchMagicAt, 4)
	if magic != nil {
		switch string(magic) {
		case "NCSD":
			return sniffNCSD(r, size), true
		case "NCCH":
			return sniffNCCH(r, 0), true
		}
	}
	if h := at(r, 0, 4); h != nil {
		if string(h) == "3DSX" {
			return Finding{Console: "3ds", Kind: KindGame, Proof: "It's a 3DS homebrew program."}, true
		}
		if binary.LittleEndian.Uint32(h) == 0x2020 {
			return sniffCIA(r, size)
		}
	}
	return Finding{}, false
}

func sniffNCSD(r io.ReaderAt, size int64) Finding {
	f := Finding{Console: "3ds", Kind: KindGame, Proof: "It's a 3DS game card image."}
	table := at(r, 0x120, 8)
	if table == nil {
		return f
	}
	off := int64(binary.LittleEndian.Uint32(table)) * mediaUnit
	n := int64(binary.LittleEndian.Uint32(table[4:])) * mediaUnit
	if off <= 0 || n <= 0 || off+n > size {
		return f
	}
	inner := sniffNCCH(r, off)
	inner.Proof = "It's a 3DS game card image. " + inner.Proof
	return inner
}

func sniffNCCH(r io.ReaderAt, off int64) Finding {
	f := Finding{Console: "3ds", Kind: KindGame}
	b := at(r, off+ncchMagicAt, 0x90)
	if b == nil || string(b[:4]) != "NCCH" {
		f.Encrypted = true
		f.Proof = "Its program is scrambled: there's no NCCH mark where one belongs."
		return f
	}
	flags := at(r, off+ncchFlagsAt, 8)
	if flags == nil {
		f.Proof = "It's a 3DS program, cut short."
		return f
	}
	f.Serial = clean(b[0x50:0x60])
	f.NewOnly = flags[4]&ncchNew3DS != 0
	f.Encrypted = flags[7]&ncchNoCrypt == 0
	if f.Encrypted {
		f.Proof = fmt.Sprintf("Its program doesn't have the NoCrypto flag (flags byte 7 is %#02x), so it's encrypted.", flags[7])
	} else {
		f.Proof = "Its program has the NoCrypto flag, so it isn't encrypted."
	}
	return f
}

func align64(n int64) int64 { return (n + 63) &^ 63 }

// utf16Title reads a fixed field of UTF-16 text, keeping what can be shown.
func utf16Title(b []byte) string {
	out := make([]byte, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		c := binary.LittleEndian.Uint16(b[i:])
		if c == 0 {
			break
		}
		if c >= 0x20 && c < 0x7f {
			out = append(out, byte(c))
		} else {
			out = append(out, ' ')
		}
	}
	return titleIf(clean(out))
}

// sniffCIA reads an install package: a header giving the size of each part, then
// certificates, a ticket, a table of contents, and the contents themselves.
func sniffCIA(r io.ReaderAt, size int64) (Finding, bool) {
	h := at(r, 0, 0x20)
	if h == nil {
		return Finding{}, false
	}
	le := binary.LittleEndian
	certLen, tikLen, tmdLen := int64(le.Uint32(h[8:])), int64(le.Uint32(h[12:])), int64(le.Uint32(h[16:]))
	contentLen := int64(le.Uint64(h[24:]))
	tmd := align64(align64(align64(0x2020)+certLen) + tikLen)
	contents := align64(tmd + tmdLen)
	if tmdLen <= 0 || certLen < 0 || tikLen < 0 || contentLen <= 0 || contents >= size || contentLen > size-contents {
		return Finding{}, false
	}
	sig := at(r, tmd, 4)
	if sig == nil {
		return Finding{}, false
	}
	var skip int64
	switch binary.BigEndian.Uint32(sig) {
	case 0x10000, 0x10003:
		skip = 4 + 0x200 + 0x3C
	case 0x10001, 0x10004:
		skip = 4 + 0x100 + 0x3C
	case 0x10002, 0x10005:
		skip = 4 + 0x3C + 0x40
	default:
		return Finding{}, false
	}
	head := at(r, tmd+skip, 0xC4)
	rec := at(r, tmd+skip+0xC4+64*0x24, 0x30)
	if head == nil || rec == nil {
		return Finding{}, false
	}
	f := Finding{Console: "3ds", Kind: KindGame}
	be := binary.BigEndian
	switch be.Uint32(head[0x4C:]) {
	case ciaApplication, ciaDemo:
	case ciaUpdate:
		f.Kind = KindUpdate
	case ciaDLC:
		f.Kind = KindDLC
	default:
		f.Kind = KindUtility
	}
	flagged := be.Uint16(rec[6:])&1 != 0
	inner := sniffNCCH(r, contents)
	// The mark on the bytes themselves settles it: a package flagged as encrypted whose
	// program is readable isn't encrypted, and one marked clear whose program is scrambled is.
	magic := at(r, contents+ncchMagicAt, 4)
	readable := magic != nil && string(magic) == "NCCH"
	inner.Kind, inner.Console = f.Kind, "3ds"
	switch {
	case !readable && flagged:
		inner.Encrypted = true
		inner.Proof = fmt.Sprintf("It's a 3DS install package whose contents are marked encrypted, and its program is scrambled: the bytes where NCCH belongs are % x.", magic)
	case !readable:
		inner.Encrypted = true
		inner.Proof = fmt.Sprintf("It's a 3DS install package whose program is scrambled: the bytes where NCCH belongs are % x.", magic)
	default:
		inner.Proof = "It's a 3DS install package. " + inner.Proof
	}
	// A package carries its icon and title in the open, after its contents.
	if meta := le.Uint32(h[20:]); meta >= 0x400+0x36C0 {
		if name := at(r, contents+contentLen+0x400, 0x300); name != nil && string(name[:4]) == "SMDH" {
			inner.Title = utf16Title(name[8+0x200:][:0x80]) // the English one
		}
	}
	switch f.Kind {
	case KindUpdate:
		inner.Proof += " Its title id marks it as an update for a game, not a game."
	case KindDLC:
		inner.Proof += " Its title id marks it as extra content for a game, not a game."
	case KindUtility:
		inner.Proof += " Its title id marks it as system software, not a game."
	}
	return inner, true
}
