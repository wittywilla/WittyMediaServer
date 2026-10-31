package wittycd

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/bits"
	"slices"
	"sort"
	"sync"
)

// A FLAC decoder, for rips kept as a cue sheet and one .flac file, and for the
// audio inside some CHD images. It looks like a plain file of little-endian samples
// that can be read at any offset. Nothing is decoded until it is asked for, and
// only a few frames are kept, so a disc costs a few hundred kilobytes however many
// people are listening.
//
// Finding a frame without reading the whole file: frames carry their own position,
// so the decoder guesses a byte offset, finds the next frame there, reads where it
// is, and closes in. What it learns on the way is kept as a sparse index, added to
// any seek table the file came with. Every seek table entry is checked before it is
// believed, because a file can say anything.

const (
	// flacMaxBlock is the most samples a frame's header can claim.
	flacMaxBlock = 65536
	// flacMaxFrame bounds one frame's bytes. Stored raw, the largest legal frame of
	// CD audio is about 270 kilobytes, so anything longer is damage.
	flacMaxFrame = 1 << 20
	// flacScanLimit bounds how far one search for a frame looks.
	flacScanLimit = 1 << 20
	// flacWindow is how much is read from the file at a time.
	flacWindow = 64 << 10
	// flacCacheFrames is how many decoded frames are kept: a few, so a few listeners
	// playing different parts of a disc don't each start over on every read.
	flacCacheFrames = 8
	// flacMaxPoints bounds the index: about 64 kilobytes at most.
	flacMaxPoints = 4096
	// flacMaxAttempts bounds how many false starts one search will try to decode.
	flacMaxAttempts = 64
	// flacMaxWalk bounds how many frames one read steps through.
	flacMaxWalk = 1 << 16
	// flacMaxHeader is the longest a frame's header can be.
	flacMaxHeader = 16
)

var (
	errFlacBad   = errors.New("this FLAC frame makes no sense")
	errFlacShort = errors.New("this FLAC frame runs past the end of what was read")
	errFlacCRC   = errors.New("this FLAC frame fails its checksum")
)

// flacBits reads bits from the top of each byte down. It is also what CHD's map
// and Huffman data are read with, since they are packed the same way. Reading
// past the end gives zeroes and sets short, so a loop over hostile data ends
// without a check in every step.
type flacBits struct {
	b     []byte
	pos   int
	acc   uint64
	n     uint
	short bool
}

func (r *flacBits) fill() {
	for r.n <= 56 && r.pos < len(r.b) {
		r.acc |= uint64(r.b[r.pos]) << (56 - r.n)
		r.pos++
		r.n += 8
	}
}

// read takes up to 32 bits.
func (r *flacBits) read(n uint) uint32 {
	if n == 0 {
		return 0
	}
	if r.n < n {
		r.fill()
		if r.n < n {
			r.short = true
			r.acc, r.n = 0, 0
			return 0
		}
	}
	v := uint32(r.acc >> (64 - n))
	r.acc <<= n
	r.n -= n
	return v
}

// signed takes up to 32 bits as a two's complement number.
func (r *flacBits) signed(n uint) int32 {
	if n == 0 {
		return 0
	}
	return int32(r.read(n)<<(32-n)) >> (32 - n)
}

// peek looks at up to 32 bits without taking them. Past the end they read as zero.
func (r *flacBits) peek(n uint) uint32 {
	if r.n < n {
		r.fill()
	}
	return uint32(r.acc >> (64 - n))
}

// skip gives up bits already seen with peek.
func (r *flacBits) skip(n uint) {
	if r.n < n {
		r.short = true
		r.acc, r.n = 0, 0
		return
	}
	r.acc <<= n
	r.n -= n
}

// unary counts zero bits up to the next one bit, and takes them all.
func (r *flacBits) unary() uint32 {
	count := uint32(0)
	for {
		if r.acc != 0 {
			z := uint(bits.LeadingZeros64(r.acc))
			r.acc <<= z + 1
			r.n -= z + 1
			return count + uint32(z)
		}
		count += uint32(r.n)
		r.n = 0
		r.fill()
		if r.n == 0 {
			r.short = true
			return count
		}
	}
}

