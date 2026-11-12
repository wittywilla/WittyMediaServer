// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package brandart

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/jpeg"
)

// ICO wraps PNG frames, one per size, in a Windows icon (PNG inside ICO works from Vista on and
// is what the Windows tray and installers take). draw makes each size's picture.
func ICO(sizes []int, draw func(px int) Art) []byte {
	return ICOImages(sizes, func(px int) *image.RGBA { return draw(px).Image(px, px) })
}

// ICOImages is ICO from ready-made pictures, one per size.
func ICOImages(sizes []int, draw func(px int) *image.RGBA) []byte {
	var frames [][]byte
	for _, s := range sizes {
		frames = append(frames, encodePNG(draw(s)))
	}
	var b bytes.Buffer
	le := binary.LittleEndian
	_ = binary.Write(&b, le, []uint16{0, 1, uint16(len(sizes))})
	offset := 6 + 16*len(sizes)
	for i, s := range sizes {
		d := byte(s)
		if s >= 256 {
			d = 0
		}
		b.Write([]byte{d, d, 0, 0})
		_ = binary.Write(&b, le, []uint16{1, 32})
		_ = binary.Write(&b, le, uint32(len(frames[i])))
		_ = binary.Write(&b, le, uint32(offset))
		offset += len(frames[i])
	}
	for _, f := range frames {
		b.Write(f)
	}
	return b.Bytes()
}

// WindowsSizes are the sizes Windows asks an icon for (shell, taskbar, tray, Explorer views).
var WindowsSizes = []int{16, 20, 24, 32, 40, 48, 64, 96, 128, 256}

// ICNS is a macOS icon file with every size macOS picks from, each a PNG.
func ICNS(draw func(px int) Art) []byte {
	entries := []struct {
		kind string
		px   int
	}{{"icp4", 16}, {"icp5", 32}, {"icp6", 64}, {"ic07", 128}, {"ic08", 256}, {"ic09", 512}, {"ic10", 1024}, {"ic11", 32}, {"ic12", 64}, {"ic13", 256}, {"ic14", 512}}
	cache := map[int][]byte{}
	var body bytes.Buffer
	for _, e := range entries {
		data, ok := cache[e.px]
		if !ok {
			data = draw(e.px).PNG(e.px, e.px)
			cache[e.px] = data
		}
		body.WriteString(e.kind)
		_ = binary.Write(&body, binary.BigEndian, uint32(8+len(data)))
		body.Write(data)
	}
	var out bytes.Buffer
	out.WriteString("icns")
	_ = binary.Write(&out, binary.BigEndian, uint32(8+body.Len()))
	out.Write(body.Bytes())
	return out.Bytes()
}

// BMP is the 24-bit, bottom-up bitmap Windows Installer and NSIS take for wizard pictures.
func BMP(img *image.RGBA) []byte {
	img = Opaque(img, Hex("#ffffff"))
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	stride := (w*3 + 3) &^ 3
	var out bytes.Buffer
	out.WriteString("BM")
	le := binary.LittleEndian
	for _, v := range []any{uint32(54 + stride*h), uint32(0), uint32(54), uint32(40), int32(w), int32(h), uint16(1), uint16(24), uint32(0), uint32(stride * h), int32(2835), int32(2835), uint32(0), uint32(0)} {
		_ = binary.Write(&out, le, v)
	}
	row := make([]byte, stride)
	for y := h - 1; y >= 0; y-- {
		for x := 0; x < w; x++ {
			c := img.RGBAAt(b.Min.X+x, b.Min.Y+y)
			row[x*3], row[x*3+1], row[x*3+2] = c.B, c.G, c.R
		}
		out.Write(row)
	}
	return out.Bytes()
}

// JPEG encodes a picture with no transparency (the big backdrops, where PNG would be megabytes).
func JPEG(img *image.RGBA) []byte {
	var b bytes.Buffer
	_ = jpeg.Encode(&b, Opaque(img, Hex("#000000")), &jpeg.Options{Quality: 90})
	return b.Bytes()
}
