// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package brandart

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
)

// Sheet draws the comparison sheet: every concept's server and client marks at real pixel
// sizes on dark, light and TV backgrounds, the tray versions, and the player and reader. Each
// icon is rasterised at its own size, so the small ones look exactly as they will.
func Sheet(concepts []Concept, chosen Concept) *image.RGBA {
	pal := HousePalette()
	const W = 2200
	const blockH = 640
	H := 140 + blockH*len(concepts)
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	draw.Draw(img, img.Bounds(), image.NewUniform(rgba(pal.Night)), image.Point{}, draw.Src)
	text := func(s string, x, y, size float64, c Colour, bold bool) {
		layers, w := TextArt([]Run{{s, c}}, 0, size, size, bold)
		if w < 1 {
			return
		}
		a := Art{W: w + 4, H: size * 1.4, Layers: layers}
		Draw(img, a.Image(int(w+4), int(size*1.4)), int(x), int(y-size))
	}
	text(fmt.Sprintf("WittyMediaServer family: %d concepts, server and client marks", len(concepts)), 40, 70, 34, pal.Text, true)
	text("Every icon below is drawn at its real pixel size. Panels: dark (house theme), light, a TV home row. Tray: one colour, running and stopped.", 40, 112, 18, Mix(pal.Text, pal.Night, 0.3), false)

	for i, c := range concepts {
		k := Kit{Concept: c, Pal: pal}
		top := 140 + i*blockH
		label := fmt.Sprintf("%d. %s", i+1, c)
		if c == chosen {
			label += "  (chosen)"
		}
		text(label, 40, float64(top+44), 28, pal.AccentLite, true)
		panels := []struct {
			x, w  int
			bg    Colour
			light bool
		}{{40, 620, pal.Slate, false}, {680, 620, pal.Paper, true}}
		for _, pn := range panels {
			py := top + 64
			draw.Draw(img, image.Rect(pn.x, py, pn.x+pn.w, py+540), image.NewUniform(rgba(pn.bg)), image.Point{}, draw.Src)
			fg := pal.Text
			if pn.light {
				fg = pal.Night
			}
			for row, p := range []Product{Server, Client} {
				y := py + 24 + row*200
				text(p.Name(), float64(pn.x+20), float64(y+16), 16, fg, true)
				x := pn.x + 20
				for _, s := range []int{128, 64, 48, 32, 16} {
					Draw(img, k.Tile(p, s).Image(s, s), x, y+30+(128-s))
					x += s + 22
				}
				// Tray: the template (one colour) version at 22 and 44 (a 2× screen).
				for _, s := range []int{22, 44} {
					Draw(img, k.Mono(p, s, fg, 1.12).Image(s, s), x, y+30+(128-s))
					x += s + 16
				}
				if p == Server {
					Draw(img, k.Mono(p, 22, Mix(fg, pn.bg, 0.55), 1.12).Image(22, 22), x, y+30+106)
				}
			}
			y := py + 24 + 400
			x := pn.x + 20
			for _, p := range []Product{Player, Reader} {
				text(p.Name(), float64(x), float64(y+16), 16, fg, true)
				Draw(img, k.Tile(p, 96).Image(96, 96), x, y+26)
				Draw(img, k.Tile(p, 32).Image(32, 32), x+110, y+26+64)
				Draw(img, k.Tile(p, 16).Image(16, 16), x+156, y+26+80)
				x += 300
			}
		}
		// TV: a home row on a scene, tvOS icons at their real 400×240, an Android TV banner.
		tx, ty := 1320, top+64
		scene := k.Scene(840, 540, 3, false)
		Draw(img, scene.Image(840, 540), tx, ty)
		text("TV home row", float64(tx+24), float64(ty+36), 18, pal.Text, true)
		Draw(img, rounded(k.TVIcon(Client, 400, 240).Image(400, 240), 22), tx+24, ty+60)
		Draw(img, rounded(k.TVIcon(Server, 400, 240).Image(400, 240), 22), tx+440, ty+60)
		Draw(img, rounded(k.Banner(Client, 320, 180).Image(320, 180), 12), tx+24, ty+330)
		Draw(img, k.Tile(Client, 180).Image(180, 180), tx+380, ty+330)
		Draw(img, k.Tile(Server, 180).Image(180, 180), tx+590, ty+330)
	}
	return img
}

func rgba(c Colour) color.RGBA { return color.RGBA{c.R, c.G, c.B, 255} }

// rounded clears img's corners to radius r, the way a TV draws its icons.
func rounded(img *image.RGBA, r float64) *image.RGBA {
	b := img.Bounds()
	mask := Art{W: float64(b.Dx()), H: float64(b.Dy()), Layers: []Layer{{Rect(0, 0, float64(b.Dx()), float64(b.Dy()), r), Fill(Hex("#ffffff"))}}}.Image(b.Dx(), b.Dy())
	out := image.NewRGBA(b)
	draw.DrawMask(out, b, img, b.Min, mask, b.Min, draw.Over)
	return out
}
