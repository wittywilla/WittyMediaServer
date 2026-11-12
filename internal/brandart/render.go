// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package brandart

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strconv"
	"strings"

	"golang.org/x/image/vector"
)

// Colour is a straight (not premultiplied) sRGB colour.
type Colour struct{ R, G, B, A uint8 }

// Hex reads #rrggbb or #rrggbbaa.
func Hex(s string) Colour {
	s = strings.TrimPrefix(s, "#")
	v, _ := strconv.ParseUint(s, 16, 32)
	if len(s) == 6 {
		return Colour{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}
	}
	return Colour{uint8(v >> 24), uint8(v >> 16), uint8(v >> 8), uint8(v)}
}

func (c Colour) hex() string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

// android is #AARRGGBB.
func (c Colour) android() string { return fmt.Sprintf("#%02X%02X%02X%02X", c.A, c.R, c.G, c.B) }

// WithAlpha is c at opacity a (0–1).
func (c Colour) WithAlpha(a float64) Colour {
	c.A = uint8(math.Round(float64(c.A) * a))
	return c
}

// Mix is a + (b-a)·t.
func Mix(a, b Colour, t float64) Colour {
	f := func(x, y uint8) uint8 { return uint8(math.Round(float64(x) + (float64(y)-float64(x))*t)) }
	return Colour{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), f(a.A, b.A)}
}

// Stop is one colour of a gradient, at offset 0–1.
type Stop struct {
	At float64
	C  Colour
}

// Paint fills a shape: one colour, or a gradient (linear from From to To, or radial around
// From reaching To's distance).
type Paint struct {
	Solid  Colour
	Stops  []Stop
	Radial bool
	From   Pt
	To     Pt
}

// Fill is a one-colour paint.
func Fill(c Colour) Paint { return Paint{Solid: c} }

// Linear is a two-colour gradient along from→to.
func Linear(from, to Pt, a, b Colour) Paint {
	return Paint{Stops: []Stop{{0, a}, {1, b}}, From: from, To: to}
}

// Glow is a radial gradient from c at the centre to transparent at radius r.
func Glow(cx, cy, r float64, c Colour) Paint {
	return Paint{Radial: true, From: Pt{cx, cy}, To: Pt{cx + r, cy}, Stops: []Stop{{0, c}, {1, c.WithAlpha(0)}}}
}

func (p Paint) at(q Pt) Colour {
	if len(p.Stops) == 0 {
		return p.Solid
	}
	var t float64
	if p.Radial {
		t = math.Hypot(q.X-p.From.X, q.Y-p.From.Y) / math.Hypot(p.To.X-p.From.X, p.To.Y-p.From.Y)
	} else {
		d := p.To.sub(p.From)
		t = ((q.X-p.From.X)*d.X + (q.Y-p.From.Y)*d.Y) / (d.X*d.X + d.Y*d.Y)
	}
	t = math.Max(0, math.Min(1, t))
	s := p.Stops
	if t <= s[0].At {
		return s[0].C
	}
	for i := 1; i < len(s); i++ {
		if t <= s[i].At {
			return Mix(s[i-1].C, s[i].C, (t-s[i-1].At)/(s[i].At-s[i-1].At))
		}
	}
	return s[len(s)-1].C
}

// Layer is one filled shape of a drawing.
type Layer struct {
	Path  Path
	Paint Paint
}

// Art is a drawing: layers painted in order on a W×H page.
type Art struct {
	W, H   float64
	Layers []Layer
}

// With adds layers on top.
func (a Art) With(layers ...Layer) Art {
	a.Layers = append(append([]Layer(nil), a.Layers...), layers...)
	return a
}

// Over is a drawn on top of b (same page size).
func (a Art) Over(b Art) Art { return b.With(a.Layers...) }

// Map moves every layer's shape and paint through f.
func (a Art) Map(f func(Pt) Pt) Art {
	out := Art{W: a.W, H: a.H}
	for _, l := range a.Layers {
		p := l.Paint
		p.From, p.To = f(p.From), f(p.To)
		out.Layers = append(out.Layers, Layer{l.Path.Map(f), p})
	}
	return out
}

// Placed is a scaled by k about its centre and moved so its centre is at c, on a W×H page.
func (a Art) Placed(w, h float64, c Pt, k float64) Art {
	src := Pt{a.W / 2, a.H / 2}
	out := a.Map(func(q Pt) Pt { return c.add(q.sub(src).mul(k)) })
	out.W, out.H = w, h
	return out
}

