// Package isofs reads files out of optical disc images (ISO 9660 and UDF)
// without mounting them. It is WittyDisc, shared by WittyDVD and WittyBluray. DVD-Video discs are UDF/ISO 9660 "bridge" discs, so
// UDF is tried first and ISO 9660 is the fallback.
//
// Files are described as byte extents inside the image. That lets callers hand
// the raw VOB ranges to FFmpeg (via the subfile protocol) or read them directly.
package isofs

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
)

const Sector = 2048

// Extent is a contiguous byte range inside the image file.
type Extent struct {
	Offset int64 `json:"offset"`
	Length int64 `json:"length"`
}

// File is a file found on the disc.
type File struct {
	Path    string   `json:"path"` // forward-slash path, e.g. VIDEO_TS/VTS_01_0.IFO
	Size    int64    `json:"size"`
	Extents []Extent `json:"extents"`
	IsDir   bool     `json:"isDir,omitempty"`
}

// Disc is an opened disc image.
// image is where a disc's bytes come from.
type image interface {
	io.ReaderAt
	io.Closer
}

type Disc struct {
	Path   string
	Format string // "udf", "iso9660"
	Label  string

	f     image
	size  int64
	files map[string]*File // upper-cased path -> file

	walked   map[int64]bool // directories already read while indexing
	dirBytes int64
	entries  int
}

var ErrNotFound = errors.New("isofs: file not found")

// Open opens an image file and indexes its directory tree.
func Open(p string) (*Disc, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	d := &Disc{Path: p, f: f, size: st.Size(), files: map[string]*File{}}
	if err := d.index(); err != nil {
		f.Close()
		return nil, err
	}
	return d, nil
}

func (d *Disc) Close() error { return d.f.Close() }

// Size is the image size in bytes.
func (d *Disc) Size() int64 { return d.size }

// Files returns all files on the disc sorted by path.
func (d *Disc) Files() []*File {
	out := make([]*File, 0, len(d.files))
	for _, f := range d.files {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Find returns a file by case-insensitive path.
func (d *Disc) Find(p string) (*File, bool) {
	f, ok := d.files[strings.ToUpper(strings.Trim(path.Clean("/"+p), "/"))]
	return f, ok
}

// ReadFile reads a whole file (bounded by max bytes; 0 = unlimited).
func (d *Disc) ReadFile(p string, max int64) ([]byte, error) {
	f, ok := d.Find(p)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, p)
	}
	// A file's size is whatever the image says. Nothing can be longer than the
	// image itself, so that is all the memory a read may ask for.
	n := min(f.Size, d.size)
	if max > 0 && n > max {
		n = max
	}
	if n < 0 {
		return nil, fmt.Errorf("isofs: %s has a negative size", p)
	}
	// Sizes come from the image: nothing bigger than the image itself can be
	// in it, whatever a directory entry says.
	if n > d.size {
		n = d.size
	}
	buf := make([]byte, n)
	got, err := d.ReadAtFile(f, buf, 0)
	return buf[:got], err
}

// ReadAtFile reads from a file at a logical offset, spanning extents as needed.
func (d *Disc) ReadAtFile(f *File, buf []byte, off int64) (int, error) {
	total := 0
	for _, e := range f.Extents {
		if off >= e.Length {
			off -= e.Length
			continue
		}
		want := int64(len(buf) - total)
		avail := e.Length - off
		if want > avail {
			want = avail
		}
		n, err := d.f.ReadAt(buf[total:total+int(want)], e.Offset+off)
		total += n
		if err != nil && !errors.Is(err, io.EOF) {
			return total, err
		}
		off = 0
		if total >= len(buf) {
			break
		}
	}
	if total < len(buf) {
		return total, io.EOF
	}
	return total, nil
}

// ReadSectors reads raw sectors from the image (absolute sector numbers).
func (d *Disc) ReadSectors(first, count int64) ([]byte, error) {
	buf := make([]byte, count*Sector)
	n, err := d.f.ReadAt(buf, first*Sector)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return buf[:n], nil
}

