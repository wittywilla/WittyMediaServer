package cdrip

import (
	"errors"
	"io"

	"github.com/wittywilla/wittycd"
)

// imageSource reads a CD image through WittyCD, which knows the forms images come in.
type imageSource struct {
	disc   *wittycd.Disc
	info   *Info
	stream *wittycd.Stream
	audio  []*wittycd.Track
}

// IsImage says whether a path looks like a CD image by its name alone.
func IsImage(path string) bool { return wittycd.IsImageName(path) }

// MayBeImage says a path might be bare CD audio, which only opening it tells.
func MayBeImage(path string) bool { return wittycd.MayBeRaw(path) }

func infoOfTOC(info *Info, toc wittycd.TOC) {
	info.DiscID = toc.MusicBrainzID()
	info.TOC = toc.LookupString()
	info.CDDBID = toc.CDDBID()
}

func openImage(path string) (Source, error) {
	d, err := wittycd.Open(path)
	if err != nil {
		return nil, err
	}
	info := &Info{Kind: "audiocd", Path: path, Title: d.Text.Title, Performer: d.Text.Performer, Date: d.Date, Tracks: []Track{}}
	for i := range d.Tracks {
		t := &d.Tracks[i]
		info.Tracks = append(info.Tracks, Track{Number: t.Number, Audio: t.Audio, Pregap: t.Pregap, Start: t.Start, End: t.End, Title: t.Text.Title, Performer: t.Text.Performer, ISRC: t.ISRC, PreEmphasis: t.PreEmphasis})
	}
	for _, w := range d.Warnings {
		info.Warnings = append(info.Warnings, w.Message)
	}
	infoOfTOC(info, d.TOC())
	audio := d.AudioTracks()
	if len(audio) == 0 {
		d.Close()
		return nil, errors.New("this image has no audio tracks")
	}
	// The samples as the disc has them: a copy doesn't change them, even for emphasis.
	s, err := d.NewStream(nil, 1, false)
	if err != nil {
		d.Close()
		return nil, err
	}
	return &imageSource{disc: d, info: info, stream: s, audio: audio}, nil
}

func (s *imageSource) Info() *Info  { return s.info }
func (s *imageSource) Exact() bool  { return true }
func (s *imageSource) Close() error { return s.disc.Close() }

// ReadSectors maps sectors of the disc to the stream, which runs through every audio
// track's pregap and body. What isn't audio is silence.
func (s *imageSource) ReadSectors(lba int, p []byte) (int, error) {
	done := 0
	for done < len(p) {
		off, left := int64(-1), 0
		var pos int64
		for _, t := range s.audio {
			if lba >= t.Pregap && lba < t.End {
				off = pos + int64(lba-t.Pregap)*SectorBytes
				left = (t.End - lba) * SectorBytes
				break
			}
			pos += int64(t.End-t.Pregap) * SectorBytes
		}
		n := min(len(p)-done, SectorBytes)
		if off < 0 {
			clear(p[done : done+n])
			done += n
			lba++
			continue
		}
		if left > 0 {
			n = min(len(p)-done, left)
		}
		got, err := s.stream.ReadAt(p[done:done+n], off)
		done += got
		lba += got / SectorBytes
		if err != nil && !errors.Is(err, io.EOF) {
			return done, err
		}
		if got < n {
			return done, io.ErrUnexpectedEOF
		}
	}
	return done, nil
}
