package wittycd

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// CHD is the "compressed hunks of data" format that chdman makes. A file is a
// header, a map saying where each fixed-size hunk is and how it was compressed, the
// hunks, and a chain of metadata that holds the track list. Everything here works
// from version 5 of the format, the one chdman has written for many years.
//
// On a CD image every sector is a frame of 2352 bytes of audio or data followed by
// 96 of subcode. Each track is padded to a multiple of 4 frames in the file, which
// is why a track's place in the file and its place on the disc have to be worked out
// from the list. Audio in frames is big-endian whatever the machine.

const (
	chdHeaderBytes = 124
	chdSubBytes    = 96
	chdFrameBytes  = SectorBytes + chdSubBytes
	chdTrackPad    = 4 // frames
	// chdMaxLogical is more than any CD can hold, even with its padding.
	chdMaxLogical = 1200 * 1000 * 1000
	// chdMaxHunk is far beyond what any tool makes for a CD by default.
	chdMaxHunk = 8 << 20
	// chdMaxMap bounds a compressed map. A full disc's is a few hundred kilobytes.
	chdMaxMap = 64 << 20
	// chdMaxSteps bounds the metadata chain, which a damaged file could make a loop.
	chdMaxSteps = 1024
	// chdMaxHops bounds a chain of hunks that say they are the same as another.
	chdMaxHops = 64
	// chdCacheBytes is how much decompressed audio is kept.
	chdCacheBytes = 2 << 20
	// chdMinCache is how many hunks are kept, however big they are.
	chdMinCache = 4
)

// What a map entry says about a hunk.
const (
	chdType0 = iota // compressed with the header's first codec; 1 to 3 are the others
	chdType1
	chdType2
	chdType3
	chdPlain      // stored as it is
	chdSelf       // the same as another hunk in this file
	chdParent     // the same as a hunk in another file
	chdRLESmall   // the type before it, repeated a few times
	chdRLELarge   // the type before it, repeated many times
	chdSelf0      // the same other hunk as last time
	chdSelf1      // the one after the same other hunk as last time
	chdParentSame // the hunk at this position in the parent
	chdParent0    // the same parent hunk as last time
	chdParent1    // the one after the same parent hunk as last time
	chdZero       // from an uncompressed map: never written, so all zero
	chdStored     // from an uncompressed map: stored as it is, with no checksum
)

func fourCC(s string) uint32 { return binary.BigEndian.Uint32([]byte(s)) }

var (
	chdCDLZ = fourCC("cdlz")
	chdCDZL = fourCC("cdzl")
	chdCDFL = fourCC("cdfl")
	chdZLIB = fourCC("zlib")
	chdLZMA = fourCC("lzma")
	chdFLAC = fourCC("flac")
	chdHUFF = fourCC("huff")
)

// errCHDDamaged is a hunk that can't be read back, which plays as silence.
var errCHDDamaged = errors.New("this hunk of the image is damaged")

type chdEntry struct {
	offset uint64 // in the file, or a hunk number for chdSelf
	length uint32
	crc    uint16
	kind   uint8
}

type chdHunk struct {
	num  int
	data []byte
	used uint64
}

// chdFile reads the image as one long run of frames, from any number of listeners
// at once.
type chdFile struct {
	r         io.ReaderAt
	size      int64
	logical   int64
	hunkBytes int
	hunks     int
	codecs    [4]uint32
	mapOff    int64
	metaOff   int64
	hasParent bool
	entries   []chdEntry

	mu      sync.Mutex
	cache   []chdHunk
	clock   uint64
	comp    []byte
	sectors []byte
	sub     []byte
	lz      lzmaDecoder
	flac    flacScratch
	inflate io.ReadCloser
	src     bytes.Reader
}

// chdTrack is one track as the metadata lists it.
type chdTrack struct {
	number          int
	kind, subType   string
	frames          int
	pregap, postgap int
	// pregapStored: the pregap's frames are in the file, and frames counts them.
	pregapStored bool
}

