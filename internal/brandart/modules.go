// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package brandart

import (
	"image"
	"image/color"
	"math"
	"sort"
)

// Module icons: each module's own white mark on the family's tile, in its category's colour,
// with the family's spark in the corner. The marks are signed-distance shapes (a function of x,
// y that is negative inside), measured on a 128 grid, as they were in WittyModuleCatalog's old
// scripts/make-icons.mjs. Each is an original drawing: never a brand's, console's, disc
// format's, service's or studio's logo, or anything like one.

type sdf func(x, y float64) float64

func sdCircle(cx, cy, r float64) sdf {
	return func(x, y float64) float64 { return math.Hypot(x-cx, y-cy) - r }
}
func sdRing(cx, cy, r, w float64) sdf {
	return func(x, y float64) float64 { return math.Abs(math.Hypot(x-cx, y-cy)-r) - w }
}
func sdBox(cx, cy, hw, hh, r float64) sdf {
	return func(x, y float64) float64 {
		qx, qy := math.Abs(x-cx)-hw+r, math.Abs(y-cy)-hh+r
		return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - r
	}
}
func sdLine(ax, ay, bx, by, r float64) sdf {
	return func(x, y float64) float64 {
		dx, dy := bx-ax, by-ay
		t := math.Max(0, math.Min(1, ((x-ax)*dx+(y-ay)*dy)/(dx*dx+dy*dy)))
		return math.Hypot(x-ax-t*dx, y-ay-t*dy) - r
	}
}
func sdArc(cx, cy, r, w, from, to float64) sdf {
	a, b := from*math.Pi/180, to*math.Pi/180
	e0 := Pt{cx + r*math.Cos(a), cy + r*math.Sin(a)}
	e1 := Pt{cx + r*math.Cos(b), cy + r*math.Sin(b)}
	return func(x, y float64) float64 {
		t := math.Atan2(y-cy, x-cx)
		if t < a {
			t += 2 * math.Pi
		}
		if t <= b {
			return math.Abs(math.Hypot(x-cx, y-cy)-r) - w
		}
		return math.Min(math.Hypot(x-e0.X, y-e0.Y), math.Hypot(x-e1.X, y-e1.Y)) - w
	}
}
func sdPoly(round float64, pts ...Pt) sdf {
	return func(x, y float64) float64 {
		d := math.Inf(1)
		inside := false
		for i, j := 0, len(pts)-1; i < len(pts); j, i = i, i+1 {
			a, b := pts[i], pts[j]
			ex, ey := b.X-a.X, b.Y-a.Y
			t := math.Max(0, math.Min(1, ((x-a.X)*ex+(y-a.Y)*ey)/(ex*ex+ey*ey)))
			d = math.Min(d, math.Hypot(x-a.X-t*ex, y-a.Y-t*ey))
			if (a.Y > y) != (b.Y > y) && x < a.X+(y-a.Y)/(b.Y-a.Y)*ex {
				inside = !inside
			}
		}
		if inside {
			d = -d
		}
		return d - round
	}
}
func sdUnion(s ...sdf) sdf {
	return func(x, y float64) float64 {
		d := math.Inf(1)
		for _, f := range s {
			d = math.Min(d, f(x, y))
		}
		return d
	}
}
func sdCut(base sdf, holes ...sdf) sdf {
	return func(x, y float64) float64 {
		d := base(x, y)
		for _, h := range holes {
			d = math.Max(d, -h(x, y))
		}
		return d
	}
}
func sdBoth(a, b sdf) sdf {
	return func(x, y float64) float64 { return math.Max(a(x, y), b(x, y)) }
}
func sdMoved(s sdf, dx, dy float64) sdf {
	return func(x, y float64) float64 { return s(x-dx, y-dy) }
}

func sdBurst(cx, cy, outer, inner float64, n int) sdf {
	var pts []Pt
	for i := 0; i < n*2; i++ {
		a := float64(i)*math.Pi/float64(n) - math.Pi/2
		r := outer
		if i%2 == 1 {
			r = inner
		}
		pts = append(pts, Pt{cx + r*math.Cos(a), cy + r*math.Sin(a)})
	}
	return sdPoly(1.5, pts...)
}

func p(x, y float64) Pt { return Pt{x, y} }

var moduleDisc = sdCircle(64, 64, 40)

