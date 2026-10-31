package wittycd

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

// nrgBuilder writes a Nero image: sectors, then a chain of chunks, then a footer.
type nrgBuilder struct {
	sectors []byte
	chunks  bytes.Buffer
	v1      bool
}

func (n *nrgBuilder) chunk(id string, b []byte) *nrgBuilder {
	n.chunks.WriteString(id)
	binary.Write(&n.chunks, binary.BigEndian, uint32(len(b)))
	n.chunks.Write(b)
	return n
}

func (n *nrgBuilder) bytes() []byte {
	var out bytes.Buffer
	out.Write(n.sectors)
	out.Write(n.chunks.Bytes())
	out.WriteString("END!\x00\x00\x00\x00")
	if n.v1 {
		out.WriteString("NERO")
		binary.Write(&out, binary.BigEndian, uint32(len(n.sectors)))
	} else {
		out.WriteString("NER5")
		binary.Write(&out, binary.BigEndian, uint64(len(n.sectors)))
	}
	return out.Bytes()
}

type nrgCueEntry struct {
	mode, track, index byte
	lba                int32
}

func cuex(entries ...nrgCueEntry) []byte {
	var b bytes.Buffer
	for _, e := range entries {
		b.Write([]byte{e.mode, e.track, e.index, 0})
		binary.Write(&b, binary.BigEndian, e.lba)
	}
	return b.Bytes()
}

func toBCD(v int) byte { return byte(v/10<<4 | v%10) }

func cues(entries ...nrgCueEntry) []byte {
	var b bytes.Buffer
	for _, e := range entries {
		at := int(e.lba) + LeadIn
		b.Write([]byte{e.mode, e.track, e.index, 0, 0, toBCD(at / 4500), toBCD(at / 75 % 60), toBCD(at % 75)})
	}
	return b.Bytes()
}

type nrgDaoTrack struct {
	isrc      string
	stride    uint16
	mode      byte
	i0, i1, e int64
}

func dao(wide bool, upc string, first, last byte, tracks ...nrgDaoTrack) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.BigEndian, uint32(0))
	b.WriteString((upc + "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")[:13])
	b.Write([]byte{0, 0, 0, first, last})
	for _, t := range tracks {
		b.WriteString((t.isrc + "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")[:12])
		binary.Write(&b, binary.BigEndian, t.stride)
		b.Write([]byte{t.mode, 0, 0, 0})
		if wide {
			binary.Write(&b, binary.BigEndian, []int64{t.i0, t.i1, t.e})
		} else {
			binary.Write(&b, binary.BigEndian, []int32{int32(t.i0), int32(t.i1), int32(t.e)})
		}
	}
	return b.Bytes()
}

// nrgDiscImage is a disc-at-once image of 800 sectors in two tracks, with track 1's
// usual pregap stored in the file and track 2's pregap in track 1's sectors.
func nrgDiscImage(pcm []byte, v1 bool) []byte {
	lead := 150 * SectorBytes
	sectors := append(make([]byte, lead), pcm...)
	n := &nrgBuilder{sectors: sectors, v1: v1}
	entries := []nrgCueEntry{
		{0x01, 0x00, 0, -150},
		{0x21, 0x01, 0, -150},
		{0x21, 0x01, 1, 0},
		{0x11, 0x02, 0, 300},
		{0x11, 0x02, 1, 450},
		{0x11, 0x02, 2, 600},
		{0x01, 0xaa, 1, 800},
	}
	tracks := []nrgDaoTrack{
		{"USTST2600001", 2352, 7, 0, int64(lead), int64(lead + 300*SectorBytes)},
		{"USTST2600002", 2352, 7, int64(lead + 300*SectorBytes), int64(lead + 450*SectorBytes), int64(lead + 800*SectorBytes)},
	}
	if v1 {
		n.chunk("CUES", cues(entries...)).chunk("DAOI", dao(false, "1234567890123", 1, 2, tracks...))
	} else {
		n.chunk("CUEX", cuex(entries...)).chunk("DAOX", dao(true, "1234567890123", 1, 2, tracks...))
	}
	n.chunk("CDTX", append(makePacks(0x80, "Nero Disc", "First", "Second"), makePacks(0x81, "An Artist", "A", "B")...))
	n.chunk("SINF", []byte{0, 0, 0, 2})
	n.chunk("MTYP", []byte{0, 0, 0, 1})
	return n.bytes()
}

