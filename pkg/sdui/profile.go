// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"math"
	"strconv"
	"strings"
)

// Viewport is the size of the client's window in points.
type Viewport struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Insets are distances from each edge of the screen, in points.
type Insets struct {
	Top    int `json:"top"`
	Right  int `json:"right"`
	Bottom int `json:"bottom"`
	Left   int `json:"left"`
}

// CardMetrics is how big one card style is on a device, in points. Width is
// by card size (s, m, l); the height of the picture follows from Aspect.
type CardMetrics struct {
	Shape  string         `json:"shape"`
	Aspect string         `json:"aspect,omitempty"`
	Width  map[string]int `json:"width"`
	// Columns is how many of the medium card a Grid has on a device whose
	// grids don't follow the window (a television); 0 elsewhere.
	Columns int `json:"columns,omitempty"`
}

// Budget is how much of a page one device is sent at once.
type Budget struct {
	// MaxSections is the most sections a page has; the rest are left out and
	// Layout.Truncated set.
	MaxSections int `json:"maxSections"`
	// EagerSections is how many data sections arrive with their items. The
	// ones after are "deferred": the client asks for their items as they
	// near the screen.
	EagerSections int `json:"eagerSections"`
	// ShelfItems, GridItems and ListItems are the most items a section's
	// first page holds; PageSize the size of each page after.
	ShelfItems int `json:"shelfItems"`
	GridItems  int `json:"gridItems"`
	ListItems  int `json:"listItems"`
	PageSize   int `json:"pageSize"`
	// HeroItems is the most titles a Hero rotates through.
	HeroItems int `json:"heroItems"`
	// Repeats is the most copies a repeated section makes.
	Repeats int `json:"repeats"`
}

// TextLimits are the most characters the server sends for each kind of text
// on a device. Longer text is cut at a word with an ellipsis (see Truncate).
type TextLimits struct {
	Title    int `json:"title"`
	Subtitle int `json:"subtitle"`
	Synopsis int `json:"synopsis"`
	Overview int `json:"overview"`
}

// Layout is a device profile: what a page has to fit, and the numbers a
// renderer lays out with. Render fits a page to it and sends it along as
// Document.Layout.
//
// Sizes are in points: what the platform calls points, dp or CSS pixels.
// Scale is how many picture pixels a point is, used only to choose pictures.
type Layout struct {
	// FormFactor is tv, phone, tablet, desktop or web; Class tells devices of
	// one form factor apart ("hd" and "4k" for tv).
	FormFactor string `json:"formFactor"`
	Class      string `json:"class,omitempty"`
	// Canvas is the screen in points when the form factor fixes it (tv:
	// 1920×1080).
	Canvas Viewport `json:"canvas,omitzero"`
	Scale  int      `json:"scale"`
	// Breakpoint is compact (under 600 points wide), medium (under 840),
	// expanded (under 1200) or large, from the window's width when the
	// client sent it, else the form factor's usual.
	Breakpoint string `json:"breakpoint"`
	// SafeArea is the margin content keeps inside. Zero where the system
	// reports its own.
	SafeArea Insets `json:"safeArea"`
	// FocusEngine is true where focus is the only pointer (tv).
	FocusEngine bool `json:"focusEngine"`
	// Density is compact, comfortable or spacious.
	Density string `json:"density"`
	// Spacing is each spacing step in points; Text each text role's size.
	Spacing map[string]int `json:"spacing"`
	Text    map[string]int `json:"text"`
	// Cards is the size of each card style; Hero the size of a Hero.
	Cards map[string]CardMetrics `json:"cards"`
	Hero  Viewport               `json:"hero"`
	// ImageWidths are the widths pictures are asked for at: a card asks for
	// the smallest that is at least its width times Scale.
	ImageWidths []int      `json:"imageWidths"`
	Budget      Budget     `json:"budget"`
	TextLimits  TextLimits `json:"textLimits"`
	// ExternalLinks says whether the device can open a web page.
	ExternalLinks bool `json:"externalLinks"`
	// ReducedMotion is true when the client said the person asked for it.
	ReducedMotion bool `json:"reducedMotion,omitempty"`
	// Truncated is set when sections were left out to fit MaxSections.
	Truncated bool `json:"truncated,omitempty"`
	// Shell is the layout option the page was made for and how the app's frame is
	// drawn with it: see Layouts.
	Shell *Shell `json:"shell,omitempty"`
}

