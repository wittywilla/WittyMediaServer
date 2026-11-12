// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package brandart draws every logo, icon and background of the WittyMediaServer family from
// code. Everything starts as vector paths (lines and cubic curves), so the same drawing is
// written as SVG, as an Android vector drawable, and rasterised to any size of PNG, ICO, ICNS
// or BMP. scripts/genicon is the command that writes the files; docs/BRAND.md explains the marks.
package brandart

import (
	"math"
)

// Pt is a point in a drawing's own units.
type Pt struct{ X, Y float64 }

func (p Pt) add(q Pt) Pt           { return Pt{p.X + q.X, p.Y + q.Y} }
func (p Pt) sub(q Pt) Pt           { return Pt{p.X - q.X, p.Y - q.Y} }
func (p Pt) mul(k float64) Pt      { return Pt{p.X * k, p.Y * k} }
func lerpPt(a, b Pt, t float64) Pt { return Pt{a.X + (b.X-a.X)*t, a.Y + (b.Y-a.Y)*t} }

// elem is one piece of an outline: a straight line (C1, C2 unused) or a cubic curve, to To.
type elem struct {
	Curve      bool
	C1, C2, To Pt
}

// sub is one closed outline.
type sub struct {
	Start Pt
	Elems []elem
}

// Path is a set of closed outlines, filled with the non-zero rule: an outline that runs the
// other way round from the one around it is a hole. Cut and Solid set the directions, so a
// drawing never has to think about it.
type Path struct{ subs []sub }

// Add puts other's outlines into p.
func (p Path) Add(others ...Path) Path {
	out := Path{subs: append([]sub(nil), p.subs...)}
	for _, o := range others {
		out.subs = append(out.subs, o.subs...)
	}
	return out
}

// Map moves every point through f (scale, move, mirror).
func (p Path) Map(f func(Pt) Pt) Path {
	out := Path{subs: make([]sub, len(p.subs))}
	for i, s := range p.subs {
		n := sub{Start: f(s.Start), Elems: make([]elem, len(s.Elems))}
		for j, e := range s.Elems {
			n.Elems[j] = elem{Curve: e.Curve, C1: f(e.C1), C2: f(e.C2), To: f(e.To)}
		}
		out.subs[i] = n
	}
	return out
}

// Scaled grows the path by k about the point c.
func (p Path) Scaled(c Pt, k float64) Path {
	return p.Map(func(q Pt) Pt { return c.add(q.sub(c).mul(k)) })
}

// Moved shifts the path.
func (p Path) Moved(dx, dy float64) Path {
	return p.Map(func(q Pt) Pt { return Pt{q.X + dx, q.Y + dy} })
}

// MirroredX flips the path left to right about x = cx.
func (p Path) MirroredX(cx float64) Path {
	return p.Map(func(q Pt) Pt { return Pt{2*cx - q.X, q.Y} }).Solid()
}

func (s sub) reversed() sub {
	if len(s.Elems) == 0 {
		return s
	}
	n := sub{Start: s.Elems[len(s.Elems)-1].To}
	for i := len(s.Elems) - 1; i >= 0; i-- {
		from := s.Start
		if i > 0 {
			from = s.Elems[i-1].To
		}
		e := s.Elems[i]
		n.Elems = append(n.Elems, elem{Curve: e.Curve, C1: e.C2, C2: e.C1, To: from})
	}
	return n
}

// area is the signed area of the outline, flattened: positive when it runs clockwise on a
// y-down page.
func (s sub) area() float64 {
	pts := s.flatten(8)
	a := 0.0
	for i := range pts {
		j := (i + 1) % len(pts)
		a += pts[i].X*pts[j].Y - pts[j].X*pts[i].Y
	}
	return a / 2
}

func (s sub) flatten(steps int) []Pt {
	pts := []Pt{s.Start}
	cur := s.Start
	for _, e := range s.Elems {
		if !e.Curve {
			pts = append(pts, e.To)
		} else {
			for i := 1; i <= steps; i++ {
				pts = append(pts, cubicAt(cur, e.C1, e.C2, e.To, float64(i)/float64(steps)))
			}
		}
		cur = e.To
	}
	return pts
}

func cubicAt(a, b, c, d Pt, t float64) Pt {
	u := 1 - t
	return Pt{
		u*u*u*a.X + 3*u*u*t*b.X + 3*u*t*t*c.X + t*t*t*d.X,
		u*u*u*a.Y + 3*u*u*t*b.Y + 3*u*t*t*c.Y + t*t*t*d.Y,
	}
}