// Reader returns an io.ReaderAt over a file's logical bytes.
func (d *Disc) Reader(f *File) io.ReaderAt { return &fileReader{d: d, f: f} }

type fileReader struct {
	d *Disc
	f *File
}

func (r *fileReader) ReadAt(p []byte, off int64) (int, error) { return r.d.ReadAtFile(r.f, p, off) }

func (d *Disc) index() error {
	udfErr := d.indexUDF()
	if udfErr == nil && len(d.files) > 0 {
		d.Format = "udf"
		return nil
	}
	if udfErr == nil {
		udfErr = errors.New("no files")
	}
	d.files = map[string]*File{}
	d.walked, d.dirBytes, d.entries = nil, 0, 0
	if err := d.indexISO9660(); err != nil {
		return fmt.Errorf("isofs: not a readable UDF (%v) or ISO 9660 (%v) image", udfErr, err)
	}
	d.Format = "iso9660"
	return nil
}

func (d *Disc) add(f *File) {
	f.Path = strings.Trim(f.Path, "/")
	d.files[strings.ToUpper(f.Path)] = f
}

// A real disc holds hundreds of files. These bound what a crafted image can
// make the walk do: directories that list each other are read once, not once
// per path through them, which from a tiny file would never end.
const (
	maxFiles    = 200000
	maxDirBytes = 256 << 20 // directory data read in total
	maxEntries  = 1 << 20   // directory entries read in total
)

// enterDir says whether to read a directory at this place in the image.
func (d *Disc) enterDir(at int64, size int64) error {
	if d.walked == nil {
		d.walked = map[int64]bool{}
	}
	if d.walked[at] {
		return errSeen
	}
	d.walked[at] = true
	d.dirBytes += size
	if d.dirBytes > maxDirBytes || len(d.files) > maxFiles {
		return errors.New("isofs: the directory tree is too large")
	}
	return nil
}

var errSeen = errors.New("isofs: directory already read")

// nextEntry counts a directory entry. A UDF entry means reading another
// sector, so a few megabytes of crafted directory could otherwise keep a disk
// busy for minutes.
func (d *Disc) nextEntry() error {
	d.entries++
	if d.entries > maxEntries {
		return errors.New("isofs: the directory tree is too large")
	}
	return nil
}

func (d *Disc) sector(n int64) ([]byte, error) {
	if n < 0 || n*Sector >= d.size {
		return nil, fmt.Errorf("isofs: sector %d outside image", n)
	}
	b := make([]byte, Sector)
	if _, err := d.f.ReadAt(b, n*Sector); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return b, nil
}

// ---------------------------------------------------------------------------
// ISO 9660
// ---------------------------------------------------------------------------

func (d *Disc) indexISO9660() error {
	var root []byte
	for s := int64(16); s < 64; s++ {
		b, err := d.sector(s)
		if err != nil {
			return err
		}
		if string(b[1:6]) != "CD001" {
			return errors.New("no primary volume descriptor")
		}
		switch b[0] {
		case 1:
			if root == nil {
				root = append([]byte(nil), b[156:190]...)
				d.Label = strings.Trim(string(b[40:72]), " \x00")
			}
		case 2:
			// Joliet supplementary descriptor - prefer it for long names, but
			// DVD-Video names are 8.3 so ISO level is fine. Skip.
		case 255:
			if root == nil {
				return errors.New("no primary volume descriptor")
			}
			return d.walkISODir(root, "", 0)
		}
	}
	return errors.New("volume descriptor set terminator not found")
}

