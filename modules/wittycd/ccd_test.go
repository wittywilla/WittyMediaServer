package wittycd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func mkdir(p string) error { return os.Mkdir(p, 0o755) }

// ccdEntryText writes one [Entry n] section.
func ccdEntryText(n, session, point, control, plba int) string {
	return fmt.Sprintf("[Entry %d]\nSession=%d\nPoint=0x%02x\nADR=0x01\nControl=0x%02x\nTrackNo=0\nAMin=0\nASec=0\nAFrame=0\nALBA=-150\nZero=0\nPMin=%d\nPSec=%d\nPFrame=%d\nPLBA=%d\n",
		n, session, point, control, (plba+150)/4500, (plba+150)/75%60, (plba+150)%75, plba)
}

func hexPacks(packs []byte) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[CDText]\nEntries=%d\n", len(packs)/packBytes)
	for i := 0; i < len(packs)/packBytes; i++ {
		fmt.Fprintf(&b, "Entry %d=% x\n", i, packs[i*packBytes:(i+1)*packBytes])
	}
	return b.String()
}

func TestOpenCCD(t *testing.T) {
	dir := t.TempDir()
	pcm := tone(0, 800, 440)
	write(t, dir, "x.img", pcm)
	sheet := "[CloneCD]\r\nVersion=3\r\n\r\n[Disc]\r\nTocEntries=6\r\nSessions=1\r\nDataTracksScrambled=0\r\nCDTextLength=0\r\nCatalog=1234567890123\r\n\r\n[Session 1]\r\nPreGapMode=0\r\n\r\n" +
		ccdEntryText(0, 1, 0xa0, 0x00, 4350) + ccdEntryText(1, 1, 0xa1, 0x00, 13350) + ccdEntryText(2, 1, 0xa2, 0x00, 800) +
		ccdEntryText(3, 1, 0x01, 0x02, 0) + ccdEntryText(4, 1, 0x02, 0x01, 300) + ccdEntryText(5, 1, 0x03, 0x08, 600) +
		"[TRACK 1]\nMODE=0\nINDEX 1=0\n[TRACK 2]\nMODE=0\nINDEX 0=225\nINDEX 1=300\nINDEX 2=450\n[TRACK 3]\nMODE=0\nINDEX 1=600\n" +
		hexPacks(append(makePacks(0x80, "CCD Disc", "First", "Second", "Third"), makePacks(0x81, "An Artist", "A", "B", "C")...))
	d, err := Open(write(t, dir, "x.ccd", []byte(sheet)))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.Format != "ccd" || d.Catalog != "1234567890123" || d.Sessions != 1 {
		t.Fatalf("disc = %+v", d)
	}
	checkLayout(t, d, [][3]int{{0, 0, 225}, {225, 300, 600}, {600, 600, 800}})
	one, two, three := d.Tracks[0], d.Tracks[1], d.Tracks[2]
	if !one.CopyAllowed || one.PreEmphasis || one.FourChannel || !two.PreEmphasis || two.CopyAllowed || !three.FourChannel {
		t.Errorf("flags = %+v", d.Tracks)
	}
	if len(two.Indexes) != 1 || two.Indexes[0] != (Index{2, 450}) {
		t.Errorf("indexes = %+v", two.Indexes)
	}
	if d.Text.Title != "CCD Disc" || d.Text.Performer != "An Artist" || one.Text.Title != "First" || three.Text.Performer != "C" {
		t.Errorf("text = %+v, %+v", d.Text, d.Tracks)
	}
	if !bytes.Equal(wholeDisc(t, d), pcm) {
		t.Fatal("the whole-disc stream differs from the image")
	}
}

// Opening the .img finds the sheet beside it.
func TestCCDThroughImg(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "x.img", tone(0, 300, 440))
	write(t, dir, "x.ccd", []byte(ccdEntryText(0, 1, 1, 0, 0)+ccdEntryText(1, 1, 0xa2, 0, 300)))
	d, err := Open(dir + "/x.img")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.Format != "ccd" {
		t.Fatalf("format = %q", d.Format)
	}
}

