package ffmpeg

import (
	"os"
	"path/filepath"
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
	if !strings.Contains(s, "concatf:") || !strings.Contains(s, "duration 2.000000") {
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

func TestProbeSeekMargin(t *testing.T) {
	for name, want := range map[string]float64{"mpegts": 3, "mpeg": 3, "matroska,webm": 0, "mov,mp4,m4a,3gp,3g2,mj2": 0, "": 0} {
		p := Probe{Format: Format{FormatName: name}}
		if got := p.SeekMargin(); got != want {
			t.Errorf("%q: %g, want %g", name, got, want)
		}
	}
}

// A long read goes through several subfile addresses: FFmpeg goes wrong at over
// 2 GiB in one, and nothing is left out or read twice where they meet.
func TestWriteConcatSplitsLongReads(t *testing.T) {
	dir := t.TempDir()
	const size = 2*maxSubfile + 5
	p, err := WriteConcat(dir, "long", []Segment{{Extents: []wittydvd.FileExtent{{File: "/disc.iso", Offset: 100, Length: size}}, Duration: 9}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "concatf:") {
		t.Fatalf("not split:\n%s", b)
	}
	side, err := os.ReadFile(filepath.Join(dir, "long-seg1.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		SubfileURL(wittydvd.FileExtent{File: "/disc.iso", Offset: 100, Length: maxSubfile}),
		SubfileURL(wittydvd.FileExtent{File: "/disc.iso", Offset: 100 + maxSubfile, Length: maxSubfile}),
		SubfileURL(wittydvd.FileExtent{File: "/disc.iso", Offset: 100 + 2*maxSubfile, Length: 5}),
	}
	if got := strings.Fields(string(side)); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("got %v, want %v", got, want)
	}
}

// The next segment starts where this one's stated length says, so that length
// makes up for the difference in how early each segment's first packet lies.
func TestWriteConcatPlacesSegmentsByTheirFirstPicture(t *testing.T) {
	dir := t.TempDir()
	e := []wittydvd.FileExtent{{File: "/disc.iso", Length: 2048}}
	p, err := WriteConcat(dir, "lead", []Segment{{Extents: e, Duration: 10, Lead: 0}, {Extents: e, Duration: 20, Lead: 0.25}, {Extents: e, Duration: 30, Lead: 0.1}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	for _, want := range []string{"duration 9.750000", "duration 20.150000", "duration 30.100000"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("no %q in:\n%s", want, b)
		}
	}
}