// align drops bits up to the next whole byte.
func (r *flacBits) align() {
	drop := r.n & 7
	r.acc <<= drop
	r.n -= drop
}

// bytePos is how many whole bytes have been taken. Call it after align.
func (r *flacBits) bytePos() int { return r.pos - int(r.n/8) }

// flacHeader is what a frame says about itself.
type flacHeader struct {
	blocks   int // samples in the frame
	number   uint64
	variable bool  // number counts samples rather than frames
	assign   uint8 // 1 is left and right, 8 to 10 are the stereo modes
	size     int   // bytes of header, its checksum included
}

// parse reads a frame header, and is true only for one that passes its checksum
// and could be CD audio. Most of the time something that merely looks like a frame's
// start is rejected right here.
func (h *flacHeader) parse(b []byte) bool {
	if len(b) < 6 || b[0] != 0xff || b[1]&0xfe != 0xf8 {
		return false
	}
	h.variable = b[1]&1 != 0
	sizeCode, rateCode := b[2]>>4, b[2]&15
	h.assign = b[3] >> 4
	depth := b[3] >> 1 & 7
	if sizeCode == 0 || rateCode == 15 || b[3]&1 != 0 {
		return false
	}
	if h.assign != 1 && (h.assign < 8 || h.assign > 10) {
		return false // not two channels
	}
	if depth != 0 && depth != 4 {
		return false // not 16 bits
	}
	at := 4
	// The number, in an extended form of UTF-8 that reaches 36 bits.
	first := b[at]
	at++
	extra := 0
	var num uint64
	switch {
	case first < 0x80:
		num = uint64(first)
	case first&0xe0 == 0xc0:
		num, extra = uint64(first&0x1f), 1
	case first&0xf0 == 0xe0:
		num, extra = uint64(first&0x0f), 2
	case first&0xf8 == 0xf0:
		num, extra = uint64(first&0x07), 3
	case first&0xfc == 0xf8:
		num, extra = uint64(first&0x03), 4
	case first&0xfe == 0xfc:
		num, extra = uint64(first&0x01), 5
	case first == 0xfe:
		num, extra = 0, 6
	default:
		return false
	}
	if len(b) < at+extra {
		return false
	}
	for range extra {
		c := b[at]
		at++
		if c&0xc0 != 0x80 {
			return false
		}
		num = num<<6 | uint64(c&0x3f)
	}
	h.number = num
	switch {
	case sizeCode == 1:
		h.blocks = 192
	case sizeCode <= 5:
		h.blocks = 576 << (sizeCode - 2)
	case sizeCode == 6:
		if len(b) < at+1 {
			return false
		}
		h.blocks = int(b[at]) + 1
		at++
	case sizeCode == 7:
		if len(b) < at+2 {
			return false
		}
		h.blocks = int(binary.BigEndian.Uint16(b[at:])) + 1
		at += 2
	default:
		h.blocks = 256 << (sizeCode - 8)
	}
	switch rateCode {
	case 0, 9: // as STREAMINFO says, or 44.1 kHz
	case 13, 14:
		if len(b) < at+2 {
			return false
		}
		v := binary.BigEndian.Uint16(b[at:])
		at += 2
		if rateCode == 13 && v != SampleRate || rateCode == 14 && v != SampleRate/10 {
			return false
		}
	default:
		return false
	}
	if len(b) < at+1 || crc8(b[:at]) != b[at] {
		return false
	}
	h.size = at + 1
	return true
}

// flacScratch holds a frame's samples while it is decoded, and is used again for
// the next one. After a decode, l and r are the left and right channels.
type flacScratch struct {
	l, r []int32
}