func (d *Disc) readCHD(path string) error {
	d.Format = "chd"
	f, size, err := d.file(path)
	if err != nil {
		return fmt.Errorf("can't read %q: %w", filepath.Base(path), err)
	}
	c, err := openCHD(f, size)
	if err != nil {
		return err
	}
	tracks, err := c.readTracks()
	if err != nil {
		return err
	}
	if err := c.readMap(); err != nil {
		return err
	}
	if c.cutShort() {
		d.warn("truncated", "This image is shorter than its map says, so some of its audio will play as silence.")
	}
	d.layoutCHD(c, tracks)
	d.Text.Title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return nil
}

func openCHD(r io.ReaderAt, size int64) (*chdFile, error) {
	var h [chdHeaderBytes]byte
	n, err := r.ReadAt(h[:], 0)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("can't read this CHD image: %w", err)
	}
	if n < 16 || string(h[:8]) != "MComprHD" {
		return nil, fmt.Errorf("%w: this .chd file isn't really a CHD image", ErrNotCD)
	}
	if v := binary.BigEndian.Uint32(h[12:]); v != 5 {
		return nil, fmt.Errorf("%w: it is a version %d CHD and WittyCD reads version 5. Running chdman's copy command on it makes a version 5 file", ErrUnsupported, v)
	}
	if n < chdHeaderBytes || binary.BigEndian.Uint32(h[8:]) != chdHeaderBytes {
		return nil, fmt.Errorf("%w: this CHD image's header is damaged", ErrNotCD)
	}
	c := &chdFile{r: r, size: size}
	for i := range c.codecs {
		c.codecs[i] = binary.BigEndian.Uint32(h[16+i*4:])
	}
	logical := binary.BigEndian.Uint64(h[32:])
	mapOff := binary.BigEndian.Uint64(h[40:])
	metaOff := binary.BigEndian.Uint64(h[48:])
	hunk := binary.BigEndian.Uint32(h[56:])
	unit := binary.BigEndian.Uint32(h[60:])
	if mapOff >= uint64(size) || metaOff >= uint64(size) {
		return nil, fmt.Errorf("%w: this CHD image's header is damaged", ErrNotCD)
	}
	c.mapOff, c.metaOff = int64(mapOff), int64(metaOff)
	c.hasParent = !bytes.Equal(h[104:chdHeaderBytes], make([]byte, chdHeaderBytes-104))
	// What makes it a CD is checked with the metadata, for a clearer message.
	if logical > chdMaxLogical || unit != chdFrameBytes || hunk == 0 || hunk%chdFrameBytes != 0 || hunk > chdMaxHunk || logical == 0 {
		c.hunkBytes = 0
		return c, nil
	}
	c.logical = int64(logical)
	c.hunkBytes = int(hunk)
	c.hunks = int((c.logical + int64(hunk) - 1) / int64(hunk))
	return c, nil
}

// readTracks follows the metadata chain for the track list.
func (c *chdFile) readTracks() ([]chdTrack, error) {
	var tracks []chdTrack
	seen := map[int]bool{}
	gdrom := false
	off := uint64(c.metaOff)
	for steps := 0; off != 0 && steps < chdMaxSteps; steps++ {
		if off > uint64(c.size) || uint64(c.size)-off < 16 {
			break
		}
		var h [16]byte
		if _, err := c.r.ReadAt(h[:], int64(off)); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("can't read this CHD image: %w", err)
		}
		tag := string(h[:4])
		length := binary.BigEndian.Uint32(h[4:]) & 0xffffff
		next := binary.BigEndian.Uint64(h[8:])
		switch tag {
		case "CHT2", "CHTR":
			if length > 1024 || off+16+uint64(length) > uint64(c.size) {
				break
			}
			b := make([]byte, length)
			if _, err := c.r.ReadAt(b, int64(off)+16); err != nil && err != io.EOF {
				return nil, fmt.Errorf("can't read this CHD image: %w", err)
			}
			if t, ok := parseCHDTrack(string(b)); ok && !seen[t.number] && len(tracks) < MaxTracks {
				seen[t.number] = true
				tracks = append(tracks, t)
			}
		case "CHGT":
			gdrom = true
		}
		off = next
	}
	if len(tracks) == 0 {
		if gdrom {
			return nil, fmt.Errorf("%w: this is an image of a GD-ROM rather than a CD", ErrUnsupported)
		}
		return nil, fmt.Errorf("%w: this CHD image lists no CD tracks, so it holds a hard disk or something else", ErrNotCD)
	}
	if c.hunkBytes == 0 {
		return nil, fmt.Errorf("%w: this CHD image's sizes don't make sense for a CD", ErrNotCD)
	}
	slices.SortFunc(tracks, func(a, b chdTrack) int { return a.number - b.number })
	return tracks, nil
}