func (d *Disc) walkISODir(rec []byte, prefix string, depth int) error {
	if depth > 16 {
		return nil
	}
	lba := int64(binary.LittleEndian.Uint32(rec[2:6]))
	size := int64(binary.LittleEndian.Uint32(rec[10:14]))
	if size <= 0 || size > 64<<20 {
		return nil
	}
	if err := d.enterDir(lba, size); errors.Is(err, errSeen) {
		return nil
	} else if err != nil {
		return err
	}
	buf := make([]byte, size)
	if _, err := d.f.ReadAt(buf, lba*Sector); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	for off := 0; off < len(buf); {
		l := int(buf[off])
		if l == 0 {
			// Records never span sectors; skip to next sector boundary.
			off = (off/Sector + 1) * Sector
			continue
		}
		if off+l > len(buf) || l < 33 {
			break
		}
		if err := d.nextEntry(); err != nil {
			return err
		}
		r := buf[off : off+l]
		nameLen := int(r[32])
		if 33+nameLen > l {
			break
		}
		name := string(r[33 : 33+nameLen])
		flags := r[25]
		off += l
		if nameLen == 1 && (name == "\x00" || name == "\x01") {
			continue
		}
		if i := strings.IndexByte(name, ';'); i >= 0 {
			name = name[:i]
		}
		name = strings.TrimSuffix(name, ".")
		full := prefix + "/" + name
		if flags&2 != 0 {
			d.add(&File{Path: full, IsDir: true})
			if err := d.walkISODir(r, full, depth+1); err != nil {
				return err
			}
			continue
		}
		fl := int64(binary.LittleEndian.Uint32(r[2:6]))
		fs := int64(binary.LittleEndian.Uint32(r[10:14]))
		f := &File{Path: full, Size: fs}
		if fs > 0 {
			f.Extents = []Extent{{Offset: fl * Sector, Length: fs}}
		}
		// Multi-extent files (>4GiB) use the same name in consecutive records
		// with the "not final" flag; merge them.
		key := strings.ToUpper(strings.Trim(full, "/"))
		if prev, ok := d.files[key]; ok && !prev.IsDir {
			prev.Size += fs
			prev.Extents = append(prev.Extents, f.Extents...)
			continue
		}
		d.add(f)
	}
	return nil
}

// ---------------------------------------------------------------------------
// UDF (ECMA-167 / OSTA UDF 1.02 as used by DVD-Video, and UDF 2.50 with a
// metadata partition as used by Blu-ray)
// ---------------------------------------------------------------------------

type udfPartition struct {
	number uint16
	start  int64 // in sectors
	length int64
}

// udfMap is one partition map of the logical volume. Partition reference
// numbers in long_ads index the map list. A type 1 map is a physical
// partition. A type 2 "metadata partition" (UDF 2.50, Blu-ray) is a file
// whose extents hold the directory structures, so its blocks are looked up
// in that file.
type udfMap struct {
	typ        byte
	partition  uint16 // the physical partition number
	metaFile   uint32 // type 2: the metadata file's block in that partition
	metaMirror uint32 // type 2: its mirror, used when the main copy is unreadable
	extents    []Extent
}

type udfCtx struct {
	parts  []udfPartition
	maps   []udfMap
	fsdLoc longAD
}

type longAD struct {
	length uint32
	block  uint32
	part   uint16
}

func readLongAD(b []byte) longAD {
	return longAD{
		length: binary.LittleEndian.Uint32(b[0:4]) & 0x3fffffff,
		block:  binary.LittleEndian.Uint32(b[4:8]),
		part:   binary.LittleEndian.Uint16(b[8:10]),
	}
}

