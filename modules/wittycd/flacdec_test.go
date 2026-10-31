package wittycd

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// intTone is a tone built from whole numbers only, with a little noise so that the
// compressors have to work. Kept test images are compared with it, and unlike a
// sine wave it comes out exactly the same on every kind of processor.
func intTone(start int64, sectors int) []byte {
	tri := func(n, period, amp int64) int64 {
		p := n % period
		if p > period/2 {
			p = period - p
		}
		return (p*4*amp)/period - amp
	}
	out := make([]byte, sectors*SectorBytes)
	for i := range sectors * SectorSamples {
		n := start + int64(i)
		noise := int64(uint32(n)*2654435761>>13) & 7
		l := tri(n, 100, 9000) + tri(n, 37, 2000) + noise - 4
		r := tri(n, 81, 8000) + tri(n, 29, 1500) + (noise^5)/2 - 2
		binary.LittleEndian.PutUint16(out[i*4:], uint16(int16(l)))
		binary.LittleEndian.PutUint16(out[i*4+2:], uint16(int16(r)))
	}
	return out
}

// flacOf encodes samples with this module's own encoder.
func flacOf(t testing.TB, pcm []byte, start int64) []byte {
	t.Helper()
	d := &Disc{Tracks: []Track{{Number: 1, Audio: true, End: len(pcm) / SectorBytes}}}
	d.addExtent(extent{sectors: len(pcm) / SectorBytes, src: bytes.NewReader(pcm)})
	s, err := d.NewStream(nil, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := s.WriteFLAC(&out, start); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func openFlacBytes(t testing.TB, b []byte) (io.ReaderAt, int64) {
	t.Helper()
	r, n, err := newFlacSource(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	return r, n
}

func readAll(t testing.TB, r io.ReaderAt, n int64) []byte {
	t.Helper()
	got := make([]byte, n)
	if m, err := r.ReadAt(got, 0); err != nil && err != io.EOF || int64(m) != n {
		t.Fatalf("read %d of %d: %v", m, n, err)
	}
	return got
}

func TestFlacRoundTrip(t *testing.T) {
	pcm := tone(0, 900, 440)
	r, n := openFlacBytes(t, flacOf(t, pcm, 0))
	if n != int64(len(pcm)) {
		t.Fatalf("length %d, want %d", n, len(pcm))
	}
	if !bytes.Equal(readAll(t, r, n), pcm) {
		t.Fatal("the decoded samples differ from what was encoded")
	}
	// Every kind of range, in an order that keeps it seeking.
	rnd := rand.New(rand.NewPCG(3, 4))
	for range 500 {
		off := rnd.Int64N(n)
		size := min(rnd.Int64N(30000)+1, n-off)
		b := make([]byte, size)
		if _, err := r.ReadAt(b, off); err != nil && err != io.EOF {
			t.Fatal(err)
		}
		if !bytes.Equal(b, pcm[off:off+size]) {
			t.Fatalf("range %d+%d differs", off, size)
		}
	}
	// A short read at the end says so, and past it there is nothing.
	b := make([]byte, 10)
	if m, err := r.ReadAt(b, n-4); m != 4 || err != io.EOF {
		t.Fatalf("at the end: %d %v", m, err)
	}
	if m, err := r.ReadAt(b, n); m != 0 || err != io.EOF {
		t.Fatalf("past the end: %d %v", m, err)
	}
}

func TestFlacProgramStartingPartWay(t *testing.T) {
	pcm := tone(0, 300, 440)
	const start = 12345
	r, n := openFlacBytes(t, flacOf(t, pcm, start))
	want := pcm[start*SampleBytes:]
	if n != int64(len(want)) {
		t.Fatalf("length %d, want %d", n, len(want))
	}
	if !bytes.Equal(readAll(t, r, n), want) {
		t.Fatal("a stream started part way differs")
	}
}

func TestFlacLengthsAroundABlock(t *testing.T) {
	// One sample, one block, a block and a bit: the last frame is the short one, and
	// a frame this short has too few samples to predict.
	whole := tone(0, 40, 440)
	for _, samples := range []int{1, 2, 4, 5, 16, flacBlock - 1, flacBlock, flacBlock + 1, flacBlock*2 + 3} {
		pcm := whole[:samples*SampleBytes]
		d := &Disc{}
		d.addExtent(extent{sectors: samples/SectorSamples + 1, src: bytes.NewReader(whole)})
		s := &Stream{d: d, total: int64(samples), ranges: []streamRange{{sectors: samples/SectorSamples + 1}}}
		var out bytes.Buffer
		if err := s.WriteFLAC(&out, 0); err != nil {
			t.Fatal(err)
		}
		r, n := openFlacBytes(t, out.Bytes())
		if n != int64(len(pcm)) || !bytes.Equal(readAll(t, r, n), pcm) {
			t.Errorf("%d samples came back wrong (%d bytes)", samples, n)
		}
	}
}

func TestFlacCueEndToEnd(t *testing.T) {
	dir := t.TempDir()
	pcm := tone(0, 1500, 440)
	write(t, dir, "Album.flac", flacOf(t, pcm, 0))
	cue := write(t, dir, "Album.cue", []byte(`TITLE "Test Pressing"
FILE "Album.flac" WAVE
  TRACK 01 AUDIO
    INDEX 01 00:00:00
  TRACK 02 AUDIO
    INDEX 00 00:06:00
    INDEX 01 00:08:00
  TRACK 03 AUDIO
    INDEX 01 00:15:00
`))
	d, err := Open(cue)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	want := []struct{ pregap, start, end int }{{0, 0, 450}, {450, 600, 1125}, {1125, 1125, 1500}}
	for i, w := range want {
		tr := d.Tracks[i]
		if tr.Pregap != w.pregap || tr.Start != w.start || tr.End != w.end {
			t.Errorf("track %d = %+v, want %+v", i+1, tr, w)
		}
	}
	if len(d.Warnings) != 0 {
		t.Errorf("warnings: %+v", d.Warnings)
	}
	s, err := d.NewStream(nil, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readAll(t, s, s.Size()), pcm) {
		t.Fatal("the stream differs from the samples that went into the FLAC file")
	}
	// A track chosen on its own starts on the right sample.
	s2, _ := d.NewStream([]int{3}, 1, false)
	if !bytes.Equal(readAll(t, s2, s2.Size()), pcm[1125*SectorBytes:]) {
		t.Fatal("track 3 on its own differs")
	}
}

// One file for each track works the same, and joins without a seam.
func TestFlacCueOneFileEachTrack(t *testing.T) {
	dir := t.TempDir()
	pcm := tone(0, 600, 440)
	write(t, dir, "1.flac", flacOf(t, pcm[:250*SectorBytes], 0))
	write(t, dir, "2.flac", flacOf(t, pcm[250*SectorBytes:], 0))
	cue := write(t, dir, "x.cue", []byte("FILE \"1.flac\" WAVE\nTRACK 01 AUDIO\nINDEX 01 00:00:00\nFILE \"2.flac\" WAVE\nTRACK 02 AUDIO\nINDEX 01 00:00:00\n"))
	d, s := openStream(t, cue)
	if d.Tracks[1].Start != 250 || d.Tracks[1].End != 600 {
		t.Errorf("track 2 = %+v", d.Tracks[1])
	}
	if !bytes.Equal(readAll(t, s, s.Size()), pcm) {
		t.Fatal("the two files don't join up")
	}
}

func TestFlacRefusesWhatIsNotCDAudio(t *testing.T) {
	b := flacOf(t, tone(0, 20, 440), 0)
	for name, change := range map[string]func(uint64) uint64{
		"48 kHz":  func(v uint64) uint64 { return v&^(uint64(1<<20-1)<<44) | 48000<<44 },
		"mono":    func(v uint64) uint64 { return v &^ (7 << 41) },
		"24 bits": func(v uint64) uint64 { return v&^(31<<36) | 23<<36 },
	} {
		bad := bytes.Clone(b)
		binary.BigEndian.PutUint64(bad[18:], change(binary.BigEndian.Uint64(bad[18:])))
		if _, _, err := newFlacSource(bytes.NewReader(bad), int64(len(bad))); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	for _, junk := range [][]byte{nil, []byte("fLaC"), []byte("not a flac file at all"), b[:50], b[:42]} {
		if _, _, err := newFlacSource(bytes.NewReader(junk), int64(len(junk))); err == nil {
			t.Errorf("%d bytes of junk opened", len(junk))
		}
	}
}

// frameStarts finds where each of our encoder's frames begins.
func frameStarts(t *testing.T, b []byte) []int {
	t.Helper()
	var sc flacScratch
	var out []int
	for at := 42; at < len(b); {
		_, n, err := decodeFlacFrame(b[at:], &sc)
		if err != nil {
			t.Fatalf("frame at %d: %v", at, err)
		}
		out = append(out, at)
		at += n
	}
	return out
}

func TestFlacDamagedFrameIsSilence(t *testing.T) {
	pcm := tone(0, 400, 440)
	clean := flacOf(t, pcm, 0)
	starts := frameStarts(t, clean)
	check := func(name string, bad []byte, lost ...int) {
		t.Helper()
		r, n := openFlacBytes(t, bad)
		if n != int64(len(pcm)) {
			t.Fatalf("%s: length %d, want %d", name, n, len(pcm))
		}
		got := readAll(t, r, n)
		want := bytes.Clone(pcm)
		for _, f := range lost {
			clear(want[f*flacBlock*SampleBytes : (f+1)*flacBlock*SampleBytes])
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s: anything but the damaged frames should be exact, and they should be silent", name)
		}
		// Reading from just after the damage, with nothing to go on, is the same.
		r, _ = openFlacBytes(t, bad)
		at := int64(lost[len(lost)-1]+1) * flacBlock * SampleBytes
		part := make([]byte, 5000)
		r.ReadAt(part, at)
		if !bytes.Equal(part, want[at:at+5000]) {
			t.Errorf("%s: seeking past the damage differs", name)
		}
	}
	body := bytes.Clone(clean)
	body[starts[5]+500] ^= 0x10
	check("a flipped bit in a frame's body", body, 5)
	head := bytes.Clone(clean)
	head[starts[9]+1] ^= 0x01 // the blocking strategy, which the header's checksum protects
	check("a damaged frame header", head, 9)
	two := bytes.Clone(clean)
	two[starts[3]+800] ^= 0xff
	two[starts[4]+800] ^= 0xff
	check("two damaged frames together", two, 3, 4)
	wiped := bytes.Clone(clean)
	clear(wiped[starts[7] : starts[8]+40])
	check("a wiped frame and the start of the next", wiped, 7, 8)
}

func TestFlacCutShort(t *testing.T) {
	pcm := tone(0, 400, 440)
	clean := flacOf(t, pcm, 0)
	starts := frameStarts(t, clean)
	cut := clean[:starts[20]+10]
	r, n := openFlacBytes(t, cut)
	if n != 20*flacBlock*SampleBytes && n != 21*flacBlock*SampleBytes {
		t.Fatalf("length %d: a file cut short should be as long as what is left", n)
	}
	got := readAll(t, r, n)
	if !bytes.Equal(got[:20*flacBlock*SampleBytes], pcm[:20*flacBlock*SampleBytes]) {
		t.Fatal("what is left differs")
	}
}

// With no length in STREAMINFO, as a stream written to a pipe has, it is found.
func TestFlacUnknownLength(t *testing.T) {
	pcm := tone(0, 400, 440)
	b := flacOf(t, pcm, 0)
	v := binary.BigEndian.Uint64(b[18:])
	binary.BigEndian.PutUint64(b[18:], v&^uint64(1<<36-1))
	r, n := openFlacBytes(t, b)
	if n != int64(len(pcm)) || !bytes.Equal(readAll(t, r, n), pcm) {
		t.Fatalf("length %d, want %d, or the samples differ", n, len(pcm))
	}
}

// withSeekTable adds a seek table to one of our streams, which has none.
func withSeekTable(b []byte, points [][2]uint64) []byte {
	var out bytes.Buffer
	out.Write(b[:4])
	h := bytes.Clone(b[4:42])
	h[0] = 0 // no longer the last block
	out.Write(h)
	n := len(points) * 18
	out.Write([]byte{0x80 | 3, byte(n >> 16), byte(n >> 8), byte(n)})
	for _, p := range points {
		var e [18]byte
		binary.BigEndian.PutUint64(e[0:], p[0])
		binary.BigEndian.PutUint64(e[8:], p[1])
		out.Write(e[:])
	}
	out.Write(b[42:])
	return out.Bytes()
}

func TestFlacSeekTable(t *testing.T) {
	pcm := tone(0, 2000, 440)
	b := flacOf(t, pcm, 0)
	starts := frameStarts(t, b)
	good := [][2]uint64{{0, 0}}
	for i := 16; i < len(starts); i += 16 {
		good = append(good, [2]uint64{uint64(i) * flacBlock, uint64(starts[i] - 42)})
	}
	lies := [][2]uint64{
		{0, 0},
		{flacBlock * 16, uint64(starts[40] - 42)}, // a real frame, not that one
		{flacBlock * 50, 12345},                   // no frame at all
		{flacBlock * 60, 1 << 62},                 // not in the file
		{1 << 62, 3},                              // not on the disc
		{^uint64(0), 0},                           // a placeholder
		{flacBlock * 70, uint64(starts[70] - 42)}, // true
	}
	for name, table := range map[string][][2]uint64{"true": good, "lying": lies} {
		r, n := openFlacBytes(t, withSeekTable(b, table))
		rnd := rand.New(rand.NewPCG(5, 6))
		for range 200 {
			off := rnd.Int64N(n)
			size := min(rnd.Int64N(20000)+1, n-off)
			got := make([]byte, size)
			if _, err := r.ReadAt(got, off); err != nil && err != io.EOF {
				t.Fatal(err)
			}
			if !bytes.Equal(got, pcm[off:off+size]) {
				t.Fatalf("%s seek table: range %d+%d differs", name, off, size)
			}
		}
	}
}

// Seeking has to stay cheap: a few frames' work however far it jumps.
func TestFlacSeekReadsLittle(t *testing.T) {
	pcm := tone(0, 20000, 440) // over four minutes
	b := flacOf(t, pcm, 0)
	c := &countingReader{r: bytes.NewReader(b)}
	src, n, err := newFlacSource(c, int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	opened := c.bytes
	if opened > 1<<20 {
		t.Errorf("opening read %d bytes", opened)
	}
	rnd := rand.New(rand.NewPCG(7, 8))
	const seeks = 200
	got := make([]byte, 4096)
	for range seeks {
		off := rnd.Int64N(n - 4096)
		if _, err := src.ReadAt(got, off); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, pcm[off:off+4096]) {
			t.Fatalf("seek to %d differs", off)
		}
	}
	if per := (c.bytes - opened) / seeks; per > 1<<20 {
		t.Errorf("each seek read %d bytes on average", per)
	}
	if s := src.(*flacSource); len(s.points) > flacMaxPoints {
		t.Errorf("the index grew to %d entries", len(s.points))
	}
}

type countingReader struct {
	r     io.ReaderAt
	mu    sync.Mutex
	bytes int64
}

func (c *countingReader) ReadAt(p []byte, off int64) (int, error) {
	n, err := c.r.ReadAt(p, off)
	c.mu.Lock()
	c.bytes += int64(n)
	c.mu.Unlock()
	return n, err
}

func TestFlacManyListeners(t *testing.T) {
	pcm := tone(0, 3000, 440)
	r, n := openFlacBytes(t, flacOf(t, pcm, 0))
	var wg sync.WaitGroup
	failed := make(chan string, 8)
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rnd := rand.New(rand.NewPCG(uint64(g), 9))
			off := rnd.Int64N(n / 2)
			got := make([]byte, 7000)
			for range 300 {
				if off+7000 > n || rnd.IntN(40) == 0 {
					off = rnd.Int64N(n - 7000)
				}
				if _, err := r.ReadAt(got, off); err != nil {
					failed <- err.Error()
					return
				}
				if !bytes.Equal(got, pcm[off:off+7000]) {
					failed <- "a listener read the wrong samples"
					return
				}
				off += 7000
			}
		}()
	}
	wg.Wait()
	close(failed)
	for f := range failed {
		t.Fatal(f)
	}
}

// frameBuilder writes frames the module's own encoder never makes, one kind of
// subframe at a time.
type frameBuilder struct {
	w bitWriter
}

func (f *frameBuilder) start(strategy, sizeCode, assign uint64, number uint64, blocks int) {
	f.w = bitWriter{}
	f.w.put(0xfff8|strategy, 16)
	f.w.put(sizeCode<<4|9, 8)
	f.w.put(assign<<4|4<<1, 8)
	putUTF8(&f.w, number)
	switch sizeCode {
	case 6:
		f.w.put(uint64(blocks-1), 8)
	case 7:
		f.w.put(uint64(blocks-1), 16)
	}
	f.w.put(uint64(crc8(f.w.b)), 8)
}

func (f *frameBuilder) finish() []byte {
	f.w.align()
	crc := crc16(f.w.b)
	return append(f.w.b, byte(crc>>8), byte(crc))
}

func (f *frameBuilder) rice(values []int32, k uint) {
	for _, v := range values {
		u := uint64(uint32(v<<1 ^ v>>31))
		for q := u >> k; ; q-- {
			if q == 0 {
				f.w.put(1, 1)
				break
			}
			f.w.put(0, 1)
		}
		if k > 0 {
			f.w.put(u, k)
		}
	}
}

// subframe makes every kind of subframe from the same samples.
func (f *frameBuilder) subframe(kind string, x []int32, bps uint) {
	w := &f.w
	switch kind {
	case "constant":
		w.put(0, 8)
		w.put(uint64(uint32(x[0])), bps)
	case "verbatim":
		w.put(1<<1, 8)
		for _, v := range x {
			w.put(uint64(uint32(v)), bps)
		}
	case "fixed0", "fixed1", "fixed2", "fixed3", "fixed4":
		order := int(kind[5] - '0')
		w.put(uint64(8|order)<<1, 8)
		for _, v := range x[:order] {
			w.put(uint64(uint32(v)), bps)
		}
		r := fixedResidual(x, order)
		w.put(0, 2) // Rice, 4-bit parameters
		w.put(0, 4) // one partition
		w.put(8, 4)
		f.rice(r, 8)
	case "rice2":
		w.put(uint64(8|1)<<1, 8)
		w.put(uint64(uint32(x[0])), bps)
		w.put(1, 2) // Rice, 5-bit parameters
		w.put(0, 4)
		w.put(17, 5)
		f.rice(fixedResidual(x, 1), 17)
	case "escape":
		w.put(uint64(8|1)<<1, 8)
		w.put(uint64(uint32(x[0])), bps)
		w.put(0, 2)
		w.put(0, 4)
		w.put(15, 4) // no Rice code: plain numbers of a stated width
		w.put(19, 5)
		for _, v := range fixedResidual(x, 1) {
			w.put(uint64(uint32(v)), 19)
		}
	case "escape2":
		w.put(uint64(8|1)<<1, 8)
		w.put(uint64(uint32(x[0])), bps)
		w.put(1, 2)
		w.put(0, 4)
		w.put(31, 5)
		w.put(19, 5)
		for _, v := range fixedResidual(x, 1) {
			w.put(uint64(uint32(v)), 19)
		}
	case "partitions":
		w.put(uint64(8|2)<<1, 8)
		for _, v := range x[:2] {
			w.put(uint64(uint32(v)), bps)
		}
		r := fixedResidual(x, 2)
		w.put(0, 2)
		w.put(3, 4) // eight partitions, the first shorter by the order
		size := len(x) >> 3
		at := 0
		for p := range 8 {
			end := (p+1)*size - 2
			w.put(uint64(4+p%5), 4)
			f.rice(r[at:end], uint(4+p%5))
			at = end
		}
	case "lpc":
		// A real filter: x[i] is predicted as 1.5 x[i-1] - 0.5 x[i-2], in 1/2 steps.
		const order = 2
		w.put(uint64(31+order)<<1, 8)
		for _, v := range x[:order] {
			w.put(uint64(uint32(v)), bps)
		}
		w.put(5-1, 4) // 5-bit coefficients
		w.put(1, 5)   // shift
		w.put(3, 5)
		w.put(31, 5) // -1 in 5 bits
		r := make([]int32, len(x)-order)
		for i := order; i < len(x); i++ {
			r[i-order] = x[i] - (3*x[i-1]-x[i-2])>>1
		}
		w.put(0, 2)
		w.put(0, 4)
		w.put(8, 4)
		f.rice(r, 8)
	case "lpc32":
		// The longest filter there can be, with a zero for most of it.
		const order = 32
		w.put(uint64(31+order)<<1, 8)
		for _, v := range x[:order] {
			w.put(uint64(uint32(v)), bps)
		}
		w.put(15-1, 4)
		w.put(14, 5)
		w.put(1<<14-1, 15)
		for range order - 1 {
			w.put(0, 15)
		}
		r := make([]int32, len(x)-order)
		for i := order; i < len(x); i++ {
			r[i-order] = x[i] - int32((int64(1<<14-1)*int64(x[i-1]))>>14)
		}
		w.put(0, 2)
		w.put(0, 4)
		w.put(9, 4)
		f.rice(r, 9)
	case "wasted":
		// All samples even: one bit taken off, stored flag-and-unary.
		w.put(1<<1|1, 8)
		w.put(0, 2) // two more zero bits, three wasted in all
		w.put(1, 1)
		for _, v := range x {
			w.put(uint64(uint32(v>>3)), bps-3)
		}
	default:
		panic(kind)
	}
}

func fixedResidual(x []int32, order int) []int32 {
	r := make([]int32, len(x)-order)
	for i := order; i < len(x); i++ {
		switch order {
		case 0:
			r[i] = x[i]
		case 1:
			r[i-1] = x[i] - x[i-1]
		case 2:
			r[i-2] = x[i] - 2*x[i-1] + x[i-2]
		case 3:
			r[i-3] = x[i] - 3*x[i-1] + 3*x[i-2] - x[i-3]
		case 4:
			r[i-4] = x[i] - 4*x[i-1] + 6*x[i-2] - 4*x[i-3] + x[i-4]
		}
	}
	return r
}

func TestFlacEveryKindOfSubframe(t *testing.T) {
	const blocks = 1024
	l, r := make([]int32, blocks), make([]int32, blocks)
	rnd := rand.New(rand.NewPCG(11, 12))
	var a, b int32
	for i := range l {
		a += int32(rnd.IntN(401)) - 200
		b += int32(rnd.IntN(201)) - 100
		a, b = max(-30000, min(a, 30000)), max(-30000, min(b, 30000))
		l[i], r[i] = a, b
	}
	check := func(name string, frame []byte, wantL, wantR []int32) {
		t.Helper()
		var sc flacScratch
		h, n, err := decodeFlacFrame(frame, &sc)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			return
		}
		if n != len(frame) {
			t.Errorf("%s: took %d of %d bytes", name, n, len(frame))
		}
		if h.blocks != blocks {
			t.Errorf("%s: %d samples", name, h.blocks)
		}
		for i := range wantL {
			if sc.l[i] != wantL[i] || sc.r[i] != wantR[i] {
				t.Errorf("%s: sample %d is %d,%d and should be %d,%d", name, i, sc.l[i], sc.r[i], wantL[i], wantR[i])
				return
			}
		}
	}
	var f frameBuilder
	for _, kind := range []string{"verbatim", "fixed0", "fixed1", "fixed2", "fixed3", "fixed4", "rice2", "escape", "escape2", "partitions", "lpc", "lpc32"} {
		f.start(0, 10, 1, 0, blocks) // 1024 by its code, left and right apart
		f.subframe(kind, l, 16)
		f.subframe(kind, r, 16)
		check(kind, f.finish(), l, r)
	}
	// Each way of storing the two channels.
	side := make([]int32, blocks)
	mid := make([]int32, blocks)
	for i := range side {
		side[i] = l[i] - r[i]
		mid[i] = (l[i] + r[i]) >> 1
	}
	f.start(0, 7, 8, 0, blocks) // and the size written out, in 16 bits
	f.subframe("fixed2", l, 16)
	f.subframe("fixed2", side, 17)
	check("left and side", f.finish(), l, r)
	f.start(0, 10, 9, 0, blocks)
	f.subframe("fixed2", side, 17)
	f.subframe("fixed2", r, 16)
	check("side and right", f.finish(), l, r)
	f.start(0, 10, 10, 0, blocks)
	f.subframe("fixed2", mid, 16)
	f.subframe("fixed2", side, 17)
	check("mid and side", f.finish(), l, r)
	f.start(0, 10, 10, 0, blocks)
	f.subframe("lpc", mid, 16)
	f.subframe("verbatim", side, 17)
	check("mid and side with other kinds", f.finish(), l, r)
	// Constant and wasted bits.
	same := make([]int32, blocks)
	even := make([]int32, blocks)
	for i := range same {
		same[i] = -1234
		even[i] = l[i] &^ 7
	}
	f.start(0, 10, 1, 0, blocks)
	f.subframe("constant", same, 16)
	f.subframe("wasted", even, 16)
	check("constant and wasted bits", f.finish(), same, even)
	f.start(0, 10, 8, 0, blocks)
	evenSide := make([]int32, blocks)
	for i := range evenSide {
		evenSide[i] = same[i] - even[i]
	}
	f.subframe("constant", same, 16)
	f.subframe("verbatim", evenSide, 17)
	check("constant with side", f.finish(), same, even)
}

// A stream whose frames count samples rather than frames, in changing sizes.
func TestFlacVariableBlockSizes(t *testing.T) {
	const samples = 40000
	pcm := intTone(0, samples/SectorSamples+1)[:samples*SampleBytes]
	var out bytes.Buffer
	out.WriteString("fLaC")
	var info [38]byte
	info[0] = 0x80
	info[3] = 34
	binary.BigEndian.PutUint16(info[4:], 16)
	binary.BigEndian.PutUint16(info[6:], 4096)
	binary.BigEndian.PutUint64(info[14:], uint64(SampleRate)<<44|1<<41|15<<36|samples)
	out.Write(info[:])
	var f frameBuilder
	sizes := []int{4096, 1, 777, 256, 192, 3000, 16, 4096}
	for at, i := 0, 0; at < samples; i++ {
		n := min(sizes[i%len(sizes)], samples-at)
		l, r := make([]int32, n), make([]int32, n)
		for j := range n {
			l[j] = int32(int16(binary.LittleEndian.Uint16(pcm[(at+j)*4:])))
			r[j] = int32(int16(binary.LittleEndian.Uint16(pcm[(at+j)*4+2:])))
		}
		f.start(1, 7, 1, uint64(at), n)
		f.subframe("verbatim", l, 16)
		f.subframe("verbatim", r, 16)
		out.Write(f.finish())
		at += n
	}
	r, n := openFlacBytes(t, out.Bytes())
	if n != samples*SampleBytes {
		t.Fatalf("length %d", n)
	}
	if !bytes.Equal(readAll(t, r, n), pcm) {
		t.Fatal("variable-size frames came back wrong")
	}
	rnd := rand.New(rand.NewPCG(13, 14))
	for range 200 {
		off := rnd.Int64N(n - 100)
		got := make([]byte, 100)
		r.ReadAt(got, off)
		if !bytes.Equal(got, pcm[off:off+100]) {
			t.Fatalf("offset %d differs", off)
		}
	}
}

// A real encoder's output, with real filters: a small file ffmpeg made from intTone.
func TestFlacFromAnotherEncoder(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "lpc.flac"))
	if err != nil {
		t.Fatal(err)
	}
	r, n, err := newFlacSource(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	want := intTone(0, 40)
	if n != int64(len(want)) || !bytes.Equal(readAll(t, r, n), want) {
		t.Fatalf("length %d, want %d, or the samples differ", n, len(want))
	}
	// It is only worth having if it does use a filter.
	s := r.(*flacSource)
	var h flacHeader
	lpc := false
	var sc flacScratch
	for at := s.first; at < int64(len(b)); {
		_, m, err := decodeFlacFrame(b[at:], &sc)
		if err != nil {
			t.Fatal(err)
		}
		h.parse(b[at:])
		lpc = lpc || b[at+int64(h.size)]>>1&0x3f >= 32
		at += int64(m)
	}
	if !lpc {
		t.Fatal("testdata/lpc.flac has no LPC subframes")
	}
}

