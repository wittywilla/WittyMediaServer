// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package playback

import (
	"slices"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// A viewer's choice of picture size and data rate, for every kind of video: files, discs,
// channels. The player's quality menu sends it with the request that starts a stream
// (the X-WMS-Quality header, or ?quality= where a player can't set headers), and a profile
// keeps a default of its own for home and for away from home. What an administrator allows
// (Limits) always wins. See docs/CLIENT-API.md, Quality.

// Choice is what the viewer asked for. Zero values mean "no limit of my own".
type Choice struct {
	// Set: the viewer chose something, even if that is Original (no limits at all). An unset
	// choice falls back to the profile's saved default.
	Set       bool `json:"-"`
	MaxHeight int  `json:"maxHeight"` // lines, e.g. 720; 0 = the source's own
	MaxKbps   int  `json:"maxKbps"`   // picture and sound together; 0 = no limit
	// VideoCodec: "" keeps any codec the device plays; "h264" converts anything else to H.264,
	// for devices whose other decoders are slow or missing.
	VideoCodec    string `json:"videoCodec,omitempty"`
	AudioKbps     int    `json:"audioKbps,omitempty"`     // converted sound; 0 = the server's setting
	AudioChannels int    `json:"audioChannels,omitempty"` // 2 = stereo, 6 = 5.1; 0 = as the source
}

// Limits are what an administrator allows a profile, or viewers away from home. 0 = none.
type Limits struct {
	MaxHeight int `json:"maxHeight"`
	MaxKbps   int `json:"maxKbps"`
}

// Sane ranges for what a client sends.
const (
	minHeight = 144
	maxHeight = 4320
	minKbps   = 200
	maxKbps   = 400000
)

// VideoCodecs a viewer may prefer. The conversions themselves are H.264, which every
// device plays: the preference decides what is kept as it is.
var VideoCodecs = []string{"h264"}

// AudioRates are the converted sound rates a viewer may pick, in kbit/s.
var AudioRates = []int{96, 128, 160, 192, 256, 320, 384}

// Preset is one entry of the easy menu.
type Preset struct {
	Name   string `json:"name"`
	Height int    `json:"height"`
	// Kbps is the rough total a stream at this size takes, and the limit it is converted to.
	Kbps int `json:"kbps"`
}

// Presets are the easy menu's sizes, largest first. The rates are what an H.264 stream of
// that size looks good at, which is also what it is held to.
var Presets = []Preset{
	{Name: "4K", Height: 2160, Kbps: 20000},
	{Name: "1080p", Height: 1080, Kbps: 8000},
	{Name: "720p", Height: 720, Kbps: 4000},
	{Name: "480p", Height: 480, Kbps: 1500},
	{Name: "360p", Height: 360, Kbps: 800},
}

// PresetsFor lists the presets at or below a picture's size. A wide film is counted by its
// width as well, so a 1920×800 film offers 1080p. 0 (not known) offers them all.
func PresetsFor(width, height int) []Preset {
	h := SizeClass(width, height)
	if h <= 0 {
		return slices.Clone(Presets)
	}
	var out []Preset
	for _, p := range Presets {
		// A little room: a 1072-line encode is a 1080p film.
		if p.Height <= h+h/20 {
			out = append(out, p)
		}
	}
	return out
}

// PresetsForSource is PresetsFor with the source's own rate as well (kbps, 0 = not known). A
// preset is never held to more than the original carries: one at the original's size that
// would take as much or more is left out (it would only be a bigger copy of the same picture,
// and Original is in the menu already), and a smaller one is held to the original's rate.
func PresetsForSource(width, height, kbps int) []Preset {
	all := PresetsFor(width, height)
	if kbps <= 0 {
		return all
	}
	h := SizeClass(width, height)
	out := all[:0]
	for _, p := range all {
		if p.Kbps >= kbps {
			if h > 0 && p.Height >= h-h/20 {
				continue
			}
			p.Kbps = kbps
		}
		out = append(out, p)
	}
	return out
}

// SizeClass is the picture's size as people name it: its height, or for a picture wider
// than 16:9, the height a 16:9 picture of the same width would have. Upright (phone)
// videos count by their shorter side.
func SizeClass(width, height int) int {
	if width > 0 && height > 0 && width < height {
		width, height = height, width
	}
	if width > 0 && height > 0 && width*9 > height*16 {
		return width * 9 / 16
	}
	return height
}

// FitHeight is the height to scale a picture to so that it fits in the box a maxHeight
// stands for (16:9, maxHeight lines). A picture wider than 16:9 comes out shorter, so a
// "720p" 2.39:1 film is 1280 wide like any other 720p one. 0 when nothing needs doing.
func FitHeight(maxHeight, width, height int) int {
	if maxHeight <= 0 {
		return 0
	}
	if width > 0 && height > 0 && width > height && width*9 > height*16 {
		h := maxHeight * 16 * height / (9 * width)
		return max(h/2*2, 2)
	}
	return maxHeight
}

// ParseChoice reads a choice from a header or a query value: comma- or dash-separated
// fields, each a letter and a number: h720 (height), b4000 (kbit/s), a128 (sound kbit/s),
// c2 (channels), vh264 (codec). "original" is a choice with no limits; "" or "auto" is no
// choice. Anything out of range is dropped, never an error.
func ParseChoice(s string) Choice {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "auto" {
		return Choice{}
	}
	c := Choice{Set: true}
	if s == "original" || s == "o" {
		return c
	}
	c = Choice{}
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '-' || r == ';' || r == ' ' }) {
		if len(f) < 2 || len(f) > 12 {
			continue
		}
		if f[0] == 'v' {
			if slices.Contains(VideoCodecs, f[1:]) {
				c.VideoCodec = f[1:]
			}
			continue
		}
		n, err := strconv.Atoi(f[1:])
		if err != nil {
			continue
		}
		switch f[0] {
		case 'h':
			c.MaxHeight = n
		case 'b':
			c.MaxKbps = n
		case 'a':
			c.AudioKbps = n
		case 'c':
			c.AudioChannels = n
		}
	}
	c = c.Clean()
	// Nothing usable in it: no choice, as if nothing had been sent.
	c.Set = c != Choice{}
	return c
}

