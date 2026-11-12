// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package brandart

import (
	"image"
	"image/draw"
	"math"
)

// Kit is everything needed to draw the family in one concept.
type Kit struct {
	Concept Concept
	Pal     Palette
}

// NewKit is the family in concept c with the house palette.
func NewKit(c Concept) Kit { return Kit{Concept: c, Pal: HousePalette()} }

// Mark is one app's mark at a size: small below 48 pixels.
func (k Kit) Mark(p Product, px int) Mark { return Design(k.Concept, p, k.Pal, px > 0 && px < 48) }

// backOn paints the app's background inside outline.
func backOn(m Mark, outline Path) []Layer {
	var out []Layer
	for _, p := range m.Back {
		out = append(out, Layer{outline, p})
	}
	return out
}

func glyphAt(layers []Layer, k float64) []Layer {
	c := Pt{Page / 2, Page / 2}
	f := func(q Pt) Pt { return c.add(q.sub(c).mul(k)) }
	var out []Layer
	for _, l := range layers {
		p := l.Paint
		p.From, p.To = f(p.From), f(p.To)
		out = append(out, Layer{l.Path.Map(f), p})
	}
	return out
}

func square() Path { return Rect(0, 0, Page, Page, 0) }

// Full is the mark filling its whole square, for platforms that cut their own shape (iOS,
// Android's launcher, the App Store, tvOS).
func (k Kit) Full(p Product, px int) Art {
	m := k.Mark(p, px)
	return Art{W: Page, H: Page, Layers: append(backOn(m, square()), m.Glyph()...)}
}

// Back is only the background of the full square.
func (k Kit) Back(p Product) Art {
	m := k.Mark(p, 0)
	return Art{W: Page, H: Page, Layers: backOn(m, square())}
}

// Glyph is the mark with no background, at scale s of the safe area.
func (k Kit) Glyph(p Product, px int, s float64) Art {
	m := k.Mark(p, px)
	return Art{W: Page, H: Page, Layers: glyphAt(m.Glyph(), s)}
}

// Mono is the mark in one colour with no background.
func (k Kit) Mono(p Product, px int, c Colour, s float64) Art {
	m := k.Mark(p, px)
	return Art{W: Page, H: Page, Layers: glyphAt([]Layer{{m.MonoPath(), Fill(c)}}, s)}
}

// Tile is the mark on its own rounded square, for the places that show the picture as it is:
// the web, Windows, Linux, the docs. margin is the clear border (a fraction of the page).
func (k Kit) Tile(p Product, px int) Art {
	m := k.Mark(p, px)
	margin, radius, scale := Page*0.03, Page*0.22, 1.36
	if px > 0 && px < 48 {
		// The smallest sizes use every pixel: no margin, a smaller radius, a bigger glyph.
		margin, radius, scale = 0, Page*0.2, 1.08
	}
	outline := Rect(margin, margin, Page-2*margin, Page-2*margin, radius)
	return Art{W: Page, H: Page, Layers: append(backOn(m, outline), glyphAt(m.Glyph(), scale)...)}
}

// MacTile is the mark on macOS's icon grid: an 824 rounded square centred on 1024, with the
// soft shadow under it that macOS icons have.
func (k Kit) MacTile(p Product, px int) Art {
	m := k.Mark(p, px)
	const side, r = 824.0, 185.0
	o := (Page - side) / 2
	outline := Rect(o, o, side, side, r)
	var layers []Layer
	for i, a := range []float64{0.10, 0.08, 0.06} {
		g := float64(i+1) * 8
		layers = append(layers, Layer{Rect(o-g/2, o+g, side+g, side+g/2, r+g/2), Fill(Hex("#000000").WithAlpha(a))})
	}
	layers = append(layers, backOn(m, outline)...)
	return Art{W: Page, H: Page, Layers: append(layers, glyphAt(m.Glyph(), side/Page*1.3)...)}
}

// Round is the mark in a circle (Android's legacy round launcher icon).
func (k Kit) Round(p Product, px int) Art {
	m := k.Mark(p, px)
	return Art{W: Page, H: Page, Layers: append(backOn(m, Circle(Page/2, Page/2, Page/2*0.92)), glyphAt(m.Glyph(), 1.16)...)}
}

// Wordmark is an app's name in the brand's way: "Media" (or "Reader") in the accent, the rest
// in c. It returns the runs.
func (k Kit) Wordmark(p Product, c, accent Colour) []Run {
	switch p {
	case Reader:
		return []Run{{"Witty", c}, {"Reader", accent}}
	case Client:
		return []Run{{"Witty", c}, {"Media", accent}, {"Client", c}}
	case Player:
		return []Run{{"Witty", c}, {"Media", accent}, {"Player", c}}
	}
	return []Run{{"Witty", c}, {"Media", accent}, {"Server", c}}
}