// parseCHDTrack reads a track's line, such as
// "TRACK:2 TYPE:AUDIO SUBTYPE:NONE FRAMES:525 PREGAP:150 PGTYPE:VAUDIO PGSUB:NONE POSTGAP:0".
func parseCHDTrack(s string) (chdTrack, bool) {
	var t chdTrack
	haveTrack, haveFrames := false, false
	s, _, _ = strings.Cut(s, "\x00")
	for _, field := range strings.Fields(s) {
		key, val, ok := strings.Cut(field, ":")
		if !ok {
			continue
		}
		num := -1
		if v, err := strconv.Atoi(val); err == nil && v >= 0 && v <= MaxSectors {
			num = v
		}
		switch key {
		case "TRACK":
			t.number, haveTrack = num, num > 0
		case "TYPE":
			t.kind = printable(val)
		case "SUBTYPE":
			t.subType = printable(val)
		case "FRAMES":
			t.frames, haveFrames = num, num >= 0
		case "PREGAP":
			t.pregap = max(num, 0)
		case "POSTGAP":
			t.postgap = max(num, 0)
		case "PGTYPE":
			t.pregapStored = strings.HasPrefix(val, "V")
		}
	}
	return t, haveTrack && haveFrames && t.kind != "" && len(t.kind) <= 32
}

// chdModes gives a track type the name a cue sheet would use.
var chdModes = map[string]string{
	"AUDIO":          "AUDIO",
	"MODE1":          "MODE1/2048",
	"MODE1_RAW":      "MODE1/2352",
	"MODE2":          "MODE2/2336",
	"MODE2_FORM1":    "MODE2/2048",
	"MODE2_FORM2":    "MODE2/2324",
	"MODE2_FORM_MIX": "MODE2/2336",
	"MODE2_RAW":      "MODE2/2352",
}

// layoutCHD places the tracks on the disc. A stored pregap comes from the file, and
// one that isn't stored is silence the player makes up. Neither the padding nor
// anything else in the file takes up room on the disc.
func (d *Disc) layoutCHD(c *chdFile, tracks []chdTrack) {
	total := int(c.logical / chdFrameBytes)
	lba, frame := 0, 0
	graphics := false
	for _, t := range tracks {
		audio := t.kind == "AUDIO"
		mode := chdModes[t.kind]
		if mode == "" {
			mode = t.kind
		}
		tr := Track{Number: t.number, Audio: audio, Mode: mode, Pregap: lba, Session: 1}
		first := lba // where the file's frames for this track go
		if t.pregapStored {
			tr.Start = lba + min(t.pregap, t.frames)
		} else {
			if t.pregap > 0 {
				d.addExtent(extent{lba: lba, sectors: t.pregap, data: !audio})
			}
			tr.Start = lba + t.pregap
			first = tr.Start
		}
		if have := min(t.frames, total-frame); have > 0 {
			d.addExtent(extent{lba: first, sectors: have, src: c, offset: int64(frame) * chdFrameBytes, stride: chdFrameBytes, bigEndian: true, declared: true, data: !audio})
		}
		tr.End = first + t.frames
		if t.postgap > 0 {
			d.addExtent(extent{lba: tr.End, sectors: t.postgap, data: !audio})
			tr.End += t.postgap
		}
		d.Tracks = append(d.Tracks, tr)
		lba = tr.End
		frame += (t.frames + chdTrackPad - 1) / chdTrackPad * chdTrackPad
		graphics = graphics || t.subType != "" && t.subType != "NONE"
	}
	if graphics {
		d.sub = &subcode{src: chdGraphics{d, c}, stride: chdSubBytes, size: int64(MaxSectors) * chdSubBytes}
		d.HasGraphics = d.sub.looksLikeGraphics()
		if !d.HasGraphics {
			d.sub = nil
		}
	}
}

// chdGraphics reads the subcode of a disc's sectors as one long run, with sector n's
// at 96 times n. Nothing in the file lines up that simply, so it goes through the
// disc's own layout.
type chdGraphics struct {
	d *Disc
	c *chdFile
}

