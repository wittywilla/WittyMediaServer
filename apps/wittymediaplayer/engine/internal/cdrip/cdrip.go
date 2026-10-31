// Package cdrip reads audio CDs, from a drive or an image, for playing and copying. It
// reads what the disc gives by ordinary means and nothing more: a disc that won't read
// normally gets a clear message, and nothing here works around copy protection.
package cdrip

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// SectorBytes is one sector of CD audio, 1/75 of a second.
	SectorBytes = 2352
	// SectorSamples is the stereo samples in one.
	SectorSamples = 588
	// SampleBytes is one stereo sample.
	SampleBytes = 4
	// LeadIn is the 2 seconds before sector 0.
	LeadIn = 150

	// chunkSectors is what's read at a time: a little over half a second, so a bad spot
	// costs one short re-read rather than the whole track.
	chunkSectors = 40
	// tries is how many reads a chunk gets before going sector by sector.
	tries = 4
	// sectorTries is how many reads one sector gets before it's given up on.
	sectorTries = 8
	// tooManyBad stops a track that clearly isn't going to read: about 30 seconds of it.
	tooManyBad = 2250
)

// Track is one track of a disc's table of contents.
type Track struct {
	Number    int    `json:"number"`
	Audio     bool   `json:"audio"`
	Pregap    int    `json:"pregap"`
	Start     int    `json:"start"`
	End       int    `json:"end"`
	Title     string `json:"title,omitempty"`
	Performer string `json:"performer,omitempty"`
	ISRC      string `json:"isrc,omitempty"`
	// PreEmphasis: the track was mastered with emphasis; a copy keeps the disc's samples.
	PreEmphasis bool `json:"preEmphasis,omitempty"`
}

// Sectors is the track's length from its start to its end.
func (t *Track) Sectors() int { return t.End - t.Start }

// Info describes a disc.
type Info struct {
	Kind      string  `json:"kind"` // always audiocd
	Drive     bool    `json:"drive"`
	Path      string  `json:"path"`
	Title     string  `json:"title,omitempty"`
	Performer string  `json:"performer,omitempty"`
	Date      string  `json:"date,omitempty"`
	Tracks    []Track `json:"tracks"`
	// For looking the disc up: only these numbers ever leave the computer.
	DiscID string `json:"discId,omitempty"`
	TOC    string `json:"toc,omitempty"`
	CDDBID string `json:"cddbId,omitempty"`
	// Warnings are plain things worth knowing about the disc.
	Warnings []string `json:"warnings,omitempty"`
}

// AudioTracks lists the tracks with sound.
func (in *Info) AudioTracks() []*Track {
	var out []*Track
	for i := range in.Tracks {
		if in.Tracks[i].Audio {
			out = append(out, &in.Tracks[i])
		}
	}
	return out
}

// Source gives a disc's audio by sector.
type Source interface {
	Info() *Info
	// ReadSectors reads little-endian PCM from a sector, whole sectors at a time. It
	// returns how many bytes it read, with an error if that isn't all of them.
	ReadSectors(lba int, p []byte) (int, error)
	// Exact says reads can't vary: an image needs no second pass.
	Exact() bool
	Close() error
}

// Open opens a drive or an image.
func Open(path string) (Source, error) {
	if isDriveRoot(path) {
		return openDrive(path)
	}
	return openImage(path)
}

func isDriveRoot(p string) bool {
	return len(p) >= 2 && len(p) <= 3 && p[1] == ':' && (len(p) == 2 || p[2] == '\\' || p[2] == '/')
}

// Options say how to read.
type Options struct {
	// Secure reads everything twice, and again where two reads disagree.
	Secure bool
	// Offset corrects the drive's read offset, in samples.
	Offset int
}

// Report says how one track went.
type Report struct {
	Number int   `json:"number"`
	Bytes  int64 `json:"bytes"`
	// CRC is the CRC-32 of the samples delivered, for comparing with other copies.
	CRC string `json:"crc"`
	// Verified: two reads agreed on all of it.
	Verified bool `json:"verified"`
	// Retries is how many times something had to be read again.
	Retries int `json:"retries"`
	// Disagreed is how many chunks differed between reads before settling.
	Disagreed int `json:"disagreed"`
	// Unreadable is how many sectors never read, and are silence in the copy.
	Unreadable int     `json:"unreadable"`
	Seconds    float64 `json:"seconds"`
}

