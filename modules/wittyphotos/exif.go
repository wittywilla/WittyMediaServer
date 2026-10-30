// Package wittyphotos reads what a photo says about itself without decoding
// it: EXIF (when, with what, where), the picture's dimensions and the JPEG
// preview a camera RAW file carries. It is pure Go with no dependencies, reads
// only a bounded part of each file, and treats every file as untrusted: a
// malformed file gives an error or empty fields, never a panic or a large
// allocation.
package wittyphotos

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// headLimit is how much of a file is searched for EXIF. Cameras and phones put
// it at the start; 4 MB also covers HEIC files with a large item table.
const headLimit = 4 << 20

// ErrNoExif means the file has no EXIF block.
var ErrNoExif = errors.New("no EXIF data")

// Info is what a photo's EXIF says. Zero values mean the file did not say.
type Info struct {
	Taken       time.Time // when the picture was taken, local to the camera unless Offset is set
	HasOffset   bool      // Taken carries a real UTC offset (OffsetTimeOriginal)
	Make        string
	Model       string
	Lens        string
	FNumber     float64
	Exposure    string // "1/250" or "2.5"
	ISO         int
	FocalMM     float64
	Orientation int // 1 to 8; 0 when not set
	Width       int // as stored, before Orientation
	Height      int
	HasGPS      bool
	Lat         float64
	Lon         float64
	Alt         float64
}

// ReadFile reads the EXIF of a photo, whatever its container.
func ReadFile(path string) (*Info, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	head := make([]byte, headLimit)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return Parse(head[:n])
}

// Parse reads EXIF from the first bytes of a file.
func Parse(b []byte) (*Info, error) {
	tiff := findTIFF(b)
	if tiff == nil {
		return nil, ErrNoExif
	}
	return parseTIFF(tiff)
}

