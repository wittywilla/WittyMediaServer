package wittycd

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// chdImage builds a CHD with no compression, as a test can without any tool: a
// header, frames, a map with a number for each hunk, and the track list.
type chdImage struct {
	version  uint32
	hunk     uint32
	unit     uint32
	logical  uint64
	meta     []string
	metaTag  string
	frames   []byte // already big-endian with subcode
	badMap   bool
	compress [4]uint32
}

func (c *chdImage) build() []byte {
	if c.version == 0 {
		c.version = 5
	}
	if c.hunk == 0 {
		c.hunk = chdFrameBytes * 8
	}
	if c.unit == 0 {
		c.unit = chdFrameBytes
	}
	if c.logical == 0 {
		c.logical = uint64(len(c.frames))
	}
	if c.metaTag == "" {
		c.metaTag = "CHT2"
	}
	hunks := (len(c.frames) + int(c.hunk) - 1) / int(c.hunk)
	out := make([]byte, chdHeaderBytes)
	copy(out, "MComprHD")
	binary.BigEndian.PutUint32(out[8:], chdHeaderBytes)
	binary.BigEndian.PutUint32(out[12:], c.version)
	for i, v := range c.compress {
		binary.BigEndian.PutUint32(out[16+i*4:], v)
	}
	binary.BigEndian.PutUint64(out[32:], c.logical)
	binary.BigEndian.PutUint32(out[56:], c.hunk)
	binary.BigEndian.PutUint32(out[60:], c.unit)
	// The hunks go on whole multiples of a hunk, as an uncompressed map counts them.
	binary.BigEndian.PutUint64(out[48:], uint64(len(out))) // the metadata comes next
	for i, m := range c.meta {
		next := uint64(0)
		if i+1 < len(c.meta) {
			next = uint64(len(out)) + 16 + uint64(len(m)) + 1
		}
		var h [16]byte
		copy(h[:], c.metaTag)
		binary.BigEndian.PutUint32(h[4:], uint32(len(m)+1)|1<<24)
		binary.BigEndian.PutUint64(h[8:], next)
		out = append(out, h[:]...)
		out = append(out, m...)
		out = append(out, 0)
	}
	binary.BigEndian.PutUint64(out[40:], uint64(len(out)))
	mapAt := len(out)
	out = append(out, make([]byte, hunks*4)...)
	for len(out)%int(c.hunk) != 0 {
		out = append(out, 0)
	}
	for i := range hunks {
		binary.BigEndian.PutUint32(out[mapAt+i*4:], uint32(len(out))/c.hunk)
		h := make([]byte, c.hunk)
		copy(h, c.frames[i*int(c.hunk):])
		out = append(out, h...)
	}
	if c.badMap {
		binary.BigEndian.PutUint32(out[mapAt:], 0xfffffff0)
	}
	return out
}

// framesOf turns little-endian audio into CHD's frames, padded to a multiple of 4.
func framesOf(pcm []byte) []byte {
	n := len(pcm) / SectorBytes
	out := make([]byte, 0, (n+4)*chdFrameBytes)
	for i := range n {
		s := bytes.Clone(pcm[i*SectorBytes : (i+1)*SectorBytes])
		swap16(s, 0)
		out = append(out, s...)
		out = append(out, make([]byte, chdSubBytes)...)
	}
	for n%chdTrackPad != 0 {
		out = append(out, make([]byte, chdFrameBytes)...)
		n++
	}
	return out
}

func trackLine(n int, kind string, frames, pregap int, pgtype string, postgap int) string {
	return fmt.Sprintf("TRACK:%d TYPE:%s SUBTYPE:NONE FRAMES:%d PREGAP:%d PGTYPE:%s PGSUB:NONE POSTGAP:%d", n, kind, frames, pregap, pgtype, postgap)
}

