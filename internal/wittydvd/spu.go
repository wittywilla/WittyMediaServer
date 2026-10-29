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
	Forced        bool
	StartSec      float64
	StopSec       float64 // <0 when absent
	Truncated     bool
}

func spuErr(f string, a ...any) error { return fmt.Errorf("%w: %s", ErrSPU, fmt.Sprintf(f, a...)) }

// DecodeSPU decodes one sub-picture unit.
func DecodeSPU(unit []byte) (*SPU, error) {
	if len(unit) < 4 {
		return nil, spuErr("unit shorter than header")
	}
	size := int(binary.BigEndian.Uint16(unit))
	tableAt := int(binary.BigEndian.Uint16(unit[2:]))
	if size < 4 || size > len(unit) {
		return nil, spuErr("size %d vs %d bytes", size, len(unit))
	}
	if tableAt < 4 || tableAt+4 > size {
		return nil, spuErr("control table outside unit")
	}
	s := &SPU{StopSec: -1, StartSec: -1}
	var (
		haveArea, havePix bool
		topOff, botOff    int
	)
	seen := map[int]bool{}
	at := tableAt
	for n := 0; at+4 <= size && !seen[at] && n < 64; n++ {
		seen[at] = true
		delay := float64(binary.BigEndian.Uint16(unit[at:])) * 1024 / 90000
		next := int(binary.BigEndian.Uint16(unit[at+2:]))
		p := at + 4
	cmds:
		for p < size {
			op := unit[p]
			p++
			switch op {
			case spuCMDEND:
				break cmds
			case spuFSTADSP, spuSTADSP:
				if s.StartSec < 0 {
					s.StartSec = delay
					s.Forced = op == spuFSTADSP
				}
			case spuSTPDSP:
				if s.StopSec < 0 {
					s.StopSec = delay
				}
			case spuSETCOLOR:
				if p+2 > size {
					return nil, spuErr("SET_COLOR overrun")
				}
				s.Color = nibbles(unit[p : p+2])
				p += 2
			case spuSETCONTR:
				if p+2 > size {
					return nil, spuErr("SET_CONTR overrun")
				}
				s.Contrast = nibbles(unit[p : p+2])
				p += 2
			case spuSETDAREA:
				if p+6 > size {
					return nil, spuErr("SET_DAREA overrun")
				}
				a := int(unit[p])<<16 | int(unit[p+1])<<8 | int(unit[p+2])
				b := int(unit[p+3])<<16 | int(unit[p+4])<<8 | int(unit[p+5])
				s.X, s.Y = a>>12, b>>12
				s.Width, s.Height = (a&0xfff)-s.X+1, (b&0xfff)-s.Y+1
				haveArea = true
				p += 6
			case spuSETDSPXA:
				if p+4 > size {
					return nil, spuErr("SET_DSPXA overrun")
				}
				topOff = int(binary.BigEndian.Uint16(unit[p:]))
				botOff = int(binary.BigEndian.Uint16(unit[p+2:]))
				havePix = true
				p += 4
			case spuCHGCOLCON:
				if p+2 > size {
					return nil, spuErr("CHG_COLCON overrun")
				}
				l := int(binary.BigEndian.Uint16(unit[p:]))
				if l < 2 || p+l > size {
					return nil, spuErr("CHG_COLCON size")
				}
				p += l
			default:
				break cmds
			}
		}
		if next == at {
			break
		}
		at = next
	}
	if !haveArea {
		return nil, spuErr("no display area")
	}
	if !havePix {
		return nil, spuErr("no pixel data address")
	}
	if s.Width <= 0 || s.Height <= 0 || s.Width > 4096 || s.Height > 4096 {
		return nil, spuErr("display area %dx%d", s.Width, s.Height)
	}
	if topOff >= size || botOff >= size {
		return nil, spuErr("pixel data outside unit")
	}
	s.Pixels = make([]byte, s.Width*s.Height)
	endOf := func(start int) int {
		end := size
		for _, v := range []int{topOff, botOff, tableAt} {
			if v > start && v < end {
				end = v
			}
		}
		return end
	}
	t1 := decodeField(unit, topOff, endOf(topOff), s.Pixels, s.Width, s.Height, 0)
	t2 := decodeField(unit, botOff, endOf(botOff), s.Pixels, s.Width, s.Height, 1)
	s.Truncated = t1 || t2
	return s, nil
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
func (s *SPU) RenderRect(palette [16]uint32, look *Look, x0, y0, x1, y1 int) *RGBA {
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
	for y := 0; y < h; y++ {
		sy := y0 + y - s.Y
		for x := 0; x < w; x++ {
			sx := x0 + x - s.X
			c := t[s.Pixels[sy*s.Width+sx]]
			if c[3] == 0 {
				continue
			}
			i := (y*w + x) * 4
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = c[0], c[1], c[2], c[3]
		}
	}
	return out
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