func (d *Disc) indexUDF() error {
	// Anchor volume descriptor pointer at sector 256 (or N-1 / N-257).
	var mvds []byte
	for _, s := range []int64{256, d.size/Sector - 1, d.size/Sector - 257} {
		b, err := d.sector(s)
		if err != nil {
			continue
		}
		if binary.LittleEndian.Uint16(b[0:2]) == 2 { // AVDP
			mvds = b[16:24]
			break
		}
	}
	if mvds == nil {
		return errors.New("no UDF anchor")
	}
	mvdsLen := int64(binary.LittleEndian.Uint32(mvds[0:4]))
	mvdsLoc := int64(binary.LittleEndian.Uint32(mvds[4:8]))
	ctx := &udfCtx{}
	gotLVD := false
	for i := int64(0); i < mvdsLen/Sector && i < 64; i++ {
		b, err := d.sector(mvdsLoc + i)
		if err != nil {
			break
		}
		tag := binary.LittleEndian.Uint16(b[0:2])
		switch tag {
		case 1: // Primary volume descriptor
			d.Label = decodeDString(b[24:56])
		case 5: // Partition descriptor
			ctx.parts = append(ctx.parts, udfPartition{
				number: binary.LittleEndian.Uint16(b[22:24]),
				start:  int64(binary.LittleEndian.Uint32(b[188:192])),
				length: int64(binary.LittleEndian.Uint32(b[192:196])),
			})
		case 6: // Logical volume descriptor
			// Logical volume contents use is a long_ad to the file set descriptor.
			ctx.fsdLoc = readLongAD(b[248:264])
			gotLVD = true
			// Partition maps: type 1 (physical, all DVD-Video uses) and type 2
			// metadata partitions (Blu-ray). Sparable and virtual partitions
			// belong to rewritable media and are not something a video disc
			// image uses; they are left unresolved.
			mapsLen := int(binary.LittleEndian.Uint32(b[264:268]))
			nMaps := int(binary.LittleEndian.Uint32(b[268:272]))
			for off, n := 440, 0; n < nMaps && n < 16 && off+2 <= len(b) && off < 440+mapsLen; n++ {
				typ, l := b[off], int(b[off+1])
				if l < 2 || off+l > len(b) {
					break
				}
				m := udfMap{typ: typ}
				switch {
				case typ == 1 && l >= 6:
					m.partition = binary.LittleEndian.Uint16(b[off+4 : off+6])
				case typ == 2 && l >= 64 && strings.HasPrefix(string(b[off+5:off+28]), "*UDF Metadata Partition"):
					m.partition = binary.LittleEndian.Uint16(b[off+38 : off+40])
					m.metaFile = binary.LittleEndian.Uint32(b[off+40 : off+44])
					m.metaMirror = binary.LittleEndian.Uint32(b[off+44 : off+48])
				default:
					m.typ = 0
				}
				ctx.maps = append(ctx.maps, m)
				off += l
			}
		case 8: // Terminating descriptor
			i = 1 << 20
		}
	}
	if !gotLVD || len(ctx.parts) == 0 {
		return errors.New("UDF volume descriptors incomplete")
	}
	// Metadata partitions: read the file that holds their blocks (the mirror
	// if the main copy is unreadable) before anything on them is looked up.
	for i := range ctx.maps {
		m := &ctx.maps[i]
		if m.typ != 2 {
			continue
		}
		physRef := uint16(0)
		for j, pm := range ctx.maps {
			if pm.typ == 1 && pm.partition == m.partition {
				physRef = uint16(j)
			}
		}
		var err error
		for _, loc := range []uint32{m.metaFile, m.metaMirror} {
			var fe *udfEntry
			if fe, err = d.readUDFEntry(ctx, longAD{block: loc, part: physRef}); err == nil && len(fe.extents) > 0 {
				m.extents = fe.extents
				break
			}
			if err == nil {
				err = errors.New("empty metadata file")
			}
		}
		if err != nil {
			return fmt.Errorf("UDF metadata partition: %w", err)
		}
	}
	// File set descriptor
	fsdSec, err := ctx.abs(ctx.fsdLoc.part, ctx.fsdLoc.block)
	if err != nil {
		return err
	}
	fsd, err := d.sector(fsdSec)
	if err != nil {
		return err
	}
	if binary.LittleEndian.Uint16(fsd[0:2]) != 256 {
		return errors.New("UDF file set descriptor missing")
	}
	rootICB := readLongAD(fsd[400:416])
	return d.walkUDFDir(ctx, rootICB, "", 0)
}

func (c *udfCtx) physical(number uint16) (*udfPartition, error) {
	for i := range c.parts {
		if c.parts[i].number == number || len(c.parts) == 1 {
			return &c.parts[i], nil
		}
	}
	return nil, fmt.Errorf("UDF partition %d not found", number)
}