func openStream(t *testing.T, path string) (*Disc, *Stream) {
	t.Helper()
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	s, err := d.NewStream(nil, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	return d, s
}

func TestCHDHandBuilt(t *testing.T) {
	one := intTone(0, 103)
	two := intTone(103*SectorSamples, 200)
	img := chdImage{
		frames: append(framesOf(one), framesOf(two)...),
		meta: []string{
			trackLine(1, "AUDIO", 103, 0, "NONE", 0),
			trackLine(2, "AUDIO", 200, 50, "VAUDIO", 0),
			trackLine(3, "AUDIO", 0, 0, "NONE", 0),
		},
	}
	p := write(t, t.TempDir(), "Hand Built.chd", img.build())
	d, s := openStream(t, p)
	if d.Format != "chd" || d.Text.Title != "Hand Built" {
		t.Errorf("disc = %+v", d)
	}
	want := []struct{ pregap, start, end int }{{0, 0, 103}, {103, 153, 303}}
	if len(d.Tracks) != len(want) {
		t.Fatalf("tracks = %+v", d.Tracks)
	}
	for i, w := range want {
		tr := d.Tracks[i]
		if tr.Pregap != w.pregap || tr.Start != w.start || tr.End != w.end || !tr.Audio {
			t.Errorf("track %d = %+v, want %+v", i+1, tr, w)
		}
	}
	if !bytes.Equal(readAll(t, s, s.Size()), append(bytes.Clone(one), two...)) {
		t.Fatal("the stream differs from what went in, past the padding")
	}
	if hasWarning(d, "byte-order") {
		t.Error("CHD's byte order is a fact of the format and shouldn't be questioned")
	}
}

// A pregap that isn't stored is silence, and takes up no room in the file.
func TestCHDPregapNotStored(t *testing.T) {
	data := bytes.Repeat([]byte{1, 2, 3, 4}, 16*SectorBytes/4)
	audio := intTone(0, 100)
	img := chdImage{
		frames: append(framesOf(data), framesOf(audio)...),
		meta: []string{
			trackLine(1, "MODE1_RAW", 16, 0, "NONE", 0),
			trackLine(2, "AUDIO", 100, 150, "AUDIO", 30),
		},
	}
	d, s := openStream(t, write(t, t.TempDir(), "x.chd", img.build()))
	if d.Tracks[0].Audio || d.Tracks[0].Mode != "MODE1/2352" || !hasWarning(d, "data-track") {
		t.Errorf("track 1 = %+v", d.Tracks[0])
	}
	if tr := d.Tracks[1]; tr.Pregap != 16 || tr.Start != 166 || tr.End != 296 {
		t.Errorf("track 2 = %+v", tr)
	}
	want := make([]byte, 280*SectorBytes)
	copy(want[150*SectorBytes:], audio)
	if !bytes.Equal(readAll(t, s, s.Size()), want) {
		t.Fatal("the stream should be silence, the audio, and silence")
	}
}

func TestCHDRefusals(t *testing.T) {
	audio := framesOf(intTone(0, 16))
	good := trackLine(1, "AUDIO", 16, 0, "NONE", 0)
	cases := []struct {
		name string
		img  chdImage
		want error
	}{
		{"an old version", chdImage{version: 4, frames: audio, meta: []string{good}}, ErrUnsupported},
		{"a hard disk", chdImage{frames: audio, meta: []string{"CYLS:1,HEADS:1,SECS:1,BPS:512"}, metaTag: "GDDD"}, ErrNotCD},
		{"a GD-ROM", chdImage{frames: audio, meta: []string{"TRACK:1 TYPE:MODE1 SUBTYPE:NONE FRAMES:1 PAD:0"}, metaTag: "CHGT"}, ErrUnsupported},
		{"only data", chdImage{frames: audio, meta: []string{trackLine(1, "MODE1_RAW", 16, 0, "NONE", 0)}}, ErrNoAudio},
		{"too large for any CD", chdImage{logical: 2 << 30, frames: audio, meta: []string{good}}, ErrNotCD},
		{"a giant hunk", chdImage{hunk: chdFrameBytes * 4096, frames: audio, meta: []string{good}}, ErrNotCD},
		{"the wrong frame size", chdImage{hunk: 4096, unit: 512, frames: audio, meta: []string{good}}, ErrNotCD},
	}
	for _, c := range cases {
		p := write(t, t.TempDir(), "x.chd", c.img.build())
		if d, err := Open(p); !errors.Is(err, c.want) {
			if err == nil {
				d.Close()
			}
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
	for _, junk := range [][]byte{nil, []byte("MComprHD"), bytes.Repeat([]byte("x"), 200)} {
		if _, err := Open(write(t, t.TempDir(), "x.chd", junk)); !errors.Is(err, ErrNotCD) {
			t.Errorf("junk: err = %v", err)
		}
	}
}

// An image that is only the changes from another one can't be played by itself.
func TestCHDWithParentIsRefused(t *testing.T) {
	img := chdImage{frames: framesOf(intTone(0, 16)), meta: []string{trackLine(1, "AUDIO", 16, 0, "NONE", 0)}}
	b := img.build()
	// A hunk never written comes from the parent in an image that has one.
	mapAt := binary.BigEndian.Uint64(b[40:])
	binary.BigEndian.PutUint32(b[mapAt:], 0)
	if d, err := Open(write(t, t.TempDir(), "plain.chd", b)); err != nil {
		t.Fatalf("with no parent that hunk is just zeroes: %v", err)
	} else {
		d.Close()
	}
	b[104] = 1
	if _, err := Open(write(t, t.TempDir(), "child.chd", b)); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v", err)
	}
}

// The map claims a place well past the end of the file: that hunk is silence.
func TestCHDHunkPastTheEnd(t *testing.T) {
	audio := intTone(0, 160)
	img := chdImage{frames: framesOf(audio), badMap: true, meta: []string{trackLine(1, "AUDIO", 160, 0, "NONE", 0)}}
	d, s := openStream(t, write(t, t.TempDir(), "x.chd", img.build()))
	if !hasWarning(d, "truncated") {
		t.Errorf("warnings = %+v", d.Warnings)
	}
	want := bytes.Clone(audio)
	clear(want[:8*SectorBytes])
	if !bytes.Equal(readAll(t, s, s.Size()), want) {
		t.Fatal("only the hunk that isn't there should be silent")
	}
}

func TestCHDCRC(t *testing.T) {
	// The standard check value for this CRC-16.
	if got := crcCHD(0xffff, []byte("123456789")); got != 0x29b1 {
		t.Fatalf("crc = %04x", got)
	}
}

// Images chdman really made, from the same integer tones, kept small. How they were
// made is in TestChdmanFixtures.
func TestCHDFromChdman(t *testing.T) {
	want := intTone(0, 24)
	for _, name := range []string{"tones_cdlz.chd", "tones_cdzl.chd", "tones_cdfl.chd"} {
		t.Run(name, func(t *testing.T) {
			d, s := openStream(t, filepath.Join("testdata", name))
			if len(d.Tracks) != 2 || d.Tracks[1].Pregap != 10 || d.Tracks[1].Start != 14 || d.Tracks[1].End != 24 {
				t.Errorf("tracks = %+v", d.Tracks)
			}
			if !bytes.Equal(readAll(t, s, s.Size()), want) {
				t.Fatal("the stream differs from the tone that went in")
			}
		})
	}
	t.Run("mixed.chd", func(t *testing.T) {
		d, s := openStream(t, filepath.Join("testdata", "mixed.chd"))
		if len(d.Tracks) != 2 || d.Tracks[0].Audio || !d.Tracks[1].Audio {
			t.Fatalf("tracks = %+v", d.Tracks)
		}
		if !bytes.Equal(readAll(t, s, s.Size()), intTone(0, 12)) {
			t.Fatal("the audio after a data track differs")
		}
		// It is only worth having if the bitmap really is in use.
		c := d.extentAt(d.Tracks[1].Start).src.(*chdFile)
		head := make([]byte, 1)
		if err := c.readBytes(head, c.entries[0].offset); err != nil || head[0] == 0 {
			t.Fatalf("the first hunk's bitmap is %x (%v)", head, err)
		}
	})
}

// A CHD with the track list of a cue sheet must play exactly as the cue sheet does.
func compareDiscs(t *testing.T, chd, cue string) {
	t.Helper()
	dc, sc := openStream(t, chd)
	dq, sq := openStream(t, cue)
	if len(dc.Tracks) != len(dq.Tracks) {
		t.Fatalf("%d tracks, the cue sheet has %d", len(dc.Tracks), len(dq.Tracks))
	}
	for i := range dc.Tracks {
		a, b := dc.Tracks[i], dq.Tracks[i]
		if a.Number != b.Number || a.Audio != b.Audio || a.Pregap != b.Pregap || a.Start != b.Start || a.End != b.End {
			t.Errorf("track %d: %+v\nthe cue sheet says: %+v", i+1, a, b)
		}
	}
	if sc.Size() != sq.Size() {
		t.Fatalf("stream is %d bytes, the cue sheet's is %d", sc.Size(), sq.Size())
	}
	x, y := readAll(t, sc, sc.Size()), readAll(t, sq, sq.Size())
	if !bytes.Equal(x, y) {
		for i := range x {
			if x[i] != y[i] {
				t.Fatalf("the streams first differ at byte %d, sample %d", i, i/SampleBytes)
			}
		}
	}
}

// chdmanDisc is a cue sheet and the bin it needs.
type chdmanDisc struct {
	name, cue string
	bin       []byte
	more      map[string][]byte
}

func chdmanDiscs() []chdmanDisc {
	dataSector := func(n int) []byte {
		out := make([]byte, n*SectorBytes)
		for i := range n {
			s := out[i*SectorBytes : (i+1)*SectorBytes]
			copy(s[1:11], bytes.Repeat([]byte{0xff}, 10))
			s[15] = 1
			for j := 16; j < len(s); j++ {
				s[j] = byte((i + j) * 3)
			}
		}
		return out
	}
	return []chdmanDisc{
		{"plain", `FILE "x.bin" BINARY
TRACK 01 AUDIO
INDEX 01 00:00:00
TRACK 02 AUDIO
INDEX 00 00:02:10
INDEX 01 00:03:11
TRACK 03 AUDIO
INDEX 01 00:05:37
`, intTone(0, 700), nil},
		{"pregaps", `FILE "x.bin" BINARY
TRACK 01 AUDIO
INDEX 00 00:00:00
INDEX 01 00:02:00
TRACK 02 AUDIO
PREGAP 00:01:00
INDEX 01 00:04:00
TRACK 03 AUDIO
INDEX 01 00:05:13
POSTGAP 00:01:00
TRACK 04 AUDIO
INDEX 00 00:06:00
INDEX 01 00:06:40
`, intTone(0, 600), nil},
		{"data first", `FILE "x.bin" BINARY
TRACK 01 MODE1/2352
INDEX 01 00:00:00
TRACK 02 AUDIO
INDEX 00 00:02:00
INDEX 01 00:04:00
TRACK 03 AUDIO
INDEX 01 00:05:50
`, append(dataSector(150), intTone(0, 450)...), nil},
		{"data last", `FILE "x.bin" BINARY
TRACK 01 AUDIO
INDEX 01 00:00:00
TRACK 02 MODE1/2352
INDEX 00 00:04:00
INDEX 01 00:04:10
`, append(intTone(0, 300), dataSector(101)...), nil},
		{"a file for each track", `FILE "x.bin" BINARY
TRACK 01 MODE1/2352
INDEX 01 00:00:00
FILE "2.bin" BINARY
TRACK 02 AUDIO
INDEX 00 00:00:00
INDEX 01 00:02:00
FILE "3.bin" BINARY
TRACK 03 AUDIO
INDEX 00 00:00:00
INDEX 01 00:00:37
`, dataSector(77), map[string][]byte{"2.bin": intTone(0, 321), "3.bin": intTone(321*SectorSamples, 111)}},
	}
}

// Runs against the real tool when WITTYCD_CHDMAN names it: every disc through every
// form of compression chdman offers for a CD.
func TestChdmanMadeImages(t *testing.T) {
	chdman := os.Getenv("WITTYCD_CHDMAN")
	if chdman == "" {
		t.Skip("WITTYCD_CHDMAN doesn't name chdman")
	}
	options := [][]string{
		nil, // chdman's own choice
		{"-c", "cdlz"}, {"-c", "cdzl"}, {"-c", "cdfl"},
		{"-c", "none"}, {"-c", "zlib"}, {"-c", "lzma"}, {"-c", "flac"}, {"-c", "huff"},
		{"-hs", fmt.Sprint(chdFrameBytes * 2)},
		{"-hs", fmt.Sprint(chdFrameBytes * 40), "-c", "cdlz,cdfl"},
		{"-hs", fmt.Sprint(chdFrameBytes * 40), "-c", "cdzl"},
	}
	for _, disc := range chdmanDiscs() {
		dir := t.TempDir()
		write(t, dir, "x.bin", disc.bin)
		for name, b := range disc.more {
			write(t, dir, name, b)
		}
		cue := write(t, dir, "x.cue", []byte(disc.cue))
		for _, opt := range options {
			name := disc.name + " " + strings.Join(opt, " ")
			t.Run(name, func(t *testing.T) {
				out := filepath.Join(dir, "x.chd")
				args := append([]string{"createcd", "-f", "-i", cue, "-o", out}, opt...)
				if b, err := exec.Command(chdman, args...).CombinedOutput(); err != nil {
					t.Fatalf("chdman: %v\n%s", err, b)
				}
				compareDiscs(t, out, cue)
			})
		}
	}
}

// A form of compression this module doesn't read is refused in so many words.
func TestChdmanZstdIsRefused(t *testing.T) {
	chdman := os.Getenv("WITTYCD_CHDMAN")
	if chdman == "" {
		t.Skip("WITTYCD_CHDMAN doesn't name chdman")
	}
	dir := t.TempDir()
	write(t, dir, "x.bin", intTone(0, 100))
	cue := write(t, dir, "x.cue", []byte("FILE \"x.bin\" BINARY\nTRACK 01 AUDIO\nINDEX 01 00:00:00\n"))
	out := filepath.Join(dir, "x.chd")
	if b, err := exec.Command(chdman, "createcd", "-f", "-i", cue, "-o", out, "-c", "cdzs").CombinedOutput(); err != nil {
		t.Skipf("this chdman can't make one: %s", b)
	}
	if _, err := Open(out); !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), "cdzs") {
		t.Fatalf("err = %v", err)
	}
}

