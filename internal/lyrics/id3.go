package lyrics

import (
	"encoding/binary"
	"io"
	"strings"
)

// ID3v2 keeps timed lyrics in SYLT frames and plain ones in USLT. FFmpeg passes on
// USLT but not SYLT, so an MP3's frames are read here. Only those two are kept: a
// cover picture, often most of the tag, is skipped without reading it.

const maxTagBytes = 64 << 20 // no real tag is larger; a false one is ignored

// ReadID3 finds lyrics in the ID3v2 tag at the start of r. An absent or damaged
// tag gives an empty Doc and no error worth reporting.
func ReadID3(r io.ReadSeeker) Doc {
	var head [10]byte
	if _, err := io.ReadFull(r, head[:]); err != nil || string(head[:3]) != "ID3" {
		return Doc{}
	}
	major := head[3]
	if major < 3 || major > 4 {
		return Doc{} // v2.2 has other frame names, and predates timed lyrics in practice
	}
	flags := head[5]
	size := int64(synchsafe(head[6:10]))
	if size <= 0 || size > maxTagBytes {
		return Doc{}
	}
	if flags&0x80 != 0 {
		return Doc{} // the whole tag is unsynchronised: rare, and not worth the code
	}
	end := size + 10
	pos := int64(10)
	if flags&0x40 != 0 { // an extended header to step over
		var ext [4]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return Doc{}
		}
		n := int64(binary.BigEndian.Uint32(ext[:]))
		if major == 4 {
			n = int64(synchsafe(ext[:]))
		} else {
			n += 4
		}
		if n < 4 || pos+n > end {
			return Doc{}
		}
		pos += n
		if _, err := r.Seek(pos, io.SeekStart); err != nil {
			return Doc{}
		}
	}
	var out Doc
	var plain string
	for pos+10 <= end {
		var fh [10]byte
		if _, err := io.ReadFull(r, fh[:]); err != nil {
			break
		}
		pos += 10
		if fh[0] == 0 { // padding
			break
		}
		id := string(fh[:4])
		var n int64
		if major == 4 {
			n = int64(synchsafe(fh[4:8]))
		} else {
			n = int64(binary.BigEndian.Uint32(fh[4:8]))
		}
		if n < 0 || pos+n > end {
			break
		}
		if (id == "SYLT" || id == "USLT") && n <= MaxBytes*2 {
			b := make([]byte, n)
			if _, err := io.ReadFull(r, b); err != nil {
				break
			}
			if id == "SYLT" && !out.Synced() {
				out.Timed = parseSYLT(b)
			}
			if id == "USLT" && plain == "" {
				plain = parseUSLT(b)
			}
		} else if _, err := r.Seek(pos+n, io.SeekStart); err != nil {
			break
		}
		pos += n
	}
	if out.Synced() {
		var words []string
		for i := range out.Timed {
			words = append(words, out.Timed[i].Text)
			if i+1 < len(out.Timed) {
				out.Timed[i].End = out.Timed[i+1].T
			}
		}
		out.Plain = strings.Join(words, "\n")
		return out
	}
	// USLT sometimes holds a whole LRC file.
	return Parse(plain)
}

func synchsafe(b []byte) uint32 {
	return uint32(b[0]&0x7f)<<21 | uint32(b[1]&0x7f)<<14 | uint32(b[2]&0x7f)<<7 | uint32(b[3]&0x7f)
}

// text decodes one ID3 string and says how many bytes it used, ending at its
// terminator (two bytes in UTF-16).
func id3String(enc byte, b []byte) (string, int) {
	wide := enc == 1 || enc == 2
	i := 0
	if wide {
		for i+1 < len(b) && !(b[i] == 0 && b[i+1] == 0) {
			i += 2
		}
	} else {
		for i < len(b) && b[i] != 0 {
			i++
		}
	}
	raw := b[:min(i, len(b))]
	used := i + 1
	if wide {
		used = i + 2
	}
	used = min(used, len(b))
	switch enc {
	case 0:
		return latin1(raw), used
	case 1:
		if len(raw) >= 2 && raw[0] == 0xFF && raw[1] == 0xFE {
			return utf16Text(raw[2:], false), used
		}
		if len(raw) >= 2 && raw[0] == 0xFE && raw[1] == 0xFF {
			return utf16Text(raw[2:], true), used
		}
		return utf16Text(raw, false), used
	case 2:
		return utf16Text(raw, true), used
	}
	return strings.ToValidUTF8(string(raw), ""), used
}

// USLT: encoding, three bytes of language, a description, then the words.
func parseUSLT(b []byte) string {
	if len(b) < 5 {
		return ""
	}
	enc := b[0]
	rest := b[4:]
	_, n := id3String(enc, rest)
	rest = rest[n:]
	var s string
	switch enc {
	case 0:
		s = latin1(rest)
	case 1:
		s, _ = id3String(1, append(append([]byte(nil), rest...), 0, 0))
	case 2:
		s = utf16Text(rest, true)
	default:
		s = strings.ToValidUTF8(string(rest), "")
	}
	return strings.TrimSpace(s)
}

// SYLT: encoding, language, the kind of time, the kind of content, a description,
// then text and four-byte times in turn. Only times in milliseconds are read: the
// other kind counts MPEG frames, and a stream's frame rate would be needed.
func parseSYLT(b []byte) []Line {
	if len(b) < 6 || b[4] != 2 {
		return nil
	}
	enc := b[0]
	rest := b[6:]
	_, n := id3String(enc, rest)
	rest = rest[n:]
	var out []Line
	for len(rest) > 4 && len(out) < MaxLines {
		s, used := id3String(enc, rest)
		rest = rest[used:]
		if len(rest) < 4 {
			break
		}
		t := int64(binary.BigEndian.Uint32(rest[:4]))
		rest = rest[4:]
		s = strings.Trim(s, "\r\n ")
		if s != "" {
			out = append(out, Line{T: t, Text: s})
		}
	}
	return out
}
