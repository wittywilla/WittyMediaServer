package wittyplay

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Reasons a game file is refused before any core sees it. Nothing here decrypts
// anything or tries to: these only read the flags a file carries in the open.
const (
	refusedEncrypted3DS = "This 3DS game is encrypted, which these cores can't read, and this server doesn't decrypt games. A decrypted copy of a game you own will play."
	refusedCIA          = "This 3DS file is an installer (.cia), which these cores can't open. A .3ds or .cci copy of the game will play."
)

// Refusal says why a file for this console can't be played, in words for a person,
// or "" when nothing is known against it.
func Refusal(path, consoleID string) string {
	if consoleID != "3ds" {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cia":
		if ciaEncrypted(f) {
			return refusedEncrypted3DS
		}
		return refusedCIA
	case ".3ds", ".cci":
		if cartEncrypted(f) {
			return refusedEncrypted3DS
		}
	case ".cxi", ".app":
		if ncchEncrypted(f, 0) {
			return refusedEncrypted3DS
		}
	}
	return ""
}

const mediaUnit = 0x200

// cartEncrypted reads a cartridge image: an NCSD header naming its first partition,
// which is an NCCH.
func cartEncrypted(r io.ReaderAt) bool {
	var h [0x128]byte
	if _, err := r.ReadAt(h[:], 0); err != nil || !bytes.Equal(h[0x100:0x104], []byte("NCSD")) {
		return false
	}
	return ncchEncrypted(r, int64(binary.LittleEndian.Uint32(h[0x120:]))*mediaUnit)
}

// ncchEncrypted reads the NCCH flags: bit 2 of the eighth says "no crypto".
func ncchEncrypted(r io.ReaderAt, at int64) bool {
	var h [0x190]byte
	if _, err := r.ReadAt(h[:], at); err != nil || !bytes.Equal(h[0x100:0x104], []byte("NCCH")) {
		return false
	}
	return h[0x18F]&0x04 == 0
}

// ciaEncrypted reads an installer: a header giving section sizes, each section
// starting on a 64-byte boundary, then the title's metadata with one record per
// content. Bit 0 of a record's type means that content is encrypted.
func ciaEncrypted(r io.ReaderAt) bool {
	var h [0x20]byte
	if _, err := r.ReadAt(h[:], 0); err != nil {
		return false
	}
	align := func(n int64) int64 { return (n + 63) &^ 63 }
	header := int64(binary.LittleEndian.Uint32(h[0:]))
	certs := int64(binary.LittleEndian.Uint32(h[8:]))
	ticket := int64(binary.LittleEndian.Uint32(h[12:]))
	tmd := align(header) + align(certs) + align(ticket)
	var sig [4]byte
	if _, err := r.ReadAt(sig[:], tmd); err != nil {
		return false
	}
	var sigBlock int64
	switch binary.BigEndian.Uint32(sig[:]) {
	case 0x10003, 0x10000: // RSA-4096
		sigBlock = 4 + 0x200 + 0x3C
	case 0x10004, 0x10001: // RSA-2048
		sigBlock = 4 + 0x100 + 0x3C
	case 0x10005, 0x10002: // ECDSA
		sigBlock = 4 + 0x3C + 0x40
	default:
		return false
	}
	const tmdHeader, infoRecords = 0xC4, 0x900
	var rec [0x30]byte
	if _, err := r.ReadAt(rec[:], tmd+sigBlock+tmdHeader+infoRecords); err != nil {
		return false
	}
	return binary.BigEndian.Uint16(rec[6:])&1 != 0
}