func (g chdGraphics) ReadAt(p []byte, off int64) (int, error) {
	done := 0
	for len(p) > 0 {
		lba, skip := int(off/chdSubBytes), int(off%chdSubBytes)
		n := min(len(p), chdSubBytes-skip)
		e := g.d.extentAt(lba)
		if e == nil || e.src == nil {
			clear(p[:n])
		} else if _, err := g.c.ReadAt(p[:n], e.offset+int64(lba-e.lba)*chdFrameBytes+SectorBytes+int64(skip)); err != nil && err != io.EOF {
			return done, err
		}
		p = p[n:]
		off += int64(n)
		done += n
	}
	return done, nil
}

// cutShort says whether the map points past the end of the file.
func (c *chdFile) cutShort() bool {
	for i := range c.entries {
		e := &c.entries[i]
		if e.kind == chdSelf || e.kind == chdZero {
			continue
		}
		if e.offset > uint64(c.size) || uint64(c.size)-e.offset < uint64(e.length) {
			return true
		}
	}
	return false
}

// readMap reads the map into entries, whichever form it is in.
func (c *chdFile) readMap() error {
	var err error
	if c.codecs[0] == 0 {
		err = c.readPlainMap()
	} else {
		err = c.readCompressedMap()
	}
	if err != nil {
		return err
	}
	for i := range c.entries {
		k := c.entries[i].kind
		if k == chdParent || k == chdZero && c.hasParent {
			return fmt.Errorf("%w: this CHD image only holds its changes from another image, called its parent, and WittyCD reads complete images", ErrUnsupported)
		}
		if k <= chdType3 && !chdKnownCodec(c.codecs[k]) {
			return fmt.Errorf("%w: this CHD image uses the %s compression, which WittyCD doesn't read. Running chdman's copy command with -c cdlz,cdzl,cdfl makes a file it can", ErrUnsupported, codecName(c.codecs[k]))
		}
	}
	n := max(chdMinCache, chdCacheBytes/c.hunkBytes)
	c.cache = make([]chdHunk, n)
	for i := range c.cache {
		c.cache[i].num = -1
	}
	return nil
}

func chdKnownCodec(id uint32) bool {
	switch id {
	case chdCDLZ, chdCDZL, chdCDFL, chdZLIB, chdLZMA, chdFLAC, chdHUFF:
		return true
	}
	return false
}

func codecName(id uint32) string {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], id)
	return strconv.Quote(printable(string(b[:])))
}

// readPlainMap reads a map of an image with no compression at all: a number for
// each hunk, counted in hunks from the start of the file, with 0 for one never written.
func (c *chdFile) readPlainMap() error {
	need := int64(c.hunks) * 4
	if need > c.size-c.mapOff {
		return fmt.Errorf("%w: this CHD image's map is cut short", ErrNotCD)
	}
	b := make([]byte, need)
	if _, err := c.r.ReadAt(b, c.mapOff); err != nil && err != io.EOF {
		return fmt.Errorf("can't read this CHD image: %w", err)
	}
	c.entries = make([]chdEntry, c.hunks)
	for i := range c.entries {
		v := binary.BigEndian.Uint32(b[i*4:])
		e := &c.entries[i]
		if v == 0 {
			e.kind = chdZero
			continue
		}
		e.kind = chdStored
		e.offset = uint64(v) * uint64(c.hunkBytes)
		e.length = uint32(c.hunkBytes)
	}
	return nil
}