// Lockup is the tile and the wordmark side by side, height h, starting at (x, y). It returns the
// layers and the width used.
func (k Kit) Lockup(p Product, x, y, h float64, text, accent Colour) ([]Layer, float64) {
	tile := k.Tile(p, int(h)).Placed(0, 0, Pt{x + h/2, y + h/2}, h/Page)
	size := h * 0.5
	runs := k.Wordmark(p, text, accent)
	words, w := TextArt(runs, x+h*1.22, y+h/2+size*0.36, size, true)
	return append(tile.Layers, words...), h*1.22 + w
}

// Scene is a backdrop of the family's world, kept quiet so it never competes with what is in
// front of it: the deep night of the house theme, one soft violet light, a few thin orbits like
// the logo's ring (each open at a gap with its small dot in it, in the apps' dot colours), and a
// sparse field of tiny, dim stars. Wide pictures (Top Shelf, banners, installers, the website,
// the TV's connect screen) all start from it. seed turns the orbits and places the stars; no
// star or dot falls inside a clear box (where words or icons go).
func (k Kit) Scene(w, h float64, seed int, light bool, clear ...Box) Art {
	pal := k.Pal
	a := Art{W: w, H: h}
	unit, far := math.Min(w, h), math.Max(w, h)
	bg1, bg2 := Mix(pal.Night, pal.Raised, 0.35), pal.Night
	ring, glow := pal.AccentLite.WithAlpha(0.10), pal.Accent.WithAlpha(0.30)
	starC := pal.Text
	if light {
		bg1, bg2 = Mix(pal.Paper, Hex("#ffffff"), 0.6), pal.Paper
		ring, glow = pal.Accent.WithAlpha(0.12), pal.Accent.WithAlpha(0.10)
		starC = pal.Accent
	}
	rect := Rect(0, 0, w, h, 0)
	// The orbits' centre sits right of middle, so the left (where screens put words) stays calmest.
	c := Pt{w * 0.74, h * 0.56}
	a.Layers = append(a.Layers,
		Layer{rect, Linear(Pt{0, 0}, Pt{0, h}, bg1, bg2)},
		Layer{rect, Glow(c.X, c.Y, far*0.5, glow)},
	)
	r := uint32(seed)*2654435761 + 12345 // wraps the same on 32-bit and 64-bit
	next := func() float64 {
		r ^= r << 13
		r ^= r >> 17
		r ^= r << 5
		return float64(r%10000) / 10000
	}
	free := func(x, y, s float64) bool {
		for _, b := range clear {
			if x+s > b.X0 && x-s < b.X1 && y+s > b.Y0 && y-s < b.Y1 {
				return false
			}
		}
		return true
	}
	dots := []Colour{pal.Amber, pal.Mint, pal.Sky, pal.Coral}
	turn := next() * 2 * math.Pi
	for i, rr := range []float64{0.30, 0.52, 0.78, 1.08} {
		rad := unit * rr
		width := math.Max(unit*0.0035, 1.2)
		gap := 0.32 - float64(i)*0.05 // radians each side of the dot
		at := turn + float64(i)*2.1
		a.Layers = append(a.Layers, Layer{StrokeArc(c.X, c.Y, rad, width, at+gap, at+2*math.Pi-gap).Solid(), Fill(ring)})
		d := polar(c, rad, at)
		ds := math.Max(unit*0.009, 1.5)
		if free(d.X, d.Y, ds) {
			alpha := 0.55
			if light {
				alpha = 0.7
			}
			a.Layers = append(a.Layers, Layer{Circle(d.X, d.Y, ds).Solid(), Fill(dots[i%len(dots)].WithAlpha(alpha))})
		}
	}
	// Stars: few, tiny, dim, never twinkling shapes.
	for i := 0; i < int(math.Min(40, w*h/40000)); i++ {
		x, y := next()*w, next()*h
		s := math.Max(unit*(0.0012+next()*0.0018), 0.6)
		if !free(x, y, s*4) {
			continue
		}
		a.Layers = append(a.Layers, Layer{Circle(x, y, s).Solid(), Fill(starC.WithAlpha(0.10 + next()*0.18))})
	}
	return a
}

// Box is a rectangle kept clear of the scene's stars and dots.
type Box struct{ X0, Y0, X1, Y1 float64 }

// Draw paints src's pixels onto dst at (x, y).
func Draw(dst draw.Image, src image.Image, x, y int) {
	b := src.Bounds()
	draw.Draw(dst, image.Rect(x, y, x+b.Dx(), y+b.Dy()), src, b.Min, draw.Over)
}
