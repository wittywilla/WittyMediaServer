// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package brandart

import (
	"encoding/binary"
	"fmt"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// Words in pictures (the wordmark on banners, Top Shelf and lockups, the line on the social
// picture) are drawn as outlines from the website's own font, Inter, at the website's weights
// and spacing: the brand name bold (700) with -0.02em tracking, as `.brand` in its CSS, and
// other lines regular (400). Kerning comes from the font's GPOS table, as a browser applies it.
// The font files live in internal/brandart/fonts; whoever draws words calls UseFonts first
// (scripts/genicon does), so the server and tray never carry them.

// WordmarkTracking is the website's letter-spacing for the brand name, in ems.
const WordmarkTracking = -0.02

type face struct {
	f    *sfnt.Font
	upem float64
	gpos *gposKern
}

var (
	fontMu          sync.Mutex
	boldF, regularF *face
)

// UseFonts loads the bold and regular fonts the words are drawn in.
func UseFonts(bold, regular []byte) error {
	fontMu.Lock()
	defer fontMu.Unlock()
	b, err := loadFace(bold)
	if err != nil {
		return fmt.Errorf("bold font: %w", err)
	}
	r, err := loadFace(regular)
	if err != nil {
		return fmt.Errorf("regular font: %w", err)
	}
	boldF, regularF = b, r
	return nil
}

func loadFace(data []byte) (*face, error) {
	f, err := sfnt.Parse(data)
	if err != nil {
		return nil, err
	}
	return &face{f: f, upem: float64(f.UnitsPerEm()), gpos: readGPOSKern(data)}, nil
}

// Run is a piece of text in one colour.
type Run struct {
	Text string
	C    Colour
}

// TextArt lays runs on one line, the baseline at y and starting at x, size px to the em, and
// returns them as layers and the line's width. bold picks the brand weight and its tracking.
func TextArt(runs []Run, x, y, size float64, bold bool) ([]Layer, float64) {
	fontMu.Lock()
	fc, track := regularF, 0.0
	if bold {
		fc, track = boldF, WordmarkTracking
	}
	fontMu.Unlock()
	if fc == nil {
		panic("brandart: no fonts loaded: call UseFonts (with internal/brandart/fonts) before drawing words")
	}
	var buf sfnt.Buffer
	ppem := fixed.I(int(fc.upem))
	k := size / fc.upem // font units to pixels
	pen := 0.0          // in font units
	var prev sfnt.GlyphIndex
	first := true
	var layers []Layer
	for _, r := range runs {
		var p Path
		for _, ch := range r.Text {
			gi, err := fc.f.GlyphIndex(&buf, ch)
			if err != nil {
				continue
			}
			if !first {
				pen += float64(fc.gpos.pair(uint16(prev), uint16(gi)))
				pen += track * fc.upem
			}
			segs, err := fc.f.LoadGlyph(&buf, gi, ppem, nil)
			if err == nil {
				p = p.Add(glyphPath(segs, x+pen*k, y, k))
			}
			adv, _ := fc.f.GlyphAdvance(&buf, gi, ppem, font.HintingNone)
			pen += float64(adv) / 64
			prev, first = gi, false
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

// ---- GPOS pair kerning ----

// gposKern is the pair-adjustment subtables (GPOS lookup type 2, also inside type 9
// extensions) that the font's "kern" feature uses: enough to space a line as a browser does.
type gposKern struct {
	d    []byte
	subs []int // offsets of PairPos subtables in d, per lookup in order
	ends []int // where each lookup's subtables end in subs
}

func u16(d []byte, at int) int {
	if at < 0 || at+2 > len(d) {
		return 0
	}
	return int(binary.BigEndian.Uint16(d[at:]))
}

func u32(d []byte, at int) int {
	if at < 0 || at+4 > len(d) {
		return 0
	}
	return int(binary.BigEndian.Uint32(d[at:]))
}

func readGPOSKern(font []byte) *gposKern {
	n := u16(font, 4)
	var g []byte
	for i := 0; i < n; i++ {
		rec := 12 + 16*i
		if string(font[rec:rec+4]) == "GPOS" {
			off, l := u32(font, rec+8), u32(font, rec+12)
			if off+l <= len(font) {
				g = font[off : off+l]
			}
		}
	}
	if g == nil {
		return nil
	}
	featureList, lookupList := u16(g, 6), u16(g, 8)
	want := map[int]bool{}
	for i := 0; i < u16(g, featureList); i++ {
		rec := featureList + 2 + 6*i
		if string(g[rec:rec+4]) != "kern" {
			continue
		}
		feat := featureList + u16(g, rec+4)
		for j := 0; j < u16(g, feat+2); j++ {
			want[u16(g, feat+4+2*j)] = true
		}
	}
	k := &gposKern{d: g}
	for li := 0; li < u16(g, lookupList); li++ {
		if !want[li] {
			continue
		}
		lookup := lookupList + u16(g, lookupList+2+2*li)
		kind := u16(g, lookup)
		for s := 0; s < u16(g, lookup+4); s++ {
			sub := lookup + u16(g, lookup+6+2*s)
			t := kind
			if kind == 9 {
				t = u16(g, sub+2)
				sub += u32(g, sub+4)
			}
			if t == 2 {
				k.subs = append(k.subs, sub)
			}
		}
		k.ends = append(k.ends, len(k.subs))
	}
	return k
}

func coverageIndex(d []byte, at, glyph int) int {
	switch u16(d, at) {
	case 1:
		lo, hi := 0, u16(d, at+2)-1
		for lo <= hi {
			m := (lo + hi) / 2
			g := u16(d, at+4+2*m)
			if g == glyph {
				return m
			} else if g < glyph {
				lo = m + 1
			} else {
				hi = m - 1
			}
		}
	case 2:
		for i := 0; i < u16(d, at+2); i++ {
			r := at + 4 + 6*i
			if s, e := u16(d, r), u16(d, r+2); glyph >= s && glyph <= e {
				return u16(d, r+4) + glyph - s
			}
		}
	}
	return -1
}

func classOf(d []byte, at, glyph int) int {
	switch u16(d, at) {
	case 1:
		start := u16(d, at+2)
		if i := glyph - start; i >= 0 && i < u16(d, at+4) {
			return u16(d, at+6+2*i)
		}
	case 2:
		for i := 0; i < u16(d, at+2); i++ {
			r := at + 4 + 6*i
			if glyph >= u16(d, r) && glyph <= u16(d, r+2) {
				return u16(d, r+4)
			}
		}
	}
	return 0
}

func popcount(v int) int {
	n := 0
	for ; v != 0; v &= v - 1 {
		n++
	}
	return n
}

// pair is the advance adjustment between glyphs a and b, in font units.
func (k *gposKern) pair(a, b uint16) int {
	if k == nil {
		return 0
	}
	d, total, from := k.d, 0, 0
	for _, end := range k.ends {
		for _, sub := range k.subs[from:end] {
			cov := coverageIndex(d, sub+u16(d, sub+2), int(a))
			if cov < 0 {
				continue
			}
			vf1, vf2 := u16(d, sub+4), u16(d, sub+6)
			size1, size2 := 2*popcount(vf1), 2*popcount(vf2)
			xadv := func(rec int) int {
				if vf1&4 == 0 {
					return 0
				}
				return int(int16(u16(d, rec+2*popcount(vf1&3))))
			}
			if u16(d, sub) == 1 {
				set := sub + u16(d, sub+10+2*cov)
				found := false
				for i := 0; i < u16(d, set); i++ {
					rec := set + 2 + i*(2+size1+size2)
					if u16(d, rec) == int(b) {
						total += xadv(rec + 2)
						found = true
						break
					}
				}
				if found {
					break
				}
				continue
			}
			c1 := classOf(d, sub+u16(d, sub+8), int(a))
			c2 := classOf(d, sub+u16(d, sub+10), int(b))
			n2 := u16(d, sub+14)
			rec := sub + 16 + (c1*n2+c2)*(size1+size2)
			total += xadv(rec)
			break
		}
		from = end
	}
	return total
}
