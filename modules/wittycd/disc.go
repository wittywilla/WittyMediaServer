// Package wittycd reads images of audio CDs and plays them the way a CD player
// does: one continuous program area, with tracks, indexes and pregaps as places
// in it rather than as separate files. That is what makes it gapless, and what
// lets a position be a sample rather than an estimate.
//
// Everything in an image is untrusted: sizes are checked before use, a sheet can
// only name files beside itself, and a damaged image gives a smaller disc or a
// clear error, never a crash.
package wittycd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

const (
	// SectorBytes is one sector of CD audio: 1/75 of a second.
	SectorBytes = 2352
	// SectorSamples is the stereo samples in a sector.
	SectorSamples = 588
	// SampleBytes is one stereo sample: two 16-bit values.
	SampleBytes = 4
	// SampleRate is what every audio CD plays at.
	SampleRate = 44100
	// FramesPerSecond is sectors per second, the F of MM:SS:FF.
	FramesPerSecond = 75
	// LeadIn is the 2 seconds before LBA 0, in sectors: MM:SS:FF counts them on a
	// real disc, and disc IDs add them to every offset.
	LeadIn = 150

	// MaxSectors is the longest disc there can be: 99:59:74.
	MaxSectors = 450000
	// MaxTracks is the most a disc's table of contents can hold.
	MaxTracks  = 99
	maxIndexes = 100
	// maxSheet bounds any text or chunk read whole from a sheet or footer.
	maxSheet = 4 << 20
	// sessionGap is the lead-out and lead-in between a CD-Extra's two sessions.
	sessionGap = 11400
)

// Errors a caller may want to tell apart. Each reads as a sentence.
var (
	// ErrNotCD: not a CD image at all, such as a DVD or a data disc's ISO.
	ErrNotCD = errors.New("this isn't an image of an audio CD")
	// ErrNoAudio: a CD image with only data tracks.
	ErrNoAudio = errors.New("this CD image holds only data tracks, so there is nothing to play")
	// ErrUnsupported: a CD image in a form this version can't read.
	ErrUnsupported = errors.New("this CD image is stored in a form WittyCD can't read")
)

// Index is an index point inside a track.
type Index struct {
	Number int `json:"number"`
	LBA    int `json:"lba"`
}

// Track is one track of the table of contents, data tracks included.
type Track struct {
	Number int `json:"number"`
	// Audio is false for a data track, which is listed and never played.
	Audio bool   `json:"audio"`
	Mode  string `json:"mode"` // AUDIO, MODE1/2352 and so on
	// Pregap is where index 0 starts, Start where index 1 starts and End where
	// the next track's pregap starts, all in sectors from LBA 0.
	Pregap int `json:"pregap"`
	Start  int `json:"start"`
	End    int `json:"end"`
	// Indexes above 1.
	Indexes     []Index `json:"indexes,omitempty"`
	PreEmphasis bool    `json:"preEmphasis,omitempty"`
	CopyAllowed bool    `json:"copyAllowed,omitempty"`
	FourChannel bool    `json:"fourChannel,omitempty"`
	ISRC        string  `json:"isrc,omitempty"`
	Session     int     `json:"session,omitempty"`
	Text        Text    `json:"text"`
}

// Sectors is the track's length from index 1 to its end.
func (t *Track) Sectors() int { return t.End - t.Start }

// Text is what a disc says about itself or about one track.
type Text struct {
	Title      string `json:"title,omitempty"`
	Performer  string `json:"performer,omitempty"`
	Songwriter string `json:"songwriter,omitempty"`
	Composer   string `json:"composer,omitempty"`
	Arranger   string `json:"arranger,omitempty"`
	Message    string `json:"message,omitempty"`
}