// Quality says in words how a track came out.
func (r *Report) Quality() string {
	switch {
	case r.Unreadable > 0:
		return fmt.Sprintf("%d sectors couldn't be read and are silent", r.Unreadable)
	case r.Disagreed > 0 || r.Retries > 0:
		return "Read correctly, after some retries"
	case r.Verified:
		return "Read correctly, checked twice"
	}
	return "Read correctly"
}

// ErrNoRead is a disc that won't read by ordinary means. It's all a copy-protected disc
// ever gets from this program.
var ErrNoRead = errors.New("this disc can't be read normally. It may be dirty or damaged, or made so that ordinary reading fails. WittyMediaPlayer only reads discs the ordinary way")

// reader reads with retries, and with the drive's offset applied.
type reader struct {
	src    Source
	ctx    context.Context
	offset int
	report *Report
	tmp    []byte
}

// read fills p with the audio at a sample position, reading whatever sectors that spans.
// Before the disc's first audio and after its last, it's silence.
func (r *reader) read(sector, sectors int, p []byte) (bad int, err error) {
	pos := int64(sector)*SectorSamples + int64(r.offset)
	first := int(floorDiv(pos, SectorSamples))
	skip := int(pos-int64(first)*SectorSamples) * SampleBytes
	span := sectors
	if skip > 0 {
		span++
	}
	if need := span * SectorBytes; cap(r.tmp) < need {
		r.tmp = make([]byte, need)
	}
	buf := r.tmp[:span*SectorBytes]
	bad, err = r.readRange(first, span, buf)
	if err != nil {
		return bad, err
	}
	copy(p, buf[skip:skip+sectors*SectorBytes])
	return bad, nil
}

func floorDiv(a, b int64) int64 {
	d := a / b
	if a%b < 0 {
		d--
	}
	return d
}

// readRange reads sectors with retries, then one by one, and leaves what never reads as
// silence, counted.
func (r *reader) readRange(first, count int, buf []byte) (int, error) {
	for try := 0; try < tries; try++ {
		if err := r.ctx.Err(); err != nil {
			return 0, err
		}
		if try > 0 {
			r.report.Retries++
		}
		n, err := r.src.ReadSectors(first, buf)
		if n == len(buf) {
			return 0, nil
		}
		if errors.Is(err, ErrDiscGone) {
			return 0, err
		}
		if err == nil {
			break
		}
	}
	bad := 0
	for i := 0; i < count; i++ {
		if err := r.ctx.Err(); err != nil {
			return bad, err
		}
		s := buf[i*SectorBytes : (i+1)*SectorBytes]
		ok := false
		for try := 0; try < sectorTries && !ok; try++ {
			r.report.Retries++
			n, err := r.src.ReadSectors(first+i, s)
			if errors.Is(err, ErrDiscGone) {
				return bad, err
			}
			ok = n == len(s)
		}
		if !ok {
			clear(s)
			bad++
		}
	}
	return bad, nil
}

