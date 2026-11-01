package midi

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"testing"
)

// song is one track at the default tempo: two quarter notes, a drum, and a tempo change.
func song() []byte {
	var tr bytes.Buffer
	tr.Write([]byte{0, 0xFF, 0x03, 4})
	tr.WriteString("Name")
	tr.Write([]byte{0, 0xC0, 40})
	tr.Write([]byte{0, 0x90, 60, 100, 96, 0x80, 60, 0})
	tr.Write([]byte{0, 0x90, 64, 100})
	tr.Write([]byte{48, 64, 0}) // running status: a note-on of no velocity ends it
	tr.Write([]byte{0, 0xFF, 0x51, 3, 0x03, 0xD0, 0x90})
	tr.Write([]byte{48, 0x99, 38, 100, 48, 0x89, 38, 0})
	tr.Write([]byte{0, 0xFF, 0x2F, 0})
	var b bytes.Buffer
	b.WriteString("MThd")
	binary.Write(&b, binary.BigEndian, uint32(6))
	binary.Write(&b, binary.BigEndian, []uint16{1, 1, 96})
	b.WriteString("MTrk")
	binary.Write(&b, binary.BigEndian, uint32(tr.Len()))
	b.Write(tr.Bytes())
	return b.Bytes()
}

func TestParse(t *testing.T) {
	s, err := Parse(song())
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "Name" {
		t.Errorf("title %q", s.Title)
	}
	// A quarter at 0.5 s, half of one at 0.25 s, then at the new tempo (0.25 s a
	// quarter) half of one and half of one again: 0.5 + 0.25 + 0.125 + 0.125 = 1 s... the
	// drum's end is the song's end.
	if math.Abs(s.Seconds-1.0) > 0.01 {
		t.Errorf("%.3f seconds", s.Seconds)
	}
}

func TestRiffWrapped(t *testing.T) {
	inner := song()
	var b bytes.Buffer
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(4+8+len(inner)))
	b.WriteString("RMID")
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint32(len(inner)))
	b.Write(inner)
	if _, err := Parse(b.Bytes()); err != nil {
		t.Error(err)
	}
}

func TestRenderIsSound(t *testing.T) {
	s, err := Parse(song())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := s.Render(&out); err != nil {
		t.Fatal(err)
	}
	want := int((s.Seconds + 3) * SampleRate * 4)
	if d := out.Len() - want; d < -64 || d > 64 {
		t.Errorf("%d bytes, want about %d", out.Len(), want)
	}
	peak := 0.0
	b := out.Bytes()
	for i := 0; i+2 <= len(b); i += 2 {
		v := math.Abs(float64(int16(binary.LittleEndian.Uint16(b[i:]))))
		if v > peak {
			peak = v
		}
	}
	if peak < 1000 {
		t.Errorf("the song is silent: peak %v", peak)
	}
}

func TestRenderStopsWithTheWriter(t *testing.T) {
	s, err := Parse(song())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Render(failing{}); err == nil {
		t.Error("a writer that fails should stop it")
	}
}

type failing struct{}

func (failing) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestDamage(t *testing.T) {
	for name, b := range map[string][]byte{
		"empty":     nil,
		"not midi":  []byte("hello there, world"),
		"no tracks": append([]byte("MThd\x00\x00\x00\x06\x00\x00\x00\x01\x00\x60"), "XXXX\x00\x00\x00\x00"...),
		"no time":   []byte("MThd\x00\x00\x00\x06\x00\x00\x00\x01\x00\x00MTrk\x00\x00\x00\x00"),
	} {
		if _, err := Parse(b); err == nil {
			t.Errorf("%s: should fail", name)
		}
	}
	// A song that says it's a year long stops at the limit.
	long := []byte("MThd\x00\x00\x00\x06\x00\x00\x00\x01\x00\x01MTrk\x00\x00\x00\x0C\xFF\xFF\xFF\x7F\x90\x3C\x40\xFF\xFF\xFF\x7F\x00")
	if s, err := Parse(long); err == nil && s.Seconds > maxSeconds {
		t.Errorf("%v seconds", s.Seconds)
	}
}

func FuzzParse(f *testing.F) {
	f.Add(song())
	f.Add([]byte("MThd\x00\x00\x00\x06\x00\x00\x00\x01\xE7\x28MTrk\x7F\xFF\xFF\xFF"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<16 {
			return
		}
		s, err := Parse(b)
		if err != nil {
			return
		}
		s.Seconds = min(s.Seconds, 0.1) // parsing is the point; a short render checks it works at all
		var out bytes.Buffer
		_ = s.Render(&out)
	})
}