// readCompressedMap reads the usual map: a little header, each hunk's type in
// Huffman codes with runs folded up, and then each hunk's details in as few bits as
// that hunk's type needs. A checksum over the lot says whether it came out right.
func (c *chdFile) readCompressedMap() error {
	var h [16]byte
	if _, err := c.r.ReadAt(h[:], c.mapOff); err != nil {
		if err == io.EOF {
			return fmt.Errorf("%w: this CHD image's map is cut short", ErrNotCD)
		}
		return fmt.Errorf("can't read this CHD image: %w", err)
	}
	mapBytes := binary.BigEndian.Uint32(h[0:])
	first := uint64(binary.BigEndian.Uint16(h[4:]))<<32 | uint64(binary.BigEndian.Uint32(h[6:]))
	want := binary.BigEndian.Uint16(h[10:])
	lengthBits, selfBits, parentBits := uint(h[12]), uint(h[13]), uint(h[14])
	if mapBytes > chdMaxMap || int64(mapBytes) > c.size-c.mapOff-16 || lengthBits > 24 || selfBits > 32 || parentBits > 48 {
		return fmt.Errorf("%w: this CHD image's map is damaged", ErrNotCD)
	}
	raw := make([]byte, mapBytes)
	if _, err := c.r.ReadAt(raw, c.mapOff+16); err != nil && err != io.EOF {
		return fmt.Errorf("can't read this CHD image: %w", err)
	}
	bad := fmt.Errorf("%w: this CHD image's map is damaged", ErrNotCD)
	br := &flacBits{b: raw}

	tree, ok := readHuffmanRLE(br)
	if !ok {
		return bad
	}
	c.entries = make([]chdEntry, c.hunks)
	var last uint8
	repeat := 0
	for i := range c.entries {
		if repeat > 0 {
			c.entries[i].kind = last
			repeat--
			continue
		}
		v, ok := tree.decode(br)
		if !ok {
			return bad
		}
		switch v {
		case chdRLESmall:
			n, ok := tree.decode(br)
			if !ok {
				return bad
			}
			c.entries[i].kind = last
			repeat = 2 + n
		case chdRLELarge:
			a, ok1 := tree.decode(br)
			b, ok2 := tree.decode(br)
			if !ok1 || !ok2 {
				return bad
			}
			c.entries[i].kind = last
			repeat = 2 + 16 + a<<4 + b
		default:
			last = uint8(v)
			c.entries[i].kind = last
		}
	}

	cur := first
	var lastSelf uint64
	var lastParent uint64
	check := uint16(0xffff)
	unitsPerHunk := uint64(c.hunkBytes / chdFrameBytes)
	var line [12]byte
	for i := range c.entries {
		e := &c.entries[i]
		e.offset = cur
		switch e.kind {
		case chdType0, chdType1, chdType2, chdType3:
			e.length = br.read(lengthBits)
			e.crc = uint16(br.read(16))
			cur += uint64(e.length)
		case chdPlain:
			e.length = uint32(c.hunkBytes)
			e.crc = uint16(br.read(16))
			cur += uint64(e.length)
		case chdSelf:
			lastSelf = uint64(br.read(selfBits))
			e.offset = lastSelf
		case chdParent:
			lastParent = readWide(br, parentBits)
			e.offset = lastParent
		case chdSelf1:
			lastSelf++
			e.kind, e.offset = chdSelf, lastSelf
		case chdSelf0:
			e.kind, e.offset = chdSelf, lastSelf
		case chdParentSame:
			lastParent = uint64(i) * unitsPerHunk
			e.kind, e.offset = chdParent, lastParent
		case chdParent1:
			lastParent += unitsPerHunk
			e.kind, e.offset = chdParent, lastParent
		case chdParent0:
			e.kind, e.offset = chdParent, lastParent
		default:
			return bad
		}
		// The checksum is over the map as a table of 12-byte lines.
		line[0] = e.kind
		line[1], line[2], line[3] = byte(e.length>>16), byte(e.length>>8), byte(e.length)
		line[4], line[5] = byte(e.offset>>40), byte(e.offset>>32)
		binary.BigEndian.PutUint32(line[6:], uint32(e.offset))
		binary.BigEndian.PutUint16(line[10:], e.crc)
		check = crcCHD(check, line[:])
	}
	if br.short || check != want {
		return bad
	}
	return nil
}

func readWide(br *flacBits, n uint) uint64 {
	if n > 32 {
		hi := uint64(br.read(n - 32))
		return hi<<32 | uint64(br.read(32))
	}
	return uint64(br.read(n))
}

var crcCHDTable = func() (t [256]uint16) {
	for i := range t {
		c := uint16(i) << 8
		for range 8 {
			if c&0x8000 != 0 {
				c = c<<1 ^ 0x1021
			} else {
				c <<= 1
			}
		}
		t[i] = c
	}
	return
}()

// crcCHD continues a CRC-16 of the kind CHD uses; a new one starts from 0xffff.
func crcCHD(c uint16, b []byte) uint16 {
	for _, v := range b {
		c = c<<8 ^ crcCHDTable[byte(c>>8)^v]
	}
	return c
}

