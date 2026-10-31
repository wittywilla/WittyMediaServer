package wittycd

import (
	"encoding/binary"
	"io"
)

// A small FLAC encoder written for one job: CD audio, live, on weak hardware. It
// uses FLAC's fixed predictors rather than searching for the best filter, so it
// runs about 200 times faster than real time on a desktop and still takes most discs to
// about 60 to 70 percent of their size. Frames are fixed at 4096 samples.

const flacBlock = 4096

// WriteFLAC writes the program from a sample onwards as a FLAC stream. It stops
// at the first write error, which is how a listener leaving ends it.
func (s *Stream) WriteFLAC(w io.Writer, startSample int64) error {
	startSample = max(0, min(startSample, s.total))
	total := s.total - startSample
	var head [42]byte
	copy(head[:], "fLaC")
	head[4] = 0x80 // last metadata block: STREAMINFO
	head[7] = 34
	binary.BigEndian.PutUint16(head[8:], flacBlock)
	binary.BigEndian.PutUint16(head[10:], flacBlock)
	// 20 bits of rate, 3 of channels-1, 5 of bits-1, 36 of samples; frame sizes and
	// the checksum stay zero, which means "not known".
	v := uint64(SampleRate)<<44 | uint64(1)<<41 | uint64(15)<<36 | uint64(total)&(1<<36-1)
	binary.BigEndian.PutUint64(head[18:], v)
	if _, err := w.Write(head[:]); err != nil {
		return err
	}
	e := newFlacEncoder()
	pcm := make([]byte, flacBlock*SampleBytes)
	frame := uint64(0)
	for at := startSample; at < s.total; at += flacBlock {
		n := int(min(flacBlock, s.total-at))
		if err := s.readSamples(pcm[:n*SampleBytes], at*SampleBytes); err != nil {
			return err
		}
		if _, err := w.Write(e.encodeFrame(pcm[:n*SampleBytes], frame)); err != nil {
			return err
		}
		frame++
	}
	return nil
}

type flacEncoder struct {
	l, r, m, s []int32
	res        [5][]int32
	bits       bitWriter
}

func newFlacEncoder() *flacEncoder {
	e := &flacEncoder{l: make([]int32, flacBlock), r: make([]int32, flacBlock), m: make([]int32, flacBlock), s: make([]int32, flacBlock)}
	for i := range e.res {
		e.res[i] = make([]int32, flacBlock)
	}
	e.bits.b = make([]byte, 0, flacBlock*SampleBytes+64)
	return e
}

type bitWriter struct {
	b   []byte
	acc uint64
	n   uint
}

func (w *bitWriter) put(v uint64, bits uint) {
	if bits > 32 {
		w.put(v>>32, bits-32)
		v &= 1<<32 - 1
		bits = 32
	}
	w.acc = w.acc<<bits | v&(1<<bits-1)
	w.n += bits
	for w.n >= 8 {
		w.n -= 8
		w.b = append(w.b, byte(w.acc>>w.n))
	}
}

func (w *bitWriter) align() {
	if w.n > 0 {
		w.put(0, 8-w.n)
	}
}

