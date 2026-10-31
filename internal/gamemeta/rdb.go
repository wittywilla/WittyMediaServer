package gamemeta

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"strings"
)

// Reading libretro's RDB files: the magic "RARCHDB\0", the offset of a small trailing
// map as a big-endian 64-bit number, then one MessagePack map per game until that
// offset. These files come off the internet, so the reader trusts no length in them.

// rdbRecord is what's kept of one game. Only the fields something reads are kept: a
// console's list is in memory while its games are being worked on.
type rdbRecord struct {
	Name, Description, ROMName             string
	Developer, Publisher, Genre, Franchise string
	Origin                                 string
	ESRB, PEGI, CERO, ELSPA                string
	Serial                                 string
	Users, ReleaseYear, ReleaseMonth       int
	Edge, Famitsu                          int
	CRC                                    uint32
	HasCRC                                 bool
	Size                                   int64
}

const (
	rdbMaxFile   = 64 << 20
	rdbMaxString = 4 << 10
	rdbMaxFields = 64
	rdbMaxDepth  = 4
)

var errBadRDB = errors.New("not a game database file")

// parseRDB reads a whole file's records, calling fn for each.
func parseRDB(b []byte, fn func(*rdbRecord)) error {
	if len(b) < 16 || string(b[:8]) != "RARCHDB\x00" {
		return errBadRDB
	}
	end := binary.BigEndian.Uint64(b[8:16])
	if end == 0 {
		end = uint64(len(b)) // an old file: it has nothing after its records
	}
	if end < 16 || end > uint64(len(b)) {
		return errBadRDB
	}
	p := &packReader{b: b[:end], off: 16}
	for p.off < len(p.b) {
		if p.b[p.off] == 0xc0 {
			return nil // the end of the records
		}
		rec, err := p.record()
		if err != nil {
			return err
		}
		if rec != nil {
			fn(rec)
		}
	}
	return nil
}

// packReader reads the parts of MessagePack these files use.
type packReader struct {
	b   []byte
	off int
}

func (p *packReader) byte() (byte, error) {
	if p.off >= len(p.b) {
		return 0, io.ErrUnexpectedEOF
	}
	c := p.b[p.off]
	p.off++
	return c, nil
}

func (p *packReader) take(n int) ([]byte, error) {
	if n < 0 || n > len(p.b)-p.off {
		return nil, io.ErrUnexpectedEOF
	}
	out := p.b[p.off : p.off+n]
	p.off += n
	return out, nil
}

func (p *packReader) uint(n int) (uint64, error) {
	b, err := p.take(n)
	if err != nil {
		return 0, err
	}
	var v uint64
	for _, c := range b {
		v = v<<8 | uint64(c)
	}
	return v, nil
}

// packValue is one decoded value: raw bytes, a number, or nothing useful.
type packValue struct {
	raw   []byte // strings and binary
	num   int64
	isNum bool
	size  int // a map or array's element count, with isMap or isList
	isMap bool
	isArr bool
}