// huffman decodes CHD's Huffman codes through a table of every possible next few
// bits. Each entry holds a symbol and its code's length.
type huffman struct {
	table []uint16
	bits  uint
}

// newHuffman makes the codes from each symbol's length. Codes go by length with
// the longest numbered first, and any set of lengths that doesn't fit is refused.
func newHuffman(lengths []uint8, maxBits uint) (*huffman, bool) {
	var count [33]uint32
	for _, l := range lengths {
		if uint(l) > maxBits {
			return nil, false
		}
		count[l]++
	}
	var next [33]uint32
	start := uint32(0)
	for l := 32; l > 0; l-- {
		sum := start + count[l]
		if l != 1 && sum&1 != 0 {
			return nil, false
		}
		next[l] = start
		start = sum >> 1
	}
	h := &huffman{table: make([]uint16, 1<<maxBits), bits: maxBits}
	for sym, l := range lengths {
		if l == 0 {
			continue
		}
		code := next[l]
		next[l]++
		from, to := uint64(code)<<(maxBits-uint(l)), uint64(code+1)<<(maxBits-uint(l))
		if to > uint64(len(h.table)) {
			return nil, false
		}
		v := uint16(sym)<<5 | uint16(l)
		for i := from; i < to; i++ {
			h.table[i] = v
		}
	}
	return h, true
}

// decode reads one symbol, and is false for a code that doesn't exist.
func (h *huffman) decode(br *flacBits) (int, bool) {
	v := h.table[br.peek(h.bits)]
	l := uint(v & 31)
	if l == 0 {
		return 0, false
	}
	br.skip(l)
	if br.short {
		return 0, false
	}
	return int(v >> 5), true
}

// readHuffmanRLE reads the 16-symbol code the map's types use. Its lengths come in
// 4 bits each: 1 is an escape, for a real 1 or a run of something else.
func readHuffmanRLE(br *flacBits) (*huffman, bool) {
	var lengths [16]uint8
	for i := 0; i < len(lengths); {
		v := br.read(4)
		if v != 1 {
			lengths[i] = uint8(v)
			i++
			continue
		}
		v = br.read(4)
		if v == 1 {
			lengths[i] = 1
			i++
			continue
		}
		run := int(br.read(4)) + 3
		if i+run > len(lengths) {
			return nil, false
		}
		for range run {
			lengths[i] = uint8(v)
			i++
		}
	}
	if br.short {
		return nil, false
	}
	return newHuffman(lengths[:], 8)
}

// readHuffmanTree reads the 256-symbol code of the "huff" compression, whose
// lengths are themselves in a small Huffman code.
func readHuffmanTree(br *flacBits) (*huffman, bool) {
	var small [24]uint8
	small[0] = uint8(br.read(3))
	start := int(br.read(3)) + 1
	count := uint32(0)
	for i := 1; i < len(small); i++ {
		if i < start || count == 7 {
			continue
		}
		count = br.read(3)
		if count != 7 {
			small[i] = uint8(count)
		}
	}
	tree, ok := newHuffman(small[:], 6)
	if !ok {
		return nil, false
	}
	var lengths [256]uint8
	last := uint8(0)
	for i := 0; i < len(lengths); {
		v, ok := tree.decode(br)
		if !ok {
			return nil, false
		}
		if v != 0 {
			last = uint8(v - 1)
			lengths[i] = last
			i++
			continue
		}
		run := int(br.read(3)) + 2
		if run == 7+2 {
			run += int(br.read(8))
		}
		for ; run > 0 && i < len(lengths); run-- {
			lengths[i] = last
			i++
		}
	}
	if br.short {
		return nil, false
	}
	return newHuffman(lengths[:], 16)
}

// ReadAt reads the frames as they are stored: big-endian audio and subcode. Damage
// reads as silence, and only a real failure to read the file comes back as an error.
func (c *chdFile) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("negative offset")
	}
	if off >= c.logical {
		return 0, io.EOF
	}
	short := false
	if rest := c.logical - off; int64(len(p)) > rest {
		p, short = p[:rest], true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	done := 0
	for len(p) > 0 {
		n := int(off / int64(c.hunkBytes))
		data, err := c.hunk(n)
		if err != nil {
			return done, err
		}
		m := copy(p, data[off%int64(c.hunkBytes):])
		p = p[m:]
		off += int64(m)
		done += m
	}
	if short {
		return done, io.EOF
	}
	return done, nil
}