// decodeFlacFrame decodes the frame at the start of b, and says how many bytes it
// took. errFlacCRC still gives the frame's size; errFlacShort means more of the
// file is needed.
func decodeFlacFrame(b []byte, sc *flacScratch) (flacHeader, int, error) {
	var h flacHeader
	if !h.parse(b) {
		return h, 0, errFlacBad
	}
	if cap(sc.l) < h.blocks {
		sc.l = make([]int32, max(h.blocks, flacBlock))
		sc.r = make([]int32, max(h.blocks, flacBlock))
	}
	l, r := sc.l[:h.blocks], sc.r[:h.blocks]
	depthL, depthR := uint(16), uint(16)
	switch h.assign {
	case 8, 10:
		depthR = 17 // the difference of two channels needs one more bit
	case 9:
		depthL = 17
	}
	br := flacBits{b: b[h.size:]}
	err := br.subframe(l, depthL)
	if err == nil {
		err = br.subframe(r, depthR)
	}
	if br.short {
		return h, 0, errFlacShort
	}
	if err != nil {
		return h, 0, err
	}
	switch h.assign {
	case 8: // left, side
		for i := range r {
			r[i] = l[i] - r[i]
		}
	case 9: // side, right
		for i := range l {
			l[i] += r[i]
		}
	case 10: // mid, side
		for i := range l {
			m := l[i]<<1 | r[i]&1
			s := r[i]
			l[i] = (m + s) >> 1
			r[i] = (m - s) >> 1
		}
	}
	br.align()
	end := h.size + br.bytePos()
	if end+2 > len(b) {
		return h, 0, errFlacShort
	}
	if crc16(b[:end]) != binary.BigEndian.Uint16(b[end:]) {
		return h, end + 2, errFlacCRC
	}
	return h, end + 2, nil
}

// subframe decodes one channel into x, which is a whole block.
func (r *flacBits) subframe(x []int32, depth uint) error {
	head := r.read(8)
	if head&0x80 != 0 {
		return errFlacBad
	}
	kind := head >> 1 & 0x3f
	wasted := uint(0)
	if head&1 != 0 {
		// Samples all ending in zero bits are stored without them.
		wasted = uint(r.unary()) + 1
		if wasted >= depth {
			return errFlacBad
		}
		depth -= wasted
	}
	switch {
	case kind == 0: // constant
		v := r.signed(depth)
		for i := range x {
			x[i] = v
		}
	case kind == 1: // stored as it is
		for i := range x {
			x[i] = r.signed(depth)
		}
	case kind >= 8 && kind <= 12: // fixed predictor
		order := int(kind - 8)
		if order > len(x) {
			return errFlacBad
		}
		for i := range order {
			x[i] = r.signed(depth)
		}
		if err := r.residual(x, order); err != nil {
			return err
		}
		fixedPredict(x, order)
	case kind >= 32: // a filter the encoder worked out
		order := int(kind - 31)
		if order > len(x) {
			return errFlacBad
		}
		for i := range order {
			x[i] = r.signed(depth)
		}
		precision := r.read(4) + 1
		if precision == 16 {
			return errFlacBad
		}
		shift := r.signed(5)
		if shift < 0 {
			return errFlacBad
		}
		var coef [32]int32
		for i := range order {
			coef[i] = r.signed(uint(precision))
		}
		if err := r.residual(x, order); err != nil {
			return err
		}
		lpcPredict(x, coef[:order], uint(shift))
	default:
		return errFlacBad
	}
	if wasted > 0 {
		for i := range x {
			x[i] <<= wasted
		}
	}
	return nil
}

// residual reads what the predictor left over into x[order:].
func (r *flacBits) residual(x []int32, order int) error {
	method := r.read(2)
	if method > 1 {
		return errFlacBad
	}
	paramBits, escape := uint(4), uint32(15)
	if method == 1 {
		paramBits, escape = 5, 31
	}
	part := r.read(4)
	size := len(x) >> part
	if size<<part != len(x) || size < order {
		return errFlacBad
	}
	i := order
	for p := range 1 << part {
		end := (p + 1) * size
		k := r.read(paramBits)
		if k == escape {
			width := uint(r.read(5))
			for ; i < end; i++ {
				x[i] = r.signed(width)
			}
			continue
		}
		for ; i < end; i++ {
			u := r.unary()<<k | r.read(uint(k))
			x[i] = int32(u>>1) ^ -int32(u&1)
		}
		if r.short {
			return nil
		}
	}
	return nil
}

func fixedPredict(x []int32, order int) {
	switch order {
	case 1:
		for i := 1; i < len(x); i++ {
			x[i] += x[i-1]
		}
	case 2:
		for i := 2; i < len(x); i++ {
			x[i] += 2*x[i-1] - x[i-2]
		}
	case 3:
		for i := 3; i < len(x); i++ {
			x[i] += 3*x[i-1] - 3*x[i-2] + x[i-3]
		}
	case 4:
		for i := 4; i < len(x); i++ {
			x[i] += 4*x[i-1] - 6*x[i-2] + 4*x[i-3] - x[i-4]
		}
	}
}

