// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package cd

import (
	"errors"
	"io"
)

// Disc is an opened CD image, in a helper. It is safe for concurrent reads.
type Disc struct {
	// Format is the image's form: cue, toc, ccd, nrg, chd or raw.
	Format  string  `json:"format"`
	Tracks  []Track `json:"tracks"`
	Text    Text    `json:"text"`
	Catalog string  `json:"catalog,omitempty"`
	Genre   string  `json:"genre,omitempty"`
	Date    string  `json:"date,omitempty"`
	// DiscNumber is from the sheet (REM DISCNUMBER), 0 when it doesn't say.
	DiscNumber int `json:"discNumber,omitempty"`
	// Sessions is 2 for a CD-Extra.
	Sessions int `json:"sessions"`
	// HiddenTrack: track 1's pregap holds audio that is not silence.
	HiddenTrack bool `json:"hiddenTrack,omitempty"`
	// Problem is why this disc would not sound like music: "dts" or "noise" with
	// a plain Warning alongside it, "" for a normal disc.
	Problem  string    `json:"problem,omitempty"`
	Warnings []Warning `json:"warnings,omitempty"`
	// HasGraphics: the image holds CD+G subcode (a karaoke disc).
	HasGraphics bool `json:"hasGraphics,omitempty"`
	// What the helper works out, so nothing here does.
	TotalSize int64   `json:"totalBytes"`
	Duration  float64 `json:"durationSec"`
	Table     TOC     `json:"toc"`

	// Engine is the helper's half of this disc.
	Engine DiscEngine `json:"-"`
}

// DiscEngine is what a disc asks of its helper.
type DiscEngine interface {
	NewStream(tracks []int, loops int, deemphasis bool) (Stream, error)
	ReadGraphics(from, sectors int) ([]byte, error)
	Close() error
}

// Stream reads samples of a program: the whole disc as it is laid out, or chosen tracks in a
// chosen order, any number of times.
type Stream interface {
	io.ReaderAt
	Samples() int64
	Size() int64
	// WAV is the program from a sample on, as a WAV file.
	WAV(startSample int64) WAV
	// WriteFLAC writes the program from a sample on, as FLAC.
	WriteFLAC(w io.Writer, startSample int64) error
}

// WAV is a stream as a file.
type WAV interface {
	io.ReaderAt
	Size() int64
}

// ErrNoEngine: nothing has been set to open an image with.
var ErrNoEngine = errors.New("WittyCD isn't on this server")

// Open opens a CD image through the WittyCD version in use. The server sets it as it starts.
var Open = func(string) (*Disc, error) { return nil, ErrNoEngine }

// ForeignAudio, when set, makes a FLAC file of CD audio for a sound file WittyCD doesn't read itself,
// so that a cue sheet can split it. It's here since the server has FFmpeg, and WittyCD asks for it.
var ForeignAudio func(path string) (flac string, err error)

// NewStream makes a program. tracks is nil for the whole disc as laid out.
func (d *Disc) NewStream(tracks []int, loops int, deemphasis bool) (Stream, error) {
	return d.Engine.NewStream(tracks, loops, deemphasis)
}

// ReadGraphics reads a run of a karaoke disc's subcode.
func (d *Disc) ReadGraphics(from, sectors int) ([]byte, error) {
	return d.Engine.ReadGraphics(from, sectors)
}

// Close lets go of the image.
func (d *Disc) Close() error {
	if d.Engine == nil {
		return nil
	}
	return d.Engine.Close()
}

// AudioTracks lists the tracks that can be played, in order.
func (d *Disc) AudioTracks() []*Track {
	var out []*Track
	for i := range d.Tracks {
		if d.Tracks[i].Audio {
			out = append(out, &d.Tracks[i])
		}
	}
	return out
}

// Track finds a track by number.
func (d *Disc) Track(n int) *Track {
	for i := range d.Tracks {
		if d.Tracks[i].Number == n {
			return &d.Tracks[i]
		}
	}
	return nil
}

// TotalBytes is how long the whole disc is as samples.
func (d *Disc) TotalBytes() int64 { return d.TotalSize }

// DurationSec is how long the whole disc plays.
func (d *Disc) DurationSec() float64 { return d.Duration }

// TOC is the table of contents an audio player would read.
func (d *Disc) TOC() TOC { return d.Table }