// A karaoke disc's graphics travel in the subcode, which chdman keeps from a .toc.
func TestChdmanGraphics(t *testing.T) {
	chdman := os.Getenv("WITTYCD_CHDMAN")
	if chdman == "" {
		t.Skip("WITTYCD_CHDMAN doesn't name chdman")
	}
	dir := t.TempDir()
	const sectors = 100
	pcm := intTone(0, sectors)
	var bin bytes.Buffer
	want := make([]byte, 0, sectors*chdSubBytes)
	for i := range sectors {
		bin.Write(pcm[i*SectorBytes : (i+1)*SectorBytes])
		sub := make([]byte, chdSubBytes)
		for j := range 4 {
			sub[j*graphicsPacket] = graphicsCommand
			sub[j*graphicsPacket+1] = byte(i+j) & 0x3f
		}
		bin.Write(sub)
		want = append(want, sub...)
	}
	write(t, dir, "k.bin", bin.Bytes())
	toc := write(t, dir, "k.toc", []byte("CD_DA\nTRACK AUDIO RW_RAW\nDATAFILE \"k.bin\" 00:01:25\n"))
	out := filepath.Join(dir, "k.chd")
	if b, err := exec.Command(chdman, "createcd", "-f", "-i", toc, "-o", out).CombinedOutput(); err != nil {
		t.Fatalf("chdman: %v\n%s", err, b)
	}
	d, s := openStream(t, out)
	if !d.HasGraphics {
		t.Fatalf("no graphics found: %+v", d)
	}
	got, err := d.ReadGraphics(0, sectors)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("the graphics differ")
	}
	if s.Samples() != sectors*SectorSamples {
		t.Fatalf("%d samples", s.Samples())
	}
}