func (p *packReader) head() (packValue, error) {
	c, err := p.byte()
	if err != nil {
		return packValue{}, err
	}
	bytesOf := func(n int) (packValue, error) {
		b, err := p.take(n)
		return packValue{raw: b}, err
	}
	sized := func(width int, f func(n int) (packValue, error)) (packValue, error) {
		n, err := p.uint(width)
		if err != nil {
			return packValue{}, err
		}
		if n > uint64(len(p.b)) {
			return packValue{}, errBadRDB
		}
		return f(int(n))
	}
	asMap := func(n int) (packValue, error) { return packValue{isMap: true, size: n}, nil }
	asArr := func(n int) (packValue, error) { return packValue{isArr: true, size: n}, nil }
	switch {
	case c <= 0x7f:
		return packValue{num: int64(c), isNum: true}, nil
	case c >= 0xe0:
		return packValue{num: int64(int8(c)), isNum: true}, nil
	case c >= 0x80 && c <= 0x8f:
		return asMap(int(c & 0x0f))
	case c >= 0x90 && c <= 0x9f:
		return asArr(int(c & 0x0f))
	case c >= 0xa0 && c <= 0xbf:
		return bytesOf(int(c & 0x1f))
	}
	switch c {
	case 0xc0, 0xc2, 0xc3:
		return packValue{}, nil
	case 0xc4, 0xd9:
		return sized(1, bytesOf)
	case 0xc5, 0xda:
		return sized(2, bytesOf)
	case 0xc6, 0xdb:
		return sized(4, bytesOf)
	case 0xcc, 0xcd, 0xce, 0xcf:
		width := 1 << (c - 0xcc)
		v, err := p.uint(width)
		if err != nil {
			return packValue{}, err
		}
		if v > math.MaxInt64 {
			v = math.MaxInt64
		}
		return packValue{num: int64(v), isNum: true}, nil
	case 0xd0:
		v, err := p.uint(1)
		return packValue{num: int64(int8(v)), isNum: true}, err
	case 0xd1:
		v, err := p.uint(2)
		return packValue{num: int64(int16(v)), isNum: true}, err
	case 0xd2:
		v, err := p.uint(4)
		return packValue{num: int64(int32(v)), isNum: true}, err
	case 0xd3:
		v, err := p.uint(8)
		return packValue{num: int64(v), isNum: true}, err
	case 0xdc:
		return sized(2, asArr)
	case 0xdd:
		return sized(4, asArr)
	case 0xde:
		return sized(2, asMap)
	case 0xdf:
		return sized(4, asMap)
	}
	return packValue{}, errBadRDB
}

// skip passes over a value's contents, for fields nothing reads.
func (p *packReader) skip(v packValue, depth int) error {
	if depth > rdbMaxDepth {
		return errBadRDB
	}
	n := v.size
	if v.isMap {
		n *= 2
	} else if !v.isArr {
		return nil
	}
	for i := 0; i < n; i++ {
		x, err := p.head()
		if err != nil {
			return err
		}
		if err := p.skip(x, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// record reads one top-level value. Anything that isn't a map is passed over.
func (p *packReader) record() (*rdbRecord, error) {
	v, err := p.head()
	if err != nil {
		return nil, err
	}
	if !v.isMap {
		return nil, p.skip(v, 0)
	}
	if v.size > rdbMaxFields {
		return nil, errBadRDB
	}
	r := &rdbRecord{}
	for i := 0; i < v.size; i++ {
		k, err := p.head()
		if err != nil {
			return nil, err
		}
		x, err := p.head()
		if err != nil {
			return nil, err
		}
		if err := p.skip(x, 1); err != nil {
			return nil, err
		}
		text := ""
		if x.raw != nil && len(x.raw) <= rdbMaxString {
			text = string(x.raw)
		}
		num := 0
		if x.isNum && x.num > 0 && x.num < math.MaxInt32 {
			num = int(x.num)
		}
		switch string(k.raw) {
		case "name":
			r.Name = text
		case "description":
			r.Description = text
		case "rom_name":
			r.ROMName = text
		case "developer":
			r.Developer = text
		case "publisher":
			r.Publisher = text
		case "genre":
			r.Genre = text
		case "franchise":
			r.Franchise = text
		case "origin":
			r.Origin = text
		case "esrb_rating":
			r.ESRB = text
		case "pegi_rating":
			r.PEGI = text
		case "cero_rating":
			r.CERO = text
		case "elspa_rating":
			r.ELSPA = text
		case "serial":
			r.Serial = strings.TrimFunc(text, func(c rune) bool { return c <= ' ' || c == 0x7f })
		case "users":
			r.Users = num
		case "releaseyear":
			r.ReleaseYear = num
		case "releasemonth":
			r.ReleaseMonth = num
		case "edge_rating":
			r.Edge = num
		case "famitsu_rating":
			r.Famitsu = num
		case "size":
			r.Size = int64(num)
		case "crc":
			if len(x.raw) == 4 {
				r.CRC, r.HasCRC = binary.BigEndian.Uint32(x.raw), true
			}
		}
	}
	return r, nil
}