// Warning is something worth telling the owner about an image that still plays.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Disc is an opened CD image.
type Disc struct {
	Path string `json:"-"`
	// Format is the image's form: cue, toc, ccd, nrg, chd or raw.
	Format  string  `json:"format"`
	Tracks  []Track `json:"tracks"`
	Text    Text    `json:"text"`
	Catalog string  `json:"catalog,omitempty"` // the disc's barcode, if it says
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

	extents []extent
	files   map[string]*os.File
	closers []io.Closer
	sub     *subcode
}

func (d *Disc) warn(code, format string, a ...any) {
	for _, w := range d.Warnings {
		if w.Code == code {
			return
		}
	}
	d.Warnings = append(d.Warnings, Warning{Code: code, Message: fmt.Sprintf(format, a...)})
}

// AudioTracks lists the tracks that can be played, in order.
func (d *Disc) AudioTracks() []*Track {
	out := make([]*Track, 0, len(d.Tracks))
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

// Close lets go of the image's files.
func (d *Disc) Close() error {
	var first error
	for _, c := range d.closers {
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
	}
	d.closers, d.files = nil, nil
	return first
}

// IsImageName says whether a file's name marks it as a CD image's sheet or
// container. .iso, .img and .bin may be one too, which only opening them tells.
func IsImageName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".cue", ".toc", ".ccd", ".nrg", ".chd":
		return true
	}
	return false
}

// MayBeRaw says whether a file with this name might be bare CD audio.
func MayBeRaw(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".iso", ".img", ".bin":
		return true
	}
	return false
}

// Open reads a CD image by what its name says it is. An .img opens through the
// .ccd or .cue beside it when there is one.
func Open(path string) (*Disc, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("can't read this CD image: %w", err)
	}
	if st.IsDir() {
		return nil, ErrNotCD
	}
	d := &Disc{Path: path}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cue":
		err = d.readCue(path)
	case ".toc":
		err = d.readTocFile(path)
	case ".ccd":
		err = d.readCCD(path)
	case ".nrg":
		err = d.readNRG(path)
	case ".chd":
		err = d.readCHD(path)
	case ".iso", ".img", ".bin":
		base := strings.TrimSuffix(path, filepath.Ext(path))
		if sibling := firstExisting(base+".ccd", base+".cue", base+".toc"); sibling != "" {
			return Open(sibling)
		}
		err = d.readRaw(path)
	default:
		return nil, ErrNotCD
	}
	if err == nil {
		err = d.finish()
	}
	if err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

