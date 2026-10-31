package wittycd

import (
	"bytes"
	"errors"
	"testing"
)

// swapped is audio as a raw TOC file expects it: big-endian.
func swapped(pcm []byte) []byte {
	out := bytes.Clone(pcm)
	for i := 0; i+1 < len(out); i += 2 {
		out[i], out[i+1] = out[i+1], out[i]
	}
	return out
}

func wholeDisc(t *testing.T, d *Disc) []byte {
	t.Helper()
	s, err := d.NewStream(nil, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]byte, s.Size())
	if _, err := s.ReadAt(got, 0); err != nil {
		t.Fatal(err)
	}
	return got
}

func checkLayout(t *testing.T, d *Disc, want [][3]int) {
	t.Helper()
	if len(d.Tracks) != len(want) {
		t.Fatalf("tracks = %+v", d.Tracks)
	}
	for i, w := range want {
		tr := d.Tracks[i]
		if tr.Pregap != w[0] || tr.Start != w[1] || tr.End != w[2] {
			t.Errorf("track %d = pregap %d, start %d, end %d, want %v", tr.Number, tr.Pregap, tr.Start, tr.End, w)
		}
	}
}

const tocSheet = `CD_DA // audio only
CATALOG "1234567890123"
CD_TEXT {
  LANGUAGE_MAP {
    0: EN
    1 : 9
  }
  LANGUAGE 0 {
    TITLE "Test \"Quoted\" Pressing"
    PERFORMER "Caf\351"
    SONGWRITER "A Writer"
    COMPOSER "A Composer"
    ARRANGER "An Arranger"
    MESSAGE "A Message"
    DISC_ID "XY12345"
    GENRE "Test Tones"
    TOC_INFO1 { 1, 2, 3 }
    SIZE_INFO { 0, 10, 255 }
  }
  LANGUAGE 1 {
    TITLE "Not This"
  }
}

TRACK AUDIO
COPY
NO PRE_EMPHASIS
TWO_CHANNEL_AUDIO
ISRC "USTST2600001"
CD_TEXT {
  LANGUAGE 0 {
    TITLE "One" PERFORMER "A Sine"
  }
}
FILE "x.bin" 0 00:04:00

TRACK AUDIO
NO COPY
FOUR_CHANNEL_AUDIO
AUDIOFILE "x.bin" 00:04:00 00:02:00
START
AUDIOFILE "x.bin" 00:06:00 88200
INDEX 00:01:00

TRACK AUDIO
PRE_EMPHASIS
PREGAP 00:01:00
FILE "x.bin" 00:08:00
`

func TestOpenTocFile(t *testing.T) {
	dir := t.TempDir()
	pcm := tone(0, 800, 440)
	write(t, dir, "x.bin", swapped(pcm))
	d, err := Open(write(t, dir, "x.toc", []byte(tocSheet)))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.Format != "toc" || d.Catalog != "1234567890123" || d.Genre != "Test Tones" {
		t.Fatalf("disc = %+v", d)
	}
	want := Text{Title: `Test "Quoted" Pressing`, Performer: "Café", Songwriter: "A Writer", Composer: "A Composer", Arranger: "An Arranger", Message: "A Message"}
	if d.Text != want {
		t.Errorf("text = %+v", d.Text)
	}
	checkLayout(t, d, [][3]int{{0, 0, 300}, {300, 450, 600}, {600, 675, 875}})
	one, two, three := d.Tracks[0], d.Tracks[1], d.Tracks[2]
	if !one.CopyAllowed || one.PreEmphasis || one.FourChannel || one.ISRC != "USTST2600001" || one.Text.Title != "One" || one.Text.Performer != "A Sine" {
		t.Errorf("track 1 = %+v", one)
	}
	if two.CopyAllowed || !two.FourChannel {
		t.Errorf("track 2 = %+v", two)
	}
	if len(two.Indexes) != 1 || two.Indexes[0] != (Index{2, 525}) {
		t.Errorf("indexes = %+v", two.Indexes)
	}
	if !three.PreEmphasis {
		t.Errorf("track 3 = %+v", three)
	}
	if hasWarning(d, "byte-order") {
		t.Error("big-endian is what a raw file in a TOC file should be")
	}
	got := wholeDisc(t, d)
	wantPCM := bytes.Join([][]byte{pcm[:600*SectorBytes], make([]byte, 75*SectorBytes), pcm[600*SectorBytes:]}, nil)
	if !bytes.Equal(got, wantPCM) {
		t.Fatal("the whole-disc stream differs from the image")
	}
}

