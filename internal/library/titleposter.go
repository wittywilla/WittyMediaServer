// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf16"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// A title poster: a title's own words on a colour of its own, or over a frame from its
// video. Made here as a picture, so every app and device shows the same thing, and kept in
// the image cache like any other poster. A person can make one in the artwork picker for any
// title, and the scan can make one for a film that has no poster (the noPoster setting).

// SourceMade marks a poster made from the title's words.
const SourceMade = "made"

// NoPoster settings: what a film or video with no poster of its own gets.
const (
	NoPosterFrame = "frame" // a frame from the video (the default)
	NoPosterTitle = "title" // its title, made into a poster
	NoPosterNone  = "none"  // nothing: apps draw their own placeholder
)

// TitlePoster is what a made poster says and how it looks.
type TitlePoster struct {
	Title string `json:"title"`
	Line  string `json:"line"`  // a second, smaller line, such as the year
	Hue   int    `json:"hue"`   // 0 to 359; -1 is the title's own colour
	Frame string `json:"frame"` // a cache: picture to set the words over, or ""
	// Auto: made by the scan for a title with no poster, not chosen by anyone. It's
	// named apart, so a real poster replaces it and it's never written beside the media.
	Auto bool `json:"-"`
}

const (
	madeW, madeH   = 1000, 1500
	madeMaxTitle   = 120
	madeMaxLine    = 60
	madeVersion    = "2" // part of the cache name: a change to the drawing makes new files
	madeTitleSize  = 132.0
	madeMinSize    = 56.0
	madeMaxLines   = 6
	madeMarginX    = 90
	madeLineFactor = 1.12
)

// HueOf is a title's own colour, worked out from its words as the web app's artHue does
// (a letter at a time, by its first UTF-16 unit), so a made poster and a card agree.
func HueOf(title string) int {
	h := 0
	for _, r := range title {
		c := int(r)
		if r > 0xFFFF {
			hi, _ := utf16.EncodeRune(r)
			c = int(hi)
		}
		h = (h*31 + c) % 360
	}
	return h
}

// IsMadePoster: a poster this server made by itself because there was none (a frame, or
// a title poster), which a picture from a source or beside the file may replace. One a
// person made and chose in the artwork picker is theirs, like any other choice.
func IsMadePoster(v string) bool {
	return strings.HasPrefix(v, "cache:thumb-") || strings.HasPrefix(v, "cache:auto-")
}

// fallbackPoster is what a video with no poster of its own gets, by the noPoster setting:
// its address and where it came from, or nothing.
func (s *Scanner) fallbackPoster(ctx context.Context, it *Item) (string, string, error) {
	mode := NoPosterFrame
	if s.NoPoster != nil {
		mode = s.NoPoster()
	}
	switch mode {
	case NoPosterTitle:
		line := ""
		if it.Year != nil && *it.Year > 0 {
			line = fmt.Sprint(*it.Year)
		}
		p, err := s.MakeTitlePoster(TitlePoster{Title: it.Title, Line: line, Hue: -1, Auto: true})
		return p, SourceMade, err
	case NoPosterFrame:
		if it.Disc != nil && it.Disc.Scrambled {
			return "", "", nil
		}
		p, err := s.thumbnail(ctx, it)
		return p, SourceFrame, err
	}
	return "", "", nil
}

// MakeTitlePoster draws a title poster into the image cache and returns its address.
// The same words and look always give the same file.
func (s *Scanner) MakeTitlePoster(p TitlePoster) (string, error) {
	p.Title = strings.Join(strings.Fields(p.Title), " ")
	p.Line = strings.Join(strings.Fields(p.Line), " ")
	if p.Title == "" {
		return "", errors.New("a title poster needs some words")
	}
	if len([]rune(p.Title)) > madeMaxTitle || len([]rune(p.Line)) > madeMaxLine {
		return "", fmt.Errorf("a title poster holds up to %d letters, and %d on its second line", madeMaxTitle, madeMaxLine)
	}
	if p.Hue < 0 || p.Hue > 359 {
		p.Hue = HueOf(p.Title)
	}
	var under image.Image
	if p.Frame != "" {
		name := strings.TrimPrefix(p.Frame, "cache:")
		if !strings.HasPrefix(p.Frame, "cache:") || name == "" || name != filepath.Base(name) {
			return "", errors.New("a title poster can only be made over a frame from the video")
		}
		f, err := os.Open(filepath.Join(s.ImagesDir, name))
		if err != nil {
			return "", errors.New("that frame from the video has gone: take it again")
		}
		under, _, err = image.Decode(f)
		f.Close()
		if err != nil {
			return "", errors.New("that frame from the video couldn't be read")
		}
	}
	sum := sha1.Sum([]byte(strings.Join([]string{madeVersion, p.Title, p.Line, fmt.Sprint(p.Hue), p.Frame}, "\x00")))
	name := "made-" + hex.EncodeToString(sum[:10]) + ".jpg"
	if p.Auto {
		name = "auto-" + hex.EncodeToString(sum[:10]) + ".jpg"
	}
	out := filepath.Join(s.ImagesDir, name)
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return "cache:" + name, nil
	}
	b, err := drawTitlePoster(p, under)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.ImagesDir, 0o755); err != nil {
		return "", err
	}
	tmp := out + ".part"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, out); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return "cache:" + name, nil
}

var (
	madeFonts     struct{ bold, medium *opentype.Font }
	madeFontsOnce sync.Once
	madeFontsErr  error
)

