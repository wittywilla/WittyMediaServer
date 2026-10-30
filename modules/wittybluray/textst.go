package wittybluray

import (
	"fmt"
	"image/color"
	"strings"
	"unicode/utf16"
)

// Text subtitles (TextST): unlike the usual picture subtitles these are real
// text, kept in a clip of their own, drawn by the player from a style table
// in the disc's fonts. They are rare, and mostly found on discs with many
// languages. A player that lays text over video only needs the words, the
// times and the place; fonts are the browser's.

const (
	segDialogStyle        = 0x81
	segDialogPresentation = 0x82
)

// TextStyle is a region style of the dialog style segment.
type TextStyle struct {
	ID            int
	X, Y, W, H    int // the region on the video frame
	Background    color.RGBA
	BoxX, BoxY    int // the text box, relative to the region
	BoxW, BoxH    int
	Flow          int // 1 = left to right, 2 = right to left, 3 = top to bottom
	HAlign        int // 1 = left, 2 = centre, 3 = right
	VAlign        int // 1 = top, 2 = middle, 3 = bottom
	LineSpace     int
	Bold, Italic  bool
	Outline       bool
	FontSize      int
	Color         color.RGBA
	OutlineColor  color.RGBA
	OutlineWeight int
}

// TextCue is one dialog: its words, when, and in what style.
type TextCue struct {
	Start, End uint32 // 45 kHz, in the clip's time base
	Forced     bool
	Style      TextStyle
	Text       string // lines separated by \n
}

// TextSubtitles reads a text subtitle clip whole (they are small). charCode
// is the stream's character coding from the playlist.
func (d *Disc) TextSubtitles(clip string, pid uint16, charCode int) ([]TextCue, error) {
	if d.Clip(clip) == nil {
		return nil, fmt.Errorf("clip %s: no clip information", clip)
	}
	var styles []TextStyle
	var palette Palette
	var out []TextCue
	var segs []segment
	err := d.ReadPID(clip, pid, 0, 0, func(payload []byte, pts uint64) bool {
		segs = splitSegments(payload, pts, segs[:0])
		for _, s := range segs {
			switch s.typ {
			case segDialogStyle:
				styles, palette = parseDialogStyle(s.data)
			case segDialogPresentation:
				out = append(out, parseDialog(s.data, styles, &palette, charCode)...)
			}
			if len(out) > 100000 {
				return false
			}
		}
		return true
	})
	return out, err
}

// parseDialogStyle reads the style table and the palette after it.
func parseDialogStyle(b []byte) ([]TextStyle, Palette) {
	r := &rd{b: b}
	var pal Palette
	nr, nu := int(r.u8(2)), int(r.u8(3))
	p := 4
	type raw struct {
		s          TextStyle
		bg, fg, ol int
	}
	var rs []raw
	for i := 0; i < nr && r.err == nil; i, p = i+1, p+29 {
		f := r.u8(p + 24)
		rs = append(rs, raw{s: TextStyle{ID: int(r.u8(p)), X: int(r.u16(p + 1)), Y: int(r.u16(p + 3)), W: int(r.u16(p + 5)), H: int(r.u16(p + 7)),
			BoxX: int(r.u16(p + 11)), BoxY: int(r.u16(p + 13)), BoxW: int(r.u16(p + 15)), BoxH: int(r.u16(p + 17)),
			Flow: int(r.u8(p + 19)), HAlign: int(r.u8(p + 20)), VAlign: int(r.u8(p + 21)), LineSpace: int(r.u8(p + 22)),
			Bold: f&1 != 0, Italic: f&2 != 0, Outline: f&4 != 0, FontSize: int(r.u8(p + 25)), OutlineWeight: int(r.u8(p + 28))},
			bg: int(r.u8(p + 9)), fg: int(r.u8(p + 26)), ol: int(r.u8(p + 27))})
	}
	p += nu * 15 // viewer-selectable style changes: the browser sizes text itself
	l := int(r.u16(p))
	if r.err != nil || p+2+l > len(b) {
		return nil, pal
	}
	readPalette(b[p+2:p+2+l], &pal)
	out := make([]TextStyle, len(rs))
	for i, x := range rs {
		x.s.Background, x.s.Color, x.s.OutlineColor = pal[x.bg&255], pal[x.fg&255], pal[x.ol&255]
		out[i] = x.s
	}
	return out, pal
}

// readPalette fills palette entries (id, Y, Cr, Cb, alpha).
func readPalette(b []byte, pal *Palette) {
	for off := 0; off+5 <= len(b); off += 5 {
		pal[b[off]] = yCrCbA(b[off+1], b[off+2], b[off+3], b[off+4], matrix709)
	}
}

// parseDialog reads a dialog presentation segment: one or two regions of text.
func parseDialog(b []byte, styles []TextStyle, pal *Palette, charCode int) []TextCue {
	r := &rd{b: b}
	ts := func(o int) uint32 { // 33 bits at 90 kHz; the engine keeps 45 kHz
		return uint32((uint64(r.u8(o)&1)<<32 | uint64(r.u32(o+1))) / 2)
	}
	start, end := ts(0), ts(5)
	p := 10
	if r.u8(p)&0x80 != 0 {
		l := int(r.u16(p + 1))
		if p+3+l <= len(b) {
			var up = *pal
			readPalette(b[p+3:p+3+l], &up)
			pal = &up
		}
		p += 2 + l
	}
	p++
	n := int(r.u8(p))
	p++
	var out []TextCue
	for i := 0; i < n && i < 2 && r.err == nil; i++ {
		f := r.u8(p)
		id := int(r.u8(p + 1))
		l := int(r.u16(p + 2))
		p += 4
		if p+l > len(b) {
			break
		}
		c := TextCue{Start: start, End: end, Forced: f&0x40 != 0, Text: inlineText(b[p:p+l], charCode)}
		for _, s := range styles {
			if s.ID == id {
				c.Style = s
			}
		}
		out = append(out, c)
		p += l
	}
	return out
}

// inlineText takes the words out of a region's inline elements: strings and
// line breaks. Font changes inside a line are not kept.
func inlineText(b []byte, charCode int) string {
	var sb strings.Builder
	for i := 0; i < len(b); {
		if b[i] != 0x1b {
			i++
			continue
		}
		if i+3 > len(b) {
			break
		}
		typ, l := b[i+1], int(b[i+2])
		i += 3
		if typ == 0x0a {
			sb.WriteByte('\n')
		}
		if i+l > len(b) {
			break
		}
		if typ == 0x01 {
			sb.WriteString(decodeText(b[i:i+l], charCode))
		}
		i += l
	}
	return sb.String()
}

// decodeText converts a string of the disc's character coding: 1 = UTF-8, 2 =
// UTF-16. The legacy Asian codings need tables this engine does not carry;
// their bytes are dropped rather than shown as nonsense.
func decodeText(b []byte, charCode int) string {
	switch charCode {
	case 0, 1:
		return strings.ToValidUTF8(string(b), "")
	case 2:
		u := make([]uint16, len(b)/2)
		for i := range u {
			u[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
		}
		return string(utf16.Decode(u))
	}
	return ""
}
