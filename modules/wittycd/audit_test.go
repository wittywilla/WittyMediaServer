package wittycd

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// aiff builds an AIFF or AIFC file with a chunk saying what the audio is.
func aiff(kind string, channels, bits uint16, rate [4]byte, compression string, samples []byte) []byte {
	comm := make([]byte, 18)
	binary.BigEndian.PutUint16(comm[0:], channels)
	binary.BigEndian.PutUint16(comm[6:], bits)
	copy(comm[8:], rate[:])
	comm = append(comm, compression...)
	body := []byte(kind)
	chunk := func(name string, b []byte) {
		h := make([]byte, 8)
		copy(h, name)
		binary.BigEndian.PutUint32(h[4:], uint32(len(b)))
		body = append(append(body, h...), b...)
		if len(b)%2 == 1 {
			body = append(body, 0)
		}
	}
	chunk("COMM", comm)
	chunk("SSND", append(make([]byte, 8), samples...))
	out := []byte("FORM")
	out = binary.BigEndian.AppendUint32(out, uint32(len(body)))
	return append(out, body...)
}

var cdRate = [4]byte{0x40, 0x0e, 0xac, 0x44}

func TestAIFFMustBeCDAudio(t *testing.T) {
	samples := make([]byte, 2352)
	for _, c := range []struct {
		name string
		file []byte
		ok   bool
		big  bool
	}{
		{"CD audio", aiff("AIFF", 2, 16, cdRate, "", samples), true, true},
		{"AIFC, uncompressed", aiff("AIFC", 2, 16, cdRate, "NONE", samples), true, true},
		{"AIFC, little-endian", aiff("AIFC", 2, 16, cdRate, "sowt", samples), true, false},
		{"mono", aiff("AIFF", 1, 16, cdRate, "", samples), false, false},
		{"24-bit", aiff("AIFF", 2, 24, cdRate, "", samples), false, false},
		{"48 kHz", aiff("AIFF", 2, 16, [4]byte{0x40, 0x0e, 0xbb, 0x80}, "", samples), false, false},
		{"compressed", aiff("AIFC", 2, 16, cdRate, "ima4", samples), false, false},
	} {
		dir := t.TempDir()
		p := filepath.Join(dir, "a.aif")
		if err := os.WriteFile(p, c.file, 0o600); err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		st, _ := f.Stat()
		off, n, big, err := aiffData(f, st.Size(), string(c.file[8:12]) == "AIFC")
		f.Close()
		if c.ok {
			if err != nil || big != c.big || n != int64(len(samples)) || off == 0 {
				t.Errorf("%s: %d %d %v %v", c.name, off, n, big, err)
			}
		} else if !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	// The samples can come before what they are.
	noComm := []byte("FORM\x00\x00\x00\x00AIFFSSND\x00\x00\x00\x10")
	noComm = append(noComm, make([]byte, 16)...)
	dir := t.TempDir()
	p := filepath.Join(dir, "b.aif")
	if err := os.WriteFile(p, noComm, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, _, _, err := aiffData(f, int64(len(noComm)), false); err == nil {
		t.Error("a file that doesn't say what its audio is")
	}
}

// A sheet can name a file through a linked folder, and only its last part used to be
// looked at: any file the server can read would come out as a track.
func TestSheetCantLeaveThroughALinkedFolder(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.bin"), make([]byte, 2352), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Skip("this system won't make links for this account: ", err)
	}
	if _, err := resolveBeside(dir, "link/secret.bin"); err == nil {
		t.Error("it went through")
	}
	if err := os.WriteFile(filepath.Join(dir, "own.bin"), make([]byte, 2352), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveBeside(dir, "own.bin"); err != nil {
		t.Errorf("a file beside the sheet: %v", err)
	}
}