func lpcPredict(x, coef []int32, shift uint) {
	order := len(coef)
	for i := order; i < len(x); i++ {
		var sum int64
		h := x[i-order : i]
		for j, c := range coef {
			sum += int64(c) * int64(h[order-1-j])
		}
		x[i] += int32(sum >> shift)
	}
}

// flacPoint is a place in the file known, or claimed, to start a frame.
type flacPoint struct {
	sample, off int64
	checked     bool
}

// flacFrame is a decoded frame that was kept. A damaged one has no samples and
// plays as silence for its length.
type flacFrame struct {
	sample int64
	blocks int
	end    int64 // where the next frame should start
	silent bool
	pcm    []byte
	used   uint64
}

// flacSpot is a frame just decoded, whose samples are still in the scratch space.
type flacSpot struct {
	sample int64
	blocks int
	off    int64
	end    int64
	silent bool
}

type flacSource struct {
	r    io.ReaderAt
	size int64

	first      int64 // the first frame
	total      int64 // samples
	variable   bool
	fixedBlock int64
	base       int64 // the first frame's own idea of its position

	mu      sync.Mutex
	points  []flacPoint
	spacing int64
	frames  [flacCacheFrames]flacFrame
	clock   uint64
	sc      flacScratch
	win     []byte
	winOff  int64
	hint    int // the largest frame seen, plus some
	ioErr   error
}

// newFlacSource opens a FLAC file as a virtual file of CD samples, and gives its
// length in bytes.
func newFlacSource(r io.ReaderAt, size int64) (io.ReaderAt, int64, error) {
	s := &flacSource{r: r, size: size, hint: 32 << 10}
	if err := s.readMetadata(); err != nil {
		return nil, 0, err
	}
	if err := s.findEnd(); err != nil {
		return nil, 0, err
	}
	s.spacing = max(s.total/flacMaxPoints, 1)
	return s, s.total * SampleBytes, nil
}

// readMetadata reads the blocks before the first frame: STREAMINFO says what the
// audio is, and a seek table gives places to start from.
func (s *flacSource) readMetadata() error {
	var head [4]byte
	if _, err := s.r.ReadAt(head[:], 0); err != nil || string(head[:]) != "fLaC" {
		return errors.New("this file isn't really FLAC")
	}
	pos := int64(4)
	var seek []flacPoint
	seen := false
	minBlock, maxBlock := 0, 0
	for steps := 0; ; steps++ {
		var h [4]byte
		if steps >= 4096 {
			return errors.New("this FLAC file has far too many metadata blocks")
		}
		if _, err := s.r.ReadAt(h[:], pos); err != nil {
			return fmt.Errorf("this FLAC file's metadata is cut short: %w", err)
		}
		kind := h[0] & 0x7f
		n := int64(h[1])<<16 | int64(h[2])<<8 | int64(h[3])
		pos += 4
		if pos+n > s.size {
			return errors.New("this FLAC file's metadata is cut short")
		}
		switch {
		case kind == 0 && !seen:
			if n < 34 {
				return errors.New("this FLAC file's description of itself is cut short")
			}
			var b [34]byte
			if _, err := s.r.ReadAt(b[:], pos); err != nil {
				return fmt.Errorf("this FLAC file's description of itself can't be read: %w", err)
			}
			minBlock = int(binary.BigEndian.Uint16(b[0:]))
			maxBlock = int(binary.BigEndian.Uint16(b[2:]))
			v := binary.BigEndian.Uint64(b[10:])
			rate := v >> 44
			channels := v>>41&7 + 1
			depth := v>>36&31 + 1
			if rate != SampleRate || channels != 2 || depth != 16 {
				return fmt.Errorf("%w: its FLAC file isn't CD audio (%d channels, %d Hz, %d bits)", ErrUnsupported, channels, rate, depth)
			}
			s.total = int64(v & (1<<36 - 1))
			if s.total > MaxSectors*SectorSamples {
				return fmt.Errorf("%w: this FLAC file says it is longer than a CD can be", ErrNotCD)
			}
			seen = true
		case kind == 3 && seek == nil:
			seek = s.readSeekTable(pos, n)
		}
		pos += n
		if !seen {
			return errors.New("this FLAC file doesn't start with a description of itself")
		}
		if h[0]&0x80 != 0 {
			break
		}
	}
	s.first = pos
	return s.findFirst(seek, minBlock, maxBlock)
}

