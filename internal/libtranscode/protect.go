package libtranscode

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"strings"

	"github.com/wittywilla/wittydvd"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// Finding copy protection, so that it can be refused. Nothing here removes it or reads
// past it: it looks for the marks a protected file carries, and the answer is always the
// same sentence.

// ProtectedReason is what a person is told about a protected file.
const ProtectedReason = "This file is copy-protected, so it can't be converted. Encrypted media isn't supported, and nothing here bypasses copy protection."

// How much of a file is looked at. A file may be hostile, or very large.
const (
	protectScanHead = 8 << 20
	protectMoovMax  = 32 << 20
	tsSamplePackets = 3000
)

// Detect says whether a file is protected. It's cautious in one direction only: a mark
// that's really there refuses the file, and an unreadable file has nothing to say.
func Detect(path string, p *ffmpeg.Probe) bool {
	if p != nil && protectedStreams(p) {
		return true
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return false
	}
	head := make([]byte, min(int64(protectScanHead), st.Size()))
	if _, err := io.ReadFull(f, head); err != nil {
		return false
	}
	switch {
	case isMP4Head(head):
		return mp4Protected(f, st.Size())
	case bytes.HasPrefix(head, []byte{0x1A, 0x45, 0xDF, 0xA3}):
		return matroskaProtected(head)
	case bytes.HasPrefix(head, asfHeaderGUID):
		return asfProtected(head)
	case isTransportStream(head, 188) || isTransportStream(head, 192):
		return tsProtected(f, st.Size(), head)
	case bytes.HasPrefix(head, []byte{0x00, 0x00, 0x01, 0xBA}):
		return programStreamProtected(f, st.Size())
	case bytes.HasPrefix(head, []byte("TiVo")):
		return true // a .TiVo file that hasn't been decoded by its owner's own tools
	}
	return false
}

// protectedStreams: the probe already says so. Those sample tags mean the track's data is
// encrypted; a codec tag of "drms" or "drmi" is the older FairPlay form.
func protectedStreams(p *ffmpeg.Probe) bool {
	for _, s := range p.Streams {
		switch strings.ToLower(s.CodecTag) {
		case "encv", "enca", "enct", "encs", "drms", "drmi", "sinf", "cenc", "cbcs":
			return true
		}
		for _, sd := range s.SideData {
			if t, _ := sd["side_data_type"].(string); strings.Contains(strings.ToLower(t), "encryption") {
				return true
			}
		}
	}
	return false
}

func isMP4Head(b []byte) bool {
	if len(b) < 12 {
		return false
	}
	switch string(b[4:8]) {
	case "ftyp", "moov", "mdat", "styp", "free", "skip", "wide", "moof":
		return true
	}
	return false
}

// mp4Protected walks the boxes to where an encrypted track says so: the kind of each
// track's samples, and the key information of the movie. It goes by structure, because
// words like these can also turn up in a title.
func mp4Protected(f io.ReaderAt, size int64) bool {
	found := false
	walkBoxes(f, 0, size, func(kind string, off, n int64) bool {
		if kind != "moov" && kind != "moof" {
			return true
		}
		buf := make([]byte, min(n, protectMoovMax))
		if _, err := f.ReadAt(buf, off); err != nil && err != io.EOF {
			return false
		}
		found = movieProtected(buf)
		return !found && kind != "moov"
	})
	return found
}

// walkBoxes calls fn with the kind, content start and content size of each box in a
// range, until fn says stop.
func walkBoxes(f io.ReaderAt, from, to int64, fn func(kind string, off, n int64) bool) {
	off := from
	for boxes := 0; off+8 <= to && boxes < 4096; boxes++ {
		var h [16]byte
		if _, err := f.ReadAt(h[:8], off); err != nil {
			return
		}
		n := int64(binary.BigEndian.Uint32(h[:4]))
		head := int64(8)
		switch n {
		case 1:
			if _, err := f.ReadAt(h[8:16], off+8); err != nil {
				return
			}
			n = int64(binary.BigEndian.Uint64(h[8:16]))
			head = 16
		case 0:
			n = to - off
		}
		if n < head || n > to-off {
			return
		}
		if !fn(string(h[4:8]), off+head, n-head) {
			return
		}
		off += n
	}
}

// movieProtected reads one movie or fragment header held in memory.
func movieProtected(b []byte) bool {
	r := bytes.NewReader(b)
	size := int64(len(b))
	found := false
	var walk func(from, to int64, depth int)
	walk = func(from, to int64, depth int) {
		walkBoxes(r, from, to, func(kind string, off, n int64) bool {
			switch kind {
			case "pssh", "senc":
				found = true
			case "trak", "mdia", "minf", "stbl", "traf":
				if depth < 8 {
					walk(off, off+n, depth+1)
				}
			case "stsd":
				// A version and flags, the number of entries, then the entries.
				if n >= 16 {
					walkBoxes(r, off+8, off+n, func(entry string, _, _ int64) bool {
						switch entry {
						case "encv", "enca", "enct", "encs", "drms", "drmi":
							found = true
						}
						return !found
					})
				}
			}
			return !found
		})
	}
	walk(0, size, 0)
	return found
}