// abs turns a partition reference number and a logical block into an
// absolute sector of the image.
func (c *udfCtx) abs(partRef uint16, block uint32) (int64, error) {
	if int(partRef) >= len(c.maps) {
		// No usable map table: reference numbers equal partition numbers.
		p, err := c.physical(partRef)
		if err != nil {
			return 0, err
		}
		return p.start + int64(block), nil
	}
	m := &c.maps[partRef]
	if m.typ == 2 {
		off := int64(block) * Sector
		for _, e := range m.extents {
			if off < e.Length {
				return (e.Offset + off) / Sector, nil
			}
			off -= e.Length
		}
		return 0, fmt.Errorf("UDF: block %d outside the metadata partition", block)
	}
	if m.typ != 1 {
		return 0, fmt.Errorf("UDF: partition map %d is of an unsupported type", partRef)
	}
	p, err := c.physical(m.partition)
	if err != nil {
		return 0, err
	}
	return p.start + int64(block), nil
}

// udfExtents collects a file's extents from its allocation descriptors,
// following "next extent of descriptors" entries (type 3) into further
// descriptor blocks, as large files have.
func (d *Disc) udfExtents(ctx *udfCtx, e *udfEntry, ads []byte, long bool, partRef uint16, depth int) error {
	if depth > 64 {
		return errors.New("UDF: allocation descriptor chain too long")
	}
	size := 8
	if long {
		size = 16
	}
	for i := 0; i+size <= len(ads); i += size {
		raw := binary.LittleEndian.Uint32(ads[i : i+4])
		if raw == 0 {
			break
		}
		typ, l := raw>>30, raw&0x3fffffff
		pos, part := uint32(0), partRef
		if long {
			ad := readLongAD(ads[i : i+16])
			pos, part = ad.block, ad.part
		} else {
			pos = binary.LittleEndian.Uint32(ads[i+4 : i+8])
		}
		s, err := ctx.abs(part, pos)
		if err != nil {
			return err
		}
		switch typ {
		case 0, 1: // recorded, or allocated but not recorded (reads as zeros)
			e.extents = append(e.extents, Extent{Offset: s * Sector, Length: int64(l)})
		case 3: // the next block of allocation descriptors
			b, err := d.sector(s)
			if err != nil {
				return err
			}
			if binary.LittleEndian.Uint16(b[0:2]) != 258 {
				return errors.New("UDF: bad allocation extent descriptor")
			}
			n := int(binary.LittleEndian.Uint32(b[20:24]))
			if 24+n > len(b) {
				return errors.New("UDF: allocation extent descriptor overrun")
			}
			return d.udfExtents(ctx, e, b[24:24+n], long, part, depth+1)
		}
	}
	return nil
}

// udfEntry is a parsed (extended) file entry.
type udfEntry struct {
	isDir   bool
	size    int64
	extents []Extent
	inline  []byte // data embedded in the ICB (allocation type 3)
}

func (d *Disc) readUDFEntry(ctx *udfCtx, icb longAD) (*udfEntry, error) {
	sec, err := ctx.abs(icb.part, icb.block)
	if err != nil {
		return nil, err
	}
	b, err := d.sector(sec)
	if err != nil {
		return nil, err
	}
	tag := binary.LittleEndian.Uint16(b[0:2])
	var (
		fileType byte
		flags    uint16
		infoLen  int64
		eaLen    uint32
		adLen    uint32
		adStart  int64
		partRef  = icb.part
	)
	switch tag {
	case 261: // File Entry
		fileType = b[16+11]
		flags = binary.LittleEndian.Uint16(b[16+18 : 16+20])
		infoLen = int64(binary.LittleEndian.Uint64(b[56:64]))
		eaLen = binary.LittleEndian.Uint32(b[168:172])
		adLen = binary.LittleEndian.Uint32(b[172:176])
		adStart = 176 + int64(eaLen)
	case 266: // Extended File Entry
		fileType = b[16+11]
		flags = binary.LittleEndian.Uint16(b[16+18 : 16+20])
		infoLen = int64(binary.LittleEndian.Uint64(b[56:64]))
		eaLen = binary.LittleEndian.Uint32(b[208:212])
		adLen = binary.LittleEndian.Uint32(b[212:216])
		adStart = 216 + int64(eaLen)
	default:
		return nil, fmt.Errorf("UDF: unexpected tag %d at sector %d", tag, sec)
	}
	e := &udfEntry{isDir: fileType == 4, size: infoLen}
	if adStart+int64(adLen) > int64(len(b)) {
		return nil, errors.New("UDF: allocation descriptors overrun")
	}
	ads := b[adStart : adStart+int64(adLen)]
	switch flags & 7 {
	case 0, 1: // short_ad or long_ad
		if err := d.udfExtents(ctx, e, ads, flags&7 == 1, partRef, 0); err != nil {
			return nil, err
		}
	case 3: // embedded
		e.inline = append([]byte(nil), ads...)
	default:
		return nil, errors.New("UDF: extended allocation descriptors unsupported")
	}
	return e, nil
}