// readSeekTable keeps the entries that could be true. Their offsets count from the
// first frame, which isn't known yet, and whether they are right is only found out
// when one is used.
func (s *flacSource) readSeekTable(pos, n int64) []flacPoint {
	n = min(n, flacMaxPoints*18)
	b := make([]byte, n-n%18)
	if _, err := s.r.ReadAt(b, pos); err != nil {
		return nil
	}
	out := make([]flacPoint, 0, len(b)/18)
	for i := 0; i+18 <= len(b); i += 18 {
		sample, off := binary.BigEndian.Uint64(b[i:]), binary.BigEndian.Uint64(b[i+8:])
		if sample == ^uint64(0) || sample > MaxSectors*SectorSamples || off >= uint64(s.size) {
			continue
		}
		if m := len(out); m > 0 && (int64(sample) <= out[m-1].sample || int64(off) <= out[m-1].off) {
			continue
		}
		out = append(out, flacPoint{sample: int64(sample), off: int64(off)})
	}
	return out
}

// findFirst finds the first frame that is intact, which settles how the stream
// counts its position.
func (s *flacSource) findFirst(seek []flacPoint, minBlock, maxBlock int) error {
	at := s.first
	limit := min(s.size, at+flacScanLimit)
	attempts := 0
	for at < limit && attempts < flacMaxAttempts {
		b := s.bytes(at, flacMaxHeader)
		if len(b) == 0 {
			break
		}
		i := bytes.IndexByte(b, 0xff)
		if i < 0 {
			at += int64(len(b))
			continue
		}
		at += int64(i)
		var h flacHeader
		if !h.parse(s.bytes(at, flacMaxHeader)) {
			at++
			continue
		}
		attempts++
		h, n, err := s.decodeAt(at)
		if err != nil {
			at++
			continue
		}
		s.variable = h.variable
		s.fixedBlock = int64(h.blocks)
		if minBlock == maxBlock && minBlock >= 16 {
			s.fixedBlock = int64(minBlock)
		}
		// The first frame's number is normally 0. A stream cut from a longer one
		// keeps its old numbers, and its samples count from there. When the first
		// frame was damaged, what is found later is a real position.
		if at == s.first {
			s.base = s.position(&h)
		}
		s.hint = max(s.hint, n+n/2)
		s.points = append(s.points[:0], flacPoint{sample: s.position(&h) - s.base, off: at, checked: true})
		for _, p := range seek {
			p.off += s.first
			p.sample -= s.base
			last := &s.points[len(s.points)-1]
			if p.sample > last.sample && p.off > last.off && p.off < s.size && len(s.points) < flacMaxPoints {
				s.points = append(s.points, p)
			}
		}
		return nil
	}
	if s.ioErr != nil {
		return s.ioErr
	}
	return errors.New("this FLAC file has no audio in it that can be read")
}

// position is where a frame says it is, in samples, before the stream's own start
// is taken off.
func (s *flacSource) position(h *flacHeader) int64 {
	if h.variable {
		return int64(h.number)
	}
	return int64(h.number) * s.fixedBlock
}

// sampleOf is where a frame is in this stream, or -1 if it can't belong to it.
func (s *flacSource) sampleOf(h *flacHeader) int64 {
	if h.variable != s.variable {
		return -1
	}
	v := s.position(h) - s.base
	if v < 0 || v >= MaxSectors*SectorSamples {
		return -1
	}
	return v
}

