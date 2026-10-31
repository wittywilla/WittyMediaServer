package ipc

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	var b bytes.Buffer
	if err := Write(&b, MsgAudio, []byte{1, 2}, []byte{3, 4}); err != nil {
		t.Fatal(err)
	}
	big := make([]byte, 2000)
	if err := Write(&b, MsgSaveRAM, big); err != nil {
		t.Fatal(err)
	}
	r := NewReader(&b)
	typ, p, err := r.Next()
	if err != nil || typ != MsgAudio || !bytes.Equal(p, []byte{1, 2, 3, 4}) {
		t.Fatalf("%d %v %v", typ, p, err)
	}
	if typ, p, err = r.Next(); err != nil || typ != MsgSaveRAM || len(p) != 2000 {
		t.Fatalf("%d %d %v", typ, len(p), err)
	}
}

// A runner that has been taken over can claim anything: the reader must refuse
// before setting any memory aside.
func TestReaderRefusesLies(t *testing.T) {
	header := func(typ byte, n uint32) []byte {
		h := []byte{typ, 0, 0, 0, 0}
		binary.LittleEndian.PutUint32(h[1:], n)
		return h
	}
	for name, in := range map[string][]byte{
		"unknown type":   header(200, 1),
		"huge audio":     header(MsgAudio, MaxAudio+1),
		"huge video":     header(MsgVideo, MaxVideoFrame+VideoHeaderSize+1),
		"huge json":      header(MsgReady, MaxJSON+1),
		"huge blob":      header(MsgState, MaxBlob+1),
		"short body":     append(header(MsgAudio, 100), 1, 2, 3),
		"blob that lies": append(header(MsgState, MaxBlob), 1, 2, 3),
	} {
		if _, _, err := NewReader(bytes.NewReader(in)).Next(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestParseVideo(t *testing.T) {
	good := append(VideoHeader{Width: 2, Height: 2, Format: 1}.Append(nil), make([]byte, 16)...)
	if h, d, err := ParseVideo(good); err != nil || h.Width != 2 || len(d) != 16 {
		t.Fatalf("%+v %d %v", h, len(d), err)
	}
	for name, p := range map[string][]byte{
		"short":      {1, 2},
		"wrong size": append(VideoHeader{Width: 2, Height: 2, Format: 1}.Append(nil), make([]byte, 15)...),
		"zero":       VideoHeader{Width: 0, Height: 2, Format: 1}.Append(nil),
		"too wide":   VideoHeader{Width: MaxDimension + 1, Height: 1, Format: 2}.Append(nil),
		"bad format": append(VideoHeader{Width: 1, Height: 1, Format: 9}.Append(nil), 0, 0),
	} {
		if _, _, err := ParseVideo(p); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestParseInput(t *testing.T) {
	p := make([]byte, InputSize)
	p[0] = 2
	binary.LittleEndian.PutUint16(p[1:], 0x0108)
	binary.LittleEndian.PutUint16(p[3:], 0xffff) // -1
	port, b, lx, _, _, _, ok := ParseInput(p)
	if !ok || port != 2 || b != 0x0108 || lx != -1 {
		t.Fatal(port, b, lx, ok)
	}
	if _, _, _, _, _, _, ok := ParseInput(p[:5]); ok {
		t.Fatal("a short message must fail")
	}
}

func TestInputWithAndWithoutTriggers(t *testing.T) {
	short := make([]byte, InputSize)
	short[0], short[1] = 2, 0x01
	port, b, _, _, _, _, ok := ParseInput(short)
	if !ok || port != 2 || b != 1 {
		t.Fatalf("%d %d %v", port, b, ok)
	}
	if l, r := ParseTriggers(short); l != 0 || r != 0 {
		t.Errorf("%d %d", l, r)
	}
	full := append(append([]byte{}, short...), 128, 255)
	if _, _, _, _, _, _, ok := ParseInput(full); !ok {
		t.Error("the longer state should be read")
	}
	if l, r := ParseTriggers(full); l != 128 || r != 255 {
		t.Errorf("%d %d", l, r)
	}
	for _, n := range []int{0, 10, 12, 14, 64} {
		if _, _, _, _, _, _, ok := ParseInput(make([]byte, n)); ok {
			t.Errorf("%d bytes should be refused", n)
		}
	}
}