// encodeFrame turns samples into one frame. The result is valid until the next call.
func (e *flacEncoder) encodeFrame(pcm []byte, number uint64) []byte {
	n := len(pcm) / SampleBytes
	for i := range n {
		l := int32(int16(binary.LittleEndian.Uint16(pcm[i*4:])))
		r := int32(int16(binary.LittleEndian.Uint16(pcm[i*4+2:])))
		e.l[i], e.r[i] = l, r
		e.m[i], e.s[i] = (l+r)>>1, l-r
	}
	cost := func(x []int32) (uint64, int) {
		return fixedCost(x[:n])
	}
	lc, lo := cost(e.l)
	rc, ro := cost(e.r)
	mc, mo := cost(e.m)
	sc, so := cost(e.s)
	// Channel assignment: which two of the four are cheapest.
	assign, a, ao, ab, b, bo, bb := uint64(1), e.l, lo, uint(16), e.r, ro, uint(16)
	best := lc + rc
	if c := lc + sc; c < best {
		best, assign, a, ao, ab, b, bo, bb = c, 8, e.l, lo, 16, e.s, so, 17
	}
	if c := rc + sc; c < best {
		best, assign, a, ao, ab, b, bo, bb = c, 9, e.s, so, 17, e.r, ro, 16
	}
	if c := mc + sc; c < best {
		assign, a, ao, ab, b, bo, bb = 10, e.m, mo, 16, e.s, so, 17
	}

	w := &e.bits
	w.b, w.acc, w.n = w.b[:0], 0, 0
	w.put(0xfff8, 16) // sync, fixed block size
	if n == flacBlock {
		w.put(0xc9, 8) // 4096 samples, 44.1 kHz
	} else {
		w.put(0x79, 8) // the size follows the frame number
	}
	w.put(assign<<4|4<<1, 8) // 16 bits per sample
	putUTF8(w, number)
	if n != flacBlock {
		w.put(uint64(n-1), 16)
	}
	w.put(uint64(crc8(w.b)), 8)
	e.subframe(a[:n], ao, ab)
	e.subframe(b[:n], bo, bb)
	w.align()
	crc := crc16(w.b)
	w.b = append(w.b, byte(crc>>8), byte(crc))
	return w.b
}

// fixedCost picks the fixed predictor that leaves the least to code, and says
// roughly what that will cost in bits.
func fixedCost(x []int32) (uint64, int) {
	var sums [5]uint64
	var d1, d2, d3 int32
	if len(x) > 4 {
		d1 = x[3] - x[2]
		d2 = d1 - (x[2] - x[1])
		d3 = d2 - ((x[2] - x[1]) - (x[1] - x[0]))
	}
	for i := 4; i < len(x); i++ {
		v := x[i]
		e1 := v - x[i-1]
		e2 := e1 - d1
		e3 := e2 - d2
		e4 := e3 - d3
		sums[0] += abs64(v)
		sums[1] += abs64(e1)
		sums[2] += abs64(e2)
		sums[3] += abs64(e3)
		sums[4] += abs64(e4)
		d1, d2, d3 = e1, e2, e3
	}
	order, low := 0, sums[0]
	for i := 1; i < 5; i++ {
		if sums[i] < low {
			order, low = i, sums[i]
		}
	}
	// About log2(mean)+1.5 bits for each sample.
	m := low / uint64(max(len(x), 1))
	bits := uint64(2)
	for m > 0 {
		bits++
		m >>= 1
	}
	return bits * uint64(len(x)), order
}

func abs64(v int32) uint64 {
	if v < 0 {
		return uint64(-int64(v))
	}
	return uint64(v)
}

func (e *flacEncoder) subframe(x []int32, order int, bps uint) {
	w := &e.bits
	same := true
	for _, v := range x[1:] {
		if v != x[0] {
			same = false
			break
		}
	}
	if same {
		w.put(0, 8) // constant
		w.put(uint64(uint32(x[0])), bps)
		return
	}
	if len(x) <= order {
		order = 0
	}
	if len(x) <= 4 { // too short to predict: stored as it is
		w.put(1<<1, 8)
		for _, v := range x {
			w.put(uint64(uint32(v)), bps)
		}
		return
	}
	w.put(uint64(8|order)<<1, 8)
	for _, v := range x[:order] {
		w.put(uint64(uint32(v)), bps)
	}
	res := e.res[0][:len(x)-order]
	switch order {
	case 0:
		copy(res, x)
	case 1:
		for i := range res {
			res[i] = x[i+1] - x[i]
		}
	case 2:
		for i := range res {
			res[i] = x[i+2] - 2*x[i+1] + x[i]
		}
	case 3:
		for i := range res {
			res[i] = x[i+3] - 3*x[i+2] + 3*x[i+1] - x[i]
		}
	case 4:
		for i := range res {
			res[i] = x[i+4] - 4*x[i+3] + 6*x[i+2] - 4*x[i+1] + x[i]
		}
	}
	e.rice(res, len(x), order)
}