// findEnd settles the length. STREAMINFO may leave it blank, as a stream written
// to a pipe has to, and a file cut short still claims the length it had. Only
// the last frame tells, and finding it means closing in from both ends.
func (s *flacSource) findEnd() error {
	lo, ok := s.frameNear(s.points[0].off)
	if !ok {
		return errors.New("this FLAC file has no audio in it that can be read")
	}
	hi := s.size
	at := max(lo.off+1, s.size-256<<10)
	for i := 0; i < 48 && hi-lo.off > 256<<10; i++ {
		f, ok := s.findFrame(at, min(hi, at+flacScanLimit), lo.sample+1)
		if ok {
			lo = f
		} else {
			hi = at
		}
		at = lo.off + (hi-lo.off)/2
	}
	for range flacMaxWalk {
		f, ok := s.next(lo)
		if !ok {
			break
		}
		lo = f
	}
	if s.ioErr != nil {
		return s.ioErr
	}
	last := lo.sample + int64(lo.blocks)
	switch {
	case s.total == 0:
		s.total = last
	case last+2*int64(lo.blocks) < s.total:
		// More missing than a damaged frame or two: the file has been cut short.
		s.total = last
	}
	s.total = min(s.total, MaxSectors*SectorSamples)
	if s.total <= 0 {
		return errors.New("this FLAC file has no audio in it that can be read")
	}
	return nil
}

// bytes returns what is known of the file from off on: at least want bytes unless
// the file ends sooner.
func (s *flacSource) bytes(off int64, want int) []byte {
	if off >= s.size || off < 0 {
		return nil
	}
	if off >= s.winOff && off+int64(want) <= s.winOff+int64(len(s.win)) {
		return s.win[off-s.winOff:]
	}
	if off >= s.winOff && off < s.winOff+int64(len(s.win)) && s.winOff+int64(len(s.win)) == s.size {
		return s.win[off-s.winOff:]
	}
	n := int(min(int64(max(want, flacWindow)), s.size-off))
	if cap(s.win) < n {
		s.win = make([]byte, n)
	}
	s.win = s.win[:n]
	got, err := s.r.ReadAt(s.win, off)
	if err != nil && err != io.EOF {
		s.ioErr = err
	}
	s.win = s.win[:got]
	s.winOff = off
	return s.win
}

// decodeAt decodes the frame at off into the scratch space, reading more of the
// file if the first try runs out.
func (s *flacSource) decodeAt(off int64) (flacHeader, int, error) {
	want := s.hint
	for {
		b := s.bytes(off, want)
		h, n, err := decodeFlacFrame(b, &s.sc)
		if err != errFlacShort {
			if err == nil && n+n/2 > s.hint {
				s.hint = min(n+n/2, flacMaxFrame)
			}
			return h, n, err
		}
		if want >= flacMaxFrame || off+int64(len(b)) >= s.size {
			return h, n, err
		}
		want = min(max(want, len(b))*2, flacMaxFrame)
	}
}

// spotAt decodes the frame at off, damaged or not, and is false when no frame of
// this stream starts there.
func (s *flacSource) spotAt(off int64) (flacSpot, bool) {
	var h flacHeader
	if !h.parse(s.bytes(off, flacMaxHeader)) {
		return flacSpot{}, false
	}
	sample := s.sampleOf(&h)
	if sample < 0 {
		return flacSpot{}, false
	}
	_, n, err := s.decodeAt(off)
	f := flacSpot{sample: sample, blocks: h.blocks, off: off, end: off + int64(n)}
	if err != nil {
		// Damage can lead the decoder past the frame's real end, so the next frame is
		// looked for from just after this one's header.
		f.silent = true
		f.end = off + int64(h.size)
	}
	return f, true
}

// frameNear is the frame at off or the first good one after it.
func (s *flacSource) frameNear(off int64) (flacSpot, bool) {
	if f, ok := s.spotAt(off); ok {
		return f, true
	}
	return s.findFrame(off, min(s.size, off+flacScanLimit), 0)
}

// findFrame looks in [from, limit) for a frame that passes its checksums and is at
// or past a sample. Something can look like a frame by chance inside another
// frame's data, and the checksum over the whole frame is what rules that out.
func (s *flacSource) findFrame(from, limit, after int64) (flacSpot, bool) {
	attempts := 0
	for at := from; at < limit; {
		b := s.bytes(at, flacMaxHeader)
		if len(b) == 0 {
			break
		}
		i := bytes.IndexByte(b, 0xff)
		if i < 0 {
			at += int64(len(b))
			continue
		}
		at += int64(i)
		if at >= limit {
			break
		}
		var h flacHeader
		if h.parse(s.bytes(at, flacMaxHeader)) {
			if sample := s.sampleOf(&h); sample >= after {
				attempts++
				if attempts > flacMaxAttempts {
					break
				}
				if _, n, err := s.decodeAt(at); err == nil {
					return flacSpot{sample: sample, blocks: h.blocks, off: at, end: at + int64(n)}, true
				}
			}
		}
		at++
	}
	return flacSpot{}, false
}

