package wittycd

import "errors"

// A raw LZMA decoder, written from the published description of the format, for
// the audio inside CHD images. There is no stream header: the caller knows the
// settings and exactly how many bytes should come out. Output goes straight into
// the buffer it was asked to fill, which is also the history that matches copy
// from, so a hostile stream can't reach past what has already been written or
// make it use more memory than the caller gave it.

var errLZMA = errors.New("this LZMA data is damaged")

const (
	lzStates   = 12
	lzPosBits  = 4 // the most position bits a stream can use
	lzAlign    = 4
	lzLenLow   = 3
	lzLenMid   = 3
	lzLenHigh  = 8
	lzEndModel = 14
	lzFullDist = 128

	// Where each table starts in the one array of probabilities.
	lzIsMatch    = 0
	lzIsRep      = lzIsMatch + lzStates<<lzPosBits
	lzIsRepG0    = lzIsRep + lzStates
	lzIsRepG1    = lzIsRepG0 + lzStates
	lzIsRepG2    = lzIsRepG1 + lzStates
	lzIsRep0Long = lzIsRepG2 + lzStates
	lzPosSlot    = lzIsRep0Long + lzStates<<lzPosBits
	lzPosSpecial = lzPosSlot + 4<<6
	lzPosAlign   = lzPosSpecial + 1 + lzFullDist - lzEndModel
	lzLenMatch   = lzPosAlign + 1<<lzAlign
	lzLenSize    = 2 + 2<<lzPosBits<<lzLenLow + 1<<lzLenHigh
	lzLenRep     = lzLenMatch + lzLenSize
	lzLiteral    = lzLenRep + lzLenSize

	lzProbInit = 1 << 10
)

// lzmaDecoder keeps its probability tables between calls, since there is one
// call for every block of a disc.
type lzmaDecoder struct {
	probs []uint16

	src   []byte
	pos   int
	rng   uint32
	code  uint32
	short bool
}

func (z *lzmaDecoder) next() uint32 {
	if z.pos >= len(z.src) {
		z.short = true
		return 0
	}
	v := z.src[z.pos]
	z.pos++
	return uint32(v)
}

// bit decodes one bit against a probability, and moves the probability toward it.
func (z *lzmaDecoder) bit(at int) uint32 {
	if z.rng < 1<<24 {
		z.rng <<= 8
		z.code = z.code<<8 | z.next()
	}
	p := uint32(z.probs[at])
	bound := (z.rng >> 11) * p
	if z.code < bound {
		z.rng = bound
		z.probs[at] = uint16(p + (1<<11-p)>>5)
		return 0
	}
	z.rng -= bound
	z.code -= bound
	z.probs[at] = uint16(p - p>>5)
	return 1
}

// direct decodes bits that are equally likely either way.
func (z *lzmaDecoder) direct(n uint) uint32 {
	v := uint32(0)
	for range n {
		if z.rng < 1<<24 {
			z.rng <<= 8
			z.code = z.code<<8 | z.next()
		}
		z.rng >>= 1
		v <<= 1
		if z.code >= z.rng {
			z.code -= z.rng
			v |= 1
		}
	}
	return v
}

// tree decodes n bits through a tree of probabilities, top bit first.
func (z *lzmaDecoder) tree(base int, n uint) uint32 {
	m := uint32(1)
	for range n {
		m = m<<1 | z.bit(base+int(m))
	}
	return m - 1<<n
}

// treeReverse is the same with the lowest bit first.
func (z *lzmaDecoder) treeReverse(base int, n uint) uint32 {
	m, v := uint32(1), uint32(0)
	for i := range n {
		b := z.bit(base + int(m))
		m = m<<1 | b
		v |= b << i
	}
	return v
}

func (z *lzmaDecoder) length(base int, posState uint32) uint32 {
	if z.bit(base) == 0 {
		return z.tree(base+2+int(posState)<<lzLenLow, lzLenLow)
	}
	if z.bit(base+1) == 0 {
		return 1<<lzLenLow + z.tree(base+2+1<<lzPosBits<<lzLenLow+int(posState)<<lzLenMid, lzLenMid)
	}
	return 1<<lzLenLow + 1<<lzLenMid + z.tree(base+2+2<<lzPosBits<<lzLenLow, lzLenHigh)
}

