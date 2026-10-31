package wittycd

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
)

// A Stream is a program played as one unbroken run of samples: the whole disc as
// it is laid out, or chosen tracks in a chosen order, any number of times. The
// listener's device gets one long stream, so there is no join between tracks for
// it to get wrong, and a place in it is an exact sample.
//
// A track's sectors in a chosen order run from its index 1 to where the next
// track's pregap starts. Client and server work out the same layout from the
// track table, which is how a client knows which track it is hearing.

// HiddenTrackNumber asks a Stream for what is in track 1's pregap.
const HiddenTrackNumber = 0

// MaxLoops bounds how many times a program can be repeated in one stream.
const MaxLoops = 64

type streamRange struct {
	start   int64 // in samples from the stream's start
	lba     int
	sectors int
	emph    bool
}

// Stream reads samples of a program. It is safe for concurrent reads and holds no
// position, so one Disc can serve any number of listeners.
type Stream struct {
	d      *Disc
	ranges []streamRange
	total  int64 // samples
}

// NewStream makes a program. tracks is nil for the whole disc as laid out, with
// every pregap in place (the disc starts at sample 0, which is track 1's pregap
// when it has one). Pre-emphasised tracks are corrected when deemphasis is on,
// so they sound as they were meant to.
func (d *Disc) NewStream(tracks []int, loops int, deemphasis bool) (*Stream, error) {
	if loops < 1 {
		loops = 1
	}
	if loops > MaxLoops {
		return nil, fmt.Errorf("a program can repeat at most %d times", MaxLoops)
	}
	if len(tracks) > 4*MaxTracks {
		return nil, errors.New("a program can't be that long")
	}
	var once []streamRange
	add := func(lba, sectors int, emph bool) {
		if sectors <= 0 {
			return
		}
		if n := len(once); n > 0 && once[n-1].emph == emph && once[n-1].lba+once[n-1].sectors == lba {
			once[n-1].sectors += sectors
			return
		}
		once = append(once, streamRange{lba: lba, sectors: sectors, emph: emph})
	}
	if tracks == nil {
		for _, t := range d.AudioTracks() {
			add(t.Pregap, t.End-t.Pregap, t.PreEmphasis && deemphasis)
		}
	} else {
		audio := d.AudioTracks()
		for _, n := range tracks {
			if n == HiddenTrackNumber && len(audio) > 0 {
				add(audio[0].Pregap, audio[0].Start-audio[0].Pregap, audio[0].PreEmphasis && deemphasis)
				continue
			}
			t := d.Track(n)
			if t == nil || !t.Audio {
				return nil, fmt.Errorf("this disc has no audio track %d", n)
			}
			add(t.Start, t.End-t.Start, t.PreEmphasis && deemphasis)
		}
	}
	if len(once) == 0 {
		return nil, errors.New("this program has nothing in it")
	}
	s := &Stream{d: d}
	for range loops {
		for _, r := range once {
			r.start = s.total
			s.ranges = append(s.ranges, r)
			s.total += int64(r.sectors) * SectorSamples
		}
	}
	return s, nil
}

// TrackStream is one track exactly as it is on the disc, index 1 to its end, with
// nothing corrected: what a rip wants.
func (d *Disc) TrackStream(n int) (*Stream, error) { return d.NewStream([]int{n}, 1, false) }

// RangeStream is any run of audio sectors exactly as it is on the disc, for a rip that
// keeps pregaps its own way. Sectors of a data track, or that the image doesn't
// supply, read as silence.
func (d *Disc) RangeStream(from, to int) (*Stream, error) {
	if from < 0 || to <= from || to > MaxSectors {
		return nil, errors.New("that isn't a run of sectors on a disc")
	}
	s := &Stream{d: d, ranges: []streamRange{{lba: from, sectors: to - from}}}
	s.total = int64(to-from) * SectorSamples
	return s, nil
}

// Reader reads the whole program in order, as little-endian samples.
func (s *Stream) Reader() io.Reader { return io.NewSectionReader(s, 0, s.Size()) }

// Samples is the program's length.
func (s *Stream) Samples() int64 { return s.total }

// Size is the program's length in bytes of little-endian PCM.
func (s *Stream) Size() int64 { return s.total * SampleBytes }

var pcmPool = sync.Pool{New: func() any { b := make([]byte, 64<<10); return &b }}

// ReadAt reads little-endian PCM. It returns io.EOF only at the end of the
// program, and any range is allowed, as HTTP range requests are.
func (s *Stream) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("negative offset")
	}
	if off >= s.Size() {
		return 0, io.EOF
	}
	short := false
	if rest := s.Size() - off; int64(len(p)) > rest {
		p, short = p[:rest], true
	}
	n, err := s.read(p, off)
	if err == nil && short {
		err = io.EOF
	}
	return n, err
}

func (s *Stream) read(p []byte, off int64) (int, error) {
	done := 0
	// A request that starts or ends part way through a sample goes through a
	// whole-sample buffer: byte order and filtering work on whole samples.
	for len(p) > 0 && (off%SampleBytes != 0 || len(p) < SampleBytes) {
		var one [SampleBytes]byte
		at := off - off%SampleBytes
		if err := s.readSamples(one[:], at); err != nil {
			return done, err
		}
		n := copy(p, one[off-at:])
		p, off, done = p[n:], off+int64(n), done+n
	}
	whole := len(p) - len(p)%SampleBytes
	if whole > 0 {
		if err := s.readSamples(p[:whole], off); err != nil {
			return done, err
		}
		p, off, done = p[whole:], off+int64(whole), done+whole
	}
	if len(p) > 0 {
		var one [SampleBytes]byte
		if err := s.readSamples(one[:], off); err != nil {
			return done, err
		}
		done += copy(p, one[:])
	}
	return done, nil
}