func TestOpenNRG(t *testing.T) {
	pcm := tone(0, 800, 440)
	for name, v1 := range map[string]bool{"v1": true, "v2": false} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			d, err := Open(write(t, dir, "x.nrg", nrgDiscImage(pcm, v1)))
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if d.Format != "nrg" || d.Catalog != "1234567890123" || d.Sessions != 1 {
				t.Fatalf("disc = %+v", d)
			}
			checkLayout(t, d, [][3]int{{0, 0, 300}, {300, 450, 800}})
			one, two := d.Tracks[0], d.Tracks[1]
			if !one.CopyAllowed || one.PreEmphasis || !two.PreEmphasis || two.CopyAllowed || one.ISRC != "USTST2600001" || two.ISRC != "USTST2600002" {
				t.Errorf("tracks = %+v", d.Tracks)
			}
			if len(two.Indexes) != 1 || two.Indexes[0] != (Index{2, 600}) {
				t.Errorf("indexes = %+v", two.Indexes)
			}
			if d.Text.Title != "Nero Disc" || one.Text.Title != "First" || two.Text.Performer != "B" {
				t.Errorf("text = %+v, %+v", d.Text, d.Tracks)
			}
			if !bytes.Equal(wholeDisc(t, d), pcm) {
				t.Fatal("the whole-disc stream differs from the image")
			}
		})
	}
}

// Track numbers past 9 are written in BCD by most, and as plain numbers by some.
func TestNRGTrackNumbersOverNine(t *testing.T) {
	pcm := tone(0, 480, 440)
	var bcd, plain []nrgCueEntry
	var tracks []nrgDaoTrack
	for i := range 12 {
		n, lba := i+1, i*40
		bcd = append(bcd, nrgCueEntry{0x01, toBCD(n), 1, int32(lba)})
		plain = append(plain, nrgCueEntry{0x01, byte(n), 1, int32(lba)})
		tracks = append(tracks, nrgDaoTrack{"", 2352, 7, int64(lba * SectorBytes), int64(lba * SectorBytes), int64((lba + 40) * SectorBytes)})
	}
	bcd = append(bcd, nrgCueEntry{0x01, 0xaa, 1, 480})
	plain = append(plain, nrgCueEntry{0x01, 0xaa, 1, 480})
	for name, entries := range map[string][]nrgCueEntry{"bcd": bcd, "plain": plain} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			n := &nrgBuilder{sectors: pcm}
			n.chunk("CUEX", cuex(entries...)).chunk("DAOX", dao(true, "", 1, 12, tracks...))
			d, err := Open(write(t, dir, "x.nrg", n.bytes()))
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if len(d.Tracks) != 12 || d.Tracks[11].Number != 12 || d.Tracks[11].End != 480 || d.Track(10).Start != 360 {
				t.Fatalf("tracks = %+v", d.Tracks)
			}
		})
	}
}

// Track at once: no cue table, so the track table says everything.
func TestNRGTrackAtOnce(t *testing.T) {
	pcm := tone(0, 500, 440)
	first := 200 * SectorBytes
	etnf := func(off, length, mode, lba uint32) []byte {
		var b bytes.Buffer
		binary.Write(&b, binary.BigEndian, []uint32{off, length, mode, lba, 0})
		return b.Bytes()
	}
	tinf := func(off, length, mode uint32) []byte {
		var b bytes.Buffer
		binary.Write(&b, binary.BigEndian, []uint32{off, length, mode})
		return b.Bytes()
	}
	f, r := uint32(first), uint32(len(pcm)-first)
	for name, c := range map[string]struct {
		id     string
		table  []byte
		second int
	}{
		"ETN2 states where tracks are": {"ETN2", append(etn2Entry(0, int64(first), 7, 0), etn2Entry(int64(first), int64(r), 7, 350)...), 350},
		"ETNF states where tracks are": {"ETNF", append(etnf(0, f, 7, 0), etnf(f, r, 7, 350)...), 350},
		"ETNF with no real positions":  {"ETNF", append(etnf(0, f, 7, 0), etnf(f, r, 7, 0)...), 200},
		"TINF never says":              {"TINF", append(tinf(0, f, 7), tinf(f, r, 7)...), 200},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			n := &nrgBuilder{sectors: pcm}
			n.chunk(c.id, c.table)
			d, err := Open(write(t, dir, "x.nrg", n.bytes()))
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			checkLayout(t, d, [][3]int{{0, 0, 200}, {c.second, c.second, c.second + 300}})
			if !bytes.Equal(wholeDisc(t, d), pcm) {
				t.Fatal("the whole-disc stream differs from the image")
			}
		})
	}
}