// hunk returns a hunk's contents from the cache, reading it if it isn't there.
func (c *chdFile) hunk(n int) ([]byte, error) {
	slot := &c.cache[0]
	for i := range c.cache {
		h := &c.cache[i]
		if h.num == n {
			c.clock++
			h.used = c.clock
			return h.data, nil
		}
		if h.used < slot.used {
			slot = h
		}
	}
	if slot.data == nil {
		slot.data = make([]byte, c.hunkBytes)
	}
	slot.num = -1
	if err := c.decode(n, slot.data); err != nil {
		if !errors.Is(err, errCHDDamaged) {
			return nil, err
		}
		clear(slot.data)
	}
	c.clock++
	slot.num, slot.used = n, c.clock
	return slot.data, nil
}

// decode fills dst with hunk n.
func (c *chdFile) decode(n int, dst []byte) error {
	e := &c.entries[n]
	for hops := 0; e.kind == chdSelf; hops++ {
		if e.offset >= uint64(len(c.entries)) || hops >= chdMaxHops {
			return errCHDDamaged
		}
		e = &c.entries[e.offset]
	}
	checked := true
	switch e.kind {
	case chdZero:
		clear(dst)
		return nil
	case chdStored:
		checked = false
		fallthrough
	case chdPlain:
		if err := c.readBytes(dst, e.offset); err != nil {
			return err
		}
	case chdType0, chdType1, chdType2, chdType3:
		if e.offset > uint64(c.size) || uint64(c.size)-e.offset < uint64(e.length) {
			return errCHDDamaged // before any memory is set aside for it
		}
		if uint64(e.length) > uint64(len(c.comp)) {
			c.comp = make([]byte, e.length)
		}
		src := c.comp[:e.length]
		if err := c.readBytes(src, e.offset); err != nil {
			return err
		}
		ok, err := c.decompress(c.codecs[e.kind], src, dst)
		if err != nil {
			return err
		}
		checked = ok
	default:
		return errCHDDamaged
	}
	if checked && crcCHD(0xffff, dst) != e.crc {
		return errCHDDamaged
	}
	return nil
}

func (c *chdFile) readBytes(p []byte, off uint64) error {
	if off > uint64(c.size) || uint64(c.size)-off < uint64(len(p)) {
		return errCHDDamaged
	}
	if _, err := c.r.ReadAt(p, int64(off)); err != nil {
		if err == io.EOF {
			return errCHDDamaged
		}
		return err
	}
	return nil
}

// decompress undoes one hunk's compression. It says whether the hunk's checksum
// can be tested afterwards.
func (c *chdFile) decompress(codec uint32, src, dst []byte) (bool, error) {
	switch codec {
	case chdCDLZ, chdCDZL:
		return c.decompressCD(codec, src, dst)
	case chdCDFL:
		return true, c.decompressCDFL(src, dst)
	case chdZLIB:
		return true, c.inflateTo(dst, src)
	case chdLZMA:
		if c.lz.decode(dst, src, 3, 0, 2) != nil {
			return false, errCHDDamaged
		}
		return true, nil
	case chdFLAC:
		// A first letter says which way round the samples were taken.
		if len(src) == 0 || (src[0] != 'L' && src[0] != 'B') || len(dst)%SampleBytes != 0 {
			return false, errCHDDamaged
		}
		_, err := c.flacInto(dst, src[1:], src[0] == 'B')
		return true, err
	case chdHUFF:
		br := &flacBits{b: src}
		tree, ok := readHuffmanTree(br)
		if !ok {
			return false, errCHDDamaged
		}
		for i := range dst {
			v, ok := tree.decode(br)
			if !ok {
				return false, errCHDDamaged
			}
			dst[i] = byte(v)
		}
		return true, nil
	}
	return false, errCHDDamaged
}

// frameBuffers makes room for a hunk's sectors and subcode, which are compressed
// separately and put back together afterwards.
func (c *chdFile) frameBuffers(frames int) ([]byte, []byte) {
	if cap(c.sectors) < frames*SectorBytes {
		c.sectors = make([]byte, frames*SectorBytes)
		c.sub = make([]byte, frames*chdSubBytes)
	}
	return c.sectors[:frames*SectorBytes], c.sub[:frames*chdSubBytes]
}