// ModuleMarks is every module's mark, by module id.
var ModuleMarks = map[string]sdf{
	// The reader under the disc engines: plain concentric rings.
	"wittydisc": sdUnion(sdRing(64, 64, 35, 6), sdRing(64, 64, 18, 5), sdCircle(64, 64, 4.5)),
	// A disc you can play.
	"wittydvd": sdCut(moduleDisc, sdPoly(3, p(52, 44), p(52, 84), p(86, 64))),
	// A disc with its hole and a glint, for its bright, high-resolution pictures.
	"wittybluray": sdCut(moduleDisc, sdCircle(64, 64, 9),
		sdPoly(0.5, p(81, 34), p(84.5, 44.5), p(95, 48), p(84.5, 51.5), p(81, 62), p(77.5, 51.5), p(67, 48), p(77.5, 44.5))),
	// A disc with a music note.
	"wittycd": sdCut(moduleDisc, sdCircle(57, 80, 10), sdBox(64, 60, 3.5, 21, 1), sdPoly(1, p(63, 39), p(83, 47), p(83, 59), p(63, 51))),
	// An open book.
	"wittyebook": sdUnion(sdPoly(3, p(25, 40), p(60, 48), p(60, 94), p(25, 86)), sdPoly(3, p(103, 40), p(68, 48), p(68, 94), p(103, 86))),
	// A page with a turned corner and lines of text.
	"wittypdf": sdCut(sdPoly(3, p(41, 27), p(70, 27), p(90, 47), p(90, 101), p(41, 101)),
		sdLine(69, 29, 69, 49, 2.2), sdLine(69, 49, 90, 49, 2.2), sdLine(51, 62, 80, 62, 3.2), sdLine(51, 74, 80, 74, 3.2), sdLine(51, 86, 68, 86, 3.2)),
	// A comic's burst with an exclamation mark.
	"wittycomics": sdCut(sdBurst(64, 64, 49, 32, 11), sdPoly(1, p(59, 42), p(69, 42), p(67, 70), p(61, 70)), sdCircle(64, 81, 5.5)),
	// A pen nib.
	"wittyink": sdUnion(sdBox(64, 28, 15, 5, 2),
		sdCut(sdPoly(2, p(50, 38), p(78, 38), p(90, 64), p(64, 104), p(38, 64)), sdCircle(64, 64, 6.5), sdLine(64, 66, 64, 106, 1.8))),
	// A folder.
	"wittyintegratedfilebrowser": sdCut(sdUnion(sdBox(46, 40, 20, 10, 5), sdBox(64, 68, 42, 30, 7)), sdLine(16, 52, 112, 52, 2.2)),
	// A jigsaw-like piece.
	"wittymodule": sdMoved(sdCut(sdUnion(sdBox(58, 70, 28, 28, 5), sdCircle(58, 37, 11), sdCircle(91, 70, 11)), sdCircle(30, 70, 9), sdCircle(58, 98, 9)), -2, -2),
	// Mountains and the sun, in a picture's frame.
	"wittyphotos": sdUnion(sdCut(sdBox(64, 64, 44, 36, 9), sdBox(64, 64, 36, 28, 4)), sdCircle(83, 48, 7),
		sdBoth(sdBox(64, 64, 36, 28, 4), sdPoly(0, p(28, 94), p(52, 58), p(66, 78), p(77, 66), p(104, 94)))),
	// A D-pad's cross.
	"wittyplay": sdCut(sdUnion(sdBox(64, 64, 15, 42, 6), sdBox(64, 64, 42, 15, 6)), sdCircle(64, 64, 7.5)),
	// A sofa.
	"wittycouch": sdUnion(sdBox(64, 49, 28, 16, 7), sdBox(23, 69, 10, 20, 7), sdBox(105, 69, 10, 20, 7), sdBox(64, 78, 28, 11, 3), sdBox(32, 94, 5, 6, 1.5), sdBox(96, 94, 5, 6, 1.5)),
	// A source, and what it passes on.
	"wittycouchhost": sdUnion(sdLine(27, 38, 27, 90, 6.5), sdLine(49, 38, 73, 64, 6.5), sdLine(73, 64, 49, 90, 6.5), sdLine(75, 38, 99, 64, 6.5), sdLine(99, 64, 75, 90, 6.5)),
	// A graphics card with two fans.
	"wittyrigs": sdUnion(sdCut(sdBox(71, 58, 38, 25, 5), sdCircle(53, 58, 16), sdCircle(89, 58, 16)), sdCircle(53, 58, 6), sdCircle(89, 58, 6), sdBox(23, 64, 5, 36, 2), sdBox(65, 90, 24, 4.5, 1)),
	// A tower with waves.
	"wittytuner": sdMoved(sdUnion(sdLine(64, 48, 46, 100, 4.5), sdLine(64, 48, 82, 100, 4.5), sdLine(54, 80, 74, 80, 4), sdCircle(64, 44, 8),
		sdArc(64, 44, 22, 4.5, -45, 45), sdArc(64, 44, 36, 4.5, -45, 45), sdArc(64, 44, 22, 4.5, 135, 225), sdArc(64, 44, 36, 4.5, 135, 225)), 0, 4),
	// Sound levels around a centre line.
	"wittyvisualizer": sdUnion(sdLine(28, 54, 28, 74, 6), sdLine(46, 40, 46, 88, 6), sdLine(64, 26, 64, 102, 6), sdLine(82, 44, 82, 84, 6), sdLine(100, 56, 100, 72, 6)),
	// A bar chart on its baseline.
	"wittystats": sdUnion(sdBox(36, 76, 9, 18, 2), sdBox(64, 60, 9, 34, 2), sdBox(92, 46, 9, 48, 2), sdBox(64, 101, 41, 3.5, 1.5)),
}