// Remakes what is in testdata: run with WITTYCD_CHDMAN and WITTYCD_MAKE_FIXTURES set,
// and ffmpeg on the path. Only generated tones go in.
func TestChdmanFixtures(t *testing.T) {
	chdman := os.Getenv("WITTYCD_CHDMAN")
	if chdman == "" || os.Getenv("WITTYCD_MAKE_FIXTURES") == "" {
		t.Skip("this only remakes the kept test images")
	}
	dir := t.TempDir()
	write(t, dir, "x.bin", intTone(0, 24))
	cue := write(t, dir, "x.cue", []byte("FILE \"x.bin\" BINARY\nTRACK 01 AUDIO\nINDEX 01 00:00:00\nTRACK 02 AUDIO\nINDEX 00 00:00:10\nINDEX 01 00:00:14\n"))
	run := func(name string, args ...string) {
		t.Helper()
		if b, err := exec.Command(name, args...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, b)
		}
	}
	for _, codec := range []string{"cdlz", "cdzl", "cdfl"} {
		run(chdman, "createcd", "-f", "-i", cue, "-o", filepath.Join("testdata", "tones_"+codec+".chd"), "-c", codec)
	}
	// Sync and nothing else is a sector whose error correction checks out, so chdman
	// marks it in the hunk's bitmap. With only two of them, audio shares that hunk.
	data := make([]byte, 2*SectorBytes)
	for i := range 2 {
		copy(data[i*SectorBytes+1:], bytes.Repeat([]byte{0xff}, 10))
	}
	write(t, dir, "m.bin", append(data, intTone(0, 12)...))
	mixed := write(t, dir, "m.cue", []byte("FILE \"m.bin\" BINARY\nTRACK 01 MODE1/2352\nINDEX 01 00:00:00\nTRACK 02 AUDIO\nINDEX 01 00:00:02\n"))
	run(chdman, "createcd", "-f", "-i", mixed, "-o", filepath.Join("testdata", "mixed.chd"), "-c", "cdlz")
	raw := write(t, dir, "lpc.raw", intTone(0, 40))
	run("ffmpeg", "-y", "-v", "error", "-f", "s16le", "-ar", "44100", "-ac", "2", "-i", raw, "-compression_level", "12", filepath.Join("testdata", "lpc.flac"))
}

