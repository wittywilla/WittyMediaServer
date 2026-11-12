// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package brandart

import "math"

// The wide pictures: TV icons and their layers, banners, Top Shelf, splash and installer art,
// the website's hero and social picture. Each is an Art in pixels of its own size.

// placeGlyph puts layers designed on the 1024 page at centre c, so that the glyph's circle (0.65 of
// the page) is h high.
func placeGlyph(layers []Layer, c Pt, h float64) []Layer {
	k := h / (Page * 0.65)
	src := Pt{Page / 2, Page / 2}
	f := func(q Pt) Pt { return c.add(q.sub(src).mul(k)) }
	var out []Layer
	for _, l := range layers {
		p := l.Paint
		p.From, p.To = f(p.From), f(p.To)
		out = append(out, Layer{l.Path.Map(f), p})
	}
	return out
}

// backPaints stretches the app's background paints over a w×h rectangle.
func backPaints(m Mark, w, h float64) []Layer {
	sx, sy := w/Page, h/Page
	f := func(q Pt) Pt { return Pt{q.X * sx, q.Y * sy} }
	var out []Layer
	for _, p := range m.Back {
		p.From, p.To = f(p.From), f(p.To)
		if p.Radial {
			// Keep a glow round: its reach is measured across the longer side.
			r := math.Hypot(p.To.X-p.From.X, p.To.Y-p.From.Y) / sx * math.Max(sx, sy)
			p.To = Pt{p.From.X + r, p.From.Y}
		}
		out = append(out, Layer{Rect(0, 0, w, h, 0), p})
	}
	return out
}

// TVLayers is a tvOS app icon (w×h, 400×240 or 1280×768) as its parallax layers, back to front:
// the background with a glow, the character's body, and its face with the spark. The system
// moves them apart as the icon is focused, so the face floats in front of the body.
func (k Kit) TVLayers(p Product, w, h float64) []Art {
	m := k.Mark(p, 0)
	gh := h * 0.80
	c := Pt{w / 2, h / 2}
	back := Art{W: w, H: h, Layers: backPaints(m, w, h)}
	// A second, larger glow behind the character makes the depth readable when it tilts.
	back.Layers = append(back.Layers, Layer{Rect(0, 0, w, h, 0), Glow(w/2, h/2, h*0.75, Hex("#ffffff").WithAlpha(0.10))})
	body := Art{W: w, H: h, Layers: placeGlyph([]Layer{m.Body}, c, gh)}
	front := Art{W: w, H: h, Layers: placeGlyph(m.Front(), c, gh)}
	return []Art{back, body, front}
}

// TVIcon is the tvOS icon flattened.
func (k Kit) TVIcon(p Product, w, h float64) Art {
	l := k.TVLayers(p, w, h)
	return l[2].Over(l[1].Over(l[0]))
}

// Banner is a 16:9 launcher banner (Android TV's 320×180): the mark and the app's name on the
// app's background.
func (k Kit) Banner(p Product, w, h float64) Art {
	m := k.Mark(p, 0)
	a := Art{W: w, H: h, Layers: backPaints(m, w, h)}
	mark := h * 0.52
	size := h * 0.17
	runs := k.Wordmark(p, k.Pal.Text, k.Pal.AccentLite)
	tw := TextWidth(runs, size, true)
	if tw > w*0.86 {
		size *= w * 0.86 / tw
		tw = w * 0.86
	}
	top := (h - mark - size*1.5) / 2
	a.Layers = append(a.Layers, placeGlyph(m.Glyph(), Pt{w / 2, top + mark/2}, mark*0.92)...)
	words, _ := TextArt(runs, (w-tw)/2, top+mark+size*1.25, size, true)
	return a.With(words...)
}

// TopShelf is the Apple TV's Top Shelf picture (1920×720, or 2320×720 wide): the family's scene,
// the client's mark and its name, kept inside the middle where every TV shows it.
func (k Kit) TopShelf(w, h float64) Art {
	lock, lw := k.Lockup(Client, 0, 0, h*0.30, k.Pal.Text, k.Pal.AccentLite)
	x := (w - lw) / 2
	y := (h - h*0.30) / 2
	a := k.Scene(w, h, 7, false, Box{x - h*0.08, y - h*0.08, x + lw + h*0.08, y + h*0.38})
	for i := range lock {
		lock[i] = shift(lock[i], x, y)
	}
	return a.With(lock...)
}

func shift(l Layer, dx, dy float64) Layer {
	p := l.Paint
	p.From, p.To = Pt{p.From.X + dx, p.From.Y + dy}, Pt{p.To.X + dx, p.To.Y + dy}
	return Layer{l.Path.Moved(dx, dy), p}
}