// A pregap the cue table lists and the file doesn't hold is generated silence.
func TestNRGPregapNotInFile(t *testing.T) {
	pcm := tone(0, 400, 440)
	n := &nrgBuilder{sectors: pcm}
	n.chunk("CUEX", cuex(nrgCueEntry{1, 1, 0, -150}, nrgCueEntry{1, 1, 1, 0}, nrgCueEntry{1, 2, 0, 200}, nrgCueEntry{1, 2, 1, 350}, nrgCueEntry{1, 0xaa, 1, 550}))
	n.chunk("ETN2", bytes.Join([][]byte{etn2Entry(0, 200*SectorBytes, 7, 0), etn2Entry(200*SectorBytes, 200*SectorBytes, 7, 350)}, nil))
	d, err := Open(write(t, t.TempDir(), "x.nrg", n.bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	checkLayout(t, d, [][3]int{{0, 0, 200}, {200, 350, 550}})
	want := bytes.Join([][]byte{pcm[:200*SectorBytes], make([]byte, 150*SectorBytes), pcm[200*SectorBytes:]}, nil)
	if !bytes.Equal(wholeDisc(t, d), want) {
		t.Fatal("the whole-disc stream differs from the image")
	}
}

func etn2Entry(off, length int64, mode, lba uint32) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.BigEndian, off)
	binary.Write(&b, binary.BigEndian, length)
	binary.Write(&b, binary.BigEndian, mode)
	binary.Write(&b, binary.BigEndian, lba)
	binary.Write(&b, binary.BigEndian, uint64(0))
	return b.Bytes()
}