func (p Path) oriented(clockwise bool) Path {
	out := Path{subs: make([]sub, len(p.subs))}
	for i, s := range p.subs {
		if (s.area() > 0) != clockwise {
			s = s.reversed()
		}
		out.subs[i] = s
	}
	return out
}

// Solid makes every outline of p fill (none is a hole).
func (p Path) Solid() Path { return p.oriented(true) }

// Cut is base with holes punched where the holes are. Holes must lie inside base and not
// overlap each other.
func Cut(base Path, holes ...Path) Path {
	out := base.Solid()
	for _, h := range holes {
		out = out.Add(h.oriented(false))
	}
	return out
}

// Bounds is the box around the path's points (control points included, which is close enough
// for laying things out).
func (p Path) Bounds() (min, max Pt) {
	min, max = Pt{math.Inf(1), math.Inf(1)}, Pt{math.Inf(-1), math.Inf(-1)}
	see := func(q Pt) {
		min.X, min.Y = math.Min(min.X, q.X), math.Min(min.Y, q.Y)
		max.X, max.Y = math.Max(max.X, q.X), math.Max(max.Y, q.Y)
	}
	for _, s := range p.subs {
		see(s.Start)
		for _, e := range s.Elems {
			see(e.To)
			if e.Curve {
				see(e.C1)
				see(e.C2)
			}
		}
	}
	return
}

// ---- Shapes ----

// builder writes one outline.
type builder struct{ s sub }

func start(p Pt) *builder             { return &builder{s: sub{Start: p}} }
func (b *builder) line(p Pt) *builder { b.s.Elems = append(b.s.Elems, elem{To: p}); return b }
func (b *builder) cubic(c1, c2, p Pt) *builder {
	b.s.Elems = append(b.s.Elems, elem{Curve: true, C1: c1, C2: c2, To: p})
	return b
}
func (b *builder) quad(c, p Pt) *builder {
	from := b.last()
	return b.cubic(lerpPt(from, c, 2.0/3), lerpPt(p, c, 2.0/3), p)
}
func (b *builder) last() Pt {
	if n := len(b.s.Elems); n > 0 {
		return b.s.Elems[n-1].To
	}
	return b.s.Start
}

// arc continues the outline round a circle from angle a0 to a1 (radians; y-down, so positive
// angles turn clockwise). The outline must already be at the arc's start.
func (b *builder) arc(c Pt, r, a0, a1 float64) *builder {
	n := int(math.Ceil(math.Abs(a1-a0) / (math.Pi / 2)))
	if n < 1 {
		n = 1
	}
	step := (a1 - a0) / float64(n)
	k := 4.0 / 3 * math.Tan(step/4)
	for i := 0; i < n; i++ {
		t0, t1 := a0+step*float64(i), a0+step*float64(i+1)
		p0 := Pt{c.X + r*math.Cos(t0), c.Y + r*math.Sin(t0)}
		p1 := Pt{c.X + r*math.Cos(t1), c.Y + r*math.Sin(t1)}
		c1 := Pt{p0.X - k*r*math.Sin(t0), p0.Y + k*r*math.Cos(t0)}
		c2 := Pt{p1.X + k*r*math.Sin(t1), p1.Y - k*r*math.Cos(t1)}
		b.cubic(c1, c2, p1)
	}
	return b
}
func (b *builder) path() Path { return Path{subs: []sub{b.s}} }

func polar(c Pt, r, a float64) Pt { return Pt{c.X + r*math.Cos(a), c.Y + r*math.Sin(a)} }

// Circle is a full circle.
func Circle(cx, cy, r float64) Path {
	c := Pt{cx, cy}
	return start(polar(c, r, 0)).arc(c, r, 0, 2*math.Pi).path()
}

// Ellipse is an ellipse with radii rx, ry.
func Ellipse(cx, cy, rx, ry float64) Path {
	return Circle(0, 0, 1).Map(func(q Pt) Pt { return Pt{cx + q.X*rx, cy + q.Y*ry} })
}