// RipTrack reads one track and hands its samples to out in order. With Secure it reads
// everything twice, delivering on the second pass only what agrees with the first; where
// they differ, it reads until two reads match. Nothing is held on disk, and a chunk of
// memory is all it needs however long the track is.
func RipTrack(ctx context.Context, src Source, t *Track, opt Options, progress func(done, total int), out func([]byte) error) (*Report, error) {
	started := time.Now()
	rep := &Report{Number: t.Number}
	r := &reader{src: src, ctx: ctx, offset: opt.Offset, report: rep}
	secure := opt.Secure && !src.Exact()
	total := t.Sectors()
	chunks := (total + chunkSectors - 1) / chunkSectors
	if total <= 0 {
		return nil, fmt.Errorf("track %d has no audio", t.Number)
	}
	work := total
	if secure {
		work *= 2
	}
	done := 0
	var first []uint32
	buf := make([]byte, chunkSectors*SectorBytes)
	size := func(c int) int { return min(chunkSectors, total-c*chunkSectors) }
	if secure {
		first = make([]uint32, chunks)
		for c := range first {
			n := size(c)
			if _, err := r.read(t.Start+c*chunkSectors, n, buf[:n*SectorBytes]); err != nil {
				return rep, err
			}
			first[c] = crc32.ChecksumIEEE(buf[:n*SectorBytes])
			done += n
			if progress != nil {
				progress(done, work)
			}
		}
	}
	crc := crc32.NewIEEE()
	for c := 0; c < chunks; c++ {
		n := size(c)
		p := buf[:n*SectorBytes]
		bad, err := r.read(t.Start+c*chunkSectors, n, p)
		if err != nil {
			return rep, err
		}
		if secure {
			sum := crc32.ChecksumIEEE(p)
			if sum != first[c] {
				rep.Disagreed++
				// Keep reading until two reads in a row agree.
				prev := sum
				settled := false
				for try := 0; try < sectorTries; try++ {
					rep.Retries++
					if bad, err = r.read(t.Start+c*chunkSectors, n, p); err != nil {
						return rep, err
					}
					sum = crc32.ChecksumIEEE(p)
					if sum == prev {
						settled = true
						break
					}
					prev = sum
				}
				if !settled {
					// It reads differently every time: as good as unreadable.
					rep.Unreadable += n
					bad = 0
				}
			}
		}
		rep.Unreadable += bad
		if rep.Unreadable > tooManyBad {
			return rep, ErrNoRead
		}
		crc.Write(p)
		rep.Bytes += int64(len(p))
		if err := out(p); err != nil {
			return rep, err
		}
		done += n
		if progress != nil {
			progress(done, work)
		}
	}
	rep.CRC = fmt.Sprintf("%08X", crc.Sum32())
	rep.Verified = secure && rep.Unreadable == 0
	rep.Seconds = time.Since(started).Seconds()
	return rep, nil
}

// WriteImage copies a disc's audio to a .bin and a .cue beside it. It writes to temporary
// names first and never overwrites anything. A data track on the same disc is left out and
// said so: an image is for the music.
func WriteImage(ctx context.Context, src Source, cuePath string, opt Options, progress func(done, total int)) (*Report, error) {
	info := src.Info()
	if !strings.EqualFold(filepath.Ext(cuePath), ".cue") {
		return nil, errors.New("the image's name has to end in .cue")
	}
	binPath := strings.TrimSuffix(cuePath, filepath.Ext(cuePath)) + ".bin"
	for _, p := range []string{cuePath, binPath} {
		if _, err := os.Lstat(p); err == nil {
			return nil, fmt.Errorf("%s is already there, and copying a disc never overwrites anything", filepath.Base(p))
		}
	}
	if len(info.AudioTracks()) == 0 {
		return nil, errors.New("this disc has no audio tracks")
	}
	audio := info.AudioTracks()
	first, end := audio[0].Pregap, audio[len(audio)-1].End
	rep := &Report{}
	started := time.Now()
	f, err := os.OpenFile(binPath+".part", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, err
	}
	finished := false
	defer func() {
		f.Close()
		if !finished {
			_ = os.Remove(binPath + ".part")
		}
	}()
	// Everything from the first sector, read as one long track so it gets the same care.
	whole := &Track{Number: 0, Audio: true, Start: first, End: end}
	got, err := RipTrack(ctx, src, whole, opt, progress, func(p []byte) error {
		_, err := f.Write(p)
		return err
	})
	if err != nil {
		return got, err
	}
	*rep = *got
	rep.Seconds = time.Since(started).Seconds()
	if err := f.Sync(); err != nil {
		return rep, err
	}
	f.Close()
	if err := os.WriteFile(cuePath+".part", []byte(CueSheet(audioOnly(info, first), filepath.Base(binPath))), 0o644); err != nil {
		return rep, err
	}
	defer os.Remove(cuePath + ".part")
	if err := os.Rename(binPath+".part", binPath); err != nil {
		return rep, err
	}
	if err := os.Rename(cuePath+".part", cuePath); err != nil {
		_ = os.Remove(binPath)
		return rep, err
	}
	finished = true
	return rep, nil
}

// audioOnly is a disc's table of contents as its audio-only image has it: the audio tracks,
// counted from where the image starts.
func audioOnly(info *Info, first int) *Info {
	c := *info
	c.Tracks = nil
	for _, t := range info.AudioTracks() {
		n := *t
		n.Pregap -= first
		n.Start -= first
		n.End -= first
		c.Tracks = append(c.Tracks, n)
	}
	return &c
}

func msf(lba int) string {
	return fmt.Sprintf("%02d:%02d:%02d", lba/75/60, lba/75%60, lba%75)
}

