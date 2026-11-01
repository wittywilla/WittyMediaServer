package bdsession

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// aStream writes a stream of aligned units to a disc folder.
func aStream(t *testing.T, dir string, unit []byte, units int) {
	t.Helper()
	stream := filepath.Join(dir, "BDMV", "STREAM")
	if err := os.MkdirAll(stream, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stream, "00000.m2ts"), bytes.Repeat(unit, units), 0o644); err != nil {
		t.Fatal(err)
	}
}

func clearUnit() []byte {
	u := make([]byte, alignedUnit)
	for i := 0; i < alignedUnit; i += sourcePacket {
		u[i+4] = 0x47
	}
	return u
}

// A unit AACS has scrambled keeps its first header, with the copy bits set, and the rest is noise.
func scrambledUnit() []byte {
	u := make([]byte, alignedUnit)
	u[0] = 0xc0
	u[4] = 0x47
	return u
}

// The server decides this for itself, before any helper hears of the disc.
func TestScrambled(t *testing.T) {
	clear := t.TempDir()
	aStream(t, clear, clearUnit(), 64)
	if Scrambled(clear) {
		t.Error("a clear disc was called encrypted")
	}
	bad := t.TempDir()
	aStream(t, bad, scrambledUnit(), 64)
	if !Scrambled(bad) {
		t.Error("an encrypted disc wasn't found")
	}
	// As a disc's folder is named, whatever the case.
	if !Scrambled(filepath.Join(bad, "BDMV")) {
		t.Error("an encrypted disc wasn't found through its BDMV folder")
	}
	// An AACS folder and nothing to read: nothing could be checked, so it counts.
	nothing := t.TempDir()
	if err := os.MkdirAll(filepath.Join(nothing, "AACS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !Scrambled(nothing) {
		t.Error("a disc with an AACS folder and no streams wasn't refused")
	}
	if Scrambled(filepath.Join(nothing, "missing")) {
		t.Error("something that isn't there was called encrypted")
	}
}

// With no WittyBluray, opening a disc says so.
func TestNoEngineSaysSo(t *testing.T) {
	old := RemoteEngine
	RemoteEngine = nil
	defer func() { RemoteEngine = old }()
	dir := t.TempDir()
	aStream(t, dir, clearUnit(), 4)
	if _, err := Open(dir, nil); err != ErrNoEngine {
		t.Fatalf("%v", err)
	}
}
