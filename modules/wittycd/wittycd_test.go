package wittycd

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tests use tones generated here. No audio is kept in the repository.

// tone makes sectors of a continuous tone whose phase carries on from sample
// start, so any join that loses or repeats a sample shows up exactly.
func tone(start int64, sectors int, freq float64) []byte {
	out := make([]byte, sectors*SectorBytes)
	for i := range sectors * SectorSamples {
		at := float64(start+int64(i)) / SampleRate
		l := int16(9000*math.Sin(2*math.Pi*freq*at) + 3000*math.Sin(2*math.Pi*freq*2.5*at))
		r := int16(9000 * math.Cos(2*math.Pi*freq*at))
		binary.LittleEndian.PutUint16(out[i*4:], uint16(l))
		binary.LittleEndian.PutUint16(out[i*4+2:], uint16(r))
	}
	return out
}

func write(t *testing.T, dir, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMusicBrainzID(t *testing.T) {
	// The worked example on MusicBrainz's page about disc IDs.
	got := TOC{First: 1, Last: 6, LeadOut: 95462, Offsets: []int{150, 15363, 32314, 46592, 63414, 80489}}
	if id := got.MusicBrainzID(); id != "49HHV7Eb8UKF3aQiNmu1GR8vKTY-" {
		t.Fatalf("id = %s", id)
	}
	// Their CD-Extra: seven audio tracks, a data session after an 11400-sector gap.
	d := &Disc{}
	offs := []int{150, 14109, 33586, 53077, 65781, 77892, 99174}
	for i, o := range offs {
		end := 125824 - sessionGap
		if i+1 < len(offs) {
			end = offs[i+1] - 150
		}
		d.Tracks = append(d.Tracks, Track{Number: i + 1, Audio: true, Start: o - 150, Pregap: o - 150, End: end})
	}
	d.Tracks = append(d.Tracks, Track{Number: 8, Start: 125824, Pregap: 125824, End: 200000})
	toc := d.TOC()
	if toc.Last != 7 || toc.LeadOut != 114574 {
		t.Fatalf("toc = %+v", toc)
	}
	if id := toc.MusicBrainzID(); id != "BPnh1KU.hea1C.KMYWLGZkHJr0w-" {
		t.Fatalf("CD-Extra id = %s", id)
	}
}

func TestCDDBID(t *testing.T) {
	toc := TOC{First: 1, Last: 2, LeadOut: 75*60 + 150, Offsets: []int{150, 75*30 + 150}}
	// Seconds at the tracks: 2 and 32, digits 2+3+2=7; length 60; 2 tracks.
	if got := toc.CDDBID(); got != "07003c02" {
		t.Fatalf("cddb = %s", got)
	}
}

func TestParseMSF(t *testing.T) {
	if v, ok := parseMSF("01:02:03"); !ok || v != (62*75+3) {
		t.Fatalf("msf = %d %v", v, ok)
	}
	for _, bad := range []string{"", "1:2", "aa:bb:cc", "00:00:75", "-1:00:00"} {
		if _, ok := parseMSF(bad); ok {
			t.Errorf("%q should not parse", bad)
		}
	}
}

// cueDisc writes the generated test disc: three tracks of one unbroken tone with a
// 2-second pregap before track 2, whose audio carries on through it.
func cueDisc(t *testing.T) (string, []byte) {
	t.Helper()
	dir := t.TempDir()
	pcm := tone(0, 1500, 440)
	write(t, dir, "Album.bin", pcm)
	cue := `REM GENRE "Test Tones"
REM DATE 2026
PERFORMER "The Generators"
TITLE "Test Pressing"
FILE "Album.bin" BINARY
  TRACK 01 AUDIO
    TITLE "One"
    PERFORMER "A Sine"
    FLAGS DCP
    ISRC USTST2600001
    INDEX 01 00:00:00
  TRACK 02 AUDIO
    TITLE "Two"
    INDEX 00 00:06:00
    INDEX 01 00:08:00
    INDEX 02 00:10:00
  TRACK 03 AUDIO
    TITLE "Three"
    FLAGS PRE
    INDEX 01 00:15:00
`
	return write(t, dir, "Album.cue", []byte(cue)), pcm
}

func TestOpenCue(t *testing.T) {
	path, _ := cueDisc(t)
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.Format != "cue" || d.Text.Title != "Test Pressing" || d.Text.Performer != "The Generators" || d.Genre != "Test Tones" || d.Date != "2026" {
		t.Fatalf("disc = %+v", d)
	}
	if len(d.Tracks) != 3 {
		t.Fatalf("tracks = %+v", d.Tracks)
	}
	want := []struct{ pregap, start, end int }{{0, 0, 6 * 75}, {6 * 75, 8 * 75, 15 * 75}, {15 * 75, 15 * 75, 1500}}
	for i, w := range want {
		tr := d.Tracks[i]
		if tr.Pregap != w.pregap || tr.Start != w.start || tr.End != w.end {
			t.Errorf("track %d = %+v, want %+v", i+1, tr, w)
		}
	}
	if !d.Tracks[0].CopyAllowed || d.Tracks[0].ISRC != "USTST2600001" || d.Tracks[0].Text.Title != "One" || d.Tracks[0].Text.Performer != "A Sine" {
		t.Errorf("track 1 = %+v", d.Tracks[0])
	}
	if !d.Tracks[2].PreEmphasis {
		t.Error("track 3 should be pre-emphasised")
	}
	if len(d.Tracks[1].Indexes) != 1 || d.Tracks[1].Indexes[0].Number != 2 || d.Tracks[1].Indexes[0].LBA != 10*75 {
		t.Errorf("indexes = %+v", d.Tracks[1].Indexes)
	}
	if got := d.TOC().Offsets; got[0] != 150 || got[1] != 8*75+150 {
		t.Errorf("toc = %v", got)
	}
}

func TestWholeDiscIsTheImage(t *testing.T) {
	path, pcm := cueDisc(t)
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s, err := d.NewStream(nil, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]byte, s.Size())
	if _, err := s.ReadAt(got, 0); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if !bytes.Equal(got, pcm) {
		t.Fatal("the whole-disc stream differs from the image: the joins aren't seamless")
	}
	// Any range at all, including part way through a sample.
	for _, r := range [][2]int64{{0, 1}, {3, 5}, {1000, 4097}, {2352*450 - 3, 9}, {s.Size() - 2, 2}} {
		b := make([]byte, r[1])
		if _, err := s.ReadAt(b, r[0]); err != nil && err != io.EOF {
			t.Fatal(err)
		}
		if !bytes.Equal(b, pcm[r[0]:r[0]+r[1]]) {
			t.Fatalf("range %v differs", r)
		}
	}
}

