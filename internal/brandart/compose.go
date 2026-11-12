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

// Scene is a backdrop of the family's world: night, a violet glow, a scatter of sparks in the
// apps' colours. Wide pictures (Top Shelf, banners, installers, the website, the TV's connect
// screen) all start from it. seed changes where the sparks fall; no spark falls inside a clear
// box (where words or icons go).
func (k Kit) Scene(w, h float64, seed int, light bool, clear ...Box) Art {
	pal := k.Pal
	a := Art{W: w, H: h}
	bg1, bg2 := pal.Raised, pal.Night
	if light {
		bg1, bg2 = Mix(pal.Paper, Hex("#ffffff"), 0.6), pal.Paper
	}
	rect := Rect(0, 0, w, h, 0)
	a.Layers = append(a.Layers,
		Layer{rect, Linear(Pt{0, 0}, Pt{0, h}, bg1, bg2)},
		Layer{rect, Glow(w*0.72, h*0.35, math.Max(w, h)*0.55, pal.Accent.WithAlpha(0.55))},
		Layer{rect, Glow(w*0.12, h*1.05, math.Max(w, h)*0.45, pal.Coral.WithAlpha(0.20))},
		Layer{rect, Glow(w*0.95, h*1.0, math.Max(w, h)*0.35, pal.Mint.WithAlpha(0.16))},
	)
	colours := []Colour{pal.Amber, pal.Mint, pal.Coral, pal.Sky, pal.AccentLite}
	r := uint32(seed*2654435761 + 12345)
	next := func() float64 {
		r ^= r << 13
		r ^= r >> 17
		r ^= r << 5
		return float64(r%10000) / 10000
	}
	unit := math.Min(w, h)
	for i := 0; i < 18; i++ {
		x, y := next()*w, next()*h
		s := unit * (0.012 + next()*0.03)
		hit := false
		for _, b := range clear {
			if x+s > b.X0 && x-s < b.X1 && y+s > b.Y0 && y-s < b.Y1 {
				hit = true
			}
		}
		if hit {
			continue
		}
		c := colours[i%len(colours)].WithAlpha(0.35 + next()*0.45)
		if light {
			c = colours[i%len(colours)].WithAlpha(0.55 + next()*0.35)
		}
		a.Layers = append(a.Layers, Layer{Spark(x, y, s, 0.6).Solid(), Fill(c)})
	}
	return a
}

// Box is a rectangle kept clear of sparks.
type Box struct{ X0, Y0, X1, Y1 float64 }

// Draw paints src's pixels onto dst at (x, y).
func Draw(dst draw.Image, src image.Image, x, y int) {
	b := src.Bounds()
	draw.Draw(dst, image.Rect(x, y, x+b.Dx(), y+b.Dy()), src, b.Min, draw.Over)
}
