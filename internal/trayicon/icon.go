// Package trayicon draws the application icon at runtime so no asset files
// need shipping: a rounded dark tile, a violet ring and a play triangle.
package trayicon

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
)

type rgb struct{ r, g, b float64 }

func lerp(a, b rgb, t float64) rgb {
	return rgb{a.r + (b.r-a.r)*t, a.g + (b.g-a.g)*t, a.b + (b.b-a.b)*t}
}

// Image renders the icon at the given size. When dim is true the ring is grey
// (server stopped).
func Image(size int, dim bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	s := float64(size)
	cx, cy := s/2, s/2
	radius := s * 0.22
	ringR := s * 0.33
	ringW := s * 0.085
	c1, c2 := rgb{157, 123, 255}, rgb{91, 61, 245}
	if dim {
		c1, c2 = rgb{150, 150, 160}, rgb{110, 110, 120}
	}
	tile := color.NRGBA{27, 30, 37, 255}
	tx0, ty0 := cx-s*0.08, cy-s*0.16
	tx1, ty1 := cx+s*0.19, cy
	tx2, ty2 := cx-s*0.08, cy+s*0.16
	inTri := func(x, y float64) bool {
		d1 := (x-tx1)*(ty0-ty1) - (tx0-tx1)*(y-ty1)
		d2 := (x-tx2)*(ty1-ty2) - (tx1-tx2)*(y-ty2)
		d3 := (x-tx0)*(ty2-ty0) - (tx2-tx0)*(y-ty0)
		neg := d1 < 0 || d2 < 0 || d3 < 0
		pos := d1 > 0 || d2 > 0 || d3 > 0
		return !(neg && pos)
	}
	const ss = 3
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var acc [4]float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					px := float64(x) + (float64(sx)+0.5)/ss
					py := float64(y) + (float64(sy)+0.5)/ss
					dx := math.Max(math.Abs(px-cx)-(s/2-radius), 0)
					dy := math.Max(math.Abs(py-cy)-(s/2-radius), 0)
					if math.Hypot(dx, dy) > radius {
						continue
					}
					c := tile
					d := math.Hypot(px-cx, py-cy)
					g := lerp(c1, c2, (px+py)/(2*s))
					if math.Abs(d-ringR) <= ringW/2 || inTri(px, py) {
						c = color.NRGBA{uint8(g.r), uint8(g.g), uint8(g.b), 255}
					}
					acc[0] += float64(c.R)
					acc[1] += float64(c.G)
					acc[2] += float64(c.B)
					acc[3] += float64(c.A)
				}
			}
			const n = float64(ss * ss)
			img.SetNRGBA(x, y, color.NRGBA{uint8(acc[0] / n), uint8(acc[1] / n), uint8(acc[2] / n), uint8(acc[3] / n)})
		}
	}
	return img
}

// PNG encodes the icon.
func PNG(size int, dim bool) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, Image(size, dim))
	return b.Bytes()
}

// ICO wraps PNG frames in a Windows icon container (PNG-in-ICO has been
// supported since Vista and is what the Windows tray expects).
func ICO(dim bool) []byte {
	sizes := []int{16, 24, 32, 48, 64, 256}
	var frames [][]byte
	for _, s := range sizes {
		frames = append(frames, PNG(s, dim))
	}
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, []uint16{0, 1, uint16(len(sizes))})
	offset := 6 + 16*len(sizes)
	for i, s := range sizes {
		dim8 := byte(s)
		if s >= 256 {
			dim8 = 0
		}
		b.Write([]byte{dim8, dim8, 0, 0})
		_ = binary.Write(&b, binary.LittleEndian, []uint16{1, 32})
		_ = binary.Write(&b, binary.LittleEndian, uint32(len(frames[i])))
		_ = binary.Write(&b, binary.LittleEndian, uint32(offset))
		offset += len(frames[i])
	}
	for _, f := range frames {
		b.Write(f)
	}
	return b.Bytes()
}