func TestTocFileByteOrder(t *testing.T) {
	dir := t.TempDir()
	pcm := tone(0, 400, 440)

	// SWAP says so, and is believed.
	write(t, dir, "le.bin", pcm)
	d, err := Open(write(t, dir, "s.toc", []byte("CD_DA\nTRACK AUDIO\nFILE \"le.bin\" SWAP 0\n")))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(wholeDisc(t, d), pcm) || hasWarning(d, "byte-order") {
		t.Error("SWAP should read the file as little-endian")
	}
	d.Close()

	// Without it the guess is big-endian, and the audio puts that right.
	d, err = Open(write(t, dir, "g.toc", []byte("CD_DA\nTRACK AUDIO\nFILE \"le.bin\" 0\n")))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(wholeDisc(t, d), pcm) || !hasWarning(d, "byte-order") {
		t.Error("a wrong guess at the byte order should be corrected and reported")
	}
	d.Close()

	// A WAV file says its own order, and start and length count inside its samples.
	write(t, dir, "w.wav", wavFile(pcm))
	d, err = Open(write(t, dir, "w.toc", []byte("TRACK AUDIO\nFILE \"w.wav\" 00:01:00 58800\n")))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(wholeDisc(t, d), pcm[75*SectorBytes:175*SectorBytes]) {
		t.Error("the WAV's samples differ")
	}
	d.Close()

	// A byte offset skips a header in a raw file.
	write(t, dir, "h.bin", append(make([]byte, 1000), swapped(pcm)...))
	d, err = Open(write(t, dir, "h.toc", []byte("TRACK AUDIO\nFILE \"h.bin\" #1000 0\n")))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(wholeDisc(t, d), pcm) {
		t.Error("the byte offset was not honoured")
	}
	d.Close()
}

// A start in the sheet's sample counts doesn't have to be on a sector boundary.
func TestTocFileSamplePositions(t *testing.T) {
	dir := t.TempDir()
	pcm := tone(0, 400, 440)
	write(t, dir, "x.bin", swapped(pcm))
	d, err := Open(write(t, dir, "x.toc", []byte("TRACK AUDIO\nFILE \"x.bin\" 100 117600\nSILENCE 588\n")))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	checkLayout(t, d, [][3]int{{0, 0, 201}})
	got := wholeDisc(t, d)
	if !bytes.Equal(got[:200*SectorBytes], pcm[400:400+200*SectorBytes]) {
		t.Error("a start in samples landed in the wrong place")
	}
	if !bytes.Equal(got[200*SectorBytes:], make([]byte, SectorBytes)) {
		t.Error("the silence isn't silent")
	}
}

func TestTocFileStartWithLength(t *testing.T) {
	dir := t.TempDir()
	pcm := tone(0, 600, 440)
	write(t, dir, "x.bin", swapped(pcm))
	d, err := Open(write(t, dir, "x.toc", []byte("CD_DA\nTRACK AUDIO\nFILE \"x.bin\" 0\nSTART 00:02:00\n")))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	checkLayout(t, d, [][3]int{{0, 150, 600}})
	if !d.HiddenTrack {
		t.Error("audio before track 1's index 1 is a hidden track")
	}
	if !bytes.Equal(wholeDisc(t, d), pcm) {
		t.Error("the pregap should be in the whole-disc stream")
	}
}

func TestTocFileDataTracks(t *testing.T) {
	dir := t.TempDir()
	pcm := tone(0, 400, 440)
	write(t, dir, "x.bin", swapped(pcm))
	write(t, dir, "d.bin", bytes.Repeat([]byte{0x5a}, 100*2048))
	write(t, dir, "e.bin", bytes.Repeat([]byte{0xa5}, 50*2336))
	d, err := Open(write(t, dir, "m.toc", []byte(`CD_ROM
TRACK MODE1 RW
DATAFILE "d.bin" #0
TRACK MODE1
DATAFILE "d.bin" 00:01:00
ZERO MODE1 00:02:00 // post-gap
TRACK AUDIO
SILENCE 00:02:00
START
FILE "x.bin" 0
TRACK MODE2_FORM_MIX
DATAFILE "e.bin" 116800
`)))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	// d.bin is 95.5 sectors of 2144 bytes for the first track, which has subcode.
	checkLayout(t, d, [][3]int{{0, 0, 96}, {96, 96, 321}, {321, 471, 871}, {871, 871, 921}})
	if d.Tracks[0].Audio || d.Tracks[0].Mode != "MODE1/2048" || !d.Tracks[2].Audio || d.Tracks[3].Mode != "MODE2/2336" {
		t.Fatalf("tracks = %+v", d.Tracks)
	}
	if !hasWarning(d, "data-track") {
		t.Errorf("warnings = %+v", d.Warnings)
	}
	got := wholeDisc(t, d)
	if !bytes.Equal(got, append(make([]byte, 150*SectorBytes), pcm...)) {
		t.Fatal("a data track must not reach the stream")
	}
}

