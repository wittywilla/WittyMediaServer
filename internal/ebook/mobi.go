// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package ebook

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/charmap"
)

// Kindle and Palm books: a Palm database whose records hold the compressed text and,
// after it, the pictures. Older books are one long HTML file; KF8 (AZW3) is many, the
// pieces of each stored apart. Both read well enough as text in order, because the
// HTML parser puts a fragment that comes after its skeleton where it belongs.

const (
	maxRecords = 65535
)

type palmDB struct {
	data    []byte
	offsets []int
	typ     string
}

func readPalm(d []byte) (*palmDB, error) {
	if len(d) < 78 {
		return nil, errors.New("not a Palm database")
	}
	n := int(binary.BigEndian.Uint16(d[76:]))
	if n == 0 || n > maxRecords || 78+n*8 > len(d) {
		return nil, errors.New("this book's record table is damaged")
	}
	p := &palmDB{data: d, typ: string(d[60:68]), offsets: make([]int, n+1)}
	for i := 0; i < n; i++ {
		p.offsets[i] = int(binary.BigEndian.Uint32(d[78+i*8:]))
		if p.offsets[i] > len(d) || (i > 0 && p.offsets[i] < p.offsets[i-1]) {
			return nil, errors.New("this book's record table is damaged")
		}
	}
	p.offsets[n] = len(d)
	return p, nil
}

func (p *palmDB) record(i int) []byte {
	if i < 0 || i+1 >= len(p.offsets) {
		return nil
	}
	return p.data[p.offsets[i]:p.offsets[i+1]]
}

func (p *palmDB) count() int { return len(p.offsets) - 1 }

// mobiHeader is what's read from the first record of a section.
type mobiHeader struct {
	compression uint16
	encryption  uint16
	textLen     uint32
	textRecords int
	recSize     int
	mobiLen     int
	encoding    uint32
	version     uint32
	firstImage  int
	huffRec     int
	huffCount   int
	extraFlags  uint16
	titleOff    int
	titleLen    int
	exth        map[int][]byte
	isMobi      bool
}

func parseMobi(rec []byte) (*mobiHeader, error) {
	if len(rec) < 16 {
		return nil, errors.New("this book's header is damaged")
	}
	h := &mobiHeader{
		compression: binary.BigEndian.Uint16(rec[0:]),
		textLen:     binary.BigEndian.Uint32(rec[4:]),
		textRecords: int(binary.BigEndian.Uint16(rec[8:])),
		recSize:     int(binary.BigEndian.Uint16(rec[10:])),
		encryption:  binary.BigEndian.Uint16(rec[12:]),
		exth:        map[int][]byte{},
	}
	if len(rec) < 16+8 || string(rec[16:20]) != "MOBI" {
		return h, nil // a plain Palm document
	}
	h.isMobi = true
	h.mobiLen = int(binary.BigEndian.Uint32(rec[20:]))
	u32 := func(off int) uint32 {
		if off+4 > len(rec) {
			return 0
		}
		return binary.BigEndian.Uint32(rec[off:])
	}
	h.encoding = u32(28)
	h.version = u32(36)
	h.firstImage = int(u32(108))
	h.titleOff, h.titleLen = int(u32(84)), int(u32(88))
	h.huffRec, h.huffCount = int(u32(112)), int(u32(116))
	if h.mobiLen >= 228 && 16+228 <= len(rec) {
		h.extraFlags = binary.BigEndian.Uint16(rec[16+226:])
	}
	if u32(128)&0x40 != 0 {
		h.readEXTH(rec)
	}
	return h, nil
}

func (h *mobiHeader) readEXTH(rec []byte) {
	at := 16 + h.mobiLen
	if at < 0 || at+12 > len(rec) || string(rec[at:at+4]) != "EXTH" {
		return
	}
	count := int(binary.BigEndian.Uint32(rec[at+8:]))
	off := at + 12
	for i := 0; i < count && i < 4096 && off+8 <= len(rec); i++ {
		typ := int(binary.BigEndian.Uint32(rec[off:]))
		size := int(binary.BigEndian.Uint32(rec[off+4:]))
		if size < 8 || off+size > len(rec) {
			return
		}
		if _, seen := h.exth[typ]; !seen {
			h.exth[typ] = rec[off+8 : off+size]
		}
		off += size
	}
}

func (h *mobiHeader) exthInt(typ int) (int, bool) {
	b := h.exth[typ]
	if len(b) != 4 {
		return 0, false
	}
	return int(binary.BigEndian.Uint32(b)), true
}

