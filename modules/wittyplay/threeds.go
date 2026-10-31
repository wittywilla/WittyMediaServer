package wittyplay

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// 3DS game files. Most copies are encrypted, and the cores can only read them
// decrypted. Nothing here decrypts anything or goes looking for keys: this reads
// the plain headers that say what a file is, so that an encrypted one is refused
// with words a person can act on rather than a core's failure.

// ErrEncrypted3DS is why an encrypted 3DS file won't play.
var ErrEncrypted3DS = errors.New("This 3DS game file is encrypted, so it can't be played here, and this server won't decrypt it. A copy in decrypted form will play")

// ThreeDSFile is what a 3DS file's headers say.
type ThreeDSFile struct {
	Encrypted bool
	// Program is where the game's main program is, for a package that wraps it. A
	// core can't open a package, but it can open that program as a file of its own.
	Program    int64
	ProgramLen int64
	Package    bool
}

const (
	ncchFlags    = 0x188 // eight flag bytes
	ncchNoCrypto = 0x04  // in the last of them
	mediaUnit    = 0x200
	ciaAlign     = 64
)

// Inspect3DS reads a 3DS file's headers. Files it has nothing to say about, such as
// homebrew programs, are simply not encrypted as far as it knows.
func Inspect3DS(path string) (ThreeDSFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return ThreeDSFile{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return ThreeDSFile{}, err
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cia":
		return inspectCIA(f, st.Size())
	case ".3ds", ".cci":
		return inspectCCI(f, st.Size())
	case ".cxi", ".app":
		enc, err := ncchEncrypted(f, 0, st.Size())
		return ThreeDSFile{Encrypted: enc}, err
	}
	return ThreeDSFile{}, nil
}

func readAt(f io.ReaderAt, off int64, n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := f.ReadAt(b, off); err != nil {
		return nil, fmt.Errorf("this 3DS file is cut short or isn't what its name says: %w", err)
	}
	return b, nil
}

// ncchEncrypted looks at one program's header. Its magic number sits outside what
// is encrypted within a program, so if it's missing, the whole program is scrambled
// by the package around it. Only a flag saying "no encryption" counts as plain.
func ncchEncrypted(f io.ReaderAt, off, size int64) (bool, error) {
	if size < ncchFlags+8 {
		return false, errors.New("this 3DS file is too short to hold a game")
	}
	head, err := readAt(f, off+0x100, 4)
	if err != nil {
		return false, err
	}
	if string(head) != "NCCH" {
		return true, nil
	}
	flags, err := readAt(f, off+ncchFlags, 8)
	if err != nil {
		return false, err
	}
	return flags[7]&ncchNoCrypto == 0, nil
}

func inspectCCI(f io.ReaderAt, size int64) (ThreeDSFile, error) {
	head, err := readAt(f, 0x100, 4)
	if err != nil {
		return ThreeDSFile{}, err
	}
	if string(head) != "NCSD" {
		return ThreeDSFile{}, errors.New("this file doesn't look like a 3DS game card image")
	}
	table, err := readAt(f, 0x120, 8)
	if err != nil {
		return ThreeDSFile{}, err
	}
	off := int64(binary.LittleEndian.Uint32(table)) * mediaUnit
	n := int64(binary.LittleEndian.Uint32(table[4:])) * mediaUnit
	if off <= 0 || n <= 0 || off+n > size {
		return ThreeDSFile{}, errors.New("this 3DS game card image has no usable game inside")
	}
	enc, err := ncchEncrypted(f, off, n)
	return ThreeDSFile{Encrypted: enc}, err
}

func alignCIA(n int64) int64 { return (n + ciaAlign - 1) &^ (ciaAlign - 1) }

// inspectCIA reads an installable package: a header giving the size of each part,
// then certificates, a ticket, a table of contents, and the contents themselves.
func inspectCIA(f io.ReaderAt, size int64) (ThreeDSFile, error) {
	h, err := readAt(f, 0, 0x20)
	if err != nil {
		return ThreeDSFile{}, err
	}
	le := binary.LittleEndian
	headLen := int64(le.Uint32(h))
	certLen, tikLen, tmdLen := int64(le.Uint32(h[8:])), int64(le.Uint32(h[12:])), int64(le.Uint32(h[16:]))
	contentLen := int64(le.Uint64(h[24:]))
	if headLen != 0x2020 || tmdLen <= 0 || contentLen <= 0 || certLen < 0 || tikLen < 0 {
		return ThreeDSFile{}, errors.New("this file doesn't look like a 3DS install package")
	}
	tmd := alignCIA(alignCIA(alignCIA(headLen)+certLen) + tikLen)
	contents := alignCIA(tmd + tmdLen)
	if contents >= size || contentLen > size-contents {
		return ThreeDSFile{}, errors.New("this 3DS install package is cut short")
	}
	// The table of contents starts with a signature, whose type sets its length.
	sig, err := readAt(f, tmd, 4)
	if err != nil {
		return ThreeDSFile{}, err
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
		return ThreeDSFile{}, errors.New("this 3DS install package has a table of contents that can't be read")
	}
	// The header is 0xC4 bytes, then 64 info records of 0x24, then a record per content.
	first := tmd + skip + 0xC4 + 64*0x24
	if first+0x30 > tmd+tmdLen {
		return ThreeDSFile{}, errors.New("this 3DS install package has a table of contents that can't be read")
	}
	rec, err := readAt(f, first, 0x30)
	if err != nil {
		return ThreeDSFile{}, err
	}
	kind := binary.BigEndian.Uint16(rec[6:])
	n := int64(binary.BigEndian.Uint64(rec[8:]))
	if n <= 0 || n > contentLen {
		return ThreeDSFile{}, errors.New("this 3DS install package has a table of contents that can't be read")
	}
	out := ThreeDSFile{Package: true, Program: contents, ProgramLen: n}
	if kind&1 != 0 { // the package says this content is encrypted
		out.Encrypted = true
		return out, nil
	}
	out.Encrypted, err = ncchEncrypted(f, contents, n)
	return out, err
}

// CopyProgram writes the program out of an unencrypted package to a file of its
// own. It's a plain copy of bytes that are already readable.
func (t ThreeDSFile) CopyProgram(from, to string) error {
	if !t.Package || t.Encrypted {
		return errors.New("there's no unencrypted program to copy out of this file")
	}
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(to)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, io.NewSectionReader(in, t.Program, t.ProgramLen))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}
