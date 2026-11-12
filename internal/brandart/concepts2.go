// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package brandart

import "math"

// The designs besides the monogram. The owner liked the W and the orbit with its dot; the five
// after them grow from those two: plain geometry, round ends, a dot in the app's colour, each
// readable at 16 pixels.

const (
	// OrbitW: the W inside the orbit, its moon in the gap.
	OrbitW Concept = "orbitw"
	// Nodes: the W as five dots joined by short links, the last dot lit: your devices, linked.
	Nodes Concept = "nodes"
	// Wave: the W as one smooth wave, like a sound or a signal, standing on the rack.
	Wave Concept = "wave"
	// Dial: a knob with a gauge around it and a lit dot, the way you turn a volume up.
	Dial Concept = "dial"
	// Pills: five rounded bars whose tops trace a W, like a sound meter, a dot over the middle.
	Pills Concept = "pills"
)

// screen is the client's screen on a stand, which every concept's client shares.
func screen() Path {
	return Rect(222, 280, 580, 420, 92).Add(Rect(477, 680, 70, 100, 8), Capsule(412, 790, 612, 790, 46))
}

// discAndBook are the player and reader where a concept has no W of its own for them: a disc
// with a play cut, a book with lines cut, each with its dot.
func discAndBook(prod Product) (body, ink, spark Path) {
	if prod == Player {
		return Circle(512, 540, 300), Triangle(18, Pt{440, 420}, Pt{630, 540}, Pt{440, 660}), Circle(772, 270, 52)
	}
	body = Poly(46, Pt{224, 300}, Pt{512, 360}, Pt{800, 300}, Pt{800, 720}, Pt{512, 790}, Pt{224, 720})
	ink = Capsule(300, 450, 450, 470, 30).Add(Capsule(300, 540, 450, 560, 30), Capsule(574, 470, 724, 450, 30), Capsule(574, 560, 724, 540, 30))
	return body, ink, Circle(790, 220, 52)
}

// wPoints are the W's five corners in a box.
func wPoints(x, y, w, h float64) []Pt {
	return []Pt{{x, y}, {x + w*0.25, y + h}, {x + w*0.5, y + h*0.36}, {x + w*0.75, y + h}, {x + w, y}}
}

// gapped is the thick line a→b, shortened by g at each end (so neighbours don't touch).
func gapped(a, b Pt, g, w float64) Path {
	d := b.sub(a)
	l := math.Hypot(d.X, d.Y)
	u := d.mul(1 / l)
	p, q := a.add(u.mul(g)), b.sub(u.mul(g))
	return Capsule(p.X, p.Y, q.X, q.Y, w)
}

// orbitRing is a thick ring with a gap at the top right, and the moon that sits in the gap.
func orbitRing(c Pt, r, w float64) (ring, moon Path) {
	ring = StrokeArc(c.X, c.Y, r, w, -math.Pi/4+0.42, 2*math.Pi-math.Pi/4-0.42)
	m := polar(c, r, -math.Pi/4)
	// As big as the ring is thick, but always clear of the ring's round ends.
	dot := math.Min(w*0.86, r*0.42-w/2-w*0.15)
	return ring, Circle(m.X, m.Y, dot)
}

// wave is the W as a smooth curve of half-width h through a box.
func wave(x, y, w, ht, h float64, caps bool) Path {
	var pts []Pt
	const n = 72
	for i := 0; i <= n; i++ {
		t := float64(i) / n
		dip := (1 - math.Cos(4*math.Pi*t)) / 2 // 0 at the tops, 1 at the bottoms
		mid := math.Exp(-math.Pow((t-0.5)/0.13, 2)) * 0.36
		pts = append(pts, Pt{x + w*t, y + ht*math.Max(dip, mid)})
	}
	out := StrokeOutline(h, pts...)
	if !caps {
		// Cut out of something, the round ends would overlap the line and fill back in.
		return out
	}
	return out.Add(Circle(pts[0].X, pts[0].Y, h).Solid(), Circle(pts[n].X, pts[n].Y, h).Solid())
}