// How fast a real image plays back, in each of the usual forms.
func BenchmarkChdman(b *testing.B) {
	chdman := os.Getenv("WITTYCD_CHDMAN")
	if chdman == "" {
		b.Skip("WITTYCD_CHDMAN doesn't name chdman")
	}
	dir := b.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.bin"), intTone(0, 22500), 0o644); err != nil {
		b.Fatal(err)
	}
	cue := filepath.Join(dir, "x.cue")
	if err := os.WriteFile(cue, []byte("FILE \"x.bin\" BINARY\nTRACK 01 AUDIO\nINDEX 01 00:00:00\n"), 0o644); err != nil {
		b.Fatal(err)
	}
	for _, codec := range []string{"cdlz", "cdzl", "cdfl"} {
		out := filepath.Join(dir, codec+".chd")
		if m, err := exec.Command(chdman, "createcd", "-f", "-i", cue, "-o", out, "-c", codec).CombinedOutput(); err != nil {
			b.Fatalf("chdman: %v\n%s", err, m)
		}
		b.Run(codec, func(b *testing.B) {
			d, err := Open(out)
			if err != nil {
				b.Fatal(err)
			}
			defer d.Close()
			s, _ := d.NewStream(nil, 1, false)
			buf := make([]byte, 64<<10)
			b.SetBytes(s.Size())
			c := d.extents[0].src.(*chdFile)
			for b.Loop() {
				for off := int64(0); off < s.Size(); off += int64(len(buf)) {
					s.ReadAt(buf, off)
				}
				for j := range c.cache {
					c.cache[j].num = -1
				}
			}
		})
	}
}

