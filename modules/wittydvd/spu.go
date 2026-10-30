package wittydvd

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Sub-picture display control commands.
const (
	spuFSTADSP   = 0x00
	spuSTADSP    = 0x01
	spuSTPDSP    = 0x02
	spuSETCOLOR  = 0x03
	spuSETCONTR  = 0x04
	spuSETDAREA  = 0x05
	spuSETDSPXA  = 0x06
	spuCHGCOLCON = 0x07
	spuCMDEND    = 0xff
)

var ErrSPU = errors.New("dvd: malformed sub-picture")

// SPU is a decoded sub-picture: a 2-bit indexed bitmap plus its default look.
type SPU struct {
	X, Y          int
	Width, Height int
	Pixels        []byte // one code (0..3) per pixel
	Color         [4]int
	Contrast      [4]int
	// ColCon are CHG_COLCON regions: other colours and contrasts for parts
	// of the picture (karaoke wipes, two-colour subtitles, menu artwork).
	ColCon    []ColConLines
	Forced    bool
	StartSec  float64
	StopSec   float64 // <0 when absent
	Truncated bool
}

// ColConLines is one CHG_COLCON line range: from each column rightwards (to
// the next one) pixels take that entry's colours and contrasts.
type ColConLines struct {
	FirstLine, LastLine int
	Columns             []ColConColumn
}

// ColConColumn is one pixel-control entry of a CHG_COLCON line range.
type ColConColumn struct {
	Column   int
	Color    [4]int
	Contrast [4]int
}

func spuErr(f string, a ...any) error { return fmt.Errorf("%w: %s", ErrSPU, fmt.Sprintf(f, a...)) }

// spuState is the display state a sub-picture unit's control sequences build
// up; each sequence (DCSQ) changes it at its own time.
type spuState struct {
	on, forced        bool
	color, contrast   [4]int
	x, y, w, h        int
	top, bot          int
	haveArea, havePix bool
	colcon            []ColConLines
}

// parseColCon parses the parameter area of a CHG_COLCON command: line
// control entries (first line, entry count, last line), each followed by
// its pixel control entries, ended by 0x0fffffff.
func parseColCon(b []byte) []ColConLines {
	var out []ColConLines
	for p := 0; p+4 <= len(b) && len(out) < 64; {
		ln := binary.BigEndian.Uint32(b[p:])
		p += 4
		if ln == 0x0fffffff {
			break
		}
		l := ColConLines{FirstLine: int(ln>>16) & 0xfff, LastLine: int(ln) & 0xfff}
		for n := int(ln>>12) & 0xf; n > 0 && p+6 <= len(b); n-- {
			l.Columns = append(l.Columns, ColConColumn{
				Column: int(binary.BigEndian.Uint16(b[p:])) & 0xfff, Color: nibbles(b[p+2 : p+4]), Contrast: nibbles(b[p+4 : p+6]),
			})
			p += 6
		}
		out = append(out, l)
	}
	return out
}

// DecodeSPU decodes one sub-picture unit as it first appears: the state at
// its first start-display command (or, for units that never start, the
// state they leave), with StopSec the time it next changes or stops.
func DecodeSPU(unit []byte) (*SPU, error) {
	frames, last, err := decodeSPU(unit)
	if err != nil {
		return nil, err
	}
	if len(frames) > 0 {
		return frames[0], nil
	}
	return last, nil
}

// DecodeSPUFrames decodes every picture a sub-picture unit shows over time.
// A unit carries a chain of display control sequences, each applied at its
// own delay: subtitles that change colour, move, blink or show another part
// of the bitmap come out as one SPU per state, each with its own start and
// stop time (StopSec < 0: shown until the next unit replaces it).
func DecodeSPUFrames(unit []byte) ([]*SPU, error) {
	frames, _, err := decodeSPU(unit)
	return frames, err
}