func TestProgramOrder(t *testing.T) {
	path, pcm := cueDisc(t)
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s, err := d.NewStream([]int{3, 1}, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	three := pcm[15*75*SectorBytes:]
	one := pcm[:6*75*SectorBytes]
	want := bytes.Join([][]byte{three, one, three, one}, nil)
	got := make([]byte, s.Size())
	if _, err := s.ReadAt(got, 0); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("the program isn't the tracks in order")
	}
	if _, err := d.NewStream([]int{9}, 1, false); err == nil {
		t.Fatal("a track that isn't there should be refused")
	}
}

func TestRipAPI(t *testing.T) {
	path, pcm := cueDisc(t)
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	// Track 3 is flagged pre-emphasis, and a rip keeps it as it is on the disc.
	s, err := d.TrackStream(3)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(s.Reader())
	if err != nil || !bytes.Equal(got, pcm[15*75*SectorBytes:]) {
		t.Fatalf("track 3: %d bytes, %v", len(got), err)
	}
	r, err := d.RangeStream(450, 600) // track 2's pregap
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := io.ReadAll(r.Reader()); !bytes.Equal(got, pcm[450*SectorBytes:600*SectorBytes]) {
		t.Fatal("the run of sectors differs")
	}
	if _, err := d.RangeStream(5, 2); err == nil {
		t.Fatal("a backwards run should be refused")
	}
}