func TestTocFileDamage(t *testing.T) {
	dir := t.TempDir()
	pcm := tone(0, 300, 440)
	write(t, dir, "x.bin", swapped(pcm))

	// A file shorter than the sheet says.
	d, err := Open(write(t, dir, "short.toc", []byte(`TRACK AUDIO
FILE "x.bin" 0 00:02:00
TRACK AUDIO
FILE "x.bin" 00:02:00 00:10:00
TRACK AUDIO
FILE "x.bin" 00:12:00
`)))
	if err != nil {
		t.Fatal(err)
	}
	checkLayout(t, d, [][3]int{{0, 0, 150}, {150, 150, 300}})
	if !hasWarning(d, "truncated") {
		t.Errorf("warnings = %+v", d.Warnings)
	}
	d.Close()

	// Nonsense among good statements costs only itself.
	d, err = Open(write(t, dir, "odd.toc", []byte(`CD_TEXT { LANGUAGE_MAP { LANGUAGE 7 BROKEN } }
SILENCE 00:01:00
TRACK AUDIO
SILENCE 99:99:99
SILENCE 99999999999999999999
ZERO FISH
INDEX 00:99:00
INDEX notapos
FILE "x.bin" 0 123456789012345
FILE "x.bin" #99999999999999999999 0
FILE "x.bin" #-1 0
FIFO "/dev/zero" 00:01:00
FILE "x.bin" 0
UNHEARD OF WORDS { WITH { BRACES } }
"a stray string"
TRACK
`)))
	if err != nil {
		t.Fatal(err)
	}
	if d.Tracks[0].Start != 0 || !hasWarning(d, "bad-lengths") || !hasWarning(d, "fifo") {
		t.Errorf("disc = %+v", d)
	}
	d.Close()

	// Nothing usable.
	for name, sheet := range map[string]string{
		"empty.toc":   "",
		"words.toc":   "CD_DA\nthis is not a toc file\n",
		"binary.toc":  "\x00\xff\x00\xfe{{{{{\"",
		"unended.toc": "CD_TEXT { LANGUAGE 0 { TITLE \"never ends",
		"bare.toc":    "TRACK",
	} {
		if _, err := Open(write(t, dir, name, []byte(sheet))); !errors.Is(err, ErrNotCD) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, err := Open(write(t, dir, "none.toc", []byte("TRACK AUDIO\n"))); !errors.Is(err, ErrNotCD) {
		t.Errorf("a track with nothing in it: err = %v", err)
	}
	if _, err := Open(write(t, dir, "gone.toc", []byte("TRACK AUDIO\nFILE \"gone.bin\" 0\n"))); err == nil {
		t.Error("a file that isn't there should be an error")
	}

	// Only data.
	if _, err := Open(write(t, dir, "data.toc", []byte("TRACK MODE1\nDATAFILE \"x.bin\"\n"))); !errors.Is(err, ErrNoAudio) {
		t.Errorf("err = %v", err)
	}

	// Too many tracks and too many pieces are cut off, not obeyed.
	many := bytes.Repeat([]byte("TRACK AUDIO\nSILENCE 00:00:01\n"), 300)
	d, err = Open(write(t, dir, "many.toc", many))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Tracks) > MaxTracks {
		t.Errorf("%d tracks", len(d.Tracks))
	}
	d.Close()
	pieces := append([]byte("TRACK AUDIO\n"), bytes.Repeat([]byte("SILENCE 1\n"), maxTocPieces+50)...)
	d, err = Open(write(t, dir, "pieces.toc", pieces))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.extents) > maxTocPieces || !hasWarning(d, "too-many-pieces") {
		t.Errorf("%d extents, warnings = %+v", len(d.extents), d.Warnings)
	}
	d.Close()
}

func TestTocFileCantLeaveItsFolder(t *testing.T) {
	root := t.TempDir()
	inner := root + "/inner"
	if err := mkdir(inner); err != nil {
		t.Fatal(err)
	}
	write(t, root, "secret.bin", swapped(tone(0, 200, 440)))
	for _, name := range []string{"../secret.bin", `..\\secret.bin`, root + "/secret.bin", "/etc/passwd"} {
		if d, err := Open(write(t, inner, "c.toc", []byte("TRACK AUDIO\nFILE \""+name+"\" 0\n"))); err == nil {
			d.Close()
			t.Errorf("%q opened a file outside the sheet's folder", name)
		}
	}
}

func TestTocFileRenamedRip(t *testing.T) {
	dir := t.TempDir()
	pcm := tone(0, 300, 440)
	write(t, dir, "rip.bin", swapped(pcm))
	d, err := Open(write(t, dir, "rip.toc", []byte("TRACK AUDIO\nFILE \"old name.bin\" 0\n")))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if !bytes.Equal(wholeDisc(t, d), pcm) {
		t.Fatal("a sheet's only file should be found under the sheet's name")
	}
}
