// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package gameid

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

// Xbox discs use a file system of their own, which marks itself 64 KB in. A copy of the
// whole disc (a redump image) has a video partition first, and the game's file system
// after it at a fixed place. The game is default.xbe, a program whose certificate names
// it and gives its title id, from which the product code comes.

const (
	xboxMark       = "MICROSOFT*XBOX*MEDIA"
	xboxVolumeAt   = 0x10000
	xboxRedumpGame = 0x18300000 // where the game partition starts in a full disc image
	xbeHeaderLen   = 0x178
	xbeCertLen     = 0xA4
	xboxSector     = 2048
)

// sniffXISO says whether this is an Xbox disc, plain (.xiso) or whole.
func sniffXISO(r io.ReaderAt, size int64) (Finding, bool) {
	for _, base := range []int64{0, xboxRedumpGame} {
		if base > 0 && size < base+xboxVolumeAt+xboxSector {
			continue
		}
		v := at(r, base+xboxVolumeAt, 0x20)
		if v == nil || !bytes.HasPrefix(v, []byte(xboxMark)) {
			continue
		}
		f := Finding{Console: Xbox, Kind: KindGame, Proof: "It has the volume mark of an Xbox disc."}
		if base > 0 {
			f.Proof = "It has the volume mark of an Xbox disc, after the disc's video partition."
		}
		le := binary.LittleEndian
		if xbe, ok := xboxFind(r, base, int64(le.Uint32(v[0x14:])), int64(le.Uint32(v[0x18:])), "DEFAULT.XBE"); ok {
			if t, serial, region, ok := readXBE(r, base+xbe*xboxSector, size); ok {
				f.Title, f.Serial, f.Region = t, serial, region
			}
		}
		return f, true
	}
	return Finding{}, false
}

// xboxFind looks for a file in an Xbox disc's top folder. A folder is a sorted tree: each
// entry points left and right, in steps of four bytes, to names before and after it.
func xboxFind(r io.ReaderAt, base, sector, length int64, want string) (int64, bool) {
	if sector <= 0 || length <= 0 || sector > 1<<22 {
		return 0, false
	}
	dir := at(r, base+sector*xboxSector, int(min(length, 8<<10)))
	if dir == nil {
		return 0, false
	}
	le := binary.LittleEndian
	p := 0
	for steps := 0; steps < 256; steps++ {
		if p+14 > len(dir) {
			return 0, false
		}
		n := int(dir[p+13])
		if n == 0 || p+14+n > len(dir) {
			return 0, false
		}
		name := strings.ToUpper(string(dir[p+14 : p+14+n]))
		var next uint16
		switch {
		case name == want:
			return int64(le.Uint32(dir[p+4:])), true
		case want < name:
			next = le.Uint16(dir[p:])
		default:
			next = le.Uint16(dir[p+2:])
		}
		if next == 0 || next == 0xFFFF {
			return 0, false
		}
		p = int(next) * 4
	}
	return 0, false
}

// readXBE reads an Xbox program's certificate: its title, product code and regions.
func readXBE(r io.ReaderAt, off, size int64) (title, serial, region string, ok bool) {
	h := at(r, off, xbeHeaderLen)
	if h == nil || !bytes.HasPrefix(h, []byte("XBEH")) {
		return "", "", "", false
	}
	le := binary.LittleEndian
	base, cert := le.Uint32(h[0x104:]), le.Uint32(h[0x118:])
	if cert < base || cert-base < xbeHeaderLen || cert-base > 0x10000 || off+int64(cert-base)+xbeCertLen > size {
		return "", "", "", false
	}
	c := at(r, off+int64(cert-base), xbeCertLen)
	if c == nil {
		return "", "", "", false
	}
	title = utf16Title(c[0x0C : 0x0C+0x50])
	id := le.Uint32(c[0x08:])
	if a, b := byte(id>>24), byte(id>>16); isCodeLetter(a) && isCodeLetter(b) {
		serial = fmt.Sprintf("%c%c-%03d", a, b, id&0xFFFF)
	}
	switch le.Uint32(c[0xA0:]) & 7 {
	case 1:
		region = "USA"
	case 2:
		region = "Japan"
	case 4:
		region = "Europe"
	case 7:
		region = "World"
	}
	return title, serial, region, true
}

func isCodeLetter(c byte) bool { return (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') }

// sniffXBE reads a loose Xbox program, as a game kept as a folder or homebrew comes.
func sniffXBE(r io.ReaderAt, size int64) Finding {
	f := Finding{Console: Xbox, Kind: KindGame, Proof: "It's an Xbox program (.xbe)."}
	if t, serial, region, ok := readXBE(r, 0, size); ok {
		f.Title, f.Serial, f.Region = t, serial, region
	}
	return f
}