func TestFFmpegDecodesOurFLAC(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg isn't installed here")
	}
	pcm := intTone(0, 400)
	cases := map[string]struct {
		flac []byte
		want []byte
	}{
		"whole": {flacOf(t, pcm, 0), pcm},
		"part":  {flacOf(t, pcm, 12345), pcm[12345*SampleBytes:]},
	}
	for name, c := range cases {
		dir := t.TempDir()
		p := write(t, dir, "x.flac", c.flac)
		cmd := exec.Command(ffmpeg, "-v", "error", "-i", p, "-f", "s16le", "-acodec", "pcm_s16le", "-")
		var errs bytes.Buffer
		cmd.Stderr = &errs
		got, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: %v: %s", name, err, errs.String())
		}
		if errs.Len() > 0 {
			t.Errorf("%s: ffmpeg complained: %s", name, errs.String())
		}
		if !bytes.Equal(got, c.want) {
			t.Errorf("%s: ffmpeg read %d bytes, different from the %d that went in", name, len(got), len(c.want))
		}
	}
}

// Other encoders' files, in the settings that use the most of the format, when
// those encoders are installed. Each is read through and then read at random.
func TestFlacFromRealEncoders(t *testing.T) {
	ffmpeg, _ := exec.LookPath("ffmpeg")
	flac, _ := exec.LookPath("flac")
	if ffmpeg == "" && flac == "" {
		t.Skip("neither ffmpeg nor flac is installed here")
	}
	dir := t.TempDir()
	pcm := tone(0, 2500, 523)
	raw := write(t, dir, "x.raw", pcm)
	check := func(name string, cmd *exec.Cmd, p string) {
		t.Helper()
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s: %v\n%s", name, err, b)
			return
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		r, n, err := newFlacSource(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			return
		}
		if n != int64(len(pcm)) || !bytes.Equal(readAll(t, r, n), pcm) {
			t.Errorf("%s: length %d of %d, or the samples differ", name, n, len(pcm))
			return
		}
		r, _, _ = newFlacSource(bytes.NewReader(b), int64(len(b)))
		rnd := rand.New(rand.NewPCG(15, 16))
		got := make([]byte, 9000)
		for range 100 {
			off := rnd.Int64N(n - 9000)
			r.ReadAt(got, off)
			if !bytes.Equal(got, pcm[off:off+9000]) {
				t.Errorf("%s: a random read at %d differs", name, off)
				return
			}
		}
	}
	if ffmpeg != "" {
		for i, opts := range [][]string{
			{"-compression_level", "0"},
			{"-compression_level", "5"},
			{"-compression_level", "12"},
			{"-compression_level", "12", "-lpc_type", "cholesky", "-lpc_passes", "4", "-exact_rice_parameters", "1"},
			{"-frame_size", "576", "-ch_mode", "indep"},
			{"-frame_size", "1152", "-ch_mode", "left_side"},
			{"-frame_size", "4608", "-ch_mode", "right_side"},
			{"-frame_size", "16384", "-ch_mode", "mid_side"},
			{"-frame_size", "1000", "-lpc_type", "fixed"},
			{"-frame_size", "65535", "-compression_level", "8"},
		} {
			p := filepath.Join(dir, fmt.Sprintf("f%d.flac", i))
			args := append([]string{"-y", "-v", "error", "-f", "s16le", "-ar", "44100", "-ac", "2", "-i", raw}, opts...)
			check(fmt.Sprint("ffmpeg ", opts), exec.Command(ffmpeg, append(args, p)...), p)
		}
	}
	if flac != "" {
		for i, opts := range [][]string{
			{"-0"},
			{"-8"},
			{"-8", "-e", "-p", "-r", "8"},
			{"-5", "-b", "576", "--no-seektable"},
			{"-8", "--lax", "-b", "16384", "-S", "1s"},
			{"-5", "--no-mid-side"},
		} {
			p := filepath.Join(dir, fmt.Sprintf("l%d.flac", i))
			args := append([]string{"-f", "-s", "--force-raw-format", "--endian=little", "--sign=signed", "--channels=2", "--bps=16", "--sample-rate=44100", "-o", p}, opts...)
			check(fmt.Sprint("flac ", opts), exec.Command(flac, append(args, raw)...), p)
		}
	}
}

func BenchmarkFlacDecode(b *testing.B) {
	pcm := intTone(0, 7500)
	enc := flacOf(b, pcm, 0)
	buf := make([]byte, 64<<10)
	b.SetBytes(int64(len(pcm)))
	for b.Loop() {
		r, n, err := newFlacSource(bytes.NewReader(enc), int64(len(enc)))
		if err != nil {
			b.Fatal(err)
		}
		for off := int64(0); off < n; off += int64(len(buf)) {
			r.ReadAt(buf, off)
		}
	}
}

func BenchmarkFlacSeek(b *testing.B) {
	pcm := intTone(0, 20000)
	enc := flacOf(b, pcm, 0)
	r, n, err := newFlacSource(bytes.NewReader(enc), int64(len(enc)))
	if err != nil {
		b.Fatal(err)
	}
	rnd := rand.New(rand.NewPCG(1, 1))
	buf := make([]byte, 4096)
	for b.Loop() {
		r.ReadAt(buf, rnd.Int64N(n-4096))
	}
}
