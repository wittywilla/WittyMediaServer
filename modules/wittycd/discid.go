package wittycd

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// Disc IDs come from the table of contents and nothing else: no audio, no names
// and no file paths leave the server to look a disc up.

// TOC is a table of contents as disc ID services take it: audio tracks only, with
// the 2-second lead-in added to every offset.
type TOC struct {
	First, Last int
	LeadOut     int
	Offsets     []int // of tracks First to Last
}

// TOC works out the table of contents an audio player would read. Data tracks
// aren't counted. On a CD-Extra the lead-out is where the audio session ended,
// 11400 sectors before the data session if the image kept that gap.
func (d *Disc) TOC() TOC {
	audio := d.AudioTracks()
	if len(audio) == 0 {
		return TOC{}
	}
	t := TOC{First: audio[0].Number, Last: audio[len(audio)-1].Number}
	for _, a := range audio {
		t.Offsets = append(t.Offsets, a.Start+LeadIn)
	}
	end := audio[len(audio)-1].End
	for i := range d.Tracks {
		if n := &d.Tracks[i]; !n.Audio && n.Start >= end {
			if n.Start-sessionGap >= end {
				end = n.Start - sessionGap
			}
			break
		}
	}
	t.LeadOut = end + LeadIn
	return t
}

// MusicBrainzID is the disc's ID in MusicBrainz: 28 characters, from the SHA-1 of
// the table of contents written in hexadecimal.
func (t TOC) MusicBrainzID() string {
	if t.Last == 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(804)
	fmt.Fprintf(&b, "%02X%02X%08X", t.First, t.Last, t.LeadOut)
	for i := 1; i <= 99; i++ {
		v := 0
		if n := i - t.First; n >= 0 && n < len(t.Offsets) {
			v = t.Offsets[n]
		}
		fmt.Fprintf(&b, "%08X", v)
	}
	sum := sha1.Sum([]byte(b.String()))
	s := base64.StdEncoding.EncodeToString(sum[:])
	return strings.NewReplacer("+", ".", "/", "_", "=", "-").Replace(s)
}

// LookupString is the toc= value of a MusicBrainz lookup, which also finds a disc
// nobody has attached that exact ID to. A release is looked up by its track count
// from 1, so a first track above 1 (a data track first) is told as it is.
func (t TOC) LookupString() string {
	parts := []string{strconv.Itoa(t.First), strconv.Itoa(t.Last), strconv.Itoa(t.LeadOut)}
	for _, o := range t.Offsets {
		parts = append(parts, strconv.Itoa(o))
	}
	return strings.Join(parts, "+")
}

// CDDBID is the older eight-digit disc ID. It is only shown, for matching a disc
// against other tools' logs; nothing here looks it up.
func (t TOC) CDDBID() string {
	if len(t.Offsets) == 0 {
		return ""
	}
	sum := 0
	for _, o := range t.Offsets {
		for s := o / FramesPerSecond; s > 0; s /= 10 {
			sum += s % 10
		}
	}
	length := t.LeadOut/FramesPerSecond - t.Offsets[0]/FramesPerSecond
	return fmt.Sprintf("%08x", uint32(sum%255)<<24|uint32(length)<<8|uint32(len(t.Offsets)))
}