// imageWidths are the buckets every device chooses pictures from, so the
// server's picture cache holds a handful of sizes rather than one per device.
var imageWidths = []int{120, 160, 240, 320, 480, 640, 960, 1280, 1920, 2560, 3840}

// base card widths in points at size m, by form factor and shape.
var baseWidths = map[string]map[string]int{
	FormTV:      {"poster": 250, "landscape": 420, "square": 280, "circle": 220, "chip": 0},
	FormPhone:   {"poster": 120, "landscape": 224, "square": 144, "circle": 96, "chip": 0},
	FormTablet:  {"poster": 160, "landscape": 288, "square": 180, "circle": 120, "chip": 0},
	FormDesktop: {"poster": 180, "landscape": 320, "square": 200, "circle": 140, "chip": 0},
}

// tvColumns is how many medium cards of each shape a television's grid has.
var tvColumns = map[string]int{"poster": 6, "landscape": 4, "square": 6, "circle": 7}

// sizeFactor scales a medium card to small and large, in percent.
var sizeFactor = map[string]int{"s": 80, "m": 100, "l": 125}

func even(n int) int { return n / 2 * 2 }

func cardMetrics(ff string) map[string]CardMetrics {
	base := baseWidths[ff]
	if base == nil {
		base = baseWidths[FormDesktop]
	}
	out := map[string]CardMetrics{}
	for _, st := range cardStylesOnce() {
		m := CardMetrics{Shape: st.Shape, Aspect: st.Aspect, Width: map[string]int{}}
		for size, f := range sizeFactor {
			m.Width[size] = even(base[st.Shape] * f / 100)
		}
		if ff == FormTV {
			m.Columns = tvColumns[st.Shape]
		}
		out[st.Style] = m
	}
	return out
}

// ProfileFor is the device profile for a form factor and class. An unknown
// form factor gets the web profile; an unknown television class the HD one,
// the smaller of the two.
func ProfileFor(formFactor, class string) *Layout {
	l := &Layout{FormFactor: formFactor, Scale: 2, Density: "comfortable", ImageWidths: imageWidths, ExternalLinks: true}
	switch formFactor {
	case FormTV:
		l.Class, l.Scale, l.Canvas, l.Breakpoint = "hd", 1, Viewport{Width: 1920, Height: 1080}, "large"
		l.SafeArea = Insets{Top: 60, Right: 80, Bottom: 60, Left: 80}
		l.FocusEngine, l.ExternalLinks = true, false
		l.Spacing = map[string]int{"none": 0, "xs": 10, "s": 20, "m": 40, "l": 60, "xl": 100}
		// tvOS's own text styles: title 1, title 2, title 3, body, caption 1.
		l.Text = map[string]int{"pageTitle": 76, "heading": 57, "title": 48, "body": 29, "caption": 25, "badge": 23}
		l.Hero = Viewport{Width: 1920, Height: 620}
		l.Budget = Budget{MaxSections: 12, EagerSections: 4, ShelfItems: 12, GridItems: 36, ListItems: 30, PageSize: 24, HeroItems: 5, Repeats: 8}
		l.TextLimits = TextLimits{Title: 80, Subtitle: 80, Synopsis: 160, Overview: 480}
		if class == "4k" {
			l.Class, l.Scale = "4k", 2
			l.Budget = Budget{MaxSections: 20, EagerSections: 6, ShelfItems: 20, GridItems: 60, ListItems: 50, PageSize: 36, HeroItems: 5, Repeats: 12}
		}
	case FormPhone:
		l.Scale, l.Breakpoint = 3, "compact"
		l.Spacing = map[string]int{"none": 0, "xs": 4, "s": 8, "m": 16, "l": 24, "xl": 40}
		l.Text = map[string]int{"pageTitle": 28, "heading": 22, "title": 17, "body": 16, "caption": 13, "badge": 11}
		l.Hero = Viewport{Width: 430, Height: 242}
		l.Budget = Budget{MaxSections: 16, EagerSections: 5, ShelfItems: 12, GridItems: 30, ListItems: 30, PageSize: 30, HeroItems: 5, Repeats: 10}
		l.TextLimits = TextLimits{Title: 80, Subtitle: 80, Synopsis: 140, Overview: 400}
	case FormTablet:
		l.Breakpoint = "expanded"
		l.Spacing = map[string]int{"none": 0, "xs": 4, "s": 8, "m": 16, "l": 24, "xl": 48}
		l.Text = map[string]int{"pageTitle": 34, "heading": 24, "title": 20, "body": 17, "caption": 13, "badge": 12}
		l.Hero = Viewport{Width: 1024, Height: 440}
		l.Budget = Budget{MaxSections: 20, EagerSections: 6, ShelfItems: 16, GridItems: 48, ListItems: 40, PageSize: 36, HeroItems: 5, Repeats: 12}
		l.TextLimits = TextLimits{Title: 100, Subtitle: 100, Synopsis: 200, Overview: 700}
	default:
		if formFactor != FormDesktop {
			l.FormFactor = FormWeb
		}
		l.Breakpoint = "large"
		l.Spacing = map[string]int{"none": 0, "xs": 4, "s": 8, "m": 16, "l": 24, "xl": 48}
		l.Text = map[string]int{"pageTitle": 28, "heading": 20, "title": 16, "body": 14, "caption": 12, "badge": 11}
		l.Hero = Viewport{Width: 1600, Height: 620}
		l.Budget = Budget{MaxSections: 30, EagerSections: 8, ShelfItems: 20, GridItems: 60, ListItems: 50, PageSize: 48, HeroItems: 6, Repeats: 16}
		l.TextLimits = TextLimits{Title: 120, Subtitle: 120, Synopsis: 220, Overview: 900}
	}
	ff := l.FormFactor
	if ff == FormWeb {
		ff = FormDesktop
	}
	l.Cards = cardMetrics(ff)
	return l
}

