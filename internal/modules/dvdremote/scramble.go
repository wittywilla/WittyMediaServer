package dvdremote

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/dvd"
)

// The server's own refusal of encrypted discs. A helper may be a fork, and its word
// that a disc is clear means nothing, so this code reads the disc itself:
//
//   - before a helper is told of a disc: samples through its video files;
//     encrypted, and the helper never hears of it;
//   - before asking a helper for menus and subtitles, whose bytes come through the helper:
//     the bytes where it says those sectors are;
//   - after a helper says where video is: the bytes FFmpeg would read.
//
// Only a helper's refusal is believed as it stands.

// scanSectors is how much of the start of a run the library scan reads to decide, and
// it is the same here.
const scanSectors = 64

// sample is one place on the disc already looked at.
type sample struct {
	sp          dvd.Space
	file        string
	first, last int64
}

const (
	maxChecked = 4096 // remembered places
	// maxSamples is how many times one disc's extents get read: enough for the cells
	// of a viewer's first titles, and a title of many cells doesn't wait on hundreds of
	// reads. They are the second line: FFmpeg can't play what is still encrypted.
	maxSamples = 32
	// With no map of a disc, this much is spread through each file.
	rawSpots   = 8
	rawSectors = 16
)

func anyScrambled(buf []byte) bool {
	for off := 0; off+dvd.Sector <= len(buf); off += dvd.Sector {
		if dvd.PackScrambled(buf[off : off+dvd.Sector]) {
			return true
		}
	}
	return false
}

// inspect looks at a disc before any helper hears of it. It returns an error when the disc
// is encrypted.
func (r *remote) inspect() error {
	if rawScrambled(r.path) {
		return errScrambled
	}
	return nil
}

// rawScrambled looks for encrypted packs through a disc's files, with no map of them.
// A DVD's packs lie on sector boundaries, in an image as in a VOB.
func rawScrambled(path string) bool {
	files := []string{path}
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		dir := path
		if !strings.EqualFold(filepath.Base(path), "VIDEO_TS") {
			// However it is spelled: a disc's own folder name isn't always in capitals, and
			// missing it here would mean not looking at all.
			dir = filepath.Join(path, "VIDEO_TS")
			if subs, err := os.ReadDir(path); err == nil {
				for _, s := range subs {
					if s.IsDir() && strings.EqualFold(s.Name(), "VIDEO_TS") {
						dir = filepath.Join(path, s.Name())
					}
				}
			}
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
	buf := make([]byte, rawSectors*dvd.Sector)
	for i := int64(1); i <= rawSpots; i++ {
		at := st.Size() / (rawSpots + 1) * i / dvd.Sector * dvd.Sector
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
func (r *remote) rangeScrambled(sp dvd.Space, first, last uint32) bool {
	k := sample{sp: sp, first: int64(first), last: int64(last)}
	if v, ok := r.checked[k]; ok {
		return v
	}
	// With no map of its own the server reads where the helper says those sectors
	// are, and any lie about that was looked for when the disc opened.
	ext, err := r.askExtents(dvd.MethodRange, dvd.RangeCall{Space: sp, First: first, Last: last})
	if err != nil {
		return r.remember(k, false) // the call itself will meet the same trouble
	}
	return r.remember(k, r.bytesScrambled(ext, true))
}

// extentsScrambled looks at what FFmpeg is about to be told to read.
func (r *remote) extentsScrambled(ext []dvd.FileExtent) bool {
	return r.bytesScrambled(ext, false)
}

func (r *remote) bytesScrambled(ext []dvd.FileExtent, always bool) bool {
	if len(ext) == 0 {
		return false
	}
	for _, e := range []dvd.FileExtent{ext[0], ext[len(ext)/2]} {
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

func (r *remote) extentScrambled(e dvd.FileExtent) bool {
	f := r.files[e.File]
	if f == nil {
		var err error
		if f, err = os.Open(e.File); err != nil {
			return false
		}
		r.files[e.File] = f
	}
	n := min(e.Length, scanSectors*dvd.Sector)
	buf := make([]byte, n)
	got, err := f.ReadAt(buf, e.Offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	return anyScrambled(buf[:got])
}
