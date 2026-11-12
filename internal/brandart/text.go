// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package brandart

import (
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// The wordmark and the few words on banners are drawn as outlines from the Go fonts (BSD
// licence, credited in NOTICE), so no picture depends on what fonts a machine has.
var (
	fontOnce          sync.Once
	boldFace, regFace *sfnt.Font
)

func fonts() {
	fontOnce.Do(func() {
		boldFace, _ = sfnt.Parse(gobold.TTF)
		regFace, _ = sfnt.Parse(goregular.TTF)
	})
}

// Run is a piece of text in one colour.
type Run struct {
	Text string
	C    Colour
}

// TextArt lays runs on one line, the baseline at y and starting at x, size px high (the font's
// em), and returns them as layers and the line's width. bold picks the face.
func TextArt(runs []Run, x, y, size float64, bold bool) ([]Layer, float64) {
	fonts()
	f := regFace
	if bold {
		f = boldFace
	}
	var buf sfnt.Buffer
	const ppem = 1000
	k := size / ppem
	pen := 0.0
	var prev sfnt.GlyphIndex
	var layers []Layer
	for _, r := range runs {
		var p Path
		for _, ch := range r.Text {
			gi, err := f.GlyphIndex(&buf, ch)
			if err != nil {
				continue
			}
			if prev != 0 {
				if kern, err := f.Kern(&buf, prev, gi, fixed.I(ppem), font.HintingNone); err == nil {
					pen += float64(kern) / 64
				}
			}
			segs, err := f.LoadGlyph(&buf, gi, fixed.I(ppem), nil)
			if err == nil {
				p = p.Add(glyphPath(segs, x+pen*k, y, k))
			}
			adv, _ := f.GlyphAdvance(&buf, gi, fixed.I(ppem), font.HintingNone)
			pen += float64(adv) / 64
			prev = gi
		}
		layers = append(layers, Layer{p, Fill(r.C)})
	}
	return layers, pen * k
}

// TextWidth is how wide TextArt would draw the runs.
func TextWidth(runs []Run, size float64, bold bool) float64 {
	_, w := TextArt(runs, 0, 0, size, bold)
	return w
}

func glyphPath(segs sfnt.Segments, ox, oy, k float64) Path {
	pt := func(a fixed.Point26_6) Pt { return Pt{ox + float64(a.X)/64*k, oy + float64(a.Y)/64*k} }
	var out Path
	var b *builder
	for _, s := range segs {
		switch s.Op {
		case sfnt.SegmentOpMoveTo:
			if b != nil {
				out.subs = append(out.subs, b.s)
			}
			b = start(pt(s.Args[0]))
		case sfnt.SegmentOpLineTo:
			b.line(pt(s.Args[0]))
		case sfnt.SegmentOpQuadTo:
			b.quad(pt(s.Args[0]), pt(s.Args[1]))
		case sfnt.SegmentOpCubeTo:
			b.cubic(pt(s.Args[0]), pt(s.Args[1]), pt(s.Args[2]))
		}
	}
	if b != nil {
		out.subs = append(out.subs, b.s)
	}
	return out
}