func firstExisting(paths ...string) string {
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// finish checks what a reader built, repairs what it safely can, and works out
// what can only be seen from the audio.
func (d *Disc) finish() error {
	slices.SortFunc(d.extents, func(a, b extent) int { return a.lba - b.lba })
	slices.SortStableFunc(d.Tracks, func(a, b Track) int { return a.Start - b.Start })
	seen := map[int]bool{}
	tracks := make([]Track, 0, min(len(d.Tracks), MaxTracks))
	lost, cut := 0, 0
	for i := range d.Tracks {
		t := d.Tracks[i]
		t.End = min(t.End, MaxSectors)
		if t.Pregap > t.Start || t.Pregap < 0 {
			t.Pregap = t.Start
		}
		if t.End == t.Start && t.Start > 0 {
			cut++ // it starts where the image has already run out
			continue
		}
		if t.Number < 1 || t.Number > MaxTracks || seen[t.Number] || t.Start < 0 || t.End <= t.Start || len(tracks) == MaxTracks {
			lost++
			continue
		}
		if len(tracks) > 0 {
			prev := &tracks[len(tracks)-1]
			if t.Pregap < prev.Start {
				t.Pregap = t.Start
			}
			if t.Pregap < prev.End {
				prev.End = max(prev.Start+1, t.Pregap) // tracks can't overlap
				t.Pregap = max(t.Pregap, prev.End)
				if t.Start < t.Pregap {
					lost++
					continue
				}
			}
		}
		idx := make([]Index, 0, len(t.Indexes))
		for _, x := range t.Indexes {
			if x.LBA > t.Start && x.LBA < t.End && len(idx) < maxIndexes {
				idx = append(idx, x)
			}
		}
		t.Indexes = idx
		seen[t.Number] = true
		tracks = append(tracks, t)
	}
	d.Tracks = tracks
	if lost > 0 {
		d.warn("bad-tracks", "%d entries in this image's table of contents made no sense and were left out.", lost)
	}
	if cut > 0 {
		d.warn("truncated-tracks", "This image is shorter than its table of contents says: %d tracks are missing from it.", cut)
	}
	if len(d.Tracks) == 0 {
		return fmt.Errorf("%w: its table of contents lists no usable tracks", ErrNotCD)
	}
	if len(d.AudioTracks()) == 0 {
		return ErrNoAudio
	}
	if !d.clipToAudio() {
		return fmt.Errorf("the audio this image's table of contents points at is missing, so it can't be played")
	}
	if d.Sessions == 0 {
		d.Sessions = 1
	}
	for i := range d.Tracks {
		if !d.Tracks[i].Audio {
			d.warn("data-track", "Track %d is a data track (a CD-Extra or mixed-mode disc). It is listed but not played.", d.Tracks[i].Number)
		}
	}
	d.inspectAudio()
	return nil
}

// clipToAudio drops or shortens audio tracks past the end of what the image
// really holds, which is how a truncated image shows itself.
func (d *Disc) clipToAudio() bool {
	playable := d.Tracks[:0]
	lost := 0
	for i := range d.Tracks {
		t := d.Tracks[i]
		if !t.Audio {
			playable = append(playable, t)
			continue
		}
		end := d.covered(t.Start, t.End)
		if end <= t.Start {
			lost++
			continue
		}
		if end < t.End {
			d.warn("truncated", "This image is shorter than its table of contents says: track %d ends early.", t.Number)
			t.End = end
		}
		t.Pregap = d.coveredFrom(t.Pregap, t.Start)
		playable = append(playable, t)
	}
	d.Tracks = playable
	if lost > 0 {
		d.warn("truncated-tracks", "This image is shorter than its table of contents says: %d tracks are missing from it.", lost)
	}
	return len(d.AudioTracks()) > 0
}

// covered returns the first sector at or after from that nothing supplies, or to
// if all of it is there.
func (d *Disc) covered(from, to int) int {
	at := from
	for at < to {
		e := d.extentAt(at)
		if e == nil {
			return at
		}
		at = e.lba + e.sectors
	}
	return to
}

// coveredFrom returns the first sector of [from, to) with everything after it
// supplied, for a pregap that is only partly in the image.
func (d *Disc) coveredFrom(from, to int) int {
	at := to
	for at > from {
		e := d.extentAt(at - 1)
		if e == nil {
			return at
		}
		at = max(e.lba, from)
	}
	return from
}

func (d *Disc) extentAt(lba int) *extent {
	i := sort.Search(len(d.extents), func(i int) bool { return d.extents[i].lba+d.extents[i].sectors > lba })
	if i < len(d.extents) && d.extents[i].lba <= lba {
		return &d.extents[i]
	}
	return nil
}

// TotalBytes is the size of the files the image is made of.
func (d *Disc) TotalBytes() int64 {
	var total int64
	for _, f := range d.files {
		if st, err := f.Stat(); err == nil {
			total += st.Size()
		}
	}
	return total
}

// AudioEnd is the sector after the last audio track.
func (d *Disc) AudioEnd() int {
	end := 0
	for _, t := range d.AudioTracks() {
		end = max(end, t.End)
	}
	return end
}

// DurationSec is the disc's playing time, from track 1's index 1 to the last
// track's end. A hidden track is left out, as a player would.
func (d *Disc) DurationSec() float64 {
	audio := d.AudioTracks()
	sectors := 0
	for i, t := range audio {
		if i == 0 {
			sectors += t.End - t.Start
		} else {
			sectors += t.End - t.Pregap
		}
	}
	return float64(sectors) / FramesPerSecond
}