// A .bin does when there's no .img, and minutes and seconds stand in for PLBA.
func TestCCDBinAndMSF(t *testing.T) {
	dir := t.TempDir()
	pcm := tone(0, 400, 440)
	write(t, dir, "x.bin", pcm)
	d, err := Open(write(t, dir, "x.ccd", []byte(`[Entry 0]
Point=0x01
ADR=1
Control=0
PMin=0
PSec=2
PFrame=0
[Entry 1]
Point=2
Control=0
PMin=0
PSec=4
PFrame=0
[Entry 2]
Point=0xA2
PMin=0
PSec=7
PFrame=25
`)))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	checkLayout(t, d, [][3]int{{0, 0, 150}, {150, 150, 400}})
}

func TestCCDMixedModeWithScrambledData(t *testing.T) {
	dir := t.TempDir()
	pcm := tone(0, 400, 440)
	data := bytes.Repeat([]byte{1, 2, 3, 4}, 200*SectorBytes/4)
	write(t, dir, "x.img", append(bytes.Clone(data), pcm...))
	sheet := "[Disc]\nDataTracksScrambled=1\n" + ccdEntryText(0, 1, 1, 0x04, 0) + ccdEntryText(1, 1, 2, 0x00, 200) + ccdEntryText(2, 1, 0xa2, 0, 600) + "[TRACK 1]\nMODE=2\n"
	d, err := Open(write(t, dir, "x.ccd", []byte(sheet)))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	checkLayout(t, d, [][3]int{{0, 0, 200}, {200, 200, 600}})
	if d.Tracks[0].Audio || d.Tracks[0].Mode != "MODE2/2352" || !d.Tracks[1].Audio {
		t.Fatalf("tracks = %+v", d.Tracks)
	}
	if !bytes.Equal(wholeDisc(t, d), pcm) {
		t.Fatal("audio should be untouched and the data track left out")
	}
}

// A CD-Extra's data session sits after a gap in LBAs, whether or not the dump kept
// that gap in the file. The audio plays either way, and the data track is listed.
func TestCCDExtra(t *testing.T) {
	const audio, data, second = 500, 100, 500 + sessionGap
	pcm := tone(0, audio, 440)
	sheet := "[Disc]\nSessions=2\n" +
		ccdEntryText(0, 1, 1, 0x00, 0) + ccdEntryText(1, 1, 2, 0x00, 300) + ccdEntryText(2, 1, 0xa2, 0, audio) +
		ccdEntryText(3, 2, 3, 0x04, second) + ccdEntryText(4, 2, 0xa2, 0x04, second+data)
	dataBytes := bytes.Repeat([]byte{9, 8, 7, 6}, data*SectorBytes/4)
	for name, image := range map[string][]byte{
		"without the gap": bytes.Join([][]byte{pcm, dataBytes}, nil),
		"with the gap":    bytes.Join([][]byte{pcm, make([]byte, sessionGap*SectorBytes), dataBytes}, nil),
		"data cut off":    pcm,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, "x.img", image)
			d, err := Open(write(t, dir, "x.ccd", []byte(sheet)))
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if d.Sessions != 2 {
				t.Errorf("sessions = %d", d.Sessions)
			}
			checkLayout(t, d, [][3]int{{0, 0, 300}, {300, 300, audio}, {second, second, second + data}})
			if d.Tracks[2].Audio || d.Tracks[2].Session != 2 || !hasWarning(d, "data-track") {
				t.Errorf("track 3 = %+v, warnings = %+v", d.Tracks[2], d.Warnings)
			}
			if !bytes.Equal(wholeDisc(t, d), pcm) {
				t.Error("the audio session differs")
			}
			if toc := d.TOC(); toc.Last != 2 || toc.LeadOut != audio+LeadIn {
				t.Errorf("toc = %+v", toc)
			}
		})
	}
}