// rice codes the residual in up to 16 partitions, choosing how many by what they
// would cost.
func (e *flacEncoder) rice(res []int32, block, order int) {
	const maxP = 4
	w := &e.bits
	partOK := func(p uint) bool { return block%(1<<p) == 0 && block>>p > order }
	// Sums over the smallest partitions, added up for the larger ones.
	var sums [1 << maxP]uint64
	top := uint(0)
	for p := uint(maxP); p > 0; p-- {
		if partOK(p) {
			top = p
			break
		}
	}
	parts := 1 << top
	at := 0
	for i := range parts {
		end := (i + 1) * (block >> top)
		end -= order
		var sum uint64
		for _, v := range res[at:end] {
			sum += uint64(uint32(v<<1 ^ v>>31))
		}
		sums[i] = sum
		at = end
	}
	bestP, bestBits := top, ^uint64(0)
	var params, bestParams [1 << maxP]uint8
	for p := top; ; p-- {
		count := 1 << p
		step := 1 << (top - p)
		total := uint64(0)
		for i := range count {
			var sum uint64
			for j := range step {
				sum += sums[i*step+j]
			}
			n := block >> p
			if i == 0 {
				n -= order
			}
			k := uint8(0)
			for k < 14 && uint64(n)<<(k+1) < sum {
				k++
			}
			params[i] = k
			total += 4 + uint64(n)*(uint64(k)+1) + sum>>k
		}
		if total < bestBits {
			bestBits, bestP, bestParams = total, p, params
		}
		if p == 0 {
			break
		}
	}
	w.put(0, 2)
	w.put(uint64(bestP), 4)
	at = 0
	for i := range 1 << bestP {
		end := (i+1)*(block>>bestP) - order
		k := uint(bestParams[i])
		w.put(uint64(k), 4)
		for _, v := range res[at:end] {
			u := uint64(uint32(v<<1 ^ v>>31))
			q := u >> k
			for q >= 32 {
				w.put(0, 32)
				q -= 32
			}
			w.put(1, uint(q)+1)
			if k > 0 {
				w.put(u, k)
			}
		}
		at = end
	}
}

// putUTF8 writes a frame number in FLAC's extended UTF-8.
func putUTF8(w *bitWriter, v uint64) {
	switch {
	case v < 0x80:
		w.put(v, 8)
	case v < 0x800:
		w.put(0xc0|v>>6, 8)
		w.put(0x80|v&0x3f, 8)
	case v < 0x10000:
		w.put(0xe0|v>>12, 8)
		w.put(0x80|v>>6&0x3f, 8)
		w.put(0x80|v&0x3f, 8)
	case v < 0x200000:
		w.put(0xf0|v>>18, 8)
		w.put(0x80|v>>12&0x3f, 8)
		w.put(0x80|v>>6&0x3f, 8)
		w.put(0x80|v&0x3f, 8)
	case v < 0x4000000:
		w.put(0xf8|v>>24, 8)
		w.put(0x80|v>>18&0x3f, 8)
		w.put(0x80|v>>12&0x3f, 8)
		w.put(0x80|v>>6&0x3f, 8)
		w.put(0x80|v&0x3f, 8)
	default:
		w.put(0xfc|v>>30, 8)
		w.put(0x80|v>>24&0x3f, 8)
		w.put(0x80|v>>18&0x3f, 8)
		w.put(0x80|v>>12&0x3f, 8)
		w.put(0x80|v>>6&0x3f, 8)
		w.put(0x80|v&0x3f, 8)
	}
}

var crc8Table, crc16Table = func() (t8 [256]uint8, t16 [256]uint16) {
	for i := range 256 {
		c8 := uint8(i)
		c16 := uint16(i) << 8
		for range 8 {
			if c8&0x80 != 0 {
				c8 = c8<<1 ^ 0x07
			} else {
				c8 <<= 1
			}
			if c16&0x8000 != 0 {
				c16 = c16<<1 ^ 0x8005
			} else {
				c16 <<= 1
			}
		}
		t8[i], t16[i] = c8, c16
	}
	return
}()

func crc8(b []byte) uint8 {
	c := uint8(0)
	for _, v := range b {
		c = crc8Table[c^v]
	}
	return c
}

func crc16(b []byte) uint16 {
	c := uint16(0)
	for _, v := range b {
		c = c<<8 ^ crc16Table[byte(c>>8)^v]
	}
	return c
}