// With no cue table a disc-at-once table's own pregaps set the layout.
func TestNRGDaoAlone(t *testing.T) {
	pcm := tone(0, 400, 440)
	lead := 150 * SectorBytes
	n := &nrgBuilder{sectors: append(make([]byte, lead), pcm...)}
	n.chunk("DAOX", dao(true, "", 1, 2,
		nrgDaoTrack{"", 2352, 7, 0, int64(lead), int64(lead + 200*SectorBytes)},
		nrgDaoTrack{"", 2352, 7, int64(lead + 200*SectorBytes), int64(lead + 250*SectorBytes), int64(lead + 400*SectorBytes)}))
	d, err := Open(write(t, t.TempDir(), "x.nrg", n.bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	checkLayout(t, d, [][3]int{{0, 0, 200}, {200, 250, 400}})
	if !bytes.Equal(wholeDisc(t, d), pcm) {
		t.Fatal("the whole-disc stream differs from the image")
	}
}

// A CD-Extra: audio with subcode in 2448-byte sectors, then a data session after a
// gap in small sectors, each with its own tables.
func TestNRGExtraWithOtherSectorSizes(t *testing.T) {
	pcm := tone(0, 400, 440)
	var sectors bytes.Buffer
	sub := make([]byte, 96)
	sub[0] = graphicsCommand
	for i := 0; i < 400; i++ {
		sectors.Write(pcm[i*SectorBytes : (i+1)*SectorBytes])
		sectors.Write(sub)
	}
	audioEnd := sectors.Len()
	sectors.Write(bytes.Repeat([]byte{0x77}, 100*2048))
	n := &nrgBuilder{sectors: sectors.Bytes()}
	second := int32(400 + sessionGap)
	n.chunk("CUEX", cuex(nrgCueEntry{0x01, 0x01, 1, 0}, nrgCueEntry{0x01, 0xaa, 1, 400}))
	n.chunk("DAOX", dao(true, "", 1, 1, nrgDaoTrack{"", 2448, 0x10, 0, 0, int64(audioEnd)}))
	n.chunk("CUEX", cuex(nrgCueEntry{0x41, 0x02, 1, second}, nrgCueEntry{0x41, 0xaa, 1, second + 100}))
	n.chunk("DAOX", dao(true, "", 2, 2, nrgDaoTrack{"", 2048, 0, int64(audioEnd), int64(audioEnd), int64(audioEnd + 100*2048)}))
	n.chunk("SINF", []byte{0, 0, 0, 1}).chunk("SINF", []byte{0, 0, 0, 1})
	d, err := Open(write(t, t.TempDir(), "x.nrg", n.bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	checkLayout(t, d, [][3]int{{0, 0, 400}, {int(second), int(second), int(second) + 100}})
	if d.Sessions != 2 || d.Tracks[1].Audio || d.Tracks[1].Session != 2 || d.Tracks[1].Mode != "MODE1/2048" || d.Tracks[0].Mode != "CDG" {
		t.Fatalf("disc = %+v", d)
	}
	if !d.HasGraphics {
		t.Error("the subcode's graphics weren't found")
	}
	if !bytes.Equal(wholeDisc(t, d), pcm) {
		t.Fatal("audio with subcode should read as plain audio, and the data track not at all")
	}
	if toc := d.TOC(); toc.Last != 1 || toc.LeadOut != 400+LeadIn {
		t.Errorf("toc = %+v", toc)
	}
}

func TestNRGDamage(t *testing.T) {
	dir := t.TempDir()
	pcm := tone(0, 300, 440)
	good := nrgDiscImage(tone(0, 800, 440), false)

	// Cut anywhere, the footer is gone and that is said plainly.
	for _, size := range []int{0, 3, 8, 11, 12, 1000, len(good) / 2, len(good) - 1} {
		_, err := Open(write(t, dir, "cut.nrg", good[:size]))
		if !errors.Is(err, ErrNotCD) {
			t.Errorf("cut to %d bytes: err = %v", size, err)
		}
	}

	footer := func(at uint64) []byte {
		b := append(make([]byte, 100), "NER5"...)
		return binary.BigEndian.AppendUint64(b, at)
	}
	for name, img := range map[string][]byte{
		"footer past the end":     footer(1 << 62),
		"footer at the very top":  footer(0xffffffffffffffff),
		"footer into itself":      footer(104),
		"v1 footer past the end":  append(append(make([]byte, 100), "NERO"...), 0xff, 0xff, 0xff, 0xff),
		"chain is only noise":     footer(0),
		"chain with no tables":    (&nrgBuilder{sectors: pcm}).chunk("MTYP", []byte{1, 2, 3, 4}).bytes(),
		"table cut short":         (&nrgBuilder{sectors: pcm}).chunk("DAOX", []byte{1, 2, 3}).bytes(),
		"table with no tracks":    (&nrgBuilder{sectors: pcm}).chunk("DAOX", make([]byte, nrgDaoHeader)).bytes(),
		"cue table but no tracks": (&nrgBuilder{sectors: pcm}).chunk("CUEX", cuex(nrgCueEntry{1, 1, 1, 0})).bytes(),
		"empty cue table":         (&nrgBuilder{sectors: pcm}).chunk("CUEX", nil).bytes(),
	} {
		if _, err := Open(write(t, dir, "bad.nrg", img)); !errors.Is(err, ErrNotCD) {
			t.Errorf("%s: err = %v", name, err)
		}
	}

	// Chunks larger than allowed are refused unread, whatever they say they are.
	huge := &nrgBuilder{sectors: pcm}
	huge.chunks.WriteString("CDTX")
	binary.Write(&huge.chunks, binary.BigEndian, uint32(maxSheet+1))
	if _, err := Open(write(t, dir, "huge.nrg", huge.bytes())); !errors.Is(err, ErrNotCD) {
		t.Errorf("huge chunk: err = %v", err)
	}
	long := &nrgBuilder{sectors: pcm}
	long.chunks.WriteString("CUEX")
	binary.Write(&long.chunks, binary.BigEndian, uint32(0xffffffff))
	if _, err := Open(write(t, dir, "long.nrg", long.bytes())); !errors.Is(err, ErrNotCD) {
		t.Errorf("chunk past the end: err = %v", err)
	}

	// A chain that ends badly keeps what came before it.
	okThenHuge := &nrgBuilder{sectors: pcm}
	okThenHuge.chunk("DAOX", dao(true, "", 1, 1, nrgDaoTrack{"", 2352, 7, 0, 0, int64(len(pcm))}))
	okThenHuge.chunks.WriteString("CDTX")
	binary.Write(&okThenHuge.chunks, binary.BigEndian, uint32(0xfffffff0))
	d, err := Open(write(t, dir, "tail.nrg", okThenHuge.bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Tracks) != 1 || !hasWarning(d, "nrg-chain") {
		t.Errorf("disc = %+v", d)
	}
	d.Close()

	// Offsets outside the file lose their tracks and nothing else.
	lim := int64(len(pcm))
	mixed := &nrgBuilder{sectors: pcm}
	mixed.chunk("DAOX", dao(true, "", 1, 5,
		nrgDaoTrack{"", 2352, 7, 0, 0, 100 * SectorBytes},
		nrgDaoTrack{"", 2352, 7, 1 << 60, 1 << 60, 1<<60 + 1000},
		nrgDaoTrack{"", 1234, 7, 100 * SectorBytes, 100 * SectorBytes, 200 * SectorBytes},
		nrgDaoTrack{"", 2352, 7, 200 * SectorBytes, 200 * SectorBytes, 100 * SectorBytes},
		nrgDaoTrack{"", 2352, 7, -1, -1, -1},
	))
	d, err = Open(write(t, dir, "mixed.nrg", mixed.bytes()))
	if err != nil {
		t.Fatal(err)
	}
	checkLayout(t, d, [][3]int{{0, 0, 100}})
	if !hasWarning(d, "bad-tracks") {
		t.Errorf("warnings = %+v", d.Warnings)
	}
	d.Close()

	// An end past the sectors is cut to them, and said to be.
	over := &nrgBuilder{sectors: pcm}
	over.chunk("DAOX", dao(true, "", 1, 1, nrgDaoTrack{"", 2352, 7, 0, 0, lim * 1000}))
	d, err = Open(write(t, dir, "over.nrg", over.bytes()))
	if err != nil {
		t.Fatal(err)
	}
	checkLayout(t, d, [][3]int{{0, 0, 300}})
	if !hasWarning(d, "truncated") {
		t.Errorf("warnings = %+v", d.Warnings)
	}
	d.Close()

	// A cue table promising more than the file has.
	short := &nrgBuilder{sectors: pcm}
	short.chunk("CUEX", cuex(nrgCueEntry{1, 1, 1, 0}, nrgCueEntry{1, 2, 1, 200}, nrgCueEntry{1, 3, 1, 900}, nrgCueEntry{1, 0xaa, 1, 999}))
	short.chunk("DAOX", dao(true, "", 1, 3,
		nrgDaoTrack{"", 2352, 7, 0, 0, 200 * SectorBytes},
		nrgDaoTrack{"", 2352, 7, 200 * SectorBytes, 200 * SectorBytes, 300 * SectorBytes}))
	d, err = Open(write(t, dir, "short.nrg", short.bytes()))
	if err != nil {
		t.Fatal(err)
	}
	checkLayout(t, d, [][3]int{{0, 0, 200}, {200, 200, 300}})
	if !hasWarning(d, "bad-tracks") || !hasWarning(d, "truncated") {
		t.Errorf("warnings = %+v", d.Warnings)
	}
	d.Close()

	// Nonsense in a cue table is passed over.
	junk := &nrgBuilder{sectors: pcm}
	junk.chunk("CUEX", cuex(nrgCueEntry{1, 1, 1, -1000000}, nrgCueEntry{1, 1, 1, 0}, nrgCueEntry{1, 0x9f, 1, 0}, nrgCueEntry{1, 2, 200, 0}, nrgCueEntry{1, 0xaa, 1, 1 << 30}, nrgCueEntry{1, 0xaa, 1, 300}))
	junk.chunk("DAOX", dao(true, "", 1, 1, nrgDaoTrack{"", 2352, 7, 0, 0, lim}))
	d, err = Open(write(t, dir, "junk.nrg", junk.bytes()))
	if err != nil {
		t.Fatal(err)
	}
	checkLayout(t, d, [][3]int{{0, 0, 300}})
	d.Close()

	// A session table that doesn't add up isn't believed.
	silly := &nrgBuilder{sectors: pcm}
	silly.chunk("DAOX", dao(true, "", 1, 1, nrgDaoTrack{"", 2352, 7, 0, 0, lim})).chunk("SINF", []byte{0xff, 0xff, 0xff, 0xff, 0, 0, 0, 9})
	d, err = Open(write(t, dir, "silly.nrg", silly.bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if d.Sessions != 1 {
		t.Errorf("sessions = %d", d.Sessions)
	}
	d.Close()

	// Only data.
	data := &nrgBuilder{sectors: pcm}
	data.chunk("DAOX", dao(true, "", 1, 1, nrgDaoTrack{"", 2048, 0, 0, 0, lim}))
	if _, err := Open(write(t, dir, "data.nrg", data.bytes())); !errors.Is(err, ErrNoAudio) {
		t.Errorf("err = %v", err)
	}
	// A cue table saying audio over sectors too small for it can't be played.
	small := &nrgBuilder{sectors: pcm}
	small.chunk("CUEX", cuex(nrgCueEntry{1, 1, 1, 0})).chunk("DAOX", dao(true, "", 1, 1, nrgDaoTrack{"", 2048, 7, 0, 0, lim}))
	if _, err := Open(write(t, dir, "small.nrg", small.bytes())); !errors.Is(err, ErrNoAudio) {
		t.Errorf("err = %v", err)
	}
}