func TestWAV(t *testing.T) {
	path, pcm := cueDisc(t)
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s, _ := d.NewStream(nil, 1, false)
	w := s.WAV(588)
	b, err := io.ReadAll(io.NewSectionReader(w, 0, w.Size()))
	if err != nil {
		t.Fatal(err)
	}
	if string(b[:4]) != "RIFF" || binary.LittleEndian.Uint32(b[40:]) != uint32(len(pcm)-SectorBytes) {
		t.Fatalf("header % x", b[:44])
	}
	if !bytes.Equal(b[44:], pcm[SectorBytes:]) {
		t.Fatal("the WAV's samples differ")
	}
}

// A file read backwards is many times rougher; the guess must correct it.
func TestByteOrderIsDetected(t *testing.T) {
	dir := t.TempDir()
	pcm := tone(0, 800, 440)
	swapped := bytes.Clone(pcm)
	for i := 0; i+1 < len(swapped); i += 2 {
		swapped[i], swapped[i+1] = swapped[i+1], swapped[i]
	}
	write(t, dir, "x.bin", swapped)
	cue := write(t, dir, "x.cue", []byte("FILE \"x.bin\" BINARY\nTRACK 01 AUDIO\nINDEX 01 00:00:00\n"))
	d, err := Open(cue)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s, _ := d.NewStream(nil, 1, false)
	got := make([]byte, s.Size())
	s.ReadAt(got, 0)
	if !bytes.Equal(got, pcm) {
		t.Fatal("big-endian audio wasn't turned round")
	}
	if !hasWarning(d, "byte-order") {
		t.Fatalf("warnings = %+v", d.Warnings)
	}
	// MOTOROLA says so itself.
	cue2 := write(t, dir, "y.cue", []byte("FILE \"x.bin\" MOTOROLA\nTRACK 01 AUDIO\nINDEX 01 00:00:00\n"))
	d2, err := Open(cue2)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	s2, _ := d2.NewStream(nil, 1, false)
	got2 := make([]byte, s2.Size())
	s2.ReadAt(got2, 0)
	if !bytes.Equal(got2, pcm) || hasWarning(d2, "byte-order") {
		t.Fatal("MOTOROLA should be read as big-endian, with nothing to report")
	}
}

func hasWarning(d *Disc, code string) bool {
	for _, w := range d.Warnings {
		if w.Code == code {
			return true
		}
	}
	return false
}

func TestHiddenTrack(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "h.bin", tone(0, 1000, 440))
	cue := write(t, dir, "h.cue", []byte(`FILE "h.bin" BINARY
TRACK 01 AUDIO
INDEX 00 00:00:00
INDEX 01 00:05:00
TRACK 02 AUDIO
INDEX 01 00:10:00
`))
	d, err := Open(cue)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if !d.HiddenTrack || d.Tracks[0].Pregap != 0 || d.Tracks[0].Start != 375 {
		t.Fatalf("hidden = %v, track 1 = %+v", d.HiddenTrack, d.Tracks[0])
	}
	s, err := d.NewStream([]int{HiddenTrackNumber}, 1, false)
	if err != nil || s.Samples() != 375*SectorSamples {
		t.Fatalf("hidden stream: %v %v", s, err)
	}
	// The same layout with a silent pregap is no hidden track.
	pcm := tone(0, 1000, 440)
	clear(pcm[:375*SectorBytes])
	write(t, dir, "s.bin", pcm)
	cue2 := write(t, dir, "s.cue", []byte(`FILE "s.bin" BINARY
TRACK 01 AUDIO
INDEX 00 00:00:00
INDEX 01 00:05:00
`))
	d2, err := Open(cue2)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	if d2.HiddenTrack {
		t.Fatal("silence isn't a hidden track")
	}
}

