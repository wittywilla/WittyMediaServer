package ffmpeg

import (
	"os"
	"strings"
	"testing"

	"github.com/wittywilla/wittydvd"
)

// Streams must be pinned by id before the first file, or the concat demuxer
// pairs them by position (which differs between DVD cells).
func TestWriteConcatPinsStreams(t *testing.T) {
	dir := t.TempDir()
	segs := []Segment{
		{Extents: []wittydvd.FileExtent{{File: "/disc.iso", Offset: 2048, Length: 4096}}, Duration: 2},
		{Extents: []wittydvd.FileExtent{{File: "/disc.iso", Offset: 8192, Length: 2048}, {File: "/disc.iso", Offset: 20480, Length: 2048}}, Duration: 1},
	}
	p, err := WriteConcat(dir, "t1", segs, 0x1e0, 0x81)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	want := "ffconcat version 1.0\nstream\nexact_stream_id 0x1e0\nstream\nexact_stream_id 0x81\nfile "
	if !strings.HasPrefix(s, want) {
		t.Fatalf("list does not start with the stream directives:\n%s", s)
	}
	if !strings.Contains(s, "concatf:") || !strings.Contains(s, "duration 2.000") {
		t.Fatalf("segments missing:\n%s", s)
	}

	// Without ids the list is unchanged from before.
	p, _ = WriteConcat(dir, "t2", segs[:1])
	b, _ = os.ReadFile(p)
	if strings.Contains(string(b), "stream\n") {
		t.Fatalf("unexpected stream directive:\n%s", b)
	}
}

func TestWriteConcatRefusesLineBreaks(t *testing.T) {
	seg := Segment{Extents: []wittydvd.FileExtent{{File: "/media/x\nsubfile,,start,0,end,99,,:/etc/passwd\n/VTS_01_1.VOB", Length: 1}}}
	if _, err := WriteConcat(t.TempDir(), "x", []Segment{seg}); err == nil {
		t.Fatal("a line break in a folder name would add an address to the list")
	}
}
