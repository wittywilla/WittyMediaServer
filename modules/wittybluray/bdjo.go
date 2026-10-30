package wittybluray

import (
	"errors"
	"fmt"
	"strings"
)

// BDMV/BDJO/xxxxx.bdjo: a BD-J object, the Java counterpart of a movie
// object. It lists the applications of a title (their JAR files and first
// classes), which playlists they may play, the keys they want and the files
// they may read. Reading it needs no Java at all: its playlist table says
// what the title plays even when the applications cannot run.

// BDJObject is one .bdjo file.
type BDJObject struct {
	Name    string
	Version string
	// DefaultFont names a font in AUXDATA ("" = none). HAViConfig is the
	// graphics configuration the applications start with.
	DefaultFont     string
	HAViConfig      int
	MenuCallMask    bool
	TitleSearchMask bool
	Cache           []BDJCacheItem
	// Playlists the applications may play. AccessToAll allows every playlist
	// on the disc; AutostartFirst plays the first one as the title starts.
	Playlists      []int
	AccessToAll    bool
	AutostartFirst bool
	Apps           []BDJApp
	KeyInterest    uint32 // the 11 remote keys the title wants, top bit first
	// FileAccess lists the folders the applications may read ("." = all).
	FileAccess []string
}

// BDJCacheItem is a JAR file or folder to load before the title starts.
type BDJCacheItem struct {
	Dir  bool // a folder rather than a JAR
	Name string
	Lang string // "*.*" = always; "%%%" = when no language matches
}

// BDJApp is one application of the title.
type BDJApp struct {
	Control      int // 1 = starts by itself, 2 = another application starts it
	Type         int // 1 = BD-J
	Org          uint32
	ID           uint16
	Priority     int
	Binding      int
	Visibility   int
	Names        map[string]string // by language
	Icon         string
	BaseDir      string // the JAR file's name, without .jar
	ClassPath    string
	InitialClass string
	Params       []string
}

// Autostart lists the applications that start with the title.
func (o *BDJObject) Autostart() []*BDJApp {
	var out []*BDJApp
	for i := range o.Apps {
		if o.Apps[i].Control == 1 {
			out = append(out, &o.Apps[i])
		}
	}
	return out
}

// ParseBDJObject parses one .bdjo file.
func ParseBDJObject(name string, b []byte) (*BDJObject, error) {
	r := &rd{b: b}
	if r.str(0, 4) != "BDJO" {
		return nil, errors.New("not a BD-J object")
	}
	o := &BDJObject{Name: name, Version: r.str(4, 4)}
	p := 48 // after the signature, the version and the address table

	// Terminal information.
	if f := r.str(p+4, 5); f != "*****" {
		o.DefaultFont = strings.TrimSpace(f)
	}
	flags := r.u8(p + 9)
	o.HAViConfig = int(flags >> 4)
	o.MenuCallMask, o.TitleSearchMask = flags&8 != 0, flags&4 != 0
	p += 14

	// Application cache.
	n := int(r.u8(p + 4))
	p += 6
	for i := 0; i < n && r.err == nil; i++ {
		o.Cache = append(o.Cache, BDJCacheItem{Dir: r.u8(p) == 2, Name: r.str(p+1, 5), Lang: r.str(p+6, 3)})
		p += 12
	}

	// Accessible playlists.
	w := r.u32(p + 4)
	n = int(w >> 21)
	o.AccessToAll, o.AutostartFirst = w&(1<<20) != 0, w&(1<<19) != 0
	p += 8
	for i := 0; i < n && r.err == nil; i++ {
		var num int
		if _, err := fmt.Sscanf(r.str(p, 5), "%d", &num); err == nil {
			o.Playlists = append(o.Playlists, num)
		}
		p += 6
	}

	// Application management table.
	n = int(r.u8(p + 4))
	p += 6
	for i := 0; i < n && r.err == nil; i++ {
		a, next := parseBDJApp(r, p)
		o.Apps = append(o.Apps, a)
		p = next
	}

	o.KeyInterest = r.u32(p)
	p += 4
	l := int(r.u16(p))
	if s := r.str(p+2, l); s != "" {
		o.FileAccess = strings.Split(s, ";")
	}
	if r.err != nil {
		return nil, fmt.Errorf("%s.bdjo: %w", name, r.err)
	}
	return o, nil
}

func parseBDJApp(r *rd, p int) (BDJApp, int) {
	a := BDJApp{Control: int(r.u8(p)), Type: int(r.u8(p+1) >> 4), Org: r.u32(p + 2), ID: r.u16(p + 6), Names: map[string]string{}}
	p += 8 + 10
	profiles := int(r.u16(p) >> 12)
	p += 2 + profiles*6
	a.Priority = int(r.u8(p))
	f := r.u8(p + 1)
	a.Binding, a.Visibility = int(f>>6), int(f>>4)&3
	p += 2
	// Names: language and text records, kept to an even length.
	l := int(r.u16(p))
	q := p + 2
	for end := q + l; q+4 <= end && r.err == nil; {
		k := int(r.u8(q + 3))
		a.Names[r.str(q, 3)] = r.str(q+4, k)
		q += 4 + k
	}
	p += 2 + l + l&1
	// Strings with one length byte, padded so length byte and text are even.
	text := func() string {
		k := int(r.u8(p))
		s := r.str(p+1, k)
		p += 1 + k + 1 - k&1
		return s
	}
	a.Icon = text()
	p += 2 // icon flags
	a.BaseDir = text()
	a.ClassPath = text()
	a.InitialClass = text()
	l = int(r.u8(p))
	q = p + 1
	for end := q + l; q < end && r.err == nil; {
		k := int(r.u8(q))
		a.Params = append(a.Params, r.str(q+1, k))
		q += 1 + k
	}
	p += 1 + l + 1 - l&1
	return a, p
}

// BDJObject reads a BD-J object by name, from its backup copy if it is damaged.
func (d *Disc) BDJObject(name string) (*BDJObject, error) {
	if !safeName(name) {
		return nil, fmt.Errorf("%q is not a BD-J object name", name)
	}
	return readStructure(d, "BDJO/"+name+".bdjo", func(b []byte) (*BDJObject, error) { return ParseBDJObject(name, b) })
}

// safeName accepts the five-character names a disc's tables use, so a name
// from a damaged or hostile table can never reach outside its folder.
func safeName(s string) bool {
	if s == "" || len(s) > 16 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}

// ReadProgramFile reads a file of the disc's program by its path under BDMV.
func (d *Disc) ReadProgramFile(rel string, max int64) ([]byte, error) {
	return d.Source.ReadFile(d.layout.root+"/"+rel, max)
}
