package cdrip

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeDisc is a drive that does what real ones do: fails sometimes, and now and then
// hands back different samples for the same place.
type fakeDisc struct {
	pcm []byte
	inf *Info
	// fail[sector] is how many more times reading that sector fails.
	fail map[int]int
	// jitter[sector] is how many more reads of that sector come back wrong.
	jitter map[int]int
	reads  int
	exact  bool
}

func (f *fakeDisc) Info() *Info  { return f.inf }
func (f *fakeDisc) Exact() bool  { return f.exact }
func (f *fakeDisc) Close() error { return nil }

func (f *fakeDisc) ReadSectors(lba int, p []byte) (int, error) {
	f.reads++
	for s := 0; s*SectorBytes < len(p); s++ {
		sec := lba + s
		if f.fail[sec] > 0 {
			f.fail[sec]--
			return s * SectorBytes, fmt.Errorf("can't read sector %d", sec)
		}
		d := p[s*SectorBytes : (s+1)*SectorBytes]
		if sec < 0 || (sec+1)*SectorBytes > len(f.pcm) {
			clear(d)
			continue
		}
		copy(d, f.pcm[sec*SectorBytes:(sec+1)*SectorBytes])
		if f.jitter[sec] > 0 {
			f.jitter[sec]--
			d[10] ^= byte(f.jitter[sec] + 1)
		}
	}
	return len(p), nil
}

func tone(sectors int) []byte {
	b := make([]byte, sectors*SectorBytes)
	for i := 0; i+4 <= len(b); i += 4 {
		v := int16(12000 * math.Sin(float64(i)/4/44100*2*math.Pi*440))
		binary.LittleEndian.PutUint16(b[i:], uint16(v))
		binary.LittleEndian.PutUint16(b[i+2:], uint16(-v))
	}
	return b
}

func disc(sectors int) *fakeDisc {
	pcm := tone(sectors)
	return &fakeDisc{pcm: pcm, fail: map[int]int{}, jitter: map[int]int{}, inf: &Info{Kind: "audiocd", Tracks: []Track{
		{Number: 1, Audio: true, Pregap: 0, Start: 0, End: sectors / 2},
		{Number: 2, Audio: true, Pregap: sectors / 2, Start: sectors / 2, End: sectors},
	}}}
}

func rip(t *testing.T, f *fakeDisc, n int, opt Options) ([]byte, *Report, error) {
	t.Helper()
	var out bytes.Buffer
	tr := &f.inf.Tracks[n]
	rep, err := RipTrack(context.Background(), f, tr, opt, nil, func(p []byte) error {
		out.Write(p)
		return nil
	})
	return out.Bytes(), rep, err
}

func TestCleanCopy(t *testing.T) {
	f := disc(500)
	got, rep, err := rip(t, f, 1, Options{Secure: true})
	if err != nil {
		t.Fatal(err)
	}
	want := f.pcm[250*SectorBytes : 500*SectorBytes]
	if !bytes.Equal(got, want) {
		t.Fatal("the copy differs")
	}
	if !rep.Verified || rep.Retries != 0 || rep.Unreadable != 0 || rep.CRC != fmt.Sprintf("%08X", crc32.ChecksumIEEE(want)) {
		t.Errorf("%+v", rep)
	}
	if rep.Quality() != "Read correctly, checked twice" {
		t.Error(rep.Quality())
	}
}

