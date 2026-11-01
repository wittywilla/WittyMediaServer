// Package patch applies a person's own IPS and BPS patch files to a copy of a game. A
// patch is an untrusted file, so every length is checked before it's used and the
// output's size is limited before any memory is set aside.
package patch

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
)

// Limits.
const (
	MaxPatch  = 64 << 20  // the patch file
	MaxOutput = 512 << 20 // the game it produces
	// maxIPSWork is how much an IPS patch may write, counting a place each time. No real
	// patch comes close, and records that write the same place again could take minutes.
	maxIPSWork = 2 * MaxOutput
)

// Kinds of patch.
const (
	IPS = "ips"
	BPS = "bps"
)

// ErrSource means the patch was made for a different copy of the game.
var ErrSource = errors.New("this patch was made for a different version of this game")

// Detect says what kind of patch a file is, checking enough of it to refuse a file
// that only starts like one. An error says why it isn't usable.
func Detect(p []byte) (string, error) {
	if len(p) > MaxPatch {
		return "", fmt.Errorf("this patch is too large: %d MB at most", MaxPatch>>20)
	}
	switch {
	case bytes.HasPrefix(p, []byte("PATCH")):
		if _, err := applyIPS(nil, p, true); err != nil {
			return "", err
		}
		return IPS, nil
	case bytes.HasPrefix(p, []byte("BPS1")):
		if err := checkBPS(p); err != nil {
			return "", err
		}
		return BPS, nil
	}
	return "", errors.New("this isn't an IPS or BPS patch")
}

// Apply patches a copy of a game, never the original.
func Apply(game, p []byte) ([]byte, error) {
	kind, err := Detect(p)
	if err != nil {
		return nil, err
	}
	if kind == IPS {
		return applyIPS(game, p, false)
	}
	return applyBPS(game, p)
}

// IPS: "PATCH", then records of a 3-byte offset, a 2-byte size and that many bytes, or
// a size of 0 with a 2-byte count and one byte to repeat; "EOF", then perhaps a 3-byte
// size to cut the game to.
func applyIPS(game, p []byte, dry bool) ([]byte, error) {
	const eof = 0x454f46
	bad := errors.New("this IPS patch is damaged")
	var out []byte
	if !dry {
		out = append(make([]byte, 0, len(game)), game...)
	}
	work := 0
	grow := func(end, n int) error {
		if end > MaxOutput {
			return errors.New("this patch would make the game too large")
		}
		if work += n; work > maxIPSWork {
			return errors.New("this patch writes far more than a game holds, so it isn't one to trust")
		}
		if !dry && end > len(out) {
			out = append(out, make([]byte, end-len(out))...)
		}
		return nil
	}
	i := 5
	for {
		if i+3 > len(p) {
			return nil, bad
		}
		off := int(p[i])<<16 | int(p[i+1])<<8 | int(p[i+2])
		i += 3
		if off == eof {
			break
		}
		if i+2 > len(p) {
			return nil, bad
		}
		n := int(binary.BigEndian.Uint16(p[i:]))
		i += 2
		if n == 0 { // a run
			if i+3 > len(p) {
				return nil, bad
			}
			run, v := int(binary.BigEndian.Uint16(p[i:])), p[i+2]
			i += 3
			if run == 0 {
				return nil, bad
			}
			if err := grow(off+run, run); err != nil {
				return nil, err
			}
			if !dry {
				for k := off; k < off+run; k++ {
					out[k] = v
				}
			}
			continue
		}
		if i+n > len(p) {
			return nil, bad
		}
		if err := grow(off+n, n); err != nil {
			return nil, err
		}
		if !dry {
			copy(out[off:], p[i:i+n])
		}
		i += n
	}
	switch rest := len(p) - i; rest {
	case 0:
	case 3:
		size := int(p[i])<<16 | int(p[i+1])<<8 | int(p[i+2])
		if !dry && size <= len(out) {
			out = out[:size]
		}
	default:
		return nil, bad
	}
	return out, nil
}