// Recoloured paints every layer with one colour: the monochrome versions.
func (a Art) Recoloured(c Colour) Art {
	out := Art{W: a.W, H: a.H}
	for _, l := range a.Layers {
		out.Layers = append(out.Layers, Layer{l.Path, Fill(c)})
	}
	return out
}

// ---- Raster ----

// Image rasterises the drawing to w×h pixels.
func (a Art) Image(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	sx, sy := float64(w)/a.W, float64(h)/a.H
	for _, l := range a.Layers {
		z := vector.NewRasterizer(w, h)
		z.DrawOp = draw.Over
		for _, s := range l.Path.subs {
			z.MoveTo(float32(s.Start.X*sx), float32(s.Start.Y*sy))
			for _, e := range s.Elems {
				if e.Curve {
					z.CubeTo(float32(e.C1.X*sx), float32(e.C1.Y*sy), float32(e.C2.X*sx), float32(e.C2.Y*sy), float32(e.To.X*sx), float32(e.To.Y*sy))
				} else {
					z.LineTo(float32(e.To.X*sx), float32(e.To.Y*sy))
				}
			}
			z.ClosePath()
		}
		if len(l.Paint.Stops) == 0 {
			c := l.Paint.Solid
			z.Draw(img, img.Bounds(), image.NewUniform(color.NRGBA{c.R, c.G, c.B, c.A}), image.Point{})
			continue
		}
		// A gradient: the shape's coverage first, then the colour worked out only where it
		// covers, blended straight into the pixels (much faster than an image.Image source).
		mask := image.NewAlpha(img.Bounds())
		z.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				cov := mask.Pix[y*mask.Stride+x]
				if cov == 0 {
					continue
				}
				c := l.Paint.at(Pt{(float64(x) + 0.5) / sx, (float64(y) + 0.5) / sy})
				a := uint32(c.A) * uint32(cov) / 255
				if a == 0 {
					continue
				}
				i := y*img.Stride + x*4
				d := img.Pix[i : i+4 : i+4]
				inv := 255 - a
				d[0] = uint8((uint32(c.R)*a + uint32(d[0])*inv + 127) / 255)
				d[1] = uint8((uint32(c.G)*a + uint32(d[1])*inv + 127) / 255)
				d[2] = uint8((uint32(c.B)*a + uint32(d[2])*inv + 127) / 255)
				d[3] = uint8((255*a + uint32(d[3])*inv + 127) / 255)
			}
		}
	}
	return img
}

// PNG encodes the drawing at w×h pixels.
func (a Art) PNG(w, h int) []byte { return encodePNG(a.Image(w, h)) }

func encodePNG(img image.Image) []byte {
	var b bytes.Buffer
	e := png.Encoder{CompressionLevel: png.BestCompression}
	_ = e.Encode(&b, img)
	return b.Bytes()
}

// Opaque is img over a solid colour: for formats and stores that refuse transparency (the App
// Store's icon, BMPs).
func Opaque(img *image.RGBA, under Colour) *image.RGBA {
	out := image.NewRGBA(img.Bounds())
	draw.Draw(out, out.Bounds(), image.NewUniform(color.NRGBA{under.R, under.G, under.B, 255}), image.Point{}, draw.Src)
	draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Over)
	return out
}

// ---- Vector ----

func num(v float64) string {
	s := strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
	if s == "-0" {
		return "0"
	}
	return s
}

func (p Path) d() string {
	var b strings.Builder
	for _, s := range p.subs {
		fmt.Fprintf(&b, "M%s %s", num(s.Start.X), num(s.Start.Y))
		for _, e := range s.Elems {
			if e.Curve {
				fmt.Fprintf(&b, "C%s %s %s %s %s %s", num(e.C1.X), num(e.C1.Y), num(e.C2.X), num(e.C2.Y), num(e.To.X), num(e.To.Y))
			} else {
				fmt.Fprintf(&b, "L%s %s", num(e.To.X), num(e.To.Y))
			}
		}
		b.WriteString("Z")
	}
	return b.String()
}

