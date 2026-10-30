package stream

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func flvTag(typ byte, data []byte) []byte {
	h := []byte{typ, byte(len(data) >> 16), byte(len(data) >> 8), byte(len(data)), 0, 0, 0, 0, 0, 0, 0}
	out := append(h, data...)
	return binary.BigEndian.AppendUint32(out, uint32(len(out)))
}

func TestReadFLV(t *testing.T) {
	var s bytes.Buffer
	s.Write(make([]byte, 13))
	s.Write(flvTag(18, []byte("metadata")))
	avcc := []byte{1, 0x42, 0xe0, 0x1f, 0xff, 0xe1, 0, 2, 0x67, 0xaa, 1, 0, 2, 0x68, 0xbb}
	s.Write(flvTag(9, append([]byte{0x17, 0, 0, 0, 0}, avcc...)))
	s.Write(flvTag(9, []byte{0x17, 1, 0, 0, 0, 0, 0, 0, 2, 0x65, 0xcc}))
	s.Write(flvTag(9, []byte{0x27, 1, 0, 0, 0, 0, 0, 0, 1, 0x41}))
	var got []AU
	_ = readFLV(&s, func(a AU) { got = append(got, a) })
	if len(got) != 2 {
		t.Fatalf("got %d frames", len(got))
	}
	want := []byte{0, 0, 0, 1, 0x67, 0xaa, 0, 0, 0, 1, 0x68, 0xbb, 0, 0, 0, 1, 0x65, 0xcc}
	if !got[0].Key || !bytes.Equal(got[0].Data, want) {
		t.Fatalf("keyframe %x", got[0].Data)
	}
	if got[1].Key || !bytes.Equal(got[1].Data, []byte{0, 0, 0, 1, 0x41}) {
		t.Fatalf("delta %x", got[1].Data)
	}
}

func TestReadFLVRejectsLies(t *testing.T) {
	var s bytes.Buffer
	s.Write(make([]byte, 13))
	s.Write(flvTag(9, []byte{0x27, 1, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 0x41}))
	if err := readFLV(&s, func(AU) { t.Fatal("no frame should come out") }); err == nil {
		t.Fatal("a NAL length past the frame should be an error")
	}
}

func oggPage(segments [][]byte) []byte {
	out := append([]byte("OggS"), make([]byte, 22)...)
	out = append(out, byte(len(segments)))
	for _, s := range segments {
		out = append(out, byte(len(s)))
	}
	for _, s := range segments {
		out = append(out, s...)
	}
	return out
}

func TestReadOgg(t *testing.T) {
	var s bytes.Buffer
	s.Write(oggPage([][]byte{[]byte("OpusHead")}))
	s.Write(oggPage([][]byte{[]byte("OpusTags")}))
	s.Write(oggPage([][]byte{[]byte("one"), []byte("two")}))
	var got []string
	_ = readOgg(&s, func(p []byte) { got = append(got, string(p)) })
	if len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("got %q", got)
	}
}