func (d *Disc) walkUDFDir(ctx *udfCtx, icb longAD, prefix string, depth int) error {
	if depth > 16 {
		return nil
	}
	ent, err := d.readUDFEntry(ctx, icb)
	if err != nil {
		return err
	}
	if ent.size < 0 || ent.size > 16<<20 {
		return errors.New("UDF: directory too large")
	}
	if err := d.enterDir(int64(icb.part)<<32|int64(icb.block), ent.size); errors.Is(err, errSeen) {
		return nil
	} else if err != nil {
		return err
	}
	var data []byte
	if ent.inline != nil {
		data = ent.inline
	} else {
		if ent.size < 0 || ent.size > 16<<20 {
			return errors.New("UDF: directory too large")
		}
		data = make([]byte, ent.size)
		tmp := &File{Size: ent.size, Extents: ent.extents}
		if _, err := d.ReadAtFile(tmp, data, 0); err != nil && !errors.Is(err, io.EOF) {
			return err
		}
	}
	for off := 0; off+38 <= len(data); {
		if binary.LittleEndian.Uint16(data[off:off+2]) != 257 { // FID
			break
		}
		if err := d.nextEntry(); err != nil {
			return err
		}
		chars := data[off+18]
		fidICB := readLongAD(data[off+20 : off+36])
		implLen := int(binary.LittleEndian.Uint16(data[off+36 : off+38]))
		nameLen := int(data[off+19])
		nameOff := off + 38 + implLen
		if nameOff+nameLen > len(data) {
			break
		}
		name := decodeDChars(data[nameOff : nameOff+nameLen])
		total := 38 + implLen + nameLen
		total = (total + 3) &^ 3
		off += total
		if chars&8 != 0 || chars&4 != 0 { // parent or deleted
			continue
		}
		if name == "" {
			continue
		}
		full := prefix + "/" + name
		if chars&2 != 0 { // directory
			d.add(&File{Path: full, IsDir: true})
			if err := d.walkUDFDir(ctx, fidICB, full, depth+1); err != nil {
				return err
			}
			continue
		}
		fe, err := d.readUDFEntry(ctx, fidICB)
		if err != nil {
			continue
		}
		f := &File{Path: full, Size: fe.size, Extents: fe.extents}
		if fe.inline != nil {
			// Tiny embedded file: expose it via a synthetic extent at the ICB.
			sec, _ := ctx.abs(fidICB.part, fidICB.block)
			f.Extents = []Extent{{Offset: sec*Sector + int64(Sector-len(fe.inline)), Length: int64(len(fe.inline))}}
		}
		d.add(f)
	}
	return nil
}

// decodeDChars decodes an OSTA compressed unicode string (first byte is the
// compression ID: 8 = latin1, 16 = UTF-16BE).
func decodeDChars(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	switch b[0] {
	case 8:
		return string(b[1:])
	case 16:
		var sb strings.Builder
		for i := 1; i+1 < len(b); i += 2 {
			sb.WriteRune(rune(binary.BigEndian.Uint16(b[i : i+2])))
		}
		return sb.String()
	}
	return string(b)
}

// decodeDString decodes a fixed-length dstring (last byte is the length).
func decodeDString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	l := int(b[len(b)-1])
	if l == 0 || l > len(b)-1 {
		return ""
	}
	return strings.TrimRight(decodeDChars(b[:l]), "\x00 ")
}