func decodeSPU(unit []byte) ([]*SPU, *SPU, error) {
	if len(unit) < 4 {
		return nil, nil, spuErr("unit shorter than header")
	}
	size := int(binary.BigEndian.Uint16(unit))
	tableAt := int(binary.BigEndian.Uint16(unit[2:]))
	if size < 4 || size > len(unit) {
		return nil, nil, spuErr("size %d vs %d bytes", size, len(unit))
	}
	if tableAt < 4 || tableAt+4 > size {
		return nil, nil, spuErr("control table outside unit")
	}
	var (
		st       spuState
		frames   []*SPU
		cur      *SPU
		firstSTP = -1.0
		lastAt   float64
		bitmaps  = map[[6]int]*SPU{}
	)
	snapshot := func(at float64) (*SPU, error) {
		if !st.haveArea {
			return nil, spuErr("no display area")
		}
		if !st.havePix {
			return nil, spuErr("no pixel data address")
		}
		if st.w <= 0 || st.h <= 0 || st.w > 4096 || st.h > 4096 {
			return nil, spuErr("display area %dx%d", st.w, st.h)
		}
		if st.top >= size || st.bot >= size {
			return nil, spuErr("pixel data outside unit")
		}
		k := [6]int{st.x, st.y, st.w, st.h, st.top, st.bot}
		bm := bitmaps[k]
		if bm == nil {
			bm = &SPU{X: st.x, Y: st.y, Width: st.w, Height: st.h, Pixels: make([]byte, st.w*st.h)}
			endOf := func(start int) int {
				end := size
				for _, v := range []int{st.top, st.bot, tableAt} {
					if v > start && v < end {
						end = v
					}
				}
				return end
			}
			t1 := decodeField(unit, st.top, endOf(st.top), bm.Pixels, st.w, st.h, 0)
			t2 := decodeField(unit, st.bot, endOf(st.bot), bm.Pixels, st.w, st.h, 1)
			bm.Truncated = t1 || t2
			if len(bitmaps) < 16 {
				bitmaps[k] = bm
			}
		}
		f := *bm
		f.Color, f.Contrast, f.ColCon, f.Forced = st.color, st.contrast, st.colcon, st.forced
		f.StartSec, f.StopSec = at, -1
		return &f, nil
	}
	seen := map[int]bool{}
	at := tableAt
	for n := 0; at+4 <= size && !seen[at] && n < 64; n++ {
		seen[at] = true
		// Sequences come in time order; a delay that goes back counts as
		// "together with the previous one".
		delay := max(float64(binary.BigEndian.Uint16(unit[at:]))*1024/90000, lastAt)
		lastAt = delay
		next := int(binary.BigEndian.Uint16(unit[at+2:]))
		changed, stop := false, false
		p := at + 4
	cmds:
		for p < size {
			op := unit[p]
			p++
			switch op {
			case spuCMDEND:
				break cmds
			case spuFSTADSP, spuSTADSP:
				if st.on && st.forced != (op == spuFSTADSP) {
					changed = true
				}
				st.on, st.forced = true, op == spuFSTADSP
			case spuSTPDSP:
				stop, st.on = true, false
				if firstSTP < 0 {
					firstSTP = delay
				}
			case spuSETCOLOR:
				if p+2 > size {
					return nil, nil, spuErr("SET_COLOR overrun")
				}
				st.color, changed = nibbles(unit[p:p+2]), true
				p += 2
			case spuSETCONTR:
				if p+2 > size {
					return nil, nil, spuErr("SET_CONTR overrun")
				}
				st.contrast, changed = nibbles(unit[p:p+2]), true
				p += 2
			case spuSETDAREA:
				if p+6 > size {
					return nil, nil, spuErr("SET_DAREA overrun")
				}
				a := int(unit[p])<<16 | int(unit[p+1])<<8 | int(unit[p+2])
				b := int(unit[p+3])<<16 | int(unit[p+4])<<8 | int(unit[p+5])
				st.x, st.y = a>>12, b>>12
				st.w, st.h = (a&0xfff)-st.x+1, (b&0xfff)-st.y+1
				st.haveArea, changed = true, true
				p += 6
			case spuSETDSPXA:
				if p+4 > size {
					return nil, nil, spuErr("SET_DSPXA overrun")
				}
				st.top = int(binary.BigEndian.Uint16(unit[p:]))
				st.bot = int(binary.BigEndian.Uint16(unit[p+2:]))
				st.havePix, changed = true, true
				p += 4
			case spuCHGCOLCON:
				if p+2 > size {
					return nil, nil, spuErr("CHG_COLCON overrun")
				}
				l := int(binary.BigEndian.Uint16(unit[p:]))
				if l < 2 || p+l > size {
					return nil, nil, spuErr("CHG_COLCON size")
				}
				st.colcon, changed = parseColCon(unit[p+2:p+l]), true
				p += l
			default:
				break cmds
			}
		}
		if cur != nil && (stop || changed) {
			cur.StopSec, cur = delay, nil
		}
		if st.on && cur == nil {
			// A start before the area or pixel address is known shows from
			// the sequence that completes them.
			if f, err := snapshot(delay); err == nil {
				cur = f
				frames = append(frames, f)
			}
		}
		if next == at {
			break
		}
		at = next
	}
	if len(frames) > 0 {
		return frames, nil, nil
	}
	// Never shown (menu artwork without a start command, or a unit that only
	// stops): the state it leaves.
	last, err := snapshot(-1)
	if err != nil {
		return nil, nil, err
	}
	last.StopSec = firstSTP
	return nil, last, nil
}