// Clean drops values outside sane ranges.
func (c Choice) Clean() Choice {
	if c.MaxHeight != 0 && (c.MaxHeight < minHeight || c.MaxHeight > maxHeight) {
		c.MaxHeight = 0
	}
	if c.MaxKbps != 0 && (c.MaxKbps < minKbps || c.MaxKbps > maxKbps) {
		c.MaxKbps = 0
	}
	if !slices.Contains(AudioRates, c.AudioKbps) {
		c.AudioKbps = 0
	}
	if c.AudioChannels != 2 && c.AudioChannels != 6 {
		c.AudioChannels = 0
	}
	if c.VideoCodec != "" && !slices.Contains(VideoCodecs, c.VideoCodec) {
		c.VideoCodec = ""
	}
	return c
}

// String is the choice in ParseChoice's form, the same for the same choice: "" for no
// choice, "original" for one with no limits. It also names streams made for it.
func (c Choice) String() string {
	if !c.Set {
		return ""
	}
	var f []string
	add := func(k string, v int) {
		if v > 0 {
			f = append(f, k+strconv.Itoa(v))
		}
	}
	add("h", c.MaxHeight)
	add("b", c.MaxKbps)
	if c.VideoCodec != "" {
		f = append(f, "v"+c.VideoCodec)
	}
	add("a", c.AudioKbps)
	add("c", c.AudioChannels)
	if len(f) == 0 {
		return "original"
	}
	return strings.Join(f, "-")
}

// Within is the choice held to the limits: the lower of each.
func (c Choice) Within(limits ...Limits) Choice {
	for _, l := range limits {
		c.MaxHeight = Smallest(c.MaxHeight, l.MaxHeight)
		c.MaxKbps = Smallest(c.MaxKbps, l.MaxKbps)
	}
	return c
}

// Quality is the encoder settings with the choice applied: the total rate becomes a hard
// cap (sound within it, the picture the rest, as for viewers away from home), and the
// sound rate the viewer picked, if it fits. The hardware encoders and preset stay the
// administrator's.
func (c Choice) Quality(q ffmpeg.Quality) ffmpeg.Quality {
	if c.AudioKbps > 0 {
		q.AudioRate = strconv.Itoa(c.AudioKbps) + "k"
	}
	if c.MaxKbps > 0 {
		capped := q.WithCap(strconv.Itoa(c.MaxKbps) + "k")
		// A lower cap already there (away from home) stays.
		if have := ffmpeg.ParseRate(q.CapRate); have > 0 && have < ffmpeg.ParseRate(capped.CapRate) {
			capped.CapRate = q.CapRate
		}
		// WithCap keeps sound small on a small total; a rate the viewer chose themself stays
		// if the picture still gets most of it.
		if c.AudioKbps > 0 && c.AudioKbps*4 <= c.MaxKbps {
			capped.AudioRate = q.AudioRate
		}
		q = capped
	}
	return q
}

// Profile is the client's profile narrowed by the choice: a preferred codec keeps only that
// one as it is, and stereo means more channels are converted.
func (c Choice) Profile(p Profile) Profile {
	if c.VideoCodec != "" {
		keep := func(list []string) []string {
			out := []string{}
			if slices.Contains(list, c.VideoCodec) {
				out = append(out, c.VideoCodec)
			}
			return out
		}
		p.VideoCodecs, p.HLSVideo, p.FMP4Video = keep(p.VideoCodecs), keep(p.HLSVideo), keep(p.FMP4Video)
	}
	switch c.AudioChannels {
	case 2:
		p.MaxAudioChannels, p.SurroundOutput = 2, false
	case 6:
		p.SurroundOutput = true
	}
	return p
}

// Request fills a request's limits from the choice.
func (c Choice) Request(q Request) Request {
	q.MaxHeight = Smallest(q.MaxHeight, c.MaxHeight)
	q.MaxBitrateKbps = Smallest(q.MaxBitrateKbps, c.MaxKbps)
	return q
}
