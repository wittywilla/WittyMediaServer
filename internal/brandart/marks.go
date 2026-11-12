// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package brandart

import (
	"fmt"
	"math"
	"strings"
)

// Product is one app of the family.
type Product string

const (
	Server Product = "server" // WittyMediaServer
	Client Product = "client" // WittyMediaClient
	Player Product = "player" // WittyMediaPlayer
	Reader Product = "reader" // WittyReader
)

// Products is every app with a mark.
var Products = []Product{Server, Client, Player, Reader}

// Name is the product's name as people see it.
func (p Product) Name() string {
	return map[Product]string{Server: "WittyMediaServer", Client: "WittyMediaClient", Player: "WittyMediaPlayer", Reader: "WittyReader"}[p]
}

// Concept is one of the family's designs. OrbitW is the one in use; the others are kept,
// complete, so the owner can switch with one flag (genicon -concept, or WITTY_BRAND_CONCEPT for
// scripts/sync-brand.sh).
type Concept string

const (
	// Monogram: a heavy rounded W, set on a server rack, inside a screen, on a disc, on a book.
	Monogram Concept = "monogram"
	// Orbit: a core with a thick orbit and its moon. The client is a screen with the same cut in.
	Orbit Concept = "orbit"
)

// Concepts is every design, the one in use first.
var Concepts = []Concept{OrbitW, Monogram, Orbit, Nodes, Wave, Dial, Pills}

// DefaultConcept is the family's mark in use: the owner picked the W in its orbit.
const DefaultConcept = OrbitW

// ParseConcept checks a name.
func ParseConcept(s string) (Concept, error) {
	for _, c := range Concepts {
		if string(c) == s {
			return c, nil
		}
	}
	var names []string
	for _, c := range Concepts {
		names = append(names, string(c))
	}
	return "", fmt.Errorf("no concept %q (have %s)", s, strings.Join(names, ", "))
}

// Page is the size every mark is designed on.
const Page = 1024.0

// Mark is one app's mark, in parts, so each platform can stack them its own way (tvOS parallax
// layers, Android's adaptive foreground and background, a flat tile, a one-colour tray icon).
type Mark struct {
	Product Product
	Concept Concept
	Back    []Paint // painted in order over whatever outline the platform gives the icon
	Body    Layer   // the character's body: the middle depth
	Ink     Layer   // its features, painted on the body: holes in the one-colour version
	Accents []Layer // small touches in the app's colour on the body (front depth)
	Spark   Layer   // the family's spark, in the app's colour (front depth)
}

// Glyph is the mark without its background.
func (m Mark) Glyph() []Layer {
	return append(append([]Layer{m.Body, m.Ink}, m.Accents...), m.Spark)
}

// Front is the nearest depth: features, accents and spark.
func (m Mark) Front() []Layer { return append(append([]Layer{m.Ink}, m.Accents...), m.Spark) }

// MonoPath is the mark in one colour: the body with its features cut out, and the spark.
func (m Mark) MonoPath() Path {
	return Cut(m.Body.Path, m.Ink.Path).Add(m.Spark.Path.Solid())
}

// productLook is what every concept shares for one app: the mark's gradient and its dot. Each
// app's dot is its own colour (server amber, client mint, player sky, reader coral), so the
// four tell apart at a glance even where their shapes are small.
// Every tile is the website's: the house theme's dark slate with a soft glow of the app's
// colour, and the mark itself in that colour, as the site's own icon is.
type productLook struct {
	from, to func(p Palette) Colour
	spark    func(p Palette) Colour
}

var looks = map[Product]productLook{
	// Violet, the house colour, for the server; an amber dot like a status light.
	Server: {func(p Palette) Colour { return Mix(p.AccentLite, Hex("#ffffff"), 0.12) }, func(p Palette) Colour { return p.Accent }, func(p Palette) Colour { return p.Amber }},
	// The client: violet into the Midnight theme's sky, a mint dot.
	Client: {func(p Palette) Colour { return p.AccentLite }, func(p Palette) Colour { return p.Sky }, func(p Palette) Colour { return p.Mint }},
	// The player: coral into violet, a sky-blue dot.
	Player: {func(p Palette) Colour { return p.Coral }, func(p Palette) Colour { return p.Accent }, func(p Palette) Colour { return p.Sky }},
	// The reader: mint into sky, a coral dot.
	Reader: {func(p Palette) Colour { return p.Mint }, func(p Palette) Colour { return p.Sky }, func(p Palette) Colour { return p.Coral }},
}

// TileColour is the dark of every tile, and of the cuts in the marks.
func (p Palette) TileColour() Colour { return Mix(p.Night, p.Slate, 0.45) }

// Design is one app's mark in one concept, fitted inside the circle every launcher mask shows.
// small thickens the features for 16–32 pixel icons.
func Design(c Concept, prod Product, pal Palette, small bool) Mark {
	look := looks[prod]
	tile := pal.TileColour()
	from, to := look.from(pal), look.to(pal)
	m := Mark{Product: prod, Concept: c, Back: []Paint{
		Linear(Pt{0, 0}, Pt{0, Page}, Mix(tile, pal.Raised, 0.5), Mix(tile, pal.Night, 0.5)),
		Glow(Page/2, Page*0.45, Page*0.6, to.WithAlpha(0.22)),
	}}
	sparkC := look.spark(pal)
	var body, ink, spark Path
	var accents []Layer
	switch c {
	case Monogram:
		body, ink, spark, accents = monogram(prod, pal, sparkC, small)
	default:
		body, ink, spark = moreConcepts(c, prod, small)
	}
	lo, hi := body.Bounds()
	m.Body = Layer{body.Solid(), Linear(lo, hi, from, to)}
	m.Ink = Layer{ink, Fill(tile)}
	m.Accents = accents
	m.Spark = Layer{spark.Solid(), Fill(sparkC)}
	return m.fitted(small)
}