func TestReadFailuresAreRetried(t *testing.T) {
	f := disc(300)
	f.fail[50] = 2 // fails twice, then reads
	got, rep, err := rip(t, f, 0, Options{Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, f.pcm[:150*SectorBytes]) || rep.Retries == 0 || rep.Unreadable != 0 {
		t.Errorf("%+v", rep)
	}
}

func TestSectorThatNeverReadsIsSilenceAndCounted(t *testing.T) {
	f := disc(300)
	f.fail[50] = 1 << 30
	f.fail[51] = 1 << 30
	got, rep, err := rip(t, f, 0, Options{Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Unreadable != 2 || !strings.Contains(rep.Quality(), "2 sectors") {
		t.Errorf("%+v", rep)
	}
	want := bytes.Clone(f.pcm[:150*SectorBytes])
	clear(want[50*SectorBytes : 52*SectorBytes])
	if !bytes.Equal(got, want) {
		t.Error("everything readable should be there, and the rest silent")
	}
}

// Two reads that disagree are read again until two in a row match.
func TestDisagreementIsSettled(t *testing.T) {
	f := disc(300)
	f.jitter[60] = 2 // the first two reads of this sector are wrong, and differ
	got, rep, err := rip(t, f, 0, Options{Secure: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Disagreed != 1 || rep.Unreadable != 0 || !rep.Verified {
		t.Errorf("%+v", rep)
	}
	if !bytes.Equal(got, f.pcm[:150*SectorBytes]) {
		t.Error("it should have settled on the true samples")
	}
	if rep.Quality() != "Read correctly, after some retries" {
		t.Error(rep.Quality())
	}
}

// Without verifying, a bad read isn't noticed, which is why secure is the default.
func TestBurstReadsOnce(t *testing.T) {
	f := disc(300)
	_, rep, err := rip(t, f, 0, Options{Secure: false})
	if err != nil || rep.Verified {
		t.Fatalf("%+v %v", rep, err)
	}
	if f.reads > 150/chunkSectors+1 {
		t.Errorf("%d reads", f.reads)
	}
}

// An image can't read differently, so it needs no second pass.
func TestImagesReadOnce(t *testing.T) {
	f := disc(300)
	f.exact = true
	if _, _, err := rip(t, f, 0, Options{Secure: true}); err != nil {
		t.Fatal(err)
	}
	if f.reads > 150/chunkSectors+1 {
		t.Errorf("%d reads", f.reads)
	}
}

func TestDriveOffsetShiftsTheSamples(t *testing.T) {
	f := disc(400)
	got, _, err := rip(t, f, 0, Options{Secure: false, Offset: 6})
	if err != nil {
		t.Fatal(err)
	}
	want := f.pcm[6*SampleBytes : 6*SampleBytes+200*SectorBytes]
	if !bytes.Equal(got, want) {
		t.Error("a positive offset reads later samples")
	}
	got, _, err = rip(t, f, 1, Options{Secure: false, Offset: -30})
	if err != nil {
		t.Fatal(err)
	}
	start := 200*SectorBytes - 30*SampleBytes
	if !bytes.Equal(got, f.pcm[start:start+200*SectorBytes]) {
		t.Error("a negative offset reads earlier samples")
	}
	// Past the disc's start, it's silence.
	got, _, err = rip(t, f, 0, Options{Secure: false, Offset: -10})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got[:10*SampleBytes], make([]byte, 10*SampleBytes)) || !bytes.Equal(got[10*SampleBytes:], f.pcm[:200*SectorBytes-10*SampleBytes]) {
		t.Error("before the disc's start should be silence")
	}
}

// A disc that won't read at all gets a clear message rather than a silent copy.
func TestDiscThatWontReadStops(t *testing.T) {
	f := disc(6000)
	for s := 0; s < 3000; s++ {
		f.fail[s] = 1 << 30
	}
	_, _, err := rip(t, f, 0, Options{Secure: false})
	if !errors.Is(err, ErrNoRead) {
		t.Fatalf("%v", err)
	}
	if !strings.Contains(err.Error(), "can't be read normally") {
		t.Error(err)
	}
}

func TestStopWorks(t *testing.T) {
	f := disc(300)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := RipTrack(ctx, f, &f.inf.Tracks[0], Options{}, nil, func([]byte) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
}

func TestStreamIsAWAVOfTheAudio(t *testing.T) {
	f := disc(300)
	f.inf.Tracks[0].Start = 10
	l := StreamLayout(f.inf)
	if l.First != 10 || l.Last != 300 {
		t.Fatalf("%+v", l)
	}
	head := make([]byte, 44)
	if n, err := ReadStream(f, l, head, 0); n != 44 || err != nil || string(head[:4]) != "RIFF" || string(head[36:40]) != "data" {
		t.Fatalf("%d %v %q", n, err, head[:4])
	}
	if got := binary.LittleEndian.Uint32(head[40:]); got != 290*SectorBytes {
		t.Errorf("data size %d", got)
	}
	// Any range reads, across the header's end and part way through sectors.
	p := make([]byte, 5000)
	n, err := ReadStream(f, l, p, 30)
	if err != nil || n != 5000 {
		t.Fatalf("%d %v", n, err)
	}
	if !bytes.Equal(p[14:], f.pcm[10*SectorBytes:10*SectorBytes+5000-14]) {
		t.Error("the samples after the header")
	}
	off := int64(44 + 7*SectorBytes + 100)
	n, err = ReadStream(f, l, p, off)
	if err != nil || n != 5000 || !bytes.Equal(p, f.pcm[17*SectorBytes+100:17*SectorBytes+5100]) {
		t.Errorf("mid-sector read %d %v", n, err)
	}
	// A scratched sector plays as silence rather than stopping.
	f.fail[20] = 1 << 30
	if n, err := ReadStream(f, l, p[:SectorBytes], 44+10*SectorBytes); err != nil || n != SectorBytes {
		t.Errorf("%d %v", n, err)
	}
}

// Real images, made here from nothing: a cue and bin, read back through WittyCD.
func writeImage(t *testing.T, dir string, tracks []int) (string, []byte) {
	t.Helper()
	total := 0
	for _, s := range tracks {
		total += s
	}
	pcm := tone(total)
	if err := os.WriteFile(filepath.Join(dir, "disc.bin"), pcm, 0o644); err != nil {
		t.Fatal(err)
	}
	var cue strings.Builder
	cue.WriteString("PERFORMER \"The Testers\"\nTITLE \"Made Here\"\nFILE \"disc.bin\" BINARY\n")
	at := 0
	for i, s := range tracks {
		fmt.Fprintf(&cue, "  TRACK %02d AUDIO\n    TITLE \"Song %d\"\n    INDEX 01 %s\n", i+1, i+1, msf(at))
		at += s
	}
	path := filepath.Join(dir, "disc.cue")
	if err := os.WriteFile(path, []byte(cue.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path, pcm
}

func TestRealImageThroughWittyCD(t *testing.T) {
	dir := t.TempDir()
	cue, pcm := writeImage(t, dir, []int{400, 600, 300})
	src, err := Open(cue)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	info := src.Info()
	if len(info.AudioTracks()) != 3 || info.Title != "Made Here" || info.Performer != "The Testers" || info.Tracks[1].Title != "Song 2" || info.DiscID == "" || info.TOC == "" || !src.Exact() {
		t.Fatalf("%+v", info)
	}
	var out bytes.Buffer
	rep, err := RipTrack(context.Background(), src, info.AudioTracks()[1], Options{Secure: true}, nil, func(p []byte) error {
		out.Write(p)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), pcm[400*SectorBytes:1000*SectorBytes]) {
		t.Error("track 2's samples differ")
	}
	if rep.Unreadable != 0 || rep.Retries != 0 {
		t.Errorf("%+v", rep)
	}
	// The whole disc to a new cue and bin, which reads back the same.
	img := filepath.Join(t.TempDir(), "copy.cue")
	if _, err := WriteImage(context.Background(), src, img, Options{Secure: true}, nil); err != nil {
		t.Fatal(err)
	}
	bin, err := os.ReadFile(strings.TrimSuffix(img, ".cue") + ".bin")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bin, pcm) {
		t.Error("the image's samples differ")
	}
	back, err := Open(img)
	if err != nil {
		t.Fatal(err)
	}
	defer back.Close()
	if back.Info().DiscID != info.DiscID || back.Info().Tracks[2].Title != "Song 3" || back.Info().Title != "Made Here" {
		t.Errorf("%+v", back.Info())
	}
	// And it never overwrites.
	if _, err := WriteImage(context.Background(), src, img, Options{}, nil); err == nil || !strings.Contains(err.Error(), "never overwrites") {
		t.Errorf("%v", err)
	}
	ents, _ := os.ReadDir(filepath.Dir(img))
	if len(ents) != 2 {
		t.Errorf("left behind: %v", ents)
	}
}

func TestStoppedImageLeavesNothing(t *testing.T) {
	f := disc(300)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dir := t.TempDir()
	if _, err := WriteImage(ctx, f, filepath.Join(dir, "x.cue"), Options{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 0 {
		t.Errorf("left behind: %v", ents)
	}
}

// A file that appears during the copy is still never overwritten.
func TestImageNeverOverwritesWhatAppearedMeanwhile(t *testing.T) {
	for _, name := range []string{"x.bin", "x.cue"} {
		dir := t.TempDir()
		there := filepath.Join(dir, name)
		appeared := false
		_, err := WriteImage(context.Background(), disc(40), filepath.Join(dir, "x.cue"), Options{}, func(int, int) {
			if !appeared {
				appeared = true
				if err := os.WriteFile(there, []byte("mine"), 0o644); err != nil {
					t.Error(err)
				}
			}
		})
		if err == nil || !strings.Contains(err.Error(), "already there") {
			t.Errorf("%s: %v", name, err)
		}
		if b, _ := os.ReadFile(there); string(b) != "mine" {
			t.Errorf("%s was overwritten", name)
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) != 1 {
			t.Errorf("%s: left behind %v", name, entries)
		}
	}
}

func TestCueSheet(t *testing.T) {
	info := &Info{Title: `A "Quoted" Album`, Tracks: []Track{{Number: 1, Audio: true, Pregap: 0, Start: 150, End: 300, Title: "One", ISRC: "USABC0000001"}, {Number: 2, Pregap: 300, Start: 300, End: 900}}}
	got := CueSheet(info, "x.bin")
	for _, want := range []string{`TITLE "A 'Quoted' Album"`, `FILE "x.bin" BINARY`, "TRACK 01 AUDIO", "ISRC USABC0000001", "INDEX 00 00:00:00", "INDEX 01 00:02:00", "TRACK 02 MODE1/2352", "INDEX 01 00:04:00"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in:\n%s", want, got)
		}
	}
}