// Breakpoint names the window class for a width in points.
func Breakpoint(width int) string {
	switch {
	case width < 600:
		return "compact"
	case width < 840:
		return "medium"
	case width < 1200:
		return "expanded"
	}
	return "large"
}

// WithViewport returns the profile for a window of this size: the breakpoint
// follows the width. A television's canvas is fixed and is left alone.
func (l *Layout) WithViewport(v Viewport) *Layout {
	c := l.clone()
	if c.FormFactor != FormTV && v.Width > 0 {
		c.Breakpoint = Breakpoint(v.Width)
		c.Canvas = v
	}
	return c
}

// WithDensity returns the profile at another density: compact tightens the
// spacing by a quarter and spacious loosens it by a quarter. Card sizes are
// the profile's own choice per section, not the density's.
func (l *Layout) WithDensity(d string) *Layout {
	if d == "" || d == l.Density || !contains(Densities, d) {
		return l
	}
	c := l.clone()
	c.Density = d
	f := 100
	switch d {
	case "compact":
		f = 75
	case "spacious":
		f = 125
	}
	base := ProfileFor(l.FormFactor, l.Class).Spacing
	for k, v := range base {
		c.Spacing[k] = even(v * f / 100)
	}
	return c
}

func (l *Layout) clone() *Layout {
	if l == nil {
		return nil
	}
	c := *l
	c.Spacing, c.Text = map[string]int{}, map[string]int{}
	for k, v := range l.Spacing {
		c.Spacing[k] = v
	}
	for k, v := range l.Text {
		c.Text[k] = v
	}
	c.Cards = make(map[string]CardMetrics, len(l.Cards))
	for k, v := range l.Cards {
		w := make(map[string]int, len(v.Width))
		for s, n := range v.Width {
			w[s] = n
		}
		v.Width = w
		c.Cards[k] = v
	}
	return &c
}

// ImageWidth is the picture width a card of this width in points asks for:
// the smallest bucket that covers it at the device's scale, or the largest.
func (l *Layout) ImageWidth(points int) int {
	return l.imageWidthAt(points, l.Scale)
}

// BackdropWidth is the picture width for a backdrop that spans a window this wide in
// points: at most twice the width, whatever the device's scale. A backdrop sits behind
// text, under a scrim, so a phone's third pixel per point is never worth its bytes.
func (l *Layout) BackdropWidth(points int) int {
	return l.imageWidthAt(points, min(l.Scale, 2))
}