// findTIFF locates the TIFF structure that holds the EXIF tags.
func findTIFF(b []byte) []byte {
	switch {
	case len(b) < 12:
		return nil
	case b[0] == 0xFF && b[1] == 0xD8:
		return jpegExif(b)
	case isTIFFHeader(b):
		return b
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return pngExif(b)
	case bytes.HasPrefix(b, []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return webpExif(b)
	case bytes.HasPrefix(b, []byte("FUJIFILMCCD-RAW")):
		// RAF: the EXIF is in the JPEG preview at the offset in the header.
		if len(b) < 92 {
			return nil
		}
		off := binary.BigEndian.Uint32(b[84:88])
		if int64(off) < int64(len(b)) {
			return findTIFF(b[off:])
		}
		return nil
	}
	// HEIC, AVIF, CR3 and other box formats: the marker that starts the block.
	marker := []byte("Exif\x00\x00")
	for from := 0; ; {
		i := bytes.Index(b[from:], marker)
		if i < 0 {
			break
		}
		at := from + i + len(marker)
		if at+8 <= len(b) && isTIFFHeader(b[at:]) {
			return b[at:]
		}
		from = at
	}
	// CR3 stores a bare TIFF in a box.
	if len(b) > 12 && string(b[4:8]) == "ftyp" {
		for _, sig := range [][]byte{[]byte("II*\x00"), []byte("MM\x00*")} {
			if i := bytes.Index(b, sig); i >= 0 {
				return b[i:]
			}
		}
	}
	return nil
}

func isTIFFHeader(b []byte) bool {
	return len(b) >= 8 && ((b[0] == 'I' && b[1] == 'I' && b[2] == 42 && b[3] == 0) || (b[0] == 'M' && b[1] == 'M' && b[2] == 0 && b[3] == 42))
}

func jpegExif(b []byte) []byte {
	for i := 2; i+4 <= len(b); {
		if b[i] != 0xFF {
			return nil
		}
		m := b[i+1]
		if m == 0xFF { // padding
			i++
			continue
		}
		if m == 0xDA || m == 0xD9 {
			return nil
		}
		if m >= 0xD0 && m <= 0xD8 {
			i += 2
			continue
		}
		l := int(binary.BigEndian.Uint16(b[i+2:]))
		if l < 2 || i+2+l > len(b) {
			return nil
		}
		seg := b[i+4 : i+2+l]
		if m == 0xE1 && bytes.HasPrefix(seg, []byte("Exif\x00\x00")) && isTIFFHeader(seg[6:]) {
			return seg[6:]
		}
		i += 2 + l
	}
	return nil
}

func pngExif(b []byte) []byte {
	for i := 8; i+12 <= len(b); {
		l := int64(binary.BigEndian.Uint32(b[i:]))
		typ := string(b[i+4 : i+8])
		end := int64(i) + 8 + l
		if end > int64(len(b)) {
			return nil
		}
		if typ == "eXIf" && isTIFFHeader(b[i+8:end]) {
			return b[i+8 : end]
		}
		if typ == "IDAT" || typ == "IEND" {
			return nil // EXIF after the picture data is out of reach here
		}
		i = int(end) + 4
	}
	return nil
}

func webpExif(b []byte) []byte {
	for i := 12; i+8 <= len(b); {
		l := int64(binary.LittleEndian.Uint32(b[i+4:]))
		end := int64(i) + 8 + l
		if end > int64(len(b)) {
			return nil
		}
		if string(b[i:i+4]) == "EXIF" {
			seg := b[i+8 : end]
			seg = bytes.TrimPrefix(seg, []byte("Exif\x00\x00"))
			if isTIFFHeader(seg) {
				return seg
			}
			return nil
		}
		i = int(end) + int(l&1)
	}
	return nil
}

// reader walks a TIFF structure with bounds checks on every access.
type reader struct {
	b   []byte
	ord binary.ByteOrder
}

func newReader(b []byte) (*reader, error) {
	if !isTIFFHeader(b) {
		return nil, ErrNoExif
	}
	r := &reader{b: b, ord: binary.LittleEndian}
	if b[0] == 'M' {
		r.ord = binary.BigEndian
	}
	return r, nil
}

func (r *reader) u16(at int64) (uint16, bool) {
	if at < 0 || at+2 > int64(len(r.b)) {
		return 0, false
	}
	return r.ord.Uint16(r.b[at:]), true
}

func (r *reader) u32(at int64) (uint32, bool) {
	if at < 0 || at+4 > int64(len(r.b)) {
		return 0, false
	}
	return r.ord.Uint32(r.b[at:]), true
}

type tagEntry struct {
	id, typ uint16
	count   uint32
	at      int64 // offset of the entry's 4-byte value field
}

// maxTags limits one directory; real files have far fewer.
const maxTags = 1024

func (r *reader) ifd(at int64) ([]tagEntry, int64) {
	n, ok := r.u16(at)
	if !ok || n > maxTags {
		return nil, 0
	}
	out := make([]tagEntry, 0, n)
	for i := int64(0); i < int64(n); i++ {
		e := at + 2 + i*12
		id, ok1 := r.u16(e)
		typ, ok2 := r.u16(e + 2)
		cnt, ok3 := r.u32(e + 4)
		if !ok1 || !ok2 || !ok3 {
			return out, 0
		}
		out = append(out, tagEntry{id: id, typ: typ, count: cnt, at: e + 8})
	}
	next, _ := r.u32(at + 2 + int64(n)*12)
	return out, int64(next)
}

var typeSize = map[uint16]int64{1: 1, 2: 1, 3: 2, 4: 4, 5: 8, 6: 1, 7: 1, 8: 2, 9: 4, 10: 8, 11: 4, 12: 8}

// value is the bytes of a tag's value, inline or at its offset.
func (r *reader) value(e tagEntry) []byte {
	size, ok := typeSize[e.typ]
	if !ok || int64(e.count) > int64(len(r.b)) {
		return nil
	}
	n := size * int64(e.count)
	if n > int64(len(r.b)) {
		return nil
	}
	if n <= 4 {
		if e.at+n > int64(len(r.b)) {
			return nil
		}
		return r.b[e.at : e.at+n]
	}
	off, ok := r.u32(e.at)
	if !ok || int64(off)+n > int64(len(r.b)) {
		return nil
	}
	return r.b[int64(off) : int64(off)+n]
}

func (r *reader) str(e tagEntry) string {
	v := r.value(e)
	if v == nil || (e.typ != 2 && e.typ != 7) {
		return ""
	}
	if i := bytes.IndexByte(v, 0); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(strings.ToValidUTF8(string(v), ""))
}

func (r *reader) ints(e tagEntry) []uint32 {
	v := r.value(e)
	if v == nil {
		return nil
	}
	var out []uint32
	switch e.typ {
	case 1, 7:
		for _, b := range v {
			out = append(out, uint32(b))
		}
	case 3:
		for i := 0; i+2 <= len(v); i += 2 {
			out = append(out, uint32(r.ord.Uint16(v[i:])))
		}
	case 4:
		for i := 0; i+4 <= len(v); i += 4 {
			out = append(out, r.ord.Uint32(v[i:]))
		}
	}
	return out
}

func (r *reader) int1(e tagEntry) int {
	if v := r.ints(e); len(v) > 0 && v[0] <= math.MaxInt32 {
		return int(v[0])
	}
	return 0
}

type rational struct{ num, den int64 }

func (r *reader) rationals(e tagEntry) []rational {
	v := r.value(e)
	if v == nil || (e.typ != 5 && e.typ != 10) {
		return nil
	}
	var out []rational
	for i := 0; i+8 <= len(v); i += 8 {
		if e.typ == 5 {
			out = append(out, rational{int64(r.ord.Uint32(v[i:])), int64(r.ord.Uint32(v[i+4:]))})
		} else {
			out = append(out, rational{int64(int32(r.ord.Uint32(v[i:]))), int64(int32(r.ord.Uint32(v[i+4:])))})
		}
	}
	return out
}

func (x rational) float() float64 {
	if x.den == 0 {
		return 0
	}
	return float64(x.num) / float64(x.den)
}

// EXIF tag numbers.
const (
	tagWidth        = 0x0100
	tagHeight       = 0x0101
	tagMake         = 0x010F
	tagModel        = 0x0110
	tagOrientation  = 0x0112
	tagDateTime     = 0x0132
	tagSubIFDs      = 0x014A
	tagExifIFD      = 0x8769
	tagGPSIFD       = 0x8825
	tagExposure     = 0x829A
	tagFNumber      = 0x829D
	tagISO          = 0x8827
	tagDateOriginal = 0x9003
	tagDateDigital  = 0x9004
	tagOffsetOrig   = 0x9011
	tagFocal        = 0x920A
	tagPixelX       = 0xA002
	tagPixelY       = 0xA003
	tagLens         = 0xA434
)

func parseTIFF(b []byte) (*Info, error) {
	r, err := newReader(b)
	if err != nil {
		return nil, err
	}
	first, _ := r.u32(4)
	tags, _ := r.ifd(int64(first))
	if tags == nil {
		return nil, ErrNoExif
	}
	in := &Info{}
	var exifAt, gpsAt int64
	var dateTime string
	for _, e := range tags {
		switch e.id {
		case tagMake:
			in.Make = r.str(e)
		case tagModel:
			in.Model = r.str(e)
		case tagOrientation:
			if o := r.int1(e); o >= 1 && o <= 8 {
				in.Orientation = o
			}
		case tagDateTime:
			dateTime = r.str(e)
		case tagWidth:
			in.Width = r.int1(e)
		case tagHeight:
			in.Height = r.int1(e)
		case tagExifIFD:
			exifAt = int64(r.int1(e))
		case tagGPSIFD:
			gpsAt = int64(r.int1(e))
		}
	}
	var original, digitized, offset string
	if exifAt > 0 {
		sub, _ := r.ifd(exifAt)
		for _, e := range sub {
			switch e.id {
			case tagDateOriginal:
				original = r.str(e)
			case tagDateDigital:
				digitized = r.str(e)
			case tagOffsetOrig:
				offset = r.str(e)
			case tagExposure:
				if v := r.rationals(e); len(v) > 0 {
					in.Exposure = exposureText(v[0])
				}
			case tagFNumber:
				if v := r.rationals(e); len(v) > 0 {
					in.FNumber = v[0].float()
				}
			case tagISO:
				in.ISO = r.int1(e)
			case tagFocal:
				if v := r.rationals(e); len(v) > 0 {
					in.FocalMM = v[0].float()
				}
			case tagPixelX:
				in.Width = r.int1(e)
			case tagPixelY:
				in.Height = r.int1(e)
			case tagLens:
				in.Lens = r.str(e)
			}
		}
	}
	for _, d := range []string{original, digitized, dateTime} {
		if t, ok := parseExifTime(d, offset); ok {
			in.Taken, in.HasOffset = t, offset != "" && d == original
			break
		}
	}
	if gpsAt > 0 {
		r.gps(gpsAt, in)
	}
	return in, nil
}

func (r *reader) gps(at int64, in *Info) {
	tags, _ := r.ifd(at)
	var latRef, lonRef string
	var lat, lon []rational
	altBelow := false
	var alt float64
	for _, e := range tags {
		switch e.id {
		case 1:
			latRef = r.str(e)
		case 2:
			lat = r.rationals(e)
		case 3:
			lonRef = r.str(e)
		case 4:
			lon = r.rationals(e)
		case 5:
			altBelow = r.int1(e) == 1
		case 6:
			if v := r.rationals(e); len(v) > 0 {
				alt = v[0].float()
			}
		}
	}
	la, ok1 := degrees(lat)
	lo, ok2 := degrees(lon)
	if !ok1 || !ok2 {
		return
	}
	if strings.EqualFold(latRef, "S") {
		la = -la
	}
	if strings.EqualFold(lonRef, "W") {
		lo = -lo
	}
	// Cameras without a fix write 0,0 or values that are off the planet.
	if math.Abs(la) > 90 || math.Abs(lo) > 180 || (la == 0 && lo == 0) {
		return
	}
	if altBelow {
		alt = -alt
	}
	in.HasGPS, in.Lat, in.Lon, in.Alt = true, la, lo, alt
}

func degrees(v []rational) (float64, bool) {
	if len(v) < 1 {
		return 0, false
	}
	d := v[0].float()
	if len(v) > 1 {
		d += v[1].float() / 60
	}
	if len(v) > 2 {
		d += v[2].float() / 3600
	}
	return d, !math.IsNaN(d) && !math.IsInf(d, 0)
}

func exposureText(x rational) string {
	if x.num <= 0 || x.den <= 0 {
		return ""
	}
	if x.num >= x.den {
		return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.1f", float64(x.num)/float64(x.den)), "0"), ".")
	}
	return fmt.Sprintf("1/%d", int64(math.Round(float64(x.den)/float64(x.num))))
}

// parseExifTime reads "2006:01:02 15:04:05", with an offset like "+02:00".
func parseExifTime(s, offset string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 19 || strings.HasPrefix(s, "0000") {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("2006:01:02 15:04:05", s[:19], time.UTC)
	if err != nil {
		if t, err = time.ParseInLocation("2006-01-02 15:04:05", s[:19], time.UTC); err != nil {
			return time.Time{}, false
		}
	}
	if offset != "" {
		if z, err := time.Parse("-07:00", strings.TrimSpace(offset)); err == nil {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, z.Location())
		}
	}
	return t, true
}

// RAW file extensions this package can look for a preview in.
var rawExts = map[string]bool{
	".cr2": true, ".cr3": true, ".crw": true, ".nef": true, ".nrw": true, ".arw": true, ".srf": true, ".sr2": true,
	".dng": true, ".orf": true, ".rw2": true, ".raf": true, ".pef": true, ".srw": true, ".3fr": true, ".fff": true,
	".erf": true, ".kdc": true, ".dcr": true, ".mef": true, ".mos": true, ".mrw": true, ".iiq": true, ".x3f": true, ".rwl": true,
}

// IsRaw reports whether a file name is a camera RAW format.
func IsRaw(name string) bool { return rawExts[strings.ToLower(filepath.Ext(name))] }
