// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package gamemeta

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"testing"
)

// Made-up bytes only: no real game is in these tests.

func md5hex(b []byte) string {
	s := md5.Sum(b)
	return hex.EncodeToString(s[:])
}

func pattern(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*7 + 3)
	}
	return b
}

func raHashOf(t *testing.T, console string, b []byte) string {
	t.Helper()
	h, err := RAHash(console, bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("%s: %v", console, err)
	}
	return h
}

func TestRAHashHeaders(t *testing.T) {
	body := pattern(32 << 10)
	cases := []struct {
		name, console string
		header        []byte
		strip         bool
	}{
		{"iNES header", "nes", append([]byte("NES\x1a"), make([]byte, 12)...), true},
		{"Famicom Disk System header", "nes", append([]byte("FDS\x1a"), make([]byte, 12)...), true},
		{"NES without a header", "nes", nil, false},
		{"SNES copier header", "snes", make([]byte, 512), true},
		{"Atari 7800 header", "a7800", append([]byte("\x01ATARI7800"), make([]byte, 118)...), true},
		{"Lynx header", "lynx", append([]byte("LYNX\x00"), make([]byte, 59)...), true},
		{"Lynx without a header", "lynx", nil, false},
		{"Mega Drive is the whole file", "md", []byte("SEGA"), false},
		{"Game Boy is the whole file", "gb", []byte("GB"), false},
	}
	for _, c := range cases {
		file := append(append([]byte{}, c.header...), body...)
		want := md5hex(file)
		if c.strip {
			want = md5hex(body)
		}
		if got := raHashOf(t, c.console, file); got != want {
			t.Errorf("%s: got %s, want %s", c.name, got, want)
		}
	}
}

func TestRAHashSizeRules(t *testing.T) {
	// SNES: a header only when the size is 512 more than a whole number of 8 KB blocks.
	plain := pattern(8192 * 4)
	if got := raHashOf(t, "snes", plain); got != md5hex(plain) {
		t.Error("a SNES file with no header lost bytes")
	}
	odd := pattern(8192*4 + 100)
	if got := raHashOf(t, "snes", odd); got != md5hex(odd) {
		t.Error("a SNES file of another size lost bytes")
	}
	// PC Engine: the same, in 128 KB blocks.
	pce := pattern(128<<10 + 512)
	if got := raHashOf(t, "pce", pce); got != md5hex(pce[512:]) {
		t.Error("PC Engine header kept")
	}
	pce = pattern(8192 + 512) // 512 over 8 KB isn't 512 over 128 KB
	if got := raHashOf(t, "pce", pce); got != md5hex(pce) {
		t.Error("PC Engine file cut that has no header")
	}
}

func TestRAHashN64ByteOrders(t *testing.T) {
	z64 := append([]byte{0x80, 0x37, 0x12, 0x40}, pattern(4096-4)...)
	v64 := make([]byte, len(z64))
	n64 := make([]byte, len(z64))
	for i := 0; i < len(z64); i += 4 {
		v64[i], v64[i+1], v64[i+2], v64[i+3] = z64[i+1], z64[i], z64[i+3], z64[i+2]
		n64[i], n64[i+1], n64[i+2], n64[i+3] = z64[i+3], z64[i+2], z64[i+1], z64[i]
	}
	if v64[0] != 0x37 || n64[0] != 0x40 {
		t.Fatal("test data is wrong")
	}
	want := md5hex(z64)
	for name, b := range map[string][]byte{"z64": z64, "v64": v64, "n64": n64} {
		if got := raHashOf(t, "n64", b); got != want {
			t.Errorf("%s: got %s, want %s", name, got, want)
		}
	}
	// Bigger than one read, so the swap has to carry on across reads.
	big := append([]byte{0x37, 0x80}, pattern(200<<10-2)...)
	swapped := append([]byte{}, big...)
	for i := 0; i+1 < len(swapped); i += 2 {
		swapped[i], swapped[i+1] = swapped[i+1], swapped[i]
	}
	if got := raHashOf(t, "n64", big); got != md5hex(swapped) {
		t.Error("a big .v64 file isn't swapped throughout")
	}
}

func TestRAHashArcadeAndRefusals(t *testing.T) {
	if got, want := RAHashName("/games/arcade/mygame.zip"), md5hex([]byte("mygame")); got != want {
		t.Errorf("arcade: got %s, want %s", got, want)
	}
	if got := RAHashName("Name.With.Dots.7z"); got != md5hex([]byte("Name.With.Dots")) {
		t.Error("only the last extension comes off")
	}
	for _, c := range []string{"ps1", "saturn", "nds", "nosuch", "arcade"} {
		if _, err := RAHash(c, bytes.NewReader([]byte("x")), 1); !errors.Is(err, ErrRANotCovered) {
			t.Errorf("%s: %v", c, err)
		}
	}
	if _, err := RAHash("nes", bytes.NewReader(nil), RAMaxSize+1); err == nil {
		t.Error("an enormous file was read")
	}
	if len(RAConsoles("gb")) != 2 || len(RAConsoles("ps1")) != 0 {
		t.Error("console table")
	}
}