func TestHuffmanRefusesNonsense(t *testing.T) {
	if _, ok := newHuffman([]uint8{1, 1, 1}, 8); ok {
		t.Error("three codes of one bit can't exist")
	}
	if _, ok := newHuffman([]uint8{9}, 8); ok {
		t.Error("a code longer than the limit")
	}
	// A code the table doesn't have is an error, not a loop.
	h, ok := newHuffman([]uint8{2, 2}, 8)
	if ok {
		if _, ok := h.decode(&flacBits{b: []byte{0xff}}); ok {
			t.Error("a code that doesn't exist was decoded")
		}
	}
}

// A real raw LZMA stream from liblzma: lc=3, lp=0, pb=2, with matches at new
// distances, repeats and literals after matches.
const lzmaVector = "003a1a08ce76c7e5e9d60734c3d10ebfce55e1aabde0d53fc90ddc5631300e113949adc0bc3f1c663c2c9e4ccd9a38a918ac069cd70d96b592a749c3672b627ee9b3defd309e2523fca21ac801a0070930043d3eba7c033bfcc7fb045f503c411612098406203ef9c45db87f271a0e5fbe92cc02e9de38e52a0e698088b2ba6c074f360504ea6d13fe012a2c5ca15be0d33e758988548a952d3ccd0796a149e3d0e84fd25860a683e23b7c1016d0b897f9a702806744c1503430c2daed5b81474f8a0911219b643450b4f41bc848dd13663d0cf4bac5a559562027f100403971f47828dd3e3b0015422329b588daf51ac54dd2138d4e52a39abc5f6fa165d59ba2f76619566a6f6cadb22d7a6cecda834e12c5c7d6a5d4da947e5d540aecefa32ff964e420974a25983fb16d25203cf8a3c6e99cdbefb385286fb571e44cbd967e1cfa900eb065bbd144f1923ff84fbf67b864070cba6ed5a153c44116b03507d5094c3a9936df12db83e1de36931159426a0af0052bfa304b4b12932cd0ff6be135e4685a70cd026809698205fb82303a4b0cd15ee5fcde4531fbc16447f7ba4c2fb104301be6fa1adadced4d9768af6186d08181d60339951f849bb79ef4ccea1bac92c365360d083a07dd251fb5a6b286a501554e44fc5798abc76262bc4008af326d32a6b3eae532d52e218444d80cdcda599cba5600b9db593221517304aa4bfffc582501"