// Audio in a second session is found at the right place in both kinds of dump.
func TestCCDSecondSessionAudio(t *testing.T) {
	const first, second, lba2 = 300, 200, 300 + sessionGap
	a, b := tone(0, first, 440), tone(0, second, 880)
	sheet := ccdEntryText(0, 1, 1, 0, 0) + ccdEntryText(1, 1, 0xa2, 0, first) + ccdEntryText(2, 2, 2, 0, lba2) + ccdEntryText(3, 2, 0xa2, 0, lba2+second)
	for name, image := range map[string][]byte{
		"gapless": bytes.Join([][]byte{a, b}, nil),
		"gap":     bytes.Join([][]byte{a, make([]byte, sessionGap*SectorBytes), b}, nil),
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, "x.img", image)
			d, err := Open(write(t, dir, "x.ccd", []byte(sheet)))
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if !bytes.Equal(wholeDisc(t, d), append(bytes.Clone(a), b...)) {
				t.Error("the second session is in the wrong place")
			}
		})
	}
}

func TestCCDDamage(t *testing.T) {
	dir := t.TempDir()
	pcm := tone(0, 300, 440)
	write(t, dir, "x.img", pcm)
	three := ccdEntryText(0, 1, 1, 0, 0) + ccdEntryText(1, 1, 2, 0, 200) + ccdEntryText(2, 1, 3, 0, 500) + ccdEntryText(3, 1, 0xa2, 0, 800)

	// An image shorter than its table of contents.
	d, err := Open(write(t, dir, "x.ccd", []byte(three)))
	if err != nil {
		t.Fatal(err)
	}
	checkLayout(t, d, [][3]int{{0, 0, 200}, {200, 200, 300}})
	if !hasWarning(d, "truncated-tracks") || !hasWarning(d, "truncated") {
		t.Errorf("warnings = %+v", d.Warnings)
	}
	d.Close()

	// Bad entries cost only themselves.
	bad := write(t, dir, "bad.ccd", []byte(`[Entry 0]
Point=fish
PLBA=0
[Entry 1]
Point=0x01
Control=0
PLBA=0
[Entry 2]
Point=0x02
PLBA=99999999
[Entry 3]
Point=0x03
PLBA=-150
[Entry 4]
Point=0x04
PMin=500
PSec=500
PFrame=500
[Entry 5]
Point=0x05
ADR=0x05
PLBA=100
[Entry 6]
Point=0x63
PLBA=100
[Entry 7]
Point=0xff
PLBA=100
[Entry 8]
Point=0x01
PLBA=200
[TRACK 999]
MODE=7
INDEX 1=5
[TRACK 1]
INDEX 2=99999999
INDEX 0=-5
[CDText]
Entries=2
Entry 0=not hex at all
Entry 1=80
`))
	write(t, dir, "bad.img", pcm)
	d, err = Open(bad)
	if err != nil {
		t.Fatal(err)
	}
	checkLayout(t, d, [][3]int{{0, 0, 100}, {100, 100, 300}})
	if d.Tracks[1].Number != 99 || !hasWarning(d, "bad-tracks") || !hasWarning(d, "cdtext-damaged") {
		t.Errorf("disc = %+v", d)
	}
	d.Close()

	// Nothing usable.
	for name, sheet := range map[string]string{
		"empty.ccd": "",
		"words.ccd": "[CloneCD]\nVersion=3\n",
		"noise.ccd": "\x00\xff[[[[=]]]\n=\n[Entry\n",
	} {
		write(t, dir, strings.TrimSuffix(name, ".ccd")+".img", pcm)
		if _, err := Open(write(t, dir, name, []byte(sheet))); !errors.Is(err, ErrNotCD) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, err := Open(write(t, dir, "alone.ccd", []byte(three))); err == nil {
		t.Error("a sheet with no image beside it should be an error")
	}
	write(t, dir, "e.img", nil)
	if _, err := Open(write(t, dir, "e.ccd", []byte(three))); err == nil {
		t.Error("an empty image should be an error")
	}
	write(t, dir, "data.img", pcm)
	if _, err := Open(write(t, dir, "data.ccd", []byte(ccdEntryText(0, 1, 1, 4, 0)))); !errors.Is(err, ErrNoAudio) {
		t.Errorf("err = %v", err)
	}
}
