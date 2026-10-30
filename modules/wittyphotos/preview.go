package wittyphotos

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
)

// ErrNoPreview means no embedded JPEG was found.
var ErrNoPreview = errors.New("no embedded preview")

// Limits on a RAW file: how much is read to walk its structure, and the largest
// preview that is believed (a JPEG preview is far smaller than this).
const (
	rawHeadLimit = 8 << 20
	maxPreview   = 64 << 20
)

// RawPreview returns the largest JPEG a camera RAW file carries. Nearly every
// camera stores one of screen size or larger, so a RAW file can be shown without
// developing its sensor data.
func RawPreview(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	head := make([]byte, rawHeadLimit)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	head = head[:n]
	off, size := previewRange(head)
	if size == 0 {
		return nil, ErrNoPreview
	}
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if size > maxPreview || off < 0 || off+size > st.Size() {
		return nil, ErrNoPreview
	}
	out := make([]byte, size)
	if _, err := f.ReadAt(out, off); err != nil {
		return nil, err
	}
	if len(out) < 2 || out[0] != 0xFF || out[1] != 0xD8 {
		return nil, ErrNoPreview
	}
	return out, nil
}

// previewRange finds the preview's place in a file from its first bytes.
func previewRange(b []byte) (off, size int64) {
	switch {
	case len(b) < 16:
		return 0, 0
	case bytes.HasPrefix(b, []byte("FUJIFILMCCD-RAW")) && len(b) >= 92:
		return int64(binary.BigEndian.Uint32(b[84:])), int64(binary.BigEndian.Uint32(b[88:]))
	case isTIFFHeader(b):
		return tiffPreview(b)
	}
	return largestJPEG(b)
}

// tiffPreview looks through every image directory of a TIFF-based RAW file (the
// first, the chain after it, and sub-directories) for the largest JPEG.
func tiffPreview(b []byte) (bestOff, bestSize int64) {
	r, err := newReader(b)
	if err != nil {
		return 0, 0
	}
	first, _ := r.u32(4)
	seen := map[int64]bool{}
	var visit func(at int64, depth int)
	visit = func(at int64, depth int) {
		for at > 0 && !seen[at] && depth < 6 && len(seen) < 64 {
			seen[at] = true
			tags, next := r.ifd(at)
			var off, size, compression, stripOff, stripSize int64
			for _, e := range tags {
				switch e.id {
				case 0x0103:
					compression = int64(r.int1(e))
				case 0x0201:
					off = int64(r.int1(e))
				case 0x0202:
					size = int64(r.int1(e))
				case 0x0111:
					stripOff = int64(r.int1(e))
				case 0x0117:
					stripSize = int64(r.int1(e))
				case tagSubIFDs:
					for _, v := range r.ints(e) {
						visit(int64(v), depth+1)
					}
				case tagExifIFD:
					visit(int64(r.int1(e)), depth+1)
				}
			}
			if off == 0 && (compression == 6 || compression == 7) {
				off, size = stripOff, stripSize
			}
			if off > 0 && size > bestSize && jpegAt(b, off) {
				bestOff, bestSize = off, size
			}
			at = next
		}
	}
	visit(int64(first), 0)
	if bestSize == 0 {
		return largestJPEG(b)
	}
	return bestOff, bestSize
}

// jpegAt: the bytes at off start a JPEG, or lie beyond what was read (which
// RawPreview checks once it has them).
func jpegAt(b []byte, off int64) bool {
	if off+2 > int64(len(b)) {
		return off > 0
	}
	return b[off] == 0xFF && b[off+1] == 0xD8
}

// largestJPEG scans for JPEG streams: the way into container formats whose
// structure differs by camera (CR3, X3F and others). It looks for complete
// streams within the part of the file that was read.
func largestJPEG(b []byte) (bestOff, bestSize int64) {
	sig := []byte{0xFF, 0xD8, 0xFF}
	for from := 0; from < len(b); {
		i := bytes.Index(b[from:], sig)
		if i < 0 {
			break
		}
		at := from + i
		end := jpegEnd(b[at:])
		if end > 0 && int64(end) > bestSize {
			bestOff, bestSize = int64(at), int64(end)
		}
		if end > 0 {
			from = at + end
		} else {
			from = at + len(sig)
		}
	}
	return bestOff, bestSize
}

// jpegEnd walks a JPEG's segments to the end-of-image marker, returning its
// length, or 0 if it is not a complete JPEG.
func jpegEnd(b []byte) int {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return 0
	}
	i := 2
	for i+2 <= len(b) {
		if b[i] != 0xFF {
			return 0
		}
		m := b[i+1]
		switch {
		case m == 0xFF:
			i++
			continue
		case m == 0xD9:
			return i + 2
		case m >= 0xD0 && m <= 0xD8 || m == 0x01:
			i += 2
			continue
		}
		if i+4 > len(b) {
			return 0
		}
		l := int(binary.BigEndian.Uint16(b[i+2:]))
		if l < 2 || i+2+l > len(b) {
			return 0
		}
		i += 2 + l
		if m == 0xDA {
			// Entropy-coded data runs until the next real marker.
			for i+2 <= len(b) {
				if b[i] == 0xFF && b[i+1] != 0 && (b[i+1] < 0xD0 || b[i+1] > 0xD7) {
					break
				}
				i++
			}
		}
	}
	return 0
}