func cueQuote(s string) string { return strings.NewReplacer(`"`, `'`, "\n", " ", "\r", " ").Replace(s) }

// CueSheet writes a disc's table of contents for one .bin of the whole disc.
func CueSheet(info *Info, binName string) string {
	var b strings.Builder
	if info.Performer != "" {
		fmt.Fprintf(&b, "PERFORMER \"%s\"\r\n", cueQuote(info.Performer))
	}
	if info.Title != "" {
		fmt.Fprintf(&b, "TITLE \"%s\"\r\n", cueQuote(info.Title))
	}
	fmt.Fprintf(&b, "FILE \"%s\" BINARY\r\n", cueQuote(binName))
	for i := range info.Tracks {
		t := &info.Tracks[i]
		mode := "AUDIO"
		if !t.Audio {
			mode = "MODE1/2352"
		}
		fmt.Fprintf(&b, "  TRACK %02d %s\r\n", t.Number, mode)
		if t.Title != "" {
			fmt.Fprintf(&b, "    TITLE \"%s\"\r\n", cueQuote(t.Title))
		}
		if t.Performer != "" {
			fmt.Fprintf(&b, "    PERFORMER \"%s\"\r\n", cueQuote(t.Performer))
		}
		if t.ISRC != "" {
			fmt.Fprintf(&b, "    ISRC %s\r\n", t.ISRC)
		}
		if t.PreEmphasis {
			b.WriteString("    FLAGS PRE\r\n")
		}
		if t.Pregap < t.Start {
			fmt.Fprintf(&b, "    INDEX 00 %s\r\n", msf(t.Pregap))
		}
		fmt.Fprintf(&b, "    INDEX 01 %s\r\n", msf(t.Start))
	}
	return b.String()
}

// Layout is where a disc's audio sits in one continuous stream, for playing.
type Layout struct {
	First, Last int // sectors
}

// StreamLayout is the stretch of a disc that plays: from the first audio track's start to
// the last one's end, gaps included as they are on the disc.
func StreamLayout(info *Info) Layout {
	audio := info.AudioTracks()
	if len(audio) == 0 {
		return Layout{}
	}
	return Layout{First: audio[0].Start, Last: audio[len(audio)-1].End}
}

// WAVHeader is the 44 bytes before CD audio of this many bytes.
func WAVHeader(bytes int64) []byte {
	le32 := func(b []byte, v uint32) { b[0], b[1], b[2], b[3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24) }
	h := make([]byte, 44)
	copy(h, "RIFF")
	le32(h[4:], uint32(min(bytes+36, 0xffffffff)))
	copy(h[8:], "WAVEfmt ")
	le32(h[16:], 16)
	h[20], h[22] = 1, 2 // PCM, stereo
	le32(h[24:], 44100)
	le32(h[28:], 44100*SampleBytes)
	h[32], h[34] = SampleBytes, 16
	copy(h[36:], "data")
	le32(h[40:], uint32(min(bytes, 0xffffffff)))
	return h
}

// ReadStream reads the WAV form of a disc's playing stretch at a byte offset: what the
// player's decoder reads. A sector that won't read plays as silence rather than stopping.
func ReadStream(src Source, l Layout, p []byte, off int64) (int, error) {
	total := int64(l.Last-l.First) * SectorBytes
	h := WAVHeader(total)
	n := 0
	if off < int64(len(h)) {
		n = copy(p, h[off:])
		off += int64(n)
		p = p[n:]
	}
	off -= int64(len(h))
	if off >= total || len(p) == 0 {
		return n, nil
	}
	sector := int(off/SectorBytes) + l.First
	skip := int(off % SectorBytes)
	want := min(int64(len(p)), total-off)
	count := int((int64(skip) + want + SectorBytes - 1) / SectorBytes)
	buf := make([]byte, count*SectorBytes)
	got, err := src.ReadSectors(sector, buf)
	if got < len(buf) {
		if err != nil && got == 0 && !isGone(err) {
			clear(buf)
			got = len(buf)
		} else if err != nil && got == 0 {
			return n, err
		}
		clear(buf[got:])
	}
	return n + copy(p, buf[skip:int64(skip)+want]), nil
}

func isGone(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrDiscGone)
}

// ErrDiscGone is a drive whose disc has been taken out.
var ErrDiscGone = errors.New("the disc was taken out of the drive")
