package wittybluray

import (
	"errors"
	"image"
	"image/color"
)

// The graphics streams: menus (IG) and subtitles (PG) are made of segments
// carried in PES packets: palettes (PDS), run-length-coded pictures (ODS),
// and compositions that place pictures in windows (PCS for subtitles, ICS
// for menus). The decoders here are shared by ig.go and pg.go.

const (
	segPDS = 0x14
	segODS = 0x15
	segPCS = 0x16
	segWDS = 0x17
	segICS = 0x18
	segEnd = 0x80
)

// segment is one graphics segment.
type segment struct {
	typ  byte
	data []byte
	pts  uint64 // 90 kHz, from the PES packet
}

// splitSegments splits a PES payload into its segments.
func splitSegments(payload []byte, pts uint64, out []segment) []segment {
	for off := 0; off+3 <= len(payload); {
		typ := payload[off]
		l := int(payload[off+1])<<8 | int(payload[off+2])
		off += 3
		if off+l > len(payload) {
			break
		}
		out = append(out, segment{typ: typ, data: payload[off : off+l], pts: pts})
		off += l
	}
	return out
}

// Palette is 256 colours. Entries a segment does not set are transparent.
type Palette [256]color.RGBA

// parsePDS reads a palette definition. hd selects BT.709 colour conversion
// (HD video) rather than BT.601.
func parsePDS(data []byte, hd bool) (id int, p Palette) {
	if len(data) < 2 {
		return 0, p
	}
	id = int(data[0])
	for off := 2; off+5 <= len(data); off += 5 {
		y, cr, cb, a := float64(data[off+1]), float64(data[off+2]), float64(data[off+3]), data[off+4]
		var r, g, b float64
		yy := (y - 16) * 255 / 219
		cr, cb = cr-128, cb-128
		if hd {
			r, g, b = yy+1.5748*cr*255/224, yy-0.1873*cb*255/224-0.4681*cr*255/224, yy+1.8556*cb*255/224
		} else {
			r, g, b = yy+1.402*cr*255/224, yy-0.3441*cb*255/224-0.7141*cr*255/224, yy+1.772*cb*255/224
		}
		p[data[off]] = color.RGBA{clamp8(r), clamp8(g), clamp8(b), a}
	}
	return id, p
}

func clamp8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v + 0.5)
}

// Object is one run-length-coded picture, as indexes into a palette.
type Object struct {
	ID            int
	Width, Height int
	Pixels        []byte // Width*Height palette indexes
}

// odsAssembler puts together pictures that span several ODS segments.
type odsAssembler struct {
	id       int
	w, h     int
	want     int
	data     []byte
	complete map[int]*Object
}

func newODSAssembler() *odsAssembler { return &odsAssembler{complete: map[int]*Object{}} }

// feed adds one ODS segment; a finished picture is stored by id.
func (a *odsAssembler) feed(data []byte) {
	if len(data) < 4 {
		return
	}
	id := int(data[0])<<8 | int(data[1])
	flags := data[3]
	if flags&0x80 != 0 { // first fragment
		if len(data) < 11 {
			return
		}
		a.id = id
		a.want = int(data[4])<<16 | int(data[5])<<8 | int(data[6])
		a.w = int(data[7])<<8 | int(data[8])
		a.h = int(data[9])<<8 | int(data[10])
		a.data = append(a.data[:0], data[11:]...)
	} else if id == a.id {
		a.data = append(a.data, data[4:]...)
	} else {
		return
	}
	if flags&0x40 != 0 || len(a.data)+4 >= a.want { // last fragment
		if a.w > 0 && a.h > 0 && a.w <= 4096 && a.h <= 4096 {
			if pix, err := decodeRLE(a.data, a.w, a.h); err == nil {
				a.complete[a.id] = &Object{ID: a.id, Width: a.w, Height: a.h, Pixels: pix}
			}
		}
		a.data = a.data[:0]
		a.want = 0
	}
}