func joinFrames(dst, sectors, sub []byte) {
	frames := len(sectors) / SectorBytes
	for i := range frames {
		copy(dst[i*chdFrameBytes:], sectors[i*SectorBytes:(i+1)*SectorBytes])
		copy(dst[i*chdFrameBytes+SectorBytes:], sub[i*chdSubBytes:(i+1)*chdSubBytes])
	}
}

// decompressCD handles the two CD forms laid out the same way: a bitmap with a bit
// per frame, the length of the compressed sectors, the sectors, then the subcode.
//
// A set bit marks a data sector whose sync and error-correction bytes were taken
// out because they can be worked out again. Data is never played, so they aren't
// worked out again, and the hunk's checksum can't then be tested. Audio is
// never marked.
func (c *chdFile) decompressCD(codec uint32, src, dst []byte) (bool, error) {
	frames := len(dst) / chdFrameBytes
	bitmap := (frames + 7) / 8
	lenBytes := 2
	if len(dst) >= 1<<16 {
		lenBytes = 3
	}
	head := bitmap + lenBytes
	if len(src) < head {
		return false, errCHDDamaged
	}
	base := int(src[bitmap])<<8 | int(src[bitmap+1])
	if lenBytes == 3 {
		base = base<<8 | int(src[bitmap+2])
	}
	if base > len(src)-head {
		return false, errCHDDamaged
	}
	sectors, sub := c.frameBuffers(frames)
	var err error
	if codec == chdCDLZ {
		if c.lz.decode(sectors, src[head:head+base], 3, 0, 2) != nil {
			err = errCHDDamaged
		}
	} else {
		err = c.inflateTo(sectors, src[head:head+base])
	}
	if err == nil {
		err = c.inflateTo(sub, src[head+base:])
	}
	if err != nil {
		return false, err
	}
	joinFrames(dst, sectors, sub)
	for _, b := range src[:bitmap] {
		if b != 0 {
			return false, nil
		}
	}
	return true, nil
}

// decompressCDFL handles sectors as FLAC, with the subcode straight after the last
// frame. Which is why the decoder has to say where a frame stops.
func (c *chdFile) decompressCDFL(src, dst []byte) error {
	sectors, sub := c.frameBuffers(len(dst) / chdFrameBytes)
	used, err := c.flacInto(sectors, src, true)
	if err != nil {
		return err
	}
	if err := c.inflateTo(sub, src[used:]); err != nil {
		return err
	}
	joinFrames(dst, sectors, sub)
	return nil
}

// flacInto decodes FLAC frames until dst is full, and says how many bytes they took.
func (c *chdFile) flacInto(dst, src []byte, bigEndian bool) (int, error) {
	at := 0
	for len(dst) > 0 {
		h, n, err := decodeFlacFrame(src[at:], &c.flac)
		if err != nil {
			return 0, errCHDDamaged
		}
		m := min(h.blocks, len(dst)/SampleBytes)
		l, r := c.flac.l[:m], c.flac.r[:m]
		for i := range l {
			if bigEndian {
				binary.BigEndian.PutUint16(dst[i*4:], uint16(l[i]))
				binary.BigEndian.PutUint16(dst[i*4+2:], uint16(r[i]))
			} else {
				binary.LittleEndian.PutUint16(dst[i*4:], uint16(l[i]))
				binary.LittleEndian.PutUint16(dst[i*4+2:], uint16(r[i]))
			}
		}
		dst = dst[m*SampleBytes:]
		at += n
		if m == 0 {
			break
		}
	}
	return at, nil
}

// inflateTo fills dst from a deflate stream. Asking for no more than dst holds is
// what stops a small hostile stream from growing without limit.
func (c *chdFile) inflateTo(dst, src []byte) error {
	c.src.Reset(src)
	if c.inflate == nil {
		c.inflate = flate.NewReader(&c.src)
	} else if err := c.inflate.(flate.Resetter).Reset(&c.src, nil); err != nil {
		return errCHDDamaged
	}
	if _, err := io.ReadFull(c.inflate, dst); err != nil {
		return errCHDDamaged
	}
	return nil
}