// bpsReader reads BPS's variable-length numbers, whose ends are checked at every step.
type bpsReader struct {
	p []byte
	i int
}

func (r *bpsReader) number() (uint64, bool) {
	var v, shift uint64 = 0, 1
	for k := 0; k < 9; k++ {
		if r.i >= len(r.p) {
			return 0, false
		}
		b := r.p[r.i]
		r.i++
		v += uint64(b&0x7f) * shift
		if b&0x80 != 0 {
			return v, v < 1<<62
		}
		shift <<= 7
		v += shift
	}
	return 0, false
}

func checkBPS(p []byte) error {
	if len(p) < 4+3+12 {
		return errors.New("this BPS patch is damaged")
	}
	body := p[:len(p)-4]
	if crc32.ChecksumIEEE(body) != binary.LittleEndian.Uint32(p[len(p)-4:]) {
		return errors.New("this BPS patch is damaged: its checksum doesn't match")
	}
	r := &bpsReader{p: p[:len(p)-12], i: 4}
	_, ok1 := r.number()
	target, ok2 := r.number()
	if !ok1 || !ok2 {
		return errors.New("this BPS patch is damaged")
	}
	if target > MaxOutput {
		return errors.New("this patch would make the game too large")
	}
	return nil
}

// BPS: source size, target size, a metadata block, then actions, then checksums of the
// source, the target and the patch.
func applyBPS(game, p []byte) ([]byte, error) {
	bad := errors.New("this BPS patch is damaged")
	foot := p[len(p)-12:]
	srcCRC := binary.LittleEndian.Uint32(foot)
	dstCRC := binary.LittleEndian.Uint32(foot[4:])
	r := &bpsReader{p: p[:len(p)-12], i: 4}
	srcSize, ok1 := r.number()
	dstSize, ok2 := r.number()
	metaSize, ok3 := r.number()
	if !ok1 || !ok2 || !ok3 || dstSize > MaxOutput || metaSize > uint64(len(r.p)-r.i) {
		return nil, bad
	}
	r.i += int(metaSize)
	if srcSize != uint64(len(game)) || crc32.ChecksumIEEE(game) != srcCRC {
		return nil, ErrSource
	}
	out := make([]byte, dstSize)
	o := 0
	var srcRel, dstRel int
	for r.i < len(r.p) {
		cmd, ok := r.number()
		if !ok {
			return nil, bad
		}
		length := cmd>>2 + 1
		if length > dstSize-uint64(o) {
			return nil, bad
		}
		n := int(length)
		switch cmd & 3 {
		case 0: // the same bytes as the source, at the same place
			if uint64(o)+length > uint64(len(game)) {
				return nil, bad
			}
			copy(out[o:o+n], game[o:o+n])
		case 1: // bytes from the patch
			if n > len(r.p)-r.i {
				return nil, bad
			}
			copy(out[o:o+n], r.p[r.i:r.i+n])
			r.i += n
		case 2, 3: // from elsewhere in the source, or from what's been written so far
			off, ok := r.number()
			if !ok {
				return nil, bad
			}
			delta := int64(off >> 1)
			if off&1 != 0 {
				delta = -delta
			}
			if cmd&3 == 2 {
				next := int64(srcRel) + delta
				if next < 0 || next+int64(n) > int64(len(game)) {
					return nil, bad
				}
				copy(out[o:o+n], game[next:next+int64(n)])
				srcRel = int(next) + n
			} else {
				next := int64(dstRel) + delta
				if next < 0 || next >= int64(o) || next+int64(n) > int64(len(out)) {
					return nil, bad
				}
				// May overlap on purpose: it repeats what was just written.
				for k := 0; k < n; k++ {
					out[o+k] = out[int(next)+k]
				}
				dstRel = int(next) + n
			}
		}
		o += n
	}
	if uint64(o) != dstSize || crc32.ChecksumIEEE(out) != dstCRC {
		return nil, errors.New("this patch didn't produce what it should have, so it wasn't used")
	}
	return out, nil
}
