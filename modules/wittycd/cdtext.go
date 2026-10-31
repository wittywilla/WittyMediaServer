package wittycd

import (
	"strings"
	"unicode/utf8"
)

// CD-TEXT is stored as 18-byte packs: a type, a track number, a sequence number,
// a character position, 12 bytes of text and a checksum. Text runs from pack to
// pack, separated by NULs, one string per track.

const packBytes = 18

// parseCDText reads packs and fills in the disc's and tracks' text, without
// replacing anything already set. It returns how many good packs it found. Text
// in a character set it can't show (Japanese and other double-byte blocks) is
// passed over, which leaves the file names.
func (d *Disc) parseCDText(b []byte) int {
	// Some tools put a 4-byte length header in front.
	if len(b)%packBytes == 4 {
		b = b[4:]
	}
	if len(b) > 256*packBytes*8 { // 8 blocks of 256 packs is all a disc can hold
		b = b[:256*packBytes*8]
	}
	type stream struct {
		start int
		text  []byte
	}
	streams := map[byte]*stream{} // by pack type, block 0 only
	charset := byte(0)
	good := 0
	for ; len(b) >= packBytes; b = b[packBytes:] {
		p := b[:packBytes]
		if !packOK(p) {
			continue
		}
		good++
		kind, track, info := p[0], int(p[1]&0x7f), p[3]
		if info>>4&7 != 0 {
			continue
		}
		if kind == 0x8f {
			if p[1] == 0 { // the first size-information pack starts with the character set
				charset = p[4]
			}
			continue
		}
		if (kind > 0x85 && kind != 0x8e) || kind < 0x80 {
			continue
		}
		if info&0x80 != 0 {
			charset = 0x80 // a double-byte pack
			continue
		}
		s := streams[kind]
		if s == nil {
			s = &stream{start: track}
			streams[kind] = s
		}
		s.text = append(s.text, p[4:16]...)
	}
	if good == 0 {
		return 0
	}
	if charset >= 0x80 {
		d.warn("cdtext-charset", "This disc's CD-TEXT is in a double-byte character set that isn't shown yet, so names come from elsewhere.")
		return good
	}
	assign := func(kind byte, pick func(t *Text) *string) {
		s := streams[kind]
		if s == nil {
			return
		}
		n := s.start
		prev := ""
		for _, part := range strings.Split(string(s.text), "\x00") {
			v := strings.TrimSpace(latin1(part))
			if part == "\t" { // "the same as the one before"
				v = prev
			}
			prev = v
			var t *Text
			if n == 0 {
				t = &d.Text
			} else if tr := d.Track(n); tr != nil {
				t = &tr.Text
			}
			if t != nil && v != "" {
				if p := pick(t); *p == "" {
					*p = v
				}
			}
			if n++; n > MaxTracks {
				break
			}
		}
	}
	assign(0x80, func(t *Text) *string { return &t.Title })
	assign(0x81, func(t *Text) *string { return &t.Performer })
	assign(0x82, func(t *Text) *string { return &t.Songwriter })
	assign(0x83, func(t *Text) *string { return &t.Composer })
	assign(0x84, func(t *Text) *string { return &t.Arranger })
	assign(0x85, func(t *Text) *string { return &t.Message })
	if s := streams[0x8e]; s != nil { // the barcode, then each track's ISRC
		n := s.start
		for _, part := range strings.Split(string(s.text), "\x00") {
			if part != "" && part != "\t" {
				if tr := d.Track(n); n == 0 && d.Catalog == "" {
					d.Catalog = printable(part)
				} else if tr != nil && tr.ISRC == "" {
					tr.ISRC = printable(part)
				}
			}
			if n++; n > MaxTracks {
				break
			}
		}
	}
	return good
}

// packOK checks a pack's CRC-16 (CCITT, stored inverted).
func packOK(p []byte) bool {
	if p[0] < 0x80 || p[0] > 0x8f {
		return false
	}
	return ^crcCCITT(p[:16]) == uint16(p[16])<<8|uint16(p[17])
}

func crcCCITT(b []byte) uint16 {
	crc := uint16(0)
	for _, c := range b {
		crc ^= uint16(c) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// latin1 turns bytes in ISO 8859-1 into a string.
func latin1(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		b.WriteRune(rune(s[i]))
	}
	return b.String()
}

func printable(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
}

// sheetText turns the bytes of a text sheet into a string: a byte-order mark is
// believed, then valid UTF-8, and anything else is read as Windows Western, which
// is what most old rips were written in.
func sheetText(b []byte) string {
	switch {
	case len(b) >= 3 && b[0] == 0xef && b[1] == 0xbb && b[2] == 0xbf:
		return strings.ToValidUTF8(string(b[3:]), "�")
	case len(b) >= 2 && b[0] == 0xff && b[1] == 0xfe:
		return utf16Text(b[2:], false)
	case len(b) >= 2 && b[0] == 0xfe && b[1] == 0xff:
		return utf16Text(b[2:], true)
	case utf8.Valid(b):
		return string(b)
	}
	var s strings.Builder
	s.Grow(len(b))
	for _, c := range b {
		if c >= 0x80 && c < 0xa0 {
			s.WriteRune(cp1252[c-0x80])
		} else {
			s.WriteRune(rune(c))
		}
	}
	return s.String()
}

func utf16Text(b []byte, big bool) string {
	var s strings.Builder
	s.Grow(len(b) / 2)
	var hi rune
	for i := 0; i+1 < len(b); i += 2 {
		v := rune(b[i]) | rune(b[i+1])<<8
		if big {
			v = rune(b[i])<<8 | rune(b[i+1])
		}
		switch {
		case v >= 0xd800 && v < 0xdc00:
			hi = v
			continue
		case v >= 0xdc00 && v < 0xe000 && hi != 0:
			v = 0x10000 + (hi-0xd800)<<10 + (v - 0xdc00)
		case v >= 0xd800 && v < 0xe000:
			v = utf8.RuneError
		}
		hi = 0
		s.WriteRune(v)
	}
	return s.String()
}

var cp1252 = [32]rune{'€', '�', '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ', '�', 'Ž', '�',
	'�', '‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›', 'œ', '�', 'ž', 'Ÿ'}
