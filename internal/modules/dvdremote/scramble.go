package dvdremote

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wittywilla/wittydvd"
	"github.com/wittywilla/wittydvd/wire"
)

// The server's own refusal of encrypted discs. A helper may be a fork, and its word
// that a disc is clear means nothing, so this code reads the disc itself, always with
// the built-in copy's check:
//
//   - before a helper is told of a disc: its feature, as the library scan looks at it;
//     encrypted, and the helper never hears of it, and the built-in copy refuses it;
//   - before asking a helper for menus and subtitles, whose bytes come through the helper:
//     the very sectors asked for, through the built-in copy's own map of the disc;
//   - after a helper says where video is: the bytes FFmpeg would read.
//
// Only a helper's refusal is believed as it stands.

// scanSectors is how much of the start of a run the library scan reads to decide, and
// it is the same here.
const scanSectors = 64

// sample is one place on the disc already looked at.
type sample struct {
	sp          wittydvd.Space
	file        string
	first, last int64
}

const (
	maxChecked = 4096 // remembered places
	// maxSamples is how many times one disc's extents get read: enough for the cells
	// of a viewer's first titles, and a title of many cells doesn't wait on hundreds of
	// reads. They are the second line: FFmpeg can't play what is still encrypted.
	maxSamples = 32
	// When the built-in copy can't open a disc, this much is spread through each file.
	rawSpots   = 8
	rawSectors = 16
)

func anyScrambled(buf []byte) bool {
	for off := 0; off+wittydvd.Sector <= len(buf); off += wittydvd.Sector {
		if wittydvd.PackScrambled(buf[off : off+wittydvd.Sector]) {
			return true
		}
	}
	return false
}

// spots are the places in a run of sectors worth reading: its start, where the library
// scan looks, and its middle, as some discs begin with a clear picture.
func spots(first, last uint32) [][2]uint32 {
	end := func(at uint32) uint32 {
		if last-at < scanSectors-1 {
			return last
		}
		return at + scanSectors - 1
	}
	out := [][2]uint32{{first, end(first)}}
	if last-first >= 2*scanSectors {
		mid := first + (last-first)/2
		out = append(out, [2]uint32{mid, end(mid)})
	}
	return out
}

func readerScrambled(rd wittydvd.SectorReader, first, last uint32) bool {
	if last < first {
		return false
	}
	for _, s := range spots(first, last) {
		// A read that fails has nothing to say: the engine will meet the same trouble.
		if buf, err := rd.ReadSectors(s[0], int(s[1]-s[0])+1); err == nil && anyScrambled(buf) {
			return true
		}
	}
	return false
}

// inspect looks at a disc before any helper hears of it, and keeps the built-in copy's
// way of reading it for later. It returns an error when the disc is encrypted.
func (r *remote) inspect() error {
	o, err := wittydvd.Open(r.path)
	if err != nil {
		// A newer version may read what the built-in copy can't, which is much of the
		// point of one. There's no map of the disc, so look through its files instead.
		if rawScrambled(r.path) {
			return errScrambled
		}
		return nil
	}
	if m := o.Disc.MainTitle(); m != nil && len(m.Cells) > 0 {
		if rd, err := o.Reader(wittydvd.Space{Domain: wittydvd.DomainVTS, VTS: m.VTS}); err == nil {
			for _, c := range []wittydvd.TitleCell{m.Cells[0], m.Cells[len(m.Cells)/2]} {
				if readerScrambled(rd, c.Cell.FirstSector, c.Cell.LastSector) {
					_ = o.Close()
					return errScrambled
				}
			}
		}
	}
	o.Disc = nil // the helper's answers are what plays; this needn't stay in memory twice
	r.guard = o
	return nil
}

// rawScrambled looks for encrypted packs through a disc's files, with no map of them.
// A DVD's packs lie on sector boundaries, in an image as in a VOB.
func rawScrambled(path string) bool {
	files := []string{path}
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		dir := path
		if !strings.EqualFold(filepath.Base(path), "VIDEO_TS") {
			dir = filepath.Join(path, "VIDEO_TS")
		}
		entries, _ := os.ReadDir(dir)
		files = files[:0]
		for _, e := range entries {
			if strings.EqualFold(filepath.Ext(e.Name()), ".vob") {
				files = append(files, filepath.Join(dir, e.Name()))
			}
		}
	}
	for _, name := range files {
		if fileScrambled(name) {
			return true
		}
	}
	return false
}

func fileScrambled(name string) bool {
	f, err := os.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return false
	}
	buf := make([]byte, rawSectors*wittydvd.Sector)
	for i := int64(1); i <= rawSpots; i++ {
		at := st.Size() / (rawSpots + 1) * i / wittydvd.Sector * wittydvd.Sector
		n, err := f.ReadAt(buf, at)
		if err != nil && !errors.Is(err, io.EOF) {
			continue
		}
		if anyScrambled(buf[:n]) {
			return true
		}
	}
	return false
}

// remember keeps a place's answer, within reason.
func (r *remote) remember(k sample, v bool) bool {
	if len(r.checked) >= maxChecked {
		clear(r.checked)
	}
	r.checked[k] = v
	return v
}

// rangeScrambled looks at the sectors a helper is about to be asked about.
func (r *remote) rangeScrambled(sp wittydvd.Space, first, last uint32) bool {
	k := sample{sp: sp, first: int64(first), last: int64(last)}
	if v, ok := r.checked[k]; ok {
		return v
	}
	if r.guard != nil {
		rd, err := r.guard.Reader(sp)
		if err != nil {
			return r.remember(k, false) // no such video, so the engine has nothing to give
		}
		return r.remember(k, readerScrambled(rd, first, last))
	}
	// Without a map of its own the server reads where the helper says those sectors
	// are, and any lie about that was looked for when the disc opened.
	ext, err := r.askExtents(wire.MethodRange, wire.RangeCall{Space: sp, First: first, Last: last})
	if err != nil {
		return r.remember(k, false) // the call itself will meet the same trouble
	}
	return r.remember(k, r.bytesScrambled(ext, true))
}

// extentsScrambled looks at what FFmpeg is about to be told to read.
func (r *remote) extentsScrambled(ext []wittydvd.FileExtent) bool {
	return r.bytesScrambled(ext, false)
}

func (r *remote) bytesScrambled(ext []wittydvd.FileExtent, always bool) bool {
	if len(ext) == 0 {
		return false
	}
	for _, e := range []wittydvd.FileExtent{ext[0], ext[len(ext)/2]} {
		k := sample{file: e.File, first: e.Offset, last: e.Length}
		v, ok := r.checked[k]
		if !ok {
			if !always && r.samples >= maxSamples {
				return false
			}
			r.samples++
			v = r.remember(k, r.extentScrambled(e))
		}
		if v {
			return true
		}
	}
	return false
}

func (r *remote) extentScrambled(e wittydvd.FileExtent) bool {
	f := r.files[e.File]
	if f == nil {
		var err error
		if f, err = os.Open(e.File); err != nil {
			return false
		}
		r.files[e.File] = f
	}
	n := min(e.Length, scanSectors*wittydvd.Sector)
	buf := make([]byte, n)
	got, err := f.ReadAt(buf, e.Offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	return anyScrambled(buf[:got])
}