func moreConcepts(c Concept, prod Product, small bool) (body, ink, spark Path) {
	if c == OrbitW && (prod == Player || prod == Reader) {
		k := 1.0
		if small {
			k = 1.25
		}
		ring, moon := orbitRing(Pt{512, 512}, 300, 66*k)
		if prod == Player {
			return ring.Add(Triangle(22, Pt{446, 404}, Pt{626, 512}, Pt{446, 620})), Path{}, moon
		}
		// An open book: two pages meeting at the spine.
		book := Poly(18, Pt{352, 430}, Pt{500, 460}, Pt{500, 640}, Pt{352, 610}).Solid().Add(Poly(18, Pt{524, 460}, Pt{672, 430}, Pt{672, 610}, Pt{524, 640}).Solid())
		return ring.Add(book), Path{}, moon
	}
	if prod == Player || prod == Reader {
		return discAndBook(prod)
	}
	server := prod == Server
	k := 1.0
	if small {
		k = 1.25
	}
	switch c {
	case Orbit:
		if server {
			ring, moon := orbitRing(Pt{512, 530}, 280, 74*k)
			return Circle(512, 530, 140).Add(ring), Path{}, moon
		}
		ring, moon := orbitRing(Pt{512, 490}, 140, 46*k)
		return screen(), Circle(512, 490, 62).Add(ring), moon
	case OrbitW:
		if server {
			ring, moon := orbitRing(Pt{512, 512}, 300, 66*k)
			return ring.Add(wPath(352, 410, 320, 220, 80*k)), Path{}, moon
		}
		ring, moon := orbitRing(Pt{512, 490}, 150, 40*k)
		if small {
			// Too few pixels for the W inside: the ring and its moon carry it.
			return screen(), ring, moon
		}
		return screen(), ring.Add(wPath(450, 452, 124, 86, 27*k)), moon
	case Nodes:
		r, w := 58.0*k, 44.0*k
		if server {
			pts := wPoints(260, 300, 504, 400)
			for i, p := range pts[:4] {
				body = body.Add(Circle(p.X, p.Y, r).Solid(), gapped(p, pts[i+1], r+22, w).Solid())
			}
			return body, Path{}, Circle(pts[4].X, pts[4].Y, r)
		}
		a, b, c := Pt{360, 560}, Pt{512, 420}, Pt{664, 560}
		ink = Circle(a.X, a.Y, 44*k).Add(Circle(b.X, b.Y, 44*k), gapped(a, b, 44*k+18, 30*k), gapped(b, c, 44*k+18, 30*k))
		return screen(), ink, Circle(c.X, c.Y, 44*k)
	case Wave:
		if server {
			body = wave(262, 250, 500, 360, 50*k, true).Add(Rect(250, 700, 524, 124, 50).Solid())
			return body, Capsule(320, 762, 560, 762, 34*k), Circle(696, 762, 30*k)
		}
		return screen(), wave(350, 400, 324, 190, 30*k, false), Circle(232, 250, 46)
	case Dial:
		if server {
			c := Pt{512, 512}
			body = Circle(c.X, c.Y, 300)
			ink = StrokeArc(c.X, c.Y, 196, 44*k, math.Pi*0.75, math.Pi*2.25).Add(Capsule(512, 512, 600, 424, 44*k))
			return body, ink, Circle(c.X+250, c.Y-250, 60)
		}
		c := Pt{512, 490}
		ink = StrokeArc(c.X, c.Y, 120, 34*k, math.Pi*0.75, math.Pi*2.25).Add(Capsule(512, 490, 566, 436, 34*k))
		return screen(), ink, Circle(232, 250, 46)
	case Pills:
		// Bar tops: high, low, middle, low, high — a W's outline.
		tops := []float64{0, 1, 0.42, 1, 0}
		if server {
			for i, t := range tops {
				x := 290 + float64(i)*111
				body = body.Add(Capsule(x, 330+t*230, x, 780, 76*k).Solid())
			}
			return body, Path{}, Circle(512, 250, 50)
		}
		for i, t := range tops {
			x := 392 + float64(i)*60
			ink = ink.Add(Capsule(x, 390+t*110, x, 610, 36*k))
		}
		return screen(), ink, Circle(232, 250, 46)
	}
	return
}