// decodeField expands the RLE for one interlaced field (rows firstRow, +2, ...).
func decodeField(unit []byte, start, end int, pixels []byte, width, height, firstRow int) bool {
	nib := start * 2
	endNib := end * 2
	next := func() int {
		if nib >= endNib {
			return -1
		}
		b := unit[nib>>1]
		var v int
		if nib&1 == 1 {
			v = int(b & 0x0f)
		} else {
			v = int(b >> 4)
		}
		nib++
		return v
	}
	for row := firstRow; row < height; row += 2 {
		x := 0
		base := row * width
		for x < width {
			v := next()
			if v < 0 {
				return true
			}
			if v < 0x4 {
				n1 := next()
				if n1 < 0 {
					return true
				}
				v = v<<4 | n1
				if v < 0x10 {
					n2 := next()
					if n2 < 0 {
						return true
					}
					v = v<<4 | n2
					if v < 0x40 {
						n3 := next()
						if n3 < 0 {
							return true
						}
						v = v<<4 | n3
					}
				}
			}
			code := byte(v & 3)
			run := v >> 2
			if run == 0 {
				run = width - x
			}
			stop := x + run
			if stop > width {
				stop = width
			}
			if code != 0 {
				for i := base + x; i < base+stop; i++ {
					pixels[i] = code
				}
			}
			x = stop
		}
		if nib&1 == 1 {
			nib++
		}
	}
	return false
}

// YCrCbToRGB converts a DVD palette entry (ITU-R BT.601 limited range).
func YCrCbToRGB(y, cr, cb int) (r, g, b uint8) {
	Y := (255.0 / 219.0) * float64(y-16)
	Cr := float64(cr - 128)
	Cb := float64(cb - 128)
	k := 255.0 / 224.0
	clamp := func(v float64) uint8 {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return uint8(v + 0.5)
	}
	return clamp(Y + k*1.402*Cr),
		clamp(Y - k*1.772*(0.114/0.587)*Cb - k*1.402*(0.299/0.587)*Cr),
		clamp(Y + k*1.772*Cb)
}

// RGBA is a premultiplied-free 8-bit RGBA bitmap.
type RGBA struct {
	X, Y          int
	Width, Height int
	Pix           []byte // 4 bytes per pixel
}

// codeTable builds the RGBA for the four pixel codes from a palette and look.
func codeTable(palette [16]uint32, look Look) [4][4]uint8 {
	var t [4][4]uint8
	for code := 0; code < 4; code++ {
		e := palette[look.Color[code]&0x0f]
		r, g, b := YCrCbToRGB(int(e>>16)&0xff, int(e>>8)&0xff, int(e)&0xff)
		t[code] = [4]uint8{r, g, b, uint8((look.Contrast[code] & 0x0f) * 17)}
	}
	return t
}

// Render paints the whole SPU with the given look (nil look = the SPU's own).
func (s *SPU) Render(palette [16]uint32, look *Look) *RGBA {
	return s.RenderRect(palette, look, s.X, s.Y, s.X+s.Width-1, s.Y+s.Height-1)
}

