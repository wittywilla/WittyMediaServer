// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittyinfo

import (
	"fmt"
	"math"
	"regexp"
)

// .wittyaudio: the same container, rules and reader as .wittyinfo, for WittyAudio: a song
// (or an audiobook's file), an album, or an artist. It adds what music has that films don't:
// who made it, track and disc numbers, codes, synced lyrics (with each word's time for
// karaoke, and in other languages), loudness, and trims and fades. Chapters of an audiobook
// are its scenes; parts to skip (a long silence, a spoken intro) are its segments.
// docs/formats/wittyaudio.md.

// FormatAudio is a .wittyaudio's "format", and ExtAudio its extension.
const (
	FormatAudio = "wittyaudio"
	ExtAudio    = ".wittyaudio"
)

// Scopes of a .wittyaudio.
const (
	ScopeSong   = "song"   // one file: Song.wittyaudio
	ScopeAlbum  = "album"  // every song of an album: album.wittyaudio in its folder
	ScopeArtist = "artist" // an artist: artist.wittyaudio in their folder
)

// More artwork roles for music: the back of the sleeve and the artist's picture.
func init() { ArtworkRoles = append(ArtworkRoles, "back", "artist") }

// Limits for lyrics.
const (
	MaxLyricLines = 5000
	MaxLyricWords = 200
)

// Audio is what a .wittyaudio says about music.
type Audio struct {
	Artists     []string `json:"artists,omitempty"`
	Album       string   `json:"album,omitempty"`
	AlbumArtist string   `json:"albumArtist,omitempty"`
	Track       *int     `json:"track,omitempty"`
	TrackTotal  *int     `json:"trackTotal,omitempty"`
	Disc        *int     `json:"disc,omitempty"`
	DiscTotal   *int     `json:"discTotal,omitempty"`
	Composers   []string `json:"composers,omitempty"`
	Lyricists   []string `json:"lyricists,omitempty"`
	Label       string   `json:"label,omitempty"`
	ISRC        string   `json:"isrc,omitempty"`
	Barcode     string   `json:"barcode,omitempty"`
	BPM         *float64 `json:"bpm,omitempty"`
	Key         string   `json:"key,omitempty"` // "A minor", "F#m"
	Moods       []string `json:"moods,omitempty"`
	Explicit    bool     `json:"explicit,omitempty"`
	Compilation bool     `json:"compilation,omitempty"`
	// Biography: an artist's, in a file for an artist.
	Biography string  `json:"biography,omitempty"`
	Lyrics    *Lyrics `json:"lyrics,omitempty"`
	Loudness  *Gain   `json:"loudness,omitempty"`
	// Trim: play only from start to end (a song with a long silence or a hidden track after).
	TrimStart *Seconds `json:"trimStart,omitempty"`
	TrimEnd   *Seconds `json:"trimEnd,omitempty"`
	FadeIn    *Seconds `json:"fadeIn,omitempty"`
	FadeOut   *Seconds `json:"fadeOut,omitempty"`
	Extra
}

// Lyrics are a song's words: plain, or line by line in time (and word by word for karaoke).
type Lyrics struct {
	Language     string      `json:"language,omitempty"`
	Plain        string      `json:"plain,omitempty"`
	Lines        []LyricLine `json:"lines,omitempty"`
	Instrumental bool        `json:"instrumental,omitempty"`
	Extra
}

// LyricLine is one line, when it is sung.
type LyricLine struct {
	Start  Seconds     `json:"start"`
	End    *Seconds    `json:"end,omitempty"`
	Text   string      `json:"text"`
	Singer string      `json:"singer,omitempty"` // for duets: who sings it
	Words  []LyricWord `json:"words,omitempty"`
	Extra
}

// LyricWord is one word's time, for karaoke.
type LyricWord struct {
	Start Seconds  `json:"start"`
	End   *Seconds `json:"end,omitempty"`
	Text  string   `json:"text"`
	Extra
}

// Gain is loudness, as ReplayGain has it: a gain in dB to apply, and the loudest sample (0 to
// 1, or above for clipping).
type Gain struct {
	TrackGain *float64 `json:"trackGain,omitempty"`
	TrackPeak *float64 `json:"trackPeak,omitempty"`
	AlbumGain *float64 `json:"albumGain,omitempty"`
	AlbumPeak *float64 `json:"albumPeak,omitempty"`
	Extra
}

func (x *Audio) UnmarshalJSON(b []byte) error {
	type a Audio
	e, err := decodeKeeping(b, (*a)(x))
	x.Unknown = e
	return err
}
func (x *Audio) MarshalJSON() ([]byte, error) { type a Audio; return encodeKeeping((*a)(x), x.Unknown) }

func (x *Lyrics) UnmarshalJSON(b []byte) error {
	type a Lyrics
	e, err := decodeKeeping(b, (*a)(x))
	x.Unknown = e
	return err
}
func (x *Lyrics) MarshalJSON() ([]byte, error) {
	type a Lyrics
	return encodeKeeping((*a)(x), x.Unknown)
}

func (x *LyricLine) UnmarshalJSON(b []byte) error {
	type a LyricLine
	e, err := decodeKeeping(b, (*a)(x))
	x.Unknown = e
	return err
}
func (x *LyricLine) MarshalJSON() ([]byte, error) {
	type a LyricLine
	return encodeKeeping((*a)(x), x.Unknown)
}

func (x *LyricWord) UnmarshalJSON(b []byte) error {
	type a LyricWord
	e, err := decodeKeeping(b, (*a)(x))
	x.Unknown = e
	return err
}
func (x *LyricWord) MarshalJSON() ([]byte, error) {
	type a LyricWord
	return encodeKeeping((*a)(x), x.Unknown)
}