// next finds the frame after this one. Usually it starts right where this one
// stopped, and is taken even if it is damaged, because its header says how much
// silence is needed. Otherwise it is searched for.
func (s *flacSource) next(f flacSpot) (flacSpot, bool) {
	after := f.sample + int64(f.blocks)
	if !f.silent {
		if n, ok := s.spotAt(f.end); ok && n.sample == after {
			return n, true
		}
	}
	return s.findFrame(f.end, min(s.size, f.end+flacScanLimit), f.sample+1)
}

// keep stores the frame in the scratch space among the kept ones, in place of the
// one unused the longest. It must come straight after that frame's decode.
func (s *flacSource) keep(f flacSpot) *flacFrame {
	slot := &s.frames[0]
	for i := range s.frames {
		if s.frames[i].used < slot.used {
			slot = &s.frames[i]
		}
	}
	slot.sample, slot.blocks, slot.end, slot.silent = f.sample, f.blocks, f.end, f.silent
	s.clock++
	slot.used = s.clock
	if f.silent {
		return slot
	}
	n := f.blocks * SampleBytes
	if cap(slot.pcm) < n {
		slot.pcm = make([]byte, n)
	}
	slot.pcm = slot.pcm[:n]
	l, r := s.sc.l[:f.blocks], s.sc.r[:f.blocks]
	for i := range l {
		binary.LittleEndian.PutUint16(slot.pcm[i*4:], uint16(l[i]))
		binary.LittleEndian.PutUint16(slot.pcm[i*4+2:], uint16(r[i]))
	}
	return slot
}

// learn adds a frame to the sparse index if it isn't close to one already there.
// Spacing the entries evenly is what keeps the index small and useful.
func (s *flacSource) learn(sample, off int64) {
	i := sort.Search(len(s.points), func(i int) bool { return s.points[i].sample >= sample })
	if i < len(s.points) && (s.points[i].sample-sample < s.spacing || off >= s.points[i].off) {
		return
	}
	if i > 0 && (sample-s.points[i-1].sample < s.spacing || off <= s.points[i-1].off) {
		return
	}
	if len(s.points) >= flacMaxPoints {
		return
	}
	s.points = slices.Insert(s.points, i, flacPoint{sample: sample, off: off, checked: true})
}

// check tests an entry that came from the file's seek table, once, and removes it
// if there is no such frame there. It says whether the entry stayed.
func (s *flacSource) check(i int) bool {
	p := &s.points[i]
	if p.checked {
		return true
	}
	var h flacHeader
	if h.parse(s.bytes(p.off, flacMaxHeader)) && s.sampleOf(&h) == p.sample {
		p.checked = true
		return true
	}
	s.points = slices.Delete(s.points, i, i+1)
	return false
}

// narrow finds a frame at or before a sample, close enough to step forward from.
// It returns false if the sample comes before every frame in the file.
func (s *flacSource) narrow(sample int64) (flacPoint, bool) {
	var lo flacPoint
	hiOff, hiSample := s.size, s.total
	for {
		i := sort.Search(len(s.points), func(i int) bool { return s.points[i].sample > sample })
		if i == 0 {
			return flacPoint{}, false
		}
		if !s.check(i - 1) {
			continue
		}
		if i < len(s.points) && !s.check(i) {
			continue
		}
		lo = s.points[i-1]
		if i < len(s.points) {
			hiOff, hiSample = s.points[i].off, s.points[i].sample
		}
		break
	}
	for step := 0; step < 48 && hiOff-lo.off > flacWindow; step++ {
		// A guess from the average rate is usually close, and halving is the fallback
		// for a file that isn't even, so they take turns.
		var mid int64
		if step&1 == 0 && hiSample > lo.sample {
			part := float64(sample-lo.sample) / float64(hiSample-lo.sample)
			mid = lo.off + int64(part*float64(hiOff-lo.off))
		} else {
			mid = lo.off + (hiOff-lo.off)/2
		}
		mid = max(lo.off+1, min(mid, hiOff-1))
		f, ok := s.findFrame(mid, min(hiOff, mid+flacScanLimit), lo.sample+1)
		switch {
		case !ok || f.sample >= hiSample:
			hiOff = mid // nothing useful from there up, and the next try looks lower
		case f.sample <= sample:
			lo = flacPoint{sample: f.sample, off: f.off, checked: true}
			s.learn(f.sample, f.off)
			if sample < f.sample+int64(f.blocks) {
				return lo, true
			}
		default:
			hiOff, hiSample = f.off, f.sample
			s.learn(f.sample, f.off)
		}
	}
	return lo, true
}