// decodeRLE expands the picture's run-length code: a non-zero byte is one
// pixel of that colour; zero starts a code byte whose bits say whether a
// colour follows (else colour 0) and whether the run length is 6 or 14 bits;
// a zero code byte ends the line.
func decodeRLE(data []byte, w, h int) ([]byte, error) {
	pix := make([]byte, w*h)
	x, y := 0, 0
	for i := 0; i < len(data) && y < h; {
		b := data[i]
		i++
		if b != 0 {
			if x < w {
				pix[y*w+x] = b
			}
			x++
			continue
		}
		if i >= len(data) {
			break
		}
		f := data[i]
		i++
		if f == 0 {
			x, y = 0, y+1
			continue
		}
		run := int(f & 0x3f)
		if f&0x40 != 0 {
			if i >= len(data) {
				break
			}
			run = run<<8 | int(data[i])
			i++
		}
		c := byte(0)
		if f&0x80 != 0 {
			if i >= len(data) {
				break
			}
			c = data[i]
			i++
		}
		for k := 0; k < run; k++ {
			if x < w {
				pix[y*w+x] = c
			}
			x++
		}
	}
	if y == 0 && x == 0 {
		return nil, errors.New("empty picture")
	}
	return pix, nil
}

// draw paints an object with a palette into img at (x, y), cropped to the
// object's crop rectangle when cw > 0.
func draw(img *image.NRGBA, o *Object, p *Palette, x, y, cx, cy, cw, ch int) {
	if cw <= 0 || ch <= 0 {
		cx, cy, cw, ch = 0, 0, o.Width, o.Height
	}
	b := img.Bounds()
	for j := 0; j < ch; j++ {
		sy := cy + j
		dy := y + j
		if sy < 0 || sy >= o.Height || dy < b.Min.Y || dy >= b.Max.Y {
			continue
		}
		for i := 0; i < cw; i++ {
			sx := cx + i
			dx := x + i
			if sx < 0 || sx >= o.Width || dx < b.Min.X || dx >= b.Max.X {
				continue
			}
			c := p[o.Pixels[sy*o.Width+sx]]
			if c.A == 0 {
				continue
			}
			k := img.PixOffset(dx, dy)
			if c.A == 255 {
				img.Pix[k], img.Pix[k+1], img.Pix[k+2], img.Pix[k+3] = c.R, c.G, c.B, 255
				continue
			}
			// Blend over what is there (straight alpha).
			a := uint32(c.A)
			ia := 255 - a
			da := uint32(img.Pix[k+3])
			outA := a + da*ia/255
			if outA == 0 {
				continue
			}
			for n, v := range [3]uint8{c.R, c.G, c.B} {
				img.Pix[k+n] = uint8((uint32(v)*a + uint32(img.Pix[k+n])*da*ia/255) / outA)
			}
			img.Pix[k+3] = uint8(outA)
		}
	}
}

// Window is a rectangle of the screen that compositions draw into.
type Window struct {
	ID         int
	X, Y, W, H int
}

func parseWindows(data []byte) []Window {
	if len(data) < 1 {
		return nil
	}
	n := int(data[0])
	var out []Window
	for i, off := 0, 1; i < n && off+9 <= len(data); i, off = i+1, off+9 {
		out = append(out, Window{ID: int(data[off]), X: int(data[off+1])<<8 | int(data[off+2]), Y: int(data[off+3])<<8 | int(data[off+4]), W: int(data[off+5])<<8 | int(data[off+6]), H: int(data[off+7])<<8 | int(data[off+8])})
	}
	return out
}

// compObject places an object in a composition.
type compObject struct {
	object, window int
	forced         bool
	x, y           int
	cx, cy, cw, ch int // crop, when cw > 0
}

// parseCompObject reads one composition object; returns the bytes used.
func parseCompObject(data []byte) (compObject, int) {
	if len(data) < 8 {
		return compObject{}, len(data)
	}
	o := compObject{object: int(data[0])<<8 | int(data[1]), window: int(data[2]), forced: data[3]&0x40 != 0, x: int(data[4])<<8 | int(data[5]), y: int(data[6])<<8 | int(data[7])}
	n := 8
	if data[3]&0x80 != 0 && len(data) >= 16 {
		o.cx, o.cy, o.cw, o.ch = int(data[8])<<8|int(data[9]), int(data[10])<<8|int(data[11]), int(data[12])<<8|int(data[13]), int(data[14])<<8|int(data[15])
		n = 16
	}
	return o, n
}
