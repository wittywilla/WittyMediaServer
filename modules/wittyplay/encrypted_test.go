package wittyplay

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func ncch(flags byte) []byte {
	b := make([]byte, 0x200)
	copy(b[0x100:], "NCCH")
	b[0x18F] = flags
	return b
}

func cart(flags byte) []byte {
	b := make([]byte, 0x4000)
	copy(b[0x100:], "NCSD")
	binary.LittleEndian.PutUint32(b[0x120:], 0x4000/mediaUnit)
	return append(b, ncch(flags)...)
}

func cia(contentType uint16) []byte {
	const header, certs, ticket = 0x2020, 0xA00, 0x350
	tmd := int64((header+63)&^63) + (certs+63)&^63 + (ticket+63)&^63
	b := make([]byte, tmd+0x2000)
	binary.LittleEndian.PutUint32(b[0:], header)
	binary.LittleEndian.PutUint32(b[8:], certs)
	binary.LittleEndian.PutUint32(b[12:], ticket)
	binary.BigEndian.PutUint32(b[tmd:], 0x10004)
	binary.BigEndian.PutUint16(b[tmd+0x140+0xC4+0x900+6:], contentType)
	return b
}

func TestRefusal(t *testing.T) {
	const noCrypto = 0x04
	cases := []struct {
		name, console string
		data          []byte
		want          string
	}{
		{"game.3ds", "3ds", cart(0), "encrypted"},
		{"game.3ds", "3ds", cart(noCrypto), ""},
		{"game.cci", "3ds", cart(0), "encrypted"},
		{"game.cxi", "3ds", ncch(0), "encrypted"},
		{"game.cxi", "3ds", ncch(noCrypto), ""},
		{"game.cia", "3ds", cia(1), "encrypted"},
		{"game.cia", "3ds", cia(0), "installer"},
		{"game.3ds", "3ds", []byte("too short to say"), ""},
		{"game.3dsx", "3ds", []byte("homebrew"), ""},
		{"game.3ds", "nds", cart(0), ""},
	}
	for _, c := range cases {
		got := Refusal(writeTemp(t, c.name, c.data), c.console)
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("%s on %s: %q, want %q", c.name, c.console, got, c.want)
		}
	}
	if Refusal(filepath.Join(t.TempDir(), "missing.3ds"), "3ds") != "" {
		t.Error("a missing file is a different problem")
	}
}
