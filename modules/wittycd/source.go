package wittycd

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// An image's audio can be spread over several files, and some of it may not be
// in a file at all (a generated pregap). An extent says where a run of sectors
// comes from.
type extent struct {
	lba, sectors int
	src          io.ReaderAt // nil: silence
	offset       int64       // of the first sector in src
	stride       int64       // bytes from one sector to the next, 0 means SectorBytes
	bigEndian    bool
	declared     bool // the image states the byte order, so it isn't guessed
	data         bool // a data track's sectors, never read as audio
}

// addExtent records a run of sectors. Anything past the limit is left out.
func (d *Disc) addExtent(e extent) {
	if e.lba < 0 || e.lba >= MaxSectors || e.sectors <= 0 {
		return
	}
	e.sectors = min(e.sectors, MaxSectors-e.lba)
	if e.stride == 0 {
		e.stride = SectorBytes
	}
	d.extents = append(d.extents, e)
}

// readAudio fills p with little-endian samples from the disc, starting skip bytes
// into sector lba. Anything the image doesn't supply reads as silence, so a
// damaged sector is a dropout rather than the end of the disc. Only a real I/O
// failure comes back as an error.
func (d *Disc) readAudio(lba, skip int, p []byte) error {
	for len(p) > 0 {
		e := d.extentAt(lba)
		if e == nil || e.src == nil || e.data {
			n := min(len(p), SectorBytes-skip)
			clear(p[:n])
			p = p[n:]
			lba, skip = lba+1, 0
			continue
		}
		var n int
		if e.stride == SectorBytes {
			n = int(min(int64(len(p)), int64(e.sectors-(lba-e.lba))*SectorBytes-int64(skip)))
		} else {
			n = min(len(p), SectorBytes-skip)
		}
		off := e.offset + int64(lba-e.lba)*e.stride + int64(skip)
		if err := readFull(e.src, p[:n], off); err != nil {
			return err
		}
		if e.bigEndian {
			swap16(p[:n], skip)
		}
		p = p[n:]
		total := skip + n
		lba += total / SectorBytes
		skip = total % SectorBytes
	}
	return nil
}

// readFull reads what is there and silences the rest: a file that was cut short
// after it was opened mustn't stop the music.
func readFull(r io.ReaderAt, p []byte, off int64) error {
	n, err := r.ReadAt(p, off)
	if n < len(p) {
		clear(p[n:])
	}
	if err != nil && err != io.EOF && n == 0 {
		return err
	}
	return nil
}

// swap16 swaps each value's bytes. Reads are whole samples, so skip is even in
// everything the server asks for; an odd start leaves its first byte alone.
func swap16(p []byte, skip int) {
	i := skip & 1
	for ; i+1 < len(p); i += 2 {
		p[i], p[i+1] = p[i+1], p[i]
	}
}

// file opens a file the image names, once, and keeps it until Close.
func (d *Disc) file(path string) (*os.File, int64, error) {
	if d.files == nil {
		d.files = map[string]*os.File{}
	}
	f := d.files[path]
	if f == nil {
		if st, err := os.Stat(path); err != nil {
			return nil, 0, err
		} else if !st.Mode().IsRegular() {
			return nil, 0, fmt.Errorf("%q isn't a file", filepath.Base(path))
		}
		var err error
		if f, err = os.Open(path); err != nil {
			return nil, 0, err
		}
		d.files[path] = f
		d.closers = append(d.closers, f)
	}
	st, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	return f, st.Size(), nil
}

