// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package gameid

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"io"
)

// CSO and ZSO are disc images compressed block by block, so a block can be read without
// the ones before it. PlayStation 2 and PSP games both use them, so what's inside is
// the only way to tell them apart. The sector-by-sector code is here so that the disc
// checks can read one exactly as they read an ordinary image.

const maxCompressedBlock = 1 << 20

// compressedISO is the disc inside a .cso or .zso.
type compressedISO struct {
	r         io.ReaderAt
	zso       bool
	size      int64
	blockSize int64
	align     uint
	index     []uint32
	// The last block read: discs are read in small steps.
	cached    int64
	cachedBuf []byte
}

func openCompressedISO(r io.ReaderAt, fileSize int64) (*compressedISO, error) {
	h := at(r, 0, 0x18)
	if h == nil {
		return nil, errors.New("too short")
	}
	le := binary.LittleEndian
	c := &compressedISO{r: r, zso: string(h[:4]) == "ZISO", size: int64(le.Uint64(h[8:])), blockSize: int64(le.Uint32(h[16:])), align: uint(h[21]), cached: -1}
	if c.blockSize <= 0 || c.blockSize > maxCompressedBlock || c.align > 16 || c.size <= 0 || c.size > 1<<40 {
		return nil, errors.New("impossible header")
	}
	blocks := (c.size + c.blockSize - 1) / c.blockSize
	// Only the start of a disc is ever wanted, and a file can claim any number of blocks.
	want := min(blocks+1, 1<<16)
	if 0x18+want*4 > fileSize {
		return nil, errors.New("cut short")
	}
	raw := at(r, 0x18, int(want*4))
	if raw == nil {
		return nil, errors.New("cut short")
	}
	c.index = make([]uint32, want)
	for i := range c.index {
		c.index[i] = le.Uint32(raw[i*4:])
	}
	return c, nil
}

func (c *compressedISO) block(n int64) ([]byte, error) {
	if n == c.cached {
		return c.cachedBuf, nil
	}
	if n < 0 || n+1 >= int64(len(c.index)) {
		return nil, io.EOF
	}
	plain := c.index[n]&0x80000000 != 0
	start := int64(c.index[n]&0x7fffffff) << c.align
	end := int64(c.index[n+1]&0x7fffffff) << c.align
	if end <= start || end-start > 2*maxCompressedBlock {
		return nil, errors.New("impossible block")
	}
	raw := at(c.r, start, int(end-start))
	if raw == nil {
		return nil, io.ErrUnexpectedEOF
	}
	out := raw
	if !plain {
		var err error
		if c.zso {
			out, err = lz4Block(raw, int(c.blockSize))
		} else {
			out, err = io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(raw)), c.blockSize+1))
		}
		if err != nil {
			return nil, err
		}
	}
	c.cached, c.cachedBuf = n, out
	return out, nil
}

// ReadAt implements io.ReaderAt over the disc inside.
func (c *compressedISO) ReadAt(b []byte, off int64) (int, error) {
	done := 0
	for done < len(b) {
		blk, err := c.block((off + int64(done)) / c.blockSize)
		if err != nil {
			return done, err
		}
		in := int((off + int64(done)) % c.blockSize)
		if in >= len(blk) {
			return done, io.ErrUnexpectedEOF
		}
		done += copy(b[done:], blk[in:])
	}
	return done, nil
}

// lz4Block decodes one LZ4 block into at most limit bytes.
func lz4Block(src []byte, limit int) ([]byte, error) {
	bad := errors.New("damaged block")
	out := make([]byte, 0, limit)
	for i := 0; i < len(src); {
		token := src[i]
		i++
		n := int(token >> 4)
		if n == 15 {
			for {
				if i >= len(src) {
					return nil, bad
				}
				b := src[i]
				i++
				n += int(b)
				if b != 255 {
					break
				}
			}
		}
		if i+n > len(src) || len(out)+n > limit {
			return nil, bad
		}
		out = append(out, src[i:i+n]...)
		i += n
		if i >= len(src) {
			break // the last sequence has no match
		}
		if i+2 > len(src) {
			return nil, bad
		}
		back := int(src[i]) | int(src[i+1])<<8
		i += 2
		m := int(token & 15)
		if m == 15 {
			for {
				if i >= len(src) {
					return nil, bad
				}
				b := src[i]
				i++
				m += int(b)
				if b != 255 {
					break
				}
			}
		}
		m += 4
		if back == 0 || back > len(out) || len(out)+m > limit {
			return nil, bad
		}
		for j := 0; j < m; j++ { // overlaps are allowed, so one at a time
			out = append(out, out[len(out)-back])
		}
	}
	return out, nil
}

// sniffCompressedISO looks at the disc inside a .cso or .zso.
func sniffCompressedISO(r io.ReaderAt, size int64) Finding {
	c, err := openCompressedISO(r, size)
	if err != nil {
		return Finding{Proof: "It starts like a compressed disc image, but its header makes no sense."}
	}
	if f, ok := sniffDisc(c, c.size); ok {
		if f.Proof != "" && f.Console != "" {
			f.Proof += " (Read from inside the compressed image.)"
		}
		return f
	}
	return Finding{Proof: "It's a compressed disc image, with nothing on it that says which console it's for."}
}