// fromMobi reads a Kindle or Palm book. A book with copy protection stops here.
func fromMobi(d []byte) (*Book, error) {
	if bytes.HasPrefix(d, []byte{0xEA, 'D', 'R', 'M', 'I', 'O', 'N', 0xEE}) { // Kindle's newer protected format
		return nil, ErrProtected
	}
	db, err := readPalm(d)
	if err != nil {
		return nil, err
	}
	if db.typ != "BOOKMOBI" && db.typ != "TEXtREAd" {
		return nil, errors.New("this isn't a Kindle or Palm book")
	}
	first, err := parseMobi(db.record(0))
	if err != nil {
		return nil, err
	}
	if first.encryption != 0 {
		return nil, ErrProtected
	}
	// A book with both editions keeps KF8 after a boundary record; it reads better.
	section, base := first, 0
	if b, ok := first.exthInt(121); ok && b > 0 && b < db.count() {
		if rec := db.record(b); rec != nil {
			if h, err := parseMobi(rec); err == nil && h.isMobi && h.version >= 8 {
				if h.encryption != 0 {
					return nil, ErrProtected
				}
				section, base = h, b
			}
		}
	}
	text, err := mobiText(db, section, base)
	if err != nil {
		return nil, err
	}
	book := &Book{}
	if first.isMobi {
		rec := db.record(0)
		if first.titleOff > 0 && first.titleLen > 0 && first.titleOff+first.titleLen <= len(rec) && first.titleLen < 4096 {
			book.Title = textIn(rec[first.titleOff:first.titleOff+first.titleLen], first.encoding)
		}
		book.Author = exthText(first, 100, first.encoding)
		book.Description = tagRe.ReplaceAllString(exthText(first, 103, first.encoding), "")
		book.Date = exthText(first, 106, first.encoding)
		if t := exthText(first, 503, first.encoding); t != "" {
			book.Title = t
		}
		book.Language = exthText(first, 524, first.encoding)
	}
	// Pictures are found by record number, from the first image record on.
	imageBase := first.firstImage
	stored := map[int]string{}
	image := func(n int) string {
		if name, ok := stored[n]; ok {
			return name
		}
		name := ""
		if imageBase > 0 && imageBase+n < db.count() {
			if rec := db.record(imageBase + n); len(rec) > 0 {
				name = book.addImage(rec)
			}
		}
		stored[n] = name
		return name
	}
	if n, ok := first.exthInt(201); ok { // the cover's place among the pictures
		book.Cover = image(n)
	}
	opts := htmlOptions{image: func(src string) string {
		var n int
		switch {
		case strings.HasPrefix(src, "recindex:"):
			v, err := strconv.Atoi(strings.TrimPrefix(src, "recindex:"))
			if err != nil || v <= 0 {
				return ""
			}
			n = v - 1
		case strings.HasPrefix(src, "kindle:embed:"):
			code := strings.TrimPrefix(src, "kindle:embed:")
			if i := strings.IndexByte(code, '?'); i >= 0 {
				code = code[:i]
			}
			v, err := strconv.ParseInt(code, 32, 32)
			if err != nil || v <= 0 {
				return ""
			}
			n = int(v) - 1
		default:
			return ""
		}
		return image(n)
	}}
	switch {
	case first.isMobi && section.version >= 8:
		for _, part := range kf8Parts(text) {
			book.addChapter("", sanitizeHTML(part, opts))
		}
	default:
		for _, part := range pageBreakRe.Split(text, maxChapters) {
			book.addChapter("", sanitizeHTML(part, opts))
		}
	}
	return book, nil
}

var (
	pageBreakRe = regexp.MustCompile(`(?i)<\s*mbp:pagebreak\s*/?\s*>`)
	kf8StartRe  = regexp.MustCompile(`(?i)<\?xml[^>]*\?>|<!doctype[^>]*>\s*<html|<html[\s>]`)
)

// kf8Parts cuts KF8's text at the start of each of its HTML files.
func kf8Parts(text string) []string {
	idx := kf8StartRe.FindAllStringIndex(text, maxChapters)
	var starts []int
	last := -1 << 30
	for _, m := range idx {
		// A declaration and its html tag start the same file.
		if m[0]-last > 400 {
			starts = append(starts, m[0])
		}
		last = m[0]
	}
	if len(starts) == 0 {
		return []string{text}
	}
	out := make([]string, 0, len(starts))
	for i, s := range starts {
		end := len(text)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		out = append(out, text[s:end])
	}
	return out
}