// resolveBeside finds a file a sheet names. A sheet is someone else's text, so it
// can only ever reach files in or under its own folder: a name that is absolute
// or climbs out is looked for by its last part beside the sheet, which also
// rescues the many rips that recorded the full path on the ripper's PC.
func resolveBeside(dir, name string) (string, error) {
	name = strings.TrimSpace(strings.ReplaceAll(name, `\`, "/"))
	if name == "" || strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("a sheet names a file with no usable name")
	}
	rel := filepath.FromSlash(name)
	if !filepath.IsLocal(rel) || strings.Contains(name, ":") {
		last := name[strings.LastIndexAny(name, "/:")+1:]
		if last == "" || last == "." || last == ".." {
			return "", fmt.Errorf("a sheet names %q, which can't be used", name)
		}
		rel = filepath.FromSlash(last)
	}
	p := filepath.Join(dir, rel)
	st, err := os.Lstat(p)
	if err != nil || st.IsDir() {
		// Different letter case: a rip from Windows on a case-sensitive disk.
		if p = matchName(filepath.Dir(p), filepath.Base(p)); p == "" {
			return "", fmt.Errorf("the file %q this image needs isn't in its folder: %w", filepath.Base(rel), os.ErrNotExist)
		}
		if st, err = os.Lstat(p); err != nil {
			return "", err
		}
	}
	if st.Mode()&os.ModeSymlink != 0 && !staysInside(dir, p) {
		return "", fmt.Errorf("a sheet names %q, a link to somewhere outside its folder", name)
	}
	return p, nil
}

func staysInside(dir, p string) bool {
	root, err1 := filepath.EvalSymlinks(dir)
	target, err2 := filepath.EvalSymlinks(p)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(root, target)
	return err == nil && filepath.IsLocal(rel)
}

func matchName(dir, base string) string {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) > 20000 {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(e.Name(), base) {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

// audioFile is the audio inside a file a sheet points at.
type audioFile struct {
	src       io.ReaderAt
	offset    int64
	size      int64
	bigEndian bool
	declared  bool // the container says what the order is
}

// openAudioFile looks inside a file: RIFF WAVE, AIFF and FLAC announce themselves
// and are checked to be CD audio, and anything else is taken as raw sectors.
func (d *Disc) openAudioFile(path string, bigEndian, declared bool) (audioFile, error) {
	f, size, err := d.file(path)
	if err != nil {
		return audioFile{}, fmt.Errorf("can't open %q: %w", filepath.Base(path), err)
	}
	var head [12]byte
	if n, _ := f.ReadAt(head[:], 0); n >= 12 {
		switch {
		case string(head[:4]) == "RIFF" && string(head[8:12]) == "WAVE":
			off, length, err := wavData(f, size)
			if err != nil {
				return audioFile{}, fmt.Errorf("%q: %w", filepath.Base(path), err)
			}
			return audioFile{src: f, offset: off, size: length, declared: true}, nil
		case string(head[:4]) == "fLaC":
			r, length, err := newFlacSource(f, size)
			if err != nil {
				return audioFile{}, fmt.Errorf("%q: %w", filepath.Base(path), err)
			}
			return audioFile{src: r, size: length, declared: true}, nil
		case string(head[:4]) == "FORM" && (string(head[8:12]) == "AIFF" || string(head[8:12]) == "AIFC"):
			off, length, err := aiffData(f, size)
			if err != nil {
				return audioFile{}, fmt.Errorf("%q: %w", filepath.Base(path), err)
			}
			return audioFile{src: f, offset: off, size: length, bigEndian: true, declared: true}, nil
		}
	}
	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".mp3", ".ogg", ".oga", ".opus", ".m4a", ".aac", ".ape", ".wv", ".wma", ".tta", ".mpc":
		return audioFile{}, fmt.Errorf("%w: its audio is in a %s file", ErrUnsupported, ext[1:])
	}
	return audioFile{src: f, size: size, bigEndian: bigEndian, declared: declared}, nil
}

// wavData finds the samples in a WAV file, which must be CD audio: anything else
// played as sectors would be noise or the wrong speed.
func wavData(r io.ReaderAt, size int64) (off, length int64, err error) {
	pos := int64(12)
	fmtOK := false
	for steps := 0; pos+8 <= size && steps < 4096; steps++ {
		var h [8]byte
		if _, err := r.ReadAt(h[:], pos); err != nil {
			return 0, 0, fmt.Errorf("its WAV header can't be read: %w", err)
		}
		n := int64(binary.LittleEndian.Uint32(h[4:]))
		switch string(h[:4]) {
		case "fmt ":
			var b [40]byte
			m, _ := r.ReadAt(b[:], pos+8)
			if m < 16 {
				return 0, 0, fmt.Errorf("its WAV header is cut short")
			}
			format := binary.LittleEndian.Uint16(b[0:])
			if format == 0xfffe && m >= 26 {
				format = binary.LittleEndian.Uint16(b[24:])
			}
			ch, rate, bits := binary.LittleEndian.Uint16(b[2:]), binary.LittleEndian.Uint32(b[4:]), binary.LittleEndian.Uint16(b[14:])
			if format != 1 || ch != 2 || rate != SampleRate || bits != 16 {
				return 0, 0, fmt.Errorf("%w: its WAV file isn't CD audio (%d channels, %d Hz, %d bits)", ErrUnsupported, ch, rate, bits)
			}
			fmtOK = true
		case "data":
			if !fmtOK {
				return 0, 0, fmt.Errorf("its WAV file has its samples before its format")
			}
			return pos + 8, min(n, size-pos-8), nil
		}
		pos += 8 + n + n&1
	}
	return 0, 0, fmt.Errorf("its WAV file has no samples")
}

// aiffData is the same for AIFF, whose samples are big-endian.
func aiffData(r io.ReaderAt, size int64) (off, length int64, err error) {
	pos := int64(12)
	for steps := 0; pos+8 <= size && steps < 4096; steps++ {
		var h [8]byte
		if _, err := r.ReadAt(h[:], pos); err != nil {
			return 0, 0, fmt.Errorf("its AIFF header can't be read: %w", err)
		}
		n := int64(binary.BigEndian.Uint32(h[4:]))
		if string(h[:4]) == "SSND" {
			var b [4]byte
			if _, err := r.ReadAt(b[:], pos+8); err != nil {
				return 0, 0, fmt.Errorf("its AIFF file is cut short")
			}
			start := pos + 16 + int64(binary.BigEndian.Uint32(b[:]))
			if start >= size {
				return 0, 0, fmt.Errorf("its AIFF file is cut short")
			}
			return start, min(n-8, size-start), nil
		}
		pos += 8 + n + n&1
	}
	return 0, 0, fmt.Errorf("its AIFF file has no samples")
}