// SVG writes the drawing as a standalone SVG file.
func (a Art) SVG() []byte {
	var defs, body strings.Builder
	for i, l := range a.Layers {
		fill := l.Paint.Solid.hex()
		op := float64(l.Paint.Solid.A) / 255
		if len(l.Paint.Stops) > 0 {
			id := fmt.Sprintf("g%d", i)
			fill = "url(#" + id + ")"
			op = 1
			if l.Paint.Radial {
				r := math.Hypot(l.Paint.To.X-l.Paint.From.X, l.Paint.To.Y-l.Paint.From.Y)
				fmt.Fprintf(&defs, `<radialGradient id="%s" gradientUnits="userSpaceOnUse" cx="%s" cy="%s" r="%s">`, id, num(l.Paint.From.X), num(l.Paint.From.Y), num(r))
			} else {
				fmt.Fprintf(&defs, `<linearGradient id="%s" gradientUnits="userSpaceOnUse" x1="%s" y1="%s" x2="%s" y2="%s">`, id, num(l.Paint.From.X), num(l.Paint.From.Y), num(l.Paint.To.X), num(l.Paint.To.Y))
			}
			for _, s := range l.Paint.Stops {
				fmt.Fprintf(&defs, `<stop offset="%s" stop-color="%s"`, num(s.At), s.C.hex())
				if s.C.A < 255 {
					fmt.Fprintf(&defs, ` stop-opacity="%s"`, num(float64(s.C.A)/255))
				}
				defs.WriteString("/>")
			}
			if l.Paint.Radial {
				defs.WriteString("</radialGradient>")
			} else {
				defs.WriteString("</linearGradient>")
			}
		}
		fmt.Fprintf(&body, `<path fill="%s"`, fill)
		if op < 1 {
			fmt.Fprintf(&body, ` fill-opacity="%s"`, num(op))
		}
		fmt.Fprintf(&body, ` d="%s"/>`, l.Path.d())
	}
	var out strings.Builder
	fmt.Fprintf(&out, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %s %s" width="%s" height="%s">`, num(a.W), num(a.H), num(a.W), num(a.H))
	if defs.Len() > 0 {
		out.WriteString("<defs>" + defs.String() + "</defs>")
	}
	out.WriteString(body.String())
	out.WriteString("</svg>\n")
	return []byte(out.String())
}

// VectorDrawable writes the drawing as an Android vector drawable of wdp×hdp.
func (a Art) VectorDrawable(wdp, hdp float64, comment string) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	if comment != "" {
		b.WriteString("<!-- " + comment + " -->\n")
	}
	fmt.Fprintf(&b, "<vector xmlns:android=\"http://schemas.android.com/apk/res/android\"\n    xmlns:aapt=\"http://schemas.android.com/aapt\"\n    android:width=\"%sdp\"\n    android:height=\"%sdp\"\n    android:viewportWidth=\"%s\"\n    android:viewportHeight=\"%s\">\n", num(wdp), num(hdp), num(a.W), num(a.H))
	for _, l := range a.Layers {
		if len(l.Paint.Stops) == 0 {
			fmt.Fprintf(&b, "    <path android:fillColor=\"%s\" android:pathData=\"%s\" />\n", l.Paint.Solid.android(), l.Path.d())
			continue
		}
		fmt.Fprintf(&b, "    <path android:pathData=\"%s\">\n        <aapt:attr name=\"android:fillColor\">\n", l.Path.d())
		if l.Paint.Radial {
			r := math.Hypot(l.Paint.To.X-l.Paint.From.X, l.Paint.To.Y-l.Paint.From.Y)
			fmt.Fprintf(&b, "            <gradient android:type=\"radial\" android:centerX=\"%s\" android:centerY=\"%s\" android:gradientRadius=\"%s\">\n", num(l.Paint.From.X), num(l.Paint.From.Y), num(r))
		} else {
			fmt.Fprintf(&b, "            <gradient android:type=\"linear\" android:startX=\"%s\" android:startY=\"%s\" android:endX=\"%s\" android:endY=\"%s\">\n", num(l.Paint.From.X), num(l.Paint.From.Y), num(l.Paint.To.X), num(l.Paint.To.Y))
		}
		for _, s := range l.Paint.Stops {
			fmt.Fprintf(&b, "                <item android:offset=\"%s\" android:color=\"%s\" />\n", num(s.At), s.C.android())
		}
		b.WriteString("            </gradient>\n        </aapt:attr>\n    </path>\n")
	}
	b.WriteString("</vector>\n")
	return []byte(b.String())
}