// nextKnown is the first frame known to come after a sample, or the end.
func (s *flacSource) nextKnown(sample int64) int64 {
	i := sort.Search(len(s.points), func(i int) bool { return s.points[i].sample > sample })
	if i < len(s.points) {
		return s.points[i].sample
	}
	return s.total
}

// frameFor gives the frame that holds a sample. With no such frame, because of
// damage, it gives nil and the sample where there may be one again.
func (s *flacSource) frameFor(sample int64) (*flacFrame, int64) {
	for i := range s.frames {
		f := &s.frames[i]
		if f.blocks > 0 && sample >= f.sample && sample < f.sample+int64(f.blocks) {
			s.clock++
			f.used = s.clock
			return f, 0
		}
	}
	// The usual case by far: the frame after one just played.
	for i := range s.frames {
		f := &s.frames[i]
		if f.blocks > 0 && f.sample+int64(f.blocks) == sample {
			from := flacSpot{sample: f.sample, blocks: f.blocks, end: f.end, silent: f.silent}
			if n, ok := s.next(from); ok {
				return s.walk(n, sample)
			}
			return nil, s.holeEnd(sample)
		}
	}
	p, ok := s.narrow(sample)
	if !ok {
		return nil, s.nextKnown(sample)
	}
	f, ok := s.frameNear(p.off)
	if !ok {
		return nil, s.holeEnd(sample)
	}
	return s.walk(f, sample)
}

// holeEnd says how far silence goes when no frame could be found. It doesn't go
// far, because a frame may be just beyond where the search stopped.
func (s *flacSource) holeEnd(sample int64) int64 {
	return min(s.nextKnown(sample), sample+flacMaxBlock)
}

// walk steps forward frame by frame to the one holding a sample.
func (s *flacSource) walk(f flacSpot, sample int64) (*flacFrame, int64) {
	for range flacMaxWalk {
		if f.sample > sample {
			return nil, f.sample // the frames that should be here are lost
		}
		if sample < f.sample+int64(f.blocks) {
			return s.keep(f), 0
		}
		s.learn(f.sample, f.off)
		n, ok := s.next(f)
		if !ok {
			return nil, s.holeEnd(sample)
		}
		f = n
	}
	return nil, s.holeEnd(sample)
}

// ReadAt reads samples as little-endian bytes. Any number of listeners can read at
// once. What damage has taken reads as silence. Only a real failure to read the
// file comes back as an error.
func (s *flacSource) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("negative offset")
	}
	length := s.total * SampleBytes
	if off >= length {
		return 0, io.EOF
	}
	short := false
	if rest := length - off; int64(len(p)) > rest {
		p, short = p[:rest], true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	done := 0
	for len(p) > 0 {
		sample := off / SampleBytes
		f, until := s.frameFor(sample)
		if s.ioErr != nil {
			err := s.ioErr
			s.ioErr = nil
			return done, err
		}
		var n int
		if f == nil {
			until = max(until, sample+1)
			n = int(min(int64(len(p)), until*SampleBytes-off))
			clear(p[:n])
		} else {
			from := (sample-f.sample)*SampleBytes + off%SampleBytes
			n = int(min(int64(len(p)), int64(f.blocks)*SampleBytes-from))
			if f.silent {
				clear(p[:n])
			} else {
				copy(p[:n], f.pcm[from:])
			}
		}
		p = p[n:]
		off += int64(n)
		done += n
	}
	if short {
		return done, io.EOF
	}
	return done, nil
}