// Backdrop is a full-screen scene with nothing written on it, for behind a sign-in or connect
// screen on a TV (1920×1080) or a phone. The mark sits large and faint toward the right, so
// whatever the app draws on the left stays readable.
func (k Kit) Backdrop(p Product, w, h float64, light bool) Art {
	a := k.Scene(w, h, 3, light)
	m := k.Mark(p, 0)
	ghost := k.Pal.AccentLite.WithAlpha(0.06)
	if light {
		ghost = k.Pal.Accent.WithAlpha(0.06)
	}
	return a.With(placeGlyph([]Layer{{m.MonoPath(), Fill(ghost)}}, Pt{w * 0.78, h * 0.52}, h*0.9)...)
}

// Splash is a launch picture: the mark alone, centred, size s, on nothing (the platform paints
// the background colour).
func (k Kit) Splash(p Product, s float64) Art {
	m := k.Mark(p, 0)
	return Art{W: s, H: s, Layers: placeGlyph(m.Glyph(), Pt{s / 2, s / 2}, s)}
}

// Social is a wide picture with an app's lockup and a line under it: the 1200×630 picture link
// previews show, Google Play's 1024×500 feature graphic.
func (k Kit) Social(p Product, w, h float64, line string) Art {
	lh := h * 0.24
	lock, lw := k.Lockup(p, 0, 0, lh, k.Pal.Text, k.Pal.AccentLite)
	x, y := (w-lw)/2, h*0.32
	a := k.Scene(w, h, 11, false, Box{math.Min(x, w*0.06) - 10, y - lh*0.25, math.Max(x+lw, w*0.94) + 10, y + lh + h*0.2})
	for i := range lock {
		lock[i] = shift(lock[i], x, y)
	}
	a = a.With(lock...)
	size := h * 0.055
	runs := []Run{{line, Mix(k.Pal.Text, k.Pal.Night, 0.2)}}
	tw := TextWidth(runs, size, false)
	words, _ := TextArt(runs, (w-tw)/2, y+lh+h*0.14, size, false)
	return a.With(words...)
}

// Hero is the website's background picture behind its first section: the scene, no words, no
// mark (the page draws its own).
func (k Kit) Hero(w, h float64, light bool) Art { return k.Scene(w, h, 5, light) }

// InstallerPanel is a tall picture for an installer's first and last pages (Windows Installer's
// 164×312 strip, NSIS's 164×314): the scene and the mark near the top.
func (k Kit) InstallerPanel(p Product, w, h float64) Art {
	a := k.Scene(w, h, 13, false, Box{w * 0.1, h * 0.06, w * 0.9, h * 0.42})
	t := k.Tile(p, int(w*0.6)).Placed(0, 0, Pt{w / 2, h * 0.24}, w*0.6/Page)
	return a.With(t.Layers...)
}

// InstallerBanner is a strip along the top of installer pages (Windows Installer's 493×58, NSIS's
// 150×57): white, the mark at the right.
func (k Kit) InstallerBanner(p Product, w, h float64) Art {
	a := Art{W: w, H: h, Layers: []Layer{{Rect(0, 0, w, h, 0), Fill(Hex("#ffffff"))}}}
	s := h * 0.76
	t := k.Tile(p, int(s)).Placed(0, 0, Pt{w - s/2 - h*0.12, h / 2}, s/Page)
	return a.With(t.Layers...)
}

// DiskImage is a macOS disk image window's background (540×400 points, drawn at 2×): the scene,
// light, with an arrow from where the app sits to where Applications sits.
func (k Kit) DiskImage(w, h float64, scale float64, appX, linkX, iconY float64) Art {
	y := iconY * scale
	clear := func(cx, cy float64) Box {
		return Box{(cx - 80) * scale, (cy - 80) * scale, (cx + 80) * scale, (cy + 100) * scale}
	}
	a := k.Scene(w*scale, h*scale, 17, true, clear(appX, iconY), clear(linkX, iconY), clear((appX+linkX)/2, h-100), Box{(appX + 64) * scale, y - 40*scale, (linkX - 64) * scale, y + 40*scale})
	x0, x1 := (appX+64)*scale, (linkX-64)*scale
	arrow := Capsule(x0, y, x1-14*scale, y, 10*scale).Add(Triangle(4*scale, Pt{x1 - 26*scale, y - 18*scale}, Pt{x1, y}, Pt{x1 - 26*scale, y + 18*scale}))
	return a.With(Layer{arrow.Solid(), Fill(k.Pal.Accent.WithAlpha(0.75))})
}
