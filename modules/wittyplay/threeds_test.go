package wittyplay

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// These files are generated: the headers are real, and the rest is filler. No game
// and no key is involved.

func fakeNCCH(plain bool, size int) []byte {
	b := make([]byte, size)
	copy(b[0x100:], "NCCH")
	if plain {
		b[ncchFlags+7] = ncchNoCrypto
	}
	return b
}

func scrambled(size int) []byte {
	b := make([]byte, size)
	for i := range b {
		b[i] = byte(i*7 + 13)
	}
	return b
}

// fakeCIA lays out a package as the format says, with one content.
func fakeCIA(content []byte, kind uint16) []byte {
	const certLen, tikLen, tmdLen = 0xA00, 0x350, 0xB64
	pad := func(b []byte) []byte {
		for len(b)%ciaAlign != 0 {
			b = append(b, 0)
		}
		return b
	}
	h := make([]byte, 0x2020)
	binary.LittleEndian.PutUint32(h, 0x2020)
	binary.LittleEndian.PutUint32(h[8:], certLen)
	binary.LittleEndian.PutUint32(h[12:], tikLen)
	binary.LittleEndian.PutUint32(h[16:], tmdLen)
	binary.LittleEndian.PutUint64(h[24:], uint64(len(content)))
	out := pad(h)
	out = pad(append(out, make([]byte, certLen)...))
	out = pad(append(out, make([]byte, tikLen)...))
	tmd := make([]byte, tmdLen)
	binary.BigEndian.PutUint32(tmd, 0x10004) // an RSA-2048 signature
	rec := tmd[4+0x100+0x3C+0xC4+64*0x24:]
	binary.BigEndian.PutUint16(rec[6:], kind)
	binary.BigEndian.PutUint64(rec[8:], uint64(len(content)))
	out = pad(append(out, tmd...))
	return append(out, content...)
}

func fakeCCI(inner []byte) []byte {
	const off = 0x4000
	b := make([]byte, off, off+len(inner))
	copy(b[0x100:], "NCSD")
	binary.LittleEndian.PutUint32(b[0x120:], off/mediaUnit)
	binary.LittleEndian.PutUint32(b[0x124:], uint32(len(inner)/mediaUnit))
	return append(b, inner...)
}

func TestInspect3DS(t *testing.T) {
	const size = 0x2000
	for _, c := range []struct {
		name      string
		data      []byte
		encrypted bool
		wantErr   bool
	}{
		{"plain.cci", fakeCCI(fakeNCCH(true, size)), false, false},
		{"plain.3ds", fakeCCI(fakeNCCH(true, size)), false, false},
		{"encrypted.3ds", fakeCCI(fakeNCCH(false, size)), true, false},
		{"plain.cxi", fakeNCCH(true, size), false, false},
		{"encrypted.cxi", fakeNCCH(false, size), true, false},
		{"plain.cia", fakeCIA(fakeNCCH(true, size), 0), false, false},
		{"title-key.cia", fakeCIA(scrambled(size), 1), true, false},
		{"marked-plain-but-scrambled.cia", fakeCIA(scrambled(size), 0), true, false},
		{"inner-crypto.cia", fakeCIA(fakeNCCH(false, size), 0), true, false},
		{"homebrew.3dsx", []byte("3DSX"), false, false},
		{"junk.3ds", make([]byte, 0x400), false, true},
		{"junk.cia", make([]byte, 0x400), false, true},
		{"short.cia", []byte("no"), false, true},
		{"short.cxi", []byte("no"), false, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), c.name)
			if err := os.WriteFile(path, c.data, 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := Inspect3DS(path)
			if (err != nil) != c.wantErr {
				t.Fatalf("error = %v, want one: %v", err, c.wantErr)
			}
			if got.Encrypted != c.encrypted {
				t.Errorf("Encrypted = %v, want %v", got.Encrypted, c.encrypted)
			}
		})
	}
}

// A package that lies about its sizes gets an error, not a read out of bounds.
func TestInspectCIARefusesLies(t *testing.T) {
	good := fakeCIA(fakeNCCH(true, 0x2000), 0)
	for name, change := range map[string]func([]byte){
		"content larger than the file": func(b []byte) { binary.LittleEndian.PutUint64(b[24:], 1<<40) },
		"negative content":             func(b []byte) { binary.LittleEndian.PutUint64(b[24:], 1<<63) },
		"table larger than the file":   func(b []byte) { binary.LittleEndian.PutUint32(b[16:], 1<<30) },
		"wrong header":                 func(b []byte) { binary.LittleEndian.PutUint32(b, 7) },
		"unknown signature":            func(b []byte) { binary.BigEndian.PutUint32(b[alignCIA(alignCIA(alignCIA(0x2020)+0xA00)+0x350):], 9) },
	} {
		t.Run(name, func(t *testing.T) {
			b := append([]byte(nil), good...)
			change(b)
			path := filepath.Join(t.TempDir(), "x.cia")
			if err := os.WriteFile(path, b, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Inspect3DS(path); err == nil {
				t.Error("should be refused")
			}
		})
	}
}

func TestCopyProgramOutOfPackage(t *testing.T) {
	inner := fakeNCCH(true, 0x2000)
	copy(inner[0x1000:], "the game's own bytes")
	dir := t.TempDir()
	path := filepath.Join(dir, "plain.cia")
	if err := os.WriteFile(path, fakeCIA(inner, 0), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := Inspect3DS(path)
	if err != nil || info.Encrypted || !info.Package {
		t.Fatalf("%+v %v", info, err)
	}
	out := filepath.Join(dir, "game.cxi")
	if err := info.CopyProgram(path, out); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil || !bytes.Equal(got, inner) {
		t.Fatalf("the program didn't come out whole (%d bytes, %v)", len(got), err)
	}
	// What comes out is a file the check itself calls plain.
	if again, err := Inspect3DS(out); err != nil || again.Encrypted {
		t.Errorf("%+v %v", again, err)
	}
	encrypted := ThreeDSFile{Package: true, Encrypted: true, ProgramLen: 1}
	if err := encrypted.CopyProgram(path, out+".2"); err == nil {
		t.Error("an encrypted program must not be copied out")
	}
}