// readSamples fills p, which holds whole samples from a whole-sample offset.
func (s *Stream) readSamples(p []byte, off int64) error {
	sample := off / SampleBytes
	for len(p) > 0 {
		i := sort.Search(len(s.ranges), func(i int) bool {
			r := &s.ranges[i]
			return r.start+int64(r.sectors)*SectorSamples > sample
		})
		if i == len(s.ranges) {
			clear(p)
			return nil
		}
		r := &s.ranges[i]
		into := sample - r.start
		left := int64(r.sectors)*SectorSamples - into
		n := int(min(int64(len(p)/SampleBytes), left))
		if err := s.readRange(r, into, p[:n*SampleBytes]); err != nil {
			return err
		}
		p = p[n*SampleBytes:]
		sample += int64(n)
	}
	return nil
}

func (s *Stream) readRange(r *streamRange, into int64, p []byte) error {
	read := func(from int64, q []byte) error {
		return s.d.readAudio(r.lba+int(from/SectorSamples), int(from%SectorSamples)*SampleBytes, q)
	}
	if !r.emph {
		return read(into, p)
	}
	// The filter settles within this many samples, so starting that far back gives
	// the same result as playing from the start of the track.
	warm := min(into, int64(deemphWarmup))
	buf := pcmPool.Get().(*[]byte)
	defer pcmPool.Put(buf)
	var f deemphasisFilter
	for pos := into - warm; pos < into+int64(len(p)/SampleBytes); {
		n := int(min(int64(len(*buf)/SampleBytes), into+int64(len(p)/SampleBytes)-pos))
		chunk := (*buf)[:n*SampleBytes]
		if err := read(pos, chunk); err != nil {
			return err
		}
		f.apply(chunk)
		if pos+int64(n) > into {
			skip := max(0, into-pos)
			copy(p[(pos+skip-into)*SampleBytes:], chunk[skip*SampleBytes:])
		}
		pos += int64(n)
	}
	return nil
}

// WAV is a program as a WAV file: a header and then its samples, readable at any
// offset, with no work for the server beyond reading the image.
type WAV struct {
	s      *Stream
	from   int64 // bytes of PCM skipped
	header [44]byte
}

// WAV makes a file of the program from a sample onwards.
func (s *Stream) WAV(startSample int64) *WAV {
	startSample = max(0, min(startSample, s.total))
	w := &WAV{s: s, from: startSample * SampleBytes}
	data := min(s.Size()-w.from, 0xffffffff-36)
	h := w.header[:]
	copy(h, "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+data))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], 2)
	binary.LittleEndian.PutUint32(h[24:], SampleRate)
	binary.LittleEndian.PutUint32(h[28:], SampleRate*SampleBytes)
	binary.LittleEndian.PutUint16(h[32:], SampleBytes)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(data))
	return w
}

// Size is the whole file's size.
func (w *WAV) Size() int64 { return 44 + min(w.s.Size()-w.from, 0xffffffff-36) }

// ReadAt reads the file.
func (w *WAV) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("negative offset")
	}
	if off >= w.Size() {
		return 0, io.EOF
	}
	done := 0
	if off < 44 {
		done = copy(p, w.header[off:])
		p = p[done:]
		off += int64(done)
	}
	if len(p) == 0 {
		return done, nil
	}
	if rest := w.Size() - off; int64(len(p)) > rest {
		p = p[:rest]
	}
	n, err := w.s.ReadAt(p, off-44+w.from)
	if err == io.EOF && done+n > 0 {
		err = nil
	}
	return done + n, err
}

// deemphasisFilter undoes the 50/15 microsecond treble boost some discs were
// mastered with, which a real player does in hardware when a track's flag says
// so. One pole and one zero fitted at 44.1 kHz: within 0.1 dB of the standard
// curve up to 20 kHz (TestDeemphasisCurve).
type deemphasisFilter struct {
	x1, y1 [2]float64
}

const (
	deemphB0 = 0.460681
	deemphB1 = -0.088681
	deemphA1 = -0.628
	// deemphWarmup: 0.63^128 is far below a 16-bit step.
	deemphWarmup = 128
)

func (f *deemphasisFilter) apply(p []byte) {
	for i := 0; i+SampleBytes <= len(p); i += SampleBytes {
		for c := 0; c < 2; c++ {
			at := i + c*2
			x := float64(int16(binary.LittleEndian.Uint16(p[at:])))
			y := deemphB0*x + deemphB1*f.x1[c] - deemphA1*f.y1[c]
			f.x1[c], f.y1[c] = x, y
			v := y
			if v >= 0 {
				v += 0.5
			} else {
				v -= 0.5
			}
			if v > 32767 {
				v = 32767
			} else if v < -32768 {
				v = -32768
			}
			binary.LittleEndian.PutUint16(p[at:], uint16(int16(v)))
		}
	}
}