// matroskaProtected reads the track list for a ContentEncryption element, or a
// ContentEncodingType of 1, which is what encryption is called there. Only the track list:
// the rest of the file is picture data, which holds every byte sequence sooner or later.
func matroskaProtected(head []byte) bool {
	tracks := []byte{0x16, 0x54, 0xAE, 0x6B}
	i := bytes.Index(head, tracks)
	if i < 0 {
		return false
	}
	rest := head[i+len(tracks):]
	size, used := ebmlSize(rest)
	if used == 0 {
		return false
	}
	rest = rest[used:]
	if size >= 0 && size < int64(len(rest)) {
		rest = rest[:size]
	}
	if !bytes.Contains(rest, []byte{0x6D, 0x80}) {
		return false // no ContentEncodings at all
	}
	return bytes.Contains(rest, []byte{0x50, 0x35}) || bytes.Contains(rest, []byte{0x50, 0x33, 0x81, 0x01})
}

// ebmlSize reads an element's size. -1 means it says "unknown". used is 0 for nonsense.
func ebmlSize(b []byte) (size int64, used int) {
	if len(b) == 0 || b[0] == 0 {
		return 0, 0
	}
	n := 1
	for mask := byte(0x80); b[0]&mask == 0; mask >>= 1 {
		n++
	}
	if n > len(b) {
		return 0, 0
	}
	v := int64(b[0]) & (int64(0xFF) >> n)
	allOnes := v == int64(0xFF)>>n
	for _, c := range b[1:n] {
		v = v<<8 | int64(c)
		allOnes = allOnes && c == 0xFF
	}
	if allOnes {
		return -1, n
	}
	return v, n
}

var (
	asfHeaderGUID = []byte{0x30, 0x26, 0xB2, 0x75, 0x8E, 0x66, 0xCF, 0x11, 0xA6, 0xD9, 0x00, 0xAA, 0x00, 0x62, 0xCE, 0x6C}
	// Content Encryption and Extended Content Encryption objects.
	asfEncryption    = []byte{0xFB, 0xB3, 0x11, 0x22, 0x23, 0xBD, 0xD2, 0x11, 0xB4, 0xB7, 0x00, 0xA0, 0xC9, 0x55, 0xFC, 0x6E}
	asfExtEncryption = []byte{0x14, 0xE6, 0x8A, 0x29, 0x22, 0x26, 0x17, 0x4C, 0xB9, 0x35, 0xDA, 0xE0, 0x7E, 0xE9, 0x28, 0x9C}
)

func asfProtected(head []byte) bool {
	return bytes.Contains(head, asfEncryption) || bytes.Contains(head, asfExtEncryption)
}

func isTransportStream(b []byte, size int) bool {
	lead := size - 188
	if len(b) < size*4 {
		return false
	}
	for i := 0; i < 4; i++ {
		if b[lead+i*size] != 0x47 {
			return false
		}
	}
	return true
}

// tsProtected samples packets from the middle too: only part of a recording may be
// scrambled. A clear stream has its scrambling bits at zero in every packet.
func tsProtected(f io.ReaderAt, size int64, head []byte) bool {
	unit := 188
	if !isTransportStream(head, 188) {
		unit = 192
	}
	check := func(b []byte) (scrambled, total int) {
		lead := unit - 188
		for i := 0; i+unit <= len(b); i += unit {
			p := b[i+lead:]
			if p[0] != 0x47 {
				continue
			}
			total++
			if p[3]&0xC0 != 0 {
				scrambled++
			}
		}
		return
	}
	s, t := check(head[:min(len(head), tsSamplePackets*unit)])
	if size > int64(len(head))*2 {
		// Line up on a packet boundary near the middle.
		mid := size / 2
		mid -= mid % int64(unit)
		buf := make([]byte, tsSamplePackets*unit)
		if n, err := f.ReadAt(buf, mid); n > 0 && (err == nil || err == io.EOF) {
			s2, t2 := check(buf[:n])
			s, t = s+s2, t+t2
		}
	}
	return t > 0 && s*20 > t // more than 5% of what was sampled
}

// programStreamProtected looks at packs through the check DVD playback uses, for a .vob
// that was copied off a disc with its scrambling still on it.
func programStreamProtected(f io.ReaderAt, size int64) bool {
	const packs = 512
	buf := make([]byte, wittydvd.Sector)
	for _, from := range []int64{0, size / 2} {
		from -= from % wittydvd.Sector
		for i := int64(0); i < packs; i++ {
			at := from + i*wittydvd.Sector
			if at+wittydvd.Sector > size {
				break
			}
			if _, err := f.ReadAt(buf, at); err != nil {
				break
			}
			if wittydvd.PackScrambled(buf) {
				return true
			}
		}
	}
	return false
}