func TestMixedModeAndExtra(t *testing.T) {
	dir := t.TempDir()
	data := bytes.Repeat([]byte{0, 1, 2, 3}, 300*SectorBytes/4)
	write(t, dir, "m.bin", append(bytes.Clone(data), tone(0, 750, 440)...))
	cue := write(t, dir, "m.cue", []byte(`FILE "m.bin" BINARY
TRACK 01 MODE1/2352
INDEX 01 00:00:00
TRACK 02 AUDIO
PREGAP 00:02:00
INDEX 01 00:04:00
`))
	d, err := Open(cue)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if len(d.Tracks) != 2 || d.Tracks[0].Audio || !d.Tracks[1].Audio {
		t.Fatalf("tracks = %+v", d.Tracks)
	}
	if !hasWarning(d, "data-track") {
		t.Fatalf("warnings = %+v", d.Warnings)
	}
	if got := d.Tracks[1]; got.Pregap != 300 || got.Start != 450 || got.End != 1200 {
		t.Fatalf("track 2 = %+v", got)
	}
	s, _ := d.NewStream(nil, 1, false)
	if s.Samples() != 900*SectorSamples {
		t.Fatalf("samples = %d: the data track must not be in the stream", s.Samples())
	}
	got := make([]byte, s.Size())
	s.ReadAt(got, 0)
	if !bytes.Equal(got[:150*SectorBytes], make([]byte, 150*SectorBytes)) {
		t.Fatal("the generated pregap should be silent")
	}
	if !bytes.Equal(got[150*SectorBytes:], tone(0, 750, 440)) {
		t.Fatal("the audio after the pregap differs")
	}
	// Only data: a clear refusal.
	cue2 := write(t, dir, "d.cue", []byte("FILE \"m.bin\" BINARY\nTRACK 01 MODE1/2352\nINDEX 01 00:00:00\n"))
	if _, err := Open(cue2); !errors.Is(err, ErrNoAudio) {
		t.Fatalf("err = %v", err)
	}
}

func TestMultiFileCueWithGapInPreviousFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "01.wav", wavFile(tone(0, 450, 440)))
	write(t, dir, "02.wav", wavFile(tone(450*SectorSamples, 300, 440)))
	cue := write(t, dir, "two.cue", []byte(`TITLE "Two Files"
FILE "01.wav" WAVE
  TRACK 01 AUDIO
    INDEX 01 00:00:00
  TRACK 02 AUDIO
    INDEX 00 00:05:00
FILE "02.wav" WAVE
    INDEX 01 00:00:00
`))
	d, err := Open(cue)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.Tracks[1].Pregap != 375 || d.Tracks[1].Start != 450 || d.Tracks[1].End != 750 {
		t.Fatalf("track 2 = %+v", d.Tracks[1])
	}
	s, _ := d.NewStream(nil, 1, false)
	got := make([]byte, s.Size())
	s.ReadAt(got, 0)
	if !bytes.Equal(got, tone(0, 750, 440)) {
		t.Fatal("two files should join without a seam")
	}
}

func wavFile(pcm []byte) []byte {
	var b bytes.Buffer
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(36+len(pcm)))
	b.WriteString("WAVEfmt ")
	binary.Write(&b, binary.LittleEndian, uint32(16))
	binary.Write(&b, binary.LittleEndian, uint16(1))
	binary.Write(&b, binary.LittleEndian, uint16(2))
	binary.Write(&b, binary.LittleEndian, uint32(SampleRate))
	binary.Write(&b, binary.LittleEndian, uint32(SampleRate*4))
	binary.Write(&b, binary.LittleEndian, uint16(4))
	binary.Write(&b, binary.LittleEndian, uint16(16))
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint32(len(pcm)))
	b.Write(pcm)
	return b.Bytes()
}

func TestWAVThatIsNotCDAudioIsRefused(t *testing.T) {
	dir := t.TempDir()
	w := wavFile(tone(0, 100, 440))
	binary.LittleEndian.PutUint32(w[24:], 48000)
	write(t, dir, "a.wav", w)
	cue := write(t, dir, "a.cue", []byte("FILE \"a.wav\" WAVE\nTRACK 01 AUDIO\nINDEX 01 00:00:00\n"))
	if _, err := Open(cue); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v", err)
	}
}

