// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package libtranscode

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"strings"

	wittydvd "github.com/wittywilla/wittymediaserver/internal/dvd"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// Finding copy protection, so that it can be refused. Nothing here removes it or reads
// past it: it looks for the marks a protected file carries, and the answer is always the
// same sentence.

// ProtectedReason is what a person is told about a protected file.
const ProtectedReason = "This file is copy-protected, so it can't be converted. Encrypted media isn't supported, and nothing here bypasses copy protection."

// How much of a file is looked at. A file may be hostile, or very large.
const (
	protectScanHead = 2 << 20
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
		return matroskaProtected(f, st.Size(), head)
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

// Matroska element IDs the check reads.
const (
	mkvSegment       = 0x18538067
	mkvTracks        = 0x1654AE6B
	mkvCluster       = 0x1F43B675
	mkvTrackEntry    = 0xAE
	mkvEncodings     = 0x6D80
	mkvEncoding      = 0x6240
	mkvEncodingType  = 0x5033
	mkvEncryptionElt = 0x5035
	mkvSeekHead      = 0x114D9B74
	mkvSeek          = 0x4DBB
	mkvSeekID        = 0x53AB
	mkvSeekPos       = 0x53AC
)

// matroskaProtected reads the track list for a ContentEncryption element, or a
// ContentEncodingType of 1, which is what encryption is called there. It goes by structure:
// the segment's own Tracks element, and the content encodings inside it. Searching the bytes
// found the seek index's mention of Tracks first, and then a file's random IDs and picture
// data now and then held the marks by chance.
//
// The track list is usually in the file's first few kilobytes. When it isn't (behind the
// pictures, or longer than what was read), the segment's seek index says where it is, and it
// is read from there: a file mustn't escape the check by putting its track list late.
func matroskaProtected(f io.ReaderAt, size int64, head []byte) bool {
	rest := head
	for len(rest) > 0 {
		id, _, body, next, ok := ebmlElement(rest)
		if !ok {
			return false
		}
		if id != mkvSegment {
			rest = next // the EBML header, or anything before the segment
			continue
		}
		protected, done, at := segmentProtected(body)
		if done || at < 0 {
			return protected
		}
		// body shares head's memory: what's left of its capacity says where it starts.
		return tracksAtProtected(f, size, int64(cap(head)-cap(body))+at)
	}
	return false
}

// segmentProtected finds the segment's Tracks among its children. A cluster means the
// picture has started, and a child of unknown size can't be stepped over. done says the
// whole track list was read; otherwise at is where in the segment to read it from (from the
// seek index, or a list cut short by the end of what was read), or -1 for nowhere.
func segmentProtected(seg []byte) (protected, done bool, at int64) {
	at = -1
	rest := seg
	for len(rest) > 0 {
		id, size, body, next, ok := ebmlElement(rest)
		switch {
		case ok && id == mkvTracks && size >= 0 && size > int64(len(body)):
			return false, false, int64(cap(seg) - cap(rest))
		case ok && id == mkvTracks:
			return encodingsProtected(body, false), true, -1
		case ok && id == mkvSeekHead:
			if pos := seekPosition(body, mkvTracks); pos >= 0 {
				at = pos
			}
		}
		if !ok || id == mkvCluster || size < 0 {
			return false, false, at
		}
		rest = next
	}
	return false, false, at
}

// tracksAtProtected reads a track list where the file says it is, and looks in it.
func tracksAtProtected(f io.ReaderAt, size, off int64) bool {
	if off <= 0 || off >= size {
		return false
	}
	buf := make([]byte, min(size-off, protectMoovMax))
	n, err := f.ReadAt(buf, off)
	if err != nil && err != io.EOF {
		return false
	}
	id, _, body, _, ok := ebmlElement(buf[:n])
	return ok && id == mkvTracks && encodingsProtected(body, false)
}

// seekPosition finds where a seek index says an element is, from the segment's start, or -1.
func seekPosition(index []byte, want uint32) int64 {
	rest := index
	for len(rest) > 0 {
		id, size, body, next, ok := ebmlElement(rest)
		if !ok {
			return -1
		}
		if id == mkvSeek {
			var target uint32
			pos := int64(-1)
			inner := body
			for len(inner) > 0 {
				id, size, b, after, ok := ebmlElement(inner)
				if !ok {
					break
				}
				switch {
				case id == mkvSeekID && len(b) <= 4:
					for _, c := range b {
						target = target<<8 | uint32(c)
					}
				case id == mkvSeekPos && len(b) <= 8:
					pos = 0
					for _, c := range b {
						pos = pos<<8 | int64(c)
					}
				}
				if size < 0 {
					break
				}
				inner = after
			}
			if target == want && pos >= 0 {
				return pos
			}
		}
		if size < 0 {
			return -1
		}
		rest = next
	}
	return -1
}

// encodingsProtected walks a track list. inside says it's within ContentEncodings, where
// encryption is written down. Only the containers that lead there are opened.
func encodingsProtected(b []byte, inside bool) bool {
	rest := b
	for len(rest) > 0 {
		id, size, body, next, ok := ebmlElement(rest)
		if !ok {
			return false
		}
		switch {
		case inside && id == mkvEncryptionElt:
			return true
		case inside && id == mkvEncodingType && ebmlUintIs(body, 1):
			return true
		case id == mkvEncodings:
			if encodingsProtected(body, true) {
				return true
			}
		case id == mkvTrackEntry || (inside && id == mkvEncoding):
			if encodingsProtected(body, inside) {
				return true
			}
		}
		if size < 0 {
			return false
		}
		rest = next
	}
	return false
}

// ebmlUintIs says whether an element's unsigned integer body is v.
func ebmlUintIs(body []byte, v uint64) bool {
	if len(body) == 0 || len(body) > 8 {
		return false
	}
	var n uint64
	for _, c := range body {
		n = n<<8 | uint64(c)
	}
	return n == v
}

// ebmlElement reads one element: its ID, its size (-1 for unknown), its body as far as b
// holds it, and what follows it. ok is false for nonsense.
func ebmlElement(b []byte) (id uint32, size int64, body, next []byte, ok bool) {
	if len(b) == 0 || b[0] == 0 {
		return 0, 0, nil, nil, false
	}
	n := 1
	for mask := byte(0x80); b[0]&mask == 0; mask >>= 1 {
		n++
	}
	if n > 4 || n > len(b) {
		return 0, 0, nil, nil, false
	}
	for _, c := range b[:n] {
		id = id<<8 | uint32(c)
	}
	size, used := ebmlSize(b[n:])
	if used == 0 {
		return 0, 0, nil, nil, false
	}
	rest := b[n+used:]
	if size < 0 || size > int64(len(rest)) {
		return id, size, rest, nil, true
	}
	return id, size, rest[:size], rest[size:], true
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