func (x *Gain) UnmarshalJSON(b []byte) error {
	type a Gain
	e, err := decodeKeeping(b, (*a)(x))
	x.Unknown = e
	return err
}
func (x *Gain) MarshalJSON() ([]byte, error) { type a Gain; return encodeKeeping((*a)(x), x.Unknown) }

// IsAudio says whether a file is a .wittyaudio.
func (x *Info) IsAudio() bool { return x.Format == FormatAudio }

// ExtOf is the extension a file of this format is written with.
func (x *Info) ExtOf() string {
	if x.IsAudio() {
		return ExtAudio
	}
	return Ext
}

var (
	isrcRe = regexp.MustCompile(`^[A-Z]{2}-?[A-Z0-9]{3}-?\d{2}-?\d{5}$`)
	keyRe  = regexp.MustCompile(`^[A-Ga-g][#b♯♭]?\s?(m|min|minor|maj|major)?$`)
)

// checkAudio checks what only a .wittyaudio has, and that a .wittyinfo has none of it.
func (x *Info) checkAudio(c *checker) {
	a := x.Audio
	if !x.IsAudio() {
		if a != nil {
			c.bad("audio", "music details belong in a .wittyaudio")
		}
		return
	}
	if a == nil {
		return
	}
	c.list("audio.artists", a.Artists, MaxShortText)
	c.list("audio.composers", a.Composers, MaxShortText)
	c.list("audio.lyricists", a.Lyricists, MaxShortText)
	c.list("audio.moods", a.Moods, 64)
	for where, v := range map[string]string{"album": a.Album, "albumArtist": a.AlbumArtist, "label": a.Label, "barcode": a.Barcode} {
		c.text("audio."+where, v, MaxShortText)
	}
	c.text("audio.biography", a.Biography, MaxLongText)
	if a.ISRC != "" && !isrcRe.MatchString(a.ISRC) {
		c.bad("audio.isrc", "has to be an ISRC such as GB-AAA-26-00001")
	}
	if a.Key != "" && !keyRe.MatchString(a.Key) {
		c.bad("audio.key", "has to be a key such as A minor or F#m")
	}
	for where, v := range map[string]*int{"track": a.Track, "trackTotal": a.TrackTotal, "disc": a.Disc, "discTotal": a.DiscTotal} {
		if v != nil && (*v < 0 || *v > 9999) {
			c.bad("audio."+where, "has to be between 0 and 9999")
		}
	}
	if a.BPM != nil && (math.IsNaN(*a.BPM) || *a.BPM <= 0 || *a.BPM > 1000) {
		c.bad("audio.bpm", "has to be between 0 and 1000")
	}
	for where, v := range map[string]*Seconds{"trimStart": a.TrimStart, "trimEnd": a.TrimEnd, "fadeIn": a.FadeIn, "fadeOut": a.FadeOut} {
		if v != nil {
			c.time("audio."+where, *v)
		}
	}
	if a.TrimStart != nil && a.TrimEnd != nil && *a.TrimEnd <= *a.TrimStart {
		c.bad("audio.trimEnd", "has to come after the trim's start")
	}
	if g := a.Loudness; g != nil {
		for where, v := range map[string]*float64{"trackGain": g.TrackGain, "albumGain": g.AlbumGain} {
			if v != nil && (math.IsNaN(*v) || *v < -60 || *v > 60) {
				c.bad("audio.loudness."+where, "has to be between -60 and 60 dB")
			}
		}
		for where, v := range map[string]*float64{"trackPeak": g.TrackPeak, "albumPeak": g.AlbumPeak} {
			if v != nil && (math.IsNaN(*v) || *v < 0 || *v > 10) {
				c.bad("audio.loudness."+where, "has to be between 0 and 10")
			}
		}
	}
	checkLyrics(c, "audio.lyrics", a.Lyrics)
}

func checkLyrics(c *checker, w string, l *Lyrics) {
	if l == nil {
		return
	}
	if l.Language != "" && !validTag(l.Language) {
		c.bad(w+".language", "%q is not a language code", l.Language)
	}
	c.text(w+".plain", l.Plain, MaxLongText*5)
	if len(l.Lines) > MaxLyricLines {
		c.bad(w+".lines", "more than %d lines", MaxLyricLines)
		return
	}
	var last Seconds = -1
	for i, ln := range l.Lines {
		lw := fmt.Sprintf("%s.lines[%d]", w, i)
		c.time(lw+".start", ln.Start)
		if ln.Start < last {
			c.bad(lw+".start", "lines have to be in time order")
		}
		last = ln.Start
		if ln.End != nil && *ln.End <= ln.Start {
			c.bad(lw+".end", "has to come after the start")
		}
		c.text(lw+".text", ln.Text, 1000)
		c.text(lw+".singer", ln.Singer, MaxShortText)
		if len(ln.Words) > MaxLyricWords {
			c.bad(lw+".words", "more than %d words", MaxLyricWords)
			continue
		}
		for j, wd := range ln.Words {
			c.time(fmt.Sprintf("%s.words[%d].start", lw, j), wd.Start)
			c.text(fmt.Sprintf("%s.words[%d].text", lw, j), wd.Text, MaxShortText)
		}
	}
}

// formatScopes says which scopes each format has.
var formatScopes = map[string][]string{
	Format:      {ScopeItem, ScopeSeason, ScopeShow},
	FormatAudio: {ScopeSong, ScopeAlbum, ScopeArtist},
}

// NewAudio starts a .wittyaudio of the given scope.
func NewAudio(scope string) *Info {
	return &Info{Format: FormatAudio, Version: Version, Scope: scope, Files: map[string][]byte{}}
}