func titleFonts() (bold, medium *opentype.Font, err error) {
	madeFontsOnce.Do(func() {
		if madeFonts.bold, madeFontsErr = opentype.Parse(gobold.TTF); madeFontsErr == nil {
			madeFonts.medium, madeFontsErr = opentype.Parse(gomedium.TTF)
		}
	})
	return madeFonts.bold, madeFonts.medium, madeFontsErr
}

func drawTitlePoster(p TitlePoster, under image.Image) ([]byte, error) {
	bold, medium, err := titleFonts()
	if err != nil {
		return nil, err
	}
	img := image.NewRGBA(image.Rect(0, 0, madeW, madeH))
	// The same colours as a card without a poster: lighter at the top left, darker below.
	top, bottom := hsl(float64(p.Hue), .32, .32), hsl(float64((p.Hue+40)%360), .38, .15)
	textTop := 0.0 // where the words sit: the middle, or low over a picture
	if under == nil {
		for y := 0; y < madeH; y++ {
			for x := 0; x < madeW; x++ {
				t := (float64(y)*0.94 + float64(x)*0.34) / (madeH*0.94 + madeW*0.34)
				img.SetRGBA(x, y, mix(top, bottom, t))
			}
		}
	} else {
		// Cover the poster with the frame, then darken it towards the bottom for the words.
		b := under.Bounds()
		scale := math.Max(float64(madeW)/float64(b.Dx()), float64(madeH)/float64(b.Dy()))
		w, h := int(float64(b.Dx())*scale), int(float64(b.Dy())*scale)
		dst := image.Rect((madeW-w)/2, (madeH-h)/2, (madeW-w)/2+w, (madeH-h)/2+h)
		xdraw.ApproxBiLinear.Scale(img, dst, under, b, draw.Src, nil)
		for y := 0; y < madeH; y++ {
			a := uint8(70 + 150*math.Pow(float64(y)/madeH, 1.6))
			draw.Draw(img, image.Rect(0, y, madeW, y+1), image.NewUniform(color.RGBA{0, 0, 0, a}), image.Point{}, draw.Over)
		}
		textTop = 0.58
	}

	// The biggest size that fits in a few lines.
	size := madeTitleSize
	var face font.Face
	var lines []string
	for {
		face, err = opentype.NewFace(bold, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
		if err != nil {
			return nil, err
		}
		lines = wrapWords(face, p.Title, madeW-2*madeMarginX)
		if (len(lines) <= madeMaxLines && widest(face, lines) <= madeW-2*madeMarginX) || size <= madeMinSize {
			break
		}
		face.Close()
		size -= 8
	}
	defer face.Close()
	lineH := size * madeLineFactor
	var small font.Face
	smallH := 0.0
	if p.Line != "" {
		if small, err = opentype.NewFace(medium, &opentype.FaceOptions{Size: 52, DPI: 72, Hinting: font.HintingFull}); err != nil {
			return nil, err
		}
		defer small.Close()
		smallH = 52 * 2.1
	}
	block := lineH*float64(len(lines)) + smallH
	y := (madeH - block) / 2
	if textTop > 0 {
		y = math.Min(madeH*textTop, madeH-block-110)
	}
	ascent := float64(face.Metrics().Ascent.Round())
	for _, l := range lines {
		say(img, face, l, y+ascent, color.NRGBA{255, 255, 255, 255})
		y += lineH
	}
	if small != nil {
		say(img, small, p.Line, y+52*1.6, color.NRGBA{255, 255, 255, 215})
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 88}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// say draws a line of words centred, with a soft shadow so it reads over any picture.
func say(img draw.Image, face font.Face, s string, baseline float64, c color.NRGBA) {
	w := font.MeasureString(face, s).Round()
	x := (madeW - w) / 2
	for _, o := range []struct {
		dx, dy int
		c      color.NRGBA
	}{{0, 4, color.NRGBA{0, 0, 0, 90}}, {0, 0, c}} {
		d := &font.Drawer{Dst: img, Src: image.NewUniform(o.c), Face: face, Dot: fixed.P(x+o.dx, int(baseline)+o.dy)}
		d.DrawString(s)
	}
}

// wrapWords breaks words into lines no wider than width. A word longer than a line is
// left whole, for the caller to try a smaller size.
func wrapWords(face font.Face, s string, width int) []string {
	var lines []string
	cur := ""
	for _, w := range strings.Fields(s) {
		next := w
		if cur != "" {
			next = cur + " " + w
		}
		if cur != "" && font.MeasureString(face, next).Round() > width {
			lines = append(lines, cur)
			cur = w
			continue
		}
		cur = next
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

func widest(face font.Face, lines []string) int {
	m := 0
	for _, l := range lines {
		m = max(m, font.MeasureString(face, l).Round())
	}
	return m
}

func mix(a, b color.RGBA, t float64) color.RGBA {
	f := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return color.RGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), 255}
}

// hsl is CSS's hsl(): hue in degrees, saturation and lightness from 0 to 1.
func hsl(h, s, l float64) color.RGBA {
	c := (1 - math.Abs(2*l-1)) * s
	hp := math.Mod(h, 360) / 60
	x := c * (1 - math.Abs(math.Mod(hp, 2)-1))
	var r, g, b float64
	switch {
	case hp < 1:
		r, g = c, x
	case hp < 2:
		r, g = x, c
	case hp < 3:
		g, b = c, x
	case hp < 4:
		g, b = x, c
	case hp < 5:
		r, b = x, c
	default:
		r, b = c, x
	}
	m := l - c/2
	to := func(v float64) uint8 { return uint8(math.Round((v + m) * 255)) }
	return color.RGBA{to(r), to(g), to(b), 255}
}