func (l *Layout) imageWidthAt(points, scale int) int {
	want := points * max(scale, 1)
	for _, w := range l.ImageWidths {
		if w >= want {
			return w
		}
	}
	return l.ImageWidths[len(l.ImageWidths)-1]
}

// GridColumns is how many columns a Grid of cards this wide has: the
// television's fixed count, or as many as fit the window, at least two (three
// for cards under 130 points). The rule a renderer applies when its window
// changes size:
//
//	columns = max(minimum, floor((content + gap) / (card + gap)))
//
// where content is ContentWidth and gap the s plus the xs spacing on compact
// windows, the m spacing otherwise.
func (l *Layout) GridColumns(style, size string) int {
	m := l.Cards[style]
	if m.Columns > 0 {
		cols := m.Columns
		switch size {
		case "s":
			cols++
		case "l":
			cols--
		}
		return cols
	}
	card := m.Width[size]
	if card <= 0 {
		return 0
	}
	gap := l.Spacing["m"]
	if l.Breakpoint == "compact" {
		gap = l.Spacing["s"] + l.Spacing["xs"]
	}
	content := l.ContentWidth()
	minimum := 2
	if card < 130 {
		minimum = 3
	}
	return max(minimum, (content+gap)/(card+gap))
}

// aspectHeight is the height of a picture of this width and aspect ("16/9").
func aspectHeight(width int, aspect string) int {
	w, h, ok := strings.Cut(aspect, "/")
	wn, _ := strconv.Atoi(w)
	hn, _ := strconv.Atoi(h)
	if !ok || wn <= 0 || hn <= 0 {
		return width
	}
	return int(math.Round(float64(width) * float64(hn) / float64(wn)))
}

// Truncate cuts s to at most limit characters: at the last space within 20
// characters of the limit when there is one, trailing spaces and punctuation
// removed, an ellipsis added. Text within the limit is returned unchanged.
// Every renderer that shortens text itself (a lazily loaded page it formats)
// uses the same rule, which the conformance kit checks.
func Truncate(s string, limit int) string {
	s = strings.TrimSpace(s)
	if limit <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	cut := limit - 1
	for i := cut; i > cut-20 && i > 0; i-- {
		if r[i] == ' ' {
			cut = i
			break
		}
	}
	return strings.TrimRight(string(r[:cut]), " ,.;:-–—") + "…"
}

// WindowWidth is the window's width in points: the canvas the client sent (a
// television's fixed one), else the breakpoint's usual window. A picture that
// spans the window, such as a title's backdrop, is chosen for this width.
func (l *Layout) WindowWidth() int {
	if l.Canvas.Width > 0 {
		return l.Canvas.Width
	}
	return map[string]int{"compact": 390, "medium": 720, "expanded": 1024, "large": 1440}[l.Breakpoint]
}

// ContentWidth is the width a page's sections have to themselves, in points:
// the canvas (or the breakpoint's usual window when the client didn't say)
// less the side margins: the safe area on a television, the m spacing
// elsewhere.
func (l *Layout) ContentWidth() int {
	width := l.WindowWidth()
	if l.SafeArea.Left+l.SafeArea.Right > 0 {
		return width - l.SafeArea.Left - l.SafeArea.Right
	}
	return width - 2*l.SideMargin()
}

// SideMargin is the margin at each side of a page's content, in points: the
// safe area's left inset on a television, the m spacing on every other form
// factor (16 points on a phone, a tablet, a desktop and the web).
func (l *Layout) SideMargin() int {
	if l.SafeArea.Left+l.SafeArea.Right > 0 {
		return l.SafeArea.Left
	}
	return l.Spacing["m"]
}

// Placeholders is how many grey placeholder cards a row of cards this wide
// shows while it loads: as many as are at least partly on screen, and no
// more than the row's first page.
func (l *Layout) Placeholders(cardWidth int) int {
	if cardWidth <= 0 {
		return 0
	}
	gap := l.Spacing["m"]
	if l.Breakpoint == "compact" {
		gap = l.Spacing["s"] + l.Spacing["xs"]
	}
	n := (l.ContentWidth()+gap)/(cardWidth+gap) + 1
	return max(1, min(n, l.Budget.ShelfItems))
}
