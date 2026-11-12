// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package trayicon draws the tray and menu-bar icon at runtime from the brand drawings
// (internal/brandart, docs/BRAND.md), so no asset files need shipping: the server's mark in
// colour, grey while the server is stopped, and a one-colour template for the macOS menu bar.
package trayicon

import (
	"bytes"
	"image"
	"image/png"

	"github.com/wittywilla/wittymediaserver/internal/brandart"
)

var kit = brandart.NewKit(brandart.DefaultConcept)

// Image renders the icon at the given size. When dim is true it is grey (server stopped).
func Image(size int, dim bool) *image.RGBA {
	img := kit.Tile(brandart.Server, size).Image(size, size)
	if dim {
		for i := 0; i < len(img.Pix); i += 4 {
			r, g, b := float64(img.Pix[i]), float64(img.Pix[i+1]), float64(img.Pix[i+2])
			y := uint8((0.3*r + 0.59*g + 0.11*b) * 0.85)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2] = y, y, y
		}
	}
	return img
}

// PNG encodes the icon.
func PNG(size int, dim bool) []byte { return encode(Image(size, dim)) }

// Template is the macOS menu-bar icon: the mark in black on nothing, which macOS recolours for a
// light or dark bar. Stopped, it is faint.
func Template(size int, dim bool) []byte {
	c := brandart.Hex("#000000")
	if dim {
		c = c.WithAlpha(0.45)
	}
	return encode(kit.Mono(brandart.Server, size, c, 1.12).Image(size, size))
}

// ICO wraps the icon at the sizes the Windows tray picks from.
func ICO(dim bool) []byte {
	return brandart.ICOImages([]int{16, 20, 24, 32, 40, 48, 64, 256}, func(px int) *image.RGBA { return Image(px, dim) })
}

func encode(img image.Image) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}