// ModuleIDs is every module with a mark, sorted.
func ModuleIDs() []string {
	var ids []string
	for id := range ModuleMarks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// CategoryColours are the top and bottom of each catalogue category's gradient, all dark enough
// for a white mark.
var CategoryColours = map[string][2]string{
	"discs":  {"#5B57E8", "#3B2FA8"},
	"games":  {"#17A34A", "#14622F"},
	"tv":     {"#E0651E", "#A23A0C"},
	"read":   {"#0F8C80", "#0B5A55"},
	"photos": {"#DD3A82", "#9B1650"},
	"listen": {"#A63CD6", "#6A1C96"},
	"watch":  {"#2F6FEB", "#1C3FA0"},
	"tools":  {"#1686A6", "#125C75"},
	"stats":  {"#5E8C14", "#3D5E0D"},
	"system": {"#566579", "#28323F"},
}

// ModuleIcon draws a module's icon at px×px the way the apps' icons are drawn: the family's dark
// tile with a glow of the module's category colour, its mark in that colour, and the family's
// amber dot in the corner.
func (k Kit) ModuleIcon(id, category string, px int) *image.RGBA {
	mark, ok := ModuleMarks[id]
	cols, ok2 := CategoryColours[category]
	if !ok || !ok2 {
		return nil
	}
	s := float64(px)
	tile := Rect(0, 0, s, s, s*0.22)
	dark := k.Pal.TileColour()
	top, bottom := Mix(Hex(cols[0]), Hex("#ffffff"), 0.25), Hex(cols[0])
	art := Art{W: s, H: s, Layers: []Layer{
		{tile, Linear(Pt{0, 0}, Pt{0, s}, Mix(dark, k.Pal.Raised, 0.5), Mix(dark, k.Pal.Night, 0.5))},
		{tile, Glow(s/2, s*0.45, s*0.6, bottom.WithAlpha(0.28))},
		{Circle(s*0.86, s*0.14, s*0.05).Solid(), Fill(k.Pal.Amber)},
	}}
	img := art.Image(px, px)
	g := 128 / s
	cover := func(d float64) float64 { return math.Max(0, math.Min(1, 0.5-d)) }
	for y := 0; y < px; y++ {
		for x := 0; x < px; x++ {
			c := img.RGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			mx, my := (float64(x)+0.5)*g, (float64(y)+0.5)*g
			on := cover(mark(mx, my) / g)
			if on == 0 {
				continue
			}
			ink := Mix(top, bottom, float64(y)/s)
			f := func(v, m uint8) uint8 {
				return uint8(math.Round(float64(v) + (float64(m)*float64(c.A)/255-float64(v))*on))
			}
			img.SetRGBA(x, y, color.RGBA{f(c.R, ink.R), f(c.G, ink.G), f(c.B, ink.B), c.A})
		}
	}
	return img
}