func TestSheetCantLeaveItsFolder(t *testing.T) {
	root := t.TempDir()
	inner := filepath.Join(root, "inner")
	os.Mkdir(inner, 0o755)
	secret := tone(0, 200, 440)
	write(t, root, "secret.bin", secret)
	for _, name := range []string{"../secret.bin", `..\secret.bin`, filepath.Join(root, "secret.bin"), "/etc/passwd", `C:\Windows\win.ini`} {
		cue := write(t, inner, "c.cue", []byte("FILE \""+name+"\" BINARY\nTRACK 01 AUDIO\nINDEX 01 00:00:00\n"))
		if d, err := Open(cue); err == nil {
			d.Close()
			t.Fatalf("%q opened a file outside the sheet's folder", name)
		}
	}
	// The same name beside the sheet is what was meant.
	write(t, inner, "secret.bin", secret)
	cue := write(t, inner, "c.cue", []byte("FILE \"../secret.bin\" BINARY\nTRACK 01 AUDIO\nINDEX 01 00:00:00\n"))
	d, err := Open(cue)
	if err != nil {
		t.Fatal(err)
	}
	d.Close()
}

func TestTruncatedImage(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "t.bin", tone(0, 500, 440))
	cue := write(t, dir, "t.cue", []byte(`FILE "t.bin" BINARY
TRACK 01 AUDIO
INDEX 01 00:00:00
TRACK 02 AUDIO
INDEX 01 00:05:00
TRACK 03 AUDIO
INDEX 01 00:20:00
`))
	d, err := Open(cue)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if len(d.Tracks) != 2 || d.Tracks[1].End != 500 {
		t.Fatalf("tracks = %+v", d.Tracks)
	}
	if !hasWarning(d, "truncated-tracks") {
		t.Fatalf("warnings = %+v", d.Warnings)
	}
}

func TestNoiseAndDTSAreCalledOut(t *testing.T) {
	dir := t.TempDir()
	noise := make([]byte, 800*SectorBytes)
	r := rand.New(rand.NewPCG(1, 2))
	for i := range noise {
		noise[i] = byte(r.Uint32())
	}
	write(t, dir, "n.bin", noise)
	cue := write(t, dir, "n.cue", []byte("FILE \"n.bin\" BINARY\nTRACK 01 AUDIO\nINDEX 01 00:00:00\n"))
	d, err := Open(cue)
	if err != nil {
		t.Fatal(err)
	}
	if d.Problem != "noise" {
		t.Fatalf("problem = %q", d.Problem)
	}
	d.Close()
	dts := bytes.Clone(noise)
	for i := 0; i+4 < len(dts); i += 2048 {
		copy(dts[i:], []byte{0xfe, 0x7f, 0x01, 0x80})
	}
	write(t, dir, "d.bin", dts)
	cue = write(t, dir, "d.cue", []byte("FILE \"d.bin\" BINARY\nTRACK 01 AUDIO\nINDEX 01 00:00:00\n"))
	if d, err = Open(cue); err != nil {
		t.Fatal(err)
	}
	if d.Problem != "dts" {
		t.Fatalf("problem = %q", d.Problem)
	}
	d.Close()
}

func TestRawAudioImage(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "music.iso", tone(0, 900, 440))
	d, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.Format != "raw" || len(d.Tracks) != 1 || d.Tracks[0].End != 900 || !hasWarning(d, "no-sheet") {
		t.Fatalf("disc = %+v", d)
	}
	// A data disc isn't audio, whatever its size.
	data := make([]byte, 900*SectorBytes)
	copy(data[16*2048:], "\x01CD001")
	if _, err := Open(write(t, dir, "data.iso", data)); !errors.Is(err, ErrNotCD) {
		t.Fatalf("err = %v", err)
	}
	// Noise of the right size isn't either.
	r := rand.New(rand.NewPCG(1, 2))
	for i := range data {
		data[i] = byte(r.Uint32())
	}
	if _, err := Open(write(t, dir, "rand.bin", data)); !errors.Is(err, ErrNotCD) {
		t.Fatalf("err = %v", err)
	}
	// Wrong size.
	if _, err := Open(write(t, dir, "short.iso", make([]byte, 100))); !errors.Is(err, ErrNotCD) {
		t.Fatalf("err = %v", err)
	}
}