// fitted scales and centres the glyph. Full size, its farthest point sits 0.325 of the page
// from the centre: inside the 72-of-108 circle every Android launcher mask shows, and clear of
// iOS's and tvOS's corners. Small (16–32 pixels, no mask) its box fills 0.80 of the page.
func (m Mark) fitted(small bool) Mark {
	all := m.Body.Path.Add(m.Spark.Path)
	lo, hi := all.Bounds()
	c := Pt{(lo.X + hi.X) / 2, (lo.Y + hi.Y) / 2}
	var k float64
	if small {
		k = Page * 0.80 / math.Max(hi.X-lo.X, hi.Y-lo.Y)
	} else {
		far := 0.0
		for _, s := range all.subs {
			for _, q := range s.flatten(12) {
				far = math.Max(far, math.Hypot(q.X-c.X, q.Y-c.Y))
			}
		}
		k = Page * 0.325 / far
	}
	f := func(q Pt) Pt { return Pt{Page/2 + (q.X-c.X)*k, Page/2 + (q.Y-c.Y)*k} }
	fix := func(l Layer) Layer {
		p := l.Paint
		p.From, p.To = f(p.From), f(p.To)
		return Layer{l.Path.Map(f), p}
	}
	m.Body, m.Ink, m.Spark = fix(m.Body), fix(m.Ink), fix(m.Spark)
	for i := range m.Accents {
		m.Accents[i] = fix(m.Accents[i])
	}
	return m
}

// ---- Monogram ----

// wPath is the family's W: a thick zigzag of width t through five points, with flat-cut ends
// and bevelled bottoms, as one outline.
func wPath(x, y, w, h, t float64) Path {
	pts := []Pt{{x, y}, {x + w*0.25, y + h}, {x + w*0.5, y + h*0.36}, {x + w*0.75, y + h}, {x + w, y}}
	return StrokeOutline(t/2, pts...)
}

func monogram(prod Product, pal Palette, sparkC Colour, small bool) (body, ink, spark Path, accents []Layer) {
	t := 92.0
	if small {
		t = 120
	}
	switch prod {
	case Server:
		// The W on a rack unit with one slot and a light.
		body = wPath(250, 250, 524, 400, t).Add(Rect(250, 700, 524, 130, 44))
		ink = Capsule(320, 765, 520, 765, 34)
		accents = []Layer{{Circle(690, 765, 24), Fill(sparkC)}}
		spark = Spark(800, 228, 88, 0.6)
	case Client:
		// The W on a screen with a stand.
		body = Rect(200, 260, 624, 470, 96).Add(Rect(477, 700, 70, 80, 8), Capsule(412, 790, 612, 790, 46))
		ink = wPath(300, 360, 424, 280, math.Min(t*0.8, 84))
		spark = Spark(222, 222, 88, 0.6)
	case Player:
		body = Circle(512, 540, 300)
		ink = wPath(352, 430, 320, 230, t*0.75)
		spark = Spark(780, 262, 88, 0.6)
	case Reader:
		body = Poly(46, Pt{224, 300}, Pt{512, 360}, Pt{800, 300}, Pt{800, 720}, Pt{512, 790}, Pt{224, 720})
		ink = wPath(332, 420, 360, 270, t*0.8)
		spark = Spark(790, 212, 88, 0.6)
	}
	return
}

// StrokeOutline is the outline of a thick line of half-width h through pts: square ends, mitred
// joins, and joins sharper than about 50° bevelled flat so the points don't spike.
func StrokeOutline(h float64, pts ...Pt) Path {
	n := len(pts)
	normal := func(a, b Pt) Pt {
		d := b.sub(a)
		l := math.Hypot(d.X, d.Y)
		return Pt{-d.Y / l, d.X / l}
	}
	side := func(sign float64) []Pt {
		var out []Pt
		out = append(out, pts[0].add(normal(pts[0], pts[1]).mul(sign*h)))
		for i := 1; i < n-1; i++ {
			n1, n2 := normal(pts[i-1], pts[i]).mul(sign), normal(pts[i], pts[i+1]).mul(sign)
			// The two offset lines, a1→pts[i] direction and pts[i]→ direction, meet at the mitre.
			a1 := pts[i-1].add(n1.mul(h))
			d1 := pts[i].sub(pts[i-1])
			a2 := pts[i].add(n2.mul(h))
			d2 := pts[i+1].sub(pts[i])
			cross := d1.X*d2.Y - d1.Y*d2.X
			if math.Abs(cross) < 1e-9 {
				out = append(out, a2)
				continue
			}
			t := ((a2.X-a1.X)*d2.Y - (a2.Y-a1.Y)*d2.X) / cross
			m := a1.add(d1.mul(t))
			if math.Hypot(m.X-pts[i].X, m.Y-pts[i].Y) > 1.6*h {
				out = append(out, pts[i].add(n1.mul(h)), a2)
			} else {
				out = append(out, m)
			}
		}
		out = append(out, pts[n-1].add(normal(pts[n-2], pts[n-1]).mul(sign*h)))
		return out
	}
	left, right := side(1), side(-1)
	all := left
	for i := len(right) - 1; i >= 0; i-- {
		all = append(all, right[i])
	}
	return Poly(h*0.35, all...).Solid()
}
