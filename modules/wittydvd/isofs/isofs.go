// Package isofs reads files out of optical disc images (ISO 9660 and UDF)
// without mounting them. It is part of WittyDVD. DVD-Video discs are UDF/ISO 9660 "bridge" discs, so
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
	"sync"
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
type Disc struct {
	Path   string
	Format string // "udf", "iso9660"
	Label  string

	f     *os.File
	size  int64
	files map[string]*File // upper-cased path -> file
	mu    sync.Mutex
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
	n := f.Size
	if max > 0 && n > max {
		n = max
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
	if err := d.indexUDF(); err == nil && len(d.files) > 0 {
		d.Format = "udf"
		return nil
	}
	d.files = map[string]*File{}
	if err := d.indexISO9660(); err != nil {
		return fmt.Errorf("isofs: not a readable UDF or ISO 9660 image: %w", err)
	}
	d.Format = "iso9660"
	return nil
}

func (d *Disc) add(f *File) {
	f.Path = strings.Trim(f.Path, "/")
	d.files[strings.ToUpper(f.Path)] = f
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
				d.Label = strings.TrimSpace(string(b[40:72]))
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
// UDF (ECMA-167 / OSTA UDF 1.02+ as used by DVD-Video)
// ---------------------------------------------------------------------------

type udfPartition struct {
	number uint16
	start  int64 // in sectors
	length int64
}

type udfCtx struct {
	parts  []udfPartition
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
			// Partition maps: we only support type 1 (physical) maps which is
			// all DVD-Video uses; the partition reference number is the index
			// into the map list which for type-1 maps equals the partition number.
		case 8: // Terminating descriptor
			i = 1 << 20
		}
	}
	if !gotLVD || len(ctx.parts) == 0 {
		return errors.New("UDF volume descriptors incomplete")
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

func (c *udfCtx) abs(partRef uint16, block uint32) (int64, error) {
	for _, p := range c.parts {
		if p.number == partRef || len(c.parts) == 1 {
			return p.start + int64(block), nil
		}
	}
	return 0, fmt.Errorf("UDF partition %d not found", partRef)
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
		adStart  int
		partRef  = icb.part
	)
	switch tag {
	case 261: // File Entry
		fileType = b[16+11]
		flags = binary.LittleEndian.Uint16(b[16+18 : 16+20])
		infoLen = int64(binary.LittleEndian.Uint64(b[56:64]))
		eaLen = binary.LittleEndian.Uint32(b[168:172])
		adLen = binary.LittleEndian.Uint32(b[172:176])
		adStart = 176 + int(eaLen)
	case 266: // Extended File Entry
		fileType = b[16+11]
		flags = binary.LittleEndian.Uint16(b[16+18 : 16+20])
		infoLen = int64(binary.LittleEndian.Uint64(b[56:64]))
		eaLen = binary.LittleEndian.Uint32(b[208:212])
		adLen = binary.LittleEndian.Uint32(b[212:216])
		adStart = 216 + int(eaLen)
	default:
		return nil, fmt.Errorf("UDF: unexpected tag %d at sector %d", tag, sec)
	}
	e := &udfEntry{isDir: fileType == 4, size: infoLen}
	if adStart+int(adLen) > len(b) {
		return nil, errors.New("UDF: allocation descriptors overrun")
	}
	ads := b[adStart : adStart+int(adLen)]
	switch flags & 7 {
	case 0: // short_ad
		for i := 0; i+8 <= len(ads); i += 8 {
			l := binary.LittleEndian.Uint32(ads[i : i+4])
			if l == 0 {
				break
			}
			typ := l >> 30
			l &= 0x3fffffff
			if typ != 0 && typ != 1 { // 0 = recorded, 1 = allocated not recorded (zeros); 3 = chain (unsupported)
				continue
			}
			pos := binary.LittleEndian.Uint32(ads[i+4 : i+8])
			s, err := ctx.abs(partRef, pos)
			if err != nil {
				return nil, err
			}
			e.extents = append(e.extents, Extent{Offset: s * Sector, Length: int64(l)})
		}
	case 1: // long_ad
		for i := 0; i+16 <= len(ads); i += 16 {
			raw := binary.LittleEndian.Uint32(ads[i : i+4])
			if raw == 0 {
				break
			}
			typ := raw >> 30
			ad := readLongAD(ads[i : i+16])
			if typ != 0 && typ != 1 {
				continue
			}
			s, err := ctx.abs(ad.part, ad.block)
			if err != nil {
				return nil, err
			}
			e.extents = append(e.extents, Extent{Offset: s * Sector, Length: int64(ad.length)})
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
	var data []byte
	if ent.inline != nil {
		data = ent.inline
	} else {
		if ent.size > 16<<20 {
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