func TestCueTextEncodings(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "e.bin", tone(0, 300, 440))
	body := "TITLE \"Caf\xe9 \x96 Live\"\nFILE \"e.bin\" BINARY\nTRACK 01 AUDIO\nINDEX 01 00:00:00\n"
	d, err := Open(write(t, dir, "win.cue", []byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	if d.Text.Title != "Café – Live" {
		t.Errorf("Windows text = %q", d.Text.Title)
	}
	d.Close()
	u8 := "\xef\xbb\xbfTITLE \"日本語\"\nFILE \"e.bin\" BINARY\nTRACK 01 AUDIO\nINDEX 01 00:00:00\n"
	d, err = Open(write(t, dir, "u8.cue", []byte(u8)))
	if err != nil {
		t.Fatal(err)
	}
	if d.Text.Title != "日本語" {
		t.Errorf("UTF-8 text = %q", d.Text.Title)
	}
	d.Close()
	var u16 bytes.Buffer
	u16.Write([]byte{0xff, 0xfe})
	for _, r := range "TITLE \"Ä\"\r\nFILE \"e.bin\" BINARY\r\nTRACK 01 AUDIO\r\nINDEX 01 00:00:00\r\n" {
		binary.Write(&u16, binary.LittleEndian, uint16(r))
	}
	d, err = Open(write(t, dir, "u16.cue", u16.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if d.Text.Title != "Ä" {
		t.Errorf("UTF-16 text = %q", d.Text.Title)
	}
	d.Close()
}

// makePacks writes CD-TEXT packs for the disc title and two tracks.
func makePacks(kind byte, strs ...string) []byte {
	text := strings.Join(strs, "\x00") + "\x00"
	var out []byte
	seq := byte(0)
	for i := 0; i < len(text); i += 12 {
		p := make([]byte, packBytes)
		p[0], p[1], p[2], p[3] = kind, 0, seq, 0
		copy(p[4:16], text[i:min(i+12, len(text))])
		crc := ^crcCCITT(p[:16])
		p[16], p[17] = byte(crc>>8), byte(crc)
		out = append(out, p...)
		seq++
	}
	return out
}

func TestCDTextFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "x.bin", tone(0, 800, 440))
	write(t, dir, "x.cdt", append(makePacks(0x80, "Long Disc Title Over Packs", "First Song", "Second Song"), makePacks(0x81, "An Artist", "\t", "Other Artist")...))
	cue := write(t, dir, "x.cue", []byte("FILE \"x.bin\" BINARY\nTRACK 01 AUDIO\nINDEX 01 00:00:00\nTRACK 02 AUDIO\nINDEX 01 00:05:00\n"))
	d, err := Open(cue)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.Text.Title != "Long Disc Title Over Packs" || d.Text.Performer != "An Artist" {
		t.Errorf("disc text = %+v", d.Text)
	}
	if d.Tracks[0].Text.Title != "First Song" || d.Tracks[1].Text.Title != "Second Song" {
		t.Errorf("tracks = %+v", d.Tracks)
	}
	if d.Tracks[0].Text.Performer != "An Artist" || d.Tracks[1].Text.Performer != "Other Artist" {
		t.Errorf("performers = %+v %+v", d.Tracks[0].Text, d.Tracks[1].Text)
	}
	// A pack with a bad checksum is ignored rather than believed.
	bad := makePacks(0x80, "Bad")
	bad[5] ^= 0xff
	var e Disc
	if e.parseCDText(bad) != 0 || e.Text.Title != "" {
		t.Error("a damaged pack was believed")
	}
}

func TestCDGSidecar(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "k.bin", tone(0, 400, 440))
	cdg := make([]byte, 400*96)
	cdg[0], cdg[1] = 0x09, 1
	write(t, dir, "k.cdg", cdg)
	cue := write(t, dir, "k.cue", []byte("FILE \"k.bin\" BINARY\nTRACK 01 AUDIO\nINDEX 01 00:00:00\n"))
	d, err := Open(cue)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if !d.HasGraphics {
		t.Fatal("no graphics found")
	}
	b, err := d.ReadGraphics(0, 2)
	if err != nil || len(b) != 192 || b[0] != 9 || b[1] != 1 {
		t.Fatalf("graphics = % x %v", b[:4], err)
	}
}