func exthText(h *mobiHeader, typ int, enc uint32) string {
	b := h.exth[typ]
	if len(b) == 0 || len(b) > 1<<20 {
		return ""
	}
	return strings.TrimSpace(textIn(b, enc))
}

func textIn(b []byte, enc uint32) string {
	if enc == 65001 {
		return strings.ToValidUTF8(string(b), "")
	}
	out, err := charmap.Windows1252.NewDecoder().Bytes(b)
	if err != nil {
		return strings.ToValidUTF8(string(b), "")
	}
	return string(out)
}

// mobiText unpacks the text records of a section.
func mobiText(db *palmDB, h *mobiHeader, base int) (string, error) {
	if h.textRecords <= 0 || base+h.textRecords >= db.count()+1 {
		return "", errors.New("this book has no text records")
	}
	var huff *huffDecoder
	if h.compression == 17480 {
		var err error
		huff, err = newHuff(db, base+h.huffRec, h.huffCount)
		if err != nil {
			return "", err
		}
	}
	var out bytes.Buffer
	for i := 1; i <= h.textRecords; i++ {
		rec := db.record(base + i)
		if rec == nil {
			break
		}
		rec = rec[:len(rec)-trailing(rec, h.extraFlags)]
		var part []byte
		var err error
		switch h.compression {
		case 1:
			part = rec
		case 2:
			part, err = palmDecompress(rec)
		case 17480:
			part, err = huff.decode(rec)
		default:
			return "", fmt.Errorf("this book uses a kind of compression that isn't known (%d)", h.compression)
		}
		if err != nil {
			return "", err
		}
		if out.Len()+len(part) > maxBookText {
			return "", ErrTooBig
		}
		out.Write(part)
	}
	return textIn(out.Bytes(), h.encoding), nil
}

// trailing is the size of the extra data at the end of a text record, which the flags
// in the header say to expect.
func trailing(rec []byte, flags uint16) int {
	total := 0
	for bit := 15; bit >= 1; bit-- {
		if flags&(1<<bit) == 0 {
			continue
		}
		end := len(rec) - total
		if end <= 0 {
			return len(rec)
		}
		// A variable-length number read backwards, each byte's top bit ending it.
		size, shift := 0, 0
		for k := 0; k < 4 && end-1-k >= 0; k++ {
			b := rec[end-1-k]
			size |= int(b&0x7F) << shift
			shift += 7
			if b&0x80 != 0 {
				break
			}
		}
		total += size
	}
	if flags&1 != 0 {
		if end := len(rec) - total; end > 0 {
			total += int(rec[end-1]&3) + 1
		}
	}
	return min(total, len(rec))
}

// palmDecompress undoes PalmDOC's simple compression. Text records are 4 KiB by
// design, so anything far larger is not a book.
func palmDecompress(in []byte) ([]byte, error) {
	const maxOut = 1 << 20
	out := make([]byte, 0, min(len(in)*2, maxOut))
	for i := 0; i < len(in); {
		c := in[i]
		i++
		switch {
		case c == 0 || (c >= 0x09 && c <= 0x7F):
			out = append(out, c)
		case c >= 0x01 && c <= 0x08:
			if i+int(c) > len(in) {
				return out, nil
			}
			out = append(out, in[i:i+int(c)]...)
			i += int(c)
		case c >= 0x80 && c <= 0xBF:
			if i >= len(in) {
				return out, nil
			}
			v := int(c)<<8 | int(in[i])
			i++
			dist, length := (v&0x3FFF)>>3, v&7+3
			if dist == 0 || dist > len(out) {
				return nil, errors.New("this book's text is damaged")
			}
			for k := 0; k < length; k++ {
				out = append(out, out[len(out)-dist])
			}
		default:
			out = append(out, ' ', c^0x80)
		}
		if len(out) > maxOut {
			return nil, errors.New("this book's text is damaged")
		}
	}
	return out, nil
}

// huffDecoder undoes the Huffman compression most Kindle books of the mid-2000s to
// 2010s use (HUFF and CDIC records). Dictionary entries may themselves be compressed:
// they are unpacked once, the first time they are used, and an entry that needs itself
// is refused.
type huffDecoder struct {
	cacheLen  [256]uint32
	cacheTerm [256]bool
	cacheMax  [256]uint64
	minCode   [33]uint64
	maxCode   [33]uint64
	dict      []huffEntry
	depth     int
}