// decode fills dst from src, with the settings given. It is an error if src runs
// out or asks for anything impossible before dst is full.
func (z *lzmaDecoder) decode(dst, src []byte, lc, lp, pb uint) error {
	if lc > 8 || lp > 4 || pb > lzPosBits {
		return errLZMA
	}
	need := lzLiteral + 0x300<<(lc+lp)
	if cap(z.probs) < need {
		z.probs = make([]uint16, need)
	}
	z.probs = z.probs[:need]
	for i := range z.probs {
		z.probs[i] = lzProbInit
	}
	if len(src) < 5 || src[0] != 0 {
		return errLZMA
	}
	z.src, z.pos, z.short = src, 1, false
	z.rng = 0xffffffff
	z.code = uint32(src[1])<<24 | uint32(src[2])<<16 | uint32(src[3])<<8 | uint32(src[4])
	z.pos = 5
	if z.code == z.rng {
		return errLZMA
	}

	var state uint32
	var rep0, rep1, rep2, rep3 uint32
	pbMask, lpMask := uint32(1)<<pb-1, uint32(1)<<lp-1
	at := 0
	for at < len(dst) {
		if z.short {
			return errLZMA
		}
		posState := uint32(at) & pbMask
		if z.bit(lzIsMatch+int(state<<lzPosBits+posState)) == 0 {
			// A literal byte, predicted from the one before it.
			prev := uint32(0)
			if at > 0 {
				prev = uint32(dst[at-1])
			}
			lit := lzLiteral + 0x300*int((uint32(at)&lpMask)<<lc+prev>>(8-lc))
			sym := uint32(1)
			if state >= 7 {
				// Just after a match, the byte the match would have given next is a good guess.
				if uint64(rep0) >= uint64(at) {
					return errLZMA
				}
				match := uint32(dst[at-int(rep0)-1])
				for sym < 0x100 {
					mb := match >> 7 & 1
					match <<= 1
					b := z.bit(lit + int((1+mb)<<8+sym))
					sym = sym<<1 | b
					if mb != b {
						break
					}
				}
			}
			for sym < 0x100 {
				sym = sym<<1 | z.bit(lit+int(sym))
			}
			dst[at] = byte(sym)
			at++
			switch {
			case state < 4:
				state = 0
			case state < 10:
				state -= 3
			default:
				state -= 6
			}
			continue
		}

		var n uint32
		if z.bit(lzIsRep+int(state)) != 0 {
			// A match at one of the last four distances.
			if at == 0 {
				return errLZMA
			}
			if z.bit(lzIsRepG0+int(state)) == 0 {
				if z.bit(lzIsRep0Long+int(state<<lzPosBits+posState)) == 0 {
					// One byte, from the last distance.
					if uint64(rep0) >= uint64(at) {
						return errLZMA
					}
					dst[at] = dst[at-int(rep0)-1]
					at++
					if state < 7 {
						state = 9
					} else {
						state = 11
					}
					continue
				}
			} else {
				var d uint32
				if z.bit(lzIsRepG1+int(state)) == 0 {
					d = rep1
				} else {
					if z.bit(lzIsRepG2+int(state)) == 0 {
						d = rep2
					} else {
						d = rep3
						rep3 = rep2
					}
					rep2 = rep1
				}
				rep1 = rep0
				rep0 = d
			}
			n = z.length(lzLenRep, posState)
			if state < 7 {
				state = 8
			} else {
				state = 11
			}
		} else {
			// A match at a new distance.
			rep3, rep2, rep1 = rep2, rep1, rep0
			n = z.length(lzLenMatch, posState)
			if state < 7 {
				state = 7
			} else {
				state = 10
			}
			ls := min(n, 3)
			slot := z.tree(lzPosSlot+int(ls)<<6, 6)
			if slot < 4 {
				rep0 = slot
			} else {
				footer := uint(slot>>1 - 1)
				rep0 = (2 | slot&1) << footer
				if slot < lzEndModel {
					rep0 += z.treeReverse(lzPosSpecial+int(rep0)-int(slot), footer)
				} else {
					rep0 += z.direct(footer-lzAlign) << lzAlign
					rep0 += z.treeReverse(lzPosAlign, lzAlign)
				}
			}
			if rep0 == 0xffffffff {
				// An end marker before the end: something is missing.
				return errLZMA
			}
		}
		if uint64(rep0) >= uint64(at) {
			return errLZMA
		}
		n += 2
		if uint64(n) > uint64(len(dst)-at) {
			return errLZMA
		}
		// One byte at a time on purpose: a match can overlap what it is writing.
		from := at - int(rep0) - 1
		for i := range int(n) {
			dst[at+i] = dst[from+i]
		}
		at += int(n)
	}
	if z.short {
		return errLZMA
	}
	return nil
}