// The pre-emphasis correction must match the standard's curve.
func TestDeemphasisCurve(t *testing.T) {
	const fs = SampleRate
	for _, f := range []float64{20, 100, 1000, 3000, 5000, 10000, 15000, 20000} {
		w := 2 * math.Pi * f / fs
		// H(z) = (b0 + b1 z^-1) / (1 + a1 z^-1)
		nr, ni := deemphB0+deemphB1*math.Cos(w), -deemphB1*math.Sin(w)
		dr, di := 1+deemphA1*math.Cos(w), -deemphA1*math.Sin(w)
		got := 10 * math.Log10((nr*nr+ni*ni)/(dr*dr+di*di))
		o := 2 * math.Pi * f
		t1, t2 := 50e-6, 15e-6
		want := 10 * math.Log10((1+o*o*t2*t2)/(1+o*o*t1*t1))
		if math.Abs(got-want) > 0.15 {
			t.Errorf("%.0f Hz: %.2f dB, the standard says %.2f dB", f, got, want)
		}
	}
}

func TestDeemphasisAppliesAndIsStateless(t *testing.T) {
	path, pcm := cueDisc(t)
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s, _ := d.NewStream(nil, 1, true)
	got := make([]byte, s.Size())
	s.ReadAt(got, 0)
	if !bytes.Equal(got[:15*75*SectorBytes], pcm[:15*75*SectorBytes]) {
		t.Fatal("tracks without the flag must be untouched")
	}
	if bytes.Equal(got[15*75*SectorBytes:], pcm[15*75*SectorBytes:]) {
		t.Fatal("the flagged track wasn't corrected")
	}
	// Reading from the middle gives the same samples as reading through.
	at := int64(17*75*SectorBytes + 1000)
	part := make([]byte, 40000)
	s.ReadAt(part, at)
	if !bytes.Equal(part, got[at:at+int64(len(part))]) {
		t.Fatal("seeking changes the corrected samples")
	}
}

func TestFLACFrameHeaderAndSpeed(t *testing.T) {
	path, _ := cueDisc(t)
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s, _ := d.NewStream(nil, 1, false)
	var out bytes.Buffer
	if err := s.WriteFLAC(&out, 0); err != nil {
		t.Fatal(err)
	}
	b := out.Bytes()
	if string(b[:4]) != "fLaC" || b[4] != 0x80 || b[42] != 0xff || b[43] != 0xf8 {
		t.Fatalf("head % x", b[:48])
	}
	if out.Len() >= int(s.Size()) {
		t.Fatalf("no smaller than PCM: %d of %d", out.Len(), s.Size())
	}
	// Starting later shortens the stream's own idea of its length.
	var later bytes.Buffer
	s.WriteFLAC(&later, s.Samples()/2)
	total := binary.BigEndian.Uint64(later.Bytes()[18:]) & (1<<36 - 1)
	if int64(total) != s.Samples()-s.Samples()/2 {
		t.Fatalf("total = %d", total)
	}
}

func BenchmarkFLAC(b *testing.B) {
	pcm := tone(0, 7500, 440)
	r := rand.New(rand.NewPCG(1, 2))
	for i := 0; i+1 < len(pcm); i += 2 {
		v := int32(int16(binary.LittleEndian.Uint16(pcm[i:]))) + int32(r.IntN(200)) - 100
		binary.LittleEndian.PutUint16(pcm[i:], uint16(int16(v)))
	}
	e := newFlacEncoder()
	b.SetBytes(flacBlock * SampleBytes)
	for b.Loop() {
		e.encodeFrame(pcm[:flacBlock*SampleBytes], 1)
	}
}