// RenderRect paints the part of the SPU inside the given frame rectangle
// (inclusive, frame coordinates) with the look. Pixels outside the SPU stay
// transparent. Result coordinates are the clipped rectangle.
//
// With the SPU's own look, CHG_COLCON regions recolour their part of the
// picture; a button's look (a highlight) replaces them inside the button.
func (s *SPU) RenderRect(palette [16]uint32, look *Look, x0, y0, x1, y1 int) *RGBA {
	colcon := look == nil && len(s.ColCon) > 0
	if look == nil {
		look = &Look{Color: s.Color, Contrast: s.Contrast}
	}
	if x0 < s.X {
		x0 = s.X
	}
	if y0 < s.Y {
		y0 = s.Y
	}
	if x1 > s.X+s.Width-1 {
		x1 = s.X + s.Width - 1
	}
	if y1 > s.Y+s.Height-1 {
		y1 = s.Y + s.Height - 1
	}
	if x1 < x0 || y1 < y0 {
		return &RGBA{X: x0, Y: y0}
	}
	w, h := x1-x0+1, y1-y0+1
	out := &RGBA{X: x0, Y: y0, Width: w, Height: h, Pix: make([]byte, w*h*4)}
	t := codeTable(palette, *look)
	var tables map[ColConColumn]*[4][4]uint8
	if colcon {
		tables = map[ColConColumn]*[4][4]uint8{}
	}
	for y := 0; y < h; y++ {
		sy := y0 + y - s.Y
		var cols []ColConColumn
		if colcon {
			cols = s.colConRow(y0 + y)
		}
		for x := 0; x < w; x++ {
			sx := x0 + x - s.X
			tt := &t
			if cols != nil {
				tt = colConTable(palette, cols, x0+x, tables, &t)
			}
			c := tt[s.Pixels[sy*s.Width+sx]]
			if c[3] == 0 {
				continue
			}
			i := (y*w + x) * 4
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = c[0], c[1], c[2], c[3]
		}
	}
	return out
}

// colConRow returns the CHG_COLCON pixel controls for frame line y (the
// first line range that covers it), nil when there are none.
func (s *SPU) colConRow(y int) []ColConColumn {
	for i := range s.ColCon {
		if l := &s.ColCon[i]; y >= l.FirstLine && y <= l.LastLine && len(l.Columns) > 0 {
			return l.Columns
		}
	}
	return nil
}

// colConTable is the colour table for frame column x on a CHG_COLCON line:
// the last pixel control that starts at or left of x, else the unit's own.
func colConTable(palette [16]uint32, cols []ColConColumn, x int, cache map[ColConColumn]*[4][4]uint8, own *[4][4]uint8) *[4][4]uint8 {
	k := -1
	for i, c := range cols {
		if c.Column <= x {
			k = i
		}
	}
	if k < 0 {
		return own
	}
	t := cache[cols[k]]
	if t == nil {
		v := codeTable(palette, Look{Color: cols[k].Color, Contrast: cols[k].Contrast})
		t = &v
		cache[cols[k]] = t
	}
	return t
}

// Crop returns the bitmap trimmed to its visible pixels (zero size when
// nothing is visible). Subtitle units usually cover the whole frame.
func (r *RGBA) Crop() *RGBA {
	minX, minY, maxX, maxY := r.Width, r.Height, -1, -1
	for y := 0; y < r.Height; y++ {
		row := r.Pix[y*r.Width*4 : (y+1)*r.Width*4]
		for x := 0; x < r.Width; x++ {
			if row[x*4+3] != 0 {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				maxY = y
			}
		}
	}
	if maxX < 0 {
		return &RGBA{X: r.X, Y: r.Y}
	}
	w, h := maxX-minX+1, maxY-minY+1
	out := &RGBA{X: r.X + minX, Y: r.Y + minY, Width: w, Height: h, Pix: make([]byte, w*h*4)}
	for y := 0; y < h; y++ {
		copy(out.Pix[y*w*4:(y+1)*w*4], r.Pix[((minY+y)*r.Width+minX)*4:((minY+y)*r.Width+maxX+1)*4])
	}
	return out
}

// Opaque reports whether any pixel is visible.
func (r *RGBA) Opaque() bool {
	if r == nil {
		return false
	}
	for i := 3; i < len(r.Pix); i += 4 {
		if r.Pix[i] != 0 {
			return true
		}
	}
	return false
}