// Rect is a rectangle with rounded corners. The corners are a touch squarer than circles
// (Apple's "continuous" corners), so a large radius still reads as a rectangle.
func Rect(x, y, w, h, r float64) Path {
	r = math.Min(r, math.Min(w, h)/2)
	if r <= 0 {
		return start(Pt{x, y}).line(Pt{x + w, y}).line(Pt{x + w, y + h}).line(Pt{x, y + h}).path()
	}
	// The curve starts further along each side (1.28 r) and bends more gently: a cheap
	// stand-in for a superellipse that keeps every point on a cubic.
	e := math.Min(r*1.28, math.Min(w, h)/2)
	k := e - r*0.92
	b := start(Pt{x + e, y})
	b.line(Pt{x + w - e, y}).cubic(Pt{x + w - k, y}, Pt{x + w, y + k}, Pt{x + w, y + e})
	b.line(Pt{x + w, y + h - e}).cubic(Pt{x + w, y + h - k}, Pt{x + w - k, y + h}, Pt{x + w - e, y + h})
	b.line(Pt{x + e, y + h}).cubic(Pt{x + k, y + h}, Pt{x, y + h - k}, Pt{x, y + h - e})
	b.line(Pt{x, y + e}).cubic(Pt{x, y + k}, Pt{x + k, y}, Pt{x + e, y})
	return b.path()
}

// Poly is a polygon whose corners are rounded by r.
func Poly(r float64, pts ...Pt) Path {
	n := len(pts)
	if r <= 0 {
		b := start(pts[0])
		for _, p := range pts[1:] {
			b.line(p)
		}
		return b.path()
	}
	cut := func(i int) (in, out Pt) {
		p, prev, next := pts[i], pts[(i+n-1)%n], pts[(i+1)%n]
		dPrev, dNext := math.Hypot(prev.X-p.X, prev.Y-p.Y), math.Hypot(next.X-p.X, next.Y-p.Y)
		rr := math.Min(r, math.Min(dPrev, dNext)/2)
		return lerpPt(p, prev, rr/dPrev), lerpPt(p, next, rr/dNext)
	}
	in0, out0 := cut(0)
	b := start(out0)
	for i := 1; i < n; i++ {
		in, out := cut(i)
		b.line(in).quad(pts[i], out)
	}
	b.line(in0).quad(pts[0], out0)
	return b.path()
}

// StrokeArc is a thick arc of width w with round ends, from angle a0 to a1.
func StrokeArc(cx, cy, r, w, a0, a1 float64) Path {
	c := Pt{cx, cy}
	ro, ri, h := r+w/2, r-w/2, w/2
	b := start(polar(c, ro, a0)).arc(c, ro, a0, a1)
	b.arc(polar(c, r, a1), h, a1, a1+math.Pi)
	b.arc(c, ri, a1, a0)
	b.arc(polar(c, r, a0), h, a0+math.Pi, a0+2*math.Pi)
	return b.path()
}

// Capsule is a thick line of width w with round ends.
func Capsule(x1, y1, x2, y2, w float64) Path {
	a := math.Atan2(y2-y1, x2-x1)
	h := w / 2
	p1, p2 := Pt{x1, y1}, Pt{x2, y2}
	b := start(polar(p1, h, a-math.Pi/2)).line(polar(p2, h, a-math.Pi/2))
	b.arc(p2, h, a-math.Pi/2, a+math.Pi/2)
	b.line(polar(p1, h, a+math.Pi/2))
	b.arc(p1, h, a+math.Pi/2, a+3*math.Pi/2)
	return b.path()
}

// Polyline is a thick line through pts with round joins and ends: a capsule per piece and a
// disc at each joint, which fill as one shape under the non-zero rule.
func Polyline(w float64, pts ...Pt) Path {
	var out Path
	for i := 0; i+1 < len(pts); i++ {
		out = out.Add(Capsule(pts[i].X, pts[i].Y, pts[i+1].X, pts[i+1].Y, w).Solid())
	}
	return out
}

// Spark is the family's four-pointed sparkle: points at r, sides pinched toward the centre.
// pinch 0 is a diamond; 0.8 is a thin star.
func Spark(cx, cy, r, pinch float64) Path {
	c := Pt{cx, cy}
	tips := []Pt{{cx, cy - r}, {cx + r, cy}, {cx, cy + r}, {cx - r, cy}}
	b := start(tips[0])
	for i := 0; i < 4; i++ {
		a, z := tips[i], tips[(i+1)%4]
		mid := lerpPt(a, z, 0.5)
		ctrl := lerpPt(mid, c, pinch)
		b.quad(ctrl, z)
	}
	return b.path()
}

// Triangle is a triangle with rounded corners.
func Triangle(r float64, a, b, c Pt) Path { return Poly(r, a, b, c) }