type huffEntry struct {
	data    []byte
	plain   bool
	working bool
}

var errHuff = errors.New("this book's compression tables are damaged")

func newHuff(db *palmDB, first, count int) (*huffDecoder, error) {
	if count < 2 || count > 512 {
		return nil, errHuff
	}
	rec := db.record(first)
	if len(rec) < 16 || string(rec[:4]) != "HUFF" {
		return nil, errHuff
	}
	cacheOff := int(binary.BigEndian.Uint32(rec[8:]))
	baseOff := int(binary.BigEndian.Uint32(rec[12:]))
	if cacheOff < 0 || baseOff < 0 || cacheOff+1024 > len(rec) || baseOff+512 > len(rec) {
		return nil, errHuff
	}
	h := &huffDecoder{}
	for i := 0; i < 256; i++ {
		v := binary.BigEndian.Uint32(rec[cacheOff+i*4:])
		codeLen := v & 0x1F
		if codeLen == 0 {
			return nil, errHuff
		}
		h.cacheLen[i], h.cacheTerm[i] = codeLen, v&0x80 != 0
		h.cacheMax[i] = (uint64(v>>8)+1)<<(32-codeLen) - 1
	}
	for l := 1; l <= 32; l++ {
		h.minCode[l] = uint64(binary.BigEndian.Uint32(rec[baseOff+(l-1)*8:])) << (32 - l)
		h.maxCode[l] = (uint64(binary.BigEndian.Uint32(rec[baseOff+(l-1)*8+4:]))+1)<<(32-l) - 1
	}
	for r := first + 1; r < first+count; r++ {
		c := db.record(r)
		if len(c) < 16 || string(c[:4]) != "CDIC" {
			return nil, errHuff
		}
		entries := int(binary.BigEndian.Uint32(c[8:]))
		bits := int(binary.BigEndian.Uint32(c[12:]))
		if bits < 1 || bits > 24 {
			return nil, errHuff
		}
		n := min(1<<bits, entries-len(h.dict))
		for i := 0; i < n && 16+i*2+2 <= len(c); i++ {
			off := 16 + int(binary.BigEndian.Uint16(c[16+i*2:]))
			if off+2 > len(c) {
				h.dict = append(h.dict, huffEntry{plain: true})
				continue
			}
			v := binary.BigEndian.Uint16(c[off:])
			end := min(off+2+int(v&0x7FFF), len(c))
			h.dict = append(h.dict, huffEntry{data: c[off+2 : end], plain: v&0x8000 != 0})
		}
		if len(h.dict) > 1<<24 {
			return nil, errHuff
		}
	}
	return h, nil
}

func (h *huffDecoder) decode(in []byte) ([]byte, error) {
	if h.depth > 32 {
		return nil, errHuff
	}
	h.depth++
	defer func() { h.depth-- }()
	const maxOut = 4 << 20
	var out []byte
	bitsLeft := len(in) * 8
	// A 32-bit look at the bits at position bit; past the end reads as zeros.
	peek := func(bit int) uint64 {
		var v uint64
		at := bit / 8
		for k := 0; k < 5; k++ {
			v <<= 8
			if at+k < len(in) {
				v |= uint64(in[at+k])
			}
		}
		return (v >> (8 - uint(bit%8))) & 0xFFFFFFFF
	}
	for bit := 0; ; {
		code := peek(bit)
		first := code >> 24
		codeLen := int(h.cacheLen[first])
		maxCode := h.cacheMax[first]
		if !h.cacheTerm[first] {
			for codeLen <= 32 && code < h.minCode[codeLen] {
				codeLen++
			}
			if codeLen > 32 {
				return nil, errHuff
			}
			maxCode = h.maxCode[codeLen]
		}
		bit += codeLen
		bitsLeft -= codeLen
		if bitsLeft < 0 {
			return out, nil
		}
		if maxCode < code {
			return nil, errHuff
		}
		idx := (maxCode - code) >> (32 - uint(codeLen))
		if idx >= uint64(len(h.dict)) {
			return nil, errHuff
		}
		e := &h.dict[idx]
		if !e.plain {
			if e.working {
				return nil, errHuff
			}
			e.working = true
			plain, err := h.decode(e.data)
			e.working = false
			if err != nil {
				return nil, err
			}
			e.data, e.plain = plain, true
		}
		out = append(out, e.data...)
		if len(out) > maxOut {
			return nil, errHuff
		}
	}
}