func TestLZMAVector(t *testing.T) {
	src, err := hex.DecodeString(lzmaVector)
	if err != nil {
		t.Fatal(err)
	}
	want := lzmaPlain()
	got := make([]byte, len(want))
	var z lzmaDecoder
	if err := z.decode(got, src, 3, 0, 2); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("the output differs")
	}
	// Used again it starts fresh.
	if err := z.decode(got, src, 3, 0, 2); err != nil || !bytes.Equal(got, want) {
		t.Fatal("a second use differs")
	}
	// Cut short or damaged: an error or wrong bytes, and nothing worse.
	for i := range src {
		if err := z.decode(got, src[:i], 3, 0, 2); err == nil && i < len(src)-16 {
			t.Errorf("cut to %d of %d bytes and no error", i, len(src))
		}
	}
	for i := range src {
		bad := bytes.Clone(src)
		bad[i] ^= 0x55
		z.decode(got, bad, 3, 0, 2)
	}
	for _, s := range [][3]uint{{9, 0, 2}, {3, 5, 2}, {3, 0, 5}} {
		if z.decode(got, src, s[0], s[1], s[2]) == nil {
			t.Errorf("settings %v should be refused", s)
		}
	}
}

// lzmaPlain is what the vector decodes to.
func lzmaPlain() []byte {
	var b bytes.Buffer
	for i := range 400 {
		switch i % 7 {
		case 0, 1:
			fmt.Fprintf(&b, "the quick brown fox %d ", i%13)
		case 2:
			b.WriteString(strings.Repeat("ab", 1+i%40))
		case 3:
			b.WriteByte(byte(uint64(i) * 2654435761 >> 11))
		default:
			fmt.Fprintf(&b, "jumps over the lazy dog %d\n", i%5)
		}
	}
	return b.Bytes()
}
